// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package main

// SLOs as data (ADR-100 Phase 2, S-WELL-SPRING Phase 3). policy.yaml stays
// the reviewable source; unheaded_ops.slo_definitions (db/migrations/040)
// is what the dashboard and the compliance `slo` evidence read.
//
//	WELL_DB=unheaded_ops WELL_PASSWORD=... go run ./cmd/alert-policy -sync-well
//	WELL_DB=unheaded_ops WELL_PASSWORD=... go run ./cmd/alert-policy -check
//
// The Well is used only when WELL_DB is set (pkg/database.OpsWriterConfig:
// WELL_HOST, WELL_PORT, WELL_USER default ops_writer, WELL_PASSWORD,
// WELL_SSLMODE default require). With WELL_DB set, a Well that cannot be
// reached or read fails -check (fail closed); without it, -check is the
// YAML-only check CI runs.

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"time"

	"unheaded/pkg/database"
)

// promDuration parses a Prometheus-style duration with one unit
// (s, m, h, d, w), the form policy.yaml's window takes.
func promDuration(s string) (time.Duration, error) {
	if !durationRE.MatchString(s) {
		return 0, fmt.Errorf("duration %q: want a number and one of s m h d w", s)
	}
	n, err := strconv.ParseInt(s[:len(s)-1], 10, 64)
	unit := map[byte]time.Duration{'s': time.Second, 'm': time.Minute, 'h': time.Hour, 'd': 24 * time.Hour, 'w': 7 * 24 * time.Hour}[s[len(s)-1]]
	if err != nil || n <= 0 || n > int64(math.MaxInt64/unit) {
		return 0, fmt.Errorf("duration %q: out of range", s)
	}
	return time.Duration(n) * unit, nil
}

// sloRows renders the policy's SLOs as slo_definitions rows: the objective
// as a fraction, the SLI on one line, urgency page when the SLO pages and
// ticket otherwise, and the runbook URL its burn alerts carry.
func sloRows(p *Policy, sha string) ([]database.SLODefinition, error) {
	window, err := promDuration(p.Window)
	if err != nil {
		return nil, err
	}
	rows := make([]database.SLODefinition, 0, len(p.SLOs))
	for _, s := range p.SLOs {
		urgency := "ticket"
		if s.Page {
			urgency = "page"
		}
		book := runbookURL
		if s.Runbook != "" {
			book = repoBlob + s.Runbook
		}
		rows = append(rows, database.SLODefinition{
			Name: s.Name, Service: s.Service, Description: oneLine(s.Description),
			Objective: math.Round(s.Objective*100) / 1e4, Window: window, GoodExpr: oneLine(s.Good),
			Urgency: urgency, RunbookURL: book, SourceSHA256: sha,
		})
	}
	return rows, nil
}

// driftErrors compares the policy's rows with the Well's, field by field.
// source_sha256 and updated_at are provenance, not definition.
func driftErrors(yaml, well []database.SLODefinition) []error {
	inWell := map[string]database.SLODefinition{}
	for _, w := range well {
		inWell[w.Name] = w
	}
	inYAML := map[string]bool{}
	var errs []error
	for _, y := range yaml {
		inYAML[y.Name] = true
		w, ok := inWell[y.Name]
		if !ok {
			errs = append(errs, fmt.Errorf("drift: slo %s is in %s, not in the Well (run go run ./cmd/alert-policy -sync-well)", y.Name, policyPath))
			continue
		}
		d := func(field string, wv, yv any) {
			errs = append(errs, fmt.Errorf("drift: slo %s %s well=%v yaml=%v", y.Name, field, wv, yv))
		}
		if math.Round(w.Objective*1e4) != math.Round(y.Objective*1e4) {
			d("objective", num(w.Objective), num(y.Objective))
		}
		if w.Window != y.Window {
			d("window", w.Window, y.Window)
		}
		for _, f := range []struct{ name, w, y string }{
			{"service", w.Service, y.Service},
			{"description", w.Description, y.Description},
			{"sli_good_expr", w.GoodExpr, y.GoodExpr},
			{"sli_total_expr", w.TotalExpr, y.TotalExpr},
			{"urgency", w.Urgency, y.Urgency},
			{"runbook_url", w.RunbookURL, y.RunbookURL},
		} {
			if f.w != f.y {
				d(f.name, strconv.Quote(f.w), strconv.Quote(f.y))
			}
		}
	}
	var extra []string
	for name := range inWell {
		if !inYAML[name] {
			extra = append(extra, name)
		}
	}
	sort.Strings(extra)
	for _, name := range extra {
		errs = append(errs, fmt.Errorf("drift: slo %s is in the Well, not in %s (run go run ./cmd/alert-policy -sync-well)", name, policyPath))
	}
	return errs
}

// policyRows loads and validates the policy and renders its rows with the
// file's sha256.
func policyRows(root string) ([]database.SLODefinition, error) {
	path := filepath.Join(root, policyPath)
	data, err := os.ReadFile(path) // #nosec G304 -- repo file
	if err != nil {
		return nil, err
	}
	p, err := loadPolicy(path)
	if err != nil {
		return nil, err
	}
	if errs := p.validate(); len(errs) > 0 {
		return nil, fmt.Errorf("%s is invalid: %w", policyPath, errors.Join(errs...))
	}
	sum := sha256.Sum256(data)
	return sloRows(p, hex.EncodeToString(sum[:]))
}

// syncWell writes the policy's SLOs into slo_definitions.
func syncWell(ctx context.Context, db *sql.DB, root string) (database.SLOSyncResult, error) {
	rows, err := policyRows(root)
	if err != nil {
		return database.SLOSyncResult{}, err
	}
	return database.NewSLOStore(db).Sync(ctx, rows, "alert-policy -sync-well from "+policyPath+" sha256 "+rows[0].SourceSHA256[:12])
}

// checkWell returns one error per difference between the policy and the Well.
func checkWell(ctx context.Context, db *sql.DB, root string) []error {
	rows, err := policyRows(root)
	if err != nil {
		return []error{err}
	}
	well, err := database.NewSLOStore(db).All(ctx)
	if err != nil {
		return []error{err}
	}
	return driftErrors(rows, well)
}

var errNoWellDB = errors.New("WELL_DB is not set: -sync-well writes to the Well named by WELL_DB (unheaded_ops) as ops_writer")

// openWell connects as configured by the environment. Callers check
// WELL_DB first.
func openWell(ctx context.Context) (*sql.DB, error) {
	db, err := database.Connect(ctx, database.OpsWriterConfig())
	if err != nil {
		return nil, fmt.Errorf("WELL_DB is set but the Well is not usable: %w", err)
	}
	return db, nil
}

// runWellCheck is -check's Well leg: nil without WELL_DB (YAML only),
// otherwise the drift between the policy and slo_definitions, and any
// failure to read the Well as an error (fail closed).
func runWellCheck(ctx context.Context, root string) []error {
	if os.Getenv("WELL_DB") == "" {
		return nil
	}
	db, err := openWell(ctx)
	if err != nil {
		return []error{err}
	}
	defer db.Close()
	return checkWell(ctx, db, root)
}

// syncWellEnv is -sync-well: it needs WELL_DB.
func syncWellEnv(ctx context.Context, root string) (database.SLOSyncResult, error) {
	if os.Getenv("WELL_DB") == "" {
		return database.SLOSyncResult{}, errNoWellDB
	}
	db, err := openWell(ctx)
	if err != nil {
		return database.SLOSyncResult{}, err
	}
	defer db.Close()
	return syncWell(ctx, db, root)
}
