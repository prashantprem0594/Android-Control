package com.appcontrol.parent

import android.Manifest
import android.app.Notification
import android.app.NotificationChannel
import android.app.NotificationManager
import android.app.PendingIntent
import android.app.Service
import android.content.Intent
import android.content.pm.PackageManager
import android.content.pm.ServiceInfo
import android.net.Uri
import android.os.Handler
import android.os.IBinder
import android.os.Looper
import android.os.SystemClock
import java.time.Instant
import java.util.concurrent.Executors
import java.util.concurrent.Future

/** Explicit user-started, ten-minute watch. No boot restart or phone media access. */
class CallWatchService : Service() {
    private val handler = Handler(Looper.getMainLooper())
    private val worker = Executors.newSingleThreadExecutor()
    private var pending: Future<*>? = null
    private var generation = 0
    private var watch: CallWatchState? = null
    private var device = ""
    private val alerts = mutableSetOf<String>()
    private lateinit var notifications: NotificationManager
    private lateinit var client: ParentClient
    override fun onCreate() {
        super.onCreate()
        notifications = getSystemService(NotificationManager::class.java)
        client = ParentClient(this)
        notifications.createNotificationChannel(NotificationChannel(WATCH_CHANNEL, "Call watch status", NotificationManager.IMPORTANCE_LOW))
        notifications.createNotificationChannel(NotificationChannel(ALERT_CHANNEL, "Child incoming-call alerts", NotificationManager.IMPORTANCE_HIGH))
        // Remove stale process-local alerts after a process restart. This service owns these IDs.
        for (entry in notifications.activeNotifications) if (entry.tag?.startsWith(ALERT_TAG) == true) notifications.cancel(entry.tag, ALERT_ID)
    }
    override fun onStartCommand(intent: Intent?, flags: Int, startId: Int): Int {
        if (intent?.action == ACTION_STOP) { stopSelf(); return START_NOT_STICKY }
        val id = intent?.getStringExtra(EXTRA_DEVICE) ?: ""
        if (!id.matches(Regex("dev_[A-Za-z0-9_-]{22}"))) { stopSelf(); return START_NOT_STICKY }
        if (runCatching { startForeground(WATCH_ID, status("Checking child call state. Ten-minute observation window."), ServiceInfo.FOREGROUND_SERVICE_TYPE_DATA_SYNC) }.isFailure) { stopSelf(); return START_NOT_STICKY }
        if (!alertsAllowed() || !client.hasSession()) {
            stopSelf(); return START_NOT_STICKY
        }
        generation++
        handler.removeCallbacksAndMessages(null)
        pending?.cancel(true)
        clearAlerts()
        device = id
        watch = CallWatchState(SystemClock.elapsedRealtime())
        val expected = generation
        handler.postDelayed({ if (expected == generation) stopSelf() }, CallWatchState.WINDOW_MILLIS)
        poll(expected)
        return START_NOT_STICKY
    }
    private fun status(message: String): Notification {
        val stop = PendingIntent.getService(this, 0, Intent(this, CallWatchService::class.java).setAction(ACTION_STOP), PendingIntent.FLAG_IMMUTABLE or PendingIntent.FLAG_UPDATE_CURRENT)
        val open = PendingIntent.getActivity(this, 0, Intent(this, ParentActivity::class.java), PendingIntent.FLAG_IMMUTABLE or PendingIntent.FLAG_UPDATE_CURRENT)
        return Notification.Builder(this, WATCH_CHANNEL).setSmallIcon(android.R.drawable.sym_call_incoming)
            .setContentTitle("Child call watch").setContentText(message).setOngoing(true)
            .setVisibility(Notification.VISIBILITY_PRIVATE).setContentIntent(open).setOnlyAlertOnce(true)
            .addAction(Notification.Action.Builder(null, "Stop watching", stop).build()).build()
    }
    private fun poll(expected: Int) {
        val state = watch ?: return
        if (expected != generation) return
        if (state.expired(SystemClock.elapsedRealtime()) || !alertsAllowed()) { stopSelf(); return }
        val id = device
        val started = SystemClock.elapsedRealtime()
        pending = worker.submit {
            val response = runCatching { client.remoteCalls(id) }
            handler.post {
                if (expected != generation) return@post
                if (!alertsAllowed()) { stopSelf(); return@post }
                if (state.expired(SystemClock.elapsedRealtime())) { stopSelf(); return@post }
                val failure = response.exceptionOrNull()
                if (failure is ParentClient.SignInRequired || failure is ParentClient.RequestFailure && failure.status in setOf(401,403,404)) { stopSelf(); return@post }
                val decision = response.getOrNull()?.let { value -> runCatching {
                    val calls = value.getJSONArray("calls")
                    require(calls.length() <= 8)
                    val entries = linkedMapOf<String,String>()
                    for (index in 0 until calls.length()) {
                        val call = calls.getJSONObject(index)
                        val callId = call.getString("callId")
                        require(!entries.containsKey(callId))
                        entries[callId] = call.getString("state")
                    }
                    state.accept(value.optLong("snapshotSequence",0),
                        value.optString("observedAt").takeIf { it.isNotEmpty() }?.let(Instant::parse),
                        Instant.parse(value.getString("serverTime")), entries, started, SystemClock.elapsedRealtime())
                }.getOrNull() }
                val published = runCatching { if (decision == null) {
                    clearAlerts()
                    notifications.notify(WATCH_ID, status("Call state unavailable. Retrying while the watch remains active."))
                } else {
                    for (old in alerts.toSet() - decision.ringing()) {
                        notifications.cancel(ALERT_TAG + old, ALERT_ID); alerts.remove(old)
                    }
                    for (callId in decision.ringing()) showAlert(id, callId, callId in decision.firstSeen(), decision.validUntil() - SystemClock.elapsedRealtime())
                    notifications.notify(WATCH_ID, status("Watching one child. Alerts may be delayed. Ten-minute observation window."))
                } }
                if (published.isFailure) { stopSelf(); return@post }
                handler.postDelayed({ poll(expected) }, 5_000)
            }
        }
    }
    private fun showAlert(id: String, callId: String, first: Boolean, remainingMillis: Long) {
        if (remainingMillis <= 0) { notifications.cancel(ALERT_TAG + callId, ALERT_ID); alerts.remove(callId); return }
        val open = PendingIntent.getActivity(this, 0, Intent(this, ParentActivity::class.java)
            .setAction(ACTION_REVIEW).setData(Uri.parse("appcontrol-parent://calls/$id/$callId"))
            .putExtra(EXTRA_DEVICE, id).putExtra(EXTRA_CALL,callId)
            .addFlags(Intent.FLAG_ACTIVITY_CLEAR_TOP or Intent.FLAG_ACTIVITY_SINGLE_TOP),
            PendingIntent.FLAG_IMMUTABLE or PendingIntent.FLAG_UPDATE_CURRENT)
        val public = Notification.Builder(this, ALERT_CHANNEL).setSmallIcon(android.R.drawable.sym_call_incoming)
            .setContentTitle("App Control").setContentText("Open the parent app to review an alert.").build()
        notifications.notify(ALERT_TAG + callId, ALERT_ID, Notification.Builder(this, ALERT_CHANNEL)
            .setSmallIcon(android.R.drawable.sym_call_incoming).setContentTitle("Child reported an incoming call")
            .setContentText("Open to refresh the exact call and review available controls.")
            .setVisibility(Notification.VISIBILITY_PRIVATE).setPublicVersion(public)
            .setContentIntent(open).setOnlyAlertOnce(true).setAutoCancel(true)
            .setGroup("parent-call-watch-alerts").setGroupAlertBehavior(if (first) Notification.GROUP_ALERT_ALL else Notification.GROUP_ALERT_SUMMARY)
            .setTimeoutAfter(remainingMillis).build())
        alerts.add(callId)
    }
    private fun clearAlerts() {
        for (id in alerts) notifications.cancel(ALERT_TAG + id, ALERT_ID)
        alerts.clear()
    }
    private fun alertsAllowed(): Boolean = checkSelfPermission(Manifest.permission.POST_NOTIFICATIONS) == PackageManager.PERMISSION_GRANTED &&
        notifications.areNotificationsEnabled() && listOf(WATCH_CHANNEL, ALERT_CHANNEL).all {
            notifications.getNotificationChannel(it)?.importance != NotificationManager.IMPORTANCE_NONE
        }
    override fun onTimeout(startId: Int, fgsType: Int) { stopSelf() }
    override fun onDestroy() {
        generation++
        handler.removeCallbacksAndMessages(null)
        pending?.cancel(true)
        worker.shutdownNow()
        clearAlerts()
        stopForeground(STOP_FOREGROUND_REMOVE)
        super.onDestroy()
    }
    override fun onBind(intent: Intent?): IBinder? = null
    companion object {
        const val ACTION_REVIEW = "com.appcontrol.parent.REVIEW_CALL"
        const val EXTRA_DEVICE = "deviceId"
        const val EXTRA_CALL = "callId"
        private const val ACTION_STOP = "com.appcontrol.parent.STOP_CALL_WATCH"
        private const val WATCH_CHANNEL = "parent-call-watch"
        private const val ALERT_CHANNEL = "parent-call-alerts"
        private const val ALERT_TAG = "parent-call-watch:"
        private const val WATCH_ID = 5601
        private const val ALERT_ID = 5602
    }
}
