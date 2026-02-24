// Package ports defines the canonical port assignments for all Unheaded services.
// The "Doom Range" (16666-26666) ensures no conflicts with common dev tools.
// This file is the SINGLE SOURCE OF TRUTH for port assignments.
package ports

import "fmt"

const (
	// Infrastructure Tier (16666-16999)
	DoomBridge            = 16666
	DoomGoInjector        = 16667
	TraceCollectorHTTP    = 16670
	TraceCollectorMetrics = 16671

	// Control Plane (17000-17999)
	DaemonHTTP = 17000
	DaemonGRPC = 17001
	CLIServer  = 17010

	// Wotan — Message Bus (18000-18099)
	WotanHTTP = 18000
	WotanGRPC = 18001

	// Core Services (19000-19999)
	Timeguru     = 19000
	Architect    = 19001
	Captain      = 19002
	Micromanager = 19003
	Monad        = 19004
	Sophia       = 19005
	Cuirass      = 19006

	// Applications (20000-20999)
	DashboardBackend = 20000
	KanbanApp        = 20001
	WikiServer       = 20002

	// Gateway (21000-21443)
	GatewayHTTP  = 21000
	GatewayHTTPS = 21443

	// Utilities (22000-22099)
	CertGen = 22000

	// Customer Apps (26000-26666)
	CustomerStart = 26000
	CustomerEnd   = 26666
)

// DefaultAddr returns ":<port>" string for net.Listen.
func DefaultAddr(port int) string {
	return fmt.Sprintf(":%d", port)
}

// DefaultWotanGRPC returns the default Wotan gRPC dial target.
func DefaultWotanGRPC() string {
	return fmt.Sprintf("localhost:%d", WotanGRPC)
}

// DefaultWotanHTTP returns the default Wotan HTTP base URL.
func DefaultWotanHTTP() string {
	return fmt.Sprintf("http://localhost:%d", WotanHTTP)
}
