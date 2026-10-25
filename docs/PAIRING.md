# P1-004 enrollment protocol and verification

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

## Device verification still required

1. Build both APKs and install the child on a test device or emulator.
2. Use a reachable HTTPS server with a trusted certificate and a test parent account.
3. Create a pairing code through the authenticated endpoint and enter it in the child along with the server origin and device name.
4. Confirm the child reports enrollment confirmed; verify the device is active through the owning parent's endpoint and inaccessible to another parent.
5. Verify the key remains in AndroidKeyStore across process restart and can sign a new test challenge without exporting the private key. Record sanitized results through run-logged.ps1.

Verified build command (from android, through the logging wrapper):

```powershell
$env:GRADLE_USER_HOME = 'C:\Android Control\.cache\gradle'
& 'C:\Gradle\gradle-9.6.0\bin\gradle.bat' :parent-app:assembleDebug :child-app:assembleDebug --offline --no-daemon --no-configuration-cache '-Pkotlin.compiler.execution.strategy=in-process'
```
