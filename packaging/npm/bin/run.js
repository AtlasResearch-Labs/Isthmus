#!/usr/bin/env node

'use strict';

const fs = require('fs');
const path = require('path');
const https = require('https');
const { spawn } = require('child_process');

const VERSION = '0.5.0';

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
  console.error(`\x1b[31m[Isthmus] Unsupported OS or CPU architecture: ${process.platform} ${process.arch}\x1b[0m`);
  process.exit(1);
}

const binaryName = os === 'windows' ? `isthmus-windows-${arch}.exe` : `isthmus-${os}-${arch}`;
const targetExe = os === 'windows' ? 'isthmus.exe' : 'isthmus';

// Cache in user's home directory under .isthmus/bin
const homeDir = process.env.USERPROFILE || process.env.HOME || '.';
const cacheDir = path.join(homeDir, '.isthmus', 'bin');
const cachedBin = path.join(cacheDir, targetExe);

function executeBinary(binPath) {
  if (process.platform !== 'win32') {
    try {
      fs.chmodSync(binPath, 0o755);
    } catch (_) {}
  }

  const child = spawn(binPath, process.argv.slice(2), {
    stdio: 'inherit',
    windowsHide: false
  });

  child.on('exit', (code, signal) => {
    if (signal) {
      process.kill(process.pid, signal);
    } else {
      process.exit(code !== null ? code : 0);
    }
  });

  child.on('error', (err) => {
    console.error(`\x1b[31m[Isthmus] Execution failed: ${err.message}\x1b[0m`);
    process.exit(1);
  });

  process.on('SIGINT', () => {
    try { child.kill('SIGINT'); } catch (_) {}
  });
  process.on('SIGTERM', () => {
    try { child.kill('SIGTERM'); } catch (_) {}
  });
}

function isValidExecutable(filePath) {
  try {
    if (!fs.existsSync(filePath)) return false;
    const stat = fs.statSync(filePath);
    return stat.isFile() && stat.size > 100000;
  } catch (_) {
    return false;
  }
}

// 1. Check bundled native engine inside npm package
const bundledCandidates = [
  path.join(__dirname, 'native', binaryName),
  path.join(__dirname, 'native', targetExe),
  path.join(__dirname, binaryName),
  path.join(__dirname, targetExe)
];

for (const candidate of bundledCandidates) {
  if (isValidExecutable(candidate)) {
    executeBinary(candidate);
    return;
  }
}

// 2. Check cached binary in ~/.isthmus/bin/
if (isValidExecutable(cachedBin)) {
  executeBinary(cachedBin);
  return;
}

// 3. Check local workspace / repo build artifacts
const workspaceCandidates = [
  path.join(process.cwd(), targetExe),
  path.join(process.cwd(), 'dist', 'binaries', binaryName),
  path.join(process.cwd(), 'bin', `${os}-${arch}`, targetExe),
  path.join(process.cwd(), 'bin', binaryName)
];

for (const candidate of workspaceCandidates) {
  if (isValidExecutable(candidate)) {
    executeBinary(candidate);
    return;
  }
}

// 4. Download binary into cache directory
if (!fs.existsSync(cacheDir)) {
  try {
    fs.mkdirSync(cacheDir, { recursive: true });
  } catch (err) {
    console.error(`\x1b[31m[Isthmus] Failed to create cache directory: ${err.message}\x1b[0m`);
    process.exit(1);
  }
}

const downloadUrls = [
  `https://github.com/AtlasResearch-Labs/Isthmus/releases/download/v${VERSION}/${binaryName}`,
  `https://github.com/AtlasResearch-Labs/Isthmus/releases/latest/download/${binaryName}`,
  `https://raw.githubusercontent.com/AtlasResearch-Labs/Isthmus/main/dist/binaries/${binaryName}`
];

console.log(`\x1b[36m⚡ [Isthmus] Initializing native mesh engine for ${os}-${arch}...\x1b[0m`);

function tryDownload(urls, index, dest, onComplete) {
  if (index >= urls.length) {
    console.error(`\x1b[31m[Isthmus] Failed to retrieve native binary from all distribution mirrors.\x1b[0m`);
    console.error(`Please install manually or download from: https://github.com/AtlasResearch-Labs/Isthmus/releases`);
    process.exit(1);
  }

  const url = urls[index];

  function fetchUrl(targetUrl, redirectCount = 0) {
    if (redirectCount > 5) {
      return tryDownload(urls, index + 1, dest, onComplete);
    }

    const req = https.get(targetUrl, (res) => {
      if (res.statusCode >= 300 && res.statusCode < 400 && res.headers.location) {
        return fetchUrl(res.headers.location, redirectCount + 1);
      }

      if (res.statusCode !== 200) {
        res.resume();
        return tryDownload(urls, index + 1, dest, onComplete);
      }

      const totalBytes = parseInt(res.headers['content-length'] || '0', 10);
      let downloadedBytes = 0;
      let lastPercent = -1;

      const file = fs.createWriteStream(dest);

      res.on('data', (chunk) => {
        downloadedBytes += chunk.length;
        if (totalBytes > 0) {
          const percent = Math.floor((downloadedBytes / totalBytes) * 100);
          if (percent !== lastPercent && percent % 10 === 0) {
            lastPercent = percent;
            process.stdout.write(`\r\x1b[33m[Isthmus] Downloading: ${percent}% (${(downloadedBytes / (1024 * 1024)).toFixed(1)} MB / ${(totalBytes / (1024 * 1024)).toFixed(1)} MB)...\x1b[0m`);
          }
        }
      });

      res.pipe(file);

      file.on('finish', () => {
        file.close(() => {
          if (totalBytes > 0 && downloadedBytes < totalBytes) {
            try { fs.unlinkSync(dest); } catch (_) {}
            return tryDownload(urls, index + 1, dest, onComplete);
          }
          if (process.platform !== 'win32') {
            try { fs.chmodSync(dest, 0o755); } catch (_) {}
          }
          console.log(`\n\x1b[32m✔ [Isthmus] Engine ready! Launching...\x1b[0m\n`);
          onComplete();
        });
      });

      file.on('error', (err) => {
        try { fs.unlinkSync(dest); } catch (_) {}
        tryDownload(urls, index + 1, dest, onComplete);
      });
    });

    req.on('error', () => {
      tryDownload(urls, index + 1, dest, onComplete);
    });

    req.setTimeout(30000, () => {
      req.destroy();
      tryDownload(urls, index + 1, dest, onComplete);
    });
  }

  fetchUrl(url);
}

tryDownload(downloadUrls, 0, cachedBin, () => {
  executeBinary(cachedBin);
});
