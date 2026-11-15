// SPDX-License-Identifier: GPL-3.0-or-later
// fdprobe.c: S-STAGE-2B fd rung (ADR-135 D10-D13). Kernel fds are 16 and up,
// until rungs 5+6 (D21) make every fd the kernel's lowest free one.
// Whether a last reference dropped shows through ialloc: it hands out the lowest
// free inum, so a file created after the drop gets the freed one back.
#include "kernel/types.h"
#include "kernel/stat.h"
#include "kernel/fcntl.h"
#include "user/user.h"
#include "user/uyield.h"

#define SAY(t, c) do { if(c) printf(t "=yes\n"); else printf(t "=no\n"); } while(0)
#define WC (O_CREATE | O_WRONLY)

// ADR-135 D21. Before rungs 5+6 a kernel fd is 16 or above and BPF's are
// below; after, every fd is the kernel's and is the lowest free one. A pipe's
// fds were always the kernel's, so its first one says which build this is.
static int kbase;

static void
learn(void)
{
  int p[2];
  if(pipe(p) == 0){
    kbase = p[0] >= 16 ? 16 : 0;
    close(p[0]);
    close(p[1]);
  }
}

// The lowest free fd of the one table (used only after the rung).
static int
low(void)
{
  int d = dup(1);
  if(d >= 0)
    close(d);
  return d;
}

// A fd the kernel answered: 16 or above before the rung, the lowest free after.
#define KFD(fd, want) (kbase == 16 ? (fd) >= 16 : ((fd) >= 0 && (fd) == (want)))
// A fd BPF answered before the rung; after it, the kernel's lowest free too.
#define BFD(fd, want) (kbase == 16 ? ((fd) >= 0 && (fd) < 16) : ((fd) >= 0 && (fd) == (want)))

// SYS_upc_forknote from user code: the kernel must refuse (ADR-135 D12).
static inline int
note24(int a)
{
  register int a0 __asm__("a0") = a;
  __asm__ volatile("li a7, 24\n\tecall" : "+r"(a0) : : "memory");
  return a0;
}

// Through BPF: read-only open, no masked flag.
static int
slurp(char *path, char *buf, int max)
{
  int fd = open(path, O_RDONLY);
  if(fd < 0)
    return -1;
  int got = 0, n;
  while(got < max && (n = read(fd, buf + got, max - got)) > 0)
    got += n;
  close(fd);
  return got;
}

static int
inum(int fd)
{
  struct stat st;
  return fstat(fd, &st) == 0 ? st.ino : -1;
}

static int
fsize(int fd)
{
  struct stat st;
  return fstat(fd, &st) == 0 ? (int)st.size : -1;
}

static void
spin(void)
{
  for(;;)
    yield();
}

static void
basics(void)
{
  char buf[320], got[320];
  struct stat st;
  for(int i = 0; i < 300; i++)
    buf[i] = 'a' + i % 26;

  int w = low();
  int fd = open("fa", WC);
  SAY("FD: created", KFD(fd, w));
  SAY("FD: wrote", write(fd, buf, 300) == 300);
  SAY("FD: closed", close(fd) == 0);
  fd = open("fa", O_RDONLY);
  SAY("FD: bpf-fd", BFD(fd, w));
  close(fd);
  SAY("FD: read-back", slurp("fa", got, 320) == 300 && memcmp(buf, got, 300) == 0);

  fd = open("fa", O_WRONLY | O_TRUNC);
  SAY("FD: trunc-fd", KFD(fd, w));
  SAY("FD: trunc-empty", fsize(fd) == 0 && slurp("fa", got, 320) == 0);
  close(fd);

  fd = open("fb", O_CREATE | O_RDWR);
  SAY("FD: rw-kernel", KFD(fd, w));
  SAY("FD: rw-wrote", write(fd, "0123456789", 10) == 10);
  close(fd);
  fd = open("fb", O_CREATE | O_RDWR); // create on an existing file reopens it
  SAY("FD: kernel-read", read(fd, got, 10) == 10 && memcmp(got, "0123456789", 10) == 0);
  SAY("FD: kernel-eof", read(fd, got, 10) == 0);
  SAY("FD: fstat", fstat(fd, &st) == 0 && st.type == T_FILE && st.size == 10 && st.nlink == 1);
  int rp = open("fb", O_CREATE | O_RDWR); // fd is at EOF: a read there never touches the pointer
  SAY("FD: read-ptr-refused", read(rp, (char *)0x00800000, 4) == -1);
  close(rp);
  SAY("FD: write-ptr-refused", write(fd, (char *)0x00800000, 4) == -1);
  SAY("FD: fstat-ptr-refused", fstat(fd, (struct stat *)0x00800000) == -1);

  int d = dup(fd);
  SAY("FD: dup", KFD(d, w + 1) && d != fd); // fd holds the lowest; the dup is next
  SAY("FD: dup-shares", write(d, "x", 1) == 1 && fsize(fd) == 11);
  SAY("FD: close-dup", close(d) == 0);
  SAY("FD: close-twice", close(d) == -1);
  SAY("FD: orig-kept", write(fd, "y", 1) == 1 && fsize(fd) == 12);
  close(fd);

  SAY("FD: read-bad", read(99, got, 4) == -1);
  SAY("FD: write-neg", write(-1, got, 4) == -1);
  SAY("FD: close-unopened", close(20) == -1);
  SAY("FD: fstat-bad", fstat(31, &st) == -1);
  SAY("FD: dup-bad", dup(32) == -1);
}

static void
inherit(void)
{
  char got[40];

  int fd = open("fc", WC);
  write(fd, "pppppppppp", 10);
  int c = fork();
  if(c == 0){
    SAY("FD: child-write", write(fd, "cccccccccc", 10) == 10);
    exit(0); // fd left open: exit closes it
  }
  SAY("FD: child-reaped", wait(0) == c);
  SAY("FD: shared-offset", write(fd, "qqqqqqqqqq", 10) == 10 && fsize(fd) == 30);
  close(fd);
  SAY("FD: inherited-bytes", slurp("fc", got, 40) == 30 && memcmp(got, "pppppppppp", 10) == 0
      && memcmp(got + 10, "cccccccccc", 10) == 0 && memcmp(got + 20, "qqqqqqqqqq", 10) == 0);

  // D12's reason: the parent closes before the child's first trap.
  fd = open("fd", WC);
  c = fork();
  if(c == 0){
    SAY("FD: parent-closed", write(fd, "late", 4) == 4);
    close(fd);
    exit(0);
  }
  close(fd);
  wait(0);
  SAY("FD: parent-closed-file", slurp("fd", got, 40) == 4 && memcmp(got, "late", 4) == 0);

  // A reused slot: the row's old fds and cwd go, the new parent's arrive.
  fd = open("fe", WC);
  c = fork();
  if(c == 0)
    exit(0);
  wait(0);
  int c2 = fork();
  if(c2 == 0){
    SAY("FD: reuse-inherit", write(fd, "r", 1) == 1);
    int lw = low();
    int r = open("rel", WC); // relative: reaches the row's cwd
    SAY("FD: reuse-cwd", KFD(r, lw));
    close(r);
    exit(0);
  }
  SAY("FD: reuse-slot", c2 == c);
  wait(0);
  SAY("FD: reuse-kept", fsize(fd) == 1);
  close(fd);
}

// D13: the last reference is the child's. Exit, or a kill before its first trap.
static void
lastref(void)
{
  int fd = open("x1", WC);
  write(fd, "data", 4);
  int k = inum(fd);
  int c = fork();
  if(c == 0)
    exit(0);
  unlink("x1");
  close(fd);
  int h = open("x2", WC);
  SAY("FD: ref-held", inum(h) != k); // the note's filedup keeps the inode
  close(h);
  unlink("x2");
  wait(0);
  h = open("x3", WC);
  SAY("FD: exit-closed", inum(h) == k);
  close(h);
  unlink("x3");

  fd = open("x4", WC);
  write(fd, "data", 4);
  k = inum(fd);
  c = fork();
  if(c == 0)
    spin();
  unlink("x4");
  close(fd);
  SAY("FD: kill-ok", kill(c) == 0); // the child has not run: no first trap yet
  SAY("FD: kill-reaped", wait(0) == c);
  h = open("x5", WC);
  SAY("FD: kill-closed", inum(h) == k);
  close(h);
  unlink("x5");
}

// D12: a user's 24, against every kind of slot.
static void
forge(void)
{
  int fd = open("fg", WC);
  SAY("FD: note-zero", note24(0) == -1);
  SAY("FD: note-neg", note24(-1) == -1);
  SAY("FD: note-huge", note24(99) == -1);
  SAY("FD: note-own", note24(getpid() - 1) == -1);
  int c = fork();
  if(c == 0){
    yield();
    SAY("FD: sibling-intact", write(fd, "s", 1) == 1);
    exit(0);
  }
  SAY("FD: note-live", note24(c - 1) == -1); // a live child, already noted
  yield();
  wait(0);
  SAY("FD: sibling-file", fsize(fd) == 1);
  close(fd);
}

// T-F3 (ADR-135 D19, S-STAGE-2C Phase 7; closed by D21). Plain O_RDWR on a regular
// file carries no masked flag, so before rungs 5+6 the open is BPF's and the fd
// is below 16. A write on it used to print its bytes on the TTY and answer n,
// with the file untouched; the interim answers -1. After the rung the fd is a
// kernel file: the write lands, and the probe puts the bytes back.
static void
rdwr(void)
{
  char before[8], after[8];
  int nb = slurp("README", before, 8);
  int fd = open("README", O_RDWR);
  if(kbase == 16){
    SAY("FD: rdwr-file-write", fd >= 0 && fd < 16 && write(fd, "zz", 2) == -1);
  } else {
    int ok = fd >= 0 && write(fd, "zz", 2) == 2;
    close(fd);
    fd = open("README", O_RDWR);
    SAY("FD: rdwr-file-write", ok && write(fd, before, 2) == 2);
  }
  close(fd);
  SAY("FD: readme-intact", nb == 8 && slurp("README", after, 8) == 8 && memcmp(before, after, 8) == 0);
}

int
main(void)
{
  if(fork() == 0){
    learn();
    basics();
    inherit();
    lastref();
    forge();
    rdwr();
    exit(0);
  }
  wait(0);

  // D12's other reason: the parent exits before the child's first trap.
  if(fork() == 0){
    int fd = open("fo", WC);
    write(fd, "dead", 4);
    if(fork() == 0){
      SAY("FD: orphan-wrote", write(fd, "!", 1) == 1);
      exit(0);
    }
    exit(0);
  }
  while(wait(0) != -1)
    ;
  char got[8];
  SAY("FD: orphan-file", slurp("fo", got, 8) == 5 && memcmp(got, "dead!", 5) == 0);
  printf("FD: done\n");
  for(;;)
    ;
}
