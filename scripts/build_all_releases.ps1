# Isthmus Master Multi-Platform Release Matrix Builder
# Builds binary releases and installers across Windows, Linux, macOS, and Android

$ErrorActionPreference = "Stop"
$scriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
$projectRoot = Split-Path -Parent $scriptDir
$distDir = Join-Path $projectRoot "dist"

Write-Host "========================================================" -ForegroundColor Cyan
Write-Host "   ISTHMUS MASTER MULTI-PLATFORM RELEASE BUILD PIPELINE " -ForegroundColor Cyan
Write-Host "========================================================" -ForegroundColor Cyan

$env:PATH = "$env:USERPROFILE\go-sdk\bin;$env:PATH"
$env:CGO_ENABLED = "0"
$version = "0.5.0"

New-Item -ItemType Directory -Force -Path $distDir | Out-Null
$binDir = Join-Path $distDir "binaries"
New-Item -ItemType Directory -Force -Path $binDir | Out-Null

$targets = @(
    @{ OS = "windows"; Arch = "amd64"; Ext = ".exe"; Name = "isthmus-windows-amd64.exe" },
    @{ OS = "windows"; Arch = "arm64"; Ext = ".exe"; Name = "isthmus-windows-arm64.exe" },
    @{ OS = "linux";   Arch = "amd64"; Ext = "";     Name = "isthmus-linux-amd64" },
    @{ OS = "linux";   Arch = "arm64"; Ext = "";     Name = "isthmus-linux-arm64" },
    @{ OS = "linux";   Arch = "arm";   Ext = "";     Name = "isthmus-linux-armv7" },
    @{ OS = "darwin";  Arch = "arm64"; Ext = "";     Name = "isthmus-darwin-arm64" },
    @{ OS = "darwin";  Arch = "amd64"; Ext = "";     Name = "isthmus-darwin-amd64" }
)

Write-Host "`n[1/4] Cross-Compiling Multi-Arch Binaries..." -ForegroundColor Yellow
foreach ($t in $targets) {
    $outPath = Join-Path $binDir $t.Name
    Write-Host "  -> Building $($t.OS)/$($t.Arch) -> $($t.Name)..." -ForegroundColor Gray
    $env:GOOS = $t.OS
    $env:GOARCH = $t.Arch
    if ($t.Arch -eq "arm") {
        $env:GOARM = "7"
    } else {
        $env:GOARM = ""
    }
    go build -ldflags "-s -w -X main.version=$version" -o $outPath (Join-Path $projectRoot "cmd\isthmus\main.go")
}

Write-Host "`n[2/4] Building Windows Standalone Bundle & Installer..." -ForegroundColor Yellow
& (Join-Path $scriptDir "package_windows.ps1")

Write-Host "`n[3/4] Building Linux Debian (.deb) Packages..." -ForegroundColor Yellow
& (Join-Path $scriptDir "package_deb.ps1")

Write-Host "`n[4/4] Building Android Mobile Core & Project Bundle..." -ForegroundColor Yellow
& (Join-Path $scriptDir "build_android.ps1")

Write-Host "`n========================================================" -ForegroundColor Green
Write-Host "   ALL PLATFORM PACKAGES BUILT & VERIFIED SUCCESSFULLY! " -ForegroundColor Green
Write-Host "========================================================" -ForegroundColor Green

Get-ChildItem -Path $distDir -File | Format-Table Name, Length, LastWriteTime -AutoSize
