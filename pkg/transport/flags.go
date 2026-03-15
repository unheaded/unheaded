// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

package transport

import (
	"flag"
	"os"
	"time"
)

// RegisterFlags adds standard transport CLI flags to the given FlagSet.
// Every service calls RegisterFlags() to get consistent --transport,
// --wotan-grpc-addr, --wotan-http-addr flags.
func RegisterFlags(fs *flag.FlagSet, cfg *Config) {
	fs.StringVar((*string)(&cfg.Type), "transport", string(cfg.Type), "Transport type: grpc, http, auto (default: auto)")
	fs.StringVar(&cfg.WotanGRPCAddr, "wotan-grpc-addr", cfg.WotanGRPCAddr, "Wotan gRPC address")
	fs.StringVar(&cfg.WotanHTTPAddr, "wotan-http-addr", cfg.WotanHTTPAddr, "Wotan HTTP address")
	fs.DurationVar(&cfg.ConnectTimeout, "connect-timeout", cfg.ConnectTimeout, "Transport connect timeout")
	fs.BoolVar(&cfg.FallbackEnabled, "fallback", cfg.FallbackEnabled, "Enable transport fallback")
}

// ConfigFromEnv reads transport configuration from environment variables.
// Environment variables take precedence over compiled defaults but are
// overridden by command-line flags.
func ConfigFromEnv(cfg *Config) {
	if v := os.Getenv("TRANSPORT_TYPE"); v != "" {
		cfg.Type = Type(v)
	}
	if v := os.Getenv("WOTAN_GRPC_ADDR"); v != "" {
		cfg.WotanGRPCAddr = v
	}
	if v := os.Getenv("WOTAN_HTTP_ADDR"); v != "" {
		cfg.WotanHTTPAddr = v
	}
	if v := os.Getenv("CONNECT_TIMEOUT"); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			cfg.ConnectTimeout = d
		}
	}
}
