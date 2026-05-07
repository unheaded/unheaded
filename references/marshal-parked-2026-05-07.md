# Marshal Parking Lot — 2026-05-07

Items detected during the daytime unattended run that fall outside scope, are blocked on a human, or surfaced as adjacent work the Marshal redirected. Each entry is the Marshal's note for the relevant skill owner to triage.

Format: `### [SCOPE-CLASS] Title` → why parked, suggested next step, who owns it.

---

### [STUCK-RENEW] aya 0.13.x major-version migration
**Captured during:** Phase 3 (cmd/tools/ + aya patch update).

The patch-level update path is now exhausted. `cargo update -p aya-ebpf -p aya-log-ebpf` is a no-op: pinned versions already at the latest within `^0.1` semver (`aya-ebpf=0.1.1`, `aya-log-ebpf=0.1.0`). Upstream current is **0.13.x**. Closing this requires:

1. New ADR (suggest ADR-065 — `aya-major-migration`) capturing:
   - rationale (BPF map format compatibility, libbpf 1.7+ alignment, kernel verifier evolution)
   - breaking-change boundary (Cargo.toml edits to `ebpf/Cargo.toml` and userspace `cmd/ebpf-loader/Cargo.toml`)
   - smoke-test recipe (`cargo build --release` + `bash scripts/bpf-verifier-check.sh` + at least one `bpftool prog load` against a running kernel)
2. Small battle-plan (~1-2 day estimate) for the actual migration — most risk is BPF map type signature changes and any HashMap/Array helper API drift.
3. Linux verification host required (this shift had WEST; that work can resume on WEST or EAST).

The current `0.1.1` pin is functional, BPF verifier check passes at 7% budget. **Not blocking anything today.** Schedule when Captain confirms it isn't competing with WAVE14 / public-launch priorities.

**Owner:** Developer + Architect.
**Daytime time-budget:** 1-2 days once unblocked.

---

### [VET-FINDINGS] 4 pre-existing `go vet` warnings
**Captured during:** Phase 6 (broad gofmt sweep ran `go vet ./...` for verification).

These predate tonight's commits. None introduced by the gofmt sweep:

1. **`cmd/protocol-api/handlers_extended_test.go:48,50,52,54,68,70,72,74`** — 8x `assignment copies lock value to <var>: sync.Once contains sync.noCopy`. Test code saving + restoring `sync.Once` values for test isolation. Real bug (the saved Once is a copy, mutations to it are independent of the original). Two valid fixes:
   - replace direct copy with pointer save/restore (`*sync.Once`),
   - or replace `sync.Once` with a constructable wrapper that explicitly supports reset for tests.
2. **`cmd/dashboard-backend/internal/server/doom.go:98`** — `return copies lock value: cmd/dashboard-backend/internal/server.DoomState contains sync.RWMutex`. Returning a struct that embeds an RWMutex by value. Standard fix: return `*DoomState` instead.
3. **`pkg/ebpf/loader.go:2279`** and **`pkg/ebpf/loader.go:4026`** — `possible misuse of unsafe.Pointer`. Likely correct (eBPF loaders interface with kernel via raw pointers), but a quick audit to confirm bounds + lifetime would close the warnings, possibly with `// nolint:govet` annotation if intentional.

None block the build. None are introduced by tonight's churn. **Daytime:** Developer review + decide patch vs. annotate.

**Owner:** Developer.

---

### [ADR-058-ACTIVATION-GAPS] 5 architectural gaps blocking Planned→Accepted
**Captured during:** Phase 2 (P2-CI agent ADR-058 review). The Marshal Review section is appended to the ADR itself (`docs/adr/ADR-058-gcp-cost-alarm-bellis-tech.md`).

Restated here for completeness:

1. **No concrete numeric thresholds per API.** "50% / 80% / 95% of relevant free-tier ceilings" depends on which API ceilings; needs a `{api, free_tier_unit, ceiling, 50%, 80%, 95%}` table before alarm policies can be created. Recommend follow-on ADR section or `docs/security/bellis-tech-cost-alarms.yaml`.
2. **Dollar-threshold rationale asserted, not derived.** $5/$25/$100/$200 stated without arithmetic linking them to free-tier overage rates. Show the math.
3. **Kill-switch IAM scoping named but not bounded.** ADR notes "Billing Account Administrator IAM" but does not specify principal. Recommend dedicated service account `bellis-tech-killswitch@<project>.iam.gserviceaccount.com` with only `billing.resourceAssociations.delete` + Pub/Sub subscriber.
4. **No rollback / re-enable runbook stubs in-tree.** ADR references `runbooks/security/gcp-billing-reenable.yaml` and `runbooks/security/quarterly-gcp-alarm-test.yaml` but they don't exist. Per ADR-052 source-of-truth policy, must be in-tree even as stubs.
5. **Quarterly synthetic check has no calendar wiring.** Step 5 says "calendar reminder" without specifying calendar/channel. Recommend wiring via unheaded-calendar skill into `references/calendar/`.

**Owner:** Stevie (console hands) + Developer (runbook stubs + calendar wiring) + MoatGhost (math + IAM principal).
**Activation gate:** all 5 closed → graduate ADR-058 from Planned to Accepted.

---

### [CMD-WAF-AI-SLOP] cmd/waf/src/rules/mod.rs has 4+ pre-existing parse errors
**Captured during:** Phase 8 cargo fmt sweep.

`cargo fmt --check` on `cmd/waf` failed because the file has multiple raw-string and unterminated-string errors:

- **Line 143:** `r"(?i)\bor\s+[\d\w'\"]+\s*=\s*[\d\w'\"]+"` — raw string contains a `\"` sequence which terminates the raw string early and leaves `]+\s...` as garbage tokens. Rust 2021 also flags `'\"]+` as a reserved prefix.
- **Line 153:** Same pattern with `\band` instead of `\bor`.
- **Line 190:** `r"(?i)<\s*meta[^>]*http-equiv\s*=\s*['\"]?refresh"` — same broken `'\"`.
- **Line 530 (test code):** `let request = create_test_request("/api/users", Some("page=1&limit=10"));` — apparently triggers an unterminated double quote string error in cargo-fmt's parser; needs investigation.

**Marshal attempted a mechanical fix** on lines 143 and 153 (changed `r"..."` to `r#"..."#`). After that fix, parser advanced past line 143 and surfaced **11 additional errors** downstream — the file has more breakage than just the raw strings. Marshal reverted the fix and parked the whole file.

**Context:** `cmd/waf` was last touched in `7b16ba9d` (Wave 1 / S51 / S59 hardening), and the parent commit `b843ae56` is literally titled *"Mountain of AI slop that needs work and cleaning"* — Stevie's own self-aware tag. This file has not been built or exercised in CI since.

**Suggested daytime work:**
1. Decide whether `cmd/waf` is still in scope for the kingdom or should be removed (it is referenced from QUICKSTART.md, ADR-002, architecture docs, but apparently not actively built).
2. If keeping: a Developer + BlackMage pair (the file is a WAF rule engine — security-relevant) needs to fix the raw strings, fix line 530, run `cargo check` until green, add it to a CI build target so it stays green.
3. If dropping: remove the file, archive any salvageable rule patterns to a separate rules-spec doc, update the doc references.

**Marshal recommendation:** option 2 (keep). The file's intent is real (SQLi + XSS pattern matching — security-valuable) even if the prose is slop. ~2-4h of careful Developer time should land it green.

**Owner:** Developer + BlackMage.

---

### [GPG-AGENT-TIMEOUT] gpg-agent pinentry timed out on autonomous commit
**Captured during:** Phase 2 first commit attempt.

```
gpg: signing failed: Timeout
[GNUPG:] PINENTRY_LAUNCHED 75130 curses 1.3.1 /dev/pts/1 ...
fatal: failed to write commit object
```

Per `feedback_unsigned_commits_when_afk.md`: switched to `--no-gpg-sign` for all 4 session commits. **Not a blocker tonight, but a consistent dev-host issue:** every Claude unattended session on this host will hit this unless the gpg-agent socket / pinentry config is fixed.

**Suggested daytime:** Either (a) configure gpg-agent for non-interactive signing on this host (`pinentry-mode loopback` + cached passphrase via systemd-tmpfiles), or (b) document the `--no-gpg-sign` fallback in CLAUDE.md so future agents don't burn cycles diagnosing.

**Owner:** Stevie / dev-host config.

---

### [CARRY-FORWARD from `marshal-parked-2026-05-06.md`]

The following parking-lot items from yesterday's shift remain open. None of them were actioned tonight (most are architectural / require Stevie's decision):

- **TOOLING-GAP** — Install scancode-toolkit + syft + cyclonedx on dev hosts (MoatGhost).
- **SBOM-CADENCE** — Full ScanCode regen overdue by 2-3 days now (MoatGhost).
- **C4 heimdall-daemon TODOs (4)** — Lines 72 (GungnirSeal verify, architectural — key-mgmt UX), 135 (Wotan ML-DSA-65 signing, cross-component drift-topic scope), 147 (BPF ringbuf reader, Linux-only Aya kernel scaffolding), 148 (Gjallarhorn XDP listener, Linux-only). Now that this Marshal session was on Linux, lines 147/148 are *technically* unblockable, but both still need architectural design work (RingStatus shape for ringbuf, signing scope decision for XDP listener wiring) — not unattended-safe.
- **D4 `crates/zhend/src/jing/pilgrimage.rs`** — 3 roadmap notes; intentional design intent, leave as-is until Architect sequences.
- **D5 `crates/zhend/src/pu/codec.rs`** — `encode_for_gossip` stub; wire-format versioning decision needed.
- **D6 `crates/doom-runner/src/main.rs:624`** — `ring::status` action stub; needs RingStatus struct designed before any Linux + Aya runtime verification can land.

**Marshal recommendation:** carry these to next shift unchanged. None are blocking. Schedule a 30-minute Stevie + Architect session to triage them all in one pass — the architectural decisions are small but they unblock concrete code work on multiple fronts.

---

## End of parking lot.
