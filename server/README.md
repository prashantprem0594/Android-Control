# Control server

The server uses Go 1.27.3 with pinned, locally compiled libraries. It does not require a hosted authentication or database service.

For matched server and recovery-tool binaries, use the offline
[server bundle builder](../docs/SERVER_BUNDLES.md).

## Run locally

From `C:\App Control\server`:

```powershell
$env:GOCACHE = 'C:\App Control\.cache\go-build'
$env:GOMODCACHE = 'C:\App Control\.cache\go-mod'
go run ./cmd/control-server
```

The default listener is `:8080`.

## Configuration

| Environment variable | Default | Purpose |
|---|---:|---|
| `APP_CONTROL_HTTP_ADDRESS` | `:8080` | TCP listen address |
| `APP_CONTROL_DATABASE_PATH` | `data/app-control.db` | SQLite database file |
| `APP_CONTROL_REMOTE_CALLS_ENABLED` | `false` | Allow foreground answer/reject/end commands |
| `APP_CONTROL_READ_HEADER_TIMEOUT` | `5s` | Request-header timeout |
| `APP_CONTROL_READ_TIMEOUT` | `15s` | Full request read timeout |
| `APP_CONTROL_WRITE_TIMEOUT` | `15s` | Response write timeout |
| `APP_CONTROL_IDLE_TIMEOUT` | `60s` | Keep-alive idle timeout |
| `APP_CONTROL_SHUTDOWN_TIMEOUT` | `10s` | Maximum graceful drain time |
| `APP_CONTROL_MAX_HEADER_BYTES` | `1048576` | Maximum request-header size |
| `APP_CONTROL_REQUEST_TIMEOUT` | `12s` | Cooperative request context deadline |
| `APP_CONTROL_MAX_IN_FLIGHT` | `128` | Concurrent admitted requests |
| `APP_CONTROL_MAX_PASSWORD_IN_FLIGHT` | `2` | Concurrent registration/login operations |
| `APP_CONTROL_MAX_RATE_PEERS` | `4096` | Bounded peer bucket table |
| `APP_CONTROL_RATE_SCALE` | `1` | Per-process rate/burst multiplier |
| `APP_CONTROL_TRUSTED_PROXY_CIDRS` | Empty | Explicit forwarding proxy trust |

Invalid addresses, durations, zero or negative durations, and non-positive header limits stop startup with a structured error.
Set `APP_CONTROL_REMOTE_CALLS_ENABLED=true` only for a controlled rollout; the
child build must also opt in with `-PappControlRemoteCallsEnabled=true`.

## Operational endpoints

- `GET /healthz` reports that the process is alive.
- `GET /readyz` reports whether required dependencies are ready.

Responses use JSON, disable caching, and set `X-Content-Type-Options: nosniff`. Request logs record known methods and registered route templates, omitting dynamic identifiers, unknown raw paths, query strings, credentials, bodies and client addresses. Default admission limits, forwarding rules and reviewed limitations are in [server protections](../docs/SERVER_PROTECTION.md).

`SIGINT` and `SIGTERM` stop accepting new connections and allow active requests to complete within the configured shutdown timeout.

## Parent authentication

The versioned JSON API exposes:

- `POST /v1/auth/register` with `email` and `password`.
- `POST /v1/auth/login` with `email` and `password`.
- `POST /v1/auth/refresh` with `refreshToken`.
- `POST /v1/auth/logout` with an access-token Bearer header.
- `GET /v1/auth/me` with an access-token Bearer header.

<<<<<<< HEAD
Passwords must contain 12 to 1024 bytes and are stored as self-describing Argon2id hashes using 64 MiB of memory, three iterations, and a random 16-byte salt. Access tokens expire after 15 minutes. Parent refresh tokens last 10 years by default and renew on each successful refresh, so normal use keeps the parent signed in until sign-out. Server-side revocation, lost local app data, or an expired refresh token still requires sign-in.
=======
Passwords must contain 12 to 1024 bytes and are stored as self-describing Argon2id hashes using 64 MiB of memory, three iterations, and a random 16-byte salt. Access tokens expire after 15 minutes. Refresh-token families expire after 30 days and do not slide beyond their original expiry.
>>>>>>> origin/main

Access and refresh tokens are random opaque values. SQLite stores only their SHA-256 hashes. A refresh succeeds once and rotates the token; reuse of a consumed refresh token revokes every access and refresh token in that family. Logout performs the same family-wide revocation. Failed logins are tracked in SQLite and block an email for 15 minutes after five failures within a 15-minute window.

Authentication request bodies are limited to 16 KiB. Logs contain the method, route path, status, size, and duration but omit query values, bodies, email addresses, passwords, and tokens. Deploy the server behind HTTPS before using it outside a local development machine.

## SQLite and backups

Startup opens the local SQLite file with foreign keys, full synchronous writes, a five-second busy timeout, WAL mode, and one bounded connection. Embedded migrations run transactionally and startup stops if `PRAGMA integrity_check` fails.

Backups use a consistent `VACUUM INTO` snapshot and verify its integrity before encryption. ACBKP002 uses AES-256-GCM with authenticated record metadata and a final count tag. Restore authenticates the complete stream, verifies SQLite integrity, records recovery and publishes without replacing existing files. Earlier ACBKP001 development backups are rejected.

`cmd/control-admin` implements keygen, backup, verify, restore and integrity with exact 32-byte binary key files. `scripts/backup-database.ps1` runs logged backup/verification with a capacity gate and an exclusive lock. See [database recovery](../docs/DATABASE_RECOVERY.md) for commands, filesystem requirements, private staging files and recovery reconciliation. Keys must be stored separately from backups and never written to logs.

`control-admin tls-pin -certificate <single-leaf.pem>` derives a public Base64 SHA-256 SPKI pin without reading a private key. Android deployment properties and overlapping certificate rotation are documented in [server trust](../docs/SERVER_TRUST.md).

Pairing endpoints and device enrollment protocol: see [docs/PAIRING.md](../docs/PAIRING.md).

Device authentication and revocation: see [docs/DEVICE_CREDENTIALS.md](../docs/DEVICE_CREDENTIALS.md).

Security audit history and its parent-scoped read API: see [docs/AUDIT_EVENTS.md](../docs/AUDIT_EVENTS.md).

Audited lock-command queueing and desired-policy routes: see [docs/CONTROL_INTENTS.md](../docs/CONTROL_INTENTS.md).

Device command fetch, acknowledgement, expiry, retries, and child sync: see [docs/COMMAND_CHANNEL.md](../docs/COMMAND_CHANNEL.md). Background polling, inventory, policy results and device-control prototypes are described in [docs/DEVICE_POLICIES.md](../docs/DEVICE_POLICIES.md). Physical acceptance and live FCM integration remain pending.
