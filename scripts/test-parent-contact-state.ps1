$ErrorActionPreference = 'Stop'
$workspaceRoot = Split-Path -Parent $PSScriptRoot
$javaRoot = 'C:\Program Files\Java\jdk-17'
$outputDirectory = Join-Path $workspaceRoot '.cache\parent-contact-checks'
New-Item -ItemType Directory -Force -Path $outputDirectory | Out-Null
& (Join-Path $javaRoot 'bin\javac.exe') -d $outputDirectory (Join-Path $workspaceRoot 'android\parent-app\src\main\java\com\appcontrol\parent\ContactFreshness.java') (Join-Path $workspaceRoot 'android\parent-app\src\test\java\com\appcontrol\parent\ContactFreshnessChecks.java')
if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
& (Join-Path $javaRoot 'bin\java.exe') -cp $outputDirectory com.appcontrol.parent.ContactFreshnessChecks
exit $LASTEXITCODE
