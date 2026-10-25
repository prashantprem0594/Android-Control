param(
    [Parameter(Mandatory)] [string]$Archive,
    [Parameter(Mandatory)] [ValidatePattern('^[a-fA-F0-9]{64}$')] [string]$ExpectedSHA256
)
$ErrorActionPreference = 'Stop'
Add-Type -AssemblyName System.IO.Compression.FileSystem
Add-Type -AssemblyName System.IO.Compression
if ((Get-FileHash -LiteralPath $Archive -Algorithm SHA256).Hash.ToLowerInvariant() -cne $ExpectedSHA256.ToLowerInvariant()) { throw 'Evidence archive digest differs from the expected value.' }
$zip = [IO.Compression.ZipFile]::OpenRead($Archive)
$sha = [Security.Cryptography.SHA256]::Create()
try {
    if ($zip.Entries.Count -lt 2 -or $zip.Entries.Count -gt 512) { throw 'Unsupported evidence archive entry count.' }
    $metadataEntries = @($zip.Entries | Where-Object FullName -CEQ 'snapshot.json')
    if ($metadataEntries.Count -ne 1 -or $metadataEntries[0].Length -gt 1MB) { throw 'Evidence metadata missing or oversized.' }
    $metadataStream = $metadataEntries[0].Open()
    $metadataBuffer = [IO.MemoryStream]::new()
    try {
        $chunk = New-Object byte[] 16384
        while (($count = $metadataStream.Read($chunk, 0, $chunk.Length)) -gt 0) {
            if ($metadataBuffer.Length + $count -gt 1MB) { throw 'Evidence metadata decompression exceeds limits.' }
            $metadataBuffer.Write($chunk, 0, $count)
        }
        $metadata = [Text.Encoding]::UTF8.GetString($metadataBuffer.ToArray()) | ConvertFrom-Json
    } finally { $metadataStream.Dispose(); $metadataBuffer.Dispose() }
    if ($metadata.schemaVersion -ne 1 -or $metadata.files.Count -ne $zip.Entries.Count - 1) { throw 'Evidence inventory differs from archive entries.' }
    $records = @{}
    foreach ($record in $metadata.files) {
        if ($records.ContainsKey($record.file) -or $record.sha256 -cnotmatch '^[a-f0-9]{64}$') { throw 'Evidence inventory has invalid or duplicate records.' }
        $records[$record.file] = $record
    }
    $total = 0L
    foreach ($entry in $zip.Entries) {
        if ($entry.FullName -ceq 'snapshot.json') { continue }
        if ($entry.FullName -cnotmatch '^(README\.md|ANDROID_STUDIO_HANDOFF\.md|tracking/phase-status\.json|docs/[A-Za-z0-9_.-]+\.md|contracts/[A-Za-z0-9_.-]+\.(json|tsv)|logs/build/\d{4}-\d{2}-\d{2}(\.txt|-integrity-exceptions\.json))$') { throw 'Evidence archive contains a non-allowlisted entry.' }
        $record = $records[$entry.FullName]
        if ($null -eq $record -or $record.file -cne $entry.FullName -or $entry.Length -gt 32MB) { throw 'Evidence entry differs from inventory.' }
        $inputStream = $entry.Open()
        $sha.Initialize(); $bytesRead = 0L; $buffer = New-Object byte[] 65536
        try {
            while (($count = $inputStream.Read($buffer, 0, $buffer.Length)) -gt 0) {
                $bytesRead += $count; $total += $count
                if ($bytesRead -gt 32MB -or $total -gt 128MB) { throw 'Evidence decompression exceeds limits.' }
                $sha.TransformBlock($buffer, 0, $count, $buffer, 0) | Out-Null
            }
            $sha.TransformFinalBlock([byte[]]::new(0), 0, 0) | Out-Null
        } finally { $inputStream.Dispose() }
        $digest = ([BitConverter]::ToString($sha.Hash)).Replace('-', '').ToLowerInvariant()
        if ($digest -cne $record.sha256 -or $bytesRead -ne [long]$record.bytes -or $bytesRead -ne $entry.Length) { throw 'Archived evidence bytes or digest differ from inventory.' }
        $records.Remove($entry.FullName)
    }
    if ($records.Count -ne 0) { throw 'Evidence inventory has unmatched records.' }
    Write-Output 'Evidence archive digest, allowlisted entries and every file size/SHA-256 verified without extraction.'
} finally { $sha.Dispose(); $zip.Dispose() }
