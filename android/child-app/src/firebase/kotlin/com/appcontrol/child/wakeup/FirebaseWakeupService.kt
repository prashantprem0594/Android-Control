package com.appcontrol.child.wakeup

import com.appcontrol.child.sync.SyncScheduler
import com.google.firebase.messaging.FirebaseMessagingService
import com.google.firebase.messaging.RemoteMessage

/** No action execution, payload persistence or network work in FCM callbacks. */
class FirebaseWakeupService : FirebaseMessagingService() {
    private fun request() { runCatching { SyncScheduler.requestSync(applicationContext) } }
    override fun onNewToken(token: String) { request() }
    override fun onDeletedMessages() { request() }
    override fun onMessageReceived(message: RemoteMessage) {
        if (WakeupHint.accepts(message.data, message.notification != null)) request()
    }
}
