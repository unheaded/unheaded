// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package yaldabaoth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"

	"unheaded/pkg/logger"
)

// The netem injector (S-REAL-STEEL D-21): tc netem on the host-side veth of
// an allowlisted LXD instance (resolved from Cuirass, never named in a
// request) or on a lab interface, under our own qdisc handle c4a0:. Every
// command is an exact argv to tc, never a shell. Revert deletes only the
// c4a0: root qdisc and verifies it is gone with `tc -j qdisc show`; an
// already-deleted interface counts as reverted.

// NetemHandle is the root-qdisc handle every chaos netem carries, so revert
// touches only our qdisc and never a pre-existing one.
const NetemHandle = "c4a0:"

// NetemMode is the kind of impairment a netem injector applies.
type NetemMode string

const (
	NetemDelay     NetemMode = "delay"
	NetemLoss      NetemMode = "loss"
	NetemPartition NetemMode = "partition"
	NetemDuplicate NetemMode = "duplicate"
	NetemCorrupt   NetemMode = "corrupt"
)

var netemModes = map[NetemMode]bool{
	NetemDelay: true, NetemLoss: true, NetemPartition: true, NetemDuplicate: true, NetemCorrupt: true,
}

// NetemInjector shapes one interface's egress with tc netem.
type NetemInjector struct {
	mode     NetemMode
	runner   CommandRunner
	resolver Resolver // resolves an LXD instance's host veth; nil is fine for lab targets
	log      *logger.Logger
}

// NewNetemInjector builds an injector for one mode. runner defaults to the
// real exec runner.
func NewNetemInjector(mode NetemMode, runner CommandRunner, res Resolver, log *logger.Logger) (*NetemInjector, error) {
	if !netemModes[mode] {
		return nil, fmt.Errorf("unknown netem mode %q", mode)
	}
	if runner == nil {
		runner = ExecRunner{}
	}
	if log == nil {
		log = logger.New(io.Discard)
	}
	return &NetemInjector{mode: mode, runner: runner, resolver: res, log: log}, nil
}

// iface is the host interface this target's egress is shaped on. For an LXD
// instance it is the host-side veth from Cuirass; a non-bridged instance
// (no veth) is refused (D-21 override). Lab namespaces and interfaces map to
// their fixed steel<N>-h names.
func (n *NetemInjector) iface(ctx context.Context, t ResolvedTarget) (string, error) {
	switch t.Kind {
	case KindLXD:
		if n.resolver == nil {
			return "", errors.New("netem: no resolver to look up the host veth")
		}
		in, err := n.resolver.Instance(ctx, t.ID)
		if err != nil {
			return "", fmt.Errorf("netem: resolve %s: %w", t.ID, err)
		}
		if in.HostVeth == "" {
			return "", fmt.Errorf("netem: %s has no host veth (non-bridged NIC): not a chaos target", t.ID)
		}
		return in.HostVeth, nil
	case KindNetns:
		// The lab netns steel<N> is reached through its host-side veth steel<N>-h.
		return t.ID + "-h", nil
	case KindPacket:
		return t.ID, nil
	default:
		return "", fmt.Errorf("netem: cannot target kind %q", t.Kind)
	}
}

// netemArgs builds the netem clause (after `... netem`) for the mode.
func netemArgs(mode NetemMode, p Params) ([]string, error) {
	pct := func() (string, error) {
		v, ok, err := number(p, "loss_percent")
		if err != nil {
			return "", err
		}
		if !ok {
			return "", errors.New("netem: loss_percent is required")
		}
		return num(v) + "%", nil
	}
	switch mode {
	case NetemDelay:
		d, ok, err := number(p, "delay_ms")
		if err != nil {
			return nil, err
		}
		if !ok || d <= 0 {
			return nil, errors.New("netem: delay_ms is required for a delay fault")
		}
		args := []string{"delay", num(d) + "ms"}
		if j, ok, err := number(p, "jitter_ms"); err != nil {
			return nil, err
		} else if ok && j > 0 {
			args = append(args, num(j)+"ms")
		}
		return args, nil
	case NetemLoss:
		s, err := pct()
		if err != nil {
			return nil, err
		}
		return []string{"loss", s}, nil
	case NetemPartition:
		// A partition is total loss; it ignores any percentage parameter.
		return []string{"loss", "100%"}, nil
	case NetemDuplicate:
		s, err := pct()
		if err != nil {
			return nil, err
		}
		return []string{"duplicate", s}, nil
	case NetemCorrupt:
		s, err := pct()
		if err != nil {
			return nil, err
		}
		return []string{"corrupt", s}, nil
	default:
		return nil, fmt.Errorf("netem: unknown mode %q", mode)
	}
}

// num formats a non-negative float without a trailing ".0".
func num(f float64) string { return strconv.FormatFloat(f, 'g', -1, 64) }

// Plan resolves the interface and checks the parameters build a valid netem
// clause. The recipe carries the interface and handle so revert needs no
// resolver after a crash.
func (n *NetemInjector) Plan(ctx context.Context, t ResolvedTarget, p Params) (Recipe, error) {
	iface, err := n.iface(ctx, t)
	if err != nil {
		return nil, err
	}
	if _, err := netemArgs(n.mode, p); err != nil {
		return nil, err
	}
	return Recipe{"iface": iface, "handle": NetemHandle, "mode": string(n.mode)}, nil
}

// Inject replaces the interface's root qdisc with our netem qdisc.
func (n *NetemInjector) Inject(ctx context.Context, _ ResolvedTarget, p Params, r Recipe) error {
	iface := r["iface"]
	if iface == "" {
		return errors.New("netem: recipe has no interface")
	}
	clause, err := netemArgs(n.mode, p)
	if err != nil {
		return err
	}
	argv := append([]string{"qdisc", "replace", "dev", iface, "root", "handle", NetemHandle, "netem"}, clause...)
	if _, err := n.runner.Run(ctx, "tc", argv...); err != nil {
		return fmt.Errorf("netem: %w", err)
	}
	return nil
}

// Revert deletes our c4a0: root qdisc. It is best effort: an already-gone
// qdisc or interface errors here, and Reverted is the source of truth.
func (n *NetemInjector) Revert(ctx context.Context, r Recipe) error {
	iface := r["iface"]
	if iface == "" {
		return errors.New("netem: recipe has no interface")
	}
	_, _ = n.runner.Run(ctx, "tc", "qdisc", "del", "dev", iface, "root", "handle", NetemHandle)
	return nil
}

type tcQdisc struct {
	Kind   string `json:"kind"`
	Handle string `json:"handle"`
}

// Reverted is true when no c4a0: netem qdisc is on the interface. A missing
// interface (the instance was deleted) counts as reverted; any other error
// keeps the journal entry (D-19).
func (n *NetemInjector) Reverted(ctx context.Context, r Recipe) (bool, error) {
	iface := r["iface"]
	if iface == "" {
		return false, errors.New("netem: recipe has no interface")
	}
	out, err := n.runner.Run(ctx, "tc", "-j", "qdisc", "show", "dev", iface)
	if err != nil {
		if ifaceGone(err) {
			return true, nil
		}
		return false, fmt.Errorf("netem: verify %s: %w", iface, err)
	}
	var qdiscs []tcQdisc
	if len(out) > 0 {
		if err := json.Unmarshal(out, &qdiscs); err != nil {
			return false, fmt.Errorf("netem: parse qdisc show: %w", err)
		}
	}
	for _, q := range qdiscs {
		if q.Kind == "netem" && q.Handle == NetemHandle {
			return false, nil
		}
	}
	return true, nil
}

// ifaceGone reports whether err means the interface no longer exists.
func ifaceGone(err error) bool {
	s := strings.ToLower(err.Error())
	return strings.Contains(s, "cannot find device") || strings.Contains(s, "no such device") || strings.Contains(s, "does not exist")
}
