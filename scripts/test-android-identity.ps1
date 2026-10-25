param(
    [Parameter(Mandatory)] [ValidatePattern('^[A-Za-z0-9._:-]+$')] [string]$Serial,
    [ValidatePattern('^[a-f0-9]{64}$')] [string]$ExpectedFingerprint,
    [ValidateRange(1024, 65535)] [int]$AdbServerPort = 5037
)
$ErrorActionPreference = 'Stop'
$workspaceRoot = Split-Path -Parent $PSScriptRoot
$adb = 'C:\Users\Sushant\AppData\Local\Android\Sdk\platform-tools\adb.exe'
$expectedCheck = ''
if ($ExpectedFingerprint) {
    $expectedCheck = "if (`$fingerprint -ne '$ExpectedFingerprint') { throw 'Android identity changed across probe invocations.' }"
}
$command = @"
`$probeOutput = @(& '$adb' -P $AdbServerPort -s '$Serial' shell am instrument -w -r -e probe identity com.appcontrol.child.test/com.appcontrol.transport.ServerTrustInstrumentation 2>&1)
`$probeExit = `$LASTEXITCODE
`$probeOutput | Write-Output
`$probeText = `$probeOutput -join "``n"
if (`$probeExit -ne 0 -or
    `$probeText -notmatch '(?m)^INSTRUMENTATION_RESULT: identity_probe=passed\r?$' -or
    `$probeText -notmatch '(?m)^INSTRUMENTATION_CODE: -1\r?$') {
    throw 'Android Keystore identity probe did not pass.'
}
if (`$probeText -notmatch '(?m)^INSTRUMENTATION_RESULT: public_key_sha256=([a-f0-9]{64})\r?$') {
    throw 'Public identity fingerprint is missing.'
}
`$fingerprint = `$Matches[1]
$expectedCheck
Write-Output 'Android Keystore generation, non-exportability, proof verification and domain separation passed.'
"@
& (Join-Path $PSScriptRoot 'run-logged.ps1') -TaskId P1-004 -Category device -WorkingDirectory $workspaceRoot -Command $command
exit $LASTEXITCODE
