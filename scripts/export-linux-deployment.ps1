param(
    [Parameter(Mandatory)] [string]$BundleDirectory,
    [Parameter(Mandatory)] [ValidatePattern('^[a-fA-F0-9]{64}$')] [string]$ExpectedBundleSHA256,
    [Parameter(Mandatory)] [ValidateSet('linux-amd64','linux-arm64')] [string]$Target,
    [Parameter(Mandatory)] [string]$OutputDirectory
)
# Offline export only: never executes tools, installs services, or reads private configuration.
$ErrorActionPreference = 'Stop'
$workspace = Split-Path -Parent $PSScriptRoot
$comparison = [StringComparison]::OrdinalIgnoreCase
$copies = [Collections.Generic.List[object]]::new()
$destinations = [Collections.Generic.HashSet[string]]::new([StringComparer]::OrdinalIgnoreCase)

function Canonical-Path([string]$Value) {
    if ([string]::IsNullOrWhiteSpace($Value) -or $Value.StartsWith('\\') -or $Value -match '[\x00-\x1f]') { throw 'A canonical local path is required.' }
    foreach ($part in $Value.Split([char[]]'\/')) {
        if ($part -eq '..' -or ($part -ne '.' -and $part -match '[. ]$') -or
            $part -match '^(?i:con|prn|aux|nul|com[1-9]|lpt[1-9]|conin\$|conout\$)(?:\.|$)') { throw 'Traversal or aliased paths are refused.' }
        if ($part.Contains(':') -and $part -notmatch '^[A-Za-z]:$') { throw 'Alternate path streams are refused.' }
    }
    $full = [IO.Path]::GetFullPath($Value)
    if ($full.Equals([IO.Path]::GetPathRoot($full),$comparison)) { return $full }
    return $full.TrimEnd([char[]]'\/')
}
function No-Reparse([string]$Path) {
    $item = Get-Item -LiteralPath $Path -Force -ErrorAction Stop
    $current = $item
    while ($null -ne $current) {
        if (($current.Attributes -band [IO.FileAttributes]::ReparsePoint) -ne 0) { throw 'Symlinks and junctions are refused.' }
        if ($current.PSIsContainer) { $current = $current.Parent } else { $current = $current.Directory }
    }
    return $item
}
function Hash([string]$Path) {
    $item = No-Reparse $Path
    if ($item.PSIsContainer) { throw 'Expected a regular source file.' }
    return (Get-FileHash -LiteralPath $item.FullName -Algorithm SHA256).Hash.ToLowerInvariant()
}
function Within([string]$Path,[string]$Directory) {
    return $Path.Equals($Directory,$comparison) -or $Path.StartsWith($Directory.TrimEnd([char[]]'\/')+[IO.Path]::DirectorySeparatorChar,$comparison)
}
function Fields($Value,[string[]]$Allowed) {
    foreach ($property in $Value.PSObject.Properties.Name) {
        if ($property -cnotin $Allowed) { throw 'Unknown source metadata field; export refused.' }
    }
}
function Add-Copy([string]$Source,[string]$Relative,[string]$ExpectedHash) {
    if ($Relative -match '\\|:|^/|(^|/)\.\.?(/|$)' -or -not $destinations.Add($Relative)) { throw 'Invalid or duplicate deployment destination.' }
    $item = No-Reparse $Source
    if ($item.PSIsContainer -or $ExpectedHash -notmatch '^[a-f0-9]{64}$' -or (Hash $item.FullName) -cne $ExpectedHash) { throw 'Source file identity changed or is invalid.' }
    $copies.Add([pscustomobject]@{source=$item.FullName;path=$Relative;size=[long]$item.Length;sha256=$ExpectedHash})
}
function Linux-Header([string]$Path) {
    $stream = [IO.File]::OpenRead($Path)
    try { $header = New-Object byte[] 20; $count = $stream.Read($header,0,20) } finally { $stream.Dispose() }
    $machine = if ($Target -eq 'linux-amd64') { 62 } else { 183 }
    if ($count -ne 20 -or $header[0] -ne 127 -or $header[1] -ne 69 -or $header[2] -ne 76 -or $header[3] -ne 70 -or
        $header[4] -ne 2 -or $header[5] -ne 1 -or $header[6] -ne 1 -or $header[16] -notin @(2,3) -or
        $header[17] -ne 0 -or $header[18] -ne $machine -or $header[19] -ne 0) { throw 'Selected tool is not a matching Linux ELF64 executable.' }
}

$bundleRoot = Canonical-Path $BundleDirectory
$outputRoot = Canonical-Path $OutputDirectory
$assetRoot = Canonical-Path (Join-Path $workspace 'deploy/linux')
$null = No-Reparse $bundleRoot
$null = No-Reparse $assetRoot
if (Test-Path -LiteralPath $outputRoot) { throw 'Deployment output directory must be new.' }
if ((Within $outputRoot $bundleRoot) -or (Within $outputRoot $assetRoot)) { throw 'Output cannot be inside an input directory.' }
$parent = No-Reparse (Split-Path -Parent $outputRoot)
if (-not $parent.PSIsContainer) { throw 'Output parent must be an existing directory.' }
$sourceManifest = Join-Path $bundleRoot 'bundle.json'
$null = No-Reparse $sourceManifest
if ((Hash $sourceManifest) -cne $ExpectedBundleSHA256.ToLowerInvariant()) { throw 'Source bundle manifest does not match its independently trusted digest.' }
& (Join-Path $PSScriptRoot 'verify-server-bundle.ps1') -BundleDirectory $bundleRoot | Out-Null
$manifest = Get-Content -LiteralPath $sourceManifest -Raw | ConvertFrom-Json
Fields $manifest @('schemaVersion','createdAt','goVersion','cgoEnabled','sourceInventorySHA256','modulesSHA256','artifacts','licenseNotices','runtimeAcceptancePassed')
foreach ($artifact in @($manifest.artifacts)) { Fields $artifact @('path','size','sha256') }
foreach ($notice in @($manifest.licenseNotices)) { Fields $notice @('path','sha256') }
$sourceRows = Get-Content -LiteralPath (Join-Path $bundleRoot 'source-inventory.json') -Raw | ConvertFrom-Json
foreach ($row in @($sourceRows)) { Fields $row @('path','sha256') }

foreach ($name in @('control-server','control-admin','parent-recovery','release-manifest')) {
    $relative = "$Target/$name"
    $matchesForTool = @($manifest.artifacts | Where-Object { $_.path -ceq $relative })
    if ($matchesForTool.Count -ne 1) { throw 'The selected target must contain all four matching tools.' }
    $source = Join-Path $bundleRoot $relative
    Linux-Header $source
    Add-Copy $source "bin/$name" ($matchesForTool[0].sha256.ToLowerInvariant())
}
foreach ($name in @('app-control.service','app-control-backup.service','app-control-backup.timer','backup.sh','preflight.sh','server.env.example','Caddyfile.example')) {
    $source = Join-Path $assetRoot $name
    Add-Copy $source "deploy/$name" (Hash $source)
}
$guide = Join-Path $workspace 'docs/SERVER_HOSTING.md'
Add-Copy $guide 'SERVER_HOSTING.md' (Hash $guide)
Add-Copy $sourceManifest 'metadata/server-bundle.json' ($ExpectedBundleSHA256.ToLowerInvariant())
Add-Copy (Join-Path $bundleRoot 'source-inventory.json') 'metadata/source-inventory.json' ($manifest.sourceInventorySHA256.ToLowerInvariant())
Add-Copy (Join-Path $bundleRoot 'modules.txt') 'metadata/modules.txt' ($manifest.modulesSHA256.ToLowerInvariant())
foreach ($notice in @($manifest.licenseNotices)) {
    if ($notice.path -cnotmatch '^licenses/.+/(LICENSE|COPYING|NOTICE|PATENTS)(\.[A-Za-z0-9._-]+)?$') { throw 'Only reviewed license notice paths may be exported.' }
    Add-Copy (Join-Path $bundleRoot $notice.path) $notice.path ($notice.sha256.ToLowerInvariant())
}

$null = New-Item -ItemType Directory -Path $outputRoot -ErrorAction Stop
$null = No-Reparse $outputRoot
$records = @()
foreach ($copy in $copies) {
    $destination = Join-Path $outputRoot $copy.path
    $null = [IO.Directory]::CreateDirectory((Split-Path -Parent $destination))
    $null = No-Reparse (Split-Path -Parent $destination)
    [IO.File]::Copy($copy.source,$destination,$false)
    if ((Hash $destination) -cne $copy.sha256 -or (Get-Item -LiteralPath $destination).Length -ne $copy.size) { throw 'Copied bytes did not match reviewed source; export is incomplete.' }
    $records += [ordered]@{path=$copy.path;size=$copy.size;sha256=$copy.sha256}
}
# Detect source/asset mutation during staging. No completion record is published on failure.
foreach ($copy in $copies) {
    $null = No-Reparse $copy.source
    if ((Hash $copy.source) -cne $copy.sha256) { throw 'Source changed during export; output is incomplete.' }
    $null = No-Reparse (Join-Path $outputRoot $copy.path)
}
$completion = [ordered]@{
    schemaVersion=1;scope='offline-linux-deployment-export';createdAtUtc=[datetime]::UtcNow.ToString('o')
    target=$Target;sourceBundleSHA256=$ExpectedBundleSHA256.ToLowerInvariant();files=$records
    credentialsIncluded=$false;deploymentPerformed=$false;runtimeAcceptancePassed=$false;productionReady=$false
}
$bytes = [Text.UTF8Encoding]::new($false).GetBytes(($completion | ConvertTo-Json -Depth 8))
$stream = [IO.File]::Open((Join-Path $outputRoot 'deployment.json'),[IO.FileMode]::CreateNew,[IO.FileAccess]::Write,[IO.FileShare]::None)
try { $stream.Write($bytes,0,$bytes.Length); $stream.Flush($true) } finally { $stream.Dispose() }
Write-Output "Linux deployment export completed: $Target; $($records.Count) reviewed files. No credentials, installation or deployment."
Write-Output ('deployment_manifest_sha256='+(Hash (Join-Path $outputRoot 'deployment.json')))
