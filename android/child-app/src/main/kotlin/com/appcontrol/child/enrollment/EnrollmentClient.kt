package com.appcontrol.child.enrollment

import android.content.Context
import java.io.IOException
import org.json.JSONObject
import java.net.URL
import javax.net.ssl.HttpsURLConnection

/** Blocking client: call from a worker thread. No pairing code or challenge is persisted. */
class EnrollmentClient(context: Context) {
    private val state = context.applicationContext.getSharedPreferences("enrollment", Context.MODE_PRIVATE)
    private val identity = DeviceIdentity()

    fun enroll(server: String, code: String, displayName: String): String {
        check(state.getString("deviceId", null) == null) { "Device is already enrolled" }
        val base = URL(server.trim().trimEnd('/'))
        require(base.protocol == "https" && base.host.isNotBlank() && base.userInfo == null &&
            base.query == null && base.ref == null && (base.path.isEmpty() || base.path == "/")) {
            "Enter an HTTPS server origin"
        }
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
        return deviceId
    }

    /** Returns an in-memory bearer token. Never persist or log this value. */
    fun authenticate(): String {
        val deviceId = checkNotNull(state.getString("deviceId", null)) { "Enrollment required" }
        val base = URL(checkNotNull(state.getString("server", null)))
        require(base.protocol == "https" && base.userInfo == null)
        val challenge = post(base, "/v1/device-auth/challenges", JSONObject().put("deviceId", deviceId))
        val id = challenge.getString("challengeId")
        val value = challenge.getString("challenge")
        val session = post(base, "/v1/device-auth/sessions", JSONObject()
            .put("deviceId", deviceId).put("challengeId", id).put("challenge", value)
            .put("signature", identity.signAuthentication(deviceId, id, value)))
        return session.getString("accessToken")
    }

    private fun post(base: URL, path: String, body: JSONObject): JSONObject {
        val connection = URL(base, path).openConnection() as HttpsURLConnection
        try {
            connection.requestMethod = "POST"
            connection.instanceFollowRedirects = false
            connection.connectTimeout = 15_000
            connection.readTimeout = 15_000
            connection.doOutput = true
            connection.setRequestProperty("Content-Type", "application/json")
            connection.outputStream.use { it.write(body.toString().toByteArray(Charsets.UTF_8)) }
            check(connection.responseCode in 200..299) { "Enrollment request failed (${connection.responseCode})" }
            val bytes = connection.inputStream.use { com.appcontrol.transport.BoundedInput.read(it, 16 * 1024 + 1) }
            check(bytes.size <= 16 * 1024) { "Server response too large" }
            return JSONObject(String(bytes, Charsets.UTF_8))
        } finally {
            connection.disconnect()
        }
    }
}

