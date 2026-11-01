package com.appcontrol.parent.push;

import java.util.Map;

/** A bounded durable-ledger policy. Capacity exhaustion suppresses alerts rather than dropping deduplication. */
public final class ParentPushLedger {
    public static final int CAPACITY = 512;
    public static final long RETENTION_MILLIS = 7 * 24 * 60 * 60 * 1000L;
    private ParentPushLedger() {}
    public static boolean mark(Map<String, Long> entries, String digest, long now) {
        if (entries == null || entries.size() > CAPACITY || digest == null ||
            !digest.matches("[a-f0-9]{64}") || now < 0) throw new IllegalArgumentException("Invalid ledger");
        for (var entry : entries.entrySet()) if (!entry.getKey().matches("[a-f0-9]{64}") ||
            entry.getValue() == null || entry.getValue() < 0) throw new IllegalArgumentException("Invalid ledger entry");
        // A backwards clock retains prior identities; it must not re-enable old alerts.
        entries.entrySet().removeIf(entry -> now >= entry.getValue() && now - entry.getValue() >= RETENTION_MILLIS);
        if (entries.containsKey(digest) || entries.size() >= CAPACITY) return false;
        entries.put(digest, now);
        return true;
    }
}
