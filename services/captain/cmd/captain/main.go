// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"sync"
	"syscall"

	"unheaded/pkg/discovery"
	"unheaded/pkg/logagg"
	"unheaded/pkg/transport"
	wotanClient "unheaded/pkg/wotan-client"
	"unheaded/services/captain"
)

// main is the entry point for the captain service
func main() {
	if err := run(); err != nil {
		log.Fatalf("captain service failed: %v", err)
	}
}

// run executes the captain service
func run() error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Service discovery registration (best-effort, nil conn until transport.Connect)
	discovery.SetupServiceDiscovery(ctx, nil, "captain", 19002)

	// Configuration
	wotanAddr := getEnv("WOTAN_ADDR", "localhost:18001")
	httpAddr := getEnv("HTTP_ADDR", "0.0.0.0:19002")
	dataPath := getEnv("DATA_PATH", "/var/lib/unheaded/captain")

	// Transport config
	transportCfg := transport.DefaultConfig()
	transport.ConfigFromEnv(&transportCfg)
	transportCfg.WotanGRPCAddr = wotanAddr

	// Health server
	healthSrv := transport.NewHealthServer("captain")

	log.Printf("captain service starting (http:%s, wotan:%s, transport:%s)", httpAddr, wotanAddr, transportCfg.Type)

	// Create storage
	storage, err := captain.NewFileStorage(dataPath)
	if err != nil {
		return fmt.Errorf("create storage: %w", err)
	}

	// Create Wotan client
	wotan, err := wotanClient.NewClient(wotanAddr)
	if err != nil {
		healthSrv.SetGRPCStatus(false)
		return fmt.Errorf("create wotan client: %w", err)
	}
	defer wotan.Close()

	// Log aggregation publisher — forwards structured logs to Wotan
	var logConn transport.Connection
	if wotan != nil {
		logConn, _ = transport.Connect(ctx, transportCfg) // best-effort; nil on failure
	}
	_ = logagg.NewPublisher("captain", logConn)

	// Subscribe to critical alerts
	if _, err := wotan.Subscribe(ctx, "alerts.critical", "captain"); err != nil {
		log.Printf("warning: failed to subscribe to alerts: %v", err)
	}

	// Create captain service
	svc, err := captain.NewService(captain.Config{
		Storage: storage,
		Wotan:   wotan,
		MetricsCallback: func(metric string, value interface{}) {
			// Log metrics (would be sent to Prometheus in production)
			log.Printf("metric: %s = %v", metric, value)
		},
	})
	if err != nil {
		return fmt.Errorf("create service: %w", err)
	}
	defer svc.Close()

	// Create HTTP server
	httpServer, err := captain.NewHTTPServer(svc, httpAddr)
	if err != nil {
		return fmt.Errorf("create http server: %w", err)
	}

	if err := httpServer.Start(); err != nil {
		return fmt.Errorf("start http server: %w", err)
	}

	log.Printf("captain service running on http://%s", httpAddr)

	// Signal handling
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Start alert listener
	var wg sync.WaitGroup
	wg.Add(1)
	go listenForAlerts(ctx, &wg, wotan)

	// Wait for signal
	sig := <-sigChan
	log.Printf("received signal: %v", sig)

	// Graceful shutdown
	cancel()

	if err := httpServer.Stop(); err != nil {
		log.Printf("error stopping http server: %v", err)
	}

	wg.Wait()

	log.Println("captain service stopped")
	return nil
}

// listenForAlerts listens for critical alerts from Wotan
func listenForAlerts(ctx context.Context, wg *sync.WaitGroup, wotan *wotanClient.Client) {
	defer wg.Done()

	msgs, err := wotan.StreamMessages(ctx, "alerts.critical")
	if err != nil {
		log.Printf("warning: failed to stream alerts: %v", err)
		return
	}

	for {
		select {
		case <-ctx.Done():
			return
		case msg := <-msgs:
			if msg == nil {
				return
			}
			log.Printf("received alert: %v", msg)
		}
	}
}

// getEnv gets an environment variable with a default value
func getEnv(key, defaultValue string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return defaultValue
}
