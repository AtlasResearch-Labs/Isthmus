#!/usr/bin/env node

const fs = require('fs');
const path = require('path');
const https = require('https');
const { spawn } = require('child_process');

const PLATFORM_MAP = {
  win32: 'windows',
  darwin: 'darwin',
  linux: 'linux'
};

const ARCH_MAP = {
  x64: 'amd64',
  arm64: 'arm64',
  arm: 'armv7'
};

const os = PLATFORM_MAP[process.platform];
const arch = ARCH_MAP[process.arch];

if (!os || !arch) {
  console.error(`[Isthmus] Unsupported OS/architecture: ${process.platform} ${process.arch}`);
  process.exit(1);
}

const binaryName = os === 'windows' ? `isthmus-windows-${arch}.exe` : `isthmus-${os}-${arch}`;
const targetExe = os === 'windows' ? 'isthmus.exe' : 'isthmus';

// Cache in user's home directory under .isthmus/bin
const homeDir = process.env.USERPROFILE || process.env.HOME || '.';
const cacheDir = path.join(homeDir, '.isthmus', 'bin');
const cachedBin = path.join(cacheDir, targetExe);

function executeBinary(binPath) {
  const child = spawn(binPath, process.argv.slice(2), {
    stdio: 'inherit',
    windowsHide: false
  });
  child.on('exit', (code) => {
    process.exit(code !== null ? code : 1);
  });
  child.on('error', (err) => {
    console.error(`[Isthmus] Execution failed: ${err.message}`);
    process.exit(1);
  });
}

// 1. Check local binary in same dir or current workspace
const localCandidate = path.join(__dirname, targetExe);
if (fs.existsSync(localCandidate)) {
  executeBinary(localCandidate);
  return;
}

// 2. Check cached binary in ~/.isthmus/bin
if (fs.existsSync(cachedBin)) {
  executeBinary(cachedBin);
  return;
}

// 3. Download binary from release or repository mirror
if (!fs.existsSync(cacheDir)) {
  fs.mkdirSync(cacheDir, { recursive: true });
}

console.log(`[Isthmus] Fetching native engine for ${os}-${arch}...`);
const url = `https://github.com/AtlasResearch-Labs/Isthmus/releases/latest/download/${binaryName}`;

function download(url, dest, cb) {
  https.get(url, (res) => {
    if (res.statusCode >= 300 && res.statusCode < 400 && res.headers.location) {
      return download(res.headers.location, dest, cb);
    }
    if (res.statusCode !== 200) {
      const rawUrl = `https://raw.githubusercontent.com/AtlasResearch-Labs/Isthmus/main/dist/binaries/${binaryName}`;
      console.log(`[Isthmus] Release artifact pending; fetching from repository mirror...`);
      return downloadRaw(rawUrl, dest, cb);
    }
    const file = fs.createWriteStream(dest);
    res.pipe(file);
    file.on('finish', () => {
      file.close(() => {
        try { fs.chmodSync(dest, 0o755); } catch (_) {}
        cb();
      });
    });
  }).on('error', (err) => {
    console.error(`[Isthmus] Download failed: ${err.message}`);
    process.exit(1);
  });
}

function downloadRaw(url, dest, cb) {
  https.get(url, (res) => {
    if (res.statusCode >= 300 && res.statusCode < 400 && res.headers.location) {
      return downloadRaw(res.headers.location, dest, cb);
    }
    if (res.statusCode !== 200) {
      console.error(`[Isthmus] Failed to download binary (HTTP ${res.statusCode})`);
      process.exit(1);
    }
    const file = fs.createWriteStream(dest);
    res.pipe(file);
    file.on('finish', () => {
      file.close(() => {
        try { fs.chmodSync(dest, 0o755); } catch (_) {}
        cb();
      });
    });
  }).on('error', (err) => {
    console.error(`[Isthmus] Download failed: ${err.message}`);
    process.exit(1);
  });
}

download(url, cachedBin, () => {
  console.log(`[Isthmus] Engine ready! Launching...`);
  executeBinary(cachedBin);
});
