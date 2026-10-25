# Phase verification tracker

The authoritative tracker is [`tracking/phase-status.json`](../tracking/phase-status.json). It records every required acceptance check, how it must be verified, and the evidence collected from the implementation.

## Status rules

Each check has one of these statuses:

| Status | Meaning |
|---|---|
| `not_started` | Implementation has not begun. |
| `in_progress` | Implementation exists but verification is incomplete. |
| `passed` | The verification method succeeded and evidence is recorded. |
| `failed` | Verification ran and found incorrect behavior. |
| `blocked` | Verification cannot proceed because a prerequisite is unavailable. |

A required check cannot be marked `passed` without at least one evidence entry. Evidence can be a test file, test report, command output, screenshot, device/build identifier, or a dated test note.

## Computed phase state

The reporting script computes phase state from checks instead of trusting a manually entered percentage:

- `passed`: every required check passed and has evidence.
- `failed`: at least one required check failed.
- `blocked`: no required check failed, but at least one is blocked.
- `in_progress`: work or partial verification exists.
- `not_started`: no check has begun.

## Commands

Show the full dashboard:

```powershell
powershell -NoProfile -ExecutionPolicy Bypass -File .\scripts\check-progress.ps1
```

Show one phase:

```powershell
powershell -NoProfile -ExecutionPolicy Bypass -File .\scripts\check-progress.ps1 -Phase P4
```

Use the tracker as a release gate:

```powershell
powershell -NoProfile -ExecutionPolicy Bypass -File .\scripts\check-progress.ps1 -Phase P6 -RequireComplete
```

`-RequireComplete` returns a failing exit code unless every selected required check has passed with evidence. This can later run in CI before creating a signed APK.

## Evidence standard

Code existence alone is not sufficient evidence for behavior that depends on Android or Redmi firmware. For example:

- Device Owner checks require the physical device model, Android/HyperOS build, and test result.
- Call checks require a real SIM-call test and the Telecom result.
- Reboot checks require post-reboot state evidence.
- Security checks require an automated test or a documented attempted unauthorized action.
- Backup checks require a successful restore, not merely creation of a backup file.

The tracker is updated in the same change as the implementation or test evidence. A phase is never closed based only on an estimate.

