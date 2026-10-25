package com.appcontrol.child;

import android.app.job.JobInfo;
import android.app.job.JobScheduler;
import android.app.job.JobWorkItem;
import android.content.ComponentName;
import android.content.Context;
import android.content.Intent;
import android.os.Bundle;
import com.appcontrol.child.testing.QueueProbeService;
import java.util.concurrent.CountDownLatch;
import java.util.concurrent.TimeUnit;

/** Real JobScheduler lifecycle using a debug-only service and production drainer. */
public final class WorkQueueProbe {
    private static void require(boolean condition) { if (!condition) throw new IllegalStateException("Queue invariant failed"); }
    private static void await(CountDownLatch latch, int seconds) throws Exception { require(latch.await(seconds, TimeUnit.SECONDS)); }
    private static JobInfo job(Context context, int id) {
        return new JobInfo.Builder(id, new ComponentName(context, QueueProbeService.class))
            .setOverrideDeadline(1000).setBackoffCriteria(10_000, JobInfo.BACKOFF_POLICY_EXPONENTIAL).build();
    }
    private static void enqueue(JobScheduler scheduler, JobInfo job) {
        require(scheduler.enqueue(job, new JobWorkItem(new Intent("com.appcontrol.child.TEST_SYNC"))) == JobScheduler.RESULT_SUCCESS);
    }
    private static void waitRemoved(JobScheduler scheduler, int id) throws Exception {
        long deadline = System.nanoTime() + TimeUnit.SECONDS.toNanos(5);
        while (scheduler.getPendingJob(id) != null && System.nanoTime() < deadline) Thread.sleep(50);
        require(scheduler.getPendingJob(id) == null);
    }
    public static Bundle run(Context context) {
        Bundle result = new Bundle();
        JobScheduler scheduler = context.getSystemService(JobScheduler.class);
        String stage = "setup";
        try {
            stage = "late_request";
            QueueProbeService.State late = new QueueProbeService.State(true, false);
            QueueProbeService.states.put(4901, late);
            JobInfo first = job(context, 4901);
            enqueue(scheduler, first);
            await(late.entered, 20);
            enqueue(scheduler, first);
            late.release.countDown();
            await(late.done, 20);
            require(!late.unexpectedFailure && late.starts.get() == 1 && late.passes.get() == 2 && late.completed.get() == 2);
            waitRemoved(scheduler, 4901);

            stage = "backoff_redelivery";
            QueueProbeService.State retry = new QueueProbeService.State(false, true);
            QueueProbeService.states.put(4902, retry);
            enqueue(scheduler, job(context, 4902));
            await(retry.retried, 20);
            require(retry.completed.get() == 0);
            await(retry.done, 45);
            require(!retry.unexpectedFailure && retry.starts.get() >= 2 && retry.deliveries.get() >= 2 && retry.completed.get() == 1);
            waitRemoved(scheduler, 4902);

            stage = "cancellation";
            QueueProbeService.State cancelled = new QueueProbeService.State(true, false);
            QueueProbeService.states.put(4903, cancelled);
            enqueue(scheduler, job(context, 4903));
            await(cancelled.entered, 20);
            scheduler.cancel(4903);
            await(cancelled.stopped, 10);
            await(cancelled.exited, 10);
            require(cancelled.completed.get() == 0);
            waitRemoved(scheduler, 4903);
            result.putString("queue_probe", "passed");
            result.putString("stream", "Android queue probe: late enqueue, retry redelivery and cancellation passed\n");
        } catch (Exception failure) {
            result.putString("queue_probe", "failed");
            result.putString("observation", stage);
            result.putString("stream", "Android queue probe failed at " + stage + "\n");
        } finally {
            for (int id = 4901; id <= 4903; id++) scheduler.cancel(id);
            QueueProbeService.states.clear();
        }
        return result;
    }
}
