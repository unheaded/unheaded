// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

//go:build linux

package packet

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/cilium/ebpf"
	"github.com/cilium/ebpf/link"
)

// LinuxBackend is the real kernel: cilium/ebpf for the object, maps and the
// TCX link, and the tc binary (never a shell) for the qdisc tree.
type LinuxBackend struct {
	// TCPath is the tc binary; NewLinuxBackend resolves it.
	TCPath string
}

// NewLinuxBackend finds tc in PATH, else /sbin/tc.
func NewLinuxBackend() *LinuxBackend {
	p, err := exec.LookPath("tc")
	if err != nil {
		p = "/sbin/tc"
	}
	return &LinuxBackend{TCPath: p}
}

// Present reports whether the interface exists in this network namespace.
func (b *LinuxBackend) Present(iface string) bool {
	_, err := net.InterfaceByName(iface)
	return err == nil
}

// Load loads the object with its maps pinned by name under pinDir and pins
// the classifier at pinDir/prog. The collection's own descriptors are
// closed: the pins keep everything alive until Unpin.
func (b *LinuxBackend) Load(objPath, pinDir string) error {
	if err := os.MkdirAll(pinDir, 0o700); err != nil {
		return err
	}
	spec, err := ebpf.LoadCollectionSpec(objPath)
	if err != nil {
		return fmt.Errorf("spec: %w", err)
	}
	for _, m := range spec.Maps {
		m.Pinning = ebpf.PinByName
	}
	coll, err := ebpf.NewCollectionWithOptions(spec, ebpf.CollectionOptions{Maps: ebpf.MapOptions{PinPath: pinDir}})
	if err != nil {
		var ve *ebpf.VerifierError
		if errors.As(err, &ve) {
			return fmt.Errorf("verifier: %v", ve)
		}
		return err
	}
	defer coll.Close()
	prog := coll.Programs[ProgramName]
	if prog == nil {
		return fmt.Errorf("object has no program %s", ProgramName)
	}
	return prog.Pin(filepath.Join(pinDir, PinProgram))
}

// Attach attaches the pinned classifier at TCX egress on iface and pins the link.
func (b *LinuxBackend) Attach(iface, pinDir string) error {
	ifi, err := net.InterfaceByName(iface)
	if err != nil {
		return err
	}
	prog, err := ebpf.LoadPinnedProgram(filepath.Join(pinDir, PinProgram), nil)
	if err != nil {
		return err
	}
	defer prog.Close()
	l, err := link.AttachTCX(link.TCXOptions{Interface: ifi.Index, Program: prog, Attach: ebpf.AttachTCXEgress})
	if err != nil {
		return err
	}
	defer l.Close()
	return l.Pin(filepath.Join(pinDir, PinLink))
}

func (b *LinuxBackend) pinnedMap(pinDir, name string) (*ebpf.Map, error) {
	m, err := ebpf.LoadPinnedMap(filepath.Join(pinDir, name), nil)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("%w: %s", ErrNotFound, name)
		}
		return nil, err
	}
	return m, nil
}

// WriteTargets puts every entry into CHAOS_TARGETS (u32 -> u64).
func (b *LinuxBackend) WriteTargets(pinDir string, targets map[uint32]uint64) error {
	m, err := b.pinnedMap(pinDir, MapTargets)
	if err != nil {
		return err
	}
	defer m.Close()
	for k, v := range targets {
		if err := m.Put(k, v); err != nil {
			return fmt.Errorf("key %#x: %w", k, err)
		}
	}
	return nil
}

// WriteConfig puts one CONFIG entry.
func (b *LinuxBackend) WriteConfig(pinDir string, key uint32, val uint64) error {
	m, err := b.pinnedMap(pinDir, MapConfig)
	if err != nil {
		return err
	}
	defer m.Close()
	return m.Put(key, val)
}

// ReadStats dumps STATS.
func (b *LinuxBackend) ReadStats(pinDir string) (map[uint32]uint64, error) {
	m, err := b.pinnedMap(pinDir, MapStats)
	if err != nil {
		return nil, err
	}
	defer m.Close()
	out := map[uint32]uint64{}
	var k uint32
	var v uint64
	it := m.Iterate()
	for it.Next(&k, &v) {
		out[k] = v
	}
	return out, it.Err()
}

// Detach unpins and closes the TCX link, which detaches the classifier.
func (b *LinuxBackend) Detach(pinDir string) error {
	l, err := link.LoadPinnedLink(filepath.Join(pinDir, PinLink), nil)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("%w: link", ErrNotFound)
		}
		return err
	}
	uerr := l.Unpin()
	cerr := l.Close()
	return errors.Join(uerr, cerr)
}

// DeleteTargets removes the keys; a key already absent is fine.
func (b *LinuxBackend) DeleteTargets(pinDir string, keys []uint32) error {
	m, err := b.pinnedMap(pinDir, MapTargets)
	if err != nil {
		return err
	}
	defer m.Close()
	var errs []error
	for _, k := range keys {
		if err := m.Delete(k); err != nil && !errors.Is(err, ebpf.ErrKeyNotExist) {
			errs = append(errs, fmt.Errorf("key %#x: %w", k, err))
		}
	}
	return errors.Join(errs...)
}

// pinNames are every pin Load and Attach create.
var pinNames = []string{PinLink, PinProgram, MapTargets, MapConfig, MapStats, MapEvents}

// Unpin removes the pins and the directory.
func (b *LinuxBackend) Unpin(pinDir string) error {
	var errs []error
	for _, n := range pinNames {
		if err := os.Remove(filepath.Join(pinDir, n)); err != nil && !errors.Is(err, os.ErrNotExist) {
			errs = append(errs, err)
		}
	}
	if err := os.Remove(pinDir); err != nil && !errors.Is(err, os.ErrNotExist) {
		errs = append(errs, err)
	}
	if len(errs) == 0 {
		if _, err := os.Stat(pinDir); errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return errors.New("pin directory still exists")
	}
	return errors.Join(errs...)
}

// Pinned reports whether the maps are still pinned.
func (b *LinuxBackend) Pinned(pinDir string) bool {
	_, err := os.Stat(filepath.Join(pinDir, MapTargets))
	return err == nil
}

// Linked reports whether the link pin is still there.
func (b *LinuxBackend) Linked(pinDir string) bool {
	_, err := os.Stat(filepath.Join(pinDir, PinLink))
	return err == nil
}

// goneTexts are tc's ways of saying the qdisc is not there.
var goneTexts = []string{"Cannot delete qdisc with handle of zero", "No such file or directory", "Cannot find device", "Invalid handle"}

// TC runs one tc argv (argv[0] is "tc"; the resolved binary is used).
func (b *LinuxBackend) TC(ctx context.Context, argv []string) error {
	if len(argv) < 2 || argv[0] != "tc" {
		return errors.New("tc argv must start with tc")
	}
	cmd := exec.CommandContext(ctx, b.TCPath, argv[1:]...) // #nosec G204 -- argv comes from TCArgv/TCRevertArgv, no shell
	out, err := cmd.CombinedOutput()
	if err == nil {
		return nil
	}
	text := strings.TrimSpace(string(out))
	for _, g := range goneTexts {
		if strings.Contains(text, g) {
			return fmt.Errorf("%w: %s", ErrNotFound, text)
		}
	}
	return fmt.Errorf("%w: %s", err, text)
}

// RootQdisc reports whether the c4a0: prio root is on iface.
func (b *LinuxBackend) RootQdisc(ctx context.Context, iface string) (bool, error) {
	cmd := exec.CommandContext(ctx, b.TCPath, "qdisc", "show", "dev", iface) // #nosec G204 -- fixed argv, iface matched ifaceRe
	out, err := cmd.CombinedOutput()
	if err != nil {
		if strings.Contains(string(out), "Cannot find device") {
			return false, nil
		}
		return false, fmt.Errorf("%w: %s", err, strings.TrimSpace(string(out)))
	}
	for _, line := range strings.Split(string(out), "\n") {
		if strings.HasPrefix(line, "qdisc prio "+RootHandle) {
			return true, nil
		}
	}
	return false, nil
}
