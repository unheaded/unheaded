# MapLoader Integration Guide

## Overview

The `maploader` package provides userspace population of BPF maps from Go structs defined in `pkg/protocol/bpfschema`. This document describes how to use MapLoader in the Unheaded project.

## Architecture

```
┌─────────────────────────────────────────────────────────┐
│ Userspace Application (Go)                              │
│                                                         │
│  ┌─────────────────────────────────────────────────┐   │
│  │ pkg/protocol/bpfschema                          │   │
│  │ - HMACKeyKey / HMACKeyValue                     │   │
│  │ - SettingsKey / SettingsValue                  │   │
│  │ - HopValidatorKey / HopValidatorValue          │   │
│  │ - ... (all map types)                          │   │
│  └────────────────┬────────────────────────────────┘   │
│                   │                                      │
│  ┌────────────────▼────────────────────────────────┐   │
│  │ pkg/ebpf/maploader                              │   │
│  │ - MapLoader.LoadMap(key, value)                │   │
│  │ - MapLoader.LoadMapBatch(entries)              │   │
│  │ - MapLoader.LoadMapArray(entries)              │   │
│  │ - MapLoader.LoadErrorCounters()                │   │
│  │ - Serialization via encoding/binary (BigEndian)│   │
│  └────────────────┬────────────────────────────────┘   │
│                   │                                      │
│  ┌────────────────▼────────────────────────────────┐   │
│  │ pkg/ebpf/loader (NativeLoader)                  │   │
│  │ - BPF syscalls (BPF_MAP_UPDATE_ELEM)           │   │
│  │ - Map FD management                            │   │
│  └────────────────┬────────────────────────────────┘   │
│                   │                                      │
├───────────────────┼─────────────────────────────────────┤
│ Kernel (eBPF)     │                                     │
│                   │                                      │
│  ┌────────────────▼────────────────────────────────┐   │
│  │ BPF Maps                                        │   │
│  │ - unhd_hmac_keys (HASH)                         │   │
│  │ - unhd_settings (HASH)                          │   │
│  │ - unhd_hop_validators (HASH)                    │   │
│  │ - unhd_flow_types (ARRAY)                       │   │
│  │ - unhd_error_counters (PERCPU_ARRAY)           │   │
│  │ - ... (all maps)                                │   │
│  └─────────────────────────────────────────────────┘   │
│                                                         │
│  ┌─────────────────────────────────────────────────┐   │
│  │ eBPF Programs                                   │   │
│  │ - Shield XDP (reads blocklist, rate tokens)    │   │
│  │ - Hop XDP (reads validators, HMAC keys)        │   │
│  │ - Flow Tracker (tracks flows)                  │   │
│  │ - ... (other programs)                         │   │
│  └─────────────────────────────────────────────────┘   │
└─────────────────────────────────────────────────────────┘
```

## Wire Format

All serialization uses `encoding/binary.Write` with `binary.BigEndian` (network byte order) matching RFC 8200 (IPv6).

### Key/Value Alignment

- **Keys**: Fixed-size structs with padding fields (marked as `_`)
- **Values**: Fixed-size structs with padding fields (marked as `_`)
- **Array indices**: uint32 (4 bytes) in big-endian
- **No variable-length data**: All structs are fixed-size for BPF compatibility

### Serialization Example

```
HMACKeyKey {
  FlowID    uint16   // 2 bytes
  Namespace uint16   // 2 bytes
  _         [4]byte  // 4 bytes padding
}
// Total: 8 bytes (must match BPF_MAP_KEY_SIZE)

HMACKeyValue {
  Key       [32]byte // 32 bytes
  CreatedAt uint32   // 4 bytes
  ExpiresAt uint32   // 4 bytes
}
// Total: 40 bytes (must match BPF_MAP_VALUE_SIZE)
```

Wire format (big-endian):
```
Key bytes: [FlowID_high, FlowID_low, Namespace_high, Namespace_low, Padding...]
Value bytes: [Key[0], Key[1], ..., Key[31], CreatedAt_bytes[0:4], ExpiresAt_bytes[0:4]]
```

## Usage Patterns

### Pattern 1: Loading HASH Maps

For maps with arbitrary key-value pairs:

```go
import (
	"pkg/ebpf"
	"pkg/ebpf/maploader"
	"pkg/protocol/bpfschema"
)

// Get the eBPF loader
loader, err := ebpf.NewNativeLoader(config)
if err != nil { /* handle error */ }

// Load programs (maps are created automatically)
_, err = loader.Load(ctx, "/path/to/program.bpf.o")
if err != nil { /* handle error */ }

// Get the map file descriptor
mapInfo, err := loader.GetMap(ctx, "program_name", "unhd_hmac_keys")
if err != nil { /* handle error */ }

// Create MapLoader for population
ml := maploader.NewMapLoader(mapInfo.FD, "unhd_hmac_keys")

// Prepare data
hmacKeys := [][2]interface{}{
	{
		bpfschema.HMACKeyKey{FlowID: 1, Namespace: 1},
		bpfschema.HMACKeyValue{
			Key:       [32]byte{...},
			CreatedAt: 100,
			ExpiresAt: 0,
		},
	},
	{
		bpfschema.HMACKeyKey{FlowID: 2, Namespace: 1},
		bpfschema.HMACKeyValue{
			Key:       [32]byte{...},
			CreatedAt: 200,
			ExpiresAt: 0,
		},
	},
}

// Populate map
if err := ml.LoadMapBatch(hmacKeys); err != nil {
	// handle error
}
```

### Pattern 2: Loading ARRAY Maps

For maps indexed by position:

```go
// Get the map
mapInfo, err := loader.GetMap(ctx, "program_name", "unhd_flow_types")
if err != nil { /* handle error */ }

ml := maploader.NewMapLoader(mapInfo.FD, "unhd_flow_types")

// Prepare array entries (index = position in slice)
entries := []interface{}{
	bpfschema.FlowTypeEntry{Type: 0x00, Flags: 0x00}, // Index 0
	bpfschema.FlowTypeEntry{Type: 0x01, Flags: 0x01}, // Index 1
	bpfschema.FlowTypeEntry{Type: 0x02, Flags: 0x00}, // Index 2
}

// Populate array map
if err := ml.LoadMapArray(entries); err != nil {
	// handle error
}
```

### Pattern 3: Loading Error Counters

For initialization of per-CPU arrays:

```go
mapInfo, err := loader.GetMap(ctx, "program_name", "unhd_error_counters")
if err != nil { /* handle error */ }

ml := maploader.NewMapLoader(mapInfo.FD, "unhd_error_counters")

// Initialize all error codes (0-255) with zero counters
if err := ml.LoadErrorCounters(); err != nil {
	// handle error
}
```

### Pattern 4: Individual Entries

For single key-value pairs:

```go
key := bpfschema.SettingsKey{ConnID: 1, SettingID: 42}
value := bpfschema.SettingsValue{
	Value:        1000,
	NegotiatedAt: 1234567890,
	Flags:        0x01,
}

if err := ml.LoadMap(key, value); err != nil {
	// handle error
}
```

## Map Reference

### Q1: HMAC Keys (unhd_hmac_keys)

**Type**: HASH
**Key**: HMACKeyKey (8 bytes)
**Value**: HMACKeyValue (40 bytes)
**Purpose**: Per-flow replay protection (RFC 9000 §8.1)

```go
ml.LoadMap(
	bpfschema.HMACKeyKey{FlowID: flowID, Namespace: ns},
	bpfschema.HMACKeyValue{Key: keyBytes, CreatedAt: ts, ExpiresAt: 0},
)
```

### Q2: Sequence Counters (unhd_seq_counters)

**Type**: PERCPU_HASH
**Key**: SeqCounterKey (8 bytes)
**Value**: SeqCounterValue (16 bytes)
**Purpose**: Per-namespace sequence tracking (RFC 9000 §5.1.1)

```go
ml.LoadMap(
	bpfschema.SeqCounterKey{Namespace: ns},
	bpfschema.SeqCounterValue{Current: 1, Highest: 1, Gaps: 0, Reordered: 0},
)
```

### H4/H12: Settings (unhd_settings)

**Type**: HASH
**Key**: SettingsKey (8 bytes)
**Value**: SettingsValue (16 bytes)
**Purpose**: Per-connection capability settings (RFC 9114 §7.2.4)

```go
ml.LoadMap(
	bpfschema.SettingsKey{ConnID: connID, SettingID: settingID},
	bpfschema.SettingsValue{Value: val, NegotiatedAt: ts, Flags: 0x01},
)
```

### H6: Flow Types (unhd_flow_types)

**Type**: ARRAY (indexed by flow_id)
**Entry**: FlowTypeEntry (4 bytes)
**Purpose**: Flow type classification (RFC 9114 §6)

```go
entries := []interface{}{
	bpfschema.FlowTypeEntry{Type: 0x00, Flags: 0x00}, // Index 0
	bpfschema.FlowTypeEntry{Type: 0x01, Flags: 0x01}, // Index 1
}
ml.LoadMapArray(entries)
```

### H2: Error Counters (unhd_error_counters)

**Type**: PERCPU_ARRAY
**Key**: ErrorCounterKey (4 bytes, code: uint16)
**Value**: ErrorCounterValue (8 bytes)
**Purpose**: Error code counters (RFC 9114 §8)

```go
ml.LoadErrorCounters() // Auto-initializes codes 0-255
```

### H10: Hop Validators (unhd_hop_validators)

**Type**: HASH
**Key**: HopValidatorKey (8 bytes)
**Value**: HopValidatorValue (32 bytes)
**Purpose**: Per-hop packet validators (RFC 8200 §4)

```go
ml.LoadMap(
	bpfschema.HopValidatorKey{HopID: hopID, Namespace: ns},
	bpfschema.HopValidatorValue{...},
)
```

### H7/Q8: GOAWAY State (unhd_goaway_state)

**Type**: HASH
**Key**: GoawayStateKey (8 bytes)
**Value**: GoawayStateValue (16 bytes)
**Purpose**: GOAWAY state tracking (RFC 9114 §7.2.6)

```go
ml.LoadMap(
	bpfschema.GoawayStateKey{ConnID: connID},
	bpfschema.GoawayStateValue{LastFlowID: 0, SentAt: ts, Reason: 0, Count: 0, Flags: 0},
)
```

## Error Handling

All errors are wrapped with context:

```
maploader: <map_name>: <operation>: <error>
```

Example errors:
- `maploader: unhd_hmac_keys: map update: bad file descriptor`
- `maploader: unhd_settings: serialize value: unsupported type`
- `maploader: unhd_flow_types: serialize index 5: buffer too small`

## Performance Considerations

### Batch Processing

MapLoader uses batching for efficiency:

```go
// Default batch size (tunable)
const BatchSize = 256

// Loading 10,000 entries processes in 40 batches
// Each batch uses individual syscalls (batch syscall support pending)
entries := make([][2]interface{}, 10000)
// ... populate entries ...
err := ml.LoadMapBatch(entries)
```

Future optimization: Use `BPF_MAP_UPDATE_BATCH` syscall (kernel 5.6+) for bulk updates.

### Memory Usage

For large maps:
- HASH maps: O(n) memory for entire dataset
- ARRAY maps: O(n) memory for entire dataset
- Per-CPU maps: O(n × num_cpus) memory

For better memory efficiency, populate maps incrementally:

```go
for i := 0; i < totalEntries; i += batchSize {
	batch := entries[i:min(i+batchSize, totalEntries)]
	if err := ml.LoadMapBatch(batch); err != nil {
		// handle error
	}
}
```

## Testing

MapLoader includes comprehensive tests:

- **Serialization roundtrip tests**: Verify Go struct → binary → Go struct
- **Struct size tests**: Verify all sizes match BPF schema
- **Big-endian encoding tests**: Verify network byte order
- **Concurrent access tests**: Verify thread safety of serialization
- **Error case tests**: Verify error handling

Run tests:

```bash
go test ./pkg/ebpf/maploader -v
```

## Thread Safety

MapLoader operations are **not** thread-safe with respect to a single map. If multiple goroutines populate the same map simultaneously, results are undefined.

However:
- **Serialization** (encoding/binary) is thread-safe
- **Different maps** can be populated concurrently (different MapLoader instances)
- **Read-only** operations on populated maps are safe from multiple goroutines

For concurrent population of a single map, use external synchronization:

```go
var mu sync.Mutex

mu.Lock()
err := ml.LoadMap(key, value)
mu.Unlock()
```

## Batch Update Syscall

Future versions will use `BPF_MAP_UPDATE_BATCH` (kernel 5.6+) for better performance:

```go
// This will be optimized in a future commit
err := ml.LoadMapBatchWithBatching(entries)
```

The batch syscall reduces:
- Number of syscall invocations (O(n) → O(n/batch_size))
- Context switches
- Kernel mode entry/exit overhead

Expected improvement: 3-5× faster on large maps (10k+ entries).

## Related Documentation

- **Protocol**: `draft-bellis-unheaded-protocol-foundation-03`
- **eBPF Loader**: `pkg/ebpf/loader.go`
- **BPF Schema**: `pkg/protocol/bpfschema/bpfschema.go`
- **Map Matrix**: `PATTERN_MATRIX.md` (in eBPF program docs)
- **Concurrency**: Black Mage Concurrency Audit Checklist
