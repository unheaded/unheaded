// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

// alert-mailer is the only thing in the Kingdom that sends alert mail
// (ADR-100). Alertmanager posts page and ticket alerts here; the mailer
//
//   - mails pages at once, all pages firing together in one mail,
//   - collects tickets and mails them as one digest a day,
//   - never sends more than mailer.max_per_day mails a calendar day, nor
//     more than mailer.max_per_hour in one clock hour, and
//   - in mailer.mode dry-run sends nothing, and records what it would have.
//
// Settings come from the mailer: block of observability/alerting/policy.yaml;
// the relay from the environment (SMTP_SMARTHOST, SMTP_USERNAME,
// SMTP_PASSWORD_FILE, ALERT_MAIL_FROM, ALERT_MAIL_TO, SMTP_STARTTLS).
//
//	GET  /v1/status   cap, counts, open tickets
//	GET  /v1/outbox   the last 50 mails: sent, dry-run, capped or failed
//	POST /v1/alerts/page, /v1/alerts/ticket   Alertmanager webhooks
//	POST /v1/alerts/pipeline   the pipeline receipt (AlertPipelineHeartbeat
//	                           every 5 minutes); none for pipeline.silent_after
//	                           mails "alert pipeline silent" once
//
// With HEARTBEAT_HOSTS set it also runs the dead-man's switch on its own
// listener (-heartbeat-addr, heartbeat.go):
//
//	POST /heartbeat/{host}   X-Heartbeat-Token: <HEARTBEAT_TOKEN_FILE>
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
	"strings"
	"syscall"
	"time"
	_ "time/tzdata" // the timezone setting must work in a scratch image

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"gopkg.in/yaml.v3"

	"unheaded/pkg/cmdline"
	"unheaded/pkg/httputil"
	"unheaded/pkg/metrics"
	"unheaded/pkg/ports"
)

const maxWebhookBytes = 1 << 20

// loadSettings reads the mailer: block of the alerting policy.
func loadSettings(path string) (Settings, error) {
	data, err := os.ReadFile(path) // #nosec G304 -- operator-supplied policy path
	if err != nil {
		return Settings{}, err
	}
	var doc struct {
		Mailer struct {
			Mode       string      `yaml:"mode"`
			MaxPerDay  int         `yaml:"max_per_day"`
			MaxPerHour int         `yaml:"max_per_hour"`
			DigestAt   string      `yaml:"digest_at"`
			Timezone   string      `yaml:"timezone"`
			RunDays    *runDaysDoc `yaml:"run_days"`
		} `yaml:"mailer"`
		Page struct {
			RepeatInterval string `yaml:"repeat_interval"`
		} `yaml:"page"`
		Pipeline struct {
			SilentAfter string `yaml:"silent_after"`
		} `yaml:"pipeline"`
	}
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return Settings{}, fmt.Errorf("%s: %w", path, err)
	}
	p := doc.Mailer
	s := Settings{Mode: p.Mode, MaxPerDay: p.MaxPerDay, MaxPerHour: p.MaxPerHour}
	if s.Mode != "dry-run" && s.Mode != "send" {
		return s, fmt.Errorf("mailer.mode %q: want dry-run or send", s.Mode)
	}
	if s.MaxPerDay < 1 || s.MaxPerDay > 50 {
		return s, fmt.Errorf("mailer.max_per_day %d: want 1..50", s.MaxPerDay)
	}
	if s.MaxPerHour < 1 || s.MaxPerHour > s.MaxPerDay {
		return s, fmt.Errorf("mailer.max_per_hour %d: want 1..max_per_day", s.MaxPerHour)
	}
	if err := loadRunDays(&s, p.RunDays); err != nil {
		return s, err
	}
	t, err := time.Parse("15:04", p.DigestAt)
	if err != nil {
		return s, fmt.Errorf("mailer.digest_at %q: want HH:MM", p.DigestAt)
	}
	s.DigestH, s.DigestM = t.Hour(), t.Minute()
	if s.Reminder, err = parseDuration(doc.Page.RepeatInterval); err != nil || s.Reminder < time.Hour {
		return s, fmt.Errorf("page.repeat_interval %q: want at least 1h", doc.Page.RepeatInterval)
	}
	if s.Loc, err = time.LoadLocation(p.Timezone); err != nil || p.Timezone == "" {
		return s, fmt.Errorf("mailer.timezone %q: not an IANA zone", p.Timezone)
	}
	// pipeline.silent_after: absent turns the pipeline mail off; present, at
	// least 10m (two of Alertmanager's 5-minute repeats can be missed).
	if v := doc.Pipeline.SilentAfter; v != "" {
		if s.PipelineSilentAfter, err = parseDuration(v); err != nil || s.PipelineSilentAfter < 10*time.Minute {
			return s, fmt.Errorf("pipeline.silent_after %q: want a duration of at least 10m", v)
		}
	}
	return s, nil
}

// runDaysDoc is mailer.run_days in the policy.
type runDaysDoc struct {
	MaxPerDay  int      `yaml:"max_per_day"`
	MaxPerHour int      `yaml:"max_per_hour"`
	Dates      []string `yaml:"dates"`
}

// loadRunDays checks mailer.run_days (ADR-147 decision 2) into s. With no
// block the run-day caps equal the normal ones and no date is listed. The
// same rules are in cmd/alert-policy's validate.
func loadRunDays(s *Settings, rd *runDaysDoc) error {
	s.RunDayMaxPerDay, s.RunDayMaxPerHour, s.RunDates = s.MaxPerDay, s.MaxPerHour, map[string]bool{}
	if rd == nil {
		return nil
	}
	if rd.MaxPerDay < 1 || rd.MaxPerDay > 50 {
		return fmt.Errorf("mailer.run_days.max_per_day %d: want 1..50", rd.MaxPerDay)
	}
	if rd.MaxPerDay < s.MaxPerDay {
		return fmt.Errorf("mailer.run_days.max_per_day %d: want at least mailer.max_per_day (%d)", rd.MaxPerDay, s.MaxPerDay)
	}
	if rd.MaxPerHour < 1 || rd.MaxPerHour > rd.MaxPerDay {
		return fmt.Errorf("mailer.run_days.max_per_hour %d: want 1..run_days.max_per_day", rd.MaxPerHour)
	}
	if rd.MaxPerHour < s.MaxPerHour {
		return fmt.Errorf("mailer.run_days.max_per_hour %d: want at least mailer.max_per_hour (%d)", rd.MaxPerHour, s.MaxPerHour)
	}
	for _, d := range rd.Dates {
		if _, err := time.Parse("2006-01-02", d); err != nil {
			return fmt.Errorf("mailer.run_days.dates %q: want YYYY-MM-DD", d)
		}
		s.RunDates[d] = true
	}
	s.RunDayMaxPerDay, s.RunDayMaxPerHour = rd.MaxPerDay, rd.MaxPerHour
	return nil
}

// parseDuration accepts Go durations and Prometheus-style days ("2d").
func parseDuration(v string) (time.Duration, error) {
	if n, ok := strings.CutSuffix(v, "d"); ok {
		d, err := strconv.Atoi(n)
		return time.Duration(d) * 24 * time.Hour, err
	}
	return time.ParseDuration(v)
}

func main() {
	addr := flag.String("addr", ":"+strconv.Itoa(ports.AlertMailer), "listen address")
	policy := flag.String("policy", "/etc/alert-mailer/policy.yaml", "observability/alerting/policy.yaml")
	stateDir := flag.String("state-dir", "/var/lib/alert-mailer", "where state.json lives (cap, tickets, outbox)")
	hbAddr := flag.String("heartbeat-addr", ":"+strconv.Itoa(ports.AlertMailerHeartbeat), "heartbeat listen address (used only when HEARTBEAT_HOSTS is set)")
	cmdline.Parse(cmdline.Spec{
		Name:     "alert-mailer",
		Purpose:  "Send alert mail for Alertmanager page and ticket routes; the only mailer in the Kingdom (ADR-100).",
		Examples: []string{"alert-mailer --policy observability/alerting/policy.yaml"},
		Details:  "Relay settings come from the environment: SMTP_SMARTHOST, SMTP_USERNAME, SMTP_PASSWORD.",
	})
	zerolog.TimeFieldFormat = zerolog.TimeFormatUnixMs
	log.Logger = log.With().Str("service", "alert-mailer").Logger()

	cfg, err := loadSettings(*policy)
	if err != nil {
		log.Fatal().Err(err).Msg("alert policy")
	}
	var sender Sender
	smtpCfg, smtpErr := smtpFromEnv(os.Getenv)
	switch {
	case cfg.Mode == "send" && smtpErr != nil:
		log.Fatal().Err(smtpErr).Msg("mode send needs the relay settings")
	case smtpErr == nil:
		sender = smtpCfg
	}
	m, err := NewMailer(cfg, sender, *stateDir, time.Now)
	if err != nil {
		log.Fatal().Err(err).Msg("state")
	}
	hb, err := heartbeatFromEnv(os.Getenv, time.Now)
	if err != nil {
		log.Fatal().Err(err).Msg("heartbeat")
	}

	reg := metrics.NewRegistry()
	msgs := metrics.NewCounterVec("unheaded_alert_mailer_messages_total",
		"Mails by urgency and outcome: sent, dry-run, capped (held back by max_per_day), capped-hour (by max_per_hour), failed (relay refused; Alertmanager retries)",
		nil, []string{"urgency", "outcome"})
	reg.MustRegister(msgs)
	m.count = func(u, o string) { msgs.WithLabels(metrics.Labels{"urgency": u, "outcome": o}).Inc() }
	reg.MustRegister(metrics.NewFuncGauge("unheaded_alert_mailer_sent_today", "Mails counted against today's cap", nil,
		func() float64 { return float64(m.Status(false).SentToday) }))
	reg.MustRegister(metrics.NewFuncGauge("unheaded_alert_mailer_max_per_day", "Today's daily cap (mailer.max_per_day, or mailer.run_days.max_per_day on a listed run day)", nil,
		func() float64 { return float64(m.Status(false).MaxPerDay) }))
	reg.MustRegister(metrics.NewFuncGauge("unheaded_alert_mailer_open_tickets", "Tickets waiting for the next digest", nil,
		func() float64 { return float64(m.Status(false).OpenTickets) }))
	reg.MustRegister(metrics.NewFuncGauge("unheaded_alert_mailer_pipeline_last_seen_timestamp_seconds",
		"Unix time of the last AlertPipelineHeartbeat Alertmanager handed over (0: none since start of state)", nil,
		func() float64 {
			if t := m.PipelineLast(); !t.IsZero() {
				return float64(t.UnixNano()) / 1e9
			}
			return 0
		}))
	if hb != nil {
		reg.MustRegister(hb.Gauge())
	}

	srv := &http.Server{
		Addr:              *addr,
		Handler:           handler(m, reg),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       120 * time.Second,
		MaxHeaderBytes:    1 << 20,
	}
	go func() {
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatal().Err(err).Msg("HTTP listen failed")
		}
	}()
	var hbSrv *http.Server
	if hb != nil {
		hbSrv = &http.Server{
			Addr:              *hbAddr,
			Handler:           hb.Handler(),
			ReadHeaderTimeout: 5 * time.Second,
			ReadTimeout:       10 * time.Second,
			WriteTimeout:      10 * time.Second,
			IdleTimeout:       60 * time.Second,
			MaxHeaderBytes:    8 << 10,
		}
		go func() {
			if err := hbSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
				log.Fatal().Err(err).Msg("heartbeat listen failed")
			}
		}()
		log.Info().Str("addr", *hbAddr).Str("hosts", os.Getenv("HEARTBEAT_HOSTS")).Msg("heartbeat listener started")
	}
	log.Info().Str("addr", *addr).Str("mode", cfg.Mode).Int("max_per_day", cfg.MaxPerDay).
		Bool("relay_configured", sender != nil).Msg("alert-mailer started")

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	t := time.NewTicker(time.Minute)
	defer t.Stop()
	for {
		if err := m.Tick(); err != nil {
			log.Error().Err(err).Msg("digest not sent; retrying next minute")
		}
		select {
		case <-ctx.Done():
			sctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			_ = srv.Shutdown(sctx)
			if hbSrv != nil {
				_ = hbSrv.Shutdown(sctx)
			}
			cancel()
			return
		case <-t.C:
		}
	}
}

func handler(m *Mailer, reg *metrics.Registry) http.Handler {
	mux := http.NewServeMux()
	ok := httputil.ProbeMethods(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"status":"ok","service":"alert-mailer"}`)
	})
	mux.HandleFunc("/health", ok)
	mux.HandleFunc("/ready", ok)
	mux.Handle("/metrics", metrics.HandlerFor(reg, metrics.DefaultRegistry))
	mux.HandleFunc("GET /v1/status", func(w http.ResponseWriter, _ *http.Request) {
		httputil.WriteJSON(w, http.StatusOK, m.Status(false))
	})
	mux.HandleFunc("GET /v1/outbox", func(w http.ResponseWriter, _ *http.Request) {
		httputil.WriteJSON(w, http.StatusOK, m.Status(true).Outbox)
	})
	hook := func(f func(Webhook) error) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			var wh Webhook
			if err := httputil.DecodeJSONBody(r, maxWebhookBytes, &wh); err != nil {
				httputil.HandleRequestError(w, err)
				return
			}
			if err := f(wh); err != nil {
				// Not counted against the cap; Alertmanager retries.
				log.Error().Err(err).Str("path", r.URL.Path).Msg("mail not sent")
				httputil.WriteError(w, http.StatusServiceUnavailable, "send_failed", "relay refused the mail; retry")
				return
			}
			w.WriteHeader(http.StatusNoContent)
		}
	}
	mux.HandleFunc("POST /v1/alerts/page", hook(m.Page))
	mux.HandleFunc("POST /v1/alerts/ticket", hook(m.Ticket))
	// The pipeline receipt: the body is read through the same limit and only
	// its arrival counts (pipeline.silent_after, ADR-116 R4).
	mux.HandleFunc("POST /v1/alerts/pipeline", hook(func(Webhook) error { return m.PipelineSeen() }))
	return httputil.SecurityHeaders(httputil.NewServiceMetrics("alert-mailer").Instrument(mux, mux))
}
