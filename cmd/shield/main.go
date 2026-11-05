// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis. All rights reserved.

// Command shield runs the Kingdom's Shield WAF / Zero-Trust gateway daemon.
//
// Shield (services/shield) ships as both a library (used as middleware
// inside other Go services) and as a standalone HTTP daemon (this binary).
// The daemon exposes:
//
//	GET /health   — liveness
//	GET /ready    — readiness
//	GET /metrics  — Prometheus metrics
//	GET /api/v1/rules            — list active WAF rules
//	POST /api/v1/rules           — add a WAF rule (JSON body: shield.Rule)
//	DELETE /api/v1/rules/<id>    — remove a WAF rule
//	POST /api/v1/evaluate        — evaluate a request against the rule set
//
// Default bind: 127.0.0.1 on pkg/ports.Shield (19009). Override the port via
// -port or SHIELD_PORT, the address via -listen. Off loopback needs
// AUTH_ENABLED=true; with it, every route but /health, /ready and /metrics
// needs a credential.
//
// Wotan integration: rule mutations + threat events publish to topic
// "shield.events" (configurable via Service.Config.WotanTopic). When the
// daemon can't reach Wotan, it logs a warning and continues without
// distributed rule sync — the local rule set still enforces.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"unheaded/pkg/auth"
	"unheaded/pkg/cmdline"
	"unheaded/pkg/httputil"
	"unheaded/pkg/logger"
	"unheaded/pkg/ports"
	wotanClient "unheaded/pkg/wotan-client"
	"unheaded/services/shield"
)

func main() {
	var (
		port        = flag.Int("port", ports.Shield, "HTTP port to bind on loopback")
		listen      = flag.String("listen", "", "HTTP listen address, overriding --port; off loopback needs AUTH_ENABLED=true")
		wotanAddr   = flag.String("wotan-grpc", "127.0.0.1:18001", "Wotan gRPC address")
		shutTimeout = flag.Duration("shutdown-timeout", 10*time.Second, "graceful shutdown deadline")
	)
	cmdline.Parse(cmdline.Spec{
		Name:     "shield",
		Purpose:  "Run Shield, the Kingdom's WAF / Zero-Trust gateway daemon.",
		Examples: []string{"shield", "shield --port 19007"},
	})
	wotanClient.InitServiceIdentity("shield") // ADR-103 §6: every Wotan call carries shield's service token

	if env := os.Getenv("SHIELD_PORT"); env != "" {
		var p int
		if _, err := fmt.Sscanf(env, "%d", &p); err == nil && p > 0 && p < 65536 {
			*port = p
		}
	}

	log := logger.New(os.Stdout)
	log.Info().Int("port", *port).Str("wotan", *wotanAddr).Msg("shield daemon starting")

	// Wotan is non-fatal: shield runs in degraded mode without distributed
	// rule sync if Wotan is unreachable. Match the dashboard-backend posture.
	wcli, werr := wotanClient.NewClient(*wotanAddr)
	if werr != nil {
		log.Warn().Err(werr).Msg("wotan unreachable — shield running in standalone mode (no distributed rule sync)")
		wcli = nil
	}

	svc := shield.NewService(log, wcli, nil)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := svc.Start(ctx); err != nil {
		log.Error().Err(err).Msg("shield service failed to start")
		os.Exit(1)
	}

	mux := http.NewServeMux()
	svc.RegisterRoutes(mux) // /health, /ready, /metrics from services/shield/shield.go
	registerControlAPI(mux, svc, log)

	addr := listenAddr(*listen, *port)
	handler, err := newHandler(mux, addr, auth.LoadServiceAuthConfig("shield"))
	if err != nil {
		log.Error().Err(err).Msg("refusing to start")
		os.Exit(1)
	}
	httpSrv := &http.Server{
		Addr:              addr,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
		MaxHeaderBytes:    1 << 20, // CLAUDE.md hardening baseline
	}

	go func() {
		log.Info().Str("addr", addr).Msg("shield http listening")
		if err := httpSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Error().Err(err).Msg("http server crashed")
			os.Exit(1)
		}
	}()

	// Wait for SIGINT / SIGTERM, then drain.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	sig := <-sigCh
	log.Info().Str("signal", sig.String()).Msg("shield shutdown requested")

	shutCtx, shutCancel := context.WithTimeout(context.Background(), *shutTimeout)
	defer shutCancel()
	if err := httpSrv.Shutdown(shutCtx); err != nil {
		log.Warn().Err(err).Msg("http shutdown returned error")
	}
	if err := svc.Stop(); err != nil {
		log.Warn().Err(err).Msg("shield service stop returned error")
	}
	log.Info().Msg("shield daemon stopped cleanly")
}

// registerControlAPI wires the rule-management and request-evaluate
// endpoints. Kept here (not in services/shield) because it's the
// daemon's HTTP contract — the library form has its own embedding
// patterns and shouldn't impose this routing on importers.
func registerControlAPI(mux *http.ServeMux, svc *shield.Service, log *logger.Logger) {
	mux.HandleFunc("/api/v1/rules", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			rules := svc.ListRules()
			writeJSON(w, http.StatusOK, map[string]any{"rules": rules, "count": len(rules)})
		case http.MethodPost:
			var rule shield.Rule
			if code, err := httputil.ReadJSON(w, r, maxBodyBytes, &rule); err != nil {
				writeJSON(w, code, map[string]any{"error": "invalid JSON: " + err.Error()})
				return
			}
			if err := svc.AddRule(&rule); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
				return
			}
			log.Info().Str("rule_id", rule.ID).Str("name", rule.Name).Msg("rule added via api")
			writeJSON(w, http.StatusCreated, map[string]any{"rule": rule})
		default:
			w.Header().Set("Allow", "GET, POST")
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	})

	mux.HandleFunc("/api/v1/rules/", func(w http.ResponseWriter, r *http.Request) {
		// Path: /api/v1/rules/<id>
		id := strings.TrimPrefix(r.URL.Path, "/api/v1/rules/")
		if id == "" || strings.Contains(id, "/") {
			http.Error(w, "rule id required: /api/v1/rules/<id>", http.StatusBadRequest)
			return
		}
		if r.Method != http.MethodDelete {
			w.Header().Set("Allow", "DELETE")
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if err := svc.RemoveRule(id); err != nil {
			writeJSON(w, http.StatusNotFound, map[string]any{"error": err.Error()})
			return
		}
		log.Info().Str("rule_id", id).Msg("rule removed via api")
		writeJSON(w, http.StatusOK, map[string]any{"removed": id})
	})

	mux.HandleFunc("/api/v1/evaluate", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.Header().Set("Allow", "POST")
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var req shield.Request
		if code, err := httputil.ReadJSON(w, r, maxBodyBytes, &req); err != nil {
			writeJSON(w, code, map[string]any{"error": "invalid JSON: " + err.Error()})
			return
		}
		decision := svc.Evaluate(r.Context(), &req)
		writeJSON(w, http.StatusOK, decision)
	})
}

// maxBodyBytes bounds a rule or evaluate request.
const maxBodyBytes = 1 << 20

// listenAddr is --listen, or loopback on port when it is empty.
func listenAddr(listen string, port int) string {
	if listen != "" {
		return listen
	}
	return ports.LoopbackAddr(port)
}

// newHandler puts h behind pkg/auth (with AUTH_ENABLED=true: a token for
// every route but /health, /ready and /metrics, and a write role for the
// rule routes). Auth off is for loopback only; a listener off loopback with
// auth off, or auth on with nothing to verify against, is an error.
func newHandler(h http.Handler, addr string, cfg auth.ServiceAuthConfig) (http.Handler, error) {
	if err := auth.CheckListen([]string{addr}, cfg); err != nil {
		return nil, err
	}
	return auth.NewServiceHandler(h, cfg, auth.ServiceRoutes{})
}

func writeJSON(w http.ResponseWriter, code int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(body)
}
