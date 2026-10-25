package com.appcontrol.child;

import android.app.admin.DevicePolicyManager;
import android.content.ComponentName;
import android.content.Context;
import android.content.SharedPreferences;
import android.os.Build;
import android.os.Bundle;
import android.provider.Settings;
import com.appcontrol.child.admin.AppControlDeviceAdminReceiver;
import com.appcontrol.child.policy.PolicyEnforcer;
import java.util.Collections;
import java.util.HashMap;
import java.util.Map;
import java.util.Set;
import org.json.JSONObject;

/** Multi-invocation reboot exercise on the isolated, unenrolled Device Owner AVD. */
public final class PolicyRebootProbe {
    private static void require(boolean value) { if (!value) throw new IllegalStateException("Reboot policy invariant failed"); }
    private static void fixturePolicy(SharedPreferences cache) throws Exception {
        String raw = cache.getString("desired", null);
        if (raw == null) return;
        JSONObject policy = new JSONObject(raw);
        require(policy.getLong("version") >= 101 && policy.getLong("version") <= 103 && !policy.getBoolean("internetPaused"));
        require(policy.getJSONArray("schedules").length() == 0);
        for (String field : new String[]{"blockedPackages", "alwaysAllowedPackages"}) {
            for (int index = 0; index < policy.getJSONArray(field).length(); index++) {
                require(policy.getJSONArray(field).getString(index).equals(PackagePolicyProbe.TARGET));
            }
        }
    }
    private static void cleanup(Context context, SharedPreferences cache, SharedPreferences ledger, SharedPreferences fixture,
                                DevicePolicyManager manager, ComponentName admin) throws Exception {
        fixturePolicy(cache);
        require(ledger.getStringSet("tracked", Collections.emptySet()).stream().allMatch(PackagePolicyProbe.TARGET::equals));
        PackagePolicyProbe.save(cache, 103, null, null);
        PackagePolicyProbe.applied(new PolicyEnforcer(context).applyCached());
        require(!manager.isPackageSuspended(admin, PackagePolicyProbe.TARGET));
        Map<String, Object> baselineCache = new HashMap<>(), baselineLedger = new HashMap<>();
        if (fixture.getBoolean("hadAlarmMode", false)) baselineCache.put("alarmMode", fixture.getString("alarmMode", "none"));
        if (fixture.getBoolean("hadTracked", false)) baselineLedger.put("tracked", Collections.emptySet());
        PackagePolicyProbe.restore(ledger, baselineLedger);
        PackagePolicyProbe.restore(cache, baselineCache);
        require(fixture.edit().clear().commit());
    }
    public static Bundle run(Context context, String phase) {
        Bundle result = new Bundle();
        SharedPreferences cache = context.getSharedPreferences("policy-cache", Context.MODE_PRIVATE);
        SharedPreferences ledger = context.getSharedPreferences("package-reconciliation", Context.MODE_PRIVATE);
        SharedPreferences fixture = context.getSharedPreferences("fixture-policy-reboot", Context.MODE_PRIVATE);
        DevicePolicyManager manager = context.getSystemService(DevicePolicyManager.class);
        ComponentName admin = new ComponentName(context, AppControlDeviceAdminReceiver.class);
        PolicyEnforcer enforcer = new PolicyEnforcer(context);
        boolean cleanupOnFailure = false;
        String stage = "fixture_guard";
        try {
            require(Build.FINGERPRINT.startsWith("google/sdk_gphone64_x86_64/emu64xa:"));
            require(context.getSharedPreferences("enrollment", Context.MODE_PRIVATE).getString("deviceId", null) == null);
            require(manager.isDeviceOwnerApp(context.getPackageName()));
            int boot = Settings.Global.getInt(context.getContentResolver(), Settings.Global.BOOT_COUNT, -1);
            require(boot >= 0);
            if (phase.equals("prepare")) {
                require(fixture.getAll().isEmpty() && cache.getString("desired", null) == null);
                require(cache.getAll().keySet().stream().allMatch("alarmMode"::equals));
                require(ledger.getAll().keySet().stream().allMatch("tracked"::equals));
                require(ledger.getStringSet("tracked", Collections.emptySet()).isEmpty());
                require(!manager.isPackageSuspended(admin, PackagePolicyProbe.TARGET));
                require(fixture.edit().putBoolean("pending", true).putInt("bootCount", boot)
                    .putBoolean("hadAlarmMode", cache.contains("alarmMode")).putString("alarmMode", cache.getString("alarmMode", "none"))
                    .putBoolean("hadTracked", ledger.contains("tracked")).commit());
                cleanupOnFailure = true;
                stage = "prepare_cached_policy";
                PackagePolicyProbe.save(cache, 101, PackagePolicyProbe.TARGET, null);
                PackagePolicyProbe.applied(enforcer.applyCached());
                require(manager.isPackageSuspended(admin, PackagePolicyProbe.TARGET) && cache.getInt("localBootCount", -1) == boot);
                require(manager.setPackagesSuspended(admin, new String[]{PackagePolicyProbe.TARGET}, false).length == 0);
                require(!manager.isPackageSuspended(admin, PackagePolicyProbe.TARGET));
                result.putString("policy_reboot_probe", "prepared");
                result.putString("stream", "Cached blocking policy prepared; disposable app is temporarily unsuspended for boot reconciliation\n");
            } else if (phase.equals("verify")) {
                require(fixture.getBoolean("pending", false) && boot > fixture.getInt("bootCount", Integer.MAX_VALUE));
                fixturePolicy(cache);
                cleanupOnFailure = true;
                stage = "new_boot_reconciliation";
                require(cache.getLong("version", -1) == 101 && cache.getLong("lastAttemptedVersion", -1) == 101);
                require(cache.getInt("localBootCount", -1) == boot && "applied".equals(cache.getString("localResult", "")));
                require(manager.isPackageSuspended(admin, PackagePolicyProbe.TARGET));
                require(ledger.getStringSet("tracked", Collections.emptySet()).contains(PackagePolicyProbe.TARGET));
                stage = "post_reboot_restore";
                PackagePolicyProbe.save(cache, 102, PackagePolicyProbe.TARGET, PackagePolicyProbe.TARGET);
                PackagePolicyProbe.applied(enforcer.applyCached());
                require(!manager.isPackageSuspended(admin, PackagePolicyProbe.TARGET));
                cleanup(context, cache, ledger, fixture, manager, admin);
                cleanupOnFailure = false;
                result.putString("policy_reboot_probe", "passed");
                result.putString("stream", "Android cached policy reboot reconciliation, allowlist restoration and cleanup passed\n");
            } else if (phase.equals("cleanup")) {
                require(fixture.getBoolean("pending", false));
                cleanup(context, cache, ledger, fixture, manager, admin);
                result.putString("policy_reboot_probe", "cleaned");
                result.putString("stream", "Android reboot policy fixture cleaned\n");
            } else throw new IllegalArgumentException();
        } catch (Exception failure) {
            result.putString("policy_reboot_probe", "failed");
            result.putString("observation", stage);
            result.putString("stream", "Android policy reboot probe failed at " + stage + "\n");
            if (cleanupOnFailure) {
                try { cleanup(context, cache, ledger, fixture, manager, admin); }
                catch (Exception cleanupFailure) { result.putString("cleanup", "failed_reconciliation_state_retained"); }
            }
        }
        return result;
    }
}
