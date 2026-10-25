param(
    [Parameter(Mandatory)] [string]$DatabasePath,
    [Parameter(Mandatory)] [string]$BackupDirectory,
    [Parameter(Mandatory)] [string]$KeyFile,
    [Parameter(Mandatory)] [string]$ScratchDirectory,
    [Parameter(Mandatory)] [string]$OutputDirectory,
    [string]$AdminExecutable = '',
    [string]$PrincipalSid = '',
    [ValidatePattern('^[A-Za-z0-9][A-Za-z0-9._ -]{0,63}$')] [string]$TaskName = 'AppControlEncryptedBackup',
    [ValidateSet(15,30,60,120,240,360,720,1440)] [int]$IntervalMinutes = 1440,
    [ValidateRange(1,3650)] [int]$MaxBackups = 30,
    [ValidateRange(1,60)] [int]$TimeoutMinutes = 2
)
# Export only. No task registration, credentials, ACL changes or backup run.
$ErrorActionPreference = 'Stop'
$workspaceRoot = Split-Path -Parent $PSScriptRoot
function Local-Path([string]$Value) {
    if ([string]::IsNullOrWhiteSpace($Value) -or $Value -match '[\x00-\x1f"]') { throw 'Local deployment path is invalid.' }
    $full = [IO.Path]::GetFullPath($Value)
    if ($full.StartsWith('\\')) { throw 'S4U backup tasks require local paths, not network shares.' }
    return $full
}
function Shell-Literal([string]$Value) { return "'"+$Value.Replace("'","''")+"'" }
$databaseFull = Local-Path $DatabasePath
$backupFull = Local-Path $BackupDirectory
$keyFull = Local-Path $KeyFile
$scratchFull = Local-Path $ScratchDirectory
$outputFull = Local-Path $OutputDirectory
function Within([string]$Path,[string]$Directory) {
    return $Path.Equals($Directory,[StringComparison]::OrdinalIgnoreCase) -or $Path.StartsWith($Directory.TrimEnd('\')+'\',[StringComparison]::OrdinalIgnoreCase)
}
if ((Within $keyFull $backupFull) -or (Within $keyFull $scratchFull) -or (Within $databaseFull $backupFull) -or
    (Within $databaseFull $scratchFull) -or (Within $scratchFull $backupFull) -or (Within $backupFull $scratchFull) -or
    $keyFull.Equals($databaseFull,[StringComparison]::OrdinalIgnoreCase)) { throw 'Keep the source database, backup key, encrypted backups and private scratch storage separate.' }
if (Test-Path -LiteralPath $outputFull) { throw 'Backup task bundle directory must not exist.' }
if (-not $AdminExecutable) { $AdminExecutable = Join-Path $workspaceRoot '.cache\control-admin.exe' }
$adminFull = Local-Path $AdminExecutable
$backupScript = Join-Path $PSScriptRoot 'backup-database.ps1'
$runner = Join-Path $PSScriptRoot 'run-logged.ps1'
foreach ($path in @($adminFull,$backupScript,$runner)) { if (-not (Test-Path -LiteralPath $path -PathType Leaf)) { throw 'Reviewed backup tooling is missing.' } }
if (-not $PrincipalSid) { $PrincipalSid = [Security.Principal.WindowsIdentity]::GetCurrent().User.Value }
try { $null = [Security.Principal.SecurityIdentifier]::new($PrincipalSid) } catch { throw 'Task principal SID is invalid.' }
if ($PrincipalSid -in @('S-1-5-18','S-1-5-19','S-1-5-20')) { throw 'Built-in service accounts require a different logon mode; choose an S4U user principal.' }
$scriptHash = (Get-FileHash -LiteralPath $backupScript -Algorithm SHA256).Hash.ToLowerInvariant()
$adminHash = (Get-FileHash -LiteralPath $adminFull -Algorithm SHA256).Hash.ToLowerInvariant()
$qScript = Shell-Literal $backupScript
$qAdmin = Shell-Literal $adminFull
$command = @"
`$ErrorActionPreference = 'Stop'
if ((Get-FileHash -LiteralPath $qScript -Algorithm SHA256).Hash.ToLowerInvariant() -cne '$scriptHash') { throw 'Backup script changed; review and export the task again.' }
if ((Get-FileHash -LiteralPath $qAdmin -Algorithm SHA256).Hash.ToLowerInvariant() -cne '$adminHash') { throw 'Administration binary changed; review and export the task again.' }
& $qScript -DatabasePath $(Shell-Literal $databaseFull) -BackupDirectory $(Shell-Literal $backupFull) -KeyFile $(Shell-Literal $keyFull) -ScratchDirectory $(Shell-Literal $scratchFull) -AdminExecutable $qAdmin -MaxBackups $MaxBackups -TimeoutMinutes $TimeoutMinutes
if (`$LASTEXITCODE -ne 0) { exit `$LASTEXITCODE }
"@
$launcherText = @"
# Generated backup task launcher. Paths are literals; no secret values stored.
`$ErrorActionPreference = 'Stop'
`$job = @'
$command
'@
& $(Shell-Literal $runner) -TaskId P6-005 -Category database -WorkingDirectory $(Shell-Literal $workspaceRoot) -Command `$job
exit `$LASTEXITCODE
"@
New-Item -ItemType Directory -Path $outputFull -ErrorAction Stop | Out-Null
$launcher = Join-Path $outputFull 'backup-launcher.ps1'
[IO.File]::WriteAllText($launcher,$launcherText,[Text.UTF8Encoding]::new($true))
$xmlFile = Join-Path $outputFull 'backup-task.xml'
$settings = [Xml.XmlWriterSettings]::new();$settings.Indent=$true;$settings.Encoding=[Text.UTF8Encoding]::new($false)
$stream = [IO.File]::Open($xmlFile,[IO.FileMode]::CreateNew,[IO.FileAccess]::Write,[IO.FileShare]::None)
$writer = [Xml.XmlWriter]::Create($stream,$settings)
$ns = 'http://schemas.microsoft.com/windows/2004/02/mit/task'
function Element([string]$Name,[string]$Value) { $writer.WriteElementString($Name,$ns,$Value) }
try {
    $writer.WriteStartDocument();$writer.WriteStartElement('Task',$ns);$writer.WriteAttributeString('version','1.4')
    $writer.WriteStartElement('RegistrationInfo',$ns)
    Element 'Date' ([DateTime]::Now.ToString('s'));Element 'Author' $PrincipalSid
    Element 'Description' 'Local encrypted App Control backup followed by independent authentication/integrity verification.'
    Element 'URI' ('\'+$TaskName);$writer.WriteEndElement()
    $writer.WriteStartElement('Triggers',$ns);$writer.WriteStartElement('CalendarTrigger',$ns)
    if ($IntervalMinutes -lt 1440) {
        $writer.WriteStartElement('Repetition',$ns);Element 'Interval' ('PT'+$IntervalMinutes+'M')
        Element 'Duration' 'PT1439M';Element 'StopAtDurationEnd' 'false';$writer.WriteEndElement()
    }
    Element 'StartBoundary' ([DateTime]::Now.AddMinutes(5).ToString('s'));Element 'Enabled' 'true'
    $writer.WriteStartElement('ScheduleByDay',$ns);Element 'DaysInterval' '1';$writer.WriteEndElement()
    $writer.WriteEndElement();$writer.WriteEndElement()
    $writer.WriteStartElement('Principals',$ns);$writer.WriteStartElement('Principal',$ns);$writer.WriteAttributeString('id','BackupOperator')
    Element 'UserId' $PrincipalSid;Element 'LogonType' 'S4U';Element 'RunLevel' 'LeastPrivilege'
    $writer.WriteEndElement();$writer.WriteEndElement()
    $writer.WriteStartElement('Settings',$ns)
    Element 'MultipleInstancesPolicy' 'IgnoreNew';Element 'DisallowStartIfOnBatteries' 'false';Element 'StopIfGoingOnBatteries' 'false'
    Element 'AllowHardTerminate' 'true';Element 'StartWhenAvailable' 'true';Element 'RunOnlyIfNetworkAvailable' 'false'
    Element 'AllowStartOnDemand' 'true';Element 'Enabled' 'true';Element 'Hidden' 'false';Element 'RunOnlyIfIdle' 'false';Element 'WakeToRun' 'false'
    Element 'ExecutionTimeLimit' ('PT'+(2*$TimeoutMinutes+5)+'M');Element 'Priority' '7';$writer.WriteEndElement()
    $writer.WriteStartElement('Actions',$ns);$writer.WriteAttributeString('Context','BackupOperator');$writer.WriteStartElement('Exec',$ns)
    Element 'Command' (Join-Path $env:SystemRoot 'System32\WindowsPowerShell\v1.0\powershell.exe')
    Element 'Arguments' ('-NoProfile -NonInteractive -ExecutionPolicy Bypass -File "'+$launcher+'"')
    Element 'WorkingDirectory' $workspaceRoot
    $writer.WriteEndElement();$writer.WriteEndElement();$writer.WriteEndElement();$writer.WriteEndDocument();$writer.Flush()
} finally { $writer.Dispose();$stream.Dispose() }
$manifest = [ordered]@{
    generatedAtUtc=[DateTime]::UtcNow.ToString('o');taskName=$TaskName;principalSid=$PrincipalSid;logonType='S4U';registered=$false
    intervalMinutes=$IntervalMinutes;maxBackups=$MaxBackups;timeoutMinutes=$TimeoutMinutes
    taskXMLSHA256=(Get-FileHash -LiteralPath $xmlFile -Algorithm SHA256).Hash.ToLowerInvariant()
    launcherSHA256=(Get-FileHash -LiteralPath $launcher -Algorithm SHA256).Hash.ToLowerInvariant()
    backupScriptSHA256=$scriptHash;adminExecutableSHA256=$adminHash
    scope='Reviewable local-file backup task export only. No registration, execution, credentials, key contents, ACL changes or off-host archival.'
}
[IO.File]::WriteAllText((Join-Path $outputFull 'bundle.json'),($manifest|ConvertTo-Json -Depth 6),[Text.UTF8Encoding]::new($false))
Write-Output "Backup task bundle exported: $outputFull"
Write-Output 'Not registered or executed. Review the principal, private local paths, tool hashes and permissions before deployment.'
