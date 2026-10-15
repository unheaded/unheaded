// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package database

import (
	"errors"
	"fmt"
	"os"
	"strings"
)

// ErrMissingCredentials is returned by Connect when no user or password is
// configured. There is deliberately no built-in credential (FND-007): a
// shipped fallback password is public, and it turns a missing setting into
// a silent superuser login attempt instead of a startup error.
var ErrMissingCredentials = errors.New("database credentials not set (credentials are not stored in this repo)")

// Config holds PostgreSQL connection parameters.
type Config struct {
	Host     string
	Port     string
	User     string
	Password string
	DBName   string
	SSLMode  string
}

// DSN returns the PostgreSQL connection string. Every value is quoted, so
// a space or quote in one (a password, say) cannot add or override a
// keyword such as sslmode.
func (c Config) DSN() string {
	return fmt.Sprintf(
		"host=%s port=%s user=%s password=%s dbname=%s sslmode=%s",
		quoteDSN(c.Host), quoteDSN(c.Port), quoteDSN(c.User), quoteDSN(c.Password), quoteDSN(c.DBName), quoteDSN(c.SSLMode),
	)
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
		Host:     getEnv("WELL_HOST", "localhost"),
		Port:     getEnv("WELL_PORT", "5432"),
		User:     getEnv("WELL_USER", "ops_writer"),
		Password: os.Getenv("WELL_PASSWORD"),
		DBName:   getEnv("WELL_DB", "unheaded_ops"),
		SSLMode:  getEnv("WELL_SSLMODE", "require"),
	}
}

// AppKanbanConfig returns the database config for the kanban-app
// (task CRUD against the kanban_tasks table). The default user is the
// scoped role app_kanban; there is no default password.
func AppKanbanConfig() Config {
	return Config{
		Host:     getEnv("WELL_HOST", "localhost"),
		Port:     getEnv("WELL_PORT", "5432"),
		User:     getEnv("WELL_USER", "app_kanban"),
		Password: os.Getenv("WELL_PASSWORD"),
		DBName:   getEnv("WELL_DB", "unheaded_app"),
		SSLMode:  getEnv("WELL_SSLMODE", "require"),
	}
}

func getEnv(key, fallback string) string {
	if v, ok := os.LookupEnv(key); ok {
		return v
	}
	return fallback
}
