// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis.

package sessions

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/lib/pq"

	"unheaded/pkg/database"
)

// The Well keeps sessions in unheaded_app.bff_sessions (migrations 055 and
// 056). A front never touches that table: it logs in as its own role
// (WellRole) and is granted only the view bff_sessions_own, which shows and
// accepts the rows of the front the role is named after. One front's
// password therefore cannot read, forge or end another front's sessions.
// The command line logs in as app_auth, which may read the table and set
// revoked_at and nothing else.

const (
	// wellQueryTimeout bounds every query: a stuck Well refuses the
	// request (fail closed) instead of hanging it.
	wellQueryTimeout = 3 * time.Second

	ownView      = "bff_sessions_own"
	sessionTable = "bff_sessions"
	columns      = `id_hash, front, subject, roles, created_at, last_used_at, expires_at, absolute_expires_at, revoked_at`
)

// WellRole is the Well login role a front's sessions are kept under:
// "bff_" and the front's name with "-" as "_". The view compares it to the
// connection's user, which is why a front name carries no underscore.
func WellRole(front string) string { return "bff_" + strings.ReplaceAll(front, "-", "_") }

type rowScanner interface {
	Scan(dest ...any) error
}

func scanSession(row rowScanner) (Session, error) {
	var (
		s       Session
		roles   string
		revoked sql.NullTime
	)
	if err := row.Scan(&s.KeyHash, &s.Front, &s.Subject, &roles, &s.CreatedAt, &s.LastUsedAt, &s.ExpiresAt, &s.AbsoluteExpiresAt, &revoked); err != nil {
		return Session{}, err
	}
	if roles != "" {
		s.Roles = strings.Split(roles, ",")
	}
	s.CreatedAt, s.LastUsedAt = s.CreatedAt.UTC(), s.LastUsedAt.UTC()
	s.ExpiresAt, s.AbsoluteExpiresAt = s.ExpiresAt.UTC(), s.AbsoluteExpiresAt.UTC()
	if revoked.Valid {
		t := revoked.Time.UTC()
		s.RevokedAt = &t
	}
	return s, nil
}

func querySessions(ctx context.Context, db *sql.DB, q string, args ...any) ([]Session, error) {
	ctx, cancel := context.WithTimeout(ctx, wellQueryTimeout)
	defer cancel()
	rows, err := db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Session
	for rows.Next() {
		s, err := scanSession(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

func execCount(ctx context.Context, db *sql.DB, q string, args ...any) (int, error) {
	ctx, cancel := context.WithTimeout(ctx, wellQueryTimeout)
	defer cancel()
	res, err := db.ExecContext(ctx, q, args...)
	if err != nil {
		return 0, err
	}
	n, err := res.RowsAffected()
	return int(n), err
}

// wellStore is one front's Store over the view.
type wellStore struct {
	db    *sql.DB
	front string
}

// NewWellStore returns front's Store on db, a connection logged in as
// WellRole(front). It checks the view is readable first, so a front with
// the wrong login or an un-migrated Well finds out when it starts.
func NewWellStore(ctx context.Context, db *sql.DB, front string) (Store, error) {
	if db == nil {
		return nil, errors.New("sessions: no database")
	}
	if !ValidFront(front) {
		return nil, fmt.Errorf("%w: front %q cannot keep sessions in the Well (want [a-z0-9][a-z0-9-]*, at most 59 characters, no underscore)", ErrInvalid, front)
	}
	qctx, cancel := context.WithTimeout(ctx, wellQueryTimeout)
	defer cancel()
	if _, err := db.ExecContext(qctx, `SELECT 1 FROM `+ownView+` LIMIT 0`); err != nil {
		return nil, fmt.Errorf("sessions: the Well's %s is not readable (migrations 055 and 056 applied? logged in as %s?): %w", ownView, WellRole(front), err)
	}
	return &wellStore{db: db, front: front}, nil
}

func (w *wellStore) Create(ctx context.Context, s Session, maxLive int) error {
	if err := s.Validate(); err != nil {
		return err
	}
	if s.Front != w.front {
		return fmt.Errorf("%w: a session of %q offered to the store of %q", ErrInvalid, s.Front, w.front)
	}
	// One statement: the cap is counted and the row written together. The
	// count is of this front's live rows (the view shows no others).
	n, err := execCount(ctx, w.db,
		`INSERT INTO `+ownView+` (id_hash, front, subject, roles, created_at, last_used_at, expires_at, absolute_expires_at)
		 SELECT $1::text, $2::text, $3::text, $4::text, $5::timestamptz, $5::timestamptz, $6::timestamptz, $7::timestamptz
		 WHERE (SELECT count(*) FROM `+ownView+` WHERE revoked_at IS NULL AND expires_at > $5::timestamptz) < $8::bigint`,
		s.KeyHash, s.Front, s.Subject, strings.Join(s.Roles, ","), s.CreatedAt.UTC(), s.ExpiresAt.UTC(), s.AbsoluteExpiresAt.UTC(), maxLive)
	if err != nil {
		return fmt.Errorf("sessions: create: %w", err)
	}
	if n == 0 {
		return ErrFull
	}
	return nil
}

func (w *wellStore) Get(ctx context.Context, keyHash string) (Session, bool, error) {
	if !ValidKeyHash(keyHash) {
		return Session{}, false, nil
	}
	// front is named as well as implied by the view: two locks on one door.
	out, err := querySessions(ctx, w.db,
		`SELECT `+columns+` FROM `+ownView+` WHERE id_hash = $1::text AND front = $2::text`, keyHash, w.front)
	if err != nil {
		return Session{}, false, fmt.Errorf("sessions: get: %w", err)
	}
	if len(out) == 0 {
		return Session{}, false, nil
	}
	return out[0], true, nil
}

func (w *wellStore) Touch(ctx context.Context, keyHash string, now, expires time.Time) (bool, error) {
	if !ValidKeyHash(keyHash) {
		return false, nil
	}
	// No condition on last_used_at: two requests may write out of order,
	// and the one that lands second must still match the row, or a live
	// session would look ended. GREATEST keeps both columns moving forward.
	n, err := execCount(ctx, w.db,
		`UPDATE `+ownView+` SET last_used_at = GREATEST(last_used_at, $3::timestamptz),
		        expires_at = GREATEST(expires_at, LEAST($4::timestamptz, absolute_expires_at))
		 WHERE id_hash = $1::text AND front = $2::text AND revoked_at IS NULL
		   AND expires_at > $3::timestamptz AND absolute_expires_at > $3::timestamptz`,
		keyHash, w.front, now.UTC(), expires.UTC())
	if err != nil {
		return false, fmt.Errorf("sessions: touch: %w", err)
	}
	return n > 0, nil
}

func (w *wellStore) Revoke(ctx context.Context, keyHash string, now time.Time) error {
	if !ValidKeyHash(keyHash) {
		return nil
	}
	_, err := execCount(ctx, w.db,
		`UPDATE `+ownView+` SET revoked_at = GREATEST($3::timestamptz, created_at)
		 WHERE id_hash = $1::text AND front = $2::text AND revoked_at IS NULL`,
		keyHash, w.front, now.UTC())
	if err != nil {
		return fmt.Errorf("sessions: revoke: %w", err)
	}
	return nil
}

func (w *wellStore) Sweep(ctx context.Context, endedBefore time.Time, limit int) (int, error) {
	if limit <= 0 {
		return 0, nil
	}
	n, err := execCount(ctx, w.db,
		`DELETE FROM `+ownView+` WHERE id_hash IN (
		     SELECT id_hash FROM `+ownView+`
		     WHERE front = $1::text AND (expires_at < $2::timestamptz OR revoked_at < $2::timestamptz)
		     ORDER BY expires_at LIMIT $3::int)`,
		w.front, endedBefore.UTC(), limit)
	if err != nil {
		return 0, fmt.Errorf("sessions: sweep: %w", err)
	}
	return n, nil
}

func (w *wellStore) Close() error { return w.db.Close() }

// WellStoreConns is the size of a front's connection pool to the Well. Each
// request with a session cookie is one short read; a pool of two keeps one
// slow answer from holding up the rest. The role's connection limit
// (migration 055) is one above it.
const WellStoreConns = 2

// connectWellStore opens the Well as front's role and returns its Store.
func connectWellStore(ctx context.Context, front string) (Store, error) {
	db, err := database.Connect(ctx, database.BFFSessionConfig(WellRole(front)))
	if err != nil {
		return nil, fmt.Errorf("sessions: the Well, as %s (set AUTH_SESSION_WELL_PASSWORD): %w", WellRole(front), err)
	}
	db.SetMaxOpenConns(WellStoreConns)
	db.SetMaxIdleConns(WellStoreConns)
	// The front's role cannot read the migration ledger; the view answering
	// is the proof that 055 and 056 ran and that this is the right login.
	s, err := NewWellStore(ctx, db, front)
	if err != nil {
		_ = db.Close()
		return nil, err
	}
	return s, nil
}

// refusedByWell reports whether err is the Well, or this process's own
// configuration, saying no: a wrong password, a missing role or database, a
// missing migration, a weak sslmode. Waiting cures none of these. Anything
// else (a refused connection, a timeout) is a Well that is not there yet.
func refusedByWell(err error) bool {
	var pe *pq.Error
	switch {
	case errors.As(err, &pe):
		// A server that is starting or stopping (class 57), out of
		// connections (53) or dropping this one (08) is not there yet; any
		// other answer is a refusal: a bad password or missing role (28), a
		// missing database (3D), a missing view or grant (42).
		cls := string(pe.Code.Class())
		return cls != "57" && cls != "53" && cls != "08"
	case errors.Is(err, database.ErrMissingCredentials),
		errors.Is(err, database.ErrWeakTLS),
		errors.Is(err, database.ErrMissingCA),
		errors.Is(err, ErrInvalid):
		return true
	}
	return false
}

// OpenWell returns front's Store in the Well.
//
// A Well that answers and refuses is an error: the front must not start
// half-configured. A Well that does not answer is waited for, as kanban-app
// waits for its board: the Store is returned at once and every call on it
// is ErrUnavailable until the Well is reached, retried every second and
// then less often, up to every 30 seconds. Nothing stands in for the Well
// meanwhile: no session is accepted and no login succeeds.
func OpenWell(ctx context.Context, front string) (Store, error) {
	if !ValidFront(front) {
		return nil, fmt.Errorf("%w: front %q cannot keep sessions in the Well (want [a-z0-9][a-z0-9-]*, at most 59 characters, no underscore)", ErrInvalid, front)
	}
	return newGate(ctx, front, func(c context.Context) (Store, error) { return connectWellStore(c, front) },
		refusedByWell, time.Second, 30*time.Second)
}

// wellAdmin is the command line's Admin over the table, as app_auth.
type wellAdmin struct {
	db *sql.DB
}

// NewWellAdmin returns an Admin on db, a connection logged in as app_auth.
func NewWellAdmin(ctx context.Context, db *sql.DB) (Admin, error) {
	if db == nil {
		return nil, errors.New("sessions: no database")
	}
	qctx, cancel := context.WithTimeout(ctx, wellQueryTimeout)
	defer cancel()
	if _, err := db.ExecContext(qctx, `SELECT 1 FROM `+sessionTable+` LIMIT 0`); err != nil {
		return nil, fmt.Errorf("sessions: the Well's %s is not readable (migration 056 applied? logged in as app_auth?): %w", sessionTable, err)
	}
	return &wellAdmin{db: db}, nil
}

// OpenWellAdmin connects as database.AppAuthConfig (AUTH_WELL_PASSWORD,
// WELL_HOST, ...), the login unheaded-cli apikey uses, with one connection.
func OpenWellAdmin(ctx context.Context) (Admin, error) {
	db, err := database.Connect(ctx, database.AppAuthConfig())
	if err != nil {
		return nil, fmt.Errorf("sessions: the Well, as app_auth (set AUTH_WELL_PASSWORD): %w", err)
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	a, err := NewWellAdmin(ctx, db)
	if err != nil {
		_ = db.Close()
		return nil, err
	}
	return a, nil
}

func (a *wellAdmin) List(ctx context.Context, f Filter, now time.Time) ([]Session, error) {
	out, err := querySessions(ctx, a.db,
		`SELECT `+columns+` FROM `+sessionTable+`
		 WHERE ($1::text = '' OR subject = $1::text) AND ($2::text = '' OR front = $2::text)
		   AND ($3::boolean OR (revoked_at IS NULL AND expires_at > $4::timestamptz))
		 ORDER BY created_at DESC, id_hash LIMIT $5::int`,
		f.Subject, f.Front, f.Ended, now.UTC(), ListLimit)
	if err != nil {
		return nil, fmt.Errorf("sessions: list: %w", err)
	}
	return out, nil
}

func (a *wellAdmin) Revoke(ctx context.Context, prefix string, now time.Time) (Session, error) {
	if err := checkPrefix(prefix); err != nil {
		return Session{}, err
	}
	hits, err := querySessions(ctx, a.db,
		`SELECT `+columns+` FROM `+sessionTable+`
		 WHERE left(id_hash, length($1::text)) = $1::text AND revoked_at IS NULL
		 ORDER BY id_hash LIMIT 2`,
		prefix)
	if err != nil {
		return Session{}, fmt.Errorf("sessions: revoke: %w", err)
	}
	switch len(hits) {
	case 0:
		return Session{}, fmt.Errorf("%w: no session with id %s that is not already revoked", ErrNotFound, prefix)
	case 1:
	default:
		return Session{}, fmt.Errorf("%w: %s matches more than one session; give more characters", ErrAmbiguous, prefix)
	}
	out, err := querySessions(ctx, a.db,
		`UPDATE `+sessionTable+` SET revoked_at = GREATEST($2::timestamptz, created_at)
		 WHERE id_hash = $1::text AND revoked_at IS NULL RETURNING `+columns,
		hits[0].KeyHash, now.UTC())
	if err != nil {
		return Session{}, fmt.Errorf("sessions: revoke: %w", err)
	}
	if len(out) == 0 {
		return Session{}, fmt.Errorf("%w: session %s ended before it could be revoked", ErrNotFound, prefix)
	}
	return out[0], nil
}

func (a *wellAdmin) RevokeSubject(ctx context.Context, subject, front string, now time.Time) (int, error) {
	if !ValidSubject(subject) {
		return 0, fmt.Errorf("%w: subject", ErrInvalid)
	}
	n, err := execCount(ctx, a.db,
		`UPDATE `+sessionTable+` SET revoked_at = GREATEST($3::timestamptz, created_at)
		 WHERE subject = $1::text AND ($2::text = '' OR front = $2::text) AND revoked_at IS NULL`,
		subject, front, now.UTC())
	if err != nil {
		return 0, fmt.Errorf("sessions: revoke subject: %w", err)
	}
	return n, nil
}

func (a *wellAdmin) Close() error { return a.db.Close() }
