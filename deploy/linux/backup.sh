#!/bin/bash
# Fixed deployment paths; do not source an operator-provided shell/environment file.
set -euo pipefail
umask 077

admin=/opt/app-control/current/bin/control-admin
database=/var/lib/app-control/app-control.db
key=/etc/app-control/backup-key.bin
backups=/var/lib/app-control-backups
scratch=/var/lib/app-control-recovery
lock=/run/app-control-backup/lock

if [[ ! -x "$admin" || ! -f "$database" || ! -f "$key" || ! -d "$backups" || ! -d "$scratch" ]]; then
    printf '%s\n' 'Backup prerequisites unavailable.' >&2
    exit 1
fi

exec 9>"$lock"
if ! flock -n 9; then
    printf '%s\n' 'Another backup is already active.' >&2
    exit 1
fi

# Fail visibly at capacity. Never delete recovery evidence automatically.
count=$(find "$backups" -maxdepth 1 -type f -name 'app-control-*.acb' -printf '.' | wc -c)
if (( count >= 30 )); then
    printf '%s\n' 'Backup capacity reached. Archive verified snapshots before retrying.' >&2
    exit 1
fi

snapshot="$backups/app-control-$(date -u +%Y%m%dT%H%M%SZ)-$$.acb"
"$admin" backup -db "$database" -backup "$snapshot" -key-file "$key" -timeout 2m
"$admin" verify -backup "$snapshot" -key-file "$key" -scratch-dir "$scratch" -timeout 2m
printf '%s\n' 'Encrypted backup and independent verification passed. Off-host archival remains separate.'
