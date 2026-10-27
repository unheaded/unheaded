# S67/S68/S69 Handoff — Multi-Agent Swarm Complete

**Date**: 2026-02-26
**Sessions**: S67 (Observability) + S68 (Suricata IDS) + S69 (Alternate Routing)
**Status**: ALL THREE WAVES COMPLETE AND COMMITTED
**Next Session**: S70 — bare metal validation + go build/test on dev machine

---

## Git History (3 wave commits)

```
5367c02  [S67/S68/S69] Wave 3: NixOS tests + routing-health binary + HEALTHCHECK
981a8f0  [S67/S68/S69] Wave 2: Suricata IDS + anamnesis EVE bridge + alternate routing
7de7ad3  feat(S67/S68/S69): Wave 1 — observability stack + metrics collectors
```

**Total across 3 waves**: 81 files changed, 10,826 insertions

---

## What Was Built

### Wave 1 — Observability Stack (7de7ad3)

**pkg/metrics/** — Collector framework
- `collector.go` — `Collector` interface, `Sample` struct, `CollectorRegistry` parallel fan-out
- `baremetal.go` — /proc/meminfo, /proc/loadavg, /proc/stat, /sys/class/net/* readers
- `lxd.go` — HTTP over /var/lib/lxd/unix.socket /1.0/metrics
- `docker.go` — Docker API via /var/run/docker.sock
- `nixos.go` — systemctl list-units JSON, journalctl error count
- `collector_test.go`, `baremetal_test.go` — unit tests

**monitoring/** — Full Prometheus/Loki/VictoriaMetrics/Grafana/Alertmanager stack
- `prometheus/prometheus.yml` — all 25 Doom Range services (16666-16689) scrape targets + frr_exporter + bird_exporter + routing-health
- `prometheus/prometheus-host-b.yml` — secondary node, remote_write to host-a VM :8428
- `loki/loki.yaml` — 30d retention, TSDB v13
- `loki/loki-security.yaml` — 90d override for firewall/suricata/ids streams
- `promtail/promtail-host-a.yaml`, `promtail-host-b.yaml` — journal + EVE JSON + docker logs
- `victoriametrics/vm-config.yml` — [fd00:dead:beef::1]:8428, 12mo retention
- `alertmanager/alertmanager.yml` + `rules/monad.yml` — 6 alert rules incl. MonadHbHDropRateHigh
- `grafana/dashboards/` — 5 dashboards: infrastructure, container_fleet, routing_bgp, firewall, ebpf_pipeline
  - firewall.json: CRITICAL panel — Monad HbH Pass Rate, RED if <99.9%
- `grafana/provisioning/` — datasources (Prometheus/Loki/VictoriaMetrics) + dashboard provider
- `monitoring/docker-compose.yml` — 5-service compose stack

**nixos/modules/observability.nix**
- Options: enable, role (primary|secondary), lokiAddr, victoriaAddr, grafanaAdminPassword
- CRITICAL: `ip6 nexthdr 0 accept` (Monad HbH passthrough) in extraInputRules

**nixos/flake.nix** + flake.lock stub

### Wave 2 — Suricata IDS + Anamnesis + Routing (981a8f0)

**Suricata IDS (GPL-2.0 isolated)**
- `nixos/modules/suricata.nix` — NixOS module, SID 9000001-9000099 Monad rules, `decode-events: no`
- `docker/suricata/` — Dockerfile (ubuntu 24.04, multi-stage), entrypoint.sh, suricata.yaml
  - CRITICAL: `decode-events: no` + `ipv6-hopopts: yes` — HbH NEVER stripped
- `lxd/containers/suricata.yaml` — privileged, AppArmor, BPF fs bind, priority 195
- `routing/suricata/rules/unheaded-monad.rules` — SID 9000001/9000002/9000010/9000011/9000030/9000031/9000099
- `docs/legal/SURICATA_GPL_ISOLATION.md` — 3-point GPL boundary: EVE JSON / BPF fd / Unix socket

**pkg/anamnesis/suricata.go** — EVE JSON → Wotan bridge
- Tails /var/log/suricata/eve.json, filters SID 9000001-9000099
- Packs 64-byte RingEntry: [0:8]ts_ns | [8:12]sid | [12:28]src_ip | [28:44]dst_ip | [44]severity | [45:64]pad
- Publishes to Wotan topic `security.suricata.alert`
- 500ms poll, 5s retry backoff, graceful degraded mode
- `suricata_test.go` — 7 test functions, MockPublisher, fixture integration test
- `testdata/eve-sample.json` — 4-event NDJSON (2 Monad alerts expected)

**Alternate Routing (all 4 options)**

| Option | Config | NixOS Module |
|--------|--------|--------------|
| BGP EVPN (default) | existing routing/bgp/ | existing nixos/modules/frr.nix |
| OSPFv3 (Option A) | routing/ospf/ | nixos/modules/frr-ospf.nix |
| IS-IS+SR-MPLS (Option B) | routing/isis/ | (uses frr-mpls.nix) |
| MPLS LDP (Option C) | routing/mpls/ | nixos/modules/frr-mpls.nix |

- `scripts/routing/select-routing.sh` — live switcher: bgp-evpn|ospf|isis|mpls
  - vtysh --dryrun validation before switch, config backup
- `docs/network/ALTERNATE_ROUTING_OPTIONS.md` — 4-option comparison table
- `docker/routing/ospf/` — containerized FRR OSPFv3
- `lxd/containers/frr-ospf.yaml` — priority 188

**IS-IS NET addresses**:
- host-a: `49.0001.1020.0255.0001.00`
- host-b: `49.0001.1020.0255.0002.00`
- SRGB: 16000-23999, SRLB: 15000-15999
- Prefix-SID: host-a=16001, host-b=16002

### Wave 3 — NixOS Tests + routing-health (5367c02)

**nixos/tests/**
- `firewall-bridge.nix` — HbH nexthdr 0x00, br-unheaded, IPv6 forwarding
- `wireguard.nix` — wg0 MTU=1380, fd00:dead:beef::1/48, Doom Range 16666-16689
- `frr.nix` — frr package/service, /etc/frr/frr.conf, vtysh, BFD daemons
- `observability.nix` — prometheus/grafana/loki/promtail + HbH rule persistence
- `default.nix` — test index

**cmd/routing-health/**
- `main.go` — HTTP :8080, stdlib only, graceful degraded
  - GET /health → JSON `{status, checks: {frr, bird, wg0}}`
  - GET /ready → 200 if wg0 up, 503 otherwise
  - GET /metrics → Prometheus text: `routing_health_{frr,bird,wg0}_up`, `routing_health_{bgp_sessions,ospf_neighbors}`
  - Checker struct with injected functions (DI)
  - SIGTERM/SIGINT 5s graceful drain
- `main_test.go` — 8 tests, httptest, stdlib only, Checker mock injection

---

## CRITICAL: Monad HbH HOPOPT Safety Checklist

Every layer implemented — belt-and-suspenders at each:

| Layer | File | Rule |
|-------|------|------|
| NixOS firewall-bridge | nixos/modules/firewall-bridge.nix | `ip6 nexthdr 0 accept` |
| NixOS observability | nixos/modules/observability.nix | `ip6 nexthdr 0 accept` (redundant) |
| NixOS suricata | nixos/modules/suricata.nix | `ip6 nexthdr 0 accept` (redundant) |
| Suricata config | docker/suricata/suricata.yaml | `decode-events: no` + `ipv6-hopopts: yes` |
| MPLS | routing/mpls/frr-mpls.conf | label stack outer, IPv6+HbH inner (RFC 3031 §3.9) |
| Grafana alert | monitoring/grafana/dashboards/firewall.json | RED panel if HbH pass rate <99.9% |
| Prometheus alert | monitoring/alertmanager/rules/monad.yml | MonadHbHDropRateHigh (critical) |

---

## S70 Next Session: Bare Metal Validation

### MUST do on dev machine (requires Go 1.24):
```bash
cd ~/unheaded
go build ./...
go test ./pkg/metrics/... -race -count=1 -v
go test ./pkg/anamnesis/... -race -count=1 -v
go test ./cmd/routing-health/... -race -count=1 -v
```

### MUST do on NixOS hosts:
```bash
# host-a
sudo nixos-rebuild test --flake .#host-a
sudo systemctl status prometheus grafana loki node_exporter

# host-b
sudo nixos-rebuild test --flake .#host-b
sudo systemctl status suricata node_exporter
```

### SHOULD do (bare metal):
- Run `nix flake update` to generate real flake.lock
- Run `nix build .#checks.x86_64-linux.firewall-bridge` NixOS test
- Deploy monitoring/docker-compose.yml: `docker compose -f monitoring/docker-compose.yml up -d`
- Test routing switcher: `sudo scripts/routing/select-routing.sh ospf`
- Verify routing-health endpoint: `curl http://localhost:8080/health`

### DO NOT:
- nixos-rebuild switch (test first)
- bpftool/ip netns/eBPF load (bare metal only)
- lxc launch --privileged in CI

---

## File Tree Summary (new this session)

```
pkg/metrics/           collector.go baremetal.go lxd.go docker.go nixos.go + tests
pkg/anamnesis/         suricata.go suricata_test.go testdata/eve-sample.json
cmd/routing-health/    main.go main_test.go
nixos/modules/         observability.nix suricata.nix frr-ospf.nix frr-mpls.nix
nixos/tests/           firewall-bridge.nix wireguard.nix frr.nix observability.nix default.nix
nixos/flake.nix + flake.lock (stub)
monitoring/            prometheus/ loki/ promtail/ victoriametrics/ alertmanager/ grafana/ docker-compose.yml
docker/suricata/       Dockerfile entrypoint.sh suricata.yaml
docker/routing/ospf/   Dockerfile entrypoint.sh frr-ospf.conf
routing/ospf/          frr-ospf.conf bird-ospf.conf daemons-ospf README.md
routing/isis/          frr-isis-ha.conf frr-isis-hb.conf daemons-isis README.md
routing/mpls/          frr-mpls.conf setup-mpls-kernel.sh daemons-mpls README.md
routing/suricata/rules/ unheaded-monad.rules (SID 9000001-9000099)
scripts/routing/       select-routing.sh
scripts/suricata/      smoke-test.sh
lxd/containers/        suricata.yaml frr-ospf.yaml
lxd/profiles/          unheaded-observability.yaml + prometheus.yaml grafana.yaml loki.yaml victoriametrics.yaml promtail.yaml
docs/legal/            SURICATA_GPL_ISOLATION.md
docs/network/          ALTERNATE_ROUTING_OPTIONS.md
```

---

*S67/S68/S69 complete — 3 waves, 81 files, 10,826 insertions*
*Observability → IDS → Routing — the Kingdom watches, guards, and routes.*
