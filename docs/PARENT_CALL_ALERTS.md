# Parent incoming-call watch

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
