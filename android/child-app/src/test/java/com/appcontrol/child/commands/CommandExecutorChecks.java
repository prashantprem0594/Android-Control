package com.appcontrol.child.commands;

import java.time.Instant;
import java.util.HashMap;
import java.util.Map;

/** Dependency-free JVM checks; run through scripts/test-command-journal.ps1. */
public final class CommandExecutorChecks {
    private static final Instant NOW = Instant.parse("2026-10-23T12:00:00Z");
    private static final Instant DEADLINE = NOW.plusSeconds(60);

    private static final class MemoryJournal implements CommandExecutor.Journal {
        final Map<String, String> values = new HashMap<>();
        String failResult;
        public String read(String id) { return values.get(id); }
        public boolean write(String id, String result, Instant deadline) {
            if (result.equals(failResult)) return false;
            values.put(id, result);
            return true;
        }
    }

    private static void require(boolean condition, String message) {
        if (!condition) throw new AssertionError(message);
    }

    public static void main(String[] args) {
        MemoryJournal journal = new MemoryJournal();
        int[] executions = {0};
        CommandExecutor executor = new CommandExecutor(journal, () -> NOW);
        require(executor.execute("duplicate", DEADLINE, () -> executions[0]++).equals("succeeded"), "initial result");
        // Reconstruct executor to simulate retry after process restart and lost ACK.
        executor = new CommandExecutor(journal, () -> NOW);
        require(executor.execute("duplicate", DEADLINE, () -> executions[0]++).equals("succeeded"), "retry result");
        require(executions[0] == 1, "duplicate action executed twice");

        journal.values.put("interrupted", "started");
        require(executor.execute("interrupted", DEADLINE, () -> executions[0]++).equals("failed"), "interrupted result");
        require(executions[0] == 1, "interrupted action repeated");

        journal.failResult = "started";
        try {
            executor.execute("no-reservation", DEADLINE, () -> executions[0]++);
            throw new AssertionError("reservation failure ignored");
        } catch (IllegalStateException expected) { }
        require(executions[0] == 1, "action ran without durable reservation");

        journal.failResult = "succeeded";
        try {
            executor.execute("lost-result", DEADLINE, () -> executions[0]++);
            throw new AssertionError("result failure ignored");
        } catch (IllegalStateException expected) { }
        require(executions[0] == 2, "action did not run once");
        journal.failResult = null;
        executor = new CommandExecutor(journal, () -> NOW);
        require(executor.execute("lost-result", DEADLINE, () -> executions[0]++).equals("failed"), "uncertain outcome not failed");
        require(executions[0] == 2, "action repeated after result write failure");

        require(executor.execute("expired", NOW, () -> executions[0]++).equals("failed"), "expired result");
        require(executions[0] == 2, "expired command executed");
        require(executor.execute("action-failure", DEADLINE, () -> { throw new SecurityException(); }).equals("failed"), "action error not recorded");
        require(executor.execute("action-failure", DEADLINE, () -> executions[0]++).equals("failed"), "failed action retried");
        require(executions[0] == 2, "failed command executed again");
        System.out.println("command-journal-checks-passed: duplicate, restart, interrupted, storage failure, expiry, action failure");
    }
}
