# Inputs for live phone and screen-off verification

Current observation, 30 October 2026: the existing laptop runtime passes identity verification and pinned HTTPS readiness. Public Firebase inputs are present at `data/firebase/public-settings-001` and pass structural build validation. Both staged Firebase-enabled APKs at `data/laptop-apks-001` match their recorded hashes. Reuse this existing state for laptop testing; do not recreate identities or credentials. Both designated phones are currently absent from ADB. Reconnect them with authorized USB debugging before inspecting installed variants, pairing or live delivery. Server credential file presence has been observed, but credential validity and real Firebase delivery have not been established. Evidence: `REMAINING-LOCAL-READINESS` in `logs/build/2026-10-30.txt`.

Later the same day, the user designated an additional child phone: Xiaomi 2508CRN2BI (`tornado`, Android15/API35, serial `C6INTOMB8T55X48H`). Its configured Firebase child app was manually installed after USB installation refusal, and package metadata/foreground main-screen startup pass (`UI-CHILD-ANDROID15-VERIFIED`). Its laptop USB HTTPS reverse mapping is configured. Notification permission is denied and Device Owner is not observed; pairing, policy/call controls and delivery remain unverified. The original Android9 child mapping and separate parent mapping are preserved. See [phone readiness](PHONE_ACCEPTANCE.md).

The current offline Linux-amd64 staging package is `.cache/linux-deployment-20261030-permissions`; all 26 file sizes/hashes verify, including the corrected configuration-permission preflight. It contains no credentials and has not been deployed. `scripts/export-linux-deployment.ps1` recreates staging from a trusted verified bundle. After preparing the chosen Linux host, run its read-only `deploy/preflight.sh` before starting services. Use [phone readiness](PHONE_ACCEPTANCE.md) to inspect either designated phone without changing it.

Setup tools are now available: [Firebase project setup](FIREBASE_PROJECT_SETUP.md) imports actual Android public config; `scripts/build-wakeup-test-apks.ps1` builds and verifies controlled debug APK snapshots against real HTTPS pins. [Linux hosting](SERVER_HOSTING.md) supplies service/proxy/backup templates for an always-on host. None creates a cloud account, buys a domain or activates fixture credentials.

Both ordinary debug apps previously passed installation/startup on their designated phones. The last inspected installed parent build lacked continuous parent alerts. A running laptop server and matching Firebase-enabled APK snapshots now exist; reconnect the phones to inspect installed variants and perform installation/pairing as needed. Live notification delivery still requires verified registration and an authorized sender; changing battery settings alone cannot establish it.

## Existing services

Provide local configuration file paths, not credentials in chat. Reuse existing identities and keys if services already exist.

| Input | Needed for |
|---|---|
| Controlled HTTPS server origin and current/backup certificate SPKI pins | Build the clients for the actual server and verify trust |
| Parent Firebase public settings: applicationId, apiKey, projectId, senderId | Build the parent Firebase variant for package `com.appcontrol.parent` |
| Separate child Firebase public settings, if child push is enabled | Build child variant for `com.appcontrol.child`; parent and child application IDs must differ |
| Server configuration file | Verify API, storage, TLS and opt-in settings before starting the sender |
| Server-only Firebase sender credential file | Authorize sending to the real parent Firebase project |
| Existing parent routing-token encryption key file | Preserve/decrypt parent registration state; never replace an existing key as a test shortcut |

The server settings are `APP_CONTROL_PARENT_FCM_TOKEN_KEY_FILE`, `APP_CONTROL_PARENT_FCM_SERVICE_ACCOUNT_FILE`, and `APP_CONTROL_PARENT_FCM_SENDER_ENABLED`. Presence does not establish validity or successful delivery. Keep the sender disabled until configuration and registration are verified. See [parent push implementation](PARENT_PUSH_IMPLEMENTATION.md).

Production release inputs additionally include the existing APK signing identity and matching private update authority. These are separate from controlled debug-device delivery evidence; a passing debug test does not establish a production release. Public configuration/export fields and validation are in [production configuration](PRODUCTION_CONFIGURATION.md).

## Services not yet created

First establish whether the server will run on an existing host/domain or whether a new host/domain is needed, and whether a Firebase project already exists. Account creation, domain ownership and any paid hosting choices require the operator's access/choice. Do not invent a production hostname, project, credential, signer or pin from local fixtures.

## Phones

- Parent: Redmi 14 5G, serial `I7MVZPB6ZLJRY95X`, Android 14/API34. Connect it with authorized USB debugging for build/status and delivery observation.
- Child: Redmi Note 5 Pro, serial `43c0acdd`, Android 9/API28. Reconnect for enrollment, supervised normal-call and policy tests; confirm test SIM availability first.
- No factory reset, OS replacement or production deployment is authorized by merely resuming verification.

Once inputs and the parent device are available: validate public configuration, build/install the matching Firebase-enabled parent, verify sign-in and registration, then execute [screen-off acceptance](SCREEN_OFF_ACCEPTANCE.md). Keep failed, skipped and unperformed checks distinct from passing evidence.
