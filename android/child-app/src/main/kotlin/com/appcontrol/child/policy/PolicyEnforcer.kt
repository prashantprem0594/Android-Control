package com.appcontrol.child.policy

import android.app.admin.DevicePolicyManager
import android.content.ComponentName
import android.content.Context
import android.content.Intent
import android.content.pm.ApplicationInfo
import android.content.pm.PackageManager
import android.provider.Settings
import android.provider.Telephony
import android.telecom.TelecomManager
import com.appcontrol.child.admin.AppControlDeviceAdminReceiver
import org.json.JSONObject
import java.time.Instant

/** Applies the cached snapshot offline; never treats an unsupported control as success. */
class PolicyEnforcer(context: Context) {
    private val app = context.applicationContext
    private val manager = app.getSystemService(DevicePolicyManager::class.java)
    private val admin = ComponentName(app, AppControlDeviceAdminReceiver::class.java)
    private val cache = app.getSharedPreferences("policy-cache", Context.MODE_PRIVATE)
    private val ledger = app.getSharedPreferences("package-reconciliation", Context.MODE_PRIVATE)
    private val packages = app.packageManager

    data class Result(val version: Long, val result: String, val reason: String, val stateKey: String)

    fun applyCached(): Result? = synchronized(policyLock) {
        val raw = cache.getString("desired", null) ?: return@synchronized null
        val policy = JSONObject(raw)
        val version = policy.getLong("version")
        if (version == 0L) { ScheduleAlarm.schedule(app, null); return@synchronized null }
        require(version > 0)
        val effective = PolicyConfig.parse(policy).resolve(Instant.now())
        ScheduleAlarm.schedule(app, effective.nextBoundary)
        if (!manager.isDeviceOwnerApp(app.packageName)) return@synchronized Result(version, "failed", "device_owner_required", effective.stateKey)
        val names = effective.blockedPackages
        val engine = PackagePolicyEngine(object : PackagePolicyEngine.Platform {
            override fun installed(name: String): Boolean = runCatching {
                packages.getApplicationInfo(name, 0)
            }.isSuccess

            override fun protectedPackage(name: String): Boolean = isProtected(name)
            override fun tracked(): Set<String> = ledger.getStringSet("tracked", emptySet())!!.toSet()
            override fun saveTracked(packages: Set<String>): Boolean = ledger.edit().putStringSet("tracked", packages.toSet()).commit()
            override fun setSuspended(name: String, suspended: Boolean): Boolean =
                manager.setPackagesSuspended(admin, arrayOf(name), suspended).isEmpty()
            override fun isSuspended(name: String): Boolean = manager.isPackageSuspended(admin, name)
        })
        val packageReason = runCatching { engine.reconcile(names) }.getOrElse { "package_change_failed" }
        val internetApplied = runCatching { InternetPauseVpn.reconcile(app, version, effective.internetPaused) }.getOrDefault(false)
        val reason = packageReason.ifEmpty { if (!internetApplied) "internet_pause_unavailable" else "" }
        val result = Result(version, if (reason.isEmpty()) "applied" else "partial", reason, effective.stateKey)
        val bootCount = runCatching { Settings.Global.getInt(app.contentResolver, Settings.Global.BOOT_COUNT, -1) }.getOrDefault(-1)
        check(cache.edit().putLong("lastAttemptedVersion", version).putString("localResult", result.result)
            .putString("localReason", reason).putString("localStateKey", effective.stateKey)
            .putInt("localBootCount", bootCount).commit()) { "Could not persist policy result" }
        result
    }

    fun isProtected(name: String): Boolean {
        if (name == "android" || name == app.packageName || name.startsWith("com.android.") ||
            name.startsWith("com.google.android.") || name.startsWith("com.appcontrol.")) return true
        val info = runCatching { packages.getApplicationInfo(name, 0) }.getOrNull() ?: return true
        if (info.flags and (ApplicationInfo.FLAG_SYSTEM or ApplicationInfo.FLAG_UPDATED_SYSTEM_APP) != 0) return true
        val critical = mutableSetOf<String?>()
        manager.activeAdmins?.forEach { critical.add(it.packageName) }
        critical.add(app.getSystemService(TelecomManager::class.java).defaultDialerPackage)
        critical.add(Telephony.Sms.getDefaultSmsPackage(app))
        val keyboard = Settings.Secure.getString(app.contentResolver, Settings.Secure.DEFAULT_INPUT_METHOD)
        critical.add(keyboard?.substringBefore('/'))
        Settings.Secure.getString(app.contentResolver, Settings.Secure.ENABLED_ACCESSIBILITY_SERVICES)
            ?.split(':')?.forEach { critical.add(it.substringBefore('/')) }
        critical.add(manager.getAlwaysOnVpnPackage(admin))
        for (intent in listOf(Intent(Intent.ACTION_MAIN).addCategory(Intent.CATEGORY_HOME), Intent(Intent.ACTION_DIAL),
            Intent("android.intent.action.MANAGE_PERMISSIONS"))) {
            critical.add(packages.resolveActivity(intent, PackageManager.MATCH_DEFAULT_ONLY)?.activityInfo?.packageName)
        }
        if (name in critical) return true
        // Preserve third-party VPNs that may be carrying the management connection.
        return runCatching {
            packages.getPackageInfo(name, PackageManager.GET_SERVICES).services?.any {
                it.permission == "android.permission.BIND_VPN_SERVICE"
            } == true
        }.getOrDefault(true)
    }

    companion object { private val policyLock = Any() }
}
