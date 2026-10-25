# Contact shortcuts and Call Parent

The parent device screen includes **Call contacts and Call Parent**. Add up to
10 named contacts, optionally select one for Call Parent, then explicitly save.
Numbers require international form: `+`, a nonzero country-code digit and a
total of 7–15 digits. Extensions, pause/wait sequences, USSD, short codes and
duplicates are refused. Names allow 1–40 Unicode code points and 160 UTF-8
bytes without surrounding whitespace or control/format characters.

The child dial pad displays these contacts. Choosing one fills the number;
the child reviews it and presses **Call**. Existing Android permission,
Telecom account selection and emergency handling still apply. Parent requests,
FCM hints, incoming intents and contact sync never place a call. Ordinary dialing
and system-phone fallback remain available. This builds contact shortcuts;
an outgoing-call allowlist/restriction policy is not implemented.

## API and delivery

- Owner `GET /v1/devices/{deviceID}/call-contacts` returns `{version, contacts}`.
- Owner `PUT` at that path replaces the list using `{expectedVersion, contacts}`.
  Each contact requires `label`, `number`, `isParent`. Unknown/missing fields
  are refused. An empty list removes all shortcuts; request limit is 8 KiB.
- Authenticated child `GET /v1/device/call-contacts` returns only its own list.

Initial version is zero with an empty list. Writes increment it; stale edits
return 409. Contact and device-control policy versions are independent.
Cross-owner and revoked-device access is refused. Migration 0019 stores the
list and appends an immutable `policy_updated` event in the same transaction.
Audits contain account/device identifiers, never contact names/numbers.
An audit-write failure rolls back the contact update.

After commit, the existing wake-up adapter receives an opaque hint. The child
fetches contacts as best-effort work after durable command/policy reconciliation.
Older servers or failed contact fetches do not block those controls. Delivery
is sync-based, without a contact-specific receipt: saving a server version
does not prove child delivery. Refresh before editing again after a conflict
or uncertain save.

## Storage and privacy

Deliberately configured contacts are stored on the self-hosted backend and
included in encrypted backups. They are separate from live caller information;
this adds no caller-number collection, contact-book scanning or call history.
Contact names/numbers must never enter logs or FCM payloads.

The child copy uses Android Keystore AES-GCM encryption. Associated data and
the cache slot bind it to its enrolled server origin and device ID. Versions
cannot regress or change contents at the same version. Unreadable cache state
fails closed for shortcuts; the saved version floor is never silently discarded.
Normal dialing remains available. Phone and parent screens block screenshots
and avoid saving number editor text in activity state.

Cached shortcuts remain available offline, including old numbers until a
newer list is fetched. Network revocation cannot erase an offline cache.
Changes are not instantaneous and must not serve as outgoing-call restrictions.

## Verification limits

Focused Go service/HTTP checks cover authentication, strict payloads, dangerous
dial strings, duplicate/primary validation, stale edits, clearing, revocation
and audit rollback. Android debug builds check compilation. Parent editing,
child delivery, offline user flows and SIM dialing still require device
acceptance. No call was placed during contact implementation.

On 25 October 2026, `scripts/test-android-contact-cache.ps1` passes the
`sealed-cache-v1` probe on the isolated, unenrolled Android 37 emulator. The
production cache performs a real Android Keystore AES-GCM save/load; key bytes
are non-exportable, stored preferences contain no plaintext contact, rollback
and same-version changes are refused, another device binding cannot decrypt
copied ciphertext, corrupt state cannot silently reset the version floor, and a
newer empty list clears contacts. Fixture preferences and any newly created
contact key are cleaned up. Existing keys are retained. The probe uses separate
preferences and never modifies production enrollment or places calls. Evidence:
`logs/build/2026-10-25.txt`. Process-death/reboot persistence, server delivery,
UI selection and real-phone acceptance remain separate checks.
