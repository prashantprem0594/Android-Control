param(
    [Parameter(Mandatory)] [ValidatePattern('^[A-Za-z0-9._:-]+$')] [string]$Serial,
    [ValidateRange(1024, 65535)] [int]$AdbServerPort = 5038
)
# Invoke through run-logged.ps1. This never presses Call, supplies a number,
# registers a phone account or bypasses Android role qualification.
$ErrorActionPreference = 'Stop'
$adb = 'C:\Users\Sushant\AppData\Local\Android\Sdk\platform-tools\adb.exe'
function Invoke-AdbCapture([string[]]$Arguments) {
    $ErrorActionPreference = 'Continue'
    $captured = @(& $adb -P $AdbServerPort -s $Serial @Arguments 2>&1)
    return [pscustomobject]@{ExitCode=$LASTEXITCODE;Text=($captured -join "`n")}
}
$fingerprint = Invoke-AdbCapture @('shell','getprop','ro.build.fingerprint')
if ($fingerprint.ExitCode -ne 0 -or -not $fingerprint.Text.Trim().StartsWith('google/sdk_gphone64_x86_64/emu64xa:')) { throw 'Dialer role test requires the isolated Android acceptance emulator.' }
$state = Invoke-AdbCapture @('shell','run-as','com.appcontrol.child','cat','shared_prefs/enrollment.xml')
if ($state.ExitCode -eq 0) {
    [xml]$enrollment = $state.Text
    if ($null -ne $enrollment.SelectSingleNode("/map/string[@name='deviceId']")) { throw 'Dialer fixture refuses an enrolled child.' }
} elseif (-not $state.Text.Contains('No such file')) { throw 'Could not verify unenrolled fixture state.' }
$holders = Invoke-AdbCapture @('shell','cmd','role','get-role-holders','--user','0','android.app.role.DIALER')
$original = $holders.Text.Trim()
if ($holders.ExitCode -ne 0 -or $original -notmatch '^[A-Za-z0-9_]+(\.[A-Za-z0-9_]+)+$' -or $original -eq 'com.appcontrol.child') { throw 'Expected one original system dialer role holder.' }
try {
    $added = Invoke-AdbCapture @('shell','cmd','role','add-role-holder','--user','0','android.app.role.DIALER','com.appcontrol.child','0')
    if ($added.ExitCode -ne 0) { throw 'Android refused normal dialer role qualification.' }
    $current = Invoke-AdbCapture @('shell','cmd','telecom','get-default-dialer')
    if ($current.ExitCode -ne 0 -or $current.Text.Trim() -ne 'com.appcontrol.child') { throw 'Telecom did not observe the child as default dialer.' }
    foreach ($withScheme in @($false,$true)) {
        $arguments = @('shell','am','start','-W','-a','android.intent.action.DIAL')
        if ($withScheme) { $arguments += @('-d','tel:') }
        $started = Invoke-AdbCapture $arguments
        if ($started.ExitCode -ne 0 -or $started.Text -notmatch 'Activity: com\.appcontrol\.child/(\.calls\.DialActivity|com\.appcontrol\.child\.calls\.DialActivity)') { throw 'ACTION_DIAL did not launch the child dial pad.' }
    }
    $dump = Invoke-AdbCapture @('shell','uiautomator','dump','/sdcard/appcontrol-dialer-ui.xml')
    if ($dump.ExitCode -ne 0) { throw 'Dialer UI observation failed.' }
    $raw = Invoke-AdbCapture @('exec-out','cat','/sdcard/appcontrol-dialer-ui.xml')
    if ($raw.ExitCode -ne 0) { throw 'Dialer UI dump unavailable.' }
    [xml]$ui = $raw.Text
    $labels = @($ui.SelectNodes('//node') | ForEach-Object { $_.GetAttribute('text') })
    foreach ($label in @('App Control phone','1','9','#','Call','Delete')) { if ($labels -notcontains $label) { throw 'Dialer keypad control missing.' } }
    Write-Output 'Android accepted normal default-dialer qualification; Telecom, both ACTION_DIAL paths and empty keypad UI passed. No call placed.'
} finally {
    $restored = Invoke-AdbCapture @('shell','cmd','role','add-role-holder','--user','0','android.app.role.DIALER',$original,'0')
    $current = Invoke-AdbCapture @('shell','cmd','role','get-role-holders','--user','0','android.app.role.DIALER')
    if ($restored.ExitCode -ne 0 -or $current.ExitCode -ne 0 -or $current.Text.Trim() -ne $original) { throw 'Original emulator dialer restoration failed.' }
    Write-Output 'Original emulator dialer role restored.'
}
