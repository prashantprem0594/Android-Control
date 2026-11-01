# Pure parsers for the read-only phone inspector. Never return raw ADB output.
function Get-DesignatedPhone([string]$Role,[string]$Serial){
    if($Role -ceq 'parent' -and $Serial -ceq 'I7MVZPB6ZLJRY95X'){
        return @{role='parent';serial=$Serial;device='beryl';package='com.appcontrol.parent';minimumApi=33;label='Redmi 14 5G';firebase='ParentFirebaseMessagingService'}
    }
    if($Role -ceq 'child' -and $Serial -ceq '43c0acdd'){
        return @{role='child';serial=$Serial;device='whyred';package='com.appcontrol.child';minimumApi=28;label='Redmi Note 5 Pro';firebase='FirebaseWakeupService'}
    }
    if($Role -ceq 'child' -and $Serial -ceq 'C6INTOMB8T55X48H'){
        return @{role='child';serial=$Serial;device='tornado';package='com.appcontrol.child';minimumApi=28;label='Xiaomi 2508CRN2BI';firebase='FirebaseWakeupService'}
    }
    return $null
}
function Get-PhoneConnectionState([int]$ExitCode,[string]$Output,[string]$Serial){
    if($ExitCode -ne 0 -or $Output.Length -gt 65536 -or $Output -notmatch '(?m)^List of devices attached\r?$'){return 'query-failed'}
    $rows=@($Output -split '\r?\n' | Where-Object {$_ -cmatch ('\A'+[regex]::Escape($Serial)+'\s+')})
    if($rows.Count -eq 0){return 'absent'}
    if($rows.Count -ne 1){return 'unexpected'}
    $match=[regex]::Match($rows[0],('\A'+[regex]::Escape($Serial)+'\s+(device|offline|unauthorized)(?:\s+[^\r\n]*)?\z'))
    if(-not $match.Success){return 'unexpected'}
    return $match.Groups[1].Value
}
function Get-PhoneOwnerObservation([int]$ExitCode,[string]$Output,[string]$Package){
    if($ExitCode -ne 0 -or $Output.Length -gt 524288 -or $Output -notmatch 'Device Policy Manager state:'){return 'unknown'}
    # Ownership is a single global observation. Conflicting/duplicate sections
    # must not let the first recognized component establish Device Owner.
    $sections=[regex]::Matches($Output,'(?m)^\s*Device Owner:')
    if($sections.Count -gt 1){return 'unknown'}
    $owner=[regex]::Match($Output,'(?m)^\s*Device Owner:\s*(?:\r?\n\s*admin=)?ComponentInfo\{([A-Za-z0-9_.]+)/[A-Za-z0-9_.$]+\}')
    if($owner.Success){if($owner.Groups[1].Value -ceq $Package){return 'target-app'}else{return 'other-app'}}
    if($Output -match '(?m)^\s*Device Owner:' -and $Output -notmatch '(?m)^\s*Device Owner:\s*(?:null|none)\s*$'){return 'unknown'}
    return 'not-observed'
}
function Get-PhonePackageObservation([int]$ListExit,[string]$ListOutput,[int]$DumpExit,[string]$Dump,[string]$Package,[string]$Role,[int]$Api,[int]$UserId){
    $result=[ordered]@{name=$Package;installed=$null;versionCode=$null;versionName=$null;minimumSdk=$null;targetSdk=$null;runtimeCompatible=$null;notifications='unknown';firebaseMessagingServiceDeclared=$null}
    if($ListExit -ne 0 -or $ListOutput.Length -gt 65536){return $result}
    $lines=@($ListOutput -split '\r?\n' | Where-Object {-not [string]::IsNullOrWhiteSpace($_)})
    if($lines.Count -eq 0){$result.installed=$false;return $result}
    if(@($lines | Where-Object {$_ -cnotmatch '\Apackage:[A-Za-z0-9_.]+\z'}).Count -ne 0){return $result}
    $exact=@($lines | Where-Object {$_ -ceq ('package:'+$Package)}).Count
    if($exact -eq 0){$result.installed=$false;return $result}
    if($exact -ne 1){return $result}
    $result.installed=$true
    if($DumpExit -ne 0 -or $Dump.Length -gt 524288 -or $Dump -notmatch ('(?m)^\s*Package \['+[regex]::Escape($Package)+'\]')){return $result}
    $version=[regex]::Matches($Dump,'(?m)^\s*versionCode=([0-9]+)\s+minSdk=([0-9]+)\s+targetSdk=([0-9]+)(?:\s|$)')
    if($version.Count -eq 1){
        $number=0L
        if([long]::TryParse($version[0].Groups[1].Value,[ref]$number)){$result.versionCode=$number}
        $minimum=0;$target=0
        if([int]::TryParse($version[0].Groups[2].Value,[ref]$minimum) -and $minimum -ge 1 -and $minimum -le 100){$result.minimumSdk=$minimum}
        if([int]::TryParse($version[0].Groups[3].Value,[ref]$target) -and $target -ge 1 -and $target -le 100){$result.targetSdk=$target}
    }
    $name=[regex]::Matches($Dump,'(?m)^\s*versionName=([A-Za-z0-9][A-Za-z0-9._+\-]{0,63})\r?$')
    if($name.Count -eq 1){$result.versionName=$name[0].Groups[1].Value}
    if($null -ne $result.minimumSdk -and $Api -ge 1){$result.runtimeCompatible=($Api -ge $result.minimumSdk)}
    if($Api -lt 33 -and $Api -ge 1){$result.notifications='not-required'}else{
        $user=[regex]::Match($Dump,('(?ms)^\s*User '+$UserId+':[^\r\n]*\r?\n(?<state>.*?)(?=^\s*User [0-9]+:|\z)'))
        if($user.Success){
            $permission=[regex]::Matches($user.Groups['state'].Value,'(?m)^\s*android\.permission\.POST_NOTIFICATIONS: granted=(true|false)(?:,|\s|$)')
            if($permission.Count -eq 1){$result.notifications=if($permission[0].Groups[1].Value -ceq 'true'){'granted'}else{'denied'}}
        }
    }
    $component=if($Role -ceq 'parent'){'com\.appcontrol\.parent/(?:\.push\.ParentFirebaseMessagingService|com\.appcontrol\.parent\.push\.ParentFirebaseMessagingService)'}else{'com\.appcontrol\.child/(?:\.wakeup\.FirebaseWakeupService|com\.appcontrol\.child\.wakeup\.FirebaseWakeupService)'}
    # The exact service is an installed manifest observation, not a provider/token/delivery test.
    $result.firebaseMessagingServiceDeclared=($Dump -match ($component+'(?:\s|\}|$)'))
    return $result
}
function New-PhoneReadinessReport([string]$Role,[string]$Serial,$Snapshot){
    $definition=Get-DesignatedPhone $Role $Serial
    $report=[ordered]@{schemaVersion=1;scope='read-only-designated-phone-readiness';role=$(if($Role -cin @('parent','child')){$Role}else{'invalid'});deviceLabel=$null;connection='not-queried';authorized=$false;identityVerified=$false;api=$null;model=$null;currentUserId=$null;package=$null;deviceOwner='unknown';localAppInspectionReady=$false;acceptanceComplete=$false;blockers=@();notVerified=@('server authentication/enrollment','live notifications and Firebase delivery','screen-off/Doze reliability','device policy/call/network behavior','production signing and deployment')}
    if($null -eq $definition){$report.blockers+=@('serial-role-mismatch-or-unknown-designated-device');return $report}
    $report.deviceLabel=$definition.label
    $report.connection=Get-PhoneConnectionState $Snapshot.devices.exitCode $Snapshot.devices.output $Serial
    if($report.connection -cne 'device'){$report.blockers+=@('device-not-connected-and-authorized');return $report}
    $report.authorized=$true
    if($Snapshot.device.exitCode -ne 0 -or $Snapshot.device.output.Trim() -cne $definition.device){$report.blockers+=@('unexpected-device-identity');return $report}
    $report.identityVerified=$true
    $api=0;$user=0
    if($Snapshot.api.exitCode -ne 0 -or -not [int]::TryParse($Snapshot.api.output.Trim(),[ref]$api) -or $api -lt 1 -or $api -gt 100){$report.blockers+=@('android-api-unknown');return $report}
    $report.api=$api
    if($Snapshot.model.exitCode -eq 0 -and $Snapshot.model.output.Trim() -cmatch '\A[A-Za-z0-9][A-Za-z0-9 _.-]{0,63}\z'){$report.model=$Snapshot.model.output.Trim()}
    if($Snapshot.user.exitCode -ne 0 -or -not [int]::TryParse($Snapshot.user.output.Trim(),[ref]$user) -or $user -lt 0 -or $user -gt 21474){$report.blockers+=@('active-user-unknown');return $report}
    $report.currentUserId=$user
    if($user -ne 0){$report.blockers+=@('active-user-is-not-designated-owner-user')}
    $report.package=Get-PhonePackageObservation $Snapshot.packages.exitCode $Snapshot.packages.output $Snapshot.packageDump.exitCode $Snapshot.packageDump.output $definition.package $Role $api $user
    $report.deviceOwner=Get-PhoneOwnerObservation $Snapshot.policy.exitCode $Snapshot.policy.output $definition.package
    if($api -lt $definition.minimumApi){$report.blockers+=@('android-version-below-supported-role-minimum')}
    if($report.package.installed -ne $true){$report.blockers+=@('target-package-not-observed')}
    elseif($null -eq $report.package.minimumSdk -or $null -eq $report.package.versionCode){$report.blockers+=@('installed-package-version-or-sdk-unknown')}
    elseif(-not $report.package.runtimeCompatible){$report.blockers+=@('installed-package-minimum-sdk-exceeds-device')}
    if($Role -ceq 'child' -and $report.deviceOwner -cne 'target-app'){$report.blockers+=@('child-device-owner-not-verified')}
    if($Role -ceq 'parent' -and $report.package.installed -eq $true){
        if($report.package.notifications -cne 'granted'){$report.blockers+=@('parent-notification-runtime-permission-not-verified')}
        if($report.package.firebaseMessagingServiceDeclared -ne $true){$report.blockers+=@('parent-firebase-service-not-observed')}
    }
    $report.localAppInspectionReady=($user -eq 0 -and $api -ge $definition.minimumApi -and $report.package.installed -eq $true -and $report.package.runtimeCompatible -eq $true -and $null -ne $report.package.versionCode)
    return $report
}
