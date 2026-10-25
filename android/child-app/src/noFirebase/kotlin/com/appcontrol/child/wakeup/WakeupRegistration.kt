package com.appcontrol.child.wakeup

/** Polling-only builds have no Firebase SDK or routing registration. */
object WakeupRegistration {
    fun token(shouldStop: () -> Boolean): String? {
        check(!shouldStop()) { "Sync stopped" }
        return null
    }
}
