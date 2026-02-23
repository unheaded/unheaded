package discovery

import (
	"context"
	"fmt"
)

// SetupDiscovery creates, registers, and starts a discovery registry for a
// service. This is a convenience helper intended to be called during service
// startup. The returned Registry should be stopped (via Stop()) during
// graceful shutdown.
//
// Example usage:
//
//	reg, err := discovery.SetupDiscovery("timeguru", "10.10.10.20", 8000, wotanClient)
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
