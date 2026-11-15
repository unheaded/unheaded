// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis. All rights reserved.

package transport

import (
	"errors"
	"net"
	"strconv"
	"strings"

	"unheaded/pkg/ports"
	wotanClient "unheaded/pkg/wotan-client"
)

// NewWotanClient builds the process's one Wotan client (ADR-147 Phase 7,
// Developer D6): HTTP for control (join, publish), gRPC for streams, with
// HTTP polling only as the fallback that ends when gRPC answers again. A
// binary builds it once, right after its transport.Config is known, and
// hands it to discovery.Start, healthreport.StartService, logagg.Connect
// and its own subscriptions; pkg/wotan-client/policy_test.go fails a
// binary that builds a second one or a library that builds one.
func NewWotanClient(cfg Config, opts ...wotanClient.ClientOption) (*wotanClient.Client, error) {
	if cfg.WotanGRPCAddr == "" {
		return nil, errors.New("transport: no Wotan gRPC address (WOTAN_GRPC_ADDR, --wotan-grpc-addr); streams need one")
	}
	return wotanClient.NewClientWithGRPC(cfg.WotanHTTPAddr, cfg.WotanGRPCAddr, opts...)
}

// WotanGRPCAddr is grpcAddr, or, when it is empty, the host of httpAddr
// (a URL or host:port) on Wotan's gRPC port: for a binary configured with
// one --wotan address (akira, anomaly).
func WotanGRPCAddr(httpAddr, grpcAddr string) string {
	if grpcAddr != "" {
		return grpcAddr
	}
	hostport := httpAddr
	if i := strings.Index(hostport, "://"); i >= 0 {
		hostport = hostport[i+3:]
	}
	hostport = strings.TrimSuffix(hostport, "/")
	host := hostport
	if h, _, err := net.SplitHostPort(hostport); err == nil {
		host = h
	}
	return net.JoinHostPort(host, strconv.Itoa(ports.WotanGRPC))
}
