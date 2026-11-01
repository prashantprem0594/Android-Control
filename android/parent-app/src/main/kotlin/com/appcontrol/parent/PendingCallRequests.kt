package com.appcontrol.parent

import android.content.SharedPreferences
import org.json.JSONObject

/** A transport failure or an unrelated record must never release a retry identity. */
internal object PendingCallRequests {
    fun clearRejected(state: SharedPreferences, deviceId: String, key: String, callId: String,
                      action: String, failure: ParentClient.RequestFailure): Boolean {
        if (!failure.callRequestNotQueued) return false
        val record = JSONObject(checkNotNull(state.getString(deviceId, null)))
        check(record.getString("key") == key && record.getString("callId") == callId && record.getString("action") == action)
        check(state.edit().remove(deviceId).commit())
        return true
    }
}
