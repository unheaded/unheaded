# Translator Extensions Needed for ASCEND-LINUX Phase 1.1

**Source plan:** `references/battle-plan-ascend-linux-2026-05-08.md` Phase 1.1.
**Status:** kernel.elf BUILDS + LINKS GREEN; rv32i-to-mbc translator emits 184 errors (40 register, 144 CSR).
**Required:** extend `crates/monad-mbc/src/translator.rs` per ADR-067 decisions.

---

## Error 1 — 144 unsupported CSR instructions (opcode 0x73, funct3 ∈ {1,2,3,5,6,7})

### What

xv6 kernel uses CSR access throughout — kernelvec.S/trampoline.S/riscv.h all emit `csrr`, `csrw`, `csrrw`, `csrrs`, `csrrc`. Today translator.rs handles only `funct3==0` (ECALL/EBREAK). CSR variants fall through to UnsupportedInstruction.

### Per ADR-067 Decision 2

CSRs live at byte addresses `0x000_F000 + csr_index * 4`. Translator emits memory-mapped LD/ST.

### RV32 CSR encoding

```
opcode = 0x73
funct3 = 1=CSRRW, 2=CSRRS, 3=CSRRC, 5=CSRRWI, 6=CSRRSI, 7=CSRRCI
imm[11:0] = csr address (0-4095)
rs1 = source register (or 5-bit imm for *I variants)
rd  = destination register
```

### Translation pattern

For `CSRRW rd, csr, rs1`:
```
; load full byte address (CSR_BASE + csr*4) into a scratch register tX
LOAD_IMM32 tX, (CSR_BASE_LO + csr*4) | ((CSR_BASE_HI + 0) << 16)   ; 32-bit immediate load
LD         rd, [tX + 0]                 ; rd = old CSR value
ST         [tX + 0], rs1                ; CSR = rs1
```

For `CSRRS rd, csr, rs1`:  read, OR with rs1, write back.
For `CSRRC rd, csr, rs1`:  read, AND with ~rs1, write back.
*I variants:  rs1 field is a 5-bit zero-extended immediate, not a register.

### Implementation sketch (in translator.rs)

```rust
0x73 => {
    let imm_bits = (insn >> 20) & 0xFFF;
    let csr_addr = imm_bits as u32;
    match funct3 {
        0 => match imm_bits {
            0 => self.emit(op::SYSCALL, 0, 0, 0),  // ECALL
            1 => self.emit(op::HALT, 0, 0, 0),     // EBREAK
            _ => return Err(...),
        },
        1 | 2 | 3 | 5 | 6 | 7 => {
            // CSRR* — emit memory-mapped LD/ST
            let csr_byte_addr = 0x0000_F000u32 + csr_addr * 4;
            // Use scratch reg r12 (caller-saved, MBC convention).
            let scratch = 12;
            self.emit_load32(scratch, csr_byte_addr);
            // Read CSR (always done, even for CSRRW where rd may be x0).
            if mbc_rd != 0 {
                self.emit(op::LD, mbc_rd, scratch, 0);
            }
            // Write CSR (with possible RMW for CSRRS/CSRRC).
            let mbc_rs1 = self.map_register(rs1)?;
            if matches!(funct3, 1) {
                // CSRRW: just write rs1
                self.emit(op::ST, scratch, mbc_rs1, 0);
            } else if matches!(funct3, 2 | 3) {
                // CSRRS/CSRRC: read-modify-write
                let tmp = 13;
                self.emit(op::LD, tmp, scratch, 0);
                if funct3 == 2 {
                    self.emit(op::OR, tmp, mbc_rs1, 0);
                } else {
                    // AND with NOT(rs1)
                    self.emit(op::NOT, mbc_rs1, 0, 0);
                    self.emit(op::AND, tmp, mbc_rs1, 0);
                }
                self.emit(op::ST, scratch, tmp, 0);
            }
            // *I variants: same as 1/2/3 but rs1 field is a 5-bit imm.
            // ... handle by emitting MOVI to a tmp first.
        }
        _ => return Err(...),
    }
}
```

Estimated 60-80 LOC. Tests: 6 cases per CSR variant (read-only, write-only, RMW with various csr addresses including high CSRs like SATP=0x180).

### Linux's MRET/SRET come from xv6's kernel/riscv.h via inline asm

xv6's `w_satp(...)`, `r_mstatus()` etc. macros emit RV32 csrrw/csrr inline. The translator handles these once it learns CSRR. The MBC opcodes MRET (0x47) / SRET (0x48) we added in Phase 0.4 are emitted ONLY by code that uses inline `mret`/`sret` mnemonics — which xv6 does in start.c (replaced) and trampoline.S (we rewrote).

---

## Error 2 — 40 unsupported register usages (x16+)

### What

After stripping t3-t6 from kernelvec_mbc.S/trampoline_mbc.S, 40 x16+ usages remain. Some are RV32 standard-temp regs (gp=x3, tp=x4) that xv6 uses but MBC doesn't expose. Others are compiler-emitted from C (likely t3-t6 in inlined asm or compiler-generated stack-spill code).

### Two-step fix

1. **Audit which regs are referenced.** Run on the linked .elf:
   ```bash
   riscv64-unknown-elf-objdump -d kernel/kernel.elf | grep -oE '\b(x16|x17|x18|x19|x2[0-9]|x3[0-1]|t3|t4|t5|t6|s2|s3|s4|s5|s6|s7|s8|s9|s10|s11)\b' | sort | uniq -c
   ```
   This pinpoints the offending regs + their counts.

2. **Audit comes from .S files vs C codegen.**
   - .S file usage → rewrite in adapters/.
   - C codegen → CFLAGS gain whatever -ffixed-X is missing, OR the compiler is using x16+ for stack spills despite the flag (real C-codegen issue; might need -msave-restore or alternative ABI convention).

### Estimated effort
~4 hours: 30 min audit, 1 hr .S fixes, 2 hrs C-side investigation.

---

## Error 3 (post-Error 1+2) — runtime correctness

Once translator emits MBC for the kernel image, the resulting `xv6-mbc.mbc` will load via doom-runner. The first run will likely surface:

- **Stage-1 stub doesn't link with kernel.** `kernel-mbc.ld` places stage-1 in BOOT region but xv6 entry.S still expects to be at 0x80000000. Need the stub to call `_entry` from start_mbc.c after CSR setup.
- **xv6 expects RV64 page-table layout.** ADR-023's TLB design is Sv32-compatible per battle-plan §4 sub-phase 1.2 day-1 pair call — that will need verification.
- **Trapframe size mismatch.** We halved trapframe field offsets but xv6's C code (vm.c, proc.c) still uses sizeof()-based access; should be OK since structs are coherent, but verify with a test boot.

---

## Recommended order

1. **Error 1 first** (translator CSR handling). Without it nothing compiles to .mbc; can't even smoke-test.
2. **Error 2 second** (x16+ regs). Once translator runs, it tells us exactly which instructions are bad.
3. **Run + iterate** on Error 3 issues that surface only at boot time.

After all three, the Phase 1.1 first milestone — `cargo run -p doom-runner -- --kernel xv6-mbc.mbc` prints "xv6 booting..." and HALTs — should be reachable.
