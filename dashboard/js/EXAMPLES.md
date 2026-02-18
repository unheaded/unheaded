# JavaScript Modules - Example Usage

## Metrics Module Examples

### Initializing Metrics
```javascript
// Auto-initializes on DOM ready, or manually:
window.UnheadedMetrics.init();
```

### Processing PacketFlow Data
```javascript
const flow = {
  timestamp_ns: 1676000000000000000,
  protocol: 'HTTP/3',
  source_ip: '192.168.1.100',
  dest_ip: '10.0.0.50',
  status_code: 200,
  total_time: 50000000,  // 50ms in nanoseconds
  hops: [
    { service: 'ingress', duration_us: 10 },
    { service: 'svc-a', duration_us: 20 },
    { service: 'svc-b', duration_us: 20 },
  ],
  flow_label: 0x1234,
};

window.UnheadedMetrics.onPacketFlow(flow);
```

### Sparkline Direct Usage
```javascript
// Create custom sparkline
const canvas = document.getElementById('custom-sparkline');
const sparkline = new Sparkline(canvas, {
  color: '#00ff41',
  maxPoints: 120,
  fillOpacity: 0.1,
});

// Push data periodically
setInterval(() => {
  const randomValue = Math.random() * 100;
  sparkline.push(randomValue);
  sparkline.draw();
}, 1000);
```

### Accessing Metric Data
```javascript
// Get raw latency buffer
const latencies = window.UnheadedMetrics.latencyBuffer;

// Get protocol distribution
const protocolCounts = window.UnheadedMetrics.protocolCounts;
for (const [protocol, count] of protocolCounts) {
  console.log(`${protocol}: ${count}`);
}

// Get status code distribution
const statusCounts = window.UnheadedMetrics.statusCounts;
for (const [status, count] of statusCounts) {
  console.log(`${status}: ${count}`);
}
```

---

## Monad Decode Module Examples

### Processing PacketFlow (Auto-Synthesizes Monad)
```javascript
const flow = {
  timestamp_ns: 1676000000000000000,
  protocol: 'HTTP/3',
  source_ip: '192.168.1.100',
  dest_ip: '10.0.0.50',
  status_code: 200,
  total_time: 50000000,
  hops: [
    { service: 'ingress', duration_us: 10 },
    { service: 'svc-a', duration_us: 20 },
  ],
  flow_label: 0x1234,
};

// Module automatically synthesizes monad and updates panel
window.UnheadedMonad.onPacketFlow(flow);
```

### Processing Real Monad Event
```javascript
const anamnesisEvent = {
  timestamp_ns: 1676000000000000000,
  event_type: 1,
  hop_id: 2,
  flow_label_lo: 0x1234,
  monad: {
    flow_action: 1,      // TRACE
    hop_count: 2,
    flags: 0x03,         // TRACED | SAMPLED
    circuit_state: 0,    // CLOSED
    src_service_id: 5,
    dst_service_id: 8,
    regs: [100, 200, 0, 0],
    scratch_r0: 42,
    scratch_r1: 84,
    crc: 0xABCD,
  },
  checksum_valid: true,
  is_chaos: false,
  is_traced: true,
  is_sampled: true,
};

window.UnheadedMonad.onAnamnesisEvent(anamnesisEvent);
```

### Accessing Current Monad State
```javascript
if (window.UnheadedMonad.currentMonad) {
  const monad = window.UnheadedMonad.currentMonad.monad;
  console.log(`Flow action: ${FLOW_ACTIONS[monad.flow_action]}`);
  console.log(`Hops: ${monad.hop_count}`);
  console.log(`Circuit: ${CIRCUIT_STATES[monad.circuit_state]}`);
}
```

### Accessing Circuit Breaker State
```javascript
// Get all tracked circuits
for (const [key, data] of window.UnheadedMonad.cbState) {
  console.log(`${key}: ${CIRCUIT_STATES[data.state]}`);
}

// Find OPEN circuits
const openCircuits = Array.from(window.UnheadedMonad.cbState.entries())
  .filter(([key, data]) => data.state === 1)
  .map(([key]) => key);

console.log(`Open circuits: ${openCircuits.join(', ')}`);
```

### Triggering Chaos Injection Programmatically
```javascript
// Inject bit_flip on flow label 0x1234
const chaosPayload = {
  flow_label: 0x1234,
  mode: 'bit_flip',
  param: 0,
};

fetch('/api/v1/chaos', {
  method: 'POST',
  headers: { 'Content-Type': 'application/json' },
  body: JSON.stringify(chaosPayload),
})
  .then(res => res.json())
  .then(data => console.log('Chaos injected:', data))
  .catch(err => console.error('Chaos failed:', err));
```

### Injecting Delay Chaos
```javascript
// Inject 5ms delay on flow
const delayPayload = {
  flow_label: 0x5678,
  mode: 'delay',
  param: 5000,  // 5000 microseconds = 5ms
};

fetch('/api/v1/chaos', {
  method: 'POST',
  headers: { 'Content-Type': 'application/json' },
  body: JSON.stringify(delayPayload),
})
  .then(res => res.json())
  .catch(err => console.error('Error:', err));
```

### Stopping Chaos Injection
```javascript
fetch('/api/v1/chaos/stop', {
  method: 'DELETE',
  headers: { 'Content-Type': 'application/json' },
})
  .then(res => res.json())
  .then(data => console.log('Chaos stopped'))
  .catch(err => console.error('Error:', err));
```

### Reading Chaos Log
```javascript
const logEntries = document.querySelectorAll('.chaos-log-entry');
logEntries.forEach(entry => {
  console.log(entry.textContent);
});

// Example output:
// [14:23:45] INJECT DELAY fl=0x5678
// [14:23:50] STOPPED
```

### Monitoring Chaos Status
```javascript
// Check if chaos is active
if (window.UnheadedMonad.chaosActive) {
  console.log(`Chaos mode: ${window.UnheadedMonad.chaosMode}`);
  console.log(`Target flow: 0x${window.UnheadedMonad.chaosFlowLabel.toString(16)}`);
}
```

---

## Integration Example

### Dashboard Entry Point (packet-flow.js)
```javascript
// After UnheadedPacketFlow receives a message:
const flow = parsePacketFlow(message);

// Update metrics
window.UnheadedMetrics.onPacketFlow(flow);

// Update monad panel
window.UnheadedMonad.onPacketFlow(flow);

// Update other panels...
```

### Real Monad Event Handler (when Anamnesys sends data)
```javascript
// Received real monad packet
const anamnesisEvent = parseAnamnesisEvent(data);

// Update monad panel with real data
window.UnheadedMonad.onAnamnesisEvent(anamnesisEvent);

// Could also update metrics if needed
window.UnheadedMetrics.onAnamnesisEvent(anamnesisEvent);
```

---

## Performance Notes

- Sparklines efficiently maintain ring buffers (default 60 points, configurable)
- Canvas rendering is optimized for 1000ms update intervals
- Latency buffer keeps last 100 samples (configurable)
- Circuit breaker map capped at 20 entries
- Chaos log limited to 10 entries
- No memory leaks from event listeners (proper cleanup on reset)
- CPU usage minimal on modern browsers with hardware acceleration

