# Private release preparation

Use the [public configuration exporter](PRODUCTION_CONFIGURATION.md) to validate
deployment build inputs together and export argument arrays for both apps.
Its structural checks do not establish production readiness.

Release preparation is local until the operator explicitly distributes artifacts.
The scripts below do not publish, install, replace device ownership or change an
existing signing key. Physical pilot and deployed update acceptance remain open.

## Reproducibility

Run `scripts/test-release-reproducibility.ps1` through `run-logged.ps1`, supplying
the operator's public `ServerOrigin`, `CurrentPin` and `BackupPin`. The script
validates public inputs, then performs two clean offline parent/child release
builds with task reruns and the build cache disabled. Lint remains part of the
normal release assembly. Required dependencies must already be cached from an
authorized ordinary Gradle resolution.

Both unsigned APKs must match byte for byte by SHA-256. Each run creates a fresh
`.cache/release-repro-*` directory containing `build-1`, `build-2` and, only after
matching results, `reproducibility.json`. Failures retain available evidence;
they do not produce a successful record. The comparison covers this host and
pinned toolchain, not independent-host or supply-chain reproducibility.

Use real operator-controlled origin/pins for a deployment. Reserved fixture
origins and arbitrary public test hashes are useful only for build-tool checks;
they do not establish trust in an operational server or make a deployable APK.
Review the generated origin and network security resource before signing.

On 25 October 2026, current parent and polling-child release APKs matched
across two clean offline builds with task reruns and build cache disabled.
Both builds passed vital lint. Evidence is in
`.cache/release-repro-6394d1d8fd3848b085caa59b5afe1b57/reproducibility.json`
and `logs/build/2026-10-25.txt`. This run used version 3 / `0.1.2-setup-fixture`,
an isolated `.invalid` origin and fixture pins, with update authority and Firebase
unconfigured. It establishes reproducibility for those inputs on this host;
production configuration, enabled FCM/update variants and device continuity
require their own acceptance.

Later on 25 October, full `lintRelease` found six child errors beyond vital
lint: phone-account permission handling, API 34 endpoint guards and manifest
declarations. These were fixed; both full release lint tasks and both release
assemblies pass. The private managed-inventory use of `QUERY_ALL_PACKAGES` has
a narrowly documented lint exception. Both apps now explicitly exclude every
supported storage domain from cloud backup and device transfer, and packaged
APK inspection confirms both exclusion sections. Remaining lint warnings cover
English UI strings, missing app icons and synchronous durable preference writes.
These source changes postdate the two-build hash comparison above; that older
comparison is preserved as historical evidence, not a hash claim for new APKs.

After these fixes, the comparison was repeated successfully with the same public
fixture settings and version 3. Both current unsigned APKs match across two clean
offline builds; evidence is
`.cache/release-repro-278c3408a4014f4fa3ddfa9d10248ce3/reproducibility.json`.
This is the current-source comparison; production trust/signing, FCM and enabled
update-authority variants retain their separate acceptance requirements.

The [exported-configuration build matrix](PRODUCTION_CONFIGURATION.md) now also
passes assembly/full lint for update-enabled polling, Firebase and explicit
remote-call opt-in fixtures. That matrix validates compiled settings and APK
separation; it is separate from reproducibility, production signing and live
delivery checks.

## APK signing

Use an existing operator-controlled release keystore, backed up separately from
build artifacts. Record its certificate's SHA-256 through an independently
verified channel. Keep the same application signing identity for updates;
uninstalling/reprovisioning a Device Owner phone is not an update procedure.

Invoke `scripts/sign-private-apks.ps1` through the logger with:

- `InputDirectory`: a verified snapshot containing `parent-unsigned.apk` and
  `child-unsigned.apk`.
- `KeystoreFile` and `KeyAlias`: the operator's existing release identity.
- `ExpectedCertificateSHA256`: the exact certificate digest, without colons.
- `OutputDirectory`: a fresh private directory that does not already exist.

The script prompts securely for keystore and key passwords and supplies them
through UTF-8 stdin. Passwords are not passed in command arguments, files or
logs. Managed strings exist transiently in memory. Do not put literal passwords
in an outer logged command or pass them through shell history.

The signer checks expected package names and refuses debuggable inputs. It uses
APK signature schemes v2/v3, refuses an Android Debug certificate, verifies the
result against the independently expected release certificate and writes public
hash metadata only after both APKs pass. Existing output is never overwritten.
A failed run may retain a partial signed artifact for inspection; no failed or
partial run is approved for distribution.

`signed-apks.json` is a local hash inventory, **not a signed update manifest**.
Signed hosting/update metadata, staging, rollback/enrollment preservation,
cross-device installation and trusted key custody remain separate P7 gates.

`scripts/test-private-apk-signing.ps1` exercises the signer with a fresh isolated
fixture key, checks wrong-certificate refusal, verifies output hashes and checks
overwrite refusal. Its randomly generated password is kept out of logs, and its
disposable keystore is removed after the test. Fixture APKs and public evidence
remain local and must never be distributed as production releases.

Both modules accept paired public Gradle properties appControlVersionCode and appControlVersionName; absent inputs retain 1/0.1.0. Version codes are canonical positive integers through 2100000000 and names use a bounded printable identifier. Release reproducibility accepts VersionCode/VersionName parameters, and signed inventories record the actual APK version. Choose a higher code for staged updates; do not uninstall a managed app to force a downgrade. The isolated emulator passed a real debug version 1-to-2 package replacement, Keystore/Device Owner retention, cached-policy restoration and lower-code refusal. See EMULATOR_TESTS.md for evidence boundaries.
