# Doom Bridge Architecture

**Last updated:** 2026-03-30
**Implementation:** `crates/doom-runner/src/bridge.rs` (Rust/Axum/Aya)
**Port:** 16666 (http://0.0.0.0:16666)
**Protocol:** HTTP + WebSocket

## Overview

The bridge is integrated into doom-runner -- no separate binary. It reads the
Doom framebuffer and palette directly from BPF maps via the Aya Ebpf object
and streams frames to a browser over WebSocket. Keyboard input flows in reverse.

**Replaces:** The old Go `cmd/doom-bridge/` service that used pinned maps on port
6660. That approach was fragile (pin management, stale maps) and is no longer used.

## Data Flow

```
RAM_MAP (BPF Array<u32>, 16M entries)
    |
    +-- frame_poller (60 Hz tokio timer)
    |   reads PALETTE_ADDR (0x60000, 192 words = 768 bytes)
    |   reads SCREEN_BASE  (0x70000, 16000 words = 64000 bytes)
    |   broadcasts 64,768-byte binary frame
    |
    +-- kbd_writer (8-slot circular queue)
        writes to KBD_MAP (BPF Array<u32>, 8 entries)
        encodes: (scancode << 1) | pressed
    |
WebSocket server (Axum, port 16666)
    |
    +-- GET /   -> HTML viewer (inline <canvas> + JS)
    +-- WS /ws  -> binary frames (64,768 bytes each)
                   client sends 3-byte keyboard events
    |
Firefox canvas (320x200, bilinear CSS upscale to 960x600)
```

## Frame Format

**Server -> Client (binary WebSocket frame):**
```
Bytes 0-767:      Dynamic PLAYPAL palette (256 x RGB)
Bytes 768-64767:  Screen pixels (320x200 x palette index)
Total:            64,768 bytes per frame
```

The JS client decodes each pixel: `palette[pixel * 3 + channel]` -> RGBA canvas.

**Fallback:** If frame is exactly 64,000 bytes (legacy, no palette prefix), the
JS client uses a hardcoded PLAYPAL array baked into the HTML.

**Client -> Server (binary WebSocket frame):**
```
Bytes 0-1:  JS keyCode (uint16, little-endian)
Byte 2:     pressed (1 = down, 0 = up)
Total:      3 bytes per event
```

## Keyboard Pipeline

```
1. Browser keydown fires
2. JS: if (e.repeat) return;          // suppress auto-repeat
3. JS: sendKey(e.keyCode, true)       // 3-byte binary message
4. Axum WS handler: parse [u16 LE][u8]
5. mpsc channel -> kbd_writer task
6. kbd_writer: encode val = (scancode << 1) | pressed
7. Circular scan of KBD_MAP[0..7]:
   - Find first empty slot (value == 0)
   - Write val there, advance write_head
   - If all 8 full, overwrite write_head (drop oldest)
8. MBC executor: SYS_GET_KEY syscall
   - Scan all 8 KBD_MAP slots
   - Return first non-zero, clear slot
   - Return 0 if empty
9. i_video_mbc.c I_StartTic:
   - Poll SYS_GET_KEY up to 8 times
   - Map JS keyCode -> Doom KEY_* via switch
   - D_PostEvent(ev_keydown or ev_keyup)
```

## Frame Poller

- 60 Hz tokio interval timer
- Skips reads when no WebSocket clients connected (saves BPF map syscalls)
- Holds ebpf mutex only during RAM_MAP reads (~16K lookups)
- MissedTickBehavior::Skip prevents backlog buildup
- Broadcasts via tokio broadcast channel (capacity 2, drops old frames)

## Screen Reading Strategy

The bridge reads from **RAM_MAP** (not SCREEN_MAP) because:
- Doom's HUD (status bar) uses `memset`/`memcpy` which compile to word stores (SW)
- Word stores go to RAM_MAP only (Bug 24 fix blocks SCREEN_MAP word stores)
- Reading RAM_MAP sees ALL writes (byte stores AND word stores)
- 16,000 u32 reads is 4x fewer BPF syscalls than 64,000 u8 reads from SCREEN_MAP

## CSS Upscale

The canvas is 320x200 native, CSS-scaled to 960x600 (3x).

**Current:** Bilinear interpolation (browser default). The `image-rendering: pixelated`
rule was intentionally removed. Bilinear smoothing reduces the perception of
nearest-neighbor texture banding that is inherent to Doom's 320x200 resolution.

**To restore crisp pixels:** Add to the canvas CSS:
```css
image-rendering: pixelated;
image-rendering: crisp-edges;
```

## HTTP Endpoints

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/` | GET | HTML viewer page (inline canvas, JS, palette, controls) |
| `/ws` | GET | WebSocket upgrade (binary frames + keyboard input) |

## Concurrency Model

```
main thread
  |
  +-- tokio runtime
       |
       +-- axum HTTP server (port 16666)
       |     +-- index_handler: serves VIEWER_HTML
       |     +-- ws_handler: upgrades to WebSocket
       |           +-- send_task: forward frame broadcasts to client
       |           +-- recv_task: receive keyboard events from client
       |
       +-- frame_poller task
       |     60 Hz timer -> read RAM_MAP -> broadcast::Sender
       |
       +-- kbd_writer task
             mpsc::Receiver -> circular write to KBD_MAP

Shared state: Arc<Mutex<Ebpf>>
  - frame_poller holds lock briefly for RAM_MAP reads
  - kbd_writer holds lock briefly for KBD_MAP writes
  - Never held across await points
```

## Known Limitations

1. **Frame rate:** Browser sees ~2-3 fps despite 60 Hz poll rate
   - Bottleneck: 16,000 individual BPF map lookups per frame
   - Future: batch reads, shared memory, or frame-diff

2. **No authentication:** Anyone on the network can connect and send input
   - Acceptable for development; needs auth for production

3. **No stats endpoint:** Old Go bridge had /metrics (Prometheus). Not yet ported.

4. **No audio:** Silent. Would need separate WebSocket channel.
