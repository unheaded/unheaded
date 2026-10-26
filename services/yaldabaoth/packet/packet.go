// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

// Package packet is Yaldabaoth's packet_chaos injector (S-REAL-STEEL Phase
// 13, D-24): it loads yaldabaoth-ebpf on one lab interface, enrolls flow
// labels in CHAOS_TARGETS, builds the prio/fw/netem qdisc tree that shapes
// only packets the program marked (skb->mark 0xC4A05), and takes all of it
// back in a fixed order through the revert journal (D-19).
//
// The interface must match ^steel[0-9]+-h$ and exist: the host side of a
// lab veth, never a physical NIC. The wire format is untouched; the mark
// is kernel metadata and a non-enrolled packet is byte-identical.
//
// Everything that touches the kernel goes through Backend, so the plan,
// the map encoding, the tc argv and the revert order are unit-tested with
// a recorder, and the real BPF load is proven live (plan Step 345).
package packet

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"math"
	"path"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"

	"unheaded/pkg/monadpkt"
	"unheaded/pkg/protocol/bpfschema"
	"unheaded/services/yaldabaoth"
)

// D-24 constants. Mark is what yaldabaoth-ebpf's CHAOS_SKB_MARK stamps;
// the handles are the qdisc tree userspace owns on the lab interface.
const (
	Mark        uint32 = 0xC4A05
	RootHandle         = "c4a0:"
	Band               = "c4a0:3"
	NetemHandle        = "c4a3:"

	// DefaultPinRoot holds one directory per experiment.
	DefaultPinRoot = "/sys/fs/bpf/unheaded/chaos"
	// DefaultObject is where a package install puts the program; YALD_OBJ
	// (ObjectPath) points at a build tree instead.
	DefaultObject = "/usr/lib/unheaded/ebpf/yaldabaoth-ebpf"

	// Names in the object and under a pin directory.
	ProgramName = "yaldabaoth_tc"
	MapTargets  = "CHAOS_TARGETS"
	MapConfig   = "CONFIG"
	MapStats    = "STATS"
	MapEvents   = "ANAMNESIS"
	PinProgram  = "prog"
	PinLink     = "link"

	// CfgHopID is CONFIG[0]: the hop_id stamped on CHAOS events.
	CfgHopID uint32 = 0

	// MaxFlowLabels bounds one experiment; the map holds 4096.
	MaxFlowLabels = 64
	// MaxDelayMs bounds netem delay and jitter (the allowlist's max_delay
	// is the real fence; this stops nonsense before it).
	MaxDelayMs = 60_000
)

// STATS keys, mirroring the program's STAT_* constants.
const (
	StatPacketsTotal  uint32 = 0
	StatPacketsIPv6   uint32 = 1
	StatPacketsHbH    uint32 = 2
	StatTargetsHit    uint32 = 3
	StatModeBitFlip   uint32 = 4
	StatModeDelay     uint32 = 5
	StatModeDuplicate uint32 = 6
	StatModeTruncate  uint32 = 7
	StatModeMarker    uint32 = 8
	StatEventsSent    uint32 = 9
	StatEventsDropped uint32 = 10
	StatCloneFailed   uint32 = 11
)

var statNames = map[uint32]string{
	StatPacketsTotal: "packets_total", StatPacketsIPv6: "packets_ipv6", StatPacketsHbH: "packets_hbh",
	StatTargetsHit: "targets_hit", StatModeBitFlip: "mode_bit_flip", StatModeDelay: "mode_delay",
	StatModeDuplicate: "mode_duplicate", StatModeTruncate: "mode_truncate", StatModeMarker: "mode_marker",
	StatEventsSent: "events_sent", StatEventsDropped: "events_dropped", StatCloneFailed: "clone_failed",
}

// StatName names a STATS key.
func StatName(k uint32) string {
	if n, ok := statNames[k]; ok {
		return n
	}
	return "stat_" + strconv.FormatUint(uint64(k), 10)
}

// ErrNotFound is what a Backend returns when the thing to remove is
// already gone (no pin, no link, no qdisc): a revert step treats it as done.
var ErrNotFound = errors.New("packet: not found")

// ifaceRe is the lab interface shape: the host side of a steel<N> veth.
var ifaceRe = regexp.MustCompile(`^steel[0-9]+-h$`)

// nonceRe is a pin directory's last element.
var nonceRe = regexp.MustCompile(`^[0-9a-f]{16}$`)

// Backend is the kernel. Every method names the pin directory it works in;
// none runs a shell: TC receives argv and executes the tc binary directly.
type Backend interface {
	Present(iface string) bool
	Load(objPath, pinDir string) error
	Attach(iface, pinDir string) error
	WriteTargets(pinDir string, targets map[uint32]uint64) error
	WriteConfig(pinDir string, key uint32, val uint64) error
	ReadStats(pinDir string) (map[uint32]uint64, error)
	Detach(pinDir string) error
	DeleteTargets(pinDir string, keys []uint32) error
	Unpin(pinDir string) error
	Pinned(pinDir string) bool
	Linked(pinDir string) bool
	TC(ctx context.Context, argv []string) error
	RootQdisc(ctx context.Context, iface string) (bool, error)
}

// EncodeTarget packs a CHAOS_TARGETS value the way the program reads it:
// mode in the low 32 bits, param in the high 32 (bpfschema.ChaosTargetValue).
func EncodeTarget(mode, param uint32) uint64 {
	return uint64(mode) | uint64(param)<<32
}

// Key is the CHAOS_TARGETS key for a flow label: the label itself, 20
// bits. 0 has no correlation key and a wider value would alias another
// flow once the program masks it, so both are refused.
func Key(flowLabel uint32) (uint32, error) {
	if flowLabel == 0 || flowLabel > monadpkt.FlowMask {
		return 0, fmt.Errorf("flow label %#x: want 1..0xFFFFF", flowLabel)
	}
	return flowLabel, nil
}

var modes = map[string]uint32{
	"": bpfschema.ChaosMarker, "marker": bpfschema.ChaosMarker, "bitflip": bpfschema.ChaosBitFlip,
	"delay": bpfschema.ChaosDelay, "duplicate": bpfschema.ChaosDuplicate, "truncate": bpfschema.ChaosTruncate,
}

// ModeCode maps a mode name to the program's ChaosMode; "" is marker.
func ModeCode(name string) (uint32, error) {
	if c, ok := modes[name]; ok {
		return c, nil
	}
	return 0, fmt.Errorf("mode %q: want marker, bitflip, delay, duplicate or truncate", name)
}

func modeName(code uint32) string {
	for n, c := range modes {
		if c == code && n != "" {
			return n
		}
	}
	return strconv.FormatUint(uint64(code), 10)
}

// CheckIface accepts only a present lab interface.
func CheckIface(name string, present func(string) bool) error {
	if !ifaceRe.MatchString(name) {
		return fmt.Errorf("interface %q is not a lab interface (steel<N>-h)", name)
	}
	if present == nil || !present(name) {
		return fmt.Errorf("interface %s is not present", name)
	}
	return nil
}

// Spec is one packet_chaos experiment, parsed from its parameters.
type Spec struct {
	Iface      string
	FlowLabels []uint32 // sorted, unique
	Mode       uint32   // ChaosMode
	Param      uint32   // delay_us for delay, else 0

	lossPct, delayMs, jitterMs, dupPct     float64
	hasLoss, hasDelay, hasJitter, hasDupli bool
}

// Netem is the netem argument list the spec asks for (delay, loss,
// duplicate, in that order), or nil when nothing shapes.
func (s Spec) Netem() []string {
	var out []string
	if s.hasDelay {
		out = append(out, "delay", num(s.delayMs)+"ms")
		if s.hasJitter {
			out = append(out, num(s.jitterMs)+"ms")
		}
	}
	if s.hasLoss {
		out = append(out, "loss", num(s.lossPct)+"%")
	}
	if s.hasDupli {
		out = append(out, "duplicate", num(s.dupPct)+"%")
	}
	return out
}

func num(f float64) string { return strconv.FormatFloat(f, 'f', -1, 64) }

var paramNames = map[string]bool{"flow_labels": true, "mode": true, "loss_percent": true, "delay_ms": true, "jitter_ms": true, "duplicate_percent": true}

// ParseSpec reads an experiment's parameters. flow_labels is a list of
// numbers or "0x.." strings (what JSON gives), mode a name, and the netem
// knobs are the allowlist's loss_percent / delay_ms / jitter_ms plus
// duplicate_percent. Unknown parameters are refused.
func ParseSpec(iface string, p yaldabaoth.Params) (Spec, error) {
	s := Spec{Iface: iface}
	if !ifaceRe.MatchString(iface) {
		return s, fmt.Errorf("interface %q is not a lab interface (steel<N>-h)", iface)
	}
	for k := range p {
		if !paramNames[k] {
			return s, fmt.Errorf("unknown parameter %q", k)
		}
	}
	labels, err := flowLabels(p["flow_labels"])
	if err != nil {
		return s, err
	}
	s.FlowLabels = labels
	mode, _ := p["mode"].(string)
	if p["mode"] != nil && mode == "" {
		return s, errors.New("parameter mode must be a string")
	}
	if s.Mode, err = ModeCode(mode); err != nil {
		return s, err
	}
	pct := func(k string) (float64, bool, error) {
		v, ok, err := number(p, k)
		if err == nil && v > 100 {
			err = fmt.Errorf("parameter %s must be at most 100", k)
		}
		return v, ok, err
	}
	ms := func(k string) (float64, bool, error) {
		v, ok, err := number(p, k)
		if err == nil && v > MaxDelayMs {
			err = fmt.Errorf("parameter %s must be at most %d", k, MaxDelayMs)
		}
		return v, ok, err
	}
	if s.lossPct, s.hasLoss, err = pct("loss_percent"); err != nil {
		return s, err
	}
	if s.dupPct, s.hasDupli, err = pct("duplicate_percent"); err != nil {
		return s, err
	}
	if s.delayMs, s.hasDelay, err = ms("delay_ms"); err != nil {
		return s, err
	}
	if s.jitterMs, s.hasJitter, err = ms("jitter_ms"); err != nil {
		return s, err
	}
	if s.hasJitter && !s.hasDelay {
		return s, errors.New("jitter_ms needs delay_ms")
	}
	if s.Mode == bpfschema.ChaosDelay && s.hasDelay {
		s.Param = uint32(math.Round(s.delayMs * 1000)) // #nosec G115 -- bounded by MaxDelayMs
	}
	return s, nil
}

func number(p yaldabaoth.Params, k string) (float64, bool, error) {
	v, ok := p[k]
	if !ok {
		return 0, false, nil
	}
	var f float64
	switch n := v.(type) {
	case float64:
		f = n
	case int:
		f = float64(n)
	default:
		return 0, true, fmt.Errorf("parameter %s must be a number", k)
	}
	if math.IsNaN(f) || math.IsInf(f, 0) || f < 0 {
		return 0, true, fmt.Errorf("parameter %s must be a non-negative number", k)
	}
	return f, true, nil
}

func flowLabels(v any) ([]uint32, error) {
	var items []any
	switch l := v.(type) {
	case nil:
		return nil, errors.New("parameter flow_labels is required")
	case []any:
		items = l
	case []string:
		for _, s := range l {
			items = append(items, s)
		}
	case []uint32:
		for _, u := range l {
			items = append(items, float64(u))
		}
	default:
		return nil, errors.New("parameter flow_labels must be a list")
	}
	if len(items) == 0 {
		return nil, errors.New("parameter flow_labels is empty")
	}
	if len(items) > MaxFlowLabels {
		return nil, fmt.Errorf("at most %d flow labels", MaxFlowLabels)
	}
	seen := map[uint32]bool{}
	out := make([]uint32, 0, len(items))
	for _, it := range items {
		var fl uint32
		switch x := it.(type) {
		case string:
			v, err := monadpkt.ParseFlowLabel(x)
			if err != nil {
				return nil, err
			}
			fl = v
		case float64:
			if x != math.Trunc(x) || x < 0 || x > monadpkt.FlowMask {
				return nil, fmt.Errorf("flow label %v: want an integer 1..0xFFFFF", x)
			}
			fl = uint32(x)
		case int:
			if x < 0 || x > monadpkt.FlowMask {
				return nil, fmt.Errorf("flow label %d: want 1..0xFFFFF", x)
			}
			fl = uint32(x)
		default:
			return nil, fmt.Errorf("flow label %v: want a number or a 0x string", it)
		}
		k, err := Key(fl)
		if err != nil {
			return nil, err
		}
		if seen[k] {
			return nil, fmt.Errorf("flow label %#x listed twice", k)
		}
		seen[k] = true
		out = append(out, k)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out, nil
}

// TCArgv is the exact D-24 qdisc tree for iface: prio root c4a0:, an fw
// filter on the mark into band 3, netem on that band. nil when netem is
// empty (nothing shapes; the program still marks and flags).
func TCArgv(iface string, netem []string) [][]string {
	if len(netem) == 0 {
		return nil
	}
	third := append([]string{"tc", "qdisc", "add", "dev", iface, "parent", Band, "handle", NetemHandle, "netem"}, netem...)
	return [][]string{
		{"tc", "qdisc", "replace", "dev", iface, "root", "handle", RootHandle, "prio", "bands", "3"},
		{"tc", "filter", "add", "dev", iface, "parent", RootHandle, "protocol", "ipv6", "handle", fmt.Sprintf("0x%X", Mark), "fw", "flowid", Band},
		third,
	}
}

// TCRevertArgv deletes the root qdisc; its filter and the netem child go with it.
func TCRevertArgv(iface string) []string {
	return []string{"tc", "qdisc", "del", "dev", iface, "root", "handle", RootHandle}
}

// Options configure an Injector.
type Options struct {
	Object  string // yaldabaoth-ebpf; "" = DefaultObject
	PinRoot string // "" = DefaultPinRoot
	HopID   uint8  // CONFIG[0]
}

// ObjectPath is YALD_OBJ or DefaultObject.
func ObjectPath(getenv func(string) string) string {
	if v := getenv("YALD_OBJ"); v != "" {
		return v
	}
	return DefaultObject
}

// Injector implements yaldabaoth.Injector for packet_chaos.
type Injector struct {
	b       Backend
	obj     string
	pinRoot string
	hop     uint8

	mu   sync.Mutex
	last map[uint32]uint64
}

// New makes an injector over b.
func New(b Backend, o Options) *Injector {
	if o.Object == "" {
		o.Object = DefaultObject
	}
	if o.PinRoot == "" {
		o.PinRoot = DefaultPinRoot
	}
	return &Injector{b: b, obj: o.Object, pinRoot: path.Clean(o.PinRoot), hop: o.HopID}
}

// LastStats is the STATS map as read at the last revert.
func (in *Injector) LastStats() map[uint32]uint64 {
	in.mu.Lock()
	defer in.mu.Unlock()
	out := make(map[uint32]uint64, len(in.last))
	for k, v := range in.last {
		out[k] = v
	}
	return out
}

func (in *Injector) check(t yaldabaoth.ResolvedTarget, p yaldabaoth.Params) (Spec, error) {
	if t.Kind != yaldabaoth.KindPacket {
		return Spec{}, fmt.Errorf("packet_chaos targets an interface, not %q", t.Kind)
	}
	if err := CheckIface(t.ID, in.b.Present); err != nil {
		return Spec{}, err
	}
	return ParseSpec(t.ID, p)
}

// Plan validates the target and parameters and writes the revert recipe:
// interface, pin directory (a fresh nonce under PinRoot), flow labels,
// mode and netem. It touches nothing.
func (in *Injector) Plan(_ context.Context, t yaldabaoth.ResolvedTarget, p yaldabaoth.Params) (yaldabaoth.Recipe, error) {
	s, err := in.check(t, p)
	if err != nil {
		return nil, err
	}
	var nonce [8]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		return nil, fmt.Errorf("nonce: %w", err)
	}
	return yaldabaoth.Recipe{
		"iface":       s.Iface,
		"pin_dir":     path.Join(in.pinRoot, hex.EncodeToString(nonce[:])),
		"flow_labels": labelList(s.FlowLabels),
		"mode":        modeName(s.Mode),
		"netem":       strings.Join(s.Netem(), " "),
		"qdisc":       RootHandle,
	}, nil
}

func labelList(ls []uint32) string {
	parts := make([]string, len(ls))
	for i, l := range ls {
		parts[i] = fmt.Sprintf("0x%x", l)
	}
	return strings.Join(parts, ",")
}

type recipe struct {
	iface, pinDir string
	labels        []uint32
	netem         []string
}

// recipeOf reads a journaled recipe back and refuses one that would act
// outside this injector's fence (another interface shape, a pin directory
// not directly under PinRoot).
func (in *Injector) recipeOf(r yaldabaoth.Recipe) (recipe, error) {
	var rc recipe
	rc.iface = r["iface"]
	if !ifaceRe.MatchString(rc.iface) {
		return rc, fmt.Errorf("recipe interface %q is not a lab interface", rc.iface)
	}
	rc.pinDir = r["pin_dir"]
	dir, nonce := path.Split(rc.pinDir)
	if path.Clean(dir) != in.pinRoot || !nonceRe.MatchString(nonce) || path.Clean(rc.pinDir) != rc.pinDir {
		return rc, fmt.Errorf("recipe pin_dir %q is not under %s", rc.pinDir, in.pinRoot)
	}
	if r["flow_labels"] == "" {
		return rc, errors.New("recipe has no flow labels")
	}
	for _, s := range strings.Split(r["flow_labels"], ",") {
		fl, err := monadpkt.ParseFlowLabel(s)
		if err != nil {
			return rc, fmt.Errorf("recipe: %w", err)
		}
		rc.labels = append(rc.labels, fl)
	}
	if r["netem"] != "" {
		rc.netem = strings.Fields(r["netem"])
	}
	return rc, nil
}

// Inject loads the program, writes CONFIG and CHAOS_TARGETS, builds the
// qdisc tree and only then attaches the classifier at TCX egress, so the
// first marked packet already finds its netem band. The interface is
// checked again right before acting.
func (in *Injector) Inject(ctx context.Context, t yaldabaoth.ResolvedTarget, p yaldabaoth.Params, r yaldabaoth.Recipe) error {
	rc, err := in.recipeOf(r)
	if err != nil {
		return err
	}
	s, err := in.check(t, p)
	if err != nil {
		return err
	}
	if rc.iface != s.Iface {
		return fmt.Errorf("recipe is for %s, target is %s", rc.iface, s.Iface)
	}
	if labelList(rc.labels) != labelList(s.FlowLabels) {
		return errors.New("recipe flow labels differ from the parameters")
	}
	if err := in.b.Load(in.obj, rc.pinDir); err != nil {
		return fmt.Errorf("load %s: %w", in.obj, err)
	}
	if err := in.b.WriteConfig(rc.pinDir, CfgHopID, uint64(in.hop)); err != nil {
		return fmt.Errorf("CONFIG: %w", err)
	}
	targets := make(map[uint32]uint64, len(s.FlowLabels))
	for _, fl := range s.FlowLabels {
		targets[fl] = EncodeTarget(s.Mode, s.Param)
	}
	if err := in.b.WriteTargets(rc.pinDir, targets); err != nil {
		return fmt.Errorf("CHAOS_TARGETS: %w", err)
	}
	for _, argv := range TCArgv(s.Iface, s.Netem()) {
		if err := in.b.TC(ctx, argv); err != nil {
			return fmt.Errorf("%s: %w", strings.Join(argv, " "), err)
		}
	}
	if err := in.b.Attach(s.Iface, rc.pinDir); err != nil {
		return fmt.Errorf("attach tcx egress %s: %w", s.Iface, err)
	}
	return nil
}

// Revert takes the fault back in D-24 order: detach the link, delete the
// map entries, delete the root qdisc, unpin the maps. STATS is read first
// for the record. Every step runs even when an earlier one fails; the
// errors are joined. A step whose object is already gone is done.
func (in *Injector) Revert(ctx context.Context, r yaldabaoth.Recipe) error {
	rc, err := in.recipeOf(r)
	if err != nil {
		return err
	}
	var errs []error
	step := func(name string, err error) {
		if err != nil && !errors.Is(err, ErrNotFound) {
			errs = append(errs, fmt.Errorf("%s: %w", name, err))
		}
	}
	if st, err := in.b.ReadStats(rc.pinDir); err == nil {
		in.mu.Lock()
		in.last = st
		in.mu.Unlock()
	}
	step("detach link", in.b.Detach(rc.pinDir))
	step("delete CHAOS_TARGETS", in.b.DeleteTargets(rc.pinDir, rc.labels))
	step("tc qdisc del", in.b.TC(ctx, TCRevertArgv(rc.iface)))
	step("unpin", in.b.Unpin(rc.pinDir))
	return errors.Join(errs...)
}

// Reverted is true when no link, no pins and no c4a0: root qdisc remain.
// An interface that no longer exists has no qdisc.
func (in *Injector) Reverted(ctx context.Context, r yaldabaoth.Recipe) (bool, error) {
	rc, err := in.recipeOf(r)
	if err != nil {
		return false, err
	}
	if in.b.Linked(rc.pinDir) || in.b.Pinned(rc.pinDir) {
		return false, nil
	}
	if !in.b.Present(rc.iface) {
		return true, nil
	}
	has, err := in.b.RootQdisc(ctx, rc.iface)
	if err != nil {
		return false, err
	}
	return !has, nil
}

// DryRun writes the plan Inject would carry out and the revert that
// follows, acting on nothing. It is what the lead runs before Step 345.
func (in *Injector) DryRun(ctx context.Context, w io.Writer, t yaldabaoth.ResolvedTarget, p yaldabaoth.Params) error {
	r, err := in.Plan(ctx, t, p)
	if err != nil {
		return err
	}
	s, err := ParseSpec(t.ID, p)
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "packet_chaos on %s: flow labels %s, mode %s\n", s.Iface, r["flow_labels"], r["mode"])
	fmt.Fprintf(w, "  load %s -> pin %s\n", in.obj, r["pin_dir"])
	fmt.Fprintf(w, "  %s[%d] = %d\n", MapConfig, CfgHopID, in.hop)
	for _, fl := range s.FlowLabels {
		fmt.Fprintf(w, "  %s[0x%x] = 0x%016x\n", MapTargets, fl, EncodeTarget(s.Mode, s.Param))
	}
	argvs := TCArgv(s.Iface, s.Netem())
	if argvs == nil {
		fmt.Fprintln(w, "  no netem parameters: the program marks and flags, nothing shapes")
	}
	for _, a := range argvs {
		fmt.Fprintf(w, "  %s\n", strings.Join(a, " "))
	}
	fmt.Fprintf(w, "  attach %s tcx egress %s (skb->mark 0x%X on enrolled packets)\n", ProgramName, s.Iface, Mark)
	revert := []string{"detach link", fmt.Sprintf("delete %s[%s]", MapTargets, strings.ReplaceAll(r["flow_labels"], ",", ","+MapTargets+"["))}
	if argvs != nil {
		revert = append(revert, strings.Join(TCRevertArgv(s.Iface), " "))
	}
	revert = append(revert, "unpin")
	fmt.Fprintf(w, "  revert: %s\n", strings.Join(revert, ", "))
	return nil
}
