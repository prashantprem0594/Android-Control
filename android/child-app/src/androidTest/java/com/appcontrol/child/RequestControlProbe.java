package com.appcontrol.child;

import android.os.Bundle;
import com.appcontrol.child.commands.RequestControlChecks;
import com.appcontrol.child.commands.StalledTransportChecks;

/** Executes shared real-socket checks on Android's HTTPS implementation. */
public final class RequestControlProbe {
    private RequestControlProbe() {}
    public static Bundle run() {
        Bundle result = new Bundle();
        String stage = "request_control";
        try {
            RequestControlChecks.main(new String[0]);
            stage = "stalled_tls";
            StalledTransportChecks.main(new String[0]);
            result.putString("request_probe", "passed");
            result.putString("stream", "Android request probe: cancellation, deadline, stalled TLS and worker cleanup passed\n");
        } catch (Exception | AssertionError failure) {
            result.putString("request_probe", "failed");
            result.putString("observation", stage);
            result.putString("stream", "Android request probe failed at " + stage + "\n");
        }
        return result;
    }
}
