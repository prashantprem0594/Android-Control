param(
    [Parameter(Mandatory)] [string]$InputDirectory,
    [Parameter(Mandatory)] [string]$KeystoreFile,
    [Parameter(Mandatory)] [ValidatePattern('^[A-Za-z0-9._-]{1,128}$')] [string]$KeyAlias,
    [Parameter(Mandatory)] [ValidatePattern('^[A-Fa-f0-9]{64}$')] [string]$ExpectedCertificateSHA256,
    [Parameter(Mandatory)] [string]$OutputDirectory,
    [Security.SecureString]$StorePassword,
    [Security.SecureString]$KeyPassword
)
$ErrorActionPreference = 'Stop'
$workspaceRoot = Split-Path -Parent $PSScriptRoot
$toolsRoot = Join-Path $workspaceRoot '.tooling\android-sdk\build-tools\36.0.0'
$signer = Join-Path $toolsRoot 'apksigner.bat'
$aapt = Join-Path $toolsRoot 'aapt.exe'
function Invoke-PrivateTool([string]$Executable, [string[]]$Arguments, [string[]]$PrivateInput = @()) {
    # Native stderr must not abort collection under Windows PowerShell. Output
    # remains private to the caller, which reports only a fixed failure summary.
    $ErrorActionPreference = 'Continue'
    if ($PrivateInput.Count -gt 0) { $captured = @($PrivateInput | & $Executable @Arguments 2>&1) }
    else { $captured = @(& $Executable @Arguments 2>&1) }
    return [pscustomobject]@{ ExitCode = $LASTEXITCODE; Output = $captured }
}
if (-not (Test-Path -LiteralPath $KeystoreFile -PathType Leaf)) { throw 'Signing keystore is missing.' }
if (Test-Path -LiteralPath $OutputDirectory) { throw 'Signing output directory must not exist.' }
$inputs = @{}
$versions = @{}
foreach ($app in @('parent', 'child')) {
    $path = Join-Path $InputDirectory ($app + '-unsigned.apk')
    if (-not (Test-Path -LiteralPath $path -PathType Leaf)) { throw 'Both unsigned APKs are required.' }
    $badging = @(& $aapt dump badging $path 2>&1) -join "`n"
    if ($LASTEXITCODE -ne 0 -or -not $badging.Contains("package: name='com.appcontrol.$app'") -or
        $badging.Contains('application-debuggable')) { throw 'Expected non-debuggable parent and child APKs are required.' }
    $inputs[$app] = $path
    $code = [regex]::Match($badging, "versionCode='([0-9]+)'")
    $name = [regex]::Match($badging, "versionName='([^']+)'")
    if (-not $code.Success -or -not $name.Success) { throw 'APK version metadata is missing.' }
    $versions[$app] = @{code=[int]$code.Groups[1].Value;name=$name.Groups[1].Value}
}
if ($null -eq $StorePassword) { $StorePassword = Read-Host 'Keystore password' -AsSecureString }
if ($null -eq $KeyPassword) { $KeyPassword = Read-Host 'Signing key password (repeat keystore password if shared)' -AsSecureString }
$storePointer = [IntPtr]::Zero
$keyPointer = [IntPtr]::Zero
$previousOutputEncoding = $OutputEncoding
try {
    $OutputEncoding = [Text.UTF8Encoding]::new($false)
    $storePointer = [Runtime.InteropServices.Marshal]::SecureStringToBSTR($StorePassword)
    $keyPointer = [Runtime.InteropServices.Marshal]::SecureStringToBSTR($KeyPassword)
    $storeText = [Runtime.InteropServices.Marshal]::PtrToStringBSTR($storePointer)
    $keyText = [Runtime.InteropServices.Marshal]::PtrToStringBSTR($keyPointer)
    if ($storeText.Contains("`n") -or $storeText.Contains("`r") -or $keyText.Contains("`n") -or $keyText.Contains("`r")) { throw 'Signing passwords must not contain line breaks.' }
    New-Item -ItemType Directory -Path $OutputDirectory -ErrorAction Stop | Out-Null
    $records = @()
    foreach ($app in @('parent', 'child')) {
        $destination = Join-Path $OutputDirectory ($app + '-release.apk')
        # Passwords use UTF-8 stdin; never arguments, environment or files.
        $signResult = Invoke-PrivateTool -Executable $signer -PrivateInput @($storeText, $keyText) -Arguments @('sign', '--ks', $KeystoreFile, '--ks-key-alias', $KeyAlias, '--ks-pass', 'stdin', '--key-pass', 'stdin', '--pass-encoding', 'utf-8', '--v1-signing-enabled', 'false', '--v2-signing-enabled', 'true', '--v3-signing-enabled', 'true', '--v4-signing-enabled', 'false', '--debuggable-apk-permitted', 'false', '--min-sdk-version', '33', '--in', $inputs[$app], '--out', $destination)
        if ($signResult.ExitCode -ne 0) { throw 'APK signing failed; no artifact is approved for distribution.' }
        $verifyResult = Invoke-PrivateTool -Executable $signer -Arguments @('verify', '--verbose', '--print-certs', '--min-sdk-version', '33', $destination)
        if ($verifyResult.ExitCode -ne 0) { throw 'APK signature verification failed.' }
        $verification = $verifyResult.Output -join "`n"
        $certificates = [regex]::Matches($verification, '(?m)^Signer #[0-9]+ certificate SHA-256 digest: ([a-fA-F0-9]{64})\r?$')
        if ($certificates.Count -ne 1 -or $certificates[0].Groups[1].Value.ToLowerInvariant() -cne $ExpectedCertificateSHA256.ToLowerInvariant() -or
            $verification.Contains('CN=Android Debug')) { throw 'APK signer differs from the expected non-debug release certificate.' }
        $records += [ordered]@{
            app = $app
            file = $app + '-release.apk'
            versionCode = $versions[$app].code
            versionName = $versions[$app].name
            unsignedSHA256 = (Get-FileHash -LiteralPath $inputs[$app] -Algorithm SHA256).Hash.ToLowerInvariant()
            signedSHA256 = (Get-FileHash -LiteralPath $destination -Algorithm SHA256).Hash.ToLowerInvariant()
            signerCertificateSHA256 = $certificates[0].Groups[1].Value.ToLowerInvariant()
        }
    }
    [ordered]@{signedAtUtc=[DateTime]::UtcNow.ToString('o');apps=$records;scope='Locally signed APK inventory; this JSON is not a signed update manifest and nothing was published.'} |
        ConvertTo-Json -Depth 6 | Set-Content -LiteralPath (Join-Path $OutputDirectory 'signed-apks.json') -Encoding UTF8
    Write-Output 'Parent and child APK signatures match the expected release certificate. No artifacts published.'
} finally {
    $OutputEncoding = $previousOutputEncoding
    if ($storePointer -ne [IntPtr]::Zero) { [Runtime.InteropServices.Marshal]::ZeroFreeBSTR($storePointer) }
    if ($keyPointer -ne [IntPtr]::Zero) { [Runtime.InteropServices.Marshal]::ZeroFreeBSTR($keyPointer) }
    $storeText = $null; $keyText = $null; $signResult = $null
}
