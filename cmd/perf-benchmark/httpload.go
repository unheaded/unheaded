// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package main

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"
)

const loadTick = 5 * time.Millisecond

// runHTTPLoad sends GET -health at cfg.Rate per second for cfg.Duration,
// paced by a token bucket refilled every 5 ms. It is open-loop: a request's
// latency is measured from when the bucket released it, so time spent
// queued behind a slow server counts (no coordinated omission). A release
// that finds every worker busy and the queue full is dropped and reported
// in the note; the achieved rate is what completed.
func runHTTPLoad(ctx context.Context, cfg Config) Result {
	conc := cfg.Concurrency
	if conc < 1 {
		conc = 64
	}
	client := localClient(5*time.Second, conc)
	jobs := make(chan time.Time, conc*4)

	type workerOut struct {
		lat      []time.Duration
		errs     int
		firstErr string
	}
	outs := make([]workerOut, conc)
	var wg sync.WaitGroup
	for w := 0; w < conc; w++ {
		wg.Add(1)
		go func(o *workerOut) {
			defer wg.Done()
			for released := range jobs {
				if err := getOK(ctx, client, cfg.HealthURL); err != nil {
					o.errs++
					if o.firstErr == "" {
						o.firstErr = err.Error()
					}
					continue
				}
				o.lat = append(o.lat, time.Since(released))
			}
		}(&outs[w])
	}

	dctx, cancel := context.WithTimeout(ctx, cfg.Duration)
	defer cancel()
	start := time.Now()
	ticker := time.NewTicker(loadTick)
	var tokens float64
	var released, dropped int
	last := start
dispatch:
	for {
		select {
		case <-dctx.Done():
			break dispatch
		case now := <-ticker.C:
			tokens += now.Sub(last).Seconds() * float64(cfg.Rate)
			last = now
			for tokens >= 1 {
				tokens--
				select {
				case jobs <- now:
					released++
				default:
					dropped++
				}
			}
		}
	}
	ticker.Stop()
	close(jobs)
	wg.Wait()
	elapsed := time.Since(start)

	var lat []time.Duration
	errs := 0
	firstErr := ""
	for _, o := range outs {
		lat = append(lat, o.lat...)
		errs += o.errs
		if firstErr == "" {
			firstErr = o.firstErr
		}
	}
	r := summarize(scenarioHTTPLoad, lat, errs)
	r.Target = cfg.HealthURL
	r.FirstError = firstErr
	if elapsed > 0 {
		r.RateAchieved = float64(len(lat)+errs) / elapsed.Seconds()
	}
	r.Note = fmt.Sprintf("asked %d/s for %v with %d workers; released %d, dropped %d (workers and queue full)",
		cfg.Rate, cfg.Duration, conc, released, dropped)
	if ctx.Err() != nil {
		r.Note += "; run canceled before the duration ended"
	}
	return r
}

// getOK is one GET that must answer 2xx; the body is drained (capped) so
// the connection is reused.
func getOK(ctx context.Context, c *http.Client, url string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	resp, err := c.Do(req)
	if err != nil {
		return err
	}
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 64<<10))
	_ = resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return fmt.Errorf("GET %s: %d", url, resp.StatusCode)
	}
	return nil
}
