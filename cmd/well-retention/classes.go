// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package main

import (
	"errors"
	"fmt"
	"io"
	"regexp"

	"gopkg.in/yaml.v3"
)

// Actions a class can take (compliance/retention/classes.yaml).
const (
	actionPurge          = "purge"
	actionPurgeFunction  = "purge-function"
	actionArchivePurge   = "archive-purge"
	actionPartitionCheck = "partition-check"
	// actionHeld is never configured: a class under legal hold records it
	// instead of its own action.
	actionHeld = "held"
)

// maxOnlineDays bounds an age to something a person meant (ten years).
const maxOnlineDays = 3650

var classNameRE = regexp.MustCompile(`^[a-z][a-z0-9-]{0,47}$`)

// Class is one row of the retention table.
type Class struct {
	Name       string `yaml:"name"`
	Table      string `yaml:"table"`
	Action     string `yaml:"action"`
	OnlineDays int    `yaml:"online_days"`
	Hold       bool   `yaml:"hold"`
}

// File is compliance/retention/classes.yaml.
type File struct {
	Version  int     `yaml:"version"`
	Ratified bool    `yaml:"ratified"`
	Classes  []Class `yaml:"classes"`
}

// deletes reports whether the action removes rows.
func deletes(action string) bool {
	return action == actionPurge || action == actionPurgeFunction || action == actionArchivePurge
}

// parseClasses reads and validates the classes file. Unknown fields, an
// unknown table, an action the table does not allow, or an age that does
// not match what the database enforces are errors: the file is policy,
// and a typo in it must not delete the wrong rows.
func parseClasses(r io.Reader) (*File, error) {
	dec := yaml.NewDecoder(r)
	dec.KnownFields(true)
	var f File
	if err := dec.Decode(&f); err != nil {
		if errors.Is(err, io.EOF) {
			return nil, errors.New("classes: empty file")
		}
		return nil, fmt.Errorf("classes: %w", err)
	}
	if f.Version != 1 {
		return nil, fmt.Errorf("classes: version %d, want 1", f.Version)
	}
	if len(f.Classes) == 0 {
		return nil, errors.New("classes: none defined")
	}
	names := map[string]bool{}
	tables := map[string]string{}
	for i, c := range f.Classes {
		where := fmt.Sprintf("classes[%d] %q", i, c.Name)
		if !classNameRE.MatchString(c.Name) {
			return nil, fmt.Errorf("%s: name must match %s", where, classNameRE)
		}
		if names[c.Name] {
			return nil, fmt.Errorf("%s: duplicate name", where)
		}
		names[c.Name] = true
		tg, ok := targets[c.Table]
		if !ok {
			return nil, fmt.Errorf("%s: table %q is not one well-retention knows (cmd/well-retention/targets.go)", where, c.Table)
		}
		if prev, dup := tables[c.Table]; dup {
			return nil, fmt.Errorf("%s: table %s already belongs to class %s", where, c.Table, prev)
		}
		tables[c.Table] = c.Name
		if !tg.allows(c.Action) {
			return nil, fmt.Errorf("%s: action %q is not allowed on %s (allowed: %v)", where, c.Action, c.Table, tg.actions)
		}
		if c.OnlineDays < 0 || c.OnlineDays > maxOnlineDays {
			return nil, fmt.Errorf("%s: online_days %d outside 0..%d", where, c.OnlineDays, maxOnlineDays)
		}
		if deletes(c.Action) && c.OnlineDays == 0 {
			return nil, fmt.Errorf("%s: a deleting action needs online_days", where)
		}
		if tg.fixedDays != 0 && c.OnlineDays != tg.fixedDays {
			return nil, fmt.Errorf("%s: %s's age is %d days, set by the database (%s); change it there by migration first",
				where, c.Table, tg.fixedDays, tg.fixedBy)
		}
		if c.Hold && !deletes(c.Action) {
			return nil, fmt.Errorf("%s: hold applies only to a class that deletes", where)
		}
	}
	return &f, nil
}

// selectClasses returns every class, or the one named.
func (f *File) selectClasses(name string) ([]Class, error) {
	if name == "" {
		return f.Classes, nil
	}
	for _, c := range f.Classes {
		if c.Name == name {
			return []Class{c}, nil
		}
	}
	return nil, fmt.Errorf("no class %q", name)
}
