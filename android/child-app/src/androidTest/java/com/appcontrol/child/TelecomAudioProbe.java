package com.appcontrol.child;

import android.app.Instrumentation;
import android.app.admin.DevicePolicyManager;
import android.app.role.RoleManager;
import android.content.Context;
import android.os.Build;
import android.os.Bundle;
import android.os.SystemClock;
import android.telecom.TelecomManager;
import com.appcontrol.child.calls.CallActionGate;
import com.appcontrol.child.calls.CallRegistry;
import com.appcontrol.child.calls.ManagedInCallService;
import java.util.List;

/** Observed endpoint/feedback checks, no assertion about host or physical sound. */
public final class TelecomAudioProbe {
    private static void require(boolean value) { if (!value) throw new IllegalStateException(); }
    private static void observed(Instrumentation instrumentation,String call,String label) throws Exception {
        long deadline=SystemClock.elapsedRealtime()+12000;
        while(SystemClock.elapsedRealtime()<deadline){
            if(TelecomFixtureProbe.main(instrumentation,()->{
                ManagedInCallService service=CallRegistry.INSTANCE.getService();
                return service!=null && CallRegistry.INSTANCE.find(call)!=null && label.equals(CallRegistry.INSTANCE.getAudioStatus()) && service.audioMessage(call)==null;
            }))return;
            Thread.sleep(100);
        }
        throw new IllegalStateException();
    }
    public static Bundle run(Context context,Instrumentation instrumentation,String phase){
        Bundle result=new Bundle();String stage="guard";
        try{
            require(Build.FINGERPRINT.startsWith("google/sdk_gphone64_x86_64/emu64xa:") && BuildConfig.DEBUG && Build.VERSION.SDK_INT>=34);
            require(context.getSharedPreferences("enrollment",Context.MODE_PRIVATE).getAll().isEmpty());
            require(context.getSystemService(DevicePolicyManager.class).isDeviceOwnerApp(context.getPackageName()));
            if(!"cleanup".equals(phase))require(context.getSystemService(RoleManager.class).isRoleHeld(RoleManager.ROLE_DIALER));
            if("prepare".equals(phase)){
                require(TelecomFixtureProbe.snapshot(instrumentation)==null && !context.getSystemService(TelecomManager.class).isInCall());
                require("prepared".equals(TelecomFixtureProbe.provider(context,"prepare")));
                result.putString("telecom_audio_probe","prepared");
            }else if("run".equals(phase)){
                stage="incoming";require("requested".equals(TelecomFixtureProbe.provider(context,"incoming")));
                TelecomFixtureProbe.Snapshot ringing=TelecomFixtureProbe.await(instrumentation,CallActionGate.State.RINGING,null);
                require(TelecomFixtureProbe.main(instrumentation,()->CallRegistry.INSTANCE.userAction(ringing.id,CallActionGate.Action.ANSWER))==CallActionGate.Result.REQUESTED);
                TelecomFixtureProbe.Snapshot active=TelecomFixtureProbe.await(instrumentation,CallActionGate.State.ACTIVE,ringing.id);
                TelecomFixtureProbe.open(context,instrumentation,active.id);
                stage="available_outputs";List<ManagedInCallService.AudioChoice> choices=null;
                long deadline=SystemClock.elapsedRealtime()+10000;
                while(SystemClock.elapsedRealtime()<deadline){
                    choices=TelecomFixtureProbe.main(instrumentation,()->CallRegistry.INSTANCE.getService().audioChoices(active.id));
                    if(choices.stream().anyMatch(choice->"speaker".equals(choice.getKind())))break;
                    Thread.sleep(100);
                }
                require(choices!=null && choices.stream().anyMatch(choice->"speaker".equals(choice.getKind())));
                boolean earpiece=choices.stream().anyMatch(choice->"earpiece".equals(choice.getKind()));
                result.putString("telecom_audio_initial_route",TelecomFixtureProbe.main(instrumentation,()->CallRegistry.INSTANCE.getAudioStatus()));
                stage="speaker_shortcut";TelecomFixtureProbe.control(instrumentation,"Speaker",true);observed(instrumentation,active.id,"Speaker");
                stage="same_speaker_picker";TelecomFixtureProbe.control(instrumentation,"Audio output",true);
                TelecomFixtureProbe.control(instrumentation,"Speaker",true);observed(instrumentation,active.id,"Speaker");
                if(earpiece){
                    stage="earpiece_shortcut";TelecomFixtureProbe.control(instrumentation,"Phone audio",true);observed(instrumentation,active.id,"Earpiece");
                    stage="same_earpiece_picker";TelecomFixtureProbe.control(instrumentation,"Audio output",true);
                    TelecomFixtureProbe.control(instrumentation,"Earpiece",true);observed(instrumentation,active.id,"Earpiece");
                }
                stage="local_end";TelecomFixtureProbe.control(instrumentation,"End call",true);TelecomFixtureProbe.removed(context,instrumentation);
                result.putString("telecom_audio_probe","passed");result.putString("telecom_audio_speaker","observed");
                result.putString("telecom_audio_earpiece",earpiece?"observed":"unavailable");
            }else if("cleanup".equals(phase)){
                require("cleaned".equals(TelecomFixtureProbe.provider(context,"cleanup")));result.putString("telecom_audio_probe","cleaned");
            }else throw new IllegalArgumentException();
        }catch(Exception ignored){
            result.putString("telecom_audio_probe","failed");result.putString("telecom_audio_observation",stage);
            try{
                result.putString("telecom_audio_callback_route",TelecomFixtureProbe.main(instrumentation,()->CallRegistry.INSTANCE.getAudioStatus()));
                result.putInt("telecom_audio_current_endpoint_type",TelecomFixtureProbe.main(instrumentation,()->{
                    ManagedInCallService service=CallRegistry.INSTANCE.getService();
                    return service==null||service.getCurrentCallEndpoint()==null?-1:service.getCurrentCallEndpoint().getEndpointType();
                }));
                result.putString("telecom_audio_feedback",TelecomFixtureProbe.main(instrumentation,()->{
                    ManagedInCallService service=CallRegistry.INSTANCE.getService();String message=service==null?null:service.getRouteRequestMessage();
                    return message==null?"none":message.contains("failed")?"failed":message.contains("not been confirmed")?"unconfirmed":"waiting";
                }));
            }catch(Exception observationFailure){}
        }
        result.putString("telecom_audio_version","endpoint-feedback-v1");
        result.putString("telecom_audio_scope","platform-callbacks-only");
        return result;
    }
}
