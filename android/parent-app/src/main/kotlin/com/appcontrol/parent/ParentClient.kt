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
    constructor(context: Context) : this(SessionVault(context), { it.openConnection() as HttpsURLConnection }, 30_000, Instant::now)
    class RequestFailure(val status: Int) : Exception("Request failed")
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
        } catch (failure: Exception) { vault.clear(); throw SignInRequired() }
        if (now().plusSeconds(30).isBefore(Instant.parse(current.getString("accessExpiresAt")))) return current
        if (!now().isBefore(Instant.parse(current.getString("refreshExpiresAt")))) { vault.clear(); throw SignInRequired() }
        // Never automatically retry a rotating refresh token after an ambiguous response.
        return try {
            val next = request(base, "/v1/auth/refresh", "POST", JSONObject().put("refreshToken", current.getString("refreshToken")))
            next.put("email", current.getString("email")); validate(next); vault.save(base, next); next
        } catch (failure: Exception) { vault.clear(); throw SignInRequired() }
    }
    private fun authenticated(path: String, method: String = "GET", body: JSONObject? = null, key: String? = null): JSONObject = synchronized(sessionLock) {
        val base = origin(vault.origin() ?: throw SignInRequired())
        val tokens = session(base)
        try { request(base, path, method, body, tokens.getString("accessToken"), key) }
        catch (failure: RequestFailure) {
            if (failure.status == 401) { vault.clear(); throw SignInRequired() }; throw failure
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
    fun policy(id: String): JSONObject = authenticated(device(id) + "/policy")
    fun apps(id: String): JSONObject = authenticated(device(id) + "/apps")
    fun status(id: String): JSONObject = authenticated(device(id) + "/status")
    fun remoteCalls(id: String): JSONObject = authenticated(device(id) + "/calls")
    fun callContacts(id: String): JSONObject = authenticated(device(id) + "/call-contacts")
    fun saveCallContacts(id: String, expectedVersion: Long, contacts: org.json.JSONArray): JSONObject =
        authenticated(device(id) + "/call-contacts", "PUT", JSONObject().put("expectedVersion", expectedVersion).put("contacts", contacts))
    fun savePolicy(id: String, policy: JSONObject): JSONObject = authenticated(device(id) + "/policy", "PUT", policy)
    fun lock(id: String, key: String): JSONObject = authenticated(device(id) + "/commands", "POST",
        JSONObject().put("type", "LOCK_DEVICE").put("ttlSeconds", 60), key)
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
    fun logout(): Boolean = synchronized(sessionLock) {
        var revoked = false
        try {
            val base = origin(vault.origin() ?: return@synchronized true)
            val tokens = session(base)
            request(base, "/v1/auth/logout", "POST", JSONObject(), tokens.getString("accessToken")); revoked = true
        } catch (_: Exception) { } finally { vault.clear() }
        revoked
    }
    private fun request(base: String, path: String, method: String, body: JSONObject? = null,
                        token: String? = null, key: String? = null): JSONObject {
        val checked = ServerOrigin.require(base, BuildConfig.SERVER_ORIGIN)
        val connection = openConnection(URL(checked, path))
        val requester = Thread.currentThread()
        val deadline = RequestDeadline({ requester.isInterrupted }, { connection.disconnect() }, requestTimeoutMillis)
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
                connection.setFixedLengthStreamingMode(bytes.size); connection.outputStream.use { it.write(bytes) }
            }
            deadline.checkActive()
            val status = connection.responseCode
            deadline.checkActive()
            if (status !in 200..299) throw RequestFailure(status)
            if (status == 204) return JSONObject()
            val bytes = connection.inputStream.use { it.readNBytes(1024 * 1024 + 1) }
            deadline.checkActive()
            check(bytes.size <= 1024 * 1024) { "Server response too large" }
            return JSONObject(String(bytes, Charsets.UTF_8))
        } finally { deadline.close(); connection.disconnect() }
    }
    companion object { private val sessionLock = Any() }
}
