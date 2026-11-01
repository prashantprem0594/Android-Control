param(
    [Parameter(Mandatory)] [string]$BundleDirectory
)

$ErrorActionPreference = 'Stop'
$bundle = Get-Item -LiteralPath $BundleDirectory -ErrorAction Stop
if (-not $bundle.PSIsContainer) { throw 'Backup task bundle path must be a directory.' }
$ancestor=$bundle
while($null -ne $ancestor){
    if(($ancestor.Attributes -band [IO.FileAttributes]::ReparsePoint) -ne 0){throw 'Backup bundle root/ancestors cannot be reparse points.'}
    $ancestor=$ancestor.Parent
}
$bundleRoot = [IO.Path]::GetFullPath($bundle.FullName)
$manifestPath = Join-Path $bundleRoot 'bundle.json'
$xmlPath = Join-Path $bundleRoot 'backup-task.xml'
$launcherPath = Join-Path $bundleRoot 'backup-launcher.ps1'
foreach ($path in @($manifestPath, $xmlPath, $launcherPath)) {
    if (-not (Test-Path -LiteralPath $path -PathType Leaf)) { throw "Required backup task bundle file is missing: $(Split-Path -Leaf $path)" }
    $item = Get-Item -LiteralPath $path -ErrorAction Stop
    if (($item.Attributes -band [IO.FileAttributes]::ReparsePoint) -ne 0) {
        throw "Backup task bundle files cannot be reparse points: $(Split-Path -Leaf $path)"
    }
}

try {
    $manifest = ConvertFrom-Json -InputObject ([IO.File]::ReadAllText($manifestPath)) -ErrorAction Stop
} catch {
    throw "Backup task bundle manifest is invalid JSON: $($_.Exception.Message)"
}

function Assert-Hash([string]$Path, [string]$Expected) {
    if ($Expected -notmatch '^[0-9a-fA-F]{64}$') { throw "Invalid SHA-256 value for $(Split-Path -Leaf $Path)." }
    $actual = (Get-FileHash -LiteralPath $Path -Algorithm SHA256).Hash
    if (-not [StringComparer]::OrdinalIgnoreCase.Equals($actual, $Expected)) {
        throw "SHA-256 mismatch: $(Split-Path -Leaf $Path)"
    }
}

Assert-Hash $xmlPath ([string]$manifest.taskXMLSHA256)
Assert-Hash $launcherPath ([string]$manifest.launcherSHA256)

if ($manifest.registered -isnot [bool] -or $manifest.registered -ne $false) { throw 'Bundle manifest must confirm the task is not registered.' }
if ([string]$manifest.logonType -cne 'S4U') { throw 'Bundle manifest logon type must be S4U.' }
$timeoutMinutes = 0
if (-not [int]::TryParse([string]$manifest.timeoutMinutes, [Globalization.NumberStyles]::None, [Globalization.CultureInfo]::InvariantCulture, [ref]$timeoutMinutes) -or
    $timeoutMinutes -lt 1 -or $timeoutMinutes -gt 60) {
    throw 'Bundle timeoutMinutes must be an integer from 1 through 60.'
}
$maxBackups = 0
if (-not [int]::TryParse([string]$manifest.maxBackups, [Globalization.NumberStyles]::None, [Globalization.CultureInfo]::InvariantCulture, [ref]$maxBackups) -or
    $maxBackups -lt 1 -or $maxBackups -gt 3650) {
    throw 'Bundle maxBackups must be an integer from 1 through 3650.'
}
$intervalMinutes = 0
if (-not [int]::TryParse([string]$manifest.intervalMinutes, [Globalization.NumberStyles]::None, [Globalization.CultureInfo]::InvariantCulture, [ref]$intervalMinutes) -or
    $intervalMinutes -notin @(15,30,60,120,240,360,720,1440)) {
    throw 'Bundle intervalMinutes is not a supported interval.'
}

$principalSid = [string]$manifest.principalSid
try {
    $sid = [Security.Principal.SecurityIdentifier]::new($principalSid)
} catch {
    throw 'Bundle principalSid is not a valid Windows SID.'
}
if (-not [StringComparer]::OrdinalIgnoreCase.Equals($sid.Value, $principalSid)) {
    throw 'Bundle principalSid must use its canonical SID form.'
}
if ($principalSid -in @('S-1-5-18','S-1-5-19','S-1-5-20')) {
    throw 'S4U bundle cannot use a built-in service-account SID.'
}

$settings = [Xml.XmlReaderSettings]::new()
$settings.DtdProcessing = [Xml.DtdProcessing]::Prohibit
$settings.XmlResolver = $null
$settings.MaxCharactersInDocument = 1MB
$xmlDocument = [Xml.XmlDocument]::new()
$xmlDocument.XmlResolver = $null
$reader = $null
try {
    $reader = [Xml.XmlReader]::Create($xmlPath, $settings)
    $xmlDocument.Load($reader)
} catch {
    throw "Task XML is invalid or contains a prohibited DTD: $($_.Exception.Message)"
} finally {
    if ($null -ne $reader) { $reader.Dispose() }
}

$namespace = [Xml.XmlNamespaceManager]::new($xmlDocument.NameTable)
$namespace.AddNamespace('task', 'http://schemas.microsoft.com/windows/2004/02/mit/task')
$tasks = @($xmlDocument.SelectNodes('/task:Task', $namespace))
if ($tasks.Count -ne 1) { throw 'Task XML must have one Task root in the Task Scheduler namespace.' }
$principalNodes = @($xmlDocument.SelectNodes('/task:Task/task:Principals/task:Principal', $namespace))
if ($principalNodes.Count -ne 1) { throw 'Task XML must define exactly one principal.' }
$principal = $principalNodes[0]
foreach($name in @('UserId','LogonType','RunLevel')){
    if(@($principal.SelectNodes(('task:'+$name),$namespace)).Count -ne 1){throw 'Principal fields must be present exactly once.'}
}
if(@($principal.SelectNodes('*')).Count -ne 3){throw 'Unexpected principal fields in exported task.'}
$xmlSid = [string]$principal.SelectSingleNode('task:UserId', $namespace).InnerText
$xmlLogon = [string]$principal.SelectSingleNode('task:LogonType', $namespace).InnerText
$xmlRunLevel = [string]$principal.SelectSingleNode('task:RunLevel', $namespace).InnerText
$principalId = [string]$principal.GetAttribute('id')
if (-not [StringComparer]::OrdinalIgnoreCase.Equals($xmlSid, $principalSid)) {
    throw 'Manifest and task XML principal SIDs do not match.'
}
if ($xmlLogon -cne 'S4U' -or $xmlLogon -cne [string]$manifest.logonType) {
    throw 'Task XML and manifest must both specify S4U logon.'
}
if ($xmlRunLevel -cne 'LeastPrivilege') { throw 'Task XML must use least privilege.' }
if ([string]$xmlDocument.SelectSingleNode('/task:Task/task:RegistrationInfo/task:Author', $namespace).InnerText -cne $principalSid) {
    throw 'Task XML registration author and principal are inconsistent.'
}

$policyNodes = @($xmlDocument.SelectNodes('/task:Task/task:Settings/task:MultipleInstancesPolicy', $namespace))
if ($policyNodes.Count -ne 1 -or $policyNodes[0].InnerText -cne 'IgnoreNew') {
    throw 'Task XML must prevent overlapping instances with IgnoreNew.'
}
$limitNodes = @($xmlDocument.SelectNodes('/task:Task/task:Settings/task:ExecutionTimeLimit', $namespace))
$expectedLimit = 'PT' + (2 * $timeoutMinutes + 5) + 'M'
if ($limitNodes.Count -ne 1 -or $limitNodes[0].InnerText -cne $expectedLimit) {
    throw "Task XML execution limit must equal $expectedLimit for the manifest timeout."
}

$actions = @($xmlDocument.SelectNodes('/task:Task/task:Actions', $namespace))
if ($actions.Count -ne 1 -or [string]$actions[0].GetAttribute('Context') -cne $principalId) {
    throw 'Task action context must reference its declared principal.'
}
$execNodes = @($actions[0].SelectNodes('task:Exec', $namespace))
if ($execNodes.Count -ne 1 -or @($actions[0].SelectNodes('*')).Count -ne 1) { throw 'Task XML must define exactly one Exec action and no other actions.' }
foreach($name in @('Command','Arguments','WorkingDirectory')){
    if(@($execNodes[0].SelectNodes(('task:'+$name),$namespace)).Count -ne 1){throw 'Executable action fields must be present exactly once.'}
}
if(@($execNodes[0].SelectNodes('*')).Count -ne 3){throw 'Unexpected executable action fields.'}
$commandNode = $execNodes[0].SelectSingleNode('task:Command', $namespace)
$argumentsNode = $execNodes[0].SelectSingleNode('task:Arguments', $namespace)
if ($null -eq $commandNode -or $null -eq $argumentsNode) { throw 'Task XML Exec action is incomplete.' }
$expectedPowerShell = Join-Path $env:SystemRoot 'System32\WindowsPowerShell\v1.0\powershell.exe'
if (-not [StringComparer]::OrdinalIgnoreCase.Equals([IO.Path]::GetFullPath($commandNode.InnerText), [IO.Path]::GetFullPath($expectedPowerShell))) {
    throw 'Task XML must use the expected Windows PowerShell executable.'
}
$argumentMatch = [regex]::Match($argumentsNode.InnerText, '^-NoProfile -NonInteractive -ExecutionPolicy Bypass -File "([^"]+)"$')
if (-not $argumentMatch.Success) { throw 'Task XML launcher arguments do not match the exported safe invocation.' }
$referencedLauncher = [IO.Path]::GetFullPath($argumentMatch.Groups[1].Value)
if (-not [StringComparer]::OrdinalIgnoreCase.Equals($referencedLauncher, [IO.Path]::GetFullPath($launcherPath))) {
    throw 'Task XML does not reference the launcher in this bundle.'
}

# Check the exported daily/repetition template, not arbitrary Scheduler triggers.
$triggers=@($xmlDocument.SelectNodes('/task:Task/task:Triggers',$namespace))
if($triggers.Count -ne 1 -or @($triggers[0].SelectNodes('*')).Count -ne 1){throw 'Exactly one exported daily trigger is required.'}
$calendar=@($triggers[0].SelectNodes('task:CalendarTrigger',$namespace))
if($calendar.Count -ne 1){throw 'Only the exported calendar trigger is allowed.'}
function Require-TriggerValue([string]$Path,[string]$Value){
    $nodes=@($calendar[0].SelectNodes($Path,$namespace))
    if($nodes.Count -ne 1 -or $nodes[0].InnerText -cne $Value){throw 'Manifest interval and XML daily/repetition schedule differ.'}
}
Require-TriggerValue 'task:Enabled' 'true'
Require-TriggerValue 'task:ScheduleByDay/task:DaysInterval' '1'
$repetitions=@($calendar[0].SelectNodes('task:Repetition',$namespace))
if($intervalMinutes -eq 1440){
    if($repetitions.Count -ne 0){throw 'Daily-only task must not have extra repetition.'}
}else{
    if($repetitions.Count -ne 1){throw 'Repeated task must define exactly one repetition.'}
    Require-TriggerValue 'task:Repetition/task:Interval' ('PT'+$intervalMinutes+'M')
    Require-TriggerValue 'task:Repetition/task:Duration' 'PT1439M'
    Require-TriggerValue 'task:Repetition/task:StopAtDurationEnd' 'false'
    if(@($repetitions[0].SelectNodes('*')).Count -ne 3){throw 'Unexpected repetition fields.'}
}
$startNodes=@($calendar[0].SelectNodes('task:StartBoundary',$namespace))
$startBoundary=[datetime]::MinValue
if($startNodes.Count -ne 1 -or -not [datetime]::TryParseExact($startNodes[0].InnerText,'s',[Globalization.CultureInfo]::InvariantCulture,[Globalization.DateTimeStyles]::None,[ref]$startBoundary)){throw 'Invalid exported start boundary.'}
$expectedChildren=if($intervalMinutes -eq 1440){3}else{4}
if(@($calendar[0].SelectNodes('*')).Count -ne $expectedChildren){throw 'Unexpected calendar trigger fields.'}

Write-Output "Verified backup task bundle: principal $principalSid, S4U/LeastPrivilege, IgnoreNew, execution limit $expectedLimit, matching schedule, one action, XML and launcher hashes."
