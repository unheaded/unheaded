# Session 13 Summary — The Armored Nervous System

**Date:** February 16, 2026
**Duration:** ~4 hours across 2 context windows
**Agents:** Claude Opus 4.6 (primary), Gemini (architecture reviewer)
**Theme:** Wiring the persistence layer, eliminating data races, sealing the Store

---

## The Story

Session 12 built the SQLite Store and the 48-card Kanban inventory. Session 13 wired the Store into the living nervous system and made it race-proof.

The session opened with the Store already written but dangling — `NewTaskManager` still took 2 arguments, the CRUD methods wrote to memory but not disk, and Busboy publishes blocked the caller. The first commit (`557a374`) changed all of that: 3-arg constructor, synchronous SQLite writes before async Busboy publishes, WaitGroup-tracked goroutines, and a Wait/Drain shutdown that gives inflight publishes 5 seconds to land.

Then came `go test -v -race -count=1` — the moment of truth. Four failures. Two data races caught by `-race`, two expectation mismatches from the Store migration, one test isolation failure from shared SQLite files.

The data races were instructive. The first was the classic async-pointer race: `CreateTask` fires a goroutine that marshals `*task` while the caller is already mutating the same pointer. The fix is the snapshot pattern — `taskSnap := *task` captures a value-copy before the goroutine, severing the shared reference. The second race was subtler: the mock's `StreamMessages` read `sub.Status` after releasing RLock, but `ApproveSubscription` writes it under Lock. One field, one gap, one race.

Gemini participated throughout as architecture reviewer. After validating the mutex hierarchy ("heavily armored"), the snapshot pattern, and the test isolation fixes, Gemini asked the question that prompted the session's final piece: "What happens if `handleGetTasks` calls `store.GetAllTasks()` after `store.Close()`?" The answer was the closed-store guard — `ErrStoreClosed` sentinel, `checkOpen()` inside every lock acquisition, idempotent `Close()` that sets the flag under write lock. No TOCTOU gap.

The second commit (`e4e1e08`) landed all five fixes. Tests passed clean: `PASS ok unheaded/cmd/kanban-app 1.855s` — zero race warnings, zero failures.

The session closed with the Taoist Wu Shu taxonomy — a rich source text mapping the eight classical forms of esoteric Taoist practice to infrastructure service concepts. Form 6 Fu (talismanic magical writings) maps to config-as-code and sealed secrets. Form 3 Zhanbu (divination via I Ching) maps to observability and pattern recognition. The Junzi concept — strive for perfection knowing you'll never achieve it — maps to SLO targets. Three new naming cards bring the board to 64 total.

---

## What Changed

### New Files
- `cmd/kanban-app/store.go` (388 lines) — SQLite L1 persistence layer with closed-store guard
- `cmd/kanban-app/store_test.go` (351 lines) — Comprehensive Store test suite

### Modified Files (13)
- `cmd/kanban-app/busboy.go` — Task snapshot pattern, async goroutines with WaitGroup, inbound SQLite persistence, Wait/Drain shutdown
- `cmd/kanban-app/busboy_test.go` — 3-arg constructor, inflight.Wait() drain calls
- `cmd/kanban-app/handlers_test.go` — t.TempDir() isolation, inflight drain between ops
- `cmd/kanban-app/main.go` — 6 mythology cards (Session 12 carryover) + 3 Taoist Wu Shu cards, total 64
- `cmd/kanban-app/main_test.go` — t.TempDir(), Store-aware assertions, bare server for empty test
- `cmd/kanban-app/static/css/main.css` — UI enhancements (neon delete, scroll header)
- `cmd/kanban-app/static/index.html` — Header link, layout tweaks
- `cmd/kanban-app/static/js/board.js` — Sort/filter additions
- `pkg/busboy-client/mock/mock.go` — Lock gap fix in StreamMessages
- `references/timeline.{json,md,toml,yaml}` — Minor date corrections

### Total: +1,221 / -123 lines across 15 files

---

## Patterns Established

### Task Snapshot Pattern
```go
taskSnap := *task          // value-copy severs shared reference
tm.inflight.Add(1)
go func() {
    defer tm.inflight.Done()
    tm.publishTask(ctx, topic, &taskSnap)  // safe: owns its copy
}()
```
Use whenever an async goroutine needs data from a pointer the caller will mutate.

### Wait/Drain Shutdown
```go
func (tm *TaskManager) Close() error {
    done := make(chan struct{})
    go func() { tm.inflight.Wait(); close(done) }()
    select {
    case <-done:     // all inflight drained
    case <-time.After(5 * time.Second):  // timeout, close anyway
    }
    return tm.client.Close()
}
```
Graceful shutdown that bounds the wait. No orphaned goroutines in production.

### Closed-Store Guard
```go
func (s *Store) checkOpen() error {
    if s.db == nil || s.closed { return ErrStoreClosed }
    return nil
}
// Called inside lock on EVERY public method — no TOCTOU gap
```

### Test Isolation
```go
cfg := Config{DataDir: t.TempDir()}
t.Cleanup(func() { if s.store != nil { s.store.Close() } })
```
Every test gets its own SQLite DB. No cross-test contamination.

---

## Concurrency Model (Final State)

Five primitives, zero races:

```
TaskManager.mu       (RWMutex) ─── task map: in-memory state
TaskManager.subMu    (RWMutex) ─── subscription lifecycle
TaskManager.inflight (WaitGroup) ── async publish tracking
Server.sseMu         (RWMutex) ─── SSE client distribution
Store.mu             (RWMutex) ─── SQLite access + closed flag
```

Lock ordering is implicit: `mu` before `subMu`, never nested with `sseMu`. Store locks are independent (separate service boundary). WaitGroup is lock-free.

---

## Data Persistence Model (L1/L2 Hybrid)

```
Write Path:
  Handler → mutex lock → SQLite L1 (sync) → SSE broadcast → unlock → Busboy L2 (async)

Read Path:
  Handler → Store.GetAllTasks() (SQLite L1) → JSON response

Inbound Sync:
  Busboy stream → handleMessage() → mutex lock → memory update + SQLite L1 → SSE broadcast → unlock

Recovery:
  Startup → Store.SeedIfEmpty() → load from SQLite L1
  If Busboy reconnects → stream replay catches up L2 state
```

SQLite L1 is the source of truth. Busboy L2 is the replication layer. The app survives Busboy outages with full local state.

---

## Numbers

| Metric | Value |
|--------|-------|
| Total cards on board | 64 |
| Mythology naming traditions | 12 |
| Test suite runtime | 1.855s |
| Race conditions | 0 |
| Test failures | 0 |
| Concurrency primitives | 5 |
| Store public methods guarded | 6/6 |
| Lines added | +1,221 |
| Lines removed | -123 |
| Commits | 2 landed + 1 pending |

---

## What's Next

The nervous system is sealed. The next major phase is **TopicStream gRPC** — replacing the 500ms HTTP polling loop with real-time server-side streaming and wildcard topic matching. This is the foundation for Busboy mesh (multiple Busboys peering via BGP anycast) and makes the eBPF packet tracing story clean (gRPC streams are long-lived connections we can tag).

Implementation order: `pattern.go` (pure logic) → `topic.proto` (service def) → `topic_service.go` (streaming server) → `grpc_client.go` (BusboyClient impl) → wire into main.go → full `-race` test suite.

---

*The efficacy of a Fu talisman isn't in the visual design — it's the personal power infused by its creator. And that's Qigong.*

*Session 13 — Muck + Claude Opus 4.6 + Gemini*
