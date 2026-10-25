param(
    [string]$Date
)

$ErrorActionPreference = 'Stop'
$workspaceRoot = Split-Path -Parent $PSScriptRoot
$logRoot = Join-Path $workspaceRoot 'logs\build'

$logFiles = if ([string]::IsNullOrWhiteSpace($Date)) {
    @(Get-ChildItem -LiteralPath $logRoot -Filter '*.txt' | Where-Object { $_.Name -notlike '*-history.txt' })
} else {
    $requestedPath = Join-Path $logRoot "$Date.txt"
    if (-not (Test-Path -LiteralPath $requestedPath)) {
        throw "Build log does not exist: $requestedPath"
    }
    @(Get-Item -LiteralPath $requestedPath)
}

$totals = @{}
foreach ($logFile in $logFiles) {
    $content = Get-Content -LiteralPath $logFile.FullName -Raw
    $entries = [regex]::Matches($content, '(?ms)^={96}\r?\n(.*?)^={96}\r?$')

    foreach ($entryMatch in $entries) {
        $entry = $entryMatch.Groups[1].Value
        $taskMatch = [regex]::Match($entry, '(?m)^task_id: (.+)$')
        $tokensMatch = [regex]::Match($entry, '(?m)^logged_text_estimated_tokens: (\d+)$')
        if (-not $taskMatch.Success -or -not $tokensMatch.Success) {
            continue
        }

        $taskId = $taskMatch.Groups[1].Value.Trim()
        $estimatedTokens = [int64]$tokensMatch.Groups[1].Value
        if (-not $totals.ContainsKey($taskId)) {
            $totals[$taskId] = [pscustomobject]@{
                TaskId = $taskId
                Commands = 0
                LoggedTextEstimatedTokens = [int64]0
                ExactModelTokens = 'unavailable'
            }
        }
        $totals[$taskId].Commands++
        $totals[$taskId].LoggedTextEstimatedTokens += $estimatedTokens
    }
}

if ($totals.Count -eq 0) {
    Write-Output 'No version-two task token entries are available yet.'
    exit 0
}

$totals.Values |
    Sort-Object TaskId |
    Format-Table TaskId, Commands, LoggedTextEstimatedTokens, ExactModelTokens -AutoSize

