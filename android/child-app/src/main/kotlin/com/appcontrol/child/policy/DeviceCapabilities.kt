package com.appcontrol.child.policy

import android.app.AlarmManager
import android.app.admin.DevicePolicyManager
<<<<<<< HEAD
import com.appcontrol.child.calls.DialerSupport
import android.content.Context
import android.net.ConnectivityManager
import android.net.NetworkCapabilities
=======
import android.app.role.RoleManager
import android.content.Context
>>>>>>> origin/main
import android.net.VpnService
import android.os.Build
import org.json.JSONObject

object DeviceCapabilities {
<<<<<<< HEAD
    fun snapshot(context: Context): JSONObject {
        val connectivityManager = context.getSystemService(ConnectivityManager::class.java)
        val networkCapabilities = connectivityManager.allNetworks.mapNotNull(connectivityManager::getNetworkCapabilities)
        fun hasInternet(transport: Int? = null) = networkCapabilities.any { capabilities ->
            capabilities.hasCapability(NetworkCapabilities.NET_CAPABILITY_INTERNET) &&
                (transport == null || capabilities.hasTransport(transport))
        }
        return JSONObject()
            .put("androidApi", Build.VERSION.SDK_INT)
            .put("deviceOwner", context.getSystemService(DevicePolicyManager::class.java).isDeviceOwnerApp(context.packageName))
            .put("dialerRole", DialerSupport.isHeld(context))
            .put("vpnConsent", VpnService.prepare(context) == null)
            .put("exactAlarms", Build.VERSION.SDK_INT < 31 || context.getSystemService(AlarmManager::class.java).canScheduleExactAlarms())
            .put("networkConnected", hasInternet())
            .put("wifiConnected", hasInternet(NetworkCapabilities.TRANSPORT_WIFI))
            .put("cellularConnected", hasInternet(NetworkCapabilities.TRANSPORT_CELLULAR))
            .put("vpnActive", networkCapabilities.any { it.hasTransport(NetworkCapabilities.TRANSPORT_VPN) })
    }
=======
    fun snapshot(context: Context): JSONObject = JSONObject()
        .put("androidApi", Build.VERSION.SDK_INT)
        .put("deviceOwner", context.getSystemService(DevicePolicyManager::class.java).isDeviceOwnerApp(context.packageName))
        .put("dialerRole", context.getSystemService(RoleManager::class.java).isRoleHeld(RoleManager.ROLE_DIALER))
        .put("vpnConsent", VpnService.prepare(context) == null)
        .put("exactAlarms", context.getSystemService(AlarmManager::class.java).canScheduleExactAlarms())
>>>>>>> origin/main
}
