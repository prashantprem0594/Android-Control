# Parent push implementation contract

Status, 29 October 2026: this contract is implemented by the server registration/feed/outbox/sender/worker and opt-in parent Android Firebase/job path. Local verification passes; live Firebase/device delivery remains unaccepted. See [implementation status](PARENT_PUSH_IMPLEMENTATION.md). Existing child wakeups and ten-minute parent watch remain separate.

## Registration and authorization

Use separate parent-authenticated `PUT` and `DELETE /v1/parent/wakeup-registration` routes. PUT accepts only `registrationToken` (printable ASCII, 1–4096 bytes); DELETE has no registration identifier supplied by the caller. Derive both parent account and session family from the bearer access token. A child device credential is never accepted. No token read/list route is exposed.

Keep one registration per parent session family. Authenticated refresh retains that family and therefore its registration; token rotation replaces its ciphertext. A new login has a new family and registers independently. Logout, refresh-token reuse and operator recovery delete revoked-family registrations transactionally through database cleanup. Expired families are excluded at lookup even before retention cleanup. An offline local logout clears the local session and all alert jobs/notifications; server credentials and registration may remain valid until revocation or expiry, so no alert worker may use a stale local session.

Encrypt routing tokens with AES-256-GCM under a separately configured parent routing key. Bind a versioned parent-push domain, account ID and family ID as unambiguous authenticated data. Generate a fresh nonce on every replacement; never log token/ciphertext/key contents or put them in audit metadata. Invalid or tampered ciphertext fails closed. Recheck family/account identity, expiry and revocation inside the registration transaction to cover concurrent logout. Token values never authorize account access.

## Call events and sending

Persist a stable event identity in the same transaction that accepts a newer child call snapshot. Only first entry of an exact call into RINGING qualifies; repeated ringing reports and stale/equal sequences do not create another logical alert. The identity includes device and call identity, and keeps the accepted snapshot sequence for ordering. No event is created if the snapshot transaction rolls back.

Before sending, recheck active child ownership, active parent session family and the latest call snapshot. The exact call must still be RINGING and its observation must be at most 30 seconds old with no negative age. Expire events at that freshness boundary. Use bounded leased outbox work, finite network deadlines and retries; FCM acceptance is transport acknowledgement only. Invalid-registration cleanup must compare the attempted registration version so an old send result cannot delete a freshly rotated token.

Reuse the opaque data-only `syncHint` format. No call/device ID, name, number, contact, call state or action appears in Firebase message data or notification payloads. A hint carries no authorization. Recipient lookup uses database ownership, never an owner supplied by a child report or parent request.

## Parent application

Keep polling builds Firebase-free and add a separately configured parent Firebase variant. The FCM callback only schedules bounded authenticated background work. A signed-out app ignores hints. Work retrieves an owner-scoped event feed and current call state, validates sequence/freshness including local elapsed request time, and durably deduplicates notifications across process restart. Store no caller/contact detail in the deduplication ledger. Bound retained identities and expire them.

Post private generic notifications with generic public text, finite timeout and immutable navigation-only PendingIntents. Taps refetch the exact current call. They never directly answer/reject/end a call. Denied notification permission leaves explicit refresh and foreground watch usable. Logout/revocation cancels pending work and posted alerts, and prevents an already-running worker from publishing after its session generation changes.

## Required verification

Prove cross-account and child-token refusal; encrypted account/family binding; rotation; refresh continuity; logout/reuse/operator recovery cleanup; concurrent revocation; no token readback; invalid-token response versus concurrent rotation; transaction rollback; stale/equal/duplicate call sequences; ring/end/ring transitions; account/device revocation during retry; finite retry budgets; opaque Firebase payloads; durable app deduplication; offline logout races; denied permission; and exact-call review after the call ends.

Backend and configured-build checks can run locally. Live Firebase, signed production configuration, real device/background/OEM delivery and physical calls require separate acceptance evidence. Parent Redmi 14 5G is excluded from child provisioning/policy fixtures; child is Redmi Note 5 Pro. No credentials, deployment or hardware tests are implied by this contract.
