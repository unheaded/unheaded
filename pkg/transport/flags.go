// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package transport

import (
	"flag"
	"os"
	"strings"
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
	// WOTAN_ADDR is the convention docker-compose actually sets (host:port).
	// ConfigFromEnv only read WOTAN_HTTP_ADDR, which nothing sets, so the
	// HTTP address stayed at the localhost default inside every container —
	// where localhost is nobody. Services that worked did so only because
	// their main also passed a --wotan flag.
	//
	// WOTAN_HTTP_ADDR stays authoritative when both are set: it is the
	// explicit one.
	if v := os.Getenv("WOTAN_ADDR"); v != "" {
		cfg.WotanHTTPAddr = normalizeHTTPAddr(v)
	}
	if v := os.Getenv("WOTAN_HTTP_ADDR"); v != "" {
		cfg.WotanHTTPAddr = normalizeHTTPAddr(v)
	}
	if v := os.Getenv("CONNECT_TIMEOUT"); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			cfg.ConnectTimeout = d
		}
	}
}

// normalizeHTTPAddr accepts either a bare host:port or a full URL and returns
// a URL. Callers set these by hand in compose files and both spellings appear.
func normalizeHTTPAddr(v string) string {
	if strings.HasPrefix(v, "http://") || strings.HasPrefix(v, "https://") {
		return v
	}
	return "http://" + v
}

// FlagWasSet reports whether the named flag was given on the command line,
// as opposed to sitting at its default.
//
// Needed because a flag's default silently outranks an environment variable
// when code does `cfg.X = *flagX` unconditionally: services were overwriting
// WOTAN_ADDR from compose with the "localhost:18000" default and then failing
// to reach Wotan from inside a container. Correct precedence is
// default < environment < explicitly-passed flag.
func FlagWasSet(name string) bool {
	set := false
	flag.Visit(func(f *flag.Flag) {
		if f.Name == name {
			set = true
		}
	})
	return set
}
