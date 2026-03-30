# DOOM-over-IPv6 Cross-Compilation Build Guide

> **HISTORICAL DOCUMENT (doomgeneric era)**
> This guide was written for the doomgeneric port before the pivot to id
> linuxdoom-1.10. For current build instructions, see **RUNBOOK.md**.
> For the pivot decision, see **PIVOT-TO-ID-DOOM.md**.

**Updated:** 2026-03-02
**Status:** SUPERSEDED by RUNBOOK.md (id linuxdoom-1.10 port)
**Previous:** ARM64 macOS -> ARM64 Ubuntu VM (confirmed working)

---

## Overview

DOOM runs inside the eBPF packet-circulation ring as MBC (Monad Bytecode). The
build pipeline is:

```
C source → RV32I ELF → MBC bytecode → BPF maps → eBPF CPU execution
```

Each step:
1. **Cross-compile** doomgeneric C sources to RISC-V 32-bit integer-only ELF
2. **Translate** RV32I instructions to MBC bytecode (custom ISA, 16 registers)
3. **Extract** .rodata/.data sections from ELF for RAM_MAP
4. **Load** ROM, RAM, RV2MBC, WAD, and CPU state into BPF maps
5. **Execute** via packet-driven XDP hop ring (1 MBC instruction per hop)

---

## Prerequisites

### Toolchain (x86_64 Linux)

```bash
# Bare-metal RISC-V cross-compiler + minimal C library
sudo apt install gcc-riscv64-unknown-elf picolibc-riscv64-unknown-elf

# Verify
riscv64-unknown-elf-gcc --version
# Expected: riscv64-unknown-elf-gcc (14.2.0+19) 14.2.0
```

**Important:** Do NOT use `riscv64-linux-gnu-gcc` — it targets Linux userspace
and fails on 32-bit ilp32 ABI (missing `stubs-ilp32.h`). The bare-metal
`riscv64-unknown-elf-gcc` toolchain supports `-march=rv32i -mabi=ilp32`.

### RV32I→MBC Translator

```bash
cd crates/monad-mbc
cargo build --release
# Binary: target/release/rv32i-to-mbc
```

### WAD File

Place `doom1.wad` (4,196,020 bytes) in `~/tmp/DOOM_wads/` or project root.
The WAD is memory-mapped at address `0x00110000` in the MBC address space.

---

## Build Steps

### 1. Setup Build Directory

```bash
export PROJECT_ROOT=/home/govan/tmp/unheaded
export BUILD_DIR=/tmp/doom-build
export DOOM_SRC=$PROJECT_ROOT/demos/doom
export DOOMGENERIC_SRC=$DOOM_SRC/doomgeneric

mkdir -p $BUILD_DIR/include
```

### 2. Create Custom Minimal C Headers

The build uses custom headers instead of picolibc's system headers to avoid a
TLS `errno` conflict. picolibc defines `__thread int errno` (thread-local),
while our `libc_monad.c` defines plain `int errno` (bare-metal, no TLS).

Create these 17 headers in `$BUILD_DIR/include/`:

<details>
<summary>errno.h</summary>

```c
#ifndef _ERRNO_H
#define _ERRNO_H
extern int errno;
#define ENOENT 2
#define EINVAL 22
#define ENOMEM 12
#define EBADF  9
#define EISDIR 21
#endif
```
</details>

<details>
<summary>string.h</summary>

```c
#ifndef _STRING_H
#define _STRING_H
#include <stddef.h>
void *memcpy(void *dest, const void *src, size_t n);
void *memmove(void *dest, const void *src, size_t n);
void *memset(void *s, int c, size_t n);
int memcmp(const void *s1, const void *s2, size_t n);
size_t strlen(const char *s);
char *strcpy(char *dest, const char *src);
char *strncpy(char *dest, const char *src, size_t n);
int strcmp(const char *s1, const char *s2);
int strncmp(const char *s1, const char *s2, size_t n);
char *strcat(char *dest, const char *src);
char *strncat(char *dest, const char *src, size_t n);
char *strdup(const char *s);
char *strchr(const char *s, int c);
char *strrchr(const char *s, int c);
char *strstr(const char *haystack, const char *needle);
int strcasecmp(const char *s1, const char *s2);
int strncasecmp(const char *s1, const char *s2, size_t n);
#endif
```
</details>

<details>
<summary>strings.h</summary>

```c
#ifndef _STRINGS_H
#define _STRINGS_H
#include <string.h>
int strcasecmp(const char *s1, const char *s2);
int strncasecmp(const char *s1, const char *s2, size_t n);
#endif
```
</details>

<details>
<summary>stdio.h</summary>

```c
#ifndef _STDIO_H
#define _STDIO_H
#include <stddef.h>
#include <stdarg.h>
typedef struct _FILE FILE;
extern FILE *stdin;
extern FILE *stdout;
extern FILE *stderr;
#define EOF (-1)
#define SEEK_SET 0
#define SEEK_CUR 1
#define SEEK_END 2
#define NULL ((void*)0)
FILE *fopen(const char *path, const char *mode);
int fclose(FILE *fp);
size_t fread(void *ptr, size_t size, size_t nmemb, FILE *fp);
size_t fwrite(const void *ptr, size_t size, size_t nmemb, FILE *fp);
int fseek(FILE *fp, long offset, int whence);
long ftell(FILE *fp);
int feof(FILE *fp);
int ferror(FILE *fp);
int fflush(FILE *fp);
int fprintf(FILE *fp, const char *fmt, ...);
int printf(const char *fmt, ...);
int sprintf(char *buf, const char *fmt, ...);
int snprintf(char *buf, size_t size, const char *fmt, ...);
int vsnprintf(char *buf, size_t size, const char *fmt, va_list ap);
int vfprintf(FILE *fp, const char *fmt, va_list ap);
int sscanf(const char *str, const char *fmt, ...);
int puts(const char *s);
int putchar(int c);
int remove(const char *path);
int rename(const char *old, const char *new);
void perror(const char *s);
char *fgets(char *s, int size, FILE *fp);
int fileno(FILE *fp);
#endif
```
</details>

<details>
<summary>stdlib.h</summary>

```c
#ifndef _STDLIB_H
#define _STDLIB_H
#include <stddef.h>
void *malloc(size_t size);
void *calloc(size_t nmemb, size_t size);
void *realloc(void *ptr, size_t size);
void free(void *ptr);
void exit(int status);
void abort(void);
int abs(int j);
long labs(long j);
int atoi(const char *nptr);
long atol(const char *nptr);
double atof(const char *nptr);
long strtol(const char *nptr, char **endptr, int base);
unsigned long strtoul(const char *nptr, char **endptr, int base);
void qsort(void *base, size_t nmemb, size_t size, int (*compar)(const void *, const void *));
int rand(void);
void srand(unsigned int seed);
char *getenv(const char *name);
#define EXIT_SUCCESS 0
#define EXIT_FAILURE 1
#endif
```
</details>

<details>
<summary>ctype.h</summary>

```c
#ifndef _CTYPE_H
#define _CTYPE_H
int isalpha(int c);
int isdigit(int c);
int isalnum(int c);
int isspace(int c);
int isupper(int c);
int islower(int c);
int isprint(int c);
int toupper(int c);
int tolower(int c);
#endif
```
</details>

<details>
<summary>math.h</summary>

```c
#ifndef _MATH_H
#define _MATH_H
double sin(double x);
double cos(double x);
double tan(double x);
double sqrt(double x);
double fabs(double x);
double floor(double x);
double ceil(double x);
double pow(double base, double exp);
double log(double x);
double log10(double x);
double atan2(double y, double x);
double acos(double x);
double asin(double x);
double exp(double x);
double fmod(double x, double y);
float sinf(float x);
float cosf(float x);
float sqrtf(float x);
float fabsf(float x);
float floorf(float x);
float ceilf(float x);
#define M_PI 3.14159265358979323846
#define INFINITY (__builtin_inff())
#define NAN (__builtin_nanf(""))
#endif
```
</details>

<details>
<summary>assert.h, fcntl.h, unistd.h, signal.h, setjmp.h, time.h, inttypes.h, dirent.h, sys/stat.h, sys/types.h</summary>

These are minimal stubs — see `/tmp/doom-build/include/` for exact contents.
Key patterns:
- `assert.h`: `#define assert(x) ((void)0)` (disabled in bare-metal)
- `fcntl.h`: `O_RDONLY`, `O_WRONLY`, `O_CREAT` defines
- `unistd.h`: `read`, `write`, `close`, `lseek`, `access`, `mkdir`, `stat` declarations
- `sys/stat.h`: `struct stat` with `st_size` and `st_mode`
- `sys/types.h`: `off_t`, `ssize_t`, `mode_t`, `pid_t` typedefs
</details>

### 3. Create Patched Platform Layer

The original `doomgeneric_monad.c` has two problems:
- Defines `DG_ScreenBuffer` (conflicts with `doomgeneric.c` which also defines it)
- Has no `main()` function

Create `$BUILD_DIR/doomgeneric_monad_patched.c`:

```c
// Patched doomgeneric_monad.c — adds main(), removes DG_ScreenBuffer conflict
#include "doomgeneric.h"

#define SCREEN_BASE ((volatile uint8_t *)0x00070000)

#define SYS_DRAW_FRAME  0x01
#define SYS_GET_KEY     0x02
#define SYS_GET_TICKS   0x03
#define SYS_SLEEP       0x04

static inline uint32_t mbc_syscall(uint32_t num) {
    register uint32_t a7 asm("a7") = num;
    register uint32_t a0 asm("a0");
    asm volatile("ecall" : "=r"(a0) : "r"(a7));
    return a0;
}

extern void doomgeneric_Create(int argc, char **argv);

int main(void) {
    doomgeneric_Create(0, (char **)0);
    return 0;
}

void DG_Init(void) {}

void DG_DrawFrame(void) {
    // Convert ARGB pixels to 8-bit palette and write to VRAM
    pixel_t *fb = DG_ScreenBuffer;
    for (int i = 0; i < DOOMGENERIC_RESX * DOOMGENERIC_RESY; i++) {
        uint32_t argb = fb[i];
        uint8_t r = (argb >> 16) & 0xFF;
        uint8_t g = (argb >> 8) & 0xFF;
        uint8_t b = argb & 0xFF;
        SCREEN_BASE[i] = (uint8_t)((77 * r + 150 * g + 29 * b) >> 8);
    }
    mbc_syscall(SYS_DRAW_FRAME);
}

void DG_SleepMs(uint32_t ms) {
    (void)ms;
    mbc_syscall(SYS_SLEEP);
}

uint32_t DG_GetTicksMs(void) {
    return mbc_syscall(SYS_GET_TICKS);
}

int DG_GetKey(int *pressed, unsigned char *doomKey) {
    uint32_t raw = mbc_syscall(SYS_GET_KEY);
    if (raw == 0) return 0;
    *pressed = (raw >> 8) & 1;
    *doomKey = raw & 0xFF;
    return 1;
}

void DG_SetWindowTitle(const char *title) { (void)title; }
```

### 4. Create libgcc Stubs

The system `libgcc` uses high registers (x28-x31) which MBC doesn't support.
Create `$BUILD_DIR/libgcc_stubs.c` with custom integer math:

```c
// Custom integer math stubs — compiled with -ffixed-x16..x31
// Replaces libgcc's __mulsi3, __muldi3, __divsi3, etc.
// which use x28/x29 (t3/t4) that are outside MBC's x0-x15 range.

unsigned int __mulsi3(unsigned int a, unsigned int b) {
    unsigned int result = 0;
    while (b) {
        if (b & 1) result += a;
        a <<= 1;
        b >>= 1;
    }
    return result;
}

long long __muldi3(long long a, long long b) {
    unsigned long long ua = (unsigned long long)a;
    unsigned long long ub = (unsigned long long)b;
    unsigned long long result = 0;
    while (ub) {
        if (ub & 1) result += ua;
        ua <<= 1;
        ub >>= 1;
    }
    return (long long)result;
}

int __divsi3(int a, int b) {
    if (b == 0) return 0;
    int sign = 1;
    if (a < 0) { a = -a; sign = -sign; }
    if (b < 0) { b = -b; sign = -sign; }
    unsigned int ua = a, ub = b, q = 0;
    for (int i = 31; i >= 0; i--) {
        if (ua >= (ub << i)) { ua -= (ub << i); q |= (1u << i); }
    }
    return sign > 0 ? (int)q : -(int)q;
}

unsigned int __udivsi3(unsigned int a, unsigned int b) {
    if (b == 0) return 0;
    unsigned int q = 0;
    for (int i = 31; i >= 0; i--) {
        if (a >= (b << i)) { a -= (b << i); q |= (1u << i); }
    }
    return q;
}

int __modsi3(int a, int b) {
    return a - __divsi3(a, b) * b;
}

unsigned int __umodsi3(unsigned int a, unsigned int b) {
    return a - __udivsi3(a, b) * b;
}
```

### 5. Create Expanded Linker Script

The original `demos/doom/linker.ld` has ROM=256K and RAM=48K which overflow.
Create `$BUILD_DIR/linker_doom.ld`:

```ld
/* Doom MBC linker script — expanded memory regions
 *
 * Harvard architecture: ROM and RAM are separate address spaces,
 * both starting at 0x00000000. The LMA overlap warning is expected
 * and suppressed with --noinhibit-exec.
 */
ENTRY(_start)

MEMORY {
    ROM (rx)  : ORIGIN = 0x00000000, LENGTH = 1M
    RAM (rw)  : ORIGIN = 0x00000000, LENGTH = 16M
}

SECTIONS {
    .text : {
        *(.text.startup)
        *(.text*)
    } > ROM

    .rodata : {
        *(.rodata*)
        *(.srodata*)
    } > ROM

    .data : {
        *(.data*)
        *(.sdata*)
    } > RAM

    .bss (NOLOAD) : {
        __bss_start = .;
        *(.bss*)
        *(.sbss*)
        *(COMMON)
        __bss_end = .;
    } > RAM
}

_stack_top = 0xFFFF0000;
```

### 6. Compile All Sources

```bash
CC=riscv64-unknown-elf-gcc
GCC_INC=$(riscv64-unknown-elf-gcc -march=rv32i -mabi=ilp32 -print-file-name=include)

CFLAGS="-march=rv32i -mabi=ilp32 -O2 -nostdlib -nostdinc -ffreestanding -fno-builtin \
  -DDOOMGENERIC_RESX=320 -DDOOMGENERIC_RESY=200 \
  -ffixed-x16 -ffixed-x17 -ffixed-x18 -ffixed-x19 \
  -ffixed-x20 -ffixed-x21 -ffixed-x22 -ffixed-x23 \
  -ffixed-x24 -ffixed-x25 -ffixed-x26 -ffixed-x27 \
  -ffixed-x28 -ffixed-x29 -ffixed-x30 -ffixed-x31 \
  -isystem $GCC_INC \
  -isystem $BUILD_DIR/include \
  -I$DOOMGENERIC_SRC"

cd $BUILD_DIR

# Compile crt0 startup
$CC $CFLAGS -c $DOOM_SRC/crt0.S -o crt0.o

# Compile libc_monad (bare-metal C library)
$CC $CFLAGS -c $PROJECT_ROOT/docs/protocol/libc_monad.c -o libc_monad.o

# Compile libgcc stubs (custom integer math)
$CC $CFLAGS -c libgcc_stubs.c -o libgcc_stubs.o

# Compile patched platform layer
$CC $CFLAGS -c doomgeneric_monad_patched.c -o doomgeneric_monad_patched.o

# Compile all doomgeneric source files (80 files)
# EXCLUDE doomgeneric_monad.c (we use our patched version)
for f in $DOOMGENERIC_SRC/*.c; do
    base=$(basename "$f" .c)
    [[ "$base" == "doomgeneric_monad" ]] && continue
    [[ "$base" == "doomgeneric_sdl" ]]   && continue
    [[ "$base" == "doomgeneric_xlib" ]]  && continue
    [[ "$base" == "doomgeneric_soc" ]]   && continue
    [[ "$base" == "i_sdlmusic" ]]        && continue
    [[ "$base" == "i_sdlsound" ]]        && continue
    $CC $CFLAGS -c "$f" -o "${base}.o"
done

echo "Compiled $(ls *.o | wc -l) object files"
```

### 7. Link

```bash
# Link all objects — NO -lgcc (uses our libgcc_stubs.c instead)
# --noinhibit-exec suppresses LMA overlap warning (Harvard architecture)
$CC -march=rv32i -mabi=ilp32 -nostdlib -nostartfiles \
    -T $BUILD_DIR/linker_doom.ld \
    -Wl,--noinhibit-exec \
    -o doom.elf \
    crt0.o \
    $(ls *.o | grep -v crt0.o | sort) \
    libc_monad.o libgcc_stubs.o

echo "Linked: $(stat -c%s doom.elf) bytes"
file doom.elf
# Expected: ELF 32-bit LSB executable, UCB RISC-V, soft-float ABI, statically linked
```

### 8. Translate RV32I → MBC

```bash
TRANSLATOR=$PROJECT_ROOT/crates/monad-mbc/target/release/rv32i-to-mbc

$TRANSLATOR doom.elf -o doom.mbc --stats
# Expected output:
#   Translated 65917 RV32I instructions → 106611 MBC instructions
#   RV2MBC map: 65918 entries → doom.rv2mbc

ls -la doom.mbc doom.rv2mbc
```

### 9. Extract Data Sections

**CRITICAL:** Extract .rodata and .data as a SINGLE combined binary. These sections
are contiguous in RAM — .rodata at VMA 0x0, .data immediately after (e.g. 0x18CD0).
Extracting them separately then loading individually risks gaps or overlaps.

```bash
# Combined .rodata + .data extraction (single binary, contiguous in RAM)
riscv64-unknown-elf-objcopy -O binary -j .rodata -j '.srodata*' -j .data \
    doom.elf doom_data.bin

# Verify: should be .rodata size + .data size combined (~162K for current build)
ls -la doom_data.bin

# The combined binary starts at .rodata VMA (always 0x0 in our linker script)
# This is the base address for loading into RAM_MAP
RODATA_VMA=$(riscv64-unknown-elf-objdump -h doom.elf \
    | awk '/\.rodata[[:space:]]/{print "0x"$4}')
echo "data start address: $RODATA_VMA"
# Expected: 0x00000000
```

**Why combined extraction matters:** If doom_data.bin is generated from a different
doom.elf than doom.mbc, the `initialized` flag in .data will be at the wrong offset.
The flag persists as `1` from a previous run, causing D_DoomMain to be skipped entirely
(black screen). See Troubleshooting #9.

### 10. Copy Artifacts

```bash
cp doom.elf doom.mbc doom.rv2mbc doom_data.bin $PROJECT_ROOT/doom/
```

**Consistency check:** All artifacts MUST come from the same doom.elf build.
```bash
# Verify doom.mbc matches doom.elf (regenerate if in doubt)
$TRANSLATOR doom.elf -o doom.mbc --stats
```

---

## Loading into BPF Maps

### Prerequisites

The doom-ring must be running (6 network namespaces with eBPF programs loaded):

```bash
sudo scripts/doom-ring.sh setup
```

### Load All Data

The loader supports both `--flag` syntax and positional arguments:

```bash
LOADER=$PROJECT_ROOT/cmd/doom-loader/doom-loader
MAP_DIR=/sys/fs/bpf/unheaded/doom-ring/maps

# Build the loader if needed
(cd $PROJECT_ROOT && go build -o cmd/doom-loader/doom-loader ./cmd/doom-loader/)

# 1. Load ROM (MBC bytecode instructions)
sudo $LOADER rom $MAP_DIR/ROM_MAP doom/doom.mbc

# 2. Load combined .rodata+.data into RAM at VMA 0x0
#    CRITICAL: doom_data.bin must come from the SAME doom.elf as doom.mbc
DATA_VMA=$(riscv64-unknown-elf-objdump -h doom/doom.elf \
    | awk '/\.rodata[[:space:]]/{print "0x"$4}')
sudo $LOADER ram $MAP_DIR/RAM_MAP doom/doom_data.bin $DATA_VMA

# 3. Load WAD file into RAM at 0x110000
sudo $LOADER ram $MAP_DIR/RAM_MAP ~/tmp/DOOM_wads/doom1.wad 0x110000

# 4. Load RV2MBC address translation table
sudo $LOADER rv2mbc $MAP_DIR/RV2MBC_MAP doom/doom.rv2mbc

# 5. Initialize CPU state (instance 0xDE, SP=0x3F00000, PC=0)
#    MUST be last — loading data after CPU init can corrupt execution state
sudo $LOADER cpu --instance DE --map $MAP_DIR/CPU_MAP
```

Or use the shell wrapper (handles all steps including data VMA extraction):

```bash
sudo scripts/doom-loader.sh all
```

**Loading order matters:** ROM → data → WAD → RV2MBC → CPU. CPU must be
initialized LAST so that `initialized=0` in freshly-loaded .data is not
overwritten by stale RAM_MAP state.

### Verify CPU State

```bash
# Read CPU state from BPF map
sudo bpftool map lookup pinned $MAP_DIR/CPU_MAP key 0xDE 0x00 0x00 0x00
```

---

## Register Mapping

**Critical:** RV32I and MBC use different register numbering. The translator
remaps registers during instruction translation.

| RV32I | ABI Name | MBC Reg | Purpose |
|-------|----------|---------|---------|
| x0 | zero | r0 | Hardwired zero |
| x1 | ra | r14 | Return address |
| x2 | sp | **r15** | Stack pointer |
| x3 | gp | r1 | Global pointer |
| x4 | tp | r2 | Thread pointer |
| x5 | t0 | r3 | Temporary |
| x6 | t1 | r4 | Temporary |
| x7 | t2 | r5 | Temporary |
| x8 | s0/fp | r6 | Saved / Frame ptr |
| x9 | s1 | r7 | Saved |
| x10 | a0 | r8 | Arg 0 / Return |
| x11 | a1 | r9 | Arg 1 |
| x12 | a2 | r10 | Arg 2 |
| x13 | a3 | r11 | Arg 3 |
| x14 | a4 | r12 | Arg 4 |
| x15 | a5 | r13 | Arg 5 |

**Source:** `crates/monad-mbc/src/translator.rs:257-279`

**When inspecting CPU state:**
- "SP" is MBC `regs[15]`, NOT `regs[2]`
- "Return address" is MBC `regs[14]`, NOT `regs[1]`
- MBC `regs[2]` is actually RV32I x4 (tp), which is typically unused

---

## Memory Map

```
MBC Address Space (Harvard — ROM and RAM are separate)

ROM (instruction memory):
  0x00000000 — 0x000FFFFF   1 MiB  ROM_MAP (262K instructions max)

RAM (data memory) — current doom.elf layout:
  0x00000000 — 0x00018CCF   ~100K  .rodata (read-only data, strings, tables)
  0x00018CD0 — 0x0002778B   ~60K   .data (initialized globals, incl. initialized flag)
  0x00028000 — 0x0006393B   ~238K  .bss (zero-initialized globals, BSP vars, game state)
  0x00070000 — 0x0007F9FF   ~62K   SCREEN (320×200 8-bit framebuffer → SCREEN_MAP)
  0x00080000 — 0x0008F9FF   ~62K   FALLBACK_VIDEOBUFFER (ARGB, for 3D renderer stability)
  0x000F0000 — 0x0010FFFB   ~128K  Debug printf buffer (DBG_BUF)
  0x0010FFFC                 4B     Debug buffer length counter (DBG_LEN)
  0x00110000 — 0x005FFFFF   ~5M    WAD file (doom1.wad at 0x110000)
  0x00520000 — 0x0151FFFF   16M    Heap (malloc arena, bump allocator)
  0x03F00000                        Stack pointer (grows down)

BPF Maps:
  ROM_MAP     — Array<u32>, max_entries=262144 (1 MiB)
  RAM_MAP     — Array<u32>, max_entries=16,777,216 (64 MiB, word-addressed)
  RV2MBC_MAP  — Array<u32>, max_entries=65536
  CPU_MAP     — HashMap<u32, MbcCpuState>, key=flow_label
  SCREEN_MAP  — Array<u8>, max_entries=64000 (320×200)
  KB_MAP      — HashMap<u32, KeyboardState>, key=flow_label

Note: Byte writes to 0x70000-0x7F9FF are intercepted by eBPF and redirected
to SCREEN_MAP (separate from RAM_MAP). Word writes to the screen region go
to RAM_MAP only (Bug 24 fix). The FALLBACK_VIDEOBUFFER at 0x80000 is in
RAM_MAP — DOOM renders 3D to this buffer, then copy_fb_to_screen() copies
byte-by-byte to SCREEN_BASE (0x70000), which triggers SCREEN_MAP writes.
```

---

## Common Issues

### 1. "stubs-ilp32.h: No such file" with riscv64-linux-gnu-gcc

**Problem:** Linux cross-compiler doesn't support 32-bit ilp32 ABI.
**Fix:** Use bare-metal `riscv64-unknown-elf-gcc` instead.

### 2. "multiple definition of `DG_ScreenBuffer`"

**Problem:** Both `doomgeneric_monad.c` and `doomgeneric.c` define it.
**Fix:** Use the patched version that removes the duplicate definition.

### 3. TLS errno mismatch (picolibc vs libc_monad)

**Problem:** picolibc defines `__thread int errno`, libc_monad defines `int errno`.
**Fix:** Use custom headers with `-nostdinc -isystem` instead of picolibc headers.

### 4. ROM/RAM region overflow

**Problem:** Original linker.ld has ROM=256K, RAM=48K (both too small for DOOM).
**Fix:** Use expanded linker script with ROM=1M, RAM=16M.

### 5. LMA overlap warning

**Problem:** `.text` and `.data` both start at 0x00000000.
**Fix:** This is expected for Harvard architecture. Use `-Wl,--noinhibit-exec`.

### 6. "unsupported register x28/x29" in translation

**Problem:** libgcc's `__muldi3`/`__mulsi3` use x28-x31 (outside MBC range).
**Fix:** Create `libgcc_stubs.c` with custom implementations compiled with
`-ffixed-x16..x31`. Link without `-lgcc`.

### 7. RV2MBC_MAP overflow (>65536 entries)

**Problem:** Large builds produce more than 65536 RV32I instructions.
**Status:** First 65536 entries loaded. Overflow entries are typically for
libgcc stubs called via direct JAL (resolved at translation time, don't need
RV2MBC lookup at runtime). May need to increase `max_entries` in eBPF program
for indirect jumps beyond entry 65536.

### 8. SP appears in wrong register when inspecting CPU state

**Problem:** Looking at `regs[2]` and calling it "SP" — but MBC remaps x2→r15.
**Fix:** Check `regs[15]` for stack pointer value. See register mapping table.

### 9. Black screen — stale doom_data.bin / initialized flag persistence

**Problem:** DOOM renders a completely black screen. CPU executes billions of
instructions but SCREEN_MAP stays empty. All BSP variables (numnodes,
numsubsectors, numsegs, etc.) are zero. Debug buffer shows only startup
fragments (`\nxecutable.\n\neric 0.1\n`).

**Root cause:** The `initialized` static variable in `doomgeneric_monad_patched.c`
persists in RAM_MAP across CPU resets. When you re-initialize the CPU (PC=0) but
don't reload .data, `initialized=1` remains from the previous run. The game checks
`if (!initialized)` → skips `D_DoomMain()` entirely → no level geometry loaded →
black screen.

This commonly happens when:
- `doom_data.bin` was extracted from a different `doom.elf` than `doom.mbc` (build
  artifact mismatch — different section layouts mean the `initialized` flag is at
  the wrong offset or has the wrong value)
- `doom_data.bin` only contains `.data` but not `.rodata` (incomplete extraction)
- CPU state was re-initialized without reloading RAM_MAP data sections

**Fix:**
1. Ensure all artifacts come from the same `doom.elf`:
   ```bash
   cd /tmp/doom-build
   riscv64-unknown-elf-objcopy -O binary -j .rodata -j '.srodata*' -j .data \
       doom.elf doom_data.bin
   rv32i-to-mbc doom.elf -o doom.mbc --stats
   cp doom.elf doom.mbc doom.rv2mbc doom_data.bin $PROJECT_ROOT/doom/
   ```
2. Always reload data sections before restarting the CPU:
   ```bash
   sudo doom-loader ram $MAP_DIR/RAM_MAP doom/doom_data.bin 0x0
   sudo doom-loader cpu --instance DE --map $MAP_DIR/CPU_MAP
   ```
3. Verify `initialized=0` after loading (byte offset 0x2750C in current build):
   ```bash
   sudo bpftool map lookup pinned $MAP_DIR/RAM_MAP key hex 43 9d 00 00
   # Expected: 00 00 00 00 (initialized=0, ready for D_DoomMain)
   ```

**Diagnostic signs:** If you see `initialized=1` but all BSP variables are 0 and
gametic is 0, the game skipped initialization. Fresh load with matching artifacts
fixes this immediately.

---

## Key Files

| File | Purpose |
|------|---------|
| `demos/doom/crt0.S` | Startup code (set SP, zero BSS, call main) |
| `demos/doom/linker.ld` | Original linker script (ROM=256K, RAM=48K) |
| `demos/doom/Makefile` | Reference Makefile (may need adaptation) |
| `demos/doom/doomgeneric_monad.c` | Original MBC platform layer |
| `docs/protocol/libc_monad.c` | Complete bare-metal libc (457 lines) |
| `crates/monad-mbc/src/translator.rs` | RV32I→MBC translator (register mapping at line 257) |
| `crates/monad-mbc/src/bin/rv32i_to_mbc.rs` | Translator CLI binary |
| `cmd/doom-loader/main.go` | BPF map loader (Go) |
| `internal/bpf/bpf.go` | BPF map access + CpuState struct (Go) |
| `internal/doom/types.go` | Go types matching Rust MbcCpuState |
| `ebpf/monad-common/src/lib.rs` | Rust MbcCpuState + MbcInsn structs |
| `scripts/doom-loader.sh` | Shell wrapper for loading all data |
| `cmd/doom-bridge/main.go` | WebSocket bridge (SCREEN_MAP → browser) |
| `cmd/doom-bridge/input.go` | Keyboard input (browser → KB_MAP) |

---

## Build History

| Date | Platform | Status | Notes |
|------|----------|--------|-------|
| 2026-02 | ARM64 macOS → ARM64 Ubuntu VM | **Working** | Gameplay tested — shot bullets, changed screen size |
| 2026-03-01 | x86_64 Linux (native) | **Built** | 65,917 RV32I → 106,611 MBC. Black screen — stale doom_data.bin |
| 2026-03-02 | x86_64 Linux (native) | **Working** | Root cause fixed: build artifact mismatch + initialized flag persistence. Demo plays at ~25 ticks/sec, 99.2% screen fill. sendmmsg injector at ~97K pkt/s |

---

## CPU State Structure (104 bytes)

```
Offset  Size  Field           Notes
0       64    regs[0..15]     16 × u32, LE. r0=zero, r14=ra, r15=sp
64      4     pc              MBC instruction index (not byte address)
68      1     flags           bit0=Z, bit1=N, bit2=C
69      1     halted          0=running, 1=HALT executed
70      1     stalled         0=running, 1=waiting for cache
71      1     _pad            Alignment padding
72      8     sleep_until_ns  bpf_ktime_get_ns() sleep target
80      8     insn_count      Total instructions executed
88      8     cache_hits      L1 cache hit counter
96      8     cache_misses    L1 cache miss counter
```

Defined in:
- Rust: `ebpf/monad-common/src/lib.rs` (`MbcCpuState`)
- Go (bpf): `internal/bpf/bpf.go` (`CpuState`)
- Go (doom): `internal/doom/types.go` (`CpuState`)
