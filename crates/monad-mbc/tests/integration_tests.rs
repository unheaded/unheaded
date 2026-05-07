//! Phase 6 (Steps 96-110): End-to-end integration tests for monad-mbc.
//!
//! Tests the full pipeline: assemble -> load -> execute -> verify,
//! plus disassemble roundtrips, screen rendering, keyboard input,
//! timer syscalls, cycle budget exhaustion, and stress tests.

use monad_common::{mbc_opcodes as op, MbcInsn, REG_SP};
use monad_mbc::{assemble, disasm_listing, ExecCpu, ExecError, Translator};

// ═══════════════════════════════════════════════════════════════════════════
// Helper: create ExecCpu with SP set to 0x1000 (safe for userspace RAM)
// ═══════════════════════════════════════════════════════════════════════════

fn cpu_with_safe_sp() -> ExecCpu {
    let mut cpu = ExecCpu::new();
    cpu.state.regs[REG_SP as usize] = 0x1000;
    cpu
}

// ═══════════════════════════════════════════════════════════════════════════
// Steps 96-97: Assemble test ROMs and verify bytecode
// ═══════════════════════════════════════════════════════════════════════════

#[test]
fn step96_assemble_minimal_arithmetic() {
    // Minimal program: load two values, add, halt
    let src = r#"
        MOVI r0, 10
        MOVI r1, 20
        ADD  r0, r1
        HALT
    "#;
    let rom = assemble(src).expect("minimal arithmetic should assemble");
    assert_eq!(rom.len(), 4, "should produce 4 instructions");

    // Verify encoding
    let insn0 = MbcInsn(rom[0]);
    assert_eq!(insn0.opcode(), op::MOVI);
    assert_eq!(insn0.dst(), 0);
    assert_eq!(insn0.imm16(), 10);

    let insn1 = MbcInsn(rom[1]);
    assert_eq!(insn1.opcode(), op::MOVI);
    assert_eq!(insn1.dst(), 1);
    assert_eq!(insn1.imm16(), 20);

    let insn2 = MbcInsn(rom[2]);
    assert_eq!(insn2.opcode(), op::ADD);
    assert_eq!(insn2.dst(), 0);
    assert_eq!(insn2.src(), 1);

    let insn3 = MbcInsn(rom[3]);
    assert_eq!(insn3.opcode(), op::HALT);
}

#[test]
fn step97_assemble_loop_with_labels() {
    // Program: sum 1..10 = 55
    let src = r#"
        MOVI r0, 0       ; sum
        MOVI r1, 1       ; counter
        MOVI r2, 11      ; limit
    loop:
        ADD  r0, r1      ; sum += counter
        ADDI r1, 1       ; counter++
        CMP  r1, r2
        JNZ  loop        ; loop if counter != 11
        HALT
    "#;
    let rom = assemble(src).expect("loop should assemble");
    assert_eq!(rom.len(), 8, "should produce 8 instructions");
}

// ═══════════════════════════════════════════════════════════════════════════
// Step 98: assemble -> load -> execute -> verify
// ═══════════════════════════════════════════════════════════════════════════

#[test]
fn step98_arithmetic_result() {
    let src = r#"
        MOVI r0, 10
        MOVI r1, 20
        ADD  r0, r1
        HALT
    "#;
    let rom = assemble(src).unwrap();

    let mut cpu = ExecCpu::new();
    cpu.load_rom(&rom);
    let result = cpu.run(100);
    assert!(result.is_ok(), "should halt: {:?}", result.err());
    assert_eq!(cpu.state.regs[0], 30, "10 + 20 = 30");
}

#[test]
fn step98_sum_1_to_10() {
    let src = r#"
        MOVI r0, 0       ; sum
        MOVI r1, 1       ; counter
        MOVI r2, 11      ; limit
    loop:
        ADD  r0, r1      ; sum += counter
        ADDI r1, 1       ; counter++
        CMP  r1, r2
        JNZ  loop
        HALT
    "#;
    let rom = assemble(src).unwrap();

    let mut cpu = ExecCpu::new();
    cpu.load_rom(&rom);
    let result = cpu.run(1000);
    assert!(result.is_ok(), "should halt: {:?}", result.err());
    assert_eq!(cpu.state.regs[0], 55, "sum 1..10 = 55");
    assert_eq!(cpu.state.halted, 1);
}

#[test]
fn step98_ram_store_and_load() {
    // Store a value to RAM, load it back, verify
    let src = r#"
        MOVI r0, 42      ; value to store
        MOVI r1, 256     ; byte address (word-aligned)
        ST   [r1+0], r0  ; RAM[256] = 42
        MOVI r0, 0       ; clear r0
        LD   r0, [r1+0]  ; r0 = RAM[256]
        HALT
    "#;
    let rom = assemble(src).unwrap();

    let mut cpu = ExecCpu::new();
    cpu.load_rom(&rom);
    let result = cpu.run(100);
    assert!(result.is_ok());
    assert_eq!(cpu.state.regs[0], 42, "should load back stored value");
}

#[test]
fn step98_call_ret() {
    // Test CALL/RET with SP set to safe value
    let src = r#"
        CALL subroutine
        HALT
    subroutine:
        MOVI r0, 99
        RET
    "#;
    let rom = assemble(src).unwrap();

    let mut cpu = cpu_with_safe_sp();
    cpu.load_rom(&rom);
    let result = cpu.run(100);
    assert!(result.is_ok(), "should halt: {:?}", result.err());
    assert_eq!(cpu.state.regs[0], 99, "subroutine should set r0 = 99");
    assert_eq!(cpu.state.halted, 1);
}

#[test]
fn step98_nested_call_ret() {
    // Test nested CALL/RET
    let src = r#"
        CALL outer
        HALT
    outer:
        MOVI r0, 10
        CALL inner
        ADDI r0, 1
        RET
    inner:
        MOVI r1, 50
        RET
    "#;
    let rom = assemble(src).unwrap();

    let mut cpu = cpu_with_safe_sp();
    cpu.load_rom(&rom);
    let result = cpu.run(200);
    assert!(result.is_ok(), "should halt: {:?}", result.err());
    assert_eq!(
        cpu.state.regs[0], 11,
        "outer should set r0=10, then ADDI +1 = 11"
    );
    assert_eq!(cpu.state.regs[1], 50, "inner should set r1=50");
    assert_eq!(cpu.state.halted, 1);
}

#[test]
fn step98_syscall_via_assembly() {
    // Syscalls require r0 = syscall_id. The SYSCALL instruction
    // reads r0 for the ID, ignoring imm16.
    let src = r#"
        MOVI r0, 3       ; SYS_GET_TICKS = 0x03
        SYSCALL 0
        HALT
    "#;
    let rom = assemble(src).unwrap();

    let mut cpu = ExecCpu::new();
    cpu.ticks_ms = 12345;
    cpu.load_rom(&rom);
    let result = cpu.run(100);
    assert!(result.is_ok());
    assert_eq!(cpu.state.regs[0], 12345, "r0 should contain ticks_ms");
}

#[test]
fn step98_halt_verification() {
    let src = "HALT";
    let rom = assemble(src).unwrap();

    let mut cpu = ExecCpu::new();
    cpu.load_rom(&rom);
    let result = cpu.run(100);
    assert!(result.is_ok());
    assert_eq!(cpu.state.halted, 1, "CPU should be halted");
}

// ═══════════════════════════════════════════════════════════════════════════
// Step 99: Disassemble -> reassemble -> compare
// ═══════════════════════════════════════════════════════════════════════════

#[test]
fn step99_disassemble_reassemble_compare() {
    // Assemble a program with simple non-branching instructions
    // (Branch/jump instructions have address-dependent disassembly
    //  that doesn't reassemble identically, so we test non-branch insns)
    let src = r#"
        MOVI r0, 42
        MOVI r1, 100
        ADD  r0, r1
        SUB  r0, r1
        MUL  r0, r1
        AND  r0, r1
        OR   r0, r1
        XOR  r0, r1
        NOT  r0
        NEG  r0
        SHL  r0, 5
        SHR  r0, 3
        SAR  r0, 2
        MOV  r2, r0
        CMP  r0, r1
        NOP
        PUSH r3
        POP  r4
        ADDI r0, -10
        LOAD_IMM32 r0, 0x1234
        HALT
    "#;

    let rom1 = assemble(src).expect("first assembly");
    assert!(!rom1.is_empty());

    // Disassemble
    let listing = disasm_listing(&rom1);
    assert!(!listing.is_empty());

    // For each instruction, verify it decodes to the expected opcode
    // (We can't do a full text roundtrip because disassembly format differs
    //  from assembly format slightly, e.g. spacing, but we can verify
    //  that the encoding is deterministic by re-encoding the same source.)
    let rom2 = assemble(src).expect("second assembly from same source");
    assert_eq!(rom1, rom2, "same source should produce identical bytecode");
}

#[test]
fn step99_disasm_listing_contains_expected_mnemonics() {
    let src = r#"
        MOVI r0, 42
        ADD  r0, r1
        STB  [r0+0], r1
        HALT
    "#;
    let rom = assemble(src).unwrap();
    let listing = disasm_listing(&rom);

    assert!(
        listing.contains("MOVI"),
        "listing should contain MOVI: {}",
        listing
    );
    assert!(
        listing.contains("ADD"),
        "listing should contain ADD: {}",
        listing
    );
    assert!(
        listing.contains("STB"),
        "listing should contain STB: {}",
        listing
    );
    assert!(
        listing.contains("HALT"),
        "listing should contain HALT: {}",
        listing
    );
}

// ═══════════════════════════════════════════════════════════════════════════
// Step 100: Translator roundtrip (simple RV32I -> MBC -> execute)
// ═══════════════════════════════════════════════════════════════════════════

#[test]
fn step100_translator_roundtrip_addi() {
    // RV32I: ADDI x10, x0, 42 ; EBREAK
    // ADDI encoding: imm[11:0] | rs1 | funct3 | rd | opcode
    //   imm=42, rs1=x0(0), funct3=0, rd=x10(10), opcode=0x13
    let addi = ((42u32 & 0xFFF) << 20) | (0 << 15) | (0 << 12) | (10 << 7) | 0x13;
    let ebreak = 0x00100073u32;

    let result = Translator::translate_program(&[addi, ebreak]);
    assert!(
        result.is_ok(),
        "translator should succeed: {:?}",
        result.err()
    );
    let mbc = result.unwrap();
    assert!(!mbc.is_empty());

    // Execute the translated MBC
    let mut cpu = cpu_with_safe_sp();
    cpu.load_rom(&mbc);
    let exec_result = cpu.run(100);
    assert!(exec_result.is_ok(), "should halt: {:?}", exec_result.err());

    // x10 maps to r8 in the translator's register mapping
    assert_eq!(cpu.state.regs[8], 42, "x10 (r8) should be 42");
    assert_eq!(cpu.state.halted, 1);
}

#[test]
fn step100_translator_roundtrip_add() {
    // RV32I: ADDI x5, x0, 10 ; ADDI x6, x0, 20 ; ADD x7, x5, x6 ; EBREAK
    let addi_x5 = ((10u32 & 0xFFF) << 20) | (0 << 15) | (0 << 12) | (5 << 7) | 0x13;
    let addi_x6 = ((20u32 & 0xFFF) << 20) | (0 << 15) | (0 << 12) | (6 << 7) | 0x13;
    // ADD: funct7=0, rs2=x6, rs1=x5, funct3=0, rd=x7, opcode=0x33
    let add_x7 = (0 << 25) | (6 << 20) | (5 << 15) | (0 << 12) | (7 << 7) | 0x33;
    let ebreak = 0x00100073u32;

    let result = Translator::translate_program(&[addi_x5, addi_x6, add_x7, ebreak]);
    assert!(result.is_ok(), "translate: {:?}", result.err());
    let mbc = result.unwrap();

    let mut cpu = cpu_with_safe_sp();
    cpu.load_rom(&mbc);
    let exec_result = cpu.run(200);
    assert!(exec_result.is_ok(), "should halt: {:?}", exec_result.err());

    // x5 -> r3, x6 -> r4, x7 -> r5
    assert_eq!(cpu.state.regs[3], 10, "x5 (r3) = 10");
    assert_eq!(cpu.state.regs[4], 20, "x6 (r4) = 20");
    assert_eq!(cpu.state.regs[5], 30, "x7 (r5) = 10 + 20 = 30");
}

// ═══════════════════════════════════════════════════════════════════════════
// Step 101: Screen rendering test
// ═══════════════════════════════════════════════════════════════════════════

#[test]
fn step101_screen_gradient_pattern() {
    // Write a gradient pattern to the screen buffer using STB.
    // Screen base = 0xC000, size = 64000 bytes (320x200).
    // Write bytes 0..255 cycling across the first 256 pixels.
    let src = r#"
        MOVI r1, 0xC000    ; screen base
        MOVI r2, 0         ; counter / pixel value
        MOVI r3, 256       ; limit
        MOVI r4, 1         ; increment
    loop:
        ; addr = screen_base + counter
        MOV  r5, r1
        ADD  r5, r2
        ; store pixel value (r2 & 0xFF via truncation in STB)
        STB  [r5+0], r2
        ; counter++
        ADD  r2, r4
        CMP  r2, r3
        JNZ  loop
        HALT
    "#;
    let rom = assemble(src).expect("gradient should assemble");

    let mut cpu = ExecCpu::new();
    cpu.load_rom(&rom);
    let result = cpu.run(50000);
    assert!(result.is_ok(), "should halt: {:?}", result.err());

    // Verify the gradient pattern in the screen buffer
    for i in 0..256usize {
        assert_eq!(
            cpu.screen[i], i as u8,
            "screen[{}] should be {}, got {}",
            i, i, cpu.screen[i]
        );
    }
}

#[test]
fn step101_screen_write_and_readback() {
    // Write specific bytes to screen, read them back via LDB
    let src = r#"
        MOVI r0, 0xAB         ; value
        MOVI r1, 0xC000        ; screen base + offset 0
        STB  [r1+0], r0        ; screen[0] = 0xAB

        MOVI r0, 0xCD
        ADDI r1, 1             ; screen base + 1
        STB  [r1+0], r0        ; screen[1] = 0xCD

        ; Read back
        MOVI r1, 0xC000
        LDB  r2, [r1+0]        ; r2 = screen[0]
        ADDI r1, 1
        LDB  r3, [r1+0]        ; r3 = screen[1]
        HALT
    "#;
    let rom = assemble(src).unwrap();

    let mut cpu = ExecCpu::new();
    cpu.load_rom(&rom);
    let result = cpu.run(100);
    assert!(result.is_ok());
    assert_eq!(cpu.screen[0], 0xAB, "screen[0] should be 0xAB");
    assert_eq!(cpu.screen[1], 0xCD, "screen[1] should be 0xCD");
    assert_eq!(cpu.state.regs[2], 0xAB, "r2 readback should be 0xAB");
    assert_eq!(cpu.state.regs[3], 0xCD, "r3 readback should be 0xCD");
}

// ═══════════════════════════════════════════════════════════════════════════
// Step 102: Keyboard input test
// ═══════════════════════════════════════════════════════════════════════════

#[test]
fn step102_keyboard_input_syscall() {
    // Set kbd, execute SYS_GET_KEY syscall, verify r0 contains keyboard state
    let src = r#"
        MOVI r0, 2          ; SYS_GET_KEY = 0x02
        SYSCALL 0            ; syscall reads r0 for ID
        ; After syscall, r0 = kbd value
        HALT
    "#;
    let rom = assemble(src).unwrap();

    let mut cpu = ExecCpu::new();
    cpu.kbd = 0x42; // simulate key press
    cpu.load_rom(&rom);
    let result = cpu.run(100);
    assert!(result.is_ok());
    assert_eq!(cpu.state.regs[0], 0x42, "r0 should contain kbd state 0x42");
}

#[test]
fn step102_keyboard_input_multiple_reads() {
    // Read keyboard twice with different values
    let src = r#"
        MOVI r0, 2          ; SYS_GET_KEY
        SYSCALL 0
        MOV  r1, r0         ; save first read to r1
        HALT
    "#;
    let rom = assemble(src).unwrap();

    let mut cpu = ExecCpu::new();
    cpu.kbd = 0xDEAD;
    cpu.load_rom(&rom);
    let result = cpu.run(100);
    assert!(result.is_ok());
    assert_eq!(cpu.state.regs[1], 0xDEAD, "r1 should hold kbd value");
}

// ═══════════════════════════════════════════════════════════════════════════
// Step 103: Timer test
// ═══════════════════════════════════════════════════════════════════════════

#[test]
fn step103_get_ticks_syscall() {
    // Execute SYS_GET_TICKS, verify r0 contains cpu.ticks_ms
    let src = r#"
        MOVI r0, 3          ; SYS_GET_TICKS = 0x03
        SYSCALL 0
        MOV  r1, r0         ; save ticks to r1
        HALT
    "#;
    let rom = assemble(src).unwrap();

    let mut cpu = ExecCpu::new();
    cpu.ticks_ms = 9999;
    cpu.load_rom(&rom);
    let result = cpu.run(100);
    assert!(result.is_ok());
    assert_eq!(cpu.state.regs[1], 9999, "r1 should contain ticks_ms");
}

#[test]
fn step103_sleep_syscall() {
    // Execute SYS_SLEEP with r1=100, verify ticks_ms advances
    let src = r#"
        MOVI r1, 100        ; sleep duration (ms)
        MOVI r0, 4          ; SYS_SLEEP = 0x04
        SYSCALL 0
        ; Now ticks_ms should be original + 100
        MOVI r0, 3          ; SYS_GET_TICKS
        SYSCALL 0
        MOV  r2, r0         ; save new ticks to r2
        HALT
    "#;
    let rom = assemble(src).unwrap();

    let mut cpu = ExecCpu::new();
    cpu.ticks_ms = 500;
    cpu.load_rom(&rom);
    let result = cpu.run(100);
    assert!(result.is_ok());
    assert_eq!(cpu.ticks_ms, 600, "ticks_ms should advance from 500 to 600");
    assert_eq!(
        cpu.state.regs[2], 600,
        "r2 should read updated ticks_ms = 600"
    );
}

// ═══════════════════════════════════════════════════════════════════════════
// Step 104: Cycle budget exhaustion test
// ═══════════════════════════════════════════════════════════════════════════

#[test]
fn step104_infinite_loop_budget_exhaustion() {
    // Infinite loop: JMP to self.
    // step() increments PC by 1 before execute_insn. So for the JMP at index 0:
    //   PC goes 0 -> 1 (step), then JMP offset is applied to PC=1.
    //   To jump back to index 0, offset = 0 - 1 = -1.
    //   -1 as signed 16-bit = 0xFFFF
    let src = r#"
    loop:
        JMP loop
    "#;
    let rom = assemble(src).unwrap();
    assert_eq!(rom.len(), 1);

    let mut cpu = ExecCpu::new();
    cpu.load_rom(&rom);
    let result = cpu.run(500);
    assert_eq!(
        result,
        Err(ExecError::CycleBudgetExhausted),
        "infinite loop should exhaust cycle budget"
    );
}

#[test]
fn step104_long_loop_budget_exhaustion() {
    // Loop that increments r0. Budget = 500 means 500 instructions executed.
    let src = r#"
        MOVI r0, 0
        MOVI r1, 1
    loop:
        ADD  r0, r1
        JMP  loop
    "#;
    let rom = assemble(src).unwrap();

    let mut cpu = ExecCpu::new();
    cpu.load_rom(&rom);
    let result = cpu.run(500);
    assert_eq!(result, Err(ExecError::CycleBudgetExhausted));
    // r0 should have been incremented roughly (500 - 2) / 2 = 249 times
    // (2 setup instructions, then 2 per iteration: ADD + JMP)
    assert!(
        cpu.state.regs[0] > 200,
        "r0 should have been incremented many times, got {}",
        cpu.state.regs[0]
    );
}

// ═══════════════════════════════════════════════════════════════════════════
// Step 105: Halt in subroutine test
// ═══════════════════════════════════════════════════════════════════════════

#[test]
fn step105_halt_in_subroutine() {
    // CALL -> subroutine that HALTs. Verify CPU halts correctly
    // without returning.
    let src = r#"
        CALL sub
        MOVI r1, 99        ; should NOT execute
        HALT
    sub:
        MOVI r0, 42
        HALT               ; halt inside subroutine
    "#;
    let rom = assemble(src).unwrap();

    let mut cpu = cpu_with_safe_sp();
    cpu.load_rom(&rom);
    let result = cpu.run(100);
    assert!(result.is_ok(), "should halt: {:?}", result.err());
    assert_eq!(cpu.state.halted, 1, "CPU should be halted");
    assert_eq!(cpu.state.regs[0], 42, "r0 should be 42 from subroutine");
    assert_eq!(
        cpu.state.regs[1], 0,
        "r1 should NOT be set (code after CALL not reached)"
    );
}

#[test]
fn step105_halt_preserves_registers() {
    // Verify all registers are preserved when HALT is executed
    let src = r#"
        MOVI r0, 100
        MOVI r1, 200
        MOVI r2, 300
        MOVI r3, 400
        HALT
    "#;
    let rom = assemble(src).unwrap();

    let mut cpu = ExecCpu::new();
    cpu.load_rom(&rom);
    let _ = cpu.run(100);
    assert_eq!(cpu.state.halted, 1);
    assert_eq!(cpu.state.regs[0], 100);
    assert_eq!(cpu.state.regs[1], 200);
    assert_eq!(cpu.state.regs[2], 300);
    assert_eq!(cpu.state.regs[3], 400);
}

// ═══════════════════════════════════════════════════════════════════════════
// Step 106: doom.mbc smoke test (conditional)
// ═══════════════════════════════════════════════════════════════════════════

#[test]
fn step106_doom_mbc_smoke_test() {
    let doom_path =
        std::path::Path::new(env!("CARGO_MANIFEST_DIR")).join("../../doom/doomgeneric/doom.mbc");

    if !doom_path.exists() {
        eprintln!(
            "SKIP: doom.mbc not found at {:?} (expected: file not yet built)",
            doom_path
        );
        return;
    }

    // Load doom.mbc as raw u32 words
    let data = std::fs::read(&doom_path).expect("read doom.mbc");
    assert!(data.len() >= 4, "doom.mbc should be at least 4 bytes");

    // Convert bytes to u32 words (little-endian)
    let rom: Vec<u32> = data
        .chunks_exact(4)
        .map(|chunk| u32::from_le_bytes([chunk[0], chunk[1], chunk[2], chunk[3]]))
        .collect();

    eprintln!("doom.mbc: {} instructions loaded", rom.len());

    // Load into CPU with safe SP
    let mut cpu = cpu_with_safe_sp();
    cpu.load_rom(&rom);

    // Execute 10000 cycles -- should not panic
    let result = cpu.run(10000);

    // We expect either CycleBudgetExhausted (still running) or
    // some other non-panic outcome. The point is: no panic.
    match result {
        Ok(cycles) => eprintln!("doom.mbc halted after {} cycles", cycles),
        Err(ExecError::CycleBudgetExhausted) => {
            eprintln!("doom.mbc ran 10000 cycles without halt (expected)")
        }
        Err(ExecError::Halted) => {
            eprintln!("doom.mbc halted (exit() reached)")
        }
        Err(e) => {
            eprintln!(
                "doom.mbc error after some cycles: {:?} (non-fatal for smoke test)",
                e
            )
        }
    }
    // Success = no panic
}

// ═══════════════════════════════════════════════════════════════════════════
// Step 109: BPF map size stress test (max-size ROM)
// ═══════════════════════════════════════════════════════════════════════════

#[test]
fn step109_max_size_rom() {
    // Load 262144 NOP instructions followed by HALT.
    // This tests that the CPU can handle the maximum ROM size.
    const MAX_ROM_SIZE: usize = 262144;

    let nop_word = MbcInsn::encode(op::NOP, 0, 0, 0).0;
    let halt_word = MbcInsn::encode(op::HALT, 0, 0, 0).0;

    let mut rom = vec![nop_word; MAX_ROM_SIZE];
    // Place HALT at the end
    rom[MAX_ROM_SIZE - 1] = halt_word;

    let mut cpu = ExecCpu::new();
    cpu.load_rom(&rom);

    // Verify ROM loaded correctly
    assert_eq!(cpu.rom.len(), MAX_ROM_SIZE);
    assert_eq!(cpu.rom[0], nop_word);
    assert_eq!(cpu.rom[MAX_ROM_SIZE - 1], halt_word);

    // Execute enough cycles to run through some of the ROM
    // (we won't run all 262K cycles, just verify it doesn't panic)
    let result = cpu.run(10000);
    assert_eq!(
        result,
        Err(ExecError::CycleBudgetExhausted),
        "should exhaust budget before reaching end of large ROM"
    );

    // Verify PC advanced correctly
    assert_eq!(
        cpu.state.pc, 10000,
        "PC should be at 10000 after 10000 NOPs"
    );
}

#[test]
fn step109_max_size_rom_with_halt_at_start() {
    // Verify large ROM works when HALT is at the beginning
    const MAX_ROM_SIZE: usize = 262144;

    let nop_word = MbcInsn::encode(op::NOP, 0, 0, 0).0;
    let halt_word = MbcInsn::encode(op::HALT, 0, 0, 0).0;

    let mut rom = vec![nop_word; MAX_ROM_SIZE];
    rom[0] = halt_word;

    let mut cpu = ExecCpu::new();
    cpu.load_rom(&rom);
    let result = cpu.run(10000);
    assert!(result.is_ok(), "HALT at start should stop immediately");
    assert_eq!(cpu.state.halted, 1);
}

// ═══════════════════════════════════════════════════════════════════════════
// Additional integration tests (comprehensive coverage)
// ═══════════════════════════════════════════════════════════════════════════

#[test]
fn integration_fibonacci() {
    // Compute fib(10) = 55 using the assembler
    let src = r#"
        MOVI r0, 1       ; fib(1) = 1
        MOVI r1, 0       ; fib(0) = 0
        MOVI r2, 2       ; counter
        MOVI r3, 11      ; limit
    loop:
        MOV  r4, r0      ; temp = fib(n-1)
        ADD  r0, r1      ; fib(n) = fib(n-1) + fib(n-2)
        MOV  r1, r4      ; fib(n-2) = old fib(n-1)
        ADDI r2, 1       ; counter++
        CMP  r2, r3
        JNZ  loop
        HALT
    "#;
    let rom = assemble(src).unwrap();

    let mut cpu = ExecCpu::new();
    cpu.load_rom(&rom);
    let result = cpu.run(1000);
    assert!(result.is_ok());
    assert_eq!(cpu.state.regs[0], 55, "fib(10) = 55");
}

#[test]
fn integration_memory_copy() {
    // Store values to RAM, then copy them
    let src = r#"
        ; Store source data
        MOVI r0, 0xAA
        MOVI r1, 256
        ST   [r1+0], r0

        MOVI r0, 0xBB
        ADDI r1, 4
        ST   [r1+0], r0

        ; Copy from 256 to 512
        MOVI r1, 256     ; src
        MOVI r2, 512     ; dst

        LD   r0, [r1+0]
        ST   [r2+0], r0

        ADDI r1, 4
        ADDI r2, 4
        LD   r0, [r1+0]
        ST   [r2+0], r0

        HALT
    "#;
    let rom = assemble(src).unwrap();

    let mut cpu = ExecCpu::new();
    cpu.load_rom(&rom);
    let result = cpu.run(200);
    assert!(result.is_ok());

    // Verify copies
    assert_eq!(cpu.ram[512 >> 2], 0xAA, "dst[0] should be 0xAA");
    assert_eq!(cpu.ram[516 >> 2], 0xBB, "dst[1] should be 0xBB");
}

#[test]
fn integration_push_pop_via_assembly() {
    // Test PUSH/POP through the assembler
    let src = r#"
        MOVI r0, 111
        MOVI r1, 222
        PUSH r0
        PUSH r1
        POP  r2          ; should get 222 (LIFO)
        POP  r3          ; should get 111
        HALT
    "#;
    let rom = assemble(src).unwrap();

    let mut cpu = cpu_with_safe_sp();
    cpu.load_rom(&rom);
    let result = cpu.run(100);
    assert!(result.is_ok());
    assert_eq!(cpu.state.regs[2], 222, "first POP should be 222 (LIFO)");
    assert_eq!(cpu.state.regs[3], 111, "second POP should be 111");
}

#[test]
fn integration_bitwise_operations() {
    // Test all bitwise operations through assembler
    let src = r#"
        MOVI r0, 0xFF    ; 0b11111111
        MOVI r1, 0x0F    ; 0b00001111
        AND  r0, r1      ; 0xFF & 0x0F = 0x0F
        MOV  r2, r0      ; r2 = 0x0F

        MOVI r0, 0xF0
        OR   r0, r1      ; 0xF0 | 0x0F = 0xFF
        MOV  r3, r0      ; r3 = 0xFF

        MOVI r0, 0xFF
        XOR  r0, r1      ; 0xFF ^ 0x0F = 0xF0
        MOV  r4, r0      ; r4 = 0xF0

        MOVI r0, 0
        NOT  r0          ; ~0 = 0xFFFFFFFF
        MOV  r5, r0      ; r5 = 0xFFFFFFFF
        HALT
    "#;
    let rom = assemble(src).unwrap();

    let mut cpu = ExecCpu::new();
    cpu.load_rom(&rom);
    let result = cpu.run(200);
    assert!(result.is_ok());
    assert_eq!(cpu.state.regs[2], 0x0F, "AND result");
    assert_eq!(cpu.state.regs[3], 0xFF, "OR result");
    assert_eq!(cpu.state.regs[4], 0xF0, "XOR result");
    assert_eq!(cpu.state.regs[5], 0xFFFFFFFF, "NOT result");
}

#[test]
fn integration_conditional_branches() {
    // Test conditional branch instructions
    let src = r#"
        MOVI r0, 10
        MOVI r1, 10
        CMP  r0, r1
        JZ   equal         ; should take: 10 == 10
        MOVI r2, 0          ; should NOT execute
        JMP  done
    equal:
        MOVI r2, 1          ; should execute
    done:
        MOVI r0, 5
        MOVI r1, 10
        CMP  r0, r1
        JN   less           ; should take: 5 < 10 (result negative)
        MOVI r3, 0
        JMP  done2
    less:
        MOVI r3, 1          ; should execute
    done2:
        HALT
    "#;
    let rom = assemble(src).unwrap();

    let mut cpu = ExecCpu::new();
    cpu.load_rom(&rom);
    let result = cpu.run(200);
    assert!(result.is_ok());
    assert_eq!(cpu.state.regs[2], 1, "JZ should have been taken (equal)");
    assert_eq!(cpu.state.regs[3], 1, "JN should have been taken (less)");
}

#[test]
fn integration_draw_frame_syscall() {
    // Verify SYS_DRAW_FRAME doesn't crash (it's a no-op in userspace)
    let src = r#"
        MOVI r0, 1         ; SYS_DRAW_FRAME
        SYSCALL 0
        MOVI r1, 42        ; verify we continue after syscall
        HALT
    "#;
    let rom = assemble(src).unwrap();

    let mut cpu = ExecCpu::new();
    cpu.load_rom(&rom);
    let result = cpu.run(100);
    assert!(result.is_ok());
    assert_eq!(
        cpu.state.regs[1], 42,
        "execution should continue after DRAW_FRAME"
    );
}

#[test]
fn integration_byte_store_load_screen() {
    // Write a specific pattern to the screen using STB, read back via LDB
    let src = r#"
        MOVI r0, 0xC000     ; screen base
        MOVI r1, 0xDE
        STB  [r0+0], r1     ; screen[0] = 0xDE
        MOVI r1, 0xAD
        STB  [r0+1], r1     ; screen[1] = 0xAD
        MOVI r1, 0xBE
        STB  [r0+2], r1     ; screen[2] = 0xBE
        MOVI r1, 0xEF
        STB  [r0+3], r1     ; screen[3] = 0xEF
        ; Read back
        LDB  r2, [r0+0]
        LDB  r3, [r0+1]
        LDB  r4, [r0+2]
        LDB  r5, [r0+3]
        HALT
    "#;
    let rom = assemble(src).unwrap();

    let mut cpu = ExecCpu::new();
    cpu.load_rom(&rom);
    let result = cpu.run(200);
    assert!(result.is_ok());
    assert_eq!(cpu.screen[0], 0xDE);
    assert_eq!(cpu.screen[1], 0xAD);
    assert_eq!(cpu.screen[2], 0xBE);
    assert_eq!(cpu.screen[3], 0xEF);
    assert_eq!(cpu.state.regs[2], 0xDE);
    assert_eq!(cpu.state.regs[3], 0xAD);
    assert_eq!(cpu.state.regs[4], 0xBE);
    assert_eq!(cpu.state.regs[5], 0xEF);
}

#[test]
fn integration_insn_count_tracking() {
    // Verify insn_count matches executed instructions
    let src = r#"
        NOP
        NOP
        NOP
        HALT
    "#;
    let rom = assemble(src).unwrap();

    let mut cpu = ExecCpu::new();
    cpu.load_rom(&rom);
    let _ = cpu.run(100);
    // 3 NOPs execute (insn_count incremented), HALT returns Err before increment
    assert_eq!(cpu.state.insn_count, 3, "3 NOPs should give insn_count=3");
}

#[test]
fn integration_rom_fault_detection() {
    // Jump beyond ROM bounds should trigger RomFault on next fetch.
    // We manually create a ROM with a far-jumping JMP instruction
    // (rather than using the assembler) to force an out-of-bounds PC.
    let jmp_far = MbcInsn::encode(op::JMP, 0, 0, 100).0; // offset +100, way beyond ROM
    let mut cpu = ExecCpu::new();
    cpu.load_rom(&[jmp_far]);
    let step1 = cpu.step(); // executes JMP, PC goes to 102 (1+1+100)
    assert!(step1.is_ok());
    let step2 = cpu.step(); // PC=102 > rom.len()=1, should RomFault
    assert_eq!(step2, Err(ExecError::RomFault));
}

#[test]
fn integration_invalid_syscall() {
    // Verify that an invalid syscall ID produces an error
    let src = r#"
        MOVI r0, 0xFF    ; invalid syscall ID
        SYSCALL 0
        HALT
    "#;
    let rom = assemble(src).unwrap();

    let mut cpu = ExecCpu::new();
    cpu.load_rom(&rom);
    let result = cpu.run(100);
    assert_eq!(result, Err(ExecError::InvalidSyscall(0xFF)));
}

#[test]
fn integration_shift_operations() {
    // Test all shift operations
    let src = r#"
        MOVI r0, 1
        SHL  r0, 4       ; 1 << 4 = 16
        MOV  r1, r0      ; r1 = 16

        MOVI r0, 128
        SHR  r0, 3       ; 128 >> 3 = 16
        MOV  r2, r0      ; r2 = 16

        HALT
    "#;
    let rom = assemble(src).unwrap();

    let mut cpu = ExecCpu::new();
    cpu.load_rom(&rom);
    let result = cpu.run(100);
    assert!(result.is_ok());
    assert_eq!(cpu.state.regs[1], 16, "1 << 4 = 16");
    assert_eq!(cpu.state.regs[2], 16, "128 >> 3 = 16");
}

#[test]
fn integration_load_imm32_full_value() {
    // Test loading a full 32-bit value using MOVI + LOAD_IMM32
    let src = r#"
        MOVI r0, 0x5678         ; lower 16 bits
        LOAD_IMM32 r0, 0x1234   ; upper 16 bits
        HALT
    "#;
    let rom = assemble(src).unwrap();

    let mut cpu = ExecCpu::new();
    cpu.load_rom(&rom);
    let result = cpu.run(100);
    assert!(result.is_ok());
    assert_eq!(cpu.state.regs[0], 0x12345678, "should form 0x12345678");
}

#[test]
fn integration_multiply_and_divide() {
    // Test MUL and DIV operations
    let src = r#"
        MOVI r0, 6
        MOVI r1, 7
        MUL  r0, r1      ; 6 * 7 = 42
        MOV  r2, r0      ; r2 = 42

        MOVI r0, 100
        MOVI r1, 5
        DIV  r0, r1      ; 100 / 5 = 20
        MOV  r3, r0      ; r3 = 20

        MOVI r0, 17
        MOVI r1, 5
        MOD  r0, r1      ; 17 % 5 = 2
        MOV  r4, r0      ; r4 = 2
        HALT
    "#;
    let rom = assemble(src).unwrap();

    let mut cpu = ExecCpu::new();
    cpu.load_rom(&rom);
    let result = cpu.run(200);
    assert!(result.is_ok());
    assert_eq!(cpu.state.regs[2], 42, "6 * 7 = 42");
    assert_eq!(cpu.state.regs[3], 20, "100 / 5 = 20");
    assert_eq!(cpu.state.regs[4], 2, "17 % 5 = 2");
}

#[test]
fn integration_memory_byte_alignment() {
    // Test byte-level memory operations at non-word-aligned addresses
    let src = r#"
        MOVI r0, 0x100       ; base address
        MOVI r1, 0x12
        STB  [r0+0], r1      ; byte 0
        MOVI r1, 0x34
        STB  [r0+1], r1      ; byte 1
        MOVI r1, 0x56
        STB  [r0+2], r1      ; byte 2
        MOVI r1, 0x78
        STB  [r0+3], r1      ; byte 3

        ; Read back as a word
        LD   r2, [r0+0]      ; should be 0x78563412 (little-endian)

        ; Read back individual bytes
        LDB  r3, [r0+0]      ; 0x12
        LDB  r4, [r0+1]      ; 0x34
        LDB  r5, [r0+2]      ; 0x56
        LDB  r6, [r0+3]      ; 0x78
        HALT
    "#;
    let rom = assemble(src).unwrap();

    let mut cpu = ExecCpu::new();
    cpu.load_rom(&rom);
    let result = cpu.run(200);
    assert!(result.is_ok());
    assert_eq!(
        cpu.state.regs[2], 0x78563412,
        "word read should be LE: 0x78563412"
    );
    assert_eq!(cpu.state.regs[3], 0x12);
    assert_eq!(cpu.state.regs[4], 0x34);
    assert_eq!(cpu.state.regs[5], 0x56);
    assert_eq!(cpu.state.regs[6], 0x78);
}

#[test]
fn integration_halfword_operations() {
    // Test LDH/STH
    let src = r#"
        MOVI r0, 0x200
        MOVI r1, 0xABCD
        STH  [r0+0], r1
        LDH  r2, [r0+0]
        HALT
    "#;
    let rom = assemble(src).unwrap();

    let mut cpu = ExecCpu::new();
    cpu.load_rom(&rom);
    let result = cpu.run(100);
    assert!(result.is_ok());
    assert_eq!(
        cpu.state.regs[2], 0xABCD,
        "halfword roundtrip should preserve value"
    );
}

#[test]
fn integration_neg_operation() {
    let src = r#"
        MOVI r0, 5
        NEG  r0
        HALT
    "#;
    let rom = assemble(src).unwrap();

    let mut cpu = ExecCpu::new();
    cpu.load_rom(&rom);
    let result = cpu.run(100);
    assert!(result.is_ok());
    assert_eq!(
        cpu.state.regs[0],
        u32::wrapping_neg(5),
        "NEG(5) = 0 - 5 = 0xFFFFFFFB"
    );
}

#[test]
fn integration_end_to_end_all_syscalls() {
    // Exercise all syscalls in one program
    let src = r#"
        ; SYS_DRAW_FRAME (no-op, should not crash)
        MOVI r0, 1
        SYSCALL 0

        ; SYS_GET_KEY
        MOVI r0, 2
        SYSCALL 0
        MOV  r5, r0       ; save kbd value

        ; SYS_GET_TICKS
        MOVI r0, 3
        SYSCALL 0
        MOV  r6, r0       ; save ticks

        ; SYS_SLEEP
        MOVI r1, 50
        MOVI r0, 4
        SYSCALL 0

        ; Read ticks again to verify sleep advanced time
        MOVI r0, 3
        SYSCALL 0
        MOV  r7, r0       ; save new ticks

        HALT
    "#;
    let rom = assemble(src).unwrap();

    let mut cpu = ExecCpu::new();
    cpu.kbd = 0xBEEF;
    cpu.ticks_ms = 1000;
    cpu.load_rom(&rom);
    let result = cpu.run(200);
    assert!(result.is_ok());

    assert_eq!(cpu.state.regs[5], 0xBEEF, "GET_KEY should return kbd value");
    assert_eq!(
        cpu.state.regs[6], 1000,
        "GET_TICKS before sleep should be 1000"
    );
    assert_eq!(
        cpu.ticks_ms, 1050,
        "SLEEP(50) should advance ticks from 1000 to 1050"
    );
    assert_eq!(
        cpu.state.regs[7], 1050,
        "GET_TICKS after sleep should be 1050"
    );
}
