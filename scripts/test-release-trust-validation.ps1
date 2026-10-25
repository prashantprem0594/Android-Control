$ErrorActionPreference = 'Stop'
$workspaceRoot = Split-Path -Parent $PSScriptRoot
$runner = Join-Path $PSScriptRoot 'run-logged.ps1'
$androidRoot = Join-Path $workspaceRoot 'android'
$cacheLiteral = "'" + (Join-Path $workspaceRoot '.cache\gradle').Replace("'", "''") + "'"
$base = "`$env:JAVA_HOME='C:\Program Files\Java\jdk-17'; `$env:GRADLE_USER_HOME=$cacheLiteral; & 'C:\Gradle\gradle-9.6.0\bin\gradle.bat' --offline --no-daemon"
$tasks = ':parent-app:validateReleaseServerTrust :child-app:validateReleaseServerTrust'
function Invoke-Gate([string]$Command, [int]$ExpectedExit) {
    $ErrorActionPreference = 'Continue'
    $captured = @(& powershell.exe -NoProfile -ExecutionPolicy Bypass -File $runner -TaskId P6-005 -Category verification -WorkingDirectory $androidRoot -Command $Command 2>&1)
    if ($LASTEXITCODE -ne $ExpectedExit) { throw 'Unexpected release trust validation exit status.' }
    return ($captured -join "`n")
}
$negative = Invoke-Gate "$base $tasks" 1
if (-not $negative.Contains('Release builds require server origin')) { throw 'Unconfigured release trust was not explicitly refused.' }
$sha = [Security.Cryptography.SHA256]::Create()
try {
    $current = [Convert]::ToBase64String($sha.ComputeHash([Text.Encoding]::UTF8.GetBytes('public-release-gate-current')))
    $backup = [Convert]::ToBase64String($sha.ComputeHash([Text.Encoding]::UTF8.GetBytes('public-release-gate-backup')))
} finally { $sha.Dispose() }
$configured = "$base '-PappControlServerOrigin=https://gate-fixture.example.org' '-PappControlCurrentPin=$current' '-PappControlBackupPin=$backup' $tasks"
$first = Invoke-Gate $configured 0
$second = Invoke-Gate $configured 0
if (-not $second.Contains('Configuration cache entry reused.')) { throw 'Configured trust gate did not reuse the configuration cache.' }
Write-Output 'Both release trust gates pass with complete fixture inputs and cached configuration; unconfigured release is refused.'
