$ErrorActionPreference = "Stop"

$ProjectDir = Split-Path -Parent $PSScriptRoot
Set-Location $ProjectDir

function Assert-LastExitCode {
    param([string]$Step)
    if ($LASTEXITCODE -ne 0) {
        throw "$Step failed with exit code $LASTEXITCODE"
    }
}

$GoCommand = Get-Command go -ErrorAction Stop
$GoCompiler = (Resolve-Path $GoCommand.Source).Path
$GoDirectory = Split-Path -Parent $GoCompiler
$env:Path = "$GoDirectory;$env:Path"

$NodeCommand = Get-Command node -ErrorAction Stop
$NodeDirectory = Split-Path -Parent (Resolve-Path $NodeCommand.Source).Path
$PnpmCommand = Get-Command pnpm -ErrorAction Stop
$PnpmDirectory = Split-Path -Parent (Resolve-Path $PnpmCommand.Source).Path
$env:Path = "$PnpmDirectory;$NodeDirectory;$env:Path"

$WailsCommand = Get-Command wails -ErrorAction SilentlyContinue
if (-not $WailsCommand) {
    & $GoCompiler install github.com/wailsapp/wails/v2/cmd/wails@v2.13.0
    Assert-LastExitCode "Install Wails"
    $GoPath = & $GoCompiler env GOPATH
    Assert-LastExitCode "Read GOPATH"
    $env:Path = "$GoPath\bin;$env:Path"
    $WailsCommand = Get-Command wails -ErrorAction Stop
}
$WailsExecutable = (Resolve-Path $WailsCommand.Source).Path
$env:Path = "$(Split-Path -Parent $WailsExecutable);$env:Path"

& $WailsExecutable build -clean -trimpath -nsis -installscope user -webview2 error -compiler $GoCompiler
Assert-LastExitCode "Build Windows app"
& $GoCompiler test ./...
Assert-LastExitCode "Run Go tests"
pnpm --dir frontend typecheck
Assert-LastExitCode "Run frontend type check"
pnpm --dir frontend test:i18n
Assert-LastExitCode "Run language tests"
pnpm --dir frontend test:export
Assert-LastExitCode "Run export tests"
pnpm --dir frontend test:scroll
Assert-LastExitCode "Run scroll sync tests"
pnpm --dir frontend test:ui
Assert-LastExitCode "Run UI state tests"
pnpm --dir frontend test:installer
Assert-LastExitCode "Verify Windows installer configuration"
node scripts/verify-offline.mjs
Assert-LastExitCode "Verify offline assets"

$Executable = Join-Path $ProjectDir "build\bin\inkmark.exe"
if (-not (Test-Path $Executable)) {
    throw "Windows build output is missing: $Executable"
}

Write-Host "Windows app: $Executable"
$Installer = Get-ChildItem -Path (Join-Path $ProjectDir "build\bin") -Filter "*installer.exe" -File | Select-Object -First 1
if ($Installer) {
    Write-Host "Windows installer: $($Installer.FullName)"
} else {
    Write-Warning "NSIS installer was not generated; install makensis to package file associations."
}
