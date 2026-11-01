param(
    [Parameter(Mandatory)] [string]$ParentGoogleServicesFile,
    [string]$ChildGoogleServicesFile,
    [Parameter(Mandatory)] [string]$OutputDirectory
)
# Public Android settings only. Does not create projects, request tokens, build or install apps.
$ErrorActionPreference = 'Stop'
$maximumBytes = 262144
function Assert-PublicTree($Value,[int]$Depth=0) {
    if($Depth -gt 32){throw 'Firebase public JSON nesting exceeds its bound.'}
    if($Value -is [pscustomobject]){
        foreach($property in $Value.PSObject.Properties){
            if($property.Name -match '^(private[_-]?key(?:[_-]?id)?|client[_-]?secret|client[_-]?email|password|access[_-]?token|refresh[_-]?token|service[_-]?account)$'){
                throw 'Private credentials are not accepted as Android public settings.'
            }
            if($property.Name -ieq 'type' -and $property.Value -ieq 'service_account'){throw 'Service accounts are server-only; use the Android google-services.json file.'}
            Assert-PublicTree $property.Value ($Depth+1)
        }
    }elseif($Value -is [System.Collections.IEnumerable] -and $Value -isnot [string]){
        foreach($entry in $Value){Assert-PublicTree $entry ($Depth+1)}
    }
}
function Read-PublicJson([string]$Path) {
    $stream=$null
    try{
        $stream=[IO.File]::Open($Path,[IO.FileMode]::Open,[IO.FileAccess]::Read,[IO.FileShare]::Read)
        if($stream.Length -lt 1 -or $stream.Length -gt $maximumBytes){throw 'size'}
        $buffer=New-Object byte[] ($maximumBytes+1)
        $total=0
        while($total -lt $buffer.Length){$count=$stream.Read($buffer,$total,$buffer.Length-$total);if($count -eq 0){break};$total+=$count}
        if($total -gt $maximumBytes){throw 'size'}
        $text=[Text.UTF8Encoding]::new($false,$true).GetString($buffer,0,$total)
        $value=$text | ConvertFrom-Json
    }catch{throw 'Cannot read Android public JSON: require valid UTF-8 JSON of at most 256 KiB.'}
    finally{if($null -ne $stream){$stream.Dispose()}}
    Assert-PublicTree $value
    if($value -isnot [pscustomobject]){throw 'Expected an Android public settings object.'}
    $fields=@($value.PSObject.Properties.Name)
    if($fields.Count -ne 3 -or @($fields | Where-Object {$_ -cnotin @('project_info','client','configuration_version')}).Count -ne 0 -or $value.configuration_version -isnot [string] -or $value.configuration_version -cne '1'){
        throw 'Expected the Android google-services.json schema and configuration version 1.'
    }
    return $value
}
function Select-Settings($Config,[string]$Package) {
    $project=$Config.project_info
    if($project -isnot [pscustomobject] -or $project.project_number -isnot [string] -or $project.project_number -cnotmatch '\A[0-9]{1,32}\z' -or
        $project.project_id -isnot [string] -or $project.project_id -cnotmatch '\A[a-z0-9][a-z0-9-]{4,62}\z'){
        throw 'Invalid public Firebase project ID or project number.'
    }
    if($Config.client -isnot [array] -or $Config.client.Count -lt 1 -or $Config.client.Count -gt 64){throw 'Expected a bounded Android Firebase client array.'}
    $matches=@($Config.client | Where-Object {$_.client_info -is [pscustomobject] -and $_.client_info.android_client_info -is [pscustomobject] -and
        $_.client_info.android_client_info.package_name -is [string] -and $_.client_info.android_client_info.package_name -ceq $Package})
    if($matches.Count -ne 1){throw 'Require exactly one Firebase client for the requested App Control package.'}
    $selected=$matches[0]
    $application=$selected.client_info.mobilesdk_app_id
    if($application -isnot [string] -or $application -cnotmatch '\A1:[0-9]{1,32}:android:[a-fA-F0-9]+\z' -or $application.Length -gt 256 -or
        $application.Split(':')[1] -cne $project.project_number){throw 'Firebase Android application ID and project number must agree.'}
    if($selected.api_key -isnot [array] -or $selected.api_key.Count -ne 1){throw 'Require one unambiguous public Android Firebase API key.'}
    $apiKey=$selected.api_key[0].current_key
    if($apiKey -isnot [string] -or $apiKey -cnotmatch '\A[A-Za-z0-9_-]{20,128}\z'){throw 'Invalid public Android Firebase API key.'}
    return [ordered]@{applicationId=$application;apiKey=$apiKey;projectId=$project.project_id;senderId=$project.project_number}
}
function Write-NewJson([string]$Path,$Value) {
    $bytes=[Text.UTF8Encoding]::new($false).GetBytes(($Value | ConvertTo-Json -Depth 8))
    $file=[IO.File]::Open($Path,[IO.FileMode]::CreateNew,[IO.FileAccess]::Write,[IO.FileShare]::None)
    try{$file.Write($bytes,0,$bytes.Length);$file.Flush($true)}finally{$file.Dispose()}
}
$parent=Select-Settings (Read-PublicJson $ParentGoogleServicesFile) 'com.appcontrol.parent'
$child=$null
if(-not [string]::IsNullOrWhiteSpace($ChildGoogleServicesFile)){
    $child=Select-Settings (Read-PublicJson $ChildGoogleServicesFile) 'com.appcontrol.child'
    if($child.applicationId -ieq $parent.applicationId){throw 'Parent and child require distinct Firebase Android application IDs.'}
    if(($child.projectId -ceq $parent.projectId) -ne ($child.senderId -ceq $parent.senderId)){throw 'Parent and child Firebase project IDs and project numbers are inconsistent.'}
}
$arguments=@('-PappControlParentFirebase=true',('-PappControlParentFirebaseApplicationId='+$parent.applicationId),
    ('-PappControlParentFirebaseApiKey='+$parent.apiKey),('-PappControlParentFirebaseProjectId='+$parent.projectId),
    ('-PappControlParentFirebaseSenderId='+$parent.senderId))
$tasks=@(':parent-app:assembleFcmDebug')
if($null -ne $child){
    $arguments+=@('-PappControlFirebase=true',('-PappControlFirebaseApplicationId='+$child.applicationId),
        ('-PappControlFirebaseApiKey='+$child.apiKey),('-PappControlFirebaseProjectId='+$child.projectId),
        ('-PappControlFirebaseSenderId='+$child.senderId))
    $tasks+=':child-app:assembleFcmDebug'
}else{$arguments+='-PappControlFirebase=false'}
try{
    $output=[IO.Path]::GetFullPath($OutputDirectory)
    if(Test-Path -LiteralPath $output){throw 'exists'}
    $null=New-Item -ItemType Directory -Path $output -ErrorAction Stop
}catch{throw 'Firebase import output directory must be a new writable directory.'}
try{
    Write-NewJson (Join-Path $output 'parent-firebase-public.json') $parent
    if($null -ne $child){Write-NewJson (Join-Path $output 'child-firebase-public.json') $child}
    Write-NewJson (Join-Path $output 'debug-arguments.json') ([ordered]@{arguments=$arguments;tasks=$tasks})
    $files=@(Get-ChildItem -LiteralPath $output -File | Sort-Object Name | ForEach-Object {[ordered]@{name=$_.Name;sha256=(Get-FileHash -LiteralPath $_.FullName -Algorithm SHA256).Hash.ToLowerInvariant()}})
    Write-NewJson (Join-Path $output 'firebase-import.json') ([ordered]@{
        schemaVersion=1;scope='public-firebase-settings-import';structuralValidationPassed=$true;productionReady=$false
        firebaseRegistrationVerified=$false;liveDeliveryVerified=$false;files=$files
    })
}catch{throw 'Firebase public export failed; do not use output without its completion marker.'}
Write-Output 'Public Firebase settings imported for App Control test builds. Project ownership, server configuration, signing and live delivery remain unverified.'
