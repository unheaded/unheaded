# Busboy Protocol Specification (BP/1)

## 0. Scope and Status

This document specifies the Busboy Protocol, version **BP/1**. BP/1 defines:

- REST control plane semantics
- gRPC data plane streaming semantics
- Subscription and message state machines
- Error conditions and response structures

**Status**: Alpha. Interfaces and wire formats may evolve. Backward-incompatible changes will increment the protocol version.

## 1. Conventions and Terminology

### 1.1 Normative Language

The key words **MUST**, **MUST NOT**, **REQUIRED**, **SHALL**, **SHALL NOT**, **SHOULD**, **SHOULD NOT**, **RECOMMENDED**, **MAY**, and **OPTIONAL** are interpreted as described in RFC 2119.

### 1.2 Terms

| Term | Definition |
|------|------------|
| Server | A BP/1 implementation providing REST and gRPC interfaces |
| Client | Any consumer of the REST and/or gRPC interfaces |
| Control Plane | REST endpoints for operational actions |
| Data Plane | gRPC streaming for message delivery |
| Topic | An isolated namespace for messages |
| Subscriber | A client identity with access to a topic |

### 1.3 Time and Encoding

- Timestamps MUST be RFC 3339 strings (e.g., `2026-01-26T18:04:05Z`)
- Request/response bodies MUST be UTF-8 JSON
- Identifiers MUST be string-encoded UUIDs (v4 RECOMMENDED)

## 2. Transport and Security

### 2.1 TLS

Servers SHOULD support TLS 1.3 for both REST and gRPC transports. Production deployments MUST use TLS.

### 2.2 Authentication

BP/1 specifies authorization semantics but does not require a specific authentication mechanism. Implementations MAY employ mTLS, bearer tokens, or operator-managed identity mapping.

## 3. Data Model

### 3.1 Message

```json
{
    "message_id": "uuid",
    "topic": "string",
    "sender_id": "uuid",
    "created_at": "RFC3339",
    "seq": 12345,
    "payload": "string",
    "deleted": false
}
```

Requirements:

- `message_id`, `topic`, `sender_id`, `created_at`, `seq`, and `deleted` MUST be present
- `seq` MUST be monotonically increasing per topic
- For `deleted=true`, `payload` SHOULD be empty

### 3.2 Ring Buffer Retention

Servers MUST implement per-topic fixed-capacity retention:

- Capacity MUST be configured at startup
- When capacity is exceeded, oldest messages are overwritten
- Sequence numbers MUST NOT reset on wrap

### 3.3 Subscriber

```json
{
    "subscriber_id": "uuid",
    "topic": "string",
    "display_name": "string",
    "status": "pending|approved|denied|revoked",
    "requested_at": "RFC3339",
    "decided_at": "RFC3339",
    "decided_by": "string"
}
```

Fields `decided_at` and `decided_by` MAY be absent until a decision occurs.

## 4. State Machines

### 4.1 Subscription State Machine

States:

| State | Description |
|-------|-------------|
| `PENDING` | Request received, not yet approved |
| `APPROVED` | May read and write to topic |
| `DENIED` | Explicitly refused |
| `REVOKED` | Previously approved, later removed |

Transitions:

- `PENDING` -> `APPROVED` (admin action)
- `PENDING` -> `DENIED` (admin action)
- `APPROVED` -> `REVOKED` (admin action)

Invariants:

- Subscribers in `PENDING`, `DENIED`, or `REVOKED` MUST NOT publish messages
- Subscribers in `PENDING`, `DENIED`, or `REVOKED` MUST NOT receive stream messages

### 4.2 Message Lifecycle

States:

| State | Description |
|-------|-------------|
| `ACTIVE` | Message available for delivery |
| `DELETED` | Tombstoned |

Transitions:

- `ACTIVE` -> `DELETED` via delete operation

Invariants:

- Only the original sender MAY delete a message
- After deletion, servers MUST NOT deliver the original payload

## 5. Error Model

### 5.1 Error Envelope

Non-2xx responses MUST return:

```json
{
    "error": {
        "code": "string",
        "message": "string",
        "details": {}
    }
}
```

### 5.2 HTTP Status Codes

| Code | Meaning |
|------|---------|
| 400 | Malformed request |
| 401 | Authentication required |
| 403 | Not authorized |
| 404 | Resource not found |
| 409 | State conflict |
| 422 | Validation failure |
| 429 | Rate limit exceeded |
| 500 | Internal error |
| 503 | Service unavailable |

### 5.3 Error Codes

| Code | Description |
|------|-------------|
| `INVALID_REQUEST` | Malformed input |
| `INVALID_TOPIC` | Unknown topic |
| `INVALID_SUBSCRIBER` | Unknown subscriber |
| `SUBSCRIPTION_PENDING` | Subscriber not yet approved |
| `SUBSCRIPTION_DENIED` | Subscriber denied |
| `SUBSCRIPTION_REVOKED` | Subscriber revoked |
| `MESSAGE_NOT_FOUND` | Unknown message |
| `NOT_MESSAGE_OWNER` | Cannot delete others' messages |
| `RATE_LIMITED` | Too many requests |
| `INTERNAL` | Server error |

## 6. REST Control Plane

### 6.0 Versioning

Endpoints SHOULD be rooted under `/api/v1/`.

### 6.1 Subscribe to Topic

`POST /api/v1/topics/{topic}/subscribe`

Request:

```json
{"display_name": "string"}
```

Response `201 Created`:

```json
{
    "subscriber": {
        "subscriber_id": "uuid",
        "topic": "string",
        "display_name": "string",
        "status": "pending",
        "requested_at": "RFC3339"
    }
}
```

### 6.2 List Pending Subscriptions (Admin)

`GET /api/v1/admin/pending?topic={topic}`

Response `200 OK`:

```json
{
    "pending": [
        {
            "subscriber_id": "uuid",
            "topic": "string",
            "display_name": "string",
            "requested_at": "RFC3339"
        }
    ]
}
```

### 6.3 Publish Message

`POST /api/v1/topics/{topic}/publish`

Request:

```json
{
    "subscriber_id": "uuid",
    "payload": "string"
}
```

Response `201 Created`:

```json
{"message": {}}
```

Validation:

- `payload` MUST be non-empty
- `payload` SHOULD be <= 64KB (implementation-defined limit)
- `subscriber_id` MUST reference an `APPROVED` subscriber

### 6.4 Get Messages

`GET /api/v1/topics/{topic}/messages?after_seq={n}&limit={k}`

Response `200 OK`:

```json
{
    "topic": "string",
    "messages": [],
    "next_after_seq": 12345
}
```

Authorization:

- Caller MUST be `APPROVED` for the topic

### 6.5 Delete Message

`POST /api/v1/topics/{topic}/messages/{message_id}/delete`

Request:

```json
{"subscriber_id": "uuid"}
```

Response `200 OK`:

```json
{
    "deleted": true,
    "message_id": "uuid",
    "topic": "string"
}
```

Authorization:

- Server MUST verify message ownership
- Non-owners receive `403 NOT_MESSAGE_OWNER`

### 6.6 Approve Subscription (Admin)

`POST /api/v1/admin/approve`

Request:

```json
{
    "subscriber_id": "uuid",
    "approved_by": "string"
}
```

Response `200 OK`:

```json
{"subscriber": {}}
```

### 6.7 Deny Subscription (Admin)

`POST /api/v1/admin/deny`

Request:

```json
{
    "subscriber_id": "uuid",
    "denied_by": "string"
}
```

Response `200 OK`:

```json
{"subscriber": {}}
```

## 7. gRPC Data Plane

### 7.1 Service Definition

```protobuf
service BusboyStream {
    rpc StreamMessages(StreamRequest) returns (stream StreamEvent);
    rpc Ping(PingRequest) returns (PingResponse);
}

message StreamRequest {
    string topic = 1;
    string subscriber_id = 2;
    int64 after_seq = 3;
}

message StreamEvent {
    oneof event {
        MessageEvent message = 1;
        TombstoneEvent tombstone = 2;
        ControlEvent control = 3;
    }
}

message MessageEvent {
    string message_id = 1;
    string topic = 2;
    string sender_id = 3;
    int64 seq = 4;
    int64 created_unix_ms = 5;
    bytes payload = 6;
}

message TombstoneEvent {
    string message_id = 1;
    string topic = 2;
    int64 seq = 3;
}

message ControlEvent {
    string type = 1;
    string code = 2;
    string message = 3;
}

message PingRequest {}

message PingResponse {
    string status = 1;
    int64 timestamp = 2;
}
```

### 7.2 Stream Authorization

Before delivering messages:

- Server MUST verify subscriber is `APPROVED`
- Unauthorized requests receive `PERMISSION_DENIED`

### 7.3 Resumption

Clients SHOULD resume after disconnect using `after_seq`:

- If `after_seq` refers to overwritten history, server MAY:
  - Start from earliest available (indicate truncation)
  - Fail with `FAILED_PRECONDITION`

### 7.4 Delivery Guarantees

BP/1 provides:

- At-most-once delivery per stream session
- No durability guarantees
- Clients MUST tolerate message loss from buffer overwrites

## 8. Rate Limiting

- Servers MAY apply token-bucket rate limiting
- Servers SHOULD bound stream buffers
- Slow consumers MAY be disconnected with `RESOURCE_EXHAUSTED`

## 9. Conformance

A conforming BP/1 server MUST:

- Enforce subscription gating for publish/read/stream
- Provide per-topic monotonic sequence numbers
- Implement fixed-capacity per-topic retention
- Implement owner-only message deletion
- Return errors using the envelope in Section 5.1

A conforming BP/1 client SHOULD:

- Handle `pending` subscription status
- Reconnect and resume using `after_seq`
- Tolerate truncation and message loss
