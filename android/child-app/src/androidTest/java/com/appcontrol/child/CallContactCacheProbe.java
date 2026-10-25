package com.appcontrol.child;

import android.content.Context;
import android.content.ContextWrapper;
import android.content.SharedPreferences;
import android.os.Build;
import android.os.Bundle;
import com.appcontrol.child.calls.CallContactStore;
import java.security.KeyStore;
import java.util.Map;
import java.util.UUID;
import org.json.JSONArray;
import org.json.JSONObject;

/** Real Keystore/cache checks using isolated preferences on the unenrolled AVD. */
public final class CallContactCacheProbe {
    private static final String ALIAS = "appcontrol.callcontacts.v1";
    private static void require(boolean value) { if (!value) throw new IllegalStateException(); }
    private interface Checked { void run() throws Exception; }
    private static void refused(Checked action) throws Exception {
        boolean rejected = false;
        try { action.run(); } catch (Exception expected) { rejected = true; }
        require(rejected);
    }
    private static JSONObject snapshot(long version, String label) throws Exception {
        return new JSONObject().put("version", version).put("contacts", new JSONArray().put(
            new JSONObject().put("label", label).put("number", "+12025550123").put("isParent", true)));
    }
    public static Bundle run(Context context) {
        Bundle result = new Bundle();
        String prefix = "fixture-contacts-" + UUID.randomUUID() + "-";
        boolean removeKey = false, passed = false, cleanup = true;
        KeyStore keys = null;
        String phase = "guard";
        try {
            require(Build.FINGERPRINT.startsWith("google/sdk_gphone64_x86_64/emu64xa:"));
            require(BuildConfig.DEBUG && "com.appcontrol.child".equals(context.getPackageName()));
            require(context.getSharedPreferences("enrollment", Context.MODE_PRIVATE).getAll().isEmpty());
            Context isolated = new ContextWrapper(context) {
                @Override public Context getApplicationContext() { return this; }
                @Override public SharedPreferences getSharedPreferences(String name, int mode) {
                    return context.getSharedPreferences(prefix + name, mode);
                }
            };
            SharedPreferences enrollment = isolated.getSharedPreferences("enrollment", Context.MODE_PRIVATE);
            SharedPreferences cache = isolated.getSharedPreferences("call-contact-cache", Context.MODE_PRIVATE);
            String origin = BuildConfig.SERVER_ORIGIN.isEmpty() ? "https://contact-cache-fixture.invalid" : BuildConfig.SERVER_ORIGIN;
            require(enrollment.edit().putString("server", origin).putString("deviceId", "dev_AAAAAAAAAAAAAAAAAAAAAA").commit());
            keys = KeyStore.getInstance("AndroidKeyStore"); keys.load(null);
            removeKey = !keys.containsAlias(ALIAS);
            CallContactStore store = new CallContactStore(isolated);
            phase = "roundtrip";
            require(store.load().getVersion() == 0 && store.load().getContacts().isEmpty());
            store.save(snapshot(2, "Fixture parent"));
            require(new CallContactStore(isolated).load().getVersion() == 2);
            require(store.load().getContacts().get(0).getNumber().equals("+12025550123"));
            require(keys.getKey(ALIAS, null).getEncoded() == null);
            Map<String, ?> sealed = cache.getAll(); require(sealed.size() == 1);
            String slot = sealed.keySet().iterator().next(), raw = (String) sealed.get(slot);
            require(!raw.contains("Fixture parent") && !raw.contains("12025550123"));
            phase = "versions";
            store.save(snapshot(2, "Fixture parent"));
            refused(() -> store.save(snapshot(1, "Fixture parent")));
            refused(() -> store.save(snapshot(2, "Changed parent")));
            require(store.load().getContacts().get(0).getLabel().equals("Fixture parent"));
            phase = "binding";
            require(enrollment.edit().putString("deviceId", "dev_BBBBBBBBBBBBBBBBBBBBBB").commit());
            require(store.load().getVersion() == 0);
            store.save(snapshot(1, "Second fixture"));
            String other = cache.getAll().keySet().stream().filter(name -> !name.equals(slot)).findFirst().orElseThrow();
            require(cache.edit().putString(other, raw).commit());
            refused(() -> store.load());
            require(enrollment.edit().putString("deviceId", "dev_AAAAAAAAAAAAAAAAAAAAAA").commit());
            require(store.load().getVersion() == 2);
            phase = "tamper";
            JSONObject damaged = new JSONObject(raw);
            byte[] bytes = android.util.Base64.decode(damaged.getString("ciphertext"), android.util.Base64.NO_WRAP);
            bytes[0] ^= 1;
            damaged.put("ciphertext", android.util.Base64.encodeToString(bytes, android.util.Base64.NO_WRAP));
            require(cache.edit().putString(slot, damaged.toString()).commit());
            refused(() -> store.load());
            refused(() -> store.save(snapshot(3, "Fixture parent")));
            require(cache.edit().putString(slot, raw).commit());
            store.save(new JSONObject().put("version", 3).put("contacts", new JSONArray()));
            require(store.load().getVersion() == 3 && store.load().getContacts().isEmpty());
            passed = true;
        } catch (Exception ignored) {
            // Only fixed phase/result labels are returned; no contact or key material.
        } finally {
            for (String name : new String[]{"enrollment", "call-contact-cache"}) {
                if (!context.deleteSharedPreferences(prefix + name)) cleanup = false;
            }
            if (removeKey && keys != null) try { keys.deleteEntry(ALIAS); } catch (Exception ignored) { cleanup = false; }
        }
        result.putString("contact_cache_probe", passed && cleanup ? "passed" : "failed");
        result.putString("contact_cache_version", "sealed-cache-v1");
        result.putString("contact_cache_phase", passed ? "complete" : phase);
        result.putString("contact_cache_cleanup", cleanup ? "passed" : "failed");
        return result;
    }
}
