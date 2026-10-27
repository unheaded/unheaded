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
		"auth":       {AppAuthConfig(), "app_auth"},
	} {
		if c.cfg.User != c.user {
			t.Errorf("%s: default user %q, want scoped role %q", name, c.cfg.User, c.user)
		}
		if c.cfg.Password != "" {
			t.Errorf("%s: a default password is built in", name)
		}
	}
}

// The API-key store (pkg/auth, migration 036) logs in as app_auth to
// unheaded_app whatever role and database the service itself uses: its
// WELL_USER/WELL_PASSWORD/WELL_DB belong to that other role (kanban-app's
// app_kanban, dashboard-backend's unheaded_ops). Host, port and sslmode
// are the same Well's.
func TestAppAuthConfig_OwnCredentials(t *testing.T) {
	unsetWell(t)
	for _, k := range []string{"AUTH_WELL_USER", "AUTH_WELL_PASSWORD", "AUTH_WELL_DB"} {
		t.Setenv(k, "x")
		unsetenv(t, k)
	}
	t.Setenv("WELL_USER", "app_kanban")
	t.Setenv("WELL_PASSWORD", "kanban-pw")
	t.Setenv("WELL_DB", "unheaded_ops")
	t.Setenv("WELL_HOST", "10.10.10.5")
	t.Setenv("WELL_SSLMODE", "verify-full")
	c := AppAuthConfig()
	if c.User != "app_auth" || c.Password != "" || c.DBName != "unheaded_app" {
		t.Errorf("took the service's own Well login: %+v", c)
	}
	if c.Host != "10.10.10.5" || c.SSLMode != "verify-full" || c.Port != "5432" {
		t.Errorf("not the same Well: %+v", c)
	}
	t.Setenv("AUTH_WELL_USER", "app_auth_ro")
	t.Setenv("AUTH_WELL_PASSWORD", "auth-pw")
	t.Setenv("AUTH_WELL_DB", "unheaded_app_test")
	c = AppAuthConfig()
	if c.User != "app_auth_ro" || c.Password != "auth-pw" || c.DBName != "unheaded_app_test" {
		t.Errorf("AUTH_WELL_* not read: %+v", c)
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

// The Well accepts TLS only (db/pg_hba.conf). A client that would talk
// without TLS to anything but this host, or fall back to plaintext when
// asked (allow, prefer), is refused before it connects: a server that
// answers without TLS there is not The Well.
func TestConnect_RefusesWeakSSLMode(t *testing.T) {
	for name, cfg := range map[string]Config{
		"disable, network host":  {Host: "postgres", SSLMode: "disable"},
		"disable, private IP":    {Host: "172.28.1.10", SSLMode: "disable"},
		"disable, loopback name": {Host: "127.example.com", SSLMode: "disable"},
		"prefer, loopback":       {Host: "127.0.0.1", SSLMode: "prefer"},
		"allow, network host":    {Host: "the-well", SSLMode: "allow"},
		"empty":                  {Host: "postgres", SSLMode: ""},
		"unknown":                {Host: "postgres", SSLMode: "on"},
	} {
		t.Run(name, func(t *testing.T) {
			cfg.Port, cfg.User, cfg.Password, cfg.DBName = "1", "ops_writer", "p", "unheaded_ops"
			if _, err := Connect(context.Background(), cfg); !errors.Is(err, ErrWeakTLS) {
				t.Fatalf("got %v, want ErrWeakTLS (before any connection attempt)", err)
			}
		})
	}
	for name, cfg := range map[string]Config{
		"require":            {Host: "postgres", SSLMode: "require"},
		"verify-full":        {Host: "the-well", SSLMode: "verify-full"},
		"verify-ca":          {Host: "10.0.0.5", SSLMode: "verify-ca"},
		"disable, 127.0.0.1": {Host: "127.0.0.1", SSLMode: "disable"},
		"disable, ::1":       {Host: "::1", SSLMode: "disable"},
		"disable, localhost": {Host: "localhost", SSLMode: "disable"},
		"disable, socket":    {Host: "/var/run/postgresql", SSLMode: "disable"},
	} {
		t.Run("allowed "+name, func(t *testing.T) {
			if err := CheckTLS(cfg); err != nil {
				t.Fatalf("CheckTLS refused %+v: %v", cfg, err)
			}
		})
	}
}

func unsetenv(t *testing.T, k string) {
	t.Helper()
	if err := os.Unsetenv(k); err != nil {
		t.Fatal(err)
	}
}
