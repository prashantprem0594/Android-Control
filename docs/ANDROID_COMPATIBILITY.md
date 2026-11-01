# Android installation and feature compatibility

The child APK declares **Android 9 or later (API28+)** as its installation baseline. It is not restricted to the two designated Redmi phones. The parent APK separately requires **Android 13 or later (API33+)**. `targetSdk=37` is the Android behavior target; it does not raise the minimum installation version to Android 17.

Installation eligibility does not establish that every control, call or background feature works on every phone. Package signature/version checks, the phone's installation policy and available storage also apply. Do not uninstall an existing enrolled installation to bypass a signature mismatch; preserve its data and intended signer.

| Feature | Requirements and version handling |
|---|---|
| Pairing and authenticated sync | Matching server origin, HTTPS trust/pins, an operational server, a parent-generated pairing code and the device's Android Keystore. Local pairing state does not establish a currently reachable server. |
| Lock and app restrictions | Authorized Android Device Owner setup on a suitable managed device. Ordinary APK installation or parent pairing does not grant Device Owner. Manufacturer setup/provisioning limitations must be tested separately. |
| Phone and remote call controls | Telephony/SIM support, user-selected default phone app and required permissions. Android 9 uses the legacy system default-dialer request; Android 10+ uses the dialer-role flow. Remote call controls additionally require the configured rollout flag and server authorization. |
| Call audio | The code uses the legacy audio-route path below Android 14 and the endpoint path on Android 14+. Wired/Bluetooth, locked-screen, dual-SIM and emergency behavior need supervised checks on each target device. |
| Notifications and call alerts | Android 13+ notification permission; Android 14+ full-screen incoming-call access where applicable. Notification channels and manufacturer background restrictions still affect behavior. |
| Schedules | Cached policies support offline evaluation. Android 12+ exact-alarm access is checked before precise scheduling; missing access can affect timing. |
| Internet pause | Device Owner, explicit VPN consent and supported foreground-service behavior. Only one VPN can normally be active. DNS, IPv6, mobile network and switching behavior require device/network acceptance. |
| Firebase wake-up | A Firebase-enabled build with real public settings, compatible Google Play services, server sender configuration and verified registration/delivery. A phone without the required Google services cannot be assumed to receive Firebase hints. Polling builds do not provide continuous Firebase alerts. |
| Home-network server | Android 17/API37+ local-network access is requested when required. Older versions retain HTTPS hostname and pin checks. The laptop localhost profile also requires its designated USB forwarding connection. |
| In-app private updates | Compiled update authority and a platform security provider supporting Ed25519. Unsupported providers are refused explicitly; use the intended operator installation workflow with the same signer instead. |

Telephony and Bluetooth are optional manifest hardware features, so devices without those features are not excluded from installation on that basis. Their absence still prevents the corresponding call/audio functions. The capability snapshot reports Device Owner, phone role, VPN consent, API and exact-alarm access; it is not a complete hardware or OEM certification.

## Evidence and further devices

Existing scoped evidence includes physical Android 9/API28 child launch, capabilities and identity checks, and Android 17/API37 emulator fixtures. The parent has separate Android 14 physical launch evidence. These checks do not certify every intermediate Android release, every manufacturer or all functionality on those tested versions. Physical policy/call/network acceptance and live Firebase delivery remain open.

For another phone, first confirm its Android version and intended parent/child role, then inspect the installed APK and run the relevant supervised acceptance checks. The current read-only [phone readiness tool](PHONE_ACCEPTANCE.md) deliberately accepts only the operator-confirmed Redmi serial/device mappings; that tooling safeguard is not an APK device restriction. Add a new confirmed mapping before using that tool on another phone. Never reuse a designated serial or bypass the role safeguard.

Code references: `android/child-app/build.gradle.kts`, `android/child-app/src/main/AndroidManifest.xml`, `calls/DialerSupport.kt`, `calls/ManagedInCallService.kt`, `policy/DeviceCapabilities.kt`, `policy/ScheduleAlarm.kt`, and shared `LocalNetworkAccess.java` / `ReleaseUpdate.java`.
