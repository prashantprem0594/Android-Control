# Shared local-runtime guards. No command in this file starts or stops a process.
$ErrorActionPreference='Stop'
function Assert-LaptopNoReparse([string]$Path) {
    $item=Get-Item -LiteralPath $Path -Force -ErrorAction Stop
    for($current=$item;$null -ne $current;){
        if(($current.Attributes -band [IO.FileAttributes]::ReparsePoint) -ne 0){throw 'Laptop runtime refuses symlink/junction paths.'}
        if($current.PSIsContainer){$current=$current.Parent}else{$current=$current.Directory}
    }
    return $item
}
function Resolve-LaptopState([string]$Workspace,[string]$Directory) {
    if([string]::IsNullOrWhiteSpace($Directory)){$Directory=Join-Path $Workspace 'data/laptop-server'}
    if($Directory -match '[\x00-\x1f]' -or @($Directory.Split([char[]]'\/')|Where-Object {$_ -eq '..'}).Count){throw 'Laptop state path is invalid.'}
    $full=[IO.Path]::GetFullPath($Directory).TrimEnd([char[]]'\/')
    $parent=Split-Path -Parent $full
    $allowed=@((Join-Path $Workspace 'data'),(Join-Path $Workspace '.cache'))
    if($parent -notin $allowed -or (Split-Path -Leaf $full) -notmatch '^laptop-server(?:-[A-Za-z0-9-]+)?$'){throw 'Use an isolated workspace data/laptop-server or .cache/laptop-server-* directory.'}
    $null=Assert-LaptopNoReparse $Workspace
    if(-not(Test-Path -LiteralPath $parent)){$null=New-Item -ItemType Directory -Path $parent}
    $null=Assert-LaptopNoReparse $parent
    if(Test-Path -LiteralPath $full){$null=Assert-LaptopNoReparse $full}
    return $full
}
function Set-LaptopPrivateAcl([string]$Directory) {
    $null=Assert-LaptopNoReparse $Directory
    # Modify only the discretionary ACL. Set-Acl can attempt to persist audit
    # sections and require SeSecurityPrivilege even for a directory we own.
    $acl=[Security.AccessControl.DirectorySecurity]::new()
    $acl.SetAccessRuleProtection($true,$false)
    $identity=[Security.Principal.WindowsIdentity]::GetCurrent().User
    $system=[Security.Principal.SecurityIdentifier]::new('S-1-5-18')
    foreach($principal in @($identity,$system)){
        $rule=[Security.AccessControl.FileSystemAccessRule]::new($principal,'FullControl','ContainerInherit,ObjectInherit','None','Allow')
        $acl.AddAccessRule($rule)
    }
    $directoryInfo=[IO.DirectoryInfo]::new($Directory)
    $directoryInfo.SetAccessControl($acl)
}
function Assert-LaptopFileDestination([string]$Path) {
    $null=Assert-LaptopNoReparse (Split-Path -Parent $Path)
    if(Test-Path -LiteralPath $Path){$item=Assert-LaptopNoReparse $Path;if($item.PSIsContainer){throw 'Expected a regular laptop runtime file.'}}
}
function Get-LaptopHash([string]$Path) {
    $null=Assert-LaptopNoReparse $Path
    return (Get-FileHash -LiteralPath $Path -Algorithm SHA256).Hash.ToLowerInvariant()
}
function Read-LaptopRuntime([string]$State) {
    $path=Join-Path $State 'private/runtime.json'
    if(-not(Test-Path -LiteralPath $path)){return $null}
    $item=Assert-LaptopNoReparse $path
    if($item.Length -gt 16384){throw 'Laptop runtime process record is invalid.'}
    $record=Get-Content -LiteralPath $path -Raw | ConvertFrom-Json
    if($record.schemaVersion -ne 1){throw 'Laptop runtime process record is unsupported.'}
    return $record
}
function Get-LaptopOwnedProcess($Entry,[string]$ExpectedPath) {
    if($null -eq $Entry){return $null}
    if($Entry.processId -notmatch '^[1-9][0-9]*$' -or $Entry.sha256 -notmatch '^[a-f0-9]{64}$' -or [long]$Entry.startTimeUtcTicks -le 0){throw 'Laptop process ownership record is invalid.'}
    $process=Get-Process -Id ([int]$Entry.processId) -ErrorAction SilentlyContinue
    if($null -eq $process){return $null}
    if(-not [string]::Equals([IO.Path]::GetFullPath($process.Path),[IO.Path]::GetFullPath($ExpectedPath),[StringComparison]::OrdinalIgnoreCase) -or
        $process.StartTime.ToUniversalTime().Ticks -ne [long]$Entry.startTimeUtcTicks -or (Get-LaptopHash $ExpectedPath) -cne $Entry.sha256){throw 'Refusing an action against a process outside this laptop runtime identity.'}
    return $process
}
function Assert-LaptopNotRunning([string]$State) {
    $record=Read-LaptopRuntime $State
    if($null -eq $record){return}
    $server=Get-LaptopOwnedProcess $record.server (Join-Path $State 'bin/control-server.exe')
    $proxy=Get-LaptopOwnedProcess $record.proxy (Join-Path $State 'bin/laptop-proxy.exe')
    if($null -ne $server -or $null -ne $proxy){throw 'Laptop runtime is already active. Stop this exact state before starting it again.'}
}
function New-LaptopProcessRecord($Process,[string]$Executable) {
    return [ordered]@{processId=$Process.Id;startTimeUtcTicks=$Process.StartTime.ToUniversalTime().Ticks;sha256=(Get-LaptopHash $Executable)}
}
function Write-LaptopRuntime([string]$State,$Value) {
    $path=Join-Path $State 'private/runtime.json'
    Assert-LaptopFileDestination $path
    [IO.File]::WriteAllText($path,($Value|ConvertTo-Json -Depth 7),[Text.UTF8Encoding]::new($false))
}
