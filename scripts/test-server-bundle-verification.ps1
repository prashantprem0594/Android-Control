$ErrorActionPreference = 'Stop'
$workspace = Split-Path -Parent $PSScriptRoot
$fixtureRoot = Join-Path $workspace ('.cache\server-bundle-verification-' + [Guid]::NewGuid().ToString('N'))
$verifier = Join-Path $PSScriptRoot 'verify-server-bundle.ps1'
$utf8 = [Text.UTF8Encoding]::new($false)
$junctions = [Collections.Generic.List[string]]::new()

function Write-FixtureFile([string]$Path, [string]$Contents) {
    $parent = Split-Path -Parent $Path
    $null = New-Item -ItemType Directory -Path $parent -Force
    [IO.File]::WriteAllText($Path, $Contents, $utf8)
}

function Get-FixtureHash([string]$Path) {
    return (Get-FileHash -LiteralPath $Path -Algorithm SHA256).Hash.ToLowerInvariant()
}

function Write-FixtureBundle([string]$Directory) {
    $null = New-Item -ItemType Directory -Path $Directory -Force
    $artifactPath = Join-Path $Directory 'windows-amd64\control-server.exe'
    $noticePath = Join-Path $Directory 'licenses\example.org\sample@v1.0.0\LICENSE'
    Write-FixtureFile $artifactPath 'fixture binary bytes; verifier must not execute these'
    Write-FixtureFile $noticePath 'Fixture license notice.'
    Write-FixtureFile (Join-Path $Directory 'modules.txt') "example.org/sample v1.0.0`n"
    $inventory = @(
        [ordered]@{ path = 'cmd/control-server/main.go'; sha256 = ('a' * 64) },
        [ordered]@{ path = 'go.mod'; sha256 = ('b' * 64) }
    )
    $inventory = ConvertTo-Json -InputObject $inventory -Depth 4 -Compress
    Write-FixtureFile (Join-Path $Directory 'source-inventory.json') $inventory
    $manifest = [ordered]@{
        schemaVersion = 1
        sourceInventorySHA256 = Get-FixtureHash (Join-Path $Directory 'source-inventory.json')
        modulesSHA256 = Get-FixtureHash (Join-Path $Directory 'modules.txt')
        artifacts = @([ordered]@{
            path = 'windows-amd64/control-server.exe'
            size = (Get-Item -LiteralPath $artifactPath).Length
            sha256 = Get-FixtureHash $artifactPath
        })
        licenseNotices = @([ordered]@{
            path = 'licenses/example.org/sample@v1.0.0/LICENSE'
            sha256 = Get-FixtureHash $noticePath
        })
    } | ConvertTo-Json -Depth 6
    Write-FixtureFile (Join-Path $Directory 'bundle.json') $manifest
}

function Invoke-ExpectedFailure([string]$Directory, [string]$Label) {
    $failed = $false
    try {
        & $verifier -BundleDirectory $Directory | Out-Null
    } catch {
        $failed = $true
    }
    if (-not $failed) { throw "Verifier accepted the $Label fixture." }
}

try {
    $null = New-Item -ItemType Directory -Path $fixtureRoot
    $valid = Join-Path $fixtureRoot 'valid'
    Write-FixtureBundle $valid
    & $verifier -BundleDirectory $valid | Out-Null

    $tampered = Join-Path $fixtureRoot 'tampered'
    Copy-Item -LiteralPath $valid -Destination $tampered -Recurse
    [IO.File]::AppendAllText((Join-Path $tampered 'windows-amd64\control-server.exe'), 'tampered')
    Invoke-ExpectedFailure $tampered 'tampered artifact'

    $missingArtifact = Join-Path $fixtureRoot 'missing-artifact'
    Copy-Item -LiteralPath $valid -Destination $missingArtifact -Recurse
    Remove-Item -LiteralPath (Join-Path $missingArtifact 'windows-amd64\control-server.exe')
    Invoke-ExpectedFailure $missingArtifact 'missing artifact'

    $missing = Join-Path $fixtureRoot 'missing'
    Copy-Item -LiteralPath $valid -Destination $missing -Recurse
    Remove-Item -LiteralPath (Join-Path $missing 'licenses\example.org\sample@v1.0.0\LICENSE')
    Invoke-ExpectedFailure $missing 'missing notice'

    $badSize = Join-Path $fixtureRoot 'bad-size'
    Copy-Item -LiteralPath $valid -Destination $badSize -Recurse
    $manifestPath = Join-Path $badSize 'bundle.json'
    $manifest = Get-Content -LiteralPath $manifestPath -Raw | ConvertFrom-Json
    $manifest.artifacts[0].size = '1.5'
    [IO.File]::WriteAllText($manifestPath, ($manifest | ConvertTo-Json -Depth 6), $utf8)
    Invoke-ExpectedFailure $badSize 'invalid artifact size'

    $badSourceHash = Join-Path $fixtureRoot 'bad-source-hash'
    Copy-Item -LiteralPath $valid -Destination $badSourceHash -Recurse
    [IO.File]::AppendAllText((Join-Path $badSourceHash 'source-inventory.json'), ' ')
    Invoke-ExpectedFailure $badSourceHash 'tampered source inventory'

    $badModuleHash = Join-Path $fixtureRoot 'bad-module-hash'
    Copy-Item -LiteralPath $valid -Destination $badModuleHash -Recurse
    [IO.File]::AppendAllText((Join-Path $badModuleHash 'modules.txt'), 'example.org/other v1.0.0')
    Invoke-ExpectedFailure $badModuleHash 'tampered module inventory'

    $duplicate = Join-Path $fixtureRoot 'duplicate'
    Copy-Item -LiteralPath $valid -Destination $duplicate -Recurse
    $manifestPath = Join-Path $duplicate 'bundle.json'
    $manifest = Get-Content -LiteralPath $manifestPath -Raw | ConvertFrom-Json
    $manifest.artifacts += $manifest.artifacts[0]
    [IO.File]::WriteAllText($manifestPath, ($manifest | ConvertTo-Json -Depth 6), $utf8)
    Invoke-ExpectedFailure $duplicate 'duplicate artifact'

    $traversal = Join-Path $fixtureRoot 'traversal'
    Copy-Item -LiteralPath $valid -Destination $traversal -Recurse
    $manifestPath = Join-Path $traversal 'bundle.json'
    $manifest = Get-Content -LiteralPath $manifestPath -Raw | ConvertFrom-Json
    $manifest.artifacts[0].path = '../outside.exe'
    [IO.File]::WriteAllText($manifestPath, ($manifest | ConvertTo-Json -Depth 6), $utf8)
    Invoke-ExpectedFailure $traversal 'path traversal'

    $outside = Join-Path $fixtureRoot 'outside'
    Copy-Item -LiteralPath $valid -Destination $outside -Recurse
    $manifestPath = Join-Path $outside 'bundle.json'
    $manifest = Get-Content -LiteralPath $manifestPath -Raw | ConvertFrom-Json
    $manifest.licenseNotices[0].path = [IO.Path]::GetFullPath((Join-Path $fixtureRoot 'external-license.txt'))
    [IO.File]::WriteAllText($manifestPath, ($manifest | ConvertTo-Json -Depth 6), $utf8)
    Invoke-ExpectedFailure $outside 'absolute external path'

    foreach ($badPath in @('windows-amd64/control-server.exe.','windows-amd64/control-server.exe ', 'windows-amd64/NUL.exe')) {
        $alias = Join-Path $fixtureRoot ('alias-'+[guid]::NewGuid().ToString('N'))
        Copy-Item -LiteralPath $valid -Destination $alias -Recurse
        $manifestPath = Join-Path $alias 'bundle.json'
        $manifest = Get-Content -LiteralPath $manifestPath -Raw | ConvertFrom-Json
        $manifest.artifacts[0].path = $badPath
        [IO.File]::WriteAllText($manifestPath, ($manifest | ConvertTo-Json -Depth 6), $utf8)
        Invoke-ExpectedFailure $alias 'noncanonical Windows path'
    }

    $metadataAlias = Join-Path $fixtureRoot 'metadata-alias'
    Copy-Item -LiteralPath $valid -Destination $metadataAlias -Recurse
    $manifestPath = Join-Path $metadataAlias 'bundle.json'
    $manifest = Get-Content -LiteralPath $manifestPath -Raw | ConvertFrom-Json
    $manifest.artifacts[0].path = 'modules.txt'
    $manifest.artifacts[0].size = (Get-Item -LiteralPath (Join-Path $metadataAlias 'modules.txt')).Length
    $manifest.artifacts[0].sha256 = Get-FixtureHash (Join-Path $metadataAlias 'modules.txt')
    [IO.File]::WriteAllText($manifestPath, ($manifest | ConvertTo-Json -Depth 6), $utf8)
    Invoke-ExpectedFailure $metadataAlias 'metadata reused as artifact'

    $rootLink = Join-Path $fixtureRoot 'root-junction'
    $null = New-Item -ItemType Junction -Path $rootLink -Target $valid
    $junctions.Add($rootLink)
    Invoke-ExpectedFailure $rootLink 'bundle-root junction'

    $parentLink = Join-Path $fixtureRoot 'parent-junction'
    $null = New-Item -ItemType Junction -Path $parentLink -Target $fixtureRoot
    $junctions.Add($parentLink)
    Invoke-ExpectedFailure (Join-Path $parentLink 'valid') 'bundle ancestor junction'

    $nested = Join-Path $fixtureRoot 'nested-junction'
    Write-FixtureBundle $nested
    $externalFiles = Join-Path $fixtureRoot 'external-files'
    $null = New-Item -ItemType Directory -Path $externalFiles
    Copy-Item -LiteralPath (Join-Path $valid 'windows-amd64\control-server.exe') -Destination (Join-Path $externalFiles 'control-server.exe')
    $nestedLink = Join-Path $nested 'linked'
    $null = New-Item -ItemType Junction -Path $nestedLink -Target $externalFiles
    $junctions.Add($nestedLink)
    $manifestPath = Join-Path $nested 'bundle.json'
    $manifest = Get-Content -LiteralPath $manifestPath -Raw | ConvertFrom-Json
    $manifest.artifacts[0].path = 'linked/control-server.exe'
    [IO.File]::WriteAllText($manifestPath, ($manifest | ConvertTo-Json -Depth 6), $utf8)
    Invoke-ExpectedFailure $nested 'nested external junction'

    Write-Output 'Server bundle verifier fixtures passed: valid/tampered/missing files, size/inventory/duplicate/path checks, Windows aliases/reserved names, metadata overlap and root/ancestor/nested junctions.'
} finally {
    # Remove junction nodes without traversing their targets before recursive cleanup.
    foreach ($junction in $junctions) {
        if (Test-Path -LiteralPath $junction) { [IO.Directory]::Delete($junction) }
    }
    if (Test-Path -LiteralPath $fixtureRoot) {
        $resolvedFixture = [IO.Path]::GetFullPath($fixtureRoot)
        $expectedPrefix = [IO.Path]::GetFullPath((Join-Path $workspace '.cache')).TrimEnd('\') + '\'
        if (-not $resolvedFixture.StartsWith($expectedPrefix,[StringComparison]::OrdinalIgnoreCase) -or
            [IO.Path]::GetFileName($resolvedFixture) -notlike 'server-bundle-verification-*') { throw 'Refusing cleanup outside the fixture workspace.' }
        Remove-Item -LiteralPath $resolvedFixture -Recurse -Force
    }
}
