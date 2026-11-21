// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

package cmd

import (
	"encoding/binary"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"unheaded/pkg/hoprules"
	"unheaded/pkg/protocol/bpfschema"
)

// fakePin is one pinned map; hop rules apply reaches it through openHopMap.
type fakePin struct {
	path    string
	entries map[string][]byte // what the map holds
	writes  [][2][]byte
	closed  int
	fail    error
}

func (m *fakePin) Update(key, value []byte) error {
	if m.fail != nil {
		return m.fail
	}
	m.writes = append(m.writes, [2][]byte{key, value})
	if m.entries == nil {
		m.entries = map[string][]byte{}
	}
	m.entries[string(key)] = value
	return nil
}

func (m *fakePin) Lookup(key []byte) ([]byte, error) {
	return m.entries[string(key)], nil
}

func (m *fakePin) Iterate(fn func(key, value []byte) error) error {
	for k, v := range m.entries {
		if err := fn([]byte(k), v); err != nil {
			return err
		}
	}
	return nil
}

func (m *fakePin) Close() error {
	m.closed++
	return nil
}

// fakePins swaps openHopMap for one that serves the given map paths.
func fakePins(t *testing.T) map[string]*fakePin {
	t.Helper()
	pins := map[string]*fakePin{}
	old := openHopMap
	openHopMap = func(path string) (hopMap, error) {
		if pins[path] == nil {
			pins[path] = &fakePin{path: path}
		}
		return pins[path], nil
	}
	t.Cleanup(func() { openHopMap = old })
	return pins
}

// rulesFile writes a key and a rules file naming it: one validator that asks
// for a key, the flow-0 key FND-035 wants beside it, and a key for flow 7.
func rulesFile(t *testing.T) string {
	t.Helper()
	return rulesFileWith(t, "validators:\n  - hop: 3\n    namespace: 1\n    require_hmac: true\nhmac_keys:\n  - flow: 7\n    namespace: 1\n    key_file: %[1]s\n  - flow: 0\n    namespace: 1\n    key_file: %[1]s\n")
}

// rulesFileWith writes a key file and a rules file whose body names it as
// %[1]s.
func rulesFileWith(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir()
	key := filepath.Join(dir, "lab.key")
	if err := os.WriteFile(key, make([]byte, 32), 0o400); err != nil {
		t.Fatal(err)
	}
	rules := filepath.Join(dir, "rules.yaml")
	if err := os.WriteFile(rules, []byte(fmt.Sprintf(body, key)), 0o600); err != nil {
		t.Fatal(err)
	}
	return rules
}

func TestHopRulesApply_WritesThroughThePins(t *testing.T) {
	pins := fakePins(t)
	ctx := testContext(DefaultConfig())
	err := run(t, ctx, NewHopCommand(), "rules apply", "--file", rulesFile(t), "--pin-dir", "/pins")
	if err != nil {
		t.Fatal(err)
	}
	v := pins["/pins/"+bpfschema.MapHopValidators]
	k := pins["/pins/"+bpfschema.MapHMACKeys]
	if v == nil || k == nil || len(pins) != 2 {
		t.Fatalf("pins opened: %v", pins)
	}
	if len(v.writes) != 1 || binary.LittleEndian.Uint64(v.writes[0][0]) != 3|1<<32 {
		t.Errorf("HOP_VALIDATORS writes = %x", v.writes)
	}
	if len(k.writes) != 2 || binary.LittleEndian.Uint64(k.writes[0][0]) != 7|1<<16 || binary.LittleEndian.Uint64(k.writes[1][0]) != 1<<16 {
		t.Errorf("HMAC_KEYS writes = %x", k.writes)
	}
	if v.closed != 1 || k.closed != 1 {
		t.Errorf("closed %d and %d times", v.closed, k.closed)
	}
	if out := testBuf.String(); !strings.Contains(out, "1 validators, 2 HMAC keys, 0 retry entries") {
		t.Errorf("output = %q", out)
	}
}

func TestHopRulesApply_DefaultPinDir(t *testing.T) {
	pins := fakePins(t)
	ctx := testContext(DefaultConfig())
	if err := run(t, ctx, NewHopCommand(), "rules apply", "--file", rulesFile(t)); err != nil {
		t.Fatal(err)
	}
	if pins["/sys/fs/bpf/unheaded/hop-ebpf/"+bpfschema.MapHopValidators] == nil {
		t.Fatalf("pins opened: %v", pins)
	}
}

func TestHopRulesApply_Refuses(t *testing.T) {
	bad := filepath.Join(t.TempDir(), "bad.yaml")
	if err := os.WriteFile(bad, []byte("nonsense: 1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	loose := rulesFile(t)
	if err := os.Chmod(loose, 0o664); err != nil {
		t.Fatal(err)
	}
	cases := map[string]struct {
		args []string
		want string
	}{
		"group-writable": {[]string{"--file", loose}, "group- or world-writable"},
		"no file":        {nil, "--file"},
		"missing file":   {[]string{"--file", bad + ".gone"}, "no such file"},
		"stray arg":      {[]string{"--file", bad, "extra"}, "unexpected argument"},
		"bad rules":      {[]string{"--file", bad}, "nonsense"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			pins := fakePins(t)
			err := run(t, testContext(DefaultConfig()), NewHopCommand(), "rules apply", c.args...)
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Fatalf("err = %v, want %q", err, c.want)
			}
			if len(pins) != 0 {
				t.Errorf("opened pins: %v", pins)
			}
		})
	}
}

// --file left out is a usage mistake: main exits 2.
func TestHopRulesApply_NoFileIsUsage(t *testing.T) {
	err := run(t, testContext(DefaultConfig()), NewHopCommand(), "rules apply")
	var ue *UsageError
	if !errors.As(err, &ue) {
		t.Fatalf("err = %T %v", err, err)
	}
}

// A failed write says what had landed, because the earlier writes stay.
func TestHopRulesApply_PartialWriteIsReported(t *testing.T) {
	boom := errors.New("map full")
	old := openHopMap
	t.Cleanup(func() { openHopMap = old })
	var opened []*fakePin
	openHopMap = func(path string) (hopMap, error) {
		p := &fakePin{path: path}
		if strings.HasSuffix(path, bpfschema.MapHopValidators) {
			p.fail = boom
		}
		opened = append(opened, p)
		return p, nil
	}
	err := run(t, testContext(DefaultConfig()), NewHopCommand(), "rules apply", "--file", rulesFile(t))
	if !errors.Is(err, boom) {
		t.Fatalf("err = %v", err)
	}
	if !strings.Contains(err.Error(), "0 validators, 2 HMAC keys, 0 retry entries") {
		t.Errorf("err = %v", err)
	}
	for _, p := range opened {
		if p.closed != 1 {
			t.Errorf("%s closed %d times", p.path, p.closed)
		}
	}
}

func TestHopRulesApply_NoPinIsAnError(t *testing.T) {
	boom := errors.New("no such pin")
	old := openHopMap
	t.Cleanup(func() { openHopMap = old })
	openHopMap = func(string) (hopMap, error) { return nil, boom }
	err := run(t, testContext(DefaultConfig()), NewHopCommand(), "rules apply", "--file", rulesFile(t))
	if !errors.Is(err, boom) {
		t.Fatalf("err = %v", err)
	}
}

func TestHopCommand_Registered(t *testing.T) {
	if NewRootCommand().SubCommands["hop"] == nil {
		t.Fatal("root has no hop command")
	}
}

func TestHopMode_RefusesEnforceAndWritesNothing(t *testing.T) {
	pins := fakePins(t)
	ctx := testContext(DefaultConfig())
	err := run(t, ctx, NewHopCommand(), "mode", "--pin-dir", "/pins", "enforce")
	if !errors.Is(err, hoprules.ErrEnforceRefused) {
		t.Fatalf("err = %v, want ErrEnforceRefused", err)
	}
	if len(pins) != 0 {
		t.Errorf("a refusal opened %v", pins)
	}
}

func TestHopMode_WritesObserve(t *testing.T) {
	pins := fakePins(t)
	ctx := testContext(DefaultConfig())
	if err := run(t, ctx, NewHopCommand(), "mode", "--pin-dir", "/pins", "observe"); err != nil {
		t.Fatal(err)
	}
	c := pins["/pins/CONFIG"]
	if c == nil || len(c.writes) != 1 || binary.LittleEndian.Uint32(c.writes[0][0]) != 4 || binary.LittleEndian.Uint64(c.writes[0][1]) != 1 {
		t.Fatalf("pins = %v", pins)
	}
	if c.closed != 1 {
		t.Errorf("closed %d times", c.closed)
	}
}

func TestHopMode_NeedsOneKnownMode(t *testing.T) {
	fakePins(t)
	for _, args := range [][]string{{}, {"drop"}, {"off", "observe"}} {
		ctx := testContext(DefaultConfig())
		if err := run(t, ctx, NewHopCommand(), "mode", args...); err == nil {
			t.Errorf("%v accepted", args)
		}
	}
}

// noFlowZero asks for a key on namespace 1 and gives flow 7 one: every
// Shield-born packet (flow 0) would fail the check (FND-035).
const noFlowZero = "validators:\n  - hop: 3\n    namespace: 1\n    require_hmac: true\nhmac_keys:\n  - flow: 7\n    namespace: 1\n    key_file: %[1]s\n"

func TestHopRulesApply_RefusesAValidatorWithNoFlowZeroEntry(t *testing.T) {
	pins := fakePins(t)
	err := run(t, testContext(DefaultConfig()), NewHopCommand(), "rules apply", "--file", rulesFileWith(t, noFlowZero), "--pin-dir", "/pins")
	if !errors.Is(err, hoprules.ErrNoFlowZero) {
		t.Fatalf("err = %v, want ErrNoFlowZero", err)
	}
	for _, s := range []string{"validators[0]", "namespace 1", "require_hmac", "--allow-no-flow0"} {
		if !strings.Contains(err.Error(), s) {
			t.Errorf("refusal %q does not say %q", err, s)
		}
	}
	for path, p := range pins {
		if len(p.writes) != 0 {
			t.Errorf("a refusal wrote to %s: %x", path, p.writes)
		}
		if p.closed != 1 {
			t.Errorf("%s closed %d times", path, p.closed)
		}
	}
}

// The entry may already be in the map, live, from an earlier apply.
func TestHopRulesApply_FlowZeroEntryInTheMapIsEnough(t *testing.T) {
	pins := fakePins(t)
	key := binary.LittleEndian.AppendUint64(nil, 1<<16) // flow 0, namespace 1
	pins["/pins/"+bpfschema.MapHMACKeys] = &fakePin{entries: map[string][]byte{string(key): make([]byte, 40)}}
	if err := run(t, testContext(DefaultConfig()), NewHopCommand(), "rules apply", "--file", rulesFileWith(t, noFlowZero), "--pin-dir", "/pins"); err != nil {
		t.Fatal(err)
	}
	if v := pins["/pins/"+bpfschema.MapHopValidators]; v == nil || len(v.writes) != 1 {
		t.Errorf("validator not written: %v", pins)
	}
}

// --allow-no-flow0 is the operator saying a ring carries no Shield-born
// packets: the file is applied and the output says the check did not run.
func TestHopRulesApply_AllowNoFlowZero(t *testing.T) {
	pins := fakePins(t)
	if err := run(t, testContext(DefaultConfig()), NewHopCommand(), "rules apply", "--file", rulesFileWith(t, noFlowZero), "--pin-dir", "/pins", "--allow-no-flow0"); err != nil {
		t.Fatal(err)
	}
	if v := pins["/pins/"+bpfschema.MapHopValidators]; v == nil || len(v.writes) != 1 {
		t.Errorf("validator not written: %v", pins)
	}
	if out := testBuf.String(); !strings.Contains(out, "flow-0 check skipped") {
		t.Errorf("output = %q", out)
	}
}

// A ttl on the flow-0 entry is the unattended failure: the output says so.
func TestHopRulesApply_NotesATTLOnFlowZero(t *testing.T) {
	fakePins(t)
	body := "validators:\n  - namespace: 1\n    require_hmac: true\nhmac_keys:\n  - flow: 0\n    namespace: 1\n    ttl: 24h\n    key_file: %[1]s\n"
	if err := run(t, testContext(DefaultConfig()), NewHopCommand(), "rules apply", "--file", rulesFileWith(t, body), "--pin-dir", "/pins"); err != nil {
		t.Fatal(err)
	}
	if out := testBuf.String(); !strings.Contains(out, "hmac_keys[0]") || !strings.Contains(out, "expires in 24h") {
		t.Errorf("output = %q", out)
	}
}
