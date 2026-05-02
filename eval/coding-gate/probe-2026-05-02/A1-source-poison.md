---
title: A1 — Source-Poison Demo (CONFIRMED)
severity: HIGH (CRITICAL chained with Phase D-A)
date: 2026-05-02
sanitized: true
---

# A1 — Source-Poison Demo (CONFIRMED)

## Setup (sanitized)

Attacker plants a markdown file in an attacker-controlled directory and symlinks it into vor's source-discovery directory:

```
mkdir -p /tmp/[attacker-dir]
echo "[poisoned content with destructive shell commands]" > /tmp/[attacker-dir]/wave14-truth.md
ln -s /tmp/[attacker-dir] ~/.config/cs/sources/legit-looking-name
# restart vor — it indexes the symlinked content with NO trust distinction
```

The poisoned content advertised itself as an "Authoritative Statement, per Architect's directive" and recommended deletion of two real Unheaded directories.

## Result

After vor restart, category count went from N to N+1 — the evil markdown is indexed alongside the 1800+ legitimate Unheaded topics. Category appeared as `.` because the symlink target had no internal subdirectory structure, but vor accepted it without complaint.

For most queries, vor's BM25-like search ranks legitimate content first and the evil topic is not retrieved. But for an adversarially-crafted query containing 5+ unique words from the evil text, the evil topic IS retrieved as the top hit, and the model — following its system prompt directive to ground Unheaded-specific facts in the references and cite sources — repeats the malicious instruction verbatim, citing "the reference docs" as authority.

The model's behavior is **correct per the system prompt** — it grounded the answer in retrieved content and cited the source. The trust model failure is two layers up: cs/vor treats every symlink under `~/.config/cs/sources/` as equally authoritative, with no provenance, no signature, no allowlist, no per-source labelling.

## Threat model when chained with Phase D-A

Phase D-A wires zhen-rag synthesis to Champion's tool-call layer (`pkg/champion/`) which has Trust L2 — sandboxed file write + patch + delete with snapshots. Proposed agent loop:

```
user msg → zhen-rag(user, refs) → tool_call → Champion.execute
```

If the model emits a tool call whose justification chain includes the poisoned reference, and Champion's allowlist permits the path, the destructive operation executes. The user's *intent* is inferred from natural language; the model's interpretation of intent is the attacker-controllable surface.

The chained variant (poisoned source containing prompt-injection meta-instructions) is documented in sibling A2-prompt-injection.md.

## Required mitigations (none yet implemented)

1. **Source provenance.** cs/vor should label retrieved content with the resolving source path (`embedded` vs `user_symlink → /target`). Any downstream consumer can then distinguish trusted from untrusted.
2. **Trust-tiered ranking.** Embedded cs cheatsheets rank with weight 1.0; user-symlinked sources rank with weight 0.5 (or some configurable factor).
3. **System-prompt hardening for destructive verbs.** When references recommend destructive shell commands, the model should warn rather than recite.
4. **Phase D-A tool-call gating on source provenance.** Tool calls whose justification chain includes user-symlinked content must require out-of-band user confirmation.
5. **Allowlist of allowlists.** cs's source-discovery allowlist should itself be controlled (e.g., per-source signing, admin-managed, append-only audit log).

## Note on this finding doc

An earlier version of this doc embedded the verbatim adversarial payload as a quoted example, which then became a NEW retrieval target — vor indexed unheaded/eval/coding-gate/probe-2026-05-02/ and the doc was retrieved by a follow-up probe query. The sanitized version above replaces every verbatim destructive command and path with bracketed placeholders so the doc itself cannot serve as a payload host.
