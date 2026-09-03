# Isthmus Windows 1-Click Installer
# Installs Isthmus to %LOCALAPPDATA%\isthmus\bin, updates PATH, and creates shortcuts

$ErrorActionPreference = "Stop"
$scriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path

$installRoot = Join-Path $env:LOCALAPPDATA "isthmus"
$binDir = Join-Path $installRoot "bin"

Write-Host "=========================================" -ForegroundColor Cyan
Write-Host "    Installing Isthmus Mesh Engine       " -ForegroundColor Cyan
Write-Host "=========================================" -ForegroundColor Cyan

# 1. Create Directories
Write-Host "[1/4] Setting up directories at $installRoot..." -ForegroundColor Yellow
New-Item -ItemType Directory -Force -Path $binDir | Out-Null

# 2. Copy or Download Executables
$targetExe = Join-Path $binDir "isthmus.exe"
$localCandidate = $null

if ($PSScriptRoot) {
    if (Test-Path (Join-Path $PSScriptRoot "bin\isthmus.exe")) {
        $localCandidate = Join-Path $PSScriptRoot "bin\isthmus.exe"
    } elseif (Test-Path (Join-Path $PSScriptRoot "isthmus.exe")) {
        $localCandidate = Join-Path $PSScriptRoot "isthmus.exe"
    }
}

if ($localCandidate -and (Test-Path $localCandidate)) {
    Write-Host "[2/4] Copying binary from $localCandidate..." -ForegroundColor Yellow
    Copy-Item $localCandidate -Destination $targetExe -Force
} else {
    Write-Host "[2/4] Downloading latest Isthmus Windows binary from GitHub..." -ForegroundColor Yellow
    $downloadUrl = "https://github.com/AtlasResearch-Labs/Isthmus/releases/latest/download/isthmus-windows-amd64.exe"
    try {
        [Net.ServicePointManager]::SecurityProtocol = [Net.SecurityProtocolType]::Tls12
        Invoke-WebRequest -Uri $downloadUrl -OutFile $targetExe -UseBasicParsing
    } catch {
        Write-Host "  -> Releases download failed, fetching from repository..." -ForegroundColor Yellow
        Invoke-WebRequest -Uri "https://raw.githubusercontent.com/AtlasResearch-Labs/Isthmus/main/dist/binaries/isthmus-windows-amd64.exe" -OutFile $targetExe -UseBasicParsing
    }
}

# 3. Add to User Environment PATH
Write-Host "[3/4] Registering in PATH..." -ForegroundColor Yellow
$userPath = [Environment]::GetEnvironmentVariable("PATH", "User")
if ($userPath -notlike "*$binDir*") {
    $newPath = "$binDir;$userPath"
    [Environment]::SetEnvironmentVariable("PATH", $newPath, "User")
    $env:PATH = "$binDir;$env:PATH"
    Write-Host "  -> Added '$binDir' to User PATH." -ForegroundColor Green
} else {
    Write-Host "  -> PATH already configured." -ForegroundColor Gray
}

# 4. Create Desktop & Start Menu Shortcuts
Write-Host "[4/4] Creating Desktop and Start Menu shortcuts..." -ForegroundColor Yellow
try {
    $wshShell = New-Object -ComObject WScript.Shell

    # Desktop Shortcut
    $desktopPath = [Environment]::GetFolderPath("Desktop")
    $shortcutDesktop = $wshShell.CreateShortcut((Join-Path $desktopPath "Isthmus Studio.lnk"))
    $shortcutDesktop.TargetPath = (Join-Path $binDir "isthmus.exe")
    $shortcutDesktop.Arguments = "gui --port 7788"
    $shortcutDesktop.Description = "Isthmus Cross-Device Mesh Studio Workbench"
    $shortcutDesktop.Save()

    # Start Menu Shortcut
    $startMenuPrograms = [Environment]::GetFolderPath("Programs")
    $shortcutStart = $wshShell.CreateShortcut((Join-Path $startMenuPrograms "Isthmus Studio.lnk"))
    $shortcutStart.TargetPath = (Join-Path $binDir "isthmus.exe")
    $shortcutStart.Arguments = "gui --port 7788"
    $shortcutStart.Description = "Isthmus Cross-Device Mesh Studio Workbench"
    $shortcutStart.Save()

    Write-Host "  -> Shortcuts created successfully." -ForegroundColor Green
} catch {
    Write-Host "  -> Skipped shortcut creation: $($_.Exception.Message)" -ForegroundColor Gray
}

Write-Host "=========================================" -ForegroundColor Green
Write-Host " Installation Complete! " -ForegroundColor Green
Write-Host " You can now run 'isthmus gui' or 'isthmus' in any terminal." -ForegroundColor Green
Write-Host "=========================================" -ForegroundColor Green
