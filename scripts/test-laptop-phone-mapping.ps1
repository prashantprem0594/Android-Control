# Mock ADB only. No phones, forwarding, installs, roles or services are changed.
$ErrorActionPreference='Stop'
. (Join-Path $PSScriptRoot 'laptop-phone-rules.ps1')
function Require([bool]$Condition){if(-not $Condition){throw 'Laptop reverse mapping contract failed.'}}
function Run-Scenario([string]$Before,[string]$After,[bool]$Race=$false,[bool]$Refused=$false,[int]$Creates=0) {
    $scenario=@{lists=0;creates=0;before=$Before;after=$After;race=$Race}
    $query={param([string[]]$Arguments)
        if(($Arguments -join ' ') -ceq 'reverse --list'){
            $scenario.lists++
            if($scenario.lists -eq 1){return $scenario.before}else{return $scenario.after}
        }
        if(($Arguments -join ' ') -cne 'reverse --no-rebind tcp:8443 tcp:8443'){throw 'Unexpected mock ADB command.'}
        $scenario.creates++
        if($scenario.race){throw 'Mock ADB refuses concurrent port binding.'}
        return ''
    }.GetNewClosure()
    $failed=$false
    try{Set-LaptopReverseMapping 8443 $query}catch{$failed=$true}
    Require ($failed -eq $Refused -and $scenario.creates -eq $Creates)
}
Run-Scenario '' 'UsbFfs tcp:8443 tcp:8443' -Creates 1
Run-Scenario 'UsbFfs tcp:8443 tcp:8443' 'UsbFfs tcp:8443 tcp:8443'
Run-Scenario 'UsbFfs tcp:8443 tcp:9000' '' -Refused $true
Run-Scenario "UsbFfs tcp:8443 tcp:8443`nUsbFfs tcp:8443 tcp:9000" '' -Refused $true
Run-Scenario 'UsbFfs tcp:9000 tcp:9000' "UsbFfs tcp:9000 tcp:9000`nUsbFfs tcp:8443 tcp:8443" -Creates 1
Run-Scenario '' '' -Race $true -Refused $true -Creates 1
Run-Scenario '' 'UsbFfs tcp:8443 tcp:9000' -Refused $true -Creates 1
Write-Output 'Laptop reverse mapping checks passed: creation, idempotency, conflict/ambiguity refusal, unrelated mapping preservation, concurrent binding refusal and postcondition verification.'
