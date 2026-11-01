# Parent application

<<<<<<< HEAD
29 October: **Background alert status** is available before and after sign-in. It distinguishes builds with continuous incoming-call alerts from builds without them, shows sign-in/notification/background-restriction state, and opens Android app settings. See [screen-off acceptance](SCREEN_OFF_ACCEPTANCE.md) for the real Redmi 14 5G test matrix. UI build/lint passes; real screen-off delivery remains pending.

The optional parent Firebase variant now has a separate [authenticated event synchronization path](PARENT_FIREBASE_SYNC.md). Local implementation/build checks do not establish live Firebase or OEM delivery. The bounded foreground watch below remains separate.

26 October: the foreground setup button requests Android 17 home-network access. Denial keeps setup signed out and explains the retry; consent does not weaken HTTPS trust. Parent UI/control-intent and bounded CallWatchService runtime checks pass in disposable gateway acceptance apps; continuous push and physical call acceptance remain open.

The optional [incoming-call watch](PARENT_CALL_ALERTS.md) supplies bounded
parent-side alerts and fresh exact-call review. Continuous push delivery and physical call acceptance remain open; notification/watch runtime passes on the isolated AVD.
=======
The optional [incoming-call watch](PARENT_CALL_ALERTS.md) supplies bounded
parent-side alerts and fresh exact-call review. Continuous push delivery and
runtime notification/call acceptance remain open.
>>>>>>> origin/main

Parent-managed [contact shortcuts and Call Parent](CALL_CONTACTS.md) are available
from the device screen. Saving confirms server state; child delivery awaits sync.

The native Android parent prototype now launches `ParentActivity`, replacing the
original placeholder screen. It uses platform views and HTTPS without new
runtime dependencies. The backend and matching APKs must be updated together;
the server applies its bundled database migrations during normal startup.
Back up the database before an operational upgrade.

## Available workflows

- Register a parent account, sign in, resume a stored session and sign out.
- Browse parent-scoped device pages and issue a short-lived enrollment code.
- Inspect the last child contact and reported Android API, Device Owner,
  dialer role, VPN consent and exact-alarm access.
- Request an internet pause, select inventory-backed app blocks and
  always-allowed apps, and add/edit/remove explicit-zone weekly schedules.
<<<<<<< HEAD
- Request that the child open a reported launchable app or Android Settings.
  Android shows a child-side notification and requires the child user to tap it.
=======
>>>>>>> origin/main
- Queue an idempotent 60-second lock command and inspect paged command history.
- Revoke device credentials after an explicit confirmation.
- Open private app updates when the operator configures update authority;
  see [private updates](PRIVATE_UPDATES.md).

Saving a policy confirms server intent only. The display separately compares
desired version/effective-state key with the last child report. Queued and
delivered commands do not imply successful execution. Last contact is a
server-received capability report, not a persistent connection or proof the
device is online. Capability booleans are observations of setup state, not
hardware acceptance results. Unreported capability state remains unknown.

Contact freshness compares the server receipt time against the server's current
response time, avoiding the parent phone's clock. Reports less than 20 minutes
old show recent contact; older reports show **Offline or synchronization delayed**.
This is an observation threshold around the nominal 15-minute periodic sync,
not an uptime or delivery guarantee. Idle restrictions can delay jobs further.
A significant server clock rollback makes freshness uncertain. Older responses
without server time remain unknown, and malformed responses cannot crash the
screen's completion handler.

Policy updates preserve the edited snapshot's schedules and allowlist and send
its expected version. A concurrent edit conflicts instead of silently replacing
newer state. The schedule editor preloads all existing fields. Inventory excludes
protected targets from blocking choices; the server and child independently
enforce their protections. Offline restrictions may survive credential revocation;
restore restrictions and verify the child before removing server access.

The current lock retry key survives an ambiguous network result. Retrying that
intent retrieves the same command rather than issuing another physical action.
The key is cleared after the acknowledged server response is recorded. History
pages use newest-first timestamp/ID cursors and reconcile expired pending commands.
Refreshing from the first page includes new commands; pages are not a frozen
snapshot of concurrent activity.

## Session handling

Every network operation gates the HTTPS origin against compiled deployment
configuration, keeps platform trust/hostname checks and refuses redirects.
Responses are bounded, and network work runs away from the UI thread. The
deployment's current/backup pins still apply through Android Network Security
Configuration. See [server trust](SERVER_TRUST.md).

Access and rotating refresh tokens are stored as AES-GCM ciphertext under a
non-exportable Android Keystore key. The origin is authenticated as additional
data. Passwords are cleared from input fields after submission, excluded from
saved view state and never persisted. The parent window disables screenshots.
Session work is serialized across client instances so concurrent requests do
not reuse a rotating refresh token. An ambiguous refresh response clears local
credentials and requires sign-in; it is never retried automatically. HTTP 401
also clears local access. Local logout always clears the vault and explicitly
reports when server revocation could not be confirmed.

Operator password recovery is described in [parent recovery](PARENT_RECOVERY.md).
There is no public unauthenticated password-reset endpoint.

## API additions

| Endpoint | Meaning |
|---|---|
| `GET /v1/devices?limit=50&after=...` | Parent device list, bounded to 100, stable ID cursor |
| `POST /v1/device/status` | Signed-session child capability/contact report |
| `GET /v1/devices/{deviceID}/status` | Parent-scoped last received capability report |
| `GET /v1/devices/{deviceID}/commands?limit=50&after=...` | Parent-scoped command history, bounded to 100 |

Unknown, empty, duplicate or malformed paging parameters are rejected. Cross-parent
cursors never grant access to another parent's data. Capability reports require
all fields; false is distinct from omitted. Revoked/expired device sessions
cannot update last contact. No phone number, contact name or call content is
included in capability reports.

## Verification and remaining gates

<<<<<<< HEAD
The later 26 October production-screen run also passes active-device editing
against a server-only synthetic enrollment: create/remove an explicit UTC weekly
schedule, save/restore pause intent, show pending confirmation and unknown
capabilities, save a primary contact, reject a concurrent stale contact edit,
and display a queued lock command/history. The screen feedback now survives the
device-page refresh. The required additional marker is
`server_https_parent_controls_ui_probe=passed`. Evidence:
`.cache/android-server-https-reports/e04c520bb8d641deaa5332ef2f036061.json`;
all 13 integration stages and fixture cleanup passed. The synthetic device has
only an ephemeral test-process key, so no child applies a policy or command.
Actual parent-to-child controls, real phones and live call-watch behavior remain
separate acceptance gates.

On 26 October, the side-by-side `.acceptance` parent app also passed production
ParentActivity screens over live fixture HTTPS: incorrect-password rejection,
sign-in with cleared password fields, populated device list, saved-session
activity reopening, revoked-device refusal, fresh pairing-code display,
sign-out and signed-out activity reopening. The required result marker is
`server_https_parent_ui_probe=passed`. Report:
`.cache/android-server-https-reports/d989871e5d0a4ddcb697f6847039b39c.json`;
all 13 combined stages and fixture cleanup passed. The production activity uses
a temporary test-only CA/SPKI socket factory and an otherwise empty acceptance
app session, cleaned afterward. No physical phone is touched. This proves
activity reopening in the same process; live process death/reboot and
physical-phone acceptance remain unverified; the later active-device editing check is described above.

=======
>>>>>>> origin/main
The Android 37 acceptance emulator passed vault checks for encryption, distinct
nonces, non-exportability, origin binding, tamper rejection and clearing. Controlled
connection-double checks passed origin use, bearer requests, concurrent refresh
serialization, redirect refusal, uncertain-refresh handling and local logout.
The launch screen rendered server/account/password, login and registration controls.
<<<<<<< HEAD
These component checks alone do not establish live TLS, server login, child enrollment or physical
=======
These checks do not establish live TLS, server login, child enrollment or physical
>>>>>>> origin/main
policy execution from the parent UI.

The internally logged runners are `scripts/test-android-parent-session.ps1` and
`scripts/test-android-parent-client.ps1`, using the explicit acceptance-emulator
serial and isolated ADB port. Test fixtures use separate private preferences and
Keystore aliases and never access an existing production session.

Full authenticated parent-to-server-to-child Android acceptance and live process/reboot
session behavior, live operator recovery and Redmi coverage remain open.
Continuous call push, runtime call-watch delivery/actions, account self-service
and live FCM remain unfinished.
P5 remains in progress.

Synthetic vault lifecycle evidence (24 October 2026): ParentPersistenceProbe passed prepare, confirmed process stop/new-process verification, actual emulator reboot verification and cleanup. Boot count increased; unchanged ciphertext and nonce decrypted with the retained non-exportable Android Keystore key. The isolated fixture uses synthetic credentials and never reads or changes the production vault. This proves storage persistence, not live login, refresh, revocation or account recovery after reboot. An earlier post-reboot instrumentation crash and overlapping cleanup are excluded; the repeated strictly sequential phases passed. Run scripts/test-android-parent-persistence.ps1 with explicit prepare/restart/reboot/cleanup phases, serial and ADB port. Log process termination/reboot separately and wait for each phase to finish before starting the next. Interrupted tests require fixture cleanup.
