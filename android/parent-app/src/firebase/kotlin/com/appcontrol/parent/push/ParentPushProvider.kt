package com.appcontrol.parent.push

import android.content.Context
import android.os.Looper
import android.os.SystemClock
import com.google.firebase.messaging.FirebaseMessaging

/** SDK owns token persistence. Null means transient unavailability, not unregister. */
object ParentPushProvider {
    fun enabled(): Boolean = true

    fun token(context: Context, shouldStop: () -> Boolean): String? {
        check(Looper.myLooper() != Looper.getMainLooper()) { "Parent sync worker required" }
        check(!shouldStop() && !Thread.currentThread().isInterrupted) { "Parent sync stopped" }
        val task = FirebaseMessaging.getInstance().token
        val started = SystemClock.elapsedRealtime()
        while (!task.isComplete) {
            check(!shouldStop() && !Thread.currentThread().isInterrupted) { "Parent sync stopped" }
            check(SystemClock.elapsedRealtime() - started < 10_000) { "Parent routing token unavailable" }
            Thread.sleep(50)
        }
        check(!shouldStop() && !Thread.currentThread().isInterrupted) { "Parent sync stopped" }
        if (!task.isSuccessful) return null
        val token = task.result ?: return null
        require(token.length in 1..4096 && token.all { it.code in 33..126 }) { "Invalid parent routing token" }
        return token
    }
}
