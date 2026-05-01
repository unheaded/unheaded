# Tool Battle Plan Index — Community-First Doctrine Bound

**Forged**: 2026-04-30 by Warmonger swarm (10 parallel agents)
**Doctrine**: c6108fb8 in CLAUDE.md — *FREE TO USE. FREE TO SHARE. NO SELLING.*
**License**: GPL-3.0 (with Apache-2.0 / MIT for select CLIs where ecosystem reach demands)
**Source brainstorm**: `../ROUND-TABLE-2026-04-30-practical-tooling.md`

---

## Purpose

Each file in this directory is an exhaustive, numbered-step Warmonger battle plan for
extracting one tool / suite from the Unheaded monorepo and gifting it to the community as
a hardened, free, GPL-3.0 standalone artifact. Plans include: phase-by-phase steps,
verification gates, debug branches, agent matrix, emergency procedures, compliance
evidence runbooks, and 72h pre-release Lich campaigns.

These plans are the **implementation maps**. Future sessions execute them.

---

## The 10 Plans

### P0 — Publish in 6 weeks (highest impact × already 80% built)

| Plan | Status | Lines | Phases | Steps | Hard-Block |
|------|--------|-------|--------|-------|------------|
| [Mímir Drift](./mimir-drift-battle-plan.md) | REAL-METAL VALIDATED on EAST | 4,002 | 15 | ~347 | Bare-metal Phase 9/11/13 (full bootstrap, stress, gate eval) |
| [Anamnesis Lite](./anamnesis-lite-battle-plan.md) | eBPF foundation working | 3,316 | 17 | ~225 | None — drop-in adapters first |
| [Zhen On-Prem](./zhen-on-prem-battle-plan.md) | Operational, needs air-gap proof | 811 | 18 | ~545 | Kernel-level egress block validation |

### P1 — Publish in 12 weeks (deeper build)

| Plan | Status | Lines | Phases | Steps | Hard-Block |
|------|--------|-------|--------|-------|------------|
| [Gungnir Distribute](./gungnir-distribute-battle-plan.md) | Foundation built (sealed-cask works) | 5,520 | 15 | ~318 | Federation witness protocol design |
| [UPC Sandbox](./upc-sandbox-battle-plan.md) | Doom-on-Monad proves completeness | 505 | 19 | ~104 | Doom PC corruption blocker (`docs/doom/FINDINGS.md`) |
| [Sentinel Edge](./sentinel-edge-battle-plan.md) | Daily Lich loop running | 502 | 18 | ~347 | Multi-arch single-binary build + Pi reference design |

### P2 — Defer to next Round Table (research / IANA-gated / ethics-gated)

| Plan | Status | Lines | Phases | Steps | Hard-Block |
|------|--------|-------|--------|-------|------------|
| [Lich Runner](./lich-runner-battle-plan.md) | Engine works locally | 378 | 17 | ~287 | **Barrister ethics review** — autonomous adversary tool legality |
| [Wotan Cloud](./wotan-cloud-battle-plan.md) | Topic signing implemented | 3,933 | 18 | ~347 | **Foundation-06 IANA registry approval** + Wotan-03 error code taxonomy |
| [Rosetta IaC](./rosetta-iac-battle-plan.md) | Greenfield (largest build) | 6,741 | 20 | ~365 | None — ground-up plan |

### Cross-cutting bundle

| Plan | Status | Lines | Phases | Steps | Notes |
|------|--------|-------|--------|-------|-------|
| [OSS CLI Suite](./oss-cli-suite-battle-plan.md) | 9-11 sharp-edge CLIs | 2,327 | 18 | ~527 | Composable building blocks for the larger tools — also useful standalone |

**Total: ~28,000 lines of battle-plan content. ~3,400 numbered steps across 165 phases.**

---

## What Every Plan Has (the 7-gate QA matrix)

Every plan in this directory enforces:

1. **SPDX coverage 100%** — every file tagged with `SPDX-License-Identifier: GPL-3.0-or-later`
2. **SBOM clean** — ScanCode + FOSSology + ORT scan, transitive dep audit, GPL boundary documented
3. **Auth framework wired** — no `NoopAuthenticator` in release builds; APIKey or JWT only
4. **Sealed Cask reproducible build** — bit-identical artifacts proven by SHA256 binding rune
5. **Hardening baseline** — seccomp, capability bounding, RO FS, NoNewPrivileges, PrivateTmp
6. **Audit log on adopter-facing endpoints** — no telemetry phone-home; local-only by default
7. **Zero adopter-data access architecturally proven** — eBPF / champion sandbox / federation enforce it

Plus: **72h Lich pre-release campaign** required before public-release on every plan.

---

## Doctrine Compliance

Every plan in this directory was forged AFTER the Community-First Doctrine commit
`c6108fb8` (2026-04-30). Each plan ends with the affirmation **FREE TO USE. FREE TO
SHARE. NO SELLING.** Each plan uses community-oriented vocabulary throughout: *share,
publish, adopter, contribute, dogfood, gift, federate, peer, commons*. None contains
*sell, paid, monetize, customer, GTM, ACV, revenue, pricing, premium tier, upsell*.

---

## Federation Architecture (cross-cutting)

Several tools rely on **community-hosted federation** rather than a centralized SaaS:

- **Sentinel Edge** — peer-to-peer threat-intel gossip; opt-in self-hosted aggregators
- **Gungnir Distribute** — multi-witness signature federation, threshold attestation
- **Wotan Cloud** — peering across organizational boundaries via gungnir-signed introductions
- **Lich Runner** — federation requires explicit gungnir-signed peer consent token
- **Anamnesis Lite** — P2P trace export between adopter clusters
- **Zhen On-Prem** — community-shared corpus update bundles, never centralized

The pattern: every tool can run fully standalone in a single-org deployment, and ALSO
federate with consenting peers via gungnir-signed introductions. There is no central
"Unheaded Inc." server in any architecture.

---

## Execution Order Recommendation

When the time comes to build:

1. **Mímir Drift** first — REAL-METAL VALIDATED, fastest to public release, broadest compliance value, low-risk extraction.
2. **Anamnesis Lite** second — eBPF observability is the most-asked-for capability and we already have the AF_XDP foundation.
3. **OSS CLI Suite** third (parallel with #1 and #2) — the sharp-edge CLIs unblock everything else and seed the broader ecosystem.
4. **Zhen On-Prem** fourth — kernel-level air-gap proof is the killer demo for regulated communities.
5. **Sentinel Edge** fifth — warm hobbyist market, low-effort first commit to public OSS appliance work.
6. **Gungnir Distribute** sixth — supply-chain federation work, foundational for all later tools.
7. **UPC Sandbox** seventh — only after Doom PC-corruption blocker is fixed.
8. **Wotan Cloud** eighth — only after Foundation-06 IANA registries approved.
9. **Lich Runner** ninth — only after Barrister ethics review completes.
10. **Rosetta IaC** tenth — largest greenfield build; sequence last unless community demand pulls earlier.

---

## Maintenance Cadence

- Re-read each plan when the corresponding tool's source components change materially.
- Update this INDEX.md when adding new tool plans.
- Verify doctrine vocabulary on every plan amendment.
- The Librarian skill is responsible for keeping this directory in lock-step with the
  rest of the document web (see `docs/Librarian` patterns).

---

**FREE TO USE. FREE TO SHARE. NO SELLING.**
LOVE SERVE REMEMBER. PEACE AND LOVE. KGLW. <3
