package com.appcontrol.child;

import android.app.admin.DevicePolicyManager;
import android.content.ComponentName;
import android.content.Context;
import android.content.SharedPreferences;
import android.os.Build;
import android.os.Bundle;
import android.provider.Settings;
import com.appcontrol.child.admin.AppControlDeviceAdminReceiver;
import com.appcontrol.child.enrollment.DeviceIdentity;
import com.appcontrol.child.policy.PolicyEnforcer;
import java.util.Collections;
import java.util.HashMap;
import java.util.Map;
import org.json.JSONObject;

/** Same-signing-key package replacement on the isolated unenrolled test AVD. */
public final class PolicyUpgradeProbe {
    private static void require(boolean value) { if (!value) throw new IllegalStateException("Upgrade invariant failed"); }
    private static void cleanup(Context context, SharedPreferences cache, SharedPreferences ledger, SharedPreferences fixture,
                                DevicePolicyManager manager, ComponentName admin) throws Exception {
        JSONObject policy = new JSONObject(cache.getString("desired", "{}"));
        require(policy.getLong("version") == 301 || policy.getLong("version") == 302);
        require(!policy.getBoolean("internetPaused") && policy.getJSONArray("schedules").length() == 0);
        for (String field : new String[]{"blockedPackages", "alwaysAllowedPackages"})
            for (int i = 0; i < policy.getJSONArray(field).length(); i++) require(PackagePolicyProbe.TARGET.equals(policy.getJSONArray(field).getString(i)));
        require(ledger.getStringSet("tracked", Collections.emptySet()).stream().allMatch(PackagePolicyProbe.TARGET::equals));
        PackagePolicyProbe.save(cache, 302, null, null);
        PackagePolicyProbe.applied(new PolicyEnforcer(context).applyCached());
        require(!manager.isPackageSuspended(admin, PackagePolicyProbe.TARGET));
        Map<String, Object> oldCache = new HashMap<>(), oldLedger = new HashMap<>();
        if (fixture.getBoolean("hadAlarmMode", false)) oldCache.put("alarmMode", fixture.getString("alarmMode", "none"));
        if (fixture.getBoolean("hadTracked", false)) oldLedger.put("tracked", Collections.emptySet());
        PackagePolicyProbe.restore(ledger, oldLedger); PackagePolicyProbe.restore(cache, oldCache);
        require(fixture.edit().clear().commit());
    }
    public static Bundle run(Context context, String phase) {
        Bundle result = new Bundle();
        SharedPreferences cache = context.getSharedPreferences("policy-cache", Context.MODE_PRIVATE);
        SharedPreferences ledger = context.getSharedPreferences("package-reconciliation", Context.MODE_PRIVATE);
        SharedPreferences fixture = context.getSharedPreferences("fixture-policy-upgrade", Context.MODE_PRIVATE);
        DevicePolicyManager manager = context.getSystemService(DevicePolicyManager.class);
        ComponentName admin = new ComponentName(context, AppControlDeviceAdminReceiver.class);
        boolean cleanupOnFailure = false;
        String stage = "fixture_guard";
        try {
            require(Build.FINGERPRINT.startsWith("google/sdk_gphone64_x86_64/emu64xa:"));
            require(context.getSharedPreferences("enrollment", Context.MODE_PRIVATE).getString("deviceId", null) == null);
            require(manager.isDeviceOwnerApp(context.getPackageName()));
            int boot = Settings.Global.getInt(context.getContentResolver(), Settings.Global.BOOT_COUNT, -1); require(boot >= 0);
            long version = context.getPackageManager().getPackageInfo(context.getPackageName(), 0).getLongVersionCode();
            if ("prepare".equals(phase)) {
                require(fixture.getAll().isEmpty() && cache.getString("desired", null) == null);
                require(cache.getAll().keySet().stream().allMatch("alarmMode"::equals));
                require(ledger.getAll().keySet().stream().allMatch("tracked"::equals));
                require(ledger.getStringSet("tracked", Collections.emptySet()).isEmpty());
                require(!manager.isPackageSuspended(admin, PackagePolicyProbe.TARGET));
                require(fixture.edit().putBoolean("pending", true).putInt("bootCount", boot).putLong("versionCode", version)
                    .putString("publicKey", new DeviceIdentity().publicKey()).putBoolean("hadAlarmMode", cache.contains("alarmMode"))
                    .putString("alarmMode", cache.getString("alarmMode", "none")).putBoolean("hadTracked", ledger.contains("tracked")).commit());
                cleanupOnFailure = true; stage = "prepare_cached_policy";
                PackagePolicyProbe.save(cache, 301, PackagePolicyProbe.TARGET, null);
                PackagePolicyProbe.applied(new PolicyEnforcer(context).applyCached());
                require(manager.isPackageSuspended(admin, PackagePolicyProbe.TARGET));
                require(manager.setPackagesSuspended(admin, new String[]{PackagePolicyProbe.TARGET}, false).length == 0);
                require(!manager.isPackageSuspended(admin, PackagePolicyProbe.TARGET));
                result.putString("policy_upgrade_probe", "prepared");
            } else if ("verify".equals(phase)) {
                require(fixture.getBoolean("pending", false)); cleanupOnFailure = true; stage = "package_replacement";
                require(version > fixture.getLong("versionCode", Long.MAX_VALUE) && boot == fixture.getInt("bootCount", -1));
                require(new DeviceIdentity().publicKey().equals(fixture.getString("publicKey", "")));
                require(cache.getLong("version", -1) == 301 && cache.getLong("lastAttemptedVersion", -1) == 301);
                require("applied".equals(cache.getString("localResult", "")) && cache.getInt("localBootCount", -1) == boot);
                require(manager.isPackageSuspended(admin, PackagePolicyProbe.TARGET));
                require(ledger.getStringSet("tracked", Collections.emptySet()).contains(PackagePolicyProbe.TARGET));
                cleanup(context, cache, ledger, fixture, manager, admin); cleanupOnFailure = false;
                result.putString("policy_upgrade_probe", "passed");
            } else if ("cleanup".equals(phase)) {
                require(fixture.getBoolean("pending", false)); cleanup(context, cache, ledger, fixture, manager, admin);
                result.putString("policy_upgrade_probe", "cleaned");
            } else throw new IllegalArgumentException();
            result.putString("stream", "Android package upgrade phase " + phase + " completed\n");
        } catch (Exception failure) {
            result.putString("policy_upgrade_probe", "failed"); result.putString("observation", stage);
            if (cleanupOnFailure) {
                try { cleanup(context, cache, ledger, fixture, manager, admin); }
                catch (Exception ignored) { result.putString("cleanup", "failed_reconciliation_state_retained"); }
            }
        }
        return result;
    }
}
