$ErrorActionPreference = 'Stop'
$workspaceRoot = Split-Path -Parent $PSScriptRoot
$javaRoot = 'C:\Program Files\Java\jdk-17'
$outputDirectory = Join-Path $workspaceRoot '.cache\package-policy-checks'
New-Item -ItemType Directory -Force -Path $outputDirectory | Out-Null
$source = Join-Path $workspaceRoot 'android\child-app\src\main\java\com\appcontrol\child\policy\PackagePolicyEngine.java'
$checks = Join-Path $workspaceRoot 'android\child-app\src\test\java\com\appcontrol\child\policy\PackagePolicyChecks.java'
$retrySource = Join-Path $workspaceRoot 'android\child-app\src\main\java\com\appcontrol\child\policy\PolicyRetry.java'
$internetSource = Join-Path $workspaceRoot 'android\child-app\src\main\java\com\appcontrol\child\policy\InternetPausePlan.java'
$internetChecks = Join-Path $workspaceRoot 'android\child-app\src\test\java\com\appcontrol\child\policy\InternetPauseChecks.java'
$scheduleSource = Join-Path $workspaceRoot 'android\child-app\src\main\java\com\appcontrol\child\policy\ScheduledPolicy.java'
$scheduleChecks = Join-Path $workspaceRoot 'android\child-app\src\test\java\com\appcontrol\child\policy\ScheduledPolicyChecks.java'
& (Join-Path $javaRoot 'bin\javac.exe') -d $outputDirectory $source $checks $retrySource $internetSource $internetChecks $scheduleSource $scheduleChecks
if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
& (Join-Path $javaRoot 'bin\java.exe') -cp $outputDirectory com.appcontrol.child.policy.PackagePolicyChecks
if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
& (Join-Path $javaRoot 'bin\java.exe') -cp $outputDirectory com.appcontrol.child.policy.InternetPauseChecks
if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
& (Join-Path $javaRoot 'bin\java.exe') -cp $outputDirectory com.appcontrol.child.policy.ScheduledPolicyChecks (Join-Path $workspaceRoot 'contracts\schedule-vectors.tsv')
exit $LASTEXITCODE
