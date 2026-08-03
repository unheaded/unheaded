<!--
SPDX-License-Identifier: GPL-3.0-or-later
Copyright (c) 2024-2026 Stevie Bellis.
-->

# Staging Ladder — Live Progress Log

Execution log for `docs/battle-plans/STAGING-LADDER-2026-08-03.md`.
**Stevie: read this first.** One row per completed phase, in ladder order.

**Branch**: `staging` (from `develop` @ `b39fb207`)
**Started**: 2026-08-03, overnight autonomous run
**Signing**: gpg-agent has no cached key overnight → commits are `--no-gpg-sign`.
**Every commit below needs re-signing before it crosses into `main`** (ADR-089 §exit gate).

---

## KNOWN FAILURES BASELINE (recorded before any edit)

**`next.md` claims "979 tests pass, 0 failed" at `b39fb207`. That is not reproducible.**
Measured on this machine at `b39fb207` with only documentation files added:

| Package | Failures | Cause |
|---|---|---|
| `cmd/wiki-server` | 4 | **Stale tests.** `879c91cf` ("drop redundant Home list entry") intentionally changed `listPages` to skip `README.md`; the tests still assert the old behaviour. Product code is correct. |
| `cmd/dashboard-backend/internal/server` | 2 | `websocket: bad handshake` — under investigation |

Also emitted (not a failure, expected without a DB):
`Postgres unavailable — health persistence disabled: password authentication failed`.

These 6 failures are **pre-existing** and are not caused by this plan. Every later phase
compares against this baseline, not against zero.

### Other baselines at `b39fb207`

| Linter | Baseline |
|---|---|
| ruff | 427 |
| bandit | 212 (0 HIGH / 51 MEDIUM / 161 LOW) |
| shellcheck | 350 (0 error / 98 warning / 252 note) |
| eslint | unmeasured — no config, no `package.json` (**now 57**) |
| clippy | 0 (gating) |
| `monad-cpu-ebpf --features ascend-linux` | 901,888 bytes |

---

## Commits on `staging`, in ladder order

| # | Rung | Phase | Commit | What | Baseline delta | Signed |
|---|---|---|---|---|---|---|
| 1 | — | 0 | `e21aea4b` | Plan, ADR-089, ADR-090, progress scaffold | — | no |
| 2 | R0 | 1 | `63f81e93` | Untracked file disposition | — | no |
| 3 | R0 | 2 | `0584650e` | Notebook repair + notebook syntax gate | **ruff 427 → 335** | no |
| 4 | R1 | 1.5 | `e634508e` | wiki-server stale test assertions | failures 6 → 2 | no |
| 5 | R1 | 1.5 | `cc0e0390` | dashboard-backend WebSocket Origin header | **failures 2 → 0** | no |
| 6 | R0 | 3 | `4c46d279` | Shebang/exec-bit reconciliation + dead shebang repair | ruff 335 → 302 | no |
| 7 | R1 | 4a | `2542d3fc` | shellcheck SC2155 declare-and-assign split | **shellcheck 359 → 343** | no |
| 8 | R1 | 5 | `011b1f50` | explicit `check=False` on subprocess.run | ruff 302 → 272 | no |
| 9 | — | — | `e9692dab` | progress log through Phase 5 | — | no |
| 10 | R1 | 4b | `ff8d090a` | shellcheck SC2034, consumers verified first | **shellcheck 343 → 281** | no |
| 11 | — | — | `39c19b19` | progress log through Phase 4b | — | no |
| 12 | R1 | 6 | `6218e9bf` | timezone-aware datetimes, no clock moved | ruff 272 → 254 | no |
| 13 | R2 | 7A | `a11894b9` | notebook unused imports + dead f-prefixes | ruff 254 → 243 | no |
| 14 | R2 | 7A | `1af220d2` | F841 / RUF059 / PLR1722 | ruff 243 → 228 | no |
| 15 | — | — | `c2ad00c9` | progress log through Phase 7A | — | no |
| 16 | R2 | 7B | `8d07d33d` | PIE810 / ISC004 / C401 / PERF102 / SIM101 / TRY201 / RUF013 | ruff 228 → 210 | no |
| 17 | — | — | `94a87a16` | progress log through 7B | — | no |
| 18 | — | — | (sha fix) | correct a SHA in the table | — | no |
| 19 | R2 | 7C | `9ed74515` | module logger, read-only globals, TRY004 trap | ruff 210 → 189 | no |
| 20 | R2 | 7C | `896f302e` | SIM115 / SIM102 / TRY002 | ruff 189 → 180 | no |
| 21 | R2 | 7 | `90307f1b` | noqa-comment trap + stale binding | ruff 180 → 176 | no |
| 22 | — | — | (log) | progress log through Phase 7 | — | no |
| 23 | R2 | 8a | `fb5d7091` | **E722 → 0** — bare except no longer eats Ctrl-C | ruff 176 → 170 | no |
| 24 | R2 | 8b | `6618561e` | **S110 → 0** — silent swallows now visible | ruff 170 → 146 | no |
| 25 | R2 | 8c | `725b4e3d` | BLE001 triaged (not fixed) — decision doc | — | no |
| 26 | R3 | 13 | `bbfb4806` | async blocking calls annotated w/ expiry condition | ruff 146 → 139 | no |
| 27 | R2 | 9/10 | `71f43a11` | B105 false positive + env URL scheme guard | bandit 212 → 183 | no |
| 28 | — | — | (log) | progress log through Phase 9 | — | no |
| 29 | R2 | 9 | `e4461566` | B108 /tmp paths dispositioned per interface | bandit 183 → 173 | no |
| 30 | R2 | 9/10 | `70b917da` | bandit group disposition + CI severity-filter finding | 43 left to decide | no |
| 31 | — | — | (log) | progress log through Phase 9/10 | — | no |
| 32 | R3 | 11 | `2c760ab7` | **eslint measured for the first time** + a real bug fixed | eslint → 57 | no |
| 33 | — | — | (log) | progress log through Phase 11 | — | no |
| 34 | R3 | 12 | `700b80c0` | **FillRing had the same wraparound bug**; 3 red-then-green tests | — | no |
| 35 | — | — | (log) | progress log through Phase 12 | — | no |
| 36 | — | 14 | `6925c775` | **ruff GATING at 0**; bandit loses its `-ll` severity filter | **ruff ratcheted** | no |
| 37 | — | 15 | `315ad4cd` | decision queue — 10 items costed for Stevie | — | no |
| 38 | R1 | post | `93c42495` | **SC2086 → 0** — one real quoting bug in a credential path | shellcheck 281 → 246 | no |
| 39 | — | — | (log) | progress log through SC2086 | — | no |
| 40 | R1 | post | `c5133060` | stderr escaping a soak log; 3 traps; 2 bare `cd` | warnings 20 → 14 | no |
| 41 | R1 | post | `2cd73ed3` | **shellcheck now GATING at warning level, 0 exclusions** | warnings 98 → 0 | no |
| 42 | — | — | (log) | progress log through the shellcheck flip | — | no |
| 43 | R2 | post | `2769cbdb` | **eslint 57 → 0 and GATING**, 0 exclusions | **eslint → 0** | no |

**Current state**: `go test ./...` = 244 packages ok, **0 failures**.
**ruff 427 → 139 (−67%)** · **bandit 212 → 173**, and only **43** survive the proposed rule-ID skip · **shellcheck 359 → 281 (−22%)** · clippy still 0 · bandit untouched at 212.

---

## Findings worth Stevie's attention

1. **`next.md`'s "979 tests pass, 0 failed" was not reproducible.** Six tests failed at
   `b39fb207`. All six were stale tests that had never been updated when the production
   behaviour they assert was intentionally changed — four by `879c91cf` (wiki nav), two
   by the WebSocket origin hardening. No product bugs. The suite is green now.

2. **`notebooks/02_hypothesis_matrix.ipynb` has never been runnable.** Three code cells
   fail to parse, and they fail to parse in `3dbd7eee`, the only commit that ever touched
   the file. It was committed without being executed. Twenty two-line plot labels were
   written with real newlines inside single-quoted strings.

3. **A GATING check was blind to a whole artifact class.** `check-python-syntax.sh` only
   globbed `*.py`, so it reported "all 70 files parse" while a tracked notebook in the
   tree could not. This is the same failure shape the script was originally written to
   catch, one artifact class over. Now extended to `*.ipynb`, verified red-then-green.

4. **One broken quote was 24% of the ruff backlog.** Syntax errors cascade in the parser:
   3 unterminated strings generated 104 of 427 findings. The ruff number was never as bad
   as it looked.

5. **`scripts/bulk_inject.py` could never have run.** Its first line was
   `#\!/usr/bin/env python3` — bytes `23 5c 21`, a backslash-escaped `!` that leaked in
   from a shell heredoc written to dodge history expansion. `#\!` is not a shebang, so
   the kernel handed the file to `/bin/sh`, which tried to execute the docstring as a
   command and resolved `import` to ImageMagick. The file is mode 775, so someone
   intended it to be executable. Repaired and verified.

6. **The shellcheck baseline in CI is stale.** `static-analysis.yml`'s header records
   361 findings with 11 errors. Measured today: 359, with **0** errors. The errors were
   cleared at some point without the header being updated. Worth fixing when the ratchet
   lands in Phase 14, since that header is what future sessions measure against.

7. **A CI gate does not check the thing it computes.** `scripts/bpf-verifier-check.sh`
   captures `BUILD_EXIT=$?` after `cargo build --release` and an `ERRORS` count of
   `^error[` lines, then reads **neither**. Only `LINK_ERRORS` feeds `FAILURES`. A BPF
   program that fails to compile with an ordinary `error[E0433]` leaves this gate
   reporting success. Both variables were kept and annotated rather than deleted — they
   are the only evidence the check was intended. **Wiring it in can turn CI red
   immediately, so it needs your call, not mine.**

8. **Two advertised CLI flags do nothing.** `tomb/provision.sh --verbose` and
   `scripts/pre-flight-check.sh --strict` are both documented in their usage text,
   parsed into a variable, and never read. Accepted silently, no effect. Flagged rather
   than deleted, because the bug is the missing behaviour, not the assignment.

9. **`scripts/doom-test.sh` prints a variable that is never assigned.** The SCREEN_MAP
   sample line renders `${pixel_8000}`, which nothing sets, so it always shows `??`. The
   declaration named `pixel_32000` — a third sample read that was intended and never
   written. Cosmetic, but it means that diagnostic has never shown real data.

10. **Vendored code has been edited before.** `5b80e288` applied ruff autofixes to
   `crates/xv6-mbc/upstream/test-xv6.py`, which is vendored xv6-riscv from `28d5ac98`.
   Not reverted (harmless, and reverting adds churn), but the Phase 14 ruff ratchet
   should exclude `crates/xv6-mbc/upstream/` by path so upstream findings stop counting
   as ours. This is also open question 1 in ADR-090.

---

## Deviations from the plan as written

- **H5 falsified at Phase 0.** The plan assumed a green suite; it was not green. The
  KNOWN FAILURES BASELINE above is the corrected starting point.
- **Phase 1.5 added** (rung R1, test-only) to fix the six stale tests, since they were
  cheap, genuinely low-risk, and blocking any honest "no regressions" claim later.
- **H3 partially falsified.** The plan said all 104 invalid-syntax findings were one
  notebook *cell*; it was one notebook, three cells, twenty sites. The plan's Step 26
  debug branch covered exactly this and was followed.

---

## The ladder is complete

Phases 0-15 are done. Phase 16 (handoff) is `~/tmp/next.md`.

What is left is **not work, it is decisions** — 10 of them, costed in
`docs/battle-plans/STAGING-LADDER-DECISIONS.md`. The largest is D1 (134 blind excepts,
71 of them silent), which is parked behind the ruff ratchet's one rule-ID exclusion and
is best done as the first exercise of the ADR-090 sweep.

## Where the run stopped

Phases 0-6, 4b, and Phase 7 Batches A+B complete. **Next up: Phase 7 Batch C**
(LOG015 9, PLW0602 8, SIM115 5, SIM102 3, TRY002 3, TRY004 1, UP031 1, PLC0206 2,
SIM103 2 — ~34 sites), then the big one, Phase 8 (exception handling:
BLE001 123 + S110 30 + E722 11 = **164 sites**, ~38% of the original backlog).

**Phase 8 is the one that will want your eye.** Every site is a judgement about what
an `except Exception:` is allowed to swallow, and there is no autofix. The plan's
approach is a per-site worksheet at
`docs/security/exception-handling-triage-2026-08-03.md` — a row per site with what the
try body can raise and the chosen disposition (narrow / keep-but-log / keep-annotated)
— written *before* any edit, so the commit is reviewable as reasoning rather than as
164 scattered diffs.

Remaining ruff by rule (228 total, 3 of them in vendored xv6 upstream):

```
123 BLE001   blind-except            <- Phase 8  (the 4h one)
 30 S110     try-except-pass         <- Phase 8
 11 E722     bare-except             <- Phase 8
  9 LOG015   root-logger-call        <- Phase 7C
  8 PLW0602  global-not-assigned     <- Phase 7B
  7 PIE810   multiple-startswith     <- Phase 7B
  5 ASYNC230 blocking-open-in-async  <- Phase 13
  5 SIM115   open-without-context    <- Phase 7B
  4 ISC004   implicit-str-concat     <- Phase 7B
  3 SIM102   collapsible-if          <- Phase 7B
  3 TRY002   raise-vanilla-class     <- Phase 7C
      ... remainder single-digit
```

### Two near-misses worth knowing about

Both were caught by verification, not by review, which is the argument for the
`[V]` gates being non-optional:

- Deleting SC2034 sites **by line number** removed a `for attempt in ...; do` loop
  *header* in `phase3b-nixos-lxd.sh`, orphaning its body. `bash -n` caught it.
- The same pass deleted `local pixel_0 pixel_100 pixel_32000`, where only the third
  was unused — silently promoting two live variables to globals. An audit of every
  deleted line against "is this a plain assignment?" caught it.

Similarly in Phase 5, anchoring an AST insertion on the closing paren orphaned
`, check=False)` onto its own line in five multi-line calls; the notebook-aware
syntax gate from Phase 2 caught all five. **That gate has now paid for itself
three times in one session.**

## Stuck items

_none._ No phase has hit the Skip Protocol.

## Promotion, when you're ready

Each commit is independently revertible and reviewable in ladder order. Review from
the bottom rung up; you can stop anywhere and the tree is still whole.

```bash
cd ~/tmp/unheaded
git log --oneline develop..staging          # the ladder, oldest first at the bottom
git show <sha>                              # review one rung
```

**Before anything crosses into `main`, re-sign it** — every commit above was made with
`--no-gpg-sign` because gpg-agent had no cached key overnight (ADR-089 §exit gate):

```bash
ssh-add ~/.ssh/id_ed25519
git rebase --exec 'git commit --amend --no-edit -S' b39fb207
```
