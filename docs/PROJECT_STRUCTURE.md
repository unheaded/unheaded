# Unheaded Kingdom - Project Structure

## The Complete Architecture

```
unheaded/                           # THE KINGDOM - Root of all infrastructure
│
├── cmd/                            # ═══════════════════════════════════════════
│   │                               # SERVICE BINARIES - Executable entry points
│   │                               # ═══════════════════════════════════════════
│   │
│   ├── unheaded-daemon/            # 🏰 THE CUIRASS - Control Plane Agent
│   │   ├── main.go                 # Entry point, HTTP server, reconciliation loop
│   │   └── internal/
│   │       ├── config/             # Configuration loading & validation
│   │       ├── state/              # Desired vs actual state management
│   │       ├── lxd/                # LXD container orchestration
│   │       └── ebpf/               # eBPF program loader interface
│   │
│   ├── unheaded-cli/               # 🧤 THE GAUNTLETS - Command Line Interface
│   │   ├── main.go                 # CLI entry point
│   │   ├── cmd/                    # Cobra command definitions
│   │   │   ├── root.go             # Root command
│   │   │   ├── container.go        # container list/create/stop/rm
│   │   │   ├── secret.go           # secret list/get/set/rotate
│   │   │   ├── deploy.go           # deploy create/status/rollback
│   │   │   ├── service.go          # service list/describe/logs
│   │   │   └── status.go           # System status overview
│   │   └── output/                 # Table, JSON, YAML formatters
│   │
│   ├── trace-collector/            # 🌑 THE WHISPERING VOID BRIDGE (Rust)
│   │   ├── Cargo.toml              # Rust dependencies
│   │   └── src/
│   │       ├── main.rs             # Entry point
│   │       ├── bpf/                # eBPF program loading
│   │       ├── collector/          # Trace aggregation
│   │       ├── events/             # Event parsing
│   │       ├── metrics/            # Prometheus exposition
│   │       ├── proto/              # gRPC definitions
│   │       └── publisher/          # Wotan integration
│   │
│   ├── dashboard-backend/          # 📊 THE CAPE - Metrics Aggregator
│   │   ├── main.go                 # Entry point
│   │   └── internal/
│   │       ├── server/             # REST API + WebSocket server
│   │       ├── scraper/            # Prometheus-compatible scraping
│   │       ├── websocket/          # Pure Go WebSocket (no gorilla)
│   │       ├── health/             # Service health monitoring
│   │       ├── events/             # Wotan event streaming
│   │       ├── metrics/            # Metrics aggregation
│   │       └── packetflow/         # eBPF trace visualization
│   │
│   ├── kanban-app/                 # 📋 THE META MOMENT - Self-Tracking
│   │   ├── main.go                 # Go backend with embedded static files
│   │   ├── middleware.go           # Logging, CORS, auth
│   │   ├── wotan.go               # Wotan client integration
│   │   └── static/                 # Vanilla HTML/CSS/JS frontend
│   │       ├── index.html          # Main HTML
│   │       ├── css/
│   │       │   ├── main.css        # Kingdom theming (dark + gold)
│   │       │   ├── board.css       # Board layout
│   │       │   └── cards.css       # Card components
│   │       └── js/
│   │           ├── app.js          # Main orchestrator
│   │           ├── board.js        # Board state management
│   │           ├── cards.js        # Card components, drag-drop
│   │           ├── api.js          # Timeguru API client
│   │           └── websocket.js    # Real-time updates
│   │
│   ├── unheaded/                   # 🎯 UNIFIED CLI (alternative)
│   │   └── main.go                 # Single binary for all operations
│   │
│   └── waf/                        # 🛡️ THE SHIELD STANDALONE (Rust)
│       └── src/
│           ├── main.rs             # WAF entry point
│           └── rules/              # Detection rules
│
├── services/                       # ═══════════════════════════════════════════
│   │                               # MICROSERVICES - Royal Court Integration
│   │                               # ═══════════════════════════════════════════
│   │
│   ├── gateway/                    # 🛡️ THE SHIELD - API Gateway
│   │   ├── cmd/                    # Entry point
│   │   ├── config/                 # Gateway configuration
│   │   ├── middleware/             # Auth, rate limit, WAF
│   │   ├── proxy/                  # Reverse proxy
│   │   └── routes/                 # Route definitions
│   │
│   ├── timeguru/                   # 🔮 THE ORACLE'S ANTRE - Timeline Service
│   │   ├── cmd/timeguru/           # Entry point
│   │   └── internal/
│   │       ├── api/                # REST endpoints
│   │       ├── parser/             # Markdown timeline parser
│   │       ├── storage/            # Timeline persistence
│   │       └── timeline/           # Timeline data structures
│   │
│   ├── captain/                    # 👑 STRATEGIC VISION - Strategy Service
│   │   └── cmd/                    # Entry point
│   │
│   ├── architect/                  # 📚 THE SAGE'S LAIR - Design Service
│   │   └── cmd/                    # Entry point
│   │
│   ├── micromanager/               # 📋 EXECUTION ENGINE - Task Service
│   │   └── cmd/                    # Entry point
│   │
│   └── wotan/                     # 🧚 THE FAE CHAMBER - Message Bus
│       └── (external: github.com/unheaded/wotan)
│
├── pkg/                            # ═══════════════════════════════════════════
│   │                               # SHARED PACKAGES - The Kingdom's Arsenal
│   │                               # ═══════════════════════════════════════════
│   │
│   │                               # ─── CORE INFRASTRUCTURE ───
│   │
│   ├── mesh/                       # ⛓️ THE HAUBERK - Service Mesh (5,914 LOC)
│   │   ├── mesh.go                 # Core mesh manager
│   │   ├── discovery.go            # Service discovery (DNS, registry, K8s)
│   │   ├── loadbalancer.go         # Load balancing algorithms
│   │   ├── circuit.go              # Circuit breaker, bulkhead, retry
│   │   ├── proxy.go                # Inbound/outbound proxy
│   │   └── config.go               # Traffic policies
│   │
│   ├── loadbalancer/               # 🏋️ THE PAULDRONS - Load Balancer (6,719 LOC)
│   │   ├── balancer.go             # Core orchestrator
│   │   ├── backend.go              # Backend pool management
│   │   ├── algorithms.go           # 12+ algorithms (Maglev, P2C, etc)
│   │   ├── health.go               # Active/passive health checks
│   │   ├── session.go              # Sticky sessions
│   │   ├── l4.go                   # TCP/UDP proxy
│   │   ├── l7.go                   # HTTP/HTTPS proxy
│   │   └── config.go               # Configuration
│   │
│   ├── waf/                        # 🛡️ THE SHIELD - Web Application Firewall
│   │   ├── detection/              # Detection engines (6,057 LOC)
│   │   │   ├── sqli.go             # SQL injection (tokenization)
│   │   │   ├── xss.go              # XSS (HTML tokenizer)
│   │   │   ├── path.go             # Path traversal
│   │   │   ├── ssrf.go             # SSRF + DNS rebinding
│   │   │   ├── bot.go              # Bot detection
│   │   │   ├── scoring.go          # Anomaly scoring
│   │   │   └── waf.go              # Main integration
│   │   ├── ip/                     # IP reputation/blocking
│   │   ├── ratelimit/              # Rate limiting
│   │   ├── response/               # Block/challenge pages
│   │   └── rules/                  # Rule definitions
│   │
│   ├── deploy/                     # ⚔️ THE SWORD - Deployment Engine
│   │   ├── deploy.go               # Main deployer
│   │   ├── pipeline/               # Pipeline orchestration (7,746 LOC)
│   │   │   ├── pipeline.go         # Pipeline management
│   │   │   ├── stage.go            # Stage execution
│   │   │   ├── strategy.go         # Rolling, canary, blue-green
│   │   │   ├── artifact.go         # Artifact tracking
│   │   │   ├── hooks.go            # Pre/post hooks
│   │   │   ├── rollback.go         # Rollback management
│   │   │   ├── healthgate.go       # Health gates
│   │   │   └── notification.go     # Slack, Teams, Discord
│   │   ├── artifact/               # Artifact storage
│   │   ├── rollback/               # Rollback logic
│   │   └── strategy/               # Strategy implementations
│   │
│   ├── runtime/                    # 📦 CONTAINER RUNTIME (6,955 LOC)
│   │   ├── runtime.go              # Core interface
│   │   ├── container.go            # OCI spec generation
│   │   ├── container_linux.go      # Linux lifecycle
│   │   ├── image.go                # Registry communication
│   │   ├── exec.go                 # Container exec
│   │   ├── logs.go                 # Log streaming
│   │   ├── cgroups.go              # cgroups v2
│   │   ├── namespace.go            # Linux namespaces
│   │   ├── volume.go               # Volume management
│   │   └── sandbox.go              # Pod sandbox
│   │
│   ├── scheduler/                  # 📅 CONTAINER SCHEDULER (5,496 LOC)
│   │   ├── scheduler.go            # Main scheduler loop
│   │   ├── queue.go                # Priority queue
│   │   ├── node.go                 # Node tracking
│   │   ├── algorithm.go            # Bin-pack, spread, balanced
│   │   ├── affinity.go             # Affinity rules
│   │   ├── preemption.go           # Preemption logic
│   │   ├── binding.go              # Workload binding
│   │   └── quota.go                # Resource quotas
│   │
│   ├── dns/                        # 🌐 DNS SERVER (4,462 LOC)
│   │   ├── server.go               # UDP + TCP server
│   │   ├── resolver.go             # Recursive resolver
│   │   ├── cache.go                # TTL-aware caching
│   │   ├── zone.go                 # Zone management
│   │   ├── records.go              # A, AAAA, SRV, TXT, etc
│   │   ├── protocol.go             # Wire protocol
│   │   └── discovery.go            # DNS-SD integration
│   │
│   │                               # ─── LOW-LEVEL LIBRARIES ───
│   │
│   ├── ebpf/                       # 🌑 eBPF LOADER (3,937 LOC) - ORIGINAL
│   │   └── loader.go               # Direct BPF syscalls, ELF parsing
│   │
│   ├── netlink/                    # 🔗 NETLINK (2,136 LOC) - ORIGINAL
│   │   └── netlink.go              # RTNetlink, XDP attachment
│   │
│   ├── logger/                     # 📝 LOGGER (1,533 LOC) - ORIGINAL
│   │   └── logger.go               # Structured logging, zero-alloc
│   │
│   ├── metrics/                    # 📊 METRICS (1,168 LOC) - ORIGINAL
│   │   └── metrics.go              # Prometheus-compatible output
│   │
│   │                               # ─── SECURITY LAYER ───
│   │
│   ├── secrets/                    # 💎 CRYSTAL GROTTO - Secrets (4,976 LOC)
│   │   ├── secrets.go              # Secrets manager
│   │   ├── encryption/             # AES-GCM, ChaCha20
│   │   ├── rotation/               # Automatic rotation
│   │   └── store/                  # Backend stores
│   │
│   ├── certs/                      # 🔐 CERTIFICATE MANAGER (3,375 LOC)
│   │   ├── certs.go                # Main manager
│   │   ├── acme/                   # Let's Encrypt
│   │   ├── ca/                     # Internal CA
│   │   ├── issue/                  # Certificate issuance
│   │   ├── rotation/               # Rotation logic
│   │   └── store/                  # Cert storage
│   │
│   ├── audit/                      # 📜 AUDIT SYSTEM (5,022 LOC)
│   │   ├── audit.go                # Main auditor
│   │   ├── logger/                 # Tamper-evident logging
│   │   ├── storage/                # Audit storage
│   │   ├── query/                  # Audit querying
│   │   └── export/                 # Export formats
│   │
│   ├── compliance/                 # 📋 COMPLIANCE ENGINE (5,861 LOC)
│   │   ├── compliance.go           # Main engine
│   │   ├── standards/              # SOC2, NIST, PCI, HIPAA, GDPR
│   │   ├── controls/               # Control definitions
│   │   ├── audit/                  # Compliance auditing
│   │   └── report/                 # Report generation
│   │
│   │                               # ─── STORAGE LAYER ───
│   │
│   ├── storage/                    # 📦 TASSETS - Storage (6,989 LOC)
│   │   ├── storage.go              # Main interface
│   │   ├── kv/                     # Key-value store
│   │   ├── object/                 # Object storage
│   │   ├── cache/                  # Caching layer
│   │   └── wal/                    # Write-ahead log
│   │
│   ├── baremetal/                  # 👢 SABATONS - Bare Metal (5,524 LOC)
│   │   ├── baremetal.go            # Main provisioner
│   │   ├── pxe/                    # PXE boot server
│   │   ├── ipmi/                   # IPMI control
│   │   ├── inventory/              # Hardware inventory
│   │   └── image/                  # OS images
│   │
│   │                               # ─── SUPPORTING PACKAGES ───
│   │
│   ├── alerting/                   # 🚨 ALERTING (4,888 LOC)
│   │   ├── alerting.go             # Main alerter
│   │   ├── rules/                  # Alert rules
│   │   ├── manager/                # Alert lifecycle
│   │   └── notify/                 # Notification channels
│   │
│   ├── health/                     # 💚 HEALTH AGGREGATOR (1,601 LOC)
│   │   └── aggregator.go           # Health check aggregation
│   │
│   ├── tracing/                    # 🔍 DISTRIBUTED TRACING (2,066 LOC)
│   │   └── collector.go            # Trace collection
│   │
│   ├── network/                    # 🌐 NETWORK POLICY (2,015 LOC)
│   │   └── policy_controller.go    # iptables, nftables integration
│   │
│   ├── state/                      # 📊 STATE RECONCILER (1,850 LOC)
│   │   └── reconciler.go           # Desired vs actual state
│   │
│   ├── nix/                        # ❄️ NIX BUILDER (1,709 LOC)
│   │   └── builder.go              # NixOS container builder
│   │
│   ├── lxd/                        # 📦 LXD CLIENT (1,648 LOC)
│   │   ├── client.go               # Mock client
│   │   └── real_client.go.dev      # Real client (dev placeholder)
│   │
│   ├── wotan-client/              # 🧚 WOTAN CLIENT (1,853 LOC)
│   │   ├── client.go               # HTTP client
│   │   ├── grpc_client.go.dev      # gRPC client (dev placeholder)
│   │   └── mock/                   # Mock for testing
│   │
│   ├── events/                     # 📨 SHARED EVENTS
│   │   ├── events.go               # Event types
│   │   └── router.go               # Event router
│   │
│   ├── container/                  # 📦 CONTAINER TYPES
│   │   ├── runtime.go              # Container runtime interface
│   │   └── runtime_test.go         # Tests
│   │
│   ├── config/                     # ⚙️ CONFIGURATION
│   │   └── merge/                  # Config merging
│   │
│   ├── http/                       # 🌐 HTTP UTILITIES
│   │
│   ├── worker/                     # 👷 WORKER POOL
│   │
│   ├── eventbus/                   # 📨 EVENT BUS
│   │
│   ├── telemetry/                  # 📡 TELEMETRY
│   │
│   └── testing/                    # 🧪 TEST UTILITIES
│       └── testdata/               # Test fixtures
│
├── ebpf/                           # ═══════════════════════════════════════════
│   │                               # RUST eBPF PROGRAMS - The Whispering Void
│   │                               # ═══════════════════════════════════════════
│   │
│   ├── common/                     # Shared types (TraceId, FlowKey)
│   │   └── src/lib.rs
│   │
│   ├── packet-marker/              # XDP program - Trace ID injection
│   │   ├── Cargo.toml
│   │   └── src/main.rs
│   │
│   ├── flow-tracker/               # TC program - Connection tracking
│   │   ├── Cargo.toml
│   │   └── src/main.rs
│   │
│   ├── latency-probe/              # Kprobe - RTT measurement
│   │   ├── Cargo.toml
│   │   └── src/main.rs
│   │
│   └── syscall-tracer/             # Raw tracepoint - Security audit
│       ├── Cargo.toml
│       └── src/main.rs
│
├── nix/                            # ═══════════════════════════════════════════
│   │                               # NIXOS CONTAINERS - Immutable Citadels
│   │                               # ═══════════════════════════════════════════
│   │
│   ├── flake.nix                   # Main flake
│   ├── containers/                 # Per-service container definitions
│   ├── modules/                    # Reusable NixOS modules
│   ├── packages/                   # Package definitions
│   └── tests/                      # NixOS tests
│
├── dashboard/                      # ═══════════════════════════════════════════
│   │                               # DASHBOARD UI - The Kingdom's Face
│   │                               # ═══════════════════════════════════════════
│   │
│   ├── index.html                  # Main entry
│   ├── css/                        # Styling
│   ├── js/                         # JavaScript
│   └── assets/
│       ├── fonts/                  # Custom fonts
│       └── images/                 # Icons, logos
│
├── kanban/                         # ═══════════════════════════════════════════
│   │                               # KANBAN UI - The Meta Moment
│   │                               # ═══════════════════════════════════════════
│   │
│   ├── index.html                  # Main entry
│   ├── css/                        # Kingdom theming
│   ├── js/                         # Board logic
│   └── assets/
│       ├── fonts/
│       └── images/
│
├── docs/                           # ═══════════════════════════════════════════
│   │                               # DOCUMENTATION - The Great Library
│   │                               # ═══════════════════════════════════════════
│   │
│   ├── FAE_CHAMBER_CONTRACTS.md    # Wotan message contracts
│   ├── RUST_COMPONENTS.md          # What must be Rust
│   ├── PROJECT_STRUCTURE.md        # This file
│   └── adr/                        # Architecture Decision Records
│
├── tests/                          # ═══════════════════════════════════════════
│   │                               # TEST SUITES
│   │                               # ═══════════════════════════════════════════
│   │
│   ├── e2e/                        # End-to-end tests
│   ├── integration/                # Integration tests
│   └── unit/                       # Unit tests
│
├── scripts/                        # Build and deployment scripts
├── build/                          # Build artifacts
├── bin/                            # Compiled binaries
├── references/                     # Reference documents
│   └── timeline.md                 # The Living Grimoire
│
├── LICENSES/                       # License files
│   └── THIRD_PARTY.md              # Apache 2.0 attribution
│
├── docker-compose.yml              # Docker orchestration
├── Dockerfile                      # Multi-stage build
├── Makefile                        # Build commands
├── go.mod                          # Go module definition
├── go.sum                          # Dependency checksums
│
├── CLAUDE.md                       # Claude Code context
├── SECURITY-AUDIT-2026-01-30.md    # Security audit
└── 2026-01-30-session-handoff.md   # Session handoff
```

---

## The Armor Mapping

### Core Armor Pieces → Code Locations

| Armor Piece | Domain | Primary Package | Status |
|-------------|--------|-----------------|--------|
| 🏰 **Cuirass** | Control Plane | `cmd/unheaded-daemon/` | 75% |
| 🛡️ **Shield** | WAF/Gateway | `pkg/waf/`, `services/gateway/` | 90% |
| ⚔️ **Sword** | Deployment | `pkg/deploy/` | 85% |
| ⛓️ **Hauberk** | Service Mesh | `pkg/mesh/` | 85% |
| 🏋️ **Pauldrons** | Load Balancer | `pkg/loadbalancer/` | 90% |
| 👀 **Vambraces** | Observability | `pkg/tracing/`, `cmd/dashboard-backend/` | 70% |
| 🧤 **Gauntlets** | CLI + API | `cmd/unheaded-cli/` | 80% |
| 📦 **Tassets** | Storage | `pkg/storage/` | 80% |
| 👢 **Sabatons** | Bare Metal | `pkg/baremetal/` | 75% |

### Arcane Hollows → Code Locations

| Hollow | Domain | Package | Status |
|--------|--------|---------|--------|
| 🌑 **Whispering Void** | eBPF | `pkg/ebpf/`, `ebpf/` (Rust) | 55% |
| 💎 **Crystal Grotto** | Secrets | `pkg/secrets/` | 90% |
| 🔮 **Oracle's Antre** | Timeline | `services/timeguru/` | 85% |
| 🧚 **Fae Chamber** | Messages | `pkg/wotan-client/`, `pkg/events/` | 95% |
| 📚 **Sage's Lair** | ADRs | `services/architect/`, `docs/adr/` | 80% |
| 🌀 **Mythic Abyss** | Telemetry | `cmd/trace-collector/` | 60% |

---

## Code Statistics

```
LANGUAGE        FILES       LINES       PERCENTAGE
──────────────────────────────────────────────────
Go              280+       127,000          79%
Rust             22         5,293           3%
Nix              25         1,540           1%
JavaScript       15         4,500           3%
HTML/CSS         20         3,000           2%
Markdown        100+       20,000          12%
──────────────────────────────────────────────────
TOTAL           462+      161,333         100%
```

---

## Dependency Philosophy

### Production Code (Ship-Ready)
```
✅ Go standard library (pkg.go.dev/std)
✅ golang.org/x/sys (syscall wrappers)
✅ Rust standard library
✅ Internal Kingdom packages (pkg/*)
```

### Development Placeholders (Must Replace)
```
⚠️ prometheus/client_golang → pkg/metrics
⚠️ rs/zerolog → pkg/logger
⚠️ vishvananda/netlink → pkg/netlink
⚠️ cilium/ebpf → pkg/ebpf
⚠️ grpc-go → std lib HTTP/2
```

Files with external deps are marked `.dev` and excluded from builds.

---

## Build Commands

```bash
# Build all
make build

# Build specific package
go build ./pkg/mesh/...
go build ./pkg/loadbalancer/...
go build ./pkg/scheduler/...

# Build binaries
go build -o bin/unheaded-daemon ./cmd/unheaded-daemon
go build -o bin/unheaded-cli ./cmd/unheaded-cli
go build -o bin/kanban-app ./cmd/kanban-app
go build -o bin/dashboard-backend ./cmd/dashboard-backend

# Run tests
make test
go test ./pkg/...

# Docker
docker compose up -d
```

---

*The Knight is Armored. The Kingdom Rises.*
