<!--
SPDX-License-Identifier: GPL-3.0-or-later
Copyright (c) 2024-2026 Stevie Bellis.
-->

# S-TRUE-WIRE BATTLE PLAN: 13 Phases, 404 Steps

**Date**: 2026-10-27
**Sprint**: Sprint 5 of 7 (index: `NEXT-7-SPRINTS-2026-10-27.md`), TRUE WIRE. Spec and code agree on every wire value and registry; the RFC Editor desk clears its S5 queue so ADR-108 can be accepted; the Shield data plane does what its docs claim or the docs stop claiming it; the ADR-012 verifier-growth ratchet exists; the supply-chain registers and licence headers are complete.
**Prerequisite**: branch based on `a7fe3acd` (develop, 2026-10-27) or newer with S-CLEAN-SLATE's exit gates passed (CI green on develop; `go build ./...` and `go mod tidy -diff` clean, both verified at `a7fe3acd` on 2026-10-27). S-IRON-GATE (service auth, internal TLS) is recommended first: Steps 300-304 (signed Sophia deltas distributed through the ADR-103 key layout) are tagged [AFTER-IRON-GATE] and are skipped, not STUCK, while it is undone. Passwordless sudo on west for the eBPF load gate (`scripts/check-ebpf-loads.sh` exits 2 without it, by design). `xml2rfc` at `~/.local/bin/xml2rfc`. Root-gated steps run on west only; east is never touched by this plan.
**Target**: `docs/protocol/RFC-EDITOR-REQUESTS-S5.md` has every item ruled with draft text in the drafts themselves; `pkg/protocol/bpfschema`, `ebpf/monad-common` and foundation-06 carry the same flow-action and event-type values with an old-parser compatibility test proving the change is safe; Shield's HMAC, retry-token, hop-validator, GOAWAY and authority checks run inside `bpf_loop` with a userspace writer for their maps, or every doc that says "WIRED" says "stubbed" instead; `scripts/check-ebpf-insns.sh` fails CI on more than 10 % verified-instruction growth per program variant against a committed baseline; every first-party `.rs` file has an SPDX header and CI checks it; the ADR-096 register has rows for `gopkg.in/yaml.v3`, `cilium/ebpf` and `goldmark`; the ADR-090 ledger has run its step 1. The ascend and Doom insn counts (926,542 / 761,580) and the MBC byte-identity gate are unchanged, or the new numbers are recorded with the reason in the same commit.
**Estimated Duration**: 38-48 h of execution across 8-10 sessions. Seven waits on Stevie (E1 ADR-108 acceptance, E2 wire-value renumber confirmation, E3 delete table, E4 relicense, E5 supply-chain register sign-offs, E6 datatracker upload, E7 ADR acceptances); none blocks more than its own chain.
**Agent Strategy**: Coordinator owns Phases 0, 5, 7 (root for the load gate, iterative verifier work) and 12. Phase 1 (RFC desk) is [P] with Phase 3 (bpfschema hygiene); Phase 4 (error registry) is [P] with Phase 9 (SPDX); Phase 8 (FND-020 + Sophia signing) is [P] with Phase 10 (supply-chain registers). Phase 2 (renumber) must finish before Phase 5 (Shield) because Shield emits event types. Each parallel agent works in its own git worktree off the sprint branch (Steps 24-26). Hosts are read-only for every agent; the only host-touching steps are the BPF load gates, which load and never attach.
**Commit Cadence**: every 5 steps (max(3, min(5, 404/20)) = 5), at every exit gate, before every STUCK skip, after every red-to-green test. Never commit red. Never push (Stevie pushes). Signed if `git commit-tree -S` succeeds in Step 6, otherwise `git commit --no-gpg-sign` (memory: feedback_unsigned_commits_when_afk) and `scripts/resign-unpushed.py` later. Every commit message ends with the two trailer lines in Appendix C.
**Stuck Protocol**: skip after 3x the step's `~time` or 2 failed debug attempts; write the STUCK marker into this file, commit, continue with the next step that does not depend on it (Appendix B dependency map). Phase exit gates are hard stops for their own chains only.

**Builds on (cited, not re-derived):**

| Source | What this plan takes from it |
|---|---|
| `docs/sweeps/OUTSTANDING-2026-10-27.md` (master list, develop a7fe3acd) | Every item here is an M-id from epics E10, E11, E12 and decisions 33-42; Appendix D maps each to its steps. |
| `docs/protocol/RFC-EDITOR-REQUESTS-S5.md` | Items 1-5, their registries, policies and the draft text already written for the reviewer. Phase 1 rules on them in place. |
| `docs/protocol/draft-bellis-unheaded-protocol-foundation-06.md` §Monad Flow Action Registry (:1239-1249) and §Anamnesis Event Types (:1317-1325) | The spec values: FORWARD = 0x00; EVENT_BORN 0x00, EVENT_COMPUTED 0x01, EVENT_WOTAN_RD 0x02, EVENT_WOTAN_WR 0x03, EVENT_CHAOS 0x04, EVENT_ROLLBACK 0x05, EVENT_DIED 0x06, EVENT_KEY_OP 0x07, EVENT_ANOMALY 0x08. |
| `docs/protocol/RFC-ALIGNMENT-FINDINGS.md` row 5 | "Flow action values off-by-one from RFC (Forward=0x01 vs RFC 0x00), Major" — open since the S77 alignment pass. |
| E4 decision (2026-10-26, `NEXT-5-SPRINTS-2026-10-23.md` "Decisions taken") | Precedent: when spec and code disagree on a wire value, the spec wins and the code moves (EVENT_CHAOS 0x04 / EVENT_ANOMALY 0x08, commits 6ae71c3f, 8397b41e). Phase 2 applies the same rule to the remaining two disagreements. |
| ADR-108 (Proposed) D-C4, D-C5 | The cartridge registry key and the Anamnesis sub-kind that items 1 and 2 register. |
| ADR-012 (Accepted) | The promise: measure the verified instruction count and gate growth. The gate shipped as `scripts/bpf-verifier-check.sh` estimating bytes/8 against a 900K budget; ascend verifies at 926,542 (S5 Phase 0), above the budget the ADR named. Phase 7 builds the ratchet the ADR described. |
| `scripts/check-ebpf-loads.sh` + `cmd/ebpf-load-check` (Rust) | The only honest verifier gate: loads every program (never attaches), membership-compared against `docs/policy/ebpf-load-known-failing.txt`. Phase 7 extends it to record `verified N insns` per program and variant. |
| `scripts/upc-regression.sh` | The MBC byte-identity gate and the ascend / Doom / cartridge insn numbers (926,542 / 761,580 / cart_xdp 42,743 / cart_tick 146,897, S5 close). Every eBPF step re-runs it. |
| `ebpf/cartridge-ebpf/src/main.rs:276-540` | The `bpf_loop` + `#[inline(never)]` callback pattern (S5 Phase 11) that Phase 5 reuses for Shield's deferred checks. |
| `ebpf/shield-ebpf/src/main.rs:268-273, 505-545, 455-480` | The deferred-check comment ("populated by userspace; validation logic will be enabled once bpf_loop() is integrated"), the GOAWAY/error-counter/authority stubs, the debug TC stats. |
| `docs/security/shield-ebpf-pqc-fast-path-unwired-2026-08-03.md` | `PQC_SIG_STATUS` has no producer and `pqc_fast_path_check` no caller; option 3 (leave) stops being acceptable once internet-exposed. The repo is public (ADR-095:24). |
| `docs/UNREACHED-GO-PACKAGES.md` (regenerated 2026-10-26) | G8 reference packages (17, `pkg/protocol/*`), G9 control planes (10), F6 `services/loadbalancer`; the regeneration command (:28-29). |
| `docs/sweeps/REAL-STEEL-DECISIONS.md`, `REAL-STEEL-UNREACHED.tsv` | E1's verdict format (one table Stevie answers once); FLAGGED rows that this plan's E3 table re-asks with new evidence. |
| ADR-090 (Accepted), `docs/sweeps/TOTAL-SOURCE-SWEEP.tsv` | Ledger rules (:82-98): status ∈ PENDING/SWEPT/SKIPPED/DEFERRED, step 1 marks generated/vendored SKIPPED; 2192 rows PENDING at a7fe3acd. |
| ADR-096 (dependency gate), `docs/policy/dependency-baseline.txt` | "A new line is a reviewed admission: a person named it." Phase 10 adds register rows, never baseline lines. ADR-004 :104-105 admissions pending sign-off. |
| ADR-062 :115, :199 | Mandatory delta signatures would reject every unsigned delta today; the severity ladder is implicit and "a separate document under docs/security/severity-ladder.md would slot in cleanly". |
| FND-020 (`compliance/findings/register.yaml:362`) | `replication.proto` carries no signature, key or signed_at; a signed event replayed from a peer reaches subscribers unsigned. |
| `pkg/wotansig` (ADR-043 #2, ADR-105) + `pkg/crypto/pqc` | `Canonicalize`, `TopicSigner.Sign`, `VerifyEvent`, `MLDSASigner`/`MLDSAVerifier`: the signing primitives Phase 8 reuses for WAL replication and Sophia deltas. |
| `docs/security/bandit-disposition-2026-08-03.md` :130-132 | Why B310 (35), B104 (2), B615 (2), B405/B314 (4) were left to a person: boundary check already fixed for B310; bind address and HF revision are deployment facts. |
| `.github/workflows/ci.yml:157-177` | `spdx-check` scans `*.go` only. 71 of 232 first-party `.rs` files have no header (measured 2026-10-27, excluding `target/`, `llama.cpp/`, `doom/`, `crates/xv6-mbc/upstream/`). |
| Code read 2026-10-27 at a7fe3acd | Every path:line in SITUATION. `pkg/protocol/errors`, `settings`, `lifecycle`, `tlv`, `sophiasync`, `pkg/afxdp`, `services/keymgmt`, `cmd/pqc-verifier`, `internal/bpfmap`, `pkg/ebpf/maploader`, `cmd/ebpf-exporter` all still present; `pkg/protocol/ringpath`, `goaway`, `cmd/keymgmt` do not exist. LICENSE line 2 already reads "Stevie Bellis" (the "Steven" item is closed). `check-hadolint.sh:29` already pins by digest (the "latest-alpine" item is closed). |

---

## LEGEND

```
[B]            Bash command (run directly)
[V]            Verification step (MUST pass before proceeding)
[D]            Debug step (only if the prior step fails)
[W]            Write/create file
[R]            Read/inspect file
[S]            Sudo/elevated privileges required
[P]            Parallelizable with other [P] steps in the same phase
[P:N]          Parallelizable with Phase N
[SEQ]          MUST be sequential
[C]            Commit checkpoint
[ENV]          Environment setup/check
[BUILD]        Build/compilation step
[TEST]         Test execution
[CODE]         Code implementation (test first: red, then green)
[DESIGN]       Design decision recorded in-repo
[STUCK]        Step skipped via Skip Protocol (needs human)
[BLOCKED]      Blocked by an upstream [STUCK] (never used for decisions)
[DECIDE]       Decision with a pre-seeded RECOMMENDATION; proceed without asking
[ESCALATE]     Needs Stevie. STOP this chain; continue independent chains
[AFTER-E<n>]   Runs only once E<n> is answered; skipped (not STUCK) until then
[AFTER-IRON-GATE] Runs only once S-IRON-GATE's exit gates have passed; skipped until then
[PREFLIGHT]    Hypothesis verification (Phase 0 only)
[REGEN]        Regenerate derived artifacts
[AUDIT-UPDATE] Mark a finding resolved in its tracking doc
[DOC-UPDATE]   Update downstream docs (wiki, CLAUDE.md, next.md)
[SECURITY]     Security review step
[COMPLIANCE]   License/SBOM/dependency check
[BARE-METAL]   Requires the real hosts (west/east), real kernel, real LXD
[LIVE-RO]      Live read-only verification on west/east (GET, list, show)
[LIVE-RW]      Live host-changing step; only under an answered [ESCALATE]
[SPEC]         Touches a draft, a registry or a wire value: RFC Editor review step required in the same phase
```

**[DECIDE] format**: RECOMMENDATION, Rationale, "Override ONLY if". The agent proceeds with the recommendation.
**[ESCALATE] format**: Context, the exact question(s), "Why no recommendation" (or why the call is Stevie's even with one). The agent stops that chain only.
**Time tags**: `~Nm` is expected wall-clock. 3x over = Skip Protocol.

**DoD-STD (every phase's Definition of Done includes these; Micromanager gate):**
1. Every step executed or STUCK-logged in this file.
2. Every [V] in the phase passed.
3. Zero NEW test failures versus the KNOWN FAILURES BASELINE (`go test ./...`, plus `-race` on touched packages; `cargo test` on touched crates).
4. Commit checkpoint made; tree clean (`git status --short` empty).
5. No new dependency without a Barrister/ADR-004 check (`go mod tidy -diff` empty, `scripts/check-dependency-baseline.sh` green, `cargo tree` diff reviewed for Rust).
6. Attack surface unchanged, or the change is listed in Phase 11's review table.
7. Every claim of "real" in a commit message has a [V] step that proved it, cited by step number.
8. Zero user data access: no step reads packet payloads, log contents of other services, or user data; the stress cannon and lab netns carry synthetic packets only.
9. **Wire rule**: no Monad wire byte, MBC opcode or registry value changes outside a [SPEC] step that cites its [DECIDE]/[ESCALATE] and its RFC Editor review step; `scripts/upc-regression.sh` byte-identity passes or the new hash is recorded with the reason in the same commit.

---

## SITUATION (verified at a7fe3acd, 2026-10-27)

**Protocol spec vs code**
- Flow action: foundation-06 :1249 and sophia-dictionary-03 :665 say `0x00 FORWARD (REQUIRED)`; `pkg/protocol/bpfschema/core_maps.go:95-99` says `ActionForward = 0x01 … ActionDrop = 0x05`. RFC-ALIGNMENT-FINDINGS row 5 recorded this as Major and it was never closed.
- Anamnesis event types: foundation-06 :1317-1325 has nine values 0x00-0x08; `ebpf/monad-common/src/lib.rs:183-199` has `Birth = 0x01, Hop = 0x02, Death = 0x03, Chaos = 0x04, Anomaly = 0x08`; `pkg/protocol/bpfschema/core_maps.go:139-143` mirrors the Rust. E4 (2026-10-26) moved only CHAOS and ANOMALY to the spec's values. Birth/Hop/Death still sit on the spec's COMPUTED/WOTAN_RD/WOTAN_WR codes; the spec has no "Hop" type and the code has no BORN (0x00) or DIED (0x06).
- `FlagK0`/`FlagK1` at `core_maps.go:80-81` are documented as "Kingdom Mode"; `ebpf/monad-common/src/lib.rs:166-172` and `pkg/monad/flags.go:14-15` call bits 1:0 `CUSTOM` and `RSVD`. `KingdomNormal..KingdomReserved` (:85-90) describe a 2-bit field the spec does not have.
- Four CRC-16/CCITT implementations: `pkg/protocol/encoding/encoding.go:211`, `pkg/protocol/integrity/hmac.go:89`, `pkg/ebpf/anamnesis.go:279`, `pkg/cartridge/loader_linux.go:441`.
- `docs/protocol/error-registry.md` lists 13 codes (0x00-0x0C) + RESERVED + greasing; `pkg/protocol/errors/errors.go` has `UNHD_*` constants (e.g. `UNHD_SETTINGS_TIMEOUT = 0x0004`) that do not match the registry's names or numbers, and nothing imports the package (G8). Rust has no error-code enum; `shield-ebpf/src/main.rs:525` uses `monad.hop_count as u16` as the error code.
- Wotan-03 (`ietf-submission/draft-bellis-unheaded-wotan-memory-00.md:59,139-149`) defines a structured error code (severity, origin, category, detail). No Go type implements it. `services/wotan/cmd/wotan/main.go:374` handles only `SIGINT`/`SIGTERM`: no `SIGHUP` reload of the topic config.
- RFC Editor S5 items 1-5 are all `open`; draft text for items 1-3 exists in the requests file; nobody has reviewed anything since 2026-10-23. ADR-108 stays Proposed on them.
- Six `-00` drafts exist under `ietf-submission/` (foundation, sophia-dictionary, wotan-memory, mbc-isa, pqc-authentication, shim) with `make validate` (`xml2rfc --check`). There is no `draft-bellis-unheaded-anamnesis-00` and no protocol-overview draft. Nothing has been uploaded to the datatracker.
- `pkg/protocol/sophiasync/sophiasync.go:439-440` "signs" a dictionary delta with a bare SHA-256 (`computeSignature`); sophia-dictionary-03 :566, :613 requires ML-DSA-65. `pkg/crypto/pqc` has `MLDSASigner`/`MLDSAVerifier`; `pkg/wotansig` already signs Wotan events with them.

**Shield data plane**
- `ebpf/shield-ebpf/src/main.rs:268-273`: HMAC_KEYS, RETRY_TOKENS, HOP_VALIDATORS lookups "deferred to reduce verifier instruction count … validation logic will be enabled once bpf_loop() is integrated". The maps exist (:91, :97, :103). No Go code references `unhd_hmac_keys`, `unhd_retry_tokens` or `unhd_hop_validators` outside `pkg/protocol/bpfschema` and the parked `pkg/ebpf/maploader`; `cmd/shield/main.go` opens no BPF map. "Populated by userspace" is false.
- :513-519 GOAWAY check looks up `GOAWAY_STATE` and does nothing with the result. :535-540 authority check looks up `AUTHORITY` and discards it. :521-532 error counter keys on `hop_count`.
- :455-480 `try_shield_tc` increments `STAT_TC_ENTRY`, `STAT_TC_IPV6`, `STAT_TC_HBH` on every packet (three map ops) — debug counters in the production object.
- `PQC_SIG_STATUS` is declared in shield (`main.rs` ~:886) with `pqc_fast_path_check` never called; `cmd/pqc-verifier` is unreached; `services/keymgmt{,/handler,/lifecycle,/store}` is unreached (G9). `wiki/PQC-Authentication.md:24` says Shield "reads cached verification status from the PQC_SIG_STATUS map".
- `ebpf/packet-marker/src/main.rs:190-199, 214-216`: TCP and IP option trace-ID extraction disabled with `TODO(E1)` "re-enable when aya-ebpf supports BPF subprograms (#[inline(never)])". `ebpf/cartridge-ebpf/src/main.rs:372,385,454` uses `#[inline(never)]` callbacks under `bpf_loop` today, so the stated blocker is gone.
- `docs/archive/internal-2026/battle-plans/S24-handoff-battle-orders.md` says the HMAC/retry/hop checks are WIRED.
- `docs/UNREACHED-GO-PACKAGES.md:136-137` and the live regeneration (2026-10-27) list `services/{compliance,firewall,nfv,qos,selfheal,version,keymgmt(+3),loadbalancer,sabatons}` and `services/wotan/internal/compute` as reached by no binary; `pkg/afxdp` is reached by no binary; `internal/bpfmap`, `pkg/ebpf/maploader`, `cmd/ebpf-exporter` are FLAGGED/parked rows in REAL-STEEL-UNREACHED.tsv.

**Verifier ratchet**
- `scripts/bpf-verifier-check.sh:9,121-123`: `BUDGET=900000`, `EST_INSNS=$((SIZE / 8))` — a byte-size estimate. ADR-012 :5 records "monad-cpu-ebpf measured at ~70K instructions" (2026-04-03). `scripts/upc-regression.sh:66` greps the kernel's `verified N insns (x% of 1M ceiling)` line: ascend 926,542 and Doom 761,580 at S5 close. The gate ADR-012 described (fail on growth) does not exist; `check-ebpf-loads.sh` records pass/fail only.

**Supply chain and licences**
- 71 first-party `.rs` files lack `SPDX-License-Identifier`; `ci.yml:164-176` checks `*.go` only.
- `grep -rl 'SPDX-License-Identifier: MIT'` outside vendored trees: 39 `nix/modules`, 32 `containers`, 31 `ansible`, 23 `services`, 17 `hosts`, 11 `firewall`, 9 `dark-mirror`, 6 each `wireguard`/`profiles`/`frr`/`cerydwyn`/`bare-metal`, more; `docs/legal/LICENSE-MIT.md` is the only legal note that mentions MIT (the Suricata GPL-isolation note was deleted with Suricata). LICENSE line 2 already reads "Stevie Bellis".
- `docs/adr/ADR-004:104-105`: `anthropic-sdk-go` and `aws-lc-rs`/`aws-lc-sys` "owner sign-off pending" since 2026-10-23. ADR-096 §Progress names `gopkg.in/yaml.v3` (archived upstream, 35 prod files), `cilium/ebpf` (8 files) and `goldmark` as unrecorded.
- `docs/sweeps/TOTAL-SOURCE-SWEEP.tsv`: 2192 rows PENDING; ADR-090 :98 step 1 (generated/vendored → SKIPPED) never run.
- `.github/workflows/static-analysis.yml:197-201`: bandit `continue-on-error: true`, skips `B603,B607,B404,B311,B605,B101`; 43 findings left (B310 ×35, B104 ×2, B615 ×2, B405/B314 ×4). `security.yml:319-345` trivy `scanners: vuln` and `secret` with `exit-code: '1'`; no misconfig scanner. `check-hadolint.sh:29` pins `hadolint/hadolint:v${VERSION}-alpine@${DIGEST}` (the "latest-alpine" item is closed).
- `.golangci.yml:99` excludes `G115` wholesale (~410 sites per the ADR sweep); `ST1000/1003/1016/1020-1022` excluded (:83-88).
- `notebooks/requirements.txt` and `tomb/grimoire/rag/requirements.txt` have zero `==` pins; `.github/ci-python/requirements.in` → `requirements.txt` is hash-locked by `scripts/ci-python-lock.sh`. `pip-compile`, `scancode`, `cosign`, `reuse`, `cargo-deny` are not installed on west; `cargo-audit` is.
- `docs/legal/scancode-full.json` dates from 2026-04-04; 108 packages were deleted since (E1).
- `services/wotan/proto/replication.proto:26-33` `WALEntry{sequence, data, crc, node_id, timestamp, epoch}`: no signature, public_key, algorithm or signed_at (FND-020).

**Escalations in this plan**

| Id | Question (one line) | Blocks |
|---|---|---|
| E1 | Accept ADR-108 now that items 1-5 are ruled (Step 58) | nothing downstream; ADR status only |
| E2 | Confirm spec-wins renumber of flow-action and event-type wire values before the sprint branch merges to develop (Step 97) | merge of Phase 2 commits; Phase 5 proceeds on the branch |
| E3 | One delete/keep table: PQC_SIG_STATUS path, 8 G9 control planes + F6, `pkg/afxdp`, `internal/bpfmap`, `pkg/ebpf/maploader`, `cmd/ebpf-exporter`, `pkg/waf` 7 pkgs, `lxd/`, host-a/host-b stacks (Step 247) | Steps 248-257 [AFTER-E3] |
| E4 | Apply the scripted MIT→GPL relicense diff (Step 334) | Steps 335-339 [AFTER-E4] |
| E5 | ADR-096 thresholds; `yaml.v3` successor; `cilium/ebpf` record-vs-own; ADR-004 admissions; ADR-090 Q1-Q3 (Step 353) | Steps 354-360 [AFTER-E5] |
| E6 | Datatracker upload of the -00 drafts (browser, Stevie's account) (Step 76) | nothing in code |
| E7 | Formal acceptance of ADRs 093, 094, 101, 104, 105, 106, 107 (Step 398) | nothing in code |
| E8 | Shield check semantics (added 2026-10-28): what keys the HMAC / retry-token / hop-validator lookups at XDP ingress, where packets have no Monad yet, and what AUTHORITY guards at TC egress (Steps 183-205, 207 authority half) | Steps 183-205; FND-025 stays open |

---

## PHASE 0: PREFLIGHT (Steps 1-28)

**Goal**: Re-verify every SITUATION claim live, record the KNOWN FAILURES BASELINE and the eBPF insn baseline before any change, set up the sprint branch and worktrees. (Covers the preconditions of every M-id in Appendix D.)
**Prerequisite**: S-CLEAN-SLATE exit gates passed; on west; passwordless sudo available for Step 14.
**Time**: 75 min
**Agent**: Coordinator

### Environment

- [x] **Step 1** [ENV][B] ~1m: Confirm base commit and clean tree
  ```bash
  cd /home/govan/tmp/unheaded && git rev-parse --abbrev-ref HEAD && git log --oneline -1 && git status --short | wc -l
  ```
- [x] **Step 2** [V] ~1m: Branch is `develop` at or after `a7fe3acd` (`git merge-base --is-ancestor a7fe3acd HEAD && echo ok`), `git status --short | wc -l` prints `0` (the two untracked `host-agent` entries from the handoff are allowed: `git status --short | grep -v host-agent | wc -l` prints `0`). If fail → `git stash list` and STOP; do not start a sprint on a dirty tree.
- [x] **Step 3** [ENV][B] ~1m: Create the sprint branch
  ```bash
  cd /home/govan/tmp/unheaded && git checkout -b sprint/true-wire && git branch --show-current
  ```
- [x] **Step 4** [ENV][B] ~2m: Required tools
  ```bash
  for t in go cargo rustup bpftool jq python3 xml2rfc cargo-audit docker sudo; do printf '%-12s %s\n' "$t" "$(command -v $t 2>/dev/null || echo MISSING)"; done; go version; cargo --version; ls ~/.local/bin/xml2rfc
  ```
- [x] **Step 5** [V] ~1m: Every tool present except none of them MISSING; `xml2rfc` resolves (either on PATH or at `~/.local/bin/xml2rfc`; later steps use `~/.local/bin/xml2rfc` explicitly). If `bpftool` MISSING → Step 5a.
- [ ] **Step 5a** [D][S][B] ~3m: `sudo apt-get install -y linux-tools-common linux-tools-$(uname -r) | tail -2`; re-run Step 4.
- [x] **Step 6** [ENV][B] ~1m: GPG probe decides the commit flag for this session
  ```bash
  cd /home/govan/tmp/unheaded && if timeout 20 git commit-tree -S -m probe "$(git rev-parse HEAD^{tree})" >/dev/null 2>&1; then echo "SIGN=yes"; else echo "SIGN=no -> use --no-gpg-sign"; fi
  ```
- [x] **Step 7** [ENV][B] ~1m: Sudo for the load gate
  ```bash
  sudo -n true && echo "sudo: ok" || echo "sudo: NEEDS PASSWORD (load gates will exit 2)"
  ```

### Baselines (recorded, never edited by hand later)

- [x] **Step 8** [BUILD][B] ~4m: Go build + tidy
  ```bash
  cd /home/govan/tmp/unheaded && go build ./... && go mod tidy -diff | head -5; echo "tidy-rc=${PIPESTATUS[0]}"
  ```
- [x] **Step 9** [V] ~1m: `go build` exit 0 and `tidy-rc=0` with no diff lines. If fail → STOP (S-CLEAN-SLATE regressed; fix there first).
- [x] **Step 10** [TEST][B] ~15m: KNOWN FAILURES BASELINE
  ```bash
  cd /home/govan/tmp/unheaded && mkdir -p docs/battle-plans/progress && go test ./... 2>&1 | grep -E '^(ok|FAIL|---)' | sort > docs/battle-plans/progress/S-TRUE-WIRE-go-baseline.txt; grep -c '^ok' docs/battle-plans/progress/S-TRUE-WIRE-go-baseline.txt; grep -E '^FAIL' docs/battle-plans/progress/S-TRUE-WIRE-go-baseline.txt || echo "no FAIL lines"
  ```
- [x] **Step 11** [TEST][B] ~6m: Rust baselines for the crates this plan touches
  ```bash
  cd /home/govan/tmp/unheaded && for c in ebpf/monad-common pkg/../crates/upc-api crates/monad-mbc; do [ -f $c/Cargo.toml ] && (cd $c && cargo test 2>&1 | grep -E '^test result' | tail -1 | sed "s|^|$c: |"); done; (cd ebpf && cargo check --target=x86_64-unknown-linux-gnu 2>&1 | tail -1)
  ```
- [x] **Step 12** [V] ~1m: Every `test result:` line shows `0 failed`; `cargo check` prints `Finished`. Record the three lines into the progress file. If fail → the failing crate is a pre-existing failure: record it as KNOWN and continue.
- [x] **Step 13** [TEST][S][BARE-METAL][B] ~6m: eBPF load gate as it stands today
  ```bash
  cd /home/govan/tmp/unheaded && sudo -n true && scripts/check-ebpf-loads.sh 2>&1 | tail -8; echo "rc=${PIPESTATUS[0]}"
  ```
- [x] **Step 14** [V] ~1m: `rc=0` ([PASS]). If `rc=2` → sudo missing: mark Steps 13-16, 203-208, 224-230, 270-278 [BLOCKED] on sudo and continue (Appendix A.1). If `rc=1` → the known-failing list drifted on a7fe3acd: STOP, this is a S-CLEAN-SLATE regression.
- [x] **Step 15** [PREFLIGHT][S][BARE-METAL][B] ~8m: Record today's verified-insn numbers (the ratchet's seed; Phase 7 turns this into a file)
  ```bash
  cd /home/govan/tmp/unheaded && scripts/upc-regression.sh 2>&1 | grep -aE 'verified [0-9]+ insns|ascend|Doom|cart_(xdp|tick)|SAME|DIFF|ALL GREEN|FAIL' | head -14
  ```
- [x] **Step 16** [V] ~1m: Output contains `ALL GREEN`, ascend `926,542` (or the kernel's exact `verified 926542 insns`), Doom `761,580`, and `12/12 SAME`. Any other ascend/Doom number → record it in the progress file as the pre-sprint number with the kernel version (`uname -r`); the plan's gates compare against the recorded number, not the S5 one. If the harness fails → Appendix A.2.

### Hypothesis verification (one pair per SITUATION claim)

- [x] **Step 17** [PREFLIGHT][B] ~1m: Flow action and event type values in code
  ```bash
  cd /home/govan/tmp/unheaded && grep -nE 'ActionForward|ActionDrop|EventBirth|EventAnomaly|FlagK0|KingdomReserved' pkg/protocol/bpfschema/core_maps.go; grep -nE 'Birth = |Hop = |Death = |Chaos = |Anomaly = ' ebpf/monad-common/src/lib.rs
  ```
- [x] **Step 18** [V] ~1m: `ActionForward uint8 = 0x01`, `ActionDrop uint8 = 0x05`, `EventBirth uint8 = 0x01`, `FlagK0 uint8 = 0x01`, Rust `Birth = 0x01 … Anomaly = 0x08`. If any differs → someone moved first; re-read Phase 2 against the actual values before executing it.
- [x] **Step 19** [PREFLIGHT][B] ~1m: Spec values
  ```bash
  cd /home/govan/tmp/unheaded && sed -n '1249p;1317,1325p' docs/protocol/draft-bellis-unheaded-protocol-foundation-06.md; grep -c 'open' docs/protocol/RFC-EDITOR-REQUESTS-S5.md
  ```
- [x] **Step 20** [V] ~1m: `0x00 FORWARD`, the nine EVENT_* lines 0x00-0x08, and `open` count ≥ 5. If the requests file has fewer than 5 `open` → someone ruled already; Phase 1 reviews the rulings instead of writing them.
- [x] **Step 21** [PREFLIGHT][B] ~1m: Shield stubs, debug stats, PQC map, packet-marker TODOs
  ```bash
  cd /home/govan/tmp/unheaded && grep -nE 'Deferred to reduce verifier|populated by|Future: validate monotonicity|Future: check fingerprint|hop_count as u16|STAT_TC_ENTRY|PQC_SIG_STATUS|pqc_fast_path_check' ebpf/shield-ebpf/src/main.rs | head -12; grep -nc 'TODO(E1)' ebpf/packet-marker/src/main.rs; grep -rlE 'unhd_hmac_keys|MapHmacKeys' --include=*.go cmd services pkg | grep -v bpfschema | grep -v maploader || echo "no userspace writer (as claimed)"
  ```
- [x] **Step 22** [V] ~1m: All eight patterns found in shield; `TODO(E1)` count is 2; "no userspace writer (as claimed)". If a writer exists → read it before Phase 5 and shrink Steps 191-199 accordingly.
- [x] **Step 23** [PREFLIGHT][B] ~2m: Supply-chain and licence facts
  ```bash
  cd /home/govan/tmp/unheaded && echo "rs-no-spdx=$(find . -name '*.rs' -not -path '*/target/*' -not -path './llama.cpp/*' -not -path './doom/*' -not -path './crates/xv6-mbc/upstream/*' | xargs grep -L 'SPDX-License-Identifier' 2>/dev/null | wc -l)"; echo "mit-spdx=$(grep -rl 'SPDX-License-Identifier: MIT' --exclude-dir=node_modules --exclude-dir=llama.cpp --exclude-dir=target --exclude-dir=.git --exclude-dir=doom --exclude-dir=upstream . | wc -l)"; echo "ledger-pending=$(awk -F'\t' 'NR>1 && $2=="PENDING"' docs/sweeps/TOTAL-SOURCE-SWEEP.tsv | wc -l)"; grep -c 'sign-off pending' docs/adr/ADR-004*.md; sed -n '2p' LICENSE; grep -nE '^(SYFT|GRYPE)_VERSION' scripts/lib/tool-pins.sh
  ```
- [x] **Step 24** [V] ~1m: `rs-no-spdx=71` (±3), `mit-spdx` ≈ 290, `ledger-pending=2192`, sign-off pending `2`, LICENSE line 2 `Copyright (c) 2024-2026 Stevie Bellis`. Record the exact numbers in the progress file; Phases 9-10 ratchet against them.

### Worktrees for parallel agents (skip if executing solo)

- [x] **Step 25** [ENV][B] ~2m: One worktree per parallel lane
  ```bash
  cd /home/govan/tmp/unheaded && for lane in rfc-desk bpfschema errors-wotan spdx fnd020-sophia supply; do git worktree add -q ../unheaded-tw-$lane -b sprint/true-wire-$lane sprint/true-wire 2>&1 | tail -1; done; git worktree list | grep true-wire | wc -l
  ```
- [x] **Step 26** [V] ~1m: `6` worktrees listed. Each agent merges its lane branch back into `sprint/true-wire` at its phase exit gate (`git merge --no-ff sprint/true-wire-<lane>`); the coordinator resolves conflicts. If any `git worktree add` failed → remove the half-made dir and retry once; else run that lane in the main tree sequentially.
- [x] **Step 27** [V] ~2m: **PHASE 0 EXIT GATE** — Steps 2, 5, 9, 12, 14 (or its BLOCKED note), 16, 18, 20, 22, 24 all passed; baselines written to `docs/battle-plans/progress/S-TRUE-WIRE-go-baseline.txt` and the progress file. If any hypothesis failed → amend the affected phase's SITUATION line before proceeding; do not execute a phase against a false premise.
- [x] **Step 28** [C] ~1m: **COMMIT CHECKPOINT**
  ```bash
  cd /home/govan/tmp/unheaded && git add docs/battle-plans/progress/ && git commit $([ "$(git config commit.gpgsign)" = true ] && timeout 20 git commit-tree -S -m p "$(git rev-parse HEAD^{tree})" >/dev/null 2>&1 || echo --no-gpg-sign) -m "[PLAN S-TRUE-WIRE] Steps 1-28: preflight, baselines, sprint branch

Phase 0: Preflight
Steps completed: 1-28
Verification gates passed: 2, 5, 9, 12, 14, 16, 18, 20, 22, 24, 27
Next: Step 29 (RFC Editor desk)

Co-Authored-By: Claude Fable 5.1 <noreply@anthropic.com>
Claude-Session: https://claude.ai/code/session_01TzL3TuPhKAVVHnD9jzuabE"
  ```

---

## PHASE 1: RFC EDITOR DESK — S5 ITEMS 1-5, TWO NEW DRAFTS (Steps 29-78)

**Goal**: Rule on RFC Editor items 1-5 in place (M-142), put the ruled text into the drafts, add the Anamnesis-00 stub and the Protocol Overview draft (M-147), validate with xml2rfc, and hand Stevie E1 (accept ADR-108) and E6 (datatracker upload). Nothing here changes a wire byte: items 1-3 are registrations of provisional names already in code; item 4 is text; item 5 fixes a registry table that contradicts itself.
**Prerequisite**: Phase 0 exit gate. Lane: `rfc-desk` worktree.
**Time**: 5 h
**Agent**: Agent [P:3] (pure document work; the RFC Editor role)

### Item 1 — Sophia root dictionary key for the cartridge registry

- [x] **Step 29** [R][B] ~3m: Read the Sophia root-key registry as it stands and the draft text for item 1
  ```bash
  cd /home/govan/tmp/unheaded && grep -nE '^0x1[0-9A-F]|^\| *0x1[0-9A-F]|Root Dictionary Keys|First Come' docs/protocol/draft-bellis-unheaded-sophia-dictionary-03.md | head -20; sed -n '/### Item 1/,/### Item 2/p' docs/protocol/RFC-EDITOR-REQUESTS-S5.md
  ```
- [x] **Step 30** [DECIDE][SPEC] ~5m: **Root key assignment.** RECOMMENDATION: assign the next free FCFS value shown by Step 29 (the draft text proposes `0x15`; use the actual next free value), category `cartridge`, type COMPOSITE, reference ADR-108 D-C4, entry schema as drafted. Rationale: FCFS policy, the request is well-formed, the namespace string `x-cartridge` was always provisional. Override ONLY if Step 29 shows `0x15` or the next free value is already taken in a later draft revision (then take the next one).
- [x] **Step 31** [W][SPEC] ~10m: Add the registry row and a one-paragraph entry-schema note to `docs/protocol/draft-bellis-unheaded-sophia-dictionary-03.md` under the Root Dictionary Keys table (the row: `| 0xNN | cartridge | COMPOSITE | MBC cartridge registry (ADR-108) | ADR-108, this document |`), mirror it into `ietf-submission/draft-bellis-unheaded-sophia-dictionary-00.md` and `.xml` (same table), and change the item-1 Status cell in `RFC-EDITOR-REQUESTS-S5.md` from `open` to `ruled 2026-MM-DD: 0xNN cartridge (Step 31)`.
- [x] **Step 32** [V] ~2m: `grep -n 'cartridge' docs/protocol/draft-bellis-unheaded-sophia-dictionary-03.md ietf-submission/draft-bellis-unheaded-sophia-dictionary-00.md | wc -l` ≥ 4; `grep -c '| open |' docs/protocol/RFC-EDITOR-REQUESTS-S5.md` is 4.
- [x] **Step 33** [CODE][B] ~5m: The code's registry key becomes the assigned value, keeping `x-cartridge` as an alias for one release
  ```bash
  cd /home/govan/tmp/unheaded && grep -rn '"x-cartridge"\|x-cartridge' --include=*.go --include=*.rs pkg/cartridge crates/upc-cartridge cmd/upc-cartridgectl 2>/dev/null | grep -v _test | head -8
  ```
  Then [W]: add `const CartridgeRootKey uint8 = 0xNN // sophia-dictionary-03, ruled S-TRUE-WIRE Step 30` next to the first hit and use it where the string is used as a key; keep the string accepted on read. Add a test asserting the constant equals the draft row (parse the draft line in the test).
- [x] **Step 34** [TEST][B] ~3m: `cd /home/govan/tmp/unheaded && go test ./pkg/cartridge/... -race -count=1 2>&1 | tail -3`
- [x] **Step 35** [V] ~1m: `ok`. If fail → the alias path is missing: keep both until the test passes; do not drop the string.

### Item 2 — Anamnesis sub-kind for cartridge trace records

- [x] **Step 36** [DECIDE][SPEC] ~5m: **Sub-kind, not a new Event Type.** RECOMMENDATION: cartridge step records reuse `EVENT_COMPUTED (0x01)` with payload first octet `0x43`; receivers that do not know the sub-kind treat the record as opaque EVENT_COMPUTED. No new type. Rationale: foundation-06's registry policy is Specification Required and the request is exactly the "computed" semantics; a new type would cost a registry value for a payload discriminator. Override ONLY if Phase 2's event-type renumber (Step 86) makes `0x01` mean something other than COMPUTED (it does not; Phase 2 moves code to the spec, where 0x01 is COMPUTED).
- [x] **Step 37** [W][SPEC] ~10m: Add a sub-section "EVENT_COMPUTED payload sub-kinds" after the Anamnesis Event Types registry in `docs/protocol/draft-bellis-unheaded-protocol-foundation-06.md` (:1325 area) with a two-row table (`0x43 'C' cartridge step record, ADR-108`; `other: reserved, treat as opaque`), mirror into `ietf-submission/draft-bellis-unheaded-protocol-foundation-00.md` + `.xml`, and set item 2's Status to `ruled: sub-kind 0x43 under EVENT_COMPUTED (Step 37)`.
- [x] **Step 38** [V] ~1m: `grep -n '0x43' docs/protocol/draft-bellis-unheaded-protocol-foundation-06.md ietf-submission/draft-bellis-unheaded-protocol-foundation-00.md | wc -l` ≥ 2; `open` count is 3.

### Item 3 — Wotan topics `anomaly.scores` and `upc.cart.*`

- [x] **Step 39** [R][B] ~2m: `cd /home/govan/tmp/unheaded && grep -nE 'anomaly\.scores|upc\.cart' docs/protocol/WOTAN_TOPICS.md docs/protocol/draft-bellis-unheaded-wotan-memory-03.md services/anomaly/*.go pkg/cartridge/*.go 2>/dev/null | head -10`
- [x] **Step 40** [DECIDE][SPEC] ~3m: **Register the names as used.** RECOMMENDATION: add both to the Wotan Topic Namespace registry (Expert Review) with the drafted descriptions and the payload constraint "flow metadata and a score only; never packet payload". Rationale: the names are in production code and follow the existing `service.noun` convention; the constraint is the ADR-108 FIELD table. Override ONLY if `WOTAN_TOPICS.md` already reserves `anomaly.*` for another producer.
- [x] **Step 41** [W][SPEC] ~8m: Add the two rows to `docs/protocol/WOTAN_TOPICS.md` (the canonical topic table) and to the topic registry in `docs/protocol/draft-bellis-unheaded-wotan-memory-03.md` + `ietf-submission/draft-bellis-unheaded-wotan-memory-00.md`/`.xml`; drop the word "provisional" from the code comments found in Step 39; set item 3's Status to `ruled (Step 41)`.
- [x] **Step 42** [V] ~1m: `grep -c 'anomaly.scores' docs/protocol/WOTAN_TOPICS.md` ≥ 1; `open` count is 2.
- [x] **Step 43** [C] ~1m: **COMMIT CHECKPOINT** `[PLAN S-TRUE-WIRE] Steps 29-43: RFC items 1-3 ruled (cartridge root key, sub-kind 0x43, anomaly.scores/upc.cart.*)` (trailers as in Step 28).

### Item 4 — CSR byte-address range text (ADR-067 Decision 2)

- [x] **Step 44** [R][B] ~3m: `cd /home/govan/tmp/unheaded && grep -nE 'CSR|0xF000|csr\*4|12-bit' docs/protocol/draft-bellis-unheaded-mbc-isa-00.md docs/adr/ADR-067-mbc-isa-v2-and-upc-abi-v1.md | head -14; git show acf62722 --stat | head -8`
- [x] **Step 45** [W][SPEC] ~10m: In `docs/protocol/draft-bellis-unheaded-mbc-isa-00.md` (and the `ietf-submission` twin + `.xml`) add to the CSR section: "CSR `csr` (12-bit index 0x000-0xFFF) is addressed at byte address `0xF000 + csr*4`, range `0xF000-0x12FFC`; a read is `LD rd, [0xF000 + csr*4]`; a write is `ST`; a read-modify-write (`csrrw`, `csrrs`, `csrrc`) is the LD/op/ST sequence the translator emits (ADR-067 S5 implementation note, commit acf62722)." Set item 4's Status to `ruled: text added (Step 45)`.
- [x] **Step 46** [V] ~1m: `grep -n '0x12FFC' docs/protocol/draft-bellis-unheaded-mbc-isa-00.md ietf-submission/draft-bellis-unheaded-mbc-isa-00.md | wc -l` ≥ 2; `open` count is 1.

### Item 5 — KF10 opcode table conflicts and MOVI sign

- [x] **Step 47** [R][B] ~3m: `cd /home/govan/tmp/unheaded && grep -nE '^ *0x1[1-9A-B]|RESERVED|MOVI' docs/protocol/draft-bellis-unheaded-mbc-isa-00.md | head -20; grep -rnE 'MOVI|imm16' crates/monad-mbc/src/*.rs ebpf/monad-cpu-ebpf/src/*.rs 2>/dev/null | grep -iE 'zero|sign|extend|as u32|as i16' | head -6`
- [x] **Step 48** [DECIDE][SPEC] ~5m: **Table follows the implementations.** RECOMMENDATION: remove 0x17/0x18/0x1A/0x1B from the RESERVED span (list the span as the actual unassigned values) and document MOVI as zero-extending `imm16`, since both interpreters zero-extend and the byte-identity gate proves the behaviour. Rationale: Standards Action policy applies to new assignments, not to correcting a self-contradicting table; changing the interpreters would be a wire-behaviour change. Override ONLY if Step 47 shows an interpreter that sign-extends (then STOP: that is a differential bug for S-ASCEND-2, not a text fix).
- [x] **Step 49** [W][SPEC] ~10m: Edit the opcode table and the MOVI description in `docs/protocol/draft-bellis-unheaded-mbc-isa-00.md` + `ietf-submission` twin + `.xml`; add a KF10 note to `docs/protocol/mbc-isa-reference.md` if it repeats the table; set item 5's Status to `ruled (Step 49)`.
- [x] **Step 50** [V] ~1m: `grep -c '| open |' docs/protocol/RFC-EDITOR-REQUESTS-S5.md` is `0`; `grep -ci 'zero-extend' docs/protocol/draft-bellis-unheaded-mbc-isa-00.md` ≥ 1.
- [x] **Step 51** [TEST][B] ~4m: Nothing in code moved for items 4-5; prove it
  ```bash
  cd /home/govan/tmp/unheaded && git diff --stat sprint/true-wire -- crates ebpf pkg cmd services | tail -3; (cd crates/monad-mbc && cargo test 2>&1 | grep -E '^test result' | tail -1)
  ```
- [x] **Step 52** [V] ~1m: Only `pkg/cartridge` (Step 33) appears in the diff stat; monad-mbc `0 failed`.

### Validate and close the S5 queue

- [x] **Step 53** [BUILD][B] ~4m: `cd /home/govan/tmp/unheaded/ietf-submission && make validate 2>&1 | grep -E '===|Error|Warning|Clean' | head -24`
- [x] **Step 54** [V] ~1m: Every `===` block ends with `Clean` or only pre-existing `Warning:` lines (compare with `git stash; make validate; git stash pop` if unsure). Any `Error:` → Step 54a.
- [ ] **Step 54a** [D][B] ~5m: `~/.local/bin/xml2rfc --check ietf-submission/<draft>.xml 2>&1 | head -10`; fix the XML (usually an unescaped `<` or a table row with the wrong cell count); re-run Step 53. Two failures → STUCK (XML tooling), continue with Step 55 using the `.md` as source of truth.
- [x] **Step 55** [REGEN][B] ~3m: `cd /home/govan/tmp/unheaded/ietf-submission && make txt 2>&1 | tail -3 && ls -la *.txt | wc -l`
- [x] **Step 56** [W] ~8m: Rewrite the "Status at sprint close" section of `docs/protocol/RFC-EDITOR-REQUESTS-S5.md` as "Status at S-TRUE-WIRE (2026-MM-DD)": all five ruled, by whom (the in-repo RFC Editor pass, this plan, step numbers), what changed in each draft, and the one remaining action (E1). Update `docs/adr/ADR-108-mbc-cartridges.md` Status line to `Proposed (RFC Editor items 1-5 ruled in S-TRUE-WIRE Steps 29-50; acceptance is Stevie's, E1)`.
- [x] **Step 57** [C] ~1m: **COMMIT CHECKPOINT** `[PLAN S-TRUE-WIRE] Steps 44-57: RFC items 4-5 ruled, drafts validated, S5 queue closed`.
- [ ] **Step 58** [ESCALATE] ~2m: **E1 — Accept ADR-108.** Context: items 1-5 are ruled with text in the drafts (Steps 31, 37, 41, 45, 49); no Monad byte and no opcode changed (Step 52). Question: accept ADR-108 now, or keep it Proposed until the cartridges run on east (S-ASCEND-2 M-228)? Why Stevie: ADR acceptance is the owner's. Continue with Step 59.

### Two new drafts (M-147)

- [x] **Step 59** [R][B] ~5m: What Anamnesis text exists to lift into a stub
  ```bash
  cd /home/govan/tmp/unheaded && grep -nE '^#+ .*Anamnesis|AnamnesisEvent|32-byte|ring buffer' docs/protocol/draft-bellis-unheaded-protocol-foundation-06.md | head -12; grep -rn 'anamnesis-00' docs/protocol/*.md | head -3; sed -n '1,30p' ietf-submission/draft-bellis-unheaded-shim-00.md
  ```
- [x] **Step 60** [W][SPEC] ~40m: Create `ietf-submission/draft-bellis-unheaded-anamnesis-00.md` (kramdown-rfc front matter copied from the shim draft; `category: exp`): Abstract; Introduction (the per-CPU ring buffer, 32-byte event, who emits); Event Record Format (the struct from `pkg/protocol/bpfschema` `AnamnesisEvent`, offsets from `TestAnamnesisEventFieldOffsets`); Event Types (reference the foundation-06 registry, do not duplicate it); Sub-kinds (Step 37); Consumer rules (unknown type → count and drop, never crash; unknown sub-kind → opaque); Security Considerations (no payload bytes, flow metadata only; ring overflow is a drop counter, not a stall); IANA Considerations: none (registries live in foundation). Then `cp` to `docs/protocol/`.
- [x] **Step 61** [W][SPEC] ~45m: Create `ietf-submission/draft-bellis-unheaded-protocol-overview-00.md`: a reader's map of the six drafts (foundation, sophia-dictionary, wotan-memory, mbc-isa, pqc-authentication, shim) plus anamnesis: one section per draft, what it defines, which registries it owns, the layering diagram (Monad on IPv6 HbH → Sophia dictionaries → Wotan memory → Shim/UPC compute → Anamnesis events), the frozen-at-v0x01 statement, and the conformance targets (which drafts a Kingdom node, a hop, a Shield, a Wotan must implement). No new normative text.
- [x] **Step 62** [B] ~6m: Generate XML for both (the Makefile's `DRAFTS` list drives everything)
  ```bash
  cd /home/govan/tmp/unheaded/ietf-submission && grep -n '^DRAFTS' Makefile && sed -i 's/^DRAFTS *=\(.*\)$/DRAFTS =\1 draft-bellis-unheaded-anamnesis-00 draft-bellis-unheaded-protocol-overview-00/' Makefile && grep -n '^DRAFTS' Makefile && (command -v kramdown-rfc >/dev/null && make all 2>&1 | tail -5 || echo "kramdown-rfc missing: write the .xml by hand from the shim .xml skeleton")
  ```
- [x] **Step 63** [V] ~2m: `ls ietf-submission/draft-bellis-unheaded-anamnesis-00.xml ietf-submission/draft-bellis-unheaded-protocol-overview-00.xml` both exist. If `kramdown-rfc` is missing → Step 63a.
- [ ] **Step 63a** [D][W] ~30m: Copy `draft-bellis-unheaded-shim-00.xml` to each new name and transcribe sections by hand (xml2rfc v3 `<section><name>…</name><t>…</t>` blocks); keep it short; the `.md` stays the source.
- [x] **Step 64** [BUILD][B] ~3m: `cd /home/govan/tmp/unheaded/ietf-submission && make validate 2>&1 | grep -E 'anamnesis|overview|Error|Clean' | head -8`
- [x] **Step 65** [V] ~1m: Both new drafts `Clean` (or only Warnings). Errors → Step 54a procedure (max 2 tries, then STUCK on XML, keep the `.md`).
- [x] **Step 66** [W][DOC-UPDATE] ~8m: Add both drafts to `docs/protocol/IETF_SUBMISSION_GUIDE.md`'s draft list and to `docs/SUBMISSION_SUMMARY.md`; add one row per draft to `docs/legal/IANA-REGISTRATION.md` if it indexes drafts.
- [x] **Step 67** [C] ~1m: **COMMIT CHECKPOINT** `[PLAN S-TRUE-WIRE] Steps 59-67: anamnesis-00 stub + protocol-overview-00 drafts, validated`.

### G8 reference packages ruling (M-146) — the RFC Editor decides what a conformance reference is

- [x] **Step 68** [R][B] ~5m: The 17 G8 packages and what each claims to implement
  ```bash
  cd /home/govan/tmp/unheaded && for p in amplification dos errors flowtype integrity intermediary lifecycle migration prefetch registry sequence settings sophiasync tlv; do d=pkg/protocol/$p; [ -d $d ] && printf '%-14s files=%s tests=%s doc=%s\n' "$p" "$(ls $d/*.go 2>/dev/null | grep -vc _test)" "$(ls $d/*_test.go 2>/dev/null | wc -l)" "$(head -3 $d/doc.go 2>/dev/null | tail -1 | cut -c1-70)"; done; ls -d pkg/monad 2>/dev/null || echo "pkg/monad gone"
  ```
- [x] **Step 69** [DECIDE] ~10m: **Per-package ruling.** RECOMMENDATION: (a) KEEP AS CONFORMANCE REFERENCE, with a `doc.go` banner "reference implementation of <draft §>; not linked into any binary; its tests are the conformance suite", for packages whose tests encode spec rules: `errors` (after Phase 4 renames it to the registry), `settings`, `tlv`, `sequence`, `flowtype`, `registry`; (b) WIRE IN (a binary gains it in this or a named sprint): `sophiasync` (Phase 8, Sophia signing), `integrity` (its CRC16 is folded into the single implementation in Phase 3, the HMAC part stays reference); (c) DELETE (reference of nothing in the drafts, or superseded): `amplification`, `dos`, `intermediary`, `lifecycle` (GOAWAY/SETTINGS reference → Phase 4 decides keep-or-delete with Wotan-03), `migration`, `prefetch`. Rationale: a conformance reference earns its place only if a draft section points at it; the rest is dead weight the ADR-090 sweep already named. Override ONLY if Step 68 shows a package with a test that is cited by a draft or by `docs/protocol/SPEC-VS-CODE-AUDIT.md` as the oracle (then KEEP).
- [x] **Step 70** [W] ~15m: Write `docs/protocol/G8-REFERENCE-RULINGS.md`: one row per package (ruling, draft §, action, step). Add the `doc.go` banners to the KEEP set. Add the DELETE set to the E3 table (Step 247) rather than deleting here; this phase rules, Phase 6 deletes.
- [x] **Step 71** [V] ~1m: `grep -c '^| pkg/protocol' docs/protocol/G8-REFERENCE-RULINGS.md` is 14 (+1 if `pkg/monad` exists); `go build ./...` still clean.
- [x] **Step 72** [DOC-UPDATE][W] ~5m: In `docs/UNREACHED-GO-PACKAGES.md` G8 paragraph (:55), link the rulings file; mark the KEEP set "reference, by ruling" so future regenerations do not re-ask.
- [x] **Step 73** [C] ~1m: **COMMIT CHECKPOINT** `[PLAN S-TRUE-WIRE] Steps 68-73: G8 reference-package rulings`.

### Phase 1 close

- [x] **Step 74** [TEST][B] ~3m: `cd /home/govan/tmp/unheaded && go build ./... && go test ./pkg/cartridge/... ./pkg/protocol/... -count=1 2>&1 | grep -E '^(ok|FAIL)' | grep -v '^ok' || echo "all ok"`
- [x] **Step 75** [V] ~1m: `all ok`.
- [ ] **Step 76** [ESCALATE] ~2m: **E6 — Datatracker upload.** Context: eight `-00` drafts validate (Steps 53, 64); upload needs a browser login to datatracker.ietf.org with Stevie's account; `docs/protocol/IETF_SUBMISSION_GUIDE.md` has the sequence. Question: upload now (all eight), or wait for the first staging→main promotion? Why Stevie: the account and the public act are his. Record the resulting URLs in `docs/legal/IANA-REGISTRATION.md` when done. Continue with Step 77.
- [x] **Step 77** [V] ~2m: **PHASE 1 EXIT GATE** — `grep -c '| open |' docs/protocol/RFC-EDITOR-REQUESTS-S5.md` is 0; `make validate` clean for all eight drafts; `docs/protocol/G8-REFERENCE-RULINGS.md` exists; `go build ./...` clean; E1 and E6 filed. If the `open` count is not 0 → DO NOT PROCEED to Phase 2's [SPEC] steps; finish the ruling first.
- [x] **Step 78** [C] ~1m: **COMMIT CHECKPOINT** `[PLAN S-TRUE-WIRE] Steps 74-78: Phase 1 exit gate` and merge the `rfc-desk` lane into `sprint/true-wire`.

---

## PHASE 2: WIRE VALUES — FLOW ACTION AND EVENT TYPES FOLLOW THE SPEC (Steps 79-100)

**Goal**: Apply the E4 rule to the two remaining spec-vs-code disagreements (M-140, M-141): code moves to the spec's values with an old-parser compatibility test, a one-release dual-accept on the read side, and an RFC Editor review step; close RFC-ALIGNMENT-FINDINGS row 5. [SPEC] throughout.
**Prerequisite**: Phase 1 exit gate (the registries are settled). Lane: main tree (Phase 5 depends on this).
**Time**: 4 h
**Agent**: Coordinator

### Flow action values (M-140)

- [x] **Step 79** [R][B] ~5m: Every producer and consumer of `ActionForward..ActionDrop` and the Rust/eBPF equivalents
  ```bash
  cd /home/govan/tmp/unheaded && grep -rnE 'Action(Forward|Trace|Sample|Mirror|Drop)\b' --include=*.go pkg cmd services | grep -v _test | grep -v 'core_maps.go' | head -20; grep -rnE 'flow_action|FLOW_ACTION|FlowAction' --include=*.rs ebpf crates | grep -v target/ | head -12; grep -rnE 'flow_action' services/sophia/*.go | head -5
  ```
- [x] **Step 80** [DECIDE][SPEC] ~5m: **Spec wins.** RECOMMENDATION: `ActionForward = 0x00, ActionTrace = 0x01, ActionSample = 0x02, ActionMirror = 0x03, ActionDrop = 0x04` (foundation-06 :1239-1249 order; verify the exact spec sequence in Step 81 and use it), same in Rust. Readers accept both encodings for one release behind `bpfschema.FlowActionLegacy(v)` and count legacy hits in a metric. Rationale: E4 precedent; the wire byte at offset 0x05 is `exponent`-typed per foundation-06 :356 so the default-forward packet (all-zero) becomes spec-correct; today a zeroed Monad reads as "undefined action". Override ONLY if Step 79 shows a persisted store (Wotan WAL, Well table) that holds flow-action bytes (then add a one-shot migration step before Step 85).
- [x] **Step 81** [R][B] ~2m: `cd /home/govan/tmp/unheaded && sed -n '1239,1260p' docs/protocol/draft-bellis-unheaded-protocol-foundation-06.md`
- [x] **Step 82** [CODE][W] ~15m: Red: add `pkg/protocol/bpfschema/flow_action_spec_test.go` that parses foundation-06 :1239-1260 for `0xNN NAME` rows and asserts each `Action*` constant equals the spec value; add a legacy-decode table test (`0x01→Forward … 0x05→Drop` through `FlowActionLegacy`). Run: `go test ./pkg/protocol/bpfschema/ -run 'FlowAction' -count=1 2>&1 | tail -4` → expect FAIL.
- [x] **Step 83** [V] ~1m: FAIL with the current `0x01` values (the test is honest). If it passes → the test is reading the wrong table; fix the test.
- [x] **Step 84** [CODE][W] ~20m: Green: renumber the constants in `core_maps.go:95-99` with a comment citing foundation-06 and this step; add `FlowActionLegacy(v uint8) (uint8, bool)`; update every consumer from Step 79 to call it on read; Rust: add `pub mod flow_action { pub const FORWARD: u8 = 0x00; … }` to `ebpf/monad-common/src/lib.rs` next to `flags`, and change any eBPF producer found in Step 79 to use it. If no eBPF producer writes the byte today, say so in the commit message.
- [x] **Step 85** [TEST][B] ~5m: `cd /home/govan/tmp/unheaded && go test ./pkg/protocol/... ./services/sophia/... ./pkg/ebpf/... -race -count=1 2>&1 | grep -E '^(ok|FAIL|---)' | grep -v '^ok' || echo "all ok"; (cd ebpf && cargo check --target=x86_64-unknown-linux-gnu 2>&1 | tail -1)`
- [x] **Step 86** [V] ~1m: `all ok` and `Finished`. If a consumer test breaks on the legacy path → it was asserting the old value: update the assertion to the spec value and note it in the commit.

### Anamnesis event types (M-141)

- [x] **Step 87** [R][B] ~5m: Every emit and decode site
  ```bash
  cd /home/govan/tmp/unheaded && grep -rnE 'EventType::(Birth|Hop|Death|Chaos|Anomaly)' --include=*.rs ebpf crates | grep -v target/ | awk -F: '{print $1}' | sort | uniq -c; grep -rnE 'Event(Birth|Hop|Death|Chaos|Anomaly)\b' --include=*.go pkg cmd services dashboard 2>/dev/null | grep -v _test | grep -v core_maps.go | head -16; grep -rnE 'event_type|eventType' dashboard/js/*.js 2>/dev/null | head -5
  ```
- [x] **Step 88** [DECIDE][SPEC] ~6m: **Spec wins, with a name map.** RECOMMENDATION: Rust `EventType { Born = 0x00, Computed = 0x01, WotanRd = 0x02, WotanWr = 0x03, Chaos = 0x04, Rollback = 0x05, Died = 0x06, KeyOp = 0x07, Anomaly = 0x08 }`; semantic mapping of today's emitters: Shield ingress `Birth` → `Born (0x00)`; hop programs' `Hop` → `Computed (0x01)` (a hop executes the Shim step; foundation-06 :1318 "Shim executed, Monad updated"); Shield egress `Death` → `Died (0x06)`. Go mirrors with the spec names and keeps `EventBirth/EventHop/EventDeath` as deprecated aliases for one release. Decoders accept both numbering schemes for one release keyed on a `legacy` flag set per ring source (the loader knows which object version it loaded). Rationale: E4 precedent; the spec has no Hop type and never will (hop = computed); a BORN of 0x00 means an all-zero event reads as a birth, which is why the decoder must also check `timestamp != 0`. Override ONLY if the Anamnesis consumer (`pkg/ebpf/anamnesis.go`) persists raw type bytes into the Well or Wotan WAL (then add a migration step before Step 91; check with `grep -n 'event_type' db/migrations/*.sql`).
- [x] **Step 89** [CODE][W] ~15m: Red: `pkg/protocol/bpfschema/event_type_spec_test.go` parsing foundation-06 :1317-1325 and asserting each Go constant; a decoder test feeding one legacy record (type 0x01 from an old object) and one spec record (type 0x00 with non-zero timestamp) and expecting Born for both. `go test ./pkg/protocol/bpfschema/ -run 'EventType' -count=1 2>&1 | tail -4` → FAIL.
- [x] **Step 90** [V] ~1m: FAIL as expected.
- [x] **Step 91** [CODE][W] ~35m: Green: Rust enum + `impl EventType::from_u8` (unknown → None, unchanged rule); update the three emitter families (shield ingress/egress, hop, any `Hop` emitter in `monad-cpu-ebpf`/`cartridge-ebpf`); Go constants + aliases + decoder dual-accept in `pkg/ebpf/anamnesis.go` and the dashboard decode (`dashboard/js` file from Step 87, if any) + `EventType` strings in `pkg/protocol/bpfschema/core_maps.go:139-143`.
- [x] **Step 92** [TEST][B] ~6m: `cd /home/govan/tmp/unheaded && go test ./pkg/protocol/... ./pkg/ebpf/... ./cmd/ebpf-collector/... -race -count=1 2>&1 | grep -E '^(FAIL|---)' || echo "all ok"; (cd ebpf && cargo check --target=x86_64-unknown-linux-gnu 2>&1 | tail -1); (cd ebpf && cargo build --release 2>&1 | tail -1)`
- [x] **Step 93** [V] ~1m: `all ok`, `Finished` twice.
- [x] **Step 94** [TEST][S][BARE-METAL][B] ~10m: The eBPF objects still load and the UPC numbers did not move (event emission is on the same path)
  ```bash
  cd /home/govan/tmp/unheaded && scripts/check-ebpf-loads.sh 2>&1 | tail -3; scripts/upc-regression.sh 2>&1 | grep -aE 'ALL GREEN|FAIL|verified [0-9]+ insns|SAME|DIFF' | head -8
  ```
- [x] **Step 95** [V] ~2m: `[PASS]`, `ALL GREEN`, ascend/Doom insns equal to Step 16's numbers (a one-byte constant change must not move the verifier count; if it did by ≤ 0.1 % record the new number with "Step 91 event-type renumber" as the reason in the progress file and in the Phase 7 baseline; if more → Appendix A.3).
- [x] **Step 96** [DOC-UPDATE][W][SPEC] ~8m: Close `docs/protocol/RFC-ALIGNMENT-FINDINGS.md` row 5 (`Resolved S-TRUE-WIRE Step 84`), add a row for the event-type alignment (`Resolved Step 91`), and add an "Implementation note (2026-MM-DD)" to foundation-06 §Anamnesis: "implementations before S-TRUE-WIRE emitted 0x01/0x02/0x03 for birth/hop/death; decoders SHOULD accept both for one release".
- [ ] **Step 97** [ESCALATE] ~2m: **E2 — Confirm before merge to develop.** Context: two wire-value renumbers landed on `sprint/true-wire` under the E4 rule with dual-accept decoders (Steps 84, 91) and the load/byte-identity gates green (Step 95). Question: confirm "spec wins" for flow-action and event-type values so the sprint branch can merge, or revert Phase 2 and file errata instead? Why Stevie: it is the same class of call as E4, made explicit because it touches two wire bytes at once. The plan continues on the branch; only the merge waits.
- [x] **Step 98** [TEST][B] ~5m: **Backward-compat proof** — old parser on new packets: build the pre-sprint `pkg/ebpf` decoder from `a7fe3acd` in a temp dir and feed it a spec-numbered record
  ```bash
  cd /home/govan/tmp/unheaded && git worktree add -q /tmp/claude-1000/-home-govan-tmp/f267c16f-13e4-4b29-87ee-e434cae4bce1/scratchpad/tw-old a7fe3acd && (cd /tmp/claude-1000/-home-govan-tmp/f267c16f-13e4-4b29-87ee-e434cae4bce1/scratchpad/tw-old && go test ./pkg/ebpf/ -run 'TestAnamnesis.*Unknown|TestDecode' -count=1 2>&1 | tail -3); git worktree remove --force /tmp/claude-1000/-home-govan-tmp/f267c16f-13e4-4b29-87ee-e434cae4bce1/scratchpad/tw-old
  ```
- [x] **Step 99** [V] ~2m: **PHASE 2 EXIT GATE** — the old decoder's unknown-type path counts-and-drops (no panic) on `0x00`/`0x06`; Steps 86, 93, 95 passed; RFC-ALIGNMENT-FINDINGS row 5 closed; E2 filed. If the old decoder panics on an unknown type → that is a pre-existing bug worth its own finding (register it) and the dual-accept window becomes mandatory, not optional.
- [x] **Step 100** [C] ~1m: **COMMIT CHECKPOINT** `[PLAN S-TRUE-WIRE] Steps 79-100: flow-action and event-type values follow foundation-06 (spec wins, E4 rule), dual-accept decoders`.

---

## PHASE 3: BPFSCHEMA HYGIENE — FLAG NAMES, ONE CRC16, STRUCT SINGLE SOURCE OF TRUTH (Steps 101-130)

**Goal**: `pkg/protocol/bpfschema` stops describing a Kingdom Mode field the spec does not have (M-143); the four CRC-16/CCITT copies become one (M-143); the Go/Rust struct parity test derives sizes and offsets from the Rust source instead of hardcoding them (M-185); dead `unhd_*` map names go.
**Prerequisite**: Phase 0 exit gate. Lane: `bpfschema` worktree. [P:1]
**Time**: 3.5 h
**Agent**: Agent

### Flag names (M-143)

- [x] **Step 101** [R][B] ~3m: Who uses the Kingdom names
  ```bash
  cd /home/govan/tmp/unheaded && grep -rnE 'FlagK0|FlagK1|Kingdom(Normal|Priority|Experimental|Reserved)' --include=*.go pkg cmd services dashboard 2>/dev/null | grep -v 'core_maps.go' | head -12; grep -rn 'Kingdom Mode' pkg/protocol/bpfschema/*.go docs/protocol/draft-bellis-unheaded-protocol-foundation-06.md | head -6
  ```
- [x] **Step 102** [DECIDE][SPEC] ~3m: **Rename to the spec, keep aliases one release.** RECOMMENDATION: `FlagCustom = 0x02`, `FlagReserved = 0x01` as the names; `FlagK1`/`FlagK0` become `Deprecated:` aliases; delete `KingdomNormal..KingdomReserved` unless Step 101 shows a caller (then alias + deprecate). The audit comment "Per IANA: 00=NORMAL…" is replaced by the foundation-06 Flags Bitfield row. Rationale: `pkg/monad/flags.go` and the Rust already use CUSTOM/RSVD; the Kingdom Mode registry in foundation-06 is a separate (parked, wire-unfreeze) concept and must not be confused with two live flag bits. Override ONLY if a caller writes Kingdom values into the wire (then that caller is a bug: stop and register a finding).
- [x] **Step 103** [CODE][W] ~10m: Red: extend `pkg/protocol/bpfschema/core_maps_test.go` with a test that asserts `FlagCustom == monad.FlagCustom` and `FlagReserved == monad.FlagReserved` (import `unheaded/pkg/monad`) and that `FlagK0 == FlagReserved` (alias). Run `go test ./pkg/protocol/bpfschema/ -run Flag -count=1 2>&1 | tail -3` → compile FAIL (names missing).
- [x] **Step 104** [CODE][W] ~10m: Green: rename + aliases + comment in `core_maps.go:78-90`; update callers from Step 101.
- [x] **Step 105** [TEST][B] ~3m: `cd /home/govan/tmp/unheaded && go build ./... && go test ./pkg/protocol/bpfschema/ ./pkg/monad/ -race -count=1 2>&1 | tail -3`
- [x] **Step 106** [V] ~1m: `ok` twice; `go build` clean.

### One CRC-16/CCITT (M-143)

- [x] **Step 107** [R][B] ~4m: Compare the four implementations byte for byte on a corpus
  ```bash
  cd /home/govan/tmp/unheaded && for f in pkg/protocol/encoding/encoding.go pkg/protocol/integrity/hmac.go pkg/ebpf/anamnesis.go pkg/cartridge/loader_linux.go; do echo "== $f"; grep -nE -A12 'func [cC][rR][cC]16' $f | grep -E 'poly|0x1021|0xFFFF|init|xor|^\s*[0-9]+-\s*return' | head -5; done; grep -rn 'crc16\|CRC16' --include=*.rs ebpf/monad-common/src/lib.rs | head -3
  ```
- [x] **Step 108** [DECIDE] ~3m: **`pkg/protocol/encoding.CRC16CCITT` is the one.** RECOMMENDATION: keep `encoding.CRC16CCITT` (it is the Monad checksum used by the wire codec and already tested against the Rust `verify_checksum`), make the other three thin wrappers that call it (or direct call-site replacements), add a cross-check test that feeds 1,000 random buffers plus the Monad golden vectors through all four entry points and the Rust-generated vectors in `ebpf/monad-common` test data. Rationale: one algorithm, one bug surface; `integrity` is a G8 reference package (Step 69) and must not own a live primitive. Override ONLY if Step 107 shows two different polynomials or init values in production use (then there are two checksums on the wire and that is a finding, not a refactor).
- [x] **Step 109** [CODE][W] ~10m: Red: `pkg/protocol/encoding/crc16_single_test.go` with the four-way equivalence over 1,000 random buffers (seeded) and the golden vectors from `pkg/protocol/encoding` existing tests; make it fail by asserting the three other symbols are gone (`go vet` cannot; use a `grep`-based test: read the three files and assert no `func CRC16CCITT(` / `func crc16CCITT(` definition remains). Run → FAIL.
- [x] **Step 110** [CODE][W] ~15m: Green: replace the three definitions with calls to `encoding.CRC16CCITT` (import cycles: `pkg/ebpf` → `pkg/protocol/encoding` is fine; `pkg/cartridge` → fine; `pkg/protocol/integrity` → fine); delete the duplicate bodies.
- [x] **Step 111** [TEST][B] ~4m: `cd /home/govan/tmp/unheaded && go build ./... && go test ./pkg/protocol/... ./pkg/ebpf/... ./pkg/cartridge/... -race -count=1 2>&1 | grep -E '^(FAIL|---)' || echo "all ok"`
- [x] **Step 112** [V] ~1m: `all ok`. The cartridge byte-identity (`cart` hashes) is covered by Step 16's numbers; Phase 5's Step 224 re-runs the harness.
- [x] **Step 113** [C] ~1m: **COMMIT CHECKPOINT** `[PLAN S-TRUE-WIRE] Steps 101-113: bpfschema flag names follow foundation-06; one CRC-16/CCITT`.

### Struct single source of truth (M-185)

- [x] **Step 114** [R][B] ~4m: What the parity test hardcodes and what Rust declares
  ```bash
  cd /home/govan/tmp/unheaded && grep -nE 'expected|:= [0-9]+|Sizeof|Offsetof' pkg/protocol/bpfschema/parity_test.go | head -20; grep -nE 'pub const MONAD_SIZE|pub const .*_SIZE|#\[repr\(C\)\]|pub struct (Monad|AnamnesisEvent|FlowKey|FlowState)\b' ebpf/monad-common/src/lib.rs | head -12; grep -rn 'unhd_' pkg/protocol/bpfschema/bpfschema.go | wc -l; grep -rhoE 'unhd_[a-z_]+' ebpf/*/src/*.rs | sort -u | wc -l
  ```
- [x] **Step 115** [DECIDE] ~4m: **Generate the Go expectations from the Rust source at test time.** RECOMMENDATION: a `go:generate`-free approach: the parity test opens `../../../ebpf/monad-common/src/lib.rs`, extracts `pub const *_SIZE: usize = N;` and each `#[repr(C)] pub struct X { … }` field list with a small parser, computes expected offsets by C layout rules for the primitive types used (u8/u16/u32/u64/[u8;N]), and compares with `unsafe.Sizeof`/`Offsetof` of the Go structs. Map names: the test also asserts every `MapXxx = "unhd_…"` constant in `bpfschema.go` appears in at least one `.rs` under `ebpf/*/src` (or is listed in a small `legacyMapNames` set with a reason), and vice-versa for every `#[map(name = "unhd_…")]`. Rationale: the Rust is the thing the verifier loads; Go must follow it, and a test that reads the Rust cannot drift. Override ONLY if the repo already has a generator for this (Step 114 would show a `//go:generate` or `tools/` entry; none was found 2026-10-27).
- [x] **Step 116** [CODE][W] ~35m: Red: `pkg/protocol/bpfschema/rust_source_test.go` with the parser + assertions; dead-name assertion for `unhd_*`. Run `go test ./pkg/protocol/bpfschema/ -run 'RustSource' -count=1 2>&1 | tail -6` → expect FAIL on the dead names (Step 114 count mismatch) and PASS on sizes.
- [x] **Step 117** [V] ~2m: Sizes/offsets PASS (if a size fails, the Go struct is wrong today: stop, record the finding, fix the Go struct, re-run); dead names FAIL with the list printed.
- [x] **Step 118** [CODE][W] ~10m: Green: remove each dead `unhd_*` constant that no Rust program declares and no Go code uses (`grep -rn <name> --include=*.go`); for names used by Go but absent from Rust, add them to `legacyMapNames` with the reason and a `Deprecated:` comment.
- [x] **Step 119** [TEST][B] ~3m: `cd /home/govan/tmp/unheaded && go build ./... && go test ./pkg/protocol/bpfschema/ -race -count=1 2>&1 | tail -3`
- [x] **Step 120** [V] ~1m: `ok`.
- [x] **Step 121** [CODE][W] ~6m: Replace the hardcoded numbers in `parity_test.go` with references to the Rust-derived values (or delete the hardcoded tests now that `rust_source_test.go` covers them; keep one hardcoded `MONAD_SIZE == 20` sentinel so a parser bug cannot silently pass).
- [x] **Step 122** [TEST][B] ~3m: `cd /home/govan/tmp/unheaded && go test ./pkg/protocol/bpfschema/ -race -count=1 -v 2>&1 | grep -E '^(--- |ok|FAIL)' | tail -12`
- [x] **Step 123** [V] ~1m: Every `--- PASS`; the sentinel present.
- [x] **Step 124** [DOC-UPDATE][W] ~5m: `pkg/protocol/bpfschema/doc.go` (create if absent): "Rust is the source of truth; `rust_source_test.go` enforces it; add a Go mirror only after the Rust struct exists."

### Phase 3 close

- [x] **Step 125** [TEST][B] ~8m: Whole-tree regression against the baseline
  ```bash
  cd /home/govan/tmp/unheaded && go test ./... 2>&1 | grep -E '^(ok|FAIL|---)' | sort > /tmp/claude-1000/-home-govan-tmp/f267c16f-13e4-4b29-87ee-e434cae4bce1/scratchpad/tw-p3.txt; diff <(grep '^FAIL' docs/battle-plans/progress/S-TRUE-WIRE-go-baseline.txt) <(grep '^FAIL' /tmp/claude-1000/-home-govan-tmp/f267c16f-13e4-4b29-87ee-e434cae4bce1/scratchpad/tw-p3.txt) && echo "no new FAIL"
  ```
- [x] **Step 126** [V] ~1m: `no new FAIL`. New FAIL → Step 126a.
- [ ] **Step 126a** [D][B] ~5m: `go test <failing pkg> -run <Test> -v -count=1 2>&1 | tail -30`; the usual cause is a caller asserting an old flag name or a CRC symbol. Fix the caller, not the test. Two failures → STUCK, revert the offending step's file, continue.
- [x] **Step 127** [SECURITY][R] ~3m: Attack-surface note for Phase 11: Phase 3 changed no wire byte and no map; the CRC consolidation changes which function computes the Monad checksum in `pkg/ebpf/anamnesis.go` and `pkg/cartridge` (same algorithm, proven by Step 111). Write the row into the Phase 11 table draft in the progress file.
- [x] **Step 128** [V] ~2m: **PHASE 3 EXIT GATE** — Steps 106, 112, 120, 123, 126 passed; `grep -c 'func CRC16CCITT\|func crc16CCITT' $(git ls-files 'pkg/**/*.go')` is 1; no `Kingdom(Normal|Priority|Experimental|Reserved)` in `core_maps.go` unless aliased with a caller.
- [x] **Step 129** [C] ~1m: **COMMIT CHECKPOINT** `[PLAN S-TRUE-WIRE] Steps 114-129: bpfschema parity derives from the Rust source; dead unhd_* names removed`.
- [x] **Step 130** [B] ~2m: Merge the `bpfschema` lane: `cd /home/govan/tmp/unheaded && git merge --no-ff sprint/true-wire-bpfschema -m "merge S-TRUE-WIRE Phase 3 (bpfschema hygiene)" 2>&1 | tail -2`

---

## PHASE 4: ERROR REGISTRY, WOTAN-03 ERRORS, GOAWAY/SETTINGS, TOPIC RELOAD (Steps 131-168)

**Goal**: The 13-code error registry has symbols in Go and Rust and Shield emits a real error code (M-144); Wotan-03's structured error exists in Go and Wotan reloads its topic config on SIGHUP (M-145); the GOAWAY/SETTINGS reference packages are ruled keep-or-delete with the lifecycle package (M-145, M-146 follow-through); TLV container and Ring Path Counter stay reference-only by ruling (M-144).
**Prerequisite**: Phase 1 exit gate (G8 rulings), Phase 3 merged (no CRC conflicts). Lane: `errors-wotan` worktree. [P:9]
**Time**: 5 h
**Agent**: Agent

### Error-code registry symbols (M-144)

- [x] **Step 131** [R][B] ~4m: Registry vs code
  ```bash
  cd /home/govan/tmp/unheaded && grep -E '^\| 0x' docs/protocol/error-registry.md | cut -d'|' -f2,4 | head -16; grep -nE 'ErrorCode = 0x' pkg/protocol/errors/errors.go | head -16; grep -rnE 'ERROR_COUNTERS|make_error_key|error_code' ebpf/shield-ebpf/src/main.rs ebpf/monad-common/src/lib.rs | head -8
  ```
- [x] **Step 132** [DECIDE][SPEC] ~4m: **Registry names and numbers win; the Go package is rewritten to match.** RECOMMENDATION: `pkg/protocol/errors` exports `type Code uint8` with the 13 registry constants (`NoError = 0x00, CRCValidationFailed = 0x01 … QUICVersionMismatch = 0x0C, Reserved0D = 0x0D`), `Level` (Flow/Domain/System), `String()`, and a table test that parses `docs/protocol/error-registry.md` rows; the old `UNHD_*` 16-bit constants are deleted (nothing imports the package: G8). Rust: `pub enum ErrorCode` with the same 13 values in `ebpf/monad-common/src/lib.rs` next to `EventType`, plus a `#[test]` that reads the registry file at test time (host tests only). Rationale: the registry is IANA-style text in the drafts; two codebases with private numberings is how the hop_count proxy happened. Override ONLY if `error-registry.md` disagrees with foundation-06 §Error Code Registry (:1328-1352) (then foundation-06 wins and the md is fixed first, [SPEC]).
- [x] **Step 133** [CODE][W] ~20m: Red: Go table test (parse the md) + Rust test (parse the md via `include_str!` relative path) → FAIL (symbols missing / wrong).
- [x] **Step 134** [CODE][W] ~25m: Green: Go `pkg/protocol/errors/errors.go` rewritten; Rust enum + `from_u8`; a `doc.go` banner "conformance reference for foundation-06 §Error Code Registry; the Shield emits these codes (Step 136)".
- [x] **Step 135** [TEST][B] ~4m: `cd /home/govan/tmp/unheaded && go test ./pkg/protocol/errors/ -race -count=1 2>&1 | tail -2; (cd ebpf/monad-common && cargo test 2>&1 | grep -E '^test result' | tail -1)`
- [x] **Step 136** [CODE][W] ~15m: Shield emits a real code: in `ebpf/shield-ebpf/src/main.rs:521-532` replace `monad.hop_count as u16` with `ErrorCode::CrcValidationFailed as u16` when `!monad.verify_checksum()`, `ErrorCode::VersionNotSupported` when the version byte is not 0x01 (add the check if absent), and leave CHAOS-flagged packets out of the error counter (chaos is not an error; EVENT_CHAOS already records it). Keep the map key shape.
- [x] **Step 137** [BUILD][B] ~4m: `cd /home/govan/tmp/unheaded/ebpf && cargo check --target=x86_64-unknown-linux-gnu 2>&1 | tail -1 && cargo build --release -p shield-ebpf 2>&1 | tail -1`
- [x] **Step 138** [V] ~1m: `Finished` twice. Verifier load is proved in Phase 5 (Step 203) which rebuilds shield anyway; if this lane runs before Phase 5, run `sudo scripts/check-ebpf-loads.sh | tail -3` here.
- [x] **Step 139** [DECIDE] ~3m: **TLV container and Ring Path Counter stay reference-only.** RECOMMENDATION: `pkg/protocol/tlv` is KEEP-AS-CONFORMANCE (Step 69); no eBPF program parses TLVs beyond the Monad option today and foundation-06 :1007 makes extensions additive and optional; Ring Path Counter (TLV 0x01) gets a one-line "not emitted by any implementation (2026-10-27)" note in `docs/protocol/draft-bellis-unheaded-protocol-foundation-06.md` :1371. Rationale: implementing TLV parsing in XDP costs verifier budget for a feature with no producer. Override ONLY if the Shield phase finds a packet path that already carries a second TLV.
- [x] **Step 140** [W][SPEC] ~4m: Add the implementation-status note at foundation-06 :1371 and the twin in `ietf-submission`.
- [x] **Step 141** [C] ~1m: **COMMIT CHECKPOINT** `[PLAN S-TRUE-WIRE] Steps 131-141: error-code registry symbols in Go and Rust; Shield emits real codes`.

### Wotan-03 structured error (M-145)

- [x] **Step 142** [R][B] ~4m: `cd /home/govan/tmp/unheaded && sed -n '135,175p' ietf-submission/draft-bellis-unheaded-wotan-memory-00.md; grep -rnE 'type .*Error struct|errors\.New\("wotan|fmt\.Errorf\("wotan' --include=*.go services/wotan/internal pkg/wotan-client | grep -v _test | head -10`
- [x] **Step 143** [DECIDE] ~4m: **One Go type, wrapped at the API boundary only.** RECOMMENDATION: `pkg/wotanerr` (new, small): `type Code struct { Severity, Origin, Category, Detail uint8 }` with the draft's tables as constants, `Encode() uint32`/`Decode(uint32)`, `Error()` string, `Recovery()` from the draft's recommended-action column, and a table test parsing the draft section. Wotan's HTTP handlers and gRPC status details carry `X-Wotan-Error: 0xSSOOCCDD` / `errdetails` on error responses; `pkg/wotan-client` decodes it into a typed error. Internal code keeps plain Go errors. Rationale: the draft defines a wire-visible taxonomy for clients, not an internal error type; wrapping at the boundary gives conformance without touching every `return err`. Override ONLY if the draft mandates the code inside Wotan WAL entries (Step 142 text would say so; it does not as of 2026-10-27).
- [x] **Step 144** [CODE][W] ~15m: Red: `pkg/wotanerr/wotanerr_test.go` (round trip, table from the draft, three known error paths in `services/wotan/internal/api/handlers.go` produce the right code). → FAIL.
- [x] **Step 145** [CODE][W] ~30m: Green: the package; the middleware in `services/wotan/internal/api` that maps the three paths found in Step 142 (unauthorized publish, unknown topic, signature required) to codes and sets the header; `pkg/wotan-client` decode into `*wotanerr.Code` with `errors.As` support.
- [x] **Step 146** [TEST][B] ~5m: `cd /home/govan/tmp/unheaded && go test ./pkg/wotanerr/ ./services/wotan/... ./pkg/wotan-client/ -race -count=1 2>&1 | grep -E '^(FAIL|---)' || echo "all ok"`
- [x] **Step 147** [V] ~1m: `all ok`.
- [x] **Step 148** [W][SPEC] ~5m: Draft note in `docs/protocol/draft-bellis-unheaded-wotan-memory-03.md` + twin: "Implementation: `X-Wotan-Error` header / gRPC error detail carries the 32-bit code; S-TRUE-WIRE Step 145."

### GOAWAY / SETTINGS reference packages (M-145, M-146)

- [x] **Step 149** [R][B] ~4m: `cd /home/govan/tmp/unheaded && ls pkg/protocol/lifecycle pkg/protocol/settings; grep -nE '^func |^type ' pkg/protocol/lifecycle/*.go pkg/protocol/settings/*.go | grep -v _test | head -16; grep -rn 'GOAWAY\|SETTINGS' docs/protocol/draft-bellis-unheaded-wotan-memory-03.md docs/protocol/draft-bellis-unheaded-protocol-foundation-06.md | head -6`
- [x] **Step 150** [DECIDE] ~4m: **`settings` KEEP as conformance reference; `lifecycle` (GOAWAY) KEEP only if a draft §cites GOAWAY on topic streams, else DELETE via E3.** RECOMMENDATION: based on Step 149: if wotan-memory-03 or foundation-06 has a GOAWAY section → KEEP both with `doc.go` banners and a conformance test each; if neither → `settings` KEEP (foundation-06 names SETTINGS_* in its registries), `lifecycle` → E3 DELETE row. Shield's `GOAWAY_STATE` map and the dead check at `main.rs:513-519` follow the same ruling: KEEP → Phase 5 implements monotonicity; DELETE → Phase 5 removes the map and the lookup (one less map op per egress packet). Rationale: RFC 9114 GOAWAY semantics only make sense on a stream the Kingdom actually multiplexes. Override ONLY if `services/wotan` gRPC streams already send GOAWAY frames (Step 142 grep would show it).
- [x] **Step 151** [W] ~8m: Record the ruling in `docs/protocol/G8-REFERENCE-RULINGS.md` (update the `lifecycle`/`settings` rows) and in the progress file for Phase 5 (Step 213 reads it).
- [x] **Step 152** [V] ~1m: `grep -nE 'lifecycle|settings' docs/protocol/G8-REFERENCE-RULINGS.md` shows a ruling for each.

### SIGHUP topic-config reload (M-145)

- [x] **Step 153** [R][B] ~3m: `cd /home/govan/tmp/unheaded && grep -nE 'WOTAN_TOPIC_CONFIG|topics\.|LoadTopicConfig|auto_approve' services/wotan/cmd/wotan/main.go services/wotan/internal/grpc/topic_service.go | head -10; sed -n '370,380p' services/wotan/cmd/wotan/main.go`
- [x] **Step 154** [CODE][W] ~10m: Red: `services/wotan/internal/grpc/topic_service_reload_test.go`: start the topic service with a temp config, SIGHUP-equivalent `Reload()` with an edited file adding an `auto_approve` name, assert the new name is approved and a removed one is refused; a malformed file keeps the old config and increments `unheaded_wotan_topic_config_reload_failures_total`. → FAIL.
- [x] **Step 155** [CODE][W] ~25m: Green: `Reload()` on the topic service (atomic swap of the parsed config under the existing lock), `signal.Notify(hup, syscall.SIGHUP)` in `main.go:374` area calling it, the two metrics (`…_reloads_total`, `…_reload_failures_total`) through the shared registry, a log line with the diff count. Document in `wiki/Service-Wotan.md` "Reload topics: `systemctl kill -s HUP unheaded-wotan`".
- [x] **Step 156** [TEST][B] ~4m: `cd /home/govan/tmp/unheaded && go test ./services/wotan/... -race -count=1 2>&1 | grep -E '^(FAIL|---)' || echo "all ok"`
- [x] **Step 157** [V] ~1m: `all ok`.
- [x] **Step 158** [TEST][B] ~5m: Live-ish: run Wotan from the tree on a free port with a temp config, send SIGHUP, read the metric
  ```bash
  cd /home/govan/tmp/unheaded && go build -o /tmp/claude-1000/-home-govan-tmp/f267c16f-13e4-4b29-87ee-e434cae4bce1/scratchpad/wotan ./services/wotan/cmd/wotan && cp configs/wotan.yaml /tmp/claude-1000/-home-govan-tmp/f267c16f-13e4-4b29-87ee-e434cae4bce1/scratchpad/wotan.yaml && (WOTAN_TOPIC_CONFIG=/tmp/claude-1000/-home-govan-tmp/f267c16f-13e4-4b29-87ee-e434cae4bce1/scratchpad/wotan.yaml /tmp/claude-1000/-home-govan-tmp/f267c16f-13e4-4b29-87ee-e434cae4bce1/scratchpad/wotan -http-port 28000 -grpc-port 28001 >/tmp/claude-1000/-home-govan-tmp/f267c16f-13e4-4b29-87ee-e434cae4bce1/scratchpad/wotan.log 2>&1 & echo $! > /tmp/claude-1000/-home-govan-tmp/f267c16f-13e4-4b29-87ee-e434cae4bce1/scratchpad/wotan.pid); sleep 2; kill -HUP "$(cat /tmp/claude-1000/-home-govan-tmp/f267c16f-13e4-4b29-87ee-e434cae4bce1/scratchpad/wotan.pid)"; sleep 1; curl -s http://127.0.0.1:28000/metrics | grep -E 'topic_config_reload' ; kill "$(cat /tmp/claude-1000/-home-govan-tmp/f267c16f-13e4-4b29-87ee-e434cae4bce1/scratchpad/wotan.pid)"
  ```
  (Adjust the port flags to Wotan's real flag names from `main.go` if they differ.)
- [x] **Step 159** [V] ~1m: `unheaded_wotan_topic_config_reloads_total 1`. If the process ignores HUP → the handler is registered after the blocking serve call; move it before.
- [x] **Step 160** [C] ~1m: **COMMIT CHECKPOINT** `[PLAN S-TRUE-WIRE] Steps 142-160: Wotan-03 structured error at the API boundary; SIGHUP topic reload; GOAWAY/SETTINGS ruling`.

### Light-touch wotan-04 verifications (M-145 tail)

- [x] **Step 161** [R][B] ~5m: The wotan-04 verification list from the sweep
  ```bash
  cd /home/govan/tmp/unheaded && grep -nE 'WOT-0[79]' docs/sweeps/outstanding-2026-10-27/sweep-protocol.md | cut -c1-400
  ```
- [x] **Step 162** [TEST][B] ~15m: For each verification named in Step 161 that is a read-only check against the code or a unit test (wildcard topic rules, seq/after_seq paging, cursor semantics), write or extend one table test in `services/wotan/internal/...` so the draft statement has an executable witness; skip anything needing the live stack and list it for Phase 12's handoff.
- [x] **Step 163** [V] ~2m: `go test ./services/wotan/... -count=1 | tail -2` ok; the list of skipped live items is in the progress file.

### Phase 4 close

- [x] **Step 164** [TEST][B] ~8m: Regression vs baseline (same command as Step 125 with `tw-p4.txt`).
- [x] **Step 165** [V] ~1m: `no new FAIL`.
- [x] **Step 166** [SECURITY][W] ~3m: Phase 11 rows: new `X-Wotan-Error` header (information disclosure review: codes only, no internals); SIGHUP handler (signal from the same uid only; systemd unit unchanged); Shield error-code change (no new map, same key shape).
- [x] **Step 167** [V] ~2m: **PHASE 4 EXIT GATE** — Steps 135, 138, 147, 157, 159, 163, 165 passed; registry parity tests exist in Go and Rust; rulings recorded for `lifecycle`/`settings`/TLV.
- [x] **Step 168** [C] ~1m: **COMMIT CHECKPOINT** `[PLAN S-TRUE-WIRE] Steps 161-168: Phase 4 exit gate` and merge the `errors-wotan` lane.

---

## PHASE 5: SHIELD DATA PLANE TRUTH (Steps 169-230)

**Goal**: Shield's deferred checks run (HMAC key presence, retry-token validity, hop-validator match) inside a `bpf_loop` callback with a userspace writer for the three maps (M-180); the GOAWAY and authority lookups either enforce or disappear (Step 150 ruling); debug TC counters leave the production object (M-183); packet-marker extracts trace IDs from TCP/IP options through an `#[inline(never)]` subprogram (M-182); the stress cannon sends correct CRCs (M-183); every "WIRED" claim is corrected; a finding is registered for the period the docs were wrong. Verifier budget and the UPC gates are re-measured after every object change.
**Prerequisite**: Phase 2 exit gate (event types settled), Phase 4 Steps 136-138 (error codes) merged. Passwordless sudo on west. Lane: main tree.
**Time**: 9 h
**Agent**: Coordinator

### Register the finding first (the docs were wrong for months)

- [x] **Step 169** *(committed on develop `7955ef3d`: the register lives there)* [W][AUDIT-UPDATE] ~8m: Add `FND-025` to `compliance/findings/register.yaml`: title "Shield HMAC/retry/hop-validator/GOAWAY/authority checks stubbed while docs and code comment said wired"; severity medium; evidence `ebpf/shield-ebpf/src/main.rs:268-273,513-540`, `docs/archive/internal-2026/battle-plans/S24-handoff-battle-orders.md`; remediation "this plan Steps 169-230"; due 30 days. Follow the schema of FND-024's entry.
- [x] **Step 170** [B] ~3m: `cd /home/govan/tmp/unheaded && go run ./cmd/compliance-evidence findings -adr docs/adr/ADR-098-findings-register-and-baseline-modes.md 2>&1 | tail -2 && grep -c 'FND-025' docs/adr/ADR-098-findings-register-and-baseline-modes.md`
- [x] **Step 171** [V] ~1m: Register regenerates; `FND-025` appears once in the ADR's generated section.
- [x] **Step 172** *(already corrected 2026-10-27 as M-180; FND-025 id added 2026-10-28)* [W][DOC-UPDATE] ~5m: Correct the claims now, before the code: `docs/archive/internal-2026/battle-plans/S24-handoff-battle-orders.md` "WIRED" → "STUBBED (FND-025, S-TRUE-WIRE Phase 5)"; `ebpf/shield-ebpf/src/main.rs:268-273` comment → "maps exist; no userspace writer yet (FND-025)". Both get fixed again at Step 228 when it is true.
- [x] **Step 173** [C] ~1m: **COMMIT CHECKPOINT** `[PLAN S-TRUE-WIRE] Steps 169-173: FND-025 registered; WIRED claims corrected`.

### Measure before touching (the verifier budget is the constraint)

- [x] **Step 174** [TEST][S][BARE-METAL][B] ~6m: Shield's verified instruction count today, both programs (XDP ingress, TC egress)
  ```bash
  cd /home/govan/tmp/unheaded/ebpf && cargo build --release -p shield-ebpf 2>&1 | tail -1 && cd .. && ls cmd/ebpf-load-check/src && (cd cmd/ebpf-load-check && cargo build --release 2>&1 | tail -1) && sudo cmd/ebpf-load-check/target/release/ebpf-load-check ebpf/target/bpfel-unknown-none/release/shield-ebpf 2>&1 | grep -aE 'verified|processed|OK|FAIL' | head -6
  ```
  (If `ebpf-load-check` takes different arguments, read `cmd/ebpf-load-check/src/main.rs` `--help` first; the goal is the kernel's `processed N insns` line per program.)
- [x] **Step 175** [V] ~2m: Two numbers recorded in the progress file as `shield_xdp_insns_pre`, `shield_tc_insns_pre`. If the tool prints no insn line → Step 175a.
- [x] **Step 175a** *(not needed: ebpf-load-check now prints the count)* [D][S][B] ~5m: `sudo bpftool prog load ebpf/target/bpfel-unknown-none/release/shield-ebpf /sys/fs/bpf/tw-shield-probe type xdp 2>&1 | grep -E 'processed|verification' ; sudo rm -f /sys/fs/bpf/tw-shield-probe` — bpftool prints the verifier summary on load with `-d`. Record the numbers.
- [x] **Step 176** [R][B] ~6m: The pattern to copy
  ```bash
  cd /home/govan/tmp/unheaded && sed -n '276,300p;370,400p;440,470p;525,545p' ebpf/cartridge-ebpf/src/main.rs
  ```

### Debug counters out of production (M-183) — do the subtraction before the addition

- [x] **Step 177** [CODE][W] ~12m: Gate `STAT_TC_ENTRY`, `STAT_TC_IPV6`, `STAT_TC_HBH` (and any `STAT_*` that fires on every packet before the Monad is identified) behind a Cargo feature `debug-stats` in `ebpf/shield-ebpf/Cargo.toml` (`[features] debug-stats = []`), `#[cfg(feature = "debug-stats")]` on the three `increment_stat` calls at `main.rs:460,467,476`. Keep `STAT_ANOMALIES`, `STAT_DEATHS`, births: those are the protocol's own counters.
- [x] **Step 178** [BUILD][B] ~3m: `cd /home/govan/tmp/unheaded/ebpf && cargo build --release -p shield-ebpf 2>&1 | tail -1 && cargo build --release -p shield-ebpf --features debug-stats 2>&1 | tail -1`
- [x] **Step 179** [TEST][S][B] ~4m: Re-measure the default object (Step 174 command); record `shield_tc_insns_post_stats`.
- [x] **Step 180** [V] ~1m: TC insns decreased (three map ops gone); XDP unchanged. Any increase → the cfg is on the wrong lines.
- [x] **Step 181** [W] ~4m: `scripts/check-ebpf-loads.sh` `VARIANTS` gains `shield-ebpf:debug-stats` so both variants stay load-tested (ADR-080 rule: every feature variant is a different program).
- [x] **Step 182** [C] ~1m: **COMMIT CHECKPOINT** `[PLAN S-TRUE-WIRE] Steps 174-182: shield debug TC stats behind a feature; pre-change insn numbers recorded`.

### The userspace writer (nothing populates the maps today)

- [ ] **Step 183** *(ESCALATED E8, 2026-10-28: see the progress log "E8"; not started)* [R][B] ~5m: What `cmd/shield` is and how other loaders pin/populate maps
  ```bash
  cd /home/govan/tmp/unheaded && sed -n '1,60p' cmd/shield/main.go | grep -nE 'import|ebpf|cilium|flag\.|func main' ; grep -rnE 'ebpf\.LoadPinnedMap|PinPath|/sys/fs/bpf/unhd' --include=*.go pkg/ebpf/loader.go cmd/upc-cartridgectl cmd/chaos-controller 2>/dev/null | head -8; grep -nE 'HmacKey|RetryToken|HopValidator' pkg/protocol/bpfschema/bpfschema.go | head -12
  ```
- [ ] **Step 184** *(ESCALATED E8, 2026-10-28: see the progress log "E8"; not started)* [DECIDE] ~5m: **The writer lives in `cmd/shield` (the service that already owns the Shield's port), reading `/etc/unheaded/shield/keys.yaml`.** RECOMMENDATION: `pkg/shieldmaps` (new): typed `Put/Delete/List` for the three maps using cilium/ebpf `LoadPinnedMap` on `/sys/fs/bpf/unhd_{hmac_keys,retry_tokens,hop_validators}` with the key/value structs from `bpfschema`; `cmd/shield` gains `-keys-file` (default `/etc/unheaded/shield/keys.yaml`, 0400, owner `unheaded-shield`), loads it at start and on SIGHUP, writes the maps, exposes counts as metrics; when the pins are absent (shield-ebpf not loaded) it logs once and serves everything else. Key material never in the repo: the file is an env-pointed host path; the test uses a temp file and a `MockMapper`. Rationale: the loader (`cmd/ebpf-loader`) pins; the service populates; that is the split every other Kingdom program uses (cartridgectl, chaos-controller). Override ONLY if Step 183 shows `cmd/shield` is a WAF stub unrelated to the eBPF object (then the writer goes into `cmd/ebpf-loader` behind a `-shield-keys` flag).
- [ ] **Step 185** *(ESCALATED E8, 2026-10-28: see the progress log "E8"; not started)* [CODE][W] ~15m: Red: `pkg/shieldmaps/shieldmaps_test.go` with a `Mapper` interface (real cilium/ebpf impl + fake), tests for Put/Delete/List, key encoding against `bpfschema` sizes (`unsafe.Sizeof`), and the YAML schema (`hmac_keys: [{flow_prefix, key_hex}]`, `retry_tokens: […]`, `hop_validators: […]`). → compile FAIL.
- [ ] **Step 186** *(ESCALATED E8, 2026-10-28: see the progress log "E8"; not started)* [CODE][W] ~35m: Green: the package; the `cmd/shield` wiring (`-keys-file`, SIGHUP, metrics `unheaded_shield_hmac_keys`, `…_retry_tokens`, `…_hop_validators` gauges, `…_keys_reload_failures_total`); a `configs/shield/keys.example.yaml` with obviously fake values and a header comment "copy to /etc/unheaded/shield/keys.yaml; never commit the real one".
- [ ] **Step 187** *(ESCALATED E8, 2026-10-28: see the progress log "E8"; not started)* [TEST][B] ~4m: `cd /home/govan/tmp/unheaded && go test ./pkg/shieldmaps/ ./cmd/shield/ -race -count=1 2>&1 | tail -3 && scripts/check-gitleaks.sh 2>&1 | tail -1`
- [ ] **Step 188** *(ESCALATED E8, 2026-10-28: see the progress log "E8"; not started)* [V] ~1m: `ok` twice; gitleaks `[PASS]` (the example file must not look like a secret: use `0000…` and say so).
- [ ] **Step 189** *(ESCALATED E8, 2026-10-28: see the progress log "E8"; not started)* [W] ~6m: `deploy/systemd/unheaded-shield.service` (new if absent; check `ls deploy/systemd | grep shield`): `User=unheaded-shield`, `DynamicUser=no`, `CapabilityBoundingSet=CAP_BPF`, `AmbientCapabilities=CAP_BPF`, `BindReadOnlyPaths=/sys/fs/bpf`, `ReadOnlyPaths=/etc/unheaded/shield`, the hardening block from `unheaded-anomaly.service`; `sysusers.d` entry via `pkg/uids` (next free uid, record in `wiki/Port-Registry.md`'s uid table).
- [ ] **Step 190** *(ESCALATED E8, 2026-10-28: see the progress log "E8"; not started)* [TEST][B] ~3m: `cd /home/govan/tmp/unheaded && scripts/check-systemd-units.sh 2>&1 | tail -3 && go test ./pkg/uids/ -count=1 | tail -1`
- [ ] **Step 191** *(ESCALATED E8, 2026-10-28: see the progress log "E8"; not started)* [V] ~1m: Units gate `[PASS]`; uids test ok (no collision).
- [ ] **Step 192** *(ESCALATED E8, 2026-10-28: see the progress log "E8"; not started)* [C] ~1m: **COMMIT CHECKPOINT** `[PLAN S-TRUE-WIRE] Steps 183-192: pkg/shieldmaps + cmd/shield writes HMAC/retry/hop-validator maps from a host keys file`.

### The checks, inside bpf_loop (M-180)

- [ ] **Step 193** *(ESCALATED E8, 2026-10-28: see the progress log "E8"; not started)* [DESIGN][W] ~15m: Write `ebpf/shield-ebpf/DESIGN-checks.md` (short): one `bpf_loop` with at most 3 iterations over a `[CheckKind; 3]` table (`Hmac`, `Retry`, `HopValidator`), callback `#[inline(never)] fn shield_check_step(i: u32, ctx: *mut CheckCtx) -> i32`; `CheckCtx` ≤ 64 bytes on the stack (flow_key, monad ptr, verdict bits); each step does ONE map lookup and sets a bit; after the loop: no HMAC key for a flow whose Monad has `ENCRYPT` set → `EVENT_ANOMALY` + error code `CrcValidationFailed`? No: add registry code use `FlowStateCorruption (0x0A)` for "integrity material missing" (document the mapping in the design); retry token present but expired → drop with `FlowLabelInvalid (0x03)`; hop validator mismatch → anomaly, pass (observe mode first, env `SHIELD_ENFORCE=1` flips to drop via `SHIELD_CONFIG` map key 3). Default is observe: count and emit, never drop, until the SLO guard has seen a week of zero false positives (same rule as the chaos envelope).
- [ ] **Step 194** *(ESCALATED E8, 2026-10-28: see the progress log "E8"; not started)* [CODE][W] ~45m: Implement per the design in `ebpf/shield-ebpf/src/main.rs`: the callback, the loop call at :268 replacing the comment, the `SHIELD_CONFIG` enforce key, three new `STAT_*` counters (checks_run, checks_failed, checks_enforced).
- [ ] **Step 195** *(ESCALATED E8, 2026-10-28: see the progress log "E8"; not started)* [BUILD][B] ~4m: `cd /home/govan/tmp/unheaded/ebpf && cargo check --target=x86_64-unknown-linux-gnu 2>&1 | tail -2 && cargo build --release -p shield-ebpf 2>&1 | tail -2`
- [ ] **Step 196** *(ESCALATED E8, 2026-10-28: see the progress log "E8"; not started)* [V] ~1m: `Finished` twice. Compile errors → the usual: the callback signature must be `extern "C" fn(u32, *mut c_void) -> i64` for aya's `bpf_loop` binding (copy cartridge-ebpf's exactly).
- [ ] **Step 197** *(ESCALATED E8, 2026-10-28: see the progress log "E8"; not started)* [TEST][S][BARE-METAL][B] ~5m: Load and measure (Step 174 command).
- [ ] **Step 198** *(ESCALATED E8, 2026-10-28: see the progress log "E8"; not started)* [V] ~2m: Loads (`OK`); `shield_xdp_insns_post_checks` recorded; growth over `shield_xdp_insns_pre` is under 25 %. Rejected → Step 198a; over 25 % → Step 198b.
- [ ] **Step 198a** *(ESCALATED E8, 2026-10-28: see the progress log "E8"; not started)* [D][S][B] ~10m: `sudo bpftool prog load … -d 2>&1 | grep -B5 -E 'invalid|unbounded|R[0-9] ' | tail -30`; the usual culprits: a pointer to `ctx` escaping into the callback (pass offsets, not pointers), or stack > 512 (shrink `CheckCtx`). Max 2 attempts, then STUCK and continue at Step 206 with the checks behind `#[cfg(feature = "checks")]` off by default.
- [ ] **Step 198b** *(ESCALATED E8, 2026-10-28: see the progress log "E8"; not started)* [D][W] ~10m: Reduce iterations to the checks that have map entries (early `return 1` from the callback when the kind's map is empty is not visible to the verifier; instead gate the whole loop on `SHIELD_CONFIG` key 4 "checks enabled" so the verifier still sees it but the cost is a lookup when off). Re-measure. Record the final number either way.
- [ ] **Step 199** *(ESCALATED E8, 2026-10-28: see the progress log "E8"; not started)* [TEST][S][BARE-METAL][B] ~15m: Behavioural proof in a lab netns (the S5 pattern): attach shield to a veth, write one HMAC key through `cmd/shield -keys-file` (temp file, fake key), send 100 Kingdom packets with ENCRYPT set for that flow and 100 for another flow, read the counters
  ```bash
  cd /home/govan/tmp/unheaded && grep -nE '^(lab_up|lab_down|send_|attach_)' scripts/upc-regression.sh scripts/lab/*.sh 2>/dev/null | head -8
  ```
  Then run the lab helper the grep shows (or the chaos lab recipe in `wiki/Chaos-Engineering.md` "lab netns") with shield attached; expected: `checks_run = 200`, `checks_failed = 100` (the keyless flow), `checks_enforced = 0`, zero drops.
- [ ] **Step 200** *(ESCALATED E8, 2026-10-28: see the progress log "E8"; not started)* [V] ~2m: Counters as expected; `scripts/upc-regression.sh`'s lab teardown leaves no netns (`ip netns list | wc -l` = 0). If `checks_failed` is 200 → the key write did not land (pin path mismatch between loader and writer: compare `bpftool map list | grep unhd_hmac`).
- [ ] **Step 201** *(ESCALATED E8, 2026-10-28: see the progress log "E8"; not started)* [TEST][S][BARE-METAL][B] ~4m: Enforce mode: set `SHIELD_CONFIG[3]=1` via `cmd/shield -enforce` (or `bpftool map update`), resend the keyless flow, expect `checks_enforced = 100` and `XDP_DROP` count 100; reset to observe.
- [ ] **Step 202** *(ESCALATED E8, 2026-10-28: see the progress log "E8"; not started)* [V] ~1m: As expected; observe mode restored (`SHIELD_CONFIG[3]=0`).
- [ ] **Step 203** *(ESCALATED E8, 2026-10-28: see the progress log "E8"; not started)* [TEST][S][BARE-METAL][B] ~8m: Whole-tree eBPF gates: `cd /home/govan/tmp/unheaded && scripts/check-ebpf-loads.sh 2>&1 | tail -3 && scripts/upc-regression.sh 2>&1 | grep -aE 'ALL GREEN|FAIL|SAME|DIFF' | head -4`
- [ ] **Step 204** *(ESCALATED E8, 2026-10-28: see the progress log "E8"; not started)* [V] ~1m: `[PASS]`, `ALL GREEN`, `12/12 SAME`. Shield is not on the UPC path, so ascend/Doom numbers must be identical to Step 16.
- [ ] **Step 205** *(ESCALATED E8, 2026-10-28: see the progress log "E8"; not started)* [C] ~1m: **COMMIT CHECKPOINT** `[PLAN S-TRUE-WIRE] Steps 193-205: Shield HMAC/retry/hop-validator checks run in bpf_loop, observe mode default, enforce behind SHIELD_CONFIG`.

### GOAWAY and authority: enforce or remove (Step 150 ruling)

- [x] **Step 206** [R] ~2m: Read the Phase 4 ruling for `lifecycle` from `docs/protocol/G8-REFERENCE-RULINGS.md`.
- [x] **Step 207** *(GOAWAY half done 2026-10-28; the AUTHORITY half is part of E8)* [CODE][W] ~20m: If DELETE: remove `GOAWAY_STATE` map, `make_goaway_key`, the block at :513-519, and `MapGoawayState`/`GoawayState*` from `bpfschema` (the Phase 3 Rust-source test will demand it). If KEEP: implement monotonicity — value `[u8;16]` = `{last_flow_id: u32, sent_at_ns: u64, pad}`; a packet whose `flow_label` > `last_flow_id` on a connection with a GOAWAY → anomaly with `FlowLabelInvalid`. Authority (:535-540): implement the check the comment promises — `AUTHORITY` value carries `{fingerprint: [u8;8], expires_ns: u64}`; mismatch or expiry → anomaly, observe mode only; the writer for `AUTHORITY` is the Sophia service (add a TODO-free `pkg/shieldmaps.Authority` Put and a `services/sophia` call at dictionary publish; if that is more than 30 min, make the authority writer an [AFTER-IRON-GATE] follow-on and leave the check in observe mode with a counter).
- [x] **Step 208** [BUILD][TEST][S][B] ~8m: Steps 195 + 197 again; record `shield_tc_insns_post_goaway`.
- [x] **Step 209** [V] ~1m: Loads; number recorded; `go test ./pkg/protocol/bpfschema/ -count=1 | tail -1` ok (the Rust-source parity test agrees with the map set).
- [x] **Step 210** [C] ~1m: **COMMIT CHECKPOINT** `[PLAN S-TRUE-WIRE] Steps 206-210: GOAWAY/authority ruling applied in Shield`.

### Stress cannon CRC (M-183)

- [ ] **Step 211** [R][B] ~3m: `cd /home/govan/tmp/unheaded && grep -rnE 'cannon|stress' scripts/*.sh tools/ cmd/ 2>/dev/null | grep -iE 'monad|crc|checksum' | head -6; ls scripts | grep -iE 'cannon|stress|flood'`
- [ ] **Step 212** [CODE][W] ~15m: The sender computes the Monad CRC with `encoding.CRC16CCITT` (Go) or `monad_common` (Rust) before send; add a `-bad-crc` flag for the negative test; a unit test asserts a generated packet verifies.
- [ ] **Step 213** [TEST][B] ~3m: `go test ./<cannon pkg>/ -count=1 | tail -1` (path from Step 211).
- [ ] **Step 214** [V] ~1m: ok. Now `STAT_ANOMALIES` under the cannon measures real anomalies, not the cannon's own garbage.

### packet-marker trace-ID extraction (M-182)

- [ ] **Step 215** [R][B] ~4m: `cd /home/govan/tmp/unheaded && sed -n '150,230p' ebpf/packet-marker/src/main.rs | grep -nE 'fn |TODO|option|doff|ihl|trace_id' | head -16; grep -nE 'TRACE_OPT|0x[0-9A-F]{2} *// *trace|kind' ebpf/packet-marker/src/main.rs | head -6`
- [ ] **Step 216** [DESIGN][W] ~10m: Note in `ebpf/packet-marker/DESIGN-options.md`: `#[inline(never)] fn scan_tcp_options(ctx_off: u32, hdr_len: u32, data_end: u32) -> u64` under `bpf_loop` with max 10 iterations (TCP options ≤ 40 bytes, min option 2 bytes → ≤ 20 but 10 covers every real stack; document), each iteration reads `kind,len` with explicit bounds checks against `data_end` captured as a value, returns the trace ID when `kind == TRACE_OPT_KIND`; IP options the same with max 10. The verifier-eliminated-bounds-check problem goes away because each iteration's offsets are loop-carried values, not LLVM-provable constants.
- [ ] **Step 217** [CODE][W] ~40m: Implement; remove the two `TODO(E1)` blocks; keep `TRACE_INJECT` precedence as is (extracted ID wins over injected when both exist? the existing code injects only when `trace_id.is_zero()`, keep that).
- [ ] **Step 218** [BUILD][TEST][S][B] ~6m: `cd /home/govan/tmp/unheaded/ebpf && cargo build --release -p packet-marker 2>&1 | tail -1 && cd .. && sudo cmd/ebpf-load-check/target/release/ebpf-load-check ebpf/target/bpfel-unknown-none/release/packet-marker 2>&1 | grep -aE 'verified|processed|OK|FAIL' | head -3`
- [ ] **Step 219** [V] ~2m: Loads; record `packet_marker_insns_post`. Rejected → Step 198a procedure (max 2), then STUCK with the TODO restored and a note that the subprogram approach needs a smaller loop bound.
- [ ] **Step 220** [TEST][S][BARE-METAL][B] ~10m: Lab netns: send 50 TCP SYNs carrying the trace option (the Go injector `cmd/doom-go-injector` or `scripts/bulk_inject.py` already builds options? check with `grep -n 'option' scripts/bulk_inject.py | head -3`; else a 20-line scapy-free Python sender using raw sockets in the netns) and 50 without; read `STAT_TRACE_EXTRACTED` (new) vs `STAT_TRACE_INJECTED`.
- [ ] **Step 221** [V] ~1m: extracted = 50; the collector shows non-zero `trace_id` for those flows (`curl -s http://127.0.0.1:16671/api/v1/flows | jq '[.[]|select(.trace_id!="0000000000000000")]|length'` if the collector is running in the lab; otherwise the counter alone).
- [ ] **Step 222** [DOC-UPDATE][W] ~4m: `wiki/Packet-Marker.md` (or the eBPF page that describes it) and `next.md`'s "All-zero trace_id is expected (TODO(E1))" line → removed with the step number.
- [ ] **Step 223** [C] ~1m: **COMMIT CHECKPOINT** `[PLAN S-TRUE-WIRE] Steps 211-223: cannon sends real CRCs; packet-marker extracts trace IDs from TCP/IP options via bpf_loop`.

### Phase 5 close

- [ ] **Step 224** [TEST][S][BARE-METAL][B] ~10m: `cd /home/govan/tmp/unheaded && scripts/check-ebpf-loads.sh 2>&1 | tail -3 && scripts/upc-regression.sh 2>&1 | grep -aE 'ALL GREEN|FAIL|verified [0-9]+ insns|SAME|DIFF' | head -8 && make test-ebpf-compat 2>&1 | tail -1`
- [ ] **Step 225** [V] ~2m: `[PASS]`, `ALL GREEN`, ascend/Doom numbers equal Step 16, `12/12 SAME`, compat passed.
- [ ] **Step 226** [TEST][B] ~8m: Go regression vs baseline (`tw-p5.txt`).
- [ ] **Step 227** [V] ~1m: `no new FAIL`.
- [ ] **Step 228** [DOC-UPDATE][W] ~8m: Now it is true: `S24-handoff-battle-orders.md` → "WIRED (S-TRUE-WIRE Step 194, observe mode)"; `main.rs` comment → the design file; `wiki/Service-Shield.md` (create if absent) with: the three maps, the keys file, observe vs enforce, the counters, the insn numbers before/after (from the progress file), and "debug stats are a build feature". FND-025 → `Mitigated` in `register.yaml` with the step numbers; regenerate ADR-098 (Step 170 command).
- [ ] **Step 229** [V] ~2m: **PHASE 5 EXIT GATE** — Steps 171, 180, 188, 191, 198 (or its STUCK), 200, 202, 204, 209, 214, 219 (or STUCK), 221, 225, 227 passed; FND-025 Mitigated; every doc that said WIRED now says what the code does.
- [ ] **Step 230** [C] ~1m: **COMMIT CHECKPOINT** `[PLAN S-TRUE-WIRE] Steps 224-230: Phase 5 exit gate, Shield docs tell the truth, FND-025 mitigated`.

---

## PHASE 6: PQC_SIG_STATUS, PARKED CONTROL PLANES, THE DELETE TABLE (Steps 231-258)

**Goal**: Nothing in the tree claims an XDP-layer PQC guarantee that does not exist (M-181); the eight G9 control planes, F6, `pkg/afxdp`, `internal/bpfmap`, `pkg/ebpf/maploader`, `cmd/ebpf-exporter`, the FLAGGED `pkg/waf` set, `lxd/` and the host-a/host-b stacks get one decision table (M-186, M-187, M-168, decision 39-42) that Stevie answers once (E3), and the answered deletions land with the ADR-090 gates (build, vet, full test suite vs baseline).
**Prerequisite**: Phase 5 exit gate (the Shield object is final before the PQC map decision); Phase 1 Step 70 (G8 DELETE set).
**Time**: 4 h + wait on E3
**Agent**: Coordinator

### PQC_SIG_STATUS (M-181)

- [ ] **Step 231** [R][B] ~4m: `cd /home/govan/tmp/unheaded && grep -nE 'PQC_SIG_STATUS|pqc_fast_path_check|PqcSigStatus' ebpf/shield-ebpf/src/main.rs ebpf/pqc-common/src/*.rs 2>/dev/null | head -10; grep -rln 'PQC_SIG_STATUS\|fast path' wiki docs/security docs/protocol --include=*.md | head -8; ls cmd/pqc-verifier services/keymgmt`
- [ ] **Step 232** [DECIDE] ~5m: **Delete the dead path; keep the design doc; no XDP PQC claim anywhere until keymgmt is un-parked.** RECOMMENDATION: remove `PQC_SIG_STATUS`, `pqc_fast_path_check`, `PqcSigStatusEntry` from shield and `ebpf/pqc-common` (if nothing else uses the type), add `cmd/pqc-verifier` and `services/keymgmt(+3)` to the E3 table as DELETE-or-UNPARK rows, rewrite `wiki/PQC-Authentication.md:24` to "Layer-1 verification is a design (PQC-00 Phases 1-3); no implementation; see docs/security/shield-ebpf-pqc-fast-path-unwired-2026-08-03.md", and mark that security doc "Resolved by removal, S-TRUE-WIRE Step 234". Rationale: master-list default (decision 40); the repo is public; an unverified map that reads as a guarantee is worse than no map; a future PQC sprint re-adds it with a producer. Override ONLY if S-IRON-GATE has already landed keymgmt as the ADR-103 issuer's key store (check `git log --oneline -5 -- services/keymgmt`), in which case WIRE instead: OPTIMISTIC mode, populator in keymgmt, and the verifier budget re-check.
- [ ] **Step 233** [CODE][W] ~15m: Remove the map, the function, the entry type; `cargo build`; update `bpfschema` if it names the map (the Phase 3 test will tell).
- [ ] **Step 234** [DOC-UPDATE][W] ~10m: The wiki line; the security doc resolution banner; `docs/protocol/INNOVATIONS.md:129-131` and `SPEC-VS-CODE-AUDIT.md:624` rows → "not implemented (design only)"; `docs/protocol/draft-bellis-unheaded-pqc-authentication-00.md` Phases 1-3 gain "Implementation status (2026-MM-DD): none" ([SPEC] text only).
- [ ] **Step 235** [BUILD][TEST][S][B] ~6m: `cd /home/govan/tmp/unheaded/ebpf && cargo build --release -p shield-ebpf 2>&1 | tail -1 && cd .. && sudo scripts/check-ebpf-loads.sh 2>&1 | tail -2 && go test ./pkg/protocol/bpfschema/ -count=1 | tail -1`
- [ ] **Step 236** [V] ~1m: `Finished`, `[PASS]`, `ok`; `grep -rn PQC_SIG_STATUS --include=*.rs --include=*.go . | grep -v target/ | wc -l` is 0.
- [ ] **Step 237** [C] ~1m: **COMMIT CHECKPOINT** `[PLAN S-TRUE-WIRE] Steps 231-237: PQC_SIG_STATUS dead path removed; docs stop claiming XDP PQC`.

### The decision table (E3)

- [ ] **Step 238** [R][B] ~8m: Evidence per row, regenerated now
  ```bash
  cd /home/govan/tmp/unheaded && mains=$(go list -f '{{if eq .Name "main"}}{{.ImportPath}}{{end}}' ./...); comm -23 <(go list ./... | sort -u) <(go list -deps $mains | grep '^unheaded/' | sort -u) > /tmp/claude-1000/-home-govan-tmp/f267c16f-13e4-4b29-87ee-e434cae4bce1/scratchpad/unreached-now.txt; wc -l < /tmp/claude-1000/-home-govan-tmp/f267c16f-13e4-4b29-87ee-e434cae4bce1/scratchpad/unreached-now.txt; for p in services/compliance services/firewall services/nfv services/qos services/selfheal services/version services/keymgmt services/loadbalancer services/sabatons pkg/afxdp internal/bpfmap pkg/ebpf/maploader cmd/ebpf-exporter cmd/pqc-verifier; do printf '%-26s files=%-3s tests=%-3s last=%s ebpf_half=%s\n' "$p" "$(ls $p/*.go 2>/dev/null | grep -vc _test)" "$(ls $p/*_test.go 2>/dev/null | wc -l)" "$(git log -1 --format=%cs -- $p)" "$(ls -d ebpf/${p##*/}-ebpf 2>/dev/null || echo -)"; done; grep -E 'pkg/waf' docs/sweeps/REAL-STEEL-UNREACHED.tsv | cut -f1,2,4 | head -8; ls -d lxd docker/compose.host-* 2>/dev/null
  ```
- [ ] **Step 239** [W] ~25m: `docs/sweeps/TRUE-WIRE-DECISIONS.md` in the REAL-STEEL-DECISIONS format: one row per package/dir with evidence (Step 238), the eBPF half's load-gate status (from `docs/policy/ebpf-load-known-failing.txt`), the sweep default (decisions 39-42 + Step 69 G8 DELETE set + Step 150 `lifecycle` ruling), and the consequence of each choice (what un-parking costs: a `cmd/` per the `cmd/anomaly` recipe, a unit, a uid, a port). Default column: DELETE for `amplification, dos, intermediary, migration, prefetch` (G8), `services/{compliance,firewall,nfv,qos,selfheal,version}` and their eBPF halves' consumers, `services/sabatons` (parked by NEXT-5), `pkg/afxdp` (retire Decision 5; the supervised XDP/TC path is live), `internal/bpfmap`, `pkg/ebpf/maploader`, `cmd/ebpf-exporter`, `cmd/pqc-verifier`; KEEP-PARKED for `services/keymgmt(+3)` (ADR-103 may want it: say so) and `services/loadbalancer` (F6, maglev Phase 2); ASK for `pkg/waf` 7 pkgs (tests/s77 asserts files: delete the assertion too?), `lxd/` (superseded by Cuirass), host-a/host-b compose stacks (FND-009 D11). Note the test files that would go with each.
- [ ] **Step 240** [V] ~2m: Every row has evidence, default, and consequence; `grep -c '^| ' docs/sweeps/TRUE-WIRE-DECISIONS.md` ≥ 28.
- [ ] **Step 241** [TEST][B] ~10m: Dry-run the default set: in a scratch worktree delete every DEFAULT=DELETE path, run `go build ./... && go vet ./... && go test ./... 2>&1 | grep -E '^(FAIL|ok)' | sort | diff - docs/battle-plans/progress/S-TRUE-WIRE-go-baseline.txt | grep '^[<>] FAIL' || echo "no new FAIL"`; record which `tests/` files reference deleted paths (`grep -rl <pkg> tests/`), and the ebpf programs whose only consumer goes away (`docs/policy/ebpf-load-known-failing.txt` entries to retire).
- [ ] **Step 242** [V] ~2m: `no new FAIL` or the exact list of test files that need to go with the packages (added to the table). `git worktree remove --force` the scratch tree.
- [ ] **Step 243** [W] ~5m: Add the dry-run result column to the table.
- [ ] **Step 244** [C] ~1m: **COMMIT CHECKPOINT** `[PLAN S-TRUE-WIRE] Steps 238-244: TRUE-WIRE decision table (delete/keep/unpark) with dry-run evidence`.
- [ ] **Step 245** [DOC-UPDATE][W] ~4m: Point `docs/UNREACHED-GO-PACKAGES.md` at the new table for G9/F6/FLAGGED rows.
- [ ] **Step 246** [V] ~1m: `go build ./...` clean (nothing deleted yet).
- [ ] **Step 247** [ESCALATE] ~3m: **E3 — Answer the table.** Context: `docs/sweeps/TRUE-WIRE-DECISIONS.md`, every row with evidence and a dry-run that shows zero new test failures for the default set. Question: apply all defaults (as E1), or mark rows KEEP/UNPARK? For `pkg/waf`: delete the 7 packages and amend `tests/s77`, or keep? `lxd/` and host-a/host-b: delete? Why Stevie: deleting whole services is the owner's call even when reversible; E1 set the precedent of one table, one pass.

### Apply the answers ([AFTER-E3])

- [ ] **Step 248** [AFTER-E3][CODE][B] ~15m: One commit per group (G8 refs; G9 services; afxdp; bpfmap+maploader+exporter+pqc-verifier; waf; lxd; host stacks), each: `git rm -r <paths>` + the test files from Step 241 + `docs/policy/ebpf-load-known-failing.txt` retirements + `go build ./... && go vet ./...`.
- [ ] **Step 249** [AFTER-E3][TEST][B] ~12m: Full `go test ./...` vs baseline after the last group → `no new FAIL`.
- [ ] **Step 250** [AFTER-E3][V] ~1m: Clean. Any FAIL → the deleted package was reached by a test-only import: restore that package's test helper or delete the test with a note; never leave a red suite.
- [ ] **Step 251** [AFTER-E3][REGEN][B] ~4m: `cd /home/govan/tmp/unheaded && scripts/check-dependency-baseline.sh 2>&1 | tail -3` → if it says packages are gone from the build, `scripts/check-dependency-baseline.sh --update` (shrink-only) and show the removed lines in the commit.
- [ ] **Step 252** [AFTER-E3][REGEN][B] ~4m: Regenerate `docs/UNREACHED-GO-PACKAGES.md` (the command at its :28-29) and mark the ledger rows in `REAL-STEEL-UNREACHED.tsv` / `TOTAL-SOURCE-SWEEP.tsv` SWEPT with the commit.
- [ ] **Step 253** [AFTER-E3][TEST][S][B] ~6m: `sudo scripts/check-ebpf-loads.sh | tail -3` (the known-failing list may have shrunk: the gate demands the stale entries be removed; Step 248 did it).
- [ ] **Step 254** [AFTER-E3][V] ~1m: `[PASS]`.
- [ ] **Step 255** [AFTER-E3][DOC-UPDATE][W] ~8m: ADR status lines for anything retired (`pkg/afxdp` → the ADR that holds "Decision 5 AF_XDP product path" gets "Retired S-TRUE-WIRE Step 248"; `docs/BENCHMARKS.md:96`'s "AF_XDP 920K pps" line gains "(Python sendto ceiling; AF_XDP path retired)"); wiki pages that list deleted services.
- [ ] **Step 256** [AFTER-E3][C] ~1m: **COMMIT CHECKPOINT** `[PLAN S-TRUE-WIRE] Steps 248-256: E3 deletions applied, ledgers and baselines regenerated`.
- [ ] **Step 257** [V] ~2m: **PHASE 6 EXIT GATE** — Step 236 passed; the decision table is committed; E3 filed; if E3 answered: Steps 250, 254 passed and `go build ./...` clean. Unanswered E3 does not block Phases 7-12.
- [ ] **Step 258** [C] ~1m: **COMMIT CHECKPOINT** `[PLAN S-TRUE-WIRE] Steps 245-258: Phase 6 exit gate`.

---

## PHASE 7: THE VERIFIER RATCHET ADR-012 PROMISED (Steps 259-282)

**Goal**: A committed per-program, per-variant baseline of kernel-verified instruction counts, a gate that fails on more than 10 % growth (or any growth past the 1M ceiling's 90 %), wired into CI where root is available and honest (exit 2) where it is not; `scripts/bpf-verifier-check.sh` stops estimating bytes/8 (M-184).
**Prerequisite**: Phase 5 and Phase 6 Step 236 (the eBPF object set is final for this sprint). Passwordless sudo on west.
**Time**: 3 h
**Agent**: Coordinator

- [ ] **Step 259** [R][B] ~5m: How the load checker reports today
  ```bash
  cd /home/govan/tmp/unheaded && grep -nE 'processed|verified|insn|println|print!' cmd/ebpf-load-check/src/main.rs | head -12; sed -n '100,128p' scripts/check-ebpf-loads.sh
  ```
- [ ] **Step 260** [DECIDE] ~4m: **One TSV baseline, one gate script, the load checker prints the number.** RECOMMENDATION: `cmd/ebpf-load-check` prints `OK <program> insns=<N>` by reading the verifier log's `processed N insns` line (aya exposes `VerifierLogLevel`; capture at level STATS which costs nothing); `scripts/check-ebpf-loads.sh` passes the number through in its `results`; new `docs/policy/ebpf-insns-baseline.tsv` (`program\tvariant\tinsns\tkernel\tdate`); new `scripts/check-ebpf-insns.sh` compares: FAIL if any program grew > 10 % over its baseline row, FAIL if any program > 900,000 (ADR-012's budget) unless listed in a `docs/policy/ebpf-insns-over-budget.txt` allowlist that may only SHRINK (ascend at 926,542 is the one entry today, with ADR-012's own words as the reason), PASS otherwise; `--update` rewrites the baseline from the current numbers and prints the diff (a person reviews it in the commit). `scripts/bpf-verifier-check.sh` loses the bytes/8 block and prints the baseline row instead. Rationale: membership-compared baselines are the house pattern (secrets, dependencies, load-known-failing); the kernel's own count is the only truth. Override ONLY if `cmd/ebpf-load-check` cannot read the verifier log without root changes beyond what the load gate already needs (it can: the same BPF_PROG_LOAD returns it).
- [ ] **Step 261** [CODE][W] ~20m: `cmd/ebpf-load-check/src/main.rs`: capture the verifier log at STATS level, parse `processed (\d+) insns`, print `insns=N` on the OK line (and on FAIL lines where present). `cargo build --release` in that dir.
- [ ] **Step 262** [TEST][S][B] ~5m: `cd /home/govan/tmp/unheaded && sudo cmd/ebpf-load-check/target/release/ebpf-load-check ebpf/target/bpfel-unknown-none/release/shield-ebpf 2>&1 | head -3`
- [ ] **Step 263** [V] ~1m: `OK … insns=<N>` with N equal to Step 208's recorded number. Different → the parser caught a different program's line (multi-program objects print one line each: key the output by program name).
- [ ] **Step 264** [CODE][W] ~15m: `scripts/check-ebpf-loads.sh`: keep the pass/fail logic unchanged; additionally write `program<TAB>variant<TAB>insns` lines to `${EBPF_INSNS_OUT:-/dev/null}` for every OK result.
- [ ] **Step 265** [W] ~25m: `scripts/check-ebpf-insns.sh` (header comment in the house style: why, what it compares, exit codes 0/1/2), `docs/policy/ebpf-insns-baseline.tsv` header line, `docs/policy/ebpf-insns-over-budget.txt` with the single ascend entry and its reason, shrink-only.
- [ ] **Step 266** [TEST][S][B] ~8m: Seed the baseline
  ```bash
  cd /home/govan/tmp/unheaded && EBPF_INSNS_OUT=/tmp/claude-1000/-home-govan-tmp/f267c16f-13e4-4b29-87ee-e434cae4bce1/scratchpad/insns.tsv scripts/check-ebpf-loads.sh 2>&1 | tail -2 && scripts/check-ebpf-insns.sh --update --from /tmp/claude-1000/-home-govan-tmp/f267c16f-13e4-4b29-87ee-e434cae4bce1/scratchpad/insns.tsv 2>&1 | tail -5 && wc -l docs/policy/ebpf-insns-baseline.tsv && grep -E 'monad-cpu-ebpf' docs/policy/ebpf-insns-baseline.tsv
  ```
- [ ] **Step 267** [V] ~2m: One row per loadable program × variant (count equals the load gate's OK count); `monad-cpu-ebpf ascend-linux` row shows Step 16's ascend number; `monad-cpu-ebpf default` (Doom) shows 761,580 or Step 16's number; cartridge rows show 42,743 / 146,897 or the current numbers.
- [ ] **Step 268** [TEST][S][B] ~6m: The gate passes on itself and fails when it should
  ```bash
  cd /home/govan/tmp/unheaded && scripts/check-ebpf-insns.sh 2>&1 | tail -2; echo "rc=$?"; cp docs/policy/ebpf-insns-baseline.tsv /tmp/claude-1000/-home-govan-tmp/f267c16f-13e4-4b29-87ee-e434cae4bce1/scratchpad/b.tsv && awk -F'\t' 'BEGIN{OFS="\t"} NR>1 && $1=="shield-ebpf"{$3=int($3*0.8)} {print}' /tmp/claude-1000/-home-govan-tmp/f267c16f-13e4-4b29-87ee-e434cae4bce1/scratchpad/b.tsv > docs/policy/ebpf-insns-baseline.tsv && scripts/check-ebpf-insns.sh 2>&1 | tail -2; echo "rc-provoked=$?"; cp /tmp/claude-1000/-home-govan-tmp/f267c16f-13e4-4b29-87ee-e434cae4bce1/scratchpad/b.tsv docs/policy/ebpf-insns-baseline.tsv
  ```
- [ ] **Step 269** [V] ~1m: `rc=0` then `rc-provoked=1` naming shield-ebpf's growth. A gate that cannot fail is not a gate (`scripts/check-gates-can-fail.sh` learns this one in Step 273).
- [ ] **Step 270** [CODE][W] ~10m: `scripts/bpf-verifier-check.sh:115-125`: replace the bytes/8 block with a lookup of the program's row in the baseline (`awk`), print `verified insns (baseline <date>): N (x % of 1M)`, keep the static anti-pattern checks; `BUDGET` stays as documentation of ADR-012's number, the enforcement is `check-ebpf-insns.sh`.
- [ ] **Step 271** [TEST][B] ~3m: `cd /home/govan/tmp/unheaded && scripts/bpf-verifier-check.sh 2>&1 | tail -6`
- [ ] **Step 272** [V] ~1m: Prints the baseline row for monad-cpu-ebpf; no `estimated instructions` string remains (`grep -c 'SIZE / 8' scripts/bpf-verifier-check.sh` = 0).
- [ ] **Step 273** [W] ~8m: CI: in `.github/workflows/ebpf.yml` the load-gate job (root on the runner: check `grep -n 'check-ebpf-loads' .github/workflows/*.yml`) gains `EBPF_INSNS_OUT=$RUNNER_TEMP/insns.tsv` and a following step `scripts/check-ebpf-insns.sh --from $RUNNER_TEMP/insns.tsv`; if the load gate does not run in CI (no root), the insns gate runs in `scripts/ci-local.sh` and the meta-gate only, and the workflow gets a comment saying so. Add `check-ebpf-insns` to `scripts/check-gates-can-fail.sh`'s list with the Step 268 provocation.
- [ ] **Step 274** [TEST][B] ~4m: `cd /home/govan/tmp/unheaded && bash -n scripts/check-ebpf-insns.sh && shellcheck scripts/check-ebpf-insns.sh 2>&1 | tail -3; python3 -c "import yaml,sys; yaml.safe_load(open('.github/workflows/ebpf.yml'))" && echo "yaml ok"`
- [ ] **Step 275** [V] ~1m: shellcheck clean (or only pre-existing style codes), `yaml ok`.
- [ ] **Step 276** [DOC-UPDATE][W] ~8m: ADR-012: Status line gains "Ratchet implemented S-TRUE-WIRE (Steps 259-275): `docs/policy/ebpf-insns-baseline.tsv` + `scripts/check-ebpf-insns.sh`; ascend 926,542 is over the 900K budget by design and allowlisted in `ebpf-insns-over-budget.txt`"; `wiki/BPF-Verifier-Gates.md` (or the page that documents `check-ebpf-loads.sh`) gets the new gate; `docs/MARSHAL-COOKBOOK.md` gate count.
- [ ] **Step 277** [C] ~1m: **COMMIT CHECKPOINT** `[PLAN S-TRUE-WIRE] Steps 259-277: verified-insn baseline + growth ratchet (ADR-012 promise kept)`.
- [ ] **Step 278** [TEST][S][B] ~8m: The whole eBPF gate chain, as the meta-gate runs it: `cd /home/govan/tmp/unheaded && scripts/check-ebpf-loads.sh 2>&1 | tail -1 && scripts/check-ebpf-insns.sh 2>&1 | tail -1 && scripts/bpf-verifier-check.sh 2>&1 | tail -1 && scripts/upc-regression.sh 2>&1 | grep -aE 'ALL GREEN|FAIL' | head -2`
- [ ] **Step 279** [V] ~1m: Four green lines.
- [ ] **Step 280** [SECURITY][W] ~2m: Phase 11 row: new gate scripts (read-only, root for load only), no new dependency.
- [ ] **Step 281** [V] ~2m: **PHASE 7 EXIT GATE** — Steps 263, 267, 269, 272, 275, 279 passed; baseline committed; `check-gates-can-fail.sh` knows the new gate.
- [ ] **Step 282** [C] ~1m: **COMMIT CHECKPOINT** `[PLAN S-TRUE-WIRE] Steps 278-282: Phase 7 exit gate`.

---

## PHASE 8: FND-020 AND SOPHIA DICTIONARY SIGNING (Steps 283-314)

**Goal**: Wotan replication carries message signatures so a signed event replayed from a peer is still signed (M-188, FND-020); Sophia dictionary deltas are signed with ML-DSA-65 through `pkg/crypto/pqc` instead of a bare SHA-256, with a mandatory-signature rollout behind a flag and the severity ladder ADR-062 asked for (M-149). Key distribution for the Sophia signer uses the ADR-103 key layout and is [AFTER-IRON-GATE].
**Prerequisite**: Phase 0 exit gate. Lane: `fnd020-sophia` worktree. [P:10]
**Time**: 5 h
**Agent**: Agent

### FND-020: replication keeps signatures (M-188)

- [x] **Step 283** [R][B] ~5m: `cd /home/govan/tmp/unheaded && sed -n '20,80p' services/wotan/proto/replication.proto; grep -nE 'signature|public_key|algorithm|signed_at' services/wotan/proto/topic.proto | head -8; grep -rnE 'WALEntry\{|\.Data =|wal\.(Append|Entry)' --include=*.go services/wotan/internal/replication services/wotan/internal/wal 2>/dev/null | grep -v _test | head -10; grep -nE 'protoc|buf generate' Makefile scripts/*.sh | head -3`
- [x] **Step 284** [DECIDE] ~4m: *(Superseded 2026-10-28 by `2af5142b`, PR #2 review: the fields duplicated `store.Message.Signed` and were removed; the standby verifies the signature inside the stored message.)* **Additive fields on `WALEntry`, verified on the standby before apply.** RECOMMENDATION: `WALEntry` gains `bytes signature = 7; bytes public_key = 8; string algorithm = 9; int64 signed_at_unix_ms = 10; string sender_id = 11;` copied verbatim from the `TopicEvent` the entry serialises; the standby's `CatchUp` apply path runs `wotansig.VerifyEvent` with the same trusted-key set the primary used when the entry's topic requires a signature (`signing.RequiresSignature`), and rejects (counts + logs, does not apply) an entry that fails; old primaries sending entries without the fields are accepted only for topics that do not require signatures (the policy decides, not the sender). Rationale: FND-020's text; proto3 additive fields are wire-compatible; the verification primitive exists. Override ONLY if the WAL `data` blob already embeds the full `TopicEvent` proto (Step 283 would show `proto.Marshal(event)` into `Data`); then the fix is "verify what is already inside" and no proto change is needed.
- [x] **Step 285** [CODE][W] ~15m: Red: `services/wotan/internal/replication/signature_test.go`: a signed `config.x` event round-trips through `WALEntry` and verifies on the standby; a tampered payload is rejected and counted (`unheaded_wotan_replication_rejected_total{reason="signature"}`); an unsigned entry on a non-signing topic applies. → FAIL (fields absent).
- [x] **Step 286** [CODE][W] ~25m: Green: proto fields; regenerate (`make proto` or the command from Step 283); primary fills them from the event; standby verifies; metric; log line with topic + seq.
- [x] **Step 287** [TEST][B] ~5m: `cd /home/govan/tmp/unheaded && go test ./services/wotan/... -race -count=1 2>&1 | grep -E '^(FAIL|---)' || echo "all ok"; go vet ./services/wotan/... && git status --short services/wotan/proto | head -5`
- [x] **Step 288** [V] ~1m: `all ok`; generated files changed only in the expected `.pb.go`.
- [x] **Step 289** [TEST][B] ~10m: Two Wotans from the tree, primary + standby on scratch ports, publish one signed event, catch up, read the standby's rejected counter (0) and its copy's signature (non-empty)
  ```bash
  cd /home/govan/tmp/unheaded && grep -nE 'replication|standby|primary|peer' services/wotan/cmd/wotan/main.go | grep -E 'flag\.' | head -6
  ```
  Then start both with the flags shown, publish via `curl -s -X POST http://127.0.0.1:28000/api/v1/topics/config.test/publish …` using `cmd/wotan-publish`-style signing if a CLI exists (`ls cmd | grep -i wotan`), else the Go test from Step 285 is the witness and this step records "live leg: in-process only".
- [x] **Step 290** [V] ~1m: Standby copy carries the signature; `rejected_total 0`.
- [ ] **Step 291** [AUDIT-UPDATE][W] ~4m: FND-020 → `Mitigated` in `compliance/findings/register.yaml` with Steps 285-290; regenerate ADR-098 (Step 170 command).
- [x] **Step 292** [C] ~1m: **COMMIT CHECKPOINT** `[PLAN S-TRUE-WIRE] Steps 283-292: WAL replication carries and verifies ML-DSA-65 signatures (FND-020)`.

### Sophia dictionary signing (M-149)

- [x] **Step 293** [R][B] ~5m: `cd /home/govan/tmp/unheaded && sed -n '25,50p;245,265p;430,450p' pkg/protocol/sophiasync/sophiasync.go; grep -rn 'sophiasync\.' --include=*.go services/sophia cmd | grep -v _test | head -6; grep -nE 'ML-DSA|signature' docs/protocol/draft-bellis-unheaded-sophia-dictionary-03.md | sed -n '1,8p'; grep -nE '^func (New)?MLDSA|^func .*Sign\(|^func .*Verify\(' pkg/crypto/pqc/ml_dsa.go`
- [x] **Step 294** [DECIDE] ~5m: **ML-DSA-65 over the canonical delta bytes; SHA-256 stays as the integrity digest field, not the signature.** RECOMMENDATION: `DictionaryDelta` keeps `Signature []byte` but it now holds an ML-DSA-65 signature over `Canonical(delta)` (the existing serialisation minus the signature field), with new fields `PublicKey []byte`, `Algorithm string ("ML-DSA-65")`, `SignedAtUnixMs int64` mirroring `pkg/wotansig.Signed`; `computeSignature` becomes `Digest()` (SHA-256, kept for the delta id); `Verify(delta, trusted [][]byte)` uses `pqc.MLDSAVerifier`; a `Signer` wraps `pqc.MLDSASigner`. Rollout: `SOPHIA_REQUIRE_DELTA_SIGNATURE` env (default `false` this sprint: "observe": unsigned deltas are counted + logged; `true` rejects) with the severity ladder document saying when it flips. Rationale: sophia-dictionary-03 :566/:613; the primitive and the field shape already exist in `wotansig`; ADR-062 :115 warned that mandatory signatures reject every unsigned delta today, so observe first. Override ONLY if `services/sophia` does not use `sophiasync` at all (Step 293 grep empty), in which case the package is reference-only (Step 69 ruling) and this becomes a conformance test change, not a service change.
- [x] **Step 295** [CODE][W] ~15m: Red: `pkg/protocol/sophiasync/signing_test.go`: sign/verify round trip with a fresh ML-DSA-65 key, tamper → fail, wrong key → fail, unsigned + require=false → accepted and counted, unsigned + require=true → rejected; wire-format test that a signed delta still serialises (`sigLen` field grows: assert the length prefix handles 3,309-byte signatures). → FAIL.
- [x] **Step 296** [CODE][W] ~30m: Green: the fields, `Signer`, `Verify`, `Digest`, the env flag read at the one `services/sophia` call site (if any), metrics `unheaded_sophia_delta_unsigned_total`, `…_delta_signature_failures_total`.
- [x] **Step 297** [TEST][B] ~4m: `cd /home/govan/tmp/unheaded && go test ./pkg/protocol/sophiasync/ ./services/sophia/... -race -count=1 2>&1 | grep -E '^(FAIL|---)' || echo "all ok"`
- [x] **Step 298** [V] ~1m: `all ok`.
- [x] **Step 299** [W][SECURITY] ~25m: `docs/security/severity-ladder.md` (ADR-062 :199): the Kingdom's severity ladder made explicit (CRITICAL/HIGH block ship; MEDIUM logged with compensating control; LOW tracked), how findings map to it (ADR-098 register severities), and the signature-rollout rule: observe → enforce after 7 days of zero `unsigned_total` growth in the live stack, flipped by a commit that sets the env in the unit (never a silent default change). Link from ADR-062 and ADR-098.
- [x] **Step 300** [AFTER-IRON-GATE][CODE][W] ~20m: Sophia's signing key comes from the ADR-103 per-service key layout (`/etc/unheaded/sophia/keys/delta-signing.key`, 0400, `unheaded-sophia`), trusted public keys from `trust.d`; until S-IRON-GATE lands, the signer is constructed only when `SOPHIA_DELTA_SIGNING_KEY` points at a file (dev/test), and the service logs "delta signing: off (no key)".
- [x] **Step 301** [AFTER-IRON-GATE][TEST][B] ~4m: `go test ./services/sophia/... -count=1 | tail -1`.
- [x] **Step 302** [AFTER-IRON-GATE][V] ~1m: ok; `scripts/check-gitleaks.sh | tail -1` PASS (no key material in tests: generate at test time).
- [x] **Step 303** [AFTER-IRON-GATE][DOC-UPDATE][W] ~5m: `wiki/Service-Sophia.md` signing section; `docs/protocol/draft-bellis-unheaded-sophia-dictionary-03.md` implementation note ([SPEC] text).
- [x] **Step 304** [AFTER-IRON-GATE][C] ~1m: **COMMIT CHECKPOINT** `[PLAN S-TRUE-WIRE] Steps 300-304: Sophia delta signer keyed from the ADR-103 layout`.
- [x] **Step 305** [C] ~1m: **COMMIT CHECKPOINT** `[PLAN S-TRUE-WIRE] Steps 293-299, 305: Sophia deltas signed with ML-DSA-65 (observe mode), severity ladder documented`.

### Phase 8 close

- [x] **Step 306** [TEST][B] ~8m: Regression vs baseline (`tw-p8.txt`).
- [x] **Step 307** [V] ~1m: `no new FAIL`.
- [x] **Step 308** [SECURITY][W] ~3m: Phase 11 rows: new proto fields (additive; standby verifies before apply: rejection path tested); new env flags (`SOPHIA_REQUIRE_DELTA_SIGNATURE`, `SOPHIA_DELTA_SIGNING_KEY`), both fail-safe defaults.
- [x] **Step 309** [COMPLIANCE][B] ~3m: `cd /home/govan/tmp/unheaded && go mod tidy -diff | head -3; scripts/check-dependency-baseline.sh 2>&1 | tail -1`
- [x] **Step 310** [V] ~1m: tidy empty; baseline `[PASS]` (no new module: `pkg/crypto/pqc` already pulls `mldsa65`).
- [x] **Step 311** [TEST][B] ~5m: Fuzz the new parsers for 60 s each: `cd /home/govan/tmp/unheaded && go test ./pkg/protocol/sophiasync/ -run '^$' -fuzz 'FuzzDeserialize' -fuzztime 60s 2>&1 | tail -2` (write `FuzzDeserialize` in Step 295 if absent; the signature length prefix is the new attack surface).
- [x] **Step 312** [V] ~1m: No crashers.
- [x] **Step 313** [V] ~2m: **PHASE 8 EXIT GATE** — Steps 288, 290, 298, 307, 310, 312 passed; FND-020 Mitigated; severity ladder committed; [AFTER-IRON-GATE] steps either done or marked skipped-until.
- [x] **Step 314** [C] ~1m: **COMMIT CHECKPOINT** `[PLAN S-TRUE-WIRE] Steps 306-314: Phase 8 exit gate` and merge the `fnd020-sophia` lane.

---

## PHASE 9: LICENCE HEADERS — RUST SPDX, THE RELICENSE PASS (Steps 315-340)

**Goal**: Every first-party `.rs` file carries the SPDX header and CI checks `.rs` as it checks `.go` (M-160); the ~290 MIT-headed non-Go files get a scripted, reviewable relicense diff that skips anything deliberately MIT, for Stevie to approve (E4) and the plan to apply (M-161); the 21 Go/Rust files with "MIT License" prose under a GPL SPDX line are fixed; `RELEASE_CHECKLIST.md` stops saying MIT.
**Prerequisite**: Phase 0 exit gate. Lane: `spdx` worktree. [P:4]
**Time**: 3 h + wait on E4
**Agent**: Agent

### Rust SPDX (M-160)

- [x] **Step 315** [R][B] ~3m: `cd /home/govan/tmp/unheaded && find . -name '*.rs' -not -path '*/target/*' -not -path './llama.cpp/*' -not -path './doom/*' -not -path './crates/xv6-mbc/upstream/*' | xargs grep -L 'SPDX-License-Identifier' 2>/dev/null | cut -d/ -f2,3 | sort | uniq -c | sort -rn | head -12; head -3 ebpf/monad-common/src/lib.rs`
- [x] **Step 316** [DECIDE] ~3m: **Same two-line header as Go, `//` comments, inserted above any `#![…]` crate attribute.** RECOMMENDATION: `// SPDX-License-Identifier: GPL-3.0-or-later` / `// Copyright (c) 2024-2026 Stevie Bellis.` as the first two lines (before `#![no_std]` etc.: inner attributes may follow comments). The vendored trees (`crates/xv6-mbc/upstream`, `llama.cpp`, `doom/`) are excluded by path and listed in the script. Rationale: matches the 838 Go files (S52) and the Rust files that already have it. Override ONLY if a crate is dual-licensed by its own `Cargo.toml` `license` field (check: `grep -h '^license' crates/*/Cargo.toml ebpf/*/Cargo.toml | sort | uniq -c`; a non-GPL crate gets its own identifier).
- [x] **Step 317** [W] ~15m: `scripts/spdx-backfill.sh` (idempotent: skips files that already have a header; `--dry-run` lists; `--apply` edits; exclusion list in the script header), and `scripts/check-spdx.sh` (the gate: scans `*.go`, `*.rs`, `*.py`, `*.sh` under the same exclusions; prints missing files; exit 1 on any; `--langs` to limit). The CI job in `ci.yml:157-177` calls `scripts/check-spdx.sh` instead of the inline `find`.
- [x] **Step 318** [TEST][B] ~3m: `cd /home/govan/tmp/unheaded && scripts/spdx-backfill.sh --dry-run | wc -l; scripts/check-spdx.sh --langs rs 2>&1 | tail -2; echo "rc=$?"`
- [x] **Step 319** [V] ~1m: dry-run count equals Step 23's `rs-no-spdx`; gate `rc=1` naming them (red before green).
- [x] **Step 320** [B] ~3m: `cd /home/govan/tmp/unheaded && scripts/spdx-backfill.sh --apply | tail -2 && scripts/check-spdx.sh --langs rs 2>&1 | tail -1; echo "rc=$?"`
- [x] **Step 321** [V] ~1m: `rc=0`.
- [x] **Step 322** [BUILD][B] ~6m: Nothing but comments changed: `cd /home/govan/tmp/unheaded/ebpf && cargo check --target=x86_64-unknown-linux-gnu 2>&1 | tail -1; cd .. && for c in crates/*/Cargo.toml cmd/*/Cargo.toml; do (cd $(dirname $c) && cargo check 2>&1 | tail -1 | sed "s|^|$(dirname $c): |"); done 2>/dev/null | grep -v Finished || echo "all crates check"`
- [x] **Step 323** [V] ~1m: `all crates check` (a header above `#![no_std]` is legal; a header inside a `//!` doc block is not: the script inserts at line 1).
- [x] **Step 324** [TEST][B] ~3m: `.py` and `.sh` coverage as information only: `scripts/check-spdx.sh --langs py,sh 2>&1 | tail -3` → record the counts in the progress file; the gate for those languages is enabled in Step 326 only if the count is 0 after a backfill, else they stay `--langs go,rs` and the remainder goes to the Librarian epic.
- [x] **Step 325** [B] ~4m: `cd /home/govan/tmp/unheaded && scripts/spdx-backfill.sh --apply --langs py,sh | tail -2 && scripts/check-spdx.sh --langs go,rs,py,sh 2>&1 | tail -1; echo "rc=$?"` (shell files get `# SPDX…` after the shebang; Python the same; the script handles both).
- [x] **Step 326** [W] ~4m: `ci.yml` spdx-check job → `run: ./scripts/check-spdx.sh --langs go,rs$( [ rc = 0 ] && echo ,py,sh)` written literally as the langs that passed in Step 325; add the gate to `scripts/check-gates-can-fail.sh` (provocation: a temp file without a header).
- [x] **Step 327** [C] ~1m: **COMMIT CHECKPOINT** `[PLAN S-TRUE-WIRE] Steps 315-327: SPDX headers on every first-party .rs (and .py/.sh where green); check-spdx.sh gate covers them`.

### MIT wording under GPL SPDX (M-161, the 21 files)

- [x] **Step 328** [R][B] ~3m: `cd /home/govan/tmp/unheaded && grep -rlE 'licensed under the MIT License|MIT License' --include=*.go --include=*.rs . 2>/dev/null | grep -v -E 'target/|llama.cpp|upstream|THIRD_PARTY|docs/legal' | head -25 | tee /tmp/claude-1000/-home-govan-tmp/f267c16f-13e4-4b29-87ee-e434cae4bce1/scratchpad/mit-prose.txt | wc -l`
- [x] **Step 329** [W][B] ~8m: For each file in the list whose SPDX line says GPL-3.0-or-later, delete the MIT prose paragraph (keep the SPDX + copyright); for a file whose SPDX says MIT, leave it for the E4 table. `sed`-free: a 20-line Python script in `scripts/` run with `-I`, printing each change.
- [x] **Step 330** [V] ~1m: `grep -rlE 'MIT License' --include=*.go --include=*.rs . | grep -v -E 'target/|llama.cpp|upstream|THIRD_PARTY|docs/legal'` lists only files whose own SPDX is MIT (or nothing).

### The relicense diff for E4 (M-161)

- [x] **Step 331** [R][B] ~5m: `cd /home/govan/tmp/unheaded && grep -rl 'SPDX-License-Identifier: MIT' --exclude-dir=node_modules --exclude-dir=llama.cpp --exclude-dir=target --exclude-dir=.git --exclude-dir=doom --exclude-dir=upstream . | sed 's|^\./||' | sort > /tmp/claude-1000/-home-govan-tmp/f267c16f-13e4-4b29-87ee-e434cae4bce1/scratchpad/mit-files.txt; wc -l < /tmp/claude-1000/-home-govan-tmp/f267c16f-13e4-4b29-87ee-e434cae4bce1/scratchpad/mit-files.txt; cut -d/ -f1-2 /tmp/claude-1000/-home-govan-tmp/f267c16f-13e4-4b29-87ee-e434cae4bce1/scratchpad/mit-files.txt | sort | uniq -c | sort -rn | head -20; cat docs/legal/LICENSE-MIT.md | head -20; grep -rn 'MIT' LICENSES/ 2>/dev/null | head -3; grep -n 'MIT' THIRD_PARTY.md | head -5`
- [x] **Step 332** [W] ~20m: `scripts/relicense-mit-to-gpl.sh` with `--dry-run` (default) and `--apply`: input list = Step 331's file set MINUS an explicit `KEEP_MIT` list in the script (anything `docs/legal/LICENSE-MIT.md` or `THIRD_PARTY.md` names as deliberately MIT, third-party-derived files identified by a `Copyright` line not naming Stevie Bellis, `LICENSES/*`); the edit is the one SPDX line + adding the Stevie copyright line if absent; dry-run prints `path<TAB>action<TAB>reason`; also writes `docs/legal/RELICENSE-2026-10-27.md` with the per-directory counts, the KEEP_MIT list with reasons, and the question for E4.
- [x] **Step 333** [TEST][B] ~3m: `cd /home/govan/tmp/unheaded && scripts/relicense-mit-to-gpl.sh --dry-run | awk -F'\t' '{print $2}' | sort | uniq -c; scripts/relicense-mit-to-gpl.sh --dry-run | awk -F'\t' '$2=="KEEP"' | head -10`
- [x] **Step 334** [ESCALATE] ~3m: **E4 — Apply the relicense.** Context: `docs/legal/RELICENSE-2026-10-27.md` (counts, KEEP list with reasons, the exact diff the script will make). Question: apply to every RELICENSE row? Any file to move to KEEP? Why Stevie: relicensing is the author's act; the Suricata-era MIT isolation reason is gone with Suricata, but the owner says so, not the script.
- [x] **Step 335** [AFTER-E4][B] ~3m: `cd /home/govan/tmp/unheaded && scripts/relicense-mit-to-gpl.sh --apply | tail -3 && git diff --stat | tail -1`
- [x] **Step 336** [AFTER-E4][V] ~2m: Changed-file count equals the RELICENSE row count; `git diff` shows only SPDX/copyright lines (`git diff | grep '^[-+]' | grep -v '^[-+][-+]' | grep -vE 'SPDX|Copyright' | wc -l` is 0).
- [x] **Step 337** [AFTER-E4][TEST][B] ~4m: Nothing functional changed: `cd /home/govan/tmp/unheaded && go build ./... && bash -n $(git diff --name-only | grep '\.sh$') && nix-instantiate --parse $(git diff --name-only | grep '\.nix$' | head -5) >/dev/null 2>&1 && echo "parse ok" || echo "check the nix files by hand (nix may be absent)"`
- [x] **Step 338** [AFTER-E4][DOC-UPDATE][W] ~6m: `docs/launch/RELEASE_CHECKLIST.md` "LICENSE is MIT" → GPL-3.0-or-later with the dual-licensed protocol note; `THIRD_PARTY.md` and `docs/legal/LICENSE-MIT.md` updated to list what remains MIT and why; `scripts/check-spdx.sh` gains a `--forbid MIT` mode for first-party paths (allowlist = KEEP list) so the count can only shrink.
- [x] **Step 339** [AFTER-E4][C] ~1m: **COMMIT CHECKPOINT** `[PLAN S-TRUE-WIRE] Steps 335-339: MIT→GPL-3.0-or-later relicense applied per E4; KEEP list recorded`.
- [x] **Step 340** [V] ~2m: **PHASE 9 EXIT GATE** — Steps 321, 323, 330 passed; `check-spdx.sh` in CI; E4 filed; if E4 answered: Steps 336-337 passed. Merge the `spdx` lane. [C] `[PLAN S-TRUE-WIRE] Steps 328-340: Phase 9 exit gate`.

---

## PHASE 10: SUPPLY-CHAIN REGISTERS AND RATCHETS (Steps 341-380)

**Goal**: The ADR-096 register has rows for every unrecorded direct dependency with the thresholds Stevie sets (M-162, E5); the two ADR-004 admissions are presented for sign-off (M-163, E5); the ADR-090 ledger runs its step 1 and asks Q1-Q3 (M-164, E5); bandit's 43 findings get their disposition applied and the job stops being advisory (M-165); trivy gains a misconfig scanner behind a shrink-only baseline (M-165); `omitempty`/G115/ST10xx become ratchets instead of blanket excludes (M-166); every Python requirements file is hash-pinned (M-167); ScanCode re-runs on HEAD and `THIRD_PARTY.md` records SPDX OR elections (M-169).
**Prerequisite**: Phase 6 (deletions change the dependency set; run this phase after E3 is applied, or re-run Steps 341-345 after). Lane: `supply` worktree. [P:8]
**Time**: 6 h + wait on E5
**Agent**: Agent

### Registers (M-162, M-163)

- [ ] **Step 341** [R][B] ~5m: `cd /home/govan/tmp/unheaded && sed -n '189,240p' docs/adr/ADR-096-dependency-gate-cost-to-fake.md; grep -rl 'gopkg.in/yaml.v3' --include=*.go pkg cmd services | grep -v _test | wc -l; grep -rl 'github.com/cilium/ebpf' --include=*.go . | grep -v _test | wc -l; grep -rl 'goldmark' --include=*.go . | wc -l; grep -nE 'yaml|cilium|goldmark' go.mod`
- [ ] **Step 342** [W] ~25m: Register rows (the ADR-096 §Progress format or `docs/adr/ADR-095-slow-roll-structural-review.md`'s register table, whichever is the live one: the ADR text says) for `gopkg.in/yaml.v3` (archived upstream: date, last release, importers, the two candidate successors `goccy/go-yaml` and `sigs.k8s.io/yaml` with their org and release cadence, cost-to-replace = file count from Step 341), `github.com/cilium/ebpf` (org, release cadence, `GO-2026-6238` reachability via `govulncheck ./... 2>&1 | grep -A3 6238`), `goldmark` (who imports it, why). Each row ends with "threshold: E5".
- [ ] **Step 343** [W] ~10m: ADR-004 :104-105: the two admission reviews are already written; add a "Sign-off" line each with a blank for Stevie's date and the Step number; present both in E5.
- [ ] **Step 344** [COMPLIANCE][B] ~4m: `cd /home/govan/tmp/unheaded && govulncheck ./... 2>&1 | tail -5; cargo audit --file Cargo.lock 2>&1 | tail -3 || true; for d in ebpf crates/zhend cmd/waf; do (cd $d && cargo audit 2>&1 | tail -1 | sed "s|^|$d: |"); done`
- [ ] **Step 345** [V] ~1m: Outputs recorded in the progress file (counts of findings per tool); any new HIGH is its own FND row (register it; do not fix it silently in this phase).

### ADR-090 ledger step 1 (M-164)

- [ ] **Step 346** [R][B] ~4m: `cd /home/govan/tmp/unheaded && sed -n '78,110p' docs/adr/ADR-090-total-source-sweep-slop-removal.md; head -2 docs/sweeps/TOTAL-SOURCE-SWEEP.tsv; awk -F'\t' 'NR>1{c[$2]++} END{for(k in c) print c[k], k}' docs/sweeps/TOTAL-SOURCE-SWEEP.tsv`
- [ ] **Step 347** [W] ~20m: `scripts/sweep-ledger-step1.py` (run with `python3 -I`): marks `SKIPPED` with reason `generated` every PENDING row whose file has a `Code generated … DO NOT EDIT` / `@generated` / `.pb.go` / `_string.go` marker, and `SKIPPED vendored` for rows under `llama.cpp/`, `crates/xv6-mbc/upstream/`, `doom/`, `LICENSES/`, `node_modules/`; prints counts; never touches SWEPT rows; `--dry-run` default.
- [ ] **Step 348** [TEST][B] ~3m: `cd /home/govan/tmp/unheaded && python3 -I scripts/sweep-ledger-step1.py --dry-run | tail -5`
- [ ] **Step 349** [V] ~1m: Counts printed; PENDING would drop by the generated+vendored count; nothing else moves.
- [ ] **Step 350** [B] ~2m: `cd /home/govan/tmp/unheaded && python3 -I scripts/sweep-ledger-step1.py --apply | tail -3 && awk -F'\t' 'NR>1{c[$2]++} END{for(k in c) print c[k], k}' docs/sweeps/TOTAL-SOURCE-SWEEP.tsv`
- [ ] **Step 351** [V] ~1m: `SKIPPED` count > 0; `PENDING` < 2192; total rows unchanged.
- [ ] **Step 352** [C] ~1m: **COMMIT CHECKPOINT** `[PLAN S-TRUE-WIRE] Steps 341-352: ADR-096 register rows drafted, ADR-090 ledger step 1 run`.
- [ ] **Step 353** [ESCALATE] ~4m: **E5 — Supply-chain decisions.** Context: register rows (Step 342), admissions (Step 343), ledger counts (Step 350). Questions: (1) ADR-095/096 thresholds: a dependency is flagged at what age without a release, how few importers, which CVE severity? (2) `gopkg.in/yaml.v3`: keep-and-record, or migrate to a named successor (which)? (3) `cilium/ebpf`: record, or own a minimal loader? (4) `lib/pq` (open since session 5): keep-and-record or own a minimal client? (5) Sign off `aws-lc-rs`/`aws-lc-sys` and `anthropic-sdk-go` admissions. (6) ADR-090 Q1 vendored scope (the step-1 list is the proposal), Q2 `docs/` in scope, Q3 the 29 `#[ignore]` forge tests: restore or delete. Why Stevie: ADR-096 says a person admits; thresholds are policy.
- [ ] **Step 354** [AFTER-E5][W] ~10m: Write the thresholds into ADR-095/096 as a decision record; set the sign-off dates in ADR-004; mark the ledger Q1-Q3 answers in ADR-090 §Progress.
- [ ] **Step 355** [AFTER-E5][CODE][W] ~15m: If (2) says migrate: a `go.mod` change is a new admission → stop and file it as its own [ESCALATE] in the next sprint; if keep-and-record: the register row's status is `recorded`. Same for (3) and (4). No dependency is added in this plan.
- [ ] **Step 356** [AFTER-E5][V] ~1m: `go mod tidy -diff` empty; `scripts/check-dependency-baseline.sh` `[PASS]`.
- [ ] **Step 357** [AFTER-E5][W] ~10m: If Q3 = delete: `git rm` the 29 ignored forge tests (`grep -rln '#\[ignore' crates/zhenai-forge/src | head`), `cargo test` in the crate; if restore: list them as a card for the forge track (parked, LoRA trigger), do not run them here.
- [ ] **Step 358** [AFTER-E5][C] ~1m: **COMMIT CHECKPOINT** `[PLAN S-TRUE-WIRE] Steps 354-358: E5 answers recorded (thresholds, admissions, ledger Q1-Q3)`.

### bandit, trivy, hadolint (M-165)

- [ ] **Step 359** [R][B] ~4m: `cd /home/govan/tmp/unheaded && sed -n '125,140p' docs/security/bandit-disposition-2026-08-03.md; ls .github/ci-python/; (test -x /tmp/claude-1000/-home-govan-tmp/f267c16f-13e4-4b29-87ee-e434cae4bce1/scratchpad/civenv/bin/bandit || python3 -m venv /tmp/claude-1000/-home-govan-tmp/f267c16f-13e4-4b29-87ee-e434cae4bce1/scratchpad/civenv && /tmp/claude-1000/-home-govan-tmp/f267c16f-13e4-4b29-87ee-e434cae4bce1/scratchpad/civenv/bin/pip install -q --require-hashes --no-deps --only-binary=:all: -r .github/ci-python/requirements.txt); /tmp/claude-1000/-home-govan-tmp/f267c16f-13e4-4b29-87ee-e434cae4bce1/scratchpad/civenv/bin/bandit -r . -x ./llama.cpp,./target,./node_modules --skip B603,B607,B404,B311,B605,B101 -f json 2>/dev/null | python3 -I -c 'import json,sys,collections; d=json.load(sys.stdin); c=collections.Counter(r["test_id"] for r in d["results"]); print(dict(c))'`
- [ ] **Step 360** [DECIDE] ~5m: **Apply the disposition doc's own analysis; make the job gate.** RECOMMENDATION: B310 (35): skip by rule ID with the disposition's reason (boundary check at `71f43a11` is the control; add a test that the scheme check still exists: `grep -n 'scheme' raft/*.py | head`); B104 (2): `zhen_app.py` binds `0.0.0.0` → bind `ZHEN_BIND_ADDR` env defaulting to `127.0.0.1`, with the compose/unit setting it to the lxdbr0 or host address explicitly (a real exposure change: it goes in E5's answer list as item 7, default = loopback + explicit env; the master list already names "0.0.0.0 bind (zhen UI) keep/bind lxdbr0" as decision 32); B615 (2): pin the HF revision in `08_train_qlora.py` to the commit hash of the model actually used (from `/var/zhen/models/*/refs` or the download log; if unknown, `revision="main"` plus a `# TODO-free: pinned when the model is re-fetched` is NOT acceptable: instead fail loudly if `HF_REVISION` env is unset); B405/B314 (4): the disposition says `defusedxml` is textbook but a new dependency; without it, wrap the parse in a size cap + `forbid_dtd` via `xml.etree` `XMLParser` is not available… so: keep as `# nosec B314 -- trusted local archive, size-capped` with the cap added in code, no new dependency. Flip `continue-on-error: true` → remove it, and add `--skip` for exactly the rule IDs the disposition names; 0 findings → gating. Rationale: the doc did the analysis in August; this applies it. Override ONLY if Step 359 shows new rule IDs beyond the 43 (then triage those first; the gate flips only at 0).
- [ ] **Step 361** [CODE][W] ~30m: The code changes from Step 360 (bind env, HF revision guard, XML size cap + nosec with reason); `static-analysis.yml:197-201` gating; `docs/security/bandit-disposition-2026-08-03.md` gets a "Applied S-TRUE-WIRE Steps 359-363" section.
- [ ] **Step 362** [TEST][B] ~4m: Re-run the Step 359 bandit command with the new skips → `{}`; `cd raft && ../scratchpad-civenv-path/python3 -m pytest tests -q -x 2>&1 | tail -2` (use the civenv python; the raft tests must still pass with the bind env unset → loopback).
- [ ] **Step 363** [V] ~1m: `{}` and tests pass.
- [ ] **Step 364** [W] ~12m: trivy misconfig: in `security.yml` `trivy-fs` job add a third step `scanners: misconfig`, `exit-code: '0'`, `format: json`, `output: trivy-misconfig.json`, followed by `scripts/check-trivy-misconfig.sh trivy-misconfig.json` (new; shrink-only baseline `docs/policy/trivy-misconfig-baseline.txt` of `AVD-ID<TAB>path` rows, membership-compared, the house pattern). Seed the baseline locally: `docker run --rm -v "$PWD:/repo:ro" aquasec/trivy:<pinned tag from tool-pins or the action's version> fs --scanners misconfig -f json /repo > /tmp/…/trivy.json` (pin the image by digest in `scripts/lib/tool-pins.sh`; add it to `check-tool-freshness.sh`'s list).
- [ ] **Step 365** [TEST][B] ~5m: `cd /home/govan/tmp/unheaded && scripts/check-trivy-misconfig.sh /tmp/claude-1000/-home-govan-tmp/f267c16f-13e4-4b29-87ee-e434cae4bce1/scratchpad/trivy.json 2>&1 | tail -2; echo "rc=$?"; wc -l docs/policy/trivy-misconfig-baseline.txt`
- [ ] **Step 366** [V] ~1m: `rc=0` with the seeded baseline; add the gate to `check-gates-can-fail.sh` (provocation: delete one baseline row → rc=1 "now PASSES but still listed", the same both-directions rule as the load gate).
- [ ] **Step 367** [V] ~1m: hadolint: `grep -n 'HADOLINT_IMAGE_DIGEST' scripts/check-hadolint.sh scripts/lib/tool-pins.sh | head -3` shows the digest pin (already done 2026-10-27; record "closed" in Appendix D).
- [ ] **Step 368** [C] ~1m: **COMMIT CHECKPOINT** `[PLAN S-TRUE-WIRE] Steps 359-368: bandit dispositions applied and gating; trivy misconfig ratchet`.

### Lint ratchets (M-166)

- [ ] **Step 369** [R][B] ~5m: `cd /home/govan/tmp/unheaded && grep -rnE 'omitempty' --include=*.go pkg cmd services | grep -E 'struct \{|time\.Time|\] *[A-Z][A-Za-z]* *\x60json' | grep -vE 'string|int|bool|\*|\[\]' | head -20 | wc -l; go run ./tools/gosec -quiet -include=G115 -fmt=json ./... 2>/dev/null | python3 -I -c 'import json,sys; print(len(json.load(sys.stdin).get("Issues",[])))' 2>/dev/null || echo "gosec run failed; use scripts/check-gosec-ratchet.sh --count"`
- [ ] **Step 370** [DECIDE] ~4m: **Fix the 18 `omitempty` misuses now; G115 and ST10xx become counted ratchets, not blanket excludes.** RECOMMENDATION: `omitempty` on struct/`time.Time` fields does nothing (Go 1.24+ has `omitzero`): switch to `omitzero` where Go ≥ 1.24 (check `go.mod` `go` line) else remove the tag; G115: remove the blanket exclude in `.golangci.yml:99`, add `scripts/check-gosec-ratchet.sh` support for a per-rule count baseline (`docs/policy/gosec-g115-baseline.txt` = current count; may only shrink), so new conversions need a `// #nosec G115 -- reason` (memory: `#nosec` must LEAD the comment); ST1000/ST1003 stay excluded (style), ST1016/ST1020-22 are removed from excludes if `golangci-lint run ./... 2>&1 | grep -c ST10` is under 50 (fix them), else ratchet like G115. Rationale: ADR-098's rule "fix all findings, ratchet by rule-ID exclusion never by severity filter" applied to the linters. Override ONLY if `go.mod` is below 1.24 (then `omitempty` → removed, not `omitzero`).
- [ ] **Step 371** [CODE][W] ~25m: The 18 field tags; the G115 baseline + ratchet mode; the ST10xx decision applied; `.golangci.yml` edited with a comment per change.
- [ ] **Step 372** [TEST][B] ~8m: `cd /home/govan/tmp/unheaded && golangci-lint run ./... 2>&1 | tail -3; scripts/check-gosec-ratchet.sh 2>&1 | tail -2; go test ./pkg/... -count=1 2>&1 | grep -E '^FAIL' || echo "pkg ok"`
- [ ] **Step 373** [V] ~1m: lint 0 issues (or only the ratcheted counts at baseline); gosec ratchet `[PASS]`; `pkg ok`.

### Python pins (M-167) and ScanCode (M-169)

- [ ] **Step 374** [CODE][W] ~15m: `notebooks/requirements.in` and `tomb/grimoire/rag/requirements.in` written from the current unpinned lists with exact `==` versions (resolve with `pip index versions <pkg>` in the civenv, or the versions installed in `~/.venv/zhen` for the ones Zhen uses: `~/.venv/zhen/bin/pip freeze | grep -iE '<names>'`); extend `scripts/ci-python-lock.sh` to take a directory argument and produce the hash-locked `requirements.txt` for each (it needs `pip-tools` in a container or the civenv: `civenv/bin/pip install pip-tools==<pinned>` is itself an admission → use `pip download` + `pip hash` loop the script already does for CI, or `python3 -m pip install --dry-run --report`; read `scripts/ci-python-lock.sh` first and reuse its method).
- [ ] **Step 375** [TEST][B] ~6m: `cd /home/govan/tmp/unheaded && scripts/ci-python-lock.sh notebooks 2>&1 | tail -2 && scripts/ci-python-lock.sh tomb/grimoire/rag 2>&1 | tail -2 && grep -c -- '--hash=sha256' notebooks/requirements.txt tomb/grimoire/rag/requirements.txt && scripts/check-python-audit.sh 2>&1 | tail -2`
- [ ] **Step 376** [V] ~1m: Both files hash-pinned (count > 0); `check-python-audit.sh` `[PASS]` or its accepted list updated with reasons (shrink-only file `docs/policy/python-audit-accepted.txt`).
- [ ] **Step 377** [COMPLIANCE][B] ~15m: ScanCode on HEAD (the tool is not installed: run the pinned container)
  ```bash
  cd /home/govan/tmp/unheaded && grep -nE 'scancode' scripts/lib/tool-pins.sh scripts/*.sh Makefile | head -3; docker run --rm -v "$PWD:/src:ro" -v /tmp/claude-1000/-home-govan-tmp/f267c16f-13e4-4b29-87ee-e434cae4bce1/scratchpad:/out ghcr.io/aboutcode-org/scancode-toolkit:<version pinned in tool-pins.sh; add it if absent, by digest> -clpieu --json-pp /out/scancode-$(date +%F).json --ignore 'llama.cpp/*' --ignore 'target/*' --ignore 'node_modules/*' /src 2>&1 | tail -3
  ```
- [ ] **Step 378** [W] ~15m: Replace `docs/legal/scancode-full.json` with the new output (or keep both with dates if the old one is cited), diff the licence summary (`python3 -I -c` over the two JSONs: counts per detected licence), record in `docs/legal/SCANCODE-2026-10-27.md`; `THIRD_PARTY.md`: for every dependency whose licence is an SPDX `OR` expression, add the election column (which branch the Kingdom takes and why: GPL-compatible branch always); fair-use notice added to `docs/protocol/the_first_packet.md` if it quotes third-party text (check `grep -n 'Copyright\|©' docs/protocol/the_first_packet.md`).
- [ ] **Step 379** [V] ~2m: **PHASE 10 EXIT GATE** — Steps 345, 351, 356 (if E5 answered), 363, 366, 373, 376 passed; register rows exist; ScanCode output dated today committed; E5 filed. [C] `[PLAN S-TRUE-WIRE] Steps 369-379: lint ratchets, Python hash pins, ScanCode on HEAD`.
- [ ] **Step 380** [C] ~1m: **COMMIT CHECKPOINT** `[PLAN S-TRUE-WIRE] Step 380: Phase 10 exit gate` and merge the `supply` lane.

---

## PHASE 11: SECURITY AND COMPLIANCE REVIEW (Steps 381-396)

**Goal**: Every attack-surface change this sprint made is listed with its proof step; the gates that guard the repo are re-run on the final tree; the fuzz targets that cover the new parsers have run; nothing new reads user data.
**Prerequisite**: Phases 1-10 exit gates (unanswered escalations are allowed; their [AFTER-E] steps are listed as pending in the table).
**Time**: 2.5 h
**Agent**: Coordinator

- [ ] **Step 381** [SECURITY][W] ~25m: Write the attack-surface table into this file (below) from the per-phase rows collected in the progress file: `change | surface | direction (new/changed/removed) | fail mode | proof step`. Mandatory rows: flow-action and event-type renumber (wire bytes; dual-accept decoders; Steps 86, 93, 98); Shield `bpf_loop` checks (new map reads on the hot path; observe mode default; Steps 198-202); `pkg/shieldmaps` writer + `/etc/unheaded/shield/keys.yaml` (new secret-bearing file on hosts; 0400; nologin user; Steps 188-191); `cmd/shield` SIGHUP (same-uid signal); Shield error-code emission (same map, real codes; Step 138); GOAWAY/authority (enforce or removed; Step 209); PQC_SIG_STATUS removed (a claimed-but-absent guarantee is gone; Step 236); packet-marker option parsing (new packet-bytes read in XDP, bounds-checked per iteration; verifier-accepted; Step 219); WAL replication signature fields (standby verifies before apply; Step 290); Sophia delta signatures (observe mode; new length-prefixed field fuzzed; Step 312); Wotan `X-Wotan-Error` header (codes only; Step 147); Wotan SIGHUP reload (malformed config keeps old; Step 157); new gate scripts (read-only; Steps 269, 321, 366); E3 deletions (surface removed; Step 250); relicense (no code change; Step 336); bandit bind change (zhen UI loopback by default; Step 363).
- [ ] **Step 382** [SECURITY][B] ~6m: `cd /home/govan/tmp/unheaded && scripts/check-gitleaks.sh 2>&1 | tail -1; scripts/check-secrets-baseline.sh 2>&1 | tail -1; scripts/check-gosec-ratchet.sh 2>&1 | tail -1; scripts/check-dependency-baseline.sh 2>&1 | tail -1; scripts/check-spdx.sh 2>&1 | tail -1; scripts/check-systemd-units.sh 2>&1 | tail -1; scripts/check-runbooks.sh 2>&1 | tail -1; scripts/check-crypto-inventory.sh 2>&1 | tail -1; scripts/check-pq-kex.sh 2>&1 | tail -1`
- [ ] **Step 383** [V] ~2m: Nine `[PASS]` lines. Any FAIL → fix in this phase (a gate the sprint broke is the sprint's bug), re-run.
- [ ] **Step 384** [SECURITY][S][B] ~10m: `cd /home/govan/tmp/unheaded && scripts/check-ebpf-loads.sh 2>&1 | tail -1 && scripts/check-ebpf-insns.sh 2>&1 | tail -1 && scripts/upc-regression.sh 2>&1 | grep -aE 'ALL GREEN|FAIL|verified [0-9]+ insns' | head -4`
- [ ] **Step 385** [V] ~2m: `[PASS]` ×2, `ALL GREEN`, ascend/Doom numbers equal Step 16 (or the recorded reasoned change).
- [ ] **Step 386** [SECURITY][B] ~15m: Fuzz the parsers this sprint touched, 2 min each: `cd /home/govan/tmp/unheaded && for t in 'pkg/protocol/sophiasync:FuzzDeserialize' 'pkg/protocol/bpfschema:FuzzFlowActionLegacy' 'pkg/wotanerr:FuzzDecode' 'pkg/shieldmaps:FuzzKeysYAML' 'pkg/protocol/errors:FuzzCodeFromByte'; do p=${t%%:*}; f=${t##*:}; grep -q "func $f" $p/*_test.go 2>/dev/null && (go test ./$p/ -run '^$' -fuzz "$f" -fuzztime 120s 2>&1 | tail -1 | sed "s|^|$t: |") || echo "$t: no fuzz target (write one if the parser is new)"; done`
- [ ] **Step 387** [V] ~2m: No crashers; every new parser has a target (write the missing ones now, 10 min each, re-run).
- [ ] **Step 388** [SECURITY][B] ~6m: Rust: `cd /home/govan/tmp/unheaded/ebpf && cargo clippy --target=x86_64-unknown-linux-gnu -- -D warnings 2>&1 | tail -3; cd ../tomb/lich 2>/dev/null && ls | grep -iE 'cart|shield' | head -3` — if a Lich fuzz target covers Shield or the cartridge decoder (`fuzz_cart_image`, `fuzz_cart_differential`), run each 120 s: `cargo fuzz run <target> -- -max_total_time=120 2>&1 | tail -2`.
- [ ] **Step 389** [V] ~1m: clippy clean; fuzzers clean.
- [ ] **Step 390** [COMPLIANCE][B] ~5m: `cd /home/govan/tmp/unheaded && go mod tidy -diff | wc -l; make license-check 2>&1 | tail -2; scripts/check-tool-freshness.sh 2>&1 | grep -E 'STALE|FAIL' || echo "tools fresh"`
- [ ] **Step 391** [V] ~1m: `0`, licence check passes, `tools fresh` (a STALE here is S-CLEAN-SLATE's recurring item: bump the pin in this phase if the cooldown has passed, else note it).
- [ ] **Step 392** [SECURITY][R] ~8m: Zero-user-data audit of every new read path: `pkg/shieldmaps` reads a host config file (metadata: keys, not traffic); packet-marker reads TCP/IP option bytes (headers, not payload: confirm the loop bound stops at the header length, Step 216); Sophia deltas and WAL entries are Kingdom control-plane data. Write the three sentences into the table.
- [ ] **Step 393** [SECURITY][W] ~10m: Threat-model touch: `docs/security/threat-model-*.md` (the Shield/data-plane one) gets the `bpf_loop` checks and the keys file as assets/controls; `tomb/grimoire/threat-models/kingdom-threat-model.md` line for Shield updated from "stubbed" to "observe mode".
- [ ] **Step 394** [AUDIT-UPDATE][B] ~3m: `cd /home/govan/tmp/unheaded && go run ./cmd/compliance-evidence findings -adr docs/adr/ADR-098-findings-register-and-baseline-modes.md 2>&1 | tail -1 && grep -E 'FND-020|FND-025' docs/adr/ADR-098-findings-register-and-baseline-modes.md | cut -c1-80`
- [ ] **Step 395** [V] ~2m: **PHASE 11 EXIT GATE** — Steps 383, 385, 387, 389, 391 passed; the table is complete (every Phase 1-10 row present); FND-020 and FND-025 show `Mitigated`.
- [ ] **Step 396** [C] ~1m: **COMMIT CHECKPOINT** `[PLAN S-TRUE-WIRE] Steps 381-396: security and compliance review`.

### Attack-surface table (filled by Step 381)

| Change | Surface | Direction | Fail mode | Proof step |
|---|---|---|---|---|
| _(filled at execution)_ | | | | |

---

## PHASE 12: DOCS, REGISTERS, HANDOFF (Steps 397-404)

**Goal**: Every downstream document tells the truth about what this sprint changed; the master list rows are marked; the timeline carries one entry; next.md hands off; the remaining escalations are listed once, in one place.
**Prerequisite**: Phase 11 exit gate.
**Time**: 2 h
**Agent**: Coordinator (Librarian role for the ripple)

- [ ] **Step 397** [DOC-UPDATE][W] ~30m: Ripple (Librarian's eight layers): `CLAUDE.md` (the "Age 2 Remaining Epics" lines "Foundation spec draft-06 (IANA integration)", "Wotan draft-03 (error code taxonomy)" → done with step refs; the S67 paragraph's "Monad Flow Actions (13 entries)" stays; add one S-TRUE-WIRE paragraph in the house format: what, where, numbers); `wiki/Home.md` Recent Progress line; `wiki/Service-Shield.md`, `wiki/Service-Wotan.md` (reload, error header), `wiki/Service-Sophia.md` (signing), `wiki/BPF-Verifier-Gates.md` (insns ratchet), `wiki/Port-Registry.md` (shield uid), `wiki/Security-Tooling.md` (bandit gating, trivy misconfig, spdx langs, new gates); `docs/adr/ADR-INDEX.md` statuses (ADR-012 note, ADR-108 line, ADR-036 untouched here: Librarian epic); `docs/protocol/RFC-ALIGNMENT-FINDINGS.md` already closed rows; `docs/UNREACHED-GO-PACKAGES.md` regenerated (Step 252 if E3 answered, else a note); `docs/MARSHAL-COOKBOOK.md` gate count (+3: insns, spdx, trivy-misconfig).
- [ ] **Step 398** [ESCALATE] ~3m: **E7 — Formal ADR acceptances.** Context: ADRs 093, 094, 101, 104, 105, 106, 107 are built and running (the master list M-148 cites each); ADR-108 waits on E1. Question: accept them (status line + ADR-INDEX) in one commit? Why Stevie: acceptance is the owner's act; the plan prepares the diff (`docs/adr/ACCEPTANCE-2026-MM-DD.diff` with every status line change) so the answer is one word.
- [ ] **Step 399** [AUDIT-UPDATE][W] ~15m: `docs/sweeps/OUTSTANDING-2026-10-27.md`: every M-id in Appendix D gets a status suffix in its row (`DONE S-TRUE-WIRE Step n` / `PARTIAL: …` / `AFTER-E<n>` / `deferred to S-<other>`); §6 counts updated.
- [ ] **Step 400** [DOC-UPDATE][W] ~10m: `references/timeline.md`: one Age 3 entry dated the day of the Phase 11 gate ("S-TRUE-WIRE: flow-action and event-type wire values follow foundation-06; RFC Editor S5 queue cleared; Shield checks real (observe); ADR-012 insn ratchet; Rust SPDX; FND-020/025 mitigated; E1-E7 open/answered"), then the mirrors: `cd /home/govan/tmp/unheaded && grep -nE 'sync|mirror' services/timeguru/cmd/timeguru/main.go | head -3` and run the timeguru sync (or `make`-less: `go run ./services/timeguru/cmd/timeguru -sync-once` if that flag exists; the S22 commit f83c9cde shows the four files that must change together); `scripts/check-timeline-freshness.sh | tail -2`.
- [ ] **Step 401** [V] ~1m: FRESH; `git status --short references/ | wc -l` is 4 (md + json + yaml + toml).
- [ ] **Step 402** [DOC-UPDATE][W] ~15m: `~/tmp/next.md`: a new top section "S-TRUE-WIRE (date): state, what shipped (step refs), the seven escalations with one line each and what each unblocks, live state (no BPF attached, no netns, no scratch Wotan: verify with `sudo bpftool prog list | grep -c unhd`, `ip netns list | wc -l`), bring-up notes (shield keys file path, observe→enforce rule, SIGHUP commands), and the next sprint pointer (S-ASCEND-2 per the index)".
- [ ] **Step 403** [V] ~2m: **PHASE 12 EXIT GATE** — `git status --short | grep -v host-agent | wc -l` is 0 after the commit; `scripts/check-timeline-freshness.sh` FRESH; Appendix D has no M-id without a status; the escalations table at the top of this file shows each E's state.
- [ ] **Step 404** [C] ~1m: **COMMIT CHECKPOINT** `[PLAN S-TRUE-WIRE] Steps 397-404: docs, registers, timeline, handoff; sprint close` — then merge `sprint/true-wire` into `develop` with `--no-ff` ONLY after E2 is answered "spec wins" (Step 97); until then the branch stays, and next.md says so.

---

## APPENDIX A: EMERGENCY PROCEDURES

**A.1 Load gate exits 2 (no root).** The gate refuses to pass by skipping. (1) `sudo -n true`; if it prompts, the session has no passwordless sudo: run Steps 13-16, 174-182, 197-204, 208, 218-221, 224, 235, 253, 262-269, 278, 384 in an attended window; (2) everything else proceeds; mark those steps `[BLOCKED: sudo]` not STUCK; (3) never "fix" this by making the gate skip.

**A.2 `upc-regression.sh` fails before the sprint changed anything (Step 16).** (1) `ls ~/tmp/projects/doom-related/DOOM.WAD` (the harness needs the retail WAD; `DOOM_WAD=` overrides); (2) `cd ebpf && cargo build --release -p monad-cpu-ebpf --features ascend-linux` (the harness rebuilds, but a stale `target/` from another feature set confuses it: `cargo clean -p monad-cpu-ebpf` once); (3) `uname -r` vs the kernel the baselines were measured on (6.17.0-41): a different kernel gives different insn counts: record, do not "fix"; (4) if the harness itself is broken, that is a S-CLEAN-SLATE regression: STOP the sprint, fix it there.

**A.3 Verifier rejects Shield after Step 194 (or insns grow > 25 %).** (1) `sudo bpftool prog load <obj> /sys/fs/bpf/probe type xdp -d 2>&1 | grep -B8 -E 'invalid|unbounded|exceeds|R[0-9]+ ' | tail -40; sudo rm -f /sys/fs/bpf/probe`; (2) the three usual causes, in order: a raw pointer into packet data passed into the callback (pass `u32` offsets + `data_end` as values, re-derive the pointer inside with a bounds check), `CheckCtx` + callback frame over 512 bytes (shrink: `u8` verdict bits, no `[u8; N]` buffers), the loop count not a verifier-visible constant (`bpf_loop(3, …)` literal); (3) if two attempts fail: `#[cfg(feature = "checks")]` the loop off, ship the writer and the maps (Steps 183-192 stand alone), STUCK the enforcement with the verifier log excerpt in this file, continue at Step 206.

**A.4 `cargo check` passes but `go test ./pkg/protocol/bpfschema` fails after a Rust edit.** The Rust-source parity test (Step 116) is doing its job: a struct or map name changed in `monad-common` or a program and the Go mirror did not. (1) read the failure: it names the struct/field/map; (2) update the Go side or add the name to `legacyMapNames` with a reason; (3) never loosen the parser to pass.

**A.5 A renumbered wire value breaks a live consumer (Phase 2).** (1) Which consumer: the dual-accept path is in `FlowActionLegacy` / the decoder `legacy` flag; a consumer that reads raw bytes without going through them is the bug; `grep -rn 'FlowAction\b\|\.Flags&\|EventType(' --include=*.go --include=*.js pkg cmd services dashboard`; (2) route it through the accessor; (3) if it is a stored byte (WAL, Well), E2's "revert and file errata" path is the honest answer: say so in the escalation, do not migrate silently.

**A.6 Wotan does not reload on SIGHUP (Step 159).** (1) `signal.Notify` must be registered before the blocking `Serve`; (2) the handler must swap the parsed config under the topic service's existing mutex, not rebuild the service; (3) a malformed file must keep the old config: test it by writing `: :` into the temp yaml and sending HUP; the failure counter increments, nothing else changes.

**A.7 The relicense script touches a file it should not (Step 336).** `git diff --name-only | xargs -n1 git log -1 --format='%h %an %s' --` : any file whose last author is not Stevie Bellis / Claude, or whose `Copyright` names someone else, goes to KEEP_MIT; `git checkout -- <file>`; re-run dry-run; the E4 doc is amended before `--apply` runs again.

**A.8 bandit finds a new rule ID (Step 359).** Do not add it to `--skip`. Triage it in `docs/security/bandit-disposition-2026-08-03.md` (new section with date), fix or `# nosec <ID> -- reason` per site; the gate flips to gating only at 0 unskipped findings.

**A.9 Escalation answered differently from the recommendation.** [DECIDE] steps carry "Override ONLY if"; an override from Stevie is a plan amendment: write it under the step as `AMENDED <date>: <answer>`, adjust the dependent steps' text (not their numbers), commit `[PLAN S-TRUE-WIRE] amendment: E<n> answered <x>`.

---

## APPENDIX B: AGENT ASSIGNMENT MATRIX, CRITICAL PATH, DEPENDENCY MAP

| Phase | Agent | Lane / worktree | Parallel with | Depends on | Time |
|---|---|---|---|---|---|
| 0 Preflight | Coordinator | main | — | S-CLEAN-SLATE gates | 1.25 h |
| 1 RFC desk | Agent | rfc-desk | 3 | 0 | 5 h |
| 2 Wire values | Coordinator | main | — | 1 (registries settled) | 4 h |
| 3 bpfschema hygiene | Agent | bpfschema | 1 | 0 | 3.5 h |
| 4 Error registry, Wotan-03 | Agent | errors-wotan | 9 | 1 (G8 rulings), 3 merged | 5 h |
| 5 Shield truth | Coordinator | main (root) | — | 2, 4 Steps 136-138 | 9 h |
| 6 PQC, delete table | Coordinator | main | — | 5, 1 Step 70 | 4 h + E3 |
| 7 Insn ratchet | Coordinator | main (root) | — | 5, 6 Step 236 | 3 h |
| 8 FND-020, Sophia signing | Agent | fnd020-sophia | 10 | 0 (300-304: S-IRON-GATE) | 5 h |
| 9 SPDX, relicense | Agent | spdx | 4 | 0 | 3 h + E4 |
| 10 Supply chain | Agent | supply | 8 | 6 applied (re-run 341-345 otherwise) | 6 h + E5 |
| 11 Security review | Coordinator | main (root) | — | 1-10 | 2.5 h |
| 12 Docs, handoff | Coordinator | main | — | 11 | 2 h |

**Critical path**: 0 → 1 → 2 → 5 → 6 → 7 → 11 → 12 (≈ 30.75 h of the 38-48 h estimate; Phases 3, 4, 8, 9, 10 fill the gaps on parallel lanes). Phase 5 is the long pole and the only phase with iterative verifier work.

**Dependency map (escalations → steps)**: E1 → nothing in code (ADR-108 status only). E2 → the merge of `sprint/true-wire` into develop (Step 404), not the sprint's progress. E3 → Steps 248-256; Phase 10 Steps 341-345 should be re-run after. E4 → Steps 335-339. E5 → Steps 354-358. E6 → nothing in code. E7 → nothing in code. S-IRON-GATE → Steps 300-304. Sudo → every [S] step (Appendix A.1).

**Skip Protocol acknowledgment**: the executing agent confirms at Step 1 that it will (a) skip after 3x `~time` or 2 debug attempts, (b) write the STUCK marker here, (c) commit before skipping, (d) continue with the next independent chain, (e) produce the STUCK REPORT when no un-blocked step remains.

---

## APPENDIX C: QUICK REFERENCE

**Paths**
- Master list: `docs/sweeps/OUTSTANDING-2026-10-27.md` (M-ids). Decision tables: `docs/sweeps/TRUE-WIRE-DECISIONS.md` (E3), `docs/legal/RELICENSE-2026-10-27.md` (E4).
- Specs: `docs/protocol/draft-bellis-unheaded-protocol-foundation-06.md` (flow actions :1239-1249, event types :1317-1325, error codes :1328-1352, TLV :1353-1371), `…sophia-dictionary-03.md`, `…wotan-memory-03.md`, `…mbc-isa-00.md`; IETF twins + Makefile (`validate`, `txt`) in `ietf-submission/`; `~/.local/bin/xml2rfc`.
- RFC desk: `docs/protocol/RFC-EDITOR-REQUESTS-S5.md`, `docs/protocol/G8-REFERENCE-RULINGS.md` (new), `docs/protocol/error-registry.md`, `docs/protocol/WOTAN_TOPICS.md`, `docs/protocol/RFC-ALIGNMENT-FINDINGS.md`.
- Go schema: `pkg/protocol/bpfschema/{core_maps.go,bpfschema.go,parity_test.go,rust_source_test.go}`; errors `pkg/protocol/errors`; `pkg/wotanerr` (new); `pkg/shieldmaps` (new); `pkg/protocol/sophiasync`; `pkg/wotansig`; `pkg/crypto/pqc/ml_dsa.go`; CRC `pkg/protocol/encoding.CRC16CCITT`.
- Rust: `ebpf/monad-common/src/lib.rs` (flags :160-175, `EventType` :183-200, `Monad` :388, `MONAD_SIZE` :69); `ebpf/shield-ebpf/src/main.rs` (maps :74-135, checks :268, TC :458, egress :505-545); `ebpf/cartridge-ebpf/src/main.rs` (`bpf_loop` :276-540); `ebpf/packet-marker/src/main.rs` (:186-222).
- Gates: `scripts/check-ebpf-loads.sh` (+`docs/policy/ebpf-load-known-failing.txt`), `scripts/check-ebpf-insns.sh` (new, +`docs/policy/ebpf-insns-baseline.tsv`, `ebpf-insns-over-budget.txt`), `scripts/bpf-verifier-check.sh`, `scripts/upc-regression.sh`, `scripts/check-spdx.sh` (new), `scripts/spdx-backfill.sh` (new), `scripts/relicense-mit-to-gpl.sh` (new), `scripts/check-trivy-misconfig.sh` (new, +`docs/policy/trivy-misconfig-baseline.txt`), `scripts/check-gosec-ratchet.sh`, `scripts/check-dependency-baseline.sh`, `scripts/check-gates-can-fail.sh`, `scripts/ci-python-lock.sh`, `scripts/sweep-ledger-step1.py` (new), `cmd/ebpf-load-check` (Rust).
- Registers: `compliance/findings/register.yaml` (FND-020, FND-025), regen `go run ./cmd/compliance-evidence findings -adr docs/adr/ADR-098-findings-register-and-baseline-modes.md`; `docs/adr/ADR-004` :104-105 admissions; ADR-095/096 register; `docs/sweeps/TOTAL-SOURCE-SWEEP.tsv`.
- Hosts: shield keys `/etc/unheaded/shield/keys.yaml` (0400, `unheaded-shield`), pins `/sys/fs/bpf/unhd_{hmac_keys,retry_tokens,hop_validators}`; Sophia signing key `/etc/unheaded/sophia/keys/delta-signing.key` (after S-IRON-GATE).

**Numbers (pre-sprint, 2026-10-27)**: ascend 926,542 insns; Doom 761,580; cart_xdp 42,743; cart_tick 146,897 (gate 500,000); goldens 12/12; `.rs` without SPDX 71; MIT SPDX files ≈ 290; ledger PENDING 2,192; bandit 43 (B310 35, B104 2, B615 2, XML 4); open FND 14.

**Ports (pkg/ports)**: Wotan 18000/18001; Shield 19009; Sophia 19005; Anomaly 19018; cartridgectl metrics 26610; scratch Wotan for tests 28000/28001.

**Env flags introduced**: `SHIELD_ENFORCE` (via `SHIELD_CONFIG` map key 3; default observe), `SOPHIA_REQUIRE_DELTA_SIGNATURE` (default false), `SOPHIA_DELTA_SIGNING_KEY` (unset = signing off), `ZHEN_BIND_ADDR` (default 127.0.0.1), `EBPF_INSNS_OUT` (load gate side-output).

**Commit trailers (every commit)**:
```
Co-Authored-By: Claude Fable 5.1 <noreply@anthropic.com>
Claude-Session: https://claude.ai/code/session_01TzL3TuPhKAVVHnD9jzuabE
```
Signing: `git commit-tree -S` probe in Step 6; `--no-gpg-sign` when it fails; `scripts/resign-unpushed.py` later. Never push.

---

## APPENDIX D: COVERAGE (master-list id → steps)

| M-id | Steps | Status at forge |
|---|---|---|
| M-140 flow-action values | 79-86, 96-98 | planned ([DECIDE] spec wins + E2 confirm) |
| M-141 Anamnesis event types | 87-98 | planned ([DECIDE] spec wins + E2 confirm) |
| M-142 RFC Editor items 1-5 → ADR-108 | 29-58 | planned (E1 acceptance) |
| M-143 FlagK0/K1 → CUSTOM/RSVD; one CRC16 | 101-113 | planned |
| M-144 error-code registry symbols; TLV/Ring Path ruling | 131-141 | planned |
| M-145 Wotan-03 error type; GOAWAY/SETTINGS; SIGHUP reload; wotan-04 checks | 142-163, 206-210 | planned |
| M-146 G8 reference package rulings | 68-73, 150-152, 239 | planned (deletes via E3) |
| M-147 Anamnesis-00 stub, Protocol Overview I-D, datatracker | 59-67, 76 | planned (E6 upload) |
| M-148 formal ADR acceptances | 398 | E7 |
| M-149 Sophia ML-DSA-65 delta signing; mandatory rollout; severity ladder | 293-305 | planned (300-304 after S-IRON-GATE) |
| M-160 Rust SPDX + CI | 315-327 | planned |
| M-161 MIT→GPL relicense; LICENSE "Steven" | 328-339 | planned (E4); LICENSE line already fixed (verified 2026-10-27) |
| M-162 yaml.v3 / cilium-ebpf / goldmark register rows; thresholds | 341-342, 353-356 | planned (E5) |
| M-163 ADR-004 admissions | 343, 353-354 | E5 |
| M-164 ADR-090 ledger step 1; Q1-Q3 | 346-351, 353-357 | planned (E5) |
| M-165 bandit groups; trivy IaC ratchet; hadolint pin | 359-368 | planned; hadolint pin already done (Step 367 verifies) |
| M-166 omitempty / G115 / ST10xx | 369-373 | planned |
| M-167 Python hash pins | 374-376 | planned |
| M-168 delete words (waf, lxd, host stacks, bpfmap, maploader, ebpf-exporter) | 238-256 | E3 |
| M-169 ScanCode re-run; THIRD_PARTY OR elections; fair-use notice | 377-378 | planned |
| M-180 Shield HMAC/retry/hop checks via bpf_loop; writer; finding; docs | 169-173, 183-205, 228 | planned |
| M-181 PQC_SIG_STATUS | 231-237 | planned ([DECIDE] delete) |
| M-182 packet-marker trace-ID extraction | 215-223 | planned |
| M-183 debug TC stats; stress cannon CRC | 177-182, 211-214 | planned |
| M-184 ADR-012 insn ratchet | 259-282 | planned |
| M-185 BPF struct single source of truth | 114-124 | planned |
| M-186 G9 control planes | 238-256 | E3 ([DECIDE] default delete, keymgmt/loadbalancer parked) |
| M-187 AF_XDP retire Decision 5 | 239, 248, 255 | E3 ([DECIDE] retire) |
| M-188 FND-020 replication signatures | 283-292 | planned |
| Decisions 33-42 | 80, 88, 58/97, 232, 239/247, 353 | [DECIDE] with defaults; E1-E5 where the owner must act |

Deferred to other sprints, by name: Wotan `display_name` identity binding (M-052) and the Sophia signer's key distribution → S-IRON-GATE; east-side anything → S-ASCEND-2; Doom → S-DOOM-LAST; the Librarian doc-hygiene list (§5 of the master) → S-CLEAN-SLATE's doc phase, except the items this plan's own changes make true (Steps 96, 172, 228, 234, 276, 397).

---

*S-TRUE-WIRE Battle Plan — Forged 2026-10-27*
*13 Phases. 404 Steps. The spec and the code say the same thing, and the gates prove it.*
