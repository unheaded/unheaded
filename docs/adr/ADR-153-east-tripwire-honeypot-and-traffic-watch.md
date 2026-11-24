<!--
SPDX-License-Identifier: GPL-3.0-or-later
Copyright (c) 2024-2026 Stevie Bellis.
-->
# ADR-153: East tripwire: a honeypot and a traffic watch

**Status:** Proposed. The battle plan was written on Stevie's word, 2026-11-24, with the Captain (why) and the BlackMage (threat model) seats. Nothing is built, no branch exists, no host has changed. Execution waits for Stevie's go and his answers under "To settle". It is not part of the ADR-147 series, and the ADR-112 freeze does not hold it: Stevie asked for it by name.
**Date:** 2026-11-24
**Related:** ADR-098 (findings register; `inet unheaded` table, commit-confirm), ADR-100 (detection alerts are metadata), ADR-117 (Prometheus evaluates, VictoriaMetrics stores), ADR-147 decisions 3, 17, 18 (east is a real second home), ADR-150 (east and west as a pair; east has no uplink of its own), `observability/alerting/policy.yaml` (`mailer.immediate`), `monitoring/prometheus/rules/detection.yml`.

## The ask (Stevie, 2026-11-24)

> "let's make a honey pot service on east - and closely monitor it, right now it has 2 connections at ip layer to west ipv6 and ipv4 but otherwise no internet or outbound/inbound traffic should really show up"

His answers the same day:

| Question | Answer |
|---|---|
| Where the code lives | Unheaded `develop`: a first-party Go service with its own nologin account, a unit, and metrics. Alerts go through Alertmanager and then alert-mailer |
| Decoys | Fake SSH on 2222, plus common ports. "Map the defaults into a config file allowing them to be disabled or set to other ports to make end users able to adopt to their ecosystem if desired - yaml" |
| A hit reaches him | Immediate mail. The honeypot is an IDS, so it is the exception to the one-digest-a-day rule |
| Traffic outside the two links | Alert on it, "but account for running apt-update and stuff like that .... it should be configureable for multiple dynamic use cases" |

## Why (Captain)

East has one cable, and its only neighbour is west (`docs/adr/ADR-150-east-and-west-as-a-pair.md`, "What links them"). Any packet that reaches a decoy on east was sent by something on west or on the lab behind it. That makes east a cheap, quiet place to catch lateral movement, and the honeypot answers one question: did something inside the Kingdom go looking? Legitimate traffic never reaches a decoy, so a hit is close to certain evidence, and the alert can mail at once without flooding Stevie.

The traffic watch answers the second question: did east talk to anyone it should not have? Today the answer would only show up as a byte counter (`OutboundVolumeDeviation` in `monitoring/prometheus/rules/detection.yml`). The watch names each flow class instead.

For adopters (small shops, see the project memory on audience), both pieces come as one YAML file each, with defaults that work on a host with one uplink. They map to NIST SP 800-53 SC-26 (decoys) and SI-4(4) and SI-4(11) (inbound and outbound traffic monitoring), which the compliance bundle can cite. Nothing third-party is added: the SSH decoy uses `golang.org/x/crypto/ssh`, already a direct dependency (`go.mod:38`), and YAML uses `gopkg.in/yaml.v3` (`go.mod:44`).

## What east looks like today (measured 2026-11-24, after the 26.04 upgrade)

| Fact | Evidence |
|---|---|
| Ubuntu 26.04.1 LTS, kernel 7.0.0-44 | `ssh east uname -r` |
| Links: `enp2s0` 192.168.13.1/30 (west 192.168.13.2); `wg0` fd00:dead:beef::2 (peer ::1). Other NICs down | `ip -br addr` |
| Default route **via west** (192.168.13.2): east reaches the internet through west's NAT | `ip -br route` |
| DNS: 10.10.20.252, 1.1.1.1, 8.8.8.8 | `resolvectl dns` |
| Time: chrony to `{1..4}.ntp.ubuntu.com` and `ntp-bootstrap.ubuntu.com` with NTS (TCP 4460, then UDP 123) | `/etc/chrony/sources.d/ubuntu-ntp-pools.sources` |
| apt: Ubuntu's archive through west; west's repo `192.168.13.2:18888` (source disabled until it serves `resolute`) | `/etc/apt/sources.list.d/` |
| Established flows: only to 192.168.13.2 (ssh, Huginn push to 8428) and fd00:dead:beef::1 (scrapes, BGP, services) | `ss -tunp state established` |
| Firewall: only `inet unheaded_chaos_guard`. `deploy/nftables/east.nft` (default deny inbound) is **not applied** | `nft list ruleset` |
| sshd listens on 22 (east.nft expects 7375) | `ss -ltnp` |

So the claim that east has "no internet" is not quite true today. DNS, NTP and apt go out through west. The watch's default profile has to allow exactly those, and nothing else.

## Threat model (BlackMage)

| # | Adversary or abuse | What it tries | Answer in this design |
|---|---|---|---|
| T1 | Code on west (a container, a compromised service) sweeps east | Port scan, SSH spraying | Every decoy port records the touch → `HoneypotTouched` pages. The watch's `probe_in` class counts inbound to ports nothing serves |
| T2 | Attacker fingerprints the honeypot and avoids it | Banner and timing checks | The decoys are tripwires, not research: a single SYN that completes counts. Banners are configurable to match the host's real versions |
| T3 | Flooding the honeypot to exhaust east (7 GiB, 4 cores) | Many connections or slowloris | Caps on total and per-source connections; read deadline; byte cap; `MemoryMax`, `TasksMax`; an nft meter rate-limits new connections to decoy ports |
| T4 | Exploiting the honeypot itself | Parser bugs in SSH or banner handling | Go, no exec, no shell, no file writes beyond its state dir. Zero capabilities. `SystemCallFilter=@system-service`. Fuzz targets on every reader. The kernel drops every outbound connection the honeypot account would start (nft `meta skuid`) |
| T5 | A compromised honeypot binds a real port (22) to phish logins | Needs `CAP_NET_BIND_SERVICE` | The honeypot gets **no** capability: low decoy ports reach it by an nft `redirect` to high internal ports |
| T6 | Log or alert injection | Control characters, huge usernames | Every attacker-supplied string is bounded and escaped before the journal. Metrics carry no attacker text and no source address (also bounds label cardinality) |
| T7 | A real password typed into the decoy by mistake | An operator fat-fingers `-p 2222` | Passwords are never stored. A keyed HMAC (host key, 0600) lets repeats be correlated without keeping the secret; the length is logged |
| T8 | Root on east blinds the sensors | Flushes the tables, stops the units | `TrafficWatchBlind` (table missing) pages. Honeypot `up == 0` and a stale sampler raise a ticket. Huginn and node_exporter already report the host |
| T9 | Mail flooding by repeated hits | Trigger the page in a loop | Alertmanager grouping, plus the mailer's own caps (`max_per_hour: 3`, `max_per_day: 6`, `observability/alerting/policy.yaml:49-52`). The flood itself is the signal |
| T10 | Exfiltration hidden in allowed classes | DNS tunnelling to 1.1.1.1; HTTPS while the apt window is open | Allowed classes are still counted per class and logged at a rate. Windows close in the kernel by timeout. Per-class byte rates go to VictoriaMetrics for a later deviation rule (out of scope here, item 9 in "Not established") |
| T11 | The watch locks the host out | A bad ruleset drops ssh | The watch **never drops**: it only counts and logs, at priority after the filter. The only drop is the honeypot account's own egress |

Severity by the BlackMage scale: T4 and T5 are HIGH if missed (attacker code on east). T1 and T8 are the reason this exists. T10 is a MEDIUM gap that stays recorded.

## Design

Two programs, two YAML files, and two nft tables, each owned by its package. Nothing touches `inet unheaded` (ADR-098's table).

### `cmd/honeypot`: `unheaded-honeypot.service`

- **Account:** its own account, `unheaded-honeypot` (nologin, `deploy/sysusers.d/unheaded.conf`). No capabilities. The state directory `/var/lib/unheaded/honeypot` holds the generated SSH host key and the HMAC key, both 0600.
- **Listening:** on the configured addresses (default: both of east's link addresses). Decoys below 1024 bind `internal_port_base + port`, and the nft table redirects to them.
- **Decoy kinds:**
  - `ssh`: a real handshake with `x/crypto/ssh`. It records the client version, usernames and the method tried, and every authentication fails. Passwords are kept only as an HMAC and a length.
  - `banner`: writes the configured banner, then reads up to `read_bytes`.
  - `http`: one fixed 401 reply after reading the request head.
  - `silent`: accepts and reads.
  - Every connection is counted at accept time, before a single byte arrives.
- **Metrics:** served on `metrics_listen` (default `[fd00:dead:beef::2]:19150`).
  - `unheaded_honeypot_connections_total{decoy}`
  - `unheaded_honeypot_auth_attempts_total{decoy,method}`
  - `unheaded_honeypot_bytes_total{decoy}`
  - `unheaded_honeypot_refused_total{reason}` (caps hit)
  - `unheaded_honeypot_decoys{decoy,port}` (1 per enabled decoy)
  - No address labels.
- **Journal:** one structured line per connection. It carries the source address and port, the decoy, the duration and the byte count, the sanitized and bounded first `log_bytes` bytes (hex-escaped), and for SSH the client version, the usernames and the password HMACs.

`/etc/unheaded/honeypot.yaml`. Every key is optional; the shipped file (`deploy/honeypot/honeypot.yaml`) shows the defaults:

```yaml
listen: ["192.168.13.1", "fd00:dead:beef::2"]   # [] = every address
metrics_listen: "[fd00:dead:beef::2]:19150"
internal_port_base: 42000        # decoy port p < 1024 is served on base+p and redirected by nft
limits:
  max_conns: 64                  # open at once, all decoys together
  max_conns_per_source: 4
  new_conns_per_second: 20       # nft meter on decoy ports, per source
  read_timeout: 10s
  read_bytes: 4096
  log_bytes: 256
state_dir: /var/lib/unheaded/honeypot
decoys:
  - {name: ssh,      kind: ssh,    port: 2222, banner: "SSH-2.0-OpenSSH_9.6p1 Ubuntu-3ubuntu13.5"}
  - {name: telnet,   kind: banner, port: 23,   banner: "\r\nUbuntu 24.04 LTS\r\nlogin: "}
  - {name: http,     kind: http,   port: 80}
  - {name: http-alt, kind: http,   port: 8080}
  - {name: smb,      kind: silent, port: 445}
  - {name: rdp,      kind: silent, port: 3389}
  - {name: postgres, kind: silent, port: 5432}
  - {name: redis,    kind: banner, port: 6379, banner: "-NOAUTH Authentication required.\r\n"}
# A decoy: enabled: false to switch it off; port: N to move it. Names are unique, ports are unique,
# and a port that a real listener holds at start is refused with the reason (never stolen).
```

The nft table `inet unheaded_honeypot` is rendered from that file by `honeypot render-nft`. It holds three things: the redirects for low ports, the per-source meter, and `meta skuid unheaded-honeypot ct state new drop` on output.

### `cmd/unheaded-watch`: the traffic watch

- **Table:** `inet unheaded_watch`, rendered from `/etc/unheaded/watch.yaml` by `unheaded-watch render`. The table counts and logs and never drops: chains at priority `filter + 10`, policy accept. Each packet that starts a flow (`ct state new`, plus untracked packets) is classified once, into the first class that matches:
  - `peer`: the configured links.
  - `profile:<name>`: a standing profile.
  - `window:<name>`: an open window.
  - `decoy`: a honeypot port.
  - `probe_in`: inbound to a closed port from a peer.
  - `unexpected_in` and `unexpected_out`: everything else.
  - Every class has a named counter. Unexpected classes are also logged (`log prefix "unheaded-watch: "`, at `log_rate`).
- **Windows:** each window is a set typed `ipv4_addr . inet_proto . inet_service` (and the `ipv6_addr` twin) with `flags interval, timeout`.
  - Opening one (`unheaded-watch open apt [30m]`) adds that window's elements with a timeout, and the kernel removes them when the time runs out. A window forgotten open closes by itself.
  - `max` caps any duration asked for.
  - The `open_on: [apt]` trigger installs `deploy/apt/99unheaded-watch`, an `APT::Update::Pre-Invoke` and `DPkg::Pre-Invoke` hook. So `apt update`, unattended-upgrades and `apt-daily` open the window themselves.
- **Sampler:** `unheaded-watch-sample.service` is a long-running process under its own account, `unheaded-watch`. It has `CAP_NET_ADMIN` only and **no network sockets** (`RestrictAddressFamilies=AF_NETLINK AF_UNIX`, `IPAddressDeny=any`). Every `every` (default 10 s) it reads the counters and set elements through `nft -j` and writes them to `/var/lib/unheaded/textfile/watch.prom`, which Huginn already serves (`cmd/huginn/textfile.go`):
  - `unheaded_watch_packets_total{class,direction}`
  - `unheaded_watch_bytes_total{class,direction}`
  - `unheaded_watch_window_open{window}`
  - `unheaded_watch_table_present`
  - `unheaded_watch_mode{mode}`
  - `unheaded_watch_sample_ok`
- **Loading:** one template unit, `unheaded-nft-table@.service` (`@honeypot`, `@watch`), loads a rendered table at boot. It is modelled on `deploy/systemd/unheaded-nft.service`: `CAP_NET_ADMIN` only, and the file is handed in on stdin.

`/etc/unheaded/watch.yaml` defaults (`deploy/watch/watch.yaml`):

```yaml
mode: learn                    # learn: count and log, rules do not page; alert: they page
every: 10s
log_rate: 10/minute
textfile: /var/lib/unheaded/textfile/watch.prom
interfaces: [enp2s0, wg0]      # [] = all
peers:                         # the expected links: always fine, both ways
  - {name: west-p2p, address: 192.168.13.2}
  - {name: west-wg,  address: "fd00:dead:beef::1"}
  - {name: link-local, address: "fe80::/10"}
profiles:                      # standing, outbound unless in: is used
  - name: dns
    out: [{proto: udp, port: 53, to: [10.10.20.252, 1.1.1.1, 8.8.8.8]},
          {proto: tcp, port: 53, to: [10.10.20.252, 1.1.1.1, 8.8.8.8]}]
  - name: ntp
    out: [{proto: udp, port: 123}, {proto: tcp, port: 4460}]
windows:                       # opened on demand; the kernel closes them
  - name: apt
    default: 30m
    max: 2h
    open_on: [apt]
    out: [{proto: tcp, port: 80}, {proto: tcp, port: 443},
          {proto: tcp, port: 18888, to: [192.168.13.2]}]
  - name: fwupd
    default: 15m
    max: 1h
    out: [{proto: tcp, port: 443}]
```

An omitted `to:` means any address. A window or profile can be removed, or new ones added (`snap`, `git`, a backup target), without code.

### Alerts (`monitoring/prometheus/rules/tripwire.yml`)

| Alert | Expression (outline) | Urgency | Mail |
|---|---|---|---|
| `HoneypotTouched` | `increase(unheaded_honeypot_connections_total[2m]) > 0` | page | `mailer.immediate` |
| `HoneypotCredentialTried` | `increase(unheaded_honeypot_auth_attempts_total[2m]) > 0` | page | `mailer.immediate` |
| `EastTrafficUnexpected` | `increase(unheaded_watch_packets_total{class=~"unexpected_.*\|probe_in"}[2m]) > 0 and on(host) unheaded_watch_mode{mode="alert"} == 1` | page | `mailer.immediate` |
| `TrafficWatchBlind` | `unheaded_watch_table_present == 0` for 1m | page | `mailer.immediate` |
| `HoneypotDown` | `up{job="honeypot"} == 0` for 5m | ticket | digest |
| `TrafficWatchStale` | `time() - huginn_textfile_mtime_seconds{file="watch.prom"} > 60` for 5m | ticket | digest |

Mail and labels carry no attacker text. The mail names the host and the decoy or class, and points at `journalctl -u unheaded-honeypot` or `journalctl -k -g unheaded-watch:` on east.

## To settle (Stevie)

| # | Choice | Options | Recommendation |
|---|---|---|---|
| H1 | East's DNS and time | (a) allow the outside resolvers and Ubuntu's NTS pools as standing profiles; (b) point east at west for both, so the only standing peers are west's two addresses | **(a) now**, as configured profiles; (b) belongs to S-EAST-PAIR, and ADR-150 gives east its own uplink later, which changes this file and no code |
| H2 | Low decoy ports | (a) nft redirect to high ports, honeypot with no capability; (b) `CAP_NET_BIND_SERVICE` | **(a)**: T5 |
| H3 | Passwords tried | (a) HMAC and length only; (b) clear text in the journal | **(a)**: T7 |
| H4 | Learn period before `mode: alert` | 24 h, 48 h, 72 h | **48 h**, then review the classes with Stevie once |
| H5 | `HoneypotDown` and `TrafficWatchStale` | ticket (digest) or page (immediate) | **ticket**: a sensor going quiet is not proof of intrusion, while a missing table is (`TrafficWatchBlind` pages) |
| H6 | Loader account | reuse `unheaded-nft` for `unheaded-nft-table@` (same job: load one table, `CAP_NET_ADMIN`), or a new account per table | **reuse**: one job, one account. The honeypot and the watch each still get their own |
| H7 | Delivery to east | (a) `.deb` from west's repo once it serves `resolute`; (b) a built binary installed by hand, as orb was | **(b) for the first run**, (a) once west is upgraded |
| H8 | Mail drill | the plan sends one real immediate mail (Phase 12) | **yes**, attended or announced |

## Not established

1. Whether nft on 7.0 accepts `interval, timeout` on the concatenated window sets: Step 74 checks it, and Step 75 is the fallback.
2. Whether Alertmanager's current route groups the new pages into one mail per burst: Step 100 reads `monitoring/alertmanager/`.
3. The real Ubuntu archive addresses used through west (CDN). The apt window allows any address on 80 and 443 while open, on purpose.
4. Whether unattended-upgrades on 26.04 runs the `APT::Update::Pre-Invoke` hooks: Step 89 checks it.
5. Whether Huginn on east has `textfile_dir` set: Step 110 checks it.
6. The IPv6 router and neighbour discovery noise on `enp2s0` during learn: Phase 11 reads it.
7. sshd on 22 while `east.nft` expects 7375: out of scope, recorded only.
8. Applying `east.nft` itself (ADR-098 step 6): out of scope. The tripwire works with or without it.
9. A per-class byte-rate deviation rule for T10: out of scope, after 30 days of data.
10. orb runs on east since 2026-11-24 and fetches its two feeds over HTTPS (the user's `~/.config/orb/config`). Those flows count as `unexpected_out` unless east's own `/etc/unheaded/watch.yaml` gets an `orb-feeds` profile. That profile names the feed host's addresses, which are private, so it is never written into `deploy/watch/watch.yaml` or any file in this repository. Phase 11's review must find these flows and move them into that profile.

---

# S-EAST-TRIPWIRE BATTLE PLAN: 13 Phases, 138 Steps

**Date:** 2026-11-24
**Sprint:** S-EAST-TRIPWIRE: a honeypot and a traffic watch on east, with IDS alerts that mail at once
**Prerequisite:** this ADR's "To settle" answered; Stevie's go; east reachable as `ssh east` with passwordless `sudo -n`; develop clean except `demos/doom/`
**Target:** a touch on any decoy, or any east flow outside its peers, profiles and open windows, mails Stevie within about 3 minutes. apt runs never alert, and every default can be changed in YAML.
**Estimated Duration:** about 10 to 12 hours of work over 3 sessions, plus the 48 h learn period (H4)
**Agent Strategy:** Phases 2 to 5 (honeypot) and 6 to 7 (watch) can run in parallel in two lanes; everything else is the coordinator's job (sudo on east, alerting, drills)
**Commit Cadence:** every 5 steps, and at every exit gate (138 / 20 → 5). Stage named files only (`git add <paths>`), and every commit is signed
**Stuck Protocol:** skip after 3× the step estimate or 2 failed debug attempts; write a `[STUCK]` marker, commit, and continue with the next independent step

## LEGEND

[B] Bash · [V] Verification (must pass) · [D] Debug (only if the prior step failed) · [W] Write file · [R] Read · [S] sudo (on east unless said) · [P] parallel with other [P] steps · [C] commit checkpoint · [STUCK] skipped by the Skip Protocol · [BLOCKED] blocked by a STUCK step

Paths are relative to `/home/govan/tmp/unheaded` unless absolute. `E` means `ssh east`.

## DEPENDENCY MAP

```
P0 ─ P1 ─┬─ P2 ─ P3 ─ P4 ─ P5 ─┐
         └─ P6 ─ P7 ───────────┼─ P8 ─ P9 ─ P10 ─ P11 (48 h) ─ P12 ─ P13
```
Critical path: P0 → P1 → P2 → P3 → P4 → P5 → P8 → P9 → P10 → P11 → P12 → P13.

---

## PHASE 0: PREFLIGHT (Steps 1-8)

**Goal:** the repository, the toolchain and east are in the state this plan assumes.
**Prerequisite:** none · **Time:** 15 min · **Agent:** Coordinator

- [ ] **Step 1** [B] ~1m: Branch from develop.
  ```bash
  cd /home/govan/tmp/unheaded && git switch develop && git status -sb | head -3 && git switch -c sprint/s-east-tripwire
  ```
- [ ] **Step 2** [V] ~30s: The tree is clean apart from `?? demos/doom/`. If not, STOP and ask: never stash Stevie's work.
- [ ] **Step 3** [B] ~30s: Toolchain.
  ```bash
  go version && grep -n "golang.org/x/crypto \|gopkg.in/yaml.v3" go.mod && which promtool nft || true
  ```
- [ ] **Step 4** [V]: Go ≥ 1.27, both modules present (no new dependency). `promtool` present on west (or the container route in `docs/battle-plans/` CI notes; see `reference_ci_repro_recipe`).
- [ ] **Step 5** [B] ~1m: East's state, recorded for later comparison.
  ```bash
  ssh east 'uname -r; ip -br addr; ip -br route; sudo -n nft list tables; sudo -n ss -ltnpH | awk "{print \$4}" | sort' | tee /tmp/claude-1000/tripwire-east-before.txt
  ```
- [ ] **Step 6** [V]: kernel 7.0.x; `enp2s0` 192.168.13.1/30; `wg0` fd00:dead:beef::2; no table named `unheaded_honeypot` or `unheaded_watch`; none of ports 2222, 23, 80, 8080, 445, 3389, 5432, 6379, 19150 or 42000-43023 listening.
  - If a decoy port is taken → change that decoy's default port in Step 14's file, and note it in the ADR.
- [ ] **Step 7** [R] ~5m: Read the patterns this plan copies: `deploy/systemd/unheaded-nft.service`, `deploy/systemd/huginn.service` (sandbox block), `cmd/huginn/textfile.go`, `scripts/host/nft-apply-confirm.sh`, and `monitoring/prometheus/rules/detection.yml` with its test.
- [ ] **Step 8** [V]: **PHASE 0 EXIT GATE**: the branch exists, the toolchain is fine, east matches the ADR's "today" table, and the decoy ports are free.

## PHASE 1: SKELETON AND DEFAULT FILES (Steps 9-16)

**Goal:** the two commands exist and the shipped YAML defaults are in the repository.
**Prerequisite:** P0 · **Time:** 30 min · **Agent:** Coordinator

- [ ] **Step 9** [W] ~3m: `cmd/honeypot/main.go`: `cmdline.Parse(cmdline.Spec{...})` (ADR-111; `pkg/cmdline/policy_test.go` fails a binary that skips it), `--config` (default `/etc/unheaded/honeypot.yaml`), `--version`, subcommands `serve` (default), `check`, `render-nft` via `cmdline.ReportUnknownCommand`. SPDX header as in its neighbours.
- [ ] **Step 10** [W] ~3m: `cmd/unheaded-watch/main.go`: the same `pkg/cmdline` shape; subcommands `render`, `sample`, `open <window> [dur]`, `close <window>`, `status`, `check`; `--config` default `/etc/unheaded/watch.yaml`.
- [ ] **Step 11** [B] ~1m:
  ```bash
  go build -o /dev/null ./cmd/honeypot ./cmd/unheaded-watch && go vet ./cmd/honeypot ./cmd/unheaded-watch && go test ./pkg/cmdline -run Policy -count=1 && scripts/check-version-flag.sh 2>&1 | tail -3
  ```
- [ ] **Step 12** [V]: Both build and vet clean; the cmdline policy test and `check-version-flag.sh` pass with the two new binaries.
- [ ] **Step 13** [C]: `git add cmd/honeypot/main.go cmd/unheaded-watch/main.go && git commit -S -m "[PLAN S-EAST-TRIPWIRE] Steps 9-12: honeypot and unheaded-watch skeletons"`
- [ ] **Step 14** [W] ~3m: `deploy/honeypot/honeypot.yaml`: exactly the ADR's default block, with one comment line per key.
- [ ] **Step 15** [W] ~3m: `deploy/watch/watch.yaml`: exactly the ADR's default block, commented.
- [ ] **Step 16** [V]: **PHASE 1 EXIT GATE**: `python3 -c 'import sys,yaml;[yaml.safe_load(open(f)) for f in sys.argv[1:]]' deploy/honeypot/honeypot.yaml deploy/watch/watch.yaml && echo ok` prints `ok`. Commit both files (`[C]`).

## PHASE 2: HONEYPOT CONFIG, TEST FIRST (Steps 17-30)

**Goal:** a strict config loader. Unknown keys, duplicate names or ports, bad kinds and out-of-range limits are all refused.
**Prerequisite:** P1 · **Time:** 60 min · **Agent:** Agent [P] (lane A)

- [ ] **Step 17** [W] ~5m: `cmd/honeypot/config_test.go`. It covers: the defaults load; an empty file gives the defaults; `KnownFields(true)` refuses unknown keys; duplicate decoy name; duplicate port; port 0 and 65536; kind `rce`; a banner over 512 bytes; `read_bytes` > 1 MiB; `log_bytes` > `read_bytes`; `max_conns_per_source` > `max_conns`; `internal_port_base + 1023` > 65535; a disabled decoy is skipped; a moved port is honoured.
- [ ] **Step 18** [B] ~30s: `go test ./cmd/honeypot -run Config -count=1 2>&1 | tail -5`
- [ ] **Step 19** [V]: RED: it fails to compile or fails (no implementation yet).
- [ ] **Step 20** [W] ~10m: `cmd/honeypot/config.go`: `Load(path) (Config, error)` with `yaml.v3` `Decoder.KnownFields(true)`. The defaults are the struct's zero values filled from one `defaults()`. Read through `io.LimitReader` at 64 KiB.
- [ ] **Step 21** [B]: `go test ./cmd/honeypot -run Config -count=1 -race 2>&1 | tail -5`
- [ ] **Step 22** [V]: GREEN.
  - [D] Step 22a: run with `-v` on the failing case. Duplicate checks belong in `validate()`, not in the decoder.
- [ ] **Step 23** [C]: commit `cmd/honeypot/config.go cmd/honeypot/config_test.go`.
- [ ] **Step 24** [W] ~5m: `FuzzLoadConfig` in `config_test.go`, seeded with the shipped file and the refusal cases.
- [ ] **Step 25** [B] ~2m: `go test ./cmd/honeypot -run xxx -fuzz FuzzLoadConfig -fuzztime 90s 2>&1 | tail -3`
- [ ] **Step 26** [V]: No crash. Any crasher → fix it and keep the corpus file under `testdata/fuzz/`.
- [ ] **Step 27** [W] ~3m: `honeypot check` prints each decoy with its port and listen address and exits 0, or names the first problem and exits 2.
- [ ] **Step 28** [B]: `go run ./cmd/honeypot check -config deploy/honeypot/honeypot.yaml`
- [ ] **Step 29** [V]: Eight decoys listed; `smb` shows `445 → 42445`.
- [ ] **Step 30** [V]: **PHASE 2 EXIT GATE**: config tests pass with `-race` and the fuzz run is clean. Commit (`[C]`).

## PHASE 3: TCP DECOYS AND LIMITS (Steps 31-46)

**Goal:** `banner`, `http` and `silent` decoys that count on accept and hold to every limit.
**Prerequisite:** P2 · **Time:** 90 min · **Agent:** Agent [P] (lane A)

- [ ] **Step 31** [W] ~10m: `cmd/honeypot/decoy_test.go`, using `net.Listen("tcp","127.0.0.1:0")` listeners, cases:
  - A connect with no bytes counts 1.
  - The banner is sent exactly once.
  - The read stops at `read_bytes`.
  - The read deadline closes the connection.
  - The `max_conns + 1`th connection is refused, closed at once, and counted in `refused_total{reason="max_conns"}`.
  - The `max_conns_per_source + 1`th from one address is refused.
  - `http` answers one 401 and closes.
  - Log bytes are hex-escaped and cut at `log_bytes`.
- [ ] **Step 32** [B]: `go test ./cmd/honeypot -run Decoy -count=1 2>&1 | tail -5` → RED [V].
- [ ] **Step 33** [W] ~20m: `cmd/honeypot/decoy.go`. It has one accept loop per decoy and a semaphore for `max_conns`. The per-source count lives in a map under a mutex, keyed by `netip.Addr`, and its entry is deleted on close. `SetDeadline` covers the whole connection. All reads go through `io.LimitReader`. The counter is incremented before anything is read.
- [ ] **Step 34** [W] ~5m: `cmd/honeypot/sanitize.go`: `escape(b []byte, max int) string`. It keeps printable ASCII and writes `\xHH` for everything else. The same function bounds usernames and client versions (≤ 64 bytes).
- [ ] **Step 35** [B]: `go test ./cmd/honeypot -count=1 -race 2>&1 | tail -5`
- [ ] **Step 36** [V]: GREEN, race-free.
  - [D] Step 36a: a goroutine leak in the deadline case. Use `t.Cleanup` with a `runtime.NumGoroutine` settle loop and check every return path closes the connection.
- [ ] **Step 37** [C]: commit `cmd/honeypot/decoy.go cmd/honeypot/decoy_test.go cmd/honeypot/sanitize.go`.
- [ ] **Step 38** [W] ~5m: `FuzzEscape`: the output is always printable ASCII and never longer than `4*max`.
- [ ] **Step 39** [B]: `go test ./cmd/honeypot -run xxx -fuzz FuzzEscape -fuzztime 60s 2>&1 | tail -3` → [V] clean.
- [ ] **Step 40** [W] ~10m: a port already held at start is reported with its decoy name, the other decoys still start, and `unheaded_honeypot_decoys` omits it. Test: hold a port in the test and expect the named error.
- [ ] **Step 41** [B]: `go test ./cmd/honeypot -run Held -count=1` → [V] GREEN.
- [ ] **Step 42** [C]: commit.
- [ ] **Step 43** [B] ~2m: a local smoke run.
  ```bash
  go build -o /tmp/claude-1000/honeypot ./cmd/honeypot && printf 'listen: ["127.0.0.1"]\nmetrics_listen: "127.0.0.1:19150"\ninternal_port_base: 42000\ndecoys:\n  - {name: t, kind: banner, port: 12323, banner: "login: "}\n' > /tmp/claude-1000/hp.yaml && (/tmp/claude-1000/honeypot serve -config /tmp/claude-1000/hp.yaml & echo $! > /tmp/claude-1000/hp.pid); sleep 1; printf 'root\r\n' | timeout 3 nc 127.0.0.1 12323; kill $(cat /tmp/claude-1000/hp.pid)
  ```
- [ ] **Step 44** [V]: `login: ` is printed and the journal or stderr shows one line with `decoy=t` and the escaped `root\x0d\x0a`.
- [ ] **Step 45** [D]: Nothing printed → check the listen address and that `serve` did not exit on `check` errors.
- [ ] **Step 46** [V]: **PHASE 3 EXIT GATE**: all decoy tests pass with `-race` and the smoke run behaves. Commit (`[C]`).

## PHASE 4: SSH DECOY (Steps 47-58)

**Goal:** a real SSH handshake that records the attempt and never authenticates.
**Prerequisite:** P3 · **Time:** 75 min · **Agent:** Agent [P] (lane A)

- [ ] **Step 47** [W] ~5m: `cmd/honeypot/keys.go`. On first start it generates an Ed25519 host key and a 32-byte HMAC key in `state_dir` (0600, `O_EXCL`, refusing a symlink), and later starts load them. Tests: created once, then reused; mode 0600; a symlink in place is refused.
- [ ] **Step 48** [W] ~10m: `cmd/honeypot/ssh_test.go`, using an `x/crypto/ssh` client against the decoy:
  - The client version is recorded and escaped.
  - Password auth fails and counts `auth_attempts_total{decoy="ssh",method="password"}`.
  - The HMAC is stable for the same password and different for another.
  - The password itself never appears in the log buffer: assert `!strings.Contains(log, pw)`.
  - Publickey auth fails and counts `method="publickey"`.
  - A client that stalls before its version string is closed at the deadline.
  - After 6 attempts the server disconnects.
- [ ] **Step 49** [B]: `go test ./cmd/honeypot -run SSH -count=1 2>&1 | tail -5` → RED [V].
- [ ] **Step 50** [W] ~20m: `cmd/honeypot/ssh.go`: `ssh.ServerConfig` with `ServerVersion` from the config, plus `PasswordCallback`, `PublicKeyCallback` and `KeyboardInteractiveCallback`. Each callback records and then returns an error. `MaxAuthTries: 6`. No channel is ever accepted. The handshake runs under the connection deadline.
- [ ] **Step 51** [B]: `go test ./cmd/honeypot -count=1 -race 2>&1 | tail -5`
- [ ] **Step 52** [V]: GREEN.
  - [D] Step 52a: if the version string is refused, `ServerVersion` must start with `SSH-2.0-`. Validate that in `config.go` and test it in Step 17's file.
- [ ] **Step 53** [C]: commit `cmd/honeypot/keys.go cmd/honeypot/ssh.go cmd/honeypot/ssh_test.go`.
- [ ] **Step 54** [W] ~5m: `FuzzSSHPreamble`: random bytes as the client's first flight, through `net.Pipe`. The connection must end within the deadline, without a panic or a goroutine leak.
- [ ] **Step 55** [B]: `go test ./cmd/honeypot -run xxx -fuzz FuzzSSHPreamble -fuzztime 120s 2>&1 | tail -3` → [V] clean.
- [ ] **Step 56** [B] ~2m: an OpenSSH client against a local run (same as Step 43 with a `kind: ssh` decoy on 12222).
  ```bash
  ssh -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null -o PreferredAuthentications=password -o NumberOfPasswordPrompts=1 -p 12222 probe@127.0.0.1 true </dev/null; echo "exit $?"
  ```
- [ ] **Step 57** [V]: The client fails authentication (exit 255) and the log line shows `user=probe` with `client=SSH-2.0-OpenSSH_...`.
- [ ] **Step 58** [V]: **PHASE 4 EXIT GATE**: SSH tests and fuzz are clean, the OpenSSH client is refused, and no password appears in any log. Commit (`[C]`).

## PHASE 5: METRICS, JOURNAL, RENDER-NFT (Steps 59-68)

**Goal:** the honeypot is observable, and its nft table is rendered from the same YAML.
**Prerequisite:** P4 · **Time:** 60 min · **Agent:** Agent [P] (lane A)

- [ ] **Step 59** [W] ~10m: register `HoneypotMetrics = 19150` in a new `// Tripwire (19150-19159)` block of `pkg/ports/ports.go` (19130 is alert-mailer's) and use it as the `metrics_listen` default. Then write `cmd/honeypot/metrics.go`: the five metric families named in the ADR. They are served on `metrics_listen` only, with `/metrics` alone and every other path returning 404. Use the metrics helper the other commands use (`pkg/metrics`, see `cmd/alert-mailer/critical.go` imports) and add nothing new.
- [ ] **Step 60** [W] ~5m: the metrics `http.Server` sets `ReadHeaderTimeout` (`scripts/check-http-timeouts.sh` fails without it). `metrics_test.go`: after one connection the scrape shows `unheaded_honeypot_connections_total{decoy="t"} 1`; no label value contains `.` or `:` (no addresses); `/debug` returns 404.
- [ ] **Step 61** [B]: `go test ./cmd/honeypot -count=1 -race 2>&1 | tail -3` → [V] GREEN.
- [ ] **Step 62** [W] ~10m: `cmd/honeypot/nft.go`, which renders `table inet unheaded_honeypot` with these pieces:
  - A `prerouting` nat chain: `tcp dport <p> redirect to :<base+p>` for each enabled decoy below 1024, only on `listen` addresses (`ip daddr`/`ip6 daddr`).
  - A per-source meter at `new_conns_per_second` that drops over-rate SYNs to decoy ports.
  - An `output` filter chain: `meta skuid "unheaded-honeypot" ct state new counter drop`.
  - The rendered text starts with `table inet unheaded_honeypot` and then `delete table inet unheaded_honeypot`, as `deploy/nftables/east.nft` does.
- [ ] **Step 63** [W] ~5m: a golden test `testdata/honeypot.nft.golden` from the shipped YAML, plus a test that a disabled decoy leaves no rule.
- [ ] **Step 64** [B]: `go test ./cmd/honeypot -run Nft -count=1 && go run ./cmd/honeypot render-nft -config deploy/honeypot/honeypot.yaml > /tmp/claude-1000/hp.nft && sudo -n nft -c -f /tmp/claude-1000/hp.nft && echo nft-ok`
- [ ] **Step 65** [V]: `nft-ok` (west's nft checks the syntax; the account name only has to exist at load time).
  - [D] Step 65a: if `-c` fails on `skuid` for an unknown user on west, check with `meta skuid 0` swapped in for the syntax check alone, and keep the name in the real render.
- [ ] **Step 66** [C]: commit the metrics and nft files with their tests and golden.
- [ ] **Step 67** [B]: `go test ./cmd/honeypot -count=1 -race -cover 2>&1 | tail -2`
- [ ] **Step 68** [V]: **PHASE 5 EXIT GATE**: coverage ≥ 85% on `cmd/honeypot`, all tests green, render checked by `nft -c`. Commit (`[C]`).

## PHASE 6: WATCH CONFIG AND RENDERER (Steps 69-80)

**Goal:** `watch.yaml` becomes a counting-only `inet unheaded_watch` table with a class per flow and timed window sets.
**Prerequisite:** P1 · **Time:** 90 min · **Agent:** Agent [P] (lane B)

- [ ] **Step 69** [W] ~10m: `cmd/unheaded-watch/config_test.go`. It covers:
  - The defaults load.
  - Unknown keys are refused, and so are duplicate profile or window names.
  - Bad CIDRs and ports outside 1-65535 are refused, and so is a proto other than tcp or udp.
  - `default` > `max`, or `max` > 24 h, are refused.
  - `mode` other than learn or alert is refused.
  - `log_rate` must match `^[0-9]+/(second|minute|hour)$`.
  - `every` must be within 2 s to 5 min.
  - An unknown `open_on` trigger is refused.
- [ ] **Step 70** [W] ~10m: `cmd/unheaded-watch/config.go` (strict `yaml.v3`, the same 64 KiB cap). Step 69's tests → GREEN [V].
- [ ] **Step 71** [W] ~20m: `cmd/unheaded-watch/render.go`. It renders input and output chains at `priority filter + 10; policy accept;`. Rules carry only `counter name <class_dir>`, `log` and `return`, and never `drop`, `reject` or `accept` (accept would skip later chains). Rule order: `ct state established,related` → counter `established` + return; peers; profiles; window sets; decoy ports (read from the honeypot YAML if given with `-honeypot`); `probe_in`; `unexpected_*` with `log prefix "unheaded-watch: <class> " limit rate <log_rate>`.
- [ ] **Step 72** [W] ~5m: `render_test.go`. A golden from the shipped files, and a test that walks the rendered text and asserts the tokens `drop`, `reject` and `accept` appear nowhere (T11).
- [ ] **Step 73** [B]: `go test ./cmd/unheaded-watch -count=1 -race 2>&1 | tail -3` → [V] GREEN.
- [ ] **Step 74** [B] ~1m: kernel acceptance of the window sets, on east (7.0).
  ```bash
  go run ./cmd/unheaded-watch render -config deploy/watch/watch.yaml -honeypot deploy/honeypot/honeypot.yaml > /tmp/claude-1000/watch.nft && scp -q /tmp/claude-1000/watch.nft east:/tmp/watch.nft && ssh east 'sudo -n nft -c -f /tmp/watch.nft && echo nft-ok'
  ```
- [ ] **Step 75** [D]: If `interval, timeout` on a concatenated set is refused: render one set per window with `flags timeout` keyed `inet_proto . inet_service`, with an address match per `to:` in the rule. Update the golden, rerun Step 74, and record the fallback in this ADR's "Not established" item 1.
- [ ] **Step 76** [V]: `nft-ok` on east.
- [ ] **Step 77** [C]: commit the watch config, renderer, tests and golden.
- [ ] **Step 78** [W] ~5m: `unheaded-watch check` prints peers, profiles and windows (default and max) and exits 2 on the first problem.
- [ ] **Step 79** [B]: `go run ./cmd/unheaded-watch check -config deploy/watch/watch.yaml` → [V] 3 peers, 2 profiles, 2 windows.
- [ ] **Step 80** [V]: **PHASE 6 EXIT GATE**: tests green, the golden contains no drop, reject or accept, and east's nft accepts the render. Commit (`[C]`).

## PHASE 7: SAMPLER, WINDOWS, APT HOOK (Steps 81-92)

**Goal:** counters become Huginn textfile metrics, and windows open and close on demand or by themselves for apt.
**Prerequisite:** P6 · **Time:** 75 min · **Agent:** Agent [P] (lane B)

- [ ] **Step 81** [W] ~10m: `cmd/unheaded-watch/sample.go`. It runs `/usr/sbin/nft -j list counters table inet unheaded_watch` and `nft -j list sets table inet unheaded_watch` with a 5 s timeout and parses the JSON strictly (sizes capped at 4 MiB). It writes the six metric families atomically (temp file, `fsync`, `rename`) to `textfile`. If the table is missing it writes `unheaded_watch_table_present 0` and keeps running.
- [ ] **Step 82** [W] ~5m: `sample_test.go`, using recorded nft JSON fixtures in `testdata/` (taken from Step 74's table loaded on east in Step 109; until then hand-written from the nft JSON schema). Cases: counters map to `{class,direction}`; a missing table gives 0; malformed JSON gives `unheaded_watch_sample_ok 0` and no partial file.
- [ ] **Step 83** [W] ~5m: `FuzzParseNftJSON` → `go test ./cmd/unheaded-watch -run xxx -fuzz FuzzParseNftJSON -fuzztime 90s` → [V] clean.
- [ ] **Step 84** [C]: commit.
- [ ] **Step 85** [W] ~10m: `open.go`. `open <window> [dur]` adds the window's elements with `timeout min(dur, max)` (default `default`), using `nft add element` built from the rendered element list and never from a shell string. `close` flushes the window's sets. `status` lists open windows with the time left. Tests: duration capping; an unknown window exits 2; the built argv contains only expected tokens.
- [ ] **Step 86** [B]: `go test ./cmd/unheaded-watch -count=1 -race` → [V] GREEN.
- [ ] **Step 87** [W] ~5m: `deploy/apt/99unheaded-watch`:
  ```
  APT::Update::Pre-Invoke { "/usr/local/sbin/unheaded-watch open apt || true"; };
  DPkg::Pre-Invoke { "/usr/local/sbin/unheaded-watch open apt || true"; };
  ```
  (`|| true`: the watch must never break apt.)
- [ ] **Step 88** [C]: commit `cmd/unheaded-watch/open.go`, its test and `deploy/apt/99unheaded-watch`.
- [ ] **Step 89** [R] ~5m: on east, check that unattended-upgrades and `apt-daily.service` run the hooks.
  ```bash
  ssh east 'systemctl cat apt-daily.service | grep ExecStart; grep -rn "Pre-Invoke" /etc/apt/apt.conf.d/ | head'
  ```
- [ ] **Step 90** [V]: `apt-daily` runs `apt.systemd.daily`, which calls `apt-get update` (the hook runs). If unattended-upgrades bypasses `DPkg::Pre-Invoke`, add `ExecStartPre=-/usr/local/sbin/unheaded-watch open apt` drop-ins for `apt-daily.service` and `apt-daily-upgrade.service` (`deploy/systemd/dropins/`) and note it in "Not established" item 4.
- [ ] **Step 91** [B]: `go test ./cmd/unheaded-watch -count=1 -race -cover | tail -2`
- [ ] **Step 92** [V]: **PHASE 7 EXIT GATE**: coverage ≥ 85% on `cmd/unheaded-watch`, the fuzz run is clean, and the hook file is committed. Commit (`[C]`).

## PHASE 8: UNITS, ACCOUNTS, SANDBOX (Steps 93-99)

**Goal:** least-privilege units that `systemd-analyze security` rates well.
**Prerequisite:** P5, P7 · **Time:** 45 min · **Agent:** Coordinator

- [ ] **Step 93** [W] ~5m: `deploy/sysusers.d/unheaded.conf`: add `unheaded-honeypot` and `unheaded-watch` (nologin, no fixed uid unless `pkg/uids` needs one; they are host units, not containers). `unheaded-watch` also gets `m unheaded-watch unheaded-huginn` only if the textfile directory's group requires it; check `huginn.service`'s `textfile_dir` ownership first.
- [ ] **Step 94** [W] ~10m: `deploy/systemd/unheaded-honeypot.service`. Copy huginn's sandbox block, then set:
  - `User=unheaded-honeypot`
  - `CapabilityBoundingSet=` and `AmbientCapabilities=` (both empty)
  - `StateDirectory=unheaded/honeypot` with `StateDirectoryMode=0700`
  - `ReadOnlyPaths=/etc/unheaded/honeypot.yaml`
  - `RestrictAddressFamilies=AF_INET AF_INET6 AF_UNIX`
  - `SystemCallFilter=@system-service` and `SystemCallFilter=~@privileged @resources`
  - `MemoryMax=128M`, `Environment=GOMEMLIMIT=96MiB` (`scripts/check-unit-ceilings.sh` wants both on every Go unit), `TasksMax=64`
  - `Restart=no` (ADR-147 decision 1)
  - `Slice=unheaded-host.slice`
  - `After=` and `Requires=` on `unheaded-nft-table@honeypot.service`
- [ ] **Step 95** [W] ~10m: `deploy/systemd/unheaded-watch-sample.service` (`User=unheaded-watch`, `AmbientCapabilities=CAP_NET_ADMIN`, `CapabilityBoundingSet=CAP_NET_ADMIN`, `RestrictAddressFamilies=AF_NETLINK AF_UNIX`, `IPAddressDeny=any`, `ReadWritePaths=` the textfile directory only, `MemoryMax=64M`, `Environment=GOMEMLIMIT=48MiB`). Also `deploy/systemd/unheaded-nft-table@.service`, copied from `unheaded-nft.service` with `StandardInput=file:/var/lib/unheaded/nft/%i.nft`, `ExecStop=-/usr/sbin/nft delete table inet unheaded_%i` and `ConditionPathExists=` on that file.
- [ ] **Step 96** [B] ~1m: `for u in deploy/systemd/unheaded-honeypot.service deploy/systemd/unheaded-watch-sample.service deploy/systemd/unheaded-nft-table@.service; do systemd-analyze verify "$u" 2>&1 | grep -v "not found" ; done`
- [ ] **Step 97** [V]: No errors other than missing binaries on west, and `scripts/check-unit-ceilings.sh` passes.
- [ ] **Step 98** [C]: commit the sysusers line and the three unit files.
- [ ] **Step 99** [V]: **PHASE 8 EXIT GATE**: units verify, every new process has its own account, and only the watch sampler and the loader hold `CAP_NET_ADMIN`, with no network socket. Commit (`[C]`).

## PHASE 9: ALERTS, POLICY, SCRAPES (Steps 100-108)

**Goal:** the six alerts exist, are unit-tested, the four pages mail at once, and west scrapes east's new sources.
**Prerequisite:** P8 · **Time:** 60 min · **Agent:** Coordinator

- [ ] **Step 100** [R] ~5m: `monitoring/alertmanager/` route and grouping; `cmd/alert-policy` (how `promote:` and `mailer.immediate` are validated). Record in "Not established" item 2 whether pages group into one mail per burst.
- [ ] **Step 101** [W] ~10m: `monitoring/prometheus/rules/tripwire.yml`: the six alerts from the ADR's table, each with `host` and `decoy` or `class` in labels, summary text with no attacker data, and `runbook` → `wiki/Tripwire.md`.
- [ ] **Step 102** [W] ~10m: `monitoring/prometheus/rules/tests/tripwire_test.yml`. It covers:
  - One connection fires `HoneypotTouched` within 2m and the alert resolves afterwards.
  - `unexpected_out` in `mode="learn"` does **not** fire `EastTrafficUnexpected`.
  - The same in `mode="alert"` fires.
  - `window` and `profile` classes never fire.
  - Table present 0 fires `TrafficWatchBlind` after 1m.
- [ ] **Step 103** [B]: `promtool test rules monitoring/prometheus/rules/tests/tripwire_test.yml` → [V] SUCCESS.
- [ ] **Step 104** [W] ~5m: `observability/alerting/policy.yaml`: add `promote:` entries (four `urgency: page`, two `ticket`), each with a `reason:` that cites this ADR. Then set `mailer.immediate: [HoneypotTouched, HoneypotCredentialTried, EastTrafficUnexpected, TrafficWatchBlind]` and replace the "None exists yet" comment with a pointer to ADR-153.
- [ ] **Step 105** [W] ~5m: scrape jobs.
  - `monitoring/prometheus/prometheus.yml`: job `honeypot` → `['[fd00:dead:beef::2]:19150']` (`host: east`); add the target `['[fd00:dead:beef::2]:9110']` (`host: east`) to job `huginn`.
  - The same two in `config/victoria/scrape.yml` (rule: a job in one needs the other).
  - Then run `python3 scripts/gen-prom-keep-list.py`.
- [ ] **Step 106** [B] ~2m: `go run ./cmd/alert-policy && scripts/check-prometheus-rules.sh 2>&1 | tail -5` (or the CI recipe's container if promtool is absent).
- [ ] **Step 107** [V]: The policy validates (each immediate name is a promoted page) and the rule check passes with the regenerated keep list.
  - [D] Step 107a: if the keep-list check fails, rerun the generator and `git diff monitoring/prometheus/rule-series.txt`.
- [ ] **Step 108** [V]: **PHASE 9 EXIT GATE**: promtool green, policy valid, scrape jobs in both files. Commit (`[C]`): the rules, test, policy, both scrape files and `rule-series.txt`.

## PHASE 10: INSTALL ON EAST, LEARN MODE (Steps 109-118)

**Goal:** both tables loaded, both services running on east, west scraping, the watch in `learn`.
**Prerequisite:** P9; H7 answered · **Time:** 45 min · **Agent:** Coordinator [S]

- [ ] **Step 109** [S][B] ~5m: build and install (H7 (b)).
  ```bash
  cd /home/govan/tmp/unheaded && CGO_ENABLED=0 go build -trimpath -ldflags "-s -w" -o /tmp/claude-1000/honeypot ./cmd/honeypot && CGO_ENABLED=0 go build -trimpath -ldflags "-s -w" -o /tmp/claude-1000/unheaded-watch ./cmd/unheaded-watch && scp -q /tmp/claude-1000/honeypot /tmp/claude-1000/unheaded-watch deploy/honeypot/honeypot.yaml deploy/watch/watch.yaml deploy/apt/99unheaded-watch deploy/systemd/unheaded-honeypot.service deploy/systemd/unheaded-watch-sample.service deploy/systemd/unheaded-nft-table@.service deploy/sysusers.d/unheaded.conf east:/tmp/tripwire/ 2>/dev/null || { ssh east mkdir -p /tmp/tripwire && scp -q /tmp/claude-1000/honeypot /tmp/claude-1000/unheaded-watch deploy/honeypot/honeypot.yaml deploy/watch/watch.yaml deploy/apt/99unheaded-watch deploy/systemd/unheaded-honeypot.service deploy/systemd/unheaded-watch-sample.service deploy/systemd/unheaded-nft-table@.service deploy/sysusers.d/unheaded.conf east:/tmp/tripwire/; }
  ssh east 'cd /tmp/tripwire && sudo -n install -m 755 honeypot /usr/local/sbin/honeypot && sudo -n install -m 755 unheaded-watch /usr/local/sbin/unheaded-watch && sudo -n install -d -m 755 /etc/unheaded && sudo -n install -m 644 honeypot.yaml watch.yaml /etc/unheaded/ && sudo -n install -m 644 unheaded.conf /etc/sysusers.d/unheaded.conf && sudo -n systemd-sysusers && sudo -n install -m 644 unheaded-honeypot.service unheaded-watch-sample.service unheaded-nft-table@.service /etc/systemd/system/ && sudo -n systemctl daemon-reload && id unheaded-honeypot && id unheaded-watch'
  ```
- [ ] **Step 110** [V]: Both accounts exist. Check Huginn's `textfile_dir` on east: `ssh east 'sudo -n grep -n textfile_dir /etc/huginn.yaml /etc/unheaded/huginn.yaml 2>/dev/null'`.
  - [D] Step 110a: if it is unset, set `textfile_dir: /var/lib/unheaded/textfile` the way the west host file does (`cmd/huginn/huginn.yaml.example:36-38`), create the directory (root 0755, `unheaded-watch` writes one file), restart huginn, and record it.
- [ ] **Step 111** [S][B] ~2m: render and load both tables.
  ```bash
  ssh east 'sudo -n install -d -m 700 /var/lib/unheaded/nft && sudo -n sh -c "/usr/local/sbin/honeypot render-nft -config /etc/unheaded/honeypot.yaml > /var/lib/unheaded/nft/honeypot.nft && /usr/local/sbin/unheaded-watch render -config /etc/unheaded/watch.yaml -honeypot /etc/unheaded/honeypot.yaml > /var/lib/unheaded/nft/watch.nft" && sudo -n chmod 600 /var/lib/unheaded/nft/*.nft && sudo -n systemctl enable --now unheaded-nft-table@honeypot unheaded-nft-table@watch && sudo -n nft list tables'
  ```
- [ ] **Step 112** [V]: `inet unheaded_honeypot` and `inet unheaded_watch` are listed. `ssh east true` still works (T11: nothing drops).
  - [D] Step 112a: ssh lost → from the console: `sudo nft delete table inet unheaded_watch; sudo nft delete table inet unheaded_honeypot` (Appendix A1).
- [ ] **Step 113** [C]: commit any fixes found in Steps 109-112 (deploy files only).
- [ ] **Step 114** [S][B] ~2m: start the services and install the apt hook.
  ```bash
  ssh east 'sudo -n install -m 644 /tmp/tripwire/99unheaded-watch /etc/apt/apt.conf.d/99unheaded-watch && sudo -n systemctl enable --now unheaded-honeypot unheaded-watch-sample && sleep 15 && systemctl is-active unheaded-honeypot unheaded-watch-sample && sudo -n ss -ltnpH | grep -E ":(2222|8080|3389|5432|6379|19150|4202[3]|42080|42445) " | wc -l && cat /var/lib/unheaded/textfile/watch.prom | head -20'
  ```
- [ ] **Step 115** [V]: Both services are active; 9 listeners (8 decoys + metrics); `watch.prom` has `unheaded_watch_table_present 1` and `unheaded_watch_mode{mode="learn"} 1`.
- [ ] **Step 116** [B] ~2m: reload Prometheus and VictoriaMetrics on west (their documented reload routes), then:
  ```bash
  curl -s 'http://localhost:9090/api/v1/query?query=up{job="honeypot"}' | head -c 300; echo; curl -s 'http://localhost:9090/api/v1/query?query=unheaded_watch_table_present' | head -c 300
  ```
- [ ] **Step 117** [V]: `up{job="honeypot"} 1` and `unheaded_watch_table_present{host="east"} 1`.
  - [D] Step 117a: scrape down → `nc -vz fd00:dead:beef::2 19150` from west. If it is closed, check `metrics_listen` and that the `east.nft` default deny is not applied (it is not, per the ADR).
- [ ] **Step 118** [V]: **PHASE 10 EXIT GATE**: tables loaded, services running, both scraped from west, no alert fired (learn mode, no touches yet). Commit (`[C]`). Write the start time of learn into this ADR (H4).

## PHASE 11: LEARN, THEN ALERT (Steps 119-124)

**Goal:** 48 h of real traffic classified; the profiles fit the truth; `mode: alert`.
**Prerequisite:** P10 + 48 h (H4) · **Time:** 30 min of work after the wait · **Agent:** Coordinator

- [ ] **Step 119** [B] ~5m: the classes over the period, from VictoriaMetrics.
  ```bash
  curl -s 'http://localhost:8428/api/v1/query' --data-urlencode 'query=sum by (class,direction) (increase(unheaded_watch_packets_total{host="east"}[48h]))' | python3 -m json.tool | head -60
  ```
- [ ] **Step 120** [B] ~5m: what the unexpected classes were.
  ```bash
  ssh east 'sudo -n journalctl -k --since "-48h" -g "unheaded-watch: " --no-pager | sed -E "s/.*(unheaded-watch: [a-z_]+).*(SRC=[^ ]+).*(DST=[^ ]+).*(PROTO=[A-Z0-9]+).*(DPT=[0-9]+)?.*/\1 \2 \3 \4 \5/" | sort | uniq -c | sort -rn | head -30'
  ```
- [ ] **Step 121** [V]: Each unexpected line is explained. Normal traffic that is missing from the profiles (for example IPv6 ND, or a resolver not in the list) becomes a profile edit in `deploy/watch/watch.yaml` and `/etc/unheaded/watch.yaml`. Anything unexplained is a finding: STOP and show Stevie.
- [ ] **Step 122** [S][B] ~2m: after Stevie's look (H4), switch to alert mode.
  ```bash
  ssh east 'sudo -n sed -i "s/^mode: learn/mode: alert/" /etc/unheaded/watch.yaml && sudo -n sh -c "/usr/local/sbin/unheaded-watch render -config /etc/unheaded/watch.yaml -honeypot /etc/unheaded/honeypot.yaml > /var/lib/unheaded/nft/watch.nft" && sudo -n systemctl restart unheaded-nft-table@watch unheaded-watch-sample && sleep 12 && grep mode /var/lib/unheaded/textfile/watch.prom'
  ```
- [ ] **Step 123** [C]: commit the profile edits (`deploy/watch/watch.yaml`) with the 48 h class table pasted into this ADR as a dated update.
- [ ] **Step 124** [V]: **PHASE 11 EXIT GATE**: `unheaded_watch_mode{mode="alert"} 1`, and no `EastTrafficUnexpected` firing for 30 minutes after the switch.

## PHASE 12: RED-TEAM DRILLS (BlackMage) (Steps 125-134)

**Goal:** every threat row with a detection is proven by doing it once.
**Prerequisite:** P11; H8 answered; Stevie told that mail is coming · **Time:** 60 min · **Agent:** Coordinator (attended or announced)

- [ ] **Step 125** [B] ~2m: T1, a scan from west.
  ```bash
  for p in 23 80 445 2222 3389 5432 6379 8080; do timeout 2 bash -c "</dev/tcp/192.168.13.1/$p" 2>/dev/null; done; echo done
  ```
- [ ] **Step 126** [V]: Within 3 minutes `HoneypotTouched` fires for 8 decoys, **one mail** arrives (grouped) or at most `max_per_hour`, and the journal on east shows 8 lines with `SRC=192.168.13.2`.
  - [D] Step 126a: no mail → check `curl -s localhost:9093/api/v2/alerts | head`, then the alert-mailer outbox and its log (`docker logs` of the monitoring project's alert-mailer, last 50 lines).
- [ ] **Step 127** [B] ~1m: T1 and T7, SSH spraying with a marker password.
  ```bash
  sshpass -p 'tripwire-drill-7f3a' ssh -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null -o PreferredAuthentications=password -p 2222 admin@192.168.13.1 true; echo "exit $?"
  ```
  (No `sshpass` → run the same `ssh` command and type the password at the prompt.)
- [ ] **Step 128** [V]: `HoneypotCredentialTried` fires; `ssh east 'sudo -n journalctl -u unheaded-honeypot --since -5m | grep -c tripwire-drill-7f3a'` prints **0** (the password is never logged); the line shows `user=admin` and a `pw_hmac=`.
- [ ] **Step 129** [B] ~2m: T3, a flood.
  ```bash
  for i in $(seq 1 300); do (timeout 15 bash -c "exec 3<>/dev/tcp/192.168.13.1/5432; sleep 14" &) ; done; sleep 5; ssh east 'systemctl show unheaded-honeypot -p MemoryCurrent -p TasksCurrent; sudo -n nft list table inet unheaded_honeypot | grep -A2 meter | head'
  ```
- [ ] **Step 130** [V]: East stays responsive (`ssh east true` < 2 s); memory < 128M; `unheaded_honeypot_refused_total` grows or the meter drops the excess; the service stays active.
- [ ] **Step 131** [B] ~2m: T10, egress outside a window, then the apt window.
  ```bash
  ssh east 'curl -s -m 5 -o /dev/null https://example.com; echo curl=$?; sudo -n apt-get update -qq; echo apt=$?; /usr/local/sbin/unheaded-watch status'
  ```
- [ ] **Step 132** [V]: The `curl` (no window open) fires `EastTrafficUnexpected` (class `unexpected_out`). The `apt-get update` opened `apt` (status shows it with time left), and **no** new unexpected count followed from it.
- [ ] **Step 133** [S][B] ~2m: T8, blinding.
  ```bash
  ssh east 'sudo -n nft delete table inet unheaded_watch; sleep 90; sudo -n systemctl restart unheaded-nft-table@watch; sudo -n nft list tables | grep -c unheaded_watch'
  ```
- [ ] **Step 134** [V]: **PHASE 12 EXIT GATE**: `TrafficWatchBlind` fired and mailed, then resolved after the reload, and the table is back (count 1). The drill table (Steps 125-133: fired? mailed? seconds to mail) is written into this ADR as a dated update. Commit (`[C]`).

## PHASE 13: DOCUMENTS AND HAND-OFF (Steps 135-138)

**Goal:** an adopter can install, tune and switch it off from the wiki alone.
**Prerequisite:** P12 · **Time:** 40 min · **Agent:** Coordinator (Librarian's rules)

- [ ] **Step 135** [W] ~20m: `wiki/Tripwire.md`. It covers:
  - What each piece does, and both YAML files with every key.
  - How to add a decoy, move a port, add a profile or a window, and open or close a window by hand.
  - Learn and alert modes, the six alerts, and what each one asks of the reader.
  - How to switch everything off (Appendix A2).
  - Also add rows in `wiki/Systemd-Units.md` (east) and `wiki/_Sidebar.md`.
- [ ] **Step 136** [W] ~5m: `docs/adr/ADR-INDEX.md` row for 153 (status per the outcome). This ADR's Status becomes Accepted with the built commits. Update the `~/tmp/next.md` top section.
- [ ] **Step 137** [B] ~5m: the full local CI.
  ```bash
  cd /home/govan/tmp/unheaded && scripts/ci-local.sh 2>&1 | tail -15
  ```
- [ ] **Step 138** [V]: **PHASE 13 EXIT GATE**: CI passed with 0 failures (restore the `sbom/` files the SBOM step rewrites, and do not commit them). The final commit is on `sprint/s-east-tripwire`. Merging to develop is Stevie's word; pushing is his.

---

## Appendix A: Emergency procedures

**A1. Lost SSH to east after a table load.** The watch never drops (Step 72's test), so first suspect the honeypot's nat table. At east's console:
1. `sudo nft delete table inet unheaded_honeypot`
2. `sudo nft delete table inet unheaded_watch`
3. `sudo systemctl disable unheaded-nft-table@honeypot unheaded-nft-table@watch`
4. Reconnect, then diff the rendered files against the goldens.

**A2. Switch everything off.**
```bash
ssh east 'sudo -n systemctl disable --now unheaded-honeypot unheaded-watch-sample unheaded-nft-table@honeypot unheaded-nft-table@watch && sudo -n rm -f /etc/apt/apt.conf.d/99unheaded-watch'
```
The ExecStop lines delete both tables. Then snooze the six alerts in `observability/alerting/policy.yaml` (`snooze_until`, `snooze_reason`) so `HoneypotDown` stays quiet.

**A3. Mail flood.** The caps hold the day (`max_per_day: 6`). To stop pages for a known cause, snooze `HoneypotTouched` or `EastTrafficUnexpected` with a reason (policy file header). Never remove them from `mailer.immediate` without Stevie's word.

**A4. apt fails.** The hook is `|| true`, so the watch cannot fail apt. If apt fails anyway, `sudo rm /etc/apt/apt.conf.d/99unheaded-watch` and rerun to prove it is unrelated.

**A5. A false positive from a legitimate new flow.** Add a profile or window in `/etc/unheaded/watch.yaml` and run `unheaded-watch check`. Re-render to `/var/lib/unheaded/nft/watch.nft`, then `systemctl restart unheaded-nft-table@watch`. Mirror the change in `deploy/watch/watch.yaml` if it is general.

## Appendix B: Agent assignment

| Phase | Agent | Parallel | Depends on | Time |
|---|---|---|---|---|
| 0 Preflight | Coordinator | no | none | 15 m |
| 1 Skeleton | Coordinator | no | 0 | 30 m |
| 2 Honeypot config | Lane A | with 6, 7 | 1 | 60 m |
| 3 TCP decoys | Lane A | with 6, 7 | 2 | 90 m |
| 4 SSH decoy | Lane A | with 6, 7 | 3 | 75 m |
| 5 Metrics, nft | Lane A | with 6, 7 | 4 | 60 m |
| 6 Watch renderer | Lane B | with 2-5 | 1 | 90 m |
| 7 Sampler, windows | Lane B | with 2-5 | 6 | 75 m |
| 8 Units | Coordinator | no | 5, 7 | 45 m |
| 9 Alerts | Coordinator | no | 8 | 60 m |
| 10 Install east | Coordinator [S] | no | 9 | 45 m |
| 11 Learn → alert | Coordinator | no | 10 + 48 h | 30 m |
| 12 Drills | Coordinator, attended | no | 11 | 60 m |
| 13 Docs | Coordinator | no | 12 | 40 m |

Critical path: 0 → 1 → 2 → 3 → 4 → 5 → 8 → 9 → 10 → 11 → 12 → 13, about 10.5 h of work plus 48 h of waiting.

## Appendix C: Quick reference

| Thing | Value |
|---|---|
| Configs | `/etc/unheaded/honeypot.yaml`, `/etc/unheaded/watch.yaml` (repo defaults in `deploy/honeypot/`, `deploy/watch/`) |
| Binaries | `/usr/local/sbin/honeypot`, `/usr/local/sbin/unheaded-watch` |
| Tables | `inet unheaded_honeypot` (redirects, meter, own-egress drop), `inet unheaded_watch` (count and log only) |
| Rendered tables | `/var/lib/unheaded/nft/{honeypot,watch}.nft` (root 0600), loaded by `unheaded-nft-table@{honeypot,watch}` |
| Accounts | `unheaded-honeypot` (no caps), `unheaded-watch` (`CAP_NET_ADMIN`, no network), `unheaded-nft` (loader) |
| Metrics | honeypot `[fd00:dead:beef::2]:19150/metrics`; watch via Huginn textfile `watch.prom` on `:9110` |
| Windows | `unheaded-watch open apt 30m` · `close apt` · `status` |
| Logs | `journalctl -u unheaded-honeypot`; `journalctl -k -g 'unheaded-watch: '` |
| Alerts | `monitoring/prometheus/rules/tripwire.yml`; immediate list in `observability/alerting/policy.yaml` |
| Decoy defaults | ssh 2222, telnet 23→42023, http 80→42080, http-alt 8080, smb 445→42445, rdp 3389, postgres 5432, redis 6379 |

---

*S-EAST-TRIPWIRE Battle Plan, forged 2026-11-24*
*13 phases, 138 steps. East keeps quiet, and anything that reaches it on its own trips an alert.*
