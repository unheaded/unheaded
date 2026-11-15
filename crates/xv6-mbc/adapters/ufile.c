// SPDX-License-Identifier: GPL-3.0-or-later
//
// ufile.c — UNHEADED file-descriptor support layer (ftable + devsw).
// Track 2 Phase 2.4 (ADR-081 "evolve from xv6, never break the boot").
//
// Replaces MIT kernel/file.c in the Makefile.mbc OBJS list; the MIT file
// stays vendored, unwired.
//
// Boot-live: fileinit at main(), and the devsw[] array LIVES here —
// uconsole.c's consoleinit wires devsw[CONSOLE].read/.write into it, so
// this file is the console stack's link-time anchor. BPF owns fds below 16;
// fds at 16 and up route here (ADR-135 D10, D11), so fileread, filewrite and
// fileclose run for those. All keep stock strength: the devsw major bounds
// checks, fileclose's copy-then-release pattern, and filewrite's MAXOPBLOCKS
// transaction batching are verbatim.
//
// Support functions for system calls that involve file descriptors.

#include "types.h"
#include "riscv.h"
#include "defs.h"
#include "param.h"
#include "fs.h"
#include "spinlock.h"
#include "sleeplock.h"
#include "file.h"
#include "stat.h"
#include "proc.h"
#include "upc_slots.h"

// uconsole.c: the console's write and its polled read for a slot row
// (ADR-135 D19, D20); under UPC_RUNG56 a console file reaches them from here.
int consolewrite(int user_src, uint64 src, int n);
int upc_consoleread(uint64 dst, int n);

struct devsw devsw[NDEV];
struct {
  struct spinlock lock;
  struct file file[NFILE];
} ftable;

void
fileinit(void)
{
  initlock(&ftable.lock, "ftable");
}

// Allocate a file structure.
struct file*
filealloc(void)
{
  struct file *f;

  acquire(&ftable.lock);
  for(f = ftable.file; f < ftable.file + NFILE; f++){
    if(f->ref == 0){
      f->ref = 1;
      release(&ftable.lock);
      return f;
    }
  }
  release(&ftable.lock);
  return 0;
}

// Increment ref count for file f.
struct file*
filedup(struct file *f)
{
  acquire(&ftable.lock);
  if(f->ref < 1)
    panic("filedup");
  f->ref++;
  release(&ftable.lock);
  return f;
}

// Close file f.  (Decrement ref count, close when reaches 0.)
void
fileclose(struct file *f)
{
  struct file ff;

  acquire(&ftable.lock);
  if(f->ref < 1)
    panic("fileclose");
  if(--f->ref > 0){
    release(&ftable.lock);
    return;
  }
  ff = *f;
  f->ref = 0;
  f->type = FD_NONE;
  release(&ftable.lock);

  if(ff.type == FD_PIPE){
    pipeclose(ff.pipe, ff.writable);
  } else if(ff.type == FD_INODE || ff.type == FD_DEVICE){
    // ADR-135 D21: the console file has no inode (ip == 0); nothing to put.
    if(ff.ip){
      begin_op();
      iput(ff.ip);
      end_op();
    }
  }
}

// Get metadata about file f.
// addr is a user virtual address, pointing to a struct stat.
int
filestat(struct file *f, uint64 addr)
{
  struct proc *p = myproc();
  struct stat st;

  if(f->type == FD_INODE || f->type == FD_DEVICE){
    // ADR-135 D21: no inode, no stat. -1 is what BPF's fstat answered for a
    // console fd.
    if(f->ip == 0)
      return -1;
    ilock(f->ip);
    stati(f->ip, &st);
    iunlock(f->ip);
    if(copyout(p->pagetable, addr, (char *)&st, sizeof(st)) < 0)
      return -1;
    return 0;
  }
  return -1;
}

// Read from file f.
// addr is a user virtual address.
int
fileread(struct file *f, uint64 addr, int n)
{
  int r = 0;

  if(f->readable == 0)
    return -1;

  if(f->type == FD_PIPE){
    r = piperead(f->pipe, addr, n);
  } else if(f->type == FD_DEVICE){
    if(f->major < 0 || f->major >= NDEV || !devsw[f->major].read)
      return -1;
#if UPC_RUNG56
    // ADR-135 D20, D21: inside a tagged trap the console is read by the polled
    // read, which never sleeps and may answer UPC_RETRY_RC. upc_slot_active(),
    // not upc_is_slot_row(): slot 0 is the one row sleep() does not refuse,
    // and consoleread would sleep it with nothing runnable (review F1).
    if(f->major == CONSOLE && upc_slot_active())
      return upc_consoleread(addr, n);
#endif
    r = devsw[f->major].read(1, addr, n);
  } else if(f->type == FD_INODE){
    ilock(f->ip);
    if((r = readi(f->ip, 1, addr, f->off, n)) > 0)
      f->off += r;
    iunlock(f->ip);
  } else {
    panic("fileread");
  }

  return r;
}

// Write to file f.
// addr is a user virtual address.
int
filewrite(struct file *f, uint64 addr, int n)
{
  int r, ret = 0;

  if(f->writable == 0)
    return -1;

  if(f->type == FD_PIPE){
    ret = pipewrite(f->pipe, addr, n);
  } else if(f->type == FD_DEVICE){
    if(f->major < 0 || f->major >= NDEV || !devsw[f->major].write)
      return -1;
#if UPC_RUNG56
    // ADR-135 D19, D21: a copy that fails before the first byte is a bad
    // pointer and answers -1; stock passes consolewrite's 0 through.
    if(f->major == CONSOLE && upc_slot_active()){
      ret = consolewrite(1, addr, n);
      return (ret == 0 && n > 0) ? -1 : ret;
    }
#endif
    ret = devsw[f->major].write(1, addr, n);
  } else if(f->type == FD_INODE){
    // write a few blocks at a time to avoid exceeding
    // the maximum log transaction size, including
    // i-node, indirect block, allocation blocks,
    // and 2 blocks of slop for non-aligned writes.
    int max = ((MAXOPBLOCKS-1-1-2) / 2) * BSIZE;
    int i = 0;
    while(i < n){
      int n1 = n - i;
      if(n1 > max)
        n1 = max;

      begin_op();
      ilock(f->ip);
      if ((r = writei(f->ip, 1, addr + i, f->off, n1)) > 0)
        f->off += r;
      iunlock(f->ip);
      end_op();

      if(r != n1){
        // error from writei
        break;
      }
      i += r;
    }
    ret = (i == n ? n : -1);
  } else {
    panic("filewrite");
  }

  return ret;
}
