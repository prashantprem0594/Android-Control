package com.appcontrol.child.sync

import android.app.job.JobInfo
import android.app.job.JobScheduler
import android.app.job.JobWorkItem
import android.content.ComponentName
import android.content.Context
import android.content.Intent

object SyncScheduler {
    private const val JOB_ID = 4101
    private const val POLICY_JOB_ID = 4102
    internal const val REQUEST_JOB_ID = 4103
    @Synchronized
    fun ensureScheduled(context: Context) {
        val app = context.applicationContext
        if (app.getSharedPreferences("enrollment", Context.MODE_PRIVATE).getString("deviceId", null) == null) return
        val scheduler = app.getSystemService(JobScheduler::class.java)
        if (scheduler.getPendingJob(JOB_ID) == null) check(scheduler.schedule(JobInfo.Builder(JOB_ID, ComponentName(app, SyncJobService::class.java))
            .setRequiredNetworkType(JobInfo.NETWORK_TYPE_ANY)
            .setPeriodic(15 * 60 * 1000L)
            .setBackoffCriteria(30_000L, JobInfo.BACKOFF_POLICY_EXPONENTIAL)
            .setPersisted(true).build()) == JobScheduler.RESULT_SUCCESS) { "Could not schedule sync" }
        if (scheduler.getPendingJob(POLICY_JOB_ID) == null) check(scheduler.schedule(JobInfo.Builder(POLICY_JOB_ID, ComponentName(app, PolicyJobService::class.java))
            .setPeriodic(15 * 60 * 1000L).setBackoffCriteria(30_000L, JobInfo.BACKOFF_POLICY_EXPONENTIAL)
            .setPersisted(true).build()) == JobScheduler.RESULT_SUCCESS) { "Could not schedule offline policy" }
        // Boot, app update, clock changes and foreground startup should request
        // convergence without waiting for the periodic polling interval.
        requestSync(app)
    }

    @Synchronized
    fun requestSync(context: Context) {
        val app = context.applicationContext
        if (app.getSharedPreferences("enrollment", Context.MODE_PRIVATE).getString("deviceId", null) == null) return
        val scheduler = app.getSystemService(JobScheduler::class.java)
        // Use identical JobInfo for every enqueue. Work arriving during a sync
        // remains queued without replacing/stopping the running job.
        val job = JobInfo.Builder(REQUEST_JOB_ID, ComponentName(app, SyncJobService::class.java))
            .setRequiredNetworkType(JobInfo.NETWORK_TYPE_ANY)
            .setBackoffCriteria(30_000L, JobInfo.BACKOFF_POLICY_EXPONENTIAL).build()
        check(scheduler.enqueue(job, JobWorkItem(Intent("com.appcontrol.child.SYNC"))) == JobScheduler.RESULT_SUCCESS) {
            "Could not schedule requested sync"
        }
    }
}
