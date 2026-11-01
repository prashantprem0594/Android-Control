package com.appcontrol.child.calls

import android.app.Notification
import android.app.NotificationChannel
import android.app.NotificationManager
import android.app.PendingIntent
import android.app.Person
import android.content.Context
import android.content.Intent
import android.net.Uri
import android.os.Build
import android.telecom.Call

object CallNotifications {
    private const val CHANNEL = "telecom-calls"
    private const val ID = 4501
    fun show(context: Context, entry: CallRegistry.Entry) {
        val manager = context.getSystemService(NotificationManager::class.java)
        manager.createNotificationChannel(NotificationChannel(CHANNEL, "Phone calls", NotificationManager.IMPORTANCE_HIGH).apply {
            setSound(null, null) // Telecom retains ringing responsibility.
        })
        if (CallRegistry.state(entry.call) == CallActionGate.State.DISCONNECTED) { cancel(context, entry.id); return }
        val open = PendingIntent.getActivity(context, 0,
            Intent(context, CallActivity::class.java).setData(Uri.parse("appcontrol://calls/${entry.id}"))
                .putExtra("callId", entry.id).addFlags(Intent.FLAG_ACTIVITY_NEW_TASK or Intent.FLAG_ACTIVITY_SINGLE_TOP),
            PendingIntent.FLAG_IMMUTABLE or PendingIntent.FLAG_UPDATE_CURRENT)
        fun action(type: CallActionGate.Action): PendingIntent = PendingIntent.getBroadcast(context, 0,
            Intent(context, CallActionReceiver::class.java).setData(Uri.parse("appcontrol://calls/${entry.id}/${type.name}"))
                .putExtra("callId", entry.id).putExtra("action", type.name),
            PendingIntent.FLAG_IMMUTABLE or PendingIntent.FLAG_UPDATE_CURRENT)
        val ringing = CallRegistry.state(entry.call) == CallActionGate.State.RINGING
        val person = Person.Builder().setName(CallRegistry.label(entry.call)).setImportant(true).build()
        val publicVersion = Notification.Builder(context, CHANNEL)
            .setSmallIcon(android.R.drawable.sym_call_incoming)
            .setContentTitle(if (ringing) "Incoming call" else "Call in progress")
            .setContentText(CallNotificationPrivacy.lockScreenText())
            .setCategory(Notification.CATEGORY_CALL)
            .build()
        val builder = Notification.Builder(context, CHANNEL).setSmallIcon(android.R.drawable.sym_call_incoming)
            .setContentTitle(if (ringing) "Incoming call" else "Call in progress")
            .setContentText(CallRegistry.label(entry.call)).setCategory(Notification.CATEGORY_CALL)
            .setContentIntent(open).setOngoing(true).setOnlyAlertOnce(true)
            .setVisibility(Notification.VISIBILITY_PRIVATE).setPublicVersion(publicVersion)
        if (Build.VERSION.SDK_INT >= 31) {
            builder.setStyle(if (ringing) Notification.CallStyle.forIncomingCall(person, action(CallActionGate.Action.REJECT), action(CallActionGate.Action.ANSWER))
                else Notification.CallStyle.forOngoingCall(person, action(CallActionGate.Action.END)))
        } else {
            fun add(label: String, type: CallActionGate.Action) {
                builder.addAction(Notification.Action.Builder(null, label, action(type)).build())
            }
            if (ringing) {
                add("Reject", CallActionGate.Action.REJECT)
                add("Answer", CallActionGate.Action.ANSWER)
            } else add("End", CallActionGate.Action.END)
        }
        if (ringing && (Build.VERSION.SDK_INT < 34 || manager.canUseFullScreenIntent())) builder.setFullScreenIntent(open, true)
        runCatching { manager.notify(entry.id, ID, builder.build()) }
    }
    fun cancel(context: Context, id: String) { context.getSystemService(NotificationManager::class.java).cancel(id, ID) }
}
