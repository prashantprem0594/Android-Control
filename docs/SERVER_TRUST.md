# Server trust and certificate rotation

Android 17 (target SDK 37) requires separate local-network consent for home-hosted servers. Both apps declare ACCESS_LOCAL_NETWORK and provide an explicit foreground setup button. Known private numeric/.local/.home.arpa origins prompt for setup before requests; private DNS deployments can use the same button. No background prompt, DNS discovery, TLS fallback or trust bypass is added. Production configured origins remain DNS-only. See [Android documentation](https://developer.android.com/privacy-and-security/local-network-permission).

Both Android apps use Android Network Security Configuration, retaining system certificate validation and hostname verification. No custom TrustManager, HostnameVerifier, trust-all callback, user-CA anchor or debug trust override is installed. Configured builds add SHA-256 SubjectPublicKeyInfo pins for the exact DNS host. The child also checks the compiled HTTPS origin, including port, before enrollment, authentication and synchronization. Redirects remain disabled. The parent uses the same manifest trust configuration and shared origin validation in ParentClient.

Android recommends a backup key when pinning; pins match public keys in the validated certificate chain. See the [official network security configuration guide](https://developer.android.com/privacy-and-security/security-config) and [HTTPS API reference](https://developer.android.com/reference/javax/net/ssl/HttpsURLConnection). Resources are generated through the Android variant source API, following the [official Gradle extension guidance](https://developer.android.com/build/extend-agp).

## Deployment inputs

Supply all three Gradle properties together:

| Property | Meaning |
|---|---|
| appControlServerOrigin | HTTPS DNS origin, optional port, no credentials/path/query/fragment |
| appControlCurrentPin | Canonical Base64 SHA-256 of the current leaf certificate's DER SPKI |
| appControlBackupPin | Pin of a distinct backup leaf key controlled by the operator |

The two pins must decode to exactly 32 bytes and differ. Hostnames are ASCII DNS names, normalized to lowercase. Port 443 and a root slash normalize away. IP-address deployment origins are not supported for configured builds. Pin values and hostnames are public metadata; private keys never belong in Gradle properties or logs.

Unconfigured debug builds retain system-trusted HTTPS for development, with origin syntax validation but no deployment binding or pins. Any partially supplied or malformed configuration fails Gradle configuration. Release tasks require a complete configuration. There are no built-in deployment hosts, example pins or pin expiration dates that could silently disable pinning. Release signing and reproducibility remain separate unfinished release work.

Derive each pin offline from exactly one PEM certificate, using the administration executable through the logger:

```powershell
& .\scripts\run-logged.ps1 -TaskId P6-005 -Category tooling -WorkingDirectory . -Command '& ".\.cache\control-admin.exe" tls-pin -certificate "C:\PrivateDeployment\current-leaf.pem"'
```

Run separately for the backup certificate. The command prints only the public Base64 pin. It rejects certificate bundles, private-key PEM blocks, malformed certificates and files larger than 1 MiB. It calculates a fingerprint; it does not establish certificate trust or verify the deployed hostname, validity or private-key custody. Obtain the certificates from the controlled deployment files, rather than trusting a first network connection.

Put the three public values in an operator-controlled Gradle properties file, or pass them as quoted -P arguments to the normal logged build command. Use the identical properties for both apps. Keep deployment-specific properties out of the repository. After building, verify the generated network-security resource, manifest reference and BuildConfig origin before distributing the APK. Debug builds with complete inputs use the same pins; no debug bypass is present.

## Rotation procedure

### Certificate-file preflight

Before building or changing the deployment, validate the current and backup
leaf-first certificate chains with the administration executable:

```powershell
& .\scripts\run-logged.ps1 -TaskId P6-005 -Category verification -Command '& ".\.cache\control-admin.exe" tls-preflight -hostname "control.example.org" -certificate "C:\PrivateDeployment\current-chain.pem" -backup-certificate "C:\PrivateDeployment\backup-chain.pem"'
```

Use the DNS host from the deployment origin, without scheme or port. Each file
must contain only PEM certificates, with its leaf first and any intermediates
following, at most 16 certificates and 1 MiB total. Private keys, malformed
prefixes, non-certificate blocks and trailing data are rejected. The existing
`tls-pin` command still requires exactly one certificate and uses the same
strict reader.

Preflight checks both leaves for the hostname, current validity, server-auth
usage and a chain to the operator host's system trust store. It requires distinct
leaf SPKI pins and emits only JSON with the normalized hostname, current/backup
pins, expiry times and check time after both chains pass. There is no option to
trust a supplied root; including one in a chain does not make it a trust anchor.
These checks use [Go certificate verification](https://pkg.go.dev/crypto/x509#Certificate.Verify),
whose platform verifier and system trust can differ from Android. Platform
certificate services may consult network resources; this is not a guarantee of
network-free execution. The command does not connect to the deployment origin,
prove private-key possession, perform an Android handshake or check the
deployment's actual served chain. Successful file preflight is preparation,
not completion of the device rotation gate.

### Deployment and device verification

1. Prepare valid server certificates for independent leaf keys A and B. Keep normal CA/hostname validation working. Derive both public pins offline.
2. Distribute APKs containing A+B while the server still uses A. Confirm every enrolled device, including devices returning from offline periods, runs this trust configuration and synchronizes.
3. Change the server to B through the deployment process. Confirm enrollment/authentication, command fetch/ack and policy reporting on each supported Android/HyperOS version. An unpinned key C must fail TLS before application credentials are sent.
4. Prepare C and distribute B+C before retiring A completely or rotating to C. Preserve overlap for offline devices. A newly issued certificate reusing key B has the same SPKI pin but still needs valid CA, hostname and validity checks.

The origin is bound at build time. Moving the server to another host or port requires a correctly configured app update and a separate enrollment/server-state migration plan. The child does not rewrite persisted enrollment state merely because the build origin changes. A mismatch fails before sending credentials. Recovery from losing both pinned keys requires a trusted app update; there is no automatic fallback to an arbitrary certificate, unpinned HTTPS or another host.

## Verification boundary

Use [the Android acceptance record](SERVER_TRUST_ACCEPTANCE.md) to capture the
remaining device handshake and rotation results, including negative certificates
and recovery after failures.

Both modules now build a dependency-free instrumentation test APK with a
credential-free `/healthz` TLS probe at the compiled deployment origin.
`scripts/test-android-server-trust.ps1` logs and validates probe results;
`scripts/test-android-trust-runner.ps1` tests its failure handling with fixtures.
`scripts/test-server-trust-apks.ps1` checks that the probe exists only in test
APKs and that their instrumentation targets match the respective apps. Build
and packaging checks do not establish device TLS behavior.

`scripts/test-server-origin.ps1` covers canonical origins and negative cases on the JVM. Build checks verify generated pin resources, complete-input validation and refusal of unconfigured release tasks. Go tests independently verify SPKI derivation and rejected certificate inputs. These are code/build checks, not proof of a physical Android TLS handshake or successful live certificate rotation.

Physical acceptance must exercise A to B with the same enrolled device, resumed/background/reboot sync, CA-trusted unpinned C, wrong-host and expired/untrusted certificates, certificate renewal with the same key, an offline device returning after rotation and the trusted-update recovery path. P6-005 remains in progress until those results and clean-host database recovery are recorded.

The original `enrollment/EnrollmentClient.kt` is write-protected in this workspace and retained as unused historical source. The active UI and command path use `network/DeviceEnrollmentClient.kt`; new callers must use that client and the shared origin helper.
