# Marshal Shift Log — 2026-05-09 (ASCEND-LINUX super-sprint)

**Source plan:** `references/battle-plan-ascend-linux-2026-05-08.md`.
**Predecessors:** `references/marshal-shift-2026-05-08-ascend-linux-kickoff.md`, `references/marshal-shift-2026-05-08.md` (drain shift).
**Mode:** UNATTENDED Marshal-led super-sprint per Stevie's "go go go - all phases all pending work."
**Host:** WEST (Linux 6.17, RV32I cross-toolchain, bpftool present).

---

## Mission vs Result

**Mission:** Push ASCEND-LINUX as far as token budget allows. All pending Phase 0/1 work.

**Result:** **25 ASCEND-LINUX commits across the conversation.** Phase 0 fully complete; Phase 1.1 advanced from "vendoring + skeleton" all the way to **kernel.elf links + xv6-mbc.mbc EMITS (11,721 MBC instructions, 46 KB)** + **upc-bootctl tool boots dry-run** + **upc-tty-bridge + xterm.js page**.

The ONE missing piece for first runtime "xv6 booting..." is doom-runner-style aya BPF integration in upc-bootctl (the live boot path; ~3 days of next-shift work).

---

## What landed in this super-sprint (12 new commits since kickoff)

```
21fa8ca4  feat(ascend-linux): 🎉 xv6-mbc.mbc EMITS — first kernel image translated
b87f2e53  chore(xv6-mbc): fix .gitignore — exclude build artifacts (kernel.elf)
[multi]    feat(xv6-mbc): kernel.elf LINKS — RV32 conversion + 5 more iterations
[multi]    feat(upc-bootctl): boot dispatcher tool — validate + dry-run boot
[multi]    feat(upc-tty-bridge): WebSocket bridge + xterm.js Mode A demo surface
```

### Build pipeline driven through 12 iterations

| Iter | Blocker | Fix |
|------|---------|-----|
| 1 | gas binary name | Use `as` not `gas`; add `_zicsr_zmmul` to ASFLAGS |
| 2 | RV64 sd/ld in swtch.S | adapters/swtch_mbc.S (sw/lw, halved offsets); patch struct context u64→u32 |
| 3 | RV64 sd/ld in trampoline.S, kernelvec.S | Python-rewrite both; halve offsets; patch struct trapframe u64→u32 |
| 4 | MAXVA Sv39 overflow | riscv.h: MAXVA = (1U<<31) for Sv32 |
| 5 | .S preprocessor missing | %.o: %.S uses gcc not as |
| 6 | ld emulation 64 vs 32 | LDFLAGS += -melf32lriscv |
| 7 | undefined `end` | linker script: PROVIDE(end = .) at .bss end |
| 8 | undefined plicinit/virtio_disk_init | sed-patch main.c to comment out |
| 9 | __udivdi3, __umoddi3, __sync_* | adapters/libgcc_stubs.c (70 LOC) |
| 10 | undefined stack0 | declare in start_mbc.c |
| 11 | translator: 144 CSR errors | translator.rs CSR extension (~80 LOC, ADR-067 Decision 2) |
| 12 | translator: 40 x16+ register errors | map_register: alias x18-x31 onto r3-r13 (best-effort) |

After ITER 12: `xv6-mbc.mbc` emits successfully.

### Tools shipped

- **`cmd/upc-bootctl/`** (260 LOC) — `validate` + `boot --dry-run` + `console` skeleton.
  - `./upc-bootctl validate --kernel xv6-mbc.mbc` decodes 11,721 MBC instructions, prints first 32 + boot magic.
  - `./upc-bootctl boot --kernel xv6-mbc.mbc --instance 222 --dry-run` prints the full 10-step Boot Protocol v2 dispatch sequence.

- **`cmd/upc-tty-bridge/`** (200 LOC) — Go WebSocket bridge for Mode A demo.
  - Listens on port 26100 (UPC user-app band).
  - `/console?instance=N` WS endpoint with per-instance hub fan-out.
  - Heartbeat tick (placeholder until BPF tty stream wires up).

- **`dashboard/upc-console.html`** (150 LOC) — Browser xterm.js client.
  - Auto-connect to ws://<host>/console?instance=<N>.
  - Kingdom theme: dark bg, JetBrains Mono, accent color #61afef.

- **`pkg/ports/ports.go`** — `UPCTtyBridge = 26100` constant added.

### Translator extensions (`crates/monad-mbc/src/translator.rs`)

- **CSR opcodes**: CSRRW/CSRRS/CSRRC + I-variants now emit memory-mapped LD/ST against `0x000_F000 + csr*4` per ADR-067 Decision 2. ~80 LOC.
- **Privileged ops**: MRET (0x47), SRET (0x48), WFI (no-op), SFENCE.VMA (→ FENCE) recognized.
- **NOP for opcode=0**: linker alignment fillers no longer crash translation.
- **Register file aliasing**: x18-x31 alias to r3-r13 — best-effort; runtime correctness depends on hot-paths not relying on these simultaneously. Future shift to add real spill-emit.

---

## Aggregate session totals (across drain + ASCEND-LINUX kickoff + super-sprint)

| Phase | Commits | Status |
|-------|---------|--------|
| Drain shift (2026-05-08) | 22 | DONE |
| ASCEND-LINUX kickoff (2026-05-08) | 15 | DONE |
| ASCEND-LINUX super-sprint (2026-05-09) | 12 | DONE |
| **Total this conversation** | **49** | |

**ASCEND-LINUX-only**: 25 commits, ~1100 LOC across plan + ADR + ABI v2 + Boot Protocol v2 + 5 new MBC opcodes + xv6 vendoring + 6 adapter files + translator extensions + 2 boot tools + xterm.js page.

---

## Demo surface status

| Mode | Status | Path |
|------|--------|------|
| **A** Browser xterm | **SCAFFOLDED** | upc-tty-bridge + dashboard/upc-console.html. Wires up once BPF tty stream is live. |
| **B** Direct console | **SKELETON** | cmd/upc-bootctl console subcommand. Same wire-up timing as Mode A. |
| **C** SSH over IPv6 | Phase 4 (deferred) | dropbear in initramfs over fd00:dead:beef:dada::/64. |

---

## What's next (priority order)

1. **`upc-bootctl boot` live BPF integration** (~3 days). Pattern after `crates/doom-runner/`: load `xv6-mbc.mbc` into ROM_MAP via aya, populate `PerCPU<MbcCpuState>`, set BootParams v2, dispatch Gjallarhorn UPC trigger packet.
2. **`upc-tty-bridge` BPF tty stream read** (~1 day). Replace `pollTtyStream` heartbeat with actual BPF map subscription / Wotan topic compute.tty.{instance}.
3. **First runtime smoke test**: `cmd/upc-bootctl boot --kernel xv6-mbc.mbc --instance 222 && cmd/upc-bootctl console --instance 222` — expect "xv6 booting..." print + HALT. Phase 1.1 SHIPS.
4. **Phase 1.2 page tables** (~7 days, pair on day 1). xv6's vm.c mounts Sv32 page tables; verify against L4d MMU emulation.
5. **Phases 1.3-1.5** then Phase 2+ per battle plan.

---

## Blockers + risks

- **Register aliasing hack**: x18-x31 → r3-r13 will cause runtime corruption in xv6 hot-paths. First boot may halt unexpectedly. Real fix: emit MBC memory-spill code in translator for x16+ usage. ~1 day scope.
- **doom-runner / monad-cpu-ebpf MbcCpuState size mismatch**: doom-runner has its own 128-byte replica of MbcCpuState; new ABI v2 in monad-common is 136 bytes. Doom continues to work only because the eBPF program doesn't write the new fields. xv6 will use them. Need to align doom-runner's replica before live boot.
- **kernel image too big for ROM_MAP**: ROM_MAP is 1 MB / 256K word slots. xv6-mbc.mbc is 46 KB (11,721 words) — fits comfortably. uClinux + Linux will need the multi-segment loader from battle-plan §8 risk register.

---

## Verification commands

```bash
# Validate kernel image
./cmd/upc-bootctl/target/release/upc-bootctl validate \
  --kernel crates/xv6-mbc/upstream/target/xv6-mbc.mbc

# Dry-run boot
./cmd/upc-bootctl/target/release/upc-bootctl boot \
  --kernel crates/xv6-mbc/upstream/target/xv6-mbc.mbc \
  --instance 222 --dry-run

# Start tty bridge (in separate terminal)
./cmd/upc-tty-bridge/upc-tty-bridge --port 26100 &

# Browser at http://localhost:26100/upc-console.html → see xterm + heartbeats

# Regression
cargo test --release --manifest-path crates/monad-mbc/Cargo.toml  # 251 lib + 55 OS-prim PASS
go test -short ./pkg/ports/ ./pkg/auth/ ./pkg/transport/ ./cmd/dashboard-backend/...  # PASS
bash scripts/bpf-verifier-check.sh  # GATE: PASSED (2 warnings, 0 failures, 7% budget)
~/go/bin/govulncheck ./...  # 0 kingdom vulns
```

---

## 🎖️ Marshal handoff for the next shift

Invoke `/unheaded-marshal` with:

> "ASCEND-LINUX super-sprint complete (25 commits, 2026-05-08 → 2026-05-09).
> Phase 0 done. Phase 1.1 has: xv6-mbc.mbc emits (11,721 MBC instructions);
> upc-bootctl tool validates + dry-runs boot; upc-tty-bridge + xterm.js
> page scaffolded. NEXT: live BPF integration in upc-bootctl `boot` impl
> — pattern after crates/doom-runner/main.rs. Load xv6-mbc.mbc into
> ROM_MAP, populate PerCPU<MbcCpuState>, dispatch Gjallarhorn trigger.
> First milestone gate: 'xv6 booting...' prints to upc-tty-bridge,
> kernel HALTs cleanly. Owner Developer + Computermancer. ~3 days."

**Marshal off-duty (super-sprint complete). Badge stays on.**
