# Zhen Demo Questions

Curated questions for demonstrating Zhen's RAG capabilities against the Unheaded codebase (385K LOC, 21K+ vectors).

---

## Getting Started

1. **What is Unheaded?**
   _Expected: Configuration management automation platform, "configuration management automation platform."_

2. **What services does Unheaded provide?**
   _Expected: Lists 10 services — timeguru, captain, architect, micromanager, monad, sophia, dashboard-backend, kanban-app, wotan, unheaded-daemon._

3. **What is the Meta Moment?**
   _Expected: Self-hosting validation — Unheaded building and hosting its own development infrastructure._

---

## Architecture

4. **What are the 6 layers of the Unheaded architecture?**
   _Expected: Layer 0 (Infrastructure) through Layer 5 (User Interface)._

5. **How does service discovery work?**
   _Expected: Four-layer approach — Wotan registration, port scan, convention scan, static fallback._

6. **What is the gRPC-first transport strategy?**
   _Expected: Primary gRPC streaming on port 18001, fallback cascade HTTP/3 -> HTTP/2 -> HTTP/1.1._

---

## Infrastructure

7. **What port does Wotan use?**
   _Expected: HTTP 18000, gRPC 18001._

8. **What is the Doom Range?**
   _Expected: Port range 16666-26666 for all Unheaded services, avoids conflicts with standard dev tools._

9. **How does the network design work?**
   _Expected: Bridge lxdbr0 (10.10.10.0/24), gateway at 10.10.10.100, Wotan at 10.10.10.10._

---

## Observability

10. **How does eBPF tracing work in Unheaded?**
    _Expected: Aya (Rust) for kernel programs, cilium/ebpf for Go userspace. Traces from packet zero — XDP packet markers, connection tracking, latency probes._

11. **What observability backends does Unheaded support?**
    _Expected: Prometheus, Grafana, ELK, Fluentd, Jaeger, Nagios — all interchangeable._

12. **How does log aggregation work?**
    _Expected: pkg/logagg/ with ring buffer (10K entries), zerolog hook, Wotan topic logs.<service>.<level>._

---

## Protocol

13. **What is the Monad wire format size?**
    _Expected: 20 bytes, frozen at version 0x01._

14. **What is the S67 Foundation spec?**
    _Expected: draft-05 with 12 IANA registries, Monad flags bitfield (C|Y|T|E|S|M|CUST|R), 13 flow actions._

15. **What is the Monad service?**
    _Expected: Unified state management on port 19004, gRPC protocol._

---

## Advanced

16. **How does the authentication framework work?**
    _Expected: Pluggable authenticators — Noop, APIKey, JWT. RBAC authorizer. 64 test cases, 100% coverage._

17. **What is the Kingdom health monitoring system?**
    _Expected: Percentage-based consensus severity — OK/WARN/ERROR/CRITICAL/PANIC based on % of services reporting failures._

18. **What is the AF_XDP pipeline performance?**
    _Expected: 920K packets per second validated, Rust FFI + Go bridge._

---

**Usage**: Click the suggested questions in the Zhen UI, or paste these into the input box. Each answer draws from the indexed codebase with source attribution.
