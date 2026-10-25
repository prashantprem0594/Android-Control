package com.appcontrol.child.sync

import android.app.job.JobParameters
import android.app.job.JobService
import android.os.Handler
import android.os.Looper
import com.appcontrol.child.policy.PolicyEnforcer
import com.appcontrol.child.policy.PolicyRetry
import java.util.concurrent.atomic.AtomicBoolean

/** Offline fallback even when network-constrained synchronization cannot run. */
class PolicyJobService : JobService() {
    private var stopped = AtomicBoolean()
    override fun onStartJob(params: JobParameters): Boolean {
        val cancelled = AtomicBoolean()
        stopped = cancelled
        Thread {
            val result = runCatching {
                if (cancelled.get()) null else PolicyEnforcer(applicationContext).applyCached()
            }
            Handler(Looper.getMainLooper()).post {
                val retry = result.isFailure || result.getOrNull()?.let { PolicyRetry.required(it.result, it.reason) } == true
                if (!cancelled.get()) jobFinished(params, retry)
            }
        }.start()
        return true
    }
    override fun onStopJob(params: JobParameters): Boolean { stopped.set(true); return true }
}
