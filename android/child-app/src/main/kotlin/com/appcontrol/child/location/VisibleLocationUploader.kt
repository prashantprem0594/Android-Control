package com.appcontrol.child.location

import android.content.Context
import android.location.Location
import com.appcontrol.child.BuildConfig
import com.appcontrol.child.network.DeviceEnrollmentClient
import com.appcontrol.transport.ServerOrigin
import org.json.JSONObject
import java.net.URL
import java.time.Instant
import javax.net.ssl.HttpsURLConnection

/** Uploads only points produced by the running visible foreground service. */
internal class VisibleLocationUploader(context: Context) {
    private val app = context.applicationContext
    private val enrollment = app.getSharedPreferences("enrollment", Context.MODE_PRIVATE)

    fun upload(location: Location) {
        val deviceId = enrollment.getString("deviceId", null) ?: return
        check(deviceId.matches(Regex("dev_[A-Za-z0-9_-]{22}")))
        val base = ServerOrigin.require(enrollment.getString("server", null) ?: return, BuildConfig.SERVER_ORIGIN)
        val capturedAt = Instant.ofEpochMilli(location.time)
        val body = JSONObject()
            .put("latitudeE7", (location.latitude * 10_000_000.0).toLong())
            .put("longitudeE7", (location.longitude * 10_000_000.0).toLong())
            .put("accuracyMeters", location.accuracy.toDouble())
            .put("capturedAt", capturedAt.toString())
        DeviceEnrollmentClient(app).withAuthenticatedSession {
            token -> post(base, token, body)
        }
    }

    private fun post(base: URL, token: String, body: JSONObject) {
        val connection = URL(base, "/v1/device/visible-location").openConnection() as HttpsURLConnection
        try {
            val bytes = body.toString().toByteArray(Charsets.UTF_8)
            connection.requestMethod = "POST"
            connection.instanceFollowRedirects = false
            connection.connectTimeout = 15_000
            connection.readTimeout = 15_000
            connection.doOutput = true
            connection.setFixedLengthStreamingMode(bytes.size)
            connection.setRequestProperty("Content-Type", "application/json")
            connection.setRequestProperty("Authorization", "Bearer $token")
            connection.outputStream.use { it.write(bytes) }
            check(connection.responseCode in 200..299) { "Location sharing upload failed" }
        } finally { connection.disconnect() }
    }
}
