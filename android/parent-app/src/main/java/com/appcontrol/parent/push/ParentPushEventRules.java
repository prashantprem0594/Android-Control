package com.appcontrol.parent.push;

import java.nio.charset.StandardCharsets;
import java.security.MessageDigest;
import java.time.Duration;
import java.time.Instant;
import java.util.Map;
import java.util.UUID;

/** Pure validation for an authenticated feed plus a separately refreshed current call. */
public final class ParentPushEventRules {
    private ParentPushEventRules() {}
    public static final class Event {
        public final String eventId, deviceId, callId;
        public final long sequence;
        public final Instant observedAt, expiresAt;
        public Event(String eventId, String deviceId, String callId, long sequence, Instant observedAt, Instant expiresAt) {
            if (eventId == null || eventId.length() < 1 || eventId.length() > 192 ||
                !eventId.chars().allMatch(c -> c >= 33 && c <= 126) ||
                deviceId == null || !deviceId.matches("dev_[A-Za-z0-9_-]{22}") || sequence < 1 ||
                observedAt == null || expiresAt == null || !expiresAt.isAfter(observedAt) ||
                expiresAt.isAfter(observedAt.plusSeconds(30))) throw new IllegalArgumentException("Invalid event");
            UUID call = UUID.fromString(callId);
            if (!call.toString().equals(callId) || call.version() != 4 || call.variant() != 2)
                throw new IllegalArgumentException("Invalid call identity");
            this.eventId = eventId; this.deviceId = deviceId; this.callId = callId;
            this.sequence = sequence; this.observedAt = observedAt; this.expiresAt = expiresAt;
        }
    }
    public static long remainingMillis(Event event, Instant feedTime, long feedStarted,
                                       Instant reportTime, Instant reportObserved, long reportSequence,
                                       Map<String, String> currentCalls, long reportStarted, long now) {
        try {
            if (event == null || feedTime == null || reportTime == null || reportObserved == null ||
                currentCalls == null || currentCalls.size() > 8 ||
                feedStarted < 0 || reportStarted < feedStarted || now < reportStarted ||
                event.observedAt.isAfter(feedTime) || reportTime.isBefore(feedTime) ||
                reportObserved.isAfter(reportTime) || reportSequence < event.sequence ||
                !"RINGING".equals(currentCalls.get(event.callId))) return 0;
            long eventBudget = Duration.between(feedTime, event.expiresAt).toMillis() - (now - feedStarted);
            long reportBudget = Duration.between(reportTime, reportObserved.plusSeconds(30)).toMillis() - (now - reportStarted);
            return Math.max(0, Math.min(30_000, Math.min(eventBudget, reportBudget)));
        } catch (RuntimeException invalid) { return 0; }
    }
    public static String fingerprint(String value) {
        try {
            byte[] digest = MessageDigest.getInstance("SHA-256").digest(value.getBytes(StandardCharsets.UTF_8));
            StringBuilder result = new StringBuilder(64);
            for (byte item : digest) result.append(String.format(java.util.Locale.ROOT, "%02x", item & 255));
            return result.toString();
        } catch (java.security.NoSuchAlgorithmException unavailable) { throw new IllegalStateException(unavailable); }
    }
}
