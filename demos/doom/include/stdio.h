// SPDX-License-Identifier: GPL-3.0-or-later
// Minimal stdio.h for bare-metal Doom on MBC
#ifndef _STDIO_H
#define _STDIO_H

#include <stddef.h>
#include <stdarg.h>

#define EOF (-1)
#define SEEK_SET 0
#define SEEK_CUR 1
#define SEEK_END 2

#define FILENAME_MAX 256

typedef struct _FILE FILE;
extern FILE *stdin;
extern FILE *stdout;
extern FILE *stderr;

FILE *fopen(const char *path, const char *mode);
int fclose(FILE *stream);
size_t fread(void *ptr, size_t size, size_t nmemb, FILE *stream);
size_t fwrite(const void *ptr, size_t size, size_t nmemb, FILE *stream);
int fseek(FILE *stream, long offset, int whence);
long ftell(FILE *stream);
int feof(FILE *stream);
int ferror(FILE *stream);
void rewind(FILE *stream);

int printf(const char *fmt, ...);
int fprintf(FILE *stream, const char *fmt, ...);
int sprintf(char *str, const char *fmt, ...);
int snprintf(char *str, size_t size, const char *fmt, ...);
int vprintf(const char *fmt, va_list ap);
int vfprintf(FILE *stream, const char *fmt, va_list ap);
int vsprintf(char *str, const char *fmt, va_list ap);
int vsnprintf(char *str, size_t size, const char *fmt, va_list ap);
int sscanf(const char *str, const char *fmt, ...);

int putchar(int c);
int puts(const char *s);
int fputs(const char *s, FILE *stream);
int fputc(int c, FILE *stream);
int fgetc(FILE *stream);
char *fgets(char *s, int size, FILE *stream);

int rename(const char *oldpath, const char *newpath);
int remove(const char *pathname);

FILE *tmpfile(void);
int fflush(FILE *stream);

#endif
