# S-LOG-HARVEST BATTLE PLAN: 12 Phases, 214 Steps

**Date**: 2026-10-30
**Sprint**: S-LOG-HARVEST. Collect the log from every device, service and application in the lab and keep it in the long-term store. That includes Pi-hole, Grafana and the other third-party containers, and the network gear. Then prove it continuously.
**Prerequisite**: Stevie has answered decisions D1–D7 (Phase 0). The monitoring stack is up for the runtime phases (FND-001).
**Target**: Every source in `config/logging/sources.yaml` has a line in ClickHouse less than its `max_silence` old. A canary per source proves the path end to end. A CI gate fails when a new unit, container or host has no entry, and an alert fires when a source goes quiet.
**Estimated duration**: 10–14 h over 3–4 sessions. Phases 2 and 5 are code and need no stack. Phases 3, 4, 6 and 9 need the stack. Phases 7 and 8 need Stevie in a host window.
**Agent strategy**: Phases 1, 2 and 5 can run in parallel. Phases 3, 4 and 6 run one after another on west. Phase 7 (east) and Phase 8 (devices) need Stevie. Phases 9–11 run last.
**Commit cadence**: every 5 steps, at every exit gate, before every skip. Signed, named files only (never `git add -A <dir>`). Stevie pushes.
**Stuck protocol**: skip after 3× the step estimate or two failed debug attempts, then mark the step `[STUCK]` and its dependents `[BLOCKED]`.

## Grounding (read before Step 1)

This plan builds on what already exists. It doesn't reinvent it.
- **ADR-092** (`docs/adr/ADR-092-var-log-discipline-and-rotation.md`):
  - Local logs are a buffer; every log ships to long-term storage (Stevie, 2026-10-13).
  - Step 8: Vector's `journald` source ships the host journal to ClickHouse (`unheaded_logs`), with retention classes `audit` (455 d) and `operational` (90 d).
  - First-party container stdout goes into the journal through the journald driver (2026-10-27).
  - East's shipper is prepared but not installed (`deploy/systemd/vector-east.service`, `config/vector/vector-east.yaml`).
- **ADR-086 (Muninn)** was superseded by Vector. **ADR-087 (NOC)** plans network-device syslog but nothing is built.
- **Existing alerts** (`monitoring/prometheus/rules/logs.yml`): JournalNotShipping, JournalShipErrors, VectorDown, LogSilence (per unit, journal only), AuthFailureBurst. They only fire once the stack runs (FND-001).
- **Constraints:**
  - **Third-party freeze** (2026-10-29): this plan adds **no** new third-party library, tool, image or package. Vector, ClickHouse, Grafana, rsyslog and auditd are already on the inventory, and their verdicts are on hold. The plan only configures them, and D1 lets Stevie say no to that too. Every piece of new code is first-party Go on the standard library.
  - **Firewall**: Claude never applies firewall changes unattended. Those steps are marked **[STEVIE]**.
  - **orion** is out of scope (Stevie's personal server; no repo changes).

## Gap map (found 2026-10-30, read-only)

| Source | Host | Where it logs today | Shipped? |
|---|---|---|---|
| systemd units, kernel, sshd/sudo/PAM, docker daemon, LXD daemon, haproxy (`/dev/log`) | west | journal | **No**: Vector is built, but the stack is down (FND-001) |
| first-party containers (12) | west | journald driver | **No**: same reason |
| Grafana, traefik, postgres, clickhouse, victoria, coredns, volume-init | west | json-file 3×10 MB | **Never**: not in the journal |
| nginx access/error | west | `/var/log/nginx/*.log` | **Never** |
| fail2ban | west | `/var/log/fail2ban.log` | **Never** |
| FRR | west | `/var/log/frr/frr.log` | **Never** |
| auditd (kernel audit) | west, east | `/var/log/audit/audit.log` (0600 root); `systemd-journald-audit.socket` **disabled** | **Never**: ADR-092 says audit records ship, but they don't reach the journal |
| dpkg / apt / unattended-upgrades | west, east | `/var/log/{dpkg.log,apt/history.log,unattended-upgrades/}` | **Never** |
| Pi-hole: `pihole.log` (DNS queries), `FTL.log`, `webserver.log`, container journal | west (LXD `pihole`, 10.10.20.252) | files inside the container; container journal | **Never** |
| all east units (Unheaded services, bird, huginn …) | east | journal | **Never**: shipper not installed |
| router / AP (192.168.69.129), other LAN devices | LAN | their own buffers | **Never**: there is no receiver |
| DNS of LAN devices and east | LAN, east | nowhere: they stopped using Pi-hole on 2026-08-22, and east uses public resolvers (IOC sweep F-P6) | **Never**: needs Stevie's DNS decision (part of D7) |

## Decisions for Stevie (Phase 0 blocks on these)

| # | Question | Recommendation | Without an answer |
|---|---|---|---|
| D1 | Keep using Vector → ClickHouse as the collector and store while the third-party verdicts are on hold? | **Yes, configure only.** It's built, tested (`scripts/test-vector-config.sh`) and alerted on. Replacing it is the third-party discussion, not this sprint. | Phases 3, 4, 6, 7 and 9 are BLOCKED; Phases 1, 2 and 5 go ahead |
| D2 | What receives syslog from devices and the Pi-hole container? | **Native `cmd/syslog-relay`** (Go, stdlib): RFC 5424/3164 over UDP and TCP in, journald native protocol out, so Vector's existing journald source ships it. Alternative: rsyslog `imudp` on west, which is already installed but widens use of a third-party tool. | Phase 5 is built but not deployed |
| D3 | How do Pi-hole's logs leave the container? | **rsyslog inside the container**, already installed and active: `imfile` reads `pihole.log`/`FTL.log`/`webserver.log`, `omfwd` sends over TCP to the relay at 10.10.20.1. The config lives in the repo (`deploy/pihole/`). | Phase 6 is BLOCKED |
| D4 | How does east ship? | **Install the prepared `vector-east`** in a host window (decision E-D). Alternative: a native journal forwarder to west's relay, which means more code. | Phase 7 is BLOCKED |
| D5 | Move third-party container stdout (Grafana, traefik, postgres, clickhouse, victoria, coredns, volume-init) to the journald driver? Vector stays on json-file. | **Yes**: one path for everything, and `check-compose-log-caps.sh` already accepts journald | Phase 4 is BLOCKED |
| D6 | Audit records: turn on `systemd-journald-audit.socket`, or read `audit.log` as a file? | **File source** with auditd `log_group = adm`. This keeps the 1 GB journal budget for everything else. | Phase 3's audit steps are BLOCKED |
| D7 | Network devices: which models, can they send syslog, and do we allow 514/udp+tcp on west from their IPs only? | Stevie identifies them; **Stevie applies the firewall rule** | Phase 8 is BLOCKED |

---

## LEGEND

- `[B]`: bash command, run directly
- `[V]`: verification; it must pass before you go on
- `[D]`: debug step, only if the step before it failed
- `[W]`: write or create a file
- `[R]`: read or inspect a file
- `[S]`: needs sudo
- `[P]`: can run in parallel with other `[P]` steps
- `[C]`: commit checkpoint (signed, named files)
- `[STEVIE]`: Stevie runs it or says yes first (firewall, host window, decision)
- `[STUCK]` / `[BLOCKED]`: set by the Skip Protocol

`~Nm` is the expected wall-clock time.

---

## PHASE 0: DECISIONS AND PRECONDITIONS (Steps 1–12)

**Goal**: D1–D7 are answered and recorded, and the branch and tools are ready.
**Prerequisite**: none.
**Time**: 20 min plus Stevie.
**Agent**: Coordinator.

- [ ] **Step 1** [R] ~5m: Read the grounding docs: ADR-092 (all of it), ADR-086, ADR-087 §Muninn extension, `config/vector/vector.yaml`, `monitoring/prometheus/rules/logs.yml`.
- [ ] **Step 2** [STEVIE] ~?: Ask D1–D7 from the table above. Record the answers verbatim under "Decisions" in `docs/battle-plans/progress/S-LOG-HARVEST-progress.md`.
- [ ] **Step 3** [V] ~1m: Every D has an answer, or is marked "not yet" with the phases it blocks marked `[BLOCKED]`.
- [ ] **Step 4** [B] ~1m: Branch off develop.
  ```bash
  cd /home/govan/tmp/unheaded && git switch develop && git switch -c sprint/log-harvest
  ```
- [ ] **Step 5** [B] ~1m: Check the tools are present. **Don't run a command that isn't installed.** On Ubuntu, `lxc` is the lxd-installer wrapper, and running it installs the LXD snap (this happened on east on 2026-10-30 and was reverted).
  ```bash
  for t in go docker /snap/bin/lxc journalctl logger dig ss; do command -v $t >/dev/null && echo "$t ok" || echo "$t MISSING"; done
  ssh -o BatchMode=yes east 'for t in journalctl logger ss; do command -v $t >/dev/null && echo "east $t ok" || echo "east $t MISSING"; done'
  ```
- [ ] **Step 6** [V] ~1m: Everything is ok. `dig` missing on west → use `getent hosts` for the canaries in Phase 6 (don't install bind9-dnsutils without asking).
- [ ] **Step 7** [B] ~1m: Is the stack up? (Phases 3, 4, 6 and 9 need it.)
  ```bash
  sudo -n docker ps --format '{{.Names}} {{.Status}}' | grep -E 'unheaded-(vector|clickhouse|grafana|prometheus|victoria)' || echo "STACK DOWN (FND-001)"
  ```
- [ ] **Step 8** [V] ~1m: Stack up → go on. Down → Phases 3, 4, 6 and 9 do their code and config steps, and their runtime `[V]` steps wait. Write that in the progress file. Bringing the stack up is Stevie's call (FND-001).
- [ ] **Step 9** [B] ~2m: Baseline the gates this sprint touches.
  ```bash
  ./scripts/check-compose-log-caps.sh; ./scripts/test-vector-config.sh; go test ./pkg/uids/... 2>&1 | tail -3
  ```
- [ ] **Step 10** [V] ~1m: All three pass on the branch tip. Red → fix or record it before going on; this sprint must not inherit red.
- [ ] **Step 11** [W] ~3m: Create `docs/battle-plans/progress/S-LOG-HARVEST-progress.md` with three sections: Decisions, Step log, Stuck.
- [ ] **Step 12** [C][V] ~1m: **PHASE 0 EXIT GATE.** Decisions are recorded and the baseline is green. Commit `[PLAN S-LOG-HARVEST] Steps 1-12: decisions + baseline`.

---

## PHASE 1: SOURCE INVENTORY (Steps 13–34)

**Goal**: One machine-readable list of every log source that should exist, with how to prove it ships.
**Prerequisite**: Phase 0.
**Time**: 60 min.
**Agent**: Agent [P], read-only on the hosts.

- [ ] **Step 13** [B][P] ~2m: Running units on each host.
  ```bash
  systemctl list-units --type=service --state=running --no-legend --plain | awk '{print $1}' | sort > /tmp/claude-1000/lh-west-units.txt
  ssh east "systemctl list-units --type=service --state=running --no-legend --plain | awk '{print \$1}' | sort" > /tmp/claude-1000/lh-east-units.txt
  wc -l /tmp/claude-1000/lh-*-units.txt
  ```
- [ ] **Step 14** [B][P] ~2m: Files written in the last 7 days outside the journal.
  ```bash
  for h in west east; do echo "## $h"; c="sudo -n find /var/log -type f -mtime -7 -size +0 2>/dev/null | grep -vE '(\.gz|\.[0-9]+)$|/journal/|/sysstat/|wtmp|btmp|lastlog' | sort"; [ $h = west ] && bash -c "$c" || ssh east "$c"; done
  ```
- [ ] **Step 15** [B][P] ~2m: Where each file-logging daemon points (syslog or file).
  ```bash
  grep -nE '^\s*log' /etc/haproxy/haproxy.cfg; grep -nE '^logtarget' /etc/fail2ban/fail2ban.{conf,local} 2>/dev/null; grep -nE '^log ' /etc/frr/frr.conf; grep -rnE '^\s*(access_log|error_log)' /etc/nginx/; systemctl is-enabled systemd-journald-audit.socket
  ```
- [ ] **Step 16** [B][P] ~2m: Compose services and their logging drivers.
  ```bash
  docker compose config --format json 2>/dev/null | python3 -I -c 'import json,sys; c=json.load(sys.stdin); [print(n, (s.get("logging") or {}).get("driver","default")) for n,s in sorted(c["services"].items())]'
  ```
- [ ] **Step 17** [B][P] ~2m: Pi-hole container: its log files, rsyslog state, journal size.
  ```bash
  sudo -n /snap/bin/lxc exec pihole -- sh -c 'ls -la /var/log/pihole; systemctl is-active rsyslog; journalctl --disk-usage'
  ```
- [ ] **Step 18** [B][P] ~2m: Other LXD instances and stale neighbours on lxdbr0. (2026-10-30: .66/.128/.234/.127 seen with only `pihole` listed; the IOC sweep has the explanation.)
  ```bash
  sudo -n /snap/bin/lxc list -f csv; ip neigh show dev lxdbr0
  ```
- [ ] **Step 19** [B][P] ~2m: LAN devices seen from each host.
  ```bash
  ip neigh show | awk '$NF!="FAILED"{print $1,$3,$5}' | sort -u; ssh east "ip neigh show | awk '\$NF!=\"FAILED\"{print \$1,\$3,\$5}' | sort -u"
  ```
- [ ] **Step 20** [W] ~15m: Write `config/logging/sources.yaml`. Schema `unheaded.logging.sources/v1`; one entry per source:
  ```yaml
  - id: west/nginx-access          # host/name, unique
    host: west                     # west | east | pihole | device:<name>
    kind: file                     # journal-unit | journal-container | file | lxd-file | lxd-journal | syslog-device
    match: /var/log/nginx/access.log   # unit name, container name, path, or sender IP
    retention: operational         # audit | operational (ADR-092 / retention-policy-v1)
    max_silence: 6h                # how long quiet is normal
    canary: http-get               # logger | http-get | dns-query | none(+reason)
    owner: architect
    status: gap                    # shipped | gap | blocked:<D#> | excluded:<reason>
  ```
  Cover every running unit from Step 13 (rule: `unheaded-*`, `huginn`, `node-exporter`, `ssh`, `auditd`, `docker`, `containerd`, `haproxy`, `nginx`, `fail2ban`, `frr`, `bird`, `lldpd`, `chrony`, `snap.lxd.daemon`, and `kernel`). Also cover every compose service from Step 16, every file from Step 14, Pi-hole's three files plus its journal, and every device from Step 19 (named `device:<ip>` until Stevie names it).
- [ ] **Step 21** [V] ~2m: Every row of the gap map above has an entry.
  ```bash
  grep -cE '^- id:' config/logging/sources.yaml; grep -E 'status: (gap|blocked)' config/logging/sources.yaml | wc -l
  ```
- [ ] **Step 22** [B] ~1m: Every id is unique.
  ```bash
  grep -E '^- id:' config/logging/sources.yaml | sort | uniq -d
  ```
- [ ] **Step 23** [V] ~1m: There's no output.
- [ ] **Step 24** [W] ~5m: Excluded sources need a reason in the file (for example: `sysstat`, binary and not a log; `wtmp`, already in the journal as a PAM session). Nothing is silently left out.
- [ ] **Step 25** [C] ~1m: Commit `config/logging/sources.yaml` and the progress file.
- [ ] **Step 26** [R] ~5m: Cross-check the retention classes against `docs/policy/retention-policy-v1-2026-05-06.md`. Pick a class for every source:
  - auth, sudo, PAM, audit, fail2ban, dpkg/apt → `audit`.
  - Pi-hole queries → `operational`. The query DB keeps 91 d (`maxDBdays = 91`); 90 d matches it.
- [ ] **Step 27** [V] ~1m: Every entry has a retention class the policy names.
- [ ] **Step 28** [W] ~5m: Add a **privacy note** to `sources.yaml` and the progress file. The Pi-hole query log names every LAN client and every domain it looks up; nginx logs client IPs. Both are personal data under the retention policy. They ship to ClickHouse (loopback only, one user) and nowhere else.
- [ ] **Step 29** [B] ~2m: Check the file with Python's built-in YAML-free path. The repo already parses YAML in CI, so check how (`grep -rn 'yaml' scripts/check-*.sh | head`) and use the same tool. Don't add a parser.
- [ ] **Step 30** [V] ~1m: It parses.
- [ ] **Step 31** [D] ~3m: Doesn't parse → fix indentation; the schema is flat on purpose.
- [ ] **Step 32** [W] ~3m: Under "Gap map" in the progress file, put a count: sources by status (`shipped` / `gap` / `blocked` / `excluded`). This is the sprint's burn-down.
- [ ] **Step 33** [V] ~1m: **PHASE 1 EXIT GATE.** `sources.yaml` exists, parses and is unique, and every gap-map row has an entry.
- [ ] **Step 34** [C] ~1m: Commit `[PLAN S-LOG-HARVEST] Steps 13-34: source inventory`.

---

## PHASE 2: STATIC COVERAGE GATE (Steps 35–58)

**Goal**: CI fails when a new systemd unit, compose service or host has no `sources.yaml` entry. The gate is provoked red before it's trusted.
**Prerequisite**: Phase 1.
**Time**: 75 min.
**Agent**: Agent [P] (Go, no host access).

- [ ] **Step 35** [R] ~5m: Read the patterns: `scripts/check-compose-log-caps.sh`, `pkg/uids/compose_test.go` (a Go test that reads the compose file), and `scripts/check-gates-can-fail.sh` (the meta-gate).
- [ ] **Step 36** [W] ~10m: Test first. `pkg/logsources/logsources_test.go`:
  - `TestEveryUnitHasASource`: every `deploy/systemd/*.service` and `*@.service`.
  - `TestEveryComposeServiceHasASource`.
  - `TestEveryHostHasASource`: `compliance/hosts/*.yaml`.
  - `TestNoDuplicateIDs`.
  - `TestRetentionClassKnown`.
  - `TestExcludedNeedsReason`.
- [ ] **Step 37** [B] ~1m: Run it. It must fail, because the package doesn't exist yet.
  ```bash
  go test ./pkg/logsources/... 2>&1 | tail -5
  ```
- [ ] **Step 38** [W] ~15m: `pkg/logsources/logsources.go` has `Load(path) ([]Source, error)` and `Validate([]Source) []error`. Use the stdlib only. Use the YAML reader the repo already depends on if there is one (`grep -n yaml go.mod`); if there isn't, write `sources.yaml` as the JSON-compatible YAML subset and parse it with `encoding/json`. **No new module.**
- [ ] **Step 39** [B] ~2m:
  ```bash
  go test ./pkg/logsources/... -race 2>&1 | tail -5
  ```
- [ ] **Step 40** [V] ~1m: Green.
- [ ] **Step 41** [D] ~5m: Red on a real gap → add the missing entry to `sources.yaml` (status `gap`). Don't weaken the test.
- [ ] **Step 42** [C] ~1m: Commit.
- [ ] **Step 43** [B] ~3m: **Provoke 1.** Add `deploy/systemd/unheaded-zzz-probe.service` (uncommitted) → the test must fail and name it. Then delete it.
- [ ] **Step 44** [B] ~3m: **Provoke 2.** Add a compose service `zzz-probe` in a scratch copy, through an env override of the compose path → it must fail. Then revert.
- [ ] **Step 45** [B] ~3m: **Provoke 3.** Duplicate an id in `sources.yaml` → it must fail. Then revert.
- [ ] **Step 46** [V] ~1m: All three provokes went red with a message that names the culprit.
- [ ] **Step 47** [W] ~5m: `scripts/check-log-sources.sh` wraps the go test and exits non-zero on any error, in the same style as `check-compose-log-caps.sh`.
- [ ] **Step 48** [W] ~5m: Register it with the meta-gate (`scripts/check-gates-can-fail.sh`) and give it one provoke the meta-gate runs.
- [ ] **Step 49** [B] ~3m:
  ```bash
  ./scripts/check-gates-can-fail.sh 2>&1 | tail -15
  ```
- [ ] **Step 50** [V] ~1m: The new gate is listed as `can fail: yes`. The only other red is the known ruff freshness hold.
- [ ] **Step 51** [W] ~5m: Wire it into CI. Add a step to the job in `.github/workflows/security.yml` that runs `check-compose-log-caps.sh`. **No new action.** It's a shell step.
- [ ] **Step 52** [B] ~2m: Check the workflow YAML with the repo's existing workflow lint, if there is one (`grep -rn actionlint scripts/ .github/`); otherwise `python3 -I -c` parse it.
- [ ] **Step 53** [V] ~1m: It parses, and the step sits next to the existing log-caps step.
- [ ] **Step 54** [W] ~5m: Add to `man`/docs: a `docs/policy/log-sources.md` stub (what the file is, how to add a source, the status values). Run `gen-man` only if a command was added (none here).
- [ ] **Step 55** [B] ~2m: `gofmt -l pkg/logsources; go vet ./pkg/logsources/...; ./scripts/check-gosec-ratchet.sh 2>&1 | tail -3` (never `~/go/bin/gosec`: it checks nothing on Go 1.27)
- [ ] **Step 56** [V] ~1m: Clean.
- [ ] **Step 57** [V] ~1m: **PHASE 2 EXIT GATE.** The gate is green on the tree, red on all three provokes, registered with the meta-gate and wired into CI.
- [ ] **Step 58** [C] ~1m: Commit `[PLAN S-LOG-HARVEST] Steps 35-58: static coverage gate`.

---

## PHASE 3: WEST HOST FILE SOURCES (Steps 59–88)

**Goal**: nginx, fail2ban, FRR, audit, and dpkg/apt/unattended-upgrades on west ship to ClickHouse with the right retention class.
**Prerequisite**: Phase 1; D1 yes; D6 answered for the audit steps. The stack must be up for Steps 80–86.
**Time**: 90 min.
**Agent**: Coordinator (host reads, sudo).

- [ ] **Step 59** [R] ~5m: Read `config/vector/vector.yaml`'s `journal_rows` remap and `redact_secrets`. File rows must reuse the same row shape and pass through `redact_secrets`.
- [ ] **Step 60** [B] ~2m: File ownership and modes, which decide what Vector's uid (16706 + group adm) can read.
  ```bash
  sudo -n stat -c '%n %U:%G %a' /var/log/nginx/*.log /var/log/fail2ban.log /var/log/frr/frr.log /var/log/audit/audit.log /var/log/dpkg.log /var/log/apt/history.log /var/log/unattended-upgrades/*.log 2>&1
  ```
- [ ] **Step 61** [V] ~1m: Note which files are readable by group adm. As of 2026-10-30, nginx is `www-data:adm 640`, which is readable. Any file that isn't readable needs a host change (Step 62) and not a chmod by hand.
- [ ] **Step 62** [STEVIE][S] ~5m: Host changes, applied by the host profile under `compliance/hosts/west.yaml` so they're reproducible, never hand-edited:
  - D6 = file: `/etc/audit/auditd.conf` `log_group = adm`, then `systemctl restart auditd` (it re-creates `audit.log` 0640 root:adm).
  - FRR: no change if `frr.log` is already adm-readable.
  - fail2ban: `logtarget = SYSTEMD-JOURNAL` (one path, the journal) instead of a file source. **Recommended**, because it's simpler than a file source.
- [ ] **Step 63** [V] ~1m: Re-run Step 60; every file in scope is adm-readable or moved to the journal.
- [ ] **Step 64** [W] ~10m: `config/vector/vector.yaml`: a `host_files` source, `type: file`, with includes for the paths in scope and `read_from: beginning` on the first run only (the checkpoint lives in `vector-data`). Set `ignore_older_secs: 604800`.
- [ ] **Step 65** [W] ~10m: A `file_rows` remap gives the same columns as `journal_rows`:
  - `unit` = a short source name (`nginx-access`, `audit`, …).
  - `retention` = `audit` for audit/dpkg/apt/fail2ban, `operational` for the rest.
  - `host` = the host label.
  - The message hash is part of the dedup key.
- [ ] **Step 66** [W] ~5m: Route `file_rows` through `redact_secrets` into the existing `clickhouse` sink.
- [ ] **Step 67** [W] ~5m: In `docker-compose.yml`, mount each file's directory read-only into `vector`: `/var/log/nginx`, `/var/log/audit`, `/var/log/apt`, `/var/log/unattended-upgrades`, `/var/log/frr`, and `dpkg.log` as a single file. No other directories.
- [ ] **Step 68** [W] ~10m: Tests in `config/vector/tests/files.yaml`:
  - an nginx line → `unit=nginx-access`, `retention=operational`;
  - an audit `type=USER_AUTH` line → `retention=audit`;
  - a dpkg `install` line → `audit`;
  - a line carrying `PASSWORD=hunter2` comes out redacted.
- [ ] **Step 69** [B] ~3m:
  ```bash
  ./scripts/test-vector-config.sh 2>&1 | tail -8
  ```
- [ ] **Step 70** [V] ~1m: All tests pass, including the existing redact/ssh/volume tests.
- [ ] **Step 71** [D] ~5m: Failure is a VRL type error → check `parse_*` fallibility (use `!` only where the input is ours). Failure is the vector-east transforms-equality check → mirror the transform in `vector-east.yaml`.
- [ ] **Step 72** [C] ~1m: Commit (vector.yaml, tests/files.yaml, compose).
- [ ] **Step 73** [B] ~2m: The provoke. Break the retention mapping (audit → operational) → the test must go red. Then revert.
- [ ] **Step 74** [V] ~1m: It went red.
- [ ] **Step 75** [W] ~5m: In `sources.yaml`, flip the Phase 3 entries to `shipped`. Mark them pending until Step 86 passes.
- [ ] **Step 76** [B] ~2m: `./scripts/check-compose-log-caps.sh && ./scripts/check-log-sources.sh`
- [ ] **Step 77** [V] ~1m: Green.
- [ ] **Step 78** [C] ~1m: Commit.
- [ ] **Step 79** [STEVIE] ~2m: The stack is up, or Stevie says yes to `docker compose up -d vector` (it recreates Vector only).
- [ ] **Step 80** [B] ~2m: Canary per file source.
  ```bash
  T=lh-$(date +%s); curl -s -o /dev/null "http://127.0.0.1/$T" ; sudo -n logger -t lh-canary "$T"
  ```
  (nginx logs the path.) For audit, run `sudo -n true`, which produces a USER_CMD record. For dpkg there's no safe canary, so check the last-seen time.
- [ ] **Step 81** [B] ~2m: Query ClickHouse over its HTTP interface inside the container (loopback only).
  ```bash
  sudo -n docker exec unheaded-clickhouse clickhouse-client -q "SELECT unit, retention, max(timestamp) FROM unheaded_logs.logs WHERE timestamp > now() - INTERVAL 10 MINUTE AND unit IN ('nginx-access','audit') GROUP BY unit, retention"
  ```
- [ ] **Step 82** [V] ~1m: nginx-access shows `operational` and audit shows `audit`, both within the last 2 minutes.
- [ ] **Step 83** [D] ~5m: Nothing arrives → `docker logs unheaded-vector 2>&1 | grep -iE 'permission|file' | tail`. "Permission denied" means Step 62 didn't take. "No such file" means the mount path is wrong.
- [ ] **Step 84** [B] ~2m: Restart Vector and send the canary again. There must be exactly one row per canary (the dedup key works across a checkpoint resume).
- [ ] **Step 85** [V] ~1m: One row.
- [ ] **Step 86** [V] ~1m: **PHASE 3 EXIT GATE.** Every Phase 3 source has a row less than 10 minutes old with the right class, the tests are green, and the provoke went red.
- [ ] **Step 87** [W] ~2m: Update the progress file and the burn-down count.
- [ ] **Step 88** [C] ~1m: Commit `[PLAN S-LOG-HARVEST] Steps 59-88: west file sources ship`.

---

## PHASE 4: THIRD-PARTY CONTAINER STDOUT, INCLUDING GRAFANA (Steps 89–104)

**Goal**: Grafana, traefik, postgres, clickhouse, victoria, coredns and volume-init log through journald, and Vector ships them. Vector itself stays on json-file.
**Prerequisite**: D5 yes; Phase 3 done (it uses the same Vector).
**Time**: 40 min.
**Agent**: Coordinator.

- [ ] **Step 89** [R] ~3m: Read the 2026-10-27 amendment in ADR-092 and the `x-logging-journald` anchor in `docker-compose.yml`.
- [ ] **Step 90** [W] ~5m: Switch each listed service's `logging:` to `*x-logging-journald` (tag `{{.Name}}`). Leave `vector` on json-file with a comment: it must not ingest its own output.
- [ ] **Step 91** [W] ~3m: Grafana: set `GF_LOG_MODE=console` and `GF_LOG_LEVEL=info` (env in compose) so its own logs go to stdout and not a file in its volume. Check with `grep -n GF_LOG docker-compose.yml`.
- [ ] **Step 92** [B] ~2m: `./scripts/check-compose-log-caps.sh`
- [ ] **Step 93** [V] ~1m: Green. It accepts journald because every host profile applies the journald caps.
- [ ] **Step 94** [B] ~2m: Provoke. Set grafana to `driver: local` → the gate must go red. Then revert.
- [ ] **Step 95** [V] ~1m: It went red.
- [ ] **Step 96** [C] ~1m: Commit.
- [ ] **Step 97** [STEVIE] ~3m: Recreate the third-party containers: `docker compose up -d grafana traefik postgres clickhouse victoria coredns`. **The Well (postgres) restarts.** Do it in a quiet moment, and only on Stevie's yes.
- [ ] **Step 98** [B] ~2m: Grafana canary: an unauthenticated `GET /api/health` and a login page hit.
  ```bash
  curl -s -o /dev/null -w '%{http_code}\n' http://127.0.0.1:3000/api/health
  ```
- [ ] **Step 99** [B] ~2m:
  ```bash
  sudo -n docker exec unheaded-clickhouse clickhouse-client -q "SELECT unit, count(), max(timestamp) FROM unheaded_logs.logs WHERE timestamp > now() - INTERVAL 10 MINUTE AND unit IN ('unheaded-grafana','unheaded-traefik','unheaded-postgres','unheaded-clickhouse','unheaded-victoria','unheaded-coredns') GROUP BY unit"
  ```
  (Check the container names with `docker ps` first; adjust them if they aren't `unheaded-*`.)
- [ ] **Step 100** [V] ~1m: Every container that logged since the restart has rows. A container that is quiet by nature gets `max_silence` set in `sources.yaml` instead of a failure.
- [ ] **Step 101** [D] ~5m: Missing → `journalctl CONTAINER_NAME=<name> -n 5` on the host. If the lines are there, the problem is Vector-side (unit mapping); if they aren't, it's the driver.
- [ ] **Step 102** [B] ~2m: Postgres logs can contain statement text. Check that a failed login with a fake password shows up redacted (`redact_secrets`).
- [ ] **Step 103** [V] ~1m: **PHASE 4 EXIT GATE.** All seven ship or are documented as quiet, Grafana is in the table, and nothing secret leaks.
- [ ] **Step 104** [C] ~1m: Commit `[PLAN S-LOG-HARVEST] Steps 89-104: third-party container stdout ships`.

---

## PHASE 5: NATIVE SYSLOG RELAY `cmd/syslog-relay` (Steps 105–140)

**Goal**: A first-party, stdlib-only receiver. It takes RFC 5424 and RFC 3164 over UDP and TCP and writes each message to the local journal through the native protocol, with the sender recorded. Vector's existing journald source then ships it.
**Prerequisite**: Phase 0 (D2 = native). This phase is code only, so it can run before the stack is up.
**Time**: 3 h.
**Agent**: Agent [P] (Go, TDD).

**Design** (write it as the ADR in Step 105 before any code):
- **Listeners**: configurable `--listen udp://10.10.20.1:514`, `--listen tcp://10.10.20.1:514`, plus the LAN address for devices (D7). Never `0.0.0.0` by default. No capability: systemd binds 514 in a `.socket` unit and passes it (socket activation, Stevie's answer #18 of 2026-11-09; ADR-124).
- **Output**: the `/run/systemd/journal/socket` native protocol (the datagram is `KEY=value\n` pairs; values containing a newline use the binary length form). Fields:
  - `MESSAGE`, `PRIORITY`, `SYSLOG_FACILITY`, `SYSLOG_IDENTIFIER` (from the message);
  - `REMOTE_ADDR`, `REMOTE_HOSTNAME_CLAIMED` (the hostname the message claims, **not trusted**);
  - `UNHEADED_SOURCE` = the `sources.yaml` id whose `match` is the sender IP, or `unknown`.
- **Hostile input**: 8 KiB max message (RFC 5424 §6.1 recommends at least 2048 be accepted; anything longer is truncated and flagged). Per-sender token bucket (default 200 msg/s, burst 1000); over the limit, it drops and counts. Control characters are escaped. The TCP octet-counting frame length is capped, and so is the TCP connection count. An idle timeout applies.
- **Allowlist**: senders not in `sources.yaml` are counted (`syslog_relay_unknown_sender_total`) and dropped by default (`--accept-unknown` is for the inventory phase only).
- **Metrics**: `:9599` on loopback. Received, dropped{reason}, per-source last-seen.
- **Identity**: its own nologin user `unheaded-syslog-relay`, registered in pkg/uids and `deploy/sysusers.d/unheaded.conf`, with the house systemd hardening.

- [ ] **Step 105** [W] ~20m: `docs/adr/ADR-111-native-syslog-relay.md` (the next free number: `ls docs/adr | sort -V | tail -3`). Context = the gap map. Decision = the design above. Alternatives = rsyslog `imudp`, and `systemd-journal-remote` (a new package, so a third-party freeze ask). Add it to `ADR-INDEX.md` and `wiki/ADR-Index.md`.
- [ ] **Step 106** [C] ~1m: Commit the ADR.
- [ ] **Step 107** [W] ~20m: Tests first, in `pkg/syslog/parse_test.go`:
  - RFC 5424 examples §6.5 (all four);
  - RFC 3164 BSD lines (with and without hostname, and no PRI);
  - PRI out of range (`<192>`);
  - a truncated header;
  - a NUL in the message;
  - a 1 MiB input that gets truncated;
  - invalid UTF-8;
  - structured data containing a `]`;
  - octet-counting frames: valid, length greater than the cap, length smaller than the frame.
- [ ] **Step 108** [B] ~1m: `go test ./pkg/syslog/...` must fail to compile, because nothing exists yet.
- [ ] **Step 109** [W] ~40m: `pkg/syslog/parse.go`: `Parse([]byte) (Message, error)`, `Frame(r *bufio.Reader, max int) ([]byte, error)`. Stdlib only.
- [ ] **Step 110** [B] ~2m: `go test ./pkg/syslog/... -race -count=1 2>&1 | tail -5`
- [ ] **Step 111** [V] ~1m: Green.
- [ ] **Step 112** [C] ~1m: Commit.
- [ ] **Step 113** [W] ~10m: `FuzzParse` and `FuzzFrame`. The invariant: no panic, output ≤ max, and no raw control bytes in `MESSAGE`.
- [ ] **Step 114** [B] ~6m:
  ```bash
  go test ./pkg/syslog/ -run '^$' -fuzz FuzzParse -fuzztime 120s && go test ./pkg/syslog/ -run '^$' -fuzz FuzzFrame -fuzztime 120s
  ```
- [ ] **Step 115** [V] ~1m: No crashers. If there are any, add the input to `testdata/fuzz` as a regression test and fix it.
- [ ] **Step 116** [W] ~5m: Register it with LICH as `tomb/lich/LICH-016-syslog-relay/README.md`: the target, the corpus, the invariant.
- [ ] **Step 117** [C] ~1m: Commit.
- [ ] **Step 118** [W] ~20m: Tests first, in `pkg/journald/native_test.go`. The test sets up a unixgram socket in a tempdir, encodes `MESSAGE` with an embedded newline (binary length form), rejects field names outside `[A-Z0-9_]`, and checks that a leading `_` (trusted fields) can't be set.
- [ ] **Step 119** [W] ~20m: `pkg/journald/native.go`: `Send(fields map[string]string) error`. A datagram over the socket's limit → `memfd` fallback is out of scope; truncate and set `UNHEADED_TRUNCATED=1`.
- [ ] **Step 120** [B] ~2m: `go test ./pkg/journald/... -race`
- [ ] **Step 121** [V] ~1m: Green.
- [ ] **Step 122** [C] ~1m: Commit.
- [ ] **Step 123** [W] ~30m: `cmd/syslog-relay/main.go`: flags `--listen` (repeatable), `--sources config/logging/sources.yaml`, `--rate`, `--burst`, `--max-msg`, `--max-conns`, `--metrics 127.0.0.1:9599`, `--accept-unknown`, `--man`. It uses `pkg/logsources` for the allowlist. On SIGHUP it reloads sources.
- [ ] **Step 124** [W] ~20m: `cmd/syslog-relay/main_test.go`, end to end:
  - start the relay on 127.0.0.1 ephemeral ports with a fake journal socket;
  - send UDP 5424, TCP octet-counted and TCP LF-framed messages;
  - assert the fields that arrive;
  - an unknown sender is dropped and counted;
  - a burst over the limit is dropped and counted;
  - a 1 MiB TCP frame doesn't grow memory (cap check) and closes the connection.
- [ ] **Step 125** [B] ~3m: `go test ./cmd/syslog-relay/... -race -count=1 2>&1 | tail -5`
- [ ] **Step 126** [V] ~1m: Green.
- [ ] **Step 127** [D] ~5m: A flaky UDP test → wait on the fake journal socket with a deadline, not a sleep.
- [ ] **Step 128** [C] ~1m: Commit.
- [ ] **Step 129** [W] ~5m: In pkg/uids, add `syslog-relay` from the free range; in `deploy/sysusers.d/unheaded.conf`, add `u unheaded-syslog-relay <uid> - - /usr/sbin/nologin`.
- [ ] **Step 130** [W] ~10m: `deploy/systemd/unheaded-syslog-relay.service`, a copy of the `unheaded-kev-poller.service` hardening, plus `deploy/systemd/unheaded-syslog-relay.socket` (socket activation, answer #18 of 2026-11-09):
  - the socket: `ListenDatagram=10.10.20.1:514`, `ListenStream=10.10.20.1:514`, `FreeBind=yes`, its own `IPAddressAllow`; the relay reads `LISTEN_FDS` natively and refuses a socket its `--listen` does not name.
  - `User=unheaded-syslog-relay`, no capability (`CapabilityBoundingSet=` empty), `PrivateUsers=yes`, `Requires=` the socket.
  - `RestrictAddressFamilies=AF_UNIX AF_INET AF_INET6`.
  - `ProtectSystem=strict`, `MemoryMax=128M`.
  - `ExecStart=/usr/local/bin/syslog-relay --listen udp://10.10.20.1:514 --listen tcp://10.10.20.1:514 --sources /etc/unheaded/log-sources.yaml`.
- [ ] **Step 131** [B] ~2m: `systemd-analyze verify deploy/systemd/unheaded-syslog-relay.service 2>&1 | grep -v 'not found' | head`; `systemd-analyze security --offline=yes deploy/systemd/unheaded-syslog-relay.service 2>/dev/null | tail -1`
- [ ] **Step 132** [V] ~1m: It verifies, and the exposure score is ≤ the kev-poller unit's.
- [ ] **Step 133** [B] ~2m: `go test ./pkg/uids/... ./pkg/logsources/...`. The unit is in `sources.yaml` (Phase 2's gate catches it if it isn't).
- [ ] **Step 134** [V] ~1m: Green.
- [ ] **Step 135** [B] ~3m: Gates: `gofmt -l`, `go vet`, gosec, `scripts/check-dependency-baseline.sh` (must be **unchanged**, no new modules), gen-man (adds `man/syslog-relay.1`).
- [ ] **Step 136** [V] ~1m: All green; the dependency baseline is unchanged.
- [ ] **Step 137** [C] ~1m: Commit.
- [ ] **Step 138** [W] ~5m: Add `monitoring/prometheus/prometheus.yml` job `syslog-relay`, plus rules in `logs.yml`:
  - SyslogRelayDown;
  - SyslogRelayDropping: `increase(syslog_relay_dropped_total[15m]) > 0` by reason;
  - SyslogUnknownSender.
  Each has a promtool test and a runbook (`runbooks/observe/syslog-relay.yaml`, read-only steps). Refresh the MANIFEST.
- [ ] **Step 139** [B] ~3m: `./scripts/check-prometheus-rules.sh && /tmp/claude-1000/civenv/bin/pytest tests/runbooks -q 2>&1 | tail -3`
- [ ] **Step 140** [V][C] ~2m: **PHASE 5 EXIT GATE.** The relay builds and is tested and fuzzed; the unit verifies; there are no new dependencies; rules and runbook are green. Commit `[PLAN S-LOG-HARVEST] Steps 105-140: native syslog relay`.

---
## PHASE 6: PI-HOLE → RELAY → JOURNAL → CLICKHOUSE (Steps 141–164)

**Goal**: Pi-hole's DNS query log, FTL log, web server log and container journal reach ClickHouse, tagged `pihole/*`.
**Prerequisite**: Phase 5. D2 and D3 answered. The stack is up for Steps 158–162.
**Time**: 75 min.
**Agent**: Coordinator (sudo, LXD).

- [ ] **Step 141** [STEVIE][S] ~5m: Install the relay on west. Stevie says yes to a new listening service.
  ```bash
  cd /home/govan/tmp/unheaded && go build -trimpath -o /tmp/claude-1000/syslog-relay ./cmd/syslog-relay
  sudo -n install -m 0755 /tmp/claude-1000/syslog-relay /usr/local/bin/syslog-relay
  sudo -n install -D -m 0644 config/logging/sources.yaml /etc/unheaded/log-sources.yaml
  sudo -n install -m 0644 deploy/sysusers.d/unheaded.conf /etc/sysusers.d/unheaded.conf && sudo -n systemd-sysusers
  sudo -n install -m 0644 deploy/systemd/unheaded-syslog-relay.service deploy/systemd/unheaded-syslog-relay.socket /etc/systemd/system/
  sudo -n systemctl daemon-reload && sudo -n systemctl enable --now unheaded-syslog-relay.socket unheaded-syslog-relay.service
  ```
- [ ] **Step 142** [V] ~1m: The relay listens on lxdbr0's address only.
  ```bash
  systemctl is-active unheaded-syslog-relay && sudo -n ss -lunpt | grep ':514 '
  ```
  Expect `10.10.20.1:514` (udp and tcp), and nothing on `0.0.0.0` or the LAN address.
- [ ] **Step 143** [D] ~3m: The bind fails → `journalctl -u unheaded-syslog-relay -n 20`. The 514 bind is the socket's (`systemctl status unheaded-syslog-relay.socket`; `FreeBind=yes` covers lxdbr0 not being up yet). The relay refusing a passed socket means the socket's `Listen*=` lines and `ExecStart=`'s `--listen` differ.
- [ ] **Step 144** [B] ~2m: Local canary, sent from the host as a known source (`west/self`, in sources.yaml).
  ```bash
  T=lh-$(date +%s); printf '<134>1 %s west lh-canary - - - %s' "$(date -u +%FT%TZ)" "$T" > /dev/udp/10.10.20.1/514; sleep 1; journalctl -t lh-canary --since -1min -o cat | grep "$T"
  ```
- [ ] **Step 145** [V] ~1m: The line is in the journal with `REMOTE_ADDR` and `UNHEADED_SOURCE` set (`journalctl -t lh-canary -o verbose -n1`).
- [ ] **Step 146** [C] ~1m: Record in the progress file: relay live on west.
- [ ] **Step 147** [B] ~2m: Can the container reach the relay? The test sends from inside the container, so nothing is installed.
  ```bash
  sudo -n /snap/bin/lxc exec pihole -- bash -c 'T=lh-ct-$(date +%s); echo "<134>$(date "+%b %e %T") pihole lh-canary: $T" > /dev/udp/10.10.20.1/514; echo $T'
  ```
- [ ] **Step 148** [V] ~1m: The token shows in the host journal.
- [ ] **Step 149** [D][STEVIE] ~5m: It doesn't arrive → it's the west input firewall on lxdbr0. **Don't change the firewall.** Write the exact rule for Stevie: `iifname "lxdbr0" ip saddr 10.10.20.252 ip daddr 10.10.20.1 udp/tcp dport 514 accept`, placed in the host profile (`compliance/hosts/west.yaml` → nft). Mark Steps 150–162 `[BLOCKED]` until Stevie has applied it.
- [ ] **Step 150** [W] ~15m: `deploy/pihole/60-unheaded-forward.conf` (rsyslog, RainerScript):
  - `module(load="imfile")`;
  - one `input(type="imfile" File="/var/log/pihole/pihole.log" Tag="pihole-dns" Severity="info" Facility="local5")` per file: pihole.log → `pihole-dns`, FTL.log → `pihole-ftl`, webserver.log → `pihole-web`;
  - `*.* action(type="omfwd" Target="10.10.20.1" Port="514" Protocol="tcp" TCP_Framing="octet-counted" Template="RSYSLOG_SyslogProtocol23Format" queue.type="LinkedList" queue.filename="unheaded-fwd" queue.maxDiskSpace="64m" queue.saveOnShutdown="on" action.resumeRetryCount="-1")`.
  The disk queue holds logs while the host relay restarts. That's rsyslog core functionality, **not a new package**.
  **Written 2026-11-10, and it departs from the line above** (tested with rsyslogd 8.2504.0 on made-up lines, network off; the reasons are in the file's comments):
  - The queue as specified (disk-assisted, default size) held a share of the lines back until shutdown once it passed its high watermark, and lost lines at shutdown in two runs (178 of 1505, 60 of 1545). The file lines now have their own ruleset with a 5000-line memory queue that makes imfile wait at 4500, so the log file is the buffer. The disk part is kept for shutdown only.
  - The relay drops above 200 lines a second per sender (burst 1000). `pihole.log.1` peaked at 802 lines in a second and 14,577 in a minute, so the queue sends 150 a second.
  - `rsyslog.conf` in the container has `$RepeatedMsgReduction on`, which folded 500 identical lines into one "message repeated". It is off for the two forward actions; `/var/log/syslog` keeps it.
  - File lines go to the relay only, not also into the container's `/var/log/syslog` (the DNS log is personal data). The container's own syslog has a separate memory queue that drops when full, so local logging never waits on the relay.
  - Results on the file as committed: burst 1505 of 1505, relay down 12 s 8050 of 8050, relay restart 2000 of 2000, rsyslog restart 3000 of 3000, rsyslog stopped while the relay is down 998 of 1000 (the batch in hand). Never more than 150 a second.
  - Not done: `rsyslogd -N1` on the container's 8.2312.0 (Step 153). rsyslogd is AppArmor-confined on west and in the container, so the tests ran the host binary in a container with no network.
- [ ] **Step 151** [B] ~2m: Check that rsyslog in the container has imfile (it's part of the rsyslog package; check, don't install).
  ```bash
  sudo -n /snap/bin/lxc exec pihole -- sh -c 'ls /usr/lib/x86_64-linux-gnu/rsyslog/ | grep -E "imfile|omfwd" ; rsyslogd -v | head -1'
  ```
- [ ] **Step 152** [V] ~1m: imfile is present (omfwd is built in). If it's missing → STOP and ask Stevie (it would be a package change).
- [ ] **Step 153** [S] ~3m: Push the config and validate it **before** restarting.
  ```bash
  sudo -n /snap/bin/lxc file push deploy/pihole/60-unheaded-forward.conf pihole/etc/rsyslog.d/60-unheaded-forward.conf --mode 0644
  sudo -n /snap/bin/lxc exec pihole -- rsyslogd -N1 2>&1 | tail -3
  ```
- [ ] **Step 154** [V] ~1m: `rsyslogd: End of config validation run. Bye.` with no errors.
- [ ] **Step 155** [D] ~3m: An imfile permission error → the files are `pihole:pihole 0640`, and rsyslog in the container runs as `syslog`. The fix is `usermod -aG pihole syslog` inside the container (Stevie OK: a group membership change in the container) or a `$PrivDropToUser` change. Don't chmod the files: Pi-hole's logrotate re-creates them.
- [ ] **Step 156** [S] ~1m: `sudo -n /snap/bin/lxc exec pihole -- systemctl restart rsyslog && sudo -n /snap/bin/lxc exec pihole -- systemctl is-active rsyslog`
- [ ] **Step 157** [C] ~1m: Commit `deploy/pihole/60-unheaded-forward.conf` and the sources.yaml entries `pihole/dns`, `pihole/ftl`, `pihole/web` and `pihole/journal` (match = 10.10.20.252 + tag).
- [ ] **Step 158** [B] ~2m: DNS canary: query a name through Pi-hole that's unique and harmless (`.invalid` never leaves Pi-hole for the internet as a real name; it gets NXDOMAIN locally or upstream).
  ```bash
  T=lh-$(date +%s); dig +short @10.10.20.252 "$T.canary.invalid" >/dev/null 2>&1 || getent hosts "$T.canary.invalid"; echo $T
  ```
- [ ] **Step 159** [B] ~2m:
  ```bash
  sudo -n docker exec unheaded-clickhouse clickhouse-client -q "SELECT unit, retention, message FROM unheaded_logs.logs WHERE timestamp > now() - INTERVAL 5 MINUTE AND message LIKE '%$T%' FORMAT TSV"
  ```
- [ ] **Step 160** [V] ~1m: A `pihole/dns` row (unit = the relay's `UNHEADED_SOURCE`, host `pihole`) with the query, `retention=operational` from the row, not the facility.
- [ ] **Step 161** [B] ~3m: Durability. Stop the relay for 60 s, send a canary, start the relay → the canary arrives (rsyslog's disk queue held it).
- [ ] **Step 162** [V] ~1m: It arrived exactly once.
  Note 2026-11-10: the canary waits in rsyslog's memory queue or in the log file, not on disk (Step 150). TCP has no acknowledgement, so a line written to the socket as the relay stops can be lost and a batch can arrive twice; Step 164d fills a loss.
- [ ] **Step 163** [V] ~1m: **PHASE 6 EXIT GATE.** All four Pi-hole sources have rows, the DNS canary works end to end, and the queue survives a relay restart.
- [ ] **Step 164** [C] ~1m: Commit `[PLAN S-LOG-HARVEST] Steps 141-164: Pi-hole ships`.

### The old history (Steps 164a–164f, added 2026-11-10, lettered so the numbering holds)

Stevie, 2026-11-09 (#33): export the old Pi-hole history into the relay's format, lose nothing, then delete the retired rows. `scripts/pihole-history.sh` does it; `scripts/test-pihole-history.sh` proves it on made-up data. The staging directory holds personal data (every LAN client and every name it looked up): it is 0700, outside the repo, and the last stage removes it. Needs Steps 141–163 passed, so the relay's own `pihole/dns` rows exist.

- [ ] **Step 164a** [STEVIE][S] ~5m: Export. Read-only in the container; FTL's session table is not exported. Root only makes the directory; the tool runs as you (lxc and docker need no sudo on west).
  ```bash
  cd /home/govan/tmp/unheaded && sudo install -d -m 0700 -o "$(id -un)" -g "$(id -gn)" /var/lib/unheaded/pihole-history && scripts/pihole-history.sh export 2>&1 | tail -3 | cut -c1-120; stat -c '%a %U' /var/lib/unheaded/pihole-history; du -sh /var/lib/unheaded/pihole-history/in | cut -c1-60
  ```
  Expect: `exported to ...: N log files, M stored queries (zone ...)`, `700 govan`, a size. Else: the message names the file it could not read.
- [ ] **Step 164b** [B] ~10m: Transform (`clickhouse local`, no network).
  ```bash
  cd /home/govan/tmp/unheaded && scripts/pihole-history.sh transform 2>&1 | tail -8 | cut -c1-120
  ```
  Expect: rows per unit, rows rebuilt from FTL's table (older than the oldest file line), rows past 90 days counted and not written. Else: "no time at all" names a file with no timestamp in it: stop, and read that file's head.
- [ ] **Step 164c** [B] ~10m: Load. It refuses until the relay's rows match staged lines, so the two meet without a gap.
  ```bash
  cd /home/govan/tmp/unheaded && scripts/pihole-history.sh load 2>&1 | tail -8 | cut -c1-120
  ```
  Expect: rows loaded per unit, and the count left to the relay's own copy. Else: "no relay rows" → Phase 6 is not live yet; "match nothing" → the relay's rows and the files disagree: STOP, do not pass `--allow-no-overlap` without reading both.
- [ ] **Step 164d** [V] ~3m: Verify. A rerun of `load` adds nothing; a rerun after a relay gap loads only what the relay did not ship.
  ```bash
  cd /home/govan/tmp/unheaded && scripts/pihole-history.sh verify 2>&1 | tail -4 | cut -c1-120
  ```
  Expect: every staged row is in the table or left to the relay's copy, exit 0. Else: do not clean; the staging is the only copy outside the container.
- [ ] **Step 164e** [S] ~1m: Clean (refuses without a passed verify).
  ```bash
  cd /home/govan/tmp/unheaded && scripts/pihole-history.sh clean 2>&1 | tail -2 | cut -c1-100; sudo rmdir /var/lib/unheaded/pihole-history 2>/dev/null; test ! -e /var/lib/unheaded/pihole-history && echo GONE
  ```
  Expect: `staging removed`, then `GONE` (the tool empties the directory; root removes the empty directory it made).
- [ ] **Step 164f** [W][C] ~10m: Delete the retired rows `pihole/pihole-log`, `pihole/ftl-log`, `pihole/webserver-log` from `config/logging/sources.yaml` (keep `lan/dns-queries`), rerun `scripts/check-log-sources.sh` and the gap map, commit. Imported rows carry `source = 'import'`.

---

## PHASE 7: EAST (Steps 165–178)

**Goal**: East's journal, audit and dpkg/apt logs reach the same ClickHouse.
**Prerequisite**: D4 answered and a host window booked with Stevie. Phase 3's transforms are mirrored into `vector-east.yaml` (the equality check in `test-vector-config.sh` enforces this).
**Time**: 45 min in the window.
**Agent**: Coordinator plus Stevie.

- [ ] **Step 165** [R] ~5m: Read `deploy/systemd/vector-east.service` and `config/vector/vector-east.yaml`. Note the sink address (west's ClickHouse over WireGuard, or loopback?) and the credential path (an env file, never the repo).
- [ ] **Step 166** [W] ~10m: Add the east file sources (audit, dpkg, apt, unattended-upgrades) to `vector-east.yaml`. The same `file_rows` remap.
- [ ] **Step 167** [B] ~3m: `./scripts/test-vector-config.sh`
- [ ] **Step 168** [V] ~1m: Green, and the transforms are equal.
- [ ] **Step 169** [C] ~1m: Commit.
- [ ] **Step 170** [STEVIE][S] ~15m: In the host window, install it per D4. For prepared vector-east: install the binary from the pinned image (`docker create` + `docker cp` on west, scp to east, the same digest), the unit, the env file (0600, root), auditd `log_group = adm` (D6), and `systemctl enable --now vector-east`. **Never** run a package manager or `lxc` on east without checking.
- [ ] **Step 171** [V] ~2m: `ssh east systemctl is-active vector-east` and `ssh east journalctl -u vector-east -n 5 --no-pager`
- [ ] **Step 172** [B] ~2m: Canary.
  ```bash
  T=lh-east-$(date +%s); ssh east "logger -t lh-canary $T"; sleep 20
  sudo -n docker exec unheaded-clickhouse clickhouse-client -q "SELECT host, unit, retention FROM unheaded_logs.logs WHERE message LIKE '%$T%'"
  ```
- [ ] **Step 173** [V] ~1m: There's a row with `host=east`.
- [ ] **Step 174** [B] ~2m: Audit canary: `ssh east sudo -n true`, then check for a `host=east unit=audit retention=audit` row.
- [ ] **Step 175** [V] ~1m: Present.
- [ ] **Step 176** [W] ~3m: Flip the east entries in sources.yaml to `shipped`.
- [ ] **Step 177** [V] ~1m: **PHASE 7 EXIT GATE.** East's journal, audit and package logs are in ClickHouse.
- [ ] **Step 178** [C] ~1m: Commit `[PLAN S-LOG-HARVEST] Steps 165-178: east ships`.

---

## PHASE 8: NETWORK DEVICES (Steps 179–192)

**Goal**: Every LAN device that can send syslog does, to the relay's LAN listener, and every device that can't is recorded as `excluded` with the reason.
**Prerequisite**: Phase 5 deployed (Phase 6 Step 141); D7 answered.
**Time**: 30 min plus Stevie at each device's admin UI.
**Agent**: Stevie plus Coordinator.

- [ ] **Step 179** [STEVIE] ~10m: Name every device from Phase 1 Step 19: the default gateway/AP 192.168.69.129, the east–west link, switches, the NAS, printers. For each one: the model, whether it does remote syslog, and its transport (udp/tcp/TLS).
- [ ] **Step 180** [W] ~5m: Rename `device:<ip>` entries in sources.yaml to `device:<name>`, with `match: <ip>`. Mark the ones that can't send syslog `excluded: no remote syslog (<model>)`. **Caveat (IOC sweep 2026-10-30, F-P6):** since 2026-08-22 the only Pi-hole client is west, and east resolves via 1.1.1.1/8.8.8.8 directly. So a device that can't send syslog has *no* log at all unless Stevie points LAN DHCP DNS (and east) back at Pi-hole. Record that choice in the reason.
- [ ] **Step 181** [C] ~1m: Commit.
- [ ] **Step 182** [W] ~3m: Add the LAN listener to the unit's `ExecStart`: `--listen udp://<west LAN IP>:514`, plus TCP if the devices support it. Bind the LAN address only.
- [ ] **Step 183** [STEVIE][S] ~5m: Firewall: allow 514 on west **only from the named device IPs**, through the host profile. Stevie applies it.
- [ ] **Step 184** [S] ~2m: Reinstall the unit and sources, then `systemctl restart unheaded-syslog-relay`.
- [ ] **Step 185** [V] ~1m: `ss -lun | grep ':514 '` shows lxdbr0 and the LAN address only.
- [ ] **Step 186** [B] ~2m: From a host that is **not** on the allowlist (east), send one message to the LAN listener. It must be dropped and counted as an unknown sender (`curl -s 127.0.0.1:9599/metrics | grep unknown_sender`).
- [ ] **Step 187** [V] ~1m: Dropped and counted, and the SyslogUnknownSender rule would fire.
- [ ] **Step 188** [STEVIE] ~10m: On each capable device, set the remote syslog server to west's LAN IP:514.
- [ ] **Step 189** [B] ~2m: Generate an event on each device (log in to its UI, or toggle a setting Stevie names), then:
  ```bash
  sudo -n docker exec unheaded-clickhouse clickhouse-client -q "SELECT JSONExtractString(fields,'UNHEADED_SOURCE') s, count(), max(timestamp) FROM unheaded_logs.logs WHERE timestamp > now() - INTERVAL 30 MINUTE AND unit != '' AND s LIKE 'device:%' GROUP BY s"
  ```
  (Adjust the `fields` column name to the schema in `config/clickhouse/schema/001-logs.sql`.)
- [ ] **Step 190** [V] ~1m: Every capable device has a row.
- [ ] **Step 191** [V] ~1m: **PHASE 8 EXIT GATE.** Every device is either `shipped` with a row or `excluded` with a reason.
- [ ] **Step 192** [C] ~1m: Commit `[PLAN S-LOG-HARVEST] Steps 179-192: network devices`.

---

## PHASE 9: RUNTIME COVERAGE: PROVE EVERY SOURCE, CONTINUOUSLY (Steps 193–206)

**Goal**: A per-source last-seen metric for every `sources.yaml` entry, with an alert when a shipped source has been quiet past its `max_silence`. This is the "ensure" in the sprint goal; LogSilence covers journal units only.
**Prerequisite**: Phases 3–8 done or blocked. The stack is up.
**Time**: 90 min.
**Agent**: Agent (Go) plus Coordinator for the deploy.

- [ ] **Step 193** [W] ~20m: Tests first, in `cmd/logcov/main_test.go`. A fake ClickHouse `httptest.Server` answers `SELECT ... GROUP BY` as TSV. logcov maps the rows to source ids and writes `unheaded_log_source_last_seen_seconds{source,host,kind}` plus `unheaded_log_source_max_silence_seconds{source}` in Prometheus text format. An `excluded` entry emits nothing; an unknown row is counted under `source="unmatched"`.
- [ ] **Step 194** [W] ~30m: `cmd/logcov/main.go`. It's stdlib only (`net/http` to ClickHouse's HTTP interface on 127.0.0.1:8123, a read-only user, the password from env or a credential file, never argv). One query per run groups by the source key: `unit`/host for the journal and files, `UNHEADED_SOURCE` for the relay. It writes atomically to the node-exporter textfile directory (`/var/lib/node_exporter/textfile/logcov.prom`; check the path in `deploy/systemd/node-exporter.service`).
- [ ] **Step 195** [B] ~2m: `go test ./cmd/logcov/... -race`
- [ ] **Step 196** [V] ~1m: Green.
- [ ] **Step 197** [C] ~1m: Commit.
- [ ] **Step 198** [W] ~10m: `deploy/systemd/unheaded-logcov.{service,timer}`: every 5 min, its own nologin user (pkg/uids, sysusers), house hardening, read-only except the textfile directory. A ClickHouse read-only user with `SELECT` on `unheaded_logs.logs` only, added in `config/clickhouse/schema/` (a grant, with the password from env).
- [ ] **Step 199** [W] ~10m: Rule `LogSourceSilent` in `logs.yml`: `time() - unheaded_log_source_last_seen_seconds > on(source) unheaded_log_source_max_silence_seconds`, for 15m, severity warning (critical for the `audit` class). Also `LogCoverageStale`: the logcov textfile is older than 15 min. Add the promtool tests, a runbook `runbooks/observe/log-source-silent.yaml` (read-only diagnosis per kind) and refresh the MANIFEST.
- [ ] **Step 200** [B] ~3m: `./scripts/check-prometheus-rules.sh && /tmp/claude-1000/civenv/bin/pytest tests/runbooks -q 2>&1 | tail -3`
- [ ] **Step 201** [V] ~1m: Green.
- [ ] **Step 202** [W] ~15m: A Grafana dashboard JSON `monitoring/grafana/dashboards/log-coverage.json`: a table of sources with last seen, max silence and status, coloured by state. Provisioned like the existing dashboards. "Human-only live view" applies.
- [ ] **Step 203** [STEVIE][S] ~5m: Deploy logcov and its timer on west, and reload Prometheus.
- [ ] **Step 204** [B] ~3m: **Provoke**: stop the Pi-hole container's rsyslog for `max_silence + 20m` (set a temporary 1m `max_silence` on `pihole/web` in `/etc/unheaded/log-sources.yaml` only) → LogSourceSilent fires for `pihole/web`. Then restore both.
- [ ] **Step 205** [V] ~1m: The alert fired and cleared.
- [ ] **Step 206** [V][C] ~2m: **PHASE 9 EXIT GATE.** Every `shipped` source shows a last-seen under its max silence; the provoke fired the alert. Commit `[PLAN S-LOG-HARVEST] Steps 193-206: runtime coverage`.

---

## PHASE 10: RECORDS AND COMPLIANCE (Steps 207–211)

**Goal**: The docs say what's true.
**Agent**: Librarian.

- [ ] **Step 207** [W] ~10m: ADR-092: steps 5 and 8 progress, the file sources, east, the Pi-hole and device paths, and logcov. ADR-086: a note that the relay is the native piece Muninn's §journald/syslog intended. ADR-087: the device syslog section points at the relay.
- [ ] **Step 208** [W] ~10m: `wiki/Logging.md` (new, and added to `_Sidebar.md`) holds the ops facts: where each log goes, the retention classes, how to query ClickHouse, the relay ports, the Pi-hole config path, and how to add a source. Update CLAUDE.md (one line plus the link) and the runbook counts.
- [ ] **Step 209** [W] ~10m: Compliance evidence: map to NIST 800-53 AU-2/AU-3/AU-6/AU-9/AU-11/AU-12/SI-4 and CIS 8.2/8.5/8.9/8.10 in the existing control files (`grep -rln 'AU-12' compliance/ | head`). The evidence is `sources.yaml`, logcov and LogSourceSilent.
- [ ] **Step 210** [B] ~3m: The docs gates: `./scripts/check-gates-can-fail.sh 2>&1 | tail -10`
- [ ] **Step 211** [V][C] ~1m: **PHASE 10 EXIT GATE.** Only the known holds are red. Commit `[PLAN S-LOG-HARVEST] Steps 207-211: records`.

---

## PHASE 11: FINAL GATE AND HANDOFF (Steps 212–214)

- [ ] **Step 212** [B] ~10m: The full sweep: `go test ./... 2>&1 | grep -cE '^ok'`, `go test ./... 2>&1 | grep FAIL`, `./scripts/test-vector-config.sh`, `./scripts/check-log-sources.sh`, plus one canary per kind (journal, file, container, Pi-hole DNS, east, device) queried in a single ClickHouse statement.
- [ ] **Step 213** [V] ~2m: **SPRINT EXIT GATE.** In sources.yaml every entry is `shipped` (with a row under its max silence) or `excluded` with a reason, and `gap` = 0. Any `blocked:<D#>` is listed for Stevie.
- [ ] **Step 214** [W][C] ~5m: Write the handoff to `~/tmp/next.md` and the progress file. Commit `[PLAN S-LOG-HARVEST] Steps 212-214: sprint closed`. Stevie pushes.

---

## APPENDIX A: EMERGENCY PROCEDURES

**A1. Vector backfill floods ClickHouse or the disk.** The first start with new file sources reads up to 7 days back (`ignore_older_secs`).
1. `sudo -n docker exec unheaded-clickhouse clickhouse-client -q "SELECT formatReadableSize(sum(bytes_on_disk)) FROM system.parts WHERE database='unheaded_logs'"`
2. `df -h /var/lib/docker`
3. If it's above 80%: `docker compose stop vector` (Vector's checkpoints persist, so nothing is lost), and lower `ignore_older_secs` to 86400.

**A2. The relay floods the journal** (a device in a loop). The journal cap (1 GB) protects the disk but rotates out other units' history.
1. `curl -s 127.0.0.1:9599/metrics | grep dropped`
2. Lower `--rate` for that source with a per-source override in sources.yaml, then SIGHUP.
3. Last resort: `systemctl stop unheaded-syslog-relay`. Devices buffer or drop; nothing else is affected.

**A3. Pi-hole DNS breaks after the rsyslog change.** It can't, because rsyslog isn't in the DNS path; but if DNS fails:
1. `sudo -n /snap/bin/lxc exec pihole -- pihole status`
2. Remove `60-unheaded-forward.conf` and restart rsyslog.
3. Only if FTL itself is down, restart `pihole-FTL`; tell Stevie.

**A4. The Postgres restart in Phase 4 leaves the Well unhealthy.** Follow `runbooks/` well-health. Don't proceed with Phase 4 until `pg_isready` passes.

**A5. Lockout risk.** Every firewall step is [STEVIE]. If a host becomes unreachable, stop. Claude never tries to fix a firewall remotely.

**A6. The lxd-installer trap.** On a host without the LXD snap, `lxc` *installs* it. Use `/snap/bin/lxc` explicitly, and only on west.

## APPENDIX B: AGENT ASSIGNMENT MATRIX

| Phase | Agent | Parallel | Depends on | Time | Needs Stevie |
|---|---|---|---|---|---|
| 0 Decisions | Coordinator | no | none | 20m | D1–D7 |
| 1 Inventory | Agent | [P] with 2 and 5 | 0 | 60m | no |
| 2 Static gate | Agent | [P] | 1 | 75m | no |
| 3 West files | Coordinator | no | 1, D1, D6 | 90m | host profile change, stack up |
| 4 Third-party stdout | Coordinator | no | 3, D5 | 40m | container recreate |
| 5 Syslog relay | Agent | [P] | 0, D2 | 3h | no |
| 6 Pi-hole | Coordinator | no | 5, D3 | 75m | relay install, maybe firewall |
| 7 East | Coordinator + Stevie | [P] with 6 | 3, D4 | 45m | host window |
| 8 Devices | Stevie + Coordinator | after 6 | 5, D7 | 30m+ | device UIs, firewall |
| 9 Runtime coverage | Agent + Coordinator | no | 3–8 | 90m | deploy |
| 10 Records | Librarian | [P] with 9 | 3–8 | 30m | no |
| 11 Final | Coordinator | no | all | 20m | push |

**Critical path**: 0 → 1 → 5 → 6 → 8 → 9 → 11 (about 8 h of work, plus Stevie's windows).

## APPENDIX C: QUICK REFERENCE

| Thing | Value |
|---|---|
| Long-term store | ClickHouse `unheaded_logs` (schema `config/clickhouse/schema/001-logs.sql`), loopback only |
| Query | `sudo -n docker exec unheaded-clickhouse clickhouse-client -q "<SQL>"` |
| Retention | `audit` 455 d, `operational` 90 d (table TTLs; `docs/policy/retention-policy-v1-2026-05-06.md`) |
| Vector config / tests | `config/vector/vector.yaml`, `vector-east.yaml`, `config/vector/tests/`, `scripts/test-vector-config.sh` |
| Vector metrics | 172.17.0.1:9598 (docker0 only), job `vector` |
| Relay | `cmd/syslog-relay`, unit `unheaded-syslog-relay`, 10.10.20.1:514 udp/tcp (+ LAN after D7), metrics 127.0.0.1:9599 |
| Pi-hole | LXD `pihole` on west, 10.10.20.252; logs `/var/log/pihole/{pihole,FTL,webserver}.log`; config `/etc/pihole/pihole.toml`; forwarder `deploy/pihole/60-unheaded-forward.conf` |
| Source list | `config/logging/sources.yaml` → `/etc/unheaded/log-sources.yaml`; gate `scripts/check-log-sources.sh` |
| Coverage | `cmd/logcov` → textfile `unheaded_log_source_last_seen_seconds`; alert `LogSourceSilent` |
| Hosts | west 192.168.13.2 (lab link), east 192.168.13.1; default gateway/AP 192.168.69.129 |

---

*S-LOG-HARVEST Battle Plan, forged 2026-10-30*
*12 phases. 214 steps. Every raven comes home to the same well, and none comes home in silence.*
