$ErrorActionPreference = 'Stop'
$workspaceRoot = Split-Path -Parent $PSScriptRoot
$javaRoot = 'C:\Program Files\Java\jdk-17'
$outputDirectory = Join-Path $workspaceRoot '.cache\wakeup-hint-checks'
New-Item -ItemType Directory -Force -Path $outputDirectory | Out-Null
& (Join-Path $javaRoot 'bin\javac.exe') -d $outputDirectory (Join-Path $workspaceRoot 'android\child-app\src\main\java\com\appcontrol\child\wakeup\WakeupHint.java') (Join-Path $workspaceRoot 'android\child-app\src\test\java\com\appcontrol\child\wakeup\WakeupHintChecks.java')
if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
& (Join-Path $javaRoot 'bin\java.exe') -cp $outputDirectory com.appcontrol.child.wakeup.WakeupHintChecks
exit $LASTEXITCODE
