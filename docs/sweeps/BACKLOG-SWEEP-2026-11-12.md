<!--
SPDX-License-Identifier: GPL-3.0-or-later
Copyright (c) 2026 Stevie Bellis.
-->
# Backlog sweep (2026-11-12)

## How this was made

1. Sources: `~/tmp/next.md` (513 lines, as of 2026-11-12 07:20Z), `docs/adr/ADR-INDEX.md`, `compliance/findings/register.yaml`, `docs/battle-plans/` with `progress/`, `docs/sweeps/`, and `git log` on `develop` at `fe4b4e5e`.
2. Date: 2026-11-12, while the night session was still committing (ADR-144 `8d7f5ad6` and the export audit `fe4b4e5e` landed during the sweep and are included).
3. Read-only: nothing was edited, committed, started or run with sudo; this file is the only output.
4. Made by an AI agent (Claude), with three read-only sub-agents for the 2026-11-04 sprint progress files; their rows are marked PROG and were spot-checked (14 of 14 checked claims held).
5. Not yet checked by a person. Where two sources disagree the row is in group E, not guessed.

Stevie's ask (2026-11-11, todo 7): "ensure you have visibility and /unheaded-warmonger battle plans for all backlog items". This file is the visibility half. Plans are forged from it in a later session.

## Counts per group

| Group | Meaning | Rows |
|---|---|---|
| A | Next move is Stevie's (hands or a decision), no plan step covers it | 68 |
| B | Covered by an existing plan (plan and steps named) | 28 |
| C | ADRs with status Proposed, Deferred or In Progress | 45 |
| D | Open, agent-doable, NO plan (candidates for new plans) | 34 |
| E | Status unclear (two sources disagree, or no closure found) | 16 |
| | Total | 191 |

One row can stand for several small items of the same kind (the row says how many). The findings register has 169 open entries (25 FND ids, 144 evidence-source rows, 77 of those accepted with an expiry); each is placed in a group through Appendix C.

## Source keys

| Key | Means |
|---|---|
| N-night | `~/tmp/next.md`, "Night session 2026-11-12" |
| N-todo n | `~/tmp/next.md`, "Stevie's todo notes", item n |
| N-queue n | `~/tmp/next.md`, agreed queue of 13, item n |
| N-decide n | `~/tmp/next.md`, "Decide first", numbered item n |
| N-hands n | `~/tmp/next.md`, "For Stevie (your hands)", item n |
| N-Q n | `~/tmp/next.md`, "Questions for you", question n |
| N-know, N-watchdog, N-state | `~/tmp/next.md`, "Things you should know", "Task watchdog retired", "State" |
| REG | `compliance/findings/register.yaml` |
| OUT M-nnn | `docs/sweeps/OUTSTANDING-2026-10-27.md` |
| DP-n | `docs/battle-plans/DECISION-PACK-2026-11-03.md` |
| E1 n | `docs/battle-plans/S-E1-LIVE-battle-plan.md`, Step n; E1-prog is its progress file |
| LH n | `docs/battle-plans/S-LOG-HARVEST-battle-plan.md`, Step n; LH-prog is its progress file |
| PROG plan:line | `docs/battle-plans/progress/<plan>-progress.md`, line number |

## A. Needs Stevie's hands or decision

### A1. Hands (minutes each)

| ID | Item, and exactly what | Source |
|---|---|---|
| A-01 | Seal `runbooks/MANIFEST.sha256` and place the public key on west. Then in `zhen-cli`: `/schedule resume`, `/confirm <token>`. Until then every live runbook is refused and the scheduler stays paused. | N-night queue 1; N-hands 2 |
| A-02 | Approve `runbooks/security/ioc-audit.yaml` (`status: approved`, `approved_by`, `approved_at`), run `scripts/runbook-manifest.sh --write`, seal, uncomment the four `ioc-audit` lines in `configs/zhen-schedules.yaml`. Checked: `approved_by` is empty and the lines are commented. | N-night "three things" 1 |
| A-03 | Re-sign, then push. Unsigned: 14 commits on develop (`c870e9e4` to `fe4b4e5e`), `36d7c93` in orb, `9f69412` in unheaded-terraform. develop is 27 ahead of `origin/develop`. | N-night "three things" 3; N-hands 9; `git log origin/develop..develop` |
| A-04 | Look at the monitor once: `orb -gfx` blanking after 5 minutes was tested with no panel attached. | N-night "three things" 2 |
| A-05 | Delete vor's `harden/api-dos-and-traversal` branch on GitHub if not yet done. | N-hands 9; N-decide 5 |
| A-06 | Optional: create `unheaded-waf` on GitHub and push it; `unheaded-terraform` has no remote either. | N-hands 9; N-night queue 11 |
| A-07 | `sudo docker builder prune` on west: old builder cache layers still hold the pre-rotation secrets. | N-night queue 3 |
| A-08 | Delete `/etc/unheaded/rotation-20261112T063907Z` once the new passwords have held a few days. | N-night queue 4 |
| A-09 | `sudo bash ~/.local/state/unheaded/lanfwd.sh up` if the laptop test continues (Prometheus, Alertmanager, VictoriaMetrics forwards are down since the power-off and have no login). | N-night last bullet; N-decide 1 |
| A-10 | Root disk, his call: `/root/tmp` 20G, `/root/bakunheaded` 2.2G, `~/tmp/wt` 22G (39 worktrees, most merged), `~/.venv` 27G, `/opt/rocm-6.4.2` 18G, python dist-packages 16G, ollama 10G, Rust `target/` about 9G, grype and trivy databases 4.8G. | N-night 1 |
| A-11 | Scratch left on disk that a safety check stopped Claude deleting: root-owned synthetic files under an old scratchpad, clean clones `/var/tmp/unheaded-sprint/mg-clone-1110a`, `b`, `c`, the merged worktree `.claude/worktrees/agent-aa2a6bfa1f6254235`. | N-know |
| A-12 | Moved-out leftovers in the checkout: untracked `demos/doom/` (a whole-tree stage would pick it up), ignored build output `crates/doom-runner/target` and `crates/zhenai-forge/target`, `cmd/doom-go-injector`; two uncommitted edits in `~/tmp/projects/doomgeneric` (`r_data.c`, `w_wad.c`) that are in no repo. | PROG S-MOVE-OUT:9, 28, 29; `git status` |
| A-13 | steel-echo-v2 x3 still run under LXD on west (DP-711 said delete): allow `restricted.backups` and rerun S-HOST-TIDY Steps 47-48, or delete without an export. | PROG S-HOST-TIDY:28, 29 |
| A-14 | Install cargo-fuzz on west (a third-party tool, so his word): the local `fuzz_mbc_execute` run is a skip, only GitHub's `lich.yml` runs it. | PROG S-STAGE-2B:21, 40, 105, 119 |
| A-15 | Outside the repo, noted only: two blog slugs with a guessed intent (`evpn-pig-pen`, `letter-to-nsa`); the blog "editor" is not built and not asked for. | N "Blog topic ideas" |

### A2. Decisions that block or shape work

| ID | Decision, with the options on record | Source |
|---|---|---|
| A-16 | Zhen on the LAN: (A) close `POST /api/v1/highlight` behind the login and drop `session_id` from `/health`, then open (recommended in the note); or (B) open as it is. Zhen itself is paused on his word; opening is `zhen_lan.sh up` after it is started. | N-night queue 8 |
| A-17 | IOC audit, log text: (A) more Vector counters by kind, or (B) a read-only ClickHouse login for `zhen-agentd`. Also: narrow `IOC_OPERATOR_NETS` for west? | N-night 4 |
| A-18 | Alert on Wotan topic refusals (`increase(unheaded_wotan_topic_permission_refusals_total[15m]) > 0`)? It needs a runbook file. Checked: no such rule exists. | N-night queue 6 |
| A-19 | `~/.config/cs` still named in about 30 tracked files outside dated records: READMEs are a plain edit; the system prompt text in `pkg/zhenrag/prompt.go` is a prompt change that re-runs the runbook eval (ADR-024). Rule on both. | N-night queue 5 |
| A-20 | Pi-hole blocklists: his word per list. Survey recommends HaGeZi Multi Normal first, one week, then the AdGuard DNS filter. | N-night "Todo, blocklists"; `docs/policy/pihole-blocklist-survey-2026-11-12.md` |
| A-21 | Pi-hole over IPv6, three attended steps in order: a ULA /64 on `lxdbr0`; an `ip6 nat` rule for port 53 matching `ip6 saddr fe80::/10` only; router RDNSS and DHCPv6 DNS set to west's link-local address (and DHCPv4 DNS to `.184`). No plan exists; see bundle 8. | N-night "Todo, Pi-hole IPv6" |
| A-22 | Trace collector `LATENCY_MAP` defect (71,000 `bpf()` calls a second, map never shrinks, inserts fail silently at 65,536 entries; east at 1.56 cores for 20 days): his word to take the fix out of the freeze, and whether to record it in the findings register. Same item as the unanswered eviction question from S-WIRE-HONEST. | N-night "Todo, footprint"; ADR-144; PROG S-WIRE-HONEST-lat:21, 29 |
| A-23 | East (question 28): (a) Evenings 10 and 10B together with him, (b) Claude makes the apt key unseen and walks the runbook after his approval, (c) a test cut through the old unsigned repo. And whether the Step 206 certificates, the Steps 190/192 edits and a trial deb are done ahead. Blocks B-10 to B-12. | N-Q 28; N-decide 5 |
| A-24 | Drafts, before Step 261: five judgment calls, chiefly ruling 37 (the replay check became per signature, not per packet). His note: later, with Fable. | N-decide 4; E1-prog "Evening 14, second sitting" |
| A-25 | Export notification email: send or not. The audit he asked for is done: `docs/legal/EXPORT-CRYPTO-AUDIT-2026-11-12.md` (`fe4b4e5e`, by an agent, not legal advice, not checked by a person). It found no algorithm of the project's own, and three self-defined constructions that encrypt or establish keys (`pkg/secrets/encryption/age.go`, the zhend hybrid KEM, the KEM tunnel of the PQC draft). Read its section "What a person must read in the regulation before deciding", decide, then E1 20-21. Also his call: delete the two constructions that have no callers. | N-night queue 13; N-Q 26; N-hands 8 |
| A-26 | Go bumps held: anthropic-sdk-go 1.84.0 is clean alone; grpc 1.85.0 adds `go.opencensus.io` and `golang/groupcache` as go.sum lines. | N-hands 7 |
| A-27 | "Log it to the well" (compliance page): (a) the page reads from the Well, (b) a Kanban card, (c) page-load timing. | N-todo 18 |
| A-28 | ADR-117 questions: (1) rule evaluator after Prometheus: keep until ADR-116's warden, native, `vmalert`, or Grafana alerting; (2) Grafana's calls to grafana.com, a ClickHouse datasource plugin, the seven unported dashboards, sophia-eye's own Prometheus; (3) the literal Grafana password in `tomb/dark-mirror/docker-compose.yml`. | N-todo 19; S-ONE-GRAFANA plan E1-E3 |
| A-29 | Zhen source viewer: vendor the Google Fonts and drop both hosts from the policy; make relative links open the other sheet; give the chat page a Content-Security-Policy. | N-todo 20, questions 1-3 |
| A-30 | Zhen's new name: "not now". When he names it or asks for candidates it becomes a parked ADR. 943 tracked files mention `zhen`. | N-todo 21 |
| A-31 | vor has no certification-path sheet (CCNA, JNCIA): his to write or to ask for. | N-todo 16 "Left" |
| A-32 | Pi-hole forward, three questions: Step 155 group add; imported rows `source = 'import'` or `'journald'`; accept the forward config's residual loss. | N-Q 4-6; LH-prog For Stevie 10-12 |
| A-33 | Accept two rows in `docs/policy/http-auth-baseline.txt`: `cmd/zhen-ui-front/main.go` and `cmd/logstore-writer/main.go`. | N-Q 7, 31 |
| A-34 | How the scraper reaches loopback-only host units; the dashboard's `zhenai` row needs 20112 carried by the host gateway proxy. | N-Q 8 |
| A-35 | Certificates that name the LAN address: `cert-gen` has no flag for extra names, so every LAN page warns on the name. | N-Q 9, 21; N-todo 13 |
| A-36 | Units for llama-server and the Flask app (`zhen_app.py`), the app under its own nologin account; today both are hand-started and die at reboot. | N-Q 10, 25 |
| A-37 | Evening 0 departures to accept: the ceremony runbook's `luksFormat` guard (and should plan Step 181 carry it); the `psk-gate.sh` hash-pin coupling; the "both hosts" wording of the Wotan SLO in `policy.yaml`. | N-Q 13-15 |
| A-38 | chaos-controller on west: fix `chaos.env` now (Cuirass URL is http, `AUTH_ALLOW_CALLERS` empty, listener on `*:19012` plain HTTP) or at its evening. ADR-136 says its certificate waits on the BIND and CA work. | N-Q 18 |
| A-39 | Migration 029 was applied outside the plan's count: a dated note on the plan, or is the progress record enough. | N-Q 20 |
| A-40 | Log store week: a user timer for the daily reading or once per session; bump Vector so the writer's link can run the Strict profile. | N-Q 30, 32 |
| A-41 | Two red gates on develop: bump the oxlint pin 1.90.0 to 1.91.0; `check-ebpf-size-pin` (946688 against the pinned 918840): find what grew or re-pin. | N-Q 34, 35; PROG S-STAGE-2B:32 |
| A-42 | Slice limits on west: `MemorySwapMax` is infinity, no `MemoryHigh`, `TasksMax` 4096 could be 2048, an optional cgroup alert. `kernel.panic` and the hardware watchdog stay "later" on his word. | N-watchdog gaps 1-5; N-hands 6 |
| A-43 | Sudo: the narrowed rule is parked until the code is on main (his word, 2026-11-10). The two high register rows `sudo-password-required` stay open until then. | N "Answers 2026-11-10"; N-hands 3; REG |
| A-44 | Accept the built Proposed ADRs: 124 (syslog relay, installed), 125 (hop checks), 127 (compliance fold, code behind `--serve`), 135 (reply channel), and 093 "with the DP-506 wording". (103 and 106 are E1 114.) | ADR status lines; DP-603; PROG S-STAGE-2B:20, 41, 107 |
| A-45 | Third-party sitting, which also holds staging: DP-217 (ADR-113's thirteen questions), DP-403 (register thresholds and four modules, including the yaml.v3 swap of FND-030), DP-404 (admissions awaiting a signature), DP-24, DP-713, DP-720, DP-721's later stages; and the 235 undecided rows on `/verdicts`. | DP "Still open"; N-night queue 9; OUT M-162, M-163 |
| A-46 | Staging promotion (DP-601): held until the third-party sitting. Then develop to staging, staging to main, a signed tag. The only plan is `STAGING-LADDER-2026-08-03.md`, which predates everything since. | OUT M-008; DP-601 |
| A-47 | Naming sitting with Lore (DP-304): Shop, the verdicts service, the warden, unheadnix (M-230), and Zhen's new name if he wants it there. | DP-304; OUT M-230 |
| A-48 | Restore-proposal design (Enkrateia restore proposals v2, semantic drift diff): needs a single-file restore command first. | OUT M-202; S-ASCEND-2 plan deferred step |
| A-49 | Operating system: which LTS and when. Both hosts run a release out of support since 2026-07-09; FND-010 and FND-011 wait on the same move. | REG `os-supported` x2 (high), FND-010, FND-011; OUT M-012 |
| A-50 | Register acceptances expire: 3 rows on 2026-12-29, 57 on 2026-12-30, 17 on 2027-02-02. Re-accept, fix, or let lapse; nothing schedules the review. | REG (77 `accepted` blocks) |
| A-51 | Register cleanup he asked to see: 5 of the 10 "remove" entries are still listed (`check-service-auth.sh` gate row, `git-signatures:develop`, and the ruff+bandit, sbom and gosec job rows). | PROG S-HOST-TIDY:43 |
| A-52 | `journald-caps-west` sits in `deploy/ansible/compliance/profile-all.yml` with no host guard: exclude it, guard it, or accept it. Checked: line 21. | PROG S-HOST-TIDY:19 |
| A-53 | Host rows with a question attached: does west need ISP IPv6 at all (`accept_ra`); a wired link for west or accept the wireless radio. | REG `host-sysctl:west:net.ipv6.conf.all.accept_ra=0`, `host-probe:west:radio-wireless-down` |
| A-54 | Relicense veto list: the "Stevie" column of `docs/legal/RELICENSE-CANDIDATES-2026-11-04.md` is empty; `design-system.css` is still BUSL-1.1. | PROG S-SUPPLY-FIRST:12 |
| A-55 | Image bumps waiting on him (postgres 16.15, grafana 11.5.1, prometheus v2.50.1, nginx 1.25.5, haproxy 2.9.15, traefik v3.3.7, vector 0.43.1, alertmanager v0.34.1, qdrant, clickhouse, TEI); coredns 1.12.4 needs a minor move to 1.14.x for nine CVEs; and the rule: keep bumps that raise raw finding counts, or count net. | PROG S-SUPPLY-FIRST:78, 80, 81 |
| A-56 | sophia-eye compose: llama-server must be reachable at `host.docker.internal` but listens on `127.0.0.1:8081` only (a host and firewall change); `sophia-gateway` lists two model names that reach one llama-server. | PROG S-CLEAN-CUT:13, 15 |
| A-57 | Two small rulings: keep `flask-cors==6.0.2` in the pip freeze (it came from the host environment); should the meta-gate's throwaway push in `test-git-mirror.sh` have been a stop. | PROG S-CLEAN-CUT:25, 32 |
| A-58 | Zhen design calls left by S-ZHEN-POLISH: code shard Well-backed or embedded; label the shard `external` like vor; in-root queued writes approvable or run by a daemon-side executor; `-audit-well` on the `zhen-agentd` unit (it writes no `llm_requests` rows today); the 7-day blanking migration for `zhen_pending_confirmations`; refuse Hangul fillers and U+2800 in teach text; `/var/zhen` on west is owned by govan. | PROG S-ZHEN-POLISH:99, 103, 104, 107, 108, 114, 115 |
| A-59 | Doom records (lowest priority): ADR-079 and CLAUDE.md still credit the RET floor with part of the Doom PC corruption; hypothesis H2 (misaligned `LD`) is untested and has no parked ADR. | PROG S-MOVE-OUT:20, 22 |

### A3. UPC and hop-check calls

| ID | Decision | Source |
|---|---|---|
| A-60 | D6 (`UPC_D6_MISS_FAULT`, off in the tree) and the missing privilege check on MRET/SRET: a U-mode `sret` can forge replies and fork notes. Enable D6 and reject both opcodes at priv 3: plan it? | OUT M-247; PROG S-STAGE-2B:36, 42, 101, 102 |
| A-61 | UPC design calls from ADR-135's review: exec does not zero `.bss` (R-NC); LR_W/SC_W/XCHG/CAS skip `translate_address`, an opcode scan is offered (R-S4); sbrk zeroing may need chunking; `open(O_RDWR)` on a regular file stays in BPF and writes go to the TTY (T-F3); a bad `.data` sidecar only warns; the Step 232 stop was not taken; ush has no `cd`; park ADR-135's F7 and F8 ideas as ADRs or drop them. | PROG S-STAGE-2B:34, 38, 47, 48, 53, 81, 87, 94 |
| A-62 | Which of the offered UPC probes to build (unlink with data, link nlink, pause tick count, file past one block, forged replies with D6 on, `hostile_ptr` for unlink, link and chdir, `upc-rung0-assert.awk` in the harness). | PROG S-STAGE-2B:66, 71, 77, 89, 104 |
| A-63 | Hop checks, what the register leaves unruled: which of four packet classes may pass an enforcing hop (FND-033); the flow-0 entry that drops every Shield-born packet when missing (FND-035). FND-031 (no per-packet integrity) stays behind them. | REG FND-031, FND-033, FND-035; PROG S-WIRE-HONEST:27, 31, 33, 103, 104 |
| A-64 | Hop tooling calls: `hop rules apply` trusts `--pin-dir` and the loader dies on SIGTERM before cleanup; `HMAC_KEYS` holds 32 key bytes nothing reads; `readKey` checks no owner, the rules file mode is unchecked, ANOMALY has no rate limit; no way to remove an entry short of a reload; `load_shield` treats pin failures as warnings; SLH-DSA deterministic or hedged; should lanes only ever merge themselves. | PROG S-WIRE-HONEST:29, 30, 105, 106, 107; hop:65, 82, 86, 87; pq:17 |

### A4. Findings whose next move is his

| ID | Finding | Severity | What closes it | Source |
|---|---|---|---|---|
| A-65 | FND-004: west holds a public IPv6 address and nothing has probed it from outside | high | one recorded probe from a vantage point outside the home network | REG |
| A-66 | FND-013 (Dependabot alert 116): torch advisories, pinned 2.10.0, west runs 2.6.0 | low | bump to 2.13.0 or later with an attended ROCm training smoke test; the forge now lives in `unheaded-forge` | REG; N-know |
| A-67 | FND-039: evidence is not sealed | medium | a design: the seal's key custody and what counts as a re-runnable check | REG |
| A-68 | FND-040: `notebooks/requirements.txt` is not audited | low | admit the notebook lock (new packages, each an ADR-096 admission) or drop the notebooks' dependencies | REG; OUT M-167 |

## B. Covered by an existing plan

"Who" is who acts next. Attended means Stevie present; the plan is `S-E1-LIVE-battle-plan.md` (19 sittings, 306 steps) unless named.

| ID | Item | Plan and steps | State | Who | Also closes |
|---|---|---|---|---|---|
| B-01 | Evening 1, paper: send the export email (after A-25), read and ratify retention policy v1, sign three attestations | E1 20-29 | 18-19 done 2026-11-10 | Stevie | OUT M-017, M-074, M-100 |
| B-02 | Evening 2, GitHub, Console, Wi-Fi: secret protection and push protection, protect `main` with `enforce_admins`, enable CI then Docker, eBPF & Rust, Release, revoke the April Console key, drop `ALERT_MAILER` from `.bashrc`, new Wi-Fi passphrase | E1 30-50 | not started | Stevie | FND-027, FND-042, OUT M-016; REG github-job and `slo:ci-develop` rows clear on a green run |
| B-03 | Evening 3, credentials: name the 20 history fingerprints live or dead, Pi-hole admin password, heartbeat token, rewrite of 16 lines, baseline shrink | E1 51-62 | not started as a sitting; 15 service passwords were rotated 2026-11-12 outside the plan. Not rotated: keys, certificates, the sword and cuirass API keys, the SMTP password | attended | FND-041, OUT M-010 |
| B-04 | Evening 5 remainder: DKIM, Workspace app password, sender; DMARC; a real alert end to end; revoke the old Gmail app password; backups green on both hosts; retention timer; accept ADR-103 and ADR-106 | E1 81-83, 102-117 | 84-100 ran as a test cut 2026-11-10 | attended | FND-001, OUT M-011, M-050; `host-probe:west:alert-route` |
| B-05 | Until the mail steps run: renew the 24 h alert silence first thing every session (`maintenance.sh 1440m`); mailer stays `dry-run` | E1 88; N "Off switches" | recurring | Claude | |
| B-06 | Evening 6, east door: east firewall apply with rollback timer, sshd to port 7375; the one-time hand test of east's password | E1 118-135 | not started | attended | REG `east:firewall-inbound-deny` (high), `east:firewall-single-utility`; PROG S-HOST-TIDY:9, 24 |
| B-07 | Evening 7, west firewall | E1 136-150 | not started; needs E1 live and quiet two days | attended | REG `west:firewall-inbound-deny` (high), `ip_forward`, `ipv6 forwarding`, `svc-web-server-not-in-use` |
| B-08 | Evening 8, west sshd: name the `.135` and `.137` devices, hybrid-only key exchange, a laptop key, key-only, fail2ban on the right port | E1 151-166 | not started | attended | OUT M-015; REG `west:sshd-kex-pq`, three `host-sshd` rows; N-night "West accepts password logins"; PROG S-HOST-TIDY:6 |
| B-09 | Evening 9, PSK timer: command-scoped key, gate on east, one live rotation, enable the weekly timer | E1 167-177 | not started | attended | FND-016, OUT M-014 |
| B-10 | Evening 10, custody and signing: YubiKey build-key ceremony, apt repo key and signed publish, Heimdall and Akira keys; approve three runbooks (`build-key-ceremony`, `apt-repo-publish`, `heimdall-enrol-drift-key`) | E1 178-205 | not started; waits on A-23 | Stevie | FND-018, FND-019 (west), FND-026, OUT M-053, M-061, M-062, M-201 |
| B-11 | Evening 10B, east onto current debs and TLS | E1 206-219 | not started; needs B-10 | attended | FND-017, FND-036, FND-019 (east); REG `host-agent:east:baseline-report` |
| B-12 | Evening 11, east as a node (Mímir phases, weights over Wotan, LICH-015 arm) | E1 220-233 | not started; needs B-06, B-11 | attended | OUT M-228; DP-206; 70 deferred steps of S-UPC-WIRE-COMPUTE |
| B-13 | Evening 12, browser xterm end to end and the Lich live local-model run | E1 234-243 | not started | attended | OUT M-231 |
| B-14 | Evening 13, first Zhen nights: his env file, approve `zhen-overnight-abort`, a fake night, a live night capped at $3 | E1 245-257 | 244 and 244a done (`zhen-agentd` runs, scheduler paused, see A-01) | Stevie | OUT M-126 |
| B-15 | Evening 14, Internet-Drafts: upload six `-01` drafts after 2026-11-14 23:59 PST, record URLs and the Track call | E1 261-266 | 258-260 done; waits on A-24 | Stevie | OUT M-147 |
| B-16 | Evening 15, PostgreSQL 18: approve `well-pg18-upgrade`, dump and restore into a new volume | E1 267-275 | not started; needs E1 stable a week and a green drill | attended | FND-015, OUT M-071 |
| B-17 | Evening 16, log store week: run `scripts/logstore-measure.sh` once every session; he reads the week against bar (A) about 2026-11-17; off switch for the loser | E1 282 (daily), 284-288 | 276-283 done; week started 2026-11-10 18:34Z | Claude daily, Stevie at 284 | |
| B-18 | Evening 17, leftovers then a reboot of both hosts: 25.8 west umask, 25.4 `/tmp` noexec with GOTMPDIR, 25.3 `/var` nodev and nosuid, 25.5 purge the X server, register rows | E1 289-306 | not started | attended | OUT M-105 (part); REG `default-umask`, `west:mount-tmp-noexec`, four `mount-var-*` rows, `svc-x-server-not-in-use`; PROG S-HOST-TIDY:37, 45 |
| B-19 | West host file sources into Vector (audit, auth, nginx, fail2ban and the rest) | LH 59-88 (Phase 3) | not started | Claude, runtime checks need the stack | part of 61 `gap` source rows |
| B-20 | Third-party container stdout to the journald driver | LH 89-104 (Phase 4) | not started | Claude | part of 61 `gap` source rows |
| B-21 | Pi-hole to relay to journal to ClickHouse: validate the forward config on the container's rsyslog 8.2312.0, push, `usermod`, restart; then the history import and deletion of the retired rows | LH 150-164, 164a-164f (Phase 6) | 141-148 done 2026-11-12; 150 written, not installed | attended or Stevie; waits on A-32 | four `pihole/*` rows stay `gap`; N-hands 4 |
| B-22 | East ships logs: install the prepared `vector-east` in a host window | LH 165-178 (Phase 7) | not started | Stevie at Step 170 | OUT M-082; N-night "east's logs are not in ClickHouse" |
| B-23 | Network devices send syslog to the relay; point the router at it | LH 179-192 (Phase 8) | blocked on D7, the device list | Stevie | three `blocked:D7` rows |
| B-24 | Prove every source continuously, records, final gate | LH 193-214 (Phases 9-11) | not started; 39 rows still say `blocked:FND-001` though the stack is up | Claude | |
| B-25 | Retire Prometheus as rule evaluator | S-ONE-GRAFANA Step 14 | waits on A-28 (1); Steps 9-13 ran 2026-11-11 though their boxes are unticked | Stevie, then Claude | |
| B-26 | Attended leftovers of S-LOW-FRUIT: apply unit hardening to installed units on west; Vector silent-sink game day; prompt-cache residue probe (DP-720) | S-LOW-FRUIT 31-35, 67-70, 78-83 | not run | attended | |
| B-27 | Doom: docs, units off root, `doom.mbc` provenance, one banding pass | S-DOOM-LAST (9 phases, 104 steps) | forged 2026-10-27, not started. Doom moved to `~/tmp/projects/unheaded-doom` on 2026-11-05, so the plan's paths are stale. Lowest priority | Claude | OUT M-240 to M-246; FND-010 (Doom half) |
| B-28 | Scrape targets still down on purpose: `heimdall` on both hosts until B-10 and B-11 | E1 200, 215 | waits | attended | N-todo 11 |

## C. ADRs with status Proposed, Deferred or In Progress

From `docs/adr/ADR-INDEX.md` at `fe4b4e5e` (41 Proposed, 3 Deferred, 1 In Progress). "Parked" means no plan by design under the plan freeze (ADR-112). Ten more ADRs are Planned (032, 033, 037, 041, 042, 055, 058, 085, 087, 088) and ADR-029 is partially implemented (cross-host ballot pending); they are outside the three statuses asked for.

| ID | ADR | One line | State |
|---|---|---|---|
| C-01 | 009 | Parish boundaries (binding circles) | Deferred to Beta |
| C-02 | 039 | CS precision reference service | In Progress |
| C-03 | 040 | Kubernetes ecosystem strategy | Deferred (ADR-047, decision 43) |
| C-04 | 69420 | BGP Sleipnir and Unheaded OS | Deferred to Q4 2026 |
| C-05 | 056 | pgvector auxiliary corpus sharding | Proposed, gated on WAVE15 H0 |
| C-06 | 057 | Source code indexing for retrieval | Proposed, gated on 056; two open questions in A-58 |
| C-07 | 076 | One source of truth for the topology map | Proposed, re-scoped 2026-10-27; no plan |
| C-08 | 093 | What shape is this thing | Proposed; acceptance in A-44 |
| C-09 | 095 | Slow-roll structural and supply-chain review | Proposed, deferred; see D-24 |
| C-10 | 096 | Dependency gate v2 | Proposed; thresholds wait on A-45 |
| C-11 | 098 | Rolling findings register, audit and enforcing modes | Proposed (rolling); enforcing mode is D-22 |
| C-12 | 103 | Service auth, Ed25519 JWTs | Index says Proposed (design only); the file says implemented; acceptance is E1 114 (D-26) |
| C-13 | 106 | Two brains, one gate | Proposed, built; acceptance is E1 114 |
| C-14 | 108 | MBC cartridges | Index says Proposed; the file says Accepted 2026-11-03 (D-26) |
| C-15 | 110 | Kernel clock from BPF ticks | Index says Proposed; the file says Accepted 2026-11-04 (D-26) |
| C-16 | 113 | Third-party verdicts service | Parked; `/verdicts` read-only page built 2026-11-12; thirteen questions in A-45 |
| C-17 | 114 | User authentication for every web page | Parked; SSO pitch extends it (D-08) |
| C-18 | 115 | Smallest install for a shop owner | Parked |
| C-19 | 116 | Monitor the monitors | Parked, high emphasis |
| C-20 | 118 | Pi-hole twice behind one front | Parked |
| C-21 | 119 | Shield XDP post-quantum fast path | Parked; design done, build after the freeze |
| C-22 | 120 | Estimated energy per LLM request | Proposed; carried by S-ZHEN-POLISH (merged); acceptance not recorded |
| C-23 | 121 | A future WAF | Parked, lowest priority |
| C-24 | 122 | Go embedding path on llama-server | Parked |
| C-25 | 123 | Uniform logs and columns at the sink | Parked, on hold until the Well stores logs (B-17 decides) |
| C-26 | 124 | Native syslog relay | Proposed, built and installed on west; acceptance in A-44 |
| C-27 | 125 | Hop checks move to hop-ebpf | Proposed, built; acceptance in A-44 |
| C-28 | 126 | Private Hop-by-Hop option registry | Parked |
| C-29 | 127 | Fold wire compliance into compliance-evidence | Parked; code landed behind `--serve`; residue in D-29 |
| C-30 | 128 | Fold firewall into Cuirass | Parked |
| C-31 | 129 | Fold QoS into Cuirass | Parked |
| C-32 | 130 | Fold NFV | Parked |
| C-33 | 131 | Fold failover into Cuirass | Parked |
| C-34 | 132 | Fold the load balancer into Cuirass | Parked |
| C-35 | 133 | Fold version translation into monad | Parked |
| C-36 | 134 | Fold keymgmt into unheaded-auth | Parked |
| C-37 | 135 | Owned kernel's reply channel to BPF | Proposed, built (S-STAGE-2B); acceptance in A-44 |
| C-38 | 136 | BIND zones and a locked-down CA | Parked; BIND is third party; chaos-controller's certificate waits on it |
| C-39 | 137 | Doom Range port check and dashboard links move to `uhl` | Parked |
| C-40 | 139 | One administration page (users, roles, resets, mail) | Parked, far future; Kanban "contact the owner" waits on it |
| C-41 | 140 | Notifications and messages across the pages | Parked, far future |
| C-42 | 141 | Chaos controller always on, after 1.0.0 | Parked |
| C-43 | 142 | Wotan fast and lean enough to stand beside Redis | Parked |
| C-44 | 143 | Terraform multi-cloud template | Parked; empty skeleton repo only |
| C-45 | 144 | Footprint: measure, then shrink the hungriest | Parked; its trace collector item is a defect (A-22, D-17) |

## D. Open with NO plan (agent-doable)

"After" names the A row that must be answered first. Rows with no "After" can be planned now.

### D1. Web pages and Zhen

| ID | Item | Source | After |
|---|---|---|---|
| D-01 | Close `/api/v1/highlight` behind the login, drop `session_id` from `/health`; the CSRF test suite uses the open route, so the gate's tests change too. | N-night queue 8 | A-16 |
| D-02 | Answers through the daemon show `0 tok` (the API returns no token count on that path); add the CCNA and JNCIA questions to the Zhen quality gate. | N-todo 16 "Left" | |
| D-03 | A Zhen login lasts 15 minutes; before login the page shows `?` for sheets and categories. | N "Zhen login" found (i), (ii); N-todo 17 | |
| D-04 | `raft/start-zhen.sh -gated` starts `./bin/zhen-agentd` (built 2026-10-29), not `bin/release`; its stop hint prints `pkill -f`, the known west trap. Checked: lines 107 and 260. | N "Zhen login" found (iii) | |
| D-05 | Start-up hardening: units for llama-server and the Zhen app (both `-E` GPU values needed); `zhen-agentd` gives up after five quick failures when compose or `wg0` is late; the Zhen front exits when its LAN address is missing. | N-night queue 1, 8; N-todo 13 | A-36 |
| D-06 | Source viewer: an unknown id shows the nearest sheet instead of a 404 (fix in vor or in `view_source()`); Pygments still runs on code from untrusted sheets. | N-todo 20 | |
| D-07 | Compliance page reads from the Well: the dashboard service has no `WELL_OPS_PASSWORD`, so all three calls answer from files. Checked: the variable is absent from `docker-compose.yml`. | N-todo 18 | A-27 |
| D-08 | Write the parked ADR for single sign-on (extends ADR-114): one login across our pages, an outside OIDC provider as an option, the users file as default and break-glass. | N-todo 14 | |

### D2. Wiki and vor

| ID | Item | Source | After |
|---|---|---|---|
| D-09 | Wiki link defects: four sheets share a name with another topic and 404 from search; links inside a sheet are not rewritten; about 136 dead `[x](Page.md)` links; bare `..` and image links. | N-todo 22 "Noticed" | |
| D-10 | Move the five hand-written vor callers (`zhen-mcp`, `zhen-rag`, `zhen-cli`, `zhen-agent`, `zhen-agentd`) onto `pkg/vorclient` (checked: only `wiki-server` imports it); `cmd/alert-policy/main.go:43` keeps its own copy of the GitHub blob address. | N-todo 22 | |
| D-11 | In vor (its own repo): exact id or 404 for an unknown topic; word-boundary matching in search. | N-todo 16, 20, 22 | |
| D-12 | vor's copy of the repository is a snapshot (last export 2026-11-12 at `0efd63e6`): nothing reruns `scripts/vor-export-source.sh` after doc changes. | N-todo 22; N-night queue 5 | |

### D3. Monitoring

| ID | Item | Source | After |
|---|---|---|---|
| D-13 | Scrape jobs and alerts for things not deployed: `sword` is still a down target; `ZhenSchedulerStalled` cannot tell absent from stalled. | N-todo 11 | |
| D-14 | Dashboards: `wotan_idempotency_entries` and `wotan_idempotency_hits_total` are not emitted (two empty panels); `09_compliance` shows 0 of 8 until `cmd/unheaded-baseline` emits. Cut-over checks the agent did not run: `tests/scripts/test_image_digests.py`, `check-prometheus-rules.sh`, gitleaks. | N-todo 19 | |
| D-15 | Gates do not read some compose files (`haproxy:2.9.15-alpine` x2, `unheaded-sword`); an untracked `docker-compose.override.yml` sits in the main checkout; `scripts/bootstrap-llm-lab.sh` pulls `:latest`; `lxd/telemetry` downloads archives. | N-todo 19 "Found on the way" | |
| D-16 | Wotan refusal alert and its runbook. | N-night queue 6 | A-18 |
| D-17 | Trace collector fix: batch the map reads and slow the sweep, bound `LATENCY_MAP`, or push one event per measurement; fix the silent insert failure at 65,536 entries; stale "per-flow" comments in latency-probe. | ADR-144; PROG S-WIRE-HONEST-lat:21, 29, 30 | A-22 |
| D-18 | IOC audit: Vector counters for account changes, sudo command lines, cron edits and kernel messages. East cannot be audited until B-22. | N-night 4 | A-17 |

### D4. Compliance and security

| ID | Item | Source | After |
|---|---|---|---|
| D-19 | Rename `~/.config/cs` to `~/.config/vor` in the READMEs, `raft/`, and the prompt text with its golden files. | N-night queue 5 | A-19 |
| D-20 | FND-037 (high): add the host-side evidence kind, so controls read what runs and not the compose file. East must be redeployed first (B-11). | REG FND-037 | |
| D-21 | AIDE on both hosts with its daily timer; auditd rules on west (move west to the lab-hardened profile); journal the lab fixes on east through `unheaded-remediate`. Host changes, so attended. No S-E1-LIVE step covers them. | REG `aide-installed` x2 (medium), `aide-timer` x2, `west:auditd-rules-loaded` (medium), `host-journal:east:apply-journal`; PROG S-HOST-TIDY:34, 35, 41, 42 | |
| D-22 | Enforcing mode (ADR-098): typed enforce actions through the ADR-099 executor, a flapping breaker, a preinst gate, a 7-day soak, a Lich campaign against the baseline agent. DP-10 answered "refuse installs only". | OUT M-106; DP-10 | |
| D-23 | Live legs never run: `anomaly.scores` over Wotan, the Sophia registry backend; retrain the scorer on steel-echo flows (DP-504). Blocked today: no real flow capture. | OUT M-229; DP-504 | |
| D-34 | Per the export audit: the drafted notice omits the zhend hybrid with AES-GCM, the KEM tunnel and a hand-written SHA-256 (`crates/upc-api/src/surface_hash.rs`); `compliance/crypto/cbom.cdx.json` has no row for ChaCha20-Poly1305, HKDF, AES-GCM, x25519-dalek or BLAKE3. Not determined there: private images on GHCR, the date the repository became public, six other public repositories. | N-night queue 13 | A-25 for the notice text |
| D-24 | ADR-090 total source sweep: 2,192 rows of `docs/sweeps/TOTAL-SOURCE-SWEEP.tsv` are still PENDING; ADR-095's slow-roll review has not started. | OUT M-164; TSV counts | |

### D5. UPC

| ID | Item | Source | After |
|---|---|---|---|
| D-25 | Forge the plan for EVOLUTION-1 Stage 2 rungs 3 to 7: console write, console read, fstat/dup/close with the fs_walk hand-off, fork/exec/wait hand-off. Done so far: rungs 0-2 and all of rung 8 (sbrk, kill, mkdir, unlink, link, sleep, chdir, pipe) plus the fd table. ush pipelines wait on rung 6. | PROG S-STAGE-2B:99, 116, 123; OUT M-221, M-226 | |
| D-26 | Index and record drift: ADR-INDEX rows for 103, 108 and 110 disagree with the ADR files; timeline mirrors `references/timeline.{json,yaml,toml}` lack S-STAGE-2B and have no sync command. | ADR files; PROG S-STAGE-2B:122 | |
| D-27 | UPC hardening left open by ADR-135's review: BPF `open` and `exec` read user paths with no bound (R-F5); the fork tagged-cursor fix, I9 tripwire and slot reservation (R-F3, R-F6); slot 0's only bound is the 8 MiB ceiling; `write()` from a bad buffer on a pipe returns 0, not -1. | PROG S-STAGE-2B:43, 44, 57, 99 | A-60 for the D6 parts |

### D6. Residue of the 2026-11-04 sprints (small, mechanical)

| ID | Item | Source | After |
|---|---|---|---|
| D-28 | Docs that no longer match the tree: `wiki/Microservices.md` calls the Go `shield` a WAF; `wiki/Systemd-Units.md` has no `unheaded-vor.service` row; `unheaded hop` is missing from shell completions and the wiki; `BPF_IPV6_INTERFACE_MAP.md` tables are stale; circl v1.6.3 named where go.mod has v1.6.5; `hqc.go` says "ROADMAP (Q3 2026)"; three orphaned `cmd/waf` lines in `docs/architecture/PROJECT_STRUCTURE.md`; "vLLM/ROCm" text in `nixos/`; stale container counts in `lxd/`; `cmd/zhen-mcp/main.go` says "Six read-only tools"; `man/zhen-cli.1` carries a `+dirty` stamp; the `unheaded-vector` sysusers comment says "(east)"; `scripts/east-bootstrap.sh` names `root@`. | N "WAF archived"; PROG S-ZHEN-POLISH:59, 109, 118; S-WIRE-HONEST-hop:75, 81, 83, 88; pq:18, 19; S-CLEAN-CUT:9, 11, 23; S-HOST-TIDY:8, 47 | |
| D-29 | Compliance fold (ADR-127) residue: a failed ring consumer prints one line and keeps serving `no_data`, its goroutine is not joined at shutdown; `no_data` is missing from `/metrics` and `Stats()` and is recomputed only on the aggregation tick; a help text still says "unavailable Wotan"; allow lists and `auto_approve` when `wire` gets its own account. | PROG S-G9-FOLD:90, 94, 96, 97, 98 | |
| D-30 | Zhen code residue: `AppendTurns` never tested on Postgres; `TTYConfirmer` prompts for queue-only `teach_write`; a shard with `CURRENT` but no slot file reads as no hits, not an error; `CodeBackend` ignores its context. | PROG S-ZHEN-POLISH:110, 113, 116, 117 | |
| D-31 | Deploy file residue: `tomb/ansible/playbook-grimoire.yml` deploys `files/grimoire-search.sh`, which does not exist, under `failed_when: false`; `trace-collector.nix` sets two variables the Go binary never reads; `sophia-eye.nix` keeps a dead vLLM variable and runs embeddings-bge as root; `shield-rules.yaml` has no egress rule for llama-server (after A-56). | PROG S-SUPPLY-FIRST:15; S-CLEAN-CUT:12, 14, 22 | |
| D-32 | Housekeeping: `GRAFANA_ADMIN_PASSWORD` in `monitoring.env` is read by nothing; `zhen_login_test.sh.obsolete-2026-11-12` remains; the wiki unit's `After=docker.service` fix lives only in a host drop-in and is proven only at the next boot; the `s-clean-cut` worktree was never removed (its Step 128); ruff and pip-audit are not on PATH on west, so a plain meta-gate run is INCONCLUSIVE; ruff I001 at `scripts/lib/sbom_licenses.py:18`; `check-perf-regression` exits 2 on usage. | N-night 2, queue 2, 3; N "Evening 16" left alone; N-todo 20; PROG S-CLEAN-CUT:38; S-MOVE-OUT:25 | |
| D-33 | Kanban "Request changes" tells no one: contacting the owner needs users and mail (ADR-139, parked). Listed so the gap is visible; no work until the ADR opens. | N-state "Kanban Request changes" | C-40 |

## E. Status unclear

| ID | Item | Source one says | Source two says |
|---|---|---|---|
| E-01 | FND-043 (medium), Wotan per-topic permissions | N-state: fixed and enforced 2026-11-12 (`24025056`), gaps closed (`52f79a9a`) | REG: entry has no `resolved` block |
| E-02 | FND-025 (medium), hop checks prove key presence only | REG FND-025: closes on a root run of `tests/hopchecks` at HEAD | REG FND-033 text: that run passed 2026-11-09 (46 pass); FND-025 still listed |
| E-03 | FND-009 (low), WireGuard container needs root | REG: "stays listed", no next action written | No acceptance block and no plan; open by design or awaiting an acceptance |
| E-04 | `host-probe:west:alert-route` (high), `host-probe:west:db-backup-recent` (medium) | REG: clear when E1 brings the stack up | N: stack up since 2026-11-10, backup succeeded 2026-11-12, mail leg not run; rows still listed |
| E-05 | Six SLO rows (`slo:dashboard`, `log-shipping`, `well`, `well-backups`, `wotan`, `ci-develop`, all medium) | REG: "re-read after the auth/TLS rollout settles" | No step in any plan re-reads them |
| E-06 | CI rows: `github-job` x4, `git-signatures:develop`, gate row `check-service-auth.sh` (high) | REG: fixed on develop, close on the next push or green run | PROG S-HOST-TIDY:43: five were marked "remove" and are still listed; CI is disabled on GitHub until B-02 |
| E-07 | 26 host rows with no plan step found. 18 mount rows: `/dev/shm` noexec (both hosts), east `/tmp` noexec, west `/home` nodev and nosuid, west `/var/log`, `/var/log/audit` and `/var/tmp` options (9), west separate filesystems (4). Then ftp and telnet clients (4), `var-log-perms` x2, `dot-files`, `root-path` | REG: each has a fix written, none accepted | DP-25 "all thirteen rows as recommended" and S-HOST-TIDY applied "safe rows"; the row-to-step map was not found |
| E-08 | S-E1-LIVE Evenings 4 and 5 (Steps 63-80, 84-100) | E1-prog: run 2026-11-10 as a test cut on unrotated secrets | Plan boxes unticked; nothing says whether the cut counts or the evenings rerun after B-03 |
| E-09 | Migrations 047, 049, 050 | N-hands 5 lists them as evening work | N "kingdom is UP": 52 applied, 0 pending; S-NIGHT-SHIFT progress: 050 never written |
| E-10 | Live proofs promised by the 2026-10-27 sprints, no later closure found and no S-E1-LIVE step: Akira cert-expiry rotation proof (M-054), ci-status timer (M-087; no such timer on west today), live perf numbers and hypotheses H1-H9 (M-089, M-090), mandatory Sophia dictionary signatures (M-149), Akira cross-host ballot (M-200, ADR-029), staging-merge deploy to east and the signing timer of DP-18 (M-063), ScanCode re-run (M-169, stuck on an image pull) | OUT: "code merged, live proof pending" or "stuck" | No later document closes or re-plans them |
| E-11 | Likely closed, not verified: per-topic publish ACLs and the whole-stack run (M-052), deploy history and chaos journal in the Well (M-072), SLO definitions synced to the Well (M-077), ADR-019 L2 amendment (M-125), Kanban Runbooks view live (M-128) | OUT: live step pending after migrations | N: migrations applied 2026-11-10, FND-043 enforced, images rebuilt; none names these items |
| E-12 | Answered in the decision pack, no plan step found: clap for the Rust binaries (DP-406), snapshots in the debs (DP-707), edge certificates through orion (DP-702, "back burner") | DP answers of 2026-11-03/04 | No plan in the 2026-11-04 series cites the id |
| E-13 | Parked groups with no ADR row: Sabatons IPMI/Redfish, `tools/` extractions, PQC parked ideas, Sleipnir Phase 1, Yggdrasil #65, repo split and multi-tenant wishlist | OUT section 4 | DP-20 re-deferred them; nothing since |
| E-14 | East host state: `aide-install` and `apparmor-enforce` reported not applied | PROG S-HOST-TIDY:34, 35, 41, 42 | East was not contacted by this sweep; REG lists east AppArmor as accepted to 2027-02-02 |
| E-15 | `golang.org/x/crypto` on an untagged pseudo-version; `max_hop_count` wording ("at or under") against ruling 38's countdown | PROG S-SUPPLY-FIRST:14; S-WIRE-HONEST:102 | No network check was made; ADR-125 line 29 and the `-01` draft were not reconciled |
| E-16 | Unticked boxes in older plans: S-IRON-GATE 72, S-WELL-SPRING 70, S-TRUE-WIRE 58, S-UPC-WIRE-COMPUTE 72, S-ASCEND-2 44, S-CHAMPION-HARDENING 22, S-CLEAN-SLATE 9, COMPLIANCE-WELL 43, UPC-LINUX-MASTER 7, STAGING-LADDER 143 | Plans: steps gated on escalations or live windows | NEXT-7 index and DP: escalations answered and rolled into the 2026-11-04 series and S-E1-LIVE; not mapped step by step here |

## Proposed bundles for group D

Names follow the existing `S-` convention. Each line is a goal and its rows, not a plan.

| # | Name | One-line goal | Rows |
|---|---|---|---|
| 1 | S-ZHEN-FRONT | The Zhen page and its units start, log in and answer the same way after a reboot, with no route open before login | D-01 to D-07 (D-01, D-05, D-07 after A-16, A-36, A-27) |
| 2 | S-WIKI-VOR | One vor client, exact topic lookups, and wiki links that resolve | D-09 to D-12 |
| 3 | S-WATCH-TRUE | Every scrape job, alert and panel matches something that runs; the trace collector stops burning a core | D-13 to D-18 (D-16, D-17, D-18 after A-18, A-22, A-17) |
| 4 | S-EVIDENCE-RUNS | Compliance reads what runs on the hosts, the register and the crypto inventory match the evidence, and enforcing mode has a path | D-20, D-21, D-22, D-34; settles E-01 to E-07 on the way; attended for the host steps |
| 5 | S-STAGE-2C | Stage 2 rungs 3 to 7 in the owned kernel, with the review's hardening items | D-25, D-27 (plus A-60 to A-62 once answered) |
| 6 | S-RESIDUE | Clear the small leftovers of the 2026-11-04 sprints and the record drift in one pass | D-19 (after A-19), D-26, D-28 to D-32 |
| 7 | (no plan) | Leave unbundled: D-08 is one ADR to write; D-23 waits on real flow capture; D-24 is a standing background lane, not a sprint; D-33 waits on ADR-139 | D-08, D-23, D-24, D-33 |
| 8 | S-PIHOLE-DNS (attended, rows are in A and B) | Pi-hole serves IPv6, forwards its logs, and carries the lists he chose | A-20, A-21, A-32, B-21 |

Order suggestion, oldest backlog first: 6 and 3 can start with no answer from him; 1 and 2 need A-16; 4 needs east (B-11); 5 follows the UPC answers.

## Appendix A. Plan status

| Plan | State on 2026-11-12 |
|---|---|
| S-E1-LIVE | In progress. Done: Evening 0, Steps 18-19, 63-80 and 84-100 (test cut), 244, 244a, 258-260, 276-283. Left: rows B-01 to B-18 |
| S-LOG-HARVEST | In progress. Done: Steps 1-58 (Phases 0-2), 105-148 (Phase 5 and the relay install). Left: rows B-19 to B-24 |
| S-ONE-GRAFANA | Done but Step 14 (B-25) |
| S-CLEAN-CUT, S-MOVE-OUT, S-SUPPLY-FIRST, S-ZHEN-POLISH, S-WIRE-HONEST, S-G9-FOLD, S-STAGE-2B, S-HOST-TIDY | Merged 2026-11-04/05; boxes in the plan files were never ticked (progress files hold the record). Residue is in A-12 to A-14, A-51 to A-64, D-25 to D-32 |
| S-ANSWERED, S-NIGHT-SHIFT | Done 2026-11-07 and 2026-11-09 |
| S-CLEAN-SLATE, S-IRON-GATE, S-WELL-SPRING, S-CHAMPION-HARDENING, S-TRUE-WIRE, S-ASCEND-2 (NEXT-7, 2026-10-27) | Code complete or shipped; live and escalation steps rolled into the decision pack and S-E1-LIVE (E-16) |
| S-DOOM-LAST (NEXT-7 sprint 7) | Not started (B-27) |
| S-QUANTUM-ARMOR, S-TWO-BRAINS, S-REAL-STEEL, S-LICH-AWAKENS, S-UPC-WIRE-COMPUTE (NEXT-5, 2026-10-23) | Done 2026-10-26 per the index; LoRA phase deferred on data and since moved to `unheaded-forge`; east steps are B-12 |
| S-LOW-FRUIT | Done but its attended legs (B-26) |
| EVOLUTION-1-STAGE-2 | 160 of 160 ticked; continued by S-STAGE-2B; next rungs have no plan (D-25) |
| STAGING-LADDER-2026-08-03 | Stale; promotion is held (A-46) |
| `battle-plan.md`, WAVE and PHASE plans, round tables | Historical (April to July 2026); not swept row by row |

## Appendix B. OUTSTANDING-2026-10-27 M-numbers not marked closed there

| M-number | Where it is now |
|---|---|
| M-008 | A-46 |
| M-010 | B-03 |
| M-011 | B-04 |
| M-012 | A-49 |
| M-014 | B-09 |
| M-015 | B-08 |
| M-016 | B-02 |
| M-017 | A-25, B-01 |
| M-050, M-051 | B-04, B-11 |
| M-052, M-072, M-077, M-125, M-128 | E-11 |
| M-053, M-061, M-062, M-201 | B-10 |
| M-054, M-063, M-087, M-089, M-090, M-149, M-169, M-200 | E-10 |
| M-071 | B-16 |
| M-074, M-100 | B-01 (ratify, sign), B-04 (retention timer) |
| M-082 | B-22 |
| M-105 | B-18, D-21, E-07 |
| M-106 | D-22 |
| M-120 | Approvals case by case (DP-301): B-10, B-14, B-16, A-01, A-02 |
| M-126 | B-14 |
| M-147 | B-15 |
| M-148 | A-44, B-04 |
| M-162, M-163 | A-45 |
| M-164 | D-24 |
| M-167 | A-68 |
| M-180 | A-63, E-02 |
| M-186 | C-29 to C-36 |
| M-202 | A-48 |
| M-221, M-226 | D-25 |
| M-228 | B-12 |
| M-229 | D-23 |
| M-230 | A-47 |
| M-231 | B-13 |
| M-240 to M-246 | B-27 |
| M-247 | A-60 |
| Treated as closed by later work, not listed as rows | M-055 (logins exercised on west 2026-11-10 and 2026-11-12), M-067 (DP-713), M-070 (52 migrations applied 2026-11-10), M-080 (east heartbeat lands; alert renamed `e10ae2e0`), M-107 (syntax check is in `static-analysis.yml`; the roles ride ADR-088), M-130 and M-131 (units installed 2026-11-10 and 2026-11-12), M-140 to M-146, M-160, M-165, M-166, M-168, M-181 to M-185, M-187, M-188, M-204, M-220, M-222 to M-225, M-227 (marked DONE in the sweep; their sprints merged), M-248 (DP-415; meta-gate split into shards 2026-11-06) |

Section 5 of that sweep (21 doc fixes) maps to S-CLEAN-SLATE Phase 6, Steps 221-260, all ticked; not re-verified.

## Appendix C. Findings register, every open entry

FND ids (25 open; FND-032 and FND-038 carry `resolved` blocks):

| FND | Severity | Title | Row |
|---|---|---|---|
| FND-001 | high | Alert delivery to Stevie is not proven end to end on west | B-04 |
| FND-004 | high | West's public IPv6 address was never probed from outside | A-65 |
| FND-009 | low | WireGuard compose service is not least-privilege | E-03 |
| FND-010 | medium | Units that still start as root (Well backup jobs; the Doom half moved out) | A-49 |
| FND-011 | high | Two KEV kernel vulnerabilities unpatched on both hosts; stopgap applied | A-49 |
| FND-013 | low | PyTorch advisories | A-66 |
| FND-015 | medium | The Well's TLS key exchange is classical | B-16 |
| FND-016 | low | WireGuard PSK timer not enabled | B-09 |
| FND-017 | medium | Internal TLS declared and unproven | B-11 |
| FND-018 | low | No production evidence build key | B-10 |
| FND-019 | low | Heimdall's signed drift alerts reach no one | B-10, B-11 |
| FND-025 | medium | Hop checks test key presence, not a tag | E-02 |
| FND-026 | medium | West's apt repo is the old unsigned one | B-10 |
| FND-027 | medium | Main CI workflow disabled on GitHub | B-02 |
| FND-030 | low | `gopkg.in/yaml.v3` is archived upstream | A-45 |
| FND-031 | low | No per-packet integrity on the Monad | A-63 |
| FND-033 | low | hop-ebpf enforce mode does not cover every packet | A-63 |
| FND-035 | low | One missing flow-0 entry drops every Shield-born packet | A-63 |
| FND-036 | medium | East's wotan and timeguru units restart in a loop (disabled since 2026-11-10) | B-11 |
| FND-037 | high | Evidence reads declared configuration, not what runs | D-20 |
| FND-039 | medium | Evidence is not sealed | A-67 |
| FND-040 | low | `notebooks/requirements.txt` not audited | A-68 |
| FND-041 | medium | Public repository, credentials in history not yet rotated | B-03 |
| FND-042 | medium | GitHub branch protection is off | B-02 |
| FND-043 | medium | Wotan had no per-topic read permission | E-01 |

Evidence-source rows (144; the id is the title):

| Family | Severity | Count | Ids | Accepted until | Row |
|---|---|---|---|---|---|
| `host-probe` firewall | high | 2 | `east:firewall-inbound-deny`, `west:firewall-inbound-deny` | no | B-06, B-07 |
| `host-probe` os | high | 2 | `west:os-supported`, `east:os-supported` | no | A-49 |
| `host-probe` sudo | high | 2 | `west:sudo-password-required`, `east:sudo-password-required` | no | A-43 |
| `host-probe` service | high | 1 | `west:alert-route` | no | E-04 |
| `gate-script` | high | 1 | `scripts/compliance/check-service-auth.sh` | no | E-06 |
| `slo` | medium | 6 | `dashboard`, `log-shipping`, `well`, `well-backups`, `wotan`, `ci-develop` | no | E-05 |
| `host-probe` package | medium | 2 | `west:aide-installed`, `east:aide-installed` | no | D-21 |
| `host-probe` service, backup | medium | 2 | `west:auditd-rules-loaded`, `west:db-backup-recent` | no | D-21, E-04 |
| `github-job` | medium | 1 | `Static Analysis/Gates can actually fail` | no | E-06 |
| `host-sshd` | medium | 3 | `east:passwordauthentication=no`, `east:kbdinteractiveauthentication=no`, `west:kbdinteractiveauthentication=no` | 2026-12-30 | B-06, B-08, A-50 |
| `gate-script` | medium | 1 | `scripts/compliance/check-edge-tls.sh` | 2026-12-29 | A-50, E-12 |
| `host-sysctl` | low | 3 | `west:net.ipv6.conf.all.accept_ra=0`, `west:net.ipv4.ip_forward=0`, `west:net.ipv6.conf.all.forwarding=0` | no | A-53, B-07 |
| `host-probe` sshd | low | 1 | `west:sshd-kex-pq` | no | B-08 |
| `host-probe` mount | low | 23 | east: `dev-shm-noexec`, `tmp-noexec`, `var-nodev`, `var-nosuid`; west: `dev-shm-noexec`, `home-nodev`, `home-nosuid`, `home-separate`, `tmp-noexec`, `var-log-audit-{nodev,noexec,nosuid,separate}`, `var-log-{nodev,noexec,nosuid,separate}`, `var-nodev`, `var-nosuid`, `var-tmp-{nodev,noexec,nosuid,separate}` | no | B-18 (five rows), E-07 (the rest) |
| `host-probe` mount | low | 15 | east: `home-{nodev,nosuid,separate}`, `var-log-audit-{nodev,noexec,nosuid,separate}`, `var-log-{nodev,noexec,nosuid,separate}`, `var-tmp-{nodev,noexec,nosuid,separate}` | 2027-02-02 | A-50 |
| `host-probe` package, service | low | 6 | `west:pkg-ftp-client-absent`, `west:pkg-telnet-client-absent`, `east:pkg-ftp-client-absent`, `east:pkg-telnet-client-absent`, `west:svc-x-server-not-in-use`, `west:svc-web-server-not-in-use` | no | E-07 (four), B-18, B-07 |
| `host-probe` package | low | 2 | `west:aide-timer`, `east:aide-timer` | no | D-21 |
| `host-probe` fileperm, shell | low | 5 | `west:var-log-perms`, `east:var-log-perms`, `west:dot-files`, `west:root-path`, `west:default-umask` | no | E-07 (four), B-18 |
| `host-probe` account | low | 1 | `west:radio-wireless-down` | no | A-53 |
| `host-probe` firewall | low | 1 | `east:firewall-single-utility` | no | B-06 |
| `host-journal` | low | 1 | `east:apply-journal` | no | D-21 |
| `host-agent` | low | 1 | `east:baseline-report` | no | B-11 |
| `git-signatures`, `github-job` | low | 4 | `git-signatures:develop`, `Static Analysis/Python (ruff + bandit)`, `SBOM + Vulnerability Scan/sbom`, `Security Scan (Daily + PR)/Go Security Scan (gosec)` | no | E-06 |
| `host-probe` account (PAM, password ageing) | low | 38 | `pw-max-days`, `pw-inactive`, 3 `pam-faillock-*`, 8 `pam-pwq*`, 4 `pam-pwhistory-*`, 2 `pam-unix-*`, each on west and east | 2026-12-30 | A-50 |
| `host-probe` sshd, shell, boot, sudo, fileperm, package | low | 15 | `sshd-macs`, `sshd-maxstartups` (west); `sshd-banner`, `sshd-access`, `shell-tmout`, `grub-password`, `sudo-logfile`, `pkg-pam-pwquality` (both hosts); `west:perm-sshd-config` | 2026-12-30 | A-50 |
| `host-sshd` | low | 3 | `west:logingracetime<=60`, `west:disableforwarding=yes`, `east:disableforwarding=yes` | 2026-12-30 and 2026-12-29 | A-50 |
| `host-probe` mac | low | 2 | `west:apparmor-profiles-enforced`, `east:apparmor-profiles-enforced` | 2027-02-02 | A-50, E-14 |

## Appendix D. Not read, and limits

- `~/tmp/next-archive-2026-11-09.md` (376 KB): grepped by the sub-agents for closures, not read whole. `next.md` says its own page wins.
- `/var/tmp/unheaded-sprint/s-answered-wave1-forstevie.md` and `wave2`: used by the sub-agents as closure sources only.
- `docs/sweeps/outstanding-2026-10-27/` (six raw sweeps), `REAL-STEEL-DECISIONS.md`, `REAL-STEEL-UNREACHED.tsv`, `TRUE-WIRE-DECISIONS.md`: not read; their consolidated forms (OUTSTANDING, the decision pack) were.
- Plans over 1,000 lines were read by headings, step titles and box counts, not line by line. Progress files for the 2026-10-23 and 2026-10-27 sprints were not read.
- East was not contacted. No gate was run; gate states are as the sources report them.
- `~/tmp/secrets-2026-11.md` and every `.env` were not opened.
- The repository changed during the sweep (`8d7f5ad6`, `fe4b4e5e`). Anything after `next.md` 2026-11-12 07:20Z is not reflected. The export audit itself was read only as far as its summary and the `next.md` note about it.
