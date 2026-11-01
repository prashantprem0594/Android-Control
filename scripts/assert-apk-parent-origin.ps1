param(
    [Parameter(Mandatory)] [string]$Apk,
    [Parameter(Mandatory)] [string]$ExpectedOrigin
)
$ErrorActionPreference='Stop'
$workspace=Split-Path -Parent $PSScriptRoot
$dexdump=Join-Path $workspace '.tooling/android-sdk/build-tools/36.0.0/dexdump.exe'
Add-Type -AssemblyName System.IO.Compression.FileSystem
$zip=[IO.Compression.ZipFile]::OpenRead((Resolve-Path -LiteralPath $Apk).Path)
$found=$false
try {
    foreach($entry in $zip.Entries){
        if($entry.FullName -notmatch '^classes[0-9]*\.dex$'){continue}
        $scratch=Join-Path $workspace ('.cache/origin-inspection-'+[guid]::NewGuid().ToString('N')+'.dex')
        try {
            [IO.Compression.ZipFileExtensions]::ExtractToFile($entry,$scratch,$false)
            $lines=@(& $dexdump -d $scratch 2>&1)
            if($LASTEXITCODE -ne 0){throw 'Could not inspect packaged parent bytecode.'}
            $definition=$lines | Select-String 'Class descriptor.*Lcom/appcontrol/parent/ParentClient;' | Select-Object -First 1
            if($null -eq $definition){continue}
            $start=$definition.LineNumber
            $end=$start
            while($end -lt $lines.Count -and $lines[$end] -notmatch '^Class #'){$end++}
            $block=$lines[$start..($end-1)]
            $method=$block | Select-String "name\s+: 'server'" | Select-Object -First 1
            if($null -eq $method){throw 'Packaged parent server accessor is missing.'}
            $offset=$method.LineNumber-1
            $body=($block[$offset..([Math]::Min($offset+25,$block.Count-1))]) -join "`n"
            if($body -notmatch ('const-string[^\r\n]*"'+[regex]::Escape($ExpectedOrigin)+'"')){
                throw 'Packaged ParentClient server origin differs from the requested configuration; generated BuildConfig source alone is insufficient evidence.'
            }
            $found=$true
            break
        } finally {if(Test-Path -LiteralPath $scratch){Remove-Item -LiteralPath $scratch -Force}}
    }
    if(-not $found){throw 'Packaged ParentClient definition is missing.'}
    Write-Output 'Packaged ParentClient server accessor matches the requested origin.'
} finally {$zip.Dispose()}
