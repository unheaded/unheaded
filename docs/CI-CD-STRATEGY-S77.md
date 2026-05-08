# S77 — Phase 3: CI/CD Strategy

**Sprint:** S77 (Age 2 Acceleration)
**Phase:** 3 — SBOM + CI/CD Fortress
**Status:** Shipped — GHA workflows + Jenkinsfile both hardened

---

## Strategy

Two parallel CI surfaces: **GitHub Actions** (fast feedback on every PR) + **Jenkins** (deeper hardware-dependent validation gated on bare-metal hosts WEST/EAST).

```
Developer push
   │
   ├─ GHA: lint → unit → integration → SBOM → GPL-boundary → coverage
   │     (typically green in 8-12 min)
   │
   └─ Jenkins (post-merge to main): full hardware validation
         eBPF verifier check → bpftool prog load → cross-host flow graph
         (typically 25-45 min on WEST, runs nightly on a cron)
```

## Local CI script — `scripts/ci-local.sh`

Mirrors the GHA matrix on the developer's host before pushing. Runs in this order:

1. `gofmt -l ./...` — format drift
2. `go vet ./...` — vet warnings (gate)
3. `go build ./...` — full build
4. `go test -short -count=1 ./...` — short tests
5. `cargo build --release --workspace` for the Rust workspaces in scope (zhend, doom-runner, monad-mbc, zhenai-forge, ebpf, cmd/ebpf-loader, cmd/trace-collector)
6. `cargo test --release` for the same
7. `bash scripts/verify-gpl-boundary.sh` — GPL-license boundary check
8. `bash scripts/bpf-verifier-check.sh` — BPF verifier instruction-budget guard

## GPL boundary

`scripts/verify-gpl-boundary.sh` walks `THIRD_PARTY.md`, `go.sum`, and the Rust `Cargo.lock` files; flags any GPL-licensed transitive dependency that crosses the kingdom-core boundary (`pkg/`, `services/`, `crates/`). The Kingdom is GPL-3.0; pulling another GPL crate is fine, but a GPL-incompatible (e.g. AGPL or CDDL) pull triggers a fatal exit.

`.github/workflows/gpl-boundary.yml` runs the same script on every PR and blocks the merge if it fails.

## Makefile targets

| Target | Purpose |
|--------|---------|
| `make ci-local` | Full local CI (mirrors GHA). |
| `make ci-security` | Subset: cargo audit, gosec, gpl-boundary, bpf-verifier. |
| `make sbom-verify` | Regenerate SBOM and diff against committed `docs/sbom/`; fails on drift. |
| `make install-hooks` | Symlink `scripts/git-hooks/pre-commit` into `.git/hooks/`. |
| `make test-hooks` | Run `scripts/git-hooks/test-pre-commit.sh` (T1-T6 audit). |

## Jenkinsfile

Hardware-gated stages:

1. **License Check** — runs `verify-gpl-boundary.sh` (mirrors GHA).
2. **eBPF Verifier** — `bash scripts/bpf-verifier-check.sh`; asserts each program stays under 7%/900K instruction budget.
3. **bpftool Smoke** — actually loads each `.bpf.o` against the running kernel and asserts attach success.
4. **Cross-host Flow Graph** — runs the BPF flow tracer on WEST, sends Monad packets to EAST over the WG overlay, asserts the trace surfaces at WEST's dashboard.

## SBOM hygiene

- `docs/sbom/` is the in-tree canonical SBOM directory.
- ScanCode + syft + cyclonedx generate three views (license, package, BOM) committed alongside each release tag.
- `make sbom-verify` ensures the three views agree.

## References

- `Makefile` — see targets above.
- `.github/workflows/gpl-boundary.yml`.
- `Jenkinsfile` — full pipeline.
- `scripts/ci-local.sh`, `scripts/verify-gpl-boundary.sh`, `scripts/bpf-verifier-check.sh`.
- `docs/sbom/`.
- `tests/s77/s77_verification_test.go::TestPhase3_CICD`.
