# S-LOW-FRUIT log

Plan: `docs/battle-plans/S-LOW-FRUIT-battle-plan.md` (approved `dcf331fd`). Branch `sprint/low-fruit` from develop `dcf331fd`.

## Phase 0 — preflight (2026-10-29, west)

- Step 0a: `sprint/low-fruit` at `dcf331fd` (G).
- Steps 1-2: `go build ./... && go vet ./...` exit 0 (13.7 s), go 1.27.3.
- Steps 4-5: `go test -race ./pkg/cmdline/ ./cmd/...` — no non-ok lines, rc=0.
- Step 6: `git status --short | wc -l` = 1 before this log. **PHASE 0 GATE: PASS.**

## Phase 1 — one version for every binary (`033b8112`)

- `GO_LDFLAGS` carries `-X unheaded/pkg/cmdline.Version=$(VERSION)`; `make build-all` builds all 59 mains into `bin/` and runs the gate. `./bin/akira --version` → `akira (unheaded) dcf331fd-dirty`.
- `scripts/check-version-flag.sh` found 3 misses (steel-echo printed `dev` over the link-time version; wotan-ctl's own format; unheaded-cli rejected `--version`), all fixed. Spec.Version `dev` now means unset (test added).
- Meta-gate: `check-version-flag` registered; `--only check-version-flag` PASS (bites).

## Phase 2 — ReadHeaderTimeout (Slowloris)

- 22 offenders (as measured). `ReadHeaderTimeout: 10 * time.Second` added to each; ebpf-exporter gets 5 s (its ReadTimeout is 5 s). All files already imported `time`.
- `go build ./...`, `go vet`, `go test -race` on services + touched cmds: 0 FAIL.
- `scripts/check-http-timeouts.sh`: OK, 47 literals. Registered (`provoke_http_timeouts`: field deleted from timeguru).

## Phase 3 — systemd ceilings (Steps 24-30, 36; 31-35 [BLOCKED, E3])

- Step 25: 36 units without `MemoryMax` (as planned); 19 of them run a Go binary (upc-bootctl is Rust, excluded).
- Step 27: those 19 get `MemoryMax`/`MemoryHigh` (90 %)/`TasksMax=512`/`GOMEMLIMIT=MemoryHigh`: 512M default, dashboard-backend + kanban 1G, wotan 2G. The gate also caught chaos-controller, kev-poller and sword (MemoryMax 128M, no GOMEMLIMIT): MemoryHigh 115M + GOMEMLIMIT added.
- Step 28: the five bare units get a partial block (NoNewPrivileges, ProtectSystem=full, ProtectKernelModules/Logs, ProtectClock/Hostname, LockPersonality, RestrictRealtime/SUIDSGID, SystemCallArchitectures=native), each with a comment on why it stops there: kingdom-startup/shutdown run docker compose as root (shutdown's `sudo rm` runs as root, so NNP doesn't bite); the Doom units run from /home, enter netns and load BPF as root.
- Step 30: `systemd-analyze verify` on all 39: no errors beyond binaries not installed on west.
- Step 36: `scripts/check-unit-ceilings.sh` OK (22 Go units). Registered (`provoke_unit_ceilings`).
- **Steps 31-35 BLOCKED (E3, attended):** installed on west and now different from the repo: huginn, unheaded-baseline, unheaded-chaos-controller, unheaded-kev-poller, unheaded-trace-collector (and unchanged ones). Install + restart + `systemd-analyze security` table wait for Stevie.

## Phase 5 — native man pages

- `pkg/cmdline/man.go`: `Man(w, fs, spec)` → roff (.TH with SOURCE_DATE_EPOCH, NAME, SYNOPSIS, DESCRIPTION, COMMANDS, OPTIONS, EXIT STATUS or NOTES, EXAMPLES, SEE ALSO). `--help` and the page share `flagRow`. Hidden `--man` in `ParseSet` (not in `--help`, not counted for `[OPTION]...`). Tests: sections, bare spec, details → NOTES, hostile text (leading `.`/`'`, `\`, `-`), `--man` via ParseSet. `go test -race ./pkg/cmdline` ok; `groff -man -ww` no warnings on akira's page.
- 39 of 59 binaries answer `--man` → `man/*.1` (`make man`). 20 don't (subcommand dispatchers, or mains not on ParseSet): `docs/policy/man-missing-baseline.txt`, shrink-only. `scripts/gen-man.sh` checks by default (ignores the .TH line, which moves with each commit), `--write` regenerates; registered (`provoke_gen_man`).
- `.deb`s install `man/NAME.1` gzipped (`gzip -9n`) via `install_man` in `build-all-debs.sh`.
- **Finding (ADR-111 gap, not fixed here):** kev-poller, steel-echo, upc-cartridgectl, zhen-claude and zhen-eval answer an unknown option with Go's raw `flag provided but not defined` + full usage dump instead of one line + hint. `pkg/cmdline/policy_test.go` doesn't catch them. Candidate for the man-baseline burn-down.

## Phase 6 — config errors name the file

- Step 58: the plan's pipeline finds 3; a wider search (decode after `os.ReadFile`, error returned bare) finds 5. Wrapped `%s: %w` with the path: akira `loadConfig`, unheaded-cli `~/.unheaded/config.yaml`, huginn `loadConfig`, timeguru `loadTimeline`. crypto-inventory's `parseCargoDeps` takes bytes (its caller has the path) — left.
- Step 61: `akira --config /tmp/bad.yaml` → `error="/tmp/bad.yaml: yaml: line 1: did not find expected node content"`; huginn → `huginn: config load error: /tmp/bad.yaml: yaml: line 1: …`. (Running the huginn binary for this tried to bind :9110 and failed because the installed huginn holds it; nothing changed.)
- Step 62: `cmd/huginn/config_test.go` (two broken files → error names path + line; missing file stays optional). Tests green.

## Phase 4 — pprof + profiles (Steps 38-44, 48; 45-47 Go leg done, BPF leg pending)

- Deviation from Step 40 (recorded): pprof runs on **its own loopback listener** (`httputil.StartPprof`, refuses any non-loopback address with exit 2), not on a "metrics mux", because akira and wotan serve /metrics on their service port. MoatGhost's conditions hold: off by default, loopback only, raw profiles never committed.
- `--pprof ADDR` on akira, huginn, heimdall-daemon, upc-tty-bridge, doom-bridge; `PPROF_ADDR` on wotan and dashboard-backend (dashboard's hand-rolled listener now uses StartPprof, so it gained the loopback check). Tests: index served, loopback table, wildcard refused, second bind fails.
- `scripts/flamegraph.sh ADDR BINARY [SECONDS] [--top]`, `scripts/bpf-profile.sh [PROG] [SECONDS]`, `make build-profile` (symbols), `.gitignore` `var/pprof/` + `*.pb.gz`, `wiki/Profiling.md` (sidebar + Observability-Backends link).
- Step 45 Go leg: `./bin/akira --pprof 127.0.0.1:29660 --port 29661` → index served; `flamegraph.sh … 5 --top` wrote `var/pprof/akira-*.pb.gz` (ignored by git) and pprof read it (idle akira: 0 samples). `akira --pprof 0.0.0.0:6060` → refused, rc=2. **BPF leg pending** (needs a UPC boot; next attended or UPC session).
- gen-man caught the new flag in 5 pages; regenerated.
