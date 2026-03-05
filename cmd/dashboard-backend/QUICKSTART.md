# Dashboard Backend - Quick Start

**Get running in < 5 minutes.**

---

## Prerequisites

- Go 1.21+ installed
- Wotan running (default: `localhost:9090`)

---

## Build & Run

```bash
# Clone and navigate
cd /path/to/unheaded/cmd/dashboard-backend

# Install dependencies
make deps

# Run tests (verify everything works)
make test-race

# Build
make build

# Run (connects to localhost:9090 Wotan)
./bin/dashboard-backend -debug
```

**That's it!** Server running on `http://localhost:8080`

---

## Verify It's Working

```bash
# Health check
curl http://localhost:8080/health
# {"status":"healthy"}

# Readiness check
curl http://localhost:8080/ready
# {"status":"ready","connections":0,"series":0}

# Prometheus metrics
curl http://localhost:8080/metrics
```

---

## Connect WebSocket Client

```javascript
// Browser console or Node.js
const ws = new WebSocket('ws://localhost:8080/ws');

ws.onmessage = (event) => {
  const data = JSON.parse(event.data);
  console.log('Packet flow:', data);
};

// You'll receive packet flows as eBPF traces arrive
```

---

## Query Metrics (when available)

```bash
# Query metrics API
curl -X POST http://localhost:8080/api/v1/metrics/query \
  -H "Content-Type: application/json" \
  -d '{
    "name": "http_requests_total",
    "start": "2026-01-27T00:00:00Z",
    "end": "2026-01-27T23:59:59Z",
    "aggregate": "sum"
  }'
```

---

## Configuration

### Command Line

```bash
./bin/dashboard-backend \
  -listen :8080 \           # HTTP listen address
  -wotan localhost:9090 \  # Wotan server address
  -debug                    # Enable debug logging
```

### Code (main.go)

Adjust config in `main.go`:

```go
config := &server.Config{
    WebSocketConfig: &websocket.Config{
        MaxConnections: 100,      // Max WebSocket clients
        ReadTimeout:    60 * time.Second,
        WriteTimeout:   10 * time.Second,
        BufferSize:     256,
    },

    MetricsConfig: &metrics.Config{
        RetentionPeriod: 1 * time.Hour,  // How long to keep metrics
        MaxSeries:       10000,           // Max unique series
        FlushInterval:   1 * time.Minute, // Cleanup frequency
    },

    PacketFlowConfig: &packetflow.Config{
        Interval:       100 * time.Millisecond, // Processing interval
        MaxFlows:       50,
        TraceIDPattern: "trace-%d",
    },
}
```

---

## Development Workflow

### 1. Make Changes

```bash
# Edit code
vim internal/server/server.go
```

### 2. Run Tests

```bash
# Unit tests
make test

# With race detector (IMPORTANT)
make test-race

# Check coverage
make test-coverage
```

### 3. Build & Test Locally

```bash
# Build
make build

# Run with debug logging
./bin/dashboard-backend -debug
```

### 4. Verify

```bash
# In another terminal
curl http://localhost:8080/health
```

---

## Troubleshooting

### "go: command not found"

**Solution:** Install Go 1.21+

```bash
# macOS
brew install go

# Linux
wget https://go.dev/dl/go1.21.linux-amd64.tar.gz
sudo tar -C /usr/local -xzf go1.21.linux-amd64.tar.gz
export PATH=$PATH:/usr/local/go/bin
```

### "connect to wotan: connection refused"

**Solution:** Ensure Wotan is running

```bash
# Start Wotan (from wotan repo)
cd /path/to/wotan
./bin/wotan
```

Or point to different address:

```bash
./bin/dashboard-backend -wotan remote-host:9090
```

### "max connections reached"

**Solution:** Increase limit in code or disconnect idle clients

```go
WebSocketConfig: &websocket.Config{
    MaxConnections: 500,  // Increase from 100
}
```

### Tests failing

**Solution:** Check if Go modules are up to date

```bash
go mod download
go mod tidy
make test
```

---

## Production Deployment

### Docker

```dockerfile
FROM golang:1.21-alpine AS builder
WORKDIR /app
COPY . .
RUN go build -o dashboard-backend .

FROM alpine:latest
RUN apk add --no-cache ca-certificates
COPY --from=builder /app/dashboard-backend /usr/local/bin/
ENTRYPOINT ["dashboard-backend"]
```

```bash
docker build -t dashboard-backend .
docker run -p 8080:8080 dashboard-backend -wotan wotan:9090
```

### Kubernetes

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: dashboard-backend
spec:
  replicas: 2
  template:
    spec:
      containers:
      - name: dashboard-backend
        image: dashboard-backend:latest
        args: ["-wotan=wotan:9090"]
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

---

## Next Steps

1. Read [README.md](README.md) for full documentation
2. Check [SECURITY_AUDIT.md](SECURITY_AUDIT.md) for security considerations
3. Review [BUILD_SUMMARY.md](BUILD_SUMMARY.md) for architecture details
4. Run `make help` to see all build targets

---

## Need Help?

- Check logs: Debug mode (`-debug`) shows detailed logs
- Run tests: `make test-race` catches most issues
- Review code: Well-commented, follow patterns
- Security: See `SECURITY_AUDIT.md` for threat model

---

**Happy hacking!** 🚀
