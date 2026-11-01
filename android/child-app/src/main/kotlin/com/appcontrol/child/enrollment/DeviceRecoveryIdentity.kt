package com.appcontrol.child.enrollment

import android.annotation.SuppressLint
import android.content.Context
import android.provider.Settings
import android.util.Base64
import java.security.MessageDigest

/** A same-installation locator. It is public metadata, never an authentication credential. */
class DeviceRecoveryIdentity(private val context: Context) {
    @SuppressLint("HardwareIds") // Required locator by protocol; never accepted as authorization.
    fun handle(): String {
        val androidId = checkNotNull(
            Settings.Secure.getString(context.contentResolver, Settings.Secure.ANDROID_ID),
        ) { "Android device identifier is unavailable" }
        require(androidId.isNotBlank()) { "Android device identifier is unavailable" }
        return derive(context.packageName, androidId)
    }

    companion object {
        private const val DOMAIN = "app-control-recovery-handle-v1\n"

        internal fun derive(packageName: String, androidId: String): String {
            require(packageName.isNotBlank() && androidId.isNotBlank())
            val digest = MessageDigest.getInstance("SHA-256").apply {
                update(DOMAIN.toByteArray(Charsets.UTF_8))
                update(packageName.toByteArray(Charsets.UTF_8))
                update('\n'.code.toByte())
                update(androidId.toByteArray(Charsets.UTF_8))
            }.digest()
            return Base64.encodeToString(digest, Base64.URL_SAFE or Base64.NO_WRAP or Base64.NO_PADDING)
                .also { require(it.length == 43) }
        }
    }
}
