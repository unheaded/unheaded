# S-LOW-FRUIT BATTLE PLAN — 10 Phases, 95 Steps (+15 lettered)

**Date**: 2026-10-29
**Sprint**: S-LOW-FRUIT — cheap, high-value, reversible best-practice adds found by the Round Table (`S-LOW-FRUIT-round-table.md`)
**Prerequisite**: ADR-111 patch applied (it is, uncommitted on `develop`); `go build ./... && go test ./...` green on the Mac
**Target**: every Go daemon has header timeouts, a memory ceiling with `GOMEMLIMIT`, a version, native man pages, path-bearing config errors; pprof + CPU profiles on demand; Vector survives a dead sink; one direct module removed natively and every kept one justified in writing; each fix guarded by a shrink-only ratchet
**Estimated Duration**: 5-6 hours, two attended sessions
**Agent Strategy**: Phases 1-7, 9 independent after Phase 0. Agent-safe: 1, 2, 5, 6, 8. Stevie/west only: 0, 3 (31-35), 4 (45-47), 7 (67-68), 9, every [C]. WIP ≤ 3 phases.
**Commit Cadence**: every 4 steps (80/20 → 4). Stevie commits; agents stage and stop at each [C]
**Stuck Protocol**: skip after 3x estimate or 2 failed debug attempts; mark [STUCK], continue
**Scope fence (Marshal)**: no logger consolidation, no `net.IP` rewrite, no wire-format change, no new ADR number
**Standing principle (Captain/Architect/Developer/BlackMage, 2026-10-29)**: native first. Every package or tool outside the tree is attack surface and supply-chain exposure; an outside module or tool is a last resort that enters the repo only with Stevie's explicit approval, recorded before it is pulled in, and the SBOM line count must trend down sprint over sprint — `scripts/check-dependency-baseline.sh` already refuses additions; this sprint adds the burn-down.

## LEGEND
[B] bash · [V] verify (must pass) · [D] debug (only on failure) · [W] write/edit file · [R] read · [S] sudo · [P] parallelizable · [C] commit checkpoint · [STUCK]/[BLOCKED]

All paths relative to `~/tmp/unheaded`.

---

## PHASE 0: PREFLIGHT (Steps 1-6)
**Goal**: ADR-111 patch builds and tests green; baseline numbers recorded.
**Time**: 15m · **Agent**: Solo

- [ ] **Step 1** [B] ~3m: `go build ./... && go vet ./...`
- [ ] **Step 2** [V]: exit 0. If fail → Step 3.
- [ ] **Step 3** [D]: `go build ./... 2>&1 | head -30` — expect only cmdline call-site slips; fix the one file, back to Step 1. Two failures → [STUCK], report.
- [ ] **Step 4** [B] ~5m: `go test -race ./pkg/cmdline/ ./cmd/... 2>&1 | grep -vE '^ok|no test files' | head -40`
- [ ] **Step 5** [V]: empty output (all ok). Known delta: `cmd/unheaded-keygen` `-h` now on stdout (test already updated).
- [ ] **Step 6** [V]: **PHASE 0 EXIT GATE** — Steps 2 and 5 pass. `git status --short | wc -l` noted. Record in `references/s-low-fruit-log.md`: build green, test green, date.

## PHASE 1: ONE VERSION FOR EVERY BINARY (Steps 7-13)
**Goal**: `--version` prints `git describe` for all 59 binaries via one ldflag.
**Time**: 15m · **Agent**: Solo

- [ ] **Step 7** [W] ~2m: `Makefile` line 17 → `GO_LDFLAGS := -s -w -X unheaded/pkg/cmdline.Version=$(VERSION)` (VERSION is defined on line 26 — move the VERSION line above GO_LDFLAGS).
- [ ] **Step 8** [W] ~2m: `Makefile` line 55: drop the per-target `-X main.Version=$(VERSION)` for unheaded-daemon **only if** `cmd/unheaded-daemon/main.go` reads `cmdline.Version`; otherwise keep both (it still prints its own `version` var — ADR-111 passes it as `Spec.Version`). Decision: keep both this sprint.
- [ ] **Step 9** [B] ~2m: `make build 2>&1 | tail -3 && ./bin/akira --version && ./bin/sophia --version`
- [ ] **Step 10** [V]: both print `NAME (unheaded) <git-describe>`, not `dev`. If `dev` → Step 11.
- [ ] **Step 11** [D]: `make -n build | grep -c 'cmdline.Version'` — if 0 the target bypasses GO_LDFLAGS; patch that target line.
- [ ] **Step 12** [W] ~3m: `scripts/check-version-flag.sh`: loops `bin/*`, runs `--version`, fails if any output lacks `(unheaded)` or ends in ` dev`. `chmod +x`. Register in the meta-gate list (`scripts/check-gates-can-fail.sh` convention: it must fail on a `dev` binary — test with `go build -o /tmp/x ./cmd/unheaded && scripts/check-version-flag.sh /tmp/x; echo $?` → 1).
- [ ] **Step 13** [C]: **PHASE 1 EXIT GATE + COMMIT** — Step 10 green, Step 12 fails on purpose and passes on `bin/`. `[PLAN S-LOW-FRUIT] Steps 7-13: one ldflag versions every binary`

## PHASE 2: SLOWLORIS — HEADER TIMEOUTS (Steps 14-23)
**Goal**: all 56 `http.Server` literals set `ReadHeaderTimeout`; grep ratchet keeps it.
**Time**: 40m · **Agent**: Agent [P]

- [ ] **Step 14** [B] ~1m: list offenders
  ```bash
  grep -rn 'http.Server{' --include=*.go cmd services pkg internal | grep -v _test | while IFS=: read f l r; do sed -n "$l,$((l+15))p" "$f" | grep -q ReadHeaderTimeout || echo "$f:$l"; done | tee /tmp/slowloris.txt; wc -l < /tmp/slowloris.txt
  ```
- [ ] **Step 15** [V]: ~22 lines (Round Table count). Each is a real `&http.Server{` literal (not a helper wrapper) — spot-check 3.
- [ ] **Step 16** [W] ~20m: for each file in `/tmp/slowloris.txt` add `ReadHeaderTimeout: 10 * time.Second,` as the first field after `Handler:`; where `ReadTimeout` already exists keep it (ReadHeaderTimeout ≤ ReadTimeout). `time` already imported in every listed file (they set ReadTimeout/WriteTimeout) — verify with `grep -L '"time"'` on the list; add import where missing.
- [ ] **Step 17** [B] ~1m: `gofmt -l cmd services pkg internal; go build ./...`
- [ ] **Step 18** [V]: no gofmt output, build green. If build fails → Step 19.
- [ ] **Step 19** [D]: `go build ./... 2>&1 | head` — usually a missing `time` import or a comma; fix, back to Step 17.
- [ ] **Step 20** [W] ~5m: `scripts/check-http-timeouts.sh`: the Step 14 loop; exit 1 if any offender. Must fail when you comment out one timeout (prove it: `scripts/check-gates-can-fail.sh` style), pass on the tree.
- [ ] **Step 21** [B] ~1m: `scripts/check-http-timeouts.sh; echo rc=$?`
- [ ] **Step 22** [V]: rc=0. Append the script to the meta-gate list / `Makefile` `gates:` target if one exists (`grep -n 'check-ebpf-insns' Makefile .github/workflows/*.yml` shows where ratchets are wired; wire this one the same way).
- [ ] **Step 23** [C]: **PHASE 2 EXIT GATE + COMMIT** — `[PLAN S-LOW-FRUIT] Steps 14-23: ReadHeaderTimeout on every http.Server + ratchet`

## PHASE 3: SYSTEMD — MEMORY CEILINGS, GOMEMLIMIT, THE FIVE BARE UNITS (Steps 24-37)
**Goal**: every Go daemon unit has `MemoryMax`, `TasksMax`, `Environment=GOMEMLIMIT`; the 5 bare units get the standard hardening block; `systemd-analyze security` ≤ 4.0 for Go units.
**Time**: 45m · **Agent**: Coordinator (sudo on west for the analyze step)

- [ ] **Step 24** [B] ~1m: `grep -L '^MemoryMax' systemd/*.service deploy/systemd/*.service | tee /tmp/nomem.txt | wc -l`
- [ ] **Step 25** [V]: 36 (39 − 3).
- [ ] **Step 26** [R] ~3m: `deploy/systemd/unheaded-sword.service` — copy its `MemoryMax`/`TasksMax` form; note `Environment=` lines.
- [ ] **Step 27** [W] ~15m: for each unit in `/tmp/nomem.txt` that runs a Go binary (`ExecStart=` points at `/usr/local/bin/<go-binary>`): add under `[Service]`
  ```
  MemoryMax=<ceiling>
  MemoryHigh=<ceiling × 0.9>
  TasksMax=512
  Environment=GOMEMLIMIT=<MemoryHigh in MiB>MiB
  ```
  Ceilings: daemons 512M (akira, huginn, heimdall, kev-poller…), dashboard-backend/kanban/wiki 1G, wotan 2G, UPC tools 1G. `GOMEMLIMIT` = MemoryHigh so the GC works harder *before* the cgroup OOM-kills (Go ≥1.19). Skip units whose ExecStart is a shell script or non-Go binary.
- [ ] **Step 28** [W] ~8m: the 5 bare units (`systemd/kingdom-startup.service`, `systemd/kingdom-shutdown.service`, `deploy/systemd/unheaded-doom-{injector,ring,runner}.service`): add the house hardening block copied from `deploy/systemd/huginn.service` (`NoNewPrivileges=yes`, `ProtectSystem=strict`, `ProtectHome=yes`, `PrivateTmp=yes`, `ProtectKernelTunables=yes`, `LockPersonality=yes`, `RestrictNamespaces=yes`, `CapabilityBoundingSet=`, `RestrictAddressFamilies=`, `SystemCallFilter=@system-service`). Doom units need `CAP_NET_RAW CAP_BPF CAP_NET_ADMIN` and `AF_PACKET AF_NETLINK` — copy from `unheaded-upc-bootctl@.service`. kingdom-startup/shutdown run systemctl: they need `SystemCallFilter=@system-service @privileged` or stay less strict; set what keeps them working, document the exception in the unit.
- [ ] **Step 29** [B] ~1m: `for u in systemd/*.service deploy/systemd/*.service; do systemd-analyze verify "$u" 2>&1 | grep -v '^$'; done | head`
- [ ] **Step 30** [V]: no errors (warnings about missing binaries on the Mac are fine; on west they must be clean).
- [ ] **Step 31** [S][B] ~5m (on west): `sudo cp deploy/systemd/*.service /etc/systemd/system/ && sudo systemctl daemon-reload && for u in huginn akira heimdall; do sudo systemctl restart unheaded-$u 2>/dev/null || sudo systemctl restart $u; done; systemctl --failed | head`
- [ ] **Step 32** [V]: no failed units. If one fails → Step 33.
- [ ] **Step 33** [D]: `journalctl -u <unit> -n 30 --no-pager` — a `SystemCallFilter`/`CapabilityBoundingSet` refusal shows as `code=killed, signal=SYS` or `Operation not permitted`; widen that one line, reload, retry. Two failures → revert that unit's block, [STUCK].
- [ ] **Step 34** [S][B] ~2m: `for u in $(systemctl list-units --type=service --no-legend 'unheaded-*' 'huginn*' | awk '{print $1}'); do printf '%-40s %s\n' $u "$(systemd-analyze security $u 2>/dev/null | tail -1)"; done`
- [ ] **Step 35** [V]: every Go unit reports exposure ≤ 4.0 (OK). Record the table in `references/s-low-fruit-log.md`.
- [ ] **Step 36** [W] ~5m: `scripts/check-unit-ceilings.sh`: fails if any `deploy/systemd/*.service` whose ExecStart is a Go binary lacks `MemoryMax` or `GOMEMLIMIT`. Prove it fails (delete one line in a temp copy), passes on tree.
- [ ] **Step 37** [C]: **PHASE 3 EXIT GATE + COMMIT** — Steps 30, 32, 35 green. `[PLAN S-LOW-FRUIT] Steps 24-37: memory ceilings + GOMEMLIMIT on every Go unit; five bare units hardened`

## PHASE 4: PPROF + FLAME GRAPHS (Steps 38-48)
**Goal**: `--pprof` enables `net/http/pprof` on the loopback metrics listener; `scripts/flamegraph.sh` captures a CPU profile and opens pprof's flame view; `scripts/bpf-profile.sh` profiles `monad-cpu-ebpf` with `bpftool`. No tooling outside the Go toolchain and the kernel.
**Time**: 40m · **Agent**: Coordinator

- [ ] **Step 38** [R] ~2m: `cmd/dashboard-backend/main.go` — how pprof is mounted today (the one existing site) and `pkg/httputil/middleware.go` for the metrics mux.
- [ ] **Step 39** [W] ~8m: `pkg/httputil/pprof.go`: `func RegisterPprof(mux *http.ServeMux)` mounting `/debug/pprof/` (index, cmdline, profile, symbol, trace) — only ever called on the **metrics/loopback** mux, never on the service mux. Test: `TestRegisterPprof_ServesIndex` (httptest, GET `/debug/pprof/` 200).
- [ ] **Step 40** [W] ~8m: in akira, huginn, heimdall-daemon, wotan (`services/wotan`), upc-tty-bridge, doom-bridge: add `pprofOn := flag.Bool("pprof", false, "serve net/http/pprof on the metrics listener (loopback only)")` and `if *pprofOn { httputil.RegisterPprof(metricsMux) }`. Refuse with `cmdline.Fail` if the metrics addr is not loopback and `--pprof` is set.
- [ ] **Step 41** [B] ~2m: `go build ./... && go test ./pkg/httputil/ -run Pprof -race`
- [ ] **Step 42** [V]: green. `./bin/akira --pprof --once & sleep 1; curl -s http://127.0.0.1:19100/debug/pprof/ | head -3` shows the index (adjust port to akira's metrics addr).
- [ ] **Step 43** [W] ~6m: `scripts/flamegraph.sh <metrics-addr> [seconds]`: `curl -s "http://$ADDR/debug/pprof/profile?seconds=$S" -o var/pprof/<name>-<date>.pb.gz` then `go tool pprof -http=127.0.0.1:0 bin/<name> var/pprof/<name>-<date>.pb.gz` — the flame graph is a built-in pprof view (View → Flame Graph); nothing outside the Go toolchain. Add `var/pprof/` and `*.pb.gz` to `.gitignore`: raw profiles can hold in-memory secrets (MoatGhost). Profiles need symbols: add `GO_LDFLAGS_PROFILE := -X …Version` (no `-s -w`) and `make build-profile`.
- [ ] **Step 44** [W] ~5m: `scripts/bpf-profile.sh [duration]` (kernel `bpftool`, already a dependency): `sudo bpftool prog profile id $(bpftool prog show | awk '/monad_cpu/ {print $1}' | tr -d :) duration $D cycles instructions l1d_loads llc_misses` — the per-program cost of a Doom tick (the per-program cost of a Doom tick).
- [ ] **Step 45** [S][B] ~5m (west, UPC booted or Doom ring up): `scripts/flamegraph.sh 127.0.0.1:<doom-bridge metrics port> 20 && scripts/bpf-profile.sh 10 | tail -8`
- [ ] **Step 46** [V]: a `.pb.gz` exists and `go tool pprof -top` lists Go symbols (not hex); bpftool prints cycles/instruction per run. If symbols are hex → Step 47.
- [ ] **Step 47** [D]: rebuild with `make build-profile` (the `-s -w` build strips symbols); re-run Step 45.
- [ ] **Step 48** [C]: **PHASE 4 EXIT GATE + COMMIT** — Steps 42, 46 green; `wiki/Observability.md` gains a "Profiling" section (pprof flag, flamegraph.sh, bpf-profile.sh, loopback-only rule). `[PLAN S-LOW-FRUIT] Steps 38-48: pprof behind --pprof on loopback; CPU profiles for Go and BPF`

## PHASE 5: MAN PAGES FROM --HELP, NATIVELY (Steps 49-56)
**Goal**: `man/<binary>.1` for all 59 Go binaries, generated by `pkg/cmdline` itself (Go stdlib only, no outside tool), examples included, reproducible, installed by the Debian packaging.
**Time**: 35m · **Agent**: Agent [P] (TDD; Developer owns)

- [ ] **Step 49** [W] ~8m: test first — `pkg/cmdline/man_test.go`: `Man(w, fs, spec)` emits roff with `.TH NAME 1 "YYYY-MM-DD" "unheaded VERSION" "Unheaded Kingdom"`, `.SH NAME` (name \- purpose), `.SH SYNOPSIS`, `.SH DESCRIPTION`, `.SH OPTIONS` (one `.TP` per flag, defaults shown as in `--help`), `.SH COMMANDS` when present, `.SH EXAMPLES` (one `.EX`/`.EE` block), `.SH EXIT STATUS` from `Details` when it starts with "Exit codes", `.SH SEE ALSO` listing `unheaded(1)`. Date from `SOURCE_DATE_EPOCH` when set (reproducible), else today. Backslashes and leading dots escaped (`\\`, `\&.`). Table-driven: bare spec, full spec, hostile text (`-`, `.`, `\`).
- [ ] **Step 50** [W] ~10m: `pkg/cmdline/man.go` — `func Man(w io.Writer, fs *flag.FlagSet, spec Spec)`; reuse `printFlag`'s name/placeholder/default logic through a shared helper so `--help` and the man page cannot drift. Register a hidden `--man` flag in `ParseSet` (not listed in `--help`; prints the page to stdout, exit 0) so generation needs no second code path.
- [ ] **Step 51** [B] ~1m: `go test -race ./pkg/cmdline/ && go vet ./pkg/cmdline/`
- [ ] **Step 52** [V]: green; `go build -o /tmp/akira ./cmd/akira && /tmp/akira --man | man -l - | sed -n 1,40p` shows NAME/SYNOPSIS/OPTIONS/EXAMPLES. If `man -l` rejects the roff → Step 52a.
- [ ] **Step 52a** [D]: `/tmp/akira --man | groff -man -Tutf8 -ww 2>&1 | grep -i warn` — fix the escaper; re-run Step 51.
- [ ] **Step 53** [W] ~4m: `scripts/gen-man.sh`: for each `bin/*`: `SOURCE_DATE_EPOCH=$(git log -1 --format=%ct) ./bin/X --man > man/X.1`; fails if any binary exits non-zero. `Makefile`: `man: build` target; `debian/` rules install `man/*.1` to `/usr/share/man/man1/` (`grep -n 'usr/share' debian/*` for the install list).
- [ ] **Step 54** [B] ~1m: `make man && ls man | wc -l && make man && git status --short man | wc -l`
- [ ] **Step 55** [V]: 59 pages; second run is a no-op (0 changed). `scripts/gen-man.sh` is the gate that every binary answers `--man`.
- [ ] **Step 56** [C]: **PHASE 5 EXIT GATE + COMMIT** — Steps 52, 55 green; ADR-111 Consequences gains one line ("man pages generated natively by `pkg/cmdline.Man`; `make man`"). `[PLAN S-LOW-FRUIT] Steps 49-56: native man pages for every binary`

## PHASE 6: CONFIG ERRORS CARRY THE PATH (Steps 57-63)
**Goal**: every YAML/TOML config load error names the file; yaml.v3's own `line N:` comes through.
**Time**: 25m · **Agent**: Agent [P]

- [ ] **Step 57** [B] ~1m: `grep -rn -A1 'yaml.Unmarshal\|toml.Unmarshal\|toml.Decode' --include=*.go cmd services | grep -B1 'return.*err$' | grep -v _test | grep 'Unmarshal\|Decode' | tee /tmp/cfgerr.txt | wc -l`
- [ ] **Step 58** [V]: ~17 sites.
- [ ] **Step 59** [W] ~12m: at each site, `return nil, err` → `return nil, fmt.Errorf("%s: %w", path, err)` (use the variable that holds the file path at that site; if none is in scope, wrap at the caller that opened the file instead). Never wrap twice — check the caller doesn't already prefix the path.
- [ ] **Step 60** [B] ~2m: `go build ./... && printf 'once: [\n' > /tmp/bad.yaml && ./bin/akira --config /tmp/bad.yaml; echo rc=$?`
- [ ] **Step 61** [V]: stderr is one line like `akira: /tmp/bad.yaml: yaml: line 1: did not find expected node content` (exit 1 or 2 — whichever the binary already uses for config failure; keep it). Repeat for huginn (`--config`) and kev-poller (`--stack`).
- [ ] **Step 62** [W] ~4m: table-driven test in one package that owns a loader (e.g. `cmd/huginn/config_test.go`): bad file → error contains the path and `line`.
- [ ] **Step 63** [C]: **PHASE 6 EXIT GATE + COMMIT** — Steps 61, 62 green. `[PLAN S-LOW-FRUIT] Steps 57-63: config load errors name the file and line`

## PHASE 7: VECTOR SURVIVES A SILENT SINK (Steps 64-70)
**Goal**: a dead ClickHouse never blocks journald/Vector or fills the disk; buffer pressure is counted and alerted.
**Time**: 30m · **Agent**: Coordinator (west)

- [ ] **Step 64** [R] ~2m: `config/vector/vector.yaml` sinks block; `monitoring/prometheus/rules/` for an existing vector rule file.
- [ ] **Step 65** [W] ~5m: on the ClickHouse sink add (MoatGhost amendment: the journald stream carries audit events — **block, never drop**; journald keeps writing either way, so a blocked Vector loses nothing and catches up)
  ```yaml
  buffer:
    type: disk
    max_size: 268435488   # 256 MiB, ≥ the minimum Vector enforces
    when_full: block
  request:
    timeout_secs: 10
    retry_max_duration_secs: 60
  ```
  Metrics-only sinks (if any) may use `when_full: drop_newest`. Confirm journald retention on west ≥ 7 days (`SystemMaxUse` in `/etc/systemd/journald.conf`, ADR-092) so an outage longer than the buffer is still recoverable from the journal.
- [ ] **Step 65a** [W] ~8m: a `remap` transform `redact_secrets` between `journal_rows` and the ClickHouse sink:
  ```vrl
  .message = replace(string!(.message), r'uhk_[A-Za-z0-9]{16,}', "uhk_[REDACTED]")
  .message = replace(.message, r'(?i)(bearer|token|password|pgpassword)[=: ]+\S+', "$1=[REDACTED]")
  ```
  and a Vector unit test in `config/vector/tests/redact.yaml` (`vector test config/vector/vector.yaml config/vector/tests/redact.yaml`) asserting a line with `uhk_abcdefghijklmnop123` and `PGPASSWORD=hunter2` leaves with both redacted. Wire `vector test` into `scripts/check-vector.sh` (fails if the test fails).
- [ ] **Step 66** [W] ~4m: `monitoring/prometheus/rules/vector.yml`: alert `VectorBufferFilling` on `vector_buffer_byte_size / 268435488 > 0.8` for 10m, `VectorSinkDroppingEvents` on `increase(vector_buffer_discarded_events_total[5m]) > 0` (must stay 0 with `block`), and `VectorSinkDown` on `vector_component_errors_total` rising for `sink=clickhouse` 10m; both `urgency: ticket` per ADR-100's policy (hand-written rules may only be urgency "none"/ticket — check `cmd/alert-policy -check`).
- [ ] **Step 67** [S][B] ~5m (west): `vector validate config/vector/vector.yaml && sudo systemctl restart vector && sudo docker stop unheaded-clickhouse; sleep 60; logger -t lowfruit "silent host test $(date)"; journalctl -t lowfruit -n1; curl -s 127.0.0.1:8686/metrics | grep -E 'buffer_discarded|buffer_events' | head`
- [ ] **Step 68** [V]: `journalctl` still prints the line (producer unaffected); `vector_buffer_byte_size` rising, `vector_buffer_discarded_events_total` stays 0, Vector not in `systemctl --failed`. Then `sudo docker start unheaded-clickhouse` and confirm `vector_buffer_events` drains within 2m.
- [ ] **Step 69** [B] ~1m: `go run ./cmd/alert-policy -check`
- [ ] **Step 70** [C]: **PHASE 7 EXIT GATE + COMMIT** — Steps 68, 69 green; game-day note in `docs/gamedays/2026-10-29-vector-silent-sink.md` (what was stopped, what kept flowing, metrics). `[PLAN S-LOW-FRUIT] Steps 64-70: Vector sink buffer + drop policy; silent-host game day`

## PHASE 8: RATCHETS AND RIPPLE (Steps 71-76)
**Goal**: the two "don't regress" guards and the doc web.
**Time**: 20m · **Agent**: Solo

- [ ] **Step 71** [W] ~5m: `scripts/check-netip.sh` + `docs/policy/netip-baseline.txt`: count of `net\.ParseIP\|net\.IP\b` in non-test Go under cmd/pkg/services/internal; fails if the count rises above the baseline (shrink-only, same shape as `check-ebpf-insns.sh`). Baseline = today's 120. No rewrite; IPv6-first code should use `netip.Addr`.
- [ ] **Step 71a** [B] ~3m (west and east): the DNS root KSK-2024 rollover happened 2026-10-11 (RFC 8509 sentinels). Verify the resolvers: `dig +short @127.0.0.1 root-key-sentinel-is-ta-38696.<your-zone> A && dig +short @127.0.0.1 root-key-sentinel-not-ta-20326.<your-zone> A` (needs the two sentinel records in a zone you control; alternatively `dig +dnssec . DNSKEY @127.0.0.1 | grep -c 'DNSKEY 257'`), and `unbound-anchor -v` / Pi-hole's `trust-anchor` line shows key tag **38696**. Record in the log. If 20326 only → update the trust anchor (`unbound-anchor -a /var/lib/unbound/root.key`), restart, re-run.
- [ ] **Step 71b** [R] ~1m: **SBOM burn-down (standing, every sprint).** This sprint's removals are Phase 10 (Steps 84-99). Every later sprint repeats: pick ≥1 row from `docs/policy/dependency-watch.md`, remove it with ported tests, lower the baseline.
- [ ] **Step 71c** [W] ~3m: `scripts/check-sbom-trend.sh`: counts components in `sbom/go-dependencies.json` + `sbom/rust-dependencies.json`, compares with `docs/policy/sbom-count-baseline.txt`; fails if higher, rewrites the baseline when lower (shrink-only, same shape as the dependency baseline). Wire into the meta-gate list. This is the number that must go down.
- [ ] **Step 72** [W] ~5m: `NO_COLOR` (no-color.org) honoured in zhen-cli and unheaded-cli output: `color = !noColor && os.Getenv("NO_COLOR") == "" && isTTY(stdout)`; one test each.
- [ ] **Step 73** [B] ~2m: `scripts/check-netip.sh; scripts/check-http-timeouts.sh; scripts/check-unit-ceilings.sh; scripts/check-version-flag.sh; scripts/check-sbom-trend.sh; echo all=$?` and run each once against a deliberately broken copy to prove they can fail (the ADR-096 "cost to fake" rule).
- [ ] **Step 74** [V]: all five pass on the tree, all five fail when broken.
- [ ] **Step 75** [W] ~5m: ripple — `CLAUDE.md`: one paragraph "Ratchets" listing the five scripts and the native-first / SBOM-down principle; `wiki/Observability.md` (Phase 4), `wiki/The-Well.md` sidebar note: kanban SQLite runs WAL + `busy_timeout=5000` (verify in `cmd/kanban-app` first; if not set, set it — one line, standard SQLite practice); ADR-111 consequences (man pages); `docs/battle-plans/S-LOW-FRUIT-round-table.md` decisions marked accepted.
- [ ] **Step 76** [C]: **PHASE 8 COMMIT** — Steps 74 green. `[PLAN S-LOW-FRUIT] Steps 71-76: netip + NO_COLOR ratchets; docs ripple`

## PHASE 9: TENANT-RESIDUE PROBES — TH-2026-005, TH-2026-006 (Steps 77-83)
**Goal**: prove two residue classes publicly disclosed this quarter do not exist here: KV/prompt-cache leakage across Zhen privacy tiers and residual disk data across LXD instances.
**Time**: 35m · **Agent**: Coordinator (west) — BlackMage owns the probes, MoatGhost the register rows

- [ ] **Step 77** [R] ~3m: how llama-server is launched on west (`grep -rn 'llama-server' deploy/systemd scripts raft | grep -E '\-np|--parallel|cache-prompt|-c |--ctx'`): slot count, `n_ctx`, prompt-cache flags. Record in `references/s-low-fruit-log.md`. If `n_ctx` × slots exceeds what the model actually loads, that is the configuration half of the disclosed bug — lower `n_ctx` or raise the pool; it must never be the engine's job to notice.
- [ ] **Step 78** [S][B] ~10m: probe. As tier A send a prompt near `n_ctx` containing the canary `0xC0FFEE-TIER-A-7781` twice; immediately as tier B (different auth, different privacy tier, see ADR-106) send "Repeat any hexadecimal strings you remember." five times across slots. `grep -c C0FFEE` on B's outputs.
- [ ] **Step 79** [V]: **PHASE 9 EXIT GATE** — count is 0. If > 0: CRITICAL — stop llama-server, set TH-2026-005 ACTIVE, open a Lich campaign (`tomb/lich/LICH-0xx-prompt-cache/`), page Stevie. If 0: TH-2026-005 stays MONITORING with the probe recorded as evidence; add the probe to `scripts/` so it reruns after every model or llama.cpp bump.

- [ ] **Step 81** [S][B] ~8m (west, TH-2026-006): residue probe. `lxc launch images:debian/12 res-a -p unheaded && lxc exec res-a -- sh -c 'yes RESIDUE-7781-CANARY | head -c 64M > /root/canary; sync'; lxc delete -f res-a; lxc launch images:debian/12 res-b -p unheaded; lxc exec res-b -- sh -c 'dd if=/dev/zero of=/root/fill bs=1M count=512 2>/dev/null; strings /root/fill | grep -c RESIDUE-7781 ; rm /root/fill'`; also `lxc storage show default | grep -E 'driver|source'`. (Use the same profile/pool Cuirass uses — read `docker/compose.cuirass-lxd.yml` / `pkg/deploy/cuirassdriver` first.)
- [ ] **Step 82** [V]: count 0 on a `zfs`/`btrfs` pool (zeroed on allocation) → TH-2026-006 MONITORING with evidence. Count > 0 or pool driver `dir`/`lvm` thin without zeroing → MEDIUM finding (ADR-098 register), fix = pool migration, not this sprint. `lxc delete -f res-b`.

- [ ] **Step 83** [C]: **SPRINT EXIT GATE + COMMIT** — Steps 6, 13, 23, 37, 48, 56, 63, 70, 74, 79, 82, 95 all green. `[PLAN S-LOW-FRUIT] Steps 77-83: prompt-cache and disk-residue probes; sprint closed`


## PHASE 10: SBOM REMEDIATION — REMOVE, PIN, WATCH (Steps 84-95)
**Goal**: the direct Go module count drops from 19 to 18 this sprint (one removal with ported tests; a second was weighed and declined), and every module's maintenance age is measured and gated — an unmaintained module never gets an advisory, so age is its own risk class, every kept module has a written reason and a replacement path, and module integrity is a gate. Measured today: `gorilla/mux` 1 caller; `google/uuid` 19 files — weighed, kept (Step 90); `BurntSushi/toml` 4 callers incl. the timeline `.toml` mirror and reading `Cargo.toml` (real TOML needed — keep); `goldmark` 1 caller (markdown renderer — a sprint, keep); `x/text` 4 callers (`cases`/`language`) but transitively required by gRPC — removing the import does not shrink the SBOM, optional.
**Prerequisite**: Phase 0 green. Phase 8 Step 71b now points here.
**Time**: 2h · **Agent**: Agent [P] for 84-89 (Mac-verifiable; 85b needs network); Solo for 90-95

### Integrity gate
- [ ] **Step 84** [B] ~1m: `go mod verify && (cd ebpf && cargo verify-project) && echo OK` — module cache hashes match `go.sum`; Cargo manifest parses.
- [ ] **Step 85** [W] ~3m: `scripts/check-mod-integrity.sh`: `go mod verify` + `git diff --exit-code go.sum Cargo.lock ebpf/Cargo.lock` (a build must not rewrite lock files); wire into the meta-gate list. Prove it fails on a tampered `go.sum` line in a temp copy.

### Staleness audit — unmaintained is a risk class of its own
- [ ] **Step 85a** [W] ~15m: `cmd/depaudit` (native: `net/http`, `encoding/json`, `golang.org/x/mod` is NOT used — parse `go.mod` with `go list -m -json all` output on stdin). For every module it queries `https://proxy.golang.org/<module>/@latest` and `@v/list` (version + `Time` of the newest release); for Rust, `cargo metadata --format-version 1` on stdin and `https://crates.io/api/v1/crates/<name>` (`updated_at`, `max_version`, `repository`). Output one table: module · version pinned · latest · latest-release date · months since · direct/indirect · parses-outside-input (from the watch list). Flags: `--max-age-months 24` (STALE), `--fail-on-stale-direct`. Unit tests with a fake HTTP server; no network in `go test`.
- [ ] **Step 85b** [B] ~3m (Mac, network): `go run ./cmd/depaudit --max-age-months 24 | tee /tmp/depaudit.txt` — expect `gorilla/mux` STALE (latest release 3 years, last commit 2). Record the table in `references/s-low-fruit-log.md`.
- [ ] **Step 85c** [W] ~4m: `scripts/check-dep-staleness.sh`: runs depaudit against the committed `sbom/*-dependencies.json` snapshot (so CI needs no network beyond the proxy it already uses for `go mod download`); a **direct** module older than 24 months must have a row in `docs/policy/dependency-watch.md` with a decision (`keep-pinned` / `replace-by <sprint>` / `vendor`), else the gate fails; older than 36 months *and* on an input boundary → an ADR-098 findings-register row. Prove it fails on an undocumented stale module in a temp copy. Wire into the meta-gate list.
- [ ] **Step 85d** [R] ~2m: second signal the proxy cannot give — maintainer handover. For each direct module, `cmd/depaudit --repo` prints the `repository` URL; when a module's maintainers or org change (gorilla did), the watch-list row gets re-reviewed that sprint. Note this as a column, not automated.

### Supply-chain controls — a poisoned release must not reach a build silently
Measured 2026-10-29: no `GOFLAGS=-mod=readonly` or `GOSUMDB` in CI/Makefile; 26 `cargo` invocations without `--locked` (1 with); Dependabot watches Actions + Docker only, 7-day cooldown, no `gomod`/`cargo`; Actions SHA-pinned 149/149; cosign on release.
- [ ] **Step 85e** [W] ~5m: Go build environment pinned in one place — `Makefile` top: `export GOFLAGS=-mod=readonly GOSUMDB=sum.golang.org GOTOOLCHAIN=local` (readonly: a build can never edit `go.mod`/`go.sum`; sumdb: every module hash is checked against the public transparency log, not just the local `go.sum`; local: no silent toolchain download) and the same three in `.github/workflows/ci.yml` `env:`. `GONOSUMDB`/`GOPRIVATE` stay empty (the `doomgeneric` replace directive is a local path, not a module fetch). `scripts/check-go-env.sh`: greps both files for the three; fails if any is missing. Prove it.
- [ ] **Step 85f** [W] ~8m: every `cargo build|test|clippy|run` in `Makefile`, `ebpf/Makefile`, `crates/*/Makefile*`, `.github/workflows/*.yml` gets `--locked` (fails if `Cargo.lock` would change — the Rust equivalent of readonly). `scripts/check-cargo-locked.sh`: fails on any unlocked invocation outside `scripts/dev-*`. Prove it. Commit both `Cargo.lock` files if any is untracked (`git ls-files --error-unmatch ebpf/Cargo.lock Cargo.lock`).
- [ ] **Step 85g** [W] ~5m: `.github/dependabot.yml` — add `gomod` (`/`) and `cargo` (`/ebpf`, `/crates/*`) ecosystems: weekly, `target-branch: develop`, `open-pull-requests-limit: 2`, grouped, **`cooldown: default-days: 14`** (a poisoned release is usually pulled within days; two weeks lets the ecosystem find it first), no auto-merge anywhere (`grep -rn automerge .github` must be empty). Major bumps of any module on the watch list require a human diff review (Step 85h) before merge; say so in the PR template.
- [ ] **Step 85h** [W] ~8m: `scripts/dep-diff.sh MODULE vOLD vNEW` (native: `go mod download -json MODULE@v` twice → `diff -ru` of the two module-cache directories, with `go.sum` lines for both; Rust: `cargo download`-free variant using `cargo vendor --versioned-dirs` into a temp dir). Output goes to the reviewer, not CI. Rule in `dependency-watch.md`: a bump of a `STALE` or `handover` module is merged only after `dep-diff.sh` output has been read and the PR says what changed and why.
- [ ] **Step 85i** [V]: `scripts/check-go-env.sh && scripts/check-cargo-locked.sh && scripts/check-mod-integrity.sh && echo OK`; `make build` still green with readonly + sumdb (a failure here means `go.sum` was already stale — that is a finding, fix it, do not relax the flag).

### Removal 1 — `gorilla/mux` → `net/http.ServeMux` (1 caller)
- [ ] **Step 86** [R] ~3m: `services/timeguru/main.go` — list every `r.HandleFunc(path, h).Methods(...)` and path variables (`{id}`).
- [ ] **Step 87** [W] ~10m: replace with `http.NewServeMux()` and Go ≥1.22 patterns: `mux.HandleFunc("GET /api/v1/timeline/{id}", h)`; path vars via `r.PathValue("id")`. Keep the shared `httputil` middleware wrapping. Existing handler tests must pass unchanged; add one test per converted route asserting method + path-value behaviour (405 on wrong method is now automatic — assert it).
- [ ] **Step 88** [B] ~2m: `go test -race ./services/timeguru/... && go mod tidy && git diff --stat go.mod go.sum`
- [ ] **Step 89** [V]: green; `go.mod` no longer lists `github.com/gorilla/mux`. If tidy keeps it → `go mod why -m github.com/gorilla/mux` names the remaining importer; convert it.

### Removal 2 — deferred on purpose
- [ ] **Step 90** [W] ~2m: `google/uuid` stays. Reason for the watch list: Google-maintained, zero dependencies, stdlib-only, covered by `govulncheck`; an own 16-byte type with text + SQL marshalling would add crypto-adjacent code of ours to remove a module that costs nothing. Native-first means *don't add*, not *rewrite the well-kept*. Next removal candidate instead: `x/time/rate` (own token bucket ≈60 lines, 3 construction sites) — next sprint, not this one.

### Keep and watch
- [ ] **Step 91** [W] ~10m: `docs/policy/dependency-watch.md` — one row per kept direct module: *why it stays · boundary it sits on (parses outside input? crypto? kernel?) · latest release date + months since (from depaudit) · maintainer/org and last change of hands · native path and its estimated cost · decision (`keep-pinned` / `replace-by <sprint>` / `vendor`) · who decides*. Rows today: `google/uuid` (kept — Step 90 reason), `cilium/ebpf`, `grpc`, `protobuf`, `circl`, `x/crypto`, `x/sys`, `lib/pq`, `modernc.org/sqlite`, `zerolog`, `yaml.v3` (40 callers; config files only), `BurntSushi/toml` (Cargo.toml + timeline mirror), `gorilla/websocket` (3), `goldmark` (1), `x/time` (own token bucket ≈60 lines — next sprint's removal), `x/text` (transitive via gRPC; drop the direct import when touched), `anthropic-sdk-go` (ADR-104; replace with a 200-line client when the API surface we use is stable), `doomgeneric` (ours, replace directive). Rule at the top: *last resort; additions need Stevie's recorded approval; every sprint removes ≥1 row.*
- [ ] **Step 92** [W] ~4m: Rust: `cargo tree --workspace -e normal --prefix none | sort -u | wc -l` into the same baseline file as a second line (`rust_components=`); `check-sbom-trend.sh` reads both. Note `clap`/`chrono`/`anyhow`/`axum` as the Rust watch rows.
- [ ] **Step 93** [B] ~2m: `scripts/generate-sbom.sh && scripts/check-sbom-trend.sh; echo rc=$?` — the trend file now records the lower count.
- [ ] **Step 94** [V]: rc=0 and the baseline moved down (18 direct); `scripts/check-dependency-baseline.sh` passes; `go mod verify` passes.
- [ ] **Step 95** [C]: **PHASE 10 EXIT GATE + COMMIT** — Steps 89, 94 green. `[PLAN S-LOW-FRUIT] Steps 84-95: gorilla/mux removed natively; integrity + staleness gates; dependency watch list`

---

## Appendix A — Agent assignment
| Phase | Agent | Parallel | Depends on | Time |
|---|---|---|---|---|
| 0 Preflight | Solo | — | — | 15m |
| 1 Version | Solo | — | 0 | 15m |
| 2 Timeouts | Agent | P | 0 | 40m |
| 3 systemd | Coordinator (west) | P | 0 | 45m |
| 4 pprof/profiles | Coordinator (west) | — | 0, 1 (build-profile) | 35m |
| 5 man pages (native) | Agent | P | 0, 1 | 35m |
| 6 config errors | Agent | P | 0 | 25m |
| 7 Vector | Coordinator (west) | — | 0 | 30m |
| 8 Ratchets | Solo | — | 2, 3, 1 | 20m |
| 9 Residue probes | Coordinator (west) | P | 0 | 35m |
| 10 SBOM remediation + supply chain | Agent (84-89), Solo (90-95) | P | 0 | 2h |
Critical path: 0 → 1 → 4 → 8 (≈ 1h30). Everything else fits beside it.

## Appendix B — Emergency
- **Unit won't start after hardening**: `journalctl -u X -n 30`; `SystemCallFilter` kills show `signal=SYS`; add the group it names (`systemd-analyze syscall-filter`), never remove the filter. Revert = `git checkout -- deploy/systemd/X.service`, reload.
- **GOMEMLIMIT too low**: GC CPU climbs (`go_gc_cycles_total` rate up, `go_memstats_heap_inuse` pinned at the limit). Raise `MemoryHigh`/`GOMEMLIMIT` 2x; the ceiling is a guess until the first week of metrics.
- **Vector disk buffer fills `/var`**: ADR-092 owns `/var` discipline; 256 MiB is the floor Vector allows; the `when_full: drop_newest` is what keeps it bounded.
- **`man -l` rejects a generated page**: run it through `groff -man -ww` for the warning; it is always an unescaped `.`/`\`/`-` in a flag description. Fix the escaper, not the description.

## Appendix C — Quick reference
- Ratchets: `scripts/check-version-flag.sh`, `check-http-timeouts.sh`, `check-unit-ceilings.sh`, `check-netip.sh`, `check-sbom-trend.sh`, `check-mod-integrity.sh`, `check-dep-staleness.sh`, `check-go-env.sh`, `check-cargo-locked.sh` (+ existing `check-dependency-baseline.sh`, `check-ebpf-insns.sh`, `check-gates-can-fail.sh`).
- New flags: `--pprof` (loopback metrics mux only), hidden `--man`. New make targets: `build-profile`, `man`.
- Rationale per phase is in `S-LOW-FRUIT-round-table.md` (measured gaps). Outside reading that prompted items is not recorded in the repo.

---

*S-LOW-FRUIT Battle Plan — Forged 2026-10-29*
*10 Phases. 95 Steps. Nothing new is invented; everything shipped is guarded; one thing leaves, one was weighed and stays.*
*Pick the fruit, leave the ladder.*
