# Road to Linux — UPC OS Primitive Design

## Table of Contents
1. The Ladder
2. Interrupt Emulation
3. MMU / Paging Emulation
4. Syscall Interface
5. Block Device Emulation
6. Console / TTY
7. Boot Protocol
8. xv6 Porting Notes
9. FUZIX Porting Notes
10. Linux (nommu) Porting Notes
11. Linux (full) — The Dream

---

## 1. The Ladder

Each level requires ALL previous levels working and tested.

```
Level 2: Doom runs                      ← PREREQUISITE FOR EVERYTHING
Level 3: Doom runs smooth (35fps+)      ← CURRENT TARGET
         ↓
Level 4: OS Primitives
  4a: Timer interrupts (BPF timer → INT)
  4b: Syscall dispatch (INT n → handler table)
  4c: Basic scheduler (round-robin, 2 processes)
  4d: MMU emulation (page tables in Wotan extended memory)
  4e: Block device (Wotan extended memory as ramdisk)
  4f: Console I/O (Busboy → tty)
         ↓
Level 5: Minimal Kernel
  5a: xv6-riscv adapted to MBC ISA
  5b: Boot → init → shell → simple commands
  5c: Fork/exec/wait working
  5d: File system (in-memory, then ramdisk)
         ↓
Level 6: Linux
  6a: uClinux (nommu) — simplest Linux variant
  6b: Full Linux with MMU emulation
  6c: Userspace: busybox shell, basic utils
  6d: Networking stack (inception: IP inside Monad)
```

---

## 2. Interrupt Emulation

### Design

Real CPUs have hardware interrupt lines. UPC uses BPF timer callbacks.

```rust
// Timer interrupt: BPF timer fires every N microseconds
// Triggers INT 0x20 (timer) in MBC execution

fn bpf_timer_callback(map: *mut c_void, key: *const c_void, value: *mut c_void) -> i32 {
    // Save current MBC state
    save_context();

    // Set interrupt flag
    flags.interrupt_pending = true;
    flags.interrupt_vector = 0x20; // Timer

    // Execution loop checks interrupt_pending after each instruction
    // If set: push PC + flags, jump to IVT[vector]
    0 // success
}

// Interrupt Vector Table at Wotan address 0x0000-0x00FF
// Each entry: 4 bytes (handler address)
// Vector 0x20 (timer) at offset 0x80
// Vector 0x21 (keyboard) at offset 0x84
// Vector 0x80 (syscall) at offset 0x200

fn handle_interrupt(vector: u8) {
    // Push flags
    push_stack(flags_to_u32());
    // Push return address
    push_stack(pc);
    // Disable further interrupts
    flags.interrupts_enabled = false;
    // Jump to handler
    let ivt_addr = (vector as u32) * 4;
    let mut handler_buf = [0u8; 4];
    bpf_wotan_read(flow_label, ivt_addr, handler_buf.as_mut_ptr(), 4);
    pc = u32::from_le_bytes(handler_buf);
}

// IRET instruction (opcode 0x18):
fn handle_iret() {
    pc = pop_stack();
    let saved_flags = pop_stack();
    u32_to_flags(saved_flags);
    flags.interrupts_enabled = true;
}
```

### Interrupt Sources

| Vector | Source | Trigger |
|--------|--------|---------|
| 0x00-0x1F | Exceptions (divide by zero, invalid opcode, etc.) | Synchronous |
| 0x20 | Timer | BPF timer callback (~100Hz for scheduler) |
| 0x21 | Keyboard/Input | Busboy `compute.input.{label}` message arrival |
| 0x22 | Disk complete | Wotan extended memory DMA complete |
| 0x80 | Syscall | `INT 0x80` instruction from userspace |

---

## 3. MMU / Paging Emulation

### Why This Is Hard

Real MMU = hardware translation on every memory access. UPC must do this in software inside BPF.
Every LOAD/STORE becomes: virtual address → page table walk → physical address → actual access.
Overhead: ~3-5 additional Wotan reads per memory access for page table walk.

### Design: Simple Two-Level Page Table

```
Virtual address (32-bit):
  [31:22] = Page Directory Index (10 bits, 1024 entries)
  [21:12] = Page Table Index    (10 bits, 1024 entries)
  [11:0]  = Page Offset         (12 bits, 4KB pages)

Page Directory: 1024 × 4 bytes = 4KB (in Wotan extended memory)
Page Table:     1024 × 4 bytes = 4KB (in Wotan extended memory)

Page Table Entry:
  [31:12] = Physical page frame number
  [11]    = Present
  [10]    = Read/Write
  [9]     = User/Supervisor
  [8]     = Accessed
  [7]     = Dirty
  [6:0]   = Reserved
```

### TLB Cache

Software TLB in BPF per-cpu array map to avoid page table walk on every access:

```rust
// TLB: 64 entries, direct-mapped by virtual page number
const TLB_SIZE: u32 = 64;

struct TLBEntry {
    virtual_page: u32,   // virtual page number (addr >> 12)
    physical_page: u32,  // physical page frame
    flags: u32,          // permissions
    valid: bool,
}

// In LOAD/STORE handler:
fn translate_address(vaddr: u32) -> Result<u32, PageFault> {
    let vpn = vaddr >> 12;
    let offset = vaddr & 0xFFF;
    let tlb_idx = vpn & (TLB_SIZE - 1);

    // Check TLB first (one BPF map lookup)
    let entry = bpf_map_lookup_elem(&TLB, &tlb_idx);
    if let Some(e) = entry {
        if e.valid && e.virtual_page == vpn {
            // TLB hit — ~100-200ns
            return Ok((e.physical_page << 12) | offset);
        }
    }

    // TLB miss — walk page tables (~400-600ns, 2 Wotan reads)
    let pde_addr = page_directory_base + ((vpn >> 10) * 4);
    let pde = wotan_read_u32(pde_addr)?;
    if (pde & PRESENT) == 0 { return Err(PageFault::NotPresent); }

    let pte_addr = (pde & 0xFFFFF000) + ((vpn & 0x3FF) * 4);
    let pte = wotan_read_u32(pte_addr)?;
    if (pte & PRESENT) == 0 { return Err(PageFault::NotPresent); }

    let phys_page = pte >> 12;

    // Update TLB
    let new_entry = TLBEntry { virtual_page: vpn, physical_page: phys_page, flags: pte, valid: true };
    bpf_map_update_elem(&TLB, &tlb_idx, &new_entry, BPF_ANY);

    Ok((phys_page << 12) | offset)
}
```

### Performance Impact

With 64-entry TLB:
- Hot path (TLB hit): +100-200ns per memory access
- Cold path (TLB miss): +400-600ns per memory access
- For Doom: TLB hit rate ~95%+ (small working set, spatial locality)
- For Linux: TLB hit rate ~80-90% (larger working set, more random access)

**This is the single biggest performance tax for running Linux.**
Optimize TLB size aggressively. 256 entries if BPF map allows.

---

## 4. Syscall Interface

```rust
// INT 0x80 handler
fn handle_syscall() {
    let syscall_num = regs[0]; // r0 = syscall number
    let arg1 = regs[1];        // r1-r3 = arguments
    let arg2 = regs[2];
    let arg3 = regs[3];

    let result = match syscall_num {
        1  => sys_exit(arg1),
        2  => sys_fork(),
        3  => sys_read(arg1, arg2, arg3),
        4  => sys_write(arg1, arg2, arg3),
        5  => sys_open(arg1, arg2),
        6  => sys_close(arg1),
        7  => sys_waitpid(arg1, arg2),
        11 => sys_execve(arg1, arg2, arg3),
        20 => sys_getpid(),
        45 => sys_brk(arg1),
        // ... Linux-compatible syscall numbers
        _  => Err(ENOSYS),
    };

    regs[0] = match result {
        Ok(val) => val as u32,
        Err(errno) => (-(errno as i32)) as u32,
    };
}
```

---

## 5. Block Device Emulation

Wotan extended memory (0x10000-0xFFFFFF = 16MB) as ramdisk.

```rust
// Ramdisk: 4MB starting at 0x100000
const RAMDISK_BASE: u32 = 0x100000;
const RAMDISK_SIZE: u32 = 4 * 1024 * 1024;
const BLOCK_SIZE: u32 = 512;

fn sys_read_block(block_num: u32, buf_addr: u32) -> Result<u32, i32> {
    let offset = block_num * BLOCK_SIZE;
    if offset + BLOCK_SIZE > RAMDISK_SIZE { return Err(EIO); }

    let src_addr = RAMDISK_BASE + offset;

    // DMA-style bulk read: one Wotan read for entire block
    bpf_wotan_read(flow_label, src_addr, buf_addr, BLOCK_SIZE)?;
    Ok(BLOCK_SIZE)
}
```

---

## 6. Console / TTY

Map to Busboy pub/sub channels.

```
Output: Shim writes to Wotan I/O address 0xC001 → Busboy publish "compute.tty.{label}"
Input:  Busboy subscribe "compute.input.{label}" → Wotan input register 0xFFFF

For full TTY:
  0xC001: TTY data output (write byte → publish)
  0xC002: TTY status register (read → buffer empty/full)
  0xC003: TTY control register (baud rate emulation, line discipline)
```

---

## 7. Boot Protocol

```
1. Computermancer loads kernel image into Wotan extended memory at 0x10000
2. Sets up initial page tables (identity map for first 1MB)
3. Initializes IVT (interrupt vector table at 0x0000)
4. Sets stack pointer (r3/SP) to top of data memory
5. Sets PC (r4) to 0x10000
6. Starts Shim execution loop

BOOT SEQUENCE:
  Wotan[0x0000-0x00FF] = IVT (256 vectors × 4 bytes)
  Wotan[0x0100-0x01FF] = Boot parameters (memory size, ramdisk addr, etc.)
  Wotan[0x0200-0x7FFF] = Kernel stack + early heap
  Wotan[0x10000+]      = Kernel image
  Wotan[0x100000+]     = Ramdisk (initrd/initramfs)
```

---

## 8. xv6 Porting Notes

xv6-riscv → xv6-mbc port considerations:

| xv6 Feature | RISC-V | MBC Equivalent |
|-------------|--------|----------------|
| Registers | x0-x31 | r0-r3 + shadow r16-r31 in Wotan |
| CSRs | mstatus, mtvec, etc. | Wotan special addresses |
| Page tables | Sv39 | Two-level 32-bit (see §3) |
| Timer | CLINT | BPF timer → INT 0x20 |
| UART | MMIO 0x10000000 | Wotan I/O 0xC001 |
| Disk | virtio | Ramdisk in Wotan extended |
| Atomics | AMO instructions | bpf_wotan_cas() |

Key challenge: xv6 uses 64-bit RISC-V. MBC is 32-bit. Need xv6-riscv32 port first, or adapt directly.

---

## 9. FUZIX Porting Notes

FUZIX targets Z80, 6502, 68000, ARM — already designed for tiny systems.

Advantages:
- Runs in <128KB RAM (we have 16MB in Wotan extended)
- Simple block device interface (matches our ramdisk)
- No MMU required (optional MMU support)
- ~30 syscalls to implement

This is probably the **fastest path to "Unix runs on UPC."**

---

## 10. Linux (nommu) — uClinux

Linux without MMU. Runs on ARM Cortex-M, m68k, etc.

Requirements:
- 32-bit flat address space ✅ (Wotan extended)
- 1MB+ RAM ✅ (16MB available)
- Timer interrupt ✅ (BPF timer)
- Serial console ✅ (Busboy tty)
- Block device ✅ (Wotan ramdisk)
- NO MMU needed ✅

This is **Level 6a** — the first real Linux milestone.
Estimated effort: MBC ISA needs ~10 more instructions (atomic ops, barrier, etc.)

---

## 11. Linux (full) — The Dream

Full Linux with MMU emulation.

The software TLB adds ~100-200ns per memory access.
Linux does A LOT of memory accesses.
Estimated performance: ~1/10th native speed minimum.

But it would BOOT. And that's the point.

The showcase: "I built a computer from a network protocol and booted Linux on it."
That's the resume line. That's the conversation starter. That's the proof of concept.

GPL ensures anyone can study how. Fork it. Improve it. The protocol is free.

---

*SPDX-License-Identifier: GPL-3.0-or-later*
*The road is long. The dream is real. One level at a time.*
