package com.appcontrol.child.calls

import android.content.Context
import android.security.keystore.KeyGenParameterSpec
import android.security.keystore.KeyProperties
import android.util.Base64
import com.appcontrol.child.BuildConfig
import com.appcontrol.transport.ServerOrigin
import java.security.KeyStore
import java.security.MessageDigest
import javax.crypto.Cipher
import javax.crypto.KeyGenerator
import javax.crypto.SecretKey
import javax.crypto.spec.GCMParameterSpec
import org.json.JSONArray
import org.json.JSONObject

/** Parent-maintained shortcuts only; never grants permission or places a call. */
class CallContactStore(context: Context) {
    private val app = context.applicationContext
    private val preferences = app.getSharedPreferences("call-contact-cache", Context.MODE_PRIVATE)
    data class Contact(val label: String, val number: String, val isParent: Boolean)
    data class Snapshot(val version: Long, val contacts: List<Contact>)

    private fun binding(): ByteArray {
        val enrollment = app.getSharedPreferences("enrollment", Context.MODE_PRIVATE)
        val origin = ServerOrigin.require(checkNotNull(enrollment.getString("server", null)), BuildConfig.SERVER_ORIGIN)
        val device = checkNotNull(enrollment.getString("deviceId", null))
        require(device.matches(Regex("dev_[A-Za-z0-9_-]{22}")))
        return "call-contacts-v1\n$origin\n$device".toByteArray(Charsets.UTF_8)
    }
    private fun key(create: Boolean): SecretKey {
        val store = KeyStore.getInstance("AndroidKeyStore").apply { load(null) }
        (store.getKey(ALIAS, null) as? SecretKey)?.let { return it }
        check(create) { "Contact cache key unavailable" }
        return KeyGenerator.getInstance(KeyProperties.KEY_ALGORITHM_AES, "AndroidKeyStore").run {
            init(KeyGenParameterSpec.Builder(ALIAS, KeyProperties.PURPOSE_ENCRYPT or KeyProperties.PURPOSE_DECRYPT)
                .setBlockModes(KeyProperties.BLOCK_MODE_GCM).setEncryptionPaddings(KeyProperties.ENCRYPTION_PADDING_NONE)
                .setKeySize(256).build())
            generateKey()
        }
    }
    private fun slot(): String = "sealed-" + MessageDigest.getInstance("SHA-256").digest(binding()).joinToString("") { "%02x".format(it.toInt() and 255) }
    fun load(): Snapshot = synchronized(lock) {
        val raw = preferences.getString(slot(), null) ?: return@synchronized Snapshot(0, emptyList())
        require(raw.length <= 32768)
        val sealed = JSONObject(raw)
        val iv = Base64.decode(sealed.getString("iv"), Base64.NO_WRAP)
        val bytes = Base64.decode(sealed.getString("ciphertext"), Base64.NO_WRAP)
        require(iv.size == 12 && bytes.size in 16..16384)
        val cipher = Cipher.getInstance("AES/GCM/NoPadding")
        cipher.init(Cipher.DECRYPT_MODE, key(false), GCMParameterSpec(128, iv))
        cipher.updateAAD(binding())
        parse(JSONObject(String(cipher.doFinal(bytes), Charsets.UTF_8)))
    }
    fun save(response: JSONObject) = synchronized(lock) {
        val snapshot = parse(response)
        val old = load() // Fail closed on unreadable state; never silently reset the version floor.
        require(snapshot.version >= old.version)
        require(snapshot.version != old.version || snapshot == old)
        val contacts = JSONArray()
        for (c in snapshot.contacts) contacts.put(JSONObject().put("label", c.label).put("number", c.number).put("isParent", c.isParent))
        val bytes = JSONObject().put("version", snapshot.version).put("contacts", contacts).toString().toByteArray(Charsets.UTF_8)
        val cipher = Cipher.getInstance("AES/GCM/NoPadding")
        cipher.init(Cipher.ENCRYPT_MODE, key(true))
        cipher.updateAAD(binding())
        val sealed = JSONObject().put("iv", Base64.encodeToString(cipher.iv, Base64.NO_WRAP))
            .put("ciphertext", Base64.encodeToString(cipher.doFinal(bytes), Base64.NO_WRAP))
        check(preferences.edit().putString(slot(), sealed.toString()).commit()) { "Contact cache unavailable" }
    }
    companion object {
        private const val ALIAS = "appcontrol.callcontacts.v1"
        private val lock = Any()
        fun parse(value: JSONObject): Snapshot {
            require(value.length() == 2 && value.has("version") && value.has("contacts"))
            val rawVersion = value.get("version")
            require(rawVersion is Int || rawVersion is Long)
            val version = value.getLong("version")
            require(version in 0..9007199254740991L)
            val rows = value.getJSONArray("contacts")
            require(rows.length() <= 10 && (version > 0 || rows.length() == 0))
            val contacts = (0 until rows.length()).map {
                val row = rows.getJSONObject(it)
                require(row.length() == 3 && row.get("label") is String && row.get("number") is String && row.get("isParent") is Boolean)
                val label = row.getString("label")
                val number = row.getString("number")
                require(label.isNotEmpty() && label.trim() == label && label.codePointCount(0, label.length) <= 40 && label.toByteArray(Charsets.UTF_8).size <= 160)
                require(label.codePoints().noneMatch { cp -> Character.isISOControl(cp) || Character.getType(cp) in setOf(Character.FORMAT.toInt(), Character.LINE_SEPARATOR.toInt(), Character.PARAGRAPH_SEPARATOR.toInt(), Character.SURROGATE.toInt()) })
                require(number.matches(Regex("\\+[1-9][0-9]{6,14}")))
                Contact(label, number, row.getBoolean("isParent"))
            }
            require(contacts.map { it.number }.distinct().size == contacts.size && contacts.count { it.isParent } <= 1)
            return Snapshot(version, contacts)
        }
    }
}
