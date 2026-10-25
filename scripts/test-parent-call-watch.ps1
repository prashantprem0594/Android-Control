# Invoke through run-logged.ps1.
$ErrorActionPreference = 'Stop'
$workspace = Split-Path -Parent $PSScriptRoot
$java = 'C:\Program Files\Java\jdk-17\bin'
$output = Join-Path $workspace ('.cache\parent-call-watch-' + [guid]::NewGuid().ToString('N'))
$null = New-Item -ItemType Directory -Path $output
$source = Join-Path $workspace 'android\parent-app\src\main\java\com\appcontrol\parent\CallWatchState.java'
$checks = Join-Path $workspace 'android\parent-app\src\test\java\com\appcontrol\parent\CallWatchChecks.java'
& (Join-Path $java 'javac.exe') -d $output $source $checks
if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
& (Join-Path $java 'java.exe') -cp $output com.appcontrol.parent.CallWatchChecks
exit $LASTEXITCODE
