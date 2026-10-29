// SPDX-License-Identifier: GPL-3.0-or-later
//
// upc-tty-bridge — WebSocket bridge for UPC tty streams (Mode A demo surface).
// ASCEND-LINUX Phase 1 deliverable per
// references/battle-plan-ascend-linux-2026-05-08.md.
//
// Pattern after cmd/doom-bridge/main.go but frames TTY BYTES (xterm.js
// consumable) instead of framebuffer pixels. Subscribes to the BPF tty
// output stream + Wotan compute.tty.{instance} topic, ships bytes over
// WS to browser xterm.js client; queues keystrokes from WS per instance for
// `upc-bootctl console` to drain (GET /api/v1/kbd/drain, loopback only) into
// the guest's KBD ring (kbd.go).
//
// Default listen: 127.0.0.1:26100 (UPC user-app band 26000-26666 per
// pkg/ports). Typing into the UPC shell is a write path; widen --listen only
// deliberately.

package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/gorilla/websocket"

	"unheaded/pkg/cmdline"
	"unheaded/pkg/httputil"
)

const (
	serviceVersion = "0.1.0-skeleton"
	serviceName    = "upc-tty-bridge"
	defaultPort    = 26100
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 4096,
	// Same origin, or an exact entry of WS_ALLOWED_ORIGINS. Accepting every
	// Origin let any web page open in a LAN browser read the UPC console,
	// and type into it once KBD_MAP input lands; the LAN posture (ADR-049)
	// does not stop a page the operator's own browser loads.
	CheckOrigin: httputil.SameOriginOrAllowed(httputil.OriginsFromEnv("WS_ALLOWED_ORIGINS")),
}

// Subscriber tracks one connected browser xterm.js session.
type Subscriber struct {
	conn     *websocket.Conn
	instance uint8 // UPC instance ID (low byte of CPU_MAP key)
	out      chan []byte
}

type Hub struct {
	mu          sync.RWMutex
	subscribers map[*Subscriber]struct{}
	kbd         *kbdQueue
}

func newHub() *Hub {
	return &Hub{
		subscribers: make(map[*Subscriber]struct{}),
		kbd:         newKbdQueue(kbdQueueCap),
	}
}

func (h *Hub) add(s *Subscriber) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.subscribers[s] = struct{}{}
}

func (h *Hub) remove(s *Subscriber) {
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.subscribers, s)
	close(s.out)
}

// broadcastTty fan-outs a tty byte chunk to all subscribers watching
// the given instance.
func (h *Hub) broadcastTty(instance uint8, data []byte) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	for s := range h.subscribers {
		if s.instance == instance {
			select {
			case s.out <- data:
			default:
				// drop on slow consumer; xterm.js can re-fetch via /api/v1/tty/snapshot
			}
		}
	}
}

// parseInstanceParam decodes the ?instance=<decimal> query value into a UPC
// instance ID (low byte of CPU_MAP key). Returns the default 0xDE (xv6
// first-boot target) for: empty input, non-numeric input, or out-of-range
// values (<0 or >255). Pure function — unit-testable.
func parseInstanceParam(raw string) uint8 {
	const defaultInstance uint8 = 0xDE
	if raw == "" {
		return defaultInstance
	}
	var n int
	if _, err := fmt.Sscanf(raw, "%d", &n); err != nil {
		return defaultInstance
	}
	if n < 0 || n > 255 {
		return defaultInstance
	}
	return uint8(n)
}

func (h *Hub) handleWs(w http.ResponseWriter, r *http.Request) {
	instance := parseInstanceParam(r.URL.Query().Get("instance"))

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	sub := &Subscriber{conn: conn, instance: instance, out: make(chan []byte, 64)}
	h.add(sub)
	defer h.remove(sub)

	// Send a hello banner so the browser knows the channel is live.
	hello := fmt.Sprintf("upc-tty-bridge: attached to UPC instance 0x%02X\r\n", instance)
	_ = conn.WriteMessage(websocket.TextMessage, []byte(hello))

	// Writer goroutine — push tty bytes from out → WS.
	done := make(chan struct{})
	go func() {
		defer close(done)
		for data := range sub.out {
			if err := conn.WriteMessage(websocket.BinaryMessage, data); err != nil {
				return
			}
		}
	}()

	// Reader loop — queue browser keystrokes for the console's drain.
	for {
		_, msg, err := conn.ReadMessage()
		if err != nil {
			return
		}
		if kept := h.kbd.push(instance, msg); kept < len(msg) {
			fmt.Fprintf(os.Stderr, "[upc-tty-bridge] kbd queue full for instance 0x%02X: dropped %d bytes\n",
				instance, len(msg)-kept)
		}
	}
}

// pollTtyStream is the placeholder for the actual BPF-map-read /
// Wotan-topic-subscribe loop. For now it generates a heartbeat byte every
// 5s so the WS channel stays warm.
func (h *Hub) pollTtyStream(ctx context.Context, instance uint8) {
	tick := time.NewTicker(5 * time.Second)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case t := <-tick.C:
			line := fmt.Sprintf("[heartbeat %s] instance=0x%02X (real BPF tty read NYI)\r\n",
				t.Format(time.RFC3339), instance)
			h.broadcastTty(instance, []byte(line))
		}
	}
}

func handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	resp := map[string]any{
		"service": serviceName,
		"version": serviceVersion,
		"status":  "ok",
		"mode":    "skeleton",
	}
	_ = json.NewEncoder(w).Encode(resp)
}

// ttyIngestRequest is the JSON body POSTed by upc-bootctl's TTY poller
// to /api/v1/tty/ingest. Bytes are JSON-array-of-numbers (not base64) so
// the body shows readable in curl/logs during dev.
type ttyIngestRequest struct {
	Instance uint8 `json:"instance"`
	Bytes    []int `json:"bytes"`
}

// handleTtyIngest accepts TTY bytes from upc-bootctl and fans them out
// to all WebSocket subscribers watching the same instance.
//
// Response: 204 No Content on success, 400 on bad body.
func (h *Hub) handleTtyIngest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST only", http.StatusMethodNotAllowed)
		return
	}
	var req ttyIngestRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, fmt.Sprintf("bad JSON: %v", err), http.StatusBadRequest)
		return
	}
	bytes := make([]byte, len(req.Bytes))
	for i, v := range req.Bytes {
		if v < 0 || v > 255 {
			http.Error(w, fmt.Sprintf("byte %d out of range: %d", i, v), http.StatusBadRequest)
			return
		}
		bytes[i] = byte(v)
	}
	h.broadcastTty(req.Instance, bytes)
	w.WriteHeader(http.StatusNoContent)
}

func main() {
	listen := flag.String("listen", fmt.Sprintf("127.0.0.1:%d", defaultPort),
		"listen address (host:port); loopback by default")
	port := flag.Int("port", 0, "deprecated: listen on 127.0.0.1:<port> (use --listen)")
	cmdline.Parse(cmdline.Spec{
		Name:     "upc-tty-bridge",
		Purpose:  "Bridge UPC TTY byte streams to browser terminals over WebSocket (ASCEND-LINUX Mode A).",
		Examples: []string{"upc-tty-bridge --port 26100"},
	})
	addr := *listen
	if *port != 0 {
		addr = fmt.Sprintf("127.0.0.1:%d", *port)
	}

	hub := newHub()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Heartbeat for instance 0xDE (default xv6 first-boot target).
	go hub.pollTtyStream(ctx, 0xDE)

	mux := http.NewServeMux()
	mux.HandleFunc("/health", handleHealth)
	mux.HandleFunc("/console", hub.handleWs)
	mux.HandleFunc("/api/v1/tty/ingest", hub.handleTtyIngest)
	mux.HandleFunc("/api/v1/kbd/drain", hub.handleKbdDrain)
	mux.HandleFunc("/metrics", hub.handleMetrics)
	// Static asset endpoint for xterm.js page (served from ../../dashboard/).
	mux.Handle("/", http.FileServer(http.Dir("../../dashboard/")))

	fmt.Fprintf(os.Stderr, "[upc-tty-bridge] listening on %s\n", addr)
	fmt.Fprintf(os.Stderr, "[upc-tty-bridge] connect: ws://%s/console?instance=222\n", addr)

	srv := &http.Server{
		Addr:              addr,
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
	}

	// Graceful shutdown on SIGINT/SIGTERM.
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sig
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer shutdownCancel()
		_ = srv.Shutdown(shutdownCtx)
	}()

	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		fmt.Fprintf(os.Stderr, "[upc-tty-bridge] server error: %v\n", err)
		os.Exit(1)
	}
}
