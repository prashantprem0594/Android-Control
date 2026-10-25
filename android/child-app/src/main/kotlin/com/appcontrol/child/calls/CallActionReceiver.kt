package com.appcontrol.child.calls

import android.content.BroadcastReceiver
import android.content.Context
import android.content.Intent

class CallActionReceiver : BroadcastReceiver() {
    override fun onReceive(context: Context, intent: Intent) {
        val id = intent.getStringExtra("callId") ?: return
        val action = runCatching { CallActionGate.Action.valueOf(intent.getStringExtra("action") ?: "") }.getOrNull() ?: return
        CallRegistry.userAction(id, action)
    }
}
