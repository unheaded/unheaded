# Unheaded Documentation

**Configuration management automation platform.**

Unheaded is a configuration management automation platform built on eBPF-based observability, declarative infrastructure, and an IPv6 protocol extension for per-packet metadata. User brings their app ("the head"), we provide everything else.

## Directory Guide

Generated from `ls docs` on 2026-10-27; one line per subdirectory.

| Directory | Contents |
|-----------|----------|
| `adr/` | Architecture Decision Records; index `adr/ADR-INDEX.md` |
| `architecture/` | Component architecture (AF_XDP, AI stack, Kingdom, Zhen, Fae chambers) |
| `archive/` | Historical documents kept for reference, incl. `internal-2026/` (S-era battle plans and session logs) |
| `bare-metal/` | West/east host boot sequences, WireGuard tunnel, bare-metal verdict |
| `battle-plans/` | Sprint battle plans; current index `battle-plans/NEXT-7-SPRINTS-2026-10-27.md`; `progress/` logs |
| `binder-book/` | Study notes: assembly, BGP, binary/hex, BPF and HTTP/3 |
| `branch-audits/` | Branch audits from 2026-04-27 |
| `compliance/` | Compliance audits, control matrices (2026-05-06 snapshots), SBOM and license audit |
| `conference/` | Talk abstract and outline (unverified numbers) |
| `decisions/` | Track-call decision record (2026-04-29) |
| `demo/` | Demo video script (dormant) |
| `dev/` | Branch-hygiene notes |
| `doom/` | Doom-over-UPC docs, plus ASCEND-LINUX rung docs (ROAD_TO_LINUX, UCLINUX, FUZIX) |
| `gamedays/` | Chaos game-day reports and template |
| `history/` | Session handoffs from January 2026 |
| `infrastructure/` | AF_XDP deployment, load balancers |
| `labs/` | Kubernetes lab plan and sessions |
| `launch/` | S37/S73 release checklist and readiness report (historical) |
| `legal/` | IP inventory, IANA registration plan, IETF Note Well review, license docs |
| `lore/` | Naming and mythology behind component names |
| `monitoring/` | Telemetry stack |
| `network/` | Ports, firewall topology, IPv6 address plan, Monad HbH firewall rules |
| `philosophy/` | Essays |
| `planning/` | Older planning notes, release notes, repo-split plan |
| `policy/` | Gate baselines read by `scripts/check-*.sh` (dependencies, images, PQ KEX, hadolint) |
| `protocol/` | Monad / Sophia / Wotan specifications and drafting history |
| `research/` | Research notes (RAG, embeddings, bare-metal QA, verdicts) |
| `runbook-briefs/` | Briefs for runbooks still to be written |
| `runbooks/` | Five prose runbooks for a Kubernetes/Vault deployment that is not running; live runbooks are `runbooks/**/*.yaml` at the repo root |
| `sbom/` | SBOM deltas and tool status (May 2026) |
| `security/` | Security assessments, Lich fuzzing setup, gitleaks baseline manifest, PQ host escalations |
| `sessions/` | A few remaining session notes; most moved to `archive/internal-2026/sessions/` |
| `skills/` | Notes on the Claude Code skills (canonical copies in `~/.claude/skills/`) |
| `specs/` | Spec status snapshots and the UPC reference manual |
| `sprints/` | Sprint summaries and handoff indexes |
| `sweeps/` | Doc and source sweeps; master list `sweeps/OUTSTANDING-2026-10-27.md` |
| `talks/` | eBPF Summit talk draft (built on a research proposal; numbers unverified) |
| `templates/` | Post-incident review template |
| `workflow/` | Agent operating procedure |

## Key Documents

- [ARCHITECTURE.md](ARCHITECTURE.md) -- Full system architecture
- [../VISION.md](../VISION.md) -- Project vision
- [protocol/](protocol/) -- Protocol specifications and drafts
- [EXTERNAL_DEPENDENCIES.md](EXTERNAL_DEPENDENCIES.md) -- Third-party dependency inventory
