// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

// sword is the Kingdom's deploy service (services/sword): rolling,
// blue-green, canary and recreate deployments into LXD through Cuirass,
// with canary analysis from the instances' own metrics and the Kingdom SLO
// guard, and rollbacks to the previous success.
//
// Environment:
//
//	HTTP_ADDR         listen address (default :19011, pkg/ports.Sword)
//	CUIRASS_URL       Cuirass base URL (default http://127.0.0.1:19006)
//	AUTH_KEY_FILE     Sword's Ed25519 key: writes to Cuirass carry a service
//	                  token minted from it (aud unheaded-daemon, ADR-103)
//	CUIRASS_API_KEY   fallback when there is no AUTH_KEY_FILE; Sword refuses
//	                  to start with neither
//	SWORD_DATA_DIR    deployment history file when WELL_DB is unset; with
//	                  neither, read-only (writes 503)
//	WELL_DB           The Well: history in unheaded_ops.deploy_history
//	                  (migration 039) as WELL_USER (default ops_writer) with
//	                  WELL_PASSWORD, WELL_HOST, WELL_PORT, WELL_SSLMODE
//	                  (database.OpsWriterConfig). Set and unreachable, or 039
//	                  missing, Sword refuses to start: it never falls back to
//	                  the file. The first start on an empty Well imports
//	                  SWORD_DATA_DIR's file.
//	KINGDOM_NODE      this host's name in the Well (default: the hostname)
//	VM_URL            VictoriaMetrics for the SLO guard; unset = canary refused
//	SWORD_SLO_POLICY  alerting policy file (default /etc/unheaded/sword/policy.yaml)
//	WOTAN_ADDR, WOTAN_GRPC_ADDR  Wotan, for deploy.* events (best effort)
//	AUTH_ENABLED=true, AUTH_TRUST_DIR, AUTH_ALLOW_CALLERS, AUTH_API_KEYS
//	                  pkg/auth (ADR-103; AUTH_JWT_KEY is refused); without
//	                  AUTH_ENABLED every mutating route answers 403
package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/rs/zerolog"

	"unheaded/pkg/auth"
	"unheaded/pkg/database"
	"unheaded/pkg/deploy/cuirassdriver"
	"unheaded/pkg/discovery"
	"unheaded/pkg/healthreport"
	"unheaded/pkg/httputil"
	"unheaded/pkg/ports"
	"unheaded/pkg/sloguard"
	utls "unheaded/pkg/tls"
	"unheaded/pkg/transport"
	wotanClient "unheaded/pkg/wotan-client"
	"unheaded/services/sword"
)

const serviceName = "sword"

type config struct {
	addr       string
	cuirassURL string
	cuirassKey string
	keyFile    string
	dataDir    string
	wellDB     string
	node       string
	vmURL      string
	policy     string
	wotanHTTP  string
	wotanGRPC  string
}

func envOr(getenv func(string) string, k, def string) string {
	if v := getenv(k); v != "" {
		return v
	}
	return def
}

// cuirassAudience is Cuirass's auth identity, the "aud" of Sword's tokens.
const cuirassAudience = "unheaded-daemon"

// loadConfig reads the environment. A missing Cuirass key is an error:
// credentials fail loud, never a default.
func loadConfig(getenv func(string) string) (config, error) {
	cuirassScheme := "http"
	if getenv(utls.EnvEnabled) == "true" { // Cuirass serves TLS too (S-IRON-GATE Phase 8)
		cuirassScheme = "https"
	}
	c := config{
		addr:       envOr(getenv, "HTTP_ADDR", ports.DefaultAddr(ports.Sword)),
		cuirassURL: envOr(getenv, "CUIRASS_URL", fmt.Sprintf("%s://127.0.0.1:%d", cuirassScheme, ports.Cuirass)),
		cuirassKey: getenv("CUIRASS_API_KEY"),
		keyFile:    getenv("AUTH_KEY_FILE"),
		dataDir:    getenv("SWORD_DATA_DIR"),
		wellDB:     getenv("WELL_DB"),
		node:       getenv("KINGDOM_NODE"),
		vmURL:      getenv("VM_URL"),
		policy:     envOr(getenv, "SWORD_SLO_POLICY", "/etc/unheaded/sword/policy.yaml"),
		wotanHTTP:  getenv("WOTAN_ADDR"),
		wotanGRPC:  getenv("WOTAN_GRPC_ADDR"),
	}
	if c.cuirassKey == "" && c.keyFile == "" {
		return c, errors.New("neither AUTH_KEY_FILE nor CUIRASS_API_KEY is set: Sword deploys through Cuirass's write API and credentials are not stored in the repo")
	}
	return c, nil
}

// build wires the service from a config.
func build(c config, log *zerolog.Logger) (*sword.Service, error) {
	dc := cuirassdriver.Config{URL: c.cuirassURL, APIKey: c.cuirassKey}
	// https to Cuirass trusts the deployment CA (UNHEADED_TLS_CA_CERT); no
	// proxy, as the driver's own default.
	tr, err := utls.TransportFromEnv()
	if err != nil {
		return nil, fmt.Errorf("cuirass TLS: %w", err)
	}
	tr.Proxy = nil
	dc.Client = &http.Client{Transport: tr}
	if c.keyFile != "" {
		minter, err := auth.LoadServiceTokenMinter(serviceName, c.keyFile)
		if err != nil {
			return nil, fmt.Errorf("sword key: %w", err)
		}
		dc.Authorizer = func() (string, error) { return minter.AuthorizationHeader(cuirassAudience) }
	}
	d, err := cuirassdriver.New(dc)
	if err != nil {
		return nil, err
	}
	cfg := sword.Config{
		DataDir:     c.dataDir,
		AuthEnabled: auth.LoadServiceAuthConfig(serviceName).Enabled,
		Driver:      d,
		Log:         log,
	}
	if c.wellDB != "" {
		h, err := openWellHistory(c)
		if err != nil {
			return nil, fmt.Errorf("history in the Well (WELL_DB=%s): %w", c.wellDB, err)
		}
		cfg.History = h
	}
	if c.vmURL != "" {
		g, err := sloguard.New(c.vmURL, c.policy)
		if err != nil {
			return nil, fmt.Errorf("SLO guard: %w", err)
		}
		cfg.Analysis = sword.NewAnalyzer(g)
	} else {
		log.Warn().Msg("VM_URL is not set: no SLO guard, so canary deployments are refused")
	}
	if c.dataDir == "" && c.wellDB == "" {
		log.Warn().Msg("neither WELL_DB nor SWORD_DATA_DIR is set: read-only, every deployment request answers 503")
	}
	if !cfg.AuthEnabled {
		log.Warn().Msg("AUTH_ENABLED is not true: every deployment request answers 403")
	}
	if c.wotanHTTP != "" {
		var wc *wotanClient.Client
		if c.wotanGRPC != "" {
			wc, err = wotanClient.NewClientWithGRPC(c.wotanHTTP, c.wotanGRPC)
		} else {
			wc, err = wotanClient.NewClient(c.wotanHTTP)
		}
		if err != nil {
			log.Warn().Err(err).Msg("Wotan client: deploy events off")
		} else {
			cfg.Publisher = sword.NewWotanPublisher(wc)
		}
	}
	return sword.New(cfg)
}

// openWellHistory connects as the ops writer and checks migration 039.
var openWellHistory = func(c config) (sword.History, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	dbCfg := database.OpsWriterConfig()
	dbCfg.DBName = c.wellDB
	db, err := database.Connect(ctx, dbCfg)
	if err != nil {
		return nil, err
	}
	node := c.node
	if node == "" {
		if node, err = os.Hostname(); err != nil {
			return nil, err
		}
	}
	h, err := sword.NewWellHistory(ctx, db, node)
	if err != nil {
		_ = db.Close()
		return nil, err
	}
	return h, nil
}

// handler is Sword's mux with auth from authCfg in front (S-IRON-GATE
// Phase 6). With auth on, /api/v1/deployments needs a token or key: a
// write (deploy, rollback) needs admin, operator or service (the CLI's and
// the dashboard backend's service tokens, an AUTH_API_KEYS key), a read
// also takes observer. DefaultServicePolicies alone would keep everything
// under /api/v1/deploy* for admin and operator. An error means auth is on
// but unusable, and Sword must not start.
func handler(s *sword.Service, authCfg auth.ServiceAuthConfig) (http.Handler, error) {
	mux := s.Handler()
	rbac := auth.DefaultServicePolicies(serviceName)
	rbac.Policies = append([]auth.EndpointPolicy{
		{PathPrefix: "/api/v1/deployments", Methods: []string{http.MethodPost, http.MethodPut, http.MethodDelete, http.MethodPatch},
			RequiredRoles: []string{auth.RoleAdmin, auth.RoleOperator, auth.RoleService}},
		{PathPrefix: "/api/v1/deployments", Methods: []string{http.MethodGet, http.MethodHead},
			RequiredRoles: []string{auth.RoleAdmin, auth.RoleOperator, auth.RoleObserver, auth.RoleService}},
	}, rbac.Policies...)
	h, err := auth.NewServiceHandler(mux, authCfg, auth.ServiceRoutes{RBAC: &rbac})
	if err != nil {
		return nil, err
	}
	return httputil.SecurityHeaders(httputil.NewServiceMetrics(serviceName).Instrument(mux, h)), nil
}

func main() {
	wotanClient.InitServiceIdentity("sword") // ADR-103 §6: every Wotan call carries sword's service token
	log := zerolog.New(os.Stderr).With().Timestamp().Str("service", serviceName).Logger()
	c, err := loadConfig(os.Getenv)
	if err != nil {
		log.Fatal().Err(err).Msg("sword cannot start")
	}
	s, err := build(c, &log)
	if err != nil {
		log.Fatal().Err(err).Msg("sword cannot start")
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	transportCfg := transport.DefaultConfig()
	transport.ConfigFromEnv(&transportCfg)
	discovery.Start(ctx, transportCfg, serviceName, ports.PortOf(c.addr, ports.Sword))
	healthreport.StartService(ctx, transportCfg, serviceName, healthreport.HealthURL(c.addr, ports.PortOf(c.addr, ports.Sword))) // ADR-029: a vote on Akira's ballot

	h, err := handler(s, auth.LoadServiceAuthConfig(serviceName))
	if err != nil {
		log.Fatal().Err(err).Msg("sword cannot start: auth (see ADR-103)")
	}
	srv := &http.Server{
		Addr:              c.addr,
		Handler:           h,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    1 << 20,
	}
	// TLS from UNHEADED_TLS_* (S-IRON-GATE Phase 8): never a silent
	// plaintext fallback.
	tlsOn, err := utls.ConfigureServer(srv)
	if err != nil {
		log.Fatal().Err(err).Msg("sword cannot start: TLS")
	}
	errc := make(chan error, 1)
	go func() {
		log.Info().Str("addr", c.addr).Bool("tls", tlsOn).Msg("sword listening")
		errc <- utls.ListenAndServe(srv)
	}()
	select {
	case err := <-errc:
		if !errors.Is(err, http.ErrServerClosed) {
			log.Error().Err(err).Msg("HTTP server")
		}
	case <-ctx.Done():
	}
	sctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(sctx); err != nil {
		log.Error().Err(err).Msg("HTTP shutdown")
	}
	s.Close()
	log.Info().Msg("sword stopped")
}
