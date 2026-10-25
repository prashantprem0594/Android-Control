# Offline schedules and always-allowed apps

Policy format 2 adds `alwaysAllowedPackages` and `schedules`. Each policy remains
a full versioned replacement. Omitting these optional fields on a parent PUT
clears them; clients editing existing policies must preserve fields they intend
to keep. Base-only policies use format 1. Device fetches advertise
`X-Policy-Format: 2`; a legacy fetch receives 409 `unsupported_policy_format`
when the desired policy needs format 2. The child validates the complete
snapshot before replacing its private cache.

Example parent PUT body for weekday school time and daily bedtime:

```json
{
  "expectedVersion": 3,
  "internetPaused": false,
  "blockedPackages": [],
  "alwaysAllowedPackages": ["org.example.familychat"],
  "schedules": [
    {
      "id": "school",
      "timeZone": "Asia/Kolkata",
      "daysOfWeek": [1, 2, 3, 4, 5],
      "startMinute": 480,
      "endMinute": 960,
      "internetPaused": true,
      "blockedPackages": ["org.example.game"]
    },
    {
      "id": "bedtime",
      "timeZone": "Asia/Kolkata",
      "daysOfWeek": [1, 2, 3, 4, 5, 6, 7],
      "startMinute": 1320,
      "endMinute": 420,
      "internetPaused": true,
      "blockedPackages": ["org.example.game"]
    }
  ]
}
```

## Rule semantics

Weekdays use ISO numbering: Monday 1 through Sunday 7. Minutes are local wall
clock values in the rule's explicit time zone. Starts are inclusive and ends
exclusive. An overnight rule belongs to its starting weekday: Friday 22:00 to
07:00 continues into Saturday morning. End minute 1440 means midnight at the
end of the selected day; 0 to 1440 represents a full day. Equal endpoints are
rejected. Different time zones can be used for different rules.

Active rules add restrictions to the base policy. App blocks are unioned and
internet pause uses OR; a rule cannot override a base pause with false.
Always-allowed apps are removed from the effective block set and from the VPN
target list. This exemption does not override a separate device-lock command.
Critical/system exclusions remain mandatory and cannot be removed by policy.

There are at most 20 rules, 100 always-allowed packages, and 100 distinct blocked
targets across the base and all rules. Rule IDs are distinct, time zones must
resolve, and duplicate weekday/package entries are rejected. Serialized policy
snapshots are capped at 64 KiB. Existing ownership, active-device, inventory,
optimistic-version and immutable-audit checks still apply.

During daylight-saving gaps, only actual local clock times are evaluated; a
window wholly inside a skipped interval never becomes active. During repeated
hours, the same wall-clock window can become active twice. Boundary planning
includes both offsets of a repeated time, gap transitions, and the next zone
offset change. Rule order does not change the effective result.

## Offline execution and timing

The child evaluates the latest private cached snapshot without contacting the
server. It applies the resulting package set and VPN state, then arms the next
boundary. A separate persisted policy job has no network constraint. Boot,
app-update, wall-clock, time-zone and alarm-access events re-evaluate the cache.
Late delivery evaluates the current window rather than replaying old windows.
Boundary changes also request a network sync to publish a result when possible.

Android alarm behavior follows the
[AlarmManager guidance](https://developer.android.com/develop/background-work/services/alarms).
The child uses exact idle-aware alarms when access is available, with an inexact
fallback otherwise. **Allow precise schedule timing** opens Android's alarm
access settings at the user's request. Inexact alarms and periodic jobs can be
delayed, especially during idle/power restrictions; a short window can be missed.
The app does not claim exact boundary enforcement without that access and real
device verification. Boot restoration still waits for unlocked app storage.

## Reporting effective state

Policy version alone cannot identify a changing schedule window. New child
reports therefore include a SHA-256 `stateKey` over a canonical representation
of version, effective pause, sorted block targets and sorted always-allowed
packages. The server independently calculates the current key. A stale or
clock/zone-mismatched key returns 409; format 2 cannot be confirmed by a legacy
report without a key. Identical results deduplicate. A different effective key
creates a new audit event even when the policy version is unchanged.

Parent reads expose current `effectiveStateKey` separately from
`reported.stateKey`. A matching version/result from a previous window is not
current confirmation if these keys differ. Format 1 legacy reports without keys
remain supported; new children provide keys for both formats. Reports describe
the last accepted observation, not a guarantee that Android remained unchanged.
Keys travel only over our HTTPS API and are not added to FCM or audit payloads.

## Verification

`contracts/policy.schema.json` documents snapshots; semantic checks also run in
the service. `contracts/schedule-vectors.tsv` supplies 12 common expected states
and independently computed hashes to Go and JVM checks. Tests cover half-open
boundaries, overnight/weekly wrap, daylight-saving gaps/repeated hours, overlap,
allow overrides, validation, capability negotiation, stale reports, rollback,
audit deduplication and encrypted restore of rules and results.

Full Go tests, vet, server build, JVM policy checks and both debug APK builds
passed on 23 October 2026. Actual Redmi boundary timing, idle behavior,
force-stop/reboot recovery, offline package/VPN changes and permission revocation
remain acceptance work. No attached device was available for those checks.

## Real Android alarm boundary evidence (24 October 2026)

scripts/test-android-schedule-boundary.ps1 runs a test-APK-only probe on the isolated, unenrolled Device Owner Android 37 AVD. Exact alarm access is required. The initial run refused the default permission state without changing the policy. After temporary test access was granted, a UTC rule for the disposable fixture crossed actual start and end wall-clock boundaries. Only initial setup called PolicyEnforcer: subsequent suspension/restoration and changed effective-state keys came from the production alarm path. Both DPM and PackageManager observations passed within a 45-second tolerance. The fixture and cached preferences were restored, and the original alarm access mode was restored separately through logged ADB. No synthetic broadcast or device clock change was used. This proves the exact-alarm path on this active emulator; Redmi, idle/inexact timing, enrolled delivery and offline VPN remain open.

Run through the internally logged runner with -Serial 127.0.0.1:5581 -AdbServerPort 5038 after building/installing the matching child test APK. The probe refuses enrolled devices, nonmatching emulator fingerprints, nonempty desired policies/ledgers, missing Device Owner or missing exact-alarm access. Interrupted instrumentation may leave its fixture policy; inspect and recover before reuse.
