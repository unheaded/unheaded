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
- `wotan.go` — `Start()`: joins Wotan, announces every 15 s, retries until Wotan answers; no stream

## Registration

Each service announces itself on startup with its one Wotan client:

```go
wc, err := transport.NewWotanClient(transportCfg) // the process's one client
discovery.Start(ctx, wc, "timeguru", ports.PortOf(addr, ports.Timeguru))
```

Seven services call it (timeguru, dashboard-backend, kanban-app,
unheaded-daemon, trace-collector, sword, chaos-controller). It joins
`system.discovery` under the service's name, which must be on
`topics.auto_approve` in `configs/wotan.yaml`, and announces
`$DISCOVERY_ADDR` (else the hostname; in compose the container's short ID,
which Docker's DNS resolves) every 15 s. It opens no stream and builds no
registry (ADR-147 Phase 7: twelve announcers each streamed the topic and
nobody read what they gathered). Best-effort: if Wotan is not up yet it
retries in the background (1 s doubling to 1 min), logging only the first
failure and the recovery. Until 2026-10-22 every caller passed a nil
connection, so nothing ever registered.

The same client carries the service's health reports and its logs;
`pkg/wotan-client/policy_test.go` fails a binary that builds a second client
or a library that builds one.

Announcements are not authenticated: any member auto-approved on the topic
can announce any name. Nothing reads them yet; the registry (`New`,
`Resolve`, the reaper) stays in the package for the reader.

## Static Fallback

`configs/services.yaml` contains all service endpoints with Doom Range ports. Used when Wotan is unavailable.
