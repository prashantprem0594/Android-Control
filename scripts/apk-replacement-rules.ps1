# Pure acceptance boundary rules, shared by the device inspector and focused tests.
function Resolve-AcceptanceApkVersion([string]$Package,[string]$Version,[string]$InstrumentationTarget,[string]$InstrumentationRunner){
    if($Version -cmatch '^[0-9]+$'){return [long]$Version}
    $targets=@{'com.appcontrol.child.test'='com.appcontrol.child';'com.appcontrol.parent.acceptance.test'='com.appcontrol.parent.acceptance'}
    if($Version -ceq '' -and $targets.ContainsKey($Package) -and $InstrumentationTarget -ceq $targets[$Package] -and $InstrumentationRunner -ceq 'com.appcontrol.transport.ServerTrustInstrumentation'){return [long]0}
    throw 'Missing APK version is allowed only for the exact versionless acceptance instrumentation manifest.'
}
function Get-AcceptanceInstalledBaseApk([int]$ExitCode,[string[]]$Output){
    if([string]::IsNullOrWhiteSpace(($Output -join '')) -and $ExitCode -in @(0,1)){return $null}
    if($ExitCode -ne 0){throw 'Installed package query failed.'}
    $base=@($Output | Where-Object { $_ -match '^package:/data/app/.+/base\.apk\s*$' })
    if($base.Count -ne 1 -or @($Output | Where-Object { $_ -notmatch '^package:/data/app/.+\.apk\s*$' }).Count){throw 'Expected installed base APK paths without diagnostic text.'}
    return $base[0].Trim().Substring(8)
}
function Assert-AcceptanceReplacementIdentity([long]$ReplacementVersion,[long]$InstalledVersion,[bool]$InstalledDebuggable,[string]$ReplacementSigner,[string]$InstalledSigner){
    if(-not $InstalledDebuggable){throw 'Installed APK is not a disposable debug build.'}
    Assert-AppControlReplacementIdentity $ReplacementVersion $InstalledVersion $ReplacementSigner $InstalledSigner
}
function Assert-AppControlReplacementIdentity([long]$ReplacementVersion,[long]$InstalledVersion,[string]$ReplacementSigner,[string]$InstalledSigner){
    if($ReplacementVersion -lt $InstalledVersion){throw 'Replacement would downgrade the installed APK.'}
    if($ReplacementSigner -cnotmatch '^[a-f0-9]{64}$' -or $InstalledSigner -cnotmatch '^[a-f0-9]{64}$' -or $ReplacementSigner -cne $InstalledSigner){throw 'Replacement APK signer differs from the installed identity; no uninstall or reset permitted.'}
}
