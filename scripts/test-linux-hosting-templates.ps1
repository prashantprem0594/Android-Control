$ErrorActionPreference='Stop'
$workspace=Split-Path -Parent $PSScriptRoot
$deploy=Join-Path $workspace 'deploy/linux'
$bash='C:\Program Files\Git\bin\bash.exe'
if(-not (Test-Path -LiteralPath $bash)){throw 'Git Bash is required for local shell syntax checks.'}
foreach($name in @('backup.sh','preflight.sh')){
    $path=Join-Path $deploy $name
    $raw=[IO.File]::ReadAllText($path)
    if($raw.Contains("`r")){throw 'Linux shell assets must use LF line endings.'}
    & $bash -n $path
    if($LASTEXITCODE -ne 0){throw 'Linux shell syntax check failed.'}
}
$envText=Get-Content (Join-Path $deploy 'server.env.example') -Raw
$config=Get-Content (Join-Path $workspace 'server/internal/config/config.go') -Raw
foreach($match in [regex]::Matches($envText,'(?m)^(APP_CONTROL_[A-Z_]+)=')){
    if(-not $config.Contains('"'+$match.Groups[1].Value+'"')){throw 'Deployment environment name not supported by server.'}
}
foreach($required in @('APP_CONTROL_HTTP_ADDRESS=127.0.0.1:8080','APP_CONTROL_TRUSTED_PROXY_CIDRS=127.0.0.1/32','APP_CONTROL_REMOTE_CALLS_ENABLED=false','APP_CONTROL_PARENT_FCM_SENDER_ENABLED=false')){
    if($envText -notmatch ('(?m)^'+[regex]::Escape($required)+'\r?$')){throw 'Unsafe deployment default.'}
}
$preflight=Get-Content (Join-Path $deploy 'preflight.sh') -Raw
$protectedFunction=[regex]::Match($preflight,'(?ms)^protected_file\(\) \{\n.*?^\}\n').Value
if([string]::IsNullOrWhiteSpace($protectedFunction)){throw 'Host permission check function missing.'}
# Exercise the real permission validator with fixed stat metadata. The fixture file
# exists, but no host accounts, permissions or deployment configuration are changed.
$permissionFixture=Join-Path $workspace ('.cache/host-permission-fixture-'+[guid]::NewGuid().ToString('N'))
$null=New-Item -ItemType Directory -Path $permissionFixture
$permissionHarness=@'
set -euo pipefail
failures=0
pass() { :; }
fail() { failures=$((failures+1)); }
stat() {
    case "$2" in
        %U) printf '%s\n' root ;;
        %G) printf '%s\n' appcontrol ;;
        %a) if [[ "${@: -1}" == "$fixture" ]]; then printf '%s\n' "$fixture_mode"; else printf '%s\n' 755; fi ;;
        *) return 1 ;;
    esac
}
'@
$permissionHarness+="`n"+$protectedFunction+@'

fixture=$(cygpath -u "$0")
for fixture_mode in 640 644 600 660; do
    failures=0
    protected_file server_environment "$fixture" appcontrol 640
    if [[ "$fixture_mode" == 640 ]]; then
        (( failures == 0 )) || exit 1
    else
        (( failures == 1 )) || exit 1
    fi
done
printf '%s\n' 'Server environment permissions: 0640 accepted; 0644, 0600 and 0660 refused.'
'@
$permissionScript=Join-Path $permissionFixture 'permissions.sh'
[IO.File]::WriteAllText($permissionScript,$permissionHarness.Replace("`r`n","`n"),[Text.UTF8Encoding]::new($false))
& $bash $permissionScript
if($LASTEXITCODE -ne 0){throw 'Host environment permission regression failed.'}
if($preflight -match '(?m)^\s*(eval\s|source\s|\.\s|systemctl\s+(start|restart|enable|daemon-reload)|useradd\s|chmod\s|chown\s|rm\s)'){
    throw 'Read-only host preflight contains a mutation command.'
}
foreach($unitName in @('app-control.service','app-control-backup.service')){
    $unit=Get-Content (Join-Path $deploy $unitName) -Raw
    foreach($line in @('User=appcontrol','Group=appcontrol','NoNewPrivileges=yes','ProtectSystem=strict','UMask=0077')){
        if($unit -notmatch ('(?m)^'+[regex]::Escape($line)+'\r?$')){throw 'Service identity/isolation contract missing.'}
    }
}
Write-Output 'Linux template checks passed: shell syntax/LF, supported env names, loopback/disabled defaults, nonroot units and read-only preflight contract. Native host runtime not tested.'
