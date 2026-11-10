// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

// logstore-writer puts a second copy of the host journal rows into the
// Well, table unheaded_ops.log_events, for the measured week of S-E1-LIVE
// Evening 16 (ADR-117; db/migrations/048_ops_log_events.sql). ClickHouse
// stays the record. Vector posts the rows it also sends there, after the
// same redaction:
//
//	POST /ingest    newline-delimited JSON, Authorization: Bearer <token>
//	GET  /health, /ready, /metrics
//
// The Well is the system of record for everything else, so the writer
// holds the flood back: a row cap per clock minute (429), a byte ceiling
// on the table (503), two batches at a time, and a pool of three
// connections as a role that can only INSERT. Vector's sink for this
// writer drops what it cannot deliver; nothing here is the audit trail.
//
// The login comes from the environment (WELL_HOST, WELL_PORT, WELL_USER,
// WELL_PASSWORD, WELL_DB, WELL_SSLMODE, WELL_SSLROOTCERT), and so does the
// ingest token (LOGSTORE_INGEST_TOKEN, 32 bytes or more). Neither has a
// default.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"sync"
	"syscall"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

	"unheaded/pkg/cmdline"
	"unheaded/pkg/database"
	"unheaded/pkg/httputil"
	"unheaded/pkg/metrics"
	"unheaded/pkg/ports"
	utls "unheaded/pkg/tls"
)

const (
	service         = "logstore-writer"
	maintainEvery   = 5 * time.Minute
	maintainTimeout = 20 * time.Second
)

func main() {
	addr := flag.String("addr", ":"+strconv.Itoa(ports.LogstoreWriter), "listen address")
	maxRows := flag.Int("max-rows-per-minute", 6000, "rows accepted in one clock minute; past it /ingest answers 429")
	maxBytes := flag.Int64("max-bytes", 4<<30, "ceiling on what log_events may hold, in bytes; over it /ingest answers 503")
	cmdline.Parse(cmdline.Spec{
		Name:     service,
		Purpose:  "Insert the journal rows Vector posts into the Well's log_events table, capped (S-E1-LIVE Evening 16).",
		Examples: []string{"logstore-writer --max-rows-per-minute 6000 --max-bytes 4294967296"},
		Details:  "The Well login comes from WELL_HOST, WELL_USER, WELL_PASSWORD, WELL_DB, WELL_SSLMODE, WELL_SSLROOTCERT; the ingest token from LOGSTORE_INGEST_TOKEN.",
	})
	zerolog.TimeFieldFormat = zerolog.TimeFormatUnixMs
	log.Logger = log.With().Str("service", service).Logger()

	if *maxRows < maxRowsPerPost {
		// A batch larger than the cap could never be accepted.
		log.Fatal().Int("max_rows_per_minute", *maxRows).Int("least", maxRowsPerPost).Msg("--max-rows-per-minute is below one batch")
	}
	reg := metrics.NewRegistry()
	in, err := NewIngest(nil, os.Getenv("LOGSTORE_INGEST_TOKEN"), *maxRows, *maxBytes, time.Now, counters(reg))
	if err != nil {
		log.Fatal().Err(err).Msg("ingest")
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	db, err := database.Connect(ctx, database.LogWriterConfig())
	if err != nil {
		log.Fatal().Err(err).Msg("the Well")
	}
	defer db.Close() // #nosec G104 -- shutdown path
	// log_writer's connection limit is 4: two batches, one maintenance call.
	db.SetMaxOpenConns(3)
	db.SetMaxIdleConns(3)
	in.store = wellStore{db: db}

	maintainFailures := metrics.NewCounter("unheaded_logstore_maintain_failures_total",
		"Calls of log_events_maintain() that failed (behind a backup the lock times out; the next call catches up)", nil)
	reg.MustRegister(maintainFailures)
	reg.MustRegister(metrics.NewFuncGauge("unheaded_logstore_stored_bytes", "Bytes log_events held at the last maintenance call", nil,
		func() float64 { return float64(in.StoredBytes()) }))
	reg.MustRegister(metrics.NewFuncGauge("unheaded_logstore_max_bytes", "The byte ceiling (--max-bytes)", nil,
		func() float64 { return float64(*maxBytes) }))
	reg.MustRegister(metrics.NewFuncGauge("unheaded_logstore_last_maintain_timestamp_seconds", "Unix time of the last good maintenance call; 0 is never", nil,
		func() float64 { return float64(in.LastMaintained()) }))

	srv := &http.Server{
		Addr:              *addr,
		Handler:           handler(in, reg),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       120 * time.Second,
		MaxHeaderBytes:    16 << 10,
	}
	// TLS from UNHEADED_TLS_* (S-IRON-GATE Phase 8): misconfigured TLS is an
	// error, never a plaintext listener. The token crosses this link.
	tlsOn, err := utls.ConfigureServer(srv)
	if err != nil {
		log.Fatal().Err(err).Msg("TLS")
	}
	go func() {
		if err := utls.ListenAndServe(srv); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatal().Err(err).Msg("HTTP listen failed")
		}
	}()
	log.Info().Str("addr", *addr).Bool("tls", tlsOn).Int("max_rows_per_minute", *maxRows).Int64("max_bytes", *maxBytes).Msg("logstore-writer started")

	t := time.NewTicker(maintainEvery)
	defer t.Stop()
	wasFull := false
	for {
		mctx, cancel := context.WithTimeout(ctx, maintainTimeout)
		err := in.RunMaintenance(mctx)
		cancel()
		switch {
		case err != nil && ctx.Err() == nil:
			maintainFailures.Inc()
			log.Warn().Err(err).Msg("log_events_maintain failed; trying again in five minutes")
		case err == nil && in.Full() != wasFull:
			wasFull = in.Full()
			log.Warn().Bool("full", wasFull).Int64("stored_bytes", in.StoredBytes()).Int64("max_bytes", *maxBytes).
				Msg("log_events crossed its byte ceiling")
		}
		select {
		case <-ctx.Done():
			sctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			_ = srv.Shutdown(sctx)
			cancel()
			return
		case <-t.C:
		}
	}
}

// counters registers the writer's counters and returns them as the
// functions Ingest calls. A refused batch is logged at most once a minute
// for each reason, and never with its content: these lines come back
// through Vector as rows.
func counters(reg *metrics.Registry) Counters {
	rows := metrics.NewCounterVec("unheaded_logstore_rows_total",
		"Rows by outcome: inserted, duplicate (already there), rejected (can never be stored)", nil, []string{"outcome"})
	rejected := metrics.NewCounterVec("unheaded_logstore_rejected_rows_total",
		"Rejected rows by reason: bad_json, bad_timestamp, out_of_window, bad_value, too_long", nil, []string{"reason"})
	refused := metrics.NewCounterVec("unheaded_logstore_refused_requests_total",
		"Batches refused whole by reason: unauthorized, bad_request, too_large, busy, not_ready, full, rate, store", nil, []string{"reason"})
	insert := metrics.NewHistogram(metrics.HistogramOpts{
		Name: "unheaded_logstore_insert_duration_seconds", Help: "Time one batch INSERT took", Buckets: metrics.DefaultBuckets,
	})
	reg.MustRegister(rows)
	reg.MustRegister(rejected)
	reg.MustRegister(refused)
	reg.MustRegister(insert)
	var mu sync.Mutex
	lastLogged := map[string]time.Time{}
	return Counters{
		Rows:     func(o string, n int) { rows.WithLabels(metrics.Labels{"outcome": o}).Add(float64(n)) },
		Rejected: func(r string, n int) { rejected.WithLabels(metrics.Labels{"reason": r}).Add(float64(n)) },
		Refused: func(r string) {
			refused.WithLabels(metrics.Labels{"reason": r}).Inc()
			mu.Lock()
			quiet := time.Since(lastLogged[r]) < time.Minute
			if !quiet {
				lastLogged[r] = time.Now()
			}
			mu.Unlock()
			if !quiet {
				log.Warn().Str("reason", r).Msg("batch refused (logged at most once a minute for each reason)")
			}
		},
		Insert: insert.Observe,
	}
}

func handler(in *Ingest, reg *metrics.Registry) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/health", httputil.ProbeMethods(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{"status":"ok","service":%q}`, service)
	}))
	// Ready once the table has partitions. A full table is still ready: it
	// refuses rows on purpose, and a restart would not help.
	mux.HandleFunc("/ready", httputil.ProbeMethods(func(w http.ResponseWriter, _ *http.Request) {
		if !in.Ready() {
			httputil.WriteError(w, http.StatusServiceUnavailable, "not_ready", "log_events_maintain() has not succeeded yet")
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{"status":"ok","service":%q}`, service)
	}))
	mux.Handle("/metrics", metrics.HandlerFor(reg, metrics.DefaultRegistry))
	mux.Handle("POST /ingest", in)
	return httputil.SecurityHeaders(httputil.NewServiceMetrics(service).Instrument(mux, mux))
}
