// Physical memory allocator, for user processes,
// kernel stacks, page-table pages,
// and pipe buffers. Allocates whole 4096-byte pages.

#include "types.h"
#include "param.h"
#include "memlayout.h"
#include "spinlock.h"
#include "riscv.h"
#include "defs.h"

// Phase 1.4 init-loop bisection — see adapters/start_mbc.c::mmio_puts and
// references/phase14-xv6-init-loop-root-cause-2026-05-13.md. Single-char
// markers minimise TTY pressure when freerange iterates ~1900 times.
extern void mmio_puts(const char *s);

void freerange(void *pa_start, void *pa_end);

extern char end[]; // first address after kernel.
                   // defined by kernel.ld.

struct run {
  struct run *next;
};

struct {
  struct spinlock lock;
  struct run *freelist;
} kmem;

void
kinit()
{
  initlock(&kmem.lock, "kmem");
  freerange(end, (void*)PHYSTOP);
}

void
freerange(void *pa_start, void *pa_end)
{
  char *p;
  p = (char*)PGROUNDUP((uint64)pa_start);
  mmio_puts("freerange enter\n");
  for(; p + PGSIZE <= (char*)pa_end; p += PGSIZE)
    kfree(p);
  mmio_puts("freerange exit\n");
}

// Free the page of physical memory pointed at by pa,
// which normally should have been returned by a
// call to kalloc().  (The exception is when
// initializing the allocator; see kinit above.)
void
kfree(void *pa)
{
  struct run *r;

  if(((uint64)pa % PGSIZE) != 0 || (char*)pa < end || (uint64)pa >= PHYSTOP)
    panic("kfree");

#ifndef UPC_SKIP_KFREE_MEMSET
  // Fill with junk to catch dangling refs.
  memset(pa, 1, PGSIZE);
#endif

  r = (struct run*)pa;

  static int probed = 0;
  if (!probed) { probed = 1; mmio_puts("kfree-A\n"); }
  acquire(&kmem.lock);
  if (probed == 1) { probed = 2; mmio_puts("kfree-B\n"); }
  r->next = kmem.freelist;
  kmem.freelist = r;
  release(&kmem.lock);
  if (probed == 2) { probed = 3; mmio_puts("kfree-C\n"); }
}

// Allocate one 4096-byte page of physical memory.
// Returns a pointer that the kernel can use.
// Returns 0 if the memory cannot be allocated.
void *
kalloc(void)
{
  struct run *r;

  acquire(&kmem.lock);
  r = kmem.freelist;
  if(r)
    kmem.freelist = r->next;
  release(&kmem.lock);

#ifndef UPC_SKIP_KALLOC_MEMSET
  if(r)
    memset((char*)r, 5, PGSIZE); // fill with junk
#endif
  return (void*)r;
}
