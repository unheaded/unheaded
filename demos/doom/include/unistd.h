// SPDX-License-Identifier: GPL-3.0-or-later
#ifndef _UNISTD_H
#define _UNISTD_H

#include <stddef.h>

int isatty(int fd);
int fileno(void *stream);
int access(const char *path, int mode);
int mkdir(const char *path, unsigned int mode);

#define R_OK 4
#define W_OK 2
#define X_OK 1
#define F_OK 0

#endif
