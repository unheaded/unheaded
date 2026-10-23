// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"unheaded/pkg/claudebridge"
	"unheaded/pkg/zhenagent"
)

// Review notes (S-TWO-BRAINS Step 212): one Markdown file per run in the
// review dir ($STATE/zhen-reviews/<branch name>.md, 0600), never in the
// branch (a committed note would pollute every merge). Its path goes into
// the card's commits entry and the run's route_summary.

const noteCheckTail = 2 << 10

func (w *worker) writeNote(ctx context.Context, c zhenagent.Card, spec zhenagent.Spec, run zhenagent.Run, wt, status, reason string,
	o claudebridge.RouteOutcome, head string, artefacts []string) string {
	name := strings.TrimPrefix(run.Branch, "zhen/") + ".md"
	path := filepath.Join(w.cfg.ReviewDir, name)
	var b strings.Builder
	fmt.Fprintf(&b, "# Zhen run %s: card %s\n\n", run.ID, c.ID)
	fmt.Fprintf(&b, "- Card: %s (%s)\n", c.ID, oneLine(c.Title, 120))
	fmt.Fprintf(&b, "- Status: **%s**", status)
	if reason != "" {
		fmt.Fprintf(&b, ": %s", oneLine(reason, 300))
	}
	fmt.Fprintf(&b, "\n- Mode: %s; written %s\n", run.Mode, w.cfg.Now().UTC().Format(time.RFC3339))
	fmt.Fprintf(&b, "- Branch: `%s`, worktree `%s`\n", run.Branch, wt)
	fmt.Fprintf(&b, "- Base: `%s` at %s\n", w.cfg.Base, run.BaseCommit)
	if head != "" {
		fmt.Fprintf(&b, "- Head: %s\n", head)
	}
	fmt.Fprintf(&b, "- Route: %s, reason %s, local attempts %d, escalated %v", o.Route, o.RouteReason, o.LocalAttempts, o.Escalated)
	if o.Stop != "" {
		fmt.Fprintf(&b, ", stop %s", o.Stop)
	}
	fmt.Fprintf(&b, "\n- Check: `%s` on `%s`: %s\n", spec.Check, spec.Path, map[bool]string{true: o.Check, false: "not run"}[o.Check != ""])
	fmt.Fprintf(&b, "- Claude cost: %s\n\n", o.Cost.USD())

	b.WriteString("## Task (from the card; untrusted text)\n\n")
	for _, l := range strings.Split(spec.Task, "\n") {
		b.WriteString("    " + l + "\n")
	}

	b.WriteString("\n## Changes\n\n```text\n")
	if head != "" {
		if out, err := w.cfg.Git(ctx, wt, "diff", "--stat", run.BaseCommit+".."+head); err == nil {
			b.Write(out)
		}
	} else if _, err := os.Stat(wt); err == nil {
		if out, err := w.cfg.Git(ctx, wt, "status", "--short", "--untracked-files=all"); err == nil {
			if len(strings.TrimSpace(string(out))) == 0 {
				b.WriteString("(no changes in the worktree)\n")
			}
			b.Write(out)
		}
	} else {
		b.WriteString("(no worktree)\n")
	}
	b.WriteString("```\n")
	if len(artefacts) > 0 {
		b.WriteString("\nTool files the check left in the worktree, not committed (new files in a hidden top-level directory never are):\n\n")
		for i, a := range artefacts {
			if i >= 20 {
				fmt.Fprintf(&b, "- ... and %d more\n", len(artefacts)-20)
				break
			}
			fmt.Fprintf(&b, "- `%s`\n", a)
		}
	}

	if o.CheckOutput != "" {
		tail := o.CheckOutput
		if len(tail) > noteCheckTail {
			tail = "...\n" + tail[len(tail)-noteCheckTail:]
		}
		b.WriteString("\n## Last check output (tail)\n\n```text\n" + strings.ReplaceAll(tail, "```", "'''") + "\n```\n")
	}

	b.WriteString("\n## Review\n\n```bash\n")
	if status == zhenagent.StatusSucceeded {
		fmt.Fprintf(&b, "git -C %s log --stat %s..%s\n", w.cfg.Repo, w.cfg.Base, run.Branch)
		fmt.Fprintf(&b, "git -C %s switch %s && git -C %s merge --no-ff %s   # your own merge; nothing was pushed\n", w.cfg.Repo, w.cfg.Base, w.cfg.Repo, run.Branch)
		fmt.Fprintf(&b, "zhen-claude review accept %s            # after the merge\n", run.ID)
		fmt.Fprintf(&b, "zhen-claude review reject %s -reason '...'\n", run.ID)
	} else {
		fmt.Fprintf(&b, "git -C %s status   # the worktree is kept for inspection\n", wt)
		fmt.Fprintf(&b, "git -C %s worktree remove --force %s   # when done with it\n", w.cfg.Repo, wt)
	}
	b.WriteString("```\n")
	if err := os.WriteFile(path, []byte(b.String()), 0o600); err != nil {
		w.cfg.Log.Error("worker_review_note", "error", err.Error())
		return ""
	}
	return path
}
