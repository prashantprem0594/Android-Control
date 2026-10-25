param(
    [string]$Date = '2026-10-23',
    [switch]$RequireRedactionProbe
)

$ErrorActionPreference = 'Stop'
$workspaceRoot = Split-Path -Parent $PSScriptRoot
$logPath = Join-Path $workspaceRoot "logs\build\$Date.txt"
$exceptionPath = Join-Path $workspaceRoot "logs\build\$Date-integrity-exceptions.json"

if (-not (Test-Path -LiteralPath $logPath)) {
    throw "Build log does not exist: $logPath"
}

$content = Get-Content -LiteralPath $logPath -Raw -Encoding UTF8
$separator = '=' * 96
$escapedSeparator = [regex]::Escape($separator)
$entryPattern = "(?ms)^$escapedSeparator\r?\n(?<body>.*?)\r?\nentry_sha256: (?<hash>[a-f0-9]{64})\r?\n$escapedSeparator(?:\r?\n|\z)"
$entries = [regex]::Matches($content, $entryPattern)
$entryCount = $entries.Count

if ($entryCount -eq 0) {
    throw 'Build log has no hashed entries.'
}

$unparsed = [regex]::Replace($content, $entryPattern, '')
if (-not [string]::IsNullOrWhiteSpace($unparsed)) {
    throw 'Build log contains data outside valid entry boundaries.'
}

$requiredFields = @(
    'started_at_ist:',
    'finished_at_ist:',
    'category:',
    'working_directory:',
    'command:',
    'output:',
    'exit_code:',
    'duration_ms:'
)

$sha256 = [System.Security.Cryptography.SHA256]::Create()
$versionTwoCount = 0
$hashExceptionCount = 0
$hashExceptions = @()
if (Test-Path -LiteralPath $exceptionPath) {
    $exceptionDocument = Get-Content -LiteralPath $exceptionPath -Raw | ConvertFrom-Json
    if ($exceptionDocument.schemaVersion -ne 1 -or $exceptionDocument.logDate -ne $Date) {
        throw "Invalid integrity exception document: $exceptionPath"
    }
    $hashExceptions = @($exceptionDocument.exceptions)
}
$usedExceptionHashes = @()
try {
    for ($index = 0; $index -lt $entries.Count; $index++) {
        $body = $entries[$index].Groups['body'].Value
        $expectedHash = $entries[$index].Groups['hash'].Value
        $bodyBytes = [Text.Encoding]::UTF8.GetBytes($body)
        $actualHash = ([BitConverter]::ToString($sha256.ComputeHash($bodyBytes))).Replace('-', '').ToLowerInvariant()
        if ($actualHash -ne $expectedHash) {
            $matchingExceptions = @($hashExceptions | Where-Object {
                $_.storedHash -eq $expectedHash -and
                $_.computedHash -eq $actualHash -and
                -not [string]::IsNullOrWhiteSpace($_.reason)
            })
            if ($matchingExceptions.Count -ne 1) {
                throw "Build log entry $($index + 1) failed its SHA-256 integrity check."
            }
            $hashExceptionCount++
            $usedExceptionHashes += $expectedHash
        }

        foreach ($field in $requiredFields) {
            if (-not [regex]::IsMatch($body, "(?m)^$([regex]::Escape($field))")) {
                throw "Build log entry $($index + 1) is missing '$field'."
            }
        }

        if ([regex]::IsMatch($body, '(?m)^format_version: 2$')) {
            $versionTwoCount++
        }
    }
} finally {
    $sha256.Dispose()
}

if ($usedExceptionHashes.Count -ne $hashExceptions.Count) {
    throw 'One or more declared integrity exceptions did not match exactly one log entry.'
}

$versionTwoFields = @(
    'task_id:',
    'model_input_tokens:',
    'model_output_tokens:',
    'logged_text_estimated_tokens:',
    'token_estimation_method:'
)

for ($index = 0; $index -lt $entries.Count; $index++) {
    $body = $entries[$index].Groups['body'].Value
    if (-not [regex]::IsMatch($body, '(?m)^format_version: 2$')) {
        continue
    }
    foreach ($field in $versionTwoFields) {
        if (-not [regex]::IsMatch($body, "(?m)^$([regex]::Escape($field))")) {
            throw "Version-two build log entry $($index + 1) is missing '$field'."
        }
    }
}

$redactionProbe = 'demo-' + 'secret-value'
if ($content.Contains($redactionProbe)) {
    throw 'The redaction probe was written to the build log without redaction.'
}
if ($RequireRedactionProbe -and -not $content.Contains('token=[REDACTED]')) {
    throw 'The expected redaction marker is missing from the build log.'
}

Write-Output "build-log-valid entries=$entryCount version_two_entries=$versionTwoCount hash_exceptions=$hashExceptionCount path=$logPath"


