package com.appcontrol.child.policy

import android.content.BroadcastReceiver
import android.content.Context
import android.content.Intent
import com.appcontrol.child.sync.SyncScheduler

class PolicyBoundaryReceiver : BroadcastReceiver() {
    override fun onReceive(context: Context, intent: Intent) {
        val pending = goAsync()
        Thread {
            try {
                runCatching { PolicyEnforcer(context).applyCached() }
                runCatching { SyncScheduler.requestSync(context) }
            } finally { pending.finish() }
        }.start()
    }
}
