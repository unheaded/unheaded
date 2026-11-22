// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

// Package lifecycle holds WaitForShutdown: block until SIGINT or SIGTERM, shut
// the HTTP server down within a timeout, then run cleanup functions. No
// binary imports it today.
package lifecycle

import (
	"context"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/rs/zerolog/log"
)

// WaitForShutdown blocks until SIGINT or SIGTERM is received, then gracefully
// shuts down the HTTP server and runs any cleanup functions.
// This replaces the identical shutdown boilerplate each service carried.
func WaitForShutdown(server *http.Server, timeout time.Duration, cleanup ...func()) {
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	sig := <-sigCh
	log.Info().Str("signal", sig.String()).Msg("shutdown signal received")

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		log.Error().Err(err).Msg("server shutdown error")
	}

	for _, fn := range cleanup {
		fn()
	}

	log.Info().Msg("service stopped")
}
