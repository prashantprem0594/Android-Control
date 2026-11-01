param(
    [ValidateSet('127.0.0.1:5581','emulator-5580')] [string]$Serial='127.0.0.1:5581',
    [ValidateSet(5038)] [int]$AdbServerPort=5038
)
# Kills only the validated debug fixture subprocess through its own application UID.
# Never uses force-stop, clears app data, resets a device, or enqueues replacement work.
$ErrorActionPreference='Stop'
$workspace=Split-Path -Parent $PSScriptRoot
$adb=Join-Path $workspace '.tooling\android-sdk\platform-tools\adb.exe'
$runEvidence=Join-Path $workspace ('.cache\parent-push-death-'+[guid]::NewGuid().ToString('N'))
$body=@'
$ErrorActionPreference='Stop'
function Read-LiveOutput([string]$Path){
    $stream=[IO.File]::Open($Path,[IO.FileMode]::Open,[IO.FileAccess]::Read,([IO.FileShare]::ReadWrite -bor [IO.FileShare]::Delete))
    try{
        if($stream.Length -gt 1048576){throw 'Fixture instrumentation output exceeded its bound.'}
        $reader=[IO.StreamReader]::new($stream)
        try{return $reader.ReadToEnd()}finally{$reader.Dispose()}
    }finally{$stream.Dispose()}
}
function Device([string[]]$Arguments){
    $ErrorActionPreference='Continue'
    $result=@(& $adb -P $port -s $serial @Arguments 2>&1)
    $code=$LASTEXITCODE
    if($code -ne 0){
        $operation=$Arguments -join ' '
        $detail='output suppressed for fixture-state/privacy protection'
        if($operation -notmatch 'files/parent-queue-death|shared_prefs/'){
            $detail=($result -join ' ').Replace([string][char]0,'') -replace '[\x00-\x1f\x7f]',' '
            if($detail.Length -gt 512){$detail=$detail.Substring(0,512)}
        }
        throw "Guarded process-death device command failed (exit $code): $operation; $detail"
    }
    return ($result -join "`n")
}
function Probe([string]$Phase,[string]$Expected){
    $proof=Device @('shell','am','instrument','-w','-r','-e','probe','parent_push_queue_death','-e','phase',$Phase,'com.appcontrol.parent.acceptance.test/com.appcontrol.transport.ServerTrustInstrumentation')
    $proof | Write-Output
    if($proof -notmatch ('(?m)^INSTRUMENTATION_RESULT: parent_push_queue_death_probe='+$Expected+'\r?$') -or $proof -notmatch '(?m)^INSTRUMENTATION_CODE: -1\r?$'){throw 'Process-death phase proof missing or failed.'}
    $script:lastProof=$proof
}
function Read-State{
    $raw=Device @('shell','run-as','com.appcontrol.parent.acceptance','cat','files/parent-queue-death-state.json')
    if($raw.Length -gt 8192){throw 'Process-death fixture record exceeded its bound.'}
    try{$state=$raw | ConvertFrom-Json}catch{throw 'Invalid process-death fixture record.'}
    $names=@('version','generation','firstPid','replayPid','starts','passes','firstExited','replayed','dirtyBeforeFirst','dirtyBeforeReplay','generationMatches','phase','stopCount')
    if($state -isnot [pscustomobject] -or @($state.PSObject.Properties.Name | Where-Object {$_ -cnotin $names}).Count -or $state.version -cne 'parent-queue-death-v1' -or $state.generation -cnotmatch '^[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12}$'){throw 'Unexpected process-death fixture schema.'}
    foreach($key in @('firstPid','replayPid','starts','passes','stopCount')){if(($state.$key -isnot [int] -and $state.$key -isnot [long]) -or $state.$key -lt 0){throw 'Invalid fixture counter.'}}
    foreach($key in @('firstExited','replayed','dirtyBeforeFirst','dirtyBeforeReplay','generationMatches')){if($state.$key -isnot [bool]){throw 'Invalid fixture state.'}}
    return $state
}
function Job-Present{
    $ErrorActionPreference='Continue'
    $output=@(& $adb -P $port -s $serial shell cmd jobscheduler get-job-state -u 0 com.appcontrol.parent.acceptance 5701 2>&1)
    $code=$LASTEXITCODE
    $text=($output -join "`n").Trim()
    # Android's unknown-job shell status is 23 on this API37 image; older images use 0/1.
    # Accept it only with the exact fixture/user/job absence response and no other lines.
    if($code -in @(0,1,23) -and $text -match '^unknown\(u[0-9]+a[0-9]+/jid5701\)' -and @($output | Where-Object {"$_" -notmatch '^unknown\(u[0-9]+a[0-9]+/jid5701\)\s*$|^Could not find job 5701 in package com\.appcontrol\.parent\.acceptance / user 0\s*$'}).Count -eq 0){return $false}
    if($code -ne 0){
        $diagnostic=$text -replace '[\x00-\x1f\x7f]',' '
        if($diagnostic.Length -gt 512){$diagnostic=$diagnostic.Substring(0,512)}
        throw "Framework job-state query failed (exit $code): $diagnostic"
    }
    if($text -notmatch '\b(pending|active|ready|waiting|user-stopped|backing-up|no-component)\b'){throw 'Unrecognized framework job-state response.'}
    return $true
}
$runInvoked=$false
$instrument=$null
$primaryFailure=$null
$passed=$false
$retryMode='automatic'
try{
    $fingerprint=(Device @('shell','getprop','ro.build.fingerprint')).Trim()
    if($fingerprint -notlike 'google/sdk_gphone64_x86_64/emu64xa:*'){throw 'Isolated acceptance emulator required.'}
    $package=Device @('shell','dumpsys','package','com.appcontrol.parent.acceptance')
    if($package -notmatch 'pkgFlags=.*DEBUGGABLE'){throw 'Disposable debug parent acceptance package required.'}
    if($package -notmatch 'android\.permission\.POST_NOTIFICATIONS: granted=true'){throw 'Fixture notification permission must already be granted.'}
    $null=New-Item -ItemType Directory -Path $evidenceRoot
    $stdout=Join-Path $evidenceRoot 'instrumentation-stdout.txt'
    $stderr=Join-Path $evidenceRoot 'instrumentation-stderr.txt'
    $instrument=Start-Process -FilePath $adb -ArgumentList @('-P',"$port",'-s',$serial,'shell','am','instrument','-w','-r','-e','probe','parent_push_queue_death','-e','phase','run','com.appcontrol.parent.acceptance.test/com.appcontrol.transport.ServerTrustInstrumentation') -WindowStyle Hidden -RedirectStandardOutput $stdout -RedirectStandardError $stderr -PassThru
    $runInvoked=$true
    $readyDeadline=[datetime]::UtcNow.AddSeconds(30)
    $readyProof=''
    do{
        Start-Sleep -Milliseconds 100
        if(Test-Path -LiteralPath $stdout){$readyProof=Read-LiveOutput $stdout}
        if($readyProof -match '(?m)^INSTRUMENTATION_STATUS: parent_push_queue_death_probe=prepared\r?$' -and $readyProof -match '(?m)^INSTRUMENTATION_STATUS_CODE: 1\r?$'){break}
        $instrument.Refresh()
        if($instrument.HasExited){throw 'Live fixture instrumentation exited before its prepared status; preserve evidence.'}
    }while([datetime]::UtcNow -lt $readyDeadline)
    if($readyProof -notmatch '(?m)^INSTRUMENTATION_STATUS: parent_push_queue_death_probe=prepared\r?$' -or $readyProof -notmatch '(?m)^INSTRUMENTATION_STATUS_CODE: 1\r?$'){throw 'Live instrumentation prepared status timed out.'}
    if($readyProof -notmatch '(?m)^INSTRUMENTATION_STATUS: parent_push_queue_death_pid=([1-9][0-9]*)\r?$'){throw 'Prepared fixture PID missing.'}
    $fixturePid=[int]$Matches[1]
    if($readyProof -notmatch '(?m)^INSTRUMENTATION_STATUS: parent_push_queue_death_generation=([a-f0-9-]{36})\r?$'){throw 'Prepared fixture generation missing.'}
    $generation=$Matches[1]
    $state=Read-State
    if($state.generation -cne $generation -or $state.firstPid -ne $fixturePid -or $state.replayed -or $state.firstExited -or $state.phase -cne 'holding' -or $state.starts -ne 1 -or $state.passes -ne 1 -or $state.dirtyBeforeFirst){throw 'Prepared fixture generation/counters are inconsistent.'}
    $xmlText=Device @('shell','run-as','com.appcontrol.parent.acceptance','cat','shared_prefs/parent-push-work.xml')
    try{[xml]$prefs=$xmlText}catch{throw 'Invalid durable fixture preferences.'}
    if(@($prefs.map.string | Where-Object {$_.name -ceq 'generation' -and $_.'#text' -ceq $generation}).Count -ne 1 -or @($prefs.map.boolean | Where-Object {$_.name -ceq 'dirty' -and $_.value -ceq 'false'}).Count -ne 1){throw 'Fixture generation/cleared dirty state was not durable before process death.'}
    if(-not (Job-Present)){throw 'Original retained framework job is missing before process death.'}
    # Recheck exact cmdline immediately before kill. Kernel UID enforcement comes from run-as.
    $cmdline=(Device @('shell','run-as','com.appcontrol.parent.acceptance','cat',"/proc/$fixturePid/cmdline")).Replace([string][char]0,'').Trim()
    if($cmdline -cne 'com.appcontrol.parent.acceptance:parent_queue_death'){throw 'PID is not the exact disposable fixture process.'}
    $null=Device @('shell','run-as','com.appcontrol.parent.acceptance','kill','-9',"$fixturePid")
    $deadline=[datetime]::UtcNow.AddSeconds(90)
    $forceAt=[datetime]::UtcNow.AddSeconds(45)
    $forced=$false
    do{
        Start-Sleep -Milliseconds 250
        $state=Read-State
        if($state.generation -cne $generation -or $state.firstPid -ne $fixturePid){throw 'Fixture identity changed across process death.'}
        if($state.replayed){break}
        if(-not $forced -and [datetime]::UtcNow -ge $forceAt){
            if(-not (Job-Present)){throw 'Framework lost the retained job; replacement enqueue is forbidden.'}
            $null=Device @('shell','cmd','jobscheduler','run','-f','-u','0','com.appcontrol.parent.acceptance','5701')
            $forced=$true;$retryMode='shell-forced-retained-item'
        }
    }while([datetime]::UtcNow -lt $deadline)
    if(-not $state.replayed -or $state.firstExited -or $state.replayPid -le 0 -or $state.replayPid -eq $fixturePid -or $state.starts -lt 2 -or $state.passes -ne 2 -or $state.stopCount -ne 0 -or $state.dirtyBeforeFirst -or $state.dirtyBeforeReplay -or -not $state.generationMatches){throw 'Fresh-process retained-work replay was not observed.'}
    Write-Output ("INSTRUMENTATION_RESULT: parent_push_queue_death_first_pid="+$state.firstPid)
    Write-Output ("INSTRUMENTATION_RESULT: parent_push_queue_death_replay_pid="+$state.replayPid)
    Write-Output ("INSTRUMENTATION_RESULT: parent_push_queue_death_starts="+$state.starts)
    Write-Output ("INSTRUMENTATION_RESULT: parent_push_queue_death_passes="+$state.passes)
    Write-Output 'INSTRUMENTATION_RESULT: parent_push_queue_death_stop_count=0'
    Write-Output 'INSTRUMENTATION_RESULT: parent_push_queue_death_first_exited=false'
    Write-Output 'INSTRUMENTATION_RESULT: parent_push_queue_death_dirty_before_first=false'
    Write-Output 'INSTRUMENTATION_RESULT: parent_push_queue_death_dirty_before_replay=false'
    Write-Output 'INSTRUMENTATION_RESULT: parent_push_queue_death_generation_matches=true'
    $removeDeadline=[datetime]::UtcNow.AddSeconds(10)
    while((Job-Present) -and [datetime]::UtcNow -lt $removeDeadline){Start-Sleep -Milliseconds 200}
    if(Job-Present){throw 'Retained job did not drain before verification instrumentation.'}
    Write-Output "INSTRUMENTATION_RESULT: parent_push_queue_death_retry_mode=$retryMode"
    $finishDeadline=[datetime]::UtcNow.AddSeconds(20)
    while(-not $instrument.HasExited -and [datetime]::UtcNow -lt $finishDeadline){Start-Sleep -Milliseconds 100;$instrument.Refresh()}
    if(-not $instrument.HasExited){throw 'Live verification instrumentation did not finish after replay.'}
    $instrument.WaitForExit()
    $finishedProof=Read-LiveOutput $stdout
    $finishedProof | Write-Output
    if($instrument.ExitCode -ne 0 -or $finishedProof -notmatch '(?m)^INSTRUMENTATION_RESULT: parent_push_queue_death_probe=passed\r?$' -or $finishedProof -notmatch '(?m)^INSTRUMENTATION_CODE: -1\r?$'){throw 'Live instrumentation replay verification did not pass; preserve evidence.'}
    $passed=$true
}catch{
    $primaryFailure=$_
    throw
}finally{
    if($null -ne $instrument){
        # Let the bounded instrumentation unwind before starting its separate cleanup phase.
        $endDeadline=[datetime]::UtcNow.AddSeconds(135)
        while(-not $instrument.HasExited -and [datetime]::UtcNow -lt $endDeadline){Start-Sleep -Milliseconds 250;$instrument.Refresh()}
        if(-not $instrument.HasExited){
            if(-not [string]::Equals([IO.Path]::GetFullPath($instrument.Path),[IO.Path]::GetFullPath($adb),[StringComparison]::OrdinalIgnoreCase)){throw 'Refusing to stop an unexpected local helper.'}
            $instrument.Kill()
            $null=$instrument.WaitForExit(10000)
        }
    }
    if($runInvoked){
        try{
            Probe 'cleanup' 'cleaned'
            if($lastProof -notmatch '(?m)^INSTRUMENTATION_RESULT: parent_push_queue_death_cleanup=passed\r?$'){throw 'Process-death fixture cleanup did not pass.'}
        }catch{
            if($null -eq $primaryFailure){throw}
            Write-Output 'Process-death fixture cleanup was not proven after the original failure; preserve fixture evidence and inspect before reuse.'
        }
    }
    if(Test-Path -LiteralPath $evidenceRoot){Write-Output "Process-death runner evidence: $evidenceRoot"}
}
if(-not $passed){throw 'Parent process-death acceptance failed.'}
Write-Output 'Parent retained work survived exact fixture process death with cleared durable dirty state; preference cleanup passed. No Firebase, OEM or physical-device claim.'
'@
$prefix="`$adb='$($adb.Replace("'","''"))';`$serial='$Serial';`$port=$AdbServerPort;`$evidenceRoot='$($runEvidence.Replace("'","''"))'`n"
& (Join-Path $PSScriptRoot 'run-logged.ps1') -TaskId PARENT-PUSH-DEATH -Category device -WorkingDirectory $workspace -Command ($prefix+$body)
exit $LASTEXITCODE
