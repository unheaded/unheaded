<!-- SPDX-License-Identifier: GPL-3.0-or-later -->
# Brief: claude-api-key-rotation-spend-breach (runbooks/security/claude-api-key-rotation-spend-breach.yaml)

**Scenario.** Either the Claude API key may be exposed, or spend went past
what was expected (a cap hit, an agent loop, a surprise bill). Stop all
spending first, find out what spent, rotate the key, and resume only after
review.

**Owner** moatghost. **Risk** high. **requires_sudo** false (true only for
the system env file). Duration about 30m.

**Where the key lives (never in the repo):** Stevie's shell environment
(`ANTHROPIC_API_KEY`), and on hosts an env file: `/etc/zhen-agentd.env`
(`deploy/systemd/zhen-agentd.service` `EnvironmentFile=`, installed 0600
root, `deploy/README.md`). Known finding: `/etc/unheaded/anthropic.env` on
west was found world-readable (0644): include a step that lists the mode of
each env file (`stat -c '%a %U %n'`), never its content. **No step prints,
greps for or echoes the key value**; name the variable, never expand it.

**Order:**
1. Stop spending (shell, safe to re-run): stop `zhen-agentd`
   (`systemctl stop zhen-agentd`, needs sudo; manual), and, when the
   overnight mode of Phase 7 is installed, `touch "$STATE/zhen-agentd.stop"`
   with parameter `STATE` default `${XDG_STATE_HOME:-$HOME/.local/state}/unheaded`
   and `systemctl --user stop zhen-agentd-overnight.timer zhen-agentd-overnight.service`
   (ignore "not loaded" if Phase 7 is not installed yet).
2. Breach analysis, read-only:
   - the spend ledger `$STATE/zhen-claude/spend.json` (`{day, spent_nano_usd, requests}`;
     `jq . "$STATE/zhen-claude/spend.json"`),
   - when the Well is up, `unheaded_ops.llm_requests` (manual SQL):
     spend by hour (`date_trunc('hour', occurred_at)`, `sum(cost_nano_usd)/1e9`),
     by `source_service`, by `route_reason`; a loop shows as many
     `iteration`s per `conversation_id`.
3. Rotate (manual, irreversible: true): create a new key in the Anthropic
   Console for the organisation Stevie uses, then revoke the old one there.
   Update the env file(s) and Stevie's shell (editor, not echo).
4. Verify with a live smoke that costs at most $0.02 (ADR-104):
   `go run ./cmd/zhen-claude -tools none -effort low -max-tokens 1024 -daily-cap-usd 0.50 -p "Reply with the single word OK." 2>smoke.log`;
   the answer is `OK`, and the log's start line names the credential source
   (`credentials=env ANTHROPIC_API_KEY`), never the value
   (`grep -o 'credentials=[^ ]*' smoke.log`). (`-dry-run` needs no credential and names none.)
5. Spend-cap semantics, for the notes: the cap is per UTC day per ledger
   file (`-daily-cap-usd`, default 5, env `ZHEN_CLAUDE_DAILY_CAP_USD`); the
   worst case is reserved before a request is sent and refused if it would
   pass the cap; an unparseable ledger refuses every request.
6. Resume only after review (manual): restart `zhen-agentd`, remove the
   stop file, start the timer.

**Rollback:** none for a revoked key (that is the point); resuming is step 6.
