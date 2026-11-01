param(
    [Parameter(Mandatory)] [ValidateNotNullOrEmpty()] [string]$Serial,
    [Parameter(Mandatory)] [ValidateSet('parent','child')] [string]$App,
    [Parameter(Mandatory)] [string]$Apk
)
# Installs only a first copy or a same-signer, non-downgrading replacement.
# It intentionally has no uninstall, clear-data or signature-bypass path.
$ErrorActionPreference='Stop'
. (Join-Path $PSScriptRoot 'apk-replacement-rules.ps1')
$workspace=Split-Path -Parent $PSScriptRoot
$adb=Join-Path $workspace '.tooling\android-sdk\platform-tools\adb.exe'
$aapt=Join-Path $workspace '.tooling\android-sdk\build-tools\36.0.0\aapt.exe'
$apksigner=Join-Path $workspace '.tooling\android-sdk\build-tools\36.0.0\apksigner.bat'
foreach($tool in @($adb,$aapt,$apksigner)){if(-not(Test-Path -LiteralPath $tool -PathType Leaf)){throw 'Pinned Android deployment tool is missing.'}}
$apkPath=[IO.Path]::GetFullPath($Apk)
if(-not(Test-Path -LiteralPath $apkPath -PathType Leaf)){throw 'APK is missing.'}
$expectedPackage='com.appcontrol.'+$App
function Read-Signer([string]$Path){
    $output=@(& $apksigner verify --print-certs $Path 2>&1)
    if($LASTEXITCODE -ne 0){throw 'APK signature verification failed.'}
    $digests=@($output|ForEach-Object{if("$_" -cmatch '^Signer #[0-9]+ certificate SHA-256 digest: ([a-fA-F0-9]{64})$'){$Matches[1].ToLowerInvariant()}})
    if($digests.Count -ne 1){throw 'Exactly one APK signer is required.'}
    return $digests[0]
}
$badging=@(& $aapt dump badging $apkPath 2>&1)
if($LASTEXITCODE -ne 0 -or ($badging -join "`n") -notmatch "(?m)^package: name='([^']+)' versionCode='([0-9]+)'"){throw 'APK package metadata is invalid.'}
if($Matches[1] -cne $expectedPackage){throw 'APK package does not match the selected app role.'}
$replacementVersion=[long]$Matches[2]
$replacementSigner=Read-Signer $apkPath
$ErrorActionPreference='Continue'
$installed=@(& $adb -s $Serial shell pm path $expectedPackage 2>&1)
$queryExit=$LASTEXITCODE
$ErrorActionPreference='Stop'
$remote=Get-AcceptanceInstalledBaseApk $queryExit @($installed|ForEach-Object{"$_"})
if($null -ne $remote){
    $metadata=@(& $adb -s $Serial shell dumpsys package $expectedPackage 2>&1)
    if($LASTEXITCODE -ne 0 -or ($metadata -join "`n") -notmatch '\bversionCode=([0-9]+)\b'){throw 'Installed APK version could not be verified.'}
    $installedVersion=[long]$Matches[1]
    $inspection=Join-Path $workspace ('.cache\app-control-replacement-'+[guid]::NewGuid().ToString('N'))
    $null=New-Item -ItemType Directory -Path $inspection
    try{
        $copy=Join-Path $inspection 'installed.apk'
        & $adb -s $Serial pull $remote $copy | Out-Null
        if($LASTEXITCODE -ne 0){throw 'Installed signer inspection copy failed.'}
        Assert-AppControlReplacementIdentity $replacementVersion $installedVersion $replacementSigner (Read-Signer $copy)
    }finally{
        $resolved=[IO.Path]::GetFullPath($inspection);$cache=[IO.Path]::GetFullPath((Join-Path $workspace '.cache'))
        if(-not $resolved.StartsWith($cache+[IO.Path]::DirectorySeparatorChar,[StringComparison]::OrdinalIgnoreCase)){throw 'Refusing cleanup outside workspace cache.'}
        Remove-Item -LiteralPath $resolved -Recurse -Force
    }
}
& $adb -s $Serial install -r $apkPath
if($LASTEXITCODE -ne 0){throw 'APK replacement failed; app data was not intentionally removed.'}
Write-Output "$expectedPackage installed without uninstalling or clearing enrollment data."
