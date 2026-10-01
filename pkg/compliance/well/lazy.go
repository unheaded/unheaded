// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package well

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sync"
	"time"

	"unheaded/pkg/database"
)

// Lazy is a connection to The Well for long-running readers such as the
// dashboard: it connects on first use and, while The Well is down, tries
// again at most once per retry interval, so a reader started before The
// Well catches up without a restart and never hammers it meanwhile (the
// 2026-09 failed-login spam was a retry loop like that, unthrottled).
type Lazy struct {
	// Env names the variables that configure this connection, for the
	// error a reader shows when they are missing.
	Env string

	cfg   database.Config
	retry time.Duration
	now   func() time.Time

	mu      sync.Mutex
	db      *sql.DB
	lastTry time.Time
	lastErr error
}

// NewLazy returns a lazy connection; nothing is dialled yet.
func NewLazy(cfg database.Config, retry time.Duration) *Lazy {
	return &Lazy{cfg: cfg, retry: retry, now: time.Now}
}

// DB returns the connection, or the reason there is none. Between retries
// it returns the last error without dialling.
func (l *Lazy) DB(ctx context.Context) (*sql.DB, error) {
	if l == nil {
		return nil, errNotConfigured
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.db != nil {
		return l.db, nil
	}
	if !l.lastTry.IsZero() && l.now().Sub(l.lastTry) < l.retry {
		return nil, l.lastErr
	}
	l.lastTry = l.now()
	db, err := database.Connect(ctx, l.cfg)
	if err != nil {
		if errors.Is(err, database.ErrMissingCredentials) && l.Env != "" {
			err = fmt.Errorf("%w: set %s", err, l.Env)
		}
		l.lastErr = err
		return nil, err
	}
	l.db = db
	return db, nil
}

// Close closes the connection if one was made.
func (l *Lazy) Close() error {
	if l == nil {
		return nil
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.db == nil {
		return nil
	}
	err := l.db.Close()
	l.db = nil
	return err
}
