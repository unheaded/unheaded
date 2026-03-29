// SPDX-License-Identifier: GPL-3.0-or-later
#ifndef _UNISTD_H
#define _UNISTD_H

#include <stddef.h>

typedef int ssize_t;
typedef int off_t;
typedef int pid_t;
typedef unsigned int uid_t;

int isatty(int fd);
int fileno(void *stream);
int access(const char *path, int mode);
int mkdir(const char *path, unsigned int mode);

ssize_t read(int fd, void *buf, size_t count);
ssize_t write(int fd, const void *buf, size_t count);
int close(int fd);
off_t lseek(int fd, off_t offset, int whence);

unsigned int sleep(unsigned int seconds);
int usleep(unsigned int usec);

uid_t getuid(void);
pid_t getpid(void);

#define R_OK 4
#define W_OK 2
#define X_OK 1
#define F_OK 0

#define SEEK_SET 0
#define SEEK_CUR 1
#define SEEK_END 2

#endif
