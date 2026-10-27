<!--
SPDX-License-Identifier: GPL-3.0-or-later
Copyright (c) 2024-2026 Stevie Bellis.
-->

# MASTER OUTSTANDING — consolidated from six doc sweeps (develop a7fe3acd, 2026-10-27)

Sources: ADR = sweep-adr.md, PLANS = sweep-plans.md, SEC = sweep-security.md, PROTO = sweep-protocol.md, UPC = sweep-upc.md, INT = sweep-internal.md. Every M-item was verified against code at a7fe3acd by its sweep and re-checked here (§3). Status: P = PARTIAL, NS = NOT STARTED. Stevie column: N, or Y + the question. Size S/M/L/XL.
Priority order (Stevie 2026-10-27): (a) CI green + promotion readiness → (b) dated security findings + public-repo implications → (c) live-service correctness → (d) platform epics → (e) UPC non-Doom → (f) Doom LAST.

## 1. Epics (priority order)

### E01 — CI GREEN ON DEVELOP, PROMOTION READY  (tier a, size S, 8 items)
| ID | Item | Sources | Doc | St | Stevie | Sz |
|---|---|---|---|---|---|---|
| M-001 | Dependency baseline lists 4 Go modules gone from the build (fsnotify, kr/pty, gobreaker, x/term): `check-dependency-baseline.sh --update` (shrink-only) | parent repro; SEC untriaged gitleaks job | docs/policy/dependency-baseline.txt | P | N | S |
| M-002 | `scripts/lib/tool-pins.sh`: syft 1.54.1→1.55.0, grype 0.120.1→0.121.0 + sha256 (the "Pinned tools" exit 2) | parent repro | scripts/lib/tool-pins.sh | NS | N | S |
| M-003 | Runbook pytest step fails on GitHub only (passes local venv, CI-pinned venv on py3.13, depth-1 clone); make the step self-reporting (`-rA`, `$GITHUB_STEP_SUMMARY`) and reproduce with Python 3.12 | parent repro | .github/workflows/static-analysis.yml:215-221 | P | Y: pull the job log for run 40426721063 (needs auth) | S |
| M-004 | "Gates can actually fail" cancelled at 25 min every run; measure and set timeout-minutes, or split the provocation into two jobs | PLANS SPRINT-AUDIT; next.md S21 | static-analysis.yml:276 | P | N | S |
| M-005 | Nightly drift-detect (terragrunt-action) fails 5/5 on main: delete or self-host | next.md S13 | .github/workflows/drift-detect.yml | NS | Y: delete, dispatch-only, or self-hosted runner? | S |
| M-006 | Register the gitleaks-job failure as a finding or close with evidence after M-001 (untriaged row in ADR-098) | SEC untriaged | compliance/findings/register.yaml | NS | N | S |
| M-007 | modernc.org/sqlite 1.60.1→1.61.0 after 2026-10-28 cooldown | next.md S16 | go.mod:44 | NS | N | S |
| M-008 | develop→staging promotion backlog (1062 commits since 2026-09-24); then staging→main first promotion + signed tag, GitHub default branch develop + protect main | ADR S-15/S-16; PLANS B-23/B-18 | STAGING-PROMOTION-LOG.md | P | Y: promote on your word; GitHub settings are yours | M |

### E02 — PUBLIC-REPO SECURITY POSTURE + DATED FINDINGS  (tier b, size M, 12 items)
| ID | Item | Sources | Doc | St | Stevie | Sz |
|---|---|---|---|---|---|---|
| M-010 | Rotate every baselined credential (20 history fingerprints, 25 .gitleaksignore): repo IS public, PRE-PUBLIC "lab-only" exemption void; then shrink baseline | SEC SP-01, CG-17; PLANS D-01 | docs/security/PRE-PUBLIC-BLOCKERS.md | NS | Y: rotation is yours (OPNsense API, Grafana, LXD trust, tokens) | M |
| M-011 | FND-001 (high, due 10-30): alert-mailer dry-run → `mode send`; SMTP relay + password file; then `alert-route` host probe | SEC FND-001, CG-02 | ADR-098:276; wiki/Alerting.md | P | Y: Gmail account + app password | S |
| M-012 | FND-011 (high, due 11-18): add ebtables/ebtable_nat/ebt_snat `install /bin/false` to fixes.yaml + apply both hosts; decide west userns vs LXD; kernel+reboot when Ubuntu ships | SEC FND-011 | compliance/remediation/fixes.yaml:78-88 | NS | Y: host change; LXD userns test | S |
| M-013 | FND-014: live Well role password rotation + `pg_stat_statements_reset()` + old PG logs | SEC FND-014; PLANS F-42 | ADR-098:281 | P | Y: live DB window | S |
| M-014 | FND-016 / E3: first live WireGuard PSK `--apply` rotation on west, remove east stale key, enable weekly timer | SEC FND-016; ADR tracked E3 | deploy/systemd/unheaded-wg-psk-rotate.timer | NS | Y: E3 go | S |
| M-015 | E2: sshd hybrid-only KEX on west/east (lockout risk) | NEXT-5 E2 | docs/security/pq-host-escalations.md | NS | Y: E2 go | S |
| M-016 | GitHub secret scanning + push protection on; record in BRANCH_PROTECTION.md | SEC SP-02 | .github/BRANCH_PROTECTION.md | NS | Y: GitHub admin | S |
| M-017 | Export-control notice (EAR 5D002 / TSU email) in README + BIS/NSA email; Barrister reviews PQC licensing §E | SEC CG-21, DL-06 | docs/compliance/control-matrix/itar-ear | NS | Y: send the TSU email | S |
| M-018 | README security-posture section (gates, ratchets, SECURITY.md link); SECURITY.md versions table + `/.well-known/security.txt` | SEC SP-04, CG-18 | README.md; SECURITY.md | NS | N | S |
| M-019 | zhen-agentd: refuse start without auth unless `--no-auth`; `-rate-limit` default > 0; no synthesized `direct-user` justification | SEC RT-01, RT-02 | cmd/zhen-agentd/main.go:75; toolexec.go:144 | P | N | S |
| M-020 | FND-005: `git push east` mirror timer on west (accepted-low, tool-only) | SEC FND-005 | deploy/systemd | NS | N | S |
| M-021 | Install `unheaded-baseline` as the enforced audit agent on both hosts is DONE; add `compliance.deviation.<host>` Wotan publish + `.deb` (or decide units-only) | ADR P-08 | ADR-098 Phase 2 | P | N | S |

### E03 — LIVE-SERVICE CORRECTNESS  (tier c, size M, 11 items)
| ID | Item | Sources | Doc | St | Stevie | Sz |
|---|---|---|---|---|---|---|
| M-030 | Wotan admin API key unsettable: `SetAdminAPIKey` has zero callers, `-admin` defaults on, wiki says it works. Read `WOTAN_ADMIN_API_KEY` fail-loud or default `-admin=false`; one env name | PLANS D-08; SEC SP-17 | services/wotan/internal/api/handlers.go:45-50 | P | N | S |
| M-031 | One-shot "wotan unavailable (degraded mode)" in micromanager(5), monad(3), sophia(3), gateway(1), architect(3): `SubscribeWithRetry` per service + late-Wotan test | PLANS F-03; next.md | services/*/ | NS | N | M |
| M-032 | No periodic `ProbeGRPC`: HTTP-degraded client never promotes back to gRPC | PLANS F-04 | pkg/wotan-client/client.go:290 | NS | N | S |
| M-033 | FND-024: monitoring compose stack started by nobody → SLO guard fails closed after every cold boot. Start from kingdom-startup.sh (after main stack) or fold Prometheus+Alertmanager into docker-compose.yml; `slo:error_ratio:5m` presence smoke | SEC FND-024; PLANS F-02; INF-07 | scripts/kingdom-startup.sh | NS | Y: which of the two | S |
| M-034 | FND-023: discover meter follows Cuirass 30 s sync (0.934/0.983 vs 0.99): client probe + drop-on-first-error, or Cuirass health transitions on Wotan; re-run Step 310 | SEC FND-023; PLANS F-01 | ADR-098:425 | NS | Y: meter probes vs Wotan transitions | M |
| M-035 | Both haproxy configs fail `haproxy -c` (internal: log-format req.hdr in defaults; edge: `module-path`, cert dir, rule order) | next.md S14 | docker/haproxy/{internal,edge}/haproxy.cfg | NS | N | S |
| M-036 | Zhen UI chat path bypasses zhen-agentd/Champion gate (`proxy_via_agentd=None`); flip to `/api/v1/agent/ask`, re-run coding gate, close T6a; CSRF token (T8) | PLANS C-06; SEC RT-11 | raft/scripts/zhen_rag.py:231 | P | N | M |
| M-037 | switch-model.sh fit check ignores llama-server `--cache-ram` (8 GiB default; wedged at 9.4 GiB RSS) | PLANS F-20 | scripts/switch-model.sh | NS | N | S |
| M-038 | Wotan server leftovers: rate limiter keys on IP:port; CORS echoes origin with credentials under `*`; `streams_active` ignores topic streams; verify gRPC publish→store | PLANS F-09 | services/wotan/internal/middleware | P | N | M |
| M-039 | Cuirass/Yaldabaoth probes accept any instance-reported address: restrict to lxdbr0 subnet | PLANS F-05 | services/yaldabaoth/allowlist.go:128 | NS | N | S |
| M-040 | `LXD_TRUST_PASSWORD:-changeme` default in setup-host.sh → `:?` or drop trust_password | PLANS D-09 | scripts/setup-host.sh:222 | P | N | S |
| M-041 | East deploy bundle: rebuild debs (huginn rename, new units, wotan.yaml auto-approve chaos-controller), upgrade east+west, update huginn baseline | PLANS F-44/F-06/B-29 | scripts/build-all-debs.sh | P | Y: host window | S |
| M-042 | Routed local turns (`-route local -family -worktree`) not captured as zhen-capture records (feeds the LoRA data trigger) | PLANS F-21 | cmd/zhen-claude/routed.go | NS | N | S |

### E04 — SERVICE AUTH + INTERNAL TLS  (tier d, size L, 7 items)
| ID | Item | Sources | Doc | St | Stevie | Sz |
|---|---|---|---|---|---|---|
| M-050 | ADR-103 build: Ed25519 service JWTs, per-service 0400 keys + `trust.d`, `unheaded-keygen`, hashed `uhk_` API keys + gitleaks rule, OIDC/JWKS built-in issuer, BFF cookies; Cuirass first, Wotan last; closes `check-service-auth.sh` high | ADR T-01; SEC CG-12; PLANS D-10 | docs/adr/ADR-103 | NS | Y: Q1 issuer placement, Q2 lab IdP, Q3 Ed25519→ML-DSA, Q4 key store | L |
| M-051 | FND-017: Wotan + 11 services `-enable-tls` in compose/units with pkg/tls Strict profile; cert distribution via M-050 key layout; non-localhost REST refuses plaintext | ADR T-02; SEC FND-017; PLANS D-10/F-40; INT D-17 | docker-compose.yml:106-117 | P | N (after M-050) | M |
| M-052 | Wotan auto-approve binds to token/cert identity, not self-reported `display_name` | PROTO WOT-06 | services/wotan/internal/grpc/topic_service.go:56-67 | NS | N | M |
| M-053 | Wotan topic signing for `audit.`/`health.`/`alerts.critical` prefixes + drift keys (E5) | SEC CG-14, RT-20 | services/wotan/internal/signing/policy.go:18 | P | Y: E5 per-node key custody | M |
| M-054 | Akira cert-expiry exec check + rotation test (ADR-034 Phases 7/8) | ADR T-03 | runbooks/pki/akira-cert-expiry-check.yaml | NS | N | M |
| M-055 | Dashboard admin: login + RBAC `operator`, signed profile-switch POST to the Well | ADR T-04; PLANS B-30; INT B-3 | ADR-099 §5 | NS | N (after M-050) | M |
| M-056 | Security-headers middleware in pkg/httputil applied by every router | SEC SP-14 | pkg/httputil | NS | N | S |

### E05 — RELEASE PIPELINE: SIGN, PUBLISH, DEPLOY  (tier d, size M, 8 items)
| ID | Item | Sources | Doc | St | Stevie | Sz |
|---|---|---|---|---|---|---|
| M-060 | release.yml cosign + SLSA steps are `echo TODO`: keyless cosign sign-blob + slsa-github-generator; sign future tags | PLANS B-08; INT D-24; SEC SP-05, CG-15, RT-18 | .github/workflows/release.yml:91-100 | NS | N | M |
| M-061 | E4 / FND-018: production evidence build key (ML-DSA-65 seed, custody, `build.pubkey`, quarterly ceremony runbook); Sealed Cask rune signed with Gungnir, Go verifier, tar hardening | SEC FND-018, RT-18; ADR M-08, S-08 | docs/adr/ADR-043 #4 | NS | Y: custody location | M |
| M-062 | deb signing + reprepro publish + fix `unheaded-unheaded-daemon` name; apt repo on west, east `sources.list.d`, upgrade hook | ADR P-01, P-02 | scripts/build-all-debs.sh | P | Y: Jenkins vs GHA as live runner | M |
| M-063 | release.yml builds all debs + eBPF objects; staging merge deploys to east | ADR P-03, P-04 | release.yml | P | N | M |
| M-064 | CHANGELOG rebuilt from sprint summaries; cut v0.2.0-alpha; SBOM attached to release | INT D-25; SEC SP-05 | CHANGELOG.md | P | N | S |
| M-065 | OpenSSF Scorecard + CodeQL (Go/JS/Py) workflows | SEC SP-06, SP-07 | .github/workflows | NS | N | S |
| M-066 | ADR-089 entry gate: scripted byte-size pin for `monad-cpu-ebpf --features ascend-linux` | ADR P-07 | security.yml | NS | N | S |
| M-067 | Pin the 7 unversioned sophia-eye images; Dependabot docker ecosystem | ADR P-09 | docs/policy/image-tag-baseline.txt | P | N | S |

### E06 — THE WELL AS SOURCE OF TRUTH + DATA HYGIENE  (tier d, size M, 8 items)
| ID | Item | Sources | Doc | St | Stevie | Sz |
|---|---|---|---|---|---|---|
| M-070 | ADR-106 Q1: live apply migrations 030-034 as one batch (unblocks `-confirm queue`, worker, kanban card branch/review_note) | ADR-106 Q1; PLANS F-22 | db/migrations/030-034 | NS | Y: apply window | S |
| M-071 | E1 / FND-015: Well PG 16→18 with `ssl_groups` (runbook ready) | SEC FND-015; NEXT-5 E1 | runbooks/data/well-pg18-upgrade.yaml | NS | Y: E1 go (downtime) | M |
| M-072 | Deploy history + chaos journal into the Well (today pkg/statefile); Sword + chaos-controller writers | PLANS F-08; ADR-099 | S-REAL-STEEL follow-on | NS | N | M |
| M-073 | kanban-app + timeguru still `CREATE TABLE IF NOT EXISTS` at startup: ledger check that refuses on missing migration | ADR W-09 | cmd/kanban-app/store.go | P | N | S |
| M-074 | Retention: ratify policy v1; archive-then-purge respecting append-only trigger; timers for service_health_*, zhen_conversations (DELETE grant); PII redaction before `_pg_log` | SEC DL-01, RT-12, RT-13, CG-04 | docs/policy/retention-policy-v1 | P | Y: ratify | M |
| M-075 | kanban_tasks soft-delete purge (ADR-011 Q4) or amend Q4 | ADR X-02 | db/migrations | P | N | S |
| M-076 | ERDA deferred-with-trigger: drop zero-scan indexes after 2026-11-15; NOT NULL/identity migration | PLANS B-25 | WELL-ERDA-AUDIT:240-255 | P | N | S |
| M-077 | SLO definitions as data in the Well + `slo` evidence kind | ADR O-14 | ADR-100 Phase 2 | NS | N | M |

### E07 — OBSERVABILITY + SRE REMAINDER  (tier d, size M, 11 items)
| ID | Item | Sources | Doc | St | Stevie | Sz |
|---|---|---|---|---|---|---|
| M-080 | Watchdog dead-man's switch (placement) + always-firing rule + receiver | ADR S-07, O-13 | ADR-100 §4 | NS | Y: west/east cross-watch vs outside service | S |
| M-081 | Postmortems: template + Well silent outage + Well auth burst + Well exit 255 + clippy red; `docs/incidents/`; one canonical template | ADR O-15; PLANS B-31; INT D-26 | docs/postmortems/ | NS | Y: threshold (ADR-100 Q3) | S |
| M-082 | Vector/ClickHouse: restart Vector for Phase 3 metrics; container stdout (socket proxy vs journald driver); east journal shipping; amend ADR-076 if Vector is the one log path | ADR O-05, O-17 | config/vector/vector.yaml | P | Y: driver vs proxy | M |
| M-083 | 13 dead alerts (monad.yml unheaded_* no code emits; haproxy_*/nginx_* no scrape job) | next.md S7b | monitoring/prometheus/rules | NS | Y: which metrics or delete | S |
| M-084 | TraceCollectorDown rule + YAML runbook for the live collector; HighLatency YAML runbook | INT D-14, D-15 | monitoring/prometheus/rules | NS | N | S |
| M-085 | heimdall-daemon exposes no /metrics; loopback listener + scrape | PLANS F-43 | cmd/heimdall-daemon | NS | N | S |
| M-086 | Last hand-rolled `WriteMetrics` in 5 binaries → registry collectors; delete 8 unset `unheaded_pqc_*` + 12 unserved pkg/secrets metrics (or wire) | ADR O-11, O-12 | pkg/logagg/metrics.go | P | N | S |
| M-087 | `runbook_url` gate on notifying alerts; CI-red SLO; error-budget dashboard panel | ADR O-16, O-18, O-19 | ADR-100 | P | N | S |
| M-088 | Topology map served live (`/viz/topology.html` adapter over real APIs) and Muninn: re-scope against Vector before any build | ADR O-01..O-08 | ADR-076, ADR-086 | NS | Y: still wanted post-Vector? | M |
| M-089 | Unmeasured gates: packet→browser e2e p99 (PERFORMANCE-S77 cites nonexistent `e2ePacketTrace`/`ci-bench`), container start <10 s, 1000 req/s, HTTP/3 leg; perf regression export | PLANS D-02, B-10, D-04, B-11, B-12; INT B-1; PROTO INF-06 | cmd/perf-benchmark | NS | N | M |
| M-090 | Bare-metal hypotheses H1-H9 never scored (BENCHMARKS.md measured most); BENCHMARKS "AF_XDP 920K pps" label wrong (was Python sendto) | PROTO INF-09; INT D-6 | docs/research/VERDICTS.md | P | N | S |

### E08 — COMPLIANCE PAPERWORK + ENFORCING MODE  (tier d, size L, 10 items)
| ID | Item | Sources | Doc | St | Stevie | Sz |
|---|---|---|---|---|---|---|
| M-100 | Three attestations never filed (security-policy-review, ir-plan-review, access-review): zero is unclaimable | SEC never-observed, CG-05/06/09; ADR C-01 | compliance/attestations/ | NS | Y: review + signed commit | S |
| M-101 | Information security policy + single-operator posture page (roles, AT/PS accepted gaps) | SEC CG-05, CG-06 | compliance/ | NS | Y: sign | M |
| M-102 | IR plan: refresh §1 (Suricata gone), first tabletop recorded, counsel/DFIR decision | SEC CG-01 | runbooks/security/ir-plan-v1 | P | Y | M |
| M-103 | System contingency plan (hosts, Wotan state, /var/lib/unheaded, models, LXD; RTO per service; one host-rebuild exercise) | SEC CG-03 | compliance/ | P | N | M |
| M-104 | Risk strategy (appetite, categorization, criticality, SoA) + SSP generated from catalog + evidence | SEC CG-07, CG-08 | compliance/catalog | NS | Y: appetite | L |
| M-105 | Host CIS hardening applied (ADR-098 steps 2-6, east first, nft commit-confirm, AIDE, AppArmor enforce); re-apply lab fixes via `unheaded-remediate` so hosts have journals; retire remediate.sh; journald caps | SEC CG-13; ADR C-05; next.md S7b | compliance/findings/register.yaml host-journal rows | P | Y: host changes, SystemMaxUse numbers | L |
| M-106 | Enforcing mode: typed enforce actions via ADR-099 executor, flapping breaker, preinst gate, 7-day soak, Lich campaign against the baseline agent; Q5 | ADR C-03 | cmd/unheaded-baseline | NS | Y: Q5 refuse starts vs installs | L |
| M-107 | Rendered Ansible syntax-check in CI; Kingdom Ansible roles (kingdom-base, huginn, victoria) | ADR C-04, K-05 | deploy/ansible/compliance | NS | N | S |
| M-108 | Crosswalk mappings checked against OLIR/TSC/CCM; control matrices bannered as May-2026 snapshot (cite deleted code, GDPR/CCPA PII rows MAPPED to deleted DataIsolationConfig) | SEC CG-02, CG-26; INT B-3 | docs/compliance/control-matrix | NS | N | S |
| M-109 | Privacy pack (F-PII scan, notice, processing record, DPIA for eBPF + Zhen capture); processor/BAA template; MFA scope decision; FIPS build variant; supplier inventory; independent assessment decision; host malware/IDS decision | SEC CG-10, CG-11, CG-16, CG-19, CG-20, CG-22, CG-23 | docs/compliance | NS | Y: MFA scope, external pentest, IDS/AV | L |

### E09 — CHAMPION / ZHEN HARDENING + RUNBOOKS  (tier d, size M, 12 items)
| ID | Item | Sources | Doc | St | Stevie | Sz |
|---|---|---|---|---|---|---|
| M-120 | 11 runbooks `status: draft`, `approved_by: ""`, all `authored_by: claude` (local models 0.796 < 0.85 bar): approve, and record "Claude authors, Zhen executes" in ADR-024/106 or re-run rb-author when a local model clears | INT A-1, A-2; ADR-106 Q3 | runbooks/**/*.yaml | NS | Y: approve each; author policy | S |
| M-121 | Trust-label check is `== "external"` only (typos/case/NUL pass); exact allowlist + vor chain verifier + fuzz target | SEC RT-06 | pkg/champion/toolcall.go:187 | NS | N | M |
| M-122 | Confirm tokens not bound to requester; no cleanup ticker; no write-size cap; `runbook_show`/`runbook_list`/`system_command` listed not dispatched; audit-write failures silent | SEC RT-03, RT-04, RT-05, RT-09, RT-10 | pkg/champion/{confirm,write,dispatch}.go | P | N | S |
| M-123 | Destructive-verb list incomplete + system prompt copied in 3 places → `pkg/zhenrag`; runbook body scan + hash check before execute; GGUF/embedder SHA pins | SEC RT-07, RT-08, RT-15 | cmd/zhen-cli, cmd/zhen-rag, raft | NS | N | M |
| M-124 | Champion sandbox write TOCTOU (`openat2 RESOLVE_BENEATH`) | ADR T-05 | pkg/champion | NS | N | S |
| M-125 | Routers: fold `ThresholdRouter`/`AutoRouter` or keep; family caps leave one worst-case request; ADR-019 L2 amendment; ADR-019 open-question checklist closed | PLANS F-23, F-24; ADR S-02, Z-19 | pkg/claudebridge | P | Y: fold?, caps, amendment | S |
| M-126 | First unattended overnight run (when, cap, env file) | ADR-106 Q2 | cmd/zhen-agentd overnight | NS | Y | S |
| M-127 | zhen-eval still skips `runbook-draft` though Phase 8 passed; un-skip with tests/runbooks as judge; run-runbook.py executes `verification:` or migrate 4 runbooks | ADR Z-08; INT A-5 | cmd/zhen-eval/main.go:307 | P | N | S |
| M-128 | Kanban Runbooks view (create task from runbook, per-step progress via Wotan); card modal shows branch/review_note; Task:↔commit trailer hook or mark ADR-038 dormant | ADR Z-07, Z-09; PLANS F-22 | cmd/kanban-app | NS | N | M |
| M-129 | zhen-cli Phase 2b/3: `/recall` `/remember` persistence, `/confirm`, API key, readline; `/model`; ADR-060 tail (LICH-013, swap metrics, `--json`, rollback) | ADR Z-11, Z-12, Z-13 | cmd/zhen-cli | NS | N | M |
| M-130 | zhen_scheduler.py: supervise (unit) or fold into zhen-agentd; Well writes; Kanban card per run; heartbeat alerts | ADR Z-06 | raft/zhen_scheduler.py | P | N | M |
| M-131 | vor retrieval backend unmanaged (no unit, no pkg/ports entry); memory consolidation job; recalled-memory provenance shown; `_strip_tool_call_json` test | ADR L-04, Z-17; SEC RT-14, RT-16 | deploy/systemd | P | N | S |
| M-132 | Go MCP server replacing Python (`.mcp.json` still launches zhen-mcp.sh); MCP tests not run in raft CI; isError on denied | ADR Z-02; next.md S17 | cmd/lich-mcp skeleton | P | N | M |

### E10 — PROTOCOL SPEC-VS-CODE + RFC EDITOR DESK  (tier d, size M, 10 items)
| ID | Item | Sources | Doc | St | Stevie | Sz |
|---|---|---|---|---|---|---|
| M-140 | Flow-action values: spec FORWARD=0x00, code 0x01/Drop 0x05 (wire byte) | PROTO MON-02 | bpfschema/core_maps.go:95-99 | NS | Y: spec wins (E4 precedent) or erratum? | M |
| M-141 | Anamnesis event types 0x00-0x03/0x06 vs code Birth/Hop/Death 0x01-0x03; 0x05-0x07 have no producer | PROTO MON-03, ANA-02, ANA-03 | ebpf/monad-common/src/lib.rs:185 | NS | Y: spec wins? | M |
| M-142 | RFC Editor items 1-5 (Sophia `x-cartridge` key, sub-kind 0x43, Wotan topics, CSR range text, KF10 opcode table + MOVI) → ADR-108 Accepted; Gjallarhorn Kingdom Mode + Flow Action values claimed | PROTO MON-15; UPC W-3; ADR U-09; PLANS U-10 | docs/protocol/RFC-EDITOR-REQUESTS-S5.md | NS | Y: accept ADR-108 after | S |
| M-143 | bpfschema `FlagK0/K1` "Kingdom Mode" → CUSTOM/RSVD; audit line wrong; 4 duplicate CRC16 impls → one | PROTO MON-01, MON-16; INT D-11 | pkg/protocol/bpfschema | P | N | S |
| M-144 | Error-code registry (13 codes) has no Go/Rust symbols; shield uses hop_count proxy; TLV container + Ring Path Counter reference-only | PROTO MON-07, MON-08, SHD-01/02 | docs/protocol/error-registry.md | NS | N | M |
| M-145 | Wotan-03 structured error code type; GOAWAY + SETTINGS on topic streams (or delete reference pkgs); SIGHUP topics reload; wotan-04 light-touch verifications | PROTO WOT-03, WOT-04, WOT-07, WOT-09 | services/wotan | NS | N | M |
| M-146 | G8 reference packages (17): RFC Editor rules keep-as-conformance / wire-in / delete per package | PROTO UNR-06 | docs/UNREACHED-GO-PACKAGES.md:28-33 | NS | N | M |
| M-147 | Anamnesis-00 stub; Protocol Overview 7th I-D; IETF datatracker upload of six -00 drafts + record URLs | PROTO ANA-01, MON-13, MON-14; PLANS B-02, B-05; INT D-23 | ietf-submission/ | NS | Y: upload (browser), Track call | M |
| M-148 | Formal acceptance of built Proposed ADRs: 093, 094, 101, 104, 105, 106, 107 (+108 after M-142); doctrine amendment ADR-004:25 | ADR S-01, W-07 | docs/adr/ADR-INDEX.md | NS | Y: accept | S |
| M-149 | Sophia dictionary signing is bare SHA-256 (`sophiasync`), ML-DSA-65 required; mandatory delta signatures rollout (ADR-062 tail) + severity ladder doc | PROTO SOP-04; ADR L-01 | pkg/protocol/sophiasync | NS | N | M |

### E11 — SUPPLY CHAIN + SOURCE SWEEP  (tier d, size L background, 10 items)
| ID | Item | Sources | Doc | St | Stevie | Sz |
|---|---|---|---|---|---|---|
| M-160 | 71 of 232 first-party .rs files lack SPDX; ci spdx-check scans *.go only → backfill + extend | PLANS B-06; SEC SP-19 | scripts/check-spdx*.sh | P | N | S |
| M-161 | ~290 non-Go files + LICENSE line 1 ("Steven Bellis") + 21 Go/Rust headers still MIT wording under GPL SPDX; RELEASE_CHECKLIST says MIT | next.md S13/S12; INT #6 | LICENSE; scripts/kingdom-shutdown.sh | NS | Y: relicense call | S |
| M-162 | `gopkg.in/yaml.v3` archived, 35 prod files; `cilium/ebpf` 8 files unrecorded (GO-2026-6238 reachable); goldmark unrecorded; register thresholds | ADR W-02, W-05, S-10, W-04 | docs/adr/ADR-096 | NS | Y: thresholds; sign-offs | M |
| M-163 | ADR-004 admissions: aws-lc-rs, anthropic-sdk-go (register dates) | NEXT-5 item 1 | docs/adr/ADR-004 | NS | Y: sign off | S |
| M-164 | ADR-090 ledger: step 1 (vendored/generated → SKIPPED) never run; 2192 PENDING; Q1/Q2 open; 29 `#[ignore]` forge tests | ADR W-01, S-06; PLANS B-22 | docs/sweeps/TOTAL-SOURCE-SWEEP.tsv | P | Y: Q1/Q2/Q3 | L |
| M-165 | Bandit 43 findings (B310 x35, B104 x2, B615 x2, XML x4) → decide groups, flip gating; trivy IaC `exit-code 0` → ratchet; hadolint `latest-alpine` pin | SEC SP-08, SP-09; PLANS B-16 | static-analysis.yml:190 | P | Y: four groups; 0.0.0.0 bind | S |
| M-166 | 18 `omitempty` struct/time fields in 9 files; BLE001 134 blind excepts (triaged, excluded); gosec G115 ~410 sites + ST10xx | SEC SP-11; PLANS B-14; ADR W-08 | .golangci.yml | P | N | M |
| M-167 | Python requirements: notebooks/ + tomb/grimoire/rag unpinned, none hash-pinned; hash-pin pip CI tools | ADR W-06; next.md S15 | .github/ci-python | NS | N | S |
| M-168 | pkg/waf 7 Go packages kept only because tests/s77 asserts files; lxd/ container defs superseded by Cuirass; host-a/host-b stacks (FND-009 D11); `internal/bpfmap` + `pkg/ebpf/maploader` delete; `cmd/ebpf-exporter` ORPHAN; `pkg/certs/acme` gone (done) | ADR S-05; PLANS B-09, B-19, B-20; PROTO UNR-03, UNR-07 | docs/sweeps/REAL-STEEL-UNREACHED.tsv | NS | Y: each delete word | S |
| M-169 | ScanCode re-run on HEAD (last 2026-04-04, 108 pkgs deleted since); THIRD_PARTY.md SPDX OR elections; fair-use notice in the_first_packet.md | PLANS D-14; SEC DL-02, DL-04; ADR X-04 | docs/legal | P | N | S |

### E12 — SHIELD / eBPF DATA-PLANE TRUTH  (tier d, size L, 9 items)
| ID | Item | Sources | Doc | St | Stevie | Sz |
|---|---|---|---|---|---|---|
| M-180 | Shield HMAC / retry-token / hop-validator / GOAWAY / Sophia-authority / Dest-Opts checks are stubbed ("Deferred until bpf_loop"); no userspace writes the 3 maps; S24 docs + code comment say WIRED. Restore via bpf_loop (cartridge-ebpf pattern), loader for maps, register finding | INT D-1; PROTO MON-07 | ebpf/shield-ebpf/src/main.rs:268-273,516-544 | P | N | L |
| M-181 | `PQC_SIG_STATUS` has no producer/consumer; `cmd/pqc-verifier` ORPHAN; services/keymgmt parked: wire OPTIMISTIC + populator, or delete so nothing claims XDP PQC | SEC SP-12, RT-24; PLANS B-15; PROTO MON-10; INT D-2/D-3/D-4 | docs/security/shield-ebpf-pqc-fast-path-unwired | NS | Y: fail-open/closed/delete; lift Layer-1 freeze? | L |
| M-182 | packet-marker TCP/IP-option trace-ID extraction `TODO(E1)` (blocker cleared by cartridge-ebpf subprograms) | INT D-9 | ebpf/packet-marker/src/main.rs:190,214 | NS | N | M |
| M-183 | Debug TC stats (keys 13-15, 3 map ops/packet) still in production Shield; stress cannon sends no CRC; faster sender to find real ceiling | INT D-6, D-7 | ebpf/shield-ebpf/src/main.rs:460-475 | NS | N | S |
| M-184 | ADR-012 ratchet does not exist: ascend 926,542 > 900K trigger; `bpf-verifier-check.sh` estimates bytes/8; record verified insns per variant from `check-ebpf-loads.sh`, fail on >10 % growth | ADR U-01 | scripts/bpf-verifier-check.sh | P | N | M |
| M-185 | BPF struct single source of truth (parity test hardcodes sizes; dead `unhd_*` names) | INT D-10 | pkg/protocol/bpfschema/parity_test.go | NS | N | M |
| M-186 | 8 G9 control planes with no `cmd/` (compliance, firewall, nfv, qos, selfheal, version, keymgmt+3; loadbalancer F6): build per `cmd/anomaly` recipe or delete | PROTO UNR-01, UNR-02; INT D-20/D-21/D-22 | docs/UNREACHED-GO-PACKAGES.md:98-110 | NS | Y: un-park or delete | M each |
| M-187 | AF_XDP: product path (loader + XSK bind + real NIC) or retire Decision 5 | INT D-19; PROTO INF-10 | pkg/afxdp | P | Y | M |
| M-188 | FND-020: Wotan replication.proto drops message signatures (additive fields) | SEC FND-020; PLANS F-41 | services/wotan/proto | NS | N | S |

### E13 — CONSENSUS + DRIFT: AKIRA, HEIMDALL, MÍMIR  (tier d, size M, 6 items)
| ID | Item | Sources | Doc | St | Stevie | Sz |
|---|---|---|---|---|---|---|
| M-200 | Akira: per-service health reporter in every binary (hosts read 0 %/100 %); Wotan ballot + `system.health.remediate`; cross-host two-reporter run recorded | ADR H-01, H-02, H-03 | docs/adr/ADR-029 | NS | N | M |
| M-201 | E5 / FND-019: Heimdall drift keys per node, `heimdall` in auto_approve, unit on west/east (ORPHAN today) | SEC FND-019, RT-19; ADR M-01 dep | cmd/heimdall-daemon | NS | Y: key custody | M |
| M-202 | LICH-012 L12a (key storage) + L12d (eBPF drift fuzz) not run; Enkrateia auto-restore v2; semantic drift diff; CTF activation wiring | ADR M-01, M-03, L-02 | tomb/lich/LICH-012-config-convergence | NS | N | L |
| M-203 | Detection rules missing (SSH origin, DGA/beaconing, outbound volume, DNS tunnelling, lockout); NVD/GHSA pollers (ADR-055 P2) or declare KEV-only | SEC RT-21, RT-23, SP-18 | monitoring/prometheus/rules | NS | N | M |
| M-204 | Multicast bootstrap `ff02::1:abba` never run end to end on L2 (east as node) | ADR M-07 | cmd/gjallarhorn-sender | P | Y: east | S |
| M-205 | Threat-model review cadence lines; RT-32 CPU_MAP concurrent-write disposition; Tomb offensive campaigns registered in ADR-062 before host exposure | SEC RT-31, RT-32, RT-29 | docs/security/threat-model-* | NS | N | S |

### E14 — UPC (NON-DOOM)  (tier e, size L, 12 items)
| ID | Item | Sources | Doc | St | Stevie | Sz |
|---|---|---|---|---|---|---|
| M-220 | getpid flip: `GETPID_INJECT=false` one const away; run the named probe (BPF-forked child calls getpid flag-ON); decide pid numbering; proc[] dual-books reconciliation | UPC DL-1, E-1, E-2, S-1 | ebpf/monad-cpu-ebpf/src/main.rs:413 | P | Y: xv6 `p->pid` vs BPF slot | M |
| M-221 | Verifier delta now NEGATIVE (ON 911,319 vs OFF 926,542); EVOLUTION-1's own rule says migration ladder self-funding → re-read D1 reach, then forge Stage 2 plan (sbrk/pipe/kill/mkdir/unlink/link/chdir/sleep arrive in-kernel) | UPC S-10, E-3 | EVOLUTION-1-TRAP-FLIP.md | NS | Y: re-read D1 | L |
| M-222 | `gate_nway` N-way concurrent isolation has never passed (KF1 since Gate A); gate2 sequential is the only isolation proof; forkbomb gate | UPC K-8; ADR U-06 | crates/xv6-mbc/upstream/user/gate_nway.c | P | N | M |
| M-223 | ADR-080 loader I/O-time gates (surface-vs-object feature check, per-pid MMU slice non-overlap) named in docs, not performed; closes Epic 1.2.5 | UPC K-1; ADR U-02 | crates/upc-api/src/workload.rs:178 | NS | N | M |
| M-224 | Required-before-prod (brief 2026-05-13) 0/5: nft forward-drop on veth-upc0p; bootctl unprivileged + CAP_BPF/NET_ADMIN; seccomp; `UPC_DEMO_MODE` gate; RV2MBC SHA default-on + BootParams v3 | SEC RT-U1..U5; UPC K-11; INT D-12 | references/security-upc-linux-spitball-2026-05-13.md | P | N | M |
| M-225 | Freeze ROM_MAP + RV2MBC_MAP after load (`NewLoadSequence` has zero callers); remove hardcoded `unheaded-doom-secret` HMAC default; flow-label instance hijack validation | SEC RT-25; INT D-13 | pkg/ebpf/mapfreeze.go; monad-cpu-ebpf main.rs:644 | P | N | M |
| M-226 | FS substrate: device inodes (T_DEVICE never handled); FS writes + `>` (no block alloc; route decision in-BPF at 92.7 % vs Stage 2); pipes | UPC K-3, K-4, K-5; ADR U-03..05 | UPC-LINUX-MASTER-BATTLE-PLAN.md:78-92 | NS | Y: in-BPF vs kernel-side route | L |
| M-227 | riscv.h `uint64` CSR helpers undefined high bits on ilp32e | UPC K-9 | crates/xv6-mbc/upstream/kernel/riscv.h | NS | N | S |
| M-228 | East as a node: Mímir Phases 9/11/13 re-run (Step 177), DISTRIBUTED scorer weights over Wotan (Step 435), full-engine LICH-015 arm, adversarial MBC fuzz 0x47-0x4A; ADR-043 status claims 13 phases shipped | UPC W-1, W-2, W-7, S-2, S-12; SEC RT-28; PLANS F-61 | S-UPC-WIRE-COMPUTE plan Steps 177/435 | NS | Y: approve east, netns fallback, or drop Phase 18 | M |
| M-229 | Live legs never run: `anomaly.scores` over Wotan, Sophia registry backend; scorer trained on synthetic only (real labelled flows now exist via steel-echo); interpreter-cost gates #1(a)/#2 accept or fund (8 steps/callback, DOT4 ISA gate) | UPC W-4, W-5, W-6, S-4, S-5, S-6 | references/s5-upc-wire-log.md | P | Y: DOT4 sprint?, gates accept?, scorer real data? | M |
| M-230 | unheadnix naming ceremony (Q5) + find-replace "Unheaded Linux"; ADR-023 → Superseded-by-074; wiki L6 rung reconciled; `crates/upc-bootstub` delete or keep; Phase 2.5 L6 brief (pool-node image) when sequenced | UPC S-7, S-9, S-11, DL-2, DL-3 | docs/adr/ADR-081 | NS | Y: name + Lore; when L6 starts | S/L |
| M-231 | Browser xterm end-to-end with live keyboard never run in a real browser; Lich Phase 5 live local-model e2e never run (`LICH_E2E=1`) | UPC K-16; PLANS F-60 | dashboard/upc-console.html | P | Y: one attended run | S |

### E15 — DOOM  (tier f, LAST, size M, 7 items)
| ID | Item | Sources | Doc | St | Stevie | Sz |
|---|---|---|---|---|---|---|
| M-240 | STATUS.md/README say banding "NOT A BUG / authentic magnification"; HANDOFF proves Z_Malloc/`texturecolumnlump` corruption. Fix the docs first | UPC D-2 | docs/doom/STATUS.md:82 | NS | N | S |
| M-241 | Banding root cause: differential-test the two executor hypotheses (misaligned LD, word-memcpy) on LICH-015 harness; try bump-only zone allocator (Doom C source out-of-repo) | UPC D-2 | docs/doom/HANDOFF.md:147-318 | P | N | L |
| M-242 | E1M2 texture corruption open since 2026-03-30; confirm same corruption as M-241 (not size/IWAD) | UPC D-1 | docs/doom/RUNBOOK.md:430 | NS | N | M |
| M-243 | CPU_MAP "corrupt" readback + `.bss` visibility: one sentinel shift to close H1 | UPC D-3 | docs/doom/HANDOFF.md:124-135 | P | N | S |
| M-244 | Committed `doom.mbc` ≠ translator output of committed `doom.elf`; regenerate or record provenance; harness self-consistency check | UPC D-10 | doom/ | P | N | S |
| M-245 | 3 Doom units run as root from /home (FND-010 half): `unheaded-doom` sysuser + caps; then flip `User=` gate to enforce | PLANS B-26, B-28; SEC FND-010 | deploy/systemd/unheaded-doom-*.service | NS | N | M |
| M-246 | Weapon-sprite flash; demo video; visible fps never re-measured with bridge.rs; sound/multiplayer (ISA gate) stretch | UPC D-5, D-7, D-8, D-9 | docs/doom/NEXT-STEPS.md | NS | N | S/L |

## 2. Decisions for Stevie (numbered, grouped)
Defaults are the sweeps' recommendations where one exists.

**E01/E05 CI + release**
1. Nightly drift-detect on main: delete / dispatch-only / self-host? Default: delete (terragrunt never deployed). Unblocks M-005.
2. "Gates can actually fail": raise timeout to measured time or split? Default: measure once, set +20 %. M-004.
3. Promote develop→staging (1062 commits) on your word; then staging→main + signed tag; GitHub default branch develop + protect main; secret scanning + push protection on. M-008, M-016.
4. Live release runner: Jenkins or GitHub Actions? Default: GHA (Jenkins not in LIVE-PATHS). M-062, M-063.
5. Build-key custody (E4) and remediation-snapshot key (ADR-043 #4): where, who holds, how `build.pubkey` publishes. M-061, M-105.
6. ADR-004 admissions: aws-lc-rs, anthropic-sdk-go. M-163.

**E02 security**
7. Credential rotation: you rotate the baselined set (OPNsense API, Grafana, LXD trust, tokens in 20 fingerprints); I shrink the baseline after. M-010.
8. FND-001 SMTP relay: Gmail account + app password. M-011.
9. FND-011: apply ebtables modprobe stopgap on both hosts; keep west userns open for LXD (test first)? Default: stopgap yes, userns stays open until tested. M-012.
10. E1 PG 16→18, E2 sshd hybrid-only, E3 PSK rotation + timer, E5 Heimdall drift keys: go/no-go each. M-071, M-015, M-014, M-201.
11. Send the EAR TSU notification email. M-017.
12. Relicense ~290 MIT-header non-Go files + fix LICENSE "Steven"? Default: yes, mechanical, except any file deliberately MIT. M-161.

**E03 live services**
13. FND-024: start monitoring from kingdom-startup.sh, or fold Prometheus+Alertmanager into main compose? Default: kingdom-startup (keeps stacks separable). M-033.
14. FND-023: client probe between refreshes, or Cuirass publishes health transitions? Default: client probe (no new Wotan topic). M-034.
15. East deploy window for debs + wotan.yaml + huginn baseline. M-041.

**E04 service auth (ADR-103 Q1-4)**
16. Issuer: own `unheaded-auth` service [rec] or inside dashboard-backend. 17. Lab IdP: built-in [rec] or Authentik/Keycloak. 18. Tokens: Ed25519 now, ML-DSA-65 later [rec] or ML-DSA from day one. 19. API-key store: the Well [rec]. M-050.

**E06/E07/E08 Well, SRE, compliance**
20. Apply migrations 030-034 live as one batch (ADR-106 Q1). M-070.
21. Ratify retention policy v1; sign the three attestations after reviewing policy / IR plan / access list. M-074, M-100.
22. Watchdog placement: west/east cross-watch or outside service? Default: cross-watch now, outside later. M-080.
23. Postmortem threshold (ADR-100 Q3). Default: any customer-visible outage > 1 h or any data-loss near-miss. M-081.
24. Vector container stdout: socket proxy vs journald log driver? Default: journald driver (no Docker socket exposure; amends ADR-092). M-082.
25. 13 dead alerts: which metrics (frr_exporter BGP?) or delete? Default: delete, re-add with a producer. M-083.
26. journald SystemMaxUse=1G RuntimeMaxUse=200M (east first); two world-writable ROCm logs chmod 0640. M-105.
27. Topology map + Muninn (ADR-076/086): still wanted post-Vector? Default: re-scope to Vector config; retire Muninn daemon. M-088.
28. Enforcing-mode Q5 (refuse starts vs installs); MFA scope; external pentest; host AV/IDS stance. M-106, M-109.
29. Accept built ADRs: 093, 094, 101, 104, 105, 106, 107; 108 after RFC items. M-148.

**E09 Zhen**
30. Approve the 11 draft runbooks; record "Claude authors, Zhen executes" or wait for a local model ≥ 0.85. M-120.
31. ADR-106 Q2 first unattended night (when, cap, env); Q6 ADR-019 L2 amendment accept/reject; Q7 fold routers?; family caps headroom; defaults `-daily-cap-usd` $5 / max_tokens 16000. M-125, M-126.
32. Bandit groups: skip B310 (guarded), 0.0.0.0 bind (zhen UI) keep/bind lxdbr0, pin HF revision, defusedxml y/n. M-165.

**E10 protocol**
33. Flow-action values: spec wins (renumber code) or erratum? Default: spec wins (E4 precedent). M-140.
34. Anamnesis event types: spec wins? Default: spec wins. M-141.
35. RFC Editor items 1-5: I run the RFC Editor pass; you accept ADR-108 after. M-142.
36. IETF datatracker upload (browser) + Track call. M-147.

**E11/E12 supply chain, data plane**
37. ADR-095/096 register thresholds (age, importers, CVE); lib/pq keep-and-record vs own client; yaml.v3 successor; cilium/ebpf own vs record. M-162.
38. ADR-090 Q1 vendored scope, Q2 docs/ in scope, Q3 29 ignored forge tests restore/delete. M-164.
39. Delete words: pkg/waf 7 pkgs (amend tests/s77), lxd/, host-a/host-b stacks (FND-009 D11), internal/bpfmap + maploader, cmd/ebpf-exporter. M-168.
40. PQC_SIG_STATUS: wire OPTIMISTIC / delete / lift Layer-1 freeze? Default: delete until keymgmt is un-parked. M-181.
41. G9 control planes (8): un-park which, delete the rest? Default: keymgmt only if M-181 wires; delete firewall/nfv/qos/selfheal/compliance/version/canary-ebpf consumers. M-186.
42. AF_XDP: product path or retire Decision 5? Default: retire (XDP/TC supervised path is live). M-187.
43. Is Kubernetes still a deploy target? Decides CG-24/SP-08/SP-23/B-17/K-01..06/A-4 (all parked in §4 until answered). Default: lab-only, not a target.

**E14/E15 UPC + Doom**
44. getpid numbering: xv6 `p->pid` (init=1) or BPF slot (init=0). Default: kernel numbering (EVOLUTION-1 direction). M-220.
45. Re-read D1 reach against the negative verifier delta (self-funding ladder → push toward (a)). M-221.
46. East as a node: approve Steps 177/435, netns fallback, or drop Phase 18. M-228.
47. DOT4 ISA sprint? Accept gates #1(a)/#2 as interpreter cost? Scorer on real traffic or demonstrator? Default: accept gates, retrain scorer on steel-echo flows, no DOT4 yet. M-229.
48. FS writes route: in-BPF (92.7 % wall) or kernel-side via Stage 2. Default: Stage 2. M-226.
49. unheadnix naming ceremony with Lore; when Phase 2.5 (L6) starts. M-230.
50. Doom stays in repo (S35 "move out before public" superseded): one-line note. Doom is lowest priority; E15 runs only after E01-E14 or in an idle window.

**Standing / parked**
51. Sleipnir Phase 1 "deferred to Q4 2026" (now): start or re-defer with a date. 52. Yggdrasil task #65 first real build (Q4 2026 trigger). 53. GCP cost alarms (ADR-058, ~30 min console). 54. NetBox on west (ADR-033). 55. tools/ standalone extractions still wanted? 56. zhend Layer 0, Sabatons, ADR-064 Wotan active/active go-ahead. 57. Demo video + IETF upload + Track-call record retro-fill.

## 3. Marshal re-grounding (git log since 2026-10-20: 652 commits; every tier (a)-(c) and S item grepped at a7fe3acd)

**Removed as already closed:**
- Well DB pointer split (promotion log): `pkg/database` now has deliberate `OpsWriterConfig`→unheaded_ops and `AppKanbanConfig`→unheaded_app; compose sets WELL_DB per service; raft uses unheaded_app; ADR-101 ledger W-01..W-24 DONE. Not an item.
- `bootstrap-llm-lab.sh` Node via curl|bash: gone. `ci-protocol.yml`: deleted. `services/canary`: deleted (E1 G3). FND-021/022: closed 06280b2d. Repo public flip: DONE. Heimdall C4 TODOs: landed in S5. kingdom-shutdown monitoring straggler: fixed 168410a9. Champion symlink path check: done. Doom PC-corruption ↔ RET floor: verified not a blocker (ADR-079 run 2026-07-03). Phase 1.3 E/F: done. Live keyboard (S5 Phase 4): done. ASCEND fork decision: ADR-081 done.
- `go mod tidy -diff`: clean at a7fe3acd (was non-empty at f7d730e7). `go build ./...`: clean.

**Contradictions between sweeps, resolved:**
- Open FND count: next.md said 4, SEC register says 14 (+1 untriaged, 3 never-observed). SEC wins (regenerated live 2026-10-27).
- PLANS part B had "sub-50 ms e2e DONE (18 ms)"; INT B-1 + PROTO INF-06 show `e2ePacketTrace`/`ci-bench` do not exist. Kept open as M-089; PERFORMANCE-S77 is a doc fix (§5).
- UPC DL-5 says browser fps item OBSOLETE (bridge.rs 60 Hz) while PLANS U-11 keeps "fps ≥ 15" open: kept as "never re-measured" inside M-246, not a build item.
- ADR sweep W1-01 "Snapshot RPC M" vs PLANS F-07 "gated on ADR-064": parked (§4) per Stevie's go-ahead rule.
- SEC SP-16 (Falco) OBSOLETE vs ADR M-01 Heimdall: Heimdall is the runtime monitor; folded into M-201/M-202.
- INT D-17 "mTLS on every link" = FND-017 = ADR T-02 = PLANS D-10: one item M-051.

**Downgraded to doc-hygiene (§5):** ADR-043 status, STATUS.md "NOT A BUG", ADR-036 status, PERFORMANCE-S77 numbers, S24 "WIRED" claim, ALIGNMENT_NOTES_DRAFT05 fabricated, timeline header, INF-07/OBS-02 telemetry topology, INF-16/18 architecture vision pages, CLAUDE.md stale epics/criteria, ADR-092/094/107 progress notes, K-2/K-15/D-4 stale REMAINS clauses, marshal-parked C4 file, ADR-022/045/015/040 statuses.

## 4. Parked (standing decision that parks each)
- **LoRA / forge** (Sprint 2 Phase 12 DEFERRED-on-data; trigger ≥ 300 SFT examples across ≥ 3 families): BackwardScratch, KV-cache, MockBackend, fuzz loaders, scrambled-label control. **Do first when the trigger fires:** per-eval-sequence `answer_start` fix (PLANS C-02, S) before any "learned" claim. M-042 feeds the trigger.
- **ADR-064 Wotan active/active** (Stevie go-ahead): snapshot RPC, raft, StatefulSet 3, cutover runbook stub, FND-020 field design follows it.
- **zhend Layer 0** (NEXT-5 Parked): 8 success metrics, HbH registry entry. **Sabatons IPMI/Redfish** (Parked). **ADR-088 Kubernetes** + all k8s rows (CG-24, SP-08, SP-23, B-17, K-01..K-06, A-4 cutover, kev-poller DaemonSet) until decision 43.
- **tools/ standalone extractions** (10 plans, README scaffolds only) until decision 55. **ADR-087 NOC** until a managed Junos device exists. **ADR-058 GCP alarms**, **ADR-033 NetBox**: Stevie's console time.
- **Sophia draft-04 items behind the ratified DEFER** (CBOR, root ARRAY_OF_MAPS, nested sub-dicts, QPACK sync, version wraparound text). **Kingdom Mode / Inverse Mask** (wire unfreeze). **FN-DSA/HQC** (no library). **CRC-32C** rejected.
- **G9/F6 parked eBPF apps** until decision 41. **Zhen Python→Go UI port** (Stevie's exercise; CSRF T8 rides on it, interim token in M-036). **WAVE14 retrain** superseded by Phase 12 trigger.
- **Sleipnir Phase 1**, **Yggdrasil #65**, **unheadnix L6 Phase 2.5**, **DL-4 guest networking / SSH Mode C**, **K-6 code-store B/C**, **K-10 CSR forms**, **K-17 guest-plugin seam**, **K-12 DISTRIBUTED pgd handle**: trigger-gated by their own plans.
- **PQC parked ideas** (ML-DSA X.509, CNSA 2.0, Rosenpass, age X25519, PSK-less peers): material for a future PQC sprint. **ADR-053 workflow templates** (Pipe Dream). **Repo split / multi-tenant / multi-cloud / SIEM-IDP / competitor-parity wishlist**: no ADR; drop or ADR first.
- **Doom stretch** (sound, multiplayer, perf beyond playable, doom-bridge audio): inside E15 only if Doom UX becomes a goal.

## 5. Doc-hygiene / Librarian epic (one pass; doc says X, code says Y → fix the doc)
1. `references/timeline.md` header/footer: LAST UPDATED 10-13, HEAD paragraph (09-29), STATUS "ASCEND-LINUX L5→L6 actively shipping", "Synced 2026-09-30", Remaining-for-Age-3 (WAVE14 retrain, "VC/public readiness"). Body + mirrors are current to 10-26.
2. `docs/protocol/ALIGNMENT_NOTES_DRAFT05.md` + `PROTOCOL_STATUS_S72.md` test table: fabricated (17/17 paths absent) → banner "historical, unverified" or retire.
3. ADR-043 status "All 13 phases shipped" vs Phases 9/11/13 pending east; `wiki/Mimirs-Law.md`, CLAUDE.md Mímir bullet.
4. `docs/doom/STATUS.md:82` "NOT A BUG" banding vs HANDOFF; CLAUDE.md "Current blocker: PC corruption" stale; NEXT-STEPS/STATUS fps lines (2026-03-30).
5. ADR-036 → Withdrawn (no training on Claude outputs); ADR-INDEX row; archive `docs/TODO-claude-qa-generation.md`; confirm `/var/zhen/distilled_qa_claude.jsonl` excluded under `--strict-data`.
6. `docs/INTERNAL.md` "active plan" S73 stale; byte-identical S73 copy under docs/battle-plans → delete one. ~128 of 150 docs/internal files purely historical → docs/archive.
7. `docs/workflow/AGENT-OPERATING-PROCEDURE.md`: `git push origin main`, S<N> handoff files, "134/134", Go 1.24.
8. `docs/launch/RELEASE_CHECKLIST.md` says LICENSE is MIT; LAUNCH_READINESS cites deleted SURICATA_GPL_ISOLATION, "Branch: main / READY TO LAUNCH".
9. CHANGELOG.md [Unreleased] advertises deleted mTLS mesh (also M-064).
10. `docs/PERFORMANCE-S77.md:40-63` nonexistent `e2ePacketTrace`/`--only`/`ci-bench`/"18 ms p99"; `docs/BENCHMARKS.md:96` + CLAUDE.md:829 "AF_XDP 920K pps" (Python sendto ceiling).
11. S24-handoff-battle-orders "WIRED" + shield-ebpf main.rs:270 "populated by userspace": both false (M-180 code fix separately).
12. `docs/compliance/control-matrix/gdpr`/ccpa PII rows MAPPED to deleted `DataIsolationConfig`; matrices cite Suricata/old engine/"Sentinel daily daemon" → May-2026 snapshot banner (M-108).
13. `docs/VISION.md` (≠ root VISION.md) nonexistent adapters/"25 microservices"/wrong draft names; `docs/GEMINI.md` dead make targets, :8080; `docs/README.md` lists 12 of ~50 subdirs, misdescribes runbooks/.
14. `docs/skills/README.md` + `unheaded-sentinel.md` point at nonexistent `.skills/`; Suricata listed; `docs/potential-skill-adds.md` absorbed → archive.
15. `docs/runbooks/*.md` 5 K8s/Vault prose runbooks for infra not deployed; `vault-seal.md` + dangling SecretProviderClass → delete or decide; wiki/Runbooks.md caveat.
16. CLAUDE.md: `main/staging/feature/` line :1180; Age 2 "remaining" lists shipped items (WireGuard, Foundation-06, Sophia-03, Wotan-03), "RAFT QLoRA Mistral-7B", "Zhen Engine"; Alpha criteria prune to true remainder; §Metrics `service` label wording; Doom "move out before public".
17. ADR statuses: ADR-060 → Accepted/Implemented (8623c864); ADR-031 → Superseded-by-104/106; ADR-045 shipped; ADR-015 Supersede (Fiber never started); ADR-040 Deferred; ADR-022 Pi-hole already in LXD; ADR-092 west shipping deployed; ADR-107 chaos unit installed; ADR-094 `NewServiceMetrics` note stale; ADR-067 Decision 3 PerCPU deviation; ADR-023 Superseded-by-074; ADR-004:25 commercial framing.
18. UPC master plan stale REMAINS: 1.2.2 byte oracle done (K-2); upc-regression.sh:113 "future enhancement" (K-15); `references/marshal-parked-2026-05-06.md` C4 resolved, C2 moot; ROAD_TO_LINUX L6 rungs + UCLINUX/FUZIX docs superseded by ADR-081; wiki L6 rung text; `docs/doom/DOOM_IMPLEMENTATION.md` audio_init refs.
19. `docs/decisions/2026-04-29-track-call.md` template never filled (Track C 2026-05-11); demo-script/DEMO_VIDEO_SCRIPT dormant under Track C (dead commands); `docs/talks/EBPF_SUMMIT_2026.md` built on a 52-byte option that never existed; TALK-ABSTRACT numbers; `docs/SUBMISSION_SUMMARY.md` file list -06/-03; `docs/dev/branch-hygiene-2026-05-06.md` unresolved + wrong paths (CLAUDE.md:1122,1146 same).
20. `docs/network/INGRESS_EGRESS_PORTS.md` gRPC 50051-50067 "In Use", `docs/monitoring/TELEMETRY_STACK.md` vLLM/ebpf-exporter/VM topology → rewrite against pkg/ports + docker/telemetry, fold FND-024; ARCHITECTURE.md Future Enhancements → ADR pointers; AI_STACK/ZHEN_ARCHITECTURE/EXTERNAL_DEPENDENCIES Mistral/vLLM stale; KINGDOM_ARCHITECTURE VXLAN/Vault/mTLS "(future)" → Sleipnir/not planned/ADR-103; SERVICE_BREAKOUT_STRATEGY retire under ADR-047; FUTURE_INTEGRATIONS Suricata checklist; IPV6_METRIC_CAPACITY superseded; dashboard-gap-analysis historical; frr-isis TODO marker done; `sbom-verify` target wording; MARSHAL-COOKBOOK 11-gate vs 8; RFC-ALIGNMENT-FINDINGS rows MON-04/06/21/22 stale.
21. Wiki: ~70 ADR pages reachable only via ADR-Index; Service-Wotan admin-key section (after M-030); Librarian skill gets a "Security Considerations" checklist line (ADR-043:288).

## 6. Counts
| Epic | Items | | Epic | Items |
|---|---|---|---|---|
| E01 CI green | 8 | | E09 Champion/Zhen | 13 |
| E02 Public-repo security | 12 | | E10 Protocol/RFC desk | 10 |
| E03 Live-service correctness | 13 | | E11 Supply chain/sweep | 10 |
| E04 Service auth + TLS | 7 | | E12 Shield/eBPF truth | 9 |
| E05 Release pipeline | 8 | | E13 Consensus/drift | 6 |
| E06 Well as truth | 8 | | E14 UPC non-Doom | 12 |
| E07 Observability/SRE | 11 | | E15 Doom (LAST) | 7 |
| E08 Compliance | 10 | | **Total M-items** | **144** |

Decisions for Stevie: 57 numbered (≈ 70 individual questions). Parked groups: 10. Doc-hygiene entries: 21 (one Librarian epic). Marshal removals: 13 items closed by commits/decisions since the sweeps' sources were written (listed in §3). Sweep rows consolidated: 133 ADR + 101 PLANS + 111 SEC + 44 PROTO + 33 UPC + 55 INT = 477 → 144 M-items + 21 doc fixes + 10 parked groups.
