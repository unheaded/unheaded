// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

package champion

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"unheaded/pkg/zhencapture"
)

// teach_write (DP-309): text a human wants Zhen to know, stored as one
// markdown file in the zhen-taught source. The queue is the only route:
// a call is always held (AlwaysConfirmTools), never gets a token
// (QueueOnlyTools), and runs only from `zhen-claude confirm approve`, the
// one process given a taught directory. Taught text is read back through
// vor as `external`, so it never justifies a mutating call.

// teachTool is the tool's registered name.
const teachTool = "teach_write"

const (
	// MaxTeachTitleBytes caps a title; it is shown whole to the approver.
	MaxTeachTitleBytes = 256
	// MaxTeachTextBytes caps a text, under agentd's 64 KiB body limit
	// with room for JSON escapes.
	MaxTeachTextBytes = 32 << 10

	maxTeachSlugBytes = 80
)

// QueueOnlyTools run from a queue approval and from nothing else: no
// pending token, no confirmer but QueuePrincipal.
var QueueOnlyTools = map[string]bool{teachTool: true}

// AlwaysConfirmTools answer to a human whatever their justification, so
// neither a retrieval chain nor a claimed direct-user label passes Rule 2.
// A person runs one from a pending token; a model loop's call stops at the
// gate, because only ConfirmableTools reach the approval queue. heldForHuman
// names the two argument shapes that keep the ordinary rules, and
// DispatchStanding is the scheduler's route for a job a person set.
var AlwaysConfirmTools = map[string]bool{
	teachTool:         true,
	"runbook_execute": true,
	"schedule_update": true,
	"model_switch":    true,
}

// ErrQueueOnly refuses any route to a QueueOnlyTools call but the queue.
var ErrQueueOnly = errors.New("this tool runs only from a queue approval")

// ErrNoTaughtDir means this process was given no taught directory.
var ErrNoTaughtDir = errors.New("no taught directory; set ZHEN_TAUGHT_DIR for zhen-claude confirm approve")

// WithTaughtDir names the directory teach_write writes, and shuts every
// file tool out of it, by the path as given and as resolved. Only
// `zhen-claude confirm approve` calls this: no daemon has a taught dir.
func (c *Champion) WithTaughtDir(dir string) *Champion {
	if dir == "" {
		return c
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		abs = filepath.Clean(dir)
	}
	c.taughtDir = abs
	c.config.DeniedPaths = append(c.config.DeniedPaths, abs)
	if resolved, err := realPath(abs); err == nil && resolved != abs {
		c.config.DeniedPaths = append(c.config.DeniedPaths, resolved)
	}
	return c
}

// teachCheck validates a teach_write call and returns its title and text.
// It needs no Champion: the gate runs it in processes with no taught dir.
// Nothing here repeats an argument, and nothing is stripped: an edited
// text is not what the human approved.
//
// Secrets are checked in each string, in the stored head, and in title
// and text run together, the last two after teachFold, so a token cut in
// half or spaced with look-alikes still shows. A secret split so that no
// join shows it passes; the approver reads the whole text.
func teachCheck(args map[string]any) (string, string, error) {
	for k := range args {
		if k != "title" && k != "text" {
			return "", "", errors.New("the only arguments are title and text")
		}
	}
	title, ok := args["title"].(string)
	if !ok {
		return "", "", errors.New("title must be a string")
	}
	text, ok := args["text"].(string)
	if !ok {
		return "", "", errors.New("text must be a string")
	}
	if len(title) > MaxTeachTitleBytes {
		return "", "", fmt.Errorf("the title is over %d bytes", MaxTeachTitleBytes)
	}
	if len(text) > MaxTeachTextBytes {
		return "", "", fmt.Errorf("the text is over %d bytes", MaxTeachTextBytes)
	}
	if !utf8.ValidString(title) || !utf8.ValidString(text) {
		return "", "", errors.New("title and text must be valid UTF-8")
	}
	if strings.TrimSpace(text) == "" {
		return "", "", errors.New("the text is empty")
	}
	if strings.ContainsFunc(title, func(r rune) bool { return teachBadTextRune(r) || unicode.IsControl(r) }) {
		return "", "", errors.New("the title holds a control or invisible character")
	}
	if strings.ContainsFunc(text, teachBadTextRune) {
		return "", "", errors.New("the text holds a control or invisible character")
	}
	if teachSlug(title) == "" {
		return "", "", errors.New("the title needs an ASCII letter or digit")
	}
	for _, s := range []string{title, text, teachFold(teachHead(title, text)), teachFold(title + text)} {
		if _, n := zhencapture.Redact(s); n > 0 {
			return "", "", errors.New("it looks like it holds a secret; nothing was queued")
		}
	}
	return title, text, nil
}

// teachBadTextRune reports a rune a text may not hold: a control other
// than tab and line ends, or something an approver cannot see and a model
// can read. Cf covers tag characters, zero-width characters and bidi
// overrides. From U+E0000 up lie the tag block's unassigned points, the
// supplementary variation selectors and the private planes; Co is the
// private area of the first plane.
func teachBadTextRune(r rune) bool {
	switch r {
	case '\n', '\r', '\t':
		return false
	}
	return unicode.IsControl(r) || unicode.In(r, unicode.Cf, unicode.Co) || r >= 0xE0000
}

// teachFold maps Unicode spaces to a plain one and fullwidth ASCII to
// ASCII, for the secret check only; the stored text keeps its bytes.
func teachFold(s string) string {
	return strings.Map(func(r rune) rune {
		switch {
		case unicode.IsSpace(r):
			return ' '
		case r >= 0xFF01 && r <= 0xFF5E:
			return r - 0xFEE0
		}
		return r
	}, s)
}

// teachHead is what the file stores before its digest line.
func teachHead(title, text string) string {
	return "# " + title + "\n\n" + text
}

// teachSlug is title's lowercase ASCII letter and digit runs joined by
// single hyphens: never a path, and "" when the title has none.
func teachSlug(title string) string {
	var b strings.Builder
	gap := false
	for i := 0; i < len(title) && b.Len() < maxTeachSlugBytes; i++ {
		ch := title[i]
		if ch >= 'A' && ch <= 'Z' {
			ch += 'a' - 'A'
		}
		if (ch < 'a' || ch > 'z') && (ch < '0' || ch > '9') {
			gap = b.Len() > 0
			continue
		}
		if gap {
			b.WriteByte('-')
			gap = false
		}
		b.WriteByte(ch)
	}
	return b.String()
}

// teachWrite is the tool body, reached only from an approved queue row.
// It repeats teachCheck, since a row can be written by hand, and creates
// a new file: nothing that exists is opened.
func (c *Champion) teachWrite(ctx context.Context, tc ToolCall) (any, error) {
	start := time.Now()
	if c.taughtDir == "" {
		return nil, ErrNoTaughtDir
	}
	title, text, err := teachCheck(tc.Args)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", teachTool, err)
	}
	name := start.UTC().Format("20060102") + "-" + teachSlug(title) + ".md"
	actionID, auditErr := c.logMutation(ctx, "corpus.teach", fmt.Sprintf("Teach: %s (%d bytes)", name, len(text)), "user")
	if auditErr != nil {
		return nil, auditErr
	}
	sum := sha256.Sum256([]byte(text))
	body := teachHead(title, text) + "\n\n<!-- teach_write text-sha256:" + hex.EncodeToString(sum[:]) + " -->\n"
	if err := c.writeNewBeneath(c.taughtDir, name, body); err != nil {
		c.completeAction(ctx, actionID, "failed", "", err.Error(), time.Since(start))
		return nil, fmt.Errorf("%s: %w", teachTool, err)
	}
	c.completeAction(ctx, actionID, "completed", "Wrote "+name, "", time.Since(start))
	return "wrote " + name + "; a vor restart serves it", nil
}
