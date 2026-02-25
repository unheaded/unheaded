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
- `registrar.go` — Transport-layer registration via Wotan topics
- `resolver.go` — Multi-layer resolution cascade
- `static.go` — Static YAML fallback config
- `setup.go` — `SetupServiceDiscovery()` one-liner for services

## Registration

Services register on startup and deregister on shutdown:

```go
discovery.SetupServiceDiscovery(ctx, conn, "timeguru", 19000)
```

All 10 services call this on startup. Registration is best-effort — services operate normally if discovery is unavailable.

## Static Fallback

`configs/services.yaml` contains all service endpoints with Doom Range ports. Used when Wotan is unavailable.
