package com.appcontrol.parent

import android.content.Context
import android.os.Bundle
import com.appcontrol.transport.AcceptanceHttps
import java.net.URL
import java.time.Instant
import javax.net.ssl.HttpsURLConnection
import org.json.JSONArray

/** Uses ParentClient against a disposable local server with an isolated session vault. */
object LiveServerHttpsProbe {
    private const val ownerEmail = "acceptance-owner@example.test"
    private const val otherEmail = "acceptance-other@example.test"
    private const val password = "acceptance-fixture-password-not-secret"

    @JvmStatic
    fun run(context: Context): Bundle {
        val result = Bundle()
        val origin = BuildConfig.SERVER_ORIGIN
        val ownerName = "acceptance-server-https-owner"
        val otherName = "acceptance-server-https-other"
        val ownerVault = SessionVault(context, ownerName)
        val otherVault = SessionVault(context, otherName)
        var ownerTouched = false
        var otherTouched = false
        var phase = "guard"
        try {
            check(AcceptanceHttps.isConfigured())
            check(URL(origin).host in setOf("localhost", "10.0.2.2") && URL(origin).protocol == "https")
            check(context.getSharedPreferences(ownerName, Context.MODE_PRIVATE).all.isEmpty())
            check(context.getSharedPreferences(otherName, Context.MODE_PRIVATE).all.isEmpty())
            val sockets = AcceptanceHttps.socketFactory()
            val open: (URL) -> HttpsURLConnection = { url ->
                (url.openConnection() as HttpsURLConnection).apply { sslSocketFactory = sockets }
            }
            var clientTime = Instant.now()
            val owner = ParentClient(ownerVault, open) { clientTime }
            phase = "owner-session-create"
            owner.login(origin, ownerEmail, password)
            ownerTouched = true
            check(owner.hasSession())
            val firstTokens = ownerVault.load(origin)
            phase = "owner-device-list"
            val devices = owner.devices().getJSONArray("devices")
            check(devices.length() > 0)
            val deviceId = devices.getJSONObject(0).getString("id")
            check(deviceId.matches(Regex("dev_[A-Za-z0-9_-]{22}")))
            phase = "owner-session-refresh"
            clientTime = Instant.parse(firstTokens.getString("accessExpiresAt")).minusSeconds(5)
            val refreshed = owner.devices().getJSONArray("devices")
            val nextTokens = ownerVault.load(origin)
            check(refreshed.length() > 0)
            check(firstTokens.getString("accessToken") != nextTokens.getString("accessToken"))
            clientTime = Instant.now()
            phase = "owner-session-logout"
            check(owner.logout() && !owner.hasSession())
            var signedOut = false
            try {
                owner.devices()
            } catch (_: ParentClient.SignInRequired) {
                signedOut = true
            }
            check(signedOut)
            phase = "other-session-create"
            val other = ParentClient(otherVault, open) { Instant.now() }
            try {
                other.register(origin, otherEmail, password)
            } catch (failure: ParentClient.RequestFailure) {
                if (failure.status != 409) throw failure
            }
            other.login(origin, otherEmail, password)
            otherTouched = true
            phase = "other-account-authorization"
            val otherDevices: JSONArray = other.devices().getJSONArray("devices")
            check(otherDevices.length() == 0)
            var denied = false
            try {
                other.policy(deviceId)
            } catch (failure: ParentClient.RequestFailure) {
                denied = failure.status == 404
            }
            check(denied)
            phase = "other-session-logout"
            check(other.logout() && !other.hasSession())
            result.putString("server_https_parent_probe", "passed")
        } catch (_: Exception) {
            result.putString("server_https_parent_probe", "failed")
            result.putString("server_https_parent_phase", phase)
        } finally {
            try {
                if (ownerTouched) ownerVault.clear()
                KeyStoreCleanup.delete(context, ownerName)
                if (otherTouched) otherVault.clear()
                KeyStoreCleanup.delete(context, otherName)
            } catch (_: Exception) {
                result.putString("server_https_parent_probe", "failed")
                result.putString("server_https_parent_phase", "cleanup")
            }
        }
        return result
    }
}

private object KeyStoreCleanup {
    fun delete(context: Context, name: String) {
        context.deleteSharedPreferences(name)
        java.security.KeyStore.getInstance("AndroidKeyStore").apply {
            load(null)
            deleteEntry("app-control.$name")
        }
    }
}
