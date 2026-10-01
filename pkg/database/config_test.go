// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package database

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/lib/pq"
)

func unsetWell(t *testing.T) {
	t.Helper()
	for _, k := range []string{"WELL_HOST", "WELL_PORT", "WELL_USER", "WELL_PASSWORD", "WELL_DB", "WELL_SSLMODE"} {
		t.Setenv(k, "x") // registers restore on cleanup
		unsetenv(t, k)
	}
}

// FND-007: no credential is built in. The default user is the service's
// scoped role, never the superuser, and there is no default password.
func TestConfig_NoCredentialDefaults(t *testing.T) {
	unsetWell(t)
	for name, c := range map[string]struct {
		cfg  Config
		user string
	}{
		"ops writer": {OpsWriterConfig(), "ops_writer"},
		"kanban":     {AppKanbanConfig(), "app_kanban"},
	} {
		if c.cfg.User != c.user {
			t.Errorf("%s: default user %q, want scoped role %q", name, c.cfg.User, c.user)
		}
		if c.cfg.Password != "" {
			t.Errorf("%s: a default password is built in", name)
		}
	}
}

func TestConnect_MissingCredentialsFailsLoudly(t *testing.T) {
	for name, cfg := range map[string]Config{
		"no password": {Host: "127.0.0.1", Port: "1", User: "ops_writer", DBName: "unheaded_ops", SSLMode: "disable"},
		"no user":     {Host: "127.0.0.1", Port: "1", Password: "p", DBName: "unheaded_ops", SSLMode: "disable"},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := Connect(context.Background(), cfg); !errors.Is(err, ErrMissingCredentials) {
				t.Fatalf("got %v, want ErrMissingCredentials (before any connection attempt)", err)
			}
		})
	}
}

// A value with a space or quote must not add or change DSN keywords.
func TestDSN_QuotesEveryValue(t *testing.T) {
	cfg := Config{Host: "h", Port: "5432", User: "u", Password: `p w' sslmode=disable \x`, DBName: "d", SSLMode: "require"}
	want := `host='h' port='5432' user='u' password='p w\' sslmode=disable \\x' dbname='d' sslmode='require'`
	if got := cfg.DSN(); got != want {
		t.Errorf("DSN:\n got %s\nwant %s", got, want)
	}
	// The driver itself must read it back as one value per keyword.
	if _, err := pq.NewConnector(cfg.DSN()); err != nil {
		t.Errorf("lib/pq rejects the quoted DSN: %v", err)
	}
}

func unsetenv(t *testing.T, k string) {
	t.Helper()
	if err := os.Unsetenv(k); err != nil {
		t.Fatal(err)
	}
}
