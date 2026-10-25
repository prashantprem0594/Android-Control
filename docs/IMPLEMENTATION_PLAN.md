# Implementation plan

## Delivery strategy

The project is delivered through gated phases. A phase is complete only when its acceptance checks pass on a physical Redmi device. The highest-risk capability, Android call control, is tested before significant dashboard work.

For one experienced developer, the expected duration is approximately 10–14 weeks. Redmi/HyperOS compatibility and default-dialer behavior are the largest schedule risks.

## Phase 0 — Feasibility and toolchain (week 1)

Build small vertical experiments on physical Redmi devices before building the full product.

Work:

- Install Go, Android Studio, Android SDK, platform tools, and device USB drivers.
- Create the child feasibility application in Kotlin.
- Factory-reset and provision a Redmi as Device Owner.
- Verify `DevicePolicyManager.lockNow()`.
- Verify package suspension and restoration.
- Verify a local `VpnService` can block a selected application without blocking the management channel.
- Implement the minimum default-dialer components: `ACTION_DIAL`, `InCallService`, incoming-call UI, and active-call UI.
- Answer, reject, and end a real SIM call on Android 13 and Android 14+.
- Route an answered call to speaker on both Telecom API paths.
- Create a Firebase Spark project with only Cloud Messaging enabled and no billing account.
- Verify that an FCM wake-up hint causes the child to fetch a command from the local development server.

Exit gate:

- All four required capabilities work independently on the target Redmi hardware.
- Emergency calling still routes through Android correctly.
- Call control works while the screen is locked.
- Known MIUI/HyperOS differences are documented.

If call answering fails this gate, the project pauses for a Telecom-specific redesign before product development continues.

## Phase 1 — Secure foundation (weeks 2–3)

Build the durable backend and shared protocol.

Work:

- Implement configuration, structured logging, health checks, and graceful shutdown.
- Add SQLite in WAL mode, migrations, transactions, and encrypted backups.
- Implement parent accounts with Argon2id password hashing and rotating refresh tokens.
- Implement one-time enrollment codes.
- Generate device keys in Android Keystore and register their public keys.
- Implement signed device challenges and credential revocation.
- Version command and policy contracts.
- Add immutable audit events for security-sensitive actions.

Exit gate:

- One parent can enroll a clean device.
- Another account cannot read or command that device.
- Revoking the device prevents further authenticated requests.
- Database recovery from a backup is demonstrated.

## Phase 2 — Reliable command channel (weeks 3–4)

Build command delivery before implementing every command type.

Work:

- Store every command before attempting delivery.
- Add FCM as a replaceable wake-up provider.
- Send only opaque synchronization hints through FCM.
- Add authenticated HTTPS command fetch and acknowledgement.
- Add optional WebSocket status updates while apps are active.
- Implement command expiry, idempotency, retries, and deduplication.
- Implement policy-version reconciliation after reboot or reconnect.
- Show queued, delivered, succeeded, failed, and expired states.

Exit gate:

- A command issued while the child is offline executes or expires correctly after reconnection.
- Duplicate delivery does not repeat a completed action.
- FCM failure does not lose the command.
- The parent never reports success before receiving a child acknowledgement.

## Phase 3 — Device controls (weeks 4–6)

Implement the three non-call controls as independent commands and persistent policies.

Work:

- `LOCK_DEVICE` using Device Policy APIs.
- `SET_APP_BLOCKS` using package suspension.
- Installed-app inventory with critical packages excluded from blocking.
- `PAUSE_INTERNET` and `RESUME_INTERNET` through the local VPN policy engine.
- Preserve backend, FCM, Telecom, emergency, and recovery connectivity during internet pause.
- Add always-allowed applications.
- Add school, study, bedtime, and custom schedules.
- Apply the last downloaded policy while offline.

Exit gate:

- Each control works after reboot and without an active dashboard.
- Internet can always be restored remotely or with the recovery procedure.
- System UI, emergency calls, Telecom, and the management application cannot be accidentally blocked.

## Phase 4 — Production call handling (weeks 6–8)

Turn the Phase 0 Telecom experiment into a complete, recoverable default dialer.

Work:

- Dial pad and outgoing-call flow.
- Incoming and active call screens.
- Call state reporting: ringing, active, disconnected, and failed.
- Parent commands for answer, reject, and end.
- Speaker, earpiece, wired headset, and Bluetooth routing.
- Short command expirations and exact call-state validation.
- Approved-contact policy and Call Parent shortcut.
- Dual-SIM testing.
- Fallback to the system dialer if our service fails.

Exit gate:

- Normal and remote-controlled calls pass the Redmi device matrix.
- Remote call commands cannot affect the wrong or already-ended call.
- Emergency calling and crash recovery pass destructive testing.
- The active call remains visibly indicated on the child device.

## Phase 5 — Parent application (weeks 8–10)

Build the parent experience on the proven backend and device capabilities.

Work:

- Login and session management.
- Device enrollment and capability status.
- Online/offline and last-seen indicators.
- Lock, internet, and application-control screens.
- Incoming-call event and remote call actions.
- Schedules and always-allowed application management.
- Command history with pending, success, failure, and expiry states.
- Recovery workflow and device removal.

Exit gate:

- A parent can complete setup and operate every required capability without developer tools.
- The interface clearly distinguishes requested state from confirmed device state.

## Phase 6 — Hardening and reliability (weeks 10–12)

Work:

- Test reboot, process death, network switching, airplane mode, and server downtime.
- Test Android 13, Android 14, and available later versions on Redmi/MIUI/HyperOS.
- Test duplicate, expired, delayed, and out-of-order commands.
- Add rate limiting, request-size limits, token revocation, and audit review.
- Redact secrets, phone numbers, and sensitive payloads from logs.
- Add database integrity checks and automated encrypted backups.
- Pin server trust and implement safe certificate rotation.
- Measure battery, mobile data, reconnection latency, and FCM delivery latency.

Exit gate:

- No critical failure leaves the phone permanently restricted.
- Recovery works without editing the database or rebuilding the APK.
- A 72-hour pilot has no unexplained disconnections or policy drift.

## Phase 7 — Private release and pilot (weeks 12–14)

Work:

- Produce reproducible signed parent and child APKs.
- Host a signed release manifest and APKs on the self-hosted server.
- Add staged update and rollback procedures.
- Provision the actual family devices.
- Run a one-to-two-week supervised pilot.
- Record support procedures for lost server access, changed SIM, changed router, and device replacement.

Exit gate:

- The production server can be restored from backup.
- Both applications can be updated without losing enrollment.
- Required controls are reliable on every enrolled Redmi device.

## Explicitly deferred

- iOS support.
- Google Play distribution.
- Cellular call recording.
- Hidden microphone streaming.
- Multi-family commercial tenancy.
- Custom Android firmware or proprietary hardware.

