# Windows encrypted-backup task bundle

`scripts/export-backup-task.ps1` builds Task Scheduler XML, a PowerShell launcher
and hash inventory. Exporting does not register a task, run a backup, create a
database/key, change ACLs or store account passwords.

The launcher uses `scripts/backup-database.ps1` for encrypted backup followed by
independent verification. It pins the reviewed script and administration binary
hashes; changes require a reviewed re-export. Paths are quoted literals, and
execution/failure passes through the command logger.

## Export parameters

Run through `scripts/run-logged.ps1` with task `P6-005` and provide:

- `DatabasePath`: existing server database on the deployment host.
- `BackupDirectory`: local encrypted-snapshot storage.
- `KeyFile`: protected 32-byte key outside snapshots and scratch.
- `ScratchDirectory`: separate private local verification storage.
- `OutputDirectory`: fresh task-bundle directory.
- `AdminExecutable`: matching reviewed binary, if not the default.
- `PrincipalSid`: deployment user's SID; omission uses the exporting user.

Database/key contents are not read by export. Tooling must exist. UNC paths,
overlapping backup/scratch directories, keys/source databases inside them and
incompatible built-in service-account SIDs are refused. These are lexical
layout checks; deployment ACLs, filesystem aliases and identity still need
operator review.

The default interval is daily. `IntervalMinutes` supports 15, 30, 60, 120, 240,
360, 720 or 1440 minutes. Bounded within-day repetition avoids an exact duplicate
at the day boundary. Missed starts may run when available; the task does not
wake a sleeping machine. `IgnoreNew` and the backup command's lock prevent
overlapping work. Runtime allows both operation deadlines plus cleanup.

The existing default cap is 30 snapshots. Reaching `MaxBackups` fails visibly
rather than automatically deleting recovery evidence. Archive verified backups
through the approved process before clearing capacity.

## Deployment boundary

The template uses S4U/least privilege. Microsoft documents that this
[logon mode](https://learn.microsoft.com/en-us/windows/win32/taskschd/taskschedulerschema-logontype-simpletype)
stores no password and cannot access network resources or EFS-encrypted files.
This is a local-backup template; off-host archival needs a separate approved
identity/channel. The application's encrypted `.acb` format remains usable
when ordinary ACLs permit reading its binary key.

The selected account needs appropriate tooling, SQLite/WAL and key access plus
write access to private backup, scratch and log directories. Protect tooling
and launcher against modification by other accounts. Do not publish the
operational task bundle with public APK releases.

Review the three output files and hashes, select the actual account, then use
the operator's Windows Task Scheduler import/registration procedure. Confirm
schedule, last result, hashed command log and verified snapshot under that
identity. Microsoft's [daily trigger XML example](https://learn.microsoft.com/en-us/windows/win32/taskschd/daily-trigger-example--xml-)
documents the format. The exporter runs no registration command.

A synthetic bundle exported successfully; XML/settings and launcher syntax
checks passed without opening its nonexistent database/key. Task Scheduler
in-memory schema validation was unavailable. No task was registered/executed.
Unattended identity/ACL behavior, production restore, off-host archival and
monitoring remain acceptance work.
