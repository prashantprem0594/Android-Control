package com.appcontrol.child.commands

import android.app.Activity
import android.content.Intent
import android.os.Bundle
import android.provider.Settings
import android.widget.Toast

/** Notification tap gate: the parent request must still be current when acted on. */
class LaunchRequestActivity : Activity() {
    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        val expiresAt = intent.getLongExtra(EXTRA_EXPIRES_AT, 0L)
        if (expiresAt <= System.currentTimeMillis()) {
            Toast.makeText(this, "This parent request has expired. Ask them to send it again.", Toast.LENGTH_LONG).show()
            finish()
            return
        }
        val packageName = intent.getStringExtra(EXTRA_PACKAGE)
        val destination = if (packageName == null) Intent(Settings.ACTION_SETTINGS) else runCatching {
            require(packageName.matches(Regex("[A-Za-z_][A-Za-z0-9_]*(\\.[A-Za-z_][A-Za-z0-9_]*)+")))
            packageManager.getLaunchIntentForPackage(packageName) ?: error("App is no longer available")
        }.getOrElse {
            Toast.makeText(this, "That app is no longer available to open.", Toast.LENGTH_LONG).show()
            finish()
            return
        }
        runCatching { startActivity(destination.addFlags(Intent.FLAG_ACTIVITY_NEW_TASK)) }
            .onFailure { Toast.makeText(this, "Android could not open that screen.", Toast.LENGTH_LONG).show() }
        finish()
    }

    companion object {
        const val EXTRA_PACKAGE = "requested_package"
        const val EXTRA_EXPIRES_AT = "request_expires_at"
    }
}
