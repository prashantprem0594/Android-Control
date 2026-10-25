package com.appcontrol.child.calls;

/** Combines the build-time rollout gate with fail-closed call classification. */
public final class RemoteCallPermission {
    private RemoteCallPermission() {}

    public static boolean allowed(boolean featureEnabled, boolean platformEmergency,
                                  String scheme, String address, EmergencyCallGuard.Classifier classifier) {
        return featureEnabled && !EmergencyCallGuard.isProtected(platformEmergency, scheme, address, classifier);
    }
}
