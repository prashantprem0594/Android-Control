param(
    [Parameter(Mandatory)] [ValidatePattern('^[A-Za-z0-9._:-]+$')] [string]$Serial,
    [ValidateRange(1024,65535)] [int]$AdbServerPort = 5038
)
$ErrorActionPreference = 'Stop'
$workspace = Split-Path -Parent $PSScriptRoot
$adb = Join-Path $workspace '.tooling\android-sdk\platform-tools\adb.exe'
$quotedAdb = "'" + $adb.Replace("'", "''") + "'"
$command = @"
`$output = @(& $quotedAdb -P $AdbServerPort -s '$Serial' shell am instrument -w -r -e probe contact_cache com.appcontrol.child.test/com.appcontrol.transport.ServerTrustInstrumentation 2>&1)
`$exitCode = `$LASTEXITCODE
`$output | Write-Output
`$proof = `$output -join "``n"
if (`$exitCode -ne 0 -or
    `$proof -notmatch '(?m)^INSTRUMENTATION_RESULT: contact_cache_probe=passed\r?$' -or
    `$proof -notmatch '(?m)^INSTRUMENTATION_RESULT: contact_cache_version=sealed-cache-v1\r?$' -or
    `$proof -notmatch '(?m)^INSTRUMENTATION_RESULT: contact_cache_cleanup=passed\r?$' -or
    `$proof -notmatch '(?m)^INSTRUMENTATION_CODE: -1\r?$') { throw 'Android encrypted contact cache verification did not pass.' }
Write-Output 'Real Android Keystore contact round-trip, versions, enrollment binding, tamper rejection and fixture cleanup passed. No network or calls.'
"@
& (Join-Path $PSScriptRoot 'run-logged.ps1') -TaskId P4-001 -Category device -WorkingDirectory $workspace -Command $command
exit $LASTEXITCODE
