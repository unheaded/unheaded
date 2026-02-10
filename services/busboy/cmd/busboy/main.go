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

	chatpb "unheaded/services/busboy/proto"
	"unheaded/services/busboy/internal/api"
	"unheaded/services/busboy/internal/busboy"
	grpcservice "unheaded/services/busboy/internal/grpc"
	"unheaded/services/busboy/internal/logger"
	"unheaded/services/busboy/internal/member"
	"unheaded/services/busboy/internal/metrics"
	"unheaded/services/busboy/internal/middleware"
	"unheaded/services/busboy/internal/room"
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

	// CORS
	CORSOrigins []string
}

func main() {
	// Parse command line flags
	config := parseFlags()

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

	// Start metrics collector
	go collectSystemMetrics(m)

	// Initialize managers
	roomManager := room.NewManager(config.BufferSize)
	memberManager := member.NewManager()
	messageBusboy := busboy.NewBusboy()

	log.Info().Msg("initialized_core_managers")

	// Create API server
	apiServer := api.NewServer(roomManager, memberManager, messageBusboy, config.PendingApprovalTimeout)
	apiServer.InitTopics() // Enable topic pub/sub (Fae Chamber message bus)

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

	httpServer := &http.Server{
		Addr:         fmt.Sprintf(":%d", config.HTTPPort),
		Handler:      httpHandler,
		ReadTimeout:  config.ReadTimeout,
		WriteTimeout: config.WriteTimeout,
		IdleTimeout:  config.IdleTimeout,
	}

	// Configure TLS if enabled
	if config.EnableTLS {
		if config.TLSCertFile == "" || config.TLSKeyFile == "" {
			log.Fatal().Msg("TLS enabled but cert/key files not provided")
		}

		tlsConfig := &tls.Config{
			MinVersion:               tls.VersionTLS13,
			PreferServerCipherSuites: true,
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

	// Create and start gRPC streaming service
	chatService := grpcservice.NewChatService(roomManager, memberManager, messageBusboy)

	go func() {
		lis, err := net.Listen("tcp", fmt.Sprintf(":%d", config.GRPCPort))
		if err != nil {
			log.Fatal().Err(err).Msg("failed_to_listen_grpc_port")
		}

		var grpcServer *grpc.Server
		if config.EnableTLS {
			creds, err := credentials.NewServerTLSFromFile(config.TLSCertFile, config.TLSKeyFile)
			if err != nil {
				log.Fatal().Err(err).Msg("failed_to_load_tls_credentials")
			}
			grpcServer = grpc.NewServer(grpc.Creds(creds))
		} else {
			grpcServer = grpc.NewServer()
		}

		// Register gRPC service implementation
		chatpb.RegisterChatStreamServer(grpcServer, chatService)

		log.Info().
			Int("port", config.GRPCPort).
			Bool("tls", config.EnableTLS).
			Msg("starting_grpc_server")

		if err := grpcServer.Serve(lis); err != nil {
			log.Fatal().Err(err).Msg("grpc_server_error")
		}
	}()

	// Start admin CLI in goroutine if enabled
	if config.AdminEnabled {
		go startAdminCLI(memberManager, roomManager)
	}

	// Start periodic cleanup of expired pending members
	go cleanupExpiredMembers(memberManager)

	// Wait for shutdown signal
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop

	log.Info().Msg("shutdown_signal_received")

	// Graceful shutdown
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	log.Info().Msg("shutting_down_http_server")
	if err := httpServer.Shutdown(ctx); err != nil {
		log.Error().Err(err).Msg("http_server_shutdown_error")
	}

	log.Info().Msg("closing_message_busboy")
	messageBusboy.Close()

	log.Info().Msg("server_stopped_gracefully")
}

// parseFlags parses command line flags and returns configuration
func parseFlags() Config {
	config := Config{}

	// Server flags
	flag.IntVar(&config.BufferSize, "buffer-size", 10000, "Ring buffer size per room")
	flag.IntVar(&config.HTTPPort, "http-port", 8080, "HTTP REST API port")
	flag.IntVar(&config.GRPCPort, "grpc-port", 9090, "gRPC streaming port")
	flag.BoolVar(&config.AdminEnabled, "admin", true, "Enable admin endpoints")

	// TLS flags
	flag.BoolVar(&config.EnableTLS, "enable-tls", false, "Enable TLS 1.3")
	flag.StringVar(&config.TLSCertFile, "tls-cert", "", "TLS certificate file")
	flag.StringVar(&config.TLSKeyFile, "tls-key", "", "TLS key file")

	// Logging flags
	flag.StringVar(&config.LogLevel, "log-level", "info", "Log level (debug, info, warn, error)")
	flag.BoolVar(&config.LogPretty, "log-pretty", false, "Pretty print logs for development")

	// Rate limiting flags
	flag.Float64Var(&config.RateLimit, "rate-limit", 100.0, "Rate limit (requests per second)")
	flag.IntVar(&config.RateBurst, "rate-burst", 200, "Rate limit burst size")
	config.RateCleanup = 5 * time.Minute

	// Timeout flags
	config.ReadTimeout = 15 * time.Second
	config.WriteTimeout = 15 * time.Second
	config.IdleTimeout = 60 * time.Second

	var pendingApprovalTimeout time.Duration
	flag.DurationVar(&pendingApprovalTimeout, "pending-approval-timeout", 1*time.Hour, "Timeout for pending member approval requests")

	// CORS flags
	config.CORSOrigins = []string{"*"} // Configure as needed

	flag.Parse()
	config.PendingApprovalTimeout = pendingApprovalTimeout
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

// collectSystemMetrics periodically collects system metrics
func collectSystemMetrics(m *metrics.Metrics) {
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()

	var mem runtime.MemStats
	for range ticker.C {
		runtime.ReadMemStats(&mem)
		m.GoroutinesActive.Set(float64(runtime.NumGoroutine()))
		m.MemoryAllocated.Set(float64(mem.Alloc))
	}
}

// startAdminCLI monitors and logs pending member requests
func startAdminCLI(memberMgr *member.Manager, roomMgr *room.Manager) {
	log.Info().Msg("admin_cli_started")
	log.Info().Msg("use_http_api_endpoints_to_approve_deny_members")

	// Periodic check for pending members
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
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

// cleanupExpiredMembers periodically removes expired pending member requests
func cleanupExpiredMembers(memberMgr *member.Manager) {
	// Run cleanup every 30 seconds
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		expiredCount := memberMgr.CleanupExpiredPending()
		if expiredCount > 0 {
			log.Info().
				Int("expired_count", expiredCount).
				Msg("cleaned_up_expired_pending_members")
		}
	}
}
