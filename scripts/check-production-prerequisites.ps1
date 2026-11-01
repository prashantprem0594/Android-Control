param(
    [string]$ConfigurationFile,
    [string]$ApkSigningKeystore,
    [string]$UpdateSigningKeyFile,
    [string]$ServerConfigurationFile
)
# Read-only inventory. Private input bytes, paths and configuration values are never printed.
# File presence is not credential validity or proof of custody.
$ErrorActionPreference = 'Stop'
$checks = @()
function Add-Check([string]$Name,[bool]$Passed,[string]$Action) {
    $script:checks += [ordered]@{name=$Name;passed=$Passed;action=$(if($Passed){''}else{$Action})}
}
function Has-File([string]$Path) {
    if ([string]::IsNullOrWhiteSpace($Path)) { return $false }
    try { return (Get-Item -LiteralPath $Path -ErrorAction Stop) -is [IO.FileInfo] } catch { return $false }
}
$publicFileExists = Has-File $ConfigurationFile
Add-Check 'publicConfigurationFile' $publicFileExists 'Supply ConfigurationFile: public JSON containing the controlled HTTPS origin, distinct current/backup SPKI pins, APK certificate digest, update public authority, version/sequence, rollout flag and independent optional Firebase settings.'
$valid = $false
if ($publicFileExists) {
    try {
        & (Join-Path $PSScriptRoot 'export-production-configuration.ps1') -ConfigurationFile $ConfigurationFile -ValidateOnly | Out-Null
        $valid = $true
    } catch {
        # Never echo parser errors or input values; an incorrectly supplied file could contain secrets.
    }
}
Add-Check 'publicConfigurationStructure' $valid 'Correct the public JSON using docs/PRODUCTION_CONFIGURATION.md. Reserved fixture hosts, unknown/private fields and inconsistent Firebase app/sender settings are refused. Validation does not establish control of supplied identities.'
Add-Check 'apkSigningKeystoreFilePresent' (Has-File $ApkSigningKeystore) 'Supply ApkSigningKeystore pointing to the existing intended signing identity; keep its passwords outside logs and source control. Presence alone does not verify the signer.'
Add-Check 'updateSigningKeyFilePresent' (Has-File $UpdateSigningKeyFile) 'Supply UpdateSigningKeyFile pointing to the existing private update authority matching the public key. Presence alone does not verify key matching or custody.'
Add-Check 'serverConfigurationFilePresent' (Has-File $ServerConfigurationFile) 'Supply ServerConfigurationFile pointing to the deployment configuration stored outside source control. Contents, credentials, permissions and runtime behavior are not inspected.'
[ordered]@{
    schemaVersion=1
    scope='read-only-production-input-inventory'
    inputInventoryComplete=(@($checks | Where-Object { -not $_.passed }).Count -eq 0)
    productionReady=$false
    checks=$checks
    remainingAcceptance=@(
        'Verify HTTPS hostname/CA trust, controlled current and backup pins, and certificate rotation.',
        'Verify intended APK signer and matching private update authority, then enrolled-device upgrade continuity.',
        'If Firebase is enabled, verify app/package registration, authorized sender credentials, token lifecycle and real delivery.',
        'Deploy and exercise backup restore/recovery on the intended host; complete physical Redmi/SIM and elapsed pilot acceptance.'
    )
} | ConvertTo-Json -Depth 6
