<!--
SPDX-License-Identifier: GPL-3.0-or-later
Copyright (c) 2024-2026 Stevie Bellis.
-->
# S-MOVE-OUT BATTLE PLAN: 25 Phases, 117 Steps

**Date**: 2026-11-04
**Sprint**: S-MOVE-OUT: Doom to `~/tmp/projects/unheaded-doom`, Kubernetes to `~/tmp/projects/unheaded-k8s`, the forge to `~/tmp/projects/unheaded-forge`, history preserved, everything local
**Mode**: unattended, local only, Lane A after S-CLEAN-CUT (`docs/battle-plans/SPRINT-SERIES-2026-11-04.md`)
**Rules**: bound by series rules 1-17 in `docs/battle-plans/SPRINT-SERIES-2026-11-04.md`, and its reserved numbers (ADR-125 to 134 are S-WIRE-HONEST and S-G9-FOLD, S-STAGE-2B starts at 135).
**Answers carried**: DP-305, DP-17, DP-314, and round 4's forge move (with DP-308 GGUF and corpus, DP-608 `answer_start`, DP-402 Q3's 28 tests, all of which travel with it). "Answers recorded" in `DECISION-PACK-2026-11-03.md` override the prompts.
**Prerequisite**: S-CLEAN-CUT, **S-STAGE-2B** and **S-SUPPLY-FIRST** merged to `develop` (see "Order against S-STAGE-2B"; S-SUPPLY-FIRST moves `zhend` and `zhenai-forge` to GPL-3.0-or-later first). `~/tmp/projects/unheaded-doom`, `unheaded-k8s` and `unheaded-forge` do not exist.
**Target**: three local repositories with their history that build on their own; this repo keeps `doom/doom.mbc`, `doom/doom.rv2mbc`, `doom/doom.rv2mbc.sha256` and `doom/LICENSE` as the UPC fixture; upc-regression, upc-api, upc-bootctl and CI pass; `go.mod` loses doomgeneric; no gate, baseline, CI step or log-source row names what left; Doom's lessons stay in `docs/upc/DOOM-LESSONS.md`.
**Estimated Duration**: 5 hours with the swarm table; 9 serial.
**Branch / worktree**: `sprint/s-move-out` in `~/tmp/wt/s-move-out`. The three new repositories are built in place under `~/tmp/projects/`. Merge with `git merge --no-ff -S`. **Stevie creates the GitHub repositories and pushes; nothing here adds a remote.**
**Agent Strategy**: model tag per phase (rule 10), swarm table below, three builders at most (rule 3).
**Commit Cadence**: every [C] through `~/tmp/wt/bin/plan-commit` (from S-CLEAN-CUT Step 3; Step 2 checks it): signed, named files, `[PLAN S-MOVE-OUT] Steps X-Y: ...` with both trailers.
**Stuck Protocol**: rules 8, 12 and 13. Unpredicted output in a Haiku or Sonnet phase is `[STUCK]` at once: record it, go to the next independent step.
**Progress file**: `docs/battle-plans/progress/S-MOVE-OUT-progress.md`, "For Stevie" oldest first.

## GROUNDING: what the tree said on 2026-11-04 (`d6ad43e2`, trial runs in a scratch clone)

| Plan input | What the tree proves | Effect |
|---|---|---|
| History method | `git subtree split` takes one prefix; these moves are many paths. `git filter-branch --index-filter` on a fresh clone took 73 s for 3,500 commits. Doom: 274 commits, 1.9 MB. K8s: 127 commits, 452 KB. | filter-branch, native. It strips GPG signatures: new-repo history is unsigned. |
| Secrets in moved history | `gitleaks git` with the repo's rules: k8s 0, forge 0. **Doom 2, both false positives**: the RFC 6455 sample nonce in `tests/ws1-integration.sh`. **`doomgeneric/DOOM1.WAD` is in doomgeneric's history** (upstream `b4d5f74`, deleted in `63551ae`): a filter removes it before the merge, and a pathspec check cannot see it through an `-s ours` merge, so the check walks objects. | Steps 10, 19, 26 and a final scan of all three tips (Step 116). |
| "The doomgeneric port" | Not in this tree. `doom/doomgeneric/` is gitignored. The port is its own repo `~/tmp/projects/doomgeneric` (origin `unheaded/doomgeneric`, master `c2d7b25`, 120 commits), used through `go.mod`'s `replace`. It has **two uncommitted edits** (`r_data.c`, `w_wad.c`). | Merged in with its history as `doomgeneric/` by the classic subtree merge. `git subtree add` fails here (`invalid path './.gitignore'`); `read-tree --prefix` works (trial: 395 commits total). The edits are not included, and the WAD is filtered out first. |
| DP-305's list | Short of what must move for the tree to stay whole. Also Doom-only: `cmd/doom`, `cmd/doom-bridge`, `cmd/doom-cpu-dump`, `cmd/doom-go-injector`, `cmd/doom-injector-c`, `cmd/test_batch`, `internal/doom`, `internal/bpf` (Go `internal`: cannot be imported across modules), `scripts/doom*`, `scripts/ws3`, `tests/doom-*`, `tests/security/doom`, `dashboard/doom.html`, `docker/services/doom`, `configs/doom.toml`, `nixos/modules/services/doom.nix`, two man pages, two container definitions. | All move. Full list in Step 6. Said in the report to Stevie. |
| `doom/doom.elf` | Not in the keep list, but `scripts/upc-regression.sh` passes it to `doom-runner`. | Moves. The harness takes it from the sibling repo. |
| `docs/doom/` | Ten files are UPC, not Doom. `UPC_BOOT_PROTOCOL_V2.md` is the frozen ABI spec (ADR-067) cited by `cmd/upc-bootctl`, `crates/xv6-mbc`, `ebpf/monad-cpu-ebpf`, ADRs 072, 074, 075, 109 and the RFC desk. | Those ten go to `docs/upc/`, references follow. The new repo keeps a full copy of `docs/doom` from history. |
| `scripts/upc-regression.sh` Variant 2 | Loads the Doom object with `crates/doom-runner`, skipped when the WAD is absent. S-STAGE-2B's gate needs that load. | It takes `doom-runner` and `doom.elf` from the sibling repo and gains an always-on fixture check; Phase 9 proves the load still prints the verified line. |
| CI coverage of the fixture | `crates/monad-mbc` `step106_doom_mbc_smoke_test` looks at `doom/doomgeneric/doom.mbc`, a path that has never existed in a checkout, so it always skips. | Pointed at `doom/doom.mbc`: the fixture gets real CI coverage (Phase 9). |
| `cmd/wotan-ctl/doom.go`, `load_mem.go` | Package `main`: they need `Command`, `Context`, `BPFMap`, `CpuState` from sibling files. Removing them leaves `wotan-ctl` green; `go.mod` then drops doomgeneric (trial). | New repo gets `cmd/doom-ctl` = the two files plus copies of the other `cmd/wotan-ctl` sources. |
| `cmd/dashboard-backend/.../doom.go` | Not in DP-305's list. Serves `doom-viewport.js`, which leaves; `session_test.go` uses `/api/v1/doom/input` as its sample write route. | Handlers and state go (kept in the new repo under `reference/`); the test moves to another route. |
| `dashboard/js/doom-viewport.js` | Already an orphan: no page loads it (REAL-STEEL row PARK). | Row closed. |
| `pkg/ports` Doom names, "Doom Range" | The name of the port band, used everywhere, not Doom code. `doom-security` CI job is the port-range check. | Stay. |
| `monad-cpu-ebpf` `doom-debug`, Doom build variant, `upc-api` Doom surface | UPC code. | Stay. |
| DP-314 | `vor` indexes markdown under `~/.config/cs/sources/unheaded` (a symlink to the repo) and has no exclude list. The code index of ADR-057 is S-ZHEN-POLISH's, and its filter excludes any path with `doom`. | Moving `docs/doom` out removes it from vor. ADR-057's out-of-scope list gains Doom; `raft/scripts/distill_qa.py` loses `doom_docs`. |
| `helm`, `kubernetes`, `deploy/k8s`, `deploy/argocd` | 163 files. Tied to this repo by: `pkg/uids` (`TestManifestsMatchRegistry`, `helm_test.go`), gates `check-helm-security`, `check-k8s-well-initdb` and `render-k8s-initdb.py`, the meta-gate's `provoke_manifest_yaml` probe path, `security.yml`, `plan.yml`, `.trivyignore.yaml`, 278 trivy baseline rows, `image-tag-baseline`, `pq-kex-baseline`, helm and kubeconform pins. | All follow. The new repo copies `scripts/ci-install-tools.sh`, `scripts/lib/tool-pins.sh`, `scripts/lib/fetch-tool.sh` so its CI keeps working. |
| `pkg/iac/kubernetes.go` | The IaC renderer that writes manifests for users. Not the repo's own K8s material. | Stays; CLAUDE.md's backend table stays true. |
| `kubernetes/manifests/overlays/gpu-vllm` | DP-713 left it for this plan. | Moves to the lab repo; `pkg/uids` `VLLM` becomes a reserved number. |
| Forge: what moves | `crates/zhenai-forge` (kernels, notes, 111 tests plus the 28 ignored); `cmd/zhen-flywheel` with `man/zhen-flywheel.1` (round 5); scripts `zlora-to-gguf.py`, `build-coding-corpus.py`, `tokenize-kingdom-for-gemma4.py`, `tokenize-coding-for-gemma4.py`, `forge-train.sh`, `forge-watchdog.sh`, `gemma4-*.py`, `auto-distill.sh`, `wave14-*.py`, `scripts/lib/gpulease_forge_systemd_test.go`, `scripts/two-brains/test_export_roundtrip.py`; `raft/kingdom-w12` (six LoRA files, 22.6 MB each, the bulk of the 101 MB history), `raft/experiments`, every numbered `raft/scripts` file, `add-source.sh`, `ab_test_checkpoint.sh`, `distill_qa.py`, `raft/ADD_SOURCES.md`; `runbooks/data/raft-training-pipeline.yaml`, `corpus-rebuild.yaml`, `zhen-index-rebuild.yaml`. | Phases 4A, 12A. |
| Forge: what stays | **Nothing running needs what moves**: `zhen-flywheel` has no unit, uid, port or `sources.yaml` row; captures are off; no serving code imports a numbered raft script. `raft/zhen_app.py`, `zhen_rag.py`, `zhen_prompt_generated.py`, `action_manager.py`, `raft/tests` stay. **`pkg/zhencapture` stays**: `cmd/zhen-claude`, `cmd/zhen-mcp`, `cmd/well-retention` and `pkg/claudebridge` import it. `pkg/gpulease` (zhen-agentd), `scripts/lib/gpu-lease.sh`. Migration 033 and the capture tables stay: a migration is never removed. Champion's audit trail (`llm_requests`, `zhen_actions`, Anamnesis) stays. **No `eval/` directory is training-only**: `eval/coding-gate`, `two-brains`, `champion-runbooks` are Zhen's quality gates; the last still names the moved runbooks in frozen prompts, left as they are. ADRs and WAVE plans are copied. | Nothing to do here. |
| Forge: ripple | No workflow, Makefile target, unit, uid, port or `sources.yaml` row names it. It has no path dependency and no root Cargo workspace holds it. `scripts/check-clippy.sh` exports `ZHENAI_FORGE_SKIP_KERNELS`; `scripts/ci-local.sh` and `scripts/kingdom-health.sh` list it; `.github/dependabot.yml`, cargo audit, clippy and SBOM discover it by glob, so they follow on their own. `scripts/lib/gpulease_scripts_test.go` runs `forge-train.sh`. `scripts/well/test-zhen-captures.sh --with-ingest` tests `cmd/zhen-flywheel`'s store; the meta-gate runs that script without the flag. `raft/zhen_app.py:726` reads the forge log for a status line inside `try`: harmless with no log, left alone since that UI is retiring (ADR-114 Q7). | Phases 4B, 10A, 15A, 16, 17. |
| Forge: facts for its README | Trial: `cargo test --lib` 111 passed, 28 ignored, with real HIP kernels; `ZHENAI_FORGE_SKIP_KERNELS=1` cannot link the tests. `cmd/zhen-flywheel` builds and tests there against `../../unheaded`. **Gemma-4 base is not on west**: `/var/zhen/models` has no Gemma file and `~/tmp/gemma-4-E2B-it` holds config and tokenizer only. **Scrambled LoRAs confirmed in the code**: `zlora-to-gguf.py` writes the forge's row-major A (n_embd x rank) and B (rank x n_embd) under GGUF dims whose fastest axis is the other one, with no transpose, and truncates K/V columns. | Phase 4B. |

**Worktree trap (rule 1).** The `~/tmp/wt/projects` symlink serves until Phase 8 removes the `replace`.

**Rule 13 in this plan.** The history split is Opus (Phases 1 and 3): filter-branch, the subtree merge and anything that deletes under `~/tmp/projects`. The two sudo harnesses are Opus (Phase 9). `kingdom-shutdown.sh`, `ci-local.sh`, `clean-artifacts.sh` and the Nix firewall line are Opus (Phase 10A). The moved runner unit is edited in Phase 1. Phases 4A and 4B are Opus. The live harness and the load gate are named [S] steps. No step uses `pkill -f`; S-STAGE-2B Step 6 took it out of the harness. Scratch is `/var/tmp/s-move-out-*`. Forgers ran nothing as root; the live harness has not been rehearsed.

**Order against S-STAGE-2B (decided here).** S-STAGE-2B's gate runs `scripts/upc-regression.sh`, starts `doom-runner`, and goes red if the harness skips Doom or stops printing `verified 758199 insns`. Two things follow, and this plan does both:

1. **S-MOVE-OUT lands after S-STAGE-2B.** Its gate builds `crates/doom-runner` in its own worktree, and its Step 6 rewrites the same harness lines (`doom_stop`, the recorded pid). Phase 9's exact text is written against that post-2B harness and was dry-run on it. Phase 0 refuses to start without the merge.
2. **The harness keeps running the real Doom load**, through a prebuilt runner from the new repo by path: `DOOM_RUNNER` (default `~/tmp/projects/unheaded-doom/crates/doom-runner/target/release/doom-runner`) and `DOOM_ELF`, with the object and fixture from this repo (`doom-runner run` resolves `ebpf/target/...` from the working directory, and the harness runs from `$ROOT`). `upc-api` stays a path dependency, so the runner tracks ABI changes when rebuilt. Phase 9 builds it and requires the line. The harness fails on a skip when `UPC_REQUIRE_DOOM=1`; without that flag a skip exits 0, so any gate that needs Doom sets it.

**Other gates that needed Doom bits (checked).** `internal/bpf/bpf_test.go` moves with its package and passes there; no workflow or `check-*` script names it. `cmd/upc-bootctl/src/runner.rs` and `netns.rs` only mention doom-runner in comments ("lifted from", "mirrors"): `upc-bootctl` builds and its tests pass without the crate. `scripts/ci-local.sh` has `crates/doom-runner` in its list: Phase 10A. Workflows: twelve fetch steps across five files, Phase 8. `doom-runner`'s own tests read `../../doom/doom.rv2mbc` and `include!` `ebpf/monad-cpu-ebpf/surface.rs`: the new repo gets its own copy of the fixture and a sibling path (Phases 1, 2).

**S-ZHEN-POLISH.** It removes nothing of the forge and, as reviewed at `3f633578`, has no step that needs a forge file: its only mention is `crates/zhenai-forge/...` as literal paths in a code-index filter test, which pass with or without the crate.

**Overlap.** Wave 2 runs beside S-G9-FOLD and S-LOG-HARVEST Phase 5. `config/logging/sources.yaml` is touched by the latter: three deletions here merge cleanly, but whoever merges second re-runs `scripts/check-log-sources.sh`.

## LEGEND

[B] bash · [V] verify (must pass) · [D] debug (only on failure) · [W] write/edit file · [R] read · [S] sudo · [P] parallelizable · [C] commit checkpoint · [STUCK]/[BLOCKED]

**Standing rules for every step.**
- Bash blocks run from the directory their first line names. `$WT` means `~/tmp/wt/s-move-out`. Go commands run with `export GOMAXPROCS=4 GOTMPDIR=/var/tmp`.
- `sub.py` success output is `sub.py: <file>: N block(s) applied`. `plan-commit` output ends in `committed <hash>`.
- **Any other output** than a step names, in a Haiku or Sonnet phase: mark `[STUCK]`, paste it under that step number in the progress file, go to the next independent step.
- Never run `git add`, `git rm` or `git mv` in the worktree by hand: use `rm`, `mv`, and list both paths at commit. (Inside the three new repositories one agent works alone, so plain git is fine there.)

## SWARM TABLE

| Wave | Phases at once | Model | Builder? | Owns |
|---|---|---|---|---|
| 0 | 0 | Haiku | no | worktree |
| 1 | 1 then 2 | Opus, then Sonnet | yes (Phase 2) | `~/tmp/projects/unheaded-doom` |
| 1 | 3 then 4 | Opus, then Sonnet | no | `~/tmp/projects/unheaded-k8s` |
| 1 | 4A then 4B | Opus | yes (4B) | `~/tmp/projects/unheaded-forge` |
| 1 | 5 | Haiku | no | `docs/upc`, reference rewrites |
| 2 (after Phases 1 and 3 pass their leak scans) | 6 | Haiku | no | Doom paths in this repo |
| 2 | 12 | Haiku | no | K8s paths in this repo |
| 3 | 7 | Sonnet | yes | `cmd/wotan-ctl`, `cmd/dashboard-backend` |
| 3 | 8 | Haiku | no | `go.mod`, `.github/workflows/{ci,gpl-boundary,sbom-audit,security}.yml` |
| 3 | 9 | Opus | yes | `scripts/upc-regression.sh`, `scripts/ascend-linux-smoke.sh` |
| 3 | 9A (after 7) | Sonnet | yes | `crates/monad-mbc/tests` |
| 3 | 10 | Haiku | no | logging, units, baselines (Doom rows) |
| 3 | 10A (after 10: shared manifest) | Opus | no | root scripts, runbooks, deletion list, Nix host, Doom residue |
| 3 (after 10A: shared manifest) | 12A | Haiku | no | forge paths in this repo |
| 3 | 13 | Sonnet | yes | `pkg/uids` |
| 3 | 14 | Sonnet | no | gates and pins, `.trivyignore.yaml`, K8s baseline rows |
| 4 | 11 | Opus | no | `docs/upc/DOOM-LESSONS.md` |
| 4 | 15 | Haiku | no | K8s docs, ADR notes |
| 4 | 15A | Haiku | no | forge gates and docs |
| 5 | 16 | Haiku, one agent | no | shared ledgers: `CLAUDE.md`, `README.md`, `THIRD_PARTY.md`, `Makefile`, wiki, `docs/LIVE-PATHS.md` |
| 6 | 17, then 18 | Sonnet, then Opus | yes | regenerate, exit, merge |

Phases 8 and 14 both edit `.github/workflows/security.yml`: 8 first, 14 after its commit. Wave 3 has at most Phases 7, 9 and 13 building at once; 9A waits for 7. S-G9-FOLD and S-LOG-HARVEST count against the same three slots (rule 3): when they are building, run these one at a time. Phases 10 and 14 share `trivy-misconfig-baseline.tsv` and `image-tag-baseline.txt`, and 15 and 15A share `wiki/Project-Structure.md`: each pair runs one after the other.

---

## PHASE 0: PREFLIGHT
**Goal**: worktree, helper check, baseline. **Prerequisite**: S-CLEAN-CUT, S-STAGE-2B and S-SUPPLY-FIRST merged. **Time**: 10m. **Agent**: one. **Model**: Haiku

- [ ] **Step 1** [B] ~1m: `cd ~/tmp/unheaded && git log --oneline develop | grep -c 'PLAN S-CLEAN-CUT' && grep -c doom_stop scripts/upc-regression.sh && grep -c 'GPL-3.0-or-later' crates/zhenai-forge/Cargo.toml && git status --short | grep -v '^??' | wc -l && git worktree add ~/tmp/wt/s-move-out -b sprint/s-move-out develop | tail -1` Expect three numbers above `0`, then `0`, `HEAD is now at` plus a hash. Any of the first three at `0`: `[BLOCKED]`: S-CLEAN-CUT, S-STAGE-2B or S-SUPPLY-FIRST has not merged.
- [ ] **Step 2** [V] ~1m: `test -x ~/tmp/wt/bin/sub.py && test -x ~/tmp/wt/bin/plan-commit && test -d ~/tmp/wt/projects/doomgeneric && test ! -e ~/tmp/projects/unheaded-doom && test ! -e ~/tmp/projects/unheaded-k8s && test ! -e ~/tmp/projects/unheaded-forge && echo READY` Expect `READY`. Missing helpers: run S-CLEAN-CUT Steps 2 and 3 again.
- [ ] **Step 3** [B] ~4m: `cd ~/tmp/wt/s-move-out && export GOMAXPROCS=4 GOTMPDIR=/var/tmp && go build ./... && go test ./pkg/uids/ ./cmd/wotan-ctl/ 2>&1 | tail -2 | cut -c1-40` Expect two lines starting `ok`.
- [ ] **Step 4** [W] ~1m:
```bash
cd ~/tmp/wt/s-move-out && cat > docs/battle-plans/progress/S-MOVE-OUT-progress.md <<'MD'
<!--
SPDX-License-Identifier: GPL-3.0-or-later
Copyright (c) 2024-2026 Stevie Bellis.
-->
# S-MOVE-OUT progress

## For Stevie (oldest first)

## Stuck steps

## Evidence
MD
grep -c '^## ' docs/battle-plans/progress/S-MOVE-OUT-progress.md
```
  Expect `3`.
- [ ] **Step 5** [C] ~1m: **PHASE 0 EXIT GATE + COMMIT.** `~/tmp/wt/bin/plan-commit s-move-out S-MOVE-OUT "Steps 1-5: worktree and progress file" docs/battle-plans/progress/S-MOVE-OUT-progress.md`

## PHASE 1: build unheaded-doom with its history (DP-305)
**Goal**: a local repository holding every Doom path with its history, plus doomgeneric. Opus: this rewrites history (rule 13). **Prerequisite**: Phase 0. **Time**: 15m. **Agent**: Doom repo lane. **Model**: Opus

- [ ] **Step 6** [W] ~1m: The path list.
```bash
cat > ~/tmp/wt/doom-paths.txt <<'TXT'
crates/doom-runner
demos/doom
docs/doom
runbooks/doom
dashboard/js/doom-viewport.js
dashboard/doom.html
deploy/systemd/unheaded-doom-injector.service
deploy/systemd/unheaded-doom-ring.service
deploy/systemd/unheaded-doom-runner.service
deploy/systemd/unheaded-doom.target
cmd/doom
cmd/doom-bridge
cmd/doom-cpu-dump
cmd/doom-go-injector
cmd/doom-injector-c
cmd/doom-loader
cmd/test_batch
cmd/wotan-ctl/doom.go
cmd/wotan-ctl/load_mem.go
cmd/dashboard-backend/internal/server/doom.go
cmd/dashboard-backend/internal/server/doom_test.go
internal/doom
internal/bpf
scripts/doom
scripts/ws3
scripts/doom-build-check.sh
scripts/doom-capture-reference.py
scripts/doom-cpu-dump.py
scripts/doom-cpu-monitor.sh
scripts/doom-loader-core.py
scripts/doom-loader.sh
scripts/doom-ring.sh
scripts/doom-screen-check.py
scripts/doom-smoke.sh
scripts/doom-test.sh
scripts/doom-tick.py
scripts/doom-xdp-attach.sh
tests/doom-memwrite
tests/doom-reference
tests/security/doom
tests/ws1-integration.sh
docker/services/doom
configs/doom.toml
nixos/modules/services/doom.nix
man/doom-bridge.1
man/doom-go-injector.1
containers/lxd/instances/doom-bridge.yaml
containers/containerd/services/doom-bridge.json
doom/doom.elf
doom/LICENSE
doom/doom.mbc
doom/doom.rv2mbc
doom/doom.rv2mbc.sha256
TXT
wc -l < ~/tmp/wt/doom-paths.txt
```
  Expect `53`. The last four are copies: they also stay here (Phase 6).
- [ ] **Step 7** [B] ~3m: Clone and filter. `develop` only; the clone has no link back.
```bash
set -euo pipefail
cd ~/tmp/projects && git clone -q --no-local --single-branch --branch develop ~/tmp/unheaded unheaded-doom && cd unheaded-doom && git remote remove origin
[ "$(basename "$(git rev-parse --show-toplevel)")" = unheaded-doom ]
[ -z "$(git remote)" ]
git rev-parse HEAD > ~/tmp/wt/unheaded-doom.source
export FILTER_BRANCH_SQUELCH_WARNING=1
P=$(tr '\n' ' ' < ~/tmp/wt/doom-paths.txt)
git filter-branch -f --prune-empty --index-filter "git read-tree --empty; git reset -q \$GIT_COMMIT -- $P 2>/dev/null; true" -- develop 2>&1 | tail -1
rm -rf .git/refs/original && git reflog expire --expire=now --all && git gc -q --prune=now
git branch -m develop main && git rev-list --count main && git ls-files | wc -l
```
  Expect `Ref 'refs/heads/develop' was rewritten`, a commit count near `274`, a file count near `173`. The two guard lines stop the block unless it stands in the new clone, so `filter-branch` and `gc --prune=now` cannot reach the main repo. Keep them as two plain lines: `set -e` does not stop on a failed test inside an `&&` list.
- [ ] **Step 8** [B] ~2m: Merge doomgeneric with its history, without the WAD. The filter works on a scratch clone, so `~/tmp/projects/doomgeneric` is not touched.
```bash
set -euo pipefail
git -C ~/tmp/projects/doomgeneric status --short | wc -l
git clone -q --no-hardlinks ~/tmp/projects/doomgeneric /var/tmp/s-move-out-dg && cd /var/tmp/s-move-out-dg && git remote remove origin
[ "$(pwd)" = /var/tmp/s-move-out-dg ]
[ -z "$(git remote)" ]
FILTER_BRANCH_SQUELCH_WARNING=1 git filter-branch -f --index-filter 'git rm -q --cached --ignore-unmatch doomgeneric/DOOM1.WAD' -- master 2>&1 | tail -1
cd ~/tmp/projects/unheaded-doom && git config user.name 'Stevie Bellis' && git config user.email stevie@bellis.tech
git remote add doomgeneric /var/tmp/s-move-out-dg && git fetch -q doomgeneric master && git merge -q -s ours --no-commit --allow-unrelated-histories doomgeneric/master; git read-tree --prefix=doomgeneric/ -u doomgeneric/master && git commit -q --no-gpg-sign -m 'Merge doomgeneric with its history (subtree merge, WAD filtered out)' && git remote remove doomgeneric
ls doomgeneric/unheaded/go.mod && git rev-list --count main
```
  Expect `2` (the two uncommitted edits, left where they are), `Ref 'refs/heads/master' was rewritten`, the path, a count near `395`. Then append to "For Stevie": `~/tmp/projects/doomgeneric has uncommitted edits to doomgeneric/r_data.c and doomgeneric/w_wad.c; they are not in unheaded-doom. DOOM1.WAD stays out of unheaded-doom but remains in upstream doomgeneric history.`
- [ ] **Step 9** [W] ~2m: The moved units run privileged, so their paths change here (rule 13), not in Phase 2. No unit is installed on west. The runner keeps its working directory: `doom-runner run` resolves `ebpf/target/...` from it, and the object stays in `unheaded`. The old unit pointed at `demos/doom/doom.mbc`, a path that never held it: the three files are under `doom/`.
```bash
cd ~/tmp/projects/unheaded-doom && sed -i 's#/home/govan/tmp/unheaded/#/home/govan/tmp/projects/unheaded-doom/#g; s#^WorkingDirectory=/home/govan/tmp/unheaded$#WorkingDirectory=/home/govan/tmp/projects/unheaded-doom#; s#unheaded-doom/demos/doom/doom\.#unheaded-doom/doom/doom.#' deploy/systemd/unheaded-doom-ring.service deploy/systemd/unheaded-doom-injector.service deploy/systemd/unheaded-doom-runner.service deploy/systemd/unheaded-doom.target
sed -i 's#^WorkingDirectory=/home/govan/tmp/projects/unheaded-doom$#WorkingDirectory=/home/govan/tmp/unheaded#' deploy/systemd/unheaded-doom-runner.service
git grep -c '/home/govan/tmp/unheaded/' -- deploy/systemd; git add deploy/systemd/unheaded-doom-ring.service deploy/systemd/unheaded-doom-injector.service deploy/systemd/unheaded-doom-runner.service deploy/systemd/unheaded-doom.target && git commit -q --no-gpg-sign -m 'Units point at this repository' && echo COMMITTED
```
  Expect `COMMITTED`, and no count line from the `grep`.
- [ ] **Step 10** [V] ~2m: Secrets and game data, now over everything that will ship, with the repo's own rules. Both expected findings are the RFC 6455 sample nonce.
```bash
cd ~/tmp/projects/unheaded-doom && gitleaks git . --no-banner --redact --config ~/tmp/unheaded/.gitleaks.toml -f json -r /var/tmp/s-move-out-doom-leaks.json -l error
grep -c 'dGhlIHNhbXBsZSBub25jZQ==' tests/ws1-integration.sh
python3 - <<'PY'
import json
d=json.load(open('/var/tmp/s-move-out-doom-leaks.json'))
files={x['File'] for x in d}
print('leaks',len(d),sorted(files))
assert files<= {'tests/ws1-integration.sh'}
open('.gitleaksignore','w').write(''.join(x['Fingerprint']+'\n' for x in d))
PY
git add .gitleaksignore && git commit -q --no-gpg-sign -m 'gitleaks: the RFC 6455 sample nonce is not a secret'
gitleaks git . --no-banner --redact --config ~/tmp/unheaded/.gitleaks.toml -f json -r /var/tmp/s-move-out-doom-leaks.json -l error; python3 -c "import json;print('leaks',len(json.load(open('/var/tmp/s-move-out-doom-leaks.json'))))"; git rev-list --objects --all | grep -c -i '\.wad$'; echo rc=$?
```
  Expect a count above `0`, `leaks 2 ['tests/ws1-integration.sh']`, `leaks 0`, `0`, `rc=1`. A finding in any other file, or any `.wad`: `[BLOCKED]`. Do not go to Phase 6; put file names (never values) under "For Stevie".
- [ ] **Step 11** [V] ~1m: **PHASE 1 EXIT GATE.** `cd ~/tmp/projects/unheaded-doom && git remote | wc -l && git status --short | wc -l && git log --format='%an' main | sort -u | head -5` Expect `0`, `0`, author names. No commit to this repo's sprint branch.

## PHASE 2: unheaded-doom builds on its own (DP-305)
**Goal**: `go build ./...`, `go test ./...` and `cargo check` pass in the new repository against a sibling `~/tmp/unheaded`. **Layout**: both repos are checked out as siblings of `tmp/`: `unheaded-doom` at `~/tmp/projects/`, this repo at `~/tmp/unheaded`, so the relative path is `../../unheaded`.
**Prerequisite**: Phase 1. **Time**: 30m. **Agent**: Doom repo lane. **Model**: Sonnet

- [ ] **Step 12** [W] ~3m: Layout. `reference/` holds the dashboard endpoints that no longer have a server to live in.
```bash
cd ~/tmp/projects/unheaded-doom && mkdir -p reference && git mv cmd/dashboard-backend/internal/server/doom.go reference/dashboard-doom.go.txt && git mv cmd/dashboard-backend/internal/server/doom_test.go reference/dashboard-doom_test.go.txt
git mv cmd/wotan-ctl cmd/doom-ctl
for f in ~/tmp/unheaded/cmd/wotan-ctl/*.go; do b=$(basename "$f"); [ -e "cmd/doom-ctl/$b" ] || cp "$f" cmd/doom-ctl/; done
rmdir cmd/dashboard-backend/internal/server cmd/dashboard-backend/internal cmd/dashboard-backend 2>/dev/null
ls cmd/doom-ctl | wc -l; ls cmd/dashboard-backend 2>/dev/null | wc -l
```
  Expect `8` (`boot.go`, `bpf_maps.go`, `doom.go`, `load_mem.go`, `load_rom.go`, `load_rom_test.go`, `main.go`, `mkfs.go`), `0`.
- [ ] **Step 13** [W] ~2m: Module.
```bash
cd ~/tmp/projects/unheaded-doom && cat > go.mod <<'MOD'
module unheaded-doom

go 1.26.0

toolchain go1.27.3

require (
	github.com/unheaded/doomgeneric v0.0.0
	unheaded v0.0.0
)

replace (
	github.com/unheaded/doomgeneric => ./doomgeneric/unheaded
	unheaded => ../../unheaded
)
MOD
git grep -l '"unheaded/internal/' -- '*.go' | xargs sed -i 's#"unheaded/internal/#"unheaded-doom/internal/#'
sed -i 's#upc-api = { path = "../upc-api" }#upc-api = { path = "../../../../unheaded/crates/upc-api" }#' crates/doom-runner/Cargo.toml
sed -i 's#include!("../../../ebpf/monad-cpu-ebpf/surface.rs");#include!("../../../../../unheaded/ebpf/monad-cpu-ebpf/surface.rs");#' crates/doom-runner/src/main.rs
git grep -c '"unheaded/internal/' -- '*.go'; echo rc=$?
```
  Expect `rc=1`.
- [ ] **Step 14** [V] ~6m: Green, all of it.
```bash
cd ~/tmp/projects/unheaded-doom && export GOMAXPROCS=4 GOTMPDIR=/var/tmp GOFLAGS=-mod=mod GOWORK=off && go mod tidy && go build ./... && go test ./... 2>&1 | grep -c -v -E '^(ok|\?)'; (cd crates/doom-runner && CARGO_TARGET_DIR=/var/tmp/s-move-out-doom cargo test --locked -j4 2>&1 | grep -E '^test result|^error' | head -3)
```
  Expect `0` and `test result: ok`. A `go` failure in `cmd/doom-ctl`: run `go build ./cmd/doom-ctl 2>&1 | head`; if it names a type in another `cmd/wotan-ctl` file that the copy lacks, that is `[STUCK]`.
- [ ] **Step 15** [W] ~3m: The repo's front door.
```bash
cd ~/tmp/projects/unheaded-doom && cp ~/tmp/unheaded/LICENSE LICENSE && printf 'target/\n__pycache__/\n*.wad\n*.WAD\n' > .gitignore
cat > README.md <<'MD'
# unheaded-doom

Doom on the Unheaded Protocol Computer: id's engine compiled to MBC bytecode and run by the UPC interpreter in eBPF.

Moved out of `unheaded/unheaded` on 2026-11-04 with its history (DP-305). The UPC itself stays there, along with a fixture of `doom.mbc`, `doom.rv2mbc` and its sha256 for its regression harness.

## Layout

| Path | What |
|---|---|
| `crates/doom-runner` | Aya runtime: loads the object, ROM and WAD, attaches XDP, runs the ring |
| `cmd/doom-ctl` | operator tool; `doom` and `load-mem` came from `wotan-ctl`, the rest is a copy of it |
| `cmd/doom-*`, `internal/` | loaders, bridge, injectors |
| `demos/doom`, `doomgeneric/` | the port and the C sources (GPL-2.0, see `doom/LICENSE`) |
| `docs/doom`, `runbooks/doom` | everything learned, and how to run it |
| `reference/` | the dashboard REST endpoints, kept as text |

## Building

This repository expects `unheaded` checked out at `../../unheaded` relative to it: `go.mod` and `crates/doom-runner/Cargo.toml` use that path for `unheaded/pkg/*` and `crates/upc-api`. So it cannot build on GitHub Actions as it stands (`actions/checkout` cannot write outside the workspace), and no workflow came with it. A `upc-api` change can break `doom-runner` with no gate: rebuild it after one. `doom/doom.elf` here must match the `doom.mbc` and `doom.rv2mbc` fixture in `unheaded`.

    go build ./...
    cd crates/doom-runner && cargo build --release

WAD files are not in this repository.
MD
grep -c '^# unheaded-doom' README.md
```
  Expect `1`.
- [ ] **Step 16** [C] ~1m: **PHASE 2 EXIT GATE + COMMIT (new repo).** `cd ~/tmp/projects/unheaded-doom && git add .gitignore go.mod go.sum README.md LICENSE crates/doom-runner/Cargo.toml crates/doom-runner/Cargo.lock crates/doom-runner/src/main.rs cmd/doom-ctl $(git grep -l '"unheaded-doom/internal/' -- '*.go') reference && git commit -q --no-gpg-sign -m 'Stand alone: module, sibling paths, doom-ctl, README' && git status --short | wc -l` Expect `0`. This repo's commits are unsigned by design: Stevie signs on his own push if he wants.

## PHASE 3: build unheaded-k8s with its history (DP-17)
**Goal**: same as Phase 1. Opus: this rewrites history (rule 13). **Prerequisite**: Phase 0. **Time**: 5m. **Agent**: K8s repo lane. **Model**: Opus

- [ ] **Step 17** [W] ~1m:
```bash
cat > ~/tmp/wt/k8s-paths.txt <<'TXT'
kubernetes
helm
deploy/k8s
deploy/argocd
.github/workflows/helm.yml
scripts/check-helm-security.sh
scripts/check-k8s-well-initdb.sh
scripts/well/render-k8s-initdb.py
scripts/k8s-lab-up.sh
scripts/k8s-lab-down.sh
scripts/rollback.sh
scripts/ci-install-tools.sh
scripts/lib/tool-pins.sh
scripts/lib/fetch-tool.sh
docs/labs/k8s-kingdom-lab.md
docs/labs/k8s-kingdom-lab-plan.md
docs/labs/k8s-kingdom-lab-sessions.md
eval/k8s-bringup/2026-05-05/RESULTS.md
TXT
wc -l < ~/tmp/wt/k8s-paths.txt
```
  Expect `18`. `scripts/rollback.sh` is an `argocd` wrapper: nothing else calls it.
- [ ] **Step 18** [B] ~3m:
```bash
set -euo pipefail
cd ~/tmp/projects && git clone -q --no-local --single-branch --branch develop ~/tmp/unheaded unheaded-k8s && cd unheaded-k8s && git remote remove origin
[ "$(basename "$(git rev-parse --show-toplevel)")" = unheaded-k8s ]
[ -z "$(git remote)" ]
git rev-parse HEAD > ~/tmp/wt/unheaded-k8s.source
export FILTER_BRANCH_SQUELCH_WARNING=1
P=$(tr '\n' ' ' < ~/tmp/wt/k8s-paths.txt)
git filter-branch -f --prune-empty --index-filter "git read-tree --empty; git reset -q \$GIT_COMMIT -- $P 2>/dev/null; true" -- develop 2>&1 | tail -1
rm -rf .git/refs/original && git reflog expire --expire=now --all && git gc -q --prune=now
git branch -m develop main && git config user.name 'Stevie Bellis' && git config user.email stevie@bellis.tech && git ls-files | sed 's#/.*##' | sort -u | tr '\n' ' '
```
  Expect `Ref 'refs/heads/develop' was rewritten`, then `.github deploy docs eval helm kubernetes scripts`.
- [ ] **Step 19** [V] ~1m: **PHASE 3 EXIT GATE.** `cd ~/tmp/projects/unheaded-k8s && gitleaks git . --no-banner --redact --config ~/tmp/unheaded/.gitleaks.toml -f json -r /var/tmp/s-move-out-k8s-leaks.json -l error; python3 -c "import json;print('leaks',len(json.load(open('/var/tmp/s-move-out-k8s-leaks.json'))))"; git remote | wc -l` Expect `leaks 0`, `0`. A finding: `[BLOCKED]`, file names only under "For Stevie", no Phase 12.

## PHASE 4: unheaded-k8s stands alone (DP-17)
**Goal**: its gates and CI work without this repo beside them, except the initdb render, which reads `db/` from a sibling. **Prerequisite**: Phase 3. **Time**: 15m. **Agent**: K8s repo lane. **Model**: Sonnet

- [ ] **Step 20** [W] ~3m: The render script reads `db/` from `UNHEADED_REPO`, default the sibling. Only `MIG` and the two `db/` reads move; the outputs stay in this repo.
```bash
cd ~/tmp/projects/unheaded-k8s && python3 ~/tmp/wt/bin/sub.py scripts/well/render-k8s-initdb.py <<'SUB'
<<<<
import pathlib
import sys

REPO = pathlib.Path(__file__).resolve().parents[2]
MIG = REPO / "db" / "migrations"
====
import os
import pathlib
import sys

REPO = pathlib.Path(__file__).resolve().parents[2]
# db/ lives in the main repository, a sibling checkout by default.
MAIN = pathlib.Path(os.environ.get("UNHEADED_REPO", REPO.parent.parent / "unheaded"))
MIG = MAIN / "db" / "migrations"
>>>>
SUB
sed -i 's#REPO / "db"#MAIN / "db"#g' scripts/well/render-k8s-initdb.py
grep -c 'REPO / "db"' scripts/well/render-k8s-initdb.py; echo rc=$?
```
  Expect the `sub.py` line, `0`, `rc=1`.
- [ ] **Step 21** [V] ~1m: `cd ~/tmp/projects/unheaded-k8s && python3 scripts/well/render-k8s-initdb.py --check; echo rc=$?` Expect `rc=0`. `rc=1` means the ConfigMap is stale against `db/`: run it without `--check` once and keep the result.
- [ ] **Step 22** [W] ~3m: The front door.
```bash
cd ~/tmp/projects/unheaded-k8s && cp ~/tmp/unheaded/LICENSE LICENSE && printf 'target/\n__pycache__/\n*.wad\n*.WAD\n' > .gitignore && cat > README.md <<'MD'
# unheaded-k8s

Kubernetes material for Unheaded: manifests, a Helm chart, ArgoCD apps and a kind lab. **A lab component, not a product target** (DP-17): the platform scales from one host to many without it. No cluster runs anywhere; ADR-055 keeps k3s off west and east.

Moved out of `unheaded/unheaded` on 2026-11-04 with its history.

- `kubernetes/manifests/`, `deploy/k8s/`, `deploy/argocd/`: manifests and apps
- `helm/unheaded/`: the chart
- `scripts/check-helm-security.sh`: every rendered container runs read-only
- `scripts/well/render-k8s-initdb.py`: renders the Well's ConfigMap from `db/` in a sibling `unheaded` checkout (`UNHEADED_REPO` overrides)
- `docs/labs/`: the lab plan

`kubernetes/manifests/overlays/gpu-vllm` is parked with vLLM (DP-713): it needs a better GPU.

uid numbers come from `pkg/uids` in the main repository; the test that held manifests to it stayed behind when these files left.
MD
bash -n scripts/check-helm-security.sh && bash -n scripts/ci-install-tools.sh && git add .gitignore LICENSE README.md scripts/well/render-k8s-initdb.py kubernetes/manifests/base/the-well/configmap-initdb.yaml kubernetes/manifests/base/the-well/configmap-config.yaml && git commit -q --no-gpg-sign -m 'Stand alone: README, licence, render reads db/ from the sibling' && git status --short | wc -l
```
  Expect `0`.
- [ ] **Step 23** [V] ~1m: **PHASE 4 EXIT GATE.** `cd ~/tmp/projects/unheaded-k8s && python3 scripts/well/render-k8s-initdb.py --check; echo rc=$?; git status --short | wc -l; git remote | wc -l` Expect `rc=0`, `0`, `0`.

## PHASE 4A: build unheaded-forge with its history (round 4)
**Goal**: every forge and training path with its history, in a local repository. Opus: this rewrites history (rule 13). **Prerequisite**: Phase 0. **Time**: 10m. **Agent**: forge repo lane. **Model**: Opus

- [ ] **Step 24** [W] ~1m: The lists. `forge-move.txt` leaves this repo; `forge-copy.txt` stays here too.
```bash
cat > ~/tmp/wt/forge-move.txt <<'TXT'
crates/zhenai-forge
cmd/zhen-flywheel
man/zhen-flywheel.1
scripts/zlora-to-gguf.py
scripts/build-coding-corpus.py
scripts/tokenize-kingdom-for-gemma4.py
scripts/tokenize-coding-for-gemma4.py
scripts/two-brains/test_export_roundtrip.py
scripts/forge-train.sh
scripts/forge-watchdog.sh
scripts/gemma4-decode-tokens.py
scripts/gemma4-encode-prompt.py
scripts/auto-distill.sh
scripts/wave14-rag-baseline.py
scripts/wave14-score-runB.py
scripts/lib/gpulease_forge_systemd_test.go
raft/kingdom-w12
raft/experiments
raft/ADD_SOURCES.md
raft/scripts/01_prepare_corpus.py
raft/scripts/02_embeddings.py
raft/scripts/04_integration_tests.py
raft/scripts/05_generate_qa.py
raft/scripts/06_expand_corpus.py
raft/scripts/06b_embed_chunked.py
raft/scripts/07_prepare_training.py
raft/scripts/08_train_qlora.py
raft/scripts/09_merge_and_quantize.sh
raft/scripts/10_hotswap_index.py
raft/scripts/11_generate_qa_ring234.py
raft/scripts/12_process_stackoverflow.py
raft/scripts/13_ingest_skills.py
raft/scripts/14_extract_wikipedia.py
raft/scripts/15_rebuild_corpus_v2.py
raft/scripts/16_embed_v2.py
raft/scripts/17_subsample_wikipedia.py
raft/scripts/18_embed_wiki_subsample.py
raft/scripts/19_context_benchmark.py
raft/scripts/ab_test_checkpoint.sh
raft/scripts/add-source.sh
raft/scripts/distill_qa.py
runbooks/data/raft-training-pipeline.yaml
runbooks/data/corpus-rebuild.yaml
runbooks/data/zhen-index-rebuild.yaml
TXT
cat > ~/tmp/wt/forge-copy.txt <<'TXT'
scripts/lib/gpu-lease.sh
scripts/lib/logpath.sh
docs/adr/ADR-018-zhen-raft-training-battle-plan.md
docs/adr/ADR-030-zhenai-forge-rust-training.md
docs/adr/ADR-036-claude-distillation-training.md
docs/adr/ADR-045-wave-10d-gpu-backward-first-real-training.md
docs/adr/ADR-048-forge-backend-trait.md
docs/adr/ADR-049-wave11-gpu-kernels-for-gemma4-training.md
docs/adr/ADR-050-wave12-gpu-resident-activations.md
docs/adr/ADR-051-wave13-generate-path.md
docs/adr/ADR-061-cloud-rented-training-purpose-built-coding-model.md
docs/battle-plans/WAVE7-FORGE-QUALITY.md
docs/battle-plans/WAVE8-FORGE-PRODUCTION.md
docs/battle-plans/WAVE10-RAFT-QUALITY.md
docs/battle-plans/WAVE10B-FORGE-FIX.md
docs/battle-plans/WAVE10C-PROPER-BACKPROP.md
docs/battle-plans/WAVE10D-GPU-BACKWARD.md
docs/battle-plans/WAVE10E-GEMMA4-FORGE.md
docs/battle-plans/WAVE10F-FORGE-REAL-ATTENTION-GEMMA4.md
docs/battle-plans/WAVE13-INFERENCE.md
docs/battle-plans/WAVE13-PHASE2-REMOTE-PACKET.md
docs/battle-plans/WAVE14-STUB.md
docs/sessions/2026-04-30-wave14-session.md
docs/planning/zhen/unheaded-zhen-raft-spec.md
wiki/Wave-10C-Backprop.md
wiki/Zhen-Training-Data.md
docs/architecture/zhen-capture.md
TXT
wc -l < ~/tmp/wt/forge-move.txt; wc -l < ~/tmp/wt/forge-copy.txt; cd ~/tmp/unheaded && cat ~/tmp/wt/forge-move.txt ~/tmp/wt/forge-copy.txt | while read -r p; do [ -e "$p" ] || echo MISSING "$p"; done
```
  Expect `44`, `27`, no `MISSING` line.
- [ ] **Step 25** [B] ~3m:
```bash
set -euo pipefail
cd ~/tmp/projects && git clone -q --no-local --single-branch --branch develop ~/tmp/unheaded unheaded-forge && cd unheaded-forge && git remote remove origin
[ "$(basename "$(git rev-parse --show-toplevel)")" = unheaded-forge ]
[ -z "$(git remote)" ]
git rev-parse HEAD > ~/tmp/wt/unheaded-forge.source
export FILTER_BRANCH_SQUELCH_WARNING=1
P=$(cat ~/tmp/wt/forge-move.txt ~/tmp/wt/forge-copy.txt | tr '\n' ' ')
git filter-branch -f --prune-empty --index-filter "git read-tree --empty; git reset -q \$GIT_COMMIT -- $P 2>/dev/null; true" -- develop 2>&1 | tail -1
rm -rf .git/refs/original && git reflog expire --expire=now --all && git gc -q --prune=now
git branch -m develop main && git config user.name 'Stevie Bellis' && git config user.email stevie@bellis.tech && git rev-list --count main && git ls-files | wc -l && du -sh .git | cut -f1
```
  Expect `Ref 'refs/heads/develop' was rewritten`, a commit count above `226`, a file count above `190`, about `101M`.
- [ ] **Step 26** [V] ~1m: **PHASE 4A EXIT GATE.** `cd ~/tmp/projects/unheaded-forge && gitleaks git . --no-banner --redact --config ~/tmp/unheaded/.gitleaks.toml -f json -r /var/tmp/s-move-out-forge-leaks.json -l error; python3 -c "import json;print('leaks',len(json.load(open('/var/tmp/s-move-out-forge-leaks.json'))))"; git rev-list --objects --all | git cat-file --batch-check='%(objectsize) %(rest)' | sort -n | tail -1; git remote | wc -l` Expect `leaks 0`, a largest object under `100000000` (GitHub's limit; it was 22610957), `0`. A finding: `[BLOCKED]`, file names only, no Phase 12A.

## PHASE 4B: unheaded-forge stands alone and says why (round 4)
**Goal**: tests pass there and the README records the work, so nobody digs through history. Opus: the README is judgement. **Prerequisite**: Phase 4A. **Time**: 30m. **Agent**: forge repo lane. **Model**: Opus

- [ ] **Step 27** [V] ~6m: `cd ~/tmp/projects/unheaded-forge/crates/zhenai-forge && CARGO_TARGET_DIR=/var/tmp/s-move-out-forge cargo test --locked -j4 --lib 2>&1 | grep -E '^test result|^error' | head -2` Expect `test result: ok. 111 passed; 0 failed; 28 ignored`. Needs the real HIP kernels (hipcc): `ZHENAI_FORGE_SKIP_KERNELS=1` cannot link the tests.
- [ ] **Step 28** [W] ~4m: `zhen-flywheel` gets a module. It imports `unheaded/pkg/zhencapture`, `pkg/cmdline` and `pkg/claudebridge/privacy`, which stay.
```bash
cd ~/tmp/projects/unheaded-forge && cat > go.mod <<'MOD'
module unheaded-forge

go 1.26.0

toolchain go1.27.3

require unheaded v0.0.0

replace unheaded => ../../unheaded

replace github.com/unheaded/doomgeneric => ../doomgeneric/unheaded
MOD
export GOWORK=off GOFLAGS=-mod=mod GOMAXPROCS=4 GOTMPDIR=/var/tmp && go mod tidy && go build -o /dev/null ./cmd/zhen-flywheel && go test ./cmd/zhen-flywheel/ 2>&1 | tail -1 | cut -c1-60
```
  Expect a line starting `ok`. The doomgeneric `replace` is not needed by today's imports (measured); it covers a new import until the merge. Step 115 drops it.
- [ ] **Step 29** [W] ~15m: README, licence, the moved test as text, DP-314. Add detail from `crates/zhenai-forge/notes` where it helps; keep every number below.
```bash
cd ~/tmp/projects/unheaded-forge && cp ~/tmp/unheaded/LICENSE LICENSE && printf 'target/\n__pycache__/\n*.wad\n*.WAD\n' > .gitignore && mkdir -p reference && git mv scripts/lib/gpulease_forge_systemd_test.go reference/gpulease_forge_systemd_test.go.txt
sed -n '/^func TestForgeTrain_WaitsForTheLease/,/^}/p' ~/tmp/unheaded/scripts/lib/gpulease_scripts_test.go > reference/gpulease_scripts_test.go.txt
python3 - <<'PY'
import re
p='raft/scripts/distill_qa.py'; s=open(p).read()
s2,n=re.subn(r'^    "doom_docs": \{\n.*?^    \},\n','',s,count=1,flags=re.S|re.M)
open(p,'w').write(s2); print('doom source removed',n)
PY
cat > README.md <<'MD'
# unheaded-forge

Zhen's training work: `zhenai-forge`, a Rust LoRA trainer with HIP kernels for Gemma-4 E2B, and everything that fed it. Moved out of `unheaded/unheaded` on 2026-11-04 with its history. GPL-3.0-or-later.

## Why it is here

**Back burner.** Training is unworkable on west and very little progress came through the experiments. It waits for a machine with enough GPU; the project's effort goes to tangible features. The work is kept here so it can be picked up without digging through history. The running Zhen does not need any of it: it serves through llama-server.

## State of the world

- **The Gemma-4 base is not on west.** `/var/zhen/models` has no Gemma file and `~/tmp/gemma-4-E2B-it` holds config and tokenizer only. The conversion used llama.cpp `convert_hf_to_gguf.py` from the HF safetensors. Re-acquire it before anything else.
- **`kingdom-v2-lora.gguf` and `kingdom-v3-lora.gguf` are scrambled.** `scripts/zlora-to-gguf.py` never transposed: it writes the forge's row-major A (n_embd x rank) and B (rank x n_embd) under GGUF dims whose fastest axis is the other one, truncates K/V columns, and has Mistral-7B's GQA baked in. It reads `ZLORA` only; the Gemma path saves `ZLG4`. Nothing served from those two files means anything.
- **The corpus.** Anthropic's 2026-10-26 answer requires prior written authorization to train on Claude outputs. No training run uses repo text until that reply is on file (DP-705). Claude answers and captures feed no forge or LoRA run without that written reply, and `raft/scripts/distill_qa.py` defaults to the Claude API: it must not run in API mode until then. `zhen-flywheel`'s `-include-claude-text` and `ZHEN_FLYWHEEL_INCLUDE_CLAUDE_TEXT` stay off until then.

## How the tests were done

`cargo test --lib` runs 111 tests, with real HIP kernels (hipcc). 28 more are `#[ignore]`: they need the base at `/var/zhen/models/gemma-4-E2B-it.gguf` and memory, so `cargo test -- --ignored` one at a time, `jobs = 4` (`.cargo/config.toml`, `notes/test-runner-hardening.md`). 25 assert Gemma-4, 3 the old Mistral path.

The Learning Gate (`notes/wave-10f`, `src/eval.rs`) separated learning from memorizing on the real GGUF: held-out eval loss fell 42 % (CI95 excludes 1.0); a LoRA-A of zero reproduced the base bit for bit; eval scaled monotonically with dataset size, and `|T|=1` got worse, the memorization signature; the generalization-gap exponent was 0.27. The scrambled-labels control became a diagnostic: it showed `log(262144) = 12.48` is an information floor. `lr=1e-3` was the stable point; `3e-3` blew up by step 100.

## What came of it

WAVE12: 500 steps at seq 384, held-out eval 21.10 to 6.78 over 397 sequences. WAVE13 Phase 2 then scored 0 of 8 LoRA-better on real questions: RETRAIN. The H6 root cause was found after: the line parser took the JSON `answer_start` as a stray token, so 11 days of training were corrupted (`notes/wave-14`).

## Carried here, not done

- DP-308: emit GGUF from Rust and retire `zlora-to-gguf.py`. The corpus is Unheaded-only: `--include-stackoverflow` and `--include-wikipedia` in `build-coding-corpus.py` go.
- DP-608: `answer_start` per example, before any LoRA window.

## Layout

| Path | What |
|---|---|
| `crates/zhenai-forge` | trainer, kernels, notes |
| `cmd/zhen-flywheel` | ingests captures, labels them, exports training sets. Builds against `../../unheaded` for `pkg/zhencapture`. The capture tables stay in the Well and captures stay off |
| `scripts/` | train, watchdog, tokenizers, converter |
| `raft/kingdom-w12` | the WAVE12 LoRA and its checkpoints |
| `raft/scripts` | the old Mistral RAFT pipeline and the corpus scripts that fed the retired FAISS index |
| `runbooks/data` | the three runbooks that drove them |
| `docs/` | the ADRs and WAVE plans, copied |

Scripts assume the old repository layout in places (`auto-distill.sh`, `raft/scripts`).
MD
python3 -m py_compile raft/scripts/distill_qa.py && git add .gitignore LICENSE README.md go.mod go.sum raft/scripts/distill_qa.py reference && git commit -q --no-gpg-sign -m 'Stand alone: README records the work, why it stopped, and the scrambled LoRAs' && git status --short | wc -l; git remote | wc -l
```
  Expect `doom source removed 1`, `0`, `0`.

## PHASE 5: the UPC specs stay (DP-305)
**Goal**: ten UPC documents leave `docs/doom/` for `docs/upc/` before the rest goes, and every living reference follows. **Prerequisite**: Phase 0. **Time**: 10m. **Agent**: docs lane. **Model**: Haiku

- [ ] **Step 30** [B] ~1m:
```bash
cd ~/tmp/wt/s-move-out && mkdir -p docs/upc && for f in UPC_BOOT_PROTOCOL.md UPC_BOOT_PROTOCOL_V2.md UPC_PAGE_TABLE_LAYOUT.md UPC_OS_PRIMITIVES.md UPC-PORT-PROCESS.md UPC_BOOT_DEMO.md UCLINUX_PORT_GUIDE.md ROAD_TO_LINUX.md BPF-MAP-REFERENCE.md COMPUTATIONAL_GENERALITY.md; do mv docs/doom/$f docs/upc/; done; ls docs/upc | wc -l; ls docs/doom | wc -l
```
  Expect `10`, `21`.
- [ ] **Step 31** [W] ~2m: References, in living files only: `docs/archive`, `references`, old sprint plans and `docs/legal` are history.
```bash
cd ~/tmp/wt/s-move-out && git grep -l -E 'docs/doom/(UPC_|UPC-|UCLINUX|ROAD_TO_LINUX|BPF-MAP-REFERENCE|COMPUTATIONAL_GENERALITY)' -- . ':!docs/archive' ':!references' ':!docs/legal' ':!docs/sweeps' ':!docs/doom' ':!docs/upc' ':!docs/battle-plans/S-*' ':!docs/battle-plans/DECISION-PACK-2026-11-03.md' > /var/tmp/s-move-out-upcrefs.txt
xargs sed -i -E 's#docs/doom/(UPC_|UPC-|UCLINUX|ROAD_TO_LINUX|BPF-MAP-REFERENCE|COMPUTATIONAL_GENERALITY)#docs/upc/\1#g' < /var/tmp/s-move-out-upcrefs.txt
wc -l < /var/tmp/s-move-out-upcrefs.txt; git grep -c -E 'docs/doom/UPC_BOOT_PROTOCOL_V2' -- cmd crates ebpf docs/adr docs/protocol wiki; echo rc=$?
```
  Expect a count near `30`, `rc=1`.
- [ ] **Step 32** [V] ~1m: Comment-only edits still parse: `cd ~/tmp/wt/s-move-out && (cd cmd/upc-bootctl && cargo fmt -- --check 2>&1 | tail -1); git diff --stat -- ebpf crates cmd | tail -1` Expect a stat line of comment changes; no `rustfmt` error.
- [ ] **Step 33** [C] ~1m: **PHASE 5 EXIT GATE + COMMIT.** `cd ~/tmp/wt/s-move-out && ~/tmp/wt/bin/plan-commit s-move-out S-MOVE-OUT "Steps 6-33: UPC specs move to docs/upc, references follow (DP-305)" $(for f in UPC_BOOT_PROTOCOL.md UPC_BOOT_PROTOCOL_V2.md UPC_PAGE_TABLE_LAYOUT.md UPC_OS_PRIMITIVES.md UPC-PORT-PROCESS.md UPC_BOOT_DEMO.md UCLINUX_PORT_GUIDE.md ROAD_TO_LINUX.md BPF-MAP-REFERENCE.md COMPUTATIONAL_GENERALITY.md; do echo docs/doom/$f docs/upc/$f; done) $(cat /var/tmp/s-move-out-upcrefs.txt) docs/battle-plans/progress/S-MOVE-OUT-progress.md`

## PHASE 6: remove the Doom paths from this repo (DP-305)
**Goal**: everything in Step 6 gone except the fixture and `doom/LICENSE`. **Prerequisite**: Phase 1's and Phase 2's exit gates passed, Phase 5 committed. **Time**: 5m. **Agent**: one. **Model**: Haiku

- [ ] **Step 34** [V] ~1m: The move is safe: `cd ~/tmp/projects/unheaded-doom && git log -1 --format=%s && test -f crates/doom-runner/Cargo.toml && test -f docs/doom/FINDINGS.md && test -f doom/doom.elf && echo NEW-REPO-OK` Expect `Stand alone: module, sibling paths, doom-ctl, README`, `NEW-REPO-OK`.
- [ ] **Step 35** [V] ~1m: Every blob about to leave exists in the new repo.
```bash
cat > ~/tmp/wt/bin/blobs-present <<'SH'
#!/usr/bin/env bash
# blobs-present NEWREPO LISTFILE: every blob under the listed paths at HEAD must exist in NEWREPO.
set -u
repo=$1 list=$2; n=0; miss=0
while read -r _ _ sha path; do n=$((n+1)); git -C "$repo" cat-file -e "$sha" 2>/dev/null || { echo "MISSING $path"; miss=$((miss+1)); }; done < <(xargs git ls-tree -r HEAD -- < "$list")
echo "blobs $n missing $miss"; [ "$miss" = 0 ]
SH
chmod +x ~/tmp/wt/bin/blobs-present; cd ~/tmp/wt/s-move-out && ~/tmp/wt/bin/blobs-present ~/tmp/projects/unheaded-doom ~/tmp/wt/doom-paths.txt
```
  Expect `blobs <n> missing 0` with `<n>` near `167`. A `MISSING` line: stop, nothing is removed.
- [ ] **Step 36** [B] ~1m:
```bash
cd ~/tmp/wt/s-move-out && grep -v -E '^doom/(LICENSE|doom\.mbc|doom\.rv2mbc|doom\.rv2mbc\.sha256)$' ~/tmp/wt/doom-paths.txt > /var/tmp/s-move-out-doom-rm.txt && xargs rm -rf < /var/tmp/s-move-out-doom-rm.txt && sha256sum doom/doom.rv2mbc | cut -d' ' -f1 | diff - doom/doom.rv2mbc.sha256 && ls doom | tr '\n' ' '
```
  Expect `LICENSE doom.mbc doom.rv2mbc doom.rv2mbc.sha256` (plus `doomgeneric` only if an empty local directory exists).
- [ ] **Step 37** [C] ~1m: **PHASE 6 EXIT GATE + COMMIT.** The tree does not build until Phase 7. `cd ~/tmp/wt/s-move-out && ~/tmp/wt/bin/plan-commit s-move-out S-MOVE-OUT "Steps 34-37: Doom leaves for unheaded-doom; the fixture stays (DP-305)" $(cat /var/tmp/s-move-out-doom-rm.txt) docs/battle-plans/progress/S-MOVE-OUT-progress.md`

## PHASE 7: wotan-ctl and the dashboard (DP-305)
**Goal**: this repo builds again. **Prerequisite**: Phase 6. **Time**: 25m. **Agent**: Go lane. **Model**: Sonnet

- [ ] **Step 38** [W] ~3m: `wotan-ctl` loses two commands.
```bash
cd ~/tmp/wt/s-move-out && python3 ~/tmp/wt/bin/sub.py cmd/wotan-ctl/main.go <<'SUB'
<<<<
	root.SubCommands["load-mem"] = newLoadMemCmd()
	root.SubCommands["doom"] = newDoomCmd()
====
>>>>
<<<<
  - load-mem: Load binary data into L2 ring buffer (RAM)
  - doom:     Doom-over-IPv6 management (load, status, input, reset)
====
>>>>
SUB
sed -i '/^\/\/   - load-mem: Load binary data into Wotan.s L2 ring buffer/d; /"wotan-ctl doom status  *Doom-over-IPv6 sta/d; s/^wotan-ctl is the operator.s tool for the Doom-over-IPv6 compute engine\.$/wotan-ctl is the operator'"'"'s tool for the UPC compute engine./' cmd/wotan-ctl/main.go
sed -i "s/Long: \`wotan-ctl is the operator's tool for the Doom-over-IPv6 compute engine\./Long: \`wotan-ctl is the operator's tool for the UPC compute engine./" cmd/wotan-ctl/main.go
sed -i '/^- \*\*load-mem\*\*: /d; /^- `newLoadMemCmd()`: Creates load-mem subcommand$/d' cmd/wotan-ctl/README.md
python3 - <<'PY'
import re
p='cmd/wotan-ctl/README.md'; s=open(p).read()
s2=re.sub(r'^### load-mem: Load Binary Data\n.*?(?=^### |^## |\Z)','',s,flags=re.S|re.M)
open(p,'w').write(s2); print('README', s!=s2)
PY
go build ./cmd/wotan-ctl && go test ./cmd/wotan-ctl/ 2>&1 | tail -1 | cut -c1-40; git grep -c -E 'newDoomCmd|newLoadMemCmd' -- cmd/wotan-ctl; echo rc=$?
```
  Expect `sub.py: cmd/wotan-ctl/main.go: 2 block(s) applied`, `README True`, a line starting `ok`, `rc=1`.
- [ ] **Step 39** [W] ~5m: Dashboard: state, routes, health.
```bash
cd ~/tmp/wt/s-move-out && python3 ~/tmp/wt/bin/sub.py cmd/dashboard-backend/internal/server/server.go <<'SUB'
<<<<

	// Doom compute state (populated by compute.* events from ingestor)
	doomState *DoomState
====
>>>>
<<<<
		doomState:         &DoomState{},
====
>>>>
<<<<
	// Doom compute endpoints (S42)
	s.mux.HandleFunc("/api/v1/doom/screen", s.handleDoomScreen)
	s.mux.HandleFunc("/api/v1/doom/cpu", s.handleDoomCPU)
	s.mux.HandleFunc("/api/v1/doom/input", s.handleDoomInput)

====
>>>>
SUB
python3 ~/tmp/wt/bin/sub.py cmd/dashboard-backend/internal/health/health.go <<'SUB'
<<<<
// whose liveness lives elsewhere (Grafana + vor at /api/health, the Doom bridge
// which only serves "/", Traefik's dashboard at /dashboard). Keys must match the
// service name used in the endpoint map / overrides file.
====
// whose liveness lives elsewhere (Grafana + vor at /api/health, Traefik's
// dashboard at /dashboard). Keys must match the service name used in the
// endpoint map / overrides file.
>>>>
<<<<
	"doom":    "/",
====
>>>>
SUB
python3 - <<'PY'
import re
p='cmd/dashboard-backend/internal/server/server_extended_test.go'; s=open(p).read()
n0=s.count('func TestHandleDoom')+s.count('func TestDoomState')
s=re.sub(r'(// ---- Doom [^\n]*----\n\n)?func TestHandleDoom\w+\(t \*testing\.T\) \{\n.*?\n\}\n\n?','',s,flags=re.S)
s=re.sub(r'(// ---- DoomState [^\n]*----\n\n)?func TestDoomState\w+\(t \*testing\.T\) \{\n.*?\n\}\n\n?','',s,flags=re.S)
open(p,'w').write(s); print('tests removed',n0)
PY
sed -i 's#const write = "/api/v1/doom/input"#const write = "/api/v1/services/config/wotan"#; s#http.MethodPost, "/api/v1/doom/input", "{}"#http.MethodPost, "/api/v1/services/config/wotan", "{}"#g' cmd/dashboard-backend/internal/server/session_test.go
gofmt -l cmd/dashboard-backend; go vet ./cmd/dashboard-backend/... 2>&1 | head -5
```
  Expect two `sub.py` lines, a `tests removed` count, no `gofmt` or `vet` lines. A `vet` line naming `DoomState` at another test: delete that test the same way and record it.
- [ ] **Step 40** [V] ~3m: `cd ~/tmp/wt/s-move-out && go test ./cmd/dashboard-backend/... 2>&1 | grep -v -E '^(ok|\?)' | head -8; git grep -n -E 'DoomState|doomState|api/v1/doom' -- cmd pkg dashboard; go build ./... 2>&1 | head -5` Expect no output at all. `TestBFF_*` failing on the new route: `[STUCK]`, name the status code. The route only needs to pass the auth layer.
- [ ] **Step 41** [C] ~1m: **PHASE 7 EXIT GATE + COMMIT.** `cd ~/tmp/wt/s-move-out && ~/tmp/wt/bin/plan-commit s-move-out S-MOVE-OUT "Steps 38-41: wotan-ctl and dashboard-backend without Doom (DP-305)" cmd/wotan-ctl/main.go cmd/wotan-ctl/README.md cmd/dashboard-backend/internal/server/server.go cmd/dashboard-backend/internal/health/health.go cmd/dashboard-backend/internal/server/server_extended_test.go cmd/dashboard-backend/internal/server/session_test.go docs/battle-plans/progress/S-MOVE-OUT-progress.md`

## PHASE 8: doomgeneric leaves go.mod and CI (DP-305)
**Goal**: no `replace`, no fetch step in any workflow, and the trap S-CLEAN-CUT worked around is gone. **Found**: twelve fetch steps across five workflows (`static-analysis.yml` too, which would have broken CI once the script went); two `--ignore`; four Dockerfiles that `sed` doomgeneric out of `go.mod`; `Makefile` handled in Phase 16. **Prerequisite**: Phase 7. **Time**: 15m. **Agent**: CI lane. **Model**: Haiku

- [ ] **Step 42** [B] ~3m: `cd ~/tmp/wt/s-move-out && go mod edit -droprequire github.com/unheaded/doomgeneric -dropreplace github.com/unheaded/doomgeneric && GOFLAGS=-mod=mod go mod tidy && go build ./... && grep -c doomgeneric go.mod go.sum; echo rc=$?` Expect `go.mod:0`, `go.sum:0`, `rc=1`.
- [ ] **Step 43** [W] ~3m: Workflows.
```bash
cd ~/tmp/wt/s-move-out && python3 - <<'PY'
import re
total=0
for p in ('ci','gpl-boundary','sbom-audit','security','static-analysis'):
    f=f'.github/workflows/{p}.yml'; s=open(f).read()
    s,n=re.subn(r'^ +- name: Fetch doomgeneric \(go\.mod replace target\)\n +run: \./scripts/ci-fetch-doomgeneric\.sh\n\n?','',s,flags=re.M); total+=n
    s=re.sub(r'^ +--ignore github\.com/unheaded/doomgeneric \\\n','',s,flags=re.M)
    s=re.sub(r'^ +#? *#?  ?github\.com/unheaded/doomgeneric: first-party, SPDX MIT in every\n +# +pkg/doom file, but its module dir( \(unheaded/\))? has no LICENSE( file to find\.| *\n +# +file of its own for go-licenses to find\.)\n','',s,flags=re.M)
    s=s.replace("# correctly configured (gosec go-installed at the module's Go, doomgeneric\n  # fetched first, grype at a medium cutoff).","# correctly configured (gosec go-installed at the module's Go, grype at a\n  # medium cutoff).")
    s=s.replace("the gosec action ran Go 1.22 and scanned 0 files, govulncheck had no\n  # doomgeneric checkout, and the SBOM job","the gosec action ran Go 1.22 and scanned 0 files, and the SBOM job")
    open(f,'w').write(s)
print('fetch steps removed',total)
PY
rm -f scripts/ci-fetch-doomgeneric.sh
sed -i "s/, 'github.com\/unheaded\/doomgeneric': 'GPL-2.0'//; s/'github.com\/unheaded\/doomgeneric': 'GPL-2.0', //" scripts/generate-sbom.sh
sed -i '/^ci-fetch-doomgeneric|/d; /^doom-smoke|/d; /^doom-ring|/d' scripts/check-gates-can-fail.sh
git grep -n 'doomgeneric' -- .github scripts/generate-sbom.sh scripts/check-gates-can-fail.sh | cut -c1-100; for f in ci gpl-boundary sbom-audit security static-analysis; do python3 -c "import yaml;yaml.safe_load(open('.github/workflows/$f.yml'))" && echo ok-$f; done; bash -n scripts/check-gates-can-fail.sh
```
  Expect `fetch steps removed 12` (3+1+2+5+1), no grep lines, five `ok-` lines. A grep line left in a comment: delete that comment line and record it.
- [ ] **Step 44** [W] ~2m: Dockerfiles stop stripping a line that no longer exists.
```bash
cd ~/tmp/wt/s-move-out && sed -i "/^    sed -i '\/doomgeneric\/d' go.mod && \\\\$/d; s/^# Remove local replace directives and doomgeneric references for container builds$/# Remove local replace directives for container builds/" Dockerfile
sed -i "/^# go.mod replaces github.com\/unheaded\/doomgeneric with a sibling checkout$/d; /^# outside the build context (..\/projects\/doomgeneric); only wotan-ctl imports$/d; s/^RUN sed -i '\/doomgeneric\/d' go.mod && go mod download$/RUN go mod download/; /^RUN sed -i '\/doomgeneric\/d' go.mod$/d" deploy/docker/zhen-agentd.Dockerfile
sed -i "/^# doomgeneric is replaced by a path outside the repo/d; s/^RUN sed -i '\/doomgeneric\/d' go.mod go.sum && go mod download$/RUN go mod download/; s/^RUN sed -i '\/doomgeneric\/d' go.mod go.sum && \\\\$/RUN \\\\/" deploy/sophia-eye/sophia-gateway/Dockerfile docker/alert-mailer/Dockerfile
git grep -n 'doomgeneric' -- Dockerfile deploy/docker/zhen-agentd.Dockerfile deploy/sophia-eye/sophia-gateway/Dockerfile docker/alert-mailer/Dockerfile | cut -c1-90
```
  Expect one line at most: the second half of the `zhen-agentd.Dockerfile` comment. Delete that comment line with `sed` on its exact text and record it. Any `RUN` line left: `[STUCK]`.
- [ ] **Step 45** [C] ~1m: **PHASE 8 EXIT GATE + COMMIT.** `cd ~/tmp/wt/s-move-out && ~/tmp/wt/bin/plan-commit s-move-out S-MOVE-OUT "Steps 42-45: doomgeneric leaves go.mod and every workflow" go.mod go.sum .github/workflows/ci.yml .github/workflows/gpl-boundary.yml .github/workflows/sbom-audit.yml .github/workflows/security.yml .github/workflows/static-analysis.yml Dockerfile deploy/docker/zhen-agentd.Dockerfile deploy/sophia-eye/sophia-gateway/Dockerfile docker/alert-mailer/Dockerfile scripts/ci-fetch-doomgeneric.sh scripts/generate-sbom.sh scripts/check-gates-can-fail.sh docs/battle-plans/progress/S-MOVE-OUT-progress.md`

## PHASE 9: the regression harness (DP-305)
**Goal**: `upc-regression.sh` passes, still runs the real Doom load through the sibling runner, and checks the fixture on every run. Opus: both scripts run sudo and attach XDP (rule 13). The text below is the harness after S-STAGE-2B Step 6. **Prerequisite**: Phases 2 and 6. **Time**: 20m. **Agent**: UPC lane. **Model**: Opus

- [ ] **Step 46** [W] ~4m: Harness.
```bash
cd ~/tmp/wt/s-move-out && python3 ~/tmp/wt/bin/sub.py scripts/upc-regression.sh <<'SUB'
<<<<
DR="$ROOT/crates/doom-runner/target/release/doom-runner"
====
# doom-runner and doom.elf live in unheaded-doom since 2026-11-04 (DP-305).
DR="${DOOM_RUNNER:-$HOME/tmp/projects/unheaded-doom/crates/doom-runner/target/release/doom-runner}"
DOOM_ELF="${DOOM_ELF:-$HOME/tmp/projects/unheaded-doom/doom/doom.elf}"
>>>>
<<<<
if [ -f "$WAD" ]; then
  { doom_stop; sudo "$DR" ring teardown --hops 2; } >/dev/null 2>&1
====
# The fixture stays in this repository: intact and structurally valid, always.
if ( cd "$ROOT/doom" && [ "$(sha256sum doom.rv2mbc | cut -d' ' -f1)" = "$(cat doom.rv2mbc.sha256)" ] ); then
  pass "Doom fixture: doom.rv2mbc matches its sha256"
else
  fail "Doom fixture: doom.rv2mbc does not match its sha256"
fi
if "$BC" validate --kernel "$ROOT/doom/doom.mbc" >/dev/null 2>&1; then
  pass "Doom fixture: doom.mbc is a valid image"
else
  fail "Doom fixture: upc-bootctl refuses doom.mbc"
fi

if [ -f "$WAD" ] && [ -x "$DR" ] && [ -f "$DOOM_ELF" ]; then
  { doom_stop; sudo "$DR" ring teardown --hops 2; } >/dev/null 2>&1
>>>>
<<<<
  --doom-elf doom/doom.elf --rv2mbc doom/doom.rv2mbc --wad "$WAD" --hops 2 >"$LOG" 2>&1 & ) 2>/dev/null
====
  --doom-elf "$DOOM_ELF" --rv2mbc doom/doom.rv2mbc --wad "$WAD" --hops 2 >"$LOG" 2>&1 & ) 2>/dev/null
>>>>
<<<<
  note "Doom: SKIPPED — WAD not found at $WAD (set DOOM_WAD=...)"
====
  if [ "${UPC_REQUIRE_DOOM:-0}" = 1 ]; then
    fail "Doom load: SKIPPED — needs $WAD, $DR and $DOOM_ELF (DOOM_WAD, DOOM_RUNNER, DOOM_ELF)"
  else
    note "Doom load: SKIPPED — needs $WAD, $DR and $DOOM_ELF (DOOM_WAD, DOOM_RUNNER, DOOM_ELF)"
  fi
>>>>
SUB
bash -n scripts/upc-regression.sh && echo SYNTAX-OK
```
  Expect `sub.py: scripts/upc-regression.sh: 4 block(s) applied`, `SYNTAX-OK`.
- [ ] **Step 47** [W] ~2m: `ascend-linux-smoke.sh` built doom-runner in place.
```bash
cd ~/tmp/wt/s-move-out && python3 ~/tmp/wt/bin/sub.py scripts/ascend-linux-smoke.sh <<'SUB'
<<<<
# 10. doom-runner builds (regression: ABI v2 MbcCpuState 136 bytes)
step "doom-runner builds with ABI v2 (regression)" \
    bash -c 'cd crates/doom-runner && cargo build --release 2>&1 | tail -1 | grep -q "Finished"'

====
>>>>
SUB
sed -i '/^#   5\. doom-runner builds (regression: ABI v2 didn.t break Doom)$/d' scripts/ascend-linux-smoke.sh && bash -n scripts/ascend-linux-smoke.sh && git grep -c 'crates/doom-runner' -- scripts/ascend-linux-smoke.sh; echo rc=$?
```
  Expect the `sub.py` line, `rc=1`.
- [ ] **Step 48** [B] ~6m: Build what the harness loads, as the normal user.
```bash
cd ~/tmp/projects/unheaded-doom/crates/doom-runner && cargo build --release --locked -j4 2>&1 | tail -1 && test -x target/release/doom-runner && test -f ~/tmp/projects/unheaded-doom/doom/doom.elf && echo RUNNER-OK
cd ~/tmp/wt/s-move-out/cmd/upc-bootctl && cargo build --release --locked -j4 2>&1 | tail -1
cd ~/tmp/wt/s-move-out && (cd crates/monad-mbc && cargo build --release --locked -j4 2>&1 | tail -1)
cd crates/xv6-mbc/upstream && make -f ../adapters/Makefile.mbc kernel 2>&1 | tail -1 && make -f ../adapters/Makefile.mbc-userland userland gates ramdisk 2>&1 | tail -1 && test -f target/fs.img && echo XV6-OK
```
  Expect `Finished`, `RUNNER-OK`, `Finished`, `Finished`, `XV6-OK`. A fresh worktree has no `crates/xv6-mbc/upstream/target`, and the harness does not build it.
- [ ] **Step 49** [S] ~15m: The harness itself, live: a named root step. It loads BPF inside netns `upc0` (S-ASCEND-2) and touches nothing rule 5 reserves.
```bash
cd ~/tmp/wt/s-move-out && UPC_REQUIRE_DOOM=1 DOOM_RUNNER=$HOME/tmp/projects/unheaded-doom/crates/doom-runner/target/release/doom-runner DOOM_ELF=$HOME/tmp/projects/unheaded-doom/doom/doom.elf bash scripts/upc-regression.sh 2>&1 | tee /var/tmp/s-move-out-harness.txt | tail -14 | cut -c1-90; grep -ac 'verified 758199 insns' /var/tmp/s-move-out-harness.txt; grep -ac 'Doom load: SKIPPED' /var/tmp/s-move-out-harness.txt
```
  Expect `PASS` on both `Doom fixture` lines, `Doom object loads`, the xv6 and cartridge lines; the count at least `1` (if S-STAGE-2B's last gate printed another number, grep for that one: `758199` was the number at forge time; the plain pattern would also match the ascend line); `0` skips. A skip means the WAD at `~/tmp/projects/doom-related/DOOM.WAD` is missing: `[BLOCKED]`, do not merge, the S-STAGE-2B gate would read it as red. No passwordless sudo: `[BLOCKED]`, this step waits for an attended session; do not merge without it.
- [ ] **Step 50** [C] ~1m: **PHASE 9 EXIT GATE + COMMIT.** `cd ~/tmp/wt/s-move-out && ~/tmp/wt/bin/plan-commit s-move-out S-MOVE-OUT "Steps 46-50: upc-regression holds the Doom fixture without the runner (DP-305)" scripts/upc-regression.sh scripts/ascend-linux-smoke.sh docs/battle-plans/progress/S-MOVE-OUT-progress.md`

## PHASE 9A: the fixture's consumers (DP-305)
**Goal**: `crates/upc-api`, `cmd/upc-bootctl` and the fixture pass as the normal user, and CI covers the fixture. **Prerequisite**: Phases 6 and 7. **Time**: 15m. **Agent**: Rust lane. **Model**: Sonnet

- [ ] **Step 51** [W] ~2m: Give the fixture real CI coverage.
```bash
cd ~/tmp/wt/s-move-out && sed -i 's#join("../../doom/doomgeneric/doom.mbc")#join("../../doom/doom.mbc")#' crates/monad-mbc/tests/integration_tests.rs && grep -c '"../../doom/doom.mbc"' crates/monad-mbc/tests/integration_tests.rs
cd crates/monad-mbc && CARGO_TARGET_DIR=/var/tmp/s-move-out-mbc cargo test --locked -j4 --test integration_tests step106 -- --nocapture 2>&1 | grep -E 'doom.mbc|^test result' | head -4
```
  Expect `1`, a line `doom.mbc: <n> instructions loaded`, and `test result: ok. 1 passed`. A failure: restore the line with `git checkout -- crates/monad-mbc/tests/integration_tests.rs`, `[STUCK]`: the fixture may not suit the host interpreter, and the sha256 check still covers it.
- [ ] **Step 52** [V] ~6m: The named consumers.
```bash
cd ~/tmp/wt/s-move-out && (cd crates/upc-api && CARGO_TARGET_DIR=/var/tmp/s-move-out-upc cargo test --locked -j4 2>&1 | grep -c -E '^test result: ok') && (cd cmd/upc-bootctl && CARGO_TARGET_DIR=/var/tmp/s-move-out-upc cargo test --locked -j4 2>&1 | grep -E '^test result' | tail -1) && CARGO_TARGET_DIR=/var/tmp/s-move-out-upc cargo run -q --locked --manifest-path cmd/upc-bootctl/Cargo.toml -- validate --kernel doom/doom.mbc 2>&1 | tail -1
```
  Expect a count of at least `2`, `test result: ok`, `✓ kernel image structurally valid`.
- [ ] **Step 53** [C] ~1m: **PHASE 9A EXIT GATE + COMMIT.** `cd ~/tmp/wt/s-move-out && ~/tmp/wt/bin/plan-commit s-move-out S-MOVE-OUT "Steps 51-53: the monad-mbc smoke test reads the real fixture (DP-305)" crates/monad-mbc/tests/integration_tests.rs docs/battle-plans/progress/S-MOVE-OUT-progress.md`

## PHASE 10: Doom rows in logging, units, scripts, Nix, baselines (DP-305, DP-314)
**Goal**: no gate or inventory names what left. **Found**: three rows in `config/logging/sources.yaml`, the runbook manifest, four baselines, `dependency-watch`, Nix host import. **Stays**: `.gitignore`'s `doom/*.wad` line; `pkg/uids` line 171's reserved number; `monitoring/prometheus` job `doom_services` (it is the port band; its targets are protocol services). **Prerequisite**: Phase 6. **Time**: 20m. **Agent**: Lane Q. **Model**: Haiku

- [ ] **Step 54** [W] ~3m: Logging inventory.
```bash
cd ~/tmp/wt/s-move-out && python3 - <<'PY'
import re
p='config/logging/sources.yaml'; s=open(p).read()
s,n=re.subn(r'^  - id: unit/unheaded-doom-(injector|ring|runner)\n(?:    .*\n)+','',s,flags=re.M)
open(p,'w').write(s); print('rows removed',n)
PY
./scripts/check-log-sources.sh 2>&1 | tail -1 | cut -c1-90; go test ./pkg/logsources/ 2>&1 | tail -1 | cut -c1-50
```
  Expect `rows removed 3`, a pass line, a line starting `ok`.
- [ ] **Step 55** [W] ~3m: Runbooks and units.
```bash
cd ~/tmp/wt/s-move-out && ./scripts/runbook-manifest.sh --write 2>&1 | tail -1; grep -c ' doom/' runbooks/MANIFEST.sha256; ./scripts/check-runbooks.sh 2>&1 | tail -1 | cut -c1-90
python3 - <<'PY'
import re
p='deploy/systemd/README.md'; s=open(p).read()
s=re.sub(r'^\| `unheaded-doom[^\n]*\n','',s,flags=re.M)
s2=re.sub(r'^## DOOM on the UPC \(full pipeline\)\n.*?(?=^## )','',s,flags=re.S|re.M)
open(p,'w').write(s2); print('section removed',s!=s2)
PY
sed -i '/^| `unheaded-doom\.target` (+ `-doom-ring`, `-doom-runner`, `-doom-injector`) | Doom on the UPC pipeline |$/d' wiki/Systemd-Units.md
git grep -c 'unheaded-doom' -- deploy/systemd/README.md wiki/Systemd-Units.md; echo rc=$?
```
  Expect a manifest line, `0`, a pass line, `section removed True`, `rc=1`. Two runbooks that still run Doom are Phase 10A. The others name the port band or a comment.
- [ ] **Step 56** [W] ~1m: Build list.
```bash
cd ~/tmp/wt/s-move-out && sed -i 's/ service-discovery doom lich / service-discovery lich /' docker/Makefile && git grep -c ' doom ' -- docker/Makefile; echo rc=$?
```
  Expect `rc=1`. The shell scripts and Nix host are Phase 10A.
- [ ] **Step 57** [W] ~3m: Baselines and watch list.
```bash
cd ~/tmp/wt/s-move-out && sed -i '/^doom$/d; /^doom-cpu-dump$/d; /^doom-loader$/d; s/^# Spec; what is left is the Doom tools\.$/# Spec./' docs/policy/man-missing-baseline.txt
sed -i '/docker\/services\/doom\//d' docs/policy/hadolint-baseline.tsv docs/policy/trivy-misconfig-baseline.tsv docs/policy/image-tag-baseline.txt
sed -i '/^| github\.com\/unheaded\/doomgeneric | /d' docs/policy/dependency-watch.md
grep -c -i -E 'docker/services/doom|doomgeneric' docs/policy/hadolint-baseline.tsv docs/policy/trivy-misconfig-baseline.tsv docs/policy/image-tag-baseline.txt docs/policy/dependency-watch.md; grep -c -i doom docs/policy/man-missing-baseline.txt
```
  Expect `:0` on all four, then `0`.
- [ ] **Step 58** [W] ~2m: DP-314.
```bash
cd ~/tmp/wt/s-move-out && python3 - <<'PY'
import re
p='docs/adr/ADR-057-unheaded-source-code-indexing.md'; s=open(p).read()
old='- Generated code (where identifiable: `*.pb.go`, `*.gen.go`, `target/debug/build/*/out`)\n'
assert s.count(old)==1
s=s.replace(old,old+'- All Doom code (DP-314, 2026-11-04): it left for `unheaded-doom`, and `doom/doom.*` here is a binary fixture\n'); open(p,'w').write(s); print('ADR-057 OK')
p='docs/sweeps/REAL-STEEL-UNREACHED.tsv'; L=open(p).read().split('\n'); n=0
for i,l in enumerate(L):
    f=l.split('\t')
    if f[0]=='dashboard/js/doom-viewport.js' and f[4]=='PARK':
        f[4]='MOVED'; f[6]+='; moved to unheaded-doom (DP-305)'; L[i]='\t'.join(f); n+=1
open(p,'w').write('\n'.join(L)); print('tsv rows',n)
PY
```
  Expect `ADR-057 OK`, `tsv rows 1`. `raft/scripts/distill_qa.py`'s `doom_docs` source is removed in the forge repo (Phase 4B), since that script moves.
- [ ] **Step 59** [C] ~1m: **PHASE 10 EXIT GATE + COMMIT.** `cd ~/tmp/wt/s-move-out && ~/tmp/wt/bin/plan-commit s-move-out S-MOVE-OUT "Steps 54-59: gates, inventories and baselines follow Doom out (DP-305, DP-314)" config/logging/sources.yaml runbooks/MANIFEST.sha256 deploy/systemd/README.md wiki/Systemd-Units.md docker/Makefile docs/policy/man-missing-baseline.txt docs/policy/hadolint-baseline.tsv docs/policy/trivy-misconfig-baseline.tsv docs/policy/image-tag-baseline.txt docs/policy/dependency-watch.md docs/adr/ADR-057-unheaded-source-code-indexing.md docs/sweeps/REAL-STEEL-UNREACHED.tsv docs/battle-plans/progress/S-MOVE-OUT-progress.md`

## PHASE 10A: scripts that reach root, deletion lists, the firewall line (DP-305)
**Goal**: the same removals in files rule 13 keeps from cheaper agents: `kingdom-shutdown.sh` (`sudo`, ssh to east), `ci-local.sh` (sudo probe), `clean-artifacts.sh` (a deletion list), the Nix host's `allowedTCPPorts`, and two runbooks whose steps run `sudo`. Editing only; nothing here runs them. **Prerequisite**: Phase 10 (same manifest). **Time**: 25m. **Agent**: Opus lane. **Model**: Opus

- [ ] **Step 60** [W] ~4m:
```bash
cd ~/tmp/wt/s-move-out && sed -i '/^    doom-bridge$/d; /^    doom-go-injector$/d; /^    doom-loader$/d' scripts/kingdom-shutdown.sh
sed -i '/^    crates\/doom-runner$/d; /^    crates\/zhenai-forge$/d' scripts/ci-local.sh
sed -i '/^    "doom\/doomgeneric\/doomgeneric\/build_monad"$/d; /^    "scripts\/doom\/__pycache__"$/d; /^    "doom-bridge"$/d; /^    "doom-go-injector"$/d; /^    "doom-loader"$/d; /^    "demos\/doom\/unheaded-doom\.\(elf\|mbc\|rv2mbc\)"$/d; /^    # Doom build artifacts (C object files)$/d' scripts/clean-artifacts.sh
sed -i '/^    \.\.\/\.\.\/modules\/services\/doom\.nix$/d; /^        16680   # DOOM compute port$/d' nixos/hosts/host-a/configuration.nix
sed -i '/^    \.\/doom\.nix  *# DOOM BPF compute substrate$/d' nixos/modules/services/default.nix
sed -i '/^- `doom\.nix` - DOOM BPF compute substrate/d; /^- 16680: doom (TCP compute substrate)$/d' nixos/modules/services/README.md
for f in scripts/kingdom-shutdown.sh scripts/ci-local.sh scripts/clean-artifacts.sh; do bash -n $f || echo BAD $f; done; git grep -c -E 'doom-(bridge|go-injector|loader)|doom-runner' -- scripts/kingdom-shutdown.sh scripts/ci-local.sh scripts/clean-artifacts.sh nixos/hosts; echo rc=$?
```
  Expect `rc=1` and no `BAD` line.
- [ ] **Step 61** [W] ~10m: Two runbooks still drive Doom with sudo: `runbooks/infra/west-startup-order.yaml` (lines 204-209 ring setup and `doom-runner`, 239-243 injector, 312 the stop line) and `runbooks/deploy/full-stack-deploy.yaml` (line 54 ring teardown). Delete those steps and renumber what follows; a runbook that loses its only Doom step keeps the rest. Read each file first: a later step may depend on the ring. Then `./scripts/runbook-manifest.sh --write && ./scripts/check-runbooks.sh | tail -1`. Expect a pass. An `approved` runbook whose body changes loses its approval by design: note it under "For Stevie".
- [ ] **Step 62** [W] ~1m:
```bash
cd ~/tmp/wt/s-move-out && sed -i '/^  └── unheaded-runner    (crates\/doom-runner Rust binary, optional)$/d' nix/yggdrasil/overlay/upc/README.md
sed -i '/^| `unheaded-runner` | `crates\/doom-runner\/` | `cargo deb` |$/d' nix/yggdrasil/repo/README.md
git grep -c 'crates/doom-runner' -- nix; echo rc=$?
```
  Expect `rc=1`.
- [ ] **Step 63** [W] ~3m: `Makefile`'s Doom targets run `sudo`, so they leave here, not in Phase 16.
```bash
cd ~/tmp/wt/s-move-out && python3 ~/tmp/wt/bin/sub.py Makefile <<'SUB'
<<<<
       doom-smoke doom-build-check \
====
>>>>
<<<<
##@ DOOM Testing

doom-smoke: ## Run DOOM full lifecycle smoke test (requires root + BPF)
	sudo ./scripts/doom-smoke.sh

doom-build-check: ## Verify DOOM build artifacts for known bug regressions
	./scripts/doom-build-check.sh

====
>>>>
SUB
git grep -c 'doom-smoke' -- Makefile; echo rc=$?
```
  Expect the `sub.py` line, `rc=1`.
- [ ] **Step 64** [W] ~10m: Residue the gates do not see. `dashboard/index.html`: delete the `doom` tab button, the `<!-- Doom Tab -->` panel and the `.doom-*` CSS; check no JS names a Doom element. `containers/lxd/instances/gateway.yaml`: delete `upstream doom_bridge`, `location /doom/` and the `location /ws/doom/` block.
```bash
cd ~/tmp/wt/s-move-out && sed -i 's#path_beg /dashboard /doom#path_beg /dashboard#' docker/haproxy/edge/haproxy.cfg docker/haproxy/internal/haproxy.cfg && ./scripts/check-haproxy-cfg.sh | tail -1 | cut -c1-80
sed -i '/^- `services\/doom\/Dockerfile` — 16680\/TCP (1GB limit)$/d; s/^- Capacity annotations (e\.g\., doom 1GB limit)$/- Capacity annotations/; /^- 16680: doom (TCP)$/d' docker/INDEX.md
sed -i '/^9\. \*\*docker\/services\/doom\/Dockerfile\*\*/d' docker/MANIFEST.md
sed -i 's/^\( *\)doom lich captain /\1lich captain /; s/^- dashboard-backend, doom$/- dashboard-backend/; /^| doom | 16680 |$/d; /doom\/Dockerfile  *(30 lines)$/d' docker/QUICKSTART.md
sed -i 's/^- `doom`, `lich`, /- `lich`, /' docker/README.md
python3 - <<'PY'
p='wiki/Architecture.md'; s=open(p).read()
old='# System Architecture\n'; assert s.count(old)==1
s=s.replace(old,old+'\n> **2026-11-04:** Doom moved to the `unheaded-doom` repository (DP-305), `doom-bridge` and `scripts/doom-ring.sh` with it. `monad_cpu` and the ring stay; the Doom examples below describe how the fixture in `doom/` runs. See `docs/upc/DOOM-LESSONS.md`.\n')
a='  doom-bridge -- Framebuffer reader, WebSocket frame streaming\n'; assert s.count(a)==1; s=s.replace(a,'')
a=' (Doom execution engine)'; assert s.count(a)==1; s=s.replace(a,''); open(p,'w').write(s)
p='wiki/Wave-10C-Backprop.md'; s=open(p).read()
a='for `crates/zhenai-forge/`.'; assert s.count(a)==1; s=s.replace(a,'for `crates/zhenai-forge/` (now in `unheaded-forge`).'); open(p,'w').write(s)
p='wiki/Runbooks.md'; s=open(p).read()
for a in ('corpus-rebuild, ','raft-training-pipeline, zhen-index-rebuild, '):
    assert s.count(a)==1; s=s.replace(a,'')
open(p,'w').write(s)
p='scripts/well/test-zhen-captures.sh'; s=open(p).read()
for a,b in (("#  10. --with-ingest: cmd/zhen-flywheel's Well store (build tag wellint)\n#      against the same server as app_zhen (WELL_TEST_APP_DSN).\n",''),
            ('test-zhen-captures.sh [--with-ingest] [image]','test-zhen-captures.sh [image]'),
            ('GO=0\nif [[ "${1:-}" == --with-ingest ]]; then GO=1; shift; fi\n','')):
    assert s.count(a)==1; s=s.replace(a,b)
i=s.index('# 10. The Go Well store against this server.\n'); j=s.index('if ((fails > 0)); then'); s=s[:i]+s[j:]; open(p,'w').write(s)
print('RESIDUE-OK')
PY
bash -n scripts/well/test-zhen-captures.sh; git grep -c -i 'doom' -- docker/INDEX.md docker/MANIFEST.md docker/QUICKSTART.md docker/README.md docker/haproxy; echo rc=$?
```
  Expect a pass line, `RESIDUE-OK`, `rc=1`. `docker/MANIFEST.md`'s list skips number 9 afterwards: leave it. `wiki/Architecture.md` keeps its Doom ring prose under the banner: rewriting it is the docs sweep's job (DP-402 Q2). The moved `well_integration_test.go` runs by hand from `unheaded-forge`.
- [ ] **Step 65** [C] ~1m: **PHASE 10A EXIT GATE + COMMIT.** `cd ~/tmp/wt/s-move-out && ~/tmp/wt/bin/plan-commit s-move-out S-MOVE-OUT "Steps 60-65: root scripts, runbooks, deletion lists and the Nix host drop Doom (DP-305)" scripts/kingdom-shutdown.sh scripts/ci-local.sh scripts/clean-artifacts.sh nixos/hosts/host-a/configuration.nix nixos/modules/services/default.nix nixos/modules/services/README.md Makefile dashboard/index.html containers/lxd/instances/gateway.yaml docker/haproxy/edge/haproxy.cfg docker/haproxy/internal/haproxy.cfg docker/INDEX.md docker/MANIFEST.md docker/QUICKSTART.md docker/README.md wiki/Architecture.md wiki/Wave-10C-Backprop.md wiki/Runbooks.md scripts/well/test-zhen-captures.sh runbooks/infra/west-startup-order.yaml runbooks/deploy/full-stack-deploy.yaml runbooks/MANIFEST.sha256 nix/yggdrasil/overlay/upc/README.md nix/yggdrasil/repo/README.md docs/battle-plans/progress/S-MOVE-OUT-progress.md`

## PHASE 11: Doom's lessons stay (DP-305)
**Goal**: Stevie wants the learning retained. One document in this repo that a reader who never saw Doom can use. Judgement: it is a distillation, not a copy.
**Prerequisite**: Phase 1 (sources in the new repo). **Time**: 40m. **Agent**: Librarian lane. **Model**: Opus

- [ ] **Step 66** [R] ~10m: Read in `~/tmp/projects/unheaded-doom/docs/doom/`: `FINDINGS.md`, `README.md`, `ARCHITECTURE.md`, `PERFORMANCE.md`, `STATUS.md`, `DOOM-BRIDGE-ARCHITECTURE.md`; in this repo `wiki/Doom-on-UPC.md`, `wiki/Doom-over-IPv6.md`, `wiki/Performance.md`, `references/phase17-gateD-shipped-2026-07-02.md`.
- [ ] **Step 67** [W] ~20m: Write `docs/upc/DOOM-LESSONS.md` (SPDX header; house style: terse, no LoC, no Age framing). Sections: **What it proved** (a real program on the UPC: frames, E1M1 complete, the computational-generality claim and its limits); **Bugs worth remembering** (PC corruption and the RET-floor hypothesis, marked unverified; banding: PU_CACHE disproven, MBC ST to RAM_MAP; the WAD size and IWAD identity bugs; each as symptom, cause, how it was found); **Verifier lessons** (budgets, what blew them, the load-test-every-variant rule that `upc-regression.sh` keeps); **Performance** (measured numbers only, what limited them, the Python `sendto` ceiling correction); **Method** (debugger first, Marshal oversight against NOP loops, reduced screen size as honest 486 behaviour); **Where everything went** (the repo, `docs/doom` there, the fixture here, the uid and ports held in reserve). Every claim carries its source file in `unheaded-doom`.
- [ ] **Step 68** [W] ~5m: `wiki/Doom-on-UPC.md` and `wiki/Doom-over-IPv6.md`: a banner under each title saying the code lives in `unheaded-doom` and pointing at `docs/upc/DOOM-LESSONS.md`; rewrite `../docs/doom/` links with `sed -i -E 's#\.\./docs/doom/[A-Za-z0-9_.-]*#../docs/upc/DOOM-LESSONS.md#g' wiki/Doom-on-UPC.md wiki/Doom-over-IPv6.md wiki/Performance.md wiki/UPC-Overview.md wiki/UPC-Dream-Ladder.md wiki/Protocol-Specs.md`.
- [ ] **Step 69** [C] ~1m: **PHASE 11 EXIT GATE + COMMIT.** `cd ~/tmp/wt/s-move-out && test -s docs/upc/DOOM-LESSONS.md && grep -c -E '^## (What it proved|Bugs worth remembering|Verifier lessons|Performance|Method|Where everything went)' docs/upc/DOOM-LESSONS.md` prints `6`, then `~/tmp/wt/bin/plan-commit s-move-out S-MOVE-OUT "Steps 66-69: Doom's lessons kept in docs/upc/DOOM-LESSONS.md (DP-305)" docs/upc/DOOM-LESSONS.md wiki/Doom-on-UPC.md wiki/Doom-over-IPv6.md wiki/Performance.md wiki/UPC-Overview.md wiki/UPC-Dream-Ladder.md wiki/Protocol-Specs.md docs/battle-plans/progress/S-MOVE-OUT-progress.md`

## PHASE 12: remove the Kubernetes paths from this repo (DP-17)
**Goal**: the material leaves. `scripts/ci-install-tools.sh` and the two `scripts/lib` files were copied, not moved: they stay. **Prerequisite**: Phase 3 and Phase 4 exit gates. **Time**: 5m. **Agent**: one. **Model**: Haiku

- [ ] **Step 70** [V] ~1m: `cd ~/tmp/wt/s-move-out && ~/tmp/wt/bin/blobs-present ~/tmp/projects/unheaded-k8s ~/tmp/wt/k8s-paths.txt` Expect `blobs <n> missing 0` with `<n>` near `176`.
- [ ] **Step 71** [B] ~1m:
```bash
cd ~/tmp/wt/s-move-out && grep -v -E '^scripts/(ci-install-tools\.sh|lib/)' ~/tmp/wt/k8s-paths.txt | tee /var/tmp/s-move-out-k8s-rm.txt | xargs rm -rf && rm -f pkg/uids/helm_test.go && rmdir docs/labs 2>/dev/null; wc -l < /var/tmp/s-move-out-k8s-rm.txt; ls deploy | tr '\n' ' '
```
  Expect `15`, and `README.md ansible docker east nftables nginx polkit sophia-eye systemd sysusers.d tofu`.
- [ ] **Step 72** [C] ~1m: **PHASE 12 EXIT GATE + COMMIT.** `cd ~/tmp/wt/s-move-out && ~/tmp/wt/bin/plan-commit s-move-out S-MOVE-OUT "Steps 70-72: Kubernetes material leaves for unheaded-k8s (DP-17)" $(cat /var/tmp/s-move-out-k8s-rm.txt) pkg/uids/helm_test.go docs/battle-plans/progress/S-MOVE-OUT-progress.md`

## PHASE 12A: remove the forge paths from this repo (round 4)
**Goal**: everything in `forge-move.txt` gone, the flywheel and the two corpus runbooks included; the copies stay. **Prerequisite**: Phases 4A and 4B exit gates; Phase 10A committed (shared manifest). **Time**: 5m. **Agent**: one. **Model**: Haiku

- [ ] **Step 73** [V] ~1m: `cd ~/tmp/projects/unheaded-forge && git log -1 --format=%s && test -f crates/zhenai-forge/Cargo.toml && test -f raft/kingdom-w12/kingdom.lora.gguf && echo NEW-REPO-OK` Expect `Stand alone: README records the work, why it stopped, and the scrambled LoRAs`, `NEW-REPO-OK`.
- [ ] **Step 74** [V] ~1m: `cd ~/tmp/wt/s-move-out && ~/tmp/wt/bin/blobs-present ~/tmp/projects/unheaded-forge ~/tmp/wt/forge-move.txt` Expect `blobs <n> missing 0`, `<n>` above `168`.
- [ ] **Step 75** [B] ~1m:
```bash
cd ~/tmp/wt/s-move-out && xargs rm -rf < ~/tmp/wt/forge-move.txt && test -f raft/zhen_app.py && test -f raft/scripts/zhen_rag.py && test -f raft/scripts/action_manager.py && ls crates | grep -c zhenai-forge; echo rc=$?; ./scripts/runbook-manifest.sh --write 2>&1 | tail -1 | cut -c1-70; grep -c -E 'raft-training-pipeline|corpus-rebuild|zhen-index-rebuild' runbooks/MANIFEST.sha256; ./scripts/check-runbooks.sh 2>&1 | tail -1 | cut -c1-80; sed -i '/^python scripts\/04_integration_tests\.py$/d' raft/DEMO_CHECKLIST.md; go test ./cmd/zhen-eval/ 2>&1 | tail -1 | cut -c1-50
```
  Expect `0`, `rc=1`, a manifest line, `0`, a pass line, a line starting `ok`. The two corpus runbooks leave in this same step, with the scripts they call.
- [ ] **Step 76** [C] ~1m: **PHASE 12A EXIT GATE + COMMIT.** `cd ~/tmp/wt/s-move-out && ~/tmp/wt/bin/plan-commit s-move-out S-MOVE-OUT "Steps 73-76: forge and training work leave for unheaded-forge (round 4)" $(cat ~/tmp/wt/forge-move.txt) runbooks/MANIFEST.sha256 raft/DEMO_CHECKLIST.md docs/battle-plans/progress/S-MOVE-OUT-progress.md`

## PHASE 13: pkg/uids without manifests (DP-17)
**Goal**: the registry test stops walking trees that left. `TestComposeMatchesRegistry` and `TestHostOnlySysusersPinned` stay. Numbers stay reserved: a uid is never reused. **Found**: with the trees gone `TestManifestsMatchRegistry` fails for every Kubernetes-only name (trial). **Prerequisite**: Phase 12. **Time**: 15m. **Agent**: Go lane. **Model**: Sonnet

- [ ] **Step 77** [V] ~1m: Red. `cd ~/tmp/wt/s-move-out && go test ./pkg/uids/ 2>&1 | grep -c 'no manifest declares it'` Expect a number above `5`.
- [ ] **Step 78** [W] ~3m: Remove the walk.
```bash
cd ~/tmp/wt/s-move-out && python3 - <<'PY'
p='pkg/uids/uids_test.go'; s=open(p).read()
a=s.index('var (\n\t// Both block style'); b=s.index('// hostOnly lists services that run only')
s=s[:a]+s[b:]
a=s.index('// pendingHardening '); b=s.index('// TestHostOnlySysusersPinned:')
s=s[:a]+s[b:]
for imp,use in (('\t"regexp"\n','regexp.'),('\t"strconv"\n','strconv.')):
    if use not in s: s=s.replace(imp,'')
open(p,'w').write(s); print('UIDS-OK')
PY
gofmt -l pkg/uids; go vet ./pkg/uids/ 2>&1 | head -5; go test ./pkg/uids/ 2>&1 | tail -1 | cut -c1-50
```
  Expect `UIDS-OK`, no `gofmt` or `vet` lines, a line starting `ok` (green). A `vet` line about an unused import: delete that import. `composeOnly` and `hostOnly` stay: `compose_test.go` and the sysusers test use them. A helper `compose_test.go` needs from the removed span: `[STUCK]`.
- [ ] **Step 79** [W] ~1m: Say what the registry now is.
```bash
cd ~/tmp/wt/s-move-out && sed -i 's/^\/\/ Armory tier (16740-16759)\. Infrastructure components under deploy\/k8s\/armory\.$/\/\/ Armory tier (16740-16759). Their manifests left for unheaded-k8s (DP-17); the numbers stay reserved./' pkg/uids/uids.go && sed -i 's/^\tVLLM          = 16781$/\tVLLM          = 16781 \/\/ reserved: vLLM is parked (DP-713)/' pkg/uids/uids.go && gofmt -l pkg/uids; go test ./pkg/uids/ 2>&1 | tail -1 | cut -c1-50; git grep -c 'deploy/k8s' -- pkg/uids; echo rc=$?
```
  Expect a line starting `ok`, `rc=1`.
- [ ] **Step 80** [C] ~1m: **PHASE 13 EXIT GATE + COMMIT.** `cd ~/tmp/wt/s-move-out && ~/tmp/wt/bin/plan-commit s-move-out S-MOVE-OUT "Steps 77-80: pkg/uids stops walking manifests that left (DP-17)" pkg/uids/uids_test.go pkg/uids/uids.go docs/battle-plans/progress/S-MOVE-OUT-progress.md`

## PHASE 14: gates, pins, trivy (DP-17)
**Goal**: no gate names a path that left; helm and kubeconform leave the pins; shrink-only files shrink. **Prerequisite**: Phases 8 and 12. **Time**: 25m. **Agent**: gates lane. **Model**: Sonnet

- [ ] **Step 81** [W] ~4m: Meta-gate.
```bash
cd ~/tmp/wt/s-move-out && python3 ~/tmp/wt/bin/sub.py scripts/check-gates-can-fail.sh <<'SUB'
<<<<
    local f="deploy/k8s/policies/.meta-gate-probe.yaml"
====
    local f="configs/.meta-gate-probe.yaml"
>>>>
<<<<
# shellcheck disable=SC2317  # invoked indirectly via REGISTRY dispatch
provoke_k8s_well_initdb() {
    # Contract (ADR-101): the ConfigMap is db/, rendered. Edit a migration
    # without re-rendering.
    local f="db/migrations/018_config_bindings_profile_index.sql"
    backup "${f}"
    printf -- '-- edited without re-rendering\n' >> "${REPO_ROOT}/${f}"
}

====
>>>>
<<<<
# shellcheck disable=SC2317  # invoked indirectly via REGISTRY dispatch
provoke_helm_security() {
    # Contract: every container the chart renders has a read-only root
    # filesystem. Flip the chart-wide default.
    local f="helm/unheaded/values.yaml"
    backup "${f}"
    sed -i 's/readOnlyRootFilesystem: true/readOnlyRootFilesystem: false/' "${REPO_ROOT}/${f}"
}

====
>>>>
SUB
sed -i '/^check-k8s-well-initdb|provoke_k8s_well_initdb|/d; /^check-helm-security|provoke_helm_security|/d; s#^ci-install-tools|installs pinned syft/grype/cosign/helm/kubeconform for CI jobs#ci-install-tools|installs pinned syft/grype/cosign for CI jobs#' scripts/check-gates-can-fail.sh
bash -n scripts/check-gates-can-fail.sh && git grep -c -E 'k8s|helm|kubeconform' -- scripts/check-gates-can-fail.sh; echo rc=$?
```
  Expect `sub.py: scripts/check-gates-can-fail.sh: 3 block(s) applied`, `rc=1`.
- [ ] **Step 82** [W] ~4m: Pins.
```bash
cd ~/tmp/wt/s-move-out && sed -i '/^HELM_VERSION=/d; /^HELM_SHA256=/d; /^KUBECONFORM_VERSION=/d; /^KUBECONFORM_SHA256=/d' scripts/lib/tool-pins.sh
sed -i '/^\t"helm|scripts\/lib\/tool-pins\.sh|HELM_VERSION|helm\/helm|v"$/d; /^\t"kubeconform|scripts\/lib\/tool-pins\.sh|KUBECONFORM_VERSION|yannh\/kubeconform|v"$/d' scripts/check-tool-freshness.sh
python3 - <<'PY'
import re
p='scripts/ci-install-tools.sh'; s=open(p).read()
s,n1=re.subn(r'^\thelm\) bin=.*?"\$HELM_SHA256" linux-amd64/helm\)" ;;\n','',s,flags=re.S|re.M)
s,n2=re.subn(r'^\tkubeconform\) bin=.*?"\$KUBECONFORM_SHA256" kubeconform\)" ;;\n','',s,flags=re.S|re.M)
s=s.replace('syft|grype|cosign|helm|kubeconform...','syft|grype|cosign...')
s=s.replace("# token), azure/setup-helm at Helm 3.14.0, and kubeconform from\n# releases/latest with no checksum. Versions","# token). Versions")
open(p,'w').write(s); print('cases removed',n1+n2)
PY
bash -n scripts/ci-install-tools.sh && bash -n scripts/check-tool-freshness.sh && git grep -c -i -E 'helm|kubeconform' -- scripts/ci-install-tools.sh scripts/lib/tool-pins.sh scripts/check-tool-freshness.sh; echo rc=$?
```
  Expect `cases removed 2`, `rc=1`.
- [ ] **Step 83** [W] ~3m: CI.
```bash
cd ~/tmp/wt/s-move-out && python3 ~/tmp/wt/bin/sub.py .github/workflows/security.yml <<'SUB'
<<<<
      # ADR-101: the Kubernetes Well's initdb ConfigMap is rendered from db/,
      # so a migration added for compose reaches Kubernetes too.
      - name: Kubernetes Well initdb matches db/
        run: ./scripts/check-k8s-well-initdb.sh

====
>>>>
SUB
sed -i '/^    - "deploy\/k8s\/\*\*"$/d' .github/workflows/plan.yml
sed -i 's/^# Step 421)\. Same pattern as check-k8s-well-initdb\.sh\.$/# Step 421)./' scripts/check-kanban-genesis.sh
for f in security plan; do python3 -c "import yaml;yaml.safe_load(open('.github/workflows/$f.yml'))" && echo ok-$f; done
```
  Expect the `sub.py` line, `ok-security`, `ok-plan`.
- [ ] **Step 84** [W] ~5m: `.trivyignore.yaml` shrinks; entries with no path left go whole.
```bash
cd ~/tmp/wt/s-move-out && python3 - <<'PY'
import re, yaml
p='.trivyignore.yaml'; L=open(p).read().split('\n')
gone=re.compile(r'^\s+- "(kubernetes/|deploy/k8s|helm/)')
starts=[i for i,l in enumerate(L) if l.startswith('  - id:')]
head=L[:starts[0]]; blocks=[]
# A block is its leading comments (taken from the tail of the one before) plus its body.
bounds=starts+[len(L)]
for k,s in enumerate(starts):
    e=bounds[k+1]; body=L[s:e]
    tail=[]
    while k+1<len(starts) and body and (body[-1].startswith('  #') or body[-1]==''):
        tail.insert(0,body.pop())
    blocks.append((body,tail))
out=list(head); kept=0; dropped=0; pend=[]
for body,tail in blocks:
    new=[l for l in body if not gone.match(l)]
    paths=[l for l in new if re.match(r'^\s+- "',l)]
    had=any(re.match(r'^\s+- "',l) for l in body)
    if had and not paths:
        dropped+=1; pend=tail; continue
    kept+=1; out+=pend+new; pend=tail
out+=pend
open(p,'w').write('\n'.join(out))
d=yaml.safe_load(open(p)); print('kept',kept,'dropped',dropped,'yaml-ok',isinstance(d,dict))
PY
grep -c -E '"(kubernetes/|deploy/k8s|helm/)' .trivyignore.yaml; echo rc=$?
```
  Expect a `kept N dropped M yaml-ok True` line with `M` above `0`, then `0`, `rc=1`.
- [ ] **Step 85** [W] ~2m: Baselines.
```bash
cd ~/tmp/wt/s-move-out && sed -i '/\tkubernetes\//d; /\tdeploy\/k8s\//d; /\thelm\//d; /\tdeploy\/argocd\//d; /^KSV-00\(39\|40\)\tdeploy\/sophia-eye\/shield-rules\.yaml\t/d; /^KSV-00\(39\|40\)\treferences\/baseline\/mjolnir\.example\.yaml\t/d' docs/policy/trivy-misconfig-baseline.tsv
sed -i '/^kubernetes\//d; /^deploy\/k8s\//d; /^helm\//d' docs/policy/image-tag-baseline.txt docs/policy/pq-kex-baseline.txt
grep -c -E 'kubernetes/|deploy/k8s|helm/' docs/policy/trivy-misconfig-baseline.tsv docs/policy/image-tag-baseline.txt docs/policy/pq-kex-baseline.txt; ./scripts/check-pq-kex.sh 2>&1 | tail -1 | cut -c1-90; ./scripts/check-image-tags.sh 2>&1 | tail -1 | cut -c1-90; ./scripts/check-manifest-yaml.sh 2>&1 | tail -1 | cut -c1-90
```
  Expect three `:0` lines and three pass lines. These gates read `git ls-files`: run them after Phase 12's commit. Rows removed: the moved paths, plus KSV-0039/0040 on `deploy/sophia-eye/shield-rules.yaml` and `references/baseline/mjolnir.example.yaml`, which those rules matched only as Kubernetes kinds while `.trivyignore.yaml`'s entries stood. Run `./scripts/check-trivy-misconfig.sh` where the pinned trivy is available; measured with 0.72, the pin is 0.76.1, so a mismatch goes under "For Stevie".
- [ ] **Step 86** [C] ~1m: **PHASE 14 EXIT GATE + COMMIT.** `cd ~/tmp/wt/s-move-out && ~/tmp/wt/bin/plan-commit s-move-out S-MOVE-OUT "Steps 81-86: gates, pins and trivy entries follow Kubernetes out (DP-17)" scripts/check-gates-can-fail.sh scripts/lib/tool-pins.sh scripts/check-tool-freshness.sh scripts/ci-install-tools.sh .github/workflows/security.yml .github/workflows/plan.yml scripts/check-kanban-genesis.sh .trivyignore.yaml docs/policy/trivy-misconfig-baseline.tsv docs/policy/image-tag-baseline.txt docs/policy/pq-kex-baseline.txt docs/battle-plans/progress/S-MOVE-OUT-progress.md`

## PHASE 15: Kubernetes docs and decisions (DP-17)
**Goal**: the living docs say a lab component in its own repository. The eleven parked rows close. **Stays**: `pkg/iac/kubernetes.go` and the IaC tables in CLAUDE.md and `wiki/IaC-Backends.md` (a renderer for users); ADR-040 and ADR-047 bodies. **Prerequisite**: Phase 12. **Time**: 15m. **Agent**: docs lane. **Model**: Haiku

- [ ] **Step 87** [W] ~4m: Lines.
```bash
cd ~/tmp/wt/s-move-out && sed -i 's/^| `kubernetes\/`, `helm\/` | K8s manifests and chart |$/| (moved) | K8s manifests and chart live in `unheaded-k8s` (DP-17) |/' wiki/Project-Structure.md
sed -i '/^- \*\*On Kubernetes\*\* (`kubernetes\/manifests\/base\/the-well\/`) the same: /d' wiki/The-Well.md
sed -i 's#, or `helm/unheaded/` for the cluster deployment)\.#; `unheaded-k8s` holds a chart for a lab cluster).#' cmd/tools/mimir/BUILD.md
sed -i 's/ Also a Dockerfile stage and a DaemonSet[^|]*|$/ Also a Dockerfile stage. |/' wiki/Service-KEV-Poller.md
git grep -n -E '`?(kubernetes/|helm/|deploy/k8s|deploy/argocd)' -- wiki README.md QUICKSTART.md VISION.md cmd/tools | cut -c1-110
```
  Expect no output. A remaining line: record it in the progress file for Phase 16.
- [ ] **Step 88** [W] ~3m: Decisions recorded where they were parked.
```bash
cd ~/tmp/wt/s-move-out && python3 - <<'PY'
note='> **2026-11-04 (DP-17):** Kubernetes is a lab component in its own repository, `unheaded-k8s`; not a product target. The platform scales from one host to many without it. The manifests, chart and lab named here moved there with their history.'
for p in ('docs/adr/ADR-047-k8s-honest-assessment-and-extractable-tools.md','docs/adr/ADR-055-kev-poller-always-on-service.md','docs/adr/ADR-088-kubernetes-service-orchestration.md'):
    s=open(p).read(); i=s.index('\n',s.index('# ADR-')); s=s[:i+1]+'\n'+note+'\n'+s[i+1:]; open(p,'w').write(s)
p='docs/sweeps/OUTSTANDING-2026-10-27.md'; s=open(p).read()
old='43. Is Kubernetes still a deploy target? Decides CG-24/SP-08/SP-23/B-17/K-01..06/A-4 (all parked in §4 until answered). Default: lab-only, not a target.'
assert s.count(old)==1
s=s.replace(old,old+' **Answered 2026-11-04 (DP-17): lab component in `unheaded-k8s`; the eleven rows close as lab only.**'); open(p,'w').write(s); print('DOCS-OK')
PY
```
  Expect `DOCS-OK`.
- [ ] **Step 89** [C] ~1m: **PHASE 15 EXIT GATE + COMMIT.** `cd ~/tmp/wt/s-move-out && ~/tmp/wt/bin/plan-commit s-move-out S-MOVE-OUT "Steps 87-89: docs and ADRs say lab component, own repository (DP-17)" wiki/Project-Structure.md wiki/The-Well.md cmd/tools/mimir/BUILD.md wiki/Service-KEV-Poller.md docs/adr/ADR-047-k8s-honest-assessment-and-extractable-tools.md docs/adr/ADR-055-kev-poller-always-on-service.md docs/adr/ADR-088-kubernetes-service-orchestration.md docs/sweeps/OUTSTANDING-2026-10-27.md docs/battle-plans/progress/S-MOVE-OUT-progress.md`

## PHASE 15A: forge ripple (round 4)
**Goal**: gates and docs follow; the running Zhen is untouched. **Prerequisite**: Phase 12A. **Time**: 15m. **Agent**: docs lane. **Model**: Haiku

- [ ] **Step 90** [W] ~3m: Gates and comments.
```bash
cd ~/tmp/wt/s-move-out && python3 ~/tmp/wt/bin/sub.py scripts/check-clippy.sh <<'SUB'
<<<<
# crates/zhenai-forge's build.rs compiles HIP kernels with hipcc (ROCm), which
# CI runners do not have; without this its build script panics and the
# workspace reads as "did not build". Clippy never links, so it needs no
# kernels. See the build.rs for the switch.
export ZHENAI_FORGE_SKIP_KERNELS=1

====
>>>>
SUB
sed -i 's# crates/upc-bootstub crates/zhenai-forge ebpf; do$# crates/upc-bootstub ebpf; do#' scripts/kingdom-health.sh
sed -i 's#^// that changes what is loaded on the GPU (scripts/switch-model.sh,$#// that changes what is loaded on the GPU (scripts/switch-model.sh, and#; s#^// scripts/forge-train.sh) takes an exclusive lease for its whole run. The$#// forge-train.sh in unheaded-forge) takes an exclusive lease for its whole run. The#' pkg/gpulease/gpulease.go
python3 - <<'PY'
import re
p='scripts/lib/gpulease_scripts_test.go'; s=open(p).read()
s2,n=re.subn(r'(^//[^\n]*\n)*^func TestForgeTrain_WaitsForTheLease\(t \*testing\.T\) \{\n.*?^\}\n\n?','',s,count=1,flags=re.S|re.M)
open(p,'w').write(s2); print('lease tests removed',n)
PY
bash -n scripts/check-clippy.sh && bash -n scripts/kingdom-health.sh && gofmt -l pkg/gpulease scripts/lib; go vet ./scripts/lib/ 2>&1 | tail -2; go test ./scripts/lib/ 2>&1 | tail -1 | cut -c1-50; git grep -c 'zhenai-forge' -- scripts/check-clippy.sh scripts/kingdom-health.sh; echo rc=$?
```
  Expect the `sub.py` line, `lease tests removed 1`, a line starting `ok`, `rc=1`. A `vet` line for an import the test alone used: delete that import. `go test` in CI runs this package, so it must stay green. The copy is in the forge's `reference/`.
- [ ] **Step 91** [W] ~4m: Docs.
```bash
cd ~/tmp/wt/s-move-out && sed -i 's/, zhend, zhenai-forge |$/, zhend |/' wiki/Project-Structure.md
sed -i 's#^| `scripts/forge-train.sh` | exclusive, for the whole run:#| `scripts/forge-train.sh` (in `unheaded-forge` since 2026-11-04) | exclusive, for the whole run:#' wiki/Zhen-Agents.md
sed -i '/^| `zhenai-forge` | LoRA fine-tuning for GGUF models on CPU\/GPU (`ForgeBackend` trait, ADR-048) (\[\[Zhen-AI\]\]) |$/d' wiki/Rust-Components.md
sed -i 's#^- \*\*Training (zhenai-forge):\*\* Rust LoRA trainer with GPU kernels, on \*\*Gemma 4 E2B\*\* (fits west.s VRAM)\.#- **Training (back burner):** the Rust LoRA trainer moved to `unheaded-forge` 2026-11-04: training is unworkable on west.#; s#^| Forge (training) | `crates/zhenai-forge/` (ADR-048 backend trait) |$#| Forge (training) | `unheaded-forge` repository (ADR-048 backend trait) |#' wiki/Zhen-AI.md
python3 - <<'PY'
p='wiki/Zhen-Training-Data.md'; s=open(p).read()
old='# Zhen Training Data\n'; assert s.count(old)==1
s=s.replace(old,old+'\n> **2026-11-04 (rounds 4 and 5):** `zhen-flywheel`, the forge and the tokenizers moved to `unheaded-forge`, which keeps a copy of this page. Captures stay off. The capture tables stay in the Well; `pkg/zhencapture` stays here because `zhen-claude`, `zhen-mcp`, `well-retention` and `claudebridge` use it.\n'); open(p,'w').write(s)
p='docs/architecture/zhen-capture.md'; s=open(p).read()
old='`cmd/zhen-flywheel` (`wiki/Zhen-Training-Data.md`).'; assert s.count(old)==1
s=s.replace(old,'`zhen-flywheel`, in the `unheaded-forge` repository since 2026-11-04 (`wiki/Zhen-Training-Data.md`).'); open(p,'w').write(s)
note='> **2026-11-04 (round 5):** `zhen-flywheel` moved to `unheaded-forge`. Captures stay off. Champion\'s audit trail (`llm_requests`, `zhen_actions`, Anamnesis) stays here.'
p='docs/compliance/privacy/dpia-ebpf-and-zhen-capture.md'; s=open(p).read()
old='`zhen-flywheel ingest` re-scans it'; assert s.count(old)==1
s=s.replace(old,'`zhen-flywheel ingest` (in `unheaded-forge` since 2026-11-04; capture stays off) re-scans it'); open(p,'w').write(s)
p='pkg/champion/runcheck.go'; s=open(p).read()
old='(zhen-flywheel label reads it)'; assert s.count(old)==1
s=s.replace(old,'(zhen-flywheel label, in unheaded-forge, reads it)'); open(p,'w').write(s)
for p in ('docs/adr/ADR-106-two-brains-one-gate.md','docs/adr/ADR-104-zhenai-claude-api-bridge.md','docs/adr/ADR-019-zhen-champion-agent.md'):
    s=open(p).read(); i=s.index('\n',s.index('# ADR-')); s=s[:i+1]+'\n'+note+'\n'+s[i+1:]; open(p,'w').write(s)
p='docs/adr/ADR-024-zhen-runbook-automation.md'; s=open(p).read(); i=s.index('\n',s.index('# ADR-'))
s=s[:i+1]+'\n> **2026-11-04 (round 5):** `corpus-rebuild`, `zhen-index-rebuild` and `raft-training-pipeline` moved to `unheaded-forge` with the scripts they call.\n'+s[i+1:]; open(p,'w').write(s)
print('NOTES-OK')
PY
python3 - <<'PY'
p='docs/adr/ADR-030-zhenai-forge-rust-training.md'; s=open(p).read()
i=s.index('\n',s.index('# ADR-030'))
s=s[:i+1]+'\n> **2026-11-04 (round 4):** the forge moved to `unheaded-forge` with its history, tests included. Training is on the back burner until a machine with enough GPU exists.\n'+s[i+1:]; open(p,'w').write(s); print('ADR-030 OK')
PY
git grep -c 'crates/zhenai-forge' -- wiki/Zhen-AI.md wiki/Project-Structure.md wiki/Rust-Components.md; echo rc=$?
```
  Expect `NOTES-OK`, `ADR-030 OK`, `rc=1`.
- [ ] **Step 92** [C] ~1m: **PHASE 15A EXIT GATE + COMMIT.** `cd ~/tmp/wt/s-move-out && ~/tmp/wt/bin/plan-commit s-move-out S-MOVE-OUT "Steps 90-92: gates and docs follow the forge out (round 4)" wiki/Zhen-Agents.md scripts/check-clippy.sh scripts/kingdom-health.sh scripts/lib/gpulease_scripts_test.go pkg/gpulease/gpulease.go wiki/Project-Structure.md wiki/Rust-Components.md wiki/Zhen-AI.md wiki/Zhen-Training-Data.md docs/architecture/zhen-capture.md docs/compliance/privacy/dpia-ebpf-and-zhen-capture.md pkg/champion/runcheck.go docs/adr/ADR-030-zhenai-forge-rust-training.md docs/adr/ADR-106-two-brains-one-gate.md docs/adr/ADR-104-zhenai-claude-api-bridge.md docs/adr/ADR-019-zhen-champion-agent.md docs/adr/ADR-024-zhen-runbook-automation.md docs/battle-plans/progress/S-MOVE-OUT-progress.md`

## PHASE 16: shared ledgers (DP-305, DP-17)
**Goal**: every file several answers touch, once. **Prerequisite**: Phases 7-15A committed or `[STUCK]`. **Time**: 25m. **Agent**: one. **Model**: Haiku

- [ ] **Step 93** [W] ~3m: `Makefile`.
```bash
cd ~/tmp/wt/s-move-out && python3 ~/tmp/wt/bin/sub.py Makefile <<'SUB'
<<<<
test-rust-doom-runner: ## Run doom-runner tests (Aya BPF runtime for Doom-on-Monad)
	cd crates/doom-runner && cargo test --locked

====
>>>>
SUB
sed -i 's/ test-rust-doom-runner//; s/ --ignore github\.com\/unheaded\/doomgeneric//' Makefile
git grep -n -E 'doom-runner|doom-smoke|doomgeneric' -- Makefile | cut -c1-90; make -n help >/dev/null && echo MAKE-OK
```
  Expect `sub.py: Makefile: 1 block(s) applied`, no grep lines, `MAKE-OK`. `ebpf-monad-cpu-doom` stays: it builds the UPC object.
- [ ] **Step 94** [W] ~6m: `CLAUDE.md`.
```bash
cd ~/tmp/wt/s-move-out && python3 - <<'PY'
p='CLAUDE.md'; s=open(p).read()
a=s.index('### Doom-over-IPv6 — ALL DOCS IN `docs/doom/`'); b=s.index('- Kingdom startup/shutdown services, host metrics storage')
new=('### Doom\n\nDoom lives in `unheaded-doom` since 2026-11-04 (DP-305): runner, demos, docs, runbooks, units, `doom-ctl`, the doomgeneric port. '
     'This repo keeps `doom/doom.mbc`, `doom/doom.rv2mbc` and its sha256 as the UPC regression fixture (`scripts/upc-regression.sh`, `crates/monad-mbc`). '
     'Lessons: `docs/upc/DOOM-LESSONS.md`. UPC specs: `docs/upc/`. Doom code is out of the retrieval index (DP-314).\n\n')
s=s[:a]+new+s[b:]
for old,rep in (
  ('- doom-runner (crates/doom-runner/): Aya-based Rust runtime replacing shell/Go loader chain\n',''),
  ('- **Doom**: Doom stays in the repo (decision 50, 2026-10-27); lowest-priority track.\n','- **Doom**: moved to `unheaded-doom` 2026-11-04 (DP-305); lowest-priority track.\n- **Kubernetes**: a lab component in `unheaded-k8s` (DP-17); not a product target. `pkg/iac` still renders manifests for users.\n- **Forge**: `zhenai-forge` and all training work moved to `unheaded-forge` 2026-11-04 (round 4): unworkable on west, back burner until a machine with enough GPU. The WAVE and Learning Gate paragraphs in this file are history; serving is llama-server.\n'),
  ('`zhen-flywheel` ingests and exports training data with Claude text off by default;','`zhen-flywheel` ingested and exported training data with Claude text off (moved to `unheaded-forge` 2026-11-04; captures stay off);'),
):
    assert s.count(old)==1, old
    s=s.replace(old,rep)
open(p,'w').write(s); print('CLAUDE-OK')
PY
python3 - <<'PY'
import collections, pathlib, re
c=collections.Counter(q.parts[1] for q in pathlib.Path('runbooks').rglob('*.yaml'))
cats=', '.join(f'{k} {v}' for k,v in sorted(c.items(), key=lambda kv:(-kv[1],kv[0])))
n=sum(c.values())
for p,pat,new in (('CLAUDE.md', r'^- \d+ Operational Runbooks \(runbooks/\): \d+ categories \([^)]*\)', f'- {n} Operational Runbooks (runbooks/): {len(c)} categories ({cats})'),
                  ('wiki/Runbooks.md', r'^\d+ runbooks\.', f'{n} runbooks.')):
    s=open(p).read(); s,k=re.subn(pat,new,s,count=1,flags=re.M); assert k==1,p; open(p,'w').write(s)
print('RUNBOOKS', n)
PY
git grep -c -E 'crates/doom-runner|docs/doom/' -- CLAUDE.md; echo rc=$?
```
  Expect `CLAUDE-OK` and `RUNBOOKS <n>`, with `<n>` near `88` (the count settles here, after every removal has landed); a count here means ASCEND-LINUX history lines remain: run `sed -i 's#docs/doom/UPC#docs/upc/UPC#g' CLAUDE.md` and record what is left.
- [ ] **Step 95** [W] ~2m: `README.md`.
```bash
cd ~/tmp/wt/s-move-out && python3 ~/tmp/wt/bin/sub.py README.md <<'SUB'
<<<<
- **Doom** — runs; playable in a browser via `doom-runner`.
====
- **Doom** — runs; lives in its own repository, `unheaded-doom`.
>>>>
<<<<
`crates/zhend/` — anti-fragile gossip knowledge substrate, PQC-secured. `crates/zhenai-forge/`
— LoRA fine-tuning on Gemma-4 / Mistral-7B via ROCm. Flask web UI on port 20103 over a
1.52M-vector RAG corpus.
====
`crates/zhend/` — knowledge store, single-node gRPC (DP-209). Training moved to `unheaded-forge`.
Flask web UI on port 20103 over `vor` retrieval.
>>>>
<<<<
Code: `ebpf/monad-cpu-ebpf/` (interpreter), `crates/xv6-mbc/`, `crates/doom-runner/`,
`cmd/upc-bootctl/`. Docs: [`wiki/UPC-Overview.md`](wiki/UPC-Overview.md),
====
Code: `ebpf/monad-cpu-ebpf/` (interpreter), `crates/xv6-mbc/`,
`cmd/upc-bootctl/`. Docs: [`wiki/UPC-Overview.md`](wiki/UPC-Overview.md),
>>>>
SUB
```
  Expect `sub.py: README.md: 3 block(s) applied`.
- [ ] **Step 96** [W] ~6m: `THIRD_PARTY.md`.
```bash
cd ~/tmp/wt/s-move-out && python3 - <<'PY'
import re
p='THIRD_PARTY.md'; s=open(p).read()
s,n=re.subn(r'^\| `doom/doomgeneric/[^\n]*\n','',s,flags=re.M)
s=s.replace('- `/doom/doomgeneric/LICENSE` — GPL v2 full text (upstream)\n','')
for pat in (r'^\| `cmd/doom[^\n]*\n',r'^\| `doom/doom1\.wad`[^\n]*\n',r'^\| `dashboard/doom\.html`[^\n]*\n',r'^\| `dashboard/js/doom-viewport\.js`[^\n]*\n',r'^\| `scripts/doom-\*[^\n]*\n',r'^\| `configs/doom\.toml`[^\n]*\n'):
    s,c=re.subn(pat,'',s,flags=re.M); n+=c
old='|  cmd/doom-bridge   cmd/doom-loader               |\n|  cmd/doom-go-injector   internal/bpf*            |\n'
assert s.count(old)==1
s=s.replace(old,'|  cmd/upc-bootctl                                 |\n')
s=s.replace('### GPL v2 Licensed Components (inside the boundary)\n','### GPL v2 Licensed Components (inside the boundary)\n\nThe Doom sources, tools and WAD handling moved to `unheaded-doom` on 2026-11-04 (DP-305). What stays is the compiled fixture.\n')
open(p,'w').write(s); print('rows removed',n)
PY
sed -i 's#The doomgeneric code lives in `doom/doomgeneric/`$#The doomgeneric code lives in the `unheaded-doom` repository;#; s#^as a git submodule\. It is compiled to MBC bytecode#it is compiled to MBC bytecode#' LICENSES/THIRD_PARTY.md
python3 - <<'PY'
p='.golangci.yml'; s=open(p).read()
a='''      # cmd/doom-bridge/bpf.go's type-alias comment (ST1021) is the one
      # doc-comment finding left when ST1020-ST1022 were enabled
      # (S-TRUE-WIRE Step 371): Doom work is the lowest-priority track and
      # was out of that change's scope. Fix the comment and drop this rule
      # the next time cmd/doom-bridge is touched.
      - path: cmd/doom-bridge/
        linters: [staticcheck]
        text: "ST1021:"

'''
b='        # ratcheted. One remains excluded by path below (cmd/doom-bridge).\n'
assert s.count(a)==1 and s.count(b)==1
s=s.replace(a,'').replace(b,'        # ratcheted.\n'); open(p,'w').write(s); print('GOLANGCI-OK')
PY
grep -c -E 'doomgeneric/|cmd/doom' THIRD_PARTY.md; git grep -n -E '`doom/doom\.(mbc|rv2mbc)`' -- THIRD_PARTY.md | wc -l
```
  Expect `rows removed 18`, `GOLANGCI-OK`, then `0`, `2`. The two fixture rows stay; `doom/LICENSE` stays.
- [ ] **Step 97** [W] ~2m: `sbom/SBOM.md` is hand-kept, so `generate-sbom.sh` does not refresh it.
```bash
cd ~/tmp/wt/s-move-out && sed -i '/^| GPL-2\.0 | 1 | unheaded\/doomgeneric (ISOLATED -- see GPL Boundary) |$/d; /^| github\.com\/unheaded\/doomgeneric | v0\.0\.0 | GPL-2\.0 |$/d' sbom/SBOM.md
sed -i 's/^| GPL-2\.0 | 1 | 0 | 0 | 2 |/| GPL-2.0 | 0 | 0 | 0 | 2 |/' sbom/SBOM.md
python3 - <<'PY'
p='sbom/SBOM.md'; s=open(p).read()
old=('The GPL boundary is clean. The only GPL-licensed dependency in the Go module graph\n'
     'is `github.com/unheaded/doomgeneric`, which is the DOOM engine used for computational\n'
     'completeness proof (DOOM-on-Monad). This dependency is architecturally isolated:\n')
new=('The GPL boundary is clean. The Go module graph holds no GPL-licensed dependency: the\n'
     'DOOM engine left for `unheaded-doom` on 2026-11-04 (DP-305). Only the compiled fixture\n'
     'under `doom/` is GPL-2.0, and it is architecturally isolated:\n')
assert s.count(old)==1; open(p,'w').write(s.replace(old,new)); print('SBOM-OK')
PY
grep -c doomgeneric sbom/SBOM.md; grep -c '^| GPL-2\.0 | 0 | 0 | 0 | 2 |' sbom/SBOM.md
```
  Expect `SBOM-OK`, `0`, `1`. The module left `go.mod` on this plan's branch. Any other output: mark `[STUCK]`.
- [ ] **Step 98** [W] ~1m: Inventories.
```bash
cd ~/tmp/wt/s-move-out && ./scripts/live-path-inventory.sh --report > docs/LIVE-PATHS.md && ./scripts/live-path-inventory.sh --check | tail -1
sed -i 's#^| `doom/` | .*#| `upc/` | UPC specs and ports, and Doom'"'"'s lessons (`DOOM-LESSONS.md`) |#' docs/README.md
git grep -c -E 'cmd/doom|zhen-flywheel' -- docs/LIVE-PATHS.md; echo rc=$?; grep -c '^| `upc/` |' docs/README.md
```
  Expect a PASS line, `rc=1`, `1`. The report regenerates rows and totals; a hand `sed` leaves the totals wrong.
- [ ] **Step 99** [C] ~1m: **PHASE 16 EXIT GATE + COMMIT.** `cd ~/tmp/wt/s-move-out && ~/tmp/wt/bin/plan-commit s-move-out S-MOVE-OUT "Steps 93-99: shared ledgers follow both moves" Makefile CLAUDE.md wiki/Runbooks.md README.md THIRD_PARTY.md LICENSES/THIRD_PARTY.md .golangci.yml sbom/SBOM.md docs/LIVE-PATHS.md docs/README.md docs/battle-plans/progress/S-MOVE-OUT-progress.md`

## PHASE 17: regenerate (DP-305, DP-17)
**Goal**: SBOM and CBOM match the tree. **Prerequisite**: Phase 16. **Time**: 20m. **Agent**: one. **Model**: Sonnet

- [ ] **Step 100** [B] ~3m: `cd ~/tmp/wt/s-move-out && go mod tidy && git status --short go.mod go.sum | wc -l && ./scripts/check-netip.sh | tail -1` Expect `0` and `OK: <n> uses (baseline <b>)`. Rule 17: patch the one number with `sed -i "s/^<b>$/<n>/" docs/policy/netip-baseline.txt`; old and new under "For Stevie".
- [ ] **Step 101** [B] ~8m: `cd ~/tmp/wt/s-move-out && ./scripts/generate-sbom.sh 2>&1 | tail -1 && ./scripts/check-sbom-trend.sh` Expect three `OK` lines with `go_components` and `rust_components` lower (doomgeneric; doom-runner's and the forge's trees). Rule 17: `sed` only those two rows of `docs/policy/sbom-count-baseline.txt` to the printed counts; leave `python_components`; old and new under "For Stevie".
- [ ] **Step 102** [W] ~2m: `cd ~/tmp/wt/s-move-out && ./scripts/check-dependency-baseline.sh 2>&1 | tail -12` lists rows `gone from the build but still in the baseline` (at forge time `ryu`, `serde_path_to_error`, `serde_urlencoded`; the forge's crates add more). Rule 17: `sed` out exactly those rows of `docs/policy/dependency-baseline.txt`, then run the gate again. Expect `[PASS]`.
- [ ] **Step 103** [B] ~3m: `cd ~/tmp/wt/s-move-out && go run ./cmd/crypto-inventory scan --out compliance/crypto/cbom.cdx.json && ./scripts/check-crypto-inventory.sh 2>&1 | tail -1 | cut -c1-90; ./scripts/check-pq-kex.sh | tail -1 | cut -c1-90; ./scripts/check-hadolint.sh 2>&1 | tail -1 | cut -c1-90` Expect three pass lines.
- [ ] **Step 104** [B] ~2m: `cd ~/tmp/wt/s-move-out && make -n build-all >/dev/null && make build-all 2>&1 | tail -1 | cut -c1-70 && ./scripts/gen-man.sh --write 2>&1 | tail -1 | cut -c1-80; ./scripts/gen-man.sh 2>&1 | tail -1 | cut -c1-80; rm -f man/test_batch.1; git status --short man | head -6` Expect a pass; the default mode only checks, so `--write` comes first. `man/test_batch.1` is orphaned by Phase 6. If `make build-all` is not a target, use `make build` and record it.
- [ ] **Step 105** [C] ~1m: **PHASE 17 EXIT GATE + COMMIT.** Name each file `git status --short` lists: `cd ~/tmp/wt/s-move-out && ~/tmp/wt/bin/plan-commit s-move-out S-MOVE-OUT "Steps 100-105: SBOM, CBOM and baselines record both moves" docs/policy/netip-baseline.txt docs/policy/sbom-count-baseline.txt docs/policy/dependency-baseline.txt compliance/crypto/cbom.cdx.json sbom/go-dependencies.json sbom/rust-dependencies.json man/wotan-ctl.1 man/test_batch.1 man/zhen-agentd.1 man/zhen-claude.1 docs/battle-plans/progress/S-MOVE-OUT-progress.md`

## PHASE 18: SPRINT EXIT
**Goal**: gates green on develop's latest, merged, Stevie's hands listed. **Prerequisite**: Phase 17. **Time**: 40m. **Agent**: coordinator. **Model**: Opus

- [ ] **Step 106** [V] ~20m: `cd ~/tmp/wt/s-move-out && export GOMAXPROCS=4 GOTMPDIR=/var/tmp && gofmt -l cmd pkg services internal; go vet ./...; go test -race ./... 2>&1 | grep -v -E '^(ok|\?)' | head -20; ./scripts/check-gosec-ratchet.sh | tail -1; ./scripts/check-dependency-baseline.sh | tail -1; ./scripts/live-path-inventory.sh --check | tail -1; ./scripts/gen-man.sh | tail -1 | cut -c1-80; ./scripts/check-trivy-misconfig.sh | tail -1 | cut -c1-80` Expect no `gofmt` or `go vet` output, no FAIL line, five pass lines. `tests/` stays out of the `gofmt` list: it holds unformatted files on `develop` that no plan owns. Compare any failure with `develop`.
- [ ] **Step 107** [C] ~1m: Commit the progress file, so the tree is clean for the meta-gate: `cd ~/tmp/wt/s-move-out && ~/tmp/wt/bin/plan-commit s-move-out S-MOVE-OUT "Steps 106-107: progress notes before the closing gates" docs/battle-plans/progress/S-MOVE-OUT-progress.md`. Nothing to commit is fine.
- [ ] **Step 108** [V][S] ~8m: Root for the load gate. This is the only place `check-gates-can-fail.sh --quick` runs: through `test-git-mirror.sh` it does a local `git push` to throwaway repositories (Appendix C). `cd ~/tmp/wt/s-move-out && git status --short | wc -l && EBPF_LOAD_TARGET_DIR=/var/tmp/s-move-out-ebpf/load-gate ./scripts/check-ebpf-loads.sh | tail -1 && ./scripts/check-ebpf-insns.sh | tail -1 && ./scripts/check-gates-can-fail.sh --quick 2>&1 | tail -2` Expect `0`, pass lines, exit 0: the manifest probe at its new path still bites.
- [ ] **Step 109** [V] ~2m: Nothing named survives: `cd ~/tmp/wt/s-move-out && git grep -l -E 'crates/doom-runner|cmd/doom|internal/doom|internal/bpf|doomgeneric|scripts/doom|deploy/k8s|helm/unheaded|kubernetes/manifests|deploy/argocd|crates/zhenai-forge|zlora-to-gguf|build-coding-corpus|zhen-flywheel|doom\.nix' -- . ':!docs' ':!references' ':!wiki' ':!CHANGELOG.md' ':!skills' ':!tomb' ':!crates/xv6-mbc/upstream' ':!.claude' ':!sbom-results' ':!LICENSES' ':!crates/upc-api' ':!cmd/upc-bootctl' ':!cmd/upc-tty-bridge' ':!cmd/depaudit' ':!ebpf' ':!demos' ':!.gitignore' ':!scripts/check-manifest-yaml.sh' ':!scripts/check-gosec-ratchet.sh' ':!scripts/upc-regression.sh' ':!.github/workflows/static-analysis.yml' ':!runbooks/security/vuln-management-2026-05-06.md' ':!runbooks/data/well-pg18-upgrade.yaml' ':!runbooks/observe/ci-red.yaml' ':!runbooks/wotan/active-active-cutover.yaml' ':!THIRD_PARTY.md' ':!CLAUDE.md' ':!pkg/agent/agent.go' ':!pkg/champion/runcheck.go' ':!db/migrations/033_app_zhen_captures.sql' ':!raft/.gitignore' ':!raft/zhen_app.py' ':!eval'` Expect empty output, except the two runbooks Opus edits only if Phase 10A left a reason on record. The exclusions are comments, dated history or a test string (measured in the dry run). Anything that executes: fix or `[STUCK]`.
- [ ] **Step 110** [V] ~5m: The moved repos still build against this tree: `cd ~/tmp/projects/unheaded-doom && GOWORK=off GOFLAGS=-mod=mod go build ./... && echo DOOM-OK; (cd crates/doom-runner && CARGO_TARGET_DIR=/var/tmp/s-move-out-doom cargo check --locked -j4 2>&1 | tail -1); cd ~/tmp/projects/unheaded-k8s && UNHEADED_REPO=$HOME/tmp/wt/s-move-out python3 scripts/well/render-k8s-initdb.py --check; echo rc=$?` Expect `DOOM-OK`, `rc=0`. `go build` here uses `~/tmp/unheaded`, which still has Doom until the merge; Step 114 re-runs it.
- [ ] **Step 111** [V] ~3m: Rule 16. `cd ~/tmp/wt/s-move-out && go run ./cmd/compliance-evidence findings -evidence /home/govan/tmp/unheaded/var/compliance/evidence.json | head -1 | cut -c1-90` Expect the open count not below `develop`'s minus what this plan closed (none: the parked K8s rows are sweep rows, not register findings). A collapse: `[STUCK]`.
- [ ] **Step 112** [B] ~30m: Rule 15: merge develop into the sprint, rebuild, run the whole closing gate on that tree. `cd ~/tmp/wt/s-move-out && git merge --no-ff -S --no-edit develop`; on a gpg failure `git merge --abort; git merge --no-ff --no-gpg-sign --no-edit develop`, recorded. ADR index rows keep both, in number order; `docs/LIVE-PATHS.md` and `cbom.cdx.json` are regenerated, never hand-merged. Any other conflict is `[STUCK]`: `git merge --abort`, no merge, "For Stevie". Then `go build ./...`, `go test ./pkg/uids/ ./cmd/wotan-ctl/ ./cmd/dashboard-backend/... ./scripts/lib/ 2>&1 | grep -c -v -E '^(ok|\?)'` (expect `0`), Step 106 again (it carries the dependency, inventory, man-page and trivy gates), the "Nothing named survives" step again, and `(cd crates/monad-mbc && cargo test --locked -j4 --test integration_tests step106 2>&1 | grep '^test result')` (expect `test result: ok`).
- [ ] **Step 113** [V] ~1m: Nothing moved on `develop` under the paths since each clone.
```bash
cd ~/tmp/unheaded && for r in doom k8s forge; do case $r in doom) L=~/tmp/wt/doom-paths.txt;; k8s) L=~/tmp/wt/k8s-paths.txt;; forge) L=~/tmp/wt/forge-move.txt;; esac; echo "$r: $(xargs git diff --stat "$(cat ~/tmp/wt/unheaded-$r.source)..develop" -- < "$L" | wc -l)"; done
```
  Expect `doom: 0`, `k8s: 0`, `forge: 0`. Anything else: a later plan changed a moved file. Port the change to the new repo by hand, or `[STUCK]`.
- [ ] **Step 114** [B] ~2m: The compliance timer runs `go run` from `~/tmp/unheaded`, so the build must be green the moment this lands.
```bash
cd ~/tmp/unheaded && git switch develop && { git merge --no-ff -S sprint/s-move-out -m '[PLAN S-MOVE-OUT] merge' || { git merge --abort; git merge --no-ff --no-gpg-sign sprint/s-move-out -m '[PLAN S-MOVE-OUT] merge'; }; } && git log --format=%G? -1 && go build ./... && (cd ~/tmp/projects/unheaded-doom && GOWORK=off GOFLAGS=-mod=mod go build ./... && echo DOOM-OK; cd crates/doom-runner && CARGO_TARGET_DIR=/var/tmp/s-move-out-doom cargo check --locked -j4 2>&1 | tail -1)
```
  Expect `G` (or `N`: record the unsigned merge in the progress file), `DOOM-OK`, `Finished`. Stevie pushes.
- [ ] **Step 115** [B] ~2m: `unheaded` no longer requires doomgeneric, so the forge's temporary `replace` goes.
```bash
cd ~/tmp/projects/unheaded-forge && export GOWORK=off GOFLAGS=-mod=mod GOMAXPROCS=4 GOTMPDIR=/var/tmp && go mod edit -dropreplace github.com/unheaded/doomgeneric && go mod tidy && go build -o /dev/null ./cmd/zhen-flywheel && git add go.mod go.sum && git commit -q --no-gpg-sign -m 'Drop the doomgeneric replace: unheaded no longer requires it' && grep -c doomgeneric go.mod; echo rc=$?
```
  Expect `0`, `rc=1`.
- [ ] **Step 116** [V] ~3m: What Stevie would push, all three tips. No push happens here.
```bash
for r in doom k8s forge; do cd ~/tmp/projects/unheaded-$r; gitleaks git . --no-banner --redact --config ~/tmp/unheaded/.gitleaks.toml -f json -r /var/tmp/s-move-out-final.json -l error; printf '%s leaks=%s wads=%s largest=%s remotes=%s dirty=%s\n' $r "$(python3 -c "import json;print(len(json.load(open('/var/tmp/s-move-out-final.json'))))")" "$(git rev-list --objects --all | grep -c -i '\.wad$')" "$(git rev-list --objects --all | git cat-file --batch-check='%(objectsize) %(rest)' | sort -n | tail -1 | cut -d' ' -f1)" "$(git remote | wc -l)" "$(git status --short | wc -l)"; done
```
  Expect `leaks=0 wads=0`, largest under `100000000`, `remotes=0`, `dirty=0` on all three (measured: doom 4.2 MB before the WAD filter, k8s 191 KB, forge 22.6 MB). Anything else: `[BLOCKED]` under "For Stevie".
- [ ] **Step 117** [W] ~5m: Fold the progress file into `~/tmp/next.md` (rule 9) with Stevie's hands: create `unheaded/unheaded-doom`, `unheaded/unheaded-k8s` and `unheaded/unheaded-forge` on GitHub, then add the origin and push `main` from each repository himself. No agent runs those commands. The `go.mod` and `Cargo.toml` sibling paths stay until a public module path is decided.

---

## Appendix A: Agent assignment

| Phase | Model | Parallel | Depends on | Time |
|---|---|---|---|---|
| 0 Preflight | Haiku | no | S-CLEAN-CUT, S-STAGE-2B, S-SUPPLY-FIRST | 10m |
| 1 Doom repo history | Opus | yes | 0 | 15m |
| 2 Doom repo stands | Sonnet | yes | 1 | 30m |
| 3 K8s repo history | Opus | yes | 0 | 5m |
| 4 K8s repo stands | Sonnet | yes | 3 | 15m |
| 4A forge repo history | Opus | yes | 0 | 10m |
| 4B forge repo stands, README | Opus | yes | 4A | 30m |
| 5 docs/upc | Haiku | yes | 0 | 10m |
| 6 Doom paths out | Haiku | no | 1, 2, 5 | 5m |
| 7 wotan-ctl, dashboard | Sonnet | yes | 6 | 25m |
| 8 go.mod, CI | Haiku | after 7 | 7 | 15m |
| 9 harness | Opus | yes | 2, 6 | 30m |
| 9A fixture consumers | Sonnet | after 7 | 6, 7 | 15m |
| 10 Doom rows | Haiku | yes | 6 | 15m |
| 10A root scripts, runbooks | Opus | after 10 | 10 | 25m |
| 11 lessons | Opus | yes | 1 | 40m |
| 12 K8s paths out | Haiku | no | 4 | 5m |
| 12A forge paths out | Haiku | no | 4B, 10A | 10m |
| 13 pkg/uids | Sonnet | yes | 12 | 15m |
| 14 gates, trivy | Sonnet | after 8 | 8, 12 | 25m |
| 15 K8s docs | Haiku | yes | 12 | 15m |
| 15A forge ripple | Haiku | yes | 12A | 15m |
| 16 ledgers | Haiku | no | 7-15A | 25m |
| 17 regenerate | Sonnet | no | 16 | 20m |
| 18 exit | Opus | no | 17 | 40m |

Critical path: 0, 1, 2, 6, 7, 8, 14, 16, 17, 18: about 3.5 hours.

## Appendix B: Emergency procedures

- **A filter run went wrong** (Opus only, it deletes outside the worktree): `rm -rf ~/tmp/projects/unheaded-doom` (or `-k8s`, `-forge`) and repeat the phase; a re-run of Step 8 also needs `rm -rf /var/tmp/s-move-out-dg` first, or its guard stops the block. The source is never touched: each works on a clone.
- **`gitleaks` finds something** other than the known nonce: stop that half of the plan. The moved history is the same history already public in this repo, so nothing new is exposed, but the rotation (DP-01) comes first. Names of files only in the progress file.
- **The tree will not build between Phases 6 and 7**: expected. Other lanes verify narrowly until Phase 7 commits.
- **`upc-regression.sh` fails on the fixture**: `doom.rv2mbc` and `.sha256` are both tracked; `git log -- doom/` shows who moved them. Never regenerate the checksum to make it pass.
- **A later fix to a moved path lands on `develop`**: S-G9-FOLD and S-LOG-HARVEST do not touch moved paths; if one does, cherry-pick by hand into the new repo.
- **A stuck process**: stop it by its recorded pid, never `pkill -f` (rule 14).
- **Undo**: nothing leaves this machine. Before Stevie pushes, `git revert -m 1 <merge>` restores the tree and the three directories can be deleted.

## Appendix C: For Stevie (known at forge time)

1. **The move list grew.** DP-305 named eleven things; the tree needs 53 paths to stay whole (four of them copied, not moved), including `cmd/doom-bridge`, `internal/bpf`, the dashboard endpoints and `doom/doom.elf`. Nothing outside Doom's own closure moved.
2. **"The doomgeneric port" was already outside this repo**, in `~/tmp/projects/doomgeneric`, with two uncommitted edits. It comes in with its history; the edits do not.
3. **The forge.** `raft/kingdom-w12` makes `unheaded-forge`'s history 101 MB (largest file 22.6 MB, under GitHub's limit). `cmd/zhen-flywheel` and the corpus scripts go too (round 5), with the two runbooks that called them. `eval/champion-runbooks` keeps prompts naming those runbooks: frozen eval text, left alone. Its `build.py` cannot be re-run after the move, because it opens the moved runbooks. The symlinks `raft/corpus`, `raft/index`, `raft/models` (to `/var/zhen`) stay.
4. **Sibling-path builds.** `unheaded-doom` and `unheaded-forge` need `unheaded` checked out beside them because `go.mod` uses module `unheaded` and `Cargo.toml` uses a path to `upc-api`. So `unheaded-doom` cannot build on GitHub Actions, and no workflow moved with it. A `upc-api` change can break `doom-runner` with no gate, the harness runs whatever prebuilt binary it finds, and `doom.elf` now lives apart from the `.mbc` and `.rv2mbc` it must match, unchecked. In `unheaded`, CI keeps the sha256 and `step106`; the Doom load runs only where the sibling, the WAD and sudo exist. A public module path is your call.
5. **Signatures.** `filter-branch` strips GPG signatures: new-repo history is unsigned.
6. **`upc-regression.sh`'s Doom load** now needs the sibling repo built. Without it the line skips and exits 0, unless `UPC_REQUIRE_DOOM=1`, which turns a skip into a failure. The fixture checks always run.
7. **`doom-ctl`** carries a full copy of `wotan-ctl` so it compiles; trimming it is work for that repo.
8. **Order.** S-MOVE-OUT waits for S-CLEAN-CUT, S-STAGE-2B and S-SUPPLY-FIRST.
9. **`unheaded-doom` carries GPL-2.0 sources, so its LICENSE must say so.** The plan copies the GPL-3 LICENSE over them; `doom/LICENSE` and `doomgeneric/LICENSE` are there too. S-SUPPLY-FIRST rows 6 and 7 hand on the `GPL-3.0-only` and doomgeneric questions. Yours to decide.
10. **`DOOM1.WAD`** stays out of `unheaded-doom` but remains in upstream doomgeneric history.
11. **`check-gates-can-fail.sh --quick` does a local `git push`** to throwaway repositories through `test-git-mirror.sh`. It is allowed only as this plan's named step, and reaches no remote. After the move, a UPC gate needs `unheaded-doom` built beside this repo and the WAD on disk, and the runner rebuilt when `upc-api` changes.

---

*S-MOVE-OUT Battle Plan: forged 2026-11-04*
*25 phases, 117 steps. Rehearsed in a scratch clone as the normal user: about 75 s per filter, 0 leaks, all three repos built. Round 4's added phases were checked piecewise, not re-run end to end. The live harness (Step 49) was not rehearsed.*
*Everything leaves with its past, and the lesson stays.*
