# Wotan Topic Pub/Sub

Wotan provides a lightweight topic-based publish/subscribe system for inter-service communication within the Unheaded Kingdom.

## Quick Start

```bash
# Subscribe to a topic
curl -X POST http://localhost:18000/api/v1/topics/alerts.critical/subscribe \
  -H 'Content-Type: application/json' \
  -d '{"display_name": "timeguru"}'

# Publish to a topic (use subscriber_id from subscribe response)
curl -X POST http://localhost:18000/api/v1/topics/alerts.critical/publish \
  -H 'Content-Type: application/json' \
  -d '{"subscriber_id": "<uuid>", "payload": "CPU threshold exceeded"}'

# Read messages from a topic
curl http://localhost:18000/api/v1/topics/alerts.critical/messages

# List all topics
curl http://localhost:18000/api/v1/topics
```

## Subscriber Auto-Approval

Not all subscribers are automatically approved. Wotan uses a **display name allowlist** to control which services can self-approve on subscribe.

### Configuration

The allowlist is defined in `configs/wotan.yaml`:

```yaml
topics:
  auto_approve:
    - "service"          # default display_name for internal services
    - "timeguru"
    - "captain"
    - "architect"
    - "micromanager"
    - "monad"
    - "sophia"
    - "dashboard"
    - "kanban"
    - "daemon"
    - "trace-collector"
    - "wiki"
```

**Behavior:**
- Subscribers whose `display_name` matches an entry in `auto_approve` are approved immediately (status: `"approved"`).
- All other subscribers are created with status `"pending"` and must be manually approved via the admin API.
- Use `"*"` to auto-approve all subscribers (development only — not recommended for production).
- If no config file exists, all subscribers default to `"pending"`.
- Matching is **case-insensitive**.

### Loading the Config

```bash
# Via command-line flag
wotan --topic-config configs/wotan.yaml

# Via environment variable (overrides flag)
WOTAN_TOPIC_CONFIG=configs/wotan.yaml wotan
```

Default path: `configs/wotan.yaml`

## Manual Approval

Subscribers not in the auto-approve list must be approved by an admin:

```bash
# List pending members
curl http://localhost:18000/api/v1/admin/pending

# Approve a specific member
curl -X POST http://localhost:18000/api/v1/admin/approve \
  -H 'Content-Type: application/json' \
  -d '{"member_id": "<uuid>"}'

# Approve all pending members
curl -X POST http://localhost:18000/api/v1/admin/approve-all
```

## API Reference

### POST /api/v1/topics/{topic}/subscribe

Subscribe to a topic. Returns a subscriber object with status.

**Request:**
```json
{
  "display_name": "timeguru"
}
```

**Response (auto-approved):**
```json
{
  "subscriber": {
    "subscriber_id": "2c208d68-ab22-4535-b5e9-75dfb794486b",
    "topic": "alerts.critical",
    "display_name": "timeguru",
    "status": "approved",
    "requested_at": "2026-03-13T10:00:00Z"
  }
}
```

**Response (pending approval):**
```json
{
  "subscriber": {
    "subscriber_id": "a1b2c3d4-...",
    "topic": "alerts.critical",
    "display_name": "unknown-service",
    "status": "pending",
    "requested_at": "2026-03-13T10:00:00Z"
  }
}
```

### POST /api/v1/topics/{topic}/publish

Publish a message to a topic. Requires an approved subscriber ID.

**Request:**
```json
{
  "subscriber_id": "2c208d68-ab22-4535-b5e9-75dfb794486b",
  "payload": "Hello from the Kingdom!"
}
```

**Response:**
```json
{
  "message_id": "f5e6d7c8-...",
  "topic": "alerts.critical",
  "seq": 42,
  "timestamp": "2026-03-13T10:00:01Z"
}
```

**Errors:**
- `400` — Missing or invalid `subscriber_id` / `payload`, or `{topic}` is a pattern
- `403` — Subscriber not approved (still pending)

### GET /api/v1/topics/{topic}/messages

Retrieve messages from a topic's ring buffer, oldest first. `{topic}` may be
a pattern (see [Patterns](#patterns)); the response then merges every
matching topic, and each message's `topic` is the topic it was published to.

**Query:** `after_seq` (default 0) returns messages with `seq` greater than
it; `limit` (default and maximum 1000) caps the page. Bad values are `400`.

**Response:**
```json
{
  "messages": [
    {"message_id": "...", "topic": "alerts.critical", "seq": 43, "payload": "...", "created_at": "..."}
  ],
  "last_seq": 57
}
```

To follow a topic or pattern, pass the highest `seq` received as the next
`after_seq`. `last_seq` is the newest seq Wotan holds (for a single topic,
that topic's newest; for a pattern, Wotan's newest). Seqs live in memory
and start again at 1 when Wotan restarts, so `last_seq` below your cursor
means Wotan restarted: start again from 0.

### GET /api/v1/topics

List all active topics with message counts.

## Topic Naming

Topics use dot-separated hierarchical names:

- `alerts.critical` — Critical system alerts
- `logs.timeguru.info` — Timeguru info-level logs
- `system.discovery` — Service discovery events

**Allowed characters:** `a-z`, `A-Z`, `0-9`, `.`, `-`, `_`, and `*`, `#` in patterns

### Sequence numbers

Every message gets a `seq` from one counter shared by all topics. Within a
topic seqs strictly increase (with gaps); across topics they give one order,
which is what lets a single cursor follow a pattern.

### Patterns

Subscribing and reading accept a pattern; publishing does not (`400`).

| pattern | matches |
|---|---|
| `alerts.*` | `alerts.critical`, `alerts.warn` (exactly one segment) |
| `ebpf.*.events` | `ebpf.flow.events`, `ebpf.packet.events` |
| `logs.#` | `logs.sophia.info`, `logs.a.b.c` (one or more trailing segments) |
| `*` or `#` alone | every topic |

`#` starts a URL fragment, so send it as `%23` (`/api/v1/topics/logs.%23/messages`).
Subscribing to a pattern creates no topic. gRPC `StreamTopics` takes the same
patterns and its `since_seq` works the same way.

## Adding a New Service

To allow a new service to auto-approve on Wotan topics:

1. Add its `display_name` to `configs/wotan.yaml` under `topics.auto_approve`
2. Restart Wotan (or send SIGHUP for hot-reload — future feature)
3. The service can now subscribe and publish immediately

## Security Considerations

> **TODO(BlackMage):** The current auto-approval mechanism relies on self-reported `display_name`, which is untrusted. A malicious service could claim any display name. Phase 2 will tie auto-approval to verified API keys or mTLS client certificates for defense in depth.
