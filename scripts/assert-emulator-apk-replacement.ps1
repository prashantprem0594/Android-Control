param(
    [Parameter(Mandatory)] [string[]]$Apk,
    [ValidateSet('127.0.0.1:5581','emulator-5580')] [string]$Serial='127.0.0.1:5581',
    [ValidateSet(5038)] [int]$AdbServerPort=5038
)
# Read-only device inspection, with temporary APK copies in a unique workspace directory.
# Checks every artifact before the caller installs any of them. Never clears or uninstalls.
$ErrorActionPreference='Stop'
. (Join-Path $PSScriptRoot 'apk-replacement-rules.ps1')
$workspace=Split-Path -Parent $PSScriptRoot
$adb=Join-Path $workspace '.tooling\android-sdk\platform-tools\adb.exe'
$aapt=Join-Path $workspace '.tooling\android-sdk\build-tools\36.0.0\aapt.exe'
$apksigner=Join-Path $workspace '.tooling\android-sdk\build-tools\36.0.0\apksigner.bat'
$fingerprint=@(& $adb -P $AdbServerPort -s $Serial shell getprop ro.build.fingerprint 2>&1)
if($LASTEXITCODE -ne 0 -or ($fingerprint -join '').Trim() -notlike 'google/sdk_gphone64_x86_64/emu64xa:*'){throw 'Isolated acceptance emulator required before inspecting installed APKs.'}
$inspection=Join-Path $workspace ('.cache\apk-replacement-'+[guid]::NewGuid().ToString('N'))
$null=New-Item -ItemType Directory -Path $inspection
$savedJava=$env:JAVA_HOME
$env:JAVA_HOME='C:\Program Files\Java\jdk-17'
function Signer([string]$Path){
    $output=@(& $apksigner verify --print-certs $Path 2>&1)
    if($LASTEXITCODE -ne 0){throw 'APK signature verification failed.'}
    $digests=@($output | ForEach-Object { if("$_" -cmatch '^Signer #[0-9]+ certificate SHA-256 digest: ([a-fA-F0-9]{64})$'){$Matches[1].ToLowerInvariant()} })
    if($digests.Count -ne 1){throw 'Exactly one APK signer is required for acceptance replacement.'}
    return $digests[0]
}
try{
    foreach($path in $Apk){
        $badging=@(& $aapt dump badging $path 2>&1)
        if($LASTEXITCODE -ne 0 -or ($badging -join "`n") -notmatch "(?m)^package: name='([^']+)' versionCode='([0-9]*)'" -or $badging -notcontains 'application-debuggable'){throw 'Replacement must be an identifiable debuggable APK.'}
        $package=$Matches[1]; $versionText=$Matches[2]
        if($package -cnotin @('com.appcontrol.child','com.appcontrol.child.test','org.example.appcontrolnetworkfixture','com.appcontrol.parent.acceptance','com.appcontrol.parent.acceptance.test')){throw 'Package is outside the disposable acceptance allowlist.'}
        $target='';$runner=''
        if($versionText -ceq ''){
            $manifest=@(& $aapt dump xmltree $path AndroidManifest.xml 2>&1) -join "`n"
            if($LASTEXITCODE -ne 0){throw 'Versionless instrumentation manifest inspection failed.'}
            if($manifest -match 'android:targetPackage\([^)]*\)="([^"]+)"'){$target=$Matches[1]}
            if($manifest -match 'android:name\([^)]*\)="(com\.appcontrol\.transport\.ServerTrustInstrumentation)"'){$runner=$Matches[1]}
        }
        $version=Resolve-AcceptanceApkVersion $package $versionText $target $runner
        $signer=Signer $path
        $ErrorActionPreference='Continue'
        $installed=@(& $adb -P $AdbServerPort -s $Serial shell pm path $package 2>&1)
        $queryExit=$LASTEXITCODE
        $ErrorActionPreference='Stop'
        $remote=Get-AcceptanceInstalledBaseApk $queryExit @($installed | ForEach-Object {"$_"})
        if($null -eq $remote){continue}
        $metadata=@(& $adb -P $AdbServerPort -s $Serial shell dumpsys package $package 2>&1)
        $text=$metadata -join "`n"
        if($LASTEXITCODE -ne 0 -or $text -notmatch '\bversionCode=([0-9]+)\b'){throw 'Installed APK version could not be verified.'}
        $installedVersion=[long]$Matches[1]
        if($version -lt $installedVersion){throw 'Replacement would downgrade an installed acceptance APK.'}
        $installedDebuggable=$text -match 'pkgFlags=.*DEBUGGABLE'
        if(-not $installedDebuggable){throw 'Installed APK is not a disposable debug build.'}
        $copy=Join-Path $inspection ($package+'.apk')
        $ErrorActionPreference='Continue'
        $null=@(& $adb -P $AdbServerPort -s $Serial pull $remote $copy 2>&1)
        $pullExit=$LASTEXITCODE
        $ErrorActionPreference='Stop'
        if($pullExit -ne 0){throw 'Installed signer inspection copy failed.'}
        Assert-AcceptanceReplacementIdentity $version $installedVersion $installedDebuggable $signer (Signer $copy)
        Write-Output "Verified replacement identity, nondecreasing version and installed signer: $package"
    }
}finally{
    $env:JAVA_HOME=$savedJava
    $resolved=[IO.Path]::GetFullPath($inspection)
    $cache=[IO.Path]::GetFullPath((Join-Path $workspace '.cache'))
    $info=Get-Item -LiteralPath $resolved -Force
    if(-not $resolved.StartsWith($cache+[IO.Path]::DirectorySeparatorChar,[StringComparison]::OrdinalIgnoreCase) -or ($info.Attributes -band [IO.FileAttributes]::ReparsePoint) -ne 0){throw 'Refusing cleanup outside the unique APK inspection directory.'}
    Remove-Item -LiteralPath $resolved -Recurse -Force
}
# A valid first install may end with pm path's expected absent-package exit 1.
# Successful policy inspection must not propagate that native status to the caller.
$global:LASTEXITCODE=0
