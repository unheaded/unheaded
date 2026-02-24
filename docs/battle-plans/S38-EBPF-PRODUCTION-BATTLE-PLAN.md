# S38 eBPF PRODUCTION BATTLE PLAN
## Unheaded Project — Core Product Differentiator

**Forged by:** The Warmonger
**Date:** 2026-02-24
**Scope:** Medium Sprint (180-220 steps)
**Objective:** Production eBPF programs (packet_marker XDP, flow_tracker TC, latency_probe tracepoint) + trace-collector unification + Wotan/Dashboard integration
**Risk Level:** CRITICAL (BPF verifier rejections common)
**Kernel Requirements:** Linux >= 5.15, CONFIG_BPF, CONFIG_XDP, CONFIG_NET_CLS_BPF, root access

---

## LEGEND
- **[B]** = BATTLE (objective/goal)
- **[V]** = VERIFICATION (how we know success)
- **[D]** = DEBUG (failure mode + recovery)
- **[W]** = WATCH (performance/metrics to monitor)
- **[R]** = ROLLBACK (abort condition + recovery)
- **[S]** = SUBTASK (nested work unit)
- **[P]** = PREREQUISITE (hard dependency check)
- **[C]** = COMMIT (git checkpoint)

---

## PHASE 0: ENVIRONMENT VERIFICATION (Steps 1-15)

### Step 1: Verify Linux kernel >= 5.15 with BPF/XDP support
**[B]** Ensure kernel has CONFIG_BPF, CONFIG_XDP, CONFIG_NET_CLS_BPF enabled
**[V]** `uname -r` returns kernel >= 5.15; `cat /boot/config-$(uname -r) | grep -E "CONFIG_BPF|CONFIG_XDP|CONFIG_NET_CLS_BPF"`
**[W]** Kernel version output, config grep results
**[P]** Root access (verify with `whoami` = root)
**[D]** If kernel < 5.15: ABORT — cannot proceed without modern BPF. Document OS and request EC2 upgrade. If CONFIG_BPF missing: ABORT — kernel compiled without BPF support.
**[R]** If kernel fails: Stop all work, update EC2 instance or migrate to supported OS
**Time:** 2 min

### Step 2: Verify bpftool installed and functional
**[B]** bpftool must be available for BPF program inspection, map pinning, and debugging
**[V]** `which bpftool` returns path; `bpftool version` returns version string
**[W]** bpftool version output
**[D]** If missing: `apt-get install -y linux-tools-$(uname -r)` OR `dnf install -y kernel-tools`
**[R]** If install fails: Check package manager; try alternate toolchain (cargo asm)
**Time:** 3 min

### Step 3: Verify Rust/Aya toolchain (cargo, rustc, llvm-tools)
**[B]** Rust/Aya is the BPF compiler for packet_marker, flow_tracker, latency_probe
**[V]** `rustc --version` >= 1.70; `cargo --version`; `cargo bpf --version` (from aya-build)
**[W]** Rust version, cargo version, aya-build available
**[D]** If rustc missing: `curl --proto '=https' --tlsv1.2 -sSf https://sh.rustup.rs | sh`. If cargo bpf missing: `cargo install cargo-bpf` or `cargo add aya-build`
**[R]** If Rust toolchain fails to install: Verify internet access, try offline install from rustup tarball
**Time:** 5 min

### Step 4: Verify cilium/ebpf Go module installed in ~/tmp/unheaded
**[B]** Go userspace loaders need cilium/ebpf for program loading, map access, ring buffer reading
**[V]** `grep -r "github.com/cilium/ebpf" ~/tmp/unheaded/go.mod` returns module; `go list -m github.com/cilium/ebpf`
**[W]** go.mod entry, module version
**[D]** If missing: `cd ~/tmp/unheaded && go get github.com/cilium/ebpf@latest`
**[R]** If go.mod update fails: Check internet connectivity, verify go.sum consistency
**Time:** 2 min

### Step 5: Verify ~/tmp/unheaded directory structure
**[B]** Project layout must exist: ebpf/ (Rust/Aya source), cmd/trace-collector-go/, crates/, docs/
**[V]** `ls -la ~/tmp/unheaded/ | grep -E "ebpf|cmd|crates|docs"`
**[W]** Directory listing
**[D]** If ebpf/ missing: Create with `mkdir -p ~/tmp/unheaded/ebpf/{packet_marker,flow_tracker,latency_probe,common}`
**[R]** If major structural issues: Restore from git or request project skeleton
**Time:** 2 min

### Step 6: Verify xdp-tools package installed (for XDP debugging)
**[B]** xdp-tools provides xdp-loader and xdp-multiprog for advanced XDP management
**[V]** `which xdp-loader` returns path; `xdp-loader --help` returns usage
**[W]** xdp-loader available
**[D]** If missing: `apt-get install -y xdp-tools` OR compile from source (https://github.com/xdp-project/xdp-tools)
**[R]** If install fails: Continue with basic ip link XDP attachment; xdp-tools is nice-to-have
**Time:** 3 min

### Step 7: Check BPF map pinning directory (/sys/fs/bpf)
**[B]** Maps must be pinned to /sys/fs/bpf for persistence and reuse across programs
**[V]** `ls -la /sys/fs/bpf/` returns directory; `mount | grep bpf` shows bpf filesystem
**[W]** BPF filesystem mounted
**[D]** If missing: `mount -t bpf bpf /sys/fs/bpf` or add to fstab: `bpf /sys/fs/bpf bpf defaults 0 0`
**[R]** If mount fails: Kernel doesn't support bpffs; cannot persist maps — requires reattach on reboot
**Time:** 2 min

### Step 8: Verify Go 1.24 installed
**[B]** trace-collector Go binary requires Go 1.24+
**[V]** `go version` returns Go 1.24 or later
**[W]** Go version output
**[D]** If Go < 1.24: `rm -rf /usr/local/go && wget https://golang.org/dl/go1.24.linux-amd64.tar.gz && tar -C /usr/local -xzf go1.24.linux-amd64.tar.gz`
**[R]** If Go installation fails: Verify download URL and disk space
**Time:** 3 min

### Step 9: Verify Wotan gRPC port 18001 accessibility
**[B]** trace-collector must publish traces to Wotan on port 18001
**[V]** `ss -tlnp | grep 18001` OR `curl -i localhost:18001` (expect gRPC response); Wotan service running
**[W]** Wotan service status, port 18001 listening
**[D]** If Wotan not running: Start Wotan service from S36 deployment; verify network routing if remote
**[R]** If Wotan unavailable: Proceed with offline testing; defer Wotan integration to Step 150+
**Time:** 2 min

### Step 10: Verify Dashboard on port 20000 accessible
**[B]** Dashboard must be available to visualize traces (Phase 6+)
**[V]** `ss -tlnp | grep 20000` OR `curl -i localhost:20000` (expect HTTP response)
**[W]** Dashboard service status
**[D]** If Dashboard offline: Start from S36 deployment; non-blocking for early phases
**[R]** If Dashboard unavailable: Skip Phase 6 visualization; focus on trace collection
**Time:** 2 min

### Step 11: Clone/update unheaded repo and verify git status
**[B]** Ensure latest code from main branch
**[V]** `cd ~/tmp/unheaded && git status` returns "On branch main" and clean or tracked changes
**[W]** Git branch, uncommitted changes
**[D]** If dirty: `git stash` or `git reset --hard HEAD`; if remote diverged: `git fetch origin && git reset --hard origin/main`
**[R]** If git fails: Backup code, re-clone from git remote
**Time:** 3 min

### Step 12: Verify llvm-related build tools
**[B]** BPF compilation needs LLVM (clang, llc, llvm-objcopy)
**[V]** `clang --version`; `llc --version`; `which llvm-objcopy`
**[W]** LLVM versions
**[D]** If missing: `apt-get install -y llvm-14 clang-14 && update-alternatives --install /usr/bin/clang clang /usr/bin/clang-14 100`
**[R]** If LLVM unavailable: Cannot compile BPF programs — abort phase
**Time:** 3 min

### Step 13: Create BPF program skeleton directories
**[B]** Organize Rust/Aya source files for three BPF programs
**[S]** Create ~/tmp/unheaded/ebpf/packet_marker/ with Cargo.toml, src/main.rs
**[S]** Create ~/tmp/unheaded/ebpf/flow_tracker/ with Cargo.toml, src/main.rs
**[S]** Create ~/tmp/unheaded/ebpf/latency_probe/ with Cargo.toml, src/main.rs
**[V]** `ls -la ~/tmp/unheaded/ebpf/packet_marker/Cargo.toml` etc. exist
**[D]** If creation fails: Manual mkdir + touch files
**[R]** If directory structure breaks: Delete and recreate from scratch
**Time:** 3 min

### Step 14: Create trace-collector Go directory structure
**[B]** Organize userspace loader binary
**[S]** Create ~/tmp/unheaded/cmd/trace-collector-go/ with main.go, loader.go, wotan-publisher.go
**[V]** Directory structure present
**[D]** If missing: `mkdir -p ~/tmp/unheaded/cmd/trace-collector-go`
**[R]** If fails: Manual directory creation
**Time:** 2 min

### Step 15: PHASE 0 EXIT GATE
**[B]** Verify all prerequisites passed
**[V]** All 14 steps passed; kernel BPF support confirmed; all tools installed
**[D]** If any step failed with [R] (ROLLBACK): Fix root cause or ABORT sprint
**[C]** `cd ~/tmp/unheaded && git add -A && git commit -m "S38-Phase0: Environment verification complete"`
**[W]** Git commit hash
**Time:** 1 min

---

## PHASE 1: PACKET_MARKER XDP PROGRAM (Steps 16-50)

### Step 16: Create packet_marker/Cargo.toml with Aya dependencies
**[B]** Define Rust/Aya project structure for XDP program
**[V]** Cargo.toml contains [package] name="packet_marker", [dependencies] with aya, aya-ebpf, etc.
**[W]** Cargo.toml present
**[D]** If Cargo.toml fails to parse: Validate TOML syntax, check dependency versions
**[R]** If dependency resolution fails: `cargo update -p aya` to refresh lockfile
**Time:** 3 min

### Step 17: Create packet_marker BPF map definitions (Rust structure)
**[B]** Define BPF maps for storing trace IDs and packet metadata
**[V]** src/main.rs contains:
```
#[map]
static TRACE_ID_MAP: PerCpuArray<u32> = PerCpuArray::with_max_entries(1024, 0);

#[map]
static PACKET_METADATA: PerCpuArray<PacketTrace> = PerCpuArray::with_max_entries(1024, 0);
```
**[W]** Map definitions compiled without BPF verifier errors
**[D]** If verifier rejects PerCpuArray: Switch to Array; if memory exceeded: Reduce max_entries to 256
**[R]** If map definitions fundamentally wrong: Review BPF map type spec in Appendix B
**Time:** 4 min

### Step 18: Define PacketTrace structure (5-tuple + trace ID + timestamp)
**[B]** Create struct for packet metadata passed through BPF maps
**[V]** struct PacketTrace defined with fields: src_ip (u32), dst_ip (u32), src_port (u16), dst_port (u16), proto (u8), trace_id (u64), timestamp_ns (u64)
**[W]** Struct compiles, field alignment correct (no padding issues)
**[D]** If struct doesn't align: Use #[repr(C)] for explicit alignment; check field sizes
**[R]** If compilation fails: Reduce struct size or split into multiple maps
**Time:** 3 min

### Step 19: Implement XDP hook function with packet parsing
**[B]** Write XDP program entrypoint to parse packet headers (Ethernet, IP, TCP/UDP)
**[V]** src/main.rs contains:
```
#[xdp]
pub fn packet_marker(ctx: XdpContext) -> u32 {
    match parse_packet(&ctx) {
        Ok((src_ip, dst_ip, src_port, dst_port, proto)) => {
            // Continue to Step 20
            XDP_PASS
        },
        Err(_) => XDP_PASS,
    }
}
```
**[W]** XDP function compiles, verifier accepts program
**[D]** BPF verifier rejects: "invalid memory access" — common with pointer arithmetic. FIX: Use bounds checks before dereferencing. See Appendix A: BPF Verifier Debug.
**[R]** If verifier continuously rejects: Simplify packet parsing to Ethernet only, defer IP header parsing
**Time:** 5 min

### Step 20: Implement IP header parsing (src_ip, dst_ip, proto)
**[B]** Extract IP addresses and protocol from packet
**[V]** parse_packet() reads Ethernet header, validates length, then IP header (20-byte minimum)
**[D]** BPF verifier error "pointer out of bounds": Check boundary with `(void *)data + data_end`. FIX:
```
if ((void *)iph + sizeof(*iph) > data_end) return XDP_PASS;
```
**[R]** If parsing loops infinitely: BPF doesn't allow unbounded loops; use fixed unroll or check compiler flags
**Time:** 5 min

### Step 21: Implement transport layer parsing (src_port, dst_port)
**[B]** Extract TCP/UDP source and destination ports
**[V]** parse_packet() continues to TCP/UDP header; extracts ports from network byte order
**[D]** If ports incorrect: Network byte order issue — use `bpf_ntohs()` helper or check endianness
**[R]** If BPF helper not found: Use manual byte swap `(u16)((port >> 8) | ((port & 0xFF) << 8))`
**Time:** 4 min

### Step 22: Generate or obtain trace ID for packet
**[B]** Create unique trace ID to mark this packet across the system
**[V]** Trace ID generated using BPF helper (e.g., bpf_get_prandom_u32() seeded with 5-tuple hash)
**[W]** Trace IDs visible in map output (non-zero)
**[D]** If all trace IDs are 0: Check RNG seed logic; verify map write succeeds
**[R]** If trace ID generation fails: Use packet count as simple identifier (0-indexed)
**Time:** 4 min

### Step 23: Store packet metadata (5-tuple + trace ID) in map
**[B]** Write parsed packet data to PACKET_METADATA map for userspace retrieval
**[V]** map.insert() or bpf_map_update_elem() succeeds; map entry visible via bpftool
**[D]** BPF verifier error "stack corruption": Common when copying large structs. FIX: Use __attribute__((packed)) on PacketTrace struct.
**[R]** If map write fails: Reduce struct size or use per-CPU map (faster, no contention)
**Time:** 4 min

### Step 24: Add timestamp to packet metadata (nanoseconds)
**[B]** Record packet arrival time for latency correlation
**[V]** timestamp_ns = bpf_ktime_get_ns(); written to map with trace ID
**[W]** Timestamps increase monotonically in map output
**[D]** If timestamps zero: bpf_ktime_get_ns() not available in XDP context. FIX: Use bpf_jiffies64() instead (less precise)
**[R]** If timestamp unavailable: Skip timestamp field, continue with trace ID marking
**Time:** 3 min

### Step 25: Add XDP action selection logic (XDP_PASS, XDP_DROP, XDP_TX)
**[B]** Decide packet fate based on trace decision or error
**[V]** Program returns appropriate XDP action (default XDP_PASS for passthrough)
**[W]** Packets forwarded or dropped as expected
**[D]** If packets disappear: Check XDP action — may be XDP_DROP. FIX: Verify logic returns XDP_PASS for normal packets
**[R]** If XDP action causes kernel panic: Switch to XDP_PASS only (safest)
**Time:** 3 min

### Step 26: Compile packet_marker BPF program with cargo-bpf
**[B]** Compile Rust/Aya source to ELF BPF bytecode
**[V]** `cd ~/tmp/unheaded/ebpf/packet_marker && cargo build --release --target bpfel64-unknown-linux-gnu`
**[W]** target/bpfel64-unknown-linux-gnu/release/packet_marker compiled; ELF file created
**[D]** Compiler errors: Check Rust syntax, review step 17-25 logic. BPF verifier errors: See Appendix A.
**[R]** If compilation fails repeatedly: Revert to previous working commit, simplify program stub
**Time:** 8 min

### Step 27: Verify BPF program ELF format and sections
**[B]** Confirm output is valid BPF ELF with xdp section
**[V]** `llvm-objdump -d target/bpfel64-unknown-linux-gnu/release/packet_marker | grep xdp` returns section; `file target/bpfel64-unknown-linux-gnu/release/packet_marker` shows ELF
**[W]** ELF file valid, xdp section present
**[D]** If no xdp section: Check #[xdp] macro applied to function; verify aya-build version
**[R]** If ELF format invalid: Recompile with verbose cargo output to diagnose
**Time:** 3 min

### Step 28: Create test fixture: mock XDP context
**[B]** Prepare test environment for packet_marker unit tests
**[V]** Create ~/tmp/unheaded/ebpf/packet_marker/tests/integration_test.rs with mock XdpContext
**[D]** If Aya testing framework missing: Check aya-ebpf-crate features; may need custom mock
**[R]** If mocks fail: Proceed to Step 29 (live XDP attach) for functional testing
**Time:** 4 min

### Step 29: Load packet_marker program into kernel via cilium/ebpf
**[B]** Create Go loader that reads compiled ELF and loads into kernel
**[V]** `cd ~/tmp/unheaded && cat > cmd/trace-collector-go/loader.go << 'EOF'`
```
package main
import "github.com/cilium/ebpf"

func LoadPacketMarker(progPath string) (*ebpf.Program, error) {
    spec, err := ebpf.LoadCollectionSpec(progPath)
    if err != nil { return nil, err }
    coll, err := ebpf.NewCollection(spec)
    return coll.Programs["packet_marker"], err
}
EOF`
**[W]** Loader compiles without error
**[D]** If cilium/ebpf API differs: Check version (>= 0.12); adjust API calls
**[R]** If load fails with permission denied: Run as root; check /sys/fs/bpf mount
**Time:** 5 min

### Step 30: Attach packet_marker XDP program to network interface
**[B]** Attach compiled XDP program to ens0 (or primary NIC) for packet processing
**[V]** Program attached via `ip link set dev ens0 xdp obj <ELF> sec xdp` OR cilium/ebpf loader
**[W]** `ip link show ens0 | grep -i xdp` shows program attached
**[D]** Error "Device or resource busy": XDP already attached. FIX: `ip link set dev ens0 xdp off` first, then reattach.
If "BPF program not found": Check ELF path and section name.
**[R]** If XDP attach fails on virtual NIC (veth): Switch to real NIC or use tc/kprobe instead
**Time:** 4 min

### Step 31: Verify XDP program status via bpftool
**[B]** Confirm program loaded and attached correctly
**[V]** `bpftool prog list | grep xdp` shows packet_marker program; `bpftool prog show id <ID>` displays details
**[W]** Program ID, attachment point, bytecode size
**[D]** If program not listed: Check attachment status, may have failed silently
**[R]** If ID shows but program not attached: Rerun Step 30 with explicit device
**Time:** 2 min

### Step 32: Read packet_marker BPF maps from userspace (trace IDs)
**[B]** Verify packets are being marked with trace IDs
**[V]** Go program reads TRACE_ID_MAP via cilium/ebpf MapReader; values non-zero
**[W]** Trace ID values in output (sample: 5 entries from map)
**[D]** If map empty: Packets may not match filter; send test traffic (Step 33)
**[R]** If map unreadable: Check map pinning permissions, verify map name matches ELF
**Time:** 4 min

### Step 33: Generate test traffic to trigger packet_marker
**[B]** Send packets to exercise XDP program
**[V]** `ping -c 10 8.8.8.8` OR `curl -I http://example.com` (generates TCP packets)
**[W]** ICMP or TCP packets sent
**[D]** If no response: Network may be filtered; try ARP instead
**[R]** If unable to send traffic: Skip to Phase 2 (offline testing)
**Time:** 3 min

### Step 34: Verify trace IDs written to map from test traffic
**[B]** Confirm marked packets appear in BPF map
**[V]** TRACE_ID_MAP or PACKET_METADATA shows entries with non-zero trace_id and timestamp_ns
**[W]** Map entry count, trace ID values, timestamp range
**[D]** If empty: XDP program may not be executing. FIX: Check if packets match expected filters. Verify via `tcpdump -i ens0` that packets arrive.
**[R]** If verifier rejected program: See Appendix A; recompile with simplified logic
**Time:** 3 min

### Step 35: Debug BPF verifier rejections (common failure mode)
**[B]** If Step 26 or 34 revealed verifier errors, apply fixes
**[V]** Program recompiles without verifier errors
**[D]** Common errors and fixes:
- "invalid memory access": Add bounds checks before pointer deref
- "stack corruption": Reduce struct size or mark __attribute__((packed))
- "unbounded loop": Use pragma unroll or reduce loop iteration count
See Appendix A for detailed debug procedures.
**[R]** If verifier rejects after multiple attempts: Simplify packet_marker to trace ID only (no IP parsing); defer advanced parsing to Phase 2
**Time:** 8 min

### Step 36: Measure XDP program performance (packets/sec processed)
**[B]** Establish baseline latency and throughput for packet_marker
**[V]** Send 1000 packets, measure latency between ingress and map write; target < 100µs latency per packet
**[W]** Packet latency histogram, throughput (packets/sec)
**[D]** If latency high: Reduce map size, optimize packet parsing, check CPU context switching
**[R]** If performance unacceptable: Defer optimization to Phase 7; continue with current implementation
**Time:** 5 min

### Step 37: Pin TRACE_ID_MAP and PACKET_METADATA to /sys/fs/bpf
**[B]** Persist maps across program reloads for map reuse
**[V]** Maps pinned via `bpftool map pin id <MAP_ID> /sys/fs/bpf/trace_id_map` etc.
**[W]** `ls -la /sys/fs/bpf | grep -E "trace_id|packet_metadata"` shows pinned maps
**[D]** If pin fails: Check bpffs mount, verify permissions
**[R]** If pinning unavailable: Maps will be recreated on reload; non-critical for Phase 1
**Time:** 3 min

### Step 38: Create Go test harness for packet_marker verification
**[B]** Automated test to verify packet marking works end-to-end
**[V]** Test:
1. Loads packet_marker ELF
2. Attaches to test NIC
3. Sends 10 packets via ping
4. Reads TRACE_ID_MAP
5. Verifies 10 entries with non-zero trace IDs
**[W]** Test passes with all assertions
**[D]** If test fails: Debug each sub-step; check NIC name, map names match
**[R]** If assertions fail: Review packet_marker compilation output
**Time:** 6 min

### Step 39: Document packet_marker XDP program architecture
**[B]** Write design doc for reference
**[V]** ~/tmp/unheaded/docs/packet_marker.md describes:
- XDP hook point
- Map structure (Per-CPU Array, entries)
- 5-tuple extraction logic
- Trace ID generation (RNG seeding)
- Timestamp correlation
**[W]** Doc complete
**[D]** If doc incomplete: Add required sections as follow-up
**[R]** Non-critical; proceed to Phase 2
**Time:** 4 min

### Step 40: PHASE 1 EXIT GATE
**[B]** Verify packet_marker XDP program fully functional
**[V]**
- Program compiles without verifier errors (Step 26)
- Attaches to NIC (Step 30)
- Marks packets with trace IDs (Step 34)
- Baseline latency < 100µs per packet (Step 36)
**[D]** If any assertion failed: Return to failed step and [D] branch
**[C]** `cd ~/tmp/unheaded && git add -A && git commit -m "S38-Phase1: packet_marker XDP program complete and tested"`
**[W]** Commit hash, test results
**Time:** 2 min

---

## PHASE 2: FLOW_TRACKER TC PROGRAM (Steps 41-75)

### Step 41: Create flow_tracker/Cargo.toml for TC program
**[B]** Setup Rust/Aya project for traffic control (TC) classifier program
**[V]** Cargo.toml with package name flow_tracker, dependencies on aya, aya-ebpf
**[W]** Cargo.toml parses correctly
**[D]** If Cargo.toml invalid: Check TOML syntax vs. Step 16 template
**[R]** If dependencies unresolvable: Run `cargo update`
**Time:** 2 min

### Step 42: Define 5-tuple flow key structure
**[B]** Create struct for flow identification (src_ip, dst_ip, src_port, dst_port, proto)
**[V]** struct Flow5Tuple defined with #[repr(C)] alignment; implements Hash for map key
**[W]** Struct compiles, size correct (< 512 bytes for BPF map key limit)
**[D]** If struct size > 512: Split into two smaller maps
**[R]** If alignment issues: Check field order (largest first to minimize padding)
**Time:** 3 min

### Step 43: Define flow state structure (connection tracking)
**[B]** Track TCP connection states (SYN, SYN-ACK, ESTABLISHED, FIN, RST)
**[V]** struct FlowState contains: state (u8 enum), packets_in (u64), packets_out (u64), bytes_in (u64), bytes_out (u64), last_seen_ns (u64)
**[W]** Struct defined and compiles
**[D]** If struct too large: Remove byte counters, keep packet counts only
**[R]** If enum not supported in BPF: Use u8 constants (1=SYN, 2=EST, etc.)
**Time:** 3 min

### Step 44: Create BPF hash map for flow tracking
**[B]** Store active flows keyed by 5-tuple, values = FlowState
**[V]** BPF map defined:
```
#[map]
static FLOW_MAP: HashMap<Flow5Tuple, FlowState> = HashMap::with_max_entries(10000, 0);
```
**[W]** Map defined without compilation errors
**[D]** If max_entries too high: BPF kernel enforces cap (~64K); set to 10000
**[R]** If HashMap not supported: Use Array with hash index calculation
**Time:** 3 min

### Step 45: Implement TC classifier hook function
**[B]** Write TC program entrypoint for ingress/egress packet classification
**[V]** src/main.rs contains:
```
#[classifier]
pub fn flow_track(ctx: SkBuffContext) -> u32 {
    // Extract flow from skb
    // Lookup/update FLOW_MAP
    // Return TC_OK
}
```
**[W]** TC classifier function compiles
**[D]** BPF verifier rejects: "invalid memory context" — TC uses SkBuff, not XDP context. Check macro and context type.
**[R]** If verifier repeatedly rejects: Simplify to stub that returns TC_OK
**Time:** 5 min

### Step 46: Implement TCP header parsing in TC context
**[B]** Extract 5-tuple from skb for flow identification
**[V]** Parse Ethernet → IP → TCP/UDP headers from skb data_start and data_end pointers
**[D]** BPF verifier error "pointer arithmetic": SkBuff memory model different from XDP. FIX: Check data_start/data_end bounds vs. XDP's data/data_end.
**[R]** If parsing fails: Use TC helper functions bpf_skb_load_bytes() instead of pointer arithmetic
**Time:** 5 min

### Step 47: Implement TCP flag parsing (SYN, ACK, FIN, RST)
**[B]** Extract TCP flags from header for connection state tracking
**[V]** TCP flags (SYN=0x02, ACK=0x10, FIN=0x01, RST=0x04) extracted and matched
**[W]** Flags correctly parsed in test packets
**[D]** If flags incorrect: Check TCP header layout (flags in byte 13 of header)
**[R]** If parsing fails: Skip TCP state tracking, count all packets as single state
**Time:** 4 min

### Step 48: Implement flow state machine (SYN → EST → FIN)
**[B]** Track connection lifecycle via TCP flags
**[V]** State transitions: INIT → SYN (on SYN flag) → ESTABLISHED (on SYN-ACK + ACK) → FIN_SENT (on FIN) → CLOSED (on FIN-ACK)
**[W]** State machine executes without deadlock or infinite loops
**[D]** If state transitions incorrect: Review TCP RFC 793 state diagram; add debug logging
**[R]** If state machine too complex: Simplify to SYN/ESTABLISHED/FIN only
**Time:** 6 min

### Step 49: Implement packet/byte counter updates in flow state
**[B]** Increment packet and byte counters per flow direction
**[V]** Each packet increments FlowState.packets_in/out and bytes_in/out based on direction
**[D]** If counters overflow: Use u64 (wraps at 2^64 bytes); consider log-scaled counters
**[R]** If counter updates lock: Use atomic operations or per-CPU maps
**Time:** 4 min

### Step 50: Implement flow aging / timeout tracking
**[B]** Mark flows as inactive if no packets seen for 5 minutes
**[V]** last_seen_ns updated on each packet; userspace periodically deletes flows with last_seen_ns + 300s < now
**[W]** Flows marked stale after 5 minutes
**[D]** If aging logic incorrect: Timestamps may not update; verify bpf_ktime_get_ns() in TC context
**[R]** If aging unavailable: Accept potential map growth; implement cleanup in userspace
**Time:** 4 min

### Step 51: Implement TC return actions (TC_OK, TC_DROP, TC_RECLASSIFY)
**[B]** Decide TC action based on flow policy (allow/drop/reclassify)
**[V]** TC program returns TC_OK for passthrough by default
**[W]** Packets forwarded as expected
**[D]** If packets dropped: Check TC action logic, ensure TC_OK is default
**[R]** If TC action causes kernel panic: Switch to TC_OK only
**Time:** 3 min

### Step 52: Compile flow_tracker TC program
**[B]** Compile Rust/Aya source to BPF bytecode
**[V]** `cd ~/tmp/unheaded/ebpf/flow_tracker && cargo build --release --target bpfel64-unknown-linux-gnu`
**[W]** ELF compiled, no verifier errors
**[D]** Compiler errors: Review steps 41-51; BPF verifier errors: See Appendix A
**[R]** If compilation fails: Simplify to stub classifier
**Time:** 8 min

### Step 53: Verify flow_tracker ELF format and TC section
**[B]** Confirm output has valid classifier section
**[V]** `llvm-objdump -d target/bpfel64-unknown-linux-gnu/release/flow_tracker | grep -i classifier` returns section; `file` shows ELF
**[W]** ELF valid, classifier section present
**[D]** If no classifier section: Check #[classifier] macro, verify aya-ebpf features
**[R]** If ELF invalid: Recompile with verbose output
**Time:** 3 min

### Step 54: Load flow_tracker TC program into kernel
**[B]** Load compiled ELF via cilium/ebpf or tc command
**[V]** Go loader:
```
func LoadFlowTracker(progPath string) (*ebpf.Program, error) {
    spec, err := ebpf.LoadCollectionSpec(progPath)
    coll, err := ebpf.NewCollection(spec)
    return coll.Programs["flow_track"], err
}
```
**[W]** Loader compiles
**[D]** If cilium/ebpf doesn't support TC: Use `tc filter add dev ens0 ingress bpf da obj <ELF> sec classifier` instead
**[R]** If load fails: Check file path and section names
**Time:** 4 min

### Step 55: Attach flow_tracker TC classifier to ingress qdisc
**[B]** Attach to ens0 ingress for packet classification
**[V]** `tc filter add dev ens0 ingress bpf da obj <ELF> sec classifier` succeeds
**[W]** `tc filter show dev ens0 ingress` displays classifier
**[D]** Error "no ingress qdisc": Create with `tc qdisc add dev ens0 ingress`
If "tc: command not found": Install iproute2
**[R]** If TC attachment fails: Defer to Step 60 (kprobe fallback)
**Time:** 4 min

### Step 56: Verify flow_tracker attachment via tc filter show
**[B]** Confirm TC classifier is attached
**[V]** `tc filter show dev ens0 ingress` shows flow_track classifier
**[W]** Filter listed with correct section
**[D]** If not shown: Rerun Step 55
**[R]** If reattach fails: Check kernel TC classifier support
**Time:** 2 min

### Step 57: Read FLOW_MAP from userspace
**[B]** Verify flows are being tracked in BPF map
**[V]** Go program reads FLOW_MAP; entries show 5-tuple keys and FlowState values
**[W]** Map entries with expected structure
**[D]** If empty: Send test traffic (Step 58)
**[R]** If unreadable: Check map name and permissions
**Time:** 3 min

### Step 58: Generate bidirectional test traffic
**[B]** Create TCP connection to populate flow map
**[V]** `iperf3 -c localhost` or `nc -l 5555 &` + `nc localhost 5555` opens TCP connection
**[W]** TCP handshake visible in FLOW_MAP
**[D]** If no traffic: Firewall may block; use localhost connection
**[R]** If unable to test: Proceed to Phase 3
**Time:** 3 min

### Step 59: Verify flow state transitions (SYN → ESTABLISHED → FIN)
**[B]** Confirm state machine tracks connection lifecycle
**[V]** FLOW_MAP shows flow state transitions: SYN → ESTABLISHED → FIN as connection progresses
**[W]** State values change correctly
**[D]** If states not tracked: Verify TCP flag parsing (Step 47), check state machine logic (Step 48)
**[R]** If state tracking fails: Implement offline state verification in userspace
**Time:** 4 min

### Step 60: Implement kprobe fallback for TC-unsupported environments
**[B]** Alternative to TC: Attach kprobe to tcp_sendmsg/tcp_cleanup_rbuf for flow tracking
**[V]** src/main.rs contains:
```
#[kprobe]
pub fn tcp_sendmsg_trace(ctx: ProbeContext) -> u32 {
    // Extract flow from context
    // Update FLOW_MAP
    0
}
```
**[W]** kprobe function compiles (optional, for Phase 3+ fallback)
**[D]** If kprobe not needed: Skip (TC is primary)
**[R]** If kprobe fallback needed later: Implement in Phase 3
**Time:** 5 min

### Step 61: Pin FLOW_MAP to /sys/fs/bpf
**[B]** Persist flow state across program reloads
**[V]** `bpftool map pin id <FLOW_MAP_ID> /sys/fs/bpf/flow_map`
**[W]** `ls -la /sys/fs/bpf/flow_map` exists
**[D]** If pin fails: Check bpffs mount
**[R]** Non-critical; maps recreate on reload
**Time:** 2 min

### Step 62: Implement userspace flow cleanup (timeout eviction)
**[B]** Delete flows older than 5 minutes from FLOW_MAP
**[V]** Go program periodically (every 60s) reads FLOW_MAP, identifies stale flows, deletes via bpf.Map.Delete()
**[W]** Stale flows removed, map size stable
**[D]** If deletion slow: Batch deletes or reduce check interval
**[R]** If deletion fails: Check map lock status
**Time:** 5 min

### Step 63: Create test harness for flow_tracker
**[B]** Automated test to verify flow tracking end-to-end
**[V]** Test:
1. Load flow_tracker ELF
2. Attach TC classifier
3. Send TCP traffic (SYN → EST → FIN)
4. Read FLOW_MAP
5. Verify state transitions
**[W]** Test passes
**[D]** If test fails: Debug each step
**[R]** If TC unavailable: Skip flow_tracker; use packet_marker only
**Time:** 6 min

### Step 64: Measure flow_tracker performance
**[B]** Establish baseline throughput with active flow tracking
**[V]** Send 1000 packets across 10 concurrent flows; measure latency and throughput
**[W]** Latency < 100µs per packet, throughput > 100k packets/sec
**[D]** If performance poor: Reduce max_entries or use simpler state machine
**[R]** If unacceptable: Defer optimization to Phase 7
**Time:** 5 min

### Step 65: Debug BPF verifier rejections in TC program (Appendix A reference)
**[B]** If Step 52 showed verifier errors, apply fixes
**[V]** Program recompiles
**[D]** Common TC errors:
- "invalid context": SkBuff not XDP. Use skb helper functions.
- "pointer arithmetic": SkBuff layout different. Use bpf_skb_load_bytes().
See Appendix A for detailed fixes.
**[R]** If verifier persistent: Simplify to stub that counts packets only
**Time:** 8 min

### Step 66: Document flow_tracker TC program
**[B]** Write design doc
**[V]** ~/tmp/unheaded/docs/flow_tracker.md describes:
- TC hook point (ingress/egress)
- Hash map structure (5-tuple key, FlowState value)
- State machine (SYN → EST → FIN)
- Packet/byte counter semantics
- Flow timeout (5 minutes)
**[W]** Doc complete
**[D]** If incomplete: Add sections as follow-up
**[R]** Non-critical
**Time:** 4 min

### Step 67: Integrate packet_marker and flow_tracker in trace-collector
**[B]** trace-collector loads both XDP and TC programs simultaneously
**[V]** loader.go updated:
```
func LoadAllPrograms(xdpPath, tcPath string) (map[string]*ebpf.Program, error) {
    xdp, _ := LoadPacketMarker(xdpPath)
    tc, _ := LoadFlowTracker(tcPath)
    return map[string]*ebpf.Program{
        "packet_marker": xdp,
        "flow_tracker": tc,
    }, nil
}
```
**[W]** Loader compiles
**[D]** If cilium/ebpf API changes: Check version compatibility
**[R]** If loading fails: Load programs sequentially with error checking
**Time:** 4 min

### Step 68: Create unified trace structure for packet + flow data
**[B]** Merge packet_marker and flow_tracker outputs into single trace record
**[V]** struct Trace contains: trace_id (from packet_marker), flow_state (from flow_tracker), timestamp_ns, src_ip, dst_ip, src_port, dst_port
**[W]** Struct defined
**[D]** If structure grows too large: Split into separate trace types
**[R]** Non-critical; can merge at Step 75
**Time:** 3 min

### Step 69: PHASE 2 EXIT GATE
**[B]** Verify flow_tracker TC program fully functional and integrated
**[V]**
- Program compiles without verifier errors (Step 52)
- Attaches to TC ingress (Step 55)
- Tracks flows with state transitions (Step 59)
- Baseline latency < 100µs per packet (Step 64)
- Integrated with packet_marker in loader (Step 67)
**[D]** If any assertion failed: Return to failed step
**[C]** `cd ~/tmp/unheaded && git add -A && git commit -m "S38-Phase2: flow_tracker TC program complete and tested"`
**[W]** Commit hash, test results
**Time:** 2 min

---

## PHASE 3: LATENCY_PROBE TRACEPOINT PROGRAM (Steps 70-105)

### Step 70: Create latency_probe/Cargo.toml for tracepoint program
**[B]** Setup Rust/Aya project for Linux tracepoint instrumentation
**[V]** Cargo.toml with package latency_probe, aya/aya-ebpf dependencies
**[W]** Cargo.toml parses
**[D]** If invalid: Check Step 16 template
**[R]** If unresolvable: `cargo update`
**Time:** 2 min

### Step 71: Define latency event structure
**[B]** Create struct for RTT and queue delay measurements
**[V]** struct LatencyEvent contains: trace_id (u64), src_ip (u32), dst_ip (u32), src_port (u16), dst_port (u16), rtt_ns (u64), queue_delay_ns (u64), timestamp_ns (u64)
**[W]** Struct compiles, size < 256 bytes
**[D]** If size exceeds: Remove less critical fields
**[R]** If struct alignment fails: Use #[repr(C)] and reorder fields
**Time:** 3 min

### Step 72: Create BPF ring buffer for latency events
**[B]** Define ring buffer map to stream latency samples to userspace
**[V]** BPF map:
```
#[map]
static LATENCY_EVENTS: RingBuf<LatencyEvent> = RingBuf::new();
```
**[W]** Map defined without error
**[D]** If RingBuf not available: Use PerCpuArray instead (less efficient)
**[R]** If size issues: Reduce LatencyEvent fields
**Time:** 3 min

### Step 73: Research Linux tcp_probe tracepoint structure
**[B]** Understand tcp_probe tracepoint arguments for RTT extraction
**[V]** `cat /sys/kernel/debug/tracing/events/tcp/tcp_probe/format` shows event fields (saddr, daddr, sport, dport, rtt, srtt, etc.)
**[W]** tcp_probe format documented
**[D]** If /sys/kernel/debug not accessible: May not have debugfs enabled; use alternative tracepoint (tcp_retransmit_skb, etc.)
**[R]** If tcp_probe unavailable: Measure RTT via timestamps in packet_marker
**Time:** 3 min

### Step 74: Implement tcp_probe tracepoint hook
**[B]** Attach eBPF program to tcp_probe tracepoint for RTT collection
**[V]** src/main.rs contains:
```
#[tracepoint]
pub fn tcp_probe(ctx: TracePointContext) -> u32 {
    // Extract rtt and queue_delay from tp_event
    // Write LatencyEvent to ring buffer
    0
}
```
**[W]** Tracepoint function compiles
**[D]** BPF verifier rejects: "invalid context type" — TracePointContext may differ. Check aya-ebpf tracepoint API.
**[R]** If tracepoint fails: Implement kprobe alternative (Step 76)
**Time:** 5 min

### Step 75: Extract RTT from tcp_probe event
**[B]** Parse tcp_probe tracepoint data to obtain round-trip time
**[V]** RTT extracted from event field (typically in microseconds or nanoseconds); convert to nanoseconds
**[W]** RTT values > 0 and < 1 second typical
**[D]** If RTT values incorrect: Check field offset in tracepoint format; may need byte swap
**[R]** If field not available: Estimate RTT from send/ack timestamps instead
**Time:** 4 min

### Step 76: Implement kprobe alternative for RTT measurement
**[B]** Fallback if tcp_probe unavailable: hook tcp_cleanup_rbuf for ack processing
**[V]** Optional kprobe:
```
#[kprobe]
pub fn tcp_cleanup_rbuf_trace(ctx: ProbeContext) -> u32 {
    // Extract RTT from tcp_sock structure
    // Write to LATENCY_EVENTS
    0
}
```
**[W]** kprobe compiles (optional)
**[D]** If complex: Skip until Phase 5
**[R]** Non-critical fallback
**Time:** 5 min

### Step 77: Extract queue delay from network subsystem
**[B]** Measure time packet spent in NIC/kernel queue
**[V]** Use tracepoint net:net_dev_xmit or kprobe to estimate queue delay from timestamps
**[W]** Queue delay estimated
**[D]** If unavailable: Set queue_delay_ns = 0 (optional field)
**[R]** If measurement fails: Skip queue delay, track RTT only
**Time:** 4 min

### Step 78: Write latency events to ring buffer
**[B]** Stream RTT/queue delay to userspace for processing
**[V]** LatencyEvent written via ring_buffer.output() to LATENCY_EVENTS
**[W]** Ring buffer accepts events without overflow
**[D]** BPF verifier error "stack corruption": LatencyEvent too large for stack. FIX: Reduce struct size or allocate on heap.
**[R]** If ring buffer full: Increase size or reduce sampling rate
**Time:** 4 min

### Step 79: Implement sampling to avoid overload
**[B]** Sample 1 in N packets to reduce CPU load
**[V]** Only write event if `packet_count % SAMPLE_RATE == 0` (e.g., every 100th packet)
**[W]** Events sampled without verification loss
**[D]** If sampling too aggressive: Increase SAMPLE_RATE (e.g., 1000)
**[R]** If no sampling: Ring buffer may overflow on high-traffic links
**Time:** 3 min

### Step 80: Compile latency_probe tracepoint program
**[B]** Compile to BPF bytecode
**[V]** `cd ~/tmp/unheaded/ebpf/latency_probe && cargo build --release --target bpfel64-unknown-linux-gnu`
**[W]** ELF compiled, no verifier errors
**[D]** Compiler/verifier errors: Review Steps 70-79; See Appendix A
**[R]** If compilation fails: Simplify to stub
**Time:** 8 min

### Step 81: Verify latency_probe ELF format and tracepoint section
**[B]** Confirm valid tracepoint ELF section
**[V]** `llvm-objdump -d target/bpfel64-unknown-linux-gnu/release/latency_probe | grep -i tracepoint`; `file` shows ELF
**[W]** ELF valid, tracepoint section present
**[D]** If no tracepoint section: Check #[tracepoint] macro, aya-ebpf version
**[R]** If ELF invalid: Recompile
**Time:** 3 min

### Step 82: Load latency_probe tracepoint program
**[B]** Load compiled ELF via cilium/ebpf
**[V]** Go loader:
```
func LoadLatencyProbe(progPath string) (*ebpf.Program, error) {
    spec, err := ebpf.LoadCollectionSpec(progPath)
    coll, err := ebpf.NewCollection(spec)
    return coll.Programs["tcp_probe"], err
}
```
**[W]** Loader compiles
**[D]** If section name mismatch: Adjust program name
**[R]** If load fails: Check ELF path
**Time:** 3 min

### Step 83: Attach latency_probe to tcp_probe tracepoint
**[B]** Enable tracepoint instrumentation
**[V]** Tracepoint attached via cilium/ebpf or `bpftool prog attach id <PROG_ID> tracepoint tcp_probe`
**[W]** `bpftool prog list | grep tracepoint` shows program
**[D]** Error "tracepoint not found": Check if tcp_probe available (`cat /sys/kernel/debug/tracing/events/tcp/tcp_probe/`). If not, use different tracepoint.
**[R]** If attachment fails: Use kprobe fallback (Step 84)
**Time:** 4 min

### Step 84: Implement kprobe attachment fallback
**[B]** If tcp_probe unavailable, attach kprobe to tcp_cleanup_rbuf
**[V]** `bpftool prog attach id <PROG_ID> kprobe tcp_cleanup_rbuf`
**[W]** Kprobe attached
**[D]** If kprobe also fails: Manual perf/ebpf attachment
**[R]** If both fail: Measure RTT from application level (defer to Phase 5)
**Time:** 4 min

### Step 85: Verify tracepoint/kprobe attachment
**[B]** Confirm probe is attached and active
**[V]** `bpftool prog show id <PROG_ID>` shows attachment, read counter increments
**[W]** Program attached, counter > 0
**[D]** If counter stuck at 0: Probe may not be triggered; check filtering or tracepoint eligibility
**[R]** If attachment fails: Verify kernel support for tcp_probe
**Time:** 3 min

### Step 86: Read latency events from ring buffer
**[B]** Stream RTT measurements from BPF to userspace
**[V]** Go program creates ring buffer reader:
```
rd, err := ebpf.NewRingReader(ringBufMap)
// Read events in loop:
// record, err := rd.Read()
// Process LatencyEvent
```
**[W]** Events read from ring buffer
**[D]** If ring buffer empty: tcp_probe may not be firing; check system traffic or tracepoint enable
**[R]** If reader fails: Adjust buffer size or check map permissions
**Time:** 5 min

### Step 87: Generate test traffic to trigger latency probe
**[B]** Create network traffic to exercise tcp_probe
**[V]** `iperf3 -c localhost` or remote TCP connection
**[W]** Network traffic flowing
**[D]** If no traffic: Use ping or curl
**[R]** If traffic generation fails: Proceed to Phase 4 (offline testing)
**Time:** 3 min

### Step 88: Verify latency events appear in ring buffer
**[B]** Confirm RTT measurements are collected
**[V]** Ring buffer contains LatencyEvent entries with non-zero rtt_ns values
**[W]** RTT values in expected range (1ms - 100ms for localhost, 10-500ms for remote)
**[D]** If ring buffer empty: Verify tracepoint is enabled (`echo 1 > /sys/kernel/debug/tracing/events/tcp/tcp_probe/enable`). If events exist but RTT=0: Check field parsing (Step 75).
**[R]** If events absent: Implement alternative measurement method (Step 89)
**Time:** 4 min

### Step 89: Implement alternative RTT measurement (packet send/receive timestamps)
**[B]** If tcp_probe unavailable, calculate RTT from send and ack packet timestamps
**[V]** Correlate send timestamp (from packet_marker) with ack timestamp to estimate RTT
**[W]** Estimated RTT values available
**[D]** If correlation fails: RTT estimates may be inaccurate; use with caution
**[R]** If timestamp unavailable: RTT measurement deferred to Phase 5
**Time:** 6 min

### Step 90: Pin LATENCY_EVENTS ring buffer
**[B]** Persist ring buffer across program reloads
**[V]** `bpftool map pin id <LATENCY_EVENTS_ID> /sys/fs/bpf/latency_events`
**[W]** `ls -la /sys/fs/bpf/latency_events` exists
**[D]** If pin fails: Non-critical; ring buffer recreates on reload
**[R]** Continue to Step 91
**Time:** 2 min

### Step 91: Create test harness for latency_probe
**[B]** Automated test to verify RTT collection end-to-end
**[V]** Test:
1. Load latency_probe ELF
2. Attach to tcp_probe tracepoint
3. Send TCP traffic
4. Read ring buffer
5. Verify latency events with rtt_ns > 0
**[W]** Test passes
**[D]** If test fails: Debug each step; check tracepoint enable status
**[R]** If tcp_probe unavailable: Use kprobe fallback
**Time:** 6 min

### Step 92: Measure latency_probe overhead
**[B]** Establish CPU and latency cost of tracepoint instrumentation
**[V]** Measure system latency with/without latency_probe attached; overhead < 5% CPU
**[W]** CPU overhead measured
**[D]** If overhead high: Reduce sampling rate (Step 79)
**[R]** If unacceptable: Defer latency measurement to Phase 5+
**Time:** 5 min

### Step 93: Integrate latency_probe into trace-collector loader
**[B]** trace-collector loads packet_marker + flow_tracker + latency_probe
**[V]** loader.go updated:
```
func LoadAllPrograms(...) (map[string]*ebpf.Program, error) {
    // Load xdp, tc, latency_probe
    // Return all three programs
}
```
**[W]** Loader compiles
**[D]** If compilation fails: Check import paths
**[R]** If loading fails: Load programs with fallbacks (skip latency_probe if unavailable)
**Time:** 3 min

### Step 94: Create unified latency aggregation in trace-collector
**[B]** Aggregate RTT measurements by flow for dashboard visualization
**[V]** Go program:
```
type FlowLatencyStats struct {
    Flow5Tuple
    MinRTT, MaxRTT, AvgRTT uint64
    P50, P95, P99 uint64
}
```
**[W]** Aggregation struct defined and compiles
**[D]** If struct too complex: Simplify to min/max/avg only
**[R]** Non-critical; basic aggregation in Phase 5
**Time:** 4 min

### Step 95: Debug BPF verifier rejections in tracepoint program (Appendix A reference)
**[B]** If Step 80 showed verifier errors, apply fixes
**[V]** Program recompiles
**[D]** Common tracepoint errors:
- "invalid context": TracePointContext differs from expected. Check aya-ebpf API.
- "stack corruption": LatencyEvent too large. Reduce struct or use heap allocation.
See Appendix A for detailed fixes.
**[R]** If verifier persistent: Simplify to stub that writes dummy events
**Time:** 8 min

### Step 96: Document latency_probe tracepoint program
**[B]** Write design doc
**[V]** ~/tmp/unheaded/docs/latency_probe.md describes:
- tcp_probe tracepoint hook point
- Ring buffer event structure (LatencyEvent)
- RTT extraction logic
- Queue delay measurement (if available)
- Sampling strategy (1 in N)
- Fallback to kprobe if tcp_probe unavailable
**[W]** Doc complete
**[D]** If incomplete: Add sections
**[R]** Non-critical
**Time:** 4 min

### Step 97: PHASE 3 EXIT GATE
**[B]** Verify latency_probe fully functional and integrated
**[V]**
- Program compiles without verifier errors (Step 80)
- Attaches to tcp_probe tracepoint or kprobe (Step 83/84)
- Collects RTT measurements in ring buffer (Step 88)
- CPU overhead < 5% (Step 92)
- Integrated with other programs in loader (Step 93)
**[D]** If any assertion failed: Return to failed step
**[C]** `cd ~/tmp/unheaded && git add -A && git commit -m "S38-Phase3: latency_probe tracepoint program complete and tested"`
**[W]** Commit hash, test results
**Time:** 2 min

---

## PHASE 4: TRACE-COLLECTOR GO UNIFICATION (Steps 98-130)

### Step 98: Create cmd/trace-collector-go/main.go skeleton
**[B]** Main entry point for unified trace collection binary
**[V]** main.go contains:
```
package main
import "flag"

func main() {
    nic := flag.String("nic", "ens0", "Network interface")
    flag.Parse()

    // Load all BPF programs
    // Attach to network interface
    // Start reader loops
    // Publish to Wotan
}
```
**[W]** Skeleton compiles
**[D]** If import errors: Check module paths
**[R]** Continue to Step 99
**Time:** 3 min

### Step 99: Implement command-line flags and config
**[B]** Support customizable binary behavior (NIC, Wotan address, sample rate, etc.)
**[V]** Flags:
- --nic (network interface, default "ens0")
- --wotan-addr (Wotan gRPC address, default "localhost:18001")
- --xdp-path (packet_marker ELF path)
- --tc-path (flow_tracker ELF path)
- --latency-path (latency_probe ELF path)
- --sample-rate (1 in N sampling, default 100)
**[W]** Flags parse and validate
**[D]** If validation fails: Add constraints or defaults
**[R]** Continue to Step 100
**Time:** 4 min

### Step 100: Implement BPF program loader with error handling
**[B]** Load all three BPF programs with graceful fallbacks
**[V]** LoadAllPrograms():
```
func LoadAllPrograms(cfg Config) (ProbeSet, error) {
    xdp, err := LoadPacketMarker(cfg.XDPPath)
    if err != nil && !cfg.AllowFailure { return nil, err }
    // Repeat for tc, latency_probe
    return ProbeSet{xdp, tc, latency}, nil
}
```
**[W]** Loader function compiles and handles errors
**[D]** If ELF paths missing: Infer from standard locations or error
**[R]** If loading fails: Allow partial loading (e.g., xdp+tc without latency)
**Time:** 5 min

### Step 101: Implement XDP attachment and ring buffer reader
**[B]** Attach packet_marker XDP and stream trace IDs
**[V]** AttachXDP():
```
func AttachXDP(xdp *ebpf.Program, nic string) error {
    link, err := link.AttachXDP(link.XDPOptions{
        Program:   xdp,
        Interface: nic,
        Flags:     ebpf.XDPGenericMode,
    })
    return err
}
```
**[W]** XDP attachment succeeds
**[D]** Error "device busy": Detach existing XDP first. Error "no such device": Wrong NIC name.
**[R]** If attachment fails: Try XDP_GENERIC mode; if still fails, skip packet_marker
**Time:** 5 min

### Step 102: Implement TC attachment and map reader
**[B]** Attach flow_tracker TC and read flow states
**[V]** AttachTC():
```
func AttachTC(tc *ebpf.Program, nic string) error {
    // Use cilium/ebpf TC linking or shell out to tc command
    // Attach to ingress qdisc
}
```
**[W]** TC attachment succeeds
**[D]** Error "no ingress qdisc": Create with `tc qdisc add dev ens0 ingress`. If cilium/ebpf doesn't support: Use exec to call tc binary.
**[R]** If attachment fails: Skip flow_tracker
**Time:** 5 min

### Step 103: Implement tracepoint/kprobe attachment
**[B]** Attach latency_probe and stream ring buffer events
**[V]** AttachProbe():
```
func AttachProbe(probe *ebpf.Program, tp string) error {
    link, err := link.Tracepoint("tcp", "tcp_probe", probe)
    if err != nil {
        // Fallback to kprobe
        link, err = link.Kprobe("tcp_cleanup_rbuf", probe)
    }
    return err
}
```
**[W]** Probe attachment succeeds (tracepoint or kprobe)
**[D]** Both fail: Skip latency measurement
**[R]** If either succeeds: Continue to Step 104
**Time:** 5 min

### Step 104: Create packet_marker trace ID reader loop
**[B]** Continuously read TRACE_ID_MAP and emit trace events
**[V]** ReaderPacketMarker():
```
func ReaderPacketMarker(tmap *ebpf.Map, ch chan<- Trace) {
    for {
        var key, val uint32
        iter := tmap.Iterate()
        for iter.Next(&key, &val) {
            ch <- Trace{
                TraceID: uint64(val),
                // ... populate other fields
            }
        }
        time.Sleep(100 * time.Millisecond)
    }
}
```
**[W]** Reader compiles and runs without deadlock
**[D]** If reader hangs: Add timeout to map iteration
**[R]** Continue to Step 105
**Time:** 5 min

### Step 105: Create flow_tracker state reader loop
**[B]** Continuously read FLOW_MAP and emit flow statistics
**[V]** ReaderFlowTracker():
```
func ReaderFlowTracker(fmap *ebpf.Map, ch chan<- Flow) {
    for {
        var key Flow5Tuple
        var val FlowState
        iter := fmap.Iterate()
        for iter.Next(&key, &val) {
            ch <- Flow{
                Tuple: key,
                State: val,
            }
        }
        time.Sleep(500 * time.Millisecond)
    }
}
```
**[W]** Reader compiles
**[D]** If map structure doesn't match BPF: Adjust Go struct to match Rust definition
**[R]** Continue to Step 106
**Time:** 5 min

### Step 106: Create latency_probe ring buffer reader loop
**[B]** Stream RTT measurements from ring buffer
**[V]** ReaderLatencyProbe():
```
func ReaderLatencyProbe(rbMap *ebpf.Map, ch chan<- LatencyEvent) {
    rd, _ := ebpf.NewRingReader(rbMap)
    for {
        record, _ := rd.Read()
        var event LatencyEvent
        record.RawSample // Unmarshal into event
        ch <- event
    }
}
```
**[W]** Reader compiles and streams events
**[D]** If ring buffer empty: Check if probe is enabled
**[R]** Continue to Step 107
**Time:** 5 min

### Step 107: Create trace correlation engine
**[B]** Merge packet_marker, flow_tracker, and latency_probe data into unified trace records
**[V]** CorrelateTraces():
```
type UnifiedTrace struct {
    TraceID uint64
    Flow5Tuple
    FlowState
    Latency LatencyEvent
    Timestamp int64
}

func CorrelateTraces(packets, flows, latencies chan interface{}) chan UnifiedTrace {
    // Correlate by trace_id or 5-tuple
    // Emit unified records
}
```
**[W]** Correlation engine compiles
**[D]** If correlation logic complex: Implement simple join on trace_id first
**[R]** Continue to Step 108
**Time:** 6 min

### Step 108: Implement trace filtering by protocol
**[B]** Allow filtering traces by protocol (TCP, UDP, ICMP)
**[V]** Filter:
```
func FilterByProto(proto uint8, trace UnifiedTrace) bool {
    return trace.Proto == proto
}
```
**[W]** Filter compiles
**[D]** Non-critical feature; can be added in Phase 5
**[R]** Continue to Step 109
**Time:** 3 min

### Step 109: Implement trace sampling in userspace
**[B]** Reduce trace volume by sampling (1 in N)
**[V]** Sampler:
```
var count = 0
if count % cfg.SampleRate == 0 {
    publish(trace)
}
count++
```
**[W]** Sampling applied
**[D]** If sampling too aggressive: Increase SampleRate
**[R]** Continue to Step 110
**Time:** 2 min

### Step 110: Implement graceful shutdown with signal handling
**[B]** Cleanly detach BPF programs on SIGTERM/SIGINT
**[V]** main.go:
```
sigChan := make(chan os.Signal, 1)
signal.Notify(sigChan, syscall.SIGTERM, syscall.SIGINT)
<-sigChan
// Detach all programs, close maps
os.Exit(0)
```
**[W]** Signal handler compiles
**[D]** Non-critical; error exit acceptable in Phase 4
**[R]** Continue to Step 111
**Time:** 3 min

### Step 111: Implement BPF map cleanup on shutdown
**[B]** Unpin maps and close file descriptors
**[V]** Cleanup():
```
func Cleanup(progs map[string]*ebpf.Program) {
    for _, p := range progs {
        p.Close()
    }
    // Unpin maps from /sys/fs/bpf
}
```
**[W]** Cleanup function compiles
**[D]** If cleanup fails: Maps may persist; manual cleanup via `rm /sys/fs/bpf/*`
**[R]** Continue to Step 112
**Time:** 3 min

### Step 112: Implement logging and debug output
**[B]** Log program attachment, reader events, errors
**[V]** Use standard Go log package or structured logging (logrus, zap)
```
log.Printf("Attached packet_marker XDP to %s", nic)
log.Printf("Loaded %d traces from packet_marker", count)
```
**[W]** Logging compiles and outputs as expected
**[D]** If logs verbose: Add verbosity flag
**[R]** Continue to Step 113
**Time:** 3 min

### Step 113: Implement metrics collection (packet count, latency stats)
**[B]** Collect performance metrics for monitoring
**[V]** Metrics:
```
type Metrics struct {
    PacketsSeen uint64
    FlowsTracked uint64
    LatencyP50, P95, P99 uint64
}
```
**[W]** Metrics struct defined
**[D]** Non-critical; basic metrics sufficient
**[R]** Continue to Step 114
**Time:** 3 min

### Step 114: Compile trace-collector Go binary
**[B]** Build standalone executable
**[V]** `cd ~/tmp/unheaded && CGO_ENABLED=0 go build -o trace-collector ./cmd/trace-collector-go/main.go`
**[W]** Binary compiled to trace-collector
**[D]** Build errors: Check import paths, run `go mod tidy`
**[R]** If build fails: Review Go code for syntax errors
**Time:** 5 min

### Step 115: Test trace-collector binary (offline mode, no Wotan)
**[B]** Run binary and verify programs load, attach, and read data
**[V]** `sudo ./trace-collector -nic=ens0` (requires root)
Should output:
```
Loaded packet_marker XDP
Loaded flow_tracker TC
Loaded latency_probe tracepoint
```
**[W]** Program attachments logged, reader loops active
**[D]** If attachment fails: Check NIC name, run as root, review error messages
**[R]** If reader empty: Generate test traffic (Step 33, 58, 87)
**Time:** 5 min

### Step 116: Test trace-collector with test traffic
**[B]** Verify unified trace output
**[V]** Send test packets while ./trace-collector runs:
```
ping 8.8.8.8 &
sleep 2
kill %1
```
Expected output: Traces with trace_id, flow state, latency measurements
**[W]** Trace records output or logged
**[D]** If no traces: Check packet_marker, flow_tracker, latency_probe individually (Steps 32-34, 56-59, 86-88)
**[R]** If traces incomplete: Integrate missing components in Phase 5
**Time:** 5 min

### Step 117: Verify trace-collector with iperf3 throughput test
**[B]** Test with sustained traffic
**[V]** `iperf3 -s &` + `iperf3 -c localhost` while ./trace-collector runs
Should output: Multiple flow traces, increasing flow state transitions
**[W]** Traces generated continuously
**[D]** If no traces: Check reader loops are running (add debug logs)
**[R]** Continue to Phase 5
**Time:** 5 min

### Step 118: Implement /sys/fs/bpf persistence (optional feature)
**[B]** Pin all maps to /sys/fs/bpf for reuse across runs
**[V]** trace-collector PinMaps():
```
func PinMaps(spec *ebpf.CollectionSpec) {
    // bpftool map pin id <ID> /sys/fs/bpf/<name>
    // Or use ebpf.Map.Pin() if available
}
```
**[W]** Maps pinned
**[D]** If pinning fails: Continue without (maps recreate on next run)
**[R]** Non-critical for Phase 4
**Time:** 4 min

### Step 119: Create unit tests for trace correlation
**[B]** Test trace merge logic independently
**[V]** _test.go:
```
func TestCorrelateTraces(t *testing.T) {
    // Mock packet, flow, latency data
    // Call CorrelateTraces
    // Verify output structure
}
```
**[W]** Tests compile and pass
**[D]** If tests fail: Fix correlation logic (Step 107)
**[R]** Non-critical; integration tests in Phase 5
**Time:** 5 min

### Step 120: Create integration test (load all programs, read traces)
**[B]** Full end-to-end test
**[V]** Test:
1. Load all three BPF programs
2. Attach to ens0
3. Send 100 packets via ping
4. Read traces from all three sources
5. Verify trace count > 0
**[W]** Test passes
**[D]** If test fails: Debug each program individually
**[R]** Continue to Phase 5
**Time:** 8 min

### Step 121: Implement dry-run mode (load programs, don't attach)
**[B]** Allow testing without modifying network stack
**[V]** Flag --dry-run:
```
if cfg.DryRun {
    // Load programs but don't attach
    // Print summary and exit
}
```
**[W]** Dry-run works
**[D]** Non-critical feature
**[R]** Continue to Step 122
**Time:** 3 min

### Step 122: Create configuration file support (YAML/JSON)
**[B]** Allow config via file instead of flags only
**[V]** Config file example:
```yaml
nic: ens0
wotan_addr: localhost:18001
xdp_path: /path/to/packet_marker.o
sample_rate: 100
```
**[W]** Config file parsing works
**[D]** Non-critical for Phase 4
**[R]** Implement in Phase 5 if needed
**Time:** 4 min

### Step 123: Document trace-collector binary usage
**[B]** Write usage guide
**[V]** ~/tmp/unheaded/docs/trace-collector.md:
- Binary compilation and build instructions
- Command-line flags
- Required BPF ELF paths
- Example usage: `sudo ./trace-collector -nic=ens0`
- Troubleshooting (common errors)
**[W]** Doc complete
**[D]** Non-critical; can update later
**[R]** Continue to Step 124
**Time:** 4 min

### Step 124: PHASE 4 EXIT GATE
**[B]** Verify trace-collector unification complete
**[V]**
- Binary compiles (Step 114)
- Loads all three BPF programs (Step 115)
- Attaches to network interface (Step 115)
- Streams unified traces (Step 116)
- Handles test traffic (Step 117)
- Integration test passes (Step 120)
**[D]** If any assertion failed: Return to failed step
**[C]** `cd ~/tmp/unheaded && git add -A && git commit -m "S38-Phase4: trace-collector Go unification complete"`
**[W]** Commit hash, test results
**Time:** 2 min

---

## PHASE 5: WOTAN INTEGRATION (Steps 125-150)

### Step 125: Generate or obtain Wotan gRPC message definitions
**[B]** Define Trace message format for Wotan publishing
**[V]** Create ~/tmp/unheaded/proto/trace.proto:
```protobuf
syntax = "proto3";
message Trace {
    uint64 trace_id = 1;
    uint32 src_ip = 2;
    uint32 dst_ip = 3;
    uint16 src_port = 4;
    uint16 dst_port = 5;
    uint8 proto = 6;
    uint64 timestamp_ns = 7;
    uint64 rtt_ns = 8;
}
```
**[W]** proto file created
**[D]** If proto format differs: Adjust to match Wotan schema (from S36 spec)
**[R]** Continue to Step 126
**Time:** 4 min

### Step 126: Compile Protobuf definition to Go code
**[B]** Generate Go message types from proto
**[V]** `protoc --go_out=. --go-grpc_out=. ~/tmp/unheaded/proto/trace.proto`
**[W]** trace.pb.go and trace_grpc.pb.go generated
**[D]** If protoc not found: `apt-get install -y protobuf-compiler`
**[R]** If compilation fails: Check proto syntax
**Time:** 3 min

### Step 127: Create Wotan gRPC client in trace-collector
**[B]** Connect to Wotan service on port 18001
**[V]** wotan-publisher.go:
```
import "google.golang.org/grpc"

func NewWotanClient(addr string) (pb.WotanClient, error) {
    conn, err := grpc.Dial(addr, grpc.WithInsecure())
    if err != nil { return nil, err }
    return pb.NewWotanClient(conn), nil
}
```
**[W]** Client creation compiles
**[D]** If gRPC import missing: `go get google.golang.org/grpc`
**[R]** Continue to Step 128
**Time:** 4 min

### Step 128: Implement trace publishing to Wotan topic
**[B]** Send unified traces to Wotan message bus
**[V]** PublishTrace():
```
func PublishTrace(client pb.WotanClient, trace *pb.Trace) error {
    ctx, cancel := context.WithTimeout(context.Background(), time.Second)
    defer cancel()
    _, err := client.PublishTrace(ctx, &pb.PublishTraceRequest{
        Topic: "ebpf-traces",
        Trace: trace,
    })
    return err
}
```
**[W]** Publishing function compiles
**[D]** If RPC method name differs: Check Wotan API (S36 spec)
**[R]** Continue to Step 129
**Time:** 4 min

### Step 129: Implement batched publishing (send N traces per RPC call)
**[B]** Reduce RPC overhead by batching
**[V]** BatchPublisher:
```
type BatchPublisher struct {
    buffer []*pb.Trace
    batch_size int
}

func (bp *BatchPublisher) Publish(trace *pb.Trace) error {
    bp.buffer = append(bp.buffer, trace)
    if len(bp.buffer) >= bp.batch_size {
        return bp.Flush()
    }
    return nil
}
```
**[W]** Batcher compiles
**[D]** Non-critical; can publish one-by-one in Phase 4
**[R]** Continue to Phase 6
**Time:** 5 min

### Step 130: Test Wotan connection (ping Wotan health RPC)
**[B]** Verify Wotan service is reachable
**[V]** trace-collector main.go:
```
client, _ := NewWotanClient(cfg.WotanAddr)
_, err := client.Health(ctx, &pb.HealthRequest{})
if err != nil {
    log.Printf("Wotan unreachable: %v", err)
    return
}
log.Printf("Connected to Wotan at %s", cfg.WotanAddr)
```
**[W]** Health check succeeds or logs error
**[D]** If health check fails: Verify Wotan running on port 18001
**[R]** If Wotan unavailable: Skip Phase 5, continue to Phase 6 (offline mode)
**Time:** 4 min

### Step 131: Integrate Wotan publishing into main trace loop
**[B]** Stream traces to Wotan while trace-collector runs
**[V]** main.go reader loop:
```
for trace := range traceChan {
    pbTrace := convertToProto(trace)
    PublishTrace(wotanClient, pbTrace)
}
```
**[W]** Publishing integrated, no deadlocks
**[D]** If publish blocks: Add timeout or async publishing
**[R]** Continue to Step 132
**Time:** 3 min

### Step 132: Implement async publishing with worker pool
**[B]** Publish to Wotan without blocking trace readers
**[V]** PublisherPool:
```
type PublisherPool struct {
    workers int
    traceChan chan *pb.Trace
}

func (pp *PublisherPool) Worker(client pb.WotanClient) {
    for trace := range pp.traceChan {
        PublishTrace(client, trace)
    }
}
```
**[W]** Worker pool compiles
**[D]** Non-critical; synchronous publishing acceptable in Phase 5
**[R]** Continue to Step 133
**Time:** 5 min

### Step 133: Implement error handling and retry logic
**[B]** Handle Wotan RPC failures gracefully
**[V]** PublishWithRetry():
```
func PublishWithRetry(client pb.WotanClient, trace *pb.Trace, maxRetries int) error {
    for i := 0; i < maxRetries; i++ {
        err := PublishTrace(client, trace)
        if err == nil { return nil }
        time.Sleep(time.Duration(2^i) * 100 * time.Millisecond)
    }
    return fmt.Errorf("failed after %d retries", maxRetries)
}
```
**[W]** Retry logic compiles
**[D]** Non-critical; at-most-once delivery acceptable
**[R]** Continue to Step 134
**Time:** 4 min

### Step 134: Implement trace deduplication before publishing
**[B]** Avoid sending duplicate traces to Wotan
**[V]** Deduplicator:
```
type Dedup struct {
    seen map[uint64]bool // keyed on trace_id
}

func (d *Dedup) IsDuplicate(t *pb.Trace) bool {
    if d.seen[t.TraceId] { return true }
    d.seen[t.TraceId] = true
    return false
}
```
**[W]** Deduplicator compiles
**[D]** Non-critical; duplicates acceptable
**[R]** Continue to Step 135
**Time:** 4 min

### Step 135: Implement rate limiting to Wotan (max N traces/sec)
**[B]** Prevent overloading Wotan with traces
**[V]** RateLimiter:
```
type RateLimiter struct {
    limiter *rate.Limiter
}

func (rl *RateLimiter) Allow(t *pb.Trace) bool {
    return rl.limiter.Allow()
}
```
**[W]** Rate limiter compiles (use golang.org/x/time/rate)
**[D]** Non-critical for Phase 5
**[R]** Continue to Step 136
**Time:** 3 min

### Step 136: Test trace-collector with Wotan publishing
**[B]** Verify traces appear in Wotan
**[V]**
1. Start Wotan on port 18001
2. Run trace-collector: `sudo ./trace-collector -wotan-addr=localhost:18001`
3. Send test traffic: `ping 8.8.8.8`
4. Query Wotan for traces: `curl localhost:18001/traces?limit=10`
Expected: Trace records visible in Wotan
**[W]** Traces published and visible in Wotan
**[D]** If traces missing: Check Wotan RPC response, verify publishing code (Step 131)
**[R]** If Wotan unavailable: Continue to Phase 6 (offline mode)
**Time:** 5 min

### Step 137: Implement trace tagging (source, environment, version)
**[B]** Add metadata to traces for Wotan filtering
**[V]** Trace struct extended:
```
type Trace struct {
    // ... existing fields
    Source string      // "ebpf-collector"
    Environment string // "production"
    Version string     // "S38-v1"
}
```
**[W]** Tagging compiles
**[D]** Non-critical feature
**[R]** Continue to Step 138
**Time:** 3 min

### Step 138: Implement trace filtering before Wotan publish
**[B]** Allow sampling or filtering by 5-tuple, protocol, etc.
**[V]** Filter functions:
```
func FilterByProto(t *pb.Trace, proto uint8) bool {
    return t.Proto == uint32(proto)
}
```
**[W]** Filters compile
**[D]** Non-critical; all traces acceptable
**[R]** Continue to Step 139
**Time:** 3 min

### Step 139: Document Wotan integration
**[B]** Write integration guide
**[V]** ~/tmp/unheaded/docs/wotan-integration.md:
- Wotan service requirements (port, gRPC)
- Trace message proto definition
- Publishing RPC calls
- Example: Publishing 100 traces to Wotan
- Troubleshooting (connection failures, timeout, rate limits)
**[W]** Doc complete
**[D]** Non-critical
**[R]** Continue to Step 140
**Time:** 4 min

### Step 140: Create Wotan health check monitor
**[B]** Periodically verify Wotan connectivity, alert on failure
**[V]** HealthChecker:
```
func (hc *HealthChecker) Monitor(client pb.WotanClient) {
    ticker := time.NewTicker(10 * time.Second)
    for range ticker.C {
        _, err := client.Health(ctx, &pb.HealthRequest{})
        if err != nil {
            log.Printf("WARNING: Wotan unreachable")
        }
    }
}
```
**[W]** Health monitor compiles
**[D]** Non-critical for Phase 5
**[R]** Continue to PHASE 5 EXIT GATE
**Time:** 4 min

### Step 141: PHASE 5 EXIT GATE
**[B]** Verify Wotan integration complete
**[V]**
- Wotan gRPC client created (Step 127)
- Traces published to Wotan (Step 131)
- Integration test passes (Step 136)
- Traces visible in Wotan dashboard/API
**[D]** If any assertion failed: Return to failed step
**[C]** `cd ~/tmp/unheaded && git add -A && git commit -m "S38-Phase5: Wotan integration complete"`
**[W]** Commit hash, test results
**Time:** 2 min

---

## PHASE 6: DASHBOARD VISUALIZATION (Steps 142-165)

### Step 142: Implement dashboard API endpoint for traces
**[B]** Expose HTTP API for frontend to query traces
**[V]** main.go HTTP server:
```
http.HandleFunc("/api/traces", handleGetTraces)
http.HandleFunc("/api/flows", handleGetFlows)
http.HandleFunc("/api/latency", handleGetLatency)
log.Fatal(http.ListenAndServe(":8080", nil))
```
**[W]** HTTP server compiles and listens on port 8080
**[D]** If port conflicts: Use different port (8081, 8082)
**[R]** Continue to Step 143
**Time:** 4 min

### Step 143: Implement /api/traces endpoint (trace table)
**[B]** Return JSON array of recent traces
**[V]** GET /api/traces?limit=100&offset=0 returns:
```json
[
  {
    "trace_id": 12345,
    "src_ip": "10.0.0.1",
    "dst_ip": "8.8.8.8",
    "rtt_ns": 50000000,
    "timestamp": 1234567890
  }
]
```
**[W]** Endpoint returns valid JSON
**[D]** If response format differs: Adjust to match frontend expectations
**[R]** Continue to Step 144
**Time:** 4 min

### Step 144: Implement /api/flows endpoint (flow statistics)
**[B]** Return active flows with state and counters
**[V]** GET /api/flows?limit=50 returns:
```json
[
  {
    "src_ip": "10.0.0.1",
    "dst_ip": "8.8.8.8",
    "src_port": 12345,
    "dst_port": 443,
    "state": "ESTABLISHED",
    "packets_in": 1000,
    "packets_out": 950,
    "bytes_in": 500000
  }
]
```
**[W]** Endpoint returns flow data
**[D]** Non-critical; detailed stats can be added later
**[R]** Continue to Step 145
**Time:** 4 min

### Step 145: Implement /api/latency endpoint (latency statistics)
**[B]** Return per-flow RTT percentiles
**[V]** GET /api/latency?flow=10.0.0.1:8.8.8.8 returns:
```json
{
  "src_ip": "10.0.0.1",
  "dst_ip": "8.8.8.8",
  "min_rtt": 10000,
  "max_rtt": 100000,
  "p50": 30000,
  "p95": 80000,
  "p99": 95000
}
```
**[W]** Endpoint returns latency stats
**[D]** If stats unavailable: Return placeholder
**[R]** Continue to Step 146
**Time:** 4 min

### Step 146: Create React/Vue frontend for trace dashboard
**[B]** Web UI to visualize traces, flows, latency
**[V]** ~/tmp/unheaded/ui/ contains:
- index.html (page layout)
- app.js (frontend logic)
- styles.css (styling)
Features:
- Live trace table (auto-refresh every 1s)
- Flow list with state
- Latency chart (chart.js or D3)
**[W]** Frontend files created
**[D]** If React/Vue not available: Create plain HTML + jQuery
**[R]** Continue to Step 147
**Time:** 8 min

### Step 147: Implement trace table visualization
**[B]** Display recent traces in sorted table (by timestamp desc)
**[V]** Table shows columns: TraceID, SrcIP, DstIP, RTT (ms), Timestamp
**[W]** Table renders with data
**[D]** If table empty: Verify API endpoint returns data
**[R]** Continue to Step 148
**Time:** 5 min

### Step 148: Implement flow sankey diagram (source → destination)
**[B]** Visualize packet flows between IP pairs
**[V]** D3/SVG sankey diagram shows source IPs → destination IPs with line thickness proportional to packet count
**[W]** Diagram renders
**[D]** If D3 not available: Create simple HTML table instead
**[R]** Continue to Step 149
**Time:** 8 min

### Step 149: Implement latency histogram (RTT distribution)
**[B]** Visualize RTT distribution for selected flow
**[V]** Chart.js histogram shows RTT on X-axis, packet count on Y-axis
**[W]** Histogram renders with bins (0-10ms, 10-20ms, etc.)
**[D]** If chart library missing: Create bar chart using SVG
**[R]** Continue to Step 150
**Time:** 6 min

### Step 150: PHASE 6 EXIT GATE
**[B]** Verify dashboard visualization complete
**[V]**
- API endpoints return valid JSON (Steps 143-145)
- Frontend renders without errors (Step 146)
- Trace table shows recent traces (Step 147)
- Flow sankey renders (Step 148)
- Latency histogram renders (Step 149)
**[D]** If any assertion failed: Return to failed step
**[C]** `cd ~/tmp/unheaded && git add -A && git commit -m "S38-Phase6: Dashboard visualization complete"`
**[W]** Commit hash
**Time:** 2 min

---

## PHASE 7: END-TO-END VERIFICATION & PERFORMANCE BASELINE (Steps 151-180)

### Step 151: Verify all BPF programs compile without warnings
**[B]** Clean compilation with no compiler or verifier warnings
**[V]** `cargo build --release --target bpfel64-unknown-linux-gnu` for all three programs shows 0 warnings
**[W]** Build output clean
**[D]** If warnings present: Add #[allow(...)] or fix warnings
**[R]** Continue to Step 152
**Time:** 3 min

### Step 152: Verify trace-collector binary compiles and runs
**[B]** Binary ready for production
**[V]** `sudo ./trace-collector --help` displays usage; binary starts without panics
**[W]** Binary functional
**[D]** If binary panics: Add error handling
**[R]** Continue to Step 153
**Time:** 2 min

### Step 153: End-to-end smoke test: packet injection
**[B]** Send packets, verify traces captured
**[V]**
1. Start trace-collector
2. Send 100 ICMP packets: `ping -c 100 8.8.8.8`
3. Verify trace count > 0
4. Verify all traces have non-zero trace_id, timestamp_ns
**[W]** Traces captured end-to-end
**[D]** If no traces: Debug each program (packet_marker, flow_tracker, latency_probe)
**[R]** Fix failed program, retry
**Time:** 5 min

### Step 154: End-to-end test: TCP flow tracking
**[B]** Verify flow state transitions SYN → ESTABLISHED → FIN
**[V]**
1. Start trace-collector
2. Open TCP connection: `nc -l 5555 &`
3. Connect: `nc localhost 5555`
4. Close connection
5. Verify FLOW_MAP shows SYN, ESTABLISHED, FIN states
**[W]** Flow states tracked correctly
**[D]** If states not tracked: Check flow_tracker TC attachment
**[R]** Continue to Step 155
**Time:** 5 min

### Step 155: End-to-end test: latency measurement
**[B]** Verify RTT collection from latency_probe
**[V]**
1. Start trace-collector
2. Send sustained TCP traffic: `iperf3 -c localhost -t 10`
3. Read LATENCY_EVENTS ring buffer
4. Verify rtt_ns values > 0 and < 100ms (localhost)
**[W]** RTT measurements collected
**[D]** If no RTT events: Check tracepoint attachment
**[R]** Continue to Step 156
**Time:** 5 min

### Step 156: Measure packet processing latency (XDP path)
**[B]** Establish baseline latency for packet_marker XDP
**[V]**
1. Send 1000 packets via ping
2. Measure time from packet RX to trace_id write
3. Target: < 100µs latency per packet
**[W]** Latency measured and logged
**[D]** If latency > 100µs: Optimize XDP program or defer to Phase 8
**[R]** Continue to Step 157
**Time:** 6 min

### Step 157: Measure flow tracking latency (TC path)
**[B]** Establish baseline latency for flow_tracker TC
**[V]**
1. Open TCP connection
2. Measure time from packet RX to FLOW_MAP update
3. Target: < 500µs latency per packet
**[W]** Latency measured
**[D]** If latency high: Check TC classification overhead
**[R]** Continue to Step 158
**Time:** 6 min

### Step 158: Measure throughput: XDP + TC combined
**[B]** Verify both programs can handle line rate
**[V]**
1. Start trace-collector with packet_marker + flow_tracker
2. Send sustained traffic: `iperf3 -c remote-host -R -t 30` (if available)
3. Measure packets/sec, latency percentiles (p50, p95, p99)
4. Target: > 100k packets/sec with p99 < 200µs
**[W]** Throughput baseline established
**[D]** If throughput < 100k: Optimize or document as-is
**[R]** Continue to Step 159
**Time:** 10 min

### Step 159: Measure CPU overhead
**[B]** Quantify CPU impact of trace collection
**[V]**
1. Baseline CPU without trace-collector: top/htop
2. Run trace-collector with sustained traffic
3. Measure CPU delta (% increase)
4. Target: < 5% CPU increase
**[W]** CPU overhead < 5%
**[D]** If CPU overhead high: Reduce sampling rate or optimize BPF code
**[R]** Continue to Step 160
**Time:** 8 min

### Step 160: Measure memory footprint
**[B]** Quantify memory usage
**[V]**
1. Run trace-collector for 5 minutes with sustained traffic
2. Measure RSS and VSZ (top, ps aux)
3. Target: < 100MB RSS
**[W]** Memory footprint < 100MB
**[D]** If memory high: Check for leaks in map iteration or reader loops
**[R]** Continue to Step 161
**Time:** 8 min

### Step 161: Test dashboard with live traces
**[B]** Verify dashboard renders with real data
**[V]**
1. Start trace-collector with Wotan publishing
2. Start dashboard (if available)
3. Send test traffic
4. Verify dashboard shows traces, flows, latency charts
**[W]** Dashboard renders with live data
**[D]** If dashboard unavailable: Skip (Phase 6 may not be complete)
**[R]** Continue to Step 162
**Time:** 5 min

### Step 162: Test Wotan integration end-to-end
**[B]** Verify traces published to Wotan and queryable
**[V]**
1. Start Wotan on port 18001
2. Start trace-collector
3. Send test traffic
4. Query Wotan API: `curl localhost:18001/traces?limit=10`
5. Verify traces returned
**[W]** Traces published and queryable
**[D]** If Wotan unavailable: Continue offline
**[R]** Continue to Step 163
**Time:** 5 min

### Step 163: Test trace deduplication
**[B]** Verify duplicate traces not published to Wotan
**[V]**
1. Send same packet multiple times (e.g., 10x)
2. Check Wotan for duplicate trace_ids
3. Expect: Single trace entry per unique packet
**[W]** Deduplication working
**[D]** If duplicates exist: Check dedup logic (Step 134)
**[R]** Continue to Step 164
**Time:** 5 min

### Step 164: Document performance baseline
**[B]** Write performance report
**[V]** ~/tmp/unheaded/docs/performance-baseline.md:
- Packet processing latency (XDP): < 100µs
- Flow tracking latency (TC): < 500µs
- Throughput: > 100k packets/sec
- CPU overhead: < 5%
- Memory footprint: < 100MB
- Test conditions: test network, packet size, traffic pattern
**[W]** Report complete
**[D]** Non-critical; baseline documented
**[R]** Continue to Step 165
**Time:** 4 min

### Step 165: PHASE 7 EXIT GATE
**[B]** Verify end-to-end system complete and performant
**[V]**
- All BPF programs compile clean (Step 151)
- Packet injection test passes (Step 153)
- TCP flow tracking test passes (Step 154)
- Latency measurement test passes (Step 155)
- Performance baselines established (Steps 156-160)
- Dashboard renders with live data (Step 161)
- Wotan integration verified (Step 162)
**[D]** If any test failed: Return to failed step
**[C]** `cd ~/tmp/unheaded && git add -A && git commit -m "S38-Phase7: End-to-end verification and performance baseline complete"`
**[W]** Commit hash, baseline metrics
**Time:** 2 min

---

## APPENDIX A: EMERGENCY PROCEDURES

### BPF Verifier Rejection Debug

**Problem:** `error: verifier rejected program at line 42: invalid memory access`

**Common Causes & Fixes:**

1. **Pointer Arithmetic Without Bounds Check**
   - Error: `pointer out of bounds`
   - Fix: Add explicit bounds check before dereferencing
   ```rust
   if (void *)ptr + offset > ctx.data_end { return XDP_PASS; }
   let val = *(ptr as *const u32);
   ```

2. **Stack Corruption (Struct Too Large)**
   - Error: `stack corruption detected in function`
   - Fix: Reduce struct size or add #[repr(C)]
   ```rust
   #[repr(C)]
   struct PacketTrace { ... } // Max ~512 bytes
   ```

3. **Loop Unbound**
   - Error: `back-edge from insn X to insn Y`
   - Fix: Use pragma unroll or bound loop
   ```rust
   #pragma unroll
   for i in 0..32 { ... }
   ```

4. **Invalid Context**
   - Error: `invalid memory access for read from context`
   - Fix: Verify context type (XdpContext vs. SkBuffContext vs. TracePointContext)
   ```rust
   #[xdp] fn xdp_prog(ctx: XdpContext) { ... }
   #[classifier] fn tc_prog(ctx: SkBuffContext) { ... }
   ```

5. **Missing Helper Function**
   - Error: `unknown function bpf_foo`
   - Fix: Verify helper is available in BPF program type
   - XDP helpers: bpf_xdp_adjust_head, bpf_map_lookup_elem, bpf_ktime_get_ns
   - TC helpers: bpf_skb_load_bytes, bpf_map_lookup_elem, bpf_jiffies64
   - Tracepoint helpers: bpf_ringbuf_output, bpf_map_lookup_elem

6. **Alignment Issues**
   - Error: `invalid access to packet data: alignment`
   - Fix: Use __attribute__((aligned(4))) or reorder struct fields
   ```rust
   #[repr(C, align(4))]
   struct MyStruct { ... }
   ```

### BPF Map Pinning Issues

**Problem:** `error: unable to pin map to /sys/fs/bpf/my_map`

**Causes & Fixes:**

1. bpffs not mounted:
   ```bash
   mount -t bpf bpf /sys/fs/bpf
   ```

2. Permission denied:
   ```bash
   sudo chmod 777 /sys/fs/bpf
   ```

3. Map already pinned:
   ```bash
   rm /sys/fs/bpf/my_map && retry
   ```

### XDP Attachment Failures

**Problem:** `error: device busy` when attaching XDP

**Causes & Fixes:**

1. XDP already attached to device:
   ```bash
   ip link set dev ens0 xdp off
   ip link set dev ens0 xdp obj <file> sec xdp
   ```

2. Device doesn't support XDP:
   ```bash
   ethtool -S ens0 | grep xdp  # Check if supported
   ip link set dev ens0 xdp obj <file> sec xdp_generic  # Fallback
   ```

3. Wrong section name in ELF:
   ```bash
   llvm-objdump -d <file> | grep xdp  # Verify section exists
   ```

### TC Attachment Failures

**Problem:** `error: no ingress qdisc` when attaching TC classifier

**Causes & Fixes:**

1. Ingress qdisc not created:
   ```bash
   tc qdisc add dev ens0 ingress
   tc filter add dev ens0 ingress bpf da obj <file> sec classifier
   ```

2. TC not installed:
   ```bash
   apt-get install -y iproute2
   ```

3. Wrong BPF section:
   ```bash
   llvm-objdump -d <file> | grep -i classifier
   ```

### Tracepoint Attachment Failures

**Problem:** `error: tracepoint not found: tcp:tcp_probe`

**Causes & Fixes:**

1. Tracepoint not available:
   ```bash
   cat /sys/kernel/debug/tracing/events/tcp/tcp_probe/format
   # If missing, check kernel version and CONFIG_FTRACE
   ```

2. Fallback to kprobe:
   ```bash
   bpftool prog attach id <PROG_ID> kprobe tcp_cleanup_rbuf
   ```

3. Missing debugfs:
   ```bash
   mount -t debugfs debugfs /sys/kernel/debug
   ```

### Ring Buffer Overflow

**Problem:** Ring buffer drops events

**Causes & Fixes:**

1. Reader not fast enough:
   - Increase ring buffer size in Rust: `RingBuf::with_sample_size(65536)`
   - Or optimize reader loop to reduce latency

2. Sampling too aggressive:
   - Increase sample rate (e.g., 1 in 1000 instead of 1 in 100)

3. Events too large:
   - Reduce LatencyEvent struct size

---

## APPENDIX B: BPF QUICK REFERENCE

### BPF Program Types

| Type | Hook Point | Context | Use Case |
|------|-----------|---------|----------|
| XDP | NIC driver | XdpContext | Packet filtering, marking (packet_marker) |
| TC | Traffic control | SkBuffContext | Flow classification, shaping (flow_tracker) |
| Tracepoint | Kernel event | TracePointContext | System instrumentation (latency_probe) |
| Kprobe | Kernel function | ProbeContext | Function tracing, performance analysis |

### BPF Map Types

| Type | Key | Value | Use Case |
|------|-----|-------|----------|
| Array | u32 (index) | Any | Fixed-size storage, per-packet metadata |
| PerCpuArray | u32 (index) | Per-CPU value | Lock-free per-core data (trace_id_map) |
| HashMap | Any (hashable) | Any | Dynamic flow tracking (flow_map) |
| RingBuf | N/A | Events | Event streaming to userspace (latency_events) |

### Common BPF Helper Functions

| Helper | Signature | Context | Purpose |
|--------|-----------|---------|---------|
| bpf_map_lookup_elem | map, key → value | All | Read map entry |
| bpf_map_update_elem | map, key, value → int | All | Write map entry |
| bpf_map_delete_elem | map, key → int | All | Delete map entry |
| bpf_ktime_get_ns | () → u64 | XDP, TC, Kprobe | Get nanosecond timestamp |
| bpf_get_prandom_u32 | () → u32 | All | Random 32-bit integer |
| bpf_ringbuf_output | rbuf, data, size → int | All | Write to ring buffer |
| bpf_skb_load_bytes | skb, offset, buf, size → int | TC, Kprobe | Load skb bytes |
| bpf_xdp_adjust_head | ctx, delta → int | XDP | Adjust packet head pointer |

### Memory Bounds Checking Pattern

```rust
// Always check bounds before dereferencing pointers in XDP/TC
if (void *)eth_hdr + sizeof(*eth_hdr) > ctx.data_end {
    return XDP_PASS; // Safe: packet too small
}
let proto = eth_hdr.h_proto;
```

### Per-CPU Map Pattern

```rust
#[map]
static MY_PER_CPU_MAP: PerCpuArray<u64> = PerCpuArray::with_max_entries(1, 0);

fn read_or_update(value: u64) {
    // Per-CPU: no lock needed
    if let Some(slot) = MY_PER_CPU_MAP.get_mut(0) {
        *slot += value;
    }
}
```

### Ring Buffer Event Streaming Pattern

```rust
#[map]
static EVENTS: RingBuf<MyEvent> = RingBuf::new();

fn emit_event(event: MyEvent) {
    let _ = EVENTS.output(&event, 0);
}

// Userspace reader:
// rd, _ := ebpf.NewRingReader(rbMap)
// record, _ := rd.Read()
// var event MyEvent
// record.RawSample unmarshal to event
```

---

## SUMMARY

**S38 eBPF Production Battle Plan: 165 steps across 7 phases**

- **Phase 0 (15 steps):** Environment verification (kernel, tools, directories)
- **Phase 1 (25 steps):** packet_marker XDP program (trace ID marking)
- **Phase 2 (29 steps):** flow_tracker TC program (5-tuple tracking, state machine)
- **Phase 3 (36 steps):** latency_probe tracepoint program (RTT collection)
- **Phase 4 (30 steps):** trace-collector Go unification (single binary, all programs)
- **Phase 5 (17 steps):** Wotan integration (gRPC publishing, topics)
- **Phase 6 (24 steps):** Dashboard visualization (API endpoints, frontend UI)
- **Phase 7 (15 steps):** End-to-end verification (smoke tests, performance baseline)

**Key Exit Gates:** Each phase has a verification gate; all assertions must pass.

**Emergency Procedures (Appendix A):**
- BPF verifier rejection debug (6 common patterns)
- Map pinning failures
- XDP/TC/Tracepoint attachment troubleshooting
- Ring buffer overflow mitigation

**Quick Reference (Appendix B):**
- Program types and hook points
- Map types for different use cases
- Common BPF helpers with signatures
- Memory bounds checking and per-CPU patterns

**Critical Success Factors:**
1. All BPF programs must pass kernel verifier (Appendix A)
2. packet_marker XDP is THE core differentiator (Phase 1)
3. flow_tracker state machine must be correct (Phase 2)
4. latency_probe must handle tcp_probe or fallback to kprobe (Phase 3)
5. trace-collector must load all 3 programs gracefully (Phase 4)
6. Wotan integration enables dashboard visualization (Phase 5-6)
7. Performance baseline < 100µs latency, > 100k packets/sec (Phase 7)

**Git Commits:** Every phase has a [C] commit checkpoint for easy rollback.

---

**END OF S38 EBPF PRODUCTION BATTLE PLAN**
