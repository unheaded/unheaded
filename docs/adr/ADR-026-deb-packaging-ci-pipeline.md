# ADR-026: .deb Packaging + Local CI/CD Pipeline — From Dev Hacks to Real Software

**Status:** Accepted
**Date:** 2026-04-03
**Deciders:** Captain, Architect, Developer, Micromanager

## Context

Unheaded currently runs as:
- Go binaries built locally with `go build` and run from `~/tmp/`
- Python apps run via `nohup python3 ... &`
- Docker containers for some services
- No package management, no versioning, no proper installation

This is fine for development but it's not how production software works. Real software:
- Installs to `/usr/local/bin/` or `/opt/unheaded/` via `apt install`
- Has systemd units that start on boot, restart on crash
- Has config files in `/etc/unheaded/`
- Has data directories in `/var/lib/unheaded/`
- Has log rotation configured
- Has proper versioning (semver)
- Can be upgraded with `apt upgrade` and rolled back with `apt install unheaded-wotan=1.2.3`

The monorepo (~415K LOC) needs to break into installable packages, each with its own .deb, Jenkinsfile, and release cycle.

## Decision

### Package Taxonomy

Break Unheaded into installable .deb packages:

| Package | Contents | Installs To |
|---------|----------|-------------|
| `unheaded-core` | pkg/ libraries, shared configs | `/opt/unheaded/lib/` |
| `unheaded-wotan` | Wotan message bus | `/opt/unheaded/bin/wotan` |
| `unheaded-daemon` | Control plane daemon | `/opt/unheaded/bin/unheaded-daemon` |
| `unheaded-timeguru` | Timeline service | `/opt/unheaded/bin/timeguru` |
| `unheaded-captain` | Strategy service | `/opt/unheaded/bin/captain` |
| `unheaded-architect` | Design service | `/opt/unheaded/bin/architect` |
| `unheaded-micromanager` | Execution service | `/opt/unheaded/bin/micromanager` |
| `unheaded-monad` | State management | `/opt/unheaded/bin/monad` |
| `unheaded-sophia` | Knowledge graph | `/opt/unheaded/bin/sophia` |
| `unheaded-dashboard` | Dashboard backend + static assets | `/opt/unheaded/bin/dashboard-backend` |
| `unheaded-kanban` | Kanban app | `/opt/unheaded/bin/kanban-app` |
| `unheaded-gateway` | nginx gateway configs | `/etc/nginx/sites-available/unheaded` |
| `unheaded-ebpf` | All eBPF programs (.o files) | `/opt/unheaded/ebpf/` |
| `unheaded-zhenai` | Zhenai web UI + RAG + runbook runner | `/opt/unheaded/zhenai/` |
| `unheaded-configs` | Default configs, systemd units | `/etc/unheaded/`, `/etc/systemd/system/` |

### Each Package Has

```
debian/
├── control          # Package metadata, dependencies
├── changelog        # Version history (dpkg-parsechangelog)
├── rules            # Build instructions (calls go build / cargo build)
├── install          # File placement mapping
├── postinst         # Post-install: systemctl daemon-reload, enable service
├── prerm            # Pre-remove: systemctl stop service
└── unheaded-*.service  # systemd unit file
```

### CI/CD Pipeline

```
Developer pushes to git
    │
    ▼
Jenkins (WEST:18080) polls or webhook triggers
    │
    ▼
Jenkinsfile stages:
    │
    ├── 1. Checkout: git clone/pull
    ├── 2. Test: go test ./... -race
    ├── 3. BPF Check: scripts/bpf-verifier-check.sh
    ├── 4. Build: go build (per-package)
    ├── 5. Package: dpkg-buildpackage -us -uc
    ├── 6. Publish: reprepro includedeb (local APT repo)
    ├── 7. Deploy (optional): ssh east 'sudo apt update && sudo apt install unheaded-wotan'
    └── 8. Verify: health check on target host
```

### Local APT Repository

WEST hosts the APT repo at port 18888:
- `reprepro` manages the repository
- nginx serves it to EAST and future hosts
- Packages are signed (GPG key per ADR-004 approved exceptions)
- EAST adds: `deb [trusted=yes] http://192.168.13.2:18888 noble main`

### Directory Layout (Installed)

```
/opt/unheaded/
├── bin/                    # Service binaries
│   ├── wotan
│   ├── timeguru
│   ├── captain
│   └── ...
├── ebpf/                   # Compiled eBPF objects
├── lib/                    # Shared libraries
├── zhenai/                 # Zhenai app + models + index
│   ├── zhen_app.py
│   ├── zhen_mcp_server.py
│   ├── models/ → /var/zhen/models
│   └── index/ → /var/zhen/index
└── runbooks/               # Operational runbooks

/etc/unheaded/
├── unheaded.yaml           # Master config
├── wotan.yaml              # Wotan topic config
├── services.yaml           # Service registry
├── certs/                  # TLS certificates
└── secrets/                # API keys (0600)

/var/lib/unheaded/
├── state/                  # Service state files
├── data/                   # Service data (SQLite, etc.)
└── logs/                   # Service logs (rotated)

/var/zhen/
├── models/                 # GGUF model files (SSD)
├── index/                  # FAISS indexes (SSD)
└── corpus/                 # Corpus files (SSD)
```

### Systemd Units

Each service gets a proper systemd unit:

```ini
# /etc/systemd/system/unheaded-wotan.service
[Unit]
Description=Wotan Message Bus — Unheaded Kingdom
After=network.target postgresql.service
Wants=postgresql.service

[Service]
Type=simple
User=unheaded
Group=unheaded
ExecStart=/opt/unheaded/bin/wotan --config /etc/unheaded/wotan.yaml
Restart=always
RestartSec=5
CapabilityBoundingSet=CAP_NET_BIND_SERVICE
NoNewPrivileges=true
ProtectSystem=strict
ProtectHome=true
ReadWritePaths=/var/lib/unheaded /var/log/unheaded

[Install]
WantedBy=multi-user.target
```

### Migration Path

1. **Phase 1 (Now):** Jenkinsfile + build scripts that produce .debs from current monorepo
2. **Phase 2:** `unheaded` system user/group, directory layout in `/opt/unheaded/`
3. **Phase 3:** Systemd units for all services, `systemctl start unheaded-wotan`
4. **Phase 4:** Local APT repo on WEST, EAST installs via `apt install`
5. **Phase 5:** Break monorepo into per-service repos (each with own Jenkinsfile)
6. **Phase 6:** Automated release pipeline — tag → build → publish → deploy

### Versioning

Semver: `MAJOR.MINOR.PATCH`
- MAJOR: Breaking protocol/API changes (wire format already frozen at v0x01)
- MINOR: New features, new services
- PATCH: Bug fixes, config changes

Changelog in `debian/changelog` follows Debian format:
```
unheaded-wotan (1.0.0-1) noble; urgency=medium

  * Initial .deb package
  * Wotan message bus with topic auto-approval
  * gRPC + HTTP dual-server

 -- Stevie Bellis <stevie@bellis.tech>  Thu, 03 Apr 2026 16:00:00 +0000
```

## Consequences

### Positive
- Real software installation via apt — professional, reproducible
- Proper process management via systemd — boot on start, restart on crash
- Version management — upgrade, downgrade, audit installed versions
- Offline distribution — EAST gets packages without internet via local APT repo
- Foundation for Yggdrasil (ADR-69420) — hardened OS ships with these .debs

### Negative
- Packaging overhead per service (~1 day initial setup for build scripts)
- Debian packaging has a learning curve (debian/ directory structure)
- Breaking monorepo is a significant refactor (Phase 5)

### Risks
- Monorepo breakup could introduce dependency hell between packages
  - Mitigate: `unheaded-core` is the shared dependency, version-pinned
- Jenkins adds maintenance overhead
  - Mitigate: Jenkins in Docker, config in git, reproducible
