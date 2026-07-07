// SPDX-License-Identifier: GPL-3.0-or-later
//
// ustring.c — UNHEADED kernel memory/string primitives.
// Track 2 Phase 2.4 (ADR-081 "evolve from xv6, never break the boot").
//
// Replaces MIT kernel/string.c in the Makefile.mbc OBJS list; the MIT file
// stays vendored, unwired.
//
// Boot-hot everywhere: every kernel printf byte, every kalloc junk-fill,
// every path/name copy transits these eight functions. The phase22 goldens
// gate them byte-for-byte.
//
// Load-bearing contracts kept behavior-identical:
//   - memmove's overlap test (s < d && s + n > d) selects the backward copy;
//     memcpy stays an alias of memmove ("to placate GCC").
//   - strncpy's double-decrement: the first loop burns one n on the NUL it
//     copies, the second pads the remainder — pad-to-n semantics, not
//     NUL-guarantee (that is safestrcpy's job).
//   - Comparisons widen through uchar so bytes >= 0x80 order unsigned.
//   - Widths are ABI: uint n for mem*, int n for strn*/safestrcpy/strlen.

#include "types.h"

void*
memset(void *dst, int c, uint n)
{
  char *cdst = (char *) dst;
  int i;
  for(i = 0; i < n; i++){
    cdst[i] = c;
  }
  return dst;
}

int
memcmp(const void *v1, const void *v2, uint n)
{
  const uchar *s1, *s2;

  s1 = v1;
  s2 = v2;
  while(n-- > 0){
    if(*s1 != *s2)
      return *s1 - *s2;
    s1++, s2++;
  }

  return 0;
}

void*
memmove(void *dst, const void *src, uint n)
{
  const char *s;
  char *d;

  if(n == 0)
    return dst;

  s = src;
  d = dst;
  if(s < d && s + n > d){
    s += n;
    d += n;
    while(n-- > 0)
      *--d = *--s;
  } else
    while(n-- > 0)
      *d++ = *s++;

  return dst;
}

// memcpy exists to placate GCC.  Use memmove.
void*
memcpy(void *dst, const void *src, uint n)
{
  return memmove(dst, src, n);
}

int
strncmp(const char *p, const char *q, uint n)
{
  while(n > 0 && *p && *p == *q)
    n--, p++, q++;
  if(n == 0)
    return 0;
  return (uchar)*p - (uchar)*q;
}

char*
strncpy(char *s, const char *t, int n)
{
  char *os;

  os = s;
  while(n-- > 0 && (*s++ = *t++) != 0)
    ;
  while(n-- > 0)
    *s++ = 0;
  return os;
}

// Like strncpy but guaranteed to NUL-terminate.
char*
safestrcpy(char *s, const char *t, int n)
{
  char *os;

  os = s;
  if(n <= 0)
    return os;
  while(--n > 0 && (*s++ = *t++) != 0)
    ;
  *s = 0;
  return os;
}

int
strlen(const char *s)
{
  int n;

  for(n = 0; s[n]; n++)
    ;
  return n;
}
