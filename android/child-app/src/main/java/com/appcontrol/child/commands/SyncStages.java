package com.appcontrol.child.commands;

import java.util.function.BooleanSupplier;

/** Keep command transport failures from starving desired-policy recovery. */
public final class SyncStages {
    private SyncStages() {}
    public interface Stage { void run() throws Exception; }

    public static void run(Stage commands, Stage policy, BooleanSupplier stopped) throws Exception {
        requireRunning(stopped);
        Exception commandFailure = null;
        try {
            commands.run();
        } catch (Exception failure) {
            commandFailure = failure;
        }
        requireRunning(stopped);
        try {
            policy.run();
        } catch (Exception failure) {
            if (commandFailure != null && commandFailure != failure) failure.addSuppressed(commandFailure);
            throw failure;
        }
        requireRunning(stopped);
        // A successful policy request does not confirm command delivery or ACK.
        // Preserve the failure so the background job schedules another attempt.
        if (commandFailure != null) throw commandFailure;
    }

    private static void requireRunning(BooleanSupplier stopped) throws InterruptedException {
        if (stopped.getAsBoolean()) throw new InterruptedException("Sync stopped");
    }
}
