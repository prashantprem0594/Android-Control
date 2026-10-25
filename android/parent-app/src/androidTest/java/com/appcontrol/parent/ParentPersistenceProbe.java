package com.appcontrol.parent;

import android.content.Context;
import android.content.SharedPreferences;
import android.os.Build;
import android.os.Bundle;
import android.os.Process;
import android.provider.Settings;
import java.security.KeyStore;
import org.json.JSONObject;

/** Synthetic vault only; phases span real process termination and device reboot. */
public final class ParentPersistenceProbe {
    private static final String NAME = "fixture-parent-persistence";
    private static final String ORIGIN = "https://vault.example.org";
    private static void require(boolean value) { if (!value) throw new IllegalStateException("Persistence invariant failed"); }
    public static Bundle run(Context context, String phase) {
        Bundle result = new Bundle();
        SharedPreferences metadata = context.getSharedPreferences(NAME + "-proof", Context.MODE_PRIVATE);
        SharedPreferences ciphertext = context.getSharedPreferences(NAME, Context.MODE_PRIVATE);
        SessionVault vault = new SessionVault(context, NAME);
        String stage = "guard";
        try {
            require(Build.FINGERPRINT.startsWith("google/sdk_gphone64_x86_64/emu64xa:"));
            int boot = Settings.Global.getInt(context.getContentResolver(), Settings.Global.BOOT_COUNT, -1);
            require(boot >= 0);
            KeyStore keys = KeyStore.getInstance("AndroidKeyStore"); keys.load(null);
            if ("prepare".equals(phase)) {
                require(metadata.getAll().isEmpty() && ciphertext.getAll().isEmpty() && !keys.containsAlias("app-control." + NAME));
                stage = "save";
                vault.save(ORIGIN, new JSONObject().put("accessToken", "synthetic-access-persistence")
                    .put("refreshToken", "synthetic-refresh-persistence"));
                require(metadata.edit().putInt("pid", Process.myPid()).putInt("boot", boot)
                    .putString("iv", ciphertext.getString("iv", ""))
                    .putString("ciphertext", ciphertext.getString("ciphertext", "")).putBoolean("pending", true).commit());
                result.putString("parent_persistence_probe", "prepared");
            } else if ("restart".equals(phase) || "reboot".equals(phase)) {
                stage = "lifecycle";
                require(metadata.getBoolean("pending", false));
                if ("restart".equals(phase)) require(boot == metadata.getInt("boot", -1) && Process.myPid() != metadata.getInt("pid", -1));
                else require(boot > metadata.getInt("boot", Integer.MAX_VALUE) && metadata.getBoolean("restartVerified", false));
                stage = "retained_ciphertext_and_key";
                require(metadata.getString("iv", "").equals(ciphertext.getString("iv", null)));
                require(metadata.getString("ciphertext", "").equals(ciphertext.getString("ciphertext", null)));
                require(ORIGIN.equals(vault.origin()));
                JSONObject tokens = vault.load(ORIGIN);
                require("synthetic-access-persistence".equals(tokens.getString("accessToken")));
                require("synthetic-refresh-persistence".equals(tokens.getString("refreshToken")));
                require(keys.getKey("app-control." + NAME, null).getEncoded() == null);
                if ("restart".equals(phase)) require(metadata.edit().putBoolean("restartVerified", true).commit());
                result.putString("parent_persistence_probe", "passed");
            } else if ("cleanup".equals(phase)) {
                stage = "cleanup";
                vault.clear(); keys.deleteEntry("app-control." + NAME);
                require(metadata.edit().clear().commit());
                require(ciphertext.getAll().isEmpty() && !keys.containsAlias("app-control." + NAME));
                result.putString("parent_persistence_probe", "cleaned");
            } else throw new IllegalArgumentException();
            result.putString("stream", "Synthetic parent vault persistence phase " + phase + " completed\n");
        } catch (Exception failure) {
            result.putString("parent_persistence_probe", "failed");
            result.putString("observation", stage);
        }
        return result;
    }
}
