<!--
SPDX-License-Identifier: GPL-3.0-or-later
Copyright (c) 2024-2026 Stevie Bellis.
-->
# ADR-115: The smallest install a shop owner can run ("Unheaded Shop", name TBD)

**Status:** Proposed (pitch, parked under ADR-112). Brainstorm before acceptance; nothing is built.
**Date:** 2026-10-30
**Grounded on:** `churn/2026-10-30` at `d453db56`
**Related:**
- VISION.md "Who It Is For"
- ADR-114 (auth, the gate, option D)
- ADR-113 (verdicts)
- ADR-088 (substrate ladder)
- ADR-026 (debs)
- ADR-69420 (Yggdrasil)
- ADR-098 step 6 (host firewall)
- ADR-107 (chaos)
- third-party policy of 2026-10-29

## The pitch

Captain, 2026-10-30, after Stevie's audience note: *"the Kingdom today is 36 containers plus a protocol stack; what's the smallest install a shop owner could actually run?"*

Stevie: *"review and expand on that last statement/note and roll into an ADR"*. Two framing notes from him the same evening:
- *"not a promise just an aid"*
- *"their app shouldn't work if they configure it poorly"*

## Framing

**An aid, not a promise.** Unheaded cannot guarantee anything about an app it did not write. It can:
- make secure best practice the default around that app;
- refuse to put the app live when its setup is unsafe;
- say plainly what it covered and what it did not.

Every user-facing word in this install follows that rule. It does not say "protected" or "can't be hacked". It says what was checked, what was turned on, and what remains the owner's job.

**Scope (Stevie, 2026-10-30):** out of the box, with all compliance controls enabled, Unheaded hardens the application's **hosting**. It "can't stop application vulnerability": it makes a flaw harder to reach, limits what an exploited app can do, and records it. Security and infra teams remain users of the same install. The Shop install is therefore the full compliance control set on a small machine, not a reduced one. What it sheds is the Kingdom's development and research stack (R1).

**Who:** the mom-and-pop shop and the vibe coder (VISION.md):
- one app, often AI-written;
- one machine or one small VPS;
- no security staff;
- little patience for configuration.

**The three outcomes the aid works against:**
- the app serving malware to its visitors;
- the app running an attacker's code (RCE);
- the app becoming a threat actor's jump box into the owner's network or other hosts.

## Context: what the Kingdom is today (measured 2026-10-30)

| Measure | Value |
|---|---|
| Compose services (main stack) | 22: 21 default + `sword` (profile `deploy`) |
| Compose services (monitoring stack) | 7 |
| Containers running before the 10-28 reboot | 36 |
| Memory ceilings, main stack default | ≈ 6.4 GiB (ClickHouse 2 G, dashboard-backend 768 M, Victoria 512 M, Grafana 512 M, …) |
| Memory ceilings, monitoring stack | ≈ 3.4 GiB (Prometheus 1 G, VictoriaMetrics 1 G, Loki 512 M, Grafana 512 M, …) |
| Host systemd units in `deploy/systemd/` | 38 (16 at `MemoryMax=512M`) |
| Programs under `cmd/` | 68 |
| Hosts | west 14 GiB RAM; east 7 GiB, 4 cores (the smaller reference machine) |
| Third-party images in the default stacks | traefik, VictoriaMetrics ×2, Postgres, ClickHouse, Vector, Grafana ×2, CoreDNS, Prometheus, Loki, Alertmanager |

**Duplication found while measuring:**
- two Grafanas;
- three metrics stores (Prometheus plus two VictoriaMetrics);
- two log stores (Loki and ClickHouse).

That should converge whatever this ADR decides. It is a separate cleanup.

**Much of the default stack serves the Kingdom's own development, not an adopter's app:**
- the skill-persona services (timeguru, captain, architect, micromanager);
- the UPC/Doom work;
- Zhen/LLM;
- chaos (ADR-107);
- multi-host deploy (Sword/Cuirass);
- BGP, BFD and WireGuard between west and east.

A shop needs none of it.

## Brainstorm (Claude's recommendations as a springboard; nothing decided)

### R1. The capability floor: what the smallest install must still include

Each row exists because it acts on one of the three outcomes. Everything else is out of the floor.

| # | Capability | Against | Kingdom piece that exists (or is designed) |
|---|---|---|---|
| F1 | **Preflight: the app does not go live until its setup passes** (R2) | all three | new; the checks reuse existing gates |
| F2 | **Gate in front of the app**: TLS with automatic certificates, security headers, request and upload limits, login + MFA for admin areas, one audit point | RCE, malware | ADR-114 option D; ACME via `golang.org/x/crypto/acme/autocert`, already in the tree, so no new module |
| F3 | **Host firewall, default deny inbound**; only the gate's 80/443 and SSH open | jump box | `deploy/nftables/*.nft` + `nft-apply-confirm.sh` (ADR-098 step 6) |
| F4 | **App egress allowlist**: the app's own uid or cgroup may reach only the destinations it declares (its database, a payment API). Everything else, including the LAN and cloud metadata at `169.254.169.254`, is dropped and logged. | jump box, malware download and C2, SSRF | new nftables rules (`meta skuid` / `socket cgroupv2`) in the same table |
| F5 | **App sandbox**: own nologin account, read-only code directory, writable data directory only, no new privileges, a systemd hardening set like the Kingdom's units | RCE blast radius, persistence | `deploy/systemd` hardening pattern, `pkg/uids`, sysusers |
| F6 | **Change detection on the app's code and web root**: a new or changed file outside a deploy is an alert. That covers a dropped webshell or an injected script. | malware, RCE persistence | Heimdall (vfs_write/execve hooks, drift events) |
| F7 | **Updates**: signed packages, automatic security updates for the OS and for Unheaded | all three | signed apt repo (S-IRON-GATE), `unattended-upgrades` |
| F8 | **Backups** of the app's data, kept off the machine when possible | recovery | Well backup pattern (ERDA) |
| F9 | **Plain-language report**: what was turned on, what preflight found, what the owner still must do, and alerts by email | all three (the human loop) | alert-mailer; a new weekly summary |

**Explicitly outside the floor (the Kingdom tier, opt-in):**
- the Monad protocol path and the eBPF tracing pipeline;
- UPC;
- Zhen;
- chaos;
- Sword/Cuirass;
- ClickHouse, VictoriaMetrics, Grafana;
- the persona services;
- BGP/WireGuard.

### R2. Preflight: "their app shouldn't work if they configure it poorly"

This is the heart of Stevie's second note. The app is started only by Unheaded. Before the first start and on every change, preflight runs a fixed list of checks, and **any failure means the app stays offline**. The owner gets one plain sentence per problem and how to fix it. There is no "ignore and continue" switch in the shop install.

Candidate checks (each mapped to an outcome):

| Check | Fails when | Against |
|---|---|---|
| Listening | the app listens on anything but the private address the gate proxies to (e.g. `0.0.0.0:3000`, which bypasses the gate) | all |
| Secrets | a password, key or token appears in the app's code, config files or committed `.env` (the gitleaks rules the repo already uses) | jump box, takeover |
| Debug and dev modes | known framework debug or dev-server flags are on (Flask/Django `DEBUG`, `NODE_ENV=development`, a dev server instead of a production one) | RCE |
| Writable code | the app's code directory is writable by the app's own account | RCE persistence, malware |
| Privilege | the app wants root, extra capabilities or a shell login | RCE blast radius |
| Egress declared | the app has no declared outbound list. Default deny then applies, so "no list" means "no outbound", never "anything". | jump box |
| Admin areas | known admin paths (`/admin`, `/wp-admin`, framework consoles) are not behind gate login | takeover |
| Uploads | upload handling allows executable types into the web root | malware, RCE |
| Dependencies | the app's lockfile is missing, or pins a version with a known-exploited vulnerability (CISA KEV, which `kev-poller` already tracks) | RCE |
| TLS | no valid certificate can be obtained for the declared domain | all (no plaintext fallback) |

Open design point: some checks are cheap and exact (listening, writable code, privilege). Others are heuristic (debug flags, secrets) and will have false positives. Heuristic findings may need an owner acknowledgement that is recorded and audited, rather than an override.

### R3. Packaging: how small, how many pieces

| Option | Shape | Pros | Cons |
|---|---|---|---|
| a. Compose subset | the current stack with most services removed | fastest to try | Docker as a dependency; still many images; third-party heavy |
| **b. One binary, one package** | `unheaded-shop`: gate + auth + preflight + firewall/egress manager + change detection + report in **one Go process**, plus the existing Heimdall eBPF hooks, shipped as a single `.deb` from the signed repo | few moving parts; one unit to harden; Go makes it easy; no new third party | a larger single process, so the internal pieces need isolation (separate uids where it matters) |
| c. Yggdrasil image | the Debian-12-based OS image (ADR-69420) with b preinstalled, for a mini PC or a cloud VM | nothing else on the box; CIS-hardened from the start | Yggdrasil is not built yet (Q4 2026+ vision) |

Recommendation: **b first, c later.** Option a only as a quick lab experiment.

### R4. Size budget (proposal, to measure against)

- Unheaded Shop itself: **≤ 256 MiB RSS** steady state, **≤ 500 MB** on disk.
- Target hosts: a 1 vCPU / 1 GiB VPS or a Raspberry Pi 4/5-class box, with east (7 GiB) as the generous ceiling.
- Install to live: **≤ 15 minutes**. The owner answers at most three questions: domain, admin email, and where the app is.
- Measured in CI on a small VM image, the same way `check-unit-ceilings.sh` and the eBPF instruction ratchet already gate.

### R5. The ladder above it

**Shop** (this ADR) → **Studio** (adds metrics, a dashboard, multiple apps) → **Kingdom** (today's full stack and the protocol). This lines up with ADR-088's substrate ladder. The same packages are installed in larger sets, never a fork. Whether the Monad protocol and eBPF tracing belong in Shop or start at Studio is the key strategic question (OQ1).

### R6. How we would know the aid works

A Lich campaign against a deliberately bad sample app (an AI-written app with a debug server, an open upload and a committed key):
- **Preflight must refuse it.** Once fixed, it may go live.
- **Then attack it:**
  - upload a webshell → blocked, or detected by F6 within a minute;
  - simulated RCE → no outbound except declared destinations, no LAN reach, no write to code;
  - attempt to host a malicious file → blocked type, or detected change.

Each result goes in the report in plain words, including the things it did not catch.

### R7. Third-party footprint

The floor needs **no new third party**: ACME from `x/crypto`, nftables and systemd already on the OS, everything else first-party. Malware *scanning* of uploads is the one capability with no first-party answer (ClamAV is the usual tool, and would be third party). Flag it for the third-party sitting rather than assuming it (OQ5).

## Open questions for the brainstorm

1. Do the Monad protocol and eBPF tracing belong in Shop, or start at Studio? The protocol is the moat (VISION.md), but a shop on a VPS may not need it, and it costs size and complexity.
2. Preflight strictness: may a heuristic finding be acknowledged (recorded, audited), or is every failure blocking? Stevie's note says a poorly configured app should not work; where is the line between "poor" and "unusual"?
3. Which app runtimes first (static sites, PHP, Node, Python, Go binaries, containers)? Each needs its own preflight knowledge.
4. Packaging: b then c, as recommended? Does Shop run on stock Ubuntu/Debian, or only on Yggdrasil eventually?
5. Upload malware scanning: none, first-party signatures only, or a third-party scanner (third-party sitting)?
6. The name. "Unheaded Shop" is a placeholder; Lore decides.
7. Converging the duplicated stores (two Grafanas, three metrics stores, two log stores). Its own ADR, but it affects Studio's size.

## Decision

None yet. Parked under ADR-112.
