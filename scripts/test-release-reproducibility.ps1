param(
    [Parameter(Mandatory)] [string]$ServerOrigin,
    [Parameter(Mandatory)] [string]$CurrentPin,
    [Parameter(Mandatory)] [string]$BackupPin,
    [ValidateRange(1, 2100000000)] [int]$VersionCode = 1,
    [ValidatePattern('^[A-Za-z0-9][A-Za-z0-9._+\-]{0,63}$')] [string]$VersionName = '0.1.0',
    [string]$UpdatePublicKey = '',
    [string]$UpdateCertificateSHA256 = '',
    [ValidateRange(0,2100000000)] [int]$UpdateMinimumSequence = 0,
    [string]$FirebasePublicConfigFile = ''
)
$ErrorActionPreference = 'Stop'
$workspaceRoot = Split-Path -Parent $PSScriptRoot
$runner = Join-Path $PSScriptRoot 'run-logged.ps1'
function ConvertTo-ShellLiteral([string]$Value) { return "'" + $Value.Replace("'", "''") + "'" }
# These are public deployment metadata, never private keys. Full origin/pin
# validation also runs in the shared Gradle trust configuration.
foreach ($pin in @($CurrentPin, $BackupPin)) {
    try { $decoded = [Convert]::FromBase64String($pin) } catch { throw 'Pin must be canonical Base64 SHA-256.' }
    if ($decoded.Length -ne 32 -or [Convert]::ToBase64String($decoded) -cne $pin) { throw 'Pin must be canonical Base64 SHA-256.' }
}
if ($CurrentPin -ceq $BackupPin) { throw 'Current and backup pins must differ.' }
$uri = $null
if (-not [Uri]::TryCreate($ServerOrigin, [UriKind]::Absolute, [ref]$uri) -or $uri.Scheme -ne 'https' -or
    $uri.UserInfo -ne '' -or $uri.Query -ne '' -or $uri.Fragment -ne '' -or $uri.AbsolutePath -ne '/' -or
    $uri.HostNameType -ne [UriHostNameType]::Dns) { throw 'An HTTPS DNS server origin is required.' }
$runRoot = Join-Path $workspaceRoot ('.cache\release-repro-' + [Guid]::NewGuid().ToString('N'))
New-Item -ItemType Directory -Path $runRoot -ErrorAction Stop | Out-Null
$arguments = @(('-PappControlServerOrigin=' + $ServerOrigin), ('-PappControlCurrentPin=' + $CurrentPin), ('-PappControlBackupPin=' + $BackupPin), ('-PappControlVersionCode=' + $VersionCode), ('-PappControlVersionName=' + $VersionName))
$updateConfigured = $UpdatePublicKey -ne '' -or $UpdateCertificateSHA256 -ne ''
if ($updateConfigured) {
    try { $keyBytes = [Convert]::FromBase64String($UpdatePublicKey) } catch { throw 'Update key must be canonical public Ed25519 DER Base64.' }
    if ($keyBytes.Length -ne 44 -or [Convert]::ToBase64String($keyBytes) -cne $UpdatePublicKey -or $UpdateCertificateSHA256 -cnotmatch '^[a-f0-9]{64}$') { throw 'Update public key and lowercase APK certificate digest are required together.' }
    $arguments += @(('-PappControlUpdatePublicKey='+$UpdatePublicKey),('-PappControlUpdateCertificateSHA256='+$UpdateCertificateSHA256),('-PappControlUpdateMinimumSequence='+$UpdateMinimumSequence))
} elseif ($UpdateMinimumSequence -ne 0) { throw 'Update sequence requires update trust configuration.' }
$childVariant = 'release'
$childTask = ':child-app:assembleRelease'
$firebaseConfig = $null
if ($FirebasePublicConfigFile -ne '') {
    $info = Get-Item -LiteralPath $FirebasePublicConfigFile
    if (-not ($info -is [IO.FileInfo]) -or $info.Length -gt 16384) { throw 'Firebase public configuration is invalid.' }
    try { $firebaseConfig = [IO.File]::ReadAllText($info.FullName,[Text.Encoding]::UTF8) | ConvertFrom-Json } catch { throw 'Firebase public configuration is invalid.' }
    $requiredNames = @('applicationId','apiKey','projectId','senderId')
    $names = @($firebaseConfig.PSObject.Properties.Name)
    if ($names.Count -ne 4 -or @($names | Where-Object { $_ -notin $requiredNames }).Count -gt 0) { throw 'Expected only four public Firebase configuration fields.' }
    $patterns = @{applicationId='^1:[0-9]+:android:[a-fA-F0-9]+$';apiKey='^[A-Za-z0-9_-]{20,128}$';projectId='^[a-z0-9][a-z0-9-]{4,62}$';senderId='^[0-9]{1,32}$'}
    foreach ($name in $requiredNames) { if ($firebaseConfig.$name -isnot [string] -or $firebaseConfig.$name -cnotmatch $patterns[$name]) { throw 'Invalid public Firebase configuration field.' } }
    if ($firebaseConfig.applicationId.Split(':')[1] -cne $firebaseConfig.senderId) { throw 'Firebase sender and application IDs differ.' }
    $arguments += @('-PappControlFirebase=true',('-PappControlFirebaseApplicationId='+$firebaseConfig.applicationId),('-PappControlFirebaseApiKey='+$firebaseConfig.apiKey),('-PappControlFirebaseProjectId='+$firebaseConfig.projectId),('-PappControlFirebaseSenderId='+$firebaseConfig.senderId))
    $childVariant = 'fcmRelease';$childTask = ':child-app:assembleFcmRelease'
}
$quoted = ($arguments | ForEach-Object { ConvertTo-ShellLiteral $_ }) -join ' '
$java = ConvertTo-ShellLiteral 'C:\Program Files\Java\jdk-17'
$cache = ConvertTo-ShellLiteral (Join-Path $workspaceRoot '.cache\gradle')
$gradle = ConvertTo-ShellLiteral 'C:\Gradle\gradle-9.6.0\bin\gradle.bat'
$hashes = @{}
for ($iteration = 1; $iteration -le 2; $iteration++) {
    $command = "`$env:JAVA_HOME=$java; `$env:GRADLE_USER_HOME=$cache; & $gradle --offline --no-daemon --no-build-cache --rerun-tasks '-Pkotlin.compiler.execution.strategy=in-process' $quoted :parent-app:clean :child-app:clean :parent-app:assembleRelease $childTask"
    & $runner -TaskId P7-001 -Category build -WorkingDirectory (Join-Path $workspaceRoot 'android') -Command $command
    if ($LASTEXITCODE -ne 0) { throw 'Clean unsigned release build failed; evidence retained.' }
    $snapshot = Join-Path $runRoot ('build-' + $iteration)
    New-Item -ItemType Directory -Path $snapshot -ErrorAction Stop | Out-Null
    foreach ($app in @('parent', 'child')) {
        $variant = if ($app -eq 'child') { $childVariant } else { 'release' }
        $source = Join-Path $workspaceRoot ("android\$app-app\build\outputs\apk\$variant\$app-app-$variant-unsigned.apk")
        $destination = Join-Path $snapshot ($app + '-unsigned.apk')
        if (-not (Test-Path -LiteralPath $source -PathType Leaf)) { throw 'Expected unsigned release APK is missing.' }
        [IO.File]::Copy($source, $destination, $false)
        $hash = (Get-FileHash -LiteralPath $destination -Algorithm SHA256).Hash.ToLowerInvariant()
        if ($iteration -eq 1) { $hashes[$app] = $hash }
        elseif ($hashes[$app] -cne $hash) { throw 'Unsigned APKs differ between clean builds; both snapshots retained.' }
    }
}
$record = [ordered]@{
    verifiedAtUtc = [DateTime]::UtcNow.ToString('o')
    serverOrigin = $ServerOrigin
    currentPin = $CurrentPin
    backupPin = $BackupPin
    versionCode = $VersionCode
    versionName = $VersionName
    childVariant = $childVariant
    updateTrust = @{configured=$updateConfigured;publicKey=$UpdatePublicKey;certificateSHA256=$UpdateCertificateSHA256;minimumSequence=$UpdateMinimumSequence}
    firebasePublicConfiguration = $firebaseConfig
    unsignedApkSHA256 = $hashes
    scope = 'Two clean unsigned release APK builds on this host/toolchain; signing and deployment acceptance remain separate.'
}
$record | ConvertTo-Json -Depth 5 | Set-Content -LiteralPath (Join-Path $runRoot 'reproducibility.json') -Encoding UTF8
Write-Output 'Both parent and child unsigned release APKs match across two clean offline builds.'
Write-Output "Evidence directory: $runRoot"
