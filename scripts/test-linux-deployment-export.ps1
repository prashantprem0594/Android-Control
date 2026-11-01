# Run through run-logged.ps1. Synthetic ELF headers only; no exported binary is executed.
$ErrorActionPreference = 'Stop'
$workspace = Split-Path -Parent $PSScriptRoot
$root = Join-Path $workspace ('.cache/linux-deployment-export-'+[guid]::NewGuid().ToString('N'))
$null = New-Item -ItemType Directory -Path $root
$exporter = Join-Path $PSScriptRoot 'export-linux-deployment.ps1'
$encoding = [Text.UTF8Encoding]::new($false)
$checks = 0
function Write-Text([string]$Path,[string]$Value) {
    $null = [IO.Directory]::CreateDirectory((Split-Path -Parent $Path))
    [IO.File]::WriteAllText($Path,$Value,$encoding)
}
function Hash([string]$Path) { return (Get-FileHash -LiteralPath $Path -Algorithm SHA256).Hash.ToLowerInvariant() }
function Fixture([string]$Name,[string]$Target='linux-amd64',[int]$Machine=62,[int]$ToolCount=4) {
    $directory = Join-Path $root $Name
    $null = [IO.Directory]::CreateDirectory((Join-Path $directory $Target))
    $artifacts = @()
    $names = @('control-server','control-admin','parent-recovery','release-manifest')
    for ($index=0;$index -lt $ToolCount;$index++) {
        $header = New-Object byte[] 64
        $header[0]=127;$header[1]=69;$header[2]=76;$header[3]=70;$header[4]=2;$header[5]=1;$header[6]=1;$header[16]=2;$header[18]=$Machine
        $path = Join-Path $directory ($Target+'/'+$names[$index])
        [IO.File]::WriteAllBytes($path,$header)
        $artifacts += [ordered]@{path=$Target+'/'+$names[$index];size=$header.Length;sha256=(Hash $path)}
    }
    $notice = 'licenses/example.org/fixture@v1.0.0/LICENSE'
    Write-Text (Join-Path $directory $notice) 'Fixture license, not real application code.'
    Write-Text (Join-Path $directory 'modules.txt') "example.org/fixture v1.0.0`n"
    Write-Text (Join-Path $directory 'source-inventory.json') '[{"path":"cmd/control-server/main.go","sha256":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}]'
    $manifest = [ordered]@{schemaVersion=1;cgoEnabled=$false;runtimeAcceptancePassed=$false
        sourceInventorySHA256=(Hash (Join-Path $directory 'source-inventory.json'));modulesSHA256=(Hash (Join-Path $directory 'modules.txt'))
        artifacts=$artifacts;licenseNotices=@([ordered]@{path=$notice;sha256=(Hash (Join-Path $directory $notice))})}
    Write-Text (Join-Path $directory 'bundle.json') ($manifest | ConvertTo-Json -Depth 8)
    # Not listed in the reviewed inventories: these must never enter the export.
    Write-Text (Join-Path $directory '.env') 'PRIVATE-FIXTURE-EXCLUDED'
    Write-Text (Join-Path $directory 'firebase-service-account.json') 'PRIVATE-FIXTURE-EXCLUDED'
    return $directory
}
function Export([string]$SourceBundle,[string]$Output,[string]$Target='linux-amd64',[string]$Digest='') {
    if (-not $Digest) { $Digest = Hash (Join-Path $SourceBundle 'bundle.json') }
    & $exporter -BundleDirectory $SourceBundle -ExpectedBundleSHA256 $Digest -Target $Target -OutputDirectory $Output | Out-Null
}
function Refuses([scriptblock]$Operation,[string]$Output,[string]$Label) {
    $failed=$false
    try { & $Operation } catch { $failed=$true }
    if (-not $failed) { throw "Deployment exporter accepted $Label." }
    if (Test-Path -LiteralPath (Join-Path $Output 'deployment.json')) { throw "Failed $Label export published a completion record." }
    $script:checks++
}
function Verify-Output([string]$Output,[string]$Target) {
    $manifest=Get-Content -LiteralPath (Join-Path $Output 'deployment.json') -Raw | ConvertFrom-Json
    if($manifest.schemaVersion -ne 1 -or $manifest.target -cne $Target -or $manifest.scope -cne 'offline-linux-deployment-export' -or
        $manifest.productionReady -ne $false -or $manifest.runtimeAcceptancePassed -ne $false -or $manifest.credentialsIncluded -ne $false -or $manifest.deploymentPerformed -ne $false){throw 'Export overstated acceptance or changed target.'}
    if(@($manifest.files | Where-Object path -like 'bin/*').Count -ne 4){throw 'Export did not preserve matched four-tool set.'}
    foreach($file in $manifest.files){$path=Join-Path $Output $file.path;if((Hash $path) -cne $file.sha256 -or (Get-Item -LiteralPath $path).Length -ne $file.size){throw 'Export inventory hash/size mismatch.'}}
    foreach($name in @('deploy/app-control.service','deploy/backup.sh','deploy/preflight.sh','deploy/Caddyfile.example','metadata/server-bundle.json','SERVER_HOSTING.md')){
        if(@($manifest.files|Where-Object path -ceq $name).Count -ne 1){throw 'Required deployment asset missing.'}}
    if(Get-ChildItem -LiteralPath $Output -Recurse -File | Where-Object Name -in @('.env','firebase-service-account.json')){throw 'Unlisted private fixture file was copied.'}
    $actual=@(Get-ChildItem -LiteralPath $Output -Recurse -File)
    if($actual.Count -ne $manifest.files.Count+1){throw 'Export contains untracked output files.'}
    $script:checks++
}

$valid = Fixture 'valid'
$output = Join-Path $root 'output-amd64'
Export $valid $output
Verify-Output $output 'linux-amd64'
$arm = Fixture 'arm' 'linux-arm64' 183
$armOutput = Join-Path $root 'output-arm64'
Export $arm $armOutput 'linux-arm64'
Verify-Output $armOutput 'linux-arm64'

$wrongDigest = Join-Path $root 'wrong-digest'
Refuses { Export $valid $wrongDigest 'linux-amd64' ('0'*64) } $wrongDigest 'wrong trusted digest'
$tampered = Fixture 'tampered'
[IO.File]::AppendAllText((Join-Path $tampered 'linux-amd64/control-server'),'changed')
$tamperedOutput=Join-Path $root 'tampered-output'
Refuses { Export $tampered $tamperedOutput } $tamperedOutput 'tampered tool'
$missing = Fixture 'missing-tool' 'linux-amd64' 62 3
$missingOutput=Join-Path $root 'missing-output'
Refuses { Export $missing $missingOutput } $missingOutput 'incomplete matching tool set'
$missingFile = Fixture 'declared-file-missing'
$missingFilePath = Join-Path $missingFile 'linux-amd64/control-admin'
[IO.File]::Move($missingFilePath,($missingFilePath+'.fixture-unavailable'))
$missingFileOutput = Join-Path $root 'declared-file-missing-output'
Refuses { Export $missingFile $missingFileOutput } $missingFileOutput 'missing declared artifact'
$wrongArch=Fixture 'wrong-architecture' 'linux-amd64' 183
$wrongArchOutput=Join-Path $root 'wrong-arch-output'
Refuses { Export $wrongArch $wrongArchOutput } $wrongArchOutput 'incorrect ELF architecture'
$existing=Join-Path $root 'existing'
$null=New-Item -ItemType Directory -Path $existing
Write-Text (Join-Path $existing 'sentinel') 'unchanged'
Refuses { Export $valid $existing } $existing 'existing output'
if((Get-Content -LiteralPath (Join-Path $existing 'sentinel') -Raw) -cne 'unchanged'){throw 'Existing output was modified.'}
$traversal=Join-Path $root 'new/../escaped'
Refuses { Export $valid $traversal } (Join-Path $root 'escaped') 'traversal output path'
$aliased=Join-Path $root 'aliased.'
Refuses { Export $valid $aliased } $aliased 'aliased output path'
$unknown=Fixture 'unknown-metadata'
$unknownManifest=Get-Content -LiteralPath (Join-Path $unknown 'bundle.json') -Raw | ConvertFrom-Json
$unknownManifest | Add-Member -NotePropertyName privateCredential -NotePropertyValue 'PRIVATE-FIXTURE-EXCLUDED'
Write-Text (Join-Path $unknown 'bundle.json') ($unknownManifest|ConvertTo-Json -Depth 8)
$unknownOutput=Join-Path $root 'unknown-output'
Refuses { Export $unknown $unknownOutput } $unknownOutput 'unrecognized metadata/private field'
$unknownInventory=Fixture 'unknown-source-metadata'
Write-Text (Join-Path $unknownInventory 'source-inventory.json') '[{"path":"go.mod","sha256":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","privateCredential":"PRIVATE-FIXTURE-EXCLUDED"}]'
$unknownInventoryManifest=Get-Content -LiteralPath (Join-Path $unknownInventory 'bundle.json') -Raw | ConvertFrom-Json
$unknownInventoryManifest.sourceInventorySHA256=Hash (Join-Path $unknownInventory 'source-inventory.json')
Write-Text (Join-Path $unknownInventory 'bundle.json') ($unknownInventoryManifest|ConvertTo-Json -Depth 8)
$unknownInventoryOutput=Join-Path $root 'unknown-source-output'
Refuses { Export $unknownInventory $unknownInventoryOutput } $unknownInventoryOutput 'private field in source metadata'

$inputLink=Join-Path $root 'input-link'
$null=New-Item -ItemType Junction -Path $inputLink -Target $valid
$inputLinkOutput=Join-Path $root 'input-link-output'
Refuses { Export $inputLink $inputLinkOutput } $inputLinkOutput 'junction source root'
$outputParent=Join-Path $root 'output-parent'
$null=New-Item -ItemType Directory -Path $outputParent
$outputLink=Join-Path $root 'output-link'
$null=New-Item -ItemType Junction -Path $outputLink -Target $outputParent
Refuses { Export $valid (Join-Path $outputLink 'new') } (Join-Path $outputParent 'new') 'junction output ancestor'

Write-Output "Linux deployment export checks passed: $checks scenarios; matching AMD64/ARM64 sets, copied hashes, private-file exclusion, tamper/missing/digest/ELF/existing/traversal/alias/metadata/junction refusals. Synthetic ELF bytes were never executed."
Write-Output "Retained fixture evidence: $root"
