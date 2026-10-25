param(
    [ValidatePattern('^P[0-7]$')]
    [string]$Phase,
    [switch]$RequireComplete,
    [switch]$Json
)

$ErrorActionPreference = 'Stop'
$workspaceRoot = Split-Path -Parent $PSScriptRoot
$trackerPath = Join-Path $workspaceRoot 'tracking\phase-status.json'
$tracker = Get-Content -LiteralPath $trackerPath -Raw | ConvertFrom-Json
$allowedStatuses = @('not_started', 'in_progress', 'passed', 'failed', 'blocked')
$errors = [System.Collections.Generic.List[string]]::new()
$knownIds = @{}

if ($tracker.schemaVersion -ne 1) {
    $errors.Add("Unsupported tracker schema version: $($tracker.schemaVersion)")
}

$selectedPhases = @($tracker.phases)
if ($Phase) {
    $selectedPhases = @($selectedPhases | Where-Object { $_.id -eq $Phase })
    if ($selectedPhases.Count -eq 0) {
        $errors.Add("Phase not found: $Phase")
    }
}

$phaseResults = foreach ($phaseItem in $selectedPhases) {
    $checks = @($phaseItem.checks)
    foreach ($check in $checks) {
        if ($knownIds.ContainsKey($check.id)) {
            $errors.Add("Duplicate check ID: $($check.id)")
        }
        $knownIds[$check.id] = $true

        if ($allowedStatuses -notcontains $check.status) {
            $errors.Add("Invalid status '$($check.status)' for $($check.id)")
        }
        if ($check.status -eq 'passed' -and @($check.evidence).Count -eq 0) {
            $errors.Add("Passed check $($check.id) has no evidence")
        }
        if ([string]::IsNullOrWhiteSpace($check.verification)) {
            $errors.Add("Check $($check.id) has no verification method")
        }
    }

    $required = @($checks | Where-Object { $_.required -eq $true })
    $passed = @($required | Where-Object { $_.status -eq 'passed' }).Count
    $failed = @($required | Where-Object { $_.status -eq 'failed' }).Count
    $blocked = @($required | Where-Object { $_.status -eq 'blocked' }).Count
    $started = @($required | Where-Object { $_.status -ne 'not_started' }).Count

    $computedState = if ($required.Count -gt 0 -and $passed -eq $required.Count) {
        'passed'
    } elseif ($failed -gt 0) {
        'failed'
    } elseif ($blocked -gt 0) {
        'blocked'
    } elseif ($started -gt 0) {
        'in_progress'
    } else {
        'not_started'
    }

    [pscustomobject]@{
        Phase = $phaseItem.id
        Name = $phaseItem.name
        State = $computedState
        Passed = $passed
        Required = $required.Count
        Percent = if ($required.Count -eq 0) { 100 } else { [math]::Round(($passed / $required.Count) * 100) }
        Blocked = $blocked
        Failed = $failed
    }
}

if ($errors.Count -gt 0) {
    Write-Error ($errors -join [Environment]::NewLine)
    exit 2
}

if ($Json) {
    $phaseResults | ConvertTo-Json -Depth 4
} else {
    $phaseResults | Format-Table Phase, Name, State, Passed, Required, Percent, Blocked, Failed -AutoSize

    foreach ($phaseItem in $selectedPhases) {
        $openChecks = @($phaseItem.checks | Where-Object { $_.required -eq $true -and $_.status -ne 'passed' })
        if ($openChecks.Count -gt 0) {
            Write-Output ""
            Write-Output "$($phaseItem.id) open checks:"
            $openChecks | Select-Object id, status, feature | Format-Table -AutoSize
        }
    }
}

if ($RequireComplete -and @($phaseResults | Where-Object { $_.State -ne 'passed' }).Count -gt 0) {
    exit 1
}

exit 0

