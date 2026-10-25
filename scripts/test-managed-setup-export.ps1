param(
    [Parameter(Mandatory)] [string]$BundleDirectory,
    [Parameter(Mandatory)] [string]$MetadataPublicKeyFile,
    [Parameter(Mandatory)] [string]$ServerOrigin,
    [Parameter(Mandatory)] [string]$ExpectedCertificateSHA256
)
# Use an independently signed, pre-handler fixture to exercise real refusals.
$ErrorActionPreference = 'Stop'
$workspace = Split-Path -Parent $PSScriptRoot
$root = Join-Path $workspace ('.cache\managed-setup-checks-' + [guid]::NewGuid().ToString('N'))
$null = New-Item -ItemType Directory -Path $root
$exporter = Join-Path $PSScriptRoot 'export-managed-setup.ps1'
$arguments = @{BundleDirectory=$BundleDirectory;MetadataPublicKeyFile=$MetadataPublicKeyFile;ServerOrigin=$ServerOrigin;ExpectedCertificateSHA256=$ExpectedCertificateSHA256}
function Assert-Refused([string]$Output,[string]$Reason) {
    $rejected = $false
    try { & $exporter @arguments -OutputDirectory $Output }
    catch {
        if ($_.Exception.Message -notlike ('*'+$Reason+'*')) { throw }
        $rejected = $true
    }
    if (-not $rejected) { throw 'Expected setup refusal did not occur.' }
    if (Test-Path -LiteralPath (Join-Path $Output 'setup-bundle.json')) { throw 'Failed export published a completion inventory.' }
}
$existing = Join-Path $root 'existing'
$null = New-Item -ItemType Directory -Path $existing
$sentinel = Join-Path $existing 'keep.txt'
[IO.File]::WriteAllText($sentinel,'preserve existing output')
$before = (Get-FileHash -LiteralPath $sentinel).Hash
Assert-Refused $existing 'output directory must be new'
if ((Get-FileHash -LiteralPath $sentinel).Hash -cne $before) { throw 'Existing output changed.' }
$nested = Join-Path $BundleDirectory ('refused-setup-'+[guid]::NewGuid().ToString('N'))
Assert-Refused $nested 'outside the immutable release bundle'
if (Test-Path -LiteralPath $nested) { throw 'Release bundle was modified.' }
$actualCertificate = $arguments.ExpectedCertificateSHA256
$arguments.ExpectedCertificateSHA256 = '0' * 64
Assert-Refused (Join-Path $root 'wrong-certificate') 'independently verified release metadata'
$arguments.ExpectedCertificateSHA256 = $actualCertificate
Assert-Refused (Join-Path $root 'old-apk') 'required protected managed-setup handler'
Write-Output 'Existing output preservation, immutable bundle protection, wrong certificate and pre-handler APK refusals pass. No completion inventory published on failure; no device operations.'
Write-Output "Evidence directory: $root"
