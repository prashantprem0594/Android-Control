package com.appcontrol.parent

import android.Manifest
import android.app.NotificationManager
import android.app.Instrumentation
import android.app.job.JobInfo
import android.app.job.JobScheduler
import android.app.job.JobWorkItem
import android.content.ComponentName
import android.content.Context
import android.content.Intent
import android.content.pm.ApplicationInfo
import android.content.pm.PackageManager
import android.os.Build
import android.os.Bundle
import android.os.ParcelFileDescriptor
import com.appcontrol.parent.push.ParentPushWork
import com.appcontrol.parent.testing.ParentQueueProbeService
import java.util.UUID
import java.util.concurrent.CountDownLatch
import java.util.concurrent.TimeUnit

/** Real Android scheduling with the production drainer and debug-only I/O substitution. */
object ParentPushQueueProbe {
    @JvmStatic fun run(context: Context, instrumentation: Instrumentation): Bundle {
        val result = Bundle()
        val scheduler = context.getSystemService(JobScheduler::class.java)
        val session = context.getSharedPreferences("parent-session", Context.MODE_PRIVATE)
        val work = context.getSharedPreferences("parent-push-work", Context.MODE_PRIVATE)
        var phase = "guard"
        var baseline: Map<String, *>? = null
        var owned = false
        var observed: ParentQueueProbeService.State? = null
        val id = 5701
        fun await(latch: CountDownLatch, seconds: Long = 25) = check(latch.await(seconds, TimeUnit.SECONDS))
        fun removed() {
            val deadline = System.nanoTime() + TimeUnit.SECONDS.toNanos(10)
            while (scheduler.getPendingJob(id) != null && System.nanoTime() < deadline) Thread.sleep(50)
            check(scheduler.getPendingJob(id) == null)
        }
        fun prepare(state: ParentQueueProbeService.State) {
            observed = state
            check(scheduler.getPendingJob(id) == null)
            check(work.edit().clear().putString("generation", UUID.randomUUID().toString()).putBoolean("dirty", true).commit())
            ParentQueueProbeService.state = state
            val job = JobInfo.Builder(id, ComponentName(context, ParentQueueProbeService::class.java))
                .setOverrideDeadline(1_000).setBackoffCriteria(10_000, JobInfo.BACKOFF_POLICY_EXPONENTIAL).build()
            check(scheduler.enqueue(job, JobWorkItem(Intent("com.appcontrol.parent.FIXTURE_QUEUE"))) == JobScheduler.RESULT_SUCCESS)
        }
        try {
            check(BuildConfig.DEBUG && context.packageName == "com.appcontrol.parent.acceptance")
            check(context.applicationInfo.flags and ApplicationInfo.FLAG_DEBUGGABLE != 0)
            check(Build.FINGERPRINT.startsWith("google/sdk_gphone64_x86_64/emu64xa:"))
            check(context.checkSelfPermission(Manifest.permission.POST_NOTIFICATIONS) == PackageManager.PERMISSION_GRANTED)
            check(!ParentPushWork.enabled()) // The fixture must never obtain a Firebase token.
            check(ParentPushWork.notificationsAllowed(context))
            check(context.getSystemService(NotificationManager::class.java).activeNotifications.none {
                it.tag?.startsWith("parent-push-call:") == true
            }) // Production reset must not remove pre-existing alerts.
            check(session.all.isEmpty()) // Never replace a logged-in account or credentials.
            check(scheduler.getPendingJob(id) == null && scheduler.getPendingJob(5703) == null)
            baseline = work.all.toMap()
            owned = true
            check(session.edit().putString("origin", "https://queue-fixture.invalid").commit())

            phase = "late-hint-coalescing"
            val late = ParentQueueProbeService.State(blockFirst = true)
            prepare(late)
            await(late.entered)
            // The scheduler already owns one retained work item. A late burst marks dirty;
            // the production finishWork/beginSync loop must consume it in a second pass.
            repeat(50) { check(work.edit().putBoolean("dirty", true).commit()) }
            late.release.countDown()
            removed()
            check(late.starts.get() == 1 && late.passes.get() == 2 && late.failures.get() == 0)
            check(!work.getBoolean("dirty", true))

            phase = "framework-retry-redelivery"
            val retry = ParentQueueProbeService.State(retryFirst = true)
            prepare(retry)
            await(retry.entered)
            await(retry.exited)
            val naturalDeadline = System.nanoTime() + TimeUnit.SECONDS.toNanos(15)
            while (retry.passes.get() < 2 && System.nanoTime() < naturalDeadline) Thread.sleep(100)
            if (retry.passes.get() < 2) {
                // Android controls background timing. Force only an already-retained
                // retry, never enqueue replacement work or pretend to prove latency.
                check(scheduler.getPendingJob(id) != null)
                val output = ParcelFileDescriptor.AutoCloseInputStream(instrumentation.uiAutomation.executeShellCommand(
                    "cmd jobscheduler run -f com.appcontrol.parent.acceptance 5701"
                )).bufferedReader().use { it.readText() }
                check(output.contains("Running job"))
                result.putString("parent_push_queue_retry_mode", "shell-forced-retained-item")
            } else result.putString("parent_push_queue_retry_mode", "automatic")
            val deadline = System.nanoTime() + TimeUnit.SECONDS.toNanos(25)
            while (retry.passes.get() < 2 && System.nanoTime() < deadline) Thread.sleep(100)
            check(retry.starts.get() >= 2 && retry.passes.get() == 2 && retry.failures.get() == 0)
            removed()

            phase = "cancel-active-work"
            val cancel = ParentQueueProbeService.State(blockFirst = true)
            prepare(cancel)
            await(cancel.entered)
            scheduler.cancel(id)
            await(cancel.stopped, 10)
            await(cancel.exited, 10)
            removed()
            check(cancel.passes.get() == 1 && cancel.failures.get() == 0)

            phase = "generation-reset"
            val reset = ParentQueueProbeService.State(blockFirst = true)
            prepare(reset)
            await(reset.entered)
            ParentPushWork.stop(context)
            await(reset.stopped, 10)
            await(reset.exited, 10)
            removed()
            check(work.getBoolean("blocked", false) && reset.passes.get() == 1 && reset.failures.get() == 0)
            result.putString("parent_push_queue_probe", "passed")
            result.putString("stream", "Production parent drainer: late dirty burst, retained-item retry redelivery, cancellation and generation reset passed. Firebase/network I/O substituted; retry timing may be shell-forced; no latency/process-death/OEM claim.\n")
        } catch (_: Exception) {
            result.putString("parent_push_queue_probe", "failed")
            result.putString("observation", phase)
            observed?.let {
                result.putInt("fixture_starts", it.starts.get())
                result.putInt("fixture_passes", it.passes.get())
                result.putInt("fixture_failures", it.failures.get())
            }
            result.putBoolean("fixture_job_retained", scheduler.getPendingJob(id) != null)
        } finally {
            if (owned) try {
                scheduler.cancel(id)
                scheduler.cancel(5703)
                // Unblock process-local state through the same production reset as login;
                // polling builds cannot schedule provider work from newSession.
                check(session.edit().clear().commit())
                ParentPushWork.newSession(context)
                val restore = work.edit().clear()
                for ((key, value) in checkNotNull(baseline)) when (value) {
                    is String -> restore.putString(key, value)
                    is Boolean -> restore.putBoolean(key, value)
                    is Long -> restore.putLong(key, value)
                    is Int -> restore.putInt(key, value)
                    is Float -> restore.putFloat(key, value)
                    is Set<*> -> restore.putStringSet(key, value.filterIsInstance<String>().toSet())
                    else -> error("Unsupported fixture baseline")
                }
                check(restore.commit())
                check(session.all.isEmpty() && work.all == baseline)
                ParentQueueProbeService.state = null
                result.putString("parent_push_queue_cleanup", "passed")
            } catch (_: Exception) {
                result.putString("parent_push_queue_probe", "failed")
                result.putString("parent_push_queue_cleanup", "failed")
            }
        }
        return result
    }
}
