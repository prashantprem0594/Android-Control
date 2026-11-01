param(
    [Parameter(Mandatory)] [ValidateSet('127.0.0.1:5581','emulator-5580')] [string]$Serial,
    [ValidateRange(1024,65535)] [int]$AdbServerPort = 5038
)
# Run through run-logged.ps1. Platform endpoint observations, not physical sound.
$ErrorActionPreference='Stop'
$workspace=Split-Path -Parent $PSScriptRoot
$adb=Join-Path $workspace '.tooling\android-sdk\platform-tools\adb.exe'
function Invoke-Adb([string[]]$Arguments){
    $ErrorActionPreference='Continue';$captured=@(& $adb -P $AdbServerPort -s $Serial @Arguments 2>&1)
    return [pscustomobject]@{ExitCode=$LASTEXITCODE;Text=($captured -join "`n")}
}
function Invoke-Probe([string]$Phase,[string]$Expected){
    $probe=Invoke-Adb @('shell','am','instrument','-w','-r','-e','probe','telecom_audio','-e','phase',$Phase,'com.appcontrol.child.test/com.appcontrol.transport.ServerTrustInstrumentation')
    Write-Output $probe.Text
    if($probe.ExitCode -ne 0 -or $probe.Text -notmatch ('(?m)^INSTRUMENTATION_RESULT: telecom_audio_probe='+$Expected+'\r?$') -or
        $probe.Text -notmatch '(?m)^INSTRUMENTATION_RESULT: telecom_audio_version=endpoint-feedback-v1\r?$' -or
        $probe.Text -notmatch '(?m)^INSTRUMENTATION_RESULT: telecom_audio_scope=platform-callbacks-only\r?$' -or
        $probe.Text -notmatch '(?m)^INSTRUMENTATION_CODE: -1\r?$'){throw 'Synthetic audio endpoint/feedback phase did not pass.'}
    if($Phase -eq 'run' -and ($probe.Text -notmatch '(?m)^INSTRUMENTATION_RESULT: telecom_audio_speaker=observed\r?$' -or
        $probe.Text -notmatch '(?m)^INSTRUMENTATION_RESULT: telecom_audio_earpiece=(observed|unavailable)\r?$')){throw 'Missing observed route proof.'}
}
$fingerprint=Invoke-Adb @('shell','getprop','ro.build.fingerprint')
if($fingerprint.ExitCode -ne 0 -or -not $fingerprint.Text.Trim().StartsWith('google/sdk_gphone64_x86_64/emu64xa:')){throw 'Isolated acceptance emulator required.'}
$holders=Invoke-Adb @('shell','cmd','role','get-role-holders','--user','0','android.app.role.DIALER');$original=$holders.Text.Trim()
if($holders.ExitCode -ne 0 -or $original -notmatch '^[A-Za-z0-9_]+(\.[A-Za-z0-9_]+)+$' -or $original -eq 'com.appcontrol.child'){throw 'Known original emulator dialer required.'}
$cleanupNeeded=$false;$cleanupFailed=$false
try{
    $added=Invoke-Adb @('shell','cmd','role','add-role-holder','--user','0','android.app.role.DIALER','com.appcontrol.child','0')
    if($added.ExitCode -ne 0){throw 'Normal role qualification failed.'}
    Invoke-Probe 'prepare' 'prepared';$cleanupNeeded=$true
    $enabled=Invoke-Adb @('shell','cmd','telecom','set-phone-account-enabled','org.example.appcontroltelecomfixture/org.example.appcontroltelecomfixture.FixtureConnectionService','app-control-synthetic-fixture','0')
    if($enabled.ExitCode -ne 0){throw 'Synthetic SIP account enablement failed.'}
    Invoke-Probe 'run' 'passed'
}finally{
    if($cleanupNeeded){try{Invoke-Probe 'cleanup' 'cleaned'}catch{$cleanupFailed=$true}}
    $restored=Invoke-Adb @('shell','cmd','role','add-role-holder','--user','0','android.app.role.DIALER',$original,'0')
    $role=Invoke-Adb @('shell','cmd','role','get-role-holders','--user','0','android.app.role.DIALER')
    $telecom=Invoke-Adb @('shell','cmd','telecom','get-default-dialer')
    if($restored.ExitCode -ne 0 -or $role.ExitCode -ne 0 -or $telecom.ExitCode -ne 0 -or $role.Text.Trim() -ne $original -or $telecom.Text.Trim() -ne $original){throw 'Original dialer restoration failed.'}
    Write-Output 'Original emulator role and Telecom dialer restored.'
    if($cleanupFailed){throw 'Audio fixture cleanup failed; inspect before reuse.'}
}
Write-Output 'Observed endpoint and repeated-choice feedback checks passed; actual sound, legacy Android 13, wired/Bluetooth and Redmi remain unverified.'
