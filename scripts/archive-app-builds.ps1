param(
    [Parameter(Mandatory)] [string]$SnapshotDirectory,
    [string]$VersionName,
    [string]$ArchiveDirectory=(Join-Path (Split-Path -Parent $PSScriptRoot) 'releases/app-history')
)
# Preserve verified snapshots by version AND content. Never replace an archived APK.
$ErrorActionPreference='Stop'
$workspace=Split-Path -Parent $PSScriptRoot
function Safe-Path([string]$Value) {
    if([string]::IsNullOrWhiteSpace($Value) -or $Value.StartsWith('\\') -or $Value -match '[\x00-\x1f]'){throw 'Canonical local paths are required.'}
    foreach($part in $Value.Split([char[]]'\/')){
        if($part -eq '..' -or ($part -ne '.' -and $part -match '[. ]$') -or
            $part -match '^(?i:con|prn|aux|nul|com[1-9]|lpt[1-9])(?:\.|$)' -or
            ($part.Contains(':') -and $part -notmatch '^[A-Za-z]:$')){throw 'Aliased or traversing paths are refused.'}
    }
    $full=[IO.Path]::GetFullPath($Value).TrimEnd([char[]]'\/')
    $cursor=$full
    while(-not [string]::IsNullOrEmpty($cursor)){
        if(Test-Path -LiteralPath $cursor){
            if(((Get-Item -LiteralPath $cursor -Force).Attributes -band [IO.FileAttributes]::ReparsePoint) -ne 0){throw 'Reparse-point paths are refused.'}
        }
        $parent=[IO.Path]::GetDirectoryName($cursor)
        if($parent -ceq $cursor){break};$cursor=$parent
    }
    return $full
}
function Hash([string]$Path){return (Get-FileHash -LiteralPath $Path -Algorithm SHA256).Hash.ToLowerInvariant()}
function Read-Record([string]$Path){
    $null=Safe-Path $Path
    $file=Get-Item -LiteralPath $Path
    if($file -isnot [IO.FileInfo] -or $file.Length -gt 32768){throw 'A bounded JSON record is required.'}
    try{return ([IO.File]::ReadAllText($file.FullName,[Text.UTF8Encoding]::new($false,$true)) | ConvertFrom-Json)}catch{throw 'Invalid archive input record.'}
}
function Keys($Value,[string[]]$Allowed){
    foreach($property in $Value.PSObject.Properties){if($property.Name -cnotin $Allowed){throw 'Unknown metadata fields are refused.'}}
}
function Write-NewRecord([string]$Path,$Value){
    $null=Safe-Path $Path
    $stream=[IO.File]::Open($Path,[IO.FileMode]::CreateNew,[IO.FileAccess]::Write,[IO.FileShare]::None)
    try{$bytes=[Text.UTF8Encoding]::new($false).GetBytes(($Value|ConvertTo-Json -Depth 4));$stream.Write($bytes,0,$bytes.Length);$stream.Flush($true)}finally{$stream.Dispose()}
    [IO.File]::SetAttributes($Path,([IO.File]::GetAttributes($Path) -bor [IO.FileAttributes]::ReadOnly))
}
$snapshot=Safe-Path $SnapshotDirectory
if(-not (Test-Path -LiteralPath $snapshot -PathType Container)){throw 'Snapshot directory is missing.'}
$record=Read-Record (Join-Path $snapshot 'wakeup-build.json')
Keys $record @('schemaVersion','scope','fixture','developmentCaUsed','pollingOnly','productionReady','liveFirebaseVerified','installed','remoteCallsEnabled','privateUpdatesEnabled','serverOrigin','versionCode','versionName','apps')
if($record.schemaVersion -ne 1 -or $record.scope -cnotin @('laptop-development-debug-build','configured-debug-wakeup-test-build') -or
    $record.versionCode -isnot [ValueType] -or $record.versionCode -is [bool] -or $record.versionCode -lt 1 -or $record.versionCode -gt 2100000000 -or
    [long]$record.versionCode -ne $record.versionCode -or @($record.apps).Count -ne 2){throw 'Unsupported snapshot metadata.'}
if($record.fixture -isnot [bool] -or $record.developmentCaUsed -isnot [bool] -or $record.productionReady -isnot [bool] -or $record.productionReady){throw 'Explicit development build context is required.'}
$origin=$null
if($record.serverOrigin -isnot [string] -or $record.serverOrigin.Length -gt 2048 -or
    -not [Uri]::TryCreate($record.serverOrigin,[UriKind]::Absolute,[ref]$origin) -or $origin.Scheme -cne 'https' -or
    $origin.UserInfo -ne '' -or $origin.AbsolutePath -ne '/' -or $origin.Query -ne '' -or $origin.Fragment -ne ''){throw 'A public HTTPS build origin is required.'}
$buildContext=[ordered]@{sourceBuildScope=$record.scope;fixture=$record.fixture;serverOrigin=$record.serverOrigin;developmentCaUsed=$record.developmentCaUsed;productionReady=$false}
if([string]::IsNullOrWhiteSpace($VersionName)){$VersionName=$record.versionName}
if($VersionName -cnotmatch '\A[A-Za-z0-9][A-Za-z0-9._+\-]{0,63}\z' -or
    ($null -ne $record.versionName -and $record.versionName -cne $VersionName)){throw 'Supply the matching public VersionName for older build reports.'}
$archive=Safe-Path $ArchiveDirectory
if(-not $archive.StartsWith(($workspace+[IO.Path]::DirectorySeparatorChar),[StringComparison]::OrdinalIgnoreCase)){throw 'Archive must stay inside this workspace.'}
$inputs=@();$names=[Collections.Generic.HashSet[string]]::new([StringComparer]::Ordinal)
foreach($app in $record.apps){
    Keys $app @('name','package','variant','minSdk','sha256')
    if($app.name -cnotin @('parent-debug.apk','child-debug.apk') -or -not $names.Add($app.name)){throw 'Exactly one parent and child APK are required.'}
    $role=if($app.name -ceq 'parent-debug.apk'){'parent'}else{'child'}
    if($app.package -cne ('com.appcontrol.'+$role) -or $app.variant -cnotin @('debug','fcmDebug') -or
        $app.sha256 -cnotmatch '\A[a-f0-9]{64}\z' -or $app.minSdk -isnot [ValueType] -or $app.minSdk -is [bool] -or
        $app.minSdk -lt 1 -or $app.minSdk -gt 2100000000 -or [long]$app.minSdk -ne $app.minSdk){throw 'APK identity metadata is invalid.'}
    $source=Safe-Path (Join-Path $snapshot $app.name)
    $file=Get-Item -LiteralPath $source
    if($file -isnot [IO.FileInfo] -or $file.Length -lt 1 -or $file.Length -gt 268435456 -or (Hash $source) -cne $app.sha256){throw 'Snapshot APK bytes do not match their inventory.'}
    $inputs+=@{app=$app;role=$role;source=$source;size=$file.Length}
}
$null=[IO.Directory]::CreateDirectory($archive);$null=Safe-Path $archive
$lockPath=Safe-Path (Join-Path $archive '.archive.lock')
$lock=[IO.File]::Open($lockPath,[IO.FileMode]::OpenOrCreate,[IO.FileAccess]::ReadWrite,[IO.FileShare]::None)
try{
    $results=@()
    foreach($snapshotApp in $inputs){
        $app=$snapshotApp.app
        $version=Join-Path (Join-Path $archive $snapshotApp.role) ($record.versionCode.ToString()+'-'+$VersionName)
        $null=Safe-Path $version;$null=[IO.Directory]::CreateDirectory($version)
        $existing=@(Get-ChildItem -LiteralPath $version -Force -Directory | Where-Object {$_.Name.EndsWith('-'+$app.sha256,[StringComparison]::Ordinal)})
        if($existing.Count -gt 1){throw 'Duplicate archive content records require review.'}
        if($existing.Count -eq 1){
            $entry=Safe-Path $existing[0].FullName
            $saved=Read-Record (Join-Path $entry 'archive.json')
            Keys $saved @('schemaVersion','scope','createdAtUtc','role','package','variant','versionCode','versionName','minSdk','apk','bytes','sha256','sourceBuildScope','fixture','serverOrigin','developmentCaUsed','productionReady')
            $savedApk=Safe-Path (Join-Path $entry $app.name)
            if($saved.schemaVersion -ne 1 -or $saved.scope -cne 'immutable-app-build-history' -or
                $saved.role -cne $snapshotApp.role -or $saved.package -cne $app.package -or $saved.variant -cne $app.variant -or
                $saved.versionCode -ne $record.versionCode -or $saved.versionName -cne $VersionName -or $saved.minSdk -ne $app.minSdk -or
                $saved.apk -cne $app.name -or $saved.bytes -ne $snapshotApp.size -or $saved.sha256 -cne $app.sha256 -or
                (Get-Item -LiteralPath $savedApk).Length -ne $snapshotApp.size -or (Hash $savedApk) -cne $app.sha256){throw 'Existing archive is incomplete or changed; nothing will be overwritten.'}
            $savedContext=$saved
            if($saved.PSObject.Properties.Name -cnotcontains 'sourceBuildScope'){
                # Older immutable inventories stay untouched. Add context once in
                # an exclusive, read-only sidecar instead of rewriting history.
                $contextPath=Safe-Path (Join-Path $entry 'build-context.json')
                if(-not (Test-Path -LiteralPath $contextPath)){Write-NewRecord $contextPath $buildContext}
                $savedContext=Read-Record $contextPath
                Keys $savedContext @('sourceBuildScope','fixture','serverOrigin','developmentCaUsed','productionReady')
            }
            foreach($key in $buildContext.Keys){if($savedContext.$key -cne $buildContext[$key]){throw 'Existing archive build context differs; nothing will be overwritten.'}}
            $results+=@{role=$snapshotApp.role;status='reused';directory=$entry;sha256=$app.sha256}
            continue
        }
        $entry=Join-Path $version ([datetime]::UtcNow.ToString('yyyyMMddTHHmmssfffffffZ')+'-'+$app.sha256)
        $null=Safe-Path $entry;$null=New-Item -ItemType Directory -Path $entry
        $destination=Join-Path $entry $app.name
        [IO.File]::Copy($snapshotApp.source,$destination,$false)
        if((Hash $destination) -cne $app.sha256 -or (Hash $snapshotApp.source) -cne $app.sha256){throw 'APK bytes changed during archival; incomplete output was preserved.'}
        $metadata=[ordered]@{schemaVersion=1;scope='immutable-app-build-history';createdAtUtc=[datetime]::UtcNow.ToString('o');role=$snapshotApp.role
            package=$app.package;variant=$app.variant;versionCode=$record.versionCode;versionName=$VersionName;minSdk=$app.minSdk
            apk=$app.name;bytes=$snapshotApp.size;sha256=$app.sha256
            sourceBuildScope=$record.scope;fixture=$record.fixture;serverOrigin=$record.serverOrigin;developmentCaUsed=$record.developmentCaUsed;productionReady=$false}
        $completion=Join-Path $entry 'archive.json'
        Write-NewRecord $completion $metadata
        foreach($path in @($destination,$completion)){[IO.File]::SetAttributes($path,([IO.File]::GetAttributes($path) -bor [IO.FileAttributes]::ReadOnly))}
        $results+=@{role=$snapshotApp.role;status='archived';directory=$entry;sha256=$app.sha256}
    }
    [ordered]@{schemaVersion=1;scope='app-build-history-result';versionCode=$record.versionCode;versionName=$VersionName;apps=$results}|ConvertTo-Json -Depth 5
}finally{$lock.Dispose()}
