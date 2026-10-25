package com.appcontrol.child.commands;

import java.io.IOException;
import java.util.ArrayDeque;
import java.util.ArrayList;
import java.util.Deque;
import java.util.List;

public final class SyncWorkQueueChecks {
    private static final class Queue implements SyncWorkQueue.Queue<Integer> {
        final Deque<Integer> pending = new ArrayDeque<>();
        final List<Integer> executing = new ArrayList<>();
        final List<Integer> completed = new ArrayList<>();
        boolean appendOnComplete;
        public Integer next() {
            Integer item = pending.poll();
            if (item != null) executing.add(item);
            return item;
        }
        public void complete(Integer item) {
            if (!executing.remove(item)) throw new AssertionError("completed work was not dequeued");
            completed.add(item);
            if (appendOnComplete) { appendOnComplete = false; pending.add(99); }
        }
        void restart() { pending.addAll(executing); executing.clear(); }
    }
    private static void require(boolean condition, String message) {
        if (!condition) throw new AssertionError(message);
    }
    public static void main(String[] args) throws Exception {
        Queue queue = new Queue();
        queue.pending.add(1); queue.pending.add(2);
        int[] passes = {0};
        require(!SyncWorkQueue.drain(queue, () -> {
            if (++passes[0] == 1) queue.pending.add(3);
            return false;
        }, () -> false), "successful drain requested retry");
        require(passes[0] == 2 && queue.completed.size() == 3 && queue.executing.isEmpty(), "request during sync was lost");
        queue.pending.add(4); queue.appendOnComplete = true;
        SyncWorkQueue.drain(queue, () -> { passes[0]++; return false; }, () -> false);
        require(queue.completed.contains(99) && passes[0] == 4, "request during completion was lost");

        Queue retry = new Queue(); retry.pending.add(1);
        require(SyncWorkQueue.drain(retry, () -> true, () -> false), "partial policy did not retain retry");
        require(retry.completed.isEmpty() && retry.executing.size() == 1, "partial result completed queued work");
        retry.restart();
        SyncWorkQueue.drain(retry, () -> false, () -> false);
        require(retry.completed.size() == 1 && retry.executing.isEmpty(), "retry did not complete retained work");

        Queue failed = new Queue(); failed.pending.add(1);
        try {
            SyncWorkQueue.drain(failed, () -> { throw new IOException("Synthetic outage"); }, () -> false);
            throw new AssertionError("transport failure reported successful work");
        } catch (IOException expected) { }
        require(failed.completed.isEmpty() && failed.executing.size() == 1, "failed work was completed");
        failed.restart();
        boolean[] stopped = {false};
        try {
            SyncWorkQueue.drain(failed, () -> { stopped[0] = true; return false; }, () -> stopped[0]);
            throw new AssertionError("stopped work was completed");
        } catch (InterruptedException expected) { }
        require(failed.completed.isEmpty(), "cancelled work was completed");

        Queue burst = new Queue();
        for (int item = 0; item < 125; item++) burst.pending.add(item);
        int[] batches = {0};
        SyncWorkQueue.drain(burst, () -> { batches[0]++; return false; }, () -> false);
        require(batches[0] == 3 && burst.completed.size() == 125, "work batching lost or repeated requests");
        System.out.println("sync-work-queue-checks-passed: late requests, completion race, partial retry, outage retention, cancellation and bounded batches");
    }
}
