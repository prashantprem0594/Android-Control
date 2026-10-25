package com.appcontrol.parent;

import java.time.Duration;
import java.time.Instant;

/** A recent report is an observation, not proof of a persistent connection. */
public final class ContactFreshness {
    public enum State { UNKNOWN, RECENT, STALE, CLOCK_CHANGED }
    private ContactFreshness() {}
    public static State classify(Instant receivedAt, Instant serverTime) {
        if (receivedAt == null) return State.UNKNOWN;
        if (serverTime == null) throw new IllegalArgumentException("Server time required");
        if (receivedAt.isAfter(serverTime.plusSeconds(60))) return State.CLOCK_CHANGED;
        return Duration.between(receivedAt, serverTime).compareTo(Duration.ofMinutes(20)) >= 0 ? State.STALE : State.RECENT;
    }
}
