# Zhen Privacy Tiers

Nothing from the Well, logs, hosts or untracked files goes to the Claude API raw. `zhen-claude` passes the prompt and **every tool result** through a local privacy tier before it becomes part of a request. Failure is closed: if the tier cannot run, the text is not sent.

## Tiers

| Tier | What | Leaves as |
|---|---|---|
| **T0 repo-public** | `corpus_search`, `runbook_list`, `runbook_show`; `read_file`/`file_read` of a file tracked by git at HEAD that is not on the deny list (`.env`, `credentials`, `secrets`, keys) | text with secrets removed |
| **T1 kingdom-ops** | everything else: `service_health`, `kanban_*`, `local_llm`, untracked files, files outside the sandbox, and any tool the classifier does not know | secrets, IPv4/IPv6 addresses, MACs, emails, home-directory user names and host names (`west`, `east`) replaced; output over 8 KiB summarised by the local model first |
| **T2 user data** | anything under `-user-data-roots` / `ZHEN_USER_DATA_ROOTS` | never: Claude gets `is_error` "withheld: user data" |

Placeholders are stable within one payload (`[REDACTED:ipv4#1]` is the same address every time it appears), so Claude can still reason about "the same host".

## How a payload is cleaned

1. `gitleaks stdin` (8.19+, 2 s limit, run outside the repo so no allowlist applies) finds secrets; each is replaced.
2. Deterministic patterns: PEM blocks, `scheme://user:pass@`, `password=`/`token=`/`api_key=` values, AWS key ids; for T1 also emails, MACs, IPv6, IPv4, `/home/<user>`, host names.
3. T1 over 8 KiB: the **redacted** text goes to the local model (llama-server, temperature 0, 600 tokens, fixed prompt: keep errors, counts, service names; drop addresses and identifiers), and the summary is redacted **again**. Local model down: the first 2 KiB of the redacted text, marked `summary_unavailable`.

Any failure (scanner missing, timeout, bad output) turns the result into `is_error` "withheld"; a prompt that cannot be checked is not sent at all.

## Modes

| `-privacy` | T1 tool results |
|---|---|
| `standard` (default) | redacted (and summarised when large) |
| `strict` | withheld, every one (overnight runs) |

## Seeing what happened

Each tool call's audit line (and `audit_events.details`) carries `tier`, `privacy_mode` and `redactions`; each request's `redactions` lands in `llm_requests` ([[Zhen Audit|Zhen-Audit]]):

```sql
SELECT date_trunc('day', occurred_at) AS day, sum(redactions) FROM llm_requests
 WHERE backend = 'claude' GROUP BY 1 ORDER BY 1 DESC;
```

A prompt you type that holds T1 values is redacted too, with a `privacy` warning naming the counts (your text, your choice: warned, not blocked).

## What still leaves

- T0 repository text (ADRs and docs name hosts and addresses; it is the public repo, accepted);
- redacted or summarised T1 text, and counts;
- your own prompt, after redaction.

**Outside this tier:** Claude Code sessions talk to Zhen's MCP server directly under your own Claude Code consent; `service_health` and `local_llm` there return T1 data unredacted. See [[Zhen Audit|Zhen-Audit]].

## Flags

`-privacy standard|strict`, `-user-data-roots a,b` (`ZHEN_USER_DATA_ROOTS`), `-secret-scanner <path>` (`ZHEN_SECRET_SCANNER`; default `gitleaks` on PATH, then `~/.local/bin/gitleaks`).

---

> **Source:** [ADR-104 §8](../docs/adr/ADR-104-zhenai-claude-api-bridge.md) · [S-TWO-BRAINS plan](../docs/battle-plans/S-TWO-BRAINS-battle-plan.md) Phase 3 · `pkg/claudebridge/privacy/`
