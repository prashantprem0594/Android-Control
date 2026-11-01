package com.appcontrol.parent.testing

import android.app.job.JobParameters
import android.content.Context
import android.os.Build
import android.os.Process
import com.appcontrol.parent.BuildConfig
import com.appcontrol.parent.push.ParentPushJobService
import com.appcontrol.parent.push.ParentPushWork

/** Separate-process debug fixture; only I/O is replaced, retained work stays with the production drainer. */
class ParentQueueDeathProbeService : ParentPushJobService() {
    override fun onStartJob(params: JobParameters): Boolean {
        check(BuildConfig.DEBUG && packageName == "com.appcontrol.parent.acceptance")
        check(Build.FINGERPRINT.startsWith("google/sdk_gphone64_x86_64/emu64xa:") && !ParentPushWork.enabled())
        ParentQueueDeathState.update(this) { it.put("starts", it.getInt("starts") + 1) }
        return super.onStartJob(params)
    }
    override fun synchronizeWork(generation: String, stopped: () -> Boolean): Boolean {
        val pid = Process.myPid()
        var first = false
        ParentQueueDeathState.update(this) { state ->
            check(state.getString("generation") == generation)
            state.put("passes", state.getInt("passes") + 1)
            val dirty = getSharedPreferences("parent-push-work", Context.MODE_PRIVATE).getBoolean("dirty", true)
            if (state.getInt("firstPid") == 0) {
                first = true
                state.put("firstPid", pid).put("dirtyBeforeFirst", dirty).put("phase", "holding")
            } else {
                check(state.getInt("firstPid") != pid && !state.getBoolean("firstExited"))
                state.put("replayPid", pid).put("dirtyBeforeReplay", dirty)
                    .put("generationMatches", true).put("replayed", true).put("phase", "replayed")
            }
        }
        if (!first) return false
        try {
            while (!stopped()) Thread.sleep(50)
        } catch (_: InterruptedException) {
            Thread.currentThread().interrupt()
        } finally {
            // SIGKILL cannot run this block. A graceful timeout/stop disqualifies the proof.
            ParentQueueDeathState.update(this) { it.put("firstExited", true).put("phase", "graceful-exit") }
        }
        return true
    }
    override fun onStopJob(params: JobParameters): Boolean {
        runCatching { ParentQueueDeathState.update(this) { it.put("stopCount", it.getInt("stopCount") + 1) } }
        return super.onStopJob(params)
    }
}
