package com.appcontrol.child;

import android.app.Instrumentation;
import android.app.KeyguardManager;
import android.app.admin.DevicePolicyManager;
import android.app.role.RoleManager;
import android.content.Context;
import android.content.Intent;
import android.net.Uri;
import android.os.Build;
import android.os.Bundle;
import android.os.SystemClock;
import android.telecom.TelecomManager;
import android.view.accessibility.AccessibilityNodeInfo;
import com.appcontrol.child.calls.CallActionGate;
import com.appcontrol.child.calls.CallRegistry;
import com.appcontrol.child.calls.CallActivity;
import java.time.Instant;
import java.util.List;
import java.util.concurrent.Callable;
import java.util.concurrent.atomic.AtomicReference;

/** Real Telecom callbacks for synthetic non-SIM calls, never network audio. */
public final class TelecomFixtureProbe {
    private static String uiObservation = "none";
    private static final Uri CONTROL = Uri.parse("content://org.example.appcontroltelecomfixture.control");
    private static void require(boolean value) { if (!value) throw new IllegalStateException("Synthetic Telecom invariant failed"); }
<<<<<<< HEAD
    static String provider(Context context, String method) {
        Bundle result = context.getContentResolver().call(CONTROL, method, null, null);
        require(result != null); return result.getString("result", "failed");
    }
    static <T> T main(Instrumentation instrumentation, Callable<T> action) throws Exception {
=======
    private static String provider(Context context, String method) {
        Bundle result = context.getContentResolver().call(CONTROL, method, null, null);
        require(result != null); return result.getString("result", "failed");
    }
    private static <T> T main(Instrumentation instrumentation, Callable<T> action) throws Exception {
>>>>>>> origin/main
        AtomicReference<T> value = new AtomicReference<>(); AtomicReference<Exception> error = new AtomicReference<>();
        instrumentation.runOnMainSync(() -> { try { value.set(action.call()); } catch (Exception failure) { error.set(failure); } });
        if (error.get() != null) throw error.get(); return value.get();
    }
<<<<<<< HEAD
    static final class Snapshot {
        final String id; final CallActionGate.State state;
        Snapshot(String id, CallActionGate.State state) { this.id = id; this.state = state; }
    }
    static Snapshot snapshot(Instrumentation instrumentation) throws Exception {
=======
    private static final class Snapshot {
        final String id; final CallActionGate.State state;
        Snapshot(String id, CallActionGate.State state) { this.id = id; this.state = state; }
    }
    private static Snapshot snapshot(Instrumentation instrumentation) throws Exception {
>>>>>>> origin/main
        return main(instrumentation, () -> {
            List<CallRegistry.Entry> calls = CallRegistry.INSTANCE.all(); require(calls.size() <= 1);
            if (calls.isEmpty()) return null;
            require(CallRegistry.INSTANCE.getService() != null);
            CallRegistry.Entry entry = calls.get(0); return new Snapshot(entry.getId(), CallRegistry.INSTANCE.state(entry.getCall()));
        });
    }
<<<<<<< HEAD
    static Snapshot await(Instrumentation instrumentation, CallActionGate.State state, String id) throws Exception {
=======
    private static Snapshot await(Instrumentation instrumentation, CallActionGate.State state, String id) throws Exception {
>>>>>>> origin/main
        long deadline = SystemClock.elapsedRealtime() + 15000;
        while (SystemClock.elapsedRealtime() < deadline) {
            Snapshot current = snapshot(instrumentation);
            if (current != null && current.state == state && (id == null || id.equals(current.id))) return current;
            Thread.sleep(100);
        }
        throw new IllegalStateException("Synthetic Telecom state timeout");
    }
<<<<<<< HEAD
    static void removed(Context context, Instrumentation instrumentation) throws Exception {
=======
    private static void removed(Context context, Instrumentation instrumentation) throws Exception {
>>>>>>> origin/main
        long deadline = SystemClock.elapsedRealtime() + 15000;
        while (SystemClock.elapsedRealtime() < deadline) {
            if (snapshot(instrumentation) == null && "disconnected".equals(provider(context, "state"))) return;
            Thread.sleep(100);
        }
        throw new IllegalStateException("Synthetic call removal timeout");
    }
<<<<<<< HEAD
    static void open(Context context, Instrumentation instrumentation, String id) throws Exception {
        main(instrumentation, () -> { context.startActivity(new Intent(context, CallActivity.class).putExtra("callId", id).addFlags(Intent.FLAG_ACTIVITY_NEW_TASK)); return null; });
        instrumentation.waitForIdleSync();
    }
    static void control(Instrumentation instrumentation, String label, boolean click) throws Exception {
=======
    private static void open(Context context, Instrumentation instrumentation, String id) throws Exception {
        main(instrumentation, () -> { context.startActivity(new Intent(context, CallActivity.class).putExtra("callId", id).addFlags(Intent.FLAG_ACTIVITY_NEW_TASK)); return null; });
        instrumentation.waitForIdleSync();
    }
    private static void control(Instrumentation instrumentation, String label, boolean click) throws Exception {
>>>>>>> origin/main
        long deadline = SystemClock.elapsedRealtime() + 10000;
        while (SystemClock.elapsedRealtime() < deadline) {
            AccessibilityNodeInfo root = instrumentation.getUiAutomation().getRootInActiveWindow();
            if (root != null) {
                for (AccessibilityNodeInfo node : root.findAccessibilityNodeInfosByText(label)) {
                    if (node.getText() == null || node.getPackageName() == null || !label.equalsIgnoreCase(node.getText().toString()) || !"com.appcontrol.child".contentEquals(node.getPackageName())) continue;
                    uiObservation = "label_found";
                    if (!node.isVisibleToUser()) { uiObservation="offscreen"; node.performAction(AccessibilityNodeInfo.AccessibilityAction.ACTION_SHOW_ON_SCREEN.getId()); continue; }
                    if (!click) return;
                    if (!node.isEnabled() || !node.isClickable()) { uiObservation="not_clickable"; continue; }
                    uiObservation="click_requested";
                    if (node.performAction(AccessibilityNodeInfo.ACTION_CLICK)) { uiObservation="click_accepted"; return; }
                    uiObservation="click_refused";
                }
            }
            Thread.sleep(100);
        }
        throw new IllegalStateException("Synthetic call screen control timeout");
    }
    public static Bundle run(Context context, Instrumentation instrumentation, String phase) {
        Bundle result = new Bundle(); result.putString("telecom_probe_version", "ui-controls-v1"); boolean cleanup = false; String stage = "fixture_guard";
        try {
            require(Build.FINGERPRINT.startsWith("google/sdk_gphone64_x86_64/emu64xa:"));
            require(context.getSharedPreferences("enrollment", Context.MODE_PRIVATE).getString("deviceId", null) == null);
            require(context.getSystemService(DevicePolicyManager.class).isDeviceOwnerApp(context.getPackageName()));
            require(context.getSystemService(RoleManager.class).isRoleHeld(RoleManager.ROLE_DIALER));
            if ("prepare".equals(phase)) {
                require(!context.getSystemService(KeyguardManager.class).isKeyguardLocked());
                require(snapshot(instrumentation) == null && !context.getSystemService(TelecomManager.class).isInCall());
                cleanup = true; stage = "account_registration"; require("prepared".equals(provider(context, "prepare")));
                cleanup = false; result.putString("telecom_probe", "prepared");
            } else if ("run".equals(phase)) {
                require(snapshot(instrumentation) == null); cleanup = true;
                stage = "incoming_callback"; require("requested".equals(provider(context, "incoming")));
                Snapshot incoming = await(instrumentation, CallActionGate.State.RINGING, null); final String first = incoming.id;
                stage = "remote_safety_gates";
                require(main(instrumentation, () -> CallRegistry.INSTANCE.remoteAction(first, CallActionGate.Action.ANSWER, Instant.now().minusSeconds(1))) == CallActionGate.Result.EXPIRED);
                require(main(instrumentation, () -> CallRegistry.INSTANCE.remoteAction("missing-fixture", CallActionGate.Action.ANSWER, Instant.now().plusSeconds(5))) == CallActionGate.Result.NOT_FOUND);
                require(main(instrumentation, () -> CallRegistry.INSTANCE.remoteAction(first, CallActionGate.Action.ANSWER, Instant.now().plusSeconds(120))) == CallActionGate.Result.INVALID_DEADLINE);
                require(main(instrumentation, () -> CallRegistry.INSTANCE.remoteAction(first, CallActionGate.Action.ANSWER, Instant.now().plusSeconds(5))) == CallActionGate.Result.EMERGENCY_PROTECTED);
                require(await(instrumentation, CallActionGate.State.RINGING, first) != null);
                stage = "incoming_call_ui"; open(context, instrumentation, first); control(instrumentation, "Incoming call", false);
                stage = "answer_ui_click"; control(instrumentation, "Answer audio", true);
                stage = "answer_telecom_callback";
                await(instrumentation, CallActionGate.State.ACTIVE, first); require("active".equals(provider(context, "state")));
                require(main(instrumentation, () -> CallRegistry.INSTANCE.userAction(first, CallActionGate.Action.ANSWER)) == CallActionGate.Result.WRONG_STATE);
                stage = "active_call_ui"; control(instrumentation, "Call active", false);
                stage = "hold_ui_observed"; control(instrumentation, "Hold call", true); await(instrumentation, CallActionGate.State.HOLDING, first);
                require("holding".equals(provider(context, "state"))); control(instrumentation, "Call on hold", false);
                stage = "resume_ui_observed"; control(instrumentation, "Resume call", true); await(instrumentation, CallActionGate.State.ACTIVE, first);
                stage = "mute_ui_observed"; control(instrumentation, "Mute", true); control(instrumentation, "Unmute", false);
                stage = "unmute_ui_observed"; control(instrumentation, "Unmute", true); control(instrumentation, "Mute", false);
                stage = "end_ui_observed"; control(instrumentation, "End call", true);
                removed(context, instrumentation);
                control(instrumentation, "No current call", false);
                require(main(instrumentation, () -> CallRegistry.INSTANCE.userAction(first, CallActionGate.Action.END)) == CallActionGate.Result.NOT_FOUND);
                stage = "new_identity_and_reject"; require("requested".equals(provider(context, "incoming")));
                Snapshot second = await(instrumentation, CallActionGate.State.RINGING, null); require(!second.id.equals(first));
                require(main(instrumentation, () -> CallRegistry.INSTANCE.userAction(first, CallActionGate.Action.REJECT)) == CallActionGate.Result.NOT_FOUND);
                stage = "reject_ui_observed"; open(context, instrumentation, second.id); control(instrumentation, "Reject", true);
                removed(context, instrumentation);
                result.putString("telecom_probe", "passed");
                result.putString("stream", "Synthetic Telecom callbacks and incoming/active UI answer, hold, resume, mute, unmute, end and reject passed; call identity and remote safety gates retained\n");
            } else if ("cleanup".equals(phase)) {
                cleanup = true; require("cleaned".equals(provider(context, "cleanup"))); cleanup = false;
                result.putString("telecom_probe", "cleaned");
            } else throw new IllegalArgumentException();
        } catch (Exception failure) {
            result.putString("telecom_probe", "failed"); result.putString("observation", stage);
            result.putString("ui_observation",uiObservation);
            try { Snapshot current=snapshot(instrumentation);result.putString("call_state",current==null?"none":current.state.name());result.putString("fixture_connection",provider(context,"state")); } catch(Exception ignored) { }
        } finally {
            if (cleanup) {
                try { require("cleaned".equals(provider(context, "cleanup"))); }
                catch (Exception failure) { result.putString("telecom_probe", "failed"); result.putString("cleanup", "failed"); }
            }
        }
        return result;
    }
}
