// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"unheaded/services/yaldabaoth/packet"
)

// recBackend is a packet.Backend that records calls and holds lab state.
type recBackend struct {
	mu     sync.Mutex
	ops    []string
	fail   map[string]error
	pinned map[string]bool
	linked map[string]bool
	qdisc  map[string]bool
}

func newRecBackend() *recBackend {
	return &recBackend{fail: map[string]error{}, pinned: map[string]bool{}, linked: map[string]bool{}, qdisc: map[string]bool{}}
}

func (b *recBackend) op(s string) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.ops = append(b.ops, s)
	return b.fail[strings.Fields(s)[0]]
}
func (b *recBackend) Present(iface string) bool { return iface == "steel0-h" }
func (b *recBackend) Load(obj, pin string) error {
	if err := b.op("load " + obj); err != nil {
		return err
	}
	b.pinned[pin] = true
	return nil
}
func (b *recBackend) Attach(iface, pin string) error {
	if err := b.op("attach " + iface); err != nil {
		return err
	}
	b.linked[pin] = true
	return nil
}
func (b *recBackend) WriteTargets(pin string, t map[uint32]uint64) error {
	return b.op(fmt.Sprintf("targets %d", len(t)))
}
func (b *recBackend) WriteConfig(string, uint32, uint64) error { return b.op("config") }
func (b *recBackend) ReadStats(string) (map[uint32]uint64, error) {
	return map[uint32]uint64{3: 7}, nil
}
func (b *recBackend) Detach(pin string) error {
	if err := b.op("detach"); err != nil {
		return err
	}
	delete(b.linked, pin)
	return nil
}
func (b *recBackend) DeleteTargets(string, []uint32) error { return b.op("deltargets") }
func (b *recBackend) Unpin(pin string) error {
	if err := b.op("unpin"); err != nil {
		return err
	}
	delete(b.pinned, pin)
	return nil
}
func (b *recBackend) Pinned(pin string) bool                              { return b.pinned[pin] }
func (b *recBackend) Linked(pin string) bool                              { return b.linked[pin] }
func (b *recBackend) RootQdisc(_ context.Context, i string) (bool, error) { return b.qdisc[i], nil }
func (b *recBackend) TC(_ context.Context, argv []string) error {
	if err := b.op("tc " + strings.Join(argv[1:], " ")); err != nil {
		return err
	}
	if argv[2] == "replace" {
		b.qdisc[argv[4]] = true
	}
	if argv[2] == "del" {
		if !b.qdisc[argv[4]] {
			return fmt.Errorf("%w: no qdisc", packet.ErrNotFound)
		}
		delete(b.qdisc, argv[4])
	}
	return nil
}

func (b *recBackend) has(prefix string) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	for _, o := range b.ops {
		if strings.HasPrefix(o, prefix) {
			return true
		}
	}
	return false
}

func noEnv(string) string { return "" }

func TestPacketFlags(t *testing.T) {
	good := []string{"--iface", "steel0-h", "--flow-label", "0x5EE1", "--flow-label", "24290", "--loss", "50", "--duration", "5m", "--state-dir", "/tmp/x"}
	c, err := parsePacketFlags(good, noEnv)
	if err != nil {
		t.Fatal(err)
	}
	if c.iface != "steel0-h" || len(c.labels) != 2 || c.loss != 50 || c.mode != "marker" || c.obj != packet.DefaultObject {
		t.Fatalf("cfg = %+v", c)
	}
	p := c.params()
	if p["loss_percent"] != 50.0 || p["mode"] != "marker" || len(p["flow_labels"].([]any)) != 2 || p["delay_ms"] != nil {
		t.Fatalf("params = %v", p)
	}
	c, err = parsePacketFlags([]string{"--iface", "steel0-h", "--flow-label", "1", "--dry-run"}, func(k string) string {
		return map[string]string{"YALD_OBJ": "/build/yaldabaoth-ebpf", "CHAOS_STATE_DIR": "/var/lib/x", "CHAOS_ABORT_FILE": "/run/x/ABORT"}[k]
	})
	if err != nil || c.obj != "/build/yaldabaoth-ebpf" || c.stateDir != "/var/lib/x" || c.abortFile != "/run/x/ABORT" {
		t.Fatalf("env cfg = %+v %v", c, err)
	}
	bad := map[string][]string{
		"no iface":      {"--flow-label", "1"},
		"no label":      {"--iface", "steel0-h"},
		"duration 0":    {"--iface", "steel0-h", "--flow-label", "1", "--duration", "0"},
		"duration long": {"--iface", "steel0-h", "--flow-label", "1", "--duration", "11m"},
		"hop id":        {"--iface", "steel0-h", "--flow-label", "1", "--hop-id", "256"},
		"stray arg":     {"--iface", "steel0-h", "--flow-label", "1", "extra"},
		"unknown flag":  {"--iface", "steel0-h", "--flow-label", "1", "--shell", "sh"},
	}
	for name, args := range bad {
		if _, err := parsePacketFlags(args, noEnv); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}

func TestPacketChaos_DryRunTouchesNothing(t *testing.T) {
	be := newRecBackend()
	var out bytes.Buffer
	code := runPacketChaos(context.Background(), []string{"--iface", "steel0-h", "--flow-label", "0x5EE1", "--loss", "50", "--dry-run"}, &out, be, noEnv)
	if code != 0 {
		t.Fatalf("exit %d: %s", code, out.String())
	}
	for _, want := range []string{
		"tc qdisc replace dev steel0-h root handle c4a0: prio bands 3",
		"tc filter add dev steel0-h parent c4a0: protocol ipv6 handle 0xC4A05 fw flowid c4a0:3",
		"tc qdisc add dev steel0-h parent c4a0:3 handle c4a3: netem loss 50%",
		"CHAOS_TARGETS[0x5ee1] = 0x0000000000000005",
		"attach yaldabaoth_tc tcx egress steel0-h",
		"dry run: nothing loaded, nothing attached",
	} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("missing %q in\n%s", want, out.String())
		}
	}
	if len(be.ops) != 0 {
		t.Fatalf("dry run acted: %v", be.ops)
	}
	// A host NIC never gets a plan, dry or not.
	out.Reset()
	if code := runPacketChaos(context.Background(), []string{"--iface", "enp16s0", "--flow-label", "1", "--dry-run"}, &out, be, noEnv); code == 0 {
		t.Fatalf("host NIC planned: %s", out.String())
	}
}

func TestPacketChaos_RunJournalsThenRevertsOnAbortFile(t *testing.T) {
	be := newRecBackend()
	dir := t.TempDir()
	abort := filepath.Join(dir, "ABORT")
	var out bytes.Buffer
	done := make(chan int)
	go func() {
		done <- runPacketChaos(context.Background(), []string{"--iface", "steel0-h", "--flow-label", "0x5EE1", "--loss", "50",
			"--duration", "5m", "--state-dir", dir, "--abort-file", abort}, &out, be, noEnv)
	}()
	deadline := time.Now().Add(5 * time.Second)
	for !be.has("attach") && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if !be.has("attach") {
		t.Fatalf("never attached: %s", out.String())
	}
	// The recipe is on disk while the fault is in place.
	raw, err := os.ReadFile(filepath.Join(dir, packetJournal))
	if err != nil {
		t.Fatal(err)
	}
	var j packetJournalFile
	if err := json.Unmarshal(raw, &j); err != nil || len(j.Entries) != 1 || j.Entries[0].Recipe["iface"] != "steel0-h" {
		t.Fatalf("journal = %s (%v)", raw, err)
	}
	if err := os.WriteFile(abort, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	select {
	case code := <-done:
		if code != 0 {
			t.Fatalf("exit %d: %s", code, out.String())
		}
	case <-time.After(5 * time.Second):
		t.Fatalf("abort file ignored: %s", out.String())
	}
	s := out.String()
	if !strings.Contains(s, "abort file "+abort) || !strings.Contains(s, "reverted and verified gone") || !strings.Contains(s, "STATS: targets_hit=7") {
		t.Fatalf("output:\n%s", s)
	}
	raw, _ = os.ReadFile(filepath.Join(dir, packetJournal))
	if err := json.Unmarshal(raw, &j); err != nil || len(j.Entries) != 0 {
		t.Fatalf("journal after revert = %s", raw)
	}
	if be.qdisc["steel0-h"] || len(be.linked) != 0 || len(be.pinned) != 0 {
		t.Fatalf("state left: %+v", be)
	}
}

func TestPacketChaos_LeftoverJournalIsRevertedFirst(t *testing.T) {
	be := newRecBackend()
	dir := t.TempDir()
	pin := packet.DefaultPinRoot + "/0123456789abcdef"
	be.pinned[pin], be.linked[pin], be.qdisc["steel0-h"] = true, true, true
	j := packetJournalFile{Version: 1, Entries: []packetJournalItem{{Recipe: map[string]string{
		"iface": "steel0-h", "pin_dir": pin, "flow_labels": "0x5ee1", "mode": "marker", "netem": "loss 50%", "qdisc": "c4a0:"}}}}
	raw, _ := json.Marshal(j)
	if err := os.WriteFile(filepath.Join(dir, packetJournal), raw, 0o600); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	code := runPacketChaos(context.Background(), []string{"--iface", "steel0-h", "--flow-label", "0x5EE2", "--duration", "10ms",
		"--state-dir", dir, "--abort-file", filepath.Join(dir, "none")}, &out, be, noEnv)
	if code != 0 {
		t.Fatalf("exit %d: %s", code, out.String())
	}
	s := out.String()
	if !strings.Contains(s, "reverting 1 journaled fault(s) first") || !strings.Contains(s, "0x5ee1 on steel0-h: reverted") {
		t.Fatalf("output:\n%s", s)
	}
	if be.pinned[pin] || be.linked[pin] {
		t.Fatal("leftover pins kept")
	}
}

func TestPacketChaos_InjectFailureReverts(t *testing.T) {
	be := newRecBackend()
	be.fail["attach"] = errors.New("tcx: not supported")
	dir := t.TempDir()
	var out bytes.Buffer
	code := runPacketChaos(context.Background(), []string{"--iface", "steel0-h", "--flow-label", "0x5EE1", "--loss", "50",
		"--duration", "1m", "--state-dir", dir, "--abort-file", filepath.Join(dir, "none")}, &out, be, noEnv)
	if code != 0 {
		t.Fatalf("exit %d: %s", code, out.String())
	}
	if !strings.Contains(out.String(), "inject: attach tcx egress steel0-h: tcx: not supported; reverting") {
		t.Fatalf("output:\n%s", out.String())
	}
	if be.qdisc["steel0-h"] || len(be.pinned) != 0 {
		t.Fatalf("qdisc or pins left after a failed inject: %+v", be)
	}
	raw, _ := os.ReadFile(filepath.Join(dir, packetJournal))
	var j packetJournalFile
	if err := json.Unmarshal(raw, &j); err != nil || len(j.Entries) != 0 {
		t.Fatalf("journal = %s", raw)
	}
}

func TestPacketChaos_RevertFailureKeepsTheJournal(t *testing.T) {
	be := newRecBackend()
	be.fail["detach"] = errors.New("link busy")
	dir := t.TempDir()
	var out bytes.Buffer
	code := runPacketChaos(context.Background(), []string{"--iface", "steel0-h", "--flow-label", "0x5EE1",
		"--duration", "10ms", "--state-dir", dir, "--abort-file", filepath.Join(dir, "none")}, &out, be, noEnv)
	if code != 1 {
		t.Fatalf("exit %d, want 1: %s", code, out.String())
	}
	raw, _ := os.ReadFile(filepath.Join(dir, packetJournal))
	var j packetJournalFile
	if err := json.Unmarshal(raw, &j); err != nil || len(j.Entries) != 1 {
		t.Fatalf("journal = %s", raw)
	}
}

func TestPacketChaos_UsageErrors(t *testing.T) {
	var out bytes.Buffer
	if code := runPacketChaos(context.Background(), []string{"--iface", "steel0-h"}, &out, newRecBackend(), noEnv); code != 2 {
		t.Fatalf("exit %d: %s", code, out.String())
	}
	out.Reset()
	// Refused by the plan (flow label 0) before anything is journaled.
	dir := t.TempDir()
	if code := runPacketChaos(context.Background(), []string{"--iface", "steel0-h", "--flow-label", "0", "--state-dir", dir}, &out, newRecBackend(), noEnv); code != 2 {
		t.Fatalf("exit %d: %s", code, out.String())
	}
	if _, err := os.Stat(filepath.Join(dir, packetJournal)); err == nil {
		t.Fatal("a refused plan wrote the journal")
	}
}
