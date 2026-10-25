package com.appcontrol.child.testing;

import android.app.job.JobParameters;
import android.app.job.JobService;
import android.app.job.JobWorkItem;
import android.os.Handler;
import android.os.Looper;
import com.appcontrol.child.commands.SyncWorkQueue;
import java.util.concurrent.ConcurrentHashMap;
import java.util.concurrent.CountDownLatch;
import java.util.concurrent.TimeUnit;
import java.util.concurrent.atomic.AtomicBoolean;
import java.util.concurrent.atomic.AtomicInteger;

/** Debug-only framework fixture. Never authenticates or invokes device policy. */
public final class QueueProbeService extends JobService {
    public static final class State {
        public final AtomicInteger starts = new AtomicInteger();
        public final AtomicInteger passes = new AtomicInteger();
        public final AtomicInteger completed = new AtomicInteger();
        public final AtomicInteger deliveries = new AtomicInteger();
        public final CountDownLatch entered = new CountDownLatch(1);
        public final CountDownLatch release = new CountDownLatch(1);
        public final CountDownLatch done = new CountDownLatch(1);
        public final CountDownLatch retried = new CountDownLatch(1);
        public final CountDownLatch stopped = new CountDownLatch(1);
        public final CountDownLatch exited = new CountDownLatch(1);
        public final boolean holdFirst;
        public final boolean failFirst;
        public volatile boolean unexpectedFailure;
        public State(boolean holdFirst, boolean failFirst) { this.holdFirst = holdFirst; this.failFirst = failFirst; }
    }
    public static final ConcurrentHashMap<Integer, State> states = new ConcurrentHashMap<>();
    private final ConcurrentHashMap<Integer, AtomicBoolean> cancellations = new ConcurrentHashMap<>();

    @Override public boolean onStartJob(JobParameters params) {
        State state = states.get(params.getJobId());
        if (state == null) return false;
        AtomicBoolean cancelled = new AtomicBoolean();
        cancellations.put(params.getJobId(), cancelled);
        state.starts.incrementAndGet();
        new Thread(() -> {
            boolean retry = true;
            try {
                retry = SyncWorkQueue.drain(new SyncWorkQueue.Queue<JobWorkItem>() {
                    public JobWorkItem next() {
                        JobWorkItem item = params.dequeueWork();
                        if (item != null) state.deliveries.accumulateAndGet(item.getDeliveryCount(), Math::max);
                        return item;
                    }
                    public void complete(JobWorkItem item) { params.completeWork(item); state.completed.incrementAndGet(); }
                }, () -> {
                    int pass = state.passes.incrementAndGet();
                    if (pass == 1) {
                        state.entered.countDown();
                        if (state.holdFirst) {
                            long deadline = System.nanoTime() + TimeUnit.SECONDS.toNanos(20);
                            while (!state.release.await(50, TimeUnit.MILLISECONDS)) {
                                if (cancelled.get()) throw new InterruptedException();
                                if (System.nanoTime() >= deadline) throw new IllegalStateException("Fixture release timed out");
                            }
                        }
                        if (state.failFirst) return true;
                    }
                    return false;
                }, cancelled::get);
                if (!retry) state.done.countDown();
            } catch (Exception failure) {
                if (!cancelled.get()) state.unexpectedFailure = true;
            } finally {
                boolean needsRetry = retry;
                new Handler(Looper.getMainLooper()).post(() -> {
                    if (!cancelled.get() && needsRetry) {
                        jobFinished(params, true);
                        state.retried.countDown();
                    }
                    cancellations.remove(params.getJobId(), cancelled);
                });
                state.exited.countDown();
            }
        }, "app-control-queue-probe").start();
        return true;
    }
    @Override public boolean onStopJob(JobParameters params) {
        AtomicBoolean cancelled = cancellations.remove(params.getJobId());
        if (cancelled != null) cancelled.set(true);
        State state = states.get(params.getJobId());
        if (state != null) state.stopped.countDown();
        return true;
    }
}
