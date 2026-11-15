// SPDX-License-Identifier: GPL-3.0-or-later
// chdirprobe.c: S-STAGE-2B chdir rung (ADR-135 D15). Off the root, open goes to
// the kernel and answers a fd at 16 or above; at the root it stays in BPF, below 16.
// After rungs 5+6 (D21) every open is the kernel's and answers the lowest free fd.
// A dropped cwd shows through ialloc, as in fdprobe: it hands out the lowest free inum.
#include "kernel/types.h"
#include "kernel/stat.h"
#include "kernel/fcntl.h"
#include "user/user.h"

#define SAY(t, c) do { if(c) printf(t "=yes\n"); else printf(t "=no\n"); } while(0)

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

static int
kfd(char *path)
{
  int w = low();
  int fd = open(path, O_RDONLY);
  int ok = KFD(fd, w);
  if(fd >= 0)
    close(fd);
  return ok;
}

static int
bfd(char *path)
{
  int w = low();
  int fd = open(path, O_RDONLY);
  int ok = BFD(fd, w);
  if(fd >= 0)
    close(fd);
  return ok;
}

static int
inum(char *path)
{
  struct stat st;
  int fd = open(path, O_RDONLY);
  if(fd < 0)
    return -1;
  int r = fstat(fd, &st) == 0 ? st.ino : -1;
  close(fd);
  return r;
}

// A new file's inum. The file stays, so the next call cannot get the same one.
static int
fresh(char *path)
{
  struct stat st;
  int fd = open(path, O_CREATE | O_WRONLY);
  if(fd < 0)
    return -1;
  int r = fstat(fd, &st) == 0 ? st.ino : -1;
  close(fd);
  return r;
}

static void
offroot(void)
{
  char got[8];

  SAY("CD: mkdir", mkdir("cdd") == 0);
  SAY("CD: chdir", chdir("cdd") == 0);
  SAY("CD: mkdir-rel", mkdir("in") == 0);
  SAY("CD: open-rel", kfd("in"));
  SAY("CD: root-hidden", open("README", O_RDONLY) == -1); // BPF would find /README
  SAY("CD: absolute", kfd("/README"));
  SAY("CD: nope-refused", chdir("nope") == -1);
  SAY("CD: rel-file-refused", chdir("README") == -1);
  SAY("CD: file-refused", chdir("/README") == -1);
  SAY("CD: cwd-kept", kfd("in"));

  int fd = open("f", O_CREATE | O_WRONLY);
  SAY("CD: wrote", write(fd, "chdir", 5) == 5);
  close(fd);
  fd = open("f", O_RDONLY);
  SAY("CD: read-rel", (kbase == 16 ? fd >= 16 : fd >= 0) && read(fd, got, 8) == 5 && memcmp(got, "chdir", 5) == 0);
  close(fd);

  int c = fork();
  if(c == 0){
    SAY("CD: child-inherits", kfd("in"));
    SAY("CD: child-chdir", chdir("in") == 0);
    SAY("CD: child-moved", open("in", O_RDONLY) == -1);
    exit(0);
  }
  wait(0);
  SAY("CD: parent-stayed", kfd("in"));

  SAY("CD: chdir-up", chdir("..") == 0);
  SAY("CD: bpf-again", bfd("cdd/in"));
  fd = open("cdd/f", O_RDONLY);
  SAY("CD: bpf-read", (kbase == 16 ? (fd >= 0 && fd < 16) : fd >= 0) && read(fd, got, 8) == 5 && memcmp(got, "chdir", 5) == 0);
  close(fd);
}

// The slot's last process was off the root, so this child's note has to clear the bit.
static void
reuse(void)
{
  int c = fork();
  if(c == 0){
    SAY("CD: reuse-root", bfd("README"));
    exit(0);
  }
  wait(0);
}

// D15's last limit: a dead slot holds its cwd until the slot's next note.
static void
drop(void)
{
  SAY("CD: mkdir-dz", mkdir("dz") == 0);
  int ino = inum("dz");
  int c = fork();
  if(c == 0){
    chdir("dz");
    exit(0);
  }
  wait(0);
  SAY("CD: unlink-dz", unlink("dz") == 0);
  SAY("CD: cwd-held", fresh("p1") != ino);
  c = fork();
  if(c == 0)
    exit(0);
  wait(0);
  SAY("CD: cwd-dropped", fresh("p2") == ino);
}

int
main(void)
{
  if(fork() == 0){
    learn();
    offroot();
    reuse();
    drop();
    exit(0);
  }
  wait(0);
  printf("CD: done\n");
  for(;;)
    ;
}
