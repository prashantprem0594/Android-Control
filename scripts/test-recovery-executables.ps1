param(
    [string]$ServerExecutable = '',
    [string]$AdminExecutable = '',
    [switch]$CheckIngress
)

# Run through run-logged.ps1. Uses disposable synthetic accounts and local
# loopback HTTP only; it does not touch a configured production database.
$ErrorActionPreference = 'Stop'
Add-Type -AssemblyName System.Net.Http
$workspaceRoot = Split-Path -Parent $PSScriptRoot
if (-not $ServerExecutable) { $ServerExecutable = Join-Path $workspaceRoot '.cache\control-server.exe' }
if (-not $AdminExecutable) { $AdminExecutable = Join-Path $workspaceRoot '.cache\control-admin.exe' }
$ServerExecutable = (Resolve-Path -LiteralPath $ServerExecutable).Path
$AdminExecutable = (Resolve-Path -LiteralPath $AdminExecutable).Path
$fixture = Join-Path $workspaceRoot ('.cache\recovery-executables-' + [Guid]::NewGuid().ToString('N'))
New-Item -ItemType Directory -Path $fixture | Out-Null
$source = Join-Path $fixture 'source.db'
$destination = Join-Path $fixture 'restored.db'
$key = Join-Path $fixture 'fixture-backup-key.bin'
$backup = Join-Path $fixture 'snapshot.acb'
$scratch = Join-Path $fixture 'scratch'
$activeProcesses = New-Object Collections.Generic.List[Diagnostics.Process]
$httpHandler = New-Object System.Net.Http.HttpClientHandler
$httpHandler.AllowAutoRedirect = $false
$httpHandler.UseProxy = $false
$client = New-Object System.Net.Http.HttpClient($httpHandler)
$client.Timeout = [TimeSpan]::FromSeconds(10)

function Start-FixtureServer([string]$Database, [string]$Label) {
    # Optional integrations must never inherit operator credential paths or
    # production release settings into this disposable loopback rehearsal.
    $optionalSettings = @('APP_CONTROL_FCM_SERVICE_ACCOUNT_FILE', 'APP_CONTROL_FCM_TOKEN_KEY_FILE',
        'APP_CONTROL_RELEASE_MANIFEST_FILE', 'APP_CONTROL_RELEASE_PUBLIC_KEY_FILE',
        'APP_CONTROL_RELEASE_ORIGIN', 'APP_CONTROL_RELEASE_CERTIFICATE_SHA256', 'APP_CONTROL_RELEASE_MIN_SEQUENCE')
    $savedOptional = @{}
    foreach ($setting in $optionalSettings) { $savedOptional[$setting] = [Environment]::GetEnvironmentVariable($setting, 'Process') }
    $savedDatabase = $env:APP_CONTROL_DATABASE_PATH
    $savedAddress = $env:APP_CONTROL_HTTP_ADDRESS
    $savedRate = $env:APP_CONTROL_RATE_SCALE
    $savedProxies = $env:APP_CONTROL_TRUSTED_PROXY_CIDRS
    $savedPasswordSlots = $env:APP_CONTROL_MAX_PASSWORD_IN_FLIGHT
    $savedRequestSlots = $env:APP_CONTROL_MAX_IN_FLIGHT
    $stdout = Join-Path $fixture ($Label + '-stdout.txt')
    $stderr = Join-Path $fixture ($Label + '-stderr.txt')
    try {
        foreach ($setting in $optionalSettings) { [Environment]::SetEnvironmentVariable($setting, $null, 'Process') }
        $env:APP_CONTROL_DATABASE_PATH = $Database
        $env:APP_CONTROL_HTTP_ADDRESS = '127.0.0.1:0'
        $env:APP_CONTROL_RATE_SCALE = '1'
        $env:APP_CONTROL_TRUSTED_PROXY_CIDRS = ''
        $env:APP_CONTROL_MAX_PASSWORD_IN_FLIGHT = '2'
        $env:APP_CONTROL_MAX_IN_FLIGHT = '128'
        $process = Start-Process -FilePath $ServerExecutable -WorkingDirectory $fixture -WindowStyle Hidden `
            -RedirectStandardOutput $stdout -RedirectStandardError $stderr -PassThru
        $activeProcesses.Add($process)
    } finally {
        foreach ($setting in $optionalSettings) { [Environment]::SetEnvironmentVariable($setting, $savedOptional[$setting], 'Process') }
        $env:APP_CONTROL_DATABASE_PATH = $savedDatabase
        $env:APP_CONTROL_HTTP_ADDRESS = $savedAddress
        $env:APP_CONTROL_RATE_SCALE = $savedRate
        $env:APP_CONTROL_TRUSTED_PROXY_CIDRS = $savedProxies
        $env:APP_CONTROL_MAX_PASSWORD_IN_FLIGHT = $savedPasswordSlots
        $env:APP_CONTROL_MAX_IN_FLIGHT = $savedRequestSlots
    }
    $deadline = [DateTime]::UtcNow.AddSeconds(20)
    while ([DateTime]::UtcNow -lt $deadline) {
        if ($process.HasExited) { throw 'Fixture server exited before readiness.' }
        if (Test-Path -LiteralPath $stdout) {
            foreach ($line in @(Get-Content -LiteralPath $stdout)) {
                $record = $null
                try { $record = $line | ConvertFrom-Json } catch { continue }
                if ($record.msg -eq 'control server listening' -and $record.address -match '^127[.]0[.]0[.]1:[0-9]+$') {
                    return [pscustomobject]@{ Process = $process; Origin = 'http://' + $record.address }
                }
            }
        }
        Start-Sleep -Milliseconds 100
    }
    throw 'Fixture server readiness timed out.'
}

function Invoke-FixtureApi([string]$Origin, [string]$Method, [string]$Path, $Body, [string]$Token, [int]$Status, [string]$ForwardedFor = '') {
    $request = New-Object System.Net.Http.HttpRequestMessage([System.Net.Http.HttpMethod]::new($Method), ($Origin + $Path))
    try {
        if ($null -ne $Body) {
            $request.Content = New-Object System.Net.Http.StringContent(($Body | ConvertTo-Json -Compress), [Text.Encoding]::UTF8, 'application/json')
        }
        if ($Token) { $request.Headers.Authorization = New-Object System.Net.Http.Headers.AuthenticationHeaderValue('Bearer', $Token) }
        if ($ForwardedFor) { $request.Headers.Add('X-Forwarded-For', $ForwardedFor) }
        $response = $client.SendAsync($request).GetAwaiter().GetResult()
        try {
            if ([int]$response.StatusCode -ne $Status) { throw 'Fixture API returned an unexpected status.' }
            if ($Status -in @(429, 503) -and ($null -eq $response.Headers.RetryAfter -or $response.Headers.RetryAfter.Delta.TotalSeconds -lt 1)) {
                throw 'Limited response omitted a positive Retry-After.'
            }
            $text = $response.Content.ReadAsStringAsync().GetAwaiter().GetResult()
            if ($text) {
                if ($response.Content.Headers.ContentType.MediaType -eq 'application/json') { return ($text | ConvertFrom-Json) }
                return $text
            }
        } finally { $response.Dispose() }
    } finally { $request.Dispose() }
}

function Invoke-FixtureAdmin([bool]$Success, [string[]]$AdminArguments) {
    $ErrorActionPreference = 'Continue'
    $output = @(& $AdminExecutable @AdminArguments 2>&1)
    if (($LASTEXITCODE -eq 0) -ne $Success) { throw 'Administration binary returned an unexpected result.' }
    # The production CLI prints fixed summaries. Do not relay arbitrary output
    # or fixture paths into the public build log.
}

try {
    $sourceServer = Start-FixtureServer $source 'source'
    $null = Invoke-FixtureApi $sourceServer.Origin GET '/readyz' $null '' 200
    $fixturePassword = [Guid]::NewGuid().ToString('N') + '-fixture'
    $credentials = @{ email = 'rehearsal@example.test'; password = $fixturePassword }
    $registration = Invoke-FixtureApi $sourceServer.Origin POST '/v1/auth/register' $credentials '' 201
    $originalParent = $registration.id
    if (-not $originalParent) { throw 'Fixture registration has no parent identity.' }
    if ($CheckIngress) {
        $uploads = New-Object Collections.Generic.List[object]
        try {
            $sourceUri = [Uri]$sourceServer.Origin
            for ($slot = 0; $slot -lt 2; $slot++) {
                $socket = New-Object Net.Sockets.TcpClient
                $uploads.Add([pscustomobject]@{ Socket = $socket; Stream = $null; Reader = $null })
                $upload = $uploads[$slot]
                $socket.Connect($sourceUri.Host, $sourceUri.Port)
                $upload.Stream = $socket.GetStream()
                $upload.Stream.ReadTimeout = 3000
                $upload.Stream.WriteTimeout = 3000
                $upload.Reader = New-Object IO.StreamReader($upload.Stream, [Text.Encoding]::ASCII)
                $headers = "POST /v1/auth/login HTTP/1.1`r`nHost: localhost`r`nContent-Type: application/json`r`nContent-Length: 128`r`nExpect: 100-continue`r`nConnection: close`r`n`r`n"
                $bytes = [Text.Encoding]::ASCII.GetBytes($headers)
                $upload.Stream.Write($bytes, 0, $bytes.Length)
                # Go sends 100 Continue when the admitted handler reads its body.
                if ($upload.Reader.ReadLine() -notmatch '^HTTP/1[.]1 100 Continue$' -or $upload.Reader.ReadLine() -ne '') {
                    throw 'Stalled upload did not enter an admitted password handler.'
                }
            }
            $busy = Invoke-FixtureApi $sourceServer.Origin POST '/v1/auth/login' @{ email = 'busy@example.test'; password = $fixturePassword } '' 503
            if ($busy.error -ne 'server_busy') { throw 'Password concurrency did not refuse excess work.' }
            $null = Invoke-FixtureApi $sourceServer.Origin GET '/healthz' $null '' 200
            $null = Invoke-FixtureApi $sourceServer.Origin GET '/readyz' $null '' 200
            foreach ($upload in $uploads) {
                $body = [Text.Encoding]::ASCII.GetBytes('{}' + (' ' * 126))
                $upload.Stream.Write($body, 0, $body.Length)
                if ($upload.Reader.ReadLine() -notmatch '^HTTP/1[.]1 401 ') { throw 'Released upload did not finish its handler.' }
                # Drain to EOF (Connection: close) so handler defers have run.
                $null = $upload.Reader.ReadToEnd()
            }
            $null = Invoke-FixtureApi $sourceServer.Origin POST '/v1/auth/login' @{} '' 401
            Write-Output 'Production concurrency passed: two stalled password uploads, excess-work refusal, health isolation and slot release.'
        } finally {
            foreach ($upload in $uploads) {
                if ($upload.Reader) { $upload.Reader.Dispose() }
                $upload.Socket.Dispose()
            }
        }
    }
    Invoke-FixtureAdmin $true @('keygen', '-key-file', $key)
    Invoke-FixtureAdmin $true @('backup', '-db', $source, '-backup', $backup, '-key-file', $key)
    Invoke-FixtureAdmin $true @('verify', '-backup', $backup, '-key-file', $key, '-scratch-dir', $scratch)
    if (@(Get-ChildItem -LiteralPath $scratch -Force).Count -ne 0) { throw 'Verification left scratch files.' }
    $backupHash = (Get-FileHash -LiteralPath $backup -Algorithm SHA256).Hash
    $keyHash = (Get-FileHash -LiteralPath $key -Algorithm SHA256).Hash
    $sourceServer.Process.Kill()
    $sourceServer.Process.WaitForExit()
    # Move only this known fixture file, within its generated fixture directory.
    # Source WAL/SHM files are retained together for inspection; restore must use
    # backup/key and cannot access the configured source database filename.
    Move-Item -LiteralPath $source -Destination (Join-Path $fixture 'source-retired.db')
    Invoke-FixtureAdmin $true @('restore', '-db', $destination, '-backup', $backup, '-key-file', $key)
    Invoke-FixtureAdmin $true @('integrity', '-db', $destination)
    $recoveredServer = Start-FixtureServer $destination 'recovered'
    $null = Invoke-FixtureApi $recoveredServer.Origin GET '/readyz' $null '' 200
    $login = Invoke-FixtureApi $recoveredServer.Origin POST '/v1/auth/login' $credentials '' 200
    if ($login.parent.id -ne $originalParent -or -not $login.tokens.accessToken) { throw 'Recovered parent identity differs.' }
    $me = Invoke-FixtureApi $recoveredServer.Origin GET '/v1/auth/me' $null $login.tokens.accessToken 200
    if ($me.id -ne $originalParent) { throw 'Recovered authenticated identity differs.' }
    $privateRouteMarker = 'unmatched-fixture-' + [Guid]::NewGuid().ToString('N')
    $null = Invoke-FixtureApi $recoveredServer.Origin GET ('/' + $privateRouteMarker + '?token=' + $fixturePassword) $null '' 404
    if ($CheckIngress) {
        # The successful login consumed one of the five peer password tokens.
        # Distinct nonexistent accounts avoid authentication-level lockout.
        for ($attempt = 0; $attempt -lt 4; $attempt++) {
            $invalid = @{ email = ('missing-' + $attempt + '@example.test'); password = $fixturePassword }
            $null = Invoke-FixtureApi $recoveredServer.Origin POST '/v1/auth/login' $invalid '' 401 ('203.0.113.' + ($attempt + 1))
        }
        for ($attempt = 0; $attempt -lt 3; $attempt++) {
            $invalid = @{ email = ('blocked-' + $attempt + '@example.test'); password = $fixturePassword }
            $limited = Invoke-FixtureApi $recoveredServer.Origin POST '/v1/auth/login' $invalid '' 429 ('203.0.113.' + ($attempt + 100))
            if ($limited.error -ne 'rate_limited') { throw 'Ingress rate limiter did not reject forged forwarding identities.' }
        }
        $null = Invoke-FixtureApi $recoveredServer.Origin GET '/healthz' $null '' 200
        $null = Invoke-FixtureApi $recoveredServer.Origin GET '/readyz' $null '' 200
        $null = Invoke-FixtureApi $recoveredServer.Origin GET '/v1/auth/me' $null $login.tokens.accessToken 200
        Write-Output 'Production ingress passed: forged forwarding identities cannot bypass login limits; Retry-After, health and authenticated API isolation verified.'
    }
    Invoke-FixtureAdmin $false @('restore', '-db', $destination, '-backup', $backup, '-key-file', $key)
    if ((Get-FileHash -LiteralPath $backup).Hash -ne $backupHash -or (Get-FileHash -LiteralPath $key).Hash -ne $keyHash) {
        throw 'Rehearsal changed source backup or key.'
    }
    $recoveredServer.Process.Kill()
    $recoveredServer.Process.WaitForExit()
    $capturedLogs = (@(Get-ChildItem -LiteralPath $fixture -File -Filter '*-std*.txt' | ForEach-Object {
        Get-Content -LiteralPath $_.FullName -Raw
    }) -join "`n")
    foreach ($privateValue in @($fixturePassword, $login.tokens.accessToken, $login.tokens.refreshToken, $credentials.email, $privateRouteMarker, '203.0.113.')) {
        if (-not $privateValue -or $capturedLogs.Contains($privateValue)) { throw 'Fixture credentials are missing or present in captured server logs.' }
    }
    Write-Output 'Production binaries passed: live WAL backup, verification, source retirement, restore, integrity, readiness, parent login and destination refusal.'
    Write-Output 'Captured production server logs exclude fixture credentials, issued tokens, unknown paths/query markers and forwarded addresses.'
    Write-Output 'Local loopback rehearsal only; deployed HTTPS, Android and clean-host acceptance remain open.'
} finally {
    foreach ($process in $activeProcesses) {
        if (-not $process.HasExited) { $process.Kill(); $process.WaitForExit() }
        $process.Dispose()
    }
    $client.Dispose()
    $httpHandler.Dispose()
    # Keep non-secret fixture evidence, but never retain this disposable key.
    if (Test-Path -LiteralPath $key) { Remove-Item -LiteralPath $key }
}
exit 0
