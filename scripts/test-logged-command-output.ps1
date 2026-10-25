$ErrorActionPreference = 'Stop'
$runner = Join-Path $PSScriptRoot 'run-logged.ps1'
function Assert-LoggedResult([string]$Command, [int]$ExpectedExit, [string]$Marker, [string]$ForbiddenOutputLine = '') {
    $ErrorActionPreference = 'Continue'
    $captured = @(& powershell.exe -NoProfile -ExecutionPolicy Bypass -File $runner -TaskId P6-004 -Category test -Command $Command 2>&1)
    $actualExit = $LASTEXITCODE
    if ($actualExit -ne $ExpectedExit -or ($captured -join "`n") -notlike ('*' + $Marker + '*')) {
        throw 'Logged command exit status or preceding output was lost.'
    }
    $logLines = @($captured | ForEach-Object { $_.ToString() } | Where-Object { $_.StartsWith('Build log: ') })
    if ($logLines.Count -ne 1) { throw 'Expected exactly one build log path.' }
    $logPath = $logLines[0].Substring('Build log: '.Length)
    $content = Get-Content -LiteralPath $logPath -Raw -Encoding UTF8 -ErrorAction Stop
    $separator = [regex]::Escape('=' * 96)
    $entries = [regex]::Matches($content, "(?ms)^$separator\r?\n(?<body>.*?)\r?\nentry_sha256: (?<hash>[a-f0-9]{64})\r?\n$separator(?:\r?\n|\z)")
    if ($entries.Count -eq 0) { throw 'No persisted build log entry found.' }
    # Another build may finish while this subprocess is running. Locate our
    # command instead of assuming the most recent entry belongs to this check.
    $matching = @($entries | Where-Object {
        $commandMatch = [regex]::Match($_.Groups['body'].Value, '(?ms)^command:\r?\n(?<command>.*?)\r?\noutput:\r?\n')
        $commandMatch.Success -and $commandMatch.Groups['command'].Value -ceq $Command
    })
    if ($matching.Count -eq 0) { throw 'Persisted command entry was not found.' }
    $entry = $matching[$matching.Count - 1]
    $body = $entry.Groups['body'].Value
    $outputMatch = [regex]::Match($body, '(?ms)^output:\r?\n(?<output>.*?)\r?\nexit_code: (?<exit>-?\d+)\r?$')
    if (-not $outputMatch.Success -or
        [int]$outputMatch.Groups['exit'].Value -ne $ExpectedExit -or
        -not $outputMatch.Groups['output'].Value.Contains($Marker)) {
        throw 'Persisted log lost command output or exit status.'
    }
    if ($ForbiddenOutputLine -ne '' -and [regex]::IsMatch($outputMatch.Groups['output'].Value, '(?m)^' + [regex]::Escape($ForbiddenOutputLine) + '\r?$')) {
        throw 'PowerShell command continued after a fail-fast error.'
    }
    $sha = [Security.Cryptography.SHA256]::Create()
    try {
        $digest = ([BitConverter]::ToString($sha.ComputeHash([Text.Encoding]::UTF8.GetBytes($body)))).Replace('-', '').ToLowerInvariant()
        if ($digest -ne $entry.Groups['hash'].Value) { throw 'Persisted log digest mismatch.' }
    } finally { $sha.Dispose() }
}
Assert-LoggedResult "Write-Output 'logged-success-marker'" 0 'logged-success-marker'
Assert-LoggedResult "Write-Output ('logged-unicode-marker ' + [char]0x2014 + [char]0x0915 + [char]0x4e2d)" 0 'logged-unicode-marker'
Assert-LoggedResult "Write-Output 'logged-before-throw-marker'; throw 'expected-logged-failure'" 1 'logged-before-throw-marker'
Assert-LoggedResult "Write-Output 'logged-before-native-failure-marker'; & cmd.exe /d /c 'exit 17'" 17 'logged-before-native-failure-marker'
Assert-LoggedResult "& powershell.exe -NoProfile -Command '[Console]::Error.WriteLine(''logged-native-stderr-failure-marker''); exit 23'" 23 'logged-native-stderr-failure-marker'
Assert-LoggedResult "& powershell.exe -NoProfile -Command '[Console]::Error.WriteLine(''logged-native-stderr-success-marker''); exit 0'" 0 'logged-native-stderr-success-marker'
Assert-LoggedResult "Write-Error 'logged-powershell-error-marker'; Write-Output 'continued-after-error'" 1 'logged-powershell-error-marker' 'continued-after-error'
Write-Output 'Logged command checks passed: console and hashed saved output for success, throw, native stderr/exit status and fail-fast PowerShell errors.'
exit 0
