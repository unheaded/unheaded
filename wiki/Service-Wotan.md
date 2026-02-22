# Wotan — Message Bus / Ring Buffer / Protocol RAM

The backbone of the Kingdom. Wotan serves a triple role:

1. **High-Speed Ring Buffer** — Lock-free eBPF perf_event pattern
2. **Event Bus** — Pub/sub gRPC/HTTP with topic routing
3. **Protocol RAM** — BPF map memory substrate for Monad compute

## Cluster Model

DNS-like hierarchy with subscription mirroring:
- Root → Regional → Edge Wotans
- BGP-style topic advertisement
- n+1 availability, not consensus
- Scale: 1 (dev) → 1000 (global)

## Endpoints

| Endpoint | Purpose |
|----------|---------|
| `GET /health` | Health check |
| `GET /ready` | Readiness probe |
| `GET /metrics` | Prometheus metrics |
| gRPC `:9090` | Message bus interface |

---

> **Source:** [github.com/unheaded/wotan](https://github.com/unheaded/wotan)
