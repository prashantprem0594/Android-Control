# P1-004 enrollment protocol and verification

26 October: child enrollment/manual sync supports explicit Android 17 local-network consent for home servers. The setup screen uses FLAG_SECURE and pairing/server/name fields disable saved instance state. Gateway HTTPS acceptance passes real EnrollmentClient/AndroidKeyStore authentication; physical Device Owner provisioning remains separate.

The active workspace is `C:\Android Control`. Historical logs and earlier documentation may refer to the original `C:\App Control` location.

## HTTP API

- `POST /v1/pairing/codes`: parent bearer authentication; returns a one-time 128-bit code and expiry.
- `POST /v1/enrollments`: JSON `code`, `displayName`, `publicKey`, `keyAlgorithm`. The public key is an unpadded base64url X.509 SubjectPublicKeyInfo containing ECDSA P-256. Algorithm is `ECDSA_P256_SHA256`.
- `POST /v1/enrollments/complete`: JSON `enrollmentId`, `challenge`, `signature`; returns the active device only after proof verification.
- `GET /v1/devices/{deviceID}`: parent bearer authentication; other parents receive 404.

The signature is ASN.1 DER ECDSA over SHA-256 of UTF-8 `app-control-device-enrollment-v1\n`, enrollment ID, a newline, and the decoded 32-byte challenge. Challenge and signature fields use unpadded base64url. The pairing code is consumed when enrollment starts; a failed or interrupted attempt can require a fresh code. Completion replay is rejected.

The child uses a non-exportable AndroidKeyStore key, HTTPS with platform certificate validation, bounded responses, connection timeouts, and disabled redirects. It persists only the server origin and confirmed device ID in app-private preferences. Pairing codes and challenges are never persisted. Backup is disabled. Enrollment metadata is not a device authentication credential; authenticated command transport and revocation remain later tasks.

## Evidence on 23 October 2026

Initial full Go tests passed after formatting the handoff's partial files. After HTTP integration, full Go tests and go vet passed, including an end-to-end HTTP proof exchange, code and proof reuse rejection, missing authentication, parent isolation, strict JSON, and request size limits. Existing service tests verify wrong-key proofs, expiry, and proof attempt limits.

ADB reports no attached devices. Actual AndroidKeyStore signing and end-to-end Android enrollment remain unverified. Both debug APKs built successfully offline using installed Gradle 9.6.0 and in-process Kotlin compilation. The wrapper download was blocked by network permissions; the installed distribution provided the pinned version. Actual device proof remains required before P1-004 can pass.

## Android HTTPS verification on 25 October 2026

The isolated Android 37 acceptance AVD completed the production `EnrollmentClient` flow against a disposable HTTPS server. `ParentClient` created the pairing code; the child acceptance package generated its non-exportable AndroidKeyStore key, enrolled, authenticated, and completed signed device requests. Production CommandSync fetched a test command and policy, declined the Device Owner-only action safely, and reported the actual device_owner_required state. The owning parent's live device listing included the child, reusing the consumed code was rejected, and a second account was denied access. The parent then revoked the child and subsequent device identity and command requests using its existing token were rejected. The live acceptance report and commands are in `.cache/android-server-https-reports/d976e72d6d1b4f9298ae1af23cfc63a4.json` and `logs/build/2026-10-25.txt`.

The acceptance process temporarily installed a test-only TLS socket factory pinned to an ephemeral CA and SPKI, then restored the previous process default. Production CommandSync used this same test-only trust path for its requests. `EnrollmentClient` itself kept its normal platform-default constructor and TLS path. It does not verify production server trust against the public deployment certificate, the enrollment screen, or a physical child phone. CommandSync was exercised on the guarded non-owner AVD; Device Owner command execution and policy application remain unverified. The probe cleared only its fresh acceptance-package enrollment preferences and key alias. No parent-phone state was changed.

## Remaining verification

- Exercise proof replay and wrong-parent rejection from Android; consumed-code replay, parent isolation, and parent-triggered session revocation have passed, while backend negative tests cover proof replay.
- Verify the enrollment UI and production certificate trust configuration against a configured test service.
- Repeat on a dedicated child device when available. The connected physical phone is parent-only and must not be reset, cleared, or provisioned as Device Owner.

Verified build command (from android, through the logging wrapper):

```powershell
$env:GRADLE_USER_HOME = 'C:\Android Control\.cache\gradle'
& 'C:\Gradle\gradle-9.6.0\bin\gradle.bat' :parent-app:assembleDebug :child-app:assembleDebug --offline --no-daemon --no-configuration-cache '-Pkotlin.compiler.execution.strategy=in-process'
```
