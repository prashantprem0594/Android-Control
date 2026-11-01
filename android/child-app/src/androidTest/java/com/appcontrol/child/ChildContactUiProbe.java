package com.appcontrol.child;

import android.app.Activity;
import android.app.Instrumentation;
import android.app.role.RoleManager;
import android.Manifest;
import android.content.Context;
import android.content.Intent;
import android.content.SharedPreferences;
import android.content.pm.PackageManager;
import android.os.Build;
import android.os.Bundle;
import android.view.View;
import android.view.ViewGroup;
import android.view.WindowManager;
import android.widget.Button;
import android.widget.EditText;
import com.appcontrol.child.calls.CallContactStore;
import com.appcontrol.child.calls.DialActivity;
import java.security.KeyStore;
import java.util.ArrayList;
import java.util.HashMap;
import java.util.List;
import java.util.Map;
import java.util.UUID;
import org.json.JSONArray;
import org.json.JSONObject;

/** Local cache-to-dial-pad UI only. The actual Call button is never pressed. */
public final class ChildContactUiProbe {
    private static final String ALIAS = "appcontrol.callcontacts.v1";
    private static void require(boolean condition) { if (!condition) throw new IllegalStateException(); }
    private static List<View> views(View root) {
        List<View> result = new ArrayList<>(); result.add(root);
        if (root instanceof ViewGroup) {
            ViewGroup group = (ViewGroup) root;
            for (int i = 0; i < group.getChildCount(); i++) result.addAll(views(group.getChildAt(i)));
        }
        return result;
    }
    private static Button button(Activity activity, String label) {
        for (View view : views(activity.getWindow().getDecorView())) {
            if (view instanceof Button && label.contentEquals(((Button) view).getText())) return (Button) view;
        }
        throw new IllegalStateException();
    }
    private static EditText number(Activity activity) {
        for (View view : views(activity.getWindow().getDecorView())) if (view instanceof EditText) return (EditText) view;
        throw new IllegalStateException();
    }
    private static JSONObject snapshot(long version, String label, String number) throws Exception {
        JSONArray contacts = new JSONArray();
        if (label != null) contacts.put(new JSONObject().put("label", label).put("number", number).put("isParent", true));
        return new JSONObject().put("version", version).put("contacts", contacts);
    }
    public static Bundle run(Context context, Instrumentation instrumentation) {
        Bundle result = new Bundle(); String phase = "guard";
        Activity activity = null; boolean touched = false, ownsKey = false, cleanup = true;
        SharedPreferences enrollment = context.getSharedPreferences("enrollment", Context.MODE_PRIVATE);
        SharedPreferences cache = context.getSharedPreferences("call-contact-cache", Context.MODE_PRIVATE);
        Map<String, ?> original = new HashMap<>(cache.getAll());
        try {
            require(BuildConfig.DEBUG && "com.appcontrol.child.acceptance".equals(context.getPackageName()));
            require(Build.FINGERPRINT.startsWith("google/sdk_gphone64_x86_64/emu64xa:"));
            require(context.checkSelfPermission(Manifest.permission.CALL_PHONE) != PackageManager.PERMISSION_GRANTED);
            require(!context.getSystemService(RoleManager.class).isRoleHeld(RoleManager.ROLE_DIALER));
            require(enrollment.getAll().isEmpty());
            for (Object value : original.values()) require(value instanceof String);
            KeyStore keys = KeyStore.getInstance("AndroidKeyStore"); keys.load(null);
            ownsKey = !keys.containsAlias(ALIAS);
            touched = true;
            String origin = BuildConfig.SERVER_ORIGIN.isEmpty() ? "https://contact-ui-fixture.invalid" : BuildConfig.SERVER_ORIGIN;
            String id = "dev_" + UUID.randomUUID().toString().replace("-", "").substring(0, 22);
            require(enrollment.edit().putString("server", origin).putString("deviceId", id).commit());
            CallContactStore store = new CallContactStore(context);
            store.save(snapshot(1, "UI fixture parent", "+12025550126"));
            phase = "dial-screen";
            activity = instrumentation.startActivitySync(new Intent(context, DialActivity.class)
                .setAction(Intent.ACTION_DIAL).addFlags(Intent.FLAG_ACTIVITY_NEW_TASK));
            final Activity screen = activity;
            instrumentation.waitForIdleSync();
            instrumentation.runOnMainSync(() -> {
                require((screen.getWindow().getAttributes().flags & WindowManager.LayoutParams.FLAG_SECURE) != 0);
                require(!number(screen).isSaveEnabled() && number(screen).getText().length() == 0);
                require(button(screen, "Call").isEnabled());
            });
            phase = "parent-shortcut-prefill";
            instrumentation.runOnMainSync(() -> require(button(screen, "Call Parent - UI fixture parent").performClick()));
            instrumentation.runOnMainSync(() -> require("+12025550126".contentEquals(number(screen).getText())));
            phase = "stale-shortcut-refusal";
            final Button[] stale = new Button[1];
            instrumentation.runOnMainSync(() -> {
                stale[0] = button(screen, "Call Parent - UI fixture parent");
                number(screen).setText("+12025550199");
            });
            store.save(snapshot(2, "Updated UI fixture", "+12025550127"));
            instrumentation.runOnMainSync(() -> {
                require(stale[0].performClick());
                require("+12025550199".contentEquals(number(screen).getText()));
                require(button(screen, "Call Parent - Updated UI fixture").isEnabled());
            });
            phase = "updated-shortcut-prefill";
            instrumentation.runOnMainSync(() -> {
                require(button(screen, "Call Parent - Updated UI fixture").performClick());
                require("+12025550127".contentEquals(number(screen).getText()));
                stale[0] = button(screen, "Call Parent - Updated UI fixture");
                number(screen).setText("+12025550199");
            });
            phase = "cleared-shortcut-refusal";
            store.save(snapshot(3, null, null));
            instrumentation.runOnMainSync(() -> {
                require(stale[0].performClick());
                require("+12025550199".contentEquals(number(screen).getText()));
                for (View view : views(screen.getWindow().getDecorView())) if (view instanceof Button) {
                    require(!((Button) view).getText().toString().startsWith("Call Parent - "));
                }
                // Neither this probe nor shortcut handlers invoke the actual Call control.
                require(button(screen, "Call").isEnabled());
            });
            result.putString("child_contact_ui_probe", "passed");
        } catch (Exception failure) {
            result.putString("child_contact_ui_probe", "failed");
            result.putString("child_contact_ui_phase", phase);
        } finally {
            final Activity screen = activity;
            try { if (screen != null) { instrumentation.runOnMainSync(screen::finish); instrumentation.waitForIdleSync(); } }
            catch (Exception failure) { cleanup = false; }
            if (touched) {
                try {
                    SharedPreferences.Editor edit = cache.edit().clear();
                    for (Map.Entry<String, ?> entry : original.entrySet()) edit.putString(entry.getKey(), (String) entry.getValue());
                    require(edit.commit() && original.equals(cache.getAll()));
                } catch (Exception failure) { cleanup = false; }
                try { require(enrollment.edit().clear().commit()); } catch (Exception failure) { cleanup = false; }
                if (ownsKey) try {
                    KeyStore keys = KeyStore.getInstance("AndroidKeyStore"); keys.load(null); keys.deleteEntry(ALIAS);
                    require(!keys.containsAlias(ALIAS));
                } catch (Exception failure) { cleanup = false; }
            }
            if (!cleanup) result.putString("child_contact_ui_probe", "failed");
            result.putString("child_contact_ui_cleanup", cleanup ? "passed" : "failed");
        }
        return result;
    }
}
