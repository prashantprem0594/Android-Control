# Android UI polish - 30 October 2026

Both apps use a shared native visual system in `AppAppearance.java`: light blue-gray surfaces, dark readable text, rounded cards, clear blue primary actions, focus/disabled states, ripple feedback and density-independent spacing. Text uses scalable sizes; keypad rows can grow with larger font settings. The parent and child have distinct adaptive launcher icons.

The parent login and dashboard have family headers, grouped forms, separate device cards and a centered content width for larger screens. The final correction compacts the login layout to keep sign-in controls readily reachable. Policy screens distinguish requested state from child confirmation. Existing authentication, request, command and policy workflows remain in place.

The child home screen groups device status, calling, pairing, call setup, parent controls and maintenance. Saved pairing is described as local state rather than proof of a live server connection. Dial-pad, in-call and private-update screens share the same appearance. The final dial-pad correction pins the Call control outside the scrolling content so it remains available. Call selection, permissions, Telecom actions and explicit confirmation flows remain in place.

System-bar, display-cutout and keyboard insets are handled on API30+; API28/29 retain platform fitting and keyboard resizing. The child minimum remains Android 9/API28; the parent minimum remains Android 13/API33. See [Android compatibility](ANDROID_COMPATIBILITY.md) for feature prerequisites and device-testing limits.

## Verification

`UI-POLISH-BUILD` in `logs/build/2026-10-30.txt` records successful parent/child debug assembly and both debug lint tasks. `UI-POLISH-SOURCE` records XML parsing checks. `UI-POLISH-FIREBASE` records the first configured Firebase debug assembly, lint and packaged-origin checks. `UI-POLISH-FINAL-FIREBASE` records those checks passing again after the compact-login and pinned-Call corrections. The current configured APK snapshots are at `data/laptop-apks-ui-final-20261030`, preserving the earlier `data/laptop-apks-ui-20261030` snapshots. Child metadata retains minSdk28 and parent minSdk33. Compilation does not establish visual acceptance on physical phones.

`UI-POLISH-ARTIFACTS` verifies the first staged APKs; `UI-POLISH-FINAL-ARTIFACTS` independently verifies each current APK hash against `wakeup-build.json`, inspects its packaged minimum SDK and adaptive launcher icon, confirms the shared `AppAppearance` class is present, and rejects a leaked `UiAppearanceProbe` test class. Both current app APKs pass.

| Configured Firebase debug APK | Minimum Android | SHA-256 |
|---|---|---|
| `parent-debug.apk` | 13 / API33 | `3d533d322378c9aaa5c3c26f0c47eda6a618690e5410b11a8948f0f472724cf6` |
| `child-debug.apk` | 9 / API28 | `ea9e51a1f0aa33af8f5fc7efb0b3b187301da340308d894079e4cfdf6a0c7739` |

`UI-PREVIEW-BUILD` records successful isolated preview/instrumentation assembly (138 tasks). `UI-PREVIEW-RUNTIME` passes for the initial isolated parent and child apps: guard/layout checks and all four private PNG captures (parent login; child home, dial pad and empty in-call screen). The final compact-login/pinned-Call correction requires its own updated preview verification; the earlier capture result does not establish that result.

After USB installation was refused on the physical Android 15 child phone, the earlier child snapshot from `data/laptop-apks-ui-20261030` was manually installed. `UI-CHILD-ANDROID15-VERIFIED` confirms its package, resumed main activity and running process. The final corrected APK in `data/laptop-apks-ui-final-20261030` has not yet been installed there. No completed physical visual acceptance, full Android-version matrix or live Firebase delivery is inferred.

`UiAppearanceProbe` is packaged only in instrumentation APKs and accepts only empty, debuggable `.uipreview` apps on a Google emulator. It renders synthetic app views to private PNG files without disabling secure-window protection. It never accepts production packages or physical phones. No real accounts, calls or enrollment are needed for these previews.
