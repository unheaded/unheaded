// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis.

// unheaded-auth is the built-in OIDC-shaped issuer (ADR-103 §3, S-IRON-GATE
// Phase 4): password login (argon2id), EdDSA access tokens (15 minutes) with
// a "roles" claim, JWKS and discovery. Services verify its tokens through
// auth.JWKSVerifier (AUTH_OIDC_ISSUER_URL = -issuer-url); any other OIDC
// provider with EdDSA keys plugs in the same way.
//
//	unheaded-auth [-listen :19020] -issuer-url https://host:19020 \
//	    [-key-file /etc/unheaded/keys/unheaded-auth.key] \
//	    [-users-file /etc/unheaded/auth/users.json] [-audiences unheaded,...] [-dev-plaintext]
//	unheaded-auth user add <name> --roles viewer,operator [--password-stdin]
//	unheaded-auth user list
//	unheaded-auth user del <name>
//
// TLS comes from pkg/tls (UNHEADED_TLS_*), hybrid ML-KEM only (ADR-105).
// Without it, only /health, /ready and /metrics answer, unless
// -dev-plaintext. SIGHUP re-reads the users file.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

	"unheaded/pkg/cmdline"
	"unheaded/pkg/ports"
	utls "unheaded/pkg/tls"
)

const (
	defaultKeyFile   = "/etc/unheaded/keys/unheaded-auth.key"
	defaultUsersFile = "/etc/unheaded/auth/users.json"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}

func run(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	if len(args) == 1 && cmdline.IsVersionArg(args[0]) {
		cmdline.PrintVersion(stdout, cmdline.Spec{Name: "unheaded-auth"})
		return 0
	}
	if len(args) > 0 && args[0] == "user" {
		return runUser(args[1:], stdin, stdout, stderr)
	}
	return runServer(args, stderr)
}

func runServer(args []string, stderr io.Writer) int {
	fs := flag.NewFlagSet("unheaded-auth", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	listen := fs.String("listen", fmt.Sprintf(":%d", ports.Auth), "listen address")
	issuerURL := fs.String("issuer-url", "", "external issuer URL: the tokens' iss and the discovery base (required)")
	keyFile := fs.String("key-file", defaultKeyFile, "issuer Ed25519 key (unheaded-keygen unheaded-auth), 0400")
	usersFile := fs.String("users-file", defaultUsersFile, "users file (unheaded-auth user add), 0600")
	audiences := fs.String("audiences", defaultAudience, "comma-separated audiences a login may ask for; the first is the default")
	devPlaintext := fs.Bool("dev-plaintext", false, "serve login, JWKS and discovery over plain HTTP (development only)")
	authSpec := cmdline.Spec{
		Name:     "unheaded-auth",
		Purpose:  "Serve the Kingdom's login, JWKS and OIDC discovery endpoints; `unheaded-auth user` manages the users file.",
		Commands: []cmdline.Command{{Name: "user add|list|del", Purpose: "manage users (see unheaded-auth user --help)"}},
	}
	if err := fs.Parse(args); err != nil {
		return cmdline.HandleParseError(err, fs, authSpec, args, os.Stdout, stderr)
	}
	if fs.NArg() != 0 {
		return cmdline.ReportUsageError(stderr, authSpec, "extra operand '%s'", fs.Arg(0))
	}

	log.Logger = zerolog.New(stderr).With().Timestamp().Str("service", serviceName).Logger()
	iss, err := newIssuer(issuerConfig{
		IssuerURL:    *issuerURL,
		KeyFile:      *keyFile,
		UsersFile:    *usersFile,
		DevPlaintext: *devPlaintext,
		Audiences:    splitList(*audiences),
	})
	if err != nil {
		log.Error().Err(err).Msg("refusing to start")
		return 1
	}

	srv, tlsOn, err := utls.MaybeWrapServer(*listen, iss.Handler())
	if err != nil {
		log.Error().Err(err).Msg("TLS configuration")
		return 1
	}
	if tlsOn {
		utls.ApplyStrict(srv.TLSConfig)
	} else if !*devPlaintext {
		log.Warn().Msg("no TLS (UNHEADED_TLS_ENABLED unset) and no -dev-plaintext: only /health, /ready and /metrics will answer")
	}
	srv.ReadHeaderTimeout = 5 * time.Second
	srv.ReadTimeout = 10 * time.Second
	srv.WriteTimeout = 15 * time.Second
	srv.IdleTimeout = 60 * time.Second
	srv.MaxHeaderBytes = 16 << 10

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	hup := make(chan os.Signal, 1)
	signal.Notify(hup, syscall.SIGHUP)
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case <-hup:
				if err := iss.ReloadUsers(); err != nil {
					log.Error().Err(err).Msg("users file reload failed; keeping the users already loaded")
				} else {
					log.Info().Msg("users file reloaded")
				}
			}
		}
	}()

	errc := make(chan error, 1)
	go func() {
		log.Info().Str("listen", *listen).Bool("tls", tlsOn).Str("issuer", iss.cfg.IssuerURL).Str("kid", iss.kid).Msg("issuer listening")
		if tlsOn {
			errc <- srv.ListenAndServeTLS("", "")
		} else {
			errc <- srv.ListenAndServe()
		}
	}()
	select {
	case err := <-errc:
		if !errors.Is(err, http.ErrServerClosed) {
			log.Error().Err(err).Msg("server")
			return 1
		}
	case <-ctx.Done():
	}
	shutCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = srv.Shutdown(shutCtx)
	return 0
}

func splitList(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}
