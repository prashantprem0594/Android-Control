package com.appcontrol.child.policy;

import java.util.Collection;
import java.util.LinkedHashSet;
import java.util.Set;
import java.util.regex.Pattern;

/** Reconciles only restrictions owned by this app; ledger writes precede suspension. */
public final class PackagePolicyEngine {
    private static final Pattern PACKAGE = Pattern.compile("[A-Za-z_][A-Za-z0-9_]*(\\.[A-Za-z_][A-Za-z0-9_]*)+");

    public interface Platform {
        boolean installed(String name);
        boolean protectedPackage(String name);
        Set<String> tracked();
        boolean saveTracked(Set<String> packages);
        boolean setSuspended(String name, boolean suspended) throws Exception;
        boolean isSuspended(String name) throws Exception;
    }

    private final Platform platform;
    public PackagePolicyEngine(Platform platform) { this.platform = platform; }

    /** Empty reason means all package restrictions match the desired state. */
    public String reconcile(Collection<String> requested) {
        if (requested.size() > 100) throw new IllegalArgumentException("Too many packages");
        Set<String> desired = new LinkedHashSet<>();
        for (String name : requested) {
            if (name == null || name.length() > 255 || !PACKAGE.matcher(name).matches() || !desired.add(name)) {
                throw new IllegalArgumentException("Invalid package list");
            }
        }
        String reason = "";
        Set<String> eligible = new LinkedHashSet<>();
        for (String name : desired) {
            if (!platform.installed(name)) { if (reason.isEmpty()) reason = "package_unavailable"; }
            else if (platform.protectedPackage(name)) reason = "protected_package";
            else eligible.add(name);
        }
        Set<String> tracked = new LinkedHashSet<>(platform.tracked());
        boolean restoreFailed = false;
        // Restore removed or newly protected targets before adding restrictions.
        for (String name : new LinkedHashSet<>(tracked)) {
            if (eligible.contains(name)) continue;
            try {
                if (!platform.installed(name) || (platform.setSuspended(name, false) && !platform.isSuspended(name))) {
                    tracked.remove(name);
                    save(tracked);
                } else restoreFailed = true;
            } catch (Exception failure) { restoreFailed = true; }
        }
        if (restoreFailed) return "package_change_failed";
        for (String name : eligible) {
            // Revalidate immediately before changing the OS; roles may have changed.
            if (platform.protectedPackage(name)) {
                if (tracked.contains(name)) {
                    try {
                        if (!platform.setSuspended(name, false) || platform.isSuspended(name)) return "package_change_failed";
                        tracked.remove(name); save(tracked);
                    } catch (Exception failure) { return "package_change_failed"; }
                }
                if (!"package_change_failed".equals(reason)) reason = "protected_package";
                continue;
            }
            boolean previouslyTracked = tracked.contains(name);
            if (tracked.add(name)) save(tracked); // Must be durable before any suspension.
            try {
                if (!previouslyTracked || !platform.isSuspended(name)) {
                    if (!platform.setSuspended(name, true) || !platform.isSuspended(name)) reason = "package_change_failed";
                }
            } catch (Exception failure) { reason = "package_change_failed"; }
        }
        return reason;
    }

    private void save(Set<String> tracked) {
        if (!platform.saveTracked(new LinkedHashSet<>(tracked))) {
            throw new IllegalStateException("Could not persist package reconciliation ledger");
        }
    }
}
