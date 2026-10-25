package com.appcontrol.transport;

import android.content.Context;
import android.content.pm.PackageInfo;
import android.content.pm.PackageManager;
import android.os.Bundle;
import java.io.File;
import java.io.FileInputStream;
import java.io.FileOutputStream;
import java.nio.charset.StandardCharsets;
import java.security.KeyPair;
import java.security.KeyPairGenerator;
import java.security.MessageDigest;
import java.security.Signature;
import java.time.Instant;
import java.util.Base64;
import org.json.JSONArray;
import org.json.JSONObject;

/** Real disposable-signed release APK; generated metadata key never leaves memory. */
public final class ReleaseArtifactProbe {
    private static final String ORIGIN = "https://release-manifest-fixture.example.org";
    private static void require(boolean value) { if (!value) throw new IllegalStateException("Artifact invariant failed"); }
    private static String hash(File file) throws Exception {
        MessageDigest digest = MessageDigest.getInstance("SHA-256");
        try (FileInputStream stream = new FileInputStream(file)) {
            byte[] buffer = new byte[65536]; int count;
            while ((count = stream.read(buffer)) != -1) digest.update(buffer, 0, count);
        }
        return hex(digest.digest());
    }
    private static String hex(byte[] value) {
        StringBuilder text = new StringBuilder();
        for (byte item : value) text.append(String.format(java.util.Locale.ROOT, "%02x", item & 255));
        return text.toString();
    }
    private static byte[] sign(JSONObject manifest, KeyPair key) throws Exception {
        byte[] payload = manifest.toString().getBytes(StandardCharsets.UTF_8);
        Signature signature = Signature.getInstance("Ed25519"); signature.initSign(key.getPrivate());
        signature.update("app-control-release-manifest-v1\n".getBytes(StandardCharsets.UTF_8)); signature.update(payload);
        return new JSONObject().put("version", 1).put("payload", Base64.getUrlEncoder().withoutPadding().encodeToString(payload))
            .put("signature", Base64.getUrlEncoder().withoutPadding().encodeToString(signature.sign())).toString().getBytes(StandardCharsets.UTF_8);
    }
    private static JSONObject metadata(File file, PackageInfo info, String certificate, Instant now) throws Exception {
        JSONArray apps = new JSONArray();
        for (String name : new String[]{"parent", "child"}) apps.put(new JSONObject().put("package", "com.appcontrol."+name)
            .put("versionCode", info.getLongVersionCode()).put("versionName", info.versionName)
            .put("minSdk", info.applicationInfo.minSdkVersion).put("targetSdk", info.applicationInfo.targetSdkVersion)
            .put("url", ORIGIN+"/releases/1/"+name+"-release.apk").put("bytes", file.length())
            .put("sha256", hash(file)).put("signingCertificateSHA256", certificate));
        return new JSONObject().put("version", 1).put("sequence", 1).put("issuedAt", now.toString())
            .put("expiresAt", now.plusSeconds(3600).toString()).put("origin", ORIGIN).put("apps", apps);
    }
    private interface Check { void run() throws Exception; }
    private static void rejected(Check check) throws Exception {
        boolean refused = false; try { check.run(); } catch (IllegalArgumentException expected) { refused = true; } require(refused);
    }
    public static Bundle run(Context app, Context tests) {
        Bundle result = new Bundle(); String stage = "fixture_guard";
        File file = new File(app.getCacheDir(), "fixture-release-artifact.apk");
        boolean created = false;
        try {
            require(android.os.Build.FINGERPRINT.startsWith("google/sdk_gphone64_x86_64/emu64xa:") && !file.exists());
            require(file.createNewFile()); created = true;
            try (java.io.InputStream input = tests.getAssets().open("fixture-release.apk"); FileOutputStream output = new FileOutputStream(file)) {
                byte[] buffer = new byte[65536]; int count; long bytes = 0;
                while ((count = input.read(buffer)) != -1) { bytes += count; require(bytes <= 16*1024*1024); output.write(buffer, 0, count); }
            }
            PackageManager manager = app.getPackageManager();
            PackageInfo info = manager.getPackageArchiveInfo(file.toString(), PackageManager.PackageInfoFlags.of(PackageManager.GET_SIGNING_CERTIFICATES));
            require(info != null && "com.appcontrol.parent".equals(info.packageName) && info.signingInfo != null);
            String certificate = hex(MessageDigest.getInstance("SHA-256").digest(info.signingInfo.getApkContentsSigners()[0].toByteArray()));
            KeyPair key = KeyPairGenerator.getInstance("Ed25519").generateKeyPair();
            String publicKey = Base64.getEncoder().encodeToString(key.getPublic().getEncoded());
            Instant now = Instant.now(); JSONObject manifest = metadata(file, info, certificate, now);
            stage = "replay_persistence";
            android.content.SharedPreferences replay=app.getSharedPreferences("fixture-release-state",Context.MODE_PRIVATE);
            require(replay.getAll().isEmpty());
            try {
                ReleaseState state=new ReleaseState(app,publicKey,ORIGIN,certificate,0,"fixture-release-state");
                require(state.minimumSequence()==0);
                state.verifyAndRemember(sign(manifest,key),now);
                require(new ReleaseState(app,publicKey,ORIGIN,certificate,0,"fixture-release-state").minimumSequence()==1);
                state.verifyAndRemember(sign(manifest,key),now);
                JSONObject conflicting=new JSONObject(manifest.toString());conflicting.getJSONArray("apps").getJSONObject(0).put("versionName","changed-fixture");
                rejected(()->state.verifyAndRemember(sign(conflicting,key),now));
                JSONObject newer=new JSONObject(manifest.toString()).put("sequence",2);
                for(int i=0;i<2;i++){JSONObject item=newer.getJSONArray("apps").getJSONObject(i);item.put("url",item.getString("url").replace("/releases/1/","/releases/2/"));}
                state.verifyAndRemember(sign(newer,key),now);require(state.minimumSequence()==2);
                rejected(()->state.verifyAndRemember(sign(manifest,key),now));
                rejected(()->state.verifyAndRemember(sign(newer,key),now.plusSeconds(3600)));
                byte[] invalid=sign(newer,key);invalid[0]^=1;rejected(()->state.verifyAndRemember(invalid,now));require(state.minimumSequence()==2);
                ReleaseState differentAuthority=new ReleaseState(app,publicKey,"https://other.example.org",certificate,0,"fixture-release-state");
                rejected(()->differentAuthority.verifyAndRemember(sign(newer,key),now));require(state.minimumSequence()==2);
                require(replay.edit().putString("sequence","corrupt-fixture").commit());
                rejected(()->state.verifyAndRemember(sign(newer,key),now));require("corrupt-fixture".equals(replay.getString("sequence",null)));
            }finally{require(replay.edit().clear().commit());}
            stage = "real_release_apk";
            PackageInfo candidate = ReleaseArtifact.inspect(manager, file, sign(manifest,key), publicKey, ORIGIN, certificate, 1, "com.appcontrol.parent", now);
            stage = "download_staging";
            byte[] apkBytes = java.nio.file.Files.readAllBytes(file.toPath());
            byte[] signed = sign(manifest,key);
            File staging = app.getDir("release-stage", Context.MODE_PRIVATE);
            require(staging.list() != null && staging.list().length == 0);
            Connection good = new Connection(apkBytes,200);
            File downloaded = ReleaseDownload.stage(app,signed,publicKey,ORIGIN,certificate,1,()->false,url->good,()->now);
            require(hash(downloaded).equals(hash(file)) && downloaded.delete() && good.closed && !good.getInstanceFollowRedirects());
            require(good.getRequestProperty("Authorization") == null && "identity".equals(good.getRequestProperty("Accept-Encoding")));
            for (Connection bad : new Connection[]{new Connection(apkBytes,302),new Connection(java.util.Arrays.copyOf(apkBytes,apkBytes.length-1),200),new Connection(java.util.Arrays.copyOf(apkBytes,apkBytes.length+1),200)}) {
                rejected(() -> ReleaseDownload.stage(app,signed,publicKey,ORIGIN,certificate,1,()->false,url->bad,()->now));
                require(bad.closed && staging.list().length == 0);
            }
            byte[] altered = apkBytes.clone(); altered[0]^=1;
            Connection wrongHash = new Connection(altered,200);
            rejected(() -> ReleaseDownload.stage(app,signed,publicKey,ORIGIN,certificate,1,()->false,url->wrongHash,()->now));
            require(wrongHash.closed && staging.list().length == 0);
            final int[] opened={0};
            rejected(() -> ReleaseDownload.stage(app,signed,publicKey,ORIGIN,certificate,1,()->true,url->{opened[0]++;return good;},()->now));
            require(opened[0]==0 && staging.list().length==0);
            require(!Thread.currentThread().isInterrupted());
            try {
                Thread.currentThread().interrupt();
                rejected(() -> ReleaseDownload.stage(app,signed,publicKey,ORIGIN,certificate,1,()->false,url->{opened[0]++;return good;},()->now));
            } finally { Thread.interrupted(); }
            require(opened[0]==0 && staging.list().length==0);
            Connection lengthHeader=new Connection(apkBytes,200);lengthHeader.declaredLength=apkBytes.length+1;
            Connection encodedBody=new Connection(apkBytes,200);encodedBody.encoding="gzip";
            for (Connection bad:new Connection[]{lengthHeader,encodedBody}) {
                rejected(() -> ReleaseDownload.stage(app,signed,publicKey,ORIGIN,certificate,1,()->false,url->bad,()->now));
                require(bad.closed && staging.list().length==0);
            }
            final int[] clockCalls={0}; Connection late=new Connection(apkBytes,200);
            rejected(() -> ReleaseDownload.stage(app,signed,publicKey,ORIGIN,certificate,1,()->false,url->late,()->clockCalls[0]++==0?now:now.plusSeconds(3600)));
            require(late.closed && staging.list().length==0);
            stage = "symlink_rejection";
            File link=File.createTempFile("fixture-release-link-",".apk",app.getCacheDir());
            require(link.delete());
            try {
                android.system.Os.symlink(file.getAbsolutePath(),link.getAbsolutePath());
                rejected(() -> ReleaseArtifact.inspect(manager,link,signed,publicKey,ORIGIN,certificate,1,"com.appcontrol.parent",now));
            } finally { require(link.delete()); }
            stage = "signed_metadata_mismatch";
            for (String field : new String[]{"bytes", "sha256", "versionCode", "versionName", "minSdk", "targetSdk"}) {
                JSONObject changed = new JSONObject(manifest.toString()), artifact = changed.getJSONArray("apps").getJSONObject(0);
                if (field.equals("sha256")) artifact.put(field, "0".repeat(64));
                else if (field.equals("versionName")) artifact.put(field, "unexpected-fixture-version");
                else artifact.put(field, artifact.getLong(field)+1);
                rejected(() -> ReleaseArtifact.inspect(manager, file, sign(changed,key), publicKey, ORIGIN, certificate, 1, "com.appcontrol.parent", now));
            }
            rejected(() -> ReleaseArtifact.inspect(manager, file, sign(manifest,key), publicKey, ORIGIN, certificate, 1, "com.appcontrol.child", now));
            String otherCertificate = certificate.equals("0".repeat(64)) ? "1".repeat(64) : "0".repeat(64);
            JSONObject changedSigner = new JSONObject(manifest.toString());
            for (int i=0;i<2;i++) changedSigner.getJSONArray("apps").getJSONObject(i).put("signingCertificateSHA256",otherCertificate);
            rejected(() -> ReleaseArtifact.inspect(manager, file, sign(changedSigner,key), publicKey, ORIGIN, otherCertificate, 1, "com.appcontrol.parent", now));
            stage = "upgrade_guard";
            PackageInfo baseline = new PackageInfo(); baseline.packageName=candidate.packageName; baseline.setLongVersionCode(candidate.getLongVersionCode()-1); baseline.signingInfo=candidate.signingInfo;
            ReleaseArtifact.requireUpgrade(baseline,candidate);
            baseline.setLongVersionCode(candidate.getLongVersionCode()); rejected(() -> ReleaseArtifact.requireUpgrade(baseline,candidate));
            baseline.setLongVersionCode(candidate.getLongVersionCode()+1); rejected(() -> ReleaseArtifact.requireUpgrade(baseline,candidate));
            baseline.setLongVersionCode(0); baseline.packageName="com.appcontrol.child"; rejected(() -> ReleaseArtifact.requireUpgrade(baseline,candidate));
            baseline.packageName=candidate.packageName;
            baseline.signingInfo=manager.getPackageInfo(app.getPackageName(),PackageManager.PackageInfoFlags.of(PackageManager.GET_SIGNING_CERTIFICATES)).signingInfo;
            rejected(() -> ReleaseArtifact.requireUpgrade(baseline,candidate));
            stage = "debug_rejection";
            File debug = new File(app.getApplicationInfo().sourceDir);
            PackageInfo installed = manager.getPackageInfo(app.getPackageName(),PackageManager.PackageInfoFlags.of(PackageManager.GET_SIGNING_CERTIFICATES));
            String debugCertificate=hex(MessageDigest.getInstance("SHA-256").digest(installed.signingInfo.getApkContentsSigners()[0].toByteArray()));
            JSONObject debugManifest=metadata(debug,installed,debugCertificate,now);
            rejected(() -> ReleaseArtifact.inspect(manager,debug,sign(debugManifest,key),publicKey,ORIGIN,debugCertificate,1,"com.appcontrol.parent",now));
            stage = "byte_tamper";
            try (java.io.RandomAccessFile bytes=new java.io.RandomAccessFile(file,"rw")) { bytes.seek(0); int first=bytes.read(); bytes.seek(0); bytes.write(first^1); }
            rejected(() -> ReleaseArtifact.inspect(manager,file,sign(manifest,key),publicKey,ORIGIN,certificate,1,"com.appcontrol.parent",now));
            result.putString("release_artifact_probe", "passed");
            result.putString("release_artifact_version", "staging-replay-v1");
            result.putString("stream", "Real disposable-signed APK inspection and controlled HTTPS-object staging passed; redirect/size/hash/expiry/cancellation cleanup, metadata/signer/debug rejection and synthetic upgrade guards passed; no live TLS or installation inferred\n");
        } catch (Exception failure) {
            result.putString("release_artifact_probe", "failed"); result.putString("observation",stage);
            String message = failure.getMessage();
            if (message != null && message.matches("Invalid release artifact: (metadata|file|bytes|archive|package|sdk|production|signer)")) result.putString("artifact_check", message);
        }
        finally { if (created && !file.delete()) { result.putString("release_artifact_probe","failed");result.putString("observation","cleanup"); } }
        return result;
    }
    private static final class Connection extends javax.net.ssl.HttpsURLConnection {
        private final byte[] bytes; private final int status; boolean closed; long declaredLength=-1;String encoding;
        Connection(byte[] bytes,int status) throws Exception { super(new java.net.URL(ORIGIN+"/releases/1/parent-release.apk"));this.bytes=bytes;this.status=status; }
        @Override public int getResponseCode(){return status;}
        @Override public long getContentLengthLong(){return declaredLength;}
        @Override public String getHeaderField(String name){return "Content-Encoding".equals(name)?encoding:null;}
        @Override public java.io.InputStream getInputStream(){return new java.io.ByteArrayInputStream(bytes);}
        @Override public void disconnect(){closed=true;}
        @Override public void connect(){}
        @Override public boolean usingProxy(){return false;}
        @Override public String getCipherSuite(){return "fixture";}
        @Override public java.security.cert.Certificate[] getLocalCertificates(){return null;}
        @Override public java.security.cert.Certificate[] getServerCertificates(){return null;}
    }
}
