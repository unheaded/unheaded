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
//	CUIRASS_API_KEY   required: Sword refuses to start without it
//	SWORD_DATA_DIR    deployment history; unset = read-only (writes 503)
//	VM_URL            VictoriaMetrics for the SLO guard; unset = canary refused
//	SWORD_SLO_POLICY  alerting policy file (default /etc/unheaded/sword/policy.yaml)
//	WOTAN_ADDR, WOTAN_GRPC_ADDR  Wotan, for deploy.* events (best effort)
//	AUTH_ENABLED=true and AUTH_API_KEYS / AUTH_JWT_KEY  pkg/auth; without
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
	"unheaded/pkg/deploy/cuirassdriver"
	"unheaded/pkg/discovery"
	"unheaded/pkg/httputil"
	"unheaded/pkg/ports"
	"unheaded/pkg/sloguard"
	"unheaded/pkg/transport"
	wotanClient "unheaded/pkg/wotan-client"
	"unheaded/services/sword"
)

const serviceName = "sword"

type config struct {
	addr       string
	cuirassURL string
	cuirassKey string
	dataDir    string
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

// loadConfig reads the environment. A missing Cuirass key is an error:
// credentials fail loud, never a default.
func loadConfig(getenv func(string) string) (config, error) {
	c := config{
		addr:       envOr(getenv, "HTTP_ADDR", ports.DefaultAddr(ports.Sword)),
		cuirassURL: envOr(getenv, "CUIRASS_URL", fmt.Sprintf("http://127.0.0.1:%d", ports.Cuirass)),
		cuirassKey: getenv("CUIRASS_API_KEY"),
		dataDir:    getenv("SWORD_DATA_DIR"),
		vmURL:      getenv("VM_URL"),
		policy:     envOr(getenv, "SWORD_SLO_POLICY", "/etc/unheaded/sword/policy.yaml"),
		wotanHTTP:  getenv("WOTAN_ADDR"),
		wotanGRPC:  getenv("WOTAN_GRPC_ADDR"),
	}
	if c.cuirassKey == "" {
		return c, errors.New("CUIRASS_API_KEY is not set: Sword deploys through Cuirass's write API and credentials are not stored in the repo")
	}
	return c, nil
}

// build wires the service from a config.
func build(c config, log *zerolog.Logger) (*sword.Service, error) {
	d, err := cuirassdriver.New(cuirassdriver.Config{URL: c.cuirassURL, APIKey: c.cuirassKey})
	if err != nil {
		return nil, err
	}
	cfg := sword.Config{
		DataDir:     c.dataDir,
		AuthEnabled: auth.LoadServiceAuthConfig(serviceName).Enabled,
		Driver:      d,
		Log:         log,
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
	if c.dataDir == "" {
		log.Warn().Msg("SWORD_DATA_DIR is not set: read-only, every deployment request answers 503")
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

func handler(s *sword.Service) http.Handler {
	mux := s.Handler()
	h := auth.WrapHandler(mux, auth.SetupMiddleware(auth.LoadServiceAuthConfig(serviceName)))
	return httputil.NewServiceMetrics(serviceName).Instrument(mux, h)
}

func main() {
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

	srv := &http.Server{
		Addr:              c.addr,
		Handler:           handler(s),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    1 << 20,
	}
	errc := make(chan error, 1)
	go func() {
		log.Info().Str("addr", c.addr).Msg("sword listening")
		errc <- srv.ListenAndServe()
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
