<!--
SPDX-License-Identifier: GPL-3.0-or-later
Copyright (c) 2024-2026 Stevie Bellis.
-->

# Next seven sprints (2026-10-27)

Forged from a sweep of every file under `docs/` (755 files) for work that was
planned on paper and is not in the code at `develop` `a7fe3acd`. Six scoped
sweeps (ADRs, battle plans, security and compliance, protocol and
architecture, UPC, internal and misc) produced 477 rows; each row was
verified against the code, tests and git log before it counted. A
consolidation and Marshal re-grounding pass merged them into 144 items in 15
epics, 57 decisions for Stevie, 10 parked groups and 21 doc fixes, and
removed 13 items that commits since 2026-10-20 had already closed. The full
list with sources is [`docs/sweeps/OUTSTANDING-2026-10-27.md`](../sweeps/OUTSTANDING-2026-10-27.md);
the six raw sweeps are under `docs/sweeps/outstanding-2026-10-27/`.

Order is Stevie's (2026-10-27): CI green and promotion readiness first, then
dated security findings and everything the public repo implies, then
correctness in live services, then platform epics, then UPC, and Doom last.
Every plan has its own PREFLIGHT, exit gates, [DECIDE] defaults and
[ESCALATE] steps; the decisions below are the ones that need Stevie.

| # | Sprint | Plan | Size | Epics | Status |
|---|---|---|---|---|---|
| 1 | Clean Slate: CI green, live-service bugs, public-repo hygiene, docs tell the truth | [S-CLEAN-SLATE](S-CLEAN-SLATE-battle-plan.md) | 9 phases, 288 steps | E01, E02, E03, doc hygiene | forged 2026-10-27 |
| 2 | Iron Gate: service auth (ADR-103), TLS on every internal link, signed releases and an apt repo | [S-IRON-GATE](S-IRON-GATE-battle-plan.md) | 17 phases, 402 steps | E04, E05, M-201 | forged 2026-10-27 |
| 3 | Well Spring: the Well as source of truth, SRE remainder (ADR-100), compliance paperwork and enforcing mode | [S-WELL-SPRING](S-WELL-SPRING-battle-plan.md) | 16 phases, 294 steps | E06, E07, E08, M-200/203/205 | forged 2026-10-27 |
| 4 | Champion Hardening: tool-call path, runbook approvals, eval, Zhen units | [S-CHAMPION-HARDENING](S-CHAMPION-HARDENING-battle-plan.md) | 16 phases, 252 steps | E09 | forged 2026-10-27 |
| 5 | True Wire: spec-vs-code rulings, RFC Editor desk, Shield data-plane truth, supply chain | [S-TRUE-WIRE](S-TRUE-WIRE-battle-plan.md) | 13 phases, 404 steps | E10, E11, E12 | forged 2026-10-27 |
| 6 | Ascend 2: getpid flip, N-way isolation, loader gates, required-before-prod, EVOLUTION-1 Stage 2, east as a node | [S-ASCEND-2](S-ASCEND-2-battle-plan.md) | 14 phases, 314 steps | E14, M-202/204 | forged 2026-10-27 |
| 7 | Doom Last: docs tell the truth, Doom units off root, doom.mbc provenance, one banding pass | [S-DOOM-LAST](S-DOOM-LAST-battle-plan.md) | 9 phases, 104 steps | E15 | forged 2026-10-27; runs only after 1-6 or in an idle window |

Sprints 2, 3 and 4 depend only on 1 and can run in parallel worktrees.
Sprint 5 wants 2's identity pieces for two of its items and says which.
Sprint 6 wants 5's verifier ratchet first. Sprint 7 waits for all of them.

## Verified before forging (2026-10-27, on develop at `a7fe3acd`)

- `go build ./...` and `go mod tidy -diff` clean. Local develop equals
  `origin/develop`; all 61 re-signed commits are pushed and show `G`.
- CI on `a7fe3acd`: four red gates. Dependency baseline lists four Go
  modules that left the build (shrink-only `--update` fixes it). Pinned
  tools: syft and grype are past their cooldown. Runbook pytest fails on
  GitHub only; it passes under the local venv, a clean venv from the
  CI-pinned requirements, and a depth-1 clone. "Gates can actually fail"
  is cancelled at its 25-minute timeout, not failed.
- The nightly drift-detect job on `main` has failed five nights running.
- Both haproxy configs fail `haproxy -c`.
- The findings register has 14 open FND ids, not the 4 the handoff named.
- The repo is public; 20 credential fingerprints from history are still
  only baselined.
- Timeline body and its three mirrors are current to 2026-10-26; the
  hand-written header and footer lines are stale (Sprint 1 fixes them).
- Six branches `sprint/*` are fully merged into develop; nothing to merge.

## Decisions for Stevie, by sprint

Defaults are the sweeps' recommendations where one exists. A plan step
tagged [DECIDE] proceeds with the default; a step tagged [ESCALATE] stops
its chain until answered.

### Sprint 1, Clean Slate

1. Nightly drift-detect on main: delete, dispatch-only, or self-host?
   Default: delete (terragrunt was never deployed).
2. "Gates can actually fail": the plan measures once and sets the timeout
   to the measured time plus 20 %. Override if you want it split.
3. Promote develop to staging (1062 commits) on your word; then the first
   staging to main promotion with a signed tag; set GitHub's default
   branch to develop, protect main, turn on secret scanning and push
   protection.
4. Credential rotation: you rotate the baselined set (OPNsense API,
   Grafana, LXD trust, tokens in the 20 history fingerprints); the plan
   shrinks the baseline after.
5. FND-001 (due 10-30): a Gmail account and app password for the SMTP
   relay so alerts reach someone.
6. FND-011 (due 11-18): apply the ebtables modprobe stopgap on both
   hosts; keep west's unprivileged user namespaces open for LXD until
   tested? Default: stopgap yes, userns stays open until tested.
7. E2 sshd hybrid-only key exchange on west and east (lockout risk) and
   E3 the first live WireGuard PSK rotation plus the weekly timer:
   go or no-go each.
8. Send the EAR TSU notification email.
9. Relicense the roughly 290 MIT-header non-Go files and fix LICENSE
   line 1? Default: yes, mechanical, skipping any file deliberately MIT.
10. FND-024: start the monitoring stack from kingdom-startup.sh, or fold
    Prometheus and Alertmanager into the main compose file? Default:
    kingdom-startup (keeps the stacks separable).
11. FND-023: client probe between refreshes, or Cuirass publishes health
    transitions on Wotan? Default: client probe (no new topic).
12. An east deploy window for rebuilt debs, the wotan.yaml auto-approve
    entry and the huginn baseline.
13. Pull the GitHub job log for the runbook pytest failure (run
    40426721063); reading logs needs auth.

### Sprint 2, Iron Gate

14. ADR-103 Q1 issuer placement: own `unheaded-auth` service
    (recommended) or inside dashboard-backend.
15. ADR-103 Q2 lab IdP: built-in issuer (recommended) or Authentik or
    Keycloak.
16. ADR-103 Q3 tokens: Ed25519 now with a measured switch to ML-DSA-65
    (recommended), or ML-DSA from day one.
17. ADR-103 Q4 API-key store: the Well (recommended).
18. Live release runner: Jenkins or GitHub Actions? Default: GitHub
    Actions (Jenkins is not in LIVE-PATHS).
19. Build-key custody (E4) and the remediation-snapshot key: where it
    lives, who holds it, how `build.pubkey` is published.
20. E5 Heimdall drift key custody per node.
21. ADR-004 admissions: aws-lc-rs and anthropic-sdk-go.

### Sprint 3, Well Spring

22. Apply migrations 030-034 live as one batch (ADR-106 Q1).
23. E1 Well PostgreSQL 16 to 18 with `ssl_groups` (downtime).
24. Ratify retention policy v1; sign the three attestations after
    reviewing the policy, the IR plan and the access list.
25. Watchdog placement: west/east cross-watch or an outside service?
    Default: cross-watch now, outside later.
26. Postmortem threshold (ADR-100 Q3). Default: any visible outage over
    an hour or any data-loss near miss.
27. Vector container stdout: Docker socket proxy or journald log driver?
    Default: journald driver (no socket exposure; amends ADR-092).
28. The 13 dead alerts: which metrics should they watch, or delete?
    Default: delete, re-add with a producer.
29. journald caps SystemMaxUse=1G RuntimeMaxUse=200M (east first); chmod
    0640 the two world-writable ROCm logs on west.
30. Topology map and Muninn (ADR-076/086): still wanted now that Vector
    exists? Default: re-scope to Vector config; retire the Muninn daemon.
31. Enforcing-mode Q5 (refuse starts vs installs); MFA scope; external
    pentest; host AV/IDS stance.
32. Accept the built Proposed ADRs: 093, 094, 101, 104, 105, 106, 107.

### Sprint 4, Champion Hardening

33. Approve the 11 draft runbooks; record "Claude authors, Zhen executes"
    in ADR-024/106, or wait for a local model to clear the 0.85 bar.
34. ADR-106 Q2 first unattended night: when, the cap, the env file.
    Q6 ADR-019 Level 2 amendment: accept or reject. Q7 fold the two
    routers? Default: keep both, documented. Daily cap default $5,
    max_tokens 16000.
### Sprint 5, True Wire

35. Bandit groups: skip B310 (guarded), 0.0.0.0 bind in the Zhen UI keep
    or bind lxdbr0, pin the HF revision, defusedxml yes or no.

36. Flow-action wire values: spec wins (renumber code) or erratum?
    Default: spec wins (the E4 precedent).
37. Anamnesis event types: spec wins? Default: spec wins.
38. RFC Editor items 1-5: the plan runs the RFC Editor pass in-repo;
    you accept ADR-108 after.
39. IETF datatracker upload of the six -00 drafts (browser) and the
    Track call record.
40. ADR-095/096 register thresholds (age, importers, CVE); lib/pq keep
    and record or own a client; yaml.v3 successor; cilium/ebpf own or
    record.
41. ADR-090 Q1 vendored scope, Q2 docs/ in scope, Q3 the 29 ignored forge
    tests restore or delete.
42. Delete words: pkg/waf's 7 packages (amend tests/s77), lxd/, host-a
    and host-b stacks, internal/bpfmap and pkg/ebpf/maploader,
    cmd/ebpf-exporter.
43. PQC_SIG_STATUS: wire OPTIMISTIC, delete, or lift the Layer-1 freeze?
    Default: delete until keymgmt is un-parked.
44. The 8 G9 control planes with no binary: un-park which, delete the
    rest? Default: keymgmt only if 43 wires it; delete the rest.
45. AF_XDP: product path or retire Decision 5? Default: retire.
46. Is Kubernetes still a deploy target? Decides every parked k8s row.
    Default: lab-only, not a target.

### Sprint 6, Ascend 2

47. getpid numbering: xv6 `p->pid` (init = 1) or the BPF slot (init = 0).
    Default: kernel numbering (EVOLUTION-1 direction).
48. Re-read EVOLUTION-1 D1 reach against the now-negative verifier delta
    (ON 911,319 vs OFF 926,542).
49. East as a node: approve Steps 177/435, use the netns fallback on
    west, or drop Phase 18 for good.
50. DOT4 ISA sprint? Accept interpreter-cost gates #1(a) and #2? Scorer
    on real traffic or keep it a demonstrator? Default: accept the gates,
    retrain the scorer on steel-echo flows, no DOT4 yet.
51. FS writes route: in-BPF (92.7 % of the verifier wall) or kernel-side
    via Stage 2. Default: Stage 2.
52. unheadnix naming ceremony with Lore; when Phase 2.5 (L6) starts.
53. One attended run: browser xterm end to end and the Lich Phase 5 live
    local-model e2e.

### Sprint 7, Doom Last

54. Doom stays in the repo (the "move out before public" line is
    superseded). Doom is the lowest priority of any work: Sprint 7 runs
    only after Sprints 1-6 or in an idle window you name.

### Standing and parked

55. Sleipnir Phase 1 ("deferred to Q4 2026", which is now): start or
    re-defer with a date. Yggdrasil task #65 first real build.
56. GCP cost alarms (ADR-058), NetBox on west (ADR-033): console time.
57. tools/ standalone extractions still wanted? zhend Layer 0, Sabatons
    IPMI/Redfish, ADR-064 Wotan active/active: still parked unless you
    say go.

## Parked (standing decision that parks each)

- LoRA and forge: Sprint 2 Phase 12 stays DEFERRED-on-data. Trigger:
  `zhen-flywheel export` reports 300 or more SFT train examples across 3
  or more families. First step when it fires: the per-eval-sequence
  `answer_start` fix, before any "learned" claim. Sprint 1's capture fix
  for routed local turns feeds the trigger.
- ADR-064 Wotan active/active, zhend Layer 0, Sabatons, ADR-088
  Kubernetes and every k8s row (decision 46), tools/ extractions
  (decision 57), ADR-087 NOC, ADR-058, ADR-033.
- Sophia draft-04 items behind the ratified DEFER; Kingdom Mode bits on
  the wire (needs an unfreeze); FN-DSA/HQC (no library); CRC-32C
  (rejected).
- G9/F6 parked eBPF apps until decision 44. Zhen Python to Go UI port
  (Stevie's exercise). WAVE14 retrain (superseded by the Phase 12
  trigger).
- Trigger-gated by their own plans: Sleipnir Phase 1, Yggdrasil #65,
  unheadnix L6 Phase 2.5, guest networking, code-store B/C, CSR forms,
  guest-plugin seam, DISTRIBUTED pgd handle.
- PQC parked ideas (ML-DSA X.509, CNSA 2.0, Rosenpass, age X25519,
  PSK-less peers): a future PQC sprint. Repo split, multi-tenant,
  multi-cloud, SIEM/IDP and competitor-parity wishlists: no ADR, so
  drop or ADR first.
- Doom stretch (sound, multiplayer, performance beyond playable): inside
  Sprint 7 only if Doom UX becomes a goal.

## Removed as already closed (Marshal pass, commits since 2026-10-20)

Well database pointer split (now a deliberate ops/app split per ADR-101);
bootstrap Node install via curl; the rotten ci-protocol workflow;
services/canary; FND-021 and FND-022; the repo public flip; Heimdall C4
TODOs; kingdom-shutdown's monitoring straggler; the Champion symlink check;
Doom PC corruption versus the RET floor; Phase 1.3 E/F; S5 live keyboard;
the ASCEND fork decision (ADR-081).
