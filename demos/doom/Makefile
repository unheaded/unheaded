# SPDX-License-Identifier: GPL-3.0-or-later
# Doom-over-IPv6 Build Pipeline
# C -> RV32I ELF -> MBC Bytecode
#
# Targets:
#   doom.mbc          — Full Doom port (doomgeneric + MBC platform layer)
#   unheaded-doom.mbc — Standalone stubs (doomgeneric_unheaded.c + crt0.S)
#
# Computermancer notes:
#   - RV32I restricted to x0-x15 (MBC has 16 registers)
#   - WAD is memory-mapped at 0x400000 by doom-loader
#   - Framebuffer at 0x3C0000 (double-buffer: render to RAM, flush to SCREEN)

CC = riscv64-unknown-elf-gcc
OBJCOPY = riscv64-unknown-elf-objcopy
SIZE = riscv64-unknown-elf-size

# Bare-metal RV32I with MBC register constraints
CFLAGS = -march=rv32i -mabi=ilp32 -nostdlib -nostdinc -static -ffreestanding -fno-builtin -O2
# Restrict to x0-x15 registers (MBC has 16 registers, maps to Monad 20-byte register file)
CFLAGS += -ffixed-x16 -ffixed-x17 -ffixed-x18 -ffixed-x19
CFLAGS += -ffixed-x20 -ffixed-x21 -ffixed-x22 -ffixed-x23
CFLAGS += -ffixed-x24 -ffixed-x25 -ffixed-x26 -ffixed-x27
CFLAGS += -ffixed-x28 -ffixed-x29 -ffixed-x30 -ffixed-x31
# Include paths: our stubs first, then GCC builtins, then doomgeneric headers
GCC_INCLUDE = $(shell $(CC) -print-file-name=include)
CFLAGS += -isystem include
CFLAGS += -isystem $(GCC_INCLUDE)
CFLAGS += -I$(DOOMGENERIC_DIR)
# Doom build config
CFLAGS += -DDOOMGENERIC_RESX=320 -DDOOMGENERIC_RESY=200

LDFLAGS = -Wl,-T,linker.ld -Wl,--gc-sections -Wl,--no-warn-rwx-segments
# Link libgcc for software div/mul/float (RV32I base has no hardware M extension)
LIBGCC = $(shell $(CC) -march=rv32i -mabi=ilp32 -print-libgcc-file-name)

TRANSLATOR = ../../crates/monad-mbc/target/release/rv32i-to-mbc
DOOMGENERIC_DIR = $(HOME)/tmp/doomgeneric/doomgeneric

OBJDIR = build

# ── Source files ──────────────────────────────────────────────

# Our MBC platform layer + libc stubs + startup
PLATFORM_SRCS = \
	crt0.S \
	doomgeneric_monad.c \
	libc_stubs.c \
	gcc_runtime.c

# Upstream doomgeneric engine (from ~/tmp/doomgeneric/)
# Excludes: platform-specific backends (xlib, sdl, allegro, win, soso, emscripten, linuxvt)
# Excludes: sound backends (i_sdlsound, i_sdlmusic, i_allegrosound, i_allegromusic)
# Excludes: icon.c (X11 icon data)
# Excludes: gusconf.c (GUS sound card config)
# Excludes: mus2mid.c (MIDI conversion - no sound on MBC)
DOOM_SRCS = \
	dummy.c \
	am_map.c \
	doomdef.c \
	doomstat.c \
	dstrings.c \
	d_event.c \
	d_items.c \
	d_iwad.c \
	d_loop.c \
	d_main.c \
	d_mode.c \
	d_net.c \
	f_finale.c \
	f_wipe.c \
	g_game.c \
	hu_lib.c \
	hu_stuff.c \
	info.c \
	i_cdmus.c \
	i_endoom.c \
	i_joystick.c \
	i_scale.c \
	i_sound.c \
	i_system.c \
	i_timer.c \
	i_input.c \
	i_video.c \
	memio.c \
	m_argv.c \
	m_bbox.c \
	m_cheat.c \
	m_config.c \
	m_controls.c \
	m_fixed.c \
	m_menu.c \
	m_misc.c \
	m_random.c \
	p_ceilng.c \
	p_doors.c \
	p_enemy.c \
	p_floor.c \
	p_inter.c \
	p_lights.c \
	p_map.c \
	p_maputl.c \
	p_mobj.c \
	p_plats.c \
	p_pspr.c \
	p_saveg.c \
	p_setup.c \
	p_sight.c \
	p_spec.c \
	p_switch.c \
	p_telept.c \
	p_tick.c \
	p_user.c \
	r_bsp.c \
	r_data.c \
	r_draw.c \
	r_main.c \
	r_plane.c \
	r_segs.c \
	r_sky.c \
	r_things.c \
	sha1.c \
	sounds.c \
	statdump.c \
	st_lib.c \
	st_stuff.c \
	s_sound.c \
	tables.c \
	v_video.c \
	wi_stuff.c \
	w_checksum.c \
	w_file.c \
	w_main.c \
	w_wad.c \
	z_zone.c \
	w_file_stdc.c \
	doomgeneric.c

# Object files
PLATFORM_OBJS = $(addprefix $(OBJDIR)/, $(addsuffix .o, $(basename $(PLATFORM_SRCS))))
DOOM_OBJS = $(addprefix $(OBJDIR)/doom_, $(addsuffix .o, $(basename $(DOOM_SRCS))))
ALL_OBJS = $(PLATFORM_OBJS) $(DOOM_OBJS)

# ── VPATH for source lookup ──────────────────────────────────
VPATH = $(DOOMGENERIC_DIR)

# ── Targets ───────────────────────────────────────────────────

.PHONY: all clean translator stubs

all: doom.mbc

stubs: unheaded-doom.mbc

translator:
	cd ../../crates/monad-mbc && cargo build --release --bin rv32i-to-mbc

# --- Full Doom port ---
doom.elf: $(ALL_OBJS)
	@echo "[Link] $@"
	$(CC) $(CFLAGS) $(LDFLAGS) -o $@ $^
	@$(SIZE) $@ || true

doom.mbc: doom.elf translator
	@echo "[Translate] RV32I -> MBC"
	$(TRANSLATOR) $< -o $@ --stats

# --- Standalone unheaded stubs ---
STUB_SRCS = crt0.S doomgeneric_unheaded.c
STUB_OBJS = $(addprefix $(OBJDIR)/, $(addsuffix .o, $(basename $(STUB_SRCS))))

unheaded-doom.elf: $(STUB_OBJS)
	$(CC) $(CFLAGS) -DSTANDALONE_STUBS $(LDFLAGS) -o $@ $^

unheaded-doom.mbc: unheaded-doom.elf translator
	$(TRANSLATOR) $< -o $@ --stats

# --- Compilation rules ---

$(OBJDIR):
	mkdir -p $(OBJDIR)

$(ALL_OBJS) $(STUB_OBJS): | $(OBJDIR)

# Platform sources (from current directory)
$(OBJDIR)/%.o: %.c
	@echo "[CC] $<"
	$(CC) $(CFLAGS) -c $< -o $@

$(OBJDIR)/%.o: %.S
	@echo "[AS] $<"
	$(CC) $(CFLAGS) -c $< -o $@

# Doom engine sources (from doomgeneric directory, prefixed with doom_)
$(OBJDIR)/doom_%.o: $(DOOMGENERIC_DIR)/%.c
	@echo "[CC] $< (doom)"
	$(CC) $(CFLAGS) -c $< -o $@

clean:
	rm -rf $(OBJDIR)
	rm -f doom.elf doom.mbc unheaded-doom.elf unheaded-doom.mbc
