# Busboy

A high-performance, in-memory message bus and broker for distributed systems.

Busboy is the messaging backbone of the Unheaded infrastructure platform. It provides pub/sub messaging, request-reply patterns, and event streaming with sub-millisecond latency for service-to-service communication.

## Status

**Pre-Alpha**. Interfaces, wire formats, and APIs are subject to change.

## Overview

Busboy implements a dual-plane architecture separating control operations from data delivery:

- **Control Plane (REST)**: Topic management, subscriptions, health checks, administrative operations
- **Data Plane (gRPC)**: Low-latency bidirectional streaming for message delivery

The same core patterns that enable real-time messaging scale to handle infrastructure automation events, service discovery, alert routing, and log aggregation.

## Key Properties

- **In-memory ring buffer**: Fixed-size circular buffer per topic with configurable retention
- **Zero persistence by default**: Ephemeral messaging for maximum throughput (optional WAL planned)
- **Thread-safe**: Concurrent read/write access with minimal lock contention
- **Observable**: Prometheus metrics, structured logging, health/readiness endpoints
- **Production-minded**: Rate limiting, circuit breakers, graceful shutdown, TLS 1.3

## Use Cases

- Service-to-service messaging in microservices architectures
- Event streaming and fanout
- Real-time notifications and alerts
- Infrastructure automation coordination
- Log and metrics aggregation pipelines

## Architecture

```
                           ┌─────────────────────────────────────┐
                           │            Busboy Server            │
                           │                                     │
  ┌──────────┐  REST API   │  ┌─────────┐      ┌─────────────┐  │
  │  Admin   │◀───────────▶│  │ Control │      │  Ring       │  │
  │  Client  │             │  │ Plane   │◀────▶│  Buffer(s)  │  │
  └──────────┘             │  └─────────┘      └─────────────┘  │
                           │                          │          │
  ┌──────────┐  gRPC       │  ┌─────────┐             │          │
  │ Service  │◀───────────▶│  │  Data   │◀────────────┘          │
  │    A     │  Streaming  │  │  Plane  │                        │
  └──────────┘             │  └─────────┘                        │
                           │        │                            │
  ┌──────────┐  gRPC       │        │      ┌────────────────┐   │
  │ Service  │◀────────────┼────────┴─────▶│  Pub/Sub       │   │
  │    B     │             │               │  Engine        │   │
  └──────────┘             │               └────────────────┘   │
                           └─────────────────────────────────────┘
```

## Quick Start

### Prerequisites

- Go 1.21+
- Protocol Buffers compiler (protoc)

### Build

```bash
# Generate protobuf code
cd proto && ./generate.sh && cd ..

# Build server
cd server && go build -o ../bin/busboy ./cmd/server && cd ..

# Build CLI client (optional)
cd client/terminal && go build -o ../../bin/busboy-cli . && cd ../..
```

### Run

```bash
# Start server (development mode)
./bin/busboy --buffer-size 10000 --http-port 8080 --grpc-port 9090 --log-pretty

# Start with TLS
./bin/busboy --enable-tls --tls-cert server.crt --tls-key server.key
```

### Verify

```bash
# Health check
curl http://localhost:8080/health

# Metrics
curl http://localhost:8080/metrics
```

## Configuration

| Flag | Default | Description |
|------|---------|-------------|
| `--buffer-size` | 10000 | Ring buffer capacity per topic |
| `--http-port` | 8080 | REST API port |
| `--grpc-port` | 9090 | gRPC streaming port |
| `--enable-tls` | false | Enable TLS for both transports |
| `--tls-cert` | - | Path to TLS certificate |
| `--tls-key` | - | Path to TLS private key |
| `--rate-limit` | 100 | Requests per second limit |
| `--rate-burst` | 200 | Burst capacity |
| `--log-level` | info | Log level (debug, info, warn, error) |
| `--log-pretty` | false | Human-readable log format |

## Documentation

- [Architecture](ARCHITECTURE.md): System design and component details
- [Protocol Specification](SPEC.md): Wire protocol and API contracts
- [Quick Start Guide](QUICKSTART.md): Detailed setup and usage
- [Development Guide](CLAUDE.md): Contributing and development context

## Roadmap

### Phase 1: Core Message Bus (Current)

- [x] Ring buffer implementation
- [x] Pub/sub engine
- [x] gRPC bidirectional streaming
- [x] REST control plane
- [x] Prometheus metrics
- [x] Rate limiting and circuit breakers
- [ ] Comprehensive test suite
- [ ] Performance benchmarks

### Phase 2: Production Hardening

- [ ] Message persistence (write-ahead log)
- [ ] Clustering (multi-node HA)
- [ ] Message acknowledgment and replay
- [ ] Dead letter queues
- [ ] HTTP/3 + QUIC support

### Phase 3: Infrastructure Integration

- [ ] Service registry
- [ ] Alert routing
- [ ] Log aggregation
- [ ] Metrics pipeline

## Part of Unheaded

Busboy is the messaging backbone of [Unheaded](https://github.com/unheaded), an infrastructure automation platform. The same patterns that handle service messaging will coordinate infrastructure deployments, alert routing, and observability pipelines.

## License

MIT
