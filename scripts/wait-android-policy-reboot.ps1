param(
    [Parameter(Mandatory)] [ValidatePattern('^[A-Za-z0-9._:-]+$')] [string]$Serial,
    [ValidateRange(1024, 65535)] [int]$AdbServerPort = 5038,
    [ValidateRange(10, 180)] [int]$TimeoutSeconds = 90
)
$ErrorActionPreference = 'Continue'
$adb = 'C:\Users\Sushant\AppData\Local\Android\Sdk\platform-tools\adb.exe'
$fixtureText = @(& $adb -P $AdbServerPort -s $Serial shell run-as com.appcontrol.child cat shared_prefs/fixture-policy-reboot.xml 2>&1)
if ($LASTEXITCODE -ne 0) { throw 'Policy reboot fixture metadata unavailable.' }
try {
    [xml]$fixture = $fixtureText -join "`n"
    $bootNodes = $fixture.SelectNodes("/map/int[@name='bootCount']")
    $pendingNodes = $fixture.SelectNodes("/map/boolean[@name='pending']")
    if ($bootNodes.Count -ne 1 -or $pendingNodes.Count -ne 1) { throw 'Invalid fixture' }
    $preparedBoot = [int]$bootNodes[0].GetAttribute('value')
    if ($preparedBoot -lt 0 -or $pendingNodes[0].GetAttribute('value') -ne 'true') { throw 'Invalid fixture' }
} catch { throw 'Policy reboot fixture metadata is invalid.' }
$deadline = [DateTime]::UtcNow.AddSeconds($TimeoutSeconds)
while ([DateTime]::UtcNow -lt $deadline) {
    $bootText = @(& $adb -P $AdbServerPort -s $Serial shell settings get global boot_count 2>&1)
    $currentBoot = -1
    $parsed = [int]::TryParse(($bootText -join '').Trim(), [ref]$currentBoot)
    $cacheText = @(& $adb -P $AdbServerPort -s $Serial shell run-as com.appcontrol.child cat shared_prefs/policy-cache.xml 2>&1)
    if ($LASTEXITCODE -eq 0 -and $parsed -and $currentBoot -gt $preparedBoot) {
        try {
            [xml]$cache = $cacheText -join "`n"
            $attemptBoot = [int]$cache.SelectSingleNode("/map/int[@name='localBootCount']").GetAttribute('value')
            $version = [long]$cache.SelectSingleNode("/map/long[@name='lastAttemptedVersion']").GetAttribute('value')
            $localResult = $cache.SelectSingleNode("/map/string[@name='localResult']").InnerText
            if ($attemptBoot -eq $currentBoot -and $version -eq 101 -and $localResult -eq 'applied') {
                Write-Output 'Production cached-policy reconciliation completed in a newer Android boot.'
                exit 0
            }
        } catch { }
    }
    Start-Sleep -Seconds 1
}
throw 'Production boot policy reconciliation did not become ready.'
