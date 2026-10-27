// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// e2e-trace: one packet event with a fresh trace id is published to Wotan
// on the bench topic (default bench.perf.packet, which dashboard-backend's
// eBPF ingestor consumes beside ebpf.packet.events), and the dashboard's
// /api/v1/ebpf/events, the endpoint the browser polls, is read until the
// event with that trace id is there. The time between the publish and the
// read that returns it is the sample: Wotan, the dashboard's gRPC stream,
// its ingest, its ring and the HTTP read the page makes. It does not
// include XDP (the event is synthetic) or the page's own poll interval.

const (
	eventsPage     = 1000 // the dashboard's maximum ?limit
	maxEventsBody  = 8 << 20
	defaultE2EPoll = 5 * time.Millisecond
)

type traceID struct {
	High uint64 `json:"high"`
	Low  uint64 `json:"low"`
}

func newTraceID() (traceID, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return traceID{}, err
	}
	t := traceID{High: binary.BigEndian.Uint64(b[:8]), Low: binary.BigEndian.Uint64(b[8:])}
	if t.High == 0 && t.Low == 0 {
		t.Low = 1 // zero is "no trace id" on the real path
	}
	return t, nil
}

// benchPacket is a dashboard PacketEvent between two addresses of the
// RFC 2544 benchmarking range (198.18.0.0/15), so it can never be read as
// a real host's flow. It carries metadata only.
func benchPacket(id traceID, at time.Time) []byte {
	ev := map[string]any{
		"timestamp_ns": at.UnixNano(),
		"trace_id":     id,
		"flow_key": map[string]any{
			"src_addr": "198.18.0.1", "dst_addr": "198.18.0.2",
			"src_port": 40000, "dst_port": 9, "protocol": 17,
		},
		"packet_len": 64,
		"action":     "pass",
		"direction":  "ingress",
	}
	b, _ := json.Marshal(ev) // a map of plain values always marshals
	return b
}

type e2eRunner struct {
	cfg    Config
	client *http.Client
	subID  string
	cursor uint64
	gaps   int
}

func runE2ETrace(ctx context.Context, cfg Config) Result {
	if cfg.Poll <= 0 {
		cfg.Poll = defaultE2EPoll
	}
	if cfg.E2ETimeout <= 0 {
		cfg.E2ETimeout = 5 * time.Second
	}
	e := &e2eRunner{cfg: cfg, client: localClient(5*time.Second, 4)}
	target := fmt.Sprintf("%s topic %s -> %s/api/v1/ebpf/events", cfg.WotanURL, cfg.E2ETopic, cfg.DashboardURL)

	fail := func(err error) Result {
		r := summarize(scenarioE2E, nil, cfg.Iterations)
		r.Target = target
		r.FirstError = err.Error()
		return r
	}
	if err := e.subscribe(ctx); err != nil {
		return fail(err)
	}
	_, last, err := e.events(ctx, 0, false)
	if err != nil {
		return fail(fmt.Errorf("read dashboard cursor: %w", err))
	}
	e.cursor = last

	var lat []time.Duration
	errs := 0
	first := ""
	for i := 0; i < cfg.Iterations && ctx.Err() == nil; i++ {
		d, err := e.once(ctx)
		if err != nil {
			errs++
			if first == "" {
				first = err.Error()
			}
			continue
		}
		lat = append(lat, d)
	}
	r := summarize(scenarioE2E, lat, errs)
	r.Target = target
	r.FirstError = first
	r.Note = fmt.Sprintf("publish -> event readable at the dashboard API, polled every %v (resolution); no XDP leg", cfg.Poll)
	if e.gaps > 0 {
		r.Note += fmt.Sprintf("; dashboard ring overran between reads %d times (events skipped)", e.gaps)
	}
	return r
}

// subscribe gets a publisher id for the bench topic. Wotan approves it
// only for a display name in topics.auto_approve (configs/wotan.yaml).
func (e *e2eRunner) subscribe(ctx context.Context) error {
	body, _ := json.Marshal(map[string]string{"display_name": e.cfg.WotanName})
	u := strings.TrimRight(e.cfg.WotanURL, "/") + "/api/v1/topics/" + url.PathEscape(e.cfg.E2ETopic) + "/subscribe"
	var resp struct {
		Subscriber struct {
			SubscriberID string `json:"subscriber_id"`
			Status       string `json:"status"`
		} `json:"subscriber"`
	}
	if err := e.post(ctx, u, body, &resp); err != nil {
		return fmt.Errorf("wotan subscribe: %w", err)
	}
	if resp.Subscriber.Status != "approved" || resp.Subscriber.SubscriberID == "" {
		return fmt.Errorf("wotan subscribe as %q on %s: status %q; the name must be in topics.auto_approve (configs/wotan.yaml)",
			e.cfg.WotanName, e.cfg.E2ETopic, resp.Subscriber.Status)
	}
	e.subID = resp.Subscriber.SubscriberID
	return nil
}

func (e *e2eRunner) once(ctx context.Context) (time.Duration, error) {
	id, err := newTraceID()
	if err != nil {
		return 0, err
	}
	payload, _ := json.Marshal(map[string]string{"subscriber_id": e.subID, "payload": string(benchPacket(id, time.Now()))})
	u := strings.TrimRight(e.cfg.WotanURL, "/") + "/api/v1/topics/" + url.PathEscape(e.cfg.E2ETopic) + "/publish"

	start := time.Now()
	if err := e.post(ctx, u, payload, nil); err != nil {
		return 0, fmt.Errorf("wotan publish: %w", err)
	}
	wctx, cancel := context.WithTimeout(ctx, e.cfg.E2ETimeout)
	defer cancel()
	for {
		found, last, err := e.events(wctx, e.cursor, true, id)
		if err == nil {
			if last > e.cursor {
				e.cursor = last
			}
			if found {
				return time.Since(start), nil
			}
		}
		if sleepCtx(wctx, e.cfg.Poll) != nil {
			if err != nil {
				return 0, fmt.Errorf("trace %016x%016x not seen at the dashboard within %v (last read error: %v)", id.High, id.Low, e.cfg.E2ETimeout, err)
			}
			return 0, fmt.Errorf("trace %016x%016x not seen at the dashboard within %v", id.High, id.Low, e.cfg.E2ETimeout)
		}
	}
}

// events reads the dashboard's event ring after cursor (or, with paged
// false, just its newest seq) and reports whether an event on the bench
// topic carries want. The dashboard answers newest first, at most `limit`
// events newer than after_seq, with last_seq the ring's newest; one read
// of eventsPage covers the whole ring (EventBufferSize 1000), so a full
// page whose oldest event is not cursor+1 means the ring overran.
func (e *e2eRunner) events(ctx context.Context, cursor uint64, paged bool, want ...traceID) (bool, uint64, error) {
	base := strings.TrimRight(e.cfg.DashboardURL, "/") + "/api/v1/ebpf/events"
	var page eventsPageBody
	if !paged {
		if err := e.get(ctx, base+"?limit=1", &page); err != nil {
			return false, 0, err
		}
		return false, page.LastSeq, nil
	}
	if err := e.get(ctx, base+"?limit="+strconv.Itoa(eventsPage)+"&after_seq="+strconv.FormatUint(cursor, 10), &page); err != nil {
		return false, cursor, err
	}
	found := false
	oldest := uint64(0)
	for _, ev := range page.Events {
		if oldest == 0 || ev.Seq < oldest {
			oldest = ev.Seq
		}
		if ev.Topic != e.cfg.E2ETopic || len(want) == 0 {
			continue
		}
		var d struct {
			TraceID traceID `json:"trace_id"`
		}
		if json.Unmarshal(ev.Data, &d) == nil && d.TraceID == want[0] {
			found = true
		}
	}
	if len(page.Events) >= eventsPage && cursor > 0 && oldest > cursor+1 {
		e.gaps++
	}
	if page.LastSeq > cursor {
		cursor = page.LastSeq
	}
	return found, cursor, nil
}

type eventsPageBody struct {
	Events []struct {
		Seq   uint64          `json:"seq"`
		Topic string          `json:"topic"`
		Data  json.RawMessage `json:"data"`
	} `json:"events"`
	LastSeq uint64 `json:"last_seq"`
}

func (e *e2eRunner) get(ctx context.Context, u string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return err
	}
	return e.do(req, out)
}

func (e *e2eRunner) post(ctx context.Context, u string, body []byte, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	return e.do(req, out)
}

func (e *e2eRunner) do(req *http.Request, out any) error {
	resp, err := e.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxEventsBody))
	if err != nil {
		return err
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		msg := strings.TrimSpace(string(data))
		if len(msg) > 200 {
			msg = msg[:200]
		}
		return fmt.Errorf("%s %s: %d %s", req.Method, req.URL.Path, resp.StatusCode, msg)
	}
	if out == nil {
		return nil
	}
	if err := json.Unmarshal(data, out); err != nil {
		return errors.Join(fmt.Errorf("%s %s: decode", req.Method, req.URL.Path), err)
	}
	return nil
}
