package com.appcontrol.child.policy

import org.json.JSONArray
import org.json.JSONObject

object PolicyConfig {
    fun parse(policy: JSONObject): ScheduledPolicy {
        val format = policy.optInt("formatVersion", 1)
        require(format in 1..2) { "Unsupported policy format" }
        val rules = policy.optJSONArray("schedules") ?: JSONArray()
        val parsed = (0 until rules.length()).map { index ->
            val rule = rules.getJSONObject(index)
            val days = rule.getJSONArray("daysOfWeek")
            ScheduledPolicy.Rule(rule.getString("id"), rule.getString("timeZone"),
                (0 until days.length()).map { days.getInt(it) }, rule.getInt("startMinute"), rule.getInt("endMinute"),
                rule.getBoolean("internetPaused"), strings(rule.getJSONArray("blockedPackages")))
        }
        val allowed = strings(policy.optJSONArray("alwaysAllowedPackages") ?: JSONArray())
        require(format >= 2 || parsed.isEmpty() && allowed.isEmpty()) { "Invalid policy format" }
        return ScheduledPolicy(policy.getLong("version"), policy.getBoolean("internetPaused"),
            strings(policy.getJSONArray("blockedPackages")), allowed, parsed)
    }

    private fun strings(values: JSONArray): List<String> = (0 until values.length()).map { values.getString(it) }
}
