param(
    [Parameter(Mandatory)] [ValidatePattern('^[A-Za-z0-9._:-]+$')] [string]$Serial,
    [ValidateRange(1024,65535)] [int]$AdbServerPort = 5038
)
$ErrorActionPreference = 'Stop'
$workspaceRoot = Split-Path -Parent $PSScriptRoot
$adb = 'C:\Users\Sushant\AppData\Local\Android\Sdk\platform-tools\adb.exe'
$command = @"
`$output = @(& '$adb' -P $AdbServerPort -s '$Serial' shell am instrument -w -r -e probe release_artifact com.appcontrol.parent.test/com.appcontrol.transport.ServerTrustInstrumentation 2>&1)
`$nativeExit = `$LASTEXITCODE
`$output | Write-Output
`$proof = `$output -join "``n"
if (`$nativeExit -ne 0 -or `$proof -notmatch '(?m)^INSTRUMENTATION_RESULT: release_artifact_probe=passed\r?$' -or `$proof -notmatch '(?m)^INSTRUMENTATION_RESULT: release_artifact_version=staging-replay-v1\r?$' -or `$proof -notmatch '(?m)^INSTRUMENTATION_CODE: -1\r?$') { throw 'APK artifact inspection did not pass.' }
Write-Output 'Actual APK inspection and rejection checks passed; download/install/production release remain separate.'
"@
& (Join-Path $PSScriptRoot 'run-logged.ps1') -TaskId P7-002 -Category device -WorkingDirectory $workspaceRoot -Command $command
exit $LASTEXITCODE
