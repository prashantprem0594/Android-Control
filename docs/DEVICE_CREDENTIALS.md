# Device credentials and revocation (P1-005)

Implemented routes:

- `POST /v1/device-auth/challenges`: JSON `deviceId`; active devices receive a random challenge valid for one minute. Only one challenge per device is retained; requesting another replaces it.
- `POST /v1/device-auth/sessions`: JSON `deviceId`, `challengeId`, `challenge`, `signature`; returns an opaque bearer token valid for 15 minutes. The server stores only its SHA-256 hash. Issuing a new session invalidates the previous session.
- `GET /v1/device-auth/me`: device bearer authentication; checks the session expiry and persisted active device state on every request.
- `POST /v1/devices/{deviceID}/revoke`: parent bearer authentication, scoped to the owning parent. Revocation changes persistent device state and deletes sessions and authentication challenges in one transaction. Repeated revocation by the owner succeeds; another parent receives 404.

Device authentication uses a separate signature domain from enrollment. Sign SHA-256 over UTF-8 `app-control-device-auth-v1\n`, device ID, newline, challenge ID, newline, and the decoded 32-byte challenge. ECDSA P-256 signatures use ASN.1 DER; challenge and signature use unpadded base64url. A well-formed proof submission consumes its challenge even if the signature is invalid. Expired, consumed, replaced, revoked, and wrong-device proofs are rejected.

The Android enrollment client can request a session with its existing AndroidKeyStore identity. Returned bearer tokens remain in memory; callers must not persist or log them. Use HTTPS and platform certificate validation. Authentication failures require a new challenge; revocation requires deliberate re-enrollment with a new parent-issued code rather than a local status reset.

Automated service and HTTP tests cover wrong-key proofs, replay, challenge replacement and expiry, session expiry and replacement, hashed token storage, parent isolation, immediate request rejection after revocation, idempotence, pending-proof invalidation, and service reinitialization against persisted state.

P1-005 remains in progress. There is no device socket transport yet; later transport work must revalidate persistent device authorization for each privileged operation and terminate revoked connections. Actual AndroidKeyStore authentication still needs device/emulator verification. Rate limiting for public challenge issuance remains part of hardening (P6-003); the current per-device challenge bound is not a rate limiter.

Verification result: both parent and child debug APKs built successfully offline with installed Gradle 9.6.0 and in-process Kotlin compilation on 23 October 2026.
