package com.appcontrol.parent

import android.app.ActivityManager
import android.app.Instrumentation
import android.app.NotificationManager
import android.app.job.JobInfo
import android.app.job.JobScheduler
import android.app.job.JobWorkItem
import android.content.ComponentName
import android.content.Context
import android.content.Intent
import android.content.SharedPreferences
import android.content.pm.ApplicationInfo
import android.os.Build
import android.os.Bundle
import android.util.Xml
import com.appcontrol.parent.push.ParentPushWork
import com.appcontrol.parent.testing.ParentQueueDeathProbeService
import com.appcontrol.parent.testing.ParentQueueDeathState
import org.json.JSONArray
import org.json.JSONObject
import org.xmlpull.v1.XmlPullParser
import java.io.File
import java.util.UUID
import java.util.concurrent.TimeUnit

/** One live instrumentation session spans the external SIGKILL; finishing it kills package processes. */
object ParentPushQueueDeathProbe {
    private const val ID = 5701
    private const val ORIGIN = "https://queue-death-fixture.invalid"
    private fun snapshot(values: Map<String, *>): JSONObject = JSONObject().also { result ->
        for ((key, value) in values) result.put(key, JSONObject().apply {
            when (value) {
                is String -> put("type", "string").put("value", value)
                is Boolean -> put("type", "boolean").put("value", value)
                is Long -> put("type", "long").put("value", value)
                is Int -> put("type", "int").put("value", value)
                is Float -> put("type", "float").put("value", value.toDouble())
                is Set<*> -> put("type", "set").put("value", JSONArray(value.toList()))
                else -> error("Unsupported fixture baseline")
            }
        })
    }
    private fun restore(prefs: SharedPreferences, values: JSONObject) {
        val edit = prefs.edit().clear()
        val expected = mutableMapOf<String, Any>()
        for (key in values.keys()) {
            val value = values.getJSONObject(key)
            when (value.getString("type")) {
                "string" -> value.getString("value").also { expected[key] = it; edit.putString(key, it) }
                "boolean" -> value.getBoolean("value").also { expected[key] = it; edit.putBoolean(key, it) }
                "long" -> value.getLong("value").also { expected[key] = it; edit.putLong(key, it) }
                "int" -> value.getInt("value").also { expected[key] = it; edit.putInt(key, it) }
                "float" -> value.getDouble("value").toFloat().also { expected[key] = it; edit.putFloat(key, it) }
                "set" -> value.getJSONArray("value").let { array ->
                    val items = (0 until array.length()).map { array.getString(it) }.toSet()
                    expected[key] = items
                    edit.putStringSet(key, items)
                }
                else -> error("Invalid fixture baseline")
            }
        }
        check(edit.commit())
        check(prefs.all == expected)
    }
    /** Cross-process mutations must be checked on disk, never from a cached SharedPreferences map. */
    private fun persistedWork(context: Context): Pair<String?, Boolean?> {
        var generation: String? = null
        var dirty: Boolean? = null
        File(context.applicationInfo.dataDir, "shared_prefs/parent-push-work.xml").inputStream().use { input ->
            val parser = Xml.newPullParser().apply { setInput(input, "UTF-8") }
            while (parser.next() != XmlPullParser.END_DOCUMENT) {
                if (parser.eventType != XmlPullParser.START_TAG) continue
                if (parser.name == "string" && parser.getAttributeValue(null, "name") == "generation") generation = parser.nextText()
                if (parser.name == "boolean" && parser.getAttributeValue(null, "name") == "dirty") dirty = parser.getAttributeValue(null, "value")?.let {
                    check(it == "true" || it == "false"); it == "true"
                }
            }
        }
        return generation to dirty
    }
    private fun jobAbsent(scheduler: JobScheduler) = scheduler.getPendingJob(ID) == null && scheduler.getPendingJob(5703) == null
    @JvmStatic fun run(context: Context, phase: String, instrumentation: Instrumentation): Bundle {
        val result = Bundle()
        val scheduler = context.getSystemService(JobScheduler::class.java)
        val session = context.getSharedPreferences("parent-session", Context.MODE_PRIVATE)
        val work = context.getSharedPreferences("parent-push-work", Context.MODE_PRIVATE)
        fun verifyReplay() {
            val state = ParentQueueDeathState.read(context)
            check(state.getBoolean("replayed") && !state.getBoolean("firstExited"))
            check(state.getInt("firstPid") > 1 && state.getInt("replayPid") > 1 && state.getInt("firstPid") != state.getInt("replayPid"))
            check(state.getInt("starts") >= 2 && state.getInt("passes") == 2 && state.getInt("stopCount") == 0)
            check(!state.getBoolean("dirtyBeforeFirst") && !state.getBoolean("dirtyBeforeReplay") && state.getBoolean("generationMatches"))
            check(persistedWork(context) == (state.getString("generation") to false))
            check(jobAbsent(scheduler) && session.all == mapOf("origin" to ORIGIN))
        }
        var stage = "guard"
        try {
            check(BuildConfig.DEBUG && context.packageName == "com.appcontrol.parent.acceptance")
            check(context.applicationInfo.flags and ApplicationInfo.FLAG_DEBUGGABLE != 0)
            check(Build.FINGERPRINT.startsWith("google/sdk_gphone64_x86_64/emu64xa:") && !ParentPushWork.enabled())
            when (phase) {
                "run" -> {
                    stage = "baseline"
                    check(session.all.isEmpty() && jobAbsent(scheduler))
                    check(!ParentQueueDeathState.exists(context) && !ParentQueueDeathState.exists(context, true))
                    check(ParentPushWork.notificationsAllowed(context))
                    check(context.getSystemService(NotificationManager::class.java).activeNotifications.none { it.tag?.startsWith("parent-push-call:") == true })
                    val generation = UUID.randomUUID().toString()
                    ParentQueueDeathState.create(context, JSONObject().put("version", ParentQueueDeathState.VERSION)
                        .put("work", snapshot(work.all)), true)
                    ParentQueueDeathState.create(context, JSONObject().put("version", ParentQueueDeathState.VERSION)
                        .put("generation", generation).put("firstPid", 0).put("replayPid", 0)
                        .put("starts", 0).put("passes", 0).put("firstExited", false).put("replayed", false)
                        .put("dirtyBeforeFirst", true).put("dirtyBeforeReplay", true).put("generationMatches", false)
                        .put("stopCount", 0).put("phase", "armed"))
                    check(session.edit().putString("origin", ORIGIN).commit())
                    check(work.edit().clear().putString("generation", generation).putBoolean("dirty", true).commit())
                    val job = JobInfo.Builder(ID, ComponentName(context, ParentQueueDeathProbeService::class.java))
                        .setOverrideDeadline(1_000).setBackoffCriteria(10_000, JobInfo.BACKOFF_POLICY_EXPONENTIAL).build()
                    check(scheduler.enqueue(job, JobWorkItem(Intent("com.appcontrol.parent.FIXTURE_QUEUE_DEATH"))) == JobScheduler.RESULT_SUCCESS)
                    stage = "first-pass"
                    val deadline = System.nanoTime() + TimeUnit.SECONDS.toNanos(20)
                    var state = ParentQueueDeathState.read(context)
                    while (state.getInt("firstPid") == 0 && System.nanoTime() < deadline) {
                        Thread.sleep(50); state = ParentQueueDeathState.read(context)
                    }
                    check(state.getInt("firstPid") > 1 && !state.getBoolean("dirtyBeforeFirst") && !state.getBoolean("firstExited"))
                    result.putInt("parent_push_queue_death_pid", state.getInt("firstPid"))
                    result.putString("parent_push_queue_death_generation", generation)
                    result.putString("parent_push_queue_death_probe", "prepared")
                    result.putString("parent_push_queue_death_version", ParentQueueDeathState.VERSION)
                    // Never finish between prepare and replay: Android kills every process
                    // in the package at finishInstrumentation, including the held service.
                    instrumentation.sendStatus(1, Bundle(result))
                    stage = "external-kill-and-replay"
                    val replayDeadline = System.nanoTime() + TimeUnit.SECONDS.toNanos(110)
                    while (System.nanoTime() < replayDeadline) {
                        state = ParentQueueDeathState.read(context)
                        check(!state.getBoolean("firstExited"))
                        if (state.getBoolean("replayed") && jobAbsent(scheduler)) break
                        Thread.sleep(100)
                    }
                    verifyReplay()
                    result.putString("parent_push_queue_death_probe", "passed")
                    result.putString("stream", "Retained parent work replayed after external debug-process SIGKILL with durable dirty=false; I/O substituted, no Firebase/OEM/reboot claim.\n")
                }
                "verify" -> {
                    stage = "persisted-replay"
                    verifyReplay()
                    result.putString("parent_push_queue_death_probe", "passed")
                    result.putString("stream", "Retained parent work replayed after verified debug-process SIGKILL with durable dirty=false; I/O substituted, no Firebase/OEM/reboot claim.\n")
                }
                "cleanup" -> {
                    stage = "restore"
                    check(ParentQueueDeathState.exists(context, true))
                    check(session.all.isEmpty() || session.all == mapOf("origin" to ORIGIN))
                    if (ParentQueueDeathState.exists(context)) {
                        val pending = scheduler.getPendingJob(ID)
                        check(pending == null || pending.service == ComponentName(context, ParentQueueDeathProbeService::class.java))
                        check(scheduler.getPendingJob(5703) == null)
                        scheduler.cancel(ID)
                        val deadline = System.nanoTime() + TimeUnit.SECONDS.toNanos(10)
                        var safeToRestore = false
                        while (System.nanoTime() < deadline) {
                            val current = ParentQueueDeathState.read(context)
                            val live = context.getSystemService(ActivityManager::class.java).runningAppProcesses.orEmpty().any {
                                it.pid == current.getInt("firstPid") && it.processName == "com.appcontrol.parent.acceptance:parent_queue_death"
                            }
                            if (!live || current.getBoolean("firstExited") || current.getBoolean("replayed")) { safeToRestore = true; break }
                            Thread.sleep(50)
                        }
                        check(safeToRestore && jobAbsent(scheduler))
                    } else check(jobAbsent(scheduler))
                    val baseline = ParentQueueDeathState.read(context, true).getJSONObject("work")
                    check(session.edit().clear().commit())
                    restore(work, baseline)
                    ParentQueueDeathState.remove(context)
                    result.putString("parent_push_queue_death_probe", "cleaned")
                    result.putString("parent_push_queue_death_cleanup", "passed")
                }
                else -> error("Unknown fixture phase")
            }
        } catch (_: Exception) {
            result.putString("parent_push_queue_death_probe", "failed")
            result.putString("observation", stage)
        }
        result.putString("parent_push_queue_death_version", ParentQueueDeathState.VERSION)
        return result
    }
}
