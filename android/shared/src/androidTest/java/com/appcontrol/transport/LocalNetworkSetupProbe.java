package com.appcontrol.transport;

import android.app.Activity;
import android.app.Instrumentation;
import android.content.Context;
import android.content.Intent;
import android.content.pm.ApplicationInfo;
import android.os.Build;
import android.os.Bundle;
import android.os.SystemClock;
import android.view.View;
import android.view.ViewGroup;
import android.view.accessibility.AccessibilityNodeInfo;
import android.widget.Button;
import android.widget.TextView;

/** Requests LAN access only through the production setup button in disposable acceptance apps. */
public final class LocalNetworkSetupProbe {
    private static void require(boolean value) { if (!value) throw new IllegalStateException(); }
    private static Button find(View view) {
        if (view instanceof Button && "Allow home-network server access".contentEquals(((Button) view).getText())) return (Button) view;
        if (view instanceof ViewGroup) {
            ViewGroup group = (ViewGroup) view;
            for (int i = 0; i < group.getChildCount(); i++) { Button found = find(group.getChildAt(i)); if (found != null) return found; }
        }
        return null;
    }
    private static boolean choice(Instrumentation instrumentation, boolean allow) {
        AccessibilityNodeInfo root = instrumentation.getUiAutomation().getRootInActiveWindow();
        if (root == null) return false;
        for (String search : allow ? new String[]{"Allow"} : new String[]{"allow", "Deny"}) {
            for (AccessibilityNodeInfo node : root.findAccessibilityNodeInfosByText(search)) {
                String owner = node.getPackageName() == null ? "" : node.getPackageName().toString();
                String label = node.getText() == null ? "" : node.getText().toString().replace((char) 0x2019, (char) 0x27);
                boolean matches = allow ? "Allow".equalsIgnoreCase(label) : "Don't allow".equalsIgnoreCase(label) || "Deny".equalsIgnoreCase(label);
                if ((owner.equals("com.android.permissioncontroller") || owner.equals("com.google.android.permissioncontroller")) &&
                    matches && node.isClickable() && node.isVisibleToUser()) return node.performAction(AccessibilityNodeInfo.ACTION_CLICK);
            }
        }
        return false;
    }
    private static boolean denialShown(View view) {
        if (view instanceof TextView && ((TextView) view).getText().toString().startsWith("Home-network server access was not allowed.")) return true;
        if (view instanceof ViewGroup) {
            ViewGroup group = (ViewGroup) view;
            for (int i = 0; i < group.getChildCount(); i++) if (denialShown(group.getChildAt(i))) return true;
        }
        return false;
    }
    public static Bundle run(Context context, Instrumentation instrumentation, boolean denyOnce) {
        Bundle result = new Bundle(); Activity activity = null; String phase = "guard";
        try {
            String pkg = context.getPackageName();
            require(pkg.equals("com.appcontrol.child.acceptance") || pkg.equals("com.appcontrol.parent.acceptance"));
            require(Build.FINGERPRINT.startsWith("google/sdk_gphone64_x86_64/emu64xa:"));
            require((context.getApplicationInfo().flags & ApplicationInfo.FLAG_DEBUGGABLE) != 0);
            if (denyOnce) {
                require(pkg.equals("com.appcontrol.parent.acceptance") && LocalNetworkAccess.supported(context) && !LocalNetworkAccess.granted(context));
                require(context.getSharedPreferences("parent-session", Context.MODE_PRIVATE).getAll().isEmpty());
            }
            if (!LocalNetworkAccess.supported(context)) result.putString("local_network_setup_state", "not_required");
            else if (LocalNetworkAccess.granted(context)) result.putString("local_network_setup_state", "already_allowed");
            else {
                require(LocalNetworkAccess.neededForOrigin(context, "https://10.0.2.2:8443"));
                require(LocalNetworkAccess.neededForOrigin(context, "https://server.local"));
                require(LocalNetworkAccess.neededForOrigin(context, "https://[fd00::1]"));
                require(!LocalNetworkAccess.neededForOrigin(context, "https://localhost:8443"));
                require(!LocalNetworkAccess.neededForOrigin(context, "https://public.example.org"));
                String screen = pkg.contains(".parent.") ? "com.appcontrol.parent.ParentActivity" : "com.appcontrol.child.MainActivity";
                phase = "setup_screen";
                activity = instrumentation.startActivitySync(new Intent(context, context.getClassLoader().loadClass(screen)).addFlags(Intent.FLAG_ACTIVITY_NEW_TASK));
                final Activity shown = activity;
                instrumentation.runOnMainSync(() -> { Button button = find(shown.getWindow().getDecorView()); require(button != null && button.performClick()); });
                if (denyOnce) {
                    phase = "permission_denial";
                    long denialDeadline = SystemClock.elapsedRealtime() + 15000;
                    boolean clicked = false;
                    while (!clicked && SystemClock.elapsedRealtime() < denialDeadline) { clicked = choice(instrumentation, false); if (!clicked) SystemClock.sleep(100); }
                    require(clicked);
                    final boolean[] acknowledged = new boolean[1];
                    while (!acknowledged[0] && SystemClock.elapsedRealtime() < denialDeadline) {
                        instrumentation.runOnMainSync(() -> acknowledged[0] = denialShown(shown.getWindow().getDecorView()));
                        if (!acknowledged[0]) SystemClock.sleep(100);
                    }
                    require(acknowledged[0] && !LocalNetworkAccess.granted(context));
                    require(LocalNetworkAccess.neededForOrigin(context, "https://10.0.2.2"));
                    require(context.getSharedPreferences("parent-session", Context.MODE_PRIVATE).getAll().isEmpty());
                    result.putString("local_network_denial_probe", "passed");
                    phase = "explicit_permission_retry";
                    instrumentation.runOnMainSync(() -> require(find(shown.getWindow().getDecorView()).performClick()));
                }
                phase = "permission_prompt";
                long deadline = SystemClock.elapsedRealtime() + 25000;
                while (!LocalNetworkAccess.granted(context) && SystemClock.elapsedRealtime() < deadline) {
                    choice(instrumentation, true);
                    SystemClock.sleep(100);
                }
                require(LocalNetworkAccess.granted(context));
                instrumentation.waitForIdleSync();
                require(!LocalNetworkAccess.neededForOrigin(context, "https://10.0.2.2"));
                result.putString("local_network_setup_state", "granted_through_production_ui");
            }
            result.putString("local_network_setup_probe", "passed");
        } catch (Exception failure) {
            result.putString("local_network_setup_probe", "failed"); result.putString("local_network_setup_phase", phase);
        } finally {
            final Activity shown = activity;
            try { if (shown != null) { instrumentation.runOnMainSync(shown::finish); instrumentation.waitForIdleSync(); } }
            catch (Exception failure) { result.putString("local_network_setup_probe", "failed"); result.putString("local_network_setup_phase", "cleanup"); }
        }
        return result;
    }
}
