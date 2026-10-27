// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis.

package auth

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"regexp"
	"time"

	"unheaded/pkg/database"
)

// wellQueryTimeout bounds every API-key query: a stuck Well refuses the
// request (fail closed) instead of hanging it.
const wellQueryTimeout = 3 * time.Second

// NewWellAPIKeyStore returns a store over unheaded_app.api_keys
// (migrations 035/036), on a connection as app_auth. It checks the table
// is readable before returning, so a service with the wrong login or an
// un-migrated Well refuses to start rather than refusing every key later.
func NewWellAPIKeyStore(ctx context.Context, db *sql.DB, opts ...APIKeyStoreOption) (APIKeyStore, error) {
	if db == nil {
		return nil, errors.New("auth: API key store: no database")
	}
	qctx, cancel := context.WithTimeout(ctx, wellQueryTimeout)
	defer cancel()
	if _, err := db.ExecContext(qctx, `SELECT 1 FROM api_keys LIMIT 0`); err != nil {
		return nil, fmt.Errorf("auth: API key store: the Well's api_keys is not readable (migration 036 applied? logged in as app_auth?): %w", err)
	}
	return newAPIKeyStore(&wellAPIKeys{db: db}, opts), nil
}

// openWellAPIKeyStore connects as database.AppAuthConfig (AUTH_WELL_USER,
// AUTH_WELL_PASSWORD, WELL_HOST, ...) with a two-connection pool.
func openWellAPIKeyStore(ctx context.Context, opts ...APIKeyStoreOption) (APIKeyStore, error) {
	db, err := database.Connect(ctx, database.AppAuthConfig())
	if err != nil {
		return nil, fmt.Errorf("auth: API key store (the Well, as app_auth; set AUTH_WELL_PASSWORD): %w", err)
	}
	db.SetMaxOpenConns(2)
	db.SetMaxIdleConns(2)
	s, err := NewWellAPIKeyStore(ctx, db, opts...)
	if err != nil {
		_ = db.Close()
		return nil, err
	}
	return s, nil
}

var uuidRE = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

type wellAPIKeys struct {
	db *sql.DB
}

const apiKeyColumns = `id::text, key_hash, prefix, owner, role, created_at, expires_at, revoked_at, last_used_at`

type rowScanner interface {
	Scan(dest ...any) error
}

func scanAPIKey(row rowScanner) (APIKeyRecord, error) {
	var (
		r                 APIKeyRecord
		revoked, lastUsed sql.NullTime
	)
	if err := row.Scan(&r.ID, &r.KeyHash, &r.Prefix, &r.Owner, &r.Role, &r.CreatedAt, &r.ExpiresAt, &revoked, &lastUsed); err != nil {
		return APIKeyRecord{}, err
	}
	r.CreatedAt, r.ExpiresAt = r.CreatedAt.UTC(), r.ExpiresAt.UTC()
	if revoked.Valid {
		t := revoked.Time.UTC()
		r.RevokedAt = &t
	}
	if lastUsed.Valid {
		t := lastUsed.Time.UTC()
		r.LastUsedAt = &t
	}
	return r, r.validate()
}

func (w *wellAPIKeys) query(ctx context.Context, q string, args ...any) ([]APIKeyRecord, error) {
	ctx, cancel := context.WithTimeout(ctx, wellQueryTimeout)
	defer cancel()
	rows, err := w.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []APIKeyRecord
	for rows.Next() {
		r, err := scanAPIKey(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (w *wellAPIKeys) insert(ctx context.Context, rec APIKeyRecord) error {
	ctx, cancel := context.WithTimeout(ctx, wellQueryTimeout)
	defer cancel()
	_, err := w.db.ExecContext(ctx,
		`INSERT INTO api_keys (id, key_hash, prefix, owner, role, created_at, expires_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		rec.ID, rec.KeyHash, rec.Prefix, rec.Owner, rec.Role, rec.CreatedAt, rec.ExpiresAt)
	if err != nil {
		return fmt.Errorf("auth: API key store insert: %w", err)
	}
	return nil
}

func (w *wellAPIKeys) byHash(ctx context.Context, hash string) (APIKeyRecord, bool, error) {
	recs, err := w.query(ctx, `SELECT `+apiKeyColumns+` FROM api_keys WHERE key_hash = $1`, hash)
	if err != nil || len(recs) == 0 {
		return APIKeyRecord{}, false, err
	}
	return recs[0], true, nil
}

func (w *wellAPIKeys) byPrefixOrID(ctx context.Context, s string) ([]APIKeyRecord, error) {
	if uuidRE.MatchString(s) {
		return w.query(ctx, `SELECT `+apiKeyColumns+` FROM api_keys WHERE id = $1::uuid`, s)
	}
	return w.query(ctx, `SELECT `+apiKeyColumns+` FROM api_keys WHERE prefix = $1 ORDER BY created_at, id`, s)
}

func (w *wellAPIKeys) all(ctx context.Context) ([]APIKeyRecord, error) {
	return w.query(ctx, `SELECT `+apiKeyColumns+` FROM api_keys ORDER BY created_at, id`)
}

func (w *wellAPIKeys) revoke(ctx context.Context, id string, at time.Time) (APIKeyRecord, error) {
	recs, err := w.query(ctx,
		`UPDATE api_keys SET revoked_at = coalesce(revoked_at, $2) WHERE id = $1::uuid RETURNING `+apiKeyColumns, id, at)
	if err != nil {
		return APIKeyRecord{}, fmt.Errorf("auth: API key store revoke: %w", err)
	}
	if len(recs) == 0 {
		return APIKeyRecord{}, fmt.Errorf("%w: id %q", ErrAPIKeyNotFound, id)
	}
	return recs[0], nil
}

func (w *wellAPIKeys) touch(ctx context.Context, id string, at time.Time) error {
	ctx, cancel := context.WithTimeout(ctx, wellQueryTimeout)
	defer cancel()
	_, err := w.db.ExecContext(ctx, `UPDATE api_keys SET last_used_at = $2 WHERE id = $1::uuid`, id, at)
	return err
}
