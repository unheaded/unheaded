// SPDX-License-Identifier: GPL-3.0-or-later
//
// uexec.c — UNHEADED exec (kernel-side program-image bookkeeping).
// Track 2 Phase 2.4 (ADR-081 "evolve from xv6, never break the boot").
//
// Replaces MIT kernel/exec.c in the Makefile.mbc OBJS list; the MIT file
// stays vendored, unwired.
//
// UPC patches carried verbatim (Gate B/C provenance, Phase 1.6-1.7): kexec's
// `$`/`%` entry sentinels + "KX:enter" marker, and the MBC-userland non-ELF
// path — userland is rv32i-to-mbc bytecode pre-staged in ROM_MAP/RV2MBC_MAP
// by upc-bootctl, so on ELF-magic mismatch kexec wires trapframe->epc = 0
// (user .text at RV byte 0 per user.ld), sp = 0x500000 - 16, a0 = 1,
// a1 = 0 and returns argc. The stock ELF loader below that branch is
// dormant on the UPC and keeps stock bounds (the BPF exec(7) owns the live
// argv harvest at VA 0x600000, ADR-077/078).
//
#include "types.h"
#include "param.h"
#include "memlayout.h"
#include "riscv.h"
#include "spinlock.h"
#include "proc.h"
#include "defs.h"
#include "elf.h"

static int loadseg(pde_t *, uint64, struct inode *, uint, uint);

// map ELF permissions to PTE permission bits.
int flags2perm(int flags)
{
    int perm = 0;
    if(flags & 0x1)
      perm = PTE_X;
    if(flags & 0x2)
      perm |= PTE_W;
    return perm;
}

//
// the implementation of the exec() system call
//
int
kexec(char *path, char **argv)
{
  char *s, *last;
  int i, off;
  uint64 argc, sz = 0, sp, ustack[MAXARG], stackbase;
  struct elfhdr elf;
  struct inode *ip;
  struct proghdr ph;
  pagetable_t pagetable = 0, oldpagetable;
  struct proc *p = myproc();

  extern void mmio_puts(const char *);
  extern void mmio_putc(char);  /* declared in adapters/start_mbc.c */
  mmio_putc('$');  /* kexec entry sentinel */
  mmio_puts("KX:enter\n");
  mmio_putc('%');  /* after mmio_puts */

  begin_op();

  // Open the executable file.
  if((ip = namei(path)) == 0){
    end_op();
    return -1;
  }
  ilock(ip);

  // Read the ELF header.
  if(readi(ip, 0, (uint64)&elf, 0, sizeof(elf)) != sizeof(elf))
    goto bad;

  // Is this really an ELF file?
#ifdef UPC_FLAT_TRAMPOLINE
  // On UPC the userland is MBC bytecode (rv32i-to-mbc translator output),
  // not an ELF. The bytecode + rv2mbc + .data sidecars were pre-loaded into
  // ROM_MAP / RV2MBC_MAP / RAM_MAP by upc-bootctl --userland, so the
  // kernel-side job is just to point trapframe->epc at the user entry
  // (RV byte 0 — matches crates/xv6-mbc/upstream/user/user.ld). The
  // subsequent forkret → prepare_return → SRET reads SEPC=0,
  // RV2MBC_MAP[0] = USER_ROM_BASE, and user MBC runs.
  if(elf.magic != ELF_MAGIC) {
    extern void mmio_puts(const char *);
    mmio_puts("kexec: MBC userland — wiring trapframe->epc=0\n");
    if(p->trapframe == 0)
      panic("kexec: no trapframe");
    p->trapframe->epc = 0;     // userland .text starts at RV byte 0
    // User stack sits in a free RAM_MAP region above the kernel BSS
    // (~0x114AA0) and below the fs.img ramdisk (0x800000). Grow down
    // from byte 0x500000 (5 MiB) — 1 MiB of stack room.
    p->trapframe->sp  = 0x500000UL - 16;
    p->trapframe->a0  = 1;     // argc = 1 (just "/init")
    p->trapframe->a1  = 0;     // argv = NULL (no argv frame on UPC yet)
    iunlockput(ip);
    end_op();
    return 1;                  // argc — forkret writes this to a0
  }
#else
  if(elf.magic != ELF_MAGIC)
    goto bad;
#endif

  if((pagetable = proc_pagetable(p)) == 0)
    goto bad;

  // Load program into memory.
  for(i=0, off=elf.phoff; i<elf.phnum; i++, off+=sizeof(ph)){
    if(readi(ip, 0, (uint64)&ph, off, sizeof(ph)) != sizeof(ph))
      goto bad;
    if(ph.type != ELF_PROG_LOAD)
      continue;
    if(ph.memsz < ph.filesz)
      goto bad;
    if(ph.vaddr + ph.memsz < ph.vaddr)
      goto bad;
    if(ph.vaddr % PGSIZE != 0)
      goto bad;
    uint64 sz1;
    if((sz1 = uvmalloc(pagetable, sz, ph.vaddr + ph.memsz, flags2perm(ph.flags))) == 0)
      goto bad;
    sz = sz1;
    if(loadseg(pagetable, ph.vaddr, ip, ph.off, ph.filesz) < 0)
      goto bad;
  }
  iunlockput(ip);
  end_op();
  ip = 0;

  p = myproc();
  uint64 oldsz = p->sz;

  // Allocate some pages at the next page boundary.
  // Make the first inaccessible as a stack guard.
  // Use the rest as the user stack.
  sz = PGROUNDUP(sz);
  uint64 sz1;
  if((sz1 = uvmalloc(pagetable, sz, sz + (USERSTACK+1)*PGSIZE, PTE_W)) == 0)
    goto bad;
  sz = sz1;
  uvmclear(pagetable, sz-(USERSTACK+1)*PGSIZE);
  sp = sz;
  stackbase = sp - USERSTACK*PGSIZE;

  // Copy argument strings into new stack, remember their
  // addresses in ustack[].
  for(argc = 0; argv[argc]; argc++) {
    if(argc >= MAXARG)
      goto bad;
    sp -= strlen(argv[argc]) + 1;
    sp -= sp % 16; // riscv sp must be 16-byte aligned
    if(sp < stackbase)
      goto bad;
    if(copyout(pagetable, sp, argv[argc], strlen(argv[argc]) + 1) < 0)
      goto bad;
    ustack[argc] = sp;
  }
  ustack[argc] = 0;

  // push a copy of ustack[], the array of argv[] pointers.
  sp -= (argc+1) * sizeof(uint64);
  sp -= sp % 16;
  if(sp < stackbase)
    goto bad;
  if(copyout(pagetable, sp, (char *)ustack, (argc+1)*sizeof(uint64)) < 0)
    goto bad;

  // a0 and a1 contain arguments to user main(argc, argv)
  // argc is returned via the system call return
  // value, which goes in a0.
  p->trapframe->a1 = sp;

  // Save program name for debugging.
  for(last=s=path; *s; s++)
    if(*s == '/')
      last = s+1;
  safestrcpy(p->name, last, sizeof(p->name));
    
  // Commit to the user image.
  oldpagetable = p->pagetable;
  p->pagetable = pagetable;
  p->sz = sz;
  p->trapframe->epc = elf.entry;  // initial program counter = ulib.c:start()
  p->trapframe->sp = sp; // initial stack pointer
  proc_freepagetable(oldpagetable, oldsz);

  return argc; // this ends up in a0, the first argument to main(argc, argv)

 bad:
  if(pagetable)
    proc_freepagetable(pagetable, sz);
  if(ip){
    iunlockput(ip);
    end_op();
  }
  return -1;
}

// Load an ELF program segment into pagetable at virtual address va.
// va must be page-aligned
// and the pages from va to va+sz must already be mapped.
// Returns 0 on success, -1 on failure.
static int
loadseg(pagetable_t pagetable, uint64 va, struct inode *ip, uint offset, uint sz)
{
  uint i, n;
  uint64 pa;

  for(i = 0; i < sz; i += PGSIZE){
    pa = walkaddr(pagetable, va + i);
    if(pa == 0)
      panic("loadseg: address should exist");
    if(sz - i < PGSIZE)
      n = sz - i;
    else
      n = PGSIZE;
    if(readi(ip, 0, (uint64)pa, offset+i, n) != n)
      return -1;
  }
  
  return 0;
}
