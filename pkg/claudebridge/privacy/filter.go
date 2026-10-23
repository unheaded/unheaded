// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

package privacy

import (
	"context"
	"errors"
	"fmt"
	"unicode/utf8"
)

// Completer is the local model, asked once (claudebridge.LocalLLM).
type Completer interface {
	Complete(ctx context.Context, system, prompt string) (string, error)
}

// SummariseSystem is the fixed prompt for the local summary.
const SummariseSystem = `You summarise tool output for an engineer who will read only your summary.
Keep error messages, counts, service names, states and anything that looks wrong.
Drop addresses, host names, user names, paths under home directories and any identifier.
Plain text, at most 25 lines.`

// Defaults for the summariser.
const (
	DefaultSummariseOver = 8 << 10 // T1 payloads larger than this are summarised
	DefaultFallbackBytes = 2 << 10 // kept when the local model is down
	DefaultSummaryTokens = 600
)

// Mode is how T1 tool results are treated.
type Mode string

// Modes.
const (
	ModeStandard Mode = "standard" // redact, summarise large ones
	ModeStrict   Mode = "strict"   // withhold every T1 result (overnight)
)

// Filter is the privacy tier in front of Claude: classify, redact,
// summarise. A nil Filter is not a valid way to turn it off: callers
// build one, and every failure is an error the caller must not ignore.
type Filter struct {
	Redactor   *Redactor
	Classifier Classifier
	Local      Completer // nil: no summaries, large T1 payloads are cut
	Mode       Mode      // default ModeStandard
	// SummariseOver / FallbackBytes default to the constants above.
	SummariseOver int
	FallbackBytes int
}

// Outcome describes what the filter did to one payload. No content.
type Outcome struct {
	Tier               Tier
	Counts             Counts
	Withheld           bool
	Summarised         bool
	SummaryUnavailable bool
	Mode               Mode
}

// ErrStrictWithheld: strict mode withholds every T1 result.
var ErrStrictWithheld = errors.New("withheld: kingdom ops data (privacy mode strict)")

func (f *Filter) mode() Mode {
	if f.Mode == "" {
		return ModeStandard
	}
	return f.Mode
}

// ToolResult filters one tool result. On error (T2, strict T1, a
// redactor failure) the caller sends Claude an is_error naming why,
// never the text.
func (f *Filter) ToolResult(ctx context.Context, tool string, args map[string]any, projectRoot, text string) (string, Outcome, error) {
	if f == nil || f.Redactor == nil {
		return "", Outcome{}, errors.New("privacy: no filter configured")
	}
	o := Outcome{Mode: f.mode(), Counts: Counts{}}
	o.Tier = f.Classifier.Classify(tool, args, projectRoot)
	if o.Tier == TierUserData {
		o.Withheld = true
		return "", o, ErrWithheld
	}
	if o.Tier == TierOps && o.Mode == ModeStrict {
		o.Withheld = true
		return "", o, ErrStrictWithheld
	}
	out, counts, err := f.Redactor.Redact(ctx, text, o.Tier)
	if err != nil {
		return "", o, err
	}
	o.Counts.Add(counts)
	over := f.SummariseOver
	if over <= 0 {
		over = DefaultSummariseOver
	}
	if o.Tier == TierOps && len(out) > over {
		out, err = f.summarise(ctx, out, &o)
		if err != nil {
			return "", o, err
		}
	}
	return out, o, nil
}

// Prompt redacts a user prompt as T1 (the operator's own text: warned
// and redacted, not blocked).
func (f *Filter) Prompt(ctx context.Context, text string) (string, Counts, error) {
	if f == nil || f.Redactor == nil {
		return "", nil, errors.New("privacy: no filter configured")
	}
	return f.Redactor.Redact(ctx, text, TierOps)
}

// summarise asks the local model for a summary of already-redacted text,
// then redacts the summary again (a summary can re-introduce an address).
// With no local model, or a failure, the text is cut instead.
func (f *Filter) summarise(ctx context.Context, redacted string, o *Outcome) (string, error) {
	keep := f.FallbackBytes
	if keep <= 0 {
		keep = DefaultFallbackBytes
	}
	cut := func() string {
		o.SummaryUnavailable = true
		return fmt.Sprintf("[summary_unavailable: local model down; first %d of %d bytes, redacted]\n%s",
			keep, len(redacted), cutUTF8(redacted, keep))
	}
	if f.Local == nil {
		return cut(), nil
	}
	sum, err := f.Local.Complete(ctx, SummariseSystem, redacted)
	if err != nil || sum == "" {
		return cut(), nil
	}
	again, counts, err := f.Redactor.Redact(ctx, sum, TierOps)
	if err != nil {
		return "", err
	}
	o.Counts.Add(counts)
	o.Summarised = true
	return fmt.Sprintf("[local summary of %d bytes of redacted ops output]\n%s", len(redacted), again), nil
}

func cutUTF8(s string, n int) string {
	if len(s) <= n {
		return s
	}
	for n > 0 && !utf8.RuneStart(s[n]) {
		n--
	}
	return s[:n]
}
