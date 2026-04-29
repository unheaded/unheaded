# Potential Skill Additions — BlackMage Network Ops

**Date:** 2026-03-18
**Context:** Live network investigation session on WEST dev machine
**Participants:** Stevie + Claude (BlackMage mode)

---

## What Worked Well

### Pi-hole as a Detection Platform
- Deployed Pi-hole in Docker (host networking, 0.0.0.0:53) as an instant DNS visibility layer
- Pointed router DHCP DNS at WEST — all network devices immediately visible
- Each IoT device got a DHCP lease and showed up as a distinct client IP in Pi-hole logs
- Blocking domains was instant: `pihole deny <domain>` and the TV's telemetry died immediately
- Real-time tail (`pihole -t`) matched the web UI — good for CLI-native workflow

### Methodical Device Isolation
- Unplugged all IoT devices, set phone to airplane mode, closed browsers
- Established clean baseline (only WEST + MacBook SSH)
- Plugged in devices one at a time: TV first (suspect #1), then stereo
- Each device's boot sequence was captured clean — easy to identify expected vs unexpected traffic

### Vizio ACR Discovery
- TV was not compromised but was aggressively spying via ACR (Automatic Content Recognition)
- `kinesis.us-west-2.amazonaws.com` beaconing at 1 query/second — streaming viewing habits to AWS
- `tvinteractive.tv` domains — DTS/Inscape ACR control plane
- 15 tracking/ad domains blocked in one command
- FTC fined Vizio $2.2M for exactly this behavior — good context for the user

### Stereo Analysis
- Clean boot: NTP sync + AWS IoT telemetry (one beacon) + Tidal/Airable streaming
- Minimal traffic profile compared to TV — well-behaved device
- No lateral movement indicators between TV and stereo

### BlackMage Deep Dive
- Two-phase investigation: surface scan then deep dive
- Phase 1: ARP, ports, firewall, SSH logs, processes, Docker, namespaces
- Phase 2: rootkit detection, DNS exfil analysis, tcpdump capture, binary verification, kernel modules, capabilities audit
- Found real issue: SSH password auth enabled via cloud-init override (50- vs 99- first-match-wins)
- All findings written to markdown report with severity ratings

---

## What Should Be Formalized in BlackMage Skill

### 1. Network Recon Protocol
Add a section for home/lab network investigation. Current BlackMage skill is protocol-focused (Monad/Sophia/Wotan attack vectors). Missing:
- ARP table analysis for device discovery
- Pi-hole/DNS-based traffic analysis
- IoT device fingerprinting via DNS patterns
- Device isolation methodology (clean room baseline)

### 2. Pi-hole Deployment Playbook
Quick-deploy DNS monitoring for any network:
- Docker compose with host networking
- systemd-resolved stub disable
- LXD dns.mode=none
- Static IP assignment
- Router DHCP DNS configuration
- Common IoT tracking domain blocklists (Vizio, Amazon, Samsung, LG, Roku)

### 3. IoT Threat Assessment Template
Standard format for evaluating IoT devices:
```
Device: [name]
IP: [x.x.x.x]
MAC: [vendor lookup]
Boot domains: [list]
Telemetry domains: [list with frequency]
Ad/tracking domains: [list]
Suspicious domains: [list]
Verdict: [CLEAN / SPYWARE / COMPROMISED]
Action: [block list / isolate / remove from network]
```

### 4. UPnP Post-Mortem Checklist
When UPnP was enabled and has since been disabled:
- Check router port forwarding table for remnant rules
- Check iptables/nftables for rogue NAT rules
- Audit all IoT device DNS for C2 beaconing patterns
- Check for DGA (domain generation algorithm) patterns in query logs
- Verify no inbound connections from internet

### 5. Encrypted DNS / VPN Blind Spots
Document the gaps in DNS-based monitoring:
- DoH (Chrome, Firefox, macOS) bypasses Pi-hole
- VPN tunnels bypass Pi-hole entirely
- Hardcoded IP connections bypass DNS completely
- tcpdump on the interface catches what Pi-hole misses but requires more analysis

### 6. Prompt Injection Risk in Security Sessions
Meta-security concern specific to AI-assisted investigations:
- Pasting DNS logs containing attacker-controlled domains could inject prompts
- Tool results from compromised systems could manipulate agent behavior
- Overnight unattended sessions with sudo are high-risk
- Mitigation: hooks to gate destructive ops, review diffs, don't skip permissions for security work

---

## Potential New Skill: `unheaded-sentinel`

Consider a dedicated network monitoring/defense skill separate from BlackMage (offensive). BlackMage attacks, Sentinel watches.

**Sentinel would own:**
- Pi-hole deployment and management
- Network device inventory and monitoring
- IoT traffic baselining and anomaly detection
- Firewall rule management (iptables/nftables)
- DNS log analysis and reporting
- Automated threat detection (DGA, beaconing, exfil patterns)

**BlackMage would still own:**
- Offensive testing against Unheaded protocol
- Fuzzing campaigns (Lich)
- Vulnerability assessment
- Red team exercises
- Exploit development

**Handoff:** Sentinel detects anomaly -> BlackMage investigates and exploits -> Architect hardens -> Developer patches

---

## CLI Aliases to Formalize

```bash
# Pi-hole quick commands
alias pht='sudo docker exec pihole pihole -t'           # Live tail
alias phq='sudo docker exec pihole pihole api queries'   # Recent queries
alias phb='sudo docker exec pihole pihole deny'          # Block domain
alias pha='sudo docker exec pihole pihole allow'         # Allow domain
alias phs='sudo docker exec pihole pihole status'        # Status check
```

---

## Hardware Recommendations

- **Dedicated Pi-hole box** — Raspberry Pi 4 or old laptop, not sharing with dev machine
- **Managed switch with VLANs** — isolate IoT devices from trusted devices
- **Dedicated WAP for IoT** — separate SSID, separate subnet, firewalled from trusted LAN
- **Network TAP** — for full packet capture when needed (not just DNS)

---

*Session was productive. The flow of deploy-monitor-isolate-analyze-block worked naturally.
Pi-hole + BlackMage CLI analysis is a solid combination for home lab security ops.*
