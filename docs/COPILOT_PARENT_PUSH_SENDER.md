# Parent push sender and worker

## Implemented scope

The parent outbox now has a bounded polling worker and a parent-specific FCM
sender. The worker processes at most ten deliveries per poll, sequentially, and
uses cancellable contexts with finite send deadlines. It revalidates the active
lease, device owner/state, session family, registration version, exact latest
call snapshot and ringing freshness immediately before each send. Database work
finishes before the network request begins. A send acceptance marks transport
acknowledgement only; it is not evidence that a parent device displayed an
alert.

The only application data sent to FCM is a newly generated 128-bit random
`syncHint`. The payload has no notification block, caller or device details,
call identity/state, names, numbers, contacts or actions. FCM Android message
TTL is rounded down to whole seconds and bounded by the remaining call freshness,
event expiry and lease expiry. HTTP response bodies are parsed only in memory,
never logged, and bounded to 16 KiB. Only a provider `FcmError` with the
explicit `UNREGISTERED` error code triggers conditional removal; generic 400,
401, 403, 429, 5xx, malformed responses and network failures remain retryable.

`Store.Claim` processes at most 20 event candidates and 50 parent session
families per fan-out batch, and returns at most 100 leases. Persistent ordered
cursors make progress across calls and wrap to earlier IDs after reaching the
end. The database maintenance loop runs even when sending is disabled. Each
maintenance transaction expires at most 500 stale events, expires at most 500
associated deliveries, removes at most 500 retained deliveries, and deletes
at most 500 events with no remaining deliveries.

Exact `(device, call)` event deduplication is retained for the lifetime of the
event row. Expired event rows and associated delivery history are retained for
seven days after their eligibility deadline and then pruned in bounded batches.
The event ID remains unique for the lifetime of its row; after the retention
horizon has elapsed and the row is pruned, a later report with a reused call ID
can create a new event. The bounded maintenance loop runs on a five-second
interval and stops during graceful server shutdown.

## Configuration and credentials

Parent sending is disabled by default. To opt in, configure all three private
files/settings:

- `APP_CONTROL_PARENT_FCM_SENDER_ENABLED=true`
- `APP_CONTROL_PARENT_FCM_SERVICE_ACCOUNT_FILE=<private parent-only service-account JSON>`
- `APP_CONTROL_PARENT_FCM_TOKEN_KEY_FILE=<private 32-byte AES key file>`

`APP_CONTROL_PARENT_FCM_SERVICE_ACCOUNT_FILE` must contain a valid Google
service-account credential with the Firebase Messaging scope for the parent
Firebase project. Store it outside source control with operator-only filesystem
permissions; do not include it in images, backups, support bundles or logs.
The credential is separately loaded from the child sender's
`APP_CONTROL_FCM_SERVICE_ACCOUNT_FILE`. The existing parent token-key setting
is also required when enabling the sender; that key protects parent routing
tokens independently of child token encryption. Parent registration may be
enabled with its token key while sending remains disabled.

OAuth token acquisition, bounded HTTP behavior and redirect protection reuse
the existing server wakeup provider implementation; FCM message formatting and
invalid-token classification are parent-specific. No external endpoint override
is accepted. Shutdown cancels polling and active HTTP requests, attempts to
return each still-current lease to bounded retry state, then waits for worker
and maintenance goroutines to exit.

## Database behavior

Migrations `0020_parent_push_outbox.sql` and
`0021_parent_push_processing.sql` create the encrypted registration registry,
event/delivery outbox, monotonic registration version, and persistent bounded
event/fan-out cursors. Existing rows are not backfilled into notifications.
The existing child sender and child token registry are unchanged.

Leases expire after 20 seconds, and a delivery has at most five attempts with
bounded retry delays. Expired/crashed leases consume an attempt; a crashed fifth
lease becomes terminal on the next claim. Events are eligible only while the
current exact call is ringing and fresh (30-second maximum). Logout, token
rotation/unregister, device revocation/ownership change and expired families
are rechecked or transactionally cleaned up. Old lease completions are guarded
by lease ID and expiry; stale invalid-token responses are guarded by the
monotonic registration version, including unregister/re-register cycles.

## Local verification and remaining acceptance

Tests use generated in-memory service-account keys and local `httptest` OAuth
and FCM endpoints. They cover opaque payload shape and hint freshness, TTL,
explicit `UNREGISTERED` versus other provider errors, 429/5xx/malformed
responses, network deadlines, cancellation, retry state, rotation during a
send, bounded event and family fan-out progress, bounded maintenance,
retention, and existing ownership/freshness/revocation and transaction cases.
No production Firebase request or credential was used.

This implementation does not add the Android Firebase client, notification
display, or live-delivery acceptance. The owner-scoped
`GET /v1/parent/call-events` route is outside this change and was not modified.
The parent app must register and securely handle tokens, process sync hints
through authenticated current-state sync, and enforce notification
privacy/lifecycle rules before any alert can be delivered or displayed. Live
push remains unverified until that integration is configured and accepted on
authorized test hardware.
