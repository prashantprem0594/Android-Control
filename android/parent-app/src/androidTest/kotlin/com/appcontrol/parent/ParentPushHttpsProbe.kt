package com.appcontrol.parent

import android.content.Context
import android.content.pm.ApplicationInfo
import android.os.Build
import android.os.Bundle
import com.appcontrol.transport.AcceptanceHttps
import java.net.URL
import java.security.KeyStore
import java.time.Duration
import java.time.Instant
import java.util.UUID
import javax.net.ssl.HttpsURLConnection
import org.json.JSONObject

/** Fixture HTTPS APIs only. No Firebase provider, Android call, or child command is invoked. */
object ParentPushHttpsProbe {
    private const val OTHER_PREFS = "fixture-parent-push-other"
    private const val OTHER_EMAIL = "acceptance-other@example.test"
    private const val PASSWORD = "acceptance-fixture-password-not-secret"

    @JvmStatic fun run(context: Context): Bundle {
        val result = Bundle()
        var phase = "guard"
        var proofPassed = false
        var fixture: ParentUiDeviceFixture? = null
        var otherVault: SessionVault? = null
        var other: ParentClient? = null
        var ownerClient: ParentClient? = null
        var otherTouched = false
        var ownsOtherKey = false
        var parentStateBefore: Map<String, *>? = null
        val previousSockets = HttpsURLConnection.getDefaultSSLSocketFactory()
        var socketsInstalled = false
        try {
            check(BuildConfig.DEBUG && context.packageName == "com.appcontrol.parent.acceptance")
            check((context.applicationInfo.flags and ApplicationInfo.FLAG_DEBUGGABLE) != 0)
            check(Build.FINGERPRINT.startsWith("google/sdk_gphone64_x86_64/emu64xa:"))
            check(AcceptanceHttps.isConfigured())
            val origin = BuildConfig.SERVER_ORIGIN
            val parsed = URL(origin)
            check(parsed.protocol == "https" && parsed.host in setOf("localhost", "10.0.2.2") &&
                parsed.port in 1..65535 && parsed.userInfo == null && parsed.query == null && parsed.ref == null && parsed.path.isEmpty())
            parentStateBefore = context.getSharedPreferences("parent-session", Context.MODE_PRIVATE).all.toMap()
            check(context.getSharedPreferences(OTHER_PREFS, Context.MODE_PRIVATE).all.isEmpty())
            val keys = KeyStore.getInstance("AndroidKeyStore").apply { load(null) }
            check(!keys.containsAlias("app-control.$OTHER_PREFS"))
            ownsOtherKey = true
            otherTouched = true
            val sockets = AcceptanceHttps.socketFactory()
            HttpsURLConnection.setDefaultSSLSocketFactory(sockets)
            socketsInstalled = true
            val open: (URL) -> HttpsURLConnection = { url ->
                (url.openConnection() as HttpsURLConnection).apply { sslSocketFactory = sockets }
            }
            fun keysOf(value: JSONObject) = value.keys().asSequence().toSet()
            fun feed(client: ParentClient): JSONObject {
                val response = client.parentCallEvents()
                check(keysOf(response) == setOf("serverTime", "events"))
                Instant.parse(response.getString("serverTime"))
                check(response.getJSONArray("events").length() <= 100)
                return response
            }
            fun matching(response: JSONObject, deviceId: String, callId: String): List<JSONObject> {
                val events = response.getJSONArray("events")
                return (0 until events.length()).map(events::getJSONObject).filter {
                    it.getString("deviceId") == deviceId && it.getString("callId") == callId
                }
            }
            fun requireEvent(response: JSONObject, deviceId: String, callId: String): JSONObject {
                val found = matching(response, deviceId, callId)
                check(found.size == 1)
                val event = found.single()
                check(keysOf(event) == setOf("eventId", "deviceId", "callId", "snapshotSequence", "observedAt", "expiresAt"))
                val sequence = event.get("snapshotSequence")
                check((sequence is Int || sequence is Long) && (sequence as Number).toLong() > 0)
                check(event.getString("eventId") == "$deviceId:$callId")
                val observed = Instant.parse(event.getString("observedAt"))
                val expires = Instant.parse(event.getString("expiresAt"))
                val serverTime = Instant.parse(response.getString("serverTime"))
                check(!observed.isAfter(serverTime) && expires.isAfter(serverTime) &&
                    Duration.between(observed, serverTime) <= Duration.ofSeconds(30) &&
                    !expires.isAfter(observed.plusSeconds(30)))
                return event
            }
            fun signedOut(client: ParentClient): Boolean {
                try { client.parentCallEvents() } catch (_: ParentClient.SignInRequired) { return true }
                return false
            }
            fun rawFeedStatus(bearer: String? = null): Int {
                val connection = open(URL(origin + "/v1/parent/call-events"))
                try {
                    connection.requestMethod = "GET"
                    connection.instanceFollowRedirects = false
                    connection.connectTimeout = 8_000; connection.readTimeout = 8_000
                    connection.useCaches = false
                    if (bearer != null) connection.setRequestProperty("Authorization", "Bearer $bearer")
                    return connection.responseCode
                } finally { connection.disconnect() }
            }

            phase = "unsigned-feed-refused"
            val foreignVault = SessionVault(context, OTHER_PREFS)
            otherVault = foreignVault
            val foreignClient = ParentClient(foreignVault, open, 8_000) { Instant.now() }
            other = foreignClient
            check(signedOut(foreignClient) && rawFeedStatus() == 401)

            phase = "synthetic-device-setup"
            val synthetic = ParentUiDeviceFixture(context)
            fixture = synthetic
            val deviceId = synthetic.prepare()
            val owner = synthetic.client
            ownerClient = owner
            val ownerRouting = "fixture-parent-routing-" + UUID.randomUUID()
            val foreignRouting = "fixture-foreign-routing-" + UUID.randomUUID()
            phase = "owner-registration"
            owner.registerParentWakeup(ownerRouting)
            phase = "foreign-session-create"
            try { foreignClient.register(origin, OTHER_EMAIL, PASSWORD) }
            catch (failure: ParentClient.RequestFailure) { if (failure.status != 409) throw failure }
            foreignClient.login(origin, OTHER_EMAIL, PASSWORD)
            foreignClient.registerParentWakeup(foreignRouting)

            phase = "ringing-feed"
            val firstCall = UUID.randomUUID().toString()
            synthetic.reportRinging(firstCall)
            val firstFeed = feed(owner)
            val firstEvent = requireEvent(firstFeed, deviceId, firstCall)
            check(!firstFeed.toString().contains(ownerRouting))
            val current = owner.remoteCalls(deviceId)
            check(current.getLong("snapshotSequence") == firstEvent.getLong("snapshotSequence"))
            val calls = current.getJSONArray("calls")
            check(calls.length() == 1 && calls.getJSONObject(0).getString("callId") == firstCall &&
                calls.getJSONObject(0).getString("state") == "RINGING")

            phase = "foreign-owner-refused"
            val foreignFeed = feed(foreignClient)
            check(matching(foreignFeed, deviceId, firstCall).isEmpty() && !foreignFeed.toString().contains(deviceId) &&
                !foreignFeed.toString().contains(ownerRouting) && !foreignFeed.toString().contains(foreignRouting))
            var crossOwnerDenied = false
            try { foreignClient.remoteCalls(deviceId) }
            catch (failure: ParentClient.RequestFailure) { crossOwnerDenied = failure.status == 404 }
            check(crossOwnerDenied)

            phase = "registration-removal-retains-authenticated-feed"
            owner.removeParentWakeup()
            val afterDelete = requireEvent(feed(owner), deviceId, firstCall)
            check(afterDelete.getString("eventId") == firstEvent.getString("eventId"))
            owner.registerParentWakeup("fixture-parent-rotated-" + UUID.randomUUID())

            phase = "ended-call-excluded"
            synthetic.reportRinging(null)
            check(matching(feed(owner), deviceId, firstCall).isEmpty())
            check(owner.remoteCalls(deviceId).getJSONArray("calls").length() == 0)

            phase = "revoked-device-excluded"
            val secondCall = UUID.randomUUID().toString()
            synthetic.reportRinging(secondCall)
            requireEvent(feed(owner), deviceId, secondCall)
            owner.revoke(deviceId)
            val revokedFeed = feed(owner)
            check(matching(revokedFeed, deviceId, firstCall).isEmpty() && matching(revokedFeed, deviceId, secondCall).isEmpty() &&
                !revokedFeed.toString().contains(deviceId))
            var revokedReadDenied = false
            try { owner.remoteCalls(deviceId) }
            catch (failure: ParentClient.RequestFailure) { revokedReadDenied = failure.status == 404 }
            check(revokedReadDenied)

            phase = "signed-out-session-refused"
            owner.removeParentWakeup()
            foreignClient.removeParentWakeup()
            val oldAccess = foreignVault.load(origin).getString("accessToken")
            check(foreignClient.logout() && !foreignClient.hasSession())
            check(signedOut(foreignClient) && rawFeedStatus(oldAccess) == 401)
            proofPassed = true
        } catch (_: Exception) {
            result.putString("server_https_parent_push_api_phase", phase)
        } finally {
            var cleanupFailed = false
            fun clean(action: () -> Unit) { try { action() } catch (_: Exception) { cleanupFailed = true } }
            clean { fixture?.close() }
            clean {
                other?.let { if (it.hasSession()) check(it.logout()) }
                if (otherTouched) {
                    otherVault?.clear()
                    check(context.deleteSharedPreferences(OTHER_PREFS))
                }
            }
            clean { if (ownsOtherKey) KeyStore.getInstance("AndroidKeyStore").apply { load(null); deleteEntry("app-control.$OTHER_PREFS") } }
            clean {
                parentStateBefore?.let { check(context.getSharedPreferences("parent-session", Context.MODE_PRIVATE).all == it) }
                ownerClient?.let { check(!it.hasSession()) }
            }
            clean { if (socketsInstalled) HttpsURLConnection.setDefaultSSLSocketFactory(previousSockets) }
            result.putString("server_https_parent_push_api_cleanup", if (cleanupFailed) "failed" else "passed")
            result.putString("server_https_parent_push_api_probe", if (proofPassed && !cleanupFailed) "passed" else "failed")
            if (cleanupFailed) result.putString("server_https_parent_push_api_phase", "cleanup")
        }
        return result
    }
}
