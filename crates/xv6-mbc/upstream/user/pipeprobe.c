// SPDX-License-Identifier: GPL-3.0-or-later
// pipeprobe.c: S-STAGE-2B pipe rung. A blocked end retries (ADR-135 D7), a full pipe
// writes short, and EOF arrives through exit's close (D13). One process cannot see
// another's memory, so a directory made just before a step marks that step as reached.
#include "kernel/types.h"
#include "kernel/stat.h"
#include "kernel/fcntl.h"
#include "user/user.h"
#include "user/uyield.h"

#define SAY(t, c) do { if(c) printf(t "=yes\n"); else printf(t "=no\n"); } while(0)

#define N 2000         // more than PIPESIZE: the writer has to block and go on
#define PIPESIZE 512
#define CHUNK 96       // does not divide PIPESIZE, so reads cross the ring's end
#define BAD ((int *)0x00800000)      // the user ceiling: nothing can be written
#define STRADDLE ((int *)0x007FFFFC) // first word lands, second is refused

static char out[N];
static char in[CHUNK];

// Both terms count: a slipped byte moves the first, a lost 256 the second.
static int
pat(int i)
{
  return (i + (i >> 8)) & 0xFF;
}

static int
seen(char *path)
{
  int fd = open(path, O_RDONLY);
  if(fd >= 0)
    close(fd);
  return fd >= 0;
}

static void
turns(void)
{
  for(int i = 0; i < 4; i++)
    yield();
}

// The child never closes p[1] by hand, and the parent closed its own, so only the
// child's exit can end the stream. The child's pause leaves the reader on an empty
// pipe with the write end still open, and piw appears after that pause.
static void
bulk(void)
{
  int p[2], total = 0, same = 1, n;

  SAY("PI: pipe", pipe(p) == 0);
  SAY("PI: fds", p[0] >= 16 && p[1] >= 16 && p[0] != p[1]);
  for(int i = 0; i < N; i++)
    out[i] = pat(i);
  int c = fork();
  if(c == 0){
    int off = 0, shorts = 0;
    close(p[0]);
    while(off < N){
      n = write(p[1], out + off, N - off);
      if(n <= 0)
        break;
      if(n < N - off)
        shorts++;
      off += n;
    }
    SAY("PI: wrote-all", off == N);
    SAY("PI: short-writes", shorts > 0);
    pause(200);
    mkdir("piw");
    exit(0);
  }
  close(p[1]);
  while((n = read(p[0], in, CHUNK)) > 0){
    for(int i = 0; i < n; i++)
      if((in[i] & 0xFF) != pat(total + i))
        same = 0;
    total += n;
  }
  SAY("PI: all-bytes", total == N);
  SAY("PI: every-byte", same);
  SAY("PI: eof-zero", n == 0);
  SAY("PI: eof-after-exit", seen("piw"));
  SAY("PI: eof-sticks", read(p[0], in, 1) == 0);
  SAY("PI: reaped", wait(0) == c);
  close(p[0]);
}

// A parent that spun would keep the slot and the sibling would never reach mkdir;
// one that fell through would read something other than 1.
static void
blocks(void)
{
  int p[2];
  char b = 0;

  pipe(p);
  int c = fork();
  if(c == 0){
    close(p[0]);
    turns();
    mkdir("pib");
    write(p[1], "x", 1);
    exit(0);
  }
  close(p[1]);
  SAY("PI: blocked-read", read(p[0], &b, 1) == 1 && b == 'x');
  SAY("PI: sibling-first", seen("pib"));
  wait(0);
  close(p[0]);
}

static void
noreader(void)
{
  int p[2];

  pipe(p);
  close(p[0]);
  SAY("PI: no-reader", write(p[1], "x", 1) == -1);
  close(p[1]);

  // A writer already blocked on a full pipe hears the reader go.
  pipe(p);
  if(fork() == 0){
    close(p[0]);
    if(write(p[1], out, PIPESIZE) == PIPESIZE && write(p[1], out, 1) == -1)
      mkdir("pie");
    exit(0);
  }
  close(p[1]);
  turns();
  close(p[0]);
  wait(0);
  SAY("PI: writer-told", seen("pie"));
}

// A kill lands on a slot parked on a retry: its read end closes, and it never returns.
static void
killreader(void)
{
  int p[2];
  char b;

  pipe(p);
  int c = fork();
  if(c == 0){
    close(p[1]);
    read(p[0], &b, 1);
    mkdir("pik");
    exit(0);
  }
  close(p[0]);
  turns();
  SAY("PI: kill-blocked", kill(c) == 0);
  SAY("PI: kill-reaped", wait(0) == c);
  SAY("PI: kill-closed", write(p[1], "x", 1) == -1);
  SAY("PI: kill-no-return", !seen("pik"));
  close(p[1]);
}

// More refusals than ofile has rows, so one leaked fd per call would run it out, and
// the same two numbers afterward show none did.
static void
badptr(void)
{
  int g[2], h[2], refused = 0;

  pipe(g);
  close(g[0]);
  close(g[1]);
  for(int i = 0; i < 20; i++){
    refused += pipe(BAD) == -1;
    refused += pipe(STRADDLE) == -1;
  }
  SAY("PI: bad-refused", refused == 40);
  SAY("PI: no-leak", pipe(h) == 0 && h[0] == g[0] && h[1] == g[1]);
  close(h[0]);
  close(h[1]);
}

int
main(void)
{
  if(fork() == 0){
    bulk();
    blocks();
    noreader();
    killreader();
    badptr();
    exit(0);
  }
  wait(0);
  printf("PI: done\n");
  for(;;)
    ;
}
