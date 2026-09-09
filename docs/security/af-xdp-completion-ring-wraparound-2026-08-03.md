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

## Test status — closed 2026-08-03

**Covered.** Three regression tests in `ebpf/af-xdp/src/umem.rs`, taking the
fake-ring route this section proposed: `FillRing` and `CompletionRing` gained
`#[cfg(test)] unsafe fn from_raw_for_test`, so a ring can be driven over
ordinary heap memory with no AF_XDP socket and no `CAP_NET_ADMIN`. The
privileged-integration-test alternative was rejected for the obvious reason —
it would be skipped in exactly the environment meant to catch the regression.

  - `completion_ring_consume_survives_counter_wraparound`
  - `fill_ring_free_slots_survives_counter_wraparound`
  - `fill_ring_free_slots_never_exceeds_size`

Each was verified red-then-green: with the wrapping arithmetic reverted, all
three panic with "attempt to subtract with overflow"; with it restored, the
suite is 34 pass / 0 fail.

## A second, unfixed instance of the same bug — found 2026-08-03

Writing the test surfaced that **`FillRing` had never been fixed.** The
2026-08-03 work corrected `CompletionRing::consume`; `xsk.rs` and `ring.rs` had
always used the wrapping form; `FillRing::produce` and `FillRing::free_slots`
were the last plain-arithmetic ring in the tree, in four places.

This instance is arguably worse than the one originally reported. In
`CompletionRing` a bad delta produced an over-large `Vec::with_capacity`. In
`FillRing` the bad delta lands in `free_slots`, which is the *only* thing
stopping userspace from producing into slots the kernel has not consumed yet:

```rust
let free_slots = self.size - (producer - consumer);   // both subtractions underflow
let count = (addrs.len() as u32).min(free_slots);     // bound is now meaningless
```

A wrong `free_slots` does not merely allocate badly — it hands the kernel frame
addresses that are still in flight. Fixed to match `xsk.rs`'s long-standing
form, and `fill_ring_free_slots_never_exceeds_size` now asserts the invariant
directly across four desynchronized producer/consumer pairs.
