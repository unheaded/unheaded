# Security Spitball — UPC Running Linux (Joint Sentinel + BlackMage Brief)

**Date:** 2026-05-13
**Trigger:** Stevie's question after Phase 1.3 AP-2 shipped (`73834054`): "popping a shell on this needs to be acknowledged + avoided, but for PoC we should see if it's possible for the wild WTF factor."
**Authors:** unheaded-sentinel (blue team) + unheaded-blackmage (red team), Marshal-coordinated.
**Status:** SPITBALL → feeds Phase 1.3 IMPL §Security Review prerequisites in ADR-075, plus Lich roadmap.

This in-repo copy mirrors the `security_upc_linux_gain_vs_risk` memory at `~/.claude/projects/-home-govan-tmp/memory/`. The memory is for cross-session recall; this doc is for shipping/auditors.

---

## TL;DR

Running Linux on the UPC eBPF interpreter substrate is **viable for a demo, dangerous for prod without four gates**. The "pop a shell" WTF factor is **real and brandable** — but conflated with "RCE on the host," which it isn't *by structure* but could be *by accident*. Four isolation items + integrity gating make the demo auditable.

---

## Sentinel (Blue Team) — Detection + Containment Surface

### Q1. Can a popped shell inside UPC's Linux escape into the host eBPF context or kernel?

**Short answer: structurally NO, but the perimeter is THIN.**

UPC Linux runs as MBC bytecode interpreted by the BPF program `monad-cpu-ebpf`. The "userland" never touches host syscalls directly — every syscall is dispatched by the BPF interpreter against `PROC_TABLE` / `RAM_MAP` / `TTY_MAP`. **Three concrete escape vectors to harden:**

- **BPF verifier bypass.** A crafted MBC program that the translator emits (or a hand-written `.mbc` loaded via `upc-bootctl`) could include patterns the kernel verifier accepts but reach unintended state.
  **Mitigation**: keep `bpf-verifier-check.sh` as a hard gate; never run user-supplied `.mbc` on prod; pin `monad-cpu-ebpf` SHA in production manifests.
- **`upc-bootctl` privilege.** Boot dispatch needs `sudo` (XDP attach + netns mgmt). A compromised bootctl = host root. Treat the bootctl binary like a setuid: SHA-pin, audit trail, never invoke from a network-reachable service.
- **RAM_MAP boundary.** RAM_MAP is currently 64MB inside the BPF map — but the interpreter's `mem_read_word/mem_write_word` masks address with `>> 2`. Any unmasked path (especially new opcodes added for Phase 1.3+) could read/write beyond the intended region.
  **Mitigation**: add unit tests asserting every memory accessor masks before dispatch.

### Q2. Network-side blast radius

**Currently dev-box only.** The UPC instance receives only synthetic Monad trigger packets via `veth-upc0` in netns `upc0`. No real packets reach UPC. **BUT**: the netns is on the host bridge, and the XDP program attaches to the host-side veth — so a misconfigured firewall rule on the host could route the namespace onto the corp/IoT VLAN. Pi-hole won't see anything; the traffic is BPF-internal.

### Q3. Detection signatures for production exposure

Don't expose to WEST/EAST production until these are wired:

- **Anamnesis events** (plumbed for Lich already): emit `sentinel.upc.*` topics on
  - every `populate_rom` call
  - every CSR write to non-standard regions
  - every `mret`/`sret` with priv transition
  Baseline: dev box emits ≤ N events/hour; alert on 10× drift.
- **Pi-hole**: no DNS surface on UPC itself (no userland network stack in xv6 yet), but the HOST bootctl process should be on the trusted VLAN and its DNS queries monitored — exfil via the boot loader is the realistic vector.
- **Firewall**: `iptables -A FORWARD -i veth-upc0+ -j DROP` as a default. UPC instances should never reach an external network until Phase 4 explicitly wires it via the in-Monad TCP/IP stack.
- **Suricata signature**: alert on outbound packets matching the Monad wire format from any non-UPC source — the wire format is small enough to fingerprint.

### Q4. Minimum-viable PoC isolation

For the WTF-demo phase, the FOUR ISOLATION ITEMS:

1. **Dedicated netns** (already done — `upc0`) + **`iptables -A FORWARD -i veth-upc0p -j DROP`** so even if Linux-in-UPC opens a socket, packets die at the host bridge.
2. **`upc-bootctl` runs as a dedicated unprivileged user with `CAP_BPF + CAP_NET_ADMIN` only** (drop full root). Add to `nix/yggdrasil/` baseline.
3. **`seccomp` filter on `upc-bootctl`**: deny `execve` of anything that isn't the BPF object + kernel image. The bootctl is single-purpose; lock it down.
4. **Demo flag**: require `UPC_DEMO_MODE=1` env to start with network attach; absent = `-dry-run` only.

**Anamnesis recording on by default** for every demo run — the audit trail IS the safety net.

---

## BlackMage (Red Team) — Offensive Read

### Q1. Fastest path to a guest shell + host escalation

**Guest shell is gift-wrapped.** Once Phase 1.4 ramdisk lands, `sh` is in the image. There's no AuthN inside xv6 — `init` → `sh` is the *intended* trajectory. The "attack" IS the demo. Real adversary work starts at the **guest → host boundary.**

**Three escalation paths to burn first:**

1. **RV2MBC poisoning.** The freshly-shipped `populate_rv2mbc` reads attacker-controlled bytes if the `.rv2mbc` file is sourced from anywhere but a trusted build. Ship a crafted `.rv2mbc` where index `i` points to MBC PC of a *gadget* in ROM that does MMIO writes I want. Then MRET in legitimate code lands in attacker-chosen ROM slots.
   **Mitigation**: hash the `.rv2mbc` against the `.mbc` at build time, verify in `upc-bootctl`. **CRITICAL today** — tracked as ADR-075 §Security #1, Phase 1.3 IMPL Step 6.
2. **upc-bootctl as SUID-equivalent.** It runs as root with `CAP_BPF + CAP_NET_ADMIN + CAP_SYS_ADMIN`. Any RCE there (argv parsing, path traversal in `kernel.with_extension`, BootParams field overflow into `bp.to_bytes()`) = instant host root. The `--kernel` path is taken as an arbitrary `PathBuf`. Symlink-race that file mid-boot → write where the bootctl runs `std::fs::read()`.
3. **BPF verifier ricochet.** This is the prize. The interpreter has `cfg!(feature = "ascend-linux")` gates that the verifier on kernel 6.17 walks state-prune on. The freshly-added MRET/SRET RV2MBC translation lookups expand state space. **Fuzz the verifier itself** with adversarial `.mbc` images until one is accepted but exhibits OOB in `mem_read_word` — a CRITICAL kernel-level finding.

### Q2. Exploit primitives this gives me that a normal isolated VM does NOT

- **BPF interpreter as oracle.** Every MBC instruction's behavior is observable via `CPU_MAP[instance]` reads — a precise side-channel on cache_hits, insn_count, halted state. **Spectre-class via deterministic interpreter** for cross-tenant inference if multiple UPC instances ever co-locate.
- **Synthetic Monad packet plane.** The packet plane is the trigger surface — an attacker who reaches the host bridge can inject Monad packets that the XDP program parses. Buffer-overflow in `monad_parse_hbh` = pre-auth kernel access. Already on the Lich roadmap (LICH-001).
- **RV2MBC map as influenceable lookup table.** See Q1.1. The map is essentially an attacker-influenced jump table for MRET/SRET/JALR/CALLR — game over if untrusted bytes ever load it.
- **0x47 MRET / 0x48 SRET / 0x49 LR.W / 0x4A SC.W.** Brand-new opcodes added at commit `4c3ae33a`. Zero hours of public scrutiny. LR.W/SC.W especially — reservation tracking is famous for atomicity bugs (Linux had a 7-year MIPS LL/SC race). Fuzz LR.W → context switch → SC.W with priv-level changes interleaved. Tracked as ADR-075 §Security #3, Phase 1.3 IMPL Step 7.

### Q3. WTF-factor demo that's wild but safe

Ranked:

1. **Doom-in-Linux-in-UPC** is the headline. Tweet-ready: "Doom running in Linux running in eBPF running in Linux." It's PRESS, not danger — Doom already ships on UPC at L3; layering Linux above is just window dressing.
2. **CTF flag in `/tmp/flag.txt` that prints via UPC tty bridge** when a guest user `cat`s it. Public, gameable, no host exposure. Encode as `unheaded{phase1_3_ap2_landed}`.
3. **Live kernel oops chain** — recoverable kernel panic that prints a stack trace and reboots. Shows resilience. Don't try to chain real exploits in public.

**AVOID:** reverse-shell demos, network-egress demos, anything that ends in a real `bash`. The line between "wild" and "RCE PoC" is exactly where Stevie's instinct says to stop.

### Q4. Responsible-disclosure posture for launch

- **`SECURITY.md` lands BEFORE public launch.** Email + PGP key, 90-day embargo, CVE-numbering-authority partnership (use MITRE's free CNA-LR program for individual projects).
- **`security.txt` at `/.well-known/security.txt`** on any public-facing host. RFC 9116.
- **Bug bounty optionality**: state explicitly "we welcome research, no paid bounty yet, but credit on the wall of fame + early access to next-gen builds." Researchers respect honesty about budget.
- **Pre-empt the LR.W/SC.W finding**: publish a write-up of the opcode security model BEFORE launch so when a researcher finds something, you can say "we knew, here's the threat model."
- **Coordinate with Sentinel**: every disclosed finding becomes a new Lich campaign + a new sigma rule. The vuln becomes a detection.

---

## Joint Verdict

The WTF factor is high-value branding. Treat it like a controlled fire — the four Sentinel isolations + RV2MBC integrity + bootctl seccomp = the firebreak. Without those, the same demo is a backdoor announcement.

## Phase 1.3 IMPL Prerequisites (extracted from this spitball, also in ADR-075 §Security Review)

1. **RV2MBC SHA-256 integrity gate** — Phase 1.3 IMPL Step 6 owns this. SHA in BootParams reserved bytes; `populate_rv2mbc` verifies before load.
2. **PROC_TABLE MMIO isolation** — Phase 1.3 IMPL must ensure no MMIO region overlaps PROC_TABLE's BPF-map-accessible range. Audit during Step 1 (slot widening).
3. **LR.W/SC.W priv-transition falsification tests** — Phase 1.3 IMPL Step 7 adds two tests (context switch + priv transition both invalidate reservations).

## Phase 1.4+ Carry-Overs

- **Tetragon/KRSI exploration** for the broader Yggdrasil SELinux replacement question (Stevie's idea 2026-05-13). Task #9 queued. Dual-track: SELinux baseline + BPF LSM Kingdom-aware policy.
- **Suricata Monad-format signature** — write the sigma rule + commit before any public demo.
- **Anamnesis `sentinel.upc.*` topic family** — define the schema before first prod-adjacent boot.
- **SECURITY.md + security.txt** — author both before any public-facing announcement.
- **LR.W/SC.W threat model write-up** — pre-empt the Lich's own finding by publishing the model.

## Cross-References

- ADR-075 `docs/adr/ADR-075-phase13-process-model.md` — §Security Review consolidates these three items into IMPL prerequisites.
- Memory `security_upc_linux_gain_vs_risk` at `~/.claude/projects/-home-govan-tmp/memory/`.
- Phase 1.3 IMPL plan `references/battle-plan-phase13-impl-2026-05-13.md` §Sub-phase C (Steps 6-7).
- BPF LSM ADR-draft Task #9 (Yggdrasil P2 follow-on).
- Lich roadmap: LICH-001 (Monad parser fuzzing), planned LICH-006 (BPF verifier boundary), planned LICH-007 (MBC bytecode coverage-guided fuzz).
