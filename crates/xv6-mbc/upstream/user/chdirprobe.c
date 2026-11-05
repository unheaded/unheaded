// SPDX-License-Identifier: GPL-3.0-or-later
// chdirprobe.c: S-STAGE-2B chdir rung (ADR-135 D15). Off the root, open goes to
// the kernel and answers a fd at 16 or above; at the root it stays in BPF, below 16.
// A dropped cwd shows through ialloc, as in fdprobe: it hands out the lowest free inum.
#include "kernel/types.h"
#include "kernel/stat.h"
#include "kernel/fcntl.h"
#include "user/user.h"

#define SAY(t, c) do { if(c) printf(t "=yes\n"); else printf(t "=no\n"); } while(0)

static int
kfd(char *path)
{
  int fd = open(path, O_RDONLY);
  int ok = fd >= 16;
  if(fd >= 0)
    close(fd);
  return ok;
}

static int
bfd(char *path)
{
  int fd = open(path, O_RDONLY);
  int ok = fd >= 0 && fd < 16;
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
  SAY("CD: read-rel", fd >= 16 && read(fd, got, 8) == 5 && memcmp(got, "chdir", 5) == 0);
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
  SAY("CD: bpf-read", fd >= 0 && fd < 16 && read(fd, got, 8) == 5 && memcmp(got, "chdir", 5) == 0);
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
