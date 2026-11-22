# Developer Guide

The working rules for changing Unheaded code. The full guide is `CLAUDE.md` at the repo root; this page is the short form. Session workflow: [[Agent Operating Procedure|Agent-Operating-Procedure]]. Layout: [[Project Structure|Project-Structure]].

## Branches and commits

- Work on **`develop`**; batches are promoted to `staging`, then `main` (ADR-089). `main` is old: check `develop` before calling something "not done".
- Conventional Commits: `type(scope): subject`, a body that says what and why, and a `Co-Authored-By:` trailer when Claude helped. No slogans, sign-off mantras or marketing words in commit messages.
- Commits are GPG-signed. Claude commits; Stevie pushes.
- `make install-hooks` installs the pre-commit hook (gofmt + go vet on staged files).

## Build and test

```bash
make build            # Go binaries          make ebpf          # eBPF workspace (Rust/Aya)
make test             # everything           make test-go / test-rust / test-e2e
make lint fmt         # linters, formatters  make ci-local      # the CI pipeline, locally
make ci-security      # gosec + govulncheck + GPL boundary
make dev              # compose dev stack    make deploy        # build, test, compose up, health check
make ebpf-monad-cpu-ascend   # UPC CPU with the ASCEND-LINUX opcodes (needed for the xv6 kernel)
```

Toolchain pins follow `go.mod` and latest releases everywhere (Docker and CI in sync).

## Every service

- `GET /health`, `GET /ready`, `GET /metrics`, `/api/v1/*`; register with discovery on start; shut down cleanly on context cancel.
- Prometheus metrics with the `unheaded_` prefix (`unheaded_http_requests_total`, `unheaded_http_request_duration_seconds`, …); structured JSON logs with zerolog, `trace_id` on every line.
- Talk to other services through Wotan (gRPC first, HTTP fallback, `pkg/transport`); subscribe to `alerts.critical` ([[Fae Chamber Contracts|Fae-Chamber-Contracts]]).
- Ports come from `pkg/ports` ([[Port Registry|Port-Registry]]).

## Security

- All input is hostile: validate, bound sizes, handle every error, honour context cancellation.
- No code path may read user data; nothing may weaken isolation or skip hardening. Any of these blocks a PR.
- **No credentials in the repo**, ever: read from the environment with a fail-loud guard, never a default. gitleaks + a shrink-only baseline enforce it.
- Containers and units: least capabilities, `NoNewPrivileges`, read-only filesystem, seccomp, a dedicated nologin user.
- TLS 1.3 for external traffic; The Well accepts TLS only.

## CI gates

Workflows in `.github/workflows/` (ci, security, static-analysis, ebpf, gpl-boundary, sbom, timeline-drift-guard, …), every action pinned to a SHA. Gate scripts in `scripts/check-*.sh`, including: gosec rule-ID ratchet, secrets baseline, eBPF programs load through the verifier, compose hardening, systemd units, Prometheus rules, Well migrations and schema lint, actions pinned, and a check that each gate can actually fail.

Gates not described on another page (security-tool gates: [[Security]]; eBPF gates: [[eBPF Programs|eBPF-Programs]]; crypto gates: [[Post-Quantum-Key-Exchange]]). A ratchet's baseline lives in `docs/policy/` and is written only by the gate's own `--update`:

| Script | Fails when |
|---|---|
| `check-actions-pinned.sh` | a GitHub Action is not pinned to a full commit SHA (ADR-095) |
| `check-cargo-locked.sh` | a cargo build, test, clippy or run in a Makefile or workflow lacks `--locked` |
| `check-ci-verdicts.sh` | a workflow verdict reports more than its step checked; shell verdicts run against fixture results |
| `check-clippy.sh` | clippy finds something in any Rust workspace (roots found by `Cargo.lock`; bare-metal workspaces without `--all-targets`) |
| `check-compose-bind-nesting.sh` | a compose bind mount sits inside another's mountpoint (ADR-091) |
| `check-dep-staleness.sh` | a direct Go module's latest release is over 24 months old with no row in `docs/policy/dependency-watch.md`, or over 36 months with no findings-register entry; runs `cmd/depaudit`, needs the module proxy |
| `check-evidence-pack.sh` | `yggdrasil-evidence` accepts a tampered fixture pack or refuses a valid one (ADR-105) |
| `check-go-env.sh` | `GOFLAGS=-mod=readonly`, `GOSUMDB` or `GOTOOLCHAIN` differ from go.mod in the Makefile or `ci.yml` |
| `check-http-auth.sh` | an HTTP listener's package does not use `pkg/auth` and the file is not in `docs/policy/http-auth-baseline.txt` |
| `check-http-timeouts.sh` | an `http.Server` literal lacks `ReadHeaderTimeout` |
| `check-image-digests.sh` | a third-party compose image has no `@sha256:` digest and is not in `docs/policy/image-digest-baseline.txt` (shrink-only) |
| `check-image-drift.sh` | a host check: a running compose container's image differs from its pin, or a container has no service in the files any more (ADR-117) |
| `check-json-responses.sh` | a handler encodes JSON straight to its ResponseWriter instead of `httputil.WriteJSON` |
| `check-log-sources.sh` | a unit, compose service or host has no entry in `config/logging/sources.yaml`, or the file does not validate (the tests in `pkg/logsources`) |
| `check-manifest-yaml.sh` | a tracked YAML manifest does not parse |
| `check-mod-integrity.sh` | `go mod verify` fails, or a build rewrote `go.mod`, `go.sum` or a `Cargo.lock` |
| `check-module-binds.sh` | a module's unit listens where the public side of a host can reach it (ADR-149; rule in `scripts/lib/module_binds.py`, shared with the live guard) |
| `check-netip.sh` | the count of `net.IP` / `net.ParseIP` in non-test Go differs from `docs/policy/netip-baseline.txt` (`--update` only lowers it) |
| `check-no-client-golang.sh` | anything imports `prometheus/client_golang` again (ADR-094) |
| `check-nosec-ratchet.sh` | a file has more `#nosec` / `//gosec:disable` than `docs/security/nosec-baseline.txt` allows |
| `check-python-syntax.sh` | a tracked `.py` file does not parse |
| `check-ruff.sh` | ruff finds something outside the exclusions it lists by rule ID (ADR-073) |
| `check-sbom-trend.sh` | the Go, Rust or Python component count, counted live from lock data, is above `docs/policy/sbom-count-baseline.txt` |
| `check-tmp-log-baseline.sh` | a log path under `/tmp` is added (ADR-092; `docs/policy/tmp-log-baseline.txt`, shrink-only) |
| `check-trivy-misconfig.sh` | trivy's IaC findings per (rule, file) differ from `docs/policy/trivy-misconfig-baseline.tsv` |
| `check-unit-flags.sh` | a unit's `ExecStart=` passes a flag its binary does not declare, or a binary with a relative default path has no `WorkingDirectory=` its `StateDirectory=` owns |
| `check-usage-errors.sh` | a Go binary answers an unknown option other than the ADR-111 way (one line, the `--help` hint, exit 2) |
| `check-version-flag.sh` | a Go binary does not answer `--version` as `NAME (unheaded) VERSION` with a real version |
| `check-well-lint.sh` | `scripts/well/well_lint.py` finds more per (rule, file) than `docs/policy/well-lint-baseline.tsv` |
| `check-well-migrations.sh` | a numbered file in `db/migrations` is neither applied nor skipped by `MANIFEST`, a named file is missing, or one uses a psql meta-command (ADR-101) |

## Developer scripts

| Script | What |
|---|---|
| `scripts/build.sh [DIR]` | Builds the Go binaries with version metadata |
| `scripts/pre-commit.sh` | An older stand-alone pre-commit hook (go vet, go build). `make install-hooks` links `scripts/git-hooks/pre-commit` (gofmt + go vet on staged files) and `post-commit` instead |
| `scripts/clean-artifacts.sh` | Removes ignored and generated files: `git clean -fdX`, `target/`, `bin/`, caches, fuzz corpora |
| `scripts/coverage-trend.sh [coverage.out]` | Appends total Go coverage to a CSV |
| `scripts/dep-diff.sh go\|rust NAME OLD NEW` | Prints what changed between two versions of a dependency, for the reviewer of a bump (`docs/policy/dependency-watch.md`); not a gate |
| `scripts/ci-python-lock.sh [DIR]` | Resolves `requirements.in` into a hash-pinned `requirements.txt` with uv inside a throwaway container (default `.github/ci-python`) |
| `scripts/gen-man.sh [--write]` | Checks, or with `--write` regenerates, `man/` from each Go binary's `--man` (`make man`) |
| `scripts/live-path-inventory.sh --report` | Writes `docs/LIVE-PATHS.md`: every binary by the strongest evidence that something runs it (ADR-093) |
| `scripts/sweep/unreached-evidence.sh LIST` | One TSV row of evidence per Go package no binary imports (ADR-090 input); read-only |
| `scripts/sweep-ledger-step1.py [--apply]` | ADR-090 sweep step 1: marks generated and vendored files `SKIPPED` in the file ledger; dry run by default |
| `scripts/relicense-mit-to-gpl.sh [--apply]` | Rewrites MIT licence headers to GPL-3.0-or-later (decision 12, 2026-10-27); dry run by default, refuses pending tracked changes |
| `scripts/kingdom-health.sh` | `make health`: the ADR-073 lint ratchet, vulnerability scans, tests, branch hygiene and doc drift in one read-only run with a PASS/FAIL summary |
| `scripts/qa-smoke.sh` | Measures the running dev stack against the demo baseline (the staging ladder's bar); exit 1 on a regression |
| `scripts/wait-for-healthy.sh [--timeout S]` | Polls `/health` on every service until all answer or time runs out |
| `scripts/demo-kanban.sh` | Starts `bin/kanban-app` on `KANBAN_PORT` (20001) for a demo |
| `scripts/verdicts-record.sh` | Records hypothesis verdicts after QA testing to `/var/lib/unheaded/verdicts/<date>.json` and `VERDICTS.md` |
| `scripts/drift-detect.sh` | Runs `terragrunt plan -detailed-exitcode` on each deployed OpenTofu layer (`deploy/tofu`): exit 0 clean, 2 drift, 1 error ([[IaC-Backends]]) |
| `scripts/vault-to-runway.py` | Model tiering between an HDD archive and an NVMe cache: purges the least recently used models when the NVMe passes a threshold |

## Testing

Tests first. 80%+ coverage, 100% on core packages; `go test -race`; fuzz parsers (LICH harnesses, [[LICH-Campaigns]]). Claims of training "descent" need a held-out eval set.

## Docs

A change isn't done until the docs agree with it: CLAUDE.md, `docs/`, `references/timeline.md`, this wiki, README, `LICENSES/THIRD_PARTY.md`. Plans and lab notebooks live in the repo.

---

> **Source:** [CLAUDE.md](../CLAUDE.md) · `Makefile` · `.github/workflows/`
