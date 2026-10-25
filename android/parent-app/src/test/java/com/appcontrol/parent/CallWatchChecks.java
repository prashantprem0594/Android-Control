package com.appcontrol.parent;

import java.time.Instant;
import java.util.*;

public final class CallWatchChecks {
    private static void check(boolean value) { if (!value) throw new AssertionError("Call-watch check failed"); }
    private static void refuses(Runnable action) {
        try { action.run(); } catch (IllegalArgumentException | IllegalStateException expected) { return; }
        throw new AssertionError("Unsafe report accepted");
    }
    public static void main(String[] args) {
        long start = 1_000;
        Instant now = Instant.parse("2026-10-24T12:00:00Z");
        String id = "4b73cb84-b9a9-47c6-b03b-80e3a93aa57d";
        CallWatchState watch = new CallWatchState(start);
        var first = watch.accept(10,now.minusSeconds(1),now,Map.of(id,"RINGING"),start,start+100);
        check(first.ringing().contains(id) && first.firstSeen().contains(id));
        check(first.validUntil() == start+29_000);
        var repeat = watch.accept(10,now.minusSeconds(1),now,Map.of(id,"RINGING"),start+100,start+200);
        check(repeat.ringing().contains(id) && repeat.firstSeen().isEmpty());
        check(watch.accept(9,now,now,Map.of(id,"RINGING"),start+200,start+300).ringing().isEmpty());
        check(watch.accept(11,now.minusSeconds(30),now,Map.of(id,"RINGING"),start+300,start+301).ringing().isEmpty());
        check(watch.accept(11,now.plusSeconds(1),now,Map.of(id,"RINGING"),start+300,start+400).ringing().isEmpty());
        check(watch.accept(11,null,now,Map.of(),start+300,start+400).ringing().isEmpty());
        check(watch.accept(11,now,now,Map.of(id,"ACTIVE"),start+300,start+400).ringing().isEmpty());
        var recovered = watch.accept(12,now,now,Map.of(id,"RINGING"),start+400,start+500);
        check(recovered.ringing().contains(id) && recovered.firstSeen().isEmpty());
        refuses(() -> watch.accept(13,now,now,Map.of("not-a-call","RINGING"),start+500,start+600));
        refuses(() -> watch.accept(13,now,now,Map.of(id,"INVALID"),start+500,start+600));
        refuses(() -> watch.accept(13,now,now,Map.of(id,"RINGING"),start+700,start+600));
        check(!watch.expired(start+599_999));
        check(watch.expired(start+600_000) && watch.expired(start-1));
        refuses(() -> watch.accept(13,now,now,Map.of(id,"RINGING"),start+500,start+600_000));
        CallWatchState limited = new CallWatchState(start);
        for (int index=1;index<=128;index++) limited.accept(index,now,now,Map.of(UUID.randomUUID().toString(),"RINGING"),start,start+index);
        refuses(() -> limited.accept(129,now,now,Map.of(UUID.randomUUID().toString(),"RINGING"),start,start+129));
        System.out.println("call-watch-valid freshness/order/duplicates/exact-id/deadline/capacity");
    }
}
