package com.appcontrol.child.policy

import android.app.Notification
import android.app.NotificationChannel
import android.app.NotificationManager
import android.app.PendingIntent
import android.content.Context
import android.content.BroadcastReceiver
import android.content.Intent
import android.content.IntentFilter
import android.database.ContentObserver
import android.content.pm.ApplicationInfo
import android.content.pm.ServiceInfo
import android.net.VpnService
import android.os.Build
import android.os.ParcelFileDescriptor
import android.os.Handler
import android.os.Looper
import android.provider.Settings
import android.system.Os
import android.system.OsConstants
import android.system.StructPollfd
import android.telecom.TelecomManager
import com.appcontrol.child.MainActivity
import org.json.JSONObject
import java.io.FileInputStream
import java.time.Instant
import java.util.concurrent.CompletableFuture
import java.util.concurrent.ConcurrentHashMap
import java.util.concurrent.TimeUnit
import java.util.concurrent.atomic.AtomicLong
import java.util.concurrent.atomic.AtomicBoolean

/** Local discard tunnel. Packets are neither forwarded nor recorded. */
class InternetPauseVpn : VpnService() {
    @Volatile private var destroyed = false
    private val refreshing = AtomicBoolean()
    private val refreshPending = AtomicBoolean()
    private val changes = object : BroadcastReceiver() {
        override fun onReceive(context: Context, intent: Intent) { requestRefresh() }
    }
    private val roles = object : ContentObserver(Handler(Looper.getMainLooper())) {
        override fun onChange(selfChange: Boolean) { requestRefresh() }
    }

    override fun onCreate() {
        super.onCreate()
<<<<<<< HEAD
        registerChanges(IntentFilter().apply {
            addAction(Intent.ACTION_PACKAGE_ADDED); addAction(Intent.ACTION_PACKAGE_REMOVED)
            addAction(Intent.ACTION_PACKAGE_CHANGED); addDataScheme("package")
        })
        registerChanges(IntentFilter().apply {
            addAction(TelecomManager.ACTION_DEFAULT_DIALER_CHANGED)
            addAction("android.provider.action.DEFAULT_SMS_PACKAGE_CHANGED")
        })
=======
        registerReceiver(changes, IntentFilter().apply {
            addAction(Intent.ACTION_PACKAGE_ADDED); addAction(Intent.ACTION_PACKAGE_REMOVED)
            addAction(Intent.ACTION_PACKAGE_CHANGED); addDataScheme("package")
        }, Context.RECEIVER_EXPORTED)
        registerReceiver(changes, IntentFilter().apply {
            addAction(TelecomManager.ACTION_DEFAULT_DIALER_CHANGED)
            addAction("android.provider.action.DEFAULT_SMS_PACKAGE_CHANGED")
        }, Context.RECEIVER_EXPORTED)
>>>>>>> origin/main
        contentResolver.registerContentObserver(Settings.Secure.getUriFor(Settings.Secure.DEFAULT_INPUT_METHOD), false, roles)
        contentResolver.registerContentObserver(Settings.Secure.getUriFor(Settings.Secure.ENABLED_ACCESSIBILITY_SERVICES), false, roles)
    }

<<<<<<< HEAD
    private fun registerChanges(filter: IntentFilter) {
        if (Build.VERSION.SDK_INT >= 33) registerReceiver(changes, filter, Context.RECEIVER_EXPORTED)
        else registerReceiver(changes, filter)
    }

=======
>>>>>>> origin/main
    private fun requestRefresh() {
        if (destroyed) return
        refreshPending.set(true)
        if (!refreshing.compareAndSet(false, true)) return
        Thread {
            try {
                do {
                    refreshPending.set(false)
                    if (!destroyed) runCatching { PolicyEnforcer(applicationContext).applyCached() }
                } while (refreshPending.get() && !destroyed)
            } finally {
                refreshing.set(false)
                if (refreshPending.get() && !destroyed) requestRefresh()
            }
        }.start()
    }
    override fun onStartCommand(intent: Intent?, flags: Int, startId: Int): Int {
        val generation = intent?.getLongExtra("generation", -1) ?: -1
        val version = intent?.getLongExtra("version", -1) ?: -1
        if (generation < 0 || version < 1) { stopSelf(startId); return START_NOT_STICKY }
        try {
            val notifications = getSystemService(NotificationManager::class.java)
            notifications.createNotificationChannel(NotificationChannel(CHANNEL, "Internet pause", NotificationManager.IMPORTANCE_LOW))
            val openApp = PendingIntent.getActivity(this, 0, Intent(this, MainActivity::class.java),
                PendingIntent.FLAG_IMMUTABLE or PendingIntent.FLAG_UPDATE_CURRENT)
            val notification = Notification.Builder(this, CHANNEL).setSmallIcon(android.R.drawable.ic_lock_lock)
                .setContentTitle("Internet pause active")
                .setContentText("Managed apps are paused. Device management remains connected.")
                .setContentIntent(openApp).setOngoing(true).build()
<<<<<<< HEAD
            if (Build.VERSION.SDK_INT >= 34) startForeground(NOTIFICATION_ID, notification,
                ServiceInfo.FOREGROUND_SERVICE_TYPE_SYSTEM_EXEMPTED)
            else startForeground(NOTIFICATION_ID, notification)
=======
            startForeground(NOTIFICATION_ID, notification,
                if (Build.VERSION.SDK_INT >= 34) ServiceInfo.FOREGROUND_SERVICE_TYPE_SYSTEM_EXEMPTED else 0)
>>>>>>> origin/main
        } catch (failure: Exception) {
            pending.remove(generation)?.complete(false)
            stopSelf(startId)
            return START_NOT_STICKY
        }
        Thread {
            val success = runCatching {
                val raw = getSharedPreferences("policy-cache", MODE_PRIVATE).getString("desired", null) ?: error("No policy")
                val policy = JSONObject(raw)
                val effective = PolicyConfig.parse(policy).resolve(Instant.now())
                check(policy.getLong("version") == version && effective.internetPaused)
                check(prepare(this) == null) { "VPN permission required" }
                val enforcer = PolicyEnforcer(this)
                val apps = packageManager.getInstalledApplications(0).map { info ->
                    InternetPausePlan.App(info.packageName,
                        info.flags and (ApplicationInfo.FLAG_SYSTEM or ApplicationInfo.FLAG_UPDATED_SYSTEM_APP) != 0,
                        runCatching { enforcer.isProtected(info.packageName) }.getOrDefault(true))
                }
                val targets = InternetPausePlan.targets(packageName, apps, effective.alwaysAllowedPackages)
                synchronized(tunnelLock) {
                    check(!destroyed) { "VPN service stopped" }
                    check(sequence.get() == generation) { "Superseded VPN request" }
                    if (!InternetPausePlan.needsTunnel(targets)) {
                        closeActive()
                        stopSelf(startId)
                    } else {
                        val existing = active
                        if (existing != null && existing.targets == targets && existing.descriptor.fileDescriptor.valid()) {
                            existing.generation = generation
                            existing.startId = startId
                        } else {
                            val builder = Builder().setSession("App Control internet pause").setMtu(1500)
                                .addAddress("10.254.254.1", 32).addRoute("0.0.0.0", 0)
                                .addAddress("fd42:ac::1", 128).addRoute("::", 0)
                                .addDnsServer("10.254.254.1").setBlocking(false)
                            // No empty allowed list and no management/system application can reach here.
                            for (name in targets) builder.addAllowedApplication(name)
                            val descriptor = checkNotNull(builder.establish()) { "VPN establishment failed" }
                            closeActive()
                            val session = Session(this, descriptor, targets, generation, startId)
                            active = session
                            Thread {
                                try {
                                    FileInputStream(descriptor.fileDescriptor).use { input ->
                                        val discard = ByteArray(32767)
                                        val poll = StructPollfd().apply {
                                            fd = descriptor.fileDescriptor
                                            events = OsConstants.POLLIN.toShort()
                                        }
                                        while (descriptor.fileDescriptor.valid()) {
                                            if (Os.poll(arrayOf(poll), 1000) == 0) continue
                                            if (poll.revents.toInt() and OsConstants.POLLIN == 0 || input.read(discard) < 0) break
                                            // Discard locally; never log content.
                                        }
                                    }
                                } catch (closed: Exception) {
                                    // Closing/revoking the interface wakes the reader.
                                } finally {
                                    val wasCurrent = synchronized(tunnelLock) {
                                        if (active === session) { closeActive(); true } else false
                                    }
                                    if (wasCurrent) stopSelf(session.startId)
                                }
                            }.start()
                        }
                    }
                }
                true
            }.getOrElse {
                synchronized(tunnelLock) { if (sequence.get() == generation) closeActive() }
                stopSelf(startId)
                false
            }
            pending.remove(generation)?.complete(success)
        }.start()
        return START_NOT_STICKY
    }

    override fun onRevoke() {
        sequence.incrementAndGet()
        synchronized(tunnelLock) { closeActive() }
        pending.values.forEach { it.complete(false) }
        pending.clear()
        stopSelf()
        super.onRevoke()
    }

    override fun onDestroy() {
        destroyed = true
        runCatching { unregisterReceiver(changes) }
        contentResolver.unregisterContentObserver(roles)
        synchronized(tunnelLock) { if (active?.owner === this) closeActive() }
        stopForeground(STOP_FOREGROUND_REMOVE)
        super.onDestroy()
    }

    private class Session(val owner: InternetPauseVpn, val descriptor: ParcelFileDescriptor,
                          val targets: Set<String>, var generation: Long, var startId: Int)

    companion object {
        private const val CHANNEL = "internet-pause"
        private const val NOTIFICATION_ID = 4201
        private val tunnelLock = Any()
        private var active: Session? = null
        private val sequence = AtomicLong()
        private val pending = ConcurrentHashMap<Long, CompletableFuture<Boolean>>()

        private fun closeActive() {
            val previous = active
            active = null
            runCatching { previous?.descriptor?.close() }
        }

        /** Run on a worker. Resume closes our interface before reporting success. */
        fun reconcile(context: Context, version: Long, paused: Boolean): Boolean {
            val app = context.applicationContext
            val generation = sequence.incrementAndGet()
            if (!paused) {
                synchronized(tunnelLock) { closeActive() }
                app.stopService(Intent(app, InternetPauseVpn::class.java))
                return true
            }
            if (prepare(app) != null) return false
            val completion = CompletableFuture<Boolean>()
            pending[generation] = completion
            return try {
                app.startForegroundService(Intent(app, InternetPauseVpn::class.java)
                    .putExtra("generation", generation).putExtra("version", version))
                completion.get(8, TimeUnit.SECONDS)
            } catch (failure: Exception) {
                sequence.compareAndSet(generation, generation + 1)
                synchronized(tunnelLock) { if (active?.generation == generation) closeActive() }
                false
            } finally { pending.remove(generation) }
        }
    }
}
