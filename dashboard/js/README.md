# Unheaded eBPF Dashboard - JavaScript Modules

Two production-quality vanilla JavaScript modules for the dashboard, with no external dependencies.

## File 1: metrics.js (489 lines)

Manages metric panels and real-time visualization with sparkline charts.

### Key Components

**Sparkline Class**
- Lightweight canvas-based line chart renderer
- Ring buffer maintains max 60 points by default
- Auto-scaling based on min/max values
- Configurable colors and fill opacity
- Efficient draw() method for real-time updates

**Metrics Tracked**
- `eps`: Events per second (from packet-flow.js stats)
- `avgHops`: Rolling average hop count from PacketFlow
- `latencyBuffer`: Last 100 latency samples (nanoseconds → microseconds)
- `protocolCounts`: Map of HTTP/3, gRPC, HTTP/2, WebSocket
- `statusCounts`: Map of 2xx, 3xx, 4xx, 5xx status buckets

**Update Cycle** (every 1000ms)
- Pulls stats from `window.UnheadedDashboard.getStats()`
- Updates all sparkline canvases with new data
- Calculates p50/p99 latency percentiles from sorted samples
- Redraws protocol and status distribution charts

**DOM Elements Updated**
- `#m-eps`: Events per second (formatted with comma separators)
- `#m-avghops`: Average hop count (1 decimal place)
- `#m-p50`, `#m-p99`: Latency percentiles (formatted: μs/ms/s)
- `#metrics-lag`: Status indicator (live/stale)
- `#eps-counter`: EPS in topbar
- `#proto-chart`: Horizontal stacked bar chart (HTTP/3, gRPC, HTTP/2, WebSocket)
- `#status-chart`: Horizontal stacked bar chart (2xx, 3xx, 4xx, 5xx)
- All sparkline canvases with animated line charts

**Latency Formatting**
- < 1000μs: "Xμs" format
- < 1000ms: "Xms" format
- >= 1000ms: "Xs" format
- Input: nanoseconds, conversion: ns / 1000 = μs

**Chart Colors**
- Protocol: HTTP/3=#00ff41, gRPC=#c084fc, HTTP/2=#60a5fa, WebSocket=#ffaa00
- Status: 2xx=#00ff41, 3xx=#00d4ff, 4xx=#ffaa00, 5xx=#ff4400
- Sparklines: eps=#00ff41, hops=#00d4ff, p50=#c084fc, p99=#ffaa00

**Public API**
```javascript
window.UnheadedMetrics.onPacketFlow(flow)    // Process PacketFlow message
window.UnheadedMetrics.onAnamnesisEvent(evt) // Process synthetic monad event
window.UnheadedMetrics.init()                 // Initialize all panels
window.UnheadedMetrics.reset()                // Clear all buffers
```

---

## File 2: monad-decode.js (560 lines)

Manages monad packet inspection, circuit breaker matrix, and chaos injection control.

### Key Components

**Monad Constants**
```javascript
FLOW_ACTIONS = { 0: 'FORWARD', 1: 'TRACE', 2: 'SAMPLE', 3: 'MIRROR', 4: 'DROP' }
FLAGS = { TRACED: 0x01, SAMPLED: 0x02, MIRROR: 0x04, CHAOS: 0x08, CUSTOM: 0x10 }
CIRCUIT_STATES = { 0: 'CLOSED', 1: 'OPEN', 2: 'HALF' }
```

**Monad Decode Panel**
Displays detailed monad packet information with proper color coding:
- `#mf-fa`: Flow action (FORWARD/TRACE/SAMPLE/MIRROR/DROP) with colored indicator
- `#mf-hc`: Hop count (accent color, larger if > 5)
- `#mf-flags`: Decoded flag names (TRACED|SAMPLED|MIRROR|CHAOS|CUSTOM)
- `#mf-circuit`: Circuit state (CLOSED=green, OPEN=red, HALF=amber)
- `#mf-crc`: Checksum (0x#### ✓/✗) with validity color
- `#mf-src`, `#mf-dst`: Service IDs (derived from IP addresses)
- `#mf-regs`: Register values [r0,r1,r2,r3]
- `#mf-scratch`: Scratch registers (r0=X r1=Y)
- `#monad-flow-label`: Flow label in hex format (fl=0x####)
- `#monad-hex`: 20-byte hex dump representation

**Monad Synthesis from PacketFlow**
- Flow action: 1 (TRACE) if protocol=HTTP/3, else 0 (FORWARD)
- Hop count: extracted from hops.length
- Flags: TRACED if status=200, CHAOS if status=500
- Circuit state: CLOSED=0, OPEN=1 (503), HALF=2 (429)
- Service IDs: derived from IP last octets (% 16)
- CRC: randomly generated 16-bit value for demo

**Circuit Breaker Matrix**
Tracks service-to-service connection states:
- Max 20 entries maintained in Map
- Format: "svcA→svcB" → { state, since, statusCode }
- Updated based on HTTP status codes:
  - 503: OPEN state
  - 429: HALF state
  - 200: CLOSED state
- DOM element `#circuit-matrix`: Renders list of all tracked pairs
- Each row: label | state | age (seconds)
- `#cb-open-count`: Count of OPEN circuits with danger/dim coloring

**Chaos Injection Control**
Interactive panel for injecting failure modes:
- `#chaos-flow-label`: Text input for hex flow label (4 hex digits)
- `#chaos-mode`: Select dropdown (bit_flip/delay/duplicate/truncate/marker)
- `#chaos-delay-us`: Number input (shown only when mode=delay)
- `#btn-inject-chaos`: POST to /api/v1/chaos
- `#btn-stop-chaos`: DELETE to /api/v1/chaos/stop
- `#chaos-log`: Log panel (max 10 entries, auto-scrolls)
- `#chip-chaos`: Status indicator (active when chaos running)
- `#chaos-status`: Text display of current chaos mode

**Chaos API**
POST /api/v1/chaos:
```json
{
  "flow_label": 4660,
  "mode": "delay",
  "param": 1000
}
```

DELETE /api/v1/chaos/stop: (no body)

Log entries timestamped: `[HH:MM:SS] MESSAGE`

**Error Handling**
- Validates hex flow label input (regex: /^[0-9a-fA-F]+$/)
- Fetch errors logged to chaos-log instead of throwing
- Network timeouts handled gracefully
- HTTP errors reported with status codes

**Public API**
```javascript
window.UnheadedMonad.onPacketFlow(flow)      // Process PacketFlow, synthesize monad
window.UnheadedMonad.onAnamnesisEvent(evt)   // Process real monad event
window.UnheadedMonad.init()                   // Setup event handlers
window.UnheadedMonad.reset()                  // Clear chaos state
```

---

## Integration Points

Both modules are designed to work with:
- `window.UnheadedDashboard`: Main dashboard instance (provides getStats())
- `window.UnheadedPacketFlow`: Packet flow processor (calls onPacketFlow on both modules)
- DOM elements as documented above
- Fetch API for chaos injection control

## Production Features

- Pure vanilla JavaScript, zero dependencies
- Graceful null checks on all DOM operations
- Robust error handling for network requests
- Efficient ring buffers for metric data
- Canvas-based rendering for performance
- Proper HTML escaping in DOM updates
- Auto-initialization on DOM ready
- No global namespace pollution beyond window.UnheadedMetrics/Monad
