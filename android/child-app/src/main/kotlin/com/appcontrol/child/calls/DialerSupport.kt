package com.appcontrol.child.calls

import android.app.role.RoleManager
import android.content.Context
import android.content.Intent
import android.os.Build
import android.telecom.TelecomManager

/** Uses Android 9's system-confirmed default dialer flow on older phones. */
object DialerSupport {
    fun isHeld(context: Context): Boolean =
        context.getSystemService(TelecomManager::class.java)?.defaultDialerPackage == context.packageName

    fun requestIntent(context: Context): Intent? {
        if (Build.VERSION.SDK_INT >= 29) {
            val roles = context.getSystemService(RoleManager::class.java)
            return if (roles.isRoleAvailable(RoleManager.ROLE_DIALER))
                roles.createRequestRoleIntent(RoleManager.ROLE_DIALER) else null
        }
        return Intent(TelecomManager.ACTION_CHANGE_DEFAULT_DIALER)
            .putExtra(TelecomManager.EXTRA_CHANGE_DEFAULT_DIALER_PACKAGE_NAME, context.packageName)
            .takeIf { it.resolveActivity(context.packageManager) != null }
    }
}
