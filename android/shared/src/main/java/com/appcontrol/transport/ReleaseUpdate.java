package com.appcontrol.transport;

import android.content.Context;
import android.content.pm.PackageInfo;
import android.content.pm.PackageManager;
import java.io.File;
import java.io.InputStream;
import java.io.ByteArrayOutputStream;
import java.net.URL;
import java.security.KeyFactory;
import java.security.MessageDigest;
import java.security.spec.X509EncodedKeySpec;
import java.time.Instant;
import java.util.Base64;
import java.util.function.BooleanSupplier;
import javax.net.ssl.HttpsURLConnection;

/** Fetch/checkpoint/stage a self-update using compiled authority. No installer. */
public final class ReleaseUpdate {
    private ReleaseUpdate() { }
    /** Some older Android providers cannot verify our required update algorithm. */
    public static boolean verificationSupported() {
        try {
            KeyFactory.getInstance("Ed25519");
            java.security.Signature.getInstance("Ed25519");
            return true;
        } catch (java.security.GeneralSecurityException unavailable) { return false; }
    }
    public static final class Prepared implements AutoCloseable {
        final File apk; final byte[] envelope; final String publicKey,origin,certificate;final long minimum;
        private final String versionName;private final long versionCode;
        private Prepared(File apk,byte[] envelope,String publicKey,String origin,String certificate,long minimum,PackageInfo info){
            this.apk=apk;this.envelope=envelope.clone();this.publicKey=publicKey;this.origin=origin;this.certificate=certificate;this.minimum=minimum;
            this.versionName=info.versionName;this.versionCode=info.getLongVersionCode();
        }
        public String versionName(){return versionName;}
        public long versionCode(){return versionCode;}
        @Override public void close(){apk.delete();}
    }
    /** Null means the signed server metadata offers no newer version. */
    public static Prepared prepare(Context context, String publicKey, String origin, String certificate,
                               long compiledMinimum, BooleanSupplier stopped) {
        File staged = null; boolean complete = false;
        try {
            Context app = context.getApplicationContext();
            Thread owner = Thread.currentThread();
            BooleanSupplier cancelled = () -> owner.isInterrupted() || stopped.getAsBoolean();
            if (cancelled.getAsBoolean() || publicKey.isEmpty() || !certificate.matches("[a-f0-9]{64}")) throw new IllegalArgumentException();
            URL base = ServerOrigin.require(origin, origin);
            if (!base.toString().equals(origin)) throw new IllegalArgumentException();
            byte[] key = Base64.getDecoder().decode(publicKey);
            if (key.length != 44 || !Base64.getEncoder().encodeToString(key).equals(publicKey)) throw new IllegalArgumentException();
            KeyFactory.getInstance("Ed25519").generatePublic(new X509EncodedKeySpec(key));
            PackageManager manager = app.getPackageManager();
            PackageInfo installed = manager.getPackageInfo(app.getPackageName(), PackageManager.GET_SIGNING_CERTIFICATES);
            if (installed.signingInfo == null || installed.signingInfo.hasMultipleSigners()) throw new IllegalArgumentException();
            android.content.pm.Signature[] signers = installed.signingInfo.getApkContentsSigners();
            if (signers == null || signers.length != 1) throw new IllegalArgumentException();
            StringBuilder actual = new StringBuilder();
            for (byte item : MessageDigest.getInstance("SHA-256").digest(signers[0].toByteArray())) actual.append(String.format(java.util.Locale.ROOT, "%02x", item & 255));
            if (!actual.toString().equals(certificate)) throw new IllegalArgumentException();
            ReleaseState state = new ReleaseState(app, publicKey, origin, certificate, compiledMinimum);
            state.minimumSequence(); // Refuse damaged or changed authority before networking.
            byte[] envelope = metadata(new URL(base, "/v1/releases/current"), cancelled);
            org.json.JSONObject verified=state.verifyAndRemember(envelope, Instant.now());
            org.json.JSONArray entries=verified.getJSONArray("apps");
            for(int i=0;i<entries.length();i++)if(app.getPackageName().equals(entries.getJSONObject(i).getString("package")) && entries.getJSONObject(i).getLong("versionCode")<=installed.getLongVersionCode())return null;
            staged = ReleaseDownload.stage(app, envelope, publicKey, origin, certificate, state.minimumSequence(), cancelled);
            PackageInfo candidate = ReleaseArtifact.inspect(manager, staged, envelope, publicKey, origin, certificate,
                state.minimumSequence(), app.getPackageName(), Instant.now());
            installed = manager.getPackageInfo(app.getPackageName(), PackageManager.GET_SIGNING_CERTIFICATES);
            ReleaseArtifact.requireUpgrade(installed, candidate);
            if (cancelled.getAsBoolean()) throw new IllegalArgumentException();
            complete = true;
            return new Prepared(staged,envelope,publicKey,origin,certificate,state.minimumSequence(),candidate);
        } catch (Exception failure) { throw new IllegalArgumentException("Update preparation failed"); }
        finally { if (!complete && staged != null && !staged.delete()) throw new IllegalStateException("Update staging cleanup failed"); }
    }
    private static byte[] metadata(URL url, BooleanSupplier stopped) throws Exception {
        HttpsURLConnection connection = (HttpsURLConnection) url.openConnection();
        RequestDeadline deadline = new RequestDeadline(stopped, connection::disconnect, 30_000);
        try {
            deadline.checkActive();
            connection.setRequestMethod("GET");connection.setInstanceFollowRedirects(false);connection.setUseCaches(false);
            connection.setConnectTimeout(15000);connection.setReadTimeout(15000);
            connection.setRequestProperty("Accept","application/json");connection.setRequestProperty("Accept-Encoding","identity");
            if(connection.getResponseCode()!=200)throw new IllegalArgumentException();
            deadline.checkActive();
            String encoding=connection.getHeaderField("Content-Encoding");long declared=connection.getContentLengthLong();
            if(declared>65536||encoding!=null&&!encoding.equalsIgnoreCase("identity"))throw new IllegalArgumentException();
            byte[] bytes;
            try(InputStream input=connection.getInputStream();ByteArrayOutputStream output=new ByteArrayOutputStream()){
                byte[] buffer=new byte[8192];int count;
                while(output.size()<65537 && (count=input.read(buffer,0,Math.min(buffer.length,65537-output.size())))!=-1){
                    deadline.checkActive();output.write(buffer,0,count);
                }
                bytes=output.toByteArray();
            }
            deadline.checkActive();
            if(bytes.length==0||bytes.length>65536||declared!=-1&&declared!=bytes.length)throw new IllegalArgumentException();
            return bytes;
        }finally{deadline.close();connection.disconnect();}
    }
}
