package com.appcontrol.transport;

import android.content.Context;
import android.content.SharedPreferences;
import android.content.pm.ApplicationInfo;
import android.os.Build;
import android.os.Bundle;
import android.os.Process;
import android.provider.Settings;
import java.nio.charset.StandardCharsets;
import java.security.KeyPair;
import java.security.KeyPairGenerator;
import java.security.MessageDigest;
import java.security.Signature;
import java.time.Instant;
import java.util.Base64;
import org.json.JSONArray;
import org.json.JSONObject;

/** Real process/reboot replay-floor proof; public fixtures and isolated state only. */
public final class ReleasePersistenceProbe {
    private static final String STATE = "fixture-release-persistence";
    private static final String PROOF = STATE+"-proof";
    private static final String ORIGIN = "https://replay-persistence-fixture.invalid";
    private static final String CERTIFICATE = "a".repeat(64);
    private static void require(boolean value) { if (!value) throw new IllegalStateException(); }
    private interface Checked { void run() throws Exception; }
    private static void refused(Checked action) throws Exception {
        boolean rejected = false;
        try { action.run(); } catch (IllegalArgumentException expected) { rejected=true; }
        require(rejected);
    }
    private static JSONObject manifest(int sequence, Instant now, boolean changed) throws Exception {
        JSONArray apps = new JSONArray();
        for (String name : new String[]{"parent","child"}) apps.put(new JSONObject()
            .put("package", "com.appcontrol."+name).put("versionCode", 4)
            .put("versionName", changed ? "changed-fixture" : "replay-fixture")
            .put("minSdk", 33).put("targetSdk", 37).put("bytes", 1024)
            .put("sha256", "b".repeat(64)).put("signingCertificateSHA256", CERTIFICATE)
            .put("url", ORIGIN+"/releases/"+sequence+"/"+name+"-release.apk"));
        return new JSONObject().put("version", 1).put("sequence", sequence).put("origin", ORIGIN)
            .put("issuedAt", now.minusSeconds(1).toString()).put("expiresAt", now.plusSeconds(7200).toString()).put("apps", apps);
    }
    private static String sign(JSONObject value, KeyPair key) throws Exception {
        byte[] payload = value.toString().getBytes(StandardCharsets.UTF_8);
        Signature signer = Signature.getInstance("Ed25519"); signer.initSign(key.getPrivate());
        signer.update("app-control-release-manifest-v1\n".getBytes(StandardCharsets.UTF_8)); signer.update(payload);
        return new JSONObject().put("version", 1).put("payload", Base64.getUrlEncoder().withoutPadding().encodeToString(payload))
            .put("signature", Base64.getUrlEncoder().withoutPadding().encodeToString(signer.sign())).toString();
    }
    private static String stateHash(SharedPreferences state) throws Exception {
        require(state.getAll().size() == 4);
        String value = state.getInt("version", 0)+"\n"+state.getString("binding", "")+"\n"+
            state.getLong("sequence", 0)+"\n"+state.getString("payloadSHA256", "");
        byte[] hash = MessageDigest.getInstance("SHA-256").digest(value.getBytes(StandardCharsets.UTF_8));
        StringBuilder text = new StringBuilder();
        for (byte item : hash) text.append(String.format(java.util.Locale.ROOT,"%02x",item & 255));
        return text.toString();
    }
    private static byte[] raw(SharedPreferences proof, String name) {
        return proof.getString(name, "").getBytes(StandardCharsets.UTF_8);
    }
    private static void cleanup(Context context, SharedPreferences proof) {
        require(proof.getBoolean("pending", false));
        require(context.deleteSharedPreferences(STATE));
        require(context.deleteSharedPreferences(PROOF));
    }
    public static Bundle run(Context context, String phase) {
        Bundle result = new Bundle(); String stage = "guard"; boolean preparing=false;
        SharedPreferences proof = context.getSharedPreferences(PROOF, Context.MODE_PRIVATE);
        SharedPreferences state = context.getSharedPreferences(STATE, Context.MODE_PRIVATE);
        try {
            require(Build.FINGERPRINT.startsWith("google/sdk_gphone64_x86_64/emu64xa:"));
            require("com.appcontrol.child".equals(context.getPackageName()));
            require((context.getApplicationInfo().flags & ApplicationInfo.FLAG_DEBUGGABLE) != 0);
            require(context.getSharedPreferences("enrollment", Context.MODE_PRIVATE).getAll().isEmpty());
            int boot = Settings.Global.getInt(context.getContentResolver(), Settings.Global.BOOT_COUNT, -1);
            require(boot >= 0);
            if ("prepare".equals(phase)) {
                require(proof.getAll().isEmpty() && state.getAll().isEmpty()); stage="prepare";
                KeyPair key = KeyPairGenerator.getInstance("Ed25519").generateKeyPair();
                String publicKey = Base64.getEncoder().encodeToString(key.getPublic().getEncoded());
                Instant now = Instant.now();
                require(proof.edit().putBoolean("pending", true).putInt("pid", Process.myPid()).putInt("boot", boot)
                    .putString("publicKey", publicKey).putString("older", sign(manifest(1,now,false),key))
                    .putString("accepted", sign(manifest(2,now,false),key)).putString("conflicting", sign(manifest(2,now,true),key)).commit());
                preparing=true;
                ReleaseState replay = new ReleaseState(context,publicKey,ORIGIN,CERTIFICATE,0,STATE);
                replay.verifyAndRemember(raw(proof,"accepted"),now);
                require(replay.minimumSequence()==2);
                require(proof.edit().putString("stateSHA256",stateHash(state)).commit());
                result.putString("release_persistence_probe","prepared");
            } else if ("restart".equals(phase) || "reboot".equals(phase)) {
                stage="lifecycle"; require(proof.getBoolean("pending",false));
                if ("restart".equals(phase)) require(boot==proof.getInt("boot",-1) && Process.myPid()!=proof.getInt("pid",-1));
                else require(boot>proof.getInt("boot",Integer.MAX_VALUE) && proof.getBoolean("restartVerified",false));
                stage="floor_replay_and_binding";
                String publicKey=proof.getString("publicKey",""); Instant now=Instant.now();
                ReleaseState replay=new ReleaseState(context,publicKey,ORIGIN,CERTIFICATE,0,STATE);
                require(replay.minimumSequence()==2 && stateHash(state).equals(proof.getString("stateSHA256",null)));
                replay.verifyAndRemember(raw(proof,"accepted"),now);
                refused(()->replay.verifyAndRemember(raw(proof,"older"),now));
                refused(()->replay.verifyAndRemember(raw(proof,"conflicting"),now));
                ReleaseState other=new ReleaseState(context,publicKey,"https://other-replay-fixture.invalid",CERTIFICATE,0,STATE);
                refused(()->other.verifyAndRemember(raw(proof,"accepted"),now));
                require(replay.minimumSequence()==2 && stateHash(state).equals(proof.getString("stateSHA256",null)));
                if ("restart".equals(phase)) require(proof.edit().putBoolean("restartVerified",true).commit());
                result.putString("release_persistence_probe","passed");
            } else if ("cleanup".equals(phase)) {
                stage="cleanup";cleanup(context,proof);result.putString("release_persistence_probe","cleaned");
            } else throw new IllegalArgumentException();
        } catch (Exception ignored) {
            result.putString("release_persistence_probe","failed");result.putString("release_persistence_observation",stage);
            if (preparing) try {cleanup(context,proof);} catch(Exception failure){result.putString("release_persistence_cleanup","failed");}
        }
        result.putString("release_persistence_version","replay-persistence-v1");
        return result;
    }
}
