[CmdletBinding()]
param(
    [string]$SourceDirectory = 'C:\Users\Sushant\Downloads\gradle-9.6.0-bin\gradle-9.6.0',
    [string]$TargetDirectory = 'C:\Gradle\gradle-9.6.0'
)

$ErrorActionPreference = 'Stop'

$source = (Resolve-Path -LiteralPath $SourceDirectory).Path
$sourceGradle = Join-Path $source 'bin\gradle.bat'
if (-not (Test-Path -LiteralPath $sourceGradle -PathType Leaf)) {
    throw "Gradle executable is missing from source: $sourceGradle"
}

$allowedTargetRoot = [IO.Path]::GetFullPath('C:\Gradle')
$target = [IO.Path]::GetFullPath($TargetDirectory)
$allowedPrefix = $allowedTargetRoot.TrimEnd('\') + '\'
if (-not $target.StartsWith($allowedPrefix, [StringComparison]::OrdinalIgnoreCase)) {
    throw "Target must remain under $allowedTargetRoot"
}

New-Item -ItemType Directory -Path $target -Force | Out-Null
Get-ChildItem -LiteralPath $source -Force | Copy-Item -Destination $target -Recurse -Force

$targetGradle = Join-Path $target 'bin\gradle.bat'
if (-not (Test-Path -LiteralPath $targetGradle -PathType Leaf)) {
    throw "Gradle copy verification failed: $targetGradle"
}

$gradleBin = Join-Path $target 'bin'
$userPath = [Environment]::GetEnvironmentVariable('Path', 'User')
$pathEntries = @($userPath -split ';' | Where-Object { -not [string]::IsNullOrWhiteSpace($_) })
if ($pathEntries -contains $gradleBin) {
    Write-Output "User PATH already contains $gradleBin"
} else {
    Write-Output "PATH entry requires manual Windows Environment Variables update: $gradleBin"
}

Write-Output "Gradle installed at $target"
& $targetGradle --version
