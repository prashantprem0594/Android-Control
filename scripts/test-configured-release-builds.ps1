# Run through run-logged.ps1. Public synthetic configuration, offline builds only.
$ErrorActionPreference = 'Stop'
$workspace = Split-Path -Parent $PSScriptRoot
$root = Join-Path $workspace ('.cache\configured-releases-' + [guid]::NewGuid().ToString('N'))
$null = New-Item -ItemType Directory -Path $root
$key = [byte[]]@(0x30,0x2a,0x30,0x05,0x06,0x03,0x2b,0x65,0x70,0x03,0x21,0x00) + [byte[]](1..32)
$configuration = [ordered]@{
    serverOrigin='https://configured-release-fixture.invalid'
    currentPin=[Convert]::ToBase64String([byte[]](1..32));backupPin=[Convert]::ToBase64String([byte[]](33..64))
    apkCertificateSHA256=('a'*64);updatePublicKey=[Convert]::ToBase64String($key)
    updateMinimumSequence=903;versionCode=4;versionName='0.1.3-config-fixture';remoteCallsEnabled=$false;firebase=$null
}
$aapt = Join-Path $workspace '.tooling\android-sdk\build-tools\36.0.0\aapt.exe'
$savedJava = $env:JAVA_HOME
$savedGradle = $env:GRADLE_USER_HOME
$records = @()
Add-Type -AssemblyName System.IO.Compression.FileSystem
function Assert-ApkSeparation([string]$Path,[bool]$Firebase) {
    $zip = [IO.Compression.ZipFile]::OpenRead($Path)
    $foundFirebase = $false
    try {
        foreach ($entry in $zip.Entries) {
            if ($entry.FullName -notmatch '^classes[0-9]*\.dex$') { continue }
            $stream = $entry.Open(); $buffer = [IO.MemoryStream]::new()
            try {
                $stream.CopyTo($buffer)
                $dex = [Text.Encoding]::ASCII.GetString($buffer.ToArray())
                if ($dex.Contains('Lcom/google/firebase/messaging/FirebaseMessaging;')) { $foundFirebase=$true }
                foreach ($probe in @('Lcom/appcontrol/transport/ServerTrustInstrumentation;','Lcom/appcontrol/child/CallContactCacheProbe;','Lcom/appcontrol/transport/ReleaseArtifactProbe;')) {
                    if ($dex.Contains($probe)) { throw 'Test-only code leaked into configured release APK.' }
                }
            } finally { $stream.Dispose();$buffer.Dispose() }
        }
        if ($foundFirebase -ne $Firebase) { throw 'Firebase SDK appeared in the wrong release APK variant.' }
    } finally { $zip.Dispose() }
}
try {
    $env:JAVA_HOME = 'C:\Program Files\Java\jdk-17'
    $env:GRADLE_USER_HOME = Join-Path $workspace '.cache\gradle'
    # Exercise the opt-in branch first; finish with remote actions disabled.
    foreach ($mode in @('remote-calls','polling','firebase')) {
        $configuration.remoteCallsEnabled = $mode -eq 'remote-calls'
        $configuration.firebase = $null
        if ($mode -eq 'firebase') {
            $configuration.firebase = [ordered]@{applicationId='1:12345:android:abcdef';apiKey=('x'*24);projectId='configured-release-fixture';senderId='12345'}
        }
        $inputPath = Join-Path $root ($mode+'-public.json')
        [IO.File]::WriteAllText($inputPath,($configuration | ConvertTo-Json -Depth 5),[Text.UTF8Encoding]::new($false))
        $export = Join-Path $root ($mode+'-configuration')
        & (Join-Path $PSScriptRoot 'export-production-configuration.ps1') -ConfigurationFile $inputPath -OutputDirectory $export -AllowFixture
        $inventory = Get-Content -LiteralPath (Join-Path $export 'configuration.json') -Raw | ConvertFrom-Json
        if ($inventory.mode -cne 'fixture' -or $inventory.productionReady) { throw 'Fixture evidence incorrectly claims production readiness.' }
        foreach ($file in $inventory.files) {
            if ((Get-FileHash -LiteralPath (Join-Path $export $file.name) -Algorithm SHA256).Hash.ToLowerInvariant() -cne $file.sha256) { throw 'Exported configuration changed.' }
        }
        $build = Get-Content -LiteralPath (Join-Path $export 'release-arguments.json') -Raw | ConvertFrom-Json
        $variant = if ($mode -eq 'firebase') { 'fcmRelease' } else { 'release' }
        $lint = if ($mode -eq 'firebase') { ':child-app:lintFcmRelease' } else { ':child-app:lintRelease' }
        $gradleArguments = @('--offline','--no-daemon','-Pkotlin.compiler.execution.strategy=in-process') + @($build.arguments) + @($build.tasks) + @(':parent-app:lintRelease',$lint)
        Push-Location -LiteralPath (Join-Path $workspace 'android')
        try {
            # Gradle writes ordinary compiler warnings to native stderr.
            $ErrorActionPreference = 'Continue'
            & 'C:\Gradle\gradle-9.6.0\bin\gradle.bat' @gradleArguments
            $buildExit = $LASTEXITCODE
            $ErrorActionPreference = 'Stop'
            if ($buildExit -ne 0) { throw 'Configured offline release assembly/full lint failed.' }
        } finally { $ErrorActionPreference='Stop'; Pop-Location }
        $snapshot = Join-Path $root $mode
        $null = New-Item -ItemType Directory -Path $snapshot
        foreach ($app in @('parent','child')) {
            $selected = if ($app -eq 'child') { $variant } else { 'release' }
            $base = Join-Path $workspace ('android\'+$app+'-app\build')
            $configFile = Join-Path $base ('generated\source\buildConfig\'+$selected+'\com\appcontrol\'+$app+'\BuildConfig.java')
            $compiled = [IO.File]::ReadAllText($configFile)
            foreach ($expected in @(('SERVER_ORIGIN = "'+$configuration.serverOrigin+'"'),('UPDATE_PUBLIC_KEY = "'+$configuration.updatePublicKey+'"'),('UPDATE_CERTIFICATE_SHA256 = "'+$configuration.apkCertificateSHA256+'"'),'UPDATE_MIN_SEQUENCE = 903','VERSION_CODE = 4')) {
                if (-not $compiled.Contains($expected)) { throw 'Compiled release authority/version differs from exported inputs.' }
            }
            $expectedRemote = 'REMOTE_CALLS_ENABLED = '+$configuration.remoteCallsEnabled.ToString().ToLowerInvariant()
            if ($app -eq 'child' -and -not $compiled.Contains($expectedRemote)) { throw 'Compiled remote-call rollout differs from exported inputs.' }
            if ($app -eq 'child' -and $mode -eq 'firebase') {
                $firebaseFields = @{APPLICATION_ID='applicationId';API_KEY='apiKey';PROJECT_ID='projectId';SENDER_ID='senderId'}
                foreach ($field in $firebaseFields.Keys) {
                    if (-not $compiled.Contains('FIREBASE_'+$field+' = "'+$configuration.firebase.($firebaseFields[$field])+'"')) { throw 'Compiled Firebase settings differ from exported inputs.' }
                }
            }
            $source = Join-Path $base ('outputs\apk\'+$selected+'\'+$app+'-app-'+$selected+'-unsigned.apk')
            $destination = Join-Path $snapshot ($app+'-unsigned.apk')
            [IO.File]::Copy($source,$destination,$false)
            [IO.File]::Copy($configFile,(Join-Path $snapshot ($app+'-BuildConfig.java')),$false)
            Assert-ApkSeparation $destination ($app -eq 'child' -and $mode -eq 'firebase')
            $manifest = @(& $aapt dump xmltree $destination AndroidManifest.xml) -join "`n"
            if ($LASTEXITCODE -ne 0) { throw 'Configured APK manifest inspection failed.' }
            $hasFirebase = $manifest.Contains('com.appcontrol.child.wakeup.FirebaseWakeupService')
            if ($hasFirebase -ne ($app -eq 'child' -and $mode -eq 'firebase')) { throw 'Firebase service appeared in the wrong APK variant.' }
            if ($app -eq 'child' -and $mode -eq 'firebase' -and -not $manifest.Contains('com.appcontrol.child.wakeup.FirebaseApplication')) { throw 'Firebase application missing.' }
            $report = Join-Path $base ('reports\lint-results-'+$selected+'.xml')
            [xml]$lintReport = Get-Content -LiteralPath $report -Raw
            if (@($lintReport.issues.issue | Where-Object { $_.severity -in @('Error','Fatal') }).Count) { throw 'Configured release lint has errors.' }
            [IO.File]::Copy($report,(Join-Path $snapshot ($app+'-lint.xml')),$false)
            $records += [ordered]@{mode=$mode;app=$app;variant=$selected;remoteCallsEnabled=$configuration.remoteCallsEnabled;sha256=(Get-FileHash -LiteralPath $destination -Algorithm SHA256).Hash.ToLowerInvariant();buildConfigSHA256=(Get-FileHash -LiteralPath $configFile -Algorithm SHA256).Hash.ToLowerInvariant();lintSHA256=(Get-FileHash -LiteralPath $report -Algorithm SHA256).Hash.ToLowerInvariant();fullLintPassed=$true;compiledAuthorityMatched=$true;apkSeparationVerified=$true}
        }
    }
    $summary = [ordered]@{schemaVersion=1;createdAt=[datetime]::UtcNow.ToString('o');fixtureOnly=$true;productionReady=$false;updateAuthorityConfigured=$true;liveFirebaseVerified=$false;artifacts=$records}
    [IO.File]::WriteAllText((Join-Path $root 'configured-builds.json'),($summary | ConvertTo-Json -Depth 6),[Text.UTF8Encoding]::new($false))
    Write-Output 'Exported remote-opt-in/polling/Firebase argument arrays assembled and passed full lint with matching compiled authority/rollout; final variants disable remote calls. No signing, install, live messaging or deployment.'
    Write-Output "Evidence directory: $root"
} finally { $env:JAVA_HOME=$savedJava; $env:GRADLE_USER_HOME=$savedGradle }
