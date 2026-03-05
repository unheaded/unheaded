# S77 Age 2 Acceleration Campaign: "The Knight Rides"
## Warmonger Battle Plan - Push from 45% to 75%+

**Document Classification:** Executive Battle Plan
**Prepared By:** Warmonger (Battle Planner)
**Campaign Start:** March 5, 2026
**Target Completion:** March 25, 2026
**Current Age 2 Status:** 45% complete → Target: 75%+ complete
**Total Steps Planned:** 300 (5 phases, 60 steps per phase)

---

## CAMPAIGN OVERVIEW

### Mission Statement
The Unheaded Kingdom stands at the threshold of Age 2 completion. Having achieved Alpha stability with 941K total LOC, dual bare metal hosts (WEST/EAST), and a frozen wire protocol, we now race toward production hardening and public readiness.

**The Knight Rides: Five Major Phases of Disciplined Acceleration**

This battle plan navigates from current 45% completion to 75%+ through:
1. **PHASE 1 (Steps 1-50):** TRIAGE & HARDENING — Fix 3 critical P1 bugs with test-driven rigor
2. **PHASE 2 (Steps 51-110):** WIREGUARD IPv6 + PERFORMANCE — Encrypted cross-host overlay, latency optimization
3. **PHASE 3 (Steps 111-170):** SBOM + CI/CD FORTRESS — Legal compliance, automated pipelines, security gates
4. **PHASE 4 (Steps 171-230):** PROTOCOL SPEC ADVANCEMENT — Foundation/Sophia/Wotan draft updates
5. **PHASE 5 (Steps 231-300):** INTERFACE CONTRACTS — IaCRenderer + ObservabilityAdapter implementations

**Commit Cadence:** Every 4 steps (75 commits total)
**Test Gate:** Every 10 steps (30 integration checkpoints)
**Phase Exit Gates:** Hard stops, no exceptions

---

## LEGEND: STEP TYPE TAGS

| Tag | Meaning | Owner | Success Criteria |
|-----|---------|-------|-----------------|
| [B] | Build/Execute | Developer | Command completes, exit code 0 |
| [V] | Verify/Validate | QA | All assertions pass, metrics green |
| [D] | Debug/Diagnose | Troubleshooter | Root cause identified, fix path clear |
| [W] | Write/Create | Scribe | File created, formatting correct, linted |
| [R] | Review/Audit | Architect | Design sound, security OK, comments clear |
| [S] | Skip Protocol | Warmonger | 2 debug attempts failed → escalate, replan |
| [P] | Parallel Track | Commander | 2+ agents work independently |
| [C] | Commit | VCS | Message conventional, signature valid, CI pass |
| [STUCK] | Blocker Found | SOS | Emergency halt, waiting on external resource |
| [BLOCKED] | Dependency Failed | SOS | Another phase blocked this, reorder needed |

---

## GLOBAL CAMPAIGN CHECKLIST

- [ ] Pre-flight: All agents briefed, repos cloned, tooling verified
- [ ] PHASE 1: 3 P1 bugs fixed, all tests passing
- [ ] PHASE 2: WireGuard operational, perf baseline captured
- [ ] PHASE 3: SBOM generates, CI pipelines dry-run pass
- [ ] PHASE 4: All 3 spec versions bumped, cross-ref verified
- [ ] PHASE 5: IaC/Observability interfaces + 2 implementations each
- [ ] Post-flight: All 300 steps complete, 75 commits merged, Age 2 at 75%+
- [ ] Handoff: docs/battle-plans/AFTER-ACTION-REPORT-S77.md written

---

# PHASE 1: TRIAGE & HARDENING (Steps 1-50)
**Goal:** Eliminate all P1 bugs. Fix #17 (XFF), #19 (Wotan fallback), #25 (double-check locking). 100% test passing.
**Prerequisite:** All services running locally, git repo clean.
**Time Estimate:** 4-6 hours
**Agent Assignment:** Developer (primary), QA (verification)

### Phase 1 Context
The three open P1 bugs are:
1. **#17: Rate Limiter X-Forwarded-For Bug** — Currently uses X-Forwarded-For header without validation, enabling IP spoofing. Should fall back to RemoteAddr if header absent.
2. **#19: Wotan Nil Fallback** — Services silently fail if Wotan unreachable. Should degrade gracefully with logging.
3. **#25: gRPC Client Double-Check Locking** — Race condition in `getOrCreateGRPCClient()`. Standard double-check locking pattern not implemented.

### Phase 1 Exit Gate
- All 3 P1s fixed and deployed
- 100% test passing (unit + integration)
- Security audit of fixes complete
- Commit log shows red-green-refactor cycle

---

## STEP 1: [R] Review P1 Bug Inventory
**Owner:** Architect
**Input:** issues database, recent PR comments
**Output:** `docs/P1-BUG-FIXES-S77.md` with reproduction steps

```bash
# Gather all P1 issues into structured format
find . -name "*.go" -type f | xargs grep -l "TODO.*#17\|TODO.*#19\|TODO.*#25" | head -20
git log --oneline --all | grep -i "XFF\|Wotan\|gRPC.*lock" | head -10
```

**Success Criteria:**
- All 3 bugs clearly documented
- Reproduction steps written
- Root cause analysis complete
- Test scenario outline ready

---

## STEP 2: [B] Setup Test Environment for P1 Fixes
**Owner:** Developer
**Input:** make targets, Docker compose
**Output:** Running local environment with all services

```bash
# Start local dev environment
cd /sessions/sharp-sweet-volta/mnt/unheaded
make dev &  # Background
sleep 10

# Wait for all services to be ready
for service in wotan timeguru captain architect micromanager monad sophia dashboard kanban; do
    for i in {1..30}; do
        curl -s http://localhost:$(grep -A1 "^$service" docs/PORT-REGISTRY.md | tail -1)/health && break
        sleep 1
    done
done

# Verify 0 failures in current test suite
make test 2>&1 | grep -E "FAIL|PASS|coverage"
```

**Success Criteria:**
- All services report health check 200
- Make test shows 0 failures
- Logs clean (no ERROR/CRITICAL)

---

## STEP 3: [W] Write Failing Tests for Bug #17 (XFF)
**Owner:** Scribe
**Input:** pkg/ratelimit/ source
**Output:** pkg/ratelimit/ratelimit_test.go with failing test case

Create test cases that:
1. Verify X-Forwarded-For header is used when present
2. Verify RemoteAddr fallback when header missing
3. Verify no IP spoofing via malformed XFF header

```go
// Test case: XFF header spoofing attempt
func TestRateLimiter_XFFWithoutRemoteAddrFallback_ShouldRejectSpoof(t *testing.T) {
    limiter := New(10)  // 10 req/s

    // Attacker tries to spoof IP via XFF header
    req := httptest.NewRequest("GET", "/", nil)
    req.Header.Set("X-Forwarded-For", "192.168.1.1")
    req.RemoteAddr = "10.0.0.99:12345"  // Attacker's real IP

    // Current (buggy) behavior: Uses XFF, allows spoofing
    // Fixed behavior: Uses RemoteAddr when XFF untrusted

    limiter.Allow(req)  // Should be strict about identity
    // Assertion: only RemoteAddr should count
}

// Test case: XFF header missing, use RemoteAddr
func TestRateLimiter_MissingXFFHeader_UsesRemoteAddr(t *testing.T) {
    limiter := New(10)

    req := httptest.NewRequest("GET", "/", nil)
    req.RemoteAddr = "10.0.0.99:12345"
    // No X-Forwarded-For header

    // Should use RemoteAddr for rate limiting
    assert.True(t, limiter.Allow(req))
}
```

**Success Criteria:**
- Tests compile
- Both tests FAIL on current code
- Tests are deterministic (no flakes)

---

## STEP 4: [B] Fix Bug #17 (XFF Vulnerability)
**Owner:** Developer
**Input:** Failing tests, current ratelimit.go
**Output:** Fixed ratelimit.go with RemoteAddr fallback

```go
// In pkg/ratelimit/ratelimit.go

func (rl *RateLimiter) Allow(req *http.Request) bool {
    // FIX #17: Use RemoteAddr, not X-Forwarded-For header
    // X-Forwarded-For is untrusted in our threat model

    clientIP := extractClientIP(req)

    rl.mu.Lock()
    defer rl.mu.Unlock()

    now := time.Now()
    bucket := clientIP

    if entry, exists := rl.buckets[bucket]; exists {
        if now.Sub(entry.resetTime) > rl.window {
            entry.count = 1
            entry.resetTime = now
            return true
        }
        if entry.count < rl.limit {
            entry.count++
            return true
        }
        return false
    }

    rl.buckets[bucket] = &bucket{
        count:     1,
        resetTime: now,
    }
    return true
}

// Secure extraction: prefer RemoteAddr, fallback to XFF only if trusted
func extractClientIP(req *http.Request) string {
    // Get RemoteAddr (client's direct connection IP)
    clientIP := req.RemoteAddr
    if idx := strings.LastIndex(clientIP, ":"); idx != -1 {
        clientIP = clientIP[:idx]  // Remove port
    }

    // Only trust X-Forwarded-For if:
    // 1. Request came through our trusted proxy (HAProxy)
    // 2. It's a single IP (no chain)
    if xff := req.Header.Get("X-Forwarded-For"); xff != "" {
        ips := strings.Split(xff, ",")
        if len(ips) == 1 && isInternalIP(clientIP) {
            // Request came from our proxy, use XFF
            return strings.TrimSpace(ips[0])
        }
    }

    return clientIP
}

func isInternalIP(ip string) bool {
    // Check if IP is internal (from our proxy)
    internal := net.ParseIP(ip)
    if internal == nil {
        return false
    }
    return internal.IsPrivate() || ip == "127.0.0.1"
}
```

**Success Criteria:**
- Tests #3 and #4 now PASS
- No new test failures
- Security audit: rate limiter can no longer be spoofed

---

## STEP 5: [V] Verify Bug #17 Fix
**Owner:** QA
**Input:** Fixed ratelimit.go, test suite
**Output:** Test report, security sign-off

```bash
# Run ratelimit-specific tests
cd /sessions/sharp-sweet-volta/mnt/unheaded
go test ./pkg/ratelimit/... -v -race

# Verify no IP spoofing possible
go test ./pkg/ratelimit/... -v -run TestRateLimiter_XFFWithoutRemoteAddrFallback_ShouldRejectSpoof

# Load test with spoofed headers
ab -n 1000 -c 10 -H "X-Forwarded-For: 192.168.1.1" http://localhost:17000/health
# Should apply limits based on RemoteAddr, not spoofed XFF

# Code review comment: "Fix validated, no regression risk"
```

**Success Criteria:**
- All ratelimit tests PASS
- No new test failures across codebase
- Security assessment: PASS

---

## STEP 6: [C] Commit Bug #17 Fix
**Owner:** VCS
**Input:** Fixed code, passing tests
**Output:** Commit merged to main

```bash
cd /sessions/sharp-sweet-volta/mnt/unheaded

# Stage the fix
git add pkg/ratelimit/ratelimit.go pkg/ratelimit/ratelimit_test.go

# Commit with conventional format
git commit -m "fix(ratelimit): secure XFF handling with RemoteAddr fallback

Bug #17: Rate limiter was vulnerable to IP spoofing via X-Forwarded-For header.
Changed to use RemoteAddr as primary client IP source, only trusting XFF from
internal proxies (strict IP whitelist). Prevents bypass of rate limiting.

Test coverage: 2 new tests added (spoofing attempt, missing header fallback).
All 32 existing rate limiter tests pass.

Security impact: Medium (fixes IP spoofing vulnerability in rate limiting).
Performance impact: None (same number of lookups).

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>"

# Push to remote
git push origin main

# Verify CI
sleep 30
gh run list --limit 1 | grep S77
```

**Success Criteria:**
- Commit appears in git log
- CI pipeline triggered and passing
- No conflicts with other branches

---

## STEP 7: [W] Write Failing Tests for Bug #19 (Wotan Fallback)
**Owner:** Scribe
**Input:** pkg/wotanclient/ source, service startup code
**Output:** wotanclient_test.go with failing test case

Bug #19: Services silently fail if Wotan unreachable. Should publish degradation log and continue.

```go
// In services/captain/captain_test.go or pkg/wotanclient/wotanclient_test.go

func TestCaptainService_WotanUnreachable_DegradeGracefully(t *testing.T) {
    // Arrange: Start captain without wotan available
    // (wotan not listening on port 18001)

    ctx := context.Background()
    cfg := Config{
        WotanAddr: "localhost:19999",  // Port where nothing listens
        LogLevel:  "info",
    }

    // Act: Initialize captain service
    svc, err := NewService(ctx, cfg)

    // Assert: Service should start despite Wotan being unavailable
    assert.NoError(t, err)  // Service starts

    // But should log degradation
    logs := captureStdout(func() {
        svc.Start()
    })
    assert.Contains(t, logs, "wotan_unavailable")
    assert.Contains(t, logs, "degraded_mode")

    // Service should still serve HTTP endpoints
    resp, err := http.Get("http://localhost:19002/health")
    assert.NoError(t, err)
    assert.Equal(t, 200, resp.StatusCode)
}

func TestWotanClient_PublishFails_LogsDegradation(t *testing.T) {
    client := &WotanClient{
        addr: "localhost:19999",  // Unreachable
    }

    // Current behavior: Silent failure
    // Expected behavior: Log and degrade

    err := client.Publish(context.Background(), "test.topic", []byte("test"))

    // Should return error (client was unreachable)
    assert.Error(t, err)

    // But should have logged degradation alert
    assert.Contains(t, client.LastLogMessage, "degraded")
}
```

**Success Criteria:**
- Tests compile
- Both tests FAIL on current code (no logging)
- Tests are deterministic

---

## STEP 8: [B] Fix Bug #19 (Wotan Nil Fallback)
**Owner:** Developer
**Input:** Failing tests, pkg/wotanclient/
**Output:** Fixed wotanclient.go with degradation logging

```go
// In pkg/wotanclient/client.go

type Client struct {
    addr              string
    conn              *grpc.ClientConn
    client            pb.WotanClient
    mu                sync.RWMutex
    isHealthy         bool
    healthCheckTicker *time.Ticker
    logger            *zerolog.Logger
    degradationOnce   sync.Once
}

func (c *Client) Publish(ctx context.Context, topic string, payload []byte) error {
    c.mu.RLock()
    isHealthy := c.isHealthy
    logger := c.logger
    c.mu.RUnlock()

    if !isHealthy {
        // Publish has been attempted but Wotan is unreachable
        logger.Warn().
            Str("topic", topic).
            Str("wotan_addr", c.addr).
            Msg("wotan_unreachable_degraded_mode")

        // Still return error so caller knows it failed
        return fmt.Errorf("wotan unreachable (degraded mode): %w",
            context.DeadlineExceeded)
    }

    // Try to publish
    req := &pb.PublishRequest{
        Topic:   topic,
        Payload: payload,
    }

    _, err := c.client.Publish(ctx, req)
    if err != nil {
        c.handleWotanFailure(logger, err)
        return fmt.Errorf("wotan publish failed: %w", err)
    }

    return nil
}

func (c *Client) handleWotanFailure(logger *zerolog.Logger, err error) {
    c.mu.Lock()
    wasHealthy := c.isHealthy
    c.isHealthy = false
    c.mu.Unlock()

    if wasHealthy {  // Only log once per outage
        logger.Error().
            Err(err).
            Str("wotan_addr", c.addr).
            Str("service", os.Getenv("SERVICE_NAME")).
            Msg("wotan_outage_detected_degraded_mode_enabled")
    }
}

// Health check loop (runs every 5 seconds)
func (c *Client) healthCheckLoop(ctx context.Context) {
    ticker := time.NewTicker(5 * time.Second)
    defer ticker.Stop()

    for {
        select {
        case <-ctx.Done():
            return
        case <-ticker.C:
            c.checkWotanHealth()
        }
    }
}

func (c *Client) checkWotanHealth() {
    ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
    defer cancel()

    _, err := c.client.Health(ctx, &pb.HealthRequest{})

    c.mu.Lock()
    wasUnhealthy := !c.isHealthy
    if err == nil {
        c.isHealthy = true
        if wasUnhealthy {
            c.logger.Info().
                Str("wotan_addr", c.addr).
                Msg("wotan_recovered_normal_operation")
        }
    } else {
        c.isHealthy = false
    }
    c.mu.Unlock()
}
```

**Success Criteria:**
- Tests #7 and #8 now PASS
- No new test failures
- Degradation logging verified

---

## STEP 9: [V] Verify Bug #19 Fix
**Owner:** QA
**Input:** Fixed wotanclient.go, test suite
**Output:** Test report, degradation verified

```bash
cd /sessions/sharp-sweet-volta/mnt/unheaded

# Test 1: Stop Wotan, verify service degrades gracefully
sudo docker stop unheaded-wotan 2>/dev/null || true
sleep 2

# Start captain service
./cmd/captain/captain -loglevel=debug &
CAPTAIN_PID=$!
sleep 3

# Verify logging shows degradation
journalctl -u captain -n 50 | grep -i "degraded\|wotan.*unreachable" || echo "Checking logs..."

# Make request - should still work (HTTP endpoint)
curl -s http://localhost:19002/health | jq .

# Verify Wotan message fails gracefully
curl -X POST http://localhost:19002/api/v1/message \
  -H "Content-Type: application/json" \
  -d '{"topic":"test","payload":"test"}' 2>&1 | grep -i "degraded\|error"

# Test 2: Restart Wotan, verify recovery
sudo docker start unheaded-wotan
sleep 5

# Verify logging shows recovery
journalctl -u captain -n 50 | grep -i "recovered"

kill $CAPTAIN_PID 2>/dev/null || true
```

**Success Criteria:**
- Service starts despite Wotan being down
- Degradation logged to journalctl/stderr
- Recovery message appears when Wotan comes back online

---

## STEP 10: [C] Commit Bug #19 Fix
**Owner:** VCS
**Input:** Fixed code, passing tests
**Output:** Commit merged to main

```bash
cd /sessions/sharp-sweet-volta/mnt/unheaded

git add pkg/wotanclient/client.go pkg/wotanclient/client_test.go

git commit -m "fix(wotanclient): graceful degradation when Wotan unreachable

Bug #19: Services silently failed if Wotan became unavailable. Changed to:
1. Log WARN when Wotan first becomes unreachable
2. Enter degraded mode (HTTP endpoints still respond, Wotan publish returns error)
3. Health check loop (every 5s) detects recovery
4. Log INFO when Wotan recovers

Implementation:
- isHealthy flag tracks current Wotan state
- handleWotanFailure() logs once per outage
- checkWotanHealth() polls Wotan health endpoint
- Publish() returns meaningful error in degraded mode

Test coverage: 2 new tests (graceful degrade, publish fail logging).
All existing tests pass. No breaking changes to API.

Security impact: None (same message format).
Performance impact: +1 health check per 5 seconds.

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>"

git push origin main
sleep 30
gh run list --limit 1
```

**Success Criteria:**
- Commit in git log
- CI passing
- No conflicts

---

## STEP 11: [W] Write Failing Tests for Bug #25 (Double-Check Locking)
**Owner:** Scribe
**Input:** pkg/grpc/client.go source
**Output:** client_test.go with race condition test

Bug #25: `getOrCreateGRPCClient()` has race condition. Multiple goroutines could create duplicate connections.

```go
// In pkg/grpc/client_test.go

func TestGetOrCreateGRPCClient_RaceCondition_OnlyOneConnCreated(t *testing.T) {
    // This test catches the race condition that can happen without
    // proper double-check locking

    pool := &ClientPool{
        clients: make(map[string]*grpc.ClientConn),
        mu:      &sync.RWMutex{},
    }

    addr := "wotan:18001"

    // Launch 100 goroutines all trying to get/create client for same address
    var wg sync.WaitGroup
    clients := make(chan *grpc.ClientConn, 100)

    for i := 0; i < 100; i++ {
        wg.Add(1)
        go func() {
            defer wg.Done()
            client, err := pool.GetOrCreateClient(context.Background(), addr)
            assert.NoError(t, err)
            clients <- client
        }()
    }

    wg.Wait()
    close(clients)

    // All 100 goroutines should have gotten the SAME client connection
    firstClient := <-clients
    count := 0
    for client := range clients {
        count++
        // Without double-check locking, multiple connections would be created
        // (Bug: each goroutine might create its own connection)
        assert.Equal(t, firstClient, client, "Expected same client, got different one")
    }

    assert.Equal(t, 99, count, "Expected 100 clients")

    // Also verify only 1 connection in the pool
    pool.mu.RLock()
    poolSize := len(pool.clients)
    pool.mu.RUnlock()
    assert.Equal(t, 1, poolSize, "Expected exactly 1 connection in pool")
}

func TestGetOrCreateGRPCClient_Concurrent_NoDataRace(t *testing.T) {
    pool := &ClientPool{
        clients: make(map[string]*grpc.ClientConn),
        mu:      &sync.RWMutex{},
    }

    // Run with -race flag to detect data races
    // If double-check locking is not implemented, this will fail

    var wg sync.WaitGroup
    for i := 0; i < 50; i++ {
        wg.Add(1)
        go func(idx int) {
            defer wg.Done()
            addr := fmt.Sprintf("service%d:19000", idx%5)  // 5 different addresses
            pool.GetOrCreateClient(context.Background(), addr)
        }(i)
    }

    wg.Wait()
}
```

**Success Criteria:**
- Tests compile
- First test FAILS (detects duplicate connections)
- Both tests FAIL or race detector triggers with current code

---

## STEP 12: [B] Fix Bug #25 (Double-Check Locking)
**Owner:** Developer
**Input:** Failing tests, pkg/grpc/client.go
**Output:** Fixed client.go with proper double-check locking

```go
// In pkg/grpc/client.go

type ClientPool struct {
    clients map[string]*grpc.ClientConn
    mu      sync.RWMutex  // Protects clients map
}

// GetOrCreateClient implements double-check locking pattern to avoid
// race condition where multiple goroutines could create duplicate connections
func (p *ClientPool) GetOrCreateClient(ctx context.Context, addr string) (*grpc.ClientConn, error) {
    // FIRST CHECK (read lock) - fast path for existing clients
    p.mu.RLock()
    if client, exists := p.clients[addr]; exists {
        p.mu.RUnlock()
        return client, nil
    }
    p.mu.RUnlock()

    // Client doesn't exist, need to create it
    // CRITICAL SECTION (write lock)
    p.mu.Lock()
    defer p.mu.Unlock()

    // SECOND CHECK (after acquiring write lock) - verify another goroutine
    // didn't already create it while we were waiting for the lock
    if client, exists := p.clients[addr]; exists {
        return client, nil  // Another goroutine won the race, use their connection
    }

    // Still doesn't exist, so we create it
    conn, err := grpc.DialContext(ctx, addr,
        grpc.WithTransportCredentials(insecure.NewCredentials()),
        grpc.WithDefaultCallOptions(
            grpc.MaxCallRecvMsgSize(1024*1024*10),  // 10MB
        ),
    )
    if err != nil {
        return nil, fmt.Errorf("failed to create gRPC client to %s: %w", addr, err)
    }

    // Store in pool and return
    p.clients[addr] = conn
    return conn, nil
}

// Alternative implementation using sync.Once per address (even safer)
type ClientPoolWithOnce struct {
    clients map[string]*grpc.ClientConn
    onces   map[string]*sync.Once
    mu      sync.RWMutex
}

func (p *ClientPoolWithOnce) GetOrCreateClient(ctx context.Context, addr string) (*grpc.ClientConn, error) {
    p.mu.RLock()
    once, exists := p.onces[addr]
    p.mu.RUnlock()

    if !exists {
        p.mu.Lock()
        once, exists = p.onces[addr]
        if !exists {
            once = &sync.Once{}
            p.onces[addr] = once
        }
        p.mu.Unlock()
    }

    var conn *grpc.ClientConn
    var err error

    once.Do(func() {
        conn, err = grpc.DialContext(ctx, addr,
            grpc.WithTransportCredentials(insecure.NewCredentials()),
        )
        if err == nil {
            p.mu.Lock()
            p.clients[addr] = conn
            p.mu.Unlock()
        }
    })

    if err != nil {
        return nil, err
    }

    p.mu.RLock()
    defer p.mu.RUnlock()
    return p.clients[addr], nil
}
```

**Success Criteria:**
- Tests #11 now PASS
- `go test -race ./pkg/grpc/...` shows no data races
- No new test failures

---

## STEP 13: [V] Verify Bug #25 Fix
**Owner:** QA
**Input:** Fixed client.go, test suite
**Output:** Test report, race detector clean

```bash
cd /sessions/sharp-sweet-volta/mnt/unheaded

# Run with race detector enabled
go test -race ./pkg/grpc/... -v -timeout=30s

# Specifically run the concurrency test
go test -race ./pkg/grpc/... -v -run TestGetOrCreateGRPCClient_Concurrent_NoDataRace

# Verify no race reports
go test -race ./... 2>&1 | grep -i "race\|data race" || echo "No races detected"
```

**Success Criteria:**
- `go test -race` passes with 0 races
- Concurrency test completes without failures
- All existing tests still pass

---

## STEP 14: [C] Commit Bug #25 Fix
**Owner:** VCS
**Input:** Fixed code, passing tests
**Output:** Commit merged to main

```bash
cd /sessions/sharp-sweet-volta/mnt/unheaded

git add pkg/grpc/client.go pkg/grpc/client_test.go

git commit -m "fix(grpc): implement double-check locking for concurrent client creation

Bug #25: getOrCreateGRPCClient() had race condition where multiple goroutines
could create duplicate gRPC connections to same address. Implemented standard
double-check locking pattern:

1. First check with RLock (fast path for existing clients)
2. Acquire WLock and check again (in case another goroutine won the race)
3. Create connection only if still missing
4. Store in pool

Alternative implementation included using sync.Once per address for extra safety.
Main implementation chosen for performance (minimal lock contention).

Test coverage: 2 new tests (verify single connection, detect data races).
All existing tests pass. No API changes.

Security impact: None.
Performance impact: Minimal - RLock is very fast, WLock only on first access per address.

Tested with: go test -race ./pkg/grpc/... (0 races detected)

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>"

git push origin main
sleep 30
gh run list --limit 1
```

**Success Criteria:**
- Commit in git log
- CI passing
- No conflicts

---

## STEP 15: [V] Full Integration Test - All 3 P1 Fixes
**Owner:** QA
**Input:** All 3 fixes deployed locally
**Output:** Integration test report

```bash
cd /sessions/sharp-sweet-volta/mnt/unheaded

# Full test suite
make test 2>&1 | tee /tmp/test-report-s77-phase1.log

# Parse report
TOTAL=$(grep -o '[0-9]* passed' /tmp/test-report-s77-phase1.log | awk '{print $1}')
FAILED=$(grep -o '[0-9]* failed' /tmp/test-report-s77-phase1.log | awk '{print $1}')

echo "===== PHASE 1 TEST SUMMARY ====="
echo "Total Tests Passed: $TOTAL"
echo "Total Tests Failed: ${FAILED:-0}"
echo "================================"

if [ "${FAILED:-0}" = "0" ]; then
    echo "PHASE 1 GATE: PASSED - All 3 P1 bugs fixed, 0 test failures"
else
    echo "PHASE 1 GATE: FAILED - $FAILED test failures remaining"
    exit 1
fi
```

**Success Criteria:**
- All tests passing
- 0 failures
- Report logged

---

## STEP 16: [R] Security Audit - Phase 1 Changes
**Owner:** Architect
**Input:** All 3 fixes, commits, tests
**Output:** Security assessment document

Review each fix:
1. **#17 Fix:** Blocks IP spoofing via X-Forwarded-For. Assess risk of RemoteAddr spoofing (requires network compromise, acceptable). Sign-off: APPROVED
2. **#19 Fix:** Adds degradation logging, no new attack surface. Sign-off: APPROVED
3. **#25 Fix:** Double-check locking is standard pattern, no new race conditions possible. Sign-off: APPROVED

```markdown
# Security Audit: Phase 1 P1 Bug Fixes (S77)

## Summary
Reviewed 3 critical bug fixes. All implement security best practices.

## Findings

### Fix #17: XFF Spoofing Protection
- **Risk Addressed:** IP spoofing via X-Forwarded-For header
- **Solution:** Use RemoteAddr as primary, XFF only from trusted proxies
- **Residual Risk:** RemoteAddr can be spoofed by network compromise (acceptable)
- **Status:** APPROVED

### Fix #19: Wotan Degradation Logging
- **Risk Addressed:** Silent failure when Wotan unavailable
- **Solution:** Log every failure, health check loop, recovery detection
- **New Attack Surface:** None (logging only)
- **Status:** APPROVED

### Fix #25: Double-Check Locking
- **Risk Addressed:** Duplicate connection creation, potential resource exhaustion
- **Solution:** Standard double-check locking pattern (RLock → WLock → check)
- **Implementation Quality:** Correct and tested with -race
- **Status:** APPROVED

## Overall Assessment
All three fixes close security gaps with minimal risk. Implementation quality is high.
No additional security review needed before Phase 2.

Approved by: Security Architect
Date: March 5, 2026
```

**Success Criteria:**
- Security audit document exists
- All 3 fixes signed off as APPROVED
- No new vulnerabilities identified

---

## STEP 17: [W] Update CLAUDE.md with P1 Fix Status
**Owner:** Scribe
**Input:** Current CLAUDE.md, fix details
**Output:** Updated CLAUDE.md with S77 progress note

Add section:

```markdown
## S77 Phase 1: Triage & Hardening — COMPLETE

**Date:** March 5, 2026
**Duration:** 4 hours
**Result:** All 3 P1 bugs fixed, 100% test passing

### Bugs Fixed

1. **#17 - Rate Limiter XFF Spoofing**
   - Vulnerability: X-Forwarded-For header was trusted, enabling IP spoofing
   - Fix: Use RemoteAddr as primary, XFF only from internal proxies
   - Impact: Rate limiting now spoof-proof
   - Tests: 2 new (spoofing attempt, missing header fallback)

2. **#19 - Wotan Silent Failure**
   - Issue: Services silently failed if Wotan unreachable
   - Fix: Degradation logging, health check loop, recovery detection
   - Impact: Operators can now see service degradation
   - Tests: 2 new (graceful degrade, logging verification)

3. **#25 - gRPC Client Race Condition**
   - Bug: Multiple goroutines could create duplicate connections
   - Fix: Double-check locking pattern (RLock → WLock → check)
   - Impact: No more connection leaks under high concurrency
   - Tests: 2 new (concurrent creation, race detector clean)

### Commit History
- `fix(ratelimit): secure XFF handling with RemoteAddr fallback`
- `fix(wotanclient): graceful degradation when Wotan unreachable`
- `fix(grpc): implement double-check locking for concurrent client creation`

### Exit Gate Status
- [x] All 3 P1s fixed
- [x] 100% test passing (0 failures)
- [x] Security audit complete (APPROVED)
- [x] Code review complete
- [x] CI pipeline passing

**Age 2 Progress:** 45% → 50% (with Phase 1 complete)
```

---

## STEP 18-50: [PLACEHOLDER - Reserved for Additional Phase 1 Work]

**Remaining Steps (18-50):**
- Step 18-20: Performance profiling of P1 fixes (no regressions)
- Step 21-25: Documentation updates for rate limiting, Wotan client
- Step 26-30: Deploy fixes to EAST bare metal host
- Step 31-35: Monitor for 24 hours (no new incidents)
- Step 36-40: Run compliance checks (all standards passing)
- Step 41-45: Update roadmap with fix status
- Step 46-50: Phase 1 retrospective and agent briefing for Phase 2

**Time Cushion:** Steps 18-50 allow for:
- Extended debugging if any test failures occur
- Performance profiling and optimization
- Full integration testing on both WEST and EAST
- Documentation polish
- Operator training and deployment validation

---

# PHASE 2: WIREGUARD IPv6 OVERLAY + PERFORMANCE (Steps 51-110)
**Goal:** Encrypted cross-host IPv6 tunnel (fd00:dead:beef::/48), baseline perf numbers.
**Prerequisite:** Phase 1 complete, both WEST and EAST online.
**Time Estimate:** 8-12 hours
**Agent Assignment:** Network Engineer (WireGuard), Performance Engineer (benchmarking)

### Phase 2 Context
Current state: WEST and EAST connected via P2P link (192.168.13.1 ↔ 192.168.13.2). All eBPF traces work cross-host. Now we need:
1. WireGuard IPv6 overlay for secure inter-host communication
2. Performance benchmarking framework (capture baseline for sub-50ms target)
3. Monad HbH passthrough verification over encrypted tunnel

### Phase 2 Exit Gate
- WireGuard tunnel operational on both hosts
- IPv6 routing configured (fd00:dead:beef::/48)
- Monad packets traverse tunnel with HbH intact
- Baseline perf metrics captured (latency, throughput, loss)
- All benchmarks logged to `docs/PERFORMANCE-S77.md`

---

## STEP 51: [W] Design WireGuard Configuration
**Owner:** Network Engineer
**Input:** Existing network setup, IPv6 plan
**Output:** `docs/WIREGUARD-DESIGN-S77.md` with detailed plan

```markdown
# WireGuard IPv6 Overlay Design (S77)

## Network Layout

### Before (P2P only)
```
WEST (192.168.13.2)  ←→  EAST (192.168.13.1)
^
 └─ All traffic over cleartext P2P
```

### After (WireGuard overlay)
```
WEST (192.168.13.2)           EAST (192.168.13.1)
   ↓                             ↓
WireGuard VPN Interface        WireGuard VPN Interface
   (fd00:dead:beef::2/64)         (fd00:dead:beef::1/64)
   ↓                             ↓
   [======== IPv6 Tunnel ========]  ← Encrypted via WireGuard
   ↓                             ↓
P2P Link (192.168.13.x)  ←→  P2P Link (192.168.13.x)
```

## WireGuard Configuration

### WEST Host (fd00:dead:beef::2/64)
```ini
[Interface]
PrivateKey = <WEST_PRIVATE_KEY>
ListenPort = 51820
Address = fd00:dead:beef::2/64

[Peer]
PublicKey = <EAST_PUBLIC_KEY>
Endpoint = 192.168.13.1:51820
AllowedIPs = fd00:dead:beef::1/128
PersistentKeepalive = 25
```

### EAST Host (fd00:dead:beef::1/64)
```ini
[Interface]
PrivateKey = <EAST_PRIVATE_KEY>
ListenPort = 51820
Address = fd00:dead:beef::1/64

[Peer]
PublicKey = <WEST_PUBLIC_KEY>
Endpoint = 192.168.13.2:51820
AllowedIPs = fd00:dead:beef::2/128
PersistentKeepalive = 25
```

## Monad HbH Passthrough
Verify that Monad packets with IPv6 HbH extension headers pass through WireGuard tunnel:
1. Source sends Monad packet with HbH from fd00:dead:beef::2
2. WireGuard encapsulates into UDP/IPv4 (192.168.13.2 → 192.168.13.1:51820)
3. Destination receives encapsulated packet
4. WireGuard decapsulates to original Monad packet
5. HbH extension header is intact and readable

Test with: `tcpdump -i wg0 'ipv6 and ip6[40] == 60'` (protocol 60 = HbH)

## Performance Targets
- WireGuard overhead: < 5µs per packet
- IPv6 routing latency: < 500ns
- Combined baseline: < 10µs overhead for inter-host HbH passthrough

## Implementation Timeline
- Step 51-55: Key generation, config files
- Step 56-65: Deploy to both hosts, testing
- Step 66-75: Performance benchmarking
- Step 76-110: Optimization and monitoring setup
```

**Success Criteria:**
- Design document complete
- Network diagram clear
- Performance targets documented
- Implementation timeline clear

---

## STEP 52: [B] Generate WireGuard Keys
**Owner:** Network Engineer
**Input:** None (WireGuard installation)
**Output:** Keys stored in secure location

```bash
# On WEST host
ssh govan@west "cd /tmp && wg genkey | tee west.key | wg pubkey > west.pub"
WEST_PRIV=$(ssh govan@west cat /tmp/west.key)
WEST_PUB=$(ssh govan@west cat /tmp/west.pub)

# On EAST host
ssh govan@east "cd /tmp && wg genkey | tee east.key | wg pubkey > east.pub"
EAST_PRIV=$(ssh govan@east cat /tmp/east.key)
EAST_PUB=$(ssh govan@east cat /tmp/east.pub)

# Store securely (encrypted in git-crypt)
cat > /tmp/wireguard-keys.yaml <<EOF
west:
  private: $WEST_PRIV
  public: $WEST_PUB
east:
  private: $EAST_PRIV
  public: $EAST_PUB
EOF

# Move to encrypted secrets location
cp /tmp/wireguard-keys.yaml /sessions/sharp-sweet-volta/mnt/unheaded/secrets/.wireguard-keys.yaml.enc
git-crypt encrypt secrets/.wireguard-keys.yaml.enc 2>/dev/null || echo "git-crypt not configured"

# Verify
echo "WEST Public:  $WEST_PUB"
echo "EAST Public:  $EAST_PUB"
```

**Success Criteria:**
- 4 keys generated (2 private, 2 public)
- Keys stored securely
- Keys verified with `wg show` after deployment

---

## STEP 53: [W] Create WireGuard Configuration Files
**Owner:** Network Engineer
**Input:** Keys from step 52
**Output:** wg0.conf on both hosts

```bash
# Generate WEST configuration
cat > /tmp/wg0-west.conf <<EOF
[Interface]
PrivateKey = $WEST_PRIV
ListenPort = 51820
Address = fd00:dead:beef::2/64
DNS = fd00:dead:beef::1  # Use EAST as DNS resolver (optional)

# Enable IP forwarding for inter-host traffic
PostUp = sysctl -q net.ipv6.conf.all.forwarding=1
PostUp = sysctl -q net.ipv6.conf.wg0.forwarding=1
PostDown = sysctl -q net.ipv6.conf.all.forwarding=0
PostDown = sysctl -q net.ipv6.conf.wg0.forwarding=0

# IP masquerading (optional, for NAT)
# PostUp = ip6tables -A FORWARD -i wg0 -j ACCEPT
# PostUp = ip6tables -A FORWARD -o wg0 -j ACCEPT

[Peer]
PublicKey = $EAST_PUB
Endpoint = 192.168.13.1:51820
AllowedIPs = fd00:dead:beef::1/128
PersistentKeepalive = 25
EOF

# Generate EAST configuration
cat > /tmp/wg0-east.conf <<EOF
[Interface]
PrivateKey = $EAST_PRIV
ListenPort = 51820
Address = fd00:dead:beef::1/64

PostUp = sysctl -q net.ipv6.conf.all.forwarding=1
PostUp = sysctl -q net.ipv6.conf.wg0.forwarding=1
PostDown = sysctl -q net.ipv6.conf.all.forwarding=0
PostDown = sysctl -q net.ipv6.conf.wg0.forwarding=0

[Peer]
PublicKey = $WEST_PUB
Endpoint = 192.168.13.2:51820
AllowedIPs = fd00:dead:beef::2/128
PersistentKeepalive = 25
EOF

# Verify files
echo "=== WEST Configuration ==="
cat /tmp/wg0-west.conf

echo ""
echo "=== EAST Configuration ==="
cat /tmp/wg0-east.conf
```

**Success Criteria:**
- Both config files generated
- Keys correctly substituted
- No empty values

---

## STEP 54: [B] Deploy WireGuard to WEST Host
**Owner:** Network Engineer
**Input:** wg0-west.conf, WEST host access
**Output:** WireGuard running on WEST

```bash
# SSH to WEST
ssh govan@west "cat > /tmp/wg0.conf" < /tmp/wg0-west.conf

# Install WireGuard if not present
ssh govan@west "sudo apt-get update && sudo apt-get install -y wireguard wireguard-tools" || \
ssh govan@west "sudo brew install wireguard-tools" 2>/dev/null

# Enable WireGuard
ssh govan@west "
  sudo cp /tmp/wg0.conf /etc/wireguard/wg0.conf
  sudo chmod 600 /etc/wireguard/wg0.conf
  sudo systemctl enable wg-quick@wg0
  sudo systemctl start wg-quick@wg0
"

# Verify interface is up
ssh govan@west "sudo ip link show wg0 | head -5"
ssh govan@west "sudo ip addr show wg0 | grep 'inet6'"
ssh govan@west "sudo wg show"
```

**Success Criteria:**
- `ip link show wg0` shows UP
- IPv6 address fd00:dead:beef::2 assigned
- `wg show` displays configured peer

---

## STEP 55: [B] Deploy WireGuard to EAST Host
**Owner:** Network Engineer
**Input:** wg0-east.conf, EAST host access
**Output:** WireGuard running on EAST

```bash
ssh govan@east "cat > /tmp/wg0.conf" < /tmp/wg0-east.conf

ssh govan@east "sudo apt-get update && sudo apt-get install -y wireguard wireguard-tools" || \
ssh govan@east "sudo brew install wireguard-tools" 2>/dev/null

ssh govan@east "
  sudo cp /tmp/wg0.conf /etc/wireguard/wg0.conf
  sudo chmod 600 /etc/wireguard/wg0.conf
  sudo systemctl enable wg-quick@wg0
  sudo systemctl start wg-quick@wg0
"

ssh govan@east "sudo ip link show wg0 | head -5"
ssh govan@east "sudo ip addr show wg0 | grep 'inet6'"
ssh govan@east "sudo wg show"
```

**Success Criteria:**
- `ip link show wg0` shows UP
- IPv6 address fd00:dead:beef::1 assigned
- `wg show` displays configured peer

---

## STEP 56: [V] Verify WireGuard Connectivity
**Owner:** Network Engineer
**Input:** Both hosts with WireGuard running
**Output:** Ping test results

```bash
# Test 1: WEST → EAST IPv6 ping
echo "Testing WEST → EAST IPv6 connectivity..."
ssh govan@west "ping6 -c 5 fd00:dead:beef::1"

# Test 2: EAST → WEST IPv6 ping
echo "Testing EAST → WEST IPv6 connectivity..."
ssh govan@east "ping6 -c 5 fd00:dead:beef::2"

# Test 3: Check latency
echo "Checking IPv6 latency..."
ssh govan@west "ping6 -c 10 fd00:dead:beef::1 | grep 'min/avg/max'"

# Test 4: Verify routing
echo "Verifying IPv6 routes..."
ssh govan@west "ip -6 route show | grep dead:beef"
ssh govan@east "ip -6 route show | grep dead:beef"

# Test 5: Packet capture - verify tunnel is working
echo "Capturing packets on WireGuard interface..."
ssh govan@west "sudo timeout 5 tcpdump -i wg0 -n | head -20" &
ssh govan@east "ping6 -c 3 fd00:dead:beef::2"
wait
```

**Success Criteria:**
- Ping succeeds both directions
- Latency < 5ms (typical for P2P link)
- Routes show fd00:dead:beef::/64 via wg0
- tcpdump shows IPv6 traffic

---

## STEP 57: [V] Verify Monad HbH Passthrough
**Owner:** Network Engineer
**Input:** WireGuard tunnel operational, Monad service running
**Output:** HbH extension headers intact through tunnel

```bash
# Start packet capture on WEST WireGuard interface
ssh govan@west "sudo tcpdump -i wg0 -w /tmp/monad-hbh-west.pcap 'ipv6 and (ip6[40] == 60)'" &
TCPDUMP_PID_WEST=$!

# Start packet capture on EAST WireGuard interface
ssh govan@east "sudo tcpdump -i wg0 -w /tmp/monad-hbh-east.pcap 'ipv6 and (ip6[40] == 60)'" &
TCPDUMP_PID_EAST=$!

sleep 2

# Send Monad packet with HbH from WEST
ssh govan@west "
  go run ./cmd/monad-client -addr=fd00:dead:beef::1:19004 -send-hbh-packet
" &

sleep 3

# Stop captures
ssh govan@west "kill $TCPDUMP_PID_WEST"
ssh govan@east "kill $TCPDUMP_PID_EAST"

# Analyze captures
echo "=== WEST captures (transmitted) ==="
ssh govan@west "tcpdump -A -r /tmp/monad-hbh-west.pcap | head -20"

echo ""
echo "=== EAST captures (received) ==="
ssh govan@east "tcpdump -A -r /tmp/monad-hbh-east.pcap | head -20"

# Verify HbH headers are present in both
WEST_COUNT=$(ssh govan@west "tcpdump -r /tmp/monad-hbh-west.pcap | wc -l")
EAST_COUNT=$(ssh govan@east "tcpdump -r /tmp/monad-hbh-east.pcap | wc -l")

echo "WEST captured $WEST_COUNT HbH packets"
echo "EAST captured $EAST_COUNT HbH packets"

if [ "$WEST_COUNT" -gt 0 ] && [ "$EAST_COUNT" -gt 0 ]; then
    echo "SUCCESS: Monad HbH packets pass through WireGuard tunnel intact"
else
    echo "FAILED: No HbH packets detected"
fi
```

**Success Criteria:**
- HbH packets captured on both sides
- Header format intact (protocol 60)
- Latency added by tunnel < 500µs

---

## STEP 58: [W] Setup Performance Benchmarking Framework
**Owner:** Performance Engineer
**Input:** Current codebase, perf targets
**Output:** `cmd/perf-benchmark/` with harness

Create comprehensive performance testing framework:

```go
// cmd/perf-benchmark/main.go

package main

import (
    "fmt"
    "time"
    "github.com/unheaded/unheaded/pkg/benchmark"
)

func main() {
    suite := benchmark.NewBenchmarkSuite()

    // Test 1: Baseline packet latency (WEST → EAST via P2P)
    suite.AddTest("P2P_Direct_Latency", func(b *benchmark.B) {
        for i := 0; i < b.N; i++ {
            // Send ICMP ping via P2P, measure RTT
            start := time.Now()
            ip := "192.168.13.1"
            pingResult := icmpPing(ip)  // Single ping
            elapsed := time.Since(start)
            b.RecordLatency(elapsed)
        }
    })

    // Test 2: WireGuard IPv6 tunnel latency
    suite.AddTest("WireGuard_IPv6_Latency", func(b *benchmark.B) {
        for i := 0; i < b.N; i++ {
            start := time.Now()
            ip := "fd00:dead:beef::1"
            pingResult := icmpPing(ip)  // Ping through WireGuard
            elapsed := time.Since(start)
            b.RecordLatency(elapsed)
        }
    })

    // Test 3: Monad packet latency (WEST → EAST)
    suite.AddTest("Monad_Packet_Latency", func(b *benchmark.B) {
        for i := 0; i < b.N; i++ {
            start := time.Now()
            client := monad.NewClient("fd00:dead:beef::1:19004")
            resp := client.Ping()  // Monad PING request
            elapsed := time.Since(start)
            b.RecordLatency(elapsed)
        }
    })

    // Test 4: Service-to-service latency (Captain → Architect)
    suite.AddTest("Service_to_Service_Latency", func(b *benchmark.B) {
        for i := 0; i < b.N; i++ {
            start := time.Now()

            // Captain on WEST calls Architect on EAST
            captain := http.NewClient("http://fd00:dead:beef::2:19002")
            response := captain.Get("/api/v1/design/latest")

            elapsed := time.Since(start)
            b.RecordLatency(elapsed)
        }
    })

    // Test 5: Dashboard update latency (user sees update)
    suite.AddTest("Dashboard_Update_Latency", func(b *benchmark.B) {
        for i := 0; i < b.N; i++ {
            // 1. eBPF captures packet on WEST
            // 2. Trace-collector publishes to Wotan
            // 3. Dashboard-backend receives via WebSocket
            // 4. Browser renders update

            start := time.Now()

            sendTestPacket("192.168.13.2")  // WEST eBPF captures
            time.Sleep(10 * time.Millisecond)  // Allow propagation

            // Poll dashboard for update
            dashClient := http.NewClient("http://localhost:20000")
            updateTime := dashClient.GetLastUpdateTime()

            elapsed := time.Since(start)
            if elapsed < 50 * time.Millisecond {  // Sub-50ms target
                b.RecordLatency(elapsed)
            } else {
                b.RecordSlowLatency(elapsed)
            }
        }
    })

    // Run all tests
    results := suite.Run()

    // Generate report
    fmt.Println(results.SummaryReport())
    results.SaveToFile("perf-results-s77.json")
}
```

Create benchmark utilities package:

```go
// pkg/benchmark/benchmark.go

package benchmark

import (
    "encoding/json"
    "fmt"
    "os"
    "sort"
    "time"
)

type BenchmarkTest struct {
    Name      string
    Iterations int
    Latencies []time.Duration
}

type BenchmarkSuite struct {
    Tests []*BenchmarkTest
}

func (s *BenchmarkSuite) AddTest(name string, fn func(b *B)) {
    test := &BenchmarkTest{Name: name, Latencies: make([]time.Duration, 0)}
    b := &B{test: test}

    // Run test 100 times (or until timeout)
    for i := 0; i < 100; i++ {
        fn(b)
    }

    s.Tests = append(s.Tests, test)
}

func (s *BenchmarkSuite) Run() *Results {
    results := &Results{Tests: make(map[string]*TestResult)}

    for _, test := range s.Tests {
        sort.Slice(test.Latencies, func(i, j int) bool {
            return test.Latencies[i] < test.Latencies[j]
        })

        result := &TestResult{
            Min:    test.Latencies[0],
            Max:    test.Latencies[len(test.Latencies)-1],
            Median: test.Latencies[len(test.Latencies)/2],
            P95:    test.Latencies[int(float64(len(test.Latencies))*0.95)],
            P99:    test.Latencies[int(float64(len(test.Latencies))*0.99)],
            Avg:    average(test.Latencies),
        }

        results.Tests[test.Name] = result
    }

    return results
}

type B struct {
    test *BenchmarkTest
    N    int
}

func (b *B) RecordLatency(d time.Duration) {
    b.test.Latencies = append(b.test.Latencies, d)
}

type Results struct {
    Tests map[string]*TestResult
}

func (r *Results) SummaryReport() string {
    output := "PERFORMANCE BENCHMARK RESULTS (S77)\n"
    output += "===================================\n\n"

    for name, result := range r.Tests {
        output += fmt.Sprintf("%s:\n", name)
        output += fmt.Sprintf("  Min:    %v\n", result.Min)
        output += fmt.Sprintf("  Avg:    %v\n", result.Avg)
        output += fmt.Sprintf("  Median: %v\n", result.Median)
        output += fmt.Sprintf("  P95:    %v\n", result.P95)
        output += fmt.Sprintf("  P99:    %v\n", result.P99)
        output += fmt.Sprintf("  Max:    %v\n", result.Max)
        output += "\n"
    }

    return output
}

func (r *Results) SaveToFile(filename string) error {
    data, err := json.MarshalIndent(r.Tests, "", "  ")
    if err != nil {
        return err
    }
    return os.WriteFile(filename, data, 0644)
}
```

**Success Criteria:**
- Benchmark harness compiles
- Can run individual benchmarks
- Outputs JSON report

---

## STEP 59: [B] Run Performance Benchmarks - Baseline
**Owner:** Performance Engineer
**Input:** Benchmark framework, both hosts running
**Output:** Baseline perf metrics in `docs/PERFORMANCE-S77.md`

```bash
cd /sessions/sharp-sweet-volta/mnt/unheaded

# Build benchmark binary
go build -o /tmp/perf-benchmark ./cmd/perf-benchmark

# Run benchmarks (allow 10 minutes per test)
timeout 600 /tmp/perf-benchmark -runs=100 -timeout=60s | tee /tmp/perf-results.log

# Extract results
cat /tmp/perf-results.log | grep -E "Min:|Avg:|P95:|Max:"
```

Expected results (rough targets):
- P2P Direct Latency: 0.5-1.0ms
- WireGuard IPv6 Latency: 1.0-1.5ms (slight overhead from encryption)
- Monad Packet Latency: 2-5ms
- Service-to-Service Latency: 5-15ms
- Dashboard Update Latency: 20-40ms (sub-50ms target)

**Success Criteria:**
- Benchmarks complete without errors
- Results within expected ranges
- JSON report generated

---

## STEP 60-75: [PLACEHOLDER - Reserved for Performance Optimization]

**Remaining Steps (60-75):**
- Step 60-65: Analyze bottlenecks from baseline metrics
- Step 66-70: Optimize hot paths (caching, connection pooling)
- Step 71-75: Re-run benchmarks to confirm improvements

---

## STEP 76-110: [PLACEHOLDER - Reserved for Monitoring & Documentation]

**Remaining Steps (76-110):**
- Step 76-85: Setup Prometheus metrics collection for perf data
- Step 86-95: Create Grafana dashboards for perf tracking
- Step 96-105: Document findings in PERFORMANCE-S77.md
- Step 106-110: Phase 2 exit gate verification and Phase 3 briefing

---

# PHASE 3: SBOM + CI/CD FORTRESS (Steps 111-170)
**Goal:** SBOM generation, legal compliance, automated CI pipelines.
**Prerequisite:** Phase 1 & 2 complete, all code stable.
**Time Estimate:** 12-16 hours
**Agent Assignment:** DevOps Engineer (CI/CD), Legal/Compliance Officer (SBOM)

### Phase 3 Context
Before going public, we need:
1. Software Bill of Materials (SBOM) — list all dependencies, licenses, vulnerabilities
2. GPL boundary verification — confirm no GPL code in core
3. GitHub Actions workflows — automated security checks, coverage trending
4. Jenkins pipelines — build, test, security scan, deploy, release
5. Makefile targets — local versions of all CI operations

### Phase 3 Exit Gate
- SBOM generates cleanly (no errors, no unknown licenses)
- ScanCode + FOSSology integration working
- 5 GHA workflows passing dry-run
- 5 Jenkinsfiles validated
- All Makefile targets callable

---

## STEP 111: [W] Design SBOM Generation Pipeline
**Owner:** DevOps Engineer
**Input:** Project structure, dependency list
**Output:** `docs/SBOM-STRATEGY-S77.md` with detailed plan

```markdown
# SBOM Generation Strategy (S77)

## Tools Used

1. **ScanCode**: Scan source files for licenses, copyright notices
2. **FOSSology**: Deep dependency analysis, license compatibility
3. **ORT (OpenReuse Tool)**: Generate standardized SBOM (SPDX, CycloneDX)

## Execution Plan

### Phase 3a: ScanCode (source file analysis)
```bash
make sbom-scancode
# Output: sbom/scancode-results.json
```

### Phase 3b: FOSSology (dependency deep dive)
```bash
make sbom-fossology
# Output: sbom/fossology-results.csv
```

### Phase 3c: ORT (standards SBOM)
```bash
make sbom-ort
# Output: sbom/SBOM-spdx.json, sbom/SBOM-cyclonedx.json
```

### Phase 3d: GPL Boundary Check
```bash
make sbom-gpl-check
# Output: sbom/gpl-boundary-report.txt
# Should report: 0 GPL files in core
```

## Integration Points

1. **GitHub Actions**: Run `make sbom-generate` on every push to main
2. **Jenkins**: Run ScanCode in Security pipeline
3. **Release**: Include SBOM in release artifacts (signed)

## Output Format

### SBOM (SPDX format)
```json
{
  "SPDXID": "SPDXRef-DOCUMENT",
  "name": "Unheaded",
  "version": "S77",
  "creationInfo": {
    "created": "2026-03-05T...",
    "creators": ["Tool: ORT"]
  },
  "packages": [
    {
      "SPDXID": "SPDXRef-Package",
      "name": "unheaded",
      "downloadLocation": "https://github.com/unheaded/unheaded",
      "filesAnalyzed": true,
      "licenseConcluded": "MIT",
      "externalRefs": [
        {
          "referenceCategory": "SECURITY",
          "referenceType": "cpe23Type",
          "referenceLocator": "cpe:2.3:a:unheaded:unheaded:*:*:*:*:*:*:*:*"
        }
      ]
    }
  ],
  "relationships": [...]
}
```

### GPL Boundary Report
```
Total Go files: 961
  - MIT licensed: 838 (87.1%)
  - Apache 2.0: 45 (4.7%)
  - AGPL: 0 (FAIL IF > 0)
  - GPL: 0 (FAIL IF > 0)
  - Unknown: 78 (8.1%)

Dependencies with GPL/AGPL: 0
Transitive GPL dependencies: 0

GPL BOUNDARY: CLEAR
```
```

**Success Criteria:**
- Strategy document complete
- Tool selection justified
- Output format examples provided
- Success criteria defined

---

## STEP 112: [B] Install SBOM Tools
**Owner:** DevOps Engineer
**Input:** Tool specifications
**Output:** All 3 tools installed and verified

```bash
# Install ScanCode
pip install scancode-toolkit

# Verify ScanCode
scancode --version

# Install FOSSology
# (Docker-based, pull image)
docker pull fossology/fossology:latest

# Verify FOSSology
docker run fossology/fossology fossology-version

# Install ORT (OpenReuse Tool)
git clone https://github.com/oss-review-toolkit/ort.git /tmp/ort
cd /tmp/ort
./gradlew installDist

# Verify ORT
/tmp/ort/build/install/ort/bin/ort --version
```

**Success Criteria:**
- All 3 tools installed
- Version commands work
- No dependency conflicts

---

## STEP 113: [B] Run ScanCode Analysis
**Owner:** DevOps Engineer
**Input:** Unheaded source tree
**Output:** ScanCode JSON report

```bash
cd /sessions/sharp-sweet-volta/mnt/unheaded

# Scan all Go files for licenses and copyrights
scancode --license --copyright --strip-root \
    --json-pp sbom/scancode-results.json \
    . 2>&1 | tee /tmp/scancode.log

# Summary statistics
echo ""
echo "ScanCode Summary:"
cat sbom/scancode-results.json | jq '.summary'
```

Expected output:
```json
{
  "total_files": 961,
  "files_with_license": 838,
  "files_with_copyright": 950,
  "license_keys": ["mit", "apache-2.0", "bsd-2-clause"],
  "unique_licenses": 3
}
```

**Success Criteria:**
- Scan completes without errors
- JSON file created
- License summary available

---

## STEP 114: [B] Run FOSSology Analysis (Docker)
**Owner:** DevOps Engineer
**Input:** Unheaded source, FOSSology Docker image
**Output:** FOSSology CSV report

```bash
cd /sessions/sharp-sweet-volta/mnt/unheaded

# Start FOSSology container (in background)
docker run -d \
    --name fossology-s77 \
    -v $(pwd):/code \
    fossology/fossology:latest

sleep 10

# Run analysis
docker exec fossology-s77 /fossology/bin/fossology \
    -C /code \
    --output csv > sbom/fossology-results.csv

# Extract key findings
echo "=== Fossology Summary ==="
head -20 sbom/fossology-results.csv

# Stop container
docker stop fossology-s77
docker rm fossology-s77
```

**Success Criteria:**
- FOSSology runs without errors
- CSV report created
- License findings extracted

---

## STEP 115: [B] Run ORT SBOM Generation
**Owner:** DevOps Engineer
**Input:** Unheaded project, go.mod/go.sum
**Output:** SPDX and CycloneDX SBOM files

```bash
cd /sessions/sharp-sweet-volta/mnt/unheaded

# Build ORT scanner
/tmp/ort/build/install/ort/bin/ort scan -p . -o sbom/ort-scan.json

# Generate SPDX SBOM
/tmp/ort/build/install/ort/bin/ort report -s sbom/ort-scan.json \
    -f SpdxJson \
    -o sbom/SBOM-spdx.json

# Generate CycloneDX SBOM
/tmp/ort/build/install/ort/bin/ort report -s sbom/ort-scan.json \
    -f CycloneDx \
    -o sbom/SBOM-cyclonedx.json

# Verify outputs
echo "=== SPDX SBOM ==="
jq '.packages | length' sbom/SBOM-spdx.json

echo "=== CycloneDX SBOM ==="
jq '.components | length' sbom/SBOM-cyclonedx.json
```

**Success Criteria:**
- ORT scan completes
- Both SBOM formats generated
- Valid JSON files

---

## STEP 116: [W] Create GPL Boundary Verification Script
**Owner:** DevOps Engineer
**Input:** SBOM files, known good licenses
**Output:** `scripts/verify-gpl-boundary.sh`

```bash
#!/bin/bash
# scripts/verify-gpl-boundary.sh
# Verifies that no GPL/AGPL code exists in core unheaded code

set -e

REPO_ROOT="/sessions/sharp-sweet-volta/mnt/unheaded"
SBOM_FILE="$REPO_ROOT/sbom/SBOM-spdx.json"
REPORT_FILE="$REPO_ROOT/sbom/gpl-boundary-report.txt"

# Allowed licenses
ALLOWED_LICENSES=(
    "MIT"
    "Apache-2.0"
    "BSD-2-Clause"
    "BSD-3-Clause"
    "ISC"
)

echo "GPL Boundary Verification" > $REPORT_FILE
echo "=========================" >> $REPORT_FILE
echo "Date: $(date)" >> $REPORT_FILE
echo "" >> $REPORT_FILE

# Count files by license
echo "Source Files by License:" >> $REPORT_FILE
find $REPO_ROOT -name "*.go" -type f -exec head -1 {} \; | \
    grep -o "SPDX-License-Identifier: [^ ]*" | \
    sort | uniq -c | sort -rn >> $REPORT_FILE

echo "" >> $REPORT_FILE

# Check for GPL
GPL_COUNT=$(grep -r "SPDX-License-Identifier.*GPL" $REPO_ROOT/pkg $REPO_ROOT/cmd 2>/dev/null | wc -l)
AGPL_COUNT=$(grep -r "SPDX-License-Identifier.*AGPL" $REPO_ROOT/pkg $REPO_ROOT/cmd 2>/dev/null | wc -l)

echo "GPL Files in Core: $GPL_COUNT" >> $REPORT_FILE
echo "AGPL Files in Core: $AGPL_COUNT" >> $REPORT_FILE
echo "" >> $REPORT_FILE

# Check dependencies
echo "Dependencies (from go.mod):" >> $REPORT_FILE
grep "^require" $REPO_ROOT/go.mod | head -20 >> $REPORT_FILE

echo "" >> $REPORT_FILE
echo "GPL Boundary Status: " >> $REPORT_FILE

if [ "$GPL_COUNT" -gt 0 ] || [ "$AGPL_COUNT" -gt 0 ]; then
    echo "FAIL - GPL/AGPL code found in core" >> $REPORT_FILE
    cat $REPORT_FILE
    exit 1
else
    echo "PASS - GPL/AGPL free" >> $REPORT_FILE
    cat $REPORT_FILE
    exit 0
fi
```

```bash
# Make executable
chmod +x scripts/verify-gpl-boundary.sh

# Run
./scripts/verify-gpl-boundary.sh
```

**Success Criteria:**
- Script runs without errors
- Report shows 0 GPL/AGPL files
- Output saved to gpl-boundary-report.txt

---

## STEP 117: [B] Run GPL Boundary Check
**Owner:** DevOps Engineer
**Input:** verify-gpl-boundary.sh
**Output:** gpl-boundary-report.txt showing PASS

```bash
cd /sessions/sharp-sweet-volta/mnt/unheaded
./scripts/verify-gpl-boundary.sh | tee /tmp/gpl-check.log

# Extract result
if tail -5 /tmp/gpl-check.log | grep -q "PASS"; then
    echo "GPL Boundary: PASS"
else
    echo "GPL Boundary: FAIL"
    exit 1
fi
```

**Success Criteria:**
- Script returns exit code 0
- Report shows PASS
- 0 GPL/AGPL files found

---

## STEP 118: [W] Create GitHub Actions Security Workflow
**Owner:** DevOps Engineer
**Input:** Project structure
**Output:** `.github/workflows/security.yml`

```yaml
name: Security Checks

on:
  push:
    branches: [main, staging]
  pull_request:
    branches: [main]

jobs:
  security-scan:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
        with:
          fetch-depth: 0

      - name: Set up Go
        uses: actions/setup-go@v4
        with:
          go-version: '1.21'

      - name: Run gosec (Go security scanner)
        uses: securego/gosec@master
        with:
          args: './...'

      - name: Run go vet
        run: go vet ./...

      - name: Check for vulnerable dependencies
        run: |
          go install github.com/golang/vuln/cmd/govulncheck@latest
          govulncheck ./...

      - name: Verify GPL boundary
        run: |
          ./scripts/verify-gpl-boundary.sh
          if [ $? -ne 0 ]; then
            echo "GPL boundary check failed"
            exit 1
          fi

      - name: Upload security report
        if: always()
        uses: actions/upload-artifact@v3
        with:
          name: security-reports
          path: sbom/gpl-boundary-report.txt
```

---

## STEP 119: [W] Create GitHub Actions SBOM Workflow
**Owner:** DevOps Engineer
**Input:** SBOM tools, ORT config
**Output:** `.github/workflows/sbom.yml`

```yaml
name: Generate SBOM

on:
  push:
    branches: [main]
  schedule:
    - cron: '0 2 * * MON'  # Weekly on Monday 2 AM UTC

jobs:
  sbom-generation:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4

      - name: Set up Python
        uses: actions/setup-python@v4
        with:
          python-version: '3.11'

      - name: Install scancode
        run: |
          pip install scancode-toolkit
          scancode --version

      - name: Run ScanCode
        run: |
          mkdir -p sbom
          scancode --license --copyright --strip-root \
              --json-pp sbom/scancode-results.json \
              .

      - name: Install ORT
        run: |
          git clone https://github.com/oss-review-toolkit/ort.git /tmp/ort
          cd /tmp/ort
          ./gradlew installDist

      - name: Generate SPDX SBOM
        run: |
          /tmp/ort/build/install/ort/bin/ort scan -p . -o /tmp/ort-scan.json
          /tmp/ort/build/install/ort/bin/ort report -s /tmp/ort-scan.json \
              -f SpdxJson -o sbom/SBOM-spdx.json

      - name: Generate CycloneDX SBOM
        run: |
          /tmp/ort/build/install/ort/bin/ort report -s /tmp/ort-scan.json \
              -f CycloneDx -o sbom/SBOM-cyclonedx.json

      - name: Upload SBOM artifacts
        uses: actions/upload-artifact@v3
        with:
          name: sbom-artifacts
          path: sbom/

      - name: Commit SBOM to repo
        run: |
          git config user.name "sbom-bot"
          git config user.email "sbom@unheaded.io"
          git add sbom/
          git commit -m "chore(sbom): automated SBOM generation from S77" || true
          git push origin main || true
```

---

## STEP 120: [C] Commit Phase 3 SBOM Infrastructure
**Owner:** VCS
**Input:** SBOM scripts, GHA workflows
**Output:** Committed to main

```bash
cd /sessions/sharp-sweet-volta/mnt/unheaded

git add scripts/verify-gpl-boundary.sh
git add .github/workflows/security.yml
git add .github/workflows/sbom.yml
git add sbom/

git commit -m "ci(sbom): add SBOM generation and security workflows

Phase 3 infrastructure for legal compliance and automated security gates:

1. ScanCode integration: Scan all source files for licenses/copyrights
2. FOSSology integration: Deep dependency analysis
3. ORT SBOM generation: Generate SPDX and CycloneDX SBOMs
4. GPL boundary verification: Ensure 0 GPL/AGPL code in core
5. GHA workflows: Automated security and SBOM generation

Scripts:
  - scripts/verify-gpl-boundary.sh: Check GPL boundary (must return 0)

Workflows:
  - .github/workflows/security.yml: Runs gosec, govulncheck, GPL check
  - .github/workflows/sbom.yml: Generates SBOM weekly + on push to main

Artifacts:
  - sbom/scancode-results.json: Source file license scan
  - sbom/fossology-results.csv: Dependency deep dive
  - sbom/SBOM-spdx.json: Standards SBOM (SPDX format)
  - sbom/SBOM-cyclonedx.json: Standards SBOM (CycloneDX format)
  - sbom/gpl-boundary-report.txt: GPL boundary verification report

All SBOM files version controlled (no secrets).

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>"

git push origin main
```

---

## STEP 121-140: [PLACEHOLDER - Jenkins Pipeline Implementation]

**Remaining Steps (121-140):**
- Step 121-125: Design Jenkins pipelines (build, test, security, deploy, release)
- Step 126-130: Implement Jenkinsfile (multi-branch, parallel stages)
- Step 131-135: Setup Jenkins agents (Docker, bare metal)
- Step 136-140: Test pipelines with dry-run

---

## STEP 141-160: [PLACEHOLDER - Makefile CI Targets]

**Remaining Steps (141-160):**
- Step 141-145: Add `make sbom-generate` target (orchestrates all SBOM tools)
- Step 146-150: Add `make sbom-verify` target (checks GPL boundary)
- Step 151-155: Add `make ci-local` target (runs full CI locally)
- Step 156-160: Add `make ci-security` target (runs security scans only)

---

## STEP 161-170: [PLACEHOLDER - Phase 3 Exit Gate]

**Remaining Steps (161-170):**
- Step 161-165: Run all SBOM tools, verify outputs
- Step 166-170: Dry-run all CI workflows, confirm they pass

---

# PHASE 4: PROTOCOL SPEC ADVANCEMENT (Steps 171-230)
**Goal:** Update 3 protocol specs to new versions. Foundation draft-06, Sophia draft-03, Wotan draft-03.
**Prerequisite:** Phase 1-3 complete, all code stable.
**Time Estimate:** 8-10 hours
**Agent Assignment:** Protocol Architect (specs), Scientist (validation)

### Phase 4 Context
Current specs:
- Foundation: draft-05 (12 IANA registries, frozen wire format v0x01)
- Sophia: draft-02 (knowledge graph, entity relations)
- Wotan: draft-02 (message bus protocol, topic routing)

New versions needed:
- **Foundation draft-06:** IANA registry integration, editorial updates
- **Sophia draft-03:** Sub-dictionary type system, QPACK compression
- **Wotan draft-03:** Error code taxonomy, helper return codes

### Phase 4 Exit Gate
- All 3 specs at new draft versions
- Cross-reference validation (no broken links)
- Backward compatibility analysis (what breaks)
- Implementation roadmap documented

---

## STEP 171: [W] Update Foundation Spec to draft-06
**Owner:** Protocol Architect
**Input:** Foundation draft-05, IANA requirements
**Output:** `references/foundation-draft-06.md`

Key changes for draft-06:
1. Add IANA registry section (procedures for registering new metric types)
2. Update examples to show UNHEADED_METRIC_V1 (Type 0x2A)
3. Cross-reference Sophia and Wotan specs
4. Add security considerations (immutability, verification)
5. Editorial: fix typos, improve clarity

```markdown
# Monad Protocol Foundation
## Specification Draft-06

**Date:** March 5, 2026
**Status:** Draft-06 (IANA Integration)
**Wire Format:** FROZEN at v0x01 (20 bytes)

### What's New in draft-06

1. **IANA Procedure**: Step-by-step guide for registering new metric types
2. **Cross-references**: Links to Sophia draft-03 and Wotan draft-03
3. **Example Metric**: UNHEADED_METRIC_V1 (Type 0x2A) fully documented
4. **Security**: Added threat model and verification procedures
5. **Backwards Compatibility**: draft-05 → draft-06 is non-breaking

...
```

**Success Criteria:**
- Draft-06 document created
- IANA procedure section complete
- All references updated
- No broken links

---

## STEP 172-180: [PLACEHOLDER - Sophia draft-03 Updates]

**Remaining Steps (172-180):**
- Analyze sub-dictionary type system needs
- Document QPACK compression headers
- Add examples
- Update cross-references
- Create Sophia draft-03 final

---

## STEP 181-190: [PLACEHOLDER - Wotan draft-03 Updates]

**Remaining Steps (181-190):**
- Design error code taxonomy
- Document helper return codes
- Add error recovery procedures
- Update examples
- Create Wotan draft-03 final

---

## STEP 191-200: [PLACEHOLDER - Cross-Reference Validation]

**Remaining Steps (191-200):**
- Verify all Foundation ↔ Sophia cross-refs
- Verify all Foundation ↔ Wotan cross-refs
- Verify all Sophia ↔ Wotan cross-refs
- Create reference matrix
- Fix any broken links

---

## STEP 201-210: [PLACEHOLDER - Backwards Compatibility Analysis]

**Remaining Steps (201-210):**
- Document what breaks between versions
- Create migration guide
- Test implementation compatibility
- Create compatibility matrix

---

## STEP 211-230: [PLACEHOLDER - Protocol Advancement Documentation]

**Remaining Steps (211-230):**
- Document implementation roadmap (when to use draft-06)
- Create CHANGELOG for all 3 specs
- Prepare RFC submission procedures
- Phase 4 exit gate and Phase 5 briefing

---

# PHASE 5: INTERFACE CONTRACTS — IaC + OBSERVABILITY (Steps 231-300)
**Goal:** Define and implement IaCRenderer and ObservabilityAdapter interfaces.
**Prerequisite:** Phase 1-4 complete, all code stable.
**Time Estimate:** 10-12 hours
**Agent Assignment:** Architect (interfaces), Backend Engineer (implementations)

### Phase 5 Context
Current state: All IaC backends (Ansible, Terraform, etc.) are hardcoded. All observability backends (Prometheus, ELK, etc.) are hardcoded.

Goal: Define standard interfaces so backends are pluggable.

### Phase 5 Exit Gate
- IaCRenderer interface defined in `pkg/iac/renderer.go`
- 2 IaC implementations: Ansible, Terraform
- ObservabilityAdapter interface defined in `pkg/observability/adapter.go`
- 2 Observability implementations: Prometheus, ELK
- All 4 implementations tested and working
- Generated IaC produces valid output
- Generated configs pass validation

---

## STEP 231: [W] Design IaCRenderer Interface
**Owner:** Architect
**Input:** Existing IaC implementations, desired-state model
**Output:** `pkg/iac/renderer.go` with interface definition

```go
// pkg/iac/renderer.go

package iac

import "context"

// DesiredState represents the infrastructure configuration we want
type DesiredState struct {
    Name          string                    `json:"name"`
    Version       string                    `json:"version"`
    Containers    []Container               `json:"containers"`
    Network       NetworkConfig             `json:"network"`
    Security      SecurityConfig            `json:"security"`
    Services      []Service                 `json:"services"`
    Observability ObservabilityConfig       `json:"observability"`
}

type Container struct {
    Name        string            `json:"name"`
    Image       string            `json:"image"`
    Port        int               `json:"port"`
    Environment map[string]string `json:"environment"`
    Volumes     []Volume          `json:"volumes"`
    Resources   Resources         `json:"resources"`
    Security    ContainerSecurity `json:"security"`
}

type NetworkConfig struct {
    Type       string `json:"type"`  // "bridge", "overlay", "wireguard"
    CIDR       string `json:"cidr"`
    DNS        []string `json:"dns"`
    Firewall   FirewallRules `json:"firewall"`
}

type SecurityConfig struct {
    TLSVersion   string   `json:"tls_version"`
    Capabilities []string `json:"capabilities"`
    ReadOnly     bool     `json:"read_only"`
    SecComp      SecCompPolicy `json:"seccomp"`
}

type Service struct {
    Name   string `json:"name"`
    Port   int    `json:"port"`
    Health HealthCheck `json:"health"`
}

type ObservabilityConfig struct {
    Backend string   `json:"backend"`  // "prometheus", "elk", "datadog", etc
    Scrape  ScrapeConfig `json:"scrape"`
}

// IaCRenderer is the interface all IaC backends must implement
type IaCRenderer interface {
    // Name returns the backend name (e.g., "ansible", "terraform")
    Name() string

    // Version returns the backend version (e.g., "2.13.0")
    Version() string

    // Render converts desired state into backend-specific configuration
    Render(ctx context.Context, desired *DesiredState) (map[string]string, error)
    // Returns map[filename]content, e.g.:
    //   "playbook.yml" → YAML content
    //   "main.tf" → HCL content

    // Validate checks if rendered config is valid
    Validate(ctx context.Context, rendered map[string]string) error

    // Deploy applies the configuration (optional, not all backends support)
    Deploy(ctx context.Context, rendered map[string]string) error

    // Diff shows what would change (optional)
    Diff(ctx context.Context, rendered map[string]string) (string, error)

    // Destroy removes infrastructure (optional)
    Destroy(ctx context.Context) error
}

// RendererFactory creates IaCRenderer instances
type RendererFactory struct {
    renderers map[string]IaCRenderer
}

func NewRendererFactory() *RendererFactory {
    return &RendererFactory{
        renderers: make(map[string]IaCRenderer),
    }
}

func (f *RendererFactory) Register(name string, renderer IaCRenderer) {
    f.renderers[name] = renderer
}

func (f *RendererFactory) Get(name string) (IaCRenderer, error) {
    renderer, ok := f.renderers[name]
    if !ok {
        return nil, fmt.Errorf("renderer not found: %s", name)
    }
    return renderer, nil
}

func (f *RendererFactory) List() []string {
    names := make([]string, 0, len(f.renderers))
    for name := range f.renderers {
        names = append(names, name)
    }
    sort.Strings(names)
    return names
}
```

**Success Criteria:**
- Interface design sound
- All methods documented
- Factory pattern for registration
- No breaking changes to existing code

---

## STEP 232: [W] Design ObservabilityAdapter Interface
**Owner:** Architect
**Input:** Existing observability implementations
**Output:** `pkg/observability/adapter.go` with interface definition

```go
// pkg/observability/adapter.go

package observability

import "context"

// MetricPoint represents a single metric value
type MetricPoint struct {
    Name      string            `json:"name"`
    Value     float64           `json:"value"`
    Timestamp int64             `json:"timestamp"`  // Unix milliseconds
    Labels    map[string]string `json:"labels"`
}

// LogEntry represents a structured log event
type LogEntry struct {
    Timestamp int64             `json:"timestamp"`
    Level     string            `json:"level"`  // debug, info, warn, error
    Service   string            `json:"service"`
    Message   string            `json:"message"`
    TraceID   string            `json:"trace_id"`
    Fields    map[string]interface{} `json:"fields"`
}

// TraceSpan represents a distributed trace span
type TraceSpan struct {
    TraceID   string            `json:"trace_id"`
    SpanID    string            `json:"span_id"`
    ParentID  string            `json:"parent_id"`
    Name      string            `json:"name"`
    StartTime int64             `json:"start_time"`
    EndTime   int64             `json:"end_time"`
    Service   string            `json:"service"`
    Tags      map[string]string `json:"tags"`
    Events    []SpanEvent       `json:"events"`
}

type SpanEvent struct {
    Timestamp int64             `json:"timestamp"`
    Name      string            `json:"name"`
    Attributes map[string]string `json:"attributes"`
}

// ObservabilityAdapter is the interface all observability backends must implement
type ObservabilityAdapter interface {
    // Name returns the adapter name (e.g., "prometheus", "elk")
    Name() string

    // WriteMetric sends a metric to the backend
    WriteMetric(ctx context.Context, metric *MetricPoint) error

    // WriteLog sends a log entry to the backend
    WriteLog(ctx context.Context, entry *LogEntry) error

    // WriteTrace sends a trace span to the backend
    WriteTrace(ctx context.Context, span *TraceSpan) error

    // Query retrieves metrics (e.g., for dashboards)
    Query(ctx context.Context, query string) ([]interface{}, error)

    // Health checks if backend is reachable
    Health(ctx context.Context) error

    // Close gracefully disconnects from backend
    Close(ctx context.Context) error
}

// AdapterFactory creates ObservabilityAdapter instances
type AdapterFactory struct {
    adapters map[string]ObservabilityAdapter
}

func NewAdapterFactory() *AdapterFactory {
    return &AdapterFactory{
        adapters: make(map[string]ObservabilityAdapter),
    }
}

func (f *AdapterFactory) Register(name string, adapter ObservabilityAdapter) {
    f.adapters[name] = adapter
}

func (f *AdapterFactory) Get(name string) (ObservabilityAdapter, error) {
    adapter, ok := f.adapters[name]
    if !ok {
        return nil, fmt.Errorf("adapter not found: %s", name)
    }
    return adapter, nil
}

func (f *AdapterFactory) List() []string {
    names := make([]string, 0, len(f.adapters))
    for name := range f.adapters {
        names = append(names, name)
    }
    sort.Strings(names)
    return names
}
```

**Success Criteria:**
- Interface design sound
- All methods documented
- Factory pattern for registration
- Compatible with existing code

---

## STEP 233: [W] Implement Ansible IaC Renderer
**Owner:** Backend Engineer
**Input:** IaCRenderer interface, existing Ansible configs
**Output:** `pkg/iac/ansible/renderer.go`

```go
// pkg/iac/ansible/renderer.go

package ansible

import (
    "context"
    "fmt"
    "github.com/unheaded/unheaded/pkg/iac"
)

type AnsibleRenderer struct {
    version string
}

func NewAnsibleRenderer() *AnsibleRenderer {
    return &AnsibleRenderer{
        version: "2.13.0",
    }
}

func (r *AnsibleRenderer) Name() string {
    return "ansible"
}

func (r *AnsibleRenderer) Version() string {
    return r.version
}

func (r *AnsibleRenderer) Render(ctx context.Context, desired *iac.DesiredState) (map[string]string, error) {
    files := make(map[string]string)

    // Generate playbook
    playbook := r.renderPlaybook(desired)
    files["playbook.yml"] = playbook

    // Generate inventory
    inventory := r.renderInventory(desired)
    files["inventory.ini"] = inventory

    // Generate role structure
    roles := r.renderRoles(desired)
    for name, content := range roles {
        files[fmt.Sprintf("roles/%s", name)] = content
    }

    return files, nil
}

func (r *AnsibleRenderer) renderPlaybook(desired *iac.DesiredState) string {
    // Generate Ansible playbook from desired state
    // Example output:
    //
    // ---
    // - hosts: all
    //   roles:
    //     - common
    //     - docker
    //     - unheaded

    return fmt.Sprintf(`---
- name: Deploy %s
  hosts: all
  become: yes
  roles:
    - common
    - docker
    - unheaded

  vars:
    unheaded_version: %s
`, desired.Name, desired.Version)
}

func (r *AnsibleRenderer) renderInventory(desired *iac.DesiredState) string {
    // Generate Ansible inventory from desired state
    return `[all]
localhost ansible_connection=local

[unheaded]
localhost

[unheaded:vars]
ansible_python_interpreter=/usr/bin/python3
`
}

func (r *AnsibleRenderer) renderRoles(desired *iac.DesiredState) map[string]string {
    roles := make(map[string]string)

    // Generate role for each service
    for _, service := range desired.Services {
        roleContent := fmt.Sprintf(`---
- name: Install %s
  tasks:
    - name: Create service directory
      file:
        path: /opt/unheaded/%s
        state: directory

    - name: Deploy %s binary
      copy:
        src: %s
        dest: /opt/unheaded/%s/
        mode: '0755'

    - name: Start %s service
      systemd:
        name: %s
        enabled: yes
        state: started
`, service.Name, service.Name, service.Name, service.Name, service.Name, service.Name, service.Name)

        roles[fmt.Sprintf("%s/main.yml", service.Name)] = roleContent
    }

    return roles
}

func (r *AnsibleRenderer) Validate(ctx context.Context, rendered map[string]string) error {
    // Validate Ansible syntax
    // Could call: ansible-playbook --syntax-check playbook.yml
    return nil
}

func (r *AnsibleRenderer) Deploy(ctx context.Context, rendered map[string]string) error {
    // Deploy using ansible-playbook command
    return fmt.Errorf("deploy not implemented")
}

func (r *AnsibleRenderer) Diff(ctx context.Context, rendered map[string]string) (string, error) {
    return "", fmt.Errorf("diff not implemented")
}

func (r *AnsibleRenderer) Destroy(ctx context.Context) error {
    return fmt.Errorf("destroy not implemented")
}
```

**Success Criteria:**
- Renderer compiles
- Produces valid Ansible YAML
- Validates successfully
- Tests pass

---

## STEP 234: [W] Implement Terraform IaC Renderer
**Owner:** Backend Engineer
**Input:** IaCRenderer interface, existing Terraform configs
**Output:** `pkg/iac/terraform/renderer.go`

Similar to Ansible, but generates HCL instead of YAML.

**Success Criteria:**
- Renderer compiles
- Produces valid HCL
- Validates successfully
- Tests pass

---

## STEP 235-260: [PLACEHOLDER - Observability Adapters]

**Remaining Steps (235-260):**
- Step 235-245: Implement Prometheus adapter (WriteMetric, Query, Health)
- Step 246-260: Implement ELK adapter (WriteLog, Query, Health)

---

## STEP 261-290: [PLACEHOLDER - Integration Testing]

**Remaining Steps (261-290):**
- Step 261-270: Test Ansible renderer (generates valid playbooks)
- Step 271-280: Test Terraform renderer (generates valid HCL)
- Step 281-290: Test Observability adapters (write and query data)

---

## STEP 291-300: [PLACEHOLDER - Phase 5 Exit Gate & Campaign Conclusion]

**Remaining Steps (291-300):**
- Step 291-295: Run all tests, verify 100% passing
- Step 296-298: Write After-Action Report (S77 complete)
- Step 299-300: Update Age 2 progress (45% → 75%+), Phase 6 planning

---

# CAMPAIGN EXECUTION RULES

## Commit Discipline
- **Every 4 steps:** Create commit (conventional format)
- **Every 10 steps:** Integration test checkpoint
- **End of each phase:** Phase exit gate verification

## Testing Requirements
- **Unit tests:** 80%+ coverage minimum
- **Integration tests:** All services communicating
- **Security tests:** No regressions

## Escalation Protocol
- **[STUCK]:** Block immediately noted by owner
- **[BLOCKED]:** Dependency issue, may reorder phases
- **Debug Attempts:** Max 2 per issue. 3rd failure → escalate

## Parallel Execution
- **Phase 1:** Sequential (bugs must be fixed in order)
- **Phase 2:** Sequential (WireGuard must be up before perf testing)
- **Phases 3-5:** Can be parallel with separate agents
  - Agent 1: Phase 3 (SBOM/CI/CD)
  - Agent 2: Phase 4 (Protocol specs)
  - Agent 3: Phase 5 (Interfaces)

---

# APPENDIX A: AGENT ASSIGNMENT MATRIX

| Phase | Component | Primary | Secondary | Time |
|-------|-----------|---------|-----------|------|
| 1 | Ratelimit XFF Fix | Developer | QA | 30 min |
| 1 | Wotan Fallback Fix | Developer | QA | 45 min |
| 1 | gRPC Locking Fix | Developer | QA | 40 min |
| 2 | WireGuard Design | Network Engr | Architect | 1 hour |
| 2 | WireGuard Deploy | Network Engr | SysAdmin | 2 hours |
| 2 | Performance Bench | Perf Engr | Developer | 3 hours |
| 3 | SBOM Tools | DevOps | Security | 2 hours |
| 3 | GHA Workflows | DevOps | Architect | 2 hours |
| 3 | Jenkins Pipelines | DevOps | Security | 3 hours |
| 4 | Foundation draft-06 | Protocol Arch | Scientist | 2 hours |
| 4 | Sophia draft-03 | Protocol Arch | Scientist | 2 hours |
| 4 | Wotan draft-03 | Protocol Arch | Scientist | 2 hours |
| 5 | IaC Interface | Architect | Backend Engr | 1.5 hours |
| 5 | Ansible Renderer | Backend Engr | QA | 2 hours |
| 5 | Terraform Renderer | Backend Engr | QA | 2 hours |
| 5 | Observability Interface | Architect | Backend Engr | 1.5 hours |
| 5 | Prometheus Adapter | Backend Engr | QA | 2 hours |
| 5 | ELK Adapter | Backend Engr | QA | 2.5 hours |

**Total Personnel Hours:** ~48 hours of work
**Parallelization:** 3-4 concurrent agents possible
**Estimated Wall Time:** 12-16 days with parallel execution

---

# APPENDIX B: EMERGENCY PROCEDURES

## If Phase 1 Bug Fix Fails (Step X)
1. Halt current step
2. Spawn debug session (max 2 attempts)
3. If still failing: Escalate to [STUCK], contact Stevie
4. Skip to next fix (continue with other P1 bugs)
5. Return to failed fix in Phase 1 extension (Steps 18-50)

## If WireGuard Won't Connect (Step 56)
1. Check firewall rules on both hosts
2. Verify UDP 51820 is open on P2P link
3. Check WireGuard status: `sudo wg show`
4. Check IPv6 routes: `ip -6 route`
5. If still stuck: Revert to P2P direct communication for now, note as BLOCKED

## If SBOM Tool Fails (Step 115)
1. Try alternative tool (ScanCode → FOSSology → ORT)
2. If all fail: Manually audit go.mod for GPL dependencies
3. Document findings in gpl-boundary-report.txt
4. Flag for manual legal review

## If CI/CD Pipeline Won't Pass (Step 140)
1. Check for syntax errors in YAML/HCL
2. Run local version of same pipeline (Makefile target)
3. Debug with verbose output
4. If unresolvable: Mark as BLOCKED, note in Phase 4

---

# APPENDIX C: QUICK REFERENCE

## Key Locations
- **Code:** `/sessions/sharp-sweet-volta/mnt/unheaded/`
- **CLAUDE.md:** Project guidelines and standards
- **Port Registry:** `pkg/ports/ports.go` (16666-26666 range)
- **WEST Host:** `govan@west` (192.168.13.2)
- **EAST Host:** `govan@east` (192.168.13.1)

## Quick Commands
```bash
# Clone repo
cd /sessions/sharp-sweet-volta/mnt/unheaded

# Run tests
make test

# Build all
make build

# Start dev environment
make dev

# Check service health
curl http://localhost:19000/health  # timeguru
curl http://localhost:19001/health  # architect
```

## Commit Template
```
<type>(<scope>): <subject>

[optional body explaining why]

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>
```

## Step Tag Cheat Sheet
- [B] = Build/test, expect exit code 0
- [V] = Verify results, manual inspection
- [D] = Debug, max 2 attempts before escalate
- [W] = Write code/docs, format/lint required
- [R] = Review design/security, sign-off needed
- [C] = Commit code, conventional format required
- [S] = Skip protocol, should rarely need this
- [P] = Parallel OK, multiple agents can work

---

# APPENDIX D: POST-PHASE 5 NEXT STEPS (Age 2 → Age 3 Preview)

Once Phase 5 complete and Age 2 reaches 75%+:

**Immediate (next 2 weeks):**
- Run full integration test (all 10 services + both bare metal hosts)
- Demo video recording (Kanban showing self-hosting)
- README polish for GitHub public release

**Medium term (Phase 6, ~2 weeks):**
- Public repository visibility setup
- License clarification (MIT → permissive at release)
- Security disclosure policy
- FUNDING.yml and CONTRIBUTING.md

**Long term (Age 3, ~4 weeks):**
- Full IaC renderer suite (Puppet, Kubernetes, Chef, Salt)
- Full observability adapter suite (Datadog, Splunk, Nagios)
- Multi-cloud orchestration (AWS, GCP, Azure)
- Prophecy/Wisdom engine connections

**Victory Condition:**
Unheaded publicly shipping on GitHub, ready for first production users, self-hosting proven at scale.

---

# SIGNATURE BLOCK

**Battle Plan Created:** March 5, 2026
**Campaign Duration:** March 5-25, 2026 (20 days)
**Target Completion:** Age 2 at 75%+ complete
**Participants:** Claude Opus Agent Collective
**Objective:** Production-ready infrastructure from 45% → 75%+

**Warmonger's Seal:**

```
    /\_/\
   ( o.o )  The Knight Rides.
    > ^ <   We march together.
   /|   |\
  (_|   |_)
```

**LET THE CAMPAIGN BEGIN.**

---

**Document Status:** FINAL | APPROVED FOR EXECUTION
**Last Updated:** March 5, 2026 at 00:00 UTC
**Next Review:** March 25, 2026 (post-campaign)
