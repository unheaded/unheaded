# S68 GUI BUILDOUT BATTLE PLAN — 10 Phases, 124 Steps

**Date**: 2026-02-27
**Sprint**: S68 — Dashboard stubs → live viz, Doom CSS fix, Kanban drag+Marshal, Wiki contrast fix
**Prerequisite**: S66 IaC complete, dashboard-backend serving at port 8080, kanban-app serving
**Target**: All 4 dashboard tabs rendering real/demo data, doom.html text not overlapping, kanban drag-drop working with Marshal lane checks, wiki readable
**Estimated Duration**: 8-14 hours across 3-4 sessions
**Agent Strategy**: Phases 1-2 sequential. Phases 3-5 parallelizable (independent pages). Phase 6-7 parallelizable. Phase 8-9 sequential. Phase 10 final gate.
**Commit Cadence**: Every 4 steps
**Stuck Protocol**: Skip after 3x time estimate or 2 failed attempts. Log STUCK. Move forward.

---

## LEGEND

[B] = Bash command (run directly)
[V] = Verification step (MUST pass before proceeding)
[D] = Debug step (only if prior step fails)
[W] = Write/create file
[R] = Read/inspect file
[S] = Sudo required
[P] = Parallelizable with other marked steps
[C] = Commit checkpoint
[STUCK] = Step skipped via Skip Protocol
[BLOCKED] = Step blocked by upstream STUCK step

---

## PHASE 0: INTELLIGENCE & ENVIRONMENT (Steps 1-8)

**Goal**: Confirm current state of all GUI files, identify exactly what's stub vs functional.
**Prerequisite**: Repo cloned, node/go available.
**Time**: 15 minutes
**Agent**: Coordinator

- [ ] **Step 1** [R] ~1m: Read dashboard.js to map which renderers exist vs stub
  ```bash
  wc -l /sessions/nice-blissful-dirac/mnt/tmp/unheaded/cmd/dashboard-backend/static/dashboard.js
  ```

- [ ] **Step 2** [R] ~1m: Confirm flow graph renderer exists (renderFlowGraph function)
  ```bash
  grep -n "function renderFlowGraph\|function renderLatency\|function appendEventStream\|function renderLatencySummary" /sessions/nice-blissful-dirac/mnt/tmp/unheaded/cmd/dashboard-backend/static/dashboard.js
  ```

- [ ] **Step 3** [V] ~2m: Identify the gap — what's stub vs implemented
  ```
  EXPECTED FINDINGS:
  - renderFlowGraph: EXISTS but shows "No active flows" when no WS data
  - renderLatencySummary/Charts: EXISTS but shows "No latency data" when no API data
  - appendEventStreamItems: EXISTS but stream is empty without eBPF events
  - REAL PROBLEM: No demo/synthetic data generators for these tabs
  - These pages WORK when connected to live backend — they're NOT broken,
    they just show empty state when there's no data source
  ```

- [ ] **Step 4** [R] ~1m: Check if demo-data.js exists in dashboard-backend
  ```bash
  find /sessions/nice-blissful-dirac/mnt/tmp/unheaded/cmd/dashboard-backend -name "*demo*" -o -name "*synthetic*" -o -name "*mock*" 2>/dev/null
  ```

- [ ] **Step 5** [R] ~1m: Check doom.html for CSS overlap issues
  ```bash
  # The issue: overlapping text — likely the nav, overlay, or panel positioning
  grep -n "position:\|z-index\|padding-top\|top:" /sessions/nice-blissful-dirac/mnt/tmp/unheaded/dashboard/doom.html | head -20
  ```

- [ ] **Step 6** [R] ~1m: Check kanban drag-drop implementation status
  ```bash
  grep -n "dragstart\|dragover\|drop\|ondrag\|draggable" /sessions/nice-blissful-dirac/mnt/tmp/unheaded/kanban/index.html /sessions/nice-blissful-dirac/mnt/tmp/unheaded/kanban/js/board-viz.js 2>/dev/null | head -20
  ```

- [ ] **Step 7** [R] ~1m: Check wiki-server color variables
  ```bash
  grep -n "text-secondary\|text-primary\|bg-primary" /sessions/nice-blissful-dirac/mnt/tmp/unheaded/cmd/wiki-server/main.go | head -10
  ```

- [ ] **Step 8** [V] ~1m: **PHASE 0 EXIT GATE** — All issues confirmed
  ```
  [ ] Dashboard tabs: renderers exist, need demo data generators
  [ ] doom.html: nav overlap / padding issue identified
  [ ] kanban/: drag-drop status confirmed (kanban-app has it, basic kanban doesn't)
  [ ] wiki-server: --text-secondary: #666666 confirmed too low contrast
  ```

---

## PHASE 1: WIKI TEXT CONTRAST FIX (Steps 9-14)

**Goal**: Fix wiki text readability — #666 on #0a0a0a is WCAG-failing. Quick win.
**Prerequisite**: Phase 0 complete.
**Time**: 15 minutes
**Agent**: Coordinator (quick fix)

- [ ] **Step 9** [R] ~2m: Read wiki-server main.go CSS section to find all color vars
  ```bash
  grep -n -A2 ":root" /sessions/nice-blissful-dirac/mnt/tmp/unheaded/cmd/wiki-server/main.go
  ```

- [ ] **Step 10** [W] ~3m: Fix color variables for WCAG AA contrast
  ```
  File: cmd/wiki-server/main.go
  Changes:
  OLD: --text-primary: #c9c9c9;     → NEW: --text-primary: #e0e0e0;
  OLD: --text-secondary: #666666;   → NEW: --text-secondary: #999999;
  OLD: --text-heading: #ffffff;     → KEEP (already max contrast)

  RATIONALE:
  - #666 on #0a0a0a = contrast ratio ~3.5:1 (FAILS WCAG AA 4.5:1)
  - #999 on #0a0a0a = contrast ratio ~6.3:1 (PASSES WCAG AA)
  - #e0e0e0 on #0a0a0a = contrast ratio ~13.5:1 (PASSES WCAG AAA)
  ```

- [ ] **Step 11** [W] ~2m: Also fix any inline color: #3a3a3a occurrences
  ```bash
  grep -n "#3a3a3a" /sessions/nice-blissful-dirac/mnt/tmp/unheaded/cmd/wiki-server/main.go
  ```
  Change #3a3a3a → #888888 (or use var(--text-secondary))

- [ ] **Step 12** [V] ~1m: Verify changes compile
  ```bash
  cd /sessions/nice-blissful-dirac/mnt/tmp/unheaded && go build ./cmd/wiki-server/
  ```

- [ ] **Step 13** [C] ~1m: **COMMIT CHECKPOINT**
  ```bash
  git add cmd/wiki-server/main.go && git commit -m "fix(wiki): increase text contrast to WCAG AA compliance (#999 secondary, #e0e0e0 primary)"
  ```

- [ ] **Step 14** [V] ~1m: **PHASE 1 EXIT GATE** — Wiki text readable
  - --text-secondary is now #999999 or brighter
  - --text-primary is now #e0e0e0 or brighter
  - No #3a3a3a or #666666 remains in body text contexts
  - go build succeeds

---

## PHASE 2: DOOM.HTML CSS OVERLAP FIX (Steps 15-24)

**Goal**: Fix text/element overlap on doom.html viewer page.
**Prerequisite**: Phase 0 complete.
**Time**: 30 minutes
**Agent**: Coordinator

- [ ] **Step 15** [R] ~3m: Identify overlap root cause
  ```
  KNOWN ISSUES in doom.html:
  1. body padding-top: calc(80px + var(--space-xl)) — assumes nav height is 80px
     but nav from design-system.css may have different height
  2. .overlay-panel position: absolute inside #screen-panel position: relative
     — overlay may clip or overlap the h2/controls
  3. .fps-display position: fixed top: 70px right — may collide with nav
  4. .container flex-wrap: wrap — at certain widths, panels stack poorly
  5. No max-width on #status-panel content — text can overflow
  ```

- [ ] **Step 16** [W] ~5m: Fix body padding to use CSS var instead of hardcoded 80px
  ```
  File: dashboard/doom.html
  Changes in <style>:

  OLD: padding: calc(80px + var(--space-xl)) var(--space-xl) var(--space-xl);
  NEW: padding: calc(var(--nav-height, 64px) + var(--space-xl)) var(--space-xl) var(--space-xl);
  ```

- [ ] **Step 17** [W] ~5m: Fix overlay panel positioning — constrain within canvas bounds
  ```
  File: dashboard/doom.html
  Changes:

  .overlay-panel {
    position: absolute;
    top: var(--space-sm);
    left: var(--space-sm);
    /* ADD: max-width to prevent overflow */
    max-width: calc(100% - 2 * var(--space-sm));
    /* ADD: pointer-events none already set, ensure z-index doesn't bleed */
    z-index: 1;  /* was var(--z-sticky) — too high, collides with nav */
  }
  ```

- [ ] **Step 18** [W] ~3m: Fix FPS display positioning
  ```
  File: dashboard/doom.html
  Changes:

  .fps-display {
    position: fixed;
    top: calc(var(--nav-height, 64px) + var(--space-sm));  /* was 70px */
    right: var(--space-md);
    z-index: 10;  /* was var(--z-sticky) */
  }
  ```

- [ ] **Step 19** [W] ~3m: Add min-width to container to prevent crush
  ```
  File: dashboard/doom.html
  Changes:

  .container {
    display: flex;
    gap: var(--space-xl);
    flex-wrap: wrap;
    justify-content: center;
    max-width: 1200px;
    /* ADD: */
    width: 100%;
  }

  #screen-panel {
    position: relative;
    /* ADD: flex sizing */
    flex: 1 1 640px;
    min-width: 0;  /* prevent flex blowout */
  }

  #status-panel {
    min-width: 280px;  /* was 300px */
    max-width: 360px;
    flex: 0 1 auto;
    /* ADD: overflow protection */
    overflow-wrap: break-word;
    word-break: break-all;
  }
  ```

- [ ] **Step 20** [W] ~3m: Fix register grid overflow on narrow screens
  ```
  File: dashboard/doom.html
  Changes:

  .registers {
    /* ADD: overflow scroll for tiny screens */
    overflow-x: auto;
  }

  .reg-val {
    color: var(--color-accent);
    /* ADD: prevent overflow */
    font-size: clamp(0.6rem, 1.5vw, 0.75rem);
  }
  ```

- [ ] **Step 21** [C] ~1m: **COMMIT CHECKPOINT**
  ```bash
  git add dashboard/doom.html && git commit -m "fix(doom): resolve CSS text overlap — nav padding, overlay z-index, flex layout"
  ```

- [ ] **Step 22** [W] ~3m: Fix subtitle collision with nav on scroll
  ```
  File: dashboard/doom.html
  Changes:

  /* Ensure content doesn't tuck under fixed nav */
  h1, .subtitle {
    position: relative;
    z-index: 0;
  }
  ```

- [ ] **Step 23** [V] ~3m: Visual verification — open doom.html in browser
  ```bash
  # If we can run a quick server:
  cd /sessions/nice-blissful-dirac/mnt/tmp/unheaded && python3 -m http.server 9999 &
  # Then check: no overlapping text at 1920x1080, 1366x768, 768x1024
  ```
  - If no browser available → verify CSS logic by reading the computed layout rules
  - Nav should not overlap content
  - Overlay should stay within canvas bounds
  - FPS display should not collide with nav
  - Registers should not overflow panel

- [ ] **Step 24** [V] ~1m: **PHASE 2 EXIT GATE** — doom.html layout clean
  - padding-top uses nav-height variable
  - overlay z-index lowered from z-sticky to 1
  - fps-display top uses nav-height calc
  - status-panel has overflow protection
  - Committed

---

## PHASE 3: DASHBOARD — DEMO DATA GENERATOR (Steps 25-40)

**Goal**: Create a client-side demo data generator so Flow Graph, Latency, and Events tabs show meaningful data even without a live backend.
**Prerequisite**: Phase 0 complete. Runs parallel with Phases 4-5.
**Time**: 90 minutes
**Agent**: Agent [P]

- [ ] **Step 25** [W] ~10m: Create demo data generator module
  ```
  File: cmd/dashboard-backend/static/js/demo-data.js
  Contents:
  - DemoDataGenerator class
  - generateFlows(): returns 8-15 synthetic flows between Kingdom services
    - src/dst from: captain, deck, engine, lookout, navigator, stores, kanban, gateway
    - Random protocols: TCP (70%), UDP (30%)
    - States: new, established, closing
    - Realistic byte/packet counts
  - generateLatency(): returns per-operation P50/P90/P99 with realistic distributions
    - Operations: tcp_connect, tcp_send, tcp_recv, http_request
    - P50: 1-5ms, P90: 5-20ms, P99: 20-100ms range
    - Slight random jitter per tick
  - generateEvent(): returns single event with type, topic, timestamp, summary
    - Types: packet, flow, latency, syscall
    - Realistic topics: ebpf.packet.ingress, ebpf.flow.new, etc.
  - tick(): generates one round of all data types, calls update functions
  ```

- [ ] **Step 26** [V] ~2m: Verify DemoDataGenerator produces valid data shapes
  ```
  Each generator must match the data shape dashboard.js expects:
  - Flows: { source: "synthetic", active_flows: [...], stats: { bytes_per_sec, packets_per_sec } }
  - Latency: { operations: { tcp_connect: { p50, p90, p99, unit }, ... } }
  - Events: { type, event_type, topic, timestamp, summary, data }
  ```

- [ ] **Step 27** [C] ~1m: **COMMIT CHECKPOINT**
  ```bash
  git add cmd/dashboard-backend/static/js/demo-data.js && git commit -m "feat(dashboard): add client-side demo data generator for flow/latency/events"
  ```

- [ ] **Step 28** [W] ~5m: Wire demo generator into dashboard.js initialization
  ```
  File: cmd/dashboard-backend/static/dashboard.js
  Changes at end of init():

  // After real data fetch setup, start demo mode if no WS connection after 5s
  setTimeout(function() {
      if (!state.wsConnected && typeof DemoDataGenerator !== 'undefined') {
          console.log('[Dashboard] No WS connection — starting demo mode');
          var demo = new DemoDataGenerator();
          setInterval(function() { demo.tick(); }, 2000);
          showToast('info', 'Demo Mode', 'Showing synthetic data — connect backend for real metrics');
      }
  }, 5000);
  ```

- [ ] **Step 29** [W] ~3m: Add demo-data.js script tag to index.html
  ```
  File: cmd/dashboard-backend/static/index.html
  Add before dashboard.js:
  <script src="js/demo-data.js"></script>
  ```

- [ ] **Step 30** [C] ~1m: **COMMIT CHECKPOINT**
  ```bash
  git add cmd/dashboard-backend/static/ && git commit -m "feat(dashboard): wire demo data generator — auto-activates when backend absent"
  ```

- [ ] **Step 31** [W] ~10m: Implement Flow Graph demo data with service topology
  ```
  In demo-data.js, generateFlows():

  var KINGDOM_SERVICES = [
      { name: 'gateway', ip: '10.10.10.100', port: 21443 },
      { name: 'wotan', ip: '10.10.10.10', port: 18001 },
      { name: 'captain', ip: '10.10.10.20', port: 19002 },
      { name: 'architect', ip: '10.10.10.21', port: 19001 },
      { name: 'timeguru', ip: '10.10.10.22', port: 19000 },
      { name: 'deck', ip: '10.10.10.23', port: 19006 },
      { name: 'lookout', ip: '10.10.10.24', port: 19004 },
      { name: 'navigator', ip: '10.10.10.25', port: 19005 }
  ];

  // Generate flows following the real service topology
  // Gateway → Wotan → Services → back
  // With realistic packet/byte counts that change each tick
  ```

- [ ] **Step 32** [W] ~10m: Implement Latency demo data with percentile distributions
  ```
  In demo-data.js, generateLatency():

  // Use log-normal distribution for realistic latency shapes
  function lognormal(mu, sigma) {
      var u1 = Math.random(), u2 = Math.random();
      var z = Math.sqrt(-2 * Math.log(u1)) * Math.cos(2 * Math.PI * u2);
      return Math.exp(mu + sigma * z);
  }

  // Per operation, maintain a sliding window of 100 samples
  // Calculate P50/P90/P99 from sorted window
  // Return histogram buckets for bar chart rendering
  ```

- [ ] **Step 33** [W] ~10m: Implement Events demo stream with all 4 types
  ```
  In demo-data.js, generateEvents():

  // Packet events: src→dst with protocol, every 500ms
  // Flow events: new/established/closing state changes, every 2s
  // Latency events: operation + latency_us, every 1s
  // Syscall events: read/write/connect/accept + pid, every 3s
  // Each with realistic Wotan topic paths
  ```

- [ ] **Step 34** [C] ~1m: **COMMIT CHECKPOINT**
  ```bash
  git add cmd/dashboard-backend/static/js/demo-data.js && git commit -m "feat(dashboard): full demo data — topology flows, lognormal latency, 4-type event stream"
  ```

- [ ] **Step 35** [W] ~5m: Add demo mode indicator to UI
  ```
  File: cmd/dashboard-backend/static/styles.css
  Add:
  .demo-banner {
      position: fixed;
      bottom: 40px;
      left: 50%;
      transform: translateX(-50%);
      background: rgba(255, 215, 0, 0.15);
      border: 1px solid rgba(255, 215, 0, 0.3);
      color: #ffd700;
      padding: 4px 16px;
      border-radius: 4px;
      font-size: 11px;
      font-family: monospace;
      z-index: 100;
      pointer-events: none;
  }
  ```

- [ ] **Step 36** [W] ~5m: Ensure Flow Graph canvas resizes properly on tab switch
  ```
  File: cmd/dashboard-backend/static/dashboard.js
  In switchPage():

  // After activating page, trigger resize for canvas pages
  if (page === 'flows') {
      requestAnimationFrame(resizeFlowCanvas);
  }
  if (page === 'latency') {
      requestAnimationFrame(function() {
          renderLatencyCharts(state.latencyData);
          renderLatencyHistory(state.latencyData);
      });
  }
  ```

- [ ] **Step 37** [V] ~3m: Verify all 4 tabs render with demo data
  ```
  Open dashboard in browser or audit code flow:
  [ ] Overview tab: gauges animate, services grid populated
  [ ] Flow Graph tab: canvas shows nodes in circular layout with animated edges
  [ ] Latency tab: P50/P90/P99 bars rendered, sparkline animates
  [ ] Events tab: stream scrolls with packet/flow/latency/syscall items
  ```

- [ ] **Step 38** [C] ~1m: **COMMIT CHECKPOINT**
  ```bash
  git add cmd/dashboard-backend/static/ && git commit -m "feat(dashboard): demo mode indicator, canvas resize on tab switch"
  ```

- [ ] **Step 39** [W] ~5m: Add keyboard shortcut to toggle demo mode (D key)
  ```
  File: cmd/dashboard-backend/static/dashboard.js
  Add to init():

  document.addEventListener('keydown', function(e) {
      if (e.key === 'd' && e.altKey) {
          // Toggle demo data on/off
          if (state.demoActive) { clearInterval(state.demoInterval); state.demoActive = false; }
          else { state.demoInterval = setInterval(function() { state.demo.tick(); }, 2000); state.demoActive = true; }
          showToast('info', state.demoActive ? 'Demo ON' : 'Demo OFF', 'Alt+D toggles synthetic data');
      }
  });
  ```

- [ ] **Step 40** [V] ~2m: **PHASE 3 EXIT GATE** — All dashboard tabs show data
  - demo-data.js exists with Flow, Latency, Event generators
  - Auto-activates after 5s with no WS connection
  - All 4 tabs render content (not empty/stub)
  - Demo banner visible when in demo mode
  - All committed

---

## PHASE 4: DASHBOARD — FLOW GRAPH ENHANCEMENTS (Steps 41-50)

**Goal**: Make Flow Graph tab production-quality with animated packets, hover tooltips, and service labels.
**Prerequisite**: Phase 3 complete (demo data available).
**Time**: 45 minutes
**Agent**: Agent [P with Phase 5]

- [ ] **Step 41** [W] ~10m: Add animated packet dots on flow edges
  ```
  File: cmd/dashboard-backend/static/dashboard.js
  In renderFlowGraph(), after drawing edges:

  // Animate packet dots along edges
  // Each flow gets a dot that moves from src→dst
  // Position = (timestamp % animDuration) / animDuration
  // Color matches protocol color
  // Size proportional to log(bytes)
  // Use requestAnimationFrame for smooth 60fps
  ```

- [ ] **Step 42** [W] ~5m: Add hover tooltips on flow nodes
  ```
  File: cmd/dashboard-backend/static/dashboard.js

  // Track mouse position on canvas
  // On mousemove: check distance to each node
  // If within radius: show tooltip with service name, IP, connection count
  // Draw tooltip as rounded rect with text
  ```

- [ ] **Step 43** [W] ~5m: Add service icons/labels using Kingdom naming
  ```
  File: cmd/dashboard-backend/static/dashboard.js

  var SERVICE_ICONS = {
      'gateway': '🏰',
      'wotan': '⚡',
      'captain': '👑',
      'architect': '📐',
      'timeguru': '⏰',
      'deck': '🚢',
      'lookout': '👁️',
      'navigator': '🧭',
      'kanban': '📋'
  };
  // Draw icon above node, name below
  ```

- [ ] **Step 44** [C] ~1m: **COMMIT CHECKPOINT**
  ```bash
  git add cmd/dashboard-backend/static/ && git commit -m "feat(dashboard): flow graph — animated packets, hover tooltips, service icons"
  ```

- [ ] **Step 45** [W] ~5m: Add flow graph layout toggle (circular vs force-directed)
  ```
  File: cmd/dashboard-backend/static/index.html
  Add button to flow-controls:
  <button id="flow-layout-toggle" class="flow-control-btn">Layout: Ring</button>

  File: cmd/dashboard-backend/static/dashboard.js
  Add alternate layout: simple force-directed with spring/repulsion
  ```

- [ ] **Step 46** [W] ~5m: Add flow edge thickness legend
  ```
  File: cmd/dashboard-backend/static/index.html
  After flow-legend div, add:
  <div class="flow-thickness-legend">
      <span>Edge width = log₂(bytes)</span>
  </div>
  ```

- [ ] **Step 47** [V] ~2m: Verify flow graph renders cleanly at multiple window sizes
  ```
  Check:
  [ ] 1920x1080: full spread, labels readable
  [ ] 1366x768: no clipping, nodes don't overlap
  [ ] 768x1024: canvas fills width, nodes scaled down
  ```

- [ ] **Step 48** [C] ~1m: **COMMIT CHECKPOINT**
  ```bash
  git add cmd/dashboard-backend/static/ && git commit -m "feat(dashboard): flow graph layout toggle, edge legend"
  ```

- [ ] **Step 49** [W] ~3m: Add requestAnimationFrame loop for smooth animations
  ```
  File: cmd/dashboard-backend/static/dashboard.js

  function flowAnimationLoop() {
      if (state.activePage === 'flows') {
          renderFlowGraph();  // includes packet dot animation
      }
      state.animationFrame = requestAnimationFrame(flowAnimationLoop);
  }
  // Start on tab switch to flows, cancel on tab switch away
  ```

- [ ] **Step 50** [V] ~1m: **PHASE 4 EXIT GATE** — Flow graph production-ready
  - Animated packet dots move along edges
  - Hover shows service details
  - Service icons render
  - Layout toggle works
  - Animation loop runs at 60fps without memory leak

---

## PHASE 5: DASHBOARD — LATENCY & EVENTS ENHANCEMENTS (Steps 51-62)

**Goal**: Make Latency and Events tabs production-quality.
**Prerequisite**: Phase 3 complete.
**Time**: 45 minutes
**Agent**: Agent [P with Phase 4]

- [ ] **Step 51** [W] ~5m: Add histogram rendering to latency charts
  ```
  File: cmd/dashboard-backend/static/dashboard.js
  Enhance drawBarChart():

  // When histogram data available, draw proper histogram
  // X axis: latency buckets (0.1ms, 0.5ms, 1ms, 5ms, 10ms, 50ms, 100ms+)
  // Y axis: count
  // Color gradient: green→yellow→red based on bucket position
  // Draw P50/P90/P99 vertical lines with labels
  ```

- [ ] **Step 52** [W] ~5m: Add latency anomaly highlighting
  ```
  File: cmd/dashboard-backend/static/dashboard.js
  In renderLatencyHistory():

  // When P99 exceeds 3x P50, draw red band on sparkline
  // Add tooltip: "Anomaly: P99/P50 ratio = X.Xx"
  // Flash the latency card border red briefly
  ```

- [ ] **Step 53** [W] ~5m: Enhance event stream with color-coded type badges
  ```
  File: cmd/dashboard-backend/static/styles.css
  Add:
  .event-stream-item.type-packet .event-stream-type { background: #4ecdc4; color: #0a0a0a; }
  .event-stream-item.type-flow .event-stream-type { background: #ffd700; color: #0a0a0a; }
  .event-stream-item.type-latency .event-stream-type { background: #ff9800; color: #0a0a0a; }
  .event-stream-item.type-syscall .event-stream-type { background: #9c27b0; color: #fff; }

  .event-stream-type {
      display: inline-block;
      padding: 1px 6px;
      border-radius: 3px;
      font-size: 10px;
      font-weight: bold;
      text-transform: uppercase;
      min-width: 50px;
      text-align: center;
  }
  ```

- [ ] **Step 54** [C] ~1m: **COMMIT CHECKPOINT**
  ```bash
  git add cmd/dashboard-backend/static/ && git commit -m "feat(dashboard): latency histograms, anomaly detection, colored event types"
  ```

- [ ] **Step 55** [W] ~5m: Add event stream JSON detail expansion
  ```
  File: cmd/dashboard-backend/static/dashboard.js
  In appendEventStreamItems():

  // Click on event row → expand to show full JSON payload
  item.addEventListener('click', function() {
      var detail = this.querySelector('.event-detail');
      if (detail) { detail.remove(); return; }
      detail = document.createElement('pre');
      detail.className = 'event-detail';
      detail.textContent = JSON.stringify(ev.data || ev, null, 2);
      this.appendChild(detail);
  });
  ```

- [ ] **Step 56** [W] ~3m: Style event detail expansion
  ```
  File: cmd/dashboard-backend/static/styles.css
  Add:
  .event-detail {
      background: #0d0d0d;
      border: 1px solid #222;
      border-radius: 4px;
      padding: 8px;
      margin-top: 4px;
      font-size: 11px;
      color: #adb5bd;
      max-height: 200px;
      overflow-y: auto;
      white-space: pre-wrap;
      word-break: break-all;
  }
  ```

- [ ] **Step 57** [W] ~5m: Add event rate sparkline to event controls bar
  ```
  File: cmd/dashboard-backend/static/index.html
  Add after event-stream-stats:
  <canvas id="event-rate-sparkline" width="200" height="30" style="vertical-align:middle;margin-left:8px;"></canvas>

  File: cmd/dashboard-backend/static/dashboard.js
  Track event rate per second, draw mini sparkline
  ```

- [ ] **Step 58** [C] ~1m: **COMMIT CHECKPOINT**
  ```bash
  git add cmd/dashboard-backend/static/ && git commit -m "feat(dashboard): event detail expansion, rate sparkline"
  ```

- [ ] **Step 59** [W] ~5m: Add latency heatmap view option
  ```
  File: cmd/dashboard-backend/static/dashboard.js

  // Add toggle button: "Bar Chart | Heatmap"
  // Heatmap: time on X, latency bucket on Y, color intensity = count
  // Uses canvas with cell-based rendering
  // Green (low) → Yellow (mid) → Red (high)
  ```

- [ ] **Step 60** [C] ~1m: **COMMIT CHECKPOINT**
  ```bash
  git add cmd/dashboard-backend/static/ && git commit -m "feat(dashboard): latency heatmap view toggle"
  ```

- [ ] **Step 61** [V] ~3m: Verify Latency and Events tabs render properly
  ```
  [ ] Latency: histograms visible, P-lines drawn, sparkline animates
  [ ] Latency: anomaly bands appear when P99 >> P50
  [ ] Events: stream scrolls, type badges colored, click expands JSON
  [ ] Events: rate sparkline updates
  ```

- [ ] **Step 62** [V] ~1m: **PHASE 5 EXIT GATE** — Latency + Events production-ready
  - All latency visualizations render
  - Events stream is filterable, expandable, colored
  - Rate sparkline animates
  - All committed

---

## PHASE 6: KANBAN DRAG-AND-DROP (Steps 63-82)

**Goal**: Implement drag-and-drop between columns in kanban/ (the basic kanban). Wire Review column to Marshal lane checker.
**Prerequisite**: Phase 0 complete.
**Time**: 60 minutes
**Agent**: Agent [P]

- [ ] **Step 63** [R] ~3m: Read kanban/index.html current column structure
  ```bash
  grep -n "column\|draggable\|card\|review" /sessions/nice-blissful-dirac/mnt/tmp/unheaded/kanban/index.html | head -30
  ```

- [ ] **Step 64** [R] ~3m: Read kanban-app cards.js for drag reference implementation
  ```bash
  grep -n "dragstart\|dragover\|drop\|dragend\|placeholder" /sessions/nice-blissful-dirac/mnt/tmp/unheaded/cmd/kanban-app/static/js/cards.js | head -30
  ```

- [ ] **Step 65** [W] ~10m: Add HTML5 drag-drop to kanban/index.html
  ```
  File: kanban/index.html (or kanban/js/board-viz.js)

  Changes:
  1. Add draggable="true" to all .task-card elements
  2. Add data-task-id attribute to each card
  3. Add column data-status attribute to each column body
  4. Implement dragstart: set dataTransfer, add .dragging class
  5. Implement dragover: prevent default, show drop indicator
  6. Implement drop: move card DOM, update task status via API
  7. Implement dragend: clean up classes
  ```

- [ ] **Step 66** [W] ~5m: Create drop placeholder element
  ```
  File: kanban/js/board-viz.js (or inline in index.html)

  var placeholder = document.createElement('div');
  placeholder.className = 'drop-placeholder';
  placeholder.innerHTML = '<span>Drop here</span>';

  // On dragover: insert placeholder at mouse Y position
  // On dragleave: remove placeholder
  // On drop: replace placeholder with card
  ```

- [ ] **Step 67** [W] ~5m: Style drag states
  ```
  File: kanban/css/kanban.css
  Add:

  .task-card[draggable="true"] { cursor: grab; }
  .task-card.dragging { opacity: 0.4; transform: rotate(2deg); }
  .column-body.drag-over { background: rgba(255, 215, 0, 0.05); border: 2px dashed rgba(255, 215, 0, 0.3); }
  .drop-placeholder {
      height: 60px;
      border: 2px dashed rgba(255, 215, 0, 0.4);
      border-radius: 8px;
      display: flex;
      align-items: center;
      justify-content: center;
      color: #ffd700;
      font-size: 12px;
      font-family: monospace;
      margin: 4px 0;
  }
  ```

- [ ] **Step 68** [C] ~1m: **COMMIT CHECKPOINT**
  ```bash
  git add kanban/ && git commit -m "feat(kanban): HTML5 drag-and-drop between columns with visual feedback"
  ```

- [ ] **Step 69** [W] ~10m: Implement API call on drop to persist status change
  ```
  File: kanban/js/board-viz.js

  function moveTask(taskId, newStatus) {
      // POST /api/v1/tasks/{taskId}/status
      fetch('/api/v1/tasks/' + taskId + '/status', {
          method: 'PUT',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ status: newStatus })
      })
      .then(function(r) {
          if (!r.ok) throw new Error('Status update failed');
          showToast('success', 'Task moved to ' + newStatus);
          updateColumnCounts();
      })
      .catch(function(err) {
          showToast('error', 'Failed to move task: ' + err.message);
          // Revert DOM — move card back to original column
          revertDrag(taskId);
      });
  }
  ```

- [ ] **Step 70** [W] ~3m: Add column count badge update after drag
  ```
  File: kanban/js/board-viz.js

  function updateColumnCounts() {
      document.querySelectorAll('.column').forEach(function(col) {
          var count = col.querySelectorAll('.task-card').length;
          var badge = col.querySelector('.column-count');
          if (badge) badge.textContent = count;
      });
  }
  ```

- [ ] **Step 71** [C] ~1m: **COMMIT CHECKPOINT**
  ```bash
  git add kanban/ && git commit -m "feat(kanban): persist drag-drop via API, column count badges"
  ```

- [ ] **Step 72** [W] ~10m: Wire Review column to Marshal lane checker
  ```
  File: kanban/js/board-viz.js

  // When a card is dropped into REVIEW column:
  // 1. Show "Marshal checking..." overlay on card
  // 2. POST to /api/v1/marshal/check with task details
  // 3. Marshal returns: { approved: bool, citations: [...], recommendations: [...] }
  // 4. If approved: show green checkmark, enable Approve button
  // 5. If not approved: show amber warning with citations
  //    - Display: "Marshal flags: scope creep detected" / "Missing verification gate" etc
  // 6. User can still force-approve but warning stays visible

  function marshalCheck(taskId, taskData) {
      var card = document.querySelector('[data-task-id="' + taskId + '"]');
      if (!card) return;

      // Add checking overlay
      var overlay = document.createElement('div');
      overlay.className = 'marshal-checking';
      overlay.innerHTML = '<span class="marshal-icon">⚔️</span> Marshal reviewing...';
      card.appendChild(overlay);

      fetch('/api/v1/marshal/check', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({
              task_id: taskId,
              title: taskData.title,
              description: taskData.description,
              from_status: taskData.previous_status,
              to_status: 'review'
          })
      })
      .then(function(r) { return r.json(); })
      .then(function(result) {
          overlay.remove();
          renderMarshalResult(card, result);
      })
      .catch(function() {
          overlay.remove();
          // Marshal unavailable — allow review without check
          renderMarshalResult(card, { approved: true, note: 'Marshal offline — manual review required' });
      });
  }
  ```

- [ ] **Step 73** [W] ~5m: Render Marshal review results on card
  ```
  File: kanban/js/board-viz.js

  function renderMarshalResult(card, result) {
      var badge = document.createElement('div');
      badge.className = 'marshal-badge ' + (result.approved ? 'approved' : 'flagged');

      if (result.approved) {
          badge.innerHTML = '<span class="marshal-icon">✅</span> Marshal approved';
      } else {
          badge.innerHTML = '<span class="marshal-icon">⚠️</span> Marshal flagged';
          if (result.citations && result.citations.length > 0) {
              var citList = document.createElement('ul');
              citList.className = 'marshal-citations';
              result.citations.forEach(function(c) {
                  var li = document.createElement('li');
                  li.textContent = c;
                  citList.appendChild(li);
              });
              badge.appendChild(citList);
          }
      }

      // Insert at top of card
      card.insertBefore(badge, card.firstChild);
  }
  ```

- [ ] **Step 74** [W] ~5m: Style Marshal badge and citations
  ```
  File: kanban/css/kanban.css
  Add:

  .marshal-checking {
      position: absolute; inset: 0;
      background: rgba(10, 10, 10, 0.85);
      display: flex; align-items: center; justify-content: center;
      gap: 8px; color: #ffd700; font-size: 12px; font-family: monospace;
      border-radius: inherit; z-index: 5;
      animation: pulse 1.5s ease-in-out infinite;
  }
  @keyframes pulse { 0%,100% { opacity: 0.7; } 50% { opacity: 1; } }

  .marshal-badge {
      padding: 6px 8px; border-radius: 4px; font-size: 11px;
      font-family: monospace; margin-bottom: 6px;
  }
  .marshal-badge.approved { background: rgba(0, 210, 106, 0.15); border: 1px solid rgba(0, 210, 106, 0.3); color: #00d26a; }
  .marshal-badge.flagged { background: rgba(255, 152, 0, 0.15); border: 1px solid rgba(255, 152, 0, 0.3); color: #ff9800; }

  .marshal-citations {
      margin: 4px 0 0 16px; padding: 0;
      font-size: 10px; color: #ccc; list-style: disc;
  }
  .marshal-citations li { margin: 2px 0; }
  ```

- [ ] **Step 75** [C] ~1m: **COMMIT CHECKPOINT**
  ```bash
  git add kanban/ && git commit -m "feat(kanban): Marshal lane checker on Review column — auto-review on drop"
  ```

- [ ] **Step 76** [W] ~5m: Add review action buttons (Approve / Reject / Changes Requested)
  ```
  File: kanban/js/board-viz.js

  // For cards in REVIEW column, add action buttons
  function addReviewActions(card, taskId) {
      var actions = document.createElement('div');
      actions.className = 'review-actions';
      actions.innerHTML =
          '<button class="review-btn approve" onclick="reviewAction(\'' + taskId + '\', \'approve\')">✓ Approve</button>' +
          '<button class="review-btn reject" onclick="reviewAction(\'' + taskId + '\', \'reject\')">✗ Reject</button>' +
          '<button class="review-btn changes" onclick="reviewAction(\'' + taskId + '\', \'changes\')">↩ Changes</button>';
      card.appendChild(actions);
  }

  function reviewAction(taskId, action) {
      if (action === 'approve') moveTask(taskId, 'done');
      else if (action === 'reject') moveTask(taskId, 'todo');
      else if (action === 'changes') moveTask(taskId, 'in-progress');
  }
  ```

- [ ] **Step 77** [W] ~3m: Style review action buttons
  ```
  File: kanban/css/kanban.css
  Add:

  .review-actions {
      display: flex; gap: 4px; margin-top: 8px; padding-top: 8px;
      border-top: 1px solid rgba(255, 255, 255, 0.06);
  }
  .review-btn {
      flex: 1; padding: 4px 8px; border: 1px solid #333; border-radius: 4px;
      background: transparent; color: #aaa; font-size: 11px; font-family: monospace;
      cursor: pointer; transition: all 0.15s;
  }
  .review-btn.approve:hover { background: rgba(0, 210, 106, 0.2); border-color: #00d26a; color: #00d26a; }
  .review-btn.reject:hover { background: rgba(255, 71, 87, 0.2); border-color: #ff4757; color: #ff4757; }
  .review-btn.changes:hover { background: rgba(255, 152, 0, 0.2); border-color: #ff9800; color: #ff9800; }
  ```

- [ ] **Step 78** [C] ~1m: **COMMIT CHECKPOINT**
  ```bash
  git add kanban/ && git commit -m "feat(kanban): review action buttons — approve/reject/changes requested"
  ```

- [ ] **Step 79** [W] ~5m: Add touch support for mobile drag-drop
  ```
  File: kanban/js/board-viz.js

  // HTML5 drag-drop doesn't work on mobile — add touch events
  // touchstart → record card, add dragging class
  // touchmove → update card position, detect column under finger
  // touchend → drop into column, trigger API + marshal check
  ```

- [ ] **Step 80** [V] ~3m: Test drag-drop flow end-to-end
  ```
  [ ] Drag card from TODO → IN PROGRESS: card moves, count updates
  [ ] Drag card from IN PROGRESS → REVIEW: Marshal check triggers, badge appears
  [ ] Click Approve on REVIEW card: moves to DONE
  [ ] Click Reject: moves back to TODO
  [ ] Click Changes: moves to IN PROGRESS
  [ ] API failure: card reverts to original column, error toast
  ```

- [ ] **Step 81** [C] ~1m: **COMMIT CHECKPOINT**
  ```bash
  git add kanban/ && git commit -m "feat(kanban): touch support, end-to-end drag-drop verified"
  ```

- [ ] **Step 82** [V] ~1m: **PHASE 6 EXIT GATE** — Kanban drag-drop + Marshal complete
  - Cards draggable between all 4 columns
  - Drop into Review triggers Marshal check
  - Marshal badge shows approved/flagged with citations
  - Review action buttons work
  - Touch support added
  - All committed

---

## PHASE 7: KANBAN-APP MARSHAL INTEGRATION (Steps 83-90)

**Goal**: Wire Marshal into the more feature-rich cmd/kanban-app too.
**Prerequisite**: Phase 6 patterns established.
**Time**: 30 minutes
**Agent**: Agent [P with Phase 8]

- [ ] **Step 83** [R] ~2m: Read kanban-app cards.js drag implementation
  ```bash
  grep -n "handleDrop\|dragend\|moveTask\|updateStatus" /sessions/nice-blissful-dirac/mnt/tmp/unheaded/cmd/kanban-app/static/js/cards.js | head -20
  ```

- [ ] **Step 84** [W] ~5m: Add Marshal check hook to kanban-app drop handler
  ```
  File: cmd/kanban-app/static/js/cards.js
  In drop handler, when target column is 'review':

  // After DOM move, before API persist:
  marshalCheck(taskId, taskData);
  ```

- [ ] **Step 85** [W] ~5m: Port Marshal badge rendering to kanban-app
  ```
  File: cmd/kanban-app/static/js/cards.js
  Copy/adapt renderMarshalResult from kanban/js/board-viz.js
  ```

- [ ] **Step 86** [W] ~3m: Add Marshal CSS to kanban-app
  ```
  File: cmd/kanban-app/static/css/cards.css
  Copy marshal-badge, marshal-checking, marshal-citations, review-actions styles
  ```

- [ ] **Step 87** [C] ~1m: **COMMIT CHECKPOINT**
  ```bash
  git add cmd/kanban-app/ && git commit -m "feat(kanban-app): Marshal lane checker integrated into review column"
  ```

- [ ] **Step 88** [W] ~5m: Add websocket-driven Marshal status updates
  ```
  File: cmd/kanban-app/static/js/websocket.js

  // Listen for marshal_result WS messages
  // Update badge in real-time when Marshal finishes async review
  case 'marshal_result':
      Cards.renderMarshalResult(msg.data.task_id, msg.data);
      break;
  ```

- [ ] **Step 89** [C] ~1m: **COMMIT CHECKPOINT**
  ```bash
  git add cmd/kanban-app/ && git commit -m "feat(kanban-app): real-time Marshal updates via WebSocket"
  ```

- [ ] **Step 90** [V] ~2m: **PHASE 7 EXIT GATE** — kanban-app Marshal integrated
  - Drop to review triggers marshal check
  - Badge renders with citations
  - WS updates supported
  - All committed

---

## PHASE 8: CROSS-PAGE CONSISTENCY (Steps 91-100)

**Goal**: Ensure design-system.css variables used consistently, no hardcoded colors, responsive on all pages.
**Prerequisite**: Phases 1-7 complete.
**Time**: 30 minutes
**Agent**: Coordinator

- [ ] **Step 91** [B] ~3m: Audit for hardcoded colors across all modified files
  ```bash
  grep -rn "#[0-9a-fA-F]\{3,6\}" /sessions/nice-blissful-dirac/mnt/tmp/unheaded/cmd/dashboard-backend/static/styles.css /sessions/nice-blissful-dirac/mnt/tmp/unheaded/kanban/css/kanban.css /sessions/nice-blissful-dirac/mnt/tmp/unheaded/dashboard/doom.html 2>/dev/null | grep -v "var(" | head -30
  ```

- [ ] **Step 92** [W] ~5m: Replace hardcoded colors with design-system variables where possible
  ```
  Common replacements:
  #0a0a0a → var(--color-bg)
  #ffd700 → var(--color-accent)
  #4ecdc4 → var(--color-success) or custom --color-flow-tcp
  #ff4757 → var(--color-error)
  #adb5bd → var(--color-text-secondary)
  ```

- [ ] **Step 93** [W] ~5m: Ensure all pages have consistent nav styling
  ```
  Check: dashboard, doom.html, kanban, wiki-server all share nav component
  If not, standardize nav height and styling
  ```

- [ ] **Step 94** [C] ~1m: **COMMIT CHECKPOINT**
  ```bash
  git add -A && git commit -m "style: unify colors to design-system variables across dashboard, kanban, doom"
  ```

- [ ] **Step 95** [W] ~5m: Add responsive breakpoints for dashboard tabs
  ```
  File: cmd/dashboard-backend/static/styles.css
  Add:
  @media (max-width: 768px) {
      .nav-tabs { overflow-x: auto; white-space: nowrap; }
      .latency-charts-grid { grid-template-columns: 1fr; }
      .metrics-grid { grid-template-columns: repeat(2, 1fr); }
  }
  ```

- [ ] **Step 96** [W] ~3m: Verify print/reduced-motion media queries
  ```
  Ensure all animation has prefers-reduced-motion: reduce fallback
  Already present in doom.html — verify dashboard and kanban
  ```

- [ ] **Step 97** [C] ~1m: **COMMIT CHECKPOINT**
  ```bash
  git add -A && git commit -m "style: responsive breakpoints and reduced-motion for dashboard, kanban"
  ```

- [ ] **Step 98** [V] ~3m: Cross-page visual audit
  ```
  [ ] Dashboard: all 4 tabs render, consistent typography
  [ ] Doom: no overlap, nav clear, responsive
  [ ] Kanban: drag works, marshal badges, responsive
  [ ] Wiki: text readable, contrast passes WCAG AA
  ```

- [ ] **Step 99** [C] ~1m: **COMMIT CHECKPOINT**
  ```bash
  git add -A && git commit -m "style: cross-page visual audit complete"
  ```

- [ ] **Step 100** [V] ~1m: **PHASE 8 EXIT GATE** — Design consistency verified
  - No hardcoded colors remaining (or justified exceptions)
  - Responsive at 1920/1366/768/480 widths
  - Reduced-motion supported
  - All committed

---

## PHASE 9: DOCUMENTATION RIPPLE (Steps 101-108)

**Goal**: Update docs to reflect new GUI capabilities.
**Prerequisite**: Phases 1-8 complete.
**Time**: 20 minutes
**Agent**: Agent

- [ ] **Step 101** [W] ~3m: Update CLAUDE.md dashboard section
  ```
  Add: Dashboard tabs now render demo data when backend offline
  Add: Kanban supports drag-drop with Marshal review integration
  Add: Wiki text contrast fixed to WCAG AA
  ```

- [ ] **Step 102** [W] ~3m: Update wiki/Architecture.md with GUI layer details
  ```
  Add: Dashboard demo mode, Flow Graph animation, Latency heatmap
  Add: Marshal lane checking in Kanban review workflow
  ```

- [ ] **Step 103** [W] ~3m: Create wiki/Dashboard-Guide.md
  ```
  Contents:
  - Tab overview (Overview, Flow Graph, Latency, Events)
  - Demo mode (auto-activates, Alt+D toggle)
  - Keyboard shortcuts
  - API endpoints each tab consumes
  ```

- [ ] **Step 104** [W] ~3m: Create wiki/Kanban-Guide.md
  ```
  Contents:
  - Column workflow (TODO → IN PROGRESS → REVIEW → DONE)
  - Drag-and-drop usage
  - Marshal auto-review on Review drop
  - Review action buttons
  - Touch support
  ```

- [ ] **Step 105** [C] ~1m: **COMMIT CHECKPOINT**
  ```bash
  git add CLAUDE.md wiki/ && git commit -m "docs: dashboard guide, kanban guide, architecture update"
  ```

- [ ] **Step 106** [W] ~3m: Update timeline.md with S68 session log
  ```
  Add S68 entry:
  - Dashboard stubs → live visualization (demo + real data)
  - doom.html CSS overlap fix
  - Kanban drag-drop + Marshal lane checker
  - Wiki WCAG AA contrast fix
  ```

- [ ] **Step 107** [C] ~1m: **COMMIT CHECKPOINT**
  ```bash
  git add references/timeline.md && git commit -m "docs: S68 session log in timeline"
  ```

- [ ] **Step 108** [V] ~1m: **PHASE 9 EXIT GATE** — Docs updated
  - CLAUDE.md reflects GUI changes
  - Wiki guides exist
  - Timeline updated

---

## PHASE 10: FINAL VERIFICATION & HANDOFF (Steps 109-124)

**Goal**: Full verification pass across all 4 fix areas. Write handoff.
**Prerequisite**: All prior phases complete.
**Time**: 30 minutes
**Agent**: Coordinator

### Dashboard Verification

- [ ] **Step 109** [V] ~2m: Overview tab renders with demo data
- [ ] **Step 110** [V] ~2m: Flow Graph tab shows animated topology
- [ ] **Step 111** [V] ~2m: Latency tab shows histograms + sparkline
- [ ] **Step 112** [V] ~2m: Events tab streams with colored type badges

### Doom Verification

- [ ] **Step 113** [V] ~2m: doom.html — no text overlap at 1920x1080
- [ ] **Step 114** [V] ~2m: doom.html — no text overlap at 768x1024 (tablet)
- [ ] **Step 115** [V] ~1m: doom.html — overlay stays within canvas bounds

### Kanban Verification

- [ ] **Step 116** [V] ~2m: Drag card TODO → IN PROGRESS works
- [ ] **Step 117** [V] ~2m: Drag card → REVIEW triggers Marshal check
- [ ] **Step 118** [V] ~2m: Marshal badge appears with citations
- [ ] **Step 119** [V] ~2m: Review buttons (Approve/Reject/Changes) work
- [ ] **Step 120** [V] ~1m: Column counts update after drag

### Wiki Verification

- [ ] **Step 121** [V] ~1m: Wiki --text-secondary is #999 or brighter
- [ ] **Step 122** [V] ~1m: Wiki body text readable on dark background

### Handoff

- [ ] **Step 123** [W] ~5m: Write handoff document
  ```
  File: references/S68-HANDOFF.md
  Contents:
  - What was accomplished (4 fix areas)
  - Dashboard: demo data generator, all tabs functional
  - Doom: CSS overlap resolved
  - Kanban: drag-drop + Marshal lane checking
  - Wiki: WCAG AA contrast
  - Known issues / remaining work
  - Next steps
  ```

- [ ] **Step 124** [V] ~2m: **SPRINT EXIT GATE — S68 COMPLETE**
  ```bash
  git add -A && git commit -m "[PLAN S68] Sprint complete: Dashboard live viz, Doom CSS fix, Kanban drag+Marshal, Wiki contrast"
  ```

  Final checklist:
  ```
  [ ] Dashboard 4 tabs render data (demo or live)
  [ ] doom.html no text overlap
  [ ] Kanban drag-drop functional with Marshal
  [ ] Wiki text WCAG AA compliant
  [ ] All docs updated
  [ ] All committed
  ```

---

## APPENDIX A: EMERGENCY PROCEDURES

### E1: Canvas Rendering Blank
- **Symptom**: Flow graph or latency canvas shows nothing
- **Check**: Browser console for JS errors
- **Fix**: Verify canvas element ID matches JS getElementById
- **Fix**: Verify canvas.getContext('2d') not returning null (offscreen tab issue)
- **Workaround**: Force render on tab switch with requestAnimationFrame

### E2: Drag-Drop Not Working
- **Symptom**: Cards won't drag or won't drop
- **Check**: draggable="true" attribute present on cards
- **Check**: e.preventDefault() in dragover handler (REQUIRED for drop to fire)
- **Check**: dataTransfer.setData called in dragstart
- **Fix**: Most common issue is missing preventDefault in dragover

### E3: Marshal API 404
- **Symptom**: POST /api/v1/marshal/check returns 404
- **Fix**: Marshal endpoint may not exist yet — add graceful fallback
- **Fallback**: Show "Marshal offline — manual review" badge instead of error

### E4: Wiki Server Won't Compile
- **Symptom**: go build fails after CSS changes
- **Check**: Unterminated string in embedded CSS template
- **Fix**: Ensure all backticks and quotes properly escaped in Go string literals

---

## APPENDIX B: AGENT ASSIGNMENT MATRIX

| Phase | Agent | Parallel? | Depends On | Est. Time |
|-------|-------|-----------|------------|-----------|
| 0: Intelligence | Coordinator | No | — | 15m |
| 1: Wiki Fix | Coordinator | No | P0 | 15m |
| 2: Doom CSS | Coordinator | Yes (with P3) | P0 | 30m |
| 3: Demo Data | Agent | Yes (with P2) | P0 | 90m |
| 4: Flow Graph | Agent | Yes (with P5) | P3 | 45m |
| 5: Latency+Events | Agent | Yes (with P4) | P3 | 45m |
| 6: Kanban DnD | Agent | Yes (with P4/5) | P0 | 60m |
| 7: Kanban-App Marshal | Agent | Yes (with P8) | P6 | 30m |
| 8: Consistency | Coordinator | No | P1-7 | 30m |
| 9: Docs | Agent | No | P8 | 20m |
| 10: Final Gate | Coordinator | No | ALL | 30m |

**Critical Path**: P0 → P3 → P4/P5 → P8 → P10 = 15 + 90 + 45 + 30 + 30 = **210 minutes (~3.5 hours)**
**With parallelization**: P1+P2+P6 run alongside P3. P4+P5 parallel. P7 overlaps P8. = **~6 hours wall clock**

---

## APPENDIX C: QUICK REFERENCE

### File Locations
```
Dashboard (backend):  cmd/dashboard-backend/static/
Dashboard (frontend): dashboard/
Doom viewer:          dashboard/doom.html
Kanban (basic):       kanban/
Kanban (full):        cmd/kanban-app/static/
Wiki server:          cmd/wiki-server/main.go
Design system:        dashboard/css/design-system.css
```

### Color Variables (design-system.css)
```
--color-bg:           #0a0a0a
--color-bg-secondary: #111
--color-bg-tertiary:  #161616
--color-text-primary: #f8f9fa
--color-text-secondary: #adb5bd
--color-accent:       #ffd700
--color-success:      #00d26a
--color-error:        #ff4757
--color-warning:      #ff9800
--color-border:       #1e2746
```

### Wiki Color Fix Reference
```
BEFORE: --text-secondary: #666666 (ratio 3.5:1 — FAILS WCAG AA)
AFTER:  --text-secondary: #999999 (ratio 6.3:1 — PASSES WCAG AA)
```

### Marshal API Shape
```json
POST /api/v1/marshal/check
{
  "task_id": "string",
  "title": "string",
  "description": "string",
  "from_status": "in_progress",
  "to_status": "review"
}

Response:
{
  "approved": true|false,
  "citations": ["scope creep detected", "missing exit gate"],
  "recommendations": ["add verification step", "split task"]
}
```

---

*S68 Battle Plan — Forged 2026-02-27*
*10 Phases. 124 Steps. From stub to ship.*
*The dashboard sees everything. The kanban tracks itself. The Marshal never sleeps.*
