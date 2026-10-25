param(
    [string]$TaskId = 'UNASSIGNED',
    [Parameter(Mandatory)]
    [string]$Category,
    [Parameter(Mandatory)]
    [string]$Command,
    [Parameter(Mandatory)]
    [string]$WorkingDirectory,
    [Parameter(Mandatory)]
    [AllowEmptyString()]
    [string]$Output,
    [Parameter(Mandatory)]
    [int]$ExitCode,
    [Parameter(Mandatory)]
    [long]$DurationMilliseconds,
    [datetime]$StartedAtUtc = [datetime]::UtcNow,
    [datetime]$FinishedAtUtc = [datetime]::UtcNow
)

$ErrorActionPreference = 'Stop'
$workspaceRoot = Split-Path -Parent $PSScriptRoot
$logRoot = Join-Path $workspaceRoot 'logs\build'
$indiaTimeZone = [TimeZoneInfo]::FindSystemTimeZoneById('India Standard Time')
$startedAt = [TimeZoneInfo]::ConvertTimeFromUtc($StartedAtUtc.ToUniversalTime(), $indiaTimeZone)
$finishedAt = [TimeZoneInfo]::ConvertTimeFromUtc($FinishedAtUtc.ToUniversalTime(), $indiaTimeZone)
$logPath = Join-Path $logRoot ($startedAt.ToString('yyyy-MM-dd') + '.txt')

New-Item -ItemType Directory -Force -Path $logRoot | Out-Null

function Protect-LogText {
    param([AllowEmptyString()] [string]$Text)

    if ($null -eq $Text) {
        return ''
    }

    $protected = $Text
    $protected = [regex]::Replace(
        $protected,
        '(?is)-----BEGIN (?:RSA |EC |OPENSSH )?PRIVATE KEY-----.*?-----END (?:RSA |EC |OPENSSH )?PRIVATE KEY-----',
        '[REDACTED PRIVATE KEY]'
    )
    $protected = [regex]::Replace(
        $protected,
        '(?i)(authorization\s*:\s*bearer\s+)[^\s]+',
        '$1[REDACTED]'
    )
    $protected = [regex]::Replace(
        $protected,
        '(?i)((?:password|passwd|token|secret|api[_-]?key|private[_-]?key)\s*[=:]\s*)[^\s,;]+',
        '$1[REDACTED]'
    )
    $protected = [regex]::Replace(
        $protected,
        '(?i)("(?:password|token|secret|api[_-]?key|private[_-]?key)"\s*:\s*")[^"]*(")',
        '$1[REDACTED]$2'
    )
    return $protected
}

$safeCommand = Protect-LogText -Text $Command
$safeOutput = Protect-LogText -Text $Output
$safeWorkingDirectory = Protect-LogText -Text $WorkingDirectory
$estimatedLoggedTextTokens = [math]::Ceiling(($safeCommand.Length + $safeOutput.Length) / 4.0)

$entryBody = @"
format_version: 2
task_id: $TaskId
started_at_ist: $($startedAt.ToString('yyyy-MM-dd HH:mm:ss.fff zzz'))
finished_at_ist: $($finishedAt.ToString('yyyy-MM-dd HH:mm:ss.fff zzz'))
category: $Category
working_directory: $safeWorkingDirectory
command:
$safeCommand
output:
$safeOutput
exit_code: $ExitCode
duration_ms: $DurationMilliseconds
model_input_tokens: unavailable
model_output_tokens: unavailable
logged_text_estimated_tokens: $estimatedLoggedTextTokens
token_estimation_method: ceil(sanitized command and output characters / 4)
"@

$sha256 = [System.Security.Cryptography.SHA256]::Create()
try {
    $entryBytes = [Text.Encoding]::UTF8.GetBytes($entryBody)
    $entryHash = ([BitConverter]::ToString($sha256.ComputeHash($entryBytes))).Replace('-', '').ToLowerInvariant()
} finally {
    $sha256.Dispose()
}

$separator = '=' * 96
$entry = @"
$separator
$entryBody
entry_sha256: $entryHash
$separator

"@

[System.IO.File]::AppendAllText($logPath, $entry, [Text.UTF8Encoding]::new($false))
Write-Output $logPath

