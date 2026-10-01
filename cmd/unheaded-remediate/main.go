// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

// unheaded-remediate applies fixes from the remediation model (ADR-099),
// replacing scripts/host/remediate.sh with the same behaviour (proved by
// scripts/host TestExecutorMatchesRemediateScript):
//
//	unheaded-remediate list
//	unheaded-remediate plan  <fix>         # what would change (no root needed)
//	unheaded-remediate apply <fix>         # as root, after Stevie's yes for this fix on this host
//	unheaded-remediate undo  <backup-dir>  # restore what one apply changed
//
// The model file is read only if root owns it and it and its directory are
// not group- or world-writable (ssh StrictModes): a fix runs as root, so
// whoever can edit the model must already be root. Every apply and undo is
// journaled in <state>/journal.jsonl.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"

	"unheaded/pkg/compliance/executor"
	"unheaded/pkg/compliance/remediation"
)

const maxModelBytes = 256 << 10

func main() {
	os.Exit(run(context.Background(), os.Args[1:], os.Stdout, os.Stderr))
}

func run(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	fl := flag.NewFlagSet("unheaded-remediate", flag.ContinueOnError)
	fl.SetOutput(stderr)
	modelPath := fl.String("model", "/usr/share/unheaded/remediation/fixes.yaml", "fix model (root-owned when run as root)")
	host := fl.String("host", "", "host the content is for (default: short hostname)")
	root := fl.String("root", "", "filesystem root (tests)")
	state := fl.String("state", "/var/lib/unheaded/remediate", "backups and journal")
	commit := fl.String("model-commit", "", "git commit of the model, recorded in the journal")
	if err := fl.Parse(args); err != nil {
		return 2
	}
	rest := fl.Args()
	usage := func() int {
		fmt.Fprintln(stderr, "usage: unheaded-remediate [flags] list | plan <fix> | apply <fix> | undo <backup-dir>")
		return 2
	}
	if len(rest) == 0 {
		return usage()
	}
	fail := func(err error) int {
		fmt.Fprintln(stderr, "unheaded-remediate:", err)
		return 1
	}
	allowNonRoot := os.Getenv("UNHEADED_REMEDIATE_ALLOW_NONROOT") == "1"

	trustedUID := 0
	if allowNonRoot {
		trustedUID = os.Geteuid()
	}
	writes := rest[0] == "apply" || rest[0] == "undo"
	if writes || os.Geteuid() == 0 {
		if err := checkTrust(*modelPath, trustedUID); err != nil {
			return fail(fmt.Errorf("refusing the model: %w", err))
		}
	}
	m, err := loadModel(*modelPath)
	if err != nil {
		return fail(err)
	}
	if rest[0] == "list" {
		for _, f := range m.Fixes {
			fmt.Fprintf(stdout, "%-16s %-6s %s\n", f.ID, f.Risk, f.Title)
		}
		return 0
	}
	if len(rest) != 2 {
		return usage()
	}
	h := *host
	if h == "" {
		if h, err = os.Hostname(); err != nil {
			return fail(err)
		}
		h, _, _ = strings.Cut(h, ".")
	}
	x, err := executor.New(ctx, executor.Options{Root: *root, State: *state, Host: h, Operator: operator(),
		ModelCommit: *commit, AllowNonRoot: allowNonRoot, Cmd: executor.CommandsFromEnv(), Out: stdout})
	if err != nil {
		return fail(err)
	}
	switch rest[0] {
	case "undo":
		if err := x.UndoDir(ctx, rest[1]); err != nil {
			return fail(err)
		}
		fmt.Fprintln(stdout, "undone:", rest[1])
		return 0
	case "plan", "apply":
		var fix *remediation.Fix
		for i := range m.Fixes {
			if m.Fixes[i].ID == rest[1] {
				fix = &m.Fixes[i]
			}
		}
		if fix == nil {
			return fail(fmt.Errorf("unknown fix %q (see: unheaded-remediate list)", rest[1]))
		}
		if rest[0] == "plan" {
			if err := x.Plan(ctx, *fix); err != nil {
				return fail(err)
			}
			return 0
		}
		if _, err := x.Apply(ctx, *fix); err != nil {
			return fail(err)
		}
		return 0
	}
	return usage()
}

func operator() string {
	for _, k := range []string{"SUDO_USER", "USER"} {
		if v := strings.TrimSpace(os.Getenv(k)); v != "" {
			return v
		}
	}
	return "unknown"
}

func loadModel(path string) (*remediation.Model, error) {
	f, err := os.Open(path) // #nosec G304 -- operator-supplied model path, trust-checked before any write
	if err != nil {
		return nil, err
	}
	defer f.Close()
	b, err := io.ReadAll(io.LimitReader(f, maxModelBytes+1))
	if err != nil {
		return nil, err
	}
	if len(b) > maxModelBytes {
		return nil, fmt.Errorf("%s: larger than %d bytes", path, maxModelBytes)
	}
	return remediation.Parse(b)
}

// checkTrust refuses a model anyone but uid could have written: a regular
// file (not a symlink) owned by uid, not group or world writable, in a
// directory with the same properties (as unheaded-baseline's baseline).
func checkTrust(path string, uid int) error {
	for i, p := range []string{path, filepath.Dir(path)} {
		fi, err := os.Lstat(p)
		if err != nil {
			return err
		}
		if i == 0 && !fi.Mode().IsRegular() {
			return fmt.Errorf("%s: not a regular file", p)
		}
		st, ok := fi.Sys().(*syscall.Stat_t)
		if !ok {
			return errors.New(p + ": cannot read owner")
		}
		if int(st.Uid) != uid {
			return fmt.Errorf("%s: owned by uid %d, want %d", p, st.Uid, uid)
		}
		if fi.Mode().Perm()&0o022 != 0 {
			return fmt.Errorf("%s: writable by group or others (%v)", p, fi.Mode().Perm())
		}
	}
	return nil
}
