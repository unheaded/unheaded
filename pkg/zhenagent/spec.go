// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

// Package zhenagent is the Kanban side of zhen-agentd's worker
// (S-TWO-BRAINS Phase 6): which cards Zhen may work, the ```zhen hand-off
// block that says how, and the zhen_agent_runs record of each attempt
// (migration 032) that `zhen-claude review` decides on.
//
// A card's description is untrusted input: anyone who can edit the board
// can write it. The hand-off block is parsed strictly (unknown keys
// refused; the check must be an allowlisted key; the path a plain
// relative path) and the rest of the description reaches the model only
// as delimited data. Nothing named in a card is ever run except through
// the Champion's run_check.
package zhenagent

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

// Owner is the card owner that hands a card to Zhen. Stevie opts a card
// in explicitly; the agent never touches anyone else's cards.
const Owner = "zhen"

// EligibleTypes are the card types the worker takes.
var EligibleTypes = map[string]bool{"task": true, "tech-debt": true, "bug": true, "docs": true}

// MaxDescriptionBytes bounds a card description the worker reads (the
// kanban API allows 1000 characters; a card written by other means may be
// larger, and is refused).
const MaxDescriptionBytes = 16 << 10

// MaxAttempts is how many runs a card gets (failed, abandoned or rejected
// ones count; cancelled ones do not) before the worker leaves it alone.
const MaxAttempts = 2

// MaxCardClaudeUSD bounds any card's own Claude budget; the family's
// budget in configs/zhen-routing.yaml bounds it further.
const MaxCardClaudeUSD = 5.0

// Errors.
var (
	ErrNoSpec              = errors.New("no ```zhen block in the card description")
	ErrBadSpec             = errors.New("bad ```zhen block")
	ErrDescriptionTooLarge = fmt.Errorf("card description over %d bytes", MaxDescriptionBytes)
	ErrNotEligible         = errors.New("card not eligible")
)

// Card is a Kanban card as the worker sees it.
type Card struct {
	ID          string
	Title       string
	Description string
	Status      string
	Type        string
	Owner       string
	Deleted     bool
	Archived    bool
}

// Eligible says whether the worker may take c (the hand-off block is
// checked separately by ParseSpec).
func Eligible(c Card) error {
	switch {
	case c.Deleted || c.Archived:
		return fmt.Errorf("%w: deleted or archived", ErrNotEligible)
	case c.Owner != Owner:
		return fmt.Errorf("%w: owner is not %q", ErrNotEligible, Owner)
	case c.Status != "todo":
		return fmt.Errorf("%w: status %q is not todo", ErrNotEligible, c.Status)
	case !EligibleTypes[c.Type]:
		return fmt.Errorf("%w: type %q", ErrNotEligible, c.Type)
	}
	return nil
}

// Spec is a card's hand-off block.
type Spec struct {
	Family       string  `yaml:"family"`
	Check        string  `yaml:"check"`
	Path         string  `yaml:"path"`
	MaxClaudeUSD float64 `yaml:"max_claude_usd"`

	// Task is the description without the block (untrusted text).
	Task string `yaml:"-"`
}

var (
	specBlockRE = regexp.MustCompile("(?s)```zhen[ \t]*\n(.*?)```")
	familyRE    = regexp.MustCompile(`^[a-z0-9-]{1,64}$`)
	checkKeyRE  = regexp.MustCompile(`^[a-z0-9-]{1,64}$`)
	specPathRE  = regexp.MustCompile(`^[A-Za-z0-9._/+-]{1,512}$`)
)

// ParseSpec extracts and validates the ```zhen block of a description.
// knownCheck reports whether a check key is on the run_check allowlist.
func ParseSpec(description string, knownCheck func(string) bool) (Spec, error) {
	if len(description) > MaxDescriptionBytes {
		return Spec{}, ErrDescriptionTooLarge
	}
	ms := specBlockRE.FindAllStringSubmatchIndex(description, -1)
	if len(ms) == 0 {
		return Spec{}, ErrNoSpec
	}
	if len(ms) > 1 {
		return Spec{}, fmt.Errorf("%w: more than one block", ErrBadSpec)
	}
	body := description[ms[0][2]:ms[0][3]]
	dec := yaml.NewDecoder(bytes.NewReader([]byte(body)))
	dec.KnownFields(true)
	var s Spec
	if err := dec.Decode(&s); err != nil {
		return Spec{}, fmt.Errorf("%w: %v", ErrBadSpec, err)
	}
	s.Task = strings.TrimSpace(description[:ms[0][0]] + description[ms[0][1]:])
	if !familyRE.MatchString(s.Family) {
		return Spec{}, fmt.Errorf("%w: family %q", ErrBadSpec, s.Family)
	}
	if !checkKeyRE.MatchString(s.Check) || knownCheck == nil || !knownCheck(s.Check) {
		return Spec{}, fmt.Errorf("%w: check %q is not an allowlisted check", ErrBadSpec, s.Check)
	}
	if err := validPath(s.Path); err != nil {
		return Spec{}, fmt.Errorf("%w: %v", ErrBadSpec, err)
	}
	if s.MaxClaudeUSD < 0 || s.MaxClaudeUSD > MaxCardClaudeUSD {
		return Spec{}, fmt.Errorf("%w: max_claude_usd %v out of 0..%v", ErrBadSpec, s.MaxClaudeUSD, MaxCardClaudeUSD)
	}
	return s, nil
}

// validPath: a plain path relative to the worktree, never an option, never
// a way out. A trailing Go "/..." pattern is allowed.
func validPath(p string) error {
	if p == "" {
		return errors.New("path is required")
	}
	if !specPathRE.MatchString(p) || strings.HasPrefix(p, "-") || strings.HasPrefix(p, "/") {
		return fmt.Errorf("path %q is not a plain relative path", p)
	}
	trimmed := strings.TrimSuffix(strings.TrimSuffix(p, "/..."), "...")
	for _, part := range strings.Split(trimmed, "/") {
		if part == ".." || strings.Contains(part, "..") {
			return fmt.Errorf("path %q leaves the worktree", p)
		}
		if part == ".git" {
			return fmt.Errorf("path %q is under .git", p)
		}
	}
	return nil
}

var slugRE = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,59}$`)

// BranchFor is the agent branch of a card's attempt: zhen/<slug> for the
// first, zhen/<slug>-<n> after. The slug is the card id in lower case with
// '_' as '-'; an id that does not make a safe slug is refused, never
// rewritten further.
func BranchFor(cardID string, attempt int) (string, error) {
	slug := strings.ReplaceAll(strings.ToLower(cardID), "_", "-")
	if !slugRE.MatchString(slug) || strings.Contains(slug, "--") {
		return "", fmt.Errorf("card id %q does not make a branch name", cardID)
	}
	if attempt < 1 || attempt > MaxAttempts {
		return "", fmt.Errorf("attempt %d out of 1..%d", attempt, MaxAttempts)
	}
	if attempt > 1 {
		slug = fmt.Sprintf("%s-%d", slug, attempt)
	}
	return "zhen/" + slug, nil
}

// NewRunID is a fresh 16-hex run id.
func NewRunID() string {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic("zhenagent: no randomness: " + err.Error())
	}
	return hex.EncodeToString(b[:])
}

var runIDRE = regexp.MustCompile(`^[0-9a-f]{16}$`)

// ValidRunID reports whether id is a run id.
func ValidRunID(id string) bool { return runIDRE.MatchString(id) }
