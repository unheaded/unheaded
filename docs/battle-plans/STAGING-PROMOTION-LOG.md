# Staging promotion log — develop → staging → main

Session opened 2026-08-09. Flow agreed with Stevie: pull one contiguous batch into
`staging`, prove it builds and tests green, Stevie does manual/GUI QA, **then** it
moves to `main` or the next batch comes in. Nothing leaves `staging` unapproved.

**Why contiguous:** the 124 commits are a linear chain on top of `main`
(`0f443ded`). Thematic batches are ranges in ladder order, so every promotion is a
fast-forward — zero conflicts. Cherry-picking themes out of order would collide
repeatedly (the progress-log docs alone touch one file 15 times).

## Baseline at `main` (`0f443ded`) — measured, not assumed

Established in a detached worktree so batch results can be read as deltas.

- `go build ./...` — clean
- `go vet ./...` — **exit 1**, aborts on `cmd/wotan-ctl/doom.go:12`:
  `github.com/unheaded/doomgeneric@v0.0.0: replacement directory
  ../projects/doomgeneric/unheaded does not exist`. It never reaches the rest of
  the tree.
- `go test ./...` — 7 failures:
  `cmd/wotan-ctl [setup failed]`, `dashboard-backend/internal/server` (2),
  `cmd/wiki-server` (4)

**The 6 dashboard-backend/wiki-server failures are pre-existing.** They are closed
later in the ladder by rungs #51 (`2cc3bd8c`) and #52 (`9fb5166f`), both in B5.
Do not attribute them to any batch before B5.

## Batch map

| # | rungs | head | theme | QA surface |
|---|---|---|---|---|
| B1 | 1–12 | `1dfbae77` | security remediation program, s79 CI sweep, first gosec closures | CI/lint only |
| B2 | 13–29 | `356cd372` | gosec rule closures (G118→G103) | build + go test |
| B3 | 30–38 | `197f20a1` | per-service UIDs, trivy, image pinning | docker images |
| B4 | 39–47 | `b39fb207` | ruff autofix, waf/forge lib split, clippy gate | cargo |
| B5 | 48–69 | `75cfe1d8` | python/shell hygiene phases | scripts, notebooks |
| B6 | 70–93 | `015218c7` | bandit + eslint/shellcheck gating flips | JS front ends |
| B7 | 94–109 | `910e9dfe` | The Well, dark-mirror, hosts, healthchecks, nix, runbooks | **heavy GUI QA** |
| B8 | 110–115 | `8b14029b` | daemon panic, systemd, k8s, CI gate | services start |
| B9 | 116–124 | `5b172807` | python SBOM, SRI, docs, timeguru | dashboard/timeline |

B1's boundary was originally set at rung 8 (`f3cb7bb3`) and **moved to rung 12**
during verification — see below.

## B1 — rungs 1–12, head `1dfbae77` — IN STAGING, AWAITING QA

| gate | result |
|---|---|
| `go build ./...` | clean |
| `go vet ./...` | **clean (exit 0)** — better than `main` |
| `go test ./...` | 6 failures, **identical to `main`'s pre-existing set**; `cmd/wotan-ctl` now **passes** (was `[setup failed]` at `main`) |
| `scripts/check-gosec-ratchet.sh` | PASS |
| `scripts/check-timeline-freshness.sh` | PASS |

### Why the boundary moved from rung 8 to rung 12

At rung 8, `go vet ./...` reported
`pkg/ebpf/munmap_linux.go:31:30: possible misuse of unsafe.Pointer`.

That line is **byte-identical at `main`** and no commit in rungs 1–8 touches the
file. It was not a regression — it was **previously masked**: `go vet` at `main`
aborts on the `wotan-ctl` module-resolution error before reaching `pkg/ebpf`. B1
fixes that hard failure, which uncovers the warning behind it.

Note `munmap_linux.go` carries `//nolint:govet`, which plain `go vet` does not
honour (same class of trap as `#nosec` needing to lead the comment — see
`reference_gosec_nosec_directive`). The real fix is rung #12 (`1dfbae77`), which
drops the `unsafe.Pointer` conversion. Extending B1 to include it makes the batch
green on vet rather than handing over a known-red gate.

### Tree note

`demos/doom/doom.data` (150 KB, Jul 29) shows as untracked on `staging`. It has
never been tracked in any branch; rung #49 (`e9cb6d33`, in B5) adds the ignore rule
that hides it on `develop`. Harmless, resolved by B5.

`db/migrations/init.sh` — root-owned 0-byte docker artifact from the ADR-091 bug,
already explained in `docker-compose.yml:230`. Removed.

### QA surface for B1

Thin by design. No service behaviour, no UI, no wire format changed. The batch is
CI gates, gosec annotations, per-service run-as identities, the GPL boundary
classifier, and untracking an 18 MB binary from SBOM scans. The meaningful
observable is that the gates run and pass, which is recorded above.

## Full-stack bring-up on B1 code — 2026-08-09

`docker compose up -d --build`, project `unheaded-dev`, all 10 first-party images
rebuilt from `staging`. Nothing that was previously running contained our code —
the 3 containers that were up (postgres, victoria, grafana) are stock third-party
images — so the rebuild is what put staging code into anything at all.

**16 of 17 services healthy.** Dashboard (20000), Kanban (20001), Grafana (3001),
VictoriaMetrics (8428), ClickHouse (8123), Traefik (21000) all serve. timeguru,
architect, captain, micromanager, monad, sophia and wotan all return 200 on
`/health`. Bare-root 404s on the 19xxx services are expected — they expose
`/health`, `/metrics`, `/ready`, not `/`.

### cuirass is in a restart loop — rung #110 reproduced live

```
panic: pattern "/health" (registered at cmd/unheaded-daemon/main.go:771)
       conflicts with pattern "/health" (registered at pkg/transport/health.go:93)
```

`next.md` recorded this as *"unreachable-by-config: it has never started."* That
understates it: `make dev` reaches it, and the control plane crash-loops on B1
code. Fixed by rung #110 (`7c86b443`) in B8.

**Not cherry-picked, deliberately.** An out-of-order commit on `staging` ends the
fast-forward property — every later batch would stop being a clean `--ff-only` and
begin conflicting. The choice is to advance in order through B8, or QA B1 with the
control plane down. Stevie's call.

### The Well — live state of the existing volume

Measured, not inferred, on `unheaded-dev_postgres-data`:

| database | tables | should be |
|---|---|---|
| `unheaded` (maintenance) | **9** — `kanban_tasks`, `kingdom_config`, `timeline_milestones`, `wotan_*`, `audit_events`, `service_health` | **0** |
| `unheaded_app` | 4 (zhen only) | app schema incl. `kanban_tasks` |
| `unheaded_ops` | **0** | ops schema |
| `unheaded_config` | **0** | config schema + seed |

All three databases and all eight roles exist — including `huginn_reader`, so the
volume is in better shape than `next.md` feared. But migration 003 landed in the
maintenance database instead of `unheaded_app`.

The ADR-091 bug is **dormant here**: `initdb.d` does not run against a populated
data directory, so the broken mount cannot bite this volume.

### A defect B7 does not close — found by running the stack

`pkg/database/config.go:38` reads `WELL_DB`; `docker-compose.yml` sets
`WELL_DB: "unheaded"` for kanban-app. Kanban therefore reads the maintenance
database — which is exactly where the ADR-091 bug put `kanban_tasks`. **It works
today only because the bug and the misconfiguration cancel.** Verified live:
Postgres connected, 72 tasks served, writes enabled.

On `develop` (post-B7), `db/init.sh` routes `003_app_schema.sql` to `unheaded_app`
and leaves the maintenance database empty — but `WELL_DB` is **still `"unheaded"`**
at `docker-compose.yml:759`. So after B7, any **clean volume** gives kanban-app an
empty maintenance database and `kanban_tasks` will not exist.

B7 trades a dormant bug for a live one on fresh installs. **Fix `WELL_DB` before
B7 promotes.** Existing volumes are unaffected either way.

## The bar, and how it is measured — 2026-08-09

Stevie: *"I expect everything to work as well or better than demo through these
pulls and fixes — I understand things may break and we may need to do code edits
to get them back in line."*

`scripts/qa-smoke.sh` exists so that bar is checkable rather than re-argued each
batch. 32 probes: every compose service running, every HTTP endpoint, `/health`
on all eight services, kanban rows served out of The Well, and **flow movement**.

The flow check asserts `total_packets` *advances* between two samples rather than
merely being present — a static graph was the real failure mode, and a
presence-only check would have passed while the canvas sat frozen.

Baselines land in `docs/battle-plans/qa-baseline/<ref>.json` so batches diff
against each other instead of against memory.

### B1 — 30 / 32

Both failures are cuirass: `container/cuirass restarting`, `health/cuirass 000`.

**Not a regression.** `pkg/transport/health.go:93` and
`cmd/unheaded-daemon/main.go:771` are byte-identical at `main` and on `staging`,
and `git diff main..staging` touches neither file. The demo baseline had the
control plane crash-looping too. B1 is equal; rung #110 (`7c86b443`, B8) is what
makes it better.

### The Flow Graph was never broken by the ladder

Reported as "stuck static rather than piping real data". Root cause: nothing was
publishing `ebpf.flow.events`. `demo-trace-injector` is the publisher, and per
ADR-088 it is a **bare-metal daemon, not a compose service** — `docker compose up`
never starts it, and it did not survive the reboot. The dashboard was polling
Wotan correctly the whole time against empty topics.

B1's only change to dashboard-backend is a comment plus
`//nolint:gosec` → `# #nosec` (see `reference_gosec_nosec_directive`). The flows
handler and the eBPF ingestor are byte-identical to `main`, so the graph was
equally static before the promotion.

Restarting the injector restored it: 54 → 55 active flows, 308 → 414 packets,
240 KB → 319 KB across four seconds.

**Two honesty notes.** The data is *synthetic* — the tool's own header says it
exists for when the real XDP pipeline is not running on this host. And the API
reports `"source":"ebpf"` for it regardless of publisher, because the backend
stamps that whenever the ingestor is non-nil. Real flow data needs
`trace-collector-go` attaching XDP, which requires privileges the dashboard
container deliberately drops.

The injector currently runs from a scratchpad path and will die on the next
reboot, reproducing this exact confusion. Making it a compose service or a
systemd unit is unladdered work for `develop`.

### Why cuirass cannot simply be patched on `staging`

Any commit on `staging` that is not already on `develop` ends the fast-forward
property, and every later batch stops being a clean `--ff-only`. The same applies
to an uncommitted working-tree edit, which blocks the next merge outright. So
fixing cuirass early is a **ladder-ordering decision**, not a coding one:

1. **Advance in order** (B2 → B8), smoke-testing each. Nothing regresses at any
   point, and cuirass turns green when B8 lands. Recommended.
2. **Jump to B8 now**, then come back. Fastest to green, but B2–B7 land unreviewed.
3. **Accept equal-to-baseline** and QA B1 with the control plane down.

## Definition of Done — a batch leaving `staging` for `main`

Per Micromanager. Partial credit is not done. Every line is a gate that is
actually run in this session, not an aspiration — an unrunnable checklist is
worse than none, because it gets waved through.

### Build + test
- [ ] `go build ./...` clean
- [ ] `go vet ./...` clean
- [ ] `go test ./...` — failures **no worse than the recorded `main` baseline**
      (6 pre-existing: dashboard-backend ×2, wiki-server ×4, closing at B5)
- [ ] every `scripts/check-*.sh` present at that rung PASSes
- [ ] Rust batches only: `cargo clippy` / `cargo test` on affected workspaces
- [ ] eBPF batches only: `ascend-linux` artifact still 901888 bytes
      (`scripts/bpf-verifier-check.sh` clobbers this — rebuild and re-confirm)

### Live stack
- [ ] images **rebuilt** (`docker compose up -d --build`) — a stale image means
      the batch was never actually exercised
- [ ] `scripts/qa-smoke.sh` score **≥ the previous batch's**, never lower
- [ ] any new FAIL traced to a cause and either fixed or recorded as
      pre-existing **with the evidence**, not asserted

### Review
- [ ] `/code-review` run over the batch range
- [ ] every finding dispositioned: fixed, or recorded with why not

### Sacred Principle
- [ ] no new path by which an engineer could reach customer data
- [ ] eBPF changes capture **metadata only**, never payload
- [ ] no credentials added to the repo (`feedback_no_creds_in_repo`)

### Ladder integrity
- [ ] `staging` is still a strict fast-forward of `develop`
      (`git merge-base --is-ancestor staging develop`)
- [ ] no cherry-picks, no working-tree edits carried across a merge

### Sign-off
- [ ] Stevie's manual GUI QA
- [ ] Stevie commits / promotes — **never Claude** (`feedback_stevie_owns_commits`)

## B2 — rungs 13–29, head `356cd372` — IN STAGING

17 commits, 259 files, +2564/−1421. gosec rule closures: G118, G402, G204,
G301/302/306, G304 (114 sites), G305 container-layer escape, G402 TLS, the WAF
Host-header open redirect, stored XSS in the wiki renderer, G115 integer
conversions in DNS compression pointers and ELF relocation bounds, credential
rotation rollback, and G103 bpf(2) attr structs.

| gate | result |
|---|---|
| `go build ./...` | clean |
| `go vet ./...` | clean |
| `go test ./...` | 6 failures — unchanged from `main` baseline |
| `check-gosec-ratchet.sh` | PASS |
| `check-timeline-freshness.sh` | PASS |
| `qa-smoke.sh` | pending image rebuild |
| `/code-review` | running |

Note this batch contains **real security fixes with behavioural reach** — a WAF
redirect guard, an XSS escape path, DNS pointer arithmetic, and container layer
extraction. Unlike B1 these can change what the running system does, so the smoke
run matters here in a way it did not for B1.

## Full compile + reload — B2 plus the four review fixes — 2026-08-09

Every language surface built from the `staging` working tree (fixes uncommitted,
which is what the Docker context copies).

| surface | result |
|---|---|
| Go `go build ./...` | clean |
| Go `go vet ./...` | clean |
| Rust — 11 workspace roots | **11 / 11 OK** |
| eBPF `monad-cpu-ebpf --features ascend-linux` | builds, **903864 bytes** |
| `go test ./...` | 6 failures, unchanged from the `main` baseline |
| `check-gosec-ratchet.sh` | PASS — and verified it can FAIL |
| `check-timeline-freshness.sh` | PASS |
| docker images | 10 / 10 rebuilt, stack up |
| `qa-smoke.sh` | **30 / 32** — equal to B1 and B2, no regression |

Rust roots built: `cmd/{ebpf-collector,ebpf-loader,trace-collector,upc-bootctl,waf}`,
`crates/{doom-runner,monad-mbc,upc-api,zhenai-forge,zhend}`, `ebpf/af-xdp`.

### The eBPF artifact is 903864, not the 901888 in next.md — this is not a regression

`git diff main..staging -- ebpf/` is **empty**: B1 and B2 do not touch eBPF at all,
so 903864 is `main`'s size by construction. The 901888 figure was measured at
`develop`'s tip (rung 124), where later batches change it. Expect the number to
move as the ladder advances; compare against the *previous batch*, never against
next.md. The single `unused_unsafe` warning is pre-existing for the same reason.

### A build-context defect I introduced, and fixed

The `main`-baseline worktree was created at `~/tmp/unheaded/base` — **inside the
repo** — because a relative path fell back there. `.dockerignore` does not exclude
it, so two stack builds copied **225 MB** of duplicate tree into the Docker build
context, and every `grep`/`git ls-files` sweep in this session had to filter
`base/` out by hand.

No image was corrupted (`go build ./...` skips nested modules), but it inflated
every build and would have polluted an SBOM scan. Worktree removed; the rebuild
above is from a clean context.

**Lesson for the next baseline comparison:** put throwaway worktrees outside the
repo, or add them to `.dockerignore` first. A build context is not just the files
you think you are shipping.

### Still the only failure: cuirass

Unchanged and unchangeable at this rung — `main` crash-loops identically, and the
fix is rung #110 in B8.

## PARKED — The Well reachability + client split-brain (Stevie, 2026-08-09)

Raised during B2 QA, **deliberately not acted on.** Stevie is doing lab network
segmentation next; that decision sets where every client should point, so fixing
the pointers first would be rework.

### Intent, confirmed — the loopback bind is correct, not a bug

`127.0.0.1:5432` keeps The Well off `192.168.69.0/24` so a compromised IoT device
(smart TV et al) has no path to the database. That is working as designed.

**The Well is not network-isolated:** the host reaches it on 127.0.0.1:5432
(verified), and 13 of 17 compose services sit on the `data` network and reach
`postgres:5432` by Docker DNS. zhenai is a **host process**, not a container, and
reaches it via the loopback map — its four tables do exist in `unheaded_app`.

**Wanted:** reachable from EAST and WEST *and* all containers, without exposing it
to the IoT segment. That is a segmentation build-out (data VLAN or WireGuard
overlay), not a compose port change.

### The part that is NOT blocked on segmentation

Container-to-container access already works over Docker DNS and is unaffected by
anything done at the LAN layer. Three clients currently disagree about which
database holds the data:

| client | database | correct? |
|---|---|---|
| `kanban-app` (`WELL_DB`) | `unheaded` (maintenance) | ✗ works only because ADR-091 misfiled the table there |
| `raft/zhen_app.py:137` | `unheaded_app` | ✓ matches B7's `init.sh` |
| `raft/zhen_app.py:345` | `unheaded` (maintenance) | ✗ |
| `dashboard-backend` | unset → `localhost` | ✗ never persists health; warns once, then runs degraded |

`zhen_conversations` exists in **both** `unheaded` and `unheaded_app` — kanban and
zhenai are already reading different databases for overlapping data.

Only three Go binaries import `pkg/database` (`dashboard-backend`, `kanban-app`,
`zhen-agentd`), so the fix is bounded.

**When it is picked up:** make `unheaded_app` the single answer, and land it WITH a
data reconciliation — the 72 live kanban tasks are in the maintenance database, so
moving the pointer without moving the rows empties the board. Keep the migration
separate and reversible; do not fold it into a ladder batch.
