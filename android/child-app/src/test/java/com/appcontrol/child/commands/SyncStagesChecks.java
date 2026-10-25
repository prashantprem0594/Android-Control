package com.appcontrol.child.commands;

import java.io.IOException;
import java.time.Instant;
import java.util.HashMap;
import java.util.Map;

/** Reconnection checks using the same coordinator and journal as Android. */
public final class SyncStagesChecks {
    private static void require(boolean condition, String message) {
        if (!condition) throw new AssertionError(message);
    }

    public static void main(String[] args) throws Exception {
        Map<String, String> results = new HashMap<>();
        CommandExecutor.Journal journal = new CommandExecutor.Journal() {
            public String read(String id) { return results.get(id); }
            public boolean write(String id, String value, Instant deadline) { results.put(id, value); return true; }
        };
        Instant now = Instant.parse("2026-10-24T12:00:00Z");
        int[] actions = {0}, policies = {0}, acknowledgements = {0};
        IOException lostAck = new IOException("Synthetic disconnected acknowledgement");
        try {
            SyncStages.run(() -> {
                new CommandExecutor(journal, () -> now).execute("lost-ack", now.plusSeconds(60), () -> actions[0]++);
                throw lostAck;
            }, () -> policies[0]++, () -> false);
            throw new AssertionError("command failure reported success");
        } catch (IOException expected) { require(expected == lostAck, "failure identity changed"); }
        require(actions[0] == 1 && policies[0] == 1, "lost ACK prevented policy recovery");
        SyncStages.run(() -> {
            require(new CommandExecutor(journal, () -> now).execute("lost-ack", now.plusSeconds(60), () -> actions[0]++).equals("succeeded"), "saved result lost");
            acknowledgements[0]++;
        }, () -> policies[0]++, () -> false);
        require(actions[0] == 1 && acknowledgements[0] == 1 && policies[0] == 2, "reconnect repeated action or lost policy/ACK");

        IOException fetchFailure = new IOException("Synthetic fetch outage");
        try {
            SyncStages.run(() -> { throw fetchFailure; }, () -> policies[0]++, () -> false);
            throw new AssertionError("fetch failure reported success");
        } catch (IOException expected) { require(expected == fetchFailure, "fetch failure lost"); }
        require(policies[0] == 3, "fetch outage starved policy");
        IOException policyFailure = new IOException("Synthetic policy outage");
        try {
            SyncStages.run(() -> { throw fetchFailure; }, () -> { throw policyFailure; }, () -> false);
            throw new AssertionError("combined outage reported success");
        } catch (IOException expected) {
            require(expected == policyFailure && expected.getSuppressed().length == 1 && expected.getSuppressed()[0] == fetchFailure, "combined failure lost");
        }
        boolean[] stopped = {true};
        try {
            SyncStages.run(() -> actions[0]++, () -> policies[0]++, () -> stopped[0]);
            throw new AssertionError("stopped sync ran");
        } catch (InterruptedException expected) { }
        stopped[0] = false;
        try {
            SyncStages.run(() -> { stopped[0] = true; throw fetchFailure; }, () -> policies[0]++, () -> stopped[0]);
            throw new AssertionError("cancelled sync continued");
        } catch (InterruptedException expected) { }
        require(actions[0] == 1 && policies[0] == 3, "cancellation allowed work");
        System.out.println("sync-recovery-checks-passed: lost ACK, no repeated action, independent policy, outage, combined failure, cancellation");
    }
}
