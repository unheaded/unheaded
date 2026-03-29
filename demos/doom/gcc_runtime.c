// SPDX-License-Identifier: GPL-3.0-or-later
// gcc_runtime.c — Software div/mul/float for RV32I restricted to x0-x15
//
// Replaces libgcc functions that use x16-x31 registers.
// These are compiled with our -ffixed-x16..-ffixed-x31 constraints.

#include <stdint.h>

// ============================================================
// 32-bit integer multiply
// ============================================================

// GCC calls __mulsi3 when RV32I has no M extension
int __mulsi3(int a, int b) {
    unsigned int ua = (unsigned int)a;
    unsigned int ub = (unsigned int)b;
    unsigned int result = 0;
    while (ub) {
        if (ub & 1) result += ua;
        ua <<= 1;
        ub >>= 1;
    }
    return (int)result;
}

// ============================================================
// 32-bit signed division and modulo
// ============================================================

static unsigned int udivmod(unsigned int num, unsigned int den,
                            unsigned int *rem) {
    unsigned int quot = 0;
    unsigned int bit = 1;

    if (den == 0) {
        // Division by zero — return 0
        if (rem) *rem = 0;
        return 0;
    }

    // Shift denominator left until it's >= numerator
    while (den <= num && !(den & (1u << 31))) {
        den <<= 1;
        bit <<= 1;
    }

    while (bit) {
        if (num >= den) {
            num -= den;
            quot |= bit;
        }
        den >>= 1;
        bit >>= 1;
    }

    if (rem) *rem = num;
    return quot;
}

int __divsi3(int a, int b) {
    int neg = 0;
    if (a < 0) { a = -a; neg = !neg; }
    if (b < 0) { b = -b; neg = !neg; }
    int q = (int)udivmod((unsigned int)a, (unsigned int)b, 0);
    return neg ? -q : q;
}

int __modsi3(int a, int b) {
    int neg = 0;
    if (a < 0) { a = -a; neg = 1; }
    if (b < 0) { b = -b; }
    unsigned int rem;
    udivmod((unsigned int)a, (unsigned int)b, &rem);
    return neg ? -(int)rem : (int)rem;
}

unsigned int __udivsi3(unsigned int a, unsigned int b) {
    return udivmod(a, b, 0);
}

unsigned int __umodsi3(unsigned int a, unsigned int b) {
    unsigned int rem;
    udivmod(a, b, &rem);
    return rem;
}

// 64-bit division support (GCC may call these for long long)
typedef unsigned long long uint64;
typedef long long int64;

// 64-bit multiply
uint64 __muldi3(uint64 a, uint64 b) {
    uint64 result = 0;
    while (b) {
        if (b & 1) result += a;
        a <<= 1;
        b >>= 1;
    }
    return result;
}

uint64 __udivdi3(uint64 a, uint64 b) {
    if (b == 0) return 0;
    uint64 quot = 0, bit = 1;
    while (b <= a && !(b & ((uint64)1 << 63))) { b <<= 1; bit <<= 1; }
    while (bit) {
        if (a >= b) { a -= b; quot |= bit; }
        b >>= 1; bit >>= 1;
    }
    return quot;
}

int64 __divdi3(int64 a, int64 b) {
    int neg = 0;
    if (a < 0) { a = -a; neg = !neg; }
    if (b < 0) { b = -b; neg = !neg; }
    int64 q = (int64)__udivdi3((uint64)a, (uint64)b);
    return neg ? -q : q;
}

uint64 __umoddi3(uint64 a, uint64 b) {
    return a - __udivdi3(a, b) * b;
}

int64 __moddi3(int64 a, int64 b) {
    int neg = (a < 0);
    if (a < 0) a = -a;
    if (b < 0) b = -b;
    int64 r = (int64)__umoddi3((uint64)a, (uint64)b);
    return neg ? -r : r;
}

// ============================================================
// Software floating point (minimal — Doom uses fixed-point mostly)
// ============================================================

// IEEE 754 single-precision helpers
// These are needed by v_video.c (i_scale.c uses float for scaling)

typedef union {
    float f;
    uint32_t u;
} float_bits;

// Float to int conversion (truncate toward zero)
int __fixsfsi(float a) {
    float_bits fb;
    fb.f = a;
    uint32_t bits = fb.u;

    int sign = (bits >> 31) & 1;
    int exp = ((bits >> 23) & 0xFF) - 127;
    uint32_t mant = (bits & 0x7FFFFF) | 0x800000; // implicit 1

    if (exp < 0) return 0;
    if (exp >= 31) return sign ? (int)0x80000000 : 0x7FFFFFFF;

    int result;
    if (exp >= 23) {
        result = (int)(mant << (exp - 23));
    } else {
        result = (int)(mant >> (23 - exp));
    }
    return sign ? -result : result;
}

// Int to float conversion
float __floatsisf(int a) {
    if (a == 0) return 0.0f;

    int sign = 0;
    unsigned int ua;
    if (a < 0) { sign = 1; ua = (unsigned int)(-a); } else { ua = (unsigned int)a; }

    int exp = 23 + 127;
    // Normalize
    while (ua >= (1u << 24)) { ua >>= 1; exp++; }
    while (ua < (1u << 23)) { ua <<= 1; exp--; }

    uint32_t bits = ((uint32_t)sign << 31) | ((uint32_t)exp << 23) | (ua & 0x7FFFFF);
    float_bits fb;
    fb.u = bits;
    return fb.f;
}

// Unsigned int to float
float __floatunsisf(unsigned int a) {
    if (a == 0) return 0.0f;
    int exp = 23 + 127;
    unsigned int ua = a;
    while (ua >= (1u << 24)) { ua >>= 1; exp++; }
    while (ua < (1u << 23)) { ua <<= 1; exp--; }
    float_bits fb;
    fb.u = ((uint32_t)exp << 23) | (ua & 0x7FFFFF);
    return fb.f;
}

// Float division
float __divsf3(float a, float b) {
    float_bits fa, fb;
    fa.f = a; fb.f = b;

    int sign = ((fa.u >> 31) ^ (fb.u >> 31)) & 1;
    int ea = ((fa.u >> 23) & 0xFF) - 127;
    int eb = ((fb.u >> 23) & 0xFF) - 127;
    uint32_t ma = (fa.u & 0x7FFFFF) | 0x800000;
    uint32_t mb = (fb.u & 0x7FFFFF) | 0x800000;

    // Handle special cases
    if (eb == -127 && (fb.u & 0x7FFFFF) == 0) return 0.0f; // div by zero -> 0

    // Long division: ma/mb with 24 bits of precision
    uint32_t result = 0;
    for (int i = 23; i >= 0; i--) {
        if (ma >= mb) {
            ma -= mb;
            result |= (1u << i);
        }
        ma <<= 1;
    }

    int exp = ea - eb + 127;
    // Normalize
    while (result >= (1u << 24)) { result >>= 1; exp++; }
    while (result && result < (1u << 23)) { result <<= 1; exp--; }

    if (exp <= 0) return 0.0f;
    if (exp >= 255) { float_bits r; r.u = ((uint32_t)sign << 31) | 0x7F800000; return r.f; }

    float_bits r;
    r.u = ((uint32_t)sign << 31) | ((uint32_t)exp << 23) | (result & 0x7FFFFF);
    return r.f;
}

// Float multiply
float __mulsf3(float a, float b) {
    float_bits fa, fb;
    fa.f = a; fb.f = b;

    int sign = ((fa.u >> 31) ^ (fb.u >> 31)) & 1;
    int ea = ((fa.u >> 23) & 0xFF) - 127;
    int eb = ((fb.u >> 23) & 0xFF) - 127;

    if ((fa.u & 0x7FFFFFFF) == 0 || (fb.u & 0x7FFFFFFF) == 0) return 0.0f;

    uint32_t ma = (fa.u & 0x7FFFFF) | 0x800000;
    uint32_t mb = (fb.u & 0x7FFFFF) | 0x800000;

    // Multiply mantissas (split to avoid overflow)
    uint64 product = (uint64)ma * (uint64)mb;
    uint32_t result = (uint32_t)(product >> 23);
    int exp = ea + eb + 127;

    while (result >= (1u << 24)) { result >>= 1; exp++; }
    while (result && result < (1u << 23)) { result <<= 1; exp--; }

    if (exp <= 0) return 0.0f;
    if (exp >= 255) { float_bits r; r.u = ((uint32_t)sign << 31) | 0x7F800000; return r.f; }

    float_bits r;
    r.u = ((uint32_t)sign << 31) | ((uint32_t)exp << 23) | (result & 0x7FFFFF);
    return r.f;
}

// Float subtraction
float __subsf3(float a, float b) {
    float_bits fb_bits;
    fb_bits.f = b;
    fb_bits.u ^= (1u << 31); // negate b
    return a + fb_bits.f; // use hardware add if available, or __addsf3
}

// Float addition
float __addsf3(float a, float b) {
    float_bits fa, fb;
    fa.f = a; fb.f = b;

    // Handle zeros
    if ((fa.u & 0x7FFFFFFF) == 0) return b;
    if ((fb.u & 0x7FFFFFFF) == 0) return a;

    // Ensure |a| >= |b|
    if ((fa.u & 0x7FFFFFFF) < (fb.u & 0x7FFFFFFF)) {
        float_bits tmp = fa; fa = fb; fb = tmp;
    }

    int sa = (fa.u >> 31) & 1;
    int sb = (fb.u >> 31) & 1;
    int ea = ((fa.u >> 23) & 0xFF);
    int eb = ((fb.u >> 23) & 0xFF);
    uint32_t ma = (fa.u & 0x7FFFFF) | 0x800000;
    uint32_t mb = (fb.u & 0x7FFFFF) | 0x800000;

    int shift = ea - eb;
    if (shift > 24) return fa.f; // b too small to matter
    mb >>= shift;

    uint32_t mr;
    int sr = sa;
    if (sa == sb) {
        mr = ma + mb;
    } else {
        mr = ma - mb;
        if (mr == 0) return 0.0f;
    }

    int er = ea;
    while (mr >= (1u << 24)) { mr >>= 1; er++; }
    while (mr && mr < (1u << 23)) { mr <<= 1; er--; }

    if (er <= 0) return 0.0f;
    if (er >= 255) { float_bits r; r.u = ((uint32_t)sr << 31) | 0x7F800000; return r.f; }

    float_bits r;
    r.u = ((uint32_t)sr << 31) | ((uint32_t)er << 23) | (mr & 0x7FFFFF);
    return r.f;
}

// Float comparison (unordered)
int __unordsf2(float a, float b) {
    float_bits fa, fb;
    fa.f = a; fb.f = b;
    // Check for NaN
    int a_nan = ((fa.u & 0x7F800000) == 0x7F800000) && (fa.u & 0x7FFFFF);
    int b_nan = ((fb.u & 0x7F800000) == 0x7F800000) && (fb.u & 0x7FFFFF);
    return a_nan || b_nan;
}

// Float comparisons (return 0 if equal, -1 if a<b, 1 if a>b)
int __lesf2(float a, float b) { return (a < b) ? -1 : (a > b) ? 1 : 0; }
int __gesf2(float a, float b) { return (a < b) ? -1 : (a > b) ? 1 : 0; }
int __eqsf2(float a, float b) { return (a == b) ? 0 : 1; }
int __nesf2(float a, float b) { return (a != b) ? 1 : 0; }
int __gtsf2(float a, float b) { return (a > b) ? 1 : (a < b) ? -1 : 0; }
int __ltsf2(float a, float b) { return (a < b) ? -1 : (a > b) ? 1 : 0; }

// Float to unsigned int
unsigned int __fixunssfsi(float a) {
    if (a <= 0.0f) return 0;
    return (unsigned int)__fixsfsi(a);
}

// Double stubs (Doom barely uses doubles, but libgcc may need these)
double __adddf3(double a, double b) { return (double)__addsf3((float)a, (float)b); }
double __subdf3(double a, double b) { return (double)__subsf3((float)a, (float)b); }
double __muldf3(double a, double b) { return (double)__mulsf3((float)a, (float)b); }
double __divdf3(double a, double b) { return (double)__divsf3((float)a, (float)b); }
int __fixdfsi(double a) { return __fixsfsi((float)a); }
double __floatsidf(int a) { return (double)__floatsisf(a); }
double __floatunsidf(unsigned int a) { return (double)__floatunsisf(a); }
// __truncdfsf2: convert double to float. Bit manipulation, no compiler cast.
float __truncdfsf2(double a) {
    union { double d; uint64_t u; } di;
    union { float f; uint32_t u; } fi;
    di.d = a;
    uint32_t sign = (uint32_t)(di.u >> 63) & 1;
    uint32_t d_exp = (uint32_t)((di.u >> 52) & 0x7FF);
    uint32_t d_mant = (uint32_t)((di.u >> 29) & 0x7FFFFF);
    uint32_t f_sign = sign << 31;
    if (d_exp == 0x7FF) {
        // Inf or NaN
        fi.u = f_sign | (0xFF << 23) | d_mant;
    } else if (d_exp == 0) {
        fi.u = f_sign; // zero
    } else {
        int f_exp = (int)d_exp - 1023 + 127;
        if (f_exp >= 255) fi.u = f_sign | (0xFF << 23); // overflow → inf
        else if (f_exp <= 0) fi.u = f_sign; // underflow → zero
        else fi.u = f_sign | ((uint32_t)f_exp << 23) | d_mant;
    }
    return fi.f;
}
// __extendsfdf2: convert float (IEEE 754 single) to double (IEEE 754 double).
// CANNOT use (double)a — compiler generates recursive call to this function.
// Bit-manipulate: extract sign/exp/mantissa, repack as double.
double __extendsfdf2(float a) {
    union { float f; uint32_t u; } fi;
    union { double d; uint64_t u; } di;
    fi.f = a;
    uint32_t sign = (fi.u >> 31) & 1;
    uint32_t exp = (fi.u >> 23) & 0xFF;
    uint32_t mant = fi.u & 0x7FFFFF;
    uint64_t d_sign = (uint64_t)sign << 63;
    uint64_t d_exp, d_mant;
    if (exp == 0 && mant == 0) {
        // Zero
        di.u = d_sign;
    } else if (exp == 0xFF) {
        // Inf or NaN
        d_exp = (uint64_t)0x7FF << 52;
        d_mant = (uint64_t)mant << 29;
        di.u = d_sign | d_exp | d_mant;
    } else if (exp == 0) {
        // Denormal — treat as zero for Doom (good enough)
        di.u = d_sign;
    } else {
        // Normal: rebias exponent (float bias=127, double bias=1023)
        d_exp = (uint64_t)(exp - 127 + 1023) << 52;
        d_mant = (uint64_t)mant << 29;
        di.u = d_sign | d_exp | d_mant;
    }
    return di.d;
}
int __ledf2(double a, double b) { return __lesf2((float)a, (float)b); }
int __gedf2(double a, double b) { return __gesf2((float)a, (float)b); }
int __eqdf2(double a, double b) { return __eqsf2((float)a, (float)b); }
int __ltdf2(double a, double b) { return __ltsf2((float)a, (float)b); }
int __gtdf2(double a, double b) { return __gtsf2((float)a, (float)b); }
unsigned int __fixunsdfsi(double a) { return __fixunssfsi((float)a); }
