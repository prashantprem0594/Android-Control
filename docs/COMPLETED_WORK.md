# Completed work

## Android 17 home-network access and current release fixtures - 26 October 2026

Both apps now declare ACCESS_LOCAL_NETWORK and expose explicit foreground setup. Known private origins stop for consent before requests; private DNS deployments have the same setup button. Production HTTPS origin/pin validation is unchanged. Child setup uses a secure window and disables saved state for pairing/server/name fields.

Gateway HTTPS acceptance passes all 16 integration stages and cleanup at `.cache/android-server-https-reports/2ae09f7cc3da4c7aaba85013f59c9320.json`. Connections use 10.0.2.2 without ADB reverse, an IP-SAN certificate and ephemeral CA/SPKI trust in debug-only acceptance apps. Production DNS-only origin validation remains intact.

The IPv4 TCP VPN fixture passes baseline, selected-UID pause, management-UID reachability, resume and cleanup. Earlier baseline timeouts were caused by missing Android 17 LAN consent. The test now waits for framework VPN registration before checking the selected UID. DNS, IPv6, switching networks, physical Redmi and live Firebase remain unverified.

All six unsigned parent/child configured fixture releases were refreshed after these production changes. Assembly/full lint/configuration and APK test-code exclusion pass: `.cache/configured-releases-92e4cbe479b2402689d6dcd89de3619d/configured-builds.json`. These are fixture artifacts, not signed production releases. This release checkpoint preceded Copilot parent-push server edits. The earlier independently verified server bundle is `.cache/server-checkpoint-20261026-7963355f`.

## Child shortcut UI and actual watch deadline - 26 October 2026

- Added side-by-side child DialActivity cache-to-screen checks: secure window/no saved number state, Call Parent prefill without pressing Call, stale/removed shortcut refusal that preserves manual input, updated shortcut prefill, and exact prior cache/enrollment/key restoration. Calling permission is denied and the dialer role is absent; no permission/role change or outgoing call occurs. This uses locally seeded encrypted fixture contacts, separate from the already verified live CommandSync delivery.
- Standard 14-stage HTTPS integration and cleanup pass at `.cache/android-server-https-reports/0eda627201664881b5bb5e164b69b4f7.json`. The later explicit `-ObserveCallWatchWindow` run also passes all 14 stages, current inline child safety guards and cleanup: `.cache/android-server-https-reports/d1da1d46760e4a288dbb796e12700523.json`, `.cache/integration-0f6da625bcdf4c0aa165fedc498696e8/result.json`.
- Production CallWatchService stopped automatically after an observed **600807 ms**. The authenticated parent session and synthetic device remained valid, the watch restarted normally, and UI logout removed it. Only the test activity temporarily keeps its own screen awake; no global screen setting is changed. Required marker: `parent_call_watch_window_probe=passed`. Native visible-app deadline evidence does not establish OEM background/process-death, SIM or family-pilot behavior.
- Full current Go `vet ./...` passes. Optional elapsed observation refuses non-live invocation before creating a report or running tests; the PowerShell runner syntax checks pass. Commands/results: `logs/build/2026-10-26.txt`.

## Parent call-watch runtime lifecycle - 26 October 2026

- Added live parent foreground-service/notification acceptance using authenticated synthetic server call snapshots. Production UI starts call watch through its notification-permission flow, publishes a private alert with generic public content and an immutable review-only intent, opens fresh exact-call state with remote actions disabled, removes a cleared-call alert, and shows the exact-call-gone message when the old intent is reopened.
- Explicit Stop removes service/alerts; a second watch is removed by normal UI Sign out. Fixture cleanup also stops the service before clearing its session. The notification permission choice is retained only for the side-by-side emulator `.acceptance` app, which remains installed; no physical-phone permission/settings are changed.
- The full 13-stage HTTPS suite and outer cleanup pass: `.cache/android-server-https-reports/d6ab3ce2fe064d248be437b44f827d71.json`, `logs/build/2026-10-26.txt`. Required marker: `server_https_parent_call_watch_probe=passed`. No actual incoming/outgoing call, Firebase delivery, ten-minute elapsed expiry, lock-screen visual/OEM behavior or process-death acceptance is claimed.

## Verification evidence snapshot - 26 October 2026

- Verified all four dated build logs (23-26 October) successfully, including 277 entries at the current 26 October cutoff, with no hash exceptions. Created and independently verified `.cache/evidence-checkpoint-20261026-353a6a93.zip` (SHA-256 `67aa6c5ea9d3c5e7c76211e23f627e6d87d879ac66c5fca941d4b88b5be403fb`). Every allowlisted archived file size/hash verifies without extraction.
- This is a local cutoff snapshot of sanitized logs/contracts/docs/progress, excluding credentials, databases, signing keys, APKs and caches. The archiving command and subsequent documentation are recorded after that cutoff. Off-host archival and production release/deployment evidence remain open.

## Current release artifacts - 26 October 2026

- The three public fixture configurations (remote-call opt-in, disabled polling, disabled Firebase mode) built all six parent/child unsigned release APKs with full lint, matching compiled authority/version settings and test-code exclusion. Evidence: `.cache/configured-releases-1ac0be1f07bf46778db503376c065b57/configured-builds.json`. This checkpoint predates the final parent feedback/error-parser refinement; the child production sources were unchanged afterward.
- Rebuilt the affected parent release after the final feedback and strict-error changes. Assembly and full release lint pass, its production source snapshot remained unchanged during the build, and final APK inspection excludes test probes, synthetic device helpers and ephemeral acceptance trust. Snapshot: `.cache/parent-release-checkpoint-20261026-24343d49/parent-release.json`. The initial checkpoint assertion incorrectly expected the child-only rollout flag in parent BuildConfig; corrected verification checks the parent deployment fields and the matched child rollout gate. No redundant rebuild was needed after that verification-only correction.
- All artifacts use public reserved-host fixtures and remain unsigned/non-production. Operator origin/trust/update authority, production signing, deployment and real-device acceptance are still required.

## Parent editing workflows and persistent request feedback - 26 October 2026

- Extended the production ParentActivity HTTPS probe with a server-only synthetic enrolled device whose ephemeral memory key is never installed on a child. The screens create/remove an explicit UTC weekly schedule, save/restore internet-pause intent, show pending confirmation and unknown capabilities, save a primary contact, reject a stale concurrent contact edit without overwriting newer server state, and show a queued lock intent in command history. No child receives or executes that policy/command.
- Fixed policy/lock request feedback disappearing when asynchronous device-page loading completed. The message is now displayed after the page data renders, preserving the distinction between saved/queued intent and confirmed child behavior.
- Hardened definitive-call-error parsing against duplicate JSON keys and trailing data; Android regressions confirm those responses retain the saved retry identity. The corrected full 13-stage HTTPS suite and cleanup pass: `.cache/android-server-https-reports/e04c520bb8d641deaa5332ef2f036061.json`, `logs/build/2026-10-26.txt`. The earlier expanded run reached logout but timed out after tapping while the final device list was still loading; it is excluded. The passing probe waits for the populated list before sign-out.
- Rebuilt and independently verified the current matched Windows/Linux server/tool bundle: `.cache/server-checkpoint-20261026-7963355f` (8 binaries, 125 source rows, 12 modules, 13 license notices). No deployment or database migration was performed.

## Non-loopback VPN acceptance boundary - 26 October 2026

- Replaced the misleading loopback packet target with fixed emulator host gateway `10.0.2.2` for both selected and management UIDs, versioned `ipv4-host-gateway-v2` / `uid-tcp-host-v2`. Removed the unused ADB reverse mapping, so loopback reachability cannot satisfy this proof.
- The guest can ping the gateway, but baseline raw TCP to the bounded host fixture times out before any pause policy is applied. Repeating with fixture and headless AVD launched outside the sandbox gives the same result. Consent/setup and cleanup pass; no packet-blocking, management-network preservation or physical acceptance is claimed. Logs: `logs/build/2026-10-26.txt`.
- Pinned the base acceptance APK debug signing home to ignored `.tooling/network-signing-home` (override with `-DebugUserHome`) using the existing emulator-only debug identity. This fixes sandbox/unsandboxed Gradle user-home signing drift without uninstalling, replacing signing identity or clearing app data. A prior mismatched-signer install was refused as intended. The physical parent phone was untouched.

## Recover rejected parent call requests - 26 October 2026

- Fixed a parent recovery gap where an explicitly rejected call request retained its pending retry identity and blocked requests for later calls. Only bounded, single-field `409 call_not_current` or `409 call_state_conflict` responses release an exactly matching key/call/action record. Transport failures, malformed/oversized/unknown errors, idempotency conflicts and rollout-disabled responses retain the identity.
- Android client instrumentation verifies error parsing, matching-record deletion, mismatched-key refusal, retained ambiguous records and server-error isolation. A Go regression verifies state rejections leave no command, committed requests replay after call removal, and rollout disable can hide an existing committed request (so that response must remain ambiguous).
- The full 13-stage server/Java/lint/Android/parent-UI HTTPS acceptance and fixture cleanup pass: `.cache/android-server-https-reports/73f916af6804453793aed2a4546b69d8.json`, `logs/build/2026-10-26.txt`. Actual SIM/remote call execution remains unverified; production remote calls remain disabled by default.

## Parent screens over live HTTPS - 26 October 2026

- Added a side-by-side acceptance-only ParentActivity instrumentation probe. It exercises incorrect-password rejection, real sign-in, cleared password fields, device-list rendering, saved-session reopening, revoked-device refusal, fresh pairing-code display, sign-out and signed-out reopening. It uses the production activity/client/vault with ephemeral fixture CA/SPKI trust, and cleans its session/key without touching the physical parent phone.
- The combined 13-stage suite and outer fixture cleanup pass: `.cache/android-server-https-reports/d989871e5d0a4ddcb697f6847039b39c.json`, `logs/build/2026-10-26.txt`. A separate required `server_https_parent_ui_probe=passed` marker guards the screen coverage. This is same-process activity reopening, not process death/reboot or physical-phone acceptance. Active-device policy/contact editing and runtime call-watch screens remain separate checks.

## Contact-service failure and recovery - 26 October 2026

- Added fixed fault controls only to the disposable loopback HTTPS proxy, with Go isolation/recovery tests included in the acceptance build. A child contact fetch can return 503 while command, policy and parent contact-update routes still reach the server. Production server behavior is unchanged.
- The Android probe verifies that production CommandSync acknowledges a safely declined command and fetches policy during the contact failure, preserves the exact prior encrypted contact values and readable version, then receives the replacement contact after recovery. A separate required `server_https_contact_recovery_probe=passed` marker prevents older APKs satisfying this acceptance check. Cleanup resets an enabled fault; an explicit non-owner guard precedes any test lock command.
- The full 12-stage integration suite, proxy tests and HTTPS fixture cleanup pass: `.cache/integration-31c52415106e41ebb5de4dce158129e1/result.json`, `.cache/android-server-https-reports/67652bcd119f49ff84a944423362f27c.json`, `logs/build/2026-10-26.txt`. No physical phone or call was used. This proves contact-route service failure recovery, not full-server downtime, network switching or physical-device behavior.

## Live contact synchronization - 26 October 2026

- Extended the guarded HTTPS acceptance probe to create, replace and clear synthetic parent contacts through the server API. Production `CommandSync` and new `CallContactStore` instances verify encrypted delivery at versions 1, 2 and 3, including changed label/number and an empty final list. Contact fixture preferences are isolated and deleted; existing contact key/cache state is preserved. No call is placed.
- Added the required `server_https_contact_sync_probe=passed` result to the child probe and integration runner, so older test APKs cannot satisfy this check. The full 12-stage integration suite and outer HTTPS fixture cleanup pass: `.cache/integration-c5b221d835e041bea5cdbd464ab7c753/result.json`, `.cache/android-server-https-reports/b7c094f3298f4c3b94dba6878016c328.json`, `logs/build/2026-10-26.txt`.
- Parent editing UI, local contact selection, SIM dialing, physical-device delivery and production trust remain open. The physical parent phone was not used.

## Integration checkpoint reconciliation - 26 October 2026

- Re-ran the local integration suite: all eight stages passed, covering the full Go server suite, six Java fixture groups and debug lint for both Android apps. Report: `.cache/integration-e9b4dc817f9742d9b0c7767596a28885/result.json`; commands and outputs: `logs/build/2026-10-26.txt`. This run did not request emulator instrumentation or touch the physical parent phone.
- Updated integration and command-channel documentation to match the previously passing live HTTPS checkpoint: production EnrollmentClient, AndroidKeyStore authentication and CommandSync refusal/reporting are verified on the isolated non-owner AVD. Successful Device Owner controls, UI flows, production trust and physical-device acceptance remain open.

## Integration harness and parent-phone boundary - 25 October 2026

- Added `scripts/test-integration.ps1` and `docs/INTEGRATION.md`: separate logged stages, failure stops, persistent JSON stage reports, server tests, existing Java fixture runners, Android lint, and optional guarded emulator component probes. This is combined prerequisite verification; it does not claim live Android-to-server acceptance.
- Corrected combined run passed all 12 stages: full server suite, six Java fixture groups, both-app debug lint, isolated AVD guard, parent client/session and child contact-cache instrumentation. Report: `.cache/integration-b373eba2296a4d20b66c6738f71155fb/result.json`; commands and outputs: `logs/build/2026-10-25.txt`. Initial subprocess quoting and incorrect JUnit invocation failures remain recorded; the passing runner uses the repository's existing executable Java checks.
- Separate live Android-to-server HTTPS acceptance passed on 25 October 2026: the production child `EnrollmentClient` enrolled with a parent-created code, rejected code reuse, and authenticated an AndroidKeyStore key; `ParentClient` login, refresh, owner device listing, cross-account denial and logout passed. Parent-triggered revocation also invalidated the enrolled device token for subsequent identity and command requests. Production CommandSync fetched the test command and policy, safely declined Device Owner-only execution on the non-owner AVD, acknowledged failure, and reported device_owner_required before parent revocation. The `.acceptance` apps updated in place and test key/preferences and server fixture were cleaned. Evidence: `.cache/android-server-https-reports/d976e72d6d1b4f9298ae1af23cfc63a4.json`, `logs/build/2026-10-25.txt`. Physical phone, production trust, successful Device Owner command/policy application, UI flows, Firebase and release behavior remain unverified.
- Expanded `TestEnrolledDeviceCommandTransport` to run both handler and actual local HTTPS transport. Both pass enrollment key proof, account isolation, device authentication, command delivery and acknowledgement, duplicate/conflicting acknowledgement, and revocation. Parent authentication in this scenario remains a test double; Android Keystore and platform trust are separate checks.
- Physical parent phone authorized and read-only inspected: Android 14/API 34; parent package absent. User specifies parent app only and forbids reset or data deletion. No application was installed/launched and no device settings, policy, dialer role or user data were changed. Child tests remain isolated to the acceptance AVD.

## Already-current audio output feedback - 25 October 2026

- Added versioned synthetic audio UI/endpoint checks and reproduced a modern routing feedback bug: the emulator already reported Speaker (current endpoint type 4), but reselecting it created a redundant request and later showed unconfirmed feedback.
- Fixed `ManagedInCallService.requestAudio`: when no request is pending and the exact selected endpoint is already current, clear stale feedback/generation and succeed without waiting for a new endpoint callback. A pending request still goes through Telecom so newer selections can override it. Android 13 legacy routing is unchanged.
- Child debug/test/release builds and full release lint pass. `endpoint-feedback-v1` passes actual shortcut and picker selections, observed Speaker and cleared pending feedback. Earpiece is explicitly reported unavailable on this AVD; physical sound, earpiece/wired/Bluetooth, legacy/API matrix and Redmi remain open. Fixed-account cleanup and original role/Telecom restoration pass.
- Prepared and built a separate stdlib-only bounded loopback `tools/network-fixture` endpoint for forthcoming per-UID internet-pause acceptance. No network listener or VPN packet proof is claimed by this entry.
- Logs: `logs/build/2026-10-25.txt`. These production-source fixes still require later integrated release reproducibility recheck.

## Child dialer crash fallback - 25 October 2026

- Added an explicit local **Open Android call controls** button when the child has no live call entry, with READ_PHONE_STATE checks, a local permission flow and Telecom.showInCallScreen delegation. No automatic call action or role change. Current child debug/test/release builds and full release lint pass using fixture trust.
- Built test-only `TelecomRecoveryProbe` and guarded runner. A bounded arming instrumentation emits status while staying alive; the runner validates and SIGKILLs that exact child PID, checks real framework bindings, and always cleans the fixed synthetic account and restores the original dialer. Initial harness issues (instrumentation finishing before kill, active-file sharing, cleanup order) were corrected; failures remain logged.
- `process-recovery-v1` passes via `preloaded-ui-and-explicit-next-call-requalification`: preloaded Google in-call binding survives actual child SIGKILL; stale local/remote IDs are refused while the call remains active; the new local button opens the actual preloaded UI and its End control disconnects the fixture. After the old call fully ends and normal role qualification is explicitly repeated, the next call reaches the child with a fresh ID and local rejection succeeds. Cleanup removes the fixed account and restores the original role/Telecom dialer. Evidence: `.cache/telecom-recovery-077d611af5824872842c97326a4398f2`, `logs/build/2026-10-25.txt`.
- Automatic child rebind, mid-call handoff and automatic next-call return were not observed and are not claimed. Current-build normal role/empty dial-pad and the full prior `ui-controls-v1` synthetic answer/hold/resume/mute/end/reject/safety-gate regression also pass after the UI change.
- No real SIM/outgoing/emergency call or media, physical phone, production authority or deployment was involved. The earlier release reproducibility hashes predate this production UI change; they are historical until a later integrated recheck.

## Copilot backup-task verifier review - 25 October 2026

- Reviewed local verifier/fixtures/docs. Fixed extra-action acceptance (one Exec is insufficient if another action exists), duplicate principal/action fields, missing XML/manifest schedule comparison, root/ancestor reparse rejection, strict boolean registration state and bounded XML parsing.
- Corrected fixture relocation: every copied test now updates its synthetic absolute launcher path/hash and passes a valid baseline before its targeted tamper. Corrected DTD placement after the XML declaration, so refusal tests prohibited DTDs rather than invalid declaration ordering. Repeated and daily-only positive cases pass along with expanded tamper cases.
- Reviewed verifier accepts the pre-existing cached export `.cache/backup-task-fixture-e45d8d88a33a4c2f91fe6eb4f26960e6/bundle`. No task registration, launcher execution, database/key contents, ACL or deployment changes. Logs: `logs/build/2026-10-25.txt`.
- User asked for a larger Copilot task to accelerate delivery; assigned end-to-end optional parent Firebase incoming-call push across parent app/server, with privacy/ownership/freshness/outbox tests and no child/shared-instrumentation edits. Work and live acceptance are not yet claimed complete.

## Android persistence follow-through - 25 October 2026

- Added versioned contact-cache persistence instrumentation and phase runner. Current child/test APKs build and update in place to version 4 on the isolated, unenrolled AVD. Real force-stop with confirmed absent process, a new-PID restart and a real boot-count-increasing reboot all pass. Ciphertext digest, non-exportable key and version-11 floor survive; rollback/conflict refusals preserve state. Explicit fixture/key cleanup passes; production enrollment/contact preferences are untouched.
- Added and passed `replay-persistence-v1`: the production update replay floor survives a confirmed force-stop/new-PID restart and real boot-count-increasing reboot. Sequence/content hashes persist, identical signed retry works, old sequence and same-sequence conflicting payloads are refused, another authority binding fails closed, and cleanup passes. Uses isolated preferences and public signed fixtures; ephemeral Ed25519 private keys stay only in preparation memory. No download/installer/production authority is involved.
- User reports Copilot completed dependency inventory/notice copying and logged server/QR/bundle/hash checks, pushed as `c56074a`. Upstream releases were not independently fetched. This commit is recorded as user-reported, not locally verified Git evidence. Suggested the separate read-only server-bundle verifier as Copilot's next task; Android lifecycle work remains separate.
- Logs: `logs/build/2026-10-25.txt`. No physical phone reset, provisioning, real call, production authority change or deployment.

## Copilot server-bundle verifier review - 25 October 2026

- Reviewed local Copilot verifier/test/docs changes, reported by the user as not yet pushed. Found and fixed missing bundle-root/ancestor junction checks and trailing-dot/space/reserved Windows path rejection. Reserved metadata paths so they cannot also count as artifacts/notices.
- Added fixtures for root, ancestor and nested junctions, Windows aliases/reserved names and metadata overlap. Added explicit workspace bounds before recursive fixture cleanup; junction nodes are removed without traversing targets first.
- Expanded fixture suite passes. The reviewed verifier accepts `.cache/license-review-bundle`: 4 artifacts, 122 source rows, 11 module records and 12 notices. It verifies metadata/hashes only and never executes the bundled programs. Logs: `logs/build/2026-10-25.txt`.
- Gave Copilot the independent next task of offline provisioning-QR binary/notice bundling. Android lifecycle/recovery work remains separate. No changes pushed or deployed by this session.

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

## APK inspection ï¿½ 24 October 2026
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
