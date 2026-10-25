package com.appcontrol.child;

import android.app.admin.DevicePolicyManager;
import android.content.ComponentName;
import android.content.Context;
import android.content.SharedPreferences;
import android.content.pm.ApplicationInfo;
import android.os.Build;
import android.os.Bundle;
import com.appcontrol.child.admin.AppControlDeviceAdminReceiver;
import com.appcontrol.child.policy.PolicyEnforcer;
import java.util.Map;
import java.util.Set;
import org.json.JSONArray;
import org.json.JSONObject;

/** Production policy adapter test, restricted to the isolated, unenrolled AVD. */
public final class PackagePolicyProbe {
    static final String TARGET = "org.example.appcontrolfixture";
    private static void require(boolean condition) { if (!condition) throw new IllegalStateException("Policy invariant failed"); }
    static void save(SharedPreferences cache, long version, String blocked, String allowed) throws Exception {
        JSONObject desired = new JSONObject().put("formatVersion", 2).put("version", version).put("internetPaused", false)
            .put("blockedPackages", blocked == null ? new JSONArray() : new JSONArray().put(blocked))
            .put("alwaysAllowedPackages", allowed == null ? new JSONArray() : new JSONArray().put(allowed))
            .put("schedules", new JSONArray());
        require(cache.edit().putLong("version", version).putString("desired", desired.toString()).commit());
    }
    @SuppressWarnings("unchecked")
    static void restore(SharedPreferences preferences, Map<String, ?> original) {
        SharedPreferences.Editor editor = preferences.edit().clear();
        for (Map.Entry<String, ?> entry : original.entrySet()) {
            String key = entry.getKey(); Object value = entry.getValue();
            if (value instanceof String) editor.putString(key, (String) value);
            else if (value instanceof Long) editor.putLong(key, (Long) value);
            else if (value instanceof Integer) editor.putInt(key, (Integer) value);
            else if (value instanceof Boolean) editor.putBoolean(key, (Boolean) value);
            else if (value instanceof Float) editor.putFloat(key, (Float) value);
            else if (value instanceof Set) editor.putStringSet(key, (Set<String>) value);
            else throw new IllegalStateException("Unexpected preference type");
        }
        require(editor.commit());
    }
    static void applied(PolicyEnforcer.Result result) {
        require(result != null && result.getResult().equals("applied") && result.getReason().isEmpty());
    }
    public static Bundle run(Context context) {
        Bundle result = new Bundle();
        SharedPreferences cache = context.getSharedPreferences("policy-cache", Context.MODE_PRIVATE);
        SharedPreferences ledger = context.getSharedPreferences("package-reconciliation", Context.MODE_PRIVATE);
        Map<String, ?> oldCache = cache.getAll(), oldLedger = ledger.getAll();
        DevicePolicyManager manager = context.getSystemService(DevicePolicyManager.class);
        ComponentName admin = new ComponentName(context, AppControlDeviceAdminReceiver.class);
        PolicyEnforcer enforcer = new PolicyEnforcer(context);
        boolean touched = false;
        String stage = "isolated_fixture_guard";
        try {
            require(Build.FINGERPRINT.startsWith("google/sdk_gphone64_x86_64/emu64xa:"));
            require(context.getSharedPreferences("enrollment", Context.MODE_PRIVATE).getString("deviceId", null) == null);
            require(cache.getString("desired", null) == null && ledger.getStringSet("tracked", java.util.Collections.emptySet()).isEmpty());
            require(manager.isDeviceOwnerApp(context.getPackageName()));
            require((context.getPackageManager().getApplicationInfo(TARGET, 0).flags & ApplicationInfo.FLAG_SYSTEM) == 0);
            require(!enforcer.isProtected(TARGET) && enforcer.isProtected(context.getPackageName()));
            require(!manager.isPackageSuspended(admin, TARGET));
            touched = true;

            stage = "suspension";
            save(cache, 1, TARGET, null);
            applied(enforcer.applyCached());
            require(manager.isPackageSuspended(admin, TARGET));
            require(context.getPackageManager().isPackageSuspended(TARGET));
            require(ledger.getStringSet("tracked", java.util.Collections.emptySet()).contains(TARGET));

            stage = "unchanged_policy";
            applied(enforcer.applyCached());
            require(manager.isPackageSuspended(admin, TARGET));

            stage = "allowlist_restore";
            save(cache, 2, TARGET, TARGET);
            applied(enforcer.applyCached());
            require(!manager.isPackageSuspended(admin, TARGET) && ledger.getStringSet("tracked", java.util.Collections.emptySet()).isEmpty());

            stage = "management_exclusion";
            save(cache, 3, context.getPackageName(), null);
            boolean rejected = false;
            try { enforcer.applyCached(); } catch (IllegalArgumentException expected) { rejected = true; }
            require(rejected && ledger.getStringSet("tracked", java.util.Collections.emptySet()).isEmpty());
            require(!manager.isPackageSuspended(admin, context.getPackageName()) && !manager.isPackageSuspended(admin, TARGET));
            result.putString("package_probe", "passed");
            result.putString("stream", "Android package policy probe: suspension, allowlist restoration and management exclusion passed\n");
        } catch (Exception failure) {
            result.putString("package_probe", "failed");
            result.putString("observation", stage);
            result.putString("stream", "Android package policy probe failed at " + stage + "\n");
        } finally {
            if (touched) {
                try {
                    save(cache, 4, null, null);
                    applied(enforcer.applyCached());
                    require(!manager.isPackageSuspended(admin, TARGET));
                    restore(ledger, oldLedger);
                    restore(cache, oldCache);
                } catch (Exception failure) {
                    result.putString("package_probe", "failed");
                    result.putString("observation", "cleanup");
                    result.putString("stream", "Android package policy probe cleanup failed; reconciliation state retained\n");
                }
            }
        }
        return result;
    }
}
