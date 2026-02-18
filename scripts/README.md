# Unheaded Scripts

Automation scripts for Unheaded development and deployment.

## Available Scripts

### `init-project.sh` ✨

**Purpose:** Initialize complete Unheaded project structure with placeholder files.

**Usage:**
```bash
# From project root
./scripts/init-project.sh

# Or from anywhere
/path/to/unheaded/scripts/init-project.sh
```

**What it creates:**
- ✅ **cmd/** - 4 service binaries (Go + Rust)
  - unheaded-daemon (Go)
  - trace-collector (Rust)
  - dashboard-backend (Go)
  - kanban-app (Go)

- ✅ **services/** - 5 microservices (Go)
  - wotan (link to github.com/unheaded/wotan)
  - timeguru
  - captain
  - micromanager
  - architect

- ✅ **ebpf/** - 3 eBPF programs (Rust)
  - packet_marker.rs
  - flow_tracker.rs
  - latency_probe.rs
  - Cargo.toml

- ✅ **nix/** - NixOS containers
  - flake.nix
  - containers/ (per-service configs)
  - modules/ (hardening, networking)

- ✅ **dashboard/** - Main UI
  - index.html, CSS, JS
  - Particle animation (bellis.tech style)

- ✅ **kanban/** - Kanban app (Meta Moment)
  - index.html, CSS, JS
  - Timeline reader

- ✅ **pkg/** - Shared Go packages
  - lxd/ (LXD client)
  - state/ (State management)
  - telemetry/ (Common telemetry)
  - wotan-client/ (Wotan Go client)

- ✅ **docs/** - Documentation
  - MICROSERVICES.md
  - SECURITY.md
  - (ARCHITECTURE.md, THE_META_MOMENT.md already exist)

- ✅ **references/** - Source of truth
  - timeline.json (placeholder)
  - timeline.yaml (placeholder)
  - (timeline.md already exists)

- ✅ **.gitignore** - Ignore patterns

**Total:** 50+ directories, 30+ placeholder files

**Safe to run multiple times:** Creates directories only if they don't exist, won't overwrite existing files.

---

### `setup-host.sh` ✅ Ready

**Purpose:** Fully automated host/hypervisor setup.

**Usage:**
```bash
sudo ./scripts/setup-host.sh
```

**What it does:**
- Detects OS (Ubuntu, Debian, Fedora, Arch, etc)
- Installs dependencies (LXD, Nix, eBPF tools)
- Configures networking
- Sets up eBPF environment
- Creates systemd service
- Works on: bare metal, AWS, Azure, GCP, Oracle, VMware, QEMU

**Time:** ~5-10 minutes

---

### `deploy-alpha.sh` ⏳ To Implement

**Purpose:** Deploy complete Unheaded alpha.

**Planned features:**
- Build all services
- Load eBPF programs
- Launch LXD containers
- Configure gateway
- Start dashboard

**Status:** Skeleton exists, implementation needed

---

### `load-ebpf.sh` ⏳ To Implement

**Purpose:** Load eBPF programs onto host.

**Planned features:**
- Compile eBPF programs
- Verify with bpftool
- Load into kernel
- Verify loaded

**Status:** Skeleton exists, implementation needed

---

### `demo-kanban.sh` ⏳ To Implement

**Purpose:** Run Kanban app demo.

**Planned features:**
- Start all required services
- Open browser to Kanban app
- Show packet traces in dashboard
- Interactive demo

**Status:** Skeleton exists, implementation needed

---

## Script Status

| Script | Status | Description |
|--------|--------|-------------|
| `init-project.sh` | ✅ Ready | Project structure initialization |
| `setup-host.sh` | ✅ Ready | Host setup automation |
| `deploy-alpha.sh` | ⏳ TODO | Full alpha deployment |
| `load-ebpf.sh` | ⏳ TODO | eBPF program loading |
| `demo-kanban.sh` | ⏳ TODO | Kanban demo |

---

## Development Workflow

### First Time Setup

```bash
# 1. Initialize project structure
./scripts/init-project.sh

# 2. Review created structure
tree -L 2

# 3. Initialize Go modules for services
cd services/timeguru
go mod init github.com/unheaded/timeguru
cd ../..

# 4. Start building!
```

### Host Setup (Optional for local development)

```bash
# Setup host environment
sudo ./scripts/setup-host.sh

# This prepares:
# - LXD for containers
# - eBPF environment
# - Networking
# - Systemd services
```

### Building Services

```bash
# Use Makefile
make build          # Build all Go services
make ebpf           # Build eBPF programs
make containers     # Build NixOS containers
make all            # Build everything
```

---

## Script Conventions

All scripts follow these conventions:

### Exit Codes
- `0` - Success
- `1` - Error (with descriptive message)

### Output
- 🔵 `[INFO]` - Informational messages
- 🟢 `[SUCCESS]` - Successful operations
- 🟡 `[WARN]` - Warnings (non-fatal)
- 🔴 `[ERROR]` - Errors (fatal)

### Safety
- All scripts check prerequisites
- Fail fast on errors (`set -euo pipefail`)
- Idempotent where possible
- Never delete without confirmation

---

## Adding New Scripts

When creating new scripts:

1. **Place in `scripts/` directory**
2. **Make executable**: `chmod +x scripts/your-script.sh`
3. **Add shebang**: `#!/usr/bin/env bash`
4. **Use error handling**: `set -euo pipefail`
5. **Add logging**: Use `log_info`, `log_success`, `log_error`
6. **Document in this README**
7. **Add to Makefile** if appropriate

---

## Questions?

See main project documentation:
- [README.md](../README.md) - Project overview
- [CLAUDE.md](../CLAUDE.md) - Development standards
- [docs/ARCHITECTURE.md](../docs/ARCHITECTURE.md) - System design

---

**Created:** January 26, 2026
**Maintained by:** Unheaded team
