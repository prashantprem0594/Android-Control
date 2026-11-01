package com.appcontrol.parent.push;

import java.time.Instant;
import java.util.HashMap;
import java.util.Map;

public final class ParentPushChecks {
    private static final String DEVICE = "dev_AAAAAAAAAAAAAAAAAAAAAA";
    private static final String CALL = "12345678-1234-4234-8234-123456789abc";
    private static void require(boolean value) { if (!value) throw new AssertionError("Parent push invariant failed"); }
    private static void rejects(Runnable action) {
        try { action.run(); } catch (IllegalArgumentException expected) { return; }
        throw new AssertionError("Invalid push data accepted");
    }
    public static void main(String[] ignored) {
        Instant now = Instant.parse("2026-10-26T00:00:00Z");
        var event = new ParentPushEventRules.Event(DEVICE+":"+CALL, DEVICE, CALL, 7, now.minusSeconds(3), now.plusSeconds(20));
        require(ParentPushEventRules.remainingMillis(event, now, 100, now.plusSeconds(1), now, 8,
            Map.of(CALL,"RINGING"), 200, 400) == 19_700);
        require(ParentPushEventRules.remainingMillis(event, now, 100, now.plusSeconds(1), now, 6,
            Map.of(CALL,"RINGING"), 200, 400) == 0);
        require(ParentPushEventRules.remainingMillis(event, now, 100, now.plusSeconds(1), now, 8,
            Map.of(CALL,"ACTIVE"), 200, 400) == 0);
        require(ParentPushEventRules.remainingMillis(event, now, 100, now.minusSeconds(1), now.minusSeconds(2), 8,
            Map.of(CALL,"RINGING"), 200, 400) == 0);
        require(ParentPushEventRules.remainingMillis(event, now, 100, now.plusSeconds(1), now.minusSeconds(30), 8,
            Map.of(CALL,"RINGING"), 200, 400) == 0);
        require(ParentPushEventRules.remainingMillis(event, now, 100, now.plusSeconds(1), now, 8,
            Map.of(CALL,"RINGING"), 200, 20_100) == 0);
        require(ParentPushEventRules.remainingMillis(event, now, 100, now.plusSeconds(1), now, 8,
            Map.of(CALL,"RINGING"), 200, 199) == 0);
        rejects(() -> new ParentPushEventRules.Event("event",DEVICE,CALL,0,now,now.plusSeconds(1)));
        rejects(() -> new ParentPushEventRules.Event("event",DEVICE,"bad",1,now,now.plusSeconds(1)));
        rejects(() -> new ParentPushEventRules.Event("event",DEVICE,CALL,1,now,now.plusSeconds(31)));
        rejects(() -> new ParentPushEventRules.Event("bad\nevent",DEVICE,CALL,1,now,now.plusSeconds(1)));
        var ledger = new HashMap<String,Long>();
        String digest = ParentPushEventRules.fingerprint("opaque-event");
        require(digest.matches("[a-f0-9]{64}"));
        require(ParentPushLedger.mark(ledger,digest,1_000));
        var reloaded = new HashMap<>(ledger);
        require(!ParentPushLedger.mark(reloaded,digest,1_100));
        require(!ParentPushLedger.mark(reloaded,digest,500));
        require(ParentPushLedger.mark(reloaded,digest,1_000+ParentPushLedger.RETENTION_MILLIS));
        ledger.clear();
        for (int i=0;i<ParentPushLedger.CAPACITY;i++) require(ParentPushLedger.mark(ledger,ParentPushEventRules.fingerprint("event-"+i),1_000));
        require(!ParentPushLedger.mark(ledger,digest,1_100));
        require(ledger.size()==ParentPushLedger.CAPACITY);
        System.out.println("parent-push-checks-passed: exact identity, state, sequence, freshness, elapsed budget, rollback, durable dedupe, bounded capacity");
    }
}
