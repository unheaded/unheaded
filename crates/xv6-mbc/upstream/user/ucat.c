// SPDX-License-Identifier: GPL-3.0-or-later
//
// ucat.c — UNHEADED cat: our own `cat` (Track 2 Phase 2.2, ADR-081).
//
// Built AS `cat` (target/cat.mbc — the PROGRAM_TABLE basename sh
// resolves at exec time); the MIT xv6 cat.c stays vendored, unwired.
//
// Phase 2.2 gate: identical observable behavior — same output bytes,
// same diagnostics, same exit codes. The corpus locks every interesting
// read path through this program: direct blocks (cat README),
// single-indirect blocks (cat BIGFILE, >12 KiB) and the multi-component
// path walk (cat sub/NOTE).
//
// One FS block per read: the in-BPF read(5) path serves at most a block
// per call, so a larger buffer would not reduce the syscall count. The
// UPC write(16) handler completes short writes re-entrantly (0xD0C0
// cursor), so userland still checks `!= n` only as a defensive contract,
// same as the program this replaces.

#include "kernel/types.h"
#include "kernel/fcntl.h"
#include "user/user.h"

#define CHUNK 512

static char chunk[CHUNK];

// Stream fd to stdout until EOF. Returns 0 on success, -1 after printing
// a diagnostic — the exit decision stays in main.
static int
stream(int fd)
{
  int n;

  while((n = read(fd, chunk, CHUNK)) > 0){
    if(write(1, chunk, n) != n){
      fprintf(2, "cat: write error\n");
      return -1;
    }
  }
  if(n < 0){
    fprintf(2, "cat: read error\n");
    return -1;
  }
  return 0;
}

int
main(int argc, char *argv[])
{
  if(argc <= 1){
    if(stream(0) < 0)
      exit(1);
    exit(0);
  }

  for(int i = 1; i < argc; i++){
    int fd = open(argv[i], O_RDONLY);
    if(fd < 0){
      fprintf(2, "cat: cannot open %s\n", argv[i]);
      exit(1);
    }
    if(stream(fd) < 0)
      exit(1);
    close(fd);
  }
  exit(0);
}
