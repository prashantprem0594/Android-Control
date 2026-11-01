package com.appcontrol.parent.testing

import android.app.job.JobParameters
import android.os.Build
import com.appcontrol.parent.BuildConfig
import com.appcontrol.parent.push.ParentPushJobService
import java.util.concurrent.CountDownLatch
import java.util.concurrent.TimeUnit
import java.util.concurrent.atomic.AtomicInteger

/** Debug-only, credential-free I/O fixture. The inherited worker owns every queue operation. */
class ParentQueueProbeService : ParentPushJobService() {
    class State(val blockFirst: Boolean = false, val retryFirst: Boolean = false) {
        val entered = CountDownLatch(1)
        val release = CountDownLatch(1)
        val exited = CountDownLatch(1)
        val stopped = CountDownLatch(1)
        val starts = AtomicInteger()
        val passes = AtomicInteger()
        val failures = AtomicInteger()
    }
    companion object { @Volatile var state: State? = null }
    private var active: State? = null
    override fun onStartJob(params: JobParameters): Boolean {
        check(BuildConfig.DEBUG && packageName == "com.appcontrol.parent.acceptance" &&
            Build.FINGERPRINT.startsWith("google/sdk_gphone64_x86_64/emu64xa:"))
        active = checkNotNull(state)
        active!!.starts.incrementAndGet()
        return super.onStartJob(params)
    }
    override fun synchronizeWork(generation: String, stopped: () -> Boolean): Boolean {
        val fixture = checkNotNull(active)
        val pass = fixture.passes.incrementAndGet()
        try {
            if (pass == 1) {
                fixture.entered.countDown()
                if (fixture.blockFirst) {
                    val deadline = System.nanoTime() + TimeUnit.SECONDS.toNanos(20)
                    while (!fixture.release.await(50, TimeUnit.MILLISECONDS)) {
                        if (stopped()) return false
                        check(System.nanoTime() < deadline)
                    }
                }
            }
            return fixture.retryFirst && pass == 1
        } catch (_: InterruptedException) {
            Thread.currentThread().interrupt()
            return false
        } catch (failure: Exception) {
            fixture.failures.incrementAndGet()
            throw failure
        } finally { fixture.exited.countDown() }
    }
    override fun onStopJob(params: JobParameters): Boolean {
        active?.stopped?.countDown()
        return super.onStopJob(params)
    }
}
