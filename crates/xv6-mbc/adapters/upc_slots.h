// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis.
//
// ADR-109 (EVOLUTION-1 Stage 2 rung 0): the slot tables the kernel shares with
// the BPF interpreter. Mirrored in ebpf/monad-cpu-ebpf/src/phase12.rs; the
// parity test cmd/upc-bootctl/tests/parity_slot_tables.rs keeps them equal.
// Include after proc.h (struct trapframe).
#ifndef UPC_SLOTS_H
#define UPC_SLOTS_H

#define UPC_SLOT_TABLES_BP_BYTE 0x00000160 // BootParams reserved[0]: table address, 0 = unpublished
#define UPC_SLOTS               8
#define UPC_TF_STRIDE           0x00000100 // slot s's trapframe at base + s * stride
#define UPC_ST_CUR_SLOT_OFF     0x00000800 // UPC_TAG_SLOT | s, BPF at trap injection
#define UPC_ST_TRAP_A7_OFF      0x00000804 // UPC_TAG_A7 | nr, BPF at trap injection
#define UPC_ST_FRESH_MASK_OFF   0x00000808 // BPF sets bit child at fork; kernel clears on refresh
#define UPC_ST_TICKS_OFF        0x0000080C // BPF tick count at the last injection (ADR-110)
#define UPC_ST_BRK_IN_OFF       0x00000810 // cpu.program_break, BPF at trap injection (ADR-135 D3)
#define UPC_ST_REPLY_OFF        0x00000814 // UPC_TAG_REPLY | flags, kernel; BPF reads at SRET (ADR-135 D1)
#define UPC_ST_REPLY_ARG_OFF    0x00000818 // the reply flag's one argument, kernel
#define UPC_ST_CWD_MASK_OFF     0x0000081C // bit s: row s is off the root inode, kernel; BPF reads at open (ADR-135 D15)
#define UPC_ST_KBD_IN_OFF       0x00000820 // UPC_TAG_KBD | byte, or 0: the next console byte, BPF at a console read's injection (ADR-135 D20)
#define UPC_ST_SIZE             0x00000824
#define UPC_TAG_SLOT            0x51070000
#define UPC_TAG_A7              0xA7000000
#define UPC_TAG_REPLY           0x52500000
#define UPC_TAG_KBD             0x4B420000 // ADR-135 D20
#define UPC_TAG_MASK            0xFFFF0000
#define UPC_REPLY_SET_BRK       1
#define UPC_REPLY_YIELD         2
#define UPC_REPLY_KILL          4
#define UPC_REPLY_EXIT          8 // ADR-135 D13
#define UPC_REPLY_KBD           16 // ADR-135 D20: the kernel took the byte BPF offered; BPF advances the ring
#define UPC_RETRY_RC            (-2) // a retrying handler's return; syscall() leaves a0 alone (ADR-135 D7)
// ADR-135 D21 (rungs 5+6): 1 when the kernel's fd table is the only one. It must
// equal `const UPC_RUNG56` in ebpf/monad-cpu-ebpf/src/main.rs; the parity test
// cmd/upc-bootctl/tests/parity_slot_tables.rs reads both lines. A header, not a
// compiler flag, so make rebuilds what includes it.
#define UPC_RUNG56              0
#if UPC_RUNG56
#define UPC_KFD_BASE            0  // ADR-135 D21: a kernel fd is the ofile index
#else
#define UPC_KFD_BASE            16 // ADR-135 D10: kernel fd = 16 + ofile index; BPF's NOFILE
#endif
#define UPC_HEAP_BASE           0x00010000 // ADR-135 D2: VA window, inside fork's low copy band
#define UPC_HEAP_TOP            0x00040000
#define UPC_USER_VA_CEILING     0x00800000 // ADR-135 D6: USER_VA_CEILING in main.rs
#define UPC_SLICE_BASE(s)       ((s) == 0 ? 0 : 0x01000000 + ((s) - 1) * 0x00800000) // ADR-135 D5; phase12.rs pid_phys_offset
// ADR-135 D16: slot 0's base is 0, so its VAs are physical addresses. A kernel-routed
// pointer from init is answered only inside init's own windows: the image pages above
// the first (refused already, D6) and below the TTY registers, and the stack that
// uexec.c gives it (sp = 0x500000 - 16, one MiB of room).
#define UPC_INIT_IMAGE_LO       0x00001000
#define UPC_INIT_IMAGE_HI       0x0000C000
#define UPC_INIT_STACK_LO       0x00400000
#define UPC_INIT_STACK_HI       0x00500000

struct upc_tf_slot {
  struct trapframe tf;
  uint8 pad[UPC_TF_STRIDE - sizeof(struct trapframe)];
};

struct upc_slot_tables {
  struct upc_tf_slot tf[UPC_SLOTS];
  uint32 cur_slot;
  uint32 trap_a7;
  uint32 fresh_mask;
  uint32 ticks;
  uint32 brk_in;
  uint32 reply;
  uint32 reply_arg;
  uint32 cwd_mask;
  uint32 kbd_in;
};

struct proc;
void upc_slots_init(void);
void upc_slot_enter(void);
int  upc_is_slot_row(struct proc *p);
int  upc_slot_active(void);
void upc_slot_leave(void);
void upc_reply(uint32 flags, uint32 arg);
uint64 upc_sbrk(int n);
uint64 upc_kill(int pid);
uint64 upc_exit(int n);
uint64 upc_forknote(int s);
void upc_retry(void);
int  upc_retry_pending(void);
uint64 upc_pause(int n);
uint64 upc_user_pa(uint64 va);
int  upc_kbd_take(void);
void upc_cwd_mark(struct proc *p, int off_root);

_Static_assert(sizeof(struct upc_tf_slot) == UPC_TF_STRIDE, "ADR-109 trapframe stride");
_Static_assert(sizeof(struct upc_slot_tables) == UPC_ST_SIZE, "ADR-109 slot tables size");
_Static_assert(__builtin_offsetof(struct upc_slot_tables, cur_slot) == UPC_ST_CUR_SLOT_OFF, "cur_slot offset");
_Static_assert(__builtin_offsetof(struct upc_slot_tables, trap_a7) == UPC_ST_TRAP_A7_OFF, "trap_a7 offset");
_Static_assert(__builtin_offsetof(struct upc_slot_tables, fresh_mask) == UPC_ST_FRESH_MASK_OFF, "fresh_mask offset");
_Static_assert(__builtin_offsetof(struct upc_slot_tables, ticks) == UPC_ST_TICKS_OFF, "ticks offset");
_Static_assert(__builtin_offsetof(struct upc_slot_tables, brk_in) == UPC_ST_BRK_IN_OFF, "brk_in offset");
_Static_assert(__builtin_offsetof(struct upc_slot_tables, reply) == UPC_ST_REPLY_OFF, "reply offset");
_Static_assert(__builtin_offsetof(struct upc_slot_tables, reply_arg) == UPC_ST_REPLY_ARG_OFF, "reply_arg offset");
_Static_assert(__builtin_offsetof(struct upc_slot_tables, cwd_mask) == UPC_ST_CWD_MASK_OFF, "cwd_mask offset");
_Static_assert(__builtin_offsetof(struct upc_slot_tables, kbd_in) == UPC_ST_KBD_IN_OFF, "kbd_in offset");

#endif
