# draft-bellis-unheaded-wotan-memory-03

**Status:** IETF Experimental · Independent Submission · March 2026

Wotan is the memory and I/O bus for the Unheaded Protocol, providing addressable per-flow storage for BPF programs executing within the Limited Domain. The Wotan protocol specifies the BPF helper interface for memory access, the address space layout for per-flow data structures, a five-level cache hierarchy (L0 Monad, L1 BPF hash maps, L2 per-flow ring buffers, L3 persistent Write-Ahead Log, L4 Sophia dictionaries), and the topic-based I/O model for interaction with userspace services. Draft-03 introduces a structured error code taxonomy with severity levels, helper return codes for common operations, and error recovery procedures.

## Key Sections

- **Introduction** — Role of Wotan: bridging stateless Monad compute to persistent per-flow memory; five-level cache hierarchy with latencies (L0 ~ns to L3 ~100us-1ms); cross-references to Foundation and Sophia
- **Terminology** — Flow Label, Ring Buffer, Cache Line, Write-Back, Memory-Mapped I/O
- **Error Code Taxonomy (NEW in draft-03)** — Five severity levels (INFO/WARNING/ERROR/CRITICAL/FATAL); structured 32-bit error code format (Severity[31:29] | Origin[28:24] | Category[23:16] | Detail[7:0]); 13 origin codes (WOTAN_CORE through SHIM_EXEC); 12 category codes; common error codes for L1 cache, L2 ring buffer, L3 WAL, gRPC, and Sophia subsystems
- **Error Recovery Procedures (NEW in draft-03)** — Recovery by severity; automatic state machine (HEALTHY → DEGRADED → RECOVERING → FAILED); recovery metrics (6 Prometheus counters/gauges); cross-subsystem dependency graph
- **Architecture Overview** — Memory hierarchy table; separation of compute (Monad) and memory (Wotan); Shim interaction model
- **BPF Helper Interface** — `bpf_wotan_read`, `bpf_wotan_write`, `bpf_wotan_cas` signatures and return codes; auxiliary error detail via `wotan_last_error` per-CPU map; recommended error handling by severity
- **UPC Memory Model Extensions (NEW in draft-03)** — ROM_MAP (256K instruction slots, read-only); RAM_MAP (1M word slots, 4 MiB); SCREEN_MAP (320x200 framebuffer); KBD_MAP (256-key state); CPU_MAP (per-flow processor state with registers, PC, flags, interrupt state, counters); address space layout (RAM/KBD_IO/SCREEN/DEBUG/WAD/HEAP/STACK regions)
- **WAL Specification (NEW in draft-03)** — 76-byte record format (8-byte timestamp + 4-byte address + 64-byte cache line); append/fsync semantics; crash recovery replay; compaction with exclusive locking; configuration parameters
- **TTY Subsystem (NEW in draft-03)** — 4 KiB circular buffer per flow; SYS_WRITE/SYS_READ operations on fd 0/1/2; overflow handling; EVENT_TTY_WRITE emission; Wotan topic compute.tty.{flow_label}
- **Security Considerations** — Topic injection attacks; ring buffer memory exhaustion (PATCH W6); cross-flow memory access (PATCH W2); CAS alignment violations (PATCH W3); WAL tampering detection (PATCH W4); WAL compaction race conditions (PATCH W5); GOAWAY frame DoS (PATCH W8); normative 13-code error cross-reference; error code information leakage
- **IANA Considerations** — Wotan Topic Namespace registry; Wotan Error Origin Registry (NEW); Wotan Error Category Registry (NEW)

## Related

- [[Protocol Foundation|Protocol-Foundation]]
- [[Draft Protocol Foundation 06|Draft-Protocol-Foundation-06]]
- [[Sophia Dictionaries|Sophia-Dictionaries]]
- [[Draft Sophia Dictionary 03|Draft-Sophia-Dictionary-03]]
- [[Drafts Index|Drafts-Index]]

---

> **Source:** [docs/protocol/draft-bellis-unheaded-wotan-memory-03.md](../docs/protocol/draft-bellis-unheaded-wotan-memory-03.md)
