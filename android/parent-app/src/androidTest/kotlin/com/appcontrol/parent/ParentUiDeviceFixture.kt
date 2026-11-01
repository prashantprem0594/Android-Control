package com.appcontrol.parent

import android.content.Context
import com.appcontrol.transport.AcceptanceHttps
import java.net.URL
import java.security.KeyPairGenerator
import java.security.KeyPair
import java.security.KeyStore
import java.security.Signature
import java.security.spec.ECGenParameterSpec
import java.time.Instant
import java.util.Base64
import javax.net.ssl.HttpsURLConnection
import org.json.JSONObject

/** Server-only synthetic device. Its ephemeral memory key is never installed on a child. */
internal class ParentUiDeviceFixture(private val context: Context) {
    private val name = "fixture-parent-ui-admin"
    private val vault = SessionVault(context, name)
    private var touched = false
    private var ownsKey = false
    private var enrolledId: String? = null
    private var identity: KeyPair? = null
    lateinit var client: ParentClient
        private set

    fun prepare(): String {
        check(context.packageName == "com.appcontrol.parent.acceptance")
        check(context.getSharedPreferences(name, Context.MODE_PRIVATE).all.isEmpty())
        ownsKey = !KeyStore.getInstance("AndroidKeyStore").apply { load(null) }.containsAlias("app-control.$name")
        touched = true
        client = ParentClient(vault, { url -> (url.openConnection() as HttpsURLConnection).apply {
            sslSocketFactory = AcceptanceHttps.socketFactory()
        } }) { Instant.now() }
        client.login(BuildConfig.SERVER_ORIGIN, "acceptance-owner@example.test", "acceptance-fixture-password-not-secret")
        val code = client.pairingCode().getString("code")
        val key = KeyPairGenerator.getInstance("EC").apply { initialize(ECGenParameterSpec("secp256r1")) }.generateKeyPair()
        identity = key
        val encoder = Base64.getUrlEncoder().withoutPadding()
        val started = post("/v1/enrollments", JSONObject().put("code", code).put("displayName", "UI fixture device")
            .put("publicKey", encoder.encodeToString(key.public.encoded)).put("keyAlgorithm", "ECDSA_P256_SHA256"))
        val enrollmentId = started.getString("enrollmentId")
        val challenge = started.getString("challenge")
        val signature = Signature.getInstance("SHA256withECDSA").run {
            initSign(key.private)
            update("app-control-device-enrollment-v1\n$enrollmentId\n".toByteArray(Charsets.UTF_8))
            update(Base64.getUrlDecoder().decode(challenge))
            encoder.encodeToString(sign())
        }
        val completed = post("/v1/enrollments/complete", JSONObject().put("enrollmentId", enrollmentId)
            .put("challenge", challenge).put("signature", signature))
        enrolledId = completed.getString("id")
        check(enrolledId == started.getString("deviceId") && completed.getString("state") == "active")
        return checkNotNull(enrolledId)
    }

    fun reportRinging(callId: String?) {
        val id = checkNotNull(enrolledId)
        val challenge = post("/v1/device-auth/challenges", JSONObject().put("deviceId", id))
        val challengeId = challenge.getString("challengeId")
        val value = challenge.getString("challenge")
        val signature = Signature.getInstance("SHA256withECDSA").run {
            initSign(checkNotNull(identity).private)
            update("app-control-device-auth-v1\n$id\n$challengeId\n".toByteArray(Charsets.UTF_8))
            update(Base64.getUrlDecoder().decode(value))
            Base64.getUrlEncoder().withoutPadding().encodeToString(sign())
        }
        val token = post("/v1/device-auth/sessions", JSONObject().put("deviceId", id).put("challengeId", challengeId)
            .put("challenge", value).put("signature", signature)).getString("accessToken")
        val sequence = post("/v1/device/calls/sequence", JSONObject(), token).getLong("sequence")
        val calls = org.json.JSONArray()
        if (callId != null) calls.put(JSONObject().put("callId", callId).put("state", "RINGING").put("remoteActionsAllowed", false))
        check(post("/v1/device/calls", JSONObject().put("sequence", sequence).put("calls", calls), token).getBoolean("accepted"))
    }

    private fun post(path: String, body: JSONObject, bearer: String? = null): JSONObject {
        val connection = URL(BuildConfig.SERVER_ORIGIN + path).openConnection() as HttpsURLConnection
        try {
            connection.sslSocketFactory = AcceptanceHttps.socketFactory()
            connection.requestMethod = "POST"
            connection.instanceFollowRedirects = false
            connection.connectTimeout = 10_000; connection.readTimeout = 10_000
            connection.doOutput = true
            connection.setRequestProperty("Content-Type", "application/json")
            if (bearer != null) connection.setRequestProperty("Authorization", "Bearer $bearer")
            connection.outputStream.use { it.write(body.toString().toByteArray(Charsets.UTF_8)) }
            check(connection.responseCode in 200..299)
            val bytes = connection.inputStream.use { it.readNBytes(16 * 1024 + 1) }
            check(bytes.size <= 16 * 1024)
            return JSONObject(String(bytes, Charsets.UTF_8))
        } finally { connection.disconnect() }
    }

    fun close() {
        if (!touched) return
        var failed = false
        try { enrolledId?.let { client.revoke(it) } } catch (_: Exception) { failed = true }
        try { if (::client.isInitialized && !client.logout()) failed = true } catch (_: Exception) { failed = true }
        try {
            vault.clear()
            if (ownsKey) KeyStore.getInstance("AndroidKeyStore").apply { load(null); deleteEntry("app-control.$name") }
            enrolledId?.let { id ->
                for (preferences in listOf("parent-pending-lock", "parent-command-history")) {
                    check(context.getSharedPreferences(preferences, Context.MODE_PRIVATE).edit().remove(id).commit())
                }
            }
        } catch (_: Exception) { failed = true }
        identity = null
        check(!failed)
    }
}
