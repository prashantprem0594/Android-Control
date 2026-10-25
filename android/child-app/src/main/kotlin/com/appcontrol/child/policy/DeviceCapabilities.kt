package com.appcontrol.child.policy

import android.app.AlarmManager
import android.app.admin.DevicePolicyManager
import android.app.role.RoleManager
import android.content.Context
import android.net.VpnService
import android.os.Build
import org.json.JSONObject

object DeviceCapabilities {
    fun snapshot(context: Context): JSONObject = JSONObject()
        .put("androidApi", Build.VERSION.SDK_INT)
        .put("deviceOwner", context.getSystemService(DevicePolicyManager::class.java).isDeviceOwnerApp(context.packageName))
        .put("dialerRole", context.getSystemService(RoleManager::class.java).isRoleHeld(RoleManager.ROLE_DIALER))
        .put("vpnConsent", VpnService.prepare(context) == null)
        .put("exactAlarms", context.getSystemService(AlarmManager::class.java).canScheduleExactAlarms())
}
