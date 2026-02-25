# Doom in the Data Plane: Running a Game Engine Inside eBPF

**Duration:** 30 minutes talk + 10 minutes Q&A
**Speaker:** Stevie Bellis (stevie@bellis.tech)
**Target audience:** Infrastructure engineers, SREs, eBPF developers, systems programmers
**Conference tracks:** Networking, Observability, eBPF/kernel, or Unconventional Computing

---

## Abstract

We ran Doom -- the 1993 id Software classic -- inside Linux eBPF. Not as a joke. As proof that IPv6 extension headers can carry arbitrary computation at wire speed. 559 frames rendered. 819 million instructions executed. Zero halts. If a game engine works in the data plane, packet-level observability is trivially achievable. This talk covers the architecture, the bugs that almost killed the project, and what this means for the future of network observability.

---

## Talk Structure

### Act 1: The Setup (5 minutes)
**Goal:** Establish the problem and the audacious claim.

### Act 2: The Architecture (8 minutes)
**Goal:** Explain how packets become computation.

### Act 3: The Journey (8 minutes)
**Goal:** Tell the story of making it work -- the bugs, the breakthroughs.

### Act 4: The Proof (4 minutes)
**Goal:** Show the results, hard numbers.

### Act 5: The Implication (5 minutes)
**Goal:** Connect Doom to the real product. Call to action.

---

## Slide-by-Slide Outline

### Slide 1: Title
**Content:** "Doom in the Data Plane: Running a Game Engine Inside eBPF"
**Speaker notes:** Introduce yourself. One sentence: "33 days from first commit to Doom running inside eBPF."

### Slide 2: The Problem
**Content:** Diagram showing sidecar proxy overhead in service meshes (Envoy, Istio). Arrow pointing at latency cost.
**Speaker notes:** "Current packet observability adds 1-5ms per hop. Sidecar proxies double your tail latency. eBPF is fast but hard to program beyond simple counters."

### Slide 3: The Audacious Claim
**Content:** "What if every packet could carry its own compute environment?"
**Speaker notes:** "What if instead of inspecting packets at sidecars, the packet itself carried a register file that eBPF programs read and wrote at every hop? Zero sidecar overhead. Wire-speed metadata."

### Slide 4: The Monad Protocol
**Content:** Diagram of IPv6 packet with Hop-by-Hop extension header highlighted. 20-byte register file.
**Speaker notes:** "This is the Monad protocol. 20 bytes of metadata in every packet, readable at every hop by XDP programs in under 320 nanoseconds."

### Slide 5: How Do You Prove It?
**Content:** Doom logo. "If it can run Doom, it can trace packets."
**Speaker notes:** "To prove the protocol supports arbitrary computation -- not just tagging -- we did what engineers have done since 1993: we ran Doom."

### Slide 6: The Compilation Pipeline
**Content:** Flow diagram: Doom C -> RISC-V ELF -> MBC bytecode -> BPF maps -> XDP execution
**Speaker notes:** "Cross-compile Doom to RISC-V. Translate RISC-V to our custom MBC bytecode ISA. Load bytecode into BPF maps. Execute instruction-by-instruction as packets bounce through XDP."

### Slide 7: The Packet Circulation Ring
**Content:** Diagram of 6 Linux namespaces (monad0-5) connected in a ring. Packet bouncing between them.
**Speaker notes:** "Six namespaces, each with the same XDP program. Packet enters, executes 128 instructions, bounces to the next. 255 bounces per packet. 32,640 instructions per injected packet."

### Slide 8: BPF Maps as Computer Memory
**Content:** Table: ROM_MAP (bytecode), RAM_MAP (heap/stack), CPU_MAP (registers), SCREEN_MAP (framebuffer)
**Speaker notes:** "BPF maps replace every component of a physical computer. ROM is the program. RAM is the heap and stack. CPU_MAP holds registers and program counter. SCREEN_MAP is the framebuffer."

### Slide 9: The MBC Instruction Set
**Content:** Instruction encoding diagram (32-bit: opcode, dst, src, imm16). Key opcodes listed.
**Speaker notes:** "32-bit fixed-width ISA. 16 registers. Standard arithmetic, logic, control flow, memory access. Designed to pass the BPF verifier while supporting general-purpose computation."

### Slide 10: First Sign of Life
**Content:** Screenshot or animation of instruction counter climbing during BSS clearing.
**Speaker notes:** "The first 60 million instructions are just clearing BSS -- 6 megabytes of zeroed memory, byte by byte. But the counter climbed steadily. The CPU never halted. It was working."

### Slide 11: The HashMap Catastrophe
**Content:** Diagram showing BPF HashMap silently dropping writes when full. Corrupted function pointer chain.
**Speaker notes:** "At 99 million instructions, the CPU crashed. A CALLR to address 'LMNO'. The root cause: BPF HashMap silently drops writes when full. Our 128MB RAM map was implemented as a HashMap, and writes beyond capacity just... vanished. Memory corruption cascaded."

### Slide 12: The Fix
**Content:** Code diff: HashMap -> Array. One-line type change.
**Speaker notes:** "BPF Array maps pre-allocate all entries. Writes never fail. One map type change, and the 99-million-instruction crash disappeared forever."

### Slide 13: The Bug Kill Chain
**Content:** Table of all 15 major bugs (abbreviated). Three columns: bug, instructions to manifest, fix.
**Speaker notes:** "15 major bugs. Three caused by IPv6 networking. One catastrophic HashMap corruption. Five defensive patches to Doom's error handling. Two virtual time fixes. The full list is in our wiki."

### Slide 14: D_DoomLoop Alive
**Content:** Screenshot of Doom title screen rendered from BPF SCREEN_MAP data. Frame counter showing 559.
**Speaker notes:** "Frame 559. The title screen, the credits, the demo cycle -- all rendering correctly. D_DoomLoop running continuously. Zero halts. Zero ROM faults."

### Slide 15: The Numbers
**Content:** Large-font metrics table:
- 559+ frames rendered
- 819,000,000+ instructions executed
- 0 halts, 0 ROM faults
- ~1.47M instructions per frame
- ~6 fps baseline (0.003% XDP capacity)

**Speaker notes:** "At 6 fps, we use three thousandths of one percent of estimated XDP capacity. The bottleneck is packet injection in userspace. The kernel data plane has room for 30,000x more traffic."

### Slide 16: Live Demo (Optional)
**Content:** Live browser view of Doom running via WebSocket from doom-bridge service.
**Speaker notes:** "If the demo gods are kind: this is Doom running live in eBPF on this machine, streamed to the browser via WebSocket. If not: here is the recording." (Have fallback video ready.)

### Slide 17: From Doom to Observability
**Content:** Side-by-side: Doom BPF maps vs. production tracing BPF maps. ROM_MAP -> trace logic. RAM_MAP -> flow state. SCREEN_MAP -> metrics export.
**Speaker notes:** "The same infrastructure that runs Doom runs packet tracing. Replace ROM with trace logic. Replace RAM with per-flow state. Replace the framebuffer with metrics export. The architecture is identical."

### Slide 18: The Real Product
**Content:** Diagram of Unheaded's 6-layer architecture with eBPF at Layer 1.
**Speaker notes:** "Unheaded is an infrastructure platform. eBPF-based observability from Layer 2 to Layer 7. Zero sidecar overhead. Sub-microsecond per-hop metadata. Doom was the proof. Packet tracing is the application."

### Slide 19: Performance Headroom
**Content:** Netflix PPS comparison table. XDP at 10M pps vs. our 333 pps. 30,000x headroom.
**Speaker notes:** "A single Netflix 4K stream is 1,500 packets per second. Our baseline Doom rate is 333. XDP hardware handles 10 million. We have four orders of magnitude of headroom before hitting kernel limits."

### Slide 20: Security Considerations
**Content:** Lich campaign summary (D1-D6). ROM injection, TOCTOU, flow collision.
**Speaker notes:** "We ran 6 security campaigns against the live system. ROM must be read-only. BPF maps need capability restrictions. Flow labels need 128-bit UUIDs in production. Every finding feeds the production architecture."

### Slide 21: What We Learned
**Content:** Three bullet points:
1. BPF HashMap is NOT safe for virtual memory (use Array)
2. IPv6 extension headers work for computation, not just metadata
3. The BPF verifier is your friend, not your enemy

**Speaker notes:** "Three hard-won lessons. The HashMap corruption cost us a week. Extension headers are underexplored compute space. And the verifier, which everyone complains about, forced us to write correct code."

### Slide 22: The Timeline
**Content:** Timeline graphic: Jan 20 (first commit) -> Feb 3 (alpha) -> Feb 22 (Doom) -> Mar 8 (production tracing kickoff). 33 days, one engineer, one AI.
**Speaker notes:** "33 days from first commit to Doom proven. One engineer. AI-assisted development with 15 specialized skill personas. ~260,000 lines of production code, ~464,000 with tests. We ship fast."

### Slide 23: Open Questions
**Content:** Three questions for the audience:
1. What other computations belong in the data plane?
2. Can BPF map-as-memory scale to production workloads?
3. Should IPv6 extension headers have a standard compute profile?

**Speaker notes:** "These are genuine questions. We proved one point -- computation in eBPF works. The design space is wide open."

### Slide 24: Call to Action
**Content:** GitHub link, wiki link, email.
**Speaker notes:** "The code is open. The wiki documents everything. If you want to run Doom in your own eBPF environment, the instructions are in the repo. If you want to help build production packet tracing, reach out."

### Slide 25: Thank You
**Content:** "Thank you. Questions?"
**Speaker notes:** "Open for questions. Expect questions about BPF verifier limits, performance at scale, and whether this is actually useful. Answer: yes, the verifier is manageable; yes, XDP scales; and yes, Doom proves it."

---

## Q&A Preparation

### Likely Questions

**Q: Does the BPF verifier accept programs this complex?**
A: Yes. The monad_cpu program is ~21 KB and processes 128 instructions per invocation. It passes the verifier because each invocation is bounded. The trick is executing small chunks per packet, not one giant program.

**Q: What about BPF tail calls? Could you avoid the packet ring?**
A: Tail calls could chain multiple BPF programs, but they have a depth limit (33). The packet ring gives us 255 bounces with full map access at each hop. Tail calls are a future optimization.

**Q: How do you handle BPF map memory limits?**
A: BPF Array maps with 16M entries (128 MB). The kernel allocates this at map creation. Total map memory for Doom is ~200 MB. Production tracing will need far less.

**Q: Is this actually faster than sidecars?**
A: XDP processes packets before sk_buff allocation. Per-packet overhead is nanoseconds, not milliseconds. There is no userspace context switch. For metadata tagging and trace correlation, this is orders of magnitude faster than any sidecar approach.

**Q: What about portability? Does this only work on specific kernels?**
A: Requires Linux 5.x+ with XDP support. Works on commodity hardware. No custom kernel patches. The BPF programs are compiled with standard toolchains (Aya for Rust, cilium/ebpf for Go).

**Q: Could you run Doom at 60 fps?**
A: Theoretically yes. At 32,640 instructions per packet and 1.47M instructions per frame, 60 fps requires ~2,700 packets per second. That is less than two Netflix 4K streams. The bottleneck is userspace injection, and the Go injector should achieve this easily.

---

## Technical Requirements

- Projector/screen for slides (16:9 aspect ratio)
- If live demo: laptop with Linux, XDP support, and the Doom ring pre-loaded
- Backup: pre-recorded video of Doom running in the browser via doom-bridge
- Microphone (30+ minute talk)
- No internet required for demo (everything runs locally)

---

## Timing Guide

| Section | Duration | Cumulative |
|---------|----------|------------|
| Title + Problem | 2 min | 2 min |
| Audacious Claim + Monad | 3 min | 5 min |
| Compilation Pipeline | 2 min | 7 min |
| Packet Ring + BPF Maps + ISA | 6 min | 13 min |
| First Sign of Life + HashMap + Bugs | 5 min | 18 min |
| D_DoomLoop + Numbers | 3 min | 21 min |
| Live Demo (optional) | 2 min | 23 min |
| Doom to Observability + Real Product | 3 min | 26 min |
| Performance + Security + Lessons | 2 min | 28 min |
| Timeline + Call to Action + Thank You | 2 min | 30 min |
| Q&A | 10 min | 40 min |
