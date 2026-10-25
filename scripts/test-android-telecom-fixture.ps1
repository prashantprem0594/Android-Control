param(
    [Parameter(Mandatory)] [ValidatePattern('^[A-Za-z0-9._:-]+$')] [string]$Serial,
    [ValidateRange(1024, 65535)] [int]$AdbServerPort = 5038
)
# Invoke through run-logged.ps1. Only the fixed non-SIM SIP fixture account is
# enabled. No outgoing/emergency account or media capability is requested.
$ErrorActionPreference = 'Stop'
$adb = 'C:\Users\Sushant\AppData\Local\Android\Sdk\platform-tools\adb.exe'
function Invoke-AdbCapture([string[]]$Arguments) {
    $ErrorActionPreference = 'Continue'
    $captured = @(& $adb -P $AdbServerPort -s $Serial @Arguments 2>&1)
    return [pscustomobject]@{ExitCode=$LASTEXITCODE;Text=($captured -join "`n")}
}
function Invoke-Probe([string]$Phase, [string]$Expected) {
    $probe = Invoke-AdbCapture @('shell','am','instrument','-w','-r','-e','probe','telecom_fixture','-e','phase',$Phase,'com.appcontrol.child.test/com.appcontrol.transport.ServerTrustInstrumentation')
    Write-Output $probe.Text
    if ($probe.ExitCode -ne 0 -or $probe.Text -notmatch ('(?m)^INSTRUMENTATION_RESULT: telecom_probe=' + $Expected + '\r?$') -or
        $probe.Text -notmatch '(?m)^INSTRUMENTATION_CODE: -1\r?$' -or
        $probe.Text -notmatch '(?m)^INSTRUMENTATION_RESULT: telecom_probe_version=ui-controls-v1\r?$') { throw 'Synthetic Telecom fixture phase or expected probe version did not pass.' }
}
$fingerprint = Invoke-AdbCapture @('shell','getprop','ro.build.fingerprint')
if ($fingerprint.ExitCode -ne 0 -or -not $fingerprint.Text.Trim().StartsWith('google/sdk_gphone64_x86_64/emu64xa:')) { throw 'Synthetic Telecom requires the isolated acceptance emulator.' }
$holders = Invoke-AdbCapture @('shell','cmd','role','get-role-holders','--user','0','android.app.role.DIALER')
$original = $holders.Text.Trim()
if ($holders.ExitCode -ne 0 -or $original -notmatch '^[A-Za-z0-9_]+(\.[A-Za-z0-9_]+)+$' -or $original -eq 'com.appcontrol.child') { throw 'Original emulator dialer holder must be known before the exercise.' }
$cleanupNeeded = $false
$cleanupFailed = $false
try {
    $awake = Invoke-AdbCapture @('shell','input','keyevent','KEYCODE_WAKEUP')
    $unlocked = Invoke-AdbCapture @('shell','wm','dismiss-keyguard')
    if ($awake.ExitCode -ne 0 -or $unlocked.ExitCode -ne 0) { throw 'Could not prepare unlocked emulator screen exercise.' }
    $added = Invoke-AdbCapture @('shell','cmd','role','add-role-holder','--user','0','android.app.role.DIALER','com.appcontrol.child','0')
    if ($added.ExitCode -ne 0) { throw 'Normal dialer role qualification failed.' }
    $cleanupNeeded = $true
    Invoke-Probe 'prepare' 'prepared'
    $enabled = Invoke-AdbCapture @('shell','cmd','telecom','set-phone-account-enabled','org.example.appcontroltelecomfixture/org.example.appcontroltelecomfixture.FixtureConnectionService','app-control-synthetic-fixture','0')
    if ($enabled.ExitCode -ne 0) { throw 'Synthetic managed SIP account enablement failed.' }
    Invoke-Probe 'run' 'passed'
} finally {
    if ($cleanupNeeded) {
        try { Invoke-Probe 'cleanup' 'cleaned' } catch { $cleanupFailed = $true }
    }
    $restored = Invoke-AdbCapture @('shell','cmd','role','add-role-holder','--user','0','android.app.role.DIALER',$original,'0')
    $role = Invoke-AdbCapture @('shell','cmd','role','get-role-holders','--user','0','android.app.role.DIALER')
    $telecom = Invoke-AdbCapture @('shell','cmd','telecom','get-default-dialer')
    if ($restored.ExitCode -ne 0 -or $role.ExitCode -ne 0 -or $telecom.ExitCode -ne 0 -or $role.Text.Trim() -ne $original -or $telecom.Text.Trim() -ne $original) { throw 'Original dialer restoration failed.' }
    Write-Output 'Original emulator role and Telecom dialer restored.'
    if ($cleanupFailed) { throw 'Synthetic account cleanup failed; inspect the isolated fixture before reuse.' }
}
Write-Output 'Synthetic non-SIM Telecom call/control checks passed with fixture cleanup. Real SIM, audio, emergency and Redmi tests remain open.'
