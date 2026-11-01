# Pure mock observations; no device access, device changes or live services.
$ErrorActionPreference='Stop'
. (Join-Path $PSScriptRoot 'phone-readiness-rules.ps1')
function Require([bool]$Condition){if(-not $Condition){throw 'Phone readiness contract failed.'}}
function Result([string]$Text,[int]$Code=0){return @{exitCode=$Code;output=$Text}}
function Snapshot([string]$Role='parent'){
    $child=$Role -ceq 'child'
    $serial=if($child){'43c0acdd'}else{'I7MVZPB6ZLJRY95X'}
    $package=if($child){'com.appcontrol.child'}else{'com.appcontrol.parent'}
    $minimum=if($child){28}else{33}
    return @{devices=(Result "List of devices attached`n$serial device product:mock`n")
        device=(Result $(if($child){'whyred'}else{'beryl'}));model=(Result $(if($child){'Redmi Note 5 Pro'}else{'24094RAD4I'}))
        api=(Result $(if($child){'28'}else{'34'}));user=(Result '0');packages=(Result ('package:'+$package))
        packageDump=(Result "Package [$package] (abc):`n versionCode=7 minSdk=$minimum targetSdk=37`n versionName=0.1.6-test`nUser 0: installed=true`n runtime permissions:`n android.permission.POST_NOTIFICATIONS: granted=true, flags=[]`nUser 10: installed=true`n android.permission.POST_NOTIFICATIONS: granted=false, flags=[]`n SECRET_SENTINEL never print this`n")
        policy=(Result 'Current Device Policy Manager state:')}
}
$parent=Snapshot
$newChild=Snapshot 'child'
$newChild.devices=Result "List of devices attached`nC6INTOMB8T55X48H device product:tornado_in`n"
$newChild.device=Result 'tornado';$newChild.model=Result '2508CRN2BI';$newChild.api=Result '35'
Require ((Get-DesignatedPhone 'child' 'C6INTOMB8T55X48H').minimumApi -eq 28)
$value=New-PhoneReadinessReport 'child' 'C6INTOMB8T55X48H' $newChild
Require ($value.identityVerified -and $value.localAppInspectionReady -and $value.api -eq 35 -and $value.deviceLabel -ceq 'Xiaomi 2508CRN2BI')
Require ($value.package.name -ceq 'com.appcontrol.child' -and $value.package.minimumSdk -eq 28 -and $value.blockers -contains 'child-device-owner-not-verified' -and -not $value.acceptanceComplete)
$newChild.packages=Result ''
$value=New-PhoneReadinessReport 'child' 'C6INTOMB8T55X48H' $newChild
Require ($value.identityVerified -and $value.package.installed -eq $false -and -not $value.localAppInspectionReady -and $value.blockers -contains 'target-package-not-observed')
Require (-not (New-PhoneReadinessReport 'parent' 'C6INTOMB8T55X48H' $newChild).identityVerified)
$newChild.device=Result 'whyred'
Require (-not (New-PhoneReadinessReport 'child' 'C6INTOMB8T55X48H' $newChild).identityVerified)
$newChild.device=Result 'tornado'
$newChild.devices=Result "List of devices attached`n43c0acdd device product:tornado_in`n"
Require (-not (New-PhoneReadinessReport 'child' '43c0acdd' $newChild).identityVerified)
$report=New-PhoneReadinessReport 'parent' 'I7MVZPB6ZLJRY95X' $parent
Require ($report.identityVerified -and $report.authorized -and $report.localAppInspectionReady -and -not $report.acceptanceComplete)
Require ($report.package.minimumSdk -eq 33 -and $report.package.targetSdk -eq 37 -and $report.package.versionCode -eq 7 -and $report.package.notifications -ceq 'granted')
Require (-not $report.package.firebaseMessagingServiceDeclared -and $report.blockers -contains 'parent-firebase-service-not-observed')
Require (($report | ConvertTo-Json -Depth 8) -notmatch 'SECRET_SENTINEL')
foreach($component in @('com.appcontrol.parent/.push.ParentFirebaseMessagingService','com.appcontrol.parent/com.appcontrol.parent.push.ParentFirebaseMessagingService')){
    $snapshot=Snapshot;$snapshot.packageDump.output+="`n 1234 $component filter 111`n"
    $value=New-PhoneReadinessReport 'parent' 'I7MVZPB6ZLJRY95X' $snapshot
    Require ($value.package.firebaseMessagingServiceDeclared -and -not $value.acceptanceComplete)
}
$snapshot=Snapshot;$snapshot.packageDump.output+="`ncom.appcontrol.parent/.push.ParentFirebaseMessagingServiceFake filter 111`n"
Require (-not (New-PhoneReadinessReport 'parent' 'I7MVZPB6ZLJRY95X' $snapshot).package.firebaseMessagingServiceDeclared)
$snapshot=Snapshot;$snapshot.user=Result '10'
$value=New-PhoneReadinessReport 'parent' 'I7MVZPB6ZLJRY95X' $snapshot
Require ($value.package.notifications -ceq 'denied' -and -not $value.localAppInspectionReady -and $value.blockers -contains 'active-user-is-not-designated-owner-user')
$snapshot=Snapshot;$snapshot.packageDump.output=$snapshot.packageDump.output.Replace('android.permission.POST_NOTIFICATIONS: granted=true, flags=[]','')
Require ((New-PhoneReadinessReport 'parent' 'I7MVZPB6ZLJRY95X' $snapshot).package.notifications -ceq 'unknown')
$snapshot=Snapshot;$snapshot.packages=Result ''
$value=New-PhoneReadinessReport 'parent' 'I7MVZPB6ZLJRY95X' $snapshot
Require ($value.package.installed -eq $false -and -not $value.localAppInspectionReady)
$snapshot=Snapshot;$snapshot.packages=Result 'permission denied SECRET_SENTINEL' 1
Require ($null -eq (New-PhoneReadinessReport 'parent' 'I7MVZPB6ZLJRY95X' $snapshot).package.installed)
$snapshot=Snapshot;$snapshot.packages=Result "package:com.appcontrol.parent`npackage:com.appcontrol.parent.test"
Require ((New-PhoneReadinessReport 'parent' 'I7MVZPB6ZLJRY95X' $snapshot).package.installed -eq $true)
$snapshot=Snapshot;$snapshot.packages=Result 'package:com.appcontrol.parent.test'
Require ((New-PhoneReadinessReport 'parent' 'I7MVZPB6ZLJRY95X' $snapshot).package.installed -eq $false)
$snapshot=Snapshot;$snapshot.packages=Result "package:com.appcontrol.parent`npackage:com.appcontrol.parent"
Require ($null -eq (New-PhoneReadinessReport 'parent' 'I7MVZPB6ZLJRY95X' $snapshot).package.installed)
$snapshot=Snapshot;$snapshot.packages=Result "package:com.appcontrol.parent`nUNKNOWN SECRET_SENTINEL"
Require ($null -eq (New-PhoneReadinessReport 'parent' 'I7MVZPB6ZLJRY95X' $snapshot).package.installed)
$snapshot=Snapshot;$snapshot.packageDump=Result 'SECRET_SENTINEL' 1
Require (-not (New-PhoneReadinessReport 'parent' 'I7MVZPB6ZLJRY95X' $snapshot).localAppInspectionReady)
$snapshot=Snapshot;$snapshot.packageDump.output=$snapshot.packageDump.output.Replace('minSdk=33','minSdk=40')
Require (-not (New-PhoneReadinessReport 'parent' 'I7MVZPB6ZLJRY95X' $snapshot).package.runtimeCompatible)
$snapshot=Snapshot;$snapshot.packageDump.output+="`n versionCode=8 minSdk=33 targetSdk=37`n"
Require ($null -eq (New-PhoneReadinessReport 'parent' 'I7MVZPB6ZLJRY95X' $snapshot).package.versionCode)
$snapshot=Snapshot;$snapshot.api=Result '32'
Require (-not (New-PhoneReadinessReport 'parent' 'I7MVZPB6ZLJRY95X' $snapshot).localAppInspectionReady)
foreach($state in @('offline','unauthorized')){
    $snapshot=Snapshot;$snapshot.devices=Result "List of devices attached`nI7MVZPB6ZLJRY95X $state`n"
    $value=New-PhoneReadinessReport 'parent' 'I7MVZPB6ZLJRY95X' $snapshot
    Require ($value.connection -ceq $state -and -not $value.authorized -and $null -eq $value.package)
}
$snapshot=Snapshot;$snapshot.devices=Result "List of devices attached`n43c0acdd device`n"
Require ((New-PhoneReadinessReport 'parent' 'I7MVZPB6ZLJRY95X' $snapshot).connection -ceq 'absent')
Require ((Get-PhoneConnectionState 0 "List of devices attached`nI7MVZPB6ZLJRY95X device`nI7MVZPB6ZLJRY95X device`n" 'I7MVZPB6ZLJRY95X') -ceq 'unexpected')
Require ((Get-PhoneConnectionState 0 "List of devices attached`nI7MVZPB6ZLJRY95X unexpected`n" 'I7MVZPB6ZLJRY95X') -ceq 'unexpected')
Require ((Get-PhoneConnectionState 1 'SECRET_SENTINEL' 'I7MVZPB6ZLJRY95X') -ceq 'query-failed')
Require (-not (New-PhoneReadinessReport 'child' 'I7MVZPB6ZLJRY95X' (Snapshot)).identityVerified)
Require (-not (New-PhoneReadinessReport 'parent' '43c0acdd' (Snapshot 'child')).identityVerified)
Require (-not (New-PhoneReadinessReport 'parent' 'unknown' (Snapshot)).identityVerified)
$snapshot=Snapshot;$snapshot.device=Result 'whyred'
Require (-not (New-PhoneReadinessReport 'parent' 'I7MVZPB6ZLJRY95X' $snapshot).identityVerified)
$child=Snapshot 'child'
$value=New-PhoneReadinessReport 'child' '43c0acdd' $child
Require ($value.localAppInspectionReady -and $value.package.notifications -ceq 'not-required' -and $value.blockers -contains 'child-device-owner-not-verified')
$child.policy=Result "Current Device Policy Manager state:`n Device Owner:`n  admin=ComponentInfo{com.appcontrol.child/com.appcontrol.child.admin.AppControlDeviceAdminReceiver}`n  name=SECRET_SENTINEL"
$value=New-PhoneReadinessReport 'child' '43c0acdd' $child
Require ($value.deviceOwner -ceq 'target-app' -and -not $value.acceptanceComplete)
Require (($value | ConvertTo-Json -Depth 8) -notmatch 'SECRET_SENTINEL')
Require ((Get-PhoneOwnerObservation 0 'Current Device Policy Manager state: Device Owner: ComponentInfo{another.app/.Admin}' 'com.appcontrol.child') -ceq 'not-observed')
Require ((Get-PhoneOwnerObservation 0 "Current Device Policy Manager state:`nDevice Owner: ComponentInfo{another.app/.Admin}" 'com.appcontrol.child') -ceq 'other-app')
Require ((Get-PhoneOwnerObservation 0 "Current Device Policy Manager state:`nDevice Owner: UNKNOWN SECRET_SENTINEL" 'com.appcontrol.child') -ceq 'unknown')
Require ((Get-PhoneOwnerObservation 1 'SECRET_SENTINEL' 'com.appcontrol.child') -ceq 'unknown')
foreach($secondOwner in @('ComponentInfo{another.app/.Admin}','ComponentInfo{com.appcontrol.child/.Admin}','UNKNOWN SECRET_SENTINEL','null')){
    $ambiguous="Current Device Policy Manager state:`nDevice Owner: ComponentInfo{com.appcontrol.child/.Admin}`nDevice Owner: $secondOwner"
    Require ((Get-PhoneOwnerObservation 0 $ambiguous 'com.appcontrol.child') -ceq 'unknown')
    $child.policy=Result $ambiguous
    $value=New-PhoneReadinessReport 'child' '43c0acdd' $child
    Require ($value.deviceOwner -ceq 'unknown' -and $value.blockers -contains 'child-device-owner-not-verified')
    Require (($value | ConvertTo-Json -Depth 8) -notmatch 'SECRET_SENTINEL')
}
$invalid=New-PhoneReadinessReport 'SECRET_SENTINEL' 'SECRET_SENTINEL' @{}
Require (($invalid | ConvertTo-Json -Depth 8) -notmatch 'SECRET_SENTINEL')
Write-Output 'Phone readiness mock checks passed: designated role/device gates, offline/unauthorized/absent/query errors, package/version/SDK parsing, per-user notification grant, exact Firebase component observation and sanitized owner/readiness boundaries.'
