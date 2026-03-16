// SPDX-License-Identifier: GPL-3.0-or-later
#ifndef _SYS_STAT_H
#define _SYS_STAT_H

#include <stddef.h>
#include <sys/types.h>

typedef unsigned int mode_t;
typedef int off_t;

struct stat {
    off_t st_size;
    mode_t st_mode;
};

#define S_ISDIR(m) (0)
#define S_ISREG(m) (1)

int stat(const char *path, struct stat *buf);
int mkdir(const char *path, mode_t mode);

#endif
