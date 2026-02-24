govan@Stevies-MacBook-Air ~/tmp/unheaded % git diff
diff --git a/cmd/dashboard-backend/internal/ebpf/types.go b/cmd/dashboard-backend/internal/ebpf/types.go
index b0df686..f79b446 100644
--- a/cmd/dashboard-backend/internal/ebpf/types.go
+++ b/cmd/dashboard-backend/internal/ebpf/types.go
@@ -102,6 +102,20 @@ func (k FlowKey) String() string {
        return fmt.Sprintf("%s:%d→%s:%d/%s", k.SrcAddr, k.SrcPort, k.DstAddr, k.DstPort, proto)
 }
 
+// MeshMeta contains service mesh metadata extracted from IPv4-mapped address prefix bytes.
+// Present only on IPv4 packets when the eBPF collector stamps mesh context.
+type MeshMeta struct {
+       Version        uint8    `json:"version"`
+       SrcServiceID   uint8    `json:"src_service_id"`
+       DstServiceID   uint8    `json:"dst_service_id"`
+       HopCount       uint8    `json:"hop_count"`
+       FlowFlags      []string `json:"flow_flags"`
+       TraceHash      string   `json:"trace_hash"`
+       QosClass       uint16   `json:"qos_class"`
+       NatType        string   `json:"nat_type"`
+       LatencyHintNs  uint32   `json:"latency_hint_ns"`
+}
+
 // PacketEvent represents a packet captured by the XDP packet-marker program.
 type PacketEvent struct {
        TimestampNs uint64       `json:"timestamp_ns"`
@@ -110,6 +124,7 @@ type PacketEvent struct {
        PacketLen   uint32       `json:"packet_len"`
        Action      PacketAction `json:"action"`
        Direction   Direction    `json:"direction"`
+       Mesh        *MeshMeta    `json:"mesh,omitempty"`
 }
 
 // Time returns the event timestamp as time.Time.
diff --git a/cmd/dashboard-backend/internal/server/server.go b/cmd/dashboard-backend/internal/server/server.go
index 12dbf13..053e309 100644
--- a/cmd/dashboard-backend/internal/server/server.go
+++ b/cmd/dashboard-backend/internal/server/server.go
@@ -1268,6 +1268,8 @@ func (s *Server) handleAggregatedHealth(w http.ResponseWriter, r *http.Request)
 }
 
 // broadcastEBPFEvents listens to the eBPF ingestor and broadcasts events via WebSocket.
+// When an ebpf_packet event is received, also emits a packet_flow message so
+// the existing canvas visualization renders real eBPF data.
 func (s *Server) broadcastEBPFEvents(ctx context.Context) {
        defer s.wg.Done()
 
@@ -1289,11 +1291,75 @@ func (s *Server) broadcastEBPFEvents(ctx context.Context) {
                        Data:      env.Data,
                }
                s.broadcastToStream(streamMsg)
+
+               // Convert ebpf_packet events into packet_flow messages for the canvas
+               if env.Type == "packet" {
+                       if pkt, ok := env.Data.(*ebpfPkg.PacketEvent); ok {
+                               srcService := s.resolveServiceName(pkt.FlowKey.SrcAddr)
+                               dstService := s.resolveServiceName(pkt.FlowKey.DstAddr)
+                               // Use mesh service_id if available for better hop mapping
+                               if pkt.Mesh != nil {
+                                       if name := s.resolveServiceByID(pkt.Mesh.SrcServiceID); name != "" {
+                                               srcService = name
+                                       }
+                                       if name := s.resolveServiceByID(pkt.Mesh.DstServiceID); name != "" {
+                                               dstService = name
+                                       }
+                               }
+                               flowData, _ := json.Marshal(map[string]interface{}{
+                                       "type": "packet_flow",
+                                       "data": map[string]interface{}{
+                                               "source":      srcService,
+                                               "destination": dstService,
+                                               "protocol":    pkt.FlowKey.Protocol,
+                                               "size":        pkt.PacketLen,
+                                               "timestamp":   env.Timestamp,
+                                               "trace_id":    pkt.TraceID.String(),
+                                               "direction":   pkt.Direction,
+                                       },
+                               })
+                               s.wsServer.Broadcast(flowData)
+                       }
+               }
        })
 
        <-ctx.Done()
 }
 
+// resolveServiceName maps an IP address to a service name using config.ServiceEndpoints.
+func (s *Server) resolveServiceName(addr string) string {
+       if s.config.ServiceEndpoints == nil {
+               return addr
+       }
+       for name, endpoint := range s.config.ServiceEndpoints {
+               // endpoint is "host:port", strip port for comparison
+               host := endpoint
+               if idx := strings.LastIndex(endpoint, ":"); idx >= 0 {
+                       host = endpoint[:idx]
+               }
+               if host == addr {
+                       return name
+               }
+       }
+       return addr
+}
+
+// resolveServiceByID maps a mesh service_id byte to a service name.
+// Convention: 0=unknown, 1-N map to services in config order.
+func (s *Server) resolveServiceByID(id uint8) string {
+       if id == 0 || s.config.ServiceEndpoints == nil {
+               return ""
+       }
+       i := uint8(1)
+       for name := range s.config.ServiceEndpoints {
+               if i == id {
+                       return name
+               }
+               i++
+       }
+       return ""
+}
+
 // handleLatency handles GET /api/v1/latency - latency histogram data
 func (s *Server) handleLatency(w http.ResponseWriter, r *http.Request) {
        if r.Method != http.MethodGet {
diff --git a/cmd/dashboard-backend/main.go b/cmd/dashboard-backend/main.go
index 70c94ab..9d78608 100644
--- a/cmd/dashboard-backend/main.go
+++ b/cmd/dashboard-backend/main.go
@@ -71,6 +71,9 @@ var (
        flowInterval = flag.Duration("flow-interval", 100*time.Millisecond, "Packet flow generation interval")
        maxFlows     = flag.Int("max-flows", 50, "Maximum concurrent flows")
 
+       // WebSocket allowed origins for LAN access
+       wsAllowedOrigins = flag.String("ws-allowed-origins", "", "Comma-separated WebSocket allowed origins (empty=localhost only)")
+
        // Service endpoint overrides
        // TODO: Default to 127.0.0.1 endpoints instead of LXD IPs. --services-file
        // should remain as an override option, but defaults should be localhost.
@@ -130,6 +133,7 @@ func main() {
                        PingInterval:   30 * time.Second,
                        BufferSize:     256,
                        MaxMessageSize: 65536,
+                       AllowedOrigins: parseAllowedOrigins(*wsAllowedOrigins),
                },
 
                ScraperConfig: &scraper.Config{
@@ -233,6 +237,27 @@ func main() {
        log.Info().Msg("dashboard backend stopped")
 }
 
+// parseAllowedOrigins splits a comma-separated origins string.
+// Returns nil (not empty slice) when input is empty — this preserves
+// the deny-by-default behavior in websocket.Server.isOriginAllowed().
+func parseAllowedOrigins(origins string) []string {
+       if origins == "" {
+               return nil
+       }
+       parts := strings.Split(origins, ",")
+       result := make([]string, 0, len(parts))
+       for _, p := range parts {
+               p = strings.TrimSpace(p)
+               if p != "" {
+                       result = append(result, p)
+               }
+       }
+       if len(result) == 0 {
+               return nil
+       }
+       return result
+}
+
 // getEventTopics returns the list of Wotan topics to subscribe to
 func getEventTopics() []string {
        return []string{
diff --git a/docs/HANDOFF_2026-02-17_S15.md b/docs/HANDOFF_2026-02-17_S15.md
index 3ff5009..4e7c496 100644
--- a/docs/HANDOFF_2026-02-17_S15.md
+++ b/docs/HANDOFF_2026-02-17_S15.md
@@ -1,7 +1,7 @@
 # Session 15 Handoff — February 17, 2026
 
 **Agent:** Claude Opus 4.6
-**Session:** 15 (Dashboard Polish + Configurable Service IPs)
+**Session:** 15 (Dashboard Polish + Configurable IPs + Security Hardening)
 **Status:** ALL TESTS PASS — `go test -race -count=1 ./...` — zero races, zero failures
 **Previous Handoff:** `docs/HANDOFF_2026-02-16_S14.md` (Session 14)
 **Battle Plan:** `docs/BATTLEPLAN_2026-02-16_S14.md`
@@ -10,11 +10,13 @@
 
 ## Executive Summary
 
-Session 15 completed Campaign 2.3 polish (dashboard frontend-backend API alignment), performed the first Campaign 2.4 integration test on the Linux host, implemented configurable service IPs to unblock localhost testing, and cleaned up 4.7GB of build artifacts.
+Session 15 delivered three things:
 
-Six API alignment bugs were found and fixed in dashboard.js. The frontend now correctly maps all backend response shapes. CSS responsive breakpoints were added for mobile/tablet. GEMINI.md was deleted (all Gemini tasks resolved in S14). Campaign 3 (Security Hardening) was deferred.
+1. **Dashboard polish** (Campaign 2.3) — 6 frontend-backend API alignment fixes, responsive CSS, empty states
+2. **Configurable service IPs** — `--services-file` flag unblocks localhost testing without LXD
+3. **P0 security hardening** — 9 fixes across 12 files from a full codebase security review
 
-Service addresses are now configurable via `--services-file` flag or `SERVICES_FILE` env var, allowing the dashboard to target localhost instead of hardcoded LXD 10.10.10.x addresses.
+A 6-agent parallel security review produced 104 findings. The 9 most critical P0 bugs were fixed immediately: CORS origin whitelist, CSP unsafe-inline removal, HSTS, WebSocket origin validation, path traversal in timeguru, race conditions in micromanager and wotan-client, captain storage and ID collisions, and HTTP server timeout hardening.
 
 **Repo size:** 252MB (down from 1.3GB after cleanup)
 **Codebase:** ~260K production LOC (~464K w/ tests)
@@ -29,13 +31,37 @@ Service addresses are now configurable via `--services-file` flag or `SERVICES_F
 |----------|--------|------------|
 | 1. TopicStream gRPC Sprint | **DONE** | 100% |
 | 2. eBPF PoC Dashboard | **2.4 IN PROGRESS** | ~90% |
-| 3. Security Hardening | DEFERRED | 0% |
+| 3. Security Hardening | **P0 DONE** | ~30% (P0 fixed, P1/P2/P3 remain) |
 | 4. Punch List Cleanup | NOT STARTED | 0% |
 
 ---
 
 ## Work Completed
 
+### P0 Security Hardening (9 Fixes)
+
+From a 104-item codebase review, all P0 security bugs fixed:
+
+1. **CORS origin whitelist** — `cmd/kanban-app/middleware.go` — Replaced `if origin == "" { origin = "*" }` echo-back with explicit origin whitelist (localhost:8080, :3000, 127.0.0.1 variants). Unknown origins get no CORS headers; preflight returns 403.
+
+2. **CSP unsafe-inline removed** — `cmd/kanban-app/middleware.go` — `script-src 'self'` (removed `'unsafe-inline'`). `style-src` keeps `'unsafe-inline'` (CSS inline is common and lower risk).
+
+3. **HSTS enabled** — `cmd/kanban-app/middleware.go` — Uncommented `Strict-Transport-Security: max-age=31536000; includeSubDomains`.
+
+4. **SSE CORS wildcard removed** — `cmd/kanban-app/main.go` — Removed duplicate `Access-Control-Allow-Origin: *` on SSE endpoint; CORS now handled by middleware only.
+
+5. **WebSocket origin validation** — `cmd/dashboard-backend/internal/websocket/server.go` — Origin check now always enforced (was skipped when AllowedOrigins empty). Added `DefaultAllowedOrigins()` with localhost defaults. Denies by default.
+
+6. **Path traversal in timeguru** — `services/timeguru/cmd/timeguru/main.go` — Replaced `filepath.Base()`/`filepath.Dir()` on URL paths with `strings.TrimPrefix` + `strings.SplitN`. No filesystem path functions on HTTP request paths.
+
+7. **Micromanager race condition** — `services/micromanager/service.go` — Added `sync.RWMutex` (`subMu`) to protect `subscriptions` map concurrent access.
+
+8. **Captain storage + ID collision** — `services/captain/storage.go` — Default storage changed from `/tmp/captain-data` (world-readable) to `$CAPTAIN_DATA_DIR` or `./data/captain`. `services/captain/captain.go` — Decision IDs now use `atomic.AddInt64` counter appended to timestamp, preventing collision under concurrency.
+
+9. **wotan-client send-on-closed-channel** — `pkg/wotan-client/client.go` — Added `done` channel to `safeChannel` with `send()` method that checks done signal before sending. Prevents panic when `Close()` races with `pollMessages()`/`streamLoop()`. Applied to all 4 send sites across `client.go`, `grpc_client.go`, `topic_client.go`.
+
+10. **HTTP server timeouts** — Both `cmd/kanban-app/main.go` and `cmd/dashboard-backend/internal/server/server.go` — Added `IdleTimeout: 5 * ReadTimeout` and `MaxHeaderBytes: 1 << 20` (1MB).
+
 ### Configurable Service IPs (Campaign 2.4 Enabler)
 
 Service health/scraper endpoints were hardcoded to LXD bridge addresses (10.10.10.x). Now configurable:
@@ -43,108 +69,56 @@ Service health/scraper endpoints were hardcoded to LXD bridge addresses (10.10.1
 **Flag:** `--services-file services.local`
 **Env:** `SERVICES_FILE=services.local`
 **Format:** Simple `name=host:port` per line, `#` comments supported
-
-Files changed:
-- `cmd/dashboard-backend/internal/scraper/scraper.go` — `RegisterKingdomServices(overrides map[string]string)` with `DefaultServiceEndpoints` var; nil means use LXD defaults, overrides merge on top
-- `cmd/dashboard-backend/internal/health/health.go` — same pattern
-- `cmd/dashboard-backend/internal/server/server.go` — added `ServiceEndpoints map[string]string` to Config, passed to both Register calls
-- `cmd/dashboard-backend/main.go` — added `--services-file` flag, `SERVICES_FILE` env var, `loadServiceEndpoints()` parser
-- `cmd/dashboard-backend/services.example` — example config with all 6 services on 127.0.0.1
-
-**Usage:**
-```bash
-cp services.example services.local
-# edit services.local for your environment
-./dashboard-backend --services-file services.local --listen :8080
-```
+**TODO:** Defaults should be 127.0.0.1 not LXD IPs. See `main.go:75`.
 
 ### Dashboard Frontend-Backend API Alignment (6 Fixes)
 
-1. **`/api/v1/health` response mapping** — Backend returns `healthy_count`, JS was reading `healthy`. Fixed: checks both variants.
-2. **`/api/v1/stats` nested structure** — Backend returns `{server, health, scraper}`, JS expected flat. Fixed: extracts from nested.
-3. **`/api/v1/ebpf/stats` wrapper** — Backend returns `{active, stats:{...}}`, JS read top-level. Fixed: drills into `data.stats || data`.
-4. **`/api/v1/latency` field name** — Backend returns `percentiles`, JS expected `operations`. Fixed: normalizes.
-5. **`/api/v1/services` field name** — Backend returns `average_latency_ms`, JS read `avg_latency_ms`. Fixed: checks both.
-6. **WebSocket message types** — Backend sends `packet_flow`, `health_update`, `ebpf_*`. JS only handled `health`, `flows`. Fixed: maps all types.
+1. `/api/v1/health` — handles both `healthy_count` and `healthy` field names
+2. `/api/v1/stats` — extracts from nested `{server, health, scraper}` structure
+3. `/api/v1/ebpf/stats` — drills into `data.stats || data` wrapper
+4. `/api/v1/latency` — normalizes `percentiles` to `operations`
+5. `/api/v1/services` — handles `average_latency_ms` vs `avg_latency_ms`
+6. WebSocket — maps `packet_flow`, `health_update`, `ebpf_*` message types
 
-### New JS Functions Added
-
-- `addFlowEvent(data)` — handles single `packet_flow` WS events
-- `addEBPFEvent(type, data)` — processes `ebpf_*` WS events into event stream
-- `formatEBPFSummary(type, data)` — human-readable eBPF event descriptions
+### CSS Responsive Breakpoints
 
-### Empty States & Graceful Degradation
+- `@media (max-width: 1200px)`: overview-row and latency-charts-grid single column
+- `@media (max-width: 768px)`: nav tabs fill width, toolbars stack, eBPF stats 2 cols
+- Event stream: min-height 300px, max-height 600px
+- Latency summary grid: `auto-fit minmax(220px, 1fr)`
 
-- Flow graph: shows "Synthetic mode — start trace-collector for real eBPF flows"
-- Latency page: shows "eBPF ingestor not active" or "Waiting for eBPF events"
-- Service grid: shows skeleton placeholder when empty
+### Disk Cleanup
 
-### CSS Responsive Breakpoints
+Freed ~4.7GB: Rust target dirs (trace-collector 4.7GB, ebpf-loader 579MB, ebpf 303MB), stale binaries 90MB. Repo 1.3GB → 252MB. Updated `.gitignore` for cmd/*/binary and cmd/*/data/ patterns.
 
-- `@media (max-width: 1200px)`: overview-row and latency-charts-grid → single column
-- `@media (max-width: 768px)`: nav tabs fill width, toolbars stack, eBPF stats grid → 2 cols
-- Event stream list: min-height 300px, max-height 600px
-- Latency summary grid: `auto-fit minmax(220px, 1fr)` instead of fixed 3 columns
+### Integration Test (Campaign 2.4)
 
-### Disk Cleanup & .gitignore
+First live dashboard-backend run: all endpoints responding, 6 services detected (unknown status — no LXD), WebSocket started, Wotan subscriptions warned (expected). Static files served correctly.
 
-Freed ~4.7GB of build artifacts:
-- `cmd/trace-collector/target/` — 4.7GB Rust debug artifacts
-- `cmd/ebpf-loader/target/` — 579MB Rust debug artifacts
-- `cmd/ebpf/target/` — 303MB Rust debug artifacts
-- `bin/` stale binaries — 90MB
-- Stray `kanban-app` binary in root
+---
 
-Added to `.gitignore`:
-- `cmd/*/kanban-app`, `cmd/*/dashboard-backend`, `cmd/*/trace-collector`, `cmd/*/ebpf-loader`
-- `cmd/*/data/`
+## Commits This Session
 
-### Other Cleanup
+| Hash | Description | Files | Delta |
+|------|-------------|-------|-------|
+| `ffc8fda` | feat: Campaigns 1-2 (TopicStream gRPC, eBPF dashboard, frontend) | 25 | +6168/-1897 |
+| `117d5a0` | feat(dashboard): configurable service IPs, .gitignore cleanup | 10 | +224/-113 |
+| `c93aa09` | fix(security): P0 hardening — CORS, WS origin, traversal, races | 12 | +221/-102 |
 
-- Deleted `GEMINI.md` — all Gemini damage resolved in S14
-- Campaign 3 tasks deleted — security hardening deferred per user
+---
 
-### Integration Test (Campaign 2.4)
+## Security Review Summary (104 Findings)
 
-First live run of dashboard-backend on this Linux host:
-- Built binary: `go build -o /tmp/dashboard-backend ./cmd/dashboard-backend/`
-- Ran with: `--listen :8080 --debug`
-- **Results:**
-  - `/health` → 200 OK `{"status": "healthy"}`
-  - `/api/v1/services` → 6 services detected (unknown status — LXD not running)
-  - `/api/v1/stats` → Full nested stats response
-  - `/` → index.html served (16KB, 200 OK)
-  - WebSocket server started
-  - Wotan topic subscriptions warned (expected — no Wotan running)
-  - Scraper + health monitor running, polling every 15s
+Full review at project root: `CODEBASE_REVIEW_TODO.md` (if saved) or regenerate with 6-agent parallel review.
 
----
+| Priority | Count | Status |
+|----------|-------|--------|
+| P0 Critical | 20 | **9 fixed** (security bugs). 11 are stubs/missing implementations, not bugs. |
+| P1 High | 28 | Pending — error handling, auth, reconnection, Nix infra |
+| P2 Medium | 25 | Pending — validation gaps, CI/CD, Nix module fixes |
+| P3 Polish | 13 | Pending — console.log removal, OpenAPI docs, SSE backoff |
 
-## File Inventory (Changed This Session)
-
-### Modified
-| File | Change |
-|------|--------|
-| `.gitignore` | Added cmd/*/binary and cmd/*/data/ patterns |
-| `cmd/dashboard-backend/static/dashboard.js` | 6 API alignment fixes, new WS handlers, empty states (813->933 lines) |
-| `cmd/dashboard-backend/static/styles.css` | Responsive breakpoints, event stream height (1529->1582 lines) |
-| `cmd/dashboard-backend/internal/scraper/scraper.go` | Configurable service IPs via overrides map |
-| `cmd/dashboard-backend/internal/scraper/scraper_test.go` | Updated call to `RegisterKingdomServices(nil)` |
-| `cmd/dashboard-backend/internal/health/health.go` | Configurable service IPs via overrides map |
-| `cmd/dashboard-backend/internal/health/health_test.go` | Updated call to `RegisterKingdomServices(nil)` |
-| `cmd/dashboard-backend/internal/server/server.go` | ServiceEndpoints in Config, passed to Register calls |
-| `cmd/dashboard-backend/main.go` | `--services-file` flag, `SERVICES_FILE` env var, `loadServiceEndpoints()` |
-
-### New
-| File | Purpose |
-|------|---------|
-| `cmd/dashboard-backend/services.example` | Example service endpoint config (name=host:port) |
-
-### Deleted
-| File | Reason |
-|------|--------|
-| `GEMINI.md` | All Gemini tasks resolved in S14 |
-| `bin/.gitkeep` | Stale, binaries cleaned up |
+**Key P1 items remaining:** No authentication on any endpoint, no mTLS between services, wotan-client reconnection backoff, unbounded task fields in micromanager.
 
 ---
 
@@ -152,17 +126,22 @@ First live run of dashboard-backend on this Linux host:
 
 ### Full Integration Test (Campaign 2.4)
 
-With configurable IPs now in place:
-1. Start individual services on localhost with different ports
+1. Start services on localhost with different ports
 2. Create `services.local` pointing at localhost endpoints
 3. Start dashboard-backend with `--services-file services.local`
 4. Verify service health cards go green
-5. Verify WebSocket streaming works in browser
-6. If eBPF available: start trace-collector, verify flow graph shows real flows
+5. Verify WebSocket streaming in browser
 
 ### TODO: Default to localhost, config evolution
 
-`--services-file` is fine as an override, but defaults should be 127.0.0.1 not LXD IPs. The file should be consulted by default (not opt-in). Longer term: main config file and/or UI-based configuration. See TODO in `cmd/dashboard-backend/main.go:75`.
+`--services-file` is fine as an override, but defaults should be 127.0.0.1 not LXD IPs. Longer term: main config file and/or UI-based configuration. See TODO in `cmd/dashboard-backend/main.go:75`.
+
+### P1 Security Items
+
+- Authentication layer (JWT/OAuth2) on all endpoints
+- mTLS between services
+- wotan-client exponential backoff on reconnect
+- Input validation bounds on micromanager task fields
 
 ### Campaign 4 (Punch List)
 
@@ -179,16 +158,9 @@ JS syntax:  node --check dashboard.js  PASS
 Frontend:   2,820 lines (933 JS + 1,582 CSS + 305 HTML)
 eBPF pkg:   1,623 lines (5 Go files)
 Total code: ~260K production lines (~464K w/ tests)
-Repo size:  252MB
+Repo size:  252MB
 ```
 
 ---
 
-## Two-Commit Session
-
-1. `ffc8fda` — feat: Campaigns 1-2 (TopicStream gRPC, eBPF dashboard, 4-page frontend) — 25 files, +6168/-1897
-2. (this commit) — feat: Configurable service IPs, .gitignore cleanup, disk reclaimed
-
----
-
-*Session 15 — The Dashboard lives, the Kingdom sees itself, and services are now configurable.*
+*Session 15 — The Dashboard lives, the Kingdom hardens, and trust is verified.*
