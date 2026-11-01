param(
    [ValidateSet('127.0.0.1:5581', 'emulator-5580')]
    [string]$Serial = '127.0.0.1:5581',
    [ValidateRange(60, 600)]
    [int]$HealthTimeoutSeconds = 60,
    [switch]$ObserveCallWatchWindow,
    [switch]$UseHostGateway,
    [switch]$ExerciseLocalNetworkDenial,
    [switch]$ExerciseParentPushApi,
    [switch]$VerifyAndroidBuildOnly
)

$ErrorActionPreference = 'Stop'
if($ExerciseLocalNetworkDenial -and -not $UseHostGateway){throw '-ExerciseLocalNetworkDenial requires -UseHostGateway.'}
if($VerifyAndroidBuildOnly -and ($ExerciseLocalNetworkDenial -or $ObserveCallWatchWindow -or $ExerciseParentPushApi)){throw 'Android build-only mode cannot claim runtime permission, push API or watch observations.'}
$workspace = Split-Path -Parent $PSScriptRoot
$adb = Join-Path $workspace '.tooling\android-sdk\platform-tools\adb.exe'
$gradle = 'C:\Gradle\gradle-9.6.0\bin\gradle.bat'
$go = 'C:\Program Files\Go\bin\go.exe'
$fixtureId = [guid]::NewGuid().ToString('N')
$cache = Join-Path $workspace '.cache'
$fixture = Join-Path $cache "android-server-https-$fixtureId"
$reports = Join-Path $cache 'android-server-https-reports'
$reportPath = Join-Path $reports "$fixtureId.json"
$report = [ordered]@{
    schemaVersion = 1
    task = 'local-android-to-server-https-acceptance'
    scope = $(if($VerifyAndroidBuildOnly){'android-build-only'}else{'live-android-https'})
    status = 'running'
    startedAtUtc = [datetime]::UtcNow.ToString('o')
    serial = $Serial
    callWatchWindowObservationRequested = [bool]$ObserveCallWatchWindow
    endpointMode = $(if($UseHostGateway){'emulator-host-gateway'}else{'adb-loopback'})
    localNetworkDenialRequested = [bool]$ExerciseLocalNetworkDenial
    parentPushApiRequested = [bool]$ExerciseParentPushApi
    stages = @()
    cleanup = 'pending'
    limitations = @(
        'No Firebase, production credentials, physical device, external service, deployment, or command execution is tested.',
        'The child acceptance probe uses production EnrollmentClient, AndroidKeyStore DeviceIdentity, and CommandSync over HTTPS; the Device Owner-only LOCK_DEVICE action is declined safely, and policy application is reported as device_owner_required. No physical device action is executed.',
        'The fetched LOCK_DEVICE command is acknowledged as failed without execution; policy is reported as device_owner_required and is not applied.',
        'The fixture exercises real HTTPS requests and selected production UI/watch flows in disposable apps; live production delivery and physical-device behavior remain unverified.'
    )
}
$reverseAdded = $false
$proxyStarted = $false
$serverStarted = $false
$failureStage = ''
if($VerifyAndroidBuildOnly){
    $report.limitations=@('Android build/package verification only. No APK installation, live API, UI runtime, permission denial, call-watch, Firebase, physical device, signing or deployment acceptance is exercised.')
}
$cleanupFailures = [Collections.Generic.List[string]]::new()

function Save-Report {
    $json = $script:report | ConvertTo-Json -Depth 8
    [IO.File]::WriteAllText($script:reportPath, $json + [Environment]::NewLine, [Text.UTF8Encoding]::new($false))
}

function Quote-PowerShell([string]$Value) {
    "'" + $Value.Replace("'", "''") + "'"
}

function Invoke-Logged([string]$Name, [string]$Category, [string]$Directory, [string]$Command) {
    $script:failureStage = $Name
    $started = [datetime]::UtcNow
    $timer = [Diagnostics.Stopwatch]::StartNew()
    $wrapper = Join-Path $script:PSScriptRoot 'run-logged.ps1'
    $backgroundStartup = $Name -in @('server-start', 'https-proxy-start')
    $exitCodePath = Join-Path $script:fixture "$Name-$($script:report.stages.Count).exit"
    $markerArgument = if ($backgroundStartup) { " -ExitCodeFile $(Quote-PowerShell $exitCodePath)" } else { '' }
    $inner = "& $(Quote-PowerShell $wrapper) -TaskId ANDROID-HTTPS-001 -Category $Category -WorkingDirectory $(Quote-PowerShell $Directory) -Command $(Quote-PowerShell $Command)$markerArgument"
    $encoded = [Convert]::ToBase64String([Text.Encoding]::Unicode.GetBytes($inner))
    $stageIndex = $script:report.stages.Count
    $stdoutPath = Join-Path $script:reports "$($script:fixtureId)-stage-$stageIndex.stdout.log"
    $job = $null
    $output = @()
    if ($backgroundStartup) {
        $stderrPath = Join-Path $script:reports "$($script:fixtureId)-stage-$stageIndex.stderr.log"
        $process = $null
        try {
            $process = Start-Process -FilePath 'powershell.exe' -ArgumentList @(
                '-NoProfile', '-ExecutionPolicy', 'Bypass', '-EncodedCommand', $encoded
            ) -WorkingDirectory $Directory -RedirectStandardOutput $stdoutPath -RedirectStandardError $stderrPath -WindowStyle Hidden -PassThru
            $waitTimer = [Diagnostics.Stopwatch]::StartNew()
            while (-not (Test-Path -LiteralPath $exitCodePath) -and -not $process.HasExited -and $waitTimer.Elapsed.TotalSeconds -lt 60) {
                Start-Sleep -Milliseconds 100
            }
            $waitTimer.Stop()
            if (Test-Path -LiteralPath $exitCodePath) {
                $parsedExitCode = 0
                $marker = (Get-Content -LiteralPath $exitCodePath -Raw).Trim()
                if (-not [int]::TryParse($marker, [ref]$parsedExitCode)) { throw 'Logged stage exit-code marker is invalid.' }
                $exitCode = $parsedExitCode
            } elseif ($process.HasExited) {
                $exitCode = 1
                $output = @('Logged stage exited without writing its exit-code marker.')
            } else {
                Stop-Process -Id $process.Id -Force
                try { Wait-Process -Id $process.Id -Timeout 5 -ErrorAction SilentlyContinue } catch {}
                $exitCode = 124
                $output = @('Background-service startup exceeded its 60-second timeout.')
            }
            if ($exitCode -ne 0) {
                if (Test-Path -LiteralPath $stdoutPath) { $output += Get-Content -LiteralPath $stdoutPath }
                if (Test-Path -LiteralPath $stderrPath) { $output += Get-Content -LiteralPath $stderrPath }
            }
        } catch {
            $exitCode = 1
            $output += $_.ToString()
        }
    } else {
        try {
            $job = Start-Job -ScriptBlock {
                param($encodedCommand)
                $ErrorActionPreference = 'Continue'
                $items = New-Object 'Collections.Generic.List[object]'
                & powershell.exe -NoProfile -ExecutionPolicy Bypass -OutputFormat Text -EncodedCommand $encodedCommand 2>&1 |
                    ForEach-Object { [void]$items.Add($_) }
                $code = 0
                if ($null -ne $LASTEXITCODE) { $code = [int]$LASTEXITCODE }
                [pscustomobject]@{ ExitCode = $code; Output = $items.ToArray() }
            } -ArgumentList $encoded
            $completedJob = Wait-Job -Job $job -Timeout 1800
            if ($null -eq $completedJob) {
                Stop-Job -Job $job
                try { Wait-Job -Job $job -Timeout 5 | Out-Null } catch {}
                $exitCode = 124
                $output = @("Logged stage exceeded its 30-minute timeout. Output remains in the run-logged logs.")
            } else {
                $jobOutput = @(Receive-Job -Job $job -ErrorAction Stop)
                if ($jobOutput.Count -ne 1) { throw 'Logged stage did not return exactly one process exit code.' }
                $exitCode = [int]$jobOutput[0].ExitCode
                $output = @($jobOutput[0].Output)
                $outputLines = @($output | ForEach-Object { [string]$_ })
                [IO.File]::WriteAllLines($stdoutPath, [string[]]$outputLines, [Text.UTF8Encoding]::new($false))
            }
        } catch {
            $exitCode = 1
            $output = @($_.ToString())
        } finally {
            if ($null -ne $job) { Remove-Job -Job $job -Force -ErrorAction SilentlyContinue }
        }
    }
    if ($exitCode -eq 124) {
        $output += 'The stage controller was stopped after its bounded wait.'
    }
    $timer.Stop()
    $script:report.stages += [ordered]@{
        name = $Name
        exitCode = $exitCode
        startedAtUtc = $started.ToString('o')
        finishedAtUtc = [datetime]::UtcNow.ToString('o')
        durationMilliseconds = $timer.ElapsedMilliseconds
    }
    Save-Report
    if ($exitCode -ne 0) {
        $output | ForEach-Object { Write-Output $_ }
        throw "Logged stage failed: $Name"
    }
    $script:failureStage = ''
    Write-Output "[$Name] passed; command and output recorded by run-logged.ps1."
}

function Guarded-DeviceCommand([string]$Body) {
    $adbPath = Quote-PowerShell $script:adb
    $serialValue = Quote-PowerShell $script:Serial
    @"
`$fingerprint=@(& $adbPath -P 5038 -s $serialValue shell getprop ro.build.fingerprint 2>&1)
if(`$LASTEXITCODE -ne 0 -or (`$fingerprint -join '').Trim() -notlike 'google/sdk_gphone64_x86_64/emu64xa:*'){throw 'Acceptance AVD fingerprint guard failed.'}
`$boot=@(& $adbPath -P 5038 -s $serialValue shell getprop sys.boot_completed 2>&1)
if(`$LASTEXITCODE -ne 0 -or (`$boot -join '').Trim() -ne '1'){throw 'Acceptance AVD boot guard failed.'}
$Body
"@
}

function Device-Stage([string]$Name, [string]$Body) {
    Invoke-Logged $Name 'device' $script:workspace (Guarded-DeviceCommand $Body)
}

function Invoke-HealthWait([string]$Uri, [int]$TimeoutSeconds, [string]$ReadyFile) {
    $command = @"
`$deadline=[DateTime]::UtcNow.AddSeconds($TimeoutSeconds)
`$ready=`$false
while([DateTime]::UtcNow -lt `$deadline){
    if(Test-Path -LiteralPath $(Quote-PowerShell $ReadyFile)){
        try{
            `$response=Invoke-WebRequest -UseBasicParsing -TimeoutSec 3 -Uri $(Quote-PowerShell $Uri)
            if(`$response.StatusCode -eq 200){`$ready=`$true;break}
        }catch{}
    }
    Start-Sleep -Milliseconds 500
}
if(-not `$ready){throw 'Disposable local server health check timed out.'}
Write-Output 'Disposable local HTTPS proxy and HTTP server are ready.'
"@
    Invoke-Logged 'server-health-wait' 'test' $script:workspace $command
}

function Get-ProcessPath([int]$ProcessId) {
    try {
        $path = (Get-Process -Id $ProcessId -ErrorAction Stop).Path
        if (-not [string]::IsNullOrWhiteSpace($path)) { return [IO.Path]::GetFullPath($path) }
    } catch {}
    return $null
}

function Stop-OwnedProcess([string]$Name, [string]$PidFile, [string]$Executable) {
    if (-not (Test-Path -LiteralPath $PidFile)) { return }
    $pidValue = 0
    if (-not [int]::TryParse((Get-Content -LiteralPath $PidFile -Raw).Trim(), [ref]$pidValue) -or $pidValue -le 0) {
        $script:cleanupFailures.Add("$Name PID record is invalid.")
        return
    }
    $process = Get-Process -Id $pidValue -ErrorAction SilentlyContinue
    if ($null -eq $process) { return }
    $actualPath = Get-ProcessPath $pidValue
    if ($null -eq $actualPath) {
        $script:cleanupFailures.Add("$Name PID $pidValue could not be verified; it was not stopped.")
        return
    }
    if (-not [string]::Equals($actualPath, [IO.Path]::GetFullPath($Executable), [StringComparison]::OrdinalIgnoreCase)) {
        $script:cleanupFailures.Add("$Name PID $pidValue no longer identifies the test-owned executable; it was not stopped.")
        return
    }
    $command = @"
`$targetPid=$pidValue
`$expected=$(Quote-PowerShell ([IO.Path]::GetFullPath($Executable)))
`$process=Get-Process -Id `$targetPid -ErrorAction SilentlyContinue
if(`$null -ne `$process){
    `$observed=`$process.Path
    if([string]::IsNullOrWhiteSpace(`$observed) -or -not [string]::Equals([IO.Path]::GetFullPath(`$observed), `$expected, [StringComparison]::OrdinalIgnoreCase)){throw 'Refusing to stop a process that is not the recorded test executable.'}
    `$process.Kill()
    if(-not `$process.WaitForExit(15000)){throw 'Test-owned process did not stop within the cleanup timeout.'}
}
if(Get-Process -Id `$targetPid -ErrorAction SilentlyContinue){throw 'Test-owned process remained after cleanup.'}
Write-Output '$Name stopped or had already exited.'
"@
    try {
        Invoke-Logged "cleanup-$Name" 'tooling' $script:workspace $command
    } catch {
        $script:cleanupFailures.Add("$Name process cleanup failed for PID $pidValue.")
    }
}

if (-not (Test-Path -LiteralPath $reports)) {
    New-Item -ItemType Directory -Path $reports | Out-Null
}
Save-Report

try {
    if (-not (Test-Path -LiteralPath $adb) -or -not (Test-Path -LiteralPath $gradle) -or -not (Test-Path -LiteralPath $go)) {
        throw 'A required local Android, Gradle, or Go toolchain is missing.'
    }
    if (Test-Path -LiteralPath $fixture) { throw 'The unique fixture path already exists; refusing to reuse it.' }
    if ($UseHostGateway) {
        Device-Stage 'gateway-ipv4-route-check' @"
`$route=@(& '$adb' -P 5038 -s '$Serial' shell ip -4 route get 10.0.2.2 2>&1)
if(`$LASTEXITCODE -ne 0 -or (`$route -join ' ') -notmatch '\bsrc\s+10\.0\.2\.\d+\b'){throw 'The isolated AVD has no expected IPv4 gateway route. Restore its AndroidWifi connection before retrying; no network setting was changed by this runner.'}
Write-Output 'Fixed emulator IPv4 gateway route is available.'
"@
    }

    $setup = @"
New-Item -ItemType Directory -Path $(Quote-PowerShell $script:fixture) | Out-Null
[IO.File]::WriteAllText($(Quote-PowerShell (Join-Path $script:fixture 'fixture-owned.marker')), 'local acceptance fixture')
Write-Output 'Created a unique disposable fixture directory.'
"@
    Invoke-Logged 'fixture-setup' 'tooling' $workspace $setup

    $portFile = Join-Path $fixture 'ports.txt'
    $reservePorts = @"
`$ports=@()
for(`$index=0;`$index -lt 2;`$index++){
    `$listener=[Net.Sockets.TcpListener]::new([Net.IPAddress]::Loopback,0)
    `$listener.Start()
    `$ports+=`$listener.LocalEndpoint.Port
    `$listener.Stop()
}
if(`$ports[0] -eq `$ports[1]){throw 'Unable to reserve distinct fixture ports.'}
[IO.File]::WriteAllText($(Quote-PowerShell $portFile), (`$ports -join [Environment]::NewLine))
Write-Output 'Selected two ephemeral loopback ports.'
"@
    Invoke-Logged 'fixture-port-selection' 'tooling' $workspace $reservePorts
    $ports = @(Get-Content -LiteralPath $portFile | ForEach-Object { [int]$_.Trim() })
    if ($ports.Count -ne 2 -or $ports[0] -notin 1..65535 -or $ports[1] -notin 1..65535) {
        throw 'Fixture port selection did not produce two valid ports.'
    }
    $httpPort = $ports[0]
    $tlsPort = $ports[1]

    $serverExe = Join-Path $fixture 'control-server.exe'
    $proxyExe = Join-Path $fixture 'acceptance-https-proxy.exe'
    $gocache = Join-Path $cache 'go-build'
    $gomodcache = Join-Path $cache 'go-mod'
    $goBuildServer = @"
`$env:GOPROXY='off'; `$env:GOTOOLCHAIN='local'; `$env:GOCACHE=$(Quote-PowerShell $gocache); `$env:GOMODCACHE=$(Quote-PowerShell $gomodcache)
& $(Quote-PowerShell $go) build -o $(Quote-PowerShell $serverExe) .\cmd\control-server
if(`$LASTEXITCODE -ne 0){throw 'Disposable server build failed.'}
"@

    $gofmt = Join-Path (Split-Path -Parent $go) 'gofmt.exe'
$goFormatBuild = @"
`$formatDiff=@(& $(Quote-PowerShell $gofmt) -d $(Quote-PowerShell (Join-Path $workspace 'scripts\acceptance-https-proxy.go')) 2>&1)
if(`$LASTEXITCODE -ne 0 -or `$formatDiff.Count -gt 0){throw 'Acceptance proxy source is not gofmt-clean.'}
`$env:GOPROXY='off'; `$env:GOTOOLCHAIN='local'; `$env:GOCACHE=$(Quote-PowerShell $gocache); `$env:GOMODCACHE=$(Quote-PowerShell $gomodcache)
& $(Quote-PowerShell $go) test -count=1 $(Quote-PowerShell (Join-Path $workspace 'scripts\acceptance-https-proxy.go')) $(Quote-PowerShell (Join-Path $workspace 'scripts\acceptance-https-proxy_test.go'))
if(`$LASTEXITCODE -ne 0){throw 'Acceptance proxy fault-isolation tests failed.'}
& $(Quote-PowerShell $go) build -o $(Quote-PowerShell $proxyExe) $(Quote-PowerShell (Join-Path $workspace 'scripts\acceptance-https-proxy.go'))
if(`$LASTEXITCODE -ne 0){throw 'Acceptance proxy build failed.'}
"@
    Invoke-Logged 'https-proxy-build' 'build' $workspace $goFormatBuild

    $caCommand = "& $(Quote-PowerShell $proxyExe) -mode generate -dir $(Quote-PowerShell $fixture)"
    Invoke-Logged 'fixture-tls-generation' 'tooling' $workspace $caCommand
    $pinText = (Get-Content -LiteralPath (Join-Path $fixture 'assets\acceptance-pin.txt') -Raw).Trim()
    if ($pinText -notmatch '^sha256/([A-Za-z0-9+/]{43}=)$') { throw 'Generated fixture SPKI pin is invalid.' }
    $currentPin = $Matches[1]
    do {
        $backupBytes = New-Object byte[] 32
        [Security.Cryptography.RandomNumberGenerator]::Create().GetBytes($backupBytes)
        $backupPin = [Convert]::ToBase64String($backupBytes)
    } while ($backupPin -eq $currentPin)

    $initScript = Join-Path $fixture 'acceptance.init.gradle'
    $initContents = @'
allprojects { acceptanceProject ->
    acceptanceProject.afterEvaluate {
        if (acceptanceProject.name == "parent-app" || acceptanceProject.name == "child-app") {
            def fixtureSources = System.getProperty("appControlAcceptanceJavaDir")
            if (fixtureSources == null || fixtureSources.isEmpty()) {
                throw new GradleException("Acceptance test assets were not configured")
            }
            def androidExtension = acceptanceProject.extensions.getByName("android")
            androidExtension.buildTypes.getByName("debug").applicationIdSuffix = ".acceptance"
            androidExtension.sourceSets.getByName("androidTest").java.srcDir(fixtureSources)
            def gatewayOrigin = System.getProperty("appControlAcceptanceGatewayOrigin")
            if (gatewayOrigin != null) {
                if (!(gatewayOrigin ==~ /https:\/\/10\.0\.2\.2:[0-9]{1,5}/)) throw new GradleException("Invalid fixed acceptance gateway")
                androidExtension.buildTypes.getByName("debug").buildConfigField("String", "SERVER_ORIGIN", '"' + gatewayOrigin + '"')
            }
        }
    }
}
'@
    [IO.File]::WriteAllText($initScript, $initContents, [Text.UTF8Encoding]::new($false))

    $gradleUserHome = Join-Path $cache 'gradle'
    $gradleStatus = @"
`$env:JAVA_HOME='C:\Program Files\Java\jdk-17'; `$env:GRADLE_USER_HOME=$(Quote-PowerShell $gradleUserHome)
`$status=@(& $(Quote-PowerShell $gradle) --status 2>&1)
`$status | Write-Output
if(`$LASTEXITCODE -ne 0){throw 'Could not check Gradle daemon activity.'}
if((`$status -join "``n") -match '(?im)^\s*\d+\s+BUSY\b'){throw 'A Gradle daemon is busy; refusing to overlap another build.'}
"@
    Invoke-Logged 'gradle-concurrency-check' 'tooling' $workspace $gradleStatus

    $versionFile = Join-Path $fixture 'installed-versions.txt'
    $versionCommand = @"
`$codes=@()
foreach(`$packageName in @('com.appcontrol.parent','com.appcontrol.child')){
    `$details=@(& $(Quote-PowerShell $adb) -P 5038 -s $(Quote-PowerShell $Serial) shell dumpsys package `$packageName 2>&1)
    if(`$LASTEXITCODE -ne 0){throw 'Could not read installed application version metadata.'}
    `$match=`$details | Select-String -Pattern 'versionCode=(\d+)' | Select-Object -First 1
    `$code=if(`$null -eq `$match){0}else{[int]`$match.Matches[0].Groups[1].Value}
    `$codes+=`$code
}
[IO.File]::WriteAllText($(Quote-PowerShell $versionFile), (`$codes -join [Environment]::NewLine))
Write-Output "Installed package version metadata read for the guarded AVD."
"@
    Device-Stage 'installed-version-read' $versionCommand
    $installedCodes = @(Get-Content -LiteralPath $versionFile | ForEach-Object { [int]$_.Trim() })
    if ($installedCodes.Count -ne 2) { throw 'Unable to determine target app versions safely.' }
    $versionCode = [Math]::Max(1, ($installedCodes | Measure-Object -Maximum).Maximum + 1)
    if ($versionCode -gt 2100000000) { throw 'Installed app version code is already at the supported maximum.' }
    $versionName = "acceptance-$fixtureId"
    $origin = if($UseHostGateway){"https://10.0.2.2:$tlsPort"}else{"https://localhost:$tlsPort"}
    $configuredOrigin = "https://localhost:$tlsPort"
    $androidDirectory = Join-Path $workspace 'android'
    $acceptanceGradleArgs = @(
        '--offline'
        '--no-daemon'
        '--no-build-cache'
        '-Pkotlin.incremental=false'
        "-Duser.home=$env:USERPROFILE"
        "-DappControlAcceptanceJavaDir=$(Join-Path $fixture 'generated')"
        '--init-script'
        $initScript
        "-PappControlServerOrigin=$configuredOrigin"
        "-PappControlCurrentPin=$currentPin"
        "-PappControlBackupPin=$backupPin"
        "-PappControlVersionCode=$versionCode"
        "-PappControlVersionName=$versionName"
        '-PappControlRemoteCallsEnabled=false'
        ':parent-app:clean'
        ':child-app:clean'
        ':parent-app:assembleDebug'
        ':parent-app:assembleDebugAndroidTest'
        ':child-app:assembleDebug'
        ':child-app:assembleDebugAndroidTest'
    )
    if($UseHostGateway){$acceptanceGradleArgs+=@("-DappControlAcceptanceGatewayOrigin=$origin")}
    $acceptanceGradleLine = "& $(Quote-PowerShell $gradle) " + (($acceptanceGradleArgs | ForEach-Object { Quote-PowerShell $_ }) -join ' ')
    $acceptanceGradle = @"
`$env:JAVA_HOME='C:\Program Files\Java\jdk-17'; `$env:GRADLE_USER_HOME=$(Quote-PowerShell $gradleUserHome)
$acceptanceGradleLine
if(`$LASTEXITCODE -ne 0){throw 'Acceptance debug and Android test APK build failed.'}
"@
    Invoke-Logged 'acceptance-apk-build' 'build' $androidDirectory $acceptanceGradle

    $apkFiles = @(
        @{ source = (Join-Path $androidDirectory 'parent-app\build\outputs\apk\debug\parent-app-debug.apk'); target = (Join-Path $fixture 'parent-acceptance.apk') },
        @{ source = (Join-Path $androidDirectory 'parent-app\build\outputs\apk\androidTest\debug\parent-app-debug-androidTest.apk'); target = (Join-Path $fixture 'parent-acceptance-test.apk') },
        @{ source = (Join-Path $androidDirectory 'child-app\build\outputs\apk\debug\child-app-debug.apk'); target = (Join-Path $fixture 'child-acceptance.apk') },
        @{ source = (Join-Path $androidDirectory 'child-app\build\outputs\apk\androidTest\debug\child-app-debug-androidTest.apk'); target = (Join-Path $fixture 'child-acceptance-test.apk') }
    )
    $copyAcceptance = foreach ($item in $apkFiles) {
        "if(-not (Test-Path -LiteralPath $(Quote-PowerShell $item.source))){throw 'Expected acceptance APK is missing.'}; Copy-Item -LiteralPath $(Quote-PowerShell $item.source) -Destination $(Quote-PowerShell $item.target)"
    }
    Invoke-Logged 'acceptance-apk-staging' 'release' $workspace ($copyAcceptance -join "`n")
    Invoke-Logged 'acceptance-packaged-origin-check' 'verification' $workspace (
        '& '+(Quote-PowerShell (Join-Path $PSScriptRoot 'assert-apk-parent-origin.ps1'))+
        ' -Apk '+(Quote-PowerShell (Join-Path $fixture 'parent-acceptance.apk'))+
        ' -ExpectedOrigin '+(Quote-PowerShell $origin))

    $aapt = Join-Path $workspace '.tooling\android-sdk\build-tools\36.0.0\aapt.exe'
    $packageCheck = @"
`$expected=@(
    @($(Quote-PowerShell (Join-Path $fixture 'parent-acceptance.apk')), 'com.appcontrol.parent.acceptance'),
    @($(Quote-PowerShell (Join-Path $fixture 'parent-acceptance-test.apk')), 'com.appcontrol.parent.acceptance.test'),
    @($(Quote-PowerShell (Join-Path $fixture 'child-acceptance.apk')), 'com.appcontrol.child.acceptance'),
    @($(Quote-PowerShell (Join-Path $fixture 'child-acceptance-test.apk')), 'com.appcontrol.child.acceptance.test')
)
foreach(`$entry in `$expected){
    `$badging=@(& $(Quote-PowerShell $aapt) dump badging `$entry[0] 2>&1)
    if(`$LASTEXITCODE -ne 0){throw 'Could not inspect acceptance APK application IDs.'}
    `$line=`$badging | Where-Object { `$_ -match "^package: name='([^']+)'" } | Select-Object -First 1
    if(`$null -eq `$line){throw 'Acceptance APK package metadata is missing.'}
    `$actual=[regex]::Match(`$line,"^package: name='([^']+)'").Groups[1].Value
    if(`$actual -ne `$entry[1]){throw 'Acceptance APK package ID does not match its side-by-side test identity.'}
}
Write-Output 'Verified unique side-by-side package IDs for the parent, child, and test APKs.'
"@
    Invoke-Logged 'acceptance-package-id-check' 'verification' $workspace $packageCheck

    if($VerifyAndroidBuildOnly){
        $report.status='passed'
        $report['proof']=@('Clean parent/child acceptance APK builds and side-by-side package checks passed. Packaged ParentClient origin matched the fixture. No APK was installed, server started, or runtime behavior tested.')
    } else {
    Invoke-Logged 'server-build' 'build' (Join-Path $workspace 'server') $goBuildServer
    $serverPidFile = Join-Path $fixture 'control-server.pid'
    $serverOutput = Join-Path $fixture 'control-server.stdout.log'
    $serverError = Join-Path $fixture 'control-server.stderr.log'
    $database = Join-Path $fixture 'acceptance.sqlite'
    $serverStart = @"
Get-ChildItem Env: | Where-Object { `$_.Name -like 'APP_CONTROL_*' } | ForEach-Object { Remove-Item -LiteralPath ('Env:\'+`$_.Name) }
`$env:APP_CONTROL_HTTP_ADDRESS='127.0.0.1:$httpPort'
`$env:APP_CONTROL_DATABASE_PATH=$(Quote-PowerShell $database)
`$env:APP_CONTROL_REMOTE_CALLS_ENABLED='false'
`$env:APP_CONTROL_RELEASE_MANIFEST_FILE=''; `$env:APP_CONTROL_RELEASE_PUBLIC_KEY_FILE=''; `$env:APP_CONTROL_RELEASE_ORIGIN=''; `$env:APP_CONTROL_RELEASE_CERTIFICATE_SHA256=''
`$env:APP_CONTROL_FCM_SERVICE_ACCOUNT_FILE=''; `$env:APP_CONTROL_FCM_TOKEN_KEY_FILE=''
`$env:APP_CONTROL_PARENT_FCM_TOKEN_KEY_FILE=''
$(if($ExerciseParentPushApi){'$env:APP_CONTROL_PARENT_FCM_TOKEN_KEY_FILE='+ (Quote-PowerShell (Join-Path $fixture 'parent-routing-key.bin'))}else{''})
`$process=Start-Process -FilePath $(Quote-PowerShell $serverExe) -WorkingDirectory $(Quote-PowerShell (Join-Path $workspace 'server')) -RedirectStandardOutput $(Quote-PowerShell $serverOutput) -RedirectStandardError $(Quote-PowerShell $serverError) -WindowStyle Hidden -PassThru
[IO.File]::WriteAllText($(Quote-PowerShell $serverPidFile), `$process.Id.ToString())
Write-Output 'Disposable loopback-only HTTP server started.'
"@
    $serverStarted = $true
    Invoke-Logged 'server-start' 'tooling' $workspace $serverStart

    $readyFile = Join-Path $fixture 'proxy-ready.marker'
    $proxyPidFile = Join-Path $fixture 'https-proxy.pid'
    $proxyOutput = Join-Path $fixture 'https-proxy.stdout.log'
    $proxyError = Join-Path $fixture 'https-proxy.stderr.log'
    $proxyArgs = "-mode serve -listen 127.0.0.1:$tlsPort -upstream 127.0.0.1:$httpPort -dir `"$fixture`" -ready-file `"$readyFile`""
    $proxyStart = @"
`$process=Start-Process -FilePath $(Quote-PowerShell $proxyExe) -ArgumentList $(Quote-PowerShell $proxyArgs) -WorkingDirectory $(Quote-PowerShell $workspace) -RedirectStandardOutput $(Quote-PowerShell $proxyOutput) -RedirectStandardError $(Quote-PowerShell $proxyError) -WindowStyle Hidden -PassThru
[IO.File]::WriteAllText($(Quote-PowerShell $proxyPidFile), `$process.Id.ToString())
Write-Output 'Disposable loopback-only HTTPS proxy started.'
"@
    $proxyStarted = $true
    Invoke-Logged 'https-proxy-start' 'tooling' $workspace $proxyStart
    Invoke-HealthWait "http://127.0.0.1:$httpPort/healthz" $HealthTimeoutSeconds $readyFile

    $reverse = "& $(Quote-PowerShell $adb) -P 5038 -s $(Quote-PowerShell $Serial) reverse tcp:$tlsPort tcp:$tlsPort; if(`$LASTEXITCODE -ne 0){throw 'ADB reverse setup failed.'}; Write-Output 'Temporary TLS loopback forwarding added to the guarded AVD.'"
    if(-not $UseHostGateway){
        Device-Stage 'adb-reverse-add' $reverse
        $reverseAdded = $true
    }

    $install = @"
`$installations=@(
    @($(Quote-PowerShell (Join-Path $fixture 'parent-acceptance.apk')), 'parent application'),
    @($(Quote-PowerShell (Join-Path $fixture 'parent-acceptance-test.apk')), 'parent test APK'),
    @($(Quote-PowerShell (Join-Path $fixture 'child-acceptance.apk')), 'child application'),
    @($(Quote-PowerShell (Join-Path $fixture 'child-acceptance-test.apk')), 'child test APK')
)
foreach(`$entry in `$installations){
    `$result=@(& $(Quote-PowerShell $adb) -P 5038 -s $(Quote-PowerShell $Serial) install -r `$entry[0] 2>&1)
    `$result | Write-Output
    if(`$LASTEXITCODE -ne 0){throw ('Safe in-place install failed for '+`$entry[1]+'. No uninstall or data clear was attempted.')}
}
Write-Output 'Side-by-side acceptance APKs installed with -r; existing applications were untouched.'
"@
    Device-Stage 'acceptance-apk-install' $install

    $integration = "& $(Quote-PowerShell (Join-Path $PSScriptRoot 'test-integration.ps1')) -Emulator -Serial $(Quote-PowerShell $Serial) -RequireLiveServerHttps"
    if($ObserveCallWatchWindow){$integration+=' -ObserveCallWatchWindow'}
    if($UseHostGateway){$integration+=' -RequireLocalNetwork'}
    if($ExerciseLocalNetworkDenial){$integration+=' -ExerciseLocalNetworkDenial'}
    if($ExerciseParentPushApi){$integration+=' -RequireParentPushApi'}
    Invoke-Logged 'new-acceptance-and-existing-integration' 'test' $workspace $integration

    $report.status = 'passed'
    $report['proof'] = @(
        'Production child DialActivity loaded a locally seeded encrypted fixture cache, filled the reviewable dial pad from Call Parent without pressing Call, rejected stale and removed shortcut selections while preserving manual input, and restored its prior cache/enrollment/key state. This is local cache-to-UI acceptance, separate from live contact delivery; no outgoing call was placed.',
        'Production parent call watch started through its foreground screen and notification permission flow, displayed a private ringing alert for a synthetic server report, opened fresh exact-call review with remote actions disabled, removed the ended-call alert, refused the stale exact-call review, and removed service/alerts on explicit stop and logout. Notification permission is retained only for the disposable acceptance app. No actual call or Firebase delivery is inferred; elapsed expiry is recorded only for explicit observation mode.',
        'Production parent screens edited an explicit-zone weekly schedule, saved/restored internet-pause intent with pending confirmation, saved a primary contact, rejected a concurrent stale contact edit, and displayed a queued lock command/history against a server-only synthetic device. No child received or executed that command or policy.',
        'Production ParentActivity screens rejected an incorrect password, signed in over fixture HTTPS, reopened the saved session, rendered a revoked device without controls, displayed a fresh pairing code, signed out, and reopened at sign-in. The side-by-side acceptance app session was cleaned; no physical phone or policy execution was used.',
        'A fixture-only HTTPS 503 on the child contact route preserved the prior encrypted contact bytes and readable version while production CommandSync still acknowledged a safely declined command and fetched policy. Removing the fault let the next sync converge to the replacement contact. The fault is reset in cleanup; no network switching or full-server outage is inferred.',
        'Production CommandSync delivered parent-created contact version 1, replaced it with version 2, and cleared it at version 3 over fixture HTTPS; newly loaded production CallContactStore instances verified every snapshot. The isolated contact preferences were deleted and existing cache/key state was preserved. No call was placed.',
        'ParentClient completed real local HTTPS account login, access-token refresh, owner device listing, cross-account denial, and logout.',
        'Child test process completed production EnrollmentClient enrollment, rejected reuse of the consumed pairing code, authenticated its AndroidKeyStore key over real local HTTPS, and verified parent-triggered revocation rejects the existing device token on subsequent requests; the production CommandSync fetched the command, safely declined Device Owner-only execution, acknowledged failure, and cached/reported policy state.',
        'Parent and child debug/test APKs were built with the same ephemeral local CA and leaf SPKI pin under side-by-side .acceptance package IDs; the pre-existing applications were not replaced or removed.'
    )
    if($ObserveCallWatchWindow){$report['proof']+=@('The production watch remained active until its actual ten-minute monotonic deadline, removed its service notification automatically, retained a valid authenticated session and active synthetic device, then restarted normally and cleaned up on logout. Only the test activity keeps its own screen awake; no global device setting changes or OEM background behavior are inferred.')}
    if($UseHostGateway){$report['proof']+=@('All live exchanges used the fixed private emulator host gateway 10.0.2.2 with IP-SAN hostname validation, ephemeral CA/SPKI trust and production LAN-consent setup in both acceptance apps. No ADB reverse mapping was created; LAN permission is retained only in those disposable apps.')}
    if($ExerciseLocalNetworkDenial){$report['proof']+=@('After a fixture-only parent permission reset, the production setup flow respected an explicit LAN denial, showed its denial state without creating a session, and granted access only after a second explicit button request and system consent. All subsequent private-gateway HTTPS exchanges passed.')}
    }
} catch {
    $report.status = 'failed'
    $report['failureStage'] = $failureStage
    if ([string]::IsNullOrWhiteSpace($failureStage)) { $report['failureStage'] = 'orchestration' }
    Write-Output "Acceptance failed at $($report.failureStage)."
} finally {
    if ($reverseAdded) {
        try {
            $removeReverse = "& $(Quote-PowerShell $adb) -P 5038 -s $(Quote-PowerShell $Serial) reverse --remove tcp:$tlsPort; if(`$LASTEXITCODE -ne 0){throw 'Could not remove test-only ADB reverse mapping.'}; Write-Output 'Temporary TLS forwarding removed.'"
            Device-Stage 'cleanup-adb-reverse' $removeReverse
            $reverseAdded = $false
        } catch {
            $cleanupFailures.Add("Could not remove reverse mapping on the guarded serial; remove it with: adb -P 5038 -s $Serial reverse --remove tcp:$tlsPort")
        }
    }
    if ($proxyStarted) { Stop-OwnedProcess 'https-proxy' (Join-Path $fixture 'https-proxy.pid') (Join-Path $fixture 'acceptance-https-proxy.exe') }
    if ($serverStarted) { Stop-OwnedProcess 'control-server' (Join-Path $fixture 'control-server.pid') (Join-Path $fixture 'control-server.exe') }

    $processesStopped = $cleanupFailures.Count -eq 0
    if ($processesStopped -and (Test-Path -LiteralPath (Join-Path $fixture 'fixture-owned.marker'))) {
        try {
            $deleteCommand = @"
`$target=$(Quote-PowerShell ([IO.Path]::GetFullPath($fixture)))
`$expected=$(Quote-PowerShell ([IO.Path]::GetFullPath((Join-Path $cache "android-server-https-$fixtureId"))))
if(-not [string]::Equals(`$target,`$expected,[StringComparison]::OrdinalIgnoreCase) -or -not (Test-Path -LiteralPath (Join-Path `$target 'fixture-owned.marker'))){throw 'Refusing to remove an unverified fixture directory.'}
Remove-Item -LiteralPath `$target -Recurse -Force
Write-Output 'Removed the test-owned disposable fixture directory.'
"@
            Invoke-Logged 'fixture-cleanup' 'tooling' $workspace $deleteCommand
            $report.cleanup = 'passed'
        } catch {
            $cleanupFailures.Add('Fixture directory cleanup failed; retain and inspect only the unique acceptance directory.')
            $report.cleanup = 'failed'
        }
    } elseif (Test-Path -LiteralPath $fixture) {
        $report.cleanup = 'incomplete'
        if ($cleanupFailures.Count -eq 0) { $cleanupFailures.Add('Fixture directory retained because a test-owned process may still be running.') }
    } else {
        $report.cleanup = 'not-needed'
    }
    if ($cleanupFailures.Count -gt 0) {
        $report['cleanupFailures'] = @($cleanupFailures)
        $report['recovery'] = @(
            "Keep the isolated ADB daemon on port 5038 and serial $Serial; verify the AVD fingerprint before removing any reverse mapping.",
            "Only stop a server process after confirming its image path is inside the recorded unique fixture directory.",
            "The private fixture key and database remain only under $fixture; remove that exact directory only after both recorded processes have exited."
        )
        $report.status = 'failed'
    }
    $report.finishedAtUtc = [datetime]::UtcNow.ToString('o')
    Save-Report
    Write-Output "Acceptance report: $reportPath"
}

if ($report.status -ne 'passed') { exit 1 }
