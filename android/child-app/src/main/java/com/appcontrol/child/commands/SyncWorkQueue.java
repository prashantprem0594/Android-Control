package com.appcontrol.child.commands;

import java.util.ArrayList;
import java.util.List;
import java.util.function.BooleanSupplier;

/** Batch opaque sync requests; finish each only after a successful sync. */
public final class SyncWorkQueue {
    private SyncWorkQueue() {}
    public interface Queue<T> {
        T next();
        void complete(T work);
    }
    public interface Sync { boolean retryRequired() throws Exception; }

    public static <T> boolean drain(Queue<T> queue, Sync sync, BooleanSupplier stopped) throws Exception {
        for (;;) {
            checkRunning(stopped);
            List<T> batch = new ArrayList<>();
            for (int count = 0; count < 50; count++) {
                checkRunning(stopped);
                T item = queue.next();
                if (item == null) break;
                batch.add(item);
            }
            if (batch.isEmpty()) return false;
            checkRunning(stopped);
            if (sync.retryRequired()) return true;
            for (T item : batch) {
                checkRunning(stopped);
                queue.complete(item);
            }
            // Always dequeue again. Work may have arrived during sync or
            // completion; the platform owns the empty-queue completion race.
        }
    }

    private static void checkRunning(BooleanSupplier stopped) throws InterruptedException {
        if (stopped.getAsBoolean()) throw new InterruptedException("Sync stopped");
    }
}
