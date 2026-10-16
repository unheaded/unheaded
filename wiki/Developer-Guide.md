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

Toolchain pins follow `go.mod` and latest releases everywhere (Docker, CI, Jenkins in sync).

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

## Testing

Tests first. 80%+ coverage, 100% on core packages; `go test -race`; fuzz parsers (LICH harnesses, [[LICH-Campaigns]]). Claims of training "descent" need a held-out eval set.

## Docs

A change isn't done until the docs agree with it: CLAUDE.md, `docs/`, `references/timeline.md`, this wiki, README, `LICENSES/THIRD_PARTY.md`. Plans and lab notebooks live in the repo.

---

> **Source:** [CLAUDE.md](../CLAUDE.md) · `Makefile` · `.github/workflows/`
