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

## B3 — rungs 30–38, head `197f20a1` — IN STAGING (2026-09-08)

Merged as **`4935f21a`**, a merge commit — not a fast-forward. `staging` diverged
from `develop` at B2 when the five review fixes landed here, so `--ff-only` fails
from B3 onward. That is expected and correct; see next.md §2.

**Zero conflicts.** 45 files, +988/−179.

| gate | result |
|---|---|
| `go build ./...` | clean |
| `go vet ./...` | clean (exit 0) — still better than `main` |
| `go test ./...` | **6 failures, identical to the pre-existing set** (`dashboard-backend/internal/server` ×2, `cmd/wiki-server` ×4); 242 packages ok. Closes at B5. |
| `check-gosec-ratchet.sh` | PASS |
| `check-manifest-yaml.sh` | PASS — **new in this batch** |
| `check-secrets-baseline.sh` | PASS — **new in this batch** |
| `check-timeline-freshness.sh` | PASS |
| Rust workspaces | **11 / 11** build |
| eBPF | builds; `git diff 356cd372 HEAD -- ebpf/` is **empty**, so no size question arises at this rung |
| docker images | **10 / 10 first-party rebuilt**, stack up, 17 containers |
| `qa-smoke.sh` | **33 / 35 — equal to B2, no regression** |

### The two failures are still cuirass, and still not ours to fix

`container/cuirass` restarting + `health/cuirass` 000. `main` crash-loops
identically on the duplicate `/health` pattern registration; B3 touches neither
`cmd/unheaded-daemon/main.go` nor `pkg/transport/health.go`. Fix is rung #110, B8.

### Two new gate scripts arrived with this batch — and neither could fail

`check-manifest-yaml.sh` and `check-secrets-baseline.sh` came in at rung 38
(`197f20a1`), the same commit that fixed the security workflows never running on
`develop`. Both pass on the merged tree, which is what the gate table above
records.

**Passing was not the same as working.** The `/code-review` pass below found three
separate defects in `check-secrets-baseline.sh`, any one of which let a live
credential through green, plus the same shape again in the batch's new
`pkg/uids` enforcement test. B2's lesson was that a guard which cannot fail is not
a guard; B3 shipped three more. See the review section for the fixes and the
red-first verification of each.

### Environment note — the reboot recipe held

Machine had been powered down since 2026-08-10. Every host daemon in next.md §1
(injector, wiki, llama-server, `cs serve`, zhen_app) was absent while all 17
containers reported healthy — exactly the ADR-088 confusion the briefing warns
about. After restoring all five, smoke returned to 33/35 *before* the B3 merge,
confirming the baseline was environmental and not code.

**The injector gotcha recurred as predicted.** `docker compose up -d --build`
restarts Wotan, the injector's subscription is lost, and it must be restarted or
the flow graph freezes. Done; `grep -c forbidden /tmp/injector.log` = 0 after.

**The `pgrep -f` self-kill gotcha bit a third time** (exit 144). `pgrep -f
demo-trace-inject` matches the shell running that very command. `pgrep -x` cannot
help — Linux truncates `comm` to 15 chars and `demo-trace-injector` is 19. What
works: `ps -eo pid,comm --no-headers | awk '$2 ~ /^demo-trace-inj/ {print $1}'` —
match on `comm`, which never contains the search pattern.

### Signing

`4935f21a` is **unsigned**. `gpg --sign` timed out waiting on a curses pinentry on
`/dev/pts/1` and the merge aborted with `failed to write commit object`. Re-run
with `--no-gpg-sign` so QA was not blocked. **This commit needs
`git commit --amend -S --no-edit` before B3 leaves `staging`** — it is the tip, so
the amend is cheap now and gets expensive once B4 lands on top.

### `/code-review high` over B3 — 13 findings, 11 fixed, 2 deferred

Same shape as the B2 review, and the same headline lesson: **the batch's two new
gate scripts and its new enforcement test all passed, and all three could not
fail.** B2 taught that a guard which cannot fail is not a guard; B3 shipped three
more of them. Every fix below was verified red-first.

#### Fixed

| # | file | defect |
|---|---|---|
| 1 | `deploy/k8s/policies/enforce-trusted-registries.yaml` | **the allowlist ended in bare `ghcr.io/` and `docker.io/`** |
| 2 | `internal/bpfmap/bpfmap.go` | `UpdateBatch` issued bpf cmd **12 = `BPF_MAP_GET_NEXT_ID`**, not a batch command |
| 3 | `kubernetes/manifests/base/*` ×11 | all 11 Kingdom services still shared `runAsUser: 65532` |
| 4 | `pkg/uids/uids_test.go` | the enforcement test could not see any of them |
| 5 | `pkg/uids/uids_test.go` | walk root excluded `deploy/k8s/**` and `overlays/**` |
| 6 | `go.mod` | comment said "Now 1.26.5" above `toolchain go1.25.12` |
| 9 | `.trivyignore.yaml` | KSV-0125 had no `paths:` — a repo-wide suppression |
| 10 | `scripts/check-secrets-baseline.sh` | compared a **count**, so swapping one fingerprint for another passed |
| 11 | `scripts/check-secrets-baseline.sh` | missing ceiling file silently re-baselined and passed |
| 12 | `scripts/check-secrets-baseline.sh` | `grep -cE … \|\| echo 0` yielded `"0\n0"`; every later `[ -gt ]` aborted rc=2 and fell through to PASS |
| 13 | `.github/workflows/static-analysis.yml` | `setup-python@v5` vs `@v6` everywhere else |

#### #1 — removing `"unheaded/"` was a no-op, and the rest of the list was dead weight

The rego (`trusted-registries.yaml:29`) is a plain `startswith` on the raw image
string, so a short prefix is a wildcard. With `- "ghcr.io/"` and `- "docker.io/"`
at the bottom, **`docker.io/unheaded/cuirass:latest` — the exact squatted-namespace
case the commit's own comment says was closed — was still admitted**, along with
`ghcr.io/anyone/anything`. All 20-odd specific entries above them were inert.

Rewritten so every namespace is named. Verified by extracting all 41 `image:`
strings from `kubernetes/manifests`, `deploy/k8s` and `helm` and matching each
against the list: **38 real images all admitted, 6 squat/typo cases all rejected**
(the other 3 strings are Helm values keys, not images).

Two adjacent claims were also false and are corrected: `.trivyignore.yaml` said
"all first-party manifests now use ghcr.io/unheaded" — **12 manifests use
`ghcr.io/stevenrbellis/unheaded/`**. Both namespaces are now listed explicitly.

Unqualified official images are pinned with their colon (`"busybox:"`, not
`"busybox"`) so the entry cannot also admit `busybox-evil/…`. Fully qualifying
them as `docker.io/library/*` is the better fix and is left as a manifest change.

**Also noted, not fixed:** the rego reads `spec.containers` only — `initContainers`
and `ephemeralContainers` bypass the policy entirely. Separate finding, separate fix.

#### #2 — the batch reviewed the attr struct and never checked the command number

`enum bpf_cmd` puts `BPF_MAP_GET_NEXT_ID` at 12; the batch commands start at 24
(`LOOKUP_BATCH` 24, `UPDATE_BATCH` **26**). The kernel read the attr as a
`start_id/next_id` request, wrote nothing, and returned `ENOENT` — which the error
branch deliberately swallowed as "end of map is normal", so **`UpdateBatch`
returned `(count, nil)`: full success, zero entries written.**

What makes this worth writing down: B3 *rewrote these exact lines* and added
`abi_layout_test.go` blessing the attr struct as ABI-correct. The struct was
correct. The verification was aimed one field away from the defect, which made the
call look audited.

Three changes: the command is now a named `bpfMapUpdateBatch = 26` constant
carrying the enum table; `ENOENT` is no longer exempt (that exemption belongs to
the LOOKUP family — an update has nothing to iterate); and a short write is now
reported instead of echoing the caller's own count back at them.

**No live callers** — `UpdateBatch` is unreferenced outside its own file, so this
was latent, not an active data-loss bug.

#### #3/#4/#5 — the guard was aimed at filenames that do not exist

`TestManifestsMatchRegistry` matched `<service>.yaml` and `<service>-daemonset.yaml`.
The 11 Kingdom services are laid out as `<service>/deployment.yaml`, so the walk
passed straight over every one of them while they all shared `runAsUser: 65532`.
The `found == 0` backstop could not catch it either: 8 telemetry files always
matched, permanently satisfying it.

Rewritten to match by **directory** as well as filename, across three roots
(`manifests/base`, `manifests/overlays`, `deploy/k8s`). Running it red first
exposed more than the review reported — including that my own first pass at the
Helm-only exemption list was wrong, which is the guard doing its job on its author.

The true state it uncovered: `deploy/k8s/` **already had correct per-service UIDs**
(16740–16746, 16728). It was `kubernetes/manifests/base/` that was stale — and
cuirass and dashboard-backend exist in **both trees with different UIDs**. All 11
now carry their registry UID (`runAsUser`, `runAsGroup`, `fsGroup` together).

`found == 0` is replaced by an explicit `helmOnly` list (`haproxy-ingress`,
`unheaded-daemon`), so a service that *disappears* from the walk now fails instead
of quietly joining the unchecked majority. Verified both ways: regressing one UID
to 65532 fails; deleting a manifest the registry claims fails.

**A false positive in the review, worth recording.** Findings #4 and part of #3
reported anamnesis/kenoma/pleroma as having "no runAsUser" and suricata as running
as root. The gnostic three *do* set it — in **flow style**
(`securityContext: { runAsUser: 16760, … }`), which the line-anchored regex could
not see. The regex now matches both styles. A gate that cries wolf costs the same
credibility as one that sleeps.

#### Deferred — 2 findings, both needing a live cluster

- **#7** `void-collector-daemonset.yaml` — `/sys/fs/bpf` and `/sys/kernel/debug`
  are `root:root 0700`, and **`fsGroup` does not apply to hostPath volumes**; the
  kubelet does not chown them. UID 16780 would fail at map pin / probe attach with
  `EACCES`. Plausible and specific, but the fix (an initContainer chown, or
  `BPF_F_*` pinning changes) is unverifiable without a node.
- **#8** `overlays/gpu-vllm/vllm.yaml` — UID 16781 has no `/etc/passwd` entry, so
  `HOME=/`, and vLLM/PyTorch/ROCm write `~/.cache/huggingface`, `~/.triton`,
  `~/.config/miopen` at model load with `/models` mounted read-only. Needs the GPU
  node to confirm.

Both are the category B3 itself named — "record what needs a live cluster".

**suricata and wireguard were deliberately left unhardened**, and this is now
explicit in code rather than invisible. `pendingHardening` in `uids_test.go` names
both with the reason: suricata is `runAsNonRoot: false` with NET_ADMIN/NET_RAW/
SYS_NICE for AF_PACKET capture on `-i any`; wireguard needs SYS_MODULE to insert
the kernel module and the lscr.io image's s6 init drops to PUID/PGID itself, so a
kubelet-set `runAsUser` pre-empts it. The test still asserts their manifests exist
and still fails if one is deleted — it just does not yet demand the UID. **Emptying
`pendingHardening` is the definition of done for that follow-up.**

#### The secrets ratchet now enforces what its own header promised

The header said "a new finding cannot be silenced by appending its fingerprint."
It compared totals, so delete-one-append-one kept the count at 25 and passed green
with a live credential in the tree. It now compares the fingerprint **set** against
`docs/security/gitleaks-baseline-fingerprints.txt` (which replaces the count file —
a file named `-count.txt` holding the authoritative state was itself misleading).

Verified: swapping one fingerprint for another **fails** (the old script passed it);
a missing manifest **fails** instead of self-initialising; emptying `.gitleaksignore`
correctly lowers the ceiling 25 → 0, which the `"0\n0"` bug had made impossible.

#### Gates after the fixes

Build clean, vet clean, **same 6 pre-existing test failures and no new ones**,
4/4 `check-*.sh`, stack rebuilt, **`qa-smoke.sh` 33/35 — unchanged**.

## B4 — rungs 39–47, head `b39fb207` — IN STAGING (2026-09-09)

Merged as **`50420a2d`**, signed, zero conflicts. 157 files, +9007/−3284.

| gate | result |
|---|---|
| `go build` / `go vet` | clean |
| `go test ./...` | **6 pre-existing failures, no new ones** |
| `check-clippy.sh` | PASS — **after being fixed; it was reporting green over two non-building workspaces** |
| `check-python-syntax.sh` | PASS — 70 files |
| other `check-*.sh` | 4/4 PASS |
| Rust workspaces | **18 / 18** — up from 16/18, and prior batches only ever checked a hardcoded 11 |
| docker images | rebuilt, stack up |
| `qa-smoke.sh` | **33 / 35 — equal to B2 and B3** |

Grafana briefly showed `000` on probe after the rebuild. Not a regression: the
rebuilt image ran a long schema migration, and it returns 302 on :3001 once
finished. (It is mapped to **3001**, not 3000.)

### The pattern recurred for a fourth consecutive batch

B2: gosec ratchet guards unreachable. B3: secrets ratchet compared a count, and
the uids test walked past every violation. B4 shipped `check-clippy.sh` — whose
own header calls out "green because it could not fail" — and it had the same
defect twice over:

- its grep only keeps `file:line:col` diagnostics, so target-resolution errors
  (`error: can't find bin ... --> Cargo.toml`) were dropped, **and** cargo's
  exit status was never checked. Two workspaces exiting 101 counted as zero
  warnings and zero errors.
- a baseline file with no numeric line left `ALLOWED` empty, both comparisons
  aborted rc=2, and execution fell through to PASS — byte-for-byte the bug
  fixed in `check-secrets-baseline.sh` the day before.

Both fixed and verified red-first.

### What the fixed gate then found: two crates that never built

Neither is a B4 regression — both predate develop/staging (2026-02-25 and
2026-04-11). B4 merely added their `Cargo.lock` files, which is how the gate
discovers workspace roots.

**Root cause in both cases was build-config placement, in mirror image.**
`ebpf/fuzz` was a host libFuzzer crate trapped *inside* `ebpf/.cargo/config.toml`'s
bare-metal scope; `heimdall-bpf` was a bare-metal BPF crate sitting *outside* it.
Cargo resolves `.cargo/config.toml` by walking up the tree and ignores the
crate's own `[workspace]`, and an inherited `[build] target` cannot be cancelled
from a child directory — `build-std = []` merges rather than clears. So the fuzz
crate had to move (`git mv` → `crates/lich-fuzz`), and heimdall got its own
config plus the `[profile.dev] lto = true` it was missing.

### And what THAT found: the LICH campaign asserted nothing

Disposition set with Micromanager, implemented with Developer, reviewed by
BlackMage, on Stevie's call to implement real oracles.

All four S21 harnesses had zero call sites for their own verification
functions, and `is_checksum_valid()` was `self.checksum.is_some() || true`.
**"28M executions, zero crashes" meant only "nothing panicked."**

Running them for the first time surfaced four defects in the harnesses:

1. `compact()` popped from **both** ends `len-2` times — emptying the WAL for
   any `len >= 4` — while claiming to keep first and last.
2. compaction dropped entries without folding their values: silent data loss.
3. `verify_seqno_monotonicity` required a dense `0,1,2,…` sequence, which the
   harness's own Phase 5 violates deliberately.
4. the flow-isolation check read back with the **same** `(cache_key, flow_id)`
   the write loop had just used, counting self-reads as violations — it fires
   on almost every input, which is why the harness crashes on its own seeds.

Oracles now use `assert!`, never a returned bool: libFuzzer records an artifact
on abort only, so an oracle whose result the caller ignores is invisible to the
fuzzer. That is exactly how this survived six months.

**BlackMage's scope caveat, recorded so it is not lost:** the crate links no
Unheaded code — the cache, WAL and flow table are models inside the harness
files. A clean run is *design* validation, not evidence about Wotan or the WAL.
`crates/monad-mbc/fuzz` already fuzzes the real decoder, and LICH-008/010 target
Go (`pkg/storage/{cache,wal}`), which Rust libFuzzer can never link. Porting
those two to `go test -fuzz` is the real follow-up.

### Two working-tree traps hit while landing this

- **libFuzzer crash artifacts.** Negative testing wrote `crash-*` reproducers
  into the crate dir. They are byte-identical to the triggering input, so git
  matched them against `seeds/` and reported the seeds as **renamed to the
  artifacts** — staging a deletion of the corpus. Caught before commit; all 120
  seeds verified intact and the artifacts are now gitignored.
- **`check-clippy.sh` reads `git ls-files`,** so mid-`git mv` it lints paths
  from the index that no longer exist on disk and fails transiently. Harmless,
  but do not chase it: re-stage and re-run.

## B5 — rungs 48–69, head `75cfe1d8` — IN STAGING (2026-09-09)

Merged as **`592aad48`**, signed, zero conflicts. 106 files, +2115/−318.

**THE BASELINE IS CLOSED.** `go test ./...` is green for the first time since
`main`.

| | `go vet` | `go test` |
|---|---|---|
| `main` (`0f443ded`) | **exit 1** — aborts on `cmd/wotan-ctl/doom.go:12` | **7 failures** |
| staging after B1–B4 | clean | 6 failures (the pre-existing set) |
| **staging after B5** | clean | **0 failures** |

The two rungs that did it, exactly as the ladder predicted three batches ago:

- `2cc3bd8c` `test(wiki-server): update stale assertions to the behaviour 879c91cf shipped`
- `9fb5166f` `test(dashboard-backend): send the Origin header the upgrade guard requires`

Neither was a product bug. Both were tests asserting behaviour the code had
deliberately moved past — a stale assertion and a missing `Origin` header on a
WebSocket upgrade. Worth noting because for four batches those six failures
were carried as "pre-existing, do not attribute", and it would have been easy
to start treating them as permanent scenery.

| gate | result |
|---|---|
| `go build` / `go vet` | clean |
| `go test ./...` | **0 failures** |
| `check-*.sh` | **7/7 PASS** (six gates + the meta-gate) |
| `check-gates-can-fail.sh` | **6/6 gates still bite** |
| Rust workspaces | 18 / 18 |
| docker images | rebuilt, stack up |
| `qa-smoke.sh` | **33 / 35 — equal to B2, B3 and B4** |

### The meta-gate earned its keep on its first live batch

B5 **modified two gate scripts** — `check-python-syntax.sh` (taught to parse
notebooks, rung `307e9a64`) and `check-timeline-freshness.sh`. Under the old
process that change would have shipped on the strength of "the gate still
prints PASS", which is precisely the evidence that proved worthless four times
running.

Instead the meta-gate re-proved both still FAIL when violated, automatically,
in the same run. No new `check-*.sh` landed in this batch, so the
unregistered-gate trap did not fire.

### Still the only smoke failure: cuirass

Unchanged and unchangeable at this rung — `main` crash-loops identically on the
duplicate `/health` registration. Fix is rung #110 in B8. With container
restart policies now `no` (2026-09-08), it reports `absent` rather than
`restarting`; same root cause, same score.

## The meta-gate — breaking the four-batch cycle (2026-09-09)

Four consecutive batches shipped a gate that was green because it could not
fail. Each was found by hand, by someone happening to poke it, and the fourth
was inside the very script whose own header warns about the pattern. Finding
them one at a time was not converging.

`scripts/check-gates-can-fail.sh` asserts the property instead: for each
`check-*.sh`, plant a violation it claims to catch, require a non-zero exit,
restore the tree. **It would have caught all four on the day they landed.**

| gate | provocation |
|---|---|
| `check-gosec-ratchet` | an un-baselined rule added to the workflow exclusion list |
| `check-manifest-yaml` | a tracked manifest that does not parse |
| `check-secrets-baseline` | a new fingerprint appended to `.gitleaksignore` |
| `check-python-syntax` | a syntax error in a tracked `.py` |
| `check-timeline-freshness` | `MAX_AGE_DAYS=-1`, which nothing can satisfy |
| `check-clippy` | a clippy violation in `crates/upc-api` |

All six bite. Whole run: **15 seconds**.

### The part that keeps it from becoming the same bug one level up

A `check-*.sh` with **no registered provocation is a build failure**, not
reduced coverage. Without that, adding a new unguarded gate would quietly
shrink what is proven while this kept printing PASS — precisely the defect,
one level up. Verified: dropping an unregistered `check-*.sh` into `scripts/`
fails the run.

That check deliberately runs **before** the dirty-tree refusal. It mutates
nothing, and a newly added gate arrives untracked — refusing on a dirty tree
first would hide the one message its author most needs to see.

### Design notes worth keeping

- **Each gate is run clean FIRST.** If it is already red, the provocation
  result is meaningless, so it reports INCONCLUSIVE rather than a false OK.
- **It refuses to run on a dirty tree**, because restore would clobber
  uncommitted work. Restoration is via an `EXIT`/`INT`/`TERM` trap.
- **Git-index cleanup is confined to files the provocation created.** An
  earlier draft ran `git rm --cached` over everything it touched, which would
  have untracked real files such as `.gitleaksignore` — a restore step doing
  more damage than the thing it restored from.
- The gosec provocation has to *introduce* an `-exclude=`, because the list is
  currently empty. That emptiness is the ratchet working, and it is also why
  the B2 guards were unreachable in the first place.

CI: full run on main/develop/staging, `--quick` on PRs (skips only the clippy
provocation, the one step needing the Rust toolchain).

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
