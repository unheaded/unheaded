# deploy/systemd — Kingdom service units for EAST (bare metal)

Systemd units for all Kingdom services that run natively on EAST.
WEST runs these via Docker (`docker compose`); EAST runs them via systemd.

## Boot policy

| Unit                            | Boot policy        | Rationale                                    |
|---------------------------------|--------------------|----------------------------------------------|
| `huginn.service`                | **enabled**        | Always-on; historical metrics need no gap    |
| All other units                 | disabled (manual)  | Dev/staging services, start per session      |

Grafana is NOT auto-started — resource cost isn't worth it when idle. Start it
manually with `sudo docker compose up grafana -d` when you want dashboards.

## Services

| Unit file                       | Port(s)       | Notes                              |
|---------------------------------|---------------|------------------------------------|
| `huginn.service`                | 9110          | Host metrics agent — **boot-enabled** (ADR-084) |
| `unheaded-doom.target`          | —             | **DOOM pipeline target** — starts all 3 units below |
| `unheaded-doom-ring.service`    | —             | monad netns ring setup (one-shot)  |
| `unheaded-doom-runner.service`  | —             | Aya MBC executor + ROM + XDP attach |
| `unheaded-doom-injector.service`| —             | packet circulator (monad clock)    |
| `unheaded-wotan.service`        | 18000, 18001  | Start first — others depend on it  |
| `unheaded-timeguru.service`     | 19000         |                                    |
| `unheaded-architect.service`    | 19001         |                                    |
| `unheaded-captain.service`      | 19002         |                                    |
| `unheaded-micromanager.service` | 19003         |                                    |
| `unheaded-monad.service`        | 19004         |                                    |
| `unheaded-sophia.service`       | 19005         |                                    |
| `unheaded-dashboard.service`    | 20000         |                                    |
| `unheaded-kanban.service`       | 20001         |                                    |
| `unheaded-daemon.service`       | 17000, 17001  | Control plane / drift detection    |
| `unheaded-akira.service`        | 19100         | Health monitor                     |
| `unheaded-anomaly.service`      | 19018 (loopback) | Anomaly service (`cmd/anomaly --no-cartridge`), consumes `anomaly.scores` from Wotan; no capabilities. The cartridge bridge (root lab run) is not supervised |
| `unheaded-trace-collector.service` | 16670, 16671 | eBPF (XDP/TC/kprobes) → Wotan; CAP_BPF, CAP_PERFMON, CAP_NET_ADMIN only. Runs on west too, beside the compose stack |
| `unheaded-wg-psk-rotate.{service,timer}` | — | **West only.** Weekly WireGuard PSK rotation with east (`scripts/wireguard/rotate-psk.sh --apply`, ADR-105). Shipped disabled; enabling it is ESCALATE E3 (`docs/security/pq-host-escalations.md`) |

## Install (on EAST)

```bash
# Copy units
sudo cp deploy/systemd/*.service /etc/systemd/system/

# huginn env (edit HUGINN_VM for EAST — push to WEST's VictoriaMetrics;
# 192.168.13.2 is west on the P2P link, 192.168.13.1 is east itself)
sudo cp deploy/systemd/huginn.env.example /etc/huginn.env
sudo sed -i 's|http://localhost:8428|http://192.168.13.2:8428|' /etc/huginn.env

# One nologin account per service (systemd creates each StateDirectory)
sudo install -d -m 0755 /etc/sysusers.d
sudo install -m 0644 deploy/sysusers.d/unheaded.conf /etc/sysusers.d/
sudo systemd-sysusers

sudo systemctl daemon-reload

# Enable the always-on service. VictoriaMetrics runs on west only, in
# docker compose (service `victoria`, uid 16703); there is no unit for it.
sudo systemctl enable --now huginn
```

## Manual session start (other services)

```bash
sudo systemctl start unheaded-wotan
sudo systemctl start unheaded-timeguru unheaded-architect unheaded-captain \
    unheaded-micromanager unheaded-monad unheaded-sophia
sudo systemctl start unheaded-dashboard unheaded-kanban unheaded-daemon unheaded-akira
```

## DOOM on the UPC (full pipeline)

```bash
# Start everything: ring → runner (+ XDP attach) → injector
sudo systemctl start unheaded-doom.target

# Stop everything cleanly (ring teardown included)
sudo systemctl stop unheaded-doom.target

# Status
sudo systemctl status unheaded-doom-ring unheaded-doom-runner unheaded-doom-injector

# Verify DOOM is rendering (non-zero SCREEN_MAP entries)
MAP_ID=$(sudo bpftool map list | awk '/SCREEN_MAP/{print $1}' | tr -d ':')
sudo bpftool map dump id $MAP_ID 2>/dev/null | grep -c '"value"'
# Then open http://localhost:16666/
```

The XDP attach step (the known bug in doom-runner main.rs:429-436) is handled
automatically by `ExecStartPost=doom-xdp-attach.sh` in doom-runner.service.
No manual bpftool command needed.

The dashboard web GUI runs via Docker Compose (not this target):
```bash
cd ~/tmp/unheaded && sudo docker compose up dashboard-backend -d
```

## Grafana (optional, on demand)

```bash
cd ~/tmp/unheaded && sudo docker compose up grafana -d
# Stop when done:
cd ~/tmp/unheaded && sudo docker compose stop grafana
```

## Isolation

Every service runs as its own system account from
[`deploy/sysusers.d/unheaded.conf`](../sysusers.d/unheaded.conf): no shell
(`/usr/sbin/nologin`), no home, no password, its own group. No account is
shared between services, none is root, `nobody` or uid 1000.

Each long-running unit carries the same sandbox:

| What | How |
|---|---|
| Writable state | `StateDirectory=unheaded/<svc>` only (mode 0700, owned by the service; systemd creates it); everything else read-only (`ProtectSystem=strict`, `ProtectHome=yes`, `PrivateTmp`) |
| Files it creates | `UMask=0077` |
| Privilege | no capabilities (`CapabilityBoundingSet=`), `NoNewPrivileges`, no setuid (`RestrictSUIDSGID`) |
| Kernel | no tunables, modules, kernel log, cgroups, clock, hostname, devices |
| Other processes | `ProtectProc=invisible` (huginn excepted: it reports host-wide numbers) |
| Syscalls | `@system-service` minus `@privileged @resources`, native arch only; a denied call returns EPERM |
| Network | `AF_UNIX AF_INET AF_INET6` only |

Binaries and config are root-owned and not writable by any service account:
a service must not be able to replace what it, or root, runs.

Consequence: no service is in the `docker` group any more, so
`pkg/metrics`' Docker collector reports nothing when a service runs under
systemd (FND-006: docker group membership is root-equivalent).

East's services come from the `.deb` packages (`scripts/build-all-debs.sh`,
which ships these units and the matching sysusers line). East was purged and
reinstalled at `0.1.0~alpha+<commit>` on 2026-10-21; the interim
`10-isolation.conf` drop-ins (2026-10-18) were removed then, since the
packaged units carry the same sandbox.

## Binary paths and unit names

The units here exec `/usr/local/bin/<name>`, for a hand install (copy the
unit, put the binary there).

The `.deb` packages do not use that path or these file names.
`scripts/build-all-debs.sh` installs the binary at `/opt/unheaded/bin/<name>`
and ships its own copy of the unit, with `/usr/local/bin/` rewritten to
`/opt/unheaded/bin/`, as `/etc/systemd/system/<package>.service`. The unit
name is the package name, `unheaded-<binary>`:

| Repo unit | Unit installed by the deb |
|---|---|
| `unheaded-dashboard.service` | `unheaded-dashboard-backend.service` |
| `unheaded-kanban.service` | `unheaded-kanban-app.service` |
| `unheaded-daemon.service` | `unheaded-unheaded-daemon.service` |
| `unheaded-<svc>.service` (wotan, timeguru, captain, architect, micromanager, akira) | same name |

So on east (debs) start them as `unheaded-dashboard-backend`,
`unheaded-kanban-app` and `unheaded-unheaded-daemon`
(`runbooks/deploy/start-east-services.yaml`). Packages come from WEST's local
APT repo (`apt install unheaded-<binary>`); see
`runbooks/infra/apt-repo-server.yaml`. Units not packaged (monad, sophia,
kev-poller, trace-collector, huginn, ...) are hand-installed and keep
`/usr/local/bin`.

## Logs

```bash
sudo journalctl -u huginn -f
sudo journalctl -u unheaded-wotan -f
```
