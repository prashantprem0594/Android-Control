package com.appcontrol.parent

import android.content.Context
import android.os.Bundle
import com.appcontrol.transport.AcceptanceHttps
import java.io.ByteArrayInputStream
import java.io.ByteArrayOutputStream
import java.io.IOException
import java.net.URL
import java.security.KeyStore
import java.security.cert.Certificate
import java.time.Instant
import javax.net.ssl.HttpsURLConnection
import org.json.JSONObject

/** Controlled HTTPS connection doubles verify client behavior, not platform TLS. */
object ParentClientProbe {
    private class Connection(url: URL, private val status: Int, private val body: String, private val fail: Boolean, private val waitForDisconnect: Boolean = false, private val failConnect: Boolean = false) : HttpsURLConnection(url) {
        val sent = ByteArrayOutputStream()
        @Volatile var closed = false
        val waiting = java.util.concurrent.CountDownLatch(1)
        override fun connect() { if (failConnect) throw java.net.ConnectException("Fixture server offline") }
        override fun disconnect() { closed = true }
        override fun usingProxy() = false
        override fun getCipherSuite() = "fixture"
        override fun getLocalCertificates(): Array<Certificate>? = null
        override fun getServerCertificates(): Array<Certificate> = emptyArray()
        override fun getOutputStream() = sent
        override fun getInputStream() = ByteArrayInputStream(body.toByteArray(Charsets.UTF_8))
        override fun getErrorStream() = ByteArrayInputStream(body.toByteArray(Charsets.UTF_8))
        override fun getResponseCode(): Int {
            if (waitForDisconnect) {
                waiting.countDown()
                val until = System.nanoTime() + java.util.concurrent.TimeUnit.SECONDS.toNanos(3)
                while (!closed && System.nanoTime() < until) java.util.concurrent.locks.LockSupport.parkNanos(5_000_000)
                throw IOException("Fixture stalled response")
            }
            if (fail) throw IOException("Fixture ambiguous response"); return status
        }
    }
    @JvmStatic fun run(context: Context): Bundle {
        val result = Bundle()
        val name = "fixture-parent-client"
        val state = context.getSharedPreferences(name, Context.MODE_PRIVATE)
        val vault = SessionVault(context, name)
        var touched = false
        var stage = "fixture_guard"
        try {
            check(state.all.isEmpty()); touched = true
            var time = Instant.parse("2030-01-01T00:00:00Z")
            val base = BuildConfig.SERVER_ORIGIN.ifEmpty { "https://client.example.org" }
            var refreshes = 0
            var responseStatus = 200
            var errorBody: String? = null
            var ambiguous = false
            var serverOffline = false
            val requests = mutableListOf<Connection>()
            fun tokens(suffix: String) = JSONObject().put("accessToken", "fixture-access-credential-$suffix")
                .put("refreshToken", "fixture-refresh-credential-$suffix")
                .put("accessExpiresAt", time.plusSeconds(60).toString()).put("refreshExpiresAt", time.plusSeconds(3600).toString())
            val factory: (URL) -> HttpsURLConnection = { url ->
                val path = url.path
                val response = when (path) {
                    "/v1/auth/login" -> JSONObject().put("parent", JSONObject().put("email", "fixture@example.org")).put("tokens", tokens("first")).toString()
                    "/v1/auth/refresh" -> { refreshes++; tokens("rotated").toString() }
                    "/v1/devices" -> "{\"devices\":[]}"
                    "/v1/device-recovery/requests" -> "{\"requests\":[{\"requestId\":\"recovery_request_001\",\"deviceId\":\"dev_AAAAAAAAAAAAAAAAAAAAAA\",\"displayName\":\"Fixture child\",\"expiresAt\":\"2030-01-01T00:05:00Z\"}]}"
                    "/v1/parent/call-events" -> JSONObject().put("serverTime", time.toString()).put("events", org.json.JSONArray()).toString()
                    else -> "{}"
                }
                Connection(url, responseStatus, errorBody ?: response, ambiguous && path == "/v1/auth/refresh", failConnect = serverOffline).also { requests.add(it) }
            }
            val client = ParentClient(vault, factory) { time }
            stage = "invalid_origin_rejected"
            var invalidOrigin = false
            try { client.login("http://client.example.org", "fixture@example.org", "fixture-password-value") }
            catch (_: IllegalArgumentException) { invalidOrigin = true }
            check(invalidOrigin && requests.isEmpty() && !client.hasSession())
            stage = "login_and_access"
            client.login(base, "fixture@example.org", "fixture-password-value")
            client.devices()
            check(requests.last().getRequestProperty("Authorization") == "Bearer fixture-access-credential-first")
            check(requests.all { !it.instanceFollowRedirects && it.closed && it.url.host == URL(base).host })
            stage = "device_recovery_parent_contract"
            val recovery = client.listRecoveryRequests().getJSONArray("requests").getJSONObject(0)
            check(recovery.getString("deviceId") == "dev_AAAAAAAAAAAAAAAAAAAAAA")
            val recoveryList = requests.last()
            check(recoveryList.url.path == "/v1/device-recovery/requests" && recoveryList.url.query == null &&
                recoveryList.requestMethod == "GET" && !recoveryList.doOutput && recoveryList.sent.size() == 0 &&
                recoveryList.getRequestProperty("Authorization") == "Bearer fixture-access-credential-first")
            responseStatus = 204
            client.approveRecoveryRequest("recovery_request_001")
            val approval = requests.last()
            check(approval.url.path == "/v1/device-recovery/requests/recovery_request_001/approve" &&
                approval.requestMethod == "POST" && !approval.doOutput && approval.sent.size() == 0 && approval.closed &&
                approval.getRequestProperty("Authorization") == "Bearer fixture-access-credential-first")
            client.rejectRecoveryRequest("recovery_request_002")
            val rejection = requests.last()
            check(rejection.url.path == "/v1/device-recovery/requests/recovery_request_002/reject" &&
                rejection.requestMethod == "POST" && !rejection.doOutput && rejection.sent.size() == 0 && rejection.closed &&
                rejection.getRequestProperty("Authorization") == "Bearer fixture-access-credential-first")
            val beforeInvalidRecoveryId = requests.size
            for (invalidId in listOf("", "../requests", "contains/slash", "short", "x".repeat(129))) {
                var invalidApproveRejected = false
                try { client.approveRecoveryRequest(invalidId) } catch (_: IllegalArgumentException) { invalidApproveRejected = true }
                var invalidRejectRejected = false
                try { client.rejectRecoveryRequest(invalidId) } catch (_: IllegalArgumentException) { invalidRejectRejected = true }
                check(invalidApproveRejected && invalidRejectRejected && requests.size == beforeInvalidRecoveryId)
            }
            responseStatus = 200
            stage = "parent_push_feed_contract"
            val feed = client.parentCallEvents()
            check(feed.getString("serverTime") == time.toString() && feed.getJSONArray("events").length() == 0)
            val feedRequest = requests.last()
            check(feedRequest.url.path == "/v1/parent/call-events" && feedRequest.url.query == null &&
                feedRequest.requestMethod == "GET" && !feedRequest.doOutput && feedRequest.sent.size() == 0 &&
                feedRequest.getRequestProperty("Authorization") == "Bearer fixture-access-credential-first" && feedRequest.closed)
            stage = "parent_push_registration_contract"
            responseStatus = 204
            val routingToken = "fixture-parent-routing-token"
            client.registerParentWakeup(routingToken)
            val registration = requests.last()
            val registrationBody = JSONObject(registration.sent.toString("UTF-8"))
            check(registration.url.path == "/v1/parent/wakeup-registration" && registration.url.query == null &&
                registration.requestMethod == "PUT" && registration.doOutput && registration.closed &&
                registration.getRequestProperty("Authorization") == "Bearer fixture-access-credential-first" &&
                registration.getRequestProperty("Content-Type") == "application/json" &&
                registrationBody.keys().asSequence().toSet() == setOf("registrationToken") &&
                registrationBody.getString("registrationToken") == routingToken)
            client.removeParentWakeup()
            val removal = requests.last()
            check(removal.url.path == "/v1/parent/wakeup-registration" && removal.url.query == null &&
                removal.requestMethod == "DELETE" && !removal.doOutput && removal.sent.size() == 0 && removal.closed &&
                removal.getRequestProperty("Authorization") == "Bearer fixture-access-credential-first")
            stage = "parent_push_invalid_token_before_connection"
            val beforeInvalidTokens = requests.size
            for (invalidToken in listOf("", "contains space", "control\nvalue", "control\u0000value", "non-ascii-\u00e9", "x".repeat(4097))) {
                var rejectedToken = false
                try { client.registerParentWakeup(invalidToken) }
                catch (_: IllegalArgumentException) { rejectedToken = true }
                check(rejectedToken && requests.size == beforeInvalidTokens && client.hasSession())
            }
            stage = "parent_push_unavailable_keeps_manual_session"
            val sessionBeforeUnavailableFeed = state.all.toMap()
            responseStatus = 404
            var unavailableFeed = false
            try { client.parentCallEvents() }
            catch (failure: ParentClient.RequestFailure) { unavailableFeed = failure.status == 404 }
            check(unavailableFeed && client.hasSession() && state.all == sessionBeforeUnavailableFeed && requests.last().closed)
            responseStatus = 200
            client.devices()
            check(requests.last().url.path == "/v1/devices" &&
                requests.last().getRequestProperty("Authorization") == "Bearer fixture-access-credential-first")
            stage = "parent_push_unauthorized_clears_session"
            responseStatus = 401
            var rejectedFeedSession = false
            try { client.parentCallEvents() } catch (_: ParentClient.SignInRequired) { rejectedFeedSession = true }
            check(rejectedFeedSession && !client.hasSession() && state.all.isEmpty() && requests.last().closed)
            // Restore the original fixture session/status; rotation and error tests below keep their original preconditions.
            responseStatus = 200
            client.login(base, "fixture@example.org", "fixture-password-value")
            check(client.hasSession() && refreshes == 0 && errorBody == null && !ambiguous)
            stage = "rotation_once"
            time = time.plusSeconds(45)
            val failures = java.util.Collections.synchronizedList(mutableListOf<Throwable>())
            val parallel = (0 until 2).map {
                Thread {
                    try { ParentClient(SessionVault(context, name), factory) { time }.devices() }
                    catch (failure: Throwable) { failures.add(failure) }
                }
            }
            parallel.forEach { it.start() }; parallel.forEach { it.join(10000) }
            check(parallel.none { it.isAlive } && failures.isEmpty())
            client.devices()
            check(refreshes == 1 && requests.last().getRequestProperty("Authorization") == "Bearer fixture-access-credential-rotated")
            check(JSONObject(requests.first { it.url.path == "/v1/auth/refresh" }.sent.toString("UTF-8")).getString("refreshToken") == "fixture-refresh-credential-first")
            stage = "redirect_refusal"
            responseStatus = 302
            var refused = false
            try { client.devices() } catch (failure: ParentClient.RequestFailure) { refused = failure.status == 302 }
            check(refused && !requests.last().instanceFollowRedirects)
            responseStatus = 200
            stage = "definitive_call_rejection"
            for ((body, expected) in listOf(
                "{\"error\":\"call_not_current\"}" to true,
                "{\"error\":\"call_state_conflict\"}" to true,
                " \n{ \"error\" : \"call_state_conflict\" }\r\n" to true,
                "{\"error\":\"remote_calls_disabled\"}" to false,
                "{\"error\":\"idempotency_conflict\"}" to false,
                "{\"error\":\"unknown\"}" to false,
                "{\"error\":[\"call_not_current\"]}" to false,
                "{\"error\":\"call_not_current\",\"extra\":true}" to false,
                "{\"error\":\"unknown\",\"error\":\"call_not_current\"}" to false,
                "{\"error\":\"call_not_current\"}trailing" to false,
                "not-json" to false,
                (" ".repeat(4097) + "{\"error\":\"call_not_current\"}") to false
            )) {
                responseStatus = 409; errorBody = body
                val deviceId = "dev_AAAAAAAAAAAAAAAAAAAAAA"
                val callId = java.util.UUID.randomUUID().toString()
                val key = "fixture-request-key-1234"
                val record = JSONObject().put("key", key).put("callId", callId).put("action", "ANSWER_CALL").toString()
                check(state.edit().putString(deviceId, record).commit())
                var rejected = false
                try { client.callAction(deviceId, callId, "ANSWER_CALL", 20, key) }
                catch (failure: ParentClient.RequestFailure) {
                    rejected = true
                    check(failure.status == 409 && failure.callRequestNotQueued == expected)
                    if (expected) {
                        var mismatchRefused = false
                        try { PendingCallRequests.clearRejected(state, deviceId, "another-request-key", callId, "ANSWER_CALL", failure) }
                        catch (_: IllegalStateException) { mismatchRefused = true }
                        check(mismatchRefused && state.getString(deviceId, null) == record)
                    }
                    check(PendingCallRequests.clearRejected(state, deviceId, key, callId, "ANSWER_CALL", failure) == expected)
                    check(if (expected) !state.contains(deviceId) else state.getString(deviceId, null) == record)
                }
                check(rejected && requests.last().closed && client.hasSession())
                check(state.edit().remove(deviceId).commit())
            }
            responseStatus = 503; errorBody = "{\"error\":\"call_not_current\"}"
            try { client.devices(); error("Expected unavailable response") }
            catch (failure: ParentClient.RequestFailure) { check(!failure.callRequestNotQueued) }
            responseStatus = 200; errorBody = null
            stage = "offline_refresh_keeps_session"
            time = time.plusSeconds(45); serverOffline = true
            var offline = false
            try { client.devices() } catch (_: ParentClient.ServerUnavailable) { offline = true }
            check(offline && client.hasSession() && requests.last().closed)
            serverOffline = false
            client.devices()
            check(client.hasSession())
            stage = "ambiguous_refresh"
            time = time.plusSeconds(45); ambiguous = true
            val before = requests.size
            var ended = false
            try { client.devices() } catch (_: ParentClient.SignInRequired) { ended = true }
            check(ended && !client.hasSession() && requests.size == before + 1)
            ended = false
            try { client.devices() } catch (_: ParentClient.SignInRequired) { ended = true }
            check(ended && requests.size == before + 1)
            stage = "unauthorized_session_cleared"
            ambiguous = false; client.login(base, "fixture@example.org", "fixture-password-value")
            responseStatus = 401; ended = false
            try { client.devices() } catch (_: ParentClient.SignInRequired) { ended = true }
            check(ended && !client.hasSession())
            stage = "logout_after_transport_failure"
            responseStatus = 200; client.login(base, "fixture@example.org", "fixture-password-value")
            responseStatus = 503
            check(!client.logout() && !client.hasSession())
            stage = "total_request_deadline"
            val timedConnection = java.util.concurrent.atomic.AtomicReference<Connection>()
            val timedClient = ParentClient(vault, { url -> Connection(url, 200, "{}", false, true).also { timedConnection.set(it) } }, 25) { time }
            val started = System.nanoTime(); var timedOut = false
            try { timedClient.login(base, "fixture@example.org", "fixture-password-value") } catch (_: Exception) { timedOut = true }
            check(timedOut && java.util.concurrent.TimeUnit.NANOSECONDS.toMillis(System.nanoTime() - started) < 2000 && timedConnection.get().closed && !timedClient.hasSession())
            stage = "interrupted_request_disconnect"
            val interruptedConnection = java.util.concurrent.atomic.AtomicReference<Connection>()
            val ready = java.util.concurrent.CountDownLatch(1)
            val interruptedFailure = java.util.concurrent.atomic.AtomicBoolean()
            val interruptedClient = ParentClient(vault, { url -> Connection(url, 200, "{}", false, true).also { interruptedConnection.set(it); ready.countDown() } }) { time }
            val thread = Thread {
                try { interruptedClient.login(base, "fixture@example.org", "fixture-password-value") }
                catch (_: Exception) { interruptedFailure.set(true) }
            }.apply { isDaemon = true }
            thread.start(); check(ready.await(2, java.util.concurrent.TimeUnit.SECONDS))
            check(interruptedConnection.get().waiting.await(2, java.util.concurrent.TimeUnit.SECONDS))
            thread.interrupt(); thread.join(2000)
            check(!thread.isAlive && interruptedFailure.get() && interruptedConnection.get().closed && !interruptedClient.hasSession())
            if (BuildConfig.SERVER_ORIGIN.isNotEmpty() && AcceptanceHttps.isConfigured()) {
                stage = "live_server_https"
                val live = LiveServerHttpsProbe.run(context)
                result.putString("server_https_parent_probe", live.getString("server_https_parent_probe", "failed"))
                live.getString("server_https_parent_phase")?.let {
                    result.putString("server_https_parent_phase", it)
                }
                check(live.getString("server_https_parent_probe") == "passed")
            }
            result.putString("parent_client_probe", "passed")
            result.putString("parent_push_client_probe", "passed")
            result.putString("stream", "Parent client origin/access/rotation, push feed/registration/unregister, invalid-token refusal, unavailable-feed fallback, unauthorized-feed clearing, redirect refusal, uncertain-refresh handling, logout, total deadlines and interrupted-request disconnect passed\n")
        } catch (_: Exception) {
            result.putString("parent_client_probe", "failed"); result.putString("parent_push_client_probe", "failed"); result.putString("observation", stage)
        } finally {
            if (touched) {
                try { vault.clear(); KeyStore.getInstance("AndroidKeyStore").apply { load(null); deleteEntry("app-control.$name") } }
                catch (_: Exception) { result.putString("parent_client_probe", "failed"); result.putString("parent_push_client_probe", "failed"); result.putString("observation", "cleanup") }
            }
        }
        return result
    }
}
