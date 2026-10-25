package com.appcontrol.child.sync

import android.content.BroadcastReceiver
import android.content.Context
import android.content.Intent
import com.appcontrol.child.policy.PolicyEnforcer

class BootReceiver : BroadcastReceiver() {
    override fun onReceive(context: Context, intent: Intent) {
        if (intent.action !in setOf(Intent.ACTION_BOOT_COMPLETED, Intent.ACTION_MY_PACKAGE_REPLACED,
                Intent.ACTION_TIME_CHANGED, Intent.ACTION_TIMEZONE_CHANGED,
                "android.app.action.SCHEDULE_EXACT_ALARM_PERMISSION_STATE_CHANGED")) return
        val pending = goAsync()
        Thread {
            try {
                runCatching { PolicyEnforcer(context).applyCached() }
                runCatching { SyncScheduler.ensureScheduled(context) }
            } finally { pending.finish() }
        }.start()
    }
}
