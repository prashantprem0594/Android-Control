# Private application updates

Both apps now include private update screens and shared metadata, download,
replay-protection and installer-session code. Entries appear only with compiled
update authority. There is no automatic installation, uninstall, data clearing
or factory-reset operation.

Android 9 compatibility: the child release may declare minimum API 28; the parent remains API 33. Metadata validation and signing/staging tools use those separate floors. Private update verification still requires an available Ed25519 key/signature provider. If the platform lacks it, the update screen disables checking and directs the operator to install a signed update with Android's installer. Signature, signer and replay checks are never bypassed. Enrolled update continuity on the physical child remains unverified.

## Build configuration

Provide the existing HTTPS origin/current/backup TLS pins plus:

| Gradle property | Public value |
|---|---|
| `appControlUpdatePublicKey` | Base64 of the metadata signer's 44-byte Ed25519 DER public key |
| `appControlUpdateCertificateSHA256` | Lowercase SHA-256 of the existing APK signing certificate |
| `appControlUpdateMinimumSequence` | Optional initial replay floor, default zero |

The public key and certificate must be configured together. Update trust also
requires configured HTTPS trust. Empty settings hide the entry. Private
metadata seeds and APK signing keys are never compiled into either app.

The installed signing certificate must match the configured certificate before
networking. The client fetches only `/v1/releases/current` at its compiled
origin, with platform TLS/hostname validation and no redirects. Signed metadata
selects the exact package URL.

## User workflow

1. Open **App updates** and select **Check for update**.
2. Metadata and APK bytes/package/version/SDK/signer are verified before the
   install action is enabled. No newer published version is reported separately
   from preparation failure.
3. Select **Install verified update**, then confirm locally. An ordinary app
   may first need Android's installation permission.
4. Keep the screen open for **Open Android confirmation**. Android controls
   final approval and signature enforcement.
5. Refresh status or explicitly resume/cancel an interrupted session.

Installation requires the same package/current key and a higher version. The
exact bytes copied into Android's session are rehashed; metadata is revalidated
before commit, resume and opening confirmation. Android controls subsequent
confirmation timing. Recent Android versions avoid bringing this screen forward
while the app is hidden; returning and resuming the same session is supported.

An ambiguous commit retains its session identity instead of automatically
creating a second install. A private nonce/session ID match protects result
handling. The nonce is not put in an activity URI or logged. Observed installed
version/certificate can confirm completion after restart. Raw installer error
payloads are never displayed or logged.

Replay state is bound to metadata key, origin and APK signer. A sequence can be
retried only with the same signed payload. Changed content or renewed expiry
requires a higher sequence and immutable artifact paths. Corrupt/changed
authority fails closed. Metadata key rotation requires an explicit trusted
migration; clearing app data is not enrollment-preserving recovery.

## Build, sign and stage

`scripts/test-release-reproducibility.ps1` accepts optional `UpdatePublicKey`,
`UpdateCertificateSHA256` and `UpdateMinimumSequence`. For the Firebase child,
provide `FirebasePublicConfigFile`: UTF-8 JSON with exactly the public string
fields `applicationId`, `apiKey`, `projectId` and `senderId`. This selects
`fcmRelease`; omission selects polling-only release. Do not use a service-account
file. The sender ID must match the application ID's sender component.

After signing both APKs with `scripts/sign-private-apks.ps1`, invoke
`scripts/stage-private-release.ps1` through the logger. Parameters are the signed
APK directory, fresh output directory, canonical HTTPS origin, sequence,
expected APK certificate, and separate metadata private/public key files. The
command verifies copied APK signatures/packages/versions/SDKs, signs metadata,
independently verifies it and emits:

```text
bundle.json
release-payload.json
release-manifest.json
release-public-key.pub
releases/<sequence>/parent-release.apk
releases/<sequence>/child-release.apk
```

It never copies the private key, uploads or deploys. Bundle checks cover
artifacts/metadata, not every embedded client deployment setting: use APKs built
with the intended origin/update authority. Serve only the `releases` subtree at
the immutable signed URLs. Configure the backend's verified metadata endpoint
separately as described in [release metadata](RELEASE_METADATA.md). Never replace
an already published sequence with different content.

## Evidence and remaining acceptance

Parent/child debug compilation passes with the update UI/session code. A focused
Android 37 probe passes APK inspection, controlled-connection staging, replay
persistence and rejection/cleanup checks (`staging-replay-v1`). Local staging of
real disposable-signed APKs and independently verified metadata passes; the
disposable seed was removed. No publication or installation was performed.

Current parent and polling-child unsigned release APKs match across two clean
offline builds after the updater/UI and full-lint fixes. The current proof uses
fixture TLS settings with update authority unconfigured; see
[release build evidence](RELEASE_BUILDS.md). That comparison is separate from
the exported configuration check with update authority enabled, and from
production signing or actual installation.

Production authority, live HTTPS transfer, installation prompts/callbacks,
restart recovery, enrolled-device data preservation, rollback and Redmi
acceptance remain open. The installer flow is compiled, not physically proven.
Local release checks do not satisfy the physical device matrix.

On 25 October, `scripts/test-android-release-persistence.ps1` passes
`replay-persistence-v1` on the isolated unenrolled Android 37 AVD. Separate
prepare/restart/reboot/cleanup phases span a logged force-stop and real reboot.
A new PID at the same boot and a later boot count verify the lifecycle boundary;
the production `ReleaseState` retains sequence 2 and the same state digest.
The same signed payload remains retryable; older, conflicting same-sequence and
different-binding payloads are rejected without rewriting the floor. Test
preferences are cleaned up. Only public fixture envelopes are persisted;
the preparation signing key stays in memory. This does not prove actual installer
callback recovery, production key custody, a live download or enrolled updates.
