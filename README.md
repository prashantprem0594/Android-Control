# App Control

<<<<<<< HEAD
App Control is a privately distributed Android parental-control system for compatible fully managed Android devices, with Redmi phones as the current physical acceptance targets. The child app requires Android 9 or later; the parent app requires Android 13 or later. Broader device/feature acceptance remains pending. See [remaining work](docs/REMAINING_WORK.md).
=======
App Control is a privately distributed Android parental-control system for fully managed Redmi devices running Android 13 or later.
>>>>>>> origin/main

The product has four required controls:

- Lock the managed device.
- Pause internet access while preserving the management channel.
- Block selected applications.
- Handle cellular calls, including a parent-authorized remote answer command.

The system uses native Kotlin/Java Android applications, a self-hosted Go backend, and SQLite. The optional Firebase Cloud Messaging adapter carries opaque wake-up hints; live Firebase configuration remains pending. FCM never carries command or policy content, application credentials, or audit history.

## Repository status

Feasibility verification remains open while the server and child prototypes progress. The repository currently contains:

- The implementation roadmap and architecture decisions.
- An evidence-based phase tracker and progress-check script.
- A versioned command-envelope contract.
- A Go server with authentication, enrollment, revocation, immutable audits, encrypted backups and durable command delivery.
- A child app with Keystore authentication, command journaling, background sync, installed-app inventory and versioned policy reports.
- Device Owner app-blocking and local internet-pause prototypes; a native parent prototype now supports login, enrollment, policies, schedules and command history. See [parent application](docs/PARENT_APP.md).
- Offline time-zone schedules and always-allowed apps, with distinct desired and reported effective states.
- A visible default-dialer feasibility prototype with local call controls and both audio-routing API paths; real-call acceptance remains open.
- Bounded server ingress, explicit proxy trust and route-template logs; see [server protections](docs/SERVER_PROTECTION.md).
- An administration CLI for authenticated encrypted backups, restore and integrity checks; see [database recovery](docs/DATABASE_RECOVERY.md).
- Shared Android server-trust configuration with current/backup pins, a child origin gate and release configuration checks; see [server trust](docs/SERVER_TRUST.md).
- Opt-in private update screens, signed metadata, APK staging/replay checks and user-confirmed installer sessions; production installation acceptance remains open. See [private updates](docs/PRIVATE_UPDATES.md).

<<<<<<< HEAD
Remote assistance currently provides consent-based, authenticated JPEG frame viewing and
Accessibility-backed gestures. Smooth continuous WebRTC video remains future work. The
repository has signaling/session metadata support, but Android WebRTC media integration,
parent WebRTC rendering, and a deployed TURN relay are not yet included. See the [remote
assistance roadmap](docs/REMOTE_ASSISTANCE_ROADMAP.md).

For laptop development, ADB reverse mappings are maintained with
`scripts/maintain-laptop-phone-mappings.ps1`; run it while testing so USB reconnects restore
the `localhost:8443` mapping automatically. Production deployments should use a reachable
HTTPS server hostname instead of the laptop-only localhost profile.

=======
>>>>>>> origin/main
The next acceptance gate requires a physical Redmi: enrollment, lock/app controls, network preservation, reboot recovery and real-call feasibility are not yet verified. See [device policies](docs/DEVICE_POLICIES.md) and the authoritative tracker for evidence and remaining work.

The exact supported versions are pinned in [docs/TOOLCHAIN.md](docs/TOOLCHAIN.md).

<<<<<<< HEAD
The current [remaining-work checklist](docs/REMAINING_WORK.md) distinguishes completed local push/API/release checks from the device, Firebase, production and pilot acceptance still required.

Both native apps now share [polished UI components](docs/UI_POLISH.md), with grouped setup/control screens, clearer primary actions and distinct launcher icons. The child installation baseline is Android 9/API28+, independent of the designated test-phone model; see [Android compatibility](docs/ANDROID_COMPATIBILITY.md) for feature requirements and testing limits.

=======
>>>>>>> origin/main
## Layout

```text
contracts/   Shared protocol contracts
docs/        Architecture and delivery plan
tracking/    Machine-readable acceptance status
logs/build/  Dated command and output logs
server/      Self-hosted Go control server
android/     Parent and child Android projects (Phase 0 feasibility)
```

## Check delivery progress

```powershell
powershell -NoProfile -ExecutionPolicy Bypass -File .\scripts\check-progress.ps1
```

To require a particular phase to be complete:

```powershell
powershell -NoProfile -ExecutionPolicy Bypass -File .\scripts\check-progress.ps1 -Phase P0 -RequireComplete
```

## Run and log build commands

All build, test, provisioning, migration, packaging, and deployment commands must run through the logging wrapper:

```powershell
powershell -NoProfile -ExecutionPolicy Bypass -File .\scripts\run-logged.ps1 -Category test -Command "go test ./..." -WorkingDirectory .\server
```

Logs are written to `logs/build/YYYY-MM-DD.txt`. See [the build-log policy](docs/BUILD_LOG_POLICY.md).

Each command is assigned to a tracker/task ID. Token estimates can be summarized with:

```powershell
powershell -NoProfile -ExecutionPolicy Bypass -File .\scripts\summarize-task-tokens.ps1
```

## Run the server

Once Go is installed:

```powershell
cd server
go test ./...
go run ./cmd/control-server
```

The service exposes:

```text
GET /healthz
GET /readyz
```

