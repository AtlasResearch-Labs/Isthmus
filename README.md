# Isthmus

Cross-device secure tunnel and file access system designed to connect machines seamlessly across LAN and WAN networks.

---

## Features

- **Curve25519 Cryptographic Identity**: Every node generates standard Curve25519 keypairs and SHA-256 derived device IDs.
- **Embedded SFTP Engine**: Zero dependency on OS SSH daemons. The agent runs an embedded SFTP server and client with cross-platform parity across Windows, Linux, macOS, and Android.
- **3-Tier Auto-Routing**:
  - **Tier 1 (LAN Direct)**: Zero-config subnet discovery via UDP broadcast beacons.
  - **Tier 2 (WAN Direct)**: STUN-style public IP:port exchange for direct P2P connectivity.
  - **Tier 3 (DERP Relay Fallback)**: Encrypted packet relay when firewalls or symmetric NAT block direct connections.
- **N-Device Mesh Tailnet**: Automatically synchronizes and discovers all active nodes across your private network mesh.
- **Retro Windows OLED Black TUI**: Interactive keyboard-driven file explorer on true OLED black with crisp white, gray borders, and classic Windows menu blue headers.
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

## Installation & Build

### Prerequisites
- **Go**: Version 1.22 or higher.

### Building from Source

```bash
# Clone the repository
git clone https://github.com/Eren-Jaeger-DEV/Isthmus.git
cd Isthmus

# Download dependencies
go mod download

# Build for current platform
go build -o bin/isthmus ./cmd/isthmus
go build -o bin/isthmus-coord ./cmd/isthmus-coord

# Or cross-compile for all platforms (Windows, Linux, macOS)
powershell -File scripts/build_all.ps1   # On Windows
bash scripts/build_all.sh               # On Linux / macOS
```

---

## Usage

### 1. Initialize Node
Generate cryptographic identity and local configuration:

```bash
# Default initialization
isthmus init --name "my-pc"

# Initialize with coordination server URL
isthmus init --name "my-pc" --coord "http://coord.example.com:8080"
```

### 2. Check Node Status & Devices
```bash
# View local node status
isthmus status

# View peer directory
isthmus devices
```

### 3. Discover Peers on LAN
Scan the local network for active Isthmus nodes:
```bash
isthmus discover
```

### 4. Run File Server or Daemon
```bash
# Interactive file server
isthmus serve --root "/path/to/share"

# Continuous background daemon (with discovery and WAN heartbeat)
isthmus daemon
```

### 5. Interactive Retro Windows TUI File Explorer
Launch the interactive terminal UI to browse, download, and sync files with arrow keys:
```bash
isthmus ui <peer-name-or-id> [remote-path]
```

### 6. Transfer Files with Rate Limiting
```bash
# Pull a file with bandwidth limit (with auto-resume and SHA-256 verification)
isthmus pull --limit-rate 2M <peer> <remote-file> [local-destination]

# Push a file to peer
isthmus push --limit-rate 5M <peer> <local-file> [remote-destination]
```

### 7. Directory Delta Sync
Recursively delta-sync an entire directory tree:
```bash
isthmus sync <peer> [remote-dir] [local-dir]
```

### 8. Path Access Control Lists (ACLs)
```bash
# Restrict peer to a specific subfolder
isthmus acl laptop scope "projects/demo"

# Block sensitive folders
isthmus acl laptop block ".env"

# Toggle write permissions
isthmus acl laptop deny-write
```

### 9. Tailnet Mesh Synchronization
```bash
isthmus mesh sync
```

### 10. Background OS Service Management
```bash
# Install as background Windows Service or Linux systemd daemon
isthmus service install
isthmus service start
isthmus service status
isthmus service stop
```

---

## Coordination Server Deployment

Deploy the coordination server binary on a public cloud VM:

```bash
# Start coordination server on port 8080 with relay on port 8081
./isthmus-coord -port 8080 -relay-port 8081
```

---

## Supported Platforms

- **Windows**: `windows/amd64`, `windows/arm64`
- **Linux**: `linux/amd64`, `linux/arm64` (ARM cloud servers & Raspberry Pi)
- **macOS**: `darwin/arm64` (Apple Silicon M1/M2/M3), `darwin/amd64` (Intel)
- **Android**: `pkg/mobile` Go bridge for embedding via `gomobile`

---

## License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.
