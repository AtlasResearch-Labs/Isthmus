# Local Android APK Compiler & Signer
# Uses portable JDK 17, aapt2, r8 (D8), and android.jar to produce an installable isthmus.apk

$ErrorActionPreference = "Stop"
$scriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
$projectRoot = Split-Path -Parent $scriptDir

$jdkBin = "$env:TEMP\jdk17\jdk-17.0.12+7\bin"
$toolsDir = "$env:TEMP\android_build_tools"

$java = "$jdkBin\java.exe"
$javac = "$jdkBin\javac.exe"
$keytool = "$jdkBin\keytool.exe"
$jarsigner = "$jdkBin\jarsigner.exe"

$aapt2 = "$toolsDir\aapt2.exe"
$androidJar = "$toolsDir\android.jar"
$r8Jar = "$toolsDir\r8.jar"

$buildDir = "$env:TEMP\apk_build"
if (Test-Path $buildDir) {
    Remove-Item -Recurse -Force $buildDir
}
New-Item -ItemType Directory -Force -Path $buildDir | Out-Null
$genDir = Join-Path $buildDir "gen"
$compiledResDir = Join-Path $buildDir "compiled_res"
$classesDir = Join-Path $buildDir "classes"
$dexDir = Join-Path $buildDir "dex"
$libDir = Join-Path $buildDir "apk_root\lib\arm64-v8a"

New-Item -ItemType Directory -Force -Path $genDir | Out-Null
New-Item -ItemType Directory -Force -Path $compiledResDir | Out-Null
New-Item -ItemType Directory -Force -Path $classesDir | Out-Null
New-Item -ItemType Directory -Force -Path $dexDir | Out-Null
New-Item -ItemType Directory -Force -Path $libDir | Out-Null

$manifest = Join-Path $projectRoot "android\app\src\main\AndroidManifest.xml"
$resDir = Join-Path $projectRoot "android\app\src\main\res"
$javaSrcDir = Join-Path $projectRoot "android\app\src\main\java"

Write-Host "=========================================" -ForegroundColor Cyan
Write-Host " Compiling Native Android isthmus.apk    " -ForegroundColor Cyan
Write-Host "=========================================" -ForegroundColor Cyan

# Step 1: Compile Android Resources
Write-Host "[1/6] Compiling resources with aapt2..." -ForegroundColor Yellow
$flatRes = Join-Path $compiledResDir "res.zip"
& $aapt2 compile --dir $resDir -o $flatRes

# Step 2: Link Resources and Generate Binary XML + R.java
Write-Host "[2/6] Linking Android binary package and generating R.java..." -ForegroundColor Yellow
$unalignedApk = Join-Path $buildDir "base.apk"
& $aapt2 link -I $androidJar --manifest $manifest --java $genDir -o $unalignedApk $flatRes --auto-add-overlay

# Step 3: Compile Java Sources
Write-Host "[3/6] Compiling Java classes with javac..." -ForegroundColor Yellow
$javaFiles = Get-ChildItem -Path $javaSrcDir, $genDir -Recurse -Filter "*.java" | Select-Object -ExpandProperty FullName
& $javac -cp $androidJar -d $classesDir $javaFiles

# Step 4: Convert Java Classes to Dalvik classes.dex via D8
Write-Host "[4/6] Converting bytecode to Dalvik classes.dex with D8..." -ForegroundColor Yellow
$classFiles = Get-ChildItem -Path $classesDir -Recurse -Filter "*.class" | Select-Object -ExpandProperty FullName
& $java -cp $r8Jar com.android.tools.r8.D8 --lib $androidJar --output $dexDir $classFiles

# Step 5: Bundle Native ARM64 Go Mobile Engine
Write-Host "[5/6] Cross-compiling Go mobile ARM64 engine..." -ForegroundColor Yellow
$env:PATH = "$env:USERPROFILE\go-sdk\bin;$env:PATH"
$env:GOOS = "linux"
$env:GOARCH = "arm64"
$env:CGO_ENABLED = "0"
$mobileLib = Join-Path $libDir "libisthmus.so"
go build -ldflags "-s -w -X main.version=0.5.0" -o $mobileLib (Join-Path $projectRoot "cmd\isthmus\main.go")

# Step 6: Assemble, Align & Sign APK
Write-Host "[6/6] Packaging and signing isthmus.apk..." -ForegroundColor Yellow
$apkFinal = Join-Path $projectRoot "dist\isthmus.apk"

# Extract base.apk to temp apk folder
$apkFolder = Join-Path $buildDir "apk_root"
Add-Type -AssemblyName System.IO.Compression.FileSystem
[System.IO.Compression.ZipFile]::ExtractToDirectory($unalignedApk, $apkFolder)

# Copy classes.dex
Copy-Item (Join-Path $dexDir "classes.dex") -Destination (Join-Path $apkFolder "classes.dex") -Force

# Repack into isthmus.apk
if (Test-Path $apkFinal) { Remove-Item -Force $apkFinal }
[System.IO.Compression.ZipFile]::CreateFromDirectory($apkFolder, $apkFinal)

# Generate signing keystore if needed
$keystore = Join-Path $buildDir "isthmus.keystore"
& $keytool -genkeypair -v -keystore $keystore -alias isthmus -keyalg RSA -keysize 2048 -validity 10000 -storepass isthmus123 -keypass isthmus123 -dname "CN=Isthmus, OU=Mesh, O=Isthmus, L=Global, S=Global, C=US"

# Sign APK with jarsigner (v1 APK Signature for instant phone compatibility)
& $jarsigner -keystore $keystore -storepass isthmus123 -keypass isthmus123 $apkFinal isthmus

Write-Host "=========================================" -ForegroundColor Green
Write-Host " SUCCESS! Built installable isthmus.apk  " -ForegroundColor Green
Write-Host " Output: $apkFinal" -ForegroundColor Green
Write-Host " File Size: $((Get-Item $apkFinal).Length / 1MB) MB" -ForegroundColor Green
Write-Host "=========================================" -ForegroundColor Green
