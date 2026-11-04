<!--
SPDX-License-Identifier: GPL-3.0-or-later
Copyright (c) 2024-2026 Stevie Bellis.
-->
# S-CLEAN-CUT BATTLE PLAN: 24 Phases, 128 Steps

**Date**: 2026-11-04
**Sprint**: S-CLEAN-CUT: the delete words and retirements of the decision pack, each with its ripple
**Mode**: unattended, Lane A of `docs/battle-plans/SPRINT-SERIES-2026-11-04.md`
**Rules**: bound by series rules 1-17 in `docs/battle-plans/SPRINT-SERIES-2026-11-04.md`, and its reserved numbers (ADR-125 to 134 are S-WIRE-HONEST and S-G9-FOLD, S-STAGE-2B starts at 135).
**Answers carried**: DP-401 with `cmd/waf` (round 4), DP-205, DP-209, DP-210, DP-306, DP-307, DP-407, DP-409, DP-410, DP-709, DP-713 (repo side). Dropped by round 4: DP-402 Q3 and the DP-308 corpus flags (they move with the forge, S-MOVE-OUT) and DP-706 (the queue stays). "Answers recorded" in `DECISION-PACK-2026-11-03.md` override the prompts.
**Prerequisite**: `develop` at or after `d6ad43e2`, clean tree, `go build ./...` green on west.
**Target**: everything named above gone from the tree with its importers, gates, baselines, SBOM rows and docs; XDP itself untouched; `pkg/secrets/store` kept; ADR-121 parked; a note that a better GPU brings vLLM back.
**Estimated Duration**: 5 hours with the swarm table; about 10 hours serial. Phase numbers 14, 16 and 17 are retired (round 4); the others keep their numbers.
**Branch / worktree**: `sprint/s-clean-cut` in `~/tmp/wt/s-clean-cut` (rule 1). Merge with `git merge --no-ff -S`. Stevie pushes.
**Agent Strategy**: phases carry a model tag (rule 10). Haiku phases need only this file. Swarm table below; at most three builders (rule 3), doc-only agents do not count. Go lanes in Wave 1 share one `flock`, so they count as one builder.
**Commit Cadence**: every [C] through `~/tmp/wt/bin/plan-commit` (Step 3): signed, named files, format `[PLAN S-CLEAN-CUT] Steps X-Y: ...` with both trailers (rule 2). Never `git add -A <dir>`, never `git push`.
**Stuck Protocol**: rules 8, 12 and 13. Three times the estimate or two failed debug attempts: mark `[STUCK]`, record the output in the progress file, move to the next independent step. Unpredicted output in a Haiku or Sonnet phase is `[STUCK]` at once, and so is any need for root.
**Progress file**: `docs/battle-plans/progress/S-CLEAN-CUT-progress.md`. Questions go under "For Stevie", oldest first; never stop to ask.

## GROUNDING: what the tree said on 2026-11-04 at `d6ad43e2`

| Plan input | What the tree proves | Effect |
|---|---|---|
| DP-706 | Round 4: `transport.Connect` is already gone (`231c69db`) and the queue has flushed since `c9927f77`. | **Dropped.** `pkg/wotan-client` is not touched. |
| DP-401.2 "39 definitions" | `lxd/containers/` is 34 yaml plus 5 index files (`INDEX.md`, `README.md`, `MANIFEST.txt`, `GENERATED_FILES.txt`, `_FILE_LIST.txt`) that list only those yaml. No script reads the directory. | All 39 files go. `lxd/README.md` stays. |
| DP-401.3 docker/hosts | `docs/security/compose-hardening-baseline-alt.txt` lists `docker/hosts/*` rows; the gate would fail on rows with no file. | Baseline rows go in the same phase. |
| DP-401.5 cmd/ebpf-exporter | `go mod tidy` drops no module (trial in a scratch export): cilium/ebpf has six other importers. Ripple: `man/ebpf-exporter.1`, `nixos/modules/ebpf-exporter.nix`, two host imports, two scrape jobs, anamnesis-lite docs. | SBOM count does not move for this one. |
| DP-205 AF_XDP | **Pure AF_XDP** (delete): `pkg/afxdp` (build tag `afxdp`, no importer), `ebpf/af-xdp`, `ebpf/af-xdp-common`, `ebpf/xdp-redirect` (its only job is `XSKS.redirect`). **Mixed, cut the branch, keep the program**: `ebpf/shield-ebpf` (`SHIELD_XSKS`, `SHIELD_CONFIG`, section 13), `ebpf/packet-marker` (`MARKER_XSKS`, `MARKER_CONFIG`, selective redirect), `ebpf/monad-common` (`flow_flags`, `redirect_action`, `is_af_xdp_path`). **Incidental, keep**: `pkg/ebpf/loader.go` `BPF_MAP_TYPE_XSKMAP = 17` is the kernel enum (comment fixed) and `XDP_REDIRECT = 4` is XDP itself; `cmd/trace-collector/src/bpf/mod.rs` `XskMap = 17` dies with Phase 9. | Phases 6, 7, 8. |
| DP-205 "verify Doom first" | Zero hits for `xsk\|af_xdp\|afxdp` across `ebpf/monad-cpu-ebpf`, `crates/doom-runner`, `crates/upc-api`, `cmd/upc-bootctl`, `cmd/doom*`, `internal/bpf`, `internal/doom`, `scripts/doom*`, `scripts/ws3`, the Doom units and `demos/doom`. The injector uses `socket(AF_PACKET, ...)` (`cmd/doom-injector-c/injector.c:58`). `docs/doom` mentions AF_XDP only as an unbuilt idea (4 lines). The one hit in `~/tmp/projects/doomgeneric` is a stale copy of monad-common under `unheaded/ebpf`. | Retire. Step 38 re-runs the proof first. |
| AF_XDP and `hop_id` | Shield writes `redirect_action` into Anamnesis `hop_id`. Live value is `NO_REDIRECT` = 0 (toggle off, nothing writes `SHIELD_CONFIG`). | Keep emitting 0: no wire change, no RFC desk. |
| DP-210 Rust collector | `docker/services/trace-collector/Dockerfile` runs `go build ./cmd/trace-collector`, a Rust directory: it has never built. `crates/svc-token` is still used by `cmd/ebpf-collector`; its CI step lives inside the `trace-collector` job of `ebpf.yml`. | Dockerfile points at `cmd/trace-collector-go`; the job is renamed, svc-token step kept. |
| DP-402 Q3, DP-308 corpus flags | Round 4: 25 of the 28 ignored tests assert Gemma-4, the forge's target; the forge moves out whole. | **Dropped here.** Tests and flags travel with `unheaded-forge` (S-MOVE-OUT). |
| DP-409 | `phase12-option-a` hook is also empty, but the answer names B and C only and xv6 comments cite it by name. | A and the no-op stay. |
| DP-209 CBOM | `compliance/crypto/cbom.cdx.json` is generated from source. Its zhend rows are true of the file and false of the running binary, so hand edits would fail `check-crypto-inventory.sh`. | New link-state `not-spawned` in `cmd/crypto-inventory`, tests first, regenerate. |
| DP-709 | Eleven rows at verdict `FLAGGED`: 7 `pkg/waf*`, `pkg/secrets/store`, `pkg/secrets/encryption` (imported by store), `pkg/audit/storage` (live importers), `pkg/trace` (tests/e2e). Each kept row carries its own 2026-10-26 evidence. | Waf rows DELETED, the other four KEEP. The two PARK rows for bpfmap and maploader become DELETED. |
| DP-713 | Beyond the plan's list: `docker/vllm-rocm/`, `nixos/modules/vllm-rocm.nix`, `lxd/LAUNCH_EXAMPLES.sh`, `lxd/Makefile`, `docker/Makefile`, `configs/services.yaml`, the `LLMInferenceStalled` alert. `sophia-eye`'s `open-webui` and `sophia-gateway` both depend on `vllm`. The image in the compose file does not exist; the image on west is `rocm/vllm:latest` (`e26f3c1684f3`, 30.7 GB). `scripts/bootstrap-llm-lab.sh` has 26 vLLM lines. `kubernetes/manifests/overlays/gpu-vllm` moves with S-MOVE-OUT. | Phases 3, 5A, 19, 20, 22A. The image on west is S-HOST-TIDY's (its Steps 47-50). |
| `cmd/waf` (round 4: leaves with `pkg/waf`) | Rust binary `shield`, own `Cargo.lock`. **Nothing depends on it**: no crate or Go package imports it, no unit, compose service or Dockerfile runs it (`docker/services/shield` and `nixos/modules/services/shield.nix` run the Go `cmd/shield`, which stays). Ripple: `Makefile` (`build-shield`, `test-rust-shield`), `.github/dependabot.yml`, `scripts/ci-local.sh`, `scripts/verify-gpl-boundary.sh`, `scripts/kingdom-shutdown.sh`, `services.yaml` (`shield-waf`, a design row), `docs/LIVE-PATHS.md`, `THIRD_PARTY.md`, CBOM rows, ADR-105, ADR-004. | Phase 1, Phase 9, Phase 21, Phase 22, Phase 22A, Phase 23. |
| `aws-lc-rs` after `cmd/waf` | **`crates/zhend` still needs it**: direct dependency (Ed25519 for its self-signed QUIC certificate, rustls provider, `crates/zhend/Cargo.toml:42`) although the QUIC server is never spawned. | The ADR-004 admission stands, narrowed to zhend. |

**Worktree trap (rule 1).** `go.mod`'s relative `replace` needs the `~/tmp/wt/projects` symlink: Step 2.

**Rule 13 in this plan.** Haiku and Sonnet touch nothing that runs as root, deletes outside the worktree, rewrites history, or reaches a firewall or a live host. So these sit in Opus phases: the Nix host files (their `allowedTCPPorts` is firewall config) in Phase 5A; `scripts/kingdom-shutdown.sh` (`sudo rm -rf`, ssh to east), `scripts/ci-local.sh` (sudo probe) and `scripts/vault-to-runway.py`, `nix/containers/sophia-eye.nix` and the `lxd` launchers in Phase 22A; the load gate (root for `BPF_PROG_LOAD`) as named [S] steps in Phases 7 and 24. No step uses `pkill -f` (rule 14). Scratch lives under `/var/tmp/s-clean-cut-*`, this plan's alone. Forgers ran nothing as root.

**Overlap with other lanes.** S-WIRE-HONEST (DP-716, DP-408) edits `ebpf/shield-ebpf/src/main.rs` and `pkg/protocol/bpfschema/bpfschema.go`: **S-CLEAN-CUT merges first** (series, known collisions). S-SUPPLY-FIRST (DP-405) edits `.github/dependabot.yml`. S-SUPPLY-FIRST merges clean, but its exact counts (`locks=24` at Steps 26 and 28, `4` at Step 81) hold only on a base before this merge, which removes three lockfiles: start its lanes first, or make those counts tolerate the three gone locks. Lowering `netip-baseline` to about 74 takes away 46 of headroom: a later plan adding `net.IP` fails its gate, as the ratchet intends. S-WIRE-HONEST does not use `SHIELD_CONFIG`. Whichever merges second rebases; the coordinator owns that.

## LEGEND

[B] bash · [V] verify (must pass) · [D] debug (only on failure) · [W] write/edit file · [R] read · [S] sudo · [P] parallelizable · [C] commit checkpoint · [STUCK]/[BLOCKED]

**Standing rules for every step.**
- Every bash block runs from the worktree root: start with `cd ~/tmp/wt/s-clean-cut`. Go commands run with `export GOMAXPROCS=4 GOTMPDIR=/var/tmp`.
- `sub.py` applies exact old-to-new blocks, all or nothing. Its only success output is `sub.py: <file>: N block(s) applied`.
- **Any other output** than the one a step names, in a Haiku or Sonnet phase: mark the step `[STUCK]`, paste the output under that step's number in the progress file, go to the next step of the phase. Do not improvise. A [V] that fails goes to its [D] if the phase has one, otherwise `[STUCK]`.
- `plan-commit` output ends in `committed <hash>`.

## SWARM TABLE

Every lane touches only its own files. Files shared by several answers are edited once, in Phase 22, after the lanes close.

| Wave | Phases at once | Model | Builder? | Files owned |
|---|---|---|---|---|
| 0 | 0 | Haiku | no | worktree, helpers |
| 1 | 2, 3, 5, 8*, 19 | Haiku | no | as each phase lists |
| 1 | 1, 4, 9, 10, 12, 15 | Haiku | yes, one at a time: every `go` command takes `flock ~/tmp/wt/.s-clean-cut.build` | as each phase lists |
| 1 | 5A | Opus | no | `nixos/` |
| 1 | 11 | Sonnet | yes | `cmd/upc-tty-bridge/` |
| 1 | 18 | Sonnet | yes | `cmd/crypto-inventory/`, one wiki page |
| 1 | 6 then 7 | Haiku, then Opus (load gate is a named [S] step) | yes | `ebpf/`, `pkg/afxdp`, `pkg/ebpf/loader.go` |
| 2 | 13 | Sonnet | yes | `ebpf/monad-cpu-ebpf` (after Phase 7: same workspace and lock) |
| 2 | 20 | Opus | no | `deploy/sophia-eye/`, `pkg/uids/` |
| 2 | 21 | Haiku | no | `docs/adr/`, `wiki/ADR-Index.md`, `docs/sweeps/` |
| 3 | 22 | Haiku, one agent | no | the shared ledgers |
| 3 | 22A | Opus | no | `lxd` launchers, `scripts/ci-local.sh`, `scripts/kingdom-shutdown.sh` |
| 4 | 23 | Sonnet, one agent | yes | baselines, SBOM |
| 5 | 24 | Opus / coordinator | yes | exit gate, merge |

*Phase 8 waits for Phase 6 only for its [C] message; its file moves are independent.

Concurrent commits are safe: `plan-commit` takes a lock and stages only the paths it is given. Never run `git add`, `git rm` or `git mv` by hand: they stage into the shared index and would ride along in another agent's commit. Delete with `rm`, move with `mv`, and list both paths at commit.

---

## PHASE 0: PREFLIGHT (coordinator)
**Goal**: worktree, helpers, baseline numbers recorded.
**Prerequisite**: none. **Time**: 15m. **Agent**: one. **Model**: Haiku

- [ ] **Step 1** [B] ~1m: Create the worktree.
```bash
cd ~/tmp/unheaded && git status --short | grep -v '^??' | wc -l && git worktree add ~/tmp/wt/s-clean-cut -b sprint/s-clean-cut develop
```
  Expect first line `0`, last line `HEAD is now at` plus a hash. A non-zero first line: stop, `[BLOCKED]`, tell the coordinator.
- [ ] **Step 2** [B] ~1m: Give the worktree the `go.mod` replace target.
```bash
ln -sfn ~/tmp/projects ~/tmp/wt/projects && ls ~/tmp/wt/projects/doomgeneric/unheaded/go.mod
```
  Expect the path printed back.
- [ ] **Step 3** [W] ~3m: Write the two helpers (shared by the series; skip if both exist and are identical).
```bash
mkdir -p ~/tmp/wt/bin
cat > ~/tmp/wt/bin/sub.py <<'PY'
#!/usr/bin/env python3
"""sub.py FILE < blocks: exact replacement, all or nothing.
A block is a line <<<<, old text, a line ====, new text, a line >>>>.
Every old text must occur exactly once, in order of application."""
import re, sys
path = sys.argv[1]
src = open(path, encoding="utf-8").read()
blocks = re.findall(r"^<<<<\n(.*?)^====\n(.*?)^>>>>$", sys.stdin.read(), flags=re.S | re.M)
if not blocks:
    print("sub.py: no blocks"); sys.exit(2)
for i, (old, new) in enumerate(blocks, 1):
    n = src.count(old)
    if n != 1:
        print(f"sub.py: {path}: block {i}: old text found {n} times"); sys.exit(3)
    src = src.replace(old, new)
open(path, "w", encoding="utf-8").write(src)
print(f"sub.py: {path}: {len(blocks)} block(s) applied")
PY
cat > ~/tmp/wt/bin/plan-commit <<'SH'
#!/usr/bin/env bash
# plan-commit WORKTREE-NAME PLAN "Steps X-Y: subject" PATH...
# Stages exactly the named files (a deleted path may be a directory), commits
# under a lock. Falls back to --no-gpg-sign and records it for the handoff.
set -euo pipefail
wt=$1 plan=$2 subject=$3; shift 3
cd "$HOME/tmp/wt/$wt"
exec 9>"$HOME/tmp/wt/.$wt.lock"; flock 9
for p in "$@"; do
  if [ -d "$p" ]; then echo "plan-commit: refusing directory $p: name the files"; exit 4; fi
  if [ -e "$p" ]; then git add -- "$p"; else git rm -r -q --cached --ignore-unmatch -- "$p"; fi
done
msg=$(printf '[PLAN %s] %s\n\nCo-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>\nClaude-Session: https://claude.ai/code/session_01TzL3Tqe5ebLDopsg5ZkZdp\n' "$plan" "$subject")
if ! timeout 120 git commit -q -S -m "$msg" 2>/dev/null; then
  if ! git commit -q --no-gpg-sign -m "$msg"; then git reset -q -- "$@"; echo "plan-commit: refused, unstaged"; exit 5; fi
  echo "$(git rev-parse --short HEAD) unsigned" >> "$HOME/tmp/wt/.$wt.unsigned"
fi
echo "committed $(git rev-parse --short HEAD)"
SH
chmod +x ~/tmp/wt/bin/sub.py ~/tmp/wt/bin/plan-commit && ls -l ~/tmp/wt/bin | wc -l
```
  Expect `3`. The heredocs must land with no leading indent: `head -1 ~/tmp/wt/bin/sub.py` prints `#!/usr/bin/env python3`.
- [ ] **Step 4** [V] ~1m: `cd ~/tmp/wt/s-clean-cut && printf '<<<<\nGPL\n====\nGPL\n>>>>\n' | python3 ~/tmp/wt/bin/sub.py /dev/null; echo rc=$?` Expect `sub.py: /dev/null: block 1: old text found 0 times` and `rc=3`: the helper refuses what it cannot find.
- [ ] **Step 5** [B] ~6m: Baseline build.
```bash
cd ~/tmp/wt/s-clean-cut && export GOMAXPROCS=4 GOTMPDIR=/var/tmp && go build ./... && go vet ./tests/s77/ ./pkg/auth/ ./pkg/protocol/bpfschema/ && echo BUILD-OK
```
  Expect `BUILD-OK`.
- [ ] **Step 6** [W] ~2m: Start the progress file.
```bash
cd ~/tmp/wt/s-clean-cut && cat > docs/battle-plans/progress/S-CLEAN-CUT-progress.md <<'MD'
<!--
SPDX-License-Identifier: GPL-3.0-or-later
Copyright (c) 2024-2026 Stevie Bellis.
-->
# S-CLEAN-CUT progress

## For Stevie (oldest first)

## Stuck steps

## Evidence
MD
grep -c '^## ' docs/battle-plans/progress/S-CLEAN-CUT-progress.md
```
  Expect `3`.
- [ ] **Step 7** [C] ~1m: **PHASE 0 EXIT GATE + COMMIT.** `~/tmp/wt/bin/plan-commit s-clean-cut S-CLEAN-CUT "Steps 1-7: worktree and progress file" docs/battle-plans/progress/S-CLEAN-CUT-progress.md`

## PHASE 1: pkg/waf and cmd/waf (DP-401.1, DP-709, round 4)
**Goal**: the seven Go WAF packages and the Rust `cmd/waf` leave; `tests/s77` no longer asserts them. **Found**: no Go importer outside `pkg/waf`; 47 files; `tests/s77` asserts them at two sites; `pkg/waf/rules/s77_integration_test.go` goes with the package. Nothing depends on `cmd/waf` (grounding table). The Go `cmd/shield` stays.
**Prerequisite**: Phase 0. **Time**: 20m. **Agent**: Lane A. **Model**: Haiku

- [ ] **Step 8** [B] ~1m: `cd ~/tmp/wt/s-clean-cut && git grep -c '"unheaded/pkg/waf' -- '*.go' ':!pkg/waf' ; echo rc=$?; git grep -l -E 'path *= *".*cmd/waf"|cmd/waf/target|^FROM .*waf' -- '*.toml' '*Dockerfile*' 'deploy' 'docker-compose.yml' 'nixos'; echo rc=$?` Expect `rc=1` twice and no other line: nothing depends on either.
- [ ] **Step 9** [B] ~1m: `cd ~/tmp/wt/s-clean-cut && rm -rf pkg/waf cmd/waf && test ! -e pkg/waf && test ! -e cmd/waf && test -d cmd/shield && echo GONE` Expect `GONE`.
- [ ] **Step 10** [W] ~2m: Amend the s77 test.
```bash
cd ~/tmp/wt/s-clean-cut && python3 ~/tmp/wt/bin/sub.py tests/s77/s77_verification_test.go <<'SUB'
<<<<
	t.Run("Bug17_XFF_Spoofing", func(t *testing.T) {
		fileExists(t, filepath.Join(root, "pkg/waf/rules/engine.go"))
		fileContains(t, filepath.Join(root, "pkg/waf/rules/engine.go"), "extractSecureClientIP")
		fileExists(t, filepath.Join(root, "pkg/waf/rules/clientip_test.go"))
		fileContains(t, filepath.Join(root, "pkg/waf/rules/clientip_test.go"), "TestExtractSecureClientIP")
		fileExists(t, filepath.Join(root, "pkg/waf/detection/bot_test.go"))
	})

====
>>>>
<<<<
		"pkg/waf/rules/clientip_test.go",
====
>>>>
SUB
```
  Expect `sub.py: tests/s77/s77_verification_test.go: 2 block(s) applied`.
- [ ] **Step 11** [V] ~2m: `cd ~/tmp/wt/s-clean-cut && flock ~/tmp/wt/.s-clean-cut.build go test ./tests/s77/ 2>&1 | tail -1` Expect a line starting `ok  	unheaded/tests/s77`.
- [ ] **Step 12** [W] ~2m: Doc ripple, project structure.
```bash
cd ~/tmp/wt/s-clean-cut && python3 ~/tmp/wt/bin/sub.py docs/architecture/PROJECT_STRUCTURE.md <<'SUB'
<<<<
| 🛡️ **Shield** | WAF/Gateway | `pkg/waf/`, `services/gateway/` | 90% |
====
| 🛡️ **Shield** | Gateway | `cmd/shield/`, `services/gateway/` | 90% |
>>>>
SUB
sed -i '/^│   ├── waf\/  *# 🛡️ THE SHIELD - Web Application Firewall$/,/^│   │$/d; /^│   └── waf\/  *# 🛡️ THE SHIELD STANDALONE (Rust)$/d' docs/architecture/PROJECT_STRUCTURE.md
grep -c 'THE SHIELD - Web Application Firewall' docs/architecture/PROJECT_STRUCTURE.md
```
  Expect `sub.py: docs/architecture/PROJECT_STRUCTURE.md: 1 block(s) applied`, then `0`.
- [ ] **Step 13** [W] ~1m: Mark the design doc as history.
```bash
cd ~/tmp/wt/s-clean-cut && python3 ~/tmp/wt/bin/sub.py docs/security/WAF_ARCHITECTURE.md <<'SUB'
<<<<
# WAF Architecture: Two-Tier Go Reference + Rust Acceleration

====
# WAF Architecture: Two-Tier Go Reference + Rust Acceleration

> **History (2026-11-04).** `pkg/waf` and the Rust `cmd/waf` described here were deleted by S-CLEAN-CUT (DP-401, round 4). A future WAF is parked as ADR-121.

>>>>
SUB
```
  Expect `sub.py: docs/security/WAF_ARCHITECTURE.md: 1 block(s) applied`.
- [ ] **Step 14** [W] ~1m: `cd ~/tmp/wt/s-clean-cut && sed -i '/^19\. pkg\/waf\/ — 30+ items marked "TODO: Rust rebuild"$/d' scripts/claude-handoff-prompt.md && grep -c 'pkg/waf' scripts/claude-handoff-prompt.md` Expect `0`.
- [ ] **Step 15** [C] ~1m: **PHASE 1 EXIT GATE + COMMIT.** `git -C ~/tmp/wt/s-clean-cut grep -l 'pkg/waf' -- '*.go'` prints nothing, then `~/tmp/wt/bin/plan-commit s-clean-cut S-CLEAN-CUT "Steps 8-15: delete pkg/waf and cmd/waf, amend tests/s77 (DP-401.1)" pkg/waf cmd/waf tests/s77/s77_verification_test.go docs/architecture/PROJECT_STRUCTURE.md docs/security/WAF_ARCHITECTURE.md scripts/claude-handoff-prompt.md` Expect `committed <hash>`. QUICKSTART, THIRD_PARTY and the sweep rows land in Phases 21 and 22.

## PHASE 2: lxd/containers (DP-401.2)
**Goal**: the superseded definitions go; `lxd/README.md` stays and says so. **Found**: nothing reads `lxd/containers/` (`lxd/deploy.sh` reads `profiles/`; the launchers call `lxc launch` directly). `check-image-tags.sh` already excludes `lxd/**`.
**Prerequisite**: Phase 0. **Time**: 10m. **Agent**: Lane B. **Model**: Haiku

- [ ] **Step 16** [B] ~1m: `cd ~/tmp/wt/s-clean-cut && git ls-files lxd/containers | wc -l && git grep -l 'containers/' -- lxd/deploy.sh lxd/Makefile lxd/hosts/host-a/launch-all.sh lxd/hosts/host-b/launch-minimal.sh; echo rc=$?` Expect `39`, then `rc=1`.
- [ ] **Step 17** [B] ~1m: `cd ~/tmp/wt/s-clean-cut && rm -rf lxd/containers && test -f lxd/README.md && test ! -e lxd/containers && echo GONE` Expect `GONE`.
- [ ] **Step 18** [W] ~2m: Record it in `lxd/README.md` and drop the two stale index lines.
```bash
cd ~/tmp/wt/s-clean-cut && sed -i '1a\
\
> **2026-11-04 (S-CLEAN-CUT, DP-401):** the per-service definitions in `lxd/containers/` were deleted; Cuirass (`cmd/unheaded-daemon`) manages instances. Pi-hole runs from its own runbook. This file stays as the record.' lxd/README.md
sed -i '/containers\//d' lxd/INDEX.md lxd/CREATED_FILES.md
sed -i '/^- VM definitions: `lxd\/containers\/opnsense\.yaml`, `ipfire\.yaml`$/d; /^- Routing containers: `lxd\/containers\/frr\.yaml`, `bird\.yaml`$/d' lxd/firewall/README.md
grep -c 'DP-401' lxd/README.md; git grep -c 'containers/' -- lxd/INDEX.md lxd/CREATED_FILES.md lxd/firewall/README.md; echo rc=$?
```
  Expect `1`, then `rc=1`.
- [ ] **Step 19** [C] ~1m: **PHASE 2 EXIT GATE + COMMIT.** `~/tmp/wt/bin/plan-commit s-clean-cut S-CLEAN-CUT "Steps 16-19: delete lxd/containers, keep lxd/README.md (DP-401.2)" lxd/containers lxd/README.md lxd/INDEX.md lxd/CREATED_FILES.md lxd/firewall/README.md`

## PHASE 3: docker/ lane: host stacks and the vLLM image (DP-401.3, DP-218, DP-713)
**Goal**: `docker/hosts/` and `docker/vllm-rocm/` gone, `docker/Makefile` honest, FND-009 closed to the routing containers. **Found**: `docker/Makefile`, `docker/README.md`, `docker/firewall/*`, `compliance/findings/register.yaml`, the compose-hardening baseline. `docker/DEPLOYMENT_GUIDE.md` and `docker/DOCKER_SETUP_SUMMARY.md` are session summaries with `/sessions/...` paths and nothing but the retired stacks: they move to `docs/archive/retired-2026-11/docker/`.
**Prerequisite**: Phase 0. **Time**: 30m. **Agent**: Lane C. **Model**: Haiku

- [ ] **Step 20** [B] ~1m: `cd ~/tmp/wt/s-clean-cut && rm -rf docker/hosts docker/vllm-rocm && mkdir -p docs/archive/retired-2026-11/docker && mv docker/DEPLOYMENT_GUIDE.md docker/DOCKER_SETUP_SUMMARY.md docs/archive/retired-2026-11/docker/ && ls docs/archive/retired-2026-11/docker | wc -l` Expect `2`.
- [ ] **Step 21** [W] ~4m: `docker/Makefile`: drop the host and vLLM targets.
```bash
cd ~/tmp/wt/s-clean-cut && python3 ~/tmp/wt/bin/sub.py docker/Makefile <<'SUB'
<<<<
.PHONY: all build build-svc build-vllm push pull up-forge up-outpost down logs clean preflight status help
====
.PHONY: all build build-svc push pull logs clean preflight status help
>>>>
<<<<
## Build vLLM/ROCm image
build-vllm:
	@echo "→ Building vLLM/ROCm image ($(VERSION))..."
	docker build $(BUILD_ARGS) \
	  -t unheaded-vllm-rocm:$(VERSION) \
	  -t unheaded-vllm-rocm:latest \
	  -f vllm-rocm/Dockerfile \
	  vllm-rocm/
	@echo "✓ vLLM/ROCm image built successfully"

====
>>>>
<<<<
## Push vLLM/ROCm image
push-vllm:
	@echo "→ Pushing vLLM/ROCm image to $(REGISTRY)..."
	docker tag unheaded-vllm-rocm:$(VERSION) $(REGISTRY)/vllm-rocm:$(VERSION)
	docker push $(REGISTRY)/vllm-rocm:$(VERSION)
	@echo "✓ vLLM/ROCm image pushed"

====
>>>>
<<<<
## Start Host-A (Forge) — full stack with all services
up-forge:
	@echo "→ Starting Forge (host-a)..."
	@if [ ! -d "hosts/host-a" ]; then \
	  echo "✗ hosts/host-a not found"; \
	  exit 1; \
	fi
	cd hosts/host-a && docker compose up -d
	@echo "✓ Forge (host-a) is running"
	@echo "  Use 'make status' to check service health"

## Start Host-B (Outpost) — minimal suite for edge compute
up-outpost:
	@echo "→ Starting Outpost (host-b)..."
	@if [ ! -d "hosts/host-b" ]; then \
	  echo "✗ hosts/host-b not found"; \
	  exit 1; \
	fi
	cd hosts/host-b && docker compose up -d
	@echo "✓ Outpost (host-b) is running"
	@echo "  Use 'make status' to check service health"

## Stop all services (host-a and host-b)
down:
	@echo "→ Stopping all services..."
	@cd hosts/host-a && docker compose down 2>/dev/null || true
	@cd hosts/host-b && docker compose down 2>/dev/null || true
	@echo "✓ All services stopped"

====
>>>>
<<<<
	  docker rmi unheaded-vllm-rocm:$(VERSION) unheaded-vllm-rocm:latest 2>/dev/null || true; \
====
>>>>
<<<<
	@echo "  make build-vllm               # Build vLLM/ROCm image"
	@echo "  make up-forge                 # Start host-a (full stack)"
	@echo "  make up-outpost               # Start host-b (minimal)"
====
>>>>
<<<<
	@echo "  make down                     # Stop all services"
====
>>>>
SUB
grep -c -E 'vllm|hosts/host|up-forge|up-outpost' docker/Makefile; make -n -C docker help >/dev/null && echo MAKE-OK
```
  Expect the `sub.py` line with `7 block(s)`, `0`, `MAKE-OK`.
- [ ] **Step 22** [W] ~3m: `docker/README.md`: exact lines, plus the banner for the prose the docs sweep will take (DP-402 Q2).
```bash
cd ~/tmp/wt/s-clean-cut && sed -i '/^cp \.env\.shared hosts\/host-[ab]\/\.env\.shared$/d; /^- `hosts\/host-[ab]\/\.env` (.* configuration)$/d; /^4\. Add to host compose files (`hosts\/host-a\/docker-compose\.yml`)$/d; /^# Build vLLM\/ROCm image (GPU inference)$/d; /^make build-vllm/d; /^make up-forge/d; /^make up-outpost/d' docker/README.md
sed -i '1a\
\
> **2026-11-04 (S-CLEAN-CUT, DP-218, DP-713):** the per-host stacks `docker/hosts/` and the image `docker/vllm-rocm/` were retired. The deployment that works is the root `docker-compose.yml` with Cuirass. Sections below that still say Host-A, Host-B or vLLM are history until the docs sweep.' docker/README.md
grep -c -E 'hosts/host-[ab]/docker-compose|make up-forge|make build-vllm' docker/README.md
```
  Expect `0`.
- [ ] **Step 23** [W] ~2m: `docker/firewall` docs.
```bash
cd ~/tmp/wt/s-clean-cut && sed -i '/`docker\/hosts\/host-[ab]\/\(docker-compose\.yml\|\.env\.example\)`/d; /^ *cd docker\/hosts\/host-a$/d' docker/firewall/DEPLOYMENT_NOTES.md docker/firewall/README.md
sed -i 's/^- `docker-compose.yml`: Service definitions in host-a\/host-b$/- The per-host compose stacks were retired 2026-11-04 (DP-218); these two images have no stack today./' docker/firewall/README.md
git grep -c 'docker/hosts' -- docker/firewall; echo rc=$?
```
  Expect `rc=1`.
- [ ] **Step 24** [W] ~1m: Env examples.
```bash
cd ~/tmp/wt/s-clean-cut && sed -i '/^# vLLM\/ROCm configuration$/d; /^VLLM_/d' docker/.env.example docker/.env.shared && grep -c -i vllm docker/.env.example docker/.env.shared
```
  Expect `docker/.env.example:0` and `docker/.env.shared:0`.
- [ ] **Step 25** [W] ~1m: Compose-hardening baseline rows.
```bash
cd ~/tmp/wt/s-clean-cut && sed -i '/^docker\/hosts\//d; /^docker\/vllm-rocm\//d' docs/security/compose-hardening-baseline.txt docs/security/compose-hardening-baseline-alt.txt && grep -c -E '^docker/(hosts|vllm-rocm)/' docs/security/compose-hardening-baseline.txt docs/security/compose-hardening-baseline-alt.txt
```
  Expect both counts `0`. The gate itself reads `git ls-files`, so it runs after the commit (next step but one).
- [ ] **Step 26** [W] ~2m: Close FND-009's D11 part.
```bash
cd ~/tmp/wt/s-clean-cut && python3 ~/tmp/wt/bin/sub.py compliance/findings/register.yaml <<'SUB'
<<<<
      the four s6 init caps, read-only; the opnsense/ipfire firewall VMs are gone (Stevie, 2026-11-03), frr/bird stay listed; host-a's
      shield/unheaded-daemon wait on D11; the 13 imageless host-a services get uids only if D11 keeps the stack
    decision: D11 (host-a network mode; keep or retire the per-host stacks); the firewall-VM question closed 2026-11-03 (removed)
====
      the four s6 init caps, read-only; the opnsense/ipfire firewall VMs are gone (Stevie, 2026-11-03), frr/bird stay listed; the
      per-host stacks were retired 2026-11-04 (DP-218, S-CLEAN-CUT), which closes D11 and leaves the two routing containers
    decision: D11 closed 2026-11-04 (per-host stacks retired); the firewall-VM question closed 2026-11-03 (removed)
>>>>
SUB
python3 -c "import yaml;yaml.safe_load(open('compliance/findings/register.yaml'));print('YAML-OK')"
```
  Expect `sub.py: compliance/findings/register.yaml: 1 block(s) applied`, `YAML-OK`.
- [ ] **Step 27** [C] ~1m: **PHASE 3 EXIT GATE + COMMIT.** `~/tmp/wt/bin/plan-commit s-clean-cut S-CLEAN-CUT "Steps 20-27: retire docker/hosts and docker/vllm-rocm (DP-401.3, DP-218, DP-713)" docker/hosts docker/vllm-rocm docker/DEPLOYMENT_GUIDE.md docker/DOCKER_SETUP_SUMMARY.md docs/archive/retired-2026-11/docker/DEPLOYMENT_GUIDE.md docs/archive/retired-2026-11/docker/DOCKER_SETUP_SUMMARY.md docker/Makefile docker/README.md docker/firewall/DEPLOYMENT_NOTES.md docker/firewall/README.md docker/.env.example docker/.env.shared docs/security/compose-hardening-baseline.txt docs/security/compose-hardening-baseline-alt.txt compliance/findings/register.yaml`
- [ ] **Step 28** [V] ~1m: `cd ~/tmp/wt/s-clean-cut && ./scripts/check-compose-hardening.sh 2>&1 | tail -1` Expect a line starting `[PASS]`. Another lane's uncommitted deletion can raise `FileNotFoundError`: run it again after that lane's commit.

## PHASE 4: internal/bpfmap and pkg/ebpf/maploader (DP-401.4)
**Goal**: both packages leave. **Found**: no importer. `services/wotan/internal/compute/bpfmap.go` is a different file and stays. `cmd/tools/anamnesis-lite/COMPONENTS.md` names maploader. `internal/bpf` and `internal/doom` stay until S-MOVE-OUT.
**Prerequisite**: Phase 0. **Time**: 5m. **Agent**: Lane D. **Model**: Haiku

- [ ] **Step 29** [B] ~1m: `cd ~/tmp/wt/s-clean-cut && git grep -c -E '"unheaded/(internal/bpfmap|pkg/ebpf/maploader)"' -- '*.go' ':!internal/bpfmap' ':!pkg/ebpf/maploader'; echo rc=$?` Expect `rc=1`.
- [ ] **Step 30** [B] ~1m: `cd ~/tmp/wt/s-clean-cut && rm -rf internal/bpfmap pkg/ebpf/maploader && flock ~/tmp/wt/.s-clean-cut.build go build ./pkg/ebpf/ && echo BUILD-OK` Expect `BUILD-OK`.
- [ ] **Step 31** [C] ~1m: **PHASE 4 EXIT GATE + COMMIT.** `~/tmp/wt/bin/plan-commit s-clean-cut S-CLEAN-CUT "Steps 28-31: delete internal/bpfmap and pkg/ebpf/maploader (DP-401.4)" internal/bpfmap pkg/ebpf/maploader`

## PHASE 5: cmd/ebpf-exporter (DP-401.5)
**Goal**: the orphan exporter leaves with its scrape job and docs; the Nix side is Phase 5A. **Found**: `man/ebpf-exporter.1`, one compose Prometheus job, anamnesis-lite docs. `monitoring/grafana/dashboards/02_ebpf_pipeline.json` panels stay: ADR-117 Q1 decides dashboards.
**Prerequisite**: Phase 0. **Time**: 20m. **Agent**: Lane E. **Model**: Haiku

- [ ] **Step 32** [B] ~1m: `cd ~/tmp/wt/s-clean-cut && rm -rf cmd/ebpf-exporter man/ebpf-exporter.1 && ls cmd | grep -c ebpf-exporter; echo rc=$?` Expect `0`, `rc=1`.
- [ ] **Step 33** [W] ~1m: The compose telemetry scrape job.
```bash
cd ~/tmp/wt/s-clean-cut && python3 ~/tmp/wt/bin/sub.py docker/telemetry/prometheus/prometheus.yml <<'SUB'
<<<<
  # eBPF exporter
  - job_name: ebpf-exporter
    static_configs:
      - targets: ['host.docker.internal:9435']

====
>>>>
SUB
```
  Expect `sub.py: docker/telemetry/prometheus/prometheus.yml: 1 block(s) applied`.
- [ ] **Step 34** [W] ~3m: anamnesis-lite docs.
```bash
cd ~/tmp/wt/s-clean-cut/cmd/tools/anamnesis-lite && sed -i '/^| `ebpf-exporter` | Prometheus exporter for BPF map metrics |$/d; /^go build -o bin\/ebpf-exporter      \.\/cmd\/ebpf-exporter\/$/d' README.md
sed -i '/^go build -o bin\/ebpf-exporter      \.\/cmd\/ebpf-exporter\/$/d; /^\.\/bin\/ebpf-exporter -help$/d; /^    --include "bin\/ebpf-exporter" \\$/d; s/^for tgt in trace-collector-go dashboard-backend ebpf-exporter; do$/for tgt in trace-collector-go dashboard-backend; do/' BUILD.md
sed -i '/^| `cmd\/ebpf-exporter\/main\.go` | Prometheus exporter for BPF map metrics |$/d; /^| `pkg\/ebpf\/maploader\/` | BPF map management library |$/d' COMPONENTS.md
cd ~/tmp/wt/s-clean-cut && git grep -c -E 'ebpf-exporter|maploader' -- cmd/tools/anamnesis-lite; echo rc=$?
```
  Expect `rc=1`.
- [ ] **Step 35** [C] ~1m: **PHASE 5 EXIT GATE + COMMIT.** `~/tmp/wt/bin/plan-commit s-clean-cut S-CLEAN-CUT "Steps 32-35: delete cmd/ebpf-exporter (DP-401.5)" cmd/ebpf-exporter man/ebpf-exporter.1 docker/telemetry/prometheus/prometheus.yml cmd/tools/anamnesis-lite/README.md cmd/tools/anamnesis-lite/BUILD.md cmd/tools/anamnesis-lite/COMPONENTS.md`

## PHASE 5A: Nix host files (DP-401.5, DP-713)
**Goal**: the two modules leave; host imports and the firewall port line follow. Opus because `allowedTCPPorts` is firewall configuration (rule 13). Nothing here is applied: these hosts are design files.
**Prerequisite**: Phase 0. **Time**: 10m. **Agent**: Opus lane. **Model**: Opus

- [ ] **Step 36** [W] ~3m: Nix hosts and modules.
```bash
cd ~/tmp/wt/s-clean-cut && rm -f nixos/modules/ebpf-exporter.nix nixos/modules/vllm-rocm.nix && sed -i '/^    \.\.\/\.\.\/modules\/ebpf-exporter\.nix$/d; /^    \.\.\/\.\.\/modules\/vllm-rocm\.nix$/d; /^        9435    # ebpf-exporter$/d; s/^        22 9100 9435 50051 50053 50054 50055 4222$/        22 9100 50051 50053 50054 50055 4222/' nixos/hosts/host-a/configuration.nix nixos/hosts/host-b/configuration.nix
sed -i 's#ExecStart = "/opt/unheaded/bin/trace-collector";#ExecStart = "/opt/unheaded/bin/trace-collector-go";#' nixos/modules/services/trace-collector.nix
python3 ~/tmp/wt/bin/sub.py nixos/modules/telemetry.nix <<'SUB'
<<<<
        {
          job_name = "ebpf-exporter";
          static_configs = [{ targets = [ "localhost:9435" ]; }];
        }
====
>>>>
SUB
python3 ~/tmp/wt/bin/sub.py nixos/README.md <<'SUB'
<<<<
    ├── vllm-rocm.nix        # vLLM + ROCm (Host-A only)
    ├── ebpf-exporter.nix    # eBPF → Prometheus bridge
====
>>>>
<<<<
- Host-A only: `vllm-rocm.nix`, `ebpf-exporter.nix`, full service suite
====
- Host-A only: full service suite
>>>>
SUB
git grep -c -E 'ebpf-exporter|vllm-rocm|\b9435\b' -- nixos; echo rc=$?
```
  Expect two `sub.py` lines, then `rc=1`.
- [ ] **Step 37** [C] ~1m: **PHASE 5A EXIT GATE + COMMIT.** `~/tmp/wt/bin/plan-commit s-clean-cut S-CLEAN-CUT "Steps 36-37: Nix hosts drop the exporter and vLLM modules (DP-401.5, DP-713)" nixos/modules/ebpf-exporter.nix nixos/modules/vllm-rocm.nix nixos/hosts/host-a/configuration.nix nixos/hosts/host-b/configuration.nix nixos/modules/telemetry.nix nixos/README.md nixos/modules/services/trace-collector.nix`

## PHASE 6: AF_XDP: prove, then delete the pure parts (DP-205)
**Goal**: the Doom proof re-run, then `pkg/afxdp`, `ebpf/af-xdp`, `ebpf/af-xdp-common`, `ebpf/xdp-redirect` gone. **Stop line**: if the proof finds anything, the whole AF_XDP track (Phases 6, 7, 8) stops `[BLOCKED]` and the answer goes to "For Stevie": DP-205 was conditional.
**Prerequisite**: Phase 0. **Time**: 10m. **Agent**: Lane F. **Model**: Haiku

- [ ] **Step 38** [V] ~2m: Doom does not touch AF_XDP.
```bash
cd ~/tmp/wt/s-clean-cut && git grep -n -i -E 'xsk|af_xdp|afxdp' -- ebpf/monad-cpu-ebpf crates/doom-runner crates/upc-api cmd/upc-bootctl cmd/doom-loader cmd/doom-bridge cmd/doom-go-injector cmd/doom-injector-c cmd/doom cmd/doom-cpu-dump internal/bpf internal/doom scripts/doom scripts/ws3 'scripts/doom-*' 'deploy/systemd/unheaded-doom*' demos/doom | wc -l; grep -c 'af-xdp' ebpf/monad-cpu-ebpf/Cargo.toml crates/doom-runner/Cargo.toml cmd/upc-bootctl/Cargo.toml
```
  Expect `0`, then three lines each ending `:0`. Anything else: `[BLOCKED]`, see the stop line.
- [ ] **Step 39** [V] ~1m: Nothing binds an XSK socket, and `pkg/afxdp` has no importer: `cd ~/tmp/wt/s-clean-cut && git grep -c '"unheaded/pkg/afxdp"' -- '*.go'; echo rc=$?; git grep -l -E 'afxdp_create|bind\(.*sxdp' -- '*.go' '*.rs' ':!ebpf/af-xdp' ':!ebpf/af-xdp-common' ':!pkg/afxdp'; echo rc=$?` Expect `rc=1` twice.
- [ ] **Step 40** [B] ~1m: `cd ~/tmp/wt/s-clean-cut && rm -rf pkg/afxdp ebpf/af-xdp ebpf/af-xdp-common ebpf/xdp-redirect && ls ebpf | grep -c -E 'af-xdp|xdp-redirect'; echo rc=$?` Expect `0`, `rc=1`.
- [ ] **Step 41** [W] ~2m: Workspace membership.
```bash
cd ~/tmp/wt/s-clean-cut && python3 ~/tmp/wt/bin/sub.py ebpf/Cargo.toml <<'SUB'
<<<<
    "pqc-common",
    # --- AF_XDP Zero-Copy Packet I/O ---
    "af-xdp-common",
    "xdp-redirect",
]
# af-xdp is a userspace (std) library — excluded from bpfel workspace build
exclude = ["af-xdp"]
====
    "pqc-common",
]
>>>>
SUB
python3 - <<'PY'
import re
p='ebpf/Cargo.lock'; s=open(p).read()
n=0
for name in ('af-xdp-common','xdp-redirect'):
    s,c=re.subn(r'\[\[package\]\]\nname = "'+name+r'"\n.*?\n\n','',s,count=1,flags=re.S); n+=c
open(p,'w').write(s); print('lock entries removed',n)
PY
grep -c -E 'af-xdp|xdp-redirect' ebpf/Cargo.lock; echo rc=$?
```
  Expect `sub.py: ebpf/Cargo.toml: 1 block(s) applied`, `lock entries removed 2`, `0`, `rc=1`.
- [ ] **Step 42** [W] ~1m: The kernel enum keeps its value; the comment stops pointing at deleted code.
```bash
cd ~/tmp/wt/s-clean-cut && python3 ~/tmp/wt/bin/sub.py pkg/ebpf/loader.go <<'SUB'
<<<<
	// BPF_MAP_TYPE_XSKMAP support for AF_XDP socket attachment.
	// Maps socket file descriptors to XSKMAP entries for packet redirection.
	// Usage: XSKMAP[queue_id] = socket_fd
	// eBPF: bpf_redirect_map(&XSKMAP, queue_id, 0) returns XDP_REDIRECT
	// See: ebpf/xdp-redirect/src/main.rs and pkg/afxdp/
	BPF_MAP_TYPE_XSKMAP                = 17
====
	BPF_MAP_TYPE_XSKMAP                = 17 // Kernel enum value; nothing here uses it (AF_XDP retired, DP-205)
>>>>
SUB
sed -i 's/return namedBPFObjects(max, "xdp-redirect", "compliance-ebpf"/return namedBPFObjects(max, "compliance-ebpf"/' pkg/ebpf/lich005_fuzz_test.go
gofmt -w pkg/ebpf/loader.go; gofmt -l pkg/ebpf/; go vet ./pkg/ebpf/ && git grep -c 'xdp-redirect' -- pkg/ebpf/lich005_fuzz_test.go; echo rc=$?
```
  Expect the `sub.py` line, no `gofmt` or `vet` line, `rc=1`. The fuzz seed list named the deleted object.
- [ ] **Step 43** [C] ~1m: **PHASE 6 EXIT GATE + COMMIT.** `~/tmp/wt/bin/plan-commit s-clean-cut S-CLEAN-CUT "Steps 38-43: retire AF_XDP userspace and xdp-redirect after the Doom proof (DP-205)" pkg/afxdp ebpf/af-xdp ebpf/af-xdp-common ebpf/xdp-redirect ebpf/Cargo.toml ebpf/Cargo.lock pkg/ebpf/loader.go pkg/ebpf/lich005_fuzz_test.go`

## PHASE 7: AF_XDP: cut the branches out of shield and packet-marker (DP-205)
**Goal**: no XSKMAP in any BPF object; XDP itself and the Anamnesis stream unchanged; verifier and insns gates green. **Wire**: `hop_id` keeps the value it has live today (0), so this does not touch v0x01 and needs no RFC desk.
**Prerequisite**: Phase 6. **Time**: 60m. **Agent**: eBPF lane. **Model**: Opus

- [ ] **Step 44** [W] ~5m: `ebpf/shield-ebpf/src/main.rs`.
```bash
cd ~/tmp/wt/s-clean-cut && python3 ~/tmp/wt/bin/sub.py ebpf/shield-ebpf/src/main.rs <<'SUB'
<<<<
// ── AF_XDP Maps ─────────────────────────────────────────────────────────────

/// XSKMAP — maps queue IDs to AF_XDP socket file descriptors.
/// When AF_XDP is enabled, Shield redirects Monad-stamped packets to zero-copy
/// userspace sockets instead of passing to the kernel stack.
/// Userspace writes: SHIELD_XSKS[queue_id] = socket_fd
#[map]
static SHIELD_XSKS: XskMap = XskMap::with_max_entries(64, 0);

/// Shield AF_XDP configuration.  Key 0 holds the runtime toggle.
/// Bit 0: AF_XDP redirect enabled (1=on, 0=off).  Default: disabled.
/// Userspace writes CONFIG[0] = 1 to enable AF_XDP redirect path.
#[map]
static SHIELD_CONFIG: HashMap<u32, u32> = HashMap::with_max_entries(4, 0);

====
>>>>
<<<<
/// SHIELD_CONFIG key for AF_XDP enable toggle.
const SHIELD_AF_XDP_ENABLE: u32 = 0;

====
>>>>
<<<<
    // ── 13. AF_XDP dual-path decision ──────────────────────────────────────────
    //
    // If AF_XDP is enabled via SHIELD_CONFIG[0], redirect the Monad-stamped
    // packet to the AF_XDP socket bound to this queue for zero-copy delivery.
    // Otherwise, fall through to XDP_PASS (standard kernel stack).
    let af_xdp_enabled = match unsafe { SHIELD_CONFIG.get(&SHIELD_AF_XDP_ENABLE) } {
        Some(val) => (*val & 1) != 0,
        None => false,
    };

    let redirect_act;

    if af_xdp_enabled {
        increment_stat(STAT_REDIRECT_ATTEMPTS);
        let queue_id = unsafe { (*ctx.ctx).rx_queue_index };
        match SHIELD_XSKS.redirect(queue_id, 0) {
            Ok(action) => {
                increment_stat(STAT_REDIRECT_SUCCESS);
                redirect_act = redirect_action::AF_XDP;
                // Emit BIRTH event with redirect_action encoded in hop_id
                let now = unsafe { aya_ebpf::helpers::bpf_ktime_get_ns() };
                emit_anamnesis(
                    now,
                    EventType::Born,
                    redirect_act,
                    flow_label,
                    &hbh_header.monad,
                );
                increment_stat(STAT_BIRTHS);
                return Ok(action);
            }
            Err(_) => {
                // No socket bound to this queue — fall through to kernel stack
                redirect_act = redirect_action::KERNEL_STACK;
            }
        }
    } else {
        redirect_act = redirect_action::NO_REDIRECT;
    }

    // ── 14. Emit BIRTH event to Anamnesis ─────────────────────────────────────
    let now = unsafe { aya_ebpf::helpers::bpf_ktime_get_ns() };
    emit_anamnesis(
        now,
        EventType::Born,
        redirect_act,
        flow_label,
====
    // ── 13. Emit BIRTH event to Anamnesis ─────────────────────────────────────
    // hop_id stays 0, the value it has always carried on this path.
    let now = unsafe { aya_ebpf::helpers::bpf_ktime_get_ns() };
    emit_anamnesis(
        now,
        EventType::Born,
        0,
        flow_label,
>>>>
SUB
grep -n -E 'XskMap|redirect_action|STAT_REDIRECT|\bctx\b.*rx_queue' ebpf/shield-ebpf/src/main.rs
```
  Then remove `XskMap` from the `maps::{...}` import (line 39), `redirect_action` from the `monad_common` import (line 44), and the two constants `STAT_REDIRECT_ATTEMPTS` / `STAT_REDIRECT_SUCCESS` (keep the numbering of the others; userspace reads no name). Check whether `HashMap` is still used. If the function's `ctx` parameter is now unused, keep the signature and let the compiler say so in Step 47. Then run `rustfmt --edition 2021 ebpf/shield-ebpf/src/main.rs`: the import list and the `emit_anamnesis` call need it, or the commit hook refuses the file.
- [ ] **Step 45** [W] ~3m: `ebpf/packet-marker/src/main.rs`.
```bash
cd ~/tmp/wt/s-clean-cut && python3 ~/tmp/wt/bin/sub.py ebpf/packet-marker/src/main.rs <<'SUB'
<<<<
// ── AF_XDP Maps ─────────────────────────────────────────────────────────────

/// XSKMAP — maps queue IDs to AF_XDP socket file descriptors.
/// Trace-marked packets are redirected here for zero-copy delivery.
#[map]
static MARKER_XSKS: XskMap = XskMap::with_max_entries(64, 0);

/// Packet-marker AF_XDP configuration.  Key 0 holds the runtime toggle.
/// Bit 1: AF_XDP redirect enabled for trace-marked packets.
#[map]
static MARKER_CONFIG: HashMap<u32, u32> = HashMap::with_max_entries(4, 0);

====
>>>>
<<<<
const STAT_AFXDP_REDIRECT: u32 = 8;
const STAT_AFXDP_FALLBACK: u32 = 9;

/// MARKER_CONFIG key for AF_XDP enable toggle.
const PACKET_MARKER_AF_XDP_ENABLE: u32 = 0;
====
>>>>
<<<<
    // ── AF_XDP selective redirect ──────────────────────────────────────────
    // If AF_XDP is enabled and this packet has a trace ID, redirect to the
    // AF_XDP socket for zero-copy delivery to userspace trace-collector.
    // Unmarked packets fall through to the standard kernel stack.
    let af_xdp_enabled = match unsafe { MARKER_CONFIG.get(&PACKET_MARKER_AF_XDP_ENABLE) } {
        Some(val) => (*val & 1) != 0,
        None => false,
    };

    if af_xdp_enabled && !trace_id.is_zero() {
        let queue_id = unsafe { (*ctx.ctx).rx_queue_index };
        match MARKER_XSKS.redirect(queue_id, 0) {
            Ok(action) => {
                increment_stat(STAT_AFXDP_REDIRECT);
                let packet_len = (data_end - data) as u32;
                send_packet_event(
                    &flow_key,
                    &trace_id,
                    packet_len,
                    PacketAction::Redirect,
                    Direction::Ingress,
                );
                return Ok(action);
            }
            Err(_) => {
                // No socket bound — fall through to kernel stack
                increment_stat(STAT_AFXDP_FALLBACK);
            }
        }
    }

    // Send packet event to userspace (standard path)
====
    // Send packet event to userspace
>>>>
SUB
sed -i 's/maps::{HashMap, RingBuf, XskMap},/maps::{HashMap, RingBuf},/' ebpf/packet-marker/src/main.rs && grep -c -E 'XskMap|AFXDP|AF_XDP' ebpf/packet-marker/src/main.rs; echo rc=$?
```
  Expect the `sub.py` line, `0`, `rc=1`. `PacketAction::Redirect` stays: it is a shared enum value in `ebpf/common`.
- [ ] **Step 46** [W] ~2m: `ebpf/monad-common/src/lib.rs`: `flow_flags` and `is_af_xdp_path` have no user outside this file; `redirect_action` lost its last user in Step above.
```bash
cd ~/tmp/wt/s-clean-cut && python3 ~/tmp/wt/bin/sub.py ebpf/monad-common/src/lib.rs <<'SUB'
<<<<
// ── AF_XDP flow flags ───────────────────────────────────────────────────────

/// Flow state flags shared between shield-ebpf, packet-marker, and af-xdp-engine.
/// These bits are set atomically in the per-flow FLOW_STATE map.
pub mod flow_flags {
    /// Flow entered the AF_XDP zero-copy fast path (shield redirect).
    /// Cleared when the flow returns to the standard kernel path.
    pub const AF_XDP_PATH: u8 = 0x08;

    /// Flow has an active trace ID and is eligible for AF_XDP redirect.
    pub const TRACED_AF_XDP: u8 = 0x10;
}

/// Redirect action values logged in Anamnesis events and stats.
pub mod redirect_action {
    /// No redirect — standard kernel path.
    pub const NO_REDIRECT: u8 = 0;
    /// Redirected to AF_XDP socket for zero-copy delivery.
    pub const AF_XDP: u8 = 1;
    /// Fell through to kernel stack (AF_XDP enabled but no socket bound).
    pub const KERNEL_STACK: u8 = 2;
}

/// Check whether a flow state flags byte indicates AF_XDP path.
#[inline]
pub const fn is_af_xdp_path(flow_flags: u8) -> bool {
    flow_flags & flow_flags::AF_XDP_PATH != 0
}

====
>>>>
SUB
git grep -n -E 'flow_flags::|redirect_action|is_af_xdp_path' -- ebpf crates cmd pkg
```
  Expect the `sub.py` line and no grep output once the shield import is gone. If a `#[test]` in `lib.rs` names these, delete that test (it asserts removed constants).
- [ ] **Step 47** [B][S] ~10m: Build every variant through the load gate, which is the verifier. The gate needs root for `BPF_PROG_LOAD`: this is a named root step (rule 14). `CARGO_TARGET_DIR` must not reach the gate: it hardcodes `cmd/ebpf-load-check/target/release/ebpf-load-check`, and a moved loader reads as every program failing. Host tests run from the repo root: from `ebpf/` they pick up `build-std`.
```bash
cd ~/tmp/wt/s-clean-cut/ebpf && CARGO_TARGET_DIR=/var/tmp/s-clean-cut-ebpf cargo build --release --workspace -j4 2>&1 | tail -3
cd .. && CARGO_TARGET_DIR=/var/tmp/s-clean-cut-host cargo test --locked --manifest-path ebpf/monad-common/Cargo.toml 2>&1 | grep 'test result'
env -u CARGO_TARGET_DIR EBPF_LOAD_TARGET_DIR=/var/tmp/s-clean-cut-ebpf/load-gate EBPF_INSNS_OUT=/var/tmp/s-clean-cut-insns.tsv ./scripts/check-ebpf-loads.sh 2>&1 | tail -3
```
  Expect `Finished`, a `test result: ok` line, and a final PASS from the load gate. Compile warnings about unused items are errors to fix here, not later.
- [ ] **Step 48** [D] ~10m: First rule out the harness: `test -x cmd/ebpf-load-check/target/release/ebpf-load-check`. On a real verifier failure read `references` to prior verifier budget notes: the two programs only lost code, so a failure means the edit changed control flow. Diff against `git show develop:ebpf/shield-ebpf/src/main.rs`. Two failed attempts: `[STUCK]`, revert only these three files with `git checkout develop -- <file>`, and Phase 8 still runs.
- [ ] **Step 49** [W] ~3m: Rule 17: no `--update`. Shrinkage is silent, so read the new numbers from `/var/tmp/s-clean-cut-insns.tsv`. Delete the row `xdp-redirect/xdp_redirect` and patch only `packet-marker/packet_marker` and `shield-ebpf/shield_xdp` (both feature rows) with `sed` on those lines. Put old and new numbers under "For Stevie". Any other row that moved is not this plan's: leave it, record it.
- [ ] **Step 50** [V] ~1m: `cd ~/tmp/wt/s-clean-cut && ./scripts/check-ebpf-insns.sh --from /var/tmp/s-clean-cut-insns.tsv 2>&1 | tail -1; git grep -c -E 'XskMap|XSKS' -- 'ebpf/*.rs'; echo rc=$?` Expect a PASS line, `rc=1`. Then note ADR-119's map reference for Phase 21.
- [ ] **Step 51** [C] ~1m: **PHASE 7 EXIT GATE + COMMIT.** `~/tmp/wt/bin/plan-commit s-clean-cut S-CLEAN-CUT "Steps 44-51: cut the AF_XDP branches from shield and packet-marker; XDP unchanged (DP-205)" ebpf/shield-ebpf/src/main.rs ebpf/packet-marker/src/main.rs ebpf/monad-common/src/lib.rs docs/policy/ebpf-insns-baseline.tsv`

## PHASE 8: AF_XDP docs (DP-205)
**Goal**: the AF_XDP design docs move to the archive; `ebpf/README.md` stops teaching it. **Stays on purpose**: `docs/BENCHMARKS.md` (920K pps correction), ADRs 047, 065, 094, 119, `docs/doom/*`, `demos/DEMO-SCRIPT.md`, `raft/*`, `skills/*`, `sbom/SBOM.md` (hand-kept: `generate-sbom.sh` does not write it).
**Prerequisite**: Phase 0. **Time**: 15m. **Agent**: docs lane. **Model**: Haiku

- [ ] **Step 52** [B] ~1m: 
```bash
cd ~/tmp/wt/s-clean-cut && mkdir -p docs/archive/retired-2026-11/af-xdp && mv docs/architecture/AF_XDP_ARCHITECTURE.md docs/infrastructure/AF-XDP-DEPLOYMENT.md docs/infrastructure/CHANGELOG_AFXDP.md docs/infrastructure/DEPLOYMENT_GUIDE_AFXDP.md docs/infrastructure/MIGRATION_AFXDP.md docs/infrastructure/TESTING_AFXDP.md docs/security/af-xdp-completion-ring-wraparound-2026-08-03.md docs/archive/retired-2026-11/af-xdp/ && ls docs/archive/retired-2026-11/af-xdp | wc -l
```
  Expect `7`.
- [ ] **Step 53** [W] ~4m: `ebpf/README.md`.
```bash
cd ~/tmp/wt/s-clean-cut && python3 - <<'PY'
import re
p='ebpf/README.md'; s=open(p).read()
a=s.index('## AF_XDP: Zero-Copy Packet I/O'); b=s.index('## Building')
s=s[:a]+s[b:]
a=s.index('## Design Principles'); s=s[:a].rstrip()+'\n'
for line in ('+-- af-xdp-common/       Shared AF_XDP types (no_std, kernel+userspace ABI)\n','+-- af-xdp/              Userspace AF_XDP engine (zero-copy packet I/O)\n','+-- xdp-redirect/         XDP program: AF_XDP packet steering via XSKMAP\n',
           '# Build userspace AF_XDP engine only\ncd ebpf/af-xdp\ncargo build --release\n\n','cd ebpf/xdp-redirect\ncargo build --target bpfel-unknown-none --release\n\n',
           '# All unit tests (no root)\ncd ebpf/af-xdp && cargo test\n\n# Common types tests\ncd ebpf/af-xdp-common && cargo test\n\n','\n# Integration tests (requires root)\ncd ebpf/af-xdp && sudo cargo test -- --include-ignored\n'):
    assert s.count(line)==1, line
    s=s.replace(line,'')
open(p,'w').write(s); print('README-OK')
PY
grep -c -i -E 'af.xdp|xdp-redirect' ebpf/README.md; echo rc=$?
```
  Expect `README-OK`, `0`, `rc=1`. If `grep` prints a number above `0`, list the lines in the progress file and mark `[STUCK]`: the design-principles section held the rest and it was removed whole because all five principles describe the AF_XDP path.
- [ ] **Step 54** [W] ~2m: `docs/README.md`.
```bash
cd ~/tmp/wt/s-clean-cut && python3 ~/tmp/wt/bin/sub.py docs/README.md <<'SUB'
<<<<
| `architecture/` | Component architecture (AF_XDP, AI stack, Kingdom, Zhen, Fae chambers) |
====
| `architecture/` | Component architecture (AI stack, Kingdom, Zhen, Fae chambers) |
>>>>
<<<<
| `infrastructure/` | AF_XDP deployment, load balancers |
====
| `infrastructure/` | Load balancers |
>>>>
SUB
```
  Expect `sub.py: docs/README.md: 2 block(s) applied`.
- [ ] **Step 55** [W] ~2m: `docs/architecture/RUST_COMPONENTS.md` sections for the deleted crates.
```bash
cd ~/tmp/wt/s-clean-cut && python3 - <<'PY'
import re
p='docs/architecture/RUST_COMPONENTS.md'; s=open(p).read()
n0=len(s)
s=re.sub(r'^### `xdp-redirect` -- XDP Redirect Program\n.*?(?=^### |^## |\Z)','',s,flags=re.S|re.M)
s=re.sub(r'^- `SHIELD_XSKS` .*\n|^- `SHIELD_CONFIG` .*\n|^- `redirect_action::.*\n|^- `flow_flags::TRACED_AF_XDP` .*\n','',s,flags=re.M)
s=s.replace('- **Maps:** `MARKER_XSKS` (XSKMAP), `MARKER_CONFIG` (enable toggle),','- **Maps:**')
open(p,'w').write(s); print('removed',n0-len(s)>0)
PY
grep -n -E 'XSK|AF_XDP|af-xdp' docs/architecture/RUST_COMPONENTS.md | wc -l
```
  Expect `removed True`. Record the final count in the progress file; lines that remain are prose for the docs sweep.
- [ ] **Step 56** [C] ~1m: **PHASE 8 EXIT GATE + COMMIT.** `~/tmp/wt/bin/plan-commit s-clean-cut S-CLEAN-CUT "Steps 52-56: AF_XDP docs to the archive, ebpf README rewritten (DP-205)" docs/architecture/AF_XDP_ARCHITECTURE.md docs/infrastructure/AF-XDP-DEPLOYMENT.md docs/infrastructure/CHANGELOG_AFXDP.md docs/infrastructure/DEPLOYMENT_GUIDE_AFXDP.md docs/infrastructure/MIGRATION_AFXDP.md docs/infrastructure/TESTING_AFXDP.md docs/security/af-xdp-completion-ring-wraparound-2026-08-03.md docs/archive/retired-2026-11/af-xdp/AF_XDP_ARCHITECTURE.md docs/archive/retired-2026-11/af-xdp/AF-XDP-DEPLOYMENT.md docs/archive/retired-2026-11/af-xdp/CHANGELOG_AFXDP.md docs/archive/retired-2026-11/af-xdp/DEPLOYMENT_GUIDE_AFXDP.md docs/archive/retired-2026-11/af-xdp/MIGRATION_AFXDP.md docs/archive/retired-2026-11/af-xdp/TESTING_AFXDP.md docs/archive/retired-2026-11/af-xdp/af-xdp-completion-ring-wraparound-2026-08-03.md ebpf/README.md docs/README.md docs/architecture/RUST_COMPONENTS.md`

## PHASE 9: Rust cmd/trace-collector (DP-210)
**Goal**: the Rust collector leaves; `cmd/trace-collector-go` is canonical everywhere a path is named. **Found**: `.github/workflows/ebpf.yml` job and path filters, `.dockerignore`, `.github/dependabot.yml`, `Makefile`, `scripts/ci-local.sh`, `scripts/build.sh`, `scripts/load-ebpf.sh` (Phase 22A), `jenkins/Jenkinsfile.security`, `docs/LIVE-PATHS.md`, `docker/services/trace-collector/Dockerfile`. The unit `unheaded-trace-collector.service`, port rows, uid and `configs/` already mean the Go binary: no change. `crates/svc-token` stays (`cmd/ebpf-collector`). ADR-002, 003, 066 and old plans are history and stay.
**Prerequisite**: Phase 0. **Time**: 20m. **Agent**: Lane G. **Model**: Haiku

- [ ] **Step 57** [B] ~1m: `cd ~/tmp/wt/s-clean-cut && grep -n 'ExecStart=' deploy/systemd/unheaded-trace-collector.service | cut -c1-70 && rm -rf cmd/trace-collector && test -d cmd/trace-collector-go && test ! -e cmd/trace-collector && echo GONE` Expect the `ExecStart` line, `GONE`.
- [ ] **Step 58** [W] ~4m: CI: the job keeps only the svc-token step.
```bash
cd ~/tmp/wt/s-clean-cut && python3 ~/tmp/wt/bin/sub.py .github/workflows/ebpf.yml <<'SUB'
<<<<
      - 'ebpf/**'
      - 'cmd/trace-collector/**'
      - 'crates/svc-token/**'
  pull_request:
    branches: [main, develop, staging]
    paths:
      - 'ebpf/**'
      - 'cmd/trace-collector/**'
      - 'crates/svc-token/**'

jobs:
  trace-collector:
    name: Trace Collector (Rust)
    runs-on: ubuntu-latest
====
      - 'ebpf/**'
      - 'crates/svc-token/**'
  pull_request:
    branches: [main, develop, staging]
    paths:
      - 'ebpf/**'
      - 'crates/svc-token/**'

jobs:
  svc-token:
    name: svc-token (Rust)
    runs-on: ubuntu-latest
>>>>
<<<<
      - name: Cache cargo
        uses: actions/cache@caa296126883cff596d87d8935842f9db880ef25 # v5
        with:
          path: |
            ~/.cargo/registry
            ~/.cargo/git
            cmd/trace-collector/target
          key: ${{ runner.os }}-cargo-trace-${{ hashFiles('cmd/trace-collector/Cargo.lock') }}

      - name: Check
        working-directory: cmd/trace-collector
        run: cargo check

      - name: Clippy
        working-directory: cmd/trace-collector
        run: cargo clippy --locked -- -D warnings

      - name: Format check
        working-directory: cmd/trace-collector
        run: cargo fmt -- --check

      - name: Test
        working-directory: cmd/trace-collector
        run: cargo test --locked

      # The Wotan service-token crate trace-collector and ebpf-collector use
      # (ADR-103 §6). Its Go cross-check is tests/svctoken (Go CI job).
====
      # The Wotan service-token crate ebpf-collector uses (ADR-103 §6). Its
      # Go cross-check is tests/svctoken (Go CI job).
>>>>
SUB
python3 -c "import yaml;d=yaml.safe_load(open('.github/workflows/ebpf.yml'));print('svc-token' in d['jobs'])"; grep -c 'cmd/trace-collector' .github/workflows/ebpf.yml; echo rc=$?
```
  Expect the `sub.py` line with `2 block(s)`, `True`, `0`, `rc=1`.
- [ ] **Step 59** [W] ~2m: The Dockerfile, which never built, builds the canonical collector.
```bash
cd ~/tmp/wt/s-clean-cut && sed -i 's#-o /out/trace-collector ./cmd/trace-collector$#-o /out/trace-collector ./cmd/trace-collector-go#' docker/services/trace-collector/Dockerfile && sed -i '/^cmd\/trace-collector\/target\/$/d' .dockerignore
sed -i 's/^# of SHA-256 over the raw public key. Used by cmd\/trace-collector and$/# of SHA-256 over the raw public key. Used by/' crates/svc-token/Cargo.toml
python3 ~/tmp/wt/bin/sub.py cmd/trace-collector-go/main.go <<'SUB'
<<<<
// This is the Go rewrite of cmd/trace-collector (Rust), using the shared
// pkg/ebpf infrastructure for ring buffer reading and event decoding.
====
// The canonical collector (the Rust one was retired 2026-11-04, DP-210). It
// uses the shared pkg/ebpf infrastructure for ring buffer reading and event
// decoding.
>>>>
SUB
sed -i 's/^BUILT\*\* — currently `cmd\/trace-collector` writes to Wotan but does not$/BUILT** — currently `cmd\/trace-collector-go` writes to Wotan but does not/' docs/policy/retention-policy-v1-2026-05-06.md
sed -i '/^        cmd\/trace-collector\/Cargo\.toml) return 0 ;;$/d; /^        cmd\/waf\/Cargo\.toml) return 0 ;;$/d' scripts/verify-gpl-boundary.sh
sed -i 's#^// mints with crates/svc-token (cmd/trace-collector, cmd/ebpf-collector)\.$#// mints with crates/svc-token (cmd/ebpf-collector).#' services/wotan/cmd/wotan/allowlist_test.go
sed -i 's/for _, want := range \[\]string{"trace-collector", "ebpf-collector"} {/for _, want := range []string{"ebpf-collector"} {/' services/wotan/cmd/wotan/allowlist_test.go
sed -i 's#by cmd/trace-collector and cmd/ebpf-collector#by cmd/ebpf-collector#' tests/svctoken/crosslang_test.go
flock ~/tmp/wt/.s-clean-cut.build go test ./services/wotan/cmd/wotan/ 2>&1 | tail -1 | cut -c1-50
bash -n scripts/verify-gpl-boundary.sh && git grep -c -E 'cmd/trace-collector([^-]|$)' -- scripts/verify-gpl-boundary.sh services/wotan/cmd/wotan/allowlist_test.go tests/svctoken/crosslang_test.go; echo rc=$?
grep -c 'trace-collector-go$' docker/services/trace-collector/Dockerfile; grep -c 'trace-collector-go` writes' docs/policy/retention-policy-v1-2026-05-06.md; flock ~/tmp/wt/.s-clean-cut.build go build -o /dev/null ./cmd/trace-collector-go && echo BUILD-OK
```
  Expect a line starting `ok`, the `sub.py` line, `rc=1`, `1`, `1`, `BUILD-OK`. The test file's assertion goes with its comment: it required a Rust `trace-collector` identity.
- [ ] **Step 60** [C] ~1m: **PHASE 9 EXIT GATE + COMMIT.** `~/tmp/wt/bin/plan-commit s-clean-cut S-CLEAN-CUT "Steps 57-60: retire the Rust trace-collector; Go is canonical (DP-210)" cmd/trace-collector .github/workflows/ebpf.yml docker/services/trace-collector/Dockerfile .dockerignore crates/svc-token/Cargo.toml cmd/trace-collector-go/main.go docs/policy/retention-policy-v1-2026-05-06.md scripts/verify-gpl-boundary.sh services/wotan/cmd/wotan/allowlist_test.go tests/svctoken/crosslang_test.go`

## PHASE 10: dashboard restart route and control (DP-306)
**Goal**: the 501 route, its handler, tests and the UI button go. **Found**: one route, one handler, two tests, one button, one switch case, one function. The toast styling at `services.html:893` is shared and stays. `cmd/unheaded-cli` text at `root.go:103` is about the CLI, not this route.
**Prerequisite**: Phase 0. **Time**: 15m. **Agent**: Lane H. **Model**: Haiku

- [ ] **Step 61** [W] ~2m: Backend.
```bash
cd ~/tmp/wt/s-clean-cut && python3 ~/tmp/wt/bin/sub.py cmd/dashboard-backend/internal/server/server.go <<'SUB'
<<<<
	s.mux.HandleFunc("/api/v1/services/restart/", s.handleServiceRestart)
====
>>>>
<<<<
// handleServiceRestart handles POST /api/v1/services/restart/{name}
// Stub: in production this would signal the daemon to restart the service.
func (s *Server) handleServiceRestart(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	// This answered 202 "restart signal sent to daemon", status "queued",
	// and sent nothing anywhere. The dashboard is a view; restarts are
	// Akira's (--remediate) or the operator's.
	http.Error(w, "not implemented: the dashboard does not restart services (Akira --remediate, or systemctl / docker compose restart)", http.StatusNotImplemented)
}

====
>>>>
SUB
python3 ~/tmp/wt/bin/sub.py cmd/dashboard-backend/internal/server/server_extended_test.go <<'SUB'
<<<<
// ---- Service Restart Endpoint ----

func TestHandleServiceRestart_MethodNotAllowed(t *testing.T) {
	srv := newTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/services/restart/test", nil)
	w := httptest.NewRecorder()
	srv.handleServiceRestart(w, req)
	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want 405", w.Code)
	}
}

// Restart answered 202 "queued" and did nothing; it now says so.
func TestHandleServiceRestart_NotImplemented(t *testing.T) {
	srv := newStartedTestServer(t)
	for _, path := range []string{"/api/v1/services/restart/", "/api/v1/services/restart/wotan", "/api/v1/services/restart/nonexistent"} {
		w := httptest.NewRecorder()
		srv.handleServiceRestart(w, httptest.NewRequest(http.MethodPost, path, nil))
		if w.Code != http.StatusNotImplemented {
			t.Errorf("POST %s: %d, want 501", path, w.Code)
		}
	}
}

====
>>>>
SUB
```
  Expect two `sub.py` lines.
- [ ] **Step 62** [W] ~2m: UI.
```bash
cd ~/tmp/wt/s-clean-cut && python3 ~/tmp/wt/bin/sub.py dashboard/panels/services.html <<'SUB'
<<<<
        case 'restart':
          restartService(serviceName);
          break;
====
>>>>
<<<<
    // -- Restart: confirm then POST to restart endpoint ----------

    function restartService(serviceName) {
      if (!confirm('Restart service "' + serviceName + '"?\n\nThis will interrupt any in-flight requests.')) {
        return;
      }

      showToast('Restarting ' + serviceName + '...', 'success');

      fetch(SERVICES_ENDPOINT + '/restart/' + encodeURIComponent(serviceName), {
        method: 'POST'
      })
        .then(function(response) {
          if (!response.ok) {
            throw new Error('HTTP ' + response.status + ': ' + response.statusText);
          }
          return response.text();
        })
        .then(function() {
          showToast('Service "' + serviceName + '" restart initiated.', 'success');
          // Refresh the services list after a brief delay to let the restart take effect
          setTimeout(loadServices, 2000);
        })
        .catch(function(err) {
          console.error('Failed to restart ' + serviceName + ':', err);
          showToast('Failed to restart "' + serviceName + '": ' + err.message, 'error');
        });
    }

====
>>>>
SUB
sed -i '/<button class="btn btn-secondary btn-xs" data-action="restart" title="Restart service">/,/<\/button>/d' dashboard/panels/services.html
grep -c -E 'data-action="restart"|restartService' dashboard/panels/services.html; echo rc=$?
```
  Expect the `sub.py` line, `0`, `rc=1`.
- [ ] **Step 63** [V] ~3m: `cd ~/tmp/wt/s-clean-cut && gofmt -l cmd/dashboard-backend; flock ~/tmp/wt/.s-clean-cut.build go test ./cmd/dashboard-backend/internal/server/ 2>&1 | tail -1; git grep -c 'handleServiceRestart' -- cmd; echo rc=$?` Expect no `gofmt` line, a line starting `ok`, `rc=1`.
- [ ] **Step 64** [C] ~1m: **PHASE 10 EXIT GATE + COMMIT.** `~/tmp/wt/bin/plan-commit s-clean-cut S-CLEAN-CUT "Steps 61-64: delete the dashboard restart route and its control (DP-306)" cmd/dashboard-backend/internal/server/server.go cmd/dashboard-backend/internal/server/server_extended_test.go dashboard/panels/services.html`

## PHASE 11: upc-tty-bridge poller and skeleton label (DP-307)
**Goal**: no fabricated heartbeat lines on the console; `/health` says what it is. Push ingest (`/api/v1/tty/ingest`) is the shipped design. **Found**: `pollTtyStream` was the only user of the `ctx` and `cancel` made in `main`. `dashboard/upc-console.html:17` says "skeleton" about the CDN xterm, not the bridge: it stays.
**Prerequisite**: Phase 0. **Time**: 15m. **Agent**: Lane I. **Model**: Sonnet

- [ ] **Step 65** [W] ~2m: Test first. Append to `cmd/upc-tty-bridge/main_test.go`.
```bash
cd ~/tmp/wt/s-clean-cut && python3 ~/tmp/wt/bin/sub.py cmd/upc-tty-bridge/main_test.go <<'SUB'
<<<<
	"bytes"
	"net/http"
====
	"bytes"
	"encoding/json"
	"net/http"
>>>>
SUB
cat >> cmd/upc-tty-bridge/main_test.go <<'GO'

// The bridge is push-ingest: upc-bootctl POSTs bytes. It used to report
// "skeleton" and inject a fake line every five seconds (DP-307).
func TestHealthReportsPushIngest(t *testing.T) {
	rec := httptest.NewRecorder()
	handleHealth(rec, httptest.NewRequest(http.MethodGet, "/health", nil))
	var got map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("health body: %v", err)
	}
	if got["mode"] != "push-ingest" {
		t.Errorf("mode = %v, want push-ingest", got["mode"])
	}
	if got["version"] != "0.1.0" {
		t.Errorf("version = %v, want 0.1.0", got["version"])
	}
}
GO
```
  Expect `sub.py: cmd/upc-tty-bridge/main_test.go: 1 block(s) applied`.
- [ ] **Step 66** [V] ~1m: Red. `cd ~/tmp/wt/s-clean-cut && go test ./cmd/upc-tty-bridge/ -run TestHealthReportsPushIngest 2>&1 | grep -E 'mode =|version =|^(ok|FAIL)' | head -4` Expect `mode = skeleton, want push-ingest`, `version = 0.1.0-skeleton, want 0.1.0`, `FAIL`.
- [ ] **Step 67** [W] ~3m: Delete the poller.
```bash
cd ~/tmp/wt/s-clean-cut && python3 ~/tmp/wt/bin/sub.py cmd/upc-tty-bridge/main.go <<'SUB'
<<<<
	serviceVersion = "0.1.0-skeleton"
====
	serviceVersion = "0.1.0"
>>>>
<<<<
// pollTtyStream is the placeholder for the actual BPF-map-read /
// Wotan-topic-subscribe loop. For now it generates a heartbeat byte every
// 5s so the WS channel stays warm.
func (h *Hub) pollTtyStream(ctx context.Context, instance uint8) {
	tick := time.NewTicker(5 * time.Second)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case t := <-tick.C:
			line := fmt.Sprintf("[heartbeat %s] instance=0x%02X (real BPF tty read NYI)\r\n",
				t.Format(time.RFC3339), instance)
			h.broadcastTty(instance, []byte(line))
		}
	}
}

====
>>>>
<<<<
		"mode":    "skeleton",
====
		"mode":    "push-ingest",
>>>>
<<<<
	hub := newHub()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Heartbeat for instance 0xDE (default xv6 first-boot target).
	go hub.pollTtyStream(ctx, 0xDE)

====
	hub := newHub()

>>>>
SUB
```
  Expect `sub.py: cmd/upc-tty-bridge/main.go: 4 block(s) applied`. The `context` import stays: the shutdown path at the end of `main` uses it.
- [ ] **Step 68** [V] ~2m: Green. `cd ~/tmp/wt/s-clean-cut && gofmt -l cmd/upc-tty-bridge; go vet ./cmd/upc-tty-bridge/ && go test -race ./cmd/upc-tty-bridge/ 2>&1 | tail -1; git grep -c -E 'pollTtyStream|heartbeat' -- cmd/upc-tty-bridge; echo rc=$?` Expect no `gofmt` line, a line starting `ok`, `rc=1`.
- [ ] **Step 69** [C] ~1m: **PHASE 11 EXIT GATE + COMMIT.** `~/tmp/wt/bin/plan-commit s-clean-cut S-CLEAN-CUT "Steps 65-69: tty-bridge: no fake heartbeat, no skeleton label (DP-307)" cmd/upc-tty-bridge/main.go cmd/upc-tty-bridge/main_test.go`

## PHASE 12: bpfschema placeholders (DP-407)
**Goal**: `RingPath*`, `SophiaSync*`, `TLVHandlerEntry`, `PrefetchHint*` go. **Found**: used only by their own size test, `doc.go` and the interface map. No `rust_source_test.go` or parity row names them.
**Prerequisite**: Phase 0. **Time**: 10m. **Agent**: Lane J. **Model**: Haiku

- [ ] **Step 70** [W] ~3m: Types.
```bash
cd ~/tmp/wt/s-clean-cut && python3 ~/tmp/wt/bin/sub.py pkg/protocol/bpfschema/bpfschema.go <<'SUB'
<<<<
// === Q4: Ring Path Counters (amplification package) ===

// RingPathKey identifies a flow for amplification tracking.
// Placeholder: no Rust program declares a ring-path map yet.
// Size: 16 bytes. Map type: HASH.
type RingPathKey struct {
	SrcAddr [16]byte // IPv6 source address.
}

// RingPathValue tracks ring path state.
// Size: 8 bytes.
type RingPathValue struct {
	HopCount  uint16 // Current ring hop count.
	MaxHops   uint16 // Configured maximum hops.
	BytesSent uint32 // Bytes sent (for 3× ratio enforcement).
}

====
>>>>
<<<<
// === H1: Sophia Sync State (sophiasync package) ===

// SophiaSyncKey identifies a dictionary sync channel.
// Placeholder: no Rust program declares a Sophia sync map yet.
// Size: 8 bytes. Map type: HASH.
type SophiaSyncKey struct {
	DictID uint16 // Dictionary identifier.
	PeerID uint16 // Peer node identifier.
	_      [4]byte
}

// SophiaSyncValue tracks encoder/decoder ACK state.
// Size: 24 bytes.
type SophiaSyncValue struct {
	TableVersion uint32  // Current table version.
	AckedVersion uint32  // Last ACK'd version by peer.
	PendingDelta uint32  // Pending delta entries count.
	LastSyncAt   uint32  // Unix timestamp of last sync.
	Signature    [8]byte // Truncated HMAC of current state.
}

====
>>>>
<<<<
// === H3: TLV Registry (tlv package) ===

// TLVHandlerEntry maps TLV types to handler programs.
// Placeholder: no Rust program declares a TLV registry map yet.
// Size: 8 bytes. Map type: ARRAY (index = TLV type).
type TLVHandlerEntry struct {
	ProgFD   uint32 // File descriptor of handler BPF program.
	Flags    uint16 // Handler flags (0x01 = required, 0x02 = mutable).
	Reserved uint16 // Must be zero.
}

====
>>>>
<<<<
// === H9: Prefetch Hints (prefetch package) ===

// PrefetchHintKey identifies a prefetch hint.
// Placeholder: no Rust program declares a prefetch hint map yet.
// Size: 8 bytes. Map type: LRU_HASH.
type PrefetchHintKey struct {
	FlowID uint16 // Flow identifier.
	HintID uint16 // Hint sequence number.
	ConnID uint32 // Connection identifier.
}

// PrefetchHintValue stores hint metadata.
// Size: 20 bytes (2+2+1+1+2+4+4+4).
type PrefetchHintValue struct {
	DictID    uint16 // Dictionary to prefetch.
	FieldID   uint16 // Field within dictionary.
	Priority  uint8  // 0-3 priority level.
	Status    uint8  // 0=pending, 1=fulfilled, 2=cancelled.
	_         [2]byte
	IssuedAt  uint32 // Unix timestamp.
	ExpiresAt uint32 // Unix timestamp.
	_         [4]byte
}

====
>>>>
SUB
python3 ~/tmp/wt/bin/sub.py pkg/protocol/bpfschema/bpfschema_test.go <<'SUB'
<<<<
		// Q4: Ring path.
		{"RingPathKey", unsafe.Sizeof(RingPathKey{}), 16},
		{"RingPathValue", unsafe.Sizeof(RingPathValue{}), 8},

====
>>>>
<<<<
		// H1: Sophia sync.
		{"SophiaSyncKey", unsafe.Sizeof(SophiaSyncKey{}), 8},
		{"SophiaSyncValue", unsafe.Sizeof(SophiaSyncValue{}), 24},

====
>>>>
<<<<
		// H3: TLV registry.
		{"TLVHandlerEntry", unsafe.Sizeof(TLVHandlerEntry{}), 8},

====
>>>>
<<<<
		// H9: Prefetch hints.
		{"PrefetchHintKey", unsafe.Sizeof(PrefetchHintKey{}), 8},
		// PrefetchHintValue: 2+2+1+1+2+4+4+4 = 20 bytes (no inter-field padding needed).
		{"PrefetchHintValue", unsafe.Sizeof(PrefetchHintValue{}), 20},

====
>>>>
SUB
python3 ~/tmp/wt/bin/sub.py pkg/protocol/bpfschema/doc.go <<'SUB'
<<<<
// identifier unless #[map(name = "...")] overrides it). A few key/value
// types (RingPath*, SophiaSync*, TLVHandlerEntry, PrefetchHint*) are design
// placeholders with no Rust map yet and carry no map-name constant.
====
// identifier unless #[map(name = "...")] overrides it). A type with no Rust
// map does not live here: the placeholders were deleted 2026-11-04 (DP-407).
>>>>
SUB
sed -i '/^| (new) (no Rust map) | /d; /^4\. Create `pkg\/ebpf\/maploader\/` that uses bpfschema structs to populate maps$/d' pkg/protocol/bpfschema/BPF_IPV6_INTERFACE_MAP.md
git grep -c -E 'RingPath(Key|Value)|SophiaSync(Key|Value)|TLVHandlerEntry|PrefetchHint(Key|Value)' -- pkg; echo rc=$?
```
  Expect three `sub.py` lines (4, 4, 1 blocks), `rc=1`.
- [ ] **Step 71** [V] ~2m: `cd ~/tmp/wt/s-clean-cut && gofmt -l pkg/protocol/bpfschema; flock ~/tmp/wt/.s-clean-cut.build go test ./pkg/protocol/bpfschema/ 2>&1 | tail -1` Expect no `gofmt` line, a line starting `ok`.
- [ ] **Step 72** [C] ~1m: **PHASE 12 EXIT GATE + COMMIT.** `~/tmp/wt/bin/plan-commit s-clean-cut S-CLEAN-CUT "Steps 70-72: delete the four bpfschema placeholder families (DP-407)" pkg/protocol/bpfschema/bpfschema.go pkg/protocol/bpfschema/bpfschema_test.go pkg/protocol/bpfschema/doc.go pkg/protocol/bpfschema/BPF_IPV6_INTERFACE_MAP.md`

## PHASE 13: phase12 Option B and C stubs (DP-409)
**Goal**: the two dead hooks and their feature flags go. The built code, maps and relocations do not change. **Found**: neither feature is enabled by any build; `phase12_option_a_on_context_switch` and the no-op stay.
**Prerequisite**: Phase 7 (same workspace). **Time**: 25m. **Agent**: eBPF lane. **Model**: Sonnet

- [ ] **Step 73** [B] ~6m: Hash the object first, per section. Line numbers move, so `.debug*` and `.BTF.ext` differ even when the code does not; code, maps and relocations must not.
```bash
cat > /var/tmp/s-clean-cut-elfsec.py <<'PY'
import hashlib, re, struct, sys
SKIP = re.compile(r'^(\.debug|\.rel\.debug|\.BTF\.ext|\.rel\.BTF\.ext)')
b = open(sys.argv[1], 'rb').read()
shoff = struct.unpack_from('<Q', b, 0x28)[0]
ent, num, stri = struct.unpack_from('<HHH', b, 0x3A)
S = [struct.unpack_from('<IIQQQQIIQQ', b, shoff + i * ent) for i in range(num)]
names = S[stri][4]
for s in S:
    name = b[names + s[0]:b.index(b'\0', names + s[0])].decode()
    if SKIP.match(name):
        continue
    data = b[s[4]:s[4] + s[5]] if s[1] != 8 else b''
    print(name, s[5], hashlib.sha256(data).hexdigest())
PY
cd ~/tmp/wt/s-clean-cut/ebpf && export CARGO_TARGET_DIR=/var/tmp/s-clean-cut-p13 && cargo build --release -j4 -p monad-cpu-ebpf --features ascend-linux 2>&1 | tail -1 && python3 /var/tmp/s-clean-cut-elfsec.py "$(find "$CARGO_TARGET_DIR" -type f -name 'monad-cpu-ebpf' -path '*release*' | head -1)" | sort > /var/tmp/s-clean-cut-p13.before; wc -l < /var/tmp/s-clean-cut-p13.before
```
  Expect `Finished` and a section count above `5`.
- [ ] **Step 74** [W] ~3m: Remove the stubs.
```bash
cd ~/tmp/wt/s-clean-cut && python3 ~/tmp/wt/bin/sub.py ebpf/monad-cpu-ebpf/src/phase12.rs <<'SUB'
<<<<
// ───────────────────────────────────────────────────────────────────────
// Option B — ASID-tagged shared pgd (NOT CHOSEN; stub for measurement)
// ───────────────────────────────────────────────────────────────────────

#[cfg(feature = "phase12-option-b")]
#[inline(always)]
pub fn phase12_option_b_on_context_switch(_cpu: &mut MbcCpuState, _new_asid: u8) {
    // Path-not-taken per ADR-074 pair-call. Documented for completeness.
    // Would have required:
    //   1. Add cpu.current_asid: u8 (1 new field on MbcCpuState — wire-format-compatible default 0)
    //   2. Widen TLB_MAP from Array<[u32; 3]> to Array<[u32; 4]> (ASID column)
    //   3. Update translate_address() TLB lookup to filter by ASID
    //   4. Context switch becomes a 1-byte write (no flush)
}

// ───────────────────────────────────────────────────────────────────────
// Option C — per-task pgd + pid-tagged TLB (NOT CHOSEN; future extension)
// ───────────────────────────────────────────────────────────────────────

#[cfg(feature = "phase12-option-c")]
#[inline(always)]
pub fn phase12_option_c_on_context_switch(_cpu: &mut MbcCpuState, _new_pid: u8) {
    // Path-not-taken per ADR-074 pair-call. Can be added LATER as a
    // non-breaking extension on top of Option A if defense-in-depth is
    // needed. Would have required:
    //   1. Option A's full path (pgd swap + flush)
    //   2. Plus widen TLB_MAP to include pid column
    //   3. Plus update translate_address() to filter by pid (belt-and-suspenders)
}

====
>>>>
<<<<
#[cfg(not(any(
    feature = "phase12-option-a",
    feature = "phase12-option-b",
    feature = "phase12-option-c"
)))]
#[inline(always)]
#[allow(dead_code)]
====
#[cfg(not(feature = "phase12-option-a"))]
#[inline(always)]
#[allow(dead_code)]
>>>>
SUB
python3 ~/tmp/wt/bin/sub.py ebpf/monad-cpu-ebpf/src/phase12.rs <<'SUB'
<<<<
//! Three feature flags — `phase12-option-a`, `phase12-option-b`,
//! `phase12-option-c` — gate stub functions for the three options
//! enumerated in ADR-074. The pair-call (2026-05-12) chose **Option A**;
//! that's the implementation path. Options B and C remain as no-op
//! stubs both for verifier-budget measurement and as documentation of
//! the paths-not-taken.
====
//! ADR-074's pair-call (2026-05-12) chose **Option A**, gated by
//! `phase12-option-a`. The Option B and C stubs were deleted 2026-11-04
//! (DP-409); ADR-074 keeps the paths not taken.
>>>>
SUB
python3 ~/tmp/wt/bin/sub.py ebpf/monad-cpu-ebpf/src/main.rs <<'SUB'
<<<<
    // Hook receives the *new* pid and may further mutate cpu (e.g. ASID
    // for Option B). The save+load above already established the basic
    // Option-A state; the hook is the extension point.
    #[cfg(feature = "phase12-option-a")]
    phase12::phase12_option_a_on_context_switch(cpu, next_pid as u8);
    #[cfg(feature = "phase12-option-b")]
    phase12::phase12_option_b_on_context_switch(cpu, next_pid as u8);
    #[cfg(feature = "phase12-option-c")]
    phase12::phase12_option_c_on_context_switch(cpu, next_pid as u8);
    #[cfg(not(any(
        feature = "phase12-option-a",
        feature = "phase12-option-b",
        feature = "phase12-option-c"
    )))]
    phase12::phase12_noop_on_context_switch(cpu, next_pid as u8);
====
    // Hook receives the *new* pid. The save+load above already established
    // the basic Option-A state; the hook is the extension point.
    #[cfg(feature = "phase12-option-a")]
    phase12::phase12_option_a_on_context_switch(cpu, next_pid as u8);
    #[cfg(not(feature = "phase12-option-a"))]
    phase12::phase12_noop_on_context_switch(cpu, next_pid as u8);
>>>>
SUB
python3 ~/tmp/wt/bin/sub.py ebpf/monad-cpu-ebpf/Cargo.toml <<'SUB'
<<<<
# Phase 1.2 pre-work (ADR-074). All three are NO-OPs today;
# they exist so the pair-call can build all variants for verifier measurement.
# Pair-call landed 2026-05-12: Option A chosen. Option B/C stubs kept for
# regression measurement and as documentation of the path-not-taken.
phase12-option-a = []  # per-task pgd (CHOSEN per ADR-074 Decision 2026-05-12)
phase12-option-b = []  # ASID-tagged shared pgd (stub for measurement)
phase12-option-c = []  # priv_level + pid hybrid (stub for measurement)
====
# Phase 1.2 (ADR-074): Option A chosen 2026-05-12. The Option B and C stubs
# were deleted 2026-11-04 (DP-409); ADR-074 keeps the path not taken.
phase12-option-a = []  # per-task pgd (CHOSEN per ADR-074 Decision 2026-05-12)
>>>>
SUB
git grep -c -E 'phase12-option-[bc]|phase12_option_[bc]' -- ebpf; echo rc=$?
```
  Expect four `sub.py` lines, `rc=1`.
- [ ] **Step 75** [W] ~1m: ADR-074's recipe names the flags: add one line under its title block.
```bash
cd ~/tmp/wt/s-clean-cut && python3 - <<'PY'
p='docs/adr/ADR-074-phase12-page-table-model.md'; s=open(p).read()
note='\n> **2026-11-04 (DP-409):** the `phase12-option-b` and `phase12-option-c` features and their stubs were deleted; the measurement commands below name them as history.\n'
i=s.index('\n',s.index('# ADR-074'))
s=s[:i+1]+note+s[i+1:]; open(p,'w').write(s); print('ADR-074-OK')
PY
```
  Expect `ADR-074-OK`.
- [ ] **Step 76** [V] ~6m: Same code.
```bash
cd ~/tmp/wt/s-clean-cut/ebpf && export CARGO_TARGET_DIR=/var/tmp/s-clean-cut-p13 && cargo build --release -j4 -p monad-cpu-ebpf --features ascend-linux 2>&1 | tail -1 && python3 /var/tmp/s-clean-cut-elfsec.py "$(find "$CARGO_TARGET_DIR" -type f -name 'monad-cpu-ebpf' -path '*release*' | head -1)" | sort | diff - /var/tmp/s-clean-cut-p13.before && echo SAME-OBJECT
```
  Expect `Finished`, `SAME-OBJECT`. Any section differs: `[STUCK]`, keep the edit uncommitted, tell the coordinator; the phase then needs the verifier gate run by Opus.
- [ ] **Step 77** [C] ~1m: **PHASE 13 EXIT GATE + COMMIT.** `~/tmp/wt/bin/plan-commit s-clean-cut S-CLEAN-CUT "Steps 73-77: delete phase12 Option B and C stubs and flags (DP-409)" ebpf/monad-cpu-ebpf/src/phase12.rs ebpf/monad-cpu-ebpf/src/main.rs ebpf/monad-cpu-ebpf/Cargo.toml docs/adr/ADR-074-phase12-page-table-model.md`

## PHASE 15: mTLS authenticator (DP-410)
**Goal**: `MTLSValidator`, `ErrInvalidCert` and their tests go; the README stops promising them. **Found**: no caller outside `pkg/auth`. `pkg/transport/mtls` and `pkg/tls` are transport, not this authenticator, and stay: `pkg/tls/pq.go` and Wotan's links use them. `cmd/lich-security` probes transport.
**Prerequisite**: Phase 0. **Time**: 15m. **Agent**: Lane L. **Model**: Haiku

- [ ] **Step 78** [W] ~3m: Code.
```bash
cd ~/tmp/wt/s-clean-cut && python3 - <<'PY'
s=open('pkg/auth/auth.go').read()
a=s.index('// MTLSValidator validates client certificates'); b=s.index('// Middleware returns an HTTP middleware')
s=s[:a]+s[b:]
for old in ('//   - mTLS certificate verification (stub — planned for Beta)\n','	ErrInvalidCert     = errors.New("auth: invalid client certificate")\n'):
    assert s.count(old)==1, old
    s=s.replace(old,'')
open('pkg/auth/auth.go','w').write(s)
t=open('pkg/auth/auth_test.go').read()
a=t.index('// --- MTLSValidator Stub Tests ---'); b=t.index('// --- Identity.HasRole Tests ---'); t=t[:a]+t[b:]
a=t.index('func TestMiddleware_NoAuth_MTLS'); b=t.index('// mockAuthenticator is a test helper'); t=t[:a]+t[b:]
assert t.count('ErrTokenExpired, ErrInvalidCert, ')==1
t=t.replace('ErrTokenExpired, ErrInvalidCert, ','ErrTokenExpired, ')
for imp in ('	"crypto/tls"\n','	"crypto/x509"\n','	"crypto/x509/pkix"\n'):
    name=imp.strip().strip('"').split('/')[-1]
    if name+'.' not in t:
        t=t.replace(imp,'')
open('pkg/auth/auth_test.go','w').write(t); print('AUTH-OK')
PY
gofmt -l pkg/auth; flock ~/tmp/wt/.s-clean-cut.build go vet ./pkg/auth/ && flock ~/tmp/wt/.s-clean-cut.build go test ./pkg/auth/ 2>&1 | tail -1
```
  Expect `AUTH-OK`, no `gofmt` line, a line starting `ok`.
- [ ] **Step 79** [W] ~3m: `pkg/auth/README.md`.
```bash
cd ~/tmp/wt/s-clean-cut && python3 ~/tmp/wt/bin/sub.py pkg/auth/README.md <<'SUB'
<<<<
### 2. mTLS (Inter-Service Communication)

- Used by: All service-to-service calls within the Kingdom
- Flow: Client presents certificate during TLS handshake
- Validation: Certificate chain against Kingdom CA, CN/SAN matching
- Identity: CN = service name (e.g. `timeguru`, `captain`)
- Certificate management: Short-lived certs via SOPS + age, auto-rotation
====
### 2. Service tokens (Inter-Service Communication)

- Used by: All service-to-service calls within the Kingdom (ADR-103)
- Flow: each binary mints a 5-minute EdDSA token per target; TLS 1.3 carries it
- Validation: the caller's `.pub` in `trust.d`, the caller in `AUTH_ALLOW_CALLERS`
- Identity: `sub` = key name (e.g. `timeguru`, `captain`)
>>>>
<<<<

// Example: protect inter-service routes with mTLS
mux.Handle("/internal/", auth.Middleware(mtlsValidator)(internalHandler))
====
>>>>
<<<<
2. **Beta:** JWT validation with JWKS, mTLS with auto-generated certs
====
2. **Beta:** JWT validation with JWKS
>>>>
SUB
```
  Expect `sub.py: pkg/auth/README.md: 3 block(s) applied`.
- [ ] **Step 80** [C] ~1m: **PHASE 15 EXIT GATE + COMMIT.** `git -C ~/tmp/wt/s-clean-cut grep -c 'MTLSValidator' -- '*.go'` prints nothing, then `~/tmp/wt/bin/plan-commit s-clean-cut S-CLEAN-CUT "Steps 78-80: delete the mTLS authenticator; service tokens are the S2S auth (DP-410)" pkg/auth/auth.go pkg/auth/auth_test.go pkg/auth/README.md`

## PHASE 18: zhend claims in the crypto inventory (DP-209)
**Goal**: the CBOM stops crediting zhend with a live QUIC listener. The crate and `quic.rs` stay. The inventory is generated, so the fix is in the scanner; the file is regenerated in Phase 23.
**Prerequisite**: Phase 0. **Time**: 20m. **Agent**: Lane O. **Model**: Sonnet

**Signature**: `func applyNotSpawned(a *Asset)` in `cmd/crypto-inventory/scan_rust.go`. Sets `a.LinkState = linkNotSpawned` and appends `"; " + why` to `a.Detail` when `a.Path` is a key of `notSpawned map[string]string`; otherwise leaves `a` unchanged.

- [ ] **Step 81** [W] ~3m: Test first.
```bash
cd ~/tmp/wt/s-clean-cut && cat >> cmd/crypto-inventory/scan_rust_test.go <<'GO'

// zhend is single-node gRPC-only (DP-209): quic.rs compiles and never runs, so
// the inventory must not read as a live listener.
func TestApplyNotSpawned(t *testing.T) {
	a := Asset{Path: "crates/zhend/src/api/quic.rs", Detail: "aws-lc-rs provider"}
	applyNotSpawned(&a)
	if a.LinkState != linkNotSpawned {
		t.Errorf("quic.rs link state = %q, want %q", a.LinkState, linkNotSpawned)
	}
	if !strings.Contains(a.Detail, "never spawns") {
		t.Errorf("detail = %q, want the reason", a.Detail)
	}
	b := Asset{Path: "crates/zhend/src/main.rs", Detail: "aws-lc-rs provider"}
	applyNotSpawned(&b)
	if b.LinkState != "" || b.Detail != "aws-lc-rs provider" {
		t.Errorf("a live file changed: %+v", b)
	}
}
GO
go test ./cmd/crypto-inventory/ -run TestApplyNotSpawned 2>&1 | grep -E 'undefined: (applyNotSpawned|linkNotSpawned)' | wc -l
```
  Expect `2` or more lines counted (red: the names do not exist).
- [ ] **Step 82** [W] ~3m: Implement.
```bash
cd ~/tmp/wt/s-clean-cut && python3 ~/tmp/wt/bin/sub.py cmd/crypto-inventory/cbom.go <<'SUB'
<<<<
	linkUnusedFeature = "unused-feature"
====
	linkUnusedFeature = "unused-feature"
	linkNotSpawned    = "not-spawned" // compiled, but the binary never starts it
>>>>
SUB
python3 ~/tmp/wt/bin/sub.py cmd/crypto-inventory/scan_rust.go <<'SUB'
<<<<
		a.Scanner = "rust"
		a.Owner = ownerFirstParty
		if a.LinkState == "" {
====
		a.Scanner = "rust"
		a.Owner = ownerFirstParty
		applyNotSpawned(&a)
		if a.LinkState == "" {
>>>>
<<<<
func crateProviderStatus(c *crate, rustlsVer string) (string, string) {
====
// notSpawned names first-party files whose crypto compiles into a binary that
// never starts it, with the reason. A row stays true of the file and stops
// claiming a live listener.
var notSpawned = map[string]string{
	"crates/zhend/src/api/quic.rs": "zhend's main never spawns the QUIC server; zhend is single-node gRPC-only (DP-209)",
	"crates/zhend/Cargo.toml":      "zhend's main never spawns the QUIC server; zhend is single-node gRPC-only (DP-209)",
}

// applyNotSpawned marks an asset whose file is in notSpawned.
func applyNotSpawned(a *Asset) {
	why, ok := notSpawned[a.Path]
	if !ok {
		return
	}
	a.LinkState = linkNotSpawned
	a.Detail += "; " + why
}

func crateProviderStatus(c *crate, rustlsVer string) (string, string) {
>>>>
SUB
gofmt -l cmd/crypto-inventory; go test ./cmd/crypto-inventory/ 2>&1 | tail -1
```
  Expect two `sub.py` lines, no `gofmt` line, a line starting `ok` (green).
- [ ] **Step 83** [W] ~1m:
```bash
cd ~/tmp/wt/s-clean-cut && python3 ~/tmp/wt/bin/sub.py wiki/Post-Quantum-Key-Exchange.md <<'SUB'
<<<<
| WAF (`cmd/waf`), zhend QUIC (`crates/zhend`) | Preferred (rustls, aws-lc-rs) |
====
| zhend QUIC (`crates/zhend`), compiled and never spawned | Preferred (rustls, aws-lc-rs) |
>>>>
SUB
```
  Expect `sub.py: wiki/Post-Quantum-Key-Exchange.md: 1 block(s) applied`.
- [ ] **Step 84** [C] ~1m: **PHASE 18 EXIT GATE + COMMIT.** `~/tmp/wt/bin/plan-commit s-clean-cut S-CLEAN-CUT "Steps 81-84: crypto inventory marks zhend QUIC not-spawned (DP-209)" cmd/crypto-inventory/cbom.go cmd/crypto-inventory/scan_rust.go cmd/crypto-inventory/scan_rust_test.go wiki/Post-Quantum-Key-Exchange.md`

## PHASE 19: vLLM repo leftovers and the note (DP-713)
**Goal**: `lxd/vllm-rocm` gone, the service rows and alert gone, the note written. **Stays until the docs sweep**: `lxd/INDEX.md`, `lxd/CREATED_FILES.md`, `lxd/FILES_SUMMARY.txt`, `docs/research/*`, `docs/bare-metal/*`.
**Prerequisite**: Phase 2 for `lxd/README.md` and `lxd/INDEX.md` (same files). **Time**: 15m. **Agent**: Lane B after Phase 2. **Model**: Haiku

- [ ] **Step 85** [B] ~1m: `cd ~/tmp/wt/s-clean-cut && rm -rf lxd/vllm-rocm && test ! -e lxd/vllm-rocm && echo GONE` Expect `GONE`.
- [ ] **Step 86** [W] ~1m: `lxd/README.md`; the launchers are Phase 22A.
```bash
cd ~/tmp/wt/s-clean-cut && sed -i '/vllm/Id' lxd/README.md && git grep -c -i vllm -- lxd/README.md; echo rc=$?
```
  Expect `rc=1` and nothing else.
- [ ] **Step 87** [W] ~2m: Service rows and the alert.
```bash
cd ~/tmp/wt/s-clean-cut && python3 ~/tmp/wt/bin/sub.py configs/services.yaml <<'SUB'
<<<<
  vllm-deepseek:
    host: localhost
    port: 20100
    proto: http
    display_name: "vLLM DeepSeek-R1"
    tier: applications
  vllm-qwen:
    host: localhost
    port: 20101
    proto: http
    display_name: "vLLM Qwen Coder"
    tier: applications
====
>>>>
SUB
python3 ~/tmp/wt/bin/sub.py docker/telemetry/prometheus/rules/unheaded.yml <<'SUB'
<<<<
      # LLM inference stalled alerts
      - alert: LLMInferenceStalled
        expr: |
          rate(vllm_generation_tokens_total[1m]) == 0
        for: 60s
        labels:
          severity: critical
          service: unheaded
        annotations:
          summary: "LLM inference stalled on {{ $labels.instance }}"
          description: "vLLM token generation rate is zero on {{ $labels.instance }}. Inference pipeline may be frozen."

====
>>>>
SUB
go test ./pkg/discovery/... 2>&1 | tail -1
```
  Expect two `sub.py` lines and a line starting `ok` or `?`.
- [ ] **Step 88** [W] ~2m: The note.
```bash
cd ~/tmp/wt/s-clean-cut && python3 - <<'PY'
p='docs/architecture/AI_STACK.md'; s=open(p).read()
i=s.index('\n',s.index('> **Design never built'))
note=('\n>\n> **vLLM is parked (DP-713, 2026-11-04).** llama-server is the one serving path on the GPU lease. vLLM needs a better GPU than west has: '
      'a used card in a small lab server would bring it back, and that is the condition for reopening this. No record shows it ever served on west. The repo pieces '
      '(`lxd/vllm-rocm`, `docker/vllm-rocm`, `nixos/modules/vllm-rocm.nix`, the two sophia-eye services) are in git history at `d6ad43e2`.')
s=s[:i]+note+s[i:]; open(p,'w').write(s); print('NOTE-OK')
PY
```
  Expect `NOTE-OK`.
- [ ] **Step 89** [C] ~1m: **PHASE 19 EXIT GATE + COMMIT.** `~/tmp/wt/bin/plan-commit s-clean-cut S-CLEAN-CUT "Steps 85-89: vLLM leaves the live tree; a better GPU brings it back (DP-713)" lxd/vllm-rocm lxd/README.md configs/services.yaml docker/telemetry/prometheus/rules/unheaded.yml docs/architecture/AI_STACK.md`

## PHASE 20: sophia-eye without vLLM (DP-713)
**Goal**: the stack parses and its gates pass with `vllm` and `qwen-coder` removed. **Judgement**: `open-webui` and `sophia-gateway` lose their backend. Point both at llama-server through one required variable; do not invent a new service. ADR-117 will fold this stack later, so keep the change small.
**Prerequisite**: Phase 3 (baselines). **Time**: 40m. **Agent**: Opus lane. **Model**: Opus

- [ ] **Step 90** [W] ~15m: `deploy/sophia-eye/docker-compose.yml`: delete services `vllm` and `qwen-coder`; delete volumes `vllm-cache`, `qwen-cache`; in `volume-init` delete their two mounts and two `own` lines; delete the `vllm:` and `qwen-coder:` entries from `depends_on` of `open-webui` and `sophia-gateway`. Add `extra_hosts: ["host.docker.internal:host-gateway"]` to those two and set `OPENAI_API_BASE_URL: "${LLAMA_SERVER_URL:?set LLAMA_SERVER_URL to the llama-server base URL}/v1"`, `VLLM_ENDPOINT` and `QWEN_ENDPOINT` to `"${LLAMA_SERVER_URL:?...}"`. Keep the env names: `sophia-gateway/main.go` reads them and renaming is ADR-117's sprint.
- [ ] **Step 91** [W] ~8m: `deploy/sophia-eye/prometheus.yml`: delete the `vllm` job and its comment. `deploy/sophia-eye/shield-rules.yaml`: delete the rules for the two vLLM endpoints and `allow-vllm-internal-communication`; remove `vllm-deepseek` and `vllm-qwen` from the remaining source lists; in `policy-groups.inference-service` delete the two lines `allow-inference-deepseek-ingress` and `allow-inference-qwen-ingress`. Read the file first: rules that then have an empty source list go too.
- [ ] **Step 92** [W] ~5m: `pkg/uids`: `VLLM` stays while `kubernetes/manifests/overlays/gpu-vllm` exists (S-MOVE-OUT retires it). Delete `"qwen-coder":        true,` from `composeOnly` in `pkg/uids/uids_test.go`: `TestComposeOnlyListed` fails once the compose service is gone. Change the `QwenCoder` comment in `uids.go` to retired and keep the number reserved (the registry never reuses a uid; see the Suricata line at 16782).
- [ ] **Step 93** [V] ~5m: `cd ~/tmp/wt/s-clean-cut && LLAMA_SERVER_URL=http://127.0.0.1:8081 QDRANT_API_KEY_FILE=/dev/null WEBUI_SECRET_KEY_FILE=/dev/null docker compose -f deploy/sophia-eye/docker-compose.yml config -q && echo COMPOSE-OK; ./scripts/check-compose-hardening.sh 2>&1 | tail -1; go test ./pkg/uids/ 2>&1 | tail -1; git grep -c -i 'vllm' -- deploy/sophia-eye/docker-compose.yml` Expect `COMPOSE-OK`, a PASS line, `ok`, and a count of 1 (`VLLM_ENDPOINT`; `QWEN_ENDPOINT` does not match). This validates the file only: nothing starts (rule 5).
- [ ] **Step 94** [C] ~1m: **PHASE 20 EXIT GATE + COMMIT.** `~/tmp/wt/bin/plan-commit s-clean-cut S-CLEAN-CUT "Steps 90-94: sophia-eye points at llama-server; vLLM services gone (DP-713)" deploy/sophia-eye/docker-compose.yml deploy/sophia-eye/prometheus.yml deploy/sophia-eye/shield-rules.yaml pkg/uids/uids.go pkg/uids/uids_test.go`

## PHASE 21: the parked WAF ADR and the sweep rows (DP-709)
**Goal**: ADR-121 parked at lowest priority (ADR-120 is taken; 125-134 are reserved); the eleven FLAGGED rows closed; ADR notes for AF_XDP.
**Prerequisite**: Phases 1, 4, 7 merged in the worktree (commit hashes needed). **Time**: 15m. **Agent**: Lane P. **Model**: Haiku

- [ ] **Step 95** [W] ~3m: The ADR.
```bash
cd ~/tmp/wt/s-clean-cut && cat > docs/adr/ADR-121-a-future-waf.md <<'MD'
<!--
SPDX-License-Identifier: GPL-3.0-or-later
Copyright (c) 2024-2026 Stevie Bellis.
-->
# ADR-121: A future WAF (idea, lowest priority)

**Status:** Proposed (parked under ADR-112; lowest priority of the parked ideas). Nothing is built, planned or scheduled.
**Date:** 2026-11-04
**Grounded on:** `develop` at `d6ad43e2`.
**Related:** DP-401.1, DP-709, round 4 of the decision pack, ADR-113 (third-party verdicts), ADR-115 (Shop).

## Context

`pkg/waf` (seven Go packages, no caller) and `cmd/waf` (a Rust binary named `shield`: rule engine, rate limiter, TLS on rustls with aws-lc-rs; no unit, compose service or importer ran it) left the repo in S-CLEAN-CUT. Stevie's word of 2026-11-04: a WAF stays a future idea, and any open-source WAF sits outside the repo, after the third-party sitting. The Go `cmd/shield` stays and is not a WAF. Both deleted trees are in git history at `d6ad43e2`.

## The idea

Small shops get request filtering at the edge without writing security code: an outside open-source WAF run beside the edge, as a separate deployment, admitted at the third-party sitting.

## Open question

Which WAF, and does it serve ADR-115's Shop?

No decision is made here.
MD
python3 - <<'PY'
row='| [121](ADR-121-a-future-waf.md) | A future WAF: idea only, lowest priority; `pkg/waf` and `cmd/waf` deleted, any open-source WAF sits outside the repo | **Proposed** (parked) | 2026-11-04 |\n'
p='docs/adr/ADR-INDEX.md'; s=open(p).read().rstrip('\n')+'\n'+row; open(p,'w').write(s)
p='wiki/ADR-Index.md'; L=open(p).read().split('\n')
i=max(k for k,l in enumerate(L) if l.startswith('| [120]'))
L.insert(i+1,'| [121](../docs/adr/ADR-121-a-future-waf.md) | A future WAF: idea only, lowest priority | Proposed (parked) | 2026-11-04 |'); open(p,'w').write('\n'.join(L)); print('INDEX-OK')
PY
```
  Expect `INDEX-OK`. Check the row shape against the row above it with `tail -2 docs/adr/ADR-INDEX.md | cut -c1-60` and `grep -n '^| \[120\]' wiki/ADR-Index.md | cut -c1-60`; a different column count: `[STUCK]`.
- [ ] **Step 96** [W] ~3m: Close the TSV rows.
```bash
cd ~/tmp/wt/s-clean-cut && WAF=$(git log --format=%h --grep='delete pkg/waf' -1) && MAP=$(git log --format=%h --grep='internal/bpfmap and pkg/ebpf/maploader' -1) && test -n "$WAF" -a -n "$MAP" && python3 - "$WAF" "$MAP" <<'PY'
import sys
waf, mp = sys.argv[1:3]
p='docs/sweeps/REAL-STEEL-UNREACHED.tsv'
keep={'unheaded/pkg/secrets/store':'DP-709: kept, the only Store implementation',
      'unheaded/pkg/secrets/encryption':'DP-709: kept, imported by pkg/secrets/store',
      'unheaded/pkg/audit/storage':'DP-709: kept, live importers cmd/zhen-claude and cmd/zhen-agentd',
      'unheaded/pkg/trace':'DP-709: kept, fixture of the tests/e2e live-pipeline cases'}
out=[]; n=0
for line in open(p,encoding='utf-8').read().split('\n'):
    f=line.split('\t')
    if len(f)>=7:
        if f[0] in keep and f[4]=='FLAGGED':
            f[4]='KEEP'; f[6]+='; '+keep[f[0]]; n+=1
        elif f[0].startswith('unheaded/pkg/waf') and f[4]=='FLAGGED':
            f[4]='DELETED'; f[5]=waf; f[6]+='; DP-401.1'; n+=1
        elif f[0] in ('unheaded/internal/bpfmap','unheaded/pkg/ebpf/maploader') and f[4]=='PARK':
            f[4]='DELETED'; f[5]=mp; f[6]+='; DP-401.4'; n+=1
    out.append('\t'.join(f))
open(p,'w',encoding='utf-8').write('\n'.join(out)); print('rows closed',n)
PY
cut -f5 docs/sweeps/REAL-STEEL-UNREACHED.tsv | grep -c '^FLAGGED$'; echo rc=$?
```
  Expect `rows closed 13`, `0`, `rc=1`.
- [ ] **Step 97** [W] ~3m: ADR notes: ADR-067, ADR-119 (the map must be declared anew), ADR-105 and ADR-004 (where rustls and aws-lc-rs were pinned to `cmd/waf`).
```bash
cd ~/tmp/wt/s-clean-cut && python3 - <<'PY'
for p,note in (('docs/adr/ADR-067-mbc-isa-v2-and-upc-abi-v1.md','> **History, 2026-11-04 (DP-205):** AF_XDP was retired: `pkg/afxdp`, `ebpf/af-xdp`, `ebpf/af-xdp-common` and `ebpf/xdp-redirect` deleted, the branches cut from shield and packet-marker. Doom never used it. XDP is unchanged.'),
               ('docs/adr/ADR-119-shield-xdp-pqc-fast-path-design.md','> **2026-11-04 (DP-205):** `SHIELD_CONFIG` existed only for the AF_XDP toggle and was deleted with it. Every use below assumes the map exists: it must be declared anew when this is built.'),
               ('docs/adr/ADR-105-pq-hybrid-key-exchange-and-crypto-inventory.md','> **2026-11-04 (DP-401, round 4):** `cmd/waf` was deleted, so H6, H18 and the `cmd/waf` half of decision 3 are history. `crates/zhend` stays on rustls with aws-lc-rs, compiled and never spawned (DP-209); the Rust provider decision holds for it.'),
               ('docs/adr/ADR-004-no-external-deps-policy.md','> **2026-11-04 (DP-401, round 4):** `cmd/waf` was deleted. `aws-lc-rs` and `aws-lc-sys` stay admitted for `crates/zhend` alone, which uses them directly.')):
    s=open(p).read(); i=s.index('\n',s.index('# ADR-'))
    s=s[:i+1]+'\n'+note+'\n'+s[i+1:]; open(p,'w').write(s)
p='docs/adr/ADR-004-no-external-deps-policy.md'; s=open(p).read()
old="rustls' crypto provider in `cmd/waf` and `crates/zhend`:"
assert s.count(old)==1
open(p,'w').write(s.replace(old,"rustls' crypto provider in `crates/zhend` (`cmd/waf` deleted 2026-11-04):"))
print('NOTES-OK')
PY
```
  Expect `NOTES-OK`.
- [ ] **Step 98** [C] ~1m: **PHASE 21 EXIT GATE + COMMIT.** `~/tmp/wt/bin/plan-commit s-clean-cut S-CLEAN-CUT "Steps 95-98: ADR-121 parked; eleven flagged sweep rows closed (DP-709)" docs/adr/ADR-121-a-future-waf.md docs/adr/ADR-INDEX.md wiki/ADR-Index.md docs/sweeps/REAL-STEEL-UNREACHED.tsv docs/adr/ADR-067-mbc-isa-v2-and-upc-abi-v1.md docs/adr/ADR-119-shield-xdp-pqc-fast-path-design.md docs/adr/ADR-105-pq-hybrid-key-exchange-and-crypto-inventory.md docs/adr/ADR-004-no-external-deps-policy.md`

## PHASE 22: shared ledgers (all answers)
**Goal**: every file several answers touch, edited once by one agent. **Rule**: this phase starts only after every lane has committed.
**Prerequisite**: Phases 1-21 committed or `[STUCK]`. **Time**: 30m. **Agent**: one. **Model**: Haiku

- [ ] **Step 99** [W] ~5m: `Makefile`.
```bash
cd ~/tmp/wt/s-clean-cut && python3 ~/tmp/wt/bin/sub.py Makefile <<'SUB'
<<<<
build-trace-collector: ## Build trace-collector (Rust)
	@echo "Building trace-collector (Rust)..."
	@mkdir -p $(BINARY_DIR)
	cd cmd/trace-collector && cargo build $(CARGO_BUILD_FLAGS)
	cp cmd/trace-collector/target/release/trace-collector $(BINARY_DIR)/
	@echo "✓ trace-collector built"

====
>>>>
<<<<
test-rust-ebpf-common: ## Run host-runnable tests in ebpf/{common,monad-common,pqc-common,af-xdp-common}
====
test-rust-ebpf-common: ## Run host-runnable tests in ebpf/{common,monad-common,pqc-common}
>>>>
<<<<
	@for crate in common monad-common pqc-common af-xdp-common; do \
====
	@for crate in common monad-common pqc-common; do \
>>>>
<<<<
test-rust-ebpf-af-xdp: ## Run ebpf/af-xdp tests (excluded crate with own host config)
	cd ebpf/af-xdp && cargo test --locked

====
>>>>
<<<<
build-shield: ## Build shield (cmd/waf — WAF / Zero-Trust gateway)
	@echo "Building shield (cmd/waf)..."
	@mkdir -p $(BINARY_DIR)
	cd cmd/waf && cargo build $(CARGO_BUILD_FLAGS)
	cp cmd/waf/target/release/shield $(BINARY_DIR)/

====
>>>>
<<<<
test-rust-shield: ## Run shield (cmd/waf) WAF rule-engine tests
	cd cmd/waf && cargo test --locked

====
>>>>
<<<<
test-rust-trace-collector: ## Run trace-collector tests
	cd cmd/trace-collector && cargo test --locked

====
>>>>
<<<<
	rm -rf cmd/trace-collector/target
====
>>>>
<<<<
	cd cmd/trace-collector && cargo fmt
====
>>>>
<<<<
	cd cmd/trace-collector && cargo clippy --locked
====
>>>>
<<<<
	cd cmd/trace-collector && cargo fetch
====
>>>>
SUB
sed -i 's/ test-rust-ebpf-af-xdp//; s/ test-rust-trace-collector//; s/ build-trace-collector//; s/ test-rust-shield//; s/ build-shield//' Makefile
grep -c -E 'af-xdp|cmd/trace-collector[^-]|build-trace-collector|cmd/waf|build-shield' Makefile; echo rc=$?; make -n help >/dev/null && echo MAKE-OK
```
  Expect `sub.py: Makefile: 11 block(s) applied`, `0`, `rc=1`, `MAKE-OK`.
- [ ] **Step 100** [W] ~2m: Dependabot, the Jenkins echo, the `shield-waf` design row.
```bash
cd ~/tmp/wt/s-clean-cut && sed -i 's#"/ebpf", "/ebpf/af-xdp", "/crates/\*", "/cmd/upc-bootctl", "/cmd/waf", "/cmd/trace-collector", #"/ebpf", "/crates/*", "/cmd/upc-bootctl", #' .github/dependabot.yml
sed -i 's#Cargo audit would run on: ebpf/ cmd/trace-collector/ crates/monad-mbc/#Cargo audit would run on: ebpf/ crates/monad-mbc/#' jenkins/Jenkinsfile.security
python3 - <<'PY'
import re
p='services.yaml'; s=open(p).read()
s2,n=re.subn(r'^  - service:\n      name: shield-waf\n.*?(?=^  - service:|^  # )','',s,count=1,flags=re.S|re.M)
open(p,'w').write(s2); print('shield-waf rows',n)
PY
python3 -c "import yaml;yaml.safe_load(open('services.yaml'));print('YAML-OK')"
git grep -c -E 'af-xdp|cmd/trace-collector|cmd/waf' -- .github/dependabot.yml jenkins/Jenkinsfile.security; echo rc=$?
```
  Expect `shield-waf rows 1`, `YAML-OK`, `rc=1`.
- [ ] **Step 101** [W] ~5m: `CLAUDE.md`.
```bash
cd ~/tmp/wt/s-clean-cut && python3 ~/tmp/wt/bin/sub.py CLAUDE.md <<'SUB'
<<<<
│   ├── trace-collector/   # eBPF → Wotan bridge (Rust)
====
│   ├── trace-collector-go/ # eBPF → Wotan bridge (Go; the Rust one retired 2026-11-04)
>>>>
<<<<
- eBPF AF_XDP pipeline: Rust FFI + Go bridge. 920K pps was a Python `sendto` ceiling, not AF_XDP (M-090); AF_XDP throughput not measured
====
- AF_XDP: retired 2026-11-04 (DP-205); XDP/TC with the supervised loader is the data plane. The old 920K pps figure was a Python `sendto` ceiling (M-090)
>>>>
<<<<
- Zhen Layer 0 (crates/zhend/): Anti-fragile gossip knowledge substrate, PQ crypto, 161 tests green
====
- Zhen Layer 0 (crates/zhend/): single-node gRPC only (DP-209); the QUIC, Monad bridge, topology and strata subsystems are written and never spawned; 161 tests green
>>>>
SUB
sed -i 's/rustls on aws-lc-rs (WAF, zhend QUIC)/rustls on aws-lc-rs (zhend, compiled and never spawned)/' CLAUDE.md
grep -c 'zhend QUIC' CLAUDE.md; echo rc=$?
```
  Expect `sub.py: CLAUDE.md: 3 block(s) applied`, `0`, `rc=1`.
- [ ] **Step 102** [W] ~6m: `THIRD_PARTY.md`, `LICENSES/THIRD_PARTY.md`, `SECURITY.md`, `QUICKSTART.md`, the wiki pages, `docs/architecture/RUST_COMPONENTS.md`.
```bash
cd ~/tmp/wt/s-clean-cut && sed -i '/^| `internal\/bpfmap\/` | /d; s/| Pure-Rust TLS for shield\/WAF |/| Pure-Rust TLS for zhend |/' THIRD_PARTY.md
python3 ~/tmp/wt/bin/sub.py QUICKSTART.md <<'SUB'
<<<<

# Benchmarks
go test -bench=. -benchmem ./pkg/waf/...
====
>>>>
<<<<
| vLLM | LLM inference | `docker pull rocm/vllm:latest` | `vllm-health` |
====
>>>>
<<<<

# 5. (Option A only) Start LLM inference
unheaded-download-model deepseek-ai/DeepSeek-R1-Distill-Qwen-7B deepseek-r1-7b --activate
unheaded-vllm-start /models/deepseek-r1-7b
====
>>>>
<<<<
- Review `pkg/waf/response/response.go:237` XSS fix
====
>>>>
SUB
python3 ~/tmp/wt/bin/sub.py wiki/Rust-Components.md <<'SUB'
<<<<
| `zhend` | Zhen Layer 0: gossip knowledge substrate with PQ crypto |
====
| `zhend` | Zhen Layer 0: single-node gRPC store; gossip and QUIC written, never spawned (DP-209) |
>>>>
SUB
sed -i '44s/`trace-collector`, //' wiki/Rust-Components.md
sed -i 's/^#### 2\. trace-collector (Rust binary)$/#### 2. trace-collector (retired 2026-11-04, DP-210; see `cmd\/trace-collector-go`)/; /^\*\*Location:\*\* `cmd\/trace-collector\/` (already exists as Cargo project)$/d' docs/architecture/RUST_COMPONENTS.md
sed -i 's/^Rust TLS (the WAF, zhend QUIC) uses rustls on the `aws-lc-rs` provider, which$/Rust TLS (zhend QUIC, compiled and never spawned) uses rustls on the `aws-lc-rs` provider, which/; s/the WAF, zhend QUIC/zhend QUIC (never spawned)/' SECURITY.md
sed -i "s/used in the Unheaded Kingdom's eBPF components, trace-collector, and MBC translator\./used in the Unheaded Kingdom's eBPF components and MBC translator./; s/| Async runtime for trace-collector |/| Async runtime |/; s/| gRPC framework for trace-collector → Wotan |/| gRPC framework for zhend |/; s/across trace-collector, monad-mbc, shield\/WAF, and ebpf-loader binaries\./across monad-mbc and ebpf-loader binaries./" THIRD_PARTY.md
sed -i 's/^\*\*Rust crates:\*\* trace-collector (28 deps), monad-mbc (3 deps), shield\/WAF (10 deps), ebpf-loader (5 deps)$/**Rust crates:** monad-mbc (3 deps), ebpf-loader (5 deps)/; s/^## Rust Dependencies (trace-collector, monad-mbc, shield\/WAF, ebpf-loader)$/## Rust Dependencies (monad-mbc, ebpf-loader)/; s/^Pure-Rust TLS implementation\. Used in shield\/WAF for TLS termination\.$/Pure-Rust TLS implementation. Used by zhend./; s/^- \*\*rustls\*\* - Pure-Rust TLS (shield\/WAF)$/- **rustls** - Pure-Rust TLS (zhend)/' LICENSES/THIRD_PARTY.md
sed -i '/^| `xdp-redirect` | XDP | AF_XDP redirect (with userspace `af-xdp`) |$/d; s/^Shared crates: `common`, `monad-common`, `pqc-common`, `af-xdp-common`\./Shared crates: `common`, `monad-common`, `pqc-common`./' wiki/eBPF-Programs.md
sed -i 's/^| `monad-common`, `common`, `pqc-common`, `af-xdp-common` | Shared wire types (Foundation draft), PQC auth types, AF_XDP types |$/| `monad-common`, `common`, `pqc-common` | Shared wire types (Foundation draft), PQC auth types |/; /^| `xdp-redirect`, `af-xdp` | Steer packets to AF_XDP sockets; userspace side |$/d' wiki/Rust-Components.md
sed -i '/^| Rust `cmd\/trace-collector` | `crates\/svc-token` | /d' wiki/Service-Auth.md
grep -c -E 'pkg/waf|vLLM' QUICKSTART.md; git grep -c -E 'af-xdp|xdp-redirect|shield/WAF|the WAF, zhend' -- SECURITY.md THIRD_PARTY.md LICENSES/THIRD_PARTY.md wiki/eBPF-Programs.md wiki/Rust-Components.md; git grep -c 'Rust `cmd/trace-collector`' -- wiki/Service-Auth.md; echo rc=$?
```
  Expect two `sub.py` lines (4 and 1 blocks), `0`, no `grep` file lines, `rc=1`.
- [ ] **Step 103** [W] ~4m: `docs/UNREACHED-GO-PACKAGES.md` and `docs/LIVE-PATHS.md`.
```bash
cd ~/tmp/wt/s-clean-cut && python3 ~/tmp/wt/bin/sub.py docs/UNREACHED-GO-PACKAGES.md <<'SUB'
<<<<
- `pkg/waf/*` (7, G10e): deleted, then restored under Step 62: `tests/s77`
  (G6 KEEP) asserts `pkg/waf/rules/engine.go`, `clientip_test.go` and
  `detection/bot_test.go` exist (`TestPhase1_BugFixes/Bug17_XFF_Spoofing`,
  `TestS77_AllDeliverables`). Needs Stevie's word on amending that kept test.
====
- `pkg/waf/*` (7, G10e): deleted 2026-11-04 with `tests/s77` amended (DP-401.1).
>>>>
<<<<
- F5 `internal/bpfmap`, `pkg/ebpf/maploader`: still parked. Phase 13 (Step 340)
  wrote the `CHAOS_TARGETS` loader on cilium/ebpf (already a dependency): these
  two wrap bpf(2) map ops on an fd and can neither load an object nor attach a
  TCX link.
====
- F5 `internal/bpfmap`, `pkg/ebpf/maploader`: deleted 2026-11-04 (DP-401.4).
  `pkg/afxdp` and `cmd/ebpf-exporter` went the same day (DP-205, DP-401.5).
>>>>
SUB
sed -i '/^- `pkg\/\(secrets\/encryption\|secrets\/store\|trace\)` (FLAGGED/s/(FLAGGED: /(kept, DP-709: /' docs/UNREACHED-GO-PACKAGES.md
sed -i '/^- `internal\/bpfmap` (F5, parked/d; /^- `pkg\/ebpf\/maploader` (F5, parked/d; /^- `pkg\/waf\(\/[a-z]*\)\?` (FLAGGED/d; /^- `pkg\/afxdp`/d; /^- `cmd\/ebpf-exporter`/d' docs/UNREACHED-GO-PACKAGES.md
sed -i '/^| `cmd\/ebpf-exporter` | ORPHAN |$/d; /^| `cmd\/trace-collector` | SUPERVISED |$/d; /^| `cmd\/waf` | TOOL |$/d' docs/LIVE-PATHS.md
git grep -c -E '^- `(pkg/waf|internal/bpfmap|pkg/ebpf/maploader).*(FLAGGED|parked)' -- docs/UNREACHED-GO-PACKAGES.md; echo rc=$?
```
  Expect the `sub.py` line with `2 block(s)`, `rc=1`.
- [ ] **Step 104** [C] ~1m: **PHASE 22 EXIT GATE + COMMIT.** `~/tmp/wt/bin/plan-commit s-clean-cut S-CLEAN-CUT "Steps 99-104: shared ledgers follow the deletions" Makefile .github/dependabot.yml jenkins/Jenkinsfile.security services.yaml CLAUDE.md THIRD_PARTY.md LICENSES/THIRD_PARTY.md SECURITY.md QUICKSTART.md wiki/Rust-Components.md wiki/eBPF-Programs.md wiki/Service-Auth.md docs/architecture/RUST_COMPONENTS.md docs/UNREACHED-GO-PACKAGES.md docs/LIVE-PATHS.md`

## PHASE 22A: scripts that reach root or a live host (DP-401.5, DP-210, DP-713)
**Goal**: the same one-line removals, in files rule 13 keeps from cheaper agents. Editing only: nothing here runs them. `bash -n` is the check.
**Prerequisite**: Phases 2, 9, 19. **Time**: 10m. **Agent**: Opus lane. **Model**: Opus

- [ ] **Step 105** [W] ~1m: `launch-all.sh` keeps a container whose definition is gone (`LAUNCH_EXAMPLES.sh` is the step after next).
```bash
cd ~/tmp/wt/s-clean-cut && sed -i '/^launch_container "ebpf-exporter" 1 256$/d; s/^for svc in prometheus victoriametrics loki grafana ebpf-exporter; do$/for svc in prometheus victoriametrics loki grafana; do/' lxd/hosts/host-a/launch-all.sh && bash -n lxd/hosts/host-a/launch-all.sh && grep -c 'ebpf-exporter' lxd/hosts/host-a/launch-all.sh
```
  Expect `1` (the log line on line 250 only; the next step fixes it).
- [ ] **Step 106** [W] ~1m: `cd ~/tmp/wt/s-clean-cut && sed -i '/Phase 10: Launching telemetry/s/, ebpf-exporter//' lxd/hosts/host-a/launch-all.sh && grep -c 'ebpf-exporter' lxd/hosts/host-a/launch-all.sh` Expect `0`. If `1`, `[STUCK]`.
- [ ] **Step 107** [W] ~2m: `lxd/Makefile`, `lxd/LAUNCH_EXAMPLES.sh` and `lxd/hosts/host-a/static-ips.yaml` lose vLLM and the exporter.
```bash
cd ~/tmp/wt/s-clean-cut && sed -i '/make launch-vllm/d; /^launch-vllm:$/,/^$/d; s/ launch-vllm//' lxd/Makefile
sed -i '/^launch_vllm_deepseek() {$/,/^}$/d; /launch_vllm_deepseek/d; /vllm-deepseek /d; /^launch_ebpf_exporter() {$/,/^}$/d; /launch_ebpf_exporter/d' lxd/LAUNCH_EXAMPLES.sh
sed -i '/^  ebpf-exporter:$/,/^    port: 9435$/d' lxd/hosts/host-a/static-ips.yaml
bash -n lxd/LAUNCH_EXAMPLES.sh && make -n -C lxd help >/dev/null && python3 -c "import yaml;yaml.safe_load(open('lxd/hosts/host-a/static-ips.yaml'))" && git grep -c -i -E 'vllm|ebpf.exporter' -- lxd/Makefile lxd/LAUNCH_EXAMPLES.sh lxd/hosts/host-a/static-ips.yaml; echo rc=$?
```
  Expect `rc=1`.
- [ ] **Step 108** [W] ~1m: `scripts/ci-local.sh` and `scripts/kingdom-shutdown.sh`.
```bash
cd ~/tmp/wt/s-clean-cut && sed -i '/^    cmd\/trace-collector$/d; /^    cmd\/waf$/d; s#^    for ws in crates/monad-mbc crates/zhend cmd/trace-collector; do$#    for ws in crates/monad-mbc crates/zhend; do#' scripts/ci-local.sh
sed -i '/^    ebpf-exporter$/d; /^    waf$/d' scripts/kingdom-shutdown.sh
sed -i 's#cd ${PROJECT_ROOT}/cmd/trace-collector \&\& cargo run --release -- run#${PROJECT_ROOT}/bin/trace-collector-go --unified#' scripts/load-ebpf.sh
python3 - <<'PY'
import re
p='scripts/build.sh'; s=open(p).read()
s2,n=re.subn(r'^    # Build trace-collector if it exists\n    if \[ -d "\$\{PROJECT_ROOT\}/cmd/trace-collector" \]; then\n.*?^    fi\n','',s,count=1,flags=re.S|re.M)
open(p,'w').write(s2); print('build blocks removed',n)
PY
for f in ci-local kingdom-shutdown load-ebpf build; do bash -n scripts/$f.sh || echo BAD $f; done; git grep -c -E 'cmd/trace-collector([^-]|$)|ebpf-exporter' -- scripts/ci-local.sh scripts/kingdom-shutdown.sh scripts/load-ebpf.sh scripts/build.sh; echo rc=$?
```
  Expect `build blocks removed 1`, no `BAD` line, `rc=1`. Read `scripts/build.sh` around the cut: the block must end where it began, inside its function.
- [ ] **Step 109** [W] ~8m: `scripts/bootstrap-llm-lab.sh` (a host bootstrap with 26 vLLM lines). Judgement: delete the function `phase_vllm` and its call, which removes the `rocm/vllm` pull, the `vllm_dir` block and the `unheaded-vllm-start` helper it writes to `/usr/local/bin`; the six `vllm-*` aliases; the `docker compose` hint at the "Build vLLM container" line. The llama-server and hf_transfer paths stay; read the function first in case either sits inside it. `bash -n`, then `git grep -c -i 'vllm' -- scripts/bootstrap-llm-lab.sh`; record what remains and why.
- [ ] **Step 110** [W] ~8m: `scripts/vault-to-runway.py` would re-pull the 30 GB image: `VLLM_IMAGE` defaults to `rocm/vllm:latest`. Delete `stop_vllm`, `launch_vllm`, the vLLM half of `display_status`, the `VLLM_*` constants, `--launch` and `--stop`, and their help text. Model transfer, purge and listing stay. `python3 -m py_compile scripts/vault-to-runway.py`, then `grep -c -i vllm scripts/vault-to-runway.py` is `0` apart from a `GPU_ENV` entry you decide on and record.
- [ ] **Step 111** [W] ~8m: `nix/containers/sophia-eye.nix` (firewall ports, rule 13): delete `systemd.services.vllm-deepseek` and `vllm-qwen`, the `vllm-*.service` names in the gateway's `after` and `wants`, and the ports `20100`, `20101`, `8000`, `8001`. The gateway's `VLLM_ENDPOINT` follows Phase 20's choice: llama-server. `pkg/ports` `VLLMDeepSeek` and `VLLMQwen` stay as reserved numbers, the same reasoning as `pkg/uids`: no edit. `git grep -c 'systemd.services.vllm' -- nix; echo rc=$?` Expect `rc=1`.
- [ ] **Step 112** [C] ~1m: **PHASE 22A EXIT GATE + COMMIT.** `~/tmp/wt/bin/plan-commit s-clean-cut S-CLEAN-CUT "Steps 105-112: launchers and root scripts drop what left" lxd/hosts/host-a/launch-all.sh lxd/Makefile lxd/LAUNCH_EXAMPLES.sh lxd/hosts/host-a/static-ips.yaml scripts/ci-local.sh scripts/kingdom-shutdown.sh scripts/load-ebpf.sh scripts/build.sh scripts/bootstrap-llm-lab.sh scripts/vault-to-runway.py nix/containers/sophia-eye.nix`

## PHASE 23: baselines, SBOM, CBOM (all answers)
**Goal**: every shrink-only ratchet records the shrink; generated files match the tree.
**Prerequisite**: Phases 22 and 22A. **Time**: 30m. **Agent**: one. **Model**: Sonnet

- [ ] **Step 113** [B] ~3m: `cd ~/tmp/wt/s-clean-cut && go mod tidy && git diff --stat -- go.mod go.sum | tail -1; ./scripts/check-netip.sh | tail -1` Expect no `go.mod` change (the trial dropped no module) and `OK: <n> uses (baseline 120)` with `<n>` near 74. The file holds one number, so patch it: `sed -i "s/^120$/<n>/" docs/policy/netip-baseline.txt`, and put old and new under "For Stevie" (rule 17). A `go.mod` change: keep it and name it in the progress file.
- [ ] **Step 114** [B] ~8m: `cd ~/tmp/wt/s-clean-cut && ./scripts/generate-sbom.sh 2>&1 | tail -2 && ./scripts/check-sbom-trend.sh` Expect three `OK` lines, `rust_components` lower than the baseline. Rule 17: patch only that row, `sed -i 's/^rust_components=.*/rust_components=<n>/' docs/policy/sbom-count-baseline.txt` with the printed count, and record old and new under "For Stevie". Do not touch the Go or Python lines.
- [ ] **Step 115** [B] ~2m: `cd ~/tmp/wt/s-clean-cut && ./scripts/check-dependency-baseline.sh --update && git diff --stat docs/policy/dependency-baseline.txt | tail -1 && ./scripts/check-dependency-baseline.sh | tail -1` Expect about 37 deletions, 0 insertions, then `[PASS]`. Rule 17 allows this `--update`: the rows it removes are the crates this plan deleted, and it touches no other row. Without it `security.yml`'s gate goes red, and S-WIRE-HONEST, S-ZHEN-POLISH and S-G9-FOLD would hit that after the merge.
- [ ] **Step 116** [W] ~1m: `cd ~/tmp/wt/s-clean-cut && ./scripts/live-path-inventory.sh --report > docs/LIVE-PATHS.md && git diff --stat docs/LIVE-PATHS.md | tail -1 && ./scripts/live-path-inventory.sh --check | tail -1` Expect the count table to move (21/28/24/88 to 20/27/23/85) and a PASS: Phase 22 removed rows, and `ci.yml`'s gate reads the counts too.
- [ ] **Step 117** [B] ~3m: `cd ~/tmp/wt/s-clean-cut && go run ./cmd/crypto-inventory scan --out compliance/crypto/cbom.cdx.json && ./scripts/check-crypto-inventory.sh 2>&1 | tail -2 && python3 -c "import json;d=json.load(open('compliance/crypto/cbom.cdx.json'));print(sorted({p['value'] for c in d['components'] if 'zhend/src/api/quic' in c['bom-ref'] for p in c['properties'] if p['name']=='unheaded:link-state'}))"` Expect PASS lines and `['not-spawned']`.
- [ ] **Step 118** [B] ~4m: `cd ~/tmp/wt/s-clean-cut && sed -i '/\tdocker\/vllm-rocm\/Dockerfile\t/d' docs/policy/hadolint-baseline.tsv; sed -i '/^deploy\/sophia-eye\/docker-compose\.yml vllm\/vllm-openai:latest-rocm$/d; /^docker\/firewall\/ipfire\/Dockerfile ubuntu:24\.04$/d; /^docker\/firewall\/opnsense\/Dockerfile ubuntu:24\.04$/d; /^kubernetes\/manifests\/base\/haproxy-ingress\/controller\.yaml haproxytech\/kubernetes-ingress:3\.1$/d' docs/policy/image-tag-baseline.txt; ./scripts/check-image-tags.sh; ./scripts/check-hadolint.sh; grep -c -E 'docker/hosts|docker/vllm-rocm' docs/policy/trivy-misconfig-baseline.tsv; echo rc=$?` Rule 17: rows patched by hand, no `--update`. Besides the vLLM row this removes the three stale rows left by `docker/firewall` and haproxy-ingress, which S-SUPPLY-FIRST Step 67 relies on this plan to clean. Expect both gates to pass, `0`, `rc=1`. `check-hadolint.sh` needs the docker daemon: a permission error is `[STUCK]` (never `sudo`), and Phase 24 runs it. The trivy baseline has no rows for the deleted paths; its next CI run stays green. If rows exist: delete them with `sed -i '/docker\/hosts\|docker\/vllm-rocm/d' docs/policy/trivy-misconfig-baseline.tsv`.
- [ ] **Step 119** [B] ~2m: `cd ~/tmp/wt/s-clean-cut && ./scripts/gen-man.sh --check 2>&1 | tail -1; ./scripts/check-unit-ceilings.sh 2>&1 | tail -1; ./scripts/check-log-sources.sh 2>&1 | tail -1` Expect pass lines. A usage error from `gen-man.sh --check`: run `sed -n 1,20p scripts/gen-man.sh`, use its check form, record it.
- [ ] **Step 120** [C] ~1m: **PHASE 23 EXIT GATE + COMMIT.** Named files from `git status --short`: `~/tmp/wt/bin/plan-commit s-clean-cut S-CLEAN-CUT "Steps 113-120: baselines, SBOM and CBOM record the shrink" docs/policy/netip-baseline.txt docs/policy/sbom-count-baseline.txt docs/policy/dependency-baseline.txt docs/LIVE-PATHS.md compliance/crypto/cbom.cdx.json docs/policy/hadolint-baseline.tsv docs/policy/image-tag-baseline.txt docs/battle-plans/progress/S-CLEAN-CUT-progress.md` plus each file under `sbom/` that `git status --short sbom` lists, by name.

## PHASE 24: SPRINT EXIT
**Goal**: all gates green on the branch, merged, handed off.
**Prerequisite**: Phase 23. **Time**: 45m. **Agent**: coordinator. **Model**: Opus

- [ ] **Step 121** [V] ~20m: The closing gate. `gofmt` covers only what this branch changed: 13 files under `tests/` are unformatted on develop.
```bash
cd ~/tmp/wt/s-clean-cut && export GOMAXPROCS=4 GOTMPDIR=/var/tmp
git diff --name-only --diff-filter=ACMR develop...HEAD -- '*.go' | xargs -r gofmt -l
go vet ./...; echo vet=$?
go test -race ./... > /var/tmp/s-clean-cut-test.log 2>&1; echo test=$?
grep -E '^(FAIL|--- FAIL)' /var/tmp/s-clean-cut-test.log
./scripts/check-gosec-ratchet.sh | tail -1
./scripts/check-dependency-baseline.sh | tail -1
./scripts/live-path-inventory.sh --check | tail -1
```
  Expect no gofmt line, `vet=0`, and a failure list no longer than develop's (`pkg/agent` fails there too).
- [ ] **Step 122** [V][S] ~10m: Root for the load gate. `cd ~/tmp/wt/s-clean-cut && CARGO_TARGET_DIR=/var/tmp/s-clean-cut-ebpf make test-rust-ebpf-common test-rust-svc-token 2>&1 | tail -3; env -u CARGO_TARGET_DIR EBPF_LOAD_TARGET_DIR=/var/tmp/s-clean-cut-ebpf/load-gate EBPF_INSNS_OUT=/var/tmp/s-clean-cut-insns.tsv ./scripts/check-ebpf-loads.sh | tail -1; ./scripts/check-ebpf-insns.sh --from /var/tmp/s-clean-cut-insns.tsv | tail -1` Expect pass lines.
- [ ] **Step 123** [V] ~5m: Clean tree, then prove the moved gates still bite: `cd ~/tmp/wt/s-clean-cut && git status --short | wc -l && ./scripts/check-gates-can-fail.sh --quick 2>&1 | tail -2` Expect `0` and exit 0.
- [ ] **Step 124** [V] ~2m: Nothing named survives: `cd ~/tmp/wt/s-clean-cut && git grep -l -E 'pkg/waf|cmd/waf|internal/bpfmap|ebpf/maploader|cmd/ebpf-exporter|pkg/afxdp|ebpf/af-xdp|xdp-redirect|cmd/trace-collector([^-]|$)|docker/hosts|lxd/containers|lxd/vllm-rocm|docker/vllm-rocm' -- . ':!docs' ':!references' ':!wiki' ':!CHANGELOG.md' ':!skills' ':!tomb' ':!.github/workflows/security.yml' ':!docker/README.md' ':!sbom-results' ':!sbom/SBOM.md' ':!lxd/README.md' ':!lxd/NEW_FILES_CREATED.md' ':!runbooks/security/vuln-management-2026-05-06.md' ':!scripts/check-gosec-ratchet.sh' ':!scripts/ci-local.sh' ':!kubernetes'` Expect empty output (`ci-local.sh` keeps a dated comment naming `cmd/waf`). The exclusions are dated history; `kubernetes/` leaves with S-MOVE-OUT.
- [ ] **Step 125** [V] ~3m: Rule 16: the findings table, in the worktree, with the evidence path. `cd ~/tmp/wt/s-clean-cut && go run ./cmd/compliance-evidence findings -evidence /home/govan/tmp/unheaded/var/compliance/evidence.json | head -1 | cut -c1-90` Expect the header with an open count not below the count on `develop` minus what this plan closed (none: FND-009 stays open for the routing containers; it stood at 154 at forge time). A collapse means the path is wrong: `[STUCK]`.
- [ ] **Step 126** [B] ~25m: Rule 15: merge develop into the sprint, rebuild, run the whole closing gate on that tree. `cd ~/tmp/wt/s-clean-cut && git merge --no-ff -S --no-edit develop`; on a gpg failure `git merge --abort; git merge --no-ff --no-gpg-sign --no-edit develop`, recorded. Conflicts: rows in `docs/adr/ADR-INDEX.md` and `wiki/ADR-Index.md` keep both, in number order (S-ZHEN-POLISH's 122 lands beside 121). `docs/LIVE-PATHS.md` and `cbom.cdx.json` are regenerated after the merge, never hand-merged. Any other conflict is `[STUCK]`: `git merge --abort`, no merge, "For Stevie". Then `go build ./...` and Step 121 again, plus `./scripts/check-crypto-inventory.sh | tail -1`.
- [ ] **Step 127** [B] ~2m: Merge, ahead of S-WIRE-HONEST (shared `bpfschema.go`). The compliance timer runs `go run` from `~/tmp/unheaded`, so the build must be green the moment this lands.
```bash
cd ~/tmp/unheaded && git switch develop && { git merge --no-ff -S sprint/s-clean-cut -m '[PLAN S-CLEAN-CUT] merge' || { git merge --abort; git merge --no-ff --no-gpg-sign sprint/s-clean-cut -m '[PLAN S-CLEAN-CUT] merge'; }; } && git log --format=%G? -1 && go build ./... && echo MERGED
```
  Expect `G` (or `N`: record the unsigned merge in the progress file) and `MERGED`. Stevie pushes.
- [ ] **Step 128** [W] ~5m: Fold the progress file's "For Stevie" and the `~/tmp/wt/.s-clean-cut.unsigned` list into the coordinator's `~/tmp/next.md` section (rule 9). `git worktree remove ~/tmp/wt/s-clean-cut` only after S-MOVE-OUT has read this plan's notes.

---

## Appendix A: Agent assignment

| Phase | Model | Parallel | Depends on | Time |
|---|---|---|---|---|
| 0 Preflight | Haiku | no | none | 15m |
| 1 pkg/waf | Haiku | yes | 0 | 20m |
| 2 lxd/containers | Haiku | yes | 0 | 10m |
| 3 docker lane | Haiku | yes | 0 | 30m |
| 4 bpfmap, maploader | Haiku | yes | 0 | 5m |
| 5 exporter | Haiku | yes | 0 | 15m |
| 5A Nix hosts | Opus | yes | 0 | 10m |
| 6 AF_XDP pure | Haiku | yes | 0 | 10m |
| 7 AF_XDP branches | Opus | no (eBPF lane) | 6 | 60m |
| 8 AF_XDP docs | Haiku | yes | 0 | 15m |
| 9 Rust collector | Haiku | yes | 0 | 20m |
| 10 restart route | Haiku | yes | 0 | 15m |
| 11 tty-bridge | Sonnet | yes | 0 | 15m |
| 12 bpfschema | Haiku | yes | 0 | 10m |
| 13 phase12 | Sonnet | no (eBPF lane) | 7 | 25m |
| 15 mTLS | Haiku | yes | 0 | 15m |
| 18 zhend CBOM | Sonnet | yes | 0 | 20m |
| 19 vLLM leftovers | Haiku | after 2 | 2 | 15m |
| 20 sophia-eye | Opus | yes | 3 | 40m |
| 21 ADR, sweep rows | Haiku | after 1, 4, 7 | 1, 4, 7 | 15m |
| 22 ledgers | Haiku | no | 1-21 | 30m |
| 22A root scripts | Opus | yes | 2, 9, 19 | 10m |
| 23 baselines | Sonnet | no | 22 | 30m |
| 24 exit | Opus | no | 23 | 45m |

Critical path: 0, 6, 7, 13, 22, 23, 24: about 4 hours.

## Appendix B: Emergency procedures

- **A lane broke the build for everyone**: `go build ./... 2>&1 | head` names the package; each lane owns disjoint files, so `git log -1 -- <file>` names the phase. `git revert <hash>` that one commit, mark the phase `[STUCK]`.
- **Index contention**: if `plan-commit` hangs, another agent holds `~/tmp/wt/.s-clean-cut.lock`. Wait. Never delete the lock; `flock` releases it when the holder exits.
- **A stray file was staged by hand**: `git restore --staged <file>`, then run `plan-commit` again.
- **Verifier rejects shield or packet-marker**: restore the three files with `git checkout develop -- ebpf/shield-ebpf/src/main.rs ebpf/packet-marker/src/main.rs ebpf/monad-common/src/lib.rs`; DP-205's userspace deletion stands, the branches stay dormant, a parked note goes to "For Stevie".
- **Stevie wants a deletion back**: everything is one commit per answer: `git revert <hash>`.
- **/tmp fills**: of this plan's commands only `generate-sbom.sh` writes there; check `GOTMPDIR` and `CARGO_TARGET_DIR` point at `/var/tmp`. The coordinator, not a lane, clears `/var/tmp/s-clean-cut-*` after the merge (rule 13).
- **A stuck process**: stop it by its recorded pid, never `pkill -f` (rule 14).

## Appendix C: For Stevie (known at forge time)

1. **`cmd/waf`** goes with `pkg/waf` (round 4). Nothing depended on it. `crates/zhend` keeps `aws-lc-rs`, so the ADR-004 admission narrows but stays.
2. **`SHIELD_CONFIG`** goes with AF_XDP; ADR-119 gets a note that it must be declared anew.
3. **Baselines** are patched by hand (rule 17); old and new numbers land in the progress file. One `--update` runs, on `dependency-baseline.txt`: it removes only the crates this plan deleted.

---

*S-CLEAN-CUT Battle Plan: forged 2026-11-04*
*24 phases, 128 steps. Grounded at `d6ad43e2`; the exact-text blocks were dry-run in a scratch clone as the normal user, and the plan took an independent review's findings.*
*Cut clean, leave the record.*
