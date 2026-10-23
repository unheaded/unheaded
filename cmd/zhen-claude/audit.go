// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

package main

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"os"
	"regexp"
	"strings"
	"time"

	_ "github.com/lib/pq" // PostgreSQL driver for -audit-well

	"unheaded/pkg/audit/storage"
	"unheaded/pkg/claudebridge"
	wotanClient "unheaded/pkg/wotan-client"
)

// Environment for the audit sinks. Values never come from flags, so a DSN
// with its password is never on a command line.
const (
	envWellOpsDSN = "WELL_OPS_DSN"
	envWotanAddr  = "WOTAN_ADDR"
)

// readOnlyChampionTools are the Champion tools offered when the Well
// cannot record what happens (ADR-019: no actions without an audit trail).
var readOnlyChampionTools = []string{"read_file", "kanban_list"}

// auditSetup is the auditor chain for one run and whether the run must
// stay read-only.
type auditSetup struct {
	auditor  claudebridge.Auditor
	readOnly bool
	well     *claudebridge.WellAuditor
	wotan    *claudebridge.WotanAuditor
	closers  []func()
}

// close drains the async sinks (bounded) and releases connections.
func (a *auditSetup) close() {
	if a == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if a.well != nil {
		_ = a.well.Close(ctx)
	}
	if a.wotan != nil {
		_ = a.wotan.Close(ctx)
	}
	for i := len(a.closers) - 1; i >= 0; i-- {
		a.closers[i]()
	}
}

// checkAuditEnv is the usage check for -audit-well / -audit-wotan: the
// flag without its environment variable is a mistake to report, not a
// silent fallback to no audit.
func checkAuditEnv(o *options) error {
	if o.auditWell && strings.TrimSpace(os.Getenv(envWellOpsDSN)) == "" {
		return fmt.Errorf("-audit-well needs %s (the unheaded_ops DSN for ops_writer); it is read from the environment only", envWellOpsDSN)
	}
	if o.auditWotan && strings.TrimSpace(os.Getenv(envWotanAddr)) == "" {
		return fmt.Errorf("-audit-wotan needs %s (Wotan's HTTP host:port)", envWotanAddr)
	}
	return nil
}

// openWellDB opens and pings the Well. A variable so tests can stub it.
var openWellDB = func(ctx context.Context, dsn string) (*sql.DB, error) {
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(2) // one async writer; the Well's connection budget is small
	pctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := db.PingContext(pctx); err != nil {
		_ = db.Close()
		return nil, err
	}
	return db, nil
}

// setupAudit builds the auditor: slog always, plus the Well and Wotan
// sinks when asked. A Well that cannot be reached (or lacks the 022/030
// tables) puts the run in read-only mode. source is the service name
// (zhen-claude, zhen-eval).
func setupAudit(ctx context.Context, o *options, log *slog.Logger, source string) *auditSetup {
	a := &auditSetup{}
	chain := claudebridge.MultiAuditor{claudebridge.SlogAuditor{Log: log}}
	if o.auditWell {
		w, err := wellAuditor(ctx, log, source, a)
		if err != nil {
			a.readOnly = true
			// err never holds the DSN: lib/pq's errors name host and cause only.
			log.Warn("read_only", "reason", "the Well is unreachable, so there is no audit trail; mutating tools are not offered (ADR-019)",
				"error", redactDSN(err.Error()))
		} else {
			a.well = w
			chain = append(chain, w)
		}
	}
	if o.auditWotan {
		c, err := wotanClient.NewClient(os.Getenv(envWotanAddr))
		if err == nil {
			a.closers = append(a.closers, func() { _ = c.Close() })
			wa, werr := claudebridge.NewWotanAuditor(claudebridge.WotanAuditorConfig{
				Pub: claudebridge.WotanClientPublisher{C: c}, Name: source, Backend: claudebridge.BackendClaude, Log: log,
			})
			err = werr
			if werr == nil {
				a.wotan = wa
				chain = append(chain, wa)
			}
		}
		if err != nil {
			log.Warn("audit_wotan_off", "error", err.Error())
		}
	}
	a.auditor = chain
	return a
}

func wellAuditor(ctx context.Context, log *slog.Logger, source string, a *auditSetup) (*claudebridge.WellAuditor, error) {
	db, err := openWellDB(ctx, os.Getenv(envWellOpsDSN))
	if err != nil {
		return nil, fmt.Errorf("connect: %w", err)
	}
	a.closers = append(a.closers, func() { _ = db.Close() })
	events, err := storage.NewDatabaseStorage(db, "")
	if err != nil {
		return nil, err
	}
	if _, err := db.ExecContext(ctx, "SELECT 1 FROM llm_requests LIMIT 0"); err != nil {
		return nil, fmt.Errorf("llm_requests missing (db/migrations/030 not applied?): %w", err)
	}
	return claudebridge.NewWellAuditor(claudebridge.WellAuditorConfig{
		Source: source, Backend: claudebridge.BackendClaude,
		Requests: claudebridge.SQLRequestSink{DB: db}, Events: events, Log: log,
	})
}

// dsnUserInfoRE matches the user:password@ part of a URL.
var dsnUserInfoRE = regexp.MustCompile(`([A-Za-z][A-Za-z0-9+.-]*://)[^@\s/]+@`)

// redactDSN removes anything that looks like user:password@ from s.
func redactDSN(s string) string { return dsnUserInfoRE.ReplaceAllString(s, "${1}[redacted]@") }
