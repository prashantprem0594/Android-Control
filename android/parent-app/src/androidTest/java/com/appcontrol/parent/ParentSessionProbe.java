package com.appcontrol.parent;

import android.content.Context;
import android.content.SharedPreferences;
import android.os.Bundle;
import java.security.KeyStore;
import javax.crypto.SecretKey;
import org.json.JSONObject;

/** Isolated vault fixture: no production session or server is used. */
public final class ParentSessionProbe {
    private static void require(boolean value) { if (!value) throw new IllegalStateException("Session invariant failed"); }
    public static Bundle run(Context context) {
        Bundle result = new Bundle();
        String name = "fixture-parent-session";
        SharedPreferences state = context.getSharedPreferences(name, Context.MODE_PRIVATE);
        SessionVault vault = new SessionVault(context, name);
        boolean touched = false;
        String stage = "fixture_guard";
        try {
            require(state.getAll().isEmpty());
            touched = true;
            stage = "encrypted_roundtrip";
            String origin = "https://vault.example.org";
            JSONObject fixture = new JSONObject().put("accessToken", "fixture-access-value").put("refreshToken", "fixture-refresh-value");
            vault.save(origin, fixture);
            require(vault.load(origin).getString("refreshToken").equals("fixture-refresh-value"));
            require(!state.getAll().toString().contains("fixture-refresh-value") && !state.getAll().toString().contains("fixture-access-value"));
            String firstIV = state.getString("iv", "");
            String firstCiphertext = state.getString("ciphertext", "");
            vault.save(origin, fixture);
            require(!firstIV.equals(state.getString("iv", "")) && !firstCiphertext.equals(state.getString("ciphertext", "")));
            stage = "nonexportable_key";
            KeyStore store = KeyStore.getInstance("AndroidKeyStore"); store.load(null);
            SecretKey key = (SecretKey) store.getKey("app-control."+name, null);
            require(key != null && key.getEncoded() == null);
            stage = "origin_binding";
            String other = "https://other.example.org";
            require(state.edit().putString("origin", other).commit());
            boolean rejected = false;
            try { vault.load(other); } catch (Exception expected) { rejected = true; }
            require(rejected);
            stage = "tamper_rejection";
            vault.save(origin, fixture);
            String ciphertext = state.getString("ciphertext", "");
            byte[] bytes = android.util.Base64.decode(ciphertext, android.util.Base64.NO_WRAP); bytes[0] ^= 1;
            require(state.edit().putString("ciphertext", android.util.Base64.encodeToString(bytes, android.util.Base64.NO_WRAP)).commit());
            rejected = false;
            try { vault.load(origin); } catch (Exception expected) { rejected = true; }
            require(rejected);
            stage = "clear";
            vault.clear(); require(vault.origin() == null && state.getAll().isEmpty());
            result.putString("parent_session_probe", "passed");
            result.putString("stream", "Parent vault encryption, nonce uniqueness, nonexportability, origin binding, tamper rejection and clearing passed\n");
        } catch (Exception failure) {
            result.putString("parent_session_probe", "failed"); result.putString("observation", stage);
        } finally {
            if (touched) {
                try {
                    vault.clear();
                    KeyStore store = KeyStore.getInstance("AndroidKeyStore"); store.load(null); store.deleteEntry("app-control."+name);
                } catch (Exception failure) { result.putString("parent_session_probe", "failed"); result.putString("observation", "cleanup"); }
            }
        }
        return result;
    }
}
