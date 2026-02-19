# Doom-over-IPv6 Build Pipeline
# C -> RV32I ELF -> MBC Bytecode

CC = riscv64-unknown-elf-gcc
CFLAGS = -march=rv32i -mabi=ilp32 -nostdlib -static -ffreestanding -fno-builtin -O2
# Restrict to x0-x15 registers (MBC has 16 registers)
CFLAGS += -ffixed-x16 -ffixed-x17 -ffixed-x18 -ffixed-x19
CFLAGS += -ffixed-x20 -ffixed-x21 -ffixed-x22 -ffixed-x23
CFLAGS += -ffixed-x24 -ffixed-x25 -ffixed-x26 -ffixed-x27
CFLAGS += -ffixed-x28 -ffixed-x29 -ffixed-x30 -ffixed-x31
LDFLAGS = -Wl,-Ttext=0x0 -Wl,-T,linker.ld

TRANSLATOR = ../../crates/monad-mbc/target/release/rv32i-to-mbc

# Doom source files (to be added when doomgeneric source is integrated)
DOOM_SRCS = doomgeneric_monad.c
DOOM_OBJS = $(DOOM_SRCS:.c=.o)

.PHONY: all clean translator

all: doom.mbc

translator:
	cd ../../crates/monad-mbc && cargo build --release --bin rv32i-to-mbc

doom.elf: $(DOOM_SRCS)
	$(CC) $(CFLAGS) $(LDFLAGS) -o $@ $^

doom.mbc: doom.elf translator
	$(TRANSLATOR) $< -o $@ --stats

clean:
	rm -f doom.elf doom.mbc *.o
