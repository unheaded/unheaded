// SPDX-License-Identifier: GPL-3.0-or-later
//
// uwc.c — UNHEADED wc: our own `wc` (Track 2 Phase 2.2, ADR-081).
//
// Built AS `wc` (target/wc.mbc — the PROGRAM_TABLE basename sh resolves
// at exec time); the MIT xv6 wc.c stays vendored, unwired.
//
// Phase 2.2 gate: identical observable behavior. Two contracts the
// original encodes subtly, preserved here explicitly:
//   * word separators are exactly { space \t \r \n \v }. The original
//     tests strchr(" \r\t\n\v", c), and xv6's strchr never matches the
//     terminator — so NUL bytes count as WORD characters ('\f' too).
//     is_sep() spells that set out instead of re-deriving it from
//     strchr's quirk.
//   * the no-argument (stdin) form reports the empty name, leaving a
//     trailing space before the newline — same format string, same
//     bytes out.

#include "kernel/types.h"
#include "kernel/fcntl.h"
#include "user/user.h"

#define CHUNK 512

static char chunk[CHUNK];

static int
is_sep(char c)
{
  return c == ' ' || c == '\t' || c == '\r' || c == '\n' || c == '\v';
}

// Count lines / words / bytes on fd and print the one summary line.
static void
count(int fd, char *name)
{
  int lines = 0, words = 0, bytes = 0;
  int inword = 0;
  int n;

  while((n = read(fd, chunk, CHUNK)) > 0){
    for(int i = 0; i < n; i++){
      bytes++;
      if(chunk[i] == '\n')
        lines++;
      if(is_sep(chunk[i]))
        inword = 0;
      else if(!inword){
        words++;
        inword = 1;
      }
    }
  }
  if(n < 0){
    printf("wc: read error\n");
    exit(1);
  }
  printf("%d %d %d %s\n", lines, words, bytes, name);
}

int
main(int argc, char *argv[])
{
  if(argc <= 1){
    count(0, "");
    exit(0);
  }

  for(int i = 1; i < argc; i++){
    int fd = open(argv[i], O_RDONLY);
    if(fd < 0){
      printf("wc: cannot open %s\n", argv[i]);
      exit(1);
    }
    count(fd, argv[i]);
    close(fd);
  }
  exit(0);
}
