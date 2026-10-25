package com.appcontrol.parent;

import java.time.Instant;

public final class ContactFreshnessChecks {
    private static void require(boolean value) { if (!value) throw new AssertionError("Contact freshness invariant failed"); }
    public static void main(String[] args) {
        Instant server = Instant.parse("2026-10-24T12:00:00Z");
        require(ContactFreshness.classify(null, server) == ContactFreshness.State.UNKNOWN);
        require(ContactFreshness.classify(server, server) == ContactFreshness.State.RECENT);
        require(ContactFreshness.classify(server.minusSeconds(1199), server) == ContactFreshness.State.RECENT);
        require(ContactFreshness.classify(server.minusSeconds(1200), server) == ContactFreshness.State.STALE);
        require(ContactFreshness.classify(server.minusSeconds(86400), server) == ContactFreshness.State.STALE);
        require(ContactFreshness.classify(server.plusSeconds(60), server) == ContactFreshness.State.RECENT);
        require(ContactFreshness.classify(server.plusSeconds(61), server) == ContactFreshness.State.CLOCK_CHANGED);
        System.out.println("Contact freshness checks passed: unknown, recent, delayed/offline boundary and server clock rollback.");
    }
}
