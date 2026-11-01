package com.appcontrol.child;

import android.content.Context;
import android.content.ContextWrapper;
import android.content.SharedPreferences;
import android.os.Build;
import android.os.Bundle;
import android.os.Process;
import android.provider.Settings;
import com.appcontrol.child.calls.CallContactStore;
import java.nio.charset.StandardCharsets;
import java.security.KeyStore;
import java.security.MessageDigest;
import org.json.JSONArray;
import org.json.JSONObject;

/** Phases span real process termination/reboot, using test-only preference names. */
public final class CallContactPersistenceProbe {
    private static final String PREFIX = "fixture-contact-persistence-";
    private static final String ALIAS = "appcontrol.callcontacts.v1";
    private static void require(boolean value) { if (!value) throw new IllegalStateException(); }
    private static JSONObject contacts(long version, String label) throws Exception {
        return new JSONObject().put("version", version).put("contacts", new JSONArray().put(
            new JSONObject().put("label", label).put("number", "+12025550123").put("isParent", true)));
    }
    private static String digest(String value) throws Exception {
        byte[] hash = MessageDigest.getInstance("SHA-256").digest(value.getBytes(StandardCharsets.UTF_8));
        StringBuilder text = new StringBuilder();
        for (byte item : hash) text.append(String.format(java.util.Locale.ROOT, "%02x", item & 255));
        return text.toString();
    }
    private static String ciphertext(SharedPreferences cache) {
        require(cache.getAll().size() == 1);
        return (String) cache.getAll().values().iterator().next();
    }
    private static void cleanup(Context context, SharedPreferences metadata, KeyStore keys) throws Exception {
        require(metadata.getBoolean("pending", false));
        boolean removeKey = metadata.getBoolean("createdKey", false);
        require(context.deleteSharedPreferences(PREFIX+"enrollment"));
        require(context.deleteSharedPreferences(PREFIX+"call-contact-cache"));
        if (removeKey) { keys.deleteEntry(ALIAS); require(!keys.containsAlias(ALIAS)); }
        require(context.deleteSharedPreferences(PREFIX+"proof"));
    }
    public static Bundle run(Context context, String phase) {
        Bundle result = new Bundle();
        String stage = "guard";
        SharedPreferences metadata = context.getSharedPreferences(PREFIX+"proof", Context.MODE_PRIVATE);
        KeyStore keys = null;
        boolean preparing = false;
        try {
            require(Build.FINGERPRINT.startsWith("google/sdk_gphone64_x86_64/emu64xa:"));
            require(BuildConfig.DEBUG && "com.appcontrol.child".equals(context.getPackageName()));
            require(context.getSharedPreferences("enrollment", Context.MODE_PRIVATE).getAll().isEmpty());
            require(context.getSharedPreferences("call-contact-cache", Context.MODE_PRIVATE).getAll().isEmpty());
            Context isolated = new ContextWrapper(context) {
                @Override public Context getApplicationContext() { return this; }
                @Override public SharedPreferences getSharedPreferences(String name, int mode) {
                    return context.getSharedPreferences(PREFIX+name, mode);
                }
            };
            SharedPreferences enrollment = isolated.getSharedPreferences("enrollment", Context.MODE_PRIVATE);
            SharedPreferences cache = isolated.getSharedPreferences("call-contact-cache", Context.MODE_PRIVATE);
            int boot = Settings.Global.getInt(context.getContentResolver(), Settings.Global.BOOT_COUNT, -1);
            require(boot >= 0);
            keys = KeyStore.getInstance("AndroidKeyStore"); keys.load(null);
            CallContactStore store = new CallContactStore(isolated);
            if ("prepare".equals(phase)) {
                require(metadata.getAll().isEmpty() && enrollment.getAll().isEmpty() && cache.getAll().isEmpty());
                stage = "prepare";
                require(metadata.edit().putBoolean("pending", true).putBoolean("createdKey", !keys.containsAlias(ALIAS))
                    .putInt("pid", Process.myPid()).putInt("boot", boot).commit());
                preparing = true;
                String origin = BuildConfig.SERVER_ORIGIN.isEmpty() ? "https://contact-persistence-fixture.invalid" : BuildConfig.SERVER_ORIGIN;
                require(enrollment.edit().putString("server", origin).putString("deviceId", "dev_AAAAAAAAAAAAAAAAAAAAAA").commit());
                store.save(contacts(11, "Persistence fixture"));
                require(metadata.edit().putString("sealedSHA256", digest(ciphertext(cache))).commit());
                require(keys.getKey(ALIAS, null).getEncoded() == null);
                result.putString("contact_persistence_probe", "prepared");
            } else if ("restart".equals(phase) || "reboot".equals(phase)) {
                stage = "lifecycle";
                require(metadata.getBoolean("pending", false));
                if ("restart".equals(phase)) {
                    require(boot == metadata.getInt("boot", -1) && Process.myPid() != metadata.getInt("pid", -1));
                } else require(boot > metadata.getInt("boot", Integer.MAX_VALUE) && metadata.getBoolean("restartVerified", false));
                stage = "retained_ciphertext_key_and_floor";
                require(digest(ciphertext(cache)).equals(metadata.getString("sealedSHA256", null)));
                require(store.load().getVersion() == 11 && store.load().getContacts().size() == 1);
                require(store.load().getContacts().get(0).getLabel().equals("Persistence fixture"));
                require(store.load().getContacts().get(0).getNumber().equals("+12025550123"));
                require(keys.getKey(ALIAS, null).getEncoded() == null);
                boolean rollback = false, conflict = false;
                try { store.save(contacts(10, "Persistence fixture")); } catch (IllegalArgumentException expected) { rollback=true; }
                try { store.save(contacts(11, "Changed fixture")); } catch (IllegalArgumentException expected) { conflict=true; }
                require(rollback && conflict && digest(ciphertext(cache)).equals(metadata.getString("sealedSHA256", null)));
                if ("restart".equals(phase)) require(metadata.edit().putBoolean("restartVerified", true).commit());
                result.putString("contact_persistence_probe", "passed");
            } else if ("cleanup".equals(phase)) {
                stage = "cleanup"; cleanup(context, metadata, keys);
                result.putString("contact_persistence_probe", "cleaned");
            } else throw new IllegalArgumentException();
        } catch (Exception ignored) {
            result.putString("contact_persistence_probe", "failed");
            result.putString("contact_persistence_observation", stage);
            if (preparing && keys != null) try { cleanup(context, metadata, keys); }
            catch (Exception cleanupFailure) { result.putString("contact_persistence_cleanup", "failed"); }
        }
        result.putString("contact_persistence_version", "cache-persistence-v1");
        return result;
    }
}
