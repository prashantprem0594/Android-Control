# Managed setup components

The child's Android managed-setup handlers and public provisioning-payload
generator are implemented. They do not initiate a factory reset or provisioning
operation. The current instruction not to erase the user's phone remains in
force.

Android requires an unprovisioned device for Device Owner setup, and requires
the provisioning-mode/compliance handlers for modern QR setup. See the official
[managed provisioning requirements](https://source.android.com/docs/devices/admin/provision).
An already-used personal phone is not made fully managed by merely installing
this app. Physical Redmi provisioning acceptance remains blocked.

## Child components

- `ProvisioningModeActivity` selects fully managed mode only when Android
  explicitly includes that mode in its allowed list. Work-profile-only setup
  and unconfigured server origins are refused.
- `PolicyComplianceActivity` verifies actual Device Owner state and a matching
  supplied/compiled server origin before returning success. Its saved flag
  records ownership confirmation, not enrollment or control acceptance.
- Both exported platform handlers require `BIND_DEVICE_ADMIN`, following the
  [Android DPC activity contract](https://developer.android.com/work/versions/android-10).
  Ordinary apps cannot use these activities as an unprotected setup entry.

There is no key export, account creation, permission bypass, policy activation,
data wiping or network enrollment in these handlers. Parent pairing and the
phone/VPN/alarm permission steps remain explicit after managed setup.

## Public provisioning payload

Run `scripts/new-managed-provisioning-payload.ps1` through the command logger.
Supply a staged release bundle, an independently trusted metadata public-key
file, canonical server origin, expected APK signing certificate, minimum
release sequence and a fresh JSON output filename.

The generator verifies signed metadata, the actual child APK length/hash and
signing certificate, and the presence of both correctly protected setup
activities. Debug releases and old APKs lacking the handlers are refused. It
emits public JSON containing:

- The child DeviceAdminReceiver component.
- The immutable signed child download URL.
- Its URL-safe Base64 SHA-256 APK checksum.
- A request to retain existing system applications.
- The public server origin in the admin extras bundle.

Android's [package-checksum extra](https://developer.android.com/reference/android/app/admin/DevicePolicyManager#EXTRA_PROVISIONING_DEVICE_ADMIN_PACKAGE_CHECKSUM)
binds the downloaded APK bytes. No Wi-Fi password, parent password, pairing
code, bearer token or private key is included. The payload itself is not a
one-time enrollment grant: parent pairing still requires a fresh code.

Use release APKs built with the same intended server origin. Regenerate the
payload when the published release changes. The standalone QR utility below
encodes these exact JSON bytes for Android's setup-wizard flow on an
appropriate, explicitly approved unprovisioned device. Displaying JSON alone
is not a QR code.

The standalone utility in `tools/provisioning-qr` renders the payload to PNG
with `github.com/skip2/go-qrcode` and independently decodes the resulting
image using `github.com/makiuchi-d/gozxing`. It verifies the decoded text
matches the input bytes and accepts only the expected public provisioning
payload fields. Dependency versions are pinned in that tool's own `go.mod`;
the source and licenses are vendored there, not in the server module. Build
offline from `tools/provisioning-qr` with:

```powershell
$env:GOPROXY = 'off'
go build -mod=vendor ./...
```

Run the built utility with `-input <verified-payload.json> -output
<provisioning.png>`. This produces an image and confirms a software round-trip;
it does not verify display scanning, setup-wizard compatibility, or device
provisioning.

## Combined setup export

`scripts/export-managed-setup.ps1` combines release verification, payload
generation and independently decoded QR rendering. Build the offline QR tool
as above into `.cache/provisioning-qr.exe`; the release verifier must be present
at `.cache/release-manifest.exe`. Use an independently trusted public key and
certificate digest, and a fresh output directory outside the staged release:

```powershell
& .\scripts\run-logged.ps1 -TaskId P0-003 -Category release -Command '
  & .\scripts\export-managed-setup.ps1 `
    -BundleDirectory C:\releases\family-release `
    -MetadataPublicKeyFile C:\release-trust\metadata.pub `
    -ServerOrigin https://control.example.com `
    -ExpectedCertificateSHA256 <trusted-lowercase-sha256> `
    -MinimumSequence 901 `
    -OutputDirectory C:\releases\setup-901
'
```

Replace all example values before running. Successful export creates
`provisioning.json`, `provisioning.png` and `setup-bundle.json`. The completion
inventory records public input/tool/output hashes and explicitly leaves device
scan and provisioning unverified. The exporter checks that the authority,
tools and verified payload remain unchanged during export. Use the trusted QR
executable: its successful exit is the software round-trip result.

A failed export can leave partial files for inspection. A directory without
`setup-bundle.json` is incomplete; correct the inputs and choose a new output
directory. Existing directories are never reused. This command does not
publish files, start a server, contact a phone or authorize a reset.

## Current evidence boundary

The child APK compiles with both permission-protected handlers. Generator
syntax validation, refusal of an actual pre-handler release APK, offline QR
tool build, and encoder/independent-decoder round-trip checks pass. No device
setup, reset, provisioning operation, positive provisioning download, or QR
scan was performed. P0-003 remains open; these preparations do not establish
Redmi/HyperOS compatibility.

On 25 October, combined-export refusal checks pass against a real signed
pre-handler release: existing output preservation, immutable bundle protection,
wrong-certificate rejection and missing-handler rejection. No failed check
published a completion inventory. See `scripts/test-managed-setup-export.ps1`
and `logs/build/2026-10-25.txt`.

The successful combined export also passes using freshly built parent/child
release APKs, disposable APK and metadata keys, and an isolated `.invalid`
origin. `scripts/test-managed-setup-roundtrip.ps1 -InputDirectory <unsigned-apks>`
signs the two inputs (`parent-unsigned.apk` and `child-unsigned.apk`), stages
metadata and exports the payload/QR. It independently checks output hashes,
the child APK checksum, download URL and origin; stale sequence and modified
APK refusals pass. Fixture private keys are removed in `finally`. The successful
release and setup evidence are preserved separately from the modified copy at
`.cache/setup-roundtrip-0580952457f34a5799b1a545913d806f`. This is local software
acceptance with fixture pins and keys; production deployment and physical
provisioning remain unverified.
