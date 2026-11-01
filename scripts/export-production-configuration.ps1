param(
    [Parameter(Mandatory)] [string]$ConfigurationFile,
    [string]$OutputDirectory,
    [switch]$AllowFixture,
    [switch]$ValidateOnly
)
# Invoke through run-logged.ps1. Public settings only; no builds or deployment.
$ErrorActionPreference = 'Stop'
function Require-Fields($Object,[string[]]$Names) {
    if ($null -eq $Object -or $Object -isnot [pscustomobject]) { throw 'Expected a public configuration object.' }
    $actual = @($Object.PSObject.Properties.Name)
    if ($actual.Count -ne $Names.Count -or @($actual | Where-Object { $_ -cnotin $Names }).Count) { throw 'Unexpected or missing public configuration fields.' }
}
function Require-Integer($Value,[long]$Minimum,[long]$Maximum) {
    if (($Value -isnot [int] -and $Value -isnot [long]) -or $Value -lt $Minimum -or $Value -gt $Maximum) { throw 'Invalid public configuration integer.' }
}
function Decode-PublicBase64($Value,[int]$Length) {
    if ($Value -isnot [string]) { throw 'Invalid public Base64 field.' }
    try { $decoded = [Convert]::FromBase64String($Value) } catch { throw 'Invalid public Base64 field.' }
    if ($decoded.Length -ne $Length -or [Convert]::ToBase64String($decoded) -cne $Value) { throw 'Noncanonical public Base64 field.' }
    return ,$decoded
}
function Write-NewText([string]$Path,[string]$Value) {
    $bytes = [Text.UTF8Encoding]::new($false).GetBytes($Value)
    $file = [IO.File]::Open($Path,[IO.FileMode]::CreateNew,[IO.FileAccess]::Write,[IO.FileShare]::None)
    try { $file.Write($bytes,0,$bytes.Length);$file.Flush($true) } finally { $file.Dispose() }
}
function Write-JSON([string]$Path,$Object) {
    Write-NewText $Path ($Object | ConvertTo-Json -Depth 8)
}
$inputFile = Get-Item -LiteralPath $ConfigurationFile
if ($inputFile -isnot [IO.FileInfo] -or $inputFile.Length -gt 16384) { throw 'Public configuration must be a file of at most 16 KiB.' }
try { $config = [IO.File]::ReadAllText($inputFile.FullName,[Text.UTF8Encoding]::new($false,$true)) | ConvertFrom-Json } catch { throw 'Invalid UTF-8 public configuration JSON.' }
$rootFields=@('serverOrigin','currentPin','backupPin','apkCertificateSHA256','updatePublicKey','updateMinimumSequence','versionCode','versionName','remoteCallsEnabled','firebase')
if($config.PSObject.Properties.Name -ccontains 'parentFirebase'){$rootFields+='parentFirebase'}
Require-Fields $config $rootFields
$parentFirebase=if($config.PSObject.Properties.Name -ccontains 'parentFirebase'){$config.parentFirebase}else{$null}
if ($config.serverOrigin -isnot [string] -or $config.serverOrigin -cnotmatch '^https://[a-z0-9.-]+(?::[0-9]+)?$') { throw 'Use a canonical lowercase HTTPS DNS origin without path, credentials, query or fragment.' }
$origin = [uri]$config.serverOrigin
$hostName = $origin.DnsSafeHost
if ($origin.HostNameType -ne [UriHostNameType]::Dns -or $hostName.Length -gt 253 -or $hostName -notmatch '[a-z]' -or $hostName -notmatch '\.' -or $origin.Port -notin 1..65535) { throw 'Invalid HTTPS DNS origin.' }
foreach ($label in $hostName.Split('.')) { if ($label.Length -notin 1..63 -or $label -cnotmatch '^[a-z0-9](?:[a-z0-9-]*[a-z0-9])?$') { throw 'Invalid DNS label.' } }
$canonical = 'https://' + $hostName + $(if ($origin.Port -ne 443) { ':' + $origin.Port } else { '' })
if ($canonical -cne $config.serverOrigin) { throw 'Use canonical origin spelling; omit the default port.' }
$reserved = $hostName -match '(^|\.)(invalid|test|example|localhost|local|internal)$|(^|\.)example\.(com|net|org)$'
if ($reserved -and -not $AllowFixture) { throw 'Reserved/test host requires explicit AllowFixture; no production export performed.' }
$null = Decode-PublicBase64 $config.currentPin 32
$null = Decode-PublicBase64 $config.backupPin 32
if ($config.currentPin -ceq $config.backupPin) { throw 'Current and backup TLS pins must be distinct.' }
if ($config.apkCertificateSHA256 -isnot [string] -or $config.apkCertificateSHA256 -cnotmatch '^[a-f0-9]{64}$') { throw 'Expected lowercase APK certificate SHA-256.' }
$key = Decode-PublicBase64 $config.updatePublicKey 44
$prefix = [BitConverter]::ToString($key[0..11]).Replace('-','').ToLowerInvariant()
if ($prefix -cne '302a300506032b6570032100' -or @($key[12..43] | Where-Object { $_ -ne 0 }).Count -eq 0) { throw 'Expected public Ed25519 SubjectPublicKeyInfo DER.' }
Require-Integer $config.updateMinimumSequence 0 2100000000
Require-Integer $config.versionCode 1 2100000000
if ($config.versionName -isnot [string] -or $config.versionName -cnotmatch '^[A-Za-z0-9][A-Za-z0-9._+\-]{0,63}$' -or $config.remoteCallsEnabled -isnot [bool]) { throw 'Invalid version name or remote-call flag.' }
$properties = [ordered]@{
    appControlServerOrigin=$canonical; appControlCurrentPin=$config.currentPin; appControlBackupPin=$config.backupPin
    appControlUpdatePublicKey=$config.updatePublicKey; appControlUpdateCertificateSHA256=$config.apkCertificateSHA256
    appControlUpdateMinimumSequence=$config.updateMinimumSequence; appControlVersionCode=$config.versionCode
    appControlVersionName=$config.versionName; appControlRemoteCallsEnabled=$config.remoteCallsEnabled.ToString().ToLowerInvariant()
    appControlFirebase=($null -ne $config.firebase).ToString().ToLowerInvariant()
    appControlParentFirebase=($null -ne $parentFirebase).ToString().ToLowerInvariant()
}
if ($null -ne $config.firebase) {
    Require-Fields $config.firebase @('applicationId','apiKey','projectId','senderId')
    $patterns = @{applicationId='^1:[0-9]+:android:[a-fA-F0-9]+$';apiKey='^[A-Za-z0-9_-]{20,128}$';projectId='^[a-z0-9][a-z0-9-]{4,62}$';senderId='^[0-9]{1,32}$'}
    foreach ($name in $patterns.Keys) { if ($config.firebase.$name -isnot [string] -or $config.firebase.$name -cnotmatch $patterns[$name]) { throw 'Invalid public Firebase field.' } }
    if ($config.firebase.applicationId.Split(':')[1] -cne $config.firebase.senderId) { throw 'Firebase sender/application IDs differ.' }
    $properties.appControlFirebaseApplicationId = $config.firebase.applicationId
    $properties.appControlFirebaseApiKey = $config.firebase.apiKey
    $properties.appControlFirebaseProjectId = $config.firebase.projectId
    $properties.appControlFirebaseSenderId = $config.firebase.senderId
}
if($null -ne $parentFirebase){
    Require-Fields $parentFirebase @('applicationId','apiKey','projectId','senderId')
    $patterns=@{applicationId='^1:[0-9]+:android:[a-fA-F0-9]+$';apiKey='^[A-Za-z0-9_-]{20,128}$';projectId='^[a-z0-9][a-z0-9-]{4,62}$';senderId='^[0-9]{1,32}$'}
    foreach($name in $patterns.Keys){if($parentFirebase.$name -isnot [string] -or $parentFirebase.$name -cnotmatch $patterns[$name]){throw 'Invalid public parent Firebase field.'}}
    if($parentFirebase.applicationId.Split(':')[1] -cne $parentFirebase.senderId){throw 'Parent Firebase sender/application IDs differ.'}
    if($null -ne $config.firebase -and $parentFirebase.applicationId -ceq $config.firebase.applicationId){throw 'Parent and child require distinct Firebase application IDs.'}
    $properties.appControlParentFirebaseApplicationId=$parentFirebase.applicationId
    $properties.appControlParentFirebaseApiKey=$parentFirebase.apiKey
    $properties.appControlParentFirebaseProjectId=$parentFirebase.projectId
    $properties.appControlParentFirebaseSenderId=$parentFirebase.senderId
}
if ($ValidateOnly) {
    Write-Output 'Public deployment configuration is structurally valid. Ownership, signing, live messaging and deployment remain unverified.'
    return
}
if ([string]::IsNullOrWhiteSpace($OutputDirectory)) { throw 'OutputDirectory is required unless ValidateOnly is selected.' }
$output = [IO.Path]::GetFullPath($OutputDirectory)
if (Test-Path -LiteralPath $output) { throw 'Configuration output directory must be new.' }
$null = New-Item -ItemType Directory -Path $output
$lines = @('# Public deployment settings only. Never add private keys or credentials.')
$arguments = @()
foreach ($name in $properties.Keys) { $lines += $name + '=' + $properties[$name]; $arguments += '-P' + $name + '=' + $properties[$name] }
Write-NewText (Join-Path $output 'gradle.properties') (($lines -join "`n") + "`n")
Write-JSON (Join-Path $output 'release-arguments.json') ([ordered]@{
    arguments=$arguments
    tasks=@($(if($null -ne $parentFirebase){':parent-app:assembleFcmRelease'}else{':parent-app:assembleRelease'}),$(if ($null -ne $config.firebase) { ':child-app:assembleFcmRelease' } else { ':child-app:assembleRelease' }))
})
if ($null -ne $config.firebase) { Write-JSON (Join-Path $output 'firebase-public.json') $config.firebase }
if($null -ne $parentFirebase){Write-JSON (Join-Path $output 'parent-firebase-public.json') $parentFirebase}
$files = @(Get-ChildItem -LiteralPath $output -File | Sort-Object Name | ForEach-Object { [ordered]@{name=$_.Name;sha256=(Get-FileHash -LiteralPath $_.FullName -Algorithm SHA256).Hash.ToLowerInvariant()} })
Write-JSON (Join-Path $output 'configuration.json') ([ordered]@{
    schemaVersion=1; mode=$(if ($AllowFixture) { 'fixture' } else { 'deployment-inputs' }); serverOrigin=$canonical
    structuralValidationPassed=$true; productionReady=$false; tlsOwnershipVerified=$false; firebaseRegistrationVerified=$false
    liveDeliveryVerified=$false; apkSigningVerified=$false; files=$files
})
Write-Output 'Public deployment build configuration exported. Structural checks only; live trust, signing, project registration and deployment acceptance remain open.'
