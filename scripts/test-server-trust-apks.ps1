$ErrorActionPreference = 'Stop'
$workspaceRoot = Split-Path -Parent $PSScriptRoot
$aapt = Join-Path $workspaceRoot '.tooling\android-sdk\build-tools\36.0.0\aapt.exe'
Add-Type -AssemblyName System.IO.Compression.FileSystem

function Test-ContainsProbe([string]$Apk, [string]$Descriptor = 'Lcom/appcontrol/transport/ServerTrustInstrumentation;') {
    $archive = [IO.Compression.ZipFile]::OpenRead($Apk)
    try {
        foreach ($entry in $archive.Entries) {
            if ($entry.FullName -notmatch '^classes[0-9]*\.dex$') { continue }
            $stream = $entry.Open()
            $buffer = New-Object IO.MemoryStream
            try {
                $stream.CopyTo($buffer)
                if ([Text.Encoding]::ASCII.GetString($buffer.ToArray()).Contains($Descriptor)) {
                    return $true
                }
            } finally { $stream.Dispose(); $buffer.Dispose() }
        }
        return $false
    } finally { $archive.Dispose() }
}

foreach ($app in @('parent', 'child')) {
    $module = Join-Path $workspaceRoot ('android\' + $app + '-app')
    $apk = Join-Path $module ('build\outputs\apk\debug\' + $app + '-app-debug.apk')
    $testApk = Join-Path $module ('build\outputs\apk\androidTest\debug\' + $app + '-app-debug-androidTest.apk')
    if (Test-ContainsProbe $apk) { throw 'TLS probe was packaged in the application APK.' }
    if (-not (Test-ContainsProbe $testApk)) { throw 'TLS probe is absent from test APK.' }
    $fixtureClasses = if ($app -eq 'parent') { @('Lcom/appcontrol/parent/ParentSessionProbe;', 'Lcom/appcontrol/parent/ParentClientProbe;') } else { @('Lcom/appcontrol/child/ScheduleBoundaryProbe;', 'Lcom/appcontrol/child/PolicyUpgradeProbe;') }
    if ($app -eq 'child') { $fixtureClasses += 'Lcom/appcontrol/child/TelecomFixtureProbe;' }
    if ($app -eq 'parent') { $fixtureClasses += 'Lcom/appcontrol/parent/ParentPersistenceProbe;' }
    if ($app -eq 'parent') { $fixtureClasses += 'Lcom/appcontrol/transport/ReleaseArtifactProbe;' }
    $assetArchive = [IO.Compression.ZipFile]::OpenRead($apk)
    try {
        if ($null -ne $assetArchive.GetEntry('assets/fixture-release.apk')) { throw 'Release APK fixture was packaged in an application APK.' }
    } finally { $assetArchive.Dispose() }
    if (Test-ContainsProbe $apk 'Lorg/example/appcontroltelecomfixture/FixtureConnectionService;') { throw 'Telecom fixture service was packaged in an application APK.' }
    $fixtureClasses += 'Lcom/appcontrol/transport/ReleaseManifestProbe;'
    foreach ($descriptor in $fixtureClasses) {
        if (Test-ContainsProbe $apk $descriptor) { throw 'Acceptance fixture was packaged in the application APK.' }
        if (-not (Test-ContainsProbe $testApk $descriptor)) { throw 'Acceptance fixture is absent from its test APK.' }
    }
    $manifest = @(& $aapt dump xmltree $testApk AndroidManifest.xml) -join "`n"
    if ($LASTEXITCODE -ne 0 -or
        -not $manifest.Contains('com.appcontrol.transport.ServerTrustInstrumentation') -or
        -not $manifest.Contains('android:targetPackage') -or
        -not $manifest.Contains('com.appcontrol.' + $app + '"')) {
        throw 'Test APK instrumentation target or runner is invalid.'
    }
    Write-Output "$app APK: probe confined to test APK; instrumentation runner and target verified."
}
exit 0
