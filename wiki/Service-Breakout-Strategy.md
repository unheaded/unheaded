# Service Breakout Strategy

Plan to split the monorepo into one Go module per service, each with its own repo, CI and semver, imported by `github.com/unheaded/unheaded`.

**Status (2026-10-16): not started.** The repo is still one module (a single `go.mod`). The February plan targeted 2026-03-15; it slipped and has not been rescheduled. Which tools get extracted first is now framed by ADR-047 (extractable tools).

## Target layout

| Tier | Repos | Depends on |
|------|-------|------------|
| 1: core | `unheaded/wotan`, `unheaded/gateway` | nothing internal |
| 2: application | timeguru, captain, architect, monad, sophia (`wotan`); micromanager (`wotan`, `timeguru`) | Tier 1 |
| 3: orchestrator | `unheaded/unheaded` | all services, as modules |

## Shared code

Recommended: one shared module for common packages (`pkg/` transport, auth, discovery, logagg, wotan-client), versioned like any service. Alternative: services depend only on interface contracts and copy nothing.

## Extraction steps

1. Prepare the shared package; remove imports of `internal/` across service lines.
2. Extract Wotan with history (`git filter-repo`), tag `v1.0.0`.
3. Extract Tier 2 services, each pinned to a Wotan version.
4. Restructure the main repo to import modules; keep integration tests there.

Per service: own CI (`go test -race`, lint, SPDX), a compatibility row in the version matrix, a rollback path (re-vendor into the monorepo).

## Done when

- All services in their own repos with independent CI
- Main repo builds and starts the full stack from modules
- Integration tests pass; docs and release process updated

## Cross-service health

The source doc also holds the cross-service health doctrine: every service checks its dependencies, reports go to `system.health.reports`, and severity is set by percentage consensus. That is implemented by Akira today ([[Service-Akira]], ADR-029), independent of the breakout.

---

> **Source:** [docs/architecture/SERVICE_BREAKOUT_STRATEGY.md](../docs/architecture/SERVICE_BREAKOUT_STRATEGY.md) (2026-02) · ADR-047
