// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"unheaded/pkg/claudebridge"
)

// `zhen-agentd -worker -overnight` (S-TWO-BRAINS Phase 7): the worker
// unattended inside a time window. Before every card it checks, in order:
// the stop file, the window's end, the cards-per-night limit, the
// overnight ledger (its own file and cap, never the interactive one), and
// a memory floor computed from MemAvailable only. Swap is never counted:
// fitting in swap is how a box livelocks. A held GPU lease, or too little
// memory, defers the card to the next poll; two consecutive
// infrastructure failures (not model failures) end the night. Every stop
// has a typed reason, logged, published on zhen.agent.runs and written
// into the morning report. Privacy is strict: no Kingdom-ops tool result
// leaves the host while nobody watches.

// stopReason says why a night ended.
type stopReason string

// Stop reasons.
const (
	stopWindowEnd     stopReason = "window_end"
	stopOutsideWindow stopReason = "outside_window"
	stopStopFile      stopReason = "stop_file"
	stopMaxCards      stopReason = "max_cards"
	stopSpendCap      stopReason = "spend_cap"
	stopInfraFailures stopReason = "infra_failures"
	stopSignal        stopReason = "signal"
)

// Deferral reasons.
const (
	deferGPU      = "gpu_lease"
	deferMemFloor = "mem_floor"
)

// runCheckMemoryMax is the run_check unit's MemoryMax (pkg/sandbox
// DefaultMemoryMax, "2G"): a card may need that much on top of the
// headroom.
const runCheckMemoryMax = 2 << 30

// defaultMemFloor is MemAvailable needed before a card: 2 GiB of headroom
// plus what one check may take. ZHEN_MEM_FLOOR_BYTES overrides it.
const defaultMemFloor int64 = 2<<30 + runCheckMemoryMax

// maxInfraFailures in a row end the night.
const maxInfraFailures = 2

// window is a daily local-time window; it may cross midnight.
type window struct{ start, end int } // minutes after local midnight

var windowRE = regexp.MustCompile(`^([01][0-9]|2[0-3]):([0-5][0-9])-([01][0-9]|2[0-3]):([0-5][0-9])$`)

func parseWindow(s string) (window, error) {
	m := windowRE.FindStringSubmatch(s)
	if m == nil {
		return window{}, fmt.Errorf("window %q: want HH:MM-HH:MM", s)
	}
	n := func(i int) int { v, _ := strconv.Atoi(m[i]); return v }
	w := window{start: n(1)*60 + n(2), end: n(3)*60 + n(4)}
	if w.start == w.end {
		return window{}, fmt.Errorf("window %q is empty", s)
	}
	return w, nil
}

func minuteOf(t time.Time) int { return t.Hour()*60 + t.Minute() }

func (w window) contains(t time.Time) bool {
	m := minuteOf(t)
	if w.start < w.end {
		return m >= w.start && m < w.end
	}
	return m >= w.start || m < w.end
}

// endAfter is the window's next end after t.
func (w window) endAfter(t time.Time) time.Time {
	day := time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
	end := day.Add(time.Duration(w.end) * time.Minute)
	if !end.After(t) {
		end = end.AddDate(0, 0, 1)
	}
	return end
}

func (w window) String() string {
	return fmt.Sprintf("%02d:%02d-%02d:%02d", w.start/60, w.start%60, w.end/60, w.end%60)
}

// memAvailable reads MemAvailable (bytes) from a /proc/meminfo file. It
// never reads, and never falls back to, any swap figure.
func memAvailable(path string) (int64, error) {
	f, err := os.Open(path) // #nosec G304 -- /proc/meminfo, or a test's fake
	if err != nil {
		return 0, err
	}
	defer func() { _ = f.Close() }()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		fields := strings.Fields(sc.Text())
		if len(fields) >= 2 && fields[0] == "MemAvailable:" {
			kb, err := strconv.ParseInt(fields[1], 10, 64)
			if err != nil || kb < 0 {
				return 0, fmt.Errorf("meminfo: MemAvailable %q", fields[1])
			}
			return kb * 1024, nil
		}
	}
	if err := sc.Err(); err != nil {
		return 0, err
	}
	return 0, errors.New("meminfo: no MemAvailable line")
}

// overnightConfig are the night's limits.
type overnightConfig struct {
	Window   window
	MaxCards int
	Ledger   *claudebridge.Ledger // the overnight ledger and its cap
	StopFile string
	MemFloor int64
	MemInfo  string // /proc/meminfo
	Poll     time.Duration
	Now      func() time.Time
	Sleep    func(ctx context.Context, d time.Duration) error
}

func (oc overnightConfig) stopRequested() bool {
	if oc.StopFile == "" {
		return false
	}
	_, err := os.Lstat(oc.StopFile)
	return err == nil
}

func sleepCtx(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

// nightReport is what a night did (the morning report's data).
type nightReport struct {
	Window      string
	Start, End  time.Time
	Stop        stopReason
	StopDetail  string
	Tried       int // cards claimed
	Succeeded   int
	Failed      int
	Cancelled   int
	Deferred    int
	Deferrals   map[string]int
	Escalations map[string]int // by route_reason
	LocalTries  int            // local model attempts over all cards
	SpentNano   claudebridge.NanoUSD
	CapNano     claudebridge.NanoUSD
	Results     []cardResult
	Errors      []string
}

// runNight works cards until a stop condition holds.
func runNight(ctx context.Context, w *worker, oc overnightConfig) (rep nightReport) {
	if oc.Now == nil {
		oc.Now = time.Now
	}
	if oc.Sleep == nil {
		oc.Sleep = sleepCtx
	}
	if oc.Poll <= 0 {
		oc.Poll = time.Minute
	}
	if oc.MemInfo == "" {
		oc.MemInfo = "/proc/meminfo"
	}
	rep = nightReport{Window: oc.Window.String(), Start: oc.Now(), Deferrals: map[string]int{}, Escalations: map[string]int{}}
	spent0 := spentNow(oc.Ledger)
	if oc.Ledger != nil {
		rep.CapNano = oc.Ledger.Cap()
	}
	stop := func(r stopReason, detail string) nightReport {
		rep.Stop, rep.StopDetail, rep.End = r, detail, oc.Now()
		rep.SpentNano = spentNow(oc.Ledger) - spent0
		w.cfg.Log.Info("night_stop", "reason", string(r), "detail", detail, "tried", rep.Tried, "succeeded", rep.Succeeded,
			"failed", rep.Failed, "deferred", rep.Deferred, "spent_usd", rep.SpentNano.USD())
		w.event(context.WithoutCancel(ctx), runEvent{Kind: "night_stop", Stop: string(r), CostNanoUSD: int64(rep.SpentNano)})
		return rep
	}
	if !oc.Window.contains(oc.Now()) {
		return stop(stopOutsideWindow, "started at "+oc.Now().Format("15:04")+", window "+oc.Window.String())
	}
	end := oc.Window.endAfter(oc.Now())
	infra := 0
	wait := func() bool { return oc.Sleep(ctx, min(oc.Poll, max(end.Sub(oc.Now()), 0)+time.Second)) == nil }
	for {
		switch {
		case ctx.Err() != nil:
			return stop(stopSignal, ctx.Err().Error())
		case oc.stopRequested():
			return stop(stopStopFile, oc.StopFile)
		case !oc.Now().Before(end):
			return stop(stopWindowEnd, "window "+oc.Window.String())
		case rep.Tried >= oc.MaxCards:
			return stop(stopMaxCards, fmt.Sprintf("%d cards", oc.MaxCards))
		}
		if oc.Ledger != nil {
			spent, err := oc.Ledger.Spent()
			if err != nil {
				return stop(stopSpendCap, "overnight ledger unreadable: "+err.Error())
			}
			if spent >= oc.Ledger.Cap() {
				return stop(stopSpendCap, "spent "+spent.USD()+" of "+oc.Ledger.Cap().USD())
			}
		}
		if avail, err := memAvailable(oc.MemInfo); err != nil || avail < oc.MemFloor {
			why := fmt.Sprintf("MemAvailable %d B < floor %d B (swap not counted)", avail, oc.MemFloor)
			if err != nil {
				why = "meminfo: " + err.Error()
			}
			rep.Deferred++
			rep.Deferrals[deferMemFloor]++
			w.cfg.Log.Warn("night_defer", "reason", deferMemFloor, "detail", why)
			if !wait() {
				return stop(stopSignal, "cancelled while waiting")
			}
			continue
		}
		res, err := w.runOnce(ctx)
		if err != nil {
			rep.Errors = append(rep.Errors, err.Error())
		}
		switch res.Outcome {
		case outcomeNone:
			if err != nil {
				infra++
			}
		case outcomeDeferred:
			rep.Deferred++
			reason := deferGPU
			rep.Deferrals[reason]++
			if res.Run != "" { // claimed, then cancelled mid-card: it was tried
				rep.Tried++
				rep.Results = append(rep.Results, res)
			}
		default:
			rep.Tried++
			rep.Results = append(rep.Results, res)
			rep.LocalTries += res.Route.LocalAttempts
			if res.Route.Escalated || res.Route.RouteReason == "claude_direct" {
				rep.Escalations[res.Route.RouteReason]++
			}
			switch res.Outcome {
			case outcomeSucceeded:
				rep.Succeeded++
			case outcomeFailed:
				rep.Failed++
			case outcomeCancelled:
				rep.Cancelled++
			}
			if res.Outcome == outcomeFailed && res.Infra {
				infra++
			} else {
				infra = 0
			}
		}
		if infra >= maxInfraFailures {
			return stop(stopInfraFailures, fmt.Sprintf("%d infrastructure failures in a row (last: %s)", infra, oneLine(res.Reason, 200)))
		}
		if res.Outcome == outcomeNone || res.Outcome == outcomeDeferred {
			if !wait() {
				return stop(stopSignal, "cancelled while waiting")
			}
		}
	}
}

func spentNow(l *claudebridge.Ledger) claudebridge.NanoUSD {
	if l == nil {
		return 0
	}
	s, err := l.Spent()
	if err != nil {
		return 0
	}
	return s
}

// memFloorFromEnv is ZHEN_MEM_FLOOR_BYTES or the default.
func memFloorFromEnv() (int64, error) {
	v := strings.TrimSpace(os.Getenv("ZHEN_MEM_FLOOR_BYTES"))
	if v == "" {
		return defaultMemFloor, nil
	}
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil || n < 0 {
		return 0, fmt.Errorf("ZHEN_MEM_FLOOR_BYTES %q", v)
	}
	if n < defaultMemFloor {
		return 0, fmt.Errorf("ZHEN_MEM_FLOOR_BYTES %d is below the default floor %d; it may only be raised", n, defaultMemFloor)
	}
	return n, nil
}

// applyOvernightDefaults: -overnight implies -worker, strict privacy, its
// own ledger (default $STATE/zhen-claude/spend-overnight.json) and cap
// (default $3, CAP_USD_OVERNIGHT), never the interactive ledger.
func applyOvernightDefaults(wf *workerFlags, set map[string]bool, state string) error {
	wf.enabled = true
	wf.privacyMode = "strict"
	if !set["ledger"] || wf.ledger == "" {
		wf.ledger = filepath.Join(state, "zhen-claude", "spend-overnight.json")
	}
	if !set["daily-cap-usd"] {
		wf.dailyCapUSD = 3
	}
	interactive := filepath.Join(state, "zhen-claude", "spend.json")
	if filepath.Clean(wf.ledger) == interactive {
		return fmt.Errorf("-overnight needs its own ledger, not the interactive %s", interactive)
	}
	if wf.dailyCapUSD <= 0 || wf.dailyCapUSD > claudebridge.MaxRouteClaudeUSD*4 {
		return fmt.Errorf("-daily-cap-usd %v out of range for a night", wf.dailyCapUSD)
	}
	return nil
}
