// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package database

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/lib/pq"
)

// SLODefinition is one row of unheaded_ops.slo_definitions
// (db/migrations/040). observability/alerting/policy.yaml is the reviewable
// source; cmd/alert-policy -sync-well writes these rows from it.
type SLODefinition struct {
	Name        string
	Service     string
	Description string
	// Objective is a fraction, 0 < Objective < 1, at most four decimals
	// (the column is numeric(6,4)).
	Objective float64
	// Window is the error-budget window (slo_window), whole seconds.
	Window   time.Duration
	GoodExpr string
	// TotalExpr is empty for a time-based SLI (stored as NULL).
	TotalExpr    string
	Urgency      string // page, ticket or none
	RunbookURL   string
	SourceSHA256 string
	UpdatedAt    time.Time // set by the database; ignored by Sync
}

// SLOSyncResult names the SLOs a Sync inserted, updated, deleted and left
// alone, each sorted.
type SLOSyncResult struct {
	Inserted  []string
	Updated   []string
	Deleted   []string
	Unchanged []string
}

// SLOStore reads and writes slo_definitions. Writes need ops_writer.
type SLOStore struct {
	db *sql.DB
}

// NewSLOStore returns a store on db.
func NewSLOStore(db *sql.DB) *SLOStore { return &SLOStore{db: db} }

var (
	sloNameRE   = regexp.MustCompile(`^[a-z][a-z0-9-]{0,39}$`)
	sloSHA256RE = regexp.MustCompile(`^[0-9a-f]{64}$`)
)

// The migration's CHECKs, checked first so a bad definition is refused
// before the transaction starts, with a message naming the SLO.
func (d SLODefinition) validate() error {
	bad := func(f string, a ...any) error {
		return fmt.Errorf("slo %q: %s", d.Name, fmt.Sprintf(f, a...))
	}
	switch {
	case !sloNameRE.MatchString(d.Name):
		return bad("name must match %s", sloNameRE)
	case d.Service == "" || len(d.Service) > 64:
		return bad("service must be 1-64 bytes")
	case d.Description == "" || len(d.Description) > 512:
		return bad("description must be 1-512 bytes")
	case !(d.Objective > 0 && d.Objective < 1):
		return bad("objective %v: want 0 < objective < 1", d.Objective)
	case math.Abs(d.Objective*1e4-math.Round(d.Objective*1e4)) > 1e-6:
		return bad("objective %v: at most four decimals (numeric(6,4))", d.Objective)
	case strings.TrimSpace(d.GoodExpr) == "" || len(d.GoodExpr) > 4096:
		return bad("sli_good_expr must be 1-4096 bytes")
	case len(d.TotalExpr) > 4096:
		return bad("sli_total_expr longer than 4096 bytes")
	case d.Urgency != "page" && d.Urgency != "ticket" && d.Urgency != "none":
		return bad("urgency %q: want page, ticket or none", d.Urgency)
	case d.RunbookURL == "" || len(d.RunbookURL) > 512:
		return bad("runbook_url must be 1-512 bytes")
	case !sloSHA256RE.MatchString(d.SourceSHA256):
		return bad("source_sha256 must be 64 lowercase hex")
	}
	if _, err := intervalText(d.Window); err != nil {
		return bad("window: %v", err)
	}
	return nil
}

// intervalText renders a whole-second duration as PostgreSQL interval
// input in its largest exact unit ("28 days"), so the row reads the way
// the policy does.
func intervalText(d time.Duration) (string, error) {
	if d <= 0 || d%time.Second != 0 {
		return "", fmt.Errorf("%v: want a positive whole number of seconds", d)
	}
	for _, u := range []struct {
		size time.Duration
		name string
	}{{24 * time.Hour, "days"}, {time.Hour, "hours"}, {time.Minute, "minutes"}} {
		if d%u.size == 0 {
			return fmt.Sprintf("%d %s", d/u.size, u.name), nil
		}
	}
	return fmt.Sprintf("%d seconds", d/time.Second), nil
}

// All returns every row, ordered by name.
func (s *SLOStore) All(ctx context.Context) ([]SLODefinition, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT name, service, description, objective::float8,
		extract(epoch FROM slo_window)::bigint, sli_good_expr, coalesce(sli_total_expr, ''),
		urgency, runbook_url, source_sha256, updated_at
		FROM slo_definitions ORDER BY name`)
	if err != nil {
		return nil, fmt.Errorf("read slo_definitions: %w", err)
	}
	defer rows.Close()
	var out []SLODefinition
	for rows.Next() {
		var d SLODefinition
		var secs int64
		if err := rows.Scan(&d.Name, &d.Service, &d.Description, &d.Objective, &secs, &d.GoodExpr, &d.TotalExpr,
			&d.Urgency, &d.RunbookURL, &d.SourceSHA256, &d.UpdatedAt); err != nil {
			return nil, fmt.Errorf("read slo_definitions: %w", err)
		}
		d.Window = time.Duration(secs) * time.Second
		out = append(out, d)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read slo_definitions: %w", err)
	}
	return out, nil
}

// upsertSLO changes a row only when its definition differs: a policy edit
// elsewhere in the file (a new sha256, same SLO) writes nothing and no
// history, so source_sha256 names the policy version that last changed the
// definition. RETURNING yields no row for an unchanged one.
const upsertSLO = `INSERT INTO slo_definitions AS t (name, service, description, objective, slo_window,
	sli_good_expr, sli_total_expr, urgency, runbook_url, source_sha256)
VALUES ($1, $2, $3, $4, $5::interval, $6, NULLIF($7, ''), $8, $9, $10)
ON CONFLICT (name) DO UPDATE SET service = EXCLUDED.service, description = EXCLUDED.description,
	objective = EXCLUDED.objective, slo_window = EXCLUDED.slo_window, sli_good_expr = EXCLUDED.sli_good_expr,
	sli_total_expr = EXCLUDED.sli_total_expr, urgency = EXCLUDED.urgency, runbook_url = EXCLUDED.runbook_url,
	source_sha256 = EXCLUDED.source_sha256, updated_at = now()
WHERE (t.service, t.description, t.objective, t.slo_window, t.sli_good_expr, t.sli_total_expr, t.urgency, t.runbook_url)
	IS DISTINCT FROM (EXCLUDED.service, EXCLUDED.description, EXCLUDED.objective, EXCLUDED.slo_window,
	EXCLUDED.sli_good_expr, EXCLUDED.sli_total_expr, EXCLUDED.urgency, EXCLUDED.runbook_url)
RETURNING (xmax = 0)`

// Sync makes the table hold exactly defs, in one transaction: each
// definition is inserted or updated, and every row whose name is not in
// defs is deleted. reason goes to every history row (well.change_reason).
// An empty defs is refused: it would delete every SLO.
func (s *SLOStore) Sync(ctx context.Context, defs []SLODefinition, reason string) (SLOSyncResult, error) {
	var res SLOSyncResult
	if len(defs) == 0 {
		return res, errors.New("slo sync: no definitions (refusing to delete every SLO)")
	}
	names := make([]string, 0, len(defs))
	seen := map[string]bool{}
	for _, d := range defs {
		if err := d.validate(); err != nil {
			return res, err
		}
		if seen[d.Name] {
			return res, fmt.Errorf("slo %q: duplicate name", d.Name)
		}
		seen[d.Name] = true
		names = append(names, d.Name)
	}
	if len(reason) > 512 {
		reason = reason[:512]
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return res, fmt.Errorf("slo sync: %w", err)
	}
	defer func() { _ = tx.Rollback() }() // no-op after Commit
	if _, err := tx.ExecContext(ctx, `SELECT set_config('well.change_reason', $1, true)`, reason); err != nil {
		return res, fmt.Errorf("slo sync: change reason: %w", err)
	}
	for _, d := range defs {
		iv, _ := intervalText(d.Window) // validated above
		var inserted bool
		err := tx.QueryRowContext(ctx, upsertSLO, d.Name, d.Service, d.Description,
			fmt.Sprintf("%.4f", d.Objective), iv, d.GoodExpr, d.TotalExpr, d.Urgency, d.RunbookURL, d.SourceSHA256).Scan(&inserted)
		switch {
		case errors.Is(err, sql.ErrNoRows):
			res.Unchanged = append(res.Unchanged, d.Name)
		case err != nil:
			return SLOSyncResult{}, fmt.Errorf("slo sync: %s: %w", d.Name, err)
		case inserted:
			res.Inserted = append(res.Inserted, d.Name)
		default:
			res.Updated = append(res.Updated, d.Name)
		}
	}
	rows, err := tx.QueryContext(ctx, `DELETE FROM slo_definitions WHERE NOT (name = ANY($1)) RETURNING name`, pq.Array(names))
	if err != nil {
		return SLOSyncResult{}, fmt.Errorf("slo sync: delete: %w", err)
	}
	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err != nil {
			_ = rows.Close()
			return SLOSyncResult{}, fmt.Errorf("slo sync: delete: %w", err)
		}
		res.Deleted = append(res.Deleted, n)
	}
	if err := errors.Join(rows.Err(), rows.Close()); err != nil {
		return SLOSyncResult{}, fmt.Errorf("slo sync: delete: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return SLOSyncResult{}, fmt.Errorf("slo sync: commit: %w", err)
	}
	for _, l := range [][]string{res.Inserted, res.Updated, res.Deleted, res.Unchanged} {
		sort.Strings(l)
	}
	return res, nil
}
