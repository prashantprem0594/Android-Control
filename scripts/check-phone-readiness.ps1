param(
    [Parameter(Mandatory)] [string]$Role,
    [Parameter(Mandatory)] [string]$Serial,
    [ValidateRange(1024,65535)] [int]$AdbServerPort=5037
)
# Read-only inventory. No installation, role/provisioning changes, UI dumps, screen/call actions or live services.
$ErrorActionPreference='Stop'
. (Join-Path $PSScriptRoot 'phone-readiness-rules.ps1')
$definition=Get-DesignatedPhone $Role $Serial
$snapshot=@{}
if($null -ne $definition){
    $adb=Join-Path (Split-Path -Parent $PSScriptRoot) '.tooling\android-sdk\platform-tools\adb.exe'
    function Query([string[]]$Arguments){
        try{
            $ErrorActionPreference='Continue'
            $lines=@(& $adb -P $AdbServerPort @Arguments 2>&1)
            $code=$LASTEXITCODE
            $text=$lines -join "`n"
            if($text.Length -gt 524288){return @{exitCode=1;output=''}}
            return @{exitCode=$code;output=$text}
        }catch{return @{exitCode=1;output=''}}
    }
    $snapshot.devices=Query @('devices','-l')
    $state=Get-PhoneConnectionState $snapshot.devices.exitCode $snapshot.devices.output $Serial
    if($state -ceq 'device'){
        $snapshot.device=Query @('-s',$Serial,'shell','getprop','ro.product.device')
        if($snapshot.device.exitCode -eq 0 -and $snapshot.device.output.Trim() -ceq $definition.device){
            $snapshot.api=Query @('-s',$Serial,'shell','getprop','ro.build.version.sdk')
            $snapshot.model=Query @('-s',$Serial,'shell','getprop','ro.product.model')
            $snapshot.user=Query @('-s',$Serial,'shell','am','get-current-user')
            $activeUser=0
            if($snapshot.user.exitCode -eq 0 -and [int]::TryParse($snapshot.user.output.Trim(),[ref]$activeUser) -and $activeUser -ge 0 -and $activeUser -le 21474){
                $snapshot.packages=Query @('-s',$Serial,'shell','pm','list','packages','--user',([string]$activeUser),$definition.package)
                if($snapshot.packages.exitCode -eq 0 -and @($snapshot.packages.output -split '\r?\n' | Where-Object {$_ -ceq ('package:'+$definition.package)}).Count -eq 1){
                    $snapshot.packageDump=Query @('-s',$Serial,'shell','dumpsys','package',$definition.package)
                }else{$snapshot.packageDump=@{exitCode=1;output=''}}
                $snapshot.policy=Query @('-s',$Serial,'shell','dumpsys','device_policy')
            }
        }
    }
}
$report=New-PhoneReadinessReport $Role $Serial $snapshot
$report | ConvertTo-Json -Depth 8
