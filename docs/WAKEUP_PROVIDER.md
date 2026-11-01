# Replaceable wake-up provider

`control.NewServiceWithWakeup` accepts a provider with only
`Wake(context.Context, deviceID) error`. Newly created commands and successful
policy changes invoke it after their database transaction commits. Duplicate
idempotent command creation does not send another hint. Provider failure does
not fail, roll back, or mark successful the persisted action. Contexts are
bounded to ten seconds. The ordinary `NewService` constructor keeps wakeups
disabled. The server executable enables the provider only when both private
file settings described below are supplied.

`wakeup.NewFCM` implements this boundary with the
[FCM HTTP v1 send method](https://firebase.google.com/docs/reference/fcm/rest/v1/projects.messages/send).
Its request targets the fixed HTTPS FCM endpoint for a validated project ID.
It accepts injected sources for a short-lived OAuth access token and the
device's FCM registration token. The access token belongs in the Authorization
header; the registration token is necessary message routing metadata. The
[FCM Message contract](https://firebase.google.com/docs/reference/fcm/rest/v1/projects.messages)
supports a data object, which this adapter restricts to exactly one field:
`syncHint`, a fresh random 128-bit nonce encoded as base64url.

No device ID, command ID, command type, payload, policy, call information,
application bearer session, refresh token, or credential is placed in the data
object. There is no notification text. Receivers must treat a hint only as a
request to authenticate and synchronize over our own HTTPS channel. It is not
an execution instruction or evidence of delivery success.

The HTTP client validates certificates by default, rejects redirects, and has
a ten-second timeout. Errors do not expose source diagnostics or response
bodies. `server/internal/wakeup/fcm_test.go` checks the exact JSON shape,
fresh nonces, routing separation, authentication, endpoint, bounded context,
redirect refusal, and error privacy. Control tests verify that a provider can
read committed state and that its failure cannot lose commands or policies.

## Optional server configuration

Supply both environment settings, or neither:

| Setting | Private input |
|---|---|
| `APP_CONTROL_FCM_SERVICE_ACCOUNT_FILE` | Operator-controlled Google service-account JSON |
| `APP_CONTROL_FCM_TOKEN_KEY_FILE` | Separate file containing exactly 32 random binary AES key bytes |

The server validates these files locally at startup. Partial settings fail
configuration. It derives the FCM project from the validated account and never
accepts an OAuth endpoint override from credential JSON. Credential parse/read
errors are fixed summaries and do not expose private contents. Default startup
does not read credentials or contact Google. Loading complete valid settings
does not itself perform an OAuth exchange; an actual post-commit hint does.

`ServiceAccount` follows the documented
[Google service-account JWT exchange](https://developers.google.com/identity/protocols/oauth2/service-account)
with RS256, a fixed token audience, a one-hour assertion and only the
[Firebase messaging scope](https://firebase.google.com/docs/cloud-messaging/send/v1-api).
Private keys must be single PKCS#8 RSA keys of at least 2048 bits. The HTTP
exchange is bounded, refuses redirects and never exposes response bodies through
errors. OAuth tokens remain in memory and refresh before expiry under a shared
lock. Keep the server clock correct. Credential/key rotation currently requires
an operator-controlled server restart; live IAM revocation/rotation remains an
acceptance exercise.

Migration 15 adds encrypted registration routing. With a provider configured,
signed device sessions may `PUT /v1/device/wakeup-registration` with exactly a
`registrationToken` field, or `DELETE` that route to remove their registration.
No client-supplied device ID or parent credential can select another device's
token. Registration authorization is checked inside the database transaction.
AES-GCM binds ciphertext to the device identity. Revocation deletes routing
metadata in the same database transaction. There is no HTTP endpoint to read a
registration token, and tokens are excluded from request logs and audit payloads.

Use a distinct protected routing-encryption key, retained separately from the
database and encrypted backups. The existing `control-admin keygen` can create
a fresh 32-byte key file through the normal logged workflow. Losing or replacing
this key prevents old routing tokens from being decrypted; device re-registration
is needed. Disabling FCM settings preserves the durable command and periodic-sync
paths. A failed hint still never rolls back a command/policy or confirms execution.

Tests verify the JWT signature and claims, fixed endpoint/scope, concurrent token
caching, near-expiry refresh, cancellation, malformed credentials/responses,
redirect refusal and error privacy. Registry/HTTP tests cover encrypted storage,
device-bound ciphertext replay rejection, rotation, revocation cleanup,
device-session authorization, disabled endpoints and secret-free request logs.
Configuration tests use newly generated isolated fixture credentials and make
no Google request. The full Go suite and vet pass with these additions.

The child has an opt-in Firebase adapter. Set `appControlFirebase=true` and pass
the validated public `appControlFirebaseApplicationId`,
`appControlFirebaseApiKey`, `appControlFirebaseProjectId`, and
`appControlFirebaseSenderId` Gradle properties to create `fcmDebug` and
`fcmRelease` variants. Ordinary `debug` and `release` variants remain polling-only
and do not include Firebase SDK code. The Firebase callbacks accept only the
canonical opaque `syncHint`, then request the existing network-constrained sync;
token registration uses the authenticated device session and the existing
server-origin gate.

## Fixture and build evidence

The following are local fixture/build checks only:

- Polling APK compilation, Firebase-variant configuration validation and
  opaque-hint rejection checks pass.
- The `fcmDebug` and unsigned `fcmRelease` builds pass and include the Firebase
  application and messaging service. Ordinary polling variants do not include
  Firebase SDK code.
- The 25 October exported-configuration matrix passes `lintFcmRelease` and
  assembly with synthetic Firebase settings. APK manifest/DEX inspection
  confirms the Firebase service/SDK is present only in the Firebase child APK,
  not the parent or polling releases. Compiled public settings match the
  synthetic export. See [configuration evidence](PRODUCTION_CONFIGURATION.md).

These checks used fixture Gradle properties, not a configured test Firebase
project. They establish compilation, packaging and local contract/configuration
behavior only. They did not authenticate to Firebase, obtain a live registration
token, send a message or observe a device receiving one. Successful build tasks
are not Firebase service or device-delivery evidence.

## Live Firebase and device delivery: unverified

No live Firebase project configuration or device-delivery evidence has been
verified. A real test project with only Cloud Messaging enabled, suitable
operator IAM configuration, and an authorized test device are still required
to verify project registration, token registration, OAuth/IAM behavior,
message delivery and the subsequent authenticated sync. No live message was
sent and no real Firebase credentials were read during the checks above.
Keep live-delivery and authenticated-sync acceptance open until those checks
have device evidence. The child retains foreground/background reconciliation
as the transport fallback.

The server implementation adds no external Go dependencies. Android SDK
download and live project/device evidence remain external prerequisites.
