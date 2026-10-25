package com.appcontrol.child.wakeup

import android.os.Looper
import com.google.firebase.messaging.FirebaseMessaging

/** SDK owns token persistence; this app holds only the current in-memory value. */
object WakeupRegistration {
    fun token(shouldStop: () -> Boolean): String? {
        check(Looper.myLooper() != Looper.getMainLooper()) { "Worker required" }
        check(!shouldStop()) { "Sync stopped" }
        val task = FirebaseMessaging.getInstance().token
        val started = System.nanoTime()
        while (!task.isComplete) {
            check(!shouldStop() && !Thread.currentThread().isInterrupted) { "Sync stopped" }
            check(System.nanoTime() - started < 10_000_000_000L) { "Routing token unavailable" }
            Thread.sleep(50)
        }
        check(!shouldStop() && !Thread.currentThread().isInterrupted) { "Sync stopped" }
        if (!task.isSuccessful) return null
        val token = task.result ?: return null
        require(token.length in 1..4096 && token.all { it.code in 33..126 }) { "Invalid routing token" }
        return token
    }
}
