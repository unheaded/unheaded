<!--
SPDX-License-Identifier: GPL-3.0-or-later
Copyright (c) 2026 Stevie Bellis.
-->
# Grounding probe

A live check that Zhen's answers name the right sources. It asks a running
Zhen page each question in `prompts.jsonl` and reads the `sources` list of the
answer.

## What it checks

The source line, not the answer's facts.

| id | Question | Passes when |
|---|---|---|
| `cert-ccna` | "tell me about getting the ccna" | no source is listed. The corpus has no CCNA sheet. On 2026-11-11 the answer listed the kerberos sheet, because `ccna` sits inside `KRB5CCNAME`. |
| `cert-jncia` | "explain the steps towards Junos, Associate (JNCIA-Junos)" | at least one source is a `junos-` sheet. |

Every row also needs `tokens_used` above 0: an answer that went through
`zhen-agentd` showed `0 tok` until the daemon returned its usage (backlog row
D-02).

A row has an `id`, a `prompt`, and one expectation: `"expect": "no_source"`,
or `"expect_sources_any": ["prefix", ...]` (one source id must start with one
of the prefixes).

## Why it is not in the coding gate

`eval/coding-gate` and `eval/two-brains` are pre-registered: their prompts and
scoring were fixed before the runs (`eval/two-brains/PREREG.md`), and
`cmd/zhen-eval` pins the count. Adding questions there would change a
registered set after the fact. These two questions are also not coding
questions. They test retrieval and the source line, so they live here.

The retrieval half already has offline regressions with a fake vor
(`raft/tests/test_zhen_rag_retrieve.py`). This probe is the same check against
the real corpus and the real model.

## How to run it

Zhen must be up, and you need a logged-in session in a curl cookie jar. The
script never takes a password. Make the jar once per session (a login lasts 15
minutes); curl reads the JSON body from the terminal, so the password is in no
file, argument or shell history:

```bash
umask 077
curl --curves X25519MLKEM768 --cacert /etc/unheaded/tls/ca.pem \
  --cookie-jar ~/.cache/zhen-session.jar \
  --header 'Content-Type: application/json' --data @- \
  https://127.0.0.1:20103/auth/login
```

Type `{"username":"<you>","password":"<your password>"}`, press Enter, then
Ctrl-D. Then:

```bash
ZHEN_SESSION_FILE=~/.cache/zhen-session.jar eval/grounding/run.sh https://127.0.0.1:20103
rm ~/.cache/zhen-session.jar
```

One line per row, `PASS` or `FAIL`, with the token count and the sources.
Exit 0 when every row passes, 1 when one fails, 2 for a usage problem (no
session file, a session file readable by others, a missing tool).

A run takes as long as two answers from the model: a few minutes on west.
