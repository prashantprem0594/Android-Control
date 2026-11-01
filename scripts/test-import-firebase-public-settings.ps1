# Run through run-logged.ps1. Synthetic public fixtures only; no Firebase requests or APK builds.
$ErrorActionPreference='Stop'
$workspace=Split-Path -Parent $PSScriptRoot
$root=Join-Path $workspace ('.cache/firebase-import-checks-'+[guid]::NewGuid().ToString('N'))
$null=New-Item -ItemType Directory -Path $root
$importer=Join-Path $PSScriptRoot 'import-firebase-public-settings.ps1'
function Fixture([string]$Package='com.appcontrol.parent',[string]$Application='1:12345:android:abcdef'){
    return [ordered]@{project_info=[ordered]@{project_number='12345';project_id='firebase-import-fixture';storage_bucket='fixture.example'}
        client=@([ordered]@{client_info=[ordered]@{mobilesdk_app_id=$Application;android_client_info=[ordered]@{package_name=$Package}}
            api_key=@([ordered]@{current_key=('k'*24)});oauth_client=@();services=[ordered]@{}});configuration_version='1'}
}
function Save($Value){$file=Join-Path $root ([guid]::NewGuid().ToString('N')+'.json');[IO.File]::WriteAllText($file,($Value | ConvertTo-Json -Depth 12),[Text.UTF8Encoding]::new($false));return $file}
function Refuses([string]$Parent,[string]$Child=''){
    $target=Join-Path $root ([guid]::NewGuid().ToString('N'))
    $refused=$false;$message=''
    try{& $importer -ParentGoogleServicesFile $Parent -ChildGoogleServicesFile $Child -OutputDirectory $target | Out-Null}catch{$refused=$true;$message=$_.Exception.Message}
    if(-not $refused -or (Test-Path -LiteralPath $target)){throw 'Invalid Firebase input was exported.'}
    if($message.Contains('SECRET_SENTINEL') -or $message.Contains($Parent) -or ($Child -ne '' -and $message.Contains($Child))){throw 'Refusal disclosed input contents or paths.'}
}
$parentFile=Save (Fixture)
$childFile=Save (Fixture 'com.appcontrol.child' '1:12345:android:fedcba')
$one=Join-Path $root 'parent-only'
$output=@(& $importer -ParentGoogleServicesFile $parentFile -OutputDirectory $one)
if(($output -join ' ').Contains(('k'*24)) -or ($output -join ' ').Contains($parentFile)){throw 'Importer printed public input values or paths.'}
$settings=Get-Content (Join-Path $one 'parent-firebase-public.json') -Raw | ConvertFrom-Json
if(@($settings.PSObject.Properties).Count -ne 4 -or $settings.applicationId -cne '1:12345:android:abcdef' -or $settings.senderId -cne '12345'){throw 'Wrong extracted parent settings.'}
$build=Get-Content (Join-Path $one 'debug-arguments.json') -Raw | ConvertFrom-Json
if($build.arguments -notcontains '-PappControlFirebase=false' -or $build.tasks.Count -ne 1 -or $build.tasks[0] -cne ':parent-app:assembleFcmDebug'){throw 'Wrong parent-only build arguments.'}
$both=Join-Path $root 'both'
& $importer -ParentGoogleServicesFile $parentFile -ChildGoogleServicesFile $childFile -OutputDirectory $both | Out-Null
$build=Get-Content (Join-Path $both 'debug-arguments.json') -Raw | ConvertFrom-Json
if($build.arguments -notcontains '-PappControlFirebaseApplicationId=1:12345:android:fedcba' -or $build.arguments -notcontains '-PappControlParentFirebaseApplicationId=1:12345:android:abcdef' -or $build.tasks -notcontains ':child-app:assembleFcmDebug'){throw 'Parent/child settings mixed.'}
foreach($folder in @($one,$both)){$manifest=Get-Content (Join-Path $folder 'firebase-import.json') -Raw | ConvertFrom-Json;if($manifest.productionReady -or $manifest.liveDeliveryVerified -or -not $manifest.structuralValidationPassed){throw 'Import overclaimed acceptance.'};foreach($file in $manifest.files){if((Get-FileHash (Join-Path $folder $file.name) -Algorithm SHA256).Hash.ToLowerInvariant() -cne $file.sha256){throw 'Import hash mismatch.'}}}
$refused=$false;try{& $importer -ParentGoogleServicesFile $parentFile -OutputDirectory $one | Out-Null}catch{$refused=$true};if(-not $refused){throw 'Existing output was overwritten.'}
$value=Fixture 'another.package';Refuses (Save $value)
$value=Fixture;$value.client+=@($value.client[0]);Refuses (Save $value)
$value=Fixture;$value.client[0].api_key+=@([ordered]@{current_key=('z'*24)});Refuses (Save $value)
$value=Fixture;$value.client[0].client_info.mobilesdk_app_id='1:99999:android:abcdef';Refuses (Save $value)
$value=Fixture;$value.project_info.project_number=12345;Refuses (Save $value)
$value=Fixture;$value.client[0].api_key[0].current_key='short';Refuses (Save $value)
$value=Fixture;$value.client[0].api_key[0].current_key=('k'*24)+"`n";Refuses (Save $value)
$value=Fixture;$value.client[0].client_info.android_client_info.package_name=@('com.appcontrol.parent','another.package');Refuses (Save $value)
$value=Fixture;$value.configuration_version='2';Refuses (Save $value)
$value=Fixture;$value.configuration_version=1;Refuses (Save $value)
$value=Fixture;$value.project_info.private_key='SECRET_SENTINEL';Refuses (Save $value)
Refuses (Save ([ordered]@{type='service_account';private_key='SECRET_SENTINEL';client_email='SECRET_SENTINEL'}))
$value=Fixture;$value.client[0].services.password='SECRET_SENTINEL';Refuses (Save $value)
Refuses $parentFile (Save (Fixture 'com.appcontrol.child' '1:12345:android:abcdef'))
$value=Fixture 'com.appcontrol.child' '1:12345:android:fedcba';$value.project_info.project_id='different-project';Refuses $parentFile (Save $value)
$bad=Join-Path $root 'malformed.json';[IO.File]::WriteAllText($bad,'{"SECRET_SENTINEL":');Refuses $bad
$big=Join-Path $root 'oversized.json';[IO.File]::WriteAllText($big,(' '*262145));Refuses $big
$invalid=Join-Path $root 'invalid-utf8.json';[IO.File]::WriteAllBytes($invalid,[byte[]]@(0xff,0xfe,0xff));Refuses $invalid
Refuses (Join-Path $root 'missing.json')
$mixed=Fixture;$mixed.client+=@( (Fixture 'com.appcontrol.child' '1:12345:android:fedcba').client[0] )
$multi=Save $mixed
& $importer -ParentGoogleServicesFile $multi -ChildGoogleServicesFile $multi -OutputDirectory (Join-Path $root 'shared-project-file') | Out-Null
Write-Output 'Firebase public import checks passed: exact-package selection, independent app IDs, bounded UTF-8 JSON, private-input/refusal redaction, key ambiguity, output preservation and hashes. No project registration or delivery tested.'
