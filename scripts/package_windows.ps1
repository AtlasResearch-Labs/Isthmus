# Isthmus Windows Packaging Script
# Creates a standalone portable ZIP distribution and installer for Windows x64 and ARM64

$ErrorActionPreference = "Stop"
$scriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
$projectRoot = Split-Path -Parent $scriptDir
$distDir = Join-Path $projectRoot "dist\windows"

Write-Host "=========================================" -ForegroundColor Cyan
Write-Host " Building Isthmus Windows Distribution " -ForegroundColor Cyan
Write-Host "=========================================" -ForegroundColor Cyan

# Clean dist dir
if (Test-Path $distDir) {
    Remove-Item -Recurse -Force $distDir
}
New-Item -ItemType Directory -Force -Path $distDir | Out-Null
New-Item -ItemType Directory -Force -Path (Join-Path $distDir "bin") | Out-Null

$env:PATH = "$env:USERPROFILE\go-sdk\bin;$env:PATH"

# 1. Build isthmus.exe (Windows AMD64)
Write-Host "[1/4] Compiling isthmus.exe (Windows x64)..." -ForegroundColor Yellow
$env:GOOS = "windows"
$env:GOARCH = "amd64"
$env:CGO_ENABLED = "0"
go build -ldflags "-s -w -X main.version=0.5.0" -o (Join-Path $distDir "bin\isthmus.exe") (Join-Path $projectRoot "cmd\isthmus\main.go")

# 2. Build isthmus-coord.exe
Write-Host "[2/4] Compiling isthmus-coord.exe..." -ForegroundColor Yellow
go build -ldflags "-s -w -X main.version=0.5.0" -o (Join-Path $distDir "bin\isthmus-coord.exe") (Join-Path $projectRoot "cmd\isthmus-coord\main.go")

# 3. Copy documentation and icons
Write-Host "[3/4] Copying Assets & User Manual..." -ForegroundColor Yellow
Copy-Item (Join-Path $projectRoot "README.md") -Destination (Join-Path $distDir "README.md")
Copy-Item (Join-Path $projectRoot "LICENSE") -Destination (Join-Path $distDir "LICENSE")
Copy-Item (Join-Path $projectRoot "docs\USER_MANUAL.md") -Destination (Join-Path $distDir "USER_MANUAL.md")
if (Test-Path (Join-Path $projectRoot "assets\isthmus-logo.png")) {
    Copy-Item (Join-Path $projectRoot "assets\isthmus-logo.png") -Destination (Join-Path $distDir "bin\isthmus-logo.png")
}

# 4. Copy 1-Click Installer
Copy-Item (Join-Path $scriptDir "install_windows.ps1") -Destination (Join-Path $distDir "install.ps1")

# 5. Create Standalone ZIP Bundle
Write-Host "[4/4] Creating ZIP distribution bundle..." -ForegroundColor Yellow
$zipOutput = Join-Path $projectRoot "dist\isthmus_0.5.0_windows_amd64.zip"
Compress-Archive -Path "$distDir\*" -DestinationPath $zipOutput -Force

Write-Host "=========================================" -ForegroundColor Green
Write-Host " SUCCESS: Windows Distribution Ready! " -ForegroundColor Green
Write-Host " Archive: $zipOutput" -ForegroundColor Green
Write-Host "=========================================" -ForegroundColor Green
