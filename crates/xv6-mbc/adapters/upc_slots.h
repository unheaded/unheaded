// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis.
//
// ADR-109 (EVOLUTION-1 Stage 2 rung 0): the slot tables the kernel shares with
// the BPF interpreter. Mirrored in ebpf/monad-cpu-ebpf/src/phase12.rs; the
// parity test cmd/upc-bootctl/tests/parity_slot_tables.rs keeps them equal.
#ifndef UPC_SLOTS_H
#define UPC_SLOTS_H

#define UPC_SLOT_TABLES_BP_BYTE 0x00000160 // BootParams reserved[0]: table address, 0 = unpublished
#define UPC_SLOTS               8
#define UPC_ST_TF_OFF           0x00000000 // uint32 tf[8]
#define UPC_ST_CUR_SLOT_OFF     0x00000020 // uint32 cur_slot
#define UPC_ST_GEN_OFF          0x00000024 // uint32 gen[8]
#define UPC_ST_TRAP_A7_OFF      0x00000044 // uint32 trap_a7
#define UPC_ST_TRAP_A6_OFF      0x00000048 // uint32 trap_a6

struct upc_slot_tables {
  uint32 tf[UPC_SLOTS];
  uint32 cur_slot;
  uint32 gen[UPC_SLOTS];
  uint32 trap_a7;
  uint32 trap_a6;
};

_Static_assert(sizeof(struct upc_slot_tables) == 76, "ADR-109 slot tables are 76 bytes");

#endif
