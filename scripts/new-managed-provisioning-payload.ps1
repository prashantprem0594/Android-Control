param(
    [Parameter(Mandatory)] [string]$BundleDirectory,
    [Parameter(Mandatory)] [string]$MetadataPublicKeyFile,
    [Parameter(Mandatory)] [string]$ServerOrigin,
    [Parameter(Mandatory)] [ValidatePattern('^[a-f0-9]{64}$')] [string]$ExpectedCertificateSHA256,
    [Parameter(Mandatory)] [string]$OutputFile,
    [ValidateRange(0,2100000000)] [int]$MinimumSequence = 0
)
# Produces public QR payload JSON only. It cannot reset, enroll or provision a
# device, contains no Wi-Fi/password/pairing secret, and never uses ADB.
$ErrorActionPreference = 'Stop'
$workspaceRoot = Split-Path -Parent $PSScriptRoot
if (Test-Path -LiteralPath $OutputFile) { throw 'Provisioning payload output must not exist.' }
$manifest = Join-Path $BundleDirectory 'release-manifest.json'
$cli = Join-Path $workspaceRoot '.cache\release-manifest.exe'
$ErrorActionPreference = 'Continue'
$verificationOutput = @(& $cli verify -input $manifest -public-key $MetadataPublicKeyFile -origin $ServerOrigin -certificate-sha256 $ExpectedCertificateSHA256 -minimum-sequence $MinimumSequence 2>&1)
$verificationExit = $LASTEXITCODE
$ErrorActionPreference = 'Stop'
if ($verificationExit -ne 0) { throw 'Provisioning requires independently verified release metadata.' }
$envelope = Get-Content -LiteralPath $manifest -Raw -Encoding UTF8 | ConvertFrom-Json
$encoded = $envelope.payload.Replace('-','+').Replace('_','/')
while ($encoded.Length % 4 -ne 0) { $encoded += '=' }
$metadata = [Text.Encoding]::UTF8.GetString([Convert]::FromBase64String($encoded)) | ConvertFrom-Json
$children = @($metadata.apps | Where-Object { $_.package -ceq 'com.appcontrol.child' })
if ($children.Count -ne 1) { throw 'Expected exactly one child release.' }
$child = $children[0]
$apk = Join-Path $BundleDirectory ('releases\'+$metadata.sequence+'\child-release.apk')
$info = Get-Item -LiteralPath $apk
if ($info.Length -ne $child.bytes -or (Get-FileHash -LiteralPath $apk -Algorithm SHA256).Hash.ToLowerInvariant() -cne $child.sha256) { throw 'Child APK differs from signed metadata.' }
$signer = Join-Path $workspaceRoot '.tooling\android-sdk\build-tools\36.0.0\apksigner.bat'
$savedJava = $env:JAVA_HOME
try {
    $env:JAVA_HOME = 'C:\Program Files\Java\jdk-17'
    $ErrorActionPreference = 'Continue'
    $verification = @(& $signer verify --print-certs --min-sdk-version 33 $apk 2>&1)
    $exitCode = $LASTEXITCODE
    $ErrorActionPreference = 'Stop'
    $certificates = [regex]::Matches(($verification -join "`n"),'(?m)^Signer #[0-9]+ certificate SHA-256 digest: ([a-fA-F0-9]{64})\r?$')
    if ($exitCode -ne 0 -or $certificates.Count -ne 1 -or $certificates[0].Groups[1].Value.ToLowerInvariant() -cne $ExpectedCertificateSHA256 -or ($verification -join "`n").Contains('CN=Android Debug')) { throw 'Child APK does not have the expected release signing identity.' }
    $aapt = Join-Path $workspaceRoot '.tooling\android-sdk\build-tools\36.0.0\aapt.exe'
    $ErrorActionPreference = 'Continue'
    $dump = @(& $aapt dump xmltree $apk AndroidManifest.xml 2>&1)
    $dumpExit = $LASTEXITCODE
    $ErrorActionPreference = 'Stop'
    if ($dumpExit -ne 0) { throw 'Could not inspect managed-setup manifest.' }
    $text = $dump -join "`n"
    if ($text -match 'android:(debuggable|testOnly)[^\r\n]*0xffffffff') { throw 'Managed-setup APK must be a non-debug release.' }
    $activities = [regex]::Matches($text,'(?ms)^(?<indent> *)E: activity\b.*?(?=^\k<indent>E: |\z)')
    $handlers = @{
        'com.appcontrol.child.provisioning.ProvisioningModeActivity'='android.app.action.GET_PROVISIONING_MODE'
        'com.appcontrol.child.provisioning.PolicyComplianceActivity'='android.app.action.ADMIN_POLICY_COMPLIANCE'
    }
    foreach ($component in $handlers.Keys) {
        $matching = @($activities | Where-Object {
            $_.Value.Contains('"'+$component+'"') -and $_.Value.Contains('"'+$handlers[$component]+'"') -and
            $_.Value -match 'android:permission[^\r\n]*"android.permission.BIND_DEVICE_ADMIN"' -and
            $_.Value -match 'android:exported[^\r\n]*0xffffffff'
        })
        if ($matching.Count -ne 1) { throw 'Child APK lacks a required protected managed-setup handler.' }
    }
} finally { $env:JAVA_HOME = $savedJava }
$hash = [byte[]]::new(32)
for ($i=0;$i -lt 32;$i++) { $hash[$i]=[Convert]::ToByte($child.sha256.Substring($i*2,2),16) }
$checksum = [Convert]::ToBase64String($hash).TrimEnd('=').Replace('+','-').Replace('/','_')
$payload = [ordered]@{
    'android.app.extra.PROVISIONING_DEVICE_ADMIN_COMPONENT_NAME'='com.appcontrol.child/com.appcontrol.child.admin.AppControlDeviceAdminReceiver'
    'android.app.extra.PROVISIONING_DEVICE_ADMIN_PACKAGE_DOWNLOAD_LOCATION'=$child.url
    'android.app.extra.PROVISIONING_DEVICE_ADMIN_PACKAGE_CHECKSUM'=$checksum
    'android.app.extra.PROVISIONING_LEAVE_ALL_SYSTEM_APPS_ENABLED'=$true
    'android.app.extra.PROVISIONING_ADMIN_EXTRAS_BUNDLE'=@{'appcontrol.serverOrigin'=$ServerOrigin}
}
$bytes = [Text.UTF8Encoding]::new($false).GetBytes(($payload | ConvertTo-Json -Depth 5 -Compress))
$output = [IO.File]::Open([IO.Path]::GetFullPath($OutputFile),[IO.FileMode]::CreateNew,[IO.FileAccess]::Write,[IO.FileShare]::None)
try { $output.Write($bytes,0,$bytes.Length);$output.Flush($true) } finally { $output.Dispose() }
Write-Output 'Public managed-setup payload generated. QR rendering and physical setup acceptance remain separate; no device action performed.'
