// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

package main

import (
	"crypto/sha256"
	"encoding/binary"
	"os"
	"path/filepath"
	"strings"
	"unicode"
)

// The split (S-TWO-BRAINS Step 375). A hold-out that shares a task with
// training measures memorisation, not learning, so:
//
//   - every example of one task (its group: agent run, eval task, or
//     conversation) is in the same split;
//   - a group is hold-out when sha256(family + group_key), first 8 bytes
//     big-endian, mod 100 < HoldoutPercent;
//   - eval tasks never train: a two-brains hold-out task stays hold-out,
//     every other eval task (tune split, coding-gate prompts not in the
//     hold-out) is excluded;
//   - a train prompt that overlaps a coding-gate prompt, a two-brains
//     hold-out task, or a prompt of the flywheel's own hold-out by
//     ContaminationThreshold or more (8-word shingles) is excluded.
const (
	HoldoutPercent         = 20
	ContaminationThreshold = 0.8
	shingleWords           = 8
	minShingleWords        = 4 // shorter texts are compared by exact normalized equality
)

// splitBucket is the group's bucket in [0, 100).
func splitBucket(family, groupKey string) int {
	sum := sha256.Sum256([]byte(family + groupKey))
	return int(binary.BigEndian.Uint64(sum[:8]) % 100)
}

// groupKey: all examples of one task share it.
func groupKey(c *Capture) string {
	if c.RunID != "" {
		return "run:" + c.RunID
	}
	return c.ConversationID // "eval:<task>" for eval answers
}

// evalTaskOf is the two-brains task id of an eval capture, or "".
func evalTaskOf(c *Capture) string {
	if t, ok := strings.CutPrefix(c.ConversationID, "eval:"); ok {
		return t
	}
	return ""
}

// References are the prompts no training example may resemble, and the
// two-brains split of every eval task.
type References struct {
	EvalSplit map[string]string // task id -> tune | holdout
	Prompts   []string          // coding-gate prompts and two-brains hold-out task texts
}

type splitRow struct {
	ID    string `json:"id"`
	Ref   string `json:"ref"`
	Split string `json:"split"`
}

// loadReferences reads eval/two-brains/split.jsonl, every coding-gate
// prompt, and the TASK.md of each two-brains hold-out fixture.
func loadReferences(root string) (*References, error) {
	rows, err := readJSONL[splitRow](filepath.Join(root, "eval/two-brains/split.jsonl"))
	if err != nil {
		return nil, err
	}
	ps, err := readJSONL[gatePrompt](filepath.Join(root, "eval/coding-gate/prompts.jsonl"))
	if err != nil {
		return nil, err
	}
	refs := &References{EvalSplit: map[string]string{}}
	for _, p := range ps {
		refs.Prompts = append(refs.Prompts, p.Prompt)
	}
	for _, r := range rows {
		refs.EvalSplit[r.ID] = r.Split
		if r.Split != "holdout" || !strings.HasPrefix(r.Ref, "eval/two-brains/fixtures/") {
			continue
		}
		// #nosec G304 -- a fixture path from the repository's own split file.
		b, err := os.ReadFile(filepath.Join(root, r.Ref, "TASK.md"))
		if err != nil {
			return nil, err
		}
		refs.Prompts = append(refs.Prompts, string(b))
	}
	return refs, nil
}

func words(s string) []string {
	return strings.FieldsFunc(strings.ToLower(s), func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) })
}

func shingles(w []string, n int) map[string]bool {
	out := map[string]bool{}
	for i := 0; i+n <= len(w); i++ {
		out[strings.Join(w[i:i+n], " ")] = true
	}
	return out
}

// overlap is the larger containment of two texts' shingle sets: the
// share of one's word n-grams (n = 8, or the shorter text's length) that
// also occur in the other. Texts under 4 words count only when equal.
func overlap(a, b string) float64 {
	wa, wb := words(a), words(b)
	if len(wa) == 0 || len(wb) == 0 {
		return 0
	}
	n := min(shingleWords, len(wa), len(wb))
	if n < minShingleWords {
		if strings.Join(wa, " ") == strings.Join(wb, " ") {
			return 1
		}
		return 0
	}
	sa, sb := shingles(wa, n), shingles(wb, n)
	inter := 0
	for s := range sa {
		if sb[s] {
			inter++
		}
	}
	return max(float64(inter)/float64(len(sa)), float64(inter)/float64(len(sb)))
}

// normalized is the dedupe form: lower case, whitespace collapsed.
func normalized(s string) string { return strings.Join(strings.Fields(strings.ToLower(s)), " ") }
