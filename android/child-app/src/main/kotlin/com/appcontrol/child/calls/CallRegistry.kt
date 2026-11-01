package com.appcontrol.child.calls

import android.os.Handler
<<<<<<< HEAD
import android.os.Build
=======
>>>>>>> origin/main
import android.os.Looper
import android.telecom.Call
import android.telecom.VideoProfile
import android.telephony.TelephonyManager
import com.appcontrol.child.BuildConfig
import java.time.Instant
import java.util.UUID
import java.util.concurrent.CountDownLatch
import java.util.concurrent.TimeUnit
import java.util.concurrent.atomic.AtomicBoolean
import java.util.concurrent.atomic.AtomicReference

/** Process-local Telecom objects and display information; never persisted or logged. */
object CallRegistry {
    data class Entry(val id: String, val call: Call)
    data class RemoteCallSnapshot(val callId: String, val state: String, val remoteActionsAllowed: Boolean)
    private val entries = linkedMapOf<String, Entry>()
    private val observers = linkedSetOf<() -> Unit>()
    var service: ManagedInCallService? = null
    var audioStatus: String = "Audio route pending"

    fun add(call: Call): Entry {
        requireMain()
        return entries.values.firstOrNull { it.call === call } ?: Entry(UUID.randomUUID().toString(), call).also { entries[it.id] = it }
    }
    fun remove(call: Call): Entry? {
        requireMain()
        val entry = entries.values.firstOrNull { it.call === call } ?: return null
        entries.remove(entry.id)
        changed()
        return entry
    }
    fun all(): List<Entry> = entries.values.toList()
    fun find(id: String): Entry? = entries[id]
    fun observe(observer: () -> Unit) { requireMain(); observers.add(observer) }
    fun forget(observer: () -> Unit) { requireMain(); observers.remove(observer) }
    fun changed() { requireMain(); observers.toList().forEach { it() } }
    fun clear() { requireMain(); entries.clear(); service = null; audioStatus = "Audio route pending"; changed() }
    fun remoteSnapshot(): List<RemoteCallSnapshot> {
        if (Looper.myLooper() == Looper.getMainLooper()) return snapshotOnMain()
        val result = AtomicReference<List<RemoteCallSnapshot>>()
        val failure = AtomicReference<RuntimeException?>()
        val finished = CountDownLatch(1)
        check(Handler(Looper.getMainLooper()).post {
            try {
                result.set(snapshotOnMain())
            } catch (problem: RuntimeException) {
                failure.set(problem)
            } finally {
                finished.countDown()
            }
        }) { "Could not read active call snapshot" }
        check(finished.await(5, TimeUnit.SECONDS)) { "Timed out reading active call snapshot" }
        failure.get()?.let { throw IllegalStateException("Could not read active call snapshot", it) }
        return checkNotNull(result.get())
    }

    private fun snapshotOnMain(): List<RemoteCallSnapshot> {
        requireMain()
        return entries.values.map {
            val details = it.call.details
            val platformEmergency = details.hasProperty(Call.Details.PROPERTY_EMERGENCY_CALLBACK_MODE) ||
                details.hasProperty(Call.Details.PROPERTY_NETWORK_IDENTIFIED_EMERGENCY_CALL)
            val remoteAllowed = RemoteCallPermission.allowed(BuildConfig.REMOTE_CALLS_ENABLED, platformEmergency,
                details.handle?.scheme, details.handle?.schemeSpecificPart) { address ->
<<<<<<< HEAD
                isEmergencyNumber(address)
=======
                service?.getSystemService(TelephonyManager::class.java)?.isEmergencyNumber(address) ?: true
>>>>>>> origin/main
            }
            RemoteCallSnapshot(it.id, state(it.call).name, remoteAllowed)
        }
    }

<<<<<<< HEAD
    fun state(call: Call): CallActionGate.State = when (if (Build.VERSION.SDK_INT >= 31) call.details.state else call.state) {
=======
    fun state(call: Call): CallActionGate.State = when (call.details.state) {
>>>>>>> origin/main
        Call.STATE_RINGING, Call.STATE_SIMULATED_RINGING -> CallActionGate.State.RINGING
        Call.STATE_ACTIVE -> CallActionGate.State.ACTIVE
        Call.STATE_HOLDING -> CallActionGate.State.HOLDING
        Call.STATE_DIALING -> CallActionGate.State.DIALING
        Call.STATE_CONNECTING -> CallActionGate.State.CONNECTING
        Call.STATE_SELECT_PHONE_ACCOUNT -> CallActionGate.State.SELECTING_ACCOUNT
        Call.STATE_DISCONNECTING -> CallActionGate.State.DISCONNECTING
        Call.STATE_DISCONNECTED -> CallActionGate.State.DISCONNECTED
        else -> CallActionGate.State.OTHER
    }
    fun label(call: Call): String = call.details.callerDisplayName?.takeIf { it.isNotBlank() }
        ?: call.details.handle?.schemeSpecificPart?.takeIf { it.isNotBlank() } ?: "Unknown caller"

    private fun emergency(call: Call): Boolean = runCatching {
        val details = call.details
        val platformEmergency = details.hasProperty(Call.Details.PROPERTY_EMERGENCY_CALLBACK_MODE) ||
            details.hasProperty(Call.Details.PROPERTY_NETWORK_IDENTIFIED_EMERGENCY_CALL)
        EmergencyCallGuard.isProtected(platformEmergency, details.handle?.scheme, details.handle?.schemeSpecificPart) {
<<<<<<< HEAD
            isEmergencyNumber(it)
        }
    }.getOrDefault(true)

    private fun isEmergencyNumber(number: String): Boolean =
        if (Build.VERSION.SDK_INT >= 29) service?.getSystemService(TelephonyManager::class.java)?.isEmergencyNumber(number) ?: true
        else android.telephony.PhoneNumberUtils.isEmergencyNumber(number)

=======
            service?.getSystemService(TelephonyManager::class.java)?.isEmergencyNumber(it) ?: true
        }
    }.getOrDefault(true)

>>>>>>> origin/main
    private val port = object : CallActionGate.Calls {
        override fun lookup(id: String): CallActionGate.Snapshot? = entries[id]?.let {
            CallActionGate.Snapshot(it.id, state(it.call), emergency(it.call))
        }
        override fun perform(id: String, action: CallActionGate.Action) {
            val call = checkNotNull(entries[id]).call
            when (action) {
                CallActionGate.Action.ANSWER -> call.answer(VideoProfile.STATE_AUDIO_ONLY)
                CallActionGate.Action.REJECT -> call.reject(false, null)
                CallActionGate.Action.END -> call.disconnect()
            }
        }
    }
    fun userAction(id: String, action: CallActionGate.Action): CallActionGate.Result {
        requireMain()
        val now = Instant.now()
        return CallActionGate.request(port, id, action, now.plusSeconds(5), now, false)
    }
    fun remoteAction(id: String, action: CallActionGate.Action, expires: Instant): CallActionGate.Result {
        requireMain()
        return CallActionGate.request(port, id, action, expires, Instant.now(), true)
    }

    fun executeRemoteAction(id: String, action: CallActionGate.Action, expires: Instant, shouldStop: () -> Boolean): Boolean {
        check(Looper.myLooper() != Looper.getMainLooper()) { "Call command must wait off the main thread" }
        val finished = CountDownLatch(1)
        val observed = AtomicBoolean(false)
        val requested = AtomicBoolean(false)
        val handler = Handler(Looper.getMainLooper())
        val observer = {
            if (shouldStop() || !Instant.now().isBefore(expires)) {
                finished.countDown()
            } else {
                val entry = find(id)
                val present = entry != null
                val state = entry?.let { state(it.call) } ?: CallActionGate.State.DISCONNECTED
                if (CallActionGate.observed(action, state, present)) {
                    observed.set(true)
                    finished.countDown()
                }
            }
        }
        check(handler.post {
            if (shouldStop() || !Instant.now().isBefore(expires)) {
                finished.countDown()
            } else {
                observe(observer)
                requested.set(remoteAction(id, action, expires) == CallActionGate.Result.REQUESTED)
                if (!requested.get()) {
                    forget(observer)
                    finished.countDown()
                } else {
                    observer()
                }
            }
        }) { "Could not dispatch call command" }
        try {
            while (!finished.await(100, TimeUnit.MILLISECONDS)) {
                if (shouldStop() || !Instant.now().isBefore(expires)) break
            }
        } finally {
            check(handler.post { forget(observer) }) { "Could not release call observer" }
        }
        return requested.get() && observed.get() && !shouldStop() && Instant.now().isBefore(expires)
    }

    private fun requireMain() { check(Looper.myLooper() == Looper.getMainLooper()) { "Telecom access requires main thread" } }
}
