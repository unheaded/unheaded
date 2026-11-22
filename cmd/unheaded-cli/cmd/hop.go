// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

package cmd

import (
	"flag"
	"fmt"
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
	Lookup(key []byte) ([]byte, error)
	Iterate(fn func(key, value []byte) error) error
	Delete(key []byte) error
	Close() error
}

// openHopMap opens one pinned map, whose file name is the map's name. The pin
// and its directory must be root's or the caller's and closed to group and
// world, and the kernel's name for the map must be the file's: key material
// and rules go into whatever the pin names (pkg/ebpf.OpenPinnedMapNamed).
// Tests replace it.
var openHopMap = func(path string) (hopMap, error) {
	return ebpf.OpenPinnedMapNamed(path, filepath.Base(path))
}

// pinnedWriter is a hoprules.MapWriter and MapReader over the pins in dir. A
// map is opened the first time it is used; Close releases them all.
type pinnedWriter struct {
	dir  string
	maps map[string]hopMap
}

func (w *pinnedWriter) open(mapName string) (hopMap, error) {
	if m, ok := w.maps[mapName]; ok {
		return m, nil
	}
	m, err := openHopMap(filepath.Join(w.dir, mapName))
	if err != nil {
		return nil, err
	}
	w.maps[mapName] = m
	return m, nil
}

// Update implements hoprules.MapWriter.
func (w *pinnedWriter) Update(mapName string, key, value []byte) error {
	m, err := w.open(mapName)
	if err != nil {
		return err
	}
	return m.Update(key, value)
}

// Lookup implements hoprules.MapReader.
func (w *pinnedWriter) Lookup(mapName string, key []byte) ([]byte, error) {
	m, err := w.open(mapName)
	if err != nil {
		return nil, err
	}
	return m.Lookup(key)
}

// Iterate implements hoprules.MapReader.
func (w *pinnedWriter) Iterate(mapName string, fn func(key, value []byte) error) error {
	m, err := w.open(mapName)
	if err != nil {
		return err
	}
	return m.Iterate(fn)
}

// Delete implements hoprules.MapPruner.
func (w *pinnedWriter) Delete(mapName string, key []byte) error {
	m, err := w.open(mapName)
	if err != nil {
		return err
	}
	return m.Delete(key)
}

// countingWriter is the MapWriter of a dry run: it takes every entry and
// writes none, so Apply still encodes and counts the whole file.
type countingWriter struct{}

// Update implements hoprules.MapWriter.
func (countingWriter) Update(string, []byte, []byte) error { return nil }

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

// newHopModeCommand creates `hop mode`, the one writer of CONFIG key 4 in the
// tree. Enforce is refused here until the findings in hoprules.EnforceBlockers
// close, and the packaged hop_xdp refuses it too: built without the
// hop-enforce feature, it runs key 4 = 2 as observe (ADR-125). Needs CAP_BPF.
func newHopModeCommand() *Command {
	var pinDir string
	flags := flag.NewFlagSet("mode", flag.ContinueOnError)
	flags.StringVar(&pinDir, "pin-dir", defaultHopPinDir, "Directory holding hop-ebpf's pinned maps")

	return &Command{
		Name:  "mode",
		Short: "Set hop-ebpf's check mode: off or observe (enforce is refused here and by the packaged object)",
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
// the maps pinned under --pin-dir. CONFIG key 4 stays as it is, so rules
// written while it is 0 do nothing. Needs CAP_BPF.
//
// Before anything is written the file is held to FND-035's check: a
// require_hmac or require_retry validator needs a live flow-0 entry, in the
// file or already in the maps, because every Shield-born packet is flow 0
// (hoprules.CheckFlowZero). --allow-no-flow0 skips it, for a ring that carries
// no Shield-born packets.
//
// Entries are added or replaced. --prune, off by default, also deletes what
// the three rule maps hold and the file does not, after the file's own
// entries have landed, validators first; the flow-0 check then reads the file
// alone, because the maps' other entries are about to go. --dry-run runs
// every check, says what would be written and deleted, and changes nothing.
func newHopRulesApplyCommand() *Command {
	var file, pinDir string
	var allowNoFlowZero, prune, dryRun bool
	flags := flag.NewFlagSet("apply", flag.ContinueOnError)
	flags.StringVar(&file, "file", "", "Rules file (YAML; see pkg/hoprules)")
	flags.StringVar(&pinDir, "pin-dir", defaultHopPinDir, "Directory holding hop-ebpf's pinned maps")
	flags.BoolVar(&allowNoFlowZero, "allow-no-flow0", false, "Apply a require_hmac or require_retry validator with no live flow-0 entry (it fails every Shield-born packet; FND-035)")
	flags.BoolVar(&prune, "prune", false, "Also delete the validators, HMAC keys and retry entries the maps hold and the file does not")
	flags.BoolVar(&dryRun, "dry-run", false, "Run every check and say what would be written and deleted; change nothing")

	return &Command{
		Name:  "apply",
		Short: "Add or replace the entries in a rules file; with --prune, delete the rest",
		Usage: "unheaded hop rules apply --file <rules.yaml> [--pin-dir <dir>] [--prune] [--dry-run] [--allow-no-flow0]",
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
			clock := func() time.Duration { return now }
			w := &pinnedWriter{dir: pinDir, maps: make(map[string]hopMap)}
			defer w.Close()

			if allowNoFlowZero {
				ctx.Output.WriteWarning("flow-0 check skipped (--allow-no-flow0): a require_hmac or require_retry validator in %s may fail every Shield-born packet", file)
			} else {
				// A pruning apply leaves the maps holding the file and nothing
				// else, so the file must satisfy the check by itself.
				var current hoprules.MapReader
				if !prune {
					current = w
				}
				notes, err := hoprules.CheckFlowZero(f, current, clock)
				if err != nil {
					return fmt.Errorf("%s: %w; or pass --allow-no-flow0 if that namespace carries no Shield-born packets", file, err)
				}
				for _, n := range notes {
					ctx.Output.WriteWarning("%s", n)
				}
			}
			var stale []hoprules.Stale
			if prune {
				if stale, err = hoprules.StaleEntries(f, w); err != nil {
					return err
				}
			}

			if dryRun {
				would, err := hoprules.Apply(countingWriter{}, f, clock)
				if err != nil {
					return err
				}
				if err := ctx.Output.WriteStringln("would apply " + hopApplied(would)); err != nil {
					return err
				}
				for _, s := range stale {
					if err := ctx.Output.WriteStringln("would delete " + s.String()); err != nil {
						return err
					}
				}
				return ctx.Output.WriteStringln("dry run: nothing was written")
			}

			done, err := hoprules.Apply(w, f, clock)
			if err != nil {
				return fmt.Errorf("%w (already written: %s; nothing deleted)", err, hopApplied(done))
			}
			if err := ctx.Output.WriteStringln("applied " + hopApplied(done)); err != nil {
				return err
			}
			if !prune {
				return nil
			}
			gone, err := hoprules.Prune(w, stale)
			if err != nil {
				return fmt.Errorf("%w (already deleted: %s)", err, hopPruned(gone))
			}
			for _, s := range stale {
				if err := ctx.Output.WriteStringln("deleted " + s.String()); err != nil {
					return err
				}
			}
			return ctx.Output.WriteStringln("pruned " + hopPruned(gone))
		},
	}
}

// loadHopRules parses path, which hoprules refuses if its group or everyone
// can write it, and reads the key files it names.
func loadHopRules(path string) (*hoprules.File, error) {
	f, err := hoprules.ParseFile(path)
	if err != nil {
		return nil, err
	}
	if err := f.Load(); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return f, nil
}

func hopApplied(a hoprules.Applied) string {
	return fmt.Sprintf("%d validators, %d HMAC keys, %d retry entries", a.Validators, a.HMACKeys, a.Retry)
}

func hopPruned(p hoprules.Pruned) string {
	return fmt.Sprintf("%d validators, %d HMAC keys, %d retry entries", p.Validators, p.HMACKeys, p.Retry)
}
