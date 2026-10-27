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
| `unheaded-auth.service`         | 19020         | Built-in OIDC issuer (ADR-103, `wiki/Service-Auth.md`); needs its key, a users file and TLS (`UNHEADED_TLS_*` in `/etc/unheaded/auth.env`), see the unit's header |
| `unheaded-anomaly.service`      | 19018 (loopback) | Anomaly service + cartridge bridge (`cmd/anomaly`), consumes `anomaly.scores` from Wotan; no capabilities. Reads only the `CART_EVENTS` pin (bind-mounted; `upc-cartridgectl -events-group unheaded-anomaly up`) |
| `unheaded-heimdall.service`     | 19101 (loopback, /metrics) | Drift monitor (`cmd/heimdall-daemon`), alerts only, signed `drift.detected.<node>`; account `unheaded-heimdall` (uid 16736), CAP_DAC_READ_SEARCH only. Not started anywhere until S-IRON-GATE E2 (drift-key custody); steps in `runbooks/security/heimdall-enrol-drift-key.yaml` |
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

## Keys (ADR-103)

Each service holds its own Ed25519 key and every verifier reads the public
halves from one trust directory. The units set `AUTH_KEY_FILE` and
`AUTH_TRUST_DIR` (their defaults, written out for the reader) and
`ReadOnlyPaths=-/etc/unheaded/trust.d`. Nothing is enforced until a service
runs with `AUTH_ENABLED=true` (S-IRON-GATE Phases 5-7).

Install, as root, once per service and host (`<identity>` is the name the
binary passes to `auth.LoadServiceAuthConfig`, e.g. `dashboard-backend` for
`unheaded-dashboard`, `unheaded-daemon` for Cuirass):

```bash
sudo unheaded-keygen -owner unheaded-<svc> <identity>
#   /etc/unheaded/keys/<identity>.key     0400, owned by the service (dir 0711: no listing)
#   /etc/unheaded/trust.d/<identity>.pub  0444, world-readable (public keys are not secrets)
```

The `.deb` postinst does this on first install (`scripts/lib/deb-maintscripts.sh`).
An existing key is never overwritten.

Rotation: `sudo unheaded-keygen -rotate <identity>` writes a new key (same
owner) and keeps the old public half trusted as
`trust.d/<identity>.<oldkid>.pub`; restart the service; after one token
lifetime (15 minutes) `sudo unheaded-keygen -retire <oldkid> <identity>`.
Removing a `.pub` from trust.d revokes that key. `unheaded-keygen -print-kid
<identity>` prints the current kid. Copy each host's new `.pub` to the other
host's trust.d for cross-host calls.

## TLS (ADR-105, FND-017)

S-IRON-GATE Phase 8. The thirteen first-party server units (akira,
architect, auth, captain, daemon, dashboard, kanban, micromanager, monad,
sophia, sword, timeguru, wotan) listen and dial TLS 1.3 with the Strict
hybrid ML-KEM profile (`pkg/tls`): a classical-only client is refused, and
a missing or unreadable certificate, key or CA stops the service instead of
starting it in plaintext. Each sets `UNHEADED_TLS_ENABLED=true`,
`UNHEADED_TLS_CA_CERT`, `UNHEADED_TLS_CERT`, `UNHEADED_TLS_KEY_FILE` under
`/etc/unheaded/tls/<identity>/` and `UNHEADED_TLS_MTLS_ENABLED=false` (the
caller's identity is its service token, not a client certificate). Wotan
takes its certificate from `--enable-tls --tls-cert --tls-key` (HTTP 18000
serves the Preferred profile for scrapers and scripts, gRPC 18001 Strict).
A Wotan client sends its service token over TLS only.

The units that dial Wotan or Cuirass but keep a plain listener
(chaos-controller, kev-poller, anomaly, baseline, trace-collector,
zhen-agentd) set `UNHEADED_TLS_ENABLED=true` and
`UNHEADED_TLS_CA_CERT=/etc/unheaded/tls/ca.pem` only.

`scripts/gen-dev-certs.sh` is for the compose stack only. On a host, issue
from the Kingdom CA (`runbooks/pki/generate-kingdom-certs.yaml`; rotation:
`runbooks/pki/rotate-service-cert.yaml`). Each certificate must name what
its callers dial: the host's name (`%H`, e.g. the BFFs' `AUTH_ISSUER_URL`),
`localhost`, `127.0.0.1` and `::1` (Akira and the healthchecks), and the
identity. Install, as root (`<unit>` as in `unheaded-<unit>.service`,
`<identity>` as for keys above, e.g. `daemon` / `unheaded-daemon`):

```bash
sudo install -d -m 0755 /etc/unheaded/tls
sudo install -m 0644 ca.pem /etc/unheaded/tls/ca.pem                  # client-only units
sudo install -d -m 0750 -o root -g unheaded-<unit> /etc/unheaded/tls/<identity>
sudo install -m 0644 ca.pem /etc/unheaded/tls/<identity>/ca.pem
sudo install -m 0644 svc.pem /etc/unheaded/tls/<identity>/<identity>.pem
sudo install -m 0640 -o root -g unheaded-<unit> svc-key.pem /etc/unheaded/tls/<identity>/<identity>-key.pem
```

Keys are `root:unheaded-<unit>` 0640, certificates 0644. Restarting a
unit picks up a new certificate. Turning TLS on is a host change: every
unit that talks to Wotan must move in the same window (a plaintext client
cannot reach a TLS port), and Prometheus jobs that scrape these units need
`scheme: https` with the CA.

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
