# Parent Firebase alerts and authenticated synchronization

Screen-off delivery is an explicit acceptance requirement. See [parent screen-off acceptance](SCREEN_OFF_ACCEPTANCE.md) for supported build/setup checks and pending physical scenarios. The parent UI now exposes background-alert readiness; a successful build does not establish real delivery.

The optional parent Firebase variant adds authenticated incoming-call event synchronization. It is separate from the child Firebase wakeup variant and the ten-minute foreground call watch. Live Firebase delivery and real phone/OEM background behavior remain unverified.

## Build and setup

Ordinary parent debug/release APKs use the no-Firebase provider and contain no Firebase SDK. Opt in with appControlParentFirebase=true and the separate public ApplicationId, ApiKey, ProjectId and SenderId properties prefixed appControlParentFirebase. The variants are fcmDebug/fcmRelease. Parent and child Firebase application IDs must differ. The public configuration exporter accepts optional parentFirebase independently of firebase; each app selects its own variant.

Firebase messaging auto-init remains disabled. Application startup initializes the explicit public project configuration but schedules no job and requests no token. A foreground notification-permission button is provided after sign-in; there is no automatic permission prompt from login or background callbacks. Use Refresh incoming-call alerts for an explicit sync. Android power/network restrictions can delay or prevent delivery.

The backend requires the parent routing-key configuration to enable registration and the event feed. That key and any sender service account remain server-only. A structurally valid public export does not prove project registration, server trust, signing or deployment.

## Message and authentication boundary

ParentFirebaseMessagingService accepts only the canonical opaque syncHint format. Notification payloads, call data, extra fields and malformed nonces are rejected. Token rotation and dropped-message callbacks and accepted hints enqueue generic work; callbacks perform no application network requests or notification publication.

The worker first fetches GET /v1/parent/call-events through ParentClient. It validates/restores the authenticated parent session before requesting the SDK token. An eligible worker then obtains a bounded token and uses authenticated PUT /v1/parent/wakeup-registration. Routing tokens are not copied into sync preferences; the Firebase SDK manages its own token persistence. The sync preferences keep only a token digest and registration timestamp. A null SDK token is transient unavailability, not an instruction to unregister. Backend family revocation is authoritative cleanup; a DELETE API remains available.

The feed derives account and session-family identity from the bearer token and checks family validity within its database transaction. It returns at most 100 newest fresh active-owner ringing events. Each event contains only eventId, deviceId, callId, snapshotSequence, observedAt and expiresAt. Latest snapshot sequence/observation are coherent; expiry never extends the original first-ring deadline. Caller names, numbers, contacts and routing tokens are absent. Requests cannot supply an alternate account or device owner. Responses use Cache-Control: no-store.

## Publication and lifecycle

The worker refetches current owner-authenticated call state before publication. Canonical exact-call identity, known state, increasing sequence, server freshness and local elapsed request time are checked. A stale/ended/revoked call cannot become a new alert from an old hint. Notifications are generic/private with generic public lock-screen text, a finite timeout and an immutable navigation-only intent. Tapping fetches the exact current call in the existing review screen. Push never directly answers, rejects or ends a call.

The job has a 60-second budget, 8-second request deadlines, bounded queue batches and at most five delivery attempts per queued hint. Jobs use network constraints and finite backoff. Late queued hints survive the successful queue completion path. Polling/manual refresh and foreground watch remain available independently.

The durable deduplication ledger holds SHA-256 event identities and timestamps only, with a 512-entry cap and seven-day retention. Capacity exhaustion suppresses new alerts rather than evicting unexpired deduplication identities. Commit occurs before publication: process death in that interval may lose one alert but cannot replay it after restart. A new authenticated session resets its push lifecycle state.

Logout invalidates publication before waiting for an in-flight authenticated request, cancels the owned job and removes owned alerts. Permission, session and generation checks guard token retrieval, registration and publication. Network failures during logout cannot preserve local authentication. A revoked/expired session clears the vault and blocks further work. Notification permission/settings denial leaves explicit call-state refresh available.

## Verification scope

The inherited production JobService drainer also passes guarded Android 17 framework acceptance through a debug-only subclass with credential-free I/O substitution. Late dirty writes require a second pass in one start; retained-item retry requires a second framework start, followed by cancellation and generation-reset checks. The final run reports `retry_mode=automatic`, queue success and cleanup success (`PARENT-QUEUE-RUNTIME` in `logs/build/2026-10-27.txt`). A shell-force fallback exists for scheduling diagnostics but was not used in that passing run; an earlier natural retry timeout remains failed evidence. The test refuses existing sessions, jobs and push notifications and restores its preference baseline. It does not exercise Firebase provider admission, real HTTP/notification publication, process death or OEM behavior.

Local opaque-hint and state checks pass: canonical nonce-only payloads, notification/mixed-data refusal, exact identity/state/sequence/freshness, elapsed budget, rollback, durable deduplication and capacity. Focused Go event-feed tests pass for owner/family authorization, revocation/expiry, ended/stale/reordered calls, bounds/order and response/log privacy. The final five-mode, ten-APK release matrix passed full lint, compiled authority and APK separation checks in `.cache/configured-releases-5b51979d783e4baea6c13f86134e2a0c/configured-builds.json`. Local gateway HTTPS parent push registration/feed/lifecycle API acceptance passed in report `c78ab8b94fa5412c8e26994536dee064`, with cleanup passed. Commands and probe markers are recorded in `logs/build/2026-10-27.txt`.

These are local implementation/build checks. They do not prove live Firebase registration/delivery, actual background JobScheduler publication on an OEM phone, token rotation against a real project, physical incoming calls, production signing or family-pilot acceptance. Configured FCM fixture APKs are not launched; no real Firebase requests or physical-device tests are part of these checks. User-confirmed roles are Redmi 14 5G parent and Redmi Note 5 Pro child; keep child policy/provisioning fixtures off the parent phone.
