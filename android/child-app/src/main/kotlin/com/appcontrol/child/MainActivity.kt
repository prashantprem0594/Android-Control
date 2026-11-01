package com.appcontrol.child

import android.app.Activity
import android.app.AlarmManager
import android.app.NotificationManager
<<<<<<< HEAD
import com.appcontrol.child.calls.DialerSupport
=======
import android.app.role.RoleManager
>>>>>>> origin/main
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
<<<<<<< HEAD
import android.widget.EditText
import com.appcontrol.child.network.DeviceEnrollmentClient
import com.appcontrol.child.network.DeviceRecoveryClient
=======
import android.view.Gravity
import android.widget.EditText
import com.appcontrol.child.network.DeviceEnrollmentClient
>>>>>>> origin/main
import com.appcontrol.child.commands.CommandSync
import com.appcontrol.child.sync.SyncScheduler
import com.appcontrol.child.policy.PolicyEnforcer
import com.appcontrol.child.calls.DialActivity
import android.widget.Button
<<<<<<< HEAD
=======
import android.widget.LinearLayout
>>>>>>> origin/main
import android.widget.TextView
import android.widget.Toast
import android.widget.ScrollView
import com.appcontrol.child.admin.AppControlDeviceAdminReceiver
<<<<<<< HEAD
import com.appcontrol.child.remote.RemoteScreenShareService
import com.appcontrol.child.location.VisibleLocationShareService
import com.appcontrol.child.recording.ConsentedRecorderService
import com.appcontrol.transport.LocalNetworkAccess
import com.appcontrol.transport.AppAppearance
import java.time.Instant
=======
>>>>>>> origin/main

class MainActivity : Activity() {
    private lateinit var devicePolicyManager: DevicePolicyManager
    private lateinit var adminComponent: ComponentName
    private lateinit var status: TextView
<<<<<<< HEAD
    private var recorderApprovalPending = false

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        AppAppearance.prepare(this)
        window.addFlags(android.view.WindowManager.LayoutParams.FLAG_SECURE)
=======

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
>>>>>>> origin/main

        devicePolicyManager = getSystemService(Context.DEVICE_POLICY_SERVICE) as DevicePolicyManager
        adminComponent = ComponentName(this, AppControlDeviceAdminReceiver::class.java)

<<<<<<< HEAD
        status = TextView(this).apply { AppAppearance.status(this) }

        val lockButton = Button(this).apply {
            text = "Lock device"
=======
        status = TextView(this).apply {
            textSize = 20f
            gravity = Gravity.CENTER
        }

        val lockButton = Button(this).apply {
            text = "Lock device (feasibility test)"
>>>>>>> origin/main
            setOnClickListener { lockDeviceIfOwned() }
        }

        val serverInput = EditText(this).apply {
<<<<<<< HEAD
            isSaveEnabled = false
=======
>>>>>>> origin/main
            hint = "HTTPS server URL"
            inputType = 17
            if (BuildConfig.SERVER_ORIGIN.isNotEmpty()) {
                setText(BuildConfig.SERVER_ORIGIN)
                isEnabled = false
            }
        }
<<<<<<< HEAD
        val codeInput = EditText(this).apply { hint = "One-time pairing code"; inputType = 129; isSaveEnabled = false }
        val nameInput = EditText(this).apply { hint = "Device name"; isSaveEnabled = false }
        val enrollmentStatus = AppAppearance.text(this, "Use a one-time code from the parent app to pair this device.", 14f, false)
=======
        val codeInput = EditText(this).apply { hint = "One-time pairing code"; inputType = 129 }
        val nameInput = EditText(this).apply { hint = "Device name" }
        val enrollmentStatus = TextView(this)
>>>>>>> origin/main
        val enrollButton = Button(this).apply {
            text = "Enroll with parent"
            setOnClickListener {
                val server = serverInput.text.toString()
<<<<<<< HEAD
                if (LocalNetworkAccess.neededForOrigin(this@MainActivity, server)) {
                    enrollmentStatus.text = "Allow home-network server access, then enroll again."
                    LocalNetworkAccess.request(this@MainActivity, 4437)
                    return@setOnClickListener
                }
=======
>>>>>>> origin/main
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
<<<<<<< HEAD
                            refreshDeviceStatus()
=======
>>>>>>> origin/main
                        }
                    }
                }.start()
            }
        }
<<<<<<< HEAD
        val reconnectButton = Button(this).apply {
            text = "Reconnect to parent"
            setOnClickListener {
                if (getSharedPreferences("enrollment", MODE_PRIVATE).getString("deviceId", null) != null) {
                    enrollmentStatus.text = "This phone is already connected to its parent."
                    return@setOnClickListener
                }
                val server = serverInput.text.toString()
                if (LocalNetworkAccess.neededForOrigin(this@MainActivity, server)) {
                    enrollmentStatus.text = "Allow home-network server access, then reconnect again."
                    LocalNetworkAccess.request(this@MainActivity, 4437)
                    return@setOnClickListener
                }
                isEnabled = false
                enrollButton.isEnabled = false
                enrollmentStatus.text = "Creating a secure reconnection request..."
                Thread {
                    val result = runCatching {
                        val client = DeviceRecoveryClient(applicationContext)
                        val shouldStop = { isDestroyed || isFinishing || Thread.currentThread().isInterrupted }
                        val recovery = client.start(server, shouldStop)
                        client.prove(recovery, shouldStop)
                        runOnUiThread {
                            if (!isDestroyed) enrollmentStatus.text =
                                "Request sent. Approve Reconnect in the parent app; this screen will update automatically."
                        }
                        var finalStatus: DeviceRecoveryClient.Status = DeviceRecoveryClient.Status.AwaitingParent
                        while (Instant.now().isBefore(recovery.expiresAt) && !shouldStop()) {
                            finalStatus = client.status(recovery, shouldStop)
                            if (finalStatus !is DeviceRecoveryClient.Status.AwaitingParent) break
                            Thread.sleep(2_000)
                        }
                        when (val completed = finalStatus) {
                            is DeviceRecoveryClient.Status.Approved -> {
                                client.saveApproved(recovery, completed.deviceId)
                                runCatching { SyncScheduler.ensureScheduled(applicationContext) }
                                runCatching { CommandSync(applicationContext).sync() }
                                "Reconnected to the existing parent device."
                            }
                            DeviceRecoveryClient.Status.Rejected -> "The parent rejected this reconnection request."
                            DeviceRecoveryClient.Status.Expired,
                            DeviceRecoveryClient.Status.AwaitingParent -> "The reconnection request expired. Try again."
                        }
                    }
                    runOnUiThread {
                        if (!isDestroyed) {
                            isEnabled = true
                            enrollButton.isEnabled = true
                            enrollmentStatus.text = result.getOrElse {
                                "Reconnection failed. Check the server and try again, or use a new pairing code."
                            }
                            refreshDeviceStatus()
                        }
                    }
                }.start()
            }
        }
        val syncStatus = AppAppearance.text(this, "Sync to check for the latest parent commands and schedules.", 14f, false)
        val dialerButton = Button(this).apply {
            text = "Set up phone role"
            setOnClickListener {
                val request = DialerSupport.requestIntent(this@MainActivity)
                if (request == null) {
                    Toast.makeText(this@MainActivity, "Phone role is unavailable on this device.", Toast.LENGTH_LONG).show()
                } else if (!DialerSupport.isHeld(this@MainActivity)) {
                    startActivityForResult(request, 4400)
=======
        val syncStatus = TextView(this)
        val dialerButton = Button(this).apply {
            text = "Set up phone role (feasibility)"
            setOnClickListener {
                val roles = getSystemService(RoleManager::class.java)
                if (!roles.isRoleAvailable(RoleManager.ROLE_DIALER)) {
                    Toast.makeText(this@MainActivity, "Phone role is unavailable on this device.", Toast.LENGTH_LONG).show()
                } else if (!roles.isRoleHeld(RoleManager.ROLE_DIALER)) {
                    startActivityForResult(roles.createRequestRoleIntent(RoleManager.ROLE_DIALER), 4400)
>>>>>>> origin/main
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
<<<<<<< HEAD
                if (Build.VERSION.SDK_INT < 31 || getSystemService(AlarmManager::class.java).canScheduleExactAlarms()) {
=======
                if (getSystemService(AlarmManager::class.java).canScheduleExactAlarms()) {
>>>>>>> origin/main
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
<<<<<<< HEAD
                    if (Build.VERSION.SDK_INT >= 33 && checkSelfPermission(Manifest.permission.POST_NOTIFICATIONS) != PackageManager.PERMISSION_GRANTED) {
=======
                    if (checkSelfPermission(Manifest.permission.POST_NOTIFICATIONS) != PackageManager.PERMISSION_GRANTED) {
>>>>>>> origin/main
                        requestPermissions(arrayOf(Manifest.permission.POST_NOTIFICATIONS), 4202)
                    } else requestVpnConsent()
                }
            }
        }
<<<<<<< HEAD
        val remoteAssistanceButton = Button(this).apply {
            text = "Enable remote assistance"
            setOnClickListener {
                Toast.makeText(this@MainActivity, "Android will show what this service can do. Enable it only for the parent account you trust.", Toast.LENGTH_LONG).show()
                startActivity(Intent(Settings.ACTION_ACCESSIBILITY_SETTINGS))
            }
        }
        val startScreenShareButton = Button(this).apply {
            text = "Start screen sharing"
            setOnClickListener {
                if (Build.VERSION.SDK_INT < 29) {
                    Toast.makeText(this@MainActivity, "Live screen sharing requires Android 10 or newer.", Toast.LENGTH_LONG).show()
                    return@setOnClickListener
                }
                Toast.makeText(this@MainActivity, "Android will ask you to approve sharing this screen. A visible notification lets you stop at any time.", Toast.LENGTH_LONG).show()
                startActivityForResult(getSystemService(android.media.projection.MediaProjectionManager::class.java).createScreenCaptureIntent(), SCREEN_SHARE_REQUEST)
            }
        }
        val stopScreenShareButton = Button(this).apply {
            text = "Stop screen sharing"
            setOnClickListener { RemoteScreenShareService.stop(this@MainActivity) }
        }
        val startLocationShareButton = Button(this).apply {
            text = "Start location sharing"
            setOnClickListener {
                if (getSharedPreferences("enrollment", MODE_PRIVATE).getString("deviceId", null) == null) {
                    Toast.makeText(this@MainActivity, "Pair this phone with the parent first.", Toast.LENGTH_LONG).show()
                    return@setOnClickListener
                }
                val permissions = arrayOf(Manifest.permission.ACCESS_FINE_LOCATION, Manifest.permission.ACCESS_COARSE_LOCATION)
                    .filter { checkSelfPermission(it) != PackageManager.PERMISSION_GRANTED }
                if (permissions.isNotEmpty()) {
                    Toast.makeText(this@MainActivity, "Android will ask before location sharing starts. A persistent notification lets you stop it at any time.", Toast.LENGTH_LONG).show()
                    requestPermissions(permissions.toTypedArray(), LOCATION_SHARE_REQUEST)
                } else VisibleLocationShareService.start(this@MainActivity)
            }
        }
        val stopLocationShareButton = Button(this).apply {
            text = "Stop location sharing"
            setOnClickListener { VisibleLocationShareService.stop(this@MainActivity) }
        }
        val approveRecorderButton = Button(this).apply {
            text = "Allow recorder control for 15 minutes"
            setOnClickListener {
                android.app.AlertDialog.Builder(this@MainActivity)
                    .setTitle("Allow App Control recorder?")
                    .setMessage("For the next 15 minutes, the paired parent may start or stop this app's microphone recorder. Android will show its microphone indicator whenever recording is on. Audio stays on this phone and no other recorder can be controlled. You can stop immediately from the permanent notification.")
                    .setNegativeButton("Cancel", null)
                    .setPositiveButton("Allow for 15 minutes") { _, _ ->
                        if (checkSelfPermission(Manifest.permission.RECORD_AUDIO) == PackageManager.PERMISSION_GRANTED) {
                            ConsentedRecorderService.approve(this@MainActivity)
                        } else {
                            recorderApprovalPending = true
                            requestPermissions(arrayOf(Manifest.permission.RECORD_AUDIO), RECORDER_PERMISSION_REQUEST)
                        }
                    }.show()
            }
        }
        val stopRecorderButton = Button(this).apply {
            text = "Stop recorder and end control session"
            setOnClickListener { ConsentedRecorderService.end(this@MainActivity) }
        }
        val syncButton = Button(this).apply {
            text = "Sync parent commands"
            setOnClickListener {
                val origin = getSharedPreferences("enrollment", MODE_PRIVATE).getString("server", "") ?: ""
                if (LocalNetworkAccess.neededForOrigin(this@MainActivity, origin)) {
                    syncStatus.text = "Allow home-network server access, then sync again."
                    LocalNetworkAccess.request(this@MainActivity, 4437)
                    return@setOnClickListener
                }
=======
        val syncButton = Button(this).apply {
            text = "Sync parent commands"
            setOnClickListener {
>>>>>>> origin/main
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
<<<<<<< HEAD
        listOf(serverInput, codeInput, nameInput).forEach { AppAppearance.styleField(it) }
        listOf(lockButton, enrollButton, reconnectButton, vpnButton, remoteAssistanceButton, startScreenShareButton, stopScreenShareButton, startLocationShareButton, stopLocationShareButton, approveRecorderButton, stopRecorderButton, alarmButton, dialerButton, callAlertsButton, syncButton)
            .forEach { AppAppearance.styleButton(it, false) }
        AppAppearance.styleButton(dialPadButton, true)

        val content = AppAppearance.column(this).apply {
            addView(AppAppearance.text(this@MainActivity, "FAMILY DEVICE", 12f, true).apply {
                setTextColor(android.graphics.Color.parseColor("#315BDB"))
                letterSpacing = 0.12f
            })
            addView(AppAppearance.text(this@MainActivity, "Your family phone", 28f, true))
            addView(AppAppearance.text(this@MainActivity, "Calls and family settings, together in one place.", 15f, false))
            addView(AppAppearance.card(this@MainActivity).apply {
                addView(AppAppearance.text(this@MainActivity, "Device status", 17f, true))
                addView(status)
            })
            addView(AppAppearance.card(this@MainActivity).apply {
                addView(AppAppearance.text(this@MainActivity, "Stay in touch", 21f, true))
                addView(AppAppearance.text(this@MainActivity, "Open the phone to call your parent or choose a contact.", 14f, false))
                addView(dialPadButton)
            })
            addView(AppAppearance.card(this@MainActivity).apply {
                addView(AppAppearance.text(this@MainActivity, "Connect to your parent", 20f, true))
                addView(AppAppearance.text(this@MainActivity, "Pairing links this phone to the parent account. Device Owner setup is separate.", 14f, false))
                addView(serverInput)
                addView(codeInput)
                addView(nameInput)
                addView(enrollButton)
                addView(reconnectButton)
                addView(AppAppearance.text(this@MainActivity, "After reinstalling on this same phone, reconnect preserves the existing device and policies after parent approval. Pairing remains available as a fallback.", 13f, false))
                addView(enrollmentStatus)
            })
            addView(AppAppearance.card(this@MainActivity).apply {
                addView(AppAppearance.text(this@MainActivity, "Phone & call setup", 20f, true))
                addView(AppAppearance.text(this@MainActivity, "Choose this app as the phone app and review incoming-call access.", 14f, false))
                addView(dialerButton)
                addView(callAlertsButton)
            })
            addView(AppAppearance.card(this@MainActivity).apply {
                addView(AppAppearance.text(this@MainActivity, "Parent controls", 20f, true))
                addView(AppAppearance.text(this@MainActivity, "Managed controls require Device Owner setup. Permissions below support internet pause and offline schedules.", 14f, false))
                addView(vpnButton)
                addView(AppAppearance.text(this@MainActivity, "Screen sharing lets the approved parent view the visible screen during a session you start here. A persistent Android notification always shows while sharing is active.", 14f, false))
                addView(startScreenShareButton)
                addView(stopScreenShareButton)
                addView(AppAppearance.text(this@MainActivity, "Location sharing starts only from this phone, after Android permission approval. The paired parent can view recent shared locations for up to 12 hours. A permanent notification always shows while it is active.", 14f, false))
                addView(startLocationShareButton)
                addView(stopLocationShareButton)
                addView(AppAppearance.text(this@MainActivity, "Recorder control is off unless you approve a 15-minute session here. The parent can only start or stop App Control's own recorder during that session. A permanent notification and Android microphone indicator are always visible while it records; audio remains on this phone.", 14f, false))
                addView(approveRecorderButton)
                addView(stopRecorderButton)
                addView(AppAppearance.text(this@MainActivity, "Remote gestures are separate and require Android Accessibility access.", 14f, false))
                addView(remoteAssistanceButton)
                addView(alarmButton)
                addView(lockButton)
            })
            addView(AppAppearance.card(this@MainActivity).apply {
                addView(AppAppearance.text(this@MainActivity, "Sync & maintenance", 20f, true))
                addView(syncButton)
                addView(syncStatus)
                if (Build.VERSION.SDK_INT >= 33) addView(Button(this@MainActivity).apply {
                    text = "Allow parent app requests"
                    AppAppearance.styleButton(this, false)
                    setOnClickListener {
                        if (checkSelfPermission(Manifest.permission.POST_NOTIFICATIONS) == PackageManager.PERMISSION_GRANTED) {
                            Toast.makeText(this@MainActivity, "Parent app request notifications are allowed.", Toast.LENGTH_LONG).show()
                        } else requestPermissions(arrayOf(Manifest.permission.POST_NOTIFICATIONS), 4203)
                    }
                })
                if (LocalNetworkAccess.supported(this@MainActivity)) {
                    addView(AppAppearance.text(this@MainActivity, "Using a server on your home network? Allow access before pairing or syncing.", 14f, false))
                    addView(Button(this@MainActivity).apply {
                        text = "Allow home-network server access"
                        AppAppearance.styleButton(this, false)
                        setOnClickListener {
                            if (LocalNetworkAccess.granted(this@MainActivity)) Toast.makeText(this@MainActivity, "Home-network server access is already allowed.", Toast.LENGTH_LONG).show()
                            else LocalNetworkAccess.request(this@MainActivity, 4437)
                        }
                    })
                }
                if (BuildConfig.UPDATE_PUBLIC_KEY.isNotEmpty()) addView(Button(this@MainActivity).apply {
                    text = "App updates"
                    AppAppearance.styleButton(this, false)
                    setOnClickListener { startActivity(Intent(this@MainActivity, UpdateActivity::class.java)) }
                })
            })
=======
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
>>>>>>> origin/main
        }

        setContentView(ScrollView(this).apply { addView(content) })
    }

    override fun onResume() {
        super.onResume()
        runCatching { SyncScheduler.ensureScheduled(applicationContext) }
<<<<<<< HEAD
        refreshDeviceStatus()
    }

    private fun refreshDeviceStatus() {
=======
>>>>>>> origin/main
        val isOwner = devicePolicyManager.isDeviceOwnerApp(packageName)
        status.text = if (isOwner) {
            "Device Owner: active"
        } else {
<<<<<<< HEAD
            "Device Owner: setup needed"
        }
        val paired = getSharedPreferences("enrollment", MODE_PRIVATE).getString("deviceId", null) != null
        status.append(if (paired) "\nParent pairing: saved on this phone" else "\nParent pairing: setup needed")
        status.append(if (DialerSupport.isHeld(this)) "\nPhone role: active" else "\nPhone role: not selected")
        if (LocalNetworkAccess.supported(this)) status.append("\nHome-network server access: " + if (LocalNetworkAccess.granted(this)) "allowed" else "not allowed")
        status.append("\nRecorder control: " + if (ConsentedRecorderService.sessionActive(this)) "approved temporarily" else "not approved")
=======
            "Device Owner: not enrolled"
        }
        status.append(if (getSystemService(RoleManager::class.java).isRoleHeld(RoleManager.ROLE_DIALER)) "\nPhone role: active" else "\nPhone role: not selected")
>>>>>>> origin/main
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
<<<<<<< HEAD
        if (requestCode == SCREEN_SHARE_REQUEST && resultCode == RESULT_OK && data != null) {
            RemoteScreenShareService.start(this, resultCode, data)
            Toast.makeText(this, "Screen sharing started. Keep the Android notification visible and tap Stop when finished.", Toast.LENGTH_LONG).show()
        }
    }

    private fun requestCallPermissions() {
        val missing = (listOf(Manifest.permission.CALL_PHONE, Manifest.permission.READ_PHONE_STATE) +
            if (Build.VERSION.SDK_INT >= 33) listOf(Manifest.permission.POST_NOTIFICATIONS) else emptyList())
=======
    }

    private fun requestCallPermissions() {
        val missing = listOf(Manifest.permission.CALL_PHONE, Manifest.permission.READ_PHONE_STATE, Manifest.permission.POST_NOTIFICATIONS)
>>>>>>> origin/main
            .filter { checkSelfPermission(it) != PackageManager.PERMISSION_GRANTED }
        if (missing.isNotEmpty()) requestPermissions(missing.toTypedArray(), 4402)
    }

    override fun onRequestPermissionsResult(requestCode: Int, permissions: Array<out String>, grantResults: IntArray) {
        super.onRequestPermissionsResult(requestCode, permissions, grantResults)
<<<<<<< HEAD
        if (requestCode == 4437) Toast.makeText(this, if (LocalNetworkAccess.granted(this))
            "Home-network server access is allowed. Retry enrollment or sync."
            else "Home-network server access was not allowed. You can enable it in Android app permissions when needed.", Toast.LENGTH_LONG).show()
        if (requestCode == 4202) requestVpnConsent()
        if (requestCode == LOCATION_SHARE_REQUEST && grantResults.any { it == PackageManager.PERMISSION_GRANTED }) VisibleLocationShareService.start(this)
        if (requestCode == RECORDER_PERMISSION_REQUEST && recorderApprovalPending) {
            recorderApprovalPending = false
            if (grantResults.firstOrNull() == PackageManager.PERMISSION_GRANTED) ConsentedRecorderService.approve(this)
            else Toast.makeText(this, "Microphone permission is required before a recorder session can be approved.", Toast.LENGTH_LONG).show()
        }
=======
        if (requestCode == 4202) requestVpnConsent()
>>>>>>> origin/main
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

<<<<<<< HEAD
    companion object {
        private const val VPN_REQUEST = 4200
        private const val SCREEN_SHARE_REQUEST = 4204
        private const val LOCATION_SHARE_REQUEST = 4205
        private const val RECORDER_PERMISSION_REQUEST = 4206
    }
=======
    companion object { private const val VPN_REQUEST = 4200 }
>>>>>>> origin/main
}



