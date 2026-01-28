package main

import (
	"context"
	"flag"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

	"github.com/unheaded/unheaded/cmd/dashboard-backend/internal/metrics"
	"github.com/unheaded/unheaded/cmd/dashboard-backend/internal/packetflow"
	"github.com/unheaded/unheaded/cmd/dashboard-backend/internal/server"
	"github.com/unheaded/unheaded/cmd/dashboard-backend/internal/websocket"
)

var (
	listenAddr = flag.String("listen", ":8080", "HTTP listen address")
	busboyAddr = flag.String("busboy", "localhost:9090", "Busboy server address")
	debug      = flag.Bool("debug", false, "Enable debug logging")
)

func main() {
	flag.Parse()

	// Setup logging
	zerolog.TimeFieldFormat = zerolog.TimeFormatUnix
	if *debug {
		zerolog.SetGlobalLevel(zerolog.DebugLevel)
		log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr})
	} else {
		zerolog.SetGlobalLevel(zerolog.InfoLevel)
	}

	log.Info().
		Str("service", "dashboard-backend").
		Str("version", "0.1.0").
		Msg("starting dashboard backend")

	// Create server config
	config := &server.Config{
		ListenAddr:  *listenAddr,
		BusboyAddr:  *busboyAddr,
		ServiceName: "dashboard-backend",

		WebSocketConfig: &websocket.Config{
			MaxConnections: 100,
			ReadTimeout:    60 * time.Second,
			WriteTimeout:   10 * time.Second,
			BufferSize:     256,
		},

		MetricsConfig: &metrics.Config{
			RetentionPeriod: 1 * time.Hour,
			MaxSeries:       10000,
			FlushInterval:   1 * time.Minute,
		},

		PacketFlowConfig: &packetflow.Config{
			Interval:       100 * time.Millisecond,
			MaxFlows:       50,
			TraceIDPattern: "trace-%d",
		},
	}

	// Create server
	srv, err := server.NewServer(config)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to create server")
	}

	// Start server
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := srv.Start(ctx); err != nil {
		log.Fatal().Err(err).Msg("failed to start server")
	}

	// Wait for interrupt signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	<-sigChan
	log.Info().Msg("shutdown signal received")

	// Graceful shutdown with timeout
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutdownCancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Error().Err(err).Msg("shutdown error")
		os.Exit(1)
	}

	log.Info().Msg("dashboard backend stopped")
}
