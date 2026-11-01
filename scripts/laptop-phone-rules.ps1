# Mapping checks are separate from ADB so refusal and concurrent-rebind behavior can be verified offline.
function Set-LaptopReverseMapping([int]$Port,[scriptblock]$Query) {
    if($Port -lt 1024 -or $Port -gt 65535){throw 'Invalid laptop HTTPS port.'}
    $endpoint='tcp:'+$Port
    $mappings=(& $Query -Arguments @('reverse','--list')) -split '\r?\n'
    $existing=@($mappings | Where-Object {$parts=$_ -split '\s+';$parts.Count -ge 3 -and $parts[-2] -ceq $endpoint})
    if($existing.Count -gt 1){throw 'Ambiguous existing reverse mapping; inspect it manually.'}
    if($existing.Count -eq 1){
        $parts=$existing[0] -split '\s+'
        if($parts[-1] -cne $endpoint){throw 'This device port is already mapped elsewhere; existing mapping preserved.'}
    }else{
        # Refuse a mapping created after the precheck instead of replacing it.
        [void](& $Query -Arguments @('reverse','--no-rebind',$endpoint,$endpoint))
    }
    $after=& $Query -Arguments @('reverse','--list')
    if($after -notmatch ('(?m)^\S+\s+'+[regex]::Escape($endpoint)+'\s+'+[regex]::Escape($endpoint)+'\r?$')){throw 'Expected reverse mapping not observed.'}
}
