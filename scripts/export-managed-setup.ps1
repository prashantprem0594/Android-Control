param(
    [Parameter(Mandatory)] [string]$BundleDirectory,
    [Parameter(Mandatory)] [string]$MetadataPublicKeyFile,
    [Parameter(Mandatory)] [string]$ServerOrigin,
    [Parameter(Mandatory)] [ValidatePattern('^[a-f0-9]{64}$')] [string]$ExpectedCertificateSHA256,
    [Parameter(Mandatory)] [string]$OutputDirectory,
    [ValidateRange(0,2100000000)] [int]$MinimumSequence = 0,
    [string]$QrExecutable = ''
)
# Run through run-logged.ps1. Public files only; never uses ADB or provisions.
$ErrorActionPreference = 'Stop'
$workspace = Split-Path -Parent $PSScriptRoot
if (-not $QrExecutable) { $QrExecutable = Join-Path $workspace '.cache\provisioning-qr.exe' }
if (-not (Test-Path -LiteralPath $QrExecutable -PathType Leaf)) { throw 'Build the offline provisioning QR tool first.' }
$bundle = (Resolve-Path -LiteralPath $BundleDirectory).Path.TrimEnd('\','/')
$output = [IO.Path]::GetFullPath($OutputDirectory)
if (Test-Path -LiteralPath $output) { throw 'Managed-setup output directory must be new.' }
if ($output.StartsWith($bundle + [IO.Path]::DirectorySeparatorChar,[StringComparison]::OrdinalIgnoreCase)) { throw 'Keep setup output outside the immutable release bundle.' }
$metadata = Join-Path $bundle 'release-manifest.json'
$verifier = Join-Path $workspace '.cache\release-manifest.exe'
$verifierHash = (Get-FileHash -LiteralPath $verifier -Algorithm SHA256).Hash.ToLowerInvariant()
$metadataHash = (Get-FileHash -LiteralPath $metadata -Algorithm SHA256).Hash.ToLowerInvariant()
$publicKeyHash = (Get-FileHash -LiteralPath $MetadataPublicKeyFile -Algorithm SHA256).Hash.ToLowerInvariant()
$rendererHash = (Get-FileHash -LiteralPath $QrExecutable -Algorithm SHA256).Hash.ToLowerInvariant()
$null = New-Item -ItemType Directory -Path $output
$payload = Join-Path $output 'provisioning.json'
$png = Join-Path $output 'provisioning.png'
& (Join-Path $PSScriptRoot 'new-managed-provisioning-payload.ps1') -BundleDirectory $bundle -MetadataPublicKeyFile $MetadataPublicKeyFile -ServerOrigin $ServerOrigin -ExpectedCertificateSHA256 $ExpectedCertificateSHA256 -MinimumSequence $MinimumSequence -OutputFile $payload
$payloadHash = (Get-FileHash -LiteralPath $payload -Algorithm SHA256).Hash.ToLowerInvariant()
& $QrExecutable -input $payload -output $png
if ($LASTEXITCODE -ne 0) { throw 'QR rendering/independent decoding failed; setup export is incomplete.' }
if ((Get-FileHash -LiteralPath $payload -Algorithm SHA256).Hash.ToLowerInvariant() -cne $payloadHash) { throw 'Verified provisioning payload changed during QR rendering.' }
if ((Get-FileHash -LiteralPath $metadata -Algorithm SHA256).Hash.ToLowerInvariant() -cne $metadataHash -or
    (Get-FileHash -LiteralPath $MetadataPublicKeyFile -Algorithm SHA256).Hash.ToLowerInvariant() -cne $publicKeyHash -or
    (Get-FileHash -LiteralPath $QrExecutable -Algorithm SHA256).Hash.ToLowerInvariant() -cne $rendererHash -or
    (Get-FileHash -LiteralPath $verifier -Algorithm SHA256).Hash.ToLowerInvariant() -cne $verifierHash) { throw 'Setup authority or tool changed during export.' }
& $verifier verify -input $metadata -public-key $MetadataPublicKeyFile -origin $ServerOrigin -certificate-sha256 $ExpectedCertificateSHA256 -minimum-sequence $MinimumSequence
if ($LASTEXITCODE -ne 0) { throw 'Release authority is no longer current; setup export is incomplete.' }
$report = [ordered]@{
    schemaVersion=1;createdAt=[datetime]::UtcNow.ToString('o');serverOrigin=$ServerOrigin
    metadataSHA256=$metadataHash;metadataPublicKeySHA256=$publicKeyHash;apkCertificateSHA256=$ExpectedCertificateSHA256
    rendererSHA256=$rendererHash;verifierSHA256=$verifierHash;minimumSequence=$MinimumSequence
    payloadSHA256=$payloadHash
    pngSHA256=(Get-FileHash -LiteralPath $png -Algorithm SHA256).Hash.ToLowerInvariant()
    softwareRoundTripVerified=$true;deviceScanVerified=$false;deviceProvisioned=$false
}
$bytes = [Text.UTF8Encoding]::new($false).GetBytes(($report | ConvertTo-Json -Depth 5))
$file = [IO.File]::Open((Join-Path $output 'setup-bundle.json'),[IO.FileMode]::CreateNew,[IO.FileAccess]::Write,[IO.FileShare]::None)
try { $file.Write($bytes,0,$bytes.Length);$file.Flush($true) } finally { $file.Dispose() }
Write-Output 'Verified release payload and independently decoded QR exported. No device scan, enrollment or provisioning performed.'
