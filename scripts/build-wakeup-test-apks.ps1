param(
    [string]$FirebaseSettingsDirectory,
    [Parameter(Mandatory)] [string]$ServerOrigin,
    [Parameter(Mandatory)] [string]$CurrentPin,
    [Parameter(Mandatory)] [string]$BackupPin,
    [string]$OutputDirectory,
    [switch]$PollingOnly,
    [string]$DevelopmentCaFile,
    [ValidateRange(1,2100000000)] [int]$VersionCode=1,
    [ValidatePattern('\A[A-Za-z0-9][A-Za-z0-9._+\-]{0,63}\z')] [string]$VersionName='0.1.0',
    [switch]$ValidateOnly,
    [switch]$AllowFixture
)
# Controlled debug proof only. Run through run-logged.ps1; never installs or deploys.
$ErrorActionPreference='Stop'
$workspace=Split-Path -Parent $PSScriptRoot
function Read-PublicJson([string]$Path) {
    try {
        $file=Get-Item -LiteralPath $Path -ErrorAction Stop
        if($file -isnot [IO.FileInfo] -or $file.Length -gt 16384){throw 'size'}
        return ([IO.File]::ReadAllText($file.FullName,[Text.UTF8Encoding]::new($false,$true)) | ConvertFrom-Json)
    } catch { throw 'Invalid or missing bounded public settings file.' }
}
function Require-Fields($Object,[string[]]$Fields) {
    if($Object -isnot [pscustomobject]){throw 'Expected public settings object.'}
    $names=@($Object.PSObject.Properties.Name)
    if($names.Count -ne $Fields.Count -or @($names | Where-Object {$_ -cnotin $Fields}).Count){throw 'Unexpected public settings fields.'}
}
function Read-Firebase([string]$Name) {
    $value=Read-PublicJson (Join-Path $FirebaseSettingsDirectory $Name)
    Require-Fields $value @('applicationId','apiKey','projectId','senderId')
    foreach($field in @('applicationId','apiKey','projectId','senderId')){if($value.$field -isnot [string]){throw 'Invalid public Firebase field type.'}}
    if($value.applicationId -cnotmatch '\A1:[0-9]+:android:[a-fA-F0-9]+\z' -or
        $value.apiKey -cnotmatch '\A[A-Za-z0-9_-]{20,128}\z' -or
        $value.projectId -cnotmatch '\A[a-z0-9][a-z0-9-]{4,62}\z' -or
        $value.senderId -cnotmatch '\A[0-9]{1,32}\z' -or
        $value.applicationId.Split(':')[1] -cne $value.senderId){throw 'Invalid or inconsistent public Firebase settings.'}
    return $value
}
if($ServerOrigin -cnotmatch '^https://[a-z0-9.-]+(?::[0-9]+)?$'){throw 'Use a canonical lowercase HTTPS DNS origin without path or credentials.'}
$origin=[uri]$ServerOrigin
$hostName=$origin.DnsSafeHost
$developmentLocal=$hostName -ceq 'localhost' -and -not [string]::IsNullOrWhiteSpace($DevelopmentCaFile)
if($origin.HostNameType -ne [UriHostNameType]::Dns -or $hostName.Length -gt 253 -or $hostName -notmatch '[a-z]' -or ($hostName -notmatch '\.' -and -not $developmentLocal) -or $origin.Port -notin 1..65535){throw 'Invalid HTTPS DNS origin.'}
foreach($label in $hostName.Split('.')){if($label.Length -notin 1..63 -or $label -cnotmatch '^[a-z0-9](?:[a-z0-9-]*[a-z0-9])?$'){throw 'Invalid DNS label.'}}
$canonical='https://'+$hostName+$(if($origin.Port -ne 443){':'+$origin.Port}else{''})
if($canonical -cne $ServerOrigin){throw 'Origin must be canonical; omit default port.'}
if($hostName -match '(^|\.)(invalid|test|example|localhost|local|internal)$|(^|\.)example\.(com|net|org)$' -and -not $AllowFixture -and -not $developmentLocal){throw 'Reserved host refused without AllowFixture or explicit laptop CA.'}
foreach($pin in @($CurrentPin,$BackupPin)){
    try{$bytes=[Convert]::FromBase64String($pin)}catch{throw 'Invalid public TLS pin.'}
    if($bytes.Length -ne 32 -or [Convert]::ToBase64String($bytes) -cne $pin){throw 'TLS pins must be canonical SHA256 Base64.'}
}
if($CurrentPin -ceq $BackupPin){throw 'Current and backup TLS pins must differ.'}
$developmentCa=''
if(-not [string]::IsNullOrWhiteSpace($DevelopmentCaFile)){
    if(-not $developmentLocal){throw 'Development CA is restricted to the explicit laptop localhost profile.'}
    $file=Get-Item -LiteralPath $DevelopmentCaFile -ErrorAction Stop
    if($file -isnot [IO.FileInfo] -or $file.Length -gt 65536 -or $file.FullName -match '[&|<>^"%!\r\n]'){throw 'Invalid bounded local public CA path.'}
    $pem=[IO.File]::ReadAllText($file.FullName)
    if($pem -match 'PRIVATE KEY' -or $pem -notmatch '\A\s*-----BEGIN CERTIFICATE-----\s*([A-Za-z0-9+/=\r\n]+)\s*-----END CERTIFICATE-----\s*\z'){throw 'Development trust accepts exactly one public CA certificate, never private keys.'}
    try{$certificate=[Security.Cryptography.X509Certificates.X509Certificate2]::new([Convert]::FromBase64String($Matches[1]))}catch{throw 'Invalid development X509 certificate.'}
    try{
        $basic=$certificate.Extensions | Where-Object {$_.Oid.Value -eq '2.5.29.19'} | Select-Object -First 1
        if($null -eq $basic -or -not ([Security.Cryptography.X509Certificates.X509BasicConstraintsExtension]::new($basic,$basic.Critical)).CertificateAuthority -or $certificate.NotAfter.ToUniversalTime() -le [datetime]::UtcNow -or $certificate.NotBefore.ToUniversalTime() -gt [datetime]::UtcNow){throw 'Development certificate must be a currently valid public CA.'}
    }finally{$certificate.Dispose()}
    $developmentCa=$file.FullName
}
$parent=$null;$child=$null
if($PollingOnly){
    if(-not [string]::IsNullOrWhiteSpace($FirebaseSettingsDirectory)){throw 'PollingOnly cannot include Firebase settings.'}
}else{
if([string]::IsNullOrWhiteSpace($FirebaseSettingsDirectory)){throw 'Firebase settings directory or explicit PollingOnly is required.'}
$manifest=Read-PublicJson (Join-Path $FirebaseSettingsDirectory 'firebase-import.json')
if($manifest.schemaVersion -ne 1 -or $manifest.scope -cne 'public-firebase-settings-import' -or $manifest.structuralValidationPassed -ne $true -or $manifest.productionReady -ne $false){throw 'Public Firebase import completion record required.'}
$expected=@('parent-firebase-public.json')
if(Test-Path -LiteralPath (Join-Path $FirebaseSettingsDirectory 'child-firebase-public.json')){$expected+='child-firebase-public.json'}
foreach($name in $expected){
    $entry=@($manifest.files | Where-Object {$_.name -ceq $name})
    if($entry.Count -ne 1 -or $entry[0].sha256 -cnotmatch '^[a-f0-9]{64}$' -or
        (Get-FileHash -LiteralPath (Join-Path $FirebaseSettingsDirectory $name) -Algorithm SHA256).Hash.ToLowerInvariant() -cne $entry[0].sha256){throw 'Imported public settings hash mismatch.'}
}
$parent=Read-Firebase 'parent-firebase-public.json'
$child=if($expected.Count -eq 2){Read-Firebase 'child-firebase-public.json'}else{$null}
if($null -ne $child){
    if($parent.applicationId -ieq $child.applicationId){throw 'Parent and child Firebase application IDs must differ.'}
    if(($parent.projectId -ceq $child.projectId) -ne ($parent.senderId -ceq $child.senderId)){throw 'Firebase project names and sender identities are inconsistent.'}
}
}
if($ValidateOnly){Write-Output 'Public wake-up build inputs validated structurally; no builds, service ownership or delivery verified.';return}
if([string]::IsNullOrWhiteSpace($OutputDirectory) -or (Test-Path -LiteralPath $OutputDirectory)){throw 'OutputDirectory must be a new directory.'}
$gradle='C:\Gradle\gradle-9.6.0\bin\gradle.bat'
$java='C:\Program Files\Java\jdk-17'
$aapt=Join-Path $workspace '.tooling/android-sdk/build-tools/36.0.0/aapt.exe'
if(-not (Test-Path -LiteralPath $gradle) -or -not (Test-Path -LiteralPath (Join-Path $java 'bin/java.exe')) -or -not (Test-Path -LiteralPath $aapt)){throw 'Pinned local Android build tools are unavailable.'}
$output=[IO.Path]::GetFullPath($OutputDirectory)
$null=New-Item -ItemType Directory -Path $output
$arguments=@('--offline','--no-build-cache','-Pkotlin.compiler.execution.strategy=in-process',
    "-PappControlServerOrigin=$ServerOrigin","-PappControlCurrentPin=$CurrentPin","-PappControlBackupPin=$BackupPin",
    "-PappControlVersionCode=$VersionCode","-PappControlVersionName=$VersionName",'-PappControlRemoteCallsEnabled=false',
    '-PappControlUpdatePublicKey=','-PappControlUpdateCertificateSHA256=','-PappControlUpdateMinimumSequence=0',
    "-PappControlDevelopmentCaFile=$developmentCa")
$parentVariant='debug';$parentTask='Debug'
if($null -ne $parent){
    $parentVariant='fcmDebug';$parentTask='FcmDebug'
    $arguments+=@('-PappControlParentFirebase=true',"-PappControlParentFirebaseApplicationId=$($parent.applicationId)","-PappControlParentFirebaseApiKey=$($parent.apiKey)","-PappControlParentFirebaseProjectId=$($parent.projectId)","-PappControlParentFirebaseSenderId=$($parent.senderId)")
}else{$arguments+='-PappControlParentFirebase=false'}
$childVariant='debug'
if($null -ne $child){
    $childVariant='fcmDebug'
    $arguments+=@('-PappControlFirebase=true',"-PappControlFirebaseApplicationId=$($child.applicationId)","-PappControlFirebaseApiKey=$($child.apiKey)","-PappControlFirebaseProjectId=$($child.projectId)","-PappControlFirebaseSenderId=$($child.senderId)")
}else{$arguments+='-PappControlFirebase=false'}
$childTask=if($childVariant -eq 'fcmDebug'){'FcmDebug'}else{'Debug'}
$arguments+=@(":parent-app:assemble$parentTask",":parent-app:lint$parentTask",":child-app:assemble$childTask",":child-app:lint$childTask")
$savedJava=$env:JAVA_HOME;$savedGradle=$env:GRADLE_USER_HOME
Push-Location (Join-Path $workspace 'android')
try{
    $env:JAVA_HOME=$java;$env:GRADLE_USER_HOME=Join-Path $workspace '.cache/gradle'
    $ErrorActionPreference='Continue'
    & $gradle @arguments
    $buildExit=$LASTEXITCODE
    $ErrorActionPreference='Stop'
    if($buildExit -ne 0){throw 'Wake-up test build failed; output is incomplete.'}
    $records=@()
    foreach($app in @('parent','child')){
        $variant=if($app -eq 'parent'){$parentVariant}else{$childVariant}
        $source=Join-Path $workspace "android/$app-app/build/outputs/apk/$variant/$app-app-$variant.apk"
        $destination=Join-Path $output "$app-debug.apk"
        [IO.File]::Copy($source,$destination,$false)
        $ErrorActionPreference='Continue';$badging=@(& $aapt dump badging $destination 2>&1);$inspectExit=$LASTEXITCODE;$ErrorActionPreference='Stop'
        $text=$badging -join "`n"
        $minimum=if($app -eq 'child'){28}else{33}
        if($inspectExit -ne 0 -or $text -notmatch ("(?m)^package: name='com\.appcontrol\."+$app+"' versionCode='"+$VersionCode+"'") -or
            $text -notmatch ("(?m)^sdkVersion:'"+$minimum+"'\r?$") -or $badging -notcontains 'application-debuggable'){throw 'Packaged debug identity or minimum SDK mismatch.'}
        if($app -eq 'parent'){& (Join-Path $PSScriptRoot 'assert-apk-parent-origin.ps1') -Apk $destination -ExpectedOrigin $ServerOrigin}
        $records+=@{name="$app-debug.apk";package="com.appcontrol.$app";variant=$variant;minSdk=$minimum;sha256=(Get-FileHash -LiteralPath $destination -Algorithm SHA256).Hash.ToLowerInvariant()}
    }
    $record=[ordered]@{schemaVersion=1;scope=$(if($developmentLocal){'laptop-development-debug-build'}else{'configured-debug-wakeup-test-build'});fixture=[bool]$AllowFixture;developmentCaUsed=($developmentCa -ne '');pollingOnly=[bool]$PollingOnly;productionReady=$false;liveFirebaseVerified=$false;installed=$false;remoteCallsEnabled=$false;privateUpdatesEnabled=$false;serverOrigin=$ServerOrigin;versionCode=$VersionCode;versionName=$VersionName;apps=$records}
    $path=Join-Path $output 'wakeup-build.json'
    $stream=[IO.File]::Open($path,[IO.FileMode]::CreateNew,[IO.FileAccess]::Write,[IO.FileShare]::None)
    try{$data=[Text.UTF8Encoding]::new($false).GetBytes(($record | ConvertTo-Json -Depth 6));$stream.Write($data,0,$data.Length)}finally{$stream.Dispose()}
    Write-Output 'Configured debug APK snapshots verified. No app installed, Firebase request made or service deployed.'
    & (Join-Path $PSScriptRoot 'archive-app-builds.ps1') -SnapshotDirectory $output -VersionName $VersionName
}finally{Pop-Location;$env:JAVA_HOME=$savedJava;$env:GRADLE_USER_HOME=$savedGradle}
