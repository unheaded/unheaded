# Phase 2.2 finale — sh replaced (ush). Phase 2.2 COMPLETE — 2026-07-05

**Session**: 2026-07-05 (Fable 5, continued across a machine reboot from the 2026-07-04/05
coreutils session).
**Battle plan**: `docs/battle-plans/PHASE22-SH-REPLACE.md` (10 phases, 110 steps).
**Result**: SHIPPED. The last MIT program left the exec set — init, sh, echo, cat, wc, ls
are all Unheaded-authored. Zero eBPF change (ascend verifier 900,031 / 90.0% exact; Doom
737,087 + object loads green). Full harness 16 PASS, ALL GREEN, pre- and post-commit.

## Commits

| Commit | What |
|--------|------|
| `6e9b5850` | Commit 1 — behavior-lock gates N1-N3 in `scripts/upc-regression.sh`, landed GREEN against STOCK sh (previous session, same plan Phase 1) |
| `56d82cb4` | Commit 2 — the swap: `user/ush.c` + explicit `$T/sh.elf` Makefile rule + ADR-081 / master-plan ripples + the battle plan |
| (this)     | Commit 3 — session log + SHIPPED banner + handoff refresh |

## The root-cause narrative: why EXEC-only and malloc-free

Stock xv6 sh on the UPC was running on two accidents:

1. **The malloc landmine.** Stock's parser mallocs every cmd node (execcmd/pipecmd/…
   constructors). umalloc's `morecore` rides **sbrk(12)**, which has no arm in the ascend
   ecall dispatch — a silent no-op fall-through returning garbage a0 that umalloc treated
   as a heap pointer. The garbage happened to land in unused low VA, so stock sh *worked
   by accident*. ush allocates nothing: one static 100-byte line buffer (already bounded
   upstream by the 64-byte KBD ring), argv tokenized in place.
2. **Dead grammar.** pipe(4) and chdir(9) are the same silent no-ops, so PIPE / REDIR /
   LIST / BACK clauses and the `cd` builtin cannot function regardless of parser support.
   Design B (full parser port) would have shipped ~350 lines of dead, untestable,
   malloc-dependent code. Design A (EXEC-only, ~110 lines) is 100% corpus-covered.
   B is revisitable when pipe/chdir/sbrk handlers exist (Phase 2.3+ territory).

## Four-lens panel (pre-implementation)

- **Architect**: blast radius = one .c + one Makefile rule; behavior-lock gates land
  BEFORE the swap so the swap commit is provably identical-observable. Approved.
- **BlackMage**: largest input-facing surface in the guest → hard argc cap (8 = harvester
  ARGV_MAX), explicit metachar rejection, in-place tokenization only, no malloc, no new
  syscalls, panics child-side only. Approved with the Phase 7 checklist (all items pass
  on source review + 3 hostile probes).
- **Developer**: TDD inverted — gates must be GREEN against stock first (they lock
  behavior, not implementation), then the same gates prove equivalence through ush.
- **Scientist**: golden captures make "identical observable behavior" falsifiable
  byte-by-byte; predicted all three divergences in advance (see table).

## Method note: golden captures across a reboot

The goldens are session-temp (`/tmp/phase22-golden`) and the machine rebooted between the
Phase 1 commit and this session — so goldens were re-captured against stock sh by
temporarily reverting the Makefile rule (plan Appendix A7 mechanism: the rule is the only
wiring; `ush.c` is inert unwired), rebuilding `sh` + ramdisk from MIT `sh.c` (FILE symbol
verified `sh.c`), booting all 12 captures, then restoring the ush rule and rebuilding
(FILE symbol verified `ush.c`, `_start` at RV byte 0). Each capture stores the full boot
log, the TTY `ascii:` line, and the `forks=/waitpid=/tty_r=` stats.

## Gate evidence

**Byte-identical (TTY + stats) through OUR sh vs stock goldens:**
`cat README`, `cat BIGFILE`, `cat sub/NOTE`, `ls sub`, `echo hello`,
`echo peace and love`, `wc README`, uinit round-trip (banner `0xP1D1-0UR5` → our sh →
our echo), N1 `nosuch` → `exec nosuch failed`, N2 `echo alpha`+`echo omega` (forks=3,
waitpid=2 — prompt loop survives), N3 blank line skipped without fork (forks=2).

**The one diverging field**: root `ls` shows `sh 2 3 7496` vs stock's `sh 2 3 11472` —
sh's own file size inside fs.img. That field IS the replacement (ush is 35% smaller);
every other byte of the listing and the stats are identical. Harness `ls` gates grep
only the README/NOTE tokens, so this was predicted harmless in the plan.

## Divergence table (all invisible to the corpus, all safer than stock)

| Input | Stock sh | ush | Why ush is right |
|-------|----------|-----|------------------|
| `cd sub` | parent chdir(9) silently "succeeds" through the dead syscall | `exec cd failed` + fresh prompt (observed, Step 70) | honest failure beats silent lie |
| `echo a \| cat` | undefined — dead-pipe path through malloc garbage | `syntax` + fresh prompt (observed) | reject, don't reinterpret |
| 9+ args | parser accepts 10, harvester silently drops past 8 | `too many args` at 8 + fresh prompt (observed) | cap = the real ABI, said out loud |
| 63-byte max ring line | n/a (same both) | survives; 15-char arg truncation is the harvester's ARG_CAP, not ush (observed) | pre-existing ABI |

## Learnings / footguns confirmed

- The `boot()` helper + goldens pattern (full/tty/stats triplet) is cheap and reusable —
  candidate for future swap gates (Phase 2.3 kernel edges).
- Goldens in /tmp do not survive a reboot; if a swap sprint may span sessions, capture
  goldens FIRST and re-capturing requires a temporary revert of the swap wiring (A7).
- Stats parity (`forks=/waitpid=/tty_r=`) catches structural divergence (hidden extra
  fork) that TTY diffs alone would miss — blank-line skip proved no-fork via forks=2.
- `rv32i-to-mbc` on ush's sh.elf: 1,095 RV32I → 1,874 MBC (1.7x), 7,496 bytes + 1,096
  rv2mbc entries + 2 data records.

## Next

Per ADR-081 ladder: **Phase 2.3 — own kernel edges** (entry/start, console driver,
syscall dispatch), or the Track 1 cheap win (Epic 1.3.1 device inodes, first eBPF spend
since the freeze), or the verifier-headroom-reclamation spike required before Epic 1.3.2
file writes. See `~/tmp/next.md`.
