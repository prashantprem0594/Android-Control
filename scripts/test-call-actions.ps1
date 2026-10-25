$ErrorActionPreference = 'Stop'
$workspaceRoot = Split-Path -Parent $PSScriptRoot
$javaRoot = 'C:\Program Files\Java\jdk-17'
$outputDirectory = Join-Path $workspaceRoot '.cache\call-action-checks'
New-Item -ItemType Directory -Force -Path $outputDirectory | Out-Null
$source = Join-Path $workspaceRoot 'android\child-app\src\main\java\com\appcontrol\child\calls\CallActionGate.java'
$checks = Join-Path $workspaceRoot 'android\child-app\src\test\java\com\appcontrol\child\calls\CallActionChecks.java'
$emergency = Join-Path $workspaceRoot 'android\child-app\src\main\java\com\appcontrol\child\calls\EmergencyCallGuard.java'
$remotePermission = Join-Path $workspaceRoot 'android\child-app\src\main\java\com\appcontrol\child\calls\RemoteCallPermission.java'
$emergencyChecks = Join-Path $workspaceRoot 'android\child-app\src\test\java\com\appcontrol\child\calls\EmergencyCallGuardChecks.java'
& (Join-Path $javaRoot 'bin\javac.exe') -d $outputDirectory $source $checks $emergency $remotePermission $emergencyChecks
if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
& (Join-Path $javaRoot 'bin\java.exe') -cp $outputDirectory com.appcontrol.child.calls.CallActionChecks
if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
& (Join-Path $javaRoot 'bin\java.exe') -cp $outputDirectory com.appcontrol.child.calls.EmergencyCallGuardChecks
exit $LASTEXITCODE
