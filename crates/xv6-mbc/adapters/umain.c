// SPDX-License-Identifier: GPL-3.0-or-later
//
// umain.c — UNHEADED kernel init sequence (the boot spine).
// Track 2 Phase 2.4 (ADR-081 "evolve from xv6, never break the boot").
//
// Replaces MIT kernel/main.c in the Makefile.mbc OBJS list; the MIT file
// stays vendored, unwired.
//
// Entirely boot-hot, and the most golden-coupled file in the kernel: every
// mmio_puts marker below prints INTO the golden TTY (Phase 1.4 init-loop
// bisection provenance — see adapters/start_mbc.c::mmio_puts). The markers,
// their order, and the three printf boot lines must stay byte-exact.
//
// UPC divergences from stock xv6 main(), carried verbatim: plicinit/
// plicinithart are commented out (no PLIC on UPC; trap routing via MBC IVT)
// and virtio_disk_init is commented out (blk-ramdisk.c owns the disk).

#include "types.h"
#include "param.h"
#include "memlayout.h"
#include "riscv.h"
#include "defs.h"

// Phase 1.4 init-loop bisection: direct MMIO write provided by adapters/start_mbc.c.
// Each marker is one BPF map write — independent of console.c / printf.c / locks,
// so it survives even if a spinlock or page-table breaks the printf path.
extern void mmio_puts(const char *s);

volatile static int started = 0;

// start() jumps here in supervisor mode on all CPUs.
void
main()
{
  if(cpuid() == 0){
    consoleinit();
    mmio_puts("after consoleinit\n");
    printfinit();
    mmio_puts("after printfinit\n");
    printf("\n");
    printf("xv6 kernel is booting\n");
    printf("\n");
    kinit();         // physical page allocator
    mmio_puts("after kinit\n");
    kvminit();       // create kernel page table
    mmio_puts("after kvminit\n");
    kvminithart();   // turn on paging
    mmio_puts("after kvminithart\n");
    procinit();      // process table
    mmio_puts("after procinit\n");
    trapinit();      // trap vectors
    mmio_puts("after trapinit\n");
    trapinithart();  // install kernel trap vector
    mmio_puts("after trapinithart\n");
    /* plicinit(): no PLIC on UPC */      // set up interrupt controller
    /* plicinithart(): no PLIC on UPC */  // ask PLIC for device interrupts
    binit();         // buffer cache
    mmio_puts("after binit\n");
    iinit();         // inode table
    mmio_puts("after iinit\n");
    fileinit();      // file table
    mmio_puts("after fileinit\n");
    /* virtio_disk_init(): handled by blk-ramdisk.c */ // emulated hard disk
    userinit();      // first user process
    mmio_puts("after userinit\n");
    __sync_synchronize();
    started = 1;
    mmio_puts("after started=1\n");
  } else {
    while(started == 0)
      ;
    __sync_synchronize();
    printf("hart %d starting\n", cpuid());
    kvminithart();    // turn on paging
    trapinithart();   // install kernel trap vector
    /* plicinithart(): no PLIC on UPC */   // ask PLIC for device interrupts
  }

  scheduler();
}
