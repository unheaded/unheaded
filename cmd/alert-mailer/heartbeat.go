// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package main

// The dead-man's switch (ADR-100 section 4, S-WELL-SPRING Phase 6). Each
// watched host POSTs /heartbeat/<host> every two minutes with a shared
// token; the mailer records when it last heard from each one as
// unheaded_heartbeat_last_seen_timestamp_seconds{host}, and
// monitoring/prometheus/rules/heartbeat.yml raises HeartbeatMissing when a
// host has been quiet for ten minutes.
//
// The heartbeat has its own listener (-heartbeat-addr). The other host has to
// reach it, and it must not reach the Alertmanager webhooks or the outbox,
// which stay on the loopback listener.
//
// Off unless HEARTBEAT_HOSTS names hosts. Then HEARTBEAT_TOKEN_FILE must name
// a file holding at least 32 bytes that nobody but the owner can read; the
// mailer refuses to start otherwise.

import (
	"crypto/sha256"
	"crypto/subtle"
	"errors"
	"fmt"
	"net/http"
	"os"
	"regexp"
	"strings"
	"time"

	"unheaded/pkg/metrics"
)

const (
	heartbeatHeader   = "X-Heartbeat-Token"
	minHeartbeatToken = 32
)

var heartbeatHostRE = regexp.MustCompile(`^[a-z][a-z0-9-]{0,31}$`)

// Heartbeats records the last authenticated heartbeat from each host.
type Heartbeats struct {
	tokenSum [sha256.Size]byte
	hosts    map[string]bool
	last     *metrics.GaugeVec
	now      func() time.Time
}

// heartbeatFromEnv returns nil, nil when HEARTBEAT_HOSTS is empty.
func heartbeatFromEnv(getenv func(string) string, now func() time.Time) (*Heartbeats, error) {
	var hosts []string
	for _, h := range strings.Split(getenv("HEARTBEAT_HOSTS"), ",") {
		if h = strings.TrimSpace(h); h != "" {
			hosts = append(hosts, h)
		}
	}
	if len(hosts) == 0 {
		return nil, nil
	}
	hb := &Heartbeats{
		hosts: map[string]bool{},
		last: metrics.NewGaugeVec("unheaded_heartbeat_last_seen_timestamp_seconds",
			"Unix time of the last authenticated heartbeat from each watched host (process start until the first one)",
			nil, []string{"host"}),
		now: now,
	}
	for _, h := range hosts {
		if !heartbeatHostRE.MatchString(h) {
			return nil, fmt.Errorf("HEARTBEAT_HOSTS: %q is not a host name (want %s)", h, heartbeatHostRE)
		}
		hb.hosts[h] = true
	}
	path := getenv("HEARTBEAT_TOKEN_FILE")
	if path == "" {
		return nil, errors.New("HEARTBEAT_HOSTS is set without HEARTBEAT_TOKEN_FILE")
	}
	token, err := readHeartbeatToken(path)
	if err != nil {
		return nil, err
	}
	hb.tokenSum = sha256.Sum256(token)
	// A host that never beats goes missing one window after a restart.
	start := float64(now().Unix())
	for h := range hb.hosts {
		hb.last.WithLabels(metrics.Labels{"host": h}).Set(start)
	}
	return hb, nil
}

func readHeartbeatToken(path string) ([]byte, error) {
	st, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("heartbeat token: %w", err)
	}
	if st.Mode().Perm()&0o007 != 0 {
		return nil, fmt.Errorf("heartbeat token %s is readable by others (mode %o): chmod 0400", path, st.Mode().Perm())
	}
	data, err := os.ReadFile(path) // #nosec G304 -- operator-supplied secret path
	if err != nil {
		return nil, fmt.Errorf("heartbeat token: %w", err)
	}
	token := []byte(strings.TrimSpace(string(data)))
	if len(token) < minHeartbeatToken {
		return nil, fmt.Errorf("heartbeat token %s: want at least %d bytes, got %d", path, minHeartbeatToken, len(token))
	}
	return token, nil
}

// Gauge is the last-seen gauge, for the mailer's registry.
func (h *Heartbeats) Gauge() *metrics.GaugeVec { return h.last }

// Handler serves POST /heartbeat/{host} and nothing else.
func (h *Heartbeats) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /heartbeat/{host}", func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, 1024)
		got := sha256.Sum256([]byte(r.Header.Get(heartbeatHeader)))
		if subtle.ConstantTimeCompare(got[:], h.tokenSum[:]) != 1 {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		host := r.PathValue("host")
		if !h.hosts[host] {
			http.NotFound(w, r)
			return
		}
		h.last.WithLabels(metrics.Labels{"host": host}).Set(float64(h.now().Unix()))
		w.WriteHeader(http.StatusNoContent)
	})
	return mux
}
