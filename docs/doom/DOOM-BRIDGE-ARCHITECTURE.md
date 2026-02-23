# Doom Bridge Architecture -- Fenrir's Eye Service

## Overview

**doom-bridge** reads the Doom framebuffer and CPU state from pinned eBPF BPF maps, converts palette indices to RGB, and streams frames via WebSocket to connected browser clients.

**Service Name:** Fenrir's Eye (Norse mythology: the wolf that sees all)
**Port:** 6660 (default)
**Protocol:** HTTP/WebSocket
**Binary:** `cmd/doom-bridge/`

## Data Flow

```
BPF Maps (SCREEN_MAP, CPU_MAP, STATS, KBD_MAP)
    |
doom-bridge service (Go)
    |-- screenLoop: reads SCREEN_MAP @ 30fps -> palette -> RGBA
    |-- statsLoop: reads CPU_MAP + STATS @ 2fps -> JSON stats
    |
WebSocket server (port 6660)
    |
Browser clients (dashboard/doom.html)
    |-- Canvas rendering (320x200 -> scaled)
    |-- Keyboard input (KBD_MAP writes, reverse path)
```

## Component Details

### screenLoop (Go routine)
- Polls SCREEN_MAP at ~33ms intervals (30fps target)
- Supports two read modes:
  1. Batch read: BPF_MAP_LOOKUP_BATCH (fast, sub-1ms per frame)
  2. Individual read: BPF_MAP_LOOKUP_ELEM x 64000 (fallback, ~5-10ms per frame)
- Converts 320x200 palette indices to RGBA (256000 bytes)
- Broadcasts binary frames to all connected WebSocket clients
- Frame format: `[0x01 tag] + [256000 bytes RGBA]`

### statsLoop (Go routine)
- Polls CPU_MAP and STATS at ~2fps (500ms intervals)
- Reads:
  - CPU state: PC, flags, registers, cache hit/miss counts
  - STATS: packet count, ticks, instructions, halted flag
- Marshals to JSON and broadcasts as text WebSocket messages
- Clients use this for overlay metrics (FPS, IPS, cache hit rate)

### Client Handling
- New WebSocket connection -> add to `clients` map
- Broadcast loops iterate over all clients, write frames
- Client disconnect -> remove from map
- No individual message buffering (real-time streaming)

## Frame Format (WS1 MVP)

**Binary Frame (Screen Data):**
```
Byte 0:          0x01 (tagScreen)
Bytes 1-256000:  RGBA pixel data (320 x 200 x 4 bytes)
Total:           256001 bytes per frame
```

**Text Frame (Stats):**
```json
{
  "type": "stats",
  "packets": 1200,
  "ticks": 60,
  "insns": 85000,
  "halted": 0,
  "pc": 4660,
  "flags": 66,
  "regs": [0, 0, 0, ..., 4294901760]
}
```

**Binary Frame (Keyboard Input, client -> server):**
```
Byte 0:   0x02 (tagKbd)
Byte 1-2: scancode (uint16, little-endian)
Byte 3:   pressed (1 = down, 0 = up)
```

## BPF Map Access

Uses raw `golang.org/x/sys/unix` BPF syscalls. No cilium/ebpf dependency (minimal for MVP).

**Syscalls:**
- `BPF_OBJ_GET`: Open pinned map by filesystem path
- `BPF_MAP_LOOKUP_ELEM`: Read single element
- `BPF_MAP_LOOKUP_BATCH`: Read multiple elements at once
- `BPF_MAP_UPDATE_ELEM`: Write keyboard events

## HTTP Endpoints

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/ws` | GET | WebSocket upgrade (binary frames + JSON stats) |
| `/health` | GET | Health check (JSON: status, client count, dry_run) |
| `/ready` | GET | Readiness probe (200 if maps open or dry-run; 503 otherwise) |
| `/metrics` | GET | Prometheus metrics (clients, frames, bytes, errors) |
| `/` | GET | Static file server (doom.html viewer) |

## Prometheus Metrics

| Metric | Type | Description |
|--------|------|-------------|
| `unheaded_doom_bridge_clients` | gauge | Connected WebSocket clients |
| `unheaded_doom_bridge_frames_total` | counter | Total frames sent |
| `unheaded_doom_bridge_bytes_sent_total` | counter | Total bytes sent over WebSocket |
| `unheaded_doom_bridge_errors_total` | counter | Total errors encountered |

## Scaling Limitations (WS1)

- **Frame rate:** 30fps fixed (configurable, but limited by network)
- **Client connections:** Tested to 5+ concurrent clients
- **Data rate:** ~7.7 MB/s per 30fps client (raw RGBA, no compression)
- **Latency:** Single-frame latency ~33ms (one frame interval)

**Optimization opportunities (WS3/later):**
- PNG or JPEG compression (reduce to ~20-50KB per frame)
- Incremental frame diffs
- Separate control and data channels
- Palette-indexed mode (64KB instead of 256KB per frame)

## Known Limitations

1. **Palette:** Uses synthetic VGA gradient for colors 80-255 (not true Doom palette)
   - Fix: Load actual PLAYPAL from doom.wad

2. **Keyboard input:** Basic scancode encoding, no key repeat handling
   - Fix: Implement key repeat detection on browser side

3. **No audio:** Silent video-only
   - Fix: WS2+ can add audio stream

4. **Dry-run mode only for MVP testing**
   - Real frames require active Doom ring (WS3 prerequisite)

## Security Considerations

- No authentication on WebSocket (WS5 task)
- BPF maps readable by doom-bridge process only (eBPF hardening)
- Keyboard input validated but not authenticated
- Frame data is observability-only (no secrets)
- CORS: All origins allowed in development (tighten for production)

## Testing

See `tests/ws1-integration.sh` for automated integration test suite.

**Manual test:**
```bash
go build -o doom-bridge ./cmd/doom-bridge
./doom-bridge --port 6660 --dry-run --static ./dashboard
# Open http://localhost:6660/ in browser
```

**Unit tests:**
```bash
go test -v ./cmd/doom-bridge/...
```

## Future Work (WS3+)

- Real Doom palette (load from doom.wad PLAYPAL lump)
- Frame compression (PNG/JPEG)
- Audio streaming (separate channel)
- Performance profiling (sub-10ms frame latency target)
- Wotan integration (metrics publishing)
- Authentication (WS5)
