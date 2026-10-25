package com.appcontrol.child.calls;

import java.time.Duration;
import java.time.Instant;

/** A request targets one live call. REQUESTED is not an observed call outcome. */
public final class CallActionGate {
    public enum State { RINGING, ACTIVE, HOLDING, DIALING, CONNECTING, SELECTING_ACCOUNT, DISCONNECTING, DISCONNECTED, OTHER }
    public enum Action { ANSWER, REJECT, END }
    public enum Result { REQUESTED, EXPIRED, INVALID_DEADLINE, NOT_FOUND, WRONG_STATE, EMERGENCY_PROTECTED, FAILED }
    public static final class Snapshot {
        public final String id;
        public final State state;
        public final boolean emergency;
        public Snapshot(String id, State state, boolean emergency) { this.id = id; this.state = state; this.emergency = emergency; }
    }
    public interface Calls {
        Snapshot lookup(String id);
        void perform(String id, Action action) throws Exception;
    }
    public static boolean observed(Action action, State state, boolean present) {
        if (action == null) return false;
        if (action == Action.ANSWER) return present && state == State.ACTIVE;
        return !present || state == State.DISCONNECTED;
    }
    public static Result request(Calls calls, String id, Action action, Instant expires, Instant now, boolean remote) {
        if (id == null || id.isEmpty() || action == null || expires == null || now == null) return Result.INVALID_DEADLINE;
        if (!now.isBefore(expires)) return Result.EXPIRED;
        if (Duration.between(now, expires).compareTo(Duration.ofSeconds(30)) > 0) return Result.INVALID_DEADLINE;
        Snapshot target = calls.lookup(id);
        if (target == null || !id.equals(target.id)) return Result.NOT_FOUND;
        if (remote && target.emergency) return Result.EMERGENCY_PROTECTED;
        boolean valid = action == Action.ANSWER || action == Action.REJECT ? target.state == State.RINGING :
            target.state == State.ACTIVE || target.state == State.HOLDING || !remote && (target.state == State.DIALING || target.state == State.CONNECTING || target.state == State.SELECTING_ACCOUNT);
        if (!valid) return Result.WRONG_STATE;
        try { calls.perform(id, action); return Result.REQUESTED; }
        catch (Exception failure) { return Result.FAILED; }
    }
}
