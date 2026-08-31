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

# 2. Copy Executables
Write-Host "[2/4] Copying binaries..." -ForegroundColor Yellow
$srcBin = Join-Path $scriptDir "bin\isthmus.exe"
if (-not (Test-Path $srcBin)) {
    $srcBin = Join-Path $scriptDir "isthmus.exe"
}
Copy-Item $srcBin -Destination (Join-Path $binDir "isthmus.exe") -Force

$srcCoord = Join-Path $scriptDir "bin\isthmus-coord.exe"
if (Test-Path $srcCoord) {
    Copy-Item $srcCoord -Destination (Join-Path $binDir "isthmus-coord.exe") -Force
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
