package com.appcontrol.child.enrollment

import android.security.keystore.KeyGenParameterSpec
import android.security.keystore.KeyProperties
import android.util.Base64
import java.security.KeyPairGenerator
import java.security.KeyStore
import java.security.Signature
import java.security.spec.ECGenParameterSpec

/** Private key material never leaves AndroidKeyStore. */
class DeviceIdentity {
    private val alias = "app-control-device-v1"
    private fun store() = KeyStore.getInstance("AndroidKeyStore").apply { load(null) }

    @Synchronized
    fun publicKey(): String {
        if (!store().containsAlias(alias)) {
            KeyPairGenerator.getInstance(KeyProperties.KEY_ALGORITHM_EC, "AndroidKeyStore").apply {
                initialize(KeyGenParameterSpec.Builder(alias, KeyProperties.PURPOSE_SIGN)
                    .setAlgorithmParameterSpec(ECGenParameterSpec("secp256r1"))
                    .setDigests(KeyProperties.DIGEST_SHA256)
                    .build())
            }.generateKeyPair()
        }
        return encode(store().getCertificate(alias).publicKey.encoded)
    }

    fun sign(enrollmentId: String, challenge: String): String {
        require(Regex("enr_[A-Za-z0-9_-]{22}").matches(enrollmentId))
        val bytes = Base64.decode(challenge, Base64.URL_SAFE or Base64.NO_WRAP or Base64.NO_PADDING)
        require(bytes.size == 32)
        val key = store().getKey(alias, null) as java.security.PrivateKey
        return encode(Signature.getInstance("SHA256withECDSA").run {
            initSign(key)
            update("app-control-device-enrollment-v1\n$enrollmentId\n".toByteArray(Charsets.UTF_8))
            update(bytes)
            sign()
        })
    }

    fun signAuthentication(deviceId: String, challengeId: String, challenge: String): String {
        require(Regex("dev_[A-Za-z0-9_-]{22}").matches(deviceId))
        require(Regex("dac_[A-Za-z0-9_-]{22}").matches(challengeId))
        val bytes = Base64.decode(challenge, Base64.URL_SAFE or Base64.NO_WRAP or Base64.NO_PADDING)
        require(bytes.size == 32)
        val key = store().getKey(alias, null) as java.security.PrivateKey
        return encode(Signature.getInstance("SHA256withECDSA").run {
            initSign(key)
            update("app-control-device-auth-v1\n$deviceId\n$challengeId\n".toByteArray(Charsets.UTF_8))
            update(bytes)
            sign()
        })
    }

<<<<<<< HEAD
    fun signRecovery(requestId: String, challenge: String): String {
        require(requestId.length in 1..128 && requestId.all { it.isLetterOrDigit() || it == '_' || it == '-' })
        val bytes = Base64.decode(challenge, Base64.URL_SAFE or Base64.NO_WRAP or Base64.NO_PADDING)
        require(bytes.size == 32)
        val key = store().getKey(alias, null) as java.security.PrivateKey
        return encode(Signature.getInstance("SHA256withECDSA").run {
            initSign(key)
            update("app-control-device-recovery-v1\n$requestId\n".toByteArray(Charsets.UTF_8))
            update(bytes)
            sign()
        })
    }

=======
>>>>>>> origin/main
    private fun encode(bytes: ByteArray) = Base64.encodeToString(bytes,
        Base64.URL_SAFE or Base64.NO_WRAP or Base64.NO_PADDING)
}

