param(
    [Parameter(Mandatory)] [ValidateSet('prepare','restart','reboot','cleanup')] [string]$Phase,
    [Parameter(Mandatory)] [ValidateSet('127.0.0.1:5581','emulator-5580')] [string]$Serial,
    [ValidateRange(1024,65535)] [int]$AdbServerPort = 5038
)
# Observe one phase. Real force-stop and emulator reboot are separately logged.
$ErrorActionPreference = 'Stop'
$workspace = Split-Path -Parent $PSScriptRoot
$adb = Join-Path $workspace '.tooling\android-sdk\platform-tools\adb.exe'
$quotedAdb = "'"+$adb.Replace("'","''")+"'"
$expected = if ($Phase -eq 'prepare') { 'prepared' } elseif ($Phase -eq 'cleanup') { 'cleaned' } else { 'passed' }
$command = @"
`$output = @(& $quotedAdb -P $AdbServerPort -s '$Serial' shell am instrument -w -r -e probe contact_persistence -e phase $Phase com.appcontrol.child.test/com.appcontrol.transport.ServerTrustInstrumentation 2>&1)
`$nativeExit = `$LASTEXITCODE
`$output | Write-Output
`$proof = `$output -join "``n"
if (`$nativeExit -ne 0 -or
    `$proof -notmatch '(?m)^INSTRUMENTATION_RESULT: contact_persistence_probe=$expected\r?$' -or
    `$proof -notmatch '(?m)^INSTRUMENTATION_RESULT: contact_persistence_version=cache-persistence-v1\r?$' -or
    `$proof -notmatch '(?m)^INSTRUMENTATION_CODE: -1\r?$') { throw 'Encrypted contact persistence phase did not pass.' }
Write-Output 'Isolated encrypted contact persistence phase $Phase passed; no server or call used.'
"@
& (Join-Path $PSScriptRoot 'run-logged.ps1') -TaskId P4-001 -Category device -WorkingDirectory $workspace -Command $command
exit $LASTEXITCODE
