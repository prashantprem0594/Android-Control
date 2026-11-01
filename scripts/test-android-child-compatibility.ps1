param(
    [Parameter(Mandatory)] [ValidatePattern('^[A-Za-z0-9._:-]+$')] [string]$Serial,
    [ValidateRange(1024,65535)] [int]$AdbServerPort = 5037
)
$ErrorActionPreference = 'Stop'
$workspace = Split-Path -Parent $PSScriptRoot
$adb = Join-Path $workspace '.tooling\android-sdk\platform-tools\adb.exe'
$body = @'
$ErrorActionPreference='Stop'
function Device([string[]]$Arguments) {
    $output=@(& $adb -P $port -s $serial @Arguments 2>&1)
    if($LASTEXITCODE -ne 0){throw 'Compatibility device operation failed.'}
    return ($output -join "`n")
}
if((Device @('get-state')).Trim() -ne 'device'){throw 'Device is not connected and authorized.'}
$api=[int](Device @('shell','getprop','ro.build.version.sdk')).Trim()
if($api -lt 28){throw 'Child requires Android 9/API28 or later.'}
$proof=Device @('shell','am','instrument','-w','-r','-e','probe','android_compatibility','com.appcontrol.child.test/com.appcontrol.transport.ServerTrustInstrumentation')
$proof | Write-Output
if($proof -notmatch '(?m)^INSTRUMENTATION_RESULT: android_compatibility_probe=passed\r?$' -or $proof -notmatch '(?m)^INSTRUMENTATION_CODE: -1\r?$') {throw 'Child compatibility probe did not pass.'}
$launch=Device @('shell','am','start','-W','-n','com.appcontrol.child/.MainActivity')
$launch | Write-Output
if($launch -notmatch '(?m)^Status: ok\r?$'){throw 'Child activity launch did not succeed.'}
$foreground=Device @('shell','dumpsys','activity','activities')
if($foreground -notmatch 'mResumedActivity:[^\r\n]*com\.appcontrol\.child/\.MainActivity'){throw 'Child main activity is not resumed; unlock the phone and retry.'}
$childProcessId=(Device @('shell','pidof','com.appcontrol.child')).Trim()
if($childProcessId -notmatch '^[0-9]+$'){throw 'Child app process is not running.'}
Write-Output 'android_compatibility_main_screen=passed (am start status, resumed activity and running PID)'
Write-Output 'Capability/default-dialer-intent/main-screen smoke check only; no provisioning, policy changes or calls.'
'@
$command = '$adb='''+$adb.Replace("'","''")+''';$serial='''+$Serial+''';$port='+$AdbServerPort+"`n"+$body
& (Join-Path $PSScriptRoot 'run-logged.ps1') -TaskId ANDROID9-DEVICE -Category device -WorkingDirectory $workspace -Command $command
exit $LASTEXITCODE
