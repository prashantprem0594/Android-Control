package com.appcontrol.parent

import android.content.Context
import com.appcontrol.transport.ServerOrigin
import com.appcontrol.transport.RequestDeadline
import java.net.URL
import java.time.Instant
import java.util.UUID
import javax.net.ssl.HttpsURLConnection
import org.json.JSONObject

class ParentClient internal constructor(
    private val vault: SessionVault,
    private val openConnection: (URL) -> HttpsURLConnection,
    private val requestTimeoutMillis: Long = 30_000,
    private val now: () -> Instant,
) {
<<<<<<< HEAD
    private var lifecycleContext: Context? = null
    private var shouldStop: () -> Boolean = { false }
    constructor(context: Context) : this(SessionVault(context), { it.openConnection() as HttpsURLConnection }, 30_000, Instant::now) {
        lifecycleContext = context.applicationContext
    }
    constructor(context: Context, requestTimeoutMillis: Long, cancelled: () -> Boolean) :
        this(SessionVault(context), { it.openConnection() as HttpsURLConnection }, requestTimeoutMillis, Instant::now) {
        lifecycleContext = context.applicationContext
        shouldStop = cancelled
    }
    class RequestFailure(val status: Int, val callRequestNotQueued: Boolean = false) : Exception("Request failed")
    class ServerUnavailable(cause: Exception) : Exception("Could not connect to the server", cause)
=======
    constructor(context: Context) : this(SessionVault(context), { it.openConnection() as HttpsURLConnection }, 30_000, Instant::now)
    class RequestFailure(val status: Int) : Exception("Request failed")
>>>>>>> origin/main
    class SignInRequired : Exception("Please sign in again")
    fun hasSession(): Boolean = vault.origin() != null
    fun server(): String = vault.origin() ?: BuildConfig.SERVER_ORIGIN
    private fun origin(value: String): String = ServerOrigin.require(value, BuildConfig.SERVER_ORIGIN).toString()
    fun register(server: String, email: String, password: String) = synchronized(sessionLock) {
        request(origin(server), "/v1/auth/register", "POST", JSONObject().put("email", email.trim()).put("password", password))
    }
    fun login(server: String, email: String, password: String) = synchronized(sessionLock) {
        val base = origin(server)
        val response = request(base, "/v1/auth/login", "POST", JSONObject().put("email", email.trim()).put("password", password))
        val tokens = response.getJSONObject("tokens")
        tokens.put("email", response.getJSONObject("parent").getString("email"))
        validate(tokens); vault.save(base, tokens)
<<<<<<< HEAD
        lifecycleContext?.let { runCatching { com.appcontrol.parent.push.ParentPushWork.newSession(it) } }
=======
>>>>>>> origin/main
    }
    private fun validate(tokens: JSONObject) {
        require(tokens.getString("accessToken").length in 16..2048 && tokens.getString("refreshToken").length in 16..2048)
        require(Instant.parse(tokens.getString("accessExpiresAt")).isAfter(now()))
        require(Instant.parse(tokens.getString("refreshExpiresAt")).isAfter(now()))
    }
    private fun session(base: String): JSONObject {
        val current = try {
            vault.load(base).also {
                require(it.getString("accessToken").length in 16..2048 && it.getString("refreshToken").length in 16..2048)
                Instant.parse(it.getString("accessExpiresAt")); Instant.parse(it.getString("refreshExpiresAt"))
            }
<<<<<<< HEAD
        } catch (failure: Exception) { clearSession(); throw SignInRequired() }
        if (now().plusSeconds(30).isBefore(Instant.parse(current.getString("accessExpiresAt")))) return current
        if (!now().isBefore(Instant.parse(current.getString("refreshExpiresAt")))) { clearSession(); throw SignInRequired() }
=======
        } catch (failure: Exception) { vault.clear(); throw SignInRequired() }
        if (now().plusSeconds(30).isBefore(Instant.parse(current.getString("accessExpiresAt")))) return current
        if (!now().isBefore(Instant.parse(current.getString("refreshExpiresAt")))) { vault.clear(); throw SignInRequired() }
>>>>>>> origin/main
        // Never automatically retry a rotating refresh token after an ambiguous response.
        return try {
            val next = request(base, "/v1/auth/refresh", "POST", JSONObject().put("refreshToken", current.getString("refreshToken")))
            next.put("email", current.getString("email")); validate(next); vault.save(base, next); next
<<<<<<< HEAD
        } catch (failure: ServerUnavailable) { throw failure }
        catch (failure: Exception) { clearSession(); throw SignInRequired() }
    }
    private fun authenticated(path: String, method: String = "GET", body: JSONObject? = null, key: String? = null): JSONObject = synchronized(sessionLock) {
        check(!shouldStop()) { "Request stopped" }
        val base = origin(vault.origin() ?: throw SignInRequired())
        val tokens = session(base)
        check(!shouldStop()) { "Request stopped" }
        try { request(base, path, method, body, tokens.getString("accessToken"), key) }
        catch (failure: RequestFailure) {
            if (failure.status == 401) { clearSession(); throw SignInRequired() }; throw failure
=======
        } catch (failure: Exception) { vault.clear(); throw SignInRequired() }
    }
    private fun authenticated(path: String, method: String = "GET", body: JSONObject? = null, key: String? = null): JSONObject = synchronized(sessionLock) {
        val base = origin(vault.origin() ?: throw SignInRequired())
        val tokens = session(base)
        try { request(base, path, method, body, tokens.getString("accessToken"), key) }
        catch (failure: RequestFailure) {
            if (failure.status == 401) { vault.clear(); throw SignInRequired() }; throw failure
>>>>>>> origin/main
        }
    }
    fun devices(after: String = ""): JSONObject {
        require(after.isEmpty() || after.matches(Regex("dev_[A-Za-z0-9_-]{22}")))
        return authenticated("/v1/devices?limit=50" + if (after.isEmpty()) "" else "&after=$after")
    }
    private fun device(id: String): String {
        require(id.matches(Regex("dev_[A-Za-z0-9_-]{22}"))); return "/v1/devices/$id"
    }
    fun pairingCode(): JSONObject = authenticated("/v1/pairing/codes", "POST", JSONObject())
<<<<<<< HEAD
    fun revokeDevice(id: String) = authenticated(device(id) + "/revoke", "POST", JSONObject())
    fun listRecoveryRequests(): JSONObject = authenticated("/v1/device-recovery/requests")
    private fun recoveryRequest(id: String): String {
        require(id.matches(Regex("[A-Za-z0-9_-]{16,128}")))
        return "/v1/device-recovery/requests/$id"
    }
    fun approveRecoveryRequest(id: String) = authenticated(recoveryRequest(id) + "/approve", "POST")
    fun rejectRecoveryRequest(id: String) = authenticated(recoveryRequest(id) + "/reject", "POST")
    fun policy(id: String): JSONObject = authenticated(device(id) + "/policy")
    fun apps(id: String): JSONObject = authenticated(device(id) + "/apps")
    fun status(id: String): JSONObject = authenticated(device(id) + "/status")
    /** Reads a point from the child's own visible sharing session; it cannot start sharing. */
    fun visibleLocation(id: String): JSONObject = authenticated(device(id) + "/visible-location")
    fun remoteCalls(id: String): JSONObject = authenticated(device(id) + "/calls")
    fun parentCallEvents(): JSONObject = authenticated("/v1/parent/call-events")
    fun registerParentWakeup(registrationToken: String) {
        require(registrationToken.length in 1..4096 && registrationToken.all { it.code in 33..126 })
        authenticated("/v1/parent/wakeup-registration", "PUT", JSONObject().put("registrationToken", registrationToken))
    }
    fun removeParentWakeup() { authenticated("/v1/parent/wakeup-registration", "DELETE") }
=======
    fun policy(id: String): JSONObject = authenticated(device(id) + "/policy")
    fun apps(id: String): JSONObject = authenticated(device(id) + "/apps")
    fun status(id: String): JSONObject = authenticated(device(id) + "/status")
    fun remoteCalls(id: String): JSONObject = authenticated(device(id) + "/calls")
>>>>>>> origin/main
    fun callContacts(id: String): JSONObject = authenticated(device(id) + "/call-contacts")
    fun saveCallContacts(id: String, expectedVersion: Long, contacts: org.json.JSONArray): JSONObject =
        authenticated(device(id) + "/call-contacts", "PUT", JSONObject().put("expectedVersion", expectedVersion).put("contacts", contacts))
    fun savePolicy(id: String, policy: JSONObject): JSONObject = authenticated(device(id) + "/policy", "PUT", policy)
    fun lock(id: String, key: String): JSONObject = authenticated(device(id) + "/commands", "POST",
        JSONObject().put("type", "LOCK_DEVICE").put("ttlSeconds", 60), key)
<<<<<<< HEAD
    fun openApp(id: String, packageName: String, key: String): JSONObject {
        require(packageName.matches(Regex("[A-Za-z_][A-Za-z0-9_]*(\\.[A-Za-z_][A-Za-z0-9_]*)+")))
        require(key.matches(Regex("[A-Za-z0-9_-]{16,128}")))
        return authenticated(device(id) + "/commands", "POST",
            JSONObject().put("type", "OPEN_APP").put("packageName", packageName).put("ttlSeconds", 1800), key)
    }
    fun openSettings(id: String, key: String): JSONObject {
        require(key.matches(Regex("[A-Za-z0-9_-]{16,128}")))
        return authenticated(device(id) + "/commands", "POST",
            JSONObject().put("type", "OPEN_SETTINGS").put("ttlSeconds", 1800), key)
    }
    fun remoteScreenshot(id: String, key: String): JSONObject {
        require(key.matches(Regex("[A-Za-z0-9_-]{16,128}")))
        return authenticated(device(id) + "/commands", "POST",
            JSONObject().put("type", "REMOTE_SCREENSHOT").put("ttlSeconds", 120), key)
    }
    fun remoteTap(id: String, x: Int, y: Int, key: String): JSONObject {
        require(x in 0..10000 && y in 0..10000 && key.matches(Regex("[A-Za-z0-9_-]{16,128}")))
        return authenticated(device(id) + "/commands", "POST",
            JSONObject().put("type", "REMOTE_TAP").put("x", x).put("y", y).put("ttlSeconds", 30), key)
    }
    fun remoteSwipe(id: String, x: Int, y: Int, endX: Int, endY: Int, durationMillis: Int, key: String): JSONObject {
        require(x in 0..10000 && y in 0..10000 && endX in 0..10000 && endY in 0..10000)
        require(durationMillis in 50..2000 && key.matches(Regex("[A-Za-z0-9_-]{16,128}")))
        return authenticated(device(id) + "/commands", "POST", JSONObject()
            .put("type", "REMOTE_SWIPE").put("x", x).put("y", y).put("endX", endX).put("endY", endY)
            .put("durationMillis", durationMillis).put("ttlSeconds", 30), key)
    }
    fun remoteLongPress(id: String, x: Int, y: Int, durationMillis: Int, key: String): JSONObject {
        require(x in 0..10000 && y in 0..10000 && durationMillis in 300..2000 && key.matches(Regex("[A-Za-z0-9_-]{16,128}")))
        return authenticated(device(id) + "/commands", "POST", JSONObject()
            .put("type", "REMOTE_LONG_PRESS").put("x", x).put("y", y).put("durationMillis", durationMillis).put("ttlSeconds", 30), key)
    }
    fun remoteGlobal(id: String, action: String, key: String): JSONObject {
        require(action in setOf("back", "home", "recents") && key.matches(Regex("[A-Za-z0-9_-]{16,128}")))
        return authenticated(device(id) + "/commands", "POST", JSONObject()
            .put("type", "REMOTE_GLOBAL").put("action", action).put("ttlSeconds", 30), key)
    }
    fun remoteScreen(id: String, commandId: String): JSONObject {
        require(UUID.fromString(commandId).toString() == commandId)
        return authenticated(device(id) + "/remote-assistance/screens/$commandId")
    }
    /** Starts a short-lived signaling session; the child still has to approve screen sharing locally. */
    fun remoteSessionStart(id: String, ttlSeconds: Int = 300, key: String): JSONObject {
        require(ttlSeconds in 30..900 && key.matches(Regex("[A-Za-z0-9_-]{16,128}")))
        return authenticated(device(id) + "/remote-assistance/sessions", "POST",
            JSONObject().put("ttlSeconds", ttlSeconds), key)
    }
    fun remoteSession(id: String, sessionId: String): JSONObject {
        require(UUID.fromString(sessionId).toString() == sessionId)
        return authenticated(device(id) + "/remote-assistance/sessions/$sessionId")
    }
    fun remoteSessionSignal(id: String, sessionId: String, signal: JSONObject, key: String): JSONObject {
        require(UUID.fromString(sessionId).toString() == sessionId)
        require(signal.length() in 1..8 && signal.toString().toByteArray(Charsets.UTF_8).size <= 65536)
        require(key.matches(Regex("[A-Za-z0-9_-]{16,128}")))
        require(signal.has("sdp"))
        return authenticated(device(id) + "/remote-assistance/sessions/$sessionId/offer", "PUT", signal, key)
    }
    fun remoteSessionStop(id: String, sessionId: String): JSONObject {
        require(UUID.fromString(sessionId).toString() == sessionId)
        return authenticated(device(id) + "/remote-assistance/sessions/$sessionId/close", "POST")
    }
=======
>>>>>>> origin/main
    fun callAction(id: String, callId: String, action: String, ttlSeconds: Int, key: String): JSONObject {
        val parsedCallId = UUID.fromString(callId)
        require(parsedCallId.toString() == callId && parsedCallId.version() == 4 && parsedCallId.variant() == 2)
        require(action in setOf("ANSWER_CALL", "REJECT_CALL", "END_CALL"))
        require(ttlSeconds in 1..30)
        require(key.matches(Regex("[A-Za-z0-9_-]{16,128}")))
        return authenticated(device(id) + "/commands", "POST",
            JSONObject().put("type", action).put("callId", callId).put("ttlSeconds", ttlSeconds), key)
    }
    fun command(id: String, commandId: String): JSONObject {
        require(UUID.fromString(commandId).toString() == commandId)
        return authenticated(device(id) + "/commands/$commandId")
    }
    fun history(id: String, after: String = ""): JSONObject {
        require(after.length <= 256 && after.matches(Regex("[A-Za-z0-9_-]*")))
        return authenticated(device(id) + "/commands?limit=50" + if (after.isEmpty()) "" else "&after=$after")
    }
    fun revoke(id: String) { authenticated(device(id) + "/revoke", "POST", JSONObject()) }
    /** False means local sign-out completed but server revocation could not be confirmed. */
<<<<<<< HEAD
    fun logout(): Boolean {
        // Invalidate publication before waiting for an in-flight authenticated request.
        lifecycleContext?.let { runCatching { com.appcontrol.parent.push.ParentPushWork.stop(it) } }
        return synchronized(sessionLock) {
=======
    fun logout(): Boolean = synchronized(sessionLock) {
>>>>>>> origin/main
        var revoked = false
        try {
            val base = origin(vault.origin() ?: return@synchronized true)
            val tokens = session(base)
            request(base, "/v1/auth/logout", "POST", JSONObject(), tokens.getString("accessToken")); revoked = true
<<<<<<< HEAD
        } catch (_: Exception) { } finally { clearSession() }
        revoked
        }
    }
    private fun clearSession() {
        lifecycleContext?.let { runCatching { com.appcontrol.parent.push.ParentPushWork.stop(it) } }
        vault.clear()
=======
        } catch (_: Exception) { } finally { vault.clear() }
        revoked
>>>>>>> origin/main
    }
    private fun request(base: String, path: String, method: String, body: JSONObject? = null,
                        token: String? = null, key: String? = null): JSONObject {
        val checked = ServerOrigin.require(base, BuildConfig.SERVER_ORIGIN)
        val connection = openConnection(URL(checked, path))
        val requester = Thread.currentThread()
<<<<<<< HEAD
        val deadline = RequestDeadline({ requester.isInterrupted || shouldStop() }, { connection.disconnect() }, requestTimeoutMillis)
=======
        val deadline = RequestDeadline({ requester.isInterrupted }, { connection.disconnect() }, requestTimeoutMillis)
>>>>>>> origin/main
        try {
            deadline.checkActive()
            connection.requestMethod = method; connection.instanceFollowRedirects = false
            connection.connectTimeout = 15000; connection.readTimeout = 15000; connection.useCaches = false
            connection.setRequestProperty("Accept", "application/json")
            if (token != null) connection.setRequestProperty("Authorization", "Bearer $token")
            if (key != null) connection.setRequestProperty("Idempotency-Key", key)
            if (body != null) {
                val bytes = body.toString().toByteArray(Charsets.UTF_8); require(bytes.size <= 65536)
                connection.doOutput = true; connection.setRequestProperty("Content-Type", "application/json")
<<<<<<< HEAD
                connection.setFixedLengthStreamingMode(bytes.size)
            }
            // A failed connection attempt cannot have consumed a rotating refresh token.
            try { connection.connect() } catch (failure: java.io.IOException) { throw ServerUnavailable(failure) }
            if (body != null) {
                val bytes = body.toString().toByteArray(Charsets.UTF_8)
                connection.outputStream.use { it.write(bytes) }
=======
                connection.setFixedLengthStreamingMode(bytes.size); connection.outputStream.use { it.write(bytes) }
>>>>>>> origin/main
            }
            deadline.checkActive()
            val status = connection.responseCode
            deadline.checkActive()
<<<<<<< HEAD
            if (status !in 200..299) {
                // Only these two contract errors follow the existing-command lookup.
                // A disabled rollout, unknown conflict or unreadable response is ambiguous.
                val notQueued = if (status == 409) runCatching {
                    val bytes = connection.errorStream?.use { it.readNBytes(4097) }
                    deadline.checkActive()
                    if (bytes == null || bytes.size > 4096) false else {
                        // Match the exact single-field server contract; duplicate keys,
                        // trailing data and other permissive JSON forms remain ambiguous.
                        Regex("""[ \t\r\n]*\{[ \t\r\n]*"error"[ \t\r\n]*:[ \t\r\n]*"(?:call_not_current|call_state_conflict)"[ \t\r\n]*\}[ \t\r\n]*""")
                            .matches(String(bytes, Charsets.UTF_8))
                    }
                }.getOrDefault(false) else false
                throw RequestFailure(status, notQueued)
            }
=======
            if (status !in 200..299) throw RequestFailure(status)
>>>>>>> origin/main
            if (status == 204) return JSONObject()
            val bytes = connection.inputStream.use { it.readNBytes(1024 * 1024 + 1) }
            deadline.checkActive()
            check(bytes.size <= 1024 * 1024) { "Server response too large" }
            return JSONObject(String(bytes, Charsets.UTF_8))
        } finally { deadline.close(); connection.disconnect() }
    }
    companion object { private val sessionLock = Any() }
}
