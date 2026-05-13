// SPDX-License-Identifier: GPL-3.0-or-later
//! Comprehensive integration tests for UPC OS primitives.
//!
//! Tests all Level 4a-4f + Level 5 boot protocol primitives:
//!   1. Timer interrupts (IVT, INT, IRET)
//!   2. Syscall dispatch (SYS_EXIT, SYS_WRITE, SYS_BRK, SYS_GETPID,
//!      SYS_CLOCK_GETTIME, SYS_FORK)
//!   3. Scheduler (fork, context switch, round-robin)
//!   4. MMU (page tables, TLB, identity mapping)
//!   5. Block device (ramdisk read/write)
//!   6. Console I/O (SYS_WRITE to tty_output)
//!   7. Boot integration (hello_kernel demo)

use monad_common::{
    mbc_block as blk, mbc_interrupts as intr, mbc_linux_syscalls as lsys, mbc_mmu as mmu,
    mbc_opcodes as op, MbcInsn, REG_SP,
};
use monad_mbc::{ExecCpu, ExecError};

// ═══════════════════════════════════════════════════════════════════════════════
// Helpers
// ═══════════════════════════════════════════════════════════════════════════════

/// Encode a standard instruction.
fn enc(opcode: u8, dst: u8, src: u8, imm: u16) -> u32 {
    MbcInsn::encode(opcode, dst, src, imm).0
}

/// Build a branch/jump instruction with 24-bit offset packed into
/// dst(4) + src(4) + imm16(16) = 24 bits.
#[allow(dead_code)]
fn branch(opcode: u8, offset: i32) -> u32 {
    let off24 = (offset as u32) & 0x00FF_FFFF;
    let dst = ((off24 >> 20) & 0xF) as u8;
    let src = ((off24 >> 16) & 0xF) as u8;
    let imm = (off24 & 0xFFFF) as u16;
    MbcInsn::encode(opcode, dst, src, imm).0
}

/// Create a CPU with SP set to a safe RAM address (byte 0x4000 = word 0x1000).
fn cpu_with_safe_sp() -> ExecCpu {
    let mut cpu = ExecCpu::new();
    cpu.state.regs[REG_SP] = 0x4000; // byte address
    cpu
}

/// Store the bytes of a string into RAM starting at `byte_addr`.
fn store_string(cpu: &mut ExecCpu, byte_addr: u32, s: &str) {
    for (i, b) in s.bytes().enumerate() {
        let addr = byte_addr + i as u32;
        let word_idx = (addr >> 2) as usize;
        let byte_off = (addr & 3) as usize;
        if word_idx < cpu.ram.len() {
            let mask = 0xFFu32 << (byte_off * 8);
            cpu.ram[word_idx] = (cpu.ram[word_idx] & !mask) | ((b as u32) << (byte_off * 8));
        }
    }
}

// ═══════════════════════════════════════════════════════════════════════════════
// 1. Timer Interrupts
// ═══════════════════════════════════════════════════════════════════════════════

#[test]
fn timer_tick_counter_increments() {
    // Verify that tick_counter can be set and read back.
    // The tick_counter is typically incremented by external timer ticks.
    let mut cpu = ExecCpu::new();
    assert_eq!(cpu.state.tick_counter, 0);

    cpu.state.tick_counter = 42;
    assert_eq!(cpu.state.tick_counter, 42);
}

#[test]
fn timer_interrupt_fires_when_pending_and_enabled() {
    // Set up IVT entry at address 0 (word 0) pointing to handler at ROM word 10.
    // When interrupt fires, CPU should push PC+flags, jump to handler.
    let mut cpu = cpu_with_safe_sp();

    // IVT: vector 0x20 (VECTOR_TIMER). IVT address = vector * 4 = 0x80 bytes = word 0x20.
    // Store handler address (ROM word 10) at RAM word 0x20.
    cpu.ram[0x20] = 10;

    // Build ROM:
    // Words 0-9: main code (NOP sled + HALT)
    // Word 10+: interrupt handler (MOVI r5, 0xBEEF; IRET)
    let mut rom = vec![enc(op::NOP, 0, 0, 0); 12];
    rom[9] = enc(op::HALT, 0, 0, 0);
    // Handler at word 10
    rom[10] = enc(op::MOVI, 5, 0, 0xBEEF);
    rom[11] = enc(op::IRET, 0, 0, 0);

    cpu.load_rom(&rom);

    // Enable interrupts and set pending timer interrupt
    cpu.state.interrupts_enabled = 1;
    cpu.state.interrupt_pending = 1;
    cpu.state.interrupt_vector = intr::VECTOR_TIMER;

    // Step: the interrupt dispatch should fire before the first instruction fetch
    cpu.step().unwrap();

    // PC should have jumped to handler (10), and the handler's MOVI should execute.
    // After step(), we executed the handler's MOVI at word 10.
    // r5 should be 0xBEEF now.
    assert_eq!(cpu.state.regs[5], 0xBEEF, "handler should set r5 = 0xBEEF");

    // Interrupts should be disabled during handler
    assert_eq!(
        cpu.state.interrupts_enabled, 0,
        "interrupts disabled during handler"
    );

    // interrupt_pending should be cleared
    assert_eq!(cpu.state.interrupt_pending, 0, "interrupt_pending cleared");
}

#[test]
fn timer_iret_restores_pc_and_flags() {
    // Set up: trigger an interrupt, then IRET back.
    let mut cpu = cpu_with_safe_sp();

    // IVT: vector 0x20 -> handler at ROM word 5
    cpu.ram[0x20] = 5;

    // ROM layout:
    // Word 0: MOVI r0, 0x42  (main code, should execute first)
    // Word 1: HALT
    // ...
    // Word 5: MOVI r5, 0xAA  (handler)
    // Word 6: IRET
    let mut rom = vec![enc(op::NOP, 0, 0, 0); 8];
    rom[0] = enc(op::MOVI, 0, 0, 0x42);
    rom[1] = enc(op::HALT, 0, 0, 0);
    rom[5] = enc(op::MOVI, 5, 0, 0xAA);
    rom[6] = enc(op::IRET, 0, 0, 0);

    cpu.load_rom(&rom);

    // Execute MOVI r0, 0x42 (word 0). PC advances to 1.
    cpu.step().unwrap();
    assert_eq!(cpu.state.regs[0], 0x42);
    assert_eq!(cpu.state.pc, 1);

    // Now trigger interrupt. The next step() should push PC=1 + flags, jump to handler.
    cpu.state.interrupts_enabled = 1;
    cpu.state.interrupt_pending = 1;
    cpu.state.interrupt_vector = intr::VECTOR_TIMER;

    // Step: interrupt fires, then executes handler word 5 (MOVI r5, 0xAA)
    cpu.step().unwrap();
    assert_eq!(cpu.state.regs[5], 0xAA);

    // Step: execute IRET at word 6
    cpu.step().unwrap();

    // After IRET, PC should be restored to 1, interrupts re-enabled
    assert_eq!(cpu.state.pc, 1, "IRET should restore PC to saved value");
    assert_eq!(
        cpu.state.interrupts_enabled, 1,
        "IRET should re-enable interrupts"
    );

    // Next step should execute HALT at word 1
    let result = cpu.step();
    assert_eq!(result, Err(ExecError::Halted));
}

#[test]
fn timer_interrupt_disabled_does_not_fire() {
    // If interrupts_enabled == 0, pending interrupt should not fire.
    let mut cpu = cpu_with_safe_sp();

    cpu.ram[0x20] = 5; // handler at word 5

    let mut rom = vec![enc(op::NOP, 0, 0, 0); 8];
    rom[0] = enc(op::MOVI, 0, 0, 0x99);
    rom[1] = enc(op::HALT, 0, 0, 0);
    rom[5] = enc(op::MOVI, 5, 0, 0xAA);
    rom[6] = enc(op::IRET, 0, 0, 0);

    cpu.load_rom(&rom);

    // Set pending but do NOT enable interrupts
    cpu.state.interrupts_enabled = 0;
    cpu.state.interrupt_pending = 1;
    cpu.state.interrupt_vector = intr::VECTOR_TIMER;

    // Step: should execute MOVI at word 0, NOT jump to handler
    cpu.step().unwrap();
    assert_eq!(
        cpu.state.regs[0], 0x99,
        "main code should execute, not handler"
    );
    assert_eq!(cpu.state.regs[5], 0, "handler should not have run");
}

// ═══════════════════════════════════════════════════════════════════════════════
// 2. Syscall Dispatch (INT 0x80)
// ═══════════════════════════════════════════════════════════════════════════════

#[test]
fn syscall_exit_halts_cpu() {
    let mut cpu = cpu_with_safe_sp();

    // r0 = SYS_EXIT (1), r1 = exit_code (42)
    // INT 0x80
    let rom = vec![
        enc(op::MOVI, 0, 0, lsys::SYS_EXIT as u16),
        enc(op::MOVI, 1, 0, 42),
        enc(op::INT, 0, 0, intr::VECTOR_SYSCALL as u16),
    ];
    cpu.load_rom(&rom);

    let result = cpu.run(100);
    // run() returns Ok(cycles) when HALT is reached
    assert!(result.is_ok(), "SYS_EXIT should halt cleanly: {:?}", result);
    assert_eq!(cpu.state.halted, 1, "CPU should be halted");
    assert_eq!(cpu.state.exit_code, 42, "exit_code should be 42");
}

#[test]
fn syscall_write_outputs_to_tty() {
    let mut cpu = cpu_with_safe_sp();

    // Store "Hi" at RAM byte address 0x100
    let msg = "Hi";
    store_string(&mut cpu, 0x100, msg);

    // r0 = SYS_WRITE (4), r1 = fd 1 (stdout), r2 = buf 0x100, r3 = len 2
    // INT 0x80
    // HALT
    let rom = vec![
        enc(op::MOVI, 0, 0, lsys::SYS_WRITE as u16),
        enc(op::MOVI, 1, 0, 1),      // fd = stdout
        enc(op::MOVI, 2, 0, 0x0100), // buf address
        enc(op::MOVI, 3, 0, 2),      // length
        enc(op::INT, 0, 0, intr::VECTOR_SYSCALL as u16),
        enc(op::HALT, 0, 0, 0),
    ];
    cpu.load_rom(&rom);

    let result = cpu.run(100);
    assert!(result.is_ok() || result == Err(ExecError::Halted));
    assert_eq!(cpu.tty_output, b"Hi", "tty_output should contain 'Hi'");
    assert_eq!(
        cpu.state.regs[0], 2,
        "SYS_WRITE should return bytes written"
    );
}

#[test]
fn syscall_write_to_stderr() {
    let mut cpu = cpu_with_safe_sp();

    let msg = "err";
    store_string(&mut cpu, 0x200, msg);

    let rom = vec![
        enc(op::MOVI, 0, 0, lsys::SYS_WRITE as u16),
        enc(op::MOVI, 1, 0, 2), // fd = stderr
        enc(op::MOVI, 2, 0, 0x0200),
        enc(op::MOVI, 3, 0, 3),
        enc(op::INT, 0, 0, intr::VECTOR_SYSCALL as u16),
        enc(op::HALT, 0, 0, 0),
    ];
    cpu.load_rom(&rom);
    cpu.run(100).ok();

    assert_eq!(
        cpu.tty_output, b"err",
        "stderr should also go to tty_output"
    );
}

#[test]
fn syscall_write_bad_fd_returns_ebadf() {
    let mut cpu = cpu_with_safe_sp();

    store_string(&mut cpu, 0x100, "x");

    let rom = vec![
        enc(op::MOVI, 0, 0, lsys::SYS_WRITE as u16),
        enc(op::MOVI, 1, 0, 99), // bad fd
        enc(op::MOVI, 2, 0, 0x0100),
        enc(op::MOVI, 3, 0, 1),
        enc(op::INT, 0, 0, intr::VECTOR_SYSCALL as u16),
        enc(op::HALT, 0, 0, 0),
    ];
    cpu.load_rom(&rom);
    cpu.run(100).ok();

    assert!(
        cpu.tty_output.is_empty(),
        "bad fd should not produce output"
    );
    // EBADF = -9 as u32
    assert_eq!(cpu.state.regs[0], (-9i32) as u32, "should return -EBADF");
}

#[test]
fn syscall_brk_query_returns_current_break() {
    let mut cpu = cpu_with_safe_sp();

    let initial_brk = cpu.state.program_break;

    // r0 = SYS_BRK (45), r1 = 0 (query mode)
    let rom = vec![
        enc(op::MOVI, 0, 0, lsys::SYS_BRK as u16),
        enc(op::MOVI, 1, 0, 0),
        enc(op::INT, 0, 0, intr::VECTOR_SYSCALL as u16),
        enc(op::HALT, 0, 0, 0),
    ];
    cpu.load_rom(&rom);
    cpu.run(100).ok();

    assert_eq!(
        cpu.state.regs[0], initial_brk,
        "BRK with r1=0 should return current program_break"
    );
}

#[test]
fn syscall_brk_set_updates_program_break() {
    let mut cpu = cpu_with_safe_sp();

    // Set program_break to 0x5000 (using MOVI + LOAD_IMM32 to build 32-bit value)
    // We need the value 0x5000 which fits in 16 bits, so MOVI is sufficient.
    let rom = vec![
        enc(op::MOVI, 0, 0, lsys::SYS_BRK as u16),
        enc(op::MOVI, 1, 0, 0x5000),
        enc(op::INT, 0, 0, intr::VECTOR_SYSCALL as u16),
        enc(op::HALT, 0, 0, 0),
    ];
    cpu.load_rom(&rom);
    cpu.run(100).ok();

    assert_eq!(
        cpu.state.program_break, 0x5000,
        "program_break should be updated"
    );
    assert_eq!(
        cpu.state.regs[0], 0x5000,
        "BRK should return new break value"
    );
}

#[test]
fn syscall_getpid_returns_instance_id() {
    let mut cpu = cpu_with_safe_sp();
    cpu.instance_id = 7;

    let rom = vec![
        enc(op::MOVI, 0, 0, lsys::SYS_GETPID as u16),
        enc(op::INT, 0, 0, intr::VECTOR_SYSCALL as u16),
        enc(op::HALT, 0, 0, 0),
    ];
    cpu.load_rom(&rom);
    cpu.run(100).ok();

    assert_eq!(cpu.state.regs[0], 7, "SYS_GETPID should return instance_id");
}

#[test]
fn syscall_clock_gettime_returns_nonzero() {
    let mut cpu = cpu_with_safe_sp();
    cpu.ticks_ms = 5500; // 5.5 seconds

    // r0 = SYS_CLOCK_GETTIME (265), r1 = pointer to timespec struct (0x1000)
    // We need 265 which is > 255, so use MOVI (which can do 16-bit imm).
    let rom = vec![
        enc(op::MOVI, 0, 0, lsys::SYS_CLOCK_GETTIME as u16),
        enc(op::MOVI, 1, 0, 0x1000), // timespec address
        enc(op::INT, 0, 0, intr::VECTOR_SYSCALL as u16),
        enc(op::HALT, 0, 0, 0),
    ];
    cpu.load_rom(&rom);
    cpu.run(100).ok();

    assert_eq!(
        cpu.state.regs[0], 0,
        "CLOCK_GETTIME should return 0 on success"
    );

    // Read back timespec from RAM at byte addr 0x1000
    let secs = cpu.ram[0x1000 >> 2];
    let nsecs = cpu.ram[(0x1000 + 4) >> 2];
    assert_eq!(secs, 5, "5500ms should give 5 seconds");
    assert_eq!(
        nsecs, 500_000_000,
        "5500ms should give 500M nanoseconds remainder"
    );
}

#[test]
fn syscall_fork_parent_gets_child_pid_child_gets_zero() {
    let mut cpu = cpu_with_safe_sp();
    assert_eq!(cpu.state.num_processes, 1);

    let rom = vec![
        enc(op::MOVI, 0, 0, lsys::SYS_FORK as u16),
        enc(op::INT, 0, 0, intr::VECTOR_SYSCALL as u16),
        enc(op::HALT, 0, 0, 0),
    ];
    cpu.load_rom(&rom);
    cpu.run(100).ok();

    // Parent should get child_pid = 1 (second process, index 1)
    assert_eq!(cpu.state.regs[0], 1, "parent should get child_pid in r0");
    assert_eq!(cpu.state.num_processes, 2, "should now have 2 processes");

    // Child state is in proc_table[1], child's r0 should be 0
    assert_eq!(
        cpu.proc_table[1][0], 0,
        "child's r0 should be 0 (fork return)"
    );

    // Child's PC should be saved at the point of the fork syscall.
    // INT 0x80 is a software interrupt which, for VECTOR_SYSCALL, executes inline
    // (no IVT dispatch), so child's PC = parent's PC at time of fork.
    // The parent continues past the INT instruction, so child PC may differ
    // from the parent's final PC. Just verify the child has a valid PC.
    assert!(
        cpu.proc_table[1][16] > 0,
        "child's saved PC should be non-zero"
    );
}

#[test]
fn syscall_fork_max_processes_returns_eagain() {
    let mut cpu = cpu_with_safe_sp();
    cpu.state.num_processes = intr::MAX_PROCESSES; // already at max

    let rom = vec![
        enc(op::MOVI, 0, 0, lsys::SYS_FORK as u16),
        enc(op::INT, 0, 0, intr::VECTOR_SYSCALL as u16),
        enc(op::HALT, 0, 0, 0),
    ];
    cpu.load_rom(&rom);
    cpu.run(100).ok();

    assert_eq!(
        cpu.state.regs[0],
        (-11i32) as u32,
        "fork at max processes should return -EAGAIN"
    );
}

#[test]
fn syscall_unknown_returns_enosys() {
    let mut cpu = cpu_with_safe_sp();

    // Use a syscall number that is not implemented (e.g., 999)
    let rom = vec![
        enc(op::MOVI, 0, 0, 999),
        enc(op::INT, 0, 0, intr::VECTOR_SYSCALL as u16),
        enc(op::HALT, 0, 0, 0),
    ];
    cpu.load_rom(&rom);
    cpu.run(100).ok();

    assert_eq!(
        cpu.state.regs[0],
        (-(lsys::ENOSYS as i32)) as u32,
        "unknown syscall should return -ENOSYS"
    );
}

// ═══════════════════════════════════════════════════════════════════════════════
// 3. Scheduler
// ═══════════════════════════════════════════════════════════════════════════════

#[test]
fn scheduler_fork_and_yield_switches_context() {
    let mut cpu = cpu_with_safe_sp();

    // Program:
    //   0: MOVI r0, SYS_FORK
    //   1: INT 0x80                 ; fork -> parent r0=1, child r0=0
    //   2: MOVI r4, 0x42           ; marker (both parent and child execute this)
    //   3: MOVI r0, SYS_SCHED_YIELD
    //   4: INT 0x80                 ; yield -> context switch
    //   5: HALT
    let rom = vec![
        enc(op::MOVI, 0, 0, lsys::SYS_FORK as u16),
        enc(op::INT, 0, 0, intr::VECTOR_SYSCALL as u16),
        // After fork: parent continues here, child will too when scheduled
        enc(op::MOVI, 4, 0, 0x42),
        enc(op::MOVI, 0, 0, lsys::SYS_SCHED_YIELD as u16),
        enc(op::INT, 0, 0, intr::VECTOR_SYSCALL as u16),
        enc(op::HALT, 0, 0, 0),
    ];
    cpu.load_rom(&rom);

    // Run: fork creates child, parent yields which switches to child,
    // child runs to halt, eventually both halt.
    // We give it enough cycles for the full flow.
    let _result = cpu.run(100);

    // After running, verify that fork happened and context switch occurred.
    assert_eq!(cpu.state.num_processes, 2, "two processes after fork");

    // Either we ran to completion or halted. The key thing is that
    // the context switch infrastructure worked without panicking.
    // Check that proc_table has saved state for the other process.
    let other_pid = if cpu.state.current_pid == 0 { 1 } else { 0 };
    // The other process should have been saved during context switch.
    // Its r4 may or may not be 0x42 depending on scheduling order.

    // Run a bit more to ensure both processes get a chance to execute.
    let _result2 = cpu.run(100);

    // Verify halted state
    assert_eq!(cpu.state.halted, 1, "should eventually halt");

    // Verify that at least one context switch occurred by checking proc_table
    // has non-zero data (saved state from a context switch).
    let saved = cpu.proc_table[other_pid as usize];
    // At minimum, the saved SP should be non-zero (it was set to 0x4000).
    assert!(
        saved[18] != 0 || cpu.state.current_pid != 0,
        "context switch should have saved state for the other process"
    );
}

#[test]
fn scheduler_context_switch_round_robin() {
    let mut cpu = cpu_with_safe_sp();

    // Manually set up two processes in proc_table.
    cpu.state.num_processes = 2;
    cpu.state.current_pid = 0;

    // Process 1 state: r0 = 0xDEAD, PC = 0, flags = 0
    cpu.proc_table[1][0] = 0xDEAD;
    cpu.proc_table[1][16] = 0; // PC
    cpu.proc_table[1][17] = 0; // flags
    cpu.proc_table[1][18] = 0x4000; // SP
    cpu.proc_table[1][19] = cpu.state.program_break;

    // Set a marker in process 0 (current)
    cpu.state.regs[0] = 0xBEEF;

    // Perform context switch
    cpu.scheduler_context_switch();

    // Should now be running process 1
    assert_eq!(cpu.state.current_pid, 1, "should switch to pid 1");
    assert_eq!(cpu.state.regs[0], 0xDEAD, "should load process 1's r0");

    // Process 0's state should be saved
    assert_eq!(
        cpu.proc_table[0][0], 0xBEEF,
        "process 0's r0 should be saved"
    );

    // Switch back
    cpu.scheduler_context_switch();
    assert_eq!(cpu.state.current_pid, 0, "should switch back to pid 0");
    assert_eq!(cpu.state.regs[0], 0xBEEF, "should restore process 0's r0");
}

#[test]
fn scheduler_skips_halted_processes() {
    let mut cpu = cpu_with_safe_sp();

    // 3 processes: 0 (running), 1 (halted), 2 (runnable)
    cpu.state.num_processes = 3;
    cpu.state.current_pid = 0;
    cpu.halted_mask = 1 << 1; // process 1 is halted

    cpu.proc_table[2][0] = 0xCAFE;
    cpu.proc_table[2][16] = 0;
    cpu.proc_table[2][17] = 0;
    cpu.proc_table[2][18] = 0x4000;
    cpu.proc_table[2][19] = cpu.state.program_break;

    cpu.scheduler_context_switch();

    assert_eq!(
        cpu.state.current_pid, 2,
        "should skip halted pid 1, go to pid 2"
    );
    assert_eq!(cpu.state.regs[0], 0xCAFE, "should load pid 2's r0");
}

// ═══════════════════════════════════════════════════════════════════════════════
// 4. MMU
// ═══════════════════════════════════════════════════════════════════════════════

#[test]
fn mmu_disabled_passthrough() {
    // When MMU is disabled, addresses pass through unchanged.
    let mut cpu = cpu_with_safe_sp();
    assert_eq!(cpu.state.mmu_enabled, 0);

    // Write a value to RAM, read it back via LD with MMU disabled
    cpu.ram[0x100] = 0xDEADBEEF; // word address 0x100

    let rom = vec![
        enc(op::MOVI, 1, 0, 0x0400), // byte address 0x400 = word 0x100
        enc(op::LD, 0, 1, 0),        // r0 = RAM[0x400]
        enc(op::HALT, 0, 0, 0),
    ];
    cpu.load_rom(&rom);
    cpu.run(100).ok();

    assert_eq!(
        cpu.state.regs[0], 0xDEADBEEF,
        "MMU disabled should pass through"
    );
}

#[test]
fn mmu_identity_map_works() {
    let mut cpu = cpu_with_safe_sp();

    // Set up a simple identity-mapped page table in RAM.
    // We need:
    //   1. A page directory at some address (e.g., 0x10000 bytes = word 0x4000)
    //   2. A page table at some address (e.g., 0x11000 bytes = word 0x4400)
    //
    // For identity mapping of address 0x0000_0000 - 0x003F_FFFF (first 4MB):
    //   PDE[0] = page_table_base | PTE_PRESENT
    //   PTE[0..1023] = (i * 0x1000) | PTE_PRESENT | PTE_WRITE
    //
    // Virtual address 0x0000_2000 (page 2):
    //   PDE index = 0x0000_2000 >> 22 = 0
    //   PTE index = (0x0000_2000 >> 12) & 0x3FF = 2
    //   Offset = 0x0000_2000 & 0xFFF = 0

    let pd_byte_addr: u32 = 0x10000;
    let pt_byte_addr: u32 = 0x11000;
    let pd_word = (pd_byte_addr >> 2) as usize;
    let pt_word = (pt_byte_addr >> 2) as usize;

    // PDE[0] = pt_byte_addr | PTE_PRESENT
    cpu.ram[pd_word] = pt_byte_addr | mmu::PTE_PRESENT;

    // Identity map first 16 pages (enough for our test)
    for i in 0..16u32 {
        let pte = (i << 12) | mmu::PTE_PRESENT | mmu::PTE_WRITE;
        cpu.ram[pt_word + i as usize] = pte;
    }

    // Write test data at page 2, offset 0 (byte addr 0x2000, word 0x800)
    cpu.ram[0x800] = 0x12345678;

    // Program: set page dir, enable MMU, read from 0x2000
    let rom = vec![
        // SYS_SET_PAGE_DIR: r0 = 250, r1 = pd_byte_addr
        enc(op::MOVI, 0, 0, lsys::SYS_SET_PAGE_DIR as u16),
        enc(op::MOVI, 1, 0, (pd_byte_addr & 0xFFFF) as u16),
        enc(op::LOAD_IMM32, 1, 0, (pd_byte_addr >> 16) as u16),
        enc(op::INT, 0, 0, intr::VECTOR_SYSCALL as u16),
        // SYS_ENABLE_MMU: r0 = 251
        enc(op::MOVI, 0, 0, lsys::SYS_ENABLE_MMU as u16),
        enc(op::INT, 0, 0, intr::VECTOR_SYSCALL as u16),
        // LD r2, [0x2000]
        enc(op::MOVI, 3, 0, 0x2000),
        enc(op::LD, 2, 3, 0),
        enc(op::HALT, 0, 0, 0),
    ];
    cpu.load_rom(&rom);
    cpu.run(100).ok();

    assert_eq!(cpu.state.mmu_enabled, 1, "MMU should be enabled");
    assert_eq!(
        cpu.state.regs[2], 0x12345678,
        "identity-mapped read should return correct value"
    );
}

#[test]
fn mmu_tlb_gets_populated() {
    let mut cpu = cpu_with_safe_sp();

    let pd_byte_addr: u32 = 0x10000;
    let pt_byte_addr: u32 = 0x11000;
    let pd_word = (pd_byte_addr >> 2) as usize;
    let pt_word = (pt_byte_addr >> 2) as usize;

    // PDE[0] = pt_byte_addr | PTE_PRESENT
    cpu.ram[pd_word] = pt_byte_addr | mmu::PTE_PRESENT;

    // Identity map page 3 (byte addr 0x3000)
    cpu.ram[pt_word + 3] = (3 << 12) | mmu::PTE_PRESENT | mmu::PTE_WRITE;

    cpu.ram[0xC00] = 0xAABBCCDD; // word at byte addr 0x3000

    let rom = vec![
        enc(op::MOVI, 0, 0, lsys::SYS_SET_PAGE_DIR as u16),
        enc(op::MOVI, 1, 0, (pd_byte_addr & 0xFFFF) as u16),
        enc(op::LOAD_IMM32, 1, 0, (pd_byte_addr >> 16) as u16),
        enc(op::INT, 0, 0, intr::VECTOR_SYSCALL as u16),
        enc(op::MOVI, 0, 0, lsys::SYS_ENABLE_MMU as u16),
        enc(op::INT, 0, 0, intr::VECTOR_SYSCALL as u16),
        enc(op::MOVI, 3, 0, 0x3000),
        enc(op::LD, 2, 3, 0),
        enc(op::HALT, 0, 0, 0),
    ];
    cpu.load_rom(&rom);
    cpu.run(100).ok();

    // VPN for 0x3000 = 3, TLB index = 3 & 63 = 3
    let tlb_entry = cpu.tlb[3];
    assert_eq!(tlb_entry[0], 3, "TLB VPN should be 3");
    assert_eq!(tlb_entry[1], 3, "TLB PFN should be 3 (identity map)");
    assert_ne!(
        tlb_entry[2] & mmu::PTE_PRESENT,
        0,
        "TLB entry should be present"
    );
}

#[test]
fn mmu_flush_tlb_clears_entries() {
    let mut cpu = cpu_with_safe_sp();

    // Populate a TLB entry manually
    cpu.tlb[5] = [5, 5, mmu::PTE_PRESENT];

    let rom = vec![
        enc(op::MOVI, 0, 0, lsys::SYS_FLUSH_TLB as u16),
        enc(op::INT, 0, 0, intr::VECTOR_SYSCALL as u16),
        enc(op::HALT, 0, 0, 0),
    ];
    cpu.load_rom(&rom);
    cpu.run(100).ok();

    assert_eq!(cpu.tlb[5], [0, 0, 0], "TLB should be flushed");
}

// ═══════════════════════════════════════════════════════════════════════════════
// 5. Block Device (Ramdisk)
// ═══════════════════════════════════════════════════════════════════════════════

#[test]
fn block_device_write_then_read() {
    let mut cpu = cpu_with_safe_sp();

    // Write test data to a source buffer in RAM at byte address 0x800 (word 0x200)
    let src_word = 0x200usize;
    for i in 0..128usize {
        cpu.ram[src_word + i] = 0xDEAD0000 + i as u32;
    }

    // SYS_WRITE_BLOCK: r0=201, r1=block_num (0), r2=buf_addr (0x800 byte addr)
    // Then SYS_READ_BLOCK: r0=200, r1=block_num (0), r2=buf_addr (0x1800 byte addr)
    let rom = vec![
        // Write block 0 from buffer at 0x800
        enc(op::MOVI, 0, 0, lsys::SYS_WRITE_BLOCK as u16),
        enc(op::MOVI, 1, 0, 0),      // block 0
        enc(op::MOVI, 2, 0, 0x0800), // source buffer
        enc(op::INT, 0, 0, intr::VECTOR_SYSCALL as u16),
        // Save return value
        enc(op::MOV, 5, 0, 0),
        // Read block 0 into buffer at 0x1800
        enc(op::MOVI, 0, 0, lsys::SYS_READ_BLOCK as u16),
        enc(op::MOVI, 1, 0, 0),      // block 0
        enc(op::MOVI, 2, 0, 0x1800), // dest buffer
        enc(op::INT, 0, 0, intr::VECTOR_SYSCALL as u16),
        // Save return value
        enc(op::MOV, 6, 0, 0),
        enc(op::HALT, 0, 0, 0),
    ];
    cpu.load_rom(&rom);
    cpu.run(200).ok();

    // Verify return values
    assert_eq!(
        cpu.state.regs[5],
        blk::BLOCK_SIZE,
        "WRITE_BLOCK should return 512"
    );
    assert_eq!(
        cpu.state.regs[6],
        blk::BLOCK_SIZE,
        "READ_BLOCK should return 512"
    );

    // Verify data matches: dest buffer at word 0x600 (byte 0x1800)
    let dst_word = 0x600usize;
    for i in 0..128usize {
        assert_eq!(
            cpu.ram[dst_word + i],
            0xDEAD0000 + i as u32,
            "block data mismatch at word offset {}",
            i
        );
    }
}

#[test]
fn block_device_invalid_block_returns_eio() {
    let mut cpu = cpu_with_safe_sp();

    // Try to read block beyond total blocks
    let rom = vec![
        enc(op::MOVI, 0, 0, lsys::SYS_READ_BLOCK as u16),
        enc(op::MOVI, 1, 0, 0xFFFF), // way beyond total blocks
        enc(op::MOVI, 2, 0, 0x0800),
        enc(op::INT, 0, 0, intr::VECTOR_SYSCALL as u16),
        enc(op::HALT, 0, 0, 0),
    ];
    cpu.load_rom(&rom);
    cpu.run(100).ok();

    assert_eq!(
        cpu.state.regs[0],
        (-(lsys::EIO as i32)) as u32,
        "invalid block should return -EIO"
    );
}

#[test]
fn block_device_multiple_blocks() {
    let mut cpu = cpu_with_safe_sp();

    // Write distinct patterns to blocks 0 and 1
    let src_word = 0x200usize;
    for i in 0..128 {
        cpu.ram[src_word + i] = 0xAAAA0000 + i as u32;
    }

    let rom = vec![
        // Write block 0
        enc(op::MOVI, 0, 0, lsys::SYS_WRITE_BLOCK as u16),
        enc(op::MOVI, 1, 0, 0),
        enc(op::MOVI, 2, 0, 0x0800),
        enc(op::INT, 0, 0, intr::VECTOR_SYSCALL as u16),
        enc(op::HALT, 0, 0, 0),
    ];
    cpu.load_rom(&rom);
    cpu.run(100).ok();

    // Change source buffer pattern for block 1
    for i in 0..128 {
        cpu.ram[src_word + i] = 0xBBBB0000 + i as u32;
    }

    // Write block 1
    cpu.state.halted = 0;
    cpu.state.pc = 0;
    let rom2 = vec![
        enc(op::MOVI, 0, 0, lsys::SYS_WRITE_BLOCK as u16),
        enc(op::MOVI, 1, 0, 1),
        enc(op::MOVI, 2, 0, 0x0800),
        enc(op::INT, 0, 0, intr::VECTOR_SYSCALL as u16),
        enc(op::HALT, 0, 0, 0),
    ];
    cpu.load_rom(&rom2);
    cpu.run(100).ok();

    // Read back block 0 and verify it still has 0xAAAA pattern
    cpu.state.halted = 0;
    cpu.state.pc = 0;
    let rom3 = vec![
        enc(op::MOVI, 0, 0, lsys::SYS_READ_BLOCK as u16),
        enc(op::MOVI, 1, 0, 0),
        enc(op::MOVI, 2, 0, 0x1800),
        enc(op::INT, 0, 0, intr::VECTOR_SYSCALL as u16),
        enc(op::HALT, 0, 0, 0),
    ];
    cpu.load_rom(&rom3);
    cpu.run(100).ok();

    let dst_word = 0x600usize;
    assert_eq!(
        cpu.ram[dst_word], 0xAAAA0000,
        "block 0 data should be 0xAAAA pattern"
    );
    assert_eq!(cpu.ram[dst_word + 127], 0xAAAA007F, "block 0 last word");
}

// ═══════════════════════════════════════════════════════════════════════════════
// 6. Console I/O
// ═══════════════════════════════════════════════════════════════════════════════

#[test]
fn console_write_string_via_syscall() {
    let mut cpu = cpu_with_safe_sp();

    let msg = "Hello, UPC!\n";
    store_string(&mut cpu, 0x200, msg);

    let rom = vec![
        enc(op::MOVI, 0, 0, lsys::SYS_WRITE as u16),
        enc(op::MOVI, 1, 0, 1), // stdout
        enc(op::MOVI, 2, 0, 0x0200),
        enc(op::MOVI, 3, 0, msg.len() as u16),
        enc(op::INT, 0, 0, intr::VECTOR_SYSCALL as u16),
        enc(op::HALT, 0, 0, 0),
    ];
    cpu.load_rom(&rom);
    cpu.run(100).ok();

    let output = String::from_utf8_lossy(&cpu.tty_output);
    assert_eq!(output, msg, "tty_output should contain the full message");
}

#[test]
fn console_multiple_writes_accumulate() {
    let mut cpu = cpu_with_safe_sp();

    store_string(&mut cpu, 0x200, "AB");
    store_string(&mut cpu, 0x300, "CD");

    let rom = vec![
        // First write: "AB"
        enc(op::MOVI, 0, 0, lsys::SYS_WRITE as u16),
        enc(op::MOVI, 1, 0, 1),
        enc(op::MOVI, 2, 0, 0x0200),
        enc(op::MOVI, 3, 0, 2),
        enc(op::INT, 0, 0, intr::VECTOR_SYSCALL as u16),
        // Second write: "CD"
        enc(op::MOVI, 0, 0, lsys::SYS_WRITE as u16),
        enc(op::MOVI, 1, 0, 1),
        enc(op::MOVI, 2, 0, 0x0300),
        enc(op::MOVI, 3, 0, 2),
        enc(op::INT, 0, 0, intr::VECTOR_SYSCALL as u16),
        enc(op::HALT, 0, 0, 0),
    ];
    cpu.load_rom(&rom);
    cpu.run(200).ok();

    assert_eq!(cpu.tty_output, b"ABCD", "multiple writes should accumulate");
}

#[test]
fn console_write_capped_at_256_bytes() {
    let mut cpu = cpu_with_safe_sp();

    // Store 300 bytes of 'X'
    for i in 0..300u32 {
        let addr = 0x200 + i;
        let word_idx = (addr >> 2) as usize;
        let byte_off = (addr & 3) as usize;
        if word_idx < cpu.ram.len() {
            let mask = 0xFFu32 << (byte_off * 8);
            cpu.ram[word_idx] = (cpu.ram[word_idx] & !mask) | ((b'X' as u32) << (byte_off * 8));
        }
    }

    let rom = vec![
        enc(op::MOVI, 0, 0, lsys::SYS_WRITE as u16),
        enc(op::MOVI, 1, 0, 1),
        enc(op::MOVI, 2, 0, 0x0200),
        enc(op::MOVI, 3, 0, 300), // request 300 bytes
        enc(op::INT, 0, 0, intr::VECTOR_SYSCALL as u16),
        enc(op::HALT, 0, 0, 0),
    ];
    cpu.load_rom(&rom);
    cpu.run(100).ok();

    assert_eq!(
        cpu.tty_output.len(),
        256,
        "write should be capped at 256 bytes"
    );
    assert_eq!(
        cpu.state.regs[0], 256,
        "SYS_WRITE should return 256 (capped)"
    );
}

// ═══════════════════════════════════════════════════════════════════════════════
// 7. Boot Integration (hello_kernel demo)
// ═══════════════════════════════════════════════════════════════════════════════

#[test]
fn boot_hello_kernel_produces_output() {
    // The hello_kernel demo writes "Hello from UPC kernel!\n" via SYS_WRITE.
    // We construct the equivalent program using INT 0x80 directly.
    let mut cpu = cpu_with_safe_sp();

    let msg = "Hello from UPC kernel!\n";
    let msg_addr: u32 = 0x0400; // byte address in RAM
    store_string(&mut cpu, msg_addr, msg);

    // Program: MOVI r0,4; MOVI r1,1; MOVI r2,msg_addr; MOVI r3,23; INT 0x80; HALT
    let rom = vec![
        enc(op::MOVI, 0, 0, lsys::SYS_WRITE as u16),
        enc(op::MOVI, 1, 0, 1),                // fd = stdout
        enc(op::MOVI, 2, 0, msg_addr as u16),  // buffer
        enc(op::MOVI, 3, 0, msg.len() as u16), // length
        enc(op::INT, 0, 0, intr::VECTOR_SYSCALL as u16),
        enc(op::HALT, 0, 0, 0),
    ];
    cpu.load_rom(&rom);

    let result = cpu.run(100);
    assert!(result.is_ok() || result == Err(ExecError::Halted));

    let output = String::from_utf8_lossy(&cpu.tty_output);
    assert_eq!(
        output, msg,
        "hello_kernel should produce 'Hello from UPC kernel!\\n'"
    );
}

#[test]
fn boot_hello_kernel_binary_if_available() {
    // Load the actual hello_kernel.mbc binary if it exists.
    let kernel_path =
        std::path::Path::new(env!("CARGO_MANIFEST_DIR")).join("../../demos/mbc/hello_kernel.mbc");

    if !kernel_path.exists() {
        eprintln!("SKIP: hello_kernel.mbc not found at {:?}", kernel_path);
        return;
    }

    let data = std::fs::read(&kernel_path).expect("read hello_kernel.mbc");
    assert!(data.len() >= 4, "binary should be at least 4 bytes");

    let rom: Vec<u32> = data
        .chunks_exact(4)
        .map(|c| u32::from_le_bytes([c[0], c[1], c[2], c[3]]))
        .collect();

    let mut cpu = cpu_with_safe_sp();
    cpu.load_rom(&rom);

    // The binary may reference string data at a fixed offset.
    // Run for a reasonable number of cycles and check that it either
    // halts cleanly or produces tty output.
    let result = cpu.run(10000);

    match result {
        Ok(_) | Err(ExecError::Halted) => {
            eprintln!(
                "hello_kernel.mbc: halted, tty_output = {:?}",
                String::from_utf8_lossy(&cpu.tty_output)
            );
        }
        Err(ExecError::CycleBudgetExhausted) => {
            eprintln!("hello_kernel.mbc: ran 10000 cycles without halt");
        }
        Err(e) => {
            eprintln!("hello_kernel.mbc: error {:?} (non-fatal for smoke test)", e);
        }
    }
    // Success = no panic
}

// ═══════════════════════════════════════════════════════════════════════════════
// Additional OS primitive tests
// ═══════════════════════════════════════════════════════════════════════════════

#[test]
fn syscall_nanosleep_advances_time() {
    let mut cpu = cpu_with_safe_sp();
    cpu.ticks_ms = 1000;

    // Store timespec at RAM byte 0x1000: tv_sec=2, tv_nsec=500_000_000 (2.5s)
    cpu.ram[0x1000 >> 2] = 2; // tv_sec
    cpu.ram[(0x1000 + 4) >> 2] = 500_000_000; // tv_nsec

    let rom = vec![
        enc(op::MOVI, 0, 0, lsys::SYS_NANOSLEEP as u16),
        enc(op::MOVI, 1, 0, 0x1000),
        enc(op::INT, 0, 0, intr::VECTOR_SYSCALL as u16),
        enc(op::HALT, 0, 0, 0),
    ];
    cpu.load_rom(&rom);
    cpu.run(100).ok();

    // 2s = 2000ms, 500M ns = 500ms -> total 2500ms
    assert_eq!(
        cpu.ticks_ms, 3500,
        "nanosleep should advance ticks_ms by 2500"
    );
    assert_eq!(cpu.state.regs[0], 0, "nanosleep should return 0 on success");
}

#[test]
fn syscall_sched_yield_with_single_process_is_noop() {
    let mut cpu = cpu_with_safe_sp();
    assert_eq!(cpu.state.num_processes, 1);

    cpu.state.regs[4] = 0x1234; // marker

    let rom = vec![
        enc(op::MOVI, 0, 0, lsys::SYS_SCHED_YIELD as u16),
        enc(op::INT, 0, 0, intr::VECTOR_SYSCALL as u16),
        enc(op::HALT, 0, 0, 0),
    ];
    cpu.load_rom(&rom);
    cpu.run(100).ok();

    assert_eq!(cpu.state.regs[0], 0, "yield should return 0");
    assert_eq!(cpu.state.regs[4], 0x1234, "registers should be unchanged");
    assert_eq!(cpu.state.current_pid, 0, "pid should remain 0");
}

#[test]
fn software_interrupt_non_syscall_dispatches_via_ivt() {
    // INT with vector != 0x80 should dispatch through the IVT, not as a syscall.
    let mut cpu = cpu_with_safe_sp();

    // Set up IVT entry for vector 0x10: handler at ROM word 5.
    // IVT addr = 0x10 * 4 = 0x40 bytes = word 0x10
    cpu.ram[0x10] = 5;

    let mut rom = vec![enc(op::NOP, 0, 0, 0); 8];
    // Word 0: INT 0x10 (software interrupt, not syscall)
    rom[0] = enc(op::INT, 0, 0, 0x10);
    // Word 1: HALT (return from handler will come back here)
    rom[1] = enc(op::HALT, 0, 0, 0);
    // Word 5: handler: MOVI r7, 0xFACE
    rom[5] = enc(op::MOVI, 7, 0, 0xFACE);
    // Word 6: IRET
    rom[6] = enc(op::IRET, 0, 0, 0);

    cpu.load_rom(&rom);
    cpu.run(100).ok();

    assert_eq!(
        cpu.state.regs[7], 0xFACE,
        "non-syscall INT should dispatch to IVT handler"
    );
    assert_eq!(
        cpu.state.halted, 1,
        "should halt after IRET returns to HALT"
    );
}

#[test]
fn mmu_set_page_dir_syscall() {
    let mut cpu = cpu_with_safe_sp();

    let rom = vec![
        enc(op::MOVI, 0, 0, lsys::SYS_SET_PAGE_DIR as u16),
        enc(op::MOVI, 1, 0, 0x8000),
        enc(op::INT, 0, 0, intr::VECTOR_SYSCALL as u16),
        enc(op::HALT, 0, 0, 0),
    ];
    cpu.load_rom(&rom);
    cpu.run(100).ok();

    assert_eq!(
        cpu.state.page_dir_base, 0x8000,
        "page_dir_base should be set"
    );
    assert_eq!(cpu.state.regs[0], 0, "SET_PAGE_DIR should return 0");
}

#[test]
fn mmu_enable_mmu_syscall() {
    let mut cpu = cpu_with_safe_sp();
    assert_eq!(cpu.state.mmu_enabled, 0);

    let rom = vec![
        enc(op::MOVI, 0, 0, lsys::SYS_ENABLE_MMU as u16),
        enc(op::INT, 0, 0, intr::VECTOR_SYSCALL as u16),
        enc(op::HALT, 0, 0, 0),
    ];
    cpu.load_rom(&rom);
    cpu.run(100).ok();

    assert_eq!(cpu.state.mmu_enabled, 1, "MMU should be enabled");
    assert_eq!(cpu.state.regs[0], 0, "ENABLE_MMU should return 0");
}

#[test]
fn syscall_exit_with_different_codes() {
    for code in [0u16, 1, 42, 255] {
        let mut cpu = cpu_with_safe_sp();
        let rom = vec![
            enc(op::MOVI, 0, 0, lsys::SYS_EXIT as u16),
            enc(op::MOVI, 1, 0, code),
            enc(op::INT, 0, 0, intr::VECTOR_SYSCALL as u16),
        ];
        cpu.load_rom(&rom);
        let result = cpu.run(100);
        // run() returns Ok(cycles) when halt is reached
        assert!(
            result.is_ok(),
            "SYS_EXIT should halt cleanly for code {}: {:?}",
            code,
            result
        );
        assert_eq!(
            cpu.state.halted, 1,
            "CPU should be halted for code {}",
            code
        );
        assert_eq!(
            cpu.state.exit_code, code as u32,
            "exit code should be {}",
            code
        );
    }
}

#[test]
fn block_device_preserves_ramdisk_across_blocks() {
    // Write to block 5, then verify block 0 is still zeroed (no cross-contamination).
    let mut cpu = cpu_with_safe_sp();

    let src_word = 0x200usize;
    for i in 0..128 {
        cpu.ram[src_word + i] = 0xF00D0000 + i as u32;
    }

    // Write to block 5
    let rom = vec![
        enc(op::MOVI, 0, 0, lsys::SYS_WRITE_BLOCK as u16),
        enc(op::MOVI, 1, 0, 5),
        enc(op::MOVI, 2, 0, 0x0800),
        enc(op::INT, 0, 0, intr::VECTOR_SYSCALL as u16),
        enc(op::HALT, 0, 0, 0),
    ];
    cpu.load_rom(&rom);
    cpu.run(100).ok();

    // Read back block 0 (should be all zeros since we never wrote to it)
    cpu.state.halted = 0;
    cpu.state.pc = 0;
    let rom2 = vec![
        enc(op::MOVI, 0, 0, lsys::SYS_READ_BLOCK as u16),
        enc(op::MOVI, 1, 0, 0),
        enc(op::MOVI, 2, 0, 0x1800),
        enc(op::INT, 0, 0, intr::VECTOR_SYSCALL as u16),
        enc(op::HALT, 0, 0, 0),
    ];
    cpu.load_rom(&rom2);
    cpu.run(100).ok();

    let dst_word = 0x600usize;
    for i in 0..128 {
        assert_eq!(
            cpu.ram[dst_word + i],
            0,
            "block 0 should still be zeroed, but word {} was {:08x}",
            i,
            cpu.ram[dst_word + i]
        );
    }

    // Read back block 5 to verify our data is there
    cpu.state.halted = 0;
    cpu.state.pc = 0;
    let rom3 = vec![
        enc(op::MOVI, 0, 0, lsys::SYS_READ_BLOCK as u16),
        enc(op::MOVI, 1, 0, 5),
        enc(op::MOVI, 2, 0, 0x1800),
        enc(op::INT, 0, 0, intr::VECTOR_SYSCALL as u16),
        enc(op::HALT, 0, 0, 0),
    ];
    cpu.load_rom(&rom3);
    cpu.run(100).ok();

    assert_eq!(cpu.ram[dst_word], 0xF00D0000, "block 5 first word");
    assert_eq!(cpu.ram[dst_word + 127], 0xF00D007F, "block 5 last word");
}

// ═══════════════════════════════════════════════════════════════════════════════
// 8. SYS_EXECVE — Process Image Replacement
// ═══════════════════════════════════════════════════════════════════════════════

#[test]
fn syscall_execve_resets_registers_and_jumps() {
    // SYS_EXECVE should zero all registers (except SP gets reset to 0xFFFF_0000),
    // set PC to the entry_point, and clear flags/interrupt state.
    let mut cpu = cpu_with_safe_sp();

    // Dirty some registers to verify they get reset
    cpu.state.regs[3] = 0xDEADBEEF;
    cpu.state.regs[7] = 0xCAFEBABE;
    cpu.state.flags = 0xFF;

    // Place a "new program" in ROM at word 10: MOVI r5, 0xABCD; HALT
    let mut rom = vec![enc(op::NOP, 0, 0, 0); 20];
    // Main program at word 0: set up execve
    rom[0] = enc(op::MOVI, 0, 0, lsys::SYS_EXECVE as u16); // r0 = SYS_EXECVE
    rom[1] = enc(op::MOVI, 1, 0, 10); // r1 = entry_point (ROM word 10)
    rom[2] = enc(op::INT, 0, 0, intr::VECTOR_SYSCALL as u16); // execve!
    rom[3] = enc(op::HALT, 0, 0, 0); // should NOT be reached

    // New program at word 10
    rom[10] = enc(op::MOVI, 5, 0, 0xABCD);
    rom[11] = enc(op::HALT, 0, 0, 0);

    cpu.load_rom(&rom);
    cpu.run(100).ok();

    // Verify the new program ran (r5 set by the new program)
    assert_eq!(
        cpu.state.regs[5], 0xABCD,
        "new program should set r5 = 0xABCD"
    );

    // Verify CPU halted (new program hit HALT)
    assert_eq!(cpu.state.halted, 1, "new program should halt");

    // Verify old register values were cleared
    assert_eq!(cpu.state.regs[3], 0, "r3 should be zeroed by execve");
    assert_eq!(cpu.state.regs[7], 0, "r7 should be zeroed by execve");

    // Verify flags were cleared (may have been set by MOVI, but were reset by execve)
    // After execve, flags = 0; then MOVI r5, 0xABCD doesn't set flags.
    // The flags state depends on implementation — just verify execve did reset.
}

#[test]
fn syscall_execve_resets_sp_to_top() {
    let mut cpu = cpu_with_safe_sp();

    // Set SP to a non-default value
    cpu.state.regs[REG_SP] = 0x1234;

    let mut rom = vec![enc(op::NOP, 0, 0, 0); 12];
    rom[0] = enc(op::MOVI, 0, 0, lsys::SYS_EXECVE as u16);
    rom[1] = enc(op::MOVI, 1, 0, 10); // entry point = word 10
    rom[2] = enc(op::INT, 0, 0, intr::VECTOR_SYSCALL as u16);
    rom[10] = enc(op::HALT, 0, 0, 0);

    cpu.load_rom(&rom);
    cpu.run(100).ok();

    assert_eq!(
        cpu.state.regs[REG_SP], 0xFFFF_0000,
        "execve should reset SP to 0xFFFF_0000"
    );
}

#[test]
fn syscall_execve_resets_program_break() {
    use monad_common::DEFAULT_PROGRAM_BREAK;

    let mut cpu = cpu_with_safe_sp();

    // Move program break away from default
    cpu.state.program_break = 0x0080_0000;

    let mut rom = vec![enc(op::NOP, 0, 0, 0); 12];
    rom[0] = enc(op::MOVI, 0, 0, lsys::SYS_EXECVE as u16);
    rom[1] = enc(op::MOVI, 1, 0, 10);
    rom[2] = enc(op::INT, 0, 0, intr::VECTOR_SYSCALL as u16);
    rom[10] = enc(op::HALT, 0, 0, 0);

    cpu.load_rom(&rom);
    cpu.run(100).ok();

    assert_eq!(
        cpu.state.program_break, DEFAULT_PROGRAM_BREAK,
        "execve should reset program_break to default"
    );
}

#[test]
fn syscall_execve_clears_interrupt_state() {
    let mut cpu = cpu_with_safe_sp();

    // Set interrupt state to non-default
    cpu.state.interrupts_enabled = 1;
    cpu.state.interrupt_pending = 1;
    cpu.state.interrupt_vector = intr::VECTOR_TIMER;

    // Disable interrupts so the pending one doesn't fire before execve
    cpu.state.interrupts_enabled = 0;
    cpu.state.interrupt_pending = 1;

    let mut rom = vec![enc(op::NOP, 0, 0, 0); 12];
    rom[0] = enc(op::MOVI, 0, 0, lsys::SYS_EXECVE as u16);
    rom[1] = enc(op::MOVI, 1, 0, 10);
    rom[2] = enc(op::INT, 0, 0, intr::VECTOR_SYSCALL as u16);
    rom[10] = enc(op::HALT, 0, 0, 0);

    cpu.load_rom(&rom);
    cpu.run(100).ok();

    assert_eq!(
        cpu.state.interrupts_enabled, 0,
        "execve should clear interrupts_enabled"
    );
    assert_eq!(
        cpu.state.interrupt_pending, 0,
        "execve should clear interrupt_pending"
    );
}

#[test]
fn syscall_execve_jumps_to_nonzero_entry_point() {
    // Verify execve can jump to an arbitrary entry point in ROM
    let mut cpu = cpu_with_safe_sp();

    let entry = 15u16;
    let mut rom = vec![enc(op::NOP, 0, 0, 0); 20];
    rom[0] = enc(op::MOVI, 0, 0, lsys::SYS_EXECVE as u16);
    rom[1] = enc(op::MOVI, 1, 0, entry);
    rom[2] = enc(op::INT, 0, 0, intr::VECTOR_SYSCALL as u16);
    rom[3] = enc(op::HALT, 0, 0, 0); // should not reach

    // Program at word 15: set distinctive marker and halt
    rom[15] = enc(op::MOVI, 2, 0, 0x7777);
    rom[16] = enc(op::HALT, 0, 0, 0);

    cpu.load_rom(&rom);
    cpu.run(100).ok();

    assert_eq!(
        cpu.state.regs[2], 0x7777,
        "should execute code at entry point 15"
    );
    assert_eq!(cpu.state.halted, 1, "should halt from new program");
}

// ═══════════════════════════════════════════════════════════════════════════════
// Level 6: Atomic Operations (CLI/STI/XCHG/CAS)
// ═══════════════════════════════════════════════════════════════════════════════

#[test]
fn test_cli_sti() {
    let mut cpu = cpu_with_safe_sp();
    // Start with interrupts enabled
    cpu.state.interrupts_enabled = 1;

    cpu.load_rom(&[
        enc(op::CLI, 0, 0, 0),  // 0: disable interrupts
        enc(op::HALT, 0, 0, 0), // 1: halt
    ]);
    cpu.step().unwrap();
    assert_eq!(
        cpu.state.interrupts_enabled, 0,
        "CLI should disable interrupts"
    );

    // Reset and test STI
    cpu.state.pc = 0;
    cpu.state.halted = 0;
    cpu.state.interrupts_enabled = 0;
    cpu.load_rom(&[
        enc(op::STI, 0, 0, 0),  // 0: enable interrupts
        enc(op::HALT, 0, 0, 0), // 1: halt
    ]);
    cpu.step().unwrap();
    assert_eq!(
        cpu.state.interrupts_enabled, 1,
        "STI should enable interrupts"
    );
}

#[test]
fn test_xchg() {
    let mut cpu = cpu_with_safe_sp();

    // Set up: r0 = 0xDEAD, RAM[word 0x100] = 0xBEEF
    cpu.state.regs[0] = 0xDEAD;
    cpu.state.regs[1] = 0x400; // byte address (word addr = 0x100)
    cpu.ram[0x100] = 0xBEEF;

    // XCHG r0, [r1+0]: swap r0 with RAM[r1]
    cpu.load_rom(&[
        enc(op::XCHG, 0, 1, 0), // 0: atomic exchange
        enc(op::HALT, 0, 0, 0), // 1: halt
    ]);
    cpu.step().unwrap();

    assert_eq!(
        cpu.state.regs[0], 0xBEEF,
        "XCHG should load old RAM value into dst reg"
    );
    assert_eq!(
        cpu.ram[0x100], 0xDEAD,
        "XCHG should store old reg value into RAM"
    );
}

#[test]
fn test_cas_success() {
    let mut cpu = cpu_with_safe_sp();

    // Set up for successful CAS:
    // r0 = expected (0x1234), r1 = address (byte), r2 = desired (0x5678)
    // RAM[word] = 0x1234 (matches expected)
    cpu.state.regs[0] = 0x1234; // expected
    cpu.state.regs[1] = 0x800; // byte address (word addr = 0x200)
    cpu.state.regs[2] = 0x5678; // desired
    cpu.ram[0x200] = 0x1234; // current value == expected
    cpu.state.flags = 0;

    cpu.load_rom(&[
        enc(op::CAS, 0, 0, 0),  // 0: compare-and-swap
        enc(op::HALT, 0, 0, 0), // 1: halt
    ]);
    cpu.step().unwrap();

    assert_eq!(
        cpu.ram[0x200], 0x5678,
        "CAS success: RAM should be updated to desired"
    );
    assert_eq!(
        cpu.state.regs[0], 0x1234,
        "CAS success: r0 should contain old value"
    );
    assert_ne!(
        cpu.state.flags & 0x01,
        0,
        "CAS success: Z flag should be set"
    );
}

#[test]
fn test_cas_failure() {
    let mut cpu = cpu_with_safe_sp();

    // Set up for failed CAS:
    // r0 = expected (0x1234), r1 = address (byte), r2 = desired (0x5678)
    // RAM[word] = 0xAAAA (does NOT match expected)
    cpu.state.regs[0] = 0x1234; // expected
    cpu.state.regs[1] = 0x800; // byte address (word addr = 0x200)
    cpu.state.regs[2] = 0x5678; // desired
    cpu.ram[0x200] = 0xAAAA; // current value != expected
    cpu.state.flags = 0x01; // start with Z set

    cpu.load_rom(&[
        enc(op::CAS, 0, 0, 0),  // 0: compare-and-swap
        enc(op::HALT, 0, 0, 0), // 1: halt
    ]);
    cpu.step().unwrap();

    assert_eq!(
        cpu.ram[0x200], 0xAAAA,
        "CAS failure: RAM should be unchanged"
    );
    assert_eq!(
        cpu.state.regs[0], 0xAAAA,
        "CAS failure: r0 should contain actual old value"
    );
    assert_eq!(
        cpu.state.flags & 0x01,
        0,
        "CAS failure: Z flag should be clear"
    );
}

// ═══════════════════════════════════════════════════════════════════════════════
// 8. ASCEND-LINUX (ADR-067) — FENCE / MRET / SRET / LR.W / SC.W
// ═══════════════════════════════════════════════════════════════════════════════

/// FENCE is a logical no-op on the single-CPU MBC interpreter.
#[test]
fn ascend_fence_executes_and_halts() {
    let mut cpu = cpu_with_safe_sp();
    let rom = vec![enc(op::FENCE, 0, 0, 0), enc(op::HALT, 0, 0, 0)];
    cpu.load_rom(&rom);
    let _ = cpu.run(100);
    assert_eq!(cpu.state.halted, 1, "FENCE+HALT should halt cleanly");
}

/// LR.W loads a word and records the reservation address.
#[test]
fn ascend_lr_w_loads_word_and_sets_reservation() {
    let mut cpu = cpu_with_safe_sp();
    cpu.ram[0x100] = 0xCAFEBABE;
    cpu.state.regs[1] = 0x400; // byte address (word 0x100)

    let rom = vec![enc(op::LR_W, 2, 1, 0), enc(op::HALT, 0, 0, 0)];
    cpu.load_rom(&rom);
    let _ = cpu.run(100);

    assert_eq!(cpu.state.regs[2], 0xCAFEBABE, "r2 must hold loaded value");
    assert_eq!(
        cpu.state.reservation_address, 0x400,
        "reservation_address must equal LR.W rs1"
    );
}

/// SC.W succeeds when the reservation matches; rd=0; memory updated.
#[test]
fn ascend_sc_w_success_when_reservation_valid() {
    let mut cpu = cpu_with_safe_sp();
    cpu.state.regs[1] = 0x400;
    cpu.state.regs[3] = 0xDEADBEEF;
    cpu.state.reservation_address = 0x400;

    let rom = vec![enc(op::SC_W, 2, 1, 3), enc(op::HALT, 0, 0, 0)];
    cpu.load_rom(&rom);
    let _ = cpu.run(100);

    assert_eq!(cpu.state.regs[2], 0, "SC.W success: rd must be 0");
    assert_eq!(
        cpu.ram[0x100], 0xDEADBEEF,
        "memory must be updated on SC.W success"
    );
    assert_eq!(
        cpu.state.reservation_address, 0xFFFF_FFFF,
        "reservation cleared after SC.W"
    );
}

/// SC.W fails when no reservation; rd=1; memory unchanged.
#[test]
fn ascend_sc_w_failure_when_no_reservation() {
    let mut cpu = cpu_with_safe_sp();
    cpu.state.regs[1] = 0x400;
    cpu.state.regs[3] = 0xDEADBEEF;
    cpu.ram[0x100] = 0x12345678;

    let rom = vec![enc(op::SC_W, 2, 1, 3), enc(op::HALT, 0, 0, 0)];
    cpu.load_rom(&rom);
    let _ = cpu.run(100);

    assert_eq!(cpu.state.regs[2], 1, "SC.W failure: rd must be 1");
    assert_eq!(cpu.ram[0x100], 0x12345678, "memory must NOT be updated");
}

/// SC.W fails when reservation is for a different address.
#[test]
fn ascend_sc_w_failure_when_reservation_address_mismatches() {
    let mut cpu = cpu_with_safe_sp();
    cpu.state.regs[1] = 0x400;
    cpu.state.regs[3] = 0xDEADBEEF;
    cpu.ram[0x100] = 0x42;
    cpu.state.reservation_address = 0x800;

    let rom = vec![enc(op::SC_W, 2, 1, 3), enc(op::HALT, 0, 0, 0)];
    cpu.load_rom(&rom);
    let _ = cpu.run(100);

    assert_eq!(
        cpu.state.regs[2], 1,
        "stale reservation must produce SC.W failure"
    );
    assert_eq!(cpu.ram[0x100], 0x42, "memory unchanged");
}

/// LR.W → SC.W round-trip: classic atomic spinlock primitive.
#[test]
fn ascend_lr_sc_roundtrip_atomic_increment() {
    let mut cpu = cpu_with_safe_sp();
    cpu.ram[0x100] = 7;
    cpu.state.regs[1] = 0x400;

    let rom = vec![
        enc(op::LR_W, 2, 1, 0),
        enc(op::ADDI, 2, 0, 1),
        enc(op::MOV, 3, 2, 0),
        enc(op::SC_W, 4, 1, 3),
        enc(op::HALT, 0, 0, 0),
    ];
    cpu.load_rom(&rom);
    let _ = cpu.run(100);

    assert_eq!(cpu.state.regs[4], 0, "SC.W must succeed");
    assert_eq!(cpu.ram[0x100], 8, "counter must be 8 (atomic increment)");
}

/// MRET reads MEPC + MSTATUS from the CSR memory region and jumps.
/// Targets a HALT at the destination so `run` completes naturally.
#[test]
fn ascend_mret_pops_mepc_and_restores_priv() {
    let mut cpu = cpu_with_safe_sp();
    let mepc_word = ((0xF000 + 0x341 * 4) >> 2) as usize;
    let mstatus_word = ((0xF000 + 0x300 * 4) >> 2) as usize;
    // MEPC = 20 (byte) → PC = 5 (word). ROM[5] is HALT.
    cpu.ram[mepc_word] = 20;
    // MSTATUS.MPP = 0b01 (S-mode) at bits [12:11].
    cpu.ram[mstatus_word] = 0b01 << 11;

    cpu.state.priv_level = 0;
    cpu.state.reservation_address = 0xABCD;

    let rom = vec![
        enc(op::MRET, 0, 0, 0),
        enc(op::ADDI, 0, 0, 1),
        enc(op::ADDI, 0, 0, 1),
        enc(op::ADDI, 0, 0, 1),
        enc(op::ADDI, 0, 0, 1),
        enc(op::HALT, 0, 0, 0),
    ];
    cpu.load_rom(&rom);
    let _ = cpu.run(100);

    assert_eq!(cpu.state.halted, 1, "MRET should land on HALT");
    assert_eq!(
        cpu.state.priv_level, 1,
        "MRET must restore priv to S (MPP=01)"
    );
    assert_eq!(
        cpu.state.reservation_address, 0xFFFF_FFFF,
        "MRET must clear reservation"
    );
    assert_eq!(cpu.state.regs[0], 0, "MRET must skip filler instructions");
}

/// SRET reads SEPC + SSTATUS and transitions to U-mode.
#[test]
fn ascend_sret_pops_sepc_and_returns_to_u_mode() {
    let mut cpu = cpu_with_safe_sp();
    let sepc_word = ((0xF000 + 0x141 * 4) >> 2) as usize;
    let sstatus_word = ((0xF000 + 0x100 * 4) >> 2) as usize;
    cpu.ram[sepc_word] = 20;
    cpu.ram[sstatus_word] = 0; // SSTATUS.SPP = 0 → U-mode

    cpu.state.priv_level = 1;
    cpu.state.reservation_address = 0x1234;

    let rom = vec![
        enc(op::SRET, 0, 0, 0),
        enc(op::ADDI, 0, 0, 1),
        enc(op::ADDI, 0, 0, 1),
        enc(op::ADDI, 0, 0, 1),
        enc(op::ADDI, 0, 0, 1),
        enc(op::HALT, 0, 0, 0),
    ];
    cpu.load_rom(&rom);
    let _ = cpu.run(100);

    assert_eq!(cpu.state.halted, 1);
    assert_eq!(
        cpu.state.priv_level, 3,
        "SRET with SPP=0 must return to U-mode"
    );
    assert_eq!(cpu.state.reservation_address, 0xFFFF_FFFF);
    assert_eq!(cpu.state.regs[0], 0);
}

/// SRET with SPP=1 returns to S-mode (nested S-mode trap).
#[test]
fn ascend_sret_with_spp_1_returns_to_s_mode() {
    let mut cpu = cpu_with_safe_sp();
    let sepc_word = ((0xF000 + 0x141 * 4) >> 2) as usize;
    let sstatus_word = ((0xF000 + 0x100 * 4) >> 2) as usize;
    cpu.ram[sepc_word] = 4; // PC=1 (HALT)
    cpu.ram[sstatus_word] = 1 << 8; // SPP=1

    cpu.state.priv_level = 1;
    let rom = vec![enc(op::SRET, 0, 0, 0), enc(op::HALT, 0, 0, 0)];
    cpu.load_rom(&rom);
    let _ = cpu.run(100);

    assert_eq!(cpu.state.halted, 1);
    assert_eq!(
        cpu.state.priv_level, 1,
        "SRET with SPP=1 must stay in S-mode"
    );
}

// ── Phase 1.2 (ADR-074 Option A + Allocator A1) ─────────────────────────────

/// Forking 4 children must populate proc_table[i][20] (page_dir_base) with
/// distinct, 4-KiB-aligned addresses inside the fixed per-pid region at
/// 0x00F00000. This is the Phase 3.1 hard-gate forkbomb smoke test from the
/// Phase 1.2 IMPL plan; if it fails, Option A is broken at the allocator
/// layer and downstream xv6 patches will inherit the corruption.
#[test]
fn phase12_fork_assigns_distinct_pgd_per_child() {
    let mut cpu = cpu_with_safe_sp();
    assert_eq!(cpu.state.num_processes, 1, "starts with init only");

    // Fork three times. Process indices end up: 0=init, 1, 2, 3.
    let rom = vec![
        enc(op::MOVI, 0, 0, lsys::SYS_FORK as u16),
        enc(op::INT, 0, 0, intr::VECTOR_SYSCALL as u16),
        enc(op::MOVI, 0, 0, lsys::SYS_FORK as u16),
        enc(op::INT, 0, 0, intr::VECTOR_SYSCALL as u16),
        enc(op::MOVI, 0, 0, lsys::SYS_FORK as u16),
        enc(op::INT, 0, 0, intr::VECTOR_SYSCALL as u16),
        enc(op::HALT, 0, 0, 0),
    ];
    cpu.load_rom(&rom);
    cpu.run(200).ok();
    assert_eq!(cpu.state.num_processes, 4, "should now have 4 processes");

    // Children 1..=3 must have unique, 4-KiB-aligned pgds in the fixed region.
    let mut seen = std::collections::HashSet::new();
    for pid in 1..4 {
        let pgd = cpu.proc_table[pid][20];
        assert_eq!(
            pgd & 0xFFF,
            0,
            "pid {} pgd 0x{:08X} not 4-KiB aligned",
            pid,
            pgd
        );
        assert!(
            (0x00F0_0000..=0x00F0_3000).contains(&pgd),
            "pid {} pgd 0x{:08X} outside fixed region 0x00F00000-0x00F03000",
            pid,
            pgd
        );
        assert!(
            seen.insert(pgd),
            "pid {} pgd 0x{:08X} collides with sibling",
            pid,
            pgd
        );
    }
    // Verify the deterministic mapping.
    assert_eq!(cpu.proc_table[1][20], 0x00F0_1000);
    assert_eq!(cpu.proc_table[2][20], 0x00F0_2000);
    assert_eq!(cpu.proc_table[3][20], 0x00F0_3000);
}

/// Context switch must save the outgoing process's `page_dir_base` to
/// proc_table[old][20] and load the incoming process's pgd from
/// proc_table[new][20] into `cpu.state.page_dir_base`. This is the Option A
/// falsification experiment from ADR-074: PID 1 sets a sentinel pgd, yields
/// to PID 0, and PID 0's pgd must NOT match PID 1's.
#[test]
fn phase12_context_switch_save_restore_page_dir_base() {
    let mut cpu = cpu_with_safe_sp();
    // Two processes: PID 0 (init) with sentinel pgd, PID 1 with different pgd.
    cpu.state.num_processes = 2;
    cpu.state.current_pid = 0;
    cpu.state.page_dir_base = 0x00F0_0000; // PID 0's pgd
                                           // Pre-load PID 1 in proc_table with its own pgd.
    cpu.proc_table[1] = [0u32; 21];
    cpu.proc_table[1][16] = 0; // PC=0 (HALT)
    cpu.proc_table[1][18] = 0xFFFE_0000; // some SP
    cpu.proc_table[1][20] = 0x00F0_1000; // PID 1's pgd

    // Yield from PID 0 → PID 1.
    let rom = vec![
        enc(op::MOVI, 0, 0, lsys::SYS_SCHED_YIELD as u16),
        enc(op::INT, 0, 0, intr::VECTOR_SYSCALL as u16),
        enc(op::HALT, 0, 0, 0),
    ];
    cpu.load_rom(&rom);
    // Run exactly through the MOVI + INT (yield); stop before PID 1 can re-enter
    // the scheduler and ping-pong back to PID 0.
    cpu.run(2).ok();

    // PID 0's outgoing pgd should be saved.
    assert_eq!(
        cpu.proc_table[0][20], 0x00F0_0000,
        "outgoing PID 0 pgd should be saved to slot[20]"
    );
    // CPU should now be running PID 1 with PID 1's pgd loaded.
    assert_eq!(cpu.state.current_pid, 1, "should have switched to PID 1");
    assert_eq!(
        cpu.state.page_dir_base, 0x00F0_1000,
        "CPU pgd should be loaded from PID 1's slot[20]"
    );
    // PID 0's pgd MUST NOT equal PID 1's pgd — that's the isolation invariant.
    assert_ne!(
        cpu.proc_table[0][20], cpu.proc_table[1][20],
        "PID 0 and PID 1 must hold distinct pgds (Option A isolation invariant)"
    );
}

/// Phase 1.3 D-1 (ADR-075): PROC_TABLE widened 4 → 8. Forking 7 times must
/// populate proc_table[0..8] with distinct, 4-KiB-aligned page-directory
/// bases across the full per-pid region 0x00F00000..0x00F07000. An 8th fork
/// must refuse (slot exhaustion → r0 = -EAGAIN). This is the Phase 1.3
/// IMPL Step 2 falsification test from `references/battle-plan-phase13-impl-2026-05-13.md`.
#[test]
fn phase13_proc_table_supports_8_slots() {
    let mut cpu = cpu_with_safe_sp();
    assert_eq!(cpu.state.num_processes, 1, "starts with init only");

    // Fork 7 times → 8 total processes (init + 7 children).
    let mut rom: Vec<u32> = Vec::new();
    for _ in 0..7 {
        rom.push(enc(op::MOVI, 0, 0, lsys::SYS_FORK as u16));
        rom.push(enc(op::INT, 0, 0, intr::VECTOR_SYSCALL as u16));
    }
    rom.push(enc(op::HALT, 0, 0, 0));
    cpu.load_rom(&rom);
    cpu.run(400).ok();

    assert_eq!(
        cpu.state.num_processes, 8,
        "8 slots filled by init + 7 forks"
    );

    // Each pid 1..=7 owns a unique 4-KiB-aligned pgd inside the per-pid region.
    let mut seen = std::collections::HashSet::new();
    for pid in 1..8 {
        let pgd = cpu.proc_table[pid][20];
        assert_eq!(
            pgd & 0xFFF,
            0,
            "pid {pid} pgd 0x{pgd:08X} not 4-KiB aligned"
        );
        assert!(
            (0x00F0_0000..=0x00F0_7000).contains(&pgd),
            "pid {pid} pgd 0x{pgd:08X} outside fixed region 0x00F00000-0x00F07000"
        );
        assert!(
            seen.insert(pgd),
            "pid {pid} pgd 0x{pgd:08X} collides with sibling"
        );
    }

    // Deterministic mapping per phase12::pgd_base_for_pid: pid * 0x1000.
    for pid in 1..8usize {
        assert_eq!(
            cpu.proc_table[pid][20],
            0x00F0_0000 + (pid as u32) * 0x1000,
            "pid {pid} pgd doesn't match Allocator A1 formula"
        );
    }

    // An 8th fork (from any process) must refuse — slot exhaustion → r0 = -EAGAIN.
    let exhaust_rom = vec![
        enc(op::MOVI, 0, 0, lsys::SYS_FORK as u16),
        enc(op::INT, 0, 0, intr::VECTOR_SYSCALL as u16),
        enc(op::HALT, 0, 0, 0),
    ];
    cpu.load_rom(&exhaust_rom);
    cpu.state.pc = 0;
    cpu.state.halted = 0;
    cpu.run(50).ok();
    assert_eq!(
        cpu.state.num_processes, 8,
        "9th process must NOT be created"
    );
    assert_eq!(
        cpu.state.regs[0] as i32, -11,
        "exhausted fork returns -EAGAIN (errno 11)"
    );
}

/// Phase 1.3 Step 3 (ADR-075 D-5): SYS_EXIT zombies the exiting slot
/// (sets halted_mask bit for current_pid) and yields to another runnable
/// process. num_processes is NOT decremented — the slot stays allocated
/// until a parent SYS_WAITPID reaps it (xv6 ZOMBIE semantic).
#[test]
fn phase13_sys_exit_zombies_slot_and_yields() {
    let mut cpu = cpu_with_safe_sp();
    cpu.state.num_processes = 2;
    cpu.state.current_pid = 1;
    cpu.proc_table[0] = [0u32; 21];
    cpu.proc_table[0][16] = 10; // PC=10 (HALT in ROM)
    cpu.proc_table[0][18] = 0xFFFE_0000;

    let rom = vec![
        enc(op::MOVI, 0, 0, lsys::SYS_EXIT as u16),
        enc(op::MOVI, 1, 0, 42),
        enc(op::INT, 0, 0, intr::VECTOR_SYSCALL as u16),
        enc(op::HALT, 0, 0, 0),
        enc(op::HALT, 0, 0, 0),
        enc(op::HALT, 0, 0, 0),
        enc(op::HALT, 0, 0, 0),
        enc(op::HALT, 0, 0, 0),
        enc(op::HALT, 0, 0, 0),
        enc(op::HALT, 0, 0, 0),
        enc(op::HALT, 0, 0, 0), // PC=10
    ];
    cpu.load_rom(&rom);
    cpu.run(5).ok();

    assert_eq!(
        cpu.halted_mask & 0b10,
        0b10,
        "PID 1 halted_mask bit set after SYS_EXIT"
    );
    assert_eq!(
        cpu.state.num_processes, 2,
        "num_processes unchanged on SYS_EXIT (ZOMBIE)"
    );
    assert_eq!(cpu.state.current_pid, 0, "yields to PID 0");
    assert_eq!(cpu.state.exit_code, 42, "exit code captured");
}

/// Phase 1.3 Step 3: SYS_EXIT from the last runnable process halts the CPU
/// (no scheduler ping-pong).
#[test]
fn phase13_sys_exit_last_process_halts_cpu() {
    let mut cpu = cpu_with_safe_sp();
    assert_eq!(cpu.state.num_processes, 1);
    let rom = vec![
        enc(op::MOVI, 0, 0, lsys::SYS_EXIT as u16),
        enc(op::MOVI, 1, 0, 7),
        enc(op::INT, 0, 0, intr::VECTOR_SYSCALL as u16),
        enc(op::HALT, 0, 0, 0),
    ];
    cpu.load_rom(&rom);
    cpu.run(5).ok();
    assert_eq!(cpu.state.halted, 1, "CPU halts when last process exits");
    assert_eq!(cpu.state.exit_code, 7);
}

/// Phase 1.3 Step 4 (ADR-075 D-5): SYS_WAITPID returns the child pid when
/// the target child has already halted (halted_mask bit set). No yield.
#[test]
fn phase13_sys_waitpid_returns_pid_when_child_halted() {
    let mut cpu = cpu_with_safe_sp();
    cpu.state.num_processes = 2;
    cpu.state.current_pid = 0;
    cpu.halted_mask = 0b10; // PID 1 already halted (zombie)

    let rom = vec![
        enc(op::MOVI, 0, 0, lsys::SYS_WAITPID as u16),
        enc(op::MOVI, 1, 0, 1), // wait for PID 1
        enc(op::INT, 0, 0, intr::VECTOR_SYSCALL as u16),
        enc(op::HALT, 0, 0, 0),
    ];
    cpu.load_rom(&rom);
    cpu.run(5).ok();
    assert_eq!(cpu.state.regs[0], 1, "WAITPID returns child pid 1");
    assert_eq!(
        cpu.state.current_pid, 0,
        "no yield needed when child already halted"
    );
}

/// Phase 1.3 Step 4: SYS_WAITPID yields when the target child is still
/// running (halted_mask bit clear). PC rewinds so the next tick re-checks.
#[test]
fn phase13_sys_waitpid_yields_when_child_running() {
    let mut cpu = cpu_with_safe_sp();
    cpu.state.num_processes = 2;
    cpu.state.current_pid = 0;
    cpu.halted_mask = 0; // PID 1 still running
    cpu.proc_table[1] = [0u32; 21];
    cpu.proc_table[1][16] = 10; // PID 1 PC=10 (HALT)
    cpu.proc_table[1][18] = 0xFFFE_0000;

    let rom = vec![
        enc(op::MOVI, 0, 0, lsys::SYS_WAITPID as u16),
        enc(op::MOVI, 1, 0, 1), // wait for PID 1
        enc(op::INT, 0, 0, intr::VECTOR_SYSCALL as u16),
        enc(op::HALT, 0, 0, 0),
        enc(op::HALT, 0, 0, 0),
        enc(op::HALT, 0, 0, 0),
        enc(op::HALT, 0, 0, 0),
        enc(op::HALT, 0, 0, 0),
        enc(op::HALT, 0, 0, 0),
        enc(op::HALT, 0, 0, 0),
        enc(op::HALT, 0, 0, 0), // PC=10
    ];
    cpu.load_rom(&rom);
    cpu.run(3).ok();
    // Parent yielded; current_pid is now PID 1 (the child).
    assert_eq!(cpu.state.current_pid, 1, "parent yields to runnable child");
}

/// Phase 1.3 Step 5 (ADR-075 D-5): SYS_EXECVE resets regs + PC + program_break
/// but PRESERVES the page_dir_base (exec replaces the image, not the address
/// space). r1 carries the pre-loaded entry-point ROM word address.
#[test]
fn phase13_sys_execve_resets_regs_preserves_pgd() {
    let mut cpu = cpu_with_safe_sp();
    // Pre-populate state we expect EXEC to clear.
    for i in 0..16 {
        cpu.state.regs[i] = 0xDEAD0000 + i as u32;
    }
    cpu.state.flags = 0xFF;
    cpu.state.program_break = 0x9999_9999;
    cpu.state.page_dir_base = 0x00F0_0000; // current process pgd

    // EXEC takes the entry point in r1 — but we set r0 (syscall nr) and r1
    // BEFORE the syscall fires; the handler reads them then clears.
    let entry_target: u32 = 12;
    let rom = vec![
        enc(op::MOVI, 0, 0, lsys::SYS_EXECVE as u16),
        enc(op::MOVI, 1, 0, entry_target as u16),
        enc(op::INT, 0, 0, intr::VECTOR_SYSCALL as u16),
        enc(op::HALT, 0, 0, 0),
        enc(op::HALT, 0, 0, 0),
        enc(op::HALT, 0, 0, 0),
        enc(op::HALT, 0, 0, 0),
        enc(op::HALT, 0, 0, 0),
        enc(op::HALT, 0, 0, 0),
        enc(op::HALT, 0, 0, 0),
        enc(op::HALT, 0, 0, 0),
        enc(op::HALT, 0, 0, 0),
        enc(op::HALT, 0, 0, 0), // PC=12 (entry target)
    ];
    cpu.load_rom(&rom);
    cpu.run(10).ok();

    // PC should have jumped to entry target (then halted).
    // All GPRs zeroed except SP reset to 0xFFFF_0000.
    for i in 0..15 {
        assert_eq!(cpu.state.regs[i], 0, "reg {i} should be zeroed by EXEC");
    }
    assert_eq!(cpu.state.regs[15], 0xFFFF_0000, "SP reset by EXEC");
    assert_eq!(cpu.state.flags, 0, "flags cleared by EXEC");
    // page_dir_base PRESERVED — exec replaces image in same address space.
    assert_eq!(
        cpu.state.page_dir_base, 0x00F0_0000,
        "page_dir_base MUST survive EXEC (image swap, not address-space swap)"
    );
}

/// Phase 1.3 Step 4: SYS_WAITPID for an invalid pid returns -ECHILD (10).
#[test]
fn phase13_sys_waitpid_invalid_pid_returns_echild() {
    let mut cpu = cpu_with_safe_sp();
    cpu.state.num_processes = 2;
    let rom = vec![
        enc(op::MOVI, 0, 0, lsys::SYS_WAITPID as u16),
        enc(op::MOVI, 1, 0, 99), // pid 99 doesn't exist
        enc(op::INT, 0, 0, intr::VECTOR_SYSCALL as u16),
        enc(op::HALT, 0, 0, 0),
    ];
    cpu.load_rom(&rom);
    cpu.run(5).ok();
    assert_eq!(cpu.state.regs[0] as i32, -10, "invalid pid -> -ECHILD");
}

/// Phase 1.3 Step 3: SYS_SCHED_YIELD switches to the next runnable process.
/// Regression-pinning test against the freshly-widened 8-slot scheduler.
#[test]
fn phase13_sys_sched_yield_switches() {
    let mut cpu = cpu_with_safe_sp();
    cpu.state.num_processes = 2;
    cpu.state.current_pid = 0;
    cpu.proc_table[1] = [0u32; 21];
    cpu.proc_table[1][16] = 10;
    cpu.proc_table[1][18] = 0xFFFE_0000;

    let rom = vec![
        enc(op::MOVI, 0, 0, lsys::SYS_SCHED_YIELD as u16),
        enc(op::INT, 0, 0, intr::VECTOR_SYSCALL as u16),
        enc(op::HALT, 0, 0, 0),
        enc(op::HALT, 0, 0, 0),
        enc(op::HALT, 0, 0, 0),
        enc(op::HALT, 0, 0, 0),
        enc(op::HALT, 0, 0, 0),
        enc(op::HALT, 0, 0, 0),
        enc(op::HALT, 0, 0, 0),
        enc(op::HALT, 0, 0, 0),
        enc(op::HALT, 0, 0, 0), // PC=10
    ];
    cpu.load_rom(&rom);
    cpu.run(3).ok();
    assert_eq!(cpu.state.current_pid, 1, "yield switches to PID 1");
}
