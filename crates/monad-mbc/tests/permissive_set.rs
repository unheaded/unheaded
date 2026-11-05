// SPDX-License-Identifier: GPL-3.0-or-later
//! DP-503: the 23 fabricated FUZIX calls live behind `fuzix-permissive`.
//!
//! `is_permissive` names the set for both interpreters. With the feature on
//! (the default, for Doom and the FUZIX demos) each one keeps its stub answer;
//! with it off, each one answers `-ENOSYS` instead of inventing a result.
//! Calls that do real work are never in the set.

use monad_common::{
    mbc_interrupts as intr, mbc_linux_syscalls as lsys, mbc_opcodes as op, MbcInsn, REG_SP,
};
use monad_mbc::ExecCpu;

/// Encode a standard instruction.
fn enc(opcode: u8, dst: u8, src: u8, imm: u16) -> u32 {
    MbcInsn::encode(opcode, dst, src, imm).0
}

/// Create a CPU with SP set to a safe RAM address (byte 0x4000 = word 0x1000).
fn cpu_with_safe_sp() -> ExecCpu {
    let mut cpu = ExecCpu::new();
    cpu.state.regs[REG_SP] = 0x4000; // byte address
    cpu
}

/// Run `INT 0x80` with `nr` in r0 and return the CPU after HALT.
fn syscall(nr: u32) -> ExecCpu {
    let mut cpu = cpu_with_safe_sp();
    let rom = vec![
        enc(op::MOVI, 0, 0, nr as u16),
        enc(op::INT, 0, 0, intr::VECTOR_SYSCALL as u16),
        enc(op::HALT, 0, 0, 0),
    ];
    cpu.load_rom(&rom);
    cpu.run(100).ok();
    cpu
}

/// The set, from both interpreter chains.
const PERMISSIVE: [u32; 23] = [
    lsys::SYS_OPEN,
    lsys::SYS_CLOSE,
    lsys::SYS_GETUID,
    lsys::SYS_GETGID,
    lsys::SYS_GETEUID,
    lsys::SYS_GETEGID,
    lsys::SYS_SETUID,
    lsys::SYS_SETGID,
    lsys::SYS_DUP,
    lsys::SYS_DUP2,
    lsys::SYS_KILL,
    lsys::SYS_STAT,
    lsys::SYS_FSTAT,
    lsys::SYS_LSEEK,
    lsys::SYS_GETPPID,
    lsys::SYS_UMASK,
    lsys::SYS_SYNC,
    lsys::SYS_TIMES,
    lsys::SYS_IOCTL,
    lsys::SYS_PIPE,
    lsys::SYS_FCNTL,
    lsys::SYS_ACCESS,
    lsys::SYS_CHDIR,
];

#[test]
fn is_permissive_holds_for_exactly_the_set() {
    for nr in 0..=1024u32 {
        assert_eq!(
            lsys::is_permissive(nr),
            PERMISSIVE.contains(&nr),
            "is_permissive({nr}) disagrees with the DP-503 set"
        );
    }
}

#[test]
fn real_calls_are_never_permissive() {
    for nr in [
        lsys::SYS_EXIT,
        lsys::SYS_READ,
        lsys::SYS_WRITE,
        lsys::SYS_BRK,
        lsys::SYS_GETPID,
        lsys::SYS_FORK,
        lsys::SYS_SIGNAL,
        lsys::SYS_TIME,
        lsys::SYS_READ_BLOCK,
        lsys::SYS_WRITE_BLOCK,
    ] {
        assert!(!lsys::is_permissive(nr), "{nr} does real work");
    }
}

#[cfg(feature = "fuzix-permissive")]
#[test]
fn permissive_build_keeps_the_stub_answer() {
    assert_eq!(syscall(lsys::SYS_OPEN).state.regs[0], 3);
}

#[cfg(not(feature = "fuzix-permissive"))]
#[test]
fn honest_build_answers_enosys() {
    assert_eq!(
        syscall(lsys::SYS_OPEN).state.regs[0],
        (-38i32) as u32,
        "SYS_OPEN must not fabricate fd 3"
    );
    for nr in PERMISSIVE {
        assert_eq!(
            syscall(nr).state.regs[0],
            (-38i32) as u32,
            "syscall {nr} must answer -ENOSYS"
        );
    }
}

#[cfg(not(feature = "fuzix-permissive"))]
#[test]
fn honest_build_leaves_real_calls_working() {
    let cpu = syscall(lsys::SYS_EXIT);
    assert_eq!(cpu.state.halted, 1, "SYS_EXIT still halts");
    assert_eq!(
        syscall(lsys::SYS_TIME).state.regs[0],
        0,
        "SYS_TIME still answers from ticks_ms"
    );
}
