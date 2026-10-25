param(
    [switch]$AcceptAndroidSdkLicenses
)

$ErrorActionPreference = 'Stop'

if (-not $AcceptAndroidSdkLicenses) {
    throw 'Pass -AcceptAndroidSdkLicenses after reviewing and accepting the Android SDK license.'
}

$workspaceRoot = Split-Path -Parent $PSScriptRoot
$toolingRoot = Join-Path $workspaceRoot '.tooling'
$downloadsRoot = Join-Path $toolingRoot 'downloads'
$goRoot = Join-Path $toolingRoot 'go'
$androidRoot = Join-Path $toolingRoot 'android-sdk'

New-Item -ItemType Directory -Force -Path $downloadsRoot | Out-Null
New-Item -ItemType Directory -Force -Path $androidRoot | Out-Null

function Get-VerifiedArchive {
    param(
        [Parameter(Mandatory)] [string]$Uri,
        [Parameter(Mandatory)] [string]$Destination,
        [Parameter(Mandatory)] [string]$ExpectedSha256
    )

    if (-not (Test-Path -LiteralPath $Destination)) {
        Invoke-WebRequest -Uri $Uri -OutFile $Destination
    }

    $actualHash = (Get-FileHash -LiteralPath $Destination -Algorithm SHA256).Hash.ToLowerInvariant()
    if ($actualHash -ne $ExpectedSha256.ToLowerInvariant()) {
        throw "Checksum mismatch for $Destination. Expected $ExpectedSha256, got $actualHash"
    }
}

$goArchive = Join-Path $downloadsRoot 'go1.27.3.windows-amd64.zip'
Get-VerifiedArchive `
    -Uri 'https://go.dev/dl/go1.27.3.windows-amd64.zip' `
    -Destination $goArchive `
    -ExpectedSha256 'f26081f438319757f8a1b46d7d0b2c5ec7583d4038e485e0aa725cfde7885fff'

if (-not (Test-Path -LiteralPath (Join-Path $goRoot 'bin\go.exe'))) {
    $goExtractRoot = Join-Path $toolingRoot 'go-extracted'
    Expand-Archive -LiteralPath $goArchive -DestinationPath $goExtractRoot -Force
    Copy-Item -LiteralPath (Join-Path $goExtractRoot 'go') -Destination $goRoot -Recurse -Force
}

$androidArchive = Join-Path $downloadsRoot 'commandlinetools-win-15859902_latest.zip'
Get-VerifiedArchive `
    -Uri 'https://dl.google.com/android/repository/commandlinetools-win-15859902_latest.zip' `
    -Destination $androidArchive `
    -ExpectedSha256 '90ae805d20434428bffcb699c290860f19bb5f66a67e6b330067e3de801fb04a'

$sdkManager = Join-Path $androidRoot 'cmdline-tools\latest\bin\sdkmanager.bat'
if (-not (Test-Path -LiteralPath $sdkManager)) {
    $androidExtractRoot = Join-Path $toolingRoot 'android-command-line-extracted'
    Expand-Archive -LiteralPath $androidArchive -DestinationPath $androidExtractRoot -Force
    $latestRoot = Join-Path $androidRoot 'cmdline-tools\latest'
    New-Item -ItemType Directory -Force -Path $latestRoot | Out-Null
    Copy-Item -Path (Join-Path $androidExtractRoot 'cmdline-tools\*') -Destination $latestRoot -Recurse -Force
}

$env:ANDROID_HOME = $androidRoot
$env:ANDROID_SDK_ROOT = $androidRoot

1..20 | ForEach-Object { 'y' } | & $sdkManager --sdk_root=$androidRoot --licenses | Out-Host
& $sdkManager --sdk_root=$androidRoot 'platform-tools' 'platforms;android-37' 'platforms;android-34' 'platforms;android-33' 'build-tools;36.0.0'

Write-Output "Go: $goRoot"
Write-Output "Android SDK: $androidRoot"
Write-Output 'Toolchain bootstrap complete.'

