param(
    [string]$ParentSerial = 'I7MVZPB6ZLJRY95X',
    [string]$ChildSerial = '43c0acdd',
    [ValidateRange(1,3600)] [int]$IntervalSeconds = 5,
    [switch]$Once
)

# Keeps the localhost HTTPS reverse mapping alive for the designated laptop phones.
# ADB removes reverse mappings when a device transport disconnects, so this process
# restores them after USB reconnects without changing app data, permissions or roles.
$ErrorActionPreference = 'Stop'
$workspace = Split-Path -Parent $PSScriptRoot
$connect = Join-Path $PSScriptRoot 'connect-laptop-phone.ps1'

function Ensure-Mapping([string]$Role, [string]$Serial) {
    try {
        $output = @(& powershell.exe -NoProfile -ExecutionPolicy Bypass -File $connect -Role $Role -Serial $Serial 2>&1)
        if ($LASTEXITCODE -eq 0) {
            Write-Output ("{0}: mapping ready" -f $Role)
            return $true
        }
        Write-Output ("{0}: waiting for authorized phone" -f $Role)
        return $false
    } catch {
        Write-Output ("{0}: waiting for authorized phone" -f $Role)
        return $false
    }
}

do {
    $parentReady = Ensure-Mapping 'parent' $ParentSerial
    $childReady = Ensure-Mapping 'child' $ChildSerial
    if ($Once) {
        if (-not ($parentReady -and $childReady)) { exit 1 }
        exit 0
    }
    if (-not ($parentReady -and $childReady)) { Start-Sleep -Seconds $IntervalSeconds }
} while ($true)
