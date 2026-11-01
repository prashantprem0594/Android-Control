param(
    [string]$StateDirectory,
    [ValidateRange(1,65535)] [int]$HTTPSPort=8443,
    [ValidateRange(1,65535)] [int]$BackendPort=18080,
    [switch]$InitializeOnly,
    [switch]$EnableParentSender,
    [string]$ParentFirebaseServiceAccountFile,
    [string]$ChildFirebaseServiceAccountFile,
    [switch]$EnableRemoteCalls
)
# Run through run-logged.ps1. Own-server laptop development only; no phone/firewall/trust-store changes.
$ErrorActionPreference='Stop'
. (Join-Path $PSScriptRoot 'laptop-server-common.ps1')
$workspace=Split-Path -Parent $PSScriptRoot
$state=Resolve-LaptopState $workspace $StateDirectory
if($HTTPSPort -eq $BackendPort){throw 'HTTPS and backend ports must differ.'}
if(Test-Path -LiteralPath $state){Assert-LaptopNotRunning $state}
if($EnableParentSender -and [string]::IsNullOrWhiteSpace($ParentFirebaseServiceAccountFile)){throw 'Enabling the parent sender requires a real private service-account file path.'}
foreach($path in @($ParentFirebaseServiceAccountFile,$ChildFirebaseServiceAccountFile)){
    if(-not [string]::IsNullOrWhiteSpace($path)){$item=Assert-LaptopNoReparse $path;if($item.PSIsContainer){throw 'Service account must be a private regular file.'}}
}
$go='C:\Program Files\Go\bin\go.exe'
$null=Assert-LaptopNoReparse (Join-Path $workspace '.cache')
$build=Join-Path $workspace ('.cache/laptop-build-'+[guid]::NewGuid().ToString('N'))
$null=New-Item -ItemType Directory -Path $build
$saved=@{}
foreach($name in @('GOPROXY','GOSUMDB','GOTOOLCHAIN','GOCACHE','GOMODCACHE','GOOS','GOARCH','CGO_ENABLED','GOWORK','GOFLAGS')){$saved[$name]=[Environment]::GetEnvironmentVariable($name,'Process')}
try{
    $env:GOPROXY='off';$env:GOSUMDB='off';$env:GOTOOLCHAIN='local';$env:GOCACHE=Join-Path $workspace '.cache/go-build';$env:GOMODCACHE=Join-Path $workspace '.cache/go-mod';$env:GOOS='windows';$env:GOARCH='amd64';$env:CGO_ENABLED='0';$env:GOWORK='off';$env:GOFLAGS=''
    Push-Location (Join-Path $workspace 'tools/laptop-server')
    try{& $go build -mod=readonly -trimpath -o (Join-Path $build 'laptop-proxy.exe') .;if($LASTEXITCODE -ne 0){throw 'Laptop HTTPS helper build failed.'}}finally{Pop-Location}
    Push-Location (Join-Path $workspace 'server')
    try{& $go build -mod=readonly -trimpath -o (Join-Path $build 'control-server.exe') ./cmd/control-server;if($LASTEXITCODE -ne 0){throw 'Own server build failed.'}}finally{Pop-Location}
}finally{foreach($name in $saved.Keys){[Environment]::SetEnvironmentVariable($name,$saved[$name],'Process')}}
$helper=Join-Path $build 'laptop-proxy.exe'
if(-not(Test-Path -LiteralPath $state)){$null=New-Item -ItemType Directory -Path $state;Set-LaptopPrivateAcl $state}
if(@(Get-ChildItem -LiteralPath $state -Force).Count -eq 0){
    Set-LaptopPrivateAcl $state
    & $helper init -state-dir $state -https-port $HTTPSPort -backend-port $BackendPort
    if($LASTEXITCODE -ne 0){throw 'New laptop identity initialization failed; preserve incomplete state for review.'}
}else{
    & $helper verify-state -state-dir $state
    if($LASTEXITCODE -ne 0){throw 'Existing state is invalid or incomplete. Refusing to overwrite identities.'}
}
$publicFile=Join-Path $state 'laptop-server.json'
$public=Get-Content -LiteralPath $publicFile -Raw|ConvertFrom-Json
$expectedOrigin=if($HTTPSPort -eq 443){'https://localhost'}else{'https://localhost:'+$HTTPSPort}
if($public.httpsOrigin -cne $expectedOrigin -or $public.backendAddress -cne ('127.0.0.1:'+$BackendPort)){throw 'Port selection differs from existing public metadata; reuse its ports or choose a separate new state.'}
$lockPath=Join-Path $state 'private/operation.lock';Assert-LaptopFileDestination $lockPath
$lock=[IO.File]::Open($lockPath,[IO.FileMode]::OpenOrCreate,[IO.FileAccess]::ReadWrite,[IO.FileShare]::None)
$startedServer=$null;$startedProxy=$null;$record=$null
try{
    Assert-LaptopNotRunning $state
    $bin=Join-Path $state 'bin';$null=[IO.Directory]::CreateDirectory($bin);$null=Assert-LaptopNoReparse $bin
    foreach($name in @('control-server.exe','laptop-proxy.exe')){$destination=Join-Path $bin $name;Assert-LaptopFileDestination $destination;[IO.File]::Copy((Join-Path $build $name),$destination,$true)}
    $helper=Join-Path $bin 'laptop-proxy.exe';$server=Join-Path $bin 'control-server.exe'
    if($InitializeOnly){
        [pscustomobject]@{status='initialized-only';publicMetadataFile=$publicFile;httpsOrigin=$public.httpsOrigin;stateDirectory=$state;senderEnabled=$false}|ConvertTo-Json
        return
    }
    foreach($port in @($HTTPSPort,$BackendPort)){$listener=[Net.Sockets.TcpListener]::new([Net.IPAddress]::Loopback,$port);try{$listener.Start()}finally{$listener.Stop()}}
	$recoveryKeyPath=Join-Path $state 'private/recovery-hmac-key.bin'
	Assert-LaptopFileDestination $recoveryKeyPath
	if(-not(Test-Path -LiteralPath $recoveryKeyPath)){
		$recoveryKey=New-Object byte[] 32
		(New-Object Security.Cryptography.RNGCryptoServiceProvider).GetBytes($recoveryKey)
		[IO.File]::WriteAllBytes($recoveryKeyPath,$recoveryKey)
		[Array]::Clear($recoveryKey,0,$recoveryKey.Length)
	}
	$recoveryKeyItem=Assert-LaptopNoReparse $recoveryKeyPath
	if($recoveryKeyItem.PSIsContainer -or $recoveryKeyItem.Length -ne 32){throw 'Recovery HMAC key must be a private 32-byte regular file.'}
    $environment=[ordered]@{
        APP_CONTROL_HTTP_ADDRESS=$public.backendAddress;APP_CONTROL_DATABASE_PATH=(Join-Path $state 'data/app-control.db')
        APP_CONTROL_TRUSTED_PROXY_CIDRS='127.0.0.1/32';APP_CONTROL_REMOTE_CALLS_ENABLED=([string][bool]$EnableRemoteCalls).ToLowerInvariant()
        APP_CONTROL_PARENT_FCM_TOKEN_KEY_FILE=(Join-Path $state 'private/parent-routing-key.bin');APP_CONTROL_PARENT_FCM_SENDER_ENABLED=([string][bool]$EnableParentSender).ToLowerInvariant()
		APP_CONTROL_RECOVERY_HMAC_KEY_FILE=$recoveryKeyPath
    }
    if($EnableParentSender){$environment['APP_CONTROL_PARENT_FCM_SERVICE_ACCOUNT_FILE']=[IO.Path]::GetFullPath($ParentFirebaseServiceAccountFile)}
    if(-not [string]::IsNullOrWhiteSpace($ChildFirebaseServiceAccountFile)){$environment['APP_CONTROL_FCM_TOKEN_KEY_FILE']=Join-Path $state 'private/child-routing-key.bin';$environment['APP_CONTROL_FCM_SERVICE_ACCOUNT_FILE']=[IO.Path]::GetFullPath($ChildFirebaseServiceAccountFile)}
    $lines=@($environment.Keys|ForEach-Object{$_+'='+$environment[$_]}) -join "`n"
    Assert-LaptopFileDestination (Join-Path $state 'private/server.env')
    [IO.File]::WriteAllText((Join-Path $state 'private/server.env'),($lines+"`n"),[Text.UTF8Encoding]::new($false))
    foreach($name in @('server.stdout.log','server.stderr.log','proxy.stdout.log','proxy.stderr.log')){Assert-LaptopFileDestination (Join-Path $state ('private/'+$name))}
    $prior=@{};foreach($entry in @(Get-ChildItem Env:APP_CONTROL_*)){$prior[$entry.Name]=$entry.Value;[Environment]::SetEnvironmentVariable($entry.Name,$null,'Process')}
    try{
        foreach($name in $environment.Keys){[Environment]::SetEnvironmentVariable($name,$environment[$name],'Process')}
        $startedServer=Start-Process -FilePath $server -WorkingDirectory $state -WindowStyle Hidden -PassThru -RedirectStandardOutput (Join-Path $state 'private/server.stdout.log') -RedirectStandardError (Join-Path $state 'private/server.stderr.log')
    }finally{
        foreach($entry in @(Get-ChildItem Env:APP_CONTROL_*)){[Environment]::SetEnvironmentVariable($entry.Name,$null,'Process')}
        foreach($name in $prior.Keys){[Environment]::SetEnvironmentVariable($name,$prior[$name],'Process')}
    }
    $record=[ordered]@{schemaVersion=1;status='starting';server=(New-LaptopProcessRecord $startedServer $server);proxy=$null}
    Write-LaptopRuntime $state $record
    $quotedState='"'+$state.Replace('"','\"')+'"'
    $startedProxy=Start-Process -FilePath $helper -ArgumentList @('serve','-state-dir',$quotedState) -WorkingDirectory $state -WindowStyle Hidden -PassThru -RedirectStandardOutput (Join-Path $state 'private/proxy.stdout.log') -RedirectStandardError (Join-Path $state 'private/proxy.stderr.log')
    $record.proxy=New-LaptopProcessRecord $startedProxy $helper
    Write-LaptopRuntime $state $record
    $ready=$false
    $startupDeadline=[datetime]::UtcNow.AddSeconds(30)
    do{
        if($startedServer.HasExited -or $startedProxy.HasExited){throw 'Laptop process exited during startup; private logs retain diagnostics.'}
        $previousPreference=$ErrorActionPreference;$ErrorActionPreference='Continue'
        try{$null=& $helper check -state-dir $state 2>&1;$checkExit=$LASTEXITCODE}finally{$ErrorActionPreference=$previousPreference}
        if($checkExit -eq 0){$ready=$true;break}
        Start-Sleep -Milliseconds 500
    }while([datetime]::UtcNow -lt $startupDeadline)
    if(-not $ready){throw 'Pinned localhost HTTPS readiness did not pass within the startup window.'}
    $record.status='running';Write-LaptopRuntime $state $record
    [pscustomobject]@{status='running';publicMetadataFile=$publicFile;httpsOrigin=$public.httpsOrigin;backendAddress=$public.backendAddress;stateDirectory=$state;senderEnabled=[bool]$EnableParentSender;scope='laptop-localhost-development-only'}|ConvertTo-Json
}catch{
    if($null -ne $record){foreach($name in @('proxy','server')){if($null -ne $record[$name]){$expected=if($name -eq 'proxy'){$helper}else{$server};$owned=Get-LaptopOwnedProcess $record[$name] $expected;if($null -ne $owned){Stop-Process -Id $owned.Id -Force}}}}
    throw
}finally{$lock.Dispose()}
