// SPDX-License-Identifier: GPL-3.0-or-later
#ifndef _FCNTL_H
#define _FCNTL_H

#define O_RDONLY  0
#define O_WRONLY  1
#define O_RDWR    2
#define O_CREAT   0100
#define O_TRUNC   01000
#define O_BINARY  0       /* no-op on bare metal */

int open(const char *path, int flags, ...);

#endif
