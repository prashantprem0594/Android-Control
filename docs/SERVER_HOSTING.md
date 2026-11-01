# Always-on self-hosted server: Linux preparation

The server owns accounts, pairing, commands, policies, call-state authorization, SQLite and audit history. Firebase is an optional opaque wake-up transport. The Linux assets in [deploy/linux](../deploy/linux) prepare a single-host deployment; they create no cloud account, buy no host/domain, install nothing and contain no real credentials.

The topology is **phones -> controlled HTTPS hostname/Caddy -> 127.0.0.1:8080/control-server -> private SQLite**. The Go executable serves HTTP only; never expose its port directly. Caddy terminates TLS using operator-managed certificates whose SPKI keys match the current/backup pins compiled into the apps. Only HTTPS is exposed publicly. No Firebase authentication, database or hosting service is used.

## Required host and layout

Use a maintained Linux host with systemd, Bash, GNU find/coreutils, util-linux `flock` and Caddy. Supply the actual host/domain and current/backup CA-valid certificate chains before deployment. The reserved hostname in the example is intentionally unusable. Select an already reviewed matching Linux architecture from the [server bundle](SERVER_BUNDLES.md); verify the bundle and its trusted hash before copying binaries.

| Path | Owner/mode and purpose |
|---|---|
| `/opt/app-control/releases/<reviewed-version>/bin/` | root-owned, directories 0755, reviewed binaries 0755 |
| `/opt/app-control/releases/<reviewed-version>/deploy/backup.sh` and `preflight.sh` | root-owned 0755, reviewed backup/preflight runners |
| `/opt/app-control/current` | root-owned symlink to the selected immutable release |
| `/etc/app-control` | root:appcontrol directory 0750; config/credential files 0640 |
| `/var/lib/app-control` | appcontrol:appcontrol 0700, SQLite database/WAL/SHM |
| `/var/lib/app-control-backups` | appcontrol:appcontrol 0700, encrypted backups and private temporary snapshots |
| `/var/lib/app-control-recovery` | appcontrol:appcontrol 0700, private verification scratch |
| `/etc/caddy/app-control` | root:caddy 0750; certificate/key files 0640 |
| `/srv/app-control-public/releases/<sequence>/` | root-owned directories 0755, reviewed signed APKs 0644; no private files |

Create the dedicated `appcontrol` system user/group with no login shell using the target distribution's account tools. The backend and backup services run as this user. systemd creates their private state directories with the modes above. Keep app binaries, units, environment file and backup script unwritable by this account. The server unit hides the backup key/backup/scratch paths inside its service namespace. Review effective permissions as the service user on the target host; this is not a claimed host isolation test.

## Prepare a reviewable installation

Export an offline Linux deployment directory from the reviewed server bundle first. Supply the SHA-256 of its `bundle.json` obtained independently through the approved evidence channel, select the host architecture, and use a new output directory whose parent already exists:

```powershell
& .\scripts\run-logged.ps1 -TaskId LINUX-DEPLOYMENT-EXPORT -Category tooling -Command '& .\scripts\export-linux-deployment.ps1 -BundleDirectory .cache/server-release-001 -ExpectedBundleSHA256 <trusted-64-hex-manifest-digest> -Target linux-amd64 -OutputDirectory .cache/linux-deployment-001'
```

Replace the angle-bracket digest placeholder with the actual reviewed hash before running. The exporter calls the independent server-bundle verifier, requires all four tools from one selected target and checks their ELF64 architecture. It copies only matched tools into `bin/`, explicit Linux assets into `deploy/`, reviewed licenses, source metadata into `metadata/`, and this guide. `deployment.json` is written last with each copied file's size/SHA-256 and source-manifest digest. Unknown private metadata fields, traversal/alias paths, symlink/junction ancestors, wrong architecture, changed bytes and existing output directories are refused. Extra `.env`, service-account files and other unlisted bundle files are excluded. It executes no bundled binary, changes no permissions, installs nothing and includes no private configuration. `productionReady` and `runtimeAcceptancePassed` remain false. A completion record is not a signature or Linux runtime acceptance; incomplete output has no successful completion record and must not be deployed.

Retain the trusted source-manifest and exported completion-record hashes through the approved distribution channel, and verify the copied inventory again after transferring it to the host. Build outputs and this export do not carry Unix executable mode bits reliably across Windows transfers; set the documented root ownership/modes on the target host before preflight.

1. Export a current verified server bundle as above, then stage its `bin` and `deploy` directories under a new release; preserve the matched `control-server`, `control-admin`, `parent-recovery` and `release-manifest` set and both `backup.sh`/`preflight.sh` runners. Keep license notices and bundle/deployment inventories alongside the release. Use LF line endings for `.sh`, `.service`, `.timer`, Caddy and environment files.
2. Prepare the account/directories and root-owned `current` symlink using the reviewed release version. Do not overwrite running binaries or existing identity/key/database files.
3. Copy `server.env.example` to `/etc/app-control/server.env` and review its actual values. The listener is explicitly loopback-only and trusts only this same-host proxy. Do not put private key bytes or tokens in the environment; private settings are file paths. systemd EnvironmentFile syntax is not a shell script: no `export`, command substitution or shell-sourcing.
4. Copy `app-control.service`, `app-control-backup.service`, and `app-control-backup.timer` into `/etc/systemd/system/` as root-owned 0644 files. Keep the sender and remote-call rollout flags false until their separate acceptance gates pass. Never deploy the template's placeholder release-origin/certificate settings.
5. For a new deployment only, create the backup encryption key with the matching `control-admin keygen -key-file /etc/app-control/backup-key.bin`, then set root:appcontrol 0640. For a restored/existing system, retain its actual backup key instead. Parent and child routing-token keys are different 32-byte identities; preserve their existing files during update/restore. Credentials, keys and the database never belong under the public release directory.
6. Install the controlled HTTPS full chain/private key for Caddy. Replace `control.example.invalid` in `Caddyfile.example` with the exact app origin hostname. Integrate the site into the host's reviewed Caddy configuration; do not overwrite unrelated sites. No access log is enabled by this example; proxy runtime-error logging/privacy must still be exercised on the host. Firewall the backend port and expose only the chosen HTTPS edge and operator administration access.

The unit starts the server from `/var/lib/app-control`; opening a new database applies embedded migrations. On an existing deployment, take and independently verify a backup first, review migration compatibility, and schedule an operator-controlled restart. Starting is a deployment action, not a syntax check.

## Validate before starting

Run these commands on the prepared Linux host through the operator's sanitized deployment logger. They read configuration or check syntax; Caddy certificate access and binary existence must be checked as the actual service identities too.

```sh
bash -n /opt/app-control/current/deploy/backup.sh
systemd-analyze verify /etc/systemd/system/app-control.service /etc/systemd/system/app-control-backup.service /etc/systemd/system/app-control-backup.timer
caddy validate --config /etc/caddy/Caddyfile --adapter caddyfile
```

The exported `deploy/preflight.sh` performs the fixed-layout read-only host checks without installing accounts, changing permissions, editing config or starting services. After preparing paths and private configuration, run `sudo bash /opt/app-control/current/deploy/preflight.sh` before startup. After the reviewed services are started, use `sudo bash /opt/app-control/current/deploy/preflight.sh --running https://YOUR_CONTROLLED_HOSTNAME` to check local/external readiness without bypassing TLS trust. Use the actual hostname, not this placeholder. A passing preflight does not prove Firebase transport or phone delivery.

The environment file must be exactly root:appcontrol 0640. Preflight rejects both world-readable 0644 and root-only 0600; the latter prevents the service from reading its configuration. Executable fixture checks for these modes and group-writable 0660 passed on 30 October 2026 (`HOST-ENV-PERMISSIONS`). The refreshed offline staging directory `.cache/linux-deployment-20261030-permissions` includes this check; all 26 file sizes/hashes were independently verified (`HOST-ENV-EXPORT-VERIFY`). Its `deployment.json` SHA-256 is `66ee94f5443099d3492d0acb11df8982dc244f65a75eda50c4b28f09c2fecf6d`. Earlier staged exports lack this permission check; none establishes target-host acceptance.

Review [systemd execution isolation/environment semantics](https://www.freedesktop.org/software/systemd/man/latest/systemd.exec.html), [restart/start-limit behavior](https://www.freedesktop.org/software/systemd/man/latest/systemd.service.html), [Caddy reverse-proxy headers](https://caddyserver.com/docs/caddyfile/directives/reverse_proxy) and [manual TLS certificate loading](https://caddyserver.com/docs/caddyfile/directives/tls). The single trusted edge explicitly replaces X-Forwarded-For and removes other forwarded-address headers. Adding a CDN, second proxy or another host requires a new trust/topology review rather than broadening trusted networks blindly.

After reviewing actual files, the operator can run `systemctl daemon-reload`, enable/start `app-control.service`, and reload the validated Caddy configuration. Check `curl --fail http://127.0.0.1:8080/readyz` locally and the controlled HTTPS `/readyz` externally without bypassing CA/hostname verification. Readiness is database connectivity, not Firebase/phone acceptance. Examine sanitized `journalctl -u app-control` and confirm restart after host reboot. The restart limit prevents infinite rapid retries; fix a bad configuration and use `systemctl reset-failed app-control` before restarting.

## Enable only the small Firebase wake-up adapter

The parent routing key enables authenticated registration/feed support. The sender additionally needs the actual server-only Firebase service-account file and `APP_CONTROL_PARENT_FCM_SENDER_ENABLED=true`. Install those files root:appcontrol 0640, validate with the matching backend configuration, then restart under operator control. The child adapter has separate optional account/routing-key settings. Keep APK signing and update-authority private keys off the API host where operationally possible; never copy service-account JSON into an APK or public directory.

Build the Firebase-enabled parent with real public project/app settings and the same controlled HTTPS origin/pins. Verify token registration before enabling sends, then execute [screen-off acceptance](SCREEN_OFF_ACCEPTANCE.md). Firebase carries a fresh opaque `syncHint`; the app fetches authorized current data from this server. An FCM acceptance response does not prove device receipt or a displayed alert.

## Backup, update and restore

The backup oneshot uses the matching admin executable, exactly 32-byte key file, a private snapshot directory, non-overlapping `flock`, two-minute backup and verify deadlines and a six-minute systemd cap. The daily timer catches missed runs when the host returns, with bounded random delay. Enable its timer only after one manually started backup service succeeds and its result/journal is checked. At 30 matching backups the runner fails visibly; archive verified snapshots through the approved off-host process before clearing capacity. Nothing automatically deletes recovery evidence. The database backup and parent routing key must be recoverable separately.

For an update, retain a verified encrypted backup and previous immutable release. Stop the API service before changing the root-owned `current` symlink and restart only after reviewing schema compatibility. A symlink rollback does not downgrade a migrated database. Follow [database recovery](DATABASE_RECOVERY.md) for clean-path restore; never replace a live database or copy only the main SQLite file while ignoring WAL state. Restore onto a clean host/path first, preserve HTTPS/routing identities, independently verify integrity, then reconnect the actual phones. Test operational recovery before a family pilot.

For optional APK hosting, copy only the verified staging bundle's `releases` subtree under `/srv/app-control-public/`. The Caddy matcher exposes only the signed parent/child APK URL shape; manifest/public-key files stay at private backend configuration paths. This does not generate, sign, publish or approve a release.

## Evidence boundary

These templates have local static/shell checks only. No Linux service, Caddy TLS listener, cloud host, timer, production migration, live Firebase sender, off-host archive or real host restore was started by preparing them. Those target-host checks remain required.

Local validation on 29 October 2026 passed Bash syntax, active environment names/defaults against the server loader, service identity/sandbox invariants and HTTPS/loopback topology assertions (`SERVER-HOSTING-TEMPLATES` in `logs/build/2026-10-29.txt`). All deployment assets use LF line endings. Native `systemd-analyze verify` and `caddy validate` remain target-host steps.

The offline exporter additionally passes synthetic AMD64/ARM64 output inventories and tamper/missing-tool/wrong-digest/wrong-architecture/existing-output/traversal/alias/private-metadata/junction refusal cases (`LINUX-DEPLOYMENT-EXPORT`). Fixture ELF bytes are intentionally not executable programs and are never run. Tests retain a unique `.cache/linux-deployment-export-*` evidence directory.
