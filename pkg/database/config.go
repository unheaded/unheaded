// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package database

import (
	"crypto/x509"
	"errors"
	"fmt"
	"net/netip"
	"os"
	"strings"
)

// ErrMissingCredentials is returned by Connect when no user or password is
// configured. There is deliberately no built-in credential (FND-007): a
// shipped fallback password is public, and it turns a missing setting into
// a silent superuser login attempt instead of a startup error.
var ErrMissingCredentials = errors.New("database credentials not set (credentials are not stored in this repo)")

// ErrWeakTLS is returned by Connect (and CheckTLS) for an sslmode that
// could carry the session in plaintext to another host. The Well accepts
// TLS only (db/pg_hba.conf, ERDA 4.2), so a server that answers without it
// over the network is not The Well; allow and prefer would fall back to
// plaintext without a word.
var ErrWeakTLS = errors.New("database sslmode must be require, verify-ca or verify-full (disable only on this host)")

// ErrMissingCA is returned by Connect (and CheckTLS) when the client was told
// to verify the server and has no usable CA. Without it verify-ca and
// verify-full would judge a private CA's certificate against whatever roots
// the host holds, and require with a missing file would silently verify nothing.
var ErrMissingCA = errors.New("database CA unusable: sslmode verify-ca and verify-full need WELL_SSLROOTCERT, a readable PEM file")

// CheckTLS refuses an sslmode that would let the session run without TLS
// to anything but this host: allow and prefer anywhere, disable unless the
// host is a loopback address, localhost or a Unix socket directory. It also
// refuses a client told to verify the server that has no usable CA.
func CheckTLS(c Config) error {
	switch c.SSLMode {
	case "require", "verify-ca", "verify-full":
		return checkCA(c)
	case "disable":
		if isLocalHost(c.Host) {
			return checkCA(c)
		}
	}
	return fmt.Errorf("%w: sslmode %q for host %q", ErrWeakTLS, c.SSLMode, c.Host)
}

// checkCA loads the configured CA the way the driver will. A path that is set
// is always checked, because it means someone expects verification.
func checkCA(c Config) error {
	if c.SSLRootCert == "" {
		if c.SSLMode == "verify-ca" || c.SSLMode == "verify-full" {
			return fmt.Errorf("%w: sslmode %q with no CA path", ErrMissingCA, c.SSLMode)
		}
		return nil
	}
	data, err := os.ReadFile(c.SSLRootCert) // #nosec G304 -- operator-set path of a public certificate
	if err != nil {
		return fmt.Errorf("%w: %v", ErrMissingCA, err)
	}
	if !x509.NewCertPool().AppendCertsFromPEM(data) {
		return fmt.Errorf("%w: no certificate in %s", ErrMissingCA, c.SSLRootCert)
	}
	return nil
}

func isLocalHost(host string) bool {
	if host == "localhost" || strings.HasPrefix(host, "/") || strings.HasPrefix(host, "@") {
		return true
	}
	ip, err := netip.ParseAddr(host)
	return err == nil && ip.Unmap().IsLoopback()
}

// Config holds PostgreSQL connection parameters.
type Config struct {
	Host     string
	Port     string
	User     string
	Password string
	DBName   string
	SSLMode  string
	// The CA that signed the server's certificate (WELL_SSLROOTCERT), as a
	// file path. Empty means none is configured.
	SSLRootCert string
}

// DSN returns the PostgreSQL connection string. Every value is quoted, so
// a space or quote in one (a password, say) cannot add or override a
// keyword such as sslmode.
func (c Config) DSN() string {
	dsn := fmt.Sprintf(
		"host=%s port=%s user=%s password=%s dbname=%s sslmode=%s",
		quoteDSN(c.Host), quoteDSN(c.Port), quoteDSN(c.User), quoteDSN(c.Password), quoteDSN(c.DBName), quoteDSN(c.SSLMode),
	)
	if c.SSLRootCert != "" {
		dsn += " sslrootcert=" + quoteDSN(c.SSLRootCert)
	}
	return dsn
}

// quoteDSN quotes a libpq keyword/value: single quotes around it, with
// backslash and single quote escaped by a backslash.
func quoteDSN(v string) string {
	return "'" + strings.NewReplacer(`\`, `\\`, `'`, `\'`).Replace(v) + "'"
}

// OpsWriterConfig returns the database config for the dashboard-backend
// (operational writes: health reports, transitions, hourly stats).
// Reads from environment variables. The default user is the scoped role
// ops_writer, never the superuser, and there is no default password.
func OpsWriterConfig() Config {
	return Config{
		Host:        getEnv("WELL_HOST", "localhost"),
		Port:        getEnv("WELL_PORT", "5432"),
		User:        getEnv("WELL_USER", "ops_writer"),
		Password:    os.Getenv("WELL_PASSWORD"),
		DBName:      getEnv("WELL_DB", "unheaded_ops"),
		SSLMode:     getEnv("WELL_SSLMODE", "require"),
		SSLRootCert: os.Getenv("WELL_SSLROOTCERT"),
	}
}

// AppKanbanConfig returns the database config for the kanban-app
// (task CRUD against the kanban_tasks table). The default user is the
// scoped role app_kanban; there is no default password.
func AppKanbanConfig() Config {
	return Config{
		Host:        getEnv("WELL_HOST", "localhost"),
		Port:        getEnv("WELL_PORT", "5432"),
		User:        getEnv("WELL_USER", "app_kanban"),
		Password:    os.Getenv("WELL_PASSWORD"),
		DBName:      getEnv("WELL_DB", "unheaded_app"),
		SSLMode:     getEnv("WELL_SSLMODE", "require"),
		SSLRootCert: os.Getenv("WELL_SSLROOTCERT"),
	}
}

// AppAuthConfig returns the database config for API-key stores (pkg/auth
// WellStore, unheaded-cli apikey): the scoped role app_auth on
// unheaded_app's api_keys table (migrations 035/036). The login is its
// own, AUTH_WELL_USER / AUTH_WELL_PASSWORD / AUTH_WELL_DB, never the
// service's WELL_USER/WELL_PASSWORD/WELL_DB: those belong to the role the
// service otherwise uses (app_kanban, ops_writer on unheaded_ops). Host,
// port, sslmode and CA are the same Well's (WELL_HOST, WELL_PORT,
// WELL_SSLMODE, WELL_SSLROOTCERT). There is no default password.
func AppAuthConfig() Config {
	return Config{
		Host:        getEnv("WELL_HOST", "localhost"),
		Port:        getEnv("WELL_PORT", "5432"),
		User:        getEnv("AUTH_WELL_USER", "app_auth"),
		Password:    os.Getenv("AUTH_WELL_PASSWORD"),
		DBName:      getEnv("AUTH_WELL_DB", "unheaded_app"),
		SSLMode:     getEnv("WELL_SSLMODE", "require"),
		SSLRootCert: os.Getenv("WELL_SSLROOTCERT"),
	}
}

// LogWriterConfig returns the database config for cmd/logstore-writer: the
// scoped role log_writer on unheaded_ops, which may INSERT into log_events
// and call log_events_maintain() and nothing else (migration 048). There is
// no default password.
func LogWriterConfig() Config {
	return Config{
		Host:        getEnv("WELL_HOST", "localhost"),
		Port:        getEnv("WELL_PORT", "5432"),
		User:        getEnv("WELL_USER", "log_writer"),
		Password:    os.Getenv("WELL_PASSWORD"),
		DBName:      getEnv("WELL_DB", "unheaded_ops"),
		SSLMode:     getEnv("WELL_SSLMODE", "require"),
		SSLRootCert: os.Getenv("WELL_SSLROOTCERT"),
	}
}

func getEnv(key, fallback string) string {
	if v, ok := os.LookupEnv(key); ok {
		return v
	}
	return fallback
}
