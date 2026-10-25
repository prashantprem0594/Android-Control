# App Control: Android Studio handoff

## Resume verification: 24 October 2026

Android FCM integration continuation: optional Firebase source directories now
use AGP built-in Kotlin source-set wiring, fixing the polling child compile.
Debug and androidTest APKs build, release Kotlin sources compile with synthetic
trust pins, opt-in FCM variant configuration validates, and opaque-hint checks
pass. Real Firebase project configuration and live device delivery remain
unverified. See docs/WAKEUP_PROVIDER.md and
logs/build/2026-10-24.txt; P0-010 and P2-002 remain in progress.

Optional Firebase variant build continuation: Google Maven access was available
and both `assembleFcmDebug` and `assembleFcmRelease` passed, producing APKs with
the Firebase application and messaging service. The build used fixture Gradle
properties because no actual test-project configuration was available; no
Firebase request or device delivery was performed. Live-delivery gates remain
open. The daily-log verifier separately reports a pre-existing hash mismatch in
the P7-002 entry started at 19:46:07; the entry was not modified. See
docs/COMPLETED_WORK.md and logs/build/2026-10-24.txt.

Production policy reboot milestone: cached policy/ledger were prepared on the
isolated Device Owner AVD, the fixture was deliberately unsuspended, and an
actual emulator reboot triggered production re-enforcement. A newer boot-count
marker plus applied result is verified before instrumentation starts; DPM and
ledger confirm suspension. Post-reboot allowlist restoration and baseline
cleanup pass. PolicyEnforcer's boot-count field is local diagnostic metadata.
See docs/EMULATOR_TESTS.md and test-android-policy-reboot.ps1. Physical Redmi and
enrolled server/acknowledgement recovery remain open.

Production package-policy runtime milestone: the isolated AppControlAcceptance
AVD is now Device Owner, with no factory reset or physical-device action.
policy-fixture is a disposable permission-free APK. The real PolicyEnforcer,
ledger and DPM adapter pass suspension, same-policy reapplication, allowlist
restoration, management-target parser rejection and preference/OS cleanup.
The initial probe expectation was corrected to match early parser rejection;
product behavior was unchanged. Device Owner remains enabled on the AVD.
See docs/EMULATOR_TESTS.md; Redmi launch/reboot acceptance remains open.

Android journal runtime continuation: the production executor now has actual
process-death evidence using a private SharedPreferences fixture in the separate
debug-only :journal_probe process. Persisted success is reused, interrupted
reservation fails with zero actions, and an action interrupted before result
save fails/unconfirms with one action and no repeat. Binder death and changed
worker PIDs are checked; no lock/policy/network/call operation occurs. Probe and
child app/test builds pass. Run test-android-journal-process.ps1 via the isolated
ADB server. Physical and end-to-end acknowledgement recovery remain open.

Android transport runtime continuation: RequestControlChecks and the real
silent-peer TLS test now run in Android instrumentation and pass on the
Android 37 AVD. Tests verify cancellation/deadline handling, disconnect/close
behavior and fixture-worker cleanup, retaining finite socket timeouts. No
credentials or trust overrides are used. Run test-android-request-control.ps1
against ADB server 5038/serial 127.0.0.1:5581. Full enrolled/mobile/Redmi recovery
acceptance remains open; see docs/EMULATOR_TESTS.md.

Android queue runtime continuation: a debug-only JobService fixture using the
production queue coordinator passed late enqueue without restart, unfinished
work redelivery after backoff and explicit cancellation without completion on
the Android 37 AVD. Test jobs are cleaned up; no policy/network/call actions are
used. The first failed probe lost preceding output because of a logger bug;
run-logged now captures output incrementally before exceptions and regression
checks pass. Later Android probe results pass; historical failure is retained.
See docs/EMULATOR_TESTS.md. Full enrolled-device failure matrix remains open.

Android runtime milestone: an isolated Android 37 Google Play AVD is now
bootable in workspace .tooling with WHPX. Use ADB server 5038 and serial
127.0.0.1:5581, not the default daemon. Production Keystore identity probes pass
generation, API non-exportability, enrollment/authentication signing and negative
challenge/domain checks. The public-key fingerprint remains identical across
separate instrumentation invocations and an actual emulator reboot. See
docs/EMULATOR_TESTS.md for launch/wait/install/probe commands and the test
profile's skipped ADB dialog limitation. P1-004/P1-005 remain in progress pending
actual Android HTTPS enrollment/authentication against the server. Redmi matrix
and server rotation remain open. The AVD is available for further test work.

Queued sync follow-up: on-demand requests now use fixed-JobInfo enqueue with
opaque work items instead of skipping a request when a job already exists.
The worker batches up to 50, completes only after success and dequeues again
to process requests arriving during sync/completion. Healthy queue exhaustion
uses Android's dequeue lifecycle without jobFinished(false); failures retain
unfinished items for backoff. JVM queue tests cover late requests, partial and
failed runs, cancellation and batching. Work-item platform behavior remains
unverified on Android; reboot still requests fresh sync from server state.

Transient policy recovery continuation: partial package_change_failed results
now request retry from network/offline jobs; foreground sync requests a
background follow-up. Offline jobs explicitly use exponential backoff. A later
protected-role exclusion can no longer hide an earlier package change failure.
JVM checks cover retained ledger state, retry convergence and avoiding retry
loops for protected targets. Actual Android retry timing remains open.

Production admission continuation: `-CheckIngress` now holds both password
slots with real incomplete Expect/Continue uploads, verifies a third login gets
503 plus Retry-After while health stays available, completes the uploads and
verifies slot release. The profile explicitly sets two password slots and
128 request slots, restoring caller environment after launch. The complete
production recovery/ingress/privacy rehearsal passes.

P6-003/P6-004 executable evidence: recovery rehearsal now supports
`-CheckIngress`. Sequential invalid logins with changing forged forwarding
headers remain peer-rate-limited with Retry-After; health/readiness and an
authenticated parent request still succeed. Captured production logs exclude
credentials, issued tokens, unknown-path/query markers and forwarding addresses.
The isolated server explicitly uses rate scale 1 and no trusted proxies. This
does not close concurrent-load, real proxy or Android/system-log acceptance.

Stalled transport evidence: the JVM journal-check script now tests a real
HTTPS client connecting to a silent loopback TCP peer. Deadline and explicit
cancellation cases attempt disconnection, fail closed with finite socket
timeouts available, and clean up fixture workers. All journal/stage/request/
transport JVM checks pass. This is not Android network interruption evidence.

Sync scheduling continuation: startup, boot, update and clock changes now
request a network-constrained one-shot sync alongside periodic fallback jobs.
Repeated requests preserve an already scheduled job and its backoff rather
than replacing it. Scheduler entry points are synchronized and one-shot
scheduling failures are checked. Actual Android job timing remains unverified.

Child request cancellation continuation: enrollment/authentication and sync
requests now have a 30-second elapsed budget and a cancellation watcher that
requests connection disconnection. Authentication receives job cancellation
and checks it before signed-proof submission. JVM tests cover cancellation,
deadline, single disconnect, normal close cleanup and disconnect failures.
Actual Android stalled-network interruption remains unverified; platform I/O
disconnection is best effort, not a hard real-time guarantee.

P6-001 recovery continuation: child command fetch/journal/ack failure no longer
prevents the independent policy stage after successful authentication. The
command failure is retained for retry and sync never reports full success on
that path. JVM tests verify policy progress during command failure, journal
deduplication after lost ACK, combined failures and cancellation. P6-001 is now
in progress; actual Android outage/reboot/network testing remains open.

Certificate preparation continuation: control-admin now supports
`tls-preflight -hostname ... -certificate ... -backup-certificate ...`.
It checks current/backup leaf-first chains against host system trust, DNS name,
current validity and server usage, rejects reused keys and emits public pin/
expiry JSON only after validation. The shared certificate reader also rejects
malformed PEM skipped before a valid certificate, including for tls-pin.
See `docs/SERVER_TRUST.md`; host preflight is not Android rotation acceptance.

Recovery early-failure hardening: already-cancelled backup/restore operations
now return before creating directories; restore rejects a non-regular opened
backup input before staging. Regression tests, full Go tests and go vet pass.
Both production binaries were rebuilt for the executable rehearsal.

Production executable rehearsal now passes via
`scripts/test-recovery-executables.ps1`. It uses isolated loopback server
processes, synthetic parent registration, live WAL backup/verify, source
retirement, restore, integrity, readiness and recovered authenticated identity.
It verifies destination refusal, backup/key preservation and absence of the
fixture credentials in captured server logs. Disposable keys are removed and
owned processes are stopped. See `docs/DATABASE_RECOVERY.md` for logged use.
This remains local evidence; clean-host and Android deployment gates stay open.

Process recovery continuation: the recovered HTTPS server now runs in a
separate test process. Readiness, real parent login/isolation, original device
key, policy reporting and HTTP revocation pass. Separate administration
processes exercise the production CLI entry point for keygen, WAL backup,
verification, wrong-key refusal, restore without the source database,
integrity and existing-destination refusal. Backup/key preservation and output
privacy pass. Full Go tests, focused final reruns and go vet pass. Clean-host
deployment and actual Android rotation remain open.

Android trust continuation: both modules now have separate instrumentation
test APKs containing a credential-free platform TLS probe bound to the compiled
origin. The logged physical-device runner validates instrumentation results;
fixture tests cover failed/missing runner and nonzero ADB outcomes. Packaging
checks verify the runner target and ensure the probe is absent from app APKs.
See `docs/SERVER_TRUST_ACCEPTANCE.md` for use. No live handshake result is claimed.

P6-005 continuation adds `server/internal/httpapi/recovery_test.go`: local
encrypted restore followed by HTTPS parent login, original device-key proof,
policy/queue reads, acknowledgement, idempotency and revocation. Full Go tests
and go vet pass. The tracker records this evidence and remains in progress;
clean-host recovery and real Android TLS/rotation are still pending.

Resumed at the server-trust checkpoint. Full Go tests, go vet and JVM
server-origin checks passed again; ADB still reports no attached devices.
Added `docs/SERVER_TRUST_ACCEPTANCE.md` for device rotation, certificate failure
and trusted-update evidence. P6-005 remains in progress. Commands and outputs
are in `logs/build/2026-10-24.txt`. The first daily log verification required
the synthetic redaction probe; after recording that probe, integrity and
redaction verification passed.

This document is the complete handoff for moving day-to-day development from VS Code to Android Studio. The project does **not** need to be converted or rearranged. Keep the complete repository at `C:\App Control` and open only `C:\App Control\android` as the Android Studio project. The Go server, documentation, contracts, tracker, scripts, and build logs remain in the parent directory and can be used from Android Studio's Terminal window.

Last updated: 23 October 2026 (IST).

## Continuation update: 23 October 2026

The project has been copied into `C:\Android Control`, which is now the active workspace. Paths below referencing `C:\App Control` describe the earlier location.

P1-004 is now in progress. Pairing service tests, full Go tests, go vet, and HTTP integration tests pass. Pairing routes are wired into the server. The child now includes an AndroidKeyStore P-256 identity, HTTPS enrollment client, private enrollment metadata, and an enrollment form. See `docs/PAIRING.md` for the protocol and remaining verification. No ADB devices or configured emulator AVDs were found. P1-004 remains unpassed until actual Keystore proof evidence is recorded. Build command outcomes are recorded in the dated log.

## 1. Product scope and fixed decisions

App Control is a privately distributed parental-control system for fully managed Redmi phones running Android 13, Android 14, and later.

Required controls:

- Lock the managed child device.
- Pause internet access while preserving the management connection.
- Block and restore selected applications.
- Handle cellular calls, including parent-authorized answer, reject, end-call, and speaker actions.

Deployment decisions:

- The child phone may be factory-reset and enrolled with the child APK as Android Device Owner.
- Parent and child APKs are privately sideloaded; Google Play distribution is not required.
- iOS is outside the current scope.
- Parent and child apps use native Kotlin.
- The backend is our own Go application with a local SQLite database.
- Firebase Authentication, Firestore, Realtime Database, Storage, Hosting, and Supabase are prohibited.
- Free Firebase Cloud Messaging may later be used only for opaque wake-up hints. Commands, policies, credentials, call data, and audit records remain on our server.
- Microsoft Azure is not required. Planning and progress are stored in repository Markdown/JSON files.
- Libraries compiled into our code are allowed. Hosted service dependency and unexpected service pricing are the concern.

The main architecture documents are:

- `docs/ARCHITECTURE.md`
- `docs/IMPLEMENTATION_PLAN.md`
- `docs/PHASE_TRACKER.md`
- `docs/DEPENDENCIES.md`
- `contracts/command-envelope.schema.json`

## 2. Repository layout

```text
C:\App Control\
├── android\                 Android Studio/Gradle project
│   ├── parent-app\          Parent Android application
│   ├── child-app\           Device Owner child application
│   ├── gradle\wrapper\      Pinned Gradle wrapper
│   ├── build.gradle.kts
│   ├── settings.gradle.kts
│   └── local.properties     Local SDK path; ignored by Git
├── server\                  Self-hosted Go control server
├── contracts\               Shared versioned protocol contracts
├── docs\                    Architecture, plan, policies, dependencies
├── tracking\                Machine-readable phase tracker
├── scripts\                 Logged-command and progress scripts
├── logs\build\              Detailed dated command/output text logs
├── .cache\                  Disposable local build caches and server binary
└── ANDROID_STUDIO_HANDOFF.md This file
```

Do not move only the `android` directory to a separate location. The sibling server, tracker, documentation, scripts, and logs are part of the same product and are required for a complete handoff.

## 3. Installed and pinned toolchain

| Component | Version/location |
|---|---|
| Android Studio | Rabbit 1, 2026.2.1 |
| JDK | 17 |
| Go | 1.27.3 at `C:\Program Files\Go\bin\go.exe` |
| Android SDK | `C:\Users\Sushant\AppData\Local\Android\Sdk` |
| Compile/target SDK | API 37 |
| Minimum SDK | API 33 |
| Android SDK Build Tools | 36.0.0 |
| ADB / Platform-Tools | 37.0.1 |
| Android emulator | 37.2.12 |
| Installed emulator image | Android 37 Google Play x86_64 |
| Android Gradle Plugin | 9.4.0 |
| Gradle wrapper | 9.6.0 |
| Optional global Gradle | `C:\Gradle\gradle-9.6.0` |

The checked-in Gradle wrapper is canonical. Android Studio should use the wrapper defined in `android\gradle\wrapper\gradle-wrapper.properties`; the global Gradle installation is not required for normal builds.

`android\local.properties` currently contains:

```properties
sdk.dir=C\:\\Users\\Sushant\\AppData\\Local\\Android\\Sdk
```

The exact verified toolchain is also recorded in `docs/TOOLCHAIN.md`.

## 4. Open the project in Android Studio

1. Close any project currently open in Android Studio.
2. Select **File > Open**.
3. Choose `C:\App Control\android`.
4. Allow Android Studio to trust the project.
5. Confirm that Gradle uses the wrapper and JDK 17 in **Settings > Build, Execution, Deployment > Build Tools > Gradle**.
6. Let Gradle sync complete.
7. Use the **Project** view to confirm that `parent-app` and `child-app` are present.

If Android Studio asks for an SDK, select `C:\Users\Sushant\AppData\Local\Android\Sdk`. If it rewrites `local.properties`, that is expected; the file is machine-local and ignored.

Android Studio may create `.idea`, `.gradle`, `.kotlin`, `build`, and `*.iml` files. These are ignored and should not be treated as product source.

## 5. Android modules already implemented

### Parent app

- Application ID: `com.appcontrol.parent`
- Native Kotlin Android application.
- Minimum API 33; target and compile API 37.
- A minimal baseline activity exists and the debug APK builds.
- Full login, enrollment, dashboard, controls, and call-action UI are future phases.

### Child app

- Application ID: `com.appcontrol.child`
- Device Admin receiver: `com.appcontrol.child.admin.AppControlDeviceAdminReceiver`
- Native Kotlin Android application.
- Minimum API 33; target and compile API 37.
- Declares `android.permission.INTERNET`.
- Disables application backup and cleartext traffic.
- Declares a Device Admin receiver protected by `android.permission.BIND_DEVICE_ADMIN`.
- The current activity reports whether the package is Device Owner.
- The current feasibility button invokes `DevicePolicyManager.lockNow()` only when Device Owner is active.

Verified debug APKs:

```text
C:\App Control\android\parent-app\build\outputs\apk\debug\parent-app-debug.apk
C:\App Control\android\child-app\build\outputs\apk\debug\child-app-debug.apk
```

Both APKs previously passed package, SDK, signature, and manifest inspection.

## 6. Build from Android Studio or its terminal

Android Studio build actions can build the selected module. For reproducible command-line verification, run these from `C:\App Control\android`:

```powershell
.\gradlew.bat :parent-app:assembleDebug :child-app:assembleDebug
```

Useful module-specific commands:

```powershell
.\gradlew.bat :parent-app:assembleDebug
.\gradlew.bat :child-app:assembleDebug
```

Suggested Android Studio Gradle run configurations:

| Name | Gradle task |
|---|---|
| Build both debug APKs | `:parent-app:assembleDebug :child-app:assembleDebug` |
| Build parent debug APK | `:parent-app:assembleDebug` |
| Build child debug APK | `:child-app:assembleDebug` |

The command wrapper in section 10 must be used for formal verification so the output enters the project log.

## 7. Device Owner provisioning and physical-device tests

The Redmi must be factory-reset, have no user accounts configured, and be at the initial setup state required by Android Device Policy Manager. Never automate or trigger a factory reset from project scripts.

From `C:\App Control\android`:

```powershell
adb devices -l
adb install -r ".\child-app\build\outputs\apk\debug\child-app-debug.apk"
adb shell dpm set-device-owner "com.appcontrol.child/com.appcontrol.child.admin.AppControlDeviceAdminReceiver"
adb shell dpm list-owners
```

Then launch the child app and confirm that it shows `Device Owner: active`. The lock feasibility button should lock the phone and require strong authentication.

No physical device was connected during the latest recorded check. Therefore Device Owner, application suspension, VPN filtering, dialer, SIM-call, speaker-routing, FCM, and emergency-path checks remain unverified on Redmi hardware.

## 8. Go control server already implemented

The server lives at `C:\App Control\server` and remains part of the same project even though Android Studio opens the `android` subdirectory.

Implemented secure foundation:

- Validated environment configuration.
- Structured JSON request logging that omits query values and bodies.
- `GET /healthz` and dependency-aware `GET /readyz`.
- Graceful shutdown that drains active requests.
- Local SQLite database through `modernc.org/sqlite` 1.61.0.
- Foreign keys, WAL mode, full synchronous writes, busy timeout, and bounded connections.
- Embedded ordered migrations with transaction rollback and idempotence checks.
- SQLite integrity checks.
- Consistent `VACUUM INTO` snapshots.
- Chunked AES-256-GCM encrypted backup and authenticated restore.
- Argon2id parent password hashes through `golang.org/x/crypto` 0.58.0.
- Random opaque access and refresh tokens stored only as SHA-256 hashes.
- 15-minute access tokens and fixed 30-day refresh-token families.
- Database-backed login throttling.
- Refresh rotation, reuse detection, family-wide revocation, and logout revocation.
- Bounded strict JSON authentication endpoints.

Implemented authentication routes:

```text
POST /v1/auth/register
POST /v1/auth/login
POST /v1/auth/refresh
POST /v1/auth/logout
GET  /v1/auth/me
```

Default server address: `:8080`.

Default database: `server\data\app-control.db` relative to the server working directory.

Run server tests from the Android Studio Terminal:

```powershell
cd "C:\App Control\server"
$env:GOCACHE = 'C:\App Control\.cache\go-build'
$env:GOMODCACHE = 'C:\App Control\.cache\go-mod'
& 'C:\Program Files\Go\bin\go.exe' test ./... -count=1
& 'C:\Program Files\Go\bin\go.exe' vet ./...
```

Run the server:

```powershell
cd "C:\App Control\server"
$env:GOCACHE = 'C:\App Control\.cache\go-build'
$env:GOMODCACHE = 'C:\App Control\.cache\go-mod'
& 'C:\Program Files\Go\bin\go.exe' run ./cmd/control-server
```

Production binary most recently built at:

```text
C:\App Control\.cache\bin\control-server.exe
```

Recorded binary size: 17,100,800 bytes. Recorded SHA-256:

```text
FBC1CDBA4D2CC3CA4A7F51337105AC2CB2F7C338C17E90685C73A1A8994E4BFD
```

Server environment variables are documented in `server\README.md`. The most commonly used values are:

```text
APP_CONTROL_HTTP_ADDRESS
APP_CONTROL_DATABASE_PATH
APP_CONTROL_READ_HEADER_TIMEOUT
APP_CONTROL_READ_TIMEOUT
APP_CONTROL_WRITE_TIMEOUT
APP_CONTROL_IDLE_TIMEOUT
APP_CONTROL_SHUTDOWN_TIMEOUT
APP_CONTROL_MAX_HEADER_BYTES
```

Use HTTPS before connecting real parent or child devices outside local development. TLS/deployment hardening is a later phase.

## 9. Database migrations and current partial work

Completed and verified migrations:

```text
server\internal\storage\migrations\0001_app_metadata.sql
server\internal\storage\migrations\0002_parent_authentication.sql
```

P1-004 pairing work was started immediately before this handoff was requested. These files exist but have **not yet been formatted, compiled, or tested**:

```text
server\internal\storage\migrations\0003_device_pairing.sql
server\internal\pairing\service.go
server\internal\pairing\service_test.go
```

The partial pairing design includes:

- Parent-bound one-time pairing codes stored only as SHA-256 hashes.
- Pending devices with ECDSA P-256 public keys.
- A short-lived 32-byte enrollment challenge.
- A domain-separated `SHA256withECDSA` proof of private-key possession.
- Code reuse rejection, proof attempt limits, and parent-scoped device lookup.

Work that was planned but has not been added:

- Format and run the P1-004 Go tests; fix any failures found.
- Wire pairing routes into the HTTP API and server entry point.
- Add an Android `AndroidKeyStore` P-256 identity implementation to the child app.
- Add child enrollment networking and secure local enrollment state.
- Verify key generation and signing on a real Redmi or emulator.

Do not mark P1-004 as passed based only on the partial files. The tracker intentionally still requires working Keystore proof and parent isolation evidence.

## 10. Mandatory command/output logging

The user requires a detailed text record of every build, test, provisioning, migration, packaging, deployment, and verification command and its output.

Formal commands must run through:

```powershell
powershell -NoProfile -ExecutionPolicy Bypass -File "C:\App Control\scripts\run-logged.ps1" `
  -TaskId "P1-004" `
  -Category test `
  -WorkingDirectory "C:\App Control\server" `
  -Command "& 'C:\Program Files\Go\bin\go.exe' test ./... -count=1"
```

For an Android build:

```powershell
powershell -NoProfile -ExecutionPolicy Bypass -File "C:\App Control\scripts\run-logged.ps1" `
  -TaskId "P0-013" `
  -Category build `
  -WorkingDirectory "C:\App Control\android" `
  -Command ".\gradlew.bat :parent-app:assembleDebug :child-app:assembleDebug"
```

Allowed categories are `build`, `test`, `tooling`, `database`, `android`, `device`, `release`, `verification`, and `other`.

Logs are stored in:

```text
C:\App Control\logs\build\YYYY-MM-DD.txt
```

Every version-two entry records timestamps in IST, task ID, command, sanitized output, exit code, duration, estimated logged-text tokens, and a SHA-256 integrity hash. Exact model input/output token counts are unavailable and must never be invented.

Verify the current log:

```powershell
powershell -NoProfile -ExecutionPolicy Bypass -File "C:\App Control\scripts\verify-build-log.ps1" -Date "2026-10-23"
```

One historical hash mismatch is documented rather than silently rewriting the original record:

```text
logs\build\2026-10-23-integrity-exceptions.json
```

Summarize per-task logged-text token estimates:

```powershell
powershell -NoProfile -ExecutionPolicy Bypass -File "C:\App Control\scripts\summarize-task-tokens.ps1" -Date "2026-10-23"
```

The last completed integrity check before this handoff reported 183 valid bounded entries, 175 version-two entries, and one documented historical exception. At that point P1-003 had 26 logged commands and 26,341 estimated logged-text tokens. Counts will increase because creating this handoff is also logged.

## 11. Phase tracker and verified status

The authoritative status file is:

```text
tracking\phase-status.json
```

Display progress:

```powershell
powershell -NoProfile -ExecutionPolicy Bypass -File "C:\App Control\scripts\check-progress.ps1"
```

Status at the pause point:

| Phase | Passed | Required | Percent | State |
|---|---:|---:|---:|---|
| P0 Feasibility and toolchain | 4 | 13 | 31% | In progress |
| P1 Secure foundation | 3 | 6 | 50% | In progress |
| P2 Reliable command channel | 0 | 6 | 0% | Not started |
| P3 Device controls | 0 | 5 | 0% | Not started |
| P4 Production call handling | 0 | 6 | 0% | Not started |
| P5 Parent application | 0 | 5 | 0% | Not started |
| P6 Hardening and reliability | 0 | 6 | 0% | Not started |
| P7 Private release and pilot | 0 | 6 | 0% | Not started |

Passed checks:

- P0-001: architecture, implementation plan, and command contract.
- P0-002: Go and Android build toolchains installed and reproducible.
- P0-012: sanitized command/output logs and integrity verification.
- P0-013: parent and child Android baseline APKs build reproducibly.
- P1-001: server configuration, health, logging, and graceful shutdown.
- P1-002: SQLite migrations, transactions, integrity, encrypted backup, and restore.
- P1-003: parent authentication, login rate limiting, token rotation, reuse detection, and logout revocation.

P1-004 is the next check. It remains `not_started` in the authoritative tracker because the partial implementation has not been tested. When development resumes, change it to `in_progress` only after work continues; mark it `passed` only after all specified evidence exists.

## 12. Dependencies and independence from paid services

Current pinned dependencies:

| Dependency | Version | Hosted account/fee | Purpose |
|---|---:|---|---|
| Gradle | 9.6.0 | None | Android build wrapper |
| Android Gradle Plugin | 9.4.0 | None | Android compilation |
| `modernc.org/sqlite` | 1.61.0 | None | Pure-Go SQLite driver |
| `golang.org/x/crypto` | 0.58.0 | None | Argon2id password hashing |

These libraries compile into software we control. They do not create a runtime service dependency. All Go module versions and hashes are pinned by `server\go.mod` and `server\go.sum`.

FCM has not been integrated yet. When it is added, it must carry only an opaque instruction to synchronize with our HTTPS server. The system must keep a replaceable transport boundary and reconnect/reconcile behavior.

## 13. Files that must remain private or local

The ignore rules already exclude local build state and likely secrets:

```text
.idea/
.tooling/
.cache/
.gradle/
.kotlin/
local.properties
**/build/
*.iml
data/
*.db
*.db-shm
*.db-wal
.env
*.pem
*.key
firebase-service-account.json
```

Never place passwords, bearer tokens, refresh tokens, backup keys, private keys, Firebase service-account content, phone numbers, or real call data in source files or command logs. Android device private keys must remain non-exportable in Android Keystore. Server backup keys must be stored separately from encrypted backups.

## 14. Resume checklist

When development resumes in Android Studio:

1. Open `C:\App Control\android` and complete Gradle sync.
2. Run the logged two-module debug build.
3. In the Android Studio Terminal, format the three partial P1-004 Go files.
4. Run the pairing package tests, then the full Go test suite and `go vet`, all through the logging wrapper.
5. Review and fix any P1-004 failures before adding more code.
6. Wire the pairing service into authenticated HTTP routes.
7. Add the child Android Keystore P-256 identity class and enrollment client.
8. Build both APKs again.
9. Connect a factory-reset Redmi or suitable emulator and record actual Keystore proof evidence.
10. Update `tracking\phase-status.json` and verify the dated build log.

The first expected verification commands are:

```powershell
# From C:\App Control\server, through run-logged.ps1
& 'C:\Program Files\Go\bin\gofmt.exe' -w internal\pairing\service.go internal\pairing\service_test.go
& 'C:\Program Files\Go\bin\go.exe' test ./internal/pairing -count=1 -v
& 'C:\Program Files\Go\bin\go.exe' test ./... -count=1
& 'C:\Program Files\Go\bin\go.exe' vet ./...
```

This pause is a development stop only. No phase was declared complete because of the pause, and the partial pairing files must be treated as unverified until the commands above succeed.

Latest P1-004 build result: both debug APKs built successfully using installed Gradle 9.6.0, offline mode, and the quoted '-Pkotlin.compiler.execution.strategy=in-process' property. See docs/PAIRING.md. Device Keystore proof is still pending.

P1-005 continuation: signed device authentication challenges, short-lived hashed sessions, parent-scoped revocation, and Android authentication client added. Full Go tests and go vet passed. Socket transport revocation and device verification remain pending. See docs/DEVICE_CREDENTIALS.md.

P1-006 completed: migrations 0005–0007 provide transactional append-only audit events for authentication, enrollment, revocation, lock-command creation, desired-policy updates, and database recovery. GET /v1/audit-events provides bounded parent-scoped history. Full Go tests, go vet, and the server build passed. Control intents are persisted only; Android delivery, acknowledgement, expiry processing, and policy enforcement remain pending. See docs/AUDIT_EVENTS.md and docs/CONTROL_INTENTS.md. P1-004 and P1-005 retain their device/socket verification requirements.

P2 continuation (23 October 2026): migrations 0008–0009 add audited command delivery transitions and parent request idempotency. Signed device sessions fetch commands and acknowledge results through authenticated routes; expired, other-device and revoked requests are rejected. Abrupt subprocess termination/restart preserves queue and audit state. Real HTTPS and disconnected-response tests verify that parent success requires a device result. The child now offers explicit foreground sync with a durable reservation/result journal, HTTPS fetch/ack and latest desired-policy caching. JVM failure/retry checks, full Go tests, go vet, server build and both debug APKs passed. A local SDK copy under `.tooling/android-sdk` resolves read-only installed-SDK Java compilation failures. ADB reports no attached devices. FCM integration, physical duplicate/lock/reboot verification, background synchronization and policy enforcement remain pending. See docs/COMMAND_CHANNEL.md.

P2-002 continuation: optional post-commit wake-up interface and standard-library FCM HTTP v1 adapter now exist. Payload tests enforce fresh random syncHint data only, with routing metadata outside the application data. OAuth and registration tokens are injected; redirects are refused and errors exclude secrets. Provider failure preserves committed commands/policies. Main still uses disabled-by-default wakeups. Live OAuth acquisition, registration-token storage, Android Firebase setup and delivery remain pending. See docs/WAKEUP_PROVIDER.md.

Device-policy continuation (23 October 2026): migrations 0010–0011 add versioned child policy reports and app inventory, both audited transactionally. Parent reads distinguish desired and reported versions; stale reports conflict, unchanged reports/inventory deduplicate, and protected/unknown inventory targets are rejected. The child now has a persistent package-suspension reconciliation ledger, dynamic critical/system/role exclusions, persisted network-constrained background sync, and boot/update cached reconciliation. A visible consent-based local VPN prototype discards traffic only for explicit non-critical user-installed apps; empty target sets never create a tunnel, management/system apps remain outside, and resume closes the interface. Active VPN targets refresh on package and critical-role changes. Always-on and lockdown remain disabled. Full Go tests, go vet, server build, JVM package/VPN invariant checks and both debug APKs passed. ADB still reports no devices. Physical package/network/reboot/call acceptance remains open; P3 checks are in progress. See docs/DEVICE_POLICIES.md and scripts/test-package-policy.ps1.

P3-005 continuation: migration 0012 persists schedules/always-allowed packages and effective-state report keys. Policy format 2 is explicitly negotiated; old children cannot fetch extended policies. Cached Java schedule evaluation supports explicit zones, ISO weekdays, overnight/full-day windows, overlaps, DST gaps/repeated hours and allowlist precedence over suspension/VPN. Exact boundary alarms are used when permission allows, otherwise inexact alarms and a separate offline periodic job are fallbacks. Clock/zone/boot/update events re-evaluate the cache. Child reports include a state hash; the server independently rejects stale window results and exposes current versus reported keys. Shared Go/JVM vectors and HTTPS/rollback/audit/restore tests pass; both debug APKs build. Actual Redmi timing and offline/reboot behavior remain unverified. See docs/OFFLINE_SCHEDULES.md, contracts/policy.schema.json and scripts/test-package-policy.ps1.

Call feasibility continuation: child now has explicit dialer-role/permission setup, ACTION_DIAL keypad, Telecom.placeCall, incoming/ongoing notifications, locked-screen call UI, account selection and local answer/reject/end/mute/hold/DTMF controls. Live IDs are process-local; a pure action gate validates identity/state/deadline and protects emergency calls from the internal remote feasibility entry point. Speaker uses legacy Android 13 routing or Android 14+ endpoint callbacks. JVM call-action checks pass. Phone numbers remain in-memory/display only, and remote-call HTTP commands remain disabled. Both APKs build; actual SIM, locked-screen, audio, multi-SIM, emergency and crash recovery matrix remains open because no device is attached. See docs/CALL_FEASIBILITY.md and scripts/test-call-actions.ps1.

Server hardening/recovery continuation: default ingress protection now bounds peer/process rates, global/password concurrency, body/URI sizes and request contexts. Forwarding headers require explicit proxy CIDRs. Request logs use registered templates and suppress unknown paths, dynamic IDs and panic payloads; partial-response panics abort transport. control-admin implements binary-key-file keygen, backup, restore and integrity without silently creating missing databases. ACBKP002 authenticates stream completion and metadata; legacy ACBKP001 is rejected. Backup/restore publication refuses existing destinations atomically using hard links. Regression tests cover failure/private-output cases and recovery audit preservation. P6-003/P6-004/P6-005 remain in progress pending deployed load/log checks, certificate rotation and actual device recovery. See docs/SERVER_PROTECTION.md and docs/DATABASE_RECOVERY.md.

Recovery verification continuation: control-admin verify now authenticates/decrypts backups into an explicit private scratch directory, checks SQLite and removes normal-path plaintext without migrations, recovery audit insertion or publication. Backup/restore streams check context cancellation and refuse publication after observed cancellation. scripts/backup-database.ps1 provides an unattended logged backup/verify job with unique snapshot names, an exclusive directory lock, a default 30-snapshot capacity gate and no automatic deletion. Local Windows smoke checks passed valid creation, scratch cleanup, special-character path quoting, capacity refusal, overlap refusal and invalid-key failure. Scheduling under the deployed operator account and off-host archival remain open. See docs/DATABASE_RECOVERY.md.

Server-trust continuation: both apps generate Android network security configuration through variant resource tasks, with system anchors and distinct current/backup SPKI pins when deployment properties are supplied. Release requires complete trust settings; unconfigured debug remains system-trusted HTTPS. Child UI/auth/sync bind to the compiled origin, and configured enrollment preloads that origin. The active enrollment client is network/DeviceEnrollmentClient.kt because the original enrollment source directory is write-protected. JVM origin negatives pass. Configured fixture APK inspection confirms the host, two pins, system anchors and manifest reference; unconfigured release trust validation is refused. control-admin tls-pin derives public pins from one PEM certificate and rejects private keys/bundles. Actual Android TLS handshakes and A-to-B rotation remain unverified. See docs/SERVER_TRUST.md.

Logging continuation (24 October 2026): scripts/test-logged-command-output.ps1 now checks both console output and the persisted hashed entry for successful commands, output before a terminating exception, and native exit 17. The regression passed, and build-log integrity verification passed with no exceptions. The connected Android 37 acceptance emulator also passed the production Keystore identity and Device Owner package-policy probes, including fixture cleanup. Physical Redmi acceptance remains open.

Parent continuation (24 October 2026): native ParentActivity now provides login/enrollment, capability observations, policies/schedules, lock idempotency and chronological command history. Android vault and controlled-client checks pass; actual launcher verified. Backend adds parent-scoped device pages, server-timed capability status and bounded command history (migration 0013). Migration 0014/operator recovery revokes parent sessions and unused enrollment with immutable audit, preserves active children and blocks stale verified passwords from issuing sessions. Full Go tests and vet pass after correcting and regression-testing existing inventory audit vocabulary. See docs/PARENT_APP.md and docs/PARENT_RECOVERY.md. Live deployment, FCM, physical Redmi and pilot gates remain open.

## Same-key package upgrade evidence (24 October 2026)

The isolated Android 37 Device Owner AVD passed a real debug-signed child upgrade from versionCode 1 to 2 (0.1.1-fixture). PolicyUpgradeProbe prepares cached version 301, deliberately unsuspends the disposable fixture, and records the public Keystore identity and boot count. After adb install -r, wait-android-policy-upgrade.ps1 observed production package-replaced reconciliation and suspension before verification instrumentation started. Verification confirmed the higher package version, unchanged boot, retained Device Owner, identical Keystore public key, cached policy/ledger and suspension. Cleanup restored the fixture and baseline preferences. A version-1 replacement was refused with INSTALL_FAILED_VERSION_DOWNGRADE; no downgrade override was used. The emulator child remains installed at versionCode 2. Future debug installs must use appControlVersionCode=2 or higher with appControlVersionName supplied too.

This is same-debug-signing-key emulator evidence, not production signing, live enrolled server continuity, remote update delivery or successful rollback. PolicyUpgradeProbe supports Prepare/Verify/Cleanup through scripts/test-android-policy-upgrade.ps1. Keep phases together on the guarded unenrolled AVD; the wait script observes production state and never applies policy. Failed cleanup retains reconciliation state.

FCM server continuation (24 October 2026): migration 0015, messaging-only Google service-account JWT/OAuth source, optional private file configuration, encrypted device-bound registration routes and transactional revocation cleanup are implemented. Full Go tests and vet pass using isolated generated credentials; no real Google credentials read and no live request sent. Android Firebase dependency/configuration and receive/token callbacks remain pending. See docs/WAKEUP_PROVIDER.md. Evidence ZIP tools now validate dated log integrity and exact archive hashes; local snapshot verification passes, while off-host archival remains open.

Cross-platform metadata evidence (24 October 2026): full Go tests/vet and release-manifest CLI checks pass. A newly generated public fixture signed by Go verifies with the platform Ed25519 implementation in both parent and child Android 37 test APKs. Wrong public key, origin, APK signer, replay sequence, exact expiry, altered payload, unknown envelope fields and trailing data are refused. The fixture uses synthetic APK hashes and a test clock and retains no private key. This proves metadata verification, not trusted production key embedding, artifact download/inspection, installation or hosting.

24 October continuation: versioned ui-controls-v1 synthetic Telecom screen-button checks passed for answer, hold, resume, mute/unmute, end and reject; fixture account and original dialer restored. Both installed version-2 Android clients passed shared RequestDeadline checks, including interrupted parent requests and child stalled TLS cleanup. Full Go suite and vet passed after read-only signed-release metadata hosting and expiry isolation changes. Expired authentic metadata yields 503 on its route while server health remains available. APK checks confirm Telecom acceptance code remains outside application APKs. Physical SIM/audio/Redmi, live Firebase/deployment, release lint and pilot gates remain open.

Latest executable rehearsal: rebuilt control-server/control-admin/parent-recovery/release-manifest. Live loopback WAL backup, restore, integrity, parent login, overwrite refusal, ingress/concurrency and log privacy passed. Optional inherited FCM/release settings were deliberately invalidated; fixture startup isolated them and restored caller settings. Both ADB daemons report zero authorized physical devices. These are local proofs; production host, TLS and elapsed pilot remain open.

Parent durability continuation: synthetic vault prepare/process-stop/restart/reboot/cleanup phases passed sequentially on Android 37. Retained ciphertext/nonce decrypt with the non-exportable key after increased boot count. Prior crashed/overlapping attempt is excluded. No production session touched; live authentication after reboot and Redmi remain open. ParentPersistenceProbe is confined to the test APK.

Blocked audit, 24 October 2026: no authorized physical device on either ADB daemon; optional FCM/release settings absent; lint-gradle 32.4.0 absent from project cache, with prior online resolution denied. The same external prerequisites persisted across three goal turns. Local vault lifecycle work is verified, but physical feasibility, live service integration, release acceptance and elapsed reliability/family pilot remain incomplete. Resume when the designated Redmi/test SIM and service/release prerequisites are available. Do not enable production remote-call transport before the physical feasibility gate.

Private updater checkpoint: shared ReleaseState/ReleaseUpdate/ReleaseInstaller/ReleaseActivity and both UpdateActivity entries compile. update-trust.gradle.kts anchors are public/default empty. UI requires local install confirmation and preserves ambiguous session state; resume revalidates metadata. Focused replay/staging proof passes; no actual installer commit. stage-private-release.ps1 public bundle proof passes, no publication. See docs/PRIVATE_UPDATES.md. Copilot independently owns live-call publication/foreground parent workflow; broad test runs deferred under feature-building priority.
