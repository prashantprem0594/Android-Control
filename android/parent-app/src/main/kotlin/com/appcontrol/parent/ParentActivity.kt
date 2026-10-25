package com.appcontrol.parent

import android.app.Activity
import android.app.AlertDialog
import android.os.Bundle
import android.graphics.Color
import android.text.InputType
import android.view.WindowManager
import android.widget.*
import org.json.JSONArray
import org.json.JSONObject
import java.time.ZoneId
import java.util.UUID
import java.util.concurrent.Executors

/** Parent workflows use observed server/device state; writes only confirm saved intent. */
class ParentActivity : Activity() {
    private lateinit var client: ParentClient
    private lateinit var column: LinearLayout
    private lateinit var notice: TextView
    private val worker = Executors.newSingleThreadExecutor()
    private var busy = false
    private var generation = 0
    private var pendingWatchDevice: String? = null
    private val teal = Color.rgb(0, 99, 94)
    override fun onCreate(state: Bundle?) {
        super.onCreate(state)
        window.addFlags(WindowManager.LayoutParams.FLAG_SECURE)
        client = ParentClient(this)
        if (client.hasSession()) { if (!openCallAlert(intent)) loadDevices() } else login()
    }
    override fun onNewIntent(intent: android.content.Intent) {
        super.onNewIntent(intent); setIntent(intent)
        if (client.hasSession()) openCallAlert(intent) else login()
    }
    private fun openCallAlert(source: android.content.Intent): Boolean {
        if (source.action != CallWatchService.ACTION_REVIEW) return false
        val id = source.getStringExtra(CallWatchService.EXTRA_DEVICE) ?: return false
        val callId = source.getStringExtra(CallWatchService.EXTRA_CALL) ?: return false
        val parsed = runCatching { UUID.fromString(callId) }.getOrNull()
        if (!id.matches(Regex("dev_[A-Za-z0-9_-]{22}")) || parsed == null || parsed.toString() != callId || parsed.version() != 4 || parsed.variant() != 2) return false
        if (busy) { android.widget.Toast.makeText(this, "Finish the current request, then open the alert again.", android.widget.Toast.LENGTH_LONG).show(); return true }
        // Intents provide navigation only. Fetch authorized current state; never trust an alert's old state.
        remoteCalls(id, "Selected device", callId)
        source.action = null
        return true
    }
    override fun onDestroy() { generation++; worker.shutdownNow(); super.onDestroy() }
    private fun page(title: String) {
        generation++
        column = LinearLayout(this).apply {
            orientation = LinearLayout.VERTICAL; setPadding(28, 32, 28, 32)
            setBackgroundColor(Color.rgb(246, 249, 248))
        }
        val scroll = ScrollView(this).apply { addView(column) }; setContentView(scroll)
        text("APP CONTROL", 13).setTextColor(teal)
        text(title, 27)
        notice = text("", 15)
    }
    private fun text(value: String, size: Int = 16): TextView = TextView(this).apply {
        text = value; textSize = size.toFloat(); setTextColor(Color.rgb(28, 49, 47))
        setPadding(0, 10, 0, 10); column.addView(this)
    }
    private fun field(label: String, value: String = "", password: Boolean = false): EditText {
        text(label, 14)
        return EditText(this).apply {
            setText(value); setSingleLine(true); isSaveEnabled = false
            inputType = if (password) InputType.TYPE_CLASS_TEXT or InputType.TYPE_TEXT_VARIATION_PASSWORD else InputType.TYPE_CLASS_TEXT
            column.addView(this)
        }
    }
    private fun button(label: String, action: () -> Unit): Button = Button(this).apply {
        text = label; isAllCaps = false; minHeight = 52
        setOnClickListener { if (!busy) action() }; column.addView(this)
    }
    private fun <T> task(action: () -> T, complete: (T) -> Unit) {
        if (busy) return
        busy = true; notice.text = "Connecting…"
        val expected = generation
        worker.execute {
            val result = runCatching(action)
            runOnUiThread {
                if (isDestroyed || expected != generation) { busy = false; return@runOnUiThread }
                busy = false
                result.fold({ value ->
                    notice.text = ""
                    runCatching { complete(value) }.onFailure { notice.text = "The server response could not be displayed. Refresh or sign in again." }
                }, { failure ->
                    if (failure is ParentClient.SignInRequired) { login(); notice.text = "Your session ended. Please sign in again." }
                    else notice.text = when ((failure as? ParentClient.RequestFailure)?.status) {
                        400 -> "Check the details and try again. Protected or unknown apps cannot be blocked."
                        401 -> "Email or password was not accepted."
                        404 -> "This device is unavailable. Refresh the device list."
                        409 -> "The saved state changed. Refresh before trying again."
                        429 -> "Too many requests. Wait a minute and try again."
                        else -> "Could not complete the request. Check the server address and connection."
                    }
                })
            }
        }
    }
    private fun login() {
        stopService(android.content.Intent(this, CallWatchService::class.java))
        page("Your family devices")
        text("Sign in to your private control server.")
        val server = field("HTTPS server", client.server()).apply { isEnabled = BuildConfig.SERVER_ORIGIN.isEmpty() }
        val email = field("Email").apply { inputType = InputType.TYPE_CLASS_TEXT or InputType.TYPE_TEXT_VARIATION_EMAIL_ADDRESS }
        val password = field("Password", password = true)
        button("Sign in") {
            val secret = password.text.toString(); password.setText("")
            val address = server.text.toString(); val account = email.text.toString()
            task({ client.login(address, account, secret) }) { loadDevices() }
        }
        button("Create parent account") {
            val secret = password.text.toString(); password.setText("")
            val address = server.text.toString(); val account = email.text.toString()
            task({ client.register(address, account, secret) }) { notice.text = "Account created. Enter your password to sign in." }
        }
        text("Lost your password? Your verified server operator can reset it and revoke old parent sessions. Then sign in with the new password.", 13)
        if (BuildConfig.UPDATE_PUBLIC_KEY.isNotEmpty()) button("App updates") { startActivity(android.content.Intent(this, UpdateActivity::class.java)) }
    }
    private fun loadDevices(after: String = "") {
        page("Your devices")
        if (BuildConfig.UPDATE_PUBLIC_KEY.isNotEmpty()) button("App updates") { startActivity(android.content.Intent(this, UpdateActivity::class.java)) }
        button("Refresh") { loadDevices() }
        button("Enroll a device") {
            task({ client.pairingCode() }) { code ->
                AlertDialog.Builder(this).setTitle("Enroll the child app")
                    .setMessage("Open enrollment in the child app and enter this code:\n\n${code.getString("code")}\n\nExpires: ${code.getString("expiresAt")}\n\nRefresh this list after enrollment.")
                    .setPositiveButton("Done", null).show()
            }
        }
        button("Sign out") {
            stopService(android.content.Intent(this, CallWatchService::class.java))
            task({ client.logout() }) { revoked ->
                login(); notice.text = if (revoked) "Signed out." else "Signed out on this phone. Server session revocation could not be confirmed."
            }
        }
        task({ client.devices(after) }) { response ->
            val devices = response.getJSONArray("devices")
            if (devices.length() == 0) text("No devices on this page. Enroll the child app to get started.")
            for (index in 0 until devices.length()) {
                val device = devices.getJSONObject(index)
                val name = device.getString("displayName"); val state = device.getString("state")
                button("$name · $state") {
                    if (state == "active") loadDevice(device.getString("id"), name)
                    else notice.text = "This device is $state. Controls require active enrollment."
                }
            }
            response.optString("nextCursor").takeIf { it.isNotEmpty() }?.let { cursor -> button("Next page") { loadDevices(cursor) } }
            if (after.isNotEmpty()) button("First page") { loadDevices() }
        }
    }
    private fun setOf(values: JSONArray?): MutableSet<String> = (0 until (values?.length() ?: 0)).map { values!!.getString(it) }.toMutableSet()
    private fun loadDevice(id: String, name: String) {
        page(name); button("All devices") { loadDevices() }
        task({ Triple(client.policy(id), client.apps(id), client.status(id)) }) { (policy, inventory, status) ->
            renderDevice(id, name, policy, inventory)
            val capabilities = status.optJSONObject("capabilities")
            if (capabilities == null) text("No capability report received yet.", 13)
            else {
                val receivedAt = runCatching { java.time.Instant.parse(status.getString("receivedAt")) }.getOrNull()
                val serverTime = runCatching { java.time.Instant.parse(status.getString("serverTime")) }.getOrNull()
                val freshness = if (serverTime == null) ContactFreshness.State.UNKNOWN else ContactFreshness.classify(receivedAt, serverTime)
                text(when(freshness) {
                    ContactFreshness.State.RECENT -> "Recent child contact"
                    ContactFreshness.State.STALE -> "Offline or synchronization delayed"
                    ContactFreshness.State.CLOCK_CHANGED -> "Server clock changed; contact freshness is uncertain"
                    else -> "Contact freshness unavailable"
                }, 18)
                text("Last child contact: ${status.getString("receivedAt")}\nAndroid API ${capabilities.getInt("androidApi")}\nDevice Owner: ${capabilities.getBoolean("deviceOwner")} · Dialer role: ${capabilities.getBoolean("dialerRole")}\nVPN consent: ${capabilities.getBoolean("vpnConsent")} · Exact alarms: ${capabilities.getBoolean("exactAlarms")}", 14)
            }
        }
    }
    private fun remoteCalls(id: String, name: String, onlyCallId: String? = null) {
        page("Foreground calls")
        button("Back to $name") { loadDevice(id, name) }
        button("Refresh call state") { remoteCalls(id, name, onlyCallId) }
        button("Watch incoming calls for ten minutes") { requestCallWatch(id) }
        button("Stop call watch") {
            stopService(android.content.Intent(this, CallWatchService::class.java))
            notice.text = "Call watch stopped."
        }
        text("Call watch checks one child every five seconds while its visible service is running, for at most ten minutes. Delivery may be delayed or missed; it is not a guaranteed call alert.", 13)
        val previous = getSharedPreferences("parent-call-history", MODE_PRIVATE).getString(id, null)
        if (previous != null) button("Check last call command") {
            checkCallCommand(id, previous)
        }
        val pendingJson = getSharedPreferences("parent-pending-call", MODE_PRIVATE).getString(id, null)
        val pendingCall = pendingJson?.let { runCatching { JSONObject(it) }.getOrNull() }
        if (pendingJson != null && (onlyCallId == null || pendingCall?.optString("callId") == onlyCallId)) button("Retry pending call request") {
            if (pendingCall == null) notice.text = "A saved call request could not be read. Review command history before recovering its retry state."
            else submitCallAction(id, pendingCall.optString("callId"), pendingCall.optString("action"),
                pendingCall.optInt("ttlSeconds", 20), pendingCall.optString("key"))
        }
        else if (pendingJson != null) text("Another call has a pending request. Review it from the device screen before sending a new action.", 13)
        val callRequestStarted = android.os.SystemClock.elapsedRealtime()
        task({ client.remoteCalls(id) }) { response ->
            val observedAt = runCatching { java.time.Instant.parse(response.getString("observedAt")) }.getOrNull()
            val serverTime = runCatching { java.time.Instant.parse(response.getString("serverTime")) }.getOrNull()
            val age = if (observedAt != null && serverTime != null) java.time.Duration.between(observedAt, serverTime) else null
            val received = android.os.SystemClock.elapsedRealtime()
            val totalAge = age?.plusMillis(received - callRequestStarted)
            val fresh = totalAge != null && !totalAge.isNegative && totalAge < java.time.Duration.ofSeconds(30)
            val freshUntil = if (fresh) received + java.time.Duration.ofSeconds(30).minus(totalAge).toMillis() else received
            text(if (observedAt == null) "No child call-state report is available."
                else "Child call-state report: $observedAt · ${if (fresh) "current" else "stale"}")
            val calls = response.optJSONArray("calls") ?: JSONArray()
            var matched = false
            if (calls.length() == 0) text("No foreground Telecom calls were reported.")
            for (index in 0 until calls.length()) {
                val call = calls.getJSONObject(index)
                val callId = call.getString("callId")
                if (onlyCallId != null && callId != onlyCallId) continue
                matched = true
                val state = call.getString("state")
                val remoteActionsAllowed = call.optBoolean("remoteActionsAllowed", false)
                val parsed = runCatching { UUID.fromString(callId) }.getOrNull()
                if (parsed == null || parsed.toString() != callId || parsed.version() != 4 || parsed.variant() != 2) continue
                text("Call $callId\nState: $state", 15)
                if (!fresh) continue
                val actions = when (state) {
                    "RINGING" -> listOf("ANSWER_CALL", "REJECT_CALL")
                    "ACTIVE", "HOLDING" -> listOf("END_CALL")
                    else -> emptyList()
                }
                if (!remoteActionsAllowed) text("Remote actions are disabled for this call.", 13)
                actions.forEach { action ->
                    val actionButton = button(action.removeSuffix("_CALL").lowercase().replaceFirstChar { it.uppercase() }) {
                        if (android.os.SystemClock.elapsedRealtime() >= freshUntil) {
                            notice.text = "This call report expired. Refresh call state before sending a request."
                        } else AlertDialog.Builder(this).setTitle("${action.removeSuffix("_CALL").lowercase().replaceFirstChar { it.uppercase() } } call?")
                            .setMessage("This sends a request for this exact call. The child must observe the call result before reporting success.")
                            .setNegativeButton("Cancel", null)
                            .setPositiveButton("Queue request") { _, _ ->
                                if (android.os.SystemClock.elapsedRealtime() < freshUntil) queueCallAction(id, callId, action)
                                else notice.text = "This call report expired while confirmation was open. Refresh call state."
                            }.show()
                    }
                    actionButton.isEnabled = remoteActionsAllowed
                    actionButton.postDelayed({ actionButton.isEnabled = false }, (freshUntil - android.os.SystemClock.elapsedRealtime()).coerceAtLeast(0))
                }
            }
            if (onlyCallId != null && !matched) text("The exact call from this alert is no longer in the child report. Refresh or return to the device screen.")
            text("Only opaque call IDs and Telecom states are shown. A stale snapshot cannot be used to queue a command.", 13)
        }
    }
    private fun requestCallWatch(id: String) {
        AlertDialog.Builder(this).setTitle("Watch this child's incoming calls?")
            .setMessage("For up to ten minutes this phone will check the server every five seconds and show a visible service notification. Android, network and child reporting delays can prevent alerts. Starting again replaces the previous watch.")
            .setNegativeButton("Cancel", null).setPositiveButton("Start watch") { _, _ ->
                if (checkSelfPermission(android.Manifest.permission.POST_NOTIFICATIONS) != android.content.pm.PackageManager.PERMISSION_GRANTED) {
                    pendingWatchDevice = id
                    requestPermissions(arrayOf(android.Manifest.permission.POST_NOTIFICATIONS), 5603)
                } else startCallWatch(id)
            }.show()
    }
    override fun onRequestPermissionsResult(requestCode: Int, permissions: Array<out String>, grantResults: IntArray) {
        super.onRequestPermissionsResult(requestCode, permissions, grantResults)
        if (requestCode == 5603) {
            val id = pendingWatchDevice
            pendingWatchDevice = null
            if (id != null && grantResults.firstOrNull() == android.content.pm.PackageManager.PERMISSION_GRANTED) startCallWatch(id)
            else notice.text = "Notifications are required for call watch. You can still refresh call state here."
        }
    }
    private fun startCallWatch(id: String) {
        val notifications = getSystemService(android.app.NotificationManager::class.java)
        if (!notifications.areNotificationsEnabled()) { notice.text = "Enable parent app notifications in Android settings before starting call watch."; return }
        runCatching {
            startForegroundService(android.content.Intent(this, CallWatchService::class.java).putExtra(CallWatchService.EXTRA_DEVICE,id))
        }.onSuccess { notice.text = "Ten-minute call watch requested for this child. Check its service notification; Android may pause observation." }
            .onFailure { notice.text = "Android could not start call watch. You can refresh call state here." }
    }
    private fun queueCallAction(id: String, callId: String, action: String) {
        val pending = getSharedPreferences("parent-pending-call", MODE_PRIVATE)
        val saved = pending.getString(id, null)?.let { runCatching { JSONObject(it) }.getOrNull() }
        if (pending.contains(id) && saved == null) {
            notice.text = "A saved call request could not be read; refusing to create a potentially duplicate action."
            return
        }
        if (saved != null && (saved.optString("callId") != callId || saved.optString("action") != action)) {
            notice.text = "Retry or confirm the previous call request result before queueing a different action."
            return
        }
        val record = saved ?: JSONObject().put("callId", callId).put("action", action).put("ttlSeconds", 20)
            .put("key", UUID.randomUUID().toString()).also {
                check(pending.edit().putString(id, it.toString()).commit())
            }
        submitCallAction(id, callId, action, 20, record.getString("key"))
    }
    private fun submitCallAction(id: String, callId: String, action: String, ttlSeconds: Int, key: String) {
        task({ client.callAction(id, callId, action, ttlSeconds, key) }) { command ->
            val commandId = command.getString("commandId")
            check(getSharedPreferences("parent-call-history", MODE_PRIVATE).edit().putString(id, commandId).commit())
            check(getSharedPreferences("parent-pending-call", MODE_PRIVATE).edit().remove(id).commit())
            notice.text = "Call command: ${command.getString("status")}. Awaiting the child result."
            button("Check call result") { checkCallCommand(id, commandId) }
        }
    }
    private fun checkCallCommand(id: String, commandId: String) {
        task({ client.command(id, commandId) }) { result ->
            notice.text = "Call command: ${result.getString("status")}. Only succeeded confirms the child observed the requested result."
        }
    }
    private fun renderDevice(id: String, name: String, policy: JSONObject, inventory: JSONObject) {
        text("Requested policy version ${policy.getLong("version")}", 18)
        val report = policy.optJSONObject("reported")
        val confirmed = report != null && report.optLong("version", -1) == policy.getLong("version") &&
            report.optString("result") == "applied" && report.optString("stateKey").isNotEmpty() &&
            report.optString("stateKey") == policy.optString("effectiveStateKey")
        text(if (confirmed) "Child confirmed the current effective policy." else "Pending confirmation of the current effective policy.")
        if (report != null) text("Last report: ${report.optString("result")} · ${report.optString("reportedAt")}\n${report.optString("reason")}", 13)
        text("A report records the last observation; it does not prove the device is currently online or that real call handling has passed.", 13)
        button("Refresh device state") { loadDevice(id, name) }
        button("Foreground calls") { remoteCalls(id, name) }
        button("Call contacts and Call Parent") { callContacts(id, name) }
        button("Command history") { commandHistory(id, name) }
        val paused = Switch(this).apply { text = "Request internet pause"; isChecked = policy.getBoolean("internetPaused"); column.addView(this) }
        val blocked = setOf(policy.optJSONArray("blockedPackages"))
        val allowed = setOf(policy.optJSONArray("alwaysAllowedPackages"))
        val apps = inventory.getJSONArray("apps")
        text("App inventory: ${inventory.optString("updatedAt", "not received")}", 13)
        button("Choose blocked apps (${blocked.size})") { chooseApps(apps, blocked, true) }
        button("Choose always-allowed apps (${allowed.size})") { chooseApps(apps, allowed, false) }
        text("Always-allowed apps take precedence over blocks and schedules.", 13)
        val schedules = policy.optJSONArray("schedules") ?: JSONArray()
        val rules = (0 until schedules.length()).map { JSONObject(schedules.getJSONObject(it).toString()) }.toMutableList()
        val scheduleSummary = text("")
        fun describe() { scheduleSummary.text = if (rules.isEmpty()) "No schedules." else rules.joinToString("\n") { "${it.getString("id")} · ${it.getString("timeZone")} · ${clock(it.getInt("startMinute"))}–${clock(it.getInt("endMinute"))}" } }
        describe()
        button("Add or edit schedule") {
            val choices = arrayOf("New schedule") + rules.map { it.getString("id") }
            AlertDialog.Builder(this).setTitle("Schedule").setItems(choices) { _, index ->
                editSchedule(apps, rules, if (index == 0) null else rules[index - 1]) { describe() }
            }.show()
        }
        button("Remove a schedule") {
            if (rules.isEmpty()) return@button
            AlertDialog.Builder(this).setTitle("Remove schedule").setItems(rules.map { it.getString("id") }.toTypedArray()) { _, index -> rules.removeAt(index); describe() }.show()
        }
        button("Save requested policy") {
            val body = JSONObject().put("expectedVersion", policy.getLong("version")).put("internetPaused", paused.isChecked)
                .put("blockedPackages", JSONArray(blocked.sorted())).put("alwaysAllowedPackages", JSONArray(allowed.sorted()))
                .put("schedules", JSONArray(rules))
            task({ client.savePolicy(id, body) }) { loadDevice(id, name); notice.text = "Request saved. Awaiting child confirmation." }
        }
        button("Lock device") {
            AlertDialog.Builder(this).setTitle("Lock $name?").setMessage("The command expires after 60 seconds. Success requires a child acknowledgement.")
                .setNegativeButton("Cancel", null).setPositiveButton("Request lock") { _, _ -> queueLock(id, name) }.show()
        }
        val history = getSharedPreferences("parent-command-history", MODE_PRIVATE)
        val last = history.getString(id, null)
        if (last != null) button("Check last lock command") {
            task({ client.command(id, last) }) { command -> notice.text = "Lock command: ${command.getString("status")}\nExpires: ${command.getString("expiresAt")}" }
        }
        button("Revoke device access") {
            AlertDialog.Builder(this).setTitle("Revoke $name?").setMessage("The child will lose server access. Existing offline restrictions may remain; restore them before revoking.")
                .setNegativeButton("Cancel", null).setPositiveButton("Revoke") { _, _ -> task({ client.revoke(id) }) { loadDevices() } }.show()
        }
    }
    private fun callContacts(id: String, name: String, savedVersion: Long? = null) {
        page("Call contact shortcuts")
        button("Back to $name") { loadDevice(id, name) }
        task({ client.callContacts(id) }) { saved ->
            if (savedVersion != null) notice.text = "Saved on server (version $savedVersion). Child sync is still required."
            val version = saved.getLong("version")
            val array = saved.getJSONArray("contacts")
            val entries = (0 until array.length()).map { array.getJSONObject(it) }.toMutableList()
            text("Choose up to 10 contacts in international format, such as +919876543210. One can be the Call Parent shortcut. The child reviews the number and presses Call.")
            text("These are shortcuts. Ordinary and emergency dialing stay available. Changes reach the child on its next successful sync; saved contacts remain available offline.", 13)
            val list = LinearLayout(this).apply { orientation = LinearLayout.VERTICAL; column.addView(this) }
            lateinit var redraw: () -> Unit
            fun edit(index: Int?) {
                val existing = index?.let { entries[it] }
                val form = LinearLayout(this).apply { orientation = LinearLayout.VERTICAL; setPadding(24, 12, 24, 12) }
                val label = EditText(this).apply { hint = "Contact name"; setSingleLine(true); isSaveEnabled = false; setText(existing?.getString("label") ?: ""); form.addView(this) }
                val number = EditText(this).apply { hint = "International number (+country code)"; inputType = InputType.TYPE_CLASS_PHONE; setSingleLine(true); isSaveEnabled = false; setText(existing?.getString("number") ?: ""); form.addView(this) }
                val primary = CheckBox(this).apply { text = "Use for Call Parent"; isChecked = existing?.getBoolean("isParent") == true; form.addView(this) }
                val error = TextView(this).apply { form.addView(this) }
                val dialog = AlertDialog.Builder(this).setTitle(if (index == null) "Add contact" else "Edit contact")
                    .setView(form).setNegativeButton("Cancel", null).setPositiveButton("Use contact", null).create()
                dialog.setOnShowListener {
                    dialog.getButton(AlertDialog.BUTTON_POSITIVE).setOnClickListener {
                        val title = label.text.toString().trim()
                        val phone = number.text.toString().trim()
                        val invalidLabel = title.isEmpty() || title.codePointCount(0, title.length) > 40 || title.toByteArray(Charsets.UTF_8).size > 160 ||
                            title.codePoints().anyMatch { cp -> Character.isISOControl(cp) || Character.getType(cp) in setOf(Character.FORMAT.toInt(), Character.LINE_SEPARATOR.toInt(), Character.PARAGRAPH_SEPARATOR.toInt(), Character.SURROGATE.toInt()) }
                        if (invalidLabel || !phone.matches(Regex("\\+[1-9][0-9]{6,14}")) || entries.withIndex().any { it.index != index && it.value.getString("number") == phone }) {
                            error.text = "Use a name of 1-40 characters and a unique international number with 7-15 digits."
                        } else {
                            if (primary.isChecked) entries.forEach { it.put("isParent", false) }
                            val next = JSONObject().put("label", title).put("number", phone).put("isParent", primary.isChecked)
                            if (index == null) entries.add(next) else entries[index] = next
                            redraw(); dialog.dismiss()
                        }
                    }
                }
                dialog.show()
            }
            redraw = {
                list.removeAllViews()
                for ((index, entry) in entries.withIndex()) {
                    list.addView(TextView(this).apply { text = entry.getString("label") + (if (entry.getBoolean("isParent")) " (Call Parent)" else "") + "\n" + entry.getString("number") })
                    list.addView(Button(this).apply { text = "Edit"; setOnClickListener { if (!busy) edit(index) } })
                    list.addView(Button(this).apply { text = "Remove"; setOnClickListener { if (!busy) { entries.removeAt(index); redraw() } } })
                }
                list.addView(Button(this).apply { text = "Add contact"; isEnabled = entries.size < 10; setOnClickListener { if (!busy && entries.size < 10) edit(null) } })
            }
            redraw()
            button("Save contact shortcuts") {
                task({ client.saveCallContacts(id, version, JSONArray(entries)) }) { result ->
                    // Reload before further editing: an ambiguous/stale write must never be blindly retried.
                    callContacts(id, name, result.getLong("version"))
                }
            }
            button("Discard changes and reload") { callContacts(id, name) }
        }
    }
    private fun queueLock(id: String, name: String) {
        val pending = getSharedPreferences("parent-pending-lock", MODE_PRIVATE)
        // Reuse the same key following an ambiguous network result; do not duplicate a lock request.
        val key = pending.getString(id, null) ?: UUID.randomUUID().toString().also { check(pending.edit().putString(id, it).commit()) }
        task({ client.lock(id, key) }) { command ->
            check(getSharedPreferences("parent-command-history", MODE_PRIVATE).edit().putString(id, command.getString("commandId")).commit())
            check(pending.edit().remove(id).commit())
            loadDevice(id, name); notice.text = "Lock command: ${command.getString("status")}. Awaiting child result."
        }
    }
    private fun commandHistory(id: String, name: String, after: String = "") {
        page("Command history"); button("Back to $name") { loadDevice(id, name) }
        button("Refresh newest") { commandHistory(id, name) }
        task({ client.history(id, after) }) { response ->
            val commands = response.getJSONArray("commands")
            if (commands.length() == 0) text("No commands on this page.")
            for (index in 0 until commands.length()) {
                val command = commands.getJSONObject(index)
                text("${command.getString("type")} · ${command.getString("status")}", 18)
                text("Requested: ${command.getString("issuedAt")}\nExpires: ${command.getString("expiresAt")}", 13)
            }
            text("Queued and delivered commands await a child result. A failed command does not confirm the device changed state.", 13)
            response.optString("nextCursor").takeIf { it.isNotEmpty() }?.let { cursor -> button("Older commands") { commandHistory(id, name, cursor) } }
        }
    }
    private fun chooseApps(apps: JSONArray, selected: MutableSet<String>, blocking: Boolean) {
        val entries = (0 until apps.length()).map { apps.getJSONObject(it) }.filter { !blocking || it.getBoolean("blockable") }
        if (entries.isEmpty()) { notice.text = "No eligible apps reported yet."; return }
        val edited = selected.toMutableSet()
        AlertDialog.Builder(this).setTitle(if (blocking) "Blocked apps" else "Always-allowed apps")
            .setMultiChoiceItems(entries.map { "${it.getString("label")}\n${it.getString("packageName")}" }.toTypedArray(),
                entries.map { selected.contains(it.getString("packageName")) }.toBooleanArray()) { _, index, checked ->
                val name = entries[index].getString("packageName"); if (checked) edited.add(name) else edited.remove(name)
            }.setNegativeButton("Cancel", null).setPositiveButton("Use selection") { _, _ -> selected.clear(); selected.addAll(edited) }.show()
    }
    private fun clock(minute: Int) = "%02d:%02d".format(minute / 60, minute % 60)
    private fun editSchedule(apps: JSONArray, rules: MutableList<JSONObject>, existing: JSONObject?, changed: () -> Unit) {
        val form = LinearLayout(this).apply { orientation = LinearLayout.VERTICAL; setPadding(24, 8, 24, 8) }
        fun input(hint: String, value: String = "") = EditText(this).apply { this.hint = hint; setText(value); setSingleLine(true); form.addView(this) }
        val id = input("Schedule ID", existing?.getString("id") ?: "").apply { isEnabled = existing == null }
        val zone = input("Time zone", existing?.getString("timeZone") ?: ZoneId.systemDefault().id)
        val existingDays = existing?.getJSONArray("daysOfWeek")
        val days = input("Weekdays: Monday=1, Sunday=7", if (existingDays == null) "1,2,3,4,5" else (0 until existingDays.length()).joinToString(",") { existingDays.getInt(it).toString() })
        val start = input("Start HH:mm", existing?.let { clock(it.getInt("startMinute")) } ?: "22:00")
        val end = input("End HH:mm", existing?.let { clock(it.getInt("endMinute")) } ?: "07:00")
        val paused = CheckBox(this).apply { text = "Pause internet"; isChecked = existing?.getBoolean("internetPaused") ?: false; form.addView(this) }
        val targets = setOf(existing?.optJSONArray("blockedPackages"))
        form.addView(Button(this).apply { text = "Choose scheduled app blocks"; setOnClickListener { chooseApps(apps, targets, true) } })
        val errors = TextView(this).apply { form.addView(this) }
        val dialog = AlertDialog.Builder(this).setTitle("Schedule").setView(ScrollView(this).apply { addView(form) })
            .setNegativeButton("Cancel", null).setPositiveButton("Use schedule", null).create()
        dialog.setOnShowListener {
            dialog.getButton(AlertDialog.BUTTON_POSITIVE).setOnClickListener {
                try {
                    fun minute(value: String, isEnd: Boolean): Int {
                        require(value.matches(Regex("[0-9]{2}:[0-9]{2}")))
                        val pieces = value.split(':').map(String::toInt)
                        require(pieces[1] in 0..59 && (pieces[0] in 0..23 || isEnd && pieces[0] == 24 && pieces[1] == 0))
                        return pieces[0] * 60 + pieces[1]
                    }
                    val name = id.text.toString().trim(); require(name.matches(Regex("[A-Za-z0-9_-]{1,64}")))
                    val zoneName = zone.text.toString().trim(); ZoneId.of(zoneName)
                    val week = days.text.toString().split(',').map { it.trim().toInt() }; require(week.isNotEmpty() && week.size <= 7 && week.toSet().size == week.size && week.all { it in 1..7 })
                    val from = minute(start.text.toString(), false); val until = minute(end.text.toString(), true); require(from != until)
                    val index = rules.indexOfFirst { it.getString("id") == name }; require(index >= 0 || rules.size < 20)
                    val rule = JSONObject().put("id", name).put("timeZone", zoneName).put("daysOfWeek", JSONArray(week))
                        .put("startMinute", from).put("endMinute", until).put("internetPaused", paused.isChecked).put("blockedPackages", JSONArray(targets.sorted()))
                    if (index >= 0) rules[index] = rule else rules.add(rule)
                    changed(); dialog.dismiss()
                } catch (_: Exception) { errors.text = "Check the ID, time zone, weekdays and times. Start and end must differ." }
            }
        }
        dialog.show()
    }
}
