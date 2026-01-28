# Quick Start Guide

Get Busboy running in under 5 minutes.

## Prerequisites

- Go 1.21+
- Protocol Buffers compiler (`protoc`)
- Ports 8080 (HTTP) and 9090 (gRPC) available

## Build

### 1. Generate Protocol Buffers

```bash
cd proto
./generate.sh
cd ..
```

### 2. Build Server

```bash
cd server
go mod tidy
go build -o ../bin/busboy ./cmd/server
cd ..
```

### 3. Build CLI Client (Optional)

```bash
cd client/terminal
go mod tidy
go build -o ../../bin/busboy-cli .
cd ../..
```

## Run

### Start Server

```bash
./bin/busboy \
    --buffer-size 10000 \
    --http-port 8080 \
    --grpc-port 9090 \
    --log-level info \
    --log-pretty
```

Expected output:

```
{"level":"info","service":"busboy","message":"server_starting"}
{"level":"info","buffer_size":10000,"http_port":8080,"grpc_port":9090,"message":"configuration"}
{"level":"info","port":8080,"message":"http_server_started"}
{"level":"info","port":9090,"message":"grpc_server_started"}
```

### Verify Server Health

```bash
curl http://localhost:8080/health
```

Expected response:

```json
{"status":"healthy","timestamp":"2026-01-26T12:00:00Z"}
```

## Basic Operations

### Subscribe to a Topic

```bash
curl -X POST http://localhost:8080/api/v1/topics/events/subscribe \
    -H "Content-Type: application/json" \
    -d '{"display_name": "client-1"}'
```

Response:

```json
{
    "subscriber": {
        "subscriber_id": "550e8400-e29b-41d4-a716-446655440000",
        "topic": "events",
        "status": "pending",
        "requested_at": "2026-01-26T12:00:00Z"
    }
}
```

### Approve Subscriber (Admin)

List pending subscribers:

```bash
curl http://localhost:8080/api/v1/admin/pending
```

Approve:

```bash
curl -X POST http://localhost:8080/api/v1/admin/approve \
    -H "Content-Type: application/json" \
    -d '{"member_id": "550e8400-e29b-41d4-a716-446655440000"}'
```

### Publish a Message

```bash
curl -X POST http://localhost:8080/api/v1/topics/events/publish \
    -H "Content-Type: application/json" \
    -d '{
        "member_id": "550e8400-e29b-41d4-a716-446655440000",
        "body": "deployment started"
    }'
```

### Fetch Messages

```bash
curl "http://localhost:8080/api/v1/topics/events/messages?member_id=550e8400-..."
```

## Using the CLI Client

### Connect and Subscribe

```bash
./bin/busboy-cli --name my-service --room events
```

The client will:

1. Request subscription to the topic
2. Wait for admin approval
3. Connect gRPC stream for real-time messages
4. Display incoming messages with timestamps

### CLI Commands

| Command | Description |
|---------|-------------|
| `/help` | Show available commands |
| `/ping` | Test gRPC connectivity |
| `/quit` | Exit client |
| `<text>` | Publish message |

## Observability

### Prometheus Metrics

```bash
curl http://localhost:8080/metrics | grep busboy
```

Key metrics:

- `busboy_http_requests_total` - Request counts by endpoint
- `busboy_messages_published_total` - Messages published per topic
- `busboy_streams_active` - Active gRPC streams

### Structured Logs

Server logs include:

- Request ID for tracing
- Topic and subscriber context
- Duration and status codes
- Error details with stack context

## TLS Configuration

### Generate Test Certificates

```bash
./scripts/generate-certs.sh
```

### Run with TLS

```bash
./bin/busboy \
    --enable-tls \
    --tls-cert server.crt \
    --tls-key server.key \
    --http-port 8443 \
    --grpc-port 9443
```

### Connect CLI with TLS

```bash
./bin/busboy-cli \
    --server https://localhost:8443 \
    --grpc-server localhost:9443 \
    --tls \
    --name my-service \
    --room events
```

## Docker

### Build Image

```bash
docker build -t busboy:latest .
```

### Run Container

```bash
docker run -p 8080:8080 -p 9090:9090 busboy:latest
```

### With Docker Compose

```bash
docker-compose up
```

## Troubleshooting

### Server won't start

Check port availability:

```bash
lsof -i :8080 -i :9090
```

### Client can't connect

Verify server health:

```bash
curl http://localhost:8080/health
```

Check logs for errors:

```bash
./bin/busboy --log-level debug --log-pretty
```

### Subscriber stuck in pending

List and approve:

```bash
curl http://localhost:8080/api/v1/admin/pending
curl -X POST http://localhost:8080/api/v1/admin/approve \
    -H "Content-Type: application/json" \
    -d '{"member_id": "<subscriber-id>"}'
```

### gRPC stream disconnects

- Verify subscriber is approved (not pending)
- Check for rate limiting (429 responses)
- Enable debug logging for details

## Next Steps

- Review [Architecture](ARCHITECTURE.md) for system design
- Read [Protocol Specification](SPEC.md) for API contracts
- Check [Progress](PROGRESS.md) for current status
