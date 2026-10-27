<!--
SPDX-License-Identifier: GPL-3.0-or-later
Copyright (c) 2024-2026 Stevie Bellis.
-->

# Record of Processing Activities

GDPR Art. 30 style, for the deployment on west and east. Controller: Stevie Bellis (contact: `SECURITY.md`). No processor acts for this deployment; Anthropic is a recipient under its own terms (row 4). Columns and data classes come from the [personal data inventory](pii-inventory-2026-10-27.md).

| # | Activity | Purpose | Data subjects | Categories | Recipients | Retention | Safeguards |
|---|---|---|---|---|---|---|---|
| 1 | Operator accounts and access | authenticate the operator and agents to hosts and services | operator | login name, password hash, roles, SSH public keys, API key owner | none | while access is granted; reviewed every 90 days (UH-AC-01) | argon2id hashes, key-only SSH, Ed25519 tokens (ADR-103), TLS 1.3 |
| 2 | Audit and change history | accountability for every change | operator, agents | actor name and type, source address, change reasons | none | permanent (audit and change records) | append-only tables, signed commits |
| 3 | Host and service logs | operate, detect and investigate | operator; remote parties attempting login | login names and client addresses in log lines | none | 90 days; auth/audit lines 455 days | journal file modes, ClickHouse credentials from env, LAN-only hosts |
| 4 | Zhen assistance | answer the operator's questions, draft code and runbooks | operator | prompts, answers, memories, edited file contents | Anthropic, only for turns routed to Claude, after T0/T1/T2 redaction (`pkg/claudebridge/privacy`) | conversations 90 days (policy v1, not yet enforced); memories reviewed monthly | write confirmation gate (ADR-106), redaction before send, spend cap |
| 5 | Zhen training capture | improve local models | operator | redacted prompts and answers | none | 180 days unless exported | opt-in only (`-capture` / `ZHEN_CAPTURE_DIR`); re-scan and quarantine at ingest; Claude-written text excluded from training exports by default |
| 6 | Traffic observation (eBPF) | trace and measure infrastructure traffic | parties whose packets cross the hosts | addresses, ports, sizes, timings (headers and flow metadata only) | none | in-memory rings; metrics per Prometheus retention | no payload capture by design; see the [DPIA](dpia-ebpf-and-zhen-capture.md) |
| 7 | Alert mail | tell the operator something is wrong | operator | mail address, alert text (no personal data by design) | the mail relay | relay's own retention | rate cap per day; alert text names services, not people |
| 8 | Source hosting and CI | develop and publish the code | contributors | commit author and committer names and addresses | GitHub (public repository once published) | permanent | signed commits |

**Transfers outside the US:** none by this deployment; Anthropic and GitHub process in their own regions under their terms.
**Security measures (Art. 32):** the [information security policy](../../../compliance/policies/information-security-policy.md).
