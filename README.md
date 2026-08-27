<p align="center">
  <img src="assets/isthmus-logo.png" alt="Isthmus Logo" width="180" height="180" />
</p>

<h1 align="center">Isthmus</h1>

<p align="center">
  <b>Cross-device secure tunnel and file access system designed to connect machines seamlessly across LAN and WAN networks.</b><br>
  <a href="docs/USER_MANUAL.md"><b>[ Read the Full User Manual &amp; Operating Guide ]</b></a>
</p>

---

## Table of Contents

1. [Features](#features)
2. [Architecture](#architecture)
3. [Installation & Getting Started](#installation--getting-started)
   - [Windows Installation](#windows-installation)
   - [Linux Installation](#linux-installation)
   - [macOS Installation](#macos-installation)
   - [Building from Source](#building-from-source)
4. [Core Concepts](#core-concepts)
   - [Cryptographic Device Identity](#cryptographic-device-identity)
   - [Shared Directory Root](#shared-directory-root)
   - [3-Tier Dynamic Auto-Routing](#3-tier-dynamic-auto-routing)
5. [Step-by-Step Practical Tutorials](#step-by-step-practical-tutorials)
   - [Tutorial 1: Zero-Config LAN File Transfers](#tutorial-1-zero-config-lan-file-transfers)
   - [Tutorial 2: Connecting Devices Across the Internet (WAN)](#tutorial-2-connecting-devices-across-the-internet-wan)
   - [Tutorial 3: Using the Retro Windows Interactive TUI](#tutorial-3-using-the-retro-windows-interactive-tui)
   - [Tutorial 4: Recursive Directory Delta Sync](#tutorial-4-recursive-directory-delta-sync)
   - [Tutorial 5: Bandwidth Throttling](#tutorial-5-bandwidth-throttling)
   - [Tutorial 6: Configuring Path Access Control Lists (ACLs)](#tutorial-6-configuring-path-access-control-lists-acls)
   - [Tutorial 7: Running as a Headless Background Service](#tutorial-7-running-as-a-headless-background-service)
6. [Complete CLI Command Reference](#complete-cli-command-reference)
7. [Coordination Server Deployment Guide](#coordination-server-deployment-guide)
8. [Security & Cryptographic Architecture](#security--cryptographic-architecture)
9. [Troubleshooting & FAQ](#troubleshooting--faq)
10. [License](#license)

---

## Features

- **Curve25519 Cryptographic Identity**: Every node generates standard Curve25519 keypairs and SHA-256 derived device IDs.
- **Embedded SFTP Engine**: Zero dependency on OS SSH daemons. The agent runs an embedded SFTP server and client with cross-platform parity across Windows, Linux, macOS, and Android.
- **Mutual Public Key Authentication**: Validates incoming client keys (`PublicKeyCallback`) against authorized peer identities before granting access.
- **3-Tier Auto-Routing**:
  - **Tier 1 (LAN Direct)**: Zero-config subnet discovery via UDP broadcast beacons on port 7755.
  - **Tier 2 (WAN Direct)**: STUN-style public IP:port exchange for direct P2P connectivity across the internet.
  - **Tier 3 (DERP Relay Fallback)**: Encrypted packet relay when firewalls or symmetric NAT block direct connections.
- **N-Device Mesh Tailnet**: Automatically synchronizes and discovers all active nodes across your private network mesh.
- **Retro Windows OLED Black TUI**: Interactive keyboard-driven file explorer on true OLED black (`#000000`) with crisp white text, slate gray borders, and classic Windows menu blue headers.
- **Per-Peer Path ACLs**: Granular permissions per peer, including read/write toggles, path sandboxing (`AllowedPaths`), and security deny lists (`BlockedPaths`).
- **Bandwidth Throttling**: Token bucket rate limiter supporting flexible throughput bounds (`--limit-rate 500k`, `2M`, `10M`).
- **Recursive Delta Sync & Resume**: Scans folder hierarchies, compares file sizes/timestamps, transfers only modified files, and resumes partial downloads with SHA-256 verification.
- **Crypto-Blind Control Plane**: Coordination and relay servers only pass routing headers and encrypted packets; payload data is never decrypted or inspected.

---

## Architecture

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
        |  Tier 3: DERP Relay Fallback  |<---->|  Tier 3: DERP Relay Fallback  |
        +-------------------------------+      +-------------------------------+
```

---

## Installation & Getting Started

Precompiled stripped binaries for all supported platforms are located in the `bin/` directory.

### Windows Installation

1. Create local install folder:
   ```powershell
   New-Item -ItemType Directory -Force -Path "$env:LOCALAPPDATA\isthmus\bin"
   ```

2. Copy binary:
   ```powershell
   Copy-Item "bin\windows-amd64\isthmus.exe" "$env:LOCALAPPDATA\isthmus\bin\isthmus.exe" -Force
   ```

3. Add to your User PATH:
   ```powershell
   $userPath = [Environment]::GetEnvironmentVariable("Path", "User")
   if ($userPath -notlike "*isthmus\bin*") {
       [Environment]::SetEnvironmentVariable("Path", "$userPath;$env:LOCALAPPDATA\isthmus\bin", "User")
   }
   ```

4. Open a new terminal and verify:
   ```cmd
   isthmus version
   ```

---

### Linux Installation

1. Copy binary to system path:
   ```bash
   sudo cp bin/linux-amd64/isthmus /usr/local/bin/isthmus
   sudo chmod +x /usr/local/bin/isthmus
   ```
   *(For ARM64 devices such as Raspberry Pi or Oracle Cloud ARM instances, use `bin/linux-arm64/isthmus`)*.

2. Verify installation:
   ```bash
   isthmus version
   ```

---

### macOS Installation

1. Copy binary to system path:
   ```bash
   # For Apple Silicon (M1/M2/M3/M4):
   sudo cp bin/darwin-arm64/isthmus /usr/local/bin/isthmus
   sudo chmod +x /usr/local/bin/isthmus

   # For Intel Macs:
   sudo cp bin/darwin-amd64/isthmus /usr/local/bin/isthmus
   sudo chmod +x /usr/local/bin/isthmus
   ```

2. Verify installation:
   ```bash
   isthmus version
   ```

---

### Building from Source

**Prerequisites:** Go 1.22 or higher.

```bash
# Clone the repository
git clone https://github.com/Eren-Jaeger-DEV/Isthmus.git
cd Isthmus

# Download dependencies
go mod download

# Build for current platform
go build -trimpath -o bin/isthmus ./cmd/isthmus
go build -trimpath -o bin/isthmus-coord ./cmd/isthmus-coord
```

---

## Core Concepts

### Cryptographic Device Identity
Every Isthmus node possesses an identity keypair:
- **Private Key**: A 32-byte seed stored in your local configuration (`%APPDATA%\isthmus\config.json` on Windows, `~/.config/isthmus/config.json` on Linux/macOS). Never share this key.
- **Public Key**: Curve25519 / Ed25519 public key used for peer verification.
- **Device ID**: A 32-character hexadecimal fingerprint derived from your public key (e.g. `70d350d1cd708c16d34443f95802a41b`).

### Shared Directory Root
When an Isthmus node runs `isthmus serve` or `isthmus daemon`, it serves files relative to its configured **Shared Directory**:
- Windows Default: `C:\Users\<Username>\IsthmusShare`
- Linux/macOS Default: `~/IsthmusShare`

Connecting clients cannot navigate above this root folder (path sandboxing).

### 3-Tier Dynamic Auto-Routing
When you request a file operation on a peer, Isthmus automatically tries:
1. **Tier 1 (LAN Direct)**: Subnet broadcast on UDP port 7755. If the peer is on the same local network, traffic flows over high-speed LAN.
2. **Tier 2 (WAN Direct)**: If the peer is remote, Isthmus contacts the coordination server to discover the peer's public IP:port (STUN reflection) and establishes a direct P2P connection.
3. **Tier 3 (DERP Relay Fallback)**: If direct connections fail due to restrictive firewalls or symmetric NAT, Isthmus streams encrypted frames through the coordination server's in-memory packet relay.

---

## Step-by-Step Practical Tutorials

### Tutorial 1: Zero-Config LAN File Transfers

Suppose you have **PC1 (Desktop)** and **PC2 (Laptop)** connected to the same home or office WiFi.

#### Step 1: Initialize both machines
On PC1:
```bash
isthmus init --name "desktop"
```

On PC2:
```bash
isthmus init --name "laptop"
```

#### Step 2: Start the server on PC1
On PC1:
```bash
isthmus serve
```
*Output:*
```text
[INFO] [SFTP-Server] SFTP service listening on 0.0.0.0:2222 (Root: C:\Users\HP\IsthmusShare)
[INFO] [Discovery] LAN discovery service active on 0.0.0.0:7755
```

#### Step 3: Discover PC1 from PC2
On PC2:
```bash
isthmus discover
```
*Output:*
```text
[OK] Name: desktop         ID: 70d350d1cd708c16d34443f95802a41b LAN: 192.168.1.100:2222
```

#### Step 4: Add PC1 as a trusted peer on PC2 (and vice versa)
To enable mutual public key authentication, add PC1's public key (displayed in `isthmus status`):
```bash
isthmus peer add <desktop-id> desktop <desktop-public-key> 10.77.0.1
```

#### Step 5: Browse and pull files
On PC2:
```bash
# Browse files on PC1
isthmus browse desktop

# Download a file from PC1
isthmus pull desktop project.zip

# Upload a file to PC1
isthmus push desktop report.pdf
```

---

### Tutorial 2: Connecting Devices Across the Internet (WAN)

When PC1 and PC2 are in different locations (e.g., home and office), configure both to use your cloud coordination server.

#### Step 1: Configure Coordination Server URL on both machines
```bash
isthmus coord set http://your-cloud-vm-ip:8080
```

#### Step 2: Verify WAN connectivity & STUN reflection
```bash
isthmus coord status
```
*Output:*
```text
========================= [COORDINATION SERVER STATUS] =========================
Server URL:     http://your-cloud-vm-ip:8080
Status:         [OK] ONLINE
Reflected WAN:  203.0.113.45:54321
```

#### Step 3: Start the background daemon on PC1
```bash
isthmus daemon
```

#### Step 4: Access PC1 from anywhere
On PC2 (in another city/network):
```bash
isthmus pull desktop documents/contract.pdf
```
Isthmus will automatically resolve PC1 over Tier 2 (WAN Direct via STUN) or fall back to Tier 3 (DERP Relay) without any configuration changes.

---

### Tutorial 3: Using the Retro Windows Interactive TUI

Isthmus features a keyboard-driven Terminal User Interface built on true OLED black (`#000000`).

To launch the interactive explorer:
```bash
isthmus ui desktop
```

```text
================= [ISTHMUS FILE EXPLORER - PEER: DESKTOP | TRANSPORT: LAN] =================
 PATH: /projects
--------------------------------------------------------------------------------------------
      TYPE   NAME                                 SIZE         MODIFIED             
--------------------------------------------------------------------------------------------
  ->  [DIR ] src/                                 <DIR>        2026-08-26 18:30:12
      [FILE] Makefile                             2.4 KB       2026-08-26 19:15:00
      [FILE] app.go                               14.8 KB      2026-08-26 20:01:22
      [FILE] README.md                            5.1 KB       2026-08-26 20:10:45
--------------------------------------------------------------------------------------------
 [*] Total items: 4
 [ENTER] Open   [BACKSPACE] Up   [D] Download   [S] Sync   [R] Refresh   [Q] Quit           
```

#### Keyboard Shortcuts:
| Key | Action |
| :--- | :--- |
| **Up / Down** | Move file cursor selection |
| **PageUp / PageDown** | Scroll by 10 items |
| **Home / End** | Jump to first / last item |
| **Enter** | Open folder / view file info |
| **Backspace / Left** | Navigate up to parent folder |
| **D** | Download currently selected file (with progress bar) |
| **S** | Delta-sync current folder to `./sync_output` |
| **R** | Refresh folder listing |
| **Q / Esc** | Exit file explorer |

---

### Tutorial 4: Recursive Directory Delta Sync

To synchronize an entire folder hierarchy (e.g. syncing remote source code or photos) while skipping already-downloaded and unchanged files:

```bash
isthmus sync desktop "projects/my-app" "./local-backup"
```

*Output:*
```text
[INFO] Connected via LAN Direct transport for folder synchronization.
Syncing [18/18 files] projects/my-app/pkg/server.go
[INFO] Folder sync complete. 14 downloaded, 4 skipped, 12.8 MB in 1.4s
```

- **Unchanged files** (matching size and modification timestamp) are skipped instantly with zero network transfer.
- **Interrupted downloads** automatically resume from the last received byte offset.

---

### Tutorial 5: Bandwidth Throttling

To prevent file transfers from saturating your internet connection, use `--limit-rate`:

```bash
# Limit download speed to 2 MB/s
isthmus pull --limit-rate 2M desktop large-dataset.tar.gz

# Limit upload speed to 500 KB/s
isthmus push --limit-rate 500k desktop database.bak
```

**Supported Rate Formats:**
- `500k` or `500KB` -> 500 Kilobytes/second
- `2M` or `2MB` -> 2 Megabytes/second
- `10M` or `10MB` -> 10 Megabytes/second
- `1G` or `1GB` -> 1 Gigabyte/second

---

### Tutorial 6: Configuring Path Access Control Lists (ACLs)

You can define granular permissions for each peer node to restrict what folders they can access or modify.

#### Restricting a peer to a specific subfolder (Path Sandboxing):
```bash
isthmus acl laptop scope "projects/public"
```
*Result: `laptop` can only read/write inside `C:\Users\HP\IsthmusShare\projects\public`. Requests to access parent folders or other paths are denied.*

#### Blocking sensitive folders (Security Deny List):
```bash
isthmus acl laptop block ".ssh"
isthmus acl laptop block ".env"
isthmus acl laptop block "credentials"
```

#### Disabling write permissions (Read-Only Mode):
```bash
isthmus acl laptop deny-write
```

#### Re-enabling read/write permissions:
```bash
isthmus acl laptop allow-read
isthmus acl laptop allow-write
```

---

### Tutorial 7: Running as a Headless Background Service

To keep Isthmus running persistently in the background on system boot without keeping a terminal open:

#### On Windows (Run in Administrator PowerShell):
```powershell
# 1. Install as Windows Service
isthmus service install

# 2. Start the service
isthmus service start

# 3. Check service status
isthmus service status

# 4. Stop or uninstall service
isthmus service stop
isthmus service uninstall
```

#### On Linux (systemd):
```bash
# 1. Install systemd unit
sudo isthmus service install

# 2. Start service
sudo isthmus service start

# 3. Check status
isthmus service status
```

---

## Complete CLI Command Reference

| Command | Syntax | Description |
| :--- | :--- | :--- |
| `init` | `isthmus init [--name <name>] [--coord <url>] [--force]` | Generates cryptographic identity keypair and default configuration. |
| `status` | `isthmus status` | Displays local node identity, public key, virtual IP, and ports. |
| `devices` | `isthmus devices` | Lists all configured peer devices, virtual IPs, and permissions. |
| `discover` | `isthmus discover [--timeout <dur>]` | Scans local subnet on UDP 7755 for active Isthmus nodes. |
| `serve` | `isthmus serve [--port <p>] [--root <dir>]` | Runs interactive foreground file server and LAN beacon. |
| `daemon` | `isthmus daemon` | Runs continuous background agent with LAN beacons, WAN heartbeat, and tailnet sync. |
| `ui` / `tui` | `isthmus ui <peer> [path]` | Launches the Retro Windows OLED Black interactive TUI file browser. |
| `browse` | `isthmus browse <peer> [remote-path]` | Lists remote files on a peer in a retro Windows table format. |
| `pull` | `isthmus pull [--limit-rate <r>] <peer> <remote> [local]` | Downloads file with progress bar and SHA-256 verification. |
| `push` | `isthmus push [--limit-rate <r>] <peer> <local> [remote]` | Uploads file to remote peer node. |
| `sync` | `isthmus sync <peer> [remote-dir] [local-dir]` | Recursively delta-syncs a directory tree. |
| `acl` | `isthmus acl <peer> <allow-read\|allow-write\|deny-write\|scope\|block> [path]` | Configures granular access control rules per peer. |
| `mesh` | `isthmus mesh <sync\|status>` | Queries and synchronizes active N-device tailnet topology. |
| `service` | `isthmus service <install\|start\|stop\|status\|uninstall>` | Manages headless background OS service (Windows Service / systemd). |
| `coord` | `isthmus coord <status\|set <url>>` | Sets and verifies coordination server endpoint. |
| `peer` | `isthmus peer <list\|add\|rm> [args]` | Manages trusted peer identities manually. |
| `version` | `isthmus version` | Prints binary version and build info. |

---

## Coordination Server Deployment Guide

To connect devices across the internet, deploy `isthmus-coord` on any public Linux cloud VM (e.g. Oracle Cloud Free Tier, AWS EC2, DigitalOcean, or Hetzner).

### 1. Upload Binary to VM
```bash
# Upload Linux ARM64 or x86_64 binary
scp bin/linux-amd64/isthmus-coord user@your-vm-ip:/usr/local/bin/isthmus-coord
ssh user@your-vm-ip "sudo chmod +x /usr/local/bin/isthmus-coord"
```

### 2. Open Firewall Ports on Cloud Security List
- **TCP 8080**: HTTP API & STUN reflection endpoint
- **TCP 8081**: DERP packet relay listener

On Linux VM (`iptables` / `ufw`):
```bash
sudo ufw allow 8080/tcp
sudo ufw allow 8081/tcp
```

### 3. Create systemd Service Unit
On your VM:
```bash
sudo tee /etc/systemd/system/isthmus-coord.service << EOF
[Unit]
Description=Isthmus Coordination & Relay Server
After=network.target

[Service]
Type=simple
ExecStart=/usr/local/bin/isthmus-coord -port 8080 -relay-port 8081
Restart=on-failure
RestartSec=5s

[Install]
WantedBy=multi-user.target
EOF

sudo systemctl daemon-reload
sudo systemctl enable --now isthmus-coord
```

### 4. Verify Server Health
From your local machine:
```bash
curl http://your-vm-ip:8080/api/v1/devices
```
*Should return JSON array `[]`.*

---

## Security & Cryptographic Architecture

1. **Curve25519 & Ed25519 Authentication**:
   - Every node generates a 32-byte cryptographic seed.
   - Public keys are exchanged either through local configuration (`isthmus peer add`) or the tailnet mesh coordinator.
   - Unauthorized clients attempting to connect are rejected during the initial cryptographic handshake before any file metadata or data can be requested.

2. **Payload-Blind Relay (Zero Knowledge)**:
   - In Tier 3 relay fallback mode, the coordination server only reads 32-byte target and source device ID headers to route packets.
   - The relay server has no cryptographic keys and cannot inspect, decrypt, or tamper with file contents.

3. **End-to-End Integrity Verification**:
   - Every file transfer computes a streaming SHA-256 digest on the source and destination in real time.
   - Transfers are verified before the destination file is committed.

---

## Troubleshooting & FAQ

### Q: "SSH handshake failed: unable to authenticate, attempted methods [none publickey]"
- **Cause**: The connecting client's public key is not registered in the destination peer's allowed peer list.
- **Solution**: On the receiving machine, check `isthmus devices` and add the sender's public key using `isthmus peer add <sender-id> <sender-name> <sender-public-key> <virtual-ip>`.

### Q: "Failed to listen on 0.0.0.0:2222: bind: address already in use"
- **Cause**: Another service (or another instance of Isthmus) is already using port 2222.
- **Solution**: Start Isthmus on a custom port using `isthmus serve --port 2223` or change `sftp_port` in `%APPDATA%\isthmus\config.json`.

### Q: "Access denied: path is not in the allowed paths list"
- **Cause**: An ACL rule on the destination peer is restricting access to a specific subfolder.
- **Solution**: On the destination machine, check or update ACLs using `isthmus acl <peer-name> scope <path>`.

### Q: How do I backup my Isthmus identity?
- Simply back up your `config.json` file:
  - Windows: `%APPDATA%\isthmus\config.json`
  - Linux/macOS: `~/.config/isthmus/config.json`

---

## License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.
