#!/bin/bash
# Read-only checks for the fixed layout in SERVER_HOSTING.md. Never sources env files.
set -uo pipefail
umask 077
failures=0
running_origin=''
if (( $# > 0 )); then
    if (( $# != 2 )) || [[ "$1" != '--running' || ! "$2" =~ ^https://[a-z0-9.-]+(:[0-9]+)?$ ]]; then
        printf '%s\n' 'Usage: preflight.sh [--running https://controlled-hostname] (no trailing slash)' >&2
        exit 2
    fi
    running_origin=$2
fi
pass() { printf 'passed=%s\n' "$1"; }
fail() { printf 'missing_or_invalid=%s\n' "$1"; failures=$((failures+1)); }
for tool in stat readlink dirname getent awk grep bash find flock sha256sum systemd-analyze caddy runuser; do
    if command -v "$tool" >/dev/null 2>&1; then pass "tool_$tool"; else fail "tool_$tool"; fi
done
if (( EUID != 0 )); then
    printf '%s\n' 'Run as the host operator with root access to check private configuration permissions.' >&2
    exit 1
fi
if ! command -v stat >/dev/null 2>&1 || ! command -v readlink >/dev/null 2>&1; then exit 1; fi
if command -v getent >/dev/null 2>&1 && getent passwd appcontrol >/dev/null 2>&1 && getent group appcontrol >/dev/null 2>&1; then
    pass service_identity
else fail service_identity; fi
if command -v getent >/dev/null 2>&1 && getent passwd caddy >/dev/null 2>&1; then pass proxy_identity; else fail proxy_identity; fi
protected_file() {
    local label=$1 path=$2 required_group=${3:-root} required_mode=${4:-} owner group mode ancestor ancestor_owner ancestor_mode
    if [[ ! -f "$path" || -L "$path" ]]; then fail "$label"; return; fi
    owner=$(stat -c '%U' -- "$path") || { fail "$label"; return; }
    group=$(stat -c '%G' -- "$path") || { fail "$label"; return; }
    mode=$(stat -c '%a' -- "$path") || { fail "$label"; return; }
    if [[ "$owner" != root || ! "$group" =~ ^($required_group)$ || ! "$mode" =~ ^[0-7]{3,4}$ ]]; then fail "$label"; return; fi
    if (( (8#$mode & 8#7022) != 0 )); then fail "$label"; return; fi
    if [[ -n "$required_mode" && "$mode" != "$required_mode" ]]; then fail "$label"; return; fi
    ancestor=$(dirname -- "$path")
    while [[ "$ancestor" != / ]]; do
        if [[ ! -d "$ancestor" || -L "$ancestor" ]]; then fail "${label}_parent"; return; fi
        ancestor_owner=$(stat -c '%U' -- "$ancestor") || { fail "${label}_parent"; return; }
        ancestor_mode=$(stat -c '%a' -- "$ancestor") || { fail "${label}_parent"; return; }
        if [[ "$ancestor_owner" != root || ! "$ancestor_mode" =~ ^[0-7]{3,4}$ ]]; then fail "${label}_parent"; return; fi
        if (( (8#$ancestor_mode & 8#022) != 0 )); then fail "${label}_parent"; return; fi
        ancestor=$(dirname -- "$ancestor")
    done
    pass "$label"
}
release=$(readlink -f -- /opt/app-control/current 2>/dev/null) || release=''
if [[ "$release" != /opt/app-control/releases/* || ! -d "$release" ]]; then
    fail selected_release
else
    pass selected_release
    for name in control-server control-admin parent-recovery release-manifest; do
        protected_file "binary_$name" "$release/bin/$name"
        if [[ ! -x "$release/bin/$name" ]]; then fail "executable_$name"; fi
    done
    protected_file backup_runner "$release/deploy/backup.sh"
    if ! bash -n "$release/deploy/backup.sh" >/dev/null 2>&1; then fail backup_shell_syntax; else pass backup_shell_syntax; fi
fi
protected_file server_environment /etc/app-control/server.env appcontrol 640
for name in app-control.service app-control-backup.service app-control-backup.timer; do
    protected_file "unit_$name" "/etc/systemd/system/$name"
done
protected_file caddy_configuration /etc/caddy/Caddyfile 'root|caddy'
if [[ -r /etc/caddy/Caddyfile ]] && grep -Eq '^[[:space:]]*https://[^[:space:]]*[.]invalid([[:space:]:/]|$)|^[^#]*REPLACE_WITH_' /etc/caddy/Caddyfile; then
    fail active_proxy_placeholder
fi
protected_file backup_key /etc/app-control/backup-key.bin appcontrol
if [[ -f /etc/app-control/backup-key.bin ]] && [[ $(stat -c '%s' /etc/app-control/backup-key.bin) == 32 && $(stat -c '%a' /etc/app-control/backup-key.bin) == 640 ]]; then
    pass backup_key_shape_permissions
else fail backup_key_shape_permissions; fi
if [[ -r /etc/app-control/server.env ]]; then
    if grep -qx 'APP_CONTROL_HTTP_ADDRESS=127.0.0.1:8080' /etc/app-control/server.env &&
       grep -qx 'APP_CONTROL_DATABASE_PATH=/var/lib/app-control/app-control.db' /etc/app-control/server.env &&
       grep -qx 'APP_CONTROL_TRUSTED_PROXY_CIDRS=127.0.0.1/32' /etc/app-control/server.env; then
        pass fixed_loopback_topology
    else fail fixed_loopback_topology; fi
    if grep -Eq '^[[:space:]]*APP_CONTROL_[A-Z_]+=.*(REPLACE_WITH_|example[.]invalid)' /etc/app-control/server.env; then
        fail active_configuration_placeholder
    else pass no_active_configuration_placeholder; fi
fi
if command -v systemd-analyze >/dev/null 2>&1; then
    if systemd-analyze verify /etc/systemd/system/app-control.service /etc/systemd/system/app-control-backup.service /etc/systemd/system/app-control-backup.timer >/dev/null 2>&1; then
        pass systemd_configuration
    else fail systemd_configuration; fi
fi
if command -v caddy >/dev/null 2>&1 && command -v runuser >/dev/null 2>&1; then
    if runuser -u caddy -- caddy validate --config /etc/caddy/Caddyfile --adapter caddyfile >/dev/null 2>&1; then
        pass caddy_configuration_validation
    else fail caddy_configuration_validation; fi
fi
if [[ -n "$running_origin" ]]; then
    if command -v curl >/dev/null 2>&1; then
        if curl --fail --silent --max-time 5 --output /dev/null --url http://127.0.0.1:8080/readyz; then pass running_local_readiness; else fail running_local_readiness; fi
        if curl --fail --silent --max-time 10 --output /dev/null --url "$running_origin/readyz"; then pass running_https_readiness; else fail running_https_readiness; fi
    else fail tool_curl; fi
fi
printf '%s\n' 'scope=host-preflight-only; services_started=false; firebase_delivery_verified=false; phone_acceptance_verified=false'
if (( failures != 0 )); then printf 'failed_checks=%s\n' "$failures"; exit 1; fi
printf '%s\n' 'host_preflight=passed (runtime, key custody, backup restore and live delivery remain separate)'
