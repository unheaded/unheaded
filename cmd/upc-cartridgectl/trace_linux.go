// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

//go:build linux

package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"unheaded/pkg/cartridge"
)

// cmdRecord (S5 Phase 16) turns tracing on (one run in N), drains
// CART_TRACE for a while, turns it off and writes a UCTR trace file with
// every slot's .cart sha256 from slots.json. A trace holds header fields and
// cartridge state, never payload; it is as sensitive as flow metadata (0600).
func cmdRecord(g *globals, args []string) error {
	fs := flag.NewFlagSet("record", flag.ExitOnError)
	out := fs.String("out", "", "trace file to write (0600)")
	every := fs.Uint64("every", 1, "trace one run in N (per CPU)")
	dur := fs.Duration("duration", 5*time.Second, "how long to record")
	_ = fs.Parse(args)
	if *out == "" || *every == 0 {
		return errors.New("usage: record --out FILE [--every N] [--duration D]")
	}
	c, err := cartridge.Open(g.pinDir)
	if err != nil {
		return err
	}
	defer c.Close()
	rd, err := c.Trace()
	if err != nil {
		return err
	}
	defer rd.Close()
	s0, err := c.Stats()
	if err != nil {
		return err
	}
	if err := c.SetCfg(cartridge.CfgTrace, *every); err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	deadline := time.After(*dur)
	var recs [][]byte
	t := time.NewTicker(100 * time.Millisecond)
	defer t.Stop()
loop:
	for {
		more, err := c.DrainTrace(rd)
		if err != nil {
			_ = c.SetCfg(cartridge.CfgTrace, 0)
			return err
		}
		recs = append(recs, more...)
		select {
		case <-ctx.Done():
			break loop
		case <-deadline:
			break loop
		case <-t.C:
		}
	}
	if err := c.SetCfg(cartridge.CfgTrace, 0); err != nil {
		return err
	}
	more, err := c.DrainTrace(rd)
	if err != nil {
		return err
	}
	recs = append(recs, more...)
	slots, err := readSlots(g.registryDir)
	if err != nil {
		return err
	}
	if err := os.WriteFile(*out, cartridge.EncodeTrace(slotSHAs(slots), recs), 0o600); err != nil {
		return err
	}
	s1, _ := c.Stats()
	runs := map[int]int{}
	for _, r := range recs {
		if s := cartridge.TraceRecordSlot(r); s >= 0 {
			runs[s]++
		}
	}
	fmt.Printf("recorded %d records, runs per slot %v, %d records dropped (ring full) -> %s\n",
		len(recs), runs, s1[cartridge.StatTraceDropped]-s0[cartridge.StatTraceDropped], *out)
	return nil
}

// cmdReplay runs `cart replay` on a trace for one slot, with the image the
// trace recorded for that slot taken from the registry blobs (sha-checked),
// or --image. Extra flags (--run, --to, --watch, --steps) pass through. The
// exit status is cart's: 0 bit-exact, 3 divergence.
func cmdReplay(g *globals, args []string) error {
	if len(args) < 1 {
		return errors.New("usage: replay TRACE --slot N [--image X.cart] [--run K] [--to N] [--watch rN|mem[A]] [--steps]")
	}
	fs := flag.NewFlagSet("replay", flag.ExitOnError)
	slot := slotFlag(fs, "slot")
	image := fs.String("image", "", "the .cart the slot ran (default: the registry blob the trace names)")
	run := fs.String("run", "", "only run K of the slot")
	to := fs.String("to", "", "print the state at step N")
	watch := fs.String("watch", "", "follow rN or mem[ADDR]")
	steps := fs.Bool("steps", false, "print every step")
	_ = fs.Parse(args[1:])
	if err := checkSlot(*slot); err != nil {
		return err
	}
	// Only the header is needed here (cart replay reads the whole trace).
	f, err := os.Open(args[0]) // #nosec G304 -- operator-supplied trace path
	if err != nil {
		return err
	}
	trace := make([]byte, 8+32*cartridge.Slots)
	_, err = io.ReadFull(f, trace)
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return fmt.Errorf("%s: %w", args[0], err)
	}
	img := *image
	if img == "" {
		sha, err := traceSlotSHA(trace, *slot)
		if err != nil {
			return err
		}
		if sha == "" {
			return fmt.Errorf("the trace does not name slot %d's image; pass --image", *slot)
		}
		raw, _, err := (&FileRegistry{Dir: g.registryDir}).getBlobs(sha)
		if err != nil {
			return fmt.Errorf("image %s: %w", sha, err)
		}
		dir, err := os.MkdirTemp("", "cart-replay-")
		if err != nil {
			return err
		}
		defer func() { _ = os.RemoveAll(dir) }()
		img = filepath.Join(dir, sha+".cart")
		if err := os.WriteFile(img, raw, 0o600); err != nil {
			return err
		}
	}
	cargs := []string{"replay", img, args[0], "--slot", fmt.Sprint(*slot)}
	for _, kv := range [][2]string{{"--run", *run}, {"--to", *to}, {"--watch", *watch}} {
		if kv[1] != "" {
			cargs = append(cargs, kv[0], kv[1])
		}
	}
	if *steps {
		cargs = append(cargs, "--steps")
	}
	cmd := exec.Command(g.cartBin, cargs...) // #nosec G204 -- operator-configured tool path, validated flag values
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	err = cmd.Run()
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		return &exitError{code: ee.ExitCode()}
	}
	return err
}

// exitError carries cart's exit status (3 = divergence) out of cmdReplay.
type exitError struct{ code int }

func (e *exitError) Error() string { return fmt.Sprintf("cart replay exited %d", e.code) }
