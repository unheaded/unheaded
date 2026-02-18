# Unheaded Kingdom — Quick Start Guide

**For:** Humans who want to build, run, and test the platform locally.
**Time:** ~10 minutes to first health check.

---

## Prerequisites

| Tool | Version | Check |
|------|---------|-------|
| Go | 1.24+ | `go version` |
| Docker + Compose | 20.10+ | `docker compose version` |
| curl | any | `curl --version` |
| Git | any | `git --version` |

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

### Service Port Map

| Service | Container | Port | Health Check | Role |
|---------|-----------|------|-------------|------|
| Wotan | unheaded-wotan | 5555, 8081 | `localhost:8081/health` | Message bus |
| Timeguru | unheaded-timeguru | 8082 | `localhost:8082/health` | Timeline tracking |
| Captain | unheaded-captain | 8083 | `localhost:8083/health` | Strategy & vision |
| Architect | unheaded-architect | 8084 | `localhost:8084/health` | Infrastructure design |
| Micromanager | unheaded-micromanager | 8085 | `localhost:8085/health` | Task execution |
| Monad | unheaded-monad | 8086 | `localhost:8086/health` | State management |
| Sophia | unheaded-sophia | 8087 | `localhost:8087/health` | Knowledge graph |
| Cuirass | unheaded-cuirass | 8080 | `localhost:8080/health` | Control plane |

**Optional observability stack** (Prometheus + Grafana):
```bash
docker compose --profile observability up -d
# Prometheus: localhost:9091
# Grafana:    localhost:3000 (admin/unheaded)
```

---

## 4. Verify All Services Are Healthy

```bash
# Quick health check across all services
for port in 8080 8081 8082 8083 8084 8085 8086 8087; do
  printf "localhost:%-5s → " "$port"
  curl -sf "http://localhost:$port/health" && echo "" || echo "UNREACHABLE"
done
```

**Expected:** All 8 return standard Kingdom health format:
```json
{"service":"<name>","status":"healthy","version":"<semver>","timestamp":"<RFC3339>"}
```

---

## 5. Topic Pub/Sub Smoke Test (Wotan)

Wotan is the message bus. Verify topic subscribe/publish/messages work.

> **Important:** Quote all URLs containing `?` when using zsh (e.g. `'http://...'`).

```bash
# Subscribe to a test topic
curl -s -X POST 'http://localhost:8081/api/v1/topics/test.events/subscribe' \
  -H 'Content-Type: application/json' \
  -d '{"display_name":"smoke-tester"}'
# → returns {"subscriber_id":"...","status":"subscribed",...}

# Save the subscriber_id, then publish a message
SID="<paste subscriber_id from above>"
curl -s -X POST 'http://localhost:8081/api/v1/topics/test.events/publish' \
  -H 'Content-Type: application/json' \
  -d "{\"subscriber_id\":\"$SID\",\"payload\":{\"msg\":\"hello kingdom\"}}"
# → returns {"status":"published","seq":1}

# Read messages back
curl -s 'http://localhost:8081/api/v1/topics/test.events/messages'
# → returns array of messages with seq numbers

# List all active topics
curl -s 'http://localhost:8081/api/v1/topics'
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
curl -s 'http://localhost:8082/timeline?format=json' | head -c 200
echo

# YAML
curl -s 'http://localhost:8082/timeline?format=yaml' | head -c 200
echo

# TOML
curl -s 'http://localhost:8082/timeline?format=toml' | head -c 200
echo

# Markdown
curl -s 'http://localhost:8082/timeline?format=md' | head -c 200
echo

# Kanban tasks view
curl -s 'http://localhost:8082/api/v1/timeline/tasks' | head -c 200
echo
```

### 6a. Sync & Import Endpoints

```bash
# Sync current timeline to JSON/TOML/YAML/MD files in the sync directory
curl -s -X POST 'http://localhost:8082/api/v1/timeline/sync'
# → returns {"files_written":[...],"errors":[...]}
# Note: timeline.md write may fail in Docker (mounted :ro) — this is correct

# Import a timeline from JSON (round-trip test)
curl -s 'http://localhost:8082/timeline?format=json' | \
  curl -s -X POST 'http://localhost:8082/api/v1/timeline/import?format=json' \
    -H 'Content-Type: application/json' -d @-
# → returns {"message":"timeline imported successfully","version":"..."}

# Import from TOML (round-trip test)
curl -s 'http://localhost:8082/timeline?format=toml' | \
  curl -s -X POST 'http://localhost:8082/api/v1/timeline/import?format=toml' \
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
PORT=8090 TIMEGURU_ADDR=localhost:8082 WOTAN_ADDR=localhost:5555 \
  go run ./cmd/kanban-app/...
```

Open browser: **http://localhost:8090**

### 7b. API Smoke Test (curl)

```bash
# Get timeline from Timeguru
curl -s 'http://localhost:8082/api/v1/timeline' | head -c 200
echo

# Get timeline as kanban cards
curl -s 'http://localhost:8082/api/v1/timeline/tasks' | head -c 200
echo

# Create a test task
curl -s -X POST 'http://localhost:8090/api/v1/tasks' \
  -H 'Content-Type: application/json' \
  -d '{
    "id": "smoke-test-1",
    "title": "E2E Smoke Test Task",
    "status": "backlog",
    "type": "task"
  }'
echo

# Move task to in-progress
curl -s -X PUT 'http://localhost:8090/api/v1/tasks/smoke-test-1' \
  -H 'Content-Type: application/json' \
  -d '{"status": "in_progress"}'
echo

# Verify task exists
curl -s 'http://localhost:8090/api/v1/tasks/smoke-test-1'
echo

# Delete test task
curl -s -X DELETE 'http://localhost:8090/api/v1/tasks/smoke-test-1'
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
go run ./cmd/dashboard-backend/... -listen :8088 -wotan localhost:5555
```

Open browser: **http://localhost:8088**

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
# Defaults: HTTP :8080, gRPC :9090

# Terminal 2: Timeguru (needs Wotan)
WOTAN_ADDR=localhost:8080 go run ./services/timeguru/cmd/timeguru/...
# Default: :8000

# Terminal 3: Captain
WOTAN_ADDR=localhost:8080 HTTP_ADDR=:8001 go run ./services/captain/cmd/captain/...

# Terminal 4: Kanban App (needs Timeguru + Wotan)
TIMEGURU_ADDR=localhost:8000 WOTAN_ADDR=localhost:8080 \
  go run ./cmd/kanban-app/...
```

> **Note:** Standalone Wotan defaults HTTP to :8080 (not :8081 like Docker Compose).
> Override with `--http-port 8081` to match Docker port assignments.

Default ports when running standalone (without Docker):

| Service | Default Port | Config Method |
|---------|-------------|---------------|
| Wotan HTTP | 8080 | `--http-port` flag |
| Wotan gRPC | 9090 | `--grpc-port` flag |
| Timeguru | 8000 | `PORT` env |
| Captain | 8000 | `HTTP_ADDR` env |
| Architect | 8001 | `-addr` flag |
| Micromanager | 8003 | `-port` flag |
| Monad | 8004 | `MONAD_PORT` env |
| Sophia | 8005 | `SOPHIA_LISTEN_ADDR` env |
| Cuirass | 8080 | `HTTP_ADDR` env |
| Kanban App | 8080 | `PORT` env |
| Dashboard | 8080 | `-listen` flag |

> **Note:** Several services default to 8080. Docker Compose remaps them to 8080-8087. When running standalone, override with env vars or flags.

**Docker Compose port assignments:**

| Service | Docker Port | Health Check |
|---------|------------|--------------|
| Cuirass | 8080 | `localhost:8080/health` |
| Wotan | 8081 (HTTP), 5555 (gRPC) | `localhost:8081/health` |
| Timeguru | 8082 | `localhost:8082/health` |
| Captain | 8083 | `localhost:8083/health` |
| Architect | 8084 | `localhost:8084/health` |
| Micromanager | 8085 | `localhost:8085/health` |
| Monad | 8086 | `localhost:8086/health` |
| Sophia | 8087 | `localhost:8087/health` |

---

## 11. Production Deployment (Linux + LXD)

Production uses NixOS containers on LXD — **requires a Linux host**.

```bash
# 1. Setup host (installs LXD, Nix, networking)
sudo ./scripts/setup-host.sh

# 2. Build NixOS container images
cd nix && nix build .#containers

# 3. Deploy all containers
sudo ./scripts/deploy-alpha.sh

# 4. Load eBPF programs (packet tracing)
sudo ./scripts/load-ebpf.sh
```

**Network topology (production):**

| Container | IP | Ports |
|-----------|-----|-------|
| Cuirass (control plane) | 10.10.10.5 | 8005, 9100 |
| Wotan (message bus) | 10.10.10.10 | 9090, 8080 |
| Timeguru | 10.10.10.20 | 8000, 9100 |
| Captain | 10.10.10.21 | 8001, 9100 |
| Micromanager | 10.10.10.22 | 8002, 9100 |
| Architect | 10.10.10.23 | 8003, 9100 |
| Gateway (public) | 10.10.10.100 | 80, 443 |
| Kanban | 10.10.10.200 | 8080, 9100 |
| Dashboard | 10.10.10.201 | 8081, 9100 |

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
- eBPF awakening — needs Linux + bpftool + kernel headers
- Load testing harness (target: 1000 req/s)
- Service breakout to individual repos
- Multi-node clustering

### Key Documentation
| Document | What It Covers |
|----------|---------------|
| [`CLAUDE.md`](CLAUDE.md) | Development standards, architecture, coding guidelines |
| [`docs/ARCHITECTURE.md`](docs/ARCHITECTURE.md) | 6-layer architecture deep dive |
| [`references/timeline.md`](references/timeline.md) | Living roadmap (canonical source of truth) |
| [`docs/HANDOFF_2026-02-09_S10.md`](docs/HANDOFF_2026-02-09_S10.md) | Latest session status |
| [`scripts/README.md`](scripts/README.md) | Deployment script documentation |

---

*The Knight dons the full armor. No exposed joints. No gaps.*
