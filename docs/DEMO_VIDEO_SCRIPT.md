# Unheaded Demo Video Script

**Duration**: 10 minutes
**Format**: Narrated screencast with live UI interaction
**Audience**: Engineers, investors, open-source contributors
**Resolution**: 1080p60 minimum

---

## Pre-Recording Checklist

- [ ] All services running (`docker compose up -d` or bare metal)
- [ ] Dashboard accessible at localhost:20000
- [ ] Zhen web UI accessible at localhost:20103
- [ ] Kanban app accessible at localhost:20001
- [ ] Terminal with dark theme, large font (14pt+)
- [ ] Browser zoom at 110-125% for readability
- [ ] Demo data enabled if running without live traffic

---

## Scene 1: Introduction (0:00 - 1:00)

**Visual**: Terminal showing `curl localhost:17000/health` returning healthy status.

**Script**:

> Unheaded is a configuration management automation platform.
>
> The name comes from a simple idea: you bring your application -- the head -- and we provide everything else. Networking, observability, security, service mesh, deployment. Production-ready infrastructure in hours, not months.
>
> What you are about to see is a live system. Two bare-metal hosts connected via WireGuard, running 25 services, with eBPF tracing every packet from wire to browser.
>
> Let me show you what that looks like.

**Action**: Health check loop across all service ports.

```bash
for port in 17000 18000 19000 19001 19002 19003 19004 19005 20000 20001; do
  curl -sf "http://localhost:$port/health"
done
```

---

## Scene 2: Dashboard Tour (1:00 - 3:00)

**Visual**: Browser at localhost:20000.

**Script**:

> This is the Unheaded dashboard. Every metric you see here comes from real eBPF traces and Prometheus scrapes -- no synthetic data.

**Walk through each tab**:

1. **Service Health** (15s): Show the health grid. Point out percentage-based consensus severity (green/yellow/orange/red). Explain that severity is calculated from how many dependent services report a problem, not from a single check.

2. **Packet Flow** (30s): Show the packet-flow visualization. Highlight trace IDs propagating through services. Explain: "Every packet gets a trace ID injected at the XDP layer. By the time it reaches your browser, we know every hop it took."

3. **Metrics** (15s): CPU, memory, network, disk. Standard Prometheus metrics exposed by every service.

4. **Kanban Board** (30s): Switch to the Kanban app (localhost:20001). Show tasks moving between columns. Explain: "This is the meta moment -- Unheaded tracking its own development. Every request to this board is traced by the same eBPF pipeline that would trace your application."

---

## Scene 3: Zhen AI Demo (3:00 - 5:00)

**Visual**: Browser at localhost:20103.

**Script**:

> Unheaded includes a local AI assistant called Zhen. It runs Mistral-7B locally via llama.cpp with a RAG pipeline over 1.52 million indexed knowledge chunks. No data leaves the network.

**Demo queries**:

1. **Ask**: "What is Unheaded?"
   - Wait for response. Point out that the answer references actual codebase files and documentation.

2. **Ask**: "How does the Monad wire format work?"
   - Wait for response. Highlight the technical accuracy -- register layout, CRC-16, HbH encoding.

3. **Show Search tab**: Type "eBPF XDP" and show the indexed results. Explain: "This is not a web search. These are results from our own codebase, documentation, and protocol specifications."

**Script**:

> Zhen is not a chatbot bolted onto the side. It is a service in the mesh, traced by eBPF, communicating over Wotan like everything else.

---

## Scene 4: The Unheaded Protocol Computer (5:00 - 7:00)

**Visual**: Terminal, then browser.

**Script**:

> Now for the part that makes Unheaded different from every other infrastructure platform.
>
> The Monad wire format is not just metadata. It is a 20-byte register file -- five 32-bit registers -- embedded in every IPv6 packet. At each network hop, an eBPF program reads and writes those registers. The packet itself is the working memory of a distributed computation.
>
> We took that idea to its logical conclusion. We built a complete virtual CPU inside eBPF XDP.

**Show**:

1. **MiniKernel boot sequence** (30s): Show the UPC booting. Explain the 6-level Dream Ladder: register file, ALU, memory, I/O, interrupts, and finally the arch/mbc Linux port.

2. **DOOM running on the protocol** (30s): If available, show DOOM rendering frames through the Monad register pipeline. Explain: "This is a computational completeness proof. If we can run DOOM, we can run anything."

3. **Architecture diagram** (30s): Show the UPC Reference Manual. Walk through the register layout: R0 (accumulator), R1-R4 (general purpose), CRC-16 integrity. Explain Kingdom Mode: "In a controlled EVPN-VXLAN domain, we reclaim deterministic IPv6 address bits as extended register space. A /16 deployment carries 48 bytes of computational state per packet with zero wire overhead."

**Script**:

> This is not a theoretical exercise. The protocol IS a computer. Every packet in the network is executing a program.

---

## Scene 5: Security (7:00 - 9:00)

**Visual**: Terminal showing PQC test output, then architecture diagrams.

**Script**:

> Security is not a feature we added. It is the architecture.

**Show**:

1. **Post-quantum cryptography** (40s): Run the PQC test suite.
   ```bash
   go test ./pkg/crypto/pqc/... -v -run TestPQC
   ```
   Explain: "ML-KEM for key exchange, ML-DSA for signatures, SLH-DSA for stateless hashing. FIPS 203, 204, and 205. These are not experimental -- they are the NIST post-quantum standards, integrated into Sophia's key store."

2. **eBPF tracing** (40s): Show a packet being traced from XDP entry to service delivery.
   Explain: "Every packet gets a trace ID at the first eBPF program it hits. Connection state, latency, flow -- all tracked in kernel space. By the time a request reaches your application, we have a complete audit trail."

3. **Compliance tiers** (40s): Show the 4-tier model.
   - NONE: Development, no enforcement
   - STANDARD: TLS 1.3, API keys, audit logging
   - HARDENED: mTLS, JWT federation, RBAC, seccomp profiles
   - SOVEREIGN: Post-quantum crypto, air-gapped deployment, full SIEM integration

   Explain: "You choose the tier. The platform enforces it. No half-measures."

---

## Scene 6: Closing (9:00 - 10:00)

**Visual**: Terminal, then README on GitHub.

**Script**:

> Unheaded is GPL-3.0. Free forever. Protocol specifications are dual-licensed GPL-3.0 and Apache-2.0 so the ecosystem can build on them without restriction.
>
> The codebase is roughly 450,000 lines of production code. Go for services, Rust for eBPF programs, vanilla JavaScript for the frontend. 25 services, 37 packages, 8 eBPF programs, three deployment platforms, four routing options.

**Show quick start**:

```bash
git clone https://github.com/unheaded/unheaded
cd unheaded
docker compose up -d
xdg-open http://localhost:20000
```

**Script**:

> That is four commands from clone to dashboard.
>
> You bring the app. We provide everything else.
>
> Documentation, protocol specs, and the full codebase are on GitHub. Thank you for watching.

**End card**: Project URL, author contact, GPL-3.0 badge.

---

## Post-Production Notes

- Add chapter markers at each scene transition for YouTube
- Include terminal command overlays (semi-transparent, bottom of screen)
- Background music: ambient/electronic, low volume, no vocals
- Export at 1080p60 minimum, 4K preferred
- Thumbnail: Dashboard screenshot with "Production-Ready Infrastructure" text overlay
