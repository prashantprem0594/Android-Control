# Isolated Android tests

## 25 October current-build checks

The debug child and instrumentation APKs were updated in place to version 3
without uninstall or data clearing. `sealed-cache-v1` passes the production
encrypted contact cache against real Android Keystore using isolated fixture
preferences; see [contact evidence](CALL_CONTACTS.md). Normal dialer role
qualification, both empty ACTION_DIAL paths and the keypad UI pass, followed
by the synthetic `ui-controls-v1` Telecom call exercise. Answer, hold/resume,
mute/unmute, end/reject, remote safety gates and cleanup pass; the original role
and Telecom dialer are restored. No actual SIM, emergency or outgoing call was
placed. Logs are in `logs/build/2026-10-25.txt`.

The first boot wait reported a timeout because the test ADB daemon had started
without the workspace emulator key and listed this AVD as unauthorized. With
only the isolated AVD on port 5038, restarting that daemon with `ADB_VENDOR_KEYS`
set to `.tooling/emulator-home/adbkey` restored authorization and the normal boot
probe passed. `wait-test-emulator.ps1` supplies this key when starting the daemon;
an already-running daemon retains its earlier key configuration. The physical
device daemon on port 5037 was left untouched.

An Android 37 Google Play x86_64 AVD now runs from workspace `.tooling`, using
the installed system image and WHPX. It is named `AppControlAcceptance`, uses
console port 5580 and guest ADB port 5581, and has no visible window. The image
requires 4 GiB guest RAM; the emulator raised the initial smaller request to
that amount. This is Android emulator evidence, not Redmi/HyperOS acceptance.

## Launch and connect

Run all commands from the workspace through the logger:

```powershell
& .\scripts\run-logged.ps1 -TaskId P6-002 -Category tooling -Command '& .\scripts\start-test-emulator.ps1'
& .\scripts\run-logged.ps1 -TaskId P6-002 -Category device -Command '& .\scripts\wait-test-emulator.ps1'
```

The launcher refuses an occupied console port and never wipes AVD data. It uses
software graphics, cold boot without snapshots and the emulator's
`-skip-adb-auth` flag for this local test profile. It is not a test of ADB
authorization security. ADB server 5038 uses the workspace emulator key and
connects to `127.0.0.1:5581`; use both `-P 5038` and that explicit serial for
subsequent commands. The isolated server avoids conflicts with the default
daemon. Workspace key contents must never be printed. Launch configuration
follows the [emulator command-line options](https://developer.android.com/studio/run/emulator-commandline).

Build `:child-app:assembleDebug :child-app:assembleDebugAndroidTest` through the
normal logged Gradle procedure, then install both APKs:

```powershell
& .\scripts\run-logged.ps1 -TaskId P1-004 -Category android -Command '& "C:\Users\Sushant\AppData\Local\Android\Sdk\platform-tools\adb.exe" -P 5038 -s 127.0.0.1:5581 install -r ".\android\child-app\build\outputs\apk\debug\child-app-debug.apk"'
& .\scripts\run-logged.ps1 -TaskId P1-004 -Category android -Command '& "C:\Users\Sushant\AppData\Local\Android\Sdk\platform-tools\adb.exe" -P 5038 -s 127.0.0.1:5581 install -r ".\android\child-app\build\outputs\apk\androidTest\debug\child-app-debug-androidTest.apk"'
```

## Production Keystore identity probe

```powershell
& .\scripts\test-android-identity.ps1 -Serial 127.0.0.1:5581 -AdbServerPort 5038
```

The script logs internally. The test calls the production `DeviceIdentity`
through the existing instrumentation runner's identity mode. It creates or
reuses the app's P-256 identity, checks API non-exportability, verifies enrollment
and authentication signatures, and rejects changed-challenge and cross-domain
verification. It neither enrolls with a server nor deletes an existing key.
The result prints only fixed summaries and a public-key SHA-256 fingerprint;
challenge/proof values and private keys are not printed.

Repeat with `-ExpectedFingerprint` set to the first result to check key reuse
across instrumentation processes. For reboot persistence, use the logged ADB
`reboot` command against this exact serial, run `wait-test-emulator.ps1`, then
repeat the probe with that fingerprint. No factory reset is involved.

On 24 October 2026, generation/non-exportability and both signing domains passed
on build `google/sdk_gphone64_x86_64/emu64xa:17/CE2A.260420.051.A2/16418371:user/release-keys`.
The public key remained unchanged across separate instrumentation invocations
and after an ADB reboot followed by a completed Android boot.
See `logs/build/2026-10-24.txt` for actual results and the reboot exercise.
These results do not prove hardware-backed key storage or attestation, Android
HTTPS enrollment, server certificate rotation or the supported Redmi matrix.

To stop only this AVD, first confirm its name through the emulator console,
then issue `emu kill` against `emulator-5580` through the logger. Do not delete
AVD data to stop it; that would discard the test identity.

## Android work-queue lifecycle

The debug APK includes `QueueProbeService`, protected with BIND_JOB_SERVICE,
from `src/debug` only. Its test actions use the production `SyncWorkQueue`
coordinator without network, enrollment, policy or call operations. Install
the matching debug/test APK pair, then run:

```powershell
& .\scripts\test-android-work-queue.ps1 -Serial 127.0.0.1:5581 -AdbServerPort 5038
```

The test verifies a request arriving during a held sync runs in a second pass
without restarting the JobService, a partial result leaves work uncompleted and
Android redelivers it after backoff with an increased delivery count, and an
explicit cancellation invokes onStopJob without completing held work. Test
jobs 4901–4903 are cancelled during cleanup. Explicit cancellation is not a
test of work redelivery: Android cancels that job intentionally.

This probe passed on Android 37 on 24 October 2026. The first invocation failed
but its preceding diagnostic output was lost by the command logger. The logger
was fixed to preserve incremental output before exceptions; a subsequent probe
completed all stages, followed by a second successful run. The original failure remains in the dated log and is
not rewritten or treated as successful. This is runtime evidence for the
fixture/coordinator, not the production authentication/policy path, constrained
network loss, process death, reboot command delivery or Redmi scheduling.

## Android request cancellation and stalled TLS

```powershell
& .\scripts\test-android-request-control.ps1 -Serial 127.0.0.1:5581 -AdbServerPort 5038
```

This probe reuses the dependency-free RequestControlChecks and
StalledTransportChecks in the Android test APK, exercising the production
request controller and Android HttpsURLConnection. It passed on the same
Android 37 image on 24 October 2026. Checks cover cancellation, deadline,
single disconnect, normal close cleanup, disconnect failure, and a real TLS
client connecting to a loopback peer that never completes the handshake.
Both deadline and cancellation cases fail without reporting success and clean
up their fixture workers. Finite socket timeouts remain enabled as a fallback;
the test does not assert immediate interruption.

The local peer sends no certificate or application response. No credentials,
certificate bypass or alternative trust anchors are used. This is not a live
server-trust rotation test, mobile-network switching test, stalled request-body
test, or a result for Redmi/HyperOS. The shared JVM checks are added only to the
androidTest source set and are not production application classes.

## Android journal process death

```powershell
& .\scripts\test-android-journal-process.ps1 -Serial 127.0.0.1:5581 -AdbServerPort 5038
```

The debug-only, non-exported JournalProbeService runs in the app's separate
`:journal_probe` process. The instrumentation client communicates through
[Messenger IPC](https://developer.android.com/develop/background-work/services/bound-services)
and observes binder death. Replies verify that restarted workers have different
process IDs. The service uses the production CommandExecutor with synchronous
SharedPreferences commits in `fixture-process-journal`, not the production
command journal. Its harmless action only increments a persisted fixture
counter; it performs no lock, package, VPN, call or network action.

The probe passed on Android 37 on 24 October 2026:

- A completed result survives process death; redelivery returns succeeded with
  one action, without executing again.
- Death immediately after committing the reservation recovers as failed with
  zero actions.
- Death after the action counter is committed but before the result is saved
  recovers as failed/unconfirmed with exactly one action, without repetition.

Successful runs clear their private fixture preferences and unbind the service.
No production process is intentionally killed. This is process-death journal
evidence, not a guarantee of exactly-once physical execution or proof of device
locking, server acknowledgements, reboot command delivery or Redmi behavior.

## Device Owner and production package-policy adapter

The isolated AVD was provisioned as Device Owner on 24 October 2026 using
`com.appcontrol.child/com.appcontrol.child.admin.AppControlDeviceAdminReceiver`.
The AVD name was confirmed before the logged provisioning command. No factory
reset or physical-device provisioning was performed. Device Owner remains
enabled on this test AVD for further acceptance work.

The `policy-fixture` Gradle module builds a disposable application,
`org.example.appcontrolfixture`, with no permissions, services or user data.
Build `:policy-fixture:assembleDebug` with the normal logged Gradle procedure
and install its `build/outputs/apk/debug/policy-fixture-debug.apk` through ADB
server 5038 at serial `127.0.0.1:5581`. Install the matching child test APK, then:

```powershell
& .\scripts\test-android-package-policy.ps1 -Serial 127.0.0.1:5581 -AdbServerPort 5038
```

The probe requires the emulator fingerprint, no enrollment, no existing desired
policy or tracked restrictions, Device Owner and the installed fixture. It uses
the production PolicyEnforcer, cached-policy parser, reconciliation ledger and
DevicePolicyManager adapter. The successful Android 37 run verifies suspension
through both DPM and PackageManager, reapplication of the same policy,
always-allowed precedence restoring the fixture, and parser rejection of a
management-app blocking target. The fixture is unsuspended before baseline
cache/ledger preferences are restored. Cleanup failure makes the probe fail
and retains reconciliation state instead of hiding pending restrictions.

The first test expected a partial protected-package result, but the production
parser rejects management package names earlier. That expectation was corrected
without changing product behavior; cleanup succeeded after the failed test and
the corrected probe passed. Logs preserve both results. This is emulator API
evidence; physical Redmi app launch, reboot reconciliation, schedule boundary
timing and management-server delivery are still separate acceptance checks.

## Cached-policy reboot reconciliation

On 24 October 2026, the production boot receiver/enforcer passed an actual
emulator reboot cycle. PolicyEnforcer records the local Android boot count
alongside its attempted version/result, using -1 if the diagnostic is unavailable.
This local field is not added to server reports.

Preparation requires this unenrolled Device Owner AVD and an empty baseline
cache/ledger apart from optional alarm/empty-tracked metadata. It saves and
applies a fixture blocking policy, then deliberately unsuspends the disposable
app without removing the cached policy/ledger. After reboot, the production
boot path re-suspends it. A newer boot count and applied version are checked
before verification instrumentation starts, so the verifier cannot produce
the reconciliation evidence itself.

With matching APKs and the disposable app installed:

```powershell
& .\scripts\test-android-policy-reboot.ps1 -Phase Prepare -Serial 127.0.0.1:5581 -AdbServerPort 5038
# Through run-logged.ps1: adb -P 5038 -s 127.0.0.1:5581 reboot
& .\scripts\run-logged.ps1 -TaskId P6-001 -Category device -Command '& .\scripts\wait-test-emulator.ps1'
& .\scripts\run-logged.ps1 -TaskId P6-001 -Category device -Command '& .\scripts\wait-android-policy-reboot.ps1 -Serial 127.0.0.1:5581 -AdbServerPort 5038'
& .\scripts\test-android-policy-reboot.ps1 -Phase Verify -Serial 127.0.0.1:5581 -AdbServerPort 5038
```

Keep preparation and verification together. Verification checks the new boot,
cached version/result, DPM suspension and ledger, then restores the fixture via
allowlist precedence and restores baseline preferences. The wait script reads
fixture metadata/local result fields through debug run-as and prints fixed
summaries, never raw policy XML. `-Phase Cleanup` releases a recognized pending
fixture after an interrupted exercise. No factory reset or physical-device
reboot is involved.

This verifies cached package-policy reboot recovery on this Android 37 AVD.
Enrolled server delivery/acknowledgements, schedule boundary timing, mobile
network recovery and Redmi/HyperOS coverage remain open.

## Real Android alarm boundary evidence (24 October 2026)

scripts/test-android-schedule-boundary.ps1 runs a test-APK-only probe on the isolated, unenrolled Device Owner Android 37 AVD. Exact alarm access is required. The initial run refused the default permission state without changing the policy. After temporary test access was granted, a UTC rule for the disposable fixture crossed actual start and end wall-clock boundaries. Only initial setup called PolicyEnforcer: subsequent suspension/restoration and changed effective-state keys came from the production alarm path. Both DPM and PackageManager observations passed within a 45-second tolerance. The fixture and cached preferences were restored, and the original alarm access mode was restored separately through logged ADB. No synthetic broadcast or device clock change was used. This proves the exact-alarm path on this active emulator; Redmi, idle/inexact timing, enrolled delivery and offline VPN remain open.

Run through the internally logged runner with -Serial 127.0.0.1:5581 -AdbServerPort 5038 after building/installing the matching child test APK. The probe refuses enrolled devices, nonmatching emulator fingerprints, nonempty desired policies/ledgers, missing Device Owner or missing exact-alarm access. Interrupted instrumentation may leave its fixture policy; inspect and recover before reuse.

## Same-key package upgrade evidence (24 October 2026)

The isolated Android 37 Device Owner AVD passed a real debug-signed child upgrade from versionCode 1 to 2 (0.1.1-fixture). PolicyUpgradeProbe prepares cached version 301, deliberately unsuspends the disposable fixture, and records the public Keystore identity and boot count. After adb install -r, wait-android-policy-upgrade.ps1 observed production package-replaced reconciliation and suspension before verification instrumentation started. Verification confirmed the higher package version, unchanged boot, retained Device Owner, identical Keystore public key, cached policy/ledger and suspension. Cleanup restored the fixture and baseline preferences. A version-1 replacement was refused with INSTALL_FAILED_VERSION_DOWNGRADE; no downgrade override was used. The emulator child remains installed at versionCode 2. Future debug installs must use appControlVersionCode=2 or higher with appControlVersionName supplied too.

This is same-debug-signing-key emulator evidence, not production signing, live enrolled server continuity, remote update delivery or successful rollback. PolicyUpgradeProbe supports Prepare/Verify/Cleanup through scripts/test-android-policy-upgrade.ps1. Keep phases together on the guarded unenrolled AVD; the wait script observes production state and never applies policy. Failed cleanup retains reconciliation state.

## Android role and empty dial-pad evidence (24 October 2026)

scripts/test-android-dialer-role.ps1 passed on the isolated unenrolled Android 37 AVD. Android accepted the child as ROLE_DIALER through normal qualification, and Telecom reported it as the default. Both ACTION_DIAL with no data and with an empty tel scheme launched the child DialActivity. UI hierarchy checks found the phone header, keypad, Call and Delete controls. No number was supplied and no Call button was pressed. The script never bypasses qualification, registers phone accounts or places calls. Its finally block restored the original role holder; a separate logged read confirmed that the role and Telecom default agree afterward. Physical incoming SIM, locked-screen call UI, audio routing, emergency and crash-recovery acceptance remain open.

Synthetic Telecom core evidence (24 October 2026): an isolated debug-only SIP ConnectionService fixture with no network/audio permissions supplied controlled incoming calls to Android Telecom. The production child InCallService observed ringing, active and removed calls; local answer/end/reject and fresh call IDs passed. Internal remote SIP actions, expired/deadline-invalid/missing targets and stale IDs were rejected. The fixed managed account was unregistered and the original default dialer/Telecom role restored. No SIM/outgoing/emergency call or media stream was used. Expanded screen-button verification subsequently passed with the mandatory ui-controls-v1 proof: actual incoming Answer audio, active Hold call, Resume, Mute/Unmute, End and next-call Reject buttons produced the expected Telecom callbacks and screen states. Cleanup restored the account and original dialer. A prior compile failure and stale-APK core rerun remain in logs and are excluded from this UI proof. Real SIM, actual sound, locked-screen, emergency and Redmi coverage remain open.
