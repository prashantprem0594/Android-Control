# Exercise the runner's generated proof checks without ADB, Gradle or packet traffic.
$ErrorActionPreference='Stop'
$workspace=Split-Path -Parent $PSScriptRoot
$root=Join-Path $workspace ('.cache\network-proof-contract-'+[guid]::NewGuid().ToString('N'))
$null=New-Item -ItemType Directory -Path $root
$tokens=$null;$errors=$null
$ast=[Management.Automation.Language.Parser]::ParseFile((Join-Path $PSScriptRoot 'test-android-internet-pause-network.ps1'),[ref]$tokens,[ref]$errors)
if($errors){throw 'Network runner syntax failed.'}
$function=$ast.Find({param($node) $node -is [Management.Automation.Language.FunctionDefinitionAst] -and $node.Name -ceq 'Invoke-DeviceProbe'},$true)
if($null -eq $function){throw 'Production proof validator was not found.'}
. ([scriptblock]::Create($function.Extent.Text)) # Define only the production command generator.
function Invoke-Logged([string]$TaskId,[string]$Category,[string]$Directory,[string]$Body){$script:capturedCommand=$Body}
$adb='unused-adb';$AdbServerPort=5038;$Serial='127.0.0.1:5581'
$probeVersion='ipv4-host-gateway-v4';$probeScope='ipv4-tcp-dns-wire-fixture-and-management-uid'
$ipv6ProofPath=Join-Path $root 'ipv6-proof.json'
Invoke-DeviceProbe 'run' 'passed' -FullProof
$offset=$capturedCommand.IndexOf('if($n -ne 0')
if($offset -lt 0){throw 'Generated proof checks were not found.'}
$validator=[scriptblock]::Create($capturedCommand.Substring($offset))
$common=@(
    'network_pause_probe=passed',"network_pause_version=$probeVersion",'network_pause_target=blocked_then_restored',
    'network_pause_management=reachable_while_paused','network_pause_cleanup=passed',"network_pause_scope=$probeScope",
    'network_pause_dns_target=blocked_then_restored','network_pause_dns_management=reachable_while_paused'
)
function Check([string[]]$Fields,[bool]$Pass){
    $n=0
    $t=((@($common)+$Fields | ForEach-Object {'INSTRUMENTATION_RESULT: '+$_}) + 'INSTRUMENTATION_CODE: -1') -join "`n"
    $accepted=$true
    try{& $validator}catch{$accepted=$false}
    if($accepted -ne $Pass){throw 'Network proof scope/status invariant failed.'}
}
$passed=@('network_pause_ipv6=passed','network_pause_ipv6_baseline_target=reachable','network_pause_ipv6_baseline_management=reachable','network_pause_ipv6_scope=native-ipv6-fixture-and-management-uid','network_pause_ipv6_target=blocked_then_restored','network_pause_ipv6_management=reachable_while_paused')
Check $passed $true
Check @('network_pause_ipv6=unproven','network_pause_ipv6_baseline_target=timeout','network_pause_ipv6_baseline_management=reachable','network_pause_ipv6_scope=unavailable-baseline') $true
Check @('network_pause_ipv6=unproven','network_pause_ipv6_baseline_target=reachable','network_pause_ipv6_baseline_management=reachable','network_pause_ipv6_scope=unavailable-baseline') $false
Check @($passed | Where-Object {$_ -ne 'network_pause_ipv6_management=reachable_while_paused'}) $false
Check @($passed | ForEach-Object {if($_ -ceq 'network_pause_ipv6_baseline_target=reachable'){'network_pause_ipv6_baseline_target=timeout'}else{$_}}) $false
Check @() $false
Write-Output 'Generated network proof contract passes: independent IPv4/DNS proof, conditional IPv6 baseline/proof, unavailable-baseline and contradictory/missing marker refusal.'
