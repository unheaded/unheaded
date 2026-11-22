# Host Settings

What a host's layout is made of, where each value lives, and how to set it for another host. Nothing about one host is written into code: a host's values are in files named after it, or in an env file on the host. West is the worked example (ADR-147 plan 4, S-WEST-GROUND; Stevie, 2026-11-17: "document and ensure this isn't hard coded, and is configurable").

## One manifest per host

`deploy/host-settings/<host>.tsv` lists every host file the repository is the source for: host path, repository path, mode (tab-separated). `scripts/check-host-settings.sh --host <host>` compares them and prints `OK`, `DRIFT` or `SKIP` with the path, never a line of a file.

| Mode | Meaning |
|---|---|
| `exact` | byte for byte; for a file under `/etc/sysctl.d/` each key is also read from the running kernel |
| `contains` | every non-comment line of the repository file is a whole line of the host file; for `/etc/fstab` each bind target must also be mounted |
| `redact` | keys and passwords masked on both sides, then compared (WireGuard, FRR) |
| `absent` | the host path must not exist |
| `swap` | the host path is an active swap; the `swap` rows are the host's whole active swap list (no `swap` row, no swap check) |

Run with no argument, the script checks the repository instead: every manifest parses, every repository path exists, no sysctl source reserves huge pages (ADR-147 decision 12), no slice file sets a share (decision 13), and the unit below names no host and no home directory.

## West's files

| Host path | Repository file | What to change for another host |
|---|---|---|
| `/etc/sysctl.d/99-unheaded.conf` | `deploy/sysctl.d/99-unheaded-west.conf` | Copy to `99-unheaded-<host>.conf`; `scripts/install-prerequisites.sh --with sysctl` installs the one named after the host (`SYSCTL_SOURCE=` overrides) when the host has none, and refuses when no such file exists. It does not replace a host file that differs and does not apply the values: `sysctl --system`, or the next boot |
| `/etc/fstab` (lines) | `deploy/host-settings/west-fstab.fragment` | The volume (`/dev/ubuntu-vg/unheaded-data` on `/srv/unheaded-data`), the five store binds, and the backup bind (`/var/backups/srv-backups` on `/srv/backups`, on the other disk) |
| `/etc/systemd/system/docker.service.d/10-unheaded-data.conf` | `deploy/host-settings/west-docker-data.conf` | The five bind targets again: Docker waits for them. They follow from Docker's data root, the compose project name (`unheaded-dev`) and the monitoring directory's name |
| `/usr/local/lib/unheaded/nft/west.nft` | `deploy/nftables/west.nft` | Copy to `<host>.nft` and edit only the `define` block at its top (below) |
| `/etc/wireguard/wg0.conf`, `/etc/frr/frr.conf` | `config/wireguard/west.conf`, `routing/frr/west.conf` | Per host by name |
| the four slices | `deploy/systemd/*.slice` | The same on every host; no numbers until plan 8 |
| `/swap.img` | (a `swap` row) | The host's own swap list |

## The input filter's values

`deploy/nftables/west.nft` names each interface, address and port set once, in a `define` block; the rules use the names. The loaded ruleset is the same as before the block existed (both loaded in a private namespace and compared, 2026-11-17).

| Name | West's value | What it is |
|---|---|---|
| `LAN_IF`, `LAN_V4` | `wlp17s0`, `192.168.69.128/25` | the LAN interface and the prefix that is offered services |
| `PEER_IF`, `PEER_V4` | `enp16s0`, `192.168.13.1` | the link to the second host and its address |
| `OVERLAY_IF` | `wg0` | the WireGuard overlay |
| `LXD_IF`, `DOCKER_IF`, `DOCKER_BR_IF` | `lxdbr0`, `docker0`, `br-*` | container bridges |
| `ADMIN_V4` | `192.168.69.132` | the one LAN host let in to LXD's API (`LXD_API_TCP`, 8443) |
| `SSH_TCP`, `LAN_TCP` | 7375; ssh, 80, 443, 18888, 16666, 20000 to 20002, 20103 | what the LAN may reach on the host |
| `LAN_FWD_PORTS` | 53, 3000, 3001, 16666, 20000 to 20002, 20103 | Docker-published ports the LAN may reach |
| `SCRAPE_TCP`, `TO_PEER_TCP` | 8428, 9100, 9110, 9323, 16666-26666; 9100, 9110, 16666-26666 | bridges to the host; containers to the second host |
| `LXD_UDP`, `LXD_DNS_TCP`, `SYSLOG_PORT` | 53 and 67; 53; 514 | containers to the LXD bridge |

The two rates (ping 10 a second, drop logs 5 a minute) stay in the rules: nft takes only a literal number after `limit rate`. Apply a ruleset with `scripts/host/nft-apply-confirm.sh FILE [SECONDS]` (it reverts by itself unless confirmed; state under `UNHEADED_NFT_STATE`, default `/var/lib/unheaded/nft`).

## The daily drift check

`unheaded-host-settings.timer` runs the check as root and writes `unheaded_host_settings_drift_files` for `HostSettingsDrift`. One unit for every host; its values come from `/etc/unheaded/host-settings.env` (example: `deploy/systemd/host-settings.env.example`):

| Variable | Default | What it is |
|---|---|---|
| `HOST_SETTINGS_REPO` | none; required | the checkout that holds the manifest |
| `HOST_SETTINGS_HOST` | the host's short name | which manifest |
| `HOST_SETTINGS_TEXTFILE` | `/var/lib/unheaded/textfile` | node_exporter's textfile directory (changing it also needs `ReadWritePaths=` in a drop-in) |

The schedule (10 minutes after boot, then daily) is a systemd timer: `systemctl edit unheaded-host-settings.timer`. Runbook: `runbooks/infra/host-settings-drift.yaml`.

## The boot record

`unheaded-boot-event.service` records each boot of the host for the daily digest ([[Alerting]], "A boot of the host"). One unit for every host; it reads the same `/etc/unheaded/host-settings.env` when the file is there, and every key has a default:

| Variable | Default | What it is |
|---|---|---|
| `BOOT_EVENT_HOST` | the host's short name | the name in the record and in the digest's line (`west booted ...`) |
| `BOOT_EVENT_TEXTFILE` | `/var/lib/unheaded/textfile` | huginn's textfile directory: `host_boot.prom` is written there and `planned_stop.prom` read there (changing it also needs `ReadWritePaths=` in a drop-in) |
| `BOOT_EVENT_STACK_PROJECTS` | `unheaded-dev` | the compose projects that are "the stack" in the record and in the digest's `stack down` / `stack up since`; the monitoring project is left out so that the mailer started alone does not read as "stack up" |
| `BOOT_EVENT_ONESHOT` | `volume-init clickhouse-schema` | compose services that never count as stopped: they exit 0 by design (the default is `HOST_STATE_ONESHOT` of `host-state-textfile.sh`), or run only on demand, such as a deploy service that is up only during a deploy. The record carries the list and the mailer applies it when it reads the stack's state for the digest; a service that is stopped by design and not listed makes the line read `stack partly up` |
| `BOOT_EVENT_KEEP` | 32 | boots kept in `/var/lib/unheaded/boot-event/boots.jsonl` (1 to 500; the mailer reads the newest 64) |
| `BOOT_EVENT_DOCKER_WAIT` | 20 | seconds Docker gets to answer at boot, per project (1 to 50; the unit's start timeout is 60 s); past it the boot is recorded with the stack `unknown` |

The record's directory is the unit's `StateDirectory` (`/var/lib/unheaded/boot-event`). The mailer finds it and the textfile directory through `ALERT_MAILER_BOOT_DIR` and `ALERT_MAILER_TEXTFILE_DIR` in `monitoring/.env` (unset: no boot line), and `boot.state_max_age` in `observability/alerting/policy.yaml` (5m) is how old `host_state.prom` may be and still count as the stack's state now.

## Other values a deployment sets

| What | Where | Default |
|---|---|---|
| Which networks reach the kanban task list and the dashboard's `/metrics` without a credential (plan 4, WG3) | `AUTH_ANONYMOUS_FROM` in the compose secrets file; see [[Security]] | unset in the code: everyone. The compose files (same lane) default to loopback and the Docker ranges; a link to a second host or an overlay is added per deployment, as the example in `db/.env.example` shows for west |
| Where backup sets go, and whether they must be on another disk than the data | `WELL_BACKUP_DIR`, `WELL_BACKUP_OTHER_DISK`, `CH_BACKUP_OTHER_DISK`, `VM_BACKUP_OTHER_DISK`; see [[Well-Backups]] | `/srv/backups/...`; the guard is on in the units |
| Each container's pid limit and the slice containers run in (plan 4 Phase 7; on `sprint/s-west-ground-compose` until that phase merges it) | `UNHEADED_PIDS_LIMIT`, `UNHEADED_CH_PIDS_LIMIT`, `UNHEADED_CGROUP_PARENT` in the compose env file, and again in `monitoring/.env` for the monitoring stack | 512, 2048, `unheaded-env.slice` |
| What `scripts/check-compose-pids.sh` holds the limits against (same lane) | `COMPOSE_PIDS_THREADS_MAX`, `COMPOSE_PIDS_SHARE_PERCENT`, `COMPOSE_PIDS_PER_SERVICE_MAX`, `COMPOSE_PIDS_PER_FILE_MAX`, `COMPOSE_PIDS_SLICE` | 115473 (west's `kernel.threads-max`), 15, 4096, 20000, `unheaded-env.slice` |
| A Well role's connection limit | `<ROLE>_CONNECTION_LIMIT` and `scripts/well/set-role-limits.sh`; see [[The-Well]] | the ledger's (app_zhen 20) |
| What is mailed | `mailer.delivery`, `mailer.immediate`, the caps; see [[Alerting]] | one digest a day |

## The prerequisites installer

`scripts/install-prerequisites.sh` puts on a fresh Ubuntu host what building, testing and the compose stack need (`QUICKSTART.md`). `--dry-run` prints each step and changes nothing; the real run needs root. The script itself starts, enables and restarts nothing (Ubuntu's Docker packages start the Docker daemon when installed), applies no kernel setting, and leaves alone a file or toolchain that is already there and differs (a `kept` line says what to do). A second run changes nothing. Parts: `packages`, `docker`, `go`, `rust` by default (`--without PART`); `ebpf`, `node-exporter`, `sysctl` on request (`--with PART`).

Every value is a variable at the top of the script; one given in the environment replaces the default (`sudo env NAME=value scripts/install-prerequisites.sh ...`):

| Variable | Default | What it is |
|---|---|---|
| `UBUNTU_MIN` | `24.04` | the oldest Ubuntu release it runs on; any other system is refused |
| `BASE_PACKAGES` | `ca-certificates curl git make gcc libc6-dev python3 jq` | part `packages` |
| `DOCKER_PACKAGES` | `docker.io docker-compose-v2 docker-buildx` | installed only when no docker is on the host; a host that takes Docker from another repository installs it first |
| `DOCKER_DAEMON_JSON`, `DOCKER` | `/etc/docker/daemon.json`, `docker` | where `deploy/docker/daemon.json` goes; the command asked whether Docker and its compose plugin are already there |
| `GO_VERSION`, `GO_SHA256` | `go.mod`'s toolchain line and that release's sha256 | changed together; `tests/scripts/test_install_prerequisites.sh` fails when the version is not `go.mod`'s |
| `GO_BASE_URL`, `GO_INSTALL_DIR`, `GO_PROFILE` | `https://go.dev/dl`, `/usr/local`, `/etc/profile.d/golang.sh` | where the archive comes from, where `go/` is unpacked, the file that puts it on `PATH` |
| `RUST_PACKAGES` | `rustup` | part `rust`; toolchains are installed by each user (`rustup default stable`) |
| `EBPF_PACKAGES`, `BPF_LINKER_VERSION` | `bpftool`, `0.10.2` | part `ebpf`; the `bpf-linker` version is printed in the command for the user, not installed |
| `NODE_EXPORTER_VERSION`, `NODE_EXPORTER_SHA256` | `1.12.1`, none | part `node-exporter`. The repository records no sha256 for the archive yet: the part refuses to download until one is given, from the release's `sha256sums.txt` |
| `NODE_EXPORTER_BASE_URL`, `NODE_EXPORTER_BIN`, `NODE_EXPORTER_UNIT`, `NODE_EXPORTER_USER`, `SYSUSERS_TARGET` | the project's GitHub releases, `/usr/local/bin/node_exporter`, `/etc/systemd/system/node-exporter.service`, `node-exporter`, `/etc/sysusers.d/unheaded.conf` | the unit is installed and not started; the account comes from `deploy/sysusers.d/unheaded.conf`, which makes every service account in it |
| `SYSCTL_SOURCE`, `SYSCTL_TARGET` | `deploy/sysctl.d/99-unheaded-<host>.conf`, `/etc/sysctl.d/99-unheaded.conf` | part `sysctl` (West's files, above) |

## The resolver's blocklists (Pi-hole)

West resolves through a Pi-hole in the LXD instance `pihole` (10.10.20.252, [[ADR-022-Pihole-Docker-to-LXD]]). Its blocklists are host data, not repository files:

| List | Since | Entries |
|---|---|---|
| StevenBlack `hosts` | before 2026-11 | 77,310 |
| HaGeZi Multi Normal (`adblock/multi.txt`) | 2026-11-21 22:52Z | 195,083 |

A list is added with `scripts/pihole-add-list.sh <url> "<comment>"`, started detached (`setsid nohup ... &`), because the shell that starts it resolves through the same Pi-hole. The script copies `gravity.db`, adds the list, runs gravity, and puts everything back by itself if a name on its `MUST` list stops resolving, if a name used in the last 7 days that matches `CRITICAL` is newly blocked, or if the list loads nothing. Its settings (`PIHOLE_INSTANCE`, `PIHOLE_IP`, `LXC`, `PIHOLE_LIST_STATE`, `MUST`, `CRITICAL`, `TELEMETRY_OK`) are variables with defaults at the top of the file. Each run leaves a directory under `~/.local/state/unheaded/pihole-lists/` with the blocked share of the day before the change, the names in use and the ones the new list blocks.

Two things learned on 2026-11-21: the Pi-hole refuses a client everything for at least ten seconds once it passes 1,000 queries a minute (`dns.rateLimit`), so a check that asks about every used name must be spaced; and a blocklist blocks telemetry hosts under domains that matter (`collector.github.com`), which is what it is for.

## A second host

1. Copy `deploy/host-settings/west.tsv` to `<host>.tsv` and keep only the rows the host has.
2. Copy each west-named file a row points at (`99-unheaded-west.conf`, `west-fstab.fragment`, `west-docker-data.conf`, `west.nft`) to the host's name and edit it; for the filter, only the `define` block.
3. `scripts/check-host-settings.sh` (the repository check), then on the host `scripts/check-host-settings.sh --host <host> --allow-skip`.
4. Install the unit, the timer and `/etc/unheaded/host-settings.env` with `HOST_SETTINGS_REPO` set.

## Older host scripts

Bring-up and check scripts from the first bare-metal builds (host A "the Forge" = west, host B "the Outpost" = east). They predate the manifest above; read one before running it.

| Script | What |
|---|---|
| `scripts/setup-host.sh` | Generic host setup for a Linux machine (bare metal or VM), kernel 5.8 or newer, as root |
| `scripts/pre-flight-check.sh [--strict]` | Prerequisites before a bare-metal deployment (run before `nixos-rebuild switch`); `--strict` makes the optional checks blocking |
| `scripts/capture-baseline.sh [--host-role forge\|outpost] [--output-dir DIR]` | JSON and text snapshot of CPU, memory, disk, network, kernel and eBPF support before a deployment |
| `scripts/bare-metal/preflight-host-a.sh`, `preflight-host-b.sh` | Hardware and software prerequisites per host before the boot sequence; exit 1 on a failure |
| `scripts/bare-metal/validate-host-a.sh`, `validate-host-b.sh [--json]` | Post-boot validation per host; firewall API credentials from the environment |
| `scripts/east-bootstrap.sh [phase]` | East bring-up in phases: `preflight`, `wireguard`, `bgp`, `services`, `observability`, `verify`, `all`; it assumes NixOS on east, which runs Ubuntu today |
