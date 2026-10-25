param([string]$OutputFile = '')
$ErrorActionPreference = 'Stop'
$workspaceRoot = Split-Path -Parent $PSScriptRoot
Add-Type -AssemblyName System.IO.Compression.FileSystem
Add-Type -AssemblyName System.IO.Compression
if ([string]::IsNullOrWhiteSpace($OutputFile)) {
    $OutputFile = Join-Path $workspaceRoot ('.cache\evidence-' + [DateTime]::UtcNow.ToString('yyyyMMddTHHmmss') + '-' + [Guid]::NewGuid().ToString('N') + '.zip')
}
if (Test-Path -LiteralPath $OutputFile) { throw 'Evidence output must not exist.' }
$files = New-Object 'Collections.Generic.List[IO.FileInfo]'
foreach ($name in @('README.md', 'ANDROID_STUDIO_HANDOFF.md', 'tracking\phase-status.json')) {
    $files.Add((Get-Item -LiteralPath (Join-Path $workspaceRoot $name)))
}
foreach ($directory in @('docs', 'contracts')) {
    foreach ($file in Get-ChildItem -LiteralPath (Join-Path $workspaceRoot $directory) -File) {
        if ($file.Extension -in @('.md', '.json', '.tsv') -and ($file.Attributes -band [IO.FileAttributes]::ReparsePoint) -eq 0) { $files.Add($file) }
    }
}
$logs = @(Get-ChildItem -LiteralPath (Join-Path $workspaceRoot 'logs\build') -File | Where-Object { $_.Name -match '^\d{4}-\d{2}-\d{2}\.txt$' })
if ($logs.Count -eq 0) { throw 'No dated build logs to archive.' }
foreach ($log in $logs) {
    & (Join-Path $PSScriptRoot 'verify-build-log.ps1') -Date $log.BaseName
    $files.Add($log)
    $exceptions = Join-Path $log.DirectoryName ($log.BaseName + '-integrity-exceptions.json')
    if (Test-Path -LiteralPath $exceptions) { $files.Add((Get-Item -LiteralPath $exceptions)) }
}
$destination = [IO.Path]::GetFullPath($OutputFile)
[IO.Directory]::CreateDirectory([IO.Path]::GetDirectoryName($destination)) | Out-Null
$stream = [IO.File]::Open($destination, [IO.FileMode]::CreateNew, [IO.FileAccess]::Write, [IO.FileShare]::None)
$archive = $null
$sha = [Security.Cryptography.SHA256]::Create()
try {
    $archive = [IO.Compression.ZipArchive]::new($stream, [IO.Compression.ZipArchiveMode]::Create, $true)
    $records = @()
    foreach ($file in $files | Sort-Object FullName -Unique) {
        if (($file.Attributes -band [IO.FileAttributes]::ReparsePoint) -ne 0 -or $file.Length -gt 32MB) { throw 'Evidence file is unsupported or exceeds the size limit.' }
        $relative = $file.FullName.Substring($workspaceRoot.Length + 1).Replace('\', '/')
        $bytes = [IO.File]::ReadAllBytes($file.FullName)
        # Hash and archive the same byte snapshot, including the current log cutoff.
        $digest = ([BitConverter]::ToString($sha.ComputeHash($bytes))).Replace('-', '').ToLowerInvariant()
        $entry = $archive.CreateEntry($relative, [IO.Compression.CompressionLevel]::Optimal)
        $entryStream = $entry.Open()
        try { $entryStream.Write($bytes, 0, $bytes.Length) } finally { $entryStream.Dispose() }
        $records += [ordered]@{file=$relative;bytes=$bytes.Length;sha256=$digest}
    }
    $metadata = [ordered]@{
        schemaVersion=1
        createdAtUtc=[DateTime]::UtcNow.ToString('o')
        scope='Sanitized dated logs, documented integrity exceptions, contracts, progress and handoff/docs. Excludes credentials, databases, keys, APKs and source/build caches. The archiving command entry is written after this snapshot.'
        files=$records
    } | ConvertTo-Json -Depth 6
    $entry = $archive.CreateEntry('snapshot.json')
    $entryStream = $entry.Open()
    try { $bytes=[Text.Encoding]::UTF8.GetBytes($metadata); $entryStream.Write($bytes,0,$bytes.Length) } finally { $entryStream.Dispose() }
} finally {
    if ($null -ne $archive) { $archive.Dispose() }
    $stream.Dispose(); $sha.Dispose()
}
$archiveHash = (Get-FileHash -LiteralPath $destination -Algorithm SHA256).Hash.ToLowerInvariant()
Write-Output "Evidence snapshot created: $destination"
Write-Output "Snapshot SHA-256: $archiveHash"
Write-Output 'Local snapshot only; off-host archival and production release evidence remain separate.'
