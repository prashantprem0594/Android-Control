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

The 26 October side-by-side child DialActivity also passes local encrypted-cache
selection checks: Call Parent fills the number for review, changed/removed
shortcuts cannot overwrite a manually entered number, and the updated shortcut
fills its new number. FLAG_SECURE and disabled saved number state are checked.
The probe requires calling permission denied and dialer role absent, never
presses Call, and restores prior cache/enrollment/key state. Latest evidence:
`.cache/android-server-https-reports/d1da1d46760e4a288dbb796e12700523.json`.
The cache is seeded locally for this UI component check; live server delivery
remains the separate CommandSync proof. Real SIM calling remains unverified.

Focused Go service/HTTP checks cover authentication, strict payloads, dangerous
dial strings, duplicate/primary validation, stale edits, clearing, revocation
and audit rollback. Android debug builds check compilation. Parent editing UI,
physical child delivery, offline user flows and SIM dialing still require device
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
`logs/build/2026-10-25.txt`. Server delivery, UI selection and real-phone
acceptance remain separate checks.

The later `cache-persistence-v1` probe also passes across an explicitly logged
force-stop and real Android 37 emulator reboot. Preparation records the PID,
boot count and ciphertext digest; restart requires another PID with unchanged
boot count, and reboot requires a newer boot count plus the prior restart proof.
Both phases load the same encrypted version-11 contact and retain the
non-exportable key, reject version rollback/same-version conflict, and confirm
the ciphertext stays unchanged. Separate fixture preferences and any newly
created contact key are removed by the cleanup phase; existing keys are retained.
No production enrollment/cache is modified. Run
`scripts/test-android-contact-persistence.ps1` with explicit `prepare`, `restart`,
`reboot`, `cleanup` phases; process stop and emulator reboot must be separately
logged. This is emulator cache evidence, not live delivery or a real-phone test.

On 26 October 2026, live HTTPS delivery passes on the guarded Android 37 AVD.
The test writes a synthetic parent contact through the server API; production
`CommandSync` fetches and saves it in the encrypted `CallContactStore`. New store
instances verify version 1, a replacement label/number at version 2, and an empty
list at version 3. Stored values contain neither the initial label nor number
in plaintext. The runner requires `server_https_contact_sync_probe=passed`.
Unique fixture contact preferences are deleted and existing contact keys are
retained; full server/proxy/reverse-mapping cleanup passes. Evidence:
`.cache/android-server-https-reports/b7c094f3298f4c3b94dba6878016c328.json`
and `logs/build/2026-10-26.txt`. This uses test-only parent API requests and
ephemeral CA/SPKI trust; parent editing UI, local contact selection, outgoing
calls and physical-device delivery remain unverified. No call was placed.

The later 26 October recovery check also passes: the disposable TLS proxy
returns 503 only for child contact fetches after a replacement is saved on the
server. Production CommandSync still acknowledges a safely declined command
and fetches policy. A fresh store reads the prior contact, and the encrypted
preference values remain byte-for-byte unchanged. Restoring the endpoint lets
the next sync receive the replacement; the subsequent clear also passes.
The required marker is `server_https_contact_recovery_probe=passed`. Evidence:
`.cache/android-server-https-reports/67652bcd119f49ff84a944423362f27c.json`;
all 12 integration stages and fixture cleanup passed. This tests a contact-route
503 response, not full-server downtime, airplane mode or network switching.
