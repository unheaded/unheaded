# ADR-020: Kanban Bug Fixes (Three Crowns Findings)

**Status:** Accepted — All 8 bugs addressed (2026-04-03)
**Date:** 2026-03-19
**Context:** Operation Three Crowns Docker phase UI review

### Fix Status (2026-04-03)
| Bug | Status | Fix |
|-----|--------|-----|
| 1 | FIXED | Wired kanban-app to PostgreSQL via WELL_* env vars in docker-compose.yml |
| 2 | ALREADY FIXED | board.js:425 already calls API.tasks.move() — was broken by Bug 1 (no DB) |
| 3 | FIXED | Added "dashboard-backend" to Wotan auto-approve allowlist (name mismatch) |
| 4 | FIXED | Added wiki link to dashboard nav bar |
| 5 | FIXED | Added DEMO_FLOWS fallback in refreshFlows() when no eBPF data |
| 6 | ALREADY FIXED | Backend readLocalDisks() already uses /proc/mounts + statfs. Docker overlay mounts were the symptom. |
| 7 | FIXED | Added memFree/memBuffers/memCached fields to memInfoResult + scraper metrics |
| 8 | FIXED | Same root cause as Bug 3 — "dashboard-backend" not in Wotan auto-approve list |

## Findings

During Three Crowns Docker phase UI verification, two categories of bugs were identified in the kanban app.

### Bug 1: PUT /api/v1/tasks returns 500 (task update failures)

**Symptom:** Repeated 500 errors on `PUT /api/v1/tasks/{id}` for both drag-drop status changes and existing task updates.

```
PUT /api/v1/tasks/debt-timeline-sync       status=500
PUT /api/v1/tasks/testing-kanban-mmwt5s74  status=500  (repeated 15+ times)
```

**Root cause (likely):** The kanban backend is running without a database connection (Docker container has no PostgreSQL wired). The `handleUpdateTask` handler tries to persist to the DB store and fails. Per `feedback_db_required_writes.md`: write operations require DB connection.

**Also noted:** `failed to publish task creation to wotan (local create persisted)` — Wotan pub/sub topic `tasks.created` not fully wired in containerized mode.

**Fix plan:**
1. Wire kanban-app container to the PostgreSQL container (add `depends_on: postgres` and env vars in docker-compose.yml)
2. Ensure `handleUpdateTask` returns proper error when no DB is available instead of 500
3. Add retry/backoff on Wotan publish failures

### Bug 2: Drag-and-drop not persisting to backend

**Symptom:** Drag-and-drop works visually in the browser but changes are lost on refresh.

**Root cause:** The frontend `handleDrop()` in `kanban/index.html` updates the local `tasks[]` array but does not make a `PUT` request to persist the status change to the backend.

**Fix plan:**
1. Add `fetch()` call in `handleDrop()` to `PUT /api/v1/tasks/{id}` with updated status
2. Only pull drag logic from commit `ce5be23` — do NOT touch current CSS/design (approved as-is during Three Crowns)
3. Handle 500 gracefully in the UI (revert card position on failure, show toast)

### Bug 3: Dashboard Wotan subscription warnings

**Symptom:** Dashboard-backend logs show `failed to stream topic: subscription pending approval` for 20+ topics.

**Root cause:** Wotan requires explicit topic approval. Dashboard subscribes to wildcard topics (`metrics.*`, `traces.*`, `health.*`, etc.) but these aren't pre-approved in the containerized Wotan instance.

**Fix plan:**
1. Auto-approve dashboard subscriptions in Wotan config (dashboard is a trusted internal service)
2. Or: pre-register topic ACLs in Wotan startup

### Bug 4: Dashboard missing wiki link

**Symptom:** No link to wiki (:20002) from main dashboard.

**Fix plan:** Add wiki link/tile to dashboard UI. Wiki service (`cmd/wiki-server/`) runs on port 20002.

### Bug 4b: Wiki enhancements

**Content flow:** GitHub wiki for `github.com/unheaded/unheaded` already pulls from `docs/`. The standalone wiki on :20002 (linked from dashboard) should pull from `~/tmp/unheaded-wiki/` — that repo is what pushes to the GitHub wiki. So the chain is: `~/tmp/unheaded-wiki/` -> GitHub wiki, and `~/tmp/unheaded-wiki/` -> standalone :20002 wiki.

**Requirements:**
- Add search functionality (full-text across all wiki pages)
- Render markdown to HTML using existing wiki CSS (md-to-html pipeline)
- Source content from `~/tmp/unheaded-wiki/` (the repo that pushes to GitHub wiki)
- Write a `scripts/sync-wiki.sh` or `tools/wiki-sync` to pull/refresh from `~/tmp/unheaded-wiki/` on demand
- Add Wiki link in dashboard nav bar where Doom link currently is (same position, same style)
- Move Doom link to footer div alongside "Unheaded Kingdom Dashboard v0.2.0 | Uptime: ... | Server: ..." — keeps Doom accessible for screen capture demos (click through dashboard ending on Doom/UPC)
- Nav bar should mirror and persist across all pages (dashboard, kanban, wiki) — consistent navigation like unheaded.org and bellis.tech
- Note: bellis.tech has nav bar drift bug that needs fixing separately
- **Gold standard UI/UX:** The dashboard at http://192.168.69.184:20000 is the approved reference design. All future UI work must match this aesthetic.

### Bug 5: Flowgraph latency and events not populating

**Symptom:** Dashboard flowgraph visualization shows UI chrome but latency data and events are empty.

**Root cause (likely):** eBPF trace collectors are not running in the Docker stack (no kernel-level tracing in containers). Dashboard expects data from Wotan topics `traces.latency`, `traces.flow`, `ebpf.flow.events` which have no publishers.

**Fix plan:**
1. Add mock/demo data fallback when no eBPF data is available (dashboard already has `demo-data.js`)
2. Ensure demo mode activates when trace topics have no publishers

## Decision

Fix all five bugs in a follow-up sprint after Three Crowns completes. Priority order: Bug 1 (500s) > Bug 2 (drag persistence) > Bug 5 (flowgraph) > Bug 4 (wiki link) > Bug 3 (Wotan subscriptions).

### Bug 6: Dashboard disk usage display is wonky

**Symptom:** Disk usage panel shows file paths instead of mount points, with misleading percentages:
```
DISK USAGE
/etc/resolv.conf     17.1%  336.5G/2.0T
/etc/unheaded/services.txt  73.5%  155.0G/210.8G
```

**Root cause:** Dashboard backend is reading disk stats from file paths rather than mount points. Should show actual filesystem mounts (e.g., `/`, `/home`, `/var`) with their usage.

**Fix plan:** Fix disk usage scraper in dashboard-backend to use `statfs` on mount points from `/proc/mounts` or `/etc/mtab`, filtering to real block devices only.

### Note: The Well (PostgreSQL) not wired to LXD/containerd phases

During Three Crowns, The Well ran as a standalone Docker container alongside LXD services. LXD containers and containerd services need `DATABASE_URL` env vars pointing to the host postgres. Without this, all write operations (task updates, config persistence) return 500. Future work: include postgres in LXD and containerd deployment scripts, or run it as a host-level systemd service that all runtimes can reach.

### Bug 7: System Resources memory bar needs layered colors

**Symptom:** Memory bar in System Resources panel shows a single color. Should display multiple layered colors to properly distinguish free/available/cached memory (similar to htop's memory bar).

**Fix plan:** Update dashboard memory visualization to show stacked segments like `free -m -h` output:
```
             total    used    free    shared  buff/cache  available
Mem:          14Gi    4.4Gi   5.1Gi   130Mi   5.2Gi       9.9Gi
```
Bar should show: used (solid), buff/cache (lighter shade), free (empty). Display available as a label. Data is already available from the backend scraper — just needs frontend rendering update.

### Future: Container flow labeling for flowgraph

Look into applying labels on container ingress/egress with container host and service name. This would allow mapping container-to-container flows in the dashboard flowgraph — e.g., "wotan@WEST -> monad@EAST" rather than just IP addresses. Could use eBPF cgroup attach points or tc filters to tag packets with container metadata at the network boundary.

### Bug 8: Service Health panel shows all services as "unhealthy" despite healthy /health endpoints

**Symptom:** Dashboard Service Health panel shows all services (micromanager, wotan, gateway, timeguru, captain, architect) as "unhealthy" with 0.0% uptime, even though `/health` endpoints return 200 OK.

**Root cause (likely):** Dashboard health scraper may be using Wotan topic `health.*` for status (which has subscription approval issues — see Bug 3) rather than directly polling `/health` endpoints. Or the health aggregation logic isn't receiving reports from services.

**Idea — Zhen AI as realm maintainer:** Kingdom services (timeguru, captain, micromanager, architect, etc.) could map into Zhen AI as MCP servers. Zhen Champion already handles config management enforcement — extend this so Zhen monitors service health, runs remedial playbook/runbook troubleshooting when services go unhealthy, and uses kingdom services as tools (e.g., micromanager for task execution, timeguru for timeline tracking, captain for strategic decisions). Think of it as Zhen maintaining the realm autonomously — the services are both the infrastructure AND the AI's toolkit. Some of this was scoped in the champion/battle plan ADRs already.

### Future: Per-service dynamic routing

Spitball — each service could potentially participate in routing via OSPF/BGP/IS-IS or similar. Service containers could advertise their own prefixes and health via routing protocols, enabling truly dynamic service mesh routing at L3 rather than relying solely on DNS/Wotan discovery. Needs more thought on overhead vs benefit.

### Future: Ceph for bare metal storage layer

Consider Ceph for asset/data storage on bare metal. Benefits: distributed, self-healing, good S.M.A.R.T. integration for disk health monitoring. Would replace or augment local ZFS pools on WEST/EAST. Far future TODO — not blocking anything now.

### Infra: Give EAST internet access via WEST NAT

EAST currently has no internet (offline P2P link only). Set WEST (192.168.69.184) as EAST's default gateway and configure NAT/masquerade on WEST so EAST can reach the internet through the P2P link. This would fix: snap installs, apt updates, LXD image pulls, and container registry access on EAST. Something like:
- EAST: `ip route add default via 192.168.13.2` (WEST's P2P address)
- WEST: `iptables -t nat -A POSTROUTING -s 192.168.13.0/24 -o <wan-iface> -j MASQUERADE` + `sysctl net.ipv4.ip_forward=1`

### Pre-public: Code provenance scan

Run the license scanning tools documented in `docs/legal/` (ScanCode, FOSSology, ORT) against the full codebase before going public. Need to verify Claude hasn't inadvertently incorporated copyrighted code, GPL-incompatible snippets, or other problematic content. SBOM audit (553 deps) was done at S78 but a fresh scan is needed given the volume of code since then.

### Pre-public: Trademark/IP review on lore and naming

Review all service names, kingdom lore, and branding for trademark/IP conflicts before going public. Likely fine on most but needs a sweep — service names (Wotan, Sophia, Monad, etc.), "The Well", "The Forge", "The Outpost", Kingdom terminology, etc.

## Files to Modify

- `cmd/kanban-app/main.go` — handleUpdateTask error handling
- `kanban/index.html` — handleDrop() persistence (drag logic only, no CSS changes)
- `docker-compose.yml` — kanban-app postgres dependency
- `services/wotan/` — topic auto-approval for internal services
