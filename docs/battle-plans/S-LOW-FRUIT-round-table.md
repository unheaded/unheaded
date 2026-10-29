# Round Table — S-LOW-FRUIT (low-hanging best practice)

**Convened:** 2026-10-29 00:20 CDT · **Trigger:** post-ADR-111 sweep; "what else is cheap, high-value, reversible?" (industry reading reviewed in session; notes kept outside the repo)
**Kingdom state:** develop `1db358a0` + uncommitted ADR-111 (cmdline), lore, story. EVOLUTION-1 Stage 2 rungs 0-1 closed; Yaldabaoth live; Heimdall west→east.
**Procedure:** Sprint planning (short). Decision rule: Stevie decides.

## Measured (from the tree, 2026-10-29)

| Signal | Count | Source |
|---|---|---|
| systemd units | 39; 34 hardened (NoNewPrivileges/CapabilityBoundingSet/SystemCallFilter); **5 bare** (`kingdom-startup`, `kingdom-shutdown`, `unheaded-doom-{injector,ring,runner}`); **MemoryMax on 3** | `systemd/`, `deploy/systemd/` |
| `http.Server{}` literals | 56; **22 without `ReadHeaderTimeout`** (Slowloris) | grep, 12-line window |
| `net/http/pprof` | 1 binary (dashboard-backend) | grep |
| `GOMEMLIMIT` / `SetMemoryLimit` | 0 | grep |
| `net.IP`/`net.ParseIP` vs `netip` (non-test) | 120 vs 25 | grep |
| `NO_COLOR` honoured | unheaded-cli only; zhen-cli has `--no-color` | grep |
| Version via ldflags | only `unheaded-daemon` gets `-X main.Version`; `GO_LDFLAGS := -s -w` | Makefile:17,55 |
| `yaml.Unmarshal` sites returning the bare error (no path) | 17 | grep |
| `/health` : `/ready` : `/metrics` files | 123 : 82 : 101 | grep |
| Loggers in cmd+services | zerolog 39 · slog 20 · stdlib log 8 | grep |
| Vector sink buffering / `when_full` | not configured | `config/vector/vector.yaml` |
| man pages | none; shell completion for unheaded-cli only | tree |
| CI | govulncheck in 3 workflows, cargo-audit in 1, 18 workflows | `.github/workflows` |

## Threat register intake

**TH-2026-006 — Residual disk data across container/instance lifecycles** · Category TECHNIQUE · Severity MEDIUM · Status MONITORING · Source: public container-platform disclosure, 2026-09-24.
- Kingdom impact: Cuirass → LXD instances (project `unheaded`) created/pruned for Sword deploys; Yaldabaoth container kills. Exposure UNKNOWN until probed: does a new instance's root disk ever see a previous instance's bytes (thin-provisioned zvols / dir backend)?
- Verification: Steps 81-82. If residue found: LXD storage pool `zfs` with `zfs.use_refquota`/`volume.zfs.blocksize` and `security.*` review; or `dir` backend → switch.

**TH-2026-005 — Cross-request KV/prompt-cache leakage in shared inference engines** · Category TECHNIQUE · Severity HIGH · Status MONITORING · Source: public inference-engine vendor disclosure, 2026-10-26.
- Kingdom impact: Zhenai (`services/zhen*`, llama-server on west), privacy tiers (ADR-106 §tiers), Champion gate. Exposure: UNKNOWN until probed.
- Immediate: confirm llama-server slot count (`-np`), prompt-cache mode, and that a request from one privacy tier can never reuse a slot still holding another tier's context; set `n_ctx` ≤ what the KV pool actually holds (the configuration half of the disclosed bug).
- Verification: BlackMage probe — long prompt as tier A (near `n_ctx`), then a short prompt as tier B with a canary marker in A's prompt; any marker in B's output = CRITICAL. Steps 77-78.
- ATT&CK: T1005 (data from local system), T1080-adjacent.

## Seat sign-offs (2026-10-29 00:50 CDT)

**MoatGhost — SIGNED, with two conditions**
1. **Phase 7 amendment (audit completeness, NIST AU-4/AU-5, SOC2 CC7.2):** the journald → ClickHouse stream carries sudo/ssh/PAM audit events. `when_full: drop_newest` on that sink silently discards audit records. journald is the system of record and keeps writing regardless of Vector (the "silent syslog host" hang is a producer-side bug Vector does not have), so the correct policy is a **bounded disk buffer with `when_full: block`** for the journald/audit sink — nothing dropped, Vector catches up — plus the alert. `drop_newest` is acceptable only for metrics streams. Step 65 rewritten below.
2. `--pprof` exposes heap and goroutine dumps that can contain in-memory secrets (service tokens, ML-DSA keys). Loopback-only *and* off by default is sufficient; additionally the flag must refuse when the metrics address is not loopback (already in Step 40) and flame-graph SVGs may be committed (symbols only) but **never** raw `pprof` profiles (`.pb.gz`) — add `var/pprof/` to `.gitignore` in Step 43.
- No concern: man pages (examples use placeholder paths, no secrets), version strings (`git describe`, no host/user), systemd ceilings (availability control; record `systemd-analyze security` output under ADR-098 as evidence), GOMEMLIMIT.
- Register: TH-2026-005 above.

**BlackMage — SIGNED.** Every third-party module is a parser and an update channel we don't control: the SBOM *is* the attack-surface list. Direct Go deps today: 19 (+32 indirect). Removal priority: anything that parses untrusted input (toml, yaml, websocket) before anything that doesn't (uuid). An outside man-page generator would have put a third-party binary in the Debian build path — gone; native `--man` is zero new surface. Lich's own tools (AFL++, cargo-fuzz) stay where they are: they never ship.

**Captain — SIGNED.** "We wrote the spec and the reference implementation, and the implementation is mostly ours" is a sentence adopters and auditors can verify. A shrinking SBOM is a trust metric for the front gate, the way others post uptime. Native-first is the Purity of Interface law extended from the frontend to the whole tree; it lives in CLAUDE.md, no new ADR.

**Architect — SIGNED.** Native-first costs rewrite time once and buys reproducible builds, less verifier-adjacent surface, no toolchain drift. Decision-matrix rule: *stdlib → own package → vendored copy with a Barrister row → external module last, only with Stevie's approval on record and a removal date.*

**Developer — SIGNED.** TDD on `cmdline.Man`; `--help` and the man renderer share one flag-formatting helper so they cannot drift. A removed module is done when its tests are ported, not when it compiles.

**Micromanager — SIGNED**
- Priority: P0 Phase 0 · P1 Phases 2, 3, 1 · P2 Phases 4, 5, 6, 7 · P3 Phase 8. Order in the plan already reflects this.
- WIP: ≤ 3 phases in flight (2, 5, 6 are the safe parallel trio; 3, 4, 7 need west and go one at a time).
- Delegation: **Agent-safe** = Phases 1, 2, 5, 6, 8 (edits + grep gates, verifiable on the Mac). **Stevie only** = Phase 0 (his Mac has Go + module cache), Phase 3 Steps 31-35, Phase 4 Steps 45-47, Phase 7 Steps 67-68 (sudo on west), every `[C]`.
- Acceptance = the ratchets: four `scripts/check-*.sh` that pass on the tree and fail when broken (ADR-096 cost-to-fake), plus `go test -race ./pkg/cmdline/ ./cmd/...` green. No ratchet, no DoD.
- Sacred Law check: nothing in this sprint touches user data; pprof/flame graphs expose *our* process internals only; Vector carries system logs, not user payloads. Isolation intact.
- Timeguru notified: micro sprint, no milestone moves.

Decisions 1-3 above: **accepted by Stevie 2026-10-29 00:44** ("all minor").

## Seats

- **Captain:** Adopter-facing polish (help, man pages, errors) is the cheapest trust we can buy before the public flip. Go.
- **Micromanager:** P0 = ADR-111 build green on the Mac. P1 = timeouts, units, version. P2 = pprof/flamegraph, man pages, Vector buffer, yaml paths. P3 = netip ratchet, NO_COLOR.
- **Architect:** All items reversible; none touch the wire format. `GOMEMLIMIT` from `MemoryMax` is the one that prevents a real outage (GC thrash → OOM-kill). Go ≥1.25 already sets container-aware `GOMAXPROCS`; the memory half is still manual.
- **Developer:** Timeouts and yaml-path edits are one-liners × N; each gets a grep ratchet so they stay fixed. No new deps.
- **BlackMage:** 22 servers accept slow headers forever. pprof must stay on the loopback metrics listener and behind a flag.
- **MoatGhost:** `systemd-analyze security` gives a number; use it as the gate (≤ 4.0 "OK" for Go daemons).
- **Lore:** No new names. (A sprint named S-LOW-FRUIT fits the `S-<ADJ>-<NOUN>` register: the adjective is the proof, the noun is what we pick.)
- **Librarian:** Ripple = CLAUDE.md (one paragraph), wiki/Observability page (flamegraph + pprof), ADR-111 Consequences (man pages).
- **Timeguru:** Micro sprint; ≤ 1 attended session. No milestone moves.
- **Computermancer:** Flamegraph of the Doom tick loop is the one item that feeds EVOLUTION-1 directly (where do the 758,199 insns go?).
- **Marshal:** Scope fence: no logger consolidation (zerolog/slog/log), no `net.IP` rewrite, no new ADR beyond amending ADR-111. Those are sprints, not fruit.

## Decisions (Stevie to confirm)
1. Forge S-LOW-FRUIT as a ≤ 80-step plan; execute after `go build ./... && go test ./...` is green on the Mac with the ADR-111 patch.
2. Ratchets, not rewrites: every fix lands with a `scripts/check-*.sh` that fails on regression.
3. Man pages generated natively by `pkg/cmdline.Man` (Go stdlib). Native is always preferred in this project.
4. Reading material from outside feeds informs plans but is not recorded in the repo. Third-party packages are a last resort: none is added without Stevie's explicit approval of the need, recorded before it is pulled in; exploration tooling stays in private notes.
5. **Native first; SBOM trends down.** Every outside package or tool is attack surface and supply-chain exposure. `check-dependency-baseline.sh` already refuses additions; from this sprint every sprint removes at least one module and `check-sbom-trend.sh` fails on growth. This sprint: Phase 10 removes `gorilla/mux` natively (last release 3 years, last commit 2, org archived once and revived — the unmaintained class), adds `check-mod-integrity.sh` and a native `cmd/depaudit` + `check-dep-staleness.sh` (release age from the module proxy / crates.io; >24 months on a direct module needs a recorded decision, >36 on an input boundary is a finding), and writes `docs/policy/dependency-watch.md` for everything kept. Unmaintained is its own risk class: a dead module never gets the advisory, and a widely-used dead module is the one worth taking over. Measured gaps closed in Phase 10: `GOFLAGS=-mod=readonly` + `GOSUMDB` + `GOTOOLCHAIN=local` enforced (builds cannot edit lock files or skip the transparency log), `cargo --locked` everywhere (26 invocations were unlocked), Dependabot extended to `gomod` + `cargo` with a 14-day cooldown and no auto-merge, and `scripts/dep-diff.sh` so a bump of a stale or handed-over module is read before it is merged. `google/uuid` was weighed and kept: zero-dep, well-maintained, advisory-covered — a native rewrite would add risk, not remove it. The test for a removal is *does our replacement carry less risk than the module*, not *is it foreign*.

## Wins
- 59 binaries now share one `--help`/`--version`/error shape (ADR-111).
- Lore reconciled with five months of develop; the story has seven new movements.
- gungnir/mjolnir name collision closed.

---
*Forged at the Round Table 2026-10-29. Battle plan: `S-LOW-FRUIT-battle-plan.md`.*
