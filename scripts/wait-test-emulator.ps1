param([ValidateRange(10, 300)] [int]$TimeoutSeconds = 180)
$ErrorActionPreference = 'Continue'
$workspaceRoot = Split-Path -Parent $PSScriptRoot
$adb = 'C:\Users\Sushant\AppData\Local\Android\Sdk\platform-tools\adb.exe'
$keyPath = Join-Path $workspaceRoot '.tooling\emulator-home\adbkey'
if (-not (Test-Path -LiteralPath $keyPath)) { throw 'Workspace emulator ADB key is missing.' }
$savedVendorKeys = $env:ADB_VENDOR_KEYS
try {
    $env:ADB_VENDOR_KEYS = $keyPath
    $startup = @(& $adb -P 5038 start-server 2>&1)
    if ($LASTEXITCODE -ne 0) { throw 'Isolated ADB daemon failed to start.' }
    $deadline = [DateTime]::UtcNow.AddSeconds($TimeoutSeconds)
    while ([DateTime]::UtcNow -lt $deadline) {
        $connection = @(& $adb -P 5038 connect 127.0.0.1:5581 2>&1)
        $boot = @(& $adb -P 5038 -s 127.0.0.1:5581 shell getprop sys.boot_completed 2>&1)
        if ($LASTEXITCODE -eq 0 -and $boot -contains '1') {
            Write-Output 'Isolated Android test AVD is booted; use ADB server 5038 and serial 127.0.0.1:5581.'
            & $adb -P 5038 -s 127.0.0.1:5581 shell getprop ro.build.fingerprint
            exit $LASTEXITCODE
        }
        Start-Sleep -Seconds 2
    }
    throw 'Isolated Android test AVD boot timed out.'
} finally { $env:ADB_VENDOR_KEYS = $savedVendorKeys }
