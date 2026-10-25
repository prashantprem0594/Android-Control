# Server ingress protections and review

Reviewed on 23 October 2026 against the local implementation. These protections run by default for every `httpapi.NewHandler`, including tests and the server executable.

## Admission and input bounds

The default process admits 128 concurrent requests and at most two concurrent registration/login operations. Excess work immediately receives `503 server_busy` with `Retry-After: 1`; it does not reach authentication or mutations. Slots release on returns and panics. Two password slots bound concurrent Argon2 work to approximately 128 MiB plus implementation overhead with the current 64 MiB password parameters.

Each admitted request receives a 12-second context deadline. Database work uses that context. Cancellation is cooperative: the deadline does not forcibly interrupt a password hash or code that ignores cancellation. Network read/write/header timeouts remain separate server settings. The global body ceiling is 1 MiB, including chunked bodies; route-specific smaller limits still apply. Targets longer than 8192 bytes receive 414. Non-identity Content-Encoding receives 415. Oversized declared bodies receive the existing 400 invalid_request response before mutation.

Token buckets refill continuously. Both peer and global buckets must admit work; a depleted peer does not consume the global bucket. Rejected requests receive `429 rate_limited` and a rounded-up Retry-After in seconds.

| POST group unless specified | Peer rate/minute, burst | Process rate/minute, burst |
|---|---:|---:|
| Register/login | 10, 5 | 60, 10 |
| Enrollment start/complete | 60, 20 | 240, 60 |
| Device-auth challenge/session | 120, 30 | 600, 120 |
| Refresh/logout | 120, 30 | 600, 120 |
| Other API requests | 300, 120 | 3000, 600 |
| GET/HEAD healthz/readyz | No peer bucket | 600, 30 |

IPv4 peers share a /32 bucket and IPv6 peers share a /64. Only SHA-256 keys of canonical prefixes reside in the limiter, never raw client addresses; these hashes are not anonymous identifiers and are never logged. The default maximum is 4096 peers, with idle entries removed after 15 minutes on a sweep at most once per minute. New peers receive 429 if the table is full; health checks do not allocate peer entries. These are process-local limits, reset on restart. Multiple replicas require an edge-wide limit. NAT clients share a budget. There is no new email/account lockout based on anonymous ingress traffic; existing authentication-level failure tracking remains separate.

## Proxy trust and settings

No proxy headers are trusted by default. Set `APP_CONTROL_TRUSTED_PROXY_CIDRS` only to the actual proxy source networks. The immediate socket peer must be trusted before X-Forwarded-For is examined. The chain is traversed from right to left to the first untrusted hop. Malformed, over-1024-byte or over-16-hop headers fall back to the socket peer. Forwarded and X-Real-IP are ignored. The proxy must append or replace forwarding information correctly and prevent untrusted direct access where that is part of deployment policy.

| Environment variable | Default | Supported bounds |
|---|---:|---|
| APP_CONTROL_REQUEST_TIMEOUT | 12s | Greater than zero, at most 2 minutes |
| APP_CONTROL_MAX_IN_FLIGHT | 128 | 1–1024 |
| APP_CONTROL_MAX_PASSWORD_IN_FLIGHT | 2 | 1–16, no greater than total requests |
| APP_CONTROL_MAX_RATE_PEERS | 4096 | 1–65536 |
| APP_CONTROL_RATE_SCALE | 1 | Integer 1–100, multiplies all rates/bursts |
| APP_CONTROL_TRUSTED_PROXY_CIDRS | Empty | At most 32 specific comma-separated IPv4/IPv6 CIDRs; /0 rejected |

Raising the rate scale does not increase concurrency or body limits. Keep the password concurrency within the host memory budget. The listener still requires an HTTPS deployment; proxy trust does not provide transport encryption.

## Logging and failure review

Request logs contain known HTTP method (or OTHER), registered route template, status, response size, duration, aborted and handler_panic. Dynamic device/command identifiers, unknown raw paths, query strings, addresses, headers, body values, panic values and stacks are excluded. Unknown routes and CONNECT requests use unmatched. A panic before headers returns generic server_error; a panic after a partial response aborts the HTTP transport with http.ErrAbortHandler, avoiding normal completion and Go's default panic-value/stack logging.

The review covered parent/device authorization boundaries, revoked and expired sessions, scoped audit reads, transactional command/policy writes, input limits, admission before service work, bounded limiter state, forwarding spoofing, cancellation and error paths. Existing integration cases exercise cross-parent/device access, revocation, stale versions, failed proofs and rollback. New regression tests in `server/internal/httpapi/protection_test.go` and `server/internal/config/protection_test.go` exercise throttling, refill, peer eviction, concurrent password/global admission, slot release after panic, proxy chains, IPv6 grouping, chunked limits, context propagation and secret-marker scans of captured logs. Wake-up payload privacy tests remain in `server/internal/wakeup`.

Remaining acceptance includes deployed proxy/HTTPS configuration, hostile-load measurements, representative Android/system/proxy log captures, live Firebase payload capture and active socket revocation once a socket transport exists. P6-003/P6-004 remain in progress; this review does not close physical feasibility or production security acceptance.

## Production executable loopback verification

On 24 October 2026, the production-binary recovery rehearsal added optional
ingress checks, run through the logger:

```powershell
& .\scripts\run-logged.ps1 -TaskId P6-003 -Category test -Command '& .\scripts\test-recovery-executables.ps1 -CheckIngress'
```

The isolated server uses rate scale 1, two password slots, 128 request slots and
no trusted proxy CIDRs, with parent
environment settings restored after process launch. A recovered parent logs in;
distinct invalid-account attempts consume the remaining peer password burst.
Further login requests with changing forged `X-Forwarded-For` identities receive
`429 rate_limited` and positive `Retry-After`. Health, readiness and an existing
authenticated parent API request remain available through their separate rate
groups. The rate case is a sequential local burst.

A separate controlled socket check sends two incomplete login uploads with
`Expect: 100-continue`. Receiving the interim response confirms that each
admitted handler is waiting for its body. A third login receives 503 with
Retry-After while health/readiness still succeed. Completing the two uploads
and draining their final responses releases the slots; a new request reaches
authentication rather than receiving server_busy. This verifies bounded
password admission and release in the production executable, not sustained
hostile-load performance or a deployed proxy's slow-client protection.

The rehearsal also requests a synthetic unknown path with a secret query
marker. Captured production server logs exclude that path marker, the query
secret, synthetic forwarding addresses, the parent email and password, and
issued access/refresh tokens. Only fixed result summaries enter the build log.
Evidence is in `scripts/test-recovery-executables.ps1` and
`logs/build/2026-10-24.txt`. Real proxy configuration and live device/system logs
remain outside this local verification.
