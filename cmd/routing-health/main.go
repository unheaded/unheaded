// SPDX-License-Identifier: GPL-3.0-or-later
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"syscall"
	"time"
)

// HealthResponse is the response for the /health endpoint
type HealthResponse struct {
	Status string            `json:"status"`
	Checks map[string]string `json:"checks"`
}

// Checker provides dependency-injected health check functions
type Checker struct {
	CheckFRR  func() string // returns "ok" or "degraded"
	CheckBIRD func() string // returns "ok" or "degraded"
	CheckWG0  func() bool   // returns true if up
	CheckBGP  func() int    // returns count of established BGP sessions
	CheckOSPF func() int    // returns count of OSPF neighbors
	Hostname  string        // hostname for metrics labels
}

// DefaultChecker returns a Checker with real health check implementations
func DefaultChecker() *Checker {
	hostname, _ := os.Hostname()
	return &Checker{
		CheckFRR:  checkFRRReal,
		CheckBIRD: checkBIRDReal,
		CheckWG0:  checkWG0Real,
		CheckBGP:  checkBGPReal,
		CheckOSPF: checkOSPFReal,
		Hostname:  hostname,
	}
}

// Real implementation functions

func checkFRRReal() string {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "systemctl", "is-active", "frr")
	err := cmd.Run()
	if err == nil {
		return "ok"
	}
	return "degraded"
}

func checkBIRDReal() string {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "systemctl", "is-active", "bird")
	err := cmd.Run()
	if err == nil {
		return "ok"
	}
	return "degraded"
}

func checkWG0Real() bool {
	operstateFile := "/sys/class/net/wg0/operstate"
	data, err := os.ReadFile(operstateFile)
	if err != nil {
		return false
	}
	state := strings.TrimSpace(string(data))
	// WireGuard shows "unknown" when up, but we accept both "up" and "unknown"
	return state == "up" || state == "unknown"
}

func checkBGPReal() int {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "vtysh", "-c", "show bgp summary json")
	output, err := cmd.Output()
	if err != nil {
		return 0 // vtysh not available
	}

	var result map[string]interface{}
	if err := json.Unmarshal(output, &result); err != nil {
		return 0
	}

	// Navigate to .ipv4Unicast.peers
	ipv4Unicast, ok := result["ipv4Unicast"].(map[string]interface{})
	if !ok {
		return 0
	}

	peers, ok := ipv4Unicast["peers"].(map[string]interface{})
	if !ok {
		return 0
	}

	// Count peers with peerState == "Established"
	count := 0
	for _, peerData := range peers {
		peerMap, ok := peerData.(map[string]interface{})
		if !ok {
			continue
		}
		state, ok := peerMap["peerState"].(string)
		if ok && state == "Established" {
			count++
		}
	}
	return count
}

func checkOSPFReal() int {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "vtysh", "-c", "show ip ospf neighbor json")
	output, err := cmd.Output()
	if err != nil {
		return 0 // vtysh not available
	}

	var neighbors []map[string]interface{}
	if err := json.Unmarshal(output, &neighbors); err != nil {
		return 0
	}

	return len(neighbors)
}

// HTTP Handlers

func (c *Checker) healthHandler(w http.ResponseWriter, r *http.Request) {
	checks := map[string]string{
		"frr":  c.CheckFRR(),
		"bird": c.CheckBIRD(),
		"wg0":  "degraded",
	}
	if c.CheckWG0() {
		checks["wg0"] = "ok"
	}

	status := "ok"
	for _, v := range checks {
		if v != "ok" {
			status = "degraded"
			break
		}
	}

	resp := HealthResponse{
		Status: status,
		Checks: checks,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func (c *Checker) readyHandler(w http.ResponseWriter, r *http.Request) {
	if !c.CheckWG0() {
		w.WriteHeader(http.StatusServiceUnavailable)
		return
	}
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("OK"))
}

func (c *Checker) metricsHandler(w http.ResponseWriter, r *http.Request) {
	hostname := c.Hostname
	if hostname == "" {
		hostname = "unknown"
	}

	frrStatus := 0.0
	if c.CheckFRR() == "ok" {
		frrStatus = 1.0
	}

	birdStatus := 0.0
	if c.CheckBIRD() == "ok" {
		birdStatus = 1.0
	}

	wg0Status := 0.0
	if c.CheckWG0() {
		wg0Status = 1.0
	}

	bgpCount := c.CheckBGP()
	ospfCount := c.CheckOSPF()

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	fmt.Fprintf(w, "# HELP routing_health_frr_up FRR daemon status\n")
	fmt.Fprintf(w, "# TYPE routing_health_frr_up gauge\n")
	fmt.Fprintf(w, "routing_health_frr_up{host=\"%s\"} %.1f\n", hostname, frrStatus)

	fmt.Fprintf(w, "# HELP routing_health_bird_up BIRD daemon status\n")
	fmt.Fprintf(w, "# TYPE routing_health_bird_up gauge\n")
	fmt.Fprintf(w, "routing_health_bird_up{host=\"%s\"} %.1f\n", hostname, birdStatus)

	fmt.Fprintf(w, "# HELP routing_health_wg0_up WireGuard wg0 interface status\n")
	fmt.Fprintf(w, "# TYPE routing_health_wg0_up gauge\n")
	fmt.Fprintf(w, "routing_health_wg0_up{host=\"%s\"} %.1f\n", hostname, wg0Status)

	fmt.Fprintf(w, "# HELP routing_health_bgp_sessions Established BGP sessions\n")
	fmt.Fprintf(w, "# TYPE routing_health_bgp_sessions gauge\n")
	fmt.Fprintf(w, "routing_health_bgp_sessions{host=\"%s\"} %d\n", hostname, bgpCount)

	fmt.Fprintf(w, "# HELP routing_health_ospf_neighbors OSPF neighbors\n")
	fmt.Fprintf(w, "# TYPE routing_health_ospf_neighbors gauge\n")
	fmt.Fprintf(w, "routing_health_ospf_neighbors{host=\"%s\"} %d\n", hostname, ospfCount)
}

func main() {
	checker := DefaultChecker()
	addr := os.Getenv("ROUTING_HEALTH_ADDR")
	if addr == "" {
		addr = ":8080"
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/health", checker.healthHandler)
	mux.HandleFunc("/ready", checker.readyHandler)
	mux.HandleFunc("/metrics", checker.metricsHandler)

	server := &http.Server{
		Addr:         addr,
		Handler:      mux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
	}

	// Channel for graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGTERM, syscall.SIGINT)

	go func() {
		<-sigChan
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		server.Shutdown(ctx)
	}()

	log.Printf("Starting routing-health server on %s", addr)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("Server error: %v", err)
	}
}
