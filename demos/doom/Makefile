# SPDX-License-Identifier: GPL-3.0-or-later
# id DOOM 1.10 → RV32I → MBC Bytecode
# Build pipeline for Doom-over-IPv6 using original id Software source
#
# Source: ~/tmp/projects/DOOM/linuxdoom-1.10/ (pristine, never modified)
# Platform: demos/doom/ (MBC stubs replacing i_video, i_sound, i_net, i_system)

CC = riscv64-unknown-elf-gcc
SIZE = riscv64-unknown-elf-size
NM = riscv64-unknown-elf-nm

# Bare-metal RV32I with MBC register constraints (x0-x15 only)
CFLAGS = -march=rv32i -mabi=ilp32 -nostdlib -nostdinc -static -ffreestanding -fno-builtin -O2
CFLAGS += -ffixed-x16 -ffixed-x17 -ffixed-x18 -ffixed-x19
CFLAGS += -ffixed-x20 -ffixed-x21 -ffixed-x22 -ffixed-x23
CFLAGS += -ffixed-x24 -ffixed-x25 -ffixed-x26 -ffixed-x27
CFLAGS += -ffixed-x28 -ffixed-x29 -ffixed-x30 -ffixed-x31

# Include paths: our stubs first, then GCC builtins, then id DOOM headers
GCC_INCLUDE = $(shell $(CC) -print-file-name=include)
IDDOOM_DIR = /home/govan/tmp/projects/DOOM/linuxdoom-1.10
CFLAGS += -isystem include
CFLAGS += -isystem $(GCC_INCLUDE)
CFLAGS += -I$(IDDOOM_DIR)

# id DOOM build config: NORMALUNIX enables getenv/access WAD detection
CFLAGS += -DNORMALUNIX

# id DOOM 1.10 is K&R/C89 — needs implicit int, implicit function decls
IDDOOM_CFLAGS = $(CFLAGS) -std=gnu89 -Wno-implicit-int -Wno-implicit-function-declaration
# Our MBC platform code uses C99/C11 features (for-loop declarations, etc.)
MBC_CFLAGS = $(CFLAGS)

LDFLAGS = -Wl,-T,linker.ld -Wl,--gc-sections -Wl,--no-warn-rwx-segments -Wl,--allow-multiple-definition

TRANSLATOR = ../../crates/monad-mbc/target/release/rv32i-to-mbc

OBJDIR = build

# ── id DOOM source files (57 of 62, excluding 5 platform files) ──

IDDOOM_SRCS = \
	am_map.c d_items.c d_main.c d_net.c doomdef.c doomstat.c dstrings.c \
	f_finale.c f_wipe.c g_game.c hu_lib.c hu_stuff.c info.c \
	m_argv.c m_bbox.c m_cheat.c m_fixed.c m_menu.c m_misc.c m_random.c m_swap.c \
	p_ceilng.c p_doors.c p_enemy.c p_floor.c p_inter.c p_lights.c p_map.c \
	p_maputl.c p_mobj.c p_plats.c p_pspr.c p_saveg.c p_setup.c p_sight.c \
	p_spec.c p_switch.c p_telept.c p_tick.c p_user.c \
	r_bsp.c r_data.c r_draw.c r_main.c r_plane.c r_segs.c r_sky.c r_things.c \
	s_sound.c sounds.c st_lib.c st_stuff.c tables.c \
	v_video.c w_wad.c wi_stuff.c z_zone.c

# ── MBC platform stubs (replace i_main.c, i_video.c, i_sound.c, i_net.c, i_system.c) ──

MBC_SRCS = \
	i_system_mbc.c \
	i_video_mbc.c \
	i_sound_mbc.c \
	i_net_mbc.c

# ── Support files (startup, libc, gcc runtime) ──

SUPPORT_SRCS = \
	crt0.S \
	libc_stubs.c \
	gcc_runtime.c

# ── Object files ──

IDDOOM_OBJS = $(addprefix $(OBJDIR)/iddoom_, $(addsuffix .o, $(basename $(IDDOOM_SRCS))))
MBC_OBJS = $(addprefix $(OBJDIR)/, $(addsuffix .o, $(basename $(MBC_SRCS))))
SUPPORT_OBJS = $(addprefix $(OBJDIR)/, $(addsuffix .o, $(basename $(SUPPORT_SRCS))))
ALL_OBJS = $(SUPPORT_OBJS) $(MBC_OBJS) $(IDDOOM_OBJS)

# ── Targets ──

.PHONY: all clean translator check

all: doom.mbc

translator:
	cd ../../crates/monad-mbc && cargo build --release --bin rv32i-to-mbc

doom.elf: $(ALL_OBJS)
	@echo "[Link] $@"
	$(CC) $(CFLAGS) $(LDFLAGS) -o $@ $^
	@$(SIZE) $@ || true

doom.mbc: doom.elf
	@echo "[Translate] RV32I -> MBC"
	$(TRANSLATOR) $< -o $@ --stats

check: doom.elf
	@echo "=== ELF size ==="
	$(SIZE) $<
	@echo "=== Undefined symbols ==="
	$(NM) $< | grep " U " | head -20 || echo "(none)"

# ── Build directory ──

$(OBJDIR):
	mkdir -p $(OBJDIR)

$(ALL_OBJS): | $(OBJDIR)

# ── Compilation rules ──

# id DOOM sources (from IDDOOM_DIR, prefixed with iddoom_)
$(OBJDIR)/iddoom_%.o: $(IDDOOM_DIR)/%.c
	@echo "[CC] $< (id DOOM)"
	$(CC) $(IDDOOM_CFLAGS) -c $< -o $@

# MBC platform sources (from current directory)
$(OBJDIR)/%.o: %.c
	@echo "[CC] $< (MBC)"
	$(CC) $(MBC_CFLAGS) -c $< -o $@

# Assembly sources
$(OBJDIR)/%.o: %.S
	@echo "[AS] $<"
	$(CC) $(CFLAGS) -c $< -o $@

clean:
	rm -rf $(OBJDIR)
	rm -f doom.elf doom.mbc

