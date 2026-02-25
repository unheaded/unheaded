//! Shared types for Go interop.
//!
//! These types mirror the structures in unheaded-common but are defined
//! here as documentation for the Go userspace code in cmd/trace-collector-go/flow_reader.go.
//! The canonical definitions live in ebpf/common/src/lib.rs.
//!
//! Wire layout (all little-endian unless noted):
//!
//! FlowKey (16 bytes):
//!   offset 0:  src_addr    u32   (network byte order)
//!   offset 4:  dst_addr    u32   (network byte order)
//!   offset 8:  src_port    u16   (network byte order)
//!   offset 10: dst_port    u16   (network byte order)
//!   offset 12: protocol    u8
//!   offset 13: _pad        [u8; 3]
//!
//! ConnectionState (u8):
//!   0=Unknown, 1=SynSent, 2=SynReceived, 3=Established,
//!   4=FinWait1, 5=FinWait2, 6=CloseWait, 7=Closing,
//!   8=LastAck, 9=TimeWait, 10=Closed
//!
//! FlowState (72 bytes):
//!   offset 0:  trace_id     [u8; 16]  (TraceId)
//!   offset 16: start_ns     u64
//!   offset 24: last_seen_ns u64
//!   offset 32: packets_in   u64
//!   offset 40: packets_out  u64
//!   offset 48: bytes_in     u64
//!   offset 56: bytes_out    u64
//!   offset 64: state        u8  (ConnectionState)
//!   offset 65: _pad         [u8; 7]
//!
//! FlowEvent (104 bytes):
//!   offset 0:  timestamp_ns u64
//!   offset 8:  flow_key     FlowKey (16 bytes)
//!   offset 24: state        FlowState (72 bytes)
//!   offset 96: event_type   u8 (0=New, 1=StateChange, 2=Expired, 3=Update, 4=Anomaly)
//!   offset 97: _pad         [u8; 7]

// This file is intentionally documentation-only.
// The actual types are defined in unheaded-common (ebpf/common/src/lib.rs)
// and re-exported by the flow-tracker program.
