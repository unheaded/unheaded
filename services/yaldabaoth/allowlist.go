// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package yaldabaoth

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"math"
	"net/netip"
	"os"
	"regexp"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"unheaded/pkg/probenet"
)

// The blast-radius allowlist (S-REAL-STEEL D-17). Every experiment is
// checked against it before anything is injected, and the target again
// right before injection. Nothing in a request widens it: the file on the
// host (/etc/unheaded/chaos/allowlist.yaml, seed configs/chaos/allowlist.yaml)
// is the only source. Targets are resolved, never named: an LXD instance by
// name through Cuirass (managed, in the configured project, carrying the
// label), a lab namespace or lab interface by an anchored pattern, a
// systemd unit from an explicit list and only on the configured host. A
// host veth is never a target of its own; Phase 11 resolves it from an
// allowlisted instance.

// TargetKind is what an experiment targets.
type TargetKind string

const (
	KindLXD     TargetKind = "lxd"
	KindNetns   TargetKind = "netns"
	KindSystemd TargetKind = "systemd"
	KindPacket  TargetKind = "packet"
	// KindVeth is always refused: an interface named in a request could be
	// a physical NIC. Phase 11 resolves the veth from an LXD instance.
	KindVeth TargetKind = "veth"
)

// typeKinds lists, per experiment type, the kinds it may target.
var typeKinds = map[ExperimentType][]TargetKind{
	ExpTypeNetworkDelay:     {KindLXD, KindNetns, KindPacket},
	ExpTypeNetworkLoss:      {KindLXD, KindNetns, KindPacket},
	ExpTypeNetworkPartition: {KindLXD, KindNetns, KindPacket},
	ExpTypeLatencySpike:     {KindLXD, KindNetns, KindPacket},
	ExpTypeContainerKill:    {KindLXD},
	ExpTypeProcessKill:      {KindSystemd},
	ExpTypeServiceStop:      {KindSystemd},
	ExpTypePacketChaos:      {KindPacket},
}

// hardDenied names that no allowlist file can allow.
var (
	deniedInstances = map[string]bool{"pihole": true}
	deniedUnitRe    = regexp.MustCompile(`(\.target$)|(^ssh)|(^systemd-)|(^dbus)`)
	lxdNameRe       = regexp.MustCompile(`^[a-z]([a-z0-9-]{0,61}[a-z0-9])?$`)
	unitNameRe      = regexp.MustCompile(`^[A-Za-z0-9@._-]{1,128}\.(service|socket|timer)$|^[A-Za-z0-9@._-]{1,128}\.target$`)
	projectRe       = regexp.MustCompile(`^[a-z][a-z0-9-]{0,62}$`)
	labelKVRe       = regexp.MustCompile(`^([a-z][a-z0-9_-]{0,31})=([A-Za-z0-9._-]{1,63})$`)
	hostRe          = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?$`)
	paramNames      = map[string]bool{"delay_ms": true, "jitter_ms": true, "loss_percent": true,
		"flow_labels": true, "mode": true, "duplicate_percent": true}
)

// Duration reads "10m" in YAML.
type Duration time.Duration

// D returns the time.Duration.
func (d Duration) D() time.Duration { return time.Duration(d) }

// UnmarshalYAML parses a Go duration string.
func (d *Duration) UnmarshalYAML(n *yaml.Node) error {
	v, err := time.ParseDuration(n.Value)
	if err != nil {
		return fmt.Errorf("duration %q: %w", n.Value, err)
	}
	*d = Duration(v)
	return nil
}

// Allowlist is the parsed allowlist file.
type Allowlist struct {
	LXD struct {
		Project string `yaml:"project"`
		Label   string `yaml:"label"`
	} `yaml:"lxd"`
	Netns            string `yaml:"netns"`
	PacketInterfaces string `yaml:"packet_interfaces"`
	// ProbeSubnets: steady-state probes contact an instance only at an
	// address inside one of these CIDRs (the LXD bridge). Absent means
	// pkg/probenet.DefaultCIDR; an empty or over-broad list is an error.
	// S-CLEAN-SLATE M-039.
	ProbeSubnets *[]string `yaml:"probe_subnets"`
	Systemd      struct {
		Host  string   `yaml:"host"`
		Units []string `yaml:"units"`
	} `yaml:"systemd"`
	Limits struct {
		MaxConcurrent  int      `yaml:"max_concurrent"`
		MaxDuration    Duration `yaml:"max_duration"`
		MaxLossPercent float64  `yaml:"max_loss_percent"`
		MaxDelay       Duration `yaml:"max_delay"`
	} `yaml:"limits"`

	labelKey, labelValue string
	netnsRe, packetRe    *regexp.Regexp
	units                map[string]bool
	probeNets            []netip.Prefix
}

// ProbeAddress returns the address to probe for an instance-reported ip, or
// why it is never probed (outside probe_subnets, or not an address).
func (a *Allowlist) ProbeAddress(ip string) (string, error) {
	if a == nil {
		return "", errors.New("no allowlist loaded")
	}
	return probenet.Allowed(a.probeNets, ip)
}

// LoadAllowlist reads and checks the allowlist file.
func LoadAllowlist(path string) (*Allowlist, error) {
	b, err := os.ReadFile(path) // #nosec G304 -- the operator-configured allowlist file
	if err != nil {
		return nil, fmt.Errorf("allowlist: %w", err)
	}
	return ParseAllowlist(b)
}

// anchored compiles a pattern that must match a whole name and cannot
// match everything.
func anchored(field, pat string) (*regexp.Regexp, error) {
	if !strings.HasPrefix(pat, "^") || !strings.HasSuffix(pat, "$") {
		return nil, fmt.Errorf("allowlist: %s %q must be anchored (^...$)", field, pat)
	}
	re, err := regexp.Compile(pat)
	if err != nil {
		return nil, fmt.Errorf("allowlist: %s: %w", field, err)
	}
	for _, probe := range []string{"enp16s0", "eth0", "lo", "lxdbr0", "wg0", "docker0", "x"} {
		if re.MatchString(probe) {
			return nil, fmt.Errorf("allowlist: %s %q matches %q, a host interface", field, pat, probe)
		}
	}
	return re, nil
}

// ParseAllowlist checks an allowlist. Unknown fields are errors: a typo in
// a safety file must not silently mean "no rule".
func ParseAllowlist(b []byte) (*Allowlist, error) {
	var a Allowlist
	dec := yaml.NewDecoder(bytes.NewReader(b))
	dec.KnownFields(true)
	if err := dec.Decode(&a); err != nil {
		return nil, fmt.Errorf("allowlist: %w", err)
	}
	if !projectRe.MatchString(a.LXD.Project) || a.LXD.Project == "default" {
		return nil, fmt.Errorf("allowlist: lxd.project %q: name Cuirass's own project (not default, where pihole lives)", a.LXD.Project)
	}
	m := labelKVRe.FindStringSubmatch(a.LXD.Label)
	if m == nil {
		return nil, fmt.Errorf("allowlist: lxd.label %q: want key=value", a.LXD.Label)
	}
	a.labelKey, a.labelValue = m[1], m[2]
	var err error
	if a.netnsRe, err = anchored("netns", a.Netns); err != nil {
		return nil, err
	}
	if a.packetRe, err = anchored("packet_interfaces", a.PacketInterfaces); err != nil {
		return nil, err
	}
	if a.ProbeSubnets == nil {
		a.probeNets, err = probenet.ParseCSV("")
	} else {
		a.probeNets, err = probenet.Parse(*a.ProbeSubnets)
	}
	if err != nil {
		return nil, fmt.Errorf("allowlist: probe_subnets: %w", err)
	}
	if a.Systemd.Host != "" && !hostRe.MatchString(a.Systemd.Host) {
		return nil, fmt.Errorf("allowlist: systemd.host %q", a.Systemd.Host)
	}
	a.units = map[string]bool{}
	for _, u := range a.Systemd.Units {
		if !unitNameRe.MatchString(u) {
			return nil, fmt.Errorf("allowlist: systemd unit %q", u)
		}
		a.units[u] = true
	}
	l := a.Limits
	switch {
	case l.MaxConcurrent < 1 || l.MaxConcurrent > 10:
		return nil, errors.New("allowlist: limits.max_concurrent must be 1..10")
	case l.MaxDuration.D() <= 0 || l.MaxDuration.D() > 24*time.Hour:
		return nil, errors.New("allowlist: limits.max_duration must be in (0, 24h]")
	case l.MaxLossPercent < 0 || l.MaxLossPercent > 100 || math.IsNaN(l.MaxLossPercent):
		return nil, errors.New("allowlist: limits.max_loss_percent must be 0..100")
	case l.MaxDelay.D() < 0 || l.MaxDelay.D() > time.Minute:
		return nil, errors.New("allowlist: limits.max_delay must be in [0, 1m]")
	}
	return &a, nil
}

// RefusalReason is the bounded reason label of a refusal.
type RefusalReason string

const (
	RefuseInvalid     RefusalReason = "invalid"
	RefuseAllowlist   RefusalReason = "allowlist"
	RefuseLimits      RefusalReason = "limits"
	RefuseConcurrency RefusalReason = "concurrency"
	RefuseAbortFile   RefusalReason = "abort_file"
	RefuseSteadyState RefusalReason = "steady_state"
	RefuseNoJournal   RefusalReason = "no_journal"
	RefuseNoGuard     RefusalReason = "no_guard"
	RefuseNoInjector  RefusalReason = "no_injector"
	RefuseStopped     RefusalReason = "stopped"
)

// Refusal is why an experiment did not start.
type Refusal struct {
	Reason RefusalReason
	Detail string
}

func (r *Refusal) Error() string { return fmt.Sprintf("refused (%s): %s", r.Reason, r.Detail) }

func refuse(reason RefusalReason, format string, a ...any) *Refusal {
	return &Refusal{Reason: reason, Detail: fmt.Sprintf(format, a...)}
}

// InstanceInfo is what Cuirass reports for an LXD instance.
type InstanceInfo struct {
	Name     string
	Status   string
	IP       string
	Project  string
	Managed  bool
	Labels   map[string]string
	Health   string
	HostVeth string // host-side veth of the bridged NIC (Phase 11 netem)
}

// ErrInstanceNotFound means Cuirass reports no such instance.
var ErrInstanceNotFound = errors.New("no such instance")

// Resolver looks LXD instances up (Cuirass, resolver.go).
type Resolver interface {
	Instance(ctx context.Context, name string) (*InstanceInfo, error)
}

// ResolvedTarget is a target after the allowlist resolved it.
type ResolvedTarget struct {
	Kind TargetKind `json:"kind"`
	ID   string     `json:"id"`
	IP   string     `json:"ip,omitempty"` // lxd only, from Cuirass
}

// CheckEnv is what a check needs besides the experiment.
type CheckEnv struct {
	Resolver        Resolver
	Hostname        func() (string, error)
	DefaultDuration time.Duration
}

// Check decides whether exp may run. It returns the resolved target or a
// *Refusal (allowlist for the target, limits for duration and parameters).
func (a *Allowlist) Check(ctx context.Context, exp *Experiment, env CheckEnv) (ResolvedTarget, error) {
	if a == nil {
		return ResolvedTarget{}, refuse(RefuseAllowlist, "no allowlist loaded")
	}
	kinds, ok := typeKinds[exp.Type]
	if !ok {
		return ResolvedTarget{}, refuse(RefuseAllowlist, "experiment type %s has no allowlisted target kind", exp.Type)
	}
	fits := false
	for _, k := range kinds {
		fits = fits || k == exp.Target.Kind
	}
	if !fits {
		return ResolvedTarget{}, refuse(RefuseAllowlist, "%s cannot target kind %q", exp.Type, exp.Target.Kind)
	}
	rt, err := a.CheckTarget(ctx, exp.Target, env)
	if err != nil {
		return ResolvedTarget{}, err
	}
	if err := a.checkLimits(exp, env.DefaultDuration); err != nil {
		return ResolvedTarget{}, err
	}
	return rt, nil
}

// CheckTarget resolves and checks one target. Injectors call it again
// right before they act (the second, independent check).
func (a *Allowlist) CheckTarget(ctx context.Context, t Target, env CheckEnv) (ResolvedTarget, error) {
	if a == nil {
		return ResolvedTarget{}, refuse(RefuseAllowlist, "no allowlist loaded")
	}
	switch t.Kind {
	case KindLXD:
		if !lxdNameRe.MatchString(t.ID) {
			return ResolvedTarget{}, refuse(RefuseAllowlist, "%q is not an LXD instance name", t.ID)
		}
		if deniedInstances[t.ID] {
			return ResolvedTarget{}, refuse(RefuseAllowlist, "%s is never a chaos target", t.ID)
		}
		if env.Resolver == nil {
			return ResolvedTarget{}, refuse(RefuseAllowlist, "no Cuirass to resolve %s", t.ID)
		}
		in, err := env.Resolver.Instance(ctx, t.ID)
		if err != nil {
			return ResolvedTarget{}, refuse(RefuseAllowlist, "%s: %v", t.ID, err)
		}
		switch {
		case !in.Managed:
			return ResolvedTarget{}, refuse(RefuseAllowlist, "%s is not managed by Cuirass", t.ID)
		case in.Project != a.LXD.Project:
			return ResolvedTarget{}, refuse(RefuseAllowlist, "%s is in project %q, not %q", t.ID, in.Project, a.LXD.Project)
		case in.Labels[a.labelKey] != a.labelValue:
			return ResolvedTarget{}, refuse(RefuseAllowlist, "%s does not carry %s=%s", t.ID, a.labelKey, a.labelValue)
		}
		return ResolvedTarget{Kind: KindLXD, ID: t.ID, IP: in.IP}, nil
	case KindNetns:
		if !a.netnsRe.MatchString(t.ID) {
			return ResolvedTarget{}, refuse(RefuseAllowlist, "netns %q is not a lab namespace", t.ID)
		}
		return ResolvedTarget{Kind: KindNetns, ID: t.ID}, nil
	case KindPacket:
		if !a.packetRe.MatchString(t.ID) {
			return ResolvedTarget{}, refuse(RefuseAllowlist, "interface %q is not a lab interface", t.ID)
		}
		return ResolvedTarget{Kind: KindPacket, ID: t.ID}, nil
	case KindSystemd:
		if !unitNameRe.MatchString(t.ID) || deniedUnitRe.MatchString(t.ID) {
			return ResolvedTarget{}, refuse(RefuseAllowlist, "unit %q is never a chaos target", t.ID)
		}
		if !a.units[t.ID] {
			return ResolvedTarget{}, refuse(RefuseAllowlist, "unit %s is not listed", t.ID)
		}
		if env.Hostname == nil {
			return ResolvedTarget{}, refuse(RefuseAllowlist, "cannot tell which host this is")
		}
		host, err := env.Hostname()
		if err != nil || a.Systemd.Host == "" || host != a.Systemd.Host {
			return ResolvedTarget{}, refuse(RefuseAllowlist, "systemd faults run only on %q, this is %q", a.Systemd.Host, host)
		}
		return ResolvedTarget{Kind: KindSystemd, ID: t.ID}, nil
	case KindVeth:
		return ResolvedTarget{}, refuse(RefuseAllowlist, "an interface is never named in a request; target the LXD instance")
	default:
		return ResolvedTarget{}, refuse(RefuseAllowlist, "unknown target kind %q", t.Kind)
	}
}

func number(p Params, k string) (float64, bool, error) {
	v, ok := p[k]
	if !ok {
		return 0, false, nil
	}
	f, isNum := v.(float64)
	if !isNum {
		if i, isInt := v.(int); isInt {
			f, isNum = float64(i), true
		}
	}
	if !isNum || math.IsNaN(f) || math.IsInf(f, 0) || f < 0 {
		return 0, true, fmt.Errorf("parameter %s must be a non-negative number", k)
	}
	return f, true, nil
}

func (a *Allowlist) checkLimits(exp *Experiment, def time.Duration) error {
	d := def
	if exp.Schedule != nil && exp.Schedule.Duration != 0 {
		d = exp.Schedule.Duration
	}
	if d <= 0 || d > a.Limits.MaxDuration.D() {
		return refuse(RefuseLimits, "duration %v outside (0, %v]", d, a.Limits.MaxDuration.D())
	}
	for k := range exp.Parameters {
		if !paramNames[k] {
			return refuse(RefuseLimits, "unknown parameter %q", k)
		}
	}
	loss, _, err := number(exp.Parameters, "loss_percent")
	if err != nil {
		return refuse(RefuseLimits, "%v", err)
	}
	if loss > a.Limits.MaxLossPercent {
		return refuse(RefuseLimits, "loss %.1f%% over %.1f%%", loss, a.Limits.MaxLossPercent)
	}
	dup, _, err := number(exp.Parameters, "duplicate_percent")
	if err != nil {
		return refuse(RefuseLimits, "%v", err)
	}
	if dup > a.Limits.MaxLossPercent {
		return refuse(RefuseLimits, "duplicate %.1f%% over %.1f%% (the loss limit bounds it)", dup, a.Limits.MaxLossPercent)
	}
	delay, _, err := number(exp.Parameters, "delay_ms")
	if err != nil {
		return refuse(RefuseLimits, "%v", err)
	}
	jitter, _, err := number(exp.Parameters, "jitter_ms")
	if err != nil {
		return refuse(RefuseLimits, "%v", err)
	}
	if total := time.Duration((delay + jitter) * float64(time.Millisecond)); total > a.Limits.MaxDelay.D() {
		return refuse(RefuseLimits, "delay plus jitter %v over %v", total, a.Limits.MaxDelay.D())
	}
	return nil
}
