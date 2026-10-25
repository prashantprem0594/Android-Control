param(
    [Parameter(Mandatory)] [ValidatePattern('^[A-Za-z0-9._:-]+$')] [string]$Serial,
    [Parameter(Mandatory)] [string]$FixtureFile,
    [ValidateSet('parent','child')] [string]$App = 'child',
    [ValidateRange(1024, 65535)] [int]$AdbServerPort = 5038
)
$ErrorActionPreference = 'Stop'
$workspaceRoot = Split-Path -Parent $PSScriptRoot
$adb = 'C:\Users\Sushant\AppData\Local\Android\Sdk\platform-tools\adb.exe'
$fixtureInfo = Get-Item -LiteralPath $FixtureFile
if (-not ($fixtureInfo -is [IO.FileInfo]) -or $fixtureInfo.Length -gt 10000) { throw 'Public manifest fixture exceeds the size limit.' }
$bytes = [IO.File]::ReadAllBytes($fixtureInfo.FullName)
$fixture = [Text.Encoding]::UTF8.GetString($bytes) | ConvertFrom-Json
if ($fixture.fixtureOnly -ne $true -or $fixture.origin -ne 'https://release-manifest-fixture.example.org') { throw 'Expected an isolated public release fixture.' }
$encoded = [Convert]::ToBase64String($bytes)
$command = @"
`$probeOutput = @(& '$adb' -P $AdbServerPort -s '$Serial' shell am instrument -w -r -e probe release_manifest -e fixture '$encoded' com.appcontrol.$App.test/com.appcontrol.transport.ServerTrustInstrumentation 2>&1)
`$probeExit = `$LASTEXITCODE
`$probeOutput | Write-Output
`$probeText = `$probeOutput -join "``n"
if (`$probeExit -ne 0 -or
    `$probeText -notmatch '(?m)^INSTRUMENTATION_RESULT: release_manifest_probe=passed\r?$' -or
    `$probeText -notmatch '(?m)^INSTRUMENTATION_CODE: -1\r?$') {
    throw 'Android release manifest verification probe did not pass.'
}
Write-Output 'Android $App release metadata verification and rejection checks passed.'
"@
& (Join-Path $PSScriptRoot 'run-logged.ps1') -TaskId P7-002 -Category device -WorkingDirectory $workspaceRoot -Command $command
exit $LASTEXITCODE
