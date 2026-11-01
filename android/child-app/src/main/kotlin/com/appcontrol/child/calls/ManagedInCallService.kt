package com.appcontrol.child.calls

import android.Manifest
import android.bluetooth.BluetoothDevice
import android.content.pm.PackageManager
import android.os.Build
import android.os.Handler
import android.os.Looper
import android.os.OutcomeReceiver
import android.util.Log
import android.telecom.Call
import android.telecom.CallAudioState
import android.telecom.CallEndpoint
import android.telecom.CallEndpointException
import android.telecom.InCallService
import com.appcontrol.child.commands.RemoteCallSnapshotSync
import java.util.UUID
import java.util.concurrent.Executors

/** Visible Telecom UI service; does not record, stream or persist call media. */
class ManagedInCallService : InCallService() {
    private val callbacks = mutableMapOf<Call, Call.Callback>()
    private var endpoints: List<CallEndpoint> = emptyList()
    private val snapshotHandler = Handler(Looper.getMainLooper())
    private val snapshotExecutor = Executors.newSingleThreadExecutor()
    private var snapshotPending = false
    private var snapshotDirty = false
    private var lastSnapshotFingerprint: List<CallRegistry.RemoteCallSnapshot>? = null
    private val snapshotObserver: () -> Unit = { scheduleCallSnapshot() }
    private val snapshotRunnable = Runnable { publishCallSnapshot() }
    private val snapshotHeartbeat = Runnable { scheduleCallSnapshot(force = true) }
    data class AudioChoice(val id: String, val label: String, val kind: String, val requiresBluetoothPermission: Boolean = false)
    private val bluetoothChoices = linkedMapOf<String, BluetoothDevice>()
    private val routeHandler = Handler(Looper.getMainLooper())
    private var pendingRoute: String? = null
    private var routeCallId: String? = null
    private var routeGeneration = 0L
    var routeRequestMessage: String? = null
        private set
    var muted = false
        private set

    override fun onCreate() {
        super.onCreate()
        CallRegistry.all().forEach { CallNotifications.cancel(this, it.id) }
        CallRegistry.clear()
        CallRegistry.service = this
        CallRegistry.observe(snapshotObserver)
        scheduleCallSnapshot(force = true)
    }

    override fun onCallAdded(call: Call) {
        super.onCallAdded(call)
        if (CallRegistry.service !== this) return
        val entry = CallRegistry.add(call)
        if (callbacks.containsKey(call)) return
        val callback = object : Call.Callback() {
            override fun onStateChanged(call: Call, state: Int) { refresh(call) }
            override fun onDetailsChanged(call: Call, details: Call.Details) { refresh(call) }
        }
        callbacks[call] = callback
        call.registerCallback(callback)
        CallNotifications.show(this, entry)
        CallRegistry.changed()
    }

    private fun refresh(call: Call) {
        if (CallRegistry.service !== this) return
        val entry = CallRegistry.all().firstOrNull { it.call === call } ?: return
        if (routeCallId == entry.id && CallRegistry.state(call) != CallActionGate.State.ACTIVE) clearAudioRequest()
        CallNotifications.show(this, entry)
        CallRegistry.changed()
    }

    private fun scheduleCallSnapshot(delayMillis: Long = 250, force: Boolean = false) {
        if (CallRegistry.service !== this) return
        val snapshot = CallRegistry.remoteSnapshot()
        if (!force && snapshot == lastSnapshotFingerprint) return
        lastSnapshotFingerprint = snapshot
        snapshotHandler.removeCallbacks(snapshotRunnable)
        snapshotHandler.postDelayed(snapshotRunnable, delayMillis)
    }

    private fun publishCallSnapshot() {
        if (CallRegistry.service !== this) return
        if (snapshotPending) {
            snapshotDirty = true
            return
        }
        snapshotPending = true
        snapshotDirty = false
        snapshotExecutor.execute {
            var succeeded = false
            try {
                RemoteCallSnapshotSync(applicationContext).publish()
                succeeded = true
            } catch (_: Exception) {
                Log.w("ManagedInCallService", "Call snapshot sync failed")
            }
            snapshotHandler.post {
                snapshotPending = false
                if (CallRegistry.service !== this) return@post
                if (snapshotDirty) {
                    scheduleCallSnapshot(150, force = true)
                } else if (succeeded) {
                    scheduleSnapshotHeartbeat()
                } else {
                    scheduleCallSnapshot(5_000, force = true)
                }
            }
        }
    }

    private fun scheduleSnapshotHeartbeat() {
        snapshotHandler.removeCallbacks(snapshotHeartbeat)
        if (CallRegistry.all().any { CallRegistry.state(it.call) != CallActionGate.State.DISCONNECTED }) {
            snapshotHandler.postDelayed(snapshotHeartbeat, 15_000)
        }
    }

    override fun onCallRemoved(call: Call) {
        callbacks.remove(call)?.let { call.unregisterCallback(it) }
        if (CallRegistry.service === this) {
            if (routeCallId?.let { CallRegistry.find(it)?.call } === call) clearAudioRequest()
            val entry = CallRegistry.remove(call)
            if (entry != null) CallNotifications.cancel(this, entry.id)
        }
        super.onCallRemoved(call)
    }

    override fun onCallAudioStateChanged(state: CallAudioState?) {
        super.onCallAudioStateChanged(state)
        if (CallRegistry.service !== this || state == null) return
        muted = state.isMuted
        if (Build.VERSION.SDK_INT < 34) CallRegistry.audioStatus = when (state.route) {
            CallAudioState.ROUTE_SPEAKER -> "Speaker"
            CallAudioState.ROUTE_BLUETOOTH -> "Bluetooth"
            CallAudioState.ROUTE_WIRED_HEADSET -> "Wired headset"
            else -> "Earpiece"
        }
        if (Build.VERSION.SDK_INT < 34) {
            val pending = pendingRoute
            val matching = if (pending?.startsWith("legacy:") == true) pending.removePrefix("legacy:").toIntOrNull() == state.route
                else if (pending?.startsWith("bluetooth:") == true && hasBluetoothPermission())
                    runCatching { state.route == CallAudioState.ROUTE_BLUETOOTH && state.activeBluetoothDevice == bluetoothChoices[pending] }.getOrDefault(false)
                else false
            if (matching) { pendingRoute = null; routeCallId = null; routeRequestMessage = null }
        }
        CallRegistry.changed()
    }

    override fun onAvailableCallEndpointsChanged(availableEndpoints: MutableList<CallEndpoint>) {
        if (Build.VERSION.SDK_INT < 34) return
        super.onAvailableCallEndpointsChanged(availableEndpoints)
        endpoints = availableEndpoints.toList()
        if (pendingRoute?.startsWith("endpoint:") == true && endpoints.none { "endpoint:${it.identifier}" == pendingRoute }) {
            pendingRoute = null; routeRequestMessage = "Selected audio output is no longer available."
        }
        if (CallRegistry.service === this) CallRegistry.changed()
    }

    override fun onCallEndpointChanged(endpoint: CallEndpoint) {
        if (Build.VERSION.SDK_INT < 34) return
        super.onCallEndpointChanged(endpoint)
        if (CallRegistry.service !== this) return
        if (pendingRoute == "endpoint:${endpoint.identifier}") { pendingRoute = null; routeCallId = null; routeRequestMessage = null }
        CallRegistry.audioStatus = when (endpoint.endpointType) {
            CallEndpoint.TYPE_SPEAKER -> "Speaker"
            CallEndpoint.TYPE_EARPIECE -> "Earpiece"
            CallEndpoint.TYPE_WIRED_HEADSET -> "Wired headset"
            CallEndpoint.TYPE_BLUETOOTH -> "Bluetooth"
            else -> "Call audio"
        }
        CallRegistry.changed()
    }

    override fun onMuteStateChanged(isMuted: Boolean) {
        super.onMuteStateChanged(isMuted)
        muted = isMuted
        if (CallRegistry.service === this) CallRegistry.changed()
    }

    private fun active(callId: String): Boolean {
        check(Looper.myLooper() == Looper.getMainLooper()) { "Telecom access requires main thread" }
        val entry = CallRegistry.find(callId) ?: return false
        return CallRegistry.service === this && CallRegistry.state(entry.call) == CallActionGate.State.ACTIVE
    }
    private fun clearAudioRequest() {
        pendingRoute = null; routeCallId = null; routeRequestMessage = null; routeGeneration++
    }
    fun audioMessage(callId: String): String? = routeRequestMessage.takeIf { routeCallId == callId }
    fun audioChoices(callId: String): List<AudioChoice> {
        if (!active(callId)) return emptyList()
        return runCatching {
            if (Build.VERSION.SDK_INT >= 34) endpoints.mapNotNull { endpoint ->
                val kind = when (endpoint.endpointType) {
                    CallEndpoint.TYPE_SPEAKER -> "speaker"
                    CallEndpoint.TYPE_EARPIECE -> "earpiece"
                    CallEndpoint.TYPE_WIRED_HEADSET -> "wired"
                    CallEndpoint.TYPE_BLUETOOTH -> "bluetooth"
                    else -> return@mapNotNull null
                }
                val label = when (kind) {
                    "speaker" -> "Speaker"
                    "earpiece" -> "Earpiece"
                    "wired" -> "Wired headset"
                    else -> endpoint.endpointName?.toString()?.takeIf { it.isNotBlank() }?.let { "Bluetooth: $it" } ?: "Bluetooth"
                }
                AudioChoice("endpoint:${endpoint.identifier}", label, kind)
            } else {
                val state = callAudioState ?: return emptyList()
                val choices = mutableListOf<AudioChoice>()
                for ((route, pair) in listOf(CallAudioState.ROUTE_EARPIECE to ("Earpiece" to "earpiece"),
                    CallAudioState.ROUTE_SPEAKER to ("Speaker" to "speaker"), CallAudioState.ROUTE_WIRED_HEADSET to ("Wired headset" to "wired"))) {
                    if (state.supportedRouteMask and route != 0) choices.add(AudioChoice("legacy:$route", pair.first, pair.second))
                }
                if (state.supportedRouteMask and CallAudioState.ROUTE_BLUETOOTH != 0) {
                    if (!hasBluetoothPermission())
                        choices.add(AudioChoice("bluetooth-permission", "Allow Bluetooth audio access", "bluetooth", true))
                    else {
                        val devices = state.supportedBluetoothDevices.toList()
                        bluetoothChoices.entries.removeAll { it.value !in devices }
                        devices.forEach { device ->
                            val id = bluetoothChoices.entries.firstOrNull { it.value == device }?.key
                                ?: "bluetooth:${UUID.randomUUID()}".also { bluetoothChoices[it] = device }
                            val name = try { device.name } catch (_: SecurityException) { null }
                            val label = name?.takeIf { it.isNotBlank() } ?: "Bluetooth device ${devices.indexOf(device) + 1}"
                            choices.add(AudioChoice(id, label, "bluetooth"))
                        }
                    }
                } else bluetoothChoices.clear()
                choices
            }
        }.getOrDefault(emptyList())
    }
    /** Acceptance is separate from the observed audio route. Stale choices fail. */
    fun requestAudio(callId: String, choiceId: String): Boolean {
        val choice = audioChoices(callId).firstOrNull { it.id == choiceId && !it.requiresBluetoothPermission } ?: return false
        if (!active(callId)) return false
        // Telecom need not send another endpoint-change callback for an output
        // that is already current. Do not leave redundant requests unconfirmed.
        // A different in-flight request must still be overridden through Telecom.
        if (Build.VERSION.SDK_INT >= 34 && pendingRoute == null && runCatching {
                currentCallEndpoint?.let { "endpoint:${it.identifier}" } == choice.id
            }.getOrDefault(false)) {
            clearAudioRequest()
            CallRegistry.changed()
            return true
        }
        val generation = ++routeGeneration
        pendingRoute = choice.id; routeCallId = callId; routeRequestMessage = "Audio change requested; awaiting Telecom."
        fun failed() {
            if (CallRegistry.service === this && routeGeneration == generation && pendingRoute != null) {
                pendingRoute = null; routeRequestMessage = "Audio change request failed."; CallRegistry.changed()
            }
        }
        return runCatching {
            if (Build.VERSION.SDK_INT >= 34) {
                val endpoint = endpoints.firstOrNull { "endpoint:${it.identifier}" == choice.id } ?: error("Audio output changed")
                requestCallEndpointChange(endpoint, mainExecutor, object : OutcomeReceiver<Void, CallEndpointException> {
                    override fun onResult(result: Void?) { /* Wait for the actual endpoint callback. */ }
                    override fun onError(error: CallEndpointException) { failed() }
                })
            } else if (choice.id.startsWith("bluetooth:")) {
                check(hasBluetoothPermission())
                val device = checkNotNull(bluetoothChoices[choice.id])
                check(callAudioState?.supportedBluetoothDevices?.contains(device) == true)
                requestBluetoothAudio(device)
            } else setAudioRoute(choice.id.removePrefix("legacy:").toInt())
            routeHandler.postDelayed({
                if (CallRegistry.service === this && routeGeneration == generation && pendingRoute != null) {
                    if (!active(callId)) clearAudioRequest()
                    else routeRequestMessage = "Selected audio output has not been confirmed yet."
                    CallRegistry.changed()
                }
            }, 5000)
            CallRegistry.changed()
            true
        }.getOrElse { failed(); false }
    }
    private fun hasBluetoothPermission(): Boolean =
        Build.VERSION.SDK_INT < 31 || checkSelfPermission(Manifest.permission.BLUETOOTH_CONNECT) == PackageManager.PERMISSION_GRANTED

    fun requestSpeaker(callId: String, speaker: Boolean): Boolean {
        val choices = audioChoices(callId)
        val choice = if (speaker) choices.firstOrNull { it.kind == "speaker" }
            else choices.firstOrNull { it.kind == "wired" } ?: choices.firstOrNull { it.kind == "earpiece" }
        return choice?.let { requestAudio(callId, it.id) } == true
    }

    override fun onDestroy() {
        snapshotHandler.removeCallbacksAndMessages(null)
        if (CallRegistry.service === this) CallRegistry.forget(snapshotObserver)
        snapshotExecutor.shutdownNow()
        routeHandler.removeCallbacksAndMessages(null); bluetoothChoices.clear(); endpoints = emptyList()
        clearAudioRequest()
        callbacks.forEach { (call, callback) -> call.unregisterCallback(callback) }
        callbacks.clear()
        if (CallRegistry.service === this) {
            CallRegistry.all().forEach { CallNotifications.cancel(this, it.id) }
            CallRegistry.clear()
        }
        super.onDestroy()
    }
}
