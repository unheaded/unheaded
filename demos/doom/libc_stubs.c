// SPDX-License-Identifier: GPL-3.0-or-later
// libc_stubs.c — Bare-metal libc for Doom on MBC (Monad Bytecode)
//
// Provides minimal C library functions for doomgeneric running in the UPC.
// WAD data is memory-mapped at WAD_BASE (0x10000) by the BPF map loader.
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

// Types needed before their headers
typedef struct _FILE FILE;
#define EOF (-1)
typedef long time_t;
typedef long clock_t;
typedef unsigned int jmp_buf[16];
typedef void (*sig_t)(int);
struct stat { int st_size; unsigned int st_mode; };

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

// Heap boundaries — hardcoded to match linker.ld HEAP region
// HEAP origin=0x1C0000, length=6MB → end=0x7C0000
#define HEAP_START_ADDR 0x001C0000
#define HEAP_END_ADDR   0x007C0000

static char *heap_ptr = (char *)HEAP_START_ADDR;

int errno;

void *malloc(size_t size) {
    // Align to 8 bytes
    size = (size + 7) & ~7;
    char *result = heap_ptr;
    char *new_ptr = heap_ptr + size;

    if (new_ptr > (char *)HEAP_END_ADDR || new_ptr < heap_ptr) {
        // Out of memory
        return (void *)0;
    }
    heap_ptr = new_ptr;
    return (void *)result;
}

void *calloc(size_t nmemb, size_t size) {
    size_t total = nmemb * size;
    void *p = malloc(total);
    if (p) memset(p, 0, total);
    return p;
}

void *realloc(void *ptr, size_t size) {
    // Bump allocator can't realloc — just allocate new
    void *newp = malloc(size);
    if (newp && ptr) {
        // Copy old data (conservatively assume old size <= new size)
        memcpy(newp, ptr, size);
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

static int debug_msg_written = 0;
static void debug_write_string(const char *s) {
    // Only capture the first error message (skip the "\n" that follows)
    if (debug_msg_written) return;
    if (s[0] == '\n' && s[1] == '\0') return; // skip bare newlines
    debug_msg_written = 1;
    volatile char *dst = DEBUG_MSG_ADDR;
    int i = 0;
    while (s[i] && i < DEBUG_MSG_MAX - 1) {
        dst[i] = s[i];
        i++;
    }
    dst[i] = '\0';
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
        char buf[DEBUG_MSG_MAX];
        vsnprintf(buf, sizeof(buf), fmt, ap);
        debug_write_string(buf);
    }
    return 0;
}
int putchar(int c) { (void)c; return c; }
int puts(const char *s) { (void)s; return 0; }
int fputs(const char *s, FILE *stream) { (void)s; (void)stream; return 0; }
int fputc(int c, FILE *stream) { (void)c; (void)stream; return c; }
int fgetc(FILE *stream) { (void)stream; return EOF; }
char *fgets(char *s, int size, FILE *stream) { (void)s; (void)size; (void)stream; return (char *)0; }
int sscanf(const char *str, const char *fmt, ...) { (void)str; (void)fmt; return 0; }

// ============================================================
// File I/O — Memory-mapped WAD access
// ============================================================
// The WAD file is loaded into MBC memory at WAD_BASE by the doom-loader.
// We emulate fopen/fread/fseek to read from this memory region.

#define WAD_BASE     0x00800000
#define WAD_MAX_SIZE 4196020  // doom1.wad exact size (4,196,020 bytes)

// We support at most 4 open "files" (Doom typically opens 1 WAD)
#define MAX_FILES 4

struct mbc_file {
    int in_use;
    const uint8_t *base;
    size_t size;
    size_t pos;
    int is_wad;
};

static struct mbc_file file_table[MAX_FILES];

// WAD size is stored at the start of the WAD header (bytes 4-7 = numlumps, 8-11 = infotableofs)
// We use WAD_MAX_SIZE as the reported size; the actual WAD will be smaller.
static size_t wad_file_size = WAD_MAX_SIZE;

FILE *fopen(const char *path, const char *mode) {
    (void)mode;

    // Check if this looks like a WAD file
    int is_wad = 0;
    size_t plen = strlen(path);
    if (plen >= 4) {
        const char *ext = path + plen - 4;
        if (strcasecmp(ext, ".wad") == 0) is_wad = 1;
    }

    if (!is_wad) {
        // Only WAD files are accessible on bare-metal
        return (FILE *)0;
    }

    // Find free slot
    for (int i = 0; i < MAX_FILES; i++) {
        if (!file_table[i].in_use) {
            file_table[i].in_use = 1;
            file_table[i].base = (const uint8_t *)WAD_BASE;
            file_table[i].size = wad_file_size;
            file_table[i].pos = 0;
            file_table[i].is_wad = 1;
            return (FILE *)(uintptr_t)(i + 100); // offset to avoid NULL/stdin/stdout/stderr
        }
    }
    return (FILE *)0;
}

static struct mbc_file *get_file(FILE *stream) {
    int idx = (int)(uintptr_t)stream - 100;
    if (idx < 0 || idx >= MAX_FILES) return (struct mbc_file *)0;
    if (!file_table[idx].in_use) return (struct mbc_file *)0;
    return &file_table[idx];
}

int fclose(FILE *stream) {
    struct mbc_file *f = get_file(stream);
    if (!f) return EOF;
    f->in_use = 0;
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
    (void)status;
    mbc_halt();
    while (1) {} // should never reach
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

char *getenv(const char *name) { (void)name; return (char *)0; }
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
    // On bare-metal MBC, longjmp means I_Error — halt the CPU.
    mbc_halt();
    while (1) {}
}

// ============================================================
// Misc stubs
// ============================================================

int isatty(int fd) { (void)fd; return 0; }
int fileno(void *stream) { (void)stream; return -1; }
int access(const char *path, int mode) { (void)path; (void)mode; return -1; }
int stat(const char *path, struct stat *buf) { (void)path; (void)buf; return -1; }
int mkdir(const char *path, unsigned int mode) { (void)path; (void)mode; return -1; }
int open(const char *path, int flags, ...) { (void)path; (void)flags; return -1; }

sig_t signal(int sig, sig_t handler) { (void)sig; (void)handler; return (sig_t)0; }

time_t time(time_t *t) { if (t) *t = 0; return 0; }
clock_t clock(void) { return 0; }

// Math stubs — Doom uses fixed-point, these should rarely be hit
double floor(double x) { return (double)(int)x - (x < (double)(int)x ? 1.0 : 0.0); }
double ceil(double x) { return (double)(int)x + (x > (double)(int)x ? 1.0 : 0.0); }
double fabs(double x) { return x < 0 ? -x : x; }
double sqrt(double x) {
    // Newton's method, ~5 iterations
    if (x <= 0) return 0;
    double guess = x;
    for (int i = 0; i < 10; i++) guess = (guess + x / guess) * 0.5;
    return guess;
}

// GCC may generate calls to these for struct copies
void *__memcpy_chk(void *dest, const void *src, size_t n, size_t destlen) {
    (void)destlen;
    return memcpy(dest, src, n);
}
