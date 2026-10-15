# Build stripped release binaries for Isthmus v0.5.0
$ErrorActionPreference = "Stop"
$scriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
$projectRoot = Split-Path -Parent $scriptDir
$distBin = Join-Path $projectRoot "dist\binaries"

if (-not (Test-Path $distBin)) {
    New-Item -ItemType Directory -Force -Path $distBin | Out-Null
}

$version = "0.5.0"
$targets = @(
    @{ OS = "windows"; Arch = "amd64"; Ext = ".exe"; Name = "isthmus-windows-amd64.exe" },
    @{ OS = "windows"; Arch = "arm64"; Ext = ".exe"; Name = "isthmus-windows-arm64.exe" },
    @{ OS = "linux";   Arch = "amd64"; Ext = "";     Name = "isthmus-linux-amd64" },
    @{ OS = "linux";   Arch = "arm64"; Ext = "";     Name = "isthmus-linux-arm64" },
    @{ OS = "linux";   Arch = "arm";   Ext = "";     Name = "isthmus-linux-armv7" },
    @{ OS = "darwin";  Arch = "arm64"; Ext = "";     Name = "isthmus-darwin-arm64" },
    @{ OS = "darwin";  Arch = "amd64"; Ext = "";     Name = "isthmus-darwin-amd64" }
)

Write-Host "Compiling stripped release matrix for v$version..." -ForegroundColor Cyan

foreach ($t in $targets) {
    $outPath = Join-Path $distBin $t.Name
    Write-Host "  -> Compiling $($t.OS)/$($t.Arch) -> $($t.Name)..." -ForegroundColor Yellow
    $env:GOOS = $t.OS
    $env:GOARCH = $t.Arch
    $env:CGO_ENABLED = "0"
    if ($t.Arch -eq "arm") {
        $env:GOARM = "7"
    } else {
        $env:GOARM = ""
    }
    go build -trimpath -ldflags "-s -w -X main.version=$version" -o $outPath (Join-Path $projectRoot "cmd\isthmus\main.go")
}

# Copy Windows x64 binary to root isthmus.exe
Copy-Item (Join-Path $distBin "isthmus-windows-amd64.exe") (Join-Path $projectRoot "isthmus.exe") -Force

# Reset environment
$env:GOOS = ""
$env:GOARCH = ""
$env:GOARM = ""

Write-Host "`nAll 7 release binaries successfully generated in dist/binaries/!" -ForegroundColor Green
Get-ChildItem -Path $distBin | Select-Object Name, Length, LastWriteTime
