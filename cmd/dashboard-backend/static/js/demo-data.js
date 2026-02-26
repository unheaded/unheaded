// dashboard/js/demo-data.js
// UNHEADED — Demo data generator for offline/development mode
// Provides realistic telemetry data when backend is unreachable
// "The silence between packets is also data." — The Scientist

const DemoData = (() => {
  'use strict';

  // ── Utilities ──────────────────────────────────────────────────────
  const rand    = (min, max) => Math.floor(Math.random() * (max - min + 1)) + min;
  const randF   = (min, max, decimals = 2) => parseFloat((Math.random() * (max - min) + min).toFixed(decimals));
  const pick    = arr => arr[rand(0, arr.length - 1)];
  const ts      = (offsetMs = 0) => new Date(Date.now() - offsetMs).toISOString();
  const hexId   = (bytes = 8) => Array.from({length: bytes}, () => rand(0, 255).toString(16).padStart(2,'0')).join('');

  const SERVICES = ['wotan', 'captain', 'timeguru', 'architect', 'micromanager', 'monad', 'sophia', 'dashboard', 'gateway', 'trace-collector'];
  const STATUS_CODES = [0, 0, 0, 0, 0, 2, 4, 13]; // mostly OK
  const GRPC_NAMES = ['OK','CANCELLED','UNKNOWN','INVALID_ARGUMENT','NOT_FOUND','ALREADY_EXISTS','PERMISSION_DENIED','RESOURCE_EXHAUSTED','FAILED_PRECONDITION','ABORTED','OUT_OF_RANGE','UNIMPLEMENTED','INTERNAL','UNAVAILABLE','DATA_LOSS','UNAUTHENTICATED'];
  const FLOW_TYPES = ['INGRESS', 'EGRESS', 'XDP_DROP', 'TC_REDIRECT', 'ANAMNESIS'];
  const KINGDOM_MODES = ['IDLE', 'ACTIVE', 'CLOSING', 'CLOSED'];

  // ── Packet Flow Tab ─────────────────────────────────────────────────
  function packetFlowEvents(count = 20) {
    return Array.from({length: count}, (_, i) => ({
      traceId:   hexId(16),
      spanId:    hexId(8),
      timestamp: ts(i * rand(50, 300)),
      srcService: pick(SERVICES),
      dstService: pick(SERVICES),
      protocol:  pick(['gRPC', 'HTTP/3', 'HTTP/2', 'QUIC']),
      bytes:     rand(64, 8192),
      latencyUs: rand(100, 25000),
      hopCount:  rand(1, 6),
      status:    pick(STATUS_CODES),
      flowLabel: '0x' + rand(0, 0xFFFFF).toString(16).padStart(5, '0'),
    }));
  }

  // ── Trace Table Tab ─────────────────────────────────────────────────
  function traceTableRows(count = 50) {
    return Array.from({length: count}, (_, i) => ({
      flowLabel: '0x' + rand(0, 0xFFFFF).toString(16).padStart(5, '0'),
      timestamp: ts(i * rand(100, 2000)),
      type: pick(FLOW_TYPES),
      kingdomMode: pick(KINGDOM_MODES),
      latency: rand(10, 500) + 'µs',
      action: pick(['inspect', 'drop', 'trace']),
    }));
  }

  // ── Latency Chart Tab ───────────────────────────────────────────────
  function latencyTimeSeries(points = 60) {
    let p50 = 800, p95 = 3200, p99 = 8500;
    return Array.from({length: points}, (_, i) => {
      // Simulate realistic drift + occasional spikes
      p50 += rand(-80, 100);
      p95 += rand(-150, 200);
      p99 += rand(-300, 500);
      if (Math.random() > 0.95) p99 += rand(5000, 20000); // occasional spike
      p50 = Math.max(100, Math.min(p50, 5000));
      p95 = Math.max(p50 * 2, Math.min(p95, 15000));
      p99 = Math.max(p95 * 1.5, Math.min(p99, 60000));
      return {
        timestamp: ts((points - i) * 5000),
        p50Us: Math.round(p50),
        p95Us: Math.round(p95),
        p99Us: Math.round(p99),
        rps: rand(80, 450),
      };
    });
  }

  // ── Services Tab ────────────────────────────────────────────────────
  const PORT_MAP = {
    'wotan': 18000, 'captain': 19002, 'timeguru': 19000,
    'architect': 19001, 'micromanager': 19003, 'monad': 19004,
    'sophia': 19005, 'dashboard': 20000, 'gateway': 21000, 'trace-collector': 16670
  };
  function servicesStatus() {
    return SERVICES.map(name => ({
      name,
      port: PORT_MAP[name] || 19999,
      protocol: name === 'wotan' ? 'gRPC+HTTP' : pick(['gRPC', 'HTTP']),
      status:   Math.random() > 0.1 ? 'healthy' : pick(['degraded', 'starting']),
      health:   Math.random() > 0.1 ? 'healthy' : pick(['degraded', 'starting']),
      uptime:   `${rand(1, 99)}d ${rand(0, 23)}h ${rand(0, 59)}m`,
      rps:      rand(5, 200),
      p99Ms:    randF(0.5, 25),
      version:  '0.1.0',
      tier:     name === 'wotan' ? 'message-bus' : name === 'gateway' ? 'gateway' : 'core-service',
    }));
  }

  // ── Infrastructure Tab ──────────────────────────────────────────────
  function infraStatus() {
    return {
      runtimes: [
        { name: 'Docker', status: 'running', containers: rand(8, 15), cpuPct: randF(5, 35), memGb: randF(0.5, 4) },
        { name: 'containerd', status: 'running', containers: rand(2, 6), cpuPct: randF(1, 15), memGb: randF(0.2, 2) },
        { name: 'LXD', status: pick(['running', 'stopped']), containers: rand(0, 4), cpuPct: randF(0, 10), memGb: randF(0, 1) },
        { name: 'NixOS', status: 'active', containers: rand(5, 10), cpuPct: randF(2, 20), memGb: randF(0.3, 3) },
      ],
      host: {
        cpuCores: 4, cpuPct: randF(10, 60),
        memTotalGb: 16, memUsedGb: randF(2, 12),
        diskTotalGb: 500, diskUsedGb: randF(50, 300),
        kernel: '6.1.0-nixos', uptime: `${rand(1,30)}d`,
      }
    };
  }

  // ── Logs Tab ────────────────────────────────────────────────────────
  const LOG_LEVELS = ['debug','debug','info','info','info','info','warn','error'];
  const LOG_MSGS = [
    'gRPC connection established to wotan:18001',
    'service registered: {name} port {port}',
    'health check passed',
    'Wotan topic published: logs.{svc}.info',
    'packet trace captured: traceId={id}',
    'BPF map read: 0 entries (cold start)',
    'rate limit: 0/1000 requests this second',
    'service discovery scan complete: 10 services found',
    'WebSocket client connected from 127.0.0.1',
    'gRPC stream reconnected after 1.2s',
    'config file watcher: no changes detected',
    'JWT validation skipped: noop authenticator active',
  ];
  function logLines(count = 100) {
    return Array.from({length: count}, (_, i) => {
      const svc = pick(SERVICES);
      const lvl = pick(LOG_LEVELS);
      const msg = pick(LOG_MSGS)
        .replace('{name}', svc)
        .replace('{port}', PORT_MAP[svc])
        .replace('{svc}', svc)
        .replace('{id}', hexId(8));
      return { timestamp: ts(i * rand(50, 5000)), service: svc, level: lvl, message: msg };
    });
  }

  // ── AI Inference Tab ────────────────────────────────────────────────
  function aiMetrics() {
    return {
      models: [
        {
          name: 'deepseek-r1-7b',
          status: 'loading',
          port: 20100,
          vramGb: 0, vramMaxGb: 12,
          tokensPerSec: 0,
          p50Ms: 0, p99Ms: 0,
          requestsTotal: 0,
          note: 'Awaiting ROCm GPU (RX 7700 XT) — bare metal pending',
        },
        {
          name: 'qwen-2.5-coder-7b',
          status: 'loading',
          port: 20101,
          vramGb: 0, vramMaxGb: 12,
          tokensPerSec: 0,
          p50Ms: 0, p99Ms: 0,
          requestsTotal: 0,
          note: 'Awaiting ROCm GPU (RX 7700 XT) — bare metal pending',
        },
      ],
      vector: { name: 'qdrant', status: 'loading', port: 20102, collectionsCount: 0 },
      embedder: { name: 'bge-m3', status: 'loading', port: 20104 },
    };
  }

  // ── DOOM Tab ────────────────────────────────────────────────────────
  function doomStatus() {
    return {
      status: 'awaiting-bare-metal',
      note: 'DOOM via 6-namespace packet ring pending bare metal deployment',
      lastFrame: null,
      fps: 0,
      instructions: 0,
      hopCount: 0,
      packetRingStatus: 'offline',
    };
  }

  // ── Top-level metrics (header cards) ────────────────────────────────
  function headerMetrics() {
    return {
      packetsTracedTotal: rand(1_000_000, 50_000_000),
      servicesHealthy: rand(8, 10),
      servicesTotal: 10,
      p99LatencyMs: randF(1, 25),
      activeFlows: rand(100, 5000),
      wotanTopics: rand(20, 80),
      logsPerMinute: rand(500, 5000),
      bpfPrograms: 4,
    };
  }

  // ── Legacy interface (backward compat for existing code) ────────────
  function generateTrace() {
    return {
      flowLabel: '0x' + rand(0, 0xFFFFF).toString(16).padStart(5, '0'),
      timestamp: ts(rand(0, 300000)),
      type: pick(FLOW_TYPES),
      kingdomMode: pick(KINGDOM_MODES),
      latency: rand(10, 500) + 'µs'
    };
  }

  function generateMetrics() {
    const samples = [47, 58, 72, 94, 112, 156, 203, 287, 412, 601, 841, 1200, 1600, 1800, 2100];
    return {
      activeFlows: rand(5, 25),
      packetRate: rand(1000, 15000),
      p50: samples[Math.floor(samples.length * 0.5)],
      p95: samples[Math.floor(samples.length * 0.95)],
      p99: samples[Math.floor(samples.length * 0.99)],
      max: samples[samples.length - 1]
    };
  }

  // Generate initial data
  const traces = Array.from({length: 10}, generateTrace);
  const metrics = generateMetrics();
  const services = servicesStatus();

  // Polling simulation: mutate data slightly
  function tick(data) {
    if (data.packetsTracedTotal !== undefined) {
      data.packetsTracedTotal += rand(50, 500);
      data.activeFlows += rand(-5, 8);
      data.activeFlows = Math.max(0, data.activeFlows);
    }
    return data;
  }

  return {
    // New comprehensive functions
    packetFlowEvents,
    traceTableRows,
    latencyTimeSeries,
    servicesStatus,
    infraStatus,
    logLines,
    aiMetrics,
    doomStatus,
    headerMetrics,

    // Legacy interface
    generateTrace,
    generateMetrics,
    generateServices: servicesStatus,
    traces,
    metrics,
    services,
    flows: Array.from({length: 12}, (_, i) => ({
      id: i,
      label: '0x' + rand(0, 0xFFFFF).toString(16).padStart(5, '0'),
      status: pick(['ACTIVE', 'IDLE', 'CLOSING'])
    })),

    tick,
    isDemoMode: false,
    activate() { this.isDemoMode = true; console.warn('[DEMO MODE] Backend unreachable — using generated data'); },
  };
})();

// Auto-detect demo mode: if /health returns non-200, activate demo data
(async () => {
  try {
    const r = await fetch('/health', { signal: AbortSignal.timeout(2000) });
    if (!r.ok) DemoData.activate();
  } catch {
    DemoData.activate();
  }
})();
