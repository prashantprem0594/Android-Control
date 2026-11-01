package com.appcontrol.child.commands

import android.Manifest
import android.app.Notification
import android.app.NotificationChannel
import android.app.NotificationManager
import android.app.PendingIntent
import android.content.Context
import android.content.Intent
import android.content.pm.PackageManager
import android.os.Build
import java.time.Instant

/** Android requires a user gesture before a background app can take over the screen. */
object LaunchRequestNotification {
    private const val CHANNEL = "parent_launch_requests"

    fun post(context: Context, commandId: String, packageName: String?, deadline: Instant) {
        if (Build.VERSION.SDK_INT >= 33 && context.checkSelfPermission(Manifest.permission.POST_NOTIFICATIONS) != PackageManager.PERMISSION_GRANTED) {
            error("Allow notifications on the child phone to receive app launch requests")
        }
        val manager = context.getSystemService(NotificationManager::class.java)
        manager.createNotificationChannel(NotificationChannel(CHANNEL, "Parent app requests", NotificationManager.IMPORTANCE_HIGH))
        if (packageName != null) {
            require(packageName.matches(Regex("[A-Za-z_][A-Za-z0-9_]*(\\.[A-Za-z_][A-Za-z0-9_]*)+")))
            check(context.packageManager.getLaunchIntentForPackage(packageName) != null) { "That app is no longer available to open" }
        }
        val intent = Intent(context, LaunchRequestActivity::class.java)
            .putExtra(LaunchRequestActivity.EXTRA_PACKAGE, packageName)
            .putExtra(LaunchRequestActivity.EXTRA_EXPIRES_AT, deadline.toEpochMilli())
        val pending = PendingIntent.getActivity(context, commandId.hashCode(), intent,
            PendingIntent.FLAG_UPDATE_CURRENT or PendingIntent.FLAG_IMMUTABLE)
        val title = if (packageName == null) "Open Settings" else "Open requested app"
        val notification = Notification.Builder(context, CHANNEL)
            .setSmallIcon(android.R.drawable.ic_menu_view)
            .setContentTitle(title)
            .setContentText("Tap to open on this phone")
            .setContentIntent(pending)
            .setAutoCancel(true)
            .setCategory(Notification.CATEGORY_RECOMMENDATION)
            .build()
        manager.notify(commandId.hashCode(), notification)
    }
}
