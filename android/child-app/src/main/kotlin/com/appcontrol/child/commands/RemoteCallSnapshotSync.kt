package com.appcontrol.child.commands

import android.content.Context
import com.appcontrol.child.BuildConfig
import com.appcontrol.child.calls.CallRegistry
import com.appcontrol.child.network.DeviceEnrollmentClient
import com.appcontrol.transport.ServerOrigin
import org.json.JSONArray
import org.json.JSONObject
import java.net.URL
import javax.net.ssl.HttpsURLConnection

/** Publishes only the foreground Telecom identity/state snapshot, without policy or inventory work. */
class RemoteCallSnapshotSync(context: Context) {
    private val app = context.applicationContext

    fun publish(shouldStop: () -> Boolean = { false }) {
        check(!shouldStop()) { "Call snapshot stopped" }
        val enrollment = app.getSharedPreferences("enrollment", Context.MODE_PRIVATE)
        val base = ServerOrigin.require(checkNotNull(enrollment.getString("server", null)), BuildConfig.SERVER_ORIGIN)
        val ticket = requestAuthenticated(base, "/v1/device/calls/sequence", JSONObject(), shouldStop)
        val sequence = ticket.getLong("sequence")
        require(sequence > 0)
        val calls = JSONArray().apply {
            CallRegistry.remoteSnapshot().forEach { call ->
                put(JSONObject()
                    .put("callId", call.callId)
                    .put("state", call.state)
                    .put("remoteActionsAllowed", call.remoteActionsAllowed))
            }
        }
        check(!shouldStop()) { "Call snapshot stopped" }
        val response = requestAuthenticated(base, "/v1/device/calls",
            JSONObject().put("sequence", sequence).put("calls", calls), shouldStop)
        response.getString("status").also { require(it == "received") }
    }

    private fun requestAuthenticated(base: URL, path: String, body: JSONObject,
                                     shouldStop: () -> Boolean): JSONObject =
        DeviceEnrollmentClient(app).withAuthenticatedSession(shouldStop) { token ->
            request(base, path, token, body, shouldStop)
        }

    private fun request(base: URL, path: String, token: String, body: JSONObject,
                        shouldStop: () -> Boolean): JSONObject {
        val connection = URL(base, path).openConnection() as HttpsURLConnection
        val control = RequestControl({ shouldStop() }, { connection.disconnect() }, 15_000)
        try {
            connection.requestMethod = "POST"
            connection.instanceFollowRedirects = false
            connection.connectTimeout = 10_000
            connection.readTimeout = 10_000
            connection.useCaches = false
            connection.doOutput = true
            connection.setRequestProperty("Authorization", "******")
            connection.setRequestProperty("Content-Type", "application/json")
            val bytes = body.toString().toByteArray(Charsets.UTF_8)
            require(bytes.size <= 4096)
            control.checkActive()
            connection.outputStream.use { it.write(bytes) }
            control.checkActive()
            check(connection.responseCode in 200..299) { "Call snapshot request failed (${connection.responseCode})" }
            val response = connection.inputStream.use { com.appcontrol.transport.BoundedInput.read(it, 16 * 1024 + 1) }
            control.checkActive()
            check(response.size <= 16 * 1024) { "Call snapshot response too large" }
            return JSONObject(String(response, Charsets.UTF_8))
        } finally {
            control.close()
            connection.disconnect()
        }
    }
}
