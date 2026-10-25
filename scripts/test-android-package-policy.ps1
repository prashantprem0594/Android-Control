param(
    [Parameter(Mandatory)] [ValidatePattern('^[A-Za-z0-9._:-]+$')] [string]$Serial,
    [ValidateRange(1024, 65535)] [int]$AdbServerPort = 5038
)
$ErrorActionPreference = 'Stop'
$workspaceRoot = Split-Path -Parent $PSScriptRoot
$adb = 'C:\Users\Sushant\AppData\Local\Android\Sdk\platform-tools\adb.exe'
$command = @"
`$probeOutput = @(& '$adb' -P $AdbServerPort -s '$Serial' shell am instrument -w -r -e probe package_policy com.appcontrol.child.test/com.appcontrol.transport.ServerTrustInstrumentation 2>&1)
`$probeExit = `$LASTEXITCODE
`$probeOutput | Write-Output
`$probeText = `$probeOutput -join "``n"
if (`$probeExit -ne 0 -or
    `$probeText -notmatch '(?m)^INSTRUMENTATION_RESULT: package_probe=passed\r?$' -or
    `$probeText -notmatch '(?m)^INSTRUMENTATION_CODE: -1\r?$') {
    throw 'Android package policy probe did not pass.'
}
Write-Output 'Android Device Owner package suspension, restoration, management exclusion and fixture cleanup passed.'
"@
& (Join-Path $PSScriptRoot 'run-logged.ps1') -TaskId P3-002 -Category device -WorkingDirectory $workspaceRoot -Command $command
exit $LASTEXITCODE
