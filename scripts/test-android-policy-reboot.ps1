param(
    [Parameter(Mandatory)] [ValidateSet('Prepare', 'Verify', 'Cleanup')] [string]$Phase,
    [Parameter(Mandatory)] [ValidatePattern('^[A-Za-z0-9._:-]+$')] [string]$Serial,
    [ValidateRange(1024, 65535)] [int]$AdbServerPort = 5038
)
$ErrorActionPreference = 'Stop'
$workspaceRoot = Split-Path -Parent $PSScriptRoot
$adb = 'C:\Users\Sushant\AppData\Local\Android\Sdk\platform-tools\adb.exe'
$mode = $Phase.ToLowerInvariant()
$expected = @{ Prepare = 'prepared'; Verify = 'passed'; Cleanup = 'cleaned' }[$Phase]
$command = @"
`$probeOutput = @(& '$adb' -P $AdbServerPort -s '$Serial' shell am instrument -w -r -e probe policy_reboot -e phase $mode com.appcontrol.child.test/com.appcontrol.transport.ServerTrustInstrumentation 2>&1)
`$probeExit = `$LASTEXITCODE
`$probeOutput | Write-Output
`$probeText = `$probeOutput -join "``n"
if (`$probeExit -ne 0 -or
    `$probeText -notmatch '(?m)^INSTRUMENTATION_RESULT: policy_reboot_probe=$expected\r?$' -or
    `$probeText -notmatch '(?m)^INSTRUMENTATION_CODE: -1\r?$') {
    throw 'Android cached policy reboot phase did not pass.'
}
Write-Output 'Android cached policy reboot phase $mode completed.'
"@
& (Join-Path $PSScriptRoot 'run-logged.ps1') -TaskId P6-001 -Category device -WorkingDirectory $workspaceRoot -Command $command
exit $LASTEXITCODE
