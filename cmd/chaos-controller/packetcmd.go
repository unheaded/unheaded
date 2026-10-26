// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"sort"
	"strings"
	"syscall"
	"time"

	"unheaded/pkg/statefile"
	"unheaded/services/yaldabaoth"
	"unheaded/services/yaldabaoth/packet"
)

// `chaos-controller packet-chaos` runs one packet_chaos experiment from
// the command line for the Phase 13 lab (plan Steps 344-349), where no
// Victoria SLO guard or HTTP auth is in reach of a netns on west. It keeps
// the parts of the envelope that matter there: the lab-interface fence
// (steel<N>-h, present), the revert recipe journaled before the fault
// lands (packet-chaos.json in --state-dir; leftovers are reverted at the
// next start), the ABORT file, SIGINT/SIGTERM, and a hard duration cap.
//
//	chaos-controller packet-chaos --iface steel0-h --flow-label 0x5EE1 \
//	    --mode marker --loss 50 --duration 5m [--dry-run]
//
// --dry-run prints the plan (object, pin dir, map writes, the exact tc
// argv, the attach and the revert order) and touches nothing.

const (
	packetJournal   = "packet-chaos.json"
	maxPacketHold   = 10 * time.Minute
	abortPollPeriod = time.Second
)

type labelsFlag []string

func (l *labelsFlag) String() string     { return strings.Join(*l, ",") }
func (l *labelsFlag) Set(s string) error { *l = append(*l, s); return nil }

type packetConfig struct {
	iface     string
	labels    labelsFlag
	mode      string
	loss      float64
	delayMs   float64
	jitterMs  float64
	dupPct    float64
	duration  time.Duration
	dryRun    bool
	stateDir  string
	abortFile string
	obj       string
	hopID     uint
}

func parsePacketFlags(args []string, getenv func(string) string) (packetConfig, error) {
	c := packetConfig{}
	fs := flag.NewFlagSet("packet-chaos", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.StringVar(&c.iface, "iface", "", "lab interface, the host side of a steel<N> veth (steel0-h)")
	fs.Var(&c.labels, "flow-label", "IPv6 flow label to enroll (0x5EE1 or decimal); repeat for more")
	fs.StringVar(&c.mode, "mode", "marker", "marker, bitflip, delay, duplicate or truncate")
	fs.Float64Var(&c.loss, "loss", 0, "netem loss percent on the marked band (0 = none)")
	fs.Float64Var(&c.delayMs, "delay-ms", 0, "netem delay in ms on the marked band (0 = none)")
	fs.Float64Var(&c.jitterMs, "jitter-ms", 0, "netem jitter in ms (needs --delay-ms)")
	fs.Float64Var(&c.dupPct, "duplicate", 0, "netem duplicate percent on the marked band (0 = none)")
	fs.DurationVar(&c.duration, "duration", 5*time.Minute, "how long the fault stays (at most 10m)")
	fs.BoolVar(&c.dryRun, "dry-run", false, "print the plan, touch nothing")
	fs.StringVar(&c.stateDir, "state-dir", envOr(getenv, "CHAOS_STATE_DIR", "/var/lib/unheaded-chaos"), "revert journal directory")
	fs.StringVar(&c.abortFile, "abort-file", envOr(getenv, "CHAOS_ABORT_FILE", "/run/unheaded/chaos/ABORT"), "abort file: touch it to revert now")
	fs.StringVar(&c.obj, "obj", packet.ObjectPath(getenv), "yaldabaoth-ebpf object (YALD_OBJ)")
	fs.UintVar(&c.hopID, "hop-id", 0, "hop_id stamped on CHAOS events (CONFIG[0])")
	if err := fs.Parse(args); err != nil {
		return c, err
	}
	if fs.NArg() != 0 {
		return c, fmt.Errorf("unexpected argument %q", fs.Arg(0))
	}
	switch {
	case c.iface == "":
		return c, errors.New("set --iface")
	case len(c.labels) == 0:
		return c, errors.New("set --flow-label at least once")
	case c.duration <= 0 || c.duration > maxPacketHold:
		return c, fmt.Errorf("--duration %v: want (0, %v]", c.duration, maxPacketHold)
	case c.hopID > 255:
		return c, errors.New("--hop-id: want 0..255")
	case c.stateDir == "" && !c.dryRun:
		return c, errors.New("set --state-dir: a fault is journaled before it lands")
	case c.obj == "":
		return c, errors.New("set --obj or YALD_OBJ")
	}
	return c, nil
}

// params is what the same request looks like through the HTTP API.
func (c packetConfig) params() yaldabaoth.Params {
	labels := make([]any, len(c.labels))
	for i, l := range c.labels {
		labels[i] = l
	}
	p := yaldabaoth.Params{"flow_labels": labels, "mode": c.mode}
	if c.loss > 0 {
		p["loss_percent"] = c.loss
	}
	if c.delayMs > 0 {
		p["delay_ms"] = c.delayMs
	}
	if c.jitterMs > 0 {
		p["jitter_ms"] = c.jitterMs
	}
	if c.dupPct > 0 {
		p["duplicate_percent"] = c.dupPct
	}
	return p
}

func (c packetConfig) target() yaldabaoth.ResolvedTarget {
	return yaldabaoth.ResolvedTarget{Kind: yaldabaoth.KindPacket, ID: c.iface}
}

type packetJournalFile struct {
	Version int                 `json:"version"`
	Entries []packetJournalItem `json:"entries"`
}

type packetJournalItem struct {
	Created time.Time         `json:"created"`
	Expires time.Time         `json:"expires"`
	Recipe  yaldabaoth.Recipe `json:"recipe"`
}

// runPacketChaos is the subcommand. It returns the exit code: 0 when the
// fault was reverted and verified gone (or the dry run printed), 1 when
// something is still in place (the journal keeps it; run again to retry),
// 2 for a usage error.
func runPacketChaos(ctx context.Context, args []string, out io.Writer, be packet.Backend, getenv func(string) string) int {
	c, err := parsePacketFlags(args, getenv)
	if err != nil {
		fmt.Fprintf(out, "packet-chaos: %v\n", err)
		return 2
	}
	inj := packet.New(be, packet.Options{Object: c.obj, HopID: uint8(c.hopID)}) // #nosec G115 -- checked <= 255
	if c.dryRun {
		if err := inj.DryRun(ctx, out, c.target(), c.params()); err != nil {
			fmt.Fprintf(out, "packet-chaos: %v\n", err)
			return 2
		}
		fmt.Fprintln(out, "dry run: nothing loaded, nothing attached")
		return 0
	}

	jf := statefile.In(c.stateDir, packetJournal)
	var j packetJournalFile
	if _, err := jf.Load(&j); err != nil {
		fmt.Fprintf(out, "packet-chaos: journal: %v\n", err)
		return 1
	}
	// Faults a previous run left behind come back first.
	if n := revertPacketJournal(ctx, out, inj, jf, &j); n > 0 {
		fmt.Fprintf(out, "packet-chaos: %d earlier fault(s) still in place, see %s\n", n, c.stateDir)
		return 1
	}

	recipe, err := inj.Plan(ctx, c.target(), c.params())
	if err != nil {
		fmt.Fprintf(out, "packet-chaos: refused: %v\n", err)
		return 2
	}
	now := time.Now().UTC()
	j.Version = 1
	j.Entries = append(j.Entries, packetJournalItem{Created: now, Expires: now.Add(c.duration + 30*time.Second), Recipe: recipe})
	if err := jf.Save(j); err != nil {
		fmt.Fprintf(out, "packet-chaos: journal write failed, nothing injected: %v\n", err)
		return 1
	}
	fmt.Fprintf(out, "journaled %s (pin %s)\n", recipe["flow_labels"], recipe["pin_dir"])
	if err := inj.Inject(ctx, c.target(), c.params(), recipe); err != nil {
		fmt.Fprintf(out, "packet-chaos: inject: %v; reverting\n", err)
		return finishPacket(ctx, out, inj, jf, &j, recipe, "inject failed")
	}
	fmt.Fprintf(out, "injected on %s for %v: flow labels %s, mode %s, netem %q\n", c.iface, c.duration, recipe["flow_labels"], recipe["mode"], recipe["netem"])
	fmt.Fprintf(out, "abort: touch %s, or SIGINT/SIGTERM\n", c.abortFile)

	timer := time.NewTimer(c.duration)
	defer timer.Stop()
	tick := time.NewTicker(abortPollPeriod)
	defer tick.Stop()
	why := "duration reached"
loop:
	for {
		select {
		case <-ctx.Done():
			why = "signal"
			break loop
		case <-timer.C:
			break loop
		case <-tick.C:
			if _, err := os.Stat(c.abortFile); err == nil {
				why = "abort file " + c.abortFile
				break loop
			}
		}
	}
	// The revert runs on its own context: a signal must not cut it short.
	return finishPacket(context.WithoutCancel(ctx), out, inj, jf, &j, recipe, why)
}

func finishPacket(ctx context.Context, out io.Writer, inj *packet.Injector, jf *statefile.File, j *packetJournalFile, recipe yaldabaoth.Recipe, why string) int {
	rctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	err := inj.Revert(rctx, recipe)
	gone, verr := inj.Reverted(rctx, recipe)
	printStats(out, inj.LastStats())
	if err != nil {
		fmt.Fprintf(out, "packet-chaos: revert (%s): %v\n", why, err)
	}
	if verr != nil {
		fmt.Fprintf(out, "packet-chaos: verify: %v\n", verr)
	}
	if !gone {
		fmt.Fprintln(out, "packet-chaos: the fault is still in place; its journal entry stays (run again, or Appendix A1)")
		return 1
	}
	kept := j.Entries[:0]
	for _, e := range j.Entries {
		if e.Recipe["pin_dir"] != recipe["pin_dir"] {
			kept = append(kept, e)
		}
	}
	j.Entries = kept
	if err := jf.Save(*j); err != nil {
		fmt.Fprintf(out, "packet-chaos: reverted, but the journal could not be updated: %v\n", err)
		return 1
	}
	fmt.Fprintf(out, "reverted and verified gone (%s)\n", why)
	return 0
}

// revertPacketJournal reverts every journaled recipe and returns how many remain.
func revertPacketJournal(ctx context.Context, out io.Writer, inj *packet.Injector, jf *statefile.File, j *packetJournalFile) int {
	if len(j.Entries) == 0 {
		return 0
	}
	fmt.Fprintf(out, "reverting %d journaled fault(s) first\n", len(j.Entries))
	var kept []packetJournalItem
	for _, e := range j.Entries {
		rctx, cancel := context.WithTimeout(ctx, 30*time.Second)
		err := inj.Revert(rctx, e.Recipe)
		gone, verr := inj.Reverted(rctx, e.Recipe)
		cancel()
		if err != nil || verr != nil || !gone {
			fmt.Fprintf(out, "  %s on %s: still in place (%v %v)\n", e.Recipe["flow_labels"], e.Recipe["iface"], err, verr)
			kept = append(kept, e)
			continue
		}
		fmt.Fprintf(out, "  %s on %s: reverted\n", e.Recipe["flow_labels"], e.Recipe["iface"])
	}
	j.Entries = kept
	if err := jf.Save(*j); err != nil {
		fmt.Fprintf(out, "packet-chaos: journal could not be updated: %v\n", err)
		return len(kept) + 1
	}
	return len(kept)
}

func printStats(out io.Writer, st map[uint32]uint64) {
	if len(st) == 0 {
		return
	}
	keys := make([]int, 0, len(st))
	for k := range st {
		keys = append(keys, int(k))
	}
	sort.Ints(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, fmt.Sprintf("%s=%d", packet.StatName(uint32(k)), st[uint32(k)])) // #nosec G115 -- keys came from uint32
	}
	fmt.Fprintf(out, "STATS: %s\n", strings.Join(parts, " "))
}

// packetChaosMain is the process entry for the subcommand.
func packetChaosMain(args []string) int {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	return runPacketChaos(ctx, args, os.Stdout, newPacketBackend(), os.Getenv)
}
