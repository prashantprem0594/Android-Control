# Pinned development toolchain

Verified against the official vendor documentation on 2026-10-23.

## Required installations

| Component | Pinned version | Installation source |
|---|---:|---|
| Go for Windows AMD64 | 1.27.3 | https://go.dev/dl/ |
| Android Studio | Rabbit 1, 2026.2.1 | https://developer.android.com/studio |
| JDK | 17 | Bundled with Android Studio; the installed system JDK 17 is also compatible |
| Android SDK Platform | Android 17 / API 37 | Android Studio SDK Manager |
| Android SDK Platform | Android 14 / API 34 | Android Studio SDK Manager; retained for target-device compatibility work |
| Android SDK Platform | Android 13 / API 33 | Android Studio SDK Manager; retained for minimum-version compatibility work |
| Android SDK Build Tools | 36.0.0 | Android Studio SDK Manager |
| Android SDK Platform-Tools (`adb`/`fastboot`) | 37.0.1 | Android Studio SDK Manager or manual Windows ZIP |
| Android SDK Command-line Tools | Latest available | Android Studio SDK Manager |
| Google USB Driver | Latest available | Android Studio SDK Manager |

## Project-managed versions

These are pinned in project files. The wrapper remains the canonical build entry point even though the matching Gradle version is also installed globally for bootstrapping:

| Component | Version |
|---|---:|
| Android Gradle Plugin | 9.4.0 |
| Gradle wrapper | 9.6.0 |
| Global Gradle bootstrap | 9.6.0 at `C:\Gradle\gradle-9.6.0` |
| Compile SDK | 37 |
| Target SDK | 37 |
<<<<<<< HEAD
| Minimum SDK | Child: 28 (Android 9); parent: 33 (Android 13) |
=======
| Minimum SDK | 33 |
>>>>>>> origin/main

## Not required initially

- Android NDK
- CMake
- Android Emulator or emulator system images
- Docker
- PostgreSQL
- Firebase CLI

Testing uses physical Redmi devices. The Gradle wrapper will download the pinned Gradle distribution when the first Android build runs.

The restricted workspace uses a local SDK copy at `C:\Android Control\.tooling\android-sdk`, selected by the ignored `android/local.properties`. Java compilation could not access SDK JARs in the original read-only installation. Build Tools 36.0.0 and platform `android-37.0` were copied from that installed SDK; both debug APKs then built successfully. JVM journal checks use the installed JDK 17 without external test dependencies.

## Verification

After installation, open a new PowerShell window and run:

```powershell
go version
adb version
java -version
```

Expected Go output begins with:

```text
go version go1.27.3 windows/amd64
```

The normal Android SDK location is:

```text
C:\Users\Sushant\AppData\Local\Android\Sdk
```

