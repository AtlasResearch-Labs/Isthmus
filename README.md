# Isthmus

Cross-device secure tunnel and file access system designed to connect machines seamlessly across LAN and WAN networks.

---

## Features

- **Curve25519 Cryptographic Identity**: Every node generates standard Curve25519 keypairs and SHA-256 derived device IDs.
- **Embedded SFTP Engine**: Zero dependency on OS SSH daemons. The agent runs an embedded SFTP server and client with cross-platform parity across Windows, Linux, and macOS.
- **3-Tier Auto-Routing**:
  - **Tier 1 (LAN Direct)**: Zero-config subnet discovery via UDP broadcast beacons.
  - **Tier 2 (WAN Direct)**: STUN-style public IP:port exchange for direct P2P connectivity.
  - **Tier 3 (DERP Relay Fallback)**: Encrypted packet relay when firewalls or symmetric NAT block direct connections.
- **Recursive Delta Sync**: Scans folder hierarchies, compares file sizes/timestamps, and synchronizes only modified or missing files.
- **Resumable Transfers**: Automatically resumes interrupted transfers from the last received byte offset and verifies end-to-end SHA-256 checksums.
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

# Build binaries
go build -o bin/isthmus ./cmd/isthmus
go build -o bin/isthmus-coord ./cmd/isthmus-coord
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

### 2. Check Node Status
```bash
isthmus status
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

### 5. Browse Remote Files
List files on a remote peer:
```bash
isthmus browse <peer-name-or-id> [remote-path]
```

### 6. Transfer Files
```bash
# Pull a file (with auto-resume and SHA-256 verification)
isthmus pull <peer-name-or-id> <remote-file> [local-destination]

# Push a file to peer
isthmus push <peer-name-or-id> <local-file> [remote-destination]
```

### 7. Folder Synchronization
Recursively delta-sync an entire directory tree:
```bash
isthmus sync <peer-name-or-id> <remote-dir> [local-dir]
```

### 8. Coordination Server
Manage connection to the coordination control plane:
```bash
isthmus coord set http://coord.example.com:8080
isthmus coord status
```

---

## Coordination Server Deployment

Deploy the coordination server binary on a public cloud VM:

```bash
# Start coordination server on port 8080 with relay on port 8081
./isthmus-coord -port 8080 -relay-port 8081
```

---

## Dependencies

- `golang.org/x/crypto`: Curve25519 scalar multiplication and SSH protocol transport.
- `github.com/pkg/sftp`: Embedded SFTP protocol implementation.

---

## License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.
