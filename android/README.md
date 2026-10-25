# Android build and Device Owner test

## Build

From `C:\App Control\android`:

```powershell
$env:GRADLE_USER_HOME = "C:\App Control\.cache\gradle"
.\gradlew.bat :parent-app:assembleDebug :child-app:assembleDebug --no-daemon
```

Debug APKs:

- `parent-app/build/outputs/apk/debug/parent-app-debug.apk`
- `child-app/build/outputs/apk/debug/child-app-debug.apk`

## Physical Redmi feasibility test

The phone must be factory-reset and must not have accounts, users, or a work profile configured. Complete only the minimum setup needed to enable Developer options and USB debugging.

Connect the phone by USB, accept its debugging authorization prompt, and verify it is visible:

```powershell
adb devices -l
```

Install the child debug APK:

```powershell
adb install -r ".\child-app\build\outputs\apk\debug\child-app-debug.apk"
```

Enroll it as Device Owner:

```powershell
adb shell dpm set-device-owner "com.appcontrol.child/com.appcontrol.child.admin.AppControlDeviceAdminReceiver"
```

Confirm the owner:

```powershell
adb shell dpm list-owners
```

Open **App Control Child**. It must show `Device Owner: active`. Press **Lock device (feasibility test)** and confirm the phone locks immediately.

Device Owner enrollment changes device management state. The supported clean removal path during this feasibility phase is another factory reset.

After HTTPS enrollment, **Sync parent commands** fetches pending lock commands,
uses a persistent journal to avoid repeating an action, sends results and caches
the latest desired policy. Device Owner must be active to execute a lock.
See [command-channel verification](../docs/COMMAND_CHANNEL.md). Cached policy is
reconciled through Device Owner app suspension and a consent-based internet-pause
prototype. A persisted background job and boot/update receivers provide fallback
sync and cached reconciliation. Physical Redmi behavior remains unverified;
see [device policies](../docs/DEVICE_POLICIES.md).

The child also includes **Set up phone role (feasibility)**, **Open phone**, and
**Allow lock-screen call alerts** for the Telecom experiment. These require
explicit Android role/permission setup. Read [call feasibility](../docs/CALL_FEASIBILITY.md)
before running the physical test matrix. No remote-call server RPC is enabled.
