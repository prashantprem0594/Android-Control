param(
    [Parameter(Mandatory)] [string]$InputDirectory,
    [Parameter(Mandatory)] [string]$OutputDirectory,
    [Parameter(Mandatory)] [string]$ServerOrigin,
    [Parameter(Mandatory)] [ValidateRange(1,2100000000)] [int]$Sequence,
    [Parameter(Mandatory)] [ValidatePattern('^[a-f0-9]{64}$')] [string]$ExpectedCertificateSHA256,
    [Parameter(Mandatory)] [string]$MetadataPrivateKeyFile,
    [Parameter(Mandatory)] [string]$MetadataPublicKeyFile,
    [ValidateRange(1,30)] [int]$ValidityDays = 7,
    [string]$ManifestExecutable = ''
)
# Invoke through run-logged.ps1. Builds a fresh public directory only; no upload,
# server configuration changes, private-key copying or artifact overwriting.
$ErrorActionPreference = 'Stop'
$workspaceRoot = Split-Path -Parent $PSScriptRoot
if (-not $ManifestExecutable) { $ManifestExecutable = Join-Path $workspaceRoot '.cache\release-manifest.exe' }
$toolsRoot = Join-Path $workspaceRoot '.tooling\android-sdk\build-tools\36.0.0'
$aapt = Join-Path $toolsRoot 'aapt.exe'
$signer = Join-Path $toolsRoot 'apksigner.bat'
function Invoke-PublicTool([string]$Executable,[string[]]$Arguments) {
    $ErrorActionPreference = 'Continue'
    $captured = @(& $Executable @Arguments 2>&1)
    if ($LASTEXITCODE -ne 0) { throw 'Release tool verification failed; bundle is not approved.' }
    return ($captured -join "`n")
}
function Write-NewPublicJSON([string]$Path,$Value) {
    $bytes = [Text.UTF8Encoding]::new($false).GetBytes(($Value | ConvertTo-Json -Depth 12))
    $file = [IO.File]::Open($Path,[IO.FileMode]::CreateNew,[IO.FileAccess]::Write,[IO.FileShare]::None)
    try { $file.Write($bytes,0,$bytes.Length);$file.Flush($true) } finally { $file.Dispose() }
}
if (Test-Path -LiteralPath $OutputDirectory) { throw 'Release staging directory must not exist.' }
foreach ($path in @($MetadataPrivateKeyFile,$MetadataPublicKeyFile,$ManifestExecutable)) {
    if (-not (Test-Path -LiteralPath $path -PathType Leaf)) { throw 'Required release operator file is unavailable.' }
}
$outputRoot = [IO.Path]::GetFullPath($OutputDirectory)
$sequenceText = $Sequence.ToString([Globalization.CultureInfo]::InvariantCulture)
$artifactRoot = Join-Path $outputRoot ('releases\'+$sequenceText)
New-Item -ItemType Directory -Path $artifactRoot -ErrorAction Stop | Out-Null
$apps = @()
$savedJava = $env:JAVA_HOME
try {
    $env:JAVA_HOME = 'C:\Program Files\Java\jdk-17'
    foreach ($name in @('parent','child')) {
        $source = Join-Path $InputDirectory ($name+'-release.apk')
        $destination = Join-Path $artifactRoot ($name+'-release.apk')
        if (-not (Test-Path -LiteralPath $source -PathType Leaf)) { throw 'Both signed release APKs are required.' }
        [IO.File]::Copy([IO.Path]::GetFullPath($source),$destination,$false)
        $minimumSdk = if ($name -eq 'child') { '28' } else { '33' }
        # Inspect the copied bytes, never an unsigned inventory supplied beside them.
        $signed = Invoke-PublicTool $signer @('verify','--verbose','--print-certs','--min-sdk-version',$minimumSdk,$destination)
        $certificates = [regex]::Matches($signed,'(?m)^Signer #[0-9]+ certificate SHA-256 digest: ([a-fA-F0-9]{64})\r?$')
        if ($certificates.Count -ne 1 -or $certificates[0].Groups[1].Value.ToLowerInvariant() -cne $ExpectedCertificateSHA256 -or $signed.Contains('CN=Android Debug')) { throw 'APK does not have the expected release signer.' }
        $badging = Invoke-PublicTool $aapt @('dump','badging',$destination)
        $package = [regex]::Match($badging,"(?m)^package: name='([^']+)' versionCode='([0-9]+)' versionName='([^']+)'[^\r\n]*")
        $minimum = [regex]::Match($badging,"(?m)^sdkVersion:'([0-9]+)'\r?$")
        $target = [regex]::Match($badging,"(?m)^targetSdkVersion:'([0-9]+)'\r?$")
        $manifest = Invoke-PublicTool $aapt @('dump','xmltree',$destination,'AndroidManifest.xml')
        if (-not $package.Success -or $package.Groups[1].Value -cne ('com.appcontrol.'+$name) -or
            -not $minimum.Success -or -not $target.Success -or $badging.Contains('application-debuggable') -or
            $package.Value -match "\bsplit='" -or $manifest -match 'android:(debuggable|testOnly)[^\r\n]*0xffffffff') { throw 'Expected monolithic non-debug, non-test release APKs.' }
        $info = Get-Item -LiteralPath $destination
        if ($info.Length -lt 1 -or $info.Length -gt 256MB) { throw 'APK length is outside the release limit.' }
        $apps += [ordered]@{
            package='com.appcontrol.'+$name
            versionCode=[int]$package.Groups[2].Value
            versionName=$package.Groups[3].Value
            minSdk=[int]$minimum.Groups[1].Value
            targetSdk=[int]$target.Groups[1].Value
            url=$ServerOrigin+'/releases/'+$sequenceText+'/'+$name+'-release.apk'
            bytes=$info.Length
            sha256=(Get-FileHash -LiteralPath $destination -Algorithm SHA256).Hash.ToLowerInvariant()
            signingCertificateSHA256=$ExpectedCertificateSHA256
        }
    }
    $now = [DateTime]::UtcNow
    $format = "yyyy-MM-dd'T'HH:mm:ss'Z'"
    $payload = [ordered]@{version=1;sequence=$Sequence;issuedAt=$now.ToString($format,[Globalization.CultureInfo]::InvariantCulture);
        expiresAt=$now.AddDays($ValidityDays).ToString($format,[Globalization.CultureInfo]::InvariantCulture);origin=$ServerOrigin;apps=$apps}
    $payloadPath = Join-Path $outputRoot 'release-payload.json'
    $manifestPath = Join-Path $outputRoot 'release-manifest.json'
    Write-NewPublicJSON $payloadPath $payload
    $null = Invoke-PublicTool $ManifestExecutable @('sign','-input',$payloadPath,'-private-key',$MetadataPrivateKeyFile,'-out',$manifestPath)
    $null = Invoke-PublicTool $ManifestExecutable @('verify','-input',$manifestPath,'-public-key',$MetadataPublicKeyFile,'-origin',$ServerOrigin,'-certificate-sha256',$ExpectedCertificateSHA256,'-minimum-sequence',$sequenceText)
    $publicPath = Join-Path $outputRoot 'release-public-key.pub'
    [IO.File]::Copy([IO.Path]::GetFullPath($MetadataPublicKeyFile),$publicPath,$false)
    Write-NewPublicJSON (Join-Path $outputRoot 'bundle.json') ([ordered]@{
        stagedAtUtc=[DateTime]::UtcNow.ToString('o');origin=$ServerOrigin;sequence=$Sequence;
        manifestSHA256=(Get-FileHash -LiteralPath $manifestPath -Algorithm SHA256).Hash.ToLowerInvariant();
        publicKeySHA256=(Get-FileHash -LiteralPath $publicPath -Algorithm SHA256).Hash.ToLowerInvariant();
        apps=$apps;scope='Verified local public release bundle. Nothing uploaded, deployed or installed. Private metadata key excluded.'
    })
    Write-Output "Verified public release bundle staged: $outputRoot"
    Write-Output 'Host only the releases subtree at the signed HTTPS origin; configure the backend with the verified manifest/public key. Nothing published.'
} finally { $env:JAVA_HOME = $savedJava }
