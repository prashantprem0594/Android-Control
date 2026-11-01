$ErrorActionPreference='Stop'
$workspace=Split-Path -Parent $PSScriptRoot
$root=Join-Path $workspace ('.cache/app-history-tests-'+[guid]::NewGuid().ToString('N'))
$null=New-Item -ItemType Directory -Path $root
$archiver=Join-Path $PSScriptRoot 'archive-app-builds.ps1'
$checks=0
function Write-Json([string]$Path,$Value){[IO.File]::WriteAllText($Path,($Value|ConvertTo-Json -Depth 6),[Text.UTF8Encoding]::new($false))}
function Fixture([string]$Name,[string]$Contents='initial',[int]$Version=1){
    $folder=Join-Path $root $Name;$null=New-Item -ItemType Directory -Path $folder
    $apps=@()
    foreach($role in @('parent','child')){
        $file=Join-Path $folder ($role+'-debug.apk');[IO.File]::WriteAllText($file,($role+$Contents))
        $apps+=@{name=($role+'-debug.apk');package=('com.appcontrol.'+$role);variant='fcmDebug';minSdk=$(if($role -ceq 'child'){28}else{33});sha256=(Get-FileHash -LiteralPath $file -Algorithm SHA256).Hash.ToLowerInvariant()}
    }
    Write-Json (Join-Path $folder 'wakeup-build.json') @{schemaVersion=1;scope='laptop-development-debug-build';fixture=$true;serverOrigin='https://archive-fixture.invalid';developmentCaUsed=$true;productionReady=$false;versionCode=$Version;apps=$apps}
    return $folder
}
function Invoke-Archive([string]$Snapshot,[string]$Name='0.1.0',[string]$Destination=(Join-Path $root 'history')){
    return (& $archiver -SnapshotDirectory $Snapshot -VersionName $Name -ArchiveDirectory $Destination | ConvertFrom-Json)
}
function Check([bool]$Condition){if(-not $Condition){throw 'Archive regression failed.'};$script:checks++}
function Refuses([scriptblock]$Action){$refused=$false;try{& $Action|Out-Null}catch{$refused=$true};Check $refused}
$first=Fixture 'first'
$initial=Invoke-Archive $first
Check (@($initial.apps|Where-Object status -ceq 'archived').Count -eq 2)
$repeat=Invoke-Archive $first
Check (@($repeat.apps|Where-Object status -ceq 'reused').Count -eq 2)
foreach($app in $initial.apps){
    $record=Get-Content (Join-Path $app.directory 'archive.json') -Raw|ConvertFrom-Json
    Check ($record.serverOrigin -ceq 'https://archive-fixture.invalid' -and $record.fixture -and -not $record.productionReady)
    Check (((Get-Item -LiteralPath (Join-Path $app.directory ($app.role+'-debug.apk'))).Attributes -band [IO.FileAttributes]::ReadOnly) -ne 0)
}
$second=Fixture 'same-version-new-bytes' 'changed'
$changed=Invoke-Archive $second
Check (@($changed.apps|Where-Object status -ceq 'archived').Count -eq 2)
foreach($app in $initial.apps){Check (Test-Path -LiteralPath (Join-Path $app.directory 'archive.json'))}
Check (@(Get-ChildItem -LiteralPath (Join-Path $root 'history/parent/1-0.1.0') -Directory).Count -eq 2)
$next=Fixture 'next-version' 'next' 2
$newVersion=Invoke-Archive $next '0.2.0'
Check (@($newVersion.apps|Where-Object status -ceq 'archived').Count -eq 2)
Refuses {& $archiver -SnapshotDirectory $first -ArchiveDirectory (Join-Path $root 'missing-version')}
Refuses {Invoke-Archive $first '../escape'}
Refuses {Invoke-Archive $first '0.1.0' ($root+'/../escape')}
$tampered=Fixture 'tampered'
[IO.File]::AppendAllText((Join-Path $tampered 'child-debug.apk'),'changed')
Refuses {Invoke-Archive $tampered}
$private=Fixture 'private-field'
$manifest=Get-Content (Join-Path $private 'wakeup-build.json') -Raw|ConvertFrom-Json
$manifest|Add-Member -NotePropertyName private_key -NotePropertyValue 'synthetic-not-secret'
Write-Json (Join-Path $private 'wakeup-build.json') $manifest
Refuses {Invoke-Archive $private}
$traversal=Fixture 'traversal'
$manifest=Get-Content (Join-Path $traversal 'wakeup-build.json') -Raw|ConvertFrom-Json
$manifest.apps[0].name='../parent-debug.apk';Write-Json (Join-Path $traversal 'wakeup-build.json') $manifest
Refuses {Invoke-Archive $traversal}
$duplicate=Fixture 'duplicate'
$manifest=Get-Content (Join-Path $duplicate 'wakeup-build.json') -Raw|ConvertFrom-Json
$manifest.apps[1]=$manifest.apps[0];Write-Json (Join-Path $duplicate 'wakeup-build.json') $manifest
Refuses {Invoke-Archive $duplicate}
$mismatch=Fixture 'version-mismatch'
$manifest=Get-Content (Join-Path $mismatch 'wakeup-build.json') -Raw|ConvertFrom-Json
$manifest|Add-Member -NotePropertyName versionName -NotePropertyValue '0.2.0';Write-Json (Join-Path $mismatch 'wakeup-build.json') $manifest
Refuses {Invoke-Archive $mismatch}
$large=Fixture 'oversized-json'
[IO.File]::WriteAllText((Join-Path $large 'wakeup-build.json'),(' '*32769))
Refuses {Invoke-Archive $large}
$sourceJunction=Join-Path $root 'source-junction'
$null=New-Item -ItemType Junction -Path $sourceJunction -Target $first
Refuses {Invoke-Archive $sourceJunction}
$archiveJunction=Join-Path $root 'archive-junction'
$null=New-Item -ItemType Junction -Path $archiveJunction -Target (Join-Path $root 'history')
Refuses {Invoke-Archive $first '0.1.0' $archiveJunction}
$future=Fixture 'future-minimum' 'future'
$manifest=Get-Content (Join-Path $future 'wakeup-build.json') -Raw|ConvertFrom-Json
$manifest.apps[0].minSdk=34;$manifest.apps[1].minSdk=29;Write-Json (Join-Path $future 'wakeup-build.json') $manifest
$futureResult=Invoke-Archive $future
Check (@($futureResult.apps|Where-Object status -ceq 'archived').Count -eq 2)
$legacy=Fixture 'legacy' 'legacy'
$legacyResult=Invoke-Archive $legacy
foreach($app in $legacyResult.apps){
    $recordPath=Join-Path $app.directory 'archive.json'
    $metadata=Get-Content -LiteralPath $recordPath -Raw|ConvertFrom-Json
    foreach($key in @('sourceBuildScope','fixture','serverOrigin','developmentCaUsed','productionReady')){$metadata.PSObject.Properties.Remove($key)}
    [IO.File]::SetAttributes($recordPath,[IO.FileAttributes]::Normal);Write-Json $recordPath $metadata
    [IO.File]::SetAttributes($recordPath,[IO.FileAttributes]::ReadOnly)
}
$legacyBefore=@{};foreach($app in $legacyResult.apps){$legacyBefore[$app.role]=(Get-FileHash -LiteralPath (Join-Path $app.directory 'archive.json') -Algorithm SHA256).Hash}
$legacyReused=Invoke-Archive $legacy
Check (@($legacyReused.apps|Where-Object status -ceq 'reused').Count -eq 2)
foreach($app in $legacyResult.apps){
    Check ((Get-FileHash -LiteralPath (Join-Path $app.directory 'archive.json') -Algorithm SHA256).Hash -ceq $legacyBefore[$app.role])
    $sidecar=Join-Path $app.directory 'build-context.json'
    Check (((Get-Item -LiteralPath $sidecar).Attributes -band [IO.FileAttributes]::ReadOnly) -ne 0)
}
$null=Invoke-Archive $legacy
$sidecar=Join-Path $legacyResult.apps[0].directory 'build-context.json'
[IO.File]::SetAttributes($sidecar,[IO.FileAttributes]::Normal)
$badContext=Get-Content -LiteralPath $sidecar -Raw|ConvertFrom-Json;$badContext.fixture=$false;Write-Json $sidecar $badContext
Refuses {Invoke-Archive $legacy}
$damaged=Join-Path $initial.apps[0].directory ($initial.apps[0].role+'-debug.apk')
[IO.File]::SetAttributes($damaged,[IO.FileAttributes]::Normal)
[IO.File]::AppendAllText($damaged,'corrupt')
Refuses {Invoke-Archive $first}
Write-Output ("App history checks passed: $checks; new versions/content retained, exact repeats reused, hashes and metadata validated, unsafe/incomplete history refused. Fixtures remain private under .cache.")
