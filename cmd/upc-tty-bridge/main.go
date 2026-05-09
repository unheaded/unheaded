// SPDX-License-Identifier: GPL-3.0-or-later
//
// upc-tty-bridge — WebSocket bridge for UPC tty streams (Mode A demo surface).
// ASCEND-LINUX Phase 1 deliverable per
// references/battle-plan-ascend-linux-2026-05-08.md.
//
// Pattern after cmd/doom-bridge/main.go but frames TTY BYTES (xterm.js
// consumable) instead of framebuffer pixels. Subscribes to the BPF tty
// output stream + Wotan compute.tty.{instance} topic, ships bytes over
// WS to browser xterm.js client; captures keystrokes from WS and writes
// to KBD_MAP.
//
// Status: skeleton. Real BPF integration pending — same way doom-bridge
// uses pinned BPF maps, this will after upc-bootctl ships full boot path.
//
// Default port: 26100 (UPC user-app band 26000-26666 per pkg/ports).

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
)

const (
	serviceVersion = "0.1.0-skeleton"
	serviceName    = "upc-tty-bridge"
	defaultPort    = 26100
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 4096,
	// LAN-only deployment per ADR-049 + raft/CLAUDE.md T8 disposition;
	// CSRF mitigation is the LAN posture, not Origin-checking here.
	CheckOrigin: func(r *http.Request) bool { return true },
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
}

func newHub() *Hub {
	return &Hub{subscribers: make(map[*Subscriber]struct{})}
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

func (h *Hub) handleWs(w http.ResponseWriter, r *http.Request) {
	// Query param: ?instance=222 (decimal)
	instStr := r.URL.Query().Get("instance")
	var instance uint8 = 0xDE
	if instStr != "" {
		var n int
		_, err := fmt.Sscanf(instStr, "%d", &n)
		if err == nil && n >= 0 && n <= 255 {
			instance = uint8(n)
		}
	}

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	sub := &Subscriber{conn: conn, instance: instance, out: make(chan []byte, 64)}
	h.add(sub)
	defer h.remove(sub)

	// Send a hello banner so the browser knows the channel is live.
	hello := fmt.Sprintf("upc-tty-bridge: attached to UPC instance 0x%02X\r\n", instance)
	conn.WriteMessage(websocket.TextMessage, []byte(hello))

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

	// Reader loop — capture browser keystrokes → KBD_MAP.
	for {
		_, msg, err := conn.ReadMessage()
		if err != nil {
			return
		}
		// TODO: write each byte of msg to KBD_MAP at the keyboard MMIO.
		// Placeholder: log it.
		fmt.Fprintf(os.Stderr, "[upc-tty-bridge] kbd input from instance 0x%02X: %d bytes\n", instance, len(msg))
	}
	_ = done
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

func main() {
	port := flag.Int("port", defaultPort, "WebSocket listen port")
	flag.Parse()

	hub := newHub()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Heartbeat for instance 0xDE (default xv6 first-boot target).
	go hub.pollTtyStream(ctx, 0xDE)

	mux := http.NewServeMux()
	mux.HandleFunc("/health", handleHealth)
	mux.HandleFunc("/console", hub.handleWs)
	// Static asset endpoint for xterm.js page (served from ../../dashboard/).
	mux.Handle("/", http.FileServer(http.Dir("../../dashboard/")))

	addr := fmt.Sprintf(":%d", *port)
	fmt.Fprintf(os.Stderr, "[upc-tty-bridge] listening on %s\n", addr)
	fmt.Fprintf(os.Stderr, "[upc-tty-bridge] connect: ws://localhost:%d/console?instance=222\n", *port)

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
