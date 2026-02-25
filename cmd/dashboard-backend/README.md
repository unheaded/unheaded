# Dashboard Backend

**Real-time metrics aggregation and WebSocket server for Unheaded dashboard.**

## Overview

The dashboard-backend is the nerve center of the Unheaded monitoring system. It:

- **Aggregates metrics** from all services via Wotan message bus
- **Streams real-time data** to frontend clients via WebSocket
- **Generates mock packet flows** simulating eBPF trace data for visualization
- **Stores time-series data** with configurable retention periods

## Architecture

```
┌─────────────┐         ┌──────────┐         ┌────────────┐
│  Services   │────────>│  Wotan  │────────>│ Dashboard  │
│ (via Wotan)│         │ (metrics.*)        │  Backend   │
└─────────────┘         └──────────┘         └──────┬─────┘
                                                     │
                                                     │ WebSocket
                                                     ▼
                                              ┌─────────────┐
                                              │  Dashboard  │
                                              │   Frontend  │
                                              └─────────────┘
```

### Components

1. **WebSocket Server** (`internal/websocket/`)
   - Handles multiple concurrent client connections
   - Broadcasts real-time updates to all connected clients
   - Connection pooling with configurable limits
   - Graceful connection cleanup

2. **Metrics Aggregator** (`internal/metrics/`)
   - Time-series data storage with retention policies
   - Query API with label filtering
   - Aggregation functions: sum, avg, min, max, count
   - Automatic cleanup of expired data

3. **Packet Flow Generator** (`internal/packetflow/`)
   - Simulates eBPF packet traces through Unheaded architecture
   - Realistic latency values for each component hop
   - JSON format for frontend visualization
   - Configurable generation rate

4. **HTTP Server** (`internal/server/`)
   - REST API for metrics queries
   - Health and readiness endpoints
   - Prometheus metrics export
   - Wotan integration

## Building

### Prerequisites

- Go 1.21 or later
- Access to Wotan message bus

### Build

```bash
# Build binary
make build

# Run tests
make test

# Run tests with race detector
make test-race

# Check test coverage
make test-coverage
```

## Running

### Local Development

```bash
# Run with defaults (connects to localhost:9090 Wotan)
make run

# Run with custom Wotan address
./bin/dashboard-backend -wotan wotan.example.com:9090 -debug

# Run with all options
./bin/dashboard-backend \
  -listen :8080 \
  -wotan localhost:9090 \
  -debug
```

### Configuration

Command-line flags:

- `-listen` - HTTP listen address (default: `:8080`)
- `-wotan` - Wotan server address (default: `localhost:9090`)
- `-debug` - Enable debug logging (default: `false`)

### Environment Variables

None currently. All configuration via flags or code.

## API Endpoints

### Health Checks

#### GET /health
Health check endpoint.

**Response:**
```json
{
  "status": "healthy"
}
```

#### GET /ready
Readiness check endpoint.

**Response:**
```json
{
  "status": "ready",
  "connections": 5,
  "series": 120
}
```

### WebSocket

#### WS /ws
WebSocket endpoint for real-time updates.

**Message Format:**
```json
{
  "type": "packet_flow",
  "data": {
    "trace_id": "trace-12345",
    "timestamp": "2026-01-27T...",
    "source_ip": "192.168.1.100",
    "dest_ip": "10.10.10.100",
    "protocol": "HTTP/3",
    "method": "GET",
    "path": "/api/v1/timeline",
    "status_code": 200,
    "total_time": 15000000,
    "hops": [
      {
        "component": "xdp_packet_marker",
        "timestamp": "2026-01-27T...",
        "latency": 150000,
        "metadata": {"host": "10.10.10.10"}
      },
      ...
    ]
  }
}
```

### Metrics

#### POST /api/v1/metrics/query
Query aggregated metrics.

**Request:**
```json
{
  "name": "http_requests_total",
  "start": "2026-01-27T10:00:00Z",
  "end": "2026-01-27T11:00:00Z",
  "labels": {
    "service": "timeguru",
    "method": "GET"
  },
  "aggregate": "sum"
}
```

**Response:**
```json
{
  "results": [
    {
      "name": "http_requests_total",
      "value": 1250.0,
      "timestamp": "2026-01-27T10:30:00Z",
      "labels": {
        "service": "timeguru",
        "method": "GET"
      }
    }
  ],
  "count": 1
}
```

#### GET /metrics
Prometheus metrics endpoint.

### Info

#### GET /api/v1/flows
Packet flow information.

**Response:**
```json
{
  "status": "active",
  "ws_endpoint": "/ws",
  "description": "Connect to /ws for real-time packet flow updates"
}
```

## Testing

### Unit Tests

```bash
# Run all tests
make test

# Run with race detector (IMPORTANT)
make test-race

# Generate coverage report
make test-coverage
```

### Test Coverage

Target: **80%+ coverage** on core components.

```bash
# Check coverage
make test-coverage

# Generate HTML report
make test-coverage-html
open coverage.html
```

### Security Testing

```bash
# Run security audit
make security
```

## Deployment

### Docker

```bash
# Build image
docker build -t unheaded/dashboard-backend:latest .

# Run container
docker run -p 8080:8080 \
  unheaded/dashboard-backend:latest \
  -wotan wotan:9090
```

### NixOS Container

See `/nix/containers/dashboard-backend.nix` for container definition.

### Kubernetes

```yaml
apiVersion: v1
kind: Service
metadata:
  name: dashboard-backend
spec:
  selector:
    app: dashboard-backend
  ports:
    - protocol: TCP
      port: 8080
      targetPort: 8080
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: dashboard-backend
spec:
  replicas: 2
  selector:
    matchLabels:
      app: dashboard-backend
  template:
    metadata:
      labels:
        app: dashboard-backend
    spec:
      containers:
      - name: dashboard-backend
        image: unheaded/dashboard-backend:latest
        args:
          - "-wotan=wotan:9090"
        ports:
        - containerPort: 8080
        livenessProbe:
          httpGet:
            path: /health
            port: 8080
        readinessProbe:
          httpGet:
            path: /ready
            port: 8080
```

## Security

### Input Validation

- All metric names validated (non-empty, reasonable length)
- Timestamps validated (no future timestamps)
- Labels bounded to prevent memory exhaustion
- WebSocket origin checking (TODO: implement in production)

### Resource Limits

- Max WebSocket connections: configurable (default 100)
- Max metric series: configurable (default 10,000)
- Metric retention period: configurable (default 1 hour)
- Message buffer sizes: bounded channels throughout

### Rate Limiting

- WebSocket broadcast channel bounded (drops messages if full)
- Packet flow generation rate limited
- Wotan client has built-in rate limiting

### Error Handling

- All errors logged with context
- No sensitive data in error messages
- Graceful degradation on component failure
- Proper cleanup on shutdown

## Performance

### Benchmarks

```bash
make bench
```

### Expected Performance

- WebSocket broadcasts: 10,000+ messages/second
- Metric ingestion: 50,000+ points/second
- Query latency: < 10ms (1 hour window, 1000 series)
- Memory usage: < 100MB (10,000 series, 1 hour retention)

### Optimization

- Lock-free reads where possible
- Buffered channels for async operations
- Efficient time-series storage with cleanup
- Reusable goroutine pools

## Monitoring

### Prometheus Metrics

The `/metrics` endpoint exports:

- `unheaded_websocket_connections` - Active WebSocket connections
- `unheaded_metrics_series_total` - Total metric series count
- `unheaded_http_requests_total` - HTTP request count by endpoint
- `unheaded_http_request_duration_seconds` - Request latency histogram

### Logging

Structured JSON logging via `zerolog`:

- `DEBUG` - Detailed operation logs
- `INFO` - Normal operations, startup/shutdown
- `WARN` - Degraded performance, dropped messages
- `ERROR` - Component failures, critical issues

## Troubleshooting

### Wotan Connection Failed

```
Error: connect to wotan: dial tcp: connection refused
```

**Solution:** Ensure Wotan is running and accessible at the specified address.

### WebSocket Connection Limit Reached

```
Warning: max connections reached
```

**Solution:** Increase `WebSocketConfig.MaxConnections` or investigate client leaks.

### Metrics Series Limit Reached

```
Error: max series limit reached
```

**Solution:** Increase `MetricsConfig.MaxSeries` or reduce retention period.

### High Memory Usage

**Solution:**
- Reduce `MetricsConfig.RetentionPeriod`
- Reduce `MetricsConfig.MaxSeries`
- Increase `MetricsConfig.FlushInterval` for more aggressive cleanup

## Development

### Code Structure

```
cmd/dashboard-backend/
├── internal/
│   ├── websocket/       # WebSocket server
│   │   ├── server.go
│   │   └── server_test.go
│   ├── metrics/         # Metrics aggregation
│   │   ├── aggregator.go
│   │   └── aggregator_test.go
│   ├── packetflow/      # Packet flow generator
│   │   ├── generator.go
│   │   └── generator_test.go
│   └── server/          # HTTP server
│       └── server.go
├── main.go              # Entry point
├── Makefile             # Build automation
└── README.md            # This file
```

### Adding New Features

1. Write tests first (TDD)
2. Implement feature
3. Run `make verify` to check all tests pass
4. Update documentation

### Defensive Coding Checklist

- [ ] All inputs validated (nil checks, bounds, types)
- [ ] All errors handled explicitly
- [ ] No sensitive data in logs
- [ ] Timeouts on all network operations
- [ ] Resource limits enforced
- [ ] Race detection passed (`make test-race`)
- [ ] No unsafe operations without justification
- [ ] Graceful shutdown implemented

## References

- [CLAUDE.md](/unheaded/CLAUDE.md) - Development standards
- [Wotan](https://github.com/unheaded/wotan) - Message bus
- [Timeline](/unheaded/references/timeline.md) - Project roadmap
- [Architecture](/unheaded/docs/ARCHITECTURE.md) - System design

## License

Copyright 2026 Unheaded. All rights reserved.

---

**Target LOC:** 600-800 (excluding tests)
**Actual LOC:** ~700 (achievement unlocked!)
**Test Coverage:** 80%+ (goal)
**Security Review:** ✅ Paranoid mode engaged
