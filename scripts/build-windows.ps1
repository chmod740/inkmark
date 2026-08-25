$ErrorActionPreference = "Stop"

$ProjectDir = Split-Path -Parent $PSScriptRoot
Set-Location $ProjectDir
$NoticeSource = Join-Path $ProjectDir "THIRD_PARTY_NOTICES.txt"
if (-not (Test-Path $NoticeSource -PathType Leaf) -or (Get-Item $NoticeSource).Length -eq 0) {
    throw "Missing third-party notice: $NoticeSource"
}

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
pnpm --dir frontend test:preview
Assert-LastExitCode "Run atomic preview rendering tests"
pnpm --dir frontend test:markdown
Assert-LastExitCode "Run Markdown extension tests"
pnpm --dir frontend test:dialects
Assert-LastExitCode "Run Markdown dialect tests"
pnpm --dir frontend test:diagrams
Assert-LastExitCode "Run extended diagram safety tests"
pnpm --dir frontend test:update
Assert-LastExitCode "Run update workflow tests"
pnpm --dir frontend test:ui
Assert-LastExitCode "Run UI state tests"
pnpm --dir frontend test:themes
Assert-LastExitCode "Run theme palette tests"
pnpm --dir frontend test:find
Assert-LastExitCode "Run text search tests"
pnpm --dir frontend test:fonts
pnpm --dir frontend test:editor-history
Assert-LastExitCode "Run font preference tests"
pnpm --dir frontend test:workspace
pnpm --dir frontend test:tabs
Assert-LastExitCode "Run workspace tree tests"
pnpm --dir frontend test:webdav
Assert-LastExitCode "Run WebDAV UI tests"
pnpm --dir frontend test:image
Assert-LastExitCode "Run image resource tests"
pnpm --dir frontend test:saved-webdav
Assert-LastExitCode "Run saved WebDAV connection tests"
pnpm --dir frontend test:installer
Assert-LastExitCode "Verify Windows installer configuration"
node scripts/verify-offline.mjs
Assert-LastExitCode "Verify offline assets"
pnpm --dir frontend test:notices
Assert-LastExitCode "Verify third-party notice"

$Executable = Join-Path $ProjectDir "build\bin\inkmark.exe"
if (-not (Test-Path $Executable)) {
    throw "Windows build output is missing: $Executable"
}
$PortableNotice = Join-Path $ProjectDir "build\bin\THIRD_PARTY_NOTICES.txt"
Copy-Item -LiteralPath $NoticeSource -Destination $PortableNotice -Force
if ((Get-FileHash -Algorithm SHA256 $NoticeSource).Hash -ne (Get-FileHash -Algorithm SHA256 $PortableNotice).Hash) {
    throw "Portable third-party notice copy does not match the release notice"
}

Write-Host "Windows app: $Executable"
Write-Host "Windows portable notice: $PortableNotice"
$Installer = Get-ChildItem -Path (Join-Path $ProjectDir "build\bin") -Filter "*installer.exe" -File | Select-Object -First 1
if ($Installer) {
    Write-Host "Windows installer: $($Installer.FullName)"
} else {
    Write-Warning "NSIS installer was not generated; install makensis to package file associations."
}
