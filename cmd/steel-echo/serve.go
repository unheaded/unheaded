// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"math"
	"math/rand/v2"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"sync"
	"syscall"
	"time"

	"unheaded/pkg/httputil"
	"unheaded/pkg/metrics"
)

const (
	serviceName = "steel-echo"
	maxLatency  = 10 * time.Second
)

// shutdownSignals end serve cleanly. LXD's stop sends SIGPWR to the
// container's init, and OpenRC passes SIGTERM to its services; either may
// reach us depending on how the image runs the binary.
var shutdownSignals = []os.Signal{syscall.SIGTERM, syscall.SIGINT, syscall.SIGPWR}

// knobs are a build's deliberate faults.
type knobs struct {
	errorRate float64       // fraction of / answered 500
	latency   time.Duration // added to every /
}

// parseKnobs reads STEEL_ERROR_RATE and STEEL_LATENCY_MS. A value out of
// range refuses to start rather than being clamped: an image whose fault is
// not the one its tag says would make the game day lie.
func parseKnobs(getenv func(string) string) (knobs, error) {
	var k knobs
	if s := getenv("STEEL_ERROR_RATE"); s != "" {
		r, err := strconv.ParseFloat(s, 64)
		if err != nil || math.IsNaN(r) || r < 0 || r > 1 {
			return knobs{}, fmt.Errorf("STEEL_ERROR_RATE %q: want a number in 0..1", s)
		}
		k.errorRate = r
	}
	if s := getenv("STEEL_LATENCY_MS"); s != "" {
		ms, err := strconv.Atoi(s)
		if err != nil || ms < 0 || time.Duration(ms)*time.Millisecond > maxLatency {
			return knobs{}, fmt.Errorf("STEEL_LATENCY_MS %q: want whole milliseconds in 0..%d", s, maxLatency.Milliseconds())
		}
		k.latency = time.Duration(ms) * time.Millisecond
	}
	return k, nil
}

type server struct {
	version  string
	hostname string
	knobs    knobs

	mu  sync.Mutex
	rnd *rand.Rand
}

func newServer(version, hostname string, k knobs, seed uint64) *server {
	return &server{
		version:  version,
		hostname: hostname,
		knobs:    k,
		// #nosec G404 -- fault injection draws, not a security decision; a
		// fixed seed keeps a build's error pattern reproducible.
		rnd: rand.New(rand.NewPCG(seed, seed^0x5eed)),
	}
}

func (s *server) fail() bool {
	if s.knobs.errorRate <= 0 {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.rnd.Float64() < s.knobs.errorRate
}

func (s *server) handler() http.Handler {
	mux := http.NewServeMux()
	// Plain patterns (no method, no wildcard) so the unheaded_http_* path
	// label reads "/health", "/" and so on.
	mux.HandleFunc("/health", getOnly(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = w.Write([]byte("ok\n"))
	}))
	mux.HandleFunc("/version", getOnly(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = w.Write([]byte(s.version + "\n"))
	}))
	mux.Handle("/metrics", getOnly(metrics.HandlerFor(metrics.DefaultRegistry).ServeHTTP))
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			// A fixed body: never the path the client asked for.
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		getOnly(s.root)(w, r)
	})
	return httputil.SecurityHeaders(httputil.NewServiceMetrics(serviceName).Instrument(mux, mux))
}

// getOnly refuses everything but GET and HEAD.
func getOnly(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.Header().Set("Allow", "GET, HEAD")
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		h(w, r)
	}
}

// root answers with the hostname and version only. Nothing from the request
// (path, query, headers, body) is read or written back: zero user data access.
func (s *server) root(w http.ResponseWriter, r *http.Request) {
	if s.knobs.latency > 0 {
		t := time.NewTimer(s.knobs.latency)
		select {
		case <-t.C:
		case <-r.Context().Done():
			t.Stop()
			return
		}
	}
	status := http.StatusOK
	if s.fail() {
		status = http.StatusInternalServerError
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"hostname": s.hostname, "version": s.version})
}

// serve runs the HTTP server until ctx ends, then drains it for up to 5 s.
func serve(ctx context.Context, addr string, s *server) error {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	srv := &http.Server{
		Handler:           s.handler(),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      maxLatency + 10*time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    16 << 10,
	}
	errc := make(chan error, 1)
	go func() { errc <- srv.Serve(ln) }()
	select {
	case err := <-errc:
		return err
	case <-ctx.Done():
	}
	sctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(sctx); err != nil {
		return err
	}
	if err := <-errc; err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

func runServe(args []string) error {
	fs := flag.NewFlagSet("serve", flag.ContinueOnError)
	addr := fs.String("addr", ":8080", "listen address")
	if err := fs.Parse(args); err != nil {
		return err
	}
	k, err := parseKnobs(os.Getenv)
	if err != nil {
		return err
	}
	host, err := os.Hostname()
	if err != nil {
		return fmt.Errorf("hostname: %w", err)
	}
	ctx, stop := signal.NotifyContext(context.Background(), shutdownSignals...)
	defer stop()
	fmt.Fprintf(os.Stderr, "steel-echo %s serving on %s (error_rate=%g latency=%v)\n", version, *addr, k.errorRate, k.latency)
	return serve(ctx, *addr, newServer(version, host, k, uint64(time.Now().UnixNano()))) // #nosec G115 -- a seed; sign is irrelevant
}
