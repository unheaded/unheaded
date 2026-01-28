# Captain Service

Leadership and strategy service for Unheaded. Tracks executive decisions and publishes strategic updates via Busboy.

## Overview

The captain service represents organizational leadership and strategic decision-making. It provides:

- **Vision Management**: Returns the project vision and goals
- **Strategy Planning**: Serves the strategic execution plan
- **Decision Tracking**: Logs and persists executive decisions
- **Alert Subscription**: Subscribes to critical system alerts
- **Metrics Export**: Exposes operational metrics in Prometheus format

## Architecture

### Core Components

1. **Service (captain.go)**: Business logic layer
   - `Service`: Main service struct
   - `Decision`: Decision data model
   - `Vision`: Project vision
   - `Strategy`: Strategic plan

2. **HTTP API (api.go)**: REST API layer
   - `HTTPServer`: HTTP server with endpoints
   - Request/response handling
   - Metrics tracking

3. **Storage (storage.go)**: Data persistence layer
   - `FileStorage`: File-based persistence
   - `MemoryStorage`: In-memory storage for testing
   - Path traversal protection
   - Atomic writes

4. **Main (main.go)**: Service entry point
   - Busboy integration
   - Alert listening
   - Graceful shutdown

## REST API Endpoints

### Health & Metrics

```bash
GET /health                    # Health check (200 if healthy)
GET /ready                     # Readiness check (200 if ready)
GET /metrics                   # Prometheus metrics
```

### Vision & Strategy

```bash
GET /api/v1/vision            # Get project vision
GET /api/v1/strategy          # Get strategic plan
```

### Decisions

```bash
POST /api/v1/decisions        # Create a decision
GET /api/v1/decisions         # List decisions (with ?limit=10&offset=0)
GET /api/v1/decisions/{id}    # Get specific decision
PATCH /api/v1/decisions/{id}  # Update decision status
```

## Data Model

### Decision

```go
type Decision struct {
    ID        string    `json:"id"`          // Unique identifier
    Title     string    `json:"title"`       // Decision title (max 500 chars)
    Content   string    `json:"content"`     // Decision details (max 10000 chars)
    Owner     string    `json:"owner"`       // Decision owner (max 200 chars)
    Priority  Priority  `json:"priority"`    // PriorityLow/Medium/High/Critical
    CreatedAt time.Time `json:"created_at"` // Creation timestamp
    UpdatedAt time.Time `json:"updated_at"` // Last update timestamp
    Status    string    `json:"status"`     // pending/approved/rejected/archived
}
```

### Priority Levels

- `PriorityLow` (0)
- `PriorityMedium` (1)
- `PriorityHigh` (2)
- `PriorityCritical` (3)

## Configuration

Environment variables:

```bash
BUSBOY_ADDR=localhost:9090    # Busboy message bus address
HTTP_ADDR=0.0.0.0:8000        # HTTP server address
DATA_PATH=/var/lib/unheaded/captain  # Data directory
LOG_LEVEL=info                # Logging level (debug/info/warn/error)
```

## Running Locally

### Prerequisites

- Go 1.21+
- Busboy service running
- Write access to `/var/lib/unheaded/captain`

### Start Service

```bash
# With environment variables
export BUSBOY_ADDR=localhost:9090
export HTTP_ADDR=localhost:8000
export DATA_PATH=/tmp/captain-data

# Run service
go run ./cmd/captain/main.go

# Or build binary
go build -o bin/captain ./cmd/captain
./bin/captain
```

### Test Endpoints

```bash
# Health check
curl http://localhost:8000/health

# Get vision
curl http://localhost:8000/api/v1/vision

# Get strategy
curl http://localhost:8000/api/v1/strategy

# Create decision
curl -X POST http://localhost:8000/api/v1/decisions \
  -H "Content-Type: application/json" \
  -d '{
    "title": "Deploy to production",
    "content": "All systems ready for deployment",
    "owner": "captain",
    "priority": 3
  }'

# List decisions
curl http://localhost:8000/api/v1/decisions?limit=10&offset=0

# Get specific decision
curl http://localhost:8000/api/v1/decisions/{id}

# Update decision
curl -X PATCH http://localhost:8000/api/v1/decisions/{id} \
  -H "Content-Type: application/json" \
  -d '{"status": "approved"}'
```

## Testing

### Run All Tests

```bash
# Unit tests with race detection
go test -v -race ./services/captain/...

# With coverage
go test -v -race -coverprofile=coverage.out ./services/captain/...
go tool cover -func=coverage.out
```

### Test Coverage

Target: **80%+ coverage**

Current coverage includes:

- **Service Logic** (captain_test.go)
  - NewService validation
  - Vision retrieval
  - Strategy retrieval
  - Decision logging with validation
  - Decision retrieval
  - Decision listing with pagination
  - Decision status updates
  - Service lifecycle
  - Concurrent operations
  - Benchmarks

- **Storage** (storage_test.go)
  - FileStorage persistence
  - Memory storage
  - Path traversal protection
  - File integrity
  - Cache behavior
  - Invalid data handling

- **HTTP API** (api_test.go)
  - Health endpoint
  - Readiness endpoint
  - Metrics endpoint
  - Vision endpoint
  - Strategy endpoint
  - Decision creation
  - Decision listing
  - Decision retrieval
  - Decision updates
  - Error handling
  - Content validation
  - Large payload handling

## Defensive Coding Principles

### Input Validation

All inputs are validated defensively:

- **Nil checks**: Every pointer parameter
- **Empty string checks**: Title, content, owner
- **Bounds checking**: Priority levels, decision counts
- **Size limits**: Title (500), content (10000), owner (200)
- **Path traversal**: Safe path construction with allowed characters only

### Error Handling

All errors are handled explicitly:

- Wrapped errors with context
- Distinct error types for debugging
- No silent failures
- Proper error propagation

### Concurrency Safety

All shared state is protected:

- RWMutex for read-heavy operations
- Atomic operations for persistence
- Race detection in tests
- Concurrent access testing

### Security

- **No hardcoded secrets**: All config from environment
- **Atomic writes**: Temporary files with rename
- **File permissions**: 0600 for data, 0700 for directories
- **Path validation**: Prevent directory traversal
- **Resource limits**: 10MB max request body
- **Timeout protection**: 15s read/write timeouts

## Busboy Integration

The captain service publishes decision updates to Busboy:

```
Topic: decisions.created
Message: Decision JSON payload
```

And subscribes to:

```
Topic: alerts.critical
Handler: Log alert and potentially trigger response
```

## Storage

### FileStorage

Persists decisions to disk with:

- **Atomic writes**: Write to temp file, then rename
- **Safe paths**: Validate IDs to prevent traversal
- **In-memory cache**: Fast reads for recent decisions
- **Directory isolation**: All files in basePath

### MemoryStorage

In-memory implementation for:

- Testing
- High-performance scenarios
- Temporary data storage

## Metrics

Prometheus-format metrics:

```
captain_http_requests_total      # Total HTTP requests
captain_http_requests_success    # Successful requests
captain_http_requests_error      # Failed requests
captain_decisions_logged         # Total decisions logged
captain_decisions_priority       # Decision priority distribution
```

## NixOS Container

The service includes a hardened NixOS container definition (`nix/containers/captain.nix`):

- **Capabilities**: Only `CAP_NET_BIND_SERVICE`
- **Filesystem**: Strict isolation, read-only root
- **Seccomp**: Blocks dangerous syscalls
- **Network**: Explicit allow rules, default deny
- **Resources**: Bounded memory/processes

## Security Considerations

### What This Service Does

- Manages strategic decisions
- Tracks project vision and goals
- Publishes to internal message bus
- Logs decision activity

### What This Service Does NOT Do

- Access customer data
- Expose infrastructure internals
- Store secrets in decisions
- Execute arbitrary code

### Isolation

The captain service is architecturally isolated:

- No access to customer data channels
- Separate filesystem from other services
- Cannot access other service storage
- Limited network permissions

## Performance

### Benchmarks

```
BenchmarkService_LogDecision     ~100 ops/sec (file I/O bound)
BenchmarkService_GetVision       ~50000 ops/sec (cached)
BenchmarkService_GetStrategy     ~50000 ops/sec (cached)
```

### Resource Limits

- **Memory**: <100MB typical
- **Disk**: ~1MB per 100 decisions
- **CPU**: <5% idle
- **Connections**: <10 typical

## Deployment

### Container Startup

```bash
systemctl start captain
systemctl status captain
journalctl -u captain -f
```

### Health Checks

```bash
# HTTP health check
curl -f http://localhost:8000/health

# Readiness probe
curl -f http://localhost:8000/ready
```

### Monitoring

```bash
# View metrics
curl http://localhost:8000/metrics

# View logs
journalctl -u captain -n 100
```

## Development

### Adding New Endpoints

1. Define handler in `api.go`
2. Add route in `NewHTTPServer`
3. Add tests in `api_test.go`
4. Update documentation

### Changing Data Models

1. Update struct in `captain.go`
2. Add validation method
3. Update tests
4. Add migration path for existing data

### Extending Storage

1. Implement `Storage` interface
2. Add to factory in `main.go`
3. Add tests for new implementation

## Troubleshooting

### Service Won't Start

```bash
# Check logs
journalctl -u captain -n 50

# Verify environment
echo $BUSBOY_ADDR $HTTP_ADDR $DATA_PATH

# Test connectivity
curl http://localhost:9090/health
```

### Decisions Not Persisting

```bash
# Check data directory
ls -la /var/lib/unheaded/captain

# Verify permissions
stat /var/lib/unheaded/captain

# Test storage directly
go test -v -run TestFileStorage ./services/captain
```

### High Memory Usage

```bash
# Check for large decisions
curl http://localhost:8000/api/v1/decisions?limit=1000

# Restart service
systemctl restart captain

# Check journalctl for errors
journalctl -u captain --since 1h -p warning
```

## References

- [CLAUDE.md](../../CLAUDE.md) - Development standards
- [timeline.md](../../references/timeline.md) - Project timeline
- [Busboy Documentation](https://github.com/unheaded/busboy)

## License

Unheaded - Private Project
