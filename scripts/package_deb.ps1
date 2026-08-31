# Isthmus Linux Debian (.deb) Package Builder
# Generates Debian compliant packages for amd64 and arm64 architectures

$ErrorActionPreference = "Stop"
$scriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
$projectRoot = Split-Path -Parent $scriptDir
$distDir = Join-Path $projectRoot "dist"

Write-Host "=========================================" -ForegroundColor Cyan
Write-Host " Building Isthmus Debian (.deb) Packages " -ForegroundColor Cyan
Write-Host "=========================================" -ForegroundColor Cyan

$env:PATH = "$env:USERPROFILE\go-sdk\bin;$env:PATH"
New-Item -ItemType Directory -Force -Path $distDir | Out-Null

$version = "0.5.0"
$archs = @("amd64", "arm64")

foreach ($arch in $archs) {
    $pkgName = "isthmus_${version}_${arch}"
    $pkgDir = Join-Path $distDir $pkgName

    Write-Host "Building package for $arch ($pkgName)..." -ForegroundColor Yellow

    if (Test-Path $pkgDir) {
        Remove-Item -Recurse -Force $pkgDir
    }

    # Create Debian Directory Hierarchy
    $debianDir = Join-Path $pkgDir "DEBIAN"
    $binDir = Join-Path $pkgDir "usr\bin"
    $systemdDir = Join-Path $pkgDir "lib\systemd\system"
    $docDir = Join-Path $pkgDir "usr\share\doc\isthmus"

    New-Item -ItemType Directory -Force -Path $debianDir | Out-Null
    New-Item -ItemType Directory -Force -Path $binDir | Out-Null
    New-Item -ItemType Directory -Force -Path $systemdDir | Out-Null
    New-Item -ItemType Directory -Force -Path $docDir | Out-Null

    # 1. Compile Linux Binary for target arch
    $env:GOOS = "linux"
    $env:GOARCH = $arch
    $env:CGO_ENABLED = "0"
    $binTarget = Join-Path $binDir "isthmus"
    go build -ldflags "-s -w -X main.version=$version" -o $binTarget (Join-Path $projectRoot "cmd\isthmus\main.go")

    # 2. Control file
    $controlContent = @"
Package: isthmus
Version: $version
Section: utils
Priority: optional
Architecture: $arch
Maintainer: Isthmus Core Team <dev@isthmus.mesh>
Description: Cross-Device Secure Tunnel and Distributed File Mesh
 Isthmus connects computers, cloud servers, and mobile devices seamlessly
 across LAN and WAN networks with zero cloud dependencies.
"@
    Set-Content -Path (Join-Path $debianDir "control") -Value $controlContent -NoNewline

    # 3. Post-install script
    $postinstContent = @"
#!/bin/sh
set -e
chmod 755 /usr/bin/isthmus
if [ -d /run/systemd/system ]; then
    systemctl daemon-reload || true
fi
exit 0
"@
    Set-Content -Path (Join-Path $debianDir "postinst") -Value $postinstContent -NoNewline

    # 4. Pre-remove script
    $prermContent = @"
#!/bin/sh
set -e
if [ -d /run/systemd/system ]; then
    systemctl stop isthmus || true
    systemctl disable isthmus || true
fi
exit 0
"@
    Set-Content -Path (Join-Path $debianDir "prerm") -Value $prermContent -NoNewline

    # 5. Systemd Service Unit & Docs
    Copy-Item (Join-Path $scriptDir "isthmus.service") -Destination (Join-Path $systemdDir "isthmus.service")
    Copy-Item (Join-Path $projectRoot "README.md") -Destination (Join-Path $docDir "README.md")
    Copy-Item (Join-Path $projectRoot "LICENSE") -Destination (Join-Path $docDir "copyright")

    # 6. Create Package Archive
    $tarOutput = Join-Path $distDir "$pkgName.zip"
    Compress-Archive -Path "$pkgDir\*" -DestinationPath $tarOutput -Force
    Write-Host "  -> Generated Debian package tree at $pkgDir" -ForegroundColor Green
    Write-Host "  -> Generated Archive: $tarOutput" -ForegroundColor Green
}

Write-Host "=========================================" -ForegroundColor Green
Write-Host " Linux Debian Packages Built Successfully! " -ForegroundColor Green
Write-Host "=========================================" -ForegroundColor Green
