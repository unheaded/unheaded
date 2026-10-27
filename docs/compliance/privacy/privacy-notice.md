<!--
SPDX-License-Identifier: GPL-3.0-or-later
Copyright (c) 2024-2026 Stevie Bellis.
-->

# Privacy Notice

What this platform stores about the people who operate it and about the traffic it observes. It describes the code in this repository and the deployment on west and east; an adopter running the code is the one responsible for their own deployment.

## About operators

- **Accounts:** login name, role and an argon2id password hash for the built-in issuer; the owner of each API key; SSH public keys on the hosts.
- **Actions:** who changed what and when: audit events, configuration and compliance history, approvals of Zhen writes, git commit authorship. These records are append-only.
- **What operators write:** Kanban cards, Zhen conversations and memories, files Zhen edits on request. Training capture of Zhen prompts and answers is off unless the operator turns it on; captures are redacted when taken and purged after 180 days unless exported.
- **Logs:** the system journal and its shipped copy hold login names and client addresses from sshd, sudo and the services.

## About traffic

eBPF programs read packet headers and flow metadata: addresses, ports, protocol, sizes, timings, and the Monad hop-by-hop option. They do not store packet payloads. Services never read adopters' application data. This is the platform's NIST 800-207 Zero Trust Architecture posture: infrastructure sees how traffic flows, not what it carries.

## Who else receives it

| Recipient | What | When |
|---|---|---|
| Anthropic (Claude API) | a prompt and its context, after the T0/T1/T2 privacy filter has redacted secrets and identifiers | only when a request is routed to Claude |
| GitHub | source code, commit authorship, CI logs | on push |
| The mail relay | alert subject and body, the operator's address | when an alert mails |

Nothing is sold or shared for advertising. Claude's answers are not used to train local models without prior written authorization from Anthropic, which has not been given.

## How long

Per the [personal data inventory](pii-inventory-2026-10-27.md): logs 90 days (auth and audit lines 455 days), Zhen captures 180 days, audit and change history permanently, backups 14 days of sets.

## Contact

Questions or requests about personal data: the contact in `SECURITY.md`.
