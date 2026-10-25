param(
    [Parameter(Mandatory)] [string]$OutputDirectory,
    [ValidateSet('windows-amd64','linux-amd64','linux-arm64')]
    [string[]]$Targets = @('windows-amd64','linux-amd64'),
    [string]$GoExecutable = 'C:\Program Files\Go\bin\go.exe'
)
# Run through run-logged.ps1. Produces binaries only; never starts a service.
$ErrorActionPreference = 'Stop'
$workspace = Split-Path -Parent $PSScriptRoot
$source = Join-Path $workspace 'server'
$output = [IO.Path]::GetFullPath($OutputDirectory)
if (Test-Path -LiteralPath $output) { throw 'Output directory must be new.' }
if ($output.StartsWith($source + [IO.Path]::DirectorySeparatorChar, [StringComparison]::OrdinalIgnoreCase)) {
    throw 'Output must be outside the server source tree.'
}
if (-not (Test-Path -LiteralPath $GoExecutable -PathType Leaf)) { throw 'Go executable unavailable.' }
function Invoke-Go([string[]]$Arguments) {
    $ErrorActionPreference = 'Continue'
    $result = @(& $GoExecutable @Arguments 2>&1)
    if ($LASTEXITCODE -ne 0) { throw ("Go $($Arguments[0]) failed; bundle is incomplete.`n" + ($result -join "`n")) }
    return ($result -join "`n")
}
function Get-SourceInventory {
    # Source roots include embedded SQL; exclude runtime data and local caches.
    $roots = @('cmd','internal','go.mod','go.sum') | ForEach-Object { Join-Path $source $_ }
    foreach ($root in $roots) {
        if ((Get-Item -LiteralPath $root).Attributes -band [IO.FileAttributes]::ReparsePoint) { throw 'Source symlink unsupported.' }
        if (Test-Path -LiteralPath $root -PathType Container) {
            foreach ($folder in @(Get-ChildItem -LiteralPath $root -Recurse -Directory)) {
                if ($folder.Attributes -band [IO.FileAttributes]::ReparsePoint) { throw 'Source symlink unsupported.' }
            }
        }
    }
    $rows = @(Get-ChildItem -LiteralPath $roots -Recurse -File | Sort-Object FullName | ForEach-Object {
        if (($_.Attributes -band [IO.FileAttributes]::ReparsePoint) -ne 0) { throw 'Source symlink unsupported.' }
        [ordered]@{
            path = $_.FullName.Substring($source.Length + 1).Replace('\','/')
            sha256 = (Get-FileHash -LiteralPath $_.FullName -Algorithm SHA256).Hash.ToLowerInvariant()
        }
    })
    return ($rows | ConvertTo-Json -Depth 4 -Compress)
}
$names = @('GOOS','GOARCH','CGO_ENABLED','GOPROXY','GOSUMDB','GOTOOLCHAIN','GOCACHE','GOMODCACHE','GOFLAGS','GOWORK')
$saved = @{}
foreach ($name in $names) { $saved[$name] = [Environment]::GetEnvironmentVariable($name,'Process') }
Push-Location -LiteralPath $source
try {
    $env:CGO_ENABLED = '0'
    $env:GOPROXY = 'off'
    $env:GOSUMDB = 'off'
    $env:GOTOOLCHAIN = 'local'
    $env:GOCACHE = Join-Path $workspace '.cache\go-build'
    $env:GOMODCACHE = Join-Path $workspace '.cache\go-mod'
    $env:GOFLAGS = ''
    $env:GOWORK = 'off'
    $before = Get-SourceInventory
    $version = Invoke-Go @('version')
    $null = Invoke-Go @('mod','verify')
    $moduleRows = @()
    $null = New-Item -ItemType Directory -Path $output
    $artifacts = @()
    foreach ($target in @($Targets | Select-Object -Unique)) {
        $parts = $target.Split('-')
        $env:GOOS = $parts[0]
        $env:GOARCH = $parts[1]
        $dependencies = Invoke-Go @('list','-mod=readonly','-deps','-f','{{if .Module}}{{.Module.Path}} {{.Module.Version}} {{.Module.Sum}}{{if .Module.Replace}} REPLACED{{end}}{{end}}','./cmd/...')
        if ($dependencies -match 'REPLACED') { throw 'Replaced modules require a separate reviewed packaging policy.' }
        $moduleRows += $dependencies -split "`r?`n" | Where-Object { $_.Trim() }
        $directory = Join-Path $output $target
        $null = New-Item -ItemType Directory -Path $directory
        foreach ($command in @('control-server','control-admin','parent-recovery','release-manifest')) {
            $extension = if ($env:GOOS -eq 'windows') { '.exe' } else { '' }
            $binary = Join-Path $directory ($command + $extension)
            $null = Invoke-Go @('build','-mod=readonly','-trimpath','-buildvcs=false','-o',$binary,('./cmd/' + $command))
            $info = Invoke-Go @('version','-m',$binary)
            [IO.File]::WriteAllText(($binary + '.buildinfo.txt'),$info,[Text.UTF8Encoding]::new($false))
            $artifacts += [ordered]@{
                path = $target + '/' + $command + $extension
                size = (Get-Item -LiteralPath $binary).Length
                sha256 = (Get-FileHash -LiteralPath $binary -Algorithm SHA256).Hash.ToLowerInvariant()
            }
            Write-Output "Built $target/$command$extension"
        }
    }
    if ($before -cne (Get-SourceInventory)) { throw 'Source changed during compilation; no bundle manifest published.' }
    $licenseDirectory = Join-Path $output 'licenses'
    $licenseRows = @()
    foreach ($moduleRow in @($moduleRows | Sort-Object -Unique)) {
        $moduleParts = $moduleRow -split '\s+'
        if ($moduleParts.Count -lt 2 -or $moduleParts[1] -eq '') { continue }
        $modulePath = $moduleParts[0]
        $moduleVersion = $moduleParts[1]
        $moduleCache = Join-Path $env:GOMODCACHE ($modulePath + '@' + $moduleVersion)
        if (-not (Test-Path -LiteralPath $moduleCache -PathType Container)) {
            throw "Module cache missing while collecting license notices: $modulePath $moduleVersion"
        }
        $noticeFiles = @(Get-ChildItem -LiteralPath $moduleCache -File | Where-Object {
            $_.Name -match '^(LICENSE|NOTICE|COPYING|PATENTS)(\..*)?$'
        })
        if (-not ($noticeFiles | Where-Object { $_.Name -match '^(LICENSE|COPYING)(\..*)?$' })) {
            throw "No root LICENSE or COPYING file for shipped module: $modulePath $moduleVersion"
        }
        $relativeDirectory = ($modulePath + '@' + $moduleVersion).Replace('/','\')
        $destinationDirectory = Join-Path $licenseDirectory $relativeDirectory
        $null = New-Item -ItemType Directory -Path $destinationDirectory -Force
        foreach ($noticeFile in $noticeFiles) {
            $destination = Join-Path $destinationDirectory $noticeFile.Name
            Copy-Item -LiteralPath $noticeFile.FullName -Destination $destination
            $licenseRows += [ordered]@{
                path = 'licenses/' + $modulePath + '@' + $moduleVersion + '/' + $noticeFile.Name
                sha256 = (Get-FileHash -LiteralPath $destination -Algorithm SHA256).Hash.ToLowerInvariant()
            }
        }
    }
    $utf8 = [Text.UTF8Encoding]::new($false)
    $modules = (@($moduleRows | Sort-Object -Unique) -join "`n")
    [IO.File]::WriteAllText((Join-Path $output 'source-inventory.json'),$before,$utf8)
    [IO.File]::WriteAllText((Join-Path $output 'modules.txt'),$modules,$utf8)
    $manifest = [ordered]@{
        schemaVersion = 1
        createdAt = [datetime]::UtcNow.ToString('o')
        goVersion = $version
        cgoEnabled = $false
        sourceInventorySHA256 = (Get-FileHash -LiteralPath (Join-Path $output 'source-inventory.json') -Algorithm SHA256).Hash.ToLowerInvariant()
        modulesSHA256 = (Get-FileHash -LiteralPath (Join-Path $output 'modules.txt') -Algorithm SHA256).Hash.ToLowerInvariant()
        artifacts = $artifacts
        licenseNotices = $licenseRows
        runtimeAcceptancePassed = $false
    }
    [IO.File]::WriteAllText((Join-Path $output 'bundle.json'),($manifest | ConvertTo-Json -Depth 6),$utf8)
    Write-Output "Complete server bundle: $output"
} finally {
    Pop-Location
    foreach ($name in $names) { [Environment]::SetEnvironmentVariable($name,$saved[$name],'Process') }
}
