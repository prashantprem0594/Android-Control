package com.appcontrol.child.calls

import android.Manifest
import android.app.Activity
import android.app.role.RoleManager
import android.content.Intent
import android.content.pm.PackageManager
import android.net.Uri
import android.os.Bundle
import android.telecom.TelecomManager
import android.telephony.TelephonyManager
import android.text.InputType
import android.widget.Button
import android.widget.EditText
import android.widget.LinearLayout
import android.widget.ScrollView
import android.widget.TextView
import android.widget.Toast

/** ACTION_DIAL never places a call automatically; the user must press Call. */
class DialActivity : Activity() {
    private lateinit var number: EditText
    private lateinit var contacts: LinearLayout
    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        window.addFlags(android.view.WindowManager.LayoutParams.FLAG_SECURE)
        number = EditText(this).apply { hint = "Phone number"; inputType = InputType.TYPE_CLASS_PHONE; isSaveEnabled = false }
        readIntent(intent)
        val content = LinearLayout(this).apply {
            orientation = LinearLayout.VERTICAL; setPadding(32, 48, 32, 48)
            addView(TextView(this@DialActivity).apply { text = "App Control phone"; textSize = 24f })
            addView(number)
            contacts = LinearLayout(this@DialActivity).apply { orientation = LinearLayout.VERTICAL }
            addView(contacts)
            for (row in listOf("123", "456", "789", "*0#")) addView(LinearLayout(this@DialActivity).apply {
                for (digit in row) addView(Button(this@DialActivity).apply {
                    text = digit.toString(); setOnClickListener { number.text.insert(number.selectionStart.coerceAtLeast(0), digit.toString()) }
                }, LinearLayout.LayoutParams(0, LinearLayout.LayoutParams.WRAP_CONTENT, 1f))
            })
            addView(Button(this@DialActivity).apply { text = "Delete"; setOnClickListener {
                val cursor = number.selectionStart.coerceAtLeast(0); if (cursor > 0) number.text.delete(cursor - 1, cursor)
            } })
            addView(Button(this@DialActivity).apply { text = "Call"; setOnClickListener { placeCall() } })
            addView(Button(this@DialActivity).apply { text = "Current call"; setOnClickListener { startActivity(Intent(this@DialActivity, CallActivity::class.java)) } })
            addView(Button(this@DialActivity).apply { text = "Use system phone"; setOnClickListener { systemPhone() } })
        }
        setContentView(ScrollView(this).apply { addView(content) })
    }
    override fun onNewIntent(intent: Intent) { super.onNewIntent(intent); setIntent(intent); readIntent(intent) }
    override fun onResume() { super.onResume(); renderContacts() }
    private fun renderContacts() {
        contacts.removeAllViews()
        val saved = runCatching { CallContactStore(this).load() }.getOrNull()
        if (saved == null) {
            contacts.addView(TextView(this).apply { text = "Saved contact shortcuts are unavailable. You can still dial a number." })
            return
        }
        for (contact in saved.contacts.sortedByDescending { it.isParent }) {
            contacts.addView(Button(this).apply {
                text = if (contact.isParent) "Call Parent - ${contact.label}" else contact.label
                isAllCaps = false
                setOnClickListener {
                    val current = runCatching { CallContactStore(this@DialActivity).load() }.getOrNull()
                    if (current == null || contact !in current.contacts) {
                        renderContacts()
                        Toast.makeText(this@DialActivity, "Contact changed. Choose it again.", Toast.LENGTH_SHORT).show()
                    } else {
                        number.setText(contact.number)
                        number.setSelection(number.length())
                        Toast.makeText(this@DialActivity, "Review the number, then press Call.", Toast.LENGTH_LONG).show()
                    }
                }
            })
        }
    }
    private fun readIntent(intent: Intent) {
        number.setText(intent.data?.takeIf { it.scheme == "tel" }?.schemeSpecificPart?.take(80) ?: "")
    }
    private fun placeCall() {
        val value = number.text.toString().trim()
        if (!value.matches(Regex("[+0-9*#,; ()-]{1,80}"))) {
            Toast.makeText(this, "Enter a phone number.", Toast.LENGTH_SHORT).show(); return
        }
        if (checkSelfPermission(Manifest.permission.CALL_PHONE) != PackageManager.PERMISSION_GRANTED) {
            requestPermissions(arrayOf(Manifest.permission.CALL_PHONE), 4404)
            Toast.makeText(this, "After granting permission, press Call again.", Toast.LENGTH_LONG).show(); return
        }
        // Telecom retains account selection and the system emergency-calling path.
        val result = runCatching { getSystemService(TelecomManager::class.java).placeCall(Uri.fromParts("tel", value, null), Bundle()) }
        if (result.isSuccess) {
            val emergency = runCatching { getSystemService(TelephonyManager::class.java).isEmergencyNumber(value) }.getOrDefault(true)
            if (!emergency && getSystemService(RoleManager::class.java).isRoleHeld(RoleManager.ROLE_DIALER)) {
                startActivity(Intent(this, CallActivity::class.java))
            }
        } else Toast.makeText(this, "Call could not start. Use the system phone if needed.", Toast.LENGTH_LONG).show()
    }
    private fun systemPhone() {
        val target = getSystemService(TelecomManager::class.java).systemDialerPackage
        val intent = Intent(Intent.ACTION_DIAL, Uri.fromParts("tel", number.text.toString().take(80), null))
        if (!target.isNullOrBlank() && target != packageName) intent.setPackage(target)
        runCatching { startActivity(if (intent.`package` == null) Intent.createChooser(intent, "Choose phone app") else intent) }
            .onFailure { Toast.makeText(this, "System phone is unavailable.", Toast.LENGTH_LONG).show() }
    }
}
