package com.appcontrol.transport;

import android.app.Activity;
import android.app.AlertDialog;
import android.app.admin.DevicePolicyManager;
import android.content.Intent;
import android.net.Uri;
import android.os.Bundle;
import android.provider.Settings;
import android.view.View;
import android.widget.Button;
import android.widget.LinearLayout;
import android.widget.ScrollView;
import android.widget.TextView;
import java.util.concurrent.ExecutorService;
import java.util.concurrent.Executors;

/** Private update screen: every install starts with an explicit local user press. */
public abstract class ReleaseActivity extends Activity {
    protected abstract String updatePublicKey();
    protected abstract String updateOrigin();
    protected abstract String updateCertificate();
    protected abstract long updateMinimum();
    private final ExecutorService worker=Executors.newSingleThreadExecutor();
    private TextView status,details;
    private Button check,install,confirm,resume,cancel;
    private ReleaseUpdate.Prepared prepared;
    private Intent confirmation;
    private boolean busy,submitting;
    private Button button(LinearLayout column,String label,Runnable action){
        Button button=new Button(this);button.setText(label);AppAppearance.styleButton(button,label.equals("Install verified update")||label.equals("Check for update"));
        button.setOnClickListener(view->{if(!busy)action.run();});column.addView(button);return button;
    }
    @Override public void onCreate(Bundle saved){
        super.onCreate(saved);
        AppAppearance.prepare(this);
        LinearLayout column=AppAppearance.column(this);
        column.addView(AppAppearance.text(this,"APP CONTROL / UPDATES",12,true));
        column.addView(AppAppearance.text(this,"App updates",30,true));
        LinearLayout summary=AppAppearance.card(this);column.addView(summary);
        status=AppAppearance.text(this,"",18,true);summary.addView(status);
        details=AppAppearance.text(this,"",14,false);AppAppearance.status(details);summary.addView(details);
        check=button(column,"Check for update",this::download);
        install=button(column,"Install verified update",this::requestInstall);
        confirm=button(column,"Open Android confirmation",()->{
            if(confirmation!=null)try{
                ReleaseInstaller.validatePending(this,updatePublicKey(),updateOrigin(),updateCertificate(),updateMinimum());
                startActivity(confirmation);confirmation=null;render();
            }catch(Exception failure){details.setText("This update could not be confirmed. Cancel it and check for a fresh update.");}
        });
        resume=button(column,"Resume pending installation",()->operation(()->ReleaseInstaller.resume(this,getClass(),updatePublicKey(),updateOrigin(),updateCertificate(),updateMinimum())));
        cancel=button(column,"Cancel pending update",()->new AlertDialog.Builder(this).setTitle("Cancel this update?")
            .setMessage("The staged update will be abandoned if Android still permits cancellation.")
            .setNegativeButton("Keep update",null).setPositiveButton("Cancel update",(dialog,which)->operation(()->ReleaseInstaller.cancel(this))).show());
        button(column,"Refresh update status",this::render);
        ScrollView scroll=new ScrollView(this);scroll.addView(column);setContentView(scroll);
        acceptResult(getIntent());render();
    }
    @Override protected void onResume(){super.onResume();if(status!=null)render();}
    @Override protected void onNewIntent(Intent intent){super.onNewIntent(intent);setIntent(intent);acceptResult(intent);render();}
    private void acceptResult(Intent intent){
        Intent action=ReleaseInstaller.receive(this,intent);
        if(action!=null){confirmation=action;details.setText("Android needs your confirmation. Keep this screen open to finish the update.");}
    }
    private void render(){
        String observed=ReleaseInstaller.refresh(this);
        switch(observed){
            case "updated":status.setText("Update installed.");break;
            case "not_installed":status.setText("The update was not installed.");break;
            case "confirmation_needed":status.setText("Waiting for Android confirmation.");break;
            case "installing":status.setText("Installation result pending.");break;
            case "unknown":status.setText("Installation status is uncertain. Refresh before retrying.");break;
            default:status.setText("Check for a signed update from your server.");
        }
        boolean pending=ReleaseInstaller.hasPending(this),configured=!updatePublicKey().isEmpty();
        boolean verificationSupported=ReleaseUpdate.verificationSupported();
        check.setEnabled(!busy&&!pending&&configured&&verificationSupported);
        install.setEnabled(!busy&&!pending&&prepared!=null);
        confirm.setVisibility(confirmation==null?View.GONE:View.VISIBLE);confirm.setEnabled(!busy);
        resume.setVisibility(pending?View.VISIBLE:View.GONE);resume.setEnabled(!busy);
        cancel.setVisibility(pending?View.VISIBLE:View.GONE);cancel.setEnabled(!busy);
        if(!configured)details.setText("App updates are not configured. Contact your server operator.");
        else if(!verificationSupported)details.setText("This Android version cannot verify private updates. Ask your operator to install a signed update using Android's installer.");
    }
    private void download(){
        if(prepared!=null){prepared.close();prepared=null;}
        busy=true;details.setText("Checking and verifying the update...");render();
        worker.execute(()->{
            ReleaseUpdate.Prepared downloaded=null;boolean failed=false;
            try{downloaded=ReleaseUpdate.prepare(getApplicationContext(),updatePublicKey(),updateOrigin(),updateCertificate(),updateMinimum(),()->Thread.currentThread().isInterrupted());}
            catch(Exception failure){failed=true;}
            ReleaseUpdate.Prepared ready=downloaded;boolean error=failed;
            runOnUiThread(()->{
                if(isDestroyed()){if(ready!=null)ready.close();return;}
                busy=false;prepared=ready;
                details.setText(error?"A compatible update could not be verified. Check your connection or contact your server operator.":ready==null?"No newer version is published.":"Verified update: "+ready.versionName()+". Installation requires your confirmation.");render();
            });
        });
    }
    private void requestInstall(){
        if(prepared==null)return;
        try{
            DevicePolicyManager manager=getSystemService(DevicePolicyManager.class);
            boolean owned=manager!=null&&manager.isDeviceOwnerApp(getPackageName());
            if(!owned&&!getPackageManager().canRequestPackageInstalls()){
                details.setText("Allow this app to install updates in Android settings, then return and press Install again.");
                startActivity(new Intent(Settings.ACTION_MANAGE_UNKNOWN_APP_SOURCES,Uri.parse("package:"+getPackageName())));return;
            }
        }catch(Exception failure){details.setText("Android installation permission is unavailable.");return;}
        new AlertDialog.Builder(this).setTitle("Install version "+prepared.versionName()+"?")
            .setMessage("Your app data will be retained. Keep this screen open for Android confirmation.")
            .setNegativeButton("Cancel",null).setPositiveButton("Install",(dialog,which)->submit()).show();
    }
    private void submit(){
        ReleaseUpdate.Prepared selected=prepared;if(selected==null||busy)return;
        busy=true;submitting=true;details.setText("Submitting the verified update to Android...");render();
        worker.execute(()->{
            boolean failed=false;
            try{ReleaseInstaller.submit(getApplicationContext(),selected,getClass(),()->Thread.currentThread().isInterrupted());}
            catch(Exception failure){failed=true;}
            finally{selected.close();}
            boolean error=failed;
            runOnUiThread(()->{if(isDestroyed())return;busy=false;submitting=false;prepared=null;
                details.setText(error?"Submission could not be confirmed. Refresh update status before retrying.":"The update was submitted. Awaiting Android's result.");render();});
        });
    }
    private void operation(Runnable action){
        busy=true;render();worker.execute(()->{
            boolean failed=false;try{action.run();}catch(Exception failure){failed=true;}
            boolean error=failed;runOnUiThread(()->{if(isDestroyed())return;busy=false;if(error)details.setText("Android could not confirm the request. Refresh update status.");render();});
        });
    }
    @Override protected void onDestroy(){
        worker.shutdownNow();if(prepared!=null&&!submitting)prepared.close();super.onDestroy();
    }
}
