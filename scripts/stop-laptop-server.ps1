param([string]$StateDirectory)
# Stops only the exact recorded binaries/start times of this isolated laptop state.
$ErrorActionPreference='Stop'
. (Join-Path $PSScriptRoot 'laptop-server-common.ps1')
$workspace=Split-Path -Parent $PSScriptRoot
$state=Resolve-LaptopState $workspace $StateDirectory
if(-not(Test-Path -LiteralPath $state)){Write-Output 'Laptop state is absent; nothing stopped.';return}
$lockPath=Join-Path $state 'private/operation.lock';Assert-LaptopFileDestination $lockPath
$lock=[IO.File]::Open($lockPath,[IO.FileMode]::OpenOrCreate,[IO.FileAccess]::ReadWrite,[IO.FileShare]::None)
try{
    $record=Read-LaptopRuntime $state
    if($null -eq $record){Write-Output 'No recorded laptop processes; nothing stopped.';return}
    $helper=Join-Path $state 'bin/laptop-proxy.exe';$server=Join-Path $state 'bin/control-server.exe'
    $proxy=Get-LaptopOwnedProcess $record.proxy $helper
    $backend=Get-LaptopOwnedProcess $record.server $server
    if($null -ne $proxy){
        $previousPreference=$ErrorActionPreference;$ErrorActionPreference='Continue'
        try{$null=& $helper shutdown -state-dir $state 2>&1}finally{$ErrorActionPreference=$previousPreference}
        if(-not $proxy.WaitForExit(6000)){$proxy=Get-LaptopOwnedProcess $record.proxy $helper;if($null -ne $proxy){Stop-Process -Id $proxy.Id -Force}}
    }
    if($null -ne $backend){$backend=Get-LaptopOwnedProcess $record.server $server;if($null -ne $backend){Stop-Process -Id $backend.Id -Force;$null=$backend.WaitForExit(5000)}}
    $record.status='stopped';Write-LaptopRuntime $state $record
    Write-Output 'Owned laptop processes stopped. Database, certificates and keys preserved; no device/trust/firewall changes.'
}finally{$lock.Dispose()}
