<!--
SPDX-License-Identifier: GPL-3.0-or-later
Copyright (c) 2024-2026 Stevie Bellis.
-->

# DPIA: eBPF Traffic Observation and Zhen Capture

**Date:** 2026-10-27 (draft; Stevie has not reviewed it)
**Why these two:** they are the only processing that could reach beyond the operator's own data: eBPF sees every packet crossing the hosts, and Zhen sends text to a third party and keeps text for training.

## 1. eBPF traffic observation

**What:** XDP and tc programs (`ebpf/`, trace-collector, Heimdall hooks) mark, track and time packets. They read L2-L4 headers, the Monad hop-by-hop option and flow metadata (addresses, ports, protocol, sizes, timings) into maps and ring buffers; trace-collector forwards flow records to Wotan and metrics.
**Necessity:** tracing and latency measurement are the platform's function; headers are the minimum that identifies a flow.
**Not processed:** packet payloads (design rule of the platform's NIST 800-207 Zero Trust Architecture posture: infrastructure reads metadata only; also item 8 of every S-WELL-SPRING phase's definition of done). One exception by design: the UPC programs (`monad-cpu-ebpf`, the MBC cartridges) read the payload of Monad trigger packets, which are the platform's own protocol messages, not user traffic.

| Risk | Likelihood | Impact | Mitigation | Residual |
|---|---|---|---|---|
| R1 a program copies payload bytes into a map or ring buffer | low | high (adopter content) | design rule; programs are small and reviewed; BPF verifier bounds every read | **low, not proven:** no test asserts "no payload bytes leave the kernel" for every program. Action: a static check over `ebpf/` for reads past the transport header (M-108 follow-on, needs an M-id) |
| R2 addresses identify people (home LAN devices, remote peers) | medium | low | flow records stay on the hosts (in-memory rings, metrics); none leave the LAN | low |
| R3 flow records reach the wrong reader through Wotan | low | medium | Wotan topics behind service auth (ADR-103); drift and config topics signed | low |
| R4 an adopter deploys the code where it observes third parties' traffic | medium | medium | out of scope here: the adopter is the controller; the privacy notice says what the code collects | adopter's |

## 2. Zhen to Claude, and training capture

**What:** `zhen-claude` routes a turn to the local model first and, if needed, to the Claude API. Before sending, the T0/T1/T2 privacy filter (`pkg/claudebridge/privacy`) redacts secrets and personal identifiers. With `-capture` / `ZHEN_CAPTURE_DIR` set, each turn is written redacted to a JSONL file; `zhen-flywheel ingest` re-scans it (gitleaks plus the privacy patterns) and quarantines anything that still matches before it reaches `zhen_captures`.
**Necessity:** the operator chose to use a hosted model for hard tasks; capture is optional.

| Risk | Likelihood | Impact | Mitigation | Residual |
|---|---|---|---|---|
| R5 a secret or identifier reaches Anthropic | low | medium | redaction before send; spend cap; `llm_requests` records counts only | low; the filter is pattern-based and can miss novel formats |
| R6 captured text holds personal data | low | low | redaction at capture, re-scan and quarantine at ingest, 180-day purge, only `app_zhen` and the owner can read | low |
| R7 Claude's answers train a local model without authorization | low | high (terms) | Claude-written text excluded from training exports unless `-include-claude-text`; no authorization exists, so that flag is not used | low |
| R8 capture turned on by accident | low | low | off unless a flag or env var names a directory outside any repository | low |

## Conclusion

Both may proceed. The open item is R1's missing proof; until it exists the "no payload" statement is a design rule, not a tested property. Review again when a new eBPF program is added or capture defaults change.
