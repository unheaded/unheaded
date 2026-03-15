// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

package database

import (
	"fmt"
	"os"
)

// Config holds PostgreSQL connection parameters.
type Config struct {
	Host     string
	Port     string
	User     string
	Password string
	DBName   string
	SSLMode  string
}

// DSN returns the PostgreSQL connection string.
func (c Config) DSN() string {
	return fmt.Sprintf(
		"host=%s port=%s user=%s password=%s dbname=%s sslmode=%s",
		c.Host, c.Port, c.User, c.Password, c.DBName, c.SSLMode,
	)
}

// OpsWriterConfig returns the database config for the dashboard-backend
// (operational writes: health reports, transitions, hourly stats).
// Reads from environment variables with sensible defaults.
func OpsWriterConfig() Config {
	return Config{
		Host:     getEnv("WELL_HOST", "localhost"),
		Port:     getEnv("WELL_PORT", "5432"),
		User:     getEnv("WELL_USER", "unheaded"),
		Password: getEnv("WELL_PASSWORD", "unheaded"),
		DBName:   getEnv("WELL_DB", "the_well"),
		SSLMode:  getEnv("WELL_SSLMODE", "disable"),
	}
}

// AppKanbanConfig returns the database config for the kanban-app
// (task CRUD against the kanban_tasks table).
func AppKanbanConfig() Config {
	return Config{
		Host:     getEnv("WELL_HOST", "localhost"),
		Port:     getEnv("WELL_PORT", "5432"),
		User:     getEnv("WELL_USER", "unheaded"),
		Password: getEnv("WELL_PASSWORD", "unheaded"),
		DBName:   getEnv("WELL_DB", "the_well"),
		SSLMode:  getEnv("WELL_SSLMODE", "disable"),
	}
}

func getEnv(key, fallback string) string {
	if v, ok := os.LookupEnv(key); ok {
		return v
	}
	return fallback
}
