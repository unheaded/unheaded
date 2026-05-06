# Marshal Parking Lot — 2026-05-06

Items detected during the overnight run that fall **outside** Appendix A scope, are blocked on a human, are too risky to attempt unattended, or surfaced as adjacent work the Marshal redirected. Each entry is the Marshal's note for Micromanager / the relevant skill owner to triage in the morning.

Format: `### [SCOPE-CLASS] Title` → why parked, suggested next step, who owns it.

---

### [TOOLING-GAP] Install scancode-toolkit + syft + cyclonedx on dev hosts
**Why parked:** A1 needed a full SBOM regen but none of the tools are installed on this Darwin host; installing them unattended is out of scope (would touch system Python / Homebrew). The 2026-04-04 scan ran on `Linux-6.17.0-19-generic-x86_64` — likely WEST or EAST. **Suggested next step:** `brew install syft` (or pin a specific version) and either install scancode in a venv or document the CI-only path. **Owner:** MoatGhost.

### [CI-GATE-PATCH] verify-gpl-boundary.sh has 3 real bugs (now in CE1 scope)
**Captured during:** A1.
1. Uses `grep -P` (PCRE) which BSD grep doesn't support → silent failure on macOS, reports `0/1189` SPDX coverage.
2. Final exit code is always 0 even when `RESULT: FAIL` is printed → CI gate would not fail on contamination.
3. Flags first-party Unheaded crates as "GPL/AGPL contamination" (`crates/zhend`, `crates/zhenai-forge`, `crates/doom-runner`, `cmd/ebpf-loader`, `ebpf/`, `ebpf/af-xdp/` are all GPL-licensed by design).

**Status:** Promoted into Phase A.5 scope (CE1). If CE1 ships clean tonight, **un-park.** Otherwise this entry stays for daytime work.
**Owner:** MoatGhost + Marshal.

### [SBOM-CADENCE] Full ScanCode regen is overdue by 2 days
The 2026-04-04 scan is 32 days old; ADR-052 / S37 cadence is ≤30 days. Trigger a CI/Linux full re-scan within the next ~7 days.
**Owner:** MoatGhost.

### [STUCK] C2 — aya 0.1.1 ELF map upgrade requires Linux
**Captured during:** Phase C step C2 (NORTH-STAR Appendix A).

The kanban-tracked `ebpf-aya-upgrade-mn05` upgrade goal is ELF-loader compatibility with `bpftool 7.7+` / `libbpf 1.7+`. Verifying that goal requires:
1. A Linux host with bpftool ≥ 7.7 and libbpf ≥ 1.7 installed.
2. `bpf-linker` available (Rust nightly + LLVM matching aya's pinned version).
3. Cross-compilation to `bpfel-unknown-none` actually exercising the kernel verifier, OR ability to load against a running kernel.

This unattended Marshal run is on **Darwin 25.4.0 arm64** — no `bpftool`, no `libbpf`, eBPF programs cannot load. Best-case darwin verification = Rust syntax check, which would not actually validate the upgrade goal and would risk a false-green CI signal if committed.

**Per Skip Protocol** (3× budget = 6h hard ceiling, plan-side acknowledgement: *"may surface kernel gotchas"*) — flagged STUCK and skipped tonight. **No code changes attempted.**

Current pinned versions:
- `ebpf/Cargo.toml`: `aya-ebpf = "0.1"` (resolves to `0.1.0` in `ebpf/Cargo.lock`)
- `ebpf/Cargo.toml`: `aya-log-ebpf = "0.1"`
- Userspace `cmd/ebpf-loader/Cargo.toml`: `aya = "0.13"` (already current)

**Suggested next step (daytime, on a Linux host):**
1. `cargo update -p aya-ebpf -p aya-log-ebpf` to fetch the latest 0.1.x patch.
2. `cargo build --release` against the `ebpf/` workspace using `bpf-linker`.
3. `bpftool prog load` smoke against a running kernel.
4. Run the existing `scripts/bpf-verifier-check.sh` to confirm instruction-budget unchanged.
5. Update `cmd/tools/anamnesis-lite/README.md` to remove the "0.1.1 upgrade pending" note when verified.

**Owner:** Developer (with bare-metal Linux access — WEST or EAST).

### [STUCK] C4 — heimdall-daemon: 4 TODOs all architectural / Linux-only
**Captured during:** Phase C step C4 (NORTH-STAR Appendix A).

The plan said "stub-with-comment and park if architecturally non-trivial." Going through all four:

**Line 72** — `// TODO: verify attached GungnirSeal before trusting the manifest.`
- API exists: `gungnir.Verify(payload, seal, pubKey)` in `pkg/gungnir/gungnir.go`.
- Missing decisions: (a) seal-file serialization format (YAML sidecar? protobuf? embedded?); (b) public-key discovery (flag? config file? env? TPM?); (c) "no seal found" behaviour (PoC: warn vs production: hard-fail).
- Wiring this requires defining the dev-vs-prod key-management UX. **Architectural.**
- **Suggested:** Stevie to decide seal-format + key-flag convention; ~30m to wire after that.

**Line 135** — `// TODO: wire to actual Wotan client with ML-DSA-65 signing`
- Drift events publish to `drift.detected.<node_id>`. CLAUDE.md notes ML-DSA-65 is enforced on `config.*` topics today (`services/wotan/internal/signing/`). The drift topic family is **not yet covered** by topic signing — bringing heimdall under signing requires extending the signing config and the Wotan-side enforcement. **Architectural** (cross-component design decision).
- **Suggested:** Architect + Developer pair. Decide: extend `config.*` signing to `drift.*`, OR define a new signing class. Then wire the client.

**Line 147** — `// TODO: wire BPF ringbuf reader (Aya userspace) for event-driven scanning`
- Requires Linux kernel + Aya userspace + the `crates/heimdall-bpf/` kprobe scaffold loaded against a real kernel. Cannot be developed-then-verified on Darwin. **Linux-only.**
- **Suggested:** Phase as a follow-on to the C2 aya 0.1.1 upgrade (same hardware constraint).

**Line 148** — `// TODO: wire Gjallarhorn XDP listener for UPC trigger packets`
- XDP requires Linux kernel network stack and `pkg/gjallarhorn/`. **Linux-only.**
- **Suggested:** Same hardware as TODO #147; pair them in a single bare-metal session.

**Net:** Zero TODOs closed tonight. All 4 promoted to next daytime session with bare-metal Linux access (WEST or EAST). No code changes attempted.
**Owner:** Developer + Architect (key-mgmt + Wotan signing extension), with bare-metal time on WEST/EAST for the BPF/XDP TODOs.

