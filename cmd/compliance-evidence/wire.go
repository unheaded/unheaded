// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"unheaded/pkg/auth"
	"unheaded/pkg/cmdline"
	"unheaded/pkg/httputil"
	"unheaded/pkg/logger"
	utls "unheaded/pkg/tls"
	"unheaded/services/compliance"
)

// `compliance-evidence wire`: the wire compliance plane (services/compliance),
// folded in behind --serve (ADR-127, DP-203). Without --serve it says it is
// off and does nothing: no listener, no eBPF. No unit, timer or compose service
// passes --serve. The evidence file is untouched either way. There is no Wotan
// client either way, so violations stay on the host: wire presents no Wotan
// identity until it has its own nologin account.

const wireOffMessage = "wire compliance is off; pass --serve to run it (ADR-127)"

type wireOptions struct {
	serve     bool
	listen    string
	auditRing string
}

// runWire returns the process exit status: 0, 1 for a failed serve, 2 for a
// usage error, and --help goes to stdout with 0 (ADR-111).
func runWire(args []string, stdout, stderr io.Writer) int {
	var o wireOptions
	spec := cmdline.Spec{Name: "compliance-evidence wire", Purpose: "Run the wire compliance plane; off unless --serve (ADR-127)."}
	fl := flag.NewFlagSet("wire", flag.ContinueOnError)
	fl.SetOutput(io.Discard)
	fl.BoolVar(&o.serve, "serve", false, "run the wire compliance plane (default off)")
	fl.StringVar(&o.listen, "listen", compliance.DefaultConfig().ListenAddr, "HTTP listen address; anything but loopback needs AUTH_ENABLED=true")
	fl.StringVar(&o.auditRing, "audit-ring", "", "pinned compliance-ebpf AUDIT_LOG ring to consume (default: none)")
	if err := fl.Parse(args); err != nil {
		return cmdline.HandleParseError(err, fl, spec, args, stdout, stderr)
	}
	if fl.NArg() > 0 {
		return cmdline.ReportUsageError(stderr, spec, "unexpected argument %q", fl.Arg(0))
	}
	if !o.serve {
		fmt.Fprintln(stdout, wireOffMessage)
		return 0
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := serveWire(ctx, o, nil); err != nil {
		fmt.Fprintln(stderr, "compliance-evidence wire:", err)
		return 1
	}
	return 0
}

// checkWireListen refuses a non-loopback address without authentication: the
// plane accepts policy writes.
func checkWireListen(addr string, authEnabled bool) error {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return fmt.Errorf("--listen %q: %w", addr, err)
	}
	ip := net.ParseIP(host)
	if !authEnabled && (ip == nil || !ip.IsLoopback()) {
		return fmt.Errorf("--listen %q is not loopback: set AUTH_ENABLED=true first", addr)
	}
	return nil
}

// serveWire runs the plane until ctx ends. ready, when set, receives the bound
// address once the listener is up.
func serveWire(ctx context.Context, o wireOptions, ready chan<- string) error {
	authCfg := auth.LoadServiceAuthConfig("compliance-evidence")
	if err := checkWireListen(o.listen, authCfg.Enabled); err != nil {
		return err
	}
	svc := compliance.NewService(logger.New(os.Stderr), nil, nil)
	if err := svc.StartBackground(ctx); err != nil {
		return err
	}
	if o.auditRing != "" {
		go func() {
			if err := consumeAuditRing(ctx, o.auditRing, svc); err != nil {
				fmt.Fprintln(os.Stderr, "compliance-evidence wire: audit ring:", err)
			}
		}()
	}
	mux := svc.Handler()
	h, err := auth.NewServiceHandler(mux, authCfg, auth.ServiceRoutes{})
	if err != nil {
		return err
	}
	srv, _, err := utls.MaybeWrapServer(o.listen, httputil.NewServiceMetrics("wire-compliance").Instrument(mux, h))
	if err != nil {
		return err
	}
	ln, err := net.Listen("tcp", o.listen)
	if err != nil {
		return err
	}
	if ready != nil {
		ready <- ln.Addr().String()
	}
	errc := make(chan error, 1)
	go func() {
		if srv.TLSConfig != nil {
			errc <- srv.ServeTLS(ln, "", "")
			return
		}
		errc <- srv.Serve(ln)
	}()
	select {
	case err := <-errc:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case <-ctx.Done():
		sctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return srv.Shutdown(sctx)
	}
}
