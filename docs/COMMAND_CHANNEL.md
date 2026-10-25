# Command channel verification

The channel supports `LOCK_DEVICE` and short-lived `ANSWER_CALL`, `REJECT_CALL`,
and `END_CALL` actions targeting a specific call ID. Commands are committed to
SQLite before any fetch. Parent creation can use an `Idempotency-Key` header containing
16–128 ASCII letters, digits, hyphens, or underscores. Retry the same key and TTL
to retrieve the original command, including its terminal or expired state.
Changing the TTL with the same key returns 409. Keys are scoped to parent and
device; they are not audit payloads. Without a key, every POST creates a command.

## Device API

Use the device bearer session obtained through the signed authentication flow.
Every operation rechecks the hashed session, expiry and active device state
inside its database transaction. Device IDs come from authentication rather
than caller-supplied query parameters. Parent bearer tokens cannot fetch device
commands. Deploy behind HTTPS; the child requires HTTPS and platform certificate
validation and refuses redirects.

- `GET /v1/device/commands` returns `{"commands":[...]}`, with at most 50 pending
  command envelopes plus `status` metadata. Fetch marks queued commands delivered.
  A delivered command is returned again until a terminal result or expiry.
- `POST /v1/device/commands/{commandID}/ack` accepts
  `{"result":"succeeded"}` or `{"result":"failed"}` and returns 204. Only the
  authenticated target device can acknowledge a delivered command. Identical
  terminal results are idempotent. Conflicting results, undelivered commands and
  expired commands return 409. Another device's command returns 404.
- `GET /v1/device/policy` returns only the latest desired policy snapshot. No
  intermediate offline versions are replayed. This is not an applied-state claim.
  Policies with schedules or always-allowed apps require `X-Policy-Format: 2`;
  older clients receive an explicit unsupported-format error.

Parent command inspection distinguishes queued, delivered, succeeded, failed,
and expired. Delivery attempts never imply success, even if writing the HTTP
response fails. Expiry is evaluated at fetch, acknowledgement, idempotent create
retry, and owner inspection; there is no background expiry timer yet. Deadlines
are parsed as instants, including fractional seconds. Terminal state cannot be
reopened. Status transitions and their audit events commit together.

## Child feasibility flow

After enrollment, press **Sync parent commands** in the child app. This explicit
foreground worker authenticates with AndroidKeyStore, fetches commands, executes
supported lock and queued call actions, sends results, and downloads the latest
desired policy into app-private storage. A persisted background job
also provides a polling fallback, subject to Android scheduling. FCM wakeups
and sockets remain unconfigured.

Startup/boot/update/clock-change scheduling enqueues network-constrained sync
work as well as maintaining the periodic jobs. Every enqueue uses the same
JobInfo, so a request arriving during a run is queued without replacing it.
Work items carry only a fixed synchronization action, never credentials,
commands or policies. Up to 50 waiting requests share one full sync; requests
arriving during that pass or its completion are handled by a subsequent batch.
Items are completed only after success. Partial/transient or transport failures
retain unfinished work and request backoff; cancellation leaves work unfinished.
The command journal still prevents repeated physical command actions.

Successful queue exhaustion is left to `dequeueWork()` rather than calling
`jobFinished(false)`, which can discard a concurrently arriving work item. See
the [JobParameters work lifecycle](https://developer.android.com/reference/android/app/job/JobParameters)
and [enqueue guidance](https://developer.android.com/reference/android/app/job/JobScheduler).
Scheduler entry points are serialized within the process and enqueue failure
raises an error to the caller. Android controls execution timing, quotas and
work retention. These work items are not persisted across reboot; boot requests
a fresh sync and the periodic jobs remain persisted. Enrolled-device behavior,
process death and supported Android/HyperOS coverage still require acceptance.

The Android 37 emulator fixture now verifies enqueue during an active pass,
uncompleted work redelivery after backoff, and cancellation without completion.
See [emulator test evidence](EMULATOR_TESTS.md). It uses a debug-only JobService
and the production queue coordinator, not the enrolled network sync service;
other Android/HyperOS versions and the full failure matrix remain open.

The child validates command identity, device, version, type, payload, and
deadline. Call commands contain only a canonical `callId`; their lifetime is at
most 30 seconds. The Telecom gate requires an exact live identity and a valid
state. The child acknowledges success only after observing the requested state
on that same call; an accepted Telecom request without an observed result is
not success. A synchronous private journal reserves each command before the
action and saves its result before acknowledgement. Duplicate delivery reuses
the result. An interrupted reservation becomes failed/unconfirmed without
repeating the action. This avoids duplicate locks at the cost of possible
unexecuted or unconfirmed commands after a crash. No exactly-once physical
execution guarantee is claimed. Expired commands are skipped; journal entries
are retained until one day after expiry. Client clock accuracy remains part of
device hardening. Tokens stay in memory and are not logged.

The parent foreground **Foreground calls** screen uses a separate authorized
snapshot API. Telecom callbacks trigger coalesced lightweight `POST
/v1/device/calls` reports containing bounded `callId`/state/permission pairs;
the child refreshes active snapshots every 15 seconds without running policy
or inventory sync. The parent reads through `GET /v1/devices/{deviceID}/calls`.
Server-issued monotonic sequences reject delayed older reports. Queue creation
rejects disabled, absent, stale, unauthorized, wrong-call, or invalid-state
targets before persisting a command. The child still verifies the exact live
call at execution. Snapshots are not a continuous stream. Incoming-call
notification behavior is not part of this workflow.

The child rejects version regression, reconciles cached app restrictions and
the internet-pause prototype, and submits a separate versioned result. Desired
state alone never implies success. See [device policy verification](DEVICE_POLICIES.md).

Command and policy stages now recover independently after shared authentication.
A failed command fetch, journal operation or acknowledgement still allows the
latest desired-policy snapshot to be downloaded, cached, applied and reported
if that endpoint remains reachable. The original command-stage failure is
retained after policy work, so foreground sync reports incomplete work and the
background job requests a retry. Cancellation stops further stage work. If both
stages fail, the policy failure retains the command failure as a suppressed
exception; neither path reports complete success. Authentication failure still
prevents authenticated network operations; the background job separately attempts
cached policy reconciliation before authentication.

Enrollment/authentication and sync HTTPS requests now use `RequestControl` with
a 30-second elapsed-time budget in addition to their existing 15-second connect
and read timeouts. A per-request daemon watcher checks cancellation and elapsed
time and requests connection disconnection; request code also checks the budget
before and after blocking operations. Every normal exit closes the watcher.
Authentication receives the job cancellation callback and checks it before
starting the signed-proof request. Tokens and request contents are not inspected
or logged by this control.

Disconnection is a best-effort interruption of platform networking, not a hard
real-time guarantee for every DNS, socket or Keystore operation. JVM and
Android 37 instrumentation checks cover the cancellation/deadline controller,
normal close cleanup and a stalled TLS handshake. The full enrolled-device
network, process-death and supported Redmi matrix remains open.

`StalledTransportChecks` now also runs a real JVM `HttpsURLConnection` against
a loopback socket that accepts TCP but never completes TLS. It exercises both
the request deadline and explicit cancellation, confirms a disconnection attempt
and unsuccessful request outcome, and checks fixture worker cleanup. Finite
connect/read timeouts remain enabled as a fallback; this test does not assert
instantaneous interruption or successful Android socket cancellation. The peer
provides no certificate or application response, and the client uses default
TLS trust without credentials or a verification bypass. Run it with the other
command journal checks through `scripts/test-command-journal.ps1`. The same
checks now also pass on Android 37 through `test-android-request-control.ps1`;
see [emulator evidence](EMULATOR_TESTS.md) for their scope and limitations.

## Evidence

- `server/internal/httpapi/durability_test.go`: child server process accepts an
  HTTP command, is forcibly killed without closing SQLite, then restarts against
  the same database. Command and audit history remain intact.
- `server/internal/httpapi/delivery_http_test.go`: enrollment and signed device
  authentication followed by fetch, acknowledgement, duplicate acknowledgement,
  parent-state checks and immediate rejection after revocation.
- `server/internal/httpapi/transport_tls_test.go`: HTTPS requests with validated
  test certificates, latest policy download, interrupted response, redelivery,
  and success only after an authenticated result.
- `server/internal/control/delivery_test.go`: target-device isolation, expiry,
  retries, session expiry/replacement, state immutability and audit rollback.
- `server/internal/control/service_test.go`: concurrent creation deduplication,
  TTL conflicts and preservation of original command identity after expiry.
- `scripts/test-command-journal.ps1`: dependency-free JVM checks of duplicate
  execution, reconstructed executor, interrupted action, journal failures,
  deadline expiry, and action failure. Run through `run-logged.ps1` as a test.
  It also exercises the production `SyncStages` coordinator with a lost ACK,
  a reconstructed executor reusing its saved result, independent policy work,
  command-fetch outage, combined failures and cancellation. These are JVM
  control-flow checks, not an Android network/SharedPreferences failure matrix.
  Queue checks also cover requests during sync/completion, retained partial or
  failed work, cancellation and bounded batches using the production queue
  coordinator with a simulated platform queue.

Real AndroidKeyStore enrollment, physical lock behavior, SharedPreferences
durability under force-stop/reboot, and Redmi device reconciliation still require
device evidence. The [FCM adapter and replaceable wake-up boundary](WAKEUP_PROVIDER.md)
are implemented and tested; project setup and live wake-up integration remain pending.

Android 37 instrumentation now also verifies the production executor with
synchronous SharedPreferences commits across real remote-process death. It
checks saved success reuse, reservation interruption and an action whose result
was not saved, with no repeated fixture action. See
[the process-death probe](EMULATOR_TESTS.md). Its journal is isolated from
production state; the physical action and end-to-end transport failure matrix
remain open.
