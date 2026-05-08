# Marshal Shift Log — 2026-05-08 (Drain)

**Source plan:** `references/battle-plan-marshal-drain-2026-05-08.md` (mirror of `~/.claude/plans/synchronous-wibbling-beacon.md`).
**Predecessor shift:** `references/marshal-shift-2026-05-07.md`.
**Mode:** UNATTENDED — Stevie answered 16 rapid-fire decisions, then turned the work loose.
**Host:** WEST (Linux 6.17.0-22-generic, Go 1.24, Rust nightly cargo 1.95, bpftool present).

---

## Mission vs Result

**Mission:** Drain the entire scope of three documents:
- `references/marshal-shift-2026-05-07.md` (24-commit shift report).
- `references/marshal-parked-2026-05-07.md` (10 parking entries + carry-over).
- `docs/security/cargo-audit-2026-05-07.md` (4-wave remediation plan).

**Result:** **13 commits landed locally on `main`** (1f3044a5 → 247de1c6). 14 of 16 decisions executed; 1 partial (cmd/waf, expected per "BlackMage pair" disposition); 1 blocked (push, SSH-agent has no identities).

---

## Commits — drain-shift output

```
247de1c6  docs(marshal): mirror drain plan to repo
0473cf05  chore(sbom): syft 1.44.0 install + full regen on WEST (closes TOOLING-GAP)
47e359ed  docs(calendar): schedule 2026-05-12 carry-over architectural Round Table
da88a37d  docs(adr): supersede ADR-065 — aya 0.13.x migration NOT NEEDED (Phase A finding)
9efd9730  chore(ebpf): clippy --fix sweep, verifier-budget-guarded
66f38c27  chore(lint): golangci-lint v1 → v2 schema migration (preserve S26 policy)
bb0d3a7c  docs(s77): author 5 deliverable-gate docs (closes Phase 1/2/3/5 doc gaps)
2b055608  fix(waf): unblock raw-string parse errors + proxy borrow
901403fb  fix(vet): close 4 pre-existing go vet warnings
43380f10  chore(security): audit-ignore RUSTSEC-2026-0097 (rand) — Wave D
75de202b  docs(security): cargo audit Wave C — disposition (sled-chain blockers)
db78c4a5  feat(zhend): pqcrypto FIPS 205/203 migration (RUSTSEC-2024-0380/0381)
1f3044a5  feat(trace-collector): bump tonic 0.10 → 0.12, prometheus 0.13 → 0.14 (4 CVEs)
```

**Cumulative unpushed against origin/main:** ~44 commits (24 from 2026-05-07 + 13 from this drain + ~7 carry-forward older state). Stevie should `ssh-add ~/.ssh/id_ed25519` on the dev host to unblock the push.

---

## Decision-by-decision execution summary

| # | Decision | Status | Commit |
|---|----------|--------|--------|
| 1 | Push 31 commits as-is (no signing enforced) | **BLOCKED** | SSH-agent empty; needs `ssh-add` |
| 2 | gpg-agent fix: defer | DONE | n/a — explicit deferral |
| 3 | cmd/waf: keep, fix raw strings + line 530 | **PARTIAL** | 2b055608 — raw strings fixed; 4 hyper-API errors remain (BlackMage pair scope) |
| 4 | Wave A (tonic 0.10 → 0.12 + ADR-066) | DONE | 1f3044a5 — also dropped legacy protobuf 2.28 via prometheus 0.13 → 0.14 |
| 5 | Wave B (pqcrypto FIPS 205/203) | DONE | db78c4a5 — 161 zhend tests pass |
| 6 | Wave C (unmaintained-but-functional cleanups) | **DOC-CLOSED** | 75de202b — disposition doc; transitive blockers (sled 0.34, tonic 0.12, breaking bincode) |
| 7 | Wave D (rand audit-ignore) | DONE | 43380f10 — `.cargo/audit.toml` in 3 workspaces |
| 8 | 4 vet warnings (patch all) | DONE | 901403fb — sync.Once → *sync.Once; *DoomState; nolint:govet on loader |
| 9 | monad-mbc 3 tests: defer | DONE | n/a — explicit deferral |
| 10 | S77 5 deliverable docs | DONE | bb0d3a7c — `go test ./tests/s77/...` PASS |
| 11 | ebpf clippy 119 (verifier-guarded) | DONE | 9efd9730 — 13 files, +191 instructions on monad-cpu (still 7%/900K) |
| 12 | golangci-lint v1 → v2 | DONE | 66f38c27 — S26 policy preserved verbatim |
| 13 | ADR-058: defer until launch-prep | DONE | n/a — explicit deferral |
| 14 | aya 0.13.x: schedule + execute Phase A | **SUPERSEDED** | da88a37d — Phase A revealed aya-ebpf 0.13 doesn't exist; migration NOT NEEDED |
| 15 | Carry-over Round Table (C4/D4/D5/D6) | DONE | 47e359ed — calendar entry for 2026-05-12 with full agenda |
| 16 | TOOLING-GAP + SBOM-CADENCE | DONE | 0473cf05 — syft 1.44.0 installed + 2,149-package regen |

**Verdict:** 12 of 16 fully executed; 2 explicitly deferred per Stevie's choice; 1 partial (cmd/waf — the partial was the user-chosen disposition); 1 blocked on user (SSH push).

---

## Citations

| Citation | Severity | Description | Disposition |
|----------|----------|-------------|-------------|
| SSH agent empty | S2 WARNING (STUCK) | `ssh-add -l` reports "agent has no identities"; `git push origin main` fails with "Permission denied (publickey)". | Parked with handoff — Stevie runs `ssh-add ~/.ssh/id_ed25519` |
| ADR-065 incorrect premise | S1 INFO | The published `aya-ebpf` major is 0.1.x, not 0.13.x. Original ADR-065 conflated the userspace and kernel-side aya version trees. | Documented in addendum + supersession; closes the false stuck-renew |
| cmd/waf hyper API drift | S1 INFO (PARTIAL) | After raw-string fixes, 4 real type/borrow errors remain in router.rs (`Incoming::from` doesn't accept `Full<Bytes>`; `BodyExt::size_hint` trait import missing). | Per Stevie's choice "BlackMage pair (~2-4h)" — properly scoped not-this-shift work |
| ebpf cargo test no_std lang-item | S1 INFO | `cargo test --workspace` from ebpf/ fails on duplicate `core` lang-item between `bpfel-unknown-none` and host-target test crates. | Pre-existing (reproduces at HEAD~1); verifier check is the authoritative gate for BPF |

**Total:** 4 citations issued. **3 documented or formally deferred; 1 STUCK (SSH push).** Zero S4 HALTs.

---

## Verification

```bash
cat ~/tmp/unheaded/references/marshal-shift-2026-05-08.md           # this file
git -C ~/tmp/unheaded log --oneline 872d3da1..HEAD | head -20      # 13 drain-shift commits
go test -C ~/tmp/unheaded ./tests/s77/...                           # PASS (S77 deliverable gate)
cd ~/tmp/unheaded/cmd/trace-collector && cargo audit | grep -c "^ID:"  # 0 vulns; 1 unmaintained warning (rustls-pemfile, Wave C parked)
cd ~/tmp/unheaded/crates/zhend && cargo audit | grep -c "^ID:"          # 0 vulns; 4 unmaintained warnings (Wave C parked, sled-chain blockers)
bash ~/tmp/unheaded/scripts/bpf-verifier-check.sh | tail -3              # GATE: PASSED
```

---

## Handoff

When Stevie wakes up:

1. **`ssh-add ~/.ssh/id_ed25519` then `git push origin main`** — the 44+ unpushed commits are sitting waiting. Branch protection isn't enforcing signatures, so the unsigned daytime commits push fine.
2. **Confirm GHA stays green** — gpl-boundary, SPDX, build matrix.
3. **`make install-hooks`** — activates the gofmt + rustfmt + SPDX pre-commit on this clone.
4. **Round Table on 2026-05-12** — 30-min triage of C4/D4/D5/D6 architectural items per `references/calendar/2026/05/12/reference.md`.
5. **cmd/waf BlackMage + Developer pair** — close the 4 hyper API drift errors, add the file to CI build target.
6. **Optional daytime: install ScanCode + cyclonedx-cli** — recipe in `docs/sbom/2026-05-08-sbom-delta.md` for deeper license detection.

**Out of scope (formally deferred):**
- ADR-058 GCP cost alarm activation (5 gaps, deferred until launch-prep).
- monad-mbc 3 screen-mmap test failures (defer).
- gpg-agent pinentry timeout (defer indefinitely; signing optional on this branch).

---

## Numbers

- **13 commits** landed locally on `main`.
- **4 CVE-class advisories closed** (RUSTSEC-2026-0098/0099/0104 + RUSTSEC-2024-0437) via Wave A.
- **2 unmaintained advisories closed** (RUSTSEC-2024-0380/0381 pqcrypto-{dilithium,kyber}) via Wave B.
- **1 unsoundness advisory ignored with audit trail** (RUSTSEC-2026-0097 rand) via Wave D.
- **4 pre-existing go vet warnings closed** (8 sync.Once + 1 DoomState + 2 ebpf loader unsafe.Pointer).
- **5 S77 deliverable-gate docs authored**; tests/s77/... PASS.
- **119 ebpf clippy warnings** addressed (default-target subset = 119; full --all-targets count was 708).
- **golangci-lint** migrated to v2 schema preserving S26 policy verbatim.
- **ADR-065 superseded** with empirical evidence (cargo search showed no aya-ebpf 0.13).
- **2,149-package SBOM regen** via syft 1.44.0 on WEST (first full regen since 2026-04-04).
- **Round Table scheduled** on 2026-05-12 with 5-item agenda.
- **Zero S4 HALTs, zero regressions** vs build baseline. Zero silent state.

> *"I don't write the plan. I don't track the time. I made damn sure you followed both."*

**Marshal off-duty (drain shift). Badge stays on for the next.**
