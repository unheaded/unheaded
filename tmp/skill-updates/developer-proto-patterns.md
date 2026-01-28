# Developer Addition: Proto Patterns

**Add to references/ folder as `references/proto-patterns.md`**

---

# Protocol Buffer & gRPC Patterns

Patterns for defining and implementing gRPC services in Unheaded.

## Proto File Structure

```
proto/
├── busboy/
│   └── v1/
│       ├── busboy.proto      # Service definitions
│       └── types.proto       # Shared message types
├── timeline/
│   └── v1/
│       └── timeline.proto
└── buf.yaml                  # Buf configuration
```

## Proto Style Guide

### Package Naming
```protobuf
// Package format: unheaded.<service>.v<version>
package unheaded.busboy.v1;

option go_package = "github.com/unheaded/unheaded/gen/proto/busboy/v1;busboyv1";
```

### Message Definitions
```protobuf
// Use descriptive names, PascalCase
message PublishRequest {
  // Required fields first, optional later
  string topic = 1;
  bytes payload = 2;

  // Optional metadata
  map<string, string> metadata = 3;

  // Timestamps use google.protobuf.Timestamp
  google.protobuf.Timestamp created_at = 4;
}

message PublishResponse {
  string message_id = 1;
  int64 sequence = 2;
}
```

### Service Definitions
```protobuf
service BusboyService {
  // Unary RPCs
  rpc Publish(PublishRequest) returns (PublishResponse);
  rpc Subscribe(SubscribeRequest) returns (SubscribeResponse);

  // Server streaming (for message consumption)
  rpc StreamMessages(StreamRequest) returns (stream Message);

  // Bidirectional streaming (for chat-like flows)
  rpc Chat(stream ChatMessage) returns (stream ChatMessage);
}
```

### Error Handling
```protobuf
// Use standard gRPC status codes + custom details
message ErrorDetail {
  string code = 1;      // Machine-readable: "SUBSCRIPTION_PENDING"
  string message = 2;   // Human-readable: "Subscription requires approval"
  map<string, string> metadata = 3;
}
```

## Go Implementation Patterns

### Server Implementation
```go
type busboyServer struct {
    busboyv1.UnimplementedBusboyServiceServer

    bus    *MessageBus
    logger zerolog.Logger
}

func (s *busboyServer) Publish(ctx context.Context, req *busboyv1.PublishRequest) (*busboyv1.PublishResponse, error) {
    // Validate input
    if req.Topic == "" {
        return nil, status.Error(codes.InvalidArgument, "topic required")
    }
    if len(req.Payload) == 0 {
        return nil, status.Error(codes.InvalidArgument, "payload required")
    }

    // Execute
    msg, err := s.bus.Publish(ctx, req.Topic, req.Payload)
    if err != nil {
        // Map domain errors to gRPC codes
        switch {
        case errors.Is(err, ErrNotAuthorized):
            return nil, status.Error(codes.PermissionDenied, err.Error())
        case errors.Is(err, ErrRateLimited):
            return nil, status.Error(codes.ResourceExhausted, err.Error())
        default:
            return nil, status.Error(codes.Internal, "publish failed")
        }
    }

    return &busboyv1.PublishResponse{
        MessageId: msg.ID,
        Sequence:  msg.Seq,
    }, nil
}
```

### Streaming Server
```go
func (s *busboyServer) StreamMessages(req *busboyv1.StreamRequest, stream busboyv1.BusboyService_StreamMessagesServer) error {
    ctx := stream.Context()

    // Subscribe to topic
    msgCh, err := s.bus.Subscribe(ctx, req.Topic)
    if err != nil {
        return status.Error(codes.Internal, "subscribe failed")
    }

    // Stream messages until context cancelled
    for {
        select {
        case <-ctx.Done():
            return nil
        case msg, ok := <-msgCh:
            if !ok {
                return nil
            }
            if err := stream.Send(toProto(msg)); err != nil {
                return err
            }
        }
    }
}
```

### Client Usage
```go
// Create client
conn, err := grpc.Dial(addr,
    grpc.WithTransportCredentials(insecure.NewCredentials()),
    grpc.WithKeepaliveParams(keepalive.ClientParameters{
        Time:    30 * time.Second,
        Timeout: 10 * time.Second,
    }),
)
if err != nil {
    return fmt.Errorf("dial: %w", err)
}
defer conn.Close()

client := busboyv1.NewBusboyServiceClient(conn)

// Unary call
resp, err := client.Publish(ctx, &busboyv1.PublishRequest{
    Topic:   "events.system",
    Payload: payload,
})
if err != nil {
    st, ok := status.FromError(err)
    if ok {
        // Handle specific codes
        switch st.Code() {
        case codes.PermissionDenied:
            // Handle auth error
        case codes.ResourceExhausted:
            // Handle rate limit
        }
    }
    return fmt.Errorf("publish: %w", err)
}

// Streaming
stream, err := client.StreamMessages(ctx, &busboyv1.StreamRequest{
    Topic: "events.system",
})
if err != nil {
    return fmt.Errorf("stream: %w", err)
}

for {
    msg, err := stream.Recv()
    if err == io.EOF {
        break
    }
    if err != nil {
        return fmt.Errorf("recv: %w", err)
    }
    // Handle msg
}
```

## Testing gRPC

### Unit Tests with Mock
```go
func TestPublish_Success(t *testing.T) {
    // Create mock
    ctrl := gomock.NewController(t)
    mockBus := NewMockMessageBus(ctrl)

    // Set expectations
    mockBus.EXPECT().
        Publish(gomock.Any(), "test.topic", []byte("payload")).
        Return(&Message{ID: "123", Seq: 1}, nil)

    // Create server
    server := &busboyServer{bus: mockBus}

    // Call
    resp, err := server.Publish(context.Background(), &busboyv1.PublishRequest{
        Topic:   "test.topic",
        Payload: []byte("payload"),
    })

    // Assert
    require.NoError(t, err)
    assert.Equal(t, "123", resp.MessageId)
}
```

### Integration Tests with bufconn
```go
func TestIntegration_PublishAndStream(t *testing.T) {
    // In-memory connection
    lis := bufconn.Listen(1024 * 1024)

    // Start server
    s := grpc.NewServer()
    busboyv1.RegisterBusboyServiceServer(s, newTestServer())
    go s.Serve(lis)
    defer s.Stop()

    // Create client
    conn, _ := grpc.Dial("bufnet",
        grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) {
            return lis.Dial()
        }),
        grpc.WithInsecure(),
    )
    defer conn.Close()

    client := busboyv1.NewBusboyServiceClient(conn)

    // Test publish + stream flow
    // ...
}
```

## Buf Configuration

```yaml
# buf.yaml
version: v1
name: buf.build/unheaded/unheaded
deps:
  - buf.build/googleapis/googleapis
lint:
  use:
    - DEFAULT
  except:
    - PACKAGE_VERSION_SUFFIX
breaking:
  use:
    - FILE
```

```yaml
# buf.gen.yaml
version: v1
plugins:
  - plugin: buf.build/protocolbuffers/go
    out: gen/proto
    opt: paths=source_relative
  - plugin: buf.build/grpc/go
    out: gen/proto
    opt: paths=source_relative
```

## Generation Commands

```bash
# Install buf
go install github.com/bufbuild/buf/cmd/buf@latest

# Lint protos
buf lint

# Generate Go code
buf generate

# Check for breaking changes
buf breaking --against '.git#branch=main'
```
