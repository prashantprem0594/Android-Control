# Parent incoming-call watch

The optional parent Firebase variant now has a separate [authenticated event synchronization path](PARENT_FIREBASE_SYNC.md). Local implementation/build checks do not establish live Firebase or OEM delivery. The bounded foreground watch below remains separate.

Open a device's **Foreground calls** screen, choose **Watch incoming calls for
ten minutes**, review the polling notice and grant Android notification access.
This starts a visible data-sync foreground service for one child. Starting a
new watch replaces the old one. Use **Stop call watch** or the service
notification's **Stop watching** action to end it. Signing out stops the watch.

The service reads the existing owner-authenticated call snapshot, waiting five
seconds between completed requests. It does not add parent Firebase registration,
boot receivers, permanent background polling or Telecom/media permissions.
Monitoring continues while the parent app is backgrounded if Android permits
the service to run. It is a bounded observation option, not continuous push
delivery or a guaranteed call-alert channel. Child sync, network, Android sleep,
notification settings and service restrictions can delay or prevent alerts.

## Alert behavior

Only a fresh reported `RINGING` call generates an alert. The watch validates
canonical UUID v4 call identities, known states, bounded snapshots, server
sequence ordering and server-authored report age. Its thirty-second freshness
budget includes elapsed request time. Each notification times out when that
remaining budget ends. Old/ended reports and request failures remove alerts.
Re-reporting a call does not generate another first-seen alert during the watch;
restored notifications use suppressed group alerting. Android/user settings
ultimately determine sound and presentation.

Tapping an alert opens the exact call's review screen. An intent supplies only
navigation: the app fetches current authorized state, filters to that call ID,
and requests explicit confirmation before any command. If the call is gone,
another call cannot replace it. Commands retain server/child rollout flags,
emergency protection, exact identity, TTL, idempotency and observed-outcome rules.
Buttons expire locally as the report ages, including while confirmation is open.
Alerts never execute answer/reject/end directly.

Notifications contain no caller numbers, contact names, bearer tokens or media.
Lock-screen public content is generic. PendingIntents are explicit and immutable;
the service is not exported. No watch/call-state history is persisted. The process
retains at most 128 first-seen call identities during its observation window.

## Limits and acceptance

The optional `-ObserveCallWatchWindow` run on 26 October observed the production
service's automatic stop after **600807 ms**, checked that the parent session
and synthetic device were still valid, restarted the watch, and verified logout
cleanup. All 14 integration stages and fixture cleanup passed:
`.cache/android-server-https-reports/d1da1d46760e4a288dbb796e12700523.json`.
Run `scripts/test-android-server-https.ps1 -ObserveCallWatchWindow` to repeat it.
The marker is `parent_call_watch_window_probe=passed`. The test activity keeps
its own screen awake; this is visible-app elapsed deadline evidence, not OEM
background, sleep, process-death, real SIM or live Firebase acceptance.

On 26 October the side-by-side Android 37 acceptance app passed production UI
start/notification permission, visible service and ringing alert publication,
private/generic public notification content, immutable review-only intent,
fresh exact-call review with actions disabled, cleared-call alert removal,
stale exact-call-gone review, explicit Stop and logout cleanup. The reports are
synthetic authenticated server snapshots, not Telecom/SIM calls. Evidence:
`.cache/android-server-https-reports/d6ab3ce2fe064d248be437b44f827d71.json`;
all 13 integration stages and fixture cleanup passed. The required marker is
`server_https_parent_call_watch_probe=passed`. Notification permission remains
granted only on the emulator's disposable `.acceptance` app. Actual elapsed
ten-minute expiry, lock-screen presentation, process death, OEM background
behavior, real calls and live parent push remain unverified.

Elapsed realtime bounds observation to ten minutes. Expired watches do not start
another fetch or publish a late result. Stop, replacement and destruction cancel
the worker, clear timers and remove alerts. Android can suspend the process;
cleanup after a sleeping deadline occurs when it can run again. No exact alarm
or wake lock is requested. The service does not restart itself after process death
or reboot. Disabled notifications/channels and invalid sessions end monitoring;
transient request failures clear alerts and retry within the remaining window.

The service declares `dataSync` and handles Android's `onTimeout` by stopping.
Platform requirements are described in the official
[foreground-service types](https://developer.android.com/develop/background-work/services/fgs/service-types#data-sync)
and [service timeouts](https://developer.android.com/develop/background-work/services/fgs/timeout)
documentation. No Android limit is bypassed.

Focused JVM checks exercise freshness, request latency, old sequences, repeat
alerts, invalid identities/states, monotonic expiry and memory bounds. APK
compilation is separate from runtime acceptance. Notification permissions and
channel denial, background service delivery, taps, lock-screen privacy, expiry,
logout/stop/process death, real call outcomes and Redmi behavior remain device
acceptance gates. Always-on parent push and live Firebase delivery remain open;
P5-004 is not complete.
