# Security audit events (P1-006)

The server stores security history in SQLite through migration `0005_audit_events.sql`. Events are written in the transaction that changes the protected state. Audit insertion failures abort that change. Rolling back a transaction also rolls back its audit entries.

Covered actions:

- Parent registration, successful login, persisted login failures, refresh rotation, and session-family revocation (logout or refresh reuse).
- Pairing-code issuance, enrollment start, failed enrollment proofs, successful enrollment, and device revocation (parent action or proof-failure limit).
- Successful device authentication and well-formed authentication proofs that fail verification.
- Durable lock-command creation and versioned desired-policy changes.
- Command delivery/result/expiry transitions, child policy results, and changed app inventory snapshots.
- Successful database recovery, recorded as an operator-only system event before publication.

This is a state-change audit, not a record of every incoming request. Malformed requests, unknown/expired credentials, throttled logins, and denied cross-parent lookups do not currently create events. Existing request logs retain method, path, status, and timing without bodies or query values.

Events contain an increasing ID, UTC occurrence timestamp, parent scope, actor type and opaque identifier, a fixed action, opaque target ID, and a fixed reason when applicable. Failed proofs use an anonymous actor because the caller has not proved device identity. Session revocations are attributed to the server with a reason. Enrollment proof failures use the database UTC clock; other event timestamps follow the associated state change. Inventory labels, package lists and policy contents are excluded from report/inventory audit records.

No passwords, emails, device names, phone numbers, tokens or their hashes, pairing codes, public/private keys, signatures, challenges, command bodies, or policy content are stored in audit events. Unknown-account login failures have no parent scope and are visible only through direct operator database inspection.

## Read API

`GET /v1/audit-events?after=0&limit=50` requires a parent bearer token. Results are ascending by event ID, restricted to the authenticated parent. The limit is 1–100 and defaults to 50. To continue, pass the last returned event ID as `after`. An empty page returns `{"events":[]}`. Query parameters cannot select another parent. Responses use `Cache-Control: no-store`.

There are no API operations to create arbitrary events, update, or delete history. SQLite triggers reject UPDATE, DELETE, and replacement of an existing audit ID, including INSERT OR REPLACE when recursive triggers are disabled. History has no cascading foreign keys and remains when an account or device is removed. Encrypted backups preserve the audit table and its protection triggers.

These controls prevent mutation through the running application and ordinary SQL data changes. They do not make SQLite tamper-proof against an administrator who can edit the database file or drop its schema protections. There is no external signed audit checkpoint yet. Existing databases are not backfilled with invented historical events; recording begins when migration 0005 is applied.

## Verification

Automated tests exercise authentication, enrollment, revocation, failed proofs, parent isolation, pagination, sensitive-value exclusion, update/delete/replace rejection, transaction rollback, injected audit-write failures, database reopen, and encrypted backup/restore with preserved history and immutability. Server builds and checks are recorded under task P1-006 in the dated build log.

Migration 0007 adds durable lock-command creation and versioned desired-policy updates, with `command_created` and `policy_updated` events. Service and HTTP tests exercise both flows and inspect parent-scoped history, including rollback when audit storage fails. See [control intent routes](CONTROL_INTENTS.md). Delivery, acknowledgement, execution and reconciliation remain later command-channel work and must add audit coverage when introduced.

Database recovery now records `database_restored` through migration 0006. The record is written into the private restored database and its WAL is checkpointed before publication. Audit failure aborts publication. This system event has no parent scope, actor ID, target ID, backup path, or key material; operators can inspect it directly in SQLite. Parent history remains unchanged. Restoring older backups applies current migrations before recording recovery. Tests in `server/internal/storage/recovery_audit_test.go` verify durable recovery records, failure handling, and migration preservation of existing event IDs, producer triggers, and immutability. Other recovery workflows, such as account recovery, remain future work.

23 October 2026 result: P1-006 acceptance is satisfied for the implemented enrollment, login, command creation, policy update, revocation, and database recovery flows. The combined lifecycle test preserves their history across restart and encrypted restore. Full Go tests and go vet passed, and the server binary was built at .cache/bin/control-server-p1-006.exe. No Android source changed in P1-006. Future security-sensitive flows must extend this audit contract.
