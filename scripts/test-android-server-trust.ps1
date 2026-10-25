param(
    [Parameter(Mandatory)] [ValidateSet('parent', 'child')] [string]$App,
    [Parameter(Mandatory)] [ValidateSet('success', 'tls_failure')] [string]$Expect,
    [string]$AdbExecutable = 'C:\Users\Sushant\AppData\Local\Android\Sdk\platform-tools\adb.exe'
)

$ErrorActionPreference = 'Stop'
$workspaceRoot = Split-Path -Parent $PSScriptRoot
$quotedAdb = "'" + ([IO.Path]::GetFullPath($AdbExecutable)).Replace("'", "''") + "'"
$component = 'com.appcontrol.' + $App + '.test/com.appcontrol.transport.ServerTrustInstrumentation'
# -d requires exactly one physical USB device. No implicit emulator selection,
# installation, trust-store modification or device reset occurs here.
$command = @"
`$probeOutput = @(& $quotedAdb -d shell am instrument -w -r -e expect $Expect $component 2>&1)
`$probeExit = `$LASTEXITCODE
`$probeOutput | Write-Output
if (`$probeExit -ne 0) { throw 'ADB instrumentation invocation failed.' }
`$probeText = `$probeOutput -join "``n"
if (`$probeText -notmatch '(?m)^INSTRUMENTATION_RESULT: trust_probe=passed\r?$' -or
    `$probeText -notmatch '(?m)^INSTRUMENTATION_CODE: -1\r?$') {
    throw 'Android server trust probe did not pass.'
}
Write-Output 'Android platform server trust probe passed.'
"@
& (Join-Path $PSScriptRoot 'run-logged.ps1') -TaskId P6-005 -Category device `
    -WorkingDirectory $workspaceRoot -Command $command
exit $LASTEXITCODE
