// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

// PQC Verifier Daemon — standalone post-quantum cryptographic verification
// service for the Unheaded platform. Listens on port 19008 (Doom Range).
//
// HTTP API:
//   - POST /verify            — verify a PQC-authenticated packet
//   - POST /verify/sovereign  — verify 2-of-3 sovereign multi-sig
//   - GET  /health            — health check
//   - GET  /ready             — readiness probe
//   - GET  /metrics           — verification metrics
package main

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"unheaded/pkg/policy"
	"unheaded/pkg/ports"
	"unheaded/pkg/wotan"
	"unheaded/services/shield"
)

func main() {
	if err := run(os.Stdout, os.Args); err != nil {
		fmt.Fprintf(os.Stderr, "pqc-verifier: %v\n", err)
		os.Exit(1)
	}
}

func run(logOut io.Writer, _ []string) error {
	cfg := DefaultConfig()
	cfg.LoadFromEnv()

	fmt.Fprintf(logOut, "pqc-verifier: starting on port %d\n", cfg.Port)

	// Initialize verification components.
	verifier := shield.NewPQCVerifier()
	sovereign := shield.NewSovereignVerifier(verifier)
	policyEng := shield.NewLayer2PolicyEngine()
	stateMgr := wotan.NewPQCStateManager()

	// Load policy file if configured.
	if cfg.PolicyFile != "" {
		data, err := os.ReadFile(cfg.PolicyFile)
		if err != nil {
			return fmt.Errorf("pqc-verifier: load policy: %w", err)
		}
		if err := policyEng.LoadFromYAML(data); err != nil {
			return fmt.Errorf("pqc-verifier: parse policy: %w", err)
		}
		fmt.Fprintf(logOut, "pqc-verifier: loaded %d policies from %s\n", policyEng.PolicyCount(), cfg.PolicyFile)
	}

	// Create tier verifier.
	tierVerifier := shield.NewTierVerifier(verifier, sovereign, policyEng, stateMgr)
	tierVerifier.TransitionTier(policy.ComplianceTier(cfg.DefaultTier))

	// Set downgrade warning callback.
	tierVerifier.SetDowngradeCallback(func(from, to policy.ComplianceTier) {
		fmt.Fprintf(logOut, "pqc-verifier: WARNING: tier downgrade %s → %s\n", from, to)
	})

	// Event publisher (Wotan integration).
	publisher := NewEventPublisher(nil) // nil pub = buffer-only until Wotan connected

	// Wotan event handler.
	handler := NewWotanEventHandler(logOut)
	handler.SetSigCreatedHandler(func(data []byte) {
		fmt.Fprintf(logOut, "pqc-verifier: sig.created event received (%d bytes)\n", len(data))
	})
	handler.SetKeyEventHandler(func(data []byte) {
		fmt.Fprintf(logOut, "pqc-verifier: key event received (%d bytes)\n", len(data))
	})
	handler.SetPolicyChangedHandler(func(data []byte) {
		fmt.Fprintf(logOut, "pqc-verifier: policy.changed event received (%d bytes)\n", len(data))
	})

	// HTTP server.
	mux := http.NewServeMux()
	handlers := NewHandlers(tierVerifier, publisher)
	handlers.RegisterRoutes(mux)

	addr := ports.DefaultAddr(cfg.Port)
	srv := &http.Server{
		Addr:           addr,
		Handler:        mux,
		MaxHeaderBytes: 1 << 20,
	}

	// Graceful shutdown.
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	errCh := make(chan error, 1)
	go func() {
		fmt.Fprintf(logOut, "pqc-verifier: listening on %s\n", addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errCh <- err
		}
	}()

	select {
	case <-ctx.Done():
		fmt.Fprintf(logOut, "pqc-verifier: shutting down...\n")
		handlers.SetReady(false)

		shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
		defer cancel()

		if err := srv.Shutdown(shutdownCtx); err != nil {
			return fmt.Errorf("pqc-verifier: shutdown: %w", err)
		}

		// Zero sensitive state.
		stateMgr.Write(wotan.PQCState{})
		fmt.Fprintf(logOut, "pqc-verifier: stopped cleanly\n")
		return nil

	case err := <-errCh:
		return err
	}
}
