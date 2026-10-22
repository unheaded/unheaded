# Service Discovery — The Cartographer's Eye

Four-layer service name resolution, implemented in `pkg/discovery/`.

## Resolution Cascade

| Layer | Method | Mechanism |
|-------|--------|-----------|
| 1 | **Wotan Registry** | Live service announcements via `system.discovery` topic (TTL-based) |
| 2 | **Port Scan** | TCP dial to verify expected ports are listening |
| 3 | **Convention Scan** | Filesystem: `/opt/unheaded/<service>/config.yaml` |
| 4 | **Static Fallback** | `configs/services.yaml` — known-good defaults |

## Package Files

- `discovery.go` — Registry with Wotan-based registration, TTL reaping, watch callbacks
- `scanner.go` — Convention-based filesystem scanner
- `resolver.go` — Multi-layer resolution cascade
- `static.go` — Static YAML fallback config
- `wotan.go` — `Start()`: joins Wotan, registers, retries until Wotan answers

## Registration

Services register on startup; an entry not re-announced within 30 s is reaped:

```go
discovery.Start(ctx, transportCfg, "timeguru", 19000)
```

11 services call this (timeguru, captain, architect, micromanager, monad,
sophia, dashboard-backend, kanban-app, unheaded-daemon, trace-collector,
chaos-controller). It joins `system.discovery` under the service's name,
which must be on `topics.auto_approve` in `configs/wotan.yaml`, announces
`$DISCOVERY_ADDR` (else the hostname; in compose the container's short ID,
which Docker's DNS resolves) every 15 s, and fills a local registry from
everyone else's announcements. Best-effort: if Wotan is not up yet it
retries in the background (1 s doubling to 1 min), logging only the first
failure and the recovery. Until 2026-10-22 every caller passed a nil
connection, so nothing ever registered.

Announcements are not authenticated: any member auto-approved on the topic
can announce any name. Nothing resolves through the registry yet; sign the
announcements before something does.

## Static Fallback

`configs/services.yaml` contains all service endpoints with Doom Range ports. Used when Wotan is unavailable.
