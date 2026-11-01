param(
    [Parameter(Mandatory)] [ValidateSet('127.0.0.1:5581','emulator-5580')] [string]$Serial,
    [ValidateRange(1024,65535)] [int]$AdbServerPort = 5038
)
# Invoke through run-logged.ps1. Fixed synthetic SIP account, no media/network.
$ErrorActionPreference = 'Stop'
$workspace = Split-Path -Parent $PSScriptRoot
$adb = Join-Path $workspace '.tooling\android-sdk\platform-tools\adb.exe'
function Invoke-Adb([string[]]$Arguments) {
    $ErrorActionPreference = 'Continue'
    $captured = @(& $adb -P $AdbServerPort -s $Serial @Arguments 2>&1)
    return [pscustomobject]@{ExitCode=$LASTEXITCODE;Text=($captured -join "`n")}
}
function Invoke-Probe([string]$Phase,[string]$Expected) {
    $probe=Invoke-Adb @('shell','am','instrument','-w','-r','-e','probe','telecom_recovery','-e','phase',$Phase,'com.appcontrol.child.test/com.appcontrol.transport.ServerTrustInstrumentation')
    Write-Output $probe.Text
    if($probe.ExitCode -ne 0 -or $probe.Text -notmatch ('(?m)^INSTRUMENTATION_RESULT: telecom_recovery_probe='+$Expected+'\r?$') -or
        $probe.Text -notmatch '(?m)^INSTRUMENTATION_RESULT: telecom_recovery_version=process-recovery-v1\r?$' -or
        $probe.Text -notmatch '(?m)^INSTRUMENTATION_CODE: -1\r?$'){throw 'Synthetic Telecom recovery phase did not pass.'}
}
function Test-ServiceBound([string]$Component,[string]$Package,[string]$ExpectedProcess='') {
    $service=Invoke-Adb @('shell','dumpsys','activity','services',$Component)
    $processPattern=if($ExpectedProcess -ne ''){[regex]::Escape($ExpectedProcess)+':'+[regex]::Escape($Package)+'/'}else{'[0-9]+:'+ [regex]::Escape($Package)+'(?::[A-Za-z0-9_.]+)?/'}
    return ($service.ExitCode -eq 0 -and $service.Text.Contains('hasBound=true') -and
        $service.Text -match ('app=ProcessRecord\{[^\r\n]*\s'+$processPattern))
}
$fingerprint=Invoke-Adb @('shell','getprop','ro.build.fingerprint')
if($fingerprint.ExitCode -ne 0 -or -not $fingerprint.Text.Trim().StartsWith('google/sdk_gphone64_x86_64/emu64xa:')){throw 'Isolated acceptance emulator required.'}
$holders=Invoke-Adb @('shell','cmd','role','get-role-holders','--user','0','android.app.role.DIALER')
$original=$holders.Text.Trim()
if($holders.ExitCode -ne 0 -or $original -notmatch '^[A-Za-z0-9_]+(\.[A-Za-z0-9_]+)+$' -or $original -eq 'com.appcontrol.child'){throw 'Known original emulator dialer required.'}
$system=Invoke-Adb @('shell','cmd','telecom','get-system-dialer')
$systemPackage=$system.Text.Trim()
if($system.ExitCode -ne 0 -or $systemPackage -notmatch '^[A-Za-z0-9_]+(\.[A-Za-z0-9_]+)+$'){throw 'Known preloaded dialer required.'}
$systemQuery=Invoke-Adb @('shell','cmd','package','query-services','--user','0','--brief','-a','android.telecom.InCallService','-p',$systemPackage)
$systemServices=@($systemQuery.Text -split '\r?\n' | ForEach-Object {$_.Trim()} | Where-Object {$_ -match ('^'+[regex]::Escape($systemPackage)+'/[A-Za-z0-9_.]+$')})
if($systemQuery.ExitCode -ne 0 -or $systemServices.Count -eq 0){throw 'Preloaded in-call service lookup failed.'}
$cleanupNeeded=$false;$cleanupFailed=$false;$armClient=$null
$evidence=Join-Path $workspace ('.cache\telecom-recovery-'+[guid]::NewGuid().ToString('N'))
$null=New-Item -ItemType Directory -Path $evidence
$armOutput=Join-Path $evidence 'arm-stdout.txt'
try {
    $added=Invoke-Adb @('shell','cmd','role','add-role-holder','--user','0','android.app.role.DIALER','com.appcontrol.child','0')
    if($added.ExitCode -ne 0){throw 'Normal dialer qualification failed.'}
    Invoke-Probe 'prepare' 'prepared';$cleanupNeeded=$true
    $enabled=Invoke-Adb @('shell','cmd','telecom','set-phone-account-enabled','org.example.appcontroltelecomfixture/org.example.appcontroltelecomfixture.FixtureConnectionService','app-control-synthetic-fixture','0')
    if($enabled.ExitCode -ne 0){throw 'Synthetic SIP account enablement failed.'}
    $armClient=Start-Process -FilePath $adb -WindowStyle Hidden -PassThru -ArgumentList @('-P',"$AdbServerPort",'-s',$Serial,'shell','am','instrument','-w','-r','-e','probe','telecom_recovery','-e','phase','arm','com.appcontrol.child.test/com.appcontrol.transport.ServerTrustInstrumentation') -RedirectStandardOutput $armOutput -RedirectStandardError (Join-Path $evidence 'arm-stderr.txt')
    $armDeadline=[datetime]::UtcNow.AddSeconds(40);$armedProcess=''
    while([datetime]::UtcNow -lt $armDeadline){
        if(Test-Path -LiteralPath $armOutput){
            $statusStream=[IO.File]::Open($armOutput,[IO.FileMode]::Open,[IO.FileAccess]::Read,[IO.FileShare]::ReadWrite)
            $statusReader=[IO.StreamReader]::new($statusStream,[Text.Encoding]::UTF8)
            try{$status=$statusReader.ReadToEnd()}finally{$statusReader.Dispose()}
            $reported=[regex]::Match($status,'(?m)^INSTRUMENTATION_STATUS: telecom_recovery_pid=([1-9][0-9]*)\r?$')
            if($status -match '(?m)^INSTRUMENTATION_STATUS: telecom_recovery_armed=true\r?$' -and
                $status -match '(?m)^INSTRUMENTATION_STATUS: telecom_recovery_version=process-recovery-v1\r?$' -and $reported.Success){$armedProcess=$reported.Groups[1].Value;break}
        }
        if($armClient.HasExited){throw 'Arming instrumentation exited before reporting a live synthetic call.'}
        Start-Sleep -Milliseconds 100
    }
    if($armedProcess -eq ''){throw 'Synthetic call arming status timed out.'}
    $before=Invoke-Adb @('shell','pidof','com.appcontrol.child')
    $childProcess=$before.Text.Trim()
    if($before.ExitCode -ne 0 -or $childProcess -notmatch '^[1-9][0-9]*$' -or $childProcess -ne $armedProcess){throw 'Reported arming PID must match the live child process.'}
    # Same UID via run-as; kill only this guarded emulator's exact child PID.
    $killed=Invoke-Adb @('shell','run-as','com.appcontrol.child','kill','-9',$childProcess)
    if($killed.ExitCode -ne 0){throw 'Synthetic child process termination failed.'}
    $deadline=[datetime]::UtcNow.AddSeconds(45);$rebound=$false;$preloadedBound=$false;$recoveryMode='automatic-child-rebind'
    while([datetime]::UtcNow -lt $deadline){
        $next=Invoke-Adb @('shell','pidof','com.appcontrol.child')
        $nextProcess=$next.Text.Trim()
        if($next.ExitCode -eq 0 -and $nextProcess -match '^[1-9][0-9]*$' -and $nextProcess -ne $childProcess){
            if(Test-ServiceBound 'com.appcontrol.child/.calls.ManagedInCallService' 'com.appcontrol.child' $nextProcess){$rebound=$true;break}
        }
        foreach($component in $systemServices){if(Test-ServiceBound $component $systemPackage){$preloadedBound=$true;break}}
        if($preloadedBound){break}
        Start-Sleep -Milliseconds 250
    }
    if(-not $rebound){
        if(-not $preloadedBound){throw 'Neither child rebind nor a live preloaded in-call binding was observed after process death.'}
        Write-Output 'Observed a live preloaded in-call binding after child process death; testing explicit Android UI control and next-call child recovery.'
        $recoveryMode='preloaded-ui-and-explicit-next-call-requalification'
        Invoke-Probe 'fallback' 'passed'
        # End the old call first. Re-select normally for a new call session;
        # Android does not promise a mid-call handoff back to a failed dialer.
        foreach($holder in @($systemPackage,'com.appcontrol.child')){
            $selected=Invoke-Adb @('shell','cmd','role','add-role-holder','--user','0','android.app.role.DIALER',$holder,'0')
            if($selected.ExitCode -ne 0){throw 'Post-call normal dialer requalification failed.'}
        }
        Write-Output 'Requalified the child through normal role selection after the preloaded UI ended the old call.'
        Invoke-Probe 'next-call' 'passed'
    }else{
        Write-Output ('Observed a new child process hosting ManagedInCallService before verification. Recovery mode: '+$recoveryMode)
        Invoke-Probe 'recover' 'passed'
    }
} catch {
    Write-Output ('Recovery exercise failure: '+$_.Exception.Message)
    throw
} finally {
    if($null -ne $armClient -and -not $armClient.HasExited){$armClient.Kill()}
    if($cleanupNeeded){try{
        $stopped=Invoke-Adb @('shell','am','force-stop','com.appcontrol.child')
        if($stopped.ExitCode -ne 0){throw 'Could not stop pending arming instrumentation before cleanup.'}
        Invoke-Probe 'cleanup' 'cleaned'
    }catch{Write-Output ('Recovery cleanup failure: '+$_.Exception.Message);$cleanupFailed=$true}}
    $restored=Invoke-Adb @('shell','cmd','role','add-role-holder','--user','0','android.app.role.DIALER',$original,'0')
    $role=Invoke-Adb @('shell','cmd','role','get-role-holders','--user','0','android.app.role.DIALER')
    $telecom=Invoke-Adb @('shell','cmd','telecom','get-default-dialer')
    if($restored.ExitCode -ne 0 -or $role.ExitCode -ne 0 -or $telecom.ExitCode -ne 0 -or $role.Text.Trim() -ne $original -or $telecom.Text.Trim() -ne $original){throw 'Original dialer restoration failed.'}
    Write-Output 'Original emulator role and Telecom dialer restored.'
    if($cleanupFailed){throw 'Recovery fixture cleanup failed; inspect before reuse.'}
}
Write-Output "Bounded arming-process evidence: $evidence"
Write-Output ('Synthetic process-death call continuity, fresh identity, stale-action refusal and local control passed via '+$recoveryMode+'. Real SIM, audio, locked-screen and Redmi behavior remain separate.')
