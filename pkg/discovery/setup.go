// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package discovery

import (
	"context"
	"fmt"
	"log"

	"unheaded/pkg/transport"
)

// SetupDiscovery creates, registers, and starts a discovery registry for a
// service. This is a convenience helper intended to be called during service
// startup. The returned Registry should be stopped (via Stop()) during
// graceful shutdown.
//
// Example usage:
//
//	reg, err := discovery.SetupDiscovery("timeguru", "localhost", 8000, wotanClient)
//	if err != nil {
//	    log.Fatalf("discovery setup: %v", err)
//	}
//	defer reg.Stop()
func SetupDiscovery(ctx context.Context, serviceName, addr string, port int, wotan WotanClient) (*Registry, error) {
	if serviceName == "" {
		return nil, fmt.Errorf("service name cannot be empty")
	}
	if addr == "" {
		return nil, fmt.Errorf("address cannot be empty")
	}
	if port <= 0 || port > 65535 {
		return nil, fmt.Errorf("invalid port: %d", port)
	}
	if wotan == nil {
		return nil, fmt.Errorf("wotan client cannot be nil")
	}

	reg := New(wotan)

	if err := reg.Start(ctx); err != nil {
		return nil, fmt.Errorf("start discovery: %w", err)
	}

	entry := ServiceEntry{
		Name:    serviceName,
		Address: addr,
		Port:    port,
		Health:  "healthy",
	}
	if err := reg.Register(entry); err != nil {
		reg.Stop()
		return nil, fmt.Errorf("register service: %w", err)
	}

	return reg, nil
}

// SetupServiceDiscovery registers a service using the transport layer and
// deregisters on context cancellation. This is best-effort — failures are
// logged but do not prevent startup.
//
// Example usage:
//
//	reg, _ := discovery.SetupServiceDiscovery(ctx, conn, "timeguru", 19000)
//	// reg is non-nil even on registration failure
func SetupServiceDiscovery(ctx context.Context, conn transport.Connection, name string, port int) *Registrar {
	reg := NewRegistrar(conn, name, port, "http")

	if conn == nil {
		log.Printf("[discovery] %s: no transport connection — skipping registration", name)
		return reg
	}

	if err := reg.Register(ctx); err != nil {
		log.Printf("[discovery] %s: registration failed (non-fatal): %v", name, err)
	}

	// Deregister on context cancellation.
	go func() {
		<-ctx.Done()
		if err := reg.Deregister(context.Background()); err != nil {
			log.Printf("[discovery] %s: deregistration failed: %v", name, err)
		}
	}()

	return reg
}
