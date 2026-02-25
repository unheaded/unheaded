# Doom-over-IPv6 Build Pipeline
# C -> RV32I ELF -> MBC Bytecode
#
# Targets:
#   doom.mbc          — Full Doom port (doomgeneric_monad.c, original)
#   unheaded-doom.mbc — Standalone stubs (doomgeneric_unheaded.c + crt0.S)

CC = riscv64-unknown-elf-gcc
CFLAGS = -march=rv32i -mabi=ilp32 -nostdlib -static -ffreestanding -fno-builtin -O2
# Restrict to x0-x15 registers (MBC has 16 registers)
CFLAGS += -ffixed-x16 -ffixed-x17 -ffixed-x18 -ffixed-x19
CFLAGS += -ffixed-x20 -ffixed-x21 -ffixed-x22 -ffixed-x23
CFLAGS += -ffixed-x24 -ffixed-x25 -ffixed-x26 -ffixed-x27
CFLAGS += -ffixed-x28 -ffixed-x29 -ffixed-x30 -ffixed-x31
LDFLAGS = -Wl,-T,linker.ld

TRANSLATOR = ../../crates/monad-mbc/target/release/rv32i-to-mbc

# Original Doom port (full doomgeneric integration)
DOOM_SRCS = doomgeneric_monad.c
# Standalone stubs (MBC platform layer only, no full Doom source needed)
STUB_SRCS = crt0.S doomgeneric_unheaded.c

.PHONY: all clean translator stubs

all: doom.mbc

stubs: unheaded-doom.mbc

translator:
	cd ../../crates/monad-mbc && cargo build --release --bin rv32i-to-mbc

# --- Original Doom port ---
doom.elf: $(DOOM_SRCS)
	$(CC) $(CFLAGS) $(LDFLAGS) -o $@ $^

doom.mbc: doom.elf translator
	$(TRANSLATOR) $< -o $@ --stats

# --- Standalone unheaded stubs ---
unheaded-doom.elf: $(STUB_SRCS)
	$(CC) $(CFLAGS) -DSTANDALONE_STUBS $(LDFLAGS) -o $@ $^

unheaded-doom.mbc: unheaded-doom.elf translator
	$(TRANSLATOR) $< -o $@ --stats

clean:
	rm -f doom.elf doom.mbc unheaded-doom.elf unheaded-doom.mbc *.o
