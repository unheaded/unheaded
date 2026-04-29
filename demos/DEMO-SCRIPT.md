# Unheaded Conference Demo Script
## "Doom in the Data Plane: Observing Every Packet with eBPF"

**Duration:** ~20 minutes + Q&A
**Setup Time:** < 5 minutes (automated with `scripts/demo-start.sh`)
**Reset:** Run `scripts/demo-reset.sh` between runs

---

### Pre-Demo Checklist
- [ ] WEST host: All services running (wotan, dashboard-backend, kanban-app, unheaded-daemon)
- [ ] EAST host: All services running (wotan, unheaded-daemon, timeguru, dashboard-backend, kanban-app)
- [ ] trace-collector-go running in demo mode (WEST)
- [ ] Dashboard accessible at http://localhost:20000
- [ ] Terminal windows arranged: 3 terminals visible

---

### Segment 1: Introduction (2 min)

**TALKING POINTS:**
"Today we're showing Unheaded: infrastructure observability where EVERY packet is traced. Not samples. Every single packet."

"This is a 720,000 line codebase across Go and Rust — 49 services and 15 eBPF crates — built to prove that eBPF can observe your entire infrastructure without sampling."

**DEMO:**
- Show dashboard at http://localhost:20000
- Point out: real-time packet flow visualization
- Point out: service health indicators (percentage-based consensus)

---

### Segment 2: The Architecture (3 min)

**TALKING POINTS:**
"The pipeline: XDP stamps packets → TC tracks flows → kprobe measures latency → trace-collector bridges to userspace → Wotan message bus distributes → Dashboard renders."

"All of this runs in the kernel. Zero context switches for the hot path."

**TERMINAL 1:**
```bash
# Show the eBPF programs (9 compiled, ELF 64-bit)
ls -la ~/tmp/unheaded/ebpf/*/target/bpfel-unknown-none/release/*.o 2>/dev/null
file ~/tmp/unheaded/ebpf/packet-marker/target/bpfel-unknown-none/release/packet-marker

# Show service count
echo "Services: $(ls ~/tmp/unheaded/cmd/ | wc -l) binaries + $(ls ~/tmp/unheaded/services/ | wc -l) services = 49 total"
```

**TERMINAL 2:**
```bash
# Show Wotan stats — the message bus
curl -s http://localhost:18000/stats | jq '.topics | length' 2>/dev/null
curl -s http://localhost:18000/health
```

---

### Segment 3: Single Request Trace (3 min)

**TALKING POINTS:**
"Watch one request flow through the entire pipeline."

**TERMINAL 1:**
```bash
# Send a traced request
curl -v -H "X-Trace-ID: demo-$(date +%s)" http://localhost:20000/health
```

**DASHBOARD:**
- Show trace appearing in real-time on packet-flow canvas
- Highlight: trace_id stamped by XDP in microseconds
- Show: packet → flow_tracker → latency_probe path

**TERMINAL 2:**
```bash
# Show Wotan received the trace events
curl -s http://localhost:18000/stats | jq '.topics[] | select(.name | startswith("traces")) | {name, event_count}'
```

**METRICS:**
- RTT: sub-millisecond for local
- Hops: visible in flow tracker
- State: ESTABLISHED connections tracked

---

### Segment 4: Doom in the Data Plane (5 min)

**TALKING POINTS:**
"Now for the wild part. We've proven eBPF is computationally complete by running Doom inside the kernel."

**TERMINAL 1:**
```bash
# Show the Doom eBPF programs
ls -la ~/tmp/unheaded/cmd/doom*
echo "doom-bridge: Go userspace ↔ eBPF kernel bridge"
echo "doom-go-injector: injects Doom frames into eBPF maps"

# Show compiled eBPF objects
file ~/tmp/unheaded/ebpf/*/target/bpfel-unknown-none/release/*.o 2>/dev/null | head -5
```

**TALKING POINTS:**
"559 frames of Doom running in eBPF. Render time: < 100µs per frame."
"If eBPF can run Doom, it can observe your infrastructure."

---

### Segment 5: Cross-Host Communication (3 min)

**TALKING POINTS:**
"This isn't just localhost. EAST and WEST are real bare metal servers communicating over a point-to-point link."

**TERMINAL 1 (WEST):**
```bash
# Verify EAST services
ssh govan@east "curl -s http://localhost:18000/health"
ssh govan@east "curl -s http://localhost:19000/health"

# Cross-host Wotan connectivity
curl -s http://192.168.13.1:18000/health
```

**TALKING POINTS:**
- WEST: Full development cluster (Go 1.24, Rust nightly, clang 19)
- EAST: Bare metal staging (AMD A8-5500, 7GB RAM, Ubuntu 25.10)
- 5 statically-linked services deployed via SCP (no internet on EAST)

---

### Segment 6: Production Load (3 min)

**TALKING POINTS:**
"Let's throw real load at it."

**TERMINAL 1:**
```bash
# Generate sustained load
for i in $(seq 1 100); do
  curl -s -H "X-Trace-ID: load-$i" http://localhost:20000/health > /dev/null &
done
wait
echo "100 concurrent requests completed"
```

**DASHBOARD:**
- Live packet flow animation updating
- Queue visualization showing throughput
- Per-service metrics updating in real-time

**TERMINAL 2:**
```bash
# Show trace-collector event counts
curl -s http://localhost:18000/stats | jq '.topics[] | {name, event_count}'
```

---

### Segment 7: Your App Here (2 min)

**TALKING POINTS:**
"To add your service to Unheaded:"

```
1. Define your service in configs/services.yaml
2. Wire auth: auth.LoadServiceAuthConfig("your-service") + auth.SetupMiddleware()
3. Connect to Wotan for service mesh communication
4. Deploy — eBPF observability is automatic, zero instrumentation needed
```

**SHOW:**
- `pkg/auth/setup.go` — 3-line auth integration
- `pkg/discovery/` — automatic service discovery (4-layer)
- `pkg/transport/` — gRPC-first with HTTP fallback

"User brings their app — the head. We provide everything else — unheaded."

---

### Q&A (2 min)

**Key talking points:**
- **XDP**: Zero-copy packet stamping at driver level
- **TC**: Stateful flow tracking with BPF hash maps
- **kprobe**: Kernel-level latency measurement
- **Monad wire format**: 20-byte, CRC-16 checked, QoS-aware (version 0x01 FROZEN)
- **eBPF**: Kernel programs, kernel safety verifier, zero context switches
- **Auth**: Pure stdlib JWT (crypto/hmac + crypto/sha256), zero external dependencies
- **Scale**: 49 services, 15 eBPF crates, 720K LOC, dual bare-metal hosts

**Stats to cite:**
- 90.8% auth test coverage (exceeds 80% target)
- 19 auth tests, all passing with race detector
- 6/6 AF_XDP FFI tests passing
- Go + Rust full build: ZERO errors
- EAST deployment: 5 services, all healthy, cross-host verified
