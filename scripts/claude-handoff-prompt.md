# Claude Code Bare Metal Sprint Handoff — "The Bot"
# Usage: cat scripts/claude-handoff-prompt.md | claude
# Or:   claude < scripts/claude-handoff-prompt.md
# Or:   claude --prompt "$(cat scripts/claude-handoff-prompt.md)"

Read CLAUDE.md, then read references/timeline.md, then read docs/planning/TODO.md.

You are running BARE METAL on "west" — the champion box.
- Ubuntu 25.10, kernel 6.x
- AMD RX 7700 XT (12GB VRAM, gfx1101), ROCm 6.4.2
- 1TB NVMe (root, 200GB allocated, 728GB free in VG)
- 2TB HDD mounted at /var
- Docker + LXD + FRR 10.4.1 + Rust nightly + Go + bpf-linker
- node_exporter + fail2ban + UFW — all active
- vLLM ROCm image pulled (rocm/vllm:latest + rocm/pytorch:latest)
- All 16 bootstrap phases COMPLETE

YOU ARE NOW UNBLOCKED ON LINUX. eBPF blocker B1 is DEAD.

SKILLS: Check skills/ directory — you have 28 Unheaded skills. USE THEM.
Key skills: developer, architect, blackmage, warmonger, scientist, marshal.

=== BACKLOG: 200+ ITEMS — MULTI-AGENT SPRINT MODE ===

P0 PRODUCTION BLOCKERS (do these FIRST):
1. eBPF programs are userspace STUBS — compile packet_marker, flow_tracker,
   latency_probe to REAL BPF targets with Aya on actual kernel
2. trace-collector (Rust) — wire kernel→Wotan bridge
3. No release signing or SBOM generation
4. Captain /tmp data storage fix verification

P1 DEPLOYMENT BLOCKERS:
5. No auth on ANY endpoint — wire mTLS between services + API keys
6. No TLS on gRPC connections
7. Rate limiter XFF spoofing vuln — use RemoteAddr
8. Wotan nil fallback silent failure → degraded mode
9. E2E smoke test — all 10 services running in containers
10. docker compose up full stack — make deploy is currently a NO-OP
11. SBOM generation (syft/cyclonedx)
12. Container image scanning (trivy/grype)
13. Secrets management (SOPS/age)

P1 INFRASTRUCTURE:
14. FRR networking — configure IS-IS/BGP underlay on lxdbr0
15. VXLAN + EVPN overlay
16. Sub-50ms latency validation — packet → browser
17. Nix flake.nix for nixos/ tree
18. LXD full deployment wiring

P1 CODE DEBT (50+ source TODOs):
19. pkg/waf/ — 30+ items marked "TODO: Rust rebuild"
20. Real LXD client (not stub) — cmd/unheaded-daemon/internal/lxd/
21. Real eBPF loader (not stub) — cmd/unheaded-daemon/internal/ebpf/
22. JWT validation in pkg/auth/
23. ACME cert loading in pkg/certs/
24. Wotan WAL implementation
25. protocol-api BPF map operations (pinned map lookup/update/delete)
26. getOrCreateGRPCClient double-check locking fix
27. Dead BroadcastJSON removal

P2 SERVICES:
28. Timeline REST API — serves living roadmap with MD/JSON/YAML mirrors
29. Strategy REST API — vision endpoints
30. Task execution API — WHAT & WHEN
31. TopicStream gRPC service (kill 500ms HTTP polling)
32. Anamnesis event history — immutable audit log
33. Gateway API unification

P2 IaC RENDERERS (pkg/iac/):
34. IaCRenderer interface + Ansible/Terraform/Puppet/K8s/Chef/Salt renderers

P2 OBSERVABILITY ADAPTERS (pkg/observability/):
35. ObservabilityAdapter interface + ELK/Fluentd/Jaeger/Nagios/Loki adapters

P3 BATTLE PLANS PENDING EXECUTION:
36. S72 — protocols, auth, SBOM, CI/CD (217 steps)
37. S-PQC — post-quantum cryptography
38. S50 — AI/LLM inference stack
39. WS3 — scaling beyond alpha

P3 CHAMPION (LLM training — pinned, not priority):
40. RAFT training pipeline — 4-ring corpus, QLoRA on 7700 XT
41. Killer combo download (Wikipedia, SO, GitHub, kernel, ArXiv)

BARE METAL VALIDATION (was blocked, NOW LIVE):
42. Boot NixOS containers on real LXD
43. Run setup-opnsense.sh / setup-ipfire.sh
44. Install FRR/BIRD from source
45. WireGuard key exchange, activate wg0
46. firewall-health-check.sh — all PASS
47. Monad HbH end-to-end validation (Scapy)
48. Collect H1-H8 verdicts

EXECUTION RULES:
- Spawn parallel agents for independent work streams
- Use worktrees for isolation
- Commit early, commit often — conventional commits
- Run tests after every change — 80%+ coverage or BLOCK
- Security first — every PR checked for isolation violations
- Read the skill before you start the work
- Marshal keeps you on track — no yak shaving
- SHIP. SHIP. SHIP.

Priority order: P0 → P1 → P2 → P3. Start with eBPF.
