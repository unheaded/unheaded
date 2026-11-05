<!--
SPDX-License-Identifier: GPL-3.0-or-later
Copyright (c) 2024-2026 Stevie Bellis.
-->
# Doom's Lessons

Doom left for the `unheaded-doom` repository (DP-305). This page keeps what it taught, for a reader who will never run it.

**Background.** The UPC is a virtual CPU inside an eBPF/XDP program. C compiles to RV32I, a translator turns that into MBC bytecode, and a loader puts it in BPF maps: `ROM_MAP` holds instructions, `RAM_MAP` memory, `CPU_MAP` registers. Each packet that crosses a ring of network namespaces runs a bounded slice of instructions. An injector keeps packets flowing, and that is the clock. Doom was the first guest. xv6 is the second, on the same interpreter (ADR-080).

**Sources.** A bare file name such as `FINDINGS.md` is in `docs/doom/` of `unheaded-doom`. `doom:path` is any other path in that repository. Every other path is in this one. Commit hashes given here resolve in this repository's history; several hashes quoted inside `docs/doom` do not resolve here.

**This page is the later reading.** `docs/doom` is a dated log, and its files contradict each other. `STATUS.md` and `README.md` still call banding "not a bug", and `FINDINGS.md` still says the PC corruption was confirmed. Where this page differs it says why.

## What it proved

- **A real program ran.** id's `linuxdoom-1.10`, 62 C files, against the retail `DOOM.WAD` of 12,408,292 bytes (`README.md`). The fixture kept here is 86,463 MBC instructions (`doom/doom.mbc`, 345,852 bytes, loaded whole by the `crates/monad-mbc` smoke test).
- **It drew frames.** First frames on 2026-03-30: 62,754 of 64,000 pixels non-zero, 450+ frames (`3b512de1`, `STATUS.md`). A full 320x200 frame was confirmed again on 2026-07-19 (`references/timeline.md`).
- **It played.** Game logic held 35 frames a second by the `STAT_FRAME_READY` delta. Menus, movement, weapons and doors work (`STATUS.md`, `BASELINE.md`).
- **E1M1 can be finished.** It needs the 12 MB zone; E1M2 loads, with wrong textures (`0ecb76ec`, `FINDINGS.md` "12MB zone memory", `RUNBOOK.md` known issue 2).
- **It stayed up.** 5.9B+ instructions with no crash at the 2026-03-30 baseline (`STATUS.md`). More than ten minutes and about 878B instructions on the retail WAD with no `I_Error` on 2026-07-29 (`FINDINGS.md`).

### The computational-generality claim and its limits

`docs/upc/COMPUTATIONAL_GENERALITY.md` states it as a hypothesis: what compiles to RV32I can run here. `wiki/Doom-on-UPC.md` goes further and calls Doom a proof of Turing-completeness. The evidence supports the hypothesis. It does not support "proof".

What it does show: dynamic dispatch, a heap, arbitrary memory access and a large real codebase all work through MBC and BPF maps. xv6 running on the same core (ADR-080) adds more than Doom could alone.

What it does not show:

- **The machine is finite.** `RAM_MAP` is 64 MiB and `ROM_MAP` had 262,144 slots (`ARCHITECTURE.md`). That is a big bounded machine, like any real computer, and the formal term does not apply.
- **Each slice is bounded.** 16 tail-call rounds of 16 instructions, 256 a hop (`ARCHITECTURE.md`, "Tail Call Chain"). Unbounded running comes from outside the program, from an injector that keeps sending. Stop the injector and the CPU stops.
- **It is not self-contained.** State lives in maps between packets, a userspace loader fills them, and a userspace bridge reads frames and writes keys (`DOOM-BRIDGE-ARCHITECTURE.md`). The wiki's "no external state" overstates this.
- **Running is not running correctly.** Banding is an open interpreter-side bug (below). Doom passing is no proof that the interpreter is right.
- **The next candidates are paper.** The SNES, Unix v4 and Game Boy verdicts in `COMPUTATIONAL_GENERALITY.md` are estimates. None was built.

There was never sound (`STATUS.md`), and the browser saw 2 to 3 frames a second, not 35 (see Performance).

## Bugs worth remembering

### PC corruption, which was mostly a misread instrument

**Symptom.** 2026-03-29: 24B+ instructions, black screen, no frame ever. PC samples read `0x2E6EB7AB`, `0x365E9BDD`, `0x931664E9`, far past the ROM. Out-of-range `ROM_MAP` reads return 0, opcode 0 is NOP, so the theory was a CPU running NOPs into the void. id DOOM showed the same, `0x4B200000` at 1.28B instructions (`FINDINGS.md`, "CRITICAL FINDING", "PC Corruption -- Confirmed in id DOOM Too").

**Cause.** The samples were wrong. An explicit bounds check, `pc >= 262144` halts, was added the same evening and never fired. `40b62878` records it: the sampling "was reading wrong bytes from CPU_MAP", real PC was `0x1BB4` inside `fread`, "NOT a PC corruption bug". `FINDINGS.md` kept only a warning to decode the full 128-byte struct, and its body still says "Root cause (confirmed)". That commit covers the id DOOM run; the doomgeneric samples were never re-read, but came from the same method. The same signature came back on 2026-03-31, `PC=51,379,584` and `halted=48` while the game was visibly playable (`HANDOFF.md`, H1), which fits a misread and not a dead CPU.

Next came a second false alarm: a zeroed SP, because the check read `r2` and the translator maps `x2` to `r15` (`2db6d9c6`). The real blocker was behind it. All 25 soft-float functions recursed into themselves through compiler-generated float calls (`6a2a7d16`, `1f7a60e5`). With those rewritten and a few stub bugs fixed, frames appeared within the hour (`3b512de1`).

**How it was found.** The bounds check, not more sampling: put the assertion in the executor, where it cannot misread the struct.

**The RET-floor hypothesis: unverified, and the code history argues against it.** In July, Gate D.1 found that `RET` told MBC return addresses from RV byte addresses by magnitude, a floor at `0x10000`. Doom's PCs reach `0x151BF`, so the floor could misparse its returns, and `FINDINGS.md` (2026-07-02 update) and ADR-079 named it a strong candidate for the March blocker. It cannot be the March cause. The floor first appears in `b17487ff`, 2026-05-14; at `819a32fc`, in March, `RET` is a plain `cpu.pc = regs[14]` with a zero check. The only window where it could have hurt Doom is 2026-05-14 to 2026-07-02, when, by ADR-080, `doom-runner` was not being run at all. Bit-31 tagging replaced the floor (ADR-079, `49599883`). Doom then ran with the tag live, PC valid at `0x130bb`, `regs[14]=0x800130fa` (`FINDINGS.md`, 2026-07-03). That proves the tag is compatible with Doom. It proves nothing about what caused anything earlier.

Two other explanations do not hold. `NEXT-STEPS.md` credits "CALL/RET in MBC translator", but that fix is `0864ab3c`, 2026-03-16, thirteen days before the sighting. `wiki/Doom-on-UPC.md` blames `__umoddi3` and `__udivdi3`; no document or commit message supports that.

### Banding: open, diagnosed, not fixed

**Symptom.** Horizontal multicolour bands of black, red, white and brown where stone should be, worse in later rooms, on both the retail and the shareware WAD (`HANDOFF.md`).

**Cause, as far as anyone got.** `texturecolumnlump[]` holds garbage lump numbers for some textures: BROWN96 has column 0 pointing at SSECTORS, column 1 at PLAYPAL and column 6 out of bounds, while STARTAN3 is right. So `R_GetColumn` draws non-texture lumps. A zone block header read `size=24` where 280 was expected: blocks from `Z_Malloc` are corrupted when z_zone runs on MBC, the same class `doom:demos/doom/w_file_stdc.c` already worked around for WAD handles (`HANDOFF.md`, "ROOT CAUSE FOUND"). Which opcode or sequence corrupts them was never identified.

**How it was found: by killing explanations, one at a time.**

| Explanation | Verdict | Evidence |
|---|---|---|
| Authentic nearest-neighbour magnification | Wrong, though `STATUS.md` and `README.md` still say it | chocolate-doom renders the same WAD correctly; both WADs band on MBC (`HANDOFF.md`) |
| Palette wrong | Real, 203 of 256 entries, but separate | Fixed in `bcac340c`; bands stayed (`FINDINGS.md`) |
| `CALLR` never executes, labelled "ROOT CAUSE" in `16ef371b` and `2a9bac56` | Wrong | 1.2M `CALLR` calls counted (`FINDINGS.md`, 2026-03-31) |
| PU_CACHE purging | Disproven | Each `PU_STATIC` change, tried one at a time, exhausted the zone or left the bands; a zone above 12 MB will not load (`HANDOFF.md`) |
| MBC `ST` never reaches `RAM_MAP` | Disproven as a write failure | `diag_buf` in `.bss` looked unwritten, so failures were counted: `STAT[22]`=0, `STAT[23]`=10,540,368 (`HANDOFF.md`, H1 update). Why some `.bss` reads back zero was never settled |
| Doom C source or WAD | Cleared | Lump bytes in `RAM_MAP` match the file, struct layout verified (`HANDOFF.md`) |

**The paradox.** Moving texture init from `Z_Malloc` to a bump `malloc` removed most banding, then crashed in rooms 2 and 3. Replacing z_zone outright brought banding back (`HANDOFF.md`, "THE PARADOX"). Every Doom-side workaround was reverted in `f21ec120`: stable, with banding, and the note that the fix belongs in the executor.

**What is still untested, and why it matters here.** H2 (misaligned `LD`), H3 (word `memcpy`) and H4 (fd drift) were never tested. S-DOOM-LAST planned the differential pass, but none of its steps is ticked and `FINDINGS.md` has no result. H2 is not Doom's problem alone. `LD` in `ebpf/monad-cpu-ebpf/src/main.rs` still does `word_addr = addr >> 2`, dropping the low two address bits, and that interpreter stayed in this repository and serves every guest.

### WAD size

**Symptom.** Shareware `doom1.wad` froze.

**Cause.** `open()` in `doom:demos/doom/libc_stubs.c` hardcoded 12,408,292 bytes, so a 4.2 MB file reported the retail size.

**Fix.** `doom-runner` writes the real count to `WAD_SIZE_ADDR` (0x01BFFFF0) and the stub reads it (`0ecb76ec`).

**How it was found.** The commit records the symptom and the fix, not the hunt. Worth keeping for what came later: the next WAD failure looked identical, and a `RUNBOOK.md` known issue blamed size until 2026-07-29 (`FINDINGS.md`).

### IWAD identity

**Symptom.** Shareware ran and retail did not. Before that, shareware halted about five seconds in with `PC=7972`, reason `0x80`.

**Cause.** id's `IdentifyVersion()` sets `gamemode` from the first name `access()` says exists, so whichever name the stub reports pins the game mode for the build. The stub went through four hardcoded answers: every `.wad` (Doom II commercial mode, `8a61d7f0`), then `doom.wad`, `doomu.wad` and `doom1.wad`. Each fixed one WAD and broke another. Shareware needs `shareware` because it has no `DEMO4`. Retail needs `retail` because Ultimate DOOM has no `HELP2`, which `registered` requires, and it ships named `DOOM.WAD`, which maps to `registered` (`FINDINGS.md`, 2026-07-24 and 2026-07-29).

**How it was found.** Reading `RAM_MAP` at the debug message address gave `W_GetNumForName: demo4 not found!`: read what the guest said before theorising. The fix detects the flavour from the lump directory at load time and publishes the probe name. A filename is not an identity; the lumps are.

**The trap inside it.** A stale `doom-runner` held port 16666, the new run died on the bind, and the old process went on serving retail frames during a shareware test (`FINDINGS.md`, "Testing gotcha").

### Smaller ones that carry to any guest

| Symptom | Cause | Rule | Source |
|---|---|---|---|
| Maps read all zeros after loading | Pinned maps and the program's own maps were different maps | Whoever loads the program writes its maps | `FINDINGS.md` Bug 1 |
| Function pointers go bad near 99M instructions | `RAM_MAP` was a HashMap; when full, writes were silently dropped | Memory is an Array map | `wiki/Doom-over-IPv6.md`, Phase 3 |
| Globals are zero at start | `.sdata` left out of the ELF extraction | Load every section in the RAM range | `FINDINGS.md` Bug 2 |
| Allocator returns wild addresses | `heap_ptr` sat among `.data` globals | Isolate allocator state | `FINDINGS.md` Bug 7 |
| `sprintf` writes nothing | `size_t` -1 wraps below the buffer on 32 bits | Cap the size | `FINDINGS.md`, "sprintf 32-bit Overflow" |
| All-zero frames across restarts, with 13.7B instructions claimed | `doom-runner` loads XDP and never attaches it | Loaded is not attached: check the hook | `FINDINGS.md`, 2026-07-24 |
| `SCREEN_MAP` reads about 0 | Word stores land in `RAM_MAP` only | Read the map the guest writes | `FINDINGS.md`, 2026-07-03 |

## Verifier lessons

The kernel verifier stops at 1,000,000 processed instructions per program (ADR-080). The interpreter is one program, so every feature spends from one budget.

| Date | Variant | Processed insns | Source |
|---|---|---|---|
| 2026-07-02 | ascend | 848,188 | ADR-079 |
| 2026-07-03 | Doom | over the ceiling by 1, then "right at the edge" once gated | ADR-080, `FINDINGS.md` |
| 2026-07-19 | Doom | 737,087 | `references/timeline.md` |
| 2026-10-28 | Doom | 758,199 | `docs/policy/ebpf-insns-baseline.tsv` |
| 2026-11-05 | ascend | 889,941 | `docs/policy/ebpf-insns-baseline.tsv` |

The sources do not say what took Doom from the edge to 737,087. Ascend works to a limit of 950,000 (`docs/battle-plans/EVOLUTION-1-STAGE-2-battle-plan.md`).

**What blew it.**

- **A single-pass loop.** 256 instructions a tick would not verify, so it dropped to 16 (`0864ab3c`) and tail calls won the 256 back.
- **A 16,000-iteration copy.** The framebuffer copy could never verify; it became a no-op and the work moved to userspace (`LEVEL3_OPTIMIZATIONS.md`).
- **A per-instruction diagnostic.** An SP monitor in the hot loop passed the limit; it moved to the HALT handler (`6a2a7d16`). Instrumentation spends the same budget as the feature.
- **Shared dispatch.** Ten xv6-only syscall arms sat in the object Doom also compiled, and dead-code elimination did not gate them out. Doom went over by one, unseen, for the length of the xv6 sprint (ADR-080, `FINDINGS.md`, 2026-07-03).
- **`translate_address()` in a per-byte loop.** The Gate A boot-verify of 2026-06-18 (ADR-077).

**What fit.** A workload's build carries only its own syscall surface, split by `cfg` (ADR-080 §3). Hot-path state lives in `MbcCpuState`, not behind a map lookup (ADR-077). Long operations re-enter one item per tick, and a fixed 16-byte copy with no guard fits where a loop does not (`references/phase17-gateD-shipped-2026-07-02.md`).

**The rule: load-test every variant.** Compiling proves nothing, because only a real load runs the verifier; `bpf-verifier-check.sh` only compiled and analysed (`FINDINGS.md`, 2026-07-03). `scripts/upc-regression.sh` keeps the rule. It loads the ascend object through `upc-bootctl`, the Doom object through `doom-runner`, and the cartridge object, and prints each budget. Without a WAD the Doom load skips and exits 0, so anything that needs it sets `UPC_REQUIRE_DOOM=1`. `scripts/check-ebpf-loads.sh` and `scripts/check-ebpf-insns.sh` hold the same line in the gates.

**Both variants build to one path.** An ascend object where Doom expects its own fails or misbehaves (`RUNBOOK.md` troubleshooting, `FINDINGS.md` 2026-07-23). The harness rebuilds ascend last, so the object on disk is the default one.

## Performance

Measured figures only. They come from different counters on different days and do not compare with each other. `insn_count` in `MbcCpuState` does not persist across tail calls; the `STATS` map does (`FINDINGS.md`).

| Date | Setup | Measured | Source |
|---|---|---|---|
| 2026-03 | Python injector, 3 ms delay | about 333 pps, about 6 fps | `PERFORMANCE.md` baseline |
| 2026-03 | Ring not circulating, zero delay | about 800K pkt/s, 16 insns a packet, about 12.8M insns/s | `phase8-results.md` |
| 2026-03-26 | `burst` to `sendmmsg`, batch 64 | 15K to 29M insns/s, on a blank screen | `docs/archive/HANDOFF-2026-03-26.md` |
| 2026-03-30 | Word stores, bridge reads `RAM_MAP` | 446 frames in 30 s | `a4679d08` |
| 2026-03-30 | Direct render, no per-frame copy | 35.0 fps of game logic | `1bc0a27b` |
| 2026-03-30 | `sendmmsg`, batch 200 | about 93K pps; browser 2 to 3 fps | `NEXT-STEPS.md`, `STATUS.md` |
| 2026-07-19 | Full pipeline | about 90K pkt/s, about 23M insns in 2 s | `references/timeline.md` |
| 2026-07-29 | Full pipeline, retail | about 89K pkt/s; 13.4M packets and about 878B insns in the run | `FINDINGS.md` |

**What limited them, in order.**

1. **The injection rate.** Packets are the clock. Moving from `burst` at 60 pps to `sendmmsg` took the rate from 15K to 29M insns/s (`docs/archive/HANDOFF-2026-03-26.md`).
2. **Stores per frame.** 64,000 byte stores became 16,000 word stores (`a4679d08`). A double buffer halved fps (`1a33c19b`) until the word-aligned `memcpy` made a back buffer affordable (`e846c05d`).
3. **The bridge.** 16,000 BPF map lookups per frame cap what the browser sees, whatever the guest does (`DOOM-BRIDGE-ARCHITECTURE.md`, Known Limitations).

**The Python `sendto` correction.** "920K pps" was quoted as AF_XDP throughput. It was the ceiling of a Python UDP `sendto()` sender in the Phase 10 Shield tests, and it says nothing about the data plane (`docs/BENCHMARKS.md`, `docs/sessions/PHASE10_NEXT_SENDER.md`, M-090). AF_XDP throughput was never measured. A sender-bound number measures the sender.

**Not measurements, whatever the tables look like.** The "10,000,000 pps XDP capacity" is an estimate, marked "never tested", so "0.003% utilization" and "6,600 instances" are too. The delay-sweep rows in `PERFORMANCE.md` and `wiki/Performance.md` are expected values; the `TBD`s were never filled in and H1 to H3 have no recorded result.

**Figures not to reuse.** The pre-WS3 baseline disagrees with itself: 4,080 insns a packet gives the listed 1.35M insns/s, while the listed 6 fps at 1.47M insns a frame needs about 8.8M. The "534B insns/s" in `FINDINGS.md` (2026-07-24) cannot be a rate at 19.5K pkt/s, and it reads like a running total.

## Method

**Prove the instrument before the theory.** This is the lesson the rest depends on. The PC corruption was bad sampling. The zeroed SP was the wrong register. "`CALLR` never executes" was wrong. The "definitive diagnosis (do not re-litigate)" of 2026-07-23, stuck before video init, was an unattached XDP program the next day (`FINDINGS.md`). Words like "confirmed" and "root cause" sat on all of them and carried no information. Trust a claim for its evidence, and check what the probe reads before reading what it says.

**Debugger first.** Instrument the guest, do not guess from outside. The path that worked: bounds-check PC, then SP, then identify the function, using breadcrumbs, a debug region above the stack and direct map reads (`FINDINGS.md`, "Debug Methodology"; `819a32fc`). Each step takes a question from "why is it slow" to one address.

**More instructions is not more progress.** 24B instructions looked like progress while no frame was drawn. Trust a counter that needs the program to be working, such as `STAT_FRAME_READY`, breadcrumbs or non-zero pixels (`FINDINGS.md`, "Lesson Learned"). The executor now halts on `pc >= 262144`, so a true NOP run cannot hide.

**Marshal oversight against NOP loops.** A rising counter can keep a session busy for hours with nothing useful running. A separate overseer asks for evidence before any claim of progress stands, and a claim about Doom waits for a Marshal-supervised run (`docs/upc/UPC-PORT-PROCESS.md`, lesson 6; `FINDINGS.md`, 2026-07-02 update; ADR-079).

**One change, one control.** The PU_CACHE fixes were tried singly, and that is why the table above has a verdict. chocolate-doom on the same WAD ended the "authentic rendering" reading, which had survived days of inspection (`HANDOFF.md`).

**Stale state lies.** After any restart, confirm the old process is gone by pid and the port is free, and that the new one logged what you expect (`RUNBOOK.md` troubleshooting).

**Reduced screen size is honest 486 behaviour.** The UPC targets a 486-class machine (`PIVOT-TO-ID-DOOM.md`). Shrinking the view with `+` and `-` is what players on that hardware did. The default became `screenblocks=7` because fewer columns are fewer instructions (`0ecb76ec`). It is a performance setting, not a workaround, and it has nothing to do with banding (`docs/battle-plans/S-DOOM-LAST-battle-plan.md`).

## Where everything went

| What | Where |
|---|---|
| Runner, bridge, injectors, port, build, runbooks, the three units | `unheaded-doom`, at `~/tmp/projects/unheaded-doom` |
| Doom's history | Same repository, filtered from this one; the `doomgeneric` port joined by subtree merge, with its WAD filtered out |
| `docs/doom`, all of it | `unheaded-doom`, as a full copy including the ten UPC files below |
| The ten UPC specs (`UPC_BOOT_PROTOCOL_V2.md` and the rest) | Here, in `docs/upc/`; this copy is the live one |
| The fixture: `doom/doom.mbc`, `doom/doom.rv2mbc`, `doom/doom.rv2mbc.sha256`, `doom/LICENSE` | Here; `unheaded-doom` keeps its own copy for `doom-runner`'s tests |
| `doom.elf` and the runner the harness loads | `unheaded-doom`, by path; `DOOM_RUNNER` and `DOOM_ELF` override |
| The interpreter, its Doom build variant, `doom-debug`, `upc-api` | Here: they are UPC code |
| Dashboard Doom routes | Gone from here; the REST handlers are kept as text in `doom:reference/` |
| Ports `DoomBridge` 16666, `DoomGoInjector` 16667, the "Doom Range" name | Held in `pkg/ports`; the range name is a port band, not Doom code |
| uid 16790 | Held in reserve for `unheaded-doom`, noted in `pkg/uids/uids.go`; never assigned |
| `unheaded-doom` on GitHub | Not yet: Stevie creates it and pushes |

A `upc-api` change can break `doom-runner` with no gate in that repository, and `doom/doom.elf` there must match the fixture here (`doom:README.md`). Rebuild the runner after an ABI change.
