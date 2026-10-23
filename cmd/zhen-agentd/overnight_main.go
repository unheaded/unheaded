// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"unheaded/pkg/claudebridge"
)

// runOvernight is -worker -overnight: one night, a morning report, exit.
// Exit 0 for a guarded stop (window end, card limit, stop file, spend cap,
// outside the window, a signal), 1 when infrastructure failures ended it
// or nothing could be set up.
func runOvernight(ctx context.Context, w *worker, wf workerFlags, aud *agentdAudit, log *slog.Logger) int {
	win, err := parseWindow(wf.window)
	if err != nil {
		log.Error("overnight", "error", err.Error())
		return 1
	}
	capNano, err := claudebridge.USDToNano(wf.dailyCapUSD)
	if err != nil {
		log.Error("overnight", "error", err.Error())
		return 1
	}
	ledger, err := claudebridge.NewLedger(wf.ledger, capNano)
	if err != nil {
		log.Error("overnight", "error", "ledger: "+err.Error())
		return 1
	}
	floor, err := memFloorFromEnv()
	if err != nil {
		log.Error("overnight", "error", err.Error())
		return 1
	}
	oc := overnightConfig{
		Window: win, MaxCards: wf.maxCards, Ledger: ledger, StopFile: wf.stopFile, MemFloor: floor,
		MemInfo: "/proc/meminfo", Poll: wf.poll, Now: time.Now,
	}
	w.cfg.Stop = oc.stopRequested
	sctx, stop := signal.NotifyContext(ctx, syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	log.Info("night_start", "window", win.String(), "max_cards", wf.maxCards, "cap_usd", capNano.USD(), "ledger", wf.ledger,
		"mem_floor_bytes", floor, "stop_file", wf.stopFile, "claude", wf.claude, "privacy", wf.privacyMode)
	rep := runNight(sctx, w, oc)
	if path, err := writeMorningReport(wf.reportDir, rep, w); err != nil {
		log.Error("morning_report", "error", err.Error())
	} else {
		log.Info("morning_report", "path", path)
	}
	publishReport(aud, rep)
	if rep.Stop == stopInfraFailures {
		return 1
	}
	_ = os.Stderr.Sync()
	return 0
}
