param(
    [Parameter(Mandatory)] [string]$Role,
    [Parameter(Mandatory)] [string]$Serial,
    [string]$StateDirectory=(Join-Path (Split-Path -Parent $PSScriptRoot) 'data/laptop-server'),
    [ValidateRange(1024,65535)] [int]$AdbServerPort=5037
)
# Adds only this laptop profile's reversible ADB port mapping; no installs/permissions/roles.
$ErrorActionPreference='Stop'
. (Join-Path $PSScriptRoot 'phone-readiness-rules.ps1')
. (Join-Path $PSScriptRoot 'laptop-phone-rules.ps1')
$phone=Get-DesignatedPhone $Role $Serial
if($null -eq $phone){throw 'Role and serial must match a designated App Control phone.'}
$path=Join-Path $StateDirectory 'laptop-server.json'
$file=Get-Item -LiteralPath $path
if($file -isnot [IO.FileInfo] -or $file.Length -gt 16384){throw 'Invalid laptop public metadata.'}
try{$settings=Get-Content -LiteralPath $path -Raw | ConvertFrom-Json}catch{throw 'Invalid laptop public metadata.'}
if($settings.schemaVersion -ne 1 -or $settings.scope -cne 'laptop-development-bootstrap' -or $settings.httpsOrigin -cnotmatch '\Ahttps://localhost:([0-9]+)\z'){
    throw 'Expected explicit high-port localhost laptop profile.'
}
$port=0
if(-not [int]::TryParse($Matches[1],[ref]$port) -or $port -notin 1024..65535){throw 'Invalid laptop HTTPS port.'}
$adb=Join-Path (Split-Path -Parent $PSScriptRoot) '.tooling/android-sdk/platform-tools/adb.exe'
function Device([string[]]$Arguments){
    $ErrorActionPreference='Continue'
    $output=@(& $adb -P $AdbServerPort -s $Serial @Arguments 2>&1)
    if($LASTEXITCODE -ne 0){throw 'Laptop phone connection operation failed; reconnect and authorize USB debugging.'}
    return ($output -join "`n")
}
if((Device @('get-state')).Trim() -cne 'device' -or (Device @('shell','getprop','ro.product.device')).Trim() -cne $phone.device){throw 'Expected authorized designated phone not connected.'}
Set-LaptopReverseMapping $port ${function:Device}
Write-Output 'Laptop USB HTTPS mapping configured. Use the matching laptop-debug app. USB/laptop must remain connected; no wireless, Firebase delivery or policy acceptance inferred.'
