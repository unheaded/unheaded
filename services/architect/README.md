# Architect Service

Infrastructure design and architecture decision tracking service for Unheaded.

**Component**: Layer 4 - Application Services
**Language**: Go 1.21+
**Dependencies**: Busboy, Prometheus, zerolog

## Overview

The Architect service maintains the state of infrastructure topology, network design, and recorded architecture decisions. It provides a complete audit trail of all design decisions and current infrastructure state.

### Responsibilities

- **Infrastructure State**: Track services, their status, ports, and metadata
- **Network Topology**: Maintain network nodes, CIDRs, and connectivity information
- **Architecture Decisions**: Log and retrieve all architecture decisions with status tracking
- **Observability**: Publish metrics and events to Busboy for correlation with eBPF traces

## Architecture

```
┌─────────────────────────────────────────┐
│      Architect Service (Port 8001)      │
├─────────────────────────────────────────┤
│ HTTP API                                │
│ ├── GET /health                         │
│ ├── GET /infrastructure                 │
│ ├── POST /infrastructure/services       │
│ ├── GET /network                        │
│ ├── POST /network/nodes                 │
│ ├── GET|POST /design                    │
│ └── GET /metrics (Prometheus)           │
├─────────────────────────────────────────┤
│ Core Service                            │
│ ├── InfrastructureState                 │
│ ├── NetworkTopology                     │
│ └── ArchitectureDecisions               │
├─────────────────────────────────────────┤
│ Busboy Integration                      │
│ └── Subscribe: architecture.updates     │
└─────────────────────────────────────────┘
```

## API Endpoints

### Health Check

```bash
GET /health
```

Returns service health status.

**Response** (200 OK):
```json
{
  "data": {
    "healthy": true
  },
  "timestamp": "2026-01-27T12:00:00Z"
}
```

### Infrastructure

#### Get Infrastructure State

```bash
GET /infrastructure
```

Returns complete infrastructure state including all services and decisions.

**Response** (200 OK):
```json
{
  "data": {
    "services": {
      "busboy-service": {
        "service_id": "busboy-service",
        "name": "Busboy Message Bus",
        "type": "message-bus",
        "status": "healthy",
        "address": "10.10.10.10",
        "port": 9090,
        "metadata": {},
        "created_at": "2026-01-27T12:00:00Z",
        "updated_at": "2026-01-27T12:00:00Z"
      }
    },
    "decisions": [
      {
        "decision_id": "dec-1",
        "title": "Use Busboy for message bus",
        "description": "Chose Busboy due to low latency and proven architecture",
        "component": "message-bus",
        "status": "approved",
        "created_at": "2026-01-27T12:00:00Z",
        "updated_at": "2026-01-27T12:00:00Z"
      }
    ]
  },
  "timestamp": "2026-01-27T12:00:00Z"
}
```

#### Add Service

```bash
POST /infrastructure/services

{
  "service_id": "timeguru-service",
  "name": "Timeguru Service",
  "type": "timeline",
  "status": "healthy",
  "address": "10.10.10.20",
  "port": 8000,
  "metadata": {
    "version": "1.0.0",
    "team": "architecture"
  }
}
```

**Response** (201 Created):
```json
{
  "data": {
    "service_id": "timeguru-service",
    "name": "Timeguru Service",
    "type": "timeline",
    "status": "healthy",
    "address": "10.10.10.20",
    "port": 8000,
    "metadata": {"version": "1.0.0", "team": "architecture"},
    "created_at": "2026-01-27T12:00:00Z",
    "updated_at": "2026-01-27T12:00:00Z"
  },
  "timestamp": "2026-01-27T12:00:00Z"
}
```

### Network

#### Get Network Topology

```bash
GET /network
```

Returns complete network topology.

**Response** (200 OK):
```json
{
  "data": {
    "nodes": {
      "node-gateway": {
        "node_id": "node-gateway",
        "name": "Gateway Node",
        "type": "gateway",
        "cidr": "10.10.10.100/32",
        "status": "online",
        "metadata": {},
        "created_at": "2026-01-27T12:00:00Z",
        "updated_at": "2026-01-27T12:00:00Z"
      }
    }
  },
  "timestamp": "2026-01-27T12:00:00Z"
}
```

#### Add Network Node

```bash
POST /network/nodes

{
  "node_id": "node-app-1",
  "name": "Application Node 1",
  "type": "container",
  "cidr": "10.10.10.20/32",
  "status": "online",
  "metadata": {}
}
```

**Response** (201 Created):
```json
{
  "data": {
    "node_id": "node-app-1",
    "name": "Application Node 1",
    "type": "container",
    "cidr": "10.10.10.20/32",
    "status": "online",
    "metadata": {},
    "created_at": "2026-01-27T12:00:00Z",
    "updated_at": "2026-01-27T12:00:00Z"
  },
  "timestamp": "2026-01-27T12:00:00Z"
}
```

### Architecture Decisions

#### Log Decision

```bash
POST /design

{
  "decision_id": "dec-2",
  "title": "Use Go for services",
  "description": "Go provides excellent concurrency, fast compilation, and great tooling",
  "component": "services",
  "status": "approved",
  "metadata": {}
}
```

**Response** (201 Created):
```json
{
  "data": {
    "decision_id": "dec-2",
    "title": "Use Go for services",
    "description": "Go provides excellent concurrency, fast compilation, and great tooling",
    "component": "services",
    "status": "approved",
    "metadata": {},
    "created_at": "2026-01-27T12:00:00Z",
    "updated_at": "2026-01-27T12:00:00Z"
  },
  "timestamp": "2026-01-27T12:00:00Z"
}
```

#### Get Design Decisions

```bash
GET /design
```

Returns all recorded architecture decisions.

**Response** (200 OK):
```json
{
  "data": [
    {
      "decision_id": "dec-1",
      "title": "Use Busboy for message bus",
      "description": "...",
      "component": "message-bus",
      "status": "approved",
      "created_at": "2026-01-27T12:00:00Z",
      "updated_at": "2026-01-27T12:00:00Z"
    }
  ],
  "timestamp": "2026-01-27T12:00:00Z"
}
```

## Building and Running

### Prerequisites

- Go 1.21+
- Make
- Access to Busboy (optional - can run with mock)

### Development

```bash
# Run with mock Busboy (no dependencies)
make dev

# Run tests
make test

# Run tests with race detection
make test-race

# Check coverage (target 80%+)
make test-coverage

# Build binary
make build
```

### Production

```bash
# Run service
./bin/architect \
  -addr :8001 \
  -busboy 10.10.10.10:9090 \
  -log info

# With NixOS container
systemctl start architect
systemctl enable architect
```

## Testing

### Test Coverage

Target: **80%+ coverage**

```bash
# Run all tests with coverage
make test-coverage-threshold

# View detailed coverage
open coverage.html
```

### Test Categories

1. **Unit Tests** (`core_test.go`)
   - Service validation
   - State management
   - Concurrency safety
   - Snapshot isolation

2. **HTTP Handler Tests** (`handlers_test.go`)
   - Endpoint functionality
   - Error handling
   - HTTP status codes
   - JSON serialization

3. **Integration Tests** (optional)
   - Busboy integration
   - End-to-end workflows

### Key Test Patterns

```go
// Test helpers
func createTestService(t *testing.T) *ArchitectService
func createValidService() *Service
func createValidNetworkNode() *NetworkNode
func createValidDecision() *ArchitectureDecision

// Concurrency tests
TestArchitectService_AddService_ConcurrentAccess
TestArchitectService_GetService_ConcurrentAccess
TestArchitectService_MixedConcurrentOps

// Validation tests
TestService_Validate_*
TestNetworkNode_Validate_*
TestArchitectureDecision_Validate_*

// Isolation tests
TestArchitectService_GetInfrastructureState_Snapshot
TestArchitectService_GetNetworkTopology_Snapshot
```

## Security

### Hardening (NixOS)

The service runs with strict security constraints:

- **Capabilities**: CAP_NET_BIND_SERVICE only
- **Filesystem**: Read-only system, isolated /tmp
- **Processes**: Namespace restrictions, seccomp filter
- **Resources**: CPU and memory limits
- **Networking**: Internal network only (10.10.10.0/24)

### Data Isolation

- Zero customer data access
- Architectural isolation enforced
- All state is service-internal
- No external data persistence required

### Input Validation

All API inputs are validated:

- Required fields checked
- Port ranges validated
- CIDR format verified
- Decision status enum validated
- Service IDs sanitized

## Observability

### Prometheus Metrics

```
unheaded_http_requests_total{service="architect",method="GET",path="/infrastructure",status="200"}
unheaded_http_request_duration_seconds{service="architect",method="GET",path="/infrastructure"}
unheaded_busboy_messages_published_total{service="architect",topic="architecture.updates"}
```

### Structured Logging

```
{"time":"2026-01-27T12:00:00Z","level":"info","service":"architect","operation":"ADD_SERVICE","message":"service added"}
```

### Busboy Integration

- **Subscribe**: `architecture.updates` (for receiving external changes)
- **Publish**: Architecture decisions and state changes

## Architecture Decisions

### Data Storage

Decision: In-memory state with optional persistence layer

**Rationale**:
- Service runs in containers with ephemeral state
- State is derived from Git (infrastructure configs)
- High-performance reads without disk I/O
- Can be extended with persistence later

### HTTP over gRPC

Decision: REST HTTP API over gRPC for architect service

**Rationale**:
- Simple integration with monitoring and observability tools
- JSON response formats match other Unheaded services
- Easier debugging and browser inspection
- Can add gRPC later if needed

### Thread Safety

Decision: sync.RWMutex for all shared state

**Rationale**:
- Simple, proven pattern
- No external dependencies
- Good performance for read-heavy workloads
- Easy to audit and test

## Performance

### Benchmarks

Run with:
```bash
make bench
```

Expected performance:
- `AddService`: < 100µs
- `GetService`: < 50µs
- `ListServices`: < 200µs (100 services)
- `GetInfrastructureState`: < 500µs
- HTTP request (HEAD-to-Response): < 5ms

### Load Testing

```bash
# 1000 requests/sec sustained
ab -c 100 -n 10000 http://localhost:8001/infrastructure
```

## Future Enhancements

1. **Persistence**: Add RocksDB for durable state
2. **Message Bus**: Publish state changes to Busboy
3. **Multi-region**: Support federated architect services
4. **Advanced Queries**: Filter and search infrastructure
5. **Change History**: Full audit log of state changes
6. **Webhooks**: Event subscriptions for external systems

## License

Part of Unheaded project. See root LICENSE.
