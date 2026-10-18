# Service isolation: one nologin user per service, least privilege (2026-10-18)

Stevie, 2026-10-18: "services should all be isolated with nologin user and permissions
restricted to run that service. least privilege, security first. fix services that are
deviating from that."

Rule (also memory `feedback_service_nologin_users`): every service runs as its own
system user with `/usr/sbin/nologin`, owns only its own state, gets only the
capabilities it needs. Never root, never a shared account (`unheaded`, `nobody`,
`www-data`), never uid 1000 (that is govan on both hosts).

## Audit (2026-10-18)

| Where | Deviation |
|---|---|
| repo units | 12 Unheaded services + huginn share `User=unheaded` |
| repo units | No `User=` (root): unheaded-baseline, unheaded-nft, 3 Doom units, unheaded-victoria (docker wrapper), well-backup/-drill/-offsite (`docker exec`/`docker run` = root-equivalent) |
| repo units | Doom units run binaries out of /home/govan; victoria's WorkingDirectory is /home/govan |
| west | `unheaded` (uid 993) has /bin/bash and is in docker, lxd, video, render (FND-006: docker and lxd are root-equivalent) |
| west | node-exporter runs as `nobody`; nginx workers as `www-data`; `frr` has /bin/sh |
| east | 9 services from an older generator (scripts/setup-host.sh): all `User=unheaded`, one shared `ReadWritePaths=/var/lib/unheaded /var/log/unheaded`, no capability/syscall limits |
| east | binaries in /opt/unheaded/bin owned by govan: govan can replace what the services run (privilege path into every service account) |
| east | /etc/unheaded/wotan.yaml is govan:govan 0664; 4 stale disabled units (wotan, timeguru, kanban-app, dashboard-backend) |
| containers | all 11 Unheaded images run as uid 1000 with /bin/sh (uid 1000 = govan on the host for every bind mount) |
| containers | NET_BIND_SERVICE added to services that bind ports above 1024 |
| containers | traefik runs as root with the docker socket mounted (`:ro` does not limit the API); victoria, vector run as root |
| scripts | scripts/setup-host.sh still generates `User=root` for unheaded-daemon |

## Phases

### 1. Unheaded service units (repo, then east)
- [x] `deploy/sysusers.d/unheaded.conf`: one system user per service, nologin, home `/`
- [x] every unit: own `User=`/`Group=`, `StateDirectory=` instead of hand-made dirs, `UMask=0077`, full sandbox set (zhen-agentd: `zhen` was never created anywhere, now `zhen-agentd`)
- [x] scripts/build-all-debs.sh ships the repo unit + one sysusers line per package; scripts/setup-host.sh installs sysusers + the repo daemon unit (was `User=root` + 4 caps), root-owned /opt/unheaded
- [ ] gate: every deploy/systemd *.service has a User= from sysusers (or DynamicUser). Deferred until FND-010's root units are done, so it can start at zero exceptions; check-systemd-units.sh (verify) still runs
- [x] east: drop-in `10-isolation.conf` per .deb unit (same block as the repo; packages not rebuilt, so binaries unchanged), kanban.db/timeguru.db moved into their state dirs, /opt/unheaded + wotan.yaml root-owned, 4 stale units removed (they ran as **govan**); all 9 health endpoints 200, no new journal errors

### 2. Units that ran as root
- [x] nft: `unheaded-nft` + CAP_NET_ADMIN only; confirmed.nft stays root 0600, passed on stdin by systemd (68784980)
- [ ] baseline: needs agent-mode probes without `sudo -n` (passwd -S, ufw, aa-status, `sudo -iu root` check uid 0 themselves); FND-010
- [ ] Doom: `unheaded-doom` + CAP_BPF/PERFMON/NET_ADMIN/SYS_ADMIN, artifacts out of /home; FND-010 (installed nowhere; a live Doom run is a heavy test)
- [x] victoria: the root docker-compose wrapper unit removed (installed nowhere; victoria is a compose service at uid 16703)

### 3. The Well's backup jobs off docker and root
- [ ] PostgreSQL 16 client/server binaries on the host (PGDG); backup connects over TLS as `well_backup` with a systemd credential. **Waits for Stevie's Ubuntu 26.04 upgrade** (FND-010)
- [ ] drill runs a throwaway cluster as `unheaded-well-drill` in a private dir, no network
- [ ] offsite as `unheaded-well-offsite`, read-only on the sets
- [ ] live run of each on west; restore drill green

### 4. Containers
- [x] one uid per Unheaded image (pkg/uids), nologin, numeric USER; NET_BIND_SERVICE dropped everywhere; all-in-one dev stage off root (1e9b9eeb)
- [x] all 7 third-party services: own uid, read_only, cap_drop ALL, no-new-privileges; traefik on a file provider; clickhouse off uid 101 (= _chrony on the hosts); volume-init one-shot chown. Alternate compose files: 75 services baselined (FND-009)

### 5. Host accounts
- [x] `unheaded` removed from west and east (FND-006, acct-system-no-shell, home-dirs closed)
- [x] node-exporter (repo unit), nginx workers, frr shell to nologin. Still root masters by design: haproxy and nginx (bind 80/443, drop to their own accounts)
- [x] register, wiki (Systemd-Units, Security), deploy/systemd/README, setup-host.sh, bootstrap-llm-lab.sh, east-bootstrap.sh, deploy-alpha.sh, Jenkinsfile

## Log

- 2026-10-18 03:10 west: sysusers installed, huginn as `unheaded-huginn`; all metric families present. /opt/unheaded chowned root (was `unheaded`; root's kingdom-shutdown.service runs a script from /opt/unheaded/bin at every shutdown, so `unheaded` could have had root). Undo data: /var/lib/unheaded/remediate/20261018T031030Z-service-isolation.
- 2026-10-18 03:12 east: same + 9 services via drop-ins. Backup (units, /etc/unheaded, /opt bins, /var/lib/unheaded tar): /var/lib/unheaded/remediate/20261018T031217Z-service-isolation. Akira on east listens on 16699 (old build), not 19100: pre-existing.
- Stevie 2026-10-18: no OS upgrades from me (Ubuntu 26.04 is his, manual), so Phase 3's PGDG packages wait for that.
- 2026-10-18 03:30-03:55 west: compose stack brought up with the new identities (it was down), 16 healthy + coredns/vector running, huginn reaches the Well over TLS, traefik routes from the file, Well backup + offsite + restore drill PASS (676 rows), stack stopped again. Found on the way: vector never got CLICKHOUSE_PASSWORD (fell back to "changeme", 403), fixed; clickhouse's 101 = host _chrony; tmpfs over clickhouse's config.d hid docker_related_config.xml.
- 2026-10-18 west: /opt/unheaded root-owned; `unheaded` account and the two dead units removed (wotan/daemon pointed at binaries that do not exist; daemon was User=root + CAP_SYS_ADMIN); frr nologin (BGP stayed Established); node-exporter and nginx on own accounts (same metrics; apt repo 200 from west and east).
- Not done, Stevie's call or deferred: FND-010 (baseline agent, Doom units, Well jobs still root), FND-009 (75 services in undeployed compose files), haproxy/nginx root masters, east's .deb packages still carry the old unit until rebuilt (drop-ins cover it).
