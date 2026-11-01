package com.appcontrol.child.calls

import android.Manifest
import android.app.Activity
<<<<<<< HEAD
import android.os.Build
=======
import android.app.role.RoleManager
>>>>>>> origin/main
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
<<<<<<< HEAD
import android.view.Gravity
import com.appcontrol.transport.AppAppearance
=======
>>>>>>> origin/main

/** ACTION_DIAL never places a call automatically; the user must press Call. */
class DialActivity : Activity() {
    private lateinit var number: EditText
    private lateinit var contacts: LinearLayout
    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
<<<<<<< HEAD
        AppAppearance.prepare(this)
        window.addFlags(android.view.WindowManager.LayoutParams.FLAG_SECURE)
        number = EditText(this).apply {
            hint = "Phone number"; inputType = InputType.TYPE_CLASS_PHONE; isSaveEnabled = false
            AppAppearance.styleField(this); textSize = 28f; gravity = Gravity.CENTER
        }
        readIntent(intent)
        val content = AppAppearance.column(this).apply {
            addView(AppAppearance.text(this@DialActivity, "APP CONTROL / PHONE", 12f, true).apply { setTextColor(android.graphics.Color.rgb(49, 91, 219)) })
            addView(AppAppearance.text(this@DialActivity, "Phone", 26f, true))
            addView(AppAppearance.card(this@DialActivity).apply {
            addView(number)
            for (row in listOf("123", "456", "789", "*0#")) addView(LinearLayout(this@DialActivity).apply {
                for (digit in row) addView(Button(this@DialActivity).apply {
                    text = digit.toString(); AppAppearance.styleButton(this, false); textSize = 24f
                    minimumHeight = AppAppearance.dp(this@DialActivity, 56f)
                    contentDescription = if (digit == '*') "Star" else if (digit == '#') "Hash" else digit.toString()
                    setOnClickListener { number.text.insert(number.selectionStart.coerceAtLeast(0), digit.toString()) }
                }, LinearLayout.LayoutParams(0, LinearLayout.LayoutParams.WRAP_CONTENT, 1f).apply {
                    setMargins(AppAppearance.dp(this@DialActivity, 3f), AppAppearance.dp(this@DialActivity, 3f), AppAppearance.dp(this@DialActivity, 3f), AppAppearance.dp(this@DialActivity, 3f))
                })
            })
            addView(Button(this@DialActivity).apply { text = "Delete"; AppAppearance.styleButton(this, false); setOnClickListener {
                val cursor = number.selectionStart.coerceAtLeast(0); if (cursor > 0) number.text.delete(cursor - 1, cursor)
            } })
            })
            addView(AppAppearance.card(this@DialActivity).apply {
                addView(AppAppearance.text(this@DialActivity, "CONTACT SHORTCUTS", 12f, true))
                contacts = LinearLayout(this@DialActivity).apply { orientation = LinearLayout.VERTICAL }
                addView(contacts)
            })
            addView(Button(this@DialActivity).apply { text = "Current call"; AppAppearance.styleButton(this, false); setOnClickListener { startActivity(Intent(this@DialActivity, CallActivity::class.java)) } })
            addView(Button(this@DialActivity).apply { text = "Use system phone"; AppAppearance.styleButton(this, false); setOnClickListener { systemPhone() } })
        }
        val screen = LinearLayout(this).apply {
            orientation = LinearLayout.VERTICAL
            setBackgroundColor(AppAppearance.BACKGROUND)
            addView(ScrollView(this@DialActivity).apply { isFillViewport = true; addView(content) },
                LinearLayout.LayoutParams(LinearLayout.LayoutParams.MATCH_PARENT, 0, 1f))
            addView(LinearLayout(this@DialActivity).apply {
                val spacing = AppAppearance.dp(this@DialActivity, 20f)
                setPadding(spacing, AppAppearance.dp(this@DialActivity, 4f), spacing, AppAppearance.dp(this@DialActivity, 8f))
                addView(Button(this@DialActivity).apply {
                    text = "Call"; AppAppearance.styleButton(this, true); setOnClickListener { placeCall() }
                })
            })
        }
        setContentView(screen)
=======
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
>>>>>>> origin/main
    }
    override fun onNewIntent(intent: Intent) { super.onNewIntent(intent); setIntent(intent); readIntent(intent) }
    override fun onResume() { super.onResume(); renderContacts() }
    private fun renderContacts() {
        contacts.removeAllViews()
        val saved = runCatching { CallContactStore(this).load() }.getOrNull()
        if (saved == null) {
<<<<<<< HEAD
            contacts.addView(AppAppearance.text(this, "Saved contact shortcuts are unavailable. You can still dial a number.", 14f, false))
            return
        }
        if (saved.contacts.isEmpty()) contacts.addView(AppAppearance.text(this, "Contacts shared by your parent will appear here.", 14f, false))
=======
            contacts.addView(TextView(this).apply { text = "Saved contact shortcuts are unavailable. You can still dial a number." })
            return
        }
>>>>>>> origin/main
        for (contact in saved.contacts.sortedByDescending { it.isParent }) {
            contacts.addView(Button(this).apply {
                text = if (contact.isParent) "Call Parent - ${contact.label}" else contact.label
                isAllCaps = false
<<<<<<< HEAD
                AppAppearance.styleButton(this, false)
=======
>>>>>>> origin/main
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
<<<<<<< HEAD
            val emergency = runCatching {
                if (Build.VERSION.SDK_INT >= 29) getSystemService(TelephonyManager::class.java).isEmergencyNumber(value)
                else android.telephony.PhoneNumberUtils.isEmergencyNumber(value)
            }.getOrDefault(true)
            if (!emergency && DialerSupport.isHeld(this)) {
=======
            val emergency = runCatching { getSystemService(TelephonyManager::class.java).isEmergencyNumber(value) }.getOrDefault(true)
            if (!emergency && getSystemService(RoleManager::class.java).isRoleHeld(RoleManager.ROLE_DIALER)) {
>>>>>>> origin/main
                startActivity(Intent(this, CallActivity::class.java))
            }
        } else Toast.makeText(this, "Call could not start. Use the system phone if needed.", Toast.LENGTH_LONG).show()
    }
    private fun systemPhone() {
<<<<<<< HEAD
        val target = if (Build.VERSION.SDK_INT >= 29) getSystemService(TelecomManager::class.java).systemDialerPackage
            else packageManager.queryIntentActivities(Intent(Intent.ACTION_DIAL, Uri.parse("tel:")), PackageManager.MATCH_SYSTEM_ONLY)
                .firstOrNull { it.activityInfo.packageName != packageName }?.activityInfo?.packageName
=======
        val target = getSystemService(TelecomManager::class.java).systemDialerPackage
>>>>>>> origin/main
        val intent = Intent(Intent.ACTION_DIAL, Uri.fromParts("tel", number.text.toString().take(80), null))
        if (!target.isNullOrBlank() && target != packageName) intent.setPackage(target)
        runCatching { startActivity(if (intent.`package` == null) Intent.createChooser(intent, "Choose phone app") else intent) }
            .onFailure { Toast.makeText(this, "System phone is unavailable.", Toast.LENGTH_LONG).show() }
    }
}
