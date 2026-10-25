# Completed work

## Exported release configuration verification - 25 October 2026

- Added `scripts/test-configured-release-builds.ps1` to build directly from freshly exported public argument arrays using reserved fixture origin/pins, synthetic update authority and version 4. It snapshots unsigned APKs, compiled BuildConfig, full lint XML and hashes; only complete runs publish `configured-builds.json`.
- Polling and Firebase release paths pass assembly/full lint and exact compiled origin/key/certificate/sequence/version/Firebase checks. Actual DEX/manifests verify Firebase SDK/service appears only in the Firebase child; instrumentation/contact/artifact probes are absent from release code. Evidence: `.cache/configured-releases-eb55685791aa4ab1bbf916ccefe11b1a/configured-builds.json`, `logs/build/2026-10-25.txt`.
- Corrected two new-runner issues exposed during validation: PowerShell array concatenation grouping and treating ordinary native Gradle stderr warnings as terminating errors. Failed attempts remain logged; successful result follows both fixes.
- The extended three-mode matrix also passes: explicit remote-call opt-in, disabled polling, then disabled Firebase. Evidence: `.cache/configured-releases-bd46c6ccb4cf49cf882b8f960b668ab4/configured-builds.json`. Six APK/BuildConfig/lint inventories independently rechecked; actual APK versions are 4 / 0.1.3-config-fixture and non-debug. Final ordinary release and fcmRelease generated flags are both false.
- Added the contact-cache probe to the existing debug/test APK separation checker. Updated configuration workflow and stale private-update release notes. No production-source, signing-key, phone, live Firebase or deployment change.

## Full Android lint and encrypted contact runtime - 25 October 2026

- Ran the previously deferred full release lint. Parent passed; child reported six errors. Added explicit READ_PHONE_STATE gating and SecurityException fallback for SIM-account choices, API 34 guards before endpoint callbacks and optional telephony hardware declaration. Documented the private managed-inventory QUERY_ALL_PACKAGES exception on that permission only.
- Added explicit cloud-backup and device-transfer exclusions for all supported credential/device-protected storage domains in both apps. Both full release lint tasks and both release APK assemblies pass; actual APK resource inspection confirms both complete exclusion sections. Remaining warnings are UI strings, app icons and synchronous durable preference writes.
- Corrected stale call-control tracker notes claiming the parent was still blocked on setSilent.
- Added test-APK-only `CallContactCacheProbe` and strict versioned runner. Current debug app and test APK compile and install as version 3 on the isolated acceptance AVD without uninstall/data clearing. `sealed-cache-v1` passes real Keystore encryption/non-exportability, ciphertext-only preferences, reload, rollback/conflict rejection, enrollment-device AAD separation, tamper refusal, fail-closed version preservation, newer empty list and fixture cleanup. No network, production enrollment mutation or contact calls.
- Initial emulator wait timed out because isolated ADB used the wrong key. Restarted only the test daemon on port 5038 using the workspace emulator key; Android 37 boot/fingerprint and cache proof pass. No physical phone attached or altered.
- Current-build normal dialer qualification, both empty ACTION_DIAL paths/keypad UI and `ui-controls-v1` synthetic Telecom UI/control/safety-gate checks pass. Original role and Telecom dialer are restored and the fixture account cleaned up. Actual SIM/audio/emergency/Redmi acceptance remains open.
- Full logs: `logs/build/2026-10-25.txt`. These source fixes postdate the earlier two-clean-build reproducibility proof; the earlier APK hashes are historical, not hashes of the newly fixed APKs.
- Repeated the two-clean-build comparison after all production-source fixes: current parent and child unsigned release APKs match. Current evidence: `.cache/release-repro-278c3408a4014f4fa3ddfa9d10248ce3/reproducibility.json`. Fixture origin/pins, polling child, default-disabled remote calls and unconfigured update authority remain the scope. Closed the session-launched emulator through its console serial after restoring runtime fixtures; AVD data retained.
- Local evidence snapshot `.cache/evidence-20261025-be11b975c75d4048b9aefece74abf13e.zip` verifies all three dated logs, allowlisted inventory, sizes and hashes; wrong-digest refusal passes. SHA-256: `a44317a114f9dc35978e17e7c0a193a634ae6b0795a2bdd373dbf84d6c56ed10`. The archive predates this receipt and its own logged command entry by design. No off-host upload or production deployment performed.

## Managed setup export review - 25 October 2026

- Resumed the paused combined-export work. Added verified-payload preservation and verifier identity/hash tracking alongside the existing metadata/key/renderer guards.
- Added `scripts/test-managed-setup-export.ps1` using the existing independently signed pre-handler fixture. Existing-output preservation, nested-output refusal, wrong-certificate refusal and missing-handler refusal pass; failed exports do not publish completion inventories.
- The first refusal run exposed Windows PowerShell native-stderr termination before the intended metadata error. Fixed the payload generator to capture verifier output and check its exit status; the final focused run passes.
- Documented operator invocation, completion inventory and partial-output handling in `docs/MANAGED_SETUP.md`. Logs: `logs/build/2026-10-25.txt`.
- Built current parent and child release APKs offline with an isolated `.invalid` origin and fixture pins. Both release assemblies and vital lint checks pass.
- Added and passed `scripts/test-managed-setup-roundtrip.ps1`: disposable APK/metadata signing, real protected-handler inspection, successful setup export and independent QR decoding, output hashes, payload checksum/URL/origin binding, stale-sequence refusal and modified-APK refusal. Successful evidence is preserved at `.cache/setup-roundtrip-0580952457f34a5799b1a545913d806f`; private fixture keys were removed. No production identity used.
- Vendored QR regression tests and the full Go server test suite pass. All commands are in `logs/build/2026-10-25.txt`.
- No phone reset, scan, enrollment or provisioning performed. P0-003 and physical gates remain open.
- Parent call-watch freshness/order/exact-call checks and contact freshness/offline/clock-rollback checks pass on the JVM.
- Current parent and polling-child unsigned release APK hashes match across two clean offline builds with task reruns and build cache disabled; vital lint passes in both. Evidence: `.cache/release-repro-6394d1d8fd3848b085caa59b5afe1b57/reproducibility.json`, `logs/build/2026-10-25.txt`. Fixture origin/pins, version 3, update authority and Firebase unconfigured; no production readiness claimed.
- Fixed daily log verification incorrectly requiring a redaction-probe marker on every date. Routine verification and explicit `-RequireRedactionProbe` are separate; probe leakage is always rejected. Current-day integrity, original 23 October probe verification and missing-probe refusal pass, as do the full logger output/Unicode/stderr/failure/hash regression checks.

## Pause handoff - 24 October 2026

- User requested saving and pausing until tomorrow. No device was reset, erased or provisioned.
- Copilot supplied the isolated vendored QR encoder/independent decoder in tools/provisioning-qr. Offline round-trip checks pass; dependencies remain separate from the server.
- Review found PNG overwrite protection missing. Fixed with exclusive file creation and an existing-file preservation check. Focused vendored Go tests and offline build pass; executable is .cache/provisioning-qr.exe.
- Added scripts/export-managed-setup.ps1 to combine independently verified release payload generation, QR rendering and a public completion/hash inventory. It refuses an existing output directory, keeps output outside the immutable release bundle and checks authority/renderer consistency. No end-to-end wrapper execution or current signed-release positive acceptance has been performed yet.
- Resume here: review/validate the new wrapper, document its operator invocation in docs/MANAGED_SETUP.md, then run focused refusal/positive fixture checks if a suitable current signed release is available. Do not claim software round-trip is a phone scan. P0-003 and all physical provisioning gates remain open; the user's only phone must not be erased.
- Production public-configuration exporter and focused checks are already recorded below; continuous parent Firebase push and actual production configuration/credentials remain open.

## 24 October 2026

- Fixed Android Gradle Plugin 9 built-in Kotlin source-set wiring for the
  optional Firebase and polling-only wake-up implementations. The polling
  child build had failed because Kotlin files were being added through the
  Java source directories.
- Verified the child debug APK and Android test APK build successfully.
- Verified release Kotlin sources compile using synthetic server trust pins.
- Verified optional Firebase variant configuration and the opaque wake-up
  contract checks. The checks reject notifications, extra data, commands, and
  malformed or noncanonical hints.
- Updated `docs/WAKEUP_PROVIDER.md`, `ANDROID_STUDIO_HANDOFF.md`, and
  `tracking/phase-status.json` with implementation status, evidence, and
  remaining acceptance requirements.
- Recorded build and verification commands in `logs/build/2026-10-24.txt`; the
  daily log integrity check passed.

### Remaining limitations

- Live FCM delivery and authenticated sync are not verified. Firebase
  project configuration, a real test project, and device-delivery evidence
  are still required.
- The initial offline release build was blocked by uncached
  `com.android.tools.lint:lint-gradle:32.4.0`; dependency access was later
  available and the Firebase release variant assembled successfully below.

## 24 October 2026 — Optional Firebase variants

- Confirmed Google Maven access and built both opt-in variants with:

  ```powershell
  $env:JAVA_HOME = 'C:\Program Files\Java\jdk-17'
  $env:GRADLE_USER_HOME = 'C:\Android Control\.cache\gradle'
  & 'C:\Gradle\gradle-9.6.0\bin\gradle.bat' --no-daemon `
    '-Pkotlin.compiler.execution.strategy=in-process' `
    '-PappControlFirebase=true' `
    '-PappControlFirebaseApplicationId=<test-project-application-id>' `
    '-PappControlFirebaseApiKey=<test-project-public-api-key>' `
    '-PappControlFirebaseProjectId=<test-project-id>' `
    '-PappControlFirebaseSenderId=<test-project-sender-id>' `
    '-PappControlServerOrigin=https://<test-server-host>' `
    '-PappControlCurrentPin=<synthetic-current-pin>' `
    '-PappControlBackupPin=<synthetic-backup-pin>' `
    :child-app:assembleFcmDebug :child-app:assembleFcmRelease
  ```

- Result: `BUILD SUCCESSFUL`; both APKs were created and their manifests
  contain `FirebaseApplication` and `FirebaseWakeupService`:
  `android/child-app/build/outputs/apk/fcmDebug/child-app-fcmDebug.apk` and
  `android/child-app/build/outputs/apk/fcmRelease/child-app-fcmRelease-unsigned.apk`.
  The build output and command are recorded in `logs/build/2026-10-24.txt`.
- No test Firebase project configuration was present in the workspace or
  environment. The successful compile used syntactically valid fixture
  properties, not credentials or identifiers for an actual Firebase project.
  This verifies compilation and packaging only, not project registration,
  authentication, or delivery.
- The dated build-log verifier currently reports a hash mismatch in the
  earlier `P7-002` entry (started `2026-10-24 19:46:07`), which predates this
  Firebase build. The entry was left unchanged; this verification issue is
  separate from the successful Gradle build.

### Remaining limitations

- Live FCM delivery and authenticated sync remain unverified. Use an actual
  test Firebase project and collect device evidence before changing either
  FCM acceptance gate.

## APK inspection � 24 October 2026
- Shared ReleaseArtifact verifier and conservative same-key/increasing-version upgrade guard implemented.
- Parent app/test APK builds and Android 37 actual disposable-signed APK inspection pass, including signed metadata, debug/content and incompatible upgrade-baseline rejection.
- Corrected private-storage ancestor path handling after an initial failed probe. Cleanup and test-fixture exclusion from application APKs pass.
- Download/staging, production trust anchors, installer authorization, hosted artifacts and real enrolled update/rollback remain unfinished; P7-002 remains in progress.

## Download staging and log verification - 24 October 2026
- Shared ReleaseDownload private staging implemented and verified on Android 37 with controlled HTTPS objects and a real disposable-signed APK. Redirect/length/hash/encoding/expiry/cancellation/interruption and cleanup checks pass; no live download or installation is claimed.
- Fixed SHA-256 verifier UTF-8 decoding. Every entry on 23 and 24 October matches the original stored hash without modifying log bytes. The earlier exception record is retired but preserved; Unicode logging regression passes.
- Parent app/test APKs and child polling app build successfully with the shared staging code. Production update trust, replay persistence, installer flow and device acceptance remain open.

## Release build preparation - 24 October 2026
- Strict two-clean-build parent/child assembleRelease comparison passed with matching unsigned APK SHA-256 values and vital lint enabled. Reserved host and synthetic public pins only; no deployment claim.
- Isolated random-key signing checks passed on those exact outputs; production signer and hardware acceptance remain open.
- Separate full lint found SDK drive-colon escaping in local.properties. Corrected the property; no checks disabled. A full lint rerun is deferred under the current feature-building priority.

## Remote call commands - 24 October 2026

- Added parent-queued `ANSWER_CALL`, `REJECT_CALL`, and `END_CALL` commands
  addressed to one exact canonical UUID v4 call ID, with a server-enforced
  1–30 second lifetime, parent/device ownership checks, strict payload
  decoding, and idempotency bound to the complete action input.
- Added migration support for these command types without changing existing
  command records. The child dispatches Telecom work on its main thread and
  only acknowledges success after observing `ACTIVE` for answer or
  `DISCONNECTED`/removal for reject and end; `DISCONNECTING` alone is not
  considered completion. Stale identities, invalid states, expiry, and
  protected emergency calls are rejected.
- FCM remains an opaque synchronization hint. The new API and command payload
  carry no phone number, contact name, or call content. `docs/REMOTE_CALL_API.md`
  documents the routes, request/response behavior, privacy constraints, and
  foreground parent workflow.
- Focused checks passed:

  ```powershell
  & .\scripts\test-call-actions.ps1
  Set-Location server
  & 'C:\Program Files\Go\bin\go.exe' test ./internal/control ./internal/httpapi `
    -run 'Test(CallCommandsAreBoundedExactAndParentScoped|RemoteCallCommandHTTPAuthorizationAndPayload)$' -count=1
  Set-Location ..\android
  & 'C:\Gradle\gradle-9.6.0\bin\gradle.bat' --no-daemon `
    '-Pkotlin.compiler.execution.strategy=in-process' `
    :child-app:compileDebugKotlin :child-app:compileDebugJavaWithJavac `
    :parent-app:compileDebugKotlin :parent-app:compileDebugJavaWithJavac
  ```

- Result: JVM identity/state/deadline/observed-result checks passed, focused
  Go authorization/payload/idempotency/expiry tests passed, and parent/child
  debug Kotlin/Java compilation succeeded. Commands and output are recorded in
  `logs/build/2026-10-24.txt`.
- P4-002 and P4-003 remain `in_progress`. No SIM call, enrolled-device test,
  or Telecom hardware result is claimed; hardware acceptance gates remain open.

## Foreground parent call controls - 24 October 2026

- Added authenticated `POST /v1/device/calls` and
  `GET /v1/devices/{deviceID}/calls` routes for bounded opaque call-ID/state
  snapshots. Reports contain no phone number, contact name, or call content;
  parent access remains device-owner scoped.
- The child publishes its process-local Telecom call IDs and states promptly
  after Telecom callbacks, with coalescing and a bounded 15-second lightweight
  active-call refresh. This snapshot sync does not run policy or inventory
  synchronization. Server-issued monotonic snapshot sequences prevent delayed
  older reports from overwriting newer state; service restart publishes the
  cleared call set.
- Added the parent app's foreground **Foreground calls** screen. It compares
  server-authored timestamps, hides actions for stale snapshots, and offers
  answer/reject only for ringing calls and end only for active/held calls.
  Queued status is not presented as a successful call outcome.
- Scope is foreground parent call controls. Snapshot refresh is sync-based,
  not a continuous feed; background incoming-call notification behavior is
  unchanged and excluded. P4-002/P4-003 and all hardware acceptance gates
  remain `in_progress`.

## Remote-call rollout and snapshot safety - 24 October 2026

- Added server `APP_CONTROL_REMOTE_CALLS_ENABLED=false` and child Gradle
  property `appControlRemoteCallsEnabled=false`. Disabled server mode rejects
  call commands while preserving lock and policy behavior; disabled child
  builds do not execute Telecom commands.
- Added per-call `remoteActionsAllowed`. The child fails closed for emergency
  and unclassified calls, and the server also forces permission false when
  its rollout flag is disabled. The parent disables action buttons unless the
  snapshot explicitly allows remote actions.
- Snapshot changes are sent promptly after Telecom arrival/state/removal
  callbacks through a coalesced, serialized lightweight worker. Active calls
  receive a bounded 15-second heartbeat; no full inventory or policy sync is
  run by the heartbeat.
- Server-issued persistent snapshot sequences reject out-of-order reports.
  Process/service restart clears the in-memory IDs and publishes a replacement
  snapshot; the server never accepts an older sequence over a newer one.
- The foreground snapshot worker and full command/policy sync share one
  serialized, expiry-aware in-memory device session, avoiding session
  replacement races without persisting or logging bearer tokens.
- Focused feature-flag, call-state permission, ordering and debug-compilation
  checks are recorded in `logs/build/2026-10-24.txt`. No device/hardware
  acceptance is claimed; all hardware gates remain open.
- Final focused config/control/HTTP tests and JVM permission/state checks pass.
  Child debug Kotlin/Java compilation passes; generated child configuration
  was verified false by default, true with explicit opt-in, then restored to
  false. A later full parent recheck is blocked by an unresolved
  `Notification.Builder.setSilent` call in the separate `CallWatchService.kt`
  background-notification feature, which was left untouched. No live
  Telecom/device result is claimed.
- Final child compilation after refining shared-session locking passes with
  the rollout flag verified false by default. Snapshot and full-sync network
  requests share a cached in-memory session and serialize only each individual
  request, avoiding both token replacement mid-request and delaying snapshot
  publication behind an entire full sync.

## Private update feature build - 24 October 2026
- Built compiled public update authority, persistent same-sequence payload/replay protection, signed metadata retrieval and strict self-update preparation.
- Built private update screens for both apps and PackageInstaller submission, result matching, resume/cancel and installed-version reconciliation. Revalidate authority/freshness before resumed commit and confirmation; no uninstall or phone data clearing.
- Parent/child debug compilation passes. Focused replay/staging proof passes with staging-replay-v1; actual installation remains unverified.
- Added stage-private-release.ps1 and positively staged actual disposable-signed APKs with independently verified metadata; no upload/install; disposable metadata seed removed.
- Release runner now accepts optional update authority and Firebase public config/fcmRelease selection. Syntax passes; final release/regression sweeps deferred while building features. Earlier clean-build proof predates updater changes.
- Current user/operator flow and remaining gates: docs/PRIVATE_UPDATES.md.

## Managed setup components - 24 October 2026
- Built child provisioning-mode and policy-compliance activities protected by BIND_DEVICE_ADMIN. Fully managed mode must be allowed by Android; compliance confirms actual Device Owner plus matching compiled/public origin.
- Added independently verified public provisioning-payload generator. Syntax and refusal of an old signed APK lacking handlers pass.
- Child debug build passes. No reset, provisioning operation, successful download or QR scan performed. P0-003 remains open. See docs/MANAGED_SETUP.md.

## Provisioning QR tooling - 24 October 2026
- Added the isolated `tools/provisioning-qr` Go module with pinned `go-qrcode` encoder (`v0.0.0-20200617195104-da1b6568686e`) and independent `gozxing` QR decoder (`v0.1.1`).
- Vendored the required source packages and upstream license files, including transitive `golang.org/x/text` and `golang.org/x/xerrors`. The server module was not changed.
- The utility accepts only the expected public managed-provisioning payload shape, renders PNG, and independently decodes it to verify the decoded text exactly matches the input.
- `GOPROXY=off go build -mod=vendor ./...` and `GOPROXY=off go test -mod=vendor ./... -count=1` pass from `tools/provisioning-qr`. CLI smoke verification passes. This is software encoding/decoding evidence only: no QR scan, phone reset, or device provisioning was performed; P0-003 remains open.

## Child audio-output feature build - 24 October 2026
- Added an active-call Audio output picker for earpiece, speaker, wired and Bluetooth using Android 13 route/device APIs and Android 14+ exact endpoints.
- Added explicit legacy Bluetooth permission flow, optional hardware declaration, stale call/choice checks, observed-state versus request feedback and per-call late-callback cleanup. Bluetooth names remain local and transient.
- Latest child debug APK build succeeds. Actual audio/call/device tests remain deferred; P4-004 stays in progress.

## Encrypted backup scheduler bundle - 24 October 2026
- Added export-backup-task.ps1: reviewable Windows Task Scheduler XML, logged launcher and hash inventory; no task registration/execution or key-content copying.
- Added S4U least privilege, local/disjoint path guards, tool-hash pinning, overlap prevention and bounded execution time; existing verified backup command remains the worker.
- Synthetic export XML/settings and launcher syntax checks pass. Task Scheduler schema validation unavailable; actual deployment identity/ACL behavior, off-host archival and restore remain open. See docs/BACKUP_SCHEDULING.md.

## Matched server release bundle - 24 October 2026
- Added build-server-bundle.ps1 with offline/read-only builds of control-server, control-admin, parent-recovery and release-manifest; matched Windows x64 and Linux x64 artifacts, dependency/build information and source inventory.
- Default eight binaries compiled successfully in .cache/server-bundle-bb14ecce9eb74837a52871c80bf5c197. Source remained unchanged during compilation; exported binary size/SHA-256 and source/dependency inventory hashes independently checked successfully.
- No deployment, process start, database migration or Linux execution performed. Bundle is unsigned; target runtime acceptance and dependency licence review remain open. See docs/SERVER_BUNDLES.md.

## Parent-managed call shortcuts - 24 October 2026
- Built parent contact editing and a child Call Parent/contact picker. Selecting a contact fills the dial pad; the child must review and press Call. No automatic outgoing calling or outgoing-call restriction was added.
- Added separately versioned owner/device contact APIs and migration 0019, CAS, strict international number/label validation, at most 10 unique contacts and one primary parent. Immutable content-free auditing is transactional with writes.
- Added best-effort authenticated contact sync after durable controls, enrollment-bound Keystore AES-GCM cache, rollback/same-version conflict refusal and stale-selection checks. Contacts remain usable offline; saving on server does not imply child receipt.
- Focused Go service/HTTP tests pass for payloads, authorization, stale edits, clearing, revocation and audit rollback. Both Android debug APKs build; no contact/UI/call/device acceptance was run. See docs/CALL_CONTACTS.md and contracts/call-contacts.schema.json.
- Rebuilt all eight Windows/Linux x64 server/tool binaries with migration 0019 in .cache/server-bundle-contacts-66385c4c102d4c30b10062cf9962c845. No deployment or target runtime execution. The earlier bundle predates this feature.

## Bounded parent incoming-call alerts - 24 October 2026
- Built an explicit user-started call watch for one child: notification permission, visible data-sync foreground service, serialized five-second polling between requests, ten-minute elapsed observation limit, stop action and logout cleanup. No permanent polling, boot restart or parent Firebase delivery was added.
- Ringing-call notifications carry opaque IDs and review-only immutable intents. Opening an alert fetches fresh authorized state for that exact call before offering existing confirmed commands; a different call cannot replace it. Caller numbers, contact names and media are absent.
- Added snapshot/request-latency freshness checks, old-sequence refusal, duplicate alert suppression, notification expiry at remaining freshness deadline, bounded in-memory identities and notification/channel/auth failure handling. Foreground action buttons also expire during confirmation.
- Focused JVM call-watch checks pass. Initial platform notification-builder API compilation error was corrected; final parent debug APK build passes. No notification, background-service, parent-server, real-call or Redmi runtime acceptance was performed. P5-004 remains in progress; continuous parent push/live Firebase remains open. See docs/PARENT_CALL_ALERTS.md.

## Public production build configuration - 24 October 2026
- Added export-production-configuration.ps1: strict public deployment JSON validation and exclusive export of matching Gradle properties/argument arrays, polling/FCM task selection, optional Firebase public config and hashed completion inventory.
- Validates canonical origin/pins, distinct backup pin, APK certificate digest, Ed25519 public-key encoding, version/update ranges, explicit rollout flag and Firebase application/sender consistency. Unknown fields/private-key fields and existing outputs are refused. Reserved hosts require explicit fixture mode; all outputs retain productionReady=false.
- Focused fixture/refusal/polling/Firebase/hash checks pass. Exported fixture arguments pass both actual Gradle release BuildConfig generation/trust-validation tasks; generated origin/version/update floor match for both apps. No APK assembly, signing, network deployment or real production values used in this check.
- The isolated provisioning QR renderer and independent decoder are completed with pinned, vendored dependencies; offline build and software round-trip checks pass. This does not change production configuration status or claim a device QR scan. See docs/MANAGED_SETUP.md.
