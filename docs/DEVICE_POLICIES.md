# Device policy reconciliation and verification

<<<<<<< HEAD
26 October: IPv4 gateway TCP acceptance now passes selected-UID block/resume and management-UID reachability, without ADB reverse. Android 17 LAN consent fixes the earlier baseline timeout; the probe waits for framework VPN registration. This does not establish physical Redmi, Wi-Fi/mobile switching, DNS, IPv6 or Firebase behavior.

=======
>>>>>>> origin/main
The child now applies cached application restrictions through Device Owner
package suspension and has a local internet-pause VPN prototype. Both controls
still require physical Redmi acceptance evidence. No device was attached during
the logged verification on 23 October 2026.

## Desired state and child results

Parent policy writes remain versioned desired state. The authenticated child
posts `{"version":1,"result":"applied","reason":""}` to
`POST /v1/device/policy/report`. Allowed results are `applied`, `partial`, and
`failed`; non-applied reports require a fixed reason. Arbitrary text is rejected.
The report must match the current desired version. Duplicate identical reports
preserve their timestamp and do not append another audit event. A stale report
returns 409, so the next sync fetches and applies the latest snapshot.

The parent's `GET /v1/devices/{deviceID}/policy` includes optional `reported`
with its own version, result, reason and timestamp. A report for version 1 does
not confirm a new desired version 2. A matching applied report is a child claim
that the relevant Android API operations succeeded and package states were
checked, not proof of continuing physical behavior after later system changes.
Migration 0010 audits report changes transactionally and excludes policy content.

Transient `package_change_failed` results now request a background retry even
when download and result reporting succeeded. Both the network sync job and
offline policy job use the shared `PolicyRetry` decision; foreground sync also
requests a background attempt when needed. Offline jobs explicitly use the
same 30-second exponential backoff as synchronization. Retry requests use
[JobService.jobFinished](https://developer.android.com/reference/android/app/job/JobService#jobFinished(android.app.job.JobParameters,%20boolean));
Android controls actual execution timing, including idle-mode deferral.
Missing/protected targets, Device Owner setup and VPN permission requirements
do not trigger this transient-package retry path; periodic and event-driven
reconciliation remain available. Partial policy reports remain partial and
are never converted into applied reports merely because a retry was requested.

## Application blocks

On 24 October 2026 the production Android adapter passed an Android 37 Device
Owner emulator test with a disposable installed app. It verifies suspension,
same-policy reapplication, always-allowed restoration, management-target parser
rejection and cleanup. See [emulator policy evidence](EMULATOR_TESTS.md). These
results supplement the JVM checks; Redmi launch/reboot acceptance stays open.

The Android 37 reboot exercise also passed: the cached fixture policy/ledger
remained intact while the app was temporarily unsuspended, then the production
boot receiver/enforcer re-suspended it in a newer boot. Local boot-count/result
metadata is checked before test instrumentation starts. Allowlist restoration
and baseline cleanup pass afterward. See [the reboot exercise](EMULATOR_TESTS.md).
The boot count is local diagnostic metadata and does not change server reports.

`PackagePolicyEngine` keeps a private reconciliation ledger before suspending
an application. Removed targets are restored before new restrictions are added.
Failed restoration prevents adding further restrictions in that attempt.
An interrupted suspension remains in the ledger, so a later policy or restart
can release it. Already-converged packages are checked without repeating the OS
write, avoiding package-change feedback loops. The engine only restores targets
from its own ledger.

A later role change that excludes a package must not replace an already
detected package-change failure with `protected_package`. Error precedence now
preserves the transient failure so the retry can complete other pending changes.
JVM package checks cover failed suspension, retained ledger state, successful
retry, retry completion and protection changes during the same reconciliation.

The Android adapter uses
[`DevicePolicyManager.setPackagesSuspended`](https://developer.android.com/reference/android/app/admin/DevicePolicyManager#setPackagesSuspended(android.content.ComponentName,%20java.lang.String[],%20boolean))
and verifies the resulting suspension state. System and updated-system apps,
management namespaces, active admins, default home/dialer/SMS apps, keyboard,
enabled accessibility services, and VPN packages are excluded. Classification
failures are treated conservatively. Android also rejects some essential targets.

`PUT /v1/device/apps` uploads at most 2,000 app records in a request capped at
1 MiB. Records contain package name, label, system/blockable flags and observed
suspension. `GET /v1/devices/{deviceID}/apps` exposes the snapshot only to its
parent. System and fixed management/Android namespaces cannot be marked
blockable by an upload. Once inventory exists, policy writes reject unknown or
non-blockable targets. Before the first snapshot, existing namespace validation
and child runtime checks remain in force. Migration 0011 audits changed
snapshots without recording app names or labels in the audit table. Unchanged
snapshots preserve the last-change timestamp; this is not a heartbeat timestamp.

## Internet pause prototype

Select **Allow internet pause** on the child to complete Android's VPN consent
flow. Pause uses an explicit per-app list of non-critical user-installed apps;
system apps, management, FCM, phone/SMS, keyboard, accessibility and other VPN
apps stay outside. Preinstalled system applications are deliberately exempt in
this prototype. Empty target lists never establish a tunnel, since an empty
[Android per-app VPN list](https://developer.android.com/develop/connectivity/vpn#per-app)
would otherwise capture every app.

The service installs IPv4/IPv6 default routes and discards selected-app packets
locally without forwarding, recording or logging packet contents. It maintains
a visible notification and the system VPN indication. It uses the
[system-exempted foreground-service type](https://developer.android.com/develop/background-work/services/fgs/service-types#system-exempted)
for the managed-device/VPN use case. Always-on and lockdown are disabled during
feasibility. Resume closes this app's interface; activation failures return a
partial policy result. Superseded requests are invalidated, and nonblocking
packet reads use bounded polling so shutdown does not depend on another packet.

While active, the service observes package, dialer/SMS, keyboard and accessibility
changes and rebuilds its targets when necessary. It reuses an unchanged tunnel.
Actual OEM routing, DNS, consent revocation, power management and recovery must
be tested on hardware before production use.

## Background and offline behavior

A persisted, network-constrained `JobScheduler` job provides a 15-minute minimum
polling fallback with exponential retry; Android can defer execution. It is not
real-time delivery. Short-lived commands may expire before polling. FCM setup
and delivery remain separate work. The job reconciles cached policy before a
network sync, and boot/app-update receivers reconcile without server access.
Boot restoration occurs after credential-protected storage becomes available;
pre-unlock VPN enforcement is not implemented. Stopped jobs prevent subsequent
command actions and result submissions where cancellation can be observed.

## Evidence and physical checks still required

Full Go tests cover inventory privacy/isolation, critical-target rejection,
report validation and stale-version handling, audit failure rollback, HTTPS,
revocation and preservation through encrypted restore. JVM checks in
`scripts/test-package-policy.ps1` exercise critical roles, latest-policy
replacement, owned restrictions, restoration failure, failed ledger writes,
interrupted suspension recovery and VPN target selection.

On a provisioned Redmi, verify all of the following and record device/build IDs
through the logging wrapper before closing acceptance checks:

1. Suspend and restore a non-critical test app; reboot during a policy change.
2. Change desired policy multiple times offline, then reconnect and confirm
   only the latest desired version is reported applied.
3. Attempt critical-package blocks through inventory/API and inspect exclusion.
4. Pause a test app's Wi-Fi and cellular traffic, including IPv6 and DNS, while
   child-to-server HTTPS and call/recovery functions remain usable.
5. Remotely resume, revoke VPN consent, switch networks, kill the process and
   reboot; confirm recovery and accurate partial/failed reports.
6. Change default dialer/keyboard and install an app while paused; verify target
   refresh preserves critical connectivity and includes new eligible apps.

[Offline schedules and configurable always-allowed lists](OFFLINE_SCHEDULES.md)
are now implemented with format negotiation, state-aware reports and
permission-aware boundary alarms. Physical schedule acceptance, pre-unlock
enforcement, production always-on behavior and the real-call matrix remain open.
