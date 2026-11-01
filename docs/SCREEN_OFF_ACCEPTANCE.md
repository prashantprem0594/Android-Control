# Parent screen-off acceptance

The parent must receive incoming-call alerts without an open Activity and with its screen off. A live process is not an acceptance requirement; authenticated, fresh notifications and correct recovery are. The confirmed parent is Redmi 14 5G. The connected Redmi Note 5 Pro/API28 is the child.

The background implementation uses the separately configured parent Firebase build, opaque high-priority hints, expedited JobScheduler work and authenticated current-state retrieval. Ordinary debug/release builds disable continuous push alerts. The manual foreground call watch lasts ten minutes and is not a continuous idle-delivery substitute. ParentActivity destruction cancels its UI worker, not the push jobs.

The parent has a **Background alert status** screen showing whether its build supports continuous alerts, sign-in state, notification availability and Android's background restriction flag, with a link to app settings. These local observations do not prove server registration or delivery.

Android can defer network access and jobs during idle. High-priority FCM provides a bounded processing opportunity, and Firebase recommends scheduling expedited work promptly. See [Android Doze guidance](https://developer.android.com/training/monitoring-device-state/doze-standby) and [FCM Android priority guidance](https://firebase.google.com/docs/cloud-messaging/android-message-priority). Do not replace acceptance with an endless foreground service or assume a wake lock bypasses Doze.

## Required real-device checks

Use [live setup inputs](LIVE_SETUP_INPUTS.md) to provide existing service configuration or identify services that still need setup.

Use the configured, signed parent build, real HTTPS server, authorized Firebase sender and enrolled child with a test SIM. Confirm notification permission/channel and record the parent OS/build and actual battery/autostart settings. Agree delivery-latency targets before measuring. Record timestamps and opaque test identifiers; exclude tokens, phone numbers and call content.

| Scenario | Required observation | Status |
|---|---|---|
| App foreground | Real child ringing state produces one fresh, authenticated notification; tapping fetches the exact call | Pending |
| Home screen, parent display off | Notification arrives without reopening the parent Activity | Pending |
| Parent locked | Alert respects private/public content; exact-call review occurs after appropriate user interaction | Pending |
| Natural idle and forced Doze | Delivery and latency measured independently; a shell-forced job is not live push evidence | Pending |
| Process terminated by OS | New hint restarts bounded work, preserves deduplication and suppresses ended calls | Pending |
| Reboot, unlock once, screen off | Registration/delivery resumes and fresh hints produce valid alerts | Pending |
| Offline then reconnected | Expired/ended calls do not become new alerts; fresh calls recover | Pending |
| Notifications denied/channel disabled | No misleading success claim; explicit setup/refresh remains available | Pending |
| Logout/session revoked | No further token registration, authenticated work or alert publication | Pending |
| User force-stops app | Record OS suppression; reopen app and verify recovery before resuming delivery claims | Pending |
| Manual call watch | Verify its actual ten-minute limit, screen-off observations and cleanup separately | Pending |

Only the parent phone and real service configuration can close these rows. Child launch, JVM queue tests and emulator evidence do not close them. No real parent/Firebase acceptance was performed on 29 October 2026.

## Current available evidence: 29 October 2026

The wake-up-only implementation was reviewed by separate server/Android agents. Sender registration invalidation now requires HTTP404 plus typed FCM UNREGISTERED; targeted regressions pass (`PARENT-WAKE-BOUNDARY`). Both Firebase-enabled debug variants compile and pass full lint (`WAKEUP-FCM-BUILDS`, 88 tasks), using synthetic public settings only. Those APKs were not installed/launched. None of the real-delivery rows above is closed by these builds.

Resumed parent screen-off smoke: display sleep was confirmed, the same app PID survived ten seconds with the screen off, and `am start` successfully reopened the app after wake (`PARENT-SCREEN-OFF-SMOKE`, logs/build/2026-10-29.txt). Initial immediate state check failed before the asynchronous display transition; bounded transition polling passed. This short process-survival check does not test notification delivery, network sync, natural/deep Doze or long-term OEM behavior and closes none of the live-delivery rows above. No real server/Firebase settings have been supplied.

Physical parent preflight now passes: user-confirmed Redmi 14 5G identifies as model 24094RAD4I/beryl, serial I7MVZPB6ZLJRY95X, Android 14/API34, HyperOS OS1.0. The user installed the parent APK manually after HyperOS refused ADB installation. Package version 1/0.1.0, minimum API33 and target API37 verified; activity launch and running process verified (`PARENT-PREFLIGHT`, `PARENT-LAUNCH`). UI automation opened **Background alert status** and verified the ordinary build correctly reports continuous alerts unavailable (`PARENT-STATUS-UI`). No live sign-in, Firebase or screen-off notification delivery is inferred. The parent is now connected; the child was disconnected for this preflight.

All 12 local integration stages passed: `.cache/integration-e3a7c65b32c24e01b0105910f26b3d2f/result.json`. Full Go tests and vet pass. Parent background-status UI assembly/full lint passes with task reruns and build cache disabled (`PARENT-BACKGROUND-UI`). Child physical API28 launch/capabilities/identity, stalled-transport cancellation/deadline, framework queue redelivery/cancellation and isolated journal process-death checks pass. Logs are in `logs/build/2026-10-29.txt`. These are scoped evidence, not whole-project production acceptance.
