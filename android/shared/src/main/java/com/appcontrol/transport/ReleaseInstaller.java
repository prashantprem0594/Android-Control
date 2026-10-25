package com.appcontrol.transport;

import android.app.Activity;
import android.app.ActivityOptions;
import android.app.PendingIntent;
import android.content.Context;
import android.content.Intent;
import android.content.SharedPreferences;
import android.content.pm.PackageInfo;
import android.content.pm.PackageInstaller;
import android.content.pm.PackageManager;
import android.os.Build;
import java.io.FileInputStream;
import java.io.OutputStream;
import java.security.MessageDigest;
import java.time.Instant;
import java.util.UUID;
import java.util.function.BooleanSupplier;
import org.json.JSONArray;
import org.json.JSONObject;

/** Explicitly requested same-package updates. Never uninstalls or clears data. */
public final class ReleaseInstaller {
    public static final String RESULT_ACTION="com.appcontrol.UPDATE_RESULT";
    private static final Object LOCK=new Object();
    private ReleaseInstaller(){}
    static SharedPreferences state(Context context){return context.getSharedPreferences("release-install",Context.MODE_PRIVATE);}
    private static String hex(byte[] value){StringBuilder out=new StringBuilder();for(byte item:value)out.append(String.format(java.util.Locale.ROOT,"%02x",item&255));return out.toString();}
    public static boolean hasPending(Context context){return state(context).contains("nonce");}
    static PendingIntent callback(Context app,Class<? extends Activity> activity,int session,String nonce){
        Intent intent=new Intent(app,activity).setAction(RESULT_ACTION).setData(android.net.Uri.parse("appcontrol-update://"+app.getPackageName()+"/"+session))
            .putExtra("updateNonce",nonce).putExtra(PackageInstaller.EXTRA_SESSION_ID,session).addFlags(Intent.FLAG_ACTIVITY_NEW_TASK|Intent.FLAG_ACTIVITY_SINGLE_TOP);
        android.os.Bundle options=null;
        if(Build.VERSION.SDK_INT>=34){
            ActivityOptions settings=ActivityOptions.makeBasic();
            settings.setPendingIntentCreatorBackgroundActivityStartMode(Build.VERSION.SDK_INT>=36?
                ActivityOptions.MODE_BACKGROUND_ACTIVITY_START_ALLOW_IF_VISIBLE:ActivityOptions.MODE_BACKGROUND_ACTIVITY_START_ALLOWED);
            options=settings.toBundle();
        }
        return PendingIntent.getActivity(app,session,intent,PendingIntent.FLAG_CANCEL_CURRENT|PendingIntent.FLAG_MUTABLE,options);
    }
    public static int submit(Context context,ReleaseUpdate.Prepared prepared,Class<? extends Activity> activity,BooleanSupplier stopped){
        synchronized(LOCK){
            Context app=context.getApplicationContext();PackageInstaller installer=app.getPackageManager().getPackageInstaller();
            int id=-1;boolean commitAttempted=false;PackageInstaller.Session session=null;
            try{
                if(hasPending(app)||stopped.getAsBoolean()||Thread.currentThread().isInterrupted())throw new IllegalStateException();
                ReleaseState checkpoint=new ReleaseState(app,prepared.publicKey,prepared.origin,prepared.certificate,prepared.minimum);
                JSONObject metadata=checkpoint.verifyAndRemember(prepared.envelope,Instant.now());
                PackageManager manager=app.getPackageManager();
                PackageInfo candidate=ReleaseArtifact.inspect(manager,prepared.apk,prepared.envelope,prepared.publicKey,prepared.origin,prepared.certificate,checkpoint.minimumSequence(),app.getPackageName(),Instant.now());
                ReleaseArtifact.requireUpgrade(manager.getPackageInfo(app.getPackageName(),PackageManager.PackageInfoFlags.of(PackageManager.GET_SIGNING_CERTIFICATES)),candidate);
                JSONObject expected=null;JSONArray apps=metadata.getJSONArray("apps");for(int i=0;i<apps.length();i++)if(app.getPackageName().equals(apps.getJSONObject(i).getString("package")))expected=apps.getJSONObject(i);
                if(expected==null)throw new IllegalArgumentException();long size=expected.getLong("bytes");
                PackageInstaller.SessionParams params=new PackageInstaller.SessionParams(PackageInstaller.SessionParams.MODE_FULL_INSTALL);
                params.setAppPackageName(app.getPackageName());params.setSize(size);params.setRequireUserAction(PackageInstaller.SessionParams.USER_ACTION_REQUIRED);
                id=installer.createSession(params);session=installer.openSession(id);
                String nonce=UUID.randomUUID().toString();
                if(!state(app).edit().clear().putInt("session",id).putString("nonce",nonce).putLong("version",candidate.getLongVersionCode())
                    .putString("certificate",prepared.certificate).putString("origin",prepared.origin).putString("publicKey",prepared.publicKey)
                    .putString("envelope",java.util.Base64.getEncoder().encodeToString(prepared.envelope)).putString("status","installing").commit())throw new IllegalStateException();
                MessageDigest digest=MessageDigest.getInstance("SHA-256");long total=0;
                try(FileInputStream input=new FileInputStream(prepared.apk);OutputStream output=session.openWrite("base.apk",0,size)){
                    byte[] buffer=new byte[65536];int count;
                    while((count=input.read(buffer))!=-1){
                        if(stopped.getAsBoolean()||Thread.currentThread().isInterrupted())throw new IllegalStateException();
                        total+=count;if(total>size)throw new IllegalArgumentException();digest.update(buffer,0,count);output.write(buffer,0,count);
                    }
                    if(total!=size||!hex(digest.digest()).equals(expected.getString("sha256")))throw new IllegalArgumentException();
                    session.fsync(output);
                }
                checkpoint.verifyAndRemember(prepared.envelope,Instant.now());
                if(stopped.getAsBoolean()||Thread.currentThread().isInterrupted())throw new IllegalStateException();
                PendingIntent result=callback(app,activity,id,nonce);
                commitAttempted=true;session.commit(result.getIntentSender());
                prepared.close();return id;
            }catch(Exception failure){throw new IllegalStateException(commitAttempted?"Update submission uncertain":"Update could not be submitted");}
            finally{
                if(!commitAttempted&&id>=0){try{installer.abandonSession(id);}catch(Exception ignored){}
                    if(state(app).getInt("session",-1)==id)state(app).edit().clear().putString("status","not_installed").commit();}
                if(session!=null)session.close();
            }
        }
    }
    /** Called only by the private activity targeted by our session PendingIntent. */
    public static Intent receive(Context context,Intent intent){
        try{
            SharedPreferences saved=state(context);String nonce=saved.getString("nonce",null);
            if(!RESULT_ACTION.equals(intent.getAction())||nonce==null||!nonce.equals(intent.getStringExtra("updateNonce"))||intent.getIntExtra(PackageInstaller.EXTRA_SESSION_ID,-1)!=saved.getInt("session",-2))return null;
            int status=intent.getIntExtra(PackageInstaller.EXTRA_STATUS,Integer.MAX_VALUE);
            if(status==PackageInstaller.STATUS_PENDING_USER_ACTION){
                Intent confirmation=intent.getParcelableExtra(Intent.EXTRA_INTENT,Intent.class);
                if(confirmation!=null)saved.edit().putString("status","confirmation_needed").commit();return confirmation;
            }
            if(status==PackageInstaller.STATUS_SUCCESS)saved.edit().clear().putString("status","updated").commit();
            else if(status>=PackageInstaller.STATUS_FAILURE&&status<=PackageInstaller.STATUS_FAILURE_TIMEOUT)saved.edit().clear().putString("status","not_installed").commit();
        }catch(Exception ignored){}
        return null;
    }
    public static String refresh(Context context){
        try{
            SharedPreferences saved=state(context);if(!saved.contains("nonce"))return saved.getString("status","idle");
            PackageManager manager=context.getPackageManager();PackageInfo current=manager.getPackageInfo(context.getPackageName(),PackageManager.PackageInfoFlags.of(PackageManager.GET_SIGNING_CERTIFICATES));
            if(current.getLongVersionCode()>=saved.getLong("version",Long.MAX_VALUE)&&current.signingInfo!=null&&!current.signingInfo.hasMultipleSigners()){
                android.content.pm.Signature[] signers=current.signingInfo.getApkContentsSigners();
                if(signers!=null&&signers.length==1&&hex(MessageDigest.getInstance("SHA-256").digest(signers[0].toByteArray())).equals(saved.getString("certificate",null))){saved.edit().clear().putString("status","updated").commit();return "updated";}
            }
            PackageInstaller.SessionInfo session=manager.getPackageInstaller().getSessionInfo(saved.getInt("session",-1));
            if(session==null){saved.edit().clear().putString("status","not_installed").commit();return "not_installed";}
            return saved.getString("status","installing");
        }catch(Exception ignored){return "unknown";}
    }
    public static void validatePending(Context context,String publicKey,String origin,String certificate,long minimum){
        try{
            SharedPreferences saved=state(context);
            if(!saved.contains("nonce")||!publicKey.equals(saved.getString("publicKey",null))||!origin.equals(saved.getString("origin",null))||!certificate.equals(saved.getString("certificate",null)))throw new IllegalStateException();
            String encoded=saved.getString("envelope","");if(encoded.length()==0||encoded.length()>87384)throw new IllegalStateException();
            byte[] raw=java.util.Base64.getDecoder().decode(encoded);if(raw.length>65536)throw new IllegalStateException();
            new ReleaseState(context,publicKey,origin,certificate,minimum).verifyAndRemember(raw,Instant.now());
        }catch(Exception failure){throw new IllegalStateException("Pending update authority or freshness changed");}
    }
    public static void resume(Context context,Class<? extends Activity> activity,String publicKey,String origin,String certificate,long minimum){
        try{
            validatePending(context,publicKey,origin,certificate,minimum);
            SharedPreferences saved=state(context);int id=saved.getInt("session",-1);String nonce=saved.getString("nonce",null);
            if(nonce==null)throw new IllegalStateException();
            PackageInstaller installer=context.getPackageManager().getPackageInstaller();PackageInstaller.SessionInfo info=installer.getSessionInfo(id);
            if(info==null||!context.getPackageName().equals(info.getAppPackageName()))throw new IllegalStateException();
            if(Thread.currentThread().isInterrupted())throw new IllegalStateException();
            try(PackageInstaller.Session session=installer.openSession(id)){session.commit(callback(context,activity,id,nonce).getIntentSender());}
        }catch(Exception failure){throw new IllegalStateException("Update session could not be resumed");}
    }
    public static void cancel(Context context){
        try{
            SharedPreferences saved=state(context);int id=saved.getInt("session",-1);
            if(!saved.contains("nonce"))return;
            context.getPackageManager().getPackageInstaller().abandonSession(id);
            if(!saved.edit().clear().putString("status","not_installed").commit())throw new IllegalStateException();
        }catch(Exception failure){throw new IllegalStateException("Update cancellation could not be confirmed");}
    }
}
