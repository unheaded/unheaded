// Package packetflow generates mock eBPF packet flow traces for visualization.
package packetflow

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"time"

	"unheaded/pkg/logger"
)

var (
	// ErrNilConfig indicates nil configuration
	ErrNilConfig = errors.New("config cannot be nil")
	// ErrInvalidInterval indicates invalid generation interval
	ErrInvalidInterval = errors.New("interval must be > 0")
)

// Config holds packet flow generator configuration
type Config struct {
	Interval       time.Duration // How often to generate flows
	MaxFlows       int           // Maximum concurrent flows
	TraceIDPattern string        // Trace ID pattern
}

// Validate validates configuration
func (c *Config) Validate() error {
	if c == nil {
		return ErrNilConfig
	}
	if c.Interval <= 0 {
		return ErrInvalidInterval
	}
	if c.MaxFlows <= 0 {
		c.MaxFlows = 10
	}
	if c.TraceIDPattern == "" {
		c.TraceIDPattern = "trace-%d"
	}
	return nil
}

// Hop represents a single hop in the packet flow
type Hop struct {
	Component string            `json:"component"` // Component name
	Timestamp time.Time         `json:"timestamp"` // When packet reached this hop
	Latency   time.Duration     `json:"latency"`   // Latency added at this hop
	Metadata  map[string]string `json:"metadata,omitempty"` // Additional metadata
}

// PacketFlow represents a complete packet trace through the system
type PacketFlow struct {
	TraceID    string        `json:"trace_id"`    // Unique trace identifier
	Timestamp  time.Time     `json:"timestamp"`   // Flow start time
	SourceIP   string        `json:"source_ip"`   // Source IP address
	DestIP     string        `json:"dest_ip"`     // Destination IP address
	Protocol   string        `json:"protocol"`    // Protocol (HTTP, gRPC, etc)
	Method     string        `json:"method"`      // HTTP method or RPC method
	Path       string        `json:"path"`        // Request path
	StatusCode int           `json:"status_code"` // Response status
	TotalTime  time.Duration `json:"total_time"`  // Total request time
	Hops       []Hop         `json:"hops"`        // Packet hops through system
}

// Generator generates mock packet flow data
type Generator struct {
	config  *Config
	log     *logger.Logger
	rand    *rand.Rand
	counter int64
}

// NewGenerator creates a new packet flow generator
func NewGenerator(config *Config) (*Generator, error) {
	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	return &Generator{
		config:  config,
		log:     logger.New(io.Discard),
		rand:    rand.New(rand.NewSource(time.Now().UnixNano())),
		counter: 0,
	}, nil
}

// Start starts generating packet flows
func (g *Generator) Start(ctx context.Context) (<-chan *PacketFlow, error) {
	flowCh := make(chan *PacketFlow, g.config.MaxFlows)

	go func() {
		defer close(flowCh)

		ticker := time.NewTicker(g.config.Interval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				g.log.Debug().Msg("packet flow generator stopped")
				return
			case <-ticker.C:
				flow := g.generateFlow()
				select {
				case flowCh <- flow:
				case <-ctx.Done():
					return
				default:
					// Channel full, drop flow
					g.log.Warn().Msg("flow channel full, dropping packet flow")
				}
			}
		}
	}()

	return flowCh, nil
}

// generateFlow generates a single packet flow trace
func (g *Generator) generateFlow() *PacketFlow {
	g.counter++
	now := time.Now()

	// Generate realistic components in order
	hops := g.generateHops(now)

	// Calculate total time
	var totalTime time.Duration
	for _, hop := range hops {
		totalTime += hop.Latency
	}

	flow := &PacketFlow{
		TraceID:    fmt.Sprintf(g.config.TraceIDPattern, g.counter),
		Timestamp:  now,
		SourceIP:   g.randomIP(),
		DestIP:     "10.10.10.100", // Gateway IP
		Protocol:   g.randomProtocol(),
		Method:     g.randomMethod(),
		Path:       g.randomPath(),
		StatusCode: g.randomStatusCode(),
		TotalTime:  totalTime,
		Hops:       hops,
	}

	return flow
}

// generateHops generates realistic packet hops through Unheaded architecture
func (g *Generator) generateHops(startTime time.Time) []Hop {
	// Unheaded packet flow:
	// XDP -> Gateway -> Wotan -> Service -> trace-collector

	components := []struct {
		name       string
		latencyMin time.Duration
		latencyMax time.Duration
	}{
		{"xdp_packet_marker", 50 * time.Microsecond, 200 * time.Microsecond},
		{"gateway", 1 * time.Millisecond, 5 * time.Millisecond},
		{"wotan", 500 * time.Microsecond, 2 * time.Millisecond},
		{"service", 5 * time.Millisecond, 20 * time.Millisecond},
		{"trace-collector", 1 * time.Millisecond, 3 * time.Millisecond},
	}

	hops := make([]Hop, len(components))
	currentTime := startTime

	for i, comp := range components {
		// Random latency within range
		latency := comp.latencyMin + time.Duration(g.rand.Int63n(int64(comp.latencyMax-comp.latencyMin)))
		currentTime = currentTime.Add(latency)

		hops[i] = Hop{
			Component: comp.name,
			Timestamp: currentTime,
			Latency:   latency,
			Metadata: map[string]string{
				"host": g.randomHost(),
			},
		}
	}

	return hops
}

// randomIP generates a random source IP
func (g *Generator) randomIP() string {
	return fmt.Sprintf("192.168.%d.%d",
		g.rand.Intn(256),
		g.rand.Intn(256))
}

// randomProtocol returns a random protocol
func (g *Generator) randomProtocol() string {
	protocols := []string{"HTTP/3", "gRPC", "HTTP/2", "WebSocket"}
	return protocols[g.rand.Intn(len(protocols))]
}

// randomMethod returns a random HTTP/RPC method
func (g *Generator) randomMethod() string {
	methods := []string{"GET", "POST", "PUT", "DELETE", "PATCH", "StreamMessages"}
	return methods[g.rand.Intn(len(methods))]
}

// randomPath returns a random request path
func (g *Generator) randomPath() string {
	paths := []string{
		"/api/v1/timeline",
		"/api/v1/metrics",
		"/api/v1/tasks",
		"/ws/dashboard",
		"/health",
		"/ready",
	}
	return paths[g.rand.Intn(len(paths))]
}

// randomStatusCode returns a random HTTP status code
func (g *Generator) randomStatusCode() int {
	// Weighted towards success
	r := g.rand.Intn(100)
	switch {
	case r < 85: // 85% success
		return 200
	case r < 90: // 5% created
		return 201
	case r < 93: // 3% not found
		return 404
	case r < 96: // 3% server error
		return 500
	case r < 98: // 2% bad request
		return 400
	default: // 2% rate limited
		return 429
	}
}

// randomHost returns a random container host
func (g *Generator) randomHost() string {
	hosts := []string{
		"10.10.10.10",  // wotan
		"10.10.10.20",  // timeguru
		"10.10.10.21",  // captain
		"10.10.10.22",  // micromanager
		"10.10.10.23",  // architect
		"10.10.10.100", // gateway
	}
	return hosts[g.rand.Intn(len(hosts))]
}
