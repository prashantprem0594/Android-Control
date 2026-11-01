param(
    [Parameter(Mandatory)] [ValidateSet('prepare','restart','reboot','cleanup')] [string]$Phase,
    [Parameter(Mandatory)] [ValidateSet('127.0.0.1:5581','emulator-5580')] [string]$Serial,
    [ValidateRange(1024,65535)] [int]$AdbServerPort = 5038
)
# Public synthetic metadata only. Process stop/reboot are separately logged.
$ErrorActionPreference = 'Stop'
$workspace = Split-Path -Parent $PSScriptRoot
$adb = Join-Path $workspace '.tooling\android-sdk\platform-tools\adb.exe'
$quotedAdb = "'"+$adb.Replace("'","''")+"'"
$expected = if ($Phase -eq 'prepare') { 'prepared' } elseif ($Phase -eq 'cleanup') { 'cleaned' } else { 'passed' }
$command = @"
`$output = @(& $quotedAdb -P $AdbServerPort -s '$Serial' shell am instrument -w -r -e probe release_persistence -e phase $Phase com.appcontrol.child.test/com.appcontrol.transport.ServerTrustInstrumentation 2>&1)
`$nativeExit = `$LASTEXITCODE
`$output | Write-Output
`$proof = `$output -join "``n"
if (`$nativeExit -ne 0 -or
    `$proof -notmatch '(?m)^INSTRUMENTATION_RESULT: release_persistence_probe=$expected\r?$' -or
    `$proof -notmatch '(?m)^INSTRUMENTATION_RESULT: release_persistence_version=replay-persistence-v1\r?$' -or
    `$proof -notmatch '(?m)^INSTRUMENTATION_CODE: -1\r?$') { throw 'Release replay-floor persistence phase did not pass.' }
Write-Output 'Isolated release replay-floor persistence phase $Phase passed; no live keys, downloads or installation used.'
"@
& (Join-Path $PSScriptRoot 'run-logged.ps1') -TaskId P7-002 -Category device -WorkingDirectory $workspace -Command $command
exit $LASTEXITCODE
