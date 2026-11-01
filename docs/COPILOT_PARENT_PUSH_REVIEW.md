# Parent push review

**Scope:** identify what is already present and what remains for remote parent
call alerts. This is a repository review only; no builds, device tests, Firebase
requests, production changes, or physical-device access were performed.

## Existing behavior verified in the repository

“Verified” below means the behavior is implemented and supported by the cited
automated checks or recorded acceptance evidence. It does not mean this review
reran those checks or confirmed live Firebase delivery.

| Area | Existing behavior and evidence |
|---|---|
| Child-side FCM wakeups | FCM is an optional **child** app variant. Its service schedules normal synchronization for token refresh/deletion and accepted opaque hints; it does not execute commands or perform network work in the callback ([`FirebaseWakeupService.kt`](../android/child-app/src/firebase/kotlin/com/appcontrol/child/wakeup/FirebaseWakeupService.kt#L7-L14)). The child obtains its current token after durable sync and registers it using its signed device session ([`CommandSync.kt`](../android/child-app/src/main/kotlin/com/appcontrol/child/commands/CommandSync.kt#L144-L152)). |
| Child token registry | `PUT`/`DELETE /v1/device/wakeup-registration` is device-session authenticated. The registry encrypts tokens, derives the device from that session, replaces the device's existing token on registration, and scopes ciphertext to the device identity ([`wakeup_registration.go`](../server/internal/httpapi/wakeup_registration.go#L8-L39), [`wakeup_tokens.go`](../server/internal/control/wakeup_tokens.go#L14-L80), [`0015_wakeup_registration.sql`](../server/internal/storage/migrations/0015_wakeup_registration.sql#L1-L9)). `TestWakeupTokensEncryptBindRotateAndRevoke` covers encryption, device binding, rotation, invalid credentials, and revocation cleanup ([test](../server/internal/control/wakeup_tokens_test.go#L12-L79)); HTTP authorization and payload cases are in [`wakeup_registration_test.go`](../server/internal/httpapi/wakeup_registration_test.go#L1-L80). |
| Server FCM boundary | The optional provider sends only a fresh opaque `syncHint`, with routing token separated from message data. It sends no notification text or call/command content ([`fcm.go`](../server/internal/wakeup/fcm.go#L22-L110), [`WAKEUP_PROVIDER.md`](WAKEUP_PROVIDER.md#L8-L30)). Recorded fixture/build evidence does **not** establish live Firebase registration or delivery; the docs explicitly leave those gates open ([`WAKEUP_PROVIDER.md`](WAKEUP_PROVIDER.md#L100-L124)). |
| Call state and ownership | Child call snapshots use authenticated device sessions and increasing server-allocated sequence numbers. The server only replaces a stored snapshot with a higher sequence; the parent read requires the authenticated parent to own the device ([`calls.go`](../server/internal/control/calls.go#L31-L136), [`device_status.go`](../server/internal/httpapi/device_status.go#L9-L79)). Parent call actions also re-check current, recent call state before queueing ([`calls.go`](../server/internal/control/calls.go#L142-L177)). |
| Current parent alerting | The parent currently offers an explicit, ten-minute foreground watch that polls the owner-authenticated call endpoint; it is not continuous push ([`CallWatchService.kt`](../android/parent-app/src/main/kotlin/com/appcontrol/parent/CallWatchService.kt#L17-L110), [`ParentClient.kt`](../android/parent-app/src/main/kotlin/com/appcontrol/parent/ParentClient.kt#L68-L74), [`PARENT_CALL_ALERTS.md`](PARENT_CALL_ALERTS.md#L1-L17)). The watch rejects stale/out-of-order reports and suppresses repeat alerts within its in-memory window ([`CallWatchState.java`](../android/parent-app/src/main/java/com/appcontrol/parent/CallWatchState.java#L9-L47)); `CallWatchChecks` covers ordering, repeat reports, freshness, exact identities, expiry and capacity ([test](../android/parent-app/src/test/java/com/appcontrol/parent/CallWatchChecks.java#L11-L39)). |
| Existing notification privacy | Current local parent alerts use private visibility, generic lock-screen public text, no caller/contact details, immutable navigation-only `PendingIntent`s and a report-age timeout ([`CallWatchService.kt`](../android/parent-app/src/main/kotlin/com/appcontrol/parent/CallWatchService.kt#L111-L127), [`PARENT_CALL_ALERTS.md`](PARENT_CALL_ALERTS.md#L36-L39)). The recorded emulator acceptance is synthetic and does not prove live push or physical call behavior ([`PARENT_CALL_ALERTS.md`](PARENT_CALL_ALERTS.md#L44-L64)). |
| Logout and child revocation | Parent logout stops the local call watch, requests server logout, then clears the local vault even when server revocation cannot be confirmed ([`ParentActivity.kt`](../android/parent-app/src/main/kotlin/com/appcontrol/parent/ParentActivity.kt#L137-L143), [`ParentClient.kt`](../android/parent-app/src/main/kotlin/com/appcontrol/parent/ParentClient.kt#L99-L107)). Server logout revokes the authenticated parent session family ([`auth/service.go`](../server/internal/auth/service.go#L280-L322)). Child revocation deletes device credentials/challenges, and migration 15 removes that child's routing token on state change ([`credentials.go`](../server/internal/pairing/credentials.go#L159-L178), [`0015_wakeup_registration.sql`](../server/internal/storage/migrations/0015_wakeup_registration.sql#L6-L9)). |

## Missing components for parent push

1. **Parent-app token registration and rotation.** The parent app has no Firebase
   dependency, messaging service, or FCM registration flow in its current
   manifest/build configuration ([`parent-app/build.gradle.kts`](../android/parent-app/build.gradle.kts#L15-L68),
   [`AndroidManifest.xml`](../android/parent-app/src/main/AndroidManifest.xml#L1-L29)).
   The child token flow cannot serve as a parent registration: its endpoint and
   registry are deliberately scoped to a child device credential and device ID.
   Add a separate parent-app registration route and registry. Register after
   authenticated login/session restoration, replace the token on `onNewToken`,
   and support explicit removal. A token refresh must not create an unbounded
   set of active registrations.

2. **Authenticated ownership from event to recipient.** Existing parent reads
   correctly enforce device ownership, but no parent-token mapping or push
   fan-out exists. The new registration must derive the account and session
   family from parent authentication—not from request-supplied parent/device
   identifiers. When creating or delivering an alert, resolve the child device's
   owner and active state from the server database, and route only to that
   owner's active parent-app registrations. Re-check ownership/current state
   when the parent fetches the event; an FCM hint is never authorization.

3. **Logout, session-revocation and device-revocation cleanup.** Parent logout
   currently revokes a session and clears only local app state; there is no
   parent push registration to delete. Bind registrations to a server-authenticated
   parent session family (and installation record as needed), and remove them
   transactionally when logout or any session-family revocation occurs,
   including refresh-token reuse and operator recovery. Also prevent queued
   events for a revoked child from being delivered. Client-side `DELETE` before
   logout is useful best effort, but must not be the only cleanup mechanism
   because network failure already permits local sign-out.

4. **Durable duplicate and stale-event handling.** Current snapshot sequencing
   protects the latest call snapshot, and the foreground watch deduplicates only
   in process memory. Neither is a parent-push event queue or a durable
   idempotency ledger. Add an event/outbox identity derived from the accepted
   snapshot sequence, device and call transition; create it in the same
   transaction as the accepted snapshot. Suppress duplicate transitions and
   FCM retries, discard events whose device is no longer active/owned or whose
   call is no longer freshly `RINGING`, and make notification publication
   idempotent across app/process restarts. Do not infer alert success from FCM
   acceptance.

5. **Push-path privacy and current-state validation.** The existing local
   notification text is suitably generic, but there is no parent push payload
   to assess yet. Keep FCM data-only and opaque (a generic sync hint, not a
   call ID, device ID/name, number, contact, state, or action); do not use
   OS-rendered FCM notification payloads for call events. After receiving a hint,
   the parent should authenticate to the backend, fetch current owner-scoped
   event/call state, enforce sequence and freshness, then create a generic
   private local notification. Taps must fetch and verify the exact current call
   again and remain review-only; never issue call actions directly from a push.

## Proposed implementation sequence

1. **Define lifecycle and event contract.** Specify parent registration identity,
   session-family binding, token replacement/removal, event retention/expiry,
   duplicate key, current-call freshness rule and generic notification text.
   Preserve polling as fallback and keep ordinary parent builds Firebase-free
   unless the project intentionally chooses otherwise.
2. **Add backend registration and cleanup.** Add a parent-owned encrypted token
   registry and parent-authenticated `PUT`/`DELETE` routes distinct from the
   child-device route. Derive owner/session scope from `AuthService`; implement
   transactional logout/revocation cleanup and invalid-token removal. Keep
   token values out of logs, errors, audit events and API reads.
3. **Add transactional call-event outbox and recipient resolution.** On an
   accepted, newer call snapshot, persist only qualifying new `RINGING`
   transitions with a stable idempotency key. Resolve eligible active parent
   registrations from database ownership, use bounded retry/backoff, and
   re-check current ownership/state before sending. Reuse the established FCM
   OAuth/HTTP safeguards where appropriate, without reusing the child-token
   table or treating a sent hint as command delivery.
4. **Add the parent Firebase variant and authenticated event sync.** Configure
   the parent Firebase app separately from the child app. Add an FCM service
   that only enqueues constrained background work. The work authenticates,
   fetches the parent event feed/current call state, records consumed event
   identities durably, and posts a local generic/private notification only for
   fresh valid events. Keep exact-call review and action confirmation in the
   existing parent UI flow.
5. **Accept in layers.** Run focused backend and parent JVM tests first, then
   configured Firebase-variant compilation/packaging, then live end-to-end
   delivery and lifecycle checks on a dedicated test parent device. Keep
   polling and explicit refresh functional when FCM is unavailable.

## Meaningful tests and acceptance gates

- **Backend registration:** unauthenticated and device-session credentials
  cannot register parent tokens; account A cannot register or receive for
  account B; duplicate registration replaces the old token; malformed tokens
  fail; ciphertext is not plaintext and cannot be replayed under another
  registration; no read/list endpoint exposes token values.
- **Cleanup:** logout deletes only the relevant session/install registration;
  refresh-family reuse and operator session revocation also clean it up; child
  revocation prevents delivery for that child while leaving unrelated active
  devices alone. Exercise network failure on client `DELETE` to prove server
  cleanup remains authoritative.
- **Event correctness:** accepted increasing snapshots enqueue only a new
  ringing transition; duplicate/equal and stale sequences do not enqueue;
  repeated ringing state, ended calls, old observations, revoked devices and
  cross-owner devices do not produce alerts. Transaction rollback creates no
  outbox event. Duplicate worker execution and FCM retries publish one logical
  notification, and an event becomes ineligible when the current state has
  changed.
- **Parent app:** token registration after login and session refresh, token
  rotation callback, process restart, logout while offline, denied notification
  permission, duplicate hint, stale hint, sign-out/revoked session, and exact
  call no-longer-current review. Assert the push callback itself performs no
  privileged action and does not display unvalidated event data.
- **Privacy:** assert FCM JSON contains only the opaque hint and necessary
  routing metadata; assert no call/device identifiers or personal data appear
  in logs/errors/audit records; inspect private and public local notification
  text and lock-screen presentation; verify notification tap refetches current
  state before showing controls.
- **Live acceptance:** use a dedicated Firebase test project, a least-privilege
  server service account with Cloud Messaging send access, a separate protected
  server routing-encryption key, parent Firebase public app settings for the
  parent package, correctly signed configured APKs, working HTTPS, and a
  dedicated test parent device with notification permission. Test foreground,
  background, process restart, token rotation, logout, revoke, retries and
  stale-call suppression. Emulator/build fixtures alone are not live-delivery
  evidence; Android power management and OEM behavior can still delay or
  suppress delivery.

The connected physical phone is **parent-only** and is not an acceptance
device for this work. Do not reset it, clear its data, provision Device Owner,
or run child, VPN or dialer tests on it. Any hardware acceptance requires a
separate, explicitly designated test device.
