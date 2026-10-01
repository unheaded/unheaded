// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package well

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sort"

	"github.com/lib/pq"

	"unheaded/pkg/compliance/crosswalk"
	"unheaded/pkg/compliance/remediation"
)

// Sync brings The Well in line with the repository (COMPLIANCE-WELL step
// 46), in two different ways:
//
//   - Definitions the repo authors (fixes, controls, frameworks) are
//     mirrored: inserted, or updated when their sha256 differs.
//   - State The Well owns (profiles, host bindings, findings triage) is
//     only seeded: inserted when absent, never overwritten. A row that
//     differs from its YAML is reported as diverged and left alone.
//
// Nothing is ever deleted; a mirror row the repo no longer has is
// reported as orphaned. A dry run is the same code inside transactions
// that are rolled back.

// SyncReport counts what a sync did (or, dry, would do).
type SyncReport struct {
	Created  map[string]int `json:"created"`
	Updated  map[string]int `json:"updated"`
	Same     map[string]int `json:"same"`
	Diverged []string       `json:"diverged"`
	Orphaned []string       `json:"orphaned"`
}

func newReport() *SyncReport {
	return &SyncReport{Created: map[string]int{}, Updated: map[string]int{}, Same: map[string]int{}}
}

// Changed reports whether the sync wrote (or would write) anything.
func (r *SyncReport) Changed() bool {
	for _, m := range []map[string]int{r.Created, r.Updated} {
		for _, n := range m {
			if n > 0 {
				return true
			}
		}
	}
	return false
}

// Sync runs against an unheaded_config connection as config_admin and an
// unheaded_ops connection as ops_writer. actor is recorded on every seeded
// row.
func Sync(ctx context.Context, repoDir string, cfgDB, opsDB *sql.DB, actor string, dryRun bool) (*SyncReport, error) {
	if err := checkWho(actor, "sync"); err != nil {
		return nil, err
	}
	m, err := LoadModel(repoDir)
	if err != nil {
		return nil, err
	}
	y, err := newYAMLStore(repoDir, m)
	if err != nil {
		return nil, err
	}
	cat, err := crosswalk.LoadFS(os.DirFS(repoDir), "compliance/catalog/frameworks", "compliance/catalog/controls.yaml")
	if err != nil {
		return nil, err
	}
	raw, err := os.ReadFile(repoDir + "/compliance/findings/register.yaml") // #nosec G304 -- fixed path under the operator's repo dir
	if err != nil {
		return nil, err
	}
	reg, err := crosswalk.LoadRegister(raw, cat)
	if err != nil {
		return nil, err
	}

	rep := newReport()
	cfgTx, err := cfgDB.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	if err := syncConfig(ctx, cfgTx, m, y, cat, actor, rep); err != nil {
		return nil, errors.Join(err, cfgTx.Rollback())
	}
	opsTx, err := opsDB.BeginTx(ctx, nil)
	if err != nil {
		return nil, errors.Join(err, cfgTx.Rollback())
	}
	if err := syncFindings(ctx, opsTx, reg, actor, rep); err != nil {
		return nil, errors.Join(err, opsTx.Rollback(), cfgTx.Rollback())
	}
	if dryRun {
		return rep, errors.Join(opsTx.Rollback(), cfgTx.Rollback())
	}
	// Two databases, two commits: config first, so findings never refer
	// to definitions that are not there. A failure between them leaves
	// config synced and findings not, and the next sync completes it.
	if err := cfgTx.Commit(); err != nil {
		return nil, errors.Join(err, opsTx.Rollback())
	}
	if err := opsTx.Commit(); err != nil {
		return nil, err
	}
	sort.Strings(rep.Diverged)
	sort.Strings(rep.Orphaned)
	return rep, nil
}

func hashOf(v any) (json.RawMessage, string, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return nil, "", err
	}
	s := sha256.Sum256(b)
	return b, hex.EncodeToString(s[:]), nil
}

type definition struct {
	id, title string
	value     any
}

// mirror upserts definitions into table by sha256 and reports orphans.
func mirror(ctx context.Context, tx *sql.Tx, table string, defs []definition, extra func(d definition) []any, rep *SyncReport) error {
	q := map[string][3]string{
		"compliance_fixes": {
			`SELECT id, sha256 FROM compliance_fixes`,
			`INSERT INTO compliance_fixes (id, title, body, sha256, class, risk, controls, closes) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
			`UPDATE compliance_fixes SET title = $2, body = $3, sha256 = $4, class = $5, risk = $6, controls = $7, closes = $8, synced_at = NOW() WHERE id = $1`,
		},
		"compliance_controls": {
			`SELECT id, sha256 FROM compliance_controls`,
			`INSERT INTO compliance_controls (id, title, body, sha256) VALUES ($1, $2, $3, $4)`,
			`UPDATE compliance_controls SET title = $2, body = $3, sha256 = $4, synced_at = NOW() WHERE id = $1`,
		},
		"compliance_frameworks": {
			`SELECT id, sha256 FROM compliance_frameworks`,
			`INSERT INTO compliance_frameworks (id, title, body, sha256) VALUES ($1, $2, $3, $4)`,
			`UPDATE compliance_frameworks SET title = $2, body = $3, sha256 = $4, synced_at = NOW() WHERE id = $1`,
		},
	}[table]
	rows, err := tx.QueryContext(ctx, q[0])
	if err != nil {
		return fmt.Errorf("%s: %w", table, err)
	}
	held := map[string]string{}
	for rows.Next() {
		var id, sum string
		if err := rows.Scan(&id, &sum); err != nil {
			return errors.Join(err, rows.Close())
		}
		held[id] = sum
	}
	if err := errors.Join(rows.Err(), rows.Close()); err != nil {
		return err
	}
	seen := map[string]bool{}
	for _, d := range defs {
		seen[d.id] = true
		body, sum, err := hashOf(d.value)
		if err != nil {
			return err
		}
		args := append([]any{d.id, d.title, body, sum}, extra(d)...)
		switch old, ok := held[d.id]; {
		case !ok:
			_, err = tx.ExecContext(ctx, q[1], args...)
			rep.Created[table]++
		case old != sum:
			_, err = tx.ExecContext(ctx, q[2], args...)
			rep.Updated[table]++
		default:
			rep.Same[table]++
		}
		if err != nil {
			return fmt.Errorf("%s %s: %w", table, d.id, err)
		}
	}
	for id := range held {
		if !seen[id] {
			rep.Orphaned = append(rep.Orphaned, table+":"+id)
		}
	}
	return nil
}

func syncConfig(ctx context.Context, tx *sql.Tx, m *remediation.Model, y *yamlStore, cat *crosswalk.Catalog, actor string, rep *SyncReport) error {
	var fixes, controls, frameworks []definition
	byFix := map[string]remediation.Fix{}
	for _, f := range m.Fixes {
		fixes = append(fixes, definition{f.ID, f.Title, f})
		byFix[f.ID] = f
	}
	for _, c := range cat.Controls {
		controls = append(controls, definition{c.ID, c.Title, c})
	}
	for _, f := range cat.Frameworks {
		frameworks = append(frameworks, definition{f.ID, f.Name, f})
	}
	none := func(definition) []any { return nil }
	fixExtra := func(d definition) []any {
		f := byFix[d.id]
		return []any{f.Class, f.Risk, pqArray(f.Controls), pqArray(f.Closes)}
	}
	if err := mirror(ctx, tx, "compliance_fixes", fixes, fixExtra, rep); err != nil {
		return err
	}
	if err := mirror(ctx, tx, "compliance_controls", controls, none, rep); err != nil {
		return err
	}
	if err := mirror(ctx, tx, "compliance_frameworks", frameworks, none, rep); err != nil {
		return err
	}

	for _, p := range y.profiles.Profiles {
		var all bool
		var have []string
		err := tx.QueryRowContext(ctx, `SELECT all_fixes, fixes FROM compliance_profiles WHERE name = $1`, p.Name).
			Scan(&all, pq.Array(&have))
		switch {
		case errors.Is(err, sql.ErrNoRows):
			if _, err := tx.ExecContext(ctx, `INSERT INTO compliance_profiles (name, description, all_fixes, fixes, updated_by)
				VALUES ($1, $2, $3, $4, $5)`, p.Name, p.Description, p.All, pqArray(p.Fixes), actor); err != nil {
				return fmt.Errorf("profile %s: %w", p.Name, err)
			}
			rep.Created["compliance_profiles"]++
		case err != nil:
			return fmt.Errorf("profile %s: %w", p.Name, err)
		case all != p.All || !sameSet(have, p.Fixes):
			rep.Diverged = append(rep.Diverged, "profile:"+p.Name)
		default:
			rep.Same["compliance_profiles"]++
		}
	}
	for _, b := range y.bindings {
		var profile, mode string
		err := tx.QueryRowContext(ctx, `SELECT profile, mode FROM compliance_host_bindings WHERE host = $1`, b.Host).
			Scan(&profile, &mode)
		switch {
		case errors.Is(err, sql.ErrNoRows):
			if _, err := tx.ExecContext(ctx, `INSERT INTO compliance_host_bindings (host, profile, mode, updated_by, change_reason)
				VALUES ($1, $2, $3, $4, $5)`, b.Host, b.Profile, b.Mode, actor,
				"seeded from compliance/hosts/"+b.Host+".yaml"); err != nil {
				return fmt.Errorf("binding %s: %w", b.Host, err)
			}
			rep.Created["compliance_host_bindings"]++
		case err != nil:
			return fmt.Errorf("binding %s: %w", b.Host, err)
		case profile != b.Profile || mode != b.Mode:
			rep.Diverged = append(rep.Diverged, "binding:"+b.Host)
		default:
			rep.Same["compliance_host_bindings"]++
		}
	}
	return nil
}

func sameSet(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	in := map[string]bool{}
	for _, s := range a {
		in[s] = true
	}
	for _, s := range b {
		if !in[s] {
			return false
		}
	}
	return true
}

func syncFindings(ctx context.Context, tx *sql.Tx, reg *crosswalk.Register, actor string, rep *SyncReport) error {
	for _, e := range reg.Entries {
		want, err := json.Marshal(e)
		if err != nil {
			return err
		}
		var have []byte
		err = tx.QueryRowContext(ctx, `SELECT entry FROM compliance_findings WHERE key = $1`, e.Key()).Scan(&have)
		switch {
		case errors.Is(err, sql.ErrNoRows):
			var on, until any
			if a := e.Accepted; a != nil {
				on, until = a.AcceptedOn, a.Until
			}
			if _, err := tx.ExecContext(ctx, `INSERT INTO compliance_findings
				(key, severity, class, host, step, lockout, opened, accepted_on, accepted_until, entry, updated_by, change_reason)
				VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)`,
				e.Key(), string(e.Severity), e.Class, e.Host, e.Step, e.Lockout, e.Opened, on, until, want, actor,
				"seeded from compliance/findings/register.yaml"); err != nil {
				return fmt.Errorf("finding %s: %w", e.Key(), err)
			}
			rep.Created["compliance_findings"]++
		case err != nil:
			return fmt.Errorf("finding %s: %w", e.Key(), err)
		default:
			same, err := jsonEqual(have, want)
			if err != nil {
				return err
			}
			if same {
				rep.Same["compliance_findings"]++
			} else {
				rep.Diverged = append(rep.Diverged, "finding:"+e.Key())
			}
		}
	}
	return nil
}

// jsonEqual compares two JSON documents by value (jsonb reorders keys).
func jsonEqual(a, b []byte) (bool, error) {
	var x, y any
	if err := json.Unmarshal(a, &x); err != nil {
		return false, err
	}
	if err := json.Unmarshal(b, &y); err != nil {
		return false, err
	}
	xa, _ := json.Marshal(x)
	ya, _ := json.Marshal(y)
	return bytes.Equal(xa, ya), nil
}
