package com.appcontrol.child.sync

import android.app.job.JobParameters
import android.app.job.JobService
import android.app.job.JobWorkItem
import android.os.Handler
import android.os.Looper
import com.appcontrol.child.commands.CommandSync
import com.appcontrol.child.commands.SyncWorkQueue
import com.appcontrol.child.policy.PolicyEnforcer
import java.util.concurrent.atomic.AtomicBoolean
import java.util.concurrent.ConcurrentHashMap

class SyncJobService : JobService() {
    private val stopped = ConcurrentHashMap<Int, AtomicBoolean>()

    override fun onStartJob(params: JobParameters): Boolean {
        val cancelled = AtomicBoolean(false)
        stopped[params.jobId] = cancelled
        Thread {
            // Cached app restrictions reconcile even if authentication/network later fails.
            runCatching { if (!cancelled.get()) PolicyEnforcer(applicationContext).applyCached() }
            val queued = params.jobId == SyncScheduler.REQUEST_JOB_ID
            val result = runCatching {
                if (queued) {
                    SyncWorkQueue.drain(object : SyncWorkQueue.Queue<JobWorkItem> {
                        override fun next(): JobWorkItem? = params.dequeueWork()
                        override fun complete(work: JobWorkItem) = params.completeWork(work)
                    }, { CommandSync(applicationContext).sync { cancelled.get() }.retryRequired }, { cancelled.get() })
                } else CommandSync(applicationContext).sync { cancelled.get() }.retryRequired
            }
            Handler(Looper.getMainLooper()).post {
                val retry = result.isFailure || result.getOrDefault(true)
                // Successful queued work ends through dequeueWork(), never an
                // explicit successful jobFinished that could discard late work.
                // Failures retain uncompleted work and request backoff instead.
                if (!cancelled.get() && (!queued || retry)) jobFinished(params, retry)
                stopped.remove(params.jobId, cancelled)
            }
        }.start()
        return true
    }

    override fun onStopJob(params: JobParameters): Boolean {
        stopped.remove(params.jobId)?.set(true)
        return true
    }
}
