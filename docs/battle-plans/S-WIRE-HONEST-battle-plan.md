# S-WIRE-HONEST BATTLE PLAN — 19 Phases, 137 Steps

**Date**: 2026-11-04
**Sprint**: S-WIRE-HONEST: what the wire and the crypto registry claim becomes what the tree does. DP-715, DP-716, DP-408, DP-211, DP-411, DP-412.
**Series**: `docs/battle-plans/SPRINT-SERIES-2026-11-04.md`, lane D. Its rules 1-17 bind this plan.
**Prerequisite**: `develop` builds; passwordless sudo on west; west kernel `6.17.0-41-generic` (the insns baseline's kernel); `/home/govan/tmp/projects/doomgeneric/unheaded` exists (go.mod replace).
**Target**: FND-025's evidence built with its checks where a Monad exists (closure is Stevie's word) and verifier cost measured; the TRUE-WIRE defaults applied; LATENCY_MAP keyed by what it holds; `protocol-api --mode=bpf` reading real maps with `pkg/ebpf`; FN-DSA out, SLH-DSA complete; the PQC bar and its gaps in ADR-105; the HbH registry parked; nine questions queued for the RFC desk.
**Duration**: about 13 agent-hours. Wall time 9-10 hours with three build slots, about 14 with one (rule 3 is shared across the series). Critical path Phases 0, 1, 2, 11, 12, 13, 14, 15, 16, 17.
**Branch**: `sprint/s-wire-honest`, worktree `/home/govan/tmp/wt/S-WIRE-HONEST`. Lanes: `sprint/s-wire-honest-<lane>`, worktree `/home/govan/tmp/wt/S-WIRE-HONEST-<lane>`, merged back at each lane's exit gate.
**Agent Strategy**: see "Swarm table". Phases 0-2 serial, then six lanes under rule 3's cap. Haiku for scripted edits, Sonnet for code the plan gives in full, Opus for eBPF, wire, judgement and every [S] step (rules 13, 15).
**Commit Cadence**: every 4-5 steps of change at each [C] (a run of verify-only steps has nothing to commit), through `commit.sh`: signed, named files only, format `[PLAN S-WIRE-HONEST] Steps X-Y: ...` with the series trailers. Only commit after the last [V] passed. Never `git push`, never `git add -A`.
**Stuck Protocol**: skip after 3x the estimate or 2 failed debug attempts: mark `[STUCK]` with the output, commit, move on. Questions go to the progress file under "For Stevie". Haiku and Sonnet never improvise: any output the plan did not predict is `[STUCK]`.
**Log**: `docs/battle-plans/progress/S-WIRE-HONEST-progress.md` on the sprint branch; each lane keeps `S-WIRE-HONEST-<lane>-progress.md`, so lanes never conflict on one file.
**Scope fence**: no change to any Monad byte, no new module or crate, no live service touched, no attach to a real interface, no `compose up`, no reboot. Found bugs outside the six DPs go to "For Stevie", not into the diff.

Every command runs from the phase's worktree root. Shell state does not persist between tool calls, so start each one with `cd <worktree> && P=/var/tmp/S-WIRE-HONEST-payloads && `. `$P` is this plan's private scratch directory: every temporary file goes there. Build output stays under `$P` or the worktree (`/tmp` is a 7.2 GiB tmpfs). Use `GOMAXPROCS=4`, `CARGO_BUILD_JOBS=4`.

## WHAT THE TREE SAID (measured 2026-11-04 on develop `ba87679b`)

| Plan input | Found | Effect on this plan |
|---|---|---|
| "36 TRUE-WIRE defaults" | The table has **35** rows (R10, C9, E6, L7, T3). All paths still present | Phase 3 applies 10 deletions; Appendix C places all 35 |
| DP-715 "accept all defaults" vs DP-203 "nothing deleted" | C1-C6 default DELETE; DP-203 and Stevie's brief fold them | DP-203 wins; **C1-C8 are not deleted** |
| L6 `cmd/shield` | Row is ASK: there is no default, and the pack has no answer | Left alone; For Stevie |
| L5 `cmd/pqc-verifier` | ADR-119 D1 wants it revived; the default deletes it; only docs and `kingdom-shutdown.sh` reference it | Deleted, note added to ADR-119; For Stevie |
| "implement SLH-DSA" | **Already implemented** on circl: `pkg/crypto/pqc/slh_dsa.go`, sign and verify, tested. C-10 and `pqc.go`'s header are stale. circl is **v1.6.5** (comments say 1.6.3); `sign/slhdsa` present, 12 sets; only 6 wrapped; no production caller | Phase 5 finishes it: SHAKE sets, a dispatch |
| Parameter ids | Three tables disagree: foundation-06's registry, foundation-06's identifiers, PQC-00. Code matches none fully. No value is stored | Existing ids stay; SHAKE takes PQC-00's free 0x07-0x0C; RFC item 8 asks which table owns them |
| Registry drop of FN-DSA | 0x03 is a registered code point; `services/sophia` and `services/shield` tests use 0x03 as "same family" | 0x03 stays burned and stays lattice. Draft text goes to the RFC desk |
| HQC reserved | circl v1.6.5 `kem/` has no hqc | Stays reserved |
| "move the HMAC check" | v0x01 has no tag, foundation-06 leaves the field undefined, eBPF has no HMAC helper. Hop can check that a key exists and is live, **not** a tag | Key-presence check; ADR-125 says so; residual gap becomes FND-031 |
| Retry tokens | Nothing on the wire. `RETRY_TOKENS` value comment adds to 56, the map is 48. `migration.ValidateToken` always returns true for 6+ bytes | Validated-source table; layout fixed; `migration` not wired |
| Keying | Shield draws the flow label at random **per packet**, so a label-keyed map can never hit twice. Monad has no flow id | Keys on the service pair and deploy_ring (ADR-125) |
| `AUTHORITY` | FND-025 names it, DP-716 does not. Keyed on `qos_class`, result discarded | Deleted, as FND-025's verify text allows |
| Verifier budget | `hop_xdp` **153,835**; ratchet line 169,218 (+10 %); `shield_xdp` 1,502 | Phase 11 measures; growth recorded with a reason |
| Who loads hop-ebpf | Nothing: not in `cmd/ebpf-loader`'s `ALL_PROGRAMS`, no match arm | Phase 14 adds an arm, default list unchanged |
| `protocol-api` bpf mode | `backend.go` uses cilium/ebpf; `pkg/ebpf` had no way to open a pinned map | Phase 7 adds `PinnedMap` |
| `pkg/ebpf` bug | `bpfMapLookupElem` and `bpfMapDeleteElem` wrap with `%v`, so `errors.Is(err, ENOENT)` in `ReadMap` is never true: an absent key is an error | Fixed in Phase 7 (found by the root test while forging) |
| bpf mode scope | Sophia and FLOWS have real maps. No map holds Wotan memory slots. ANAMNESIS is a ring with one consumer position: a second reader steals from trace-collector | Those stay Unimplemented, with the true reason |
| LATENCY_MAP | Live reader in `cmd/trace-collector-go`; the key held a zero trace id and 16-bit pid and tid; east showed `[::]:18570` flows | New key; old objects fail loudly in the decoder; both must ship together |
| 0x3E "zhend's" | It is **also `MONAD_OPT_TYPE`**. Only length separates them. RFC 4727 experimental, not assignable | ADR-126 says so, reserves 0x1E; RFC item 5 |
| Worktrees | go.mod replace `../projects/doomgeneric/unheaded` does not resolve from `~/tmp/wt/<plan>`: `go build ./...` fails | Step 2 makes `~/tmp/wt/projects`. **Every plan in the series needs this** |
| Rehearsal | Phases 3, 4, 6, 7, 8, 10 and the ripple scripts ran as the normal user on a scratch snapshot of develop | Their red and green output is measured. Phase 8R's root tests are not part of that re-run: the forge ran them once under sudo before rule 14 existed, and no later version of this plan has run them. Phases 5, 9 and 11-15 are unrehearsed |

## FOR STEVIE (nothing here stops the plan)

1. Row L6 (`cmd/shield` + `services/shield`) has no answer in the pack.
2. L5: deleting `cmd/pqc-verifier` while ADR-119 D1 revives it. Done per DP-715; say if you would rather keep it.
3. ADR-125 "What this is not": hop checks prove nothing about a packet's origin. FND-025 stays open until Stevie rules on its verify text (item 8); FND-031 opens for the gap. Accept or redirect.
4. Bug, not fixed here: `apply_flow_action` returns `false` for `fa::DROP`, so a DROP flow is never dropped (`ebpf/hop-ebpf/src/main.rs`; foundation-06 says "Discard packet"). One word, but outside DP-716.
5. Bug, not fixed here: hop-ebpf's `SEQ_COUNTERS` and `DOS_STATE` key on `flow_label >> 8`, which is random on Shield-born packets (RFC item 9).
6. C1-C6: DP-715's "accept all 36 defaults" (round 3) is later than DP-203, and the defaults delete six planes. DP-203 won because you said not to remove them. Say if round 3 meant otherwise.
7. Parameter ids: three tables disagree (RFC item 8). Nothing changes until you say which owns them.
8. FND-025 stays OPEN: this plan does not rewrite its verify text. ADR-098 says a finding closes when its evidence passes, never when its entry is edited, so the proposed new text waits for Stevie's word; the old text named a lab test against shield-ebpf and a key written by `cmd/shield`, which DP-716 moved away from. Old text, whole: "a lab-netns test with shield-ebpf attached counts checks_run and checks_failed for a flow with and without an HMAC key written by cmd/shield, and the GOAWAY and authority lookups either enforce or are gone from the object". Phase 15 records it here again with the new text.
9. `hop_xdp` growth: Phase 15 patches that one baseline row (rule 17). The numbers land in the hop lane's "For Stevie"; nobody self-approves them.
10. ADR-105 roadmap dates are proposals. Microsoft moved to 2029 for critical products on 2026-06-30; HKMA's index targets 2030. Both postdate the wording of your bar.
11. The nine RFC items wait for the resubmission sitting (DP-606).

## LEGEND

[B] bash · [V] verify, must pass · [D] debug, only on failure · [W] write or edit a file · [R] read · [S] sudo · [P] parallel with other [P] steps of the phase · [C] commit checkpoint · [STUCK] skipped under the Skip Protocol · [BLOCKED] waits on an upstream STUCK · [DECIDE] decided here, proceed; override only on the stated condition

---

## PHASE 0: PREFLIGHT (Steps 1-11)

**Goal**: A worktree that builds, payloads extracted, baselines recorded.
**Prerequisite**: This plan is on disk at `/home/govan/tmp/unheaded/docs/battle-plans/S-WIRE-HONEST-battle-plan.md`.
**Time**: 60m (cold eBPF build, full `-race` baseline) · **Agent**: Solo · **Model**: Opus (Step 8 is root)

- [ ] **Step 1** [B] ~1m: Create the worktree.
  ```bash
  mkdir -p /home/govan/tmp/wt && git -C /home/govan/tmp/unheaded worktree add -b sprint/s-wire-honest /home/govan/tmp/wt/S-WIRE-HONEST develop
  ```
  Expected last line: `HEAD is now at`. If the branch or path exists: [STUCK].
- [ ] **Step 2** [B] ~1m: Make go.mod's relative replace resolve from worktrees.
  ```bash
  ln -sfn /home/govan/tmp/projects /home/govan/tmp/wt/projects; ls /home/govan/tmp/wt/projects/doomgeneric/unheaded >/dev/null && echo ok
  ```
  Expected: `ok`. Any other output: [STUCK].
- [ ] **Step 3** [B] ~1m: Extract the payloads in this plan.
  ```bash
  python3 - <<'PY'
  import re, os
  plan = open('/home/govan/tmp/unheaded/docs/battle-plans/S-WIRE-HONEST-battle-plan.md').read()
  d = '/var/tmp/S-WIRE-HONEST-payloads'; os.makedirs(d, exist_ok=True)
  for name, body in re.findall(r'<!-- PAYLOAD ([\w.\-]+) -->\n````text\n(.*?)\n````\n', plan, re.S):
      open(f'{d}/{name}', 'w').write(body + '\n')
  print(len(os.listdir(d)))
  PY
  chmod +x /var/tmp/S-WIRE-HONEST-payloads/*.sh
  ```
  Expected: `21`. Any other count: [STUCK].
- [ ] **Step 4** [W] ~1m: Start the progress file.
  ```bash
  cat > docs/battle-plans/progress/S-WIRE-HONEST-progress.md <<'MD'
  <!-- SPDX-License-Identifier: GPL-3.0-or-later -->
  # S-WIRE-HONEST progress

  ## Log

  ## For Stevie
  MD
  ```
- [ ] **Step 5** [C] ~1m: **COMMIT** `/var/tmp/S-WIRE-HONEST-payloads/commit.sh S-WIRE-HONEST "Steps 1-5: worktree, payloads, progress file" docs/battle-plans/progress/S-WIRE-HONEST-progress.md`
- [ ] **Step 6** [B] ~15m: Go baseline, with `-race` so it compares like for like at Step 130. Package names only: timings differ every run.
  ```bash
  GOMAXPROCS=4 go build ./... && GOMAXPROCS=4 go vet ./pkg/... ./cmd/... ./services/... 2>&1 | tail -3; GOMAXPROCS=4 go test -race ./... 2>&1 | grep -E '^FAIL[[:space:]]+unheaded/' | awk '{print $2}' | sort -u > $P/go-baseline.txt; wc -l < $P/go-baseline.txt
  ```
- [ ] **Step 7** [V] ~1m: Build and vet printed nothing. Copy `$P/go-baseline.txt` into the progress file under "Known failures", and record `systemctl list-units 'unheaded*' --no-pager --state=running --no-legend | wc -l` there as "running units". Later gates fail only on lines not in that list.
  - [D] `go build` fails on `cmd/wotan-ctl`: re-run Step 2. Second failure: [STUCK], stop the plan.
- [ ] **Step 8** [B][S] ~25m: eBPF baseline (cold build into the worktree's `ebpf/target/load-gate`; the script uses sudo itself).
  ```bash
  CARGO_BUILD_JOBS=4 EBPF_INSNS_OUT=$P/insns-pre.tsv scripts/check-ebpf-loads.sh 2>&1 | tail -2
  ```
  Expected last line contains `[PASS]`.
- [ ] **Step 9** [V] ~1m: Baseline numbers match the policy file.
  ```bash
  grep -E '^(hop-ebpf/hop_xdp|shield-ebpf/shield_xdp|shield-ebpf/shield_tc)\s' $P/insns-pre.tsv; uname -r
  ```
  Expected: hop_xdp 153835, shield_xdp 1502, shield_tc 976 (columns may include the feature name), kernel `6.17.0-41-generic`. A different kernel: [STUCK]; the ratchet exits 2 on it.
- [ ] **Step 10** [V] ~1m: ADR numbers and the finding id are free.
  ```bash
  ls docs/adr | grep -E '^ADR-(125|126|127|128|129|130|131|132|133|134)-' ; grep -c 'id: FND-031' compliance/findings/register.yaml
  ```
  Expected: no file names, then `0`. A taken number: [STUCK], recorded under "For Stevie". ADR-121 and FND-032 are other plans', 125-134 and FND-031 are ours.
- [ ] **Step 11** [C] ~1m: **COMMIT** `/var/tmp/S-WIRE-HONEST-payloads/commit.sh S-WIRE-HONEST "Steps 6-11: preflight, baselines recorded" docs/battle-plans/progress/S-WIRE-HONEST-progress.md`

**PHASE 0 EXIT GATE**: Steps above passed; `git log -1 --format=%s` shows the commit. If not, do not start Phase 1.

## PHASE 1: RFC DESK AND SPEC COMPLIANCE GATE (Steps 12-15)

**Goal**: Every wire-adjacent item in this sprint has passed the desk before any code. The four DP-715 questions and five found items are queued.
**Prerequisite**: Phase 0. **Time**: 30m · **Agent**: Solo · **Model**: Opus

- [ ] **Step 12** [W] ~5m: Land the queue: `cp $P/RFC-EDITOR-REQUESTS-WH.md docs/protocol/RFC-EDITOR-REQUESTS-WH.md`. Load the `unheaded-rfceditor` skill and review it against the drafts; each "what the tree says" cell must hold when you re-read the cited line. Correct a cell only where the tree differs and log it.
- [ ] **Step 13** [V] ~15m: **SPEC COMPLIANCE GATE.** Record this table in the progress file, one line per change, RFC desk review = the skill's reading above:

  | Change | Wire version | Change type | Touches a Monad byte? | Backward compatible | IANA impact |
  |---|---|---|---|---|---|
  | DP-716 hop checks | v0x01 frozen | none on the wire; reads existing fields, new private map semantics | No (read only; no field is written that was not before) | Yes: mode 0 default | None |
  | DP-408 latency key | v0x01 | none: a kprobe map, never on the wire | No | Reader and object ship together | None |
  | DP-211 bpf mode | v0x01 | none: map reads | No | Yes | None |
  | DP-411 FN-DSA drop | v0x01 | code only; 0x03 burned | No | Yes: nothing ever produced 0x03 | Registry text in both drafts (foundation-06 "PQC Algorithm Identifiers", Specification Required, and PQC-00) queued, item 6. No draft edited here |
  | DP-411 SHAKE sets | v0x01 | new code at PQC-00's own 0x07-0x0C | No | Yes: additive | None; which table owns the ids is item 8 |
  | DP-412 registry | v0x01 | ADR only | No | Yes | Queued, item 5 |

  Every row must read "No" in the Monad column. If review finds a Yes: that item is [STUCK], its lane does not start, the rest continue.
- [ ] **Step 14** [V] ~2m: No wire source is on the change list: `git diff --stat $(git merge-base develop HEAD) -- ebpf/monad-common/src/lib.rs docs/protocol/draft-bellis-unheaded-protocol-foundation-06.md docs/protocol/draft-bellis-unheaded-pqc-authentication-00.md | wc -l`. Expected `0`. This gate is re-run at Step 133.
- [ ] **Step 15** [C] ~1m: **COMMIT** `$P/commit.sh S-WIRE-HONEST "Steps 12-15: RFC desk queue and spec compliance gate" docs/protocol/RFC-EDITOR-REQUESTS-WH.md docs/battle-plans/progress/S-WIRE-HONEST-progress.md`

**PHASE 1 EXIT GATE**: gate table in the progress file, all "No". If not, the affected lane stays closed.

## PHASE 2: ADRS BEFORE CODE (Steps 16-19)

**Goal**: ADR-125 and ADR-126 exist. Text is final in the payloads; this phase only lands it.
**Prerequisite**: Phase 1. **Time**: 10m · **Agent**: Solo · **Model**: Haiku

- [ ] **Step 16** [W] ~1m: `cp $P/ADR-125-hop-checks.md docs/adr/ && cp $P/ADR-126-private-hbh-option-registry.md docs/adr/`
- [ ] **Step 17** [W] ~1m: `python3 $P/index_rows_wire.py`. Expected output starts `index rows added after:`. `ANCHOR MISSING`: [STUCK].
- [ ] **Step 18** [V] ~1m: `grep -c 'ADR-12[56]-' docs/adr/ADR-INDEX.md && grep -l '^\*\*Status:\*\* Proposed' docs/adr/ADR-125-hop-checks.md docs/adr/ADR-126-private-hbh-option-registry.md | wc -l`. Expected `2` then `2`.
- [ ] **Step 19** [C] ~1m: **COMMIT** `$P/commit.sh S-WIRE-HONEST "Steps 16-19: ADR-125 hop checks, ADR-126 HbH registry (parked)" docs/adr/ADR-125-hop-checks.md docs/adr/ADR-126-private-hbh-option-registry.md docs/adr/ADR-INDEX.md`

**PHASE 2 EXIT GATE**: both ADRs on `sprint/s-wire-honest`. Lanes start from this commit: `$P/lane.sh <lane>` for tw, pq, bar, api, lat, hop, in Appendix A's order. It prints the lane's worktree and writes its progress file. A finished lane merges with `$P/lane_merge.sh <lane>`; `CONFLICT` is Opus's to resolve.

## PHASE 3: TRUE-WIRE DEFAULTS (Steps 20-29)

**Goal**: The ten deletions DP-715 leaves to this plan, with their doc ripple; all 35 rows accounted for.
**Prerequisite**: Phase 2. Lane `tw`. **Time**: 30m · **Agent**: Agent [P] · **Model**: Haiku

- [ ] **Step 20** [V] ~2m: Nothing new imports what goes.
  ```bash
  for p in amplification dos intermediary prefetch lifecycle settings flowtype sequence registry; do git grep -l "\"unheaded/pkg/protocol/$p\"" -- '*.go' | grep -v "^pkg/protocol/$p/"; done; git grep -l 'cmd/pqc-verifier"' -- '*.go'
  ```
  Expected exactly one line: `tomb/lich/harnesses/lich_006_real_lifecycle_test.go`. Anything else: [STUCK].
- [ ] **Step 21** [B] ~1m: Delete the protocol reference packages.
  ```bash
  git rm -rq pkg/protocol/amplification pkg/protocol/dos pkg/protocol/intermediary pkg/protocol/prefetch pkg/protocol/lifecycle pkg/protocol/settings pkg/protocol/flowtype pkg/protocol/sequence pkg/protocol/registry tomb/lich/harnesses/lich_006_real_lifecycle_test.go && echo ok
  ```
- [ ] **Step 22** [V] ~3m: `GOMAXPROCS=4 go build ./... && GOMAXPROCS=4 go vet ./pkg/protocol/... ./tomb/... && go mod tidy -diff | wc -l`. Expected: `0`, nothing else printed.
- [ ] **Step 23** [C] ~1m: **COMMIT** `$P/commit.sh S-WIRE-HONEST "Steps 20-23: TRUE-WIRE R1-R3, R5-R10 deleted with LICH-006's harness"`
- [ ] **Step 24** [B] ~1m: Delete L5: `git rm -rq cmd/pqc-verifier && echo ok`
- [ ] **Step 25** [W] ~1m: Ripple: `python3 $P/truewire_ripple.py`. Expected: `truewire_ripple.py: all edits applied`. `ANCHOR MISSING` or `NO CHANGE`: [STUCK] with the line.
- [ ] **Step 26** [B] ~1m: Regenerate the generated inventory: `scripts/live-path-inventory.sh --report > docs/LIVE-PATHS.md && grep -c pqc-verifier docs/LIVE-PATHS.md`. Expected `0`.
- [ ] **Step 27** [V] ~5m: 
  ```bash
  GOMAXPROCS=4 go build ./... && GOMAXPROCS=4 go test ./pkg/protocol/... ./tomb/... ./pkg/ports/... 2>&1 | grep -vE '^ok|no test files' | wc -l
  ```
  Expected `0`, or only packages in `$P/go-baseline.txt`. `scripts/kingdom-shutdown.sh` still names the binary: it is a host script, so rule 13 keeps that line for Opus at Step 126.
- [ ] **Step 28** [V] ~2m: The regenerated unreached list agrees. uutils `comm` fails on two pipes, so use files.
  ```bash
  mains=$(go list -f '{{if eq .Name "main"}}{{.ImportPath}}{{end}}' ./...); go list ./... | sort -u > $P/all.txt; go list -deps $mains | grep '^unheaded/' | sort -u > $P/reached.txt; comm -23 $P/all.txt $P/reached.txt | grep -c 'pkg/protocol/\(dos\|lifecycle\|settings\)'
  ```
  Expected `0`.
- [ ] **Step 29** [C] ~1m: **COMMIT** `$P/commit.sh S-WIRE-HONEST "Steps 24-29: TRUE-WIRE L5 deleted; all 35 rows placed" docs/UNREACHED-GO-PACKAGES.md docs/sweeps/TRUE-WIRE-DECISIONS.md docs/protocol/G8-REFERENCE-RULINGS.md docs/adr/ADR-062-fuzz-redteam-pentest-framework.md docs/adr/ADR-119-shield-xdp-pqc-fast-path-design.md docs/adr/ADR-094-own-the-metrics-stack.md wiki/Microservices.md wiki/PQC-Authentication.md wiki/Port-Registry.md docs/architecture/PQC_ARCHITECTURE.md docs/LIVE-PATHS.md`

**PHASE 3 EXIT GATE**: `git status --short | wc -l` is `0`; Appendix C matches the table now in `TRUE-WIRE-DECISIONS.md`. Merge: `$P/lane_merge.sh tw`.

## PHASE 4: FN-DSA OUT OF THE REGISTRY (Steps 30-40)

**Goal**: FN-DSA gone from code, 0x03 burned, HQC still reserved, CBOM fresh.
**Prerequisite**: Phase 2. Lane `pq`. **Time**: 25m · **Agent**: Agent [P] · **Model**: Sonnet

- [ ] **Step 30** [W] ~1m: Red first: `cp $P/withdrawn_test.go pkg/crypto/pqc/withdrawn_test.go`
- [ ] **Step 31** [B] ~1m: `GOMAXPROCS=4 go test ./pkg/crypto/pqc/ -run TestAlgoWithdrawn03 2>&1 | tail -4`
- [ ] **Step 32** [V] ~1m: **RED**: output contains `Registry(0x03) found an entry: FN-DSA was withdrawn, 0x03 must not resolve` and `FAIL`. Anything else: [STUCK].
- [ ] **Step 33** [W] ~2m: Apply every code edit: `python3 $P/fndsa.py && git rm -q pkg/crypto/pqc/fn_dsa.go`. Expected: `fndsa.py: all edits applied`. `ANCHOR MISSING` or `PATTERN MISSING`: [STUCK] with the path.
- [ ] **Step 34** [V] ~6m: **GREEN**.
  ```bash
  GOMAXPROCS=4 go build ./... && GOMAXPROCS=4 go test ./pkg/crypto/pqc/ ./pkg/policy/ ./pkg/pqc_policy/ ./pkg/sophia/... ./services/shield/... ./services/sophia/... ./tests/pqc/... ./tomb/lich/harnesses/ 2>&1 | tail -9
  ```
  Expected: eight `ok` lines, no `FAIL`. Any FAIL: [STUCK] with the test name.
- [ ] **Step 35** [C] ~1m: **COMMIT** `$P/commit.sh S-WIRE-HONEST "Steps 30-35: FN-DSA withdrawn from the registry, 0x03 burned" pkg/crypto/pqc/registry.go pkg/crypto/pqc/pqc.go pkg/crypto/pqc/security.go pkg/crypto/pqc/pqc_test.go pkg/crypto/pqc/security_test.go pkg/crypto/pqc/withdrawn_test.go pkg/policy/verification.go pkg/policy/verification_test.go pkg/pqc_policy/nist_level.go pkg/pqc_policy/policy.go pkg/sophia/pqc_maps.go services/shield/pqc_verify.go services/shield/sovereign_verify.go services/sophia/sovereign_map.go services/sophia/pqc_test.go tests/pqc/integration_test.go tomb/lich/harnesses/lich_012_pqc_fuzz_test.go ebpf/pqc-common/src/lib.rs`
- [ ] **Step 36** [V] ~2m: Rust and no leftovers.
  ```bash
  CARGO_TARGET_DIR=$P/host-target cargo test --locked --quiet --manifest-path ebpf/pqc-common/Cargo.toml 2>&1 | grep -c 'FAILED\|error'; git grep -n -iE 'fn-?dsa|falcon' -- '*.go' '*.rs' | grep -v 'withdrawn\|Withdrawn\|DP-411' | wc -l
  ```
  Expected `0` then `0`. Run from the worktree root, not `ebpf/`: `ebpf/.cargo/config.toml` targets BPF.
- [ ] **Step 37** [W] ~1m: Doc ripple: `python3 $P/fndsa_docs.py`. Expected: `fndsa_docs.py: all edits applied`. If lane `tw` is not merged yet this still applies; its edits are on other lines.
- [ ] **Step 38** [C] ~1m: **COMMIT** `$P/commit.sh S-WIRE-HONEST "Steps 36-38: docs follow FN-DSA's withdrawal" wiki/PQC-Authentication.md docs/architecture/PQC_ARCHITECTURE.md`
- [ ] **Step 39** [B] ~2m: `GOMAXPROCS=4 go run ./cmd/crypto-inventory scan --out compliance/crypto/cbom.cdx.json && ./scripts/check-crypto-inventory.sh 2>&1 | tail -1 && ./scripts/check-pq-kex.sh 2>&1 | tail -1`. Expected two `[PASS]` lines.
- [ ] **Step 40** [C] ~1m: **COMMIT** `$P/commit.sh S-WIRE-HONEST "Steps 39-40: CBOM regenerated after FN-DSA" compliance/crypto/cbom.cdx.json`. Exit 3 (`nothing staged`) means the inventory did not change, which is fine.

**PHASE 4 EXIT GATE**: green step's eight `ok`; CBOM gate `[PASS]`.

## PHASE 5: SLH-DSA FINISHED (Steps 41-49)

**Goal**: What "implement SLH-DSA" still means: the six SHAKE sets, one verify dispatch, proof on all twelve. Existing ids do not move (RFC item 8).
**Prerequisite**: Phase 4, same lane. **Time**: 45m · **Agent**: Agent · **Model**: Opus (registry values)

- [ ] **Step 41** [V] ~1m: Confirm the module: `go list -m github.com/cloudflare/circl && ls $(go list -m -f '{{.Dir}}' github.com/cloudflare/circl)/sign/slhdsa/params.go && grep -c SHAKE_ $(go list -m -f '{{.Dir}}' github.com/cloudflare/circl)/sign/slhdsa/params.go`. Expected `v1.6.5`, the path, a count of 6 or more.
- [ ] **Step 42** [W] ~8m: Red: `pkg/crypto/pqc/param_ids_test.go`: `SLHDSA_SHAKE_128s..256f` are 0x07..0x0C with PQC-00's signature sizes; the six SHA2 sets stay 0x10..0x15 and ML-DSA stays 0x01..0x03. Run it: expect `undefined: SLHDSA_SHAKE_128s`.
- [ ] **Step 43** [W] ~10m: Green: in `slh_dsa.go` add the six constants, their `slhdsa.SHAKE_128s..256f` rows and their `slhdsaParamInfo` rows. Make the stale `circl v1.6.3` comments in `pkg/crypto/pqc/*.go` version-free, `hqc.go` included.
- [ ] **Step 44** [C] ~1m: **COMMIT** once `go test ./pkg/crypto/pqc/` is `ok`: `$P/commit.sh S-WIRE-HONEST "Steps 41-44: six SLH-DSA SHAKE sets at PQC-00's ids" pkg/crypto/pqc/slh_dsa.go pkg/crypto/pqc/hqc.go pkg/crypto/pqc/registry.go pkg/crypto/pqc/param_ids_test.go`
- [ ] **Step 45** [W] ~10m: `pkg/crypto/pqc/dispatch.go`: `func VerifySignature(algo AlgoID, params ParameterSet, message, signature, publicKey []byte) (bool, error)` routing to `MLDSAVerifier` and `SLHDSAVerifier`; `ErrAlgorithmNotAvailable` for anything else, including 0x03. Test first in `dispatch_test.go`: ML-DSA-65 and SLH-DSA-SHA2-128f round trips, a flipped bit fails, 0x03 and HQC refuse.
- [ ] **Step 46** [W] ~5m: `TestSLHDSAAllParameterSets`: sign, verify, refuse a tampered message. By default it runs SHA2-128f and SHAKE-128f only; `PQC_SLOW=1` runs all twelve. `ci.yml` runs `go test -race -timeout 120s`, and the `s` sets under `-race` would blow that.
- [ ] **Step 47** [V] ~8m: `PQC_SLOW=1 GOMAXPROCS=4 go test ./pkg/crypto/pqc/ -run TestSLHDSAAllParameterSets 2>&1 | tail -1; GOMAXPROCS=4 go test -race -timeout 120s ./pkg/crypto/pqc/ 2>&1 | tail -1; GOMAXPROCS=4 go test ./pkg/pqc_policy/... ./pkg/wotansig/... ./services/wotan/internal/... ./services/sophia/... ./tests/pqc/... 2>&1 | grep -c FAIL`. Expected `ok`, `ok`, `0`.
- [ ] **Step 48** [C] ~1m: **COMMIT** `$P/commit.sh S-WIRE-HONEST "Steps 45-48: VerifySignature, all twelve SLH-DSA sets proven" pkg/crypto/pqc/dispatch.go pkg/crypto/pqc/dispatch_test.go pkg/crypto/pqc/pqc_test.go`
- [ ] **Step 49** [B] ~2m: `GOMAXPROCS=4 go run ./cmd/crypto-inventory scan --out compliance/crypto/cbom.cdx.json && ./scripts/check-crypto-inventory.sh 2>&1 | tail -1`, then `$P/commit.sh S-WIRE-HONEST "Steps 49-49: CBOM after SLH-DSA" compliance/crypto/cbom.cdx.json`. Exit 3 means no change, which is fine.

**PHASE 5 EXIT GATE**: `go test -race ./pkg/crypto/pqc/` ok; `git grep -n 'v1\.6\.3' -- pkg/crypto/pqc | wc -l` is `0`. Merge: `$P/lane_merge.sh pq`.

## PHASE 6: THE BAR IN ADR-105 (Steps 50-53)

**Goal**: Stevie's bar, the comparison and the gaps in ADR-105. Text final in the payload.
**Prerequisite**: Phase 2. Lane `bar`. Docs only. **Time**: 10m · **Agent**: Agent [P] · **Model**: Haiku

- [ ] **Step 50** [W] ~1m: Append: `cat $P/adr105_bar.md >> docs/adr/ADR-105-pq-hybrid-key-exchange-and-crypto-inventory.md`
- [ ] **Step 51** [W] ~1m: Link it from the wiki.
  ```bash
  printf '\n## The bar\n\nWhere this stands against iMessage PQ3, Signal PQXDH and SPQR and the Microsoft and HKMA roadmaps, with every gap: [ADR-105](https://github.com/unheaded/unheaded/blob/develop/docs/adr/ADR-105-pq-hybrid-key-exchange-and-crypto-inventory.md), "The bar and the gap".\n' >> wiki/Post-Quantum-Key-Exchange.md
  ```
- [ ] **Step 52** [V] ~1m: `grep -c '^| PQ-' docs/adr/ADR-105-pq-hybrid-key-exchange-and-crypto-inventory.md`. Expected `16`.
- [ ] **Step 53** [C] ~1m: **COMMIT** `$P/commit.sh S-WIRE-HONEST "Steps 50-53: ADR-105 the bar, the comparison, sixteen rows" docs/adr/ADR-105-pq-hybrid-key-exchange-and-crypto-inventory.md wiki/Post-Quantum-Key-Exchange.md`

**PHASE 6 EXIT GATE**: count is 16. Merge: `$P/lane_merge.sh bar`.

## PHASE 7: PINNED MAPS IN PKG/EBPF (Steps 54-60)

**Goal**: An in-house way to open a pinned map, and the ENOENT bug fixed.
**Prerequisite**: Phase 2. Lane `api`. **Time**: 25m · **Agent**: Agent [P] · **Model**: Sonnet

Signatures: `OpenPinnedMap(path string) (*PinnedMap, error)`; methods `KeySize() uint32`, `ValueSize() uint32`, `Lookup(key []byte) ([]byte, error)` (nil, nil when absent), `Update(key, value []byte) error`, `Delete(key []byte) error`, `Iterate(fn func(key, value []byte) error) error`, `Close() error`. Refuses per-CPU and ring-buffer maps.

- [ ] **Step 54** [W] ~1m: Red: `cp $P/pinned_linux_test.go pkg/ebpf/pinned_linux_test.go`
- [ ] **Step 55** [V] ~2m: **RED**: `GOMAXPROCS=4 go test ./pkg/ebpf/ -run PinnedMap 2>&1 | tail -4`. Expected: `undefined: ErrKeySize` or `undefined: OpenPinnedMap`, then `[build failed]`.
- [ ] **Step 56** [W] ~1m: Green: `cp $P/pinned_linux.go pkg/ebpf/pinned_linux.go && cp $P/pinned_stub.go pkg/ebpf/pinned_stub.go`
- [ ] **Step 57** [W] ~1m: Fix errno wrapping so `errors.Is(err, syscall.ENOENT)` works.
  ```bash
  sed -i 's/"%w: map_lookup: %v", ErrSyscallFailed, err/"%w: map_lookup: %w", ErrSyscallFailed, err/; s/"%w: map_delete: %v", ErrSyscallFailed, err/"%w: map_delete: %w", ErrSyscallFailed, err/' pkg/ebpf/loader.go; grep -c 'map_lookup: %w\|map_delete: %w' pkg/ebpf/loader.go
  ```
  Expected `2`.
- [ ] **Step 58** [V] ~3m: **GREEN unprivileged**: `gofmt -l pkg/ebpf; GOOS=darwin go vet ./pkg/ebpf/ && GOMAXPROCS=4 go test ./pkg/ebpf/ -run PinnedMap -v 2>&1 | grep -E '^(--- |ok|FAIL)'`. Expected: PASS for `TestOpenPinnedMap_MissingPath` and `TestPinnedMap_KeyAndValueSizeRefused`, SKIP for the other two, `ok`.
- [ ] **Step 59** [V] ~3m: Whole package: `GOMAXPROCS=4 go test ./pkg/ebpf/ ./cmd/trace-collector-go/ 2>&1 | tail -2`. Expected two `ok`. The ENOENT fix makes `ReadMap` return (nil, nil) as its code always intended; `cmd/trace-collector-go/loader_native.go:327` is its only caller.
- [ ] **Step 60** [C] ~1m: **COMMIT** `$P/commit.sh S-WIRE-HONEST "Steps 54-60: pkg/ebpf PinnedMap; ENOENT survives the syscall wrappers" pkg/ebpf/pinned_linux.go pkg/ebpf/pinned_stub.go pkg/ebpf/pinned_linux_test.go pkg/ebpf/loader.go`

**PHASE 7 EXIT GATE**: unprivileged green. The root proof is Phase 8R.

## PHASE 8: PROTOCOL-API --MODE=BPF (Steps 61-68)

**Goal**: bpf mode serves what has a map, on `pkg/ebpf`, and says plainly what has none.
**Prerequisite**: Phase 7, same lane. **Time**: 30m · **Agent**: Agent · **Model**: Sonnet

| Call | bpf mode after this phase | Why |
|---|---|---|
| sophia list, get | reads `hop-ebpf/SOPHIA` | real map |
| flow list | reads `flow-tracker/FLOWS` | real map; no flow label in it, so FlowLabel is 0 |
| flow get | Unimplemented | FLOWS is keyed by 5-tuple |
| flow inject, wotan read, wotan write | Unimplemented | no such map; DP-211 is reads only |
| anamnesis query, stream | Unimplemented | one consumer position: a second reader takes events from trace-collector |

- [ ] **Step 61** [W] ~1m: Tests first: `cp $P/bpfread_test.go cmd/protocol-api/ && cp $P/backend_root_test.go cmd/protocol-api/`
- [ ] **Step 62** [V] ~2m: **RED**: `GOMAXPROCS=4 go test ./cmd/protocol-api/ 2>&1 | tail -4`. Expected: any `undefined:` line, then `[build failed]`.
- [ ] **Step 63** [W] ~2m: `cp $P/bpfread.go cmd/protocol-api/bpfread.go && python3 $P/protoapi.py`. Expected: `protoapi.py: all edits applied`. `ANCHOR MISSING`: [STUCK].
- [ ] **Step 64** [V] ~3m: **GREEN**: `gofmt -l cmd/protocol-api; GOMAXPROCS=4 go vet ./cmd/protocol-api/ && GOMAXPROCS=4 go test ./cmd/protocol-api/ 2>&1 | tail -2`. Expected only `ok`.
- [ ] **Step 65** [C] ~1m: **COMMIT** `$P/commit.sh S-WIRE-HONEST "Steps 61-65: protocol-api bpf mode on pkg/ebpf map reads" cmd/protocol-api/backend.go cmd/protocol-api/bpfread.go cmd/protocol-api/bpfread_test.go cmd/protocol-api/backend_root_test.go cmd/protocol-api/sophia.go cmd/protocol-api/flow.go cmd/protocol-api/wotan.go cmd/protocol-api/anamnesis.go cmd/protocol-api/handlers_extended_test.go`
- [ ] **Step 66** [V] ~1m: `grep -c cilium cmd/protocol-api/backend.go; grep -c 'In a real implementation' cmd/protocol-api/*.go | grep -v ':0' | wc -l; go list -m github.com/cilium/ebpf`. Expected `0`, `0`, then the module line: other binaries still use it, so go.mod does not change.
- [ ] **Step 67** [W] ~2m: Say it in the wiki: `printf '\n`protocol-api --mode=bpf` reads `hop-ebpf/SOPHIA` and `flow-tracker/FLOWS` under `--bpf-maps` (default `/sys/fs/bpf/unheaded`, pinned by `ebpf-loader --pin-maps`). Flow get and inject, Wotan memory slots and Anamnesis stay Unimplemented: no map holds them, and a second reader on a ring takes events from trace-collector.\n' >> wiki/Protocol-Specs.md`
- [ ] **Step 68** [C] ~1m: **COMMIT** `$P/commit.sh S-WIRE-HONEST "Steps 66-68: wiki says what bpf mode reads" wiki/Protocol-Specs.md`

**PHASE 8 EXIT GATE**: `ok`, default `--mode` still `mock` (`grep -n '"mode", "mock"' cmd/protocol-api/main.go` prints one line).

## PHASE 8R: ROOT PROOFS FOR LANE API (Steps 69-72)

**Goal**: `PinnedMap` and `BPFBackend` against real pinned maps. Split out so Phases 7 and 8 stay Sonnet: every [S] step is Opus.
**Prerequisite**: Phases 7, 8, same lane. **Time**: 10m · **Agent**: Agent · **Model**: Opus

- [ ] **Step 69** [B] ~2m: Build as the normal user: `go test -c -o $P/ebpf.test ./pkg/ebpf && go test -c -o $P/api.test ./cmd/protocol-api`
- [ ] **Step 70** [V][S] ~2m: 
  ```bash
  sudo $P/ebpf.test -test.run PinnedMap -test.v 2>&1 | grep -E '^(--- |PASS|FAIL)'; sudo $P/api.test -test.run RealPinnedMap -test.v 2>&1 | grep -E '^(--- |PASS|FAIL)'; sudo ls /sys/fs/bpf | grep -c wire-honest
  ```
  Expected: five `--- PASS`, `PASS` twice, `0`.
- [ ] **Step 71** [W] ~1m: `rm -f $P/ebpf.test $P/api.test`, and log the result in the api lane's progress file.
- [ ] **Step 72** [C] ~1m: **COMMIT** `$P/commit.sh S-WIRE-HONEST "Steps 69-72: root proofs recorded" docs/battle-plans/progress/S-WIRE-HONEST-api-progress.md`

**PHASE 8R EXIT GATE**: all PASS, no pin left. Merge: `$P/lane_merge.sh api`.

## PHASE 9: LATENCY MAP TYPE, RUST (Steps 73-76)

**Goal**: `LatencyTraceKey` replaces the 5-tuple names. Rust first.
**Prerequisite**: Phase 2. Lane `lat`. **Time**: 25m · **Agent**: Agent [P] · **Model**: Opus (eBPF)

- [ ] **Step 73** [W] ~2m: `python3 $P/latency_rust.py`. Expected: `latency_rust.py: all edits applied`.
- [ ] **Step 74** [B] ~6m: Build and load.
  ```bash
  cd ebpf && CARGO_BUILD_JOBS=4 CARGO_TARGET_DIR=target/load-gate/default cargo build --release -p latency-probe 2>&1 | tail -1
  ```
  Expected: `Finished`.
- [ ] **Step 75** [V][S] ~15m: Verifier numbers: `CARGO_BUILD_JOBS=4 EBPF_INSNS_OUT=$P/lat.tsv scripts/check-ebpf-loads.sh 2>&1 | tail -1 && grep '^latency-probe' $P/lat.tsv`. All six programs load. `_exit` rows may move a little: the key is smaller, and `pid`/`tid` are no longer narrowed. Log the numbers in the lat lane's progress file. Over 10 % on any row goes under its "For Stevie", and Step 119 patches that row alone. The 65,536-entry map now holds full tids, not their low 16 bits; `samples` per thread stay correct, and `let _ = insert` drops silently when full, as before. Log whether the reader ever evicts.
- [ ] **Step 76** [C] ~1m: **COMMIT** `$P/commit.sh S-WIRE-HONEST "Steps 73-76: latency-probe keys LATENCY_MAP by LatencyTraceKey" ebpf/latency-probe/src/common.rs ebpf/latency-probe/src/main.rs`

**PHASE 9 EXIT GATE**: load gate `[PASS]`.

## PHASE 10: LATENCY MAP TYPE, GO MIRROR AND READER (Steps 77-82)

**Goal**: bpfschema mirrors the Rust; the reader decodes it, publishes real pid and tid, refuses the old key.
**Prerequisite**: Phase 9, same lane. **Time**: 15m · **Agent**: Agent · **Model**: Sonnet

- [ ] **Step 77** [V] ~1m: **RED**: the parity map entry does not exist yet: `grep -c 'LatencyTraceKey' pkg/protocol/bpfschema/core_maps.go`. Expected `0`.
- [ ] **Step 78** [W] ~1m: `python3 $P/latency_go.py`. Expected: `latency_go.py: all edits applied`. It rewrites `cmd/trace-collector-go/latency_reader.go`, `latency_reader_test.go`, `latency_event_test.go`, `main.go` (one call), `decode_fuzz_test.go`, and `pkg/protocol/bpfschema/{core_maps.go,bpfschema.go,rust_source_test.go}`. If lane `hop` merged first and `rust_source_test.go` moved, `ANCHOR MISSING`: [STUCK] for Opus.
- [ ] **Step 79** [V] ~3m: **GREEN**: `gofmt -l cmd/trace-collector-go pkg/protocol/bpfschema; GOMAXPROCS=4 go vet ./cmd/trace-collector-go/ && GOMAXPROCS=4 go test -race ./cmd/trace-collector-go/ ./pkg/protocol/bpfschema/ 2>&1 | tail -2`. Expected two `ok`. `TestRustSourceStructParity/LatencyTraceKey` is the proof the Go matches the Rust.
- [ ] **Step 80** [V] ~1m: The old key fails loudly: `go test ./cmd/trace-collector-go/ -run 'TestDecodeLatencyTraceKey_WrongSizeRefused' -v 2>&1 | grep -c '^--- PASS'`. Expected `1`.
- [ ] **Step 81** [W] ~1m: Record the ship rule in `docs/battle-plans/progress/S-WIRE-HONEST-lat-progress.md`: "LATENCY_MAP: trace-collector and latency-probe ship together. An old object under a new binary counts `decode_key` errors; nothing misparses."
- [ ] **Step 82** [C] ~1m: **COMMIT** `$P/commit.sh S-WIRE-HONEST "Steps 77-82: LatencyTraceKey mirrored in bpfschema; reader publishes real pid and tid" cmd/trace-collector-go/latency_reader.go cmd/trace-collector-go/latency_reader_test.go cmd/trace-collector-go/latency_event_test.go cmd/trace-collector-go/main.go cmd/trace-collector-go/decode_fuzz_test.go pkg/protocol/bpfschema/core_maps.go pkg/protocol/bpfschema/bpfschema.go pkg/protocol/bpfschema/rust_source_test.go docs/battle-plans/progress/S-WIRE-HONEST-lat-progress.md`

**PHASE 10 EXIT GATE**: two `ok`. Merge: `$P/lane_merge.sh lat`, **before** Phase 13, which edits the same bpfschema files.

## PHASE 11: HOP CHECKS IN HOP-EBPF (Steps 83-97)

**Goal**: ADR-125's checks in `hop_xdp`, off by default, proven by `BPF_PROG_TEST_RUN`, cost measured.
**Prerequisite**: Phases 1, 2. Lane `hop`. **Time**: 3-4 hours · **Agent**: Coordinator · **Model**: Opus (eBPF verifier, security). Marshal watches this lane.

- [ ] **Step 83** [R] ~10m: Read ADR-125, `ebpf/hop-ebpf/src/main.rs`, `pkg/monadpkt/monadpkt.go` (`Build`, `Frame`), `pkg/cartridge/ratelimit_linux_test.go` (how a test loads an object and calls `Run`).
- [ ] **Step 84** [B] ~8m: A lane worktree has no object or loader yet. Build both as the normal user.
  ```bash
  cargo build --release --locked --manifest-path cmd/ebpf-load-check/Cargo.toml 2>&1 | tail -1; (cd ebpf && CARGO_BUILD_JOBS=4 CARGO_TARGET_DIR=target/load-gate/default cargo build --release -p hop-ebpf 2>&1 | tail -1)
  ```
  `Finished` twice. The object is `ebpf/target/load-gate/default/bpfel-unknown-none/release/hop-ebpf`.
- [ ] **Step 85** [W] ~30m: Red: `tests/hopchecks/hop_checks_linux_test.go` (`//go:build linux`), `t.Skip("needs root (CAP_BPF)")` unprivileged, object from `HOP_OBJ`, default `../../ebpf/target/load-gate/default/bpfel-unknown-none/release/hop-ebpf`. It loads with cilium/ebpf (already admitted; test code only), builds a frame with `monadpkt.Build`, runs `hop_xdp`. `Build` leaves `hop_count` and `deploy_ring` at zero, so a helper patches the Monad at frame offset 58 (+0x03, +0x0A) and recomputes the CRC-16/CCITT-FALSE at +0x12. Cases, each asserting the XDP verdict and STATS 15, 16, 17:

  | # | CONFIG[4] | Setup | Verdict | run/failed/enforced |
  |---|---|---|---|---|
  | 1 | absent | validator denying everything | PASS | 0/0/0 |
  | 2 | 1 observe | no validator | PASS | 0/0/0 |
  | 3 | 1 | require_hmac, no key | PASS | 1/1/0 |
  | 4 | 1 | require_hmac, live key | PASS | 1/0/0 |
  | 5 | 1 | require_hmac, expired key | PASS | 1/1/0 |
  | 6 | 2 enforce | require_hmac, no key | DROP | 1/1/1 |
  | 7 | 2 | retry flag, entry present | PASS | 1/0/0 |
  | 8 | 2 | retry flag, wrong source | DROP | 1/1/1 |
  | 9 | 2 | max_hop_count 1, hop_count patched to 3 | DROP | 1/1/1 |
  | 10 | 2 | version range 2..3 | DROP | 1/1/1 |
  | 11 | 2 | require_crc, CUSTOM set | DROP | 1/1/1 |
  | 12 | 0 | case 6's setup | PASS | 0/0/0 |
  | 13 | 2 | max_hop_count 0, version range 0..0 (no rules) | PASS | 1/0/0 |
  | 14 | 2 | validator keyed on deploy_ring patched to 2, frame ring 0 | PASS | 0/0/0 |
- [ ] **Step 86** [B] ~2m: `gofmt -l tests/hopchecks; go vet ./tests/hopchecks/ && go test -c -o $P/hop.test ./tests/hopchecks`
- [ ] **Step 87** [V][S] ~2m: **RED**: `(cd tests/hopchecks && sudo $P/hop.test -test.v 2>&1 | grep -E '^(--- |    --- |FAIL)' | head -6)`. Expected failure from case 1: the object has no `HOP_VALIDATORS` map to populate.
- [ ] **Step 88** [W] ~60m: Implement per ADR-125: map declarations with `#[repr(C)]` value structs `HopValidator`, `HmacKeyEntry`, `RetryEntry`; `CFG_CHECK_MODE = 4`; `STAT_CHECKS_RUN = 15`, `STAT_CHECKS_FAILED = 16`, `STAT_CHECKS_ENFORCED = 17`; key helpers whose bytes equal the Go structs (`flow_id | ns << 16`; `hop_id | ns << 32`; `flow_id | src_low48 << 16`); one `#[inline(always)] fn run_checks(...) -> bool` called after the hop-limit guard, before `let mut m = monad`, reading CONFIG first so mode 0 returns at once. Use `aya_ebpf::helpers::bpf_ktime_get_boot_ns`. No wire field is written.
- [ ] **Step 89** [B] ~6m: `(cd ebpf && CARGO_BUILD_JOBS=4 CARGO_TARGET_DIR=target/load-gate/default cargo build --release -p hop-ebpf 2>&1 | tail -1)`. Expected `Finished`.
- [ ] **Step 90** [V][S] ~3m: Load and read the count. bpftool cannot read aya ELFs; `ebpf-load-check` is what the gate uses.
  ```bash
  sudo cmd/ebpf-load-check/target/release/ebpf-load-check ebpf/target/load-gate/default/bpfel-unknown-none/release/hop-ebpf
  ```
  Expected `hop_xdp OK verified N insns`. Record N as `hop_xdp_insns_post` in `docs/battle-plans/progress/S-WIRE-HONEST-hop-progress.md`.
  - `hop_xdp FAIL ...`: Step 94.
- [ ] **Step 91** [V] ~5m: **BUDGET GATE.** Under 900,000 is mandatory. Growth over 153,835: up to 25 % proceed, with before, after and reason written under "For Stevie" in the hop progress file. This gate records; it approves nothing: Stevie reads those numbers. Over 25 %: Step 93 before going on.
- [ ] **Step 92** [C] ~1m: **COMMIT** after the gate passes, not on a failing branch: `$P/commit.sh S-WIRE-HONEST "Steps 83-92: hop_xdp checks load, count recorded, mode 0 default" ebpf/hop-ebpf/src/main.rs tests/hopchecks/hop_checks_linux_test.go docs/battle-plans/progress/S-WIRE-HONEST-hop-progress.md`
- [ ] **Step 93** [D] ~30m: Over 25 %. In order: (1) make `run_checks` a separate bounded path that returns early on no validator, so the verifier's states do not multiply with the Sophia branches below; (2) `#[inline(never)]` static subprogram taking scalars only, never a packet pointer; (3) [DECIDE] drop the `FLOW_TYPES` dispatch from hop-ebpf: it only counts, it has no writer, and `pkg/protocol/flowtype` is gone. Override only if something reads STATS 23-26. Log the count after each. Two attempts, then [STUCK], ship behind cargo feature `hop-checks` with default off, and leave FND-025 open.
- [ ] **Step 94** [D][S] ~20m: `sudo cmd/ebpf-load-check/target/release/ebpf-load-check ebpf/target/load-gate/default/bpfel-unknown-none/release/hop-ebpf 2>&1 | tail -40`. Usual causes: a packet pointer past `hbh_start`'s variable bound (see the `black_box` notes in the file), a map value read without a null check, a stack over 512 bytes. Two attempts, then [STUCK] as above.
- [ ] **Step 95** [B] ~2m: `go test -c -o $P/hop.test ./tests/hopchecks`
- [ ] **Step 96** [V][S] ~5m: **GREEN**: `(cd tests/hopchecks && sudo $P/hop.test -test.v 2>&1 | grep -E '^(--- |    --- |PASS|FAIL)')`. All fourteen cases PASS. Fix code, not the table; a table change needs a logged reason.
- [ ] **Step 97** [C] ~1m: **COMMIT** `$P/commit.sh S-WIRE-HONEST "Steps 93-97: fourteen hop check cases green" ebpf/hop-ebpf/src/main.rs tests/hopchecks/hop_checks_linux_test.go`. Exit 3 means nothing changed since the last checkpoint, which is fine.

**PHASE 11 EXIT GATE**: fourteen PASS; count recorded; mode 0 is cases 1 and 12. If not, FND-025 stays open and Phases 12-15 are [BLOCKED]; every other lane still merges.

## PHASE 12: SHIELD LOSES THE STUBS (Steps 98-102)

**Goal**: The maps and the comment leave shield-ebpf; `AUTHORITY` and its discarded lookup go.
**Prerequisite**: Phase 11, same lane. **Time**: 30m · **Agent**: Coordinator · **Model**: Opus

- [ ] **Step 98** [W] ~10m: In `ebpf/shield-ebpf/src/main.rs` delete: `HMAC_KEYS`, `RETRY_TOKENS`, `HOP_VALIDATORS`, `AUTHORITY`; `make_hmac_key`, `make_retry_token_key`, `make_authority_key`; the block `// ── 4b–4d. HMAC / Retry / HopValidator lookups`; the block `// ── Authority check (RFC 8200 §4.2)`. Fix step 4's heading: it says "Drop packets" and the code passes them.
- [ ] **Step 99** [W] ~5m: `pkg/protocol/bpfschema`: delete `MapAuthority`, `AuthorityKey`, `AuthorityValue` and their rows in `bpfschema_test.go` and `rust_source_test.go`, because `TestRustSourceMapNames` fails on a constant that names no map. Do this now if lane `lat` has merged; otherwise wait for it.
- [ ] **Step 100** [B][S] ~15m: Build both feature variants through the load gate (it uses sudo itself): `CARGO_BUILD_JOBS=4 EBPF_INSNS_OUT=$P/hop.tsv scripts/check-ebpf-loads.sh 2>&1 | tail -1`. Expected `[PASS]`.
- [ ] **Step 101** [V] ~2m: `grep -E '^shield-ebpf/shield_(xdp|tc)\s' $P/hop.tsv` is at or below 1,502 and 976 (and 1,084 for `debug-stats`). `git grep -nE 'HMAC_KEYS|RETRY_TOKENS|HOP_VALIDATORS|AUTHORITY' -- ebpf/shield-ebpf | wc -l` is `0`.
- [ ] **Step 102** [C] ~1m: **COMMIT** `$P/commit.sh S-WIRE-HONEST "Steps 98-102: shield-ebpf loses the stub maps and the AUTHORITY no-op" ebpf/shield-ebpf/src/main.rs pkg/protocol/bpfschema/bpfschema.go pkg/protocol/bpfschema/bpfschema_test.go pkg/protocol/bpfschema/rust_source_test.go`

**PHASE 12 EXIT GATE**: load gate `[PASS]`.

## PHASE 13: BPFSCHEMA FOLLOWS (Steps 103-105)

**Goal**: Go mirrors match the moved maps. Rust first, as `doc.go` says.
**Prerequisite**: Phase 12 and lane `lat` merged into the sprint branch, then the sprint branch into lane `hop`: `git merge --no-ff -S --no-edit sprint/s-wire-honest` (gpg failure: `git merge --abort`, then `--no-gpg-sign`, recorded). **Time**: 30m · **Agent**: Coordinator · **Model**: Opus

- [ ] **Step 103** [W] ~15m: Comments `(shield-ebpf)` become `(hop-ebpf)` on `MapHMACKeys`, `MapRetryTokens`, `MapHopValidators`. `HMACKeyKey.FlowID`'s comment: "src_service_id | dst_service_id << 8, not a flow label". `CreatedAt`, `ExpiresAt`: CLOCK_BOOTTIME seconds, 0 = never. New `RetryTokenKey`, `RetryTokenValue` matching `RetryEntry`. Add `HopValidator`, `HmacKeyEntry`, `RetryEntry` rows to `structParity` and replace `{"MapRetryTokens", MapRetryTokens, nil, nil}` with typed entries.
- [ ] **Step 104** [V] ~3m: `GOMAXPROCS=4 go test ./pkg/protocol/bpfschema/ 2>&1 | tail -1`. `ok`. A parity failure means the Rust and the Go disagree: Rust wins.
- [ ] **Step 105** [C] ~1m: **COMMIT** `$P/commit.sh S-WIRE-HONEST "Steps 103-105: bpfschema mirrors the moved hop maps" pkg/protocol/bpfschema/bpfschema.go pkg/protocol/bpfschema/bpfschema_test.go pkg/protocol/bpfschema/rust_source_test.go`

**PHASE 13 EXIT GATE**: `ok`.

## PHASE 14: THE WRITER (Steps 106-117)

**Goal**: Something can write the maps; `ebpf-loader` can load hop-ebpf without changing its default set.
**Prerequisite**: Phases 7, 13. **Time**: 60m · **Agent**: Coordinator · **Model**: Opus (key material)

- [ ] **Step 106** [W] ~20m: `pkg/hoprules`, tests first. `Parse(io.Reader) (*File, error)`: YAML with `yaml.v3` (already admitted), `KnownFields(true)`, entries `validators`, `hmac_keys`, `retry`. Each `hmac_keys` entry names `key_file`; `Load` reads 32 bytes and refuses a file that is not mode 0400 or 0600 or not 32 bytes. Key bytes appear in no error and no log. `Apply(w MapWriter, f *File, boottime func() time.Duration) (Applied, error)` with `MapWriter{ Update(mapName string, key, value []byte) error }`; a fake in tests asserts exact key and value bytes against `bpfschema`.
- [ ] **Step 107** [W] ~15m: `cmd/unheaded-cli/cmd/hop.go`: `NewHopCommand()` with subcommand `rules apply --file F [--pin-dir /sys/fs/bpf/unheaded/hop-ebpf]`, the `Command` struct and `flag.NewFlagSet` shape of `token.go`; `root.AddCommand(NewHopCommand())` in `root.go`; writes through `ebpf.OpenPinnedMap`.
- [ ] **Step 108** [W] ~10m: `cmd/ebpf-loader/src/main.rs`: `load_hop` and the match arm `"hop-ebpf" => load_hop(&cli)`, copying `load_shield`'s pin handling and attaching only `hop_xdp`. `ALL_PROGRAMS` is **not** changed, so `--only hop-ebpf` is the one way in.
- [ ] **Step 109** [V] ~6m: `GOMAXPROCS=4 go test -race ./pkg/hoprules/ ./cmd/unheaded-cli/... 2>&1 | tail -3; (cd cmd/ebpf-loader && cargo build --locked 2>&1 | tail -1); git diff $(git merge-base develop HEAD) -- cmd/ebpf-loader/src/main.rs | grep -c '^[-+].*ALL_PROGRAMS'`. Expected `ok` lines, `Finished`, `0`.
- [ ] **Step 110** [W] ~15m: One proof that writer and hop agree on key bytes: `tests/hopchecks` setup now fills maps through `hoprules.Apply` with a `MapWriter` over the test's cilium maps, not hand-built keys.
- [ ] **Step 111** [B] ~2m: `go vet ./tests/hopchecks/ && go test -c -o $P/hop.test ./tests/hopchecks`
- [ ] **Step 112** [V][S] ~3m: `(cd tests/hopchecks && sudo $P/hop.test -test.v 2>&1 | grep -E '^(--- |    --- |PASS|FAIL)')`. Fourteen PASS again. A failure here is a key mismatch, the bug this proof exists to find: fix `pkg/hoprules` or `hop-ebpf` to match `bpfschema`.
- [ ] **Step 113** [C] ~1m: **COMMIT** `$P/commit.sh S-WIRE-HONEST "Steps 106-113: pkg/hoprules, unheaded hop rules apply, ebpf-loader --only hop-ebpf; tests go through the writer" pkg/hoprules/hoprules.go pkg/hoprules/hoprules_test.go cmd/unheaded-cli/cmd/hop.go cmd/unheaded-cli/cmd/root.go cmd/ebpf-loader/src/main.rs tests/hopchecks/hop_checks_linux_test.go`
- [ ] **Step 114** [B] ~3m: Lab files, as the normal user: `go build -o $P/unheaded ./cmd/unheaded-cli && head -c 32 /dev/zero > $P/lab.key && chmod 0400 $P/lab.key`, and `$P/lab-rules.yaml` naming one validator (hop 0, namespace 0, `require_hmac`) and one key (`key_file: <that path>`, flow 0, namespace 0). Zeros only; never a real key.
- [ ] **Step 115** [V][S] ~15m: End to end, both veth ends inside a netns, never a real interface. Use `nsenter --net`, not `ip netns exec`: the latter remounts `/sys` in a private mount namespace, and the pins live on the host's bpffs. Pass `--map-pin-path` and **not** `--pin-maps`, whose `pin_all_maps` writes under the production `/sys/fs/bpf/unheaded`.
  ```bash
  sudo ip netns add wh-lab && sudo ip -n wh-lab link add wh0 type veth peer name wh1 && sudo ip -n wh-lab link set wh0 up && sudo ip -n wh-lab link set wh1 up
  sudo nsenter --net=/run/netns/wh-lab cmd/ebpf-loader/target/debug/ebpf-loader -i wh0 --only hop-ebpf -d ebpf/target/load-gate/default/bpfel-unknown-none/release --xdp-skb-mode --map-pin-path /sys/fs/bpf/wh-lab --pid-file $P/loader.pid > $P/loader.log 2>&1 &
  for i in 1 2 3 4 5 6 7 8 9 10; do sudo test -e /sys/fs/bpf/wh-lab/hop-ebpf/HOP_VALIDATORS && break; sleep 1; done
  sudo $P/unheaded hop rules apply --file $P/lab-rules.yaml --pin-dir /sys/fs/bpf/wh-lab/hop-ebpf
  sudo bpftool map dump pinned /sys/fs/bpf/wh-lab/hop-ebpf/HOP_VALIDATORS | head -3; sudo bpftool map dump pinned /sys/fs/bpf/wh-lab/hop-ebpf/HMAC_KEYS | head -3
  ```
  Both dumps show the written entries.
- [ ] **Step 116** [B][S] ~2m: Tear down by recorded pid and exact name.
  ```bash
  sudo kill "$(sudo cat $P/loader.pid)"; sleep 1; for m in ANAMNESIS SOPHIA CIRCUIT_ERRORS CONFIG STATS SEQ_COUNTERS SETTINGS DOS_STATE FLOW_TYPES HMAC_KEYS RETRY_TOKENS HOP_VALIDATORS; do sudo rm -f "/sys/fs/bpf/wh-lab/hop-ebpf/$m"; done; sudo rmdir /sys/fs/bpf/wh-lab/hop-ebpf /sys/fs/bpf/wh-lab; sudo ip netns del wh-lab; ip netns list | grep -c wh-lab; sudo ls /sys/fs/bpf | grep -c wh-lab; rm -f $P/lab.key $P/lab-rules.yaml
  ```
  Expected `0`, `0`. A map the list missed fails `rmdir`: add its exact name and rerun.
- [ ] **Step 117** [C] ~1m: **COMMIT** `$P/commit.sh S-WIRE-HONEST "Steps 114-117: writer proven in a netns" docs/battle-plans/progress/S-WIRE-HONEST-hop-progress.md`

**PHASE 14 EXIT GATE**: lab read-back shows the written entries; no netns or pin left.

## PHASE 15: FND-025 EVIDENCE BUILT, THE RATCHET RECORDED (Steps 118-124)

**Goal**: The evidence is recorded, FND-025 stays open for Stevie's ruling, the residue opens, one baseline row records the growth.
**Prerequisite**: Phases 11-14 green; lane `lat` merged into this lane (Phase 13). **Time**: 40m · **Agent**: Solo · **Model**: Opus

- [ ] **Step 118** [B][S] ~15m: Fresh numbers, after `lat` so its rows are current: `CARGO_BUILD_JOBS=4 EBPF_INSNS_OUT=$P/post.tsv scripts/check-ebpf-loads.sh 2>&1 | tail -1`. Expected `[PASS]`.
- [ ] **Step 119** [W] ~5m: Rule 17: never `--update`. `scripts/check-ebpf-insns.sh --from $P/post.tsv 2>&1 | tail -3`. If only `hop-ebpf/hop_xdp` is over +10 %, patch that one row:
  ```bash
  N=$(awk -F'\t' '$1=="hop-ebpf/hop_xdp"{print $3}' $P/post.tsv); [ -n "$N" ] && sed -i -E "s#^(hop-ebpf/hop_xdp\tdefault\t)[0-9]+#\1$N#" docs/policy/ebpf-insns-baseline.tsv; git diff --stat docs/policy/ebpf-insns-baseline.tsv | tail -1; scripts/check-ebpf-insns.sh --from $P/post.tsv >/dev/null 2>&1; echo "rc=$?"
  ```
  Expected one line changed, `rc=0`. Both files are `name, feature, count`. A latency-probe row over +10 % is patched the same way, one row at a time. `commit.sh` has no body, so the reason goes in the hop progress file under "For Stevie" (before, after, why) and the count in the commit subject.
- [ ] **Step 120** [B] ~2m: Open count before the edit (rule 16). A worktree has no evidence file.
  ```bash
  GOMAXPROCS=4 go run ./cmd/compliance-evidence findings -evidence /home/govan/tmp/unheaded/var/compliance/evidence.json -adr docs/adr/ADR-098-findings-register-and-baseline-modes.md >/dev/null 2>&1; grep -oE '\*\*[0-9]+ open\*\*' docs/adr/ADR-098-findings-register-and-baseline-modes.md | grep -oE '[0-9]+' > $P/open-before.txt; cat $P/open-before.txt
  ```
  About 150 (measured 153 on 2026-11-04). Under 100 means the evidence file was not read: [STUCK].
- [ ] **Step 121** [W] ~5m: `compliance/findings/register.yaml`: FND-025's `verify:` and status stay **unchanged**; only its `plan:` gains "moved to hop-ebpf, ADR-125; evidence <commit>: tests/hopchecks fourteen cases, shield-ebpf no longer holds the maps, AUTHORITY gone, hop_xdp <n> verified insns; closure awaits Stevie's ruling on the verify text". Write the old verify text and the proposed new one under "For Stevie" in the hop progress file. Add **FND-031**, severity low, class app: "No per-packet integrity on the Monad: hop checks test key presence, not a tag", verify "RFC item 7 is ruled and either a tag exists or the draft drops the option", opened today, step 7. Copy FND-020's `resolved:` form.
- [ ] **Step 122** [B] ~2m: Regenerate and assert.
  ```bash
  GOMAXPROCS=4 go run ./cmd/compliance-evidence findings -evidence /home/govan/tmp/unheaded/var/compliance/evidence.json -adr docs/adr/ADR-098-findings-register-and-baseline-modes.md 2>&1 | tail -1; A=$(grep -oE '\*\*[0-9]+ open\*\*' docs/adr/ADR-098-findings-register-and-baseline-modes.md | grep -oE '[0-9]+'); B=$(cat $P/open-before.txt); echo "$B -> $A"; [ "$A" -ge "$((B-1))" ] && echo OK; grep -c 'FND-031' docs/adr/ADR-098-findings-register-and-baseline-modes.md
  ```
  FND-025 stays open and FND-031 opens, so the count rises by one. Expected `OK`, then 1 or more. No `OK`: the table collapsed; [STUCK], do not commit it.
- [ ] **Step 123** [W] ~8m: Truth in the docs: `git grep -n 'S24\|bpf_loop' -- ebpf/shield-ebpf | wc -l` is `0`; `wiki/eBPF-Programs.md` row for `hop-ebpf` reads "Per-hop Monad processing; validator, key and retry checks, off by default (ADR-125)"; the CLAUDE.md S-TRUE-WIRE line "its HMAC/retry/hop/authority checks stay stubs (FND-025, **E8**...)" becomes "moved to hop-ebpf 2026-11-04 (ADR-125; key presence, not a tag)"; `pkg/protocol/bpfschema/BPF_IPV6_INTERFACE_MAP.md` gets, under its title, "> Status 2026-11-04: HMAC_KEYS, RETRY_TOKENS and HOP_VALIDATORS moved to hop-ebpf (ADR-125, key presence, not a tag); AUTHORITY is deleted; `pkg/protocol/intermediary` is deleted. The tables below predate this." and its step 6 line "Add map definitions to shield-ebpf (HMAC, retry, hop validators)" names hop-ebpf.
- [ ] **Step 124** [C] ~1m: **COMMIT** with the count in the subject: `$P/commit.sh S-WIRE-HONEST "Steps 118-124: FND-025 evidence recorded, FND-031 opened, hop_xdp baseline <n>" compliance/findings/register.yaml docs/adr/ADR-098-findings-register-and-baseline-modes.md docs/policy/ebpf-insns-baseline.tsv wiki/eBPF-Programs.md CLAUDE.md pkg/protocol/bpfschema/BPF_IPV6_INTERFACE_MAP.md docs/battle-plans/progress/S-WIRE-HONEST-hop-progress.md`

**PHASE 15 EXIT GATE**: `scripts/check-ebpf-insns.sh --from $P/post.tsv` exits 0. Merge: `$P/lane_merge.sh hop`.

## PHASE 16: SECURITY READ AND THE SHUTDOWN LINE (Steps 125-127)

**Goal**: Fresh eyes on the two things that handle keys and drops; the one host script edit, held for Opus by rule 13.
**Prerequisite**: Phase 15. **Time**: 30m · **Agent**: Agent · **Model**: Opus

- [ ] **Step 125** [R] ~20m: Load `unheaded-blackmage` and read `pkg/hoprules`, `run_checks` and `tests/hopchecks` against: enforce mode as a DoS lever (a validator on bucket 0 drops every Shield-born packet), key bytes in errors, a symlinked `key_file`, pin ownership. Findings go to the progress file; a real one gets a register row and a fix or [STUCK].
- [ ] **Step 126** [W] ~2m: `scripts/kingdom-shutdown.sh` names the deleted binary. `sed -i '/^    pqc-verifier$/d' scripts/kingdom-shutdown.sh && bash -n scripts/kingdom-shutdown.sh && grep -c pqc-verifier scripts/kingdom-shutdown.sh`. Expected `0`. Do not run the script.
- [ ] **Step 127** [C] ~1m: **COMMIT** `$P/commit.sh S-WIRE-HONEST "Steps 125-127: security read recorded; shutdown list follows L5" docs/battle-plans/progress/S-WIRE-HONEST-progress.md scripts/kingdom-shutdown.sh`

**PHASE 16 EXIT GATE**: no open high finding from this read.

## PHASE 17: SPRINT EXIT (Steps 128-137)

**Goal**: All lanes in, develop brought in first (rule 15), every gate green on that tree, one merge, one handoff.
**Prerequisite**: every lane merged into `sprint/s-wire-honest`; **S-CLEAN-CUT merged into develop** (hard prerequisite: both edit `bpfschema.go`, and it merges first). **Time**: 60m · **Agent**: Coordinator · **Model**: Opus

- [ ] **Step 128** [V] ~1m: `git -C /home/govan/tmp/unheaded merge-base --is-ancestor sprint/s-clean-cut develop && echo MERGED`. Expected `MERGED`. Anything else: wait; do not merge. Record it in the progress file and stop here.
- [ ] **Step 129** [B] ~3m: Rule 15: `git merge --no-ff -S --no-edit develop`. gpg failure: `git merge --abort; git merge --no-ff --no-gpg-sign --no-edit develop`, recorded. A conflict is [STUCK]: `git merge --abort`, no merge to develop.
- [ ] **Step 130** [B] ~15m: Package names, timings stripped, against the `-race` baseline. `tests/pqc/integration_test.go` was unformatted before this sprint, so it is the one allowed gofmt line.
  ```bash
  gofmt -l $(git diff --name-only --diff-filter=d $(git merge-base develop HEAD) -- '*.go'); GOMAXPROCS=4 go vet ./pkg/... ./cmd/... ./services/... ./tests/hopchecks/ 2>&1 | tail -3; GOMAXPROCS=4 go test -race ./... 2>&1 | grep -E '^FAIL[[:space:]]+unheaded/' | awk '{print $2}' | sort -u > $P/go-post.txt; comm -13 $P/go-baseline.txt $P/go-post.txt | wc -l
  ```
  Expected `0`: no package failing that was not failing before.
- [ ] **Step 131** [B] ~5m: `for g in check-gosec-ratchet check-dependency-baseline check-sbom-trend check-http-timeouts check-crypto-inventory check-pq-kex gen-man; do ./scripts/$g.sh >/dev/null 2>&1 && echo "PASS $g" || echo "FAIL $g"; done`. Seven `PASS` lines. Exit code is the verdict; their last lines differ in wording.
- [ ] **Step 132** [V][S] ~20m: `CARGO_BUILD_JOBS=4 ./scripts/check-ebpf-insns.sh 2>&1 | tail -2`. Pass: it loads everything through sudo. ASCEND and cart rows must not move: nothing here touches them.
- [ ] **Step 133** [V] ~1m: **WIRE GATE, second reading**: `git diff --stat $(git merge-base develop HEAD) -- ebpf/monad-common/src/lib.rs docs/protocol/draft-bellis-unheaded-protocol-foundation-06.md docs/protocol/draft-bellis-unheaded-pqc-authentication-00.md | wc -l` is `0`.
- [ ] **Step 134** [V] ~2m: Nothing live moved: `git diff --name-only $(git merge-base develop HEAD) -- deploy docker-compose.yml | wc -l` is `0`. Log `systemctl list-units 'unheaded*' --no-pager --state=running --no-legend | wc -l` beside "running units"; other plans may move it, so a difference is noted, not a failure.
- [ ] **Step 135** [W] ~3m: Close the progress file: per-phase status, `hop_xdp` before and after, the lane progress files by name, For Stevie, unsigned commits if any.
- [ ] **Step 136** [C] ~1m: **COMMIT** `$P/commit.sh S-WIRE-HONEST "Steps 128-136: exit gates green, handoff" docs/battle-plans/progress/S-WIRE-HONEST-progress.md`
- [ ] **Step 137** [B] ~2m: Merge. The coordinator runs this on a clean develop; never push. A rerun of Steps 130 to 133 is needed if develop moved since.
  ```bash
  M=$(printf 'Merge sprint/s-wire-honest (S-WIRE-HONEST)\n\nCo-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>\nClaude-Session: https://claude.ai/code/session_01TzL3Tqe5ebLDopsg5ZkZdp'); git -C /home/govan/tmp/unheaded merge --no-ff -S sprint/s-wire-honest -m "$M" || { git -C /home/govan/tmp/unheaded merge --abort; git -C /home/govan/tmp/unheaded merge --no-ff --no-gpg-sign sprint/s-wire-honest -m "$M"; echo UNSIGNED; }
  ```
  `UNSIGNED` goes in the progress file and the handoff.

**PHASE 17 EXIT GATE**: develop holds the merge; the coordinator folds the progress files into `~/tmp/next.md`.

---

## Appendix A: Swarm table

Rule 3: at most three builders across the whole series. Doc-only lanes do not count. If other plans hold slots, take this order down to one slot: `hop`, `tw`, `pq`, `api`, `lat`.

| Wave | Lane | Worktree suffix | Phases | Model | Files it owns | Builder? |
|---|---|---|---|---|---|---|
| 0 | main | none | 0, 1, 2 | Opus, Opus, Haiku | progress file, RFC queue, ADR-125, 126, ADR-INDEX | yes, alone |
| 1 | bar | `-bar` | 6 | Haiku | ADR-105, `wiki/Post-Quantum-Key-Exchange.md` | no |
| 1, slot A | hop | `-hop` | 11, 12, (13-15 later) | Opus | `ebpf/hop-ebpf`, `ebpf/shield-ebpf`, `tests/hopchecks`, `pkg/hoprules`, `cmd/ebpf-loader`, `cmd/unheaded-cli/cmd/hop.go`, `compliance/findings`, `docs/policy/ebpf-insns-baseline.tsv` | yes |
| 1, slot B | tw, then pq | `-tw`, `-pq` | 3; then 4, 5 | Haiku; Sonnet, Opus | `pkg/protocol/<nine>`, `cmd/pqc-verifier`, ripple list; `pkg/crypto/pqc`, `pkg/policy`, `pkg/pqc_policy`, `services/shield` (2 files), `services/sophia` (2), `tests/pqc`, `ebpf/pqc-common` | yes |
| 1, slot C | api, then lat | `-api`, `-lat` | 7, 8, 8R; then 9, 10 | Sonnet, Opus; Opus, Sonnet | `pkg/ebpf/{pinned*,loader.go}`, `cmd/protocol-api`; `ebpf/latency-probe`, `cmd/trace-collector-go`, `pkg/protocol/bpfschema` | yes |
| 2 | hop | `-hop` | 13, 14, 15 | Opus | as above | yes |
| 3 | main | none | 16, 17 | Opus | progress file, `scripts/kingdom-shutdown.sh` | yes |

Phase 13 waits for `lat`'s merge; Phase 14 for `api`'s. Shared files, by line, no conflict expected: `wiki/PQC-Authentication.md` (`tw` removes one row, `pq` edits another), `docs/architecture/PQC_ARCHITECTURE.md` (same). Run `check-ebpf-loads.sh` from one lane at a time: it is the heavy build.

## Appendix B: Agent assignment and critical path

| Phase | Model | Depends on | Time |
|---|---|---|---|
| 0 Preflight | Opus | none | 60m |
| 1 RFC desk | Opus | 0 | 30m |
| 2 ADRs | Haiku | 1 | 10m |
| 3 TRUE-WIRE | Haiku | 2 | 30m |
| 4 FN-DSA | Sonnet | 2 | 25m |
| 5 SLH-DSA | Opus | 4 | 45m |
| 6 The bar | Haiku | 2 | 10m |
| 7 PinnedMap | Sonnet | 2 | 25m |
| 8 protocol-api | Sonnet | 7 | 30m |
| 8R Root proofs | Opus | 8 | 10m |
| 9 Latency Rust | Opus | 2 | 25m |
| 10 Latency Go | Sonnet | 9 | 15m |
| 11 Hop checks | Opus | 2 | 3-4h |
| 12 Shield | Opus | 11 | 30m |
| 13 bpfschema | Opus | 12, 10 | 30m |
| 14 Writer | Opus | 8R, 13 | 90m |
| 15 FND-025 | Opus | 14 | 40m |
| 16 Security read | Opus | 15 | 30m |
| 17 Exit | Opus | all, S-CLEAN-CUT | 60m |

Critical path: 0 → 1 → 2 → 11 → 12 → 13 → 14 → 15 → 16 → 17, about 9-10 hours with three slots, about 14 with one.

## Appendix C: TRUE-WIRE, all 35 rows

| Rows | Default | What happens | Where |
|---|---|---|---|
| R1 R2 R3 R5 R6 R7 R8 R9 R10 | DELETE | deleted | Phase 3 |
| R4 | PARK until E8 | E8 answered; stays parked, `ValidateToken` must not be wired | Phase 3 note |
| C1-C6 | DELETE | **not deleted**, DP-203 | S-G9-FOLD |
| C7 C8 | KEEP-PARKED | folded behind flags, DP-203 | S-G9-FOLD |
| C9 | KEEP-PARKED | parked | none |
| E1-E6 | KEEP | nothing | none |
| L1 | DELETE | DP-205 | S-CLEAN-CUT |
| L2 L3 L4 | DELETE | DP-401 | S-CLEAN-CUT |
| L5 | DELETE | deleted | Phase 3 |
| L6 | ASK | open | For Stevie |
| L7 | KEEP-PARKED | parked | none |
| T1 T2 T3 | ASK | DP-401, DP-218 | S-CLEAN-CUT |

## Appendix D: Emergency procedures

- **`go build ./...` fails on doomgeneric in a worktree**: `~/tmp/wt/projects` is missing. Step 2.
- **Verifier rejects hop_xdp**: Step 94. Never raise the budget; never add to `ebpf-insns-over-budget.txt`.
- **Load gate replaces an ELF someone needs**: it builds into `ebpf/target/load-gate` by design. Never run plain `cargo build --release` in `ebpf/` of the main tree: it replaces the `ascend-linux` monad-cpu object.
- **`bpf` pin or netns left behind** (Opus only, a named [S] step; a cheaper agent marks [STUCK]): the teardown at the end of Phase 14, by exact name. Test pins live in `/sys/fs/bpf/wire-honest-*` and clean themselves; if one survives, `sudo ls` it and `sudo rm -f` each exact path, then `sudo rmdir` the directory. No glob, no `rm -rf`. `sudo ip netns del wh-lab`. Stop a loader with `sudo kill "$(sudo cat $P/loader.pid)"`; never `pkill -f`.
- **gpg-agent timed out**: `commit.sh` and `lane_merge.sh` fall back to `--no-gpg-sign` and say so. Note it in the handoff, not the message.
- **A lane merge conflicts**: Opus resolves; Haiku and Sonnet mark [STUCK] and stop that lane.
- **Rollback of a lane** (Opus): nothing is pushed. `git worktree remove` the lane and delete `sprint/s-wire-honest-<lane>`.

## Appendix E: Quick reference

- Hop CONFIG: 0 hop_id, 1 sample_divisor, 2 circuit_threshold, 3 max_hops, **4 check mode (0 off, 1 observe, 2 enforce)**. STATS: **15 checks_run, 16 checks_failed, 17 checks_enforced**; 18, 19 free.
- `HopValidatorValue` (32): version min 0, max 1, require_crc 2, require_hmac 3, allowed_dict_ids 4-19 (not evaluated), max_hop_count 20, flags u32 at 24 (bit 0 require retry).
- Monad: version 0x00, src 0x01, dst 0x02, hop_count 0x03, qos 0x04, action 0x05, circuit 0x06, flags 0x07, latency 0x08, ring 0x0A, mesh 0x0B, prefixes 0x0C-0x0D, scratch 0x0E-0x11, checksum 0x12-0x13.
- Pins: `<base>/<program>/<MAP>`; base `/sys/fs/bpf/unheaded`.
- Object: `ebpf/target/load-gate/default/bpfel-unknown-none/release/<program>`.
- Root Go tests (Opus): `go test -c -o $P/x.test ./pkg` as the normal user, then `sudo $P/x.test -test.run NAME -test.v`.
- Verified count of one object: `sudo cmd/ebpf-load-check/target/release/ebpf-load-check <object>`. bpftool cannot read aya ELFs.

## Appendix F: Payloads

Extracted by Phase 0 to `/var/tmp/S-WIRE-HONEST-payloads/`. Rehearsed as the normal user on a scratch snapshot of develop. Do not retype them.

<!-- PAYLOAD commit.sh -->
````text
#!/usr/bin/env bash
# usage: commit.sh PLAN "Steps X-Y: subject" [path ...]   (run from the worktree root)
# Stages only the named paths; `git rm` deletions are already staged.
set -euo pipefail
plan=$1; subject=$2; shift 2
if [ $# -gt 0 ]; then git add -- "$@"; fi
if git diff --cached --quiet; then echo "commit.sh: nothing staged"; exit 3; fi
msg="[PLAN ${plan}] ${subject}

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>
Claude-Session: https://claude.ai/code/session_01TzL3Tqe5ebLDopsg5ZkZdp"
if ! git commit -q -S -m "$msg"; then
  git commit -q --no-gpg-sign -m "$msg"
  echo "commit.sh: gpg unavailable; committed UNSIGNED (record it in the handoff)"
fi
git log -1 --format='%h %G? %s'
````

<!-- PAYLOAD lane.sh -->
````text
#!/usr/bin/env bash
# usage: lane.sh <lane>   Create a lane worktree and its progress file.
set -euo pipefail
lane=$1; wt=/home/govan/tmp/wt/S-WIRE-HONEST-$lane
git -C /home/govan/tmp/wt/S-WIRE-HONEST worktree add -q -b "sprint/s-wire-honest-$lane" "$wt" sprint/s-wire-honest
f=docs/battle-plans/progress/S-WIRE-HONEST-$lane-progress.md
printf '<!-- SPDX-License-Identifier: GPL-3.0-or-later -->\n# S-WIRE-HONEST lane %s progress\n\n## Log\n\n## For Stevie\n' "$lane" > "$wt/$f"
echo "$wt"
````

<!-- PAYLOAD lane_merge.sh -->
````text
#!/usr/bin/env bash
# usage: lane_merge.sh <lane>   Merge a finished lane into sprint/s-wire-honest.
# A conflict aborts and exits 1: only Opus resolves it.
set -uo pipefail
lane=$1; b=sprint/s-wire-honest-$lane; m="Merge lane $lane into sprint/s-wire-honest"
cd /home/govan/tmp/wt/S-WIRE-HONEST || exit 1
if git merge -q --no-ff -S "$b" -m "$m"; then echo "merged $lane"; exit 0; fi
git merge --abort 2>/dev/null
if git merge -q --no-ff --no-gpg-sign "$b" -m "$m"; then echo "merged $lane UNSIGNED: record it in the handoff"; exit 0; fi
git merge --abort 2>/dev/null
echo "CONFLICT merging $lane: lane stays open, Opus resolves"; exit 1
````

<!-- PAYLOAD RFC-EDITOR-REQUESTS-WH.md -->
````text
<!-- SPDX-License-Identifier: GPL-3.0-or-later -->
# RFC Editor requests: S-WIRE-HONEST

Opened 2026-11-04 by S-WIRE-HONEST Phase 1. Items 1-4 are the four spec questions DP-715 sends to the RFC desk with the I-D resubmission (DP-606). Items 5-9 were found while forging the plan. None is ruled here. Each one either redefines a frozen Monad bit or moves a registered value, so each needs Stevie at the resubmission sitting.

Monad v0x01 is unchanged by every item here and by S-WIRE-HONEST.

| # | Draft | Question | What the tree says | Recommendation |
|---|---|---|---|---|
| 1 | pqc-authentication-00 §10 | Where do the K1\|K0 tier bits live? | Three places disagree. PQC-00 says "the Monad flags byte", but foundation-06 assigns all eight bits of 0x07 (CHAOS, CANARY, TRACED, ENCRYPT, SAMPLED, MIRROR, CUSTOM, RSVD). `docs/protocol/KINGDOM_MODE.md` says bits 6-7 of byte 0x10 (scratch[2]). `ebpf/compliance-ebpf/src/main.rs:255` reads the low two bits of scratch[0] (0x0E) and calls them data classification, a different meaning. | No wire bits: the tier is a per-service Sophia policy value, read by the verifier. PQC-00 §10 and KINGDOM_MODE.md change; compliance-ebpf's classification gets its own ruling. |
| 2 | foundation-06 "PQC Authentication Value Format" (cited as Section 11.17; the heading has no number) | 12-byte value or 6-byte window? | The section puts a 12-byte value in offsets 0x0E-0x13. That is 6 octets, and 0x12-0x13 is the checksum, so 4 usable. PQC-00 §5 uses 24-bit SigRef and KeyRef; foundation-06 and `ebpf/pqc-common` use 32-bit SigRef and 16-bit KeyRef. ADR-119 "Two wire facts" records the same. | v0x01 carries the 32-bit SigRef in scratch[0..4] and nothing else; KeyRef, HashPfx and SeqNum move into the Sophia signature entry. Both drafts change. |
| 3 | foundation-06 hop_count | Hop-limit drop or forward, and what is the default limit? | The draft: a hop MUST compare hop_count with a deployment limit, MUST emit EVENT_ANOMALY, and then drops (RECOMMENDED) or forwards by policy. `ebpf/hop-ebpf` always forwards, and CONFIG key 3 = 0 turns the check off, which the MUST does not allow. | Default limit 64; one policy key, default drop. A hop-ebpf change, after the ruling. |
| 4 | foundation-06 "IPv6 Next Header Type Allocation" | Keep the Next Header 0xFE row? | 254 is one of the two IP protocol numbers RFC 3692 reserves for experiments, so it cannot be allocated, and the draft says the normative path does not use it. | Delete the row and the IANA request. |
| 5 | foundation-06 "IPv6 Hop-by-Hop Option Type" | 0x3E is not an assignable value | The draft suggests 0x3E, which RFC 4727 sets aside for experiments. `crates/zhend/src/monad/hbh.rs:23` uses the same 0x3E for Zhen; only the option length tells them apart. ADR-126 is the private registry. | Ask IANA for a new value at the resubmission (DP-412), and keep 0x3E as the limited-domain value until then. |
| 6 | pqc-authentication-00 tables and §10 | FN-DSA withdrawn (DP-411) | PQC-00's registry row, parameter sets 0x20-0x21, the ENHANCED tier and "All three, 2-of-3" for SOVEREIGN name it, and so does foundation-06's "PQC Algorithm Identifiers" row `0x03 FN-DSA FIPS 206`. Code keeps 0x03 burned and SOVEREIGN needs SLH-DSA and ML-DSA. FN-DSA and ML-DSA were one family, so FN-DSA never added a distinct family. | Mark 0x03 Reserved in both drafts and rewrite SOVEREIGN as two distinct families. `SigRef3` stays reserved. |
| 7 | foundation-06 "(b) The optional HMAC field (not defined in this document)" | Define the field, or drop option (b)? | The Monad has no room for a tag. `pkg/protocol/integrity` makes an 8-byte tag nothing can carry, and eBPF has no HMAC helper. hop-ebpf can check that a key exists and is live, not that a packet is authentic (ADR-125). | Drop option (b); (a) ESP and (c) ML-DSA at Shield boundaries remain. |
| 8 | foundation-06 "PQC Algorithm Registry" and "PQC Algorithm Identifiers"; pqc-authentication-00 parameter sets | Which table owns these values? | Three disagree. foundation-06's registry: ML-KEM-768 0x01, ML-KEM-1024 0x02, ML-DSA-65 0x03, ML-DSA-87 0x04, SLH-DSA-SHA2-128s 0x05. foundation-06's identifiers: families SLH-DSA 0x01, ML-DSA 0x02, FN-DSA 0x03, ML-KEM 0x04, HQC 0x05. PQC-00: sets SLH-DSA 0x01-0x0C, ML-DSA 0x10-0x12. Code has PQC-00's families, SLH-DSA sets at 0x10-0x15 and ML-DSA sets at 0x01-0x03. No value is stored anywhere. S-WIRE-HONEST leaves existing ids alone and adds the six SHAKE sets at 0x07-0x0C, which no table contradicts. | A question for Stevie: one table, then code follows it. |
| 9 | foundation-06 trace_id | Is the Flow Label per flow or per packet? | Shield stamps `bpf_get_prandom_u32()` per packet (`ebpf/shield-ebpf/src/main.rs`, "Generate trace correlation Flow Label"); RFC 6437 wants one label per flow. hop-ebpf derives SEQ_COUNTERS and DOS_STATE namespaces from `flow_label >> 8`, so on a Shield-born packet those keys are random. | Label per flow, hashed from the 5-tuple. A Shield change, after the ruling. |
````

<!-- PAYLOAD ADR-125-hop-checks.md -->
````text
<!--
SPDX-License-Identifier: GPL-3.0-or-later
Copyright (c) 2024-2026 Stevie Bellis.
-->
# ADR-125: Hop checks: validators, HMAC-key presence and retry state move from Shield ingress to hop-ebpf

**Status:** Proposed. DP-716 (a) decided the move; this ADR records the semantics that answer did not spell out. Stevie accepts it.
**Date:** 2026-11-04
**Related:** FND-025, DP-716, DP-715, ADR-012 (verifier budget), ADR-119 (the PQC fast path; its fact 2 on flow labels), `docs/protocol/RFC-EDITOR-REQUESTS-WH.md`, S-TRUE-WIRE progress "E8".

## The pitch (decision pack, DP-716)

> HMAC, retry and hop checks sit at XDP ingress where packets carry no Monad yet. (a) Move them to hop-ebpf. Answer: Shield checks move to hop-ebpf.

## Context

At Shield ingress the Monad does not exist yet: Shield creates it. So `HMAC_KEYS`, `RETRY_TOKENS` and `HOP_VALIDATORS` had nothing to key on, nothing wrote them, and the checks were never built. hop-ebpf sees a Monad on every hop. Four facts decide what can move:

1. **No HMAC can be verified.** v0x01 has no tag field, foundation-06 leaves "the optional HMAC field" undefined, and eBPF has no HMAC helper. Only the presence and expiry of a key can be checked.
2. **No retry token is on the wire either.** `RETRY_TOKENS` can only say that a source was validated before and that the validation is still live.
3. **The flow label cannot key anything.** Shield draws it at random per packet. `HMACKeyKey.FlowID` is documented as "from Monad header", and the Monad has no such field.
4. **Budget.** `hop_xdp` is 153,835 verified instructions. The ratchet fails above 169,218 unless growth is reviewed and recorded.

## Decision

- **Maps.** `HMAC_KEYS`, `RETRY_TOKENS` and `HOP_VALIDATORS` move to `ebpf/hop-ebpf`; Rust first, `bpfschema` follows. `AUTHORITY` and its discarded lookup are deleted from shield-ebpf: it keyed `qos_class` as a dictionary id and no packet carries an owner. FND-025's verify text allows "enforce or gone".
- **Keys.** `flow_id` = `src_service_id | dst_service_id << 8`, the `CIRCUIT_ERRORS` pair. `namespace` = `deploy_ring`. `hop_id` = CONFIG key 0. Layouts match the Go structs byte for byte. Shield leaves service ids at 0, so Shield-born packets share one bucket until something resolves them; a validator on that bucket applies to all of them.
- **Order.** After the hop-limit guard, before the Monad is cloned: look up `HOP_VALIDATORS[(hop_id, namespace)]`. No validator: no check runs. With one: version in range (`allowed_version_min` and `max` both 0 means no version rule); hop_count at or under `max_hop_count` (0 means no cap, as CONFIG key 3 does); if `require_crc`, a CUSTOM packet fails (it has no CRC to check); if `require_hmac`, a live `HMAC_KEYS` entry; if flag bit 0, a live `RETRY_TOKENS` entry for (flow_id, low 48 bits of source). `allowed_dict_ids` is not evaluated: no dictionary id is on the wire.
- **Time.** `expires_at` is CLOCK_BOOTTIME seconds, read with `bpf_ktime_get_boot_ns`, 0 = never. The writer converts. ADR-119 D6.4(c)'s one-clock rule.
- **Modes.** CONFIG key 4: 0 off (default), 1 observe, 2 enforce. Observe counts and emits ANOMALY; enforce also drops. STATS 15 `checks_run`, 16 `checks_failed`, 17 `checks_enforced`.
- **Writer.** `unheaded hop rules apply` over `pkg/hoprules` and `pkg/ebpf.PinnedMap`. Key bytes come from 0400 files named in the rules file, never from the file itself.

## What this is not

This is an authorization check on hop-local state. It does not prove that a packet came from who it says. Integrity against a compromised intermediate node stays with foundation-06's options (a) ESP and (c) ML-DSA at Shield boundaries, or ADR-119 if it is ever built. The residual gap is a new finding, FND-031.

## Consequences

Default behaviour is unchanged: mode 0 and empty maps. Nothing loads hop-ebpf in production today. `cmd/ebpf-loader --only hop-ebpf` is new and is not in the default program list. Growth over the ratchet is recorded with `check-ebpf-insns.sh --update` and the reason.

## Open questions

1. Should the validator also key on `dst_service_id` alone, so one rule can guard a service?
2. Does `pkg/protocol/integrity` stay once RFC item 7 is ruled?
````

<!-- PAYLOAD ADR-126-private-hbh-option-registry.md -->
````text
<!--
SPDX-License-Identifier: GPL-3.0-or-later
Copyright (c) 2024-2026 Stevie Bellis.
-->
# ADR-126: A private Hop-by-Hop option registry

**Status:** Proposed (parked). No code. Stevie accepts it.
**Date:** 2026-11-04
**Related:** DP-412, DP-606, DP-209, foundation-06 "IPv6 Hop-by-Hop Option Type", `docs/protocol/RFC-EDITOR-REQUESTS-WH.md` item 5.

## The pitch (decision pack, DP-412)

> Private HbH option registry in an ADR now, IANA with the resubmission.

## Context

The pack calls 0x3E "zhend's". It is also the Monad's. `ebpf/monad-common/src/lib.rs:56` has `MONAD_OPT_TYPE = 0x3E`; `crates/zhend/src/monad/hbh.rs:23` has `ZHEN_OPTION_TYPE = 0x3E`, marked "placeholder". Only the option length tells them apart: Monad parsers want exactly 20, `parse_zhen_option` wants at least 32. Today that is harmless. zhend is single-node and gRPC-only (DP-209), so its HbH path never runs on a live packet. A third option with a length of 20 or 32 would be misread.

0x3E is one of the eight values RFC 4727 sets aside for experiments (0x1E, 0x3E, 0x5E, 0x7E, 0x9E, 0xBE, 0xDE, 0xFE), so IANA will not assign it. It is correct for a limited domain and wrong as the value foundation-06 suggests.

## Decision

Keep this table as the registry. A new option gets a row before any code uses it. A value is never reused. Two rows never share a value.

| Value | act | chg | Data length | Owner | Status |
|---|---|---|---|---|---|
| 0x3E | 00 skip | 1 mutable | 20 exactly | Monad, foundation-06 | In use; IANA value requested at the resubmission (DP-606) |
| 0x1E | 00 skip | 0 immutable | 32 or more | Zhen fragment advertisement | Reserved here; `hbh.rs` still says 0x3E until Stevie accepts this |

0x1E is the RFC 4727 experimental value with the same skip behaviour. It needs Zhen's option to be immutable en route: the file comment says "Mutable: YES", but nothing rewrites it and it carries a hash of the fragment.

## Consequences

When accepted: one constant in `hbh.rs` and its tests, plus a note in `crates/zhend`. No wire change: Zhen's option has never been on a wire. The IANA ask goes with the resubmission, as answered.

## Open questions

1. Does Zhen want a real IANA value, or stay private to the limited domain?
2. Any other option on the roadmap that should reserve a row now?
````

<!-- PAYLOAD index_rows_wire.py -->
````text
#!/usr/bin/env python3
"""Add ADR-125 and ADR-126 to docs/adr/ADR-INDEX.md after the last numbered row."""
import re, sys
p = "docs/adr/ADR-INDEX.md"; s = open(p).read()
rows = """| [125](ADR-125-hop-checks.md) | Hop checks: validators, HMAC-key presence and retry state move from Shield ingress to hop-ebpf; key presence only, the Monad carries no tag | **Proposed** | 2026-11-04 |
| [126](ADR-126-private-hbh-option-registry.md) | Private Hop-by-Hop option registry: 0x3E is the Monad's and Zhen's today, length alone separates them; Zhen reserved at 0x1E | **Proposed** (parked) | 2026-11-04 |
"""
m = list(re.finditer(r"^\| \[1\d\d\]\(ADR-1\d\d-[^\n]*\n", s, flags=re.M))
if not m:
    sys.exit("ANCHOR MISSING: no ADR-1xx rows")
i = m[-1].end()
open(p, "w").write(s[:i] + rows + s[i:])
print("index rows added after:", m[-1].group(0)[:30])
````

<!-- PAYLOAD truewire_ripple.py -->
````text
#!/usr/bin/env python3
"""DP-715 doc ripple for the deletions. Run from the worktree root."""
import re, sys
def edit(p, fn):
    s = open(p).read(); s2 = fn(s)
    if s2 == s:
        sys.exit(f"NO CHANGE in {p}")
    open(p, "w").write(s2)
def rep(old, new):
    def f(s):
        if old not in s:
            sys.exit(f"ANCHOR MISSING: {old[:70]!r}")
        return s.replace(old, new, 1)
    return f

GONE = ["amplification", "dos", "flowtype", "intermediary", "lifecycle", "prefetch", "registry", "sequence", "settings"]
def unreached(s):
    for g in GONE:
        line = f"- `pkg/protocol/{g}` (G8)\n"
        if line not in s:
            sys.exit(f"ANCHOR MISSING: {line!r}")
        s = s.replace(line, "")
    anchor = "## The list\n"
    if anchor not in s:
        sys.exit("ANCHOR MISSING: ## The list")
    return s.replace(anchor, """## Applied 2026-11-04 (DP-715, S-WIRE-HONEST)

Deleted: `pkg/protocol/{amplification,dos,intermediary,prefetch,lifecycle,settings,flowtype,sequence,registry}`,
`tomb/lich/harnesses/lich_006_real_lifecycle_test.go` (LICH-006's harness; its only target went) and
`cmd/pqc-verifier` (TRUE-WIRE L5). The other TRUE-WIRE rows and who carries them are in
[sweeps/TRUE-WIRE-DECISIONS.md](sweeps/TRUE-WIRE-DECISIONS.md), "Applied 2026-11-04".

""" + anchor, 1)
edit("docs/UNREACHED-GO-PACKAGES.md", unreached)

edit("docs/sweeps/TRUE-WIRE-DECISIONS.md", lambda s: s.rstrip("\n") + """

## Applied 2026-11-04 (DP-715: "accept all 36 defaults")

The table has 35 rows, not 36: R1-R10, C1-C9, E1-E6, L1-L7, T1-T3. DP-203 (fold, nothing deleted) overrides C1-C8.

| Rows | Disposition | Carried by |
|---|---|---|
| R1, R2, R3, R5, R6, R7, R8, R9, R10 | deleted (R6 with `lich_006_real_lifecycle_test.go`) | S-WIRE-HONEST |
| R4 `pkg/protocol/migration` | stays parked: E8 answered (a), RETRY_TOKENS moved to hop-ebpf and carries no token on the wire; `ValidateToken` is a stub that must not be wired | S-WIRE-HONEST |
| C1-C6, C7, C8 | NOT deleted: DP-203 folds them behind flags | S-G9-FOLD |
| C9 `services/sabatons` | stays parked | none |
| E1-E6 | KEEP, nothing to do | none |
| L1 `pkg/afxdp` | DP-205, verify Doom first | S-CLEAN-CUT |
| L2, L3, L4 | DP-401 | S-CLEAN-CUT |
| L5 `cmd/pqc-verifier` | deleted; ADR-119 D1 revives it from history if that ADR is ever built; port 19008 stays reserved | S-WIRE-HONEST |
| L6 `cmd/shield` + `services/shield` | ASK row: no default to accept, and no answer in the pack; open, for Stevie | none |
| L7 `ebpf/pqc-common` | stays parked | none |
| T1 `pkg/waf`, T2 `lxd/containers`, T3 `docker/hosts` | DP-401, DP-218 | S-CLEAN-CUT |
""" + "\n")

edit("docs/protocol/G8-REFERENCE-RULINGS.md", lambda s: s.rstrip("\n") + """

Applied 2026-11-04 (DP-715, S-WIRE-HONEST): the E3 delete rows above and `lifecycle`, `settings` are gone. `migration` stays parked (RETRY_TOKENS moved to hop-ebpf without a wire token). `integrity`'s HMAC half stays unwired: the Monad has no tag field (`docs/protocol/RFC-EDITOR-REQUESTS-WH.md` item 7).
""")

def adr062(s):
    s, n = re.subn(r"^\| LICH-006 \| `pkg/protocol/lifecycle` .*$", "| LICH-006 | `pkg/protocol/lifecycle` (deleted 2026-11-04, DP-715) | RETIRED | harness deleted with its target |", s, count=1, flags=re.M)
    if n != 1:
        sys.exit("ANCHOR MISSING: ADR-062 LICH-006 row")
    s, n = re.subn(r"^> \| 006 state machine \| `pkg/protocol/lifecycle` \| \*\*DONE\*\* .*$", "> | 006 state machine | `pkg/protocol/lifecycle` | **RETIRED** 2026-11-04: target deleted (DP-715); it found no defects while it ran. |", s, count=1, flags=re.M)
    if n != 1:
        sys.exit("ANCHOR MISSING: ADR-062 006 row")
    return s
edit("docs/adr/ADR-062-fuzz-redteam-pentest-framework.md", adr062)

edit("docs/adr/ADR-119-shield-xdp-pqc-fast-path-design.md", rep("## Decision\n\nNone yet.", """## Note 2026-11-04 (DP-715, TRUE-WIRE L5)

`cmd/pqc-verifier` was deleted with the other TRUE-WIRE defaults. D1's "revive" now means restoring it from git (last commit carrying it: the parent of the S-WIRE-HONEST deletion). Port 19008 stays reserved in `pkg/ports`; `services/shield` stays until TRUE-WIRE L6 is answered.

## Decision

None yet."""))

edit("wiki/Microservices.md", rep("| `pqc-verifier` | Post-quantum signature verification service | [[PQC-Authentication]] |\n", ""))
edit("wiki/PQC-Authentication.md", lambda s: re.sub(r"^\| `cmd/pqc-verifier` \(`:19008`\) \|.*\n", "", s, count=1, flags=re.M))
edit("wiki/Port-Registry.md", rep("pqc-verifier 19008", "pqc-verifier 19008 (reserved, ADR-119)"))
edit("docs/adr/ADR-094-own-the-metrics-stack.md", lambda s: rep("demo-trace-injector, pqc-verifier and shield are", "demo-trace-injector, pqc-verifier (deleted 2026-11-04, DP-715) and shield are")(rep("  pqc-verifier, shield, sophia, unheaded-daemon,", "  pqc-verifier (deleted 2026-11-04, DP-715), shield, sophia, unheaded-daemon,")(s)))
edit("docs/architecture/PQC_ARCHITECTURE.md", rep("> **\"Quantum-resistant authentication from packet zero.\"**\n", "> **\"Quantum-resistant authentication from packet zero.\"**\n\n> Status 2026-11-04: `cmd/pqc-verifier` is deleted (DP-715, TRUE-WIRE L5); sections naming it describe the design ADR-119 parks. FN-DSA is withdrawn (DP-411).\n"))
print("truewire_ripple.py: all edits applied")
````

<!-- PAYLOAD withdrawn_test.go -->
````text
// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package pqc

import "testing"

// DP-411: FN-DSA left the registry; its id stays burned.
func TestAlgoWithdrawn03(t *testing.T) {
	if _, ok := Registry(AlgoID(0x03)); ok {
		t.Fatal("Registry(0x03) found an entry: FN-DSA was withdrawn, 0x03 must not resolve")
	}
	if got := AlgoID(0x03).String(); got != "unknown" {
		t.Fatalf("AlgoID(0x03).String() = %q, want unknown", got)
	}
	if info, ok := Registry(AlgoHQC); !ok || info.Available {
		t.Fatalf("HQC must stay reserved: ok=%v available=%v", ok, info.Available)
	}
	if info, ok := Registry(AlgoSLHDSA); !ok || !info.Available {
		t.Fatalf("SLH-DSA must be available: ok=%v available=%v", ok, info.Available)
	}
}
````

<!-- PAYLOAD fndsa.py -->
````text
#!/usr/bin/env python3
"""DP-411: drop FN-DSA from the registry. Run from the worktree root. Fails loudly if any anchor is missing."""
import re, sys
def sub(path, old, new, count=1):
    s = open(path).read()
    if old not in s:
        sys.exit(f"ANCHOR MISSING in {path}: {old[:60]!r}")
    open(path, "w").write(s.replace(old, new) if count == 0 else s.replace(old, new, count))
def rx(path, pat, new, flags=re.S | re.M):
    s = open(path).read()
    s2, n = re.subn(pat, new, s, flags=flags)
    if n == 0:
        sys.exit(f"PATTERN MISSING in {path}: {pat[:60]!r}")
    open(path, "w").write(s2)

R = "pkg/crypto/pqc/registry.go"
sub(R, """	// AlgoFNDSA identifies FN-DSA (FIPS 206) - Lattice-based signature.
	// Userspace-only; requires floating point operations and cannot run in
	// BPF context.
	AlgoFNDSA AlgoID = 0x03
""", """	// AlgoWithdrawn03 was FN-DSA (FIPS 206). Withdrawn 2026-11-04 (DP-411):
	// signing needs IEEE-754 floating point, so it can never run in BPF, and
	// no Go implementation exists. The value stays burned: never reassign it.
	// It is not in the registry; Registry(AlgoWithdrawn03) reports false.
	AlgoWithdrawn03 AlgoID = 0x03
""")
rx(R, r"^\tAlgoFNDSA: \{\n.*?^\t\},\n", "")
sub(R, "Available:  true, // circl v1.6.3 provides sign/slhdsa", "Available:  true, // circl provides sign/slhdsa")

P = "pkg/crypto/pqc/pqc.go"
sub(P, "//   - SLH-DSA (FIPS 205) - Hash-based digital signature (stub, pending circl support)\n//   - FN-DSA (FIPS 206) - Lattice signature (stub, requires floating point, no BPF)\n//   - HQC (FIPS 207) - Code-based KEM (stub, pending circl support)\n",
       "//   - SLH-DSA (FIPS 205) - Hash-based digital signature\n//   - HQC (FIPS 207) - Code-based KEM (reserved: circl has no HQC)\n//\n// FN-DSA (FIPS 206) was withdrawn 2026-11-04 (DP-411); see AlgoWithdrawn03.\n")
sub(P, "// yet available (e.g., SLH-DSA, FN-DSA, HQC stubs).", "// available (HQC, reserved until a Go implementation exists).")
sub("pkg/crypto/pqc/security.go", "switch to a different algorithm (e.g., FN-DSA) on the same flow", "switch to a different algorithm (e.g., SLH-DSA) on the same flow")

T = "pkg/crypto/pqc/pqc_test.go"
sub(T, '		{AlgoFNDSA, "FN-DSA", "FIPS 206", "signature", "lattice", false, false},\n', "")
sub(T, '		{AlgoFNDSA, "FN-DSA"},\n', "")
for fn in ("TestFNDSAStubReturnsNotAvailable", "TestFNDSAParamInfo", "TestFNDSANotBPFSafe"):
    rx(T, r"^func " + fn + r"\(t \*testing\.T\) \{\n.*?^\}\n\n?", "")
sub(T, "	var _ Signer = &FNDSASigner{}\n", "")
sub(T, "	var _ Verifier = &FNDSAVerifier{}\n", "")
sub(T, "// --- FN-DSA BPF Safety Tests ---\n\n", "")
sub(T, "if len(algos) != 5 {", "if len(algos) != 4 {")
sub(T, '\tfor i, algo := range algos {\n\t\texpectedID := AlgoID(i + 1)\n\t\tif algo.ID != expectedID {\n', '\twantIDs := []AlgoID{AlgoSLHDSA, AlgoMLDSA, AlgoMLKEM, AlgoHQC}\n\tfor i, algo := range algos {\n\t\texpectedID := wantIDs[i]\n\t\tif algo.ID != expectedID {\n')
sub(T, "AllAlgorithms() returned %d entries, want 5", "AllAlgorithms() returned %d entries, want 4")

ST = "pkg/crypto/pqc/security_test.go"
sub(ST, "uint8(AlgoFNDSA)", "uint8(AlgoWithdrawn03)", 0)
sub(ST, "	// FN-DSA (0x03) is a weaker choice (requires floating point, not BPF-safe).\n", "	// 0x03 is the withdrawn FN-DSA id: never a valid downgrade target.\n")
sub(ST, 'downgrade from ML-DSA to FN-DSA should be rejected', 'downgrade from ML-DSA to the withdrawn 0x03 should be rejected')

V = "pkg/policy/verification.go"
rx(V, r"^\tAlgoFNDSA  uint8 = 0x03\n", "")
sub(V, "AllowedAlgos: []uint8{AlgoSLHDSA, AlgoMLDSA, AlgoFNDSA},", "AllowedAlgos: []uint8{AlgoSLHDSA, AlgoMLDSA},")
sub(V, "AllowedAlgos:     []uint8{AlgoSLHDSA, AlgoMLDSA, AlgoFNDSA},", "AllowedAlgos:     []uint8{AlgoSLHDSA, AlgoMLDSA},")
sub(V, "MinMultiSigCount: 2, // 2-of-3", "MinMultiSigCount: 2, // both of SLH-DSA and ML-DSA: FN-DSA is withdrawn (DP-411)")
VT = "pkg/policy/verification_test.go"
sub(VT, """	if len(p.AllowedAlgos) != 3 {
		t.Fatalf("TierNone should allow 3 algos, got %d", len(p.AllowedAlgos))""", """	if len(p.AllowedAlgos) != 2 {
		t.Fatalf("TierNone should allow 2 algos, got %d", len(p.AllowedAlgos))""")
sub(VT, """	if len(p.AllowedAlgos) != 3 {
		t.Fatalf("TierSovereign should allow 3 algos, got %d", len(p.AllowedAlgos))""", """	if len(p.AllowedAlgos) != 2 {
		t.Fatalf("TierSovereign should allow 2 algos, got %d", len(p.AllowedAlgos))""")
sub(VT, "if p.IsAlgoAllowed(AlgoFNDSA) {", "if p.IsAlgoAllowed(0x03) {")
sub(VT, 'FN-DSA should NOT be allowed in TierStandard', 'the withdrawn id 0x03 should NOT be allowed in TierStandard')

N = "pkg/pqc_policy/nist_level.go"
rx(N, r"^\talgoFNDSA  uint8 = 0x03\n", "")
rx(N, r"^\tcase algoFNDSA:\n.*?^\tcase algoMLKEM:\n", "\tcase algoMLKEM:\n")
sub("pkg/pqc_policy/policy.go", "AllowedAlgos: []uint8{algoSLHDSA, algoMLDSA, algoFNDSA},", "AllowedAlgos: []uint8{algoSLHDSA, algoMLDSA},")
sub("pkg/sophia/pqc_maps.go", "// Third signature reference (e.g., FN-DSA)", "// Third signature slot: reserved since FN-DSA was withdrawn (DP-411)")

sub("services/shield/pqc_verify.go", "			0x03: true, // FN-DSA\n", "")
sub("services/shield/sovereign_verify.go", 'FamilyLattice   = "lattice"    // ML-DSA, FN-DSA', 'FamilyLattice   = "lattice"    // ML-DSA; the withdrawn 0x03 stays here so it can never count as a new family')
sub("services/sophia/sovereign_map.go", "case 0x02, 0x03: // ML-DSA, FN-DSA = lattice", "case 0x02, 0x03: // ML-DSA; withdrawn 0x03 stays lattice so it never counts as a new family")
sub("services/sophia/pqc_test.go", "// FN-DSA pending", "// withdrawn id, pending")
sub("services/sophia/pqc_test.go", "// FN-DSA: valid (same family!)", "// withdrawn id: valid (same family!)")

I = "tests/pqc/integration_test.go"
for name in ("keygen", "sign", "verify"):
    rx(I, r'^\tt\.Run\("FN-DSA/' + name + r'", func\(t \*testing\.T\) \{\n.*?^\t\}\)\n\n', "")
sub(I, "	case pqc.AlgoFNDSA:\n		v := &pqc.FNDSAVerifier{}\n		return v.Verify(message, signature, pubKey)\n\n", "")

L = "tomb/lich/harnesses/lich_012_pqc_fuzz_test.go"
s = open(L).read()
s2 = re.sub(r"(f\.Add\(uint8\(0x03\).*?)// FN-[^\n]*", r"\1// withdrawn id 0x03", s, count=1)
if s2 == s:
    sys.exit("PATTERN MISSING in lich_012")
open(L, "w").write(s2)

C = "ebpf/pqc-common/src/lib.rs"
sub(C, "    /// FIPS 206 - FN-DSA (Lattice signature, userspace-only)\n    FnDsa = 0x03,\n", "    // 0x03 was FN-DSA, withdrawn 2026-11-04 (DP-411). The value stays burned.\n")
sub(C, "        assert_eq!(AlgoId::FnDsa as u8, 0x03);\n", "")
print("fndsa.py: all edits applied")
````

<!-- PAYLOAD fndsa_docs.py -->
````text
#!/usr/bin/env python3
"""DP-411 doc ripple. Run from the worktree root."""
import sys
def rep(p, pairs):
    s = open(p).read()
    for old, new in pairs:
        if old not in s:
            sys.exit(f"ANCHOR MISSING in {p}: {old[:70]!r}")
        s = s.replace(old, new, 1)
    open(p, "w").write(s)
rep("wiki/PQC-Authentication.md", [("ML-DSA, ML-KEM, SLH-DSA, FN-DSA, HQC and a KEM tunnel on", "ML-DSA, ML-KEM, SLH-DSA, HQC (reserved) and a KEM tunnel on")])
rep("docs/architecture/PQC_ARCHITECTURE.md", [
 ("| `0x01` | SLH-DSA | FIPS 205 | Signature | Hash-based | Yes | Stub |", "| `0x01` | SLH-DSA | FIPS 205 | Signature | Hash-based | Yes | **Full** (circl) |"),
 ("| `0x03` | FN-DSA | FIPS 206 | Signature | Lattice | No (FPU) | Stub |", "| `0x03` | withdrawn (was FN-DSA) | none | none | none | No | Burned, never reassigned (DP-411) |"),
 ("| `0x05` | HQC | FIPS 207 | KEM | Code-based | Yes | Stub |", "| `0x05` | HQC | FIPS 207 | KEM | Code-based | Yes | Reserved (circl has no HQC) |"),
 ("| FN-DSA | Level 1 (FN-DSA-512) | Level 5 (FN-DSA-1024) | — |\n", ""),
 ("| Lattice | ML-DSA, FN-DSA, ML-KEM | Module-LWE/NTRU lattice hardness |", "| Lattice | ML-DSA, ML-KEM | Module-LWE lattice hardness |"),
 ("allowed_algos: [1, 2, 3]     # SLH-DSA, ML-DSA, FN-DSA", "allowed_algos: [1, 2]        # SLH-DSA, ML-DSA"),
])
print("fndsa_docs.py: all edits applied")
````

<!-- PAYLOAD adr105_bar.md -->
````text

## The bar and the gap (2026-11-04, DP-411)

**The bar (Stevie, 2026-11-03):** at or above iMessage PQ3, Signal PQXDH and SPQR, and the Microsoft and HKMA roadmaps: hybrid post-quantum key establishment everywhere, post-quantum signatures, phased.

What each one is, from its publisher, read 2026-11-04:

| Reference | What it does | Source |
|---|---|---|
| iMessage PQ3 | Hybrid start: Kyber-1024 with P-256 ECDH. Ongoing: a Kyber-768 ratchet about every 50 messages, at least every 7 days. Authentication stays classical (ECDSA P-256, key in the Secure Enclave). | security.apple.com/blog/imessage-pq3 |
| Signal PQXDH | Hybrid start: X25519 with Kyber-1024. Authentication stays classical. | signal.org/docs/specifications/pqxdh |
| Signal SPQR | An ML-KEM-768 ratchet beside the Double Ratchet, keys mixed through a KDF ("Triple Ratchet"), with graceful downgrade for older clients. | signal.org/blog/spqr |
| Microsoft Quantum Safe Program | Inventory first. Moved on 2026-06-30: critical products and services to PQC by 2029 (was early adoption 2029, transition 2033). | microsoft.com/en-us/security/blog, 2026-06-30 |
| HKMA Quantum Preparedness Index | First issue 2026-07-27: Awareness, Planning, Pilots, Practical Preparedness; sector score 2.3 of 10, target 10 by 2030. | hkma.gov.hk press release 20260727-3 |

Note: PQ3 and Signal authenticate classically, so "PQ signatures" is above those two and is Stevie's addition. The Microsoft and HKMA dates are after the 2026-11-03 bar was written; they are the current ones.

**The tree, measured on develop 2026-11-04:**

| # | Axis | Bar | Tree today | Verdict |
|---|---|---|---|---|
| PQ-1 | Inventory, gates | Microsoft and HKMA start here | CBOM `compliance/crypto/cbom.cdx.json`, `check-crypto-inventory.sh`, `check-pq-kex.sh`, a shrink-only baseline | **At or above** |
| PQ-2 | Signatures on config, drift, health, audit, evidence, seals | PQ3 and Signal: classical | ML-DSA-65: Wotan signed prefixes, Gungnir, evidence, cartridges, Binding Rune | **Above** PQ3 and Signal, in code; drift and evidence keys wait on custody (DP-06) |
| PQ-3 | Hybrid KEX on links we own, in code | Hybrid everywhere | `pkg/tls` Strict and Preferred, rustls aws-lc-rs | **At**, in code |
| PQ-4 | Hybrid KEX live | Everywhere | The stack is down; compose Wotan and edge Traefik run plaintext until E1 (DP-23) | **Below** until E1 |
| PQ-5 | The Well | Hybrid | PG 16: P-256 only | **Below**; PG 18 in its own window (DP-03) |
| PQ-6 | SSH | Hybrid | East hybrid-only; west accepts classical KEX | **Below** on west; attended (DP-04) |
| PQ-7 | WireGuard | Hybrid KEM | Classical Noise plus a PSK, rotated by hand once; weekly timer off. No KEM in the path; the PSK protects only while fresh and only if carried over a hybrid channel | **Below**; timer in DP-05; a real hybrid needs the third-party sitting |
| PQ-8 | Strength at establishment | PQ3 and PQXDH start at Kyber-1024 | Strict prefers X25519MLKEM768; SecP384r1MLKEM1024 is third | **Below** at session start for long-lived secrets |
| PQ-9 | Ongoing PQ rekey | PQ3: every 50 messages or 7 days. SPQR: continuous | **None.** TLS 1.3 KeyUpdate is symmetric; no `MaxConnectionAge` anywhere, so a Wotan stream keeps one KEX for its process lifetime. `pkg/crypto/pqc` KEM tunnel epochs are reached by no binary | **Below; the largest gap** |
| PQ-10 | Service authentication | "PQ signatures" | Ed25519 tokens; DP-201's switch rule has not fired. EdDSA OIDC, classical TLS certificates, GPG commits | **Below** the bar, at PQ3 and Signal |
| PQ-11 | Package and build signing | "PQ signatures" | Apt repo unsigned (DP-14); build key seed waits on custody (DP-06) | **Below** |
| PQ-12 | SLH-DSA | "PQ signatures" | Implemented on circl v1.6.5, all 12 parameter sets after S-WIRE-HONEST; **no production caller** | Available, not used |
| PQ-13 | Key storage | PQ3: Secure Enclave | 0400 files under dedicated accounts; YubiKey ceremony planned (DP-06) | **Below** for daemon keys |
| PQ-14 | Published analysis | Apple and Signal each published formal analysis | None | **Below** |
| PQ-15 | Third-party links | n/a | SMTP and KEV keep a TLS 1.2 floor; the far end decides | Accepted, hybrid-opportunistic |
| PQ-16 | Wire layer | n/a | Monad PQC is design only (ADR-119, parked) though the drafts describe it | Honest in the docs; not built |

**Summary.** Above the bar on signed state and inventory. At it, in code, for first-party TLS. Below it on coverage (PQ-4 to PQ-7), on start strength (PQ-8), on ongoing rekey (PQ-9) and on authentication of live sessions (PQ-10, PQ-11).

**Phased roadmap. Dates are a proposal for Stevie; only R1's contents are already decided.**

| Phase | When | Closes |
|---|---|---|
| R1 | E1 and the windows after it | PQ-4, PQ-5, PQ-6, PQ-7 timer, PQ-2 keys, PQ-11 apt key |
| R2 | 2027 H1 | PQ-9: a connection-age cap on Wotan gRPC and first-party streams, so each link re-runs a hybrid KEX on a cadence no worse than PQ3's 7 days; PQ-8: decide ML-KEM-1024 for Strict |
| R3 | 2027 H2 | PQ-10: dual-publish service tokens, then flip (DP-201 A); hybrid signatures on `audit.` and `alerts.critical`; SLH-DSA co-signature on the build key (PQ-12) |
| R4 | 2028 | PQ-10 certificates and SSH host keys as Go and OpenSSH ship them; PQ-7 hybrid tunnel; PQ-13 |
| R5 | by end of 2029 | Inventory shows no classical-only critical link (Microsoft's date); a self-score on the HKMA's four dimensions before 2030; PQ-14 |
````

<!-- PAYLOAD pinned_linux_test.go -->
````text
// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

//go:build linux

package ebpf

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"

	cebpf "github.com/cilium/ebpf"
)

func TestOpenPinnedMap_MissingPath(t *testing.T) {
	if _, err := OpenPinnedMap("/sys/fs/bpf/does-not-exist-wire-honest"); err == nil {
		t.Fatal("OpenPinnedMap on a missing pin returned nil error")
	}
}

func TestPinnedMap_KeyAndValueSizeRefused(t *testing.T) {
	m := &PinnedMap{fd: -1, keySize: 4, valueSize: 8}
	if _, err := m.Lookup([]byte{1, 2}); !errors.Is(err, ErrKeySize) {
		t.Fatalf("Lookup short key: %v, want ErrKeySize", err)
	}
	if _, err := m.Lookup(nil); !errors.Is(err, ErrKeySize) {
		t.Fatalf("Lookup nil key: %v, want ErrKeySize", err)
	}
	if err := m.Update([]byte{1, 2, 3, 4}, []byte{1}); !errors.Is(err, ErrValueSize) {
		t.Fatalf("Update short value: %v, want ErrValueSize", err)
	}
	if err := m.Delete([]byte{1}); !errors.Is(err, ErrKeySize) {
		t.Fatalf("Delete short key: %v, want ErrKeySize", err)
	}
	if err := m.Close(); err != nil {
		t.Fatalf("Close on fd -1: %v", err)
	}
}

func pinTestMap(t *testing.T, typ cebpf.MapType) string {
	t.Helper()
	if os.Geteuid() != 0 {
		t.Skip("needs root (CAP_BPF)")
	}
	cm, err := cebpf.NewMap(&cebpf.MapSpec{Type: typ, KeySize: 4, ValueSize: 8, MaxEntries: 8})
	if err != nil {
		t.Fatalf("create map: %v", err)
	}
	t.Cleanup(func() { _ = cm.Close() })
	dir, err := os.MkdirTemp("/sys/fs/bpf", "wire-honest-")
	if err != nil {
		t.Fatalf("mkdir in bpffs: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	path := filepath.Join(dir, "m")
	if err := cm.Pin(path); err != nil {
		t.Fatalf("pin: %v", err)
	}
	return path
}

func TestPinnedMap_RoundTrip(t *testing.T) {
	m, err := OpenPinnedMap(pinTestMap(t, cebpf.Hash))
	if err != nil {
		t.Fatalf("OpenPinnedMap: %v", err)
	}
	defer m.Close()
	if m.KeySize() != 4 || m.ValueSize() != 8 {
		t.Fatalf("sizes = %d/%d, want 4/8", m.KeySize(), m.ValueSize())
	}
	k := []byte{1, 0, 0, 0}
	v := []byte{9, 8, 7, 6, 5, 4, 3, 2}
	if got, err := m.Lookup(k); err != nil || got != nil {
		t.Fatalf("Lookup absent = %v, %v; want nil, nil", got, err)
	}
	if err := m.Update(k, v); err != nil {
		t.Fatalf("Update: %v", err)
	}
	if got, err := m.Lookup(k); err != nil || !bytes.Equal(got, v) {
		t.Fatalf("Lookup = %v, %v; want %v", got, err, v)
	}
	if err := m.Update([]byte{2, 0, 0, 0}, v); err != nil {
		t.Fatalf("Update 2: %v", err)
	}
	n := 0
	if err := m.Iterate(func(key, value []byte) error { n++; return nil }); err != nil {
		t.Fatalf("Iterate: %v", err)
	}
	if n != 2 {
		t.Fatalf("Iterate saw %d entries, want 2", n)
	}
	if err := m.Delete(k); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if err := m.Delete(k); err != nil {
		t.Fatalf("Delete absent: %v", err)
	}
}

func TestOpenPinnedMap_RefusesPerCPU(t *testing.T) {
	if _, err := OpenPinnedMap(pinTestMap(t, cebpf.PerCPUHash)); !errors.Is(err, ErrUnsupportedMapType) {
		t.Fatalf("OpenPinnedMap(percpu) = %v, want ErrUnsupportedMapType", err)
	}
}
````

<!-- PAYLOAD pinned_linux.go -->
````text
// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

//go:build linux

package ebpf

import (
	"errors"
	"fmt"
	"syscall"
	"unsafe"
)

// Map types a PinnedMap refuses: their values are not one ValueSize buffer
// (per-CPU maps return one value per CPU) or they have no key at all.
const (
	mapTypePerCPUHash    = 5
	mapTypePerCPUArray   = 6
	mapTypeLRUPerCPUHash = 10
	mapTypeRingbuf       = 27
)

// ErrKeySize is returned when a key's length is not the map's key size.
var ErrKeySize = errors.New("ebpf: key size mismatch")

// ErrValueSize is returned when a value's length is not the map's value size.
var ErrValueSize = errors.New("ebpf: value size mismatch")

// PinnedMap is an open handle on a map pinned in bpffs. It is the in-house
// counterpart of the loader's ReadMap for maps this process did not load
// (DP-211). Not safe for concurrent Close.
type PinnedMap struct {
	fd        int
	keySize   uint32
	valueSize uint32
}

// OpenPinnedMap opens the map pinned at path. The caller must Close it.
func OpenPinnedMap(path string) (*PinnedMap, error) {
	fd, err := bpfObjGet(path)
	if err != nil {
		return nil, err
	}
	info, err := bpfGetMapInfo(fd)
	if err != nil {
		_ = syscall.Close(fd)
		return nil, fmt.Errorf("map info %s: %w", path, err)
	}
	switch info.Type {
	case mapTypePerCPUHash, mapTypePerCPUArray, mapTypeLRUPerCPUHash, mapTypeRingbuf:
		_ = syscall.Close(fd)
		return nil, fmt.Errorf("%w: %s has type %d", ErrUnsupportedMapType, path, info.Type)
	}
	return &PinnedMap{fd: fd, keySize: info.KeySize, valueSize: info.ValueSize}, nil
}

// KeySize returns the map's key size in bytes.
func (m *PinnedMap) KeySize() uint32 { return m.keySize }

// ValueSize returns the map's value size in bytes.
func (m *PinnedMap) ValueSize() uint32 { return m.valueSize }

func (m *PinnedMap) checkKey(key []byte) error {
	if uint32(len(key)) != m.keySize || len(key) == 0 { // #nosec G115 -- a slice length fits uint32 for any key a map accepts
		return fmt.Errorf("%w: got %d, want %d", ErrKeySize, len(key), m.keySize)
	}
	return nil
}

// Lookup returns the value for key, or (nil, nil) when the key is absent.
func (m *PinnedMap) Lookup(key []byte) ([]byte, error) {
	if err := m.checkKey(key); err != nil {
		return nil, err
	}
	value := make([]byte, m.valueSize)
	err := bpfMapLookupElem(m.fd, unsafe.Pointer(&key[0]), unsafe.Pointer(&value[0])) // #nosec G103 -- BPF kernel ABI boundary; no uintptr->Pointer round-trip
	if errors.Is(err, syscall.ENOENT) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return value, nil
}

// Update writes key=value (create or replace).
func (m *PinnedMap) Update(key, value []byte) error {
	if err := m.checkKey(key); err != nil {
		return err
	}
	if uint32(len(value)) != m.valueSize || len(value) == 0 { // #nosec G115 -- a slice length fits uint32 for any value a map accepts
		return fmt.Errorf("%w: got %d, want %d", ErrValueSize, len(value), m.valueSize)
	}
	return bpfMapUpdateElem(m.fd, unsafe.Pointer(&key[0]), unsafe.Pointer(&value[0]), BPF_ANY) // #nosec G103 -- BPF kernel ABI boundary; no uintptr->Pointer round-trip
}

// Delete removes key; an absent key is not an error.
func (m *PinnedMap) Delete(key []byte) error {
	if err := m.checkKey(key); err != nil {
		return err
	}
	err := bpfMapDeleteElem(m.fd, unsafe.Pointer(&key[0])) // #nosec G103 -- BPF kernel ABI boundary; no uintptr->Pointer round-trip
	if errors.Is(err, syscall.ENOENT) {
		return nil
	}
	return err
}

// Iterate calls fn for every entry with copies of the key and value. A non-nil
// error from fn stops the walk and is returned. An entry deleted between the
// key walk and its lookup is skipped.
func (m *PinnedMap) Iterate(fn func(key, value []byte) error) error {
	key := make([]byte, m.keySize)
	next := make([]byte, m.keySize)
	var cur unsafe.Pointer // nil: first key
	for {
		err := bpfMapGetNextKey(m.fd, cur, unsafe.Pointer(&next[0])) // #nosec G103 -- BPF kernel ABI boundary; no uintptr->Pointer round-trip
		if errors.Is(err, syscall.ENOENT) {
			return nil
		}
		if err != nil {
			return err
		}
		copy(key, next)
		cur = unsafe.Pointer(&key[0]) // #nosec G103 -- BPF kernel ABI boundary; no uintptr->Pointer round-trip
		value, err := m.Lookup(key)
		if err != nil {
			return err
		}
		if value == nil {
			continue
		}
		k := make([]byte, len(key))
		copy(k, key)
		if err := fn(k, value); err != nil {
			return err
		}
	}
}

// Close releases the handle. Safe to call twice.
func (m *PinnedMap) Close() error {
	if m.fd < 0 {
		return nil
	}
	err := syscall.Close(m.fd)
	m.fd = -1
	return err
}
````

<!-- PAYLOAD pinned_stub.go -->
````text
// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

//go:build !linux

package ebpf

// PinnedMap is unavailable off Linux.
type PinnedMap struct{}

// OpenPinnedMap returns ErrNotSupported off Linux.
func OpenPinnedMap(string) (*PinnedMap, error) { return nil, ErrNotSupported }

// KeySize returns 0 off Linux.
func (*PinnedMap) KeySize() uint32 { return 0 }

// ValueSize returns 0 off Linux.
func (*PinnedMap) ValueSize() uint32 { return 0 }

// Lookup returns ErrNotSupported off Linux.
func (*PinnedMap) Lookup([]byte) ([]byte, error) { return nil, ErrNotSupported }

// Update returns ErrNotSupported off Linux.
func (*PinnedMap) Update(_, _ []byte) error { return ErrNotSupported }

// Delete returns ErrNotSupported off Linux.
func (*PinnedMap) Delete([]byte) error { return ErrNotSupported }

// Iterate returns ErrNotSupported off Linux.
func (*PinnedMap) Iterate(func(key, value []byte) error) error { return ErrNotSupported }

// Close returns nil off Linux.
func (*PinnedMap) Close() error { return nil }
````

<!-- PAYLOAD bpfread_test.go -->
````text
// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package main

import (
	"context"
	"encoding/binary"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"

	pb "unheaded/proto/unheaded/v1"
)

func sophiaKey(dict, value uint8) []byte {
	k := make([]byte, 2)
	binary.LittleEndian.PutUint16(k, uint16(dict)<<8|uint16(value))
	return k
}

func sophiaValue(name string) []byte {
	v := make([]byte, sophiaEntrySize)
	copy(v[8:], name)
	return v
}

// withBackend installs b as the BPF-mode backend for one test.
func withBackend(t *testing.T, b Backend) {
	t.Helper()
	old, oldMock := globalBackend, globalMockMode
	globalBackend, globalMockMode = b, false
	t.Cleanup(func() { globalBackend, globalMockMode = old, oldMock })
}

func TestSophiaBPF_ListAndGet(t *testing.T) {
	m := NewMockBackend()
	for _, e := range []struct {
		dict, val uint8
		name      string
	}{{1, 3, "gold"}, {1, 4, "silver"}, {3, 7, ""}} {
		if err := m.WriteMap(mapSophia, sophiaKey(e.dict, e.val), sophiaValue(e.name)); err != nil {
			t.Fatal(err)
		}
	}
	withBackend(t, m)
	s := &sophiaServer{}

	resp, err := s.listBPF(&pb.DictionaryListRequest{})
	if err != nil {
		t.Fatalf("listBPF: %v", err)
	}
	if resp.Total != 2 || len(resp.Dictionaries) != 2 {
		t.Fatalf("total=%d len=%d, want 2/2", resp.Total, len(resp.Dictionaries))
	}
	if resp.Dictionaries[0].Id != 1 || resp.Dictionaries[1].Id != 3 {
		t.Fatalf("ids %d,%d, want 1,3", resp.Dictionaries[0].Id, resp.Dictionaries[1].Id)
	}
	if got := resp.Dictionaries[0].Entries[0].Key; got != "gold" {
		t.Fatalf("entry key %q, want gold", got)
	}
	if got := resp.Dictionaries[1].Entries[0].Key; got != "7" {
		t.Fatalf("unnamed entry key %q, want 7", got)
	}

	got, err := s.getBPF(&pb.DictionaryGetRequest{DictionaryId: 3})
	if err != nil || !got.Found {
		t.Fatalf("getBPF(3) = %v, %v", got, err)
	}
	miss, err := s.getBPF(&pb.DictionaryGetRequest{DictionaryId: 9})
	if err != nil || miss.Found {
		t.Fatalf("getBPF(9) = %v, %v; want found=false", miss, err)
	}
}

func TestSophiaBPF_BadEntryRefused(t *testing.T) {
	m := NewMockBackend()
	if err := m.WriteMap(mapSophia, []byte{1}, sophiaValue("x")); err != nil {
		t.Fatal(err)
	}
	withBackend(t, m)
	if _, err := (&sophiaServer{}).listBPF(&pb.DictionaryListRequest{}); status.Code(err) != codes.Unavailable {
		t.Fatalf("listBPF on a 1-byte key = %v, want Unavailable", err)
	}
}

func TestFlowBPF_List(t *testing.T) {
	key := make([]byte, flowKeySize)
	copy(key[0:4], []byte{10, 0, 0, 1})
	copy(key[4:8], []byte{10, 0, 0, 2})
	val := make([]byte, flowStateSize)
	binary.LittleEndian.PutUint64(val[16:24], 111)
	binary.LittleEndian.PutUint64(val[24:32], 222)
	m := NewMockBackend()
	if err := m.WriteMap(mapFlows, key, val); err != nil {
		t.Fatal(err)
	}
	withBackend(t, m)
	resp, err := (&flowServer{}).listBPF()
	if err != nil {
		t.Fatalf("listBPF: %v", err)
	}
	if resp.Total != 1 {
		t.Fatalf("total = %d, want 1", resp.Total)
	}
	f := resp.Flows[0]
	if f.CreatedAt != 111 || f.LastPacketAt != 222 || f.SourceAddress[3] != 1 || f.DestinationAddress[3] != 2 {
		t.Fatalf("flow = %+v", f)
	}
}

// The calls with no map behind them stay Unimplemented, in so many words.
func TestBPFMode_NoMapStaysUnimplemented(t *testing.T) {
	withBackend(t, NewMockBackend())
	if _, err := (&flowServer{}).GetState(context.Background(), &pb.ReadRequest{FlowLabel: 1}); status.Code(err) != codes.Unimplemented {
		t.Fatalf("flow get = %v, want Unimplemented", err)
	}
	if _, err := (&flowServer{}).List(context.Background(), &emptypb.Empty{}); err != nil {
		t.Fatalf("flow list on an empty map: %v", err)
	}
}
````

<!-- PAYLOAD backend_root_test.go -->
````text
// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

//go:build linux

package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	cebpf "github.com/cilium/ebpf"
)

// A real pinned map through BPFBackend (needs root; run the compiled test
// binary under sudo).
func TestBPFBackend_RealPinnedMap(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("needs root (CAP_BPF)")
	}
	dir, err := os.MkdirTemp("/sys/fs/bpf", "wire-honest-api-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	cm, err := cebpf.NewMap(&cebpf.MapSpec{Type: cebpf.Hash, KeySize: 2, ValueSize: 32, MaxEntries: 4})
	if err != nil {
		t.Fatal(err)
	}
	defer cm.Close()
	if err := os.MkdirAll(filepath.Join(dir, "hop-ebpf"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := cm.Pin(filepath.Join(dir, mapSophia)); err != nil {
		t.Fatal(err)
	}
	b := NewBPFBackend(dir)
	key, val := []byte{3, 1}, bytes.Repeat([]byte{7}, 32)
	if got, err := b.ReadMap(mapSophia, key); err != nil || got != nil {
		t.Fatalf("ReadMap absent = %v, %v; want nil, nil", got, err)
	}
	if err := b.WriteMap(mapSophia, key, val); err != nil {
		t.Fatal(err)
	}
	got, err := b.ReadMap(mapSophia, key)
	if err != nil || !bytes.Equal(got, val) {
		t.Fatalf("ReadMap = %v, %v", got, err)
	}
	n := 0
	if err := b.IterateMap(mapSophia, func(k, v []byte) error { n++; return nil }); err != nil || n != 1 {
		t.Fatalf("IterateMap n=%d err=%v", n, err)
	}
	if err := b.DeleteMap(mapSophia, key); err != nil {
		t.Fatal(err)
	}
}
````

<!-- PAYLOAD bpfread.go -->
````text
// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"sort"

	pb "unheaded/proto/unheaded/v1"
)

// Maps --mode=bpf reads, relative to --bpf-maps. cmd/ebpf-loader --pin-maps
// pins a program's maps at <base>/<program>/<MAP>.
const (
	mapSophia = "hop-ebpf/SOPHIA"    // HashMap<u16, SophiaEntry>
	mapFlows  = "flow-tracker/FLOWS" // LruHashMap<FlowKey, FlowState>
)

const (
	sophiaKeySize   = 2
	sophiaEntrySize = 32 // monad_common::SophiaEntry
	flowKeySize     = 16 // bpfschema.FlowKeySize
	flowStateSize   = 72 // bpfschema.FlowStateSize
)

// sophiaDictionaries groups the SOPHIA map by dictionary. The u16 key is
// (dict_id << 8) | field_value in host order (hop-ebpf sophia_key); the entry
// name is the 24-byte NUL-terminated field at offset 8.
func sophiaDictionaries(b Backend) (map[uint32]*pb.Dictionary, error) {
	dicts := map[uint32]*pb.Dictionary{}
	err := b.IterateMap(mapSophia, func(key, value []byte) error {
		if len(key) != sophiaKeySize || len(value) != sophiaEntrySize {
			return fmt.Errorf("SOPHIA entry sizes %d/%d, want %d/%d", len(key), len(value), sophiaKeySize, sophiaEntrySize)
		}
		k := binary.LittleEndian.Uint16(key)
		id := uint32(k >> 8)
		d, ok := dicts[id]
		if !ok {
			d = &pb.Dictionary{Id: id, Name: fmt.Sprintf("dict-%d", id)}
			dicts[id] = d
		}
		name, _, _ := bytes.Cut(value[8:32], []byte{0})
		entry := &pb.DictionaryEntry{Key: fmt.Sprintf("%d", k&0xFF), Value: append([]byte(nil), value...)}
		if len(name) > 0 {
			entry.Key = string(name)
		}
		d.Entries = append(d.Entries, entry)
		return nil
	})
	if err != nil {
		return nil, err
	}
	for _, d := range dicts {
		sort.Slice(d.Entries, func(i, j int) bool { return d.Entries[i].Key < d.Entries[j].Key })
	}
	return dicts, nil
}

// flowsFromMap reads FLOWS. The map is keyed by an IPv4 5-tuple and holds no
// flow label, so FlowLabel stays 0; created_at and last_packet_at are
// bpf_ktime_get_ns stamps, not wall clock.
func flowsFromMap(b Backend) ([]*pb.FlowState, error) {
	var flows []*pb.FlowState
	err := b.IterateMap(mapFlows, func(key, value []byte) error {
		if len(key) != flowKeySize || len(value) != flowStateSize {
			return fmt.Errorf("FLOWS entry sizes %d/%d, want %d/%d", len(key), len(value), flowKeySize, flowStateSize)
		}
		flows = append(flows, &pb.FlowState{
			SourceAddress:      append([]byte(nil), key[0:4]...),
			DestinationAddress: append([]byte(nil), key[4:8]...),
			CreatedAt:          binary.LittleEndian.Uint64(value[16:24]),
			LastPacketAt:       binary.LittleEndian.Uint64(value[24:32]),
		})
		return nil
	})
	if err != nil {
		return nil, err
	}
	return flows, nil
}
````

<!-- PAYLOAD protoapi.py -->
````text
#!/usr/bin/env python3
"""DP-211: protocol-api --mode=bpf on pkg/ebpf. Run from the worktree root."""
import sys
def must(s, old, path):
    if old not in s:
        sys.exit(f"ANCHOR MISSING in {path}: {old[:70]!r}")
def between(s, a, b, new, path):
    must(s, a, path); must(s, b, path)
    i = s.index(a); j = s.index(b, i)
    return s[:i] + new + s[j:]
def rep(s, old, new, path):
    must(s, old, path)
    return s.replace(old, new, 1)

p = "cmd/protocol-api/backend.go"; s = open(p).read()
s = rep(s, '''	"github.com/cilium/ebpf"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)
''', '''	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"unheaded/pkg/ebpf"
)
''', p)
s = between(s, "// BPFBackend — stub for real pinned BPF maps (bare-metal only)",
 "// ---------------------------------------------------------------------------\n// Global backend instance",
'''// BPFBackend — pinned BPF maps through the in-house pkg/ebpf (DP-211)
// ---------------------------------------------------------------------------

// BPFBackend reads and writes maps pinned under PinPath with pkg/ebpf. A map
// name is relative to PinPath: cmd/ebpf-loader --pin-maps pins each program's
// maps at <base>/<program>/<MAP>, so names read "hop-ebpf/SOPHIA".
type BPFBackend struct {
	PinPath string // e.g. /sys/fs/bpf/unheaded
}

// NewBPFBackend creates a BPFBackend targeting the given pin path.
func NewBPFBackend(pinPath string) *BPFBackend {
	return &BPFBackend{PinPath: pinPath}
}

func (b *BPFBackend) Mode() string { return "bpf" }

func (b *BPFBackend) open(mapName string) (*ebpf.PinnedMap, error) {
	m, err := ebpf.OpenPinnedMap(filepath.Join(b.PinPath, mapName))
	if err != nil {
		return nil, fmt.Errorf("open pinned map %s: %w", mapName, err)
	}
	return m, nil
}

// ReadMap returns (nil, nil) for an absent key, as the Backend contract says.
func (b *BPFBackend) ReadMap(mapName string, key []byte) ([]byte, error) {
	m, err := b.open(mapName)
	if err != nil {
		return nil, err
	}
	defer m.Close()
	value, err := m.Lookup(key)
	if err != nil {
		return nil, fmt.Errorf("lookup key in %s: %w", mapName, err)
	}
	return value, nil
}

func (b *BPFBackend) WriteMap(mapName string, key, value []byte) error {
	m, err := b.open(mapName)
	if err != nil {
		return err
	}
	defer m.Close()
	if err := m.Update(key, value); err != nil {
		return fmt.Errorf("update key in %s: %w", mapName, err)
	}
	return nil
}

func (b *BPFBackend) DeleteMap(mapName string, key []byte) error {
	m, err := b.open(mapName)
	if err != nil {
		return err
	}
	defer m.Close()
	if err := m.Delete(key); err != nil {
		return fmt.Errorf("delete key in %s: %w", mapName, err)
	}
	return nil
}

func (b *BPFBackend) IterateMap(mapName string, fn func(key, value []byte) error) error {
	m, err := b.open(mapName)
	if err != nil {
		return err
	}
	defer m.Close()
	if err := m.Iterate(fn); err != nil {
		return fmt.Errorf("iterate %s: %w", mapName, err)
	}
	return nil
}

''', p)
open(p, "w").write(s)

p = "cmd/protocol-api/sophia.go"; s = open(p).read()
s = rep(s, '''	"fmt"
	"sync"
	"time"
''', '''	"fmt"
	"sort"
	"sync"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
''', p)
s = between(s, "// listBPF reads dictionaries from BPF pinned maps", "// Get retrieves a specific dictionary by ID",
'''// listBPF lists the dictionaries in hop-ebpf's pinned SOPHIA map, by id.
func (s *sophiaServer) listBPF(req *pb.DictionaryListRequest) (*pb.DictionaryListResponse, error) {
	dicts, err := sophiaDictionaries(globalBackend)
	if err != nil {
		return nil, status.Errorf(codes.Unavailable, "sophia list: %v", err)
	}
	all := make([]*pb.Dictionary, 0, len(dicts))
	for _, d := range dicts {
		all = append(all, d)
	}
	sort.Slice(all, func(i, j int) bool { return all[i].Id < all[j].Id })

	limit, offset := int(req.Limit), int(req.Offset)
	if limit <= 0 {
		limit = 100
	}
	if offset < 0 || offset > len(all) {
		offset = len(all)
	}
	end := min(offset+limit, len(all))
	return &pb.DictionaryListResponse{
		Dictionaries: all[offset:end],
		Total:        int32(len(all)), // #nosec G115 -- at most 256 dictionary ids
	}, nil
}

''', p)
s = between(s, "// getBPF retrieves a dictionary from BPF pinned maps", "// UpdateDictionary updates an entry",
'''// getBPF returns one dictionary from hop-ebpf's pinned SOPHIA map.
func (s *sophiaServer) getBPF(req *pb.DictionaryGetRequest) (*pb.DictionaryGetResponse, error) {
	dicts, err := sophiaDictionaries(globalBackend)
	if err != nil {
		return nil, status.Errorf(codes.Unavailable, "sophia get: %v", err)
	}
	d, ok := dicts[req.DictionaryId]
	if !ok {
		return &pb.DictionaryGetResponse{
			Found:        false,
			ErrorMessage: fmt.Sprintf("Dictionary %d not found", req.DictionaryId),
		}, nil
	}
	return &pb.DictionaryGetResponse{Dictionary: d, Found: true}, nil
}

''', p)
open(p, "w").write(s)

p = "cmd/protocol-api/flow.go"; s = open(p).read()
s = rep(s, '''	"google.golang.org/protobuf/types/known/emptypb"
''', '''	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
''', p)
s = between(s, "// listBPF returns flows from BPF maps", "// GetState retrieves the state of a specific flow",
'''// listBPF lists flows from flow-tracker's pinned FLOWS map.
func (s *flowServer) listBPF() (*pb.FlowListResponse, error) {
	flows, err := flowsFromMap(globalBackend)
	if err != nil {
		return nil, status.Errorf(codes.Unavailable, "flow list: %v", err)
	}
	return &pb.FlowListResponse{
		Flows: flows,
		Total: int32(len(flows)), // #nosec G115 -- bounded by the length of an already-allocated slice
	}, nil
}

''', p)
s = rep(s, '''	// In a real implementation, this would:
	// 1. Create or lookup the flow in BPF maps
	// 2. Queue the packet for transmission
	// 3. Return packet ID and injection timestamp
	return nil, errBPFNotImplemented("flow inject")''', '''	// No map queues packets for transmission; injecting is not a map read.
	return nil, errBPFNotImplemented("flow inject")''', p)
s = rep(s, '''	if !globalMockMode {
		return nil, errBPFNotImplemented("flow get")
	}''', '''	if !globalMockMode {
		// FLOWS is keyed by 5-tuple and holds no flow label to look up.
		return nil, errBPFNotImplemented("flow get")
	}''', p)
open(p, "w").write(s)

p = "cmd/protocol-api/wotan.go"; s = open(p).read()
s = rep(s, '''	// In a real implementation, this would:
	// 1. Open the pinned BPF map at /sys/fs/bpf/unheaded/wotan/[flow_label]
	// 2. Read the 40-byte value
	// 3. Parse into MemorySlot proto
	// 4. Return with found=true/false
	return nil, errBPFNotImplemented("wotan read")''', '''	// No eBPF program declares a per-flow memory-slot map, so there is
	// nothing pinned to read.
	return nil, errBPFNotImplemented("wotan read")''', p)
s = rep(s, '''	// In a real implementation, this would:
	// 1. Validate state_data length (32 bytes)
	// 2. Pack into 40-byte structure (32 state + 8 sequence)
	// 3. Open or create pinned BPF map at /sys/fs/bpf/unheaded/wotan/[flow_label]
	// 4. Write with atomic guarantees
	// 5. Return written_at timestamp
	return nil, errBPFNotImplemented("wotan write")''', '''	// No eBPF program declares a per-flow memory-slot map, and DP-211 covers
	// map reads only.
	return nil, errBPFNotImplemented("wotan write")''', p)
open(p, "w").write(s)

p = "cmd/protocol-api/anamnesis.go"; s = open(p).read()
s = rep(s, '''	// In a real implementation, this would:
	// 1. Open the eBPF perf/ring buffer at /sys/kernel/debug/tracing/trace_pipe
	// 2. Parse protobuf-encoded events
	// 3. Apply filters (time range, flow labels, event types)
	// 4. Return paginated results
	return nil, errBPFNotImplemented("anamnesis query")''', '''	// A ring buffer has one consumer position: reading ANAMNESIS here would
	// take events away from trace-collector, and a ring cannot be queried.
	return nil, errBPFNotImplemented("anamnesis query")''', p)
s = rep(s, '''	// In a real implementation, this would:
	// 1. Attach to eBPF ring buffer
	// 2. Read protobuf-encoded events as they arrive
	// 3. Apply filters
	// 4. Send via gRPC stream
	return errBPFNotImplemented("anamnesis stream")''', '''	// A ring buffer has one consumer position: streaming ANAMNESIS here
	// would take events away from trace-collector.
	return errBPFNotImplemented("anamnesis stream")''', p)
open(p, "w").write(s)
p = "cmd/protocol-api/handlers_extended_test.go"; s = open(p).read()
s = rep(s, """// BPF mode: not implemented, and says so (it used to serve the mock data)
""", """// BPF mode: what has no map behind it says so (it used to serve the mock data)
""", p)
s = rep(s, """		"sophia list": func() error { _, err := ss.List(ctx, &pb.DictionaryListRequest{Limit: 100}); return err },
		"sophia get":  func() error { _, err := ss.Get(ctx, &pb.DictionaryGetRequest{DictionaryId: 1}); return err },
		"flow list":   func() error { _, err := fs.List(ctx, nil); return err },
		"flow get":    func() error { _, err := fs.GetState(ctx, &pb.ReadRequest{FlowLabel: 42}); return err },
""", """		"flow get": func() error { _, err := fs.GetState(ctx, &pb.ReadRequest{FlowLabel: 42}); return err },
""", p)
s = rep(s, """	ss := &sophiaServer{mockDictionaries: initializeMockDictionaries()}
	fs := &flowServer{mockFlows: make(map[uint32]*pb.FlowState)}
	as := &anamnesisServer{mockEventLog: make([]*pb.Event, 0), subscribers: make([]chan *pb.Event, 0)}

	calls := map[string]func() error{
		"wotan read":""", """	fs := &flowServer{mockFlows: make(map[uint32]*pb.FlowState)}
	as := &anamnesisServer{mockEventLog: make([]*pb.Event, 0), subscribers: make([]chan *pb.Event, 0)}

	calls := map[string]func() error{
		"wotan read":""", p)
open(p, "w").write(s)
print("protoapi.py: all edits applied")
````

<!-- PAYLOAD latency_rust.py -->
````text
#!/usr/bin/env python3
"""DP-408 Rust half. Run from the worktree root."""
import sys
def rep(p, old, new):
    s = open(p).read()
    if old not in s:
        sys.exit(f"ANCHOR MISSING in {p}: {old[:70]!r}")
    open(p, "w").write(s.replace(old, new, 1))
C = "ebpf/latency-probe/src/common.rs"
rep(C, '''//! These types define the LATENCY_MAP layout: a per-flow RTT statistics
//! map keyed by 5-tuple (LatencyKey) with aggregated min/max/avg RTT
//! values (LatencyEntry). Go userspace mirrors these byte-for-byte.
''', '''//! These types define the LATENCY_MAP layout: per-thread RTT statistics
//! keyed by trace id, pid and tid (LatencyTraceKey) with aggregated
//! min/max/avg RTT values (LatencyEntry). A kprobe on tcp_sendmsg has no
//! 5-tuple to key on. pkg/protocol/bpfschema mirrors these byte-for-byte.
''')
rep(C, '''/// Key for LATENCY_MAP — identifies a flow by 5-tuple.
///
/// IPs are stored as IPv6 (IPv4 addresses use ::ffff:a.b.c.d mapping).
/// Size: 16 + 16 + 2 + 2 + 1 + 3 = 40 bytes.
#[repr(C)]
#[derive(Clone, Copy, Default, Debug, PartialEq, Eq)]
pub struct LatencyKey {
    pub src_ip: [u8; 16],
    pub dst_ip: [u8; 16],
    pub src_port: u16,
    pub dst_port: u16,
    pub protocol: u8,
    pub _pad: [u8; 3],
}
''', '''/// Key for LATENCY_MAP — one thread's samples under one trace id.
///
/// trace_id is the socket's TraceId in big-endian bytes (zero when no socket
/// trace is associated); pid and tid are the full kernel ids.
/// Size: 16 + 4 + 4 = 24 bytes.
#[repr(C)]
#[derive(Clone, Copy, Default, Debug, PartialEq, Eq)]
pub struct LatencyTraceKey {
    pub trace_id: [u8; 16],
    pub pid: u32,
    pub tid: u32,
}
''')
rep(C, '''/// Size of LatencyKey in bytes.
#[allow(dead_code)]
pub const LATENCY_KEY_SIZE: usize = 40;
''', '''/// Size of LatencyTraceKey in bytes.
#[allow(dead_code)]
pub const LATENCY_TRACE_KEY_SIZE: usize = 24;
''')
rep(C, '''    fn test_latency_key_size() {
        assert_eq!(
            core::mem::size_of::<LatencyKey>(),
            LATENCY_KEY_SIZE,
            "LatencyKey size mismatch"
        );
    }
''', '''    fn test_latency_key_size() {
        assert_eq!(
            core::mem::size_of::<LatencyTraceKey>(),
            LATENCY_TRACE_KEY_SIZE,
            "LatencyTraceKey size mismatch"
        );
    }
''')
rep(C, '''        let key = LatencyKey::default();
        assert_eq!(key.src_ip, [0u8; 16]);
        assert_eq!(key.dst_ip, [0u8; 16]);
        assert_eq!(key.src_port, 0);
        assert_eq!(key.dst_port, 0);
        assert_eq!(key.protocol, 0);
''', '''        let key = LatencyTraceKey::default();
        assert_eq!(key.trace_id, [0u8; 16]);
        assert_eq!(key.pid, 0);
        assert_eq!(key.tid, 0);
''')
M = "ebpf/latency-probe/src/main.rs"
rep(M, "use common::{LatencyEntry, LatencyKey, MAX_LATENCY_MAP_ENTRIES};", "use common::{LatencyEntry, LatencyTraceKey, MAX_LATENCY_MAP_ENTRIES};")
rep(M, '''/// Per-flow RTT statistics map.
/// Key: LatencyKey (5-tuple), Value: LatencyEntry (min/max/latest/samples).
''', '''/// Per-thread RTT statistics map.
/// Key: LatencyTraceKey (trace id, pid, tid), Value: LatencyEntry (min/max/latest/samples).
''')
rep(M, "static LATENCY_MAP: HashMap<LatencyKey, LatencyEntry> =", "static LATENCY_MAP: HashMap<LatencyTraceKey, LatencyEntry> =")
rep(M, '''    // Update per-flow RTT statistics in LATENCY_MAP.
    // We use trace_id bytes as a proxy for the 5-tuple key since the kprobe
    // context does not directly provide IP/port info. The key is constructed
    // from trace_id high/low split into src_ip/dst_ip placeholders, with
    // pid as src_port and tid as dst_port, protocol=6 (TCP).
    let map_key = LatencyKey {
        src_ip: trace_id.to_bytes(),
        dst_ip: [0u8; 16],
        src_port: pid as u16,
        dst_port: tid as u16,
        protocol: 6, // TCP
        _pad: [0; 3],
    };
''', '''    // Update per-thread RTT statistics in LATENCY_MAP.
    let map_key = LatencyTraceKey {
        trace_id: trace_id.to_bytes(),
        pid,
        tid,
    };
''')
print("latency_rust.py: all edits applied")
````

<!-- PAYLOAD latency_go.py -->
````text
#!/usr/bin/env python3
"""DP-408 Go half: LATENCY_MAP keyed by trace id, pid, tid. Run from the worktree root."""
import re, sys
def between(s, a, b, new, p):
    if a not in s or b not in s:
        sys.exit(f"ANCHOR MISSING in {p}: {a[:60]!r} / {b[:60]!r}")
    i = s.index(a); j = s.index(b, i)
    return s[:i] + new + s[j:]
def rep(s, old, new, p):
    if old not in s:
        sys.exit(f"ANCHOR MISSING in {p}: {old[:70]!r}")
    return s.replace(old, new, 1)
R = "cmd/trace-collector-go/latency_reader.go"; s = open(R).read()
s = rep(s, """// the latency-probe kprobe program. Each entry contains per-flow RTT
// statistics (min/max/latest/samples) keyed by a 5-tuple (LatencyKey).
""", """// the latency-probe kprobe program. Each entry contains one thread's RTT
// statistics (min/max/latest/samples) keyed by trace id, pid and tid
// (LatencyTraceKey; a kprobe on tcp_sendmsg has no 5-tuple to key on).
""", R)
s = rep(s, "//   - Decodes LatencyKey/LatencyEntry from raw BPF map bytes", "//   - Decodes LatencyTraceKey/LatencyEntry from raw BPF map bytes", R)
s = between(s, "// ── LatencyKey ", "// ── LatencyEntry ", '''// ── LatencyTraceKey ────────────────────────────────────────────────────────

// LatencyTraceKey mirrors the Rust LatencyTraceKey struct from
// latency-probe/src/common.rs. Size: 16 + 4 + 4 = 24 bytes.
type LatencyTraceKey struct {
	TraceID [16]byte // Socket trace id, big-endian high then low; zero when none
	PID     uint32   // Full kernel pid (tgid)
	TID     uint32   // Full kernel tid
}

// LatencyTraceKeySize is the wire size of a LatencyTraceKey.
const LatencyTraceKeySize = bpfschema.LatencyTraceKeySize

// DecodeLatencyTraceKey decodes a key. Anything but exactly 24 bytes is
// refused: the old 40-byte 5-tuple key must fail loudly, not misparse.
func DecodeLatencyTraceKey(b []byte) (*LatencyTraceKey, error) {
	if len(b) != LatencyTraceKeySize {
		return nil, fmt.Errorf("latency key is %d bytes, want %d", len(b), LatencyTraceKeySize)
	}
	lk := &LatencyTraceKey{}
	copy(lk.TraceID[:], b[0:16])
	lk.PID = binary.LittleEndian.Uint32(b[16:20])
	lk.TID = binary.LittleEndian.Uint32(b[20:24])
	return lk, nil
}

// Encode serializes a LatencyTraceKey to a byte slice.
func (lk *LatencyTraceKey) Encode() [LatencyTraceKeySize]byte {
	var b [LatencyTraceKeySize]byte
	copy(b[0:16], lk.TraceID[:])
	binary.LittleEndian.PutUint32(b[16:20], lk.PID)
	binary.LittleEndian.PutUint32(b[20:24], lk.TID)
	return b
}

// String returns a human-readable representation.
func (lk *LatencyTraceKey) String() string {
	return fmt.Sprintf("trace=%x pid=%d tid=%d", lk.TraceID, lk.PID, lk.TID)
}

// traceHalves returns the trace id as the high and low halves the dashboard's
// LatencyEvent carries.
func (lk *LatencyTraceKey) traceHalves() map[string]uint64 {
	return map[string]uint64{
		"high": binary.BigEndian.Uint64(lk.TraceID[0:8]),
		"low":  binary.BigEndian.Uint64(lk.TraceID[8:16]),
	}
}

// latencyMapEvent renders one LATENCY_MAP entry as the dashboard's
// LatencyEvent: the entry's latest sample as latency_ns, stamped with
// wall-clock now (the dashboard's latency windows are wall-clock). There is
// no flow_key: the map holds no addresses.
func latencyMapEvent(lk *LatencyTraceKey, le *LatencyEntry, now time.Time) map[string]interface{} {
	return map[string]interface{}{
		"timestamp_ns": uint64(now.UnixNano()), // #nosec G115 -- wall clock after 1970
		"latency_ns":   le.RTTNS,
		"operation":    "tcp_send",
		"pid":          lk.PID,
		"tid":          lk.TID,
		"trace_id":     lk.traceHalves(),
		"min_rtt_ns":   le.MinRTTNS,
		"max_rtt_ns":   le.MaxRTTNS,
		"samples":      le.Samples,
	}
}

''', R)
s = rep(s, '''	"net"
''', '', R)
s = rep(s, '''	"unheaded/pkg/metrics/prom"
''', '''	"unheaded/pkg/metrics/prom"
	"unheaded/pkg/protocol/bpfschema"
''', R)
s = rep(s, "lk, err := DecodeLatencyKey(key)", "lk, err := DecodeLatencyTraceKey(key)", R)
s = rep(s, '''		SrcIP       string  `json:"src_ip"`
		DstIP       string  `json:"dst_ip"`
		SrcPort     uint16  `json:"src_port"`
		DstPort     uint16  `json:"dst_port"`
		Protocol    string  `json:"protocol"`
		RTTNS       uint64  `json:"rtt_ns"`
''', '''		TraceID     string  `json:"trace_id"`
		PID         uint32  `json:"pid"`
		TID         uint32  `json:"tid"`
		RTTNS       uint64  `json:"rtt_ns"`
''', R)
s = rep(s, '''			SrcIP:       s.Key.SrcIPAddr().String(),
			DstIP:       s.Key.DstIPAddr().String(),
			SrcPort:     s.Key.SrcPort,
			DstPort:     s.Key.DstPort,
			Protocol:    s.Key.ProtocolName(),
''', '''			TraceID:     fmt.Sprintf("%x", s.Key.TraceID),
			PID:         s.Key.PID,
			TID:         s.Key.TID,
''', R)
s, n = re.subn(r"\*LatencyKey\b", "*LatencyTraceKey", s)
open(R, "w").write(s)

T = "cmd/trace-collector-go/latency_reader_test.go"; s = open(T).read()
s = between(s, "func makeLatencyKey(", "// ── Helper: create a latency entry", '''func makeLatencyKey(pid, tid uint32) *LatencyTraceKey {
	return &LatencyTraceKey{PID: pid, TID: tid}
}

''', T)
s = between(s, "// ── LatencyKey encode/decode roundtrip", "// ── LatencyEntry encode/decode roundtrip", '''// ── LatencyTraceKey encode/decode roundtrip ─────────────────────────────────

func TestLatencyTraceKey_EncodeDecodeRoundtrip(t *testing.T) {
	original := &LatencyTraceKey{PID: 4194303, TID: 4194301}
	original.TraceID[0], original.TraceID[15] = 0xAB, 0xCD

	encoded := original.Encode()
	decoded, err := DecodeLatencyTraceKey(encoded[:])
	if err != nil {
		t.Fatalf("DecodeLatencyTraceKey: %v", err)
	}
	if *decoded != *original {
		t.Errorf("decoded = %+v, want %+v", decoded, original)
	}
}

func TestDecodeLatencyTraceKey_WrongSizeRefused(t *testing.T) {
	for _, n := range []int{0, 10, 23, 25, 40} {
		if _, err := DecodeLatencyTraceKey(make([]byte, n)); err == nil {
			t.Errorf("DecodeLatencyTraceKey accepted %d bytes", n)
		}
	}
}

func TestLatencyTraceKey_String(t *testing.T) {
	if s := makeLatencyKey(7, 8).String(); !strings.Contains(s, "pid=7 tid=8") {
		t.Errorf("String() = %q", s)
	}
}

''', T)
s = re.sub(r'makeLatencyKey\(\n\s*net\.ParseIP\("[^"]*"\),\n\s*net\.ParseIP\("[^"]*"\),\n\s*([^,\n]+), ([^,\n]+), \d+,\n\s*\)', r'makeLatencyKey(uint32(\1), uint32(\2))', s)
s = rep(s, '''	"net"
	"testing"
''', '''	"strings"
	"testing"
''', T)
s = rep(s, '''		if sample.Key.SrcPort != 12345 {
			t.Errorf("sample Key.SrcPort = %d, want 12345", sample.Key.SrcPort)
''', '''		if sample.Key.PID != 12345 {
			t.Errorf("sample Key.PID = %d, want 12345", sample.Key.PID)
''', T)
s = rep(s, '''		if sample.Key.DstPort != 80 {
			t.Errorf("sample Key.DstPort = %d, want 80", sample.Key.DstPort)
''', '''		if sample.Key.TID != 80 {
			t.Errorf("sample Key.TID = %d, want 80", sample.Key.TID)
''', T)
s = s.replace("LatencyKeySize", "LatencyTraceKeySize")
s, n = re.subn(r"// ── DecodeLatencyKey raw decode from BPF bytes[^\n]*\n\nfunc TestDecodeLatencyKey_RawBytes\(t \*testing\.T\) \{\n.*?^\}\n", """// ── DecodeLatencyTraceKey raw decode from BPF bytes ─────────────────────────

func TestDecodeLatencyTraceKey_RawBytes(t *testing.T) {
	var b [LatencyTraceKeySize]byte
	b[0], b[15] = 0xAB, 0xCD
	binary.LittleEndian.PutUint32(b[16:20], 4194303)
	binary.LittleEndian.PutUint32(b[20:24], 4194301)

	lk, err := DecodeLatencyTraceKey(b[:])
	if err != nil {
		t.Fatalf("DecodeLatencyTraceKey: %v", err)
	}
	if lk.PID != 4194303 || lk.TID != 4194301 || lk.TraceID[0] != 0xAB || lk.TraceID[15] != 0xCD {
		t.Errorf("key = %+v", lk)
	}
}
""", s, flags=re.S | re.M)
if n != 1:
    sys.exit("raw-bytes test not found")
open(T, "w").write(s)

M = "cmd/trace-collector-go/main.go"; s = open(M).read()
s = rep(s, "lk, err := DecodeLatencyKey(key)", "lk, err := DecodeLatencyTraceKey(key)", M)
open(M, "w").write(s)

F = "cmd/trace-collector-go/decode_fuzz_test.go"; s = open(F).read()
s = rep(s, "func FuzzDecodeLatencyKey(f *testing.F) {", "func FuzzDecodeLatencyTraceKey(f *testing.F) {", F)
s = rep(s, "seedSizes(f, LatencyKeySize)", "seedSizes(f, LatencyTraceKeySize)", F)
s = s.replace("DecodeLatencyKey(", "DecodeLatencyTraceKey(")
open(F, "w").write(s)

E = "cmd/trace-collector-go/latency_event_test.go"
open(E, "w").write('''// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package main

import (
	"testing"
	"time"
)

// LATENCY_MAP is keyed by trace id, pid and tid (DP-408). It used to carry
// those under 5-tuple names, truncated to 16 bits: "[::]:18570 -> [::]:18654".
func TestLatencyMapEvent_TraceKey(t *testing.T) {
	lk := &LatencyTraceKey{PID: 4194303, TID: 4194301}
	lk.TraceID[7], lk.TraceID[15] = 1, 2
	le := &LatencyEntry{RTTNS: 98879, MinRTTNS: 678, MaxRTTNS: 5460725, Samples: 607968}
	now := time.Unix(1792734554, 213496086)

	evt := latencyMapEvent(lk, le, now)
	if _, ok := evt["flow_key"]; ok {
		t.Errorf("flow_key present: the map key holds no addresses")
	}
	if evt["latency_ns"] != uint64(98879) || evt["samples"] != uint64(607968) {
		t.Errorf("latency_ns/samples = %v/%v, want 98879/607968", evt["latency_ns"], evt["samples"])
	}
	if evt["timestamp_ns"] != uint64(now.UnixNano()) {
		t.Errorf("timestamp_ns = %v, want wall clock %d", evt["timestamp_ns"], now.UnixNano())
	}
	if evt["pid"] != uint32(4194303) || evt["tid"] != uint32(4194301) {
		t.Errorf("pid/tid = %v/%v, want the full ids", evt["pid"], evt["tid"])
	}
	tr := evt["trace_id"].(map[string]uint64)
	if tr["high"] != 1 || tr["low"] != 2 {
		t.Errorf("trace_id = %v, want high 1 low 2", tr)
	}
}
''')

B = "pkg/protocol/bpfschema/core_maps.go"; s = open(B).read()
s = s.rstrip("\n") + '''

// === latency-probe (DP-408) ===

// LatencyTraceKey mirrors ebpf/latency-probe/src/common.rs LatencyTraceKey:
// one thread's samples under one socket trace id.
type LatencyTraceKey struct {
	TraceID [16]byte // Socket trace id, big-endian high then low; zero when none.
	PID     uint32   // Full kernel pid (tgid).
	TID     uint32   // Full kernel tid.
}

// LatencyTraceKeySize is the exact wire size.
const LatencyTraceKeySize = 24

// LatencyEntry mirrors ebpf/latency-probe/src/common.rs LatencyEntry.
type LatencyEntry struct {
	RTTNs        uint64 // Most recent RTT sample.
	MinRTTNs     uint64 // Minimum observed RTT.
	MaxRTTNs     uint64 // Maximum observed RTT.
	Samples      uint64 // Number of samples.
	LastUpdateNs uint64 // bpf_ktime_get_ns of the last update.
}
'''
open(B, "w").write(s)

B = "pkg/protocol/bpfschema/bpfschema.go"; s = open(B).read()
s = rep(s, '''	// H11: Dictionary authority enforcement (shield-ebpf).
''', '''	// Per-thread RTT statistics keyed by trace id, pid, tid (latency-probe).
	MapLatency = "LATENCY_MAP"

	// H11: Dictionary authority enforcement (shield-ebpf).
''', B)
open(B, "w").write(s)

B = "pkg/protocol/bpfschema/rust_source_test.go"; s = open(B).read()
s = rep(s, '''	{reflect.TypeOf(FlowCancelValue{}), "flow-tracker/src/main.rs", "CancelFlowValue"},
}
''', '''	{reflect.TypeOf(FlowCancelValue{}), "flow-tracker/src/main.rs", "CancelFlowValue"},
	{reflect.TypeOf(LatencyTraceKey{}), "latency-probe/src/common.rs", "LatencyTraceKey"},
	{reflect.TypeOf(LatencyEntry{}), "latency-probe/src/common.rs", "LatencyEntry"},
}
''', B)
s = rep(s, '''	{"MapRetryTokens", MapRetryTokens, nil, nil},
''', '''	{"MapRetryTokens", MapRetryTokens, nil, nil},
	{"MapLatency", MapLatency, reflect.TypeOf(LatencyTraceKey{}), reflect.TypeOf(LatencyEntry{})},
''', B)
open(B, "w").write(s)
import subprocess
subprocess.run(["gofmt", "-w", R, T], check=True)
print("latency_go.py: all edits applied")
````


---

*S-WIRE-HONEST Battle Plan — Forged 2026-11-04*
*19 Phases. 137 Steps. The checks go where the Monad is, and they say what they cannot prove.*
*Nothing on the wire moves. What moves is what we claim about it.*
