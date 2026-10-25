package com.appcontrol.parent

import android.content.Context
import android.os.Bundle
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
    private class Connection(url: URL, private val status: Int, private val body: String, private val fail: Boolean, private val waitForDisconnect: Boolean = false) : HttpsURLConnection(url) {
        val sent = ByteArrayOutputStream()
        @Volatile var closed = false
        val waiting = java.util.concurrent.CountDownLatch(1)
        override fun connect() { }
        override fun disconnect() { closed = true }
        override fun usingProxy() = false
        override fun getCipherSuite() = "fixture"
        override fun getLocalCertificates(): Array<Certificate>? = null
        override fun getServerCertificates(): Array<Certificate> = emptyArray()
        override fun getOutputStream() = sent
        override fun getInputStream() = ByteArrayInputStream(body.toByteArray(Charsets.UTF_8))
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
            var ambiguous = false
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
                    else -> "{}"
                }
                Connection(url, responseStatus, response, ambiguous && path == "/v1/auth/refresh").also { requests.add(it) }
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
            result.putString("parent_client_probe", "passed")
            result.putString("stream", "Parent client origin/access/rotation, redirect refusal, uncertain-refresh handling, logout, total deadlines and interrupted-request disconnect passed\n")
        } catch (_: Exception) {
            result.putString("parent_client_probe", "failed"); result.putString("observation", stage)
        } finally {
            if (touched) {
                try { vault.clear(); KeyStore.getInstance("AndroidKeyStore").apply { load(null); deleteEntry("app-control.$name") } }
                catch (_: Exception) { result.putString("parent_client_probe", "failed"); result.putString("observation", "cleanup") }
            }
        }
        return result
    }
}
