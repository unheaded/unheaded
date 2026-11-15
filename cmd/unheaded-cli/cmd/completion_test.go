// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis. All rights reserved.

package cmd

import (
	"regexp"
	"sort"
	"strings"
	"testing"
)

// The three completion scripts are hand-kept lists. These tests walk the
// root command's registry so a command added to root.go and not to the
// scripts fails here.

var (
	bashListRe = regexp.MustCompile(`(?m)^\s*local (\w+)="([^"]*)"$`)
	zshArrayRe = regexp.MustCompile(`(?ms)^\s*(\w+)=\(\n(.*?)^\s*\)$`)
	zshEntryRe = regexp.MustCompile(`(?m)^\s*['"]([a-z0-9-]+):`)
	fishLineRe = regexp.MustCompile(`(?m)^complete -c unheaded -f -n "([^"]*)" -a "([^"]*)"`)
)

// primaryCommands returns the names of c's subcommands, without aliases.
func primaryCommands(c *Command) []string {
	var names []string
	for key, sub := range c.SubCommands {
		if key == sub.Name {
			names = append(names, key)
		}
	}
	sort.Strings(names)
	return names
}

// bashLists maps each "local name=..." variable of the bash script to its words.
func bashLists(script string) map[string][]string {
	out := make(map[string][]string)
	for _, m := range bashListRe.FindAllStringSubmatch(script, -1) {
		out[m[1]] = strings.Fields(m[2])
	}
	return out
}

// zshArrays maps each "name=( ... )" array of the zsh script to its entry names.
func zshArrays(script string) map[string][]string {
	out := make(map[string][]string)
	for _, m := range zshArrayRe.FindAllStringSubmatch(script, -1) {
		for _, e := range zshEntryRe.FindAllStringSubmatch(m[2], -1) {
			out[m[1]] = append(out[m[1]], e[1])
		}
	}
	return out
}

// fishWords returns the words the fish script completes under a condition
// that contains cond.
func fishWords(script, cond string) []string {
	var out []string
	for _, m := range fishLineRe.FindAllStringSubmatch(script, -1) {
		if strings.Contains(m[1], cond) {
			out = append(out, m[2])
		}
	}
	return out
}

func missing(want, got []string) []string {
	have := make(map[string]bool, len(got))
	for _, g := range got {
		have[g] = true
	}
	var out []string
	for _, w := range want {
		if !have[w] {
			out = append(out, w)
		}
	}
	return out
}

func TestCompletionScriptsNameEveryRootCommand(t *testing.T) {
	want := primaryCommands(NewRootCommand())
	if len(want) < 13 {
		t.Fatalf("root registers %d commands, want at least 13: %v", len(want), want)
	}
	cases := map[string][]string{
		"bash": bashLists(generateBashCompletion())["commands"],
		"zsh":  zshArrays(generateZshCompletion())["commands"],
		"fish": fishWords(generateFishCompletion(), "__fish_use_subcommand"),
	}
	for shell, got := range cases {
		if len(got) == 0 {
			t.Errorf("%s: found no command list in the script", shell)
			continue
		}
		if m := missing(want, got); len(m) > 0 {
			t.Errorf("%s completion does not name: %s", shell, strings.Join(m, ", "))
		}
	}
}

func TestCompletionScriptsCompleteHopSubcommands(t *testing.T) {
	hop := NewRootCommand().SubCommands["hop"]
	if hop == nil {
		t.Fatal("root has no hop command")
	}
	want := primaryCommands(hop)
	for _, name := range []string{"rules", "mode"} {
		if len(missing([]string{name}, want)) > 0 {
			t.Fatalf("hop has no %q subcommand: %v", name, want)
		}
	}

	bash := generateBashCompletion()
	if m := missing(want, bashLists(bash)["hop_commands"]); len(m) > 0 {
		t.Errorf("bash hop_commands does not name: %s", strings.Join(m, ", "))
	}
	if !regexp.MustCompile(`(?m)^\s*hop\)\n\s*COMPREPLY=\(\$\(compgen -W "\$\{hop_commands\}"`).MatchString(bash) {
		t.Error("bash has no hop) case that completes ${hop_commands}")
	}

	zsh := generateZshCompletion()
	if m := missing(want, zshArrays(zsh)["hop_commands"]); len(m) > 0 {
		t.Errorf("zsh hop_commands does not name: %s", strings.Join(m, ", "))
	}
	if !regexp.MustCompile(`(?m)^\s*hop\)\n(.*\n)?\s*_describe -t commands 'hop command' hop_commands`).MatchString(zsh) {
		t.Error("zsh has no hop) case that describes hop_commands")
	}

	if m := missing(want, fishWords(generateFishCompletion(), "__fish_seen_subcommand_from hop")); len(m) > 0 {
		t.Errorf("fish does not complete under hop: %s", strings.Join(m, ", "))
	}
}
