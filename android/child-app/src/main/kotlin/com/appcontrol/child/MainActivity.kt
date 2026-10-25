package com.appcontrol.child

import android.app.Activity
import android.app.AlarmManager
import android.app.NotificationManager
import android.app.role.RoleManager
import android.app.admin.DevicePolicyManager
import android.content.ComponentName
import android.content.Context
import android.content.Intent
import android.content.pm.PackageManager
import android.net.VpnService
import android.net.Uri
import android.provider.Settings
import android.Manifest
import android.os.Bundle
import android.os.Build
import android.view.Gravity
import android.widget.EditText
import com.appcontrol.child.network.DeviceEnrollmentClient
import com.appcontrol.child.commands.CommandSync
import com.appcontrol.child.sync.SyncScheduler
import com.appcontrol.child.policy.PolicyEnforcer
import com.appcontrol.child.calls.DialActivity
import android.widget.Button
import android.widget.LinearLayout
import android.widget.TextView
import android.widget.Toast
import android.widget.ScrollView
import com.appcontrol.child.admin.AppControlDeviceAdminReceiver

class MainActivity : Activity() {
    private lateinit var devicePolicyManager: DevicePolicyManager
    private lateinit var adminComponent: ComponentName
    private lateinit var status: TextView

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)

        devicePolicyManager = getSystemService(Context.DEVICE_POLICY_SERVICE) as DevicePolicyManager
        adminComponent = ComponentName(this, AppControlDeviceAdminReceiver::class.java)

        status = TextView(this).apply {
            textSize = 20f
            gravity = Gravity.CENTER
        }

        val lockButton = Button(this).apply {
            text = "Lock device (feasibility test)"
            setOnClickListener { lockDeviceIfOwned() }
        }

        val serverInput = EditText(this).apply {
            hint = "HTTPS server URL"
            inputType = 17
            if (BuildConfig.SERVER_ORIGIN.isNotEmpty()) {
                setText(BuildConfig.SERVER_ORIGIN)
                isEnabled = false
            }
        }
        val codeInput = EditText(this).apply { hint = "One-time pairing code"; inputType = 129 }
        val nameInput = EditText(this).apply { hint = "Device name" }
        val enrollmentStatus = TextView(this)
        val enrollButton = Button(this).apply {
            text = "Enroll with parent"
            setOnClickListener {
                val server = serverInput.text.toString()
                val code = codeInput.text.toString()
                val name = nameInput.text.toString()
                codeInput.text.clear()
                isEnabled = false
                enrollmentStatus.text = "Enrolling..."
                Thread {
                    val result = runCatching {
                        val id = DeviceEnrollmentClient(applicationContext).enroll(server, code, name)
                        runCatching { SyncScheduler.ensureScheduled(applicationContext) }
                        id
                    }
                    runOnUiThread {
                        if (!isDestroyed) {
                            isEnabled = true
                            enrollmentStatus.text = if (result.isSuccess) "Enrollment confirmed" else
                                "Enrollment failed. Check the server and use a fresh pairing code."
                        }
                    }
                }.start()
            }
        }
        val syncStatus = TextView(this)
        val dialerButton = Button(this).apply {
            text = "Set up phone role (feasibility)"
            setOnClickListener {
                val roles = getSystemService(RoleManager::class.java)
                if (!roles.isRoleAvailable(RoleManager.ROLE_DIALER)) {
                    Toast.makeText(this@MainActivity, "Phone role is unavailable on this device.", Toast.LENGTH_LONG).show()
                } else if (!roles.isRoleHeld(RoleManager.ROLE_DIALER)) {
                    startActivityForResult(roles.createRequestRoleIntent(RoleManager.ROLE_DIALER), 4400)
                } else requestCallPermissions()
            }
        }
        val dialPadButton = Button(this).apply {
            text = "Open phone"
            setOnClickListener { startActivity(Intent(this@MainActivity, DialActivity::class.java)) }
        }
        val callAlertsButton = Button(this).apply {
            text = "Allow lock-screen call alerts"
            setOnClickListener {
                if (Build.VERSION.SDK_INT >= 34 && !getSystemService(NotificationManager::class.java).canUseFullScreenIntent()) {
                    Toast.makeText(this@MainActivity, "Allow full-screen alerts for incoming calls on the lock screen.", Toast.LENGTH_LONG).show()
                    runCatching { startActivity(Intent(Settings.ACTION_MANAGE_APP_USE_FULL_SCREEN_INTENT, Uri.parse("package:$packageName"))) }
                } else Toast.makeText(this@MainActivity, "Lock-screen call alert access is available.", Toast.LENGTH_LONG).show()
            }
        }
        val alarmButton = Button(this).apply {
            text = "Allow precise schedule timing"
            setOnClickListener {
                if (getSystemService(AlarmManager::class.java).canScheduleExactAlarms()) {
                    Toast.makeText(this@MainActivity, "Precise schedule timing is allowed.", Toast.LENGTH_LONG).show()
                } else {
                    Toast.makeText(this@MainActivity, "Allow alarms so offline controls can change at scheduled times.", Toast.LENGTH_LONG).show()
                    runCatching { startActivity(Intent(Settings.ACTION_REQUEST_SCHEDULE_EXACT_ALARM, Uri.parse("package:$packageName"))) }
                }
            }
        }
        val vpnButton = Button(this).apply {
            text = "Allow internet pause"
            setOnClickListener {
                if (!devicePolicyManager.isDeviceOwnerApp(packageName)) {
                    Toast.makeText(this@MainActivity, "Complete Device Owner setup first.", Toast.LENGTH_LONG).show()
                } else {
                    if (checkSelfPermission(Manifest.permission.POST_NOTIFICATIONS) != PackageManager.PERMISSION_GRANTED) {
                        requestPermissions(arrayOf(Manifest.permission.POST_NOTIFICATIONS), 4202)
                    } else requestVpnConsent()
                }
            }
        }
        val syncButton = Button(this).apply {
            text = "Sync parent commands"
            setOnClickListener {
                isEnabled = false
                syncStatus.text = "Syncing..."
                Thread {
                    val result = runCatching { CommandSync(applicationContext).sync() }
                    if (result.getOrNull()?.retryRequired == true) {
                        runCatching { SyncScheduler.requestSync(applicationContext) }
                    }
                    runOnUiThread {
                        if (!isDestroyed) {
                            isEnabled = true
                            syncStatus.text = result.fold(
                                { "Sync complete: ${it.acknowledged} results sent, ${it.expired} expired commands skipped. Policy ${it.downloadedPolicyVersion}: ${it.policyResult}." },
                                { "Sync failed. Check enrollment and connectivity, then retry." },
                            )
                        }
                    }
                }.start()
            }
        }
        val content = LinearLayout(this).apply {
            gravity = Gravity.CENTER
            orientation = LinearLayout.VERTICAL
            setPadding(48, 48, 48, 48)
            addView(status)
            if (BuildConfig.UPDATE_PUBLIC_KEY.isNotEmpty()) addView(Button(this@MainActivity).apply {
                text = "App updates"
                setOnClickListener { startActivity(Intent(this@MainActivity, UpdateActivity::class.java)) }
            })
            addView(lockButton)
            addView(serverInput)
            addView(codeInput)
            addView(nameInput)
            addView(enrollButton)
            addView(enrollmentStatus)
            addView(vpnButton)
            addView(alarmButton)
            addView(dialerButton)
            addView(callAlertsButton)
            addView(dialPadButton)
            addView(syncButton)
            addView(syncStatus)
        }

        setContentView(ScrollView(this).apply { addView(content) })
    }

    override fun onResume() {
        super.onResume()
        runCatching { SyncScheduler.ensureScheduled(applicationContext) }
        val isOwner = devicePolicyManager.isDeviceOwnerApp(packageName)
        status.text = if (isOwner) {
            "Device Owner: active"
        } else {
            "Device Owner: not enrolled"
        }
        status.append(if (getSystemService(RoleManager::class.java).isRoleHeld(RoleManager.ROLE_DIALER)) "\nPhone role: active" else "\nPhone role: not selected")
    }

    private fun lockDeviceIfOwned() {
        if (!devicePolicyManager.isDeviceOwnerApp(packageName)) {
            Toast.makeText(this, "Enroll this app as Device Owner first.", Toast.LENGTH_LONG).show()
            return
        }

        devicePolicyManager.lockNow()
    }

    override fun onActivityResult(requestCode: Int, resultCode: Int, data: Intent?) {
        super.onActivityResult(requestCode, resultCode, data)
        if (requestCode == VPN_REQUEST && resultCode == RESULT_OK) applyCachedPolicy()
        if (requestCode == 4400 && resultCode == RESULT_OK) requestCallPermissions()
    }

    private fun requestCallPermissions() {
        val missing = listOf(Manifest.permission.CALL_PHONE, Manifest.permission.READ_PHONE_STATE, Manifest.permission.POST_NOTIFICATIONS)
            .filter { checkSelfPermission(it) != PackageManager.PERMISSION_GRANTED }
        if (missing.isNotEmpty()) requestPermissions(missing.toTypedArray(), 4402)
    }

    override fun onRequestPermissionsResult(requestCode: Int, permissions: Array<out String>, grantResults: IntArray) {
        super.onRequestPermissionsResult(requestCode, permissions, grantResults)
        if (requestCode == 4202) requestVpnConsent()
    }

    private fun requestVpnConsent() {
        val permission = VpnService.prepare(this)
        if (permission != null) startActivityForResult(permission, VPN_REQUEST)
        else applyCachedPolicy()
    }

    private fun applyCachedPolicy() {
        Thread { runCatching { PolicyEnforcer(applicationContext).applyCached() } }.start()
        Toast.makeText(this, "Internet pause permission is ready. Sync to send current policy status.", Toast.LENGTH_LONG).show()
    }

    companion object { private const val VPN_REQUEST = 4200 }
}



