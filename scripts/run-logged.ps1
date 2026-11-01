param(
    [Parameter(Mandatory)]
    [ValidatePattern('^[A-Za-z0-9][A-Za-z0-9._-]{1,63}$')]
    [string]$TaskId,
    [Parameter(Mandatory)]
    [ValidateSet('build', 'test', 'tooling', 'database', 'android', 'device', 'release', 'verification', 'other')]
    [string]$Category,
    [Parameter(Mandatory)]
    [string]$Command,
<<<<<<< HEAD
    [string]$WorkingDirectory,
    [string]$ExitCodeFile
=======
    [string]$WorkingDirectory
>>>>>>> origin/main
)

$ErrorActionPreference = 'Stop'
$workspaceRoot = Split-Path -Parent $PSScriptRoot
$logWriter = Join-Path $PSScriptRoot 'write-build-log.ps1'

if ([string]::IsNullOrWhiteSpace($WorkingDirectory)) {
    $WorkingDirectory = $workspaceRoot
} elseif (-not [IO.Path]::IsPathRooted($WorkingDirectory)) {
    $WorkingDirectory = Join-Path $workspaceRoot $WorkingDirectory
}

$resolvedWorkingDirectory = (Resolve-Path -LiteralPath $WorkingDirectory).Path
$startedAtUtc = [datetime]::UtcNow
$stopwatch = [Diagnostics.Stopwatch]::StartNew()
$outputItems = New-Object 'Collections.Generic.List[object]'
$exitCode = 0

Push-Location -LiteralPath $resolvedWorkingDirectory
try {
    $global:LASTEXITCODE = 0
    try {
        # Collect each item as it arrives. Assigning an entire pipeline loses
        # earlier output if the command throws before the assignment completes.
        # Windows PowerShell represents native stderr as ErrorRecord objects.
        # Do not terminate a process on its first stderr line. Preserve fail-fast
        # handling of PowerShell errors explicitly in the collector instead.
        $ErrorActionPreference = 'Continue'
        & ([scriptblock]::Create($Command)) 2>&1 | ForEach-Object {
            if ($_ -is [Management.Automation.ErrorRecord] -and $_.FullyQualifiedErrorId -notlike 'NativeCommandError*') {
                throw $_
            }
            $outputItems.Add($_)
        }
        if ($null -ne $LASTEXITCODE) {
            $exitCode = [int]$LASTEXITCODE
        }
    } catch {
        $outputItems.Add($_)
        $exitCode = 1
    } finally {
        $ErrorActionPreference = 'Stop'
    }
} finally {
    Pop-Location
    $stopwatch.Stop()
}

$finishedAtUtc = [datetime]::UtcNow
$outputText = ($outputItems | Out-String).TrimEnd()

$outputItems | ForEach-Object { Write-Output $_ }

$logPath = & $logWriter `
    -TaskId $TaskId `
    -Category $Category `
    -Command $Command `
    -WorkingDirectory $resolvedWorkingDirectory `
    -Output $outputText `
    -ExitCode $exitCode `
    -DurationMilliseconds $stopwatch.ElapsedMilliseconds `
    -StartedAtUtc $startedAtUtc `
    -FinishedAtUtc $finishedAtUtc

Write-Output "Build log: $logPath"
<<<<<<< HEAD
if (-not [string]::IsNullOrWhiteSpace($ExitCodeFile)) {
    if (-not [IO.Path]::IsPathRooted($ExitCodeFile)) {
        throw 'Exit-code marker path must be absolute.'
    }
    [IO.File]::WriteAllText($ExitCodeFile, $exitCode.ToString([Globalization.CultureInfo]::InvariantCulture), [Text.UTF8Encoding]::new($false))
}
exit $exitCode
=======
exit $exitCode

>>>>>>> origin/main
