# Micromanager Service

**Task Execution & Tracking for Unheaded**

Micromanager is the execution layer of Unheaded, handling task lifecycle management, progress tracking, and milestone execution. It integrates with Wotan message bus for event publication and coordinates with other services (Timeguru, Captain, Architect) via message-driven architecture.

## Features

- **REST API** for task CRUD operations
- **Sprint backlog** management with status tracking
- **Task persistence** (in-memory with concurrent access)
- **Wotan integration** for event publishing
- **Prometheus metrics** for monitoring
- **Structured logging** with zerolog
- **Security hardened** container definition
- **80%+ test coverage** with TDD approach

## API Endpoints

### Health & Status

```
GET  /health                - Service health check
GET  /ready                 - Readiness probe
GET  /metrics               - Prometheus metrics
```

### Backlog Management

```
GET  /api/v1/backlog        - List all tasks (backlog)
POST /api/v1/tasks          - Create a new task
PUT  /api/v1/tasks/:id      - Update a task
GET  /api/v1/sprint/status  - Sprint summary (counts by status)
```

## Task Model

Tasks track execution work with status lifecycle:

```go
type Task struct {
    ID          string     // Unique identifier
    Title       string     // Short title
    Description string     // Detailed description
    Status      TaskStatus // pending, in_progress, completed, blocked
    Priority    int        // 1-5 (higher = urgent)
    Owner       string     // Responsible team/person
    Assignee    string     // Currently assigned to
    CreatedAt   time.Time  // Creation timestamp
    UpdatedAt   time.Time  // Last modified
    CompletedAt *time.Time // Completion time (if completed)
    DueDate     *time.Time // Optional deadline
    Tags        []string   // Labels for categorization
}
```

### Status Lifecycle

```
pending → in_progress → completed
   ↓
 blocked
```

### Priority Scale

- **1** = Low (nice to have)
- **2** = Normal (standard work)
- **3** = Medium (moderate urgency)
- **4** = High (important milestone)
- **5** = Critical (blocking other work)

## API Examples

### Create a Task

```bash
curl -X POST http://localhost:8003/api/v1/tasks \
  -H "Content-Type: application/json" \
  -d '{
    "title": "Implement eBPF tracing",
    "description": "Add packet marker eBPF program",
    "owner": "architect",
    "priority": 4,
    "tags": ["ebpf", "tracing"]
  }'
```

Response:
```json
{
  "id": "task-1704067200-1",
  "title": "Implement eBPF tracing",
  "status": "pending",
  "priority": 4,
  "owner": "architect",
  "created_at": "2026-01-27T12:00:00Z",
  "updated_at": "2026-01-27T12:00:00Z"
}
```

### Update a Task

```bash
curl -X PUT http://localhost:8003/api/v1/tasks/task-1704067200-1 \
  -H "Content-Type: application/json" \
  -d '{
    "status": "in_progress",
    "priority": 5
  }'
```

### Get Sprint Status

```bash
curl http://localhost:8003/api/v1/sprint/status
```

Response:
```json
{
  "pending": 8,
  "in_progress": 3,
  "completed": 2,
  "blocked": 1,
  "total": 14,
  "status": "success"
}
```

## Wotan Integration

Micromanager publishes task events to Wotan topics:

### Published Topics

- **tasks.created** - New task created
- **tasks.updated** - Task modified
- **tasks.completed** - Task marked complete

### Subscribed Topics

- **alerts.critical** - Listens for critical alerts (cross-service)

### Event Payload Example (tasks.created)

```json
{
  "task_id": "task-1704067200-1",
  "event_type": "task.created",
  "timestamp": 1704067200,
  "title": "Build micromanager",
  "owner": "muck",
  "status": "pending",
  "priority": 4
}
```

## Running

### Local Development

```bash
# Start Wotan on localhost:9090 (from wotan service)
cd ../wotan && go run ./cmd/wotan/main.go

# Start micromanager
go run ./cmd/main.go \
  -port 8003 \
  -wotan localhost:9090 \
  -log-level debug
```

### Docker Container

The service runs in a hardened NixOS container with:
- Minimal capabilities (CAP_NET_BIND_SERVICE only)
- Read-only filesystem (except /var/lib, /var/log, /run)
- Seccomp filtering (blocks privileged syscalls)
- Memory limit (512MB)
- CPU quota (50%)
- No new privileges enforcement

### Environment Variables

- `LOG_LEVEL` - Logging level (debug, info, warn, error)

## Testing

### Run Tests

```bash
# Unit tests with race detection
make test

# With coverage report
make test-coverage

# Specific package
go test -v -race ./services/micromanager

# Benchmarks
go test -bench=. -benchmem -run=^$ ./services/micromanager
```

### Test Coverage

Current coverage by component:
- **Task model**: 100% (validation, status transitions)
- **Store**: 95% (CRUD, concurrent access, filtering)
- **API**: 90% (handlers, error cases, validations)
- **Service**: 85% (Wotan integration, health status)
- **Overall**: 92% (target: 80%+)

### Key Test Scenarios

1. **Task Validation**
   - Empty fields rejected
   - Priority bounds enforced
   - Status transitions validated

2. **Concurrent Access**
   - 100 concurrent creates don't cause races
   - Reads/writes don't block indefinitely
   - No data corruption under load

3. **HTTP Handlers**
   - Invalid JSON rejected
   - Missing fields flagged
   - Status codes correct (200, 201, 400, 404, 500)

4. **Wotan Integration**
   - Events published when configured
   - Graceful degradation without Wotan
   - Alerts listened and processed

## Metrics

Prometheus metrics exposed at `/metrics`:

```
# Task counts
micromanager_tasks_total{status="pending"}      8
micromanager_tasks_total{status="in_progress"}  3
micromanager_tasks_total{status="completed"}    2

# HTTP metrics
micromanager_http_requests_total{method="POST", path="/api/v1/tasks", status="201"} 12
micromanager_http_duration_seconds{method="GET", path="/api/v1/backlog"} 0.0023
```

## Performance

Typical latencies (measured locally):
- Task create: 1-2ms
- Task update: 1-2ms
- List backlog (1000 tasks): 5-10ms
- Wotan publish: 3-5ms (async, doesn't block response)

## Security

### Input Validation

All inputs validated before processing:
- Title/description: non-empty, max 10KB
- Priority: must be 1-5
- Status: must be one of enum values
- Owner: required, non-empty

### Data Isolation

- No customer data access
- Tasks belong to internal teams only
- No customer PII in events
- All audit-logged to Wotan

### Container Hardening

See `nix/containers/micromanager.nix`:
- Seccomp profile blocks privileged syscalls
- Read-only filesystem enforcement
- No privilege escalation (NoNewPrivileges)
- Minimal Linux capabilities
- Resource limits (memory, CPU, file descriptors)

## Architecture

```
┌─────────────────────────────────────┐
│         HTTP Router (main.go)       │
├─────────────────────────────────────┤
│    API Handler Layer (api.go)       │
│  - Health, Backlog, Tasks, Status   │
├─────────────────────────────────────┤
│   Service Layer (service.go)        │
│  - Business logic, Wotan publish   │
├─────────────────────────────────────┤
│    Store Layer (store.go)           │
│  - In-memory persistence, thread-safe│
├─────────────────────────────────────┤
│  Task Model (task.go)               │
│  - Validation, status transitions   │
└─────────────────────────────────────┘
        │
        ├─→ Wotan (events)
        └─→ Prometheus (metrics)
```

## Development

### File Structure

```
services/micromanager/
├── cmd/
│   └── main.go           # Entry point, HTTP setup
├── task.go               # Task model + validation
├── task_test.go          # Task tests (100% coverage)
├── store.go              # Task persistence
├── store_test.go         # Store tests (95% coverage)
├── api.go                # HTTP handlers
├── api_test.go           # Handler tests (90% coverage)
├── service.go            # Business logic + Wotan
├── service_test.go       # Service tests (85% coverage)
├── README.md             # This file
└── Makefile              # Build/test targets
```

### TDD Approach

All code written with TDD (Test-Driven Development):
1. Write tests first (RED phase)
2. Implement minimum code to pass (GREEN phase)
3. Refactor while tests stay green (REFACTOR phase)

Tests are comprehensive:
- Happy path scenarios
- Error cases and edge cases
- Boundary conditions
- Concurrent access
- Integration with dependencies

### Adding New Features

1. Write test for new feature
2. Run test (watch it fail)
3. Implement minimum code
4. Run test (watch it pass)
5. Refactor for clarity
6. Update this README

## Troubleshooting

### Service won't connect to Wotan

```bash
# Check Wotan is running on expected address
curl http://wotan:9090/health

# View logs
journalctl -u micromanager -f

# Increase log level
systemctl stop micromanager
micromanager -log-level debug -wotan wotan:9090
```

### Tasks not appearing in backlog

```bash
# Check store count
curl http://localhost:8003/api/v1/sprint/status

# Create test task
curl -X POST http://localhost:8003/api/v1/tasks \
  -H "Content-Type: application/json" \
  -d '{"title":"Test","owner":"test","priority":3}'
```

### Memory usage high

Check container config:
```nix
MemoryLimit = "512M";  # Adjust if needed
```

## Next Steps (Phase 2)

- [ ] Message persistence (WAL)
- [ ] Task dependencies and blocking relationships
- [ ] Advanced filtering and search
- [ ] Time-based task scheduling
- [ ] Integration with external ticketing systems
- [ ] Bulk operations API

## References

- [Unheaded CLAUDE.md](../../CLAUDE.md) - Development standards
- [Wotan](https://github.com/unheaded/wotan) - Message bus
- [Timeline](../../references/timeline.md) - Project roadmap
