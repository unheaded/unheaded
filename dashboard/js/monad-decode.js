/**
 * Unheaded eBPF Dashboard - Monad Decode Module
 * Manages monad panel, circuit breaker matrix, and chaos injection control
 * Pure vanilla JS, no external dependencies
 */

/**
 * Monad Protocol Constants
 */
const FLOW_ACTIONS = {
  0: 'FORWARD',
  1: 'TRACE',
  2: 'SAMPLE',
  3: 'MIRROR',
  4: 'DROP',
};

const FLAGS = {
  TRACED: 0x01,
  SAMPLED: 0x02,
  MIRROR: 0x04,
  CHAOS: 0x08,
  CUSTOM: 0x10,
};

const CIRCUIT_STATES = {
  0: 'CLOSED',
  1: 'OPEN',
  2: 'HALF',
};

/**
 * Monad Decoder and Panel Manager
 */
window.UnheadedMonad = {
  // Current monad state
  currentMonad: null,
  
  // Circuit breaker state tracking
  cbState: new Map(), // "srcId→dstId" → { state, since }
  cbUpdateTime: 0,

  // Chaos injection state
  chaosActive: false,
  chaosMode: null,
  chaosFlowLabel: null,
  chaosLog: [],

  /**
   * Initialize monad panels
   */
  init() {
    this.setupChaosHandlers();
  },

  /**
   * Synthesize monad from PacketFlow data
   */
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
    
    // Fake register values for demo
    const regs = [
      Math.floor(Math.random() * 256),
      Math.floor(Math.random() * 256),
      0,
      0,
    ];
    
    const scratch_r0 = Math.floor(Math.random() * 256);
    const scratch_r1 = Math.floor(Math.random() * 256);
    
    // Generate fake CRC
    const crc = Math.floor(Math.random() * 65536);

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
        regs: regs,
        scratch_r0: scratch_r0,
        scratch_r1: scratch_r1,
        crc: crc,
      },
      checksum_valid: Math.random() > 0.05,
      is_chaos: (flags & FLAGS.CHAOS) !== 0,
      is_traced: (flags & FLAGS.TRACED) !== 0,
      is_sampled: (flags & FLAGS.SAMPLED) !== 0,
    };
  },

  /**
   * Extract service ID from IP address (last octet % 16)
   */
  extractServiceId(ipAddr) {
    if (!ipAddr) return Math.floor(Math.random() * 16);
    const parts = ipAddr.split('.');
    const lastOctet = parseInt(parts[parts.length - 1]) || 0;
    return lastOctet % 16;
  },

  /**
   * Update circuit breaker state for service pair
   */
  updateCircuitBreakerState(flow, state) {
    const srcId = this.extractServiceId(flow.source_ip);
    const dstId = this.extractServiceId(flow.dest_ip);
    const key = `svc${srcId}→svc${dstId}`;

    this.cbState.set(key, {
      state: state,
      since: Date.now(),
      statusCode: flow.status_code,
    });

    // Keep only last 20 entries
    if (this.cbState.size > 20) {
      const firstKey = this.cbState.keys().next().value;
      this.cbState.delete(firstKey);
    }
  },

  /**
   * Process PacketFlow message
   */
  onPacketFlow(flow) {
    // Synthesize monad from flow data
    const monad = this.synthesizeMonad(flow);
    this.currentMonad = monad;
    
    // Update monad panel
    this.updateMonadPanel(monad);
    
    // Circuit breaker already updated in synthesizeMonad
    // Update circuit breaker UI every 2 seconds
    const now = Date.now();
    if (now - this.cbUpdateTime > 2000) {
      this.updateCircuitBreakerMatrix();
      this.cbUpdateTime = now;
    }
  },

  /**
   * Process AnamnesisEvent with real monad data
   */
  onAnamnesisEvent(evt) {
    if (evt.monad) {
      this.currentMonad = evt;
      this.updateMonadPanel(evt);
    }
  },

  /**
   * Update monad panel DOM elements
   */
  updateMonadPanel(monad) {
    const m = monad.monad || {};

    // Flow action
    const faElement = document.getElementById('mf-fa');
    if (faElement) {
      const actionName = FLOW_ACTIONS[m.flow_action] || 'UNKNOWN';
      const colorClass = this.getFlowActionColor(m.flow_action);
      faElement.innerHTML = `<span class="indicator ${colorClass}">●</span> ${actionName}`;
    }

    // Hop count
    const hcElement = document.getElementById('mf-hc');
    if (hcElement) {
      const accent = m.hop_count > 5 ? ' accent-large' : ' accent';
      hcElement.className = 'metric-value' + accent;
      hcElement.textContent = m.hop_count;
    }

    // Flags
    const flagsElement = document.getElementById('mf-flags');
    if (flagsElement) {
      const flagNames = [];
      for (const [flagName, flagValue] of Object.entries(FLAGS)) {
        if ((m.flags & flagValue) !== 0) {
          flagNames.push(flagName);
        }
      }
      flagsElement.textContent = flagNames.length > 0 ? flagNames.join(' | ') : '(none)';
    }

    // Circuit state
    const csElement = document.getElementById('mf-circuit');
    if (csElement) {
      const stateName = CIRCUIT_STATES[m.circuit_state] || 'UNKNOWN';
      const stateClass = this.getCircuitStateClass(m.circuit_state);
      csElement.className = 'metric-value ' + stateClass;
      csElement.textContent = stateName;
    }

    // CRC checksum
    const crcElement = document.getElementById('mf-crc');
    if (crcElement) {
      const crcHex = '0x' + m.crc.toString(16).padStart(4, '0').toUpperCase();
      const checkmark = monad.checksum_valid ? ' ✓' : ' ✗';
      const checkClass = monad.checksum_valid ? 'valid' : 'invalid';
      crcElement.innerHTML = `<span class="${checkClass}">${crcHex}${checkmark}</span>`;
    }

    // Service IDs
    const srcElement = document.getElementById('mf-src');
    if (srcElement) {
      srcElement.textContent = m.src_service_id;
    }

    const dstElement = document.getElementById('mf-dst');
    if (dstElement) {
      dstElement.textContent = m.dst_service_id;
    }

    // Registers
    const regsElement = document.getElementById('mf-regs');
    if (regsElement) {
      const regsStr = m.regs ? `[${m.regs.join(',')}]` : '[0,0,0,0]';
      regsElement.textContent = regsStr;
    }

    // Scratch registers
    const scratchElement = document.getElementById('mf-scratch');
    if (scratchElement) {
      scratchElement.textContent = `r0=${m.scratch_r0} r1=${m.scratch_r1}`;
    }

    // Flow label
    const flElement = document.getElementById('monad-flow-label');
    if (flElement) {
      const labelHex = monad.flow_label_lo.toString(16).padStart(4, '0').toUpperCase();
      flElement.textContent = `fl=0x${labelHex}`;
    }

    // Hex dump (fake 20-byte representation)
    const hexElement = document.getElementById('monad-hex');
    if (hexElement) {
      const hexBytes = this.generateFakeHexDump(m);
      hexElement.textContent = hexBytes;
    }
  },

  /**
   * Generate fake monad hex dump for visualization
   */
  generateFakeHexDump(monad) {
    // Synthesize 20 bytes from monad fields
    const bytes = [];
    bytes.push(monad.flow_action & 0xFF);
    bytes.push((monad.hop_count & 0xFF));
    bytes.push(monad.flags & 0xFF);
    bytes.push((monad.circuit_state << 4) | (monad.src_service_id & 0x0F));
    bytes.push(monad.dst_service_id & 0xFF);
    
    for (let i = 5; i < 20; i++) {
      bytes.push(Math.floor(Math.random() * 256));
    }

    return bytes.map(b => b.toString(16).padStart(2, '0').toUpperCase()).join(' ');
  },

  /**
   * Get color class for flow action indicator
   */
  getFlowActionColor(action) {
    switch (action) {
      case 0: return 'color-forward'; // FORWARD - green
      case 1: return 'color-trace'; // TRACE - blue
      case 2: return 'color-sample'; // SAMPLE - purple
      case 3: return 'color-mirror'; // MIRROR - cyan
      case 4: return 'color-drop'; // DROP - red
      default: return 'color-default';
    }
  },

  /**
   * Get color class for circuit state
   */
  getCircuitStateClass(state) {
    switch (state) {
      case 0: return 'state-closed'; // CLOSED - green
      case 1: return 'state-open'; // OPEN - red
      case 2: return 'state-half'; // HALF - amber
      default: return 'state-default';
    }
  },

  /**
   * Update circuit breaker matrix display
   */
  updateCircuitBreakerMatrix() {
    const matrixElement = document.getElementById('circuit-matrix');
    if (!matrixElement) return;

    // Count open circuits
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
  },

  /**
   * Setup chaos injection event handlers
   */
  setupChaosHandlers() {
    const modeSelect = document.getElementById('chaos-mode');
    const delayParamRow = document.getElementById('delay-param-row');
    const injectBtn = document.getElementById('btn-inject-chaos');
    const stopBtn = document.getElementById('btn-stop-chaos');

    // Mode change handler - show/hide delay parameter
    if (modeSelect) {
      modeSelect.addEventListener('change', (e) => {
        if (delayParamRow) {
          delayParamRow.style.display = e.target.value === 'delay' ? 'block' : 'none';
        }
      });
    }

    // Inject button handler
    if (injectBtn) {
      injectBtn.addEventListener('click', () => {
        this.handleChaosInject();
      });
    }

    // Stop button handler
    if (stopBtn) {
      stopBtn.addEventListener('click', () => {
        this.handleChaosStop();
      });
    }

    // Hide delay param row initially
    if (delayParamRow && modeSelect?.value !== 'delay') {
      delayParamRow.style.display = 'none';
    }
  },

  /**
   * Handle chaos injection request
   */
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
  },

  /**
   * Handle chaos stop request
   */
  handleChaosStop() {
    fetch('/api/v1/chaos/stop', {
      method: 'DELETE',
      headers: { 'Content-Type': 'application/json' },
    })
      .then(res => {
        if (!res.ok) throw new Error(`HTTP ${res.status}`);
        return res.json();
      })
      .then(data => {
        this.chaosActive = false;
        this.chaosMode = null;
        this.chaosFlowLabel = null;

        // Update UI
        const injectBtn = document.getElementById('btn-inject-chaos');
        const stopBtn = document.getElementById('btn-stop-chaos');
        if (injectBtn) injectBtn.disabled = false;
        if (stopBtn) stopBtn.disabled = true;

        const chip = document.getElementById('chip-chaos');
        if (chip) chip.classList.remove('active');

        const statusEl = document.getElementById('chaos-status');
        if (statusEl) statusEl.textContent = 'OFF';

        this.addChaosLog('STOPPED');
      })
      .catch(err => {
        this.addChaosLog(`ERROR: ${err.message}`);
      });
  },

  /**
   * Append log entry to chaos log
   */
  addChaosLog(message) {
    const logElement = document.getElementById('chaos-log');
    if (!logElement) return;

    // Get current time
    const now = new Date();
    const timeStr = now.getHours().toString().padStart(2, '0') + ':' +
      now.getMinutes().toString().padStart(2, '0') + ':' +
      now.getSeconds().toString().padStart(2, '0');

    // Create log entry
    const entry = document.createElement('div');
    entry.className = 'chaos-log-entry';
    entry.textContent = `[${timeStr}] ${message}`;

    logElement.appendChild(entry);

    // Keep only last 10 entries
    const entries = logElement.querySelectorAll('.chaos-log-entry');
    if (entries.length > 10) {
      for (let i = 0; i < entries.length - 10; i++) {
        entries[i].remove();
      }
    }

    // Auto-scroll to bottom
    logElement.scrollTop = logElement.scrollHeight;
  },

  /**
   * Reset chaos state
   */
  reset() {
    this.chaosActive = false;
    this.chaosMode = null;
    this.chaosFlowLabel = null;
    this.chaosLog = [];

    const injectBtn = document.getElementById('btn-inject-chaos');
    const stopBtn = document.getElementById('btn-stop-chaos');
    if (injectBtn) injectBtn.disabled = false;
    if (stopBtn) stopBtn.disabled = true;

    const chip = document.getElementById('chip-chaos');
    if (chip) chip.classList.remove('active');
  },
};

// Auto-initialize when DOM is ready
if (document.readyState === 'loading') {
  document.addEventListener('DOMContentLoaded', () => {
    window.UnheadedMonad.init();
  });
} else {
  window.UnheadedMonad.init();
}
