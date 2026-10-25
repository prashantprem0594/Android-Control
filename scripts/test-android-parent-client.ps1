param(
    [Parameter(Mandatory)] [ValidatePattern('^[A-Za-z0-9._:-]+$')] [string]$Serial,
    [ValidateRange(1024, 65535)] [int]$AdbServerPort = 5038
)
$ErrorActionPreference = 'Stop'
$workspaceRoot = Split-Path -Parent $PSScriptRoot
$adb = 'C:\Users\Sushant\AppData\Local\Android\Sdk\platform-tools\adb.exe'
$command = @"
`$probeOutput = @(& '$adb' -P $AdbServerPort -s '$Serial' shell am instrument -w -r -e probe parent_client com.appcontrol.parent.test/com.appcontrol.transport.ServerTrustInstrumentation 2>&1)
`$probeExit = `$LASTEXITCODE
`$probeOutput | Write-Output
`$probeText = `$probeOutput -join "``n"
if (`$probeExit -ne 0 -or
    `$probeText -notmatch '(?m)^INSTRUMENTATION_RESULT: parent_client_probe=passed\r?$' -or
    `$probeText -notmatch '(?m)^INSTRUMENTATION_CODE: -1\r?$') {
    throw 'Android parent client probe did not pass.'
}
Write-Output 'Parent client connection-double checks passed; platform TLS and live server acceptance remain separate.'
"@
& (Join-Path $PSScriptRoot 'run-logged.ps1') -TaskId P5-001 -Category device -WorkingDirectory $workspaceRoot -Command $command
exit $LASTEXITCODE
