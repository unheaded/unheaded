# Micromanager Additions

**Replace the "Definition of Done (DoD)" section with this expanded version:**

---

### Definition of Done (DoD)

A task is DONE when ALL of the following are verified:

#### Code Quality (Developer Checklist)
- [ ] Code complete and compiles cleanly (no warnings)
- [ ] All inputs validated (nil, empty, bounds, type)
- [ ] All errors handled explicitly (no ignored errors)
- [ ] No sensitive data in logs
- [ ] No hardcoded secrets
- [ ] Timeouts on all network operations
- [ ] Resource limits configured (memory, goroutines, connections)

#### Testing (Developer Checklist)
- [ ] Unit tests pass with race detection (`go test -race`)
- [ ] Coverage maintained or improved (check TimeGuru for targets)
- [ ] Integration tests pass
- [ ] No `// TODO: add tests later` - tests come FIRST
- [ ] Benchmarks run if performance-critical

#### Security (Developer + Micromanager)
- [ ] Security review passed
- [ ] Customer data isolation verified (does this touch customer data? **NO.**)
- [ ] No unsafe operations without justification
- [ ] TLS/mTLS configured where required

#### Documentation & Review
- [ ] Documentation updated (code comments, README if public API)
- [ ] Code reviewed by peer
- [ ] PR description explains WHY not just WHAT

#### Deployment
- [ ] Merged to main
- [ ] CI pipeline green
- [ ] Acceptance criteria met
- [ ] QA sign-off received

**Partial credit is not done. "Works on my machine" is not done. We ship TESTED work.**

---

**Add to "Quick Patterns" section, after "Security Concern Raised":**

### Developer Handoff Checklist

When receiving code from Developer for QA:

```
DEVELOPER HANDOFF CHECK

Code Quality:
- [ ] No compiler warnings
- [ ] golangci-lint clean (or equivalent)
- [ ] All errors wrapped with context

Testing:
- [ ] `go test -v -race ./...` passes
- [ ] Coverage report reviewed
- [ ] No skipped tests without explanation

Security:
- [ ] Input validation on all public functions
- [ ] No customer data access paths
- [ ] Secrets management verified

Documentation:
- [ ] Public APIs documented
- [ ] Complex logic has comments
- [ ] CHANGELOG updated if user-facing

Ready for QA: [YES/NO - list blockers]
```

---

**Add to "Cross-Skill Integration" (or create if not exists):**

## Cross-Skill Integration

| Skill | Micromanager Receives | Micromanager Provides |
|-------|----------------------|----------------------|
| **Developer** | Code + test results + coverage | DoD checklist, QA sign-off |
| **Architect** | Technical designs, dependencies | Priority, timeline pressure |
| **TimeGuru** | ETA updates, milestone status | Execution reality checks |
| **Captain** | Strategic priorities | Progress updates, blockers |
