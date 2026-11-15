// SPDX-License-Identifier: GPL-3.0-or-later
// hostile_path.c: S-STAGE-2C Phase 3, ADR-135 D16 (R-F5, backlog D-27). User
// pointers that leave their owner's memory.
//
// Run as the userland directly, so this program is init (slot 0).
//
// The child (slot 1) hands BPF's open and exec a path outside its slice. A wild
// path only proves something if the unbounded read would have found a name, so a
// grandchild (slot 2, the next slice up) first leaves "README" and "echo" in its
// own memory at VA 0x100000: slot 1's VA 0x900000 is that byte when the path is
// added to the slice base with no bound. The child also puts "README" in the last
// eight bytes below its ceiling, and writes to a pipe from a buffer past it.
//
// Init then hands the kernel two pointers into kernel memory. Its base is 0, so
// its VAs are physical addresses: it finds a name in the kernel's .rodata by
// reading it (init's own loads reach low memory), asks mkdir to use that pointer,
// and asks pipe to store its two fds at the kernel's .data.
//
// Observables (fixed classes; the MBC userland varargs ABI is unreliable):
//   "HQ: open-wild-refused=yes|no"          open of a path past the ceiling is -1
//   "HQ: exec-wild-refused=yes|no"          exec of a path past the ceiling is -1
//   "HQ: open-unterminated-refused=yes|no"  a path with no NUL before the ceiling is -1
//   "HQ: pipe-badbuf-refused=yes|no"        write to a pipe from a bad buffer is -1
//   "HQ: slot0-kernel-name-refused=yes|no"  mkdir of a kernel .rodata pointer is -1,
//                                           and that directory does not exist after
//   "HQ: slot0-copyout-refused=yes|no"      pipe into the kernel's .data is -1
//   "HQ: done"                              init, after the child is reaped
#include "kernel/types.h"
#include "kernel/stat.h"
#include "user/user.h"

#define SAY(t, c) do { if(c) printf(t "=yes\n"); else printf(t "=no\n"); } while(0)

#define NEXT_SLICE_VA   0x00900000u  // slot s's VA for slot s+1's VA 0x100000
#define PLANT_VA        0x00100000u
#define CEILING_PATH    0x007FFFF8u
#define KERNEL_TEXT     0x00020000u  // kernel .text and .rodata sit in this 64 KiB
#define KERNEL_SPAN     0x00010000u
#define KERNEL_DATA     0x00100000u

static void
put(volatile char *d, const char *s)
{
  int i = 0;
  do
    d[i] = s[i];
  while(s[i++]);
}

static void
child(void)
{
  if(fork() == 0){
    put((volatile char *)PLANT_VA, "README");
    put((volatile char *)(PLANT_VA + 16), "echo");
    exit(0);
  }
  wait(0);

  SAY("HQ: open-wild-refused", open((char *)NEXT_SLICE_VA, 0) < 0);
  char *argv[] = { "echo", 0 };
  SAY("HQ: exec-wild-refused", exec((char *)(NEXT_SLICE_VA + 16), argv) == -1);

  // A path with no NUL before the ceiling. BPF's bound (D16) refuses it because
  // 128 bytes from its start cross the ceiling; the kernel's copy-in refuses it
  // because the next page is not the caller's. (A short terminated path here,
  // which D16 refused for the same 128 bytes, opens once open is the kernel's:
  // ADR-135 D21, review F2.)
  for(int i = 0; i < 8; i++)
    ((volatile char *)CEILING_PATH)[i] = 'R';
  SAY("HQ: open-unterminated-refused", open((char *)CEILING_PATH, 0) < 0);

  int p[2];
  if(pipe(p) < 0){
    printf("HQ: pipe-badbuf-refused=no\n");
    return;
  }
  SAY("HQ: pipe-badbuf-refused", write(p[1], (char *)NEXT_SLICE_VA, 4) == -1);
}

static int
lower(char c)
{
  return c >= 'a' && c <= 'z';
}

// The first NUL-bounded run of 5 to 12 lower-case letters in the kernel image
// that names nothing in the root directory. Copies it to nm and returns where the
// kernel keeps it, or 0.
static char *
kernel_name(char *nm)
{
  volatile char *k = (volatile char *)KERNEL_TEXT;
  for(uint i = 1; i + 14 < KERNEL_SPAN; i++){
    if(k[i - 1] != 0 || !lower(k[i]))
      continue;
    uint n = 0;
    while(n < 13 && lower(k[i + n]))
      n++;
    if(n < 5 || n > 12 || k[i + n] != 0){
      i += n;
      continue;
    }
    for(uint j = 0; j <= n; j++)
      nm[j] = k[i + j];
    int fd = open(nm, 0);
    if(fd >= 0){
      close(fd);
      i += n;
      continue;
    }
    return (char *)(k + i);
  }
  return 0;
}

int
main(void)
{
  if(fork() == 0){
    child();
    exit(0);
  }
  wait(0);

  char nm[16];
  char *kn = kernel_name(nm);
  // Refused, and nothing was made: the same name from init's own stack is new.
  SAY("HQ: slot0-kernel-name-refused", kn && mkdir(kn) == -1 && mkdir(nm) == 0);
  SAY("HQ: slot0-copyout-refused", pipe((int *)KERNEL_DATA) == -1);

  printf("HQ: done\n");
  for(;;)
    ;
}
