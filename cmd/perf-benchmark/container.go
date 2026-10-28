// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// container-start: for each iteration one steel-echo instance named
// steel-echo-bench-<i> is made desired in Cuirass (PUT
// /api/v1/containers/{id}), Cuirass is asked to reconcile (at most once a
// second, its own limit), and its actual state is read until the instance
// is managed, running and healthy by Cuirass's own probe. That interval is
// the sample. The instance is then deleted and the harness waits until
// Cuirass no longer reports it, whether the start succeeded or not.
//
// The instance has its own svc label (steel-echo-bench), no proxy device
// and no chaos label, so Sword, the steel-echo meter and the chaos
// endpoint never pick it up. A leftover steel-echo-bench-* refuses the run
// rather than being deleted: the harness removes only what it made.

const (
	benchSvc        = "steel-echo-bench"
	benchPrefix     = benchSvc + "-"
	reconcileGap    = time.Second
	defaultCSPoll   = 250 * time.Millisecond
	cuirassReqLimit = 10 * time.Second
)

type csSpec struct {
	ID     string            `json:"id"`
	Name   string            `json:"name"`
	Image  string            `json:"image"`
	Labels map[string]string `json:"labels"`
	Health *csHealth         `json:"health"`
}

type csHealth struct {
	Port int    `json:"port"`
	Path string `json:"path"`
}

type csState struct {
	Status  string `json:"status"`
	Health  string `json:"health"`
	Managed bool   `json:"managed"`
}

type cuirassClient struct {
	base, key     string
	c             *http.Client
	lastReconcile time.Time
}

func runContainerStart(ctx context.Context, cfg Config) Result {
	poll := cfg.Poll
	if poll <= 0 || poll > time.Second {
		poll = defaultCSPoll
	}
	timeout := cfg.CSTimeout
	if timeout <= 0 {
		timeout = 2 * time.Minute
	}
	image := cfg.CSImage
	if image == "" {
		image = "steel-echo-v1"
	}
	cc := &cuirassClient{base: strings.TrimRight(cfg.CuirassURL, "/"), key: cfg.CuirassKey, c: localClient(cuirassReqLimit, 2)}
	target := cc.base + " image " + image

	fail := func(err error) Result {
		r := summarize(scenarioContainerStart, nil, cfg.Iterations)
		r.Target = target
		r.FirstError = err.Error()
		return r
	}
	var desired map[string]json.RawMessage
	if err := cc.do(ctx, http.MethodGet, "/api/v1/state/desired", nil, &desired); err != nil {
		return fail(err)
	}
	var actual map[string]csState
	if err := cc.do(ctx, http.MethodGet, "/api/v1/state/actual", nil, &actual); err != nil {
		return fail(err)
	}
	for id := range desired {
		if strings.HasPrefix(id, benchPrefix) {
			return fail(fmt.Errorf("leftover bench instance %s is desired in Cuirass; remove it first (the harness deletes only what it made)", id))
		}
	}
	for id := range actual {
		if strings.HasPrefix(id, benchPrefix) {
			return fail(fmt.Errorf("leftover bench instance %s exists; remove it first (the harness deletes only what it made)", id))
		}
	}

	var lat []time.Duration
	errs := 0
	first := ""
	note := ""
	for i := 0; i < cfg.Iterations && ctx.Err() == nil; i++ {
		id := fmt.Sprintf("%s%d", benchPrefix, i)
		d, maybeCreated, err := cc.startOnce(ctx, id, image, poll, timeout)
		// Delete whatever may exist, also after a failure or a canceled
		// run. Only a PUT that Cuirass answered with an error made nothing.
		if maybeCreated {
			cctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), timeout)
			derr := cc.deleteAndWait(cctx, id, poll)
			cancel()
			if derr != nil {
				note += fmt.Sprintf("LEFT BEHIND %s: %v; ", id, derr)
			}
		}
		if err != nil {
			errs++
			if first == "" {
				first = err.Error()
			}
			continue
		}
		lat = append(lat, d)
	}
	r := summarize(scenarioContainerStart, lat, errs)
	r.Target = target
	r.FirstError = first
	r.Note = note + fmt.Sprintf("PUT -> Cuirass reports running+healthy (its probe), read every %v with a reconcile at most once a second", poll)
	return r
}

// startOnce reports maybeCreated false only when Cuirass refused the PUT.
func (cc *cuirassClient) startOnce(ctx context.Context, id, image string, poll, timeout time.Duration) (_ time.Duration, maybeCreated bool, _ error) {
	spec := csSpec{ID: id, Name: id, Image: image,
		Labels: map[string]string{"svc": benchSvc, "bench": "perf"},
		Health: &csHealth{Port: 8080, Path: "/health"}}
	start := time.Now()
	if err := cc.do(ctx, http.MethodPut, "/api/v1/containers/"+url.PathEscape(id), spec, nil); err != nil {
		var se *statusError
		return 0, !errors.As(err, &se), err
	}
	wctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	var lastErr error
	for {
		cc.reconcile(wctx)
		var actual map[string]csState
		err := cc.do(wctx, http.MethodGet, "/api/v1/state/actual", nil, &actual)
		if err == nil {
			if s, ok := actual[id]; ok && s.Managed && s.Status == "running" && s.Health == "healthy" {
				return time.Since(start), true, nil
			}
		} else {
			lastErr = err
		}
		if sleepCtx(wctx, poll) != nil {
			if lastErr != nil {
				return 0, true, fmt.Errorf("%s not healthy within %v (last error: %v)", id, timeout, lastErr)
			}
			return 0, true, fmt.Errorf("%s not healthy within %v", id, timeout)
		}
	}
}

func (cc *cuirassClient) deleteAndWait(ctx context.Context, id string, poll time.Duration) error {
	err := cc.do(ctx, http.MethodDelete, "/api/v1/containers/"+url.PathEscape(id), nil, nil)
	var se *statusError
	if err != nil && (!errors.As(err, &se) || se.code != http.StatusNotFound) {
		return err
	}
	for {
		cc.reconcile(ctx)
		var actual map[string]csState
		if err := cc.do(ctx, http.MethodGet, "/api/v1/state/actual", nil, &actual); err == nil {
			if _, still := actual[id]; !still {
				return nil
			}
		}
		if err := sleepCtx(ctx, poll); err != nil {
			return fmt.Errorf("still reported by Cuirass: %w", err)
		}
	}
}

// reconcile asks for a sync now; Cuirass allows one a second and answers
// 429 otherwise, which is fine: its own 30 s loop runs regardless.
func (cc *cuirassClient) reconcile(ctx context.Context) {
	if time.Since(cc.lastReconcile) < reconcileGap {
		return
	}
	cc.lastReconcile = time.Now()
	_ = cc.do(ctx, http.MethodPost, "/api/v1/reconcile", nil, nil)
}

type statusError struct {
	method, path string
	code         int
	body         string
}

func (e *statusError) Error() string {
	return fmt.Sprintf("cuirass %s %s: %d %s", e.method, e.path, e.code, e.body)
}

func (cc *cuirassClient) do(ctx context.Context, method, path string, body, out any) error {
	var rd io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		rd = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, cc.base+path, rd)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if method != http.MethodGet {
		req.Header.Set("X-API-Key", cc.key)
	}
	resp, err := cc.c.Do(req)
	if err != nil {
		return fmt.Errorf("cuirass %s %s: %w", method, path, err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return err
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		msg := strings.TrimSpace(string(data))
		if len(msg) > 200 {
			msg = msg[:200]
		}
		return &statusError{method: method, path: path, code: resp.StatusCode, body: msg}
	}
	if out != nil {
		return json.Unmarshal(data, out)
	}
	return nil
}
