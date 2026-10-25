package com.appcontrol.child.policy;

/** Retry transient OS changes; setup and invalid targets need external changes. */
public final class PolicyRetry {
    private PolicyRetry() {}
    public static boolean required(String result, String reason) {
        return ("partial".equals(result) || "failed".equals(result)) && "package_change_failed".equals(reason);
    }
}
