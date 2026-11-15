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

## The reader

Akira is the one reader (ADR-147 Phase 9, D3 = A): `discovery.Watch` on its
Wotan client, one stream. An announcement is admitted only when

1. its name is the sender Wotan verified on the publisher's token
   (`verified_sender`), so a service can announce only itself;
2. its address (an IP, or a hostname resolved then) lies inside
   `discover.subnets` in Akira's config (or `--discover-subnets`); the
   checked IP is kept, never the name;
3. the registry holds fewer than 256 names.

Each refusal is counted in `unheaded_discovery_refused_total{reason}`
(`sender`, `address`, `full`, `decode`); an entry not announced for 30 s is
reaped. Every sweep checks each admitted service the config does not list,
at `https://<ip>:<port>/health` with the announced name as the TLS server
name, under its ballot name (`dashboard-backend` is `dashboard`); its
verdict in `GET /api/v1/status` carries `"source":"discovered"`. No subnets
(the default) is off: discovery stays announce-only. The compose config
admits the `data` network, `172.28.1.0/24`.

## Static Fallback

`configs/services.yaml` contains all service endpoints with Doom Range ports. Used when Wotan is unavailable.
