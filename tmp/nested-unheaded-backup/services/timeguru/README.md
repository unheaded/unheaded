# timeguru-service

**Timeline tracking microservice for Unheaded**

The timeguru service provides a REST API for managing project timelines, milestones, and progress tracking. It integrates with the Busboy message bus to publish timeline updates and subscribe to milestone change events.

---

## Features

- **REST API** for timeline and milestone management
- **Busboy integration** for real-time updates
- **SQLite persistence** for timeline data
- **Defensive coding** with comprehensive input validation
- **Graceful shutdown** with configurable timeout
- **Health check** endpoint for monitoring

---

## API Endpoints

### `GET /health`
Health check endpoint.

**Response:**
```json
{
  "status": "healthy",
  "service": "timeguru",
  "version": "1.0.0"
}
```

### `GET /timeline`
Retrieve the complete project timeline.

**Response:**
```json
{
  "timeline": {
    "version": "1.0.0",
    "last_updated": "2026-01-27T12:00:00Z",
    "status": "alpha",
    "vision": "Production-ready infrastructure in hours, not months.",
    "phases": [...],
    "milestones": [...]
  }
}
```

### `GET /milestones`
Retrieve all milestones.

**Response:**
```json
{
  "milestones": [
    {
      "id": "milestone-1",
      "name": "eBPF Foundation",
      "status": "in_progress",
      "progress": 25,
      "owner": "Agent 5",
      "risk": "medium"
    }
  ]
}
```

### `POST /milestones/:id/update`
Update a milestone's progress and status.

**Request:**
```json
{
  "progress": 50,
  "status": "in_progress"
}
```

**Response:**
```json
{
  "milestone": {
    "id": "milestone-1",
    "name": "eBPF Foundation",
    "status": "in_progress",
    "progress": 50
  },
  "message": "milestone updated successfully"
}
```

---

## Configuration

Configuration is loaded from environment variables:

| Variable | Default | Description |
|----------|---------|-------------|
| `PORT` | `8000` | HTTP server port |
| `BUSBOY_ADDR` | `localhost:9090` | Busboy server address |
| `DB_PATH` | `/opt/unheaded/data/timeguru.db` | SQLite database path |

---

## Building

```bash
# Install dependencies
make deps

# Run tests
make test

# Run tests with race detector
make test-race

# Check test coverage
make test-coverage

# Build binary
make build

# Clean artifacts
make clean
```

---

## Running

```bash
# Run with default config
make run

# Run with custom config
PORT=8001 BUSBOY_ADDR=busboy:9090 ./bin/timeguru
```

---

## Testing

The service follows strict TDD principles with comprehensive test coverage:

- **Unit tests** for data models, storage, and handlers
- **Concurrency tests** with race detector
- **Input validation tests** for all edge cases
- **Coverage target:** 80%+

```bash
# Run all tests
make test

# Run with race detection
make test-race

# Generate coverage report
make test-coverage
```

---

## Architecture

```
cmd/timeguru/
  └── main.go              # Service entry point

internal/
  ├── timeline/            # Data models
  │   ├── timeline.go
  │   └── timeline_test.go
  ├── storage/             # Persistence layer
  │   ├── storage.go
  │   └── storage_test.go
  └── api/                 # HTTP handlers
      ├── handlers.go
      └── handlers_test.go
```

---

## Security

The service implements defensive coding practices:

- **All inputs validated** (nil, empty, bounds, type)
- **All errors handled** explicitly
- **No sensitive data** in logs
- **Timeouts** on all network operations
- **Resource limits** (connection pooling, request timeouts)
- **Race detection** in tests

---

## Busboy Integration

The service connects to Busboy on startup and:

- **Subscribes** to `timeline.updates` topic
- **Publishes** milestone change events
- **Gracefully handles** connection failures
- **Continues operation** if Busboy is unavailable

---

## Development

```bash
# Install development tools
go install github.com/cosmtrek/air@latest
go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest

# Run with auto-reload
make dev

# Lint code
make lint

# Run benchmarks
make bench
```

---

## License

Part of the Unheaded project.

---

## Contact

- **GitHub:** https://github.com/unheaded/unheaded
- **Email:** hello@unheaded.com
