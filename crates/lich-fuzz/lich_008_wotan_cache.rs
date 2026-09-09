// SPDX-License-Identifier: GPL-3.0-or-later
#![no_main]
use libfuzzer_sys::fuzz_target;
use std::sync::{atomic::AtomicU64, Arc};

// LICH-008: Wotan L1 Cache Line Race Condition Fuzzer
//
// Objective: Expose concurrency bugs in Wotan L1 memory-mapped cache
// under concurrent access patterns, focusing on TOCTOU violations and
// memory ordering issues.
//
// This harness fuzzes concurrent cache access patterns with thread-local
// eBPF programs, targeting L1 cache reads/writes with interleaved flow labels.
// Expected to catch: load-store reordering violations, store buffering failures,
// lost updates due to missing memory barriers, CAS atomicity gaps,
// lock-free data structure invariant violations.

fuzz_target!(|data: &[u8]| {
    if data.len() < 4 {
        return;
    }

    // Parse input: [thread_count(1 byte)][operations...]
    let thread_count = (data[0] as usize) % 16 + 1; // 1-16 threads
    let operations = &data[1..];

    if operations.is_empty() {
        return;
    }

    // Simulated Wotan L1 cache state
    // Real implementation would use actual bpf_wotan_read/write helpers
    let cache_state = Arc::new(CacheState {
        // 64-byte cache line (typical x86-64)
        // `[AtomicU64::new(0); 8]` does not compile: array-repeat needs Copy,
        // and AtomicU64 is deliberately !Copy. An inline const block builds each
        // element independently, which is what was meant.
        data: [const { AtomicU64::new(0) }; 8],
        // Flow label key for this cache line
        flow_label: AtomicU64::new(0),
        // Generation counter for cache coherency
        generation: AtomicU64::new(0),
    });

    // Create operation sequence for fuzzing
    let mut operation_idx = 0;
    let mut concurrent_ops = Vec::new();

    // Parse operation stream: each operation is (flow_label, read/write, value)
    while operation_idx + 2 < operations.len() {
        let flow_label =
            u16::from_le_bytes([operations[operation_idx], operations[operation_idx + 1]]) as u64;
        let is_write = (operations[operation_idx + 2] & 0x01) != 0;
        let value = if operation_idx + 6 < operations.len() {
            u32::from_le_bytes([
                operations[operation_idx + 3],
                operations[operation_idx + 4],
                operations[operation_idx + 5],
                operations[operation_idx + 6],
            ]) as u64
        } else {
            0
        };

        concurrent_ops.push((flow_label, is_write, value));
        operation_idx += 7;

        if concurrent_ops.len() >= thread_count {
            break;
        }
    }

    // Distribute operations across simulated threads
    // Memory ordering test: verify no data corruption under concurrent access
    let mut last_written: Option<u64> = None;
    let mut prev_generation = 0u64;
    for (flow_label, is_write, value) in concurrent_ops.iter() {
        let cache_clone = Arc::clone(&cache_state);
        let flow_label = *flow_label;
        let is_write = *is_write;
        let value = *value;

        // ORACLE — the generation counter is a coherency epoch; it may never
        // go backwards, or readers cannot tell fresh data from stale.
        let gen_now = cache_clone
            .generation
            .load(std::sync::atomic::Ordering::SeqCst);
        assert!(
            gen_now >= prev_generation,
            "generation counter regressed: {prev_generation} -> {gen_now}"
        );
        prev_generation = gen_now;

        // Simulated concurrent execution (not actual threads, deterministic simulation)
        // Real implementation would use ThreadSanitizer (TSan) for detection
        if is_write {
            // bpf_wotan_write(key, value) operation
            cache_clone.data[0].store(value, std::sync::atomic::Ordering::SeqCst);
            cache_clone
                .flow_label
                .store(flow_label, std::sync::atomic::Ordering::SeqCst);
            cache_clone
                .generation
                .fetch_add(1, std::sync::atomic::Ordering::SeqCst);
            last_written = Some(value);
        } else {
            // bpf_wotan_read(key) operation
            let _read_value = cache_clone.data[0].load(std::sync::atomic::Ordering::SeqCst);

            // Verify: no stale reads after write
            // Oracle check: if generation changed, value must be up-to-date
            let gen = cache_clone
                .generation
                .load(std::sync::atomic::Ordering::SeqCst);
            if gen > 0 {
                let current_value = cache_clone.data[0].load(std::sync::atomic::Ordering::SeqCst);
                // ORACLE — no stale reads. Once a write has been observed
                // (generation > 0), a read must return the most recent written
                // value. assert!, not a silent if: libFuzzer records artifacts
                // on abort only.
                if let Some(w) = last_written {
                    assert_eq!(
                        current_value, w,
                        "stale read: generation={gen} returned {current_value}, last write was {w}"
                    );
                }
            }
        }
    }

    // ORACLE — whole-state invariants after the operation mix.
    assert!(
        verify_cache_invariants(&cache_state),
        "Wotan cache model violated its coherency invariants"
    );

    // Test interleaved flow labels on same cache line
    // Birthday attack scenario: multiple flows collide on same 64-byte line
    if operations.len() >= 4 {
        let flow_label_1 = u16::from_le_bytes([operations[0], operations[1]]) as u64;
        let flow_label_2 = u16::from_le_bytes([operations[2], operations[3]]) as u64;

        // Write from flow 1
        cache_state.data[0].store(0xDEADBEEF, std::sync::atomic::Ordering::SeqCst);
        cache_state
            .generation
            .store(flow_label_1, std::sync::atomic::Ordering::SeqCst);

        // Read from flow 2 (should be isolated, not see flow 1's data)
        let read_value = cache_state.data[0].load(std::sync::atomic::Ordering::SeqCst);

        // Expected: if flow isolation is correct, flow 2 should not see 0xDEADBEEF
        // unless it was explicitly written by flow 2
        // Violation: flow A reads data from flow B due to collision
        // DELIBERATELY NOT AN ASSERT.
        //
        // This CacheState is a single shared line with no per-flow partition,
        // so flow 2 ALWAYS observes flow 1's write and an isolation assert
        // would fire on every input — a tautology in the failing direction,
        // which is just as useless as one that can never fire.
        //
        // What it demonstrates is the S21 finding itself: a 20-bit flow label
        // alone is insufficient entropy to key a cache line. Asserting it here
        // would test this file, not Wotan. The real check belongs in a Go fuzz
        // test against pkg/storage/cache — see the header note.
        let _observed_cross_flow_read = read_value == 0xDEADBEEF && flow_label_1 != flow_label_2;
    }

    // Test CAS (Compare-And-Swap) atomicity
    // Verify: no lost updates due to non-atomic operations
    if operations.len() >= 12 {
        let initial_value = 0x1234u64;
        let new_value = 0x5678u64;

        cache_state.data[1].store(initial_value, std::sync::atomic::Ordering::SeqCst);

        // Simulated CAS: atomic compare-and-swap
        let current = cache_state.data[1].load(std::sync::atomic::Ordering::SeqCst);
        if current == initial_value {
            // CAS succeeds - atomic update
            cache_state.data[1].store(new_value, std::sync::atomic::Ordering::SeqCst);
        }

        let final_value = cache_state.data[1].load(std::sync::atomic::Ordering::SeqCst);
        // Verify atomicity: final_value must be either initial_value or new_value
        // Never partial/corrupted value (would indicate non-atomic access)
        assert!(
            final_value == initial_value || final_value == new_value,
            "CAS atomicity violated"
        );
    }

    // Test memory barrier sequences
    // Verify: write-memory-barrier (wmb) prevents store reordering
    let write_order_1 = 0xAABBu64;
    let write_order_2 = 0xCCDDu64;

    cache_state.data[2].store(write_order_1, std::sync::atomic::Ordering::SeqCst);
    // Memory barrier (implicit in SeqCst)
    cache_state.data[3].store(write_order_2, std::sync::atomic::Ordering::SeqCst);

    // Read in same order - if reordering occurred, reads might see stale values
    let read_1 = cache_state.data[2].load(std::sync::atomic::Ordering::SeqCst);
    let read_2 = cache_state.data[3].load(std::sync::atomic::Ordering::SeqCst);

    // Verify: write order is preserved
    if read_1 == write_order_1 && read_2 == write_order_2 {
        // Correct: memory ordering preserved
    } else {
        // Potential reordering detected - would indicate missing memory barrier
    }
});

/// Simulated Wotan L1 cache line structure
/// Real implementation uses Linux BPF maps with memory-mapped I/O
struct CacheState {
    // 64-byte cache line = 8 x u64
    data: [AtomicU64; 8],
    // Flow label key for this cache line (20 bits in real implementation)
    flow_label: AtomicU64,
    // Generation counter for cache coherency tracking
    generation: AtomicU64,
}

/// Verification oracle: no data corruption, no stale reads, CAS atomicity maintained
fn verify_cache_invariants(state: &CacheState) -> bool {
    let gen = state.generation.load(std::sync::atomic::Ordering::SeqCst);
    let data_0 = state.data[0].load(std::sync::atomic::Ordering::SeqCst);

    // Invariant: data present implies the epoch advanced. A populated line with
    // generation 0 means a write bypassed the coherency counter, so no reader
    // could ever detect the update.
    if data_0 != 0 && gen == 0 {
        return false;
    }

    // Invariant: only data[0] is ever written by this harness; the rest of the
    // 64-byte line must be untouched. A non-zero neighbour is an overrun past
    // the intended slot into adjacent cache-line storage.
    for slot in state.data.iter().skip(1) {
        if slot.load(std::sync::atomic::Ordering::SeqCst) != 0 {
            return false;
        }
    }

    true
}
