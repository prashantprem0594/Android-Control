param(
    [ValidateSet('127.0.0.1:5581','emulator-5580')] [string]$Serial='127.0.0.1:5581',
    [ValidateSet(5038)] [int]$AdbServerPort=5038,
    [string]$DebugUserHome=''
)
# Packet-level VPN proof. Physical serials cannot pass validation.
$ErrorActionPreference='Stop'
$workspace=Split-Path -Parent $PSScriptRoot
if([string]::IsNullOrWhiteSpace($DebugUserHome)){$DebugUserHome=Join-Path $workspace '.tooling\network-signing-home'}
$adb=Join-Path $workspace '.tooling\android-sdk\platform-tools\adb.exe'
$gradle='C:\Gradle\gradle-9.6.0\bin\gradle.bat'
$java='C:\Program Files\Java\jdk-17'
$gradleHome=Join-Path $workspace '.cache\gradle'
$go='C:\Program Files\Go\bin\go.exe'
$id=[guid]::NewGuid().ToString('N')
$fixture=Join-Path $workspace ('.cache\network-pause-fixture-'+$id)
$fixtureExe=Join-Path $fixture 'network-fixture.exe'
$ready=Join-Path $fixture 'ready.txt'
$pidFile=Join-Path $fixture 'fixture.pid'
$apk=Join-Path $workspace 'android\child-app\build\outputs\apk\debug\child-app-debug.apk'
$testApk=Join-Path $workspace 'android\child-app\build\outputs\apk\androidTest\debug\child-app-debug-androidTest.apk'
$fixtureApk=Join-Path $workspace 'android\network-fixture\build\outputs\apk\debug\network-fixture-debug.apk'
$processStarted=$false
$installed=$false
$overall='failed'
$probeVersion='ipv4-host-gateway-v4'
$probeScope='ipv4-tcp-dns-wire-fixture-and-management-uid'
$reportPath=Join-Path $workspace ('.cache\network-pause-result-'+$id+'.json')
$ipv6ProofPath=Join-Path $fixture 'ipv6-proof.json'
$runtimeProof=$false
$cleanupProof=$false
$report=[ordered]@{schemaVersion=1;startedAtUtc=[datetime]::UtcNow.ToString('o');status='running';version=$probeVersion;scope=$probeScope;runtimeProofPassed=$false;androidCleanupPassed=$false;hostFixtureCleanupPassed=$false;cleanupPassed=$false;limitations=@('IPv4 emulator TCP and explicit DNS-wire UDP fixtures only.','No operating-system resolver, private DNS, real Internet, IPv6, FCM or physical-device acceptance.')}
$report.ipv6=[ordered]@{status='not_run';baselineTarget='not_run';baselineManagement='not_run';scope='not-tested'}
$report.limitations=@('IPv4 TCP and explicit DNS-wire UDP fixture proof is separate from conditional native IPv6 TCP fixture proof.','IPv6 is unproven unless both UID baselines are reachable and its own block/resume/management markers pass.','No operating-system resolver, private DNS, real Internet, FCM or physical-device acceptance.')
function Save-NetworkReport{
    [IO.File]::WriteAllText($reportPath,($report | ConvertTo-Json -Depth 5),[Text.UTF8Encoding]::new($false))
}
Save-NetworkReport

function Invoke-Logged([string]$TaskId,[string]$Category,[string]$Directory,[string]$Body){
    & (Join-Path $PSScriptRoot 'run-logged.ps1') -TaskId $TaskId -Category $Category -WorkingDirectory $Directory -Command $Body
    if($LASTEXITCODE -ne 0){throw "Logged stage failed: $TaskId"}
}
function Invoke-DeviceProbe([string]$Phase,[string]$Expected,[switch]$FullProof){
    $guard="`$f=@(& '$adb' -P $AdbServerPort -s '$Serial' shell getprop ro.build.fingerprint 2>&1); if(`$LASTEXITCODE -ne 0 -or (`$f -join '').Trim() -notlike 'google/sdk_gphone64_x86_64/emu64xa:*'){throw 'Acceptance AVD fingerprint guard failed.'}; `$b=@(& '$adb' -P $AdbServerPort -s '$Serial' shell getprop sys.boot_completed 2>&1); if(`$LASTEXITCODE -ne 0 -or (`$b -join '').Trim() -ne '1'){throw 'Acceptance AVD boot guard failed.'}"
    $instrument=@"
`$o=@(& '$adb' -P $AdbServerPort -s '$Serial' shell am instrument -w -r -e probe network_pause -e phase '$Phase' com.appcontrol.child.test/com.appcontrol.transport.ServerTrustInstrumentation 2>&1)
`$n=`$LASTEXITCODE; `$o | Write-Output; `$t=`$o -join [Environment]::NewLine
if(`$n -ne 0 -or `$t -notmatch '(?m)^INSTRUMENTATION_RESULT: network_pause_probe=$Expected\r?`$' -or `$t -notmatch '(?m)^INSTRUMENTATION_RESULT: network_pause_version=$probeVersion\r?`$' -or `$t -notmatch '(?m)^INSTRUMENTATION_CODE: -1\r?`$'){throw 'Network pause instrumentation proof missing or failed.'}
"@
    if($FullProof){$instrument+=@"
if(`$t -notmatch '(?m)^INSTRUMENTATION_RESULT: network_pause_target=blocked_then_restored\r?`$' -or `$t -notmatch '(?m)^INSTRUMENTATION_RESULT: network_pause_management=reachable_while_paused\r?`$' -or `$t -notmatch '(?m)^INSTRUMENTATION_RESULT: network_pause_cleanup=passed\r?`$'){throw 'Block, management reachability, resume, or cleanup proof missing.'}
if(`$t -notmatch '(?m)^INSTRUMENTATION_RESULT: network_pause_scope=$probeScope\r?`$'){throw 'Network proof scope mismatch.'}
if(`$t -notmatch '(?m)^INSTRUMENTATION_RESULT: network_pause_dns_target=blocked_then_restored\r?`$' -or `$t -notmatch '(?m)^INSTRUMENTATION_RESULT: network_pause_dns_management=reachable_while_paused\r?`$'){throw 'DNS wire blocking, management reachability or resume proof missing.'}
`$v6=[ordered]@{}
foreach(`$field in @('status','baselineTarget','baselineManagement','scope')){
    `$key=switch(`$field){'status'{'network_pause_ipv6'};'baselineTarget'{'network_pause_ipv6_baseline_target'};'baselineManagement'{'network_pause_ipv6_baseline_management'};'scope'{'network_pause_ipv6_scope'}}
    if(`$t -notmatch ('(?m)^INSTRUMENTATION_RESULT: '+`$key+'=([a-z0-9_-]+)\r?`$')){throw 'IPv6 baseline/status/scope proof missing.'}
    `$v6[`$field]=`$Matches[1]
}
if(`$v6.baselineTarget -notin @('reachable','timeout','unreachable','failed') -or `$v6.baselineManagement -notin @('reachable','timeout','unreachable','failed')){throw 'Unrecognized IPv6 baseline status.'}
if(`$v6.status -ceq 'passed'){
    if(`$v6.baselineTarget -cne 'reachable' -or `$v6.baselineManagement -cne 'reachable' -or `$v6.scope -cne 'native-ipv6-fixture-and-management-uid' -or `$t -notmatch '(?m)^INSTRUMENTATION_RESULT: network_pause_ipv6_target=blocked_then_restored\r?`$' -or `$t -notmatch '(?m)^INSTRUMENTATION_RESULT: network_pause_ipv6_management=reachable_while_paused\r?`$'){throw 'IPv6 success lacks independent baseline, blocking/resume or management proof.'}
}elseif(`$v6.status -ceq 'unproven'){
    if((`$v6.baselineTarget -ceq 'reachable' -and `$v6.baselineManagement -ceq 'reachable') -or `$v6.scope -cne 'unavailable-baseline' -or `$t -match '(?m)^INSTRUMENTATION_RESULT: network_pause_ipv6_(target|management)='){throw 'Unproven IPv6 status conflicts with its baseline or success markers.'}
}else{throw 'Unrecognized IPv6 proof status.'}
[IO.File]::WriteAllText('$ipv6ProofPath',(`$v6 | ConvertTo-Json),[Text.UTF8Encoding]::new(`$false))
"@}
    Invoke-Logged 'P3-004' 'device' $workspace ($guard+'; '+$instrument)
}
function Stop-Fixture{
    if(-not $processStarted){return}
    if(-not (Test-Path -LiteralPath $pidFile)){throw 'Started fixture has no process record; preserve its directory and do not claim cleanup.'}
    $pidValue=0
    if(-not [int]::TryParse((Get-Content -LiteralPath $pidFile -Raw).Trim(),[ref]$pidValue) -or $pidValue -le 0){throw 'Fixture process record is invalid; preserve its directory.'}
    $expected=[IO.Path]::GetFullPath($fixtureExe)
    $body=@"
`$p=Get-Process -Id $pidValue -ErrorAction SilentlyContinue
if(`$null -ne `$p){if(-not [string]::Equals([IO.Path]::GetFullPath(`$p.Path),'$expected',[StringComparison]::OrdinalIgnoreCase)){throw 'Refusing to stop a process outside this test fixture.'}; `$p.Kill(); if(-not `$p.WaitForExit(10000)){throw 'Fixture process remained active; preserve its directory.'}}
if(Get-Process -Id $pidValue -ErrorAction SilentlyContinue){throw 'Fixture process remained active; preserve its directory.'}
Write-Output 'Verified test-owned network fixture stopped.'
"@
    Invoke-Logged 'P3-004' 'tooling' $workspace $body
    $script:processStarted=$false
}

try{
    if(-not (Test-Path -LiteralPath (Join-Path $DebugUserHome '.android\debug.keystore'))){throw 'The existing acceptance-app debug signing home is required; no new signing key is generated.'}
    if($DebugUserHome.Contains("'")){throw 'Unsupported quote in debug signing home.'}
    if(-not (Test-Path -LiteralPath $adb)){throw 'Workspace ADB executable is missing.'}
    $adbList=@(& $adb -P $AdbServerPort devices 2>&1)
    if($LASTEXITCODE -ne 0 -or -not ($adbList -match ('^'+[regex]::Escape($Serial)+'\s+device\b'))){throw 'The selected isolated emulator is not authorized on the requested ADB server.'}
    $preflight=@"
`$f=@(& '$adb' -P $AdbServerPort -s '$Serial' shell getprop ro.build.fingerprint 2>&1); if(`$LASTEXITCODE -ne 0 -or (`$f -join '').Trim() -notlike 'google/sdk_gphone64_x86_64/emu64xa:*'){throw 'Acceptance AVD fingerprint guard failed.'}
`$b=@(& '$adb' -P $AdbServerPort -s '$Serial' shell getprop sys.boot_completed 2>&1); if(`$LASTEXITCODE -ne 0 -or (`$b -join '').Trim() -ne '1'){throw 'Acceptance AVD boot guard failed.'}
Write-Output 'Guarded Android 37 acceptance AVD confirmed.'
"@
    Invoke-Logged 'P3-004' 'device' $workspace $preflight

    # Match the highest installed managed/test version; never request a downgrade.
    $buildVersion=[long]6
    foreach($package in @('com.appcontrol.child','com.appcontrol.child.test')){
        $metadata=@(& $adb -P $AdbServerPort -s $Serial shell dumpsys package $package 2>&1)
        if($LASTEXITCODE -ne 0){throw 'Installed acceptance version query failed.'}
        $text=$metadata -join "`n"
        if($text -match '\bversionCode=([0-9]+)\b'){
            if($text -notmatch 'pkgFlags=.*DEBUGGABLE'){throw 'Existing child/test must be a disposable debug build.'}
            $null=$text -match '\bversionCode=([0-9]+)\b'
            $buildVersion=[Math]::Max($buildVersion,[long]$Matches[1])
        }
    }
    if($buildVersion -gt 2100000000){throw 'Installed acceptance version is outside the supported build range.'}

    $build=@"
`$env:JAVA_HOME='$java'; `$env:GRADLE_USER_HOME='$gradleHome'
& '$gradle' --offline --no-daemon '-Duser.home=$DebugUserHome' '-Pkotlin.compiler.execution.strategy=in-process' '-PappControlVersionCode=$buildVersion' '-PappControlVersionName=0.1.4-network-pause-fixture' :network-fixture:assembleDebug :child-app:assembleDebug :child-app:assembleDebugAndroidTest
if(`$LASTEXITCODE -ne 0){throw 'Network fixture and child instrumentation build failed.'}
"@
    Invoke-Logged 'P3-004' 'build' (Join-Path $workspace 'android') $build

    $aapt=Join-Path $workspace '.tooling\android-sdk\build-tools\36.0.0\aapt.exe'
    $metadata=@"
`$expected=@(@('$apk','com.appcontrol.child'),@('$testApk','com.appcontrol.child.test'),@('$fixtureApk','org.example.appcontrolnetworkfixture'))
foreach(`$e in `$expected){`$d=@(& '$aapt' dump badging `$e[0] 2>&1);if(`$LASTEXITCODE -ne 0 -or (`$d -join "``n") -notmatch ("(?m)^package: name='"+[regex]::Escape(`$e[1])+"'")){throw 'Built APK identity mismatch.'}}
Write-Output 'Verified base child, child test, and network fixture APK identities before installation.'
"@
    Invoke-Logged 'P3-004' 'verification' $workspace $metadata
    Invoke-Logged 'P3-004' 'verification' $workspace "& './scripts/assert-emulator-apk-replacement.ps1' -Serial '$Serial' -AdbServerPort $AdbServerPort -Apk @('$fixtureApk','$apk','$testApk')"

    $install=@"
`$f=@(& '$adb' -P $AdbServerPort -s '$Serial' shell getprop ro.build.fingerprint 2>&1);if(`$LASTEXITCODE -ne 0 -or (`$f -join '').Trim() -notlike 'google/sdk_gphone64_x86_64/emu64xa:*'){throw 'AVD fingerprint changed before install.'}
foreach(`$apkPath in @('$fixtureApk','$apk','$testApk')){`$r=@(& '$adb' -P $AdbServerPort -s '$Serial' install -r `$apkPath 2>&1);`$r|Write-Output;if(`$LASTEXITCODE -ne 0){throw 'In-place acceptance AVD install failed; no uninstall or data clear attempted.'}}
Write-Output 'Installed network fixture and updated guarded AVD child/test APKs in place; parent phone untouched.'
"@
    Invoke-Logged 'P3-004' 'device' $workspace $install
    $installed=$true
    $localPermission=@"
`$f=@(& '$adb' -P $AdbServerPort -s '$Serial' shell getprop ro.build.fingerprint 2>&1);if(`$LASTEXITCODE -ne 0 -or (`$f -join '').Trim() -notlike 'google/sdk_gphone64_x86_64/emu64xa:*'){throw 'AVD guard failed before fixture permission setup.'}
`$sdk=@(& '$adb' -P $AdbServerPort -s '$Serial' shell getprop ro.build.version.sdk);if(`$LASTEXITCODE -ne 0){throw 'Could not read fixture SDK.'}
if([int](`$sdk -join '').Trim() -ge 37){
    & '$adb' -P $AdbServerPort -s '$Serial' shell pm grant org.example.appcontrolnetworkfixture android.permission.ACCESS_LOCAL_NETWORK
    if(`$LASTEXITCODE -ne 0){throw 'Could not grant local-network access to the test-only fixture UID.'}
}
Write-Output 'Test-only network fixture LAN permission configured; the child uses its production foreground consent button.'
"@
    Invoke-Logged 'P3-004' 'device' $workspace $localPermission

    if(Test-Path -LiteralPath $fixture){throw 'Unique fixture directory unexpectedly exists.'}
    $create="New-Item -ItemType Directory -Path '$fixture' | Out-Null; [IO.File]::WriteAllText('$fixture\fixture-owned.marker','network-pause-v1'); Write-Output 'Created unique fixture-owned directory.'"
    Invoke-Logged 'P3-004' 'tooling' $workspace $create
    if(Test-Path -LiteralPath $ready){throw 'Unique readiness file unexpectedly exists.'}
    $portCheck="`$l=[Net.Sockets.TcpListener]::new([Net.IPAddress]::Loopback,19089); `$v=[Net.Sockets.TcpListener]::new([Net.IPAddress]::IPv6Loopback,19091); `$u=`$null; try{`$l.Start(); `$v.Server.DualMode=`$false; `$v.Start(); `$u=[Net.Sockets.UdpClient]::new([Net.Sockets.AddressFamily]::InterNetwork); `$u.ExclusiveAddressUse=`$true; `$u.Client.Bind([Net.IPEndPoint]::new([Net.IPAddress]::Loopback,19090))}catch{throw 'Fixed TCP19089, DNS UDP19090 or native IPv6 TCP19091 fixture port is unavailable; no process was stopped.'}finally{`$l.Stop();`$v.Stop();if(`$null -ne `$u){`$u.Dispose()}}; Write-Output 'Fixed IPv4 TCP, DNS UDP and native IPv6 TCP fixture ports are available.'"
    Invoke-Logged 'P3-004' 'tooling' $workspace $portCheck

    Invoke-Logged 'P3-004' 'build' (Join-Path $workspace 'tools\network-fixture') "`$env:GOPROXY='off'; `$env:GOTOOLCHAIN='local'; `$env:GOCACHE='$workspace\.cache\go-build'; `$env:GOMODCACHE='$workspace\.cache\go-mod'; & '$go' build -o '$fixtureExe' .; if(`$LASTEXITCODE -ne 0){throw 'Network fixture host build failed.'}"
    $start="`$fixtureArguments='-listen 127.0.0.1:19089 -duration 8m -ready-file " + '"' + $ready + '"' + "'; `$p=Start-Process -FilePath '$fixtureExe' -ArgumentList `$fixtureArguments -WindowStyle Hidden -WorkingDirectory '$workspace' -PassThru; [IO.File]::WriteAllText('$pidFile',`$p.Id.ToString()); Write-Output 'Started bounded test-owned loopback endpoint.'"
    $processStarted=$true
    Invoke-Logged 'P3-004' 'tooling' $workspace $start
    $deadline=[datetime]::UtcNow.AddSeconds(15)
    while([datetime]::UtcNow -lt $deadline -and -not (Test-Path -LiteralPath $ready)){Start-Sleep -Milliseconds 200}
    if(-not (Test-Path -LiteralPath $ready) -or (Get-Content -LiteralPath $ready -Raw).Trim() -ne 'network-fixture-v3 127.0.0.1:19089 dns=127.0.0.1:19090 ipv6=[::1]:19091'){throw 'Network fixture did not publish the expected IPv4 TCP/DNS and native IPv6 readiness marker.'}

    Invoke-DeviceProbe 'consent' 'ready'
    Invoke-DeviceProbe 'run' 'passed' -FullProof
    $report.ipv6=Get-Content -LiteralPath $ipv6ProofPath -Raw | ConvertFrom-Json
    $runtimeProof=$true
    $overall='passed'
}finally{
    if($installed){try{Invoke-DeviceProbe 'cleanup' 'cleaned';$cleanupProof=$true}catch{Write-Output 'Recovery required: cleanup instrumentation failed; inspect child journal before reuse.';$overall='failed'}}
    try{Stop-Fixture}catch{Write-Output $_.Exception.Message;$overall='failed'}
    if(-not $processStarted -and (Test-Path -LiteralPath (Join-Path $fixture 'fixture-owned.marker'))){
        $remove=@"
`$target=[IO.Path]::GetFullPath('$fixture');`$expected=[IO.Path]::GetFullPath('$fixture')
`$cache=[IO.Path]::GetFullPath((Join-Path '$workspace' '.cache'))
`$info=Get-Item -LiteralPath `$target -Force
if(-not [string]::Equals(`$target,`$expected,[StringComparison]::OrdinalIgnoreCase) -or -not `$target.StartsWith(`$cache+[IO.Path]::DirectorySeparatorChar,[StringComparison]::OrdinalIgnoreCase) -or (`$info.Attributes -band [IO.FileAttributes]::ReparsePoint) -ne 0 -or -not (Test-Path -LiteralPath (Join-Path `$target 'fixture-owned.marker'))){throw 'Refusing fixture cleanup without exact in-cache owned directory.'}
Remove-Item -LiteralPath `$target -Recurse -Force;Write-Output 'Removed the unique stopped fixture directory.'
"@
        try{Invoke-Logged 'P3-004' 'tooling' $workspace $remove}catch{$overall='failed'}
    }
    $report.status=$overall
    $report.runtimeProofPassed=$runtimeProof
    $report.androidCleanupPassed=$cleanupProof
    $report.hostFixtureCleanupPassed=(-not $processStarted -and -not (Test-Path -LiteralPath $fixture))
    $report.cleanupPassed=($cleanupProof -and $report.hostFixtureCleanupPassed)
    if(-not $report.hostFixtureCleanupPassed){$overall='failed';$report.status='failed'}
    $report.finishedAtUtc=[datetime]::UtcNow.ToString('o')
    Save-NetworkReport
    Write-Output "Network pause result: $reportPath"
}
if($overall -ne 'passed'){throw 'Per-UID internet-pause acceptance did not complete cleanly.'}
Write-Output 'Passed IPv4 fixture proof: selected UID TCP and DNS-wire packets blocked/restored; management UID reachable throughout. No OS resolver, private DNS, real Internet, FCM or hardware claim.'
Write-Output ("Separate native IPv6 fixture status: "+$report.ipv6.status+"; consult the result report for both UID baselines and scope.")
