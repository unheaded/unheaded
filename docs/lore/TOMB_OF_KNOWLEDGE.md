# Tomb of Knowledge — Architecture & System Configuration

The Tomb of Knowledge is an air-gapped Kali Linux QEMU VM running 5 layers of
security tooling for adversarial validation of the Unheaded Kingdom platform.

---

## Architecture Overview

```
Layer 5: Dark Mirror    — External observability (Prometheus, Grafana, Loki)
Layer 4: Cerydwyn         — AI threat analysis (Ollama + Mistral-7B)
Layer 3: Grimoire       — Knowledge base + RAG search (ChromaDB + MITRE ATT&CK)
Layer 2: Lich           — 10-harness fuzz framework targeting protocol stack
Layer 1: Base System    — Hardened Kali Linux, directories, SSH, tooling
```

**Core Principle:** Air-gap isolation. Tomb has no internet access — only bridge
connectivity to WEST for scraping Kingdom metrics and receiving provisioned
artifacts via SCP.

---

## QEMU VM Configuration

### Hardware

| Setting | Value | Rationale |
|---------|-------|-----------|
| CPU | 4 cores, KVM host passthrough | Fuzz throughput |
| RAM | 8 GB | Ollama needs 6GB+, Prometheus/Grafana need the rest |
| Disk | 30 GB QCOW2 | Models, indexes, campaign results |
| NIC | virtio-net, MAC 52:54:00:13:00:06 | TAP bridge to lxdbr0 |
| Console | `-nographic -serial mon:stdio` | Headless, serial console |
| Machine | q35, KVM accel | Modern chipset + hardware virtualization |

### Boot Command

```bash
qemu-system-x86_64 \
    -machine type=q35,accel=kvm \
    -cpu host \
    -m 8G \
    -smp 4 \
    -drive file=$HOME/vms/tomb-kali.qcow2,format=qcow2,if=virtio \
    -netdev tap,id=net0,ifname=tap-tomb,script=no,downscript=no \
    -device virtio-net-pci,netdev=net0,mac=52:54:00:13:00:06 \
    -nographic \
    -serial mon:stdio \
    -pidfile /tmp/tomb-qemu.pid \
    -monitor unix:/tmp/tomb-qemu.monitor,server,nowait
```

Or via Makefile:

```bash
cd tomb && make boot
```

### Disk Setup

```bash
# Create QCOW2 disk from Kali live ISO (first time only)
qemu-img create -f qcow2 $HOME/vms/tomb-kali.qcow2 30G

# Boot from ISO to install, then boot from disk
# ISO: kali-linux-2024.4-live-amd64.iso
```

### Console / Management

- **Serial console:** Ctrl+A X to exit QEMU
- **QEMU monitor:** `socat - UNIX-CONNECT:/tmp/tomb-qemu.monitor`
- **Snapshots:** `make snapshot` (QEMU savevm)
- **Shutdown:** `make shutdown` or `ssh kali@192.168.13.6 sudo poweroff`

---

## Network Topology

```
WEST Host (192.168.13.2)
│
├── lxdbr0 (bridge)
│   │
│   ├── 192.168.13.5/30  — Bridge router IP
│   │
│   └── tap-tomb ────────── virtio-net ──── Tomb VM (192.168.13.6)
│
└── br-tomb (Phase 10 stress testing bridge)
    │
    ├── fd00:13:6::1/64  — Host ULA IPv6 (stress-cannon source)
    │
    └── tap-tomb ────────── Shield XDP/TC ──── Tomb VM
```

### IPv4 Addressing

| Host | IP | Role |
|------|----|------|
| WEST | 192.168.13.2 | Host machine, Kingdom services |
| Bridge | 192.168.13.5 | Bridge router (SLIRP mode) |
| Tomb | 192.168.13.6 | Kali VM, air-gapped |

### IPv6 Addressing (Phase 10 stress testing)

| Host | IP | Notes |
|------|----|-------|
| WEST (br-tomb) | fd00:13:6::1/64 | ULA, added manually |
| Tomb (static neighbor) | fd00:13:6::6 | MAC 52:54:00:ab:cd:ef, PERMANENT |

Tomb does NOT have global IPv6 configured. The ULA and static neighbor entry
are for stress testing only — they allow kernel-routed IPv6+Monad packets to
flow through br-tomb → tap-tomb → Shield XDP/TC.

### Air-Gap Enforcement

```bash
# On Tomb VM:
# Default route goes to bridge only, NOT to internet gateway
ip route add default via 192.168.13.5

# Firewall drops all egress except bridge subnet
iptables -A OUTPUT -d 192.168.13.0/30 -j ACCEPT
iptables -A OUTPUT -o lo -j ACCEPT
iptables -A OUTPUT -j DROP

# Validation:
ping -c1 8.8.8.8    # MUST fail
ping -c1 192.168.13.2  # MUST succeed
```

### SSH Access

```bash
ssh -i ~/.ssh/id_tomb kali@192.168.13.6
# Or via Makefile:
make console
```

---

## Layer Details

### Layer 1: Base System

**Provisioned by:** `provision.sh --phase setup-base`

- Directory structure at `/opt/tomb/` with subdirectories per layer
- Kali standard tooling (nmap, nikto, sqlmap, hydra, john, gdb, wireshark)
- Python 3.10+ virtualenv for RAG pipeline and Cerydwyn daemon
- SSH key deployment (`~/.ssh/id_tomb` → `~kali/.ssh/authorized_keys`)

### Layer 2: Lich Adversary Framework

**Provisioned by:** `provision.sh --phase deploy-lich`

10 Go fuzz harnesses targeting the Unheaded protocol stack:

| Harness | Target | Tests |
|---------|--------|-------|
| LICH-001 | Monad wire format (20-byte parse + roundtrip) | 2 |
| LICH-002 | Monad codec (varint, TLV encoding) | 5 |
| LICH-003 | CRC-16 collision (hash attacks, length extension) | 5 |
| LICH-004 | MBC parser (Monad ByteCode validation) | 2 |
| LICH-005 | eBPF kernel interface (map attrs, ELF sections) | 3 |
| LICH-006 | State machine (transitions, GOAWAY monotonicity) | 3 |
| LICH-007 | MBC execution (VM arithmetic edge cases) | 2 |
| LICH-008 | Wotan cache (ring buffer push/query/capacity) | 4 |
| LICH-009 | Flow ID collision (IPv6 flow label distribution) | 4 |
| LICH-010 | WAL integrity (write-ahead log corruption) | 5 |

**Execution:**

```bash
# Run single harness for 2 minutes with 2 parallel workers
./lich-runner.sh --time 120s --parallel 2 --harness 001

# Run all harnesses
./lich-runner.sh --time 120s --parallel 2 --all
```

**Output:** `/opt/tomb/lich/results/YYYYMMDD_HHMMSS/`
- `crashes/{rce,dos,memory-leak,info-leak,unknown}/` — Crash artifacts
- `report.md` — Campaign summary
- `lich.log` — Execution log

### Layer 3: Grimoire Knowledge Base

**Provisioned by:** `provision.sh --phase deploy-grimoire`

Semantic search engine over Unheaded architecture docs and MITRE ATT&CK data.

**Components:**
- Kingdom docs collected from repo
- MITRE ATT&CK JSON (pre-downloaded, air-gap safe)
- ChromaDB vector store with sentence-transformers embeddings
- Threat models (STRIDE analysis of Kingdom)

**Usage:**

```bash
cd /opt/tomb/grimoire
python3 rag/rag-query.py --query "How does Wotan authenticate services?"
```

### Layer 4: Cerydwyn LLM

**Provisioned by:** `provision.sh --phase deploy-cerydwyn`

Autonomous threat assessment using Ollama + Mistral-7B.

**Daemon watchers (concurrent polling):**

| Watcher | Poll Interval | Monitors |
|---------|--------------|----------|
| CrashWatcher | 30s | Lich crash directories |
| AlertWatcher | 60s | Prometheus firing alerts |
| CampaignWatcher | 120s | Completed Lich campaigns |

**Output:** `/opt/tomb/cerydwyn/suggestions/YYYYMMDD-HHMMSS-{crash,alert,campaign}-*.md`

Each suggestion includes: root cause analysis, exploitability assessment, CVSS
score, CWE classification, and mitigation recommendations.

```bash
# Start daemon
python3 cerydwyn-daemon.py &

# Dry run
python3 cerydwyn-daemon.py --dry-run
```

### Layer 5: Dark Mirror Observability

**Provisioned by:** `provision.sh --phase deploy-dark-mirror`

Docker Compose stack for external observation of Kingdom services:

| Service | Port | Memory | Purpose |
|---------|------|--------|---------|
| Prometheus | 9090 | 2 GB | Scrapes Kingdom metrics (19000-19005) |
| Grafana | 3000 | 512 MB | Dashboards (attack-metrics, kingdom-overview) |
| Loki | 3100 | 1 GB | Log aggregation |
| Promtail | 9080 | 256 MB | Log shipper (Lich logs, Cerydwyn output, syslog) |

**Prometheus scrapes Kingdom services on WEST:**

```yaml
# prometheus.yml targets:
- 192.168.13.2:19000  # timeguru
- 192.168.13.2:19001  # architect
- 192.168.13.2:19002  # captain
- 192.168.13.2:19003  # micromanager
- 192.168.13.2:19004  # monad
- 192.168.13.2:19005  # sophia
```

**Air-gapped deployment:** Docker images pre-pulled on internet host, saved to
tar, SCP'd to Tomb, loaded with `docker load`:

```bash
# On internet host:
docker compose pull
docker save -o dark-mirror-images.tar prom/prometheus:v2.50.1 grafana/grafana:10.3.3 ...

# On Tomb:
./load-images.sh --load /tmp/dark-mirror-images/
docker compose up -d
```

---

## Provisioning

### Full Deployment

```bash
cd tomb

# Full provisioning (all 6 phases in order)
./provision.sh

# Single phase
./provision.sh --phase deploy-lich
./provision.sh --phase deploy-cerydwyn

# Dry run
./provision.sh --dry-run

# Override target
./provision.sh --target 192.168.13.6
```

### Ansible Alternative

```bash
make ansible-full       # All playbooks
make ansible-check      # Dry run
make ansible-lich       # Just Lich layer
```

### Provisioning Phases

1. **setup-base** — Directories, packages, users, SSH
2. **deploy-lich** — Fuzz harnesses + runner
3. **deploy-grimoire** — Knowledge base + RAG index
4. **deploy-cerydwyn** — Ollama + daemon
5. **deploy-dark-mirror** — Docker Compose observability
6. **harden** — Firewall, seccomp, audit logging

---

## Reproducibility Checklist

### Prerequisites (WEST host)

- [ ] QEMU installed with KVM support (`qemu-system-x86_64`)
- [ ] Kali ISO at `~/isos/kali-linux-2024.4-live-amd64.iso`
- [ ] QCOW2 disk at `~/vms/tomb-kali.qcow2` (30 GB)
- [ ] SSH key pair: `~/.ssh/id_tomb` (deployed to Tomb)
- [ ] Bridge `lxdbr0` or `br-tomb` configured with tap-tomb
- [ ] Pre-staged packages in `tomb/packages/` (for air-gap)
- [ ] Pre-pulled Docker images (tar'd for air-gap)
- [ ] MITRE ATT&CK JSON pre-downloaded

### Boot & Provision

```bash
# 1. Boot VM
cd tomb && make boot
# (in another terminal)

# 2. Wait for SSH
ssh -i ~/.ssh/id_tomb kali@192.168.13.6 'echo OK'

# 3. Provision all layers
./provision.sh

# 4. Verify
make test-connectivity
make test-airgap
make test-layers
make test-services
```

### Verify Air-Gap

```bash
ssh -i ~/.ssh/id_tomb kali@192.168.13.6 '
    ping -c1 -W2 8.8.8.8 && echo "FAIL: internet reachable" || echo "OK: air-gapped"
    ping -c1 -W2 192.168.13.2 && echo "OK: host reachable" || echo "FAIL: host unreachable"
'
```

---

## Key Files

| File | Purpose |
|------|---------|
| `tomb/Makefile` | QEMU boot, provisioning, testing targets |
| `tomb/provision.sh` | Master provisioner (6 phases, SSH+SCP) |
| `tomb/lich/lich-runner.sh` | Fuzz executor with parallel jobs and reporting |
| `tomb/lich/config/lich.yaml` | Harness config, fuzz durations, severity rules |
| `tomb/lich/harnesses/lich_*.go` | 10 fuzz harnesses (Go) |
| `tomb/lich/crash-triage.sh` | Crash categorization and dedup |
| `tomb/grimoire/setup-grimoire.sh` | RAG pipeline setup + indexing |
| `tomb/grimoire/rag/rag-query.py` | Query the knowledge base |
| `tomb/grimoire/threat-models/*.md` | STRIDE threat models |
| `tomb/cerydwyn/cerydwyn-daemon.py` | LLM watcher daemon |
| `tomb/cerydwyn/Modelfile-mistral` | Ollama model customization |
| `tomb/dark-mirror/docker-compose.yml` | Observability stack |
| `tomb/dark-mirror/prometheus.yml` | Kingdom scrape targets |
| `tomb/dark-mirror/setup-dark-mirror.sh` | Stack deployment script |
| `tomb/ansible/` | Ansible playbooks (alternative to provision.sh) |

---

**Created:** 2026-02-28
**Location:** WEST bare metal (host), Tomb VM (192.168.13.6)
**Status:** Provisioning scripts complete, VM not yet deployed (pending Kali ISO + disk setup)
