package com.appcontrol.child;

import android.app.AlarmManager;
import android.app.Instrumentation;
import android.app.admin.DevicePolicyManager;
import android.content.ComponentName;
import android.content.Context;
import android.content.SharedPreferences;
import android.os.Build;
import android.os.Bundle;
import android.os.SystemClock;
import com.appcontrol.child.admin.AppControlDeviceAdminReceiver;
import com.appcontrol.child.policy.PolicyEnforcer;
import java.time.Instant;
import java.time.ZoneOffset;
import java.time.ZonedDateTime;
import java.time.temporal.ChronoUnit;
import java.util.Collections;
import java.util.Map;
import org.json.JSONArray;
import org.json.JSONObject;

/** Real wall-clock alarms, with no synthetic broadcasts or clock changes. Test APK only. */
public final class ScheduleBoundaryProbe {
    private static void require(boolean value) { if (!value) throw new IllegalStateException("Schedule invariant failed"); }
    private static void status(Instrumentation instrumentation, String stage) {
        Bundle progress = new Bundle(); progress.putString("schedule_stage", stage);
        instrumentation.sendStatus(0, progress);
    }
    private static void await(Context context, DevicePolicyManager manager, ComponentName admin,
                              SharedPreferences cache, Instant boundary, boolean suspended, String previousKey) throws Exception {
        long waitMillis = Math.max(0, boundary.toEpochMilli() - System.currentTimeMillis()) + 45000;
        long deadline = SystemClock.elapsedRealtime() + waitMillis;
        while (SystemClock.elapsedRealtime() < deadline) {
            String key = cache.getString("localStateKey", "");
            if (!Instant.now().isBefore(boundary) && "applied".equals(cache.getString("localResult", "")) &&
                cache.getLong("lastAttemptedVersion", -1) == 201 && !key.isEmpty() && !key.equals(previousKey) &&
                manager.isPackageSuspended(admin, PackagePolicyProbe.TARGET) == suspended &&
                context.getPackageManager().isPackageSuspended(PackagePolicyProbe.TARGET) == suspended) return;
            Thread.sleep(250);
        }
        throw new IllegalStateException("Boundary was not observed within the acceptance tolerance");
    }
    public static Bundle run(Context context, Instrumentation instrumentation) {
        Bundle result = new Bundle();
        SharedPreferences cache = context.getSharedPreferences("policy-cache", Context.MODE_PRIVATE);
        SharedPreferences ledger = context.getSharedPreferences("package-reconciliation", Context.MODE_PRIVATE);
        Map<String, ?> oldCache = cache.getAll(), oldLedger = ledger.getAll();
        DevicePolicyManager manager = context.getSystemService(DevicePolicyManager.class);
        ComponentName admin = new ComponentName(context, AppControlDeviceAdminReceiver.class);
        boolean touched = false;
        String stage = "fixture_guard";
        try {
            require(Build.FINGERPRINT.startsWith("google/sdk_gphone64_x86_64/emu64xa:"));
            require(context.getSharedPreferences("enrollment", Context.MODE_PRIVATE).getString("deviceId", null) == null);
            require(cache.getString("desired", null) == null && ledger.getStringSet("tracked", Collections.emptySet()).isEmpty());
            require(manager.isDeviceOwnerApp(context.getPackageName()));
            require(context.getSystemService(AlarmManager.class).canScheduleExactAlarms());
            require(!manager.isPackageSuspended(admin, PackagePolicyProbe.TARGET));
            require(!new PolicyEnforcer(context).isProtected(PackagePolicyProbe.TARGET));
            ZonedDateTime start = Instant.now().plusSeconds(75).atZone(ZoneOffset.UTC).truncatedTo(ChronoUnit.MINUTES);
            ZonedDateTime end = start.plusMinutes(2);
            int startMinute = start.getHour() * 60 + start.getMinute();
            int endMinute = end.getHour() * 60 + end.getMinute();
            JSONObject rule = new JSONObject().put("id", "fixture-boundary").put("timeZone", "UTC")
                .put("daysOfWeek", new JSONArray().put(start.getDayOfWeek().getValue()))
                .put("startMinute", startMinute).put("endMinute", endMinute).put("internetPaused", false)
                .put("blockedPackages", new JSONArray().put(PackagePolicyProbe.TARGET));
            JSONObject policy = new JSONObject().put("formatVersion", 2).put("version", 201)
                .put("internetPaused", false).put("blockedPackages", new JSONArray())
                .put("alwaysAllowedPackages", new JSONArray()).put("schedules", new JSONArray().put(rule));
            touched = true;
            require(cache.edit().putLong("version", 201).putString("desired", policy.toString()).commit());
            stage = "initial_schedule";
            PackagePolicyProbe.applied(new PolicyEnforcer(context).applyCached());
            require("exact".equals(cache.getString("alarmMode", "")));
            require(!manager.isPackageSuspended(admin, PackagePolicyProbe.TARGET));
            String inactiveKey = cache.getString("localStateKey", "");
            stage = "start_boundary"; status(instrumentation, "waiting_for_start");
            await(context, manager, admin, cache, start.toInstant(), true, inactiveKey);
            String activeKey = cache.getString("localStateKey", "");
            require(ledger.getStringSet("tracked", Collections.emptySet()).contains(PackagePolicyProbe.TARGET));
            stage = "end_boundary"; status(instrumentation, "start_passed_waiting_for_end");
            await(context, manager, admin, cache, end.toInstant(), false, activeKey);
            require(inactiveKey.equals(cache.getString("localStateKey", "")));
            require(ledger.getStringSet("tracked", Collections.emptySet()).isEmpty());
            result.putString("schedule_probe", "passed");
            result.putString("stream", "Real Android schedule alarms suspended and restored the fixture with changing effective-state keys\n");
        } catch (Exception failure) {
            result.putString("schedule_probe", "failed"); result.putString("observation", stage);
            result.putString("stream", "Android schedule boundary probe failed at " + stage + "\n");
        } finally {
            if (touched) {
                try {
                    PackagePolicyProbe.save(cache, 202, null, null);
                    PackagePolicyProbe.applied(new PolicyEnforcer(context).applyCached());
                    require(!manager.isPackageSuspended(admin, PackagePolicyProbe.TARGET));
                    PackagePolicyProbe.restore(ledger, oldLedger); PackagePolicyProbe.restore(cache, oldCache);
                } catch (Exception failure) {
                    result.putString("schedule_probe", "failed"); result.putString("observation", "cleanup");
                    result.putString("stream", "Schedule fixture cleanup failed; reconciliation state retained\n");
                }
            }
        }
        return result;
    }
}
