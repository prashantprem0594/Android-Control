package com.appcontrol.parent;

import java.time.Duration;
import java.time.Instant;
import java.util.*;

/** Bounded monotonic watch; stale reports and old snapshots cannot create alerts. */
public final class CallWatchState {
    public static final long WINDOW_MILLIS = 600_000;
    private final long started;
    private long sequence;
    private final Set<String> seen = new HashSet<>();
    public CallWatchState(long started) {
        if (started < 0) throw new IllegalArgumentException("Invalid monotonic time");
        this.started = started;
    }
    public boolean expired(long now) { return now < started || now - started >= WINDOW_MILLIS; }
    public static final class Decision {
        private final Set<String> ringing, firstSeen;
        private final long validUntil;
        private Decision(Set<String> ringing, Set<String> firstSeen, long validUntil) { this.ringing = ringing; this.firstSeen = firstSeen; this.validUntil = validUntil; }
        public Set<String> ringing() { return ringing; }
        public Set<String> firstSeen() { return firstSeen; }
        public long validUntil() { return validUntil; }
    }
    public Decision accept(long incomingSequence, Instant observed, Instant serverTime,
                           Map<String,String> calls, long requestStarted, long now) {
        if (expired(now)) throw new IllegalStateException("Watch ended");
        if (requestStarted < started || requestStarted > now || calls == null || calls.size() > 8) throw new IllegalArgumentException("Invalid report");
        Set<String> states = Set.of("RINGING","ACTIVE","HOLDING","DIALING","CONNECTING","SELECTING_ACCOUNT","DISCONNECTING","DISCONNECTED","OTHER");
        for (var entry : calls.entrySet()) {
            UUID id = UUID.fromString(entry.getKey());
            if (!id.toString().equals(entry.getKey()) || id.version() != 4 || id.variant() != 2 || !states.contains(entry.getValue())) throw new IllegalArgumentException("Invalid call identity/state");
        }
        if (observed == null || serverTime == null || incomingSequence < 1 || incomingSequence < sequence) return new Decision(Set.of(), Set.of(),now);
        Duration age = Duration.between(observed, serverTime);
        if (age.isNegative() || age.plusMillis(now-requestStarted).compareTo(Duration.ofSeconds(30)) > 0) return new Decision(Set.of(), Set.of(),now);
        sequence = incomingSequence;
        Set<String> ringing = new HashSet<>(), first = new HashSet<>();
        for (var entry : calls.entrySet()) if (entry.getValue().equals("RINGING")) {
            ringing.add(entry.getKey());
            if (!seen.contains(entry.getKey())) first.add(entry.getKey());
        }
        if (seen.size() + first.size() > 128) throw new IllegalStateException("Watch capacity reached");
        seen.addAll(first);
        long remaining = Duration.ofSeconds(30).minus(age).toMillis() - (now-requestStarted);
        return new Decision(Set.copyOf(ringing), Set.copyOf(first),now+remaining);
    }
}
