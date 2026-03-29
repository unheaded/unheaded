// SPDX-License-Identifier: GPL-3.0-or-later
// libc_stubs.c — Bare-metal libc for Doom on MBC (Monad Bytecode)
//
// Provides minimal C library functions for doomgeneric running in the UPC.
// WAD data is memory-mapped at WAD_BASE (0xC00000) by doom-runner.
// Screen buffer is memory-mapped at SCREEN_BASE (0xC000).
//
// Memory layout (matches linker.ld):
//   0x00000000 - 0x0000BFFF  RAM (48K, data + bss)
//   0x0000C000 - 0x0001B9FF  SCREEN (64000 bytes, 320x200)
//   0x00020000 - 0x001FFFFF  HEAP (1.875 MB, for z_zone allocator)
//   0x00200000 - 0x005FFFFF  WAD (4 MB, memory-mapped doom1.wad)

#include <stdint.h>
#include <stddef.h>
#include <stdarg.h>

// Forward declarations — we implement these below, but need them early
void *memcpy(void *dest, const void *src, size_t n);
void *memset(void *s, int c, size_t n);
int tolower(int c);
size_t strlen(const char *s);
int strcasecmp(const char *s1, const char *s2);
void debug_breadcrumb(uint32_t milestone);

// Types needed before their headers
typedef struct _FILE FILE;
#define EOF (-1)
typedef long time_t;
typedef long clock_t;
typedef unsigned int jmp_buf[16];
typedef void (*sig_t)(int);
// struct stat is defined in sys/stat.h but we need it early for forward decls
#include <sys/stat.h>

// ============================================================
// MBC syscalls (via RISC-V ecall)
// ============================================================

#define SYS_HALT 0x00

static inline void mbc_halt(void) {
    register uint32_t a7 __asm__("a7") = SYS_HALT;
    __asm__ volatile("ebreak" : : "r"(a7));
}


// ============================================================
// Memory allocator — simple bump allocator for Doom's z_zone
// ============================================================

// Heap boundaries — matches doom-runner/src/memory.rs (THE source of truth).
// HEAP: 0x1C0000 to 0x1BC0000 = 26MB. Doom pre-Z_Init uses ~10MB, Z_Init needs 3MB more.
// WAD: 0x1C00000. Stack: 0x2100000.
#define HEAP_START_ADDR 0x001C0000
#define HEAP_END_ADDR   0x01BC0000

// CRITICAL: heap_ptr lives at a FIXED address (0x1BF0000) far from .data.
// Previously it was a static in .data at ~0x10E714, surrounded by Doom globals.
// A wild pointer write from Doom code could corrupt it, bypassing malloc bounds.
// doom-runner initializes this address to HEAP_START_ADDR on load.
#define HEAP_PTR_ADDR ((char **)0x01BF0000)
#define heap_ptr (*HEAP_PTR_ADDR)

int errno;

// Each allocation is prefixed with a size_t header storing the usable size.
// This lets realloc know how many bytes to copy from the old allocation.
#define ALLOC_HEADER_SIZE 8  // sizeof(size_t), aligned to 8

void *malloc(size_t size) {
    // Validate heap_ptr bounds on EVERY call — wild writes can corrupt at any time.
    if (heap_ptr < (char *)HEAP_START_ADDR || heap_ptr >= (char *)HEAP_END_ADDR) {
        debug_breadcrumb(0x00FE); // heap_ptr was corrupt
        heap_ptr = (char *)HEAP_START_ADDR;
    }

    // Align requested size to 8 bytes
    size_t aligned = (size + 7) & ~7;
    // Total: header + payload
    size_t total = ALLOC_HEADER_SIZE + aligned;

    char *result = heap_ptr;
    char *new_ptr = heap_ptr + total;

    if (new_ptr > (char *)HEAP_END_ADDR || new_ptr < heap_ptr) {
        // Out of memory — capture details to debug region (above WAD, safe from heap)
        volatile unsigned int *OOM_DBG = (volatile unsigned int *)0x02051000;
        OOM_DBG[0] = 0xDEAD00FD;  // OOM marker
        OOM_DBG[1] = (unsigned)(unsigned long)heap_ptr;
        OOM_DBG[2] = (unsigned)size;
        OOM_DBG[3] = (unsigned)total;
        OOM_DBG[4] = (unsigned)(unsigned long)new_ptr;
        debug_breadcrumb(0x00FD); // OOM
        return (void *)0;
    }
    heap_ptr = new_ptr;

    // Store the usable size in the header, then return pointer past header
    *(size_t *)result = aligned;
    return (void *)(result + ALLOC_HEADER_SIZE);
}

void *calloc(size_t nmemb, size_t size) {
    size_t total = nmemb * size;
    void *p = malloc(total);
    if (p) memset(p, 0, total);
    return p;
}

void *realloc(void *ptr, size_t size) {
    void *newp = malloc(size);
    if (newp && ptr) {
        // Read old allocation size from header
        size_t old_size = *(size_t *)((char *)ptr - ALLOC_HEADER_SIZE);
        // New usable size is stored in new header
        size_t new_size = *(size_t *)((char *)newp - ALLOC_HEADER_SIZE);
        // Copy min(old_size, new_size) to avoid reading past old allocation
        size_t copy_size = old_size < new_size ? old_size : new_size;
        memcpy(newp, ptr, copy_size);
    }
    return newp;
}

void free(void *ptr) {
    // Bump allocator — no-op. z_zone manages its own recycling.
    (void)ptr;
}

// ============================================================
// String functions
// ============================================================

void *memcpy(void *dest, const void *src, size_t n) {
    unsigned char *d = (unsigned char *)dest;
    const unsigned char *s = (const unsigned char *)src;
    while (n--) *d++ = *s++;
    return dest;
}

void *memmove(void *dest, const void *src, size_t n) {
    unsigned char *d = (unsigned char *)dest;
    const unsigned char *s = (const unsigned char *)src;
    if (d < s) {
        while (n--) *d++ = *s++;
    } else {
        d += n;
        s += n;
        while (n--) *--d = *--s;
    }
    return dest;
}

void *memset(void *s, int c, size_t n) {
    unsigned char *p = (unsigned char *)s;
    while (n--) *p++ = (unsigned char)c;
    return s;
}

int memcmp(const void *s1, const void *s2, size_t n) {
    const unsigned char *a = (const unsigned char *)s1;
    const unsigned char *b = (const unsigned char *)s2;
    while (n--) {
        if (*a != *b) return *a - *b;
        a++; b++;
    }
    return 0;
}

size_t strlen(const char *s) {
    const char *p = s;
    while (*p) p++;
    return (size_t)(p - s);
}

int strcmp(const char *s1, const char *s2) {
    while (*s1 && *s1 == *s2) { s1++; s2++; }
    return (unsigned char)*s1 - (unsigned char)*s2;
}

int strncmp(const char *s1, const char *s2, size_t n) {
    while (n && *s1 && *s1 == *s2) { s1++; s2++; n--; }
    if (n == 0) return 0;
    return (unsigned char)*s1 - (unsigned char)*s2;
}

char *strcpy(char *dest, const char *src) {
    char *d = dest;
    while ((*d++ = *src++));
    return dest;
}

char *strncpy(char *dest, const char *src, size_t n) {
    char *d = dest;
    while (n && (*d++ = *src++)) n--;
    while (n--) *d++ = '\0';
    return dest;
}

char *strcat(char *dest, const char *src) {
    char *d = dest;
    while (*d) d++;
    while ((*d++ = *src++));
    return dest;
}

char *strncat(char *dest, const char *src, size_t n) {
    char *d = dest;
    while (*d) d++;
    while (n-- && (*d = *src++)) d++;
    *d = '\0';
    return dest;
}

char *strchr(const char *s, int c) {
    while (*s) {
        if (*s == (char)c) return (char *)s;
        s++;
    }
    return (c == '\0') ? (char *)s : (char *)0;
}

char *strrchr(const char *s, int c) {
    const char *last = (char *)0;
    while (*s) {
        if (*s == (char)c) last = s;
        s++;
    }
    if (c == '\0') return (char *)s;
    return (char *)last;
}

char *strstr(const char *haystack, const char *needle) {
    size_t nlen = strlen(needle);
    if (!nlen) return (char *)haystack;
    while (*haystack) {
        if (strncmp(haystack, needle, nlen) == 0)
            return (char *)haystack;
        haystack++;
    }
    return (char *)0;
}

char *strdup(const char *s) {
    size_t len = strlen(s) + 1;
    char *dup = (char *)malloc(len);
    if (dup) memcpy(dup, s, len);
    return dup;
}

int strcasecmp(const char *s1, const char *s2) {
    while (*s1 && (tolower((unsigned char)*s1) == tolower((unsigned char)*s2))) {
        s1++; s2++;
    }
    return tolower((unsigned char)*s1) - tolower((unsigned char)*s2);
}

int strncasecmp(const char *s1, const char *s2, size_t n) {
    while (n && *s1 && (tolower((unsigned char)*s1) == tolower((unsigned char)*s2))) {
        s1++; s2++; n--;
    }
    if (n == 0) return 0;
    return tolower((unsigned char)*s1) - tolower((unsigned char)*s2);
}

// ============================================================
// ctype functions
// ============================================================

int toupper(int c) {
    if (c >= 'a' && c <= 'z') return c - 32;
    return c;
}

int tolower(int c) {
    if (c >= 'A' && c <= 'Z') return c + 32;
    return c;
}

int isdigit(int c) { return c >= '0' && c <= '9'; }
int isalpha(int c) { return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z'); }
int isalnum(int c) { return isalpha(c) || isdigit(c); }
int isspace(int c) { return c == ' ' || c == '\t' || c == '\n' || c == '\r' || c == '\f' || c == '\v'; }
int isprint(int c) { return c >= 0x20 && c <= 0x7E; }
int isupper(int c) { return c >= 'A' && c <= 'Z'; }
int islower(int c) { return c >= 'a' && c <= 'z'; }
int isxdigit(int c) { return isdigit(c) || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F'); }

// ============================================================
// Conversion functions
// ============================================================

int atoi(const char *s) {
    int n = 0, neg = 0;
    while (isspace(*s)) s++;
    if (*s == '-') { neg = 1; s++; }
    else if (*s == '+') { s++; }
    while (isdigit(*s)) {
        n = n * 10 + (*s - '0');
        s++;
    }
    return neg ? -n : n;
}

long atol(const char *s) { return (long)atoi(s); }
double atof(const char *s) { (void)s; return 0.0; } // Doom doesn't actually use float configs

int abs(int x) { return x < 0 ? -x : x; }

// ============================================================
// Formatted output — minimal printf implementation
// ============================================================

// For bare-metal MBC: printf output is discarded (no console).
// Doom's important output goes through the framebuffer, not stdio.

static int mini_vsnprintf(char *buf, size_t size, const char *fmt, va_list ap) {
    char *dst = buf;
    char *end = buf + (size ? size - 1 : 0);

    while (*fmt) {
        if (*fmt != '%') {
            if (dst < end) *dst = *fmt;
            dst++;
            fmt++;
            continue;
        }
        fmt++; // skip %

        // Handle flags
        int left = 0, zero_pad = 0, plus = 0;
        while (*fmt == '-' || *fmt == '0' || *fmt == '+' || *fmt == ' ') {
            if (*fmt == '-') left = 1;
            if (*fmt == '0') zero_pad = 1;
            if (*fmt == '+') plus = 1;
            fmt++;
        }
        (void)left; (void)plus;

        // Width
        int width = 0;
        while (isdigit(*fmt)) { width = width * 10 + (*fmt - '0'); fmt++; }

        // Precision
        int prec = -1;
        if (*fmt == '.') {
            fmt++;
            prec = 0;
            while (isdigit(*fmt)) { prec = prec * 10 + (*fmt - '0'); fmt++; }
        }

        // Length modifier
        int is_long = 0;
        if (*fmt == 'l') { is_long = 1; fmt++; if (*fmt == 'l') fmt++; }
        if (*fmt == 'h') { fmt++; if (*fmt == 'h') fmt++; }
        if (*fmt == 'z' || *fmt == 'j' || *fmt == 't') fmt++;

        // Conversion
        switch (*fmt) {
        case 'd': case 'i': {
            long val = is_long ? va_arg(ap, long) : (long)va_arg(ap, int);
            char tmp[20];
            int neg = 0, len = 0;
            unsigned long uval;
            if (val < 0) { neg = 1; uval = (unsigned long)(-val); } else { uval = (unsigned long)val; }
            do { tmp[len++] = '0' + (uval % 10); uval /= 10; } while (uval);
            if (neg) tmp[len++] = '-';
            int pad = width - len;
            while (!left && pad-- > 0) { if (dst < end) *dst = zero_pad ? '0' : ' '; dst++; }
            while (len--) { if (dst < end) *dst = tmp[len]; dst++; }
            while (left && pad-- > 0) { if (dst < end) *dst = ' '; dst++; }
            break;
        }
        case 'u': {
            unsigned long val = is_long ? va_arg(ap, unsigned long) : (unsigned long)va_arg(ap, unsigned int);
            char tmp[20]; int len = 0;
            do { tmp[len++] = '0' + (val % 10); val /= 10; } while (val);
            int pad = width - len;
            while (pad-- > 0) { if (dst < end) *dst = zero_pad ? '0' : ' '; dst++; }
            while (len--) { if (dst < end) *dst = tmp[len]; dst++; }
            break;
        }
        case 'x': case 'X': {
            unsigned long val = is_long ? va_arg(ap, unsigned long) : (unsigned long)va_arg(ap, unsigned int);
            const char *hex = (*fmt == 'x') ? "0123456789abcdef" : "0123456789ABCDEF";
            char tmp[16]; int len = 0;
            do { tmp[len++] = hex[val & 0xF]; val >>= 4; } while (val);
            int pad = width - len;
            while (pad-- > 0) { if (dst < end) *dst = zero_pad ? '0' : ' '; dst++; }
            while (len--) { if (dst < end) *dst = tmp[len]; dst++; }
            break;
        }
        case 'p': {
            unsigned long val = (unsigned long)va_arg(ap, void*);
            if (dst < end) *dst = '0'; dst++;
            if (dst < end) *dst = 'x'; dst++;
            const char *hex = "0123456789abcdef";
            char tmp[16]; int len = 0;
            do { tmp[len++] = hex[val & 0xF]; val >>= 4; } while (val);
            while (len--) { if (dst < end) *dst = tmp[len]; dst++; }
            break;
        }
        case 's': {
            const char *s = va_arg(ap, const char *);
            if (!s) s = "(null)";
            size_t slen = strlen(s);
            if (prec >= 0 && (size_t)prec < slen) slen = (size_t)prec;
            int pad = width - (int)slen;
            while (!left && pad-- > 0) { if (dst < end) *dst = ' '; dst++; }
            for (size_t i = 0; i < slen; i++) { if (dst < end) *dst = s[i]; dst++; }
            while (left && pad-- > 0) { if (dst < end) *dst = ' '; dst++; }
            break;
        }
        case 'c': {
            char c = (char)va_arg(ap, int);
            if (dst < end) *dst = c;
            dst++;
            break;
        }
        case '%':
            if (dst < end) *dst = '%';
            dst++;
            break;
        case '\0':
            goto done;
        default:
            if (dst < end) *dst = *fmt;
            dst++;
            break;
        }
        fmt++;
    }
done:
    if (size) *dst = '\0';
    return (int)(dst - buf);
}

int vsnprintf(char *str, size_t size, const char *fmt, va_list ap) {
    return mini_vsnprintf(str, size, fmt, ap);
}

int vsprintf(char *str, const char *fmt, va_list ap) {
    return mini_vsnprintf(str, (size_t)-1, fmt, ap);
}

int snprintf(char *str, size_t size, const char *fmt, ...) {
    va_list ap;
    va_start(ap, fmt);
    int r = vsnprintf(str, size, fmt, ap);
    va_end(ap);
    return r;
}

int sprintf(char *str, const char *fmt, ...) {
    va_list ap;
    va_start(ap, fmt);
    int r = vsprintf(str, fmt, ap);
    va_end(ap);
    return r;
}

// Console output — no-op on bare-metal MBC
int printf(const char *fmt, ...) { (void)fmt; return 0; }
int vprintf(const char *fmt, va_list ap) { (void)fmt; (void)ap; return 0; }

// FILE operations — no real file system, but we need to be able to
// open and read the WAD. We'll override stderr/stdout to discard output.

FILE *stdin = (FILE *)0;
FILE *stdout = (FILE *)1;
FILE *stderr = (FILE *)2;

// Debug: write I_Error messages to a fixed RAM region so we can read them
// via bpftool. Address 0x7BF000 (just before screen) = debug string buffer.
#define DEBUG_MSG_ADDR ((volatile char *)0x007BF000)
#define DEBUG_MSG_MAX  256

// Debug breadcrumb region: 0x7BE000 (256 bytes before debug messages).
// Each breadcrumb is a 4-byte milestone ID written sequentially.
// Read with: bpftool map lookup ... key <word_addr_of_0x7BE000>
#define BREADCRUMB_ADDR ((volatile uint32_t *)0x007BE000)
#define BREADCRUMB_MAX  64

static int breadcrumb_idx = 0;

// Write a milestone marker to a fixed RAM address for post-mortem diagnosis.
// Milestones (defined in doomgeneric_monad.c and here):
//   0x0001 = main() entered
//   0x0002 = doomgeneric_Create called
//   0x0010 = DG_Init entered
//   0x0011 = DG_Init complete
//   0x0020 = Z_Init entered
//   0x0021 = Z_Init complete
//   0x0030 = WAD fopen attempted
//   0x0031 = WAD fopen succeeded
//   0x0032 = WAD fopen failed
//   0x0040 = I_InitGraphics entered
//   0x0041 = I_InitGraphics complete
//   0x0050 = D_DoomMain game loop reached
//   0x00FF = (removed — malloc validates every call now)
//   0xDEAD = I_Error / exit with error
void debug_breadcrumb(uint32_t milestone) {
    if (breadcrumb_idx >= BREADCRUMB_MAX) return;
    BREADCRUMB_ADDR[breadcrumb_idx] = milestone;
    breadcrumb_idx++;
    // Also write count at index 63 so we know how many breadcrumbs exist
    BREADCRUMB_ADDR[BREADCRUMB_MAX - 1] = (uint32_t)breadcrumb_idx;
}

static int debug_msg_count = 0;
static void debug_write_string(const char *s) {
    // Capture ALL error messages (up to 4, at 64-byte intervals)
    if (debug_msg_count >= 4) return;
    if (s[0] == '\n' && s[1] == '\0') return; // skip bare newlines
    volatile char *dst = DEBUG_MSG_ADDR + (debug_msg_count * 64);
    int i = 0;
    while (s[i] && i < 63) {
        dst[i] = s[i];
        i++;
    }
    dst[i] = '\0';
    debug_msg_count++;
}

int fprintf(FILE *stream, const char *fmt, ...) {
    if (stream == (FILE *)2) { // stderr — capture I_Error output
        va_list ap;
        va_start(ap, fmt);
        char buf[DEBUG_MSG_MAX];
        vsnprintf(buf, sizeof(buf), fmt, ap);
        va_end(ap);
        debug_write_string(buf);
    }
    return 0;
}
int vfprintf(FILE *stream, const char *fmt, va_list ap) {
    if (stream == (FILE *)2) { // stderr
        debug_write_string(fmt);
        char buf[DEBUG_MSG_MAX];
        vsnprintf(buf, sizeof(buf), fmt, ap);
        debug_write_string(buf);
    }
    return 0;
}

// Also intercept I_Error directly via a wrapper
// Doom's I_Error calls vfprintf(stderr, ...) then exit(-1)
// But just in case, also write at function entry
void __attribute__((used)) _doom_error_hook(const char *msg) {
    debug_write_string(msg);
}
int putchar(int c) { (void)c; return c; }
int puts(const char *s) { (void)s; return 0; }
int fputs(const char *s, FILE *stream) { (void)s; (void)stream; return 0; }
int fputc(int c, FILE *stream) { (void)c; (void)stream; return c; }
int fgetc(FILE *stream) { (void)stream; return EOF; }
char *fgets(char *s, int size, FILE *stream) { (void)s; (void)size; (void)stream; return (char *)0; }
// Minimal sscanf — handles %d, %i, %x, %o, %u, %s, %c (one conversion per call).
// Doom uses sscanf for M_StrToInt (config parsing) and ParseIntParameter.
int sscanf(const char *str, const char *fmt, ...) {
    va_list ap;
    va_start(ap, fmt);
    int conversions = 0;
    const char *s = str;

    while (*fmt) {
        if (*fmt == ' ') {
            // Skip whitespace in format → skip whitespace in input
            while (isspace(*s)) s++;
            fmt++;
            continue;
        }
        if (*fmt != '%') {
            // Literal match
            if (*s != *fmt) break;
            s++; fmt++;
            continue;
        }
        fmt++; // skip '%'
        if (*fmt == '%') { // literal %
            if (*s != '%') break;
            s++; fmt++;
            continue;
        }

        // Parse conversion
        switch (*fmt) {
        case 'd': case 'i': {
            int *out = va_arg(ap, int *);
            while (isspace(*s)) s++;
            int neg = 0, val = 0, got = 0;
            if (*s == '-') { neg = 1; s++; }
            else if (*s == '+') { s++; }
            // %i: auto-detect base (0x=hex, 0=octal, else decimal)
            if (*fmt == 'i' && *s == '0') {
                if (s[1] == 'x' || s[1] == 'X') {
                    s += 2;
                    while (isxdigit(*s)) {
                        int d = isdigit(*s) ? *s - '0' : (tolower(*s) - 'a' + 10);
                        val = val * 16 + d; s++; got = 1;
                    }
                } else {
                    s++; got = 1; // the leading '0' counts
                    while (*s >= '0' && *s <= '7') { val = val * 8 + (*s - '0'); s++; }
                }
            } else {
                while (isdigit(*s)) { val = val * 10 + (*s - '0'); s++; got = 1; }
            }
            if (!got) break;
            *out = neg ? -val : val;
            conversions++;
            break;
        }
        case 'u': {
            unsigned int *out = va_arg(ap, unsigned int *);
            while (isspace(*s)) s++;
            unsigned int val = 0; int got = 0;
            while (isdigit(*s)) { val = val * 10 + (*s - '0'); s++; got = 1; }
            if (!got) break;
            *out = val;
            conversions++;
            break;
        }
        case 'x': case 'X': {
            unsigned int *out = va_arg(ap, unsigned int *);
            while (isspace(*s)) s++;
            // Skip optional 0x prefix
            if (s[0] == '0' && (s[1] == 'x' || s[1] == 'X')) s += 2;
            unsigned int val = 0; int got = 0;
            while (isxdigit(*s)) {
                int d = isdigit(*s) ? *s - '0' : (tolower(*s) - 'a' + 10);
                val = val * 16 + d; s++; got = 1;
            }
            if (!got) break;
            *out = val;
            conversions++;
            break;
        }
        case 'o': {
            unsigned int *out = va_arg(ap, unsigned int *);
            while (isspace(*s)) s++;
            unsigned int val = 0; int got = 0;
            while (*s >= '0' && *s <= '7') { val = val * 8 + (*s - '0'); s++; got = 1; }
            if (!got) break;
            *out = val;
            conversions++;
            break;
        }
        case 's': {
            char *out = va_arg(ap, char *);
            while (isspace(*s)) s++;
            while (*s && !isspace(*s)) *out++ = *s++;
            *out = '\0';
            conversions++;
            break;
        }
        case 'c': {
            char *out = va_arg(ap, char *);
            if (!*s) break;
            *out = *s++;
            conversions++;
            break;
        }
        default:
            break;
        }
        fmt++;
    }

    va_end(ap);
    return conversions;
}

// ============================================================
// File I/O — Memory-mapped WAD access
// ============================================================
// The WAD file is loaded into MBC memory at WAD_BASE by the doom-loader.
// We emulate fopen/fread/fseek to read from this memory region.

#define WAD_BASE     0x01C00000
#define WAD_MAX_SIZE 12408292  // retail DOOM.WAD exact size (12,408,292 bytes)

// File table for memory-mapped WAD access.
// Doom opens one WAD file; multiple fopen/fclose cycles reuse slots.
#define MAX_FILES 4

struct mbc_file {
    int in_use;
    const uint8_t *base;
    size_t size;
    size_t pos;
};

static struct mbc_file file_table[MAX_FILES];

// Track which slot is the "primary" WAD reader (set by last successful fopen).
// Corrupted FILE* from Z_Malloc'd structs fall back to this slot.
static int primary_wad_slot = -1;

FILE *fopen(const char *path, const char *mode) {
    (void)mode;
    debug_breadcrumb(0x0030); // fopen attempted

    // Check if this looks like a WAD file
    int is_wad = 0;
    size_t plen = strlen(path);
    if (plen >= 4) {
        const char *ext = path + plen - 4;
        if (strcasecmp(ext, ".wad") == 0) is_wad = 1;
    }

    if (!is_wad) return (FILE *)0;

    // Find free slot
    for (int i = 0; i < MAX_FILES; i++) {
        if (!file_table[i].in_use) {
            file_table[i].in_use = 1;
            file_table[i].base = (const uint8_t *)WAD_BASE;
            file_table[i].size = WAD_MAX_SIZE;
            file_table[i].pos = 0;
            primary_wad_slot = i;  // last successful open is the primary
            debug_breadcrumb(0x0031); // WAD fopen succeeded
            return (FILE *)(uintptr_t)(i + 100);
        }
    }
    debug_breadcrumb(0x0032); // fopen failed (no free slot)
    return (FILE *)0;
}

static struct mbc_file *get_file(FILE *stream) {
    uintptr_t h = (uintptr_t)stream;
    if (h <= 2) return (struct mbc_file *)0;  // stdin/stdout/stderr
    int idx = (int)h - 100;
    if (idx >= 0 && idx < MAX_FILES && file_table[idx].in_use) {
        return &file_table[idx];
    }
    // Fallback: corrupted FILE* from Z_Malloc'd structs → use primary WAD slot
    if (primary_wad_slot >= 0 && file_table[primary_wad_slot].in_use)
        return &file_table[primary_wad_slot];
    return (struct mbc_file *)0;
}

int fclose(FILE *stream) {
    // Close exact-match handles only (no fallback path).
    // Corrupted FILE* from Z_Malloc'd structs are silently ignored.
    uintptr_t h = (uintptr_t)stream;
    if (h <= 2) return 0;  // stdin/stdout/stderr — no-op
    int idx = (int)h - 100;
    if (idx >= 0 && idx < MAX_FILES && file_table[idx].in_use) {
        file_table[idx].in_use = 0;
        file_table[idx].pos = 0;
    }
    return 0;
}

size_t fread(void *ptr, size_t size, size_t nmemb, FILE *stream) {
    struct mbc_file *f = get_file(stream);
    if (!f) return 0;

    size_t total = size * nmemb;
    size_t avail = (f->pos < f->size) ? (f->size - f->pos) : 0;
    if (total > avail) total = avail;

    memcpy(ptr, f->base + f->pos, total);
    f->pos += total;
    return total / size;
}

size_t fwrite(const void *ptr, size_t size, size_t nmemb, FILE *stream) {
    // No writable files on bare-metal
    (void)ptr; (void)size; (void)nmemb; (void)stream;
    return 0;
}

int fseek(FILE *stream, long offset, int whence) {
    struct mbc_file *f = get_file(stream);
    if (!f) return -1;

    long newpos;
    switch (whence) {
    case 0: newpos = offset; break;                      // SEEK_SET
    case 1: newpos = (long)f->pos + offset; break;       // SEEK_CUR
    case 2: newpos = (long)f->size + offset; break;      // SEEK_END
    default: return -1;
    }
    if (newpos < 0) return -1;
    f->pos = (size_t)newpos;
    return 0;
}

long ftell(FILE *stream) {
    struct mbc_file *f = get_file(stream);
    if (!f) return -1;
    return (long)f->pos;
}

int feof(FILE *stream) {
    struct mbc_file *f = get_file(stream);
    if (!f) return 1;
    return f->pos >= f->size;
}

int ferror(FILE *stream) { (void)stream; return 0; }

void rewind(FILE *stream) {
    struct mbc_file *f = get_file(stream);
    if (f) f->pos = 0;
}

int rename(const char *old, const char *new_) { (void)old; (void)new_; return -1; }
int remove(const char *path) { (void)path; return -1; }
FILE *tmpfile(void) { return (FILE *)0; }
int fflush(FILE *stream) { (void)stream; return 0; }

// ============================================================
// System functions
// ============================================================

void exit(int status) {
    debug_breadcrumb(status == 0 ? 0x0000 : 0xDEAD);
    // Write exit status as first 4 bytes of debug region
    // Using byte-by-byte write which definitely works on MBC
    volatile char *d = DEBUG_MSG_ADDR;
    // Write status as decimal string
    if (status == 0) {
        d[0] = 'O'; d[1] = 'K'; d[2] = '!'; d[3] = '\0';
    } else {
        d[0] = 'E'; d[1] = 'R'; d[2] = 'R'; d[3] = '\0';
    }
    mbc_halt();
    while (1) {}
}

void abort(void) {
    mbc_halt();
    while (1) {}
}

// Minimal qsort (shell sort — simpler than quicksort, no recursion stack issues)
void qsort(void *base, size_t nmemb, size_t size,
           int (*compar)(const void *, const void *)) {
    char *arr = (char *)base;
    char tmp[64]; // Doom's qsort items are always small (pointers, ints)
    if (size > sizeof(tmp)) return; // safety

    for (size_t gap = nmemb / 2; gap > 0; gap /= 2) {
        for (size_t i = gap; i < nmemb; i++) {
            memcpy(tmp, arr + i * size, size);
            size_t j = i;
            while (j >= gap && compar(arr + (j - gap) * size, tmp) > 0) {
                memcpy(arr + j * size, arr + (j - gap) * size, size);
                j -= gap;
            }
            memcpy(arr + j * size, tmp, size);
        }
    }
}

char *getenv(const char *name) {
    if (!name) return (char *)0;
    // DOOMWADDIR: id DOOM uses this to find WAD files
    if (strcmp(name, "DOOMWADDIR") == 0) return ".";
    // HOME: Doom uses this for config file paths
    if (strcmp(name, "HOME") == 0) return "/tmp";
    return (char *)0;
}
int system(const char *command) { (void)command; return -1; }

static unsigned int rand_seed = 1;
int rand(void) {
    rand_seed = rand_seed * 1103515245 + 12345;
    return (int)((rand_seed >> 16) & 0x7FFF);
}
void srand(unsigned int seed) { rand_seed = seed; }

// ============================================================
// setjmp/longjmp — minimal RV32I implementation
// ============================================================

// Save/restore callee-saved registers for error recovery.
// Doom uses this in I_Error() for graceful error handling.
// On bare-metal MBC, I_Error() just halts.

// Global used by I_Error() in i_system.c
jmp_buf error_recovery;

int setjmp(jmp_buf env) {
    (void)env;
    return 0;
}

void longjmp(jmp_buf env, int val) {
    (void)env;
    (void)val;
    // Write marker so we know longjmp was called
    volatile char *m = DEBUG_MSG_ADDR + 240;
    m[0] = 'L'; m[1] = 'J'; m[2] = 'M'; m[3] = 'P';
    // Don't halt — the I_Error path will call exit() which halts
    return;
}

// ============================================================
// Misc stubs
// ============================================================

int isatty(int fd) { (void)fd; return 0; }
int fileno(void *stream) { (void)stream; return -1; }
int access(const char *path, int mode) {
    (void)mode;
    // WAD files are memory-mapped and always accessible.
    // id DOOM checks access("./doomu.wad", R_OK) to detect retail mode.
    // Return 0 for any .wad file to make WAD detection work.
    size_t plen = strlen(path);
    if (plen >= 4 && strcasecmp(path + plen - 4, ".wad") == 0) return 0;
    return -1;
}
int stat(const char *path, struct stat *buf) {
    // WAD files are memory-mapped; report their size so Doom can find them
    size_t plen = strlen(path);
    if (plen >= 4 && strcasecmp(path + plen - 4, ".wad") == 0) {
        if (buf) {
            memset(buf, 0, sizeof(struct stat));
            buf->st_size = WAD_MAX_SIZE;
            buf->st_mode = 0100644; // regular file
        }
        return 0;
    }
    return -1;
}
int mkdir(const char *path, unsigned int mode) { (void)path; (void)mode; return -1; }

// ============================================================
// POSIX fd-based file I/O — id DOOM uses open/read/lseek (not fopen)
// ============================================================
// w_wad.c opens WAD files with open(), reads with read(), seeks with lseek().
// We map fd numbers to the memory-mapped WAD region.

#define FD_WAD_START 3
#define MAX_FDS 8

struct mbc_fd {
    int in_use;
    const unsigned char *base;
    unsigned int size;
    unsigned int pos;
};

static struct mbc_fd fd_table[MAX_FDS];

int open(const char *path, int flags, ...) {
    (void)flags;
    // Check if this looks like a WAD file
    int is_wad = 0;
    size_t plen = strlen(path);
    if (plen >= 4) {
        const char *ext = path + plen - 4;
        if (strcasecmp(ext, ".wad") == 0) is_wad = 1;
    }
    if (!is_wad) return -1;

    // Find free fd slot
    for (int i = 0; i < MAX_FDS; i++) {
        if (!fd_table[i].in_use) {
            fd_table[i].in_use = 1;
            fd_table[i].base = (const unsigned char *)WAD_BASE;
            fd_table[i].size = WAD_MAX_SIZE;
            fd_table[i].pos = 0;
            return i + FD_WAD_START;
        }
    }
    return -1;
}

typedef int ssize_t_local;
typedef int off_t_local;

ssize_t_local read(int fd, void *buf, size_t count) {
    int idx = fd - FD_WAD_START;
    if (idx < 0 || idx >= MAX_FDS || !fd_table[idx].in_use) return -1;
    struct mbc_fd *f = &fd_table[idx];
    unsigned int avail = (f->pos < f->size) ? (f->size - f->pos) : 0;
    if (count > avail) count = avail;
    memcpy(buf, f->base + f->pos, count);
    f->pos += count;
    return (ssize_t_local)count;
}

ssize_t_local write(int fd, const void *buf, size_t count) {
    // No writable files on bare metal
    (void)fd; (void)buf; (void)count;
    return -1;
}

off_t_local lseek(int fd, off_t_local offset, int whence) {
    int idx = fd - FD_WAD_START;
    if (idx < 0 || idx >= MAX_FDS || !fd_table[idx].in_use) return -1;
    struct mbc_fd *f = &fd_table[idx];
    long newpos;
    switch (whence) {
    case 0: newpos = offset; break;              // SEEK_SET
    case 1: newpos = (long)f->pos + offset; break;  // SEEK_CUR
    case 2: newpos = (long)f->size + offset; break;  // SEEK_END
    default: return -1;
    }
    if (newpos < 0) return -1;
    f->pos = (unsigned int)newpos;
    return (off_t_local)f->pos;
}

int close(int fd) {
    int idx = fd - FD_WAD_START;
    if (idx >= 0 && idx < MAX_FDS) {
        fd_table[idx].in_use = 0;
        fd_table[idx].pos = 0;
    }
    return 0;
}

int fstat(int fd, struct stat *buf) {
    int idx = fd - FD_WAD_START;
    if (idx < 0 || idx >= MAX_FDS || !fd_table[idx].in_use) return -1;
    if (buf) {
        memset(buf, 0, sizeof(struct stat));
        buf->st_size = fd_table[idx].size;
        buf->st_mode = 0100644; // regular file
    }
    return 0;
}

long filelength(int fd) {
    int idx = fd - FD_WAD_START;
    if (idx < 0 || idx >= MAX_FDS || !fd_table[idx].in_use) return 0;
    return (long)fd_table[idx].size;
}

unsigned int sleep(unsigned int seconds) { (void)seconds; return 0; }
int usleep(unsigned int usec) { (void)usec; return 0; }
unsigned int getuid(void) { return 0; }
int getpid(void) { return 1; }
int gettimeofday(void *tv, void *tz) { (void)tv; (void)tz; return -1; }

sig_t signal(int sig, sig_t handler) { (void)sig; (void)handler; return (sig_t)0; }

time_t time(time_t *t) { if (t) *t = 0; return 0; }
clock_t clock(void) { return 0; }

// Math stubs — Doom uses fixed-point, these should rarely be hit
double floor(double x) { return (double)(int)x - (x < (double)(int)x ? 1.0 : 0.0); }
double ceil(double x) { return (double)(int)x + (x > (double)(int)x ? 1.0 : 0.0); }
double fabs(double x) { return x < 0 ? -x : x; }
double sqrt(double x) {
    // Newton's method
    if (x <= 0) return 0;
    double guess = x;
    for (int i = 0; i < 10; i++) guess = (guess + x / guess) * 0.5;
    return guess;
}
// pow() — needed only if i_sound.c's I_SetChannels is called (our stub skips it).
// Provide a minimal implementation just in case.
double pow(double base, double exp) {
    if (exp == 0.0) return 1.0;
    if (base == 0.0) return 0.0;
    // Integer exponent fast path (covers i_sound.c usage: steptable)
    int iexp = (int)exp;
    if ((double)iexp == exp && iexp >= 0) {
        double result = 1.0;
        for (int i = 0; i < iexp; i++) result = result * base;
        return result;
    }
    // Rough approximation for non-integer exponents
    return 1.0;
}

// GCC may generate calls to these for struct copies
void *__memcpy_chk(void *dest, const void *src, size_t n, size_t destlen) {
    (void)destlen;
    return memcpy(dest, src, n);
}

// setbuf / getchar / fscanf — needed by d_main.c and m_misc.c
void setbuf(FILE *stream, char *buf) { (void)stream; (void)buf; }
int getchar(void) { return EOF; }
int fscanf(FILE *stream, const char *fmt, ...) {
    (void)stream; (void)fmt;
    return 0;
}

// sndserver_filename — referenced by m_misc.c default table under SNDSERV
// We don't define SNDSERV but the variable still gets referenced from the
// defaults array in m_misc.c. Provide it to satisfy the linker.
char *sndserver_filename = "./sndserver";
