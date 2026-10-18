// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis.

package migrate

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"time"
)

// lockKey is the advisory lock every runner takes per database, so two
// runners never apply the same migration at once.
const lockKey = 0x57454c4c // "WELL"

// Runner talks to The Well. Open returns a pool for one database; the
// runner closes what it opens.
type Runner struct {
	Open func(db string) (*sql.DB, error)
	// LockTimeout bounds every lock wait inside a migration (W-10): a
	// migration that cannot get its lock fails instead of queueing every
	// query behind it. StatementTimeout bounds each statement.
	LockTimeout      time.Duration
	StatementTimeout time.Duration
	Out              io.Writer
}

// ReadLedger reads every database's ledger. A database without the ledger
// table has no records yet; reading never creates it.
func (r *Runner) ReadLedger(ctx context.Context, dbs []string) (Ledger, error) {
	led := Ledger{}
	for _, name := range dbs {
		db, err := r.Open(name)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", name, err)
		}
		rows, err := readLedger(ctx, db, name)
		if cerr := db.Close(); err == nil {
			err = cerr
		}
		if err != nil {
			return nil, fmt.Errorf("%s: %w", name, err)
		}
		led[name] = rows
	}
	return led, nil
}

func readLedger(ctx context.Context, db *sql.DB, name string) (map[string]Record, error) {
	var exists bool
	if err := db.QueryRowContext(ctx, `SELECT to_regclass('public.well_schema_migrations') IS NOT NULL`).Scan(&exists); err != nil {
		return nil, err
	}
	out := map[string]Record{}
	if !exists {
		return out, nil
	}
	rows, err := db.QueryContext(ctx, `SELECT filename, sha256, mode, applied_at, applied_by FROM well_schema_migrations`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		rec := Record{DB: name}
		if err := rows.Scan(&rec.File, &rec.SHA256, &rec.Mode, &rec.AppliedAt, &rec.AppliedBy); err != nil {
			return nil, err
		}
		out[rec.File] = rec
	}
	return out, rows.Err()
}

// Apply runs the plan's pending steps in order and stops at the first
// failure. It refuses a plan with drift. Transactional steps run with
// their ledger row in one transaction, so a failure records nothing and
// changes nothing; a notx step that fails part way is reported as such.
// With baseline, steps are recorded as applied without running them (for
// a Well whose schema already matches, after a checked shape diff).
func (r *Runner) Apply(ctx context.Context, set Set, p Plan, baseline bool) error {
	if err := p.Drift(); err != nil {
		return err
	}
	mode := "apply"
	if baseline {
		mode = "baseline"
	}
	for _, st := range p.Pending() {
		start := time.Now()
		if err := r.applyOne(ctx, set, st, mode); err != nil {
			return fmt.Errorf("%s on %s: %w", st.File.File, st.DB, err)
		}
		fmt.Fprintf(r.Out, "%-8s %-16s %s (%s)\n", mode, st.DB, st.File.File, time.Since(start).Round(time.Millisecond))
	}
	return nil
}

func (r *Runner) applyOne(ctx context.Context, set Set, st Step, mode string) error {
	db, err := r.Open(st.DB)
	if err != nil {
		return err
	}
	defer db.Close()
	conn, err := db.Conn(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = conn.Close() }()
	// The ledger's own DDL is idempotent and runs outside the migration.
	if _, err := conn.ExecContext(ctx, set.Ledger); err != nil {
		return fmt.Errorf("ledger: %w", err)
	}
	timeouts := fmt.Sprintf("SET %%s lock_timeout = '%dms'; SET %%s statement_timeout = '%dms'",
		r.LockTimeout.Milliseconds(), r.StatementTimeout.Milliseconds())
	record := `INSERT INTO well_schema_migrations (filename, sha256, mode) VALUES ($1, $2, $3)`

	if !st.NoTx || mode == "baseline" {
		tx, err := conn.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		defer func() { _ = tx.Rollback() }()
		if _, err := tx.ExecContext(ctx, fmt.Sprintf(timeouts, "LOCAL", "LOCAL")); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock($1)`, lockKey); err != nil {
			return fmt.Errorf("advisory lock: %w", err)
		}
		if done, err := recorded(ctx, tx, st.File.File); err != nil || done {
			return errors.Join(err, errAlready(done))
		}
		if mode == "apply" {
			if err := becomeOwner(ctx, tx, st.File, true); err != nil {
				return err
			}
			if _, err := tx.ExecContext(ctx, st.SQL); err != nil {
				return err
			}
			if _, err := tx.ExecContext(ctx, `RESET ROLE`); err != nil {
				return err
			}
		}
		if _, err := tx.ExecContext(ctx, record, st.File.File, st.SHA256, mode); err != nil {
			return fmt.Errorf("record: %w", err)
		}
		return tx.Commit()
	}

	// notx: one statement at a time on one session, under a session lock.
	stmts, err := Split(st.SQL)
	if err != nil {
		return err
	}
	if _, err := conn.ExecContext(ctx, fmt.Sprintf(timeouts, "SESSION", "SESSION")); err != nil {
		return err
	}
	if _, err := conn.ExecContext(ctx, `SELECT pg_advisory_lock($1)`, lockKey); err != nil {
		return fmt.Errorf("advisory lock: %w", err)
	}
	defer func() { _, _ = conn.ExecContext(context.Background(), `SELECT pg_advisory_unlock($1)`, lockKey) }()
	if done, err := recorded(ctx, conn, st.File.File); err != nil || done {
		return errors.Join(err, errAlready(done))
	}
	if err := becomeOwner(ctx, conn, st.File, false); err != nil {
		return err
	}
	defer func() { _, _ = conn.ExecContext(context.Background(), `RESET ROLE`) }()
	for i, s := range stmts {
		if _, err := conn.ExecContext(ctx, s); err != nil {
			return fmt.Errorf("statement %d of %d failed after %d ran outside a transaction (not recorded; check by hand): %w", i+1, len(stmts), i, err)
		}
	}
	if _, err := conn.ExecContext(ctx, `RESET ROLE`); err != nil {
		return err
	}
	if _, err := conn.ExecContext(ctx, record, st.File.File, st.SHA256, mode); err != nil {
		return fmt.Errorf("ran, but recording failed: %w", err)
	}
	return nil
}

type queryer interface {
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

type execQueryer interface {
	queryer
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

// becomeOwner switches to the database's owner role when that owner is not
// a superuser (ERDA 2.2, db/migrations/025), so what the migration creates
// belongs to the owner, as on a fresh Well. Entries flagged superuser keep
// the runner's role. local scopes the switch to the transaction.
func becomeOwner(ctx context.Context, x execQueryer, f File, local bool) error {
	if f.Superuser {
		return nil
	}
	var role string
	err := x.QueryRowContext(ctx, `SELECT r.rolname FROM pg_database d JOIN pg_roles r ON r.oid = d.datdba
		WHERE d.datname = current_database() AND NOT r.rolsuper`).Scan(&role)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("database owner: %w", err)
	}
	if _, err := x.ExecContext(ctx, `SELECT set_config('role', $1, $2)`, role, local); err != nil {
		return fmt.Errorf("set role %s: %w", role, err)
	}
	return nil
}

// recorded re-checks the ledger under the lock: another runner may have
// applied the step since the plan was computed.
func recorded(ctx context.Context, q queryer, file string) (bool, error) {
	var n int
	err := q.QueryRowContext(ctx, `SELECT count(*) FROM well_schema_migrations WHERE filename = $1`, file).Scan(&n)
	return n > 0, err
}

func errAlready(done bool) error {
	if done {
		return errors.New("recorded by another runner since the plan was made; run status again")
	}
	return nil
}
