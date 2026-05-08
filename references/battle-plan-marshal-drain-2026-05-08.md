# Marshal 2026-05-07 Drain Plan

## Context

Marshal session 2026-05-07 landed 24 local commits and parked 10+ items in `references/marshal-parked-2026-05-07.md` plus a 4-wave cargo-audit remediation in `docs/security/cargo-audit-2026-05-07.md`. This plan consolidates Stevie's 16 rapid-fire decisions into the actual queued execution order so a follow-on shift drains the entire scope without re-asking.

**Outcome target:** every parked item from the three documents is either shipped, formally deferred to launch-prep, or scheduled. Push the 31 unsigned commits to `origin/main` first so daytime CI catches anything before more churn lands.

## Approved work queue — execution order

### Wave 0 — Push gate (immediate, ~10 min)

1. `git push origin main` from `~/tmp/unheaded` — 31 unsigned commits; signing not enforced on this branch.
2. Watch GitHub Actions: `gpl-boundary.yml` (carry-forward 2026-05-06 watch); SPDX gate; build matrix.
3. `make install-hooks` post-push so future commits are gofmt + rustfmt + SPDX guarded.
4. **gpg-agent fix:** deferred indefinitely; signing is optional on this branch. No CLAUDE.md doc change required.

### Wave A — trace-collector tonic 0.10 → 0.12 (closes 4 CVEs, ~1-2h, Linux host)

5. Author `docs/adr/ADR-066-tonic-minor-bump-trace-collector.md` per ADR-052 policy. Rationale: closes RUSTSEC-2026-0098/0099/0104 (rustls-webpki) + RUSTSEC-2024-0437 (protobuf); aligns with zhend already on tonic 0.12.
6. Edit `cmd/trace-collector/Cargo.toml`: `tonic = "0.12"` (and dependent `prost`/`rustls` peers as needed).
7. `cd cmd/trace-collector && cargo update && cargo build --release && cargo test`.
8. `cargo audit` to confirm the 4 advisories drop.
9. Single commit: `feat(trace-collector): bump tonic 0.10 → 0.12 (closes ADR-066, 4 CVEs)`.

### Wave B — pqcrypto FIPS 205/203 migration (~1d, Architect+Developer pair)

10. Edit `crates/zhend/Cargo.toml`: `pqcrypto-dilithium` → `pqcrypto-mldsa`, `pqcrypto-kyber` → `pqcrypto-mlkem`.
11. Update import paths and key/signature type names per the `mldsa` / `mlkem` API.
12. Run zhend's 60 PQC tests; expect API shape to be near-equivalent.
13. Cross-check naming alignment with `services/wotan/internal/signing/` (Go-side ML-DSA-65 via cloudflare/circl).
14. Optional: ADR-067 capturing the FIPS standardization alignment if the reviewer wants formal trace.
15. Commit: `feat(zhend): pqcrypto FIPS 205/203 migration (RUSTSEC-2024-0380/0381)`.

### Wave C — unmaintained-but-functional cleanups (~half day)

16. `crates/zhend/Cargo.toml`: `fxhash` → `rustc-hash` (drop-in).
17. `cmd/trace-collector/Cargo.toml`: `rustls-pemfile` → `rustls-pki-types` (drop-in for newer rustls).
18. `crates/zhend/Cargo.toml`: `bincode 1` → `bincode 2` — careful API migration; run all serialization tests.
19. Bump `parking_lot` (transitively pulls `instant` out, closing RUSTSEC-2024-0384).
20. Commit per crate: `chore(deps): bincode 1 → 2`, `chore(deps): fxhash → rustc-hash`, `chore(deps): rustls-pemfile → rustls-pki-types + parking_lot bump`.

### Wave D — rand audit-ignore (~5 min)

21. Add `RUSTSEC-2026-0097` to a workspace-level `cargo audit` ignore list with rationale link to `docs/security/cargo-audit-wave-d-rand-disposition-2026-05-07.md`.
22. Commit: `chore(security): audit-ignore RUSTSEC-2026-0097 (N/A — no custom logger calls rand::rng)`.

### Code-quality patches (parallel-safe with Waves)

23. **vet warning #1** — `cmd/protocol-api/handlers_extended_test.go:48,50,52,54,68,70,72,74`: switch `sync.Once` save/restore to pointer (`*sync.Once`).
24. **vet warning #2** — `cmd/dashboard-backend/internal/server/doom.go:98`: change return type to `*DoomState`.
25. **vet warning #3+#4** — `pkg/ebpf/loader.go:2278,4026`: append `// nolint:govet` to both lines (kernel-mmap addr is intentional non-Go-managed unsafe.Pointer; rationale already in-line per Phase 19).
26. Commit: `fix(vet): pre-existing 4 warnings — pointer fixes + nolint annotations`.

### cmd/waf rescue (~2-4h, Developer + BlackMage pair)

27. `cmd/waf/src/rules/mod.rs:143,153,190` — convert `r"..."` containing `\"` to `r#"..."#`.
28. `cmd/waf/src/rules/mod.rs:530` — fix the unterminated string in test code.
29. Run `cargo check` until clean; expect 11+ cascading errors to surface and be addressed.
30. Add `cmd/waf` to the CI build target so future drift is caught.
31. BlackMage pass: review SQLi + XSS pattern matchers for completeness against current OWASP top 10.
32. Commit: `fix(waf): unblock cmd/waf compile + add to CI`.

### S77 deliverable-gate — author 5 missing docs (~2-4h)

33. `docs/sprints/S77-P1-BUG-FIXES.md` — list of P1 bugs fixed in S77 with commit refs.
34. `docs/sprints/S77-WIREGUARD-DESIGN.md` — fd00:dead:beef::/48 overlay design.
35. `docs/sprints/S77-PERFORMANCE.md` — sub-50ms latency benchmark plan.
36. `docs/sprints/S77-CI-CD-STRATEGY.md` — GHA + Jenkins hardening current state.
37. `docs/sprints/S77-INTERFACE-CONTRACTS.md` — service-to-service contract matrix.
38. Re-run `go test ./tests/s77/...` to confirm gate passes (218/221 → 219/221, plus monad-mbc 3 still deferred).
39. Commit: `docs(s77): author 5 deliverable-gate docs`.

### ebpf/ clippy 119-warning sweep (~2-3h, Round Table sign-off)

40. `bash scripts/bpf-verifier-check.sh > tmp/bpf-baseline-pre-ebpf-clippy.txt` (snapshot per-program instruction counts).
41. `cd ebpf && cargo clippy --fix --workspace --allow-dirty --allow-staged --all-targets`.
42. `cargo build --release` against the workspace using bpf-linker.
43. Re-run `bpf-verifier-check.sh`; diff per-program. Any program crossing the 7%/900K budget gets a focused un-fix or refactor.
44. Round Table sign-off: Architect + Developer + BlackMage (Computermancer if any UPC-related programs are touched).
45. Commit: `chore(ebpf): clippy --fix sweep, verifier-budget-guarded`.

### golangci-lint v2 manual migration (~30-60min)

46. Read [golangci-lint migration guide](https://golangci-lint.run/docs/product/migration-guide).
47. Manually rewrite `.golangci.yml` to v2 schema preserving the S26 Round Table critical-only policy (errcheck, govet, staticcheck, unused, gosec, bodyclose, exportloopref + explicit disable list).
48. Add top-level `version: "2"`. Migrate `output.formats` list → map. Move `run.skip-dirs/skip-files` → `issues.exclude-dirs/exclude-files`.
49. Run `golangci-lint run --config .golangci.yml ./pkg/auth/...` to capture findings volume — should match v1 baseline.
50. Commit: `chore(lint): golangci-lint v1 → v2 schema migration (preserve S26 policy)`.

### aya 0.13.x major-version migration (1-2 day budget, EXECUTE THIS SHIFT — host is WEST)

51. Already-authored `docs/adr/ADR-065-aya-major-version-migration.md` is the recipe.
52. Pre-flight: confirm `bpftool --version ≥ 7.7` on WEST (precondition met per shift-report header).
53. Execute the 6 phases per ADR-065: pre-flight → branch → Cargo.toml mechanical fixes → verifier gate → integration smoke → merge.
54. Verifier gate is the make-or-break: snapshot `bash scripts/bpf-verifier-check.sh > tmp/bpf-baseline-pre-aya-013.txt` before, diff after each program rebuild.
55. Commit cadence: one per phase per ADR-065.

**Linux-gated tasks consolidation (all executable this shift on WEST):**
- Wave A trace-collector tonic bump (build + cargo audit)
- ebpf/ clippy 119-warning sweep (BPF target, verifier-budget-guarded)
- aya 0.13.x major-version migration
- BPF verifier check baseline + post-change diffs throughout

### Carry-over architectural Round Table (~30 min, schedule with Stevie + Architect)

55. Triage in single pass:
    - C4 heimdall-daemon TODOs (lines 72 GungnirSeal verify, 135 Wotan ML-DSA-65 signing, 147 BPF ringbuf reader, 148 Gjallarhorn XDP listener)
    - D4 `crates/zhend/src/jing/pilgrimage.rs` 3 roadmap notes
    - D5 `crates/zhend/src/pu/codec.rs` `encode_for_gossip` wire-format versioning
    - D6 `crates/doom-runner/src/main.rs:624` `ring::status` struct shape
56. Output: 4-6 ADR drafts or definitive defer decisions.

### MoatGhost tooling install + SBOM regen

57. Install `scancode-toolkit + syft + cyclonedx` on WEST + EAST.
58. Full `scancode-toolkit` regen against current HEAD; emit fresh SBOM delta to `docs/sbom/2026-05-08-sbom-delta.md`.
59. Commit: `chore(sbom): scancode-toolkit + syft + cyclonedx install + full regen against current HEAD`.

## Critical files — modification map

- `docs/adr/ADR-066-tonic-minor-bump-trace-collector.md` (new — Wave A)
- `cmd/trace-collector/Cargo.{toml,lock}` (Wave A)
- `crates/zhend/Cargo.{toml,lock}` (Wave B + Wave C)
- `cmd/trace-collector/Cargo.toml` (Wave C — rustls-pemfile)
- `cmd/protocol-api/handlers_extended_test.go` (vet #1)
- `cmd/dashboard-backend/internal/server/doom.go` (vet #2)
- `pkg/ebpf/loader.go` (vet #3+#4 — nolint annotations)
- `cmd/waf/src/rules/mod.rs` (cmd/waf rescue)
- `docs/sprints/S77-*.md` × 5 (S77 gate)
- `ebpf/**/*.rs` (clippy sweep, verifier-guarded)
- `.golangci.yml` (v2 schema rewrite)
- `docs/sbom/2026-05-08-sbom-delta.md` (new — MoatGhost regen)

## Verification — end-to-end after each wave

```bash
cd ~/tmp/unheaded

# After Wave 0 push
git status                           # clean
gh run watch                         # GHA all green

# After Waves A-D
for ws in crates/zhend cmd/trace-collector ebpf; do
  (cd "$ws" && cargo audit) | grep -E "^(error|warning)" | head -20
done
# expect 4 CVEs gone (Wave A), 2 unmaintained gone (Wave B), 3-4 unmaintained gone (Wave C), rand RUSTSEC-2026-0097 in audit ignore (Wave D)

# After vet patches
go vet ./... 2>&1 | grep -v "warning:" | head -5  # zero new warnings; 4 pre-existing closed

# After cmd/waf rescue
(cd cmd/waf && cargo check) && (cd cmd/waf && cargo build --release)

# After S77
go test ./tests/s77/... -v           # gate passes

# After ebpf clippy
bash scripts/bpf-verifier-check.sh > tmp/bpf-post-ebpf-clippy.txt
diff tmp/bpf-baseline-pre-ebpf-clippy.txt tmp/bpf-post-ebpf-clippy.txt | head -50
# expect zero programs crossed the 7%/900K budget

# After golangci-lint v2
golangci-lint run --config .golangci.yml ./pkg/auth/... | wc -l
# expect baseline-equivalent finding volume

# After aya 0.13.x
cd ~/tmp/unheaded && cargo build --release --workspace
bash scripts/bpf-verifier-check.sh   # all programs verifier-clean

# After SBOM regen
ls -la docs/sbom/2026-05-08-sbom-delta.md
```

## Out-of-scope / formally deferred

- **ADR-058 GCP cost alarm activation** — deferred until launch-prep. Five gaps remain in the Marshal Review section but no work scheduled.
- **monad-mbc 3 screen-mmap test failures** — deferred. Failing as-is; carried to next sprint.
- **gpg-agent pinentry timeout** — deferred indefinitely; signing optional on this branch.
- **Captain Track-call (S3 citation)** — outside the 3-document scope; tracked separately.

## Host context

We are on WEST (Linux 6.17.0-22-generic, Go 1.24, Rust nightly cargo 1.95, bpftool present) — confirmed by Stevie. All Linux-gated items above (Wave A, ebpf/ clippy sweep, aya 0.13.x migration, every `bpf-verifier-check.sh` invocation) execute in this same shift; nothing is being parked for a future Linux session.

## Execution mode

Per `feedback_unattended_churn_with_queued_work.md`: this is a queued work plan; once approved, the executing shift churns through Wave 0 → ... → SBOM regen without per-wave check-ins. Stop only on S4 HALT or explicit Stevie interrupt. Use `--no-gpg-sign` per `feedback_unsigned_commits_when_afk.md` if gpg-agent times out.

When execution begins, copy this plan to `~/tmp/unheaded/references/battle-plan-marshal-drain-2026-05-08.md` per `feedback_persist_plans_to_disk.md` (plans live in-repo).
