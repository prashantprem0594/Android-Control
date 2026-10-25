$ErrorActionPreference = 'Stop'
$workspaceRoot = Split-Path -Parent $PSScriptRoot
$fixtureRoot = Join-Path $workspaceRoot ('.cache\trust-runner-' + [Guid]::NewGuid().ToString('N'))
New-Item -ItemType Directory -Path $fixtureRoot | Out-Null
$fakeAdb = Join-Path $fixtureRoot 'adb-fixture.cmd'
$runner = Join-Path $PSScriptRoot 'test-android-server-trust.ps1'

function Assert-ProbeResult([string[]]$Lines, [int]$NativeExit, [bool]$ShouldPass) {
    $fixture = @('@echo off') + @($Lines | ForEach-Object { 'echo ' + $_ }) + @('exit /b ' + $NativeExit)
    Set-Content -LiteralPath $fakeAdb -Value $fixture -Encoding ASCII
    $captured = @(& powershell.exe -NoProfile -ExecutionPolicy Bypass -File $runner -App child -Expect success -AdbExecutable $fakeAdb 2>&1)
    $passed = $LASTEXITCODE -eq 0
    if ($passed -ne $ShouldPass) { throw 'Trust runner accepted or rejected the wrong fixture.' }
}

Assert-ProbeResult @('INSTRUMENTATION_RESULT: trust_probe=passed', 'INSTRUMENTATION_CODE: -1') 0 $true
Assert-ProbeResult @('INSTRUMENTATION_RESULT: trust_probe=failed', 'INSTRUMENTATION_CODE: 0') 0 $false
Assert-ProbeResult @('INSTRUMENTATION_RESULT: trust_probe=passed', 'INSTRUMENTATION_CODE: 0') 0 $false
Assert-ProbeResult @('INSTRUMENTATION_FAILED: runner missing') 0 $false
Assert-ProbeResult @('INSTRUMENTATION_RESULT: trust_probe=passed', 'INSTRUMENTATION_CODE: -1') 1 $false
Write-Output 'Trust runner fixture checks passed: success, failed result, wrong code, absent runner and native failure.'
exit 0
