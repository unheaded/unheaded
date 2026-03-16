// SPDX-License-Identifier: GPL-3.0-or-later
#ifndef _TIME_H
#define _TIME_H

#include <stddef.h>

typedef long time_t;
typedef long clock_t;

struct timespec {
    time_t tv_sec;
    long tv_nsec;
};

time_t time(time_t *t);
clock_t clock(void);

#define CLOCKS_PER_SEC 1000000

#endif
