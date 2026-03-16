// SPDX-License-Identifier: GPL-3.0-or-later
// Minimal stdlib.h for bare-metal Doom on MBC
#ifndef _STDLIB_H
#define _STDLIB_H

#include <stddef.h>

#define EXIT_SUCCESS 0
#define EXIT_FAILURE 1

void *malloc(size_t size);
void *calloc(size_t nmemb, size_t size);
void *realloc(void *ptr, size_t size);
void free(void *ptr);

int atoi(const char *s);
long atol(const char *s);
double atof(const char *s);
int abs(int x);

void exit(int status);
void abort(void);

void qsort(void *base, size_t nmemb, size_t size,
           int (*compar)(const void *, const void *));

char *getenv(const char *name);

int system(const char *command);

#define RAND_MAX 0x7FFFFFFF
int rand(void);
void srand(unsigned int seed);

#endif
