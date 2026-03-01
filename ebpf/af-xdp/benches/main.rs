// SPDX-License-Identifier: GPL-2.0-only
// Unheaded Kingdom — AF_XDP Benchmark Suite Entry Point
//
// Runs all benchmarks, outputs JSON to stdout and markdown to stderr.
//
// Usage:
//   cargo build --bin af-xdp-bench
//   ./target/debug/af-xdp-bench              # run all benchmarks
//   ./target/debug/af-xdp-bench 2>/dev/null  # JSON only
//   ./target/debug/af-xdp-bench 1>/dev/null  # human-readable only
//
// Sacred Law: NO external deps. Rust std only.

mod harness;

use harness::{
    measure_cpu, print_cpu_summary, print_markdown_table, results_to_json, BenchResult, BenchRunner,
};
use std::collections::HashMap;
use std::hint::black_box;
use std::sync::atomic::{AtomicU32, Ordering};
use std::time::Duration;

// =============================================================================
// UMEM simulation — userspace-only frame allocator (no kernel required)
// =============================================================================

/// Simulated UMEM for benchmarking: mmap-backed frame pool with LIFO allocator.
/// Mirrors the real Umem struct's alloc/free path without requiring AF_XDP socket.
struct BenchUmem {
    /// Base address of allocated region
    base: *mut u8,
    /// Total region size
    size: u64,
    /// Frame size in bytes
    frame_size: u32,
    /// Free frame offsets (LIFO stack)
    free_frames: Vec<u64>,
}

impl BenchUmem {
    fn new(frame_size: u32, frame_count: u32) -> Self {
        let size = (frame_size as u64) * (frame_count as u64);
        // Use anonymous mmap just like the real Umem
        let base = unsafe {
            let ptr = libc_mmap(size);
            assert!(!ptr.is_null(), "mmap failed in BenchUmem");
            ptr
        };

        let mut free_frames = Vec::with_capacity(frame_count as usize);
        for i in 0..frame_count {
            free_frames.push((i as u64) * (frame_size as u64));
        }

        BenchUmem {
            base,
            size,
            frame_size,
            free_frames,
        }
    }

    #[inline]
    fn alloc_frame(&mut self) -> Option<u64> {
        self.free_frames.pop()
    }

    #[inline]
    fn free_frame(&mut self, offset: u64) {
        if offset < self.size {
            self.free_frames.push(offset);
        }
    }

    #[inline]
    fn frame_ptr(&self, offset: u64) -> *mut u8 {
        unsafe { self.base.add(offset as usize) }
    }

    fn free_count(&self) -> usize {
        self.free_frames.len()
    }
}

impl Drop for BenchUmem {
    fn drop(&mut self) {
        unsafe {
            libc_munmap(self.base, self.size);
        }
    }
}

/// Raw mmap via syscall (same pattern as crate::syscall::mmap but standalone).
unsafe fn libc_mmap(size: u64) -> *mut u8 {
    let ret: i64;
    std::arch::asm!(
        "syscall",
        inlateout("rax") 9_i64 => ret,          // SYS_mmap
        in("rdi") 0_u64,                         // addr = NULL
        in("rsi") size,                           // length
        in("rdx") 3_u64,                          // PROT_READ | PROT_WRITE
        in("r10") 0x22_u64,                       // MAP_PRIVATE | MAP_ANONYMOUS
        in("r8") u64::MAX,                        // fd = -1
        in("r9") 0_u64,                           // offset = 0
        lateout("rcx") _,
        lateout("r11") _,
        options(nostack),
    );
    if ret < 0 {
        std::ptr::null_mut()
    } else {
        ret as *mut u8
    }
}

unsafe fn libc_munmap(ptr: *mut u8, size: u64) {
    let _ret: i64;
    std::arch::asm!(
        "syscall",
        inlateout("rax") 11_i64 => _ret,         // SYS_munmap
        in("rdi") ptr as u64,
        in("rsi") size,
        lateout("rdx") _,
        lateout("rcx") _,
        lateout("r11") _,
        options(nostack),
    );
}

// =============================================================================
// Simulated lock-free ring buffer (mirrors crate::ring::Ring)
// =============================================================================

/// Userspace-only ring buffer for benchmarking.
/// Same SPSC lock-free design as the real Ring<T>, backed by heap memory.
struct BenchRing {
    entries: Vec<u64>,
    size: u32,
    mask: u32,
    prod: Box<AtomicU32>,
    cons: Box<AtomicU32>,
}

impl BenchRing {
    fn new(size: u32) -> Self {
        assert!(size > 0 && size.is_power_of_two());
        BenchRing {
            entries: vec![0u64; size as usize],
            size,
            mask: size - 1,
            prod: Box::new(AtomicU32::new(0)),
            cons: Box::new(AtomicU32::new(0)),
        }
    }

    /// Push a single entry. Returns true on success, false if ring is full.
    #[inline]
    fn push(&mut self, val: u64) -> bool {
        let prod = self.prod.load(Ordering::Relaxed);
        let cons = self.cons.load(Ordering::Acquire);
        let free = self.size.wrapping_sub(prod.wrapping_sub(cons));
        if free == 0 {
            return false;
        }
        let idx = (prod & self.mask) as usize;
        self.entries[idx] = val;
        self.prod.store(prod.wrapping_add(1), Ordering::Release);
        true
    }

    /// Pop a single entry. Returns None if ring is empty.
    #[inline]
    fn pop(&mut self) -> Option<u64> {
        let cons = self.cons.load(Ordering::Relaxed);
        let prod = self.prod.load(Ordering::Acquire);
        let avail = prod.wrapping_sub(cons);
        if avail == 0 {
            return None;
        }
        let idx = (cons & self.mask) as usize;
        let val = self.entries[idx];
        self.cons.store(cons.wrapping_add(1), Ordering::Release);
        Some(val)
    }

    /// Push a batch of entries. Returns how many were pushed.
    #[inline]
    fn push_batch(&mut self, vals: &[u64]) -> u32 {
        let prod = self.prod.load(Ordering::Relaxed);
        let cons = self.cons.load(Ordering::Acquire);
        let free = self.size.wrapping_sub(prod.wrapping_sub(cons));
        let count = (vals.len() as u32).min(free);
        for i in 0..count {
            let idx = ((prod.wrapping_add(i)) & self.mask) as usize;
            self.entries[idx] = vals[i as usize];
        }
        self.prod.store(prod.wrapping_add(count), Ordering::Release);
        count
    }

    /// Pop a batch of entries. Returns the entries popped.
    #[inline]
    fn pop_batch(&mut self, max: u32) -> Vec<u64> {
        let cons = self.cons.load(Ordering::Relaxed);
        let prod = self.prod.load(Ordering::Acquire);
        let avail = prod.wrapping_sub(cons);
        let count = max.min(avail);
        let mut out = Vec::with_capacity(count as usize);
        for i in 0..count {
            let idx = ((cons.wrapping_add(i)) & self.mask) as usize;
            out.push(self.entries[idx]);
        }
        self.cons.store(cons.wrapping_add(count), Ordering::Release);
        out
    }

    /// Reset the ring to empty state.
    fn reset(&mut self) {
        self.prod.store(0, Ordering::Relaxed);
        self.cons.store(0, Ordering::Relaxed);
    }
}

// =============================================================================
// Benchmark implementations
// =============================================================================

/// Benchmark 1: UMEM frame alloc/free throughput.
/// Allocates 1000 frames, frees them, measures frames/sec.
fn bench_umem_alloc_free() -> (BenchResult, Duration, Duration) {
    const FRAMES: u32 = 1000;
    const ITERS: u32 = 5000;

    let (result, user, sys) = measure_cpu(|| {
        let mut runner = BenchRunner::new("umem_alloc_free_1000", ITERS);
        let mut umem = BenchUmem::new(4096, FRAMES);

        runner.run(|_i| {
            // Allocate all frames
            let mut addrs = Vec::with_capacity(FRAMES as usize);
            for _ in 0..FRAMES {
                if let Some(addr) = umem.alloc_frame() {
                    addrs.push(addr);
                }
            }
            black_box(&addrs);

            // Free all frames
            for addr in addrs {
                umem.free_frame(addr);
            }
        });

        runner.report("frames/sec", FRAMES as u64 * 2) // alloc + free = 2 ops per frame
    });

    print_cpu_summary("umem_alloc_free", user, sys);
    (result, user, sys)
}

/// Benchmark 2: Ring buffer produce/consume at various batch sizes.
fn bench_ring_produce_consume() -> Vec<(BenchResult, Duration, Duration)> {
    const TOTAL_EVENTS: u32 = 10_000;
    const RING_SIZE: u32 = 16384; // power of 2, large enough for 10k
    const ITERS: u32 = 1000;
    let batch_sizes: &[u32] = &[1, 4, 16, 64, 256];

    let mut results = Vec::new();

    for &batch in batch_sizes {
        let bench_name: &'static str = match batch {
            1 => "ring_produce_consume_batch_1",
            4 => "ring_produce_consume_batch_4",
            16 => "ring_produce_consume_batch_16",
            64 => "ring_produce_consume_batch_64",
            256 => "ring_produce_consume_batch_256",
            _ => unreachable!(),
        };

        let (result, user, sys) = measure_cpu(|| {
            let mut runner = BenchRunner::new(bench_name, ITERS);
            let mut ring = BenchRing::new(RING_SIZE);

            // Pre-generate batch data
            let batch_data: Vec<u64> = (0..batch as u64).collect();

            runner.run(|_i| {
                ring.reset();
                let mut produced = 0u32;
                let mut consumed = 0u32;

                // Produce TOTAL_EVENTS in batches
                while produced < TOTAL_EVENTS {
                    let remaining = TOTAL_EVENTS - produced;
                    let to_push = remaining.min(batch);
                    let data = &batch_data[..to_push as usize];
                    let pushed = ring.push_batch(data);
                    produced += pushed;

                    // Consume to make room
                    let popped = ring.pop_batch(pushed);
                    consumed += popped.len() as u32;
                    black_box(&popped);
                }

                // Drain remaining
                while consumed < produced {
                    if let Some(val) = ring.pop() {
                        consumed += 1;
                        black_box(val);
                    } else {
                        break;
                    }
                }
            });

            runner.report("ops/sec", TOTAL_EVENTS as u64 * 2) // produce + consume
        });

        print_cpu_summary(bench_name, user, sys);
        results.push((result, user, sys));
    }

    results
}

/// Benchmark 3: End-to-end packet RX latency simulation.
/// Measures the userspace ring + UMEM path latency without kernel AF_XDP.
fn bench_e2e_packet_rx_latency() -> (BenchResult, Duration, Duration) {
    const FRAME_COUNT: u32 = 4096;
    const RING_SIZE: u32 = 4096;
    const ITERS: u32 = 10_000;

    let (result, user, sys) = measure_cpu(|| {
        let mut runner = BenchRunner::new("e2e_packet_rx_latency", ITERS);
        let mut umem = BenchUmem::new(4096, FRAME_COUNT);
        let mut ring = BenchRing::new(RING_SIZE);

        runner.run(|_i| {
            // Simulate RX path:
            // 1. Alloc frame from UMEM
            let addr = umem.alloc_frame().expect("UMEM OOM in bench");

            // 2. Write simulated packet data (64-byte Ethernet frame)
            let ptr = umem.frame_ptr(addr);
            unsafe {
                // Fake ethernet header (14 bytes) + IP header (20 bytes) + payload
                let slice = std::slice::from_raw_parts_mut(ptr, 64);
                for (i, b) in slice.iter_mut().enumerate() {
                    *b = i as u8;
                }
            }

            // 3. Push descriptor to ring (simulates kernel producing RX desc)
            let pushed = ring.push(addr | ((64u64) << 48)); // encode len in upper bits
            black_box(pushed);

            // 4. Pop from ring (simulates userspace consuming RX desc)
            let desc = ring.pop().expect("ring empty after push");
            let rx_addr = desc & 0x0000_FFFF_FFFF_FFFF;
            let rx_len = (desc >> 48) as usize;
            black_box(rx_len);

            // 5. Read packet data from UMEM
            let rx_ptr = umem.frame_ptr(rx_addr);
            unsafe {
                let slice = std::slice::from_raw_parts(rx_ptr, rx_len);
                black_box(slice[0]);
            }

            // 6. Free frame back to UMEM
            umem.free_frame(rx_addr);
        });

        runner.report("packets/sec", 1)
    });

    print_cpu_summary("e2e_packet_rx_latency", user, sys);
    (result, user, sys)
}

/// Benchmark 4: Batch size sweep — throughput and latency at each batch size.
fn bench_batch_size_sweep() -> Vec<(BenchResult, Duration, Duration)> {
    const FRAME_COUNT: u32 = 8192;
    const RING_SIZE: u32 = 8192;
    const ITERS: u32 = 2000;
    let batch_sizes: &[u32] = &[1, 2, 4, 8, 16, 32, 64, 128, 256];

    let mut results = Vec::new();

    for &batch in batch_sizes {
        let bench_name: &'static str = match batch {
            1 => "batch_sweep_1",
            2 => "batch_sweep_2",
            4 => "batch_sweep_4",
            8 => "batch_sweep_8",
            16 => "batch_sweep_16",
            32 => "batch_sweep_32",
            64 => "batch_sweep_64",
            128 => "batch_sweep_128",
            256 => "batch_sweep_256",
            _ => unreachable!(),
        };

        let (result, user, sys) = measure_cpu(|| {
            let mut runner = BenchRunner::new(bench_name, ITERS);
            let mut umem = BenchUmem::new(4096, FRAME_COUNT);
            let mut ring = BenchRing::new(RING_SIZE);

            runner.run(|_i| {
                // Allocate a batch of frames
                let mut addrs = Vec::with_capacity(batch as usize);
                for _ in 0..batch {
                    if let Some(addr) = umem.alloc_frame() {
                        addrs.push(addr);
                    } else {
                        break;
                    }
                }

                // Write simulated packet data to each frame
                for &addr in &addrs {
                    let ptr = umem.frame_ptr(addr);
                    unsafe {
                        let slice = std::slice::from_raw_parts_mut(ptr, 64);
                        for (i, b) in slice.iter_mut().enumerate() {
                            *b = (i & 0xFF) as u8;
                        }
                    }
                }

                // Push to ring as batch (encode len in upper bits)
                let descs: Vec<u64> = addrs.iter().map(|&a| a | (64u64 << 48)).collect();
                let pushed = ring.push_batch(&descs);
                black_box(pushed);

                // Pop batch from ring
                let popped = ring.pop_batch(pushed);

                // Read packet data + free frames
                for desc in &popped {
                    let addr = desc & 0x0000_FFFF_FFFF_FFFF;
                    let len = (*desc >> 48) as usize;
                    let ptr = umem.frame_ptr(addr);
                    unsafe {
                        let slice = std::slice::from_raw_parts(ptr, len);
                        black_box(slice[0]);
                    }
                    umem.free_frame(addr);
                }

                // Free any frames that didn't fit in the ring
                for &addr in &addrs[pushed as usize..] {
                    umem.free_frame(addr);
                }
            });

            runner.report("packets/sec", batch as u64)
        });

        print_cpu_summary(bench_name, user, sys);
        results.push((result, user, sys));
    }

    results
}

/// Benchmark 5: Memory bandwidth — measure bytes/sec through UMEM + ring path.
fn bench_memory_bandwidth() -> (BenchResult, Duration, Duration) {
    const FRAME_SIZE: u32 = 4096;
    const FRAME_COUNT: u32 = 4096;
    const RING_SIZE: u32 = 4096;
    const PACKET_SIZE: usize = 1500; // MTU-sized packets
    const ITERS: u32 = 5000;
    const BATCH: u32 = 64;

    let (result, user, sys) = measure_cpu(|| {
        let mut runner = BenchRunner::new("memory_bandwidth_1500B_batch64", ITERS);
        let mut umem = BenchUmem::new(FRAME_SIZE, FRAME_COUNT);
        let mut ring = BenchRing::new(RING_SIZE);

        // Pre-generate packet payload
        let payload: Vec<u8> = (0..PACKET_SIZE).map(|i| (i & 0xFF) as u8).collect();

        runner.run(|_i| {
            let mut total_bytes = 0usize;

            // Allocate batch of frames
            let mut addrs = Vec::with_capacity(BATCH as usize);
            for _ in 0..BATCH {
                if let Some(addr) = umem.alloc_frame() {
                    addrs.push(addr);
                } else {
                    break;
                }
            }

            // Write MTU-sized payload to each frame
            for &addr in &addrs {
                let ptr = umem.frame_ptr(addr);
                unsafe {
                    std::ptr::copy_nonoverlapping(payload.as_ptr(), ptr, PACKET_SIZE);
                }
                total_bytes += PACKET_SIZE;
            }

            // Push descriptors through ring
            let descs: Vec<u64> = addrs
                .iter()
                .map(|&a| a | ((PACKET_SIZE as u64) << 48))
                .collect();
            let pushed = ring.push_batch(&descs);

            // Pop and read back
            let popped = ring.pop_batch(pushed);
            for desc in &popped {
                let addr = desc & 0x0000_FFFF_FFFF_FFFF;
                let len = (*desc >> 48) as usize;
                let ptr = umem.frame_ptr(addr);
                unsafe {
                    let slice = std::slice::from_raw_parts(ptr, len);
                    // Touch the data to force cache effects
                    black_box(slice[0]);
                    black_box(slice[len / 2]);
                    black_box(slice[len - 1]);
                }
                total_bytes += len;
                umem.free_frame(addr);
            }

            // Free overflow
            for &addr in &addrs[pushed as usize..] {
                umem.free_frame(addr);
            }

            black_box(total_bytes);
        });

        // Each iteration: BATCH frames * PACKET_SIZE bytes * 2 (write + read)
        let bytes_per_iter = (BATCH as u64) * (PACKET_SIZE as u64) * 2;
        runner.report("bytes/sec", bytes_per_iter)
    });

    print_cpu_summary("memory_bandwidth", user, sys);
    (result, user, sys)
}

/// Benchmark 6: Simulated BPF map lookup latency (hash map lookup cost).
fn bench_bpf_map_lookup() -> (BenchResult, Duration, Duration) {
    const MAP_SIZE: usize = 4096;
    const ITERS: u32 = 100_000;

    let (result, user, sys) = measure_cpu(|| {
        let mut runner = BenchRunner::new("bpf_map_lookup_sim", ITERS);

        // Build a HashMap simulating BPF_MAP_TYPE_HASH
        // In real BPF, this is a kernel hash table; we simulate the lookup cost.
        let mut map: HashMap<u32, u64> = HashMap::with_capacity(MAP_SIZE);
        for i in 0..MAP_SIZE as u32 {
            map.insert(i, (i as u64) * 0xDEAD_BEEF);
        }

        runner.run(|i| {
            let key = i % (MAP_SIZE as u32);
            let val = map.get(&key);
            black_box(val);
        });

        runner.report("lookups/sec", 1)
    });

    print_cpu_summary("bpf_map_lookup_sim", user, sys);
    (result, user, sys)
}

/// Benchmark 7: Ring buffer vs UMEM comparison matrix.
/// Isolates ring buffer operations from UMEM operations for direct comparison.
fn bench_comparison_matrix() -> Vec<(BenchResult, Duration, Duration)> {
    const ITERS: u32 = 50_000;
    let mut results = Vec::new();

    // --- Pure ring push/pop (no UMEM involvement) ---
    {
        let (result, user, sys) = measure_cpu(|| {
            let mut runner = BenchRunner::new("ring_only_push_pop", ITERS);
            let mut ring = BenchRing::new(1024);

            runner.run(|i| {
                let pushed = ring.push(i as u64);
                black_box(pushed);
                let popped = ring.pop();
                black_box(popped);
            });

            runner.report("ops/sec", 2) // push + pop
        });

        print_cpu_summary("ring_only_push_pop", user, sys);
        results.push((result, user, sys));
    }

    // --- Pure UMEM alloc/free (no ring involvement) ---
    {
        let (result, user, sys) = measure_cpu(|| {
            let mut runner = BenchRunner::new("umem_only_alloc_free", ITERS);
            let mut umem = BenchUmem::new(4096, 1024);

            runner.run(|_i| {
                let addr = umem.alloc_frame();
                black_box(&addr);
                if let Some(a) = addr {
                    umem.free_frame(a);
                }
            });

            runner.report("ops/sec", 2) // alloc + free
        });

        print_cpu_summary("umem_only_alloc_free", user, sys);
        results.push((result, user, sys));
    }

    // --- UMEM alloc + write + ring push/pop + read + free (combined path) ---
    {
        let (result, user, sys) = measure_cpu(|| {
            let mut runner = BenchRunner::new("umem_ring_combined", ITERS);
            let mut umem = BenchUmem::new(4096, 1024);
            let mut ring = BenchRing::new(1024);

            runner.run(|_i| {
                // Alloc
                let addr = umem.alloc_frame().unwrap();

                // Write 64 bytes
                let ptr = umem.frame_ptr(addr);
                unsafe {
                    std::ptr::write_bytes(ptr, 0xAA, 64);
                }

                // Push to ring
                ring.push(addr);

                // Pop from ring
                let rx_addr = ring.pop().unwrap();

                // Read
                let rx_ptr = umem.frame_ptr(rx_addr);
                unsafe {
                    black_box(std::ptr::read(rx_ptr));
                }

                // Free
                umem.free_frame(rx_addr);
            });

            runner.report("ops/sec", 1) // 1 full cycle per iter
        });

        print_cpu_summary("umem_ring_combined", user, sys);
        results.push((result, user, sys));
    }

    results
}

// =============================================================================
// Main — orchestrate all benchmarks
// =============================================================================

fn main() {
    eprintln!("=== AF_XDP Benchmark Suite ===");
    eprintln!("Sacred Law: NO external deps. Rust std only.");
    eprintln!();

    let mut all_results: Vec<BenchResult> = Vec::new();
    let mut cpu_totals: Vec<(&str, Duration, Duration)> = Vec::new();

    // ── Benchmark 1: UMEM frame alloc/free ──────────────────────────────
    eprintln!("[1/7] UMEM frame alloc/free ...");
    let (r, u, s) = bench_umem_alloc_free();
    cpu_totals.push(("umem_alloc_free", u, s));
    all_results.push(r);

    // ── Benchmark 2: Ring buffer produce/consume ────────────────────────
    eprintln!("[2/7] Ring buffer produce/consume ...");
    let ring_results = bench_ring_produce_consume();
    for (r, u, s) in ring_results {
        cpu_totals.push(("ring_produce_consume", u, s));
        all_results.push(r);
    }

    // ── Benchmark 3: End-to-end packet RX latency ──────────────────────
    eprintln!("[3/7] End-to-end packet RX latency ...");
    let (r, u, s) = bench_e2e_packet_rx_latency();
    cpu_totals.push(("e2e_packet_rx", u, s));
    all_results.push(r);

    // ── Benchmark 4: Batch size sweep ───────────────────────────────────
    eprintln!("[4/7] Batch size sweep ...");
    let sweep_results = bench_batch_size_sweep();
    for (r, u, s) in sweep_results {
        cpu_totals.push(("batch_sweep", u, s));
        all_results.push(r);
    }

    // ── Benchmark 5: Memory bandwidth ───────────────────────────────────
    eprintln!("[5/7] Memory bandwidth ...");
    let (r, u, s) = bench_memory_bandwidth();
    cpu_totals.push(("memory_bandwidth", u, s));
    all_results.push(r);

    // ── Benchmark 6: BPF map lookup simulation ─────────────────────────
    eprintln!("[6/7] BPF map lookup simulation ...");
    let (r, u, s) = bench_bpf_map_lookup();
    cpu_totals.push(("bpf_map_lookup", u, s));
    all_results.push(r);

    // ── Benchmark 7: Comparison matrix ──────────────────────────────────
    eprintln!("[7/7] Comparison matrix (ring vs UMEM) ...");
    let cmp_results = bench_comparison_matrix();
    for (r, u, s) in cmp_results {
        cpu_totals.push(("comparison", u, s));
        all_results.push(r);
    }

    // ── Output ──────────────────────────────────────────────────────────

    // JSON to stdout
    println!("{}", results_to_json(&all_results));

    // Markdown table to stderr
    print_markdown_table(&all_results);

    // CPU time summary to stderr
    eprintln!("## CPU Time Summary");
    eprintln!();
    let total_user: Duration = cpu_totals.iter().map(|(_, u, _)| *u).sum();
    let total_sys: Duration = cpu_totals.iter().map(|(_, _, s)| *s).sum();
    eprintln!(
        "  Total CPU: user={:.3}ms system={:.3}ms combined={:.3}ms",
        total_user.as_secs_f64() * 1000.0,
        total_sys.as_secs_f64() * 1000.0,
        (total_user + total_sys).as_secs_f64() * 1000.0,
    );
    eprintln!();

    // Comparison matrix summary
    eprintln!("## Comparison Matrix: Ring Buffer vs UMEM Operations");
    eprintln!();
    // Find the relevant results
    let ring_only = all_results.iter().find(|r| r.name == "ring_only_push_pop");
    let umem_only = all_results
        .iter()
        .find(|r| r.name == "umem_only_alloc_free");
    let combined = all_results.iter().find(|r| r.name == "umem_ring_combined");

    if let (Some(ro), Some(uo), Some(co)) = (ring_only, umem_only, combined) {
        eprintln!(
            "  Ring-only push/pop:   avg={:>8}ns  p99={:>8}ns  {:.0} ops/sec",
            ro.avg_latency_ns, ro.p99_ns, ro.throughput
        );
        eprintln!(
            "  UMEM-only alloc/free: avg={:>8}ns  p99={:>8}ns  {:.0} ops/sec",
            uo.avg_latency_ns, uo.p99_ns, uo.throughput
        );
        eprintln!(
            "  Combined cycle:       avg={:>8}ns  p99={:>8}ns  {:.0} ops/sec",
            co.avg_latency_ns, co.p99_ns, co.throughput
        );

        if ro.avg_latency_ns > 0 && uo.avg_latency_ns > 0 {
            let overhead_ns = co
                .avg_latency_ns
                .saturating_sub(ro.avg_latency_ns + uo.avg_latency_ns);
            eprintln!(
                "  Integration overhead: ~{}ns (memcpy + coordination)",
                overhead_ns
            );
        }
    }

    eprintln!();
    eprintln!("=== Benchmark complete ===");
}
