package com.appcontrol.child.location

import android.app.Notification
import android.app.NotificationChannel
import android.app.NotificationManager
import android.app.PendingIntent
import android.app.Service
import android.content.Context
import android.content.Intent
import android.content.pm.PackageManager
import android.location.LocationListener
import android.location.LocationManager
import android.os.IBinder
import android.os.SystemClock

/** A child-started foreground location session. It never starts from a parent command. */
class VisibleLocationShareService : Service() {
    private var manager: LocationManager? = null
    private var listener: LocationListener? = null
    @Volatile private var lastUploadElapsed = 0L

    override fun onStartCommand(intent: Intent?, flags: Int, startId: Int): Int {
        if (intent?.action == ACTION_STOP) { stopSelf(); return START_NOT_STICKY }
        if (checkSelfPermission(android.Manifest.permission.ACCESS_FINE_LOCATION) != PackageManager.PERMISSION_GRANTED &&
            checkSelfPermission(android.Manifest.permission.ACCESS_COARSE_LOCATION) != PackageManager.PERMISSION_GRANTED) {
            stopSelf(); return START_NOT_STICKY
        }
        startForeground(NOTIFICATION_ID, notification())
        val locationManager = getSystemService(LocationManager::class.java)
        manager = locationManager
        val uploader = VisibleLocationUploader(applicationContext)
        listener = LocationListener { location ->
            // The callback arrives only after the child has started this service.
            // Coalesce provider duplicates and do network work off the callback.
            val now = SystemClock.elapsedRealtime()
            if (now - lastUploadElapsed < MIN_UPLOAD_INTERVAL_MS) return@LocationListener
            lastUploadElapsed = now
            Thread { runCatching { uploader.upload(location) } }.start()
        }
        runCatching { locationManager.requestLocationUpdates(LocationManager.GPS_PROVIDER, 30_000L, 25f, listener!!) }
        runCatching { locationManager.requestLocationUpdates(LocationManager.NETWORK_PROVIDER, 30_000L, 25f, listener!!) }
        return START_NOT_STICKY
    }

    override fun onDestroy() {
        listener?.let { value -> manager?.removeUpdates(value) }
        listener = null; manager = null
        super.onDestroy()
    }
    override fun onBind(intent: Intent?): IBinder? = null

    private fun notification(): Notification {
        val notifications = getSystemService(NotificationManager::class.java)
        notifications.createNotificationChannel(NotificationChannel(CHANNEL, "Location sharing", NotificationManager.IMPORTANCE_LOW))
        val stop = PendingIntent.getService(this, 0, Intent(this, VisibleLocationShareService::class.java).setAction(ACTION_STOP),
            PendingIntent.FLAG_UPDATE_CURRENT or PendingIntent.FLAG_IMMUTABLE)
        return Notification.Builder(this, CHANNEL)
            .setSmallIcon(android.R.drawable.ic_menu_mylocation)
            .setContentTitle("Location sharing is active")
            .setContentText("Shared with your paired parent. This session started on this phone. Tap Stop to end it.")
            .setOngoing(true)
            .addAction(Notification.Action.Builder(null, "Stop", stop).build())
            .build()
    }
    companion object {
        private const val CHANNEL = "visible_location_share"
        private const val NOTIFICATION_ID = 7302
        private const val MIN_UPLOAD_INTERVAL_MS = 30_000L
        private const val ACTION_STOP = "com.appcontrol.child.location.STOP"
        fun start(context: Context) = context.startForegroundService(Intent(context, VisibleLocationShareService::class.java))
        fun stop(context: Context) = context.stopService(Intent(context, VisibleLocationShareService::class.java))
    }
}
