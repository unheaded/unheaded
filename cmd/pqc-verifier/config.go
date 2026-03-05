// SPDX-License-Identifier: MIT
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

package main

import (
	"os"
	"strconv"
	"time"

	"unheaded/pkg/ports"
)

// Config holds the PQC verifier daemon configuration.
type Config struct {
	Port            int
	WotanAddr       string
	ShutdownTimeout time.Duration
	LogLevel        string
	PolicyFile      string
	DefaultTier     uint8
	AuthEnabled     bool
}

// DefaultConfig returns the default configuration.
func DefaultConfig() Config {
	return Config{
		Port:            ports.PQCVerifier,
		WotanAddr:       ports.DefaultWotanGRPC(),
		ShutdownTimeout: 10 * time.Second,
		LogLevel:        "info",
		DefaultTier:     0x01, // STANDARD
		AuthEnabled:     false,
	}
}

// LoadFromEnv overrides config values from environment variables.
func (c *Config) LoadFromEnv() {
	if v := os.Getenv("PQC_VERIFIER_PORT"); v != "" {
		if p, err := strconv.Atoi(v); err == nil {
			c.Port = p
		}
	}
	if v := os.Getenv("WOTAN_ADDR"); v != "" {
		c.WotanAddr = v
	}
	if v := os.Getenv("PQC_LOG_LEVEL"); v != "" {
		c.LogLevel = v
	}
	if v := os.Getenv("PQC_POLICY_FILE"); v != "" {
		c.PolicyFile = v
	}
	if v := os.Getenv("PQC_DEFAULT_TIER"); v != "" {
		if t, err := strconv.Atoi(v); err == nil {
			c.DefaultTier = uint8(t)
		}
	}
	if v := os.Getenv("PQC_AUTH_ENABLED"); v == "true" || v == "1" {
		c.AuthEnabled = true
	}
}
