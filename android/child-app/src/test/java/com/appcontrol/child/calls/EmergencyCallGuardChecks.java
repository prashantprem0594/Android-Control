package com.appcontrol.child.calls;

public final class EmergencyCallGuardChecks {
    private static void require(boolean value) { if (!value) throw new AssertionError("Emergency call protection invariant failed"); }
    public static void main(String[] args) {
        EmergencyCallGuard.Classifier ordinary = address -> false;
        require(EmergencyCallGuard.isProtected(false, null, null, ordinary));
        require(EmergencyCallGuard.isProtected(false, "tel", null, ordinary));
        require(EmergencyCallGuard.isProtected(false, "tel", "", ordinary));
        require(EmergencyCallGuard.isProtected(false, "tel", "  ", ordinary));
        require(EmergencyCallGuard.isProtected(false, "sip", "fixture.invalid", ordinary));
        require(EmergencyCallGuard.isProtected(false, "tel", "fixture", null));
        require(EmergencyCallGuard.isProtected(true, "tel", "fixture", ordinary));
        require(EmergencyCallGuard.isProtected(false, "tel", "fixture", address -> true));
        require(EmergencyCallGuard.isProtected(false, "tel", "fixture", address -> { throw new SecurityException("fixture"); }));
        require(!EmergencyCallGuard.isProtected(false, "tel", "fixture", ordinary));
        final int[] operations = {0};
        CallActionGate.Calls unknown = new CallActionGate.Calls() {
            public CallActionGate.Snapshot lookup(String id) { return new CallActionGate.Snapshot(id, CallActionGate.State.RINGING, EmergencyCallGuard.isProtected(false, null, null, ordinary)); }
            public void perform(String id, CallActionGate.Action action) { operations[0]++; }
        };
        java.time.Instant now = java.time.Instant.parse("2026-10-24T12:00:00Z");
        require(CallActionGate.request(unknown, "fixture", CallActionGate.Action.ANSWER, now.plusSeconds(5), now, true) == CallActionGate.Result.EMERGENCY_PROTECTED);
        require(operations[0] == 0);
        require(CallActionGate.request(unknown, "fixture", CallActionGate.Action.ANSWER, now.plusSeconds(5), now, false) == CallActionGate.Result.REQUESTED);
        require(operations[0] == 1);
        System.out.println("Emergency classification checks passed: missing/non-telephone/failed classification protected remotely; local user control retained.");
    }
}
