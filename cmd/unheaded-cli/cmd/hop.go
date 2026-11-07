// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

package cmd

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"unheaded/pkg/ebpf"
	"unheaded/pkg/hoprules"
)

// defaultHopPinDir is where ebpf-loader --pin-maps pins hop-ebpf's maps.
const defaultHopPinDir = "/sys/fs/bpf/unheaded/hop-ebpf"

// hopMap is what `hop rules apply` needs from a pinned map.
type hopMap interface {
	Update(key, value []byte) error
	Close() error
}

// openHopMap opens one pinned map. Tests replace it.
var openHopMap = func(path string) (hopMap, error) {
	return ebpf.OpenPinnedMap(path)
}

// pinnedWriter is a hoprules.MapWriter over the pins in dir. A map is opened
// the first time it is written; Close releases them all.
type pinnedWriter struct {
	dir  string
	maps map[string]hopMap
}

// Update implements hoprules.MapWriter.
func (w *pinnedWriter) Update(mapName string, key, value []byte) error {
	m, ok := w.maps[mapName]
	if !ok {
		var err error
		if m, err = openHopMap(filepath.Join(w.dir, mapName)); err != nil {
			return err
		}
		w.maps[mapName] = m
	}
	return m.Update(key, value)
}

// Close releases every map opened.
func (w *pinnedWriter) Close() {
	for _, m := range w.maps {
		_ = m.Close()
	}
}

// NewHopCommand creates `hop`, the writer of hop-ebpf's check maps (ADR-125).
func NewHopCommand() *Command {
	rules := &Command{
		Name:        "rules",
		Short:       "Write hop check rules to hop-ebpf's maps",
		Usage:       "unheaded hop rules <command>",
		SubCommands: make(map[string]*Command),
	}
	rules.AddCommand(newHopRulesApplyCommand())

	cmd := &Command{
		Name:        "hop",
		Short:       "Manage hop-ebpf's checks",
		Usage:       "unheaded hop <command>",
		SubCommands: make(map[string]*Command),
	}
	cmd.AddCommand(rules)
	cmd.AddCommand(newHopModeCommand())
	return cmd
}

// newHopModeCommand creates `hop mode`, the one writer of CONFIG key 4. Enforce
// is refused until the findings in hoprules.EnforceBlockers close. Needs CAP_BPF.
func newHopModeCommand() *Command {
	var pinDir string
	flags := flag.NewFlagSet("mode", flag.ContinueOnError)
	flags.StringVar(&pinDir, "pin-dir", defaultHopPinDir, "Directory holding hop-ebpf's pinned maps")

	return &Command{
		Name:  "mode",
		Short: "Set hop-ebpf's check mode: off or observe (enforce is refused for now)",
		Usage: "unheaded hop mode [--pin-dir <dir>] <off|observe|enforce>",
		Flags: flags,
		RunFunc: func(ctx *Context, args []string) error {
			if len(args) != 1 {
				return &UsageError{"want one mode: off, observe or enforce\nTry 'unheaded hop mode --help' for more information."}
			}
			mode, err := hoprules.ParseMode(args[0])
			if err != nil {
				return &UsageError{err.Error()}
			}
			w := &pinnedWriter{dir: pinDir, maps: make(map[string]hopMap)}
			defer w.Close()
			if err := hoprules.SetMode(w, mode); err != nil {
				return err
			}
			return ctx.Output.WriteStringln("check mode set to " + args[0])
		},
	}
}

// newHopRulesApplyCommand creates `hop rules apply`: validators, HMAC keys and
// retry entries from one rules file (pkg/hoprules gives the format), into
// the maps pinned under --pin-dir. Entries are added or replaced, never
// removed. CONFIG key 4 stays as it is, so rules written while it is 0 do
// nothing. Needs CAP_BPF.
func newHopRulesApplyCommand() *Command {
	var file, pinDir string
	flags := flag.NewFlagSet("apply", flag.ContinueOnError)
	flags.StringVar(&file, "file", "", "Rules file (YAML; see pkg/hoprules)")
	flags.StringVar(&pinDir, "pin-dir", defaultHopPinDir, "Directory holding hop-ebpf's pinned maps")

	return &Command{
		Name:  "apply",
		Short: "Add or replace the entries in a rules file",
		Usage: "unheaded hop rules apply --file <rules.yaml> [--pin-dir <dir>]",
		Flags: flags,
		RunFunc: func(ctx *Context, args []string) error {
			if len(args) > 0 {
				return fmt.Errorf("unexpected argument %q", args[0])
			}
			if file == "" {
				return &UsageError{"--file is required\nTry 'unheaded hop rules apply --help' for more information."}
			}
			f, err := loadHopRules(file)
			if err != nil {
				return err
			}
			now, err := hoprules.Boottime()
			if err != nil {
				return err
			}
			w := &pinnedWriter{dir: pinDir, maps: make(map[string]hopMap)}
			defer w.Close()
			done, err := hoprules.Apply(w, f, func() time.Duration { return now })
			if err != nil {
				return fmt.Errorf("%w (already written: %s)", err, hopApplied(done))
			}
			return ctx.Output.WriteStringln("applied " + hopApplied(done))
		},
	}
}

// loadHopRules parses path and reads the key files it names.
func loadHopRules(path string) (*hoprules.File, error) {
	fh, err := os.Open(path) // #nosec G304 -- the operator's --file
	if err != nil {
		return nil, err
	}
	defer func() { _ = fh.Close() }()
	f, err := hoprules.Parse(fh)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	if err := f.Load(); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return f, nil
}

func hopApplied(a hoprules.Applied) string {
	return fmt.Sprintf("%d validators, %d HMAC keys, %d retry entries", a.Validators, a.HMACKeys, a.Retry)
}
