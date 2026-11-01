$ErrorActionPreference = 'Stop'
$workspace = Split-Path -Parent $PSScriptRoot
$fixtureRoot = Join-Path $workspace ('.cache\backup-task-verification-' + [Guid]::NewGuid().ToString('N'))
$verifier = Join-Path $PSScriptRoot 'verify-backup-task-bundle.ps1'
$utf8 = [Text.UTF8Encoding]::new($false)
$namespace = 'http://schemas.microsoft.com/windows/2004/02/mit/task'
$fixtureSid = 'S-1-5-21-123456789-123456789-123456789-1001'
$junctions=[Collections.Generic.List[string]]::new()

function Get-Hash([string]$Path) {
    return (Get-FileHash -LiteralPath $Path -Algorithm SHA256).Hash.ToLowerInvariant()
}

function Write-Fixture([string]$Directory) {
    $null = New-Item -ItemType Directory -Path $Directory -Force
    $launcher = Join-Path $Directory 'backup-launcher.ps1'
    $xml = Join-Path $Directory 'backup-task.xml'
    [IO.File]::WriteAllText($launcher, '# harmless fixture; must not execute', $utf8)
    $powerShell = Join-Path $env:SystemRoot 'System32\WindowsPowerShell\v1.0\powershell.exe'
    $xmlText = @"
<?xml version="1.0" encoding="utf-8"?>
<Task xmlns="$namespace" version="1.4">
  <RegistrationInfo><Author>$fixtureSid</Author></RegistrationInfo>
  <Triggers><CalendarTrigger>
    <Repetition><Interval>PT15M</Interval><Duration>PT1439M</Duration><StopAtDurationEnd>false</StopAtDurationEnd></Repetition>
    <StartBoundary>2026-10-25T10:00:00</StartBoundary><Enabled>true</Enabled>
    <ScheduleByDay><DaysInterval>1</DaysInterval></ScheduleByDay>
  </CalendarTrigger></Triggers>
  <Principals><Principal id="BackupOperator">
    <UserId>$fixtureSid</UserId><LogonType>S4U</LogonType><RunLevel>LeastPrivilege</RunLevel>
  </Principal></Principals>
  <Settings>
    <MultipleInstancesPolicy>IgnoreNew</MultipleInstancesPolicy>
    <ExecutionTimeLimit>PT9M</ExecutionTimeLimit>
  </Settings>
  <Actions Context="BackupOperator"><Exec>
    <Command>$([Security.SecurityElement]::Escape($powerShell))</Command>
    <Arguments>-NoProfile -NonInteractive -ExecutionPolicy Bypass -File "$launcher"</Arguments>
    <WorkingDirectory>C:\fixture</WorkingDirectory>
  </Exec></Actions>
</Task>
"@
    [IO.File]::WriteAllText($xml, $xmlText, $utf8)
    $manifest = [ordered]@{
        generatedAtUtc = [DateTime]::UtcNow.ToString('o')
        taskName = 'FixtureBackup'
        principalSid = $fixtureSid
        logonType = 'S4U'
        registered = $false
        intervalMinutes = 15
        maxBackups = 30
        timeoutMinutes = 2
        taskXMLSHA256 = Get-Hash $xml
        launcherSHA256 = Get-Hash $launcher
    } | ConvertTo-Json -Depth 4
    [IO.File]::WriteAllText((Join-Path $Directory 'bundle.json'), $manifest, $utf8)
}

function Read-Manifest([string]$Directory) {
    return (ConvertFrom-Json -InputObject ([IO.File]::ReadAllText((Join-Path $Directory 'bundle.json'))))
}

function Write-Manifest([string]$Directory, $Manifest) {
    [IO.File]::WriteAllText((Join-Path $Directory 'bundle.json'), ($Manifest | ConvertTo-Json -Depth 4), $utf8)
}

function Set-XmlFixture([string]$Directory, [string]$XmlText, [switch]$UpdateHash) {
    $xml = Join-Path $Directory 'backup-task.xml'
    [IO.File]::WriteAllText($xml, $XmlText, $utf8)
    if ($UpdateHash) {
        $manifest = Read-Manifest $Directory
        $manifest.taskXMLSHA256 = Get-Hash $xml
        Write-Manifest $Directory $manifest
    }
}

function Invoke-ExpectedFailure([string]$Directory, [string]$Case) {
    $failed = $false
    try { & $verifier -BundleDirectory $Directory | Out-Null } catch { $failed = $true }
    if (-not $failed) { throw "Verifier accepted the $Case fixture." }
}

function New-Case([string]$Name) {
    $path = Join-Path $fixtureRoot $Name
    $source=Join-Path $fixtureRoot 'valid'
    Copy-Item -LiteralPath $source -Destination $path -Recurse
    # Every copied fixture must be valid at its new absolute launcher path
    # before applying the particular tamper this case is meant to exercise.
    $xmlText=[IO.File]::ReadAllText((Join-Path $path 'backup-task.xml')).Replace(
        (Join-Path $source 'backup-launcher.ps1'),(Join-Path $path 'backup-launcher.ps1'))
    Set-XmlFixture $path $xmlText -UpdateHash
    & $verifier -BundleDirectory $path | Out-Null
    return $path
}

try {
    $null = New-Item -ItemType Directory -Path $fixtureRoot
    $valid = Join-Path $fixtureRoot 'valid'
    Write-Fixture $valid
    & $verifier -BundleDirectory $valid | Out-Null

    $launcherTamper = New-Case 'launcher-tamper'
    [IO.File]::AppendAllText((Join-Path $launcherTamper 'backup-launcher.ps1'), "`n# changed")
    Invoke-ExpectedFailure $launcherTamper 'tampered launcher'

    $xmlTamper = New-Case 'xml-tamper'
    [IO.File]::AppendAllText((Join-Path $xmlTamper 'backup-task.xml'), "`n<!-- changed -->")
    Invoke-ExpectedFailure $xmlTamper 'tampered XML'

    $wrongPrincipal = New-Case 'wrong-principal'
    $manifest = Read-Manifest $wrongPrincipal
    $xmlPath = Join-Path $wrongPrincipal 'backup-task.xml'
    $xmlText = [IO.File]::ReadAllText($xmlPath).Replace('<UserId>' + $fixtureSid + '</UserId>', '<UserId>S-1-5-21-123456789-123456789-123456789-1002</UserId>')
    Set-XmlFixture $wrongPrincipal $xmlText -UpdateHash
    Invoke-ExpectedFailure $wrongPrincipal 'inconsistent principal'

    foreach ($case in @(
        @{ Name = 'password-logon'; Old = '<LogonType>S4U</LogonType>'; New = '<LogonType>Password</LogonType>' },
        @{ Name = 'high-privilege'; Old = '<RunLevel>LeastPrivilege</RunLevel>'; New = '<RunLevel>HighestAvailable</RunLevel>' },
        @{ Name = 'overlap'; Old = '<MultipleInstancesPolicy>IgnoreNew</MultipleInstancesPolicy>'; New = '<MultipleInstancesPolicy>Parallel</MultipleInstancesPolicy>' },
        @{ Name = 'execution-limit'; Old = '<ExecutionTimeLimit>PT9M</ExecutionTimeLimit>'; New = '<ExecutionTimeLimit>PT10M</ExecutionTimeLimit>' },
        @{ Name = 'extra-action'; Old = '</Actions>'; New = '<ComHandler><ClassId>{00000000-0000-0000-0000-000000000000}</ClassId></ComHandler></Actions>' },
        @{ Name = 'duplicate-command'; Old = '</Command>'; New = '</Command><Command>C:\unexpected.exe</Command>' },
        @{ Name = 'schedule-interval'; Old = '<Interval>PT15M</Interval>'; New = '<Interval>PT30M</Interval>' },
        @{ Name = 'schedule-duration'; Old = '<Duration>PT1439M</Duration>'; New = '<Duration>PT1440M</Duration>' },
        @{ Name = 'extra-trigger'; Old = '</Triggers>'; New = '<LogonTrigger><Enabled>true</Enabled></LogonTrigger></Triggers>' }
    )) {
        $directory = New-Case $case.Name
        $xmlText = [IO.File]::ReadAllText((Join-Path $directory 'backup-task.xml')).Replace($case.Old, $case.New)
        Set-XmlFixture $directory $xmlText -UpdateHash
        Invoke-ExpectedFailure $directory $case.Name
    }

    $manifestTamper = New-Case 'manifest-principal'
    $manifest = Read-Manifest $manifestTamper
    $manifest.principalSid = 'S-1-5-21-123456789-123456789-123456789-1002'
    Write-Manifest $manifestTamper $manifest
    Invoke-ExpectedFailure $manifestTamper 'manifest principal mismatch'

    $wrongBoolean=New-Case 'registered-string'
    $manifest=Read-Manifest $wrongBoolean;$manifest.registered='false';Write-Manifest $wrongBoolean $manifest
    Invoke-ExpectedFailure $wrongBoolean 'nonboolean registration claim'

    $junctionTarget=Join-Path $fixtureRoot 'junction-target'
    $null=New-Item -ItemType Directory -Path $junctionTarget
    $junctionRoot=Join-Path $fixtureRoot 'root-junction'
    $null=New-Item -ItemType Junction -Path $junctionRoot -Target $junctionTarget
    $junctions.Add($junctionRoot)
    Write-Fixture $junctionRoot
    Invoke-ExpectedFailure $junctionRoot 'bundle root junction'

    $dtd = New-Case 'dtd'
    $dtdText = [IO.File]::ReadAllText((Join-Path $dtd 'backup-task.xml')).Replace('<Task xmlns=', '<!DOCTYPE Task [<!ENTITY xxe SYSTEM "file:///nonexistent-fixture-entity">]><Task xmlns=')
    Set-XmlFixture $dtd $dtdText -UpdateHash
    Invoke-ExpectedFailure $dtd 'DTD/external entity'

    $daily=New-Case 'valid-daily'
    $manifest=Read-Manifest $daily;$manifest.intervalMinutes=1440;Write-Manifest $daily $manifest
    $xmlText=[IO.File]::ReadAllText((Join-Path $daily 'backup-task.xml')) -replace '(?s)<Repetition>.*?</Repetition>',''
    Set-XmlFixture $daily $xmlText -UpdateHash
    & $verifier -BundleDirectory $daily | Out-Null
    Write-Output 'Backup task verifier fixtures pass: repeated/daily exports, hashes, principals, policy/limits, exactly one action/field set, matching schedules and validly placed prohibited DTD.'
} finally {
    foreach($junction in $junctions){if(Test-Path -LiteralPath $junction){[IO.Directory]::Delete($junction)}}
    if (Test-Path -LiteralPath $fixtureRoot) {
        $resolved = [IO.Path]::GetFullPath($fixtureRoot)
        $cacheRoot = [IO.Path]::GetFullPath((Join-Path $workspace '.cache')).TrimEnd('\') + '\'
        if (-not $resolved.StartsWith($cacheRoot, [StringComparison]::OrdinalIgnoreCase) -or
            [IO.Path]::GetFileName($resolved) -notlike 'backup-task-verification-*') {
            throw 'Refusing fixture cleanup outside the workspace cache.'
        }
        Remove-Item -LiteralPath $resolved -Recurse -Force
    }
}
