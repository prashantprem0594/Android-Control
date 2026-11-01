# Temporary laptop server and the existing cloud path

Laptop mode runs our own API/SQLite server behind a pinned localhost HTTPS proxy. It does not use Firebase databases, authentication or hosting. State defaults to ignored `data/laptop-server`: database, private certificate/routing/backup keys and private runtime logs stay there. Keep this directory when restarting; startup refuses to replace existing identities. Do not put it in evidence archives.

The first phone connection uses USB ADB reverse forwarding. This deliberately avoids external DNS, firewall changes and a system-wide certificate installation. It is **not wireless/mobile-internet hosting**. Keep the laptop awake and the participating phones connected by authorized USB while testing. Laptop sleep, shutdown or USB disconnection interrupts this path. The [Linux/domain deployment path](SERVER_HOSTING.md) remains separate and lets the development laptop be off when another host serves the app.

## Start the own server

Run through the logging wrapper:

```powershell
& .\scripts\run-logged.ps1 -TaskId LAPTOP-START -Category tooling `
    -Command '& .\scripts\start-laptop-server.ps1'
```

Defaults are HTTPS `https://localhost:8443` and the private HTTP backend `127.0.0.1:18080`. Both bind loopback only. `-InitializeOnly` prepares the public metadata/certificates without starting processes. Parent/child senders and remote call rollout are disabled by default. No cloud account, phone settings, firewall rules or OS trust store are changed.

Public build input is `data/laptop-server/laptop-server.json` plus `root-ca.pem`; private keys and service-account files are never app assets. Startup readiness verifies the certificate chain, hostname and exact current SPKI pin. Restart retains the database and identities. The recorded process paths, hashes and start times are checked before stop actions.

## Build the matching debug apps

```powershell
& .\scripts\run-logged.ps1 -TaskId LAPTOP-APKS -Category build `
    -Command '& .\scripts\build-laptop-test-apks.ps1 -OutputDirectory .cache/laptop-apks-001'
```

The output directory must be new. Defaults build both polling debug apps, without Firebase. The explicit development CA is added only for the compiled localhost domain in debug/fcmDebug; both pins and hostname verification remain required. Release tasks refuse this development CA. This does not globally trust self-signed certificates or include a private key in either APK. `-ValidateOnly` checks public metadata/trust without building.

Install the matching parent/child snapshots on their designated phones using the existing signer and data. If HyperOS/MIUI blocks USB installation, transfer the APKs and install them manually. Installation and phone acceptance are separate from compilation.

## Connect each participating phone

```powershell
& .\scripts\connect-laptop-phone.ps1 -Role parent -Serial I7MVZPB6ZLJRY95X
& .\scripts\connect-laptop-phone.ps1 -Role child -Serial 43c0acdd
```

Each command targets the exact previously designated phone and maps its HTTPS port to the laptop's same port. It preserves unrelated mappings and refuses a conflicting mapping. It never installs, provisions, clears data, requests permissions or places calls. USB reconnects may need the command again. Open the matching parent app and create/sign into an account on this local server; pairing remains an explicit parent/child workflow.

Creation uses ADB's `--no-rebind` guard, so a conflicting mapping created after inspection is also preserved. `scripts/test-laptop-phone-mapping.ps1` verifies creation, repeated connection, conflict/ambiguity refusal, unrelated mapping preservation, concurrent binding refusal and final mapping checks using mock ADB responses. These checks passed under `LAPTOP-MAPPING-RULES` in `logs/build/2026-10-30.txt`; no phone connection or live delivery is inferred.

## Add only Firebase wake-up transport when ready

First use the [Firebase public config importer](FIREBASE_PROJECT_SETUP.md). Supply its output with `build-laptop-test-apks.ps1 -FirebaseSettingsDirectory <import-directory>` to build the Firebase-enabled parent and optional child. Stop the own laptop runtime before changing sender configuration, preserve its private keys/database, then start with the real server-only sender credential path and the corresponding opt-in switch. No credentials should be pasted into chat or compiled into APKs.

Without those real settings the local server still supports authenticated API work, but continuous parent push alerts are unavailable. A ten-minute manual watch is not continuous screen-off/Doze acceptance. Use [screen-off acceptance](SCREEN_OFF_ACCEPTANCE.md) for the actual notification tests.

## Stop and deployment boundary

```powershell
& .\scripts\run-logged.ps1 -TaskId LAPTOP-STOP -Category tooling `
    -Command '& .\scripts\stop-laptop-server.ps1'
```

Stop preserves private state and targets only the exact recorded own processes. Laptop state is development state, not an accepted production release or a proven domain-migration workflow. A cloud/domain deployment needs its controlled HTTPS identity, configured client builds and explicit enrollment/restore acceptance. The verified Linux export remains available for that deployment; no paid host is selected or started by laptop mode.
