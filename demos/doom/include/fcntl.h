// SPDX-License-Identifier: GPL-3.0-or-later
#ifndef _FCNTL_H
#define _FCNTL_H

#define O_RDONLY 0
#define O_WRONLY 1
#define O_RDWR   2
#define O_CREAT  0100

int open(const char *path, int flags, ...);

#endif
