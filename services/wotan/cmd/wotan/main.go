// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package main

import (
	"context"
	"crypto/tls"
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"runtime"
	"syscall"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/rs/zerolog/log"
	"golang.org/x/time/rate"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/keepalive"

	"unheaded/pkg/auth"
	"unheaded/services/wotan/internal/api"
	"unheaded/services/wotan/internal/cluster"
	grpcservice "unheaded/services/wotan/internal/grpc"
	"unheaded/services/wotan/internal/logger"
	"unheaded/services/wotan/internal/member"
	"unheaded/services/wotan/internal/metrics"
	"unheaded/services/wotan/internal/middleware"
	"unheaded/services/wotan/internal/room"
	"unheaded/services/wotan/internal/store"
	"unheaded/services/wotan/internal/wotan"
	chatpb "unheaded/services/wotan/proto"
)

var (
	// Version is set during build
	Version = "dev"
)

// Config holds server configuration
type Config struct {
	// Server
	BufferSize   int
	HTTPPort     int
	GRPCPort     int
	AdminEnabled bool

	// TLS
	EnableTLS   bool
	TLSCertFile string
	TLSKeyFile  string

	// Logging
	LogLevel  string
	LogPretty bool

	// Rate limiting
	RateLimit   float64
	RateBurst   int
	RateCleanup time.Duration

	// Timeouts
	ReadTimeout            time.Duration
	WriteTimeout           time.Duration
	IdleTimeout            time.Duration
	PendingApprovalTimeout time.Duration
	ShutdownTimeout        time.Duration

	// Topic config
	TopicConfigPath string

	// CORS
	CORSOrigins []string

	// Cluster
	ClusterMode            string
	ClusterRole            string
	ClusterNodeID          string
	ClusterPeerAddr        string
	ClusterReplicationPort int
	ClusterPKIDir          string
	StoreType              string
	StoreDataDir           string
	StoreConnStr           string
}

func main() {
	// Parse command line flags using the global FlagSet (real-binary path).
	// Tests construct their own FlagSet via parseFlagsFromSet so they
	// don't pollute global state.
	config := parseFlags(flag.CommandLine, os.Args[1:])

	// Initialize structured logging
	logger.Initialize(logger.Config{
		Level:      config.LogLevel,
		Pretty:     config.LogPretty,
		WithCaller: false,
	})

	log.Info().
		Str("version", Version).
		Str("go_version", runtime.Version()).
		Msg("starting_unheaded_chat_server")

	log.Info().
		Int("buffer_size", config.BufferSize).
		Int("http_port", config.HTTPPort).
		Int("grpc_port", config.GRPCPort).
		Bool("tls_enabled", config.EnableTLS).
		Float64("rate_limit", config.RateLimit).
		Int("rate_burst", config.RateBurst).
		Dur("pending_approval_timeout", config.PendingApprovalTimeout).
		Msg("server_configuration")

	// Initialize Prometheus metrics
	m := metrics.Initialize("unheaded_chat")

	// Background-task context. Canceled in the shutdown sequence so
	// every periodic-ticker goroutine observes Done() and returns
	// cleanly within the shutdown deadline (not after it).
	runCtx, runCancel := context.WithCancel(context.Background())

	// Start metrics collector
	go collectSystemMetrics(runCtx, m)

	// Initialize managers
	roomManager := room.NewManager(config.BufferSize)
	memberManager := member.NewManager()
	messageWotan := wotan.NewWotan()

	// Initialize persistent store (if configured). When this falls back
	// (err != nil) the local convention is `msgStore == nil` →
	// memory-only ring buffer. apiServer.Store is documented to accept
	// nil; do not interpret nil as "store init failed silently."
	var msgStore store.MessageStore
	if config.StoreType != "" && config.StoreType != "memory" {
		storeCfg := store.Config{
			Type:     store.StoreType(config.StoreType),
			Capacity: config.BufferSize,
			DataDir:  config.StoreDataDir,
			ConnStr:  config.StoreConnStr,
			SyncMode: "batch",
		}
		var err error
		msgStore, err = store.New(storeCfg)
		if err != nil {
			log.Warn().Err(err).Str("type", config.StoreType).Msg("persistent store failed, falling back to memory")
		} else {
			log.Info().Str("type", config.StoreType).Msg("persistent store initialized")
		}
	}

	// Cluster mode: fail-fast if the operator passed cluster flags. The
	// cluster.Config validation logic exists, but the actual replication
	// wiring isn't complete yet — silently running standalone after
	// "cluster mode enabled" was logged would mislead operators. Refuse
	// to start until cluster wiring lands.
	clusterCfg := cluster.Config{
		Mode:            cluster.Mode(config.ClusterMode),
		Role:            cluster.Role(config.ClusterRole),
		NodeID:          config.ClusterNodeID,
		PeerAddr:        config.ClusterPeerAddr,
		ReplicationPort: config.ClusterReplicationPort,
		PKIDir:          config.ClusterPKIDir,
	}
	if clusterCfg.IsCluster() {
		if err := clusterCfg.Validate(); err != nil {
			log.Fatal().Err(err).Msg("invalid cluster configuration")
		}
		log.Fatal().
			Str("mode", string(clusterCfg.Mode)).
			Str("role", string(clusterCfg.Role)).
			Str("peer", clusterCfg.PeerAddr).
			Msg("cluster mode is not yet implemented in this binary; refusing to start. Drop --cluster-* flags to run standalone.")
	}

	log.Info().Msg("initialized_core_managers")

	// Load topic configuration (auto-approval allowlist)
	topicConfigPath := os.Getenv("WOTAN_TOPIC_CONFIG")
	if topicConfigPath == "" {
		topicConfigPath = config.TopicConfigPath
	}
	topicCfg, err := api.LoadTopicConfig(topicConfigPath)
	if err != nil {
		log.Fatal().Err(err).Str("path", topicConfigPath).Msg("failed to load topic config")
	}
	log.Info().
		Str("config_path", topicConfigPath).
		Int("auto_approve_count", len(topicCfg.Topics.AutoApprove)).
		Msg("loaded_topic_config")

	// Create API server
	apiServer := api.NewServer(roomManager, memberManager, messageWotan, config.PendingApprovalTimeout)
	apiServer.TopicConfig = topicCfg
	apiServer.Store = msgStore // Wire persistent store (nil = memory-only)
	// InitTopics returns no error today (see services/wotan/internal/
	// api/topics.go); if a future change makes it fallible, wrap here.
	apiServer.InitTopics()

	// Setup rate limiter
	rateLimiter := middleware.NewRateLimiter(
		rate.Limit(config.RateLimit),
		config.RateBurst,
		config.RateCleanup,
	)

	log.Info().Msg("initialized_rate_limiter")

	// Setup HTTP server with REST API and middleware chain
	httpMux := setupHTTPRoutes(apiServer, config.AdminEnabled, m)
	httpHandler := setupMiddleware(httpMux, rateLimiter, config.CORSOrigins)

	// Auth middleware (activated via AUTH_ENABLED=true)
	authCfg := auth.LoadServiceAuthConfig("wotan")
	// Explicit interface type required: srvHandler is reassigned below to
	// auth.WrapHandler's http.Handler return value.
	var srvHandler http.Handler = httpHandler //nolint:staticcheck // QF1011 false-positive: interface type required for reassignment
	srvHandler = auth.WrapHandler(srvHandler, auth.SetupMiddleware(authCfg))

	httpServer := &http.Server{
		Addr:              fmt.Sprintf(":%d", config.HTTPPort),
		Handler:           srvHandler,
		ReadTimeout:       config.ReadTimeout,
		ReadHeaderTimeout: 5 * time.Second, // slowloris defense — header phase tighter than body
		WriteTimeout:      config.WriteTimeout,
		IdleTimeout:       config.IdleTimeout,
		MaxHeaderBytes:    1 << 20, // 1 MB
	}

	// Configure TLS if enabled
	if config.EnableTLS {
		if config.TLSCertFile == "" || config.TLSKeyFile == "" {
			log.Fatal().Msg("TLS enabled but cert/key files not provided")
		}

		tlsConfig := &tls.Config{
			MinVersion: tls.VersionTLS13,
			// PreferServerCipherSuites was deprecated in Go 1.18; TLS 1.3
			// no longer honours cipher-suite preferences. Removed.
			CurvePreferences: []tls.CurveID{
				tls.X25519,
				tls.CurveP256,
			},
		}
		httpServer.TLSConfig = tlsConfig
	}

	// Start HTTP server
	go func() {
		log.Info().
			Int("port", config.HTTPPort).
			Bool("tls", config.EnableTLS).
			Msg("starting_http_server")

		var err error
		if config.EnableTLS {
			err = httpServer.ListenAndServeTLS(config.TLSCertFile, config.TLSKeyFile)
		} else {
			err = httpServer.ListenAndServe()
		}
		if err != nil && err != http.ErrServerClosed {
			log.Fatal().Err(err).Msg("http_server_error")
		}
	}()

	// Create and start gRPC streaming services
	chatService := grpcservice.NewChatService(roomManager, memberManager, messageWotan)

	// Create shared topic sequence counter (used by both HTTP and gRPC)
	topicSeqCounter := grpcservice.NewTopicSequenceCounter()

	// Create TopicStream gRPC service - THE COSMIC WHEEL
	topicService := grpcservice.NewTopicServiceWithCounter(roomManager, memberManager, messageWotan, topicSeqCounter)

	// gRPC server hardening (CLAUDE.md baseline). Without these, a
	// client can DoS the server via giant messages, abusive keepalive
	// pings, or by holding idle connections forever.
	grpcOpts := []grpc.ServerOption{
		grpc.MaxRecvMsgSize(4 << 20), // 4 MiB inbound cap; matches typical Wotan
		// payload + a 4× margin. Larger payloads
		// belong on a streaming endpoint, not a
		// single Recv.
		grpc.MaxSendMsgSize(4 << 20), // 4 MiB outbound cap (default is 4 MiB but
		// making it explicit pins the contract).
		grpc.KeepaliveParams(keepalive.ServerParameters{
			MaxConnectionIdle: 5 * time.Minute,  // close idle conns
			Time:              30 * time.Second, // server pings clients this often
			Timeout:           10 * time.Second, // ping ack deadline
		}),
		grpc.KeepaliveEnforcementPolicy(keepalive.EnforcementPolicy{
			MinTime:             10 * time.Second, // reject clients pinging more often
			PermitWithoutStream: true,             // allow keepalive on idle streams
		}),
	}

	var grpcServer *grpc.Server
	if config.EnableTLS {
		creds, err := credentials.NewServerTLSFromFile(config.TLSCertFile, config.TLSKeyFile)
		if err != nil {
			log.Fatal().Err(err).Msg("failed_to_load_tls_credentials")
		}
		grpcOpts = append(grpcOpts, grpc.Creds(creds))
	}
	grpcServer = grpc.NewServer(grpcOpts...)

	// Register gRPC service implementations
	chatpb.RegisterChatStreamServer(grpcServer, chatService)
	chatpb.RegisterTopicStreamServer(grpcServer, topicService)

	// Register gRPC health service (grpc.health.v1)
	healthServer := health.NewServer()
	healthpb.RegisterHealthServer(grpcServer, healthServer)
	healthServer.SetServingStatus("wotan", healthpb.HealthCheckResponse_SERVING)

	go func() {
		lis, err := net.Listen("tcp", fmt.Sprintf(":%d", config.GRPCPort))
		if err != nil {
			log.Fatal().Err(err).Msg("failed_to_listen_grpc_port")
		}

		log.Info().
			Int("port", config.GRPCPort).
			Bool("tls", config.EnableTLS).
			Msg("starting_grpc_server (ChatStream + TopicStream)")

		if err := grpcServer.Serve(lis); err != nil {
			log.Fatal().Err(err).Msg("grpc_server_error")
		}
	}()

	// Pending-member monitor (10s tick logger; previously misnamed
	// `startAdminCLI` — it doesn't start a CLI, it just logs).
	if config.AdminEnabled {
		go monitorPendingMembers(runCtx, memberManager, roomManager)
	}

	// Start periodic cleanup of expired pending members
	go cleanupExpiredMembers(runCtx, memberManager)

	// Wait for shutdown signal
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop

	log.Info().Msg("shutdown_signal_received")

	// Cancel the background-task context first so ticker goroutines
	// observe Done() and exit cleanly. Without this they ride past the
	// shutdown deadline and the daemon takes the full timeout to stop
	// even when there's nothing to drain.
	runCancel()

	// Graceful shutdown — deadline configurable via --shutdown-timeout.
	ctx, cancel := context.WithTimeout(context.Background(), config.ShutdownTimeout)
	defer cancel()

	log.Info().Msg("draining_grpc_connections")
	grpcServer.GracefulStop()

	log.Info().Msg("shutting_down_http_server")
	if err := httpServer.Shutdown(ctx); err != nil {
		log.Error().Err(err).Msg("http_server_shutdown_error")
	}

	log.Info().Msg("closing_message_wotan")
	messageWotan.Close()

	log.Info().Msg("server_stopped_gracefully")
}

// parseFlags binds the wotan flag schema to fs and parses args. Pass
// flag.CommandLine + os.Args[1:] from main() for the production path;
// tests pass their own *flag.FlagSet so they don't pollute global state
// and can exercise multiple flag combinations within one test process.
func parseFlags(fs *flag.FlagSet, args []string) Config {
	config := Config{}

	// Server flags
	fs.IntVar(&config.BufferSize, "buffer-size", 10000, "Ring buffer size per room")
	fs.IntVar(&config.HTTPPort, "http-port", 18000, "HTTP REST API port")
	fs.IntVar(&config.GRPCPort, "grpc-port", 18001, "gRPC streaming port")
	fs.BoolVar(&config.AdminEnabled, "admin", true, "Enable admin endpoints")

	// TLS flags
	fs.BoolVar(&config.EnableTLS, "enable-tls", false, "Enable TLS 1.3")
	fs.StringVar(&config.TLSCertFile, "tls-cert", "", "TLS certificate file")
	fs.StringVar(&config.TLSKeyFile, "tls-key", "", "TLS key file")

	// Logging flags
	fs.StringVar(&config.LogLevel, "log-level", "info", "Log level (debug, info, warn, error)")
	fs.BoolVar(&config.LogPretty, "log-pretty", false, "Pretty print logs for development")

	// Rate limiting flags
	fs.Float64Var(&config.RateLimit, "rate-limit", 100.0, "Rate limit (requests per second)")
	fs.IntVar(&config.RateBurst, "rate-burst", 200, "Rate limit burst size")
	config.RateCleanup = 5 * time.Minute

	// Timeout flags — promoted from hardcoded constants so operators
	// can tune for slow streams without recompiling.
	fs.DurationVar(&config.ReadTimeout, "read-timeout", 15*time.Second, "HTTP server ReadTimeout (full request including body)")
	fs.DurationVar(&config.WriteTimeout, "write-timeout", 15*time.Second, "HTTP server WriteTimeout")
	fs.DurationVar(&config.IdleTimeout, "idle-timeout", 60*time.Second, "HTTP server IdleTimeout (keep-alive)")
	fs.DurationVar(&config.ShutdownTimeout, "shutdown-timeout", 15*time.Second, "graceful shutdown deadline")
	fs.DurationVar(&config.PendingApprovalTimeout, "pending-approval-timeout", 1*time.Hour, "Timeout for pending member approval requests")

	// Topic config flag
	fs.StringVar(&config.TopicConfigPath, "topic-config", "configs/wotan.yaml", "Path to topic configuration file (auto-approval allowlist)")

	// CORS flags
	config.CORSOrigins = []string{"*"} // Configure as needed

	// Cluster flags (Wave 9 — The Twin Ravens)
	fs.StringVar(&config.ClusterMode, "cluster-mode", "standalone", "Cluster mode (standalone|cluster)")
	fs.StringVar(&config.ClusterRole, "cluster-role", "primary", "Cluster role (primary|standby)")
	fs.StringVar(&config.ClusterNodeID, "cluster-node-id", "", "Node identifier (default: hostname)")
	fs.StringVar(&config.ClusterPeerAddr, "cluster-peer", "", "Peer node address (host:port)")
	fs.IntVar(&config.ClusterReplicationPort, "cluster-replication-port", 18002, "Replication gRPC port")
	fs.StringVar(&config.ClusterPKIDir, "cluster-pki-dir", "/var/lib/unheaded/pki", "PKI directory for mTLS")

	// Store flags
	fs.StringVar(&config.StoreType, "store-type", "memory", "Store backend (memory|wal|postgres|hybrid)")
	fs.StringVar(&config.StoreDataDir, "store-data-dir", "/var/lib/unheaded/wotan/data", "Store data directory (WAL/hybrid)")
	fs.StringVar(&config.StoreConnStr, "store-conn-str", "", "PostgreSQL connection string (postgres/hybrid)")

	if err := fs.Parse(args); err != nil {
		// FlagSet's default error-handling is ContinueOnError for tests
		// (returns the error without exiting); ExitOnError for the real
		// binary (fs.Parse exits on err). Either way, this branch only
		// runs in test mode where the caller decides what to do.
		return config
	}
	return config
}

// setupHTTPRoutes configures HTTP endpoints
func setupHTTPRoutes(s *api.Server, adminEnabled bool, m *metrics.Metrics) *http.ServeMux {
	mux := http.NewServeMux()

	// Observability endpoints (no rate limiting)
	mux.Handle("/metrics", promhttp.Handler())
	mux.HandleFunc("/health", s.Health)
	mux.HandleFunc("/ready", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		if _, err := w.Write([]byte(`{"status":"ready"}`)); err != nil {
			log.Error().Err(err).Msg("failed to write ready response")
		}
	})

	// Topic pub/sub endpoints (Fae Chamber message bus)
	mux.HandleFunc("/api/v1/topics/", s.TopicRouter)
	mux.HandleFunc("/api/v1/topics", s.TopicRouter)

	// Room-based API endpoints (legacy chat)
	mux.HandleFunc("/api/v1/join", s.JoinRoom)
	mux.HandleFunc("/api/v1/messages/send", s.SendMessage)
	mux.HandleFunc("/api/v1/messages/delete", s.DeleteMessage)
	mux.HandleFunc("/api/v1/messages", s.GetMessages)

	// Admin endpoints
	if adminEnabled {
		mux.HandleFunc("/api/v1/admin/pending", s.GetPendingMembers)
		mux.HandleFunc("/api/v1/admin/approve", s.ApproveMember)
		mux.HandleFunc("/api/v1/admin/approve-all", s.ApproveAllMembers)
		mux.HandleFunc("/api/v1/admin/deny", s.DenyMember)
	}

	return mux
}

// setupMiddleware creates the middleware chain
func setupMiddleware(handler http.Handler, rateLimiter *middleware.RateLimiter, corsOrigins []string) http.Handler {
	return middleware.Chain(handler,
		middleware.Recovery,                // Recover from panics
		middleware.Logging,                 // Structured logging
		middleware.Metrics,                 // Prometheus metrics
		middleware.SecurityHeaders,         // Security headers
		middleware.CORS(corsOrigins),       // CORS support
		rateLimiter.Middleware,             // Rate limiting
		middleware.Timeout(30*time.Second), // Request timeout
	)
}

// collectSystemMetrics periodically collects system metrics. Returns
// when ctx is canceled (during graceful shutdown).
func collectSystemMetrics(ctx context.Context, m *metrics.Metrics) {
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()

	var mem runtime.MemStats
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			runtime.ReadMemStats(&mem)
			m.GoroutinesActive.Set(float64(runtime.NumGoroutine()))
			m.MemoryAllocated.Set(float64(mem.Alloc))
		}
	}
}

// monitorPendingMembers logs pending member requests every 10 s. Was
// previously named `startAdminCLI`, which was a misnomer — there is no
// CLI here, just a periodic log loop. Approval/denial happens via the
// HTTP API endpoints registered in setupHTTPRoutes. Returns when ctx
// is canceled.
func monitorPendingMembers(ctx context.Context, memberMgr *member.Manager, roomMgr *room.Manager) {
	log.Info().Msg("pending_member_monitor_started")
	log.Info().Msg("use_http_api_endpoints_to_approve_deny_members")

	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			pending := memberMgr.GetAllPending()
			if len(pending) > 0 {
				for _, m := range pending {
					log.Info().
						Str("member_id", m.ID.String()).
						Str("member_name", m.Name).
						Str("room_id", m.RoomID).
						Time("requested_at", m.RequestedAt).
						Msg("pending_member_request")
				}
			}
		}
	}
}

// cleanupExpiredMembers periodically removes expired pending member
// requests. Returns when ctx is canceled.
func cleanupExpiredMembers(ctx context.Context, memberMgr *member.Manager) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			expiredCount := memberMgr.CleanupExpiredPending()
			if expiredCount > 0 {
				log.Info().
					Int("expired_count", expiredCount).
					Msg("cleaned_up_expired_pending_members")
			}
		}
	}
}
