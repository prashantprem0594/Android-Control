package com.appcontrol.child.commands;

import java.time.Instant;
import java.util.function.Supplier;

/** Durable reservation prevents repeating an action across retries or process death. */
public final class CommandExecutor {
    public interface Journal {
        String read(String id);
        boolean write(String id, String result, Instant deadline);
    }
    public interface Action { void run() throws Exception; }

    private final Journal journal;
    private final Supplier<Instant> now;

    public CommandExecutor(Journal journal, Supplier<Instant> now) {
        this.journal = journal;
        this.now = now;
    }

    public String execute(String id, Instant deadline, Action action) {
        String previous = journal.read(id);
        if ("succeeded".equals(previous) || "failed".equals(previous)) return previous;
        String result;
        if ("started".equals(previous)) {
            result = "failed"; // Interrupted outcome is unconfirmed; never repeat it.
        } else if (previous == null) {
            save(id, "started", deadline);
            result = "failed";
            if (now.get().isBefore(deadline)) {
                try {
                    action.run();
                    result = "succeeded";
                } catch (Exception failure) {
                    result = "failed";
                }
            }
        } else {
            throw new IllegalStateException("Invalid command journal");
        }
        save(id, result, deadline);
        return result;
    }

    private void save(String id, String result, Instant deadline) {
        if (!journal.write(id, result, deadline)) {
            throw new IllegalStateException("Could not persist command result");
        }
    }
}
