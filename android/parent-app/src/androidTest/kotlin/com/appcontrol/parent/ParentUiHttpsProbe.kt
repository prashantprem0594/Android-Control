package com.appcontrol.parent

import android.app.Activity
import android.app.Instrumentation
import android.app.Notification
import android.app.NotificationManager
import android.Manifest
import android.content.pm.PackageManager
import android.content.Context
import android.content.Intent
import android.os.Bundle
import android.os.SystemClock
import android.view.View
import android.view.ViewGroup
import android.view.accessibility.AccessibilityNodeInfo
import android.widget.Button
import android.widget.EditText
import android.widget.TextView
import android.widget.Switch
import com.appcontrol.transport.AcceptanceHttps
import com.appcontrol.transport.LocalNetworkAccess
import java.security.KeyStore
import javax.net.ssl.HttpsURLConnection

/** Real production screens and HTTPS; only the side-by-side disposable app is allowed. */
object ParentUiHttpsProbe {
    @JvmStatic
    @JvmOverloads
    fun run(context: Context, instrumentation: Instrumentation, observeWatchWindow: Boolean = false): Bundle {
        val result = Bundle()
        var phase = "guard"
        var activity: Activity? = null
        var ownsSession = false
        var ownsKey = false
        var deviceFixture: ParentUiDeviceFixture? = null
        var observedWindowMillis: Long? = null
        var acceptanceSockets: javax.net.ssl.SSLSocketFactory? = null
        val previousSockets = HttpsURLConnection.getDefaultSSLSocketFactory()
        try {
            check(BuildConfig.DEBUG && context.packageName == "com.appcontrol.parent.acceptance")
            check(android.os.Build.FINGERPRINT.startsWith("google/sdk_gphone64_x86_64/emu64xa:"))
            check(AcceptanceHttps.isConfigured())
            check(context.getSharedPreferences("parent-session", Context.MODE_PRIVATE).all.isEmpty())
            val keys = KeyStore.getInstance("AndroidKeyStore").apply { load(null) }
            ownsKey = !keys.containsAlias("app-control.parent-session")
            ownsSession = true
            acceptanceSockets = AcceptanceHttps.socketFactory()
            HttpsURLConnection.setDefaultSSLSocketFactory(acceptanceSockets)
            phase = "active-device-fixture"
            val fixture = ParentUiDeviceFixture(context)
            deviceFixture = fixture
            val deviceId = fixture.prepare()
            fun launch(): Activity {
                val screen = instrumentation.startActivitySync(
                    Intent(context, ParentActivity::class.java).addFlags(Intent.FLAG_ACTIVITY_NEW_TASK))
                if (observeWatchWindow) instrumentation.runOnMainSync {
                    screen.window.addFlags(android.view.WindowManager.LayoutParams.FLAG_KEEP_SCREEN_ON)
                }
                return screen
            }
            fun views(root: View): List<View> = buildList {
                add(root)
                if (root is ViewGroup) for (i in 0 until root.childCount) addAll(views(root.getChildAt(i)))
            }
            fun await(condition: () -> Boolean) {
                val deadline = SystemClock.elapsedRealtime() + 30_000
                while (SystemClock.elapsedRealtime() < deadline) {
                    var done = false
                    instrumentation.runOnMainSync { done = condition() }
                    if (done) return
                    SystemClock.sleep(100)
                }
                error("Screen condition timed out")
            }
            fun currentViews(): List<View> = views(checkNotNull(activity).window.decorView)
            fun shown(text: String): Boolean = currentViews().filterIsInstance<TextView>().any { it.text.toString() == text }
            fun click(label: String) {
                await { currentViews().filterIsInstance<Button>().firstOrNull { it.text.toString() == label }
                    ?.let { it.isEnabled && it.performClick() } == true }
            }
            fun dialogAction(label: String) {
                val deadline = SystemClock.elapsedRealtime() + 15_000
                while (SystemClock.elapsedRealtime() < deadline) {
                    val root = instrumentation.uiAutomation.rootInActiveWindow
                    val match = root?.findAccessibilityNodeInfosByText(label)?.firstOrNull {
                        it.text?.toString()?.equals(label, ignoreCase = true) == true && it.isVisibleToUser
                    }
                    var target = match
                    while (target != null && !target.isClickable) target = target.parent
                    if (target?.performAction(AccessibilityNodeInfo.ACTION_CLICK) == true) return
                    SystemClock.sleep(100)
                }
                error("Dialog action unavailable")
            }
            fun dialogField(hint: String, value: String) {
                fun find(node: AccessibilityNodeInfo): AccessibilityNodeInfo? {
                    if (node.hintText?.toString() == hint && node.isEditable) return node
                    for (index in 0 until node.childCount) node.getChild(index)?.let { child -> find(child)?.let { return it } }
                    return null
                }
                val deadline = SystemClock.elapsedRealtime() + 15_000
                while (SystemClock.elapsedRealtime() < deadline) {
                    val node = instrumentation.uiAutomation.rootInActiveWindow?.let { find(it) }
                    if (node != null) {
                        val args = Bundle().apply { putCharSequence(AccessibilityNodeInfo.ACTION_ARGUMENT_SET_TEXT_CHARSEQUENCE, value) }
                        check(node.performAction(AccessibilityNodeInfo.ACTION_SET_TEXT, args))
                        return
                    }
                    SystemClock.sleep(100)
                }
                error("Dialog field unavailable")
            }
            fun signIn(password: String) {
                instrumentation.runOnMainSync {
                    val fields = currentViews().filterIsInstance<EditText>()
                    check(fields.size == 3)
                    fields[1].setText("acceptance-owner@example.test")
                    fields[2].setText(password)
                }
                click("Sign in")
                instrumentation.runOnMainSync {
                    check(currentViews().filterIsInstance<EditText>().lastOrNull()?.text?.isEmpty() != false)
                }
            }
            phase = "login-screen"
            activity = launch()
            await { shown("Your family devices") && shown("Sign in") }
            phase = "local-network-consent"
            if (LocalNetworkAccess.supported(context)) {
                check(!LocalNetworkAccess.neededForOrigin(context, "https://localhost:8443"))
                check(!LocalNetworkAccess.neededForOrigin(context, "https://public.example.org"))
                if (!LocalNetworkAccess.granted(context)) {
                    for (origin in listOf("https://10.0.2.2:8443", "https://192.168.1.10", "https://server.local", "https://server.home.arpa", "https://[fd00::1]")) {
                        check(LocalNetworkAccess.neededForOrigin(context, origin))
                    }
                    click("Allow home-network server access")
                    val deadline = SystemClock.elapsedRealtime() + 15_000
                    while (!LocalNetworkAccess.granted(context) && SystemClock.elapsedRealtime() < deadline) {
                        val allow = instrumentation.uiAutomation.rootInActiveWindow?.findAccessibilityNodeInfosByText("Allow")?.firstOrNull {
                            it.text?.toString()?.equals("Allow", true) == true && it.isClickable && it.isVisibleToUser &&
                                it.packageName?.toString() in setOf("com.android.permissioncontroller", "com.google.android.permissioncontroller")
                        }
                        allow?.performAction(AccessibilityNodeInfo.ACTION_CLICK)
                        SystemClock.sleep(100)
                    }
                    check(LocalNetworkAccess.granted(context))
                    check(!LocalNetworkAccess.neededForOrigin(context, "https://10.0.2.2"))
                }
            }
            phase = "rejected-login"
            signIn("incorrect-fixture-password")
            await { shown("Email or password was not accepted.") }
            phase = "successful-login"
            signIn("acceptance-fixture-password-not-secret")
            await { shown("Your devices") && currentViews().filterIsInstance<Button>().any { it.text.contains("Acceptance fixture") } }
            check(SessionVault(context).origin() == BuildConfig.SERVER_ORIGIN)
            phase = "session-reopen"
            instrumentation.runOnMainSync { activity?.finish() }
            instrumentation.waitForIdleSync()
            activity = launch()
            await { shown("Your devices") && currentViews().filterIsInstance<Button>().any { it.text.contains("Acceptance fixture") } }
            phase = "revoked-device-state"
            await {
                currentViews().filterIsInstance<Button>().firstOrNull { it.text.contains("Acceptance fixture") && it.text.contains("revoked") }
                    ?.performClick() == true
            }
            await { shown("This device is revoked. Controls require active enrollment.") }
            phase = "pairing-code-screen"
            click("Enroll a device")
            val deadline = SystemClock.elapsedRealtime() + 30_000
            var pairingShown = false
            while (SystemClock.elapsedRealtime() < deadline && !pairingShown) {
                val root = instrumentation.uiAutomation.rootInActiveWindow
                if (root != null) {
                    pairingShown = root.findAccessibilityNodeInfosByText("Enroll the child app").isNotEmpty() &&
                        root.findAccessibilityNodeInfosByText("Expires:").isNotEmpty()
                    if (pairingShown) {
                        val done = root.findAccessibilityNodeInfosByText("Done").firstOrNull { it.isClickable }
                        check(done?.performAction(AccessibilityNodeInfo.ACTION_CLICK) == true)
                    }
                }
                if (!pairingShown) SystemClock.sleep(100)
            }
            check(pairingShown)
            phase = "active-device-screen"
            await { currentViews().filterIsInstance<Button>().firstOrNull {
                it.text.contains("UI fixture device") && it.text.contains("active")
            }?.performClick() == true }
            await { shown("Requested policy version 0") && shown("No capability report received yet.") }
            phase = "schedule-editor"
            click("Add or edit schedule")
            dialogAction("New schedule")
            dialogField("Schedule ID", "fixture-ui-schedule")
            dialogField("Time zone", "UTC")
            dialogField("Weekdays: Monday=1, Sunday=7", "1,3,5")
            dialogField("Start HH:mm", "22:00")
            dialogField("End HH:mm", "07:00")
            dialogAction("Use schedule")
            await { currentViews().filterIsInstance<TextView>().any { it.text.contains("fixture-ui-schedule") } }
            instrumentation.runOnMainSync {
                currentViews().filterIsInstance<Switch>().single { it.text.toString() == "Request internet pause" }.isChecked = true
            }
            click("Save requested policy")
            await { shown("Requested policy version 1") && shown("Pending confirmation of the current effective policy.") }
            val policy = fixture.client.policy(deviceId)
            check(policy.getBoolean("internetPaused") && policy.getJSONArray("schedules").length() == 1)
            val schedule = policy.getJSONArray("schedules").getJSONObject(0)
            check(schedule.getString("id") == "fixture-ui-schedule" && schedule.getString("timeZone") == "UTC" &&
                schedule.getJSONArray("daysOfWeek").toString() == "[1,3,5]" && schedule.getInt("startMinute") == 1320 && schedule.getInt("endMinute") == 420)
            phase = "restore-requested-policy"
            click("Remove a schedule")
            dialogAction("fixture-ui-schedule")
            await { shown("No schedules.") }
            instrumentation.runOnMainSync {
                currentViews().filterIsInstance<Switch>().single { it.text.toString() == "Request internet pause" }.isChecked = false
            }
            click("Save requested policy")
            await { shown("Requested policy version 2") && shown("Call contacts and Call Parent") }
            check(!fixture.client.policy(deviceId).getBoolean("internetPaused"))
            phase = "contact-editor"
            click("Call contacts and Call Parent")
            await { shown("Save contact shortcuts") }
            click("Add contact")
            dialogField("Contact name", "UI fixture parent")
            dialogField("International number (+country code)", "+12025550125")
            dialogAction("Use for Call Parent")
            dialogAction("Use contact")
            await { shown("Save contact shortcuts") && currentViews().filterIsInstance<TextView>().any { it.text.contains("UI fixture parent") } }
            click("Save contact shortcuts")
            await { shown("Saved on server (version 1). Child sync is still required.") && shown("Edit") }
            val contacts = fixture.client.callContacts(deviceId)
            check(contacts.getLong("version") == 1L && contacts.getJSONArray("contacts").length() == 1)
            val contact = contacts.getJSONArray("contacts").getJSONObject(0)
            check(contact.getString("label") == "UI fixture parent" && contact.getString("number") == "+12025550125" && contact.getBoolean("isParent"))
            phase = "contact-concurrent-edit-refusal"
            fixture.client.saveCallContacts(deviceId, 1, org.json.JSONArray())
            click("Save contact shortcuts")
            await { shown("The saved state changed. Refresh before trying again.") }
            check(fixture.client.callContacts(deviceId).getJSONArray("contacts").length() == 0)
            click("Discard changes and reload")
            await { shown("Save contact shortcuts") && !shown("Edit") }
            phase = "queued-command-screen"
            click("Back to UI fixture device")
            await { shown("Lock device") }
            phase = "call-watch-start"
            val notifications = context.getSystemService(NotificationManager::class.java)
            fun watching() = notifications.activeNotifications.any { it.notification.channelId == "parent-call-watch" }
            fun alerted() = notifications.activeNotifications.filter { it.notification.channelId == "parent-call-alerts" }
            check(!watching() && alerted().isEmpty())
            val reportedCall = java.util.UUID.randomUUID().toString()
            fixture.reportRinging(reportedCall)
            click("Foreground calls")
            await { shown("Remote actions are disabled for this call.") }
            click("Watch incoming calls for ten minutes")
            dialogAction("Start watch")
            val permissionDeadline = SystemClock.elapsedRealtime() + 15_000
            while (context.checkSelfPermission(Manifest.permission.POST_NOTIFICATIONS) != PackageManager.PERMISSION_GRANTED &&
                SystemClock.elapsedRealtime() < permissionDeadline) {
                val root = instrumentation.uiAutomation.rootInActiveWindow
                val allow = root?.findAccessibilityNodeInfosByText("Allow")?.firstOrNull {
                    it.text?.toString()?.equals("Allow", true) == true && it.isClickable && it.isVisibleToUser &&
                        it.packageName?.toString() in setOf("com.android.permissioncontroller", "com.google.android.permissioncontroller")
                }
                allow?.performAction(AccessibilityNodeInfo.ACTION_CLICK)
                SystemClock.sleep(100)
            }
            check(context.checkSelfPermission(Manifest.permission.POST_NOTIFICATIONS) == PackageManager.PERMISSION_GRANTED)
            await { watching() && alerted().size == 1 }
            val notification = alerted().single().notification
            check(notification.visibility == Notification.VISIBILITY_PRIVATE && notification.publicVersion != null)
            check(notification.publicVersion.extras.getCharSequence(Notification.EXTRA_TITLE)?.toString() == "App Control")
            check(notification.publicVersion.extras.getCharSequence(Notification.EXTRA_TEXT)?.toString() == "Open the parent app to review an alert.")
            check(notification.extras.getCharSequence(Notification.EXTRA_TITLE)?.toString() == "Child reported an incoming call")
            check(notification.extras.getCharSequence(Notification.EXTRA_TEXT)?.toString() == "Open to refresh the exact call and review available controls.")
            check(notification.actions.isNullOrEmpty() && notification.contentIntent.isImmutable && notification.contentIntent.creatorPackage == context.packageName)
            phase = "call-alert-exact-review"
            val review = notification.contentIntent
            review.send()
            await { shown("Back to Selected device") && currentViews().filterIsInstance<TextView>().any { it.text.contains(reportedCall) } && shown("Remote actions are disabled for this call.") }
            instrumentation.runOnMainSync {
                check(currentViews().filterIsInstance<Button>().filter { it.text.toString() in setOf("Answer", "Reject") }.let {
                    it.size == 2 && it.none { button -> button.isEnabled }
                })
            }
            phase = "call-alert-ended-review"
            fixture.reportRinging(null)
            await { watching() && alerted().isEmpty() }
            review.send()
            await { shown("The exact call from this alert is no longer in the child report. Refresh or return to the device screen.") }
            phase = "call-watch-stop"
            click("Stop call watch")
            await { !watching() && alerted().isEmpty() }
            click("Back to Selected device")
            await { shown("All devices") && shown("Lock device") }
            click("All devices")
            await { currentViews().filterIsInstance<Button>().any { it.text.contains("UI fixture device") } }
            await { currentViews().filterIsInstance<Button>().firstOrNull { it.text.contains("UI fixture device") }?.performClick() == true }
            await { shown("Lock device") }
            phase = "call-watch-logout-prepare"
            click("Foreground calls")
            await { shown("No foreground Telecom calls were reported.") }
            click("Watch incoming calls for ten minutes")
            val watchRequestedAt = SystemClock.elapsedRealtime()
            dialogAction("Start watch")
            await { watching() }
            if (observeWatchWindow) {
                phase = "call-watch-elapsed-window"
                val deadline = watchRequestedAt + CallWatchState.WINDOW_MILLIS + 15_000
                while (watching() && SystemClock.elapsedRealtime() < deadline) SystemClock.sleep(500)
                val elapsed = SystemClock.elapsedRealtime() - watchRequestedAt
                check(!watching() && alerted().isEmpty())
                check(elapsed in (CallWatchState.WINDOW_MILLIS - 5_000)..(CallWatchState.WINDOW_MILLIS + 15_000))
                // An early session/device failure must not masquerade as successful expiry.
                check(ParentClient(context).remoteCalls(deviceId).getJSONArray("calls").length() == 0)
                check(fixture.client.devices().getJSONArray("devices").let { devices ->
                    (0 until devices.length()).any { devices.getJSONObject(it).getString("id") == deviceId &&
                        devices.getJSONObject(it).getString("state") == "active" }
                })
                observedWindowMillis = elapsed
                phase = "call-watch-post-expiry-restart"
                click("Watch incoming calls for ten minutes")
                dialogAction("Start watch")
                await { watching() }
            }
            click("Back to UI fixture device")
            await { shown("Lock device") }
            phase = "queued-command-screen"
            click("Lock device")
            dialogAction("Request lock")
            await { shown("Lock command: queued. Awaiting child result.") && shown("Requested policy version 2") }
            val history = fixture.client.history(deviceId).getJSONArray("commands")
            check(history.length() == 1 && history.getJSONObject(0).getString("status") == "queued")
            click("Command history")
            await { currentViews().filterIsInstance<TextView>().any { it.text.contains("LOCK_DEVICE") && it.text.contains("queued") } }
            click("Back to UI fixture device")
            await { shown("All devices") && shown("Lock device") }
            click("All devices")
            await { shown("Your devices") && currentViews().filterIsInstance<Button>().any { it.text.contains("UI fixture device") } }
            phase = "logout"
            click("Sign out")
            phase = "logout-response"
            await { shown("Signed out.") && shown("Your family devices") }
            await { !watching() && alerted().isEmpty() }
            check(context.getSharedPreferences("parent-session", Context.MODE_PRIVATE).all.isEmpty())
            phase = "signed-out-reopen"
            instrumentation.runOnMainSync { activity?.finish() }
            instrumentation.waitForIdleSync()
            activity = launch()
            await { shown("Your family devices") && shown("Sign in") }
            result.putString("server_https_parent_ui_probe", "passed")
            result.putString("server_https_parent_controls_ui_probe", "passed")
            result.putString("server_https_parent_call_watch_probe", "passed")
            if (observeWatchWindow) {
                result.putString("parent_call_watch_window_probe", "passed")
                result.putLong("parent_call_watch_window_elapsed_ms", checkNotNull(observedWindowMillis))
            }
        } catch (failure: Exception) {
            result.putString("server_https_parent_ui_probe", "failed")
            result.putString("server_https_parent_ui_phase", phase)
            result.putString("server_https_parent_ui_failure", failure.javaClass.simpleName)
            result.putBoolean("server_https_parent_ui_default_tls_retained", HttpsURLConnection.getDefaultSSLSocketFactory() === acceptanceSockets)
            if (phase == "rejected-login") instrumentation.runOnMainSync {
                fun known(view: View): String? {
                    if (view is TextView) {
                        val labels = mapOf(
                            "Email or password was not accepted." to "unauthorized",
                            "Too many requests. Wait a minute and try again." to "rate_limited",
                            "Could not complete the request. Check the server address and connection." to "connection_failed",
                            "Your session ended. Please sign in again." to "session_ended")
                        labels[view.text.toString()]?.let { return it }
                    }
                    if (view is ViewGroup) for (i in 0 until view.childCount) known(view.getChildAt(i))?.let { return it }
                    return null
                }
                result.putString("server_https_parent_ui_login_state", activity?.let { known(it.window.decorView) } ?: "unclassified")
                activity?.let { shown ->
                    val clientField = ParentActivity::class.java.getDeclaredField("client").apply { isAccessible = true }
                    result.putBoolean("server_https_parent_ui_origin_matches", (clientField.get(shown) as ParentClient).server() == BuildConfig.SERVER_ORIGIN)
                }
            }
            if (phase == "rejected-login") {
                try {
                    ParentClient(context).login(BuildConfig.SERVER_ORIGIN, "acceptance-owner@example.test", "incorrect-fixture-password")
                    result.putString("server_https_parent_ui_retry", "unexpected_success")
                } catch (retry: Exception) {
                    result.putString("server_https_parent_ui_retry", retry.javaClass.simpleName)
                    if (retry is ParentClient.RequestFailure) result.putInt("server_https_parent_ui_retry_status", retry.status)
                }
            }
        } finally {
            var cleanupFailed = false
            fun clean(action: () -> Unit) { try { action() } catch (_: Exception) { cleanupFailed = true } }
            clean {
                context.stopService(Intent(context, CallWatchService::class.java))
                instrumentation.runOnMainSync { activity?.finish() }
                instrumentation.waitForIdleSync()
            }
            clean { deviceFixture?.close() }
            clean { if (ownsSession) SessionVault(context).clear() }
            clean { if (ownsKey) KeyStore.getInstance("AndroidKeyStore").apply { load(null); deleteEntry("app-control.parent-session") } }
            clean { HttpsURLConnection.setDefaultSSLSocketFactory(previousSockets) }
            if (cleanupFailed) {
                result.putString("server_https_parent_ui_probe", "failed")
                result.putString("server_https_parent_ui_phase", "cleanup")
                result.remove("server_https_parent_controls_ui_probe")
                result.remove("server_https_parent_call_watch_probe")
                result.remove("parent_call_watch_window_probe")
                result.remove("parent_call_watch_window_elapsed_ms")
            }
        }
        return result
    }
}
