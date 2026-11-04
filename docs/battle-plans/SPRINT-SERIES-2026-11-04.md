<!--
SPDX-License-Identifier: GPL-3.0-or-later
Copyright (c) 2024-2026 Stevie Bellis.
-->
# Sprint series, 2026-11-04: the decision pack rolled into battle plans

Source of every decision: `docs/battle-plans/DECISION-PACK-2026-11-03.md` (DP ids; read the "Answers recorded" sections, they override the prompts' recommendations). Stevie's word of 2026-11-04 lifts ADR-112's freeze for exactly these plans. New ideas found while executing become parked `Proposed` ADRs, never scope.

## The plans

| Plan | Mode | Lane | Answers it carries |
|---|---|---|---|
| S-CLEAN-CUT | unattended | A | DP-401, 205 (verify Doom first), 209, 210, 306, 307, 407, 409, 402 Q3, 410, 706, 709, 713 (repo side), 308 corpus flags |
| S-MOVE-OUT | unattended, local only | A, after S-CLEAN-CUT | DP-305 Doom to `unheaded-doom`, DP-17 k8s to `unheaded-k8s`; DP-314 |
| S-SUPPLY-FIRST | unattended | B | DP-722, 719, 415, 405, 08, 721 stage 1 |
| S-ZHEN-POLISH | unattended | C | DP-216, 302, 308 GGUF, 309, 311, 312, 712 (code and units, not install), 608 prerequisite `answer_start` |
| S-WIRE-HONEST | unattended | D | DP-211, 408, 411, 412, 716, 715 |
| S-G9-FOLD | unattended, ADRs first | D, after S-WIRE-HONEST | DP-203: one parked ADR per fold, compliance first |
| S-STAGE-2B | unattended, serial | E | DP-213, 413, 414, 501, 502, 503, 717, 206 prep |
| S-LOG-HARVEST Phase 5 | unattended, existing plan | C, after S-ZHEN-POLISH | native syslog relay, Steps 105-140 |
| S-HOST-TIDY | unattended, live hosts, reversible only | coordinator | DP-09, 11 (password + fail2ban, not the port), 12, 15 (`PermitRootLogin no`), 25 (safe rows), 708, 711, 723 follow-up |
| S-E1-LIVE | ATTENDED | Stevie present | E1 stack up, DP-23, 605, 603 (103, 106), 22, 04, 15 keys, 11 port, 06 ceremony, 14, 03 later, 710, 303, 505, 701, 602, 604, 606, 07 |

## Rules every plan and every agent follows

1. **Branches.** One branch per plan, `sprint/<plan-lowercase>`, in its own worktree under `~/tmp/wt/<plan>`. Merge to develop with `git merge --no-ff -S`. Never `git push`: Stevie pushes.
2. **Commits.** Signed (`-S`); if gpg-agent times out use `--no-gpg-sign` and say so in the handoff, not in the commit message. Stage named files only, never `git add -A <dir>`. Message format `[PLAN <name>] Steps X-Y: ...`, then the trailers `Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>` and `Claude-Session: https://claude.ai/code/session_01TzL3Tqe5ebLDopsg5ZkZdp`.
3. **Capacity.** West has 12 cores and 14 GiB. At most three lanes build at once; cargo with `-j4`, `GOMAXPROCS=4`. `/tmp` is a 7.2 GiB tmpfs: put build output under `/var/tmp` or the worktree. Never count swap. `Makefile.mbc clean` nukes `target/`.
4. **No third party.** No new module, crate, package, action or image without Stevie's confirm (2026-10-29 rule). Version bumps for vulnerabilities are approved (DP-722).
5. **Attended means attended.** No firewall apply, no sshd port or KEX change, no key generation for custody, no `compose up` of the stack, no reboot. Those live in S-E1-LIVE.
6. **Gates.** Go: `gofmt -l`, `go vet`, `go test -race`, `./scripts/check-gosec-ratchet.sh` (never `~/go/bin/gosec`). `./scripts/check-gates-can-fail.sh --only <gate>` needs a clean tree. Baselines only shrink. eBPF changes re-run the verifier and insns gates. Wire format is frozen at v0x01: anything touching it passes the RFC desk first.
7. **Writing.** Never mention Stevie's job, employer or work devices. No LoC figures, no sales taglines, no "free to use" preambles. Credentials never in the repo. Ops facts also go in `wiki/`.
8. **Stuck.** Skip Protocol: 3x the estimate or two failed debug attempts, mark `[STUCK]`, commit, move on. Questions go to the plan's progress file under "For Stevie", oldest first; never stop to ask.
9. **Handoff.** Each plan keeps `docs/battle-plans/progress/<PLAN>-progress.md`; the coordinator folds it into `~/tmp/next.md`.

## Detail standard (Stevie, 2026-11-04: detailed enough that Sonnet can run the well-defined items)

10. **Every phase carries a model tag.** `**Model**: Sonnet` when the steps are mechanical and every decision is already made; `**Model**: Opus` when a step needs judgement (eBPF verifier work, protocol, concurrency, security-sensitive logic, design calls). Default to Sonnet only if an agent with no project context could finish the phase from the plan alone.
11. **Sonnet phases leave nothing to infer.** Each step names the exact file and the exact text to find and to write, shows the full command and its expected output, and says what to do on any other output. For code, give the function signature, the test to write first with its assertions, and the expected red and green results. No "update as needed", "similar to", or "etc.".
12. **Stop lines.** A Sonnet agent that meets anything the plan did not predict does not improvise: it marks the step `[STUCK]`, records the output, and moves to the next independent step.

## Order

Wave 1, in parallel: S-CLEAN-CUT, S-SUPPLY-FIRST, S-ZHEN-POLISH, S-WIRE-HONEST (three builders at once; S-STAGE-2B takes a slot when one frees). Wave 2: S-MOVE-OUT, S-G9-FOLD, S-LOG-HARVEST Phase 5. S-HOST-TIDY runs from the coordinator between merges.
