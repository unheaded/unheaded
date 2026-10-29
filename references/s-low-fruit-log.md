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
