package com.appcontrol.parent.push

import android.app.job.JobParameters
import android.app.job.JobService
import android.app.job.JobWorkItem
import android.os.Handler
import android.os.Looper
import android.os.SystemClock
import com.appcontrol.parent.ParentClient
import java.util.concurrent.ArrayBlockingQueue
import java.util.concurrent.Future
import java.util.concurrent.ThreadPoolExecutor
import java.util.concurrent.TimeUnit
import java.util.concurrent.atomic.AtomicBoolean

/** One bounded worker per fixed component, at most two total; no job replaces the other's work. */
open class ParentPushJobService : JobService() {
    /** Debug fixture subclasses replace only I/O; queue ownership and lifecycle stay here. */
    protected open fun synchronizeWork(generation: String, stopped: () -> Boolean): Boolean =
        ParentPushWork.synchronize(applicationContext, generation, stopped)
    private val worker = ThreadPoolExecutor(1, 1, 0, TimeUnit.MILLISECONDS, ArrayBlockingQueue(1))
    private var cancelled = AtomicBoolean(true)
    private var pending: Future<*>? = null
    override fun onStartJob(params: JobParameters): Boolean {
        cancelled.set(true)
        pending?.cancel(true)
        worker.purge()
        val stopped = AtomicBoolean(false)
        cancelled = stopped
        val generation = ParentPushWork.generation(applicationContext) ?: return false
        if (params.jobId != ParentPushWork.JOB_ID && params.jobId != ParentPushWork.EXPEDITED_JOB_ID) return false
        val began = SystemClock.elapsedRealtime()
        fun shouldStop() = stopped.get() || Thread.currentThread().isInterrupted || SystemClock.elapsedRealtime() - began >= 60_000
        pending = try { worker.submit {
            var retry = false
            var queueFinished = false
            var item: JobWorkItem? = null
            var retainedWork = true
            try {
                while (!shouldStop() && ParentPushWork.current(applicationContext, generation)) {
                    if (item == null) {
                        item = ParentPushWork.takeWork(applicationContext, generation, params)
                            ?: run { queueFinished = true; return@submit }
                        retainedWork = true
                    }
                    if (!ParentPushQueueState.canAttempt(checkNotNull(item).deliveryCount)) {
                        val done = ParentPushWork.finishWork(applicationContext, generation, params, checkNotNull(item), true)
                        item = done.next
                        retainedWork = true
                        if (item == null) { queueFinished = true; return@submit }
                        continue
                    }
                    if (!ParentPushWork.beginSync(applicationContext, generation, retainedWork)) {
                        val done = ParentPushWork.finishWork(applicationContext, generation, params, checkNotNull(item))
                        if (done.completed) { item = done.next; retainedWork = true }
                        if (done.completed && item == null) { queueFinished = true; return@submit }
                        continue
                    }
                    retainedWork = false
                    val outcome = runCatching { synchronizeWork(generation, ::shouldStop) }
                    val failure = outcome.exceptionOrNull()
                    val transient = failure != null && failure !is ParentClient.SignInRequired &&
                        !(failure is ParentClient.RequestFailure && failure.status in setOf(400, 401, 403, 404))
                    val needsRetry = outcome.getOrDefault(false) || transient || shouldStop()
                    if (needsRetry && !stopped.get() && ParentPushWork.current(applicationContext, generation)) {
                        if (checkNotNull(item).deliveryCount < ParentPushQueueState.MAX_DELIVERIES) { retry = true; break }
                    }
                    if (stopped.get()) return@submit
                    val done = ParentPushWork.finishWork(applicationContext, generation, params,
                        checkNotNull(item), needsRetry)
                    if (done.completed) { item = done.next; retainedWork = true }
                    if (done.completed && item == null) { queueFinished = true; return@submit }
                }
                if (!stopped.get() && ParentPushWork.current(applicationContext, generation)) {
                    if (item != null && checkNotNull(item).deliveryCount >= ParentPushQueueState.MAX_DELIVERIES) {
                        val done = ParentPushWork.finishWork(applicationContext, generation, params, checkNotNull(item), true)
                        queueFinished = done.completed && done.next == null
                    }
                    retry = !queueFinished
                }
            } catch (_: Exception) {
                retry = !stopped.get() && ParentPushWork.current(applicationContext, generation) &&
                    (item == null || checkNotNull(item).deliveryCount < ParentPushQueueState.MAX_DELIVERIES)
            }
            finally {
                // A successful queue ends by dequeueWork returning null, not jobFinished(false),
                // so work arriving at completion cannot be discarded.
                if (!stopped.get() && !queueFinished) Handler(Looper.getMainLooper()).post { if (!stopped.get()) jobFinished(params, retry) }
            }
        } } catch (_: java.util.concurrent.RejectedExecutionException) { return false }
        return true
    }
    override fun onStopJob(params: JobParameters): Boolean {
        cancelled.set(true)
        pending?.cancel(true)
        val generation = ParentPushWork.generation(applicationContext) ?: return false
        return ParentPushWork.current(applicationContext, generation)
    }
    override fun onDestroy() {
        cancelled.set(true)
        pending?.cancel(true)
        worker.shutdownNow()
        super.onDestroy()
    }
}

/** Separate Android component gives expedited work independent bounded lifecycle state. */
class ParentPushExpeditedJobService : ParentPushJobService()
