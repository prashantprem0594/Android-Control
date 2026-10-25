$ErrorActionPreference = 'Stop'
$workspaceRoot = Split-Path -Parent $PSScriptRoot
$sdk = 'C:\Users\Sushant\AppData\Local\Android\Sdk'
$avdHome = Join-Path $workspaceRoot '.tooling\avd'
$avdDirectory = Join-Path $avdHome 'AppControlAcceptance.avd'
$emulatorHome = Join-Path $workspaceRoot '.tooling\emulator-home'
$androidHome = Join-Path $workspaceRoot '.tooling\android-user'
$image = Join-Path $sdk 'system-images\android-37.0\google_apis_playstore\x86_64'
if (-not (Test-Path -LiteralPath (Join-Path $image 'system.img'))) { throw 'Installed Android system image is missing.' }
$portCheck = [Net.Sockets.TcpListener]::new([Net.IPAddress]::Loopback, 5580)
try { $portCheck.Start() } finally { $portCheck.Stop() }
New-Item -ItemType Directory -Force -Path $avdDirectory, $emulatorHome, $androidHome | Out-Null
$encoding = New-Object Text.UTF8Encoding($false)
[IO.File]::WriteAllText((Join-Path $avdHome 'AppControlAcceptance.ini'), "avd.ini.encoding=UTF-8`npath=$avdDirectory`ntarget=android-37`n", $encoding)
$config = @"
AvdId=AppControlAcceptance
avd.ini.displayname=App Control Acceptance (isolated)
abi.type=x86_64
hw.cpu.arch=x86_64
hw.cpu.ncore=2
hw.ramSize=4096
hw.lcd.width=720
hw.lcd.height=1280
hw.lcd.density=320
hw.keyboard=yes
hw.gpu.enabled=yes
hw.gpu.mode=swiftshader
disk.dataPartition.size=2G
image.sysdir.1=$image
tag.id=google_apis_playstore
tag.display=Google Play
PlayStore.enabled=true
fastboot.forceColdBoot=yes
"@
[IO.File]::WriteAllText((Join-Path $avdDirectory 'config.ini'), $config, $encoding)
$savedAvd = $env:ANDROID_AVD_HOME
$savedEmulator = $env:ANDROID_EMULATOR_HOME
$savedAndroid = $env:ANDROID_USER_HOME
try {
    $env:ANDROID_AVD_HOME = $avdHome
    $env:ANDROID_EMULATOR_HOME = $emulatorHome
    $env:ANDROID_USER_HOME = $androidHome
    $process = Start-Process -FilePath (Join-Path $sdk 'emulator\emulator.exe') -WindowStyle Hidden -PassThru `
        -ArgumentList @('-avd', 'AppControlAcceptance', '-port', '5580', '-no-window', '-no-audio', '-no-boot-anim', '-no-snapshot', '-gpu', 'swiftshader', '-memory', '4096', '-cores', '2', '-no-metrics', '-skip-adb-auth') `
        -RedirectStandardOutput (Join-Path $emulatorHome 'stdout.txt') -RedirectStandardError (Join-Path $emulatorHome 'stderr.txt')
    Write-Output "Isolated emulator launched: process $($process.Id), ADB serial emulator-5580."
    Write-Output 'AVD data and emulator output stay under workspace .tooling; boot completion must be verified separately.'
} finally {
    $env:ANDROID_AVD_HOME = $savedAvd
    $env:ANDROID_EMULATOR_HOME = $savedEmulator
    $env:ANDROID_USER_HOME = $savedAndroid
}
