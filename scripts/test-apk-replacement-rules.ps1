# Invoke through run-logged.ps1. No device calls, APK installation or signing.
$ErrorActionPreference='Stop'
. (Join-Path $PSScriptRoot 'apk-replacement-rules.ps1')
function Refuses([scriptblock]$Action){$rejected=$false;try{& $Action}catch{$rejected=$true};if(-not $rejected){throw 'Unsafe APK replacement policy accepted.'}}
foreach($code in @(0,1)){if($null -ne (Get-AcceptanceInstalledBaseApk $code @())){throw 'Absent package not recognized.'}}
Refuses {Get-AcceptanceInstalledBaseApk 1 @('error: device offline')}
Refuses {Get-AcceptanceInstalledBaseApk 0 @('package:/data/app/one/base.apk','error: permission denied')}
Refuses {Get-AcceptanceInstalledBaseApk 0 @('package:/data/app/one/base.apk','package:/data/app/two/base.apk')}
if((Get-AcceptanceInstalledBaseApk 0 @('package:/data/app/one/base.apk','package:/data/app/one/split_config.apk')) -cne '/data/app/one/base.apk'){throw 'Installed APK path parsing failed.'}
$signer='a'*64
if((Resolve-AcceptanceApkVersion 'com.appcontrol.child.test' '' 'com.appcontrol.child' 'com.appcontrol.transport.ServerTrustInstrumentation') -ne 0){throw 'Versionless instrumentation Android default not recognized.'}
Refuses {Resolve-AcceptanceApkVersion 'com.appcontrol.child' '' '' ''}
Refuses {Resolve-AcceptanceApkVersion 'com.appcontrol.child.test' '' 'another.package' 'com.appcontrol.transport.ServerTrustInstrumentation'}
Refuses {Resolve-AcceptanceApkVersion 'com.appcontrol.child.test' '' 'com.appcontrol.child' 'another.Runner'}
Assert-AcceptanceReplacementIdentity 8 8 $true $signer $signer
Assert-AcceptanceReplacementIdentity 9 8 $true $signer $signer
Assert-AppControlReplacementIdentity 8 8 $signer $signer
Assert-AppControlReplacementIdentity 9 8 $signer $signer
Refuses {Assert-AcceptanceReplacementIdentity 6 8 $true $signer $signer}
Refuses {Assert-AcceptanceReplacementIdentity 8 8 $true $signer ('b'*64)}
Refuses {Assert-AcceptanceReplacementIdentity 8 8 $false $signer $signer}
Refuses {Assert-AppControlReplacementIdentity 6 8 $signer $signer}
Refuses {Assert-AppControlReplacementIdentity 8 8 $signer ('b'*64)}
Write-Output 'APK replacement rules pass: first-install absence, split base path, query failures, no downgrade, same signer and debug-only identity.'
