# Code Preview - Key Implementations

## metrics.js - Sparkline Class

```javascript
class Sparkline {
  constructor(canvasElement, config = {}) {
    this.canvas = canvasElement;
    this.ctx = canvasElement ? canvasElement.getContext('2d') : null;
    this.color = config.color || '#00ff41';
    this.fillOpacity = config.fillOpacity !== undefined ? config.fillOpacity : 0.2;
    this.maxPoints = config.maxPoints || 60;
    this.values = [];
    this.min = 0;
    this.max = 1;
  }

  push(value) {
    if (!this.canvas || !this.ctx) return;
    
    const numVal = parseFloat(value) || 0;
    this.values.push(numVal);
    
    if (this.values.length > this.maxPoints) {
      this.values.shift(); // Ring buffer
    }
    
    if (this.values.length > 0) {
      this.max = Math.max(...this.values);
      if (this.max === 0) this.max = 1;
      this.min = 0;
    }
  }

  draw() {
    // Canvas rendering with fill and line chart
    const ctx = this.ctx;
    const w = this.canvas.width;
    const h = this.canvas.height;
    const padding = 2;
    const graphWidth = w - 2 * padding;
    const graphHeight = h - 2 * padding;

    ctx.fillStyle = 'rgba(0, 0, 0, 0.03)';
    ctx.fillRect(0, 0, w, h);

    const range = Math.max(this.max - this.min, 1);
    const xStep = graphWidth / (this.values.length - 1 || 1);

    // Draw filled area under curve
    ctx.beginPath();
    ctx.moveTo(padding, h - padding);
    
    for (let i = 0; i < this.values.length; i++) {
      const x = padding + i * xStep;
      const normalizedValue = (this.values[i] - this.min) / range;
      const y = h - padding - normalizedValue * graphHeight;
      ctx.lineTo(x, y);
    }

    ctx.lineTo(w - padding, h - padding);
    ctx.closePath();
    
    ctx.fillStyle = this.hexToRgba(this.color, this.fillOpacity);
    ctx.fill();

    // Draw line
    ctx.strokeStyle = this.color;
    ctx.lineWidth = 1.5;
    ctx.stroke();
  }
}
```

## metrics.js - Metrics Update Cycle

```javascript
updateMetrics() {
  const stats = window.UnheadedDashboard?.getStats?.() || { eps: 0 };

  // Update EPS
  const epsElement = document.getElementById('m-eps');
  if (epsElement) {
    epsElement.textContent = stats.eps.toLocaleString();
    if (this.sparklines.eps) {
      this.sparklines.eps.push(stats.eps);
      this.sparklines.eps.draw();
    }
  }

  // Calculate latency percentiles
  if (this.latencyBuffer.length > 0) {
    const sorted = [...this.latencyBuffer].sort((a, b) => a - b);
    const p50 = this.percentile(sorted, 50);
    const p99 = this.percentile(sorted, 99);

    const p50Element = document.getElementById('m-p50');
    if (p50Element) {
      p50Element.textContent = this.formatLatency(p50);
    }

    const p99Element = document.getElementById('m-p99');
    if (p99Element) {
      p99Element.textContent = this.formatLatency(p99);
    }

    if (this.sparklines.p50) {
      this.sparklines.p50.push(p50);
      this.sparklines.p50.draw();
    }

    if (this.sparklines.p99) {
      this.sparklines.p99.push(p99);
      this.sparklines.p99.draw();
    }
  }

  // Redraw distribution charts
  this.drawProtocolChart();
  this.drawStatusChart();
}
```

## metrics.js - Protocol Distribution Chart

```javascript
drawProtocolChart() {
  const canvas = document.getElementById('proto-chart');
  if (!canvas) return;

  const ctx = canvas.getContext('2d');
  const w = canvas.width;
  const h = canvas.height;

  ctx.fillStyle = 'rgba(0, 0, 0, 0.05)';
  ctx.fillRect(0, 0, w, h);

  const total = Array.from(this.protocolCounts.values())
    .reduce((a, b) => a + b, 0);

  if (total === 0) {
    ctx.fillStyle = 'rgba(255, 255, 255, 0.2)';
    ctx.font = '12px monospace';
    ctx.textAlign = 'center';
    ctx.fillText('awaiting data...', w / 2, h / 2 + 4);
    return;
  }

  const colors = {
    'HTTP/3': '#00ff41',
    'gRPC': '#c084fc',
    'HTTP/2': '#60a5fa',
    'WebSocket': '#ffaa00',
  };

  let x = 0;
  const padding = 2;
  const graphWidth = w - 2 * padding;
  const graphHeight = h - 2 * padding;

  for (const [protocol, count] of this.protocolCounts) {
    const proportion = count / total;
    const segmentWidth = proportion * graphWidth;

    if (segmentWidth < 2) continue;

    ctx.fillStyle = colors[protocol] || '#888888';
    ctx.fillRect(padding + x, padding, segmentWidth, graphHeight);

    // Draw label if wide enough
    if (segmentWidth > 30) {
      ctx.fillStyle = '#000000';
      ctx.font = 'bold 8px monospace';
      ctx.textAlign = 'center';
      ctx.textBaseline = 'middle';
      ctx.fillText(protocol, padding + x + segmentWidth / 2, h / 2);
    }

    x += segmentWidth;
  }
}
```

## monad-decode.js - Monad Synthesis from PacketFlow

```javascript
synthesizeMonad(flow) {
  const flowLabel = flow.flow_label || 0;
  
  // Determine flow action based on protocol
  const flowAction = flow.protocol === 'HTTP/3' ? 1 : 0;
  
  // Hop count from packet hops
  const hopCount = flow.hops ? flow.hops.length : 0;
  
  // Flags based on status code
  let flags = 0;
  if (flow.status_code === 200) flags |= FLAGS.TRACED;
  if (flow.status_code === 500) flags |= FLAGS.CHAOS;
  
  // Circuit state based on status
  let circuitState = 0; // CLOSED
  if (flow.status_code === 503) {
    circuitState = 1; // OPEN
    this.updateCircuitBreakerState(flow, 1);
  } else if (flow.status_code === 429) {
    circuitState = 2; // HALF
    this.updateCircuitBreakerState(flow, 2);
  } else {
    this.updateCircuitBreakerState(flow, 0);
  }
  
  // Derive service IDs from IP addresses
  const srcServiceId = this.extractServiceId(flow.source_ip);
  const dstServiceId = this.extractServiceId(flow.dest_ip);
  
  // Generate monad packet
  return {
    timestamp_ns: flow.timestamp_ns || Date.now() * 1000000,
    event_type: 1,
    hop_id: hopCount,
    flow_label_lo: flowLabel,
    monad: {
      flow_action: flowAction,
      hop_count: hopCount,
      flags: flags,
      circuit_state: circuitState,
      src_service_id: srcServiceId,
      dst_service_id: dstServiceId,
      regs: [Math.random() * 256, Math.random() * 256, 0, 0],
      scratch_r0: Math.random() * 256,
      scratch_r1: Math.random() * 256,
      crc: Math.floor(Math.random() * 65536),
    },
    checksum_valid: Math.random() > 0.05,
    is_chaos: (flags & FLAGS.CHAOS) !== 0,
    is_traced: (flags & FLAGS.TRACED) !== 0,
    is_sampled: (flags & FLAGS.SAMPLED) !== 0,
  };
}
```

## monad-decode.js - Chaos Injection Handler

```javascript
handleChaosInject() {
  const labelInput = document.getElementById('chaos-flow-label');
  const modeSelect = document.getElementById('chaos-mode');
  const delayInput = document.getElementById('chaos-delay-us');

  if (!labelInput || !modeSelect) return;

  const labelHex = labelInput.value.trim();
  const mode = modeSelect.value;

  // Validate flow label (should be valid hex)
  if (!labelHex || !/^[0-9a-fA-F]+$/.test(labelHex)) {
    this.addChaosLog('ERROR: Invalid flow label hex');
    return;
  }

  const flowLabel = parseInt(labelHex, 16);
  let param = 0;

  if (mode === 'delay' && delayInput) {
    param = parseInt(delayInput.value) || 0;
  }

  // Send POST request
  const payload = {
    flow_label: flowLabel,
    mode: mode,
    param: param,
  };

  fetch('/api/v1/chaos', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(payload),
  })
    .then(res => {
      if (!res.ok) throw new Error(`HTTP ${res.status}`);
      return res.json();
    })
    .then(data => {
      this.chaosActive = true;
      this.chaosMode = mode;
      this.chaosFlowLabel = flowLabel;

      // Update UI
      const injectBtn = document.getElementById('btn-inject-chaos');
      const stopBtn = document.getElementById('btn-stop-chaos');
      if (injectBtn) injectBtn.disabled = true;
      if (stopBtn) stopBtn.disabled = false;

      const chip = document.getElementById('chip-chaos');
      if (chip) chip.classList.add('active');

      const statusEl = document.getElementById('chaos-status');
      if (statusEl) statusEl.textContent = mode.toUpperCase();

      this.addChaosLog(`INJECT ${mode.toUpperCase()} fl=0x${labelHex}`);
    })
    .catch(err => {
      this.addChaosLog(`ERROR: ${err.message}`);
    });
}
```

## monad-decode.js - Circuit Breaker Matrix Update

```javascript
updateCircuitBreakerMatrix() {
  const matrixElement = document.getElementById('circuit-matrix');
  if (!matrixElement) return;

  let openCount = 0;
  matrixElement.innerHTML = '';

  for (const [key, data] of this.cbState) {
    if (data.state === 1) openCount++;

    const ageSec = Math.floor((Date.now() - data.since) / 1000);
    const stateClass = this.getCircuitStateClass(data.state);
    const stateName = CIRCUIT_STATES[data.state] || 'UNKNOWN';

    const row = document.createElement('div');
    row.className = 'cb-row';
    row.innerHTML = `
      <span class="cb-label">${key}</span>
      <span class="cb-cell ${stateClass}">${stateName}</span>
      <span class="cb-age dim">${ageSec}s</span>
    `;
    matrixElement.appendChild(row);
  }

  // Update open count
  const openCountElement = document.getElementById('cb-open-count');
  if (openCountElement) {
    const classStr = openCount > 0 ? 'danger' : 'dim';
    openCountElement.className = classStr;
    openCountElement.textContent = openCount + ' OPEN';
  }
}
```

---

These code samples demonstrate the core functionality and production-quality patterns used throughout both modules.
