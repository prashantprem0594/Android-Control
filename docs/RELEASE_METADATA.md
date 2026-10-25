# Signed release metadata

The metadata tools prepare a verifiable public update description. Shared
Android code now retrieves metadata, protects against replay, verifies/stages
APKs and supports locally confirmed installer sessions. See
[private updates](PRIVATE_UPDATES.md) for the current flow and its verification
boundaries. Production authority, hosting, installation and enrolled-device
rollback acceptance remain P7 work.

## Format and trust

`contracts/release-manifest.schema.json` describes the outer envelope and decoded
payload. The envelope contains version 1, an unpadded canonical base64url payload
and an Ed25519 signature. The signed bytes are the UTF-8 domain prefix
`app-control-release-manifest-v1` followed by one newline and the exact payload
bytes. The domain prevents using this signature as device-command authority.

The payload binds a positive release sequence, issue/expiry timestamps,
canonical HTTPS DNS origin and exactly one parent and child artifact. Each
artifact declares package/version, Android SDK bounds, byte length, SHA-256 and
the application signer certificate's SHA-256. Artifact URLs must be exactly
`{origin}/releases/{sequence}/{parent|child}-release.apk` without alternate hosts,
queries, fragments or path traversal. Both apps use the expected independently
trusted application certificate. Metadata signing and application signing use
separate key roles.

Verification requires the trusted metadata public key, expected compiled origin,
expected APK signing certificate and a minimum previously trusted release sequence.
Neither the manifest nor its download response can supply its own trust anchors.
Expiry is exclusive; validity cannot exceed 30 days, and issue times more than
five minutes ahead are refused. Replay below the minimum sequence fails. A
sequence equal to the minimum is permitted so a failed download can be retried.
The eventual updater must persist its trusted sequence, check installed package
versions, verify downloaded byte length/hash and actual APK package/signing
identity, and obtain the appropriate installation authorization. Metadata
verification alone performs none of those steps.

The Go signer emits deterministic app ordering and JSON for the same metadata/key.
Go and Android validate field types/sets, strict integer fields, bounded sizes,
origin/signer, URLs and time/sequence constraints. Error summaries omit private
key material. No key or metadata in a fixture authorizes a production update.

## Operator CLI

Build `server/cmd/release-manifest` into `.cache/release-manifest.exe` through
the standard logger. Every operator invocation must also use the logger.

- `keygen -private-key <new .key file> -public-key <new .pub file>` creates an
  Ed25519 private seed and DER SubjectPublicKeyInfo public key. Protect private
  storage and back it up separately. Existing destinations are refused.
- `sign -input <reviewed payload JSON> -private-key <seed file> -out <new envelope>`
  signs locally after semantic validation. Populate hashes, lengths, versions and
  signing-certificate values from independently verified APKs/signing inventory.
  The CLI does not inspect APK files or prove that those artifacts were reviewed.
- `verify -input <envelope> -public-key <trusted DER key> -origin <compiled origin>
  -certificate-sha256 <trusted APK certificate> -minimum-sequence <counter>` checks
  the signature and all trust/time constraints. Its clock must be correct.
- `fixture -out <new public fixture JSON>` generates isolated public test metadata
  and a public key using a temporary in-memory signing key. It retains no private
  key and contains synthetic APK hashes, so it is not a distributable release.

The CLI accepts bounded regular files and creates new outputs exclusively.
Private seeds are never command arguments or console output. Keep application
signing keystores, metadata signing seeds and routing/backup keys distinct.

## Read-only server endpoint

The backend can serve a verified public snapshot at `GET /v1/releases/current`
(and `HEAD`) when all four operator settings are provided:

| Setting | Meaning |
|---|---|
| `APP_CONTROL_RELEASE_MANIFEST_FILE` | Signed envelope file |
| `APP_CONTROL_RELEASE_PUBLIC_KEY_FILE` | Independently trusted DER metadata public key |
| `APP_CONTROL_RELEASE_ORIGIN` | Expected canonical HTTPS deployment origin |
| `APP_CONTROL_RELEASE_CERTIFICATE_SHA256` | Expected lowercase APK signer certificate digest |

`APP_CONTROL_RELEASE_MIN_SEQUENCE` is an optional nonnegative replay floor.
Partial configuration fails startup; absent settings leave the route disabled.
Startup reads bounded regular files and verifies the snapshot before enabling it.
The in-memory document owns copies of the bytes/key, so subsequent caller/file
changes cannot replace the approved snapshot. Publishing a new snapshot requires
an operator-controlled restart with the new file and an appropriate sequence
floor. The server revalidates time constraints on every request and returns 503
after expiry instead of serving stale metadata. Authentic expired metadata does
not prevent the control server from starting; only the update endpoint becomes
unavailable. Responses use `no-store` and the
public route accepts no publishing writes.

This endpoint hosts only signed metadata. The referenced APKs still require
operator-controlled HTTPS static hosting under the declared immutable release
paths. The APK bytes must match the signed metadata and the existing installation
identity. No production server or external hosting was configured by the tests.

`ReleaseManifest.verify` is a shared Android library entry point. No exported
activity/service accepts arbitrary trust parameters to initiate updates.
`scripts/test-android-release-manifest.ps1` accepts only a public isolated fixture
and exercises the verifier in a test APK on either app. The probe checks the
Go-generated signature and wrong-key, origin, certificate, replay, expiry,
payload tampering, unknown envelope fields and trailing-data refusal. Build/test
results must be recorded before claiming cross-platform verification.

Cross-platform metadata evidence (24 October 2026): full Go tests/vet and release-manifest CLI checks pass. A newly generated public fixture signed by Go verifies with the platform Ed25519 implementation in both parent and child Android 37 test APKs. Wrong public key, origin, APK signer, replay sequence, exact expiry, altered payload, unknown envelope fields and trailing data are refused. The fixture uses synthetic APK hashes and a test clock and retains no private key. This proves metadata verification, not trusted production key embedding, artifact download/inspection, installation or hosting.

## APK inspection evidence (24 October 2026)

Shared ReleaseArtifact.inspect first verifies signed metadata with independently supplied anchors, then checks the staged file length and SHA-256, actual archive package/version/SDK declarations, non-debug/non-test-only monolithic form and current signer certificate through Android PackageManager. It accepts normal Android private-storage ancestor aliases while refusing an artifact that is itself a symlink. requireUpgrade separately enforces the same package/current signing key and strictly increasing version; signer rotation and downgrade are refused. This library does not download, install or authorize updates. A future installer must consume the same verified bytes and retain Android installation signature checks; inspection is not a replacement for that enforcement.

The parent Android 37 probe passed with an actual release APK signed by an isolated disposable test key. Test metadata keys are generated in memory. Signed length/hash/package/version/SDK/signer mismatches, a debug archive and altered bytes are rejected. Synthetic installed baselines exercise upgrade success and same-version/downgrade/package/signer refusal; these do not prove a real installed upgrade. An initial positive probe failed on Android private-storage ancestor canonicalization; the corrected run passed and cleaned its private staging file. APK checks confirm test code and the fixture APK are absent from application APKs.

To reproduce, place a public disposable-signed non-debug parent APK at fixture-release.apk within an isolated asset directory. Build parent debug plus Android test APKs with appControlArtifactFixtureDir set to that directory, install matching APKs on the guarded Android 37 AVD, then run scripts/test-android-release-artifact.ps1 with explicit serial/ADB port. The asset directory is used only by the test source set. Never use a production private key to generate the fixture. Production trust-key embedding, download/staging, sequence persistence, installation authorization, hosted artifacts and enrolled-device update/rollback acceptance remain open.

## Private download staging

ReleaseDownload.stage verifies signed metadata before using its exact HTTPS artifact URL for the current package. It sends no bearer credential, refuses redirects and encoded bodies, checks declared and actual lengths, bounds transfer time, observes caller cancellation and owner-thread interruption, and writes only an exclusively created app-private file. At completion it rechecks metadata freshness and invokes actual archive inspection. Failed attempts delete their partial file. The caller owns a successful staged file and must keep those bytes unchanged, check upgrade compatibility and obtain installation authorization. No automatic installation is performed.

Android 37 controlled-HttpsURLConnection checks pass for successful staging of the real disposable-signed fixture, redirect, truncated/oversized data, hash mismatch, declared-length/encoding mismatch, cancellation/interruption, expiry during transfer and partial-file cleanup. A real artifact symlink is refused. These connection doubles do not establish a live TLS transfer. Production trust-key embedding, persistent replay floors, trusted manifest retrieval, install flow, hosted artifacts and enrolled update/rollback remain open.
