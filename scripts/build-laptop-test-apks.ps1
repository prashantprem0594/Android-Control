param(
    [string]$StateDirectory=(Join-Path (Split-Path -Parent $PSScriptRoot) 'data/laptop-server'),
    [string]$OutputDirectory,
    [string]$FirebaseSettingsDirectory,
    [ValidateRange(1,2100000000)] [int]$VersionCode=1,
    [ValidatePattern('\A[A-Za-z0-9][A-Za-z0-9._+\-]{0,63}\z')] [string]$VersionName='0.1.0',
    [switch]$ValidateOnly
)
$ErrorActionPreference='Stop'
$path=Join-Path $StateDirectory 'laptop-server.json'
try{
    $file=Get-Item -LiteralPath $path
    if($file -isnot [IO.FileInfo] -or $file.Length -gt 16384){throw 'size'}
    $settings=[IO.File]::ReadAllText($file.FullName,[Text.UTF8Encoding]::new($false,$true)) | ConvertFrom-Json
}catch{throw 'Initialize laptop server first; bounded public laptop metadata is required.'}
if($settings.schemaVersion -ne 1 -or $settings.scope -cne 'laptop-development-bootstrap' -or
    $settings.httpsOrigin -isnot [string] -or $settings.httpsOrigin -cnotmatch '\Ahttps://localhost(?::[0-9]+)?\z' -or
    $settings.rootCertificateFile -cne 'root-ca.pem' -or $settings.rootCertificateSHA256 -cnotmatch '\A[a-f0-9]{64}\z'){
    throw 'Invalid laptop development metadata.'
}
try{$expires=[DateTimeOffset]::Parse($settings.certificatesExpireAtUtc)}catch{throw 'Invalid laptop certificate lifetime.'}
if($expires -le [DateTimeOffset]::UtcNow){throw 'Laptop certificate expired; do not build from this state.'}
$ca=Join-Path $StateDirectory 'root-ca.pem'
if((Get-FileHash -LiteralPath $ca -Algorithm SHA256).Hash.ToLowerInvariant() -cne $settings.rootCertificateSHA256){throw 'Laptop public CA hash mismatch.'}
$inputs=@{
    ServerOrigin=$settings.httpsOrigin;CurrentPin=$settings.currentPin;BackupPin=$settings.backupPin
    DevelopmentCaFile=$ca;VersionCode=$VersionCode;VersionName=$VersionName
}
if([string]::IsNullOrWhiteSpace($FirebaseSettingsDirectory)){$inputs.PollingOnly=$true}else{$inputs.FirebaseSettingsDirectory=$FirebaseSettingsDirectory}
if($ValidateOnly){$inputs.ValidateOnly=$true}else{$inputs.OutputDirectory=$OutputDirectory}
& (Join-Path $PSScriptRoot 'build-wakeup-test-apks.ps1') @inputs
