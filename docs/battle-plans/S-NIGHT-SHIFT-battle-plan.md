<!--
SPDX-License-Identifier: GPL-3.0-or-later
Copyright (c) 2024-2026 Stevie Bellis.
-->
# S-NIGHT-SHIFT BATTLE PLAN — 8 Phases, 92 Steps

**Date**: 2026-11-09
**Sprint**: S-NIGHT-SHIFT — every queued, decided task that needs no attended session, after the 2026-11-09 question rounds
**Authority**: Stevie, 2026-11-09: "I am unable to answer any more questions for a while - please create revised sprint battle plan containing any available work that is queued and does not require an attended session", then "Utilize unheaded-warmonger to write and then /loop unheaded-marshal in". His request lifts ADR-112's freeze for this one plan.
**Prerequisite**: develop at or after `274b7a4c`, clean tree apart from untracked `demos/doom/`, gpg key cached (`git commit-tree -S` probe passes).
**Target**: every item in Phases 1-5 committed signed on develop, Phase 6's full verification green on a clean clone, Phase 7's handoff written. Nothing pushed, nothing live changed.
**Estimated Duration**: 5-8 hours wall clock, run as `/loop` ticks under `unheaded-marshal`.
**Agent Strategy**: the coordinator (Opus, the `/loop` session) runs every phase itself. Phases 1-5 are independent of each other; take them in order. Phase 6 needs 1-5 done or `[STUCK]`. Phase 7 last. No subagent needs sudo; Phase 3's docker build is the coordinator's (`sudo -n docker`).
**Commit Cadence**: every 4 steps, plus at every phase gate and before any skip (`max(3, min(5, 92/20)) = 4`).
**Stuck Protocol**: skip a step after 3x its estimate or 2 failed debug attempts; mark it `[STUCK]` in `docs/battle-plans/progress/S-NIGHT-SHIFT-progress.md` with what was tried and what it needs; continue with the next independent step.
**Answers this plan executes**: `~/tmp/answers-2026-11-09.md` (26 answers). Items already done and merged before this plan are listed in Appendix D so nobody redoes them.

---

## RULES (read before Step 1)

1. **Unattended only.** No `systemctl` on east or west, no Well writes, no installs, no root eBPF loads, no `git push`, no sealing. Anything that needs those goes to the progress file's "For Stevie" list. Appendix C lists what is out of scope and why.
2. **Signed commits on develop**, conventional style, trailers exactly:
   `Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>` and `Claude-Session: https://claude.ai/code/session_01TzL3TsEuBiPTFqAKudViit`.
   If gpg times out: `--no-gpg-sign` and list the hash in the progress file (re-sign later with `scripts/resign-unpushed.py`).
3. **Stage named files only.** Never `git add -A`, `git add .` or `git add <dir>`. Never touch `demos/doom/`.
4. **TDD for code**: failing test first, then the change, then the test green.
5. **No new third-party code, tools or actions** (2026-10-29 rule). Bumping an existing pin to a newer release of the same tool is allowed (compile-against-latest rule).
6. **Never `pkill -f`**; never background a command inside a step; never run more than three builders at once (14 GiB).
7. **Dated records are never edited** (any doc with a date in its name, `docs/archive/`, `docs/sweeps/`, old battle plans). Add a one-line "superseded" note instead.
8. **Collected data stays on the host.** Generated reports go under `var/` (gitignored) or outside the tree.
9. **Doom is last** and is not in this plan.
10. **Marshal checkpoints**: at each phase gate the `/loop` tick runs the Marshal's check (is the step order kept, is the commit cadence kept, did anything drift outside the plan's scope) and records one line in the progress file.

## LEGEND

`[B]` Bash command · `[V]` Verification (must pass) · `[D]` Debug (only if the prior step fails) · `[W]` Write/create file · `[R]` Read/inspect · `[S]` Sudo (local docker only in this plan) · `[P]` Parallelizable · `[C]` Commit checkpoint · `[STUCK]` skipped via Skip Protocol · `[BLOCKED]` blocked by a STUCK step · `~Nm` expected wall-clock minutes

Paths: `R=/home/govan/tmp/unheaded`. CI venv: `/var/tmp/unheaded-sprint/civenv`. Raft venv: `/var/tmp/unheaded-sprint/raftvenv`.

---

## PHASE 0: PREFLIGHT (Steps 1-8)

**Goal**: the tree, the key and the host are fit to work.
**Prerequisite**: none. **Time**: 10 min. **Agent**: Coordinator.

- [ ] **Step 1** [B] ~1m: tree state
  ```bash
  cd /home/govan/tmp/unheaded && git status --short | grep -v '^?? demos/doom/' ; git log --oneline -1
  ```
- [ ] **Step 2** [V] ~1m: on `develop`, no output from the status line above besides nothing. If dirty → Step 3, else Step 4.
- [ ] **Step 3** [D] ~3m: read the diff (`git diff --stat`); if it is this plan's own unfinished step, finish or revert that step; otherwise STOP and record it under "For Stevie".
- [ ] **Step 4** [B] ~1m: signing probe
  ```bash
  cd /home/govan/tmp/unheaded && git commit-tree -S HEAD^{tree} -m probe </dev/null >/dev/null && echo SIGN_OK || echo SIGN_FAIL
  ```
- [ ] **Step 5** [V] ~1m: `SIGN_OK`. If `SIGN_FAIL`: carry on with `--no-gpg-sign` (Rule 2) and note it.
- [ ] **Step 6** [B] ~1m: host headroom and the task watchdog
  ```bash
  df -h /tmp /var | tail -2; free -g | head -2; cat /sys/fs/cgroup/user.slice/user-1000.slice/pids.max /sys/fs/cgroup/user.slice/user-1000.slice/memory.max
  pgrep -f thread-watch.sh >/dev/null && echo WATCH_OK || echo WATCH_MISSING
  ```
- [ ] **Step 7** [V] ~1m: `/var` over 50 GB free, `pids.max` 4096, `memory.max` 12884901888. If `WATCH_MISSING`, start `/var/tmp/unheaded-sprint/thread-watch.sh` as a background task of the session (not inside a step).
- [ ] **Step 8** [W][C] ~3m: create `docs/battle-plans/progress/S-NIGHT-SHIFT-progress.md` with sections `## Log`, `## STUCK`, `## For Stevie`, `## Marshal`; commit it.
  ```bash
  cd /home/govan/tmp/unheaded && git add docs/battle-plans/S-NIGHT-SHIFT-battle-plan.md docs/battle-plans/progress/S-NIGHT-SHIFT-progress.md && git commit -S -m "[PLAN S-NIGHT-SHIFT] Step 8: progress file" -m "Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>
  Claude-Session: https://claude.ai/code/session_01TzL3TsEuBiPTFqAKudViit"
  ```
  **PHASE 0 EXIT GATE**: clean tree on develop, signing known, progress file committed.

---

## PHASE 1: WOTAN'S OLD TABLES — DROP MIGRATION, NOT APPLIED (Steps 9-20)

**Goal**: answer 26. A reviewed migration that drops `wotan_messages`, `wotan_sequences`, `wotan_cluster` waits in the S-E1-LIVE batch. Nothing touches the live Well.
**Prerequisite**: Phase 0. **Time**: 45 min. **Agent**: Coordinator.

- [ ] **Step 9** [R] ~3m: where the tables live. The 2026-10-15 snapshot (`docs/battle-plans/well-health-2026-10-15/unheaded.txt`, lines 135-141) shows them in the legacy database `unheaded`; `db/migrations/MANIFEST` routes only `postgres`, `unheaded_app`, `unheaded_config`, `unheaded_ops`.
  ```bash
  cd /home/govan/tmp/unheaded && grep -n 'wotan_' docs/battle-plans/well-health-2026-10-15/*.txt | head; awk '!/^#/ && NF {print $1}' db/migrations/MANIFEST | sort | uniq -c
  grep -rn '"unheaded"\|database unheaded\b\|-d unheaded\b' cmd/well-migrate db/init.sh scripts/well/*.sh 2>/dev/null | head
  ```
- [ ] **Step 10** [V] ~2m: decide the route. If `well-migrate` can target the database `unheaded` (a MANIFEST row with database `unheaded` is accepted by its parser), → Step 11. If it cannot → Step 10a.
- [ ] **Step 10a** [D] ~5m: no route to `unheaded`. Do NOT add parser support unasked. Write the SQL as `db/migrations/050_unheaded_drop_wotan_tables.sql` and route it `skip` in the MANIFEST with a comment naming S-E1-LIVE, and add one S-E1-LIVE line: "apply 050 by hand to database `unheaded` with `psql -d unheaded -f`, or decide whether the legacy `unheaded` database itself is dropped." Then → Step 12.
- [ ] **Step 11** [W] ~5m: write `db/migrations/050_unheaded_drop_wotan_tables.sql` (SPDX header; one transaction; `DROP TABLE IF EXISTS wotan_messages, wotan_sequences, wotan_cluster;`; a comment citing Stevie's answer 26 and `8c21e0ca`) and its MANIFEST row in the right place in order.
- [ ] **Step 12** [B] ~3m: the migration gates
  ```bash
  cd /home/govan/tmp/unheaded && go test -count=1 ./cmd/well-migrate/... 2>&1 | tail -3; ls scripts | grep -i -E 'check-(migration|well)' ; for s in $(ls scripts | grep -i -E '^check-(migration|well)'); do bash scripts/$s >/dev/null 2>&1 && echo "$s ok" || echo "$s FAIL"; done
  ```
- [ ] **Step 13** [V] ~1m: all ok. If a MANIFEST/ledger test fails → Step 14.
- [ ] **Step 14** [D] ~5m: read the failing test; the usual cause is order, a missing `skip`, or the sha list in `db/migrations/LEDGER.sql` (only add if the other migrations have entries there before being applied; never edit an applied file).
- [ ] **Step 15** [B] ~5m: S-E1-LIVE's batch list names it
  ```bash
  cd /home/govan/tmp/unheaded && grep -n -E '047|049' docs/battle-plans/S-E1-LIVE*.md | head
  ```
- [ ] **Step 16** [W] ~5m: add 050 beside 047 and 049 in S-E1-LIVE's migration batch line and its pending count (the plan is a live plan, not a dated record).
- [ ] **Step 17** [B] ~2m: `cd /home/govan/tmp/unheaded && /var/tmp/unheaded-sprint/civenv/bin/python -m pytest -q tests/scripts/test_doc_links.py | tail -1`
- [ ] **Step 18** [V] ~1m: passes.
- [ ] **Step 19** [C] ~2m: commit `db(well): 050 drops Wotan's unused tables (S-E1-LIVE batch, not applied)` with the files from Steps 10a/11/16 named.
- [ ] **Step 20** [V] ~1m: **PHASE 1 EXIT GATE** — `git log -1 --format=%G?` is `G` (or noted), the migration file exists, `git grep -n 050_unheaded_drop_wotan_tables` hits MANIFEST and S-E1-LIVE. Marshal line in progress file.

---

## PHASE 2: TRACE-COLLECTOR — THE DEAD 68-BYTE DECODER GOES (Steps 21-32)

**Goal**: answer 3's tail. With the poll path deleted (`9da03ff4`), `DecodeTraceEntry`/`Encode` (68-byte trace entries, `types.go`) and packet_marker's `DecodeStatsEntry` have no caller outside tests and benchmarks. Remove them.
**Prerequisite**: Phase 0. **Time**: 40 min. **Agent**: Coordinator.

- [ ] **Step 21** [R] ~3m: every caller
  ```bash
  cd /home/govan/tmp/unheaded && grep -rn -E 'DecodeTraceEntry|\.Encode\(\)|DecodeStatsEntry|TraceEntrySize|traceEntrySize' --include=*.go . | grep -v '/target/' | head -30
  ```
- [ ] **Step 22** [V] ~2m: only `cmd/trace-collector-go` (and its tests/benchmarks/fuzz) uses them, and no other package imports them. If another package uses one → keep that one, record why, continue with the rest.
- [ ] **Step 23** [B] ~2m: baseline `cd /home/govan/tmp/unheaded && go test -race -count=1 ./cmd/trace-collector-go/... 2>&1 | tail -2`
- [ ] **Step 24** [W] ~10m: delete the dead functions, their tests, benchmarks and fuzz targets (`decode_fuzz_test.go` if it fuzzes only these; keep fuzz targets for live decoders). If a fuzz target is removed, check `.github/workflows/security.yml` / the Go fuzz job list and `tests/scripts` guards that enumerate fuzz targets.
- [ ] **Step 25** [B] ~3m: `cd /home/govan/tmp/unheaded && gofmt -l cmd/trace-collector-go; go vet ./cmd/trace-collector-go/... && go test -race -count=1 ./cmd/trace-collector-go/... 2>&1 | tail -2 && go build ./...`
- [ ] **Step 26** [V] ~1m: no gofmt output, vet clean, tests ok, build ok.
- [ ] **Step 27** [D] ~5m: a test still references a removed symbol → it tested only the dead path; remove it. A live path broke → restore that symbol and record it.
- [ ] **Step 28** [B] ~3m: fuzz-target guards
  ```bash
  cd /home/govan/tmp/unheaded && grep -rn -l 'FuzzDecode\|decode_fuzz' .github tests scripts 2>/dev/null | head; /var/tmp/unheaded-sprint/civenv/bin/python -m pytest -q tests/scripts 2>&1 | tail -1
  ```
- [ ] **Step 29** [V] ~1m: script tests pass; no workflow names a removed fuzz target.
- [ ] **Step 30** [B] ~2m: `cd /home/govan/tmp/unheaded && bash scripts/gen-man.sh --check | tail -1; bash scripts/check-nosec-ratchet.sh | tail -1`
- [ ] **Step 31** [C] ~2m: commit `refactor(trace-collector): drop the 68-byte trace decoder the poll path used`.
- [ ] **Step 32** [V] ~1m: **PHASE 2 EXIT GATE** — the grep of Step 21 finds no definition of the removed symbols; tests green. Marshal line.

---

## PHASE 3: DOCKER IMAGES PASS THE NEW TRIVY GATE (Steps 33-52)

**Goal**: answer 6 made docker.yml fail on any CRITICAL or HIGH, fixed or not (`109d17af`). The wotan image had 3 untriaged results. Make both images pass locally before GitHub sees them, and check `wotan --version` in the image (S-ANSWERED redci note).
**Prerequisite**: Phase 0. **Time**: 90 min. **Agent**: Coordinator (docker needs sudo on west).

- [ ] **Step 33** [B] ~3m: the pinned trivy
  ```bash
  source /home/govan/tmp/unheaded/scripts/lib/tool-pins.sh; echo "$TRIVY_VERSION"; ls /var/tmp/unheaded-sprint/bin/trivy-$TRIVY_VERSION 2>/dev/null || echo NEED_INSTALL
  ```
- [ ] **Step 34** [B] ~5m: if `NEED_INSTALL`, install the pin with the repo's checksum-verified installer into a private dir (no system install)
  ```bash
  cd /home/govan/tmp/unheaded && sed -n 1,30p scripts/install-trivy.sh | grep -E 'usage|Usage|DEST|--' ; mkdir -p /var/tmp/unheaded-sprint/bin
  # run it with the destination flag/env the script documents, e.g.:
  # TRIVY_DEST=/var/tmp/unheaded-sprint/bin bash scripts/install-trivy.sh
  ```
- [ ] **Step 35** [V] ~1m: `<dest>/trivy --version` prints `$TRIVY_VERSION`. If the installer wants root or a system path → Step 35a.
- [ ] **Step 35a** [D] ~3m: use the existing `~/.local/bin/trivy` (0.72.0) for triage only, and mark the result "pre-pin" in the progress file. Do not install elsewhere.
- [ ] **Step 36** [S][B] ~15m: build the two images the way docker.yml does (amd64, root Dockerfile targets)
  ```bash
  cd /home/govan/tmp/unheaded && sudo -n docker build --target wotan -t nightshift/wotan:local -f Dockerfile . 2>&1 | tail -3
  sudo -n docker build --target kanban-app -t nightshift/kanban:local -f Dockerfile . 2>&1 | tail -3
  ```
- [ ] **Step 37** [V] ~1m: both build. If one fails → Step 38.
- [ ] **Step 38** [D] ~10m: read the failing stage; a Go toolchain or base-image pin mismatch is the usual cause (`check-go-env.sh`, `scripts/lib/tool-pins.sh`). Fix in the Dockerfile only if it is a pin out of step with go.mod; otherwise `[STUCK]`.
- [ ] **Step 39** [S][B] ~5m: `wotan --version` in the image
  ```bash
  sudo -n docker run --rm nightshift/wotan:local --version; echo rc=$?
  ```
- [ ] **Step 40** [V] ~1m: prints `wotan (unheaded) <version>` with rc 0. If it prints the flag list → Step 41.
- [ ] **Step 41** [D] ~10m: check the image's ENTRYPOINT/CMD (an arg before `--version`?) and the `-X unheaded/pkg/cmdline.Version` ldflag in the Dockerfile's build line; fix and add the image check to `scripts/check-version-flag.sh` only if it already covers images.
- [ ] **Step 42** [S][B] ~10m: scan as the gate does
  ```bash
  T=/var/tmp/unheaded-sprint/bin/trivy-$TRIVY_VERSION; [ -x "$T" ] || T=trivy
  for i in wotan kanban; do sudo -n "$T" image --severity CRITICAL,HIGH --format table --exit-code 1 nightshift/$i:local > /var/tmp/unheaded-sprint/nightshift-trivy-$i.txt 2>&1; echo "$i rc=$?"; done
  ```
- [ ] **Step 43** [V] ~1m: both rc 0 → Step 50. Any rc 1 → Step 44.
- [ ] **Step 44** [R] ~5m: classify each finding: OS package in the base image, Go stdlib (toolchain), Go module, other.
  ```bash
  grep -E 'CRITICAL|HIGH' /var/tmp/unheaded-sprint/nightshift-trivy-*.txt | head -40
  ```
- [ ] **Step 45** [W] ~15m: fix what has a fix: bump the base image digest to the current tag's digest (pinned by digest as the tree does), bump the Go toolchain per go.mod, `go get` a fixed module version (existing modules only, Rule 5). Record each in the progress file.
- [ ] **Step 46** [B] ~15m: rebuild and rescan (Steps 36 and 42).
- [ ] **Step 47** [V] ~1m: both rc 0 → Step 49. Findings left with no fixed release → Step 48.
- [ ] **Step 48** [D] ~10m: unfixable findings: try a smaller base (distroless/static if the stage already allows it) only if the Dockerfile's design admits it without a new tool; else `[STUCK]` with the list: the gate is Stevie's, "fixed or not", so the choice (base change, ignore file with expiry, or soften) is his. Never add an ignore or `ignore-unfixed` yourself.
- [ ] **Step 49** [C] ~3m: commit `build(docker): images clear the Trivy gate` with the changed files named (Dockerfile, go.mod/go.sum, pins).
- [ ] **Step 50** [B] ~3m: `cd /home/govan/tmp/unheaded && PATH="$PATH:/var/tmp/unheaded-sprint/civenv/bin" bash scripts/test-check-ci-verdicts.sh | tail -2`
- [ ] **Step 51** [S][B] ~1m: tidy: `sudo -n docker rmi nightshift/wotan:local nightshift/kanban:local >/dev/null 2>&1; true`
- [ ] **Step 52** [V] ~1m: **PHASE 3 EXIT GATE** — both scans rc 0 (or the leftovers are `[STUCK]` with their CVE list in "For Stevie"); `--version` correct; ci-verdicts self-test passes. Marshal line.

---

## PHASE 4: PINS, SCANNERS, MISCONFIG ON THE PINNED TRIVY (Steps 53-66)

**Goal**: the routine supply-chain checks the 11-07/11-08 ticks ran, plus the two carried items: `check-trivy-misconfig` on trivy 0.76.1 (local was 0.72.0), and tool pins that left their cooldown (ruff 0.17.4 was due).
**Prerequisite**: Phase 0 (Phase 3's trivy install helps). **Time**: 60 min. **Agent**: Coordinator.

- [ ] **Step 53** [B] ~5m: `cd /home/govan/tmp/unheaded && PATH="$PATH:/var/tmp/unheaded-sprint/civenv/bin" bash scripts/check-tool-freshness.sh 2>&1 | tail -15`
- [ ] **Step 54** [V] ~1m: no stale pin → Step 57. A stale pin → Step 55.
- [ ] **Step 55** [W] ~10m: bump each stale pin to the release the script names, everywhere the pin lives (`scripts/lib/tool-pins.sh`, `.github/ci-python/requirements.txt` with hashes regenerated the way that file says, workflow `version:` lines). Existing tools only.
- [ ] **Step 56** [B] ~5m: rerun Step 53 and the gate the tool feeds (ruff: `ruff check` over the paths CI lints; golangci-lint: note it panics on go1.27 — do not chase).
- [ ] **Step 57** [C] ~2m: commit if anything changed: `chore(pins): <tool> <old> -> <new>`.
- [ ] **Step 58** [B] ~10m: misconfig ratchet on the pin
  ```bash
  cd /home/govan/tmp/unheaded && T=/var/tmp/unheaded-sprint/bin/trivy-$(. scripts/lib/tool-pins.sh; echo $TRIVY_VERSION); PATH="$(dirname $T):$PATH" bash scripts/check-trivy-misconfig.sh 2>&1 | tail -8
  ```
- [ ] **Step 59** [V] ~1m: passes. If it reports stale rows (fixed findings still baselined) → Step 60. New findings → Step 61.
- [ ] **Step 60** [W] ~5m: shrink the baseline the way the script says (`--update` only removes; never add rows).
- [ ] **Step 61** [D] ~10m: a new finding: fix the config it names if the fix is mechanical and inside the repo's own files; otherwise `[STUCK]` with the finding id.
- [ ] **Step 62** [B] ~10m: scanners
  ```bash
  cd /home/govan/tmp/unheaded && govulncheck ./... 2>&1 | tail -5; (cd ebpf && cargo audit 2>&1 | tail -5); PATH="$PATH:/var/tmp/unheaded-sprint/civenv/bin" bash scripts/check-python-audit.sh 2>&1 | tail -5
  ```
- [ ] **Step 63** [V] ~1m: govulncheck "No vulnerabilities found"; cargo audit only the allowed unmaintained warnings; pip-audit only the 2 accepted. Anything new → Step 64.
- [ ] **Step 64** [D] ~15m: a new advisory with a fixed release in an existing dependency: bump it (KEV ones first, per the 2026-11-04 rule), test the package, commit. No fixed release → "For Stevie".
- [ ] **Step 65** [C] ~2m: commit any Step 60/64 changes.
- [ ] **Step 66** [V] ~1m: **PHASE 4 EXIT GATE** — freshness clean or bumped, misconfig ratchet passes on the pin, scanners at their known baselines. Marshal line.

---

## PHASE 5: DOC TRUTH (Steps 67-78)

**Goal**: the small doc debts the 2026-11-09 work and the S-ANSWERED lanes left.
**Prerequisite**: Phase 0. **Time**: 45 min. **Agent**: Coordinator.

- [ ] **Step 67** [R] ~2m: `wiki/ADR-Index.md` lacks rows 136 and 137 (it jumps 135 → 138).
  ```bash
  cd /home/govan/tmp/unheaded && grep -n -E '^\| \[13[5-8]\]' wiki/ADR-Index.md docs/adr/ADR-INDEX.md | cut -c1-120
  ```
- [ ] **Step 68** [W] ~5m: add 136 and 137 to the wiki index, in order, with the short titles from `docs/adr/ADR-INDEX.md`.
- [ ] **Step 69** [W] ~10m: answer 25 of 2026-11-06 ("all as stated") ruled the SBOM REVIEW list: GPL CI tools are fine because they do not ship; NVIDIA wheels stay out of anything shipped; a version bump fails until `sbom/` is regenerated. `sbom/SBOM.md` still says "for a person to rule on". It is generated by `scripts/lib/sbom_licenses.py` (line ~454): put the ruling where regeneration keeps it (a rulings constant or file the generator reads, then the verdict says "ruled 2026-11-06: …" for the named classes), with a unit test if `scripts/lib` has tests. Do not hand-edit the generated file only.
- [ ] **Step 70** [B] ~5m: `cd /home/govan/tmp/unheaded && ls tests/scripts | grep -i sbom; /var/tmp/unheaded-sprint/civenv/bin/python -m pytest -q tests/scripts 2>&1 | tail -1; bash scripts/test-generate-sbom.sh 2>&1 | tail -2`
- [ ] **Step 71** [V] ~1m: tests pass. Regenerating `sbom/` needs the full generator run (scratch venvs, ScanCode); if that is heavy, regenerate only if the generator offers a verdict-only mode, else record "regenerate sbom/ at the next full run" in "For Stevie".
- [ ] **Step 72** [C] ~2m: commit `docs: wiki ADR index rows 136-137; SBOM REVIEW ruling recorded`.
- [ ] **Step 73** [R] ~3m: the evidence lane said `wiki/Releases.md`'s apt line is behind. Compare it with `scripts/apt-repo.sh` and `deploy/nginx/unheaded-apt.conf` (served root, codename, port).
- [ ] **Step 74** [W] ~5m: fix only facts the tree proves (paths, codename, port); S-E1-LIVE Step 192 moves the root to `/srv/unheaded-apt` and is not done, so say "after S-E1-LIVE Step 192" where the move matters.
- [ ] **Step 75** [W] ~5m: on the parked branch `sprint/hop-enforce-refusal` (not develop), add one line to `ebpf/README.md` naming the `hop-enforce` feature (test path only). Use a temporary worktree: `git worktree add /home/govan/tmp/wt/hop-enforce sprint/hop-enforce-refusal`, edit, commit signed there, `git worktree remove /home/govan/tmp/wt/hop-enforce`.
- [ ] **Step 76** [B] ~2m: `cd /home/govan/tmp/unheaded && /var/tmp/unheaded-sprint/civenv/bin/python -m pytest -q tests/scripts/test_doc_links.py | tail -1`
- [ ] **Step 77** [C] ~2m: commit the develop-side doc fixes (Step 74).
- [ ] **Step 78** [V] ~1m: **PHASE 5 EXIT GATE** — both indexes list 135-138; SBOM ruling survives regeneration (test or generator read); doc links pass; hop branch has the README line. Marshal line.

---

## PHASE 6: FULL VERIFICATION ON A CLEAN CLONE (Steps 79-86)

**Goal**: everything merged today (Appendix D) and in Phases 1-5 holds together, the way CI's push shards would see it.
**Prerequisite**: Phases 1-5 done or `[STUCK]`. **Time**: 60-90 min. **Agent**: Coordinator. Run nothing else meanwhile (three-builder cap).

- [ ] **Step 79** [B] ~3m: clean clone at develop's tip
  ```bash
  rm -rf /var/tmp/unheaded-sprint/nightshift-clone && git clone -q --branch develop /home/govan/tmp/unheaded /var/tmp/unheaded-sprint/nightshift-clone && git -C /var/tmp/unheaded-sprint/nightshift-clone log --oneline -1
  ```
- [ ] **Step 80** [B] ~25m: `cd /var/tmp/unheaded-sprint/nightshift-clone && go build ./... && go vet ./... && go test -race -shuffle=on -count=1 ./... 2>&1 | grep -v -E '^(ok|\?)' | tail -20`
- [ ] **Step 81** [V] ~1m: no FAIL lines. A failure → Step 82.
- [ ] **Step 82** [D] ~15m: rerun the failing package alone 3 times; a flake gets a root-cause fix in the test fixture (as `35be96b4` did), not a retry; a real failure gets fixed on develop, then Step 79 again. Two failed attempts → `[STUCK]`.
- [ ] **Step 83** [B] ~40m: full meta-gate, CI venv LAST on PATH (gotcha from 11-08: python3 must stay system for run_check)
  ```bash
  cd /var/tmp/unheaded-sprint/nightshift-clone && PATH="$PATH:/var/tmp/unheaded-sprint/civenv/bin" bash scripts/check-gates-can-fail.sh 2>&1 | tail -15
  ```
- [ ] **Step 84** [V] ~1m: every row OK or proven; rows that need sudo or a live host report as skipped, not failed. A red row → fix on develop (same rules as Step 82), then rerun that row alone.
- [ ] **Step 85** [B] ~5m: `cd /home/govan/tmp/unheaded && bash scripts/check-gitleaks.sh --history 2>&1 | tail -3; git log --format=%G? origin/develop..develop | sort | uniq -c`
- [ ] **Step 86** [V] ~1m: **PHASE 6 EXIT GATE** — race suite green, meta-gate green (or each red row `[STUCK]` with cause), gitleaks shows only the known 40, every commit since origin/develop is `G` (or listed unsigned). Marshal line; commit the progress file.

---

## PHASE 7: HANDOFF (Steps 87-92)

**Goal**: the next session, and Stevie on his phone, can see what happened in one screen.
**Prerequisite**: Phase 6. **Time**: 30 min. **Agent**: Coordinator.

- [ ] **Step 87** [B] ~2m: archive the old briefing (it is 368 KB): `cp /home/govan/tmp/next.md /home/govan/tmp/next-archive-2026-11-09.md`
- [ ] **Step 88** [W] ~15m: rewrite `/home/govan/tmp/next.md` short: state (develop tip, ahead of origin, all signed), what this plan did, every `[STUCK]` with its need, the "For Stevie" list (attended items from Appendix C first, then new ones), pointers to `~/tmp/answers-2026-11-09.md`, the progress file and the archive. Keep the standing rules block (watchdog after reboot, nginx now retries itself, never print secrets).
- [ ] **Step 89** [W] ~5m: memory: update `project_*` entries this plan changed (one file per fact; MEMORY.md one line each); add nothing the repo already records.
- [ ] **Step 90** [C] ~2m: commit the progress file's final state on develop.
- [ ] **Step 91** [B] ~1m: `cd /home/govan/tmp/unheaded && git status --short | grep -v demos/doom; git log --oneline origin/develop..develop | wc -l`
- [ ] **Step 92** [V] ~1m: **PHASE 7 EXIT GATE / SPRINT CLOSE** — clean tree; next.md rewritten; the `/loop` stops (`ScheduleWakeup stop`), and the final message to Stevie is five lines: done, stuck, needs him, nothing pushed.

---

## APPENDIX A: EMERGENCY PROCEDURES

**A1. Tasks climb past 2000 (watchdog fires).** Stop launching anything. Read the dump it names (`/var/tmp/unheaded-sprint/tasks-dump-*.txt`). If one of this plan's commands is the parent, end that command by PID (never `pkill -f`). The user slice now stops at 4096 tasks and 12 GiB, so a runaway is killed rather than freezing west; record it and resume at the step that ran it.

**A2. gpg stops signing mid-plan.** Rule 2: `--no-gpg-sign`, append each hash to the progress file's "unsigned" line; Stevie re-signs with `scripts/resign-unpushed.py ~/tmp/unheaded origin/develop --merges`.

**A3. Docker build fills /var.** `sudo -n docker builder prune -f` (local build cache only); never prune images Stevie runs (`docker ps -a` first).

**A4. A gate goes red on develop from this plan's own commit.** Fix forward on develop in the next commit; never rewrite pushed history; unpushed history may be amended only for the tip commit, before anything else lands.

**A5. west reboots mid-plan.** On return: Phase 0 again (watchdog, signing), `sudo systemctl status nginx` (it retries itself since 2026-11-09), then resume at the first unchecked step in the progress file.

**A6. A step needs a decision.** It is out of scope: record it in "For Stevie" with the options and a recommendation, mark the step `[STUCK]`, continue. Do not ask: Stevie said he cannot answer for a while.

## APPENDIX B: AGENT ASSIGNMENT MATRIX

| Phase | Agent | Parallel | Depends on | Time |
|---|---|---|---|---|
| 0 Preflight | Coordinator | no | none | 10 min |
| 1 Wotan tables migration | Coordinator | [P] with 2, 4, 5 | 0 | 45 min |
| 2 Dead decoder | Coordinator | [P] with 1, 4, 5 | 0 | 40 min |
| 3 Docker Trivy gate | Coordinator (sudo docker) | alone (builds) | 0 | 90 min |
| 4 Pins and scanners | Coordinator | [P] with 1, 2, 5 | 0 (3 for the pinned trivy) | 60 min |
| 5 Doc truth | Coordinator | [P] with 1, 2, 4 | 0 | 45 min |
| 6 Clean-clone verification | Coordinator | alone | 1-5 | 60-90 min |
| 7 Handoff | Coordinator | no | 6 | 30 min |

Critical path: 0 → 3 → 6 → 7 (about 3.5 h). One coordinator runs the phases in order; `[P]` marks what could run together if a later amendment adds agents.

## APPENDIX C: OUT OF SCOPE — NEEDS AN ATTENDED SESSION (do not start)

| Item | Why attended | Where it waits |
|---|---|---|
| hop_xdp enforce refusal proof (answer 23) | root run of `tests/hopchecks`, insns baseline update | branch `sprint/hop-enforce-refusal` (`871345d2`), commands in its commit and next.md |
| Narrow passwordless sudo (answer 15) | lockout risk high; removes the agent's own sudo | register rows `host-probe:{east,west}:sudo-password-required` |
| Apply migrations 047, 049, 050 | live Well | S-E1-LIVE batch |
| Seal `runbooks/MANIFEST.sha256` | Stevie's key | S-E1-LIVE |
| kernel.panic + hardware watchdog (answer 4 "later") | auto-reboot policy | next.md |
| Zhen UI hybrid-only front (answer 11) | ADR-138 is Proposed (parked): no code until Stevie reads it | ADR-138 |
| Branch protection on GitHub (answer 13) | GitHub settings | Stevie |
| Syslog relay install, Vector change live | host installs | S-LOG-HARVEST Step 141+ |
| Old `pihole/*-log` lxd-file rows beside the new relay rows | undecided: retire them or keep both | For Stevie |
| Drop the legacy `unheaded` database itself | live Well, undecided | For Stevie |
| Everything in S-E1-LIVE | attended by design | `docs/battle-plans/S-E1-LIVE*.md` |

## APPENDIX D: ALREADY DONE 2026-11-09 (do not redo)

`18e15950` SLH-DSA NISTLevel ids · `6406e83b` nginx retry drop-in + user-slice caps (live on west) · `109d17af` Trivy image gate, plan/apply deleted, Jenkins Deploy stage gone · `6bc5c863` dated audit restored · `8b8e95d7` DPIA/privacy wording · `1baf1091` findings/SSP stay on host · `8c21e0ca` Wotan PG/hybrid removed · `9da03ff4` trace-collector stats timer, poll path gone · `b4062485` FND-038 resolved · `62fbdede` ADR-138 parked, ADR-035 Accepted · `85f73a23` sudo rows ruled · `9a24aa49` corpus index scope · `865e53ff` syslog relay socket activation, source-row retention, deb · `274b7a4c` Zhen login 5, issuer 10/25.

## APPENDIX E: QUICK REFERENCE

- Commit trailers: see Rule 2. Message prefix for plan steps: `[PLAN S-NIGHT-SHIFT] Steps X-Y: <what>`; topical prefixes (`fix(…)`, `docs(…)`) are fine for a single-purpose commit.
- Venvs: CI `/var/tmp/unheaded-sprint/civenv`, raft `/var/tmp/unheaded-sprint/raftvenv`. System `python3` has no pytest.
- Gates: `scripts/check-gates-can-fail.sh` (meta-gate; `--quick` skips slow rows), `scripts/check-nosec-ratchet.sh [--update]`, `scripts/gen-man.sh --check`, `scripts/runbook-manifest.sh --check|--write` (never `--seal`), `scripts/check-runbooks.sh`, `scripts/check-ci-verdicts.sh` + `scripts/test-check-ci-verdicts.sh`, `tests/scripts/test_doc_links.py`.
- The pre-commit hook runs gofmt on staged Go files: `gofmt -w` and re-stage.
- West: 14 GiB RAM, user slice capped 4096 tasks / 12 GiB; `/tmp` is a 7.2 GiB tmpfs — build scratch under `/var/tmp/unheaded-sprint/`.

---

*S-NIGHT-SHIFT Battle Plan — Forged 2026-11-09*
*8 Phases. 92 Steps. The lamps stay lit while the king sleeps; every gate is kept, and nothing leaves the keep.*
