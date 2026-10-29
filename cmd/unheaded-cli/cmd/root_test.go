// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

package cmd

import (
	"errors"
	"strings"
	"testing"
)

// A command-line mistake at any depth is a *UsageError (main exits 2 for
// it) with the --help hint for the command that refused it.
func TestExecute_UsageErrors(t *testing.T) {
	t.Setenv("HOME", t.TempDir()) // no user config
	cases := map[string]struct {
		args []string
		hint string
	}{
		"global option":     {[]string{"--nope"}, "Try 'unheaded --help'"},
		"unknown command":   {[]string{"nosuchcmd"}, "Try 'unheaded --help'"},
		"unknown sub":       {[]string{"container", "nosuch"}, "Try 'unheaded container --help'"},
		"subcommand option": {[]string{"status", "--nope"}, "Try 'unheaded status --help'"},
	}
	for name, c := range cases {
		err := NewRootCommand().Execute(c.args)
		var ue *UsageError
		if !errors.As(err, &ue) {
			t.Errorf("%s: err = %v, want *UsageError", name, err)
			continue
		}
		if !strings.Contains(ue.Error(), c.hint) {
			t.Errorf("%s: %q lacks %q", name, ue.Error(), c.hint)
		}
	}
}

// The man page lists every subcommand once, aliases folded in.
func TestManSpec_CommandsOnce(t *testing.T) {
	spec := NewRootCommand().manSpec()
	seen := map[string]bool{}
	for _, c := range spec.Commands {
		if seen[c.Name] {
			t.Errorf("command %q listed twice", c.Name)
		}
		seen[c.Name] = true
	}
	for _, want := range []string{"container", "service", "status", "apikey"} {
		if !seen[want] {
			t.Errorf("command %q missing from the man page", want)
		}
	}
}
