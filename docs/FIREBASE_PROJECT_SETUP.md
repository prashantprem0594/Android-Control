# Firebase wake-up setup for our own server

Our server owns accounts, enrollment, commands, policies, call state and audit history. Firebase Cloud Messaging carries only an opaque `syncHint`; the app authenticates with our HTTPS server to retrieve current data. This setup does not use Firebase Authentication, Firestore, Realtime Database or Firebase Hosting.

## Register the Android apps

In an operator-controlled Firebase project, register an Android app with package **`com.appcontrol.parent`**. If child wakeups are also wanted, register a second Android app with package **`com.appcontrol.child`**. They can share a Firebase project, but each must have its own Firebase Android application ID. Keep the package spelling exact. Google Analytics is optional and is not needed by this wake-up implementation. Follow [Firebase's Android setup](https://firebase.google.com/docs/android/setup).

Download the Android `google-services.json` for each registered app. Store the downloads outside source control, for example in an operator input directory. These Android config files contain public project/app identifiers and an API key, not a server private key. Firebase documents package-specific client selection and these public settings in [its config-file guide](https://firebase.google.com/docs/android/google-services-plugin-and-file).

This repository explicitly initializes Firebase from compiled public settings instead of adding the Google services Gradle plugin. Import the downloaded config into the existing property-based build path:

```powershell
& .\scripts\import-firebase-public-settings.ps1 `
    -ParentGoogleServicesFile 'C:\AppControlInputs\parent-google-services.json' `
    -ChildGoogleServicesFile 'C:\AppControlInputs\child-google-services.json' `
    -OutputDirectory '.cache\firebase-public-001'
```

Omit `ChildGoogleServicesFile` for parent-only Firebase. A single downloaded file containing both registered clients may be supplied for both inputs. The importer selects exactly one matching client per package, rejects ambiguous API keys and private/service-account fields, and checks sender/project consistency. The output directory must be new. Output is limited to:

- `parent-firebase-public.json`, and optional `child-firebase-public.json`: exactly `applicationId`, `apiKey`, `projectId`, `senderId`.
- `debug-arguments.json`: Gradle argument/task arrays for the selected `fcmDebug` test builds; not a shell script.
- `firebase-import.json`: completion marker and output file SHA-256 hashes. Structural checks only; all delivery/production flags remain false.

Do not treat successful import as proof the project is controlled, the API key restrictions match the signed apps, or delivery works. No source JSON is copied wholesale, no private file is logged and no Firebase network request is made by this importer.

## Build a controlled test APK

Supply our real HTTPS origin and independently derived current/backup SPKI pins to `scripts/build-wakeup-test-apks.ps1` together with `-FirebaseSettingsDirectory '.cache\firebase-public-001'` and a new `-OutputDirectory`. The required trust arguments are `-ServerOrigin`, `-CurrentPin` and `-BackupPin`; use actual controlled values, not documentation placeholders. `-ValidateOnly` checks inputs without building.

The launcher builds debug test APKs and writes `wakeup-build.json` with `productionReady=false`. It never installs, deploys or enables private updates/remote call control. Reserved fixture origins require explicit `-AllowFixture`; those outputs must not be used as real delivery evidence. Production release signing remains a separate gate. Preserve the existing application signer and data when updating either phone; do not uninstall to hide a signing mismatch.

## Configure the server sender

The server needs its own authorized Firebase sender credentials and parent routing-token encryption key. These are separate private server files, never Android public config inputs or APK assets. Follow [Firebase's HTTP v1 server authorization guidance](https://firebase.google.com/docs/cloud-messaging/auth-server) and the repository's [parent push implementation](PARENT_PUSH_IMPLEMENTATION.md). Use the existing intended routing key for existing registrations; do not replace it as a test shortcut.

The parent sender settings are `APP_CONTROL_PARENT_FCM_SERVICE_ACCOUNT_FILE`, `APP_CONTROL_PARENT_FCM_TOKEN_KEY_FILE` and `APP_CONTROL_PARENT_FCM_SENDER_ENABLED`. Configure the intended Firebase project and validate server/API setup before enabling sending. Keep passwords, tokens and private keys outside chat, logs and source control. A service-account JSON accidentally passed to the public importer is refused.

## Verify on the phones

Install the matching configured parent test APK, sign in to our server, allow incoming-call notifications and verify authenticated token registration. Then follow [screen-off acceptance](SCREEN_OFF_ACCEPTANCE.md): real current child-call event, parent screen off/locked, idle, reconnect, revoked session, duplicate/stale suppression and exact-call review. Use the Redmi 14 5G as parent and Redmi Note 5 Pro as child. Phones may be disconnected from USB while code/configuration is prepared; reconnect for automated device observation.

Successful import, compilation, a surviving process or FCM's transport acknowledgement does not prove that the parent received and published a fresh notification. Keep those evidence levels separate.
