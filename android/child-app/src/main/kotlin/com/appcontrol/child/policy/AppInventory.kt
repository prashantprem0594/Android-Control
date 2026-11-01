package com.appcontrol.child.policy

import android.content.Context
import android.content.pm.ApplicationInfo
import org.json.JSONArray
import org.json.JSONObject

class AppInventory(context: Context) {
    private val app = context.applicationContext
    fun snapshot(): JSONArray {
        val manager = app.packageManager
        val enforcer = PolicyEnforcer(app)
        val installed = manager.getInstalledApplications(0).sortedBy { it.packageName }
        require(installed.size <= 2000) { "App inventory too large" }
        val result = JSONArray()
        for (info in installed) {
            val system = info.flags and (ApplicationInfo.FLAG_SYSTEM or ApplicationInfo.FLAG_UPDATED_SYSTEM_APP) != 0
            val protected = runCatching { enforcer.isProtected(info.packageName) }.getOrDefault(true)
            val label = runCatching { manager.getApplicationLabel(info).toString() }.getOrDefault(info.packageName)
                .filterNot { it.isISOControl() }.take(128)
            result.put(JSONObject().put("packageName", info.packageName).put("label", label)
                .put("system", system).put("blockable", !protected)
<<<<<<< HEAD
                .put("launchable", runCatching { manager.getLaunchIntentForPackage(info.packageName) != null }.getOrDefault(false))
=======
>>>>>>> origin/main
                .put("suspended", info.flags and ApplicationInfo.FLAG_SUSPENDED != 0))
        }
        return result
    }
}
