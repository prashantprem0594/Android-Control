# Encrypted backup and recovery

`server/cmd/control-admin` provides keygen, backup, verify, restore and integrity operations. Commands return nonzero on failure and print fixed summaries, without key bytes, database contents or supplied paths. The executable prints a generic failure message; it does not dump SQLite errors. Keys are exactly 32 binary bytes, supplied through a file, never a command argument or environment value containing the secret itself.

Build and run through the project logger. From `C:\Android Control`, after setting GOCACHE and GOMODCACHE to the workspace caches:

```powershell
& .\scripts\run-logged.ps1 -TaskId P6-005 -Category build -WorkingDirectory .\server -Command '& "C:\Program Files\Go\bin\go.exe" build -o "..\.cache\control-admin.exe" ./cmd/control-admin'
& .\scripts\run-logged.ps1 -TaskId P6-005 -Category tooling -WorkingDirectory . -Command '& ".\.cache\control-admin.exe" keygen -key-file ".\private-backup-key.bin"'
& .\scripts\run-logged.ps1 -TaskId P6-005 -Category tooling -WorkingDirectory . -Command '& ".\.cache\control-admin.exe" backup -db ".\server\data\app-control.db" -backup ".\backups\snapshot.acb" -key-file ".\private-backup-key.bin"'
& .\scripts\run-logged.ps1 -TaskId P6-005 -Category tooling -WorkingDirectory . -Command '& ".\.cache\control-admin.exe" verify -backup ".\backups\snapshot.acb" -key-file ".\private-backup-key.bin" -scratch-dir ".\recovery\private-scratch"'
& .\scripts\run-logged.ps1 -TaskId P6-005 -Category tooling -WorkingDirectory . -Command '& ".\.cache\control-admin.exe" restore -db ".\recovery\app-control.db" -backup ".\backups\snapshot.acb" -key-file ".\private-backup-key.bin"'
& .\scripts\run-logged.ps1 -TaskId P6-005 -Category tooling -WorkingDirectory . -Command '& ".\.cache\control-admin.exe" integrity -db ".\recovery\app-control.db"'
```

These are operator examples; running keygen against a new path creates a real secret. Keep it outside source control, logs and backup storage. Existing key files are never overwritten. Files are requested with mode 0600; Windows protection follows directory ACLs, so use a private operator directory. The example relative key location must be moved to separately protected storage for deployment. There is no automatic key escrow or key rotation. Losing the key makes its backups unrecoverable.

## Format and publication

Current backups use `ACBKP002`: AES-256-GCM records up to 64 KiB, random nonce prefix, unique counter per record, authenticated magic/chunk-size/counter/plaintext-length metadata and an authenticated final zero-length record. The final tag commits to the record count, preventing truncation at a complete-record boundary. Altered headers, missing final tags, wrong keys, corrupt records and trailing data fail restore. The earlier development `ACBKP001` format is rejected. Create fresh backups from the original database; changing the header does not convert old files. No legacy-format converter is supplied.

Backup uses VACUUM INTO for a consistent snapshot and verifies SQLite integrity before encryption. A private plaintext snapshot exists temporarily in the destination directory; protect that directory and its disk. Normal failure/success paths remove staging files, but abrupt termination can leave staging files behind. Review leftover `.app-control-*` files while the relevant operation is stopped, and apply the host's secure storage/retention policy.

Restore decrypts privately, verifies SQLite integrity, applies current embedded migrations, adds the system database_restored audit event and checkpoints its WAL before publication. Publication uses a same-directory hard link to an entirely written, synced staging file. It refuses an existing destination, including one created during the operation; filesystems without hard-link support fail closed. No overwrite/copy fallback is used. This is process-crash protection; directory metadata durability across sudden power loss still depends on the operating system and filesystem.

Backup and integrity require an existing regular database file and do not run migrations. Restore always requires a new destination. The default operation deadline is two minutes; `-timeout` accepts a positive duration up to one hour. Encryption/decryption checks cancellation between reads/writes, and backup/restore check again before publication. An already-blocked filesystem syscall is not preempted. Context-aware SQLite work observes the deadline. Use trusted local file paths and controlled directories for administration.

`verify` requires backup, key-file and an explicit private scratch-dir; db is rejected. It authenticates the entire encrypted stream, decrypts a temporary snapshot and checks SQLite integrity without migrations, recovery audit insertion or database publication. Normal success/failure paths remove plaintext files. Abrupt process termination may leave scratch files, so protect and review this directory as for other recovery staging files.

## Unattended backup job

Backup, restore and verification reject an already-cancelled context before
creating destination or scratch directories. Restore also checks that the
opened encrypted input is a regular file before creating staging files.
Regression checks cover these early-failure paths alongside stream cancellation.

`scripts/backup-database.ps1` creates a uniquely named snapshot and runs independent encrypted-backup verification before reporting success. It writes dated P6-005 logs internally and returns nonzero for missing files, invalid keys, locking, capacity, backup or verification failure. It accepts explicit database, backup, key and scratch paths plus an optional admin executable path. Use absolute paths when configuring a scheduler.

```powershell
& .\scripts\backup-database.ps1 -DatabasePath 'C:\Android Control\server\data\app-control.db' -BackupDirectory 'D:\PrivateBackups\AppControl' -KeyFile 'C:\PrivateKeys\app-control.key' -ScratchDirectory 'C:\PrivateRecovery\scratch' -MaxBackups 30 -TimeoutMinutes 2
```

This example is not registered or executed automatically. Configure Windows Task Scheduler under the intended operator account, using powershell.exe, -NoProfile, -File and the absolute script path/arguments. Set the scheduler to prevent overlapping instances, monitor nonzero results and missing recent snapshots, and choose a cadence/disk budget appropriate to the host. The script also takes an exclusive file lock in the destination directory, preventing simultaneous jobs from bypassing the capacity check.

The default capacity is 30 snapshots matching the job's exact timestamp/UUID filename pattern; MaxBackups supports 1–3650. At capacity the job fails before creating another snapshot. It never deletes or archives backups. Archive verified snapshots to separate protected storage and review retained files to keep jobs running. A failed verification leaves its encrypted snapshot for inspection and does not report success. Unrecognized filenames and crash-left staging files are outside the count; monitor free space separately. TimeoutMinutes supports 1–60 and applies independently to backup and verification.

Local Windows smoke checks exercised snapshot creation/verification, empty scratch cleanup, literal special-character paths, capacity refusal, exclusive-lock refusal and invalid-key failures. A scheduler service run under the deployment account and off-host archival remain unverified.

## Recovery exercise

### Local rehearsal using production executables

`scripts/test-recovery-executables.ps1` runs an isolated rehearsal using the
built `control-server.exe` and `control-admin.exe`, rather than test server
binaries. Build both through the logger with workspace-local Go caches:

```powershell
& .\scripts\run-logged.ps1 -TaskId P6-005 -Category build -WorkingDirectory server -Command '& "C:\Program Files\Go\bin\go.exe" build -o "..\.cache\control-server.exe" ./cmd/control-server; if ($LASTEXITCODE -ne 0) { throw "Server build failed" }; & "C:\Program Files\Go\bin\go.exe" build -o "..\.cache\control-admin.exe" ./cmd/control-admin'
& .\scripts\run-logged.ps1 -TaskId P6-005 -Category test -Command '& .\scripts\test-recovery-executables.ps1'
```

The script starts hidden server processes on ephemeral loopback ports with
databases in a unique `.cache/recovery-executables-*` directory. It creates a
synthetic parent with a random password, backs up the live WAL database,
verifies the backup and empty scratch directory, stops the source, retires its
database filename, restores to a new database and checks readiness and recovered
parent identity through login and authenticated HTTP. It verifies existing
destination refusal and unchanged backup/key hashes. Captured server logs are
checked for the fixture email, password and issued access/refresh tokens.
Owned processes are stopped and the disposable backup key is deleted in cleanup;
synthetic databases, encrypted snapshot and sanitized server logs remain in the
ignored fixture directory for inspection. No production database or account is
used. Optional executable-path parameters allow testing another locally built
binary pair.

Use `-CheckIngress` to include the sequential login-rate and forwarding-header
checks described in [server protections](SERVER_PROTECTION.md). The isolated
server uses rate scale 1 and no trusted proxies; caller environment values are
restored immediately after launch. Both modes also check that synthetic unknown
paths/query markers and forwarding addresses do not appear in captured logs.

The rehearsal passed on 24 October 2026; command output is recorded in
`logs/build/2026-10-24.txt`. It uses local loopback HTTP with no proxy. Deployed
TLS, scheduler identity, off-host storage, clean-host restore and actual Android
device reconnection remain separate acceptance requirements.

### Deployed recovery

1. Restore to a new directory while keeping the source backup/key unchanged. Verify integrity and recovered account/device/audit data without printing sensitive rows to build logs.
2. Stop the server before switching its database configuration to the recovered path. Keep the original database and any WAL/SHM files together; do not overwrite a running database.
3. Restart against the recovered path through the logger. Verify readiness, parent login, device proof and policy/queue state on an authorized test device.
4. Review the recovery audit event and any state created since the snapshot. A restore rolls back revocations, sessions, policies, acknowledgements and audit history to the snapshot; rotate/revoke credentials and reconcile uncertain commands before reconnecting devices if that rollback creates risk. The child command journal reduces duplicate execution but does not replace recovery reconciliation.

Automated storage/CLI tests prove round-trip contents, audit insertion, wrong-key refusal, non-publication on failure, destination/key preservation, stream completeness and private output using temporary fixtures. A clean-host restore with real device reconnection and server certificate rotation remain unverified; P6-005 and production recovery acceptance remain open.

On 24 October 2026, `server/internal/httpapi/recovery_test.go` added a local
HTTPS reconnection exercise. It enrolls a software P-256 device identity, stores
a desired policy and an idempotent queued command, takes an encrypted backup,
closes the source database and restores into a separate directory. Fresh service
instances then accept parent password login and signed device authentication
with the original key. The test verifies recovered policy/queue state, delivery
without premature success, acknowledgement, request idempotency, one recovery
audit event and rejection of the session after device revocation. Full Go tests
and go vet pass; outputs are in `logs/build/2026-10-24.txt`. This does not prove
Android Keystore, platform certificate pinning or clean-host operations.

The exercise now serves the restored database from a separate test-server
process (`server/internal/httpapi/recovery_process_test.go`). Its HTTPS client
trusts only that process's public test certificate and retains hostname
verification. Readiness, isolation between two real parent accounts, policy
reporting and owner-authorized revocation are checked through HTTP. The child
process is terminated and reaped during cleanup. This harness uses the real
HTTP handler and services; it does not exercise the deployed ingress proxy,
production server entry point or Android trust configuration.

`server/cmd/control-admin/process_recovery_test.go` additionally launches
separate processes invoking the production administration entry point. It
backs up committed data while the source is open in WAL mode, independently
verifies the backup, closes and removes the temporary source database, then
restores into another temporary directory. It checks wrong-key refusal,
destination preservation, recovered contents, a single recovery audit,
unchanged backup/key bytes and private command output. This exercises process
boundaries on the current host; it is not a clean-host recovery result.

Windows task export: scripts/export-backup-task.ps1 now prepares a reviewable S4U/least-privilege local-backup task without registering or running it. See docs/BACKUP_SCHEDULING.md. XML/launcher parsing passed; actual scheduled identity, ACLs, off-host archival and production restore remain open.
