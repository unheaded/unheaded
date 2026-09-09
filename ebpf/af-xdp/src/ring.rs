// SPDX-License-Identifier: GPL-2.0-only
// Unheaded Kingdom — Lock-free ring buffer for AF_XDP
//
// Generic SPSC (single-producer single-consumer) lock-free ring buffer.
// Used by the XDP engine for fill/completion/rx/tx ring management.
//
// Memory ordering strategy:
// - Acquire on reads: producer reads cons, consumer reads prod
//   ensures we see updates from the other end before proceeding
// - Release on writes: producer writes prod, consumer writes cons
//   ensures our updates are visible before the other end reads
// - Relaxed on "own" index reads (producer reads prod, consumer reads cons)
//   because only we write our own index
//
// This maintains safe concurrent access without locks (SPSC pattern).

use std::sync::atomic::{AtomicU32, Ordering};

/// Lock-free SPSC ring buffer for AF_XDP fill/completion/rx/tx rings.
///
/// Single producer, single consumer. No locks needed.
/// Producer calls `reserve()` -> write entries via `set()` -> `submit()`
/// Consumer calls `peek()` -> read entries via `get()` -> `release()`
///
/// # Safety
/// - Assumes single-threaded access per role (one producer, one consumer)
/// - UMEM frames must not be double-freed
/// - Assumes power-of-2 size for efficient wrap-around via bitmask
pub struct Ring<T: Copy> {
    entries: *mut T,
    size: u32,
    mask: u32,
    prod: *const AtomicU32,
    cons: *const AtomicU32,
    cached_prod: u32,
    cached_cons: u32,
    pub prod_failures: u64,
    pub cons_failures: u64,
}

// Ring does not use thread-local storage and the raw pointers point
// to shared mmap'd or heap memory.  The caller is responsible for
// ensuring that only one producer and one consumer access the ring.
unsafe impl<T: Copy> Send for Ring<T> {}

impl<T: Copy> Ring<T> {
    /// Create a ring from pre-allocated memory.
    ///
    /// # Safety
    /// - `entries` must point to a valid array of `size` elements
    /// - `prod` and `cons` must point to valid AtomicU32 instances
    /// - `size` must be a power of 2
    /// - Pointers must remain valid for the lifetime of the Ring
    pub unsafe fn new(
        entries: *mut T,
        size: u32,
        prod: *const AtomicU32,
        cons: *const AtomicU32,
    ) -> Self {
        debug_assert!(size > 0 && size.is_power_of_two());

        Ring {
            entries,
            size,
            mask: size - 1,
            prod,
            cons,
            cached_prod: 0,
            cached_cons: 0,
            prod_failures: 0,
            cons_failures: 0,
        }
    }

    // ── Producer operations ──────────────────────────────────────────────

    /// Reserve space for `n` entries.
    /// Returns (producer_index, available_slots).
    pub fn reserve(&mut self, _n: u32) -> (u32, u32) {
        let prod = unsafe { (*self.prod).load(Ordering::Relaxed) };
        self.cached_cons = unsafe { (*self.cons).load(Ordering::Acquire) };
        let available = self.size.wrapping_sub(prod.wrapping_sub(self.cached_cons));
        (prod, available)
    }

    /// Submit `n` entries: advance producer index.
    pub fn submit(&mut self, n: u32) {
        let prod = unsafe { (*self.prod).load(Ordering::Relaxed) };
        unsafe { (*self.prod).store(prod.wrapping_add(n), Ordering::Release) };
    }

    /// Reserve with explicit error if insufficient space.
    pub fn try_reserve(&mut self, n: u32) -> Result<u32, &'static str> {
        let (idx, avail) = self.reserve(n);
        if avail >= n {
            Ok(idx)
        } else {
            self.prod_failures += 1;
            Err("Ring full")
        }
    }

    /// Push a single entry (reserve + write + submit).
    pub fn push(&mut self, entry: T) -> bool {
        let (idx, avail) = self.reserve(1);
        if avail > 0 {
            unsafe { self.set(idx, entry) };
            self.submit(1);
            true
        } else {
            self.prod_failures += 1;
            false
        }
    }

    /// Reserve batch for producer.
    /// Returns (start_index, batch_size, wrapped).
    /// `wrapped` is true if the batch crosses the ring boundary.
    pub fn reserve_batch(&mut self, n: u32) -> (u32, u32, bool) {
        let (idx, avail) = self.reserve(n);
        let batch = core::cmp::min(n, avail);
        let start = idx & self.mask;
        let wrapped = (start + batch) > self.size;
        (idx, batch, wrapped)
    }

    // ── Consumer operations ──────────────────────────────────────────────

    /// Peek at available entries for consumption.
    /// Returns (consumer_index, count_available).
    pub fn peek(&mut self) -> (u32, u32) {
        let cons = unsafe { (*self.cons).load(Ordering::Relaxed) };
        self.cached_prod = unsafe { (*self.prod).load(Ordering::Acquire) };
        let available = self.cached_prod.wrapping_sub(cons);
        (cons, available)
    }

    /// Release `n` consumed entries: advance consumer index.
    pub fn release(&mut self, n: u32) {
        let cons = unsafe { (*self.cons).load(Ordering::Relaxed) };
        unsafe { (*self.cons).store(cons.wrapping_add(n), Ordering::Release) };
    }

    /// Pop a single entry (peek + read + release).
    pub fn pop(&mut self) -> Option<T> {
        let (idx, count) = self.peek();
        if count > 0 {
            let entry = unsafe { self.get(idx) };
            self.release(1);
            Some(entry)
        } else {
            self.cons_failures += 1;
            None
        }
    }

    /// Try to pop with Result.
    pub fn try_pop(&mut self) -> Result<T, &'static str> {
        let (idx, count) = self.peek();
        if count > 0 {
            let entry = unsafe { self.get(idx) };
            self.release(1);
            Ok(entry)
        } else {
            self.cons_failures += 1;
            Err("Ring empty")
        }
    }

    /// Peek batch for consumer.
    /// Returns (start_index, batch_size).
    pub fn peek_batch(&mut self, n: u32) -> (u32, u32) {
        let (idx, count) = self.peek();
        let batch = core::cmp::min(n, count);
        (idx, batch)
    }

    // ── Entry access ─────────────────────────────────────────────────────

    /// Read entry at index (wraps via mask).
    ///
    /// # Safety
    /// Caller must ensure `idx` came from reserve/peek and entries ptr is valid.
    #[inline]
    pub unsafe fn get(&self, idx: u32) -> T {
        *self.entries.add((idx & self.mask) as usize)
    }

    /// Write entry at index (wraps via mask).
    ///
    /// # Safety
    /// Caller must ensure `idx` came from reserve and entries ptr is valid.
    #[inline]
    pub unsafe fn set(&mut self, idx: u32, val: T) {
        *self.entries.add((idx & self.mask) as usize) = val;
    }

    // ── Introspection ────────────────────────────────────────────────────

    /// Ring capacity (number of entries).
    #[inline]
    pub fn capacity(&self) -> u32 {
        self.size
    }
}

// ── Tests ────────────────────────────────────────────────────────────────────

#[cfg(test)]
mod tests {
    use super::*;
    use std::sync::atomic::AtomicU32;

    /// Helper: create a test ring backed by a Vec.
    /// Returns (Ring, backing_vec, prod_atomic, cons_atomic).
    fn make_ring(size: u32) -> (Ring<u64>, Vec<u64>, Box<AtomicU32>, Box<AtomicU32>) {
        let mut entries = vec![0u64; size as usize];
        let prod = Box::new(AtomicU32::new(0));
        let cons = Box::new(AtomicU32::new(0));

        let ring = unsafe {
            Ring::new(
                entries.as_mut_ptr(),
                size,
                &*prod as *const AtomicU32,
                &*cons as *const AtomicU32,
            )
        };

        (ring, entries, prod, cons)
    }

    #[test]
    fn test_push_pop_single() {
        let (mut ring, _entries, _prod, _cons) = make_ring(4);

        assert!(ring.push(42));
        assert_eq!(ring.pop(), Some(42));
        assert_eq!(ring.pop(), None);
    }

    #[test]
    fn test_push_pop_fill() {
        let (mut ring, _e, _p, _c) = make_ring(4);

        assert!(ring.push(1));
        assert!(ring.push(2));
        assert!(ring.push(3));
        assert!(ring.push(4));
        // Ring full
        assert!(!ring.push(5));
        assert_eq!(ring.prod_failures, 1);

        assert_eq!(ring.pop(), Some(1));
        assert_eq!(ring.pop(), Some(2));
        assert_eq!(ring.pop(), Some(3));
        assert_eq!(ring.pop(), Some(4));
        assert_eq!(ring.pop(), None);
    }

    #[test]
    fn test_try_reserve_ok() {
        let (mut ring, _e, _p, _c) = make_ring(8);
        let idx = ring.try_reserve(4);
        assert!(idx.is_ok());
    }

    #[test]
    fn test_try_reserve_full() {
        let (mut ring, _e, _p, _c) = make_ring(4);
        // Fill up
        for i in 0..4 {
            ring.push(i);
        }
        assert!(ring.try_reserve(1).is_err());
    }

    #[test]
    fn test_try_pop_empty() {
        let (mut ring, _e, _p, _c) = make_ring(4);
        assert!(ring.try_pop().is_err());
        assert_eq!(ring.cons_failures, 1);
    }

    #[test]
    fn test_wrap_around() {
        let (mut ring, _e, _p, _c) = make_ring(4);

        // Fill and drain twice to test wrapping
        for round in 0..3 {
            for i in 0..4 {
                assert!(ring.push(round * 10 + i));
            }
            for i in 0..4 {
                assert_eq!(ring.pop(), Some(round * 10 + i));
            }
        }
    }

    #[test]
    fn test_reserve_batch() {
        let (mut ring, _e, _p, _c) = make_ring(8);

        let (idx, batch, _wrapped) = ring.reserve_batch(4);
        assert_eq!(idx, 0);
        assert_eq!(batch, 4);

        // Write entries
        for i in 0..4 {
            unsafe { ring.set(idx + i, (i + 1) as u64) };
        }
        ring.submit(4);

        // Read them back
        let (cidx, count) = ring.peek_batch(4);
        assert_eq!(count, 4);
        for i in 0..4 {
            assert_eq!(unsafe { ring.get(cidx + i) }, (i + 1) as u64);
        }
        ring.release(4);
    }

    #[test]
    fn test_capacity() {
        let (ring, _e, _p, _c) = make_ring(16);
        assert_eq!(ring.capacity(), 16);
    }

    #[test]
    fn test_stats_increment() {
        let (mut ring, _e, _p, _c) = make_ring(2);
        ring.push(1);
        ring.push(2);
        ring.push(3); // fails
        assert_eq!(ring.prod_failures, 1);

        // drain
        ring.pop();
        ring.pop();
        ring.pop(); // fails
        assert_eq!(ring.cons_failures, 1);
    }
}
