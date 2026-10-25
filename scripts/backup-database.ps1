param(
    [Parameter(Mandatory)] [string]$DatabasePath,
    [Parameter(Mandatory)] [string]$BackupDirectory,
    [Parameter(Mandatory)] [string]$KeyFile,
    [Parameter(Mandatory)] [string]$ScratchDirectory,
    [string]$AdminExecutable = '',
    [ValidateRange(1, 3650)] [int]$MaxBackups = 30,
    [ValidateRange(1, 60)] [int]$TimeoutMinutes = 2
)

$ErrorActionPreference = 'Stop'
$workspaceRoot = Split-Path -Parent $PSScriptRoot
if ([string]::IsNullOrWhiteSpace($AdminExecutable)) {
    $AdminExecutable = Join-Path $PSScriptRoot '..\.cache\control-admin.exe'
}

# All interpolated arguments are single-quoted PowerShell literals. Neither
# dollar expressions nor backticks in paths are evaluated by the logger.
function ConvertTo-ShellLiteral([string]$Value) {
    return "'" + $Value.Replace("'", "''") + "'"
}

try {
    $databaseFull = [IO.Path]::GetFullPath($DatabasePath)
    $backupFull = [IO.Path]::GetFullPath($BackupDirectory)
    $keyFull = [IO.Path]::GetFullPath($KeyFile)
    $scratchFull = [IO.Path]::GetFullPath($ScratchDirectory)
    $adminFull = [IO.Path]::GetFullPath($AdminExecutable)
    $snapshotName = 'app-control-' + [DateTime]::UtcNow.ToString('yyyyMMddTHHmmssfffffffZ') + '-' + [Guid]::NewGuid().ToString('N') + '.acb'
    $snapshotFull = Join-Path $backupFull $snapshotName

    $quotedBackup = ConvertTo-ShellLiteral $backupFull
    $quotedLock = ConvertTo-ShellLiteral (Join-Path $backupFull '.app-control-backup.lock')
    $quotedSnapshot = ConvertTo-ShellLiteral $snapshotFull
    $quotedAdmin = ConvertTo-ShellLiteral $adminFull
    $quotedDatabase = ConvertTo-ShellLiteral $databaseFull
    $quotedKey = ConvertTo-ShellLiteral $keyFull
    $quotedScratch = ConvertTo-ShellLiteral $scratchFull
    $timeout = $TimeoutMinutes.ToString() + 'm'

    $command = @"
`$ErrorActionPreference = 'Stop'
if (-not (Test-Path -LiteralPath $quotedAdmin -PathType Leaf)) { throw 'Administration executable is missing.' }
if (-not (Test-Path -LiteralPath $quotedDatabase -PathType Leaf)) { throw 'Existing database is required.' }
if (-not (Test-Path -LiteralPath $quotedKey -PathType Leaf)) { throw 'Backup key file is missing.' }
New-Item -ItemType Directory -Force -Path $quotedBackup | Out-Null
`$backupLock = [IO.File]::Open($quotedLock, [IO.FileMode]::OpenOrCreate, [IO.FileAccess]::ReadWrite, [IO.FileShare]::None)
try {
`$existingBackups = @(Get-ChildItem -LiteralPath $quotedBackup -File | Where-Object { `$_.Name -match '^app-control-[0-9]{8}T[0-9]{13}Z-[a-f0-9]{32}[.]acb$' })
if (`$existingBackups.Count -ge $MaxBackups) { throw 'Backup count limit reached. Archive verified snapshots before retrying.' }
& $quotedAdmin backup -db $quotedDatabase -backup $quotedSnapshot -key-file $quotedKey -timeout '$timeout'
if (`$LASTEXITCODE -ne 0) { throw 'Backup creation failed.' }
& $quotedAdmin verify -backup $quotedSnapshot -key-file $quotedKey -scratch-dir $quotedScratch -timeout '$timeout'
if (`$LASTEXITCODE -ne 0) { throw 'Backup verification failed.' }
Write-Output 'Scheduled backup and independent verification completed.'
} finally {
    `$backupLock.Dispose()
}
"@
} catch {
    & (Join-Path $PSScriptRoot 'write-build-log.ps1') -TaskId P6-005 -Category tooling `
        -Command 'Prepare scheduled backup job' -WorkingDirectory $workspaceRoot `
        -Output 'Backup job arguments could not be resolved.' -ExitCode 1 -DurationMilliseconds 0
    exit 1
}

# run-logged exits with the child result. Task Scheduler can use this directly.
& (Join-Path $PSScriptRoot 'run-logged.ps1') -TaskId P6-005 -Category tooling `
    -WorkingDirectory $workspaceRoot -Command $command
exit $LASTEXITCODE
