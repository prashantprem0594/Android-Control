package com.appcontrol.child.policy

import android.app.AlarmManager
import android.app.PendingIntent
import android.content.Context
import android.content.Intent
<<<<<<< HEAD
import android.os.Build
=======
>>>>>>> origin/main
import java.time.Instant

object ScheduleAlarm {
    fun schedule(context: Context, next: Instant?) {
        val app = context.applicationContext
        val manager = app.getSystemService(AlarmManager::class.java)
        val pending = PendingIntent.getBroadcast(app, 4301, Intent(app, PolicyBoundaryReceiver::class.java),
            PendingIntent.FLAG_UPDATE_CURRENT or PendingIntent.FLAG_IMMUTABLE)
        manager.cancel(pending)
        var mode = "none"
        if (next != null) {
            val trigger = maxOf(next.toEpochMilli(), System.currentTimeMillis() + 1000)
<<<<<<< HEAD
            if (Build.VERSION.SDK_INT < 31 || manager.canScheduleExactAlarms()) {
=======
            if (manager.canScheduleExactAlarms()) {
>>>>>>> origin/main
                try {
                    manager.setExactAndAllowWhileIdle(AlarmManager.RTC_WAKEUP, trigger, pending)
                    mode = "exact"
                } catch (denied: SecurityException) {
                    manager.setAndAllowWhileIdle(AlarmManager.RTC_WAKEUP, trigger, pending)
                    mode = "inexact"
                }
            } else {
                manager.setAndAllowWhileIdle(AlarmManager.RTC_WAKEUP, trigger, pending)
                mode = "inexact"
            }
        }
        app.getSharedPreferences("policy-cache", Context.MODE_PRIVATE).edit().putString("alarmMode", mode).apply()
    }
}
