param(
    [ValidateSet('127.0.0.1:5581','emulator-5580')] [string]$Serial='127.0.0.1:5581',
    [ValidateSet(5038)] [int]$AdbServerPort=5038
)
# Existing disposable acceptance APKs only. Never installs, grants permissions, clears data or resets a device.
$ErrorActionPreference='Stop'
$workspace=Split-Path -Parent $PSScriptRoot
$adb=Join-Path $workspace '.tooling\android-sdk\platform-tools\adb.exe'
$command=@"
`$adb='$($adb.Replace("'","''"))'
`$fingerprint=@(& `$adb -P $AdbServerPort -s '$Serial' shell getprop ro.build.fingerprint)
if(`$LASTEXITCODE -ne 0 -or (`$fingerprint -join '').Trim() -notlike 'google/sdk_gphone64_x86_64/emu64xa:*'){throw 'Isolated Google acceptance emulator required; physical devices are refused.'}
`$package=@(& `$adb -P $AdbServerPort -s '$Serial' shell dumpsys package com.appcontrol.parent.acceptance)
if(`$LASTEXITCODE -ne 0 -or -not (`$package -match 'pkgFlags=.*DEBUGGABLE')){throw 'Existing disposable debug parent acceptance package required.'}
`$permissions=`$package -join "``n"
if(`$permissions -notmatch 'android\.permission\.POST_NOTIFICATIONS: granted=true'){throw 'Parent acceptance fixture requires notification permission granted by its explicit setup flow.'}
`$proof=@(& `$adb -P $AdbServerPort -s '$Serial' shell am instrument -w -r -e probe parent_push_queue -e phase run com.appcontrol.parent.acceptance.test/com.appcontrol.transport.ServerTrustInstrumentation 2>&1)
`$code=`$LASTEXITCODE
`$proof | Write-Output
`$text=`$proof -join "``n"
if(`$code -ne 0 -or `$text -notmatch '(?m)^INSTRUMENTATION_RESULT: parent_push_queue_probe=passed\r?$' -or `$text -notmatch '(?m)^INSTRUMENTATION_RESULT: parent_push_queue_cleanup=passed\r?$' -or `$text -notmatch '(?m)^INSTRUMENTATION_CODE: -1\r?$'){throw 'Parent push framework queue or cleanup proof missing or failed.'}
Write-Output 'Parent push Android framework queue and fixture cleanup passed; no Firebase delivery or physical-device result is claimed.'
"@
& (Join-Path $PSScriptRoot 'run-logged.ps1') -TaskId PARENT-PUSH-QUEUE -Category device -WorkingDirectory $workspace -Command $command
exit $LASTEXITCODE
