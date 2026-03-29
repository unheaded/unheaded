// SPDX-License-Identifier: GPL-3.0-or-later
#ifndef _SYS_TIME_H
#define _SYS_TIME_H

struct timeval {
    long tv_sec;
    long tv_usec;
};

struct timezone {
    int tz_minuteswest;
    int tz_dsttime;
};

int gettimeofday(struct timeval *tv, struct timezone *tz);

#endif
