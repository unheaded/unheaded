# Service Breakout Strategy

**Status:** PLANNED (Post-Alpha)
**Target:** After stable Alpha release
**Author:** Architect + Micromanager
**Last Updated:** February 3, 2026

---

## Overview

The Unheaded Kingdom currently operates as a **monorepo** for rapid Alpha development. After stable Alpha (target: Feb 8, 2026), we will transition to a **multi-repo** architecture where each service becomes its own Go module with independent versioning.

**Current State:** Monorepo (`github.com/unheaded/unheaded`)
**Target State:** Service repos imported as Go modules

---

## Architecture Vision

```
┌─────────────────────────────────────────────────────────────────┐
│                    github.com/unheaded/unheaded                  │
│                         (Orchestrator)                           │
│                                                                  │
│   go.mod:                                                        │
│   require (                                                      │
│       github.com/unheaded/busboy v1.x.x                         │
│       github.com/unheaded/timeguru v1.x.x                       │
│       github.com/unheaded/captain v1.x.x                        │
│       github.com/unheaded/architect v1.x.x                      │
│       github.com/unheaded/micromanager v1.x.x                   │
│       github.com/unheaded/monad v1.x.x                          │
│       github.com/unheaded/sophia v1.x.x                         │
│       github.com/unheaded/gateway v1.x.x                        │
│   )                                                              │
└─────────────────────────────────────────────────────────────────┘
                              │
        ┌─────────────────────┼─────────────────────┐
        ▼                     ▼                     ▼
┌───────────────┐   ┌───────────────┐   ┌───────────────┐
│ github.com/   │   │ github.com/   │   │ github.com/   │
│ unheaded/     │   │ unheaded/     │   │ unheaded/     │
│ busboy        │   │ timeguru      │   │ captain       │
│               │   │               │   │               │
│ go.mod:       │   │ go.mod:       │   │ go.mod:       │
│ module        │   │ module        │   │ require       │
│ github.com/   │   │ github.com/   │   │ github.com/   │
│ unheaded/     │   │ unheaded/     │   │ unheaded/     │
│ busboy        │   │ timeguru      │   │ busboy        │
└───────────────┘   └───────────────┘   └───────────────┘
```

---

## Service Repositories

### Tier 1: Core Infrastructure (No Dependencies)

| Service | Repo | Description |
|---------|------|-------------|
| **Busboy** | `github.com/unheaded/busboy` | Message bus - Fae Chamber |
| **Gateway** | `github.com/unheaded/gateway` | API Gateway - The Shield |

These have **zero internal dependencies** and can be extracted first.

### Tier 2: Application Services (Depend on Busboy)

| Service | Repo | Dependencies |
|---------|------|--------------|
| **Timeguru** | `github.com/unheaded/timeguru` | busboy |
| **Captain** | `github.com/unheaded/captain` | busboy |
| **Architect** | `github.com/unheaded/architect` | busboy |
| **Micromanager** | `github.com/unheaded/micromanager` | busboy, timeguru |
| **Monad** | `github.com/unheaded/monad` | busboy |
| **Sophia** | `github.com/unheaded/sophia` | busboy |

### Tier 3: Orchestration (Depends on All)

| Service | Repo | Dependencies |
|---------|------|--------------|
| **Unheaded** | `github.com/unheaded/unheaded` | All services |

---

## Shared Packages Strategy

### Option A: Shared Module (Recommended)

Create `github.com/unheaded/pkg` for shared types:

```go
// github.com/unheaded/pkg/events/events.go
package events

type TaskEvent struct { ... }
type TimelineEvent struct { ... }
```

All services import from this shared module:
```go
import "github.com/unheaded/pkg/events"
```

### Option B: Interface Contracts

Each service defines its own interfaces, contracts exchanged via protobuf/gRPC:

```protobuf
// busboy.proto
service BusboyService {
    rpc Publish(PublishRequest) returns (PublishResponse);
    rpc Subscribe(SubscribeRequest) returns (stream Message);
}
```

Services generate clients from proto, no Go imports needed.

### Recommendation

**Hybrid approach:**
- `github.com/unheaded/pkg` for shared Go types (events, configs)
- Protobuf for service-to-service contracts
- Each service can run standalone with just Busboy connection

---

## Extraction Process

### Phase 1: Prepare Shared Package (Week 1)

1. Create `github.com/unheaded/pkg` repository
2. Extract shared types from `pkg/events/`, `pkg/types/`
3. Update monorepo to import from shared package
4. Verify build passes

### Phase 2: Extract Busboy (Week 2)

1. Create `github.com/unheaded/busboy` repository
2. Copy `services/busboy/` content
3. Set up independent CI/CD
4. Tag v1.0.0
5. Update monorepo to import `github.com/unheaded/busboy`
6. Remove `services/busboy/` from monorepo

### Phase 3: Extract Tier 2 Services (Weeks 3-4)

For each service (timeguru, captain, architect, micromanager, monad, sophia):

1. Create `github.com/unheaded/{service}` repository
2. Copy `services/{service}/` content
3. Update go.mod to import:
   - `github.com/unheaded/busboy`
   - `github.com/unheaded/pkg`
4. Set up CI/CD
5. Tag v1.0.0
6. Update monorepo to import service
7. Remove from monorepo

### Phase 4: Restructure Main Repo (Week 5)

Transform `github.com/unheaded/unheaded` into orchestrator:

```
unheaded/
├── cmd/
│   ├── unheaded-daemon/    # Control plane (keeps local)
│   └── unheaded-cli/       # CLI tool (keeps local)
├── deploy/
│   ├── docker-compose.yml  # Full stack deployment
│   ├── helm/               # Kubernetes charts
│   └── nix/                # NixOS configs
├── docs/                   # Documentation
├── go.mod                  # Imports all services
└── Makefile               # Build orchestration
```

---

## Versioning Strategy

### Semantic Versioning

All services follow semver: `vMAJOR.MINOR.PATCH`

- **MAJOR**: Breaking API changes
- **MINOR**: New features, backward compatible
- **PATCH**: Bug fixes

### Version Compatibility Matrix

Maintain `COMPATIBILITY.md` in main repo:

```markdown
| unheaded | busboy | timeguru | captain |
|----------|--------|----------|---------|
| v1.0.0   | v1.0.x | v1.0.x   | v1.0.x  |
| v1.1.0   | v1.0.x | v1.1.x   | v1.0.x  |
```

### Release Coordination

1. Services release independently
2. Main repo pins to specific versions
3. Integration tests run against pinned versions
4. Upgrade PRs update go.mod with new versions

---

## Cross-Service Health Monitoring

### The Outage Detection Pattern

**Requirement:** Each microservice MUST health check all other microservices it depends on.

When a service detects another service is down, it reports to a dedicated Busboy room for distributed outage detection. Alert severity is determined by consensus - multiple services reporting the same outage increases confidence.

### Busboy Outage Room

```
Topic: system.outage.reports
```

**Message Format:**
```json
{
  "reporter": "timeguru",
  "target": "captain",
  "status": "unreachable",
  "timestamp": "2026-02-03T12:00:00Z",
  "check_type": "http_health",
  "error": "connection refused",
  "trace_id": "abc123"
}
```

### Severity Escalation by Consensus (Percentage-Based)

Severity is calculated as: `(unique_reporters / total_dependent_services) * 100`

This scales automatically as the Kingdom grows from 8 services to 80+.

| % Reporting | Severity | UI Color | Hex | Actions |
|-------------|----------|----------|-----|---------|
| 0% - 12.49% | **OK** | Green | `#008000` | Healthy, no action |
| 12.50% - 37.49% | **WARN** | Light Brown/Yellow | `#fdda61` | Log, send email |
| 37.50% - 62.49% | **ERROR** | Bright Yellow | `#ffff00` | Log, 2nd email, optional configurable instructions |
| 62.50% - 87.49% | **CRITICAL** | Neon Orange | `#ff5c00` | Log, attempt auto-remediation, escalate |
| 87.50% - 100% | **PANIC** | Bright Red | `#ff0000` | Log, Call, Text, Email, PagerDuty, all hands |

**Example with 8 services:**
- 1 reporter (12.5%) → WARN
- 3 reporters (37.5%) → ERROR
- 5 reporters (62.5%) → CRITICAL
- 7 reporters (87.5%) → PANIC

**Example with 24 services:**
- 3 reporters (12.5%) → WARN
- 9 reporters (37.5%) → ERROR
- 15 reporters (62.5%) → CRITICAL
- 21 reporters (87.5%) → PANIC

**Why percentage-based?**
- Scales with fleet size automatically
- A single reporter could be experiencing network partition
- Multiple independent reporters confirming the same outage = high confidence
- Avoids hardcoded thresholds that break when architecture evolves

### Health Check Implementation

Each service runs a background goroutine:

```go
func (s *Service) healthCheckLoop(ctx context.Context) {
    ticker := time.NewTicker(30 * time.Second)
    defer ticker.Stop()

    for {
        select {
        case <-ctx.Done():
            return
        case <-ticker.C:
            for _, dep := range s.dependencies {
                if err := s.checkHealth(dep); err != nil {
                    s.busboy.Publish(ctx, "system.outage.reports", OutageReport{
                        Reporter:  s.name,
                        Target:    dep.Name,
                        Status:    "unreachable",
                        Timestamp: time.Now(),
                        Error:     err.Error(),
                    })
                }
            }
        }
    }
}
```

### Outage Aggregator (Cuirass Responsibility)

The control plane subscribes to `system.outage.reports` and:

1. **Deduplicates** reports within time window (5 min)
2. **Counts** unique reporters per target
3. **Escalates** based on threshold
4. **Triggers** auto-remediation (restart container) at CRITICAL
5. **Declares** incident at PANIC

```go
type Severity int

const (
    OK Severity = iota
    WARN
    ERROR
    CRITICAL
    PANIC
)

// SeverityColors for Cloak UI Dashboard
var SeverityColors = map[Severity]string{
    OK:       "#008000",  // Green
    WARN:     "#fdda61",  // Light Brown/Yellow
    ERROR:    "#ffff00",  // Bright Yellow
    CRITICAL: "#ff5c00",  // Neon Orange
    PANIC:    "#ff0000",  // Bright Red
}

type OutageAggregator struct {
    reports       map[string][]OutageReport  // target -> reports
    totalServices int                         // dynamic fleet size
    mu            sync.RWMutex
}

func (a *OutageAggregator) GetSeverity(target string) Severity {
    a.mu.RLock()
    defer a.mu.RUnlock()

    reports := a.reports[target]
    uniqueReporters := countUnique(reports)

    // Calculate percentage of fleet reporting outage
    percentage := float64(uniqueReporters) / float64(a.totalServices) * 100

    switch {
    case percentage >= 87.50:
        return PANIC
    case percentage >= 62.50:
        return CRITICAL
    case percentage >= 37.50:
        return ERROR
    case percentage >= 12.50:
        return WARN
    default:
        return OK
    }
}

func (a *OutageAggregator) GetColor(target string) string {
    return SeverityColors[a.GetSeverity(target)]
}
```

### Service Dependency Map

| Service | Health Checks |
|---------|---------------|
| **Timeguru** | busboy |
| **Captain** | busboy, timeguru |
| **Architect** | busboy |
| **Micromanager** | busboy, timeguru, captain |
| **Monad** | busboy |
| **Sophia** | busboy, monad |
| **Gateway** | all services |
| **Cuirass** | all services (aggregator) |

---

## CI/CD Per Repository

### Service Repository Template

```yaml
# .github/workflows/ci.yml
name: CI

on: [push, pull_request]

jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: '1.24'
      - run: go test -race -coverprofile=coverage.out ./...
      - run: go build ./...

  release:
    if: startsWith(github.ref, 'refs/tags/v')
    needs: test
    runs-on: ubuntu-latest
    steps:
      - uses: goreleaser/goreleaser-action@v5
```

### Main Repository Integration Tests

```yaml
# .github/workflows/integration.yml
name: Integration Tests

on:
  schedule:
    - cron: '0 0 * * *'  # Daily
  workflow_dispatch:

jobs:
  integration:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - run: docker-compose up -d
      - run: go test -tags=integration ./tests/...
```

---

## Migration Checklist

### Pre-Extraction (Per Service)

- [ ] Service has clean package boundaries
- [ ] No circular dependencies
- [ ] All internal imports use full paths
- [ ] Tests pass in isolation
- [ ] Documentation updated

### Extraction (Per Service)

- [ ] New repository created
- [ ] Code copied with git history (git filter-branch)
- [ ] go.mod updated with correct module path
- [ ] CI/CD configured
- [ ] README updated
- [ ] First version tagged (v1.0.0)

### Post-Extraction

- [ ] Main repo updated to import service
- [ ] Old code removed from monorepo
- [ ] Integration tests pass
- [ ] Docker images build correctly
- [ ] Documentation updated

---

## Rollback Plan

If extraction causes issues:

1. **Revert go.mod** to use local replace directives
2. **Re-add service code** to monorepo
3. **Tag broken version** as deprecated
4. **Investigate** root cause before retry

```go
// Temporary rollback in go.mod
replace github.com/unheaded/busboy => ./services/busboy
```

---

## Success Criteria

Breakout is complete when:

- [ ] All 8 services in separate repositories
- [ ] Each service has independent CI/CD
- [ ] Main repo imports all services as modules
- [ ] `go build ./...` passes in main repo
- [ ] `docker-compose up` starts full stack
- [ ] Integration tests pass
- [ ] Documentation reflects new architecture
- [ ] Release process documented

---

## Timeline

| Phase | Duration | Target Date |
|-------|----------|-------------|
| Alpha Stable | - | Feb 8, 2026 |
| Phase 1: Shared Package | 1 week | Feb 15, 2026 |
| Phase 2: Extract Busboy | 1 week | Feb 22, 2026 |
| Phase 3: Extract Services | 2 weeks | Mar 8, 2026 |
| Phase 4: Restructure Main | 1 week | Mar 15, 2026 |
| **Breakout Complete** | - | **Mar 15, 2026** |

---

## References

- [Go Modules Reference](https://go.dev/ref/mod)
- [Monorepo vs Multi-repo](https://earthly.dev/blog/monorepo-vs-polyrepo/)
- [Git Filter-Branch](https://git-scm.com/docs/git-filter-branch)

---

**THE ARCHITECT HAS SPOKEN.**
**THE KINGDOM WILL EXPAND.**
**EACH PIECE OF ARMOR, ITS OWN FORGE.**

⚔️🛡️🏰
