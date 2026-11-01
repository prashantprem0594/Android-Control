# Parent push implementation

**Status, 29 October 2026:** registration, call-event feed/outbox, sender/worker and parent Android Firebase/job integration are implemented and locally tested. Firebase carries only an opaque wake-up hint plus required transport routing/priority/lifetime metadata. Our HTTPS server owns authentication, accounts, enrollment, call state, policies, commands and audits. No live Firebase delivery, production deployment or physical incoming-call acceptance is claimed.

Sender hardening: registration removal requires HTTP404 with an explicitly typed FCM `UNREGISTERED` response. Transient/authentication/redirect errors retain the registration, including responses with inconsistent `UNREGISTERED` bodies. Regression tests and focused parent-push/control-server checks pass (`PARENT-WAKE-BOUNDARY`, logs/build/2026-10-29.txt). This matches the [FCM error-code contract](https://firebase.google.com/docs/cloud-messaging/error-codes).

## Implemented scope

- `PUT` and `DELETE /v1/parent/wakeup-registration` are enabled only when the
  separate parent routing-key file is configured. Both require a valid parent
  bearer access token. `PUT` accepts only `registrationToken`; the account and
  session-family identity come from server authentication. Child device
  credentials, caller-supplied owner IDs and token-read endpoints are not used.
- A single encrypted registration is stored per authenticated session family.
  Refresh retains the existing family/registration; replacing a token advances
  its registration version. A separate family-scoped version counter also
  advances across unregister/re-register cycles, so a late invalid-token result
  cannot match and delete a replacement registration. Unregister cancels active
  delivery leases. Session-family expiry is checked transactionally during
  registration and again when events are resolved.
- AES-256-GCM ciphertext uses a fresh nonce and binds a versioned parent-push
  domain, account ID and family ID as length-delimited associated data. The
  parent registry has a key separate from the existing child FCM routing-token
  key. Raw tokens are not returned from the API or included in audit metadata,
  request logs or application errors.
- Accepted increasing child call snapshots insert at most one event for each
  exact `(device, call)` identity in the same database transaction. Older and
  equal snapshots are not applied. Only a transition into `RINGING` creates an
  event; repeated ringing snapshots and a later ring report for the same call
  identity do not create another. A newer non-ringing snapshot expires an
  outstanding event and pending leases.
- The internal `parentpush.Store.Claim` resolver leases bounded batches and
  rechecks the latest snapshot sequence/state/freshness, active device
  ownership, parent session-family ownership/revocation/expiry and registration
  ciphertext before returning a token to backend code. Leases last 20 seconds,
  have at most five attempts and use bounded 1/2/4/8-second retry delays.
  Invalid-token cleanup is conditional on the attempted registration version,
  so an old send result cannot delete a rotated token. Expired events are
  eligible for retention cleanup after seven days.
- Session-family revocation triggers remove registrations and cancel active
  deliveries in the same transaction. This covers logout, refresh-token reuse
  and operator password recovery. Device revocation or owner change expires
  its active events and pending deliveries transactionally.

The event outbox and leasing API are internal Go components; there is no
delivery worker, Firebase HTTP sender, owner-scoped event-feed endpoint, or
parent-client event synchronization in this change. A lease claim is not proof
that a notification was delivered. The existing child FCM sender does not send
to this parent registry.

## Configuration

Set `APP_CONTROL_PARENT_FCM_TOKEN_KEY_FILE` to a private file containing exactly
32 cryptographically random binary bytes. The setting is independent of
`APP_CONTROL_FCM_SERVICE_ACCOUNT_FILE` and `APP_CONTROL_FCM_TOKEN_KEY_FILE`;
parent registration can be enabled without enabling the child sender. An absent
parent key disables the parent registration routes. A missing, unreadable or
wrong-sized configured key prevents server startup with a fixed, non-secret
error.

For example, use the existing `control-admin keygen -key-file <new-file>`
workflow to create a new key file. Keep this parent key outside source control,
logs, database backups and the child-token key location; protect it with
operator-only filesystem access. Back it up separately and retain it through
database restore. Losing or replacing it makes existing parent registrations
undecryptable; users must register again. Key rotation is an operational
migration/re-registration, not automatic.

No Firebase service-account credential is loaded for parent routing in this
implementation. The future sender will need a separately configured,
least-privilege Firebase Messaging credential and bounded HTTP handling.
Neither that credential nor live Firebase access was used for these changes.

## Database migration

`server/internal/storage/migrations/0020_parent_push_outbox.sql` is applied
automatically by the existing `storage.Open` migration runner. It adds:

- `parent_wakeup_registrations`, keyed by session family, with encrypted token,
  account binding and rotation version;
- `parent_wakeup_registration_versions`, retaining a monotonic version across
  explicit unregister/re-register cycles until the family is deleted;
- `parent_call_events`, uniquely keyed by device and exact call identity, with
  snapshot sequence and a 30-second eligibility deadline;
- `parent_call_event_deliveries`, keyed per event and session family, with
  bounded attempts and lease state;
- transactional triggers that clean up on family revocation and invalidate
  events on device revocation or ownership change.

Existing child registrations in migration 15 are unchanged and are not copied
or reused. Existing call snapshots are not replayed into the outbox during
migration; only subsequently accepted snapshots can create parent events.
Migration rollback preserves the existing transactional migration behavior.

## Focused verification

Tests cover authenticated parent routes and rejection of child credentials or
extra owner fields; ciphertext opacity, associated-data binding and tampering;
token rotation and stale-version invalidation; refresh continuity; logout,
refresh-reuse and operator-recovery cleanup; concurrent registration/revocation;
expired families; exact-call event deduplication; stale/equal snapshots; current
call freshness and ownership checks; retry leases; and transaction rollback
between snapshot persistence and outbox creation. These are local fixture tests,
not Firebase or device-delivery evidence.

## Remaining delivery and acceptance work

1. Supply the real HTTPS trust and separate Firebase application settings and server-only sender credential/routing-key paths; build the matching Firebase-enabled clients.
2. Run the sender on the intended always-on server. Hosting it on a laptop requires that laptop to remain available; hosting it elsewhere does not require the development laptop to stay on.
3. Exercise real registration/rotation, revoked/expired sessions, duplicate/stale hints, notification denial, screen-off/Doze/OEM behavior, reboot, offline recovery and measured delivery latency. Use [screen-off acceptance](SCREEN_OFF_ACCEPTANCE.md).
4. Complete physical normal-call, policy, production signing/deployment, reliability and supervised-pilot gates separately.

No live push is claimed. Parent is Redmi 14 5G; child is Redmi Note 5 Pro. Keep child Device Owner/policy/VPN/dialer fixtures off the parent. Local build fixtures are never deployed or treated as real Firebase settings.
