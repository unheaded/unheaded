# Phase 1.7 Gate D — FS Reader Battle Plan

**Date**: 2026-06-28
**Sprint**: Phase 1.7 Gate D — inode-backed `open`/`read`/`fstat` over xv6 `fs.img` + argv-on-stack in `exec(7)`
**Prerequisite**: Gate C shipped (`e24abeb9`, interactive `sh`); tree clean on `main`
**Target**: `ls` lists the real root dir, `cat README` prints real file text, `echo hello` prints `hello` — zero regression to Gate A/B/C
**Estimated Duration**: 6–10 hours (1–2 sessions)
**Agent Strategy**: Sequential (each phase gates the next); P1/P2 are pure-logic (no boot), P3–P6 need build+boot
**Commit Cadence**: Commit after each phase exit gate (Stevie owns final commit policy; unsigned OK if gpg-agent times out)
**Stuck Protocol**: Skip after 3× time estimate or 2 failed debug attempts; log STUCK marker; preserve known-good state

---

## SITUATION

Gate C ships the `fork→exec→wait→exit→prompt` lifecycle witnessed by the returning `$`,
using `echo` (arg-less, no output). Three pieces were deferred to **this** gate:

1. **argv-on-stack in `exec(7)`** — programs see `argc>0` (so `echo hello` → `hello`).
2. **inode-backed `open`/`read`** against `fs.img` — so `ls`/`cat` print real data.
3. **`fstat(8)`** — so `ls` can stat directory entries.

### Architecture (verified against code, not assumed)

Under `--features ascend-linux` the eBPF CPU **intercepts every user syscall directly** in
the live `op::SYSCALL` (ecall) dispatch at `ebpf/monad-cpu-ebpf/src/main.rs:1891+`, keyed by
**xv6 syscall numbers** (fork 1, exit 2, wait 3, read 5, exec 7, getpid 11, dup 10, open 15,
mknod 17, close 21, write 16). The kernel's real `fs.c`/`sysfile.c`/`file.c`/`bio.c` are
compiled (`Makefile.mbc` OBJS) but **unused on the user-syscall path** — the ecall never
reaches the kernel's `syscall()` table. The `open`/`read`/`fstat` stubs at `main.rs:1647+`
live in the **`INT 0x80` block that is dead-code-eliminated under ascend** (`main.rs:1301`).
They are NOT the live path.

**Current live state**: `open(15)`@2359 returns a console fd for every path (no inode);
`read(5)`@2418 serves only `FD_CONSOLE` (KBD ring), file fd → `-1`; `fstat(8)` is **not
handled under ascend at all**; `exec(7)`@2471 zeroes all regs → `a0=argc=0`.

**Decision — Design A: continue eBPF-native syscall interception.** Add a bounded,
re-entrant `fs.img` walker to the ascend ecall handlers. (Design B — delegate to the kernel
FS via `blk-ramdisk`→`SYS_READ_BLOCK` — would reverse three shipped gates, depend on the
unproven kernel trap→syscall→sleeplock→buffer-cache path, and still require adding
`SYS_READ_BLOCK` to the ascend ecall path, since it too is only in the dead INT-0x80 block.)

**Isolation**: the walker is kernel-trusted, reads `fs.img` from `RAM_MAP` (identity
region), and its only user-controlled output goes through `user_phys()` (H1 per-pid bound).

---

## LEGEND

`[B]` bash · `[V]` verify (must pass) · `[D]` debug (only on fail) · `[W]` write file ·
`[R]` read · `[S]` sudo · `[CODE]` implementation · `[TEST]` test · `[C]` commit checkpoint ·
`[SEC]` security/hardening review · `[GATE]` phase exit gate

---

## CORRECTNESS REFERENCE — `fs.img` address arithmetic (PROVE in `fs_walk.rs` tests)

**THE #1 TRAP:** xv6 `fs.h` `BSIZE = 1024` (mkfs includes fs.h → `fs.img` is in 1024-byte
blocks). `monad_common::mbc_block` (`BLOCK_SIZE=512`, `WORDS_PER_BLOCK=128`) is the UPC
*sector* abstraction used by `blk-ramdisk.c` — **NOT** used by the Design-A walker.

```
FS_WORDS_PER_BLOCK = 256                 // 1024 / 4
RAMDISK_BASE_WORD  = 0x200000            // fs.img loaded at byte 0x00800000
word_of_block(B)   = 0x200000 + B*256
IPB = 1024/64 = 16 ; dinode = 16 words ; NDIRECT=12 ; NINDIRECT=256
ROOTINO=1 ; FSMAGIC=0x10203040 ; DIRSIZ=14 ; dirent=16 bytes (64/dir-block)
```
- superblock @ block 1 → word `0x200000+256`; fields magic,size,nblocks,ninodes,nlog,
  logstart,**inodestart**,bmapstart (8 × u32).
- inode `i`: block `inodestart + i/16`; within-block word offset `(i%16)*16`.
- dinode words: w0=`type|major<<16`, w1=`minor|nlink<<16`, w2=`size`, w3..w15=`addrs[0..12]`.
- data block for file offset `off`: `fbn=off/1024`; `fbn<12` → `addrs[fbn]`; else read
  indirect block `addrs[12]`, index `(fbn-12) < 256`.
- dirent: w0 low16 = `inum` (0 = free), bytes 2..15 = `name[14]` (NOT NUL-terminated).

---

## VERIFIER BUDGET — binding constraint (1M processed-insn ceiling at load)

`exec(7)` already sits near 1M. **Every** new loop MUST:
- be re-entrant ≤16 iters/tick via `cpu.pc -= 1; break` (templates: exec-copy @2554,
  `SYS_READ_BLOCK` @1533, `SYS_FLUSH_TLB` @1700);
- never call `translate_address()` in a loop — direct phys only (`0x200000+B*256+off` for
  fs.img; one `user_phys(pid,va,len)` call for the user buffer);
- resolve the path basename once (exec FNV-1a @2493 is the template);
- be budget-checked after each phase. **FOOTGUN:** `scripts/bpf-verifier-check.sh` runs a
  plain `cargo build --release` (no `--features ascend-linux`) → it CLOBBERS the shared
  target object to the 136-byte struct. After every budget check, **rebuild ascend** before
  any boot.

---

## HARDENING (BlackMage) — fold into every FS handler + `fs_walk` hostile-input tests

`RAM_MAP` is the entire 64 MiB phys space → an unchecked block address = arbitrary
kernel-memory read funneled into a user buffer = isolation break.
- **H-FS1 block-addr bound:** every block number `< TOTAL_BLOCKS` and word within
  `[0x200000, 0x200000 + RAMDISK_SIZE/4)`.
- **H-FS2 inum bound:** `0 < inum < ninodes`; inode block lands in the inode region.
- **H-FS3 superblock sanity:** `magic == 0x10203040` or fail all opens; clamp fields.
- **H-FS4 indirect bound:** single indirect only; index `< 256`; indirect addr per H-FS1.
- **H-FS5 path bound:** read ≤ `PATH_CAP` bytes; `path_va < USER_VA_CEILING`.
- **H-FS6 user_phys dest guard:** `read`/`fstat` copy-out via `user_phys(pid,va,len)`.
- **H-FS7 offset/size:** clamp read len to `min(n, size-offset)`; no wrap; never past `size`.
- **H-FS8 dirent names:** compare exactly 14 bytes; skip `inum==0`.

---

## KNOWN-GOOD BASELINE (record at session start, before code changes)

- [ ] **Step 1** [B][S]: gate2 boot → expect `P: ISOLATION-PASS`.
- [ ] **Step 2** [B][S]: gate_nway boot → expect `NWAY-FAIL pid2` (PRE-EXISTING, not a regression).
- [ ] **Step 3** [B][S]: Gate C `--input $'echo\n'` → expect `forks=2 waitpid=1 tty_r=5 halted=0`.
- [ ] **Step 4** [B]: Doom (non-ascend) builds: `cd ebpf && cargo build --release -p monad-cpu-ebpf`.
- [ ] **Step 5** [B]: budget baseline: `bash scripts/bpf-verifier-check.sh` (then rebuild ascend — footgun).
- [ ] **Step 6** [C]: baseline recorded in session log; **rebuild ascend LAST** before any further boot.

---

## PHASE 0 — Preflight & demo fixture

**Goal**: doc persisted, baseline recorded, `README` in `fs.img`. **Agent**: Solo.

- [ ] **Step 7** [W]: this doc (done).
- [ ] **Step 8** [R]: re-confirm `main.rs` handler lines (open@2359, read@2418, exec@2471, INT-0x80@1301, ecall@1891).
- [ ] **Step 9** [W]: in `crates/xv6-mbc/adapters/Makefile.mbc-userland` ramdisk target, create
  `target/README` (short host-authored ASCII, e.g. the Gate-D proof line) and append to mkfs:
  `mkfs fs.img init sh ls cat echo wc README`.
- [ ] **Step 10** [B]: rebuild ramdisk: `cd crates/xv6-mbc/upstream && make -f ../adapters/Makefile.mbc-userland ramdisk`.
- [ ] **Step 11** [V][GATE]: `README` present in `fs.img` (mkfs output lists it); baseline green.
- [ ] **Step 12** [C]: commit `chore(upc): Gate D preflight — README fixture + battle plan`.

---

## PHASE 1 — `fs_walk` pure-logic module + off-target TDD ✅ DONE (2026-06-28)

**Goal**: prove the address arithmetic with zero eBPF/budget impact. **Agent**: Solo.

> **TEST-HOME CORRECTION (empirical, this session):** the ebpf *bin* crate's inline
> `#[cfg(test)]` tests (`fdtable.rs`, `phase12.rs`) **cannot actually run** — `ebpf/.cargo/config.toml`
> sets `[unstable] build-std=["core"]`, which poisons any host `cargo test` from inside `ebpf/`
> (`duplicate lang item core: sized`), and `main.rs` is unconditionally `#![no_std]/#![no_main]`.
> The working off-target harness is **`monad-common`** (`#![cfg_attr(not(test), no_std)]`), tested
> from the un-poisoned `crates/monad-mbc` workspace. So `fs_walk` lives in **`monad-common`** (the
> real ebpf path calls `monad_common::fs_walk::*`), with inline tests run via
> `cd crates/monad-mbc && cargo test -p monad-common`.

- [x] **Step 13** [W][TEST]: `ebpf/monad-common/src/fs_walk.rs` with inline `#[cfg(test)] mod tests`
  — happy paths + hostile inputs H-FS1/2/3/4/8 (inum 0/oob, block oob, sparse hole, indirect≥256,
  bad magic, 14-byte names).
- [x] **Step 14** [CODE]: pure fns `word_of_block` (H-FS1), `superblock_{magic,ninodes,inodestart,valid}`
  (H-FS3), `inum_valid` (H-FS2), `inode_block`, `dinode_word_offset`, `decode_dinode`,
  `file_block_index`, `block_for_fbn` (direct+single-indirect, H-FS4), `indirect_block`,
  `dirent_inum`, `dirent_name`, `name_eq` (H-FS8) — `FS_WORDS_PER_BLOCK = 256`, all bounded.
- [x] **Step 15** [B]: `pub mod fs_walk;` added to `monad-common/src/lib.rs`; bpfel build green (0.18s, no_std-safe).
- [x] **Step 16** [V][GATE]: `cd crates/monad-mbc && cargo test -p monad-common` → **73 passed** (60 existing + 13 fs_walk), 0 failed.
- [x] **Step 17** [C]: commit `feat(upc): fs_walk — pure xv6 fs.img walker + off-target tests`.

---

## PHASE 2 — Per-fd inode state ✅ DONE (2026-06-28)

**Goal**: fds can carry `{inum, offset}`. **Agent**: Solo.

> **TEST-HOME CORRECTION (continued from Phase 1):** the pure FD-table logic
> (kind constants, `fd_slot`, `lowest_free`, and the new inode-state model) was
> **moved into `monad-common::fdtable`** so it actually runs under
> `cargo test -p monad-common` — the bin crate's inline `fdtable.rs` tests are
> dead (build-std poison). The bin `fdtable.rs` is now `pub use
> monad_common::fdtable::*;`, so `main.rs`'s `fdtable::*` call sites are
> unchanged. The live BPF maps + accessors stay in `main.rs`.

- [x] **Step 18** [TEST]: inode-state tests in `monad-common/src/fdtable.rs` — `FD_INODE`
  distinct from `FD_FREE`/`FD_CONSOLE`; per-pid disjoint `FD_INODE_MAP` rows; bind→get,
  offset-advance, recycle-on-close clears state, out-of-range fd no-op.
- [x] **Step 19** [CODE]: added `FD_INODE = 2`; `FD_INODE_MAP: Array<[u32;2]>` (FD_TABLE_LEN
  rows, indexed by `fd_slot(pid,fd)`, value `[inum, offset]`); BPF helpers `fd_inode`,
  `fd_set_inode`, `fd_set_offset`, `fd_clear_inode`; `close(21)` now clears the inode row on
  free. (dup/fork inode-copy intentionally deferred — ls/cat open their own fds post-exec.)
- [x] **Step 20** [V][GATE]: `cargo test -p monad-common` → **85 passed** (73 prior + 12:
  7 fd-table now in their runnable home + 5 inode-state), 0 failed; ascend + non-ascend
  bin builds green (only the pre-existing `TAIL_ROUND` unused-unsafe warning), ascend rebuilt last.
- [x] **Step 21** [C]: commit `feat(upc): per-fd inode state (FD_INODE + FD_INODE_MAP)`.

---

## PHASE 3 — Real `open(15)`

**Goal**: path → inum → `FD_INODE`. **Agent**: Coordinator (build+boot).

- [ ] **Step 22** [CODE]: replace stub @2359 — strip leading `/` and `./` to basename; read
  superblock (H-FS3); walk root dir (inum 1) dirents for a name match → inum (re-entrant
  ≤8 dirents/tick, scratch cursor in a2/a3, `pc-=1; break`); alloc `FD_INODE` + `{inum,0}`;
  miss → `-1`. Keep `open("console")` → `FD_CONSOLE`.
- [ ] **Step 23** [SEC]: assert H-FS1/2/3/5 bounds on every fs.img access; path read bounded.
- [ ] **Step 24** [B]: budget check `bash scripts/bpf-verifier-check.sh`; then rebuild ascend (footgun).
- [ ] **Step 25** [V]: budget under 1M (object loads).
- [ ] **Step 26** [B][S]: boot with instrumentation; `open("init")` → non-console fd, `open("nope")` → -1.
- [ ] **Step 27** [V][GATE]: above holds AND Gate C `--input $'echo\n'` still `forks=2 waitpid=1`.
- [ ] **Step 28** [C]: commit `feat(upc): real open(15) — root-dir inode resolve`.

---

## PHASE 4 — Real `fstat(8)`

**Goal**: `ls` can stat. **Agent**: Coordinator.

- [ ] **Step 29** [CODE]: add ascend handler `syscall_nr == 8`: fd→`{inum}`→dinode→fill
  `struct stat` (`dev=1, ino=inum, type, nlink, size` — 64-bit size per `stat.h`) into the
  user buf via `user_phys` (H-FS6).
- [ ] **Step 30** [B]: budget check; rebuild ascend.
- [ ] **Step 31** [V][GATE]: budget OK; boot instrument shows sane stat for a known file.
- [ ] **Step 32** [C]: commit `feat(upc): real fstat(8) from dinode`.

---

## PHASE 5 — Real `read(5)` for `FD_INODE` → **`ls` works**

**Goal**: first visible FS win (no argv needed — `ls` lists `.`). **Agent**: Coordinator.

- [ ] **Step 33** [CODE]: extend read @2418 — for `FD_INODE`: `data_block_for_offset`, copy
  bytes to user buf re-entrant ≤16 words/tick via `user_phys` (H-FS6/7), advance stored
  `offset`, 0 at EOF. Keep `FD_CONSOLE` path untouched.
- [ ] **Step 34** [SEC]: H-FS7 clamp `min(n, size-offset)`; no wrap; indirect bounded.
- [ ] **Step 35** [B]: budget check; rebuild ascend.
- [ ] **Step 36** [V]: budget under 1M.
- [ ] **Step 37** [B][S]: `--input $'ls\n'` (3 bytes — fits 8-slot ring).
- [ ] **Step 38** [V][GATE]: TTY lists `. .. init sh ls cat echo wc README` w/ types+sizes;
  gate2 ISOLATION-PASS; Gate C green.
- [ ] **Step 39** [C]: commit `feat(upc): real read(5) for inodes — ls lists root`.

---

## PHASE 6 — argv-on-stack in `exec(7)` + enlarge KBD ring (highest verifier risk → last)

**Goal**: `cat README`, `echo hello`. **Agent**: Coordinator.

- [ ] **Step 40** [CODE]: enlarge KBD ring 8→64 — `KBD_MAP` max_entries 8→64; `& 7`→`& 63`
  at read@2440; `cmd/upc-bootctl/src/runner.rs::write_kbd` cap 8→64.
- [ ] **Step 41** [V]: Gate C `--input $'echo\n'` still works (regression guard for the ring change).
- [ ] **Step 42** [CODE]: in `exec(7)`@2471 capture `argv` VA from `a1` (stable across the
  re-entrant .data copy); AFTER the copy, build the argv frame on the new stack in
  `[sp, 0x500000)` — copy arg strings + NULL-terminated pointer array (bounded `ARGV_MAX=8`,
  `ARG_CAP=32`, re-entrant ≤16 B/tick via a second EXEC_MAGIC sub-state); set `a0=argc`,
  `a1=&argv[0]`, `sp` just below the frame. All within the child slice via `pid_phys_offset`.
- [ ] **Step 43** [R]: verify `crt0_mbc.S` `_start` forwards `a0/a1` to `main` (does not clobber).
- [ ] **Step 44** [SEC]: argv source bounded to child slice; arg count/len capped; no wrap.
- [ ] **Step 45** [B]: budget check; rebuild ascend.
- [ ] **Step 46** [V]: budget under 1M. If it blows the ceiling → STUCK: decompose further or
  defer argv to Gate D.1 (FS already shipped P1–5; do not lose it).
- [ ] **Step 47** [B][S]: `--input $'echo hello\n'` → `hello`; `--input $'cat README\n'` → README text.
- [ ] **Step 48** [V][GATE]: both print correctly; Gate C green.
- [ ] **Step 49** [C]: commit `feat(upc): argv-on-stack in exec(7) + 64-byte KBD ring`.

---

## PHASE 7 — Integration, acceptance & handoff

**Goal**: full acceptance + docs. **Agent**: Solo.

- [ ] **Step 50** [V]: **E1** `ls` → real root listing.
- [ ] **Step 51** [V]: **E2** `cat README` → exact host ASCII (anti-memorization control).
- [ ] **Step 52** [V]: **E3** `echo hello` → `hello`.
- [ ] **Step 53** [V]: **E4 regression** gate2 ISOLATION-PASS; gate_nway `NWAY-FAIL pid2`
  (unchanged); Gate C `echo\n` `forks=2 waitpid=1`; Doom builds; both configs under 1M.
- [ ] **Step 54** [W]: `references/phase17-gateD-fs-reader-2026-06-28.md` session log.
- [ ] **Step 55** [W]: update `~/tmp/next.md`, the Gate-C memory's successor, `CLAUDE.md` gate line.
- [ ] **Step 56** [W]: ADR if the eBPF-native FS-walker approach warrants one (cross-ref ADR-077).
- [ ] **Step 57** [C][GATE]: final commit; tree clean; all gates green.

---

## EMERGENCY PROCEDURES

- **`invalid value size 144, expected 136`** → ascend object clobbered (often by the verifier
  script's plain build). Rebuild `cargo build --release -p monad-cpu-ebpf --features ascend-linux` LAST.
- **Boot halts with `halt_reason`** → read `ROM-FAULT CTX` (STATS 25-29): 0x47 MRET-unmapped,
  0x48 SRET, 0x60 ROM-fetch-miss, 0x66 PC-oob, 0x80 HALT, 0x2E5E7 reset-to-0.
- **`BPF program too large` / load fail** → a loop isn't re-entrant; cut iters/tick, add
  `pc-=1; break`; re-check budget.
- **`cat`/`ls` print garbage** → block-size bug; confirm `FS_WORDS_PER_BLOCK=256` (NOT 128).
- **`ls`/`cat` block forever** → fd not `FD_INODE` (open fell to console), or EOF not returned
  (check `offset≥size`).
- **`--input` rejected >8 bytes before Phase 6** → expected; only `ls\n`/`echo\n` fit until the ring is enlarged.

## BUILD / BOOT RECIPE (footgun: rebuild ascend LAST)

```bash
cd ~/tmp/unheaded/crates/xv6-mbc/upstream && make -f ../adapters/Makefile.mbc-userland ramdisk   # 1 userland+fs.img
cd ~/tmp/unheaded/ebpf && cargo build --release -p monad-cpu-ebpf --features ascend-linux          # 2 ascend LAST
bash ~/tmp/unheaded/scripts/bpf-verifier-check.sh   # 3 budget (then re-run step 2 — script clobbers!)
cd ~/tmp/unheaded/crates/monad-mbc && cargo test -p monad-common   # 4 off-target logic tests (fs_walk etc.)
sudo ~/tmp/unheaded/cmd/upc-bootctl/target/release/upc-bootctl boot \
  --kernel ~/tmp/unheaded/crates/xv6-mbc/upstream/target/xv6-mbc.mbc \
  --ramdisk ~/tmp/unheaded/crates/xv6-mbc/upstream/target/fs.img \
  --userland ~/tmp/unheaded/crates/xv6-mbc/upstream/target/init.mbc \
  --triggers 3000000 --instance 222 --input $'ls\n'   # 5 swap --input per E1/E2/E3 (≤64B after Phase 6)
```

## CRITICAL FILES

- `ebpf/monad-cpu-ebpf/src/main.rs` — ascend ecall handlers: open@2359, read@2418, +fstat(8), exec@2471 (argv); KBD masks.
- `ebpf/monad-common/src/fs_walk.rs` — **NEW** pure walker + inline off-target tests (NOT the bin crate — see Phase 1 test-home correction).
- `ebpf/monad-common/src/fdtable.rs` — **NEW** pure FD-table logic: `FD_INODE` kind + inode-state
  model + tests (the runnable test home; bin `fdtable.rs` re-exports it). `FD_INODE_MAP` map +
  `fd_inode`/`fd_set_inode`/`fd_set_offset`/`fd_clear_inode` accessors live in `main.rs`.
- `cmd/upc-bootctl/src/runner.rs` — `write_kbd` cap 8→64.
- `crates/xv6-mbc/adapters/Makefile.mbc-userland` — add `README` to mkfs.
- `crates/xv6-mbc/adapters/crt0_mbc.S` — verify `a0/a1` forwarded (likely no change).
- Reference: `crates/xv6-mbc/upstream/kernel/fs.h`, `stat.h`, `user/ls.c`, `user/cat.c`.

---

*Gate D Battle Plan — Forged 2026-06-28*
*8 phases. 57 steps. The UPC learns to read its own disk: `ls` sees, `cat` speaks, `echo` answers.*
*Design A holds — the protocol computer walks its own inodes. Peace and love. 🌀🐕*
