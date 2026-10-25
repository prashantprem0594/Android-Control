param([Parameter(Mandatory)] [string]$InputDirectory)
# Disposable local release authority only. Run through run-logged.ps1.
$ErrorActionPreference = 'Stop'
$workspace = Split-Path -Parent $PSScriptRoot
$root = Join-Path $workspace ('.cache\setup-roundtrip-' + [guid]::NewGuid().ToString('N'))
$null = New-Item -ItemType Directory -Path $root
$keystore = Join-Path $root 'fixture.p12'
$certificate = Join-Path $root 'fixture.cer'
$seed = Join-Path $root 'metadata.seed'
$publicKey = Join-Path $root 'metadata.pub'
$keytool = 'C:\Program Files\Java\jdk-17\bin\keytool.exe'
$manifestTool = Join-Path $workspace '.cache\release-manifest.exe'
$origin = 'https://setup-fixture.invalid'
function Invoke-FixtureTool([string]$Tool,[string[]]$Arguments) {
    $ErrorActionPreference = 'Continue'
    $captured = @(& $Tool @Arguments 2>&1)
    if ($LASTEXITCODE -ne 0) { throw 'Disposable fixture tool failed.' }
}
$randomBytes = New-Object byte[] 32
$random = [Security.Cryptography.RandomNumberGenerator]::Create()
try { $random.GetBytes($randomBytes) } finally { $random.Dispose() }
$savedPassword = $env:APP_CONTROL_SETUP_FIXTURE_PASSWORD
$env:APP_CONTROL_SETUP_FIXTURE_PASSWORD = [Convert]::ToBase64String($randomBytes)
[Array]::Clear($randomBytes,0,$randomBytes.Length)
$secure = ConvertTo-SecureString $env:APP_CONTROL_SETUP_FIXTURE_PASSWORD -AsPlainText -Force
try {
    Invoke-FixtureTool $keytool @('-genkeypair','-alias','setup-fixture','-keystore',$keystore,'-storetype','PKCS12','-keyalg','RSA','-keysize','2048','-dname','CN=App Control Disposable Setup Fixture','-validity','2','-storepass:env','APP_CONTROL_SETUP_FIXTURE_PASSWORD','-keypass:env','APP_CONTROL_SETUP_FIXTURE_PASSWORD','-noprompt')
    Invoke-FixtureTool $keytool @('-exportcert','-alias','setup-fixture','-keystore',$keystore,'-file',$certificate,'-storepass:env','APP_CONTROL_SETUP_FIXTURE_PASSWORD')
    $digest = (Get-FileHash -LiteralPath $certificate -Algorithm SHA256).Hash.ToLowerInvariant()
    $signed = Join-Path $root 'signed'
    & (Join-Path $PSScriptRoot 'sign-private-apks.ps1') -InputDirectory $InputDirectory -KeystoreFile $keystore -KeyAlias setup-fixture -ExpectedCertificateSHA256 $digest -OutputDirectory $signed -StorePassword $secure -KeyPassword $secure
    Invoke-FixtureTool $manifestTool @('keygen','-private-key',$seed,'-public-key',$publicKey)
    $bundle = Join-Path $root 'release'
    & (Join-Path $PSScriptRoot 'stage-private-release.ps1') -InputDirectory $signed -OutputDirectory $bundle -ServerOrigin $origin -Sequence 902 -ExpectedCertificateSHA256 $digest -MetadataPrivateKeyFile $seed -MetadataPublicKeyFile $publicKey
    $output = Join-Path $root 'setup'
    $arguments = @{BundleDirectory=$bundle;MetadataPublicKeyFile=$publicKey;ServerOrigin=$origin;ExpectedCertificateSHA256=$digest;MinimumSequence=902}
    & (Join-Path $PSScriptRoot 'export-managed-setup.ps1') @arguments -OutputDirectory $output
    $report = Get-Content -LiteralPath (Join-Path $output 'setup-bundle.json') -Raw | ConvertFrom-Json
    foreach ($entry in @(@('provisioning.json','payloadSHA256'),@('provisioning.png','pngSHA256'))) {
        if ((Get-FileHash -LiteralPath (Join-Path $output $entry[0]) -Algorithm SHA256).Hash.ToLowerInvariant() -cne $report.($entry[1])) { throw 'Setup output inventory hash mismatch.' }
    }
    if (-not $report.softwareRoundTripVerified -or $report.deviceScanVerified -or $report.deviceProvisioned) { throw 'Incorrect evidence boundary in setup report.' }
    $payload = Get-Content -LiteralPath (Join-Path $output 'provisioning.json') -Raw | ConvertFrom-Json
    $apkHash = (Get-FileHash -LiteralPath (Join-Path $bundle 'releases\902\child-release.apk') -Algorithm SHA256).Hash.ToLowerInvariant()
    $hashBytes = [byte[]]::new(32)
    for ($i=0;$i -lt 32;$i++) { $hashBytes[$i] = [Convert]::ToByte($apkHash.Substring(2*$i,2),16) }
    $checksum = [Convert]::ToBase64String($hashBytes).TrimEnd('=').Replace('+','-').Replace('/','_')
    if ($payload.'android.app.extra.PROVISIONING_DEVICE_ADMIN_PACKAGE_CHECKSUM' -cne $checksum -or
        $payload.'android.app.extra.PROVISIONING_ADMIN_EXTRAS_BUNDLE'.'appcontrol.serverOrigin' -cne $origin -or
        $payload.'android.app.extra.PROVISIONING_DEVICE_ADMIN_PACKAGE_DOWNLOAD_LOCATION' -cne ($origin+'/releases/902/child-release.apk')) { throw 'Provisioning payload differs from verified release.' }
    # A newer required sequence must refuse this otherwise valid bundle.
    $arguments.MinimumSequence = 903
    $refused = $false
    try { & (Join-Path $PSScriptRoot 'export-managed-setup.ps1') @arguments -OutputDirectory (Join-Path $root 'stale') }
    catch { if ($_.Exception.Message -notlike '*independently verified release metadata*') { throw }; $refused = $true }
    if (-not $refused) { throw 'Old release sequence was accepted.' }
    $arguments.MinimumSequence = 902
    # Preserve the successful release evidence; mutate a separate fixture copy.
    $tamperedBundle = Join-Path $root 'tampered-release'
    Copy-Item -LiteralPath $bundle -Destination $tamperedBundle -Recurse
    $arguments.BundleDirectory = $tamperedBundle
    $apk = Join-Path $tamperedBundle 'releases\902\child-release.apk'
    $stream = [IO.File]::Open($apk,[IO.FileMode]::Append,[IO.FileAccess]::Write)
    try { $stream.WriteByte(0) } finally { $stream.Dispose() }
    $refused = $false
    try { & (Join-Path $PSScriptRoot 'export-managed-setup.ps1') @arguments -OutputDirectory (Join-Path $root 'tampered') }
    catch { if ($_.Exception.Message -notlike '*Child APK differs from signed metadata*') { throw }; $refused = $true }
    if (-not $refused) { throw 'Tampered APK was accepted.' }
    foreach ($failed in @('stale','tampered')) { if (Test-Path -LiteralPath (Join-Path $root ($failed+'\setup-bundle.json'))) { throw 'Refused export has a completion inventory.' } }
    Write-Output 'Current signed setup export, independent QR decode, payload binding, inventory hashes, stale-sequence refusal and tampered-APK refusal pass. No device operations.'
    Write-Output "Evidence directory: $root"
} finally {
    $secure.Dispose()
    if ($null -eq $savedPassword) { Remove-Item Env:\APP_CONTROL_SETUP_FIXTURE_PASSWORD -ErrorAction SilentlyContinue }
    else { $env:APP_CONTROL_SETUP_FIXTURE_PASSWORD = $savedPassword }
    foreach ($privateFile in @($keystore,$seed)) { if (Test-Path -LiteralPath $privateFile) { Remove-Item -LiteralPath $privateFile -Force } }
}
