param(
    [Parameter(Mandatory)] [string]$BundleDirectory
)

$ErrorActionPreference = 'Stop'
$bundleItem = Get-Item -LiteralPath $BundleDirectory -ErrorAction Stop
if (-not $bundleItem.PSIsContainer) { throw 'Bundle path must be a directory.' }
$ancestor = $bundleItem
while ($null -ne $ancestor) {
    if (($ancestor.Attributes -band [IO.FileAttributes]::ReparsePoint) -ne 0) {
        throw 'Reparse points are not allowed in the bundle root or its ancestors.'
    }
    $ancestor = $ancestor.Parent
}
$bundleRoot = [IO.Path]::GetFullPath($bundleItem.FullName).TrimEnd([IO.Path]::DirectorySeparatorChar, [IO.Path]::AltDirectorySeparatorChar)
$bundlePrefix = $bundleRoot + [IO.Path]::DirectorySeparatorChar
$pathComparer = [StringComparer]::OrdinalIgnoreCase
$seenPaths = [Collections.Generic.HashSet[string]]::new($pathComparer)

function Assert-RelativePath([string]$RelativePath) {
    if ([string]::IsNullOrWhiteSpace($RelativePath) -or $RelativePath.Contains('\') -or $RelativePath.Contains(':') -or $RelativePath.StartsWith('/')) {
        throw "Invalid bundle-relative path: $RelativePath"
    }
    $segments = $RelativePath.Split('/')
    if (@($segments | Where-Object { $_ -eq '' -or $_ -eq '.' -or $_ -eq '..' }).Count -gt 0) {
        throw "Invalid bundle-relative path: $RelativePath"
    }
    foreach ($segment in $segments) {
        if ($segment -match '[. ]$|[<>"|?*\x00-\x1f]' -or
            $segment -match '^(?i:con|prn|aux|nul|com[1-9]|lpt[1-9]|conin\$|conout\$)(?:\.|$)') {
            throw "Noncanonical or reserved Windows path in bundle: $RelativePath"
        }
    }
}

function Get-RequiredFile([string]$RelativePath) {
    Assert-RelativePath $RelativePath
    $segments = $RelativePath.Split('/')
    $fullPath = [IO.Path]::GetFullPath((Join-Path $bundleRoot ($RelativePath.Replace('/', [IO.Path]::DirectorySeparatorChar))))
    if (-not $fullPath.StartsWith($bundlePrefix, [StringComparison]::OrdinalIgnoreCase)) {
        throw "Path escapes bundle directory: $RelativePath"
    }
    $currentPath = $bundleRoot
    foreach ($segment in $segments) {
        $currentPath = Join-Path $currentPath $segment
        $item = Get-Item -LiteralPath $currentPath -ErrorAction Stop
        if (($item.Attributes -band [IO.FileAttributes]::ReparsePoint) -ne 0) {
            throw "Reparse points are not allowed in bundle paths: $RelativePath"
        }
    }
    if ($item.PSIsContainer) { throw "Expected a file in bundle: $RelativePath" }
    return $item
}

function Assert-UniqueBundlePath([string]$RelativePath) {
    if (-not $seenPaths.Add($RelativePath)) {
        throw "Duplicate bundle path: $RelativePath"
    }
}

function Assert-Sha256([string]$Hash, [string]$Description) {
    if ($Hash -notmatch '^[0-9a-fA-F]{64}$') { throw "Invalid SHA-256 value for $Description." }
}

function Assert-FileHash([string]$RelativePath, [string]$ExpectedHash) {
    Assert-Sha256 $ExpectedHash $RelativePath
    $file = Get-RequiredFile $RelativePath
    $actualHash = (Get-FileHash -LiteralPath $file.FullName -Algorithm SHA256).Hash
    if (-not [StringComparer]::OrdinalIgnoreCase.Equals($actualHash, $ExpectedHash)) {
        throw "SHA-256 mismatch: $RelativePath"
    }
    return $file
}

$manifestFile = Get-RequiredFile 'bundle.json'
foreach ($metadataPath in @('bundle.json','source-inventory.json','modules.txt')) { Assert-UniqueBundlePath $metadataPath }
try {
    $manifest = [IO.File]::ReadAllText($manifestFile.FullName) | ConvertFrom-Json -ErrorAction Stop
} catch {
    throw "Bundle manifest is not valid JSON: $($_.Exception.Message)"
}
if ($manifest.schemaVersion -ne 1) { throw 'Unsupported or missing bundle manifest schemaVersion.' }

$sourceInventory = Assert-FileHash 'source-inventory.json' $manifest.sourceInventorySHA256
$modulesFile = Assert-FileHash 'modules.txt' $manifest.modulesSHA256

try {
    $sourceRows = ConvertFrom-Json -InputObject ([IO.File]::ReadAllText($sourceInventory.FullName)) -ErrorAction Stop
} catch {
    throw "Source inventory is not valid JSON: $($_.Exception.Message)"
}
$sourcePaths = [Collections.Generic.HashSet[string]]::new($pathComparer)
if ($sourceRows.Count -eq 0) { throw 'Source inventory contains no entries.' }
foreach ($row in $sourceRows) {
    if ($null -eq $row -or [string]::IsNullOrWhiteSpace([string]$row.path)) { throw 'Source inventory contains an invalid entry.' }
    $sourcePath = [string]$row.path
    Assert-RelativePath $sourcePath
    if (-not $sourcePaths.Add($sourcePath)) { throw "Duplicate source inventory path: $sourcePath" }
    Assert-Sha256 ([string]$row.sha256) $sourcePath
}

$moduleLines = @([IO.File]::ReadAllLines($modulesFile.FullName) | Where-Object { -not [string]::IsNullOrWhiteSpace($_) })
if ($moduleLines.Count -eq 0) { throw 'Module inventory contains no entries.' }
$moduleSet = [Collections.Generic.HashSet[string]]::new([StringComparer]::Ordinal)
foreach ($moduleLine in $moduleLines) {
    if (-not $moduleSet.Add($moduleLine)) { throw "Duplicate module entry: $moduleLine" }
}

$artifacts = @($manifest.artifacts)
if ($artifacts.Count -eq 0) { throw 'Bundle manifest contains no artifacts.' }
foreach ($artifact in $artifacts) {
    if ($null -eq $artifact -or [string]::IsNullOrWhiteSpace([string]$artifact.path)) {
        throw 'Bundle manifest contains an invalid artifact entry.'
    }
    $relativePath = [string]$artifact.path
    Assert-UniqueBundlePath $relativePath
    $sizeText = [string]$artifact.size
    $expectedSize = 0L
    if ($sizeText -notmatch '^(0|[1-9][0-9]*)$' -or
        -not [long]::TryParse($sizeText, [Globalization.NumberStyles]::None, [Globalization.CultureInfo]::InvariantCulture, [ref]$expectedSize)) {
        throw "Invalid artifact size: $relativePath"
    }
    $file = Assert-FileHash $relativePath ([string]$artifact.sha256)
    if ([long]$file.Length -ne $expectedSize) { throw "Artifact size mismatch: $relativePath" }
}

$licenseNotices = @($manifest.licenseNotices)
if ($licenseNotices.Count -eq 0) { throw 'Bundle manifest contains no license notices.' }
foreach ($notice in $licenseNotices) {
    if ($null -eq $notice -or [string]::IsNullOrWhiteSpace([string]$notice.path)) {
        throw 'Bundle manifest contains an invalid license notice entry.'
    }
    $relativePath = [string]$notice.path
    Assert-UniqueBundlePath $relativePath
    $null = Assert-FileHash $relativePath ([string]$notice.sha256)
}

Write-Output "Verified server bundle: $($artifacts.Count) artifacts, $($sourceRows.Count) source inventory entries, $($moduleLines.Count) modules, $($licenseNotices.Count) license notices."
