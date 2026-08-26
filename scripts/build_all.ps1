# Multi-platform cross-compilation script for Isthmus
$ErrorActionPreference = "Stop"

$targets = @(
    @{ OS = "windows"; Arch = "amd64"; OutputExt = ".exe" },
    @{ OS = "windows"; Arch = "arm64"; OutputExt = ".exe" },
    @{ OS = "linux";   Arch = "amd64"; OutputExt = "" },
    @{ OS = "linux";   Arch = "arm64"; OutputExt = "" },
    @{ OS = "darwin";  Arch = "amd64"; OutputExt = "" },
    @{ OS = "darwin";  Arch = "arm64"; OutputExt = "" }
)

Write-Host "=================================================="
Write-Host "Starting Isthmus Multi-Platform Compilation Pipeline"
Write-Host "=================================================="

foreach ($t in $targets) {
    $osName = $t.OS
    $arch = $t.Arch
    $ext = $t.OutputExt

    $outDir = "bin/$osName-$arch"
    if (-not (Test-Path $outDir)) {
        New-Item -ItemType Directory -Force -Path $outDir | Out-Null
    }

    $binIsthmus = "$outDir/isthmus$ext"
    $binCoord   = "$outDir/isthmus-coord$ext"

    Write-Host "[BUILD] Target: $osName / $arch -> $binIsthmus"
    $env:GOOS = $osName
    $env:GOARCH = $arch
    $env:CGO_ENABLED = "0"

    go build -ldflags="-s -w" -o $binIsthmus ./cmd/isthmus
    if ($LASTEXITCODE -ne 0) {
        Write-Error "Failed to build isthmus for $osName/$arch"
    }

    if ($osName -eq "linux" -or ($osName -eq "windows" -and $arch -eq "amd64")) {
        Write-Host "[BUILD] Target: $osName / $arch -> $binCoord"
        go build -ldflags="-s -w" -o $binCoord ./cmd/isthmus-coord
        if ($LASTEXITCODE -ne 0) {
            Write-Error "Failed to build isthmus-coord for $osName/$arch"
        }
    }
}

# Reset environment
$env:GOOS = ""
$env:GOARCH = ""

Write-Host "=================================================="
Write-Host "Multi-Platform Compilation Complete. Output in bin/"
Write-Host "=================================================="
Get-ChildItem -Recurse bin | Select-Object FullName, Length
