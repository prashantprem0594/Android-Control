package com.appcontrol.child;

import android.app.Instrumentation;
import android.app.admin.DevicePolicyManager;
import android.app.role.RoleManager;
import android.content.Context;
import android.content.SharedPreferences;
import android.os.Build;
import android.os.Bundle;
import android.os.Process;
import android.os.SystemClock;
import android.provider.Settings;
import android.telecom.TelecomManager;
import com.appcontrol.child.calls.CallActionGate;
import com.appcontrol.child.calls.CallRegistry;
import java.time.Instant;
import java.util.ArrayDeque;
import android.view.accessibility.AccessibilityNodeInfo;

/** Synthetic call only. External process death must be separately observed. */
public final class TelecomRecoveryProbe {
    private static final String PROOF = "fixture-telecom-recovery";
    private static void require(boolean value) { if (!value) throw new IllegalStateException(); }
    private static void endInPreloadedUi(Context context, Instrumentation instrumentation) throws Exception {
        String system=context.getSystemService(TelecomManager.class).getSystemDialerPackage();
        require(system!=null && !system.equals(context.getPackageName()));
        long deadline=SystemClock.elapsedRealtime()+15000;
        while(SystemClock.elapsedRealtime()<deadline){
            AccessibilityNodeInfo root=instrumentation.getUiAutomation().getRootInActiveWindow();
            ArrayDeque<AccessibilityNodeInfo> nodes=new ArrayDeque<>();if(root!=null)nodes.add(root);
            int visited=0;
            while(!nodes.isEmpty() && visited++<512){
                AccessibilityNodeInfo node=nodes.removeFirst();
                String text=node.getText()==null?"":node.getText().toString();
                String description=node.getContentDescription()==null?"":node.getContentDescription().toString();
                if(system.contentEquals(node.getPackageName()==null?"":node.getPackageName()) && node.isVisibleToUser() &&
                    ("End call".equalsIgnoreCase(text)||"End call".equalsIgnoreCase(description))){
                    AccessibilityNodeInfo target=node;
                    for(int parents=0;parents<3 && target!=null;parents++,target=target.getParent()){
                        if(!system.contentEquals(target.getPackageName()==null?"":target.getPackageName()))break;
                        if(target.isEnabled() && target.isClickable() && target.performAction(AccessibilityNodeInfo.ACTION_CLICK))return;
                    }
                }
                for(int i=0;i<node.getChildCount();i++){AccessibilityNodeInfo child=node.getChild(i);if(child!=null)nodes.addLast(child);}
            }
            Thread.sleep(100);
        }
        throw new IllegalStateException();
    }
    private static void awaitDisconnected(Context context) throws Exception {
        require(context.checkSelfPermission(android.Manifest.permission.READ_PHONE_STATE)==android.content.pm.PackageManager.PERMISSION_GRANTED);
        long deadline=SystemClock.elapsedRealtime()+15000;
        while(SystemClock.elapsedRealtime()<deadline){
            if("disconnected".equals(TelecomFixtureProbe.provider(context,"state")) && !context.getSystemService(TelecomManager.class).isInCall())return;
            Thread.sleep(100);
        }
        throw new IllegalStateException();
    }
    public static Bundle run(Context context, Instrumentation instrumentation, String phase) {
        Bundle result = new Bundle(); String stage="guard";
        SharedPreferences proof=context.getSharedPreferences(PROOF,Context.MODE_PRIVATE);
        try {
            require(Build.FINGERPRINT.startsWith("google/sdk_gphone64_x86_64/emu64xa:") && BuildConfig.DEBUG);
            require(context.getSharedPreferences("enrollment",Context.MODE_PRIVATE).getAll().isEmpty());
            require(context.getSystemService(DevicePolicyManager.class).isDeviceOwnerApp(context.getPackageName()));
            if (!"cleanup".equals(phase)) require(context.getSystemService(RoleManager.class).isRoleHeld(RoleManager.ROLE_DIALER));
            if ("prepare".equals(phase)) {
                stage="prepare";require(proof.getAll().isEmpty());
                require(TelecomFixtureProbe.snapshot(instrumentation)==null && !context.getSystemService(TelecomManager.class).isInCall());
                require("prepared".equals(TelecomFixtureProbe.provider(context,"prepare")));
                result.putString("telecom_recovery_probe","prepared");
            } else if ("arm".equals(phase)) {
                stage="incoming";require(proof.getAll().isEmpty());
                require(TelecomFixtureProbe.snapshot(instrumentation)==null);
                require("requested".equals(TelecomFixtureProbe.provider(context,"incoming")));
                TelecomFixtureProbe.Snapshot incoming=TelecomFixtureProbe.await(instrumentation,CallActionGate.State.RINGING,null);
                require(TelecomFixtureProbe.main(instrumentation,()->CallRegistry.INSTANCE.userAction(incoming.id,CallActionGate.Action.ANSWER))==CallActionGate.Result.REQUESTED);
                TelecomFixtureProbe.Snapshot active=TelecomFixtureProbe.await(instrumentation,CallActionGate.State.ACTIVE,incoming.id);
                require("active".equals(TelecomFixtureProbe.provider(context,"state")));
                int boot=Settings.Global.getInt(context.getContentResolver(),Settings.Global.BOOT_COUNT,-1);require(boot>=0);
                require(proof.edit().putBoolean("pending",true).putString("oldCall",active.id).putInt("pid",Process.myPid()).putInt("boot",boot).commit());
                TelecomFixtureProbe.open(context,instrumentation,active.id);
                Bundle armed=new Bundle();armed.putString("telecom_recovery_armed","true");
                armed.putInt("telecom_recovery_pid",Process.myPid());
                armed.putString("telecom_recovery_version","process-recovery-v1");
                instrumentation.sendStatus(0,armed);
                // Finishing instrumentation itself terminates the target. Stay alive
                // until the separately controlled SIGKILL, with bounded cleanup on timeout.
                long holdDeadline=SystemClock.elapsedRealtime()+90000;
                while(SystemClock.elapsedRealtime()<holdDeadline)Thread.sleep(100);
                throw new IllegalStateException();
            } else if ("fallback".equals(phase)) {
                stage="preloaded_call_continuity";require(proof.getBoolean("pending",false));
                require(Process.myPid()!=proof.getInt("pid",-1));
                require(Settings.Global.getInt(context.getContentResolver(),Settings.Global.BOOT_COUNT,-2)==proof.getInt("boot",-1));
                require("active".equals(TelecomFixtureProbe.provider(context,"state")));
                String old=proof.getString("oldCall","");require(!old.isEmpty());
                require(TelecomFixtureProbe.main(instrumentation,()->CallRegistry.INSTANCE.userAction(old,CallActionGate.Action.END))==CallActionGate.Result.NOT_FOUND);
                require(TelecomFixtureProbe.main(instrumentation,()->CallRegistry.INSTANCE.remoteAction(old,CallActionGate.Action.END,Instant.now().plusSeconds(5)))==CallActionGate.Result.NOT_FOUND);
                stage="local_android_controls";
                TelecomFixtureProbe.open(context,instrumentation,old);
                TelecomFixtureProbe.control(instrumentation,"Open Android call controls",true);
                stage="preloaded_end_control";endInPreloadedUi(context,instrumentation);awaitDisconnected(context);
                require(proof.edit().putBoolean("fallbackEnded",true).commit());
                result.putString("telecom_recovery_probe","passed");
            } else if ("next-call".equals(phase)) {
                stage="next_call_guard";require(proof.getBoolean("pending",false) && proof.getBoolean("fallbackEnded",false));
                require(!context.getSystemService(TelecomManager.class).isInCall() && TelecomFixtureProbe.snapshot(instrumentation)==null);
                String old=proof.getString("oldCall","");require(!old.isEmpty());
                stage="next_call_request";
                require("requested".equals(TelecomFixtureProbe.provider(context,"incoming")));
                stage="next_call_redelivery";
                TelecomFixtureProbe.Snapshot next=TelecomFixtureProbe.await(instrumentation,CallActionGate.State.RINGING,null);
                stage="next_call_fresh_identity";
                require(!old.equals(next.id));
                stage="next_call_stale_action";
                require(TelecomFixtureProbe.main(instrumentation,()->CallRegistry.INSTANCE.userAction(old,CallActionGate.Action.REJECT))==CallActionGate.Result.NOT_FOUND);
                stage="next_call_local_reject";
                require(TelecomFixtureProbe.main(instrumentation,()->CallRegistry.INSTANCE.userAction(next.id,CallActionGate.Action.REJECT))==CallActionGate.Result.REQUESTED);
                awaitDisconnected(context);result.putString("telecom_recovery_probe","passed");
            } else if ("recover".equals(phase)) {
                stage="process_and_connection";require(proof.getBoolean("pending",false));
                require(Process.myPid()!=proof.getInt("pid",-1));
                require(Settings.Global.getInt(context.getContentResolver(),Settings.Global.BOOT_COUNT,-2)==proof.getInt("boot",-1));
                require("active".equals(TelecomFixtureProbe.provider(context,"state")));
                stage="redelivered_call";
                TelecomFixtureProbe.Snapshot active=TelecomFixtureProbe.await(instrumentation,CallActionGate.State.ACTIVE,null);
                String old=proof.getString("oldCall","");require(!old.isEmpty() && !old.equals(active.id));
                stage="stale_identity_refusal";
                require(TelecomFixtureProbe.main(instrumentation,()->CallRegistry.INSTANCE.userAction(old,CallActionGate.Action.END))==CallActionGate.Result.NOT_FOUND);
                require(TelecomFixtureProbe.main(instrumentation,()->CallRegistry.INSTANCE.remoteAction(old,CallActionGate.Action.END,Instant.now().plusSeconds(5)))==CallActionGate.Result.NOT_FOUND);
                require(TelecomFixtureProbe.main(instrumentation,()->CallRegistry.INSTANCE.remoteAction(active.id,CallActionGate.Action.END,Instant.now().plusSeconds(5)))==CallActionGate.Result.EMERGENCY_PROTECTED);
                stage="local_control_after_recovery";
                require(TelecomFixtureProbe.main(instrumentation,()->CallRegistry.INSTANCE.userAction(active.id,CallActionGate.Action.END))==CallActionGate.Result.REQUESTED);
                long deadline=SystemClock.elapsedRealtime()+15000;boolean ended=false;
                while(SystemClock.elapsedRealtime()<deadline){
                    if(TelecomFixtureProbe.snapshot(instrumentation)==null && "disconnected".equals(TelecomFixtureProbe.provider(context,"state"))){ended=true;break;}
                    Thread.sleep(100);
                }
                require(ended);result.putString("telecom_recovery_probe","passed");
            } else if ("cleanup".equals(phase)) {
                stage="cleanup";require("cleaned".equals(TelecomFixtureProbe.provider(context,"cleanup")));
                require(context.deleteSharedPreferences(PROOF));result.putString("telecom_recovery_probe","cleaned");
            } else throw new IllegalArgumentException();
        } catch(Exception ignored){
            result.putString("telecom_recovery_probe","failed");result.putString("telecom_recovery_observation",stage);
            try{
                TelecomFixtureProbe.Snapshot current=TelecomFixtureProbe.snapshot(instrumentation);
                result.putString("telecom_recovery_registry",current==null?"none":current.state.name());
                result.putString("telecom_recovery_fixture",TelecomFixtureProbe.provider(context,"state"));
            }catch(Exception observationFailure){}
            if("arm".equals(phase) && proof.getBoolean("pending",false))try{
                require("cleaned".equals(TelecomFixtureProbe.provider(context,"cleanup")));
                require(context.deleteSharedPreferences(PROOF));
            }catch(Exception cleanupFailure){result.putString("telecom_recovery_cleanup","failed");}
        }
        result.putString("telecom_recovery_version","process-recovery-v1");
        return result;
    }
}
