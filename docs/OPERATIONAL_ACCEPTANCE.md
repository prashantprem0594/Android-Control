# Operational acceptance record

These procedures prepare P7 exercises. They are not completed acceptance evidence.
Record operator/date, sanitized result, relevant build/database version, affected
device model/OS and recovery time for every actual exercise. Never record
passwords, tokens, private keys, phone numbers or real call content.

| Exercise | Procedure and required observation | Current evidence boundary |
|---|---|---|
| Lost server | Restore a verified encrypted backup onto a clean host using the controlled HTTPS origin/trust keys; verify original parent login or operator recovery, original child-key proof, policy convergence, command results and revocation. Do not reset device identity to conceal restoration failures. | Local Go recovery/HTTPS tests pass; clean deployed host and real Android reconnection pending. |
| Lost parent phone | Verify parent identity, perform operator password recovery, confirm old access/refresh rejection and new-parent login. Restore unwanted policy intent and review immutable audit. Active child enrollment must survive. | Local recovery and audit rollback tests pass; real-phone handover pending. |
| Lost child phone | Revoke the specific child's server credentials; verify future authentication fails. Retain audit. Replacing a child requires a new key and explicit enrollment; never copy a private Keystore identity. | Server revocation tests pass; physical loss/replacement procedure pending. |
| Changed router | Restore a supported connection without clearing enrollment. Verify cached policies still apply offline, then authenticated sync, inventory/report updates and expiring-command behavior after reconnect. | Emulator cached package reboot passes; real network/router exercise pending. |
| Changed SIM | Preserve management enrollment and inspect actual Telecom account selection, outgoing/incoming normal calls and allowed-contact policy. Re-run dual-SIM, audio and locked-screen checks. Use the approved carrier/device procedure for emergency-path testing. | Physical SIM/carrier tests pending. |
| Lost HTTPS keys | Use an already distributed backup pin while maintaining valid CA/hostname checks. If both keys are lost, deliver a trusted app update with new pins. There is no unpinned network fallback. | Platform trust build/probe tooling exists; A/B device rotation and trusted-update recovery pending. |
| Restriction recovery | While the child can synchronize, remove restrictions and confirm the current effective state before revocation/removal. Demonstrate recovery when the server is unavailable using the approved local provisioning/support procedure. Never claim revocation itself clears cached restrictions. | Package/allowlist emulator checks pass; complete offline local recovery workflow pending. |
| App update | Build/sign with the existing application identity and compiled server trust. Stage on an acceptance device; retain enrollment/key, Device Owner, cached policy, command journal and call role. Verify boot and interrupted-update recovery. | Isolated random-key signing checks and debug-signed Android 37 version-1-to-2 upgrade preserve Keystore identity, Device Owner and cached policy. Full release lint/reproducibility, production signer and enrolled-device continuity remain pending. |
| Rollback | Choose an approved compatible database/app rollback path, preserving identity and policy schema compatibility. Validate before broad rollout; do not use uninstall as rollback or silently enable downgrade/debug access. | Procedure design and device exercise pending. |

<<<<<<< HEAD
## Parent push after server recovery

Recover the parent routing-token encryption key separately from the encrypted
database backup. The database backup key and parent routing key have different
purposes; recovering only the database backup does not decrypt the saved push
registrations. Keep private key files outside evidence archives and logs. If the
original routing key is unavailable, restore authenticated polling first and
require parent installations to register current tokens under the replacement
key before enabling the sender. Never treat old ciphertext as a usable token.

The local recovery exercise in
`server/internal/parentpush/recovery_test.go` restores an actual encrypted SQLite
backup into a new directory after closing the source database. It checks the
original delivery identity and expiry, expired-lease retry with the retained
routing key, wrong-key refusal, refusal of a pre-outage lease, and suppression
after the 30-second call freshness window. It also runs operator password
recovery against the restored database and checks removal of the old parent
registration, cancellation of its delivery, denial of its event feed, preserved
active child enrollment and the immutable password-reset audit event.

This fixture exercise does not contact Firebase or establish clean deployed-host,
production-key custody, physical-phone reconnection or production restore
acceptance. Those observations remain required for P7-003 and P7-005.

=======
>>>>>>> origin/main
## Pilot record

The 72-hour reliability measurement and supervised one-to-two-week family pilot
require elapsed operation on the actual target Redmi devices. Keep an issue log
for unexpected restriction, missed restoration, call UI/routing failure, lost
command/result, battery/data excess and prolonged disconnection. Agree numerical
battery/data/latency targets with the operator before claiming those checks pass.
Collect fixed summaries and counters rather than logcat dumps or personal data.

Pause broad rollout for any issue that leaves restrictions unrecoverable, breaks
emergency calling, affects the wrong call, exposes credentials or reports
unacknowledged success. Record the fix and repeat the failed scenario before
continuing the pilot. No implementation/build test substitutes for elapsed pilot
evidence.
