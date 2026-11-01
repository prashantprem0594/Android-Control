# Sensitive data in logs and notifications

The server and Android clients handle credentials, pairing material, call
metadata, contact details, policies, and local storage paths. Keep those values
out of diagnostic channels:

- HTTP access logs may contain only a recognized route template (or
  `unmatched`), a normalized method, status, response byte count, duration, and
  fixed abort/panic flags. Never log request or response bodies, query values,
  headers, route identifiers, panic values, exception messages, or stack traces.
- API failures use fixed machine-readable error codes. Readiness failures return
  only `unavailable`; internal error details stay out of HTTP responses.
- The server process-level failure log uses a fixed message and category. Do
  not attach wrapped errors: configuration, storage, and transport errors can
  contain local paths or supplied values.
- Audit events are state-change records with fixed actions/reasons and opaque
  actor/target IDs. Do not add emails, passwords, tokens or token hashes,
  pairing codes, keys, signatures, challenges, phone/contact/call details,
  command contents, policy contents, or database paths.
- FCM application data contains exactly one fresh opaque `syncHint`. The
  registration token is encrypted at rest and appears only in the required
  FCM `message.token` routing field; OAuth access tokens belong only in the
  outbound Authorization header. Do not add a notification body, command,
  policy, call, contact, device, or session data. FCM failures must not expose
  provider response bodies or credential-source errors.
- Android diagnostics must use fixed text, not throwable messages or request
  data. Firebase callbacks accept only the opaque hint and schedule sync; they
  do not log or persist the payload.
- Child Telecom notifications show caller details only in the private
  notification. Their lock-screen public version is generic. Parent call-watch
  alerts are generic and private; the app UI may show authorized call/contact
  details after opening.

## Regression coverage

Run `scripts/test-sensitive-data.ps1` through `scripts/run-logged.ps1`. It runs
the full Go test suite, synthetic lock-screen redaction, call-action,
parent-call-watch and wake-up-hint fixtures, and child debug Kotlin
compilation/lint. The Go integration fixture exercises successful
and failed parent/device authentication and enrollment, token refresh,
command delivery and acknowledgement paths, call snapshots/actions, contact
and policy updates, wake-up registration, readiness and server-error responses.
It checks captured HTTP logs, audit output, exact fixed error codes, and
outbound FCM JSON using marker values for credentials, database paths, device
keys, contact/call information, policy values, and routing/OAuth tokens. The
FCM routing marker is expected only in `message.token`; the test requires it
to be absent from `message.data`, which must contain only `syncHint`.

These are local synthetic-fixture checks. They do not contact Firebase, read
real phone/contact data or credentials, or test a production device. Live
Firebase delivery, Android system/lock-screen rendering across OEM versions,
and behavior under production logging/collection infrastructure remain
unverified.
