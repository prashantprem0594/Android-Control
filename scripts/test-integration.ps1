param(
    [switch]$Emulator,
    [switch]$RequireLiveServerHttps,
    [switch]$ObserveCallWatchWindow,
    [switch]$RequireLocalNetwork,
    [switch]$ExerciseLocalNetworkDenial,
    [switch]$RequireParentPushApi,
    [ValidateSet('127.0.0.1:5581','emulator-5580')] [string]$Serial='127.0.0.1:5581'
)
# Each stage runs in a separate logged process. Never installs, clears, or resets apps.
$ErrorActionPreference='Stop'
if($RequireLiveServerHttps -and -not $Emulator){throw '-RequireLiveServerHttps requires -Emulator so live probes cannot be silently skipped.'}
if($ObserveCallWatchWindow -and -not $RequireLiveServerHttps){throw '-ObserveCallWatchWindow requires live HTTPS acceptance.'}
if($RequireLocalNetwork -and -not $RequireLiveServerHttps){throw '-RequireLocalNetwork requires live HTTPS acceptance.'}
if($ExerciseLocalNetworkDenial -and -not $RequireLocalNetwork){throw '-ExerciseLocalNetworkDenial requires local-network HTTPS acceptance.'}
if($RequireParentPushApi -and -not $RequireLiveServerHttps){throw '-RequireParentPushApi requires live HTTPS acceptance.'}
$workspace=Split-Path -Parent $PSScriptRoot
$reportDirectory=Join-Path $workspace ('.cache/integration-'+[guid]::NewGuid().ToString('N'))
New-Item -ItemType Directory -Path $reportDirectory | Out-Null
$reportScope = if ($RequireLiveServerHttps) { 'local-server-and-live-android-https-acceptance' } else { 'local-server-and-android-component-checks' }
$limitations = @('Real FCM, physical phone, SIM calls and production deployment are not tested.')
if ($RequireLiveServerHttps) {
    $limitations += @('ParentClient uses its production request flow with the test-only local CA and matching ephemeral SPKI pin.',
        'Child acceptance probe uses production EnrollmentClient and AndroidKeyStore DeviceIdentity for enrollment and authenticated HTTPS requests; the Device Owner-only LOCK_DEVICE action and policy reconciliation run through production CommandSync with the action safely declined on the non-owner AVD.',
        'The fetched LOCK_DEVICE command is acknowledged as failed and is never executed; policy reports device_owner_required and is not applied.')
} else {
    $limitations += 'Android client component probes use connection doubles, not a live Android-to-server session.'
}
$report=[ordered]@{schemaVersion=1;startedAtUtc=[datetime]::UtcNow.ToString('o');status='running';scope=$reportScope;stages=@();limitations=$limitations;emulatorRequested=[bool]$Emulator;liveServerHttpsRequired=[bool]$RequireLiveServerHttps;callWatchWindowObservationRequested=[bool]$ObserveCallWatchWindow}
$reportPath=Join-Path $reportDirectory 'result.json'
function Save-Report { $report | ConvertTo-Json -Depth 8 | Set-Content -LiteralPath $reportPath -Encoding UTF8 }
function Stage([string]$Name,[string]$Directory,[string]$Command){
    $started=[datetime]::UtcNow
    # EncodedCommand preserves literal quotes and spaces across Windows argv.
    $wrapper=(Join-Path $PSScriptRoot 'run-logged.ps1').Replace("'","''")
    $script="& '$wrapper' -TaskId INTEGRATION-001 -Category test -WorkingDirectory '"+$Directory.Replace("'","''")+"' -Command '"+$Command.Replace("'","''")+"'"
    $encoded=[Convert]::ToBase64String([Text.Encoding]::Unicode.GetBytes($script))
    & powershell.exe -NoProfile -ExecutionPolicy Bypass -OutputFormat Text -EncodedCommand $encoded
    $code=$LASTEXITCODE
    $report.stages+=@{name=$Name;exitCode=$code;startedAtUtc=$started.ToString('o');finishedAtUtc=[datetime]::UtcNow.ToString('o')}
    Save-Report
    if($code -ne 0){throw "Integration stage failed: $Name"}
}
Save-Report
try {
    if($Emulator){
        Stage 'isolated-emulator-guard' $workspace @"
`$fingerprint=@(& '.\.tooling\android-sdk\platform-tools\adb.exe' -P 5038 -s '$Serial' shell getprop ro.build.fingerprint)
if(`$LASTEXITCODE -ne 0 -or (`$fingerprint -join '').Trim() -notlike 'google/sdk_gphone64_x86_64/emu64xa:*'){throw 'Isolated acceptance emulator required; physical devices are refused.'}
Write-Output 'Isolated emulator guard passed. No installation, reset or data clearing is performed.'
"@
    }
    Stage 'server-contracts-https-recovery' 'server' '$env:GOPROXY="off"; $env:GOTOOLCHAIN="local"; $env:GOCACHE="C:\Android Control\.cache\go-build"; $env:GOMODCACHE="C:\Android Control\.cache\go-mod"; & "C:\Program Files\Go\bin\go.exe" test -count=1 ./...'
    Stage 'production-input-prerequisites' $workspace '& "./scripts/test-production-prerequisites.ps1"'
    Stage 'apk-replacement-rules' $workspace '& "./scripts/test-apk-replacement-rules.ps1"'
    Stage 'network-proof-contract' $workspace '& "./scripts/test-network-proof-contract.ps1"'
    foreach($check in @('parent-call-watch','parent-push','parent-contact-state','package-policy','call-actions','command-journal','wakeup-hints')){
        Stage "java-$check" $workspace ("& './scripts/test-$check.ps1'")
    }
    Stage 'android-lint' 'android' '$env:JAVA_HOME="C:\Program Files\Java\jdk-17"; $env:GRADLE_USER_HOME="C:\Android Control\.cache\gradle"; & "C:\Gradle\gradle-9.6.0\bin\gradle.bat" --offline --no-daemon "-Pkotlin.compiler.execution.strategy=in-process" :parent-app:lintDebug :child-app:lintDebug'
    if($Emulator){
        if($ExerciseLocalNetworkDenial){
            Stage 'prepare-parent-fixture-lan-denial' $workspace @"
`$adb='.\.tooling\android-sdk\platform-tools\adb.exe'
`$f=@(& `$adb -P 5038 -s '$Serial' shell getprop ro.build.fingerprint)
if(`$LASTEXITCODE -ne 0 -or (`$f -join '').Trim() -notlike 'google/sdk_gphone64_x86_64/emu64xa:*'){throw 'Acceptance emulator required before permission reset.'}
`$package=@(& `$adb -P 5038 -s '$Serial' shell dumpsys package com.appcontrol.parent.acceptance)
if(`$LASTEXITCODE -ne 0 -or -not (`$package -match 'pkgFlags=.*DEBUGGABLE')){throw 'Disposable debug parent package required before permission reset.'}
& `$adb -P 5038 -s '$Serial' shell pm revoke com.appcontrol.parent.acceptance android.permission.ACCESS_LOCAL_NETWORK
if(`$LASTEXITCODE -ne 0){throw 'Could not reset the fixture parent LAN permission.'}
& `$adb -P 5038 -s '$Serial' shell pm clear-permission-flags com.appcontrol.parent.acceptance android.permission.ACCESS_LOCAL_NETWORK user-set user-fixed
if(`$LASTEXITCODE -ne 0){throw 'Could not reset the fixture permission-choice flags.'}
Write-Output 'Only the disposable emulator parent LAN permission was reset; no app data or other permissions changed.'
"@
        }
        $probes=@(@('parent','parent_client'),@('parent','parent_session'),@('child','contact_cache'))
        if($RequireLiveServerHttps){$probes=@(@('child','contact_cache'),@('child','child_contact_ui'),@('parent','parent_client'),@('parent','parent_session'))}
        if($RequireLocalNetwork){$probes=@(@('child','local_network_setup'),@('parent','local_network_setup'))+$probes}
        if($RequireLiveServerHttps){$probes+=,@('parent','parent_ui_https')}
        if($RequireParentPushApi){$probes+=,@('parent','parent_push_https')}
        foreach($probe in $probes){
            $app=$probe[0];$name=$probe[1]
            $instrumentationPackage=if($RequireLiveServerHttps){"com.appcontrol.$app.acceptance.test"}else{"com.appcontrol.$app.test"}
            $resultMarker=if($name -eq 'parent_ui_https'){'server_https_parent_ui_probe'}elseif($name -eq 'parent_push_https'){'server_https_parent_push_api_probe'}else{"${name}_probe"}
            $windowArgument=if($ObserveCallWatchWindow -and $name -eq 'parent_ui_https'){'-e observe_call_watch_window true'}else{''}
            $denialArgument=if($ExerciseLocalNetworkDenial -and $app -eq 'parent' -and $name -eq 'local_network_setup'){'-e deny_lan_once true'}else{''}
            Stage "emulator-$name" $workspace @"
`$proof=@(& '.\.tooling\android-sdk\platform-tools\adb.exe' -P 5038 -s '$Serial' shell am instrument -w -r -e probe '$name' $windowArgument $denialArgument '$instrumentationPackage/com.appcontrol.transport.ServerTrustInstrumentation' 2>&1)
`$code=`$LASTEXITCODE;`$proof | Write-Output;`$text=`$proof -join "``n"
if(`$code -ne 0 -or `$text -notmatch '(?m)^INSTRUMENTATION_RESULT: ${resultMarker}=passed\r?$' -or `$text -notmatch '(?m)^INSTRUMENTATION_CODE: -1\r?$'){throw 'Android integration component proof missing or failed.'}
if('$RequireLiveServerHttps' -eq 'True'){
    if('$name' -eq 'parent_client' -and `$text -notmatch '(?m)^INSTRUMENTATION_RESULT: parent_push_client_probe=passed\r?$'){throw 'Parent push client authorization/registration/feed-fallback proof missing or failed.'}
    if('$ExerciseLocalNetworkDenial' -eq 'True' -and '$app' -eq 'parent' -and '$name' -eq 'local_network_setup' -and `$text -notmatch '(?m)^INSTRUMENTATION_RESULT: local_network_denial_probe=passed\r?$'){throw 'LAN denial and explicit retry proof missing or failed.'}
    `$liveMarker=if('$name' -eq 'parent_client'){'server_https_parent_probe=passed'}elseif('$name' -eq 'contact_cache'){'server_https_child_probe=passed'}else{''}
    if(-not [string]::IsNullOrWhiteSpace(`$liveMarker) -and `$text -notmatch ('(?m)^INSTRUMENTATION_RESULT: '+[regex]::Escape(`$liveMarker)+'\r?$')){throw 'Live Android HTTPS acceptance proof missing or failed.'}
    if('$name' -eq 'contact_cache' -and `$text -notmatch '(?m)^INSTRUMENTATION_RESULT: server_https_contact_sync_probe=passed\r?$'){throw 'Live Android HTTPS contact synchronization proof missing or failed.'}
    if('$name' -eq 'contact_cache' -and `$text -notmatch '(?m)^INSTRUMENTATION_RESULT: server_https_contact_recovery_probe=passed\r?$'){throw 'Live Android HTTPS contact recovery proof missing or failed.'}
    if('$name' -eq 'contact_cache' -and `$text -notmatch '(?m)^INSTRUMENTATION_RESULT: server_https_production_enrollment_probe=passed\r?$'){throw 'Current production DeviceEnrollmentClient enrollment and pre-revocation session proof missing or failed.'}
    if('$name' -eq 'parent_ui_https' -and `$text -notmatch '(?m)^INSTRUMENTATION_RESULT: server_https_parent_controls_ui_probe=passed\r?$'){throw 'Parent controls UI HTTPS proof missing or failed.'}
    if('$name' -eq 'parent_ui_https' -and `$text -notmatch '(?m)^INSTRUMENTATION_RESULT: server_https_parent_call_watch_probe=passed\r?$'){throw 'Parent call-watch runtime proof missing or failed.'}
    if('$name' -eq 'parent_ui_https' -and '$ObserveCallWatchWindow' -eq 'True' -and `$text -notmatch '(?m)^INSTRUMENTATION_RESULT: parent_call_watch_window_probe=passed\r?$'){throw 'Elapsed call-watch window proof missing or failed.'}
}
"@
        }
    }
    $report.status='passed'
}catch{
    $report.status='failed'
    throw
}finally{
    $report.finishedAtUtc=[datetime]::UtcNow.ToString('o')
    Save-Report
    Write-Output "Integration report: $reportPath"
}
