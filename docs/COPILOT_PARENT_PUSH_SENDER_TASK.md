# Copilot task: bounded parent push sender and worker

Implement the backend sender/worker on top of the completed parentpush foundation in C:\Android Control. Read AGENTS.md if present, PARENT_PUSH_CONTRACT.md, PARENT_PUSH_IMPLEMENTATION.md and the existing child FCM provider/tests. Preserve all uncommitted work.

## Coordination

Another agent owns Android, acceptance scripts, shared tracking/handoff and release packaging. Own server files plus a new docs/COPILOT_PARENT_PUSH_SENDER.md only. Do not edit Android, scripts, the shared tracker/handoff or existing task documents.

Read and design immediately, but wait to edit shared server files until .cache/android-server-https-reports/5d8fd31417cc409c934c68772c29f776.json has a terminal status (passed or failed) and cleanup is complete. This short wait prevents changing server source during the currently running integration suite. If that report is missing or remains running, report the coordination dependency rather than assuming it finished. Do not stop its processes.

## Deliver

1. Bound all work in parentpush.Store.Claim. It currently caps returned deliveries but scans every active event and eligible family first. Add bounded database candidate/fan-out processing with deterministic progress and meaningful tests proving no starvation across batches. Also provide bounded expiry/retention cleanup, including when event sending is disabled; stale active events must not accumulate indefinitely. Preserve exact-call deduplication and document its retention horizon.
2. Add a parent-specific FCM sender abstraction and bounded worker driven by the existing outbox. Reuse established OAuth/HTTP safeguards without routing through the child-token table or changing child behavior. Keep parent service-account configuration separate; default worker operation is disabled. Parent registration may remain enabled independently of the sender.
3. Send only fresh random opaque syncHint data. No FCM notification payload, call/device identifiers, names, numbers, contacts, state or actions. Bound message TTL by remaining eligibility. A send acceptance is transport acknowledgement, never proof that the parent displayed an alert.
4. Revalidate the lease, active ownership/family/registration and current ringing freshness immediately before sending. Keep network work outside database transactions. Bound each network attempt by remaining event/lease time, then use Finish for completion/retry. Late or superseded lease results must not corrupt a new lease or delete a rotated/re-registered token. Only explicit provider UNREGISTERED classification may trigger conditional registration deletion; generic HTTP 400/401/403 must not be treated as invalid token.
5. Add finite polling/batch/concurrency limits, context cancellation, graceful shutdown and no per-token unbounded goroutines. Clear decrypted token buffers when finished. Never log credentials, routing tokens, Firebase response bodies or call/device identifiers.
6. Wire opt-in startup/configuration and document private credential/key requirements, defaults, shutdown behavior, retry/expiry/retention semantics and remaining acceptance in docs/COPILOT_PARENT_PUSH_SENDER.md. Do not implement the parent event feed or Android Firebase integration in this task.

## Verification

Use only local deterministic fixtures with httptest OAuth/FCM endpoints and generated in-memory credentials. Test bounded multi-batch progress, stale/ended/revoked/cross-owner calls, expired families, rotation/unregister during a send, duplicate/crashed/expired leases, explicit UNREGISTERED versus other errors, 429/5xx retries, network stalls/deadlines, cancellation/shutdown, malformed provider responses and payload/log privacy. Preserve existing child FCM tests.

Run focused tests, the full Go suite and go vet through scripts/run-logged.ps1 with clear TaskIds. Do not run Gradle, ADB, release packaging, real Firebase requests, deployment or physical-device tests. Do not commit. Report changed files, test results and limitations; live push remains unverified without parent-client integration and external acceptance.
