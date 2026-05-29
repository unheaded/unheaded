# Unheaded Kingdom — Quick Start Guide

**For:** Humans who want to build, run, and test the platform locally.
**Time:** ~10 minutes to first health check.

---

## Prerequisites

### Full Lab Server Setup (Fresh Ubuntu 25.x)

If starting from a bare Ubuntu install, run the bootstrap script first — it installs **everything** (16 phases: drivers, toolchains, containers, eBPF, monitoring, LLM inference):

```bash
sudo ./scripts/bootstrap-llm-lab.sh        # installs all prerequisites + more
sudo reboot                                 # AMD GPU driver needs reboot
```

### Minimum Requirements (Dev Only)

| Tool | Version | Install | Check |
|------|---------|---------|-------|
| Go | 1.25+ | [go.dev/dl](https://go.dev/dl/) | `go version` |
| Rust | nightly | `curl --proto '=https' --tlsv1.2 -sSf https://sh.rustup.rs \| sh` | `rustc --version` |
| Docker + Compose | 24+ | [docs.docker.com](https://docs.docker.com/engine/install/) | `docker compose version` |
| Node.js | 22 LTS | [nodesource](https://deb.nodesource.com/) | `node --version` |
| Claude Code | latest | `npm install -g @anthropic-ai/claude-code` | `claude --version` |
| curl | any | pre-installed | `curl --version` |
| Git + LFS | any | `apt install git git-lfs` | `git --version` |

### Additional Tools (eBPF / Production)

| Tool | Version | Install | Check |
|------|---------|---------|-------|
| bpftool | kernel-matched | `apt install bpftool` | `bpftool version` |
| clang/llvm | 15+ | `apt install clang llvm` | `clang --version` |
| libbpf-dev | — | `apt install libbpf-dev` | — |
| Nix | 2.18+ | [nixos.org/nix](https://nixos.org/nix/) | `nix --version` |
| SOPS + age | — | See [CONTRIBUTOR-GUIDE.md](./CONTRIBUTOR-GUIDE.md) | `sops --version` |
| LXD/Incus | latest | `snap install lxd` | `lxd --version` |

### LLM Lab Server Tools (GPU Inference)

| Tool | Purpose | Install | Check |
|------|---------|---------|-------|
| ROCm | AMD GPU compute | `amdgpu-install --usecase=rocm` | `rocminfo` |
| vLLM | LLM inference | `docker pull rocm/vllm:latest` | `vllm-health` |
| Ollama | Quick local LLMs | `curl -fsSL https://ollama.com/install.sh \| sh` | `ollama --version` |
| hf_transfer | Fast model DL | `pip install hf_transfer` | — |
| huggingface-cli | Model hub | `pip install huggingface-hub` | `huggingface-cli --help` |

---

## 1. Build Everything

```bash
cd ~/tmp/unheaded

# Build all Go binaries (should complete with zero errors)
go build ./...

# Or use the Makefile
make build
```

**Verify:** Zero output = success. Any errors mean dependencies are missing.

---

## 2. Run the Test Suite

```bash
# Full test suite (100% pass rate expected)
go test ./... -count=1

# With race detector (slower but catches concurrency bugs)
go test ./... -race -count=1

# Single service only
go test ./services/captain/... -v

# With coverage
go test -cover ./services/architect/...

# Benchmarks
go test -bench=. -benchmem ./pkg/waf/...
```

---

## 3. Start the Platform (Docker Compose)

```bash
# Start all 8 core services
docker compose up -d

# Watch startup progress (services take ~60s to become healthy)
docker compose ps

# Follow logs
docker compose logs -f
```

### Service Port Map — "The Doom Range" (16666-26666)

| Service | Container | Port | Health Check | Role |
|---------|-----------|------|-------------|------|
| Wotan | unheaded-wotan | 18000 (HTTP), 18001 (gRPC) | `localhost:18000/health` | Message bus |
| Timeguru | unheaded-timeguru | 19000 | `localhost:19000/health` | Timeline tracking |
| Captain | unheaded-captain | 19002 | `localhost:19002/health` | Strategy & vision |
| Architect | unheaded-architect | 19001 | `localhost:19001/health` | Infrastructure design |
| Micromanager | unheaded-micromanager | 19003 | `localhost:19003/health` | Task execution |
| Monad | unheaded-monad | 19004 | `localhost:19004/health` | State management |
| Sophia | unheaded-sophia | 19005 | `localhost:19005/health` | Knowledge graph |
| Cuirass | unheaded-cuirass | 17000 | `localhost:17000/health` | Control plane |
| Dashboard | unheaded-dashboard | 20000 | `localhost:20000/health` | Metrics + WebSocket |
| Kanban | unheaded-kanban | 20001 | `localhost:20001/health` | Self-hosting proof |

**Optional observability stack** (Prometheus + Grafana):
```bash
docker compose --profile observability up -d
# Prometheus: localhost:9091
# Grafana:    localhost:3000 (admin/unheaded)
```

---

## 4. Verify All Services Are Healthy

```bash
# Quick health check across all services (Doom Range ports)
for port in 17000 18000 19000 19001 19002 19003 19004 19005 20000 20001; do
  printf "localhost:%-5s → " "$port"
  curl -sf "http://localhost:$port/health" && echo "" || echo "UNREACHABLE"
done
```

**Expected:** All services return standard Kingdom health format:
```json
{"service":"<name>","status":"healthy","version":"<semver>","timestamp":"<RFC3339>"}
```

---

## 5. Topic Pub/Sub Smoke Test (Wotan)

Wotan is the message bus. Verify topic subscribe/publish/messages work.

> **Important:** Quote all URLs containing `?` when using zsh (e.g. `'http://...'`).

```bash
# Subscribe to a test topic
curl -s -X POST 'http://localhost:18000/api/v1/topics/test.events/subscribe' \
  -H 'Content-Type: application/json' \
  -d '{"display_name":"smoke-tester"}'
# → returns {"subscriber_id":"...","status":"subscribed",...}

# Save the subscriber_id, then publish a message
SID="<paste subscriber_id from above>"
curl -s -X POST 'http://localhost:18000/api/v1/topics/test.events/publish' \
  -H 'Content-Type: application/json' \
  -d "{\"subscriber_id\":\"$SID\",\"payload\":{\"msg\":\"hello kingdom\"}}"
# → returns {"status":"published","seq":1}

# Read messages back
curl -s 'http://localhost:18000/api/v1/topics/test.events/messages'
# → returns array of messages with seq numbers

# List all active topics
curl -s 'http://localhost:18000/api/v1/topics'
```

**Success criteria:**
- [ ] Subscribe returns subscriber_id
- [ ] Publish succeeds (requires subscriber_id in payload)
- [ ] Messages returns published messages with seq numbers
- [ ] List topics shows active topics

---

## 6. Timeline Multi-Format Smoke Test (Timeguru)

Timeguru serves the timeline in JSON, YAML, TOML, and Markdown via `?format=` query param.

```bash
# JSON (default)
curl -s 'http://localhost:19000/timeline?format=json' | head -c 200
echo

# YAML
curl -s 'http://localhost:19000/timeline?format=yaml' | head -c 200
echo

# TOML
curl -s 'http://localhost:19000/timeline?format=toml' | head -c 200
echo

# Markdown
curl -s 'http://localhost:19000/timeline?format=md' | head -c 200
echo

# Kanban tasks view
curl -s 'http://localhost:19000/api/v1/timeline/tasks' | head -c 200
echo
```

### 6a. Sync & Import Endpoints

```bash
# Sync current timeline to JSON/TOML/YAML/MD files in the sync directory
curl -s -X POST 'http://localhost:19000/api/v1/timeline/sync'
# → returns {"files_written":[...],"errors":[...]}
# Note: timeline.md write may fail in Docker (mounted :ro) — this is correct

# Import a timeline from JSON (round-trip test)
curl -s 'http://localhost:19000/timeline?format=json' | \
  curl -s -X POST 'http://localhost:19000/api/v1/timeline/import?format=json' \
    -H 'Content-Type: application/json' -d @-
# → returns {"message":"timeline imported successfully","version":"..."}

# Import from TOML (round-trip test)
curl -s 'http://localhost:19000/timeline?format=toml' | \
  curl -s -X POST 'http://localhost:19000/api/v1/timeline/import?format=toml' \
    -H 'Content-Type: application/toml' -d @-
```

**Success criteria:**
- [ ] All 4 formats return valid, non-empty content
- [ ] Sync writes 3+ files (MD may fail in Docker — expected)
- [ ] Import round-trips without data loss (version/phases/milestones match)

---

## 7. Kanban E2E Smoke Test

The Kanban app is the "meta moment" — Unheaded tracking its own development.

### 7a. Start Kanban App (separate from Docker Compose)

```bash
# Kanban-app connects to Timeguru + Wotan
PORT=20001 TIMEGURU_ADDR=localhost:19000 WOTAN_ADDR=localhost:18001 \
  go run ./cmd/kanban-app/...
```

Open browser: **http://localhost:20001**

### 7b. API Smoke Test (curl)

```bash
# Get timeline from Timeguru
curl -s 'http://localhost:19000/api/v1/timeline' | head -c 200
echo

# Get timeline as kanban cards
curl -s 'http://localhost:19000/api/v1/timeline/tasks' | head -c 200
echo

# Create a test task
curl -s -X POST 'http://localhost:20001/api/v1/tasks' \
  -H 'Content-Type: application/json' \
  -d '{
    "id": "smoke-test-1",
    "title": "E2E Smoke Test Task",
    "status": "backlog",
    "type": "task"
  }'
echo

# Move task to in-progress
curl -s -X PUT 'http://localhost:20001/api/v1/tasks/smoke-test-1' \
  -H 'Content-Type: application/json' \
  -d '{"status": "in_progress"}'
echo

# Verify task exists
curl -s 'http://localhost:20001/api/v1/tasks/smoke-test-1'
echo

# Delete test task
curl -s -X DELETE 'http://localhost:20001/api/v1/tasks/smoke-test-1'
echo
```

**Success criteria:**
- [ ] Timeline loads from Timeguru (GET returns JSON/Markdown)
- [ ] Task creates (POST returns 200/201)
- [ ] Task moves between columns (PUT returns 200)
- [ ] Task deletes cleanly (DELETE returns 200)
- [ ] Browser shows kanban board with task cards

---

```bash
# Start dashboard-backend (separate terminal)
go run ./cmd/dashboard-backend/... -listen :20000 -wotan localhost:18001
```

Open browser: **http://localhost:20000**

**Verify:**
- [ ] Service health grid populates
- [ ] Metrics display (CPU, memory, request rates)
- [ ] WebSocket indicator shows "connected"

---

## 9. Shut Down

```bash
# Stop Docker Compose services
docker compose down

# Stop with volume cleanup (removes all data)
docker compose down -v
```

---

## 10. Running Without Docker

Each service can run standalone for development:

```bash
# Terminal 1: Wotan (message bus — start first)
go run ./services/wotan/cmd/wotan/...
# Defaults: HTTP :18000, gRPC :18001

# Terminal 2: Timeguru (needs Wotan)
WOTAN_ADDR=localhost:18001 go run ./services/timeguru/cmd/timeguru/...
# Default: :19000

# Terminal 3: Captain
WOTAN_ADDR=localhost:18001 HTTP_ADDR=:19002 go run ./services/captain/cmd/captain/...

# Terminal 4: Kanban App (needs Timeguru + Wotan)
TIMEGURU_ADDR=localhost:19000 WOTAN_ADDR=localhost:18001 \
  go run ./cmd/kanban-app/...
```

> **Note:** All services use Doom Range ports (16666-26666) by default.
> gRPC transport to Wotan (port 18001) is primary; HTTP (18000) is fallback.

Default ports — "The Doom Range" (16666-26666):

| Service | Default Port | Config Method |
|---------|-------------|---------------|
| Wotan HTTP | 18000 | `--http-port` flag |
| Wotan gRPC | 18001 | `--grpc-port` flag |
| Timeguru | 19000 | `PORT` env |
| Architect | 19001 | `-addr` flag |
| Captain | 19002 | `HTTP_ADDR` env |
| Micromanager | 19003 | `-port` flag |
| Monad | 19004 | `MONAD_PORT` env |
| Sophia | 19005 | `SOPHIA_LISTEN_ADDR` env |
| Cuirass | 17000 | `HTTP_ADDR` env |
| Dashboard | 20000 | `-listen` flag |
| Kanban App | 20001 | `PORT` env |

> **Note:** All services have unique Doom Range ports. No conflicts when running
> all services simultaneously. Each supports CLI flag + env var + config file override.

---

## 11. Production Deployment (Linux + LXD)

Production uses NixOS containers on LXD — **requires a Linux host**.

```bash
# Option A: Full LLM lab server bootstrap (Ubuntu 25.x, AMD GPU, everything)
sudo ./scripts/bootstrap-llm-lab.sh
sudo reboot

# Option B: Minimal host setup (LXD, Nix, networking only)
sudo ./scripts/setup-host.sh

# Then for both options:
# 2. Build NixOS container images
cd nix && nix build .#containers

# 3. Deploy all containers
sudo ./scripts/deploy-alpha.sh

# 4. Load eBPF programs (packet tracing)
sudo ./scripts/load-ebpf.sh

# 5. (Option A only) Start LLM inference
unheaded-download-model deepseek-ai/DeepSeek-R1-Distill-Qwen-7B deepseek-r1-7b --activate
unheaded-vllm-start /models/deepseek-r1-7b
```

**Network topology (production):**

| Container | IP | Ports |
|-----------|-----|-------|
| Cuirass (control plane) | 10.10.10.5 | 17000, 17001 |
| Wotan (message bus) | 10.10.10.10 | 18000, 18001 |
| Timeguru | 10.10.10.20 | 19000 |
| Architect | 10.10.10.21 | 19001 |
| Captain | 10.10.10.22 | 19002 |
| Micromanager | 10.10.10.23 | 19003 |
| Gateway (public) | 10.10.10.100 | 21000, 21443 |
| Dashboard | 10.10.10.200 | 20000 |
| Kanban | 10.10.10.201 | 20001 |

> See `nix/flake.nix` and `nix/containers/` for full configuration.

---

## 12. Next Steps

### P0 — Ship Blockers
- **Run the smoke tests** (Sections 5-7 above) and validate all criteria pass
- **Production LXD deployment** — requires Linux host with LXD (see Section 11)

### P1 — Important
- Dashboard UI polish (responsive layout, Kingdom theming)
- Security hardening: flip `strictEgress = true` in NixOS container defs
- Review `pkg/waf/response/response.go:237` XSS fix

### P2 — Post-Alpha
- Load testing harness (target: 1000 req/s)
- Service breakout to individual repos
- Multi-node clustering

### Key Documentation
| Document | What It Covers |
|----------|---------------|
| [`CLAUDE.md`](CLAUDE.md) | Development standards, architecture, coding guidelines |
| [`docs/ARCHITECTURE.md`](docs/ARCHITECTURE.md) | 6-layer architecture deep dive |
| [`references/timeline.md`](references/timeline.md) | Living roadmap (canonical source of truth) |
| [`docs/UPC_REFERENCE_MANUAL.md`](docs/UPC_REFERENCE_MANUAL.md) | Unheaded Protocol Computer |
| [`scripts/README.md`](scripts/README.md) | Deployment script documentation |
