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
#define UPC_ST_SIZE             0x00000810
#define UPC_TAG_SLOT            0x51070000
#define UPC_TAG_A7              0xA7000000
#define UPC_TAG_MASK            0xFFFF0000

struct upc_tf_slot {
  struct trapframe tf;
  uint8 pad[UPC_TF_STRIDE - sizeof(struct trapframe)];
};

struct upc_slot_tables {
  struct upc_tf_slot tf[UPC_SLOTS];
  uint32 cur_slot;
  uint32 trap_a7;
  uint32 fresh_mask;
  uint32 pad;
};

_Static_assert(sizeof(struct upc_tf_slot) == UPC_TF_STRIDE, "ADR-109 trapframe stride");
_Static_assert(sizeof(struct upc_slot_tables) == UPC_ST_SIZE, "ADR-109 slot tables size");
_Static_assert(__builtin_offsetof(struct upc_slot_tables, cur_slot) == UPC_ST_CUR_SLOT_OFF, "cur_slot offset");
_Static_assert(__builtin_offsetof(struct upc_slot_tables, trap_a7) == UPC_ST_TRAP_A7_OFF, "trap_a7 offset");
_Static_assert(__builtin_offsetof(struct upc_slot_tables, fresh_mask) == UPC_ST_FRESH_MASK_OFF, "fresh_mask offset");

#endif
