<!--
SPDX-License-Identifier: GPL-3.0-or-later
Copyright (c) 2024-2026 Stevie Bellis.
-->
# S-MOVE-OUT progress

## For Stevie (oldest first)

- Step 8: `~/tmp/projects/doomgeneric` has uncommitted edits to `doomgeneric/r_data.c` and `doomgeneric/w_wad.c`; they are not in unheaded-doom. DOOM1.WAD stays out of unheaded-doom but remains in upstream doomgeneric history.

## Stuck steps

## Evidence

- Phase 5 (Steps 30-33): Step 30 `10`, `21`. Step 31 rewrote 30 living files, grep rc=1 after. Step 32 rustfmt check silent; 13 code files changed, 21 insertions and 21 deletions, all doc-path lines.
- Phase 1 (Steps 6-11): source develop `2b8da94d` (recorded in `~/tmp/wt/unheaded-doom.source`). Step 6 `53`; every listed path has files at develop. Step 7 `Ref ... was rewritten`, 304 commits, 176 files. Step 8 `2`, `Ref ... was rewritten`, `doomgeneric/unheaded/go.mod`, 425 commits (304 + 120 + merge). Step 9 `COMMITTED`, no count line. Step 10 count `1`, `leaks 2 [tests/ws1-integration.sh]`, `leaks 0`, `0`, `rc=1`. Step 11 `0`, `0`, author names. Tip `0f520f3`, 427 commits, all unsigned by design. Extra check beyond the plan: upstream DOOM1.WAD blob `1a58f662` (4,196,020 bytes) is absent from the new object store; largest object is `doomgeneric/screenshots/sdl.png`, 2,243,101 bytes.
- Phase 1 stale counts, corrected: Step 7 said near 274 commits and 173 files, the tree gave 304 and 176 because develop moved 329 commits past `d6ad43e2`. Step 8 said near 395, the tree gave 425 for the same reason. The 176 equals the file count taken from develop under the Step 6 list. Step 8 "count 2" and both Step 10 findings matched exactly. Doom-named paths outside the list at develop are archive and protocol docs, `docs/battle-plans/S-DOOM-LAST-battle-plan.md`, `scripts/ci-fetch-doomgeneric.sh` (Phase 8) and two wiki pages (Phase 11): none added to the list.
- Phase 4A (Steps 24-26): lists 44 and 27, no MISSING; nothing stale against the forged list. Step 25 from develop `2b8da94d` (3,825 commits): 323 commits, 205 files, .git 102M. Step 26: leaks 0, largest object 22610957 (`raft/kingdom-w12/kingdom.lora.gguf.checkpoint-400`), 0 remotes. Beyond the gate: every path in history is on a list, 0 unreachable objects, one ref, 0 tags (`v0.1.0-alpha` is not an ancestor of develop, so it was never cloned), tip blobs match `2b8da94d` exactly. gitleaks counts 253 of 323 commits: the other 70 are 68 merges (none with content of its own) and 2 mode-only commits. A `gitleaks dir` scan of the tip also found 0. Phase 4A has no [C]; this line rides the next progress commit.

- Phase 2 (Steps 12-16): Step 12 `8`, `0`. Step 13 `rc=1`. Step 14 go tidy/build/test clean (count line empty, no non-ok lines), cargo `test result: ok. 46 passed`. Step 15 `1`. Step 16 `0`, committed unsigned in `~/tmp/projects/unheaded-doom`.
