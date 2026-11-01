$ErrorActionPreference = 'Stop'
$workspaceRoot = Split-Path -Parent $PSScriptRoot
$runner = Join-Path $PSScriptRoot 'run-logged.ps1'

$goCommand = '$env:GOPROXY="off"; $env:GOTOOLCHAIN="local"; $env:GOCACHE="C:\Android Control\.cache\go-build"; $env:GOMODCACHE="C:\Android Control\.cache\go-mod"; & "C:\Program Files\Go\bin\go.exe" test -count=1 ./...'
& $runner -TaskId SENSITIVE-DATA-001 -Category test -WorkingDirectory (Join-Path $workspaceRoot 'server') -Command $goCommand
if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }

$javaCommand = '$output = ".cache\call-notification-privacy-checks"; New-Item -ItemType Directory -Force -Path $output | Out-Null; & "C:\Program Files\Java\jdk-17\bin\javac.exe" -d $output "android\child-app\src\main\java\com\appcontrol\child\calls\CallNotificationPrivacy.java" "android\child-app\src\test\java\com\appcontrol\child\calls\CallNotificationPrivacyChecks.java"; if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }; & "C:\Program Files\Java\jdk-17\bin\java.exe" -cp $output com.appcontrol.child.calls.CallNotificationPrivacyChecks'
& $runner -TaskId SENSITIVE-DATA-002 -Category test -WorkingDirectory $workspaceRoot -Command $javaCommand
if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }

$fixtureChecks = @(
    @{ TaskId = 'SENSITIVE-DATA-CALLS'; Script = 'test-call-actions.ps1' },
    @{ TaskId = 'SENSITIVE-DATA-PARENT'; Script = 'test-parent-call-watch.ps1' },
    @{ TaskId = 'SENSITIVE-DATA-PUSH'; Script = 'test-parent-push.ps1' },
    @{ TaskId = 'SENSITIVE-DATA-WAKE'; Script = 'test-wakeup-hints.ps1' }
)
foreach ($check in $fixtureChecks) {
    $command = '& ".\scripts\' + $check.Script + '"'
    & $runner -TaskId $check.TaskId -Category test -WorkingDirectory $workspaceRoot -Command $command
    if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
}

$gradleCommand = '$env:JAVA_HOME="C:\Program Files\Java\jdk-17"; $env:GRADLE_USER_HOME="C:\Android Control\.cache\gradle"; & "C:\Gradle\gradle-9.6.0\bin\gradle.bat" --offline --no-daemon "-Pkotlin.compiler.execution.strategy=in-process" :child-app:compileDebugKotlin :child-app:lintDebug'
& $runner -TaskId SENSITIVE-DATA-003 -Category test -WorkingDirectory (Join-Path $workspaceRoot 'android') -Command $gradleCommand
exit $LASTEXITCODE
