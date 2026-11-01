package com.appcontrol.parent.push

import com.google.firebase.messaging.FirebaseMessagingService
import com.google.firebase.messaging.RemoteMessage

/** Callback schedules bounded work; no payload/token persistence, network or alert publication here. */
class ParentFirebaseMessagingService : FirebaseMessagingService() {
    private fun request(highPriority: Boolean = false) { runCatching { ParentPushWork.schedule(applicationContext, highPriority) } }

    override fun onNewToken(token: String) { request() }
    override fun onDeletedMessages() { request() }
    override fun onMessageReceived(message: RemoteMessage) {
        if (ParentPushHint.accepts(message.data, message.notification != null))
            request(message.priority == RemoteMessage.PRIORITY_HIGH)
    }
}
