# Isthmus — Debug Findings (Source Review #1)

**Reviewed:** `isthmus-source.zip` (Phase 0 submission)
**Method:** Full `go build ./...`, `go vet ./...`, `go test ./...`, plus manual trace of the actual
connection path (`AutoRouter.DialPeer`) against what the architecture doc claims.
**Verdict:** Builds clean, all tests pass. Two things need fixing before this goes near a real
network — one is a real security hole, the other is a doc/reality mismatch worth resolving on
purpose rather than by accident.

---

## 1. [SECURITY — fix before Phase 2] SFTP server has no authentication wired up

**File:** `pkg/fileserver/server.go`

```go
sshConfig := &ssh.ServerConfig{
    NoClientAuth: true,   // ← this is the default, and nothing overrides it in practice
}

if cfg.AuthPassword != "" {
    sshConfig.NoClientAuth = false
    sshConfig.PasswordCallback = func(c ssh.ConnMetadata, pass []byte) (*ssh.Permissions, error) {
        if string(pass) == cfg.AuthPassword {
            return nil, nil
        }
        return nil, fmt.Errorf("password rejected for %q", c.User())
    }
}
```

**What's wrong:**
- `ServerConfig.AllowedKeys []string` is declared but **never referenced anywhere in the codebase**. No `PublicKeyCallback` exists. It's a dead field — looks wired up, isn't.
- `AuthPassword` is the only thing that can turn auth on, and it's **never set**. Checked every call site:

```go
// cmd/isthmus/main.go — both `serve` and `daemon` commands construct the server like this:
sftpServer, err := fileserver.NewServer(fileserver.ServerConfig{
    Port:    cfg.SFTPPort,
    RootDir: cfg.SharedDir,
    // AuthPassword and AllowedKeys are never set here or anywhere else
})
```

**Net effect:** every `isthmus serve` / `isthmus daemon` node currently accepts **unauthenticated
SFTP connections from anyone who can reach the port** — LAN today, and the raw open internet the
moment Phase 2 exposes this via the Oracle VM. `AutoRouter.DialPeer` does a plain
`net.DialTimeout("tcp", ...)` to whatever address it resolves (LAN, WAN direct, or relay) — there's
no identity check at any layer before the file server itself, and the file server doesn't check
either.

**Why this matters more than a normal TODO:** the whole premise of the project is "SSH into PC1
from anywhere on Earth." An unauthenticated file server reachable from anywhere on Earth is the
literal worst-case version of that.

**The fix is mostly already sitting there unused:** `pkg/identity` already generates Curve25519
keypairs and derives device IDs. The natural fix is wiring `PublicKeyCallback` to check incoming
client public keys against the peer's `AllowedKeys` (or against the ACL registry in `pkg/config`),
rejecting anything not explicitly trusted. This should happen **before** Phase 2 opens anything to
WAN.

---

## 2. [ARCHITECTURE — doc vs reality mismatch] `pkg/tunnel` is a stub; nothing calls it

**File:** `pkg/tunnel/tunnel.go`

Every method is a no-op that logs a convincing message and flips a bool:

```go
func (t *Tunnel) Up() error {
    ...
    t.log.Info("Bringing up WireGuard tunnel interface '%s' with virtual IP %s on port %d", ...)
    t.isRunning = true
    return nil
}

func (t *Tunnel) AddPeer(peer PeerConfig) error {
    ...
    t.log.Info("Configured tunnel peer with key %s ...", ...)
    t.config.Peers = append(t.config.Peers, peer)
    return nil
}
```

No WireGuard device, no UDP listener, no key exchange, no actual packet handling. Confirmed via
grep — **no file outside `pkg/tunnel` imports this package.** It is fully dead code right now.

**What's actually happening instead:** `pkg/discovery/router.go`'s `AutoRouter.DialPeer()` — the
function that really establishes connections — does a plain `net.DialTimeout("tcp", addr, ...)`
against a real IP:port (LAN-discovered, STUN-reflected, or relay-brokered). No WireGuard virtual
IP (`10.77.0.x`) is ever assigned or used anywhere in the current code.

**Is this actually broken?** Not exactly — `pkg/fileserver/server.go` uses a real
`golang.org/x/crypto/ssh` server, so traffic is genuinely encrypted in transit at the SSH layer
even without WireGuard underneath it. Functionally, file transfers aren't going out in plaintext.

**What IS wrong:** `docs/phase0_architecture_and_progress.md` states as fact:

> **Data Plane**: WireGuard (Curve25519 encrypted tunnel) and embedded SFTP for streaming file
> transfers.

That's describing the design target, not what's built. This needs to be a conscious decision, not
an accidental gap:

- **Option A** — Actually build the WireGuard layer (via `wireguard-go` / `golang.zx2c4.com/wireguard`) and route real traffic through it. This is what makes the virtual-IP model work and is generally what makes Phase 2 NAT traversal cleaner (WireGuard's own roaming/keepalive behavior does a lot of the STUN-adjacent work for you).
- **Option B** — Consciously drop WireGuard from the design, keep SSH-over-raw-TCP-per-tier as the only encryption layer, and rewrite the architecture doc to stop claiming a tunnel that doesn't exist.

Either is fine — but right now the doc says A and the code does B, silently. Pick one and make the
doc match the code (or the code match the doc).

---

## 3. [BUG — `go vet` flagged] Mutex copied by value

**File:** `pkg/mobile/android.go`, line 58

```go
defaultCfg, err := config.NewDefaultConfig("android-device")
...
cfg = *defaultCfg   // ← copies config.Config, which embeds a sync.RWMutex, into `cfg`
```

`go vet ./...` output:
```
pkg/mobile/android.go:58:9: assignment copies lock value to cfg: isthmus/pkg/config.Config contains sync.RWMutex
```

Copying a struct that embeds a `sync.RWMutex` is undefined-behavior territory in Go — the copy
gets its own independent (and here, freshly zeroed) mutex, disconnected from any other code
holding a reference to the original. If `defaultCfg`'s mutex is ever locked elsewhere concurrently,
this copy silently desyncs from it — classic "works in testing, breaks under real concurrency" bug.

**Fix:** don't copy the struct. Keep `cfg` as `*config.Config` (a pointer) throughout `StartAgent`,
or extract only the specific fields you need instead of assigning the whole struct by value.

---

## 4. Confirmed clean (no action needed)

- `go build ./...` — succeeds, zero errors, across every package.
- `go test ./...` — all suites pass: `config`, `coord`, `discovery`, `fileserver`, `identity`,
  `mesh`, `mobile`, `relay`, `service`.
- SFTP server's crypto setup itself (host key generation, `ssh.ServerConfig`, RSA 2048 host key)
  is real, not a stub — the gap is purely on the client-auth side (finding #1 above), not the
  transport crypto.
- `pkg/relay` (DERP-style fallback) and `pkg/coord` (STUN-style exchange) both have real logic
  behind them and passing tests — these aren't stubs like `pkg/tunnel`.

---

## Priority order for fixes

1. **Wire up `PublicKeyCallback` auth** in `pkg/fileserver/server.go`, using `pkg/identity` keys —
   blocking issue before any WAN exposure.
2. **Decide WireGuard: build it or drop it** — resolve the doc/code mismatch in `pkg/tunnel`
   before Phase 2 work continues, so Phase 2's NAT-traversal design is built against what's real.
3. **Fix the mutex copy** in `pkg/mobile/android.go:58` — small, mechanical, but a real
   correctness bug `go vet` caught for a reason.
