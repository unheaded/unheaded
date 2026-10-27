// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// FuzzClassesYAML feeds parseClasses arbitrary input, seeded with the repo's
// classes file. Invariants: no panic, and whatever it accepts is policy a
// purge can run on safely: known tables only, each once, an allowed action,
// a deleting action with an age inside bounds, the database's own age where
// it sets one, hold only on a deleting class.
// S-WELL-SPRING Step 286: go test ./cmd/well-retention/ -run '^$' -fuzz=FuzzClassesYAML -fuzztime=60s
func FuzzClassesYAML(f *testing.F) {
	if b, err := os.ReadFile(filepath.Join("..", "..", "compliance", "retention", "classes.yaml")); err == nil {
		f.Add(string(b))
	}
	f.Add("version: 1\nclasses: []\n")
	f.Add("version: 1\nratified: true\nclasses:\n  - {name: a, table: x, action: purge, online_days: 1}\n")
	f.Add("version: 1\nclasses:\n  - {name: a, table: zhen_conversations, action: purge, online_days: 0}\n")
	f.Add("version: 2\n")
	f.Add("- not a map\n")
	f.Add("version: 1\nclasses:\n  - &a {name: a}\n  - *a\n")
	f.Fuzz(func(t *testing.T, doc string) {
		file, err := parseClasses(strings.NewReader(doc))
		if err != nil {
			return
		}
		if file.Version != 1 || len(file.Classes) == 0 {
			t.Fatalf("accepted version %d with %d classes", file.Version, len(file.Classes))
		}
		seen := map[string]bool{}
		for _, c := range file.Classes {
			tg, ok := targets[c.Table]
			if !ok || seen[c.Table] || !tg.allows(c.Action) || !classNameRE.MatchString(c.Name) {
				t.Fatalf("accepted class %+v", c)
			}
			seen[c.Table] = true
			if c.OnlineDays < 0 || c.OnlineDays > maxOnlineDays || (deletes(c.Action) && c.OnlineDays == 0) {
				t.Fatalf("accepted age %d for %+v", c.OnlineDays, c)
			}
			if tg.fixedDays != 0 && c.OnlineDays != tg.fixedDays {
				t.Fatalf("accepted %d days for %s, the database says %d", c.OnlineDays, c.Table, tg.fixedDays)
			}
			if c.Hold && !deletes(c.Action) {
				t.Fatalf("accepted hold on a class that does not delete: %+v", c)
			}
		}
	})
}
