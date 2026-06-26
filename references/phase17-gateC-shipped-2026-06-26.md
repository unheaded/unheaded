# Phase 1.7 Gate C SHIPPED — Interactive sh on the UPC (2026-06-26)

**Status:** SHIPPED. Commits `86feadc7` → `fbc32697` on `main` (unsigned —
gpg-agent times out non-interactively; Stevie owns commit policy, session commits
authorized per the approved plan).

**Headline:** A scripted command line is read by sh, sh forks + exec's it, the
child exits, sh reaps ITS OWN child, and a fresh `$` prompt returns. The
fork→exec→wait→exit→prompt round-trip is the Gate C proof.

```
$ upc-bootctl boot ... --input $'echo\n'
  → TTY: "init: starting sh·$$"
  → STATS: tty_r=5 forks=2 waitpid=1 ctx_sw=4 num_proc=3 halted=0 rom_fault=0
```

## Root cause of the post-`$` halt (the prior diagnosis was half-wrong)

The prior session's "ROM-fetch-miss @ PC=0x81 / rom_fault=2" was a **red herring**
— those 2 `rom_fault`s were non-fatal JMPR/CALLR "skip" diagnostics, not the halt.

The decisive tool was a reason-tagged `halt_diag()` capture added at EVERY
`cpu.halted = 1` site, recording `{target, rv2mbc_base, current_pid, prev_pc,
reason}` into STATS slots 25-29 (printed by upc-bootctl as `ROM-FAULT CTX: …
halt_reason=0x..`, using the insert-or-update `set_stat` path so HashMap slots
actually land). It pinned the halt to **`halt_reason=0x47` = MRET-unmapped**,
`mepc=0x27304`, `rv2mbc_base=0x1000`, `priv` returning to 1 (supervisor).

### Bug 1 — MRET used the user rv2mbc base for a kernel return

After `exec("sh")` set sh's per-process `cpu.rv2mbc_base = 0x1000`, a machine-mode
trap (timer) fired while SUPERVISOR (kernel) code ran. The MRET return target
`mepc` is a KERNEL RV address (translated at base 0), but the lookup added sh's
USER base 0x1000 → unmapped MBC index → halt. The single `cpu.rv2mbc_base` field
conflated kernel-vs-user translation.

**Fix:** `rv2mbc_branch_base(cpu)` returns the per-process base only in user mode
(`priv_level == 3`), kernel base 0 otherwise — mirroring `translate_address()`'s
own per-process-only-in-user rule. Applied at all 5 indirect-branch sites
(JMPR/CALLR/RET/MRET/SRET). Raw `rv2mbc_base_of()` is retained for ctx-switch
save/restore (which must persist the user base). Doom build: compile-time 0,
byte-identical.

### Bug 2 — wrong user entry → reboot loop

Fixing Bug 1 exposed a reboot loop. A new generic post-boot reset-to-0 guard
(`if cpu.pc == 0 && insn_count > 1M`) caught `prev_pc=0x6028` = sh's instruction
RET'ing to `ra = 0` → PC=0 → re-run start_mbc → reboot.

Disassembling `sh.mbc[0x28]` showed a `RET`, and `riscv64-...-nm -n sh.elf` showed
address 0 = **`getcmd`**, not `main`. The user link omits `-e main` (real
xv6-riscv uses it) and `user.ld` has no ENTRY directive, so the ELF entry
defaulted to the first source function. init.c's first function is `main` (so init
worked by luck); sh.c's is `getcmd`. exec jumped into getcmd, it printed `$` (its
`write(2,"$ ",2)`), then RET'd to ra=0.

**Fix:** `crates/xv6-mbc/adapters/crt0_mbc.S` — a `_start` (`call main; exit`)
placed in a `.text._start` section that `user.ld` emits FIRST, plus
`ENTRY(_start)`, linked ahead of every program object. Fixes ALL programs
uniformly. (ulib.c already had a `start` crt wrapper, but it was not at address
0, so it was never the entry.)

## The 4 plan stages

Plan: `~/.claude/plans/continue-churning-the-unheaded-tidy-hennessy.md`.

| Stage | Change | Commit | Verified by |
|-------|--------|--------|-------------|
| 1 | console `read(5)` BLOCKS (pc-=1; break) instead of 0=EOF | `9d2f40f9` | `$` once, sh spins in read, no re-fork cycle |
| 2 | KBD ring drain (KBD_HEAD/KBD_TAIL, O(1), H2) + `--input` (`write_kbd`, ≤8B, L1) + bounded `user_phys()` (H1) | `1df6d976` | `--input $'\n'` → tty_r=1, second `$` |
| 3 | PARENT_OF map: wait() reaps only DIRECT children + M1 reparent-on-exit + M2 skip-self | `fbc32697` | (with Stage 4) |
| 4 | register echo/ls/cat/wc in PROGRAM_TABLE loader | `fbc32697` | `--input $'echo\n'` → forks=2, waitpid=1, `$$` |

Stage 3 fixes the **grandchild-reap bug**: the old wait scan reaped "any halted
pid > me", so init (pid 0) stole sh's child and sh's wait hung forever (the
long-standing "gate_nway halt ~insn 5354501" class). Now each process reaps only
`PARENT_OF[p] == me`, skipping self.

## Hardening (BlackMage review, folded into the stages)

- **H1** bounded `user_phys(pid,va,len)`: overflow + 8 MiB slice-ceiling checks →
  EFAULT, so a user buffer VA can't escape its pid slice into another process, the
  ramdisk, staged .data, or the guest page tables. ✅
- **H2** KBD ring (not 8-slot scan): O(1) drain, verifier-budget-safe. ✅
- **M1** reparent orphans to init on exit (no zombie leak). ✅
- **M2** skip-self in wait scan + write PARENT_OF on every fork (pid-reuse safe). ✅
- **M3** block-on-empty yield: NOT added — benign no-op in current flows (sh reads
  only when it is the sole runnable user proc; it reads BEFORE forking, then
  wait() yields). Left as a noted refinement.
- **L1** `--input` pre-fill only, errors (no silent truncation) past 8 bytes,
  read does not echo. ✅

## Verification

- **gate2** → `P: ISOLATION-PASS` (clean). My changes don't touch slice/MMU
  translation.
- **gate_nway** → `NWAY-FAIL pid2`, but **PROVEN PRE-EXISTING**: rebuilt and ran
  the pre-session `685ecf4f` eBPF object against the same gate_nway.mbc — identical
  `NWAY-FAIL pid2` (149×). It is the documented N-way concurrent-fork substrate
  limitation in gate_nway.c, not a regression from the Stage 3 wait/parentage
  changes.
- **Doom (non-ascend, G3)**: builds clean; rv2mbc_branch_base / user_phys are
  compile-time-0; PARENT_OF/KBD additions sit in xv6-only syscall handlers.
- Both configs load under the **1M BPF verifier ceiling** (all boots succeed; no
  "BPF program is too large").

## Deferred — "Phase 1.7 FS reader" gate (separate, larger)

Gate C ships the lifecycle witnessed by the returning prompt, using `echo`
(arg-less, no output). NOT in scope here:
- `fstat(8)` minimal console stat (cat needs it).
- inode-backed `open`/`read` against fs.img (loaded at RAM byte 0x00800000) so
  ls/cat produce real output.
- argv-on-stack in `exec(7)` so execed programs see argc>0.

ls/cat are already registered — they resolve, then block on their first FS read
(expected).

## Files touched

- `ebpf/monad-cpu-ebpf/src/main.rs` — rv2mbc_branch_base, user_phys, KBD ring
  drain, PARENT_OF wait/fork/exit, set_stat/halt_diag diagnostics, reset guard.
- `cmd/upc-bootctl/src/main.rs` — `--input` flag, halt_reason print, register
  echo/ls/cat/wc.
- `cmd/upc-bootctl/src/runner.rs` — `write_kbd()`.
- `crates/xv6-mbc/adapters/crt0_mbc.S` (new) — user `_start` crt.
- `crates/xv6-mbc/upstream/user/user.ld` — `ENTRY(_start)` + `.text._start` first.
- `crates/xv6-mbc/adapters/Makefile.mbc-userland` — link crt0 first.
