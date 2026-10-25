param([Parameter(Mandatory)] [string]$InputDirectory)
$ErrorActionPreference = 'Stop'
$workspaceRoot = Split-Path -Parent $PSScriptRoot
$fixtureRoot = Join-Path $workspaceRoot ('.cache\signing-fixture-' + [Guid]::NewGuid().ToString('N'))
New-Item -ItemType Directory -Path $fixtureRoot -ErrorAction Stop | Out-Null
$keystore = Join-Path $fixtureRoot 'fixture.p12'
$certificate = Join-Path $fixtureRoot 'fixture.cer'
$keytool = 'C:\Program Files\Java\jdk-17\bin\keytool.exe'
$signScript = Join-Path $PSScriptRoot 'sign-private-apks.ps1'
function Invoke-FixtureKeytool([string[]]$Arguments) {
    $ErrorActionPreference = 'Continue'
    $captured = @(& $keytool @Arguments 2>&1)
    if ($LASTEXITCODE -ne 0) { throw 'Fixture signing key preparation failed.' }
}
$bytes = New-Object byte[] 32
$random = [Security.Cryptography.RandomNumberGenerator]::Create()
try { $random.GetBytes($bytes) } finally { $random.Dispose() }
$previousFixturePassword = $env:APP_CONTROL_FIXTURE_SIGNING_PASSWORD
$env:APP_CONTROL_FIXTURE_SIGNING_PASSWORD = [Convert]::ToBase64String($bytes)
[Array]::Clear($bytes, 0, $bytes.Length)
$secure = ConvertTo-SecureString $env:APP_CONTROL_FIXTURE_SIGNING_PASSWORD -AsPlainText -Force
try {
    Invoke-FixtureKeytool -Arguments @('-genkeypair', '-alias', 'isolated-fixture', '-keystore', $keystore, '-storetype', 'PKCS12', '-keyalg', 'RSA', '-keysize', '2048', '-dname', 'CN=App Control Isolated Signing Fixture', '-validity', '2', '-storepass:env', 'APP_CONTROL_FIXTURE_SIGNING_PASSWORD', '-keypass:env', 'APP_CONTROL_FIXTURE_SIGNING_PASSWORD', '-noprompt')
    Invoke-FixtureKeytool -Arguments @('-exportcert', '-alias', 'isolated-fixture', '-keystore', $keystore, '-file', $certificate, '-storepass:env', 'APP_CONTROL_FIXTURE_SIGNING_PASSWORD')
    $digest = (Get-FileHash -LiteralPath $certificate -Algorithm SHA256).Hash
    $wrongOutput = Join-Path $fixtureRoot 'wrong-certificate'
    $rejected = $false
    try { & $signScript -InputDirectory $InputDirectory -KeystoreFile $keystore -KeyAlias isolated-fixture -ExpectedCertificateSHA256 ('0' * 64) -OutputDirectory $wrongOutput -StorePassword $secure -KeyPassword $secure }
    catch { $rejected = $true }
    if (-not $rejected -or (Test-Path -LiteralPath (Join-Path $wrongOutput 'signed-apks.json'))) { throw 'Wrong signer certificate was approved.' }
    $goodOutput = Join-Path $fixtureRoot 'verified'
    & $signScript -InputDirectory $InputDirectory -KeystoreFile $keystore -KeyAlias isolated-fixture -ExpectedCertificateSHA256 $digest -OutputDirectory $goodOutput -StorePassword $secure -KeyPassword $secure
    $record = Get-Content -LiteralPath (Join-Path $goodOutput 'signed-apks.json') -Raw | ConvertFrom-Json
    if ($record.apps.Count -ne 2) { throw 'Signing inventory is incomplete.' }
    foreach ($app in $record.apps) {
        $actual = (Get-FileHash -LiteralPath (Join-Path $goodOutput $app.file) -Algorithm SHA256).Hash.ToLowerInvariant()
        if ($actual -cne $app.signedSHA256 -or $app.signerCertificateSHA256 -cne $digest.ToLowerInvariant()) { throw 'Signed artifact hash or certificate differs from inventory.' }
    }
    $rejected = $false
    try { & $signScript -InputDirectory $InputDirectory -KeystoreFile $keystore -KeyAlias isolated-fixture -ExpectedCertificateSHA256 $digest -OutputDirectory $goodOutput -StorePassword $secure -KeyPassword $secure }
    catch { $rejected = $true }
    if (-not $rejected) { throw 'Existing signing output was overwritten.' }
    Write-Output 'Isolated APK signing, expected-certificate refusal, artifact hashes and overwrite refusal passed. No production key used.'
    Write-Output "Signing fixture evidence: $fixtureRoot"
} finally {
    if ($null -eq $previousFixturePassword) { Remove-Item Env:\APP_CONTROL_FIXTURE_SIGNING_PASSWORD -ErrorAction SilentlyContinue }
    else { $env:APP_CONTROL_FIXTURE_SIGNING_PASSWORD = $previousFixturePassword }
    $secure.Dispose()
    if (Test-Path -LiteralPath $keystore) { Remove-Item -LiteralPath $keystore -Force -ErrorAction Stop }
}
