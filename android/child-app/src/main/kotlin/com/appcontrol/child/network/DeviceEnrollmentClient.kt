package com.appcontrol.child.network

import android.content.Context
import com.appcontrol.child.enrollment.DeviceIdentity
import com.appcontrol.child.BuildConfig
import com.appcontrol.transport.ServerOrigin
import com.appcontrol.child.commands.RequestControl
import org.json.JSONObject
import java.net.URL
import java.time.Instant
import javax.net.ssl.HttpsURLConnection

/** Blocking client: call from a worker thread. No pairing code or challenge is persisted. */
class DeviceEnrollmentClient(context: Context) {
<<<<<<< HEAD
    private val appContext = context.applicationContext
    private val state = appContext.getSharedPreferences("enrollment", Context.MODE_PRIVATE)
=======
    private val state = context.applicationContext.getSharedPreferences("enrollment", Context.MODE_PRIVATE)
>>>>>>> origin/main
    private val identity = DeviceIdentity()

    fun enroll(server: String, code: String, displayName: String): String {
        check(state.getString("deviceId", null) == null) { "Device is already enrolled" }
        val base = ServerOrigin.require(server, BuildConfig.SERVER_ORIGIN)
        val started = post(base, "/v1/enrollments", JSONObject()
            .put("code", code.trim()).put("displayName", displayName.trim())
            .put("publicKey", identity.publicKey()).put("keyAlgorithm", "ECDSA_P256_SHA256"))
        val id = started.getString("enrollmentId")
        val challenge = started.getString("challenge")
        require(started.getString("keyAlgorithm") == "ECDSA_P256_SHA256")
        val completed = post(base, "/v1/enrollments/complete", JSONObject()
            .put("enrollmentId", id).put("challenge", challenge)
            .put("signature", identity.sign(id, challenge)))
        val deviceId = completed.getString("id")
        require(deviceId == started.getString("deviceId") && completed.getString("state") == "active")
        check(state.edit().putString("server", base.toString()).putString("deviceId", deviceId).commit()) {
            "Could not save enrollment state"
        }
<<<<<<< HEAD
        // Enrollment is already durable if this best-effort registration is temporarily unavailable.
        // Every authenticated sync retries it.
        runCatching { bindRecoveryHandle() }
        return deviceId
    }

    fun bindRecoveryHandle(shouldStop: () -> Boolean = { false }) {
        val base = ServerOrigin.require(checkNotNull(state.getString("server", null)), BuildConfig.SERVER_ORIGIN)
        withAuthenticatedSession(shouldStop) { token ->
            DeviceRecoveryClient(appContext).bindAuthenticated(base, token, shouldStop)
        }
    }

=======
        return deviceId
    }

>>>>>>> origin/main
    /** Returns an in-memory bearer token. Never persist or log this value. */
    fun authenticate(shouldStop: () -> Boolean = { false }): String = synchronized(sessionLock) {
        check(!shouldStop()) { "Authentication stopped" }
        val deviceId = checkNotNull(state.getString("deviceId", null)) { "Enrollment required" }
        val base = ServerOrigin.require(checkNotNull(state.getString("server", null)), BuildConfig.SERVER_ORIGIN)
        val cached = cachedSession
        if (cached != null && cached.deviceId == deviceId && cached.server == base.toString() &&
            Instant.now().plusSeconds(60).isBefore(cached.expiresAt)) {
            return@synchronized cached.accessToken
        }
        val challenge = post(base, "/v1/device-auth/challenges", JSONObject().put("deviceId", deviceId), shouldStop)
        val id = challenge.getString("challengeId")
        val value = challenge.getString("challenge")
        check(!shouldStop()) { "Authentication stopped" }
        val session = post(base, "/v1/device-auth/sessions", JSONObject()
            .put("deviceId", deviceId).put("challengeId", id).put("challenge", value)
            .put("signature", identity.signAuthentication(deviceId, id, value)), shouldStop)
        val accessToken = session.getString("accessToken")
        val expiresAt = Instant.parse(session.getString("expiresAt"))
        check(expiresAt.isAfter(Instant.now().plusSeconds(60))) { "Device session lifetime is too short" }
        cachedSession = CachedSession(deviceId, base.toString(), accessToken, expiresAt)
        accessToken
    }

    fun <T> withAuthenticatedSession(shouldStop: () -> Boolean = { false }, action: (String) -> T): T =
        synchronized(sessionLock) {
            action(authenticate(shouldStop))
        }

    private fun post(base: URL, path: String, body: JSONObject, shouldStop: () -> Boolean = { false }): JSONObject {
        val connection = URL(base, path).openConnection() as HttpsURLConnection
        val control = RequestControl({ shouldStop() }, { connection.disconnect() }, 30_000)
        try {
            connection.requestMethod = "POST"
            connection.instanceFollowRedirects = false
            connection.connectTimeout = 15_000
            connection.readTimeout = 15_000
            connection.doOutput = true
            connection.setRequestProperty("Content-Type", "application/json")
            control.checkActive()
            connection.outputStream.use { it.write(body.toString().toByteArray(Charsets.UTF_8)) }
            control.checkActive()
            check(connection.responseCode in 200..299) { "Enrollment request failed (${connection.responseCode})" }
<<<<<<< HEAD
            val bytes = connection.inputStream.use { com.appcontrol.transport.BoundedInput.read(it, 16 * 1024 + 1) }
=======
            val bytes = connection.inputStream.use { it.readNBytes(16 * 1024 + 1) }
>>>>>>> origin/main
            control.checkActive()
            check(bytes.size <= 16 * 1024) { "Server response too large" }
            return JSONObject(String(bytes, Charsets.UTF_8))
        } finally {
            control.close()
            connection.disconnect()
        }

    }

    private data class CachedSession(val deviceId: String, val server: String, val accessToken: String, val expiresAt: Instant)

    companion object {
        private val sessionLock = Any()
        private var cachedSession: CachedSession? = null
    }
}
