# Public deployment build configuration

`scripts/export-production-configuration.ps1` validates one public JSON input
and exports consistent Gradle settings for both apps. It does not read private
keys, build APKs, connect to Firebase, deploy a server or operate a phone.

The input must contain exactly these fields:

| Field | Required value |
|---|---|
| `serverOrigin` | Canonical lowercase HTTPS DNS origin; omit port 443 and trailing slash |
| `currentPin`, `backupPin` | Distinct canonical Base64 SHA-256 SPKI pins derived from controlled certificates |
| `apkCertificateSHA256` | Lowercase 64-hex digest of the intended APK signing certificate |
| `updatePublicKey` | Canonical Base64 public Ed25519 SPKI DER, 44 bytes |
| `updateMinimumSequence` | Integer 0–2100000000 |
| `versionCode` | Integer 1–2100000000 |
| `versionName` | App version name accepted by android/app-version.gradle.kts |
| `remoteCallsEnabled` | Explicit boolean; use false until controlled rollout is intended |
| `firebase` | Null for polling releases, or exactly four public fields below |

Firebase fields are `applicationId`, `apiKey`, `projectId`, `senderId`.
The application and sender IDs must agree. This uses the existing child Firebase
variant; it does not add parent push delivery. Service-account JSON, passwords,
pairing grants, private keys and tokens are not accepted configuration fields.

Obtain pins using the controlled-certificate procedure in
[SERVER_TRUST.md](SERVER_TRUST.md). Obtain update public authority and signing
identity independently through [PRIVATE_UPDATES.md](PRIVATE_UPDATES.md).
Keep production private signing/metadata keys outside this public configuration.

## Export and use

From the workspace root, through the logger:

```powershell
& .\scripts\run-logged.ps1 -TaskId P7-001 -Category tooling -Command '& .\scripts\export-production-configuration.ps1 -ConfigurationFile C:\Deployment\public.json -OutputDirectory .cache/deployment-config-001'
```

Output directories must be new; files are created exclusively. Output includes
`gradle.properties`, `release-arguments.json`, an optional `firebase-public.json`
and a hashed inventory/completion record, `configuration.json`. Incomplete
directories without that completion record must not be used. The inventory
detects accidental changes; it is not a signature.

For builds, read `release-arguments.json` with `ConvertFrom-Json`, assign its
`arguments` and `tasks` arrays to local variables, and splat them to the existing
Gradle executable within a logged command. Do not execute JSON as shell code or
replace global Gradle settings. The exported task pair selects parent release
plus child polling release or child FCM release. Java, SDK and cache settings
remain those in [RELEASE_BUILDS.md](RELEASE_BUILDS.md).

The optional Firebase JSON can also be passed to the existing reproducibility
runner's `FirebasePublicConfigFile` argument. Remote call opt-in must separately
match the server's `APP_CONTROL_REMOTE_CALLS_ENABLED` setting; exporting build
properties does not change the server or grant call permission.

## Evidence boundary

Reserved/test origins are refused unless `-AllowFixture` is explicit. That
switch labels output `fixture` even for an otherwise public hostname. Normal
output is labelled `deployment-inputs`; **both modes set productionReady=false**.
No heuristic can establish that a syntactically plausible hostname, pin, key
or project is controlled by the operator.

Validation covers shape, canonical encoding and cross-field consistency.
Actual certificate chains/hostname trust, private-key custody, intended signer,
Firebase app/package registration, IAM, live messaging, published APK settings
and upgrade eligibility remain separate checks. No real deployment values were
available during implementation. Focused synthetic checks do not satisfy those
gates, physical managed setup or production acceptance.

## Offline exported-build check

Run `scripts/test-configured-release-builds.ps1` through `run-logged.ps1` with
task `P7-001`. It generates fresh public fixture configurations with update
authority enabled, exports polling and Firebase argument arrays, and passes
those arrays directly to offline Gradle release assembly and full lint. It
checks compiled origin/key/certificate/sequence/version fields and Firebase
settings, confirms remote call rollout stays disabled, inspects manifests and
DEX for SDK/test-code separation, and preserves APKs, BuildConfig and lint XML.

Each run uses a fresh `.cache/configured-releases-*` directory. Only a completed
run writes `configured-builds.json`; failed runs retain partial evidence. The
fixture metadata and APK hashes are public. These APKs are unsigned local test
artifacts with a reserved origin and synthetic trust values, not production
releases. The check does not install anything or send Firebase messages.
