package com.appcontrol.parent

import android.content.Context
import android.security.keystore.KeyGenParameterSpec
import android.security.keystore.KeyProperties
import android.util.Base64
import java.security.KeyStore
import javax.crypto.Cipher
import javax.crypto.KeyGenerator
import javax.crypto.SecretKey
import javax.crypto.spec.GCMParameterSpec
import org.json.JSONObject

/** Tokens are authenticated ciphertext; the AES key never leaves Android Keystore. */
class SessionVault(context: Context, name: String = "parent-session") {
    private val state = context.applicationContext.getSharedPreferences(name, Context.MODE_PRIVATE)
    private val alias = "app-control.$name"
    init { require(name.matches(Regex("[a-z-]{1,64}"))) }
    private fun key(): SecretKey {
        val store = KeyStore.getInstance("AndroidKeyStore").apply { load(null) }
        (store.getKey(alias, null) as? SecretKey)?.let { return it }
        return KeyGenerator.getInstance(KeyProperties.KEY_ALGORITHM_AES, "AndroidKeyStore").apply {
            init(KeyGenParameterSpec.Builder(alias, KeyProperties.PURPOSE_ENCRYPT or KeyProperties.PURPOSE_DECRYPT)
                .setBlockModes(KeyProperties.BLOCK_MODE_GCM).setEncryptionPaddings(KeyProperties.ENCRYPTION_PADDING_NONE)
                .setRandomizedEncryptionRequired(true).build())
        }.generateKey()
    }
    fun origin(): String? = state.getString("origin", null)
    fun save(origin: String, value: JSONObject) {
        val cipher = Cipher.getInstance("AES/GCM/NoPadding")
        cipher.init(Cipher.ENCRYPT_MODE, key()); cipher.updateAAD(origin.toByteArray(Charsets.UTF_8))
        val ciphertext = cipher.doFinal(value.toString().toByteArray(Charsets.UTF_8))
        check(state.edit().clear().putString("origin", origin)
            .putString("iv", Base64.encodeToString(cipher.iv, Base64.NO_WRAP))
            .putString("ciphertext", Base64.encodeToString(ciphertext, Base64.NO_WRAP)).commit()) { "Could not save session" }
    }
    fun load(origin: String): JSONObject {
        check(origin == origin()) { "Session origin changed" }
        val iv = Base64.decode(checkNotNull(state.getString("iv", null)), Base64.NO_WRAP)
        require(iv.size == 12)
        val ciphertext = Base64.decode(checkNotNull(state.getString("ciphertext", null)), Base64.NO_WRAP)
        require(ciphertext.size in 16..16384)
        val cipher = Cipher.getInstance("AES/GCM/NoPadding")
        cipher.init(Cipher.DECRYPT_MODE, key(), GCMParameterSpec(128, iv))
        cipher.updateAAD(origin.toByteArray(Charsets.UTF_8))
        return JSONObject(String(cipher.doFinal(ciphertext), Charsets.UTF_8))
    }
    fun clear() { check(state.edit().clear().commit()) { "Could not clear session" } }
}
