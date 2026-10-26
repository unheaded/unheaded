// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package yaldabaoth

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
)

// fakeRunner records every command and never runs anything. A shell would
// arrive as name "sh"/"bash"; the tests assert the name is always the fixed
// binary, so a shell can never slip in.
type fakeRunner struct {
	mu          sync.Mutex
	calls       [][]string // each entry is {name, args...}
	showJSON    string     // tc -j qdisc show output
	showErr     error
	failReplace error
	failDel     error
	failGeneric error
}

func (f *fakeRunner) Run(_ context.Context, name string, args ...string) ([]byte, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, append([]string{name}, args...))
	has := func(s string) bool {
		for _, a := range args {
			if a == s {
				return true
			}
		}
		return false
	}
	switch {
	case name == "tc" && has("show"):
		return []byte(f.showJSON), f.showErr
	case name == "tc" && has("replace"):
		return nil, f.failReplace
	case name == "tc" && has("del"):
		return nil, f.failDel
	default:
		return nil, f.failGeneric
	}
}

func (f *fakeRunner) last(verb string) []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	for i := len(f.calls) - 1; i >= 0; i-- {
		for _, a := range f.calls[i] {
			if a == verb {
				return f.calls[i]
			}
		}
	}
	return nil
}

func (f *fakeRunner) argv(verb string) string {
	return strings.Join(f.last(verb), " ")
}

func (f *fakeRunner) nonTC() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	var bad []string
	for _, c := range f.calls {
		if len(c) > 0 && c[0] != "tc" {
			bad = append(bad, c[0])
		}
	}
	return bad
}

// vethResolver answers one instance with a fixed host veth.
type vethResolver struct {
	veth    string
	missing bool
}

func (r vethResolver) Instance(_ context.Context, name string) (*InstanceInfo, error) {
	if r.missing {
		return nil, ErrInstanceNotFound
	}
	return &InstanceInfo{Name: name, Status: "running", Managed: true, Project: "unheaded",
		Labels: map[string]string{"chaos": "allowed"}, HostVeth: r.veth}, nil
}

func netemInjector(t *testing.T, mode NetemMode, run CommandRunner, res Resolver) *NetemInjector {
	t.Helper()
	inj, err := NewNetemInjector(mode, run, res, nil)
	if err != nil {
		t.Fatalf("NewNetemInjector: %v", err)
	}
	return inj
}

func planInject(t *testing.T, inj *NetemInjector, rt ResolvedTarget, p Params) Recipe {
	t.Helper()
	r, err := inj.Plan(t.Context(), rt, p)
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}
	if err := inj.Inject(t.Context(), rt, p, r); err != nil {
		t.Fatalf("Inject: %v", err)
	}
	return r
}

func TestNetem_DelayArgvExact(t *testing.T) {
	run := &fakeRunner{}
	inj := netemInjector(t, NetemDelay, run, vethResolver{veth: "vethsteel"})
	rt := ResolvedTarget{Kind: KindLXD, ID: "steel-echo-v2-0"}
	planInject(t, inj, rt, Params{"delay_ms": 200.0, "jitter_ms": 20.0})
	want := "tc qdisc replace dev vethsteel root handle c4a0: netem delay 200ms 20ms"
	if got := run.argv("replace"); got != want {
		t.Errorf("replace argv:\n got %q\nwant %q", got, want)
	}
	if bad := run.nonTC(); bad != nil {
		t.Errorf("ran non-tc binaries: %v", bad)
	}
}

func TestNetem_DelayNoJitter(t *testing.T) {
	run := &fakeRunner{}
	inj := netemInjector(t, NetemDelay, run, vethResolver{veth: "vethsteel"})
	planInject(t, inj, ResolvedTarget{Kind: KindLXD, ID: "x"}, Params{"delay_ms": 50.0})
	want := "tc qdisc replace dev vethsteel root handle c4a0: netem delay 50ms"
	if got := run.argv("replace"); got != want {
		t.Errorf("got %q want %q", got, want)
	}
}

func TestNetem_LossArgvExact(t *testing.T) {
	run := &fakeRunner{}
	inj := netemInjector(t, NetemLoss, run, vethResolver{veth: "vethsteel"})
	planInject(t, inj, ResolvedTarget{Kind: KindLXD, ID: "x"}, Params{"loss_percent": 10.0})
	want := "tc qdisc replace dev vethsteel root handle c4a0: netem loss 10%"
	if got := run.argv("replace"); got != want {
		t.Errorf("got %q want %q", got, want)
	}
}

func TestNetem_PartitionIsFullLoss(t *testing.T) {
	run := &fakeRunner{}
	inj := netemInjector(t, NetemPartition, run, vethResolver{veth: "vethsteel"})
	// Partition ignores params: always 100% loss.
	planInject(t, inj, ResolvedTarget{Kind: KindLXD, ID: "x"}, Params{"loss_percent": 10.0})
	want := "tc qdisc replace dev vethsteel root handle c4a0: netem loss 100%"
	if got := run.argv("replace"); got != want {
		t.Errorf("got %q want %q", got, want)
	}
}

func TestNetem_DuplicateAndCorrupt(t *testing.T) {
	for _, tc := range []struct {
		mode NetemMode
		want string
	}{
		{NetemDuplicate, "tc qdisc replace dev vethsteel root handle c4a0: netem duplicate 5%"},
		{NetemCorrupt, "tc qdisc replace dev vethsteel root handle c4a0: netem corrupt 5%"},
	} {
		run := &fakeRunner{}
		inj := netemInjector(t, tc.mode, run, vethResolver{veth: "vethsteel"})
		planInject(t, inj, ResolvedTarget{Kind: KindLXD, ID: "x"}, Params{"loss_percent": 5.0})
		if got := run.argv("replace"); got != tc.want {
			t.Errorf("%s: got %q want %q", tc.mode, got, tc.want)
		}
	}
}

func TestNetem_NetnsAndPacketInterfaces(t *testing.T) {
	run := &fakeRunner{}
	inj := netemInjector(t, NetemDelay, run, nil)
	// A lab netns steel3 is shaped on its host-side veth steel3-h.
	planInject(t, inj, ResolvedTarget{Kind: KindNetns, ID: "steel3"}, Params{"delay_ms": 10.0})
	if got := run.argv("replace"); !strings.Contains(got, "dev steel3-h ") {
		t.Errorf("netns iface: %q", got)
	}
	run2 := &fakeRunner{}
	inj2 := netemInjector(t, NetemDelay, run2, nil)
	planInject(t, inj2, ResolvedTarget{Kind: KindPacket, ID: "steel3-h"}, Params{"delay_ms": 10.0})
	if got := run2.argv("replace"); !strings.Contains(got, "dev steel3-h ") {
		t.Errorf("packet iface: %q", got)
	}
}

func TestNetem_RefusesNonBridgedInstance(t *testing.T) {
	inj := netemInjector(t, NetemDelay, &fakeRunner{}, vethResolver{veth: ""})
	_, err := inj.Plan(t.Context(), ResolvedTarget{Kind: KindLXD, ID: "x"}, Params{"delay_ms": 10.0})
	if err == nil || !strings.Contains(err.Error(), "host veth") {
		t.Fatalf("err = %v, want host-veth refusal", err)
	}
}

func TestNetem_RefusesMissingInstance(t *testing.T) {
	inj := netemInjector(t, NetemDelay, &fakeRunner{}, vethResolver{missing: true})
	if _, err := inj.Plan(t.Context(), ResolvedTarget{Kind: KindLXD, ID: "x"}, Params{"delay_ms": 10.0}); err == nil {
		t.Fatal("planned against a missing instance")
	}
}

func TestNetem_DelayRequiresValue(t *testing.T) {
	inj := netemInjector(t, NetemDelay, &fakeRunner{}, vethResolver{veth: "v"})
	if _, err := inj.Plan(t.Context(), ResolvedTarget{Kind: KindLXD, ID: "x"}, Params{}); err == nil {
		t.Fatal("delay with no delay_ms accepted")
	}
}

func TestNetem_RevertArgvAndVerify(t *testing.T) {
	// A netem qdisc with our handle is still in place: not reverted.
	run := &fakeRunner{showJSON: `[{"kind":"netem","handle":"c4a0:","parent":"root"},{"kind":"noqueue","handle":"0:"}]`}
	inj := netemInjector(t, NetemDelay, run, vethResolver{veth: "vethsteel"})
	recipe := Recipe{"iface": "vethsteel", "handle": NetemHandle}
	if err := inj.Revert(t.Context(), recipe); err != nil {
		t.Fatalf("Revert: %v", err)
	}
	want := "tc qdisc del dev vethsteel root handle c4a0:"
	if got := run.argv("del"); got != want {
		t.Errorf("del argv:\n got %q\nwant %q", got, want)
	}
	gone, err := inj.Reverted(t.Context(), recipe)
	if err != nil {
		t.Fatalf("Reverted: %v", err)
	}
	if gone {
		t.Error("reported reverted while the c4a0: netem qdisc is still shown")
	}
	// Now the show has no netem: reverted.
	run.showJSON = `[{"kind":"pfifo_fast","handle":"0:"}]`
	gone, err = inj.Reverted(t.Context(), recipe)
	if err != nil || !gone {
		t.Errorf("after del: gone=%v err=%v, want true/nil", gone, err)
	}
	if bad := run.nonTC(); bad != nil {
		t.Errorf("ran non-tc binaries: %v", bad)
	}
}

func TestNetem_GoneInterfaceCountsAsReverted(t *testing.T) {
	run := &fakeRunner{showErr: errors.New(`Cannot find device "vethsteel"`)}
	inj := netemInjector(t, NetemDelay, run, vethResolver{veth: "vethsteel"})
	gone, err := inj.Reverted(t.Context(), Recipe{"iface": "vethsteel", "handle": NetemHandle})
	if err != nil || !gone {
		t.Errorf("gone interface: gone=%v err=%v, want true/nil", gone, err)
	}
}

func TestNetem_VerifyErrorKeepsEntry(t *testing.T) {
	// An error that is not a missing device must not be read as "reverted".
	run := &fakeRunner{showErr: errors.New("RTNETLINK answers: Operation not permitted")}
	inj := netemInjector(t, NetemDelay, run, vethResolver{veth: "vethsteel"})
	if gone, err := inj.Reverted(t.Context(), Recipe{"iface": "vethsteel", "handle": NetemHandle}); gone || err == nil {
		t.Errorf("gone=%v err=%v, want false and an error", gone, err)
	}
}

func TestNetem_RecipeCarriesIface(t *testing.T) {
	run := &fakeRunner{}
	inj := netemInjector(t, NetemDelay, run, vethResolver{veth: "vethsteel"})
	r, err := inj.Plan(t.Context(), ResolvedTarget{Kind: KindLXD, ID: "x"}, Params{"delay_ms": 10.0})
	if err != nil {
		t.Fatal(err)
	}
	// Revert must work from the recipe alone (no resolver, after a crash).
	if r["iface"] != "vethsteel" || r["handle"] != NetemHandle {
		t.Fatalf("recipe = %v", r)
	}
}
