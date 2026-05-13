# Phase 1.3 AP-5 — fork() Bridge: xv6 kfork() ↔ eBPF SYS_FORK

**Status:** Decision recorded 2026-05-13. Pre-work for Phase 1.3 (process model).
**Parent:** `references/battle-plan-ascend-linux-2026-05-08.md` Phase 1.3.

## Substrate inventory

Three call sites span the fork bridge:

- **eBPF-side primitive: `SYS_FORK`** —
  `ebpf/monad-cpu-ebpf/src/main.rs:1036-1080`. Allocates a `PROC_TABLE` slot
  (scans for free index), copies r0..r15 from the parent slot, sets the child's
  PC, marks the slot RUNNABLE, and writes `page_dir_base` via
  `phase12::pgd_base_for_pid(child_pid)` (Phase 1.2 helper).
- **xv6-side authority: `kfork()`** —
  `crates/xv6-mbc/upstream/kernel/proc.c:267-313`. Calls `allocproc()` (slot
  reservation), `proc_pagetable()` (allocates child pgd + maps TRAMPOLINE +
  TRAPFRAME), `uvmcopy(parent_pgd, child_pgd, sz)` (clones user pages), copies
  parent trapframe to child, sets child trapframe `a0 = 0`, increments fd
  refcounts, sets parent pointer, sets child name, marks `state = RUNNABLE`.
  Returns child pid to parent.
- **Translator bridge: `syscall_shims.S`** —
  `crates/xv6-mbc/adapters/syscall_shims.S:26-50`. Converts RV32 `ecall` with
  `a7 = SYS_FORK_NUM` into an MBC `SYSCALL` (opcode `0x40`) carrying the
  syscall number in the standard register, dispatched by the BPF syscall
  table.

## Decision: xv6's kfork() is AUTHORITATIVE

The eBPF `SYS_FORK` handler is a **low-level primitive**, not a Linux-level
`fork()`. xv6's `kfork()` is the policy layer; the BPF handler is the
mechanism. Concretely, the runtime flow is:

```
user RV32 ──ecall a7=SYS_FORK──▶ translator emits MBC SYSCALL 0x40
                                       │
                                       ▼
                          BPF syscall dispatch table
                                       │
                                       ▼
                 sys_fork_handler (main.rs:1036-1080)
                   • scan PROC_TABLE for free slot
                   • copy r0..r15 from parent slot
                   • set child page_dir_base via pgd_base_for_pid
                   • set slot state RUNNABLE
                   • return child_pid in a0
                                       │
                                       ▼
                              SRET back to xv6
                                       │
                                       ▼
                         xv6 kfork() continuation (proc.c:267-313)
                   • sees a0 = child_pid (kernel-side)
                   • proc_pagetable(child) → maps TRAMPOLINE/TRAPFRAME
                   • uvmcopy(parent_pgd, child_pgd, sz)
                   • copy *parent->trapframe → *child->trapframe
                   • child->trapframe->a0 = 0 (child sees 0)
                   • parent fd refcounts, parent pointer, name
                   • child->state = RUNNABLE
                   • returns child_pid to user
```

The BPF handler does not know about fds, parent pointers, the trapframe layout,
or RUNNABLE state machine bookkeeping. That all lives in xv6 C code. The BPF
handler just owns the slot allocation and the physical pgd-base assignment —
both of which the kernel cannot easily do, because the BPF map is the only
write path to `PROC_TABLE` and `MbcCpuState`.

## Open question: uvmcopy

xv6's `uvmcopy(parent_pgd, child_pgd, sz)` walks the parent pgd, allocates a
fresh physical page for each user mapping, `memcpy`s the contents, and installs
a matching PTE in the child pgd. **Does this need a BPF-side mirror?**

**Recommendation: run uvmcopy as plain RV32 C code.** Justification:

1. The translator already lowers C-level `memcpy` to MBC `LD`/`ST` loops; no
   new MBC opcode or BPF handler is required.
2. xv6's `kalloc()` / `kfree()` already manage the physical page pool from
   inside the kernel (the Phase 1.2 allocator A1 region
   `0x00900000..0x00EFFFFF`); BPF does not need a mirror.
3. Keeping uvmcopy in C minimizes BPF instruction-budget pressure.

**Performance implication:** uvmcopy at 4 KiB/page through RV32 LD/ST loops is
slow — a 16 KiB user image clones in ~4096 MBC instructions, all of them
single-word memory ops. This is acceptable for Phase 1.3 (init + shell, small
images). If/when bigger workloads land, Phase 3+ may add a BPF-side bulk-copy
helper (`SYS_BULK_COPY_PAGE` or copy-on-write semantics via shared mappings).
This is **out of Phase 1.3 scope**.

## Action items (for Phase 1.3 IMPL)

1. Ship `sys_fork_handler` BPF code following the existing primitive pattern
   (slot allocate, copy regs, set `page_dir_base`).
2. Wire xv6 `kfork()` so it calls the syscall path; verify trapframe round-trip.
3. Leave `uvmcopy()` as plain xv6 C — no new BPF helper.
4. Track verifier delta in `references/phase13-ap6-verifier-projection.md`.

---

Free to use. Free to share.
