<p align="center">
  <img src="assets/isthmus-logo.png" alt="Isthmus Logo" width="180" height="180" />
</p>

<h1 align="center">Isthmus</h1>

<p align="center">
  <b>Decentralized Cross-Device Secure Tunnel, Distributed File Mesh, and Studio Workbench.</b><br>
  <i>Connects computers, cloud servers, and mobile devices seamlessly across LAN and WAN networks with zero cloud dependencies.</i><br><br>
  <a href="docs/USER_MANUAL.md"><b>[ Read the Full User Manual &amp; Operating Guide ]</b></a>
</p>

---

## ⚡ What is Isthmus?

**Isthmus** turns all your devices—Windows workstations, Linux cloud VMs, MacBooks, Raspberry Pis, and Android phones—into a unified, private, high-speed mesh.

* **Zero Cloud Accounts / Zero Telemetry**: You own 100% of your data and infrastructure.
* **True 100% Offline Capable**: Works over local Wi-Fi, hotspot, or airplane mode at full hardware speeds (50–120+ MB/s) with zero internet required.
* **2-Way Seamless Experience**:
  1. **High-Density Studio Workbench (Web GUI)** on True OLED Black (`#000000`) with Amber Gold accents.
  2. **Ultra-Snappy Terminal / CLI** for servers, automated headless services, and scripts.

---

## 🚀 Key Features

### 💻 Computing & Remote Management
- **In-Browser Remote Code & Text Editor**: Click any `.go`, `.py`, `.js`, `.json`, `.env`, `.md`, or `.txt` file on any remote peer to edit with live remote save (`Ctrl+S`).
- **Interactive Remote Web Terminal**: Full interactive terminal console connecting directly to remote peers inside the browser.
- **Distributed Fleet Task Runner**: Execute commands or scripts across your entire device fleet simultaneously (`isthmus exec --all "docker ps"`).

### 🔒 Security & Zero-Trust Storage
- **AES-256-GCM Zero-Trust Encrypted Vault**: Client-side authenticated encryption with 50,000-round PBKDF2-SHA256 master key derivation. Remote peers only store opaque ciphertexts (`.enc`).
- **One-Click Magic PIN & QR Pairing**: Ephemeral 6-digit PIN and QR scan to pair devices in 1 second.
- **Per-Peer Granular ACLs**: Path sandboxing (`AllowedPaths`), read/write toggles, and security deny lists (`.ssh`, `.env`, `.git`).

### 📂 File System & Streaming
- **In-Process Virtual Drive (WebDAV)**: Mount your entire mesh storage as a native local drive (`Z:\` on Windows, `/Volumes/Isthmus` on macOS, `/mnt/isthmus` on Linux) with 1 click.
- **Direct In-Browser Media Streaming**: HTTP Range partial content streaming (`http.ServeContent`) for `.mp4`, `.webm`, `.mp3`, `.wav`, and inline photo previewing.
- **Multi-Stream Turbo Transfer Engine**: Slices large files (>10MB) into parallel worker streams with concurrent chunk transfers and SHA-256 block verification (up to 1,400+ MB/s).
- **File Snapshot Time-Machine**: Content-addressed versioning history with 1-click snapshot rollback.
- **Conflict Resolver & 3-Way Merge**: Automated offline divergence detection, backup snapshot creation (`.conflicted_<peer>_<date>`), and visual side-by-side diff comparison.
- **Time-Limited Expiring Guest Share Links**: Shareable cryptographic download tokens with configurable expiration (15m, 1h, 24h) and download quotas for guests.

### 🌐 Networking & Real-Time Sync
- **3-Tier Dynamic Auto-Routing**:
  - *Tier 1 (LAN Direct)*: Subnet discovery via UDP broadcast beacons on port `7755`.
  - *Tier 2 (WAN Direct)*: Direct P2P transfers across the internet using STUN reflection.
  - *Tier 3 (Multi-Hop P2P / DERP Relay)*: Zero-knowledge intermediate peer relaying for symmetric NAT traversal without cloud servers.
- **Universal Magic Clipboard Sync**: Instant real-time clipboard sync across all mesh nodes.
- **Real-Time SSE Push Notifications**: Sliding toast alerts for peer connections, low battery, low disk space, and vault state changes.

---

## 📦 Quick Installation

### 🪟 Windows (1-Click Installer)
Run PowerShell as Administrator or standard user:
```powershell
# Run the automated installer
powershell -ExecutionPolicy Bypass -File .\scripts\install_windows.ps1
```
Or start the Studio Workbench immediately:
```powershell
isthmus gui --port 7788
```

### 🐧 Linux (Debian / Ubuntu / Raspberry Pi)
```bash
# Install Debian package
sudo dpkg -i isthmus_0.5.0_amd64.deb

# Enable and start background daemon service
sudo systemctl enable --now isthmus
```

### 📱 Android (Termux 1-Liner / APK)
In Termux:
```bash
pkg install -y golang git
curl -sSL https://raw.githubusercontent.com/Eren-Jaeger-DEV/Isthmus/main/scripts/install_android.sh | bash
isthmus gui --port 7788
```
Or build/install the native Android project from [`android/`](android/).

---

## 🛠️ CLI Quick Reference

```bash
# 1. Initialize device identity
isthmus init --name "my-laptop"

# 2. Start Studio GUI Workbench
isthmus gui --port 7788

# 3. One-Click Magic PIN Pairing
isthmus pair-code              # Display 6-digit pairing PIN and QR code
isthmus pair-join 583921       # Join device using PIN

# 4. Distributed Fleet Command Execution
isthmus exec --all "uptime"
isthmus exec --target=jack-vm "docker ps"

# 5. Zero-Trust Encrypted Vault
isthmus vault status
isthmus vault encrypt secrets.json my-master-passphrase
isthmus vault decrypt Vault/secrets.json.enc my-master-passphrase

# 6. Mount Mesh as Native Virtual Drive
isthmus mount Z:

# 7. File Transfers & Directory Watching
isthmus pull jack-vm docs/manual.pdf
isthmus push jack-vm build/release.zip
isthmus watch jack-vm ./Projects
```

---

## 🏗️ Architecture Overview

```text
               +--------------------------------------------------------+
               |               Isthmus Coordination Server              |
               |  - Device Registry (/api/v1/register)                  |
               |  - STUN-style Reflection (/api/v1/stun)                |
               |  - Peer Coordinate Exchange (/api/v1/peer-exchange)    |
               |  - DERP Packet Relay Hub (:8081)                       |
               +-------------------^----------------+-------------------+
                                   |                |
                          Register / Heartbeat      |
                          NAT Mapping Keepalive     |
                                   |                |
        +--------------------------+----+      +----+--------------------------+
        |          Node 1 (Agent)       |      |          Node 2 (Agent)       |
        |  Tier 1: LAN Discovery (UDP)  +------+  Tier 1: LAN Discovery (UDP)  |
        |  Tier 2: Direct WAN (STUN)    +------+  Tier 2: Direct WAN (STUN)    |
        |  Tier 3: Multi-Hop Relay      |<---->|  Tier 3: Multi-Hop Relay      |
        +-------------------------------+      +-------------------------------+
```

---

## 📄 License
MIT License. Free and open source for everyone forever.
