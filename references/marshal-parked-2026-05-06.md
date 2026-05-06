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

