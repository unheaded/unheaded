// SPDX-License-Identifier: GPL-3.0-or-later
package database

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"time"

	_ "github.com/lib/pq"
)

// Config holds PostgreSQL connection configuration.
type Config struct {
	Host     string
	Port     int
	User     string
	Password string
	DBName   string
	SSLMode  string
}

// DefaultConfig returns config from environment or sensible defaults.
func DefaultConfig() Config {
	return Config{
		Host:     getEnv("POSTGRES_HOST", "localhost"),
		Port:     5432,
		User:     getEnv("POSTGRES_USER", "unheaded"),
		Password: getEnv("POSTGRES_PASSWORD", "unheaded_dev"),
		DBName:   getEnv("POSTGRES_DB", "unheaded"),
		SSLMode:  getEnv("POSTGRES_SSLMODE", "disable"),
	}
}

// ---------------------------------------------------------------------------
// Multi-database factory configs — The Well v2
// ---------------------------------------------------------------------------

// AppKanbanConfig returns config for the kanban service (unheaded_app).
func AppKanbanConfig() Config {
	return Config{
		Host:     getEnv("POSTGRES_HOST", "localhost"),
		Port:     5432,
		User:     getEnv("APP_KANBAN_USER", "app_kanban"),
		Password: getEnv("APP_KANBAN_PASSWORD", "kanban_dev"),
		DBName:   "unheaded_app",
		SSLMode:  getEnv("POSTGRES_SSLMODE", "disable"),
	}
}

// AppTimeGuruConfig returns config for the timeguru service (unheaded_app).
func AppTimeGuruConfig() Config {
	return Config{
		Host:     getEnv("POSTGRES_HOST", "localhost"),
		Port:     5432,
		User:     getEnv("APP_TIMEGURU_USER", "app_timeguru"),
		Password: getEnv("APP_TIMEGURU_PASSWORD", "timeguru_dev"),
		DBName:   "unheaded_app",
		SSLMode:  getEnv("POSTGRES_SSLMODE", "disable"),
	}
}

// AppZhenConfig returns config for the Zhen AI assistant (unheaded_app).
func AppZhenConfig() Config {
	return Config{
		Host:     getEnv("POSTGRES_HOST", "localhost"),
		Port:     5432,
		User:     getEnv("APP_ZHEN_USER", "app_zhen"),
		Password: getEnv("APP_ZHEN_PASSWORD", "zhen_dev"),
		DBName:   "unheaded_app",
		SSLMode:  getEnv("POSTGRES_SSLMODE", "disable"),
	}
}

// OpsWriterConfig returns config for services that write operational data (unheaded_ops).
func OpsWriterConfig() Config {
	return Config{
		Host:     getEnv("POSTGRES_HOST", "localhost"),
		Port:     5432,
		User:     getEnv("OPS_WRITER_USER", "ops_writer"),
		Password: getEnv("OPS_WRITER_PASSWORD", "ops_writer_dev"),
		DBName:   "unheaded_ops",
		SSLMode:  getEnv("POSTGRES_SSLMODE", "disable"),
	}
}

// OpsReaderConfig returns config for services that read operational data (unheaded_ops).
func OpsReaderConfig() Config {
	return Config{
		Host:     getEnv("POSTGRES_HOST", "localhost"),
		Port:     5432,
		User:     getEnv("OPS_READER_USER", "ops_reader"),
		Password: getEnv("OPS_READER_PASSWORD", "ops_reader_dev"),
		DBName:   "unheaded_ops",
		SSLMode:  getEnv("POSTGRES_SSLMODE", "disable"),
	}
}

// ConfigAdminConfig returns config for services that manage kingdom config (unheaded_config).
func ConfigAdminConfig() Config {
	return Config{
		Host:     getEnv("POSTGRES_HOST", "localhost"),
		Port:     5432,
		User:     getEnv("CONFIG_ADMIN_USER", "config_admin"),
		Password: getEnv("CONFIG_ADMIN_PASSWORD", "config_admin_dev"),
		DBName:   "unheaded_config",
		SSLMode:  getEnv("POSTGRES_SSLMODE", "disable"),
	}
}

// ConfigReaderConfig returns config for services that read kingdom config (unheaded_config).
func ConfigReaderConfig() Config {
	return Config{
		Host:     getEnv("POSTGRES_HOST", "localhost"),
		Port:     5432,
		User:     getEnv("CONFIG_READER_USER", "config_reader"),
		Password: getEnv("CONFIG_READER_PASSWORD", "config_reader_dev"),
		DBName:   "unheaded_config",
		SSLMode:  getEnv("POSTGRES_SSLMODE", "disable"),
	}
}

// DSN returns a PostgreSQL connection string.
func (c Config) DSN() string {
	return fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=%s",
		c.Host, c.Port, c.User, c.Password, c.DBName, c.SSLMode)
}

// Connect establishes a connection to PostgreSQL with retry logic.
func Connect(ctx context.Context, cfg Config) (*sql.DB, error) {
	db, err := sql.Open("postgres", cfg.DSN())
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(5 * time.Minute)

	// Retry ping up to 5 times
	for i := 0; i < 5; i++ {
		if err = db.PingContext(ctx); err == nil {
			return db, nil
		}
		time.Sleep(time.Duration(i+1) * time.Second)
	}

	return nil, fmt.Errorf("ping database after 5 retries: %w", err)
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
