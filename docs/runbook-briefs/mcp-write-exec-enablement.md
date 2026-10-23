<!-- SPDX-License-Identifier: GPL-3.0-or-later -->
# Brief: mcp-write-exec-enablement (runbooks/security/mcp-write-exec-enablement.yaml)

**Scenario.** An attended session needs the Zhen MCP server's mutating
tools (file writes, or running a runbook) for one piece of work. Turn them on
for that one session only, use them, turn them off, review what they did.

**Owner** blackmage. **Risk** high. **requires_sudo** false. Duration about 10m
plus the work.

**Facts:**
- Server `raft/zhen_mcp_server.py`, launched by `scripts/zhen-mcp.sh`,
  registered for Claude Code in the repo's `.mcp.json` as server `zhen` with
  `"env": {}` (read-only). Gates, read at call time: `ZHEN_MCP_ALLOW_WRITE=1`
  adds `file_write` and `file_patch`; `ZHEN_MCP_ALLOW_EXEC=1` adds
  `runbook_execute` (runs `scripts/run-runbook.py`). Off means the tool is
  not listed and a call is refused.
- Enable for ONE session only: in the launching shell
  (`export ZHEN_MCP_ALLOW_WRITE=1` before starting the session), or in the
  `env` of the `zhen` entry in `.mcp.json` **without committing it**. The
  committed `.mcp.json` must keep both flags off.
- `zhen-claude` offers MCP mutating tools only with `-mcp-mutating`, and they
  still pass the Champion gate. The Champion's own write tools (snapshots,
  revert, out-of-band confirmation) are the preferred path for writes.
- Verify the tool list without credentials and without sending anything:
  `go run ./cmd/zhen-claude -dry-run -tools mcp -mcp-mutating -mcp-cmd scripts/zhen-mcp.sh -p x 2>&1 | grep -o '"tools":\[[^]]*'`
  lists `file_write`/`file_patch` only when `ZHEN_MCP_ALLOW_WRITE=1` is set
  in that shell, `runbook_execute` only with `ZHEN_MCP_ALLOW_EXEC=1`.
  Run from the repo root (parameter `REPO` default `$HOME/tmp/unheaded`).
- Guard against committing them on: `git -C "$REPO" diff --quiet -- .mcp.json`
  and `jq -e '[.mcpServers[] | .env // {} | (.ZHEN_MCP_ALLOW_WRITE // "0", .ZHEN_MCP_ALLOW_EXEC // "0")] | all(. != "1")' "$REPO/.mcp.json"`.
- Order: preflight (committed .mcp.json off) → enable (manual) → verify
  listed → do the work (manual) → disable (manual: unset / revert the
  uncommitted edit, restart the session) → verify not listed → review what
  was done: `git -C "$REPO" status --short` and `git -C "$REPO" diff --stat`;
  the Champion's `zhen_actions` / `audit_events` rows when the Well is up (manual).
- Open hardening items, so write-enable is for **attended sessions only**:
  a refused path returns an `ACCESS DENIED` text result rather than an MCP
  `isError`; `file_write` has a symlink time-of-check/time-of-use race.

**Rollback:** disable (unset the variables, `git -C "$REPO" checkout -- .mcp.json`
if it was edited), restart the session; revert unwanted file changes with git.
