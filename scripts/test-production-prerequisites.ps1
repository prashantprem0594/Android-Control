# Invoke through run-logged.ps1. Synthetic public fixtures only.
$ErrorActionPreference = 'Stop'
$workspace = Split-Path -Parent $PSScriptRoot
$root = Join-Path $workspace ('.cache\production-prerequisites-' + [guid]::NewGuid().ToString('N'))
$null = New-Item -ItemType Directory -Path $root
$checker = Join-Path $PSScriptRoot 'check-production-prerequisites.ps1'
$empty = (& $checker | ConvertFrom-Json)
if($empty.inputInventoryComplete -or $empty.productionReady -or @($empty.checks | Where-Object passed).Count){throw 'Absent production inputs incorrectly accepted.'}
$marker = 'sensitive-input-marker-do-not-print'
$bad = Join-Path $root 'bad.json'
[IO.File]::WriteAllText($bad,('{"privateKey":"'+$marker+'"}'))
$badOutput = (& $checker -ConfigurationFile $bad) -join "`n"
if($badOutput.Contains($marker) -or $badOutput.Contains($bad)){throw 'Input contents or paths leaked.'}
$badReport = $badOutput | ConvertFrom-Json
if($badReport.checks[1].passed -or $badReport.inputInventoryComplete){throw 'Malformed configuration accepted.'}
$key = [byte[]]@(0x30,0x2a,0x30,0x05,0x06,0x03,0x2b,0x65,0x70,0x03,0x21,0x00) + [byte[]](1..32)
$config = [ordered]@{serverOrigin='https://deployment-readiness.org';currentPin=[Convert]::ToBase64String([byte[]](1..32));backupPin=[Convert]::ToBase64String([byte[]](33..64));apkCertificateSHA256=('a'*64);updatePublicKey=[Convert]::ToBase64String($key);updateMinimumSequence=1;versionCode=4;versionName='readiness-fixture';remoteCallsEnabled=$false;firebase=$null;parentFirebase=$null}
$good = Join-Path $root 'public.json'
[IO.File]::WriteAllText($good,($config | ConvertTo-Json -Depth 5))
# Placeholder files demonstrate presence only, and must never imply valid credentials.
$placeholder = Join-Path $root 'placeholder'
[IO.File]::WriteAllText($placeholder,$marker)
$complete = & $checker -ConfigurationFile $good -ApkSigningKeystore $placeholder -UpdateSigningKeyFile $placeholder -ServerConfigurationFile $placeholder | ConvertFrom-Json
if(-not $complete.inputInventoryComplete -or $complete.productionReady -or @($complete.remainingAcceptance).Count -lt 4){throw 'Inventory confused with production acceptance.'}
$config.serverOrigin='https://fixture.invalid'
[IO.File]::WriteAllText($good,($config | ConvertTo-Json -Depth 5))
$fixture = & $checker -ConfigurationFile $good | ConvertFrom-Json
if($fixture.checks[1].passed){throw 'Fixture origin accepted as deployment input.'}
Write-Output 'production-prerequisites checks passed: absent inputs, malformed/private data redaction, structural validation, fixture refusal and file-presence/acceptance boundary.'
