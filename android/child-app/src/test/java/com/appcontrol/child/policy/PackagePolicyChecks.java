package com.appcontrol.child.policy;

import java.util.ArrayList;
import java.util.Arrays;
import java.util.Collections;
import java.util.HashSet;
import java.util.List;
import java.util.Set;

/** Dependency-free invariants for policy changes and interrupted OS operations. */
public final class PackagePolicyChecks {
    private static final class Device implements PackagePolicyEngine.Platform {
        final Set<String> installed = new HashSet<>(Arrays.asList("org.example.first", "org.example.second", "org.example.critical", "org.example.unmanaged"));
        final Set<String> protectedNames = new HashSet<>(Collections.singleton("org.example.critical"));
        final Set<String> suspended = new HashSet<>();
        Set<String> ledger = new HashSet<>();
        final List<String> changes = new ArrayList<>();
        boolean failSave;
        boolean failRestore;
        boolean failSuspend;
        String protectedOnRecheck;
        int rechecks;
        boolean crashAfterSuspend;
        public boolean installed(String name) { return installed.contains(name); }
        public boolean protectedPackage(String name) {
            if (name.equals(protectedOnRecheck) && ++rechecks >= 2) protectedNames.add(name);
            return protectedNames.contains(name);
        }
        public Set<String> tracked() { return new HashSet<>(ledger); }
        public boolean saveTracked(Set<String> names) { if (failSave) return false; ledger = new HashSet<>(names); return true; }
        public boolean setSuspended(String name, boolean value) {
            if (value && !ledger.contains(name)) throw new AssertionError("OS restriction before durable ledger");
            if (value && protectedNames.contains(name)) throw new AssertionError("critical package suspended");
            changes.add(name + ":" + value);
            if (!value && failRestore) return false;
            if (value && failSuspend) return false;
            if (value) suspended.add(name); else suspended.remove(name);
            if (value && crashAfterSuspend) throw new AssertionError("simulated process death");
            return true;
        }
        public boolean isSuspended(String name) { return suspended.contains(name); }
    }
    private static void require(boolean value, String message) { if (!value) throw new AssertionError(message); }
    public static void main(String[] args) {
        Device device = new Device();
        device.suspended.add("org.example.unmanaged");
        PackagePolicyEngine engine = new PackagePolicyEngine(device);
        require(engine.reconcile(Collections.singleton("org.example.first")).isEmpty(), "initial block failed");
        require(device.suspended.contains("org.example.first"), "requested package not suspended");
        int steadyChanges = device.changes.size();
        require(engine.reconcile(Collections.singleton("org.example.first")).isEmpty() && device.changes.size() == steadyChanges,
            "unchanged policy repeated OS changes and could loop package broadcasts");
        require(engine.reconcile(Collections.singleton("org.example.second")).isEmpty(), "offline replacement failed");
        require(!device.suspended.contains("org.example.first") && device.suspended.contains("org.example.second"), "old restriction survived latest policy");
        require(device.suspended.contains("org.example.unmanaged"), "unowned restriction changed");
        device.protectedNames.add("org.example.second");
        require(engine.reconcile(Collections.singleton("org.example.second")).equals("protected_package"), "newly critical role accepted");
        require(!device.suspended.contains("org.example.second") && device.ledger.isEmpty(), "newly protected package not restored");
        require(engine.reconcile(Arrays.asList("org.example.critical", "org.example.first")).equals("protected_package"), "critical request reported applied");
        require(!device.suspended.contains("org.example.critical"), "critical package blocked");

        device.failRestore = true;
        int before = device.changes.size();
        require(engine.reconcile(Collections.singleton("org.example.second")).equals("package_change_failed"), "failed restore reported applied");
        require(device.changes.subList(before, device.changes.size()).stream().noneMatch(s -> s.endsWith(":true")), "new restriction added while restoration failed");
        device.failRestore = false;
        require(engine.reconcile(Collections.emptyList()).isEmpty(), "release failed");

        device.failSave = true;
        before = device.changes.size();
        try { engine.reconcile(Collections.singleton("org.example.first")); throw new AssertionError("ledger failure ignored"); }
        catch (IllegalStateException expected) { }
        require(device.changes.size() == before, "OS changed despite reservation write failure");
        device.failSave = false;
        device.crashAfterSuspend = true;
        try { engine.reconcile(Collections.singleton("org.example.first")); throw new IllegalStateException("expected crash"); }
        catch (AssertionError expected) { require(expected.getMessage().equals("simulated process death"), "unexpected invariant failure"); }
        device.crashAfterSuspend = false;
        engine = new PackagePolicyEngine(device);
        require(engine.reconcile(Collections.emptyList()).isEmpty(), "reboot recovery failed");
        require(!device.suspended.contains("org.example.first") && device.ledger.isEmpty(), "interrupted restriction stranded");
        require(engine.reconcile(Collections.singleton("org.example.missing")).equals("package_unavailable"), "missing package reported applied");
        Device retryDevice = new Device();
        retryDevice.failSuspend = true;
        retryDevice.protectedOnRecheck = "org.example.second";
        PackagePolicyEngine retryEngine = new PackagePolicyEngine(retryDevice);
        String reason = retryEngine.reconcile(Arrays.asList("org.example.first", "org.example.second"));
        require(reason.equals("package_change_failed") && PolicyRetry.required("partial", reason),
            "later role change hid a transient failure or prevented retry");
        require(retryDevice.ledger.contains("org.example.first") && !retryDevice.suspended.contains("org.example.first"),
            "failed suspension lost its recovery reservation");
        retryDevice.failSuspend = false;
        reason = retryEngine.reconcile(Collections.singleton("org.example.first"));
        require(reason.isEmpty() && !PolicyRetry.required("applied", reason) && retryDevice.suspended.contains("org.example.first"),
            "retry did not converge or continued after success");
        reason = retryEngine.reconcile(Collections.singleton("org.example.critical"));
        require(reason.equals("protected_package") && !PolicyRetry.required("partial", reason),
            "permanent protected target entered transient retry loop");
        System.out.println("package-policy-checks-passed: critical roles, latest policy, owned restrictions, restore failure, reservation failure, crash recovery, missing packages, transient retry convergence, reason precedence");
    }
}
