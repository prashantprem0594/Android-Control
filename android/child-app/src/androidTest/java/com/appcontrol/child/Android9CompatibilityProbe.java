package com.appcontrol.child;

import android.app.Instrumentation;
import android.content.Context;
import android.content.Intent;
import android.os.Build;
import android.os.Bundle;
import com.appcontrol.child.calls.DialerSupport;
import com.appcontrol.child.policy.DeviceCapabilities;
import org.json.JSONObject;

/** Read-only capability checks; the runner verifies foreground launch through am start. */
public final class Android9CompatibilityProbe {
    public static Bundle run(Context context, Instrumentation instrumentation) {
        Bundle result = new Bundle();
        String stage = "capabilities";
        try {
            JSONObject capabilities = DeviceCapabilities.INSTANCE.snapshot(context);
            if (capabilities.getInt("androidApi") != Build.VERSION.SDK_INT) throw new IllegalStateException();
            if (Build.VERSION.SDK_INT < 31 && !capabilities.getBoolean("exactAlarms")) throw new IllegalStateException();
            stage = "dialer-request";
            DialerSupport.INSTANCE.isHeld(context);
            Intent request = DialerSupport.INSTANCE.requestIntent(context);
            if (Build.VERSION.SDK_INT == 28 && request != null &&
                !android.telecom.TelecomManager.ACTION_CHANGE_DEFAULT_DIALER.equals(request.getAction())) throw new IllegalStateException();
            result.putString("android_compatibility_probe", "passed");
            result.putInt("android_compatibility_api", Build.VERSION.SDK_INT);
            result.putString("android_compatibility_scope", "capabilities-dialer-intent-only");
        } catch (Exception | LinkageError failure) {
            result.putString("android_compatibility_probe", "failed");
            result.putString("android_compatibility_stage", stage);
        }
        return result;
    }
}
