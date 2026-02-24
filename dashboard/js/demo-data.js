/* ============================================================================
   UNHEADED DEMO DATA GENERATOR
   Generates realistic trace data when no backend is running
   ============================================================================ */

(function() {
  'use strict';

  // Helper functions
  function generateHexId() {
    return '0x' + Math.random().toString(16).substring(2, 8);
  }

  function randomChoice(arr) {
    return arr[Math.floor(Math.random() * arr.length)];
  }

  function randomBetween(min, max) {
    return Math.floor(Math.random() * (max - min + 1)) + min;
  }

  // Data generation functions
  function generateFlowLabel() {
    return generateHexId();
  }

  function generateTrace() {
    const types = ['INGRESS', 'EGRESS', 'XDP_DROP', 'TC_REDIRECT', 'ANAMNESIS'];
    const modes = ['IDLE', 'ACTIVE', 'CLOSING', 'CLOSED'];
    const latencies = [
      '47µs', '58µs', '72µs', '94µs', '112µs',
      '156µs', '203µs', '287µs', '412µs', '601µs',
      '841µs', '1.2ms', '1.6ms', '1.8ms', '2.1ms'
    ];

    return {
      flowLabel: generateFlowLabel(),
      timestamp: new Date(Date.now() - Math.random() * 300000).toISOString(),
      type: randomChoice(types),
      kingdomMode: randomChoice(modes),
      latency: randomChoice(latencies)
    };
  }

  function generateMetrics() {
    const latencySamples = [
      47, 58, 72, 94, 112, 156, 203, 287, 412, 601,
      841, 1200, 1600, 1800, 2100
    ];
    latencySamples.sort((a, b) => a - b);

    const p50idx = Math.floor(latencySamples.length * 0.5);
    const p95idx = Math.floor(latencySamples.length * 0.95);
    const p99idx = Math.floor(latencySamples.length * 0.99);

    return {
      activeFlows: randomBetween(5, 25),
      packetRate: randomBetween(1000, 15000),
      p50: latencySamples[p50idx],
      p95: latencySamples[p95idx],
      p99: latencySamples[p99idx],
      max: latencySamples[latencySamples.length - 1]
    };
  }

  function generateServices() {
    const serviceNames = [
      'api-gateway', 'auth-service', 'cache-layer',
      'database-primary', 'database-replica', 'logging-pipeline',
      'metrics-aggregator', 'trace-collector', 'config-server',
      'message-broker', 'load-balancer', 'dns-resolver'
    ];

    const protocols = ['HTTP', 'gRPC', 'TCP', 'UDP', 'WebSocket'];
    const tiers = ['CRITICAL', 'HIGH', 'MEDIUM', 'LOW'];
    const health = ['healthy', 'degraded', 'warning'];

    return serviceNames.map(name => ({
      name: name,
      port: randomBetween(8000, 9999),
      protocol: randomChoice(protocols),
      health: randomChoice(health),
      tier: randomChoice(tiers)
    }));
  }

  // Generate initial data
  const traces = [];
  for (let i = 0; i < 10; i++) {
    traces.push(generateTrace());
  }

  const metrics = generateMetrics();
  const services = generateServices();

  // Export to window
  window.DemoData = {
    generateFlowLabel,
    generateTrace,
    generateMetrics,
    generateServices,
    traces,
    flows: Array.from({length: 12}, (_, i) => ({
      id: i,
      label: generateFlowLabel(),
      status: randomChoice(['ACTIVE', 'IDLE', 'CLOSING'])
    })),
    metrics,
    services
  };

  console.log('[Demo] Data initialized:', window.DemoData);

})();
