# Read-only phone readiness

Use `scripts/check-phone-readiness.ps1` before choosing the next physical acceptance test. It observes the designated phone and installed application without changing either. It never installs an APK, provisions Device Owner, clears data, changes permissions, toggles the screen, places a call or contacts Firebase/the control server.

```powershell
& .\scripts\check-phone-readiness.ps1 -Role parent -Serial I7MVZPB6ZLJRY95X
& .\scripts\check-phone-readiness.ps1 -Role child -Serial 43c0acdd
& .\scripts\check-phone-readiness.ps1 -Role child -Serial C6INTOMB8T55X48H
```

Run through `scripts/run-logged.ps1` when saving evidence. The default ADB server port is 5037; `-AdbServerPort` can select another existing server. The commands target the supplied exact serial. Only these operator-confirmed mappings are accepted:

| Role | Phone / expected device property | App | Supported minimum |
|---|---|---|---|
| Parent | Redmi 14 5G / `beryl` | `com.appcontrol.parent` | Android 13 / API33 |
| Child | Redmi Note 5 Pro / `whyred` | `com.appcontrol.child` | Android 9 / API28 |
| Child | Xiaomi 2508CRN2BI / `tornado` | `com.appcontrol.child` | Android 9 / API28 |

A role/serial mismatch or unknown device is refused before device queries. An authorized transport must also report its expected `ro.product.device`. This matching is an observation of the designated ADB device, not cryptographic device authentication or proof of ownership. Other models/serials require an explicit update of the designated-device mapping rather than bypassing it.

The additional Xiaomi child was designated by the user on 30 October 2026. Its observed OS is Android 15/API35; API28 above is the child app's minimum, not the phone's installed OS. Its designation preserves the earlier child and parent mappings and grants no Device Owner or reset authorization.

## Report meanings

The script emits JSON with `schemaVersion=1` and `scope=read-only-designated-phone-readiness`. It reports:

- Connection state: `device`, `absent`, `offline`, `unauthorized`, `unexpected` or `query-failed`. Missing/failed observations remain unknown.
- Expected device match, sanitized model/API and active Android user. A nonzero active user blocks the default owner-user test context.
- Exact target package installation, version code/name, declared minimum/target SDK and whether the observed OS meets the installed minimum.
- Notification runtime permission scoped to the active Android user. Pre-API33 reports `not-required`; this does not prove notifications or channels are enabled.
- Exact app Firebase messaging service declaration. It indicates a matching manifest component, not enabled Google Play services, token registration, a valid project, an authenticated session, notification publication or live delivery.
- Device Owner observation: `target-app`, `other-app`, `not-observed` or `unknown`. Other owner identities and raw device policy are not returned. An unrecognized format cannot establish ownership.

`localAppInspectionReady=true` means the designated authorized app can be inspected on a supported API in owner-user context. It does **not** require or imply that all `blockers` are resolved. For example, a parent app can launch while its Firebase service is absent, or a child can launch before Device Owner setup. `acceptanceComplete` is always false. Read the explicit blocker list and run the relevant real test before marking an acceptance criterion passed.

Raw package/policy output, accounts, user-interface contents, device-owner names, tokens and keys are not emitted. No account or UI dump query is made. No readiness report should be interpreted as permission to reset/provision either phone.

## Next checks after inspection

For parent continuous alerts, use the configured Firebase build and real HTTPS/server sender inputs, verify sign-in and registration, then run the [screen-off matrix](SCREEN_OFF_ACCEPTANCE.md). A short surviving PID is not push delivery evidence. The ordinary parent build intentionally lacks continuous push.

For child policy testing, Device Owner must first be observed and the provisioning scope must be authorized. Then verify enrollment, strong-auth lock, exclusions, app suspend/restore, offline schedules, internet pause and reboot/recovery on the child only. Real calls require a test SIM and supervised normal-call/audio tests. The parent is excluded from child policy, VPN and dialer fixtures.

## Verification: 29 October 2026

`scripts/test-phone-readiness.ps1` uses mock ADB observations to check role/device safeguards, disconnected/offline/unauthorized/error states, metadata ambiguity, supported API floors, per-user notification parsing, exact Firebase service matching, Device Owner boundaries and output redaction. It performs no phone actions. Tests passed under `PHONE-READINESS-RULES` in `logs/build/2026-10-29.txt`.

The read-only designated parent invocation was recorded under `PHONE-READINESS-PARENT`; its transport was absent, so the report correctly left installed-app and owner observations unknown. No new real-phone acceptance is claimed by that invocation.

30 October 2026: ownership parsing now returns `unknown` for duplicate or conflicting Device Owner sections instead of accepting the first recognized component. Focused mock regressions cover conflicting owners, repeated target owners, unrecognized and null second sections, retained child provisioning blockers and output redaction (`PHONE-OWNER-AMBIGUITY`, `logs/build/2026-10-30.txt`). No device action or physical acceptance was performed.
