<!--
SPDX-License-Identifier: GPL-3.0-or-later
Copyright (c) 2026 Stevie Bellis.
-->

# Zhen training capture (`zhen-capture/v1`)

Opt-in, off by default. When it is on, each turn handled by a local model or
by Claude through Zhen is appended to a JSON Lines file, so the turns can
later feed training. This page is the record format and the safety rules;
what reads the files is a separate piece of work.

## Turning it on

| Producer | Opt-in | Notes |
|---|---|---|
| `zhen-claude` | `-capture <dir>` | one record per prompt (each REPL line is one) |
| Zhen MCP server, `local_llm` tool | `ZHEN_CAPTURE_DIR=<dir>` in the server's environment | one record per call |

Nothing is captured when the flag or variable is unset or empty.

## Where it writes, and what it refuses

- `<dir>/zhen-capture-YYYY-MM-DD.jsonl`, one file per UTC day, one JSON
  object per line.
- The directory must be **outside any git work tree**. The path is resolved
  (symlinks included) and every ancestor is checked for a `.git` entry;
  inside one, capture is refused. Captures hold prompts and answers; they
  must never be committed by accident.
- The directory is created `0700`. An existing directory that group or others
  can access, or that another user owns, is refused (`chmod 700` it).
- Files are opened append-only with `O_NOFOLLOW` (a symlink in place of the
  day's file is refused), forced to `0600`, and each record is one `write`
  under an exclusive `flock`, so several processes can share a directory.

`zhen-claude` checks the directory at startup and exits if it is refused. The
MCP tool checks it on every call; a refusal is logged on the server's stderr
and noted at the end of the tool's answer (`[capture failed: ...]`), and the
answer is still returned.

Suggested location: `~/.local/state/unheaded/zhen-capture/`.

## Redaction

Every string in a record, at any depth (tool inputs included), passes through
the redactor before it is written, then is cut at 64 KiB
(`\n[truncated by zhencapture]` marks a cut).

The patterns live in one file, `pkg/zhencapture/redact_patterns.json`, read by
both writers (Go `pkg/zhencapture`, Python `raft/zhen_mcp_server.py`). A match
becomes `[REDACTED:<kind>]`, keeping a `password=` style prefix where the
pattern has one. Kinds: `private_key` (PEM blocks, including an unterminated
one), `anthropic_key`, `openai_key`, `aws_access_key`, `aws_secret`,
`github_token`, `slack_token`, `jwt`, `bearer`, `url_password`
(`scheme://user:PASSWORD@`), and `assignment` (`password=`, `secret:`,
`api_key=`, `client_secret:`, `*_token=` and similar, values of four or more
characters).

This is a filter for known shapes, not a guarantee. A secret in an unusual
form gets through, which is why the directory rules above exist as well.
`redactions` in each record counts what was replaced.

## Record

```json
{
  "schema": "zhen-capture/v1",
  "id": "9f2c...32 hex",
  "ts": "2026-10-23T18:04:05.123456Z",
  "source": "zhen-claude",
  "route": "escalated",
  "system": "system prompt sent to the model(s)",
  "prompt": "the user's prompt",
  "tool_calls": [
    {"name": "file_read", "provider": "mcp:raft/zhen_mcp_server.py",
     "decision": "allowed", "input": {"path": "..."}, "output": "...", "is_error": false}
  ],
  "local":  {"model": "qwen2.5-coder-7b-instruct", "system": "local system prompt", "answer": "...",
             "stop_reason": "stop", "latency_ms": 2100},
  "check":  {"argv": ["go", "vet", "./..."], "exit": 1, "accepted": false, "error": "", "latency_ms": 900},
  "claude": {"model": "claude-opus-5-5", "served_by": "claude-opus-5-5", "answer": "...",
             "stop_reason": "end_turn", "requests": 2, "latency_ms": 8400, "cost_usd": "0.021400000"},
  "final": "the answer that was printed",
  "accepted_by": "claude",
  "latency_ms": 11400,
  "cost_usd": "0.021400000",
  "redactions": 0,
  "truncated": false
}
```

| Field | Meaning |
|---|---|
| `schema` | always `zhen-capture/v1`; a change of meaning bumps it, a new optional field does not |
| `id`, `ts` | random 128-bit id; UTC time of the write (RFC 3339) |
| `source` | `zhen-claude` or `zhen-mcp/local_llm` |
| `route` | `local`: answered locally and accepted; `claude`: Claude only; `escalated`: local tried first, then Claude |
| `system`, `prompt` | what the model was asked. For `escalated`, both legs got the same prompt; `system` is Claude's |
| `tool_calls` | Claude's tool calls in that turn, in the order they finished (parallel calls can interleave); output cut to 8 KiB |
| `local` | the local leg, when it ran: model the server reported, its own `system`, answer or error, `stop_reason` (finish reason), latency |
| `check` | the `-check` command run on the local answer: argv, exit status (`-1` if it never exited), accepted |
| `claude` | the Claude leg, when it ran: requested and serving model, answer, stop reason, API requests, cost |
| `final` | what was printed to the user (empty when every leg failed) |
| `accepted_by` | `local`, `claude`, or `""` (nothing accepted, or the MCP tool, which cannot know) |
| `cost_usd` | Claude spend for the turn, decimal string; absent when no Claude request was sent |
| `redactions`, `truncated` | set by the writer |
| `local_only` | present and `true` when the turn was marked keep-local (`-local-only` / `local_only`): it never went to Claude |
| `conversation_id` | the conversation the turn belongs to: for `zhen-claude`, the id its `llm_requests` rows carry (an escalated turn takes the Claude conversation's id); for `local_llm`, a fresh id per call. Used by `zhen-flywheel label` to join outcomes |
| `run_id` | the `zhen_agent_runs` id when an agent run produced the turn |
| `task_family` | the routing or eval family (`configs/zhen-routing.yaml`); `local_llm` takes it from its optional `task_family` argument |
| `author_kind` | `human` when `final` is human-written text; absent otherwise (each leg's author is the leg: local or Claude) |

The last four are optional and were added after v1 shipped (a new optional
field does not bump the schema). Captures become training data through
`cmd/zhen-flywheel` (`wiki/Zhen-Training-Data.md`).

Absent legs are omitted, not null. Errors are strings; an empty string means
none.

## Not captured

- Runs that stop before any model is asked (bad flags, no credentials) leave
  no record.
- The local `-check` command's own output.
- Nothing is sent anywhere: capture is local files only.
