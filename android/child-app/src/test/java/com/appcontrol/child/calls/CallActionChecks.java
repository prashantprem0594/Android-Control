package com.appcontrol.child.calls;

import java.time.Instant;
import java.util.HashMap;
import java.util.Map;

public final class CallActionChecks {
    private static final Instant NOW = Instant.parse("2026-10-23T12:00:00Z");
    private static final class Calls implements CallActionGate.Calls {
        final Map<String, CallActionGate.Snapshot> live = new HashMap<>();
        String lastTarget;
        CallActionGate.Action lastAction;
        int effects;
        boolean fail;
        boolean retarget;
        public CallActionGate.Snapshot lookup(String id) { return retarget ? live.get("call-b") : live.get(id); }
        public void perform(String id, CallActionGate.Action action) throws Exception {
            if (fail) throw new Exception("private diagnostic");
            effects++; lastTarget = id; lastAction = action;
        }
        void state(String id, CallActionGate.State state, boolean emergency) { live.put(id, new CallActionGate.Snapshot(id, state, emergency)); }
    }
    private static void require(boolean value, String message) { if (!value) throw new AssertionError(message); }
    private static CallActionGate.Result request(Calls calls, String id, CallActionGate.Action action, boolean remote) {
        return CallActionGate.request(calls, id, action, NOW.plusSeconds(15), NOW, remote);
    }
    public static void main(String[] args) {
        Calls calls = new Calls();
        calls.state("call-a", CallActionGate.State.RINGING, false);
        calls.state("call-b", CallActionGate.State.ACTIVE, false);
        require(request(calls, "call-a", CallActionGate.Action.ANSWER, true) == CallActionGate.Result.REQUESTED, "answer request rejected");
        require(calls.effects == 1 && calls.lastTarget.equals("call-a"), "answer affected wrong call");
        require(calls.live.get("call-a").state == CallActionGate.State.RINGING, "request falsely implied observed answer");
        require(request(calls, "call-b", CallActionGate.Action.ANSWER, true) == CallActionGate.Result.WRONG_STATE, "active call answered");
        require(request(calls, "call-a", CallActionGate.Action.END, true) == CallActionGate.Result.WRONG_STATE, "ringing call ended as active");
        require(request(calls, "call-a", CallActionGate.Action.REJECT, true) == CallActionGate.Result.REQUESTED && calls.lastAction == CallActionGate.Action.REJECT, "ringing reject failed");
        calls.live.remove("call-a");
        int before = calls.effects;
        require(request(calls, "call-a", CallActionGate.Action.END, true) == CallActionGate.Result.NOT_FOUND && calls.effects == before, "ended identity retargeted current call");
        calls.retarget = true;
        require(request(calls, "call-a", CallActionGate.Action.END, true) == CallActionGate.Result.NOT_FOUND, "lookup changed requested call identity");
        calls.retarget = false;
        require(CallActionGate.request(calls, "call-b", CallActionGate.Action.END, NOW, NOW, true) == CallActionGate.Result.EXPIRED, "exact deadline accepted");
        require(CallActionGate.request(calls, "call-b", CallActionGate.Action.END, NOW.plusSeconds(31), NOW, true) == CallActionGate.Result.INVALID_DEADLINE, "long-lived call action accepted");
        calls.state("call-b", CallActionGate.State.ACTIVE, true);
        require(request(calls, "call-b", CallActionGate.Action.END, true) == CallActionGate.Result.EMERGENCY_PROTECTED, "remote emergency action accepted");
        require(request(calls, "call-b", CallActionGate.Action.END, false) == CallActionGate.Result.REQUESTED, "user could not end their call");
        calls.state("call-b", CallActionGate.State.DIALING, false);
        require(request(calls, "call-b", CallActionGate.Action.END, true) == CallActionGate.Result.WRONG_STATE, "remote dialing action accepted");
        require(request(calls, "call-b", CallActionGate.Action.END, false) == CallActionGate.Result.REQUESTED, "user could not cancel dialing");
        calls.state("call-b", CallActionGate.State.SELECTING_ACCOUNT, false);
        require(request(calls, "call-b", CallActionGate.Action.END, false) == CallActionGate.Result.REQUESTED, "user could not cancel account selection");
        calls.state("call-b", CallActionGate.State.DISCONNECTED, false);
        require(request(calls, "call-b", CallActionGate.Action.END, true) == CallActionGate.Result.WRONG_STATE, "disconnected call affected");
        calls.state("call-b", CallActionGate.State.ACTIVE, false); calls.fail = true;
        require(request(calls, "call-b", CallActionGate.Action.END, true) == CallActionGate.Result.FAILED, "Telecom failure reported accepted");
        EmergencyCallGuard.Classifier ordinary = address -> false;
        require(RemoteCallPermission.allowed(true, false, "tel", "5550100", ordinary), "classified ordinary call disabled");
        require(!RemoteCallPermission.allowed(false, false, "tel", "5550100", ordinary), "build-disabled remote call allowed");
        require(!RemoteCallPermission.allowed(true, true, "tel", "5550100", ordinary), "platform emergency allowed");
        require(!RemoteCallPermission.allowed(true, false, null, "5550100", ordinary), "unclassified scheme allowed");
        require(!RemoteCallPermission.allowed(true, false, "tel", null, ordinary), "missing address allowed");
        require(!RemoteCallPermission.allowed(true, false, "tel", "5550100", address -> { throw new Exception(); }), "failed emergency classification allowed");
        require(!CallActionGate.observed(CallActionGate.Action.ANSWER, CallActionGate.State.RINGING, true), "answer request counted as observed success");
        require(CallActionGate.observed(CallActionGate.Action.ANSWER, CallActionGate.State.ACTIVE, true), "active answer result not observed");
        require(!CallActionGate.observed(CallActionGate.Action.REJECT, CallActionGate.State.RINGING, true), "ringing reject request counted as success");
        require(CallActionGate.observed(CallActionGate.Action.REJECT, CallActionGate.State.DISCONNECTED, true), "rejected disconnect not observed");
        require(CallActionGate.observed(CallActionGate.Action.REJECT, CallActionGate.State.DISCONNECTED, false), "removed rejected call not observed");
        require(!CallActionGate.observed(CallActionGate.Action.END, CallActionGate.State.ACTIVE, true), "end request counted as observed success");
        require(!CallActionGate.observed(CallActionGate.Action.END, CallActionGate.State.DISCONNECTING, true), "disconnecting counted as complete");
        require(CallActionGate.observed(CallActionGate.Action.END, CallActionGate.State.DISCONNECTED, true), "ended call result not observed");
        require(!CallActionGate.observed(null, CallActionGate.State.DISCONNECTED, false), "unknown action counted as success");
        System.out.println("call-action-checks-passed: exact identity, stale/missing/disconnected calls, state/deadline validation, emergency protection, local control and request/outcome separation");
    }
}
