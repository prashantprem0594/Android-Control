package com.appcontrol.parent.push

import android.Manifest
import android.app.Notification
import android.app.NotificationChannel
import android.app.NotificationManager
import android.app.PendingIntent
import android.app.job.JobInfo
import android.app.job.JobParameters
import android.app.job.JobScheduler
import android.app.job.JobWorkItem
import android.content.ComponentName
import android.content.Context
import android.content.Intent
import android.content.pm.PackageManager
import android.net.Uri
import android.os.SystemClock
import com.appcontrol.parent.CallWatchService
import com.appcontrol.parent.ParentActivity
import com.appcontrol.parent.ParentClient
import org.json.JSONObject
import java.time.Instant
import java.util.UUID
import java.util.concurrent.atomic.AtomicBoolean
import java.util.concurrent.atomic.AtomicLong

/** A hint only requests authenticated synchronization. No Firebase data becomes notification content. */
object ParentPushWork {
    internal const val JOB_ID = 5701
    internal const val EXPEDITED_JOB_ID = 5703
    private const val CHANNEL = "parent-push-call-alerts"
    private const val TAG = "parent-push-call:"
    private const val ALERT_ID = 5702
    private val lock = Any()
    @Volatile private var blockedInProcess = false
    private fun state(context: Context) = context.applicationContext.getSharedPreferences("parent-push-work", Context.MODE_PRIVATE)
    fun enabled(): Boolean = ParentPushProvider.enabled()
    fun notificationsAllowed(context: Context): Boolean {
        val manager = context.getSystemService(NotificationManager::class.java)
        return context.checkSelfPermission(Manifest.permission.POST_NOTIFICATIONS) == PackageManager.PERMISSION_GRANTED &&
            manager.areNotificationsEnabled() && manager.getNotificationChannel(CHANNEL)?.importance != NotificationManager.IMPORTANCE_NONE
    }
    private fun clearAlerts(context: Context) {
        val manager = context.getSystemService(NotificationManager::class.java)
        for (entry in manager.activeNotifications) if (entry.tag?.startsWith(TAG) == true) manager.cancel(entry.tag, ALERT_ID)
    }
    private fun reset(context: Context, blocked: Boolean) {
        blockedInProcess = true
        try {
            synchronized(lock) {
                check(state(context).edit().clear().putString("generation", UUID.randomUUID().toString()).putBoolean("blocked", blocked).commit())
                blockedInProcess = blocked
                clearAlerts(context)
            }
        } finally {
            if (blockedInProcess) runCatching { synchronized(lock) { clearAlerts(context) } }
            context.getSystemService(JobScheduler::class.java).cancel(JOB_ID)
            context.getSystemService(JobScheduler::class.java).cancel(EXPEDITED_JOB_ID)
        }
    }
    /** Runs before logout can block on network. A new login is required to unblock publication. */
    fun stop(context: Context) = reset(context.applicationContext, true)
    fun newSession(context: Context) {
        reset(context.applicationContext, false)
        runCatching { schedule(context) }
    }
    fun schedule(context: Context) = schedule(context, false)
    fun schedule(context: Context, highPriority: Boolean) {
        val app = context.applicationContext
        if (!enabled() || !ParentClient(app).hasSession()) return
        synchronized(lock) {
            if (blockedInProcess || state(app).getBoolean("blocked", false)) return
            if (!notificationsAllowed(app)) {
                clearAlerts(app)
                app.getSystemService(JobScheduler::class.java).cancel(JOB_ID)
                app.getSystemService(JobScheduler::class.java).cancel(EXPEDITED_JOB_ID)
                return
            }
            val prefs = state(app)
            val dirty = ParentPushQueueState(prefs.getBoolean("dirty", false))
            val changed = dirty.request()
            if (changed || prefs.getString("generation", null) == null)
                check(prefs.edit().putBoolean("dirty", true)
                    .putString("generation", prefs.getString("generation", null) ?: UUID.randomUUID().toString()).commit())
            val scheduler = app.getSystemService(JobScheduler::class.java)
            val choice = ParentPushQueueState.choose(highPriority,
                scheduler.getPendingJob(JOB_ID) != null, scheduler.getPendingJob(EXPEDITED_JOB_ID) != null)
            check(enqueueLocked(app, choice)) { "Android could not schedule parent alert synchronization" }
        }
    }
    private fun info(context: Context, expedited: Boolean): JobInfo =
        JobInfo.Builder(if (expedited) EXPEDITED_JOB_ID else JOB_ID,
            ComponentName(context, if (expedited) ParentPushExpeditedJobService::class.java else ParentPushJobService::class.java))
            .setRequiredNetworkType(JobInfo.NETWORK_TYPE_ANY)
            .setBackoffCriteria(30_000, JobInfo.BACKOFF_POLICY_EXPONENTIAL)
            .setExpedited(expedited).build()
    /** Called under lock. No running JobInfo is replaced, and no hint adds a second queued item. */
    private fun enqueueLocked(context: Context, choice: ParentPushQueueState.Queue, forceEmpty: Boolean = false): Boolean {
        if (choice == ParentPushQueueState.Queue.NONE) return true
        val scheduler = context.getSystemService(JobScheduler::class.java)
        val expedited = choice == ParentPushQueueState.Queue.EXPEDITED
        val job = info(context, expedited)
        val existing = scheduler.getPendingJob(job.id)
        if (existing != null && !forceEmpty) {
            check(existing.service == job.service) { "Parent alert job identity conflict" }
            return true
        }
        val result = scheduler.enqueue(existing ?: job,
            JobWorkItem(Intent("com.appcontrol.parent.SYNC_CALL_ALERTS")))
        if (result == JobScheduler.RESULT_SUCCESS) return true
        if (expedited) return enqueueLocked(context,
            ParentPushQueueState.fallback(scheduler.getPendingJob(JOB_ID) != null))
        return false
    }
    /** First work survives process death even if the previous process already cleared the dirty bit. */
    internal fun beginSync(context: Context, expected: String, retainedWork: Boolean): Boolean = synchronized(lock) {
        if (!current(context, expected)) return@synchronized false
        val prefs = state(context)
        val queue = ParentPushQueueState(prefs.getBoolean("dirty", false))
        if (!queue.claim(retainedWork)) return@synchronized false
        check(prefs.edit().putBoolean("dirty", queue.dirty()).commit())
        true
    }
    internal data class QueueStep(val completed: Boolean, val next: JobWorkItem?)
    private fun takeLocked(context: Context, expected: String, params: JobParameters): JobWorkItem? {
        val item = params.dequeueWork()
        if (item == null && current(context, expected) && state(context).getBoolean("dirty", false)) {
            // Covers a crash between completeWork and dequeueWork: the restored job may
            // have no item, but its durable dirty request still needs a fresh queue item.
            val queue = if (params.jobId == EXPEDITED_JOB_ID) ParentPushQueueState.Queue.EXPEDITED else ParentPushQueueState.Queue.REGULAR
            check(enqueueLocked(context, queue, true)) { "Android could not recover pending parent alert work" }
        }
        return item
    }
    internal fun takeWork(context: Context, expected: String, params: JobParameters): JobWorkItem? = synchronized(lock) {
        takeLocked(context, expected, params)
    }
    internal fun finishWork(context: Context, expected: String, params: JobParameters,
                            item: JobWorkItem, discardAttempt: Boolean = false): QueueStep = synchronized(lock) {
        if (!discardAttempt && current(context, expected) && state(context).getBoolean("dirty", false))
            return@synchronized QueueStep(false, item)
        // Scheduling uses this same lock. A late hint either sets dirty before this
        // check or sees the framework queue stopped after dequeueWork returns null.
        params.completeWork(item)
        QueueStep(true, takeLocked(context, expected, params))
    }
    internal fun generation(context: Context): String? = synchronized(lock) {
        if (blockedInProcess || state(context).getBoolean("blocked", false)) null else state(context).getString("generation", null)
    }
    private fun fastCurrent(context: Context, expected: String): Boolean = synchronized(lock) {
        !blockedInProcess && !state(context).getBoolean("blocked", false) && state(context).getString("generation", null) == expected &&
            context.applicationContext.getSharedPreferences("parent-session", Context.MODE_PRIVATE).getString("origin", null) != null
    }
    internal fun current(context: Context, expected: String): Boolean = fastCurrent(context, expected) && notificationsAllowed(context)
    private fun keys(value: JSONObject): Set<String> = value.keys().asSequence().toSet()
    private fun sequence(value: JSONObject, key: String): Long {
        val raw = value.get(key)
        require(raw is Int || raw is Long)
        return (raw as Number).toLong().also { require(it > 0) }
    }
    private fun ledger(context: Context): MutableMap<String, Long> {
        val raw = state(context).getString("seen", "{}") ?: "{}"
        require(raw.length <= 64 * 1024)
        val stored = JSONObject(raw)
        require(stored.length() <= ParentPushLedger.CAPACITY)
        return stored.keys().asSequence().associateWith { key -> sequence(stored, key) }.toMutableMap()
    }
    /** True requests bounded JobScheduler backoff, never an unbounded retry loop. */
    internal fun synchronize(context: Context, expected: String, stopped: () -> Boolean): Boolean {
        val app = context.applicationContext
        val permissionAllowed = AtomicBoolean(true)
        val permissionCheckedAt = AtomicLong(0)
        fun cancelled(): Boolean {
            if (stopped() || !fastCurrent(app, expected)) return true
            val now = SystemClock.elapsedRealtime()
            val last = permissionCheckedAt.get()
            if (now - last >= 500 && permissionCheckedAt.compareAndSet(last, now))
                permissionAllowed.set(notificationsAllowed(app))
            return !permissionAllowed.get()
        }
        // Fresh checks at security boundaries; the transport watchdog uses only
        // local generation/session reads plus a throttled permission query.
        fun freshAllowed() = !stopped() && fastCurrent(app, expected) && notificationsAllowed(app)
        if (!enabled() || !freshAllowed()) return false
        val client = ParentClient(app, 8_000, ::cancelled)
        val feedStarted = SystemClock.elapsedRealtime()
        val feed = client.parentCallEvents() // Validate the restored session before asking Firebase for a token.
        require(keys(feed) == setOf("serverTime", "events"))
        val feedTime = Instant.parse(feed.getString("serverTime"))
        val values = feed.getJSONArray("events")
        require(values.length() <= 100)
        val events = ArrayList<ParentPushEventRules.Event>()
        val identities = HashSet<String>()
        for (index in 0 until values.length()) {
            val value = values.getJSONObject(index)
            require(keys(value) == setOf("eventId", "deviceId", "callId", "snapshotSequence", "observedAt", "expiresAt"))
            val event = ParentPushEventRules.Event(value.getString("eventId"), value.getString("deviceId"), value.getString("callId"),
                sequence(value, "snapshotSequence"), Instant.parse(value.getString("observedAt")), Instant.parse(value.getString("expiresAt")))
            require(identities.add(event.eventId))
            events.add(event)
        }
        if (!freshAllowed()) return false
        val token = ParentPushProvider.token(app, ::cancelled)
        if (!freshAllowed()) return false
        val retry = token == null
        if (token != null) {
            val digest = ParentPushEventRules.fingerprint(token)
            val register = synchronized(lock) {
                val prefs = state(app)
                val registeredAt = prefs.getLong("registered_at", 0)
                val now = System.currentTimeMillis()
                prefs.getString("registered_hash", null) != digest || registeredAt <= 0 || now < registeredAt || now - registeredAt >= 3_600_000
            }
            if (register) {
                if (!freshAllowed()) return false
                client.registerParentWakeup(token)
                synchronized(lock) {
                    if (cancelled()) return false
                    check(state(app).edit().putString("registered_hash", digest).putLong("registered_at", System.currentTimeMillis()).commit())
                }
            }
        }
        for (event in events) {
            if (cancelled()) return false
            val digest = ParentPushEventRules.fingerprint(event.eventId)
            val alreadySeen = synchronized(lock) { ledger(app).containsKey(digest) }
            if (alreadySeen) continue
            val now = SystemClock.elapsedRealtime()
            if (event.observedAt.isAfter(feedTime) ||
                java.time.Duration.between(feedTime, event.expiresAt).toMillis() <= now - feedStarted) continue
            val reportStarted = SystemClock.elapsedRealtime()
            val report = try { client.remoteCalls(event.deviceId) } catch (failure: ParentClient.RequestFailure) {
                if (failure.status in setOf(403, 404)) continue else throw failure
            }
            val calls = report.getJSONArray("calls")
            require(calls.length() <= 8)
            val states = linkedMapOf<String, String>()
            for (index in 0 until calls.length()) {
                val call = calls.getJSONObject(index)
                val id = call.getString("callId")
                val parsed = UUID.fromString(id)
                require(parsed.toString() == id && parsed.version() == 4 && parsed.variant() == 2 && !states.containsKey(id))
                val status = call.getString("state")
                require(status in setOf("RINGING", "ACTIVE", "HOLDING", "DIALING", "CONNECTING", "SELECTING_ACCOUNT", "DISCONNECTING", "DISCONNECTED", "OTHER"))
                states[id] = status
            }
            val validatedAt = SystemClock.elapsedRealtime()
            val remaining = ParentPushEventRules.remainingMillis(event, feedTime, feedStarted,
                Instant.parse(report.getString("serverTime")), report.optString("observedAt").takeIf { it.isNotEmpty() }?.let(Instant::parse),
                sequence(report, "snapshotSequence"), states, reportStarted, validatedAt)
            if (remaining <= 0 || cancelled()) continue
            synchronized(lock) {
                if (!freshAllowed()) return false
                val seen = ledger(app)
                if (ParentPushLedger.mark(seen, digest, System.currentTimeMillis())) {
                    // Commit first: process death can suppress an alert, but cannot replay it.
                    check(state(app).edit().putString("seen", JSONObject(seen as Map<*, *>).toString()).commit())
                    if (freshAllowed()) publish(app, event, digest, validatedAt + remaining)
                }
            }
        }
        return retry
    }
    private fun publish(context: Context, event: ParentPushEventRules.Event, digest: String, deadline: Long) {
        val manager = context.getSystemService(NotificationManager::class.java)
        manager.createNotificationChannel(NotificationChannel(CHANNEL, "Incoming-call push alerts", NotificationManager.IMPORTANCE_HIGH))
        if (!notificationsAllowed(context)) return
        val remaining = deadline - SystemClock.elapsedRealtime()
        if (remaining <= 0) return
        val open = PendingIntent.getActivity(context, 0, Intent(context, ParentActivity::class.java)
            .setAction(CallWatchService.ACTION_REVIEW)
            .setData(Uri.parse("appcontrol-parent://calls/${event.deviceId}/${event.callId}"))
            .putExtra(CallWatchService.EXTRA_DEVICE, event.deviceId).putExtra(CallWatchService.EXTRA_CALL, event.callId)
            .addFlags(Intent.FLAG_ACTIVITY_CLEAR_TOP or Intent.FLAG_ACTIVITY_SINGLE_TOP),
            PendingIntent.FLAG_IMMUTABLE or PendingIntent.FLAG_UPDATE_CURRENT)
        val public = Notification.Builder(context, CHANNEL).setSmallIcon(android.R.drawable.sym_call_incoming)
            .setContentTitle("App Control").setContentText("Open the parent app to review an alert.").build()
        manager.notify(TAG + digest, ALERT_ID, Notification.Builder(context, CHANNEL).setSmallIcon(android.R.drawable.sym_call_incoming)
            .setContentTitle("Child reported an incoming call").setContentText("Open to refresh the exact call and review available controls.")
            .setVisibility(Notification.VISIBILITY_PRIVATE).setPublicVersion(public).setContentIntent(open)
            .setAutoCancel(true).setOnlyAlertOnce(true).setGroup("parent-push-call-alerts").setTimeoutAfter(remaining).build())
    }
}
