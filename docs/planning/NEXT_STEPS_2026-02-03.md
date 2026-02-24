# Next Steps - February 3, 2026

**Based on:** Code churn session summary
**Reviewed against:** Timeguru, Micromanager, Architect skills + timeline.md
**Current State:** Build PASS | Tests PASS (0 failures) | Alpha ~99%

---

## EXECUTIVE SUMMARY

The session delivered solid infrastructure work: test stability improvements, new Monad/Sophia container definitions, and frontend API fixes. With tests passing and build green, the Kingdom is ready for the final push to Alpha (target: Feb 8).

---

## P0 - SHIP BLOCKERS (Do First)

### 1. Kanban E2E Smoke Test (Manual Verification)
*Owner: Micromanager QA | ETA: 1-2 hours*

The frontend fixes (PUT instead of PATCH, correct DELETE path) need validation with running services.

**Steps:**
```bash
# Start the stack
cd ~/tmp/unheaded
docker-compose up -d

# Wait for health checks to pass
docker-compose ps

# Open Kanban in browser
open http://localhost:8081  # or wherever kanban-app serves

# Test CRUD operations:
# 1. View task list (GET /api/v1/tasks)
# 2. Open task modal → view details
# 3. Edit task → verify PUT /api/v1/tasks/:id works
# 4. Delete task → verify DELETE /api/v1/tasks/:id works
# 5. Verify changes persist (refresh page)
```

**Success Criteria:**
- [ ] Tasks load from Timeguru
- [ ] Task modal opens with correct data
- [ ] Edit saves via PUT (no 404/405 errors)
- [ ] Delete removes task (no 404/405 errors)
- [ ] Changes reflect in timeline source

---

### 2. Dashboard Metrics Integration Test
*Owner: Developer | ETA: 1 hour*

Session enhanced `fetchMetrics()` to also fetch `/api/v1/health`. Verify the service health grid populates.

**Steps:**
```bash
# Start dashboard-backend
go run cmd/dashboard-backend/main.go

# Open dashboard
open http://localhost:8080

# Verify:
# - System metrics display (CPU, memory, etc.)
# - Service health grid shows status for all 8 services
# - WebSocket connection indicator shows "connected"
```

**Success Criteria:**
- [ ] Metrics endpoint returns data
- [ ] Health endpoint returns service statuses
- [ ] Frontend displays both correctly

---

## P1 - CRITICAL PATH (This Week)

### 3. Monad/Sophia Test Coverage
*Owner: Developer | ETA: 2-3 hours*

New services have NixOS containers and Docker Compose entries but need unit/integration tests.

**Files to test:**
- `services/monad/monad.go` → `services/monad/monad_test.go`
- `services/sophia/sophia.go` → `services/sophia/sophia_test.go`

**Test patterns (from existing services):**
```go
func TestMonadOperations(t *testing.T) {
    // Test functional composition operations
}

func TestSophiaKnowledgeGraph(t *testing.T) {
    // Test knowledge, insights, decisions endpoints
}
```

**Coverage target:** 80%+ on core paths

---

### 4. Security P0 Remediation
*Owner: Developer + Micromanager QA | ETA: 2-4 hours*

From the Jan 30 security audit (per timeline.md), these were flagged:

| File | Issue | Fix |
|------|-------|-----|
| `pkg/waf/response/response.go:237` | XSS via unescaped requestID | Use `html.EscapeString()` |
| `pkg/deploy/pipeline/hooks.go:405` | Command injection via `bash -c` | Use temp file + whitelisted interpreters |
| `pkg/waf/response/response.go:113` | Template injection in block pages | Escape template variables |

**Verify status:** Session summary mentions fixes were applied. Confirm:
```bash
# Check for html.EscapeString usage
grep -r "html.EscapeString" pkg/waf/
grep -r "html.EscapeString" cmd/dashboard-backend/

# Check hooks.go for temp file pattern
grep -A5 "bash -c" pkg/deploy/pipeline/hooks.go
```

---

### 5. Update timeline.md with Session Progress
*Owner: Timeguru | ETA: 30 min*

The timeline.md (in `docs/history/`) shows Jan 30 as last update. Add Feb 3 session:

**Entry format:**
```markdown
### 2026-02-03 - Session Summary

**SHIPPED:**
- [x] Test stability fixes (LXD guard, defer cancel, stall detection)
- [x] Monad/Sophia NixOS container definitions
- [x] Docker Compose entries for Monad (8086) and Sophia (8087)
- [x] Kanban task modal API fixes (PUT/DELETE paths)
- [x] Dashboard fetchMetrics enhancement

**METRICS:**
- Build: PASS
- Tests: 0 failures (was 2 timeout)
- Services: 8 total

**PROGRESS:**
- Alpha: 99% → approaching launch
```

---

## P2 - IMPORTANT (Before Alpha)

### 6. Container Integration Test with New Services
*Owner: Architect | ETA: 2 hours*

Monad/Sophia added to:
- `nix/containers/monad-service.nix`
- `nix/containers/sophia-service.nix`
- `docker-compose.yml` (ports 8086, 8087)
- Container test list

Run full container stack test:
```bash
docker-compose up -d
docker-compose ps  # All 8 should be "healthy"

# Test new service endpoints
curl http://localhost:8086/health  # Monad
curl http://localhost:8087/health  # Sophia
curl http://localhost:8086/api/v1/operations
curl http://localhost:8087/api/v1/knowledge
```

---

### 7. Dashboard Frontend Polish (70% → 85%)
*Owner: Developer | ETA: 4-6 hours*

Per CLAUDE.md success criteria, dashboard needs polish:

**Areas to enhance:**
- Packet-flow visualization (WebSocket connection to backend)
- Service health grid styling
- Responsive layout for mobile
- Loading states and error handling
- Kingdom theming (dark mode, gold accents)

---

### 8. Gateway Routes Verification
*Owner: Architect | ETA: 1 hour*

Session updated gateway routes for all services. Verify routing table:

```bash
# Check gateway config includes Monad/Sophia
grep -A2 "monad\|sophia" services/gateway/cmd/main.go

# Expected routes:
# /api/v1/monad/* → 172.28.1.6:8086
# /api/v1/sophia/* → 172.28.1.7:8087
```

---

## P3 - BLOCKED (Awaiting Dependencies)

### 9. eBPF Awakening (Whispering Void)
*Blocked on: B1 - Linux/eBPF dev environment*
*Owner: Muck | Impact: HIGH*

**Status:** 55% complete, 7,196 Rust LOC
**Requires:**
- Linux kernel >= 5.15
- libbpf-dev installed
- Rust nightly with Aya framework

**When unblocked:**
- Test eBPF programs on bare metal
- Wire trace-collector to Wotan
- Enable packet marking in dashboard

---

## TIMELINE TO ALPHA

```
Feb 3 (Today):  Kanban E2E smoke test, Dashboard integration test
Feb 4:          Monad/Sophia test coverage, Security verification
Feb 5:          Dashboard polish, Gateway verification
Feb 6-7:        Integration + Final QA
Feb 8:          🎉 ALPHA LAUNCH - Self-Hosted Demo
```

---

## QUICK VERIFICATION CHECKLIST

Before next session, confirm:

- [ ] `go build ./...` passes
- [ ] `go test ./...` has 0 failures
- [ ] `docker-compose up -d` starts all 8 services
- [ ] All health endpoints return 200
- [ ] Kanban displays tasks from Timeguru
- [ ] Dashboard shows service health grid

---

## SESSION HANDOFF NOTES

**What was solid:**
- Test fixes were surgical (LXD guard pattern is good)
- Stall detection in rolling strategy prevents infinite loops
- API path fixes (PUT vs PATCH) align with backend implementation
- Docker Compose entries follow existing patterns perfectly

**Watch out for:**
- Container memory limits in test config - verify they're realistic
- The `defer cancel()` fix in health check loop - ensure context propagation is correct
- Dashboard `/api/v1/health` call - ensure backend actually serves this endpoint

**Questions for Muck:**
- ETA on Linux/eBPF dev environment? (B1 blocker)
- Alpha launch: public demo or internal validation first?
- Post-alpha: Beta Trials start Feb 16 - scope review needed?

---

**THE TIMEGURU HAS SPOKEN.**
**THE KNIGHT REMAINS ARMORED.**
**THE ALPHA APPROACHES.**

⚔️🛡️🏰

*Prepared: February 3, 2026*
*Status: 99% → Launch imminent*
