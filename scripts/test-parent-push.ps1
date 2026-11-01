# Invoke through run-logged.ps1. Local JVM checks; no Firebase or Android runtime.
$ErrorActionPreference = 'Stop'
$workspace = Split-Path -Parent $PSScriptRoot
$java = 'C:\Program Files\Java\jdk-17\bin'
$output = Join-Path $workspace ('.cache\parent-push-' + [guid]::NewGuid().ToString('N'))
$null = New-Item -ItemType Directory -Path $output
$sources = @('ParentPushEventRules','ParentPushHint','ParentPushLedger','ParentPushQueueState') | ForEach-Object {
    Join-Path $workspace ("android\parent-app\src\main\java\com\appcontrol\parent\push\$_.java")
}
$checks = @('ParentPushChecks','ParentPushHintChecks','ParentPushQueueChecks')
$sources += $checks | ForEach-Object {
    Join-Path $workspace ("android\parent-app\src\test\java\com\appcontrol\parent\push\$_.java")
}
& (Join-Path $java 'javac.exe') -d $output @sources
if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
foreach ($check in $checks) {
    & (Join-Path $java 'java.exe') -cp $output "com.appcontrol.parent.push.$check"
    if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
}
exit 0
