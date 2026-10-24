// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

// anomaly — the first binary for services/anomaly (S5 Phase 15, ADR-108).
//
// It runs the Anomaly Detection service (REST API: /api/v1/anomalies/*,
// /health, /ready, /metrics) and bridges MBC cartridge scores into it: score
// events (EMIT kind 'A' from a tick-stage scorer cartridge) are read from the
// pinned cartridge interpreter's CART_EVENTS ring buffer and turned into
// anomaly.scores messages carrying flow metadata and a score, never payload.
//
// With --wotan the bridge publishes anomaly.scores to Wotan and the service
// consumes the topic (it subscribes to it); without it the bridge hands the
// same JSON straight to the service's intake (lab mode while the stack is
// down). Either way the message goes through the same validation.
//
// Usage:
//
//	anomaly [--listen 127.0.0.1:19018] [--pin-dir /sys/fs/bpf/unheaded/cart] [--wotan host:18000] [--no-cartridge]
//
// Reading the pinned ring buffer needs CAP_BPF (root on west).
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	"unheaded/pkg/cartridge"
	"unheaded/pkg/logger"
	"unheaded/pkg/ports"
	wotanClient "unheaded/pkg/wotan-client"
	"unheaded/services/anomaly"
)

// ScoreKind is the scorer cartridge's EMIT kind ('A'): a = score, b = model id.
const ScoreKind = 0x41

// scoreFromEvent turns a cartridge score event into an anomaly.scores
// message. Only tick-stage events of kind 'A' with a score in 0..100 are
// scores; anything else (sampler events, other kinds) is skipped.
func scoreFromEvent(e cartridge.Event) (anomaly.ScoreMessage, bool) {
	if e.Stage != cartridge.StageTick || e.Kind != ScoreKind || e.A > 100 {
		return anomaly.ScoreMessage{}, false
	}
	var src, dst net.IP
	switch e.Flow.L3 {
	case 4:
		src, dst = net.IP(e.Flow.Src[:4]), net.IP(e.Flow.Dst[:4])
	case 6:
		src, dst = net.IP(e.Flow.Src[:]), net.IP(e.Flow.Dst[:])
	default:
		return anomaly.ScoreMessage{}, false
	}
	return anomaly.ScoreMessage{
		FlowKey: anomaly.FlowKey{
			SrcIP: src.String(), DstIP: dst.String(),
			SrcPort: e.Flow.Sport, DstPort: e.Flow.Dport, Proto: e.Flow.Proto,
		},
		Score:       int(e.A),
		ModelID:     e.B,
		Source:      fmt.Sprintf("cartridge:%d", e.Slot),
		TimestampMS: time.Now().UnixMilli(),
	}, true
}

// sink receives one anomaly.scores payload.
type sink func(ctx context.Context, payload []byte) error

// bridge forwards every score in evs to out; it returns how many it sent.
func bridge(ctx context.Context, evs []cartridge.Event, out sink) (int, error) {
	n := 0
	for _, e := range evs {
		m, ok := scoreFromEvent(e)
		if !ok {
			continue
		}
		b, err := json.Marshal(m)
		if err != nil {
			return n, err
		}
		if err := out(ctx, b); err != nil {
			return n, err
		}
		n++
	}
	return n, nil
}

func main() {
	listen := flag.String("listen", "127.0.0.1:"+fmt.Sprint(ports.Anomaly), "HTTP listen address (loopback by default)")
	pinDir := flag.String("pin-dir", cartridge.DefaultPinDir, "bpffs directory of the pinned cartridge interpreter")
	wotanAddr := flag.String("wotan", "", "Wotan HTTP address: publish anomaly.scores there (default: in-process intake)")
	noCart := flag.Bool("no-cartridge", false, "run the service without the cartridge bridge")
	flag.Parse()

	log := logger.New(os.Stderr)
	var wc *wotanClient.Client
	if *wotanAddr != "" {
		c, err := wotanClient.NewClient(*wotanAddr)
		if err != nil {
			fmt.Fprintf(os.Stderr, "anomaly: wotan: %v\n", err)
			os.Exit(1)
		}
		wc = c
	}
	svc := anomaly.NewService(log, wc)
	svc.ListenAddr = *listen
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := svc.Start(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "anomaly: %v\n", err)
		os.Exit(1)
	}

	if !*noCart {
		out := sink(func(_ context.Context, b []byte) error { return svc.IngestScore(b) })
		if wc != nil {
			pub, err := wotanClient.NewClient(*wotanAddr)
			if err != nil {
				fmt.Fprintf(os.Stderr, "anomaly: wotan: %v\n", err)
				os.Exit(1)
			}
			if _, err := pub.Subscribe(ctx, anomaly.ScoreTopic, "anomaly-bridge"); err != nil {
				fmt.Fprintf(os.Stderr, "anomaly: subscribe %s: %v\n", anomaly.ScoreTopic, err)
				os.Exit(1)
			}
			out = func(ctx context.Context, b []byte) error { return pub.Publish(ctx, anomaly.ScoreTopic, b) }
		}
		go func() {
			if err := runBridge(ctx, *pinDir, out, os.Stderr); err != nil && !errors.Is(err, context.Canceled) {
				fmt.Fprintf(os.Stderr, "anomaly: cartridge bridge: %v\n", err)
			}
		}()
	}
	<-ctx.Done()
	_ = svc.Stop()
}

// logf is where the bridge reports progress.
type logf = io.Writer
