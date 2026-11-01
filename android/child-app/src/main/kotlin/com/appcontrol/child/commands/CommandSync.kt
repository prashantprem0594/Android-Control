package com.appcontrol.child.commands

import android.app.admin.DevicePolicyManager
import android.content.Context
import android.util.Base64
import com.appcontrol.child.network.DeviceEnrollmentClient
import com.appcontrol.child.BuildConfig
import com.appcontrol.transport.ServerOrigin
import com.appcontrol.child.policy.PolicyEnforcer
import com.appcontrol.child.policy.AppInventory
import com.appcontrol.child.policy.PolicyConfig
import com.appcontrol.child.policy.PolicyRetry
import com.appcontrol.child.calls.CallActionGate
import com.appcontrol.child.calls.CallRegistry
import com.appcontrol.child.remote.RemoteAssistanceService
import org.json.JSONObject
import java.net.URL
import java.time.Instant
import java.util.UUID
import javax.net.ssl.HttpsURLConnection

/** Explicit foreground sync for feasibility testing. Invoke on a worker thread. */
class CommandSync(context: Context) {
    private val app = context.applicationContext
    private val enrollment = app.getSharedPreferences("enrollment", Context.MODE_PRIVATE)
    private val journal = app.getSharedPreferences("command-results", Context.MODE_PRIVATE)

    data class Summary(val acknowledged: Int, val expired: Int, val downloadedPolicyVersion: Long, val policyResult: String,
                       val retryRequired: Boolean)

    fun sync(shouldStop: () -> Boolean = { false }): Summary = synchronized(syncLock) {
        check(!shouldStop()) { "Sync stopped" }
        val deviceId = checkNotNull(enrollment.getString("deviceId", null)) { "Enrollment required" }
        val base = ServerOrigin.require(checkNotNull(enrollment.getString("server", null)), BuildConfig.SERVER_ORIGIN)
        val enrollmentClient = DeviceEnrollmentClient(app)
        // Keep the same-device recovery locator current whenever this installation proves its identity.
        runCatching { enrollmentClient.bindRecoveryHandle(shouldStop) }
        fun syncRequest(path: String, body: JSONObject? = null,
                        method: String = if (body == null) "GET" else "POST"): JSONObject =
            enrollmentClient.withAuthenticatedSession(shouldStop) { token ->
                request(base, path, token, body, method, shouldStop)
            }
        // A failed capability report must not prevent durable command/policy convergence.
        runCatching { syncRequest("/v1/device/status", com.appcontrol.child.policy.DeviceCapabilities.snapshot(app)) }
        runCatching { syncRequest("/v1/device/apps", JSONObject().put("apps", AppInventory(app).snapshot()), "PUT") }
        var acknowledged = 0
        var expired = 0
        var summary: Summary? = null
        SyncStages.run({
            val cleanup = journal.edit()
            for ((key, value) in journal.all) {
                if (key.endsWith(".expiry") && value is String &&
                    !Instant.now().isBefore(Instant.parse(value).plusSeconds(86_400))) {
                    cleanup.remove(key).remove(key.removeSuffix(".expiry"))
                }
            }
            check(cleanup.commit()) { "Could not maintain command journal" }
            val response = syncRequest("/v1/device/commands")
            val commands = response.getJSONArray("commands")
            require(commands.length() <= 50) { "Too many commands" }
            val executor = CommandExecutor(object : CommandExecutor.Journal {
                override fun read(id: String): String? = journal.getString(id, null)
                override fun write(id: String, result: String, deadline: Instant): Boolean =
                    journal.edit().putString(id, result).putString("$id.expiry", deadline.toString()).commit()
            }, Instant::now)
            for (index in 0 until commands.length()) {
                check(!shouldStop()) { "Sync stopped" }
                val command = commands.getJSONObject(index)
                val id = command.getString("commandId")
                require(UUID.fromString(id).toString() == id)
                require(command.getInt("version") == 1 && command.getString("deviceId") == deviceId)
                val type = command.getString("type")
                val payload = command.getJSONObject("payload")
                val callAction = when (type) {
                    "ANSWER_CALL" -> CallActionGate.Action.ANSWER
                    "REJECT_CALL" -> CallActionGate.Action.REJECT
                    "END_CALL" -> CallActionGate.Action.END
                    else -> null
                }
                val openPackage = if (type == "OPEN_APP") {
                    require(payload.length() == 1 && payload.has("packageName"))
                    payload.getString("packageName").also { require(it.matches(Regex("[A-Za-z_][A-Za-z0-9_]*(\\.[A-Za-z_][A-Za-z0-9_]*)+"))) }
                } else null
                val openSettings = type == "OPEN_SETTINGS"
                val remoteScreenshot = type == "REMOTE_SCREENSHOT"
                val remoteGesture = if (type == "REMOTE_GESTURE") parseRemoteGesture(payload) else null
                val callId = if (callAction != null) {
                    require(payload.length() == 1 && payload.has("callId"))
                    payload.getString("callId").also { callId ->
                        val parsedCallId = UUID.fromString(callId)
                        require(parsedCallId.toString() == callId && parsedCallId.version() == 4 && parsedCallId.variant() == 2)
                    }
                } else {
                    require(type == "LOCK_DEVICE" && payload.length() == 0 || openPackage != null || openSettings && payload.length() == 0 ||
                        remoteScreenshot && payload.length() == 0 || remoteGesture != null)
                    null
                }
                val deadline = Instant.parse(command.getString("expiresAt"))
                if (!Instant.now().isBefore(deadline)) {
                    expired++
                    continue
                }
                val result = executor.execute(id, deadline) {
                    check(!shouldStop()) { "Sync stopped" }
                    val manager = app.getSystemService(Context.DEVICE_POLICY_SERVICE) as DevicePolicyManager
                    if (callAction == null) {
                        when {
                            openPackage != null -> com.appcontrol.child.commands.LaunchRequestNotification.post(app, id, openPackage, deadline)
                            openSettings -> com.appcontrol.child.commands.LaunchRequestNotification.post(app, id, null, deadline)
                            remoteScreenshot -> {
                                val image = Base64.encodeToString(RemoteAssistanceService.Bridge.captureJpeg(), Base64.NO_WRAP)
                                syncRequest("/v1/device/remote-assistance/screens/$id", JSONObject().put("imageBase64", image), "PUT")
                            }
                            remoteGesture != null -> when (remoteGesture.action) {
                                "tap" -> RemoteAssistanceService.Bridge.tap(remoteGesture.x, remoteGesture.y)
                                "swipe" -> RemoteAssistanceService.Bridge.swipe(remoteGesture.x, remoteGesture.y, remoteGesture.endX, remoteGesture.endY, remoteGesture.durationMillis)
                                "long_press" -> RemoteAssistanceService.Bridge.longPress(remoteGesture.x, remoteGesture.y, remoteGesture.durationMillis)
                                else -> RemoteAssistanceService.Bridge.global(remoteGesture.action)
                            }
                            else -> { check(manager.isDeviceOwnerApp(app.packageName)) { "Device Owner required" }; manager.lockNow() }
                        }
                    } else {
                        check(BuildConfig.REMOTE_CALLS_ENABLED) { "Remote call commands are disabled in this build" }
                        check(CallRegistry.executeRemoteAction(checkNotNull(callId), callAction, deadline, shouldStop)) {
                            "Call outcome was not observed"
                        }
                    }
                }
                syncRequest("/v1/device/commands/$id/ack", JSONObject().put("result", result))
                acknowledged++
            }
        }, {
            check(!shouldStop()) { "Sync stopped" }
            val policy = syncRequest("/v1/device/policy")
            val version = policy.getLong("version")
            require(version >= 0)
            PolicyConfig.parse(policy).resolve(Instant.now()) // Validate the complete snapshot before caching it.
            policy.getBoolean("internetPaused")
            val packages = policy.getJSONArray("blockedPackages")
            require(packages.length() <= 100)
            for (index in 0 until packages.length()) require(packages.getString(index).length in 3..255)
            val policyCache = app.getSharedPreferences("policy-cache", Context.MODE_PRIVATE)
            check(version >= policyCache.getLong("version", 0)) { "Policy version regressed" }
            check(policyCache.edit().putLong("version", version).putString("desired", policy.toString()).commit()) {
                "Could not save desired policy"
            }
            check(!shouldStop()) { "Sync stopped" }
            val applied = PolicyEnforcer(app).applyCached()
            if (applied != null && !shouldStop()) {
                syncRequest("/v1/device/policy/report", JSONObject().put("version", applied.version)
                    .put("result", applied.result).put("reason", applied.reason).put("stateKey", applied.stateKey))
            }
            if (!shouldStop()) runCatching {
                syncRequest("/v1/device/apps", JSONObject().put("apps", AppInventory(app).snapshot()), "PUT")
            }
            val description = when (applied?.reason) {
                null -> "no saved policy"
                "" -> "applied"
                "device_owner_required" -> "Device Owner setup required"
                "protected_package" -> "essential app excluded"
                "package_unavailable" -> "requested app not installed"
                "package_change_failed" -> "app changes pending"
                "internet_pause_unavailable" -> "internet pause permission or connection needed"
                else -> "not fully applied"
            }
            summary = Summary(acknowledged, expired, version, description,
                applied?.let { PolicyRetry.required(it.result, it.reason) } == true)
        }, { shouldStop() })
        // Routing is best effort and follows durable work. Hints never carry
        // commands; registration uses the same signed device session and origin.
        if (!shouldStop()) runCatching {
            com.appcontrol.child.calls.CallContactStore(app).save(syncRequest("/v1/device/call-contacts"))
        }
        if (!shouldStop()) runCatching {
            com.appcontrol.child.wakeup.WakeupRegistration.token(shouldStop)?.let { registration ->
                syncRequest("/v1/device/wakeup-registration", JSONObject().put("registrationToken", registration), "PUT")
            }
        }
        checkNotNull(summary)
    }

    private data class RemoteGesture(val action: String, val x: Int = 0, val y: Int = 0, val endX: Int = 0, val endY: Int = 0, val durationMillis: Int = 0)
    private fun parseRemoteGesture(payload: JSONObject): RemoteGesture {
        val action = payload.getString("action")
        fun coordinate(name: String) = payload.getInt(name).also { require(it in 0..10000) }
        return when (action) {
            "tap" -> { require(payload.length() == 3); RemoteGesture(action, coordinate("x"), coordinate("y")) }
            "swipe" -> { require(payload.length() == 6); val duration = payload.getInt("durationMillis"); require(duration in 50..2000); RemoteGesture(action, coordinate("x"), coordinate("y"), coordinate("endX"), coordinate("endY"), duration) }
            "long_press" -> { require(payload.length() == 4); val duration = payload.getInt("durationMillis"); require(duration in 300..2000); RemoteGesture(action, coordinate("x"), coordinate("y"), durationMillis = duration) }
            "back", "home", "recents" -> { require(payload.length() == 1); RemoteGesture(action) }
            else -> throw IllegalArgumentException("Unsupported remote gesture")
        }
    }

    private fun request(base: URL, path: String, token: String, body: JSONObject? = null,
                        method: String = if (body == null) "GET" else "POST",
                        shouldStop: () -> Boolean = { false }): JSONObject {
        val connection = URL(base, path).openConnection() as HttpsURLConnection
        val control = RequestControl({ shouldStop() }, { connection.disconnect() }, 30_000)
        try {
            connection.requestMethod = method
            connection.instanceFollowRedirects = false
            connection.connectTimeout = 15_000
            connection.readTimeout = 15_000
            connection.setRequestProperty("Authorization", "Bearer $token")
            connection.setRequestProperty("X-Policy-Format", "2")
            control.checkActive()
            if (body != null) {
                connection.doOutput = true
                connection.setRequestProperty("Content-Type", "application/json")
                val bytes = body.toString().toByteArray(Charsets.UTF_8)
                require(bytes.size <= 1024 * 1024) { "Request too large" }
                connection.outputStream.use { it.write(bytes) }
            }
            control.checkActive()
            check(connection.responseCode in 200..299) { "Command sync failed (${connection.responseCode})" }
            control.checkActive()
            if (connection.responseCode == 204) return JSONObject()
            val bytes = connection.inputStream.use { com.appcontrol.transport.BoundedInput.read(it, 128 * 1024 + 1) }
            control.checkActive()
            check(bytes.size <= 128 * 1024) { "Server response too large" }
            return JSONObject(String(bytes, Charsets.UTF_8))
        } finally {
            control.close()
            connection.disconnect()
        }
    }

    companion object {
        private val syncLock = Any()
    }
}
