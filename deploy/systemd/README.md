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
| `unheaded-wotan.service`        | 18000, 18001  | Start first — others depend on it  |
| `unheaded-timeguru.service`     | 19000         |                                    |
| `unheaded-dashboard.service`    | 20000         |                                    |
| `unheaded-kanban.service`       | 20001         |                                    |
| `unheaded-daemon.service`       | 17000, 17001  | Control plane / drift detection    |
| `unheaded-akira.service`        | 19100         | Health monitor                     |
| `unheaded-zhen-ui-front.service` | 20103, 20112 (loopback, /metrics) | TLS front of the Zhen web UI (ADR-138): hybrid-only TLS 1.3 to the app's Unix socket; see [zhen-ui-front](#zhen-ui-front) |
| `unheaded-zhen-app.service` | none (Unix socket `/run/unheaded-zhen-ui/app.sock`) | The Zhen web UI's app (`raft/zhen_app.py`) as `unheaded-zhen-app` (uid 16764); reached only through the front above. Shipped disabled; see [zhen-ui-front](#zhen-ui-front) |
| `unheaded-llama.service` | 8081 (loopback) | llama-server for Zhen as `unheaded-llama` (uid 16792, groups `render` and `video`): the GPU and one loopback port, nothing else. Shipped disabled |
| `unheaded-wiki-server.service` | 20002 (loopback; a drop-in adds addresses) | The wiki over TLS, behind the dashboard's login (pkg/bff) when `/etc/unheaded/wiki-server.env` names `AUTH_ISSUER_URL`; vor search when vor runs on the host; see [wiki-server](#wiki-server) |
| `unheaded-auth.service`         | 19020         | Built-in OIDC issuer (ADR-103, `wiki/Service-Auth.md`); needs its key, a users file and TLS (`UNHEADED_TLS_*` in `/etc/unheaded/auth.env`), see the unit's header |
| `unheaded-anomaly.service`      | 19018 (loopback) | Anomaly service + cartridge bridge (`cmd/anomaly`), consumes `anomaly.scores` from Wotan; no capabilities. Reads only the `CART_EVENTS` pin (bind-mounted; `upc-cartridgectl -events-group unheaded-anomaly up`) |
| `unheaded-heimdall.service`     | 19101 (loopback, /metrics) | Drift monitor (`cmd/heimdall-daemon`), alerts only, signed `drift.detected.<node>`; account `unheaded-heimdall` (uid 16736), CAP_DAC_READ_SEARCH only. Not started anywhere until S-IRON-GATE E2 (drift-key custody); steps in `runbooks/security/heimdall-enrol-drift-key.yaml` |
| `unheaded-trace-collector.service` | 16671 (loopback; `TRACE_METRICS_ADDR` adds the overlay) | eBPF (XDP/TC/kprobes) → Wotan; CAP_BPF, CAP_PERFMON, CAP_NET_ADMIN only. Auth on: `/api/v1/*` needs a credential, `/health`, `/ready`, `/metrics` do not. Runs on west too, beside the compose stack |
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
sudo systemctl start unheaded-timeguru
sudo systemctl start unheaded-dashboard unheaded-kanban unheaded-daemon unheaded-akira
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
auth, daemon, dashboard, kanban, sword, timeguru, wotan) listen and dial TLS 1.3 with the Strict
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
`UNHEADED_TLS_CA_CERT=/etc/unheaded/tls/ca.pem` only. chaos-controller
serves TLS once `chaos.env` also sets `UNHEADED_TLS_CERT` and
`UNHEADED_TLS_KEY_FILE`; one without the other stops it.

`unheaded-zhen-ui-front` is outside both lists: it has no profile and no
switch, serves the hybrid group alone, and takes a certificate and key with
no CA (its clients are browsers). See [zhen-ui-front](#zhen-ui-front).

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
sudo install -d -m 0755 /etc/unheaded/tls/<identity>   # listable: Akira's cert-expiry check reads every cert
sudo install -m 0644 ca.pem /etc/unheaded/tls/<identity>/ca.pem
sudo install -m 0644 svc.pem /etc/unheaded/tls/<identity>/<identity>.pem
sudo install -m 0640 -o root -g unheaded-<unit> svc-key.pem /etc/unheaded/tls/<identity>/<identity>-key.pem
```

Keys are `root:unheaded-<unit>` 0640, certificates 0644, directories 0755 (the
hourly `runbooks/pki/akira-cert-expiry-check.yaml`, run by Akira as
`unheaded-akira`, lists every `<identity>/` and reads its certificate; it never
opens a key). Restarting a
unit picks up a new certificate. Turning TLS on is a host change: every
unit that talks to Wotan must move in the same window (a plaintext client
cannot reach a TLS port), and Prometheus jobs that scrape these units need
`scheme: https` with the CA.

## wiki-server

`unheaded-wiki-server.service` serves `wiki/` (the deb's or the operator's
copy at `/usr/share/unheaded/wiki`) over TLS with the Preferred profile
(browsers), as `unheaded-wiki-server` (uid 16757), loopback only. It needs
`/etc/unheaded/tls/wiki-server/wiki-server.pem` and `-key.pem` and the CA at
`/etc/unheaded/tls/ca.pem`, and `/etc/unheaded/trust.d` for service tokens.
It holds no service key: it verifies tokens and login sessions and mints
nothing.

Login: with `AUTH_ISSUER_URL=https://<issuer>:19020` in
`/etc/unheaded/wiki-server.env` every page needs a session (the dashboard's
login, `pkg/bff`; the issuer must list the audience `wiki-server`); a browser
without one gets the page shell with the login panel. Without that line
reads stay open, as the wiki was before 2026-11-11, and the unit logs a
warning. The issuer is reached by name through the deployment CA: on a
host that runs the compose stack, compose publishes the issuer on
`127.0.0.1:19020` and the host has the line `127.0.0.1 auth` in
`/etc/hosts` (2026-11-12). The name must stay `auth`: it is the issuer
string in every token and a name on its certificate. A container address
in that line stops working whenever the container gets another one.

Publishing: the unit binds `127.0.0.1:20002`. A host that shows the wiki on
a LAN address or to the compose scrapes adds a drop-in with one `--listen
IP:20002` per address and the matching `IPAddressAllow=` and
`SocketBindAllow=tcp:20002`; vor search needs `IPAddressAllow=localhost`
(kept) and vor on `127.0.0.1:20110`.

## vor refresh

vor reads its sources once, at start. Two pieces keep its copy of develop
current with no one typing anything:

- The repository's post-commit and post-merge hook
  (`scripts/git-hooks/post-commit`, installed by `make install-hooks`) runs
  `scripts/vor-export-source.sh <dest> develop` when develop moves, as the
  repository's owner, only into a directory an earlier export made. The
  script writes the commit's hash into `<dest>/.vor-export` and, last and by
  rename, into `<dest>.exported` (the stamp).
- `unheaded-vor-refresh.path` watches the stamp
  (`/var/zhen/vor-source/unheaded.exported`) and starts
  `unheaded-vor-refresh.service`, a oneshot that runs
  `systemctl try-restart unheaded-vor.service`. A vor that is stopped stays
  stopped.

The service runs as `unheaded-vor-refresh` (uid 16758), not root (Stevie,
2026-11-12). The account reads nothing and writes nothing. Its only power is
`deploy/polkit/61-unheaded-vor-refresh.rules`: `try-restart` of
`unheaded-vor.service`, no other unit and no other verb.

It starts at most 6 times in 10 minutes. A seventh export inside the window
is refused, and on systemd 257 the path unit then shows as failed
(`unit-start-limit-hit`) and stops watching until it is reset:

```bash
systemctl is-failed unheaded-vor-refresh.path
sudo systemctl reset-failed unheaded-vor-refresh.path unheaded-vor-refresh.service
sudo systemctl start unheaded-vor-refresh.path
```

Seven commits or merges on develop inside ten minutes did trip it (S-WIKI-VOR;
seven branches were merged on 2026-11-21). Since 2026-11-21 (M-194.1,
decision pack row 41) the service waits before the restart:
`ExecStartPre=/usr/bin/sleep ${VOR_REFRESH_SETTLE_SEC}`, 120 s by default.
While it waits it is `activating`, so an export that lands then starts
nothing new and is read by the one restart that follows. A run lasts at
least the wait, so no more than 6 starts fit in 10 minutes and exports alone
cannot reach the limit; the burst itself is not raised. The cost: vor serves
a new export up to two minutes later. To change the wait on a host, a
drop-in, with the wait at or above `StartLimitIntervalSec` /
`StartLimitBurst` (100 s) and `TimeoutStartSec` at least a minute above the
wait:

```ini
# /etc/systemd/system/unheaded-vor-refresh.service.d/10-settle.conf
[Service]
Environment=VOR_REFRESH_SETTLE_SEC=180
TimeoutStartSec=240s
```

If the limit is reached anyway (starts by hand count too), the path unit
fails, and today **nothing alerts on it**: `unheaded-host-state` does not
report `.path` units or failed oneshots yet (M-194.1's second half, not built;
read `systemctl --failed`).

Install (the account first):

```bash
sudo install -m 0644 deploy/sysusers.d/unheaded.conf /etc/sysusers.d/unheaded.conf && sudo systemd-sysusers
sudo install -m 0644 deploy/polkit/61-unheaded-vor-refresh.rules /etc/polkit-1/rules.d/61-unheaded-vor-refresh.rules
sudo cp deploy/systemd/unheaded-vor-refresh.{path,service} /etc/systemd/system/
sudo systemctl daemon-reload && sudo systemctl enable --now unheaded-vor-refresh.path
```

By hand: `scripts/vor-export-source.sh /var/zhen/vor-source/unheaded develop`.

## zhen-ui-front

`unheaded-zhen-ui-front.service` (ADR-138) is the only way to the Zhen web
UI from a browser: TLS 1.3 with hybrid ML-KEM key exchange only
(`X25519MLKEM768`), no plaintext mode and no profile switch. It proxies to
the app's Unix socket, `/run/unheaded-zhen-ui/app.sock`. The app
(`raft/zhen_app.py`) binds that socket and no TCP port. It has its own unit
and account: `unheaded-zhen-app.service`, as `unheaded-zhen-app` (uid 16764),
with the model beside it in `unheaded-llama.service` as `unheaded-llama`
(uid 16792, groups `render` and `video`). Both ship disabled and are not
started by an install; the install steps are in each unit's header.
`raft/start-zhen.sh` still starts the app by hand as the person who runs it,
and leaves alone whatever a unit already runs.

The socket directory is the one step the units cannot do. One tmpfiles.d
line, kept in the repo, as root:

```bash
sudo install -m 0644 deploy/tmpfiles.d/unheaded-zhen-ui.conf /etc/tmpfiles.d/
sudo systemd-tmpfiles --create /etc/tmpfiles.d/unheaded-zhen-ui.conf
```

The app's environment file, `/etc/unheaded/zhen-app.env` (0640
`root:unheaded-zhen-app`), holds the Well's password for `app_zhen`. Root
writes it unseen with `scripts/write-zhen-app-env.sh --db-host HOST:PORT`;
`deploy/systemd/zhen-app.env.example` lists its keys.

Owner: the app's account, the only one that can create the socket. Group:
the front's own, with the setgid bit, so the socket is born in that group
and the app gives it 0660. Mode 2750: no other account reaches the
directory. The front is the group's only member and neither account joins
the other's group. Without the setgid bit the socket is 0600 and only the
app's account opens it, which is the dev layout of `raft/start-zhen.sh`
(app and front as one account, directory under `$XDG_RUNTIME_DIR`). The
front refuses a socket with any bit for others; the app refuses a directory
its group or others can write.

Certificate and key go in `/etc/unheaded/tls/zhen-ui-front/` as in TLS
above, with no CA. The certificate names what a browser types: `localhost`,
`127.0.0.1`, and the host's name or address once a LAN listener exists.

The shipped unit listens on `127.0.0.1:20103`. A LAN listener is a host's
drop-in: `ExecStart=` again with a second `--listen <address>:20103` (no
wildcard address), the subnet in `IPAddressAllow=`, the firewall rule, and
the name or address browsers use in `ZHEN_ALLOWED_HOSTS` in the app's
environment (the app answers 403 to any other Host).

`/metrics`, `/health` and `/ready` are plain HTTP on `127.0.0.1:20112`; the
front refuses a non-loopback address for them. `/ready` is 200 while the app
answers on its socket: `zhen-mcp`'s `service_health`, the dashboard's
`zhenai` row and the runbooks probe it. Akira's `zhenai` target goes through
the front instead (`https://localhost:20103/health`, the app's own answer,
checked against the deployment CA). No Prometheus job scrapes 20112: the
scrapers reach hosts over the overlay, and this listener is loopback only.

A client must offer the hybrid group: a current browser, a Go client, curl
on OpenSSL 3.5 or later. Any other handshake fails, counted in
`unheaded_zhen_ui_front_tls_client_hellos_total{kex="classical"}`.

## Binary paths and unit names

The units here exec `/usr/local/bin/<name>`, for a hand install (copy the
unit, put the binary there).

The `.deb` packages (`scripts/build-all-debs.sh`, table in
`scripts/lib/deb-packages.sh`) install the binary at
`/opt/unheaded/bin/<name>` and ship these units under their own names in
`/etc/systemd/system/`, with `/usr/local/bin/` rewritten to
`/opt/unheaded/bin/`. Every unit here that runs a binary from
`/usr/local/bin` has a package, third-party binaries (`node_exporter`,
`vor`) and cargo-built ones aside (`scripts/test-deb-maintscripts.sh` fails
otherwise). The package is `unheaded-<binary>`, or the binary's own name
when it already starts with `unheaded-`:

| Package | Unit |
|---|---|
| `unheaded-daemon` (replaces `unheaded-unheaded-daemon`) | `unheaded-daemon.service` |
| `unheaded-dashboard-backend` | `unheaded-dashboard.service` |
| `unheaded-kanban-app` | `unheaded-kanban.service` |
| `unheaded-heimdall-daemon` | `unheaded-heimdall.service` |
| `unheaded-trace-collector-go` | `unheaded-trace-collector.service` (eBPF objects in `/usr/local/lib/unheaded/ebpf` are not packaged) |
| `unheaded-huginn`, `unheaded-zhen-agentd` | `huginn.service`, `zhen-agentd.service` |
| `unheaded-baseline` | `unheaded-baseline.service` + `.timer` + `.path` |
| `unheaded-aide` | `unheaded-aide-check.service` + `.timer` (the timer is enabled by the `aide-install` fix, never by the package) |
| `unheaded-well-retention` | `unheaded-well-retention.service` + `.timer` |
| `unheaded-syslog-relay` | `unheaded-syslog-relay.service` + `.socket` (binds 514, socket activation), with `/etc/unheaded/log-sources.yaml` as a conffile |
| `unheaded-<svc>` (wotan, akira, timeguru, auth, sword, chaos-controller, anomaly, kev-poller, zhen-ui-front) | `unheaded-<svc>.service` |
| `unheaded-keygen` | none: the key tool every key-holding package depends on |
| `unheaded-configs` | none: runbooks + `run-runbook.py` under `/opt/unheaded` (Akira's cert-expiry check) |

Until 2026-10-27 the debs named the unit after the package
(`unheaded-dashboard-backend.service`, `unheaded-kanban-app.service`,
`unheaded-unheaded-daemon.service`, east's names from the 2026-10-21
install). The new packages' preinst records whether that former unit was
enabled and running, stops and disables it, and postinst enables and starts
the new unit to match. When apt removes `unheaded-unheaded-daemon` before
unpacking `unheaded-daemon`, the old package's own prerm has already
disabled the unit and nothing is carried over: check with
`systemctl is-enabled unheaded-daemon` and enable it by hand.

A package never enables a unit on a fresh install, and postinst restarts a
running service on upgrade only when the TLS files its unit names
(`UNHEADED_TLS_CA_CERT`, `_CERT`, `_KEY_FILE`) exist: the services refuse
to start without them, and a restart into that refusal would take a running
service down. Install the TLS material first (see TLS above). Packages come
from west's signed apt repository (`scripts/apt-repo.sh`,
`runbooks/deploy/apt-repo-publish.yaml`).

## Logs

```bash
sudo journalctl -u huginn -f
sudo journalctl -u unheaded-wotan -f
```
