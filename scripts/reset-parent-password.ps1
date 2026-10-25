param(
    [Parameter(Mandatory)] [string]$Database,
    [string]$Executable = (Join-Path (Split-Path -Parent $PSScriptRoot) '.cache\parent-recovery.exe')
)
$ErrorActionPreference = 'Stop'
# Invoke through run-logged.ps1. Credentials enter through a secure prompt and
# stdin, never arguments, command text, files or output.
$credential = Get-Credential -Message 'Verified server operator: enter the parent email and NEW password'
if ($null -eq $credential) { throw 'Recovery cancelled.' }
$pointer = [Runtime.InteropServices.Marshal]::SecureStringToBSTR($credential.Password)
$previousOutputEncoding = $OutputEncoding
try {
    $OutputEncoding = [Text.UTF8Encoding]::new($false)
    $privateInput = @{email=$credential.UserName; password=[Runtime.InteropServices.Marshal]::PtrToStringBSTR($pointer)} | ConvertTo-Json -Compress
    $privateInput | & $Executable -db $Database
    if ($LASTEXITCODE -ne 0) { throw 'Parent recovery failed.' }
} finally {
    $OutputEncoding = $previousOutputEncoding
    [Runtime.InteropServices.Marshal]::ZeroFreeBSTR($pointer)
    $privateInput = $null
    $credential = $null
}
