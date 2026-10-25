$ErrorActionPreference = 'Stop'
$workspaceRoot = Split-Path -Parent $PSScriptRoot
$javaRoot = 'C:\Program Files\Java\jdk-17'
$outputDirectory = Join-Path $workspaceRoot '.cache\server-origin-checks'
New-Item -ItemType Directory -Force -Path $outputDirectory | Out-Null
$source = Join-Path $workspaceRoot 'android\shared\src\main\java\com\appcontrol\transport\ServerOrigin.java'
$checks = Join-Path $workspaceRoot 'android\shared\src\test\java\com\appcontrol\transport\ServerOriginChecks.java'
& (Join-Path $javaRoot 'bin\javac.exe') -d $outputDirectory $source $checks
if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
& (Join-Path $javaRoot 'bin\java.exe') -cp $outputDirectory com.appcontrol.transport.ServerOriginChecks
exit $LASTEXITCODE
