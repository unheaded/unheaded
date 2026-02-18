# UNHEADED — FULL CODEBASE REVIEW TODO
**Generated:** 2026-02-17 | **Method:** 10 Parallel Review Agents (Developer Skill)
**Scope:** All canonical source — services/, cmd/, pkg/, ebpf/, nix/, frontend, CI/CD
**Codebase:** ~32K LOC Go | Rust stubs | 5K LOC Nix | JS frontend

> Trust nothing. Test everything. Every input is hostile.
> Legend: 🔴 P0 (production blocker) | 🟠 P1 (deployment blocker) | 🟡 P2 (fix before GA) | 🟢 P3 (polish)

---

## 🔴 P0 — CRITICAL: BLOCK PRODUCTION (55 items)

### Security: Network & Web Layer
- [ ] **CORS origin echo wildcard fallback** — `cmd/kanban-app/middleware.go:20-26` — `if origin == "" { origin = "*" }` echoes back attacker origin. Replace with strict whitelist + 403 for unknown origins.
- [ ] **CSP `unsafe-inline` in script-src and style-src** — `cmd/kanban-app/middleware.go:227` — Defeats entire CSP. Extract inline JS/CSS to external files; use nonces.
- [ ] **WebSocket `CheckOrigin` always returns `true`** — `cmd/dashboard-backend/internal/websocket/server.go:92-95` — Cross-site WebSocket hijack. Implement origin whitelist.
- [ ] **X-Forwarded-For trusted without proxy validation** — `cmd/kanban-app/middleware.go:178-205` — Rate limit bypass. Only trust XFF from known proxy CIDRs.
- [ ] **`/metrics` endpoint completely unprotected** — `cmd/dashboard-backend/internal/server/server.go:158` — Prometheus metrics expose internal topology to anyone. Restrict to authorized IPs or add bearer auth.
- [ ] **Prometheus label cardinality attack** — `cmd/dashboard-backend/internal/metrics/aggregator.go:241` — Unbounded label key/values → OOM. Enforce max label count + length.

### Security: Services
- [ ] **Path traversal via symlinks in captain storage** — `services/captain/storage.go:217` — `isPathWithin()` uses `filepath.Rel()`/`filepath.Abs()` which follow symlinks. Attacker creates symlink pointing to `/etc/passwd`, bypasses ID char validation. Add `os.Lstat()` + reject symlinks.
- [ ] **Path traversal via `filepath.Base/Dir()` on HTTP paths** — `services/timeguru/cmd/timeguru/main.go:71-92` — `filepath.Base()` called on untrusted URL path. Switch to `strings.Split(r.URL.Path, "/")`.
- [ ] **Concurrent map race on micromanager subscriptions** — `services/micromanager/service.go:19` — `subscriptions map[string]bool` accessed from multiple goroutines without any mutex. Data race → panic. Add `sync.RWMutex`.
- [ ] **`GetInfrastructureState` returns `*Service` pointers (mutation)** — `services/architect/core.go:231` — Caller can mutate internal state: `state.Services["x"].Port = 65535`. Deep-copy all returned Service/Node objects.
- [ ] **`GetNetworkTopology` same pointer aliasing flaw** — `services/architect/core.go:329` — `*NetworkNode` pointers returned. Same fix required.
- [ ] **`ListServices` / `ListNetworkNodes` return internal pointers** — `services/architect/core.go:205,304` — All list methods expose internal object references. Deep-copy in every return path.
- [ ] **Metadata maps completely unbounded** — `services/architect/core.go:33,62,90` — `Metadata map[string]interface{}` on Service, NetworkNode, ArchitectureDecision: no size validation. 1000 × 1MB keys = OOM. Add `MaxMetadataKeys=50`, `MaxValueLen=1000`.
- [ ] **`io.ReadAll(r.Body)` without size limit** — `services/micromanager/api.go:105,194` — Multi-GB request → OOM. Wrap: `http.MaxBytesReader(w, r.Body, 10<<20)`.
- [ ] **Fire-and-forget goroutine errors silently dropped** — `services/micromanager/api.go:157-161,250-254` — Task created (201 returned) but publish failure never surfaced. Client believes success. Use synchronous publish with retry or DLQ.
- [ ] **Broken `contains()` test helper** — `services/captain/captain_test.go:765-767` — Logic is `len(s) >= len(substr) && (s == substr || len(substr) == 0 || ...)` — always returns true for empty substr AND for any non-empty input. Every error-message assertion in the test suite is invalid. Fix to use `strings.Contains()`.
- [ ] **Ignored error on `GetDecision` call** — `services/captain/api.go:360` — `decision, _ := hs.service.GetDecision(...)` — proceeds with nil decision if storage fails. Add error check and 500 response.

### Concurrency: Races & Deadlocks
- [ ] **`StreamMessages` + `Close()` race — send on closed channel** — `pkg/wotan-client/client.go:94-103,239-267` — `Close()` closes channels while `pollMessages` goroutines are still running. Panic: "send on closed channel". Add `sync.WaitGroup` to track goroutines; only close channels after all goroutines exit.
- [ ] **Multiple mutexes in wotan-client — deadlock risk** — `pkg/wotan-client/client.go:53,57` — `mu` (subscribers) and `chanMu` (channels) with complex nesting. Consolidate to single mutex or document strict lock ordering.
- [ ] **wotan-client channel leaked on early goroutine return** — `pkg/wotan-client/client.go:257-267` — Channel stored in map; if goroutine returns before `defer close(ch)`, channel persists in map until `Close()`.
- [ ] **mock: concurrent map iteration + write** — `pkg/wotan-client/mock/mock.go:102-104,247-249` — `Close()` iterates `m.channels` while `StreamMessages` could be writing it concurrently. Panic: "concurrent map iteration and write".
- [ ] **mock: race on `closed` flag** — `pkg/wotan-client/mock/mock.go:25,97-101` — Two goroutines can both pass `if m.closed` check, both try to close channels → double-close panic. Use `sync/atomic.Bool`.
- [ ] **SSE shutdown race — close then write** — `cmd/kanban-app/main.go:238-246` — `Shutdown()` closes SSE client channels while `handleSSE()` goroutines may still be writing. Use WaitGroup to drain before closing.
- [ ] **architect `cmd/main.go:90` runtime panic** — `services/architect/cmd/main.go:90` — `wotanClient.NewClient(...)` calls interface method (undefined), not package function. Variable shadowing. Will panic at runtime.
- [ ] **micromanager goroutine leaks on shutdown** — `services/micromanager/service.go:56` — `go s.listenForAlerts(ctx)` with `context.Background()` never exits. Pass cancellable context from `main()`.
- [ ] **micromanager `context.WithTimeout(context.Background())` ignores parent** — `services/micromanager/service.go:106,145,182` — Parent context cancellation is silently ignored. Use `context.WithTimeout(parentCtx, ...)`.

### Stubs Deployed as Real Code
- [ ] **eBPF programs are NOT kernel code** — `ebpf/src/packet_marker.rs`, `flow_tracker.rs`, `latency_probe.rs` — All use `fn main() { println!(...) }`. No `#![no_std]`, no `#[xdp]`/`#[kprobe]`/`#[tracepoint]`, no `aya_ebpf` imports. Will NOT compile to `bpfel-unknown-none`. Complete rewrite with aya-ebpf.
- [ ] **`aya-ebpf` crate not declared** — `ebpf/Cargo.toml` — Only `aya = "0.12"` (userspace). Kernel programs require `aya-ebpf = "0.x"` + `aya-ebpf-macros`. Build will fail.
- [ ] **No `build.rs` for two-stage eBPF compilation** — `ebpf/` — Kernel and userspace must be compiled separately. Missing `build.rs` to orchestrate `cargo build --target=bpfel-unknown-none`.
- [ ] **`unheaded-daemon` is a 15-line stub** — `cmd/unheaded-daemon/main.go` — Prints "starting...", exits 0. No signal handling, no eBPF loading, no LXD orchestration, no state enforcement.
- [ ] **`cmd/unheaded` is a 7-line stub** — `cmd/unheaded/main.go` — Prints emoji. No cobra CLI, no subcommands, no cobra/viper integration despite go.mod declaring them.
- [ ] **`trace-collector` is a stub** — `cmd/trace-collector/src/main.rs` — Prints "starting...". No tokio runtime, no ring buffer reads, no Wotan publish, no aya usage.
- [ ] **LXD client returns fake success** — `pkg/lxd/client.go` — 13 lines. `NewClient()` returns `&Client{}, nil` with zero implementation. Every caller gets silent success for operations that do nothing.

### Nix Infrastructure: Build Blockers
- [ ] **Double Prometheus node exporter — NixOS BUILD FAIL** — `nix/modules/common.nix:145-163` AND `nix/modules/networking.nix:195-208` — Both define `services.prometheus.exporters.node`. NixOS will error or apply unpredictably. Remove from one module.
- [ ] **Cross-container `systemd requires=` — ALL services fail to start** — `nix/containers/timeguru-service.nix:63`, `captain-service.nix:50`, `micromanager-service.nix:50`, `architect-service.nix:50`, `developer-service.nix:50`, `kanban-app.nix:50`, `dashboard-app.nix:50` — `wotan.service` does not exist in those containers' systemd instances. Replace with `ExecStartPre` curl health check against Wotan IP.
- [ ] **`wotan.nix` source path `/opt/unheaded/wotan` does not exist** — `nix/packages/wotan.nix:8` — Hardcoded absolute path. Will fail on `nix build`. Use `${self}/cmd/wotan` or correct relative path.
- [ ] **`wotan.nix` redefines package — conflicts with flake overlay** — `nix/containers/wotan.nix:117-132` — Package redefined both in `packages/wotan.nix` and in container config. Remove duplicate.
- [ ] **Nix `deploy` target does nothing** — `nix/flake.nix:204-225` AND `nix/Makefile:107-109` — Only runs `nix build`. No `lxc launch`, no network bridge, no container start. Implement actual deployment.
- [ ] **`vendorHash = null` on all packages** — `nix/packages/*.nix` — Every `buildGoModule` re-downloads Go deps on every build. Non-reproducible. Compute and pin hashes.
- [ ] **sysctl rules don't apply in LXD containers** — `nix/modules/hardening.nix:196-214` — `boot.kernel.sysctl` sets host-level params. LXD containers have isolated namespaces. Most settings silently no-op.
- [ ] **OUTPUT firewall rule allows all outbound** — `nix/modules/networking.nix:144` — `iptables -A OUTPUT -j ACCEPT` after restrictive rules defeats all isolation. Scope to: DNS(53), NTP(123), Wotan(9090), HTTPS(443).
- [ ] **Log forwarding commented out** — `nix/modules/common.nix:132-133` — All logs go to local files only. Container crash = logs lost. Uncomment rsyslog forwarding to aggregator.
- [ ] **`WorkingDirectory=/opt/unheaded/references` doesn't exist** — `nix/containers/timeguru-service.nix:73,77` — Path hardcoded; never created in `writablePaths` or activation. Service fails to start.
- [ ] **Audit rules are RHEL-specific** — `nix/modules/hardening.nix:221-232` — References `/etc/sysconfig/network` (RHEL path). NixOS uses `/etc/nixos/`. Rules never trigger.
- [ ] **Makefile `$$c` is wrong Make syntax** — `nix/Makefile:168` — `lxc exec unheaded-$$c` expands to `$c` (undefined). Fix: `lxc exec unheaded-$${c}`.
- [ ] **Integration tests are 90% stubs** — `nix/tests/container_test.go:226-344` — 14 of 18 integration tests contain only `t.Logf()` and TODO comments. Zero security assertions run.
- [ ] **nixpkgs input not pinned by hash** — `nix/flake.nix:20` — `github:NixOS/nixpkgs/nixos-unstable` (no `?rev=`). Reproducibility broken. Every `nix flake update` is a potential breaking change.

### CI/CD: Supply Chain
- [ ] **`gosec@master` — unpinned floating action** — `.github/workflows/ci.yml:148` — `securego/gosec@master` is supply chain risk. Pin to `securego/gosec@v2.20.0` or full SHA.
- [ ] **`gosec -no-fail` — security failures don't block CI** — `.github/workflows/ci.yml:150` — `args: '-no-fail ...'` means critical findings ship. Remove `-no-fail`; enforce `-fail`.
- [ ] **No secrets scanning in CI** — `.github/workflows/ci.yml` — No truffleHog/detect-secrets step. Credentials can be committed undetected. Add `trufflesecurity/trufflehog@v3` on every push.
- [ ] **No container image CVE scanning** — `.github/workflows/docker.yml` — Images pushed to GHCR without vulnerability scanning. Add `aquasecurity/trivy-action` post push.

---

## 🟠 P1 — HIGH: BLOCKS DEPLOYMENT (45 items)

### services/captain
- [ ] **Server startup error silently ignored** — `api.go:120-122` — `ListenAndServe` error not logged or surfaced. Add `log.Fatal()` or channel signal.
- [ ] **Decision ID collision under concurrency** — `captain.go:259` — `fmt.Sprintf("decision_%d", now.UnixNano())` collides at same nanosecond. Replace with `uuid.New()` or atomic counter + timestamp.
- [ ] **Silent data loss on startup** — `storage.go:42-46` — `loadAll()` error returns empty cache with `nil` error. Service starts with no data. Return error or panic.
- [ ] **`pathGoesUp()` always returns `false`** — `storage.go:295` — `path[0:1] == ".."` is 1 char vs 2-char check. Should be `path[:2]`. Path traversal check is broken.
- [ ] **PATCH body has no size limit** — `api.go:350` — `json.NewDecoder(r.Body)` with no `io.LimitReader`. Only POST has 10KB limit. Add limit to PATCH too.
- [ ] **`Content-Type` exact match rejects `charset=utf-8`** — `api.go:285` — `ct != "application/json"` rejects valid `application/json; charset=utf-8`. Use `mime.ParseMediaType()`.
- [ ] **`MaxBytesHandler` vs `LimitReader` mismatch** — `api.go:101` — 10MB global limit but handlers use 10KB `LimitReader`. Inconsistent. Pick one and document it.
- [ ] **Concurrent test misses write-write and close-during-write** — `captain_test.go:654-708` — Only tests concurrent reads. Add: Close() during LogDecision, multiple Close() races, UpdateDecision during GetDecision.
- [ ] **No config validation at startup** — `main.go:30-32` — Port not validated (numeric, 1-65535), path not validated. Late-binding errors confuse operators.
- [ ] **`listenForAlerts` takes concrete `*Client` not interface** — `main.go:109` — Hard dependency on concrete type. Replace with the `WotanCommunicator` interface already defined.

### services/timeguru
- [ ] **Sensitive infrastructure data in logs** — `cmd/timeguru/main.go:39-41` — DB path + Wotan addr logged at startup. Redact or move to debug level.
- [ ] **No request body size limit on milestone handlers** — `internal/api/handlers.go:125-138` — `json.NewDecoder(r.Body)` unbounded. Add `io.LimitReader`.
- [ ] **Error messages expose internal details to client** — `internal/api/handlers.go:215-233` — `errResp.Details = err.Error()` sends raw Go errors to clients. Return generic message; log detail server-side.
- [ ] **Mock store not thread-safe** — `internal/api/handlers_test.go:24-26` — `getCalls`, `saveCalls` incremented without sync. Fails `-race` detector. Add `sync.Mutex`.
- [ ] **JSON encoding errors silently discarded** — `internal/api/handlers.go:208-212,229-233` — `_ = err` after `json.NewEncoder(w).Encode()`. Log errors; can't send new status code after `WriteHeader` but at least log it.
- [ ] **Audit history INSERT failure silently swallowed** — `internal/storage/storage.go:268-273` — `_ = err` on history write. Audit trail corruption with no alert.
- [ ] **`filepath.Join()` used for string concatenation** — `internal/storage/storage_test.go:469` — `filepath.Join("milestone-", string(rune(i)))` is wrong. Use `fmt.Sprintf("milestone-%d", i)`.
- [ ] **`os.MkdirAll` with `0755` on sensitive DB dir** — `internal/storage/storage.go:49-52` — DB directory world-readable. Change to `0700`.
- [ ] **`panic()` in constructor** — `internal/api/handlers.go:47` — `panic("store cannot be nil")`. Replace with returned error.
- [ ] **Content-Type header set after `WriteHeader`** — `internal/api/handlers.go:204-205` — `w.Header().Set()` after `w.WriteHeader()` is a no-op. Move before.
- [ ] **No HTTP method validation on any endpoint** — `internal/api/handlers.go:91-105,108-122` — `HandleFunc` accepts all methods. Add explicit `r.Method` checks.

### services/architect
- [ ] **All `Validate()` methods missing enum checks** — `core.go:29-30,59,87` — `Type`, `Status` on Service/NetworkNode/Decision accept any string. Validate against defined constants.
- [ ] **`CIDR` field never validated** — `core.go:60` — `NetworkNode.CIDR` accepts `"not-a-cidr"`. Add `net.ParseCIDR()` validation.
- [ ] **`AddService/AddNetworkNode/LogDecision` mutate caller's struct** — `core.go:160-162,259-260,356-357` — `service.CreatedAt = time.Now()` on caller's pointer. Unexpected side effect. Copy struct before mutation.
- [ ] **Snapshot isolation test has false negative** — `core_test.go:645-666` — Test only checks map structure, not deep field mutation. Add: `state.Services["id"].Port = 9999; assert original port unchanged`.
- [ ] **Wrong HTTP status codes for all errors** — `handlers.go:159` — Returns 400 for infrastructure errors. Map: validation → 400, not-found → 404, infra failure → 500, timeout → 504.
- [ ] **`writeError()` and `writeSuccess()` discard `json.Encode` error** — `handlers.go:54,72` — Silent truncated response to client. Log error; add metrics counter.
- [ ] **Error messages expose internal implementation** — `handlers.go:97,128` — Raw error strings sent to client. Sanitize before sending.
- [ ] **No request body size limit on any handler** — `handlers.go:149,210,246` — `json.NewDecoder(r.Body)` unbounded. Add `io.LimitReader(r.Body, 1<<20)`.
- [ ] **All methods ignore cancelled context** — `core.go:213,383` — No `ctx.Err()` check before acquiring mutex. Cancelled context still blocks. Add: `select { case <-ctx.Done(): return ctx.Err() }` before locks.

### services/micromanager
- [ ] **Task fields unbounded** — `task.go:23,31` — `Description`, `Tags []string` no max length. Add: `MaxDesc=10000`, `MaxTags=50`, `MaxTagLen=100`. Validate in `Validate()`.
- [ ] **`alertListener` channel created but never consumed** — `service.go:30` — Buffer fills up; sender eventually blocks. Either consume it or remove it.
- [ ] **Wotan publish failure silently ignored** — `service.go:109-115,148-154,185-191` — Events lost permanently when Wotan is down. Add retry with exponential backoff + dead-letter logging.
- [ ] **Task ID regex validation missing** — `api.go:177` — `strings.TrimPrefix()` on URL path, no character/format validation. Add: `regexp.MustCompile("^task-\\d+-\\d+$")`.
- [ ] **No rate limiting on any API endpoint** — `api.go` — Zero rate limiting. Flood attack trivial. Add token bucket middleware.
- [ ] **`float64` cast to `int` without bounds check** — `api.go:227` — JSON number parsed as float64, cast to int. `9999999999.9` → undefined. Check `priority >= 1 && priority <= 5` before cast.
- [ ] **DueDate field in request but never assigned** — `api.go:141-148` — `TaskRequest.DueDate` parsed but `task.DueDate` never set. Either implement or remove.
- [ ] **No `DELETE /tasks/{id}` or `GET /tasks/{id}` endpoints** — `store.go:80-95` — `Delete()` and `Get()` methods exist but no HTTP handlers. Add routes.

### cmd/kanban-app
- [ ] **JSON decode error exposes internal detail** — `main.go:328-330,371-373` — `fmt.Sprintf("Invalid JSON: %v", err)` in response. Return generic "invalid request body"; log detail server-side.
- [ ] **No authentication on any endpoint** — `main.go:192-194` — All endpoints world-accessible. Add JWT/API key middleware before all `/api/v1/` routes.
- [ ] **Task `Type` and `Owner` fields unbounded and unvalidated** — `main.go:441-474` — `validateTaskInput()` limits `Title` and `Description` but not `Type` or `Owner`. Add length limits + `Type` enum check.
- [ ] **HTTP server missing `IdleTimeout` and `MaxHeaderBytes`** — `main.go:219-224` — Slowloris attack vector. Add `IdleTimeout: 60 * time.Second`, `MaxHeaderBytes: 1 << 20`.
- [ ] **SSE broadcast drops silently with no logging** — `main.go:559-565` — `default:` case drops message with no log, no metric. Add: `log.Warn().Msg("SSE message dropped")` + prometheus counter.
- [ ] **Fallback `http.Dir("../../kanban")` relative path** — `main.go:200` — Path traversal if working directory unexpected. Use absolute path or embedded FS only.

### cmd/dashboard-backend
- [ ] **No CORS headers on HTTP endpoints** — `internal/server/server.go:295-322` — `handleHealth`, `handleReady`, `handleMetricsQuery`, `handleFlows` set no CORS headers. Browsers will block cross-origin requests.
- [ ] **No WebSocket message size limit** — `internal/websocket/server.go:89-91` — `ReadBufferSize` is a hint, not a limit. Add `conn.SetReadLimit(1 << 20)` after upgrade.
- [ ] **Data race on `generator.counter`** — `internal/packetflow/generator.go:121` — `g.counter++` without mutex or atomic. Use `atomic.AddInt64(&g.counter, 1)`.
- [ ] **Hardcoded internal IPs in packet flow data** — `internal/packetflow/generator.go:137,240-249` — `10.10.10.10`, `10.10.10.20-23`, `10.10.10.100` in broadcast. Full topology exposed to anyone with WebSocket access. Make configurable via env.
- [ ] **Zero tests for `internal/server/server.go`** — No test file exists. All handlers untested. Add `server_test.go` covering: health, ready, metrics query, flows, CORS, auth, error paths.
- [ ] **Unbounded metrics query results** — `internal/metrics/aggregator.go:284-310` — `QueryMetrics()` returns all matching points with no pagination. Add `limit` + `offset` params.
- [ ] **No authentication on any endpoint** — All handlers — Same issue as kanban-app. All endpoints world-accessible.

### pkg/wotan-client
- [ ] **`ErrNotConnected` is dead code** — `client.go:19` — Defined but never returned. Either use it in connection check or remove.
- [ ] **No exponential backoff on reconnect** — `client.go:282-284` — `continue` with 500ms sleep on every error. Hammers recovering server. Implement: `min(baseDelay * 2^attempt, maxDelay)` + jitter.
- [ ] **No circuit breaker** — `client.go:277` — Infinite retry loop. Add half-open/open states after N consecutive failures.
- [ ] **HTTP client timeout not coordinated with context** — `client.go:69,282` — 30s HTTP timeout may conflict with caller's context deadline. Pass context to all HTTP calls.
- [ ] **Topic name not URL-encoded** — `client.go:122` — `fmt.Sprintf(".../topics/%s/subscribe", topic)` — topic with `/` or `?` breaks URL. Use `url.PathEscape(topic)`.
- [ ] **Response body not size-limited** — `client.go:302` — `io.ReadAll(resp.Body)` — malicious server sends 1GB body. Add `io.LimitReader(resp.Body, 1<<20)`.
- [ ] **mock: streaming goroutine not tracked on `Close()`** — `mock/mock.go:252-260` — Goroutine may access deleted map after `Close()`. Add `WaitGroup`.

### Nix Infrastructure
- [ ] **No TLS/mTLS between any services** — All containers — All inter-service communication is cleartext. Container escape = full traffic visibility. Implement service certificates.
- [ ] **No gateway container defined** — `nix/flake.nix` — Kanban/Dashboard containers reference `10.10.10.100` (gateway) but no NixOS config for it. Apps unreachable externally.
- [ ] **Wotan ring buffer has no persistence or crash recovery** — `nix/containers/wotan.nix:28-30` — Writable path defined but no disk format, no WAL, no replay. Messages lost on restart.
- [ ] **No DNS service discovery** — All containers — Services hard-code IPs (`10.10.10.10`, etc). Zero service agility. Add consul/coredns or use `services.resolved` with split DNS.
- [ ] **No IPv6 configured** — `nix/modules/networking.nix:70-84` — Sysctl allows IPv6 but no `ipv6.addresses` in `interfaces.eth0`. IPv6 sockets hang. Add dual-stack config.
- [ ] **No DNSSEC validation** — `nix/modules/networking.nix:83-84` — Plaintext nameservers only. Add `services.resolved.dnssec = "true"`.
- [ ] **`GOGC`/`GOMEMLIMIT` global defaults wrong for containers** — `nix/modules/common.nix:194-196` — `GOMEMLIMIT=512MiB` for all, but wotan has `MemoryMax=1G`. Override per container.
- [ ] **ExecStartPost health check races service startup** — All service containers — 2s `sleep` in `ExecStartPost` is too short for slow starts. Use retry loop: `for i in 1 2 3 4 5; do curl ... && break || sleep $i; done`.

### CI/CD
- [ ] **Go dependencies 2+ years out of date** — `go.mod:40-44` — `golang.org/x/net v0.20.0`, `golang.org/x/sys v0.16.0`, etc. (Jan 2024). Run `go get -u ./... && go mod tidy`.
- [ ] **go.mod declares `go 1.21` but CI uses `go 1.22`** — `go.mod:3` vs `ci.yml:10` — Version mismatch. Update `go.mod` to `go 1.22`.
- [ ] **Coverage not enforced — `fail_ci_if_error: false`** — `ci.yml:50-55` — Coverage can drop to 0% without failing build. Set `fail_ci_if_error: true` and minimum threshold.

### Frontend
- [ ] **Missing `metrics.js` file** — `dashboard/index.html:34` — `<script src="/js/metrics.js">` → 404. Dashboard fails to load. Create file or remove reference.
- [ ] **`fetch()` has no timeout** — `kanban/js/timeline-reader.js:159-174` — `fetchTasks()` uses bare `fetch()`. Add `AbortController` with 10s timeout.

---

## 🟡 P2 — MEDIUM: FIX BEFORE GA (35 items)

### services/captain
- [ ] **Validation duplicated between `Validate()` and `LogDecision()`** — `captain.go:88-96,228-248` — Same checks in two places. Move all validation to `Validate()`.
- [ ] **Nil context accepted (non-idiomatic)** — `captain.go:169-170` — 5 methods convert `nil` context to `Background()`. Context should never be nil per Go convention. Remove nil handling; callers must pass valid context.
- [ ] **Status constants inline instead of typed consts** — `captain.go:371-379` — Valid statuses defined in function body. Extract to `const` block.
- [ ] **Storage 1MB limit undocumented** — `storage.go:78,132` — Checked twice without explanation. Document in code and return helpful error: `"decision content exceeds 1MB limit"`.
- [ ] **Captain request IDs use per-instance counter** — `api.go:382` — Not globally unique; useless for distributed tracing. Use UUID or propagate `X-Request-ID` from incoming request.

### services/timeguru
- [ ] **Goroutine potentially hangs on `StreamMessages`** — `cmd/timeguru/main.go:193-220` — No read timeout on message channel. Add `select` with `time.After` or context deadline.
- [ ] **Database connection not health-checked between requests** — `internal/storage/storage.go:42-74` — `Ping()` only at startup. Add per-request context check or connection pool health validation.
- [ ] **`TaskStatus` is an untyped string** — `internal/timeline/timeline.go:44-80` — Use `type MilestoneStatus string` with iota-like const block to enable compile-time safety.
- [ ] **Double fetch after update** — `internal/api/handlers.go:172-182` — Retrieves timeline again after update. Return updated object directly from store.
- [ ] **Nil pointer possible in phase/milestone validation loop** — `internal/timeline/timeline.go:153-157,160-164` — Array can contain nil pointers. Add nil check before `Validate()` call.

### services/architect
- [ ] **Inconsistent error types** — `core.go:272` — Some methods return sentinel errors (`ErrEmptyServiceID`), others return `errors.New()` inline. Standardize; callers using `errors.Is()` will miss inline ones.
- [ ] **Port 0 allowed in `Service.Validate()`** — `core.go:49` — `Port < 0 || Port > 65535` permits 0. If 0 means "unset", add explicit check.
- [ ] **HTTP 400 for all errors** — `handlers.go:159` — Already in P1 but includes: add `errors.Is()` checks and return 404 for not-found, 500 for internal errors, 504 for timeouts.

### services/micromanager
- [ ] **Wotan `Subscribe` error logged but service continues** — `service.go:42-46` — Service reports healthy but has no event stream. Return error from `Start()` and fail fast.
- [ ] **Subscription status not validated after subscribe** — `service.go:48-51` — `sub.Status` logged but never checked. Verify `== "approved"` before using subscription.
- [ ] **No idempotency key on `CreateTask`** — `api.go` — Client retries create duplicate tasks. Add idempotency key header or check.
- [ ] **`CompletedAt` set but no `ListByCompletionDate` query** — `store.go` — Field stored but no query method. Either add query or remove field.
- [ ] **`Owner`/`Assignee` fields not format-validated** — `task.go:29,62` — Accept any string. Add regex for username/email format.

### cmd/kanban-app
- [ ] **Deprecated task storage dual code path active** — `main.go:54` — Dual code paths for deprecated vs current manager. Remove deprecated path.
- [ ] **Rate limiter cleanup double-lock pattern** — `middleware.go:135-154` — `rl.mu.Lock()` held while acquiring `bucket.mu.Lock()`. Potential deadlock under contention. Restructure cleanup.
- [ ] **No validation on `id` query parameter** — `main.go:411-416` — URL query `id` not validated for length or characters. Add same validation as POST body ID.
- [ ] **`task.Status` enum validation only in `wotan.go`, not in `main.go`** — Two validators, different rigor. Consolidate into `validateTaskInput()` in `main.go`.

### cmd/dashboard-backend
- [ ] **Ping interval hardcoded at 54s** — `internal/websocket/server.go:242` — Unusual value. Make configurable via `Config.PingInterval`.
- [ ] **Broadcast channel has no backpressure** — `internal/server/server.go:226-253` — 256-entry buffer; after that, all clients miss updates. Add metrics counter + consider client-level flow control.
- [ ] **JSON unmarshal errors in metrics pipeline silently skipped** — `internal/server/server.go:282-284` — Logged as WARN but pipeline continues. Add dead-letter counter.

### Nix Infrastructure
- [ ] **`nix/Makefile` `cd tests` path wrong** — `nix/Makefile:90` — Should be `cd ./tests`. Fix relative path.
- [ ] **Makefile `metrics` target only curls wotan IP** — `nix/Makefile:186` — Should curl each container's IP. Fix loop to use container-specific IPs.
- [ ] **Makefile `deploy` target doesn't provision** — `nix/Makefile:107-109` — Only calls `nix build`. Add `lxc launch` + network setup instructions.
- [ ] **All services run as shared `unheaded:unheaded` user** — `nix/modules/common.nix:208-219` — No privilege separation. Add one system user per service.
- [ ] **`wotan-service.nix` creates duplicate wotan at different IP** — `nix/containers/wotan-service.nix` — Creates confusing dual-wotan topology. Consolidate.
- [ ] **`ExecStartPost` health check blocks — use async** — All service containers — Use `ExecStartPost=+` (async) to avoid blocking systemd unit start.
- [ ] **State version hardcoded** — `nix/modules/common.nix:43` — `system.stateVersion = "24.05"`. Pin to release and document upgrade path.

### CI/CD + Build
- [ ] **No SBOM generation** — Neither CI nor release workflow produces CycloneDX/SPDX artifact.
- [ ] **No release artifact signing** — `.github/workflows/release.yml:62-65` — Checksums exist but no Sigstore/Cosign signing.
- [ ] **Missing `-buildmode=pie` hardening flag** — `Makefile:10-11` — Add to `GO_BUILD_FLAGS` for position-independent executable.
- [ ] **`sudo` in Makefile targets without script validation** — `Makefile:106,110,114,137` — Scripts may not exist. Add `[ -f script.sh ] || exit 1` guards.

### Frontend
- [ ] **No CSP meta tag in HTML files** — `kanban/index.html`, `dashboard/index.html` — Add `<meta http-equiv="Content-Security-Policy">` as defense-in-depth fallback.
- [ ] **SRI missing on Google Fonts CSS** — `kanban/index.html:9` — Add `integrity="sha384-..."` to external font link.
- [ ] **`console.log` in 12+ production locations** — `board-viz.js:147,295`, `timeline-reader.js:22,28,46,57,66,75,84,89`, `particles.js:114`, `packet-flow.js:4` — Gate behind `window.DEBUG_MODE` check or remove.
- [ ] **SSE reconnect uses fixed 3s delay** — `kanban/js/timeline-reader.js:93-97` — Implement exponential backoff: `Math.min(3000 * 2 ** attempt, 30000)` + jitter.

---

## 🟢 P3 — POLISH: BEFORE GA (12 items)

- [ ] **Request IDs not globally unique** — `services/captain/api.go:382` — Counter resets on restart. Use UUID v4 or `X-Request-ID` passthrough.
- [ ] **No per-request tracing / correlation IDs** — All services — No request-ID header propagation. Add middleware to generate/pass `X-Request-ID` through all service calls.
- [ ] **Prometheus metrics format missing trailing newline** — `services/captain/api.go:199` — Some Prometheus parsers require trailing newline.
- [ ] **`string(rune(n))` in test generates control characters** — `services/captain/captain_test.go:490` — `string(rune(0))` is a null byte. Use `fmt.Sprintf("id-%d", n)`.
- [ ] **WebSocket `sourceIP` always `192.168.x.x`** — `cmd/dashboard-backend/internal/packetflow/generator.go:189-191` — Mock data lacks subnet variety. Randomize across multiple prefixes.
- [ ] **Inconsistent logging levels** — `cmd/dashboard-backend/internal/server/server.go` — Some errors logged as `.Error()`, others as `.Warn()`. Establish and follow severity rubric.
- [ ] **No `OpenAPI/Swagger` documentation** — All services — Zero API documentation. Blocks external integration and security review.
- [ ] **No mutation testing** — All Go code — Tests pass but mutation testing could reveal gaps. Evaluate `gremlins`.
- [ ] **`go.mod` cobra/viper declared but unused in CLI** — `cmd/unheaded/main.go` — Dependencies declared but stub doesn't use them. Implement CLI or remove deps.
- [ ] **No service-level README with test commands** — Most service dirs — Add standard `README.md` with `go test -v -race ./...` and coverage target.
- [ ] **`Rust toolchain@stable` floating tag** — `.github/workflows/ci.yml:93` — Prefer pinning to specific toolchain version for reproducibility.
- [ ] **`tokio = { features = ["full"] }` in trace-collector** — `cmd/trace-collector/Cargo.toml:7` — Pulls 30+ unused crates. Enumerate: `["rt-multi-thread", "macros", "sync", "io-util"]`.

---

## 📦 MISSING IMPLEMENTATIONS (Not bugs — just absent)

These are architectural components that don't exist yet.

- [ ] **Real eBPF `packet_marker`** — XDP hook, trace_id injection into packet metadata via BPF map, entropy source for unique IDs, `xdp_action` return
- [ ] **Real eBPF `flow_tracker`** — TC/tracepoint hook, `(src_ip, src_port, dst_ip, dst_port)` tuple tracking, ring buffer emission, connection state map
- [ ] **Real eBPF `latency_probe`** — `kprobe` on TCP lifecycle events, per-flow timestamp map, RTT calculation (with wraparound handling), perf event output
- [ ] **`trace-collector` userspace implementation** — aya ring buffer reader, event struct deserialization, Wotan publisher, backpressure handling, graceful shutdown
- [ ] **`unheaded-daemon` implementation** — eBPF program loader (aya/libbpf), LXD orchestration calls, state enforcement loop, drift detection, telemetry export, signal handling
- [ ] **`cmd/unheaded` CLI** — cobra command tree, viper config, subcommands: `status`, `apply`, `logs`, `ebpf list`, `containers list`
- [ ] **LXD client implementation** — Container launch/stop/exec/delete, profile management, network config, mTLS auth, all with proper error handling and interface definition
- [ ] **Gateway container** — NixOS reverse proxy (nginx/caddy), TLS termination, routing to kanban (port 8080) and dashboard (port 8081), rate limiting at edge
- [ ] **mTLS between all services** — Certificate provisioning (step-ca or CFSSL), mutual TLS on all gRPC/HTTP channels, cert rotation
- [ ] **VXLAN + BGP network layer** — Multi-host networking with EVPN, proper L2/L3 overlay, bird/frr integration in NixOS
- [ ] **Authentication layer** — JWT/OAuth2 middleware on all HTTP/WebSocket/SSE endpoints across all services
- [ ] **Timeguru Wotan message handler** — Process incoming `timeline.*` events, update milestone state from external messages
- [ ] **Secrets management** — SOPS/age encryption for service credentials, NixOS secret management module, zero plaintext secrets in config
- [ ] **Fuzz testing** — `gofuzz` corpus for: JSON parsing, protobuf decoding, YAML config, WebSocket frames, URL path parsing
- [ ] **Frontend unit tests** — Jest suite for `escapeHtml()`, SSE reconnection, card rendering, board-viz DOM updates, error states
- [ ] **Integration test implementations** — `tests/integration/`: CORS rejection, rate limit enforcement, XSS payload injection, SSE stream behavior
- [ ] **E2E test implementations** — `tests/e2e/`: browser-level SSE, task CRUD flow, WebSocket real-time updates, cross-service message propagation

---

## 🧪 TEST GAPS BY SERVICE

| Service | Has Tests | Missing |
|---------|-----------|---------|
| **captain** | ✅ partial | Error message validation broken (`contains()` bug), no Wotan publish failure test, no write-write race test |
| **timeguru** | ✅ partial | Mock not thread-safe, no HTTP method validation tests, no concurrent update tests |
| **architect** | ✅ partial | No deep copy isolation test, no Metadata DoS test, no enum rejection test, no CIDR validation test |
| **micromanager** | ✅ partial | No subscriptions map race test, no description overflow test, no concurrent create test |
| **kanban-app** | ✅ good | No CORS rejection test, no auth (401) test, no CSP header test, no rate-limit bypass test |
| **dashboard-backend** | ✅ partial | `server.go` has ZERO tests; no CORS test, no auth test, no cardinality test |
| **wotan-client** | ✅ partial | No `StreamMessages+Close` race test, no reconnection test, no payload size test |
| **lxd client** | ❌ none | Complete stub — untestable until implemented |
| **eBPF** | ❌ none | Not real programs — no test framework possible until rewrite |
| **frontend** | ❌ none | Zero JS tests anywhere |
| **tests/unit** | ❌ empty | `.gitkeep` only |
| **tests/integration** | ❌ empty | `.gitkeep` only |
| **tests/e2e** | ❌ empty | `.gitkeep` only |
| **nix/tests** | ⚠️ 90% stub | 14/18 integration tests are log-only stubs |

---

## ⚡ QUICK WINS (< 1 hour each, highest leverage)

| # | Fix | File:Line | Time |
|---|-----|-----------|------|
| 1 | Remove CORS wildcard `origin = "*"` fallback | `middleware.go:20-26` | 5m |
| 2 | Remove `gosec -no-fail` from CI | `ci.yml:150` | 2m |
| 3 | Pin `securego/gosec@v2.20.0` | `ci.yml:148` | 2m |
| 4 | Add `conn.SetReadLimit(1<<20)` to WebSocket | `websocket/server.go:89` | 5m |
| 5 | Remove WebSocket `CheckOrigin: return true` | `websocket/server.go:92-95` | 10m |
| 6 | Fix `pathGoesUp()` — `path[0:1]` → `path[:2]` | `storage.go:295` | 2m |
| 7 | Wrap `r.Body` with `io.LimitReader` in all handlers | `handlers.go`, `api.go` | 20m |
| 8 | Add `sync.RWMutex` to `subscriptions` map | `service.go:19` | 5m |
| 9 | Fix `contains()` test helper | `captain_test.go:765` | 5m |
| 10 | Remove `metrics.js` `<script>` tag from dashboard | `dashboard/index.html:34` | 2m |
| 11 | Delete `iptables -A OUTPUT -j ACCEPT` | `networking.nix:144` | 2m |
| 12 | Remove `requires = [ "wotan.service" ]` from all service containers | `*-service.nix` | 10m |
| 13 | Fix `wotan.nix` source path | `nix/packages/wotan.nix:8` | 5m |
| 14 | Remove duplicate node exporter definition | `networking.nix:195-208` | 5m |
| 15 | Fix Makefile `$$c` → `$${c}` | `nix/Makefile:168` | 2m |
| 16 | Run `go get -u ./... && go mod tidy` | `go.mod` | 5m |
| 17 | Change captain storage default from `/tmp` to `/var/lib/captain` | `storage.go:24` | 2m |
| 18 | Change timeguru DB dir from `0755` to `0700` | `storage.go:50` | 2m |
| 19 | Add `IdleTimeout` + `MaxHeaderBytes` to HTTP servers | `main.go:219-224` | 10m |
| 20 | URL-encode topic name in wotan-client | `client.go:122` | 5m |

---

## 📊 FINAL SUMMARY

| Priority | Count | Category |
|----------|-------|----------|
| 🔴 P0 | **55** | Block production |
| 🟠 P1 | **45** | Block deployment |
| 🟡 P2 | **35** | Fix before GA |
| 🟢 P3 | **12** | Polish |
| 📦 Missing impls | **17** | Not yet built |
| **TOTAL** | **164** | |

**Production Readiness:** 🔴 NOT READY
**Internal Alpha:** 🟡 CONDITIONAL on fixing P0 security items first
**Estimated time to production-ready:** 12–16 weeks with parallel execution

---

## 🔍 AGENT INDEX (for follow-up)

| Agent | Domain | ID |
|-------|--------|----|
| A | services/captain | a4969dd |
| B | services/timeguru | a6b01ce |
| C | services/architect | a80069f |
| D | services/micromanager | a4ae9ab |
| E | cmd/kanban-app | a6de869 |
| F | cmd/dashboard-backend | a34e358 |
| G | daemon + CLI + trace-collector + eBPF | a3f6919 |
| H | pkg/wotan-client + pkg/lxd | aba2312 |
| I | nix/ infrastructure | a5614d9 |
| J | frontend + CI/CD + build | a5efd3e |

---

## 🔴 eBPF ALPHA SPRINT — DEVELOPER REVIEW AMENDMENTS (February 17, 2026)

**Context:** Review of the "Real eBPF Data into Unheaded Dashboard" sprint plan.
**Reviewer:** Developer skill (paranoid mode)
**Verdict:** Architecture is sound. Details need hardening before code ships.

### Amendment 1: TEST PLAN REQUIRED (P0)
**No tests exist in the sprint plan.** Zero `#[test]`, zero `#[cfg(test)]` modules.

Required before merge:
- [ ] `collector/src/tests.rs` — Unit tests for IPv4/IPv6 address formatting (`[u8;16]` + `af` → string). Table-driven: loopback, broadcast, mapped v4, link-local v6, all-zeros, all-ones.
- [ ] `collector/src/tests.rs` — Unit tests for JSON serialization matching `cmd/dashboard-backend/internal/ebpf/types.go` exactly (field names, types, omitempty behavior).
- [ ] `collector/src/tests.rs` — Unit tests for rate limiter logic (token bucket drain, refill, burst, edge cases).
- [ ] `common/src/lib.rs` — `#[test]` for `repr(C)` struct size assertions (`assert_eq!(std::mem::size_of::<PacketEvent>(), EXPECTED)`). Kernel/userspace size mismatch = silent corruption.

### Amendment 2: bpf-linker ARM64 FALLBACK CONFIG (P1)
**Pre-write the fallback `.cargo/config.toml` for `rust-lld`** — don't discover it mid-sprint.

- [ ] Create `ebpf-programs/.cargo/config.toml` with both configurations documented (bpf-linker primary, rust-lld fallback).
- [ ] Document: if `cargo +nightly install bpf-linker` fails on arm64, switch linker line to `linker = "rust-lld"` and add `rustflags = ["-C", "linker-flavor=gnu-lld"]`.

### Amendment 3: RATE LIMITER SPECIFICATION (P1)
**"~100 events/sec per topic" is underspecified.**

- [ ] Implement token bucket rate limiter (not sampling). Bucket size 100, refill rate 100/sec.
- [ ] When events are dropped, emit a summary counter: `{"dropped_since_last": N, "total_dropped": M}` on the same Wotan topic every 5 seconds.
- [ ] Dashboard must display "sampling X of Y events/sec" when drops are occurring. Users need to know the difference between "10 packets/sec on the wire" and "10 packets/sec shown out of 50,000."

### Amendment 4: WOTAN PUBLISH RESILIENCE (P1)
**No error handling for Wotan HTTP POST failures.**

- [ ] Implement exponential backoff with jitter on HTTP POST failure (base 100ms, max 10s, factor 2).
- [ ] Bounded in-memory ring buffer (1024 events max) for events during Wotan unavailability. Drop oldest on overflow.
- [ ] Log Wotan connection state transitions: `connected → disconnected → reconnecting → connected`.
- [ ] Do NOT crash the collector if Wotan is down. The eBPF programs keep running in kernel regardless.

### Amendment 5: `--ws-allowed-origins` MUST DEFAULT DENY (P0)
**TODO.md item already flags `CheckOrigin always returns true` as P0 security.**

- [ ] If `--ws-allowed-origins` flag is empty/unset, default to `deny-all` (reject all WebSocket upgrades with 403).
- [ ] Verify the existing `CheckOrigin` implementation in `cmd/dashboard-backend/internal/websocket/server.go:92-95` is actually replaced, not just augmented.
- [ ] Add test: WebSocket upgrade from unlisted origin returns 403.

### Amendment 6: `unsafe` BLOCK AUDIT FOR eBPF KERNEL CODE (P1)
**XDP packet parsing requires raw pointer arithmetic.**

- [ ] Enumerate all `unsafe` blocks in `packet_counter.rs` and `tcp_latency.rs` with inline `// SAFETY:` comments explaining why each is necessary.
- [ ] Minimum expected `unsafe` usage: `ctx.data()` / `ctx.data_end()` pointer access, BPF map operations.
- [ ] Verify bounds checks: `if data_offset + size > data_end { return XDP_PASS; }` before every pointer dereference. The verifier enforces this but document it for humans too.

### Amendment 7: INCREMENTAL VERIFIER STRATEGY (P1)
**Kernel verifier on arm64 is strict. Don't attempt full 5-tuple + IPv6 in first pass.**

- [ ] Phase A: Minimal XDP — count packets, emit `{ timestamp_ns, packet_len }` only. Verify loads and attaches.
- [ ] Phase B: Add IPv4 5-tuple extraction. Verify.
- [ ] Phase C: Add IPv6 parsing. Verify.
- [ ] Phase D: Add direction detection. Verify.
- [ ] Each phase must pass `sudo bpftool prog list` + ring buffer read before proceeding.

### Amendment 8: GO VERSION COMPATIBILITY CHECK (P2)
**Sprint plan says "Go 1.26" on host. Codebase built with Go 1.24.0.**

- [ ] Verify `go.mod` `go` directive is compatible. Go 1.26 should be backward-compatible but confirm no `go:build` constraints break.
- [ ] After `go build -o bin/dashboard-backend ./cmd/dashboard-backend/`, run `go vet ./cmd/dashboard-backend/...` to catch version-specific issues.

### Amendment 9: `resolveServiceByID` MAP ITERATION ORDER BUG (P1)
**Go map iteration order is randomized. Service ID→name mapping is non-deterministic.**

- [ ] `cmd/dashboard-backend/internal/server/server.go` — `resolveServiceByID()` iterates `s.config.ServiceEndpoints` map with a counter. Map iteration order changes between restarts. If eBPF collector stamps `service_id=3` meaning "timeguru", the dashboard may resolve it to "captain" or "architect" next restart.
- [ ] Fix: Replace map iteration with an explicit ordered slice or `ServiceIDMap map[uint8]string` config. The eBPF collector and dashboard-backend must agree on the ID→name mapping — it can't be emergent from map iteration.
- [ ] This also affects `resolveServiceName()` (first match wins, but multiple services could share a host IP in localhost testing — all on 127.0.0.1).

### Amendment 10: IPv6 MESH METADATA TRANSPORT (P2)
**Sprint plan punts IPv6 mesh metadata: "carried separately (future: flow label or extension header)". Needs a concrete plan.**

- [ ] **Alpha (now):** IPv6 packets get standard 5-tuple extraction with NO mesh metadata. Document this explicitly in the collector code: `// TODO: IPv6 mesh meta not yet supported — af=10 events have mesh=null`.
- [ ] **v2 candidate — IPv6 Flow Label (20 bits):** Stamp `trace_hash_lo` (truncated to 20 bits) into the flow label field at XDP. Low-cost, no packet resizing. But only 20 bits — enough for trace correlation, not full mesh context.
- [ ] **v3 candidate — Hop-by-Hop Options Header:** Custom option type in 0x1E experimental range (RFC 4727). Full mesh metadata in extension header. Requires `bpf_xdp_adjust_head()` to grow packet — complex, verifier-hostile. More practical at TC hook than XDP. Age 2/3 territory.
- [ ] **Decision needed:** When IPv6 mesh meta ships, the `PacketEvent.Mesh` field will go from always-null on IPv6 to populated. Dashboard must handle the transition (already does via `omitempty` + nil check, but verify).

---

### SPRINT REVIEW SUMMARY

| Amendment | Priority | Est. Time | Blocks Ship? |
|-----------|----------|-----------|--------------|
| 1. Test plan | P0 | 2-3 hrs | YES |
| 2. bpf-linker fallback | P1 | 15 min | No (contingency) |
| 3. Rate limiter spec | P1 | 1 hr | No (but lies to user) |
| 4. Wotan resilience | P1 | 1 hr | No (but crashes) |
| 5. WS origin deny-all | P0 | 30 min | YES (security) |
| 6. unsafe audit | P1 | 30 min | No (documentation) |
| 7. Incremental verifier | P1 | 0 min (planning) | No (risk mitigation) |
| 8. Go version check | P2 | 5 min | No |
| 9. resolveServiceByID map order | P1 | 30 min | No (but wrong data) |
| 10. IPv6 mesh metadata plan | P2 | 0 min (design doc) | No (alpha=null) |

**TESTS BEFORE FEATURES. TRUST NOTHING.**

---
*Generated 2026-02-17 by 10 parallel agents using the unheaded-developer skill.*
*Scope: 32K LOC accessible in workspace. User reports full codebase is ~432K LOC. Review should be re-run with full codebase mounted.*

---

## 📝 CREATIVE / DOCUMENTATION

- [x] **Write short fictional story (~3 pages)** — "The First Packet" — `the-first-packet.md` — Origin story of the protocol atom. (Completed 2026-02-17)
- [x] **Write PROTOCOL_FOUNDATION.md** — `docs/PROTOCOL_FOUNDATION.md` — Canonical vision document: 4-layer model, Gnostic bindings, KV peeling, exponential composition, Yaldabaoth chaos model. Supersedes ARCHITECTURE.md layer model. (Completed 2026-02-17)
- [x] **Drop a Chronicles of Amber reference** — Woven into `the-first-packet.md` (full Amber narrative throughout) AND added to `unheaded-kingdom-SKILL-UPDATE.md` (Protocol Foundation section, Amber Mapping section, glossary entries). Pattern = Protocol, Shadow = outside IPv4/IPv6, Amber = Kingdom, Logrus = Yaldabaoth, Corwin's memory = Anamnesis, Royal Family = Wotan. (Completed 2026-02-17)

## 🔮 PROTOCOL FOUNDATION — IMPLEMENTATION ITEMS (February 17, 2026)

- [ ] **Define Monad `repr(C)` canonical packet struct** — The One. The unified format. Extension header layout, key/value byte positions, version field, checksum. Everything derives from this struct. (P0 for protocol work)
- [ ] **Implement Sophia dictionary tree structure** — BPF maps of maps for exponential key composition. Root map → sub-dictionary maps. O(depth) lookups per packet. Needs: Monad struct, BPF map layout, userspace sync protocol. (P0 for protocol work)
- [ ] **Design Sophia dictionary versioning protocol** — Atomic propagation across all BPF maps on all hops. Candidate: version byte in header, BPF map per version, reader checks version. CAS for conflict resolution, Wotan as single writer. (P1)
- [ ] **Implement Anamnesis ring buffer event sourcing** — Per-CPU ring buffers storing raw exponent keys + timestamps. Replay through any Sophia dictionary version. Kenoma as materialized projection. KV peeling for arbitrary dimensions. (P1)
- [ ] **Design Yaldabaoth eBPF chaos programs** — TC-attached: bit flips, delays, duplications, truncations, chaos markers. Emits to Anamnesis for auditability. Indistinguishable from real failure at Layer 1+. (P2)
- [ ] **Exponential key composition proof-of-concept** — 2-byte composed lookup demo: key[0] selects sub-dictionary, key[1] selects meaning. Prove 65,536 meanings from 2 bytes with O(2) BPF map lookups. (P1)
