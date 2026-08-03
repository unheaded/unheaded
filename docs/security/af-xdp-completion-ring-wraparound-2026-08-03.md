# AF_XDP: completion ring desynchronizes on u32 counter wraparound (2026-08-03)

**Component:** `ebpf/af-xdp` (`src/umem.rs`, `CompletionRing::consume`)
**Severity:** Medium — reachable on any long-running high-throughput socket
**Found by:** clippy `dead_code` on the ring's `size` field, while taking the
warning count to zero
**Status:** FIXED in the working tree (uncommitted, pending review)

## What was wrong

`CompletionRing::consume` computed the number of completed frames with plain
subtraction:

```rust
let producer = *self.producer;
let consumer = *self.consumer;
let available = producer - consumer;

let mut addrs = Vec::with_capacity(available as usize);
for i in 0..available {
    let idx = ((consumer + i) & self.mask) as usize;
```

AF_XDP ring counters are **free-running `u32`s**. They are not reset and they
are not bounded by the ring size — they wrap at 2³². When `producer` wraps past
`consumer`, `producer - consumer` underflows:

- **debug builds:** panic, `attempt to subtract with overflow`;
- **release builds:** `available` becomes a value near 4.29 billion, so the very
  next line asks for `Vec::with_capacity(~4.29e9)` — a ~34 GB allocation — and
  then attempts that many iterations.

`RxRing::recv`, the other consumer-side ring, already used `wrapping_sub`
correctly. The completion ring did not. Same file, same pattern, one of them
written wrong.

## Why the `size` field was the clue

`FillRing` and `TxRing` — the two **producer**-side rings — read `self.size` to
compute free slots before writing. `RxRing` and `CompletionRing` — the two
**consumer**-side rings — stored `size` and never read it, which is what
clippy reported once `af-xdp` was linted properly.

The dead field was not the bug, but it was pointing straight at it: the
consumer paths had no notion of how large the ring was, and so no notion that
a delta larger than the ring is impossible.

## Reachability

This is not a hostile-input path — the producer index is written by the kernel
— so it needs no attacker. It needs *throughput*. The counter is incremented
once per completed TX frame, so a socket doing 1 Mpps reaches 2³² completions
in roughly 71 minutes; the 920 Kpps figure recorded in CLAUDE.md for this
pipeline puts it in the same range. Wraparound is a routine event for a
long-lived socket, not an edge case.

## Fix

```rust
let available = producer.wrapping_sub(consumer).min(self.size);
...
*self.consumer = consumer.wrapping_add(available);
```

Three changes:

1. **`wrapping_sub`** — matches `RxRing` and is the correct arithmetic for
   free-running counters.
2. **`.min(self.size)`** — a consumer can never legitimately be more than one
   full ring behind. Without it, a desynchronized producer index would re-serve
   stale slots (the `& self.mask` keeps that memory-safe, but the addresses
   handed back to `Umem::free_frame` would be frames that were never completed,
   corrupting the free pool).
3. **`consumer.wrapping_add(available)`** rather than assigning `producer`
   directly, so the consumer advances by exactly what was drained.

The same `.min(self.size)` clamp was added to `RxRing::recv`, which had correct
wrapping arithmetic but no ring-size bound — there, an over-large delta would
have re-served stale descriptors as live packets.

## Test status

`ebpf/af-xdp`: 37 tests pass. **None of them cover this.** The ring types
require a live AF_XDP socket (`CAP_NET_ADMIN`) and mmap'd kernel rings, so the
existing suite exercises construction and validation paths only. A regression
test would need either a fake ring backed by ordinary memory — the ring structs
would have to be constructible from raw pointers for that — or a privileged
integration test. Worth doing; the fix here is reasoned from the arithmetic,
not demonstrated by a failing-then-passing test, and that distinction should
not get lost.
