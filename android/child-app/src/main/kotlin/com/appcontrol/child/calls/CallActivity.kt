package com.appcontrol.child.calls

import android.app.Activity
import android.app.AlertDialog
import android.Manifest
import android.content.Intent
import android.os.Bundle
import android.telecom.Call
import android.telecom.TelecomManager
import android.view.Gravity
import android.view.MotionEvent
import android.widget.Button
import android.widget.LinearLayout
import android.widget.ScrollView
import android.widget.TextView
import android.widget.Toast

class CallActivity : Activity() {
    private lateinit var content: LinearLayout
    private var selected: String? = null
    private var pendingBluetoothCall: String? = null
    private val observer: () -> Unit = { render() }

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        setShowWhenLocked(true)
        selected = savedInstanceState?.getString("selectedCallId") ?: intent.getStringExtra("callId")
        content = LinearLayout(this).apply { orientation = LinearLayout.VERTICAL; gravity = Gravity.CENTER; setPadding(32, 40, 32, 40) }
        setContentView(ScrollView(this).apply { addView(content) })
    }
    override fun onStart() { super.onStart(); CallRegistry.observe(observer); render() }
    override fun onSaveInstanceState(outState: Bundle) {
        outState.putString("selectedCallId", selected)
        super.onSaveInstanceState(outState)
    }
    override fun onStop() { CallRegistry.forget(observer); super.onStop() }
    override fun onNewIntent(intent: Intent) { super.onNewIntent(intent); setIntent(intent); selected = intent.getStringExtra("callId"); render() }

    private fun text(value: String) { content.addView(TextView(this).apply { text = value; textSize = 22f; gravity = Gravity.CENTER }) }
    private fun button(label: String, action: () -> Unit) { content.addView(Button(this).apply { text = label; setOnClickListener { action() } }) }
    private fun action(id: String, type: CallActionGate.Action) {
        val result = CallRegistry.userAction(id, type)
        if (result != CallActionGate.Result.REQUESTED) Toast.makeText(this, "Call action is no longer available.", Toast.LENGTH_SHORT).show()
    }
    private fun render() {
        if (isDestroyed || !::content.isInitialized) return
        content.removeAllViews()
        val calls = CallRegistry.all()
        if (selected == null) selected = calls.firstOrNull { CallRegistry.state(it.call) == CallActionGate.State.RINGING }?.id ?: calls.firstOrNull()?.id
        val entry = selected?.let { CallRegistry.find(it) }
        if (entry == null) {
            setTurnScreenOn(false)
            text("No current call")
        } else {
            val state = CallRegistry.state(entry.call)
            setTurnScreenOn(state == CallActionGate.State.RINGING)
            text(CallRegistry.label(entry.call))
            text(when (state) {
                CallActionGate.State.RINGING -> "Incoming call"
                CallActionGate.State.ACTIVE -> "Call active"
                CallActionGate.State.HOLDING -> "Call on hold"
                CallActionGate.State.DIALING, CallActionGate.State.CONNECTING -> "Connecting"
                CallActionGate.State.SELECTING_ACCOUNT -> "Choose calling account"
                CallActionGate.State.DISCONNECTED -> "Call ended"
                else -> "Call pending"
            })
            if (state == CallActionGate.State.RINGING) {
                button("Answer audio") { action(entry.id, CallActionGate.Action.ANSWER) }
                button("Reject") { action(entry.id, CallActionGate.Action.REJECT) }
            } else if (state in setOf(CallActionGate.State.ACTIVE, CallActionGate.State.HOLDING, CallActionGate.State.DIALING, CallActionGate.State.CONNECTING, CallActionGate.State.SELECTING_ACCOUNT)) {
                button("End call") { action(entry.id, CallActionGate.Action.END) }
            }
            if (state == CallActionGate.State.SELECTING_ACCOUNT) {
                val telecom = getSystemService(TelecomManager::class.java)
                val accounts = if (checkSelfPermission(Manifest.permission.READ_PHONE_STATE) == android.content.pm.PackageManager.PERMISSION_GRANTED) {
                    try { telecom.callCapablePhoneAccounts } catch (_: SecurityException) { emptyList() }
                } else emptyList()
                if (accounts.isEmpty()) button("Allow account selection") { requestPermissions(arrayOf(Manifest.permission.READ_PHONE_STATE), 4405) }
                accounts.forEach { account ->
                    val label = runCatching { telecom.getPhoneAccount(account)?.label?.toString() }.getOrNull() ?: "Calling account"
                    button(label) {
                        if (CallRegistry.find(entry.id) === entry && CallRegistry.state(entry.call) == CallActionGate.State.SELECTING_ACCOUNT) {
                            runCatching { entry.call.phoneAccountSelected(account, false) }
                        }
                    }
                }
            }
            if (state == CallActionGate.State.ACTIVE || state == CallActionGate.State.HOLDING) {
                val service = CallRegistry.service
                if (state == CallActionGate.State.ACTIVE) {
                    text(CallRegistry.audioStatus)
                    service?.audioMessage(entry.id)?.let { text(it) }
                    button("Speaker") { route(entry.id, true) }
                    button("Phone audio") { route(entry.id, false) }
                    button("Audio output") { chooseAudio(entry.id) }
                    if (service != null) button(if (service.muted) "Unmute" else "Mute") {
                        if (CallRegistry.service === service && CallRegistry.find(entry.id) === entry && CallRegistry.state(entry.call) == CallActionGate.State.ACTIVE) runCatching { service.setMuted(!service.muted) }
                    }
                }
                if (entry.call.details.can(Call.Details.CAPABILITY_HOLD)) button(if (state == CallActionGate.State.HOLDING) "Resume call" else "Hold call") {
                    if (CallRegistry.find(entry.id) === entry) runCatching {
                        when (CallRegistry.state(entry.call)) {
                            CallActionGate.State.HOLDING -> entry.call.unhold()
                            CallActionGate.State.ACTIVE -> entry.call.hold()
                            else -> Unit
                        }
                    }
                }
                if (state == CallActionGate.State.ACTIVE) {
                    text("Call keypad")
                    for (row in listOf("123", "456", "789", "*0#")) {
                        content.addView(LinearLayout(this).apply {
                            for (digit in row) addView(Button(this@CallActivity).apply {
                                text = digit.toString()
                                setOnTouchListener { view, event ->
                                    when (event.action) {
                                        MotionEvent.ACTION_DOWN -> if (CallRegistry.find(entry.id) === entry && CallRegistry.state(entry.call) == CallActionGate.State.ACTIVE) runCatching { entry.call.playDtmfTone(digit) }
                                        MotionEvent.ACTION_UP, MotionEvent.ACTION_CANCEL -> { runCatching { entry.call.stopDtmfTone() }; view.performClick() }
                                    }
                                    true
                                }
                            }, LinearLayout.LayoutParams(0, LinearLayout.LayoutParams.WRAP_CONTENT, 1f))
                        })
                    }
                }
            }
        }
        for (other in calls.filter { it.id != selected }) button("View ${CallRegistry.label(other.call)}") { selected = other.id; render() }
        button("Dial pad") { startActivity(Intent(this, DialActivity::class.java)) }
    }
    private fun route(id: String, speaker: Boolean) {
        if (CallRegistry.service?.requestSpeaker(id, speaker) != true) Toast.makeText(this, "Audio route is not available yet.", Toast.LENGTH_SHORT).show()
    }
    private fun chooseAudio(id: String) {
        val service = CallRegistry.service ?: return
        val choices = service.audioChoices(id)
        if (choices.isEmpty()) { Toast.makeText(this, "No audio outputs are available yet.", Toast.LENGTH_SHORT).show(); return }
        AlertDialog.Builder(this).setTitle("Audio output").setItems(choices.map { it.label }.toTypedArray()) { _, index ->
            val choice = choices[index]
            if (CallRegistry.service !== service || CallRegistry.find(id)?.let { CallRegistry.state(it.call) } != CallActionGate.State.ACTIVE) return@setItems
            if (choice.requiresBluetoothPermission) {
                pendingBluetoothCall = id
                requestPermissions(arrayOf(Manifest.permission.BLUETOOTH_CONNECT), 4406)
            } else if (!service.requestAudio(id, choice.id)) Toast.makeText(this, "That audio output is no longer available.", Toast.LENGTH_SHORT).show()
        }.setNegativeButton("Cancel", null).show()
    }
    override fun onRequestPermissionsResult(requestCode: Int, permissions: Array<out String>, grantResults: IntArray) {
        super.onRequestPermissionsResult(requestCode, permissions, grantResults)
        if (requestCode == 4405) render()
        if (requestCode == 4406) {
            val call = pendingBluetoothCall; pendingBluetoothCall = null; render()
            if (call != null && selected == call && checkSelfPermission(Manifest.permission.BLUETOOTH_CONNECT) == android.content.pm.PackageManager.PERMISSION_GRANTED) chooseAudio(call)
        }
    }
}
