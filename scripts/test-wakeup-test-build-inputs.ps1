$ErrorActionPreference='Stop'
$workspace=Split-Path -Parent $PSScriptRoot
$root=Join-Path $workspace ('.cache/wakeup-build-inputs-'+[guid]::NewGuid().ToString('N'))
$null=New-Item -ItemType Directory -Path $root
$builder=Join-Path $PSScriptRoot 'build-wakeup-test-apks.ps1'
function Save([string]$Path,$Value){[IO.File]::WriteAllText($Path,($Value | ConvertTo-Json -Depth 8),[Text.UTF8Encoding]::new($false))}
function Import-Fixture([string]$Name,[bool]$Child){
    $directory=Join-Path $root $Name;$null=New-Item -ItemType Directory -Path $directory
    $parent=@{applicationId='1:12345:android:abcdef';apiKey='fixture-public-api-key-000000';projectId='wakeup-fixture';senderId='12345'}
    Save (Join-Path $directory 'parent-firebase-public.json') $parent
    $files=@('parent-firebase-public.json')
    if($Child){$second=$parent.Clone();$second.applicationId='1:12345:android:fedcba';Save (Join-Path $directory 'child-firebase-public.json') $second;$files+='child-firebase-public.json'}
    $rows=@($files | ForEach-Object {@{name=$_;sha256=(Get-FileHash -LiteralPath (Join-Path $directory $_) -Algorithm SHA256).Hash.ToLowerInvariant()}})
    Save (Join-Path $directory 'firebase-import.json') @{schemaVersion=1;scope='public-firebase-settings-import';structuralValidationPassed=$true;productionReady=$false;files=$rows}
    return $directory
}
$pin=[Convert]::ToBase64String([byte[]](1..32));$backup=[Convert]::ToBase64String([byte[]](33..64))
function Check($Directory,$Origin='https://wakeup-fixture.invalid',$Current=$pin,$Backup=$backup,[bool]$Fixture=$true){
    $inputs=@{FirebaseSettingsDirectory=$Directory;ServerOrigin=$Origin;CurrentPin=$Current;BackupPin=$Backup;ValidateOnly=$true}
    if($Fixture){$inputs.AllowFixture=$true}
    & $builder @inputs | Out-Null
}
function Refuses([scriptblock]$Action){$refused=$false;try{& $Action}catch{$refused=$true};if(-not $refused){throw 'Unsafe wake-up build input was accepted.'}}
$parent=Import-Fixture 'parent' $false
$both=Import-Fixture 'both' $true
Check $parent;Check $both
& $builder -PollingOnly -ServerOrigin https://wakeup-fixture.invalid -CurrentPin $pin -BackupPin $backup -ValidateOnly -AllowFixture | Out-Null
Refuses {& $builder -PollingOnly -FirebaseSettingsDirectory $both -ServerOrigin https://wakeup-fixture.invalid -CurrentPin $pin -BackupPin $backup -ValidateOnly -AllowFixture}
Refuses {& $builder -PollingOnly -ServerOrigin https://localhost:8443 -CurrentPin $pin -BackupPin $backup -ValidateOnly}
Refuses {Check $both 'http://wakeup-fixture.invalid'}
Refuses {Check $both 'https://wakeup-fixture.invalid/path'}
Refuses {Check $both 'https://wakeup-fixture.invalid:443'}
Refuses {Check $both 'https://wakeup-fixture.invalid' $pin $backup $false}
Refuses {Check $both 'https://wakeup-fixture.invalid' $pin $pin}
Refuses {Check $both 'https://wakeup-fixture.invalid' 'invalid-pin' $backup}
$tampered=Import-Fixture 'tampered' $true
[IO.File]::AppendAllText((Join-Path $tampered 'parent-firebase-public.json'),' ')
Refuses {Check $tampered}
$private=Import-Fixture 'private' $false
$file=Join-Path $private 'parent-firebase-public.json'
$value=Get-Content -LiteralPath $file -Raw | ConvertFrom-Json
$value | Add-Member -NotePropertyName private_key -NotePropertyValue 'not-a-real-secret'
Save $file $value
$record=Get-Content (Join-Path $private 'firebase-import.json') -Raw | ConvertFrom-Json
$record.files[0].sha256=(Get-FileHash $file -Algorithm SHA256).Hash.ToLowerInvariant()
Save (Join-Path $private 'firebase-import.json') $record
Refuses {Check $private}
$same=Import-Fixture 'same' $true
$file=Join-Path $same 'child-firebase-public.json'
$value=Get-Content -LiteralPath $file -Raw | ConvertFrom-Json;$value.applicationId='1:12345:android:abcdef';Save $file $value
$record=Get-Content (Join-Path $same 'firebase-import.json') -Raw | ConvertFrom-Json
($record.files | Where-Object name -eq 'child-firebase-public.json').sha256=(Get-FileHash $file -Algorithm SHA256).Hash.ToLowerInvariant()
Save (Join-Path $same 'firebase-import.json') $record
Refuses {Check $same}
$newline=Import-Fixture 'newline' $false
$file=Join-Path $newline 'parent-firebase-public.json'
$value=Get-Content -LiteralPath $file -Raw | ConvertFrom-Json;$value.apiKey+="`n";Save $file $value
$record=Get-Content (Join-Path $newline 'firebase-import.json') -Raw | ConvertFrom-Json
$record.files[0].sha256=(Get-FileHash $file -Algorithm SHA256).Hash.ToLowerInvariant()
Save (Join-Path $newline 'firebase-import.json') $record
Refuses {Check $newline}
Write-Output 'Wake-up build input checks passed: parent/both, HTTPS/pin/fixture boundaries, tamper/private-field refusal and separate app identities. No builds or network calls.'
