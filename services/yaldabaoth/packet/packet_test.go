// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package packet

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"

	"unheaded/pkg/protocol/bpfschema"
	"unheaded/services/yaldabaoth"
)

// fake is a Backend that records every call in order and can fail any of them.
type fake struct {
	mu      sync.Mutex
	ops     []string
	fail    map[string]error
	present map[string]bool
	targets map[string]map[uint32]uint64
	config  map[string]map[uint32]uint64
	loaded  map[string]bool
	linked  map[string]string // pinDir -> iface
	qdisc   map[string]bool   // iface -> root c4a0: present
	stats   map[uint32]uint64
}

func newFake() *fake {
	return &fake{fail: map[string]error{}, present: map[string]bool{"steel0-h": true, "steel7-h": true, "enp16s0": true},
		targets: map[string]map[uint32]uint64{}, config: map[string]map[uint32]uint64{}, loaded: map[string]bool{},
		linked: map[string]string{}, qdisc: map[string]bool{}, stats: map[uint32]uint64{3: 500, 8: 500}}
}

func (f *fake) op(name string, args ...any) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.ops = append(f.ops, fmt.Sprintf(name, args...))
	return f.fail[strings.SplitN(fmt.Sprintf(name, args...), " ", 2)[0]]
}

func (f *fake) Present(iface string) bool { return f.present[iface] }
func (f *fake) Load(obj, pinDir string) error {
	if err := f.op("load %s %s", obj, pinDir); err != nil {
		return err
	}
	f.loaded[pinDir] = true
	return nil
}
func (f *fake) Attach(iface, pinDir string) error {
	if err := f.op("attach %s %s", iface, pinDir); err != nil {
		return err
	}
	f.linked[pinDir] = iface
	return nil
}
func (f *fake) WriteTargets(pinDir string, t map[uint32]uint64) error {
	if err := f.op("targets %s %v", pinDir, sortedKeys(t)); err != nil {
		return err
	}
	if f.targets[pinDir] == nil {
		f.targets[pinDir] = map[uint32]uint64{}
	}
	for k, v := range t {
		f.targets[pinDir][k] = v
	}
	return nil
}
func (f *fake) WriteConfig(pinDir string, k uint32, v uint64) error {
	if err := f.op("config %s %d=%d", pinDir, k, v); err != nil {
		return err
	}
	if f.config[pinDir] == nil {
		f.config[pinDir] = map[uint32]uint64{}
	}
	f.config[pinDir][k] = v
	return nil
}
func (f *fake) ReadStats(pinDir string) (map[uint32]uint64, error) {
	if err := f.op("stats %s", pinDir); err != nil {
		return nil, err
	}
	return f.stats, nil
}
func (f *fake) Detach(pinDir string) error {
	if err := f.op("detach %s", pinDir); err != nil {
		return err
	}
	delete(f.linked, pinDir)
	return nil
}
func (f *fake) DeleteTargets(pinDir string, keys []uint32) error {
	if err := f.op("deltargets %s %v", pinDir, keys); err != nil {
		return err
	}
	for _, k := range keys {
		delete(f.targets[pinDir], k)
	}
	return nil
}
func (f *fake) Unpin(pinDir string) error {
	if err := f.op("unpin %s", pinDir); err != nil {
		return err
	}
	delete(f.loaded, pinDir)
	return nil
}
func (f *fake) Pinned(pinDir string) bool { return f.loaded[pinDir] }
func (f *fake) Linked(pinDir string) bool { _, ok := f.linked[pinDir]; return ok }
func (f *fake) TC(_ context.Context, argv []string) error {
	if err := f.op("tc %s", strings.Join(argv[1:], " ")); err != nil {
		return err
	}
	switch {
	case len(argv) > 3 && argv[1] == "qdisc" && argv[2] == "replace":
		f.qdisc[argv[4]] = true
	case len(argv) > 3 && argv[1] == "qdisc" && argv[2] == "del":
		if !f.qdisc[argv[4]] {
			// What LinuxBackend.TC makes of tc's "Cannot delete qdisc with handle of zero."
			return fmt.Errorf("%w: Error: Cannot delete qdisc with handle of zero.", ErrNotFound)
		}
		delete(f.qdisc, argv[4])
	}
	return nil
}
func (f *fake) RootQdisc(_ context.Context, iface string) (bool, error) {
	if err := f.op("qdiscshow %s", iface); err != nil {
		return false, err
	}
	return f.qdisc[iface], nil
}

func (f *fake) kinds() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]string, 0, len(f.ops))
	for _, o := range f.ops {
		out = append(out, strings.SplitN(o, " ", 2)[0])
	}
	return out
}

func sortedKeys(m map[uint32]uint64) []uint32 {
	ks := make([]uint32, 0, len(m))
	for k := range m {
		ks = append(ks, k)
	}
	for i := range ks {
		for j := i + 1; j < len(ks); j++ {
			if ks[j] < ks[i] {
				ks[i], ks[j] = ks[j], ks[i]
			}
		}
	}
	return ks
}

func lab(fl ...any) yaldabaoth.Params {
	return yaldabaoth.Params{"flow_labels": fl, "mode": "marker", "loss_percent": 50.0}
}

func target(iface string) yaldabaoth.ResolvedTarget {
	return yaldabaoth.ResolvedTarget{Kind: yaldabaoth.KindPacket, ID: iface}
}

func newInj(f *fake) *Injector {
	return New(f, Options{Object: "/obj/yaldabaoth-ebpf", PinRoot: "/sys/fs/bpf/unheaded/chaos", HopID: 9})
}

// --- encoding ---

func TestEncodeTarget_MatchesBpfschema(t *testing.T) {
	// Program side: value = (mode as u64) | ((param as u64) << 32).
	// bpfschema.ChaosTargetValue{Mode, Param} is the same 8 bytes, little-endian.
	for _, tc := range []struct{ mode, param uint32 }{{bpfschema.ChaosMarker, 0}, {bpfschema.ChaosDelay, 1500}, {bpfschema.ChaosBitFlip, 0xFFFFFFFF}} {
		var want bytes.Buffer
		if err := binary.Write(&want, binary.LittleEndian, bpfschema.ChaosTargetValue{Mode: tc.mode, Param: tc.param}); err != nil {
			t.Fatal(err)
		}
		var got [8]byte
		binary.LittleEndian.PutUint64(got[:], EncodeTarget(tc.mode, tc.param))
		if !bytes.Equal(want.Bytes(), got[:]) {
			t.Errorf("mode %d param %d: % x != % x", tc.mode, tc.param, got, want.Bytes())
		}
		if EncodeTarget(tc.mode, tc.param)&0xFF != uint64(tc.mode)&0xFF || EncodeTarget(tc.mode, tc.param)>>32 != uint64(tc.param) {
			t.Errorf("mode %d param %d: fields misplaced", tc.mode, tc.param)
		}
	}
}

func TestKey_MasksAndRefusesZero(t *testing.T) {
	if k, err := Key(0x5EE1); err != nil || k != 0x5EE1 {
		t.Fatalf("0x5EE1 -> %#x %v", k, err)
	}
	if k, err := Key(0xFFFFF); err != nil || k != 0xFFFFF {
		t.Fatalf("0xFFFFF -> %#x %v", k, err)
	}
	for _, bad := range []uint32{0, 0x100000, 0xF05EE1} {
		if _, err := Key(bad); err == nil {
			t.Errorf("%#x accepted (would alias to %#x in the map)", bad, bad&0xFFFFF)
		}
	}
}

func TestModeCode(t *testing.T) {
	want := map[string]uint32{"marker": bpfschema.ChaosMarker, "bitflip": bpfschema.ChaosBitFlip, "delay": bpfschema.ChaosDelay,
		"duplicate": bpfschema.ChaosDuplicate, "truncate": bpfschema.ChaosTruncate, "": bpfschema.ChaosMarker}
	for name, code := range want {
		if got, err := ModeCode(name); err != nil || got != code {
			t.Errorf("%q = %d %v, want %d", name, got, err, code)
		}
	}
	for _, bad := range []string{"MARKER", "drop", "5", "marker "} {
		if _, err := ModeCode(bad); err == nil {
			t.Errorf("%q accepted", bad)
		}
	}
}

// --- spec ---

func TestParseSpec(t *testing.T) {
	s, err := ParseSpec("steel0-h", yaldabaoth.Params{"flow_labels": []any{"0x5EE1", 24290.0}, "mode": "delay",
		"delay_ms": 100.0, "jitter_ms": 10.0, "loss_percent": 50.0, "duplicate_percent": 5.0})
	if err != nil {
		t.Fatal(err)
	}
	if s.Iface != "steel0-h" || !reflect.DeepEqual(s.FlowLabels, []uint32{0x5EE1, 0x5EE2}) || s.Mode != bpfschema.ChaosDelay || s.Param != 100000 {
		t.Fatalf("spec = %+v", s)
	}
	if got := s.Netem(); !reflect.DeepEqual(got, []string{"delay", "100ms", "10ms", "loss", "50%", "duplicate", "5%"}) {
		t.Fatalf("netem = %q", got)
	}
	// Marker with loss only, labels as JSON numbers (what the HTTP body decodes to).
	s, err = ParseSpec("steel0-h", lab(24289.0))
	if err != nil || s.Mode != bpfschema.ChaosMarker || s.Param != 0 || !reflect.DeepEqual(s.Netem(), []string{"loss", "50%"}) {
		t.Fatalf("marker spec = %+v %v", s, err)
	}
	// No netem parameters at all: the program marks, nothing shapes.
	s, err = ParseSpec("steel0-h", yaldabaoth.Params{"flow_labels": []any{"0x5EE1"}, "mode": "bitflip"})
	if err != nil || s.Netem() != nil {
		t.Fatalf("bitflip spec = %+v %v", s, err)
	}
}

func TestParseSpec_Refusals(t *testing.T) {
	bad := map[string]yaldabaoth.Params{
		"no labels":        {"mode": "marker", "loss_percent": 1.0},
		"empty labels":     {"flow_labels": []any{}},
		"label 0":          lab(0.0),
		"label > 20 bits":  lab(float64(0x100000)),
		"label fraction":   lab(1.5),
		"label garbage":    lab("5EE1"),
		"label type":       lab(true),
		"labels not list":  {"flow_labels": "0x5EE1"},
		"duplicate label":  lab("0x5EE1", 24289.0),
		"too many labels":  {"flow_labels": manyLabels(MaxFlowLabels + 1)},
		"bad mode":         {"flow_labels": []any{"0x5EE1"}, "mode": "drop"},
		"mode not string":  {"flow_labels": []any{"0x5EE1"}, "mode": 5.0},
		"loss > 100":       {"flow_labels": []any{"0x5EE1"}, "loss_percent": 101.0},
		"negative delay":   {"flow_labels": []any{"0x5EE1"}, "delay_ms": -1.0},
		"delay too long":   {"flow_labels": []any{"0x5EE1"}, "delay_ms": 70000.0},
		"duplicate > 100":  {"flow_labels": []any{"0x5EE1"}, "duplicate_percent": 100.5},
		"unknown param":    {"flow_labels": []any{"0x5EE1"}, "rate": 1.0},
		"jitter w/o delay": {"flow_labels": []any{"0x5EE1"}, "jitter_ms": 5.0},
	}
	for name, p := range bad {
		if _, err := ParseSpec("steel0-h", p); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}

func manyLabels(n int) []any {
	out := make([]any, n)
	for i := range out {
		out[i] = float64(i + 1)
	}
	return out
}

// --- interface ---

func TestCheckIface(t *testing.T) {
	present := func(n string) bool { return n == "steel0-h" || n == "enp16s0" || n == "steel0-n" }
	if err := CheckIface("steel0-h", present); err != nil {
		t.Fatal(err)
	}
	for _, bad := range []string{"enp16s0", "steel0-n", "steel-h", "steel0-h ", "xsteel0-h", "steel0-h\n", "steel9-h", "lo", "", "steel0-h/../enp16s0"} {
		if err := CheckIface(bad, present); err == nil {
			t.Errorf("%q accepted", bad)
		}
	}
}

// --- tc argv (D-24) ---

func TestTCArgv_IsExactlyD24(t *testing.T) {
	want := [][]string{
		{"tc", "qdisc", "replace", "dev", "steel0-h", "root", "handle", "c4a0:", "prio", "bands", "3"},
		{"tc", "filter", "add", "dev", "steel0-h", "parent", "c4a0:", "protocol", "ipv6", "handle", "0xC4A05", "fw", "flowid", "c4a0:3"},
		{"tc", "qdisc", "add", "dev", "steel0-h", "parent", "c4a0:3", "handle", "c4a3:", "netem", "loss", "50%"},
	}
	got := TCArgv("steel0-h", []string{"loss", "50%"})
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("argv =\n%q\nwant\n%q", got, want)
	}
	if got := TCRevertArgv("steel0-h"); !reflect.DeepEqual(got, []string{"tc", "qdisc", "del", "dev", "steel0-h", "root", "handle", "c4a0:"}) {
		t.Fatalf("revert argv = %q", got)
	}
	if TCArgv("steel0-h", nil) != nil {
		t.Fatal("no netem must mean no qdisc tree")
	}
	if Mark != 0xC4A05 || fmt.Sprintf("0x%X", Mark) != "0xC4A05" {
		t.Fatalf("mark = %#x", Mark)
	}
}

// --- injector: plan / inject / revert order ---

func TestPlan_RecipeAndRefusals(t *testing.T) {
	f := newFake()
	inj := newInj(f)
	r, err := inj.Plan(context.Background(), target("steel0-h"), lab("0x5EE1"))
	if err != nil {
		t.Fatal(err)
	}
	if r["iface"] != "steel0-h" || r["flow_labels"] != "0x5ee1" || !strings.HasPrefix(r["pin_dir"], "/sys/fs/bpf/unheaded/chaos/") || r["qdisc"] != "c4a0:" || r["netem"] != "loss 50%" {
		t.Fatalf("recipe = %v", r)
	}
	if strings.Contains(strings.TrimPrefix(r["pin_dir"], "/sys/fs/bpf/unheaded/chaos/"), "/") {
		t.Fatalf("pin dir escapes the root: %s", r["pin_dir"])
	}
	r2, _ := inj.Plan(context.Background(), target("steel0-h"), lab("0x5EE1"))
	if r2["pin_dir"] == r["pin_dir"] {
		t.Fatal("two plans share a pin dir")
	}
	if len(f.ops) != 0 {
		t.Fatalf("plan acted: %v", f.ops)
	}
	for name, tgt := range map[string]yaldabaoth.ResolvedTarget{
		"host nic":        target("enp16s0"),
		"absent lab":      target("steel3-h"),
		"wrong kind":      {Kind: yaldabaoth.KindNetns, ID: "steel0-h"},
		"netns side":      target("steel0-n"),
		"lxd instance":    {Kind: yaldabaoth.KindLXD, ID: "steel-echo-v2-0"},
		"empty interface": target(""),
	} {
		if _, err := inj.Plan(context.Background(), tgt, lab("0x5EE1")); err == nil {
			t.Errorf("%s: planned", name)
		}
	}
	if _, err := inj.Plan(context.Background(), target("steel0-h"), lab(0.0)); err == nil {
		t.Error("flow label 0 planned")
	}
}

func TestInject_OrderAndMapContents(t *testing.T) {
	f := newFake()
	inj := newInj(f)
	ctx := context.Background()
	p := yaldabaoth.Params{"flow_labels": []any{"0x5EE1", "0x5EE3"}, "mode": "delay", "delay_ms": 100.0, "loss_percent": 50.0}
	r, err := inj.Plan(ctx, target("steel0-h"), p)
	if err != nil {
		t.Fatal(err)
	}
	if err := inj.Inject(ctx, target("steel0-h"), p, r); err != nil {
		t.Fatal(err)
	}
	// Maps and qdisc tree before the classifier attaches: the first marked
	// packet already finds its netem band.
	want := []string{"load", "config", "targets", "tc", "tc", "tc", "attach"}
	if got := f.kinds(); !reflect.DeepEqual(got, want) {
		t.Fatalf("inject order = %v, want %v", got, want)
	}
	pin := r["pin_dir"]
	if f.config[pin][0] != 9 {
		t.Errorf("CONFIG[0] (hop_id) = %d, want 9", f.config[pin][0])
	}
	wantTargets := map[uint32]uint64{0x5EE1: EncodeTarget(bpfschema.ChaosDelay, 100000), 0x5EE3: EncodeTarget(bpfschema.ChaosDelay, 100000)}
	if !reflect.DeepEqual(f.targets[pin], wantTargets) {
		t.Errorf("CHAOS_TARGETS = %v, want %v", f.targets[pin], wantTargets)
	}
	if f.ops[0] != "load /obj/yaldabaoth-ebpf "+pin {
		t.Errorf("load = %q", f.ops[0])
	}
	if f.ops[3] != "tc qdisc replace dev steel0-h root handle c4a0: prio bands 3" ||
		f.ops[4] != "tc filter add dev steel0-h parent c4a0: protocol ipv6 handle 0xC4A05 fw flowid c4a0:3" ||
		f.ops[5] != "tc qdisc add dev steel0-h parent c4a0:3 handle c4a3: netem delay 100ms loss 50%" {
		t.Errorf("tc = %q", f.ops[3:6])
	}
	if f.ops[6] != "attach steel0-h "+pin {
		t.Errorf("attach = %q", f.ops[6])
	}
}

func TestInject_RechecksTheInterface(t *testing.T) {
	f := newFake()
	inj := newInj(f)
	ctx := context.Background()
	r, err := inj.Plan(ctx, target("steel0-h"), lab("0x5EE1"))
	if err != nil {
		t.Fatal(err)
	}
	// The interface vanished (netns torn down) between Plan and Inject.
	f.present["steel0-h"] = false
	if err := inj.Inject(ctx, target("steel0-h"), lab("0x5EE1"), r); err == nil || len(f.ops) != 0 {
		t.Fatalf("inject on a missing interface: err=%v ops=%v", err, f.ops)
	}
	// A recipe for another interface than the target is refused too.
	f.present["steel0-h"] = true
	r["iface"] = "enp16s0"
	if err := inj.Inject(ctx, target("steel0-h"), lab("0x5EE1"), r); err == nil || len(f.ops) != 0 {
		t.Fatalf("recipe/target mismatch: err=%v ops=%v", err, f.ops)
	}
}

func TestRevert_OrderAndVerify(t *testing.T) {
	f := newFake()
	inj := newInj(f)
	ctx := context.Background()
	r, _ := inj.Plan(ctx, target("steel0-h"), lab("0x5EE1", "0x5EE2"))
	if err := inj.Inject(ctx, target("steel0-h"), lab("0x5EE1", "0x5EE2"), r); err != nil {
		t.Fatal(err)
	}
	if gone, _ := inj.Reverted(ctx, r); gone {
		t.Fatal("reported reverted while in place")
	}
	f.ops = nil
	if err := inj.Revert(ctx, r); err != nil {
		t.Fatal(err)
	}
	// D-24 revert order: detach link, delete map entries, delete root qdisc, unpin maps.
	// Stats are read before the pins go (the experiment record).
	want := []string{"stats", "detach", "deltargets", "tc", "unpin"}
	if got := f.kinds(); !reflect.DeepEqual(got, want) {
		t.Fatalf("revert order = %v, want %v", got, want)
	}
	if f.ops[2] != "deltargets "+r["pin_dir"]+" [24289 24290]" {
		t.Errorf("deltargets = %q", f.ops[2])
	}
	if f.ops[3] != "tc qdisc del dev steel0-h root handle c4a0:" {
		t.Errorf("tc = %q", f.ops[3])
	}
	gone, err := inj.Reverted(ctx, r)
	if err != nil || !gone {
		t.Fatalf("reverted = %v %v", gone, err)
	}
	st := inj.LastStats()
	if st[StatTargetsHit] != 500 {
		t.Fatalf("stats = %v", st)
	}
	// A second revert is a no-op that does not fail (TTL, abort and
	// revert-all may all call it).
	f.ops = nil
	if err := inj.Revert(ctx, r); err != nil {
		t.Fatalf("second revert: %v", err)
	}
}

func TestRevert_ContinuesPastFailuresAndReportsThem(t *testing.T) {
	f := newFake()
	inj := newInj(f)
	ctx := context.Background()
	r, _ := inj.Plan(ctx, target("steel0-h"), lab("0x5EE1"))
	if err := inj.Inject(ctx, target("steel0-h"), lab("0x5EE1"), r); err != nil {
		t.Fatal(err)
	}
	f.ops = nil
	f.fail["detach"] = errors.New("link: permission denied")
	err := inj.Revert(ctx, r)
	if err == nil || !strings.Contains(err.Error(), "permission denied") {
		t.Fatalf("revert err = %v", err)
	}
	// Every later step still ran.
	want := []string{"stats", "detach", "deltargets", "tc", "unpin"}
	if got := f.kinds(); !reflect.DeepEqual(got, want) {
		t.Fatalf("order = %v", got)
	}
	if gone, _ := inj.Reverted(ctx, r); gone {
		t.Fatal("link still attached but reported reverted")
	}
}

func TestInject_FailureLeavesARevertableState(t *testing.T) {
	f := newFake()
	inj := newInj(f)
	ctx := context.Background()
	r, _ := inj.Plan(ctx, target("steel0-h"), lab("0x5EE1"))
	f.fail["attach"] = errors.New("tcx: operation not supported")
	if err := inj.Inject(ctx, target("steel0-h"), lab("0x5EE1"), r); err == nil {
		t.Fatal("inject succeeded with a failed attach")
	}
	// The Service calls Revert after a failed Inject: it must take back
	// the qdisc tree and the pins that did land.
	if err := inj.Revert(ctx, r); err != nil {
		t.Fatalf("revert after a half inject: %v", err)
	}
	if gone, _ := inj.Reverted(ctx, r); !gone || f.qdisc["steel0-h"] || f.loaded[r["pin_dir"]] {
		t.Fatalf("state left behind: qdisc=%v pinned=%v", f.qdisc["steel0-h"], f.loaded[r["pin_dir"]])
	}
}

func TestRecipe_RejectsTamperedPinDir(t *testing.T) {
	f := newFake()
	inj := newInj(f)
	ctx := context.Background()
	r, _ := inj.Plan(ctx, target("steel0-h"), lab("0x5EE1"))
	for _, bad := range []string{"/sys/fs/bpf", "/sys/fs/bpf/unheaded/chaos/../../x", "/etc", "", "/sys/fs/bpf/unheaded/chaos/a/b"} {
		rr := yaldabaoth.Recipe{}
		for k, v := range r {
			rr[k] = v
		}
		rr["pin_dir"] = bad
		if err := inj.Revert(ctx, rr); err == nil {
			t.Errorf("pin_dir %q reverted", bad)
		}
		if err := inj.Inject(ctx, target("steel0-h"), lab("0x5EE1"), rr); err == nil {
			t.Errorf("pin_dir %q injected", bad)
		}
	}
	if len(f.ops) != 0 {
		t.Fatalf("tampered recipes reached the backend: %v", f.ops)
	}
}

// --- dry run ---

func TestDryRun_PrintsThePlanAndTouchesNothing(t *testing.T) {
	f := newFake()
	inj := newInj(f)
	var out bytes.Buffer
	if err := inj.DryRun(context.Background(), &out, target("steel0-h"), lab("0x5EE1")); err != nil {
		t.Fatal(err)
	}
	s := out.String()
	for _, want := range []string{
		"load /obj/yaldabaoth-ebpf -> pin /sys/fs/bpf/unheaded/chaos/",
		"CONFIG[0] = 9",
		"CHAOS_TARGETS[0x5ee1] = 0x0000000000000005",
		"tc qdisc replace dev steel0-h root handle c4a0: prio bands 3",
		"tc filter add dev steel0-h parent c4a0: protocol ipv6 handle 0xC4A05 fw flowid c4a0:3",
		"tc qdisc add dev steel0-h parent c4a0:3 handle c4a3: netem loss 50%",
		"attach yaldabaoth_tc tcx egress steel0-h",
		"revert: detach link, delete CHAOS_TARGETS[0x5ee1], tc qdisc del dev steel0-h root handle c4a0:, unpin",
	} {
		if !strings.Contains(s, want) {
			t.Errorf("dry run lacks %q:\n%s", want, s)
		}
	}
	if len(f.ops) != 0 {
		t.Fatalf("dry run acted: %v", f.ops)
	}
	if err := inj.DryRun(context.Background(), &out, target("enp16s0"), lab("0x5EE1")); err == nil {
		t.Fatal("dry run on a host NIC planned")
	}
}

// --- default object path ---

func TestObjectPath(t *testing.T) {
	dir := t.TempDir()
	obj := filepath.Join(dir, "yaldabaoth-ebpf")
	if err := os.WriteFile(obj, []byte("ELF"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := ObjectPath(func(string) string { return "" }); got != DefaultObject {
		t.Errorf("default = %q", got)
	}
	if got := ObjectPath(func(k string) string {
		if k == "YALD_OBJ" {
			return obj
		}
		return ""
	}); got != obj {
		t.Errorf("env = %q", got)
	}
}
