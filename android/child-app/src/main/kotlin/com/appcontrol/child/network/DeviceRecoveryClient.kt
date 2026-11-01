package com.appcontrol.child.network

import android.content.Context
import com.appcontrol.child.BuildConfig
import com.appcontrol.child.commands.RequestControl
import com.appcontrol.child.enrollment.DeviceIdentity
import com.appcontrol.child.enrollment.DeviceRecoveryIdentity
import com.appcontrol.transport.BoundedInput
import com.appcontrol.transport.ServerOrigin
import org.json.JSONObject
import java.net.URL
import java.time.Instant
import javax.net.ssl.HttpsURLConnection

/** Blocking recovery transport. Request tokens and challenges are intentionally memory-only. */
class DeviceRecoveryClient(context: Context) {
    private val app = context.applicationContext
    private val state = app.getSharedPreferences("enrollment", Context.MODE_PRIVATE)
    private val identity = DeviceIdentity()
    private val recoveryIdentity = DeviceRecoveryIdentity(app)

    data class Request(
        val server: String,
        val requestId: String,
        val requestToken: String,
        val challenge: String,
        val expiresAt: Instant,
    )

    sealed class Status {
        data object AwaitingParent : Status()
        data class Approved(val deviceId: String) : Status()
        data object Rejected : Status()
        data object Expired : Status()
    }

    fun bindAuthenticated(base: URL, bearer: String, shouldStop: () -> Boolean = { false }) {
        request(
            base, "/v1/device/recovery-binding", "PUT",
            JSONObject().put("recoveryHandle", recoveryIdentity.handle()), bearer, shouldStop,
        ).also { check(it.status == 204) { "Recovery binding failed (${it.status})" } }
    }

    fun start(server: String, shouldStop: () -> Boolean = { false }): Request {
        val base = ServerOrigin.require(server, BuildConfig.SERVER_ORIGIN)
        val response = request(
            base, "/v1/device-recovery/requests", "POST",
            JSONObject()
                .put("recoveryHandle", recoveryIdentity.handle())
                .put("publicKey", identity.publicKey())
                .put("keyAlgorithm", "ECDSA_P256_SHA256"),
            null, shouldStop,
        )
        check(response.status == 201) { "Recovery request failed (${response.status})" }
        val body = response.json()
        val requestId = body.getString("requestId").also(::requireOpaque)
        val token = body.getString("requestToken").also(::requireSecret)
        val challenge = body.getString("challenge").also(::requireSecret)
        val expiresAt = Instant.parse(body.getString("expiresAt"))
        check(expiresAt.isAfter(Instant.now())) { "Recovery request already expired" }
        return Request(base.toString(), requestId, token, challenge, expiresAt)
    }

    fun prove(request: Request, shouldStop: () -> Boolean = { false }) {
        check(Instant.now().isBefore(request.expiresAt)) { "Recovery request expired" }
        val base = ServerOrigin.require(request.server, BuildConfig.SERVER_ORIGIN)
        val response = request(
            base, "/v1/device-recovery/requests/${request.requestId}/proof", "POST",
            JSONObject()
                .put("requestToken", request.requestToken)
                .put("challenge", request.challenge)
                .put("signature", identity.signRecovery(request.requestId, request.challenge)),
            null, shouldStop,
        )
        check(response.status == 204) { "Recovery proof failed (${response.status})" }
    }

    fun status(request: Request, shouldStop: () -> Boolean = { false }): Status {
        val base = ServerOrigin.require(request.server, BuildConfig.SERVER_ORIGIN)
        val response = request(
            base, "/v1/device-recovery/requests/${request.requestId}/status", "POST",
            JSONObject().put("requestToken", request.requestToken), null, shouldStop,
        )
        check(response.status == 200) { "Recovery status failed (${response.status})" }
        val body = response.json()
        return when (body.getString("state")) {
            "awaiting_parent" -> Status.AwaitingParent
            "approved" -> Status.Approved(body.getString("deviceId").also(::requireDeviceId))
            "rejected" -> Status.Rejected
            "expired" -> Status.Expired
            else -> error("Invalid recovery state")
        }
    }

    fun saveApproved(request: Request, deviceId: String) {
        requireDeviceId(deviceId)
        val base = ServerOrigin.require(request.server, BuildConfig.SERVER_ORIGIN)
        check(state.getString("deviceId", null) == null) { "Device is already enrolled" }
        check(state.edit().putString("server", base.toString()).putString("deviceId", deviceId).commit()) {
            "Could not save recovered enrollment"
        }
    }

    private data class Response(val status: Int, val bytes: ByteArray) {
        fun json(): JSONObject {
            check(bytes.isNotEmpty()) { "Server response was empty" }
            return JSONObject(String(bytes, Charsets.UTF_8))
        }
    }

    private fun request(
        base: URL,
        path: String,
        method: String,
        body: JSONObject,
        bearer: String?,
        shouldStop: () -> Boolean,
    ): Response {
        val connection = URL(base, path).openConnection() as HttpsURLConnection
        val control = RequestControl({ shouldStop() }, { connection.disconnect() }, 30_000)
        try {
            connection.requestMethod = method
            connection.instanceFollowRedirects = false
            connection.connectTimeout = 15_000
            connection.readTimeout = 15_000
            connection.doOutput = true
            connection.setRequestProperty("Content-Type", "application/json")
            connection.setRequestProperty("Accept", "application/json")
            if (bearer != null) connection.setRequestProperty("Authorization", "Bearer $bearer")
            control.checkActive()
            connection.outputStream.use { it.write(body.toString().toByteArray(Charsets.UTF_8)) }
            control.checkActive()
            val status = connection.responseCode
            val stream = if (status in 200..299) connection.inputStream else connection.errorStream
            val bytes = stream?.use { BoundedInput.read(it, MAX_RESPONSE + 1) } ?: ByteArray(0)
            control.checkActive()
            check(bytes.size <= MAX_RESPONSE) { "Server response too large" }
            return Response(status, bytes)
        } finally {
            control.close()
            connection.disconnect()
        }
    }

    private fun requireOpaque(value: String) {
        require(value.length in 1..128 && value.all { it.isLetterOrDigit() || it == '_' || it == '-' })
    }

    private fun requireSecret(value: String) {
        require(value.length == 43 && value.all { it.isLetterOrDigit() || it == '_' || it == '-' })
    }

    private fun requireDeviceId(value: String) {
        require(Regex("dev_[A-Za-z0-9_-]{22}").matches(value))
    }

    companion object { private const val MAX_RESPONSE = 16 * 1024 }
}
