package com.appcontrol.child.calls;

/** Unknown call classification must never become remote-action permission. */
public final class EmergencyCallGuard {
    public interface Classifier { boolean isEmergency(String address) throws Exception; }
    private EmergencyCallGuard() {}
    public static boolean isProtected(boolean platformEmergency, String scheme, String address, Classifier classifier) {
        if (platformEmergency || !"tel".equals(scheme) || address == null || address.trim().isEmpty() || classifier == null) return true;
        try { return classifier.isEmergency(address); }
        catch (Exception failure) { return true; }
    }
}
