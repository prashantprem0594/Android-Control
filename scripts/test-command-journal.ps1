$ErrorActionPreference = 'Stop'
$workspaceRoot = Split-Path -Parent $PSScriptRoot
$javaRoot = 'C:\Program Files\Java\jdk-17'
$outputDirectory = Join-Path $workspaceRoot '.cache\command-journal-checks'
New-Item -ItemType Directory -Force -Path $outputDirectory | Out-Null
$source = Join-Path $workspaceRoot 'android\child-app\src\main\java\com\appcontrol\child\commands\CommandExecutor.java'
$checks = Join-Path $workspaceRoot 'android\child-app\src\test\java\com\appcontrol\child\commands\CommandExecutorChecks.java'
$stages = Join-Path $workspaceRoot 'android\child-app\src\main\java\com\appcontrol\child\commands\SyncStages.java'
$recoveryChecks = Join-Path $workspaceRoot 'android\child-app\src\test\java\com\appcontrol\child\commands\SyncStagesChecks.java'
$requestControl = Join-Path $workspaceRoot 'android\child-app\src\main\java\com\appcontrol\child\commands\RequestControl.java'
$requestDeadline = Join-Path $workspaceRoot 'android\shared\src\main\java\com\appcontrol\transport\RequestDeadline.java'
$requestChecks = Join-Path $workspaceRoot 'android\child-app\src\test\java\com\appcontrol\child\commands\RequestControlChecks.java'
$transportChecks = Join-Path $workspaceRoot 'android\child-app\src\test\java\com\appcontrol\child\commands\StalledTransportChecks.java'
$workQueue = Join-Path $workspaceRoot 'android\child-app\src\main\java\com\appcontrol\child\commands\SyncWorkQueue.java'
$queueChecks = Join-Path $workspaceRoot 'android\child-app\src\test\java\com\appcontrol\child\commands\SyncWorkQueueChecks.java'
& (Join-Path $javaRoot 'bin\javac.exe') -d $outputDirectory $source $checks $stages $recoveryChecks $requestDeadline $requestControl $requestChecks $transportChecks $workQueue $queueChecks
if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
& (Join-Path $javaRoot 'bin\java.exe') -cp $outputDirectory com.appcontrol.child.commands.CommandExecutorChecks
if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
& (Join-Path $javaRoot 'bin\java.exe') -cp $outputDirectory com.appcontrol.child.commands.SyncStagesChecks
if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
& (Join-Path $javaRoot 'bin\java.exe') -cp $outputDirectory com.appcontrol.child.commands.RequestControlChecks
if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
& (Join-Path $javaRoot 'bin\java.exe') -cp $outputDirectory com.appcontrol.child.commands.StalledTransportChecks
if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
& (Join-Path $javaRoot 'bin\java.exe') -cp $outputDirectory com.appcontrol.child.commands.SyncWorkQueueChecks
exit $LASTEXITCODE
