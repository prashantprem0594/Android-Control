# Android server-trust acceptance record

Status: pending device and controlled HTTPS deployment. Local regression checks
passed on 24 October 2026; these do not establish Android handshake behavior.

Local preparation evidence on 24 October 2026:

- Both app debug APKs and both instrumentation APKs build offline.
- A configured fixture build with `https://trust-fixture.invalid` and two
  distinct public fixture pins builds; these are not deployment credentials
  or a reachable acceptance endpoint.
- APK inspection finds the runner only in test APK DEX files and verifies each
  test manifest's target package.
- Wrapper fixtures reject a failed probe, wrong instrumentation code, missing
  runner and native failure; a complete successful result is accepted.
- A real ADB probe attempt fails because no device is connected. No live TLS
  acceptance row below has been marked passed.

Commands, failures and successful reruns are retained in
`logs/build/2026-10-24.txt`.

Use this record alongside [SERVER_TRUST.md](SERVER_TRUST.md). Run provisioning,
build and verification commands through `scripts/run-logged.ps1` with task
`P6-005`. Record public metadata and sanitized results only. Keep private keys,
credentials and device identifiers out of command output and attachments.

## Test setup

Record the test date, operator, Redmi model, Android version, HyperOS build,
app version, APK SHA-256 and signing-certificate fingerprint. Record the
configured HTTPS origin, current pin A, backup pin B, and the public certificate
validity periods. Use test accounts and test enrollment data.

Prepare independent leaf keys A, B and C. Certificates for A, B and C must each
pass normal system CA validation and hostname validation at the configured
origin. Only A and B are pinned in the APK under test. This makes the C test
evidence of pin enforcement rather than ordinary CA rejection.

Keep the APK unchanged throughout the A-to-B rotation. Keep a second enrolled
device offline before switching to B, if available. Record unavailable scenarios
as pending rather than passed. Have a trusted app update and server rollback
procedure ready before changing the test deployment.

## Execution and evidence

Both apps now include a separate debug instrumentation APK with the shared
`com.appcontrol.transport.ServerTrustInstrumentation` runner. It follows the
[Android instrumentation lifecycle](https://developer.android.com/reference/android/app/Instrumentation).
Build the app and test APKs with the same complete deployment trust properties:

```text
:parent-app:assembleDebug :child-app:assembleDebug
:parent-app:assembleDebugAndroidTest :child-app:assembleDebugAndroidTest
```

Use the logged Gradle build procedure in the handoff. Install the matching app
and test APK on an authorized test device through the command logger. Test APKs
are under each module's `build/outputs/apk/androidTest/debug/`; app APKs are under
`build/outputs/apk/debug/`. The probe refuses unconfigured deployment origins.

With exactly one physical USB device attached, invoke:

```powershell
& .\scripts\test-android-server-trust.ps1 -App child -Expect success
& .\scripts\test-android-server-trust.ps1 -App parent -Expect success
```

Repeat after switching A to B without rebuilding or reinstalling. For each
negative certificate, use `-Expect tls_failure`, then restore B and repeat
`-Expect success`. Each invocation starts a new instrumented process. Run on
a test device with no active call; instrumentation restarts the target app and
does not establish background-sync or active-call continuity.

The runner sends a credential-free request to the compiled origin's `/healthz`
with default platform certificate and hostname validation and no redirects.
Success requires HTTP 200. A negative test requires `SSLHandshakeException`
or `SSLPeerUnverifiedException` (including Android hostname rejection);
DNS, timeout and other failures fail the test. The PowerShell wrapper checks
both the explicit probe result and instrumentation result code, because ADB's
exit code alone does not prove the test passed. Fixture regression checks for
the wrapper are in `scripts/test-android-trust-runner.ps1`.

These probes cover platform trust for both apps independently of their
production network clients. They do not send or prove enrollment, signed
authentication, command acknowledgements or policy reports; complete those
flows separately for the child. No trust-store changes or certificate bypasses
are provided. Use system-CA-trusted test certificates at the configured DNS
origin to distinguish pin rejection from ordinary CA rejection.

For each row record actual behavior, time, device/build, sanitized evidence
location and pass/fail/pending. A successful command requires its device result
in the parent-scoped command history, not merely HTTP acceptance by the server.

| Scenario | Expected observation | Result / evidence |
|---|---|---|
| Server A, fresh enrollment | Enrollment and signed authentication succeed | Pending |
| Server A, enrolled device | Fetch, acknowledgement and policy reporting succeed | Pending |
| Switch server to B, same APK and enrollment | Authentication, fetch, acknowledgement and policy reporting succeed | Pending |
| B after app background/resume | Synchronization succeeds and confirmed state is accurate | Pending |
| B after device reboot | Enrollment persists and synchronization resumes | Pending |
| Device offline during A-to-B switch, then reconnect | Existing A+B APK reconnects to B without reenrollment | Pending |
| CA-trusted same-host certificate C | TLS fails; authenticated route handler receives no application request | Pending |
| Wrong-host certificate | TLS fails; authenticated route handler receives no application request | Pending |
| Expired certificate | TLS fails; authenticated route handler receives no application request | Pending |
| Untrusted certificate | TLS fails; authenticated route handler receives no application request | Pending |
| Renew valid certificate with key B | Synchronization succeeds with the same pin | Pending |
| Restore valid B after negative tests | Existing enrollment synchronizes without reset | Pending |
| Trusted app update from A+B to B+C while server uses B | Enrollment survives and synchronization succeeds | Pending |
| Switch server to C after trusted update | Updated app synchronizes successfully | Pending |

Exercise every certificate failure against an enrolled device as well as the
enrollment form. Correlate test times with sanitized server route logs to verify
that failing handshakes never reach authenticated handlers. Do not log bearer
headers, proof payloads, enrollment codes or request bodies to obtain evidence.
An empty server log alone is insufficient: also record the observed Android TLS
failure and confirm that the same device succeeds against a valid pinned
certificate before and after the negative case.

The parent now has a production HTTPS client and controlled-client tests. Record instrumentation
handshake results separately from live production parent-flow acceptance. Do not
substitute a JVM TLS client for Android platform verification.

## Completion gate

Attach the completed record to `tracking/phase-status.json` under `P6-005`.
That check also requires the database recovery evidence described in
[DATABASE_RECOVERY.md](DATABASE_RECOVERY.md). Keep it in progress until both
operational recovery and the required device rotation scenarios pass.
