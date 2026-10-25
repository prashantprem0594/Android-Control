param(
    [Parameter(Mandatory)] [ValidateSet('prepare','restart','reboot','cleanup')] [string]$Phase,
    [Parameter(Mandatory)] [ValidatePattern('^[A-Za-z0-9._:-]+$')] [string]$Serial,
    [ValidateRange(1024,65535)] [int]$AdbServerPort = 5038
)
$ErrorActionPreference = 'Stop'
$workspaceRoot = Split-Path -Parent $PSScriptRoot
$adb = 'C:\Users\Sushant\AppData\Local\Android\Sdk\platform-tools\adb.exe'
$expected = if ($Phase -eq 'prepare') { 'prepared' } elseif ($Phase -eq 'cleanup') { 'cleaned' } else { 'passed' }
# This runner observes one phase only. Process termination and emulator reboot
# must be logged separately; instrumentation never reboots or kills the device.
$command = @"
`$output = @(& '$adb' -P $AdbServerPort -s '$Serial' shell am instrument -w -r -e probe parent_persistence -e phase $Phase com.appcontrol.parent.test/com.appcontrol.transport.ServerTrustInstrumentation 2>&1)
`$nativeExit = `$LASTEXITCODE
`$output | Write-Output
`$proof = `$output -join "``n"
if (`$nativeExit -ne 0 -or `$proof -notmatch '(?m)^INSTRUMENTATION_RESULT: parent_persistence_probe=$expected\r?$' -or `$proof -notmatch '(?m)^INSTRUMENTATION_CODE: -1\r?$') { throw 'Parent persistence phase did not pass.' }
Write-Output 'Synthetic parent vault persistence phase $Phase verified.'
"@
& (Join-Path $PSScriptRoot 'run-logged.ps1') -TaskId P5-001 -Category device -WorkingDirectory $workspaceRoot -Command $command
exit $LASTEXITCODE
