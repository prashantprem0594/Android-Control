package com.appcontrol.parent.push

import android.content.Context

/** Ordinary parent builds include no Firebase SDK or routing-token registration. */
object ParentPushProvider {
    fun enabled(): Boolean = false

    fun token(context: Context, shouldStop: () -> Boolean): String? {
        check(!shouldStop() && !Thread.currentThread().isInterrupted) { "Parent sync stopped" }
        return null
    }
}
