# Invoke through run-logged.ps1. Public fixtures only; no builds or deployment.
$ErrorActionPreference = 'Stop'
$workspace = Split-Path -Parent $PSScriptRoot
$root = Join-Path $workspace ('.cache\configuration-checks-' + [guid]::NewGuid().ToString('N'))
$null = New-Item -ItemType Directory -Path $root
$key = [byte[]]@(0x30,0x2a,0x30,0x05,0x06,0x03,0x2b,0x65,0x70,0x03,0x21,0x00) + [byte[]](1..32)
$config = [ordered]@{
    serverOrigin='https://configuration-fixture.invalid';currentPin=[Convert]::ToBase64String([byte[]](1..32))
    backupPin=[Convert]::ToBase64String([byte[]](33..64));apkCertificateSHA256=('a'*64)
    updatePublicKey=[Convert]::ToBase64String($key);updateMinimumSequence=1;versionCode=3
    versionName='0.1.2-fixture';remoteCallsEnabled=$false;firebase=$null
}
$exporter = Join-Path $PSScriptRoot 'export-production-configuration.ps1'
function Save-Input($Value) {
    $path = Join-Path $root ('input-' + [guid]::NewGuid().ToString('N') + '.json')
    [IO.File]::WriteAllText($path,($Value | ConvertTo-Json -Depth 6),[Text.UTF8Encoding]::new($false))
    return $path
}
function Refuses($Value,[bool]$Fixture) {
    $inputPath = Save-Input $Value
    $target = Join-Path $root ([guid]::NewGuid().ToString('N'))
    $refused = $false
    try { & $exporter -ConfigurationFile $inputPath -OutputDirectory $target -AllowFixture:$Fixture | Out-Null } catch { $refused = $true }
    if (-not $refused -or (Test-Path -LiteralPath $target)) { throw 'Invalid configuration was not refused before export.' }
}
Refuses $config $false
$inputPath = Save-Input $config
$target = Join-Path $root 'polling'
& $exporter -ConfigurationFile $inputPath -OutputDirectory $target -AllowFixture
$summary = Get-Content -LiteralPath (Join-Path $target 'configuration.json') -Raw -Encoding UTF8 | ConvertFrom-Json
$build = Get-Content -LiteralPath (Join-Path $target 'release-arguments.json') -Raw -Encoding UTF8 | ConvertFrom-Json
if ($summary.mode -cne 'fixture' -or $summary.productionReady -or $build.arguments -notcontains '-PappControlRemoteCallsEnabled=false' -or $build.tasks -notcontains ':child-app:assembleRelease') { throw 'Incorrect fixture export.' }
$refused = $false
try { & $exporter -ConfigurationFile $inputPath -OutputDirectory $target -AllowFixture | Out-Null } catch { $refused = $true }
if (-not $refused) { throw 'Existing output overwritten.' }
$config.currentPin = $config.backupPin
Refuses $config $true
$config.currentPin = [Convert]::ToBase64String([byte[]](1..32))
$config.versionCode = 1.5
Refuses $config $true
$config.versionCode = 3
$config.serverOrigin = 'https://configuration-fixture.invalid:443'
Refuses $config $true
$config.serverOrigin = 'https://configuration-fixture.invalid'
$config.privateKey = 'must-not-be-exported'
Refuses $config $true
$config.Remove('privateKey')
$config.firebase = [ordered]@{applicationId='1:12345:android:abcdef';apiKey=('x'*24);projectId='configuration-fixture';senderId='54321'}
Refuses $config $true
$config.firebase.senderId = '12345'
& $exporter -ConfigurationFile (Save-Input $config) -OutputDirectory (Join-Path $root 'firebase') -AllowFixture
$fcm = Get-Content -LiteralPath (Join-Path $root 'firebase\release-arguments.json') -Raw -Encoding UTF8 | ConvertFrom-Json
if ($fcm.tasks -notcontains ':child-app:assembleFcmRelease') { throw 'Wrong Firebase build task.' }
foreach ($folder in @('polling','firebase')) {
    $summary = Get-Content -LiteralPath (Join-Path $root "$folder\configuration.json") -Raw -Encoding UTF8 | ConvertFrom-Json
    foreach ($file in $summary.files) {
        if ((Get-FileHash -LiteralPath (Join-Path $root "$folder\$($file.name)") -Algorithm SHA256).Hash.ToLowerInvariant() -cne $file.sha256) { throw 'Export hash mismatch.' }
    }
}
Write-Output "production-configuration-valid fixture/refusal/polling/firebase/manifest checks; public fixtures at $root"
