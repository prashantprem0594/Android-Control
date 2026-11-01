# Copilot task: parent push backend foundation

Implement the backend foundation for continuous parent incoming-call push in C:\Android Control. Read AGENTS.md if present, COPILOT_PARENT_PUSH_REVIEW.md and PARENT_PUSH_CONTRACT.md first. Preserve all existing uncommitted work.

Deliver:

1. Separate parent-authenticated PUT/DELETE /v1/parent/wakeup-registration routes. Derive account/session family from authentication; never accept caller-supplied owner IDs or child credentials.
2. An encrypted parent routing-token registry with rotation, family expiry checks, transactional revocation cleanup and a separately configured parent encryption key. No token readback or token values in logs, errors or audit metadata.
3. A durable call-event outbox created transactionally with accepted snapshots. Deduplicate exact-call ringing alerts; reject stale/equal snapshots; recheck ownership, revocation and current ringing freshness when resolving eligible events.
4. Focused tests for authorization, ciphertext binding/tampering, rotation, refresh continuity, logout/reuse/operator recovery, concurrent revocation, rollback, duplicate/stale calls and expiry.
5. Configuration, migration behavior, implemented scope and remaining delivery work in docs/COPILOT_PARENT_PUSH_IMPLEMENTATION.md.

Own server code and that new implementation document only. Another agent owns Android code, acceptance scripts, the shared tracker and handoff; do not modify those. Keep schema changes forward-compatible and preserve existing child wakeup behavior.

Use scripts/run-logged.ps1 for tests with a clear TaskId. Run focused Go tests, then the full Go suite and go vet. Do not run Gradle, ADB, release packaging, Firebase requests, deployments or physical-device tests. Do not commit or overwrite unrelated changes.

Complete this backend foundation. Android Firebase delivery and the external FCM sender are separate follow-up tasks; do not claim live push works. Report changed files, test results and remaining limitations. The connected phone is parent-only and excluded from these fixtures.
