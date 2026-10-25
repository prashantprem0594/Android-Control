param(
    [Parameter(Mandatory)] [ValidatePattern('^[A-Za-z0-9._:-]+$')] [string]$Serial,
    [ValidateRange(1024, 65535)] [int]$AdbServerPort = 5038,
    [ValidateRange(10, 60)] [int]$TimeoutSeconds = 45
)
$ErrorActionPreference = 'Continue'
$adb = 'C:\Users\Sushant\AppData\Local\Android\Sdk\platform-tools\adb.exe'
$metadata = @(& $adb -P $AdbServerPort -s $Serial shell run-as com.appcontrol.child cat shared_prefs/fixture-policy-upgrade.xml 2>&1)
if ($LASTEXITCODE -ne 0) { throw 'Upgrade fixture metadata unavailable.' }
try {
    [xml]$fixture = $metadata -join "`n"
    $oldVersion = [long]$fixture.SelectSingleNode("/map/long[@name='versionCode']").GetAttribute('value')
    if ($oldVersion -lt 1 -or $fixture.SelectSingleNode("/map/boolean[@name='pending']").GetAttribute('value') -ne 'true') { throw 'Invalid fixture' }
} catch { throw 'Upgrade fixture metadata is invalid.' }
$deadline = [DateTime]::UtcNow.AddSeconds($TimeoutSeconds)
while ([DateTime]::UtcNow -lt $deadline) {
    $package = @(& $adb -P $AdbServerPort -s $Serial shell dumpsys package com.appcontrol.child 2>&1) -join "`n"
    $version = [regex]::Match($package, '(?m)^\s*versionCode=([0-9]+) ')
    $raw = @(& $adb -P $AdbServerPort -s $Serial shell run-as com.appcontrol.child cat shared_prefs/policy-cache.xml 2>&1)
    if ($LASTEXITCODE -eq 0 -and $version.Success -and [long]$version.Groups[1].Value -gt $oldVersion) {
        try {
            [xml]$cache = $raw -join "`n"
            $attempt = [long]$cache.SelectSingleNode("/map/long[@name='lastAttemptedVersion']").GetAttribute('value')
            $result = $cache.SelectSingleNode("/map/string[@name='localResult']").InnerText
            $target = @(& $adb -P $AdbServerPort -s $Serial shell dumpsys package org.example.appcontrolfixture 2>&1) -join "`n"
            if ($LASTEXITCODE -eq 0 -and $attempt -eq 301 -and $result -eq 'applied' -and $target.Contains('suspended=true')) {
                Write-Output 'Production package-replaced reconciliation observed before verification instrumentation.'
                exit 0
            }
        } catch { }
    }
    Start-Sleep -Seconds 1
}
throw 'Upgrade policy reconciliation did not become ready.'
