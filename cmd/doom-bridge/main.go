// Package main implements doom-bridge: Fenrir's Eye service for Unheaded.
//
// Fenrir's Eye reads screen framebuffer (SCREEN_MAP @ 320x200 palette) and CPU state
// from pinned eBPF maps (via raw BPF syscalls). Applies Doom palette
// (256 colors -> RGB), and pushes frames via WebSocket to browser clients.
// Handles keyboard input (KBD_MAP writes) and metadata streaming (CPU stats).
//
// Design: Minimal. No Wotan dependency for WS1 (MVP). Direct BPF map reads.
//
// Usage:
//
//	doom-bridge [--port 6660] [--map-path /sys/fs/bpf/unheaded/doom-ring/maps] [--dry-run] [--static /path/to/dashboard]
//
// Protocol: WebSocket binary frames [0x01 tag] + [RGBA pixel data 256000 bytes] at ~30fps.
//
//	JSON stats frames with CPU state at ~2fps.
//
// Version: 0.1.0-alpha
package main

import (
	"encoding/binary"
	"encoding/json"
	"flag"
	"fmt"
	"math"
	"math/rand"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/gorilla/websocket"
)

// Service metadata.
const (
	serviceVersion = "0.1.0-alpha"
	serviceName    = "doom-bridge"
)

// logf logs a structured message to stderr with timestamp and level.
func logf(level, msg string, kvPairs ...interface{}) {
	timestamp := time.Now().Format(time.RFC3339)
	fmt.Fprintf(os.Stderr, "[%s] %s: %s", timestamp, level, msg)
	for i := 0; i < len(kvPairs); i += 2 {
		if i+1 < len(kvPairs) {
			fmt.Fprintf(os.Stderr, " %v=%v", kvPairs[i], kvPairs[i+1])
		}
	}
	fmt.Fprintln(os.Stderr)
}

// Protocol tags for binary WebSocket frames.
const (
	tagScreen = 0x01 // Server -> Client: screen framebuffer
	tagKbd    = 0x02 // Client -> Server: keyboard event
)

// Screen dimensions.
const (
	screenWidth  = 320
	screenHeight = 200
	screenSize   = screenWidth * screenHeight // 64000 bytes
)

// statsMessage is the JSON stats broadcast to clients.
type statsMessage struct {
	Type    string   `json:"type"`
	Packets uint64   `json:"packets"`
	Ticks   uint64   `json:"ticks"`
	Insns   uint64   `json:"insns"`
	Halted  uint64   `json:"halted"`
	PC      uint32   `json:"pc"`
	Flags   uint8    `json:"flags"`
	Regs    []uint32 `json:"regs"`
}

// client represents a connected WebSocket client.
type client struct {
	conn *websocket.Conn
	mu   sync.Mutex
}

// bridge holds the server state.
type bridge struct {
	port    int
	mapPath string
	dryRun  bool
	static  string

	// BPF map handles (nil in dry-run mode)
	screenMap *BPFMap
	kbdMap    *BPFMap
	statsMap  *BPFMap
	cpuMap    *BPFMap

	// Whether batch read is supported for screen map
	batchSupported bool

	// Connected WebSocket clients
	clients   map[*client]struct{}
	clientsMu sync.RWMutex

	upgrader websocket.Upgrader

	// Metrics counters (atomic)
	frameCount int64
	byteCount  int64
	errorCount int64
}

func main() {
	port := flag.Int("port", 6660, "WebSocket server port")
	mapPath := flag.String("map-path", "/sys/fs/bpf/unheaded/doom-ring/maps", "BPF map pin directory")
	dryRun := flag.Bool("dry-run", false, "Run without BPF maps (serve fake gradient data)")
	staticDir := flag.String("static", "", "Static files directory (default: demos/doom/ relative to binary or CWD)")
	flag.Parse()

	// Resolve static directory
	static := *staticDir
	if static == "" {
		// Try relative to working directory first
		candidates := []string{
			"demos/doom",
			filepath.Join(filepath.Dir(os.Args[0]), "..", "..", "demos", "doom"),
		}
		for _, c := range candidates {
			if info, err := os.Stat(c); err == nil && info.IsDir() {
				static = c
				break
			}
		}
		if static == "" {
			static = "demos/doom"
		}
	}

	b := &bridge{
		port:    *port,
		mapPath: *mapPath,
		dryRun:  *dryRun,
		static:  static,
		clients: make(map[*client]struct{}),
		upgrader: websocket.Upgrader{
			ReadBufferSize:  1024,
			WriteBufferSize: screenSize*4 + 1024, // RGBA frame (256000) + tag + overhead
			CheckOrigin: func(r *http.Request) bool {
				return true // Allow all origins for development
			},
		},
	}

	// Open BPF maps unless in dry-run mode
	if !b.dryRun {
		b.openMaps()
	} else {
		logf("INFO", "BPF maps disabled", "mode", "dry-run")
	}

	// Setup HTTP routes
	mux := http.NewServeMux()
	mux.HandleFunc("/ws", b.handleWebSocket)
	mux.HandleFunc("/health", b.handleHealth)
	mux.HandleFunc("/ready", b.handleReady)
	mux.HandleFunc("/metrics", b.handleMetrics)
	mux.Handle("/", http.FileServer(http.Dir(b.static)))

	server := &http.Server{
		Addr:         fmt.Sprintf(":%d", b.port),
		Handler:      mux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
	}

	// Start polling loops
	stop := make(chan struct{})
	var wg sync.WaitGroup

	wg.Add(2)
	go b.screenLoop(stop, &wg)
	go b.statsLoop(stop, &wg)

	// Handle shutdown signals
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigCh
		logf("INFO", "Shutting down...")
		close(stop)
		server.Close()
	}()

	logf("INFO", "doom-bridge listening", "port", b.port, "dry_run", b.dryRun, "version", serviceVersion)
	logf("INFO", "endpoints", "ws", fmt.Sprintf("ws://localhost:%d/ws", b.port),
		"viewer", fmt.Sprintf("http://localhost:%d/", b.port),
		"health", fmt.Sprintf("http://localhost:%d/health", b.port),
		"ready", fmt.Sprintf("http://localhost:%d/ready", b.port),
		"metrics", fmt.Sprintf("http://localhost:%d/metrics", b.port))
	if !b.dryRun {
		logf("INFO", "BPF maps", "path", b.mapPath)
	}

	if err := server.ListenAndServe(); err != http.ErrServerClosed {
		logf("FATAL", "HTTP server error", "err", err)
		os.Exit(1)
	}

	wg.Wait()
	b.closeMaps()
	logf("INFO", "doom-bridge stopped")
}

// openMaps opens all pinned BPF maps. Non-fatal if individual maps are missing.
func (b *bridge) openMaps() {
	var err error

	b.screenMap, err = openBPFMap(filepath.Join(b.mapPath, "SCREEN_MAP"))
	if err != nil {
		logf("WARN", "SCREEN_MAP not available", "err", err)
	}

	b.kbdMap, err = openBPFMap(filepath.Join(b.mapPath, "KBD_MAP"))
	if err != nil {
		logf("WARN", "KBD_MAP not available", "err", err)
	}

	b.statsMap, err = openBPFMap(filepath.Join(b.mapPath, "STATS"))
	if err != nil {
		logf("WARN", "STATS map not available", "err", err)
	}

	b.cpuMap, err = openBPFMap(filepath.Join(b.mapPath, "CPU_MAP"))
	if err != nil {
		logf("WARN", "CPU_MAP not available", "err", err)
	}

	// Probe batch support by attempting a small batch read
	if b.screenMap != nil {
		_, _, _, probeErr := b.screenMap.LookupBatch(1, 4, 1)
		if probeErr == nil {
			b.batchSupported = true
			logf("INFO", "BPF batch lookup supported (fast screen reads)")
		} else {
			logf("INFO", "BPF batch lookup not available, using individual reads", "err", probeErr)
		}
	}
}

// closeMaps closes all open BPF map file descriptors.
func (b *bridge) closeMaps() {
	if b.screenMap != nil {
		b.screenMap.Close()
	}
	if b.kbdMap != nil {
		b.kbdMap.Close()
	}
	if b.statsMap != nil {
		b.statsMap.Close()
	}
	if b.cpuMap != nil {
		b.cpuMap.Close()
	}
}

// handleHealth returns a simple health check response.
func (b *bridge) handleHealth(w http.ResponseWriter, r *http.Request) {
	b.clientsMu.RLock()
	numClients := len(b.clients)
	b.clientsMu.RUnlock()

	w.Header().Set("Content-Type", "application/json")
	fmt.Fprintf(w, `{"status":"ok","clients":%d,"dry_run":%v}`, numClients, b.dryRun)
}

// handleReady returns readiness status. Service is ready when BPF maps are open
// (or in dry-run mode). Browser clients can poll /ready before connecting WebSocket.
func (b *bridge) handleReady(w http.ResponseWriter, r *http.Request) {
	var ready bool

	if b.dryRun {
		ready = true // Synthetic mode is always ready
	} else {
		ready = b.screenMap != nil // Only truly ready if we can read screen
	}

	w.Header().Set("Content-Type", "application/json")
	if ready {
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, `{"status":"ready","service":"%s","version":"%s"}`, serviceName, serviceVersion)
	} else {
		w.WriteHeader(http.StatusServiceUnavailable)
		fmt.Fprint(w, `{"status":"not_ready","reason":"BPF maps not accessible"}`)
	}
}

// handleMetrics returns Prometheus-format metrics.
func (b *bridge) handleMetrics(w http.ResponseWriter, r *http.Request) {
	b.clientsMu.RLock()
	numClients := len(b.clients)
	b.clientsMu.RUnlock()

	w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")

	fmt.Fprintf(w, "# HELP unheaded_doom_bridge_clients Connected WebSocket clients\n")
	fmt.Fprintf(w, "# TYPE unheaded_doom_bridge_clients gauge\n")
	fmt.Fprintf(w, "unheaded_doom_bridge_clients %d\n\n", numClients)

	fmt.Fprintf(w, "# HELP unheaded_doom_bridge_frames_total Total frames sent to all clients\n")
	fmt.Fprintf(w, "# TYPE unheaded_doom_bridge_frames_total counter\n")
	fmt.Fprintf(w, "unheaded_doom_bridge_frames_total %d\n\n", atomic.LoadInt64(&b.frameCount))

	fmt.Fprintf(w, "# HELP unheaded_doom_bridge_bytes_sent_total Total bytes sent over WebSocket\n")
	fmt.Fprintf(w, "# TYPE unheaded_doom_bridge_bytes_sent_total counter\n")
	fmt.Fprintf(w, "unheaded_doom_bridge_bytes_sent_total %d\n\n", atomic.LoadInt64(&b.byteCount))

	fmt.Fprintf(w, "# HELP unheaded_doom_bridge_errors_total Total errors encountered\n")
	fmt.Fprintf(w, "# TYPE unheaded_doom_bridge_errors_total counter\n")
	fmt.Fprintf(w, "unheaded_doom_bridge_errors_total %d\n", atomic.LoadInt64(&b.errorCount))
}

// handleWebSocket upgrades HTTP to WebSocket and manages the client lifecycle.
func (b *bridge) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := b.upgrader.Upgrade(w, r, nil)
	if err != nil {
		logf("ERROR", "WebSocket upgrade error", "err", err)
		return
	}

	c := &client{conn: conn}

	b.clientsMu.Lock()
	b.clients[c] = struct{}{}
	numClients := len(b.clients)
	b.clientsMu.Unlock()

	logf("INFO", "Client connected", "total", numClients)

	// Read loop for keyboard input from this client
	go b.readLoop(c)
}

// readLoop reads messages from a client (keyboard events) until disconnect.
func (b *bridge) readLoop(c *client) {
	defer func() {
		b.clientsMu.Lock()
		delete(b.clients, c)
		numClients := len(b.clients)
		b.clientsMu.Unlock()

		c.conn.Close()
		logf("INFO", "Client disconnected", "total", numClients)
	}()

	for {
		msgType, data, err := c.conn.ReadMessage()
		if err != nil {
			if !websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway) {
				logf("WARN", "WebSocket read error", "err", err)
			}
			return
		}

		// Only process binary messages with keyboard tag
		if msgType != websocket.BinaryMessage {
			continue
		}
		if len(data) < 1 || data[0] != tagKbd {
			continue
		}

		// Expected format: [0x02, scancode_lo, scancode_hi, pressed]
		if len(data) < 4 {
			logf("WARN", "Malformed keyboard message", "bytes", len(data))
			continue
		}

		scancode := binary.LittleEndian.Uint16(data[1:3])
		pressed := data[3] != 0

		if b.kbdMap != nil {
			if err := writeKbdMap(b.kbdMap, scancode, pressed); err != nil {
				logf("ERROR", "KBD_MAP write error", "err", err)
			}
		}
	}
}

// screenLoop polls the SCREEN_MAP at ~30fps and broadcasts frames to all clients.
func (b *bridge) screenLoop(stop chan struct{}, wg *sync.WaitGroup) {
	defer wg.Done()

	ticker := time.NewTicker(time.Second / 30) // ~33ms per frame
	defer ticker.Stop()

	localFrameCount := uint64(0)

	for {
		select {
		case <-stop:
			return
		case <-ticker.C:
			var pixels []byte
			var err error

			if b.dryRun || b.screenMap == nil {
				pixels = generateDryRunScreen(localFrameCount)
			} else if b.batchSupported {
				pixels, err = readScreenBatch(b.screenMap)
				if err != nil {
					logf("WARN", "Screen batch read error", "err", err)
					pixels, err = readScreenIndividual(b.screenMap)
					if err != nil {
						logf("ERROR", "Screen read failed", "err", err)
						atomic.AddInt64(&b.errorCount, 1)
						continue
					}
				}
			} else {
				pixels, err = readScreenIndividual(b.screenMap)
				if err != nil {
					logf("ERROR", "Screen read error", "err", err)
					atomic.AddInt64(&b.errorCount, 1)
					continue
				}
			}

			// Convert palette indices to RGBA (64000 bytes -> 256000 bytes)
			rgba := screenBufferToRGBA(pixels)

			// Build binary frame: [tag] + [RGBA data]
			frame := make([]byte, 1+len(rgba))
			frame[0] = tagScreen
			copy(frame[1:], rgba)

			b.broadcastBinary(frame)
			localFrameCount++
			atomic.AddInt64(&b.frameCount, 1)
			atomic.AddInt64(&b.byteCount, int64(len(frame)))
		}
	}
}

// statsLoop polls STATS and CPU_MAP at ~2fps and broadcasts as JSON.
func (b *bridge) statsLoop(stop chan struct{}, wg *sync.WaitGroup) {
	defer wg.Done()

	ticker := time.NewTicker(500 * time.Millisecond) // 2fps
	defer ticker.Stop()

	tick := uint64(0)

	for {
		select {
		case <-stop:
			return
		case <-ticker.C:
			var msg statsMessage
			msg.Type = "stats"

			if b.dryRun || (b.statsMap == nil && b.cpuMap == nil) {
				// Synthetic stats for dry-run
				msg.Packets = tick * 1200
				msg.Ticks = tick * 60
				msg.Insns = tick * 85000
				msg.Halted = 0
				msg.PC = uint32((tick * 7) % 65536)
				msg.Flags = 0
				msg.Regs = make([]uint32, 16)
				msg.Regs[0] = uint32(tick)
				msg.Regs[15] = 0xFFFF0000
			} else {
				// Read real stats
				if b.statsMap != nil {
					packets, ticks, insns, halted, err := readStatsMap(b.statsMap)
					if err == nil {
						msg.Packets = packets
						msg.Ticks = ticks
						msg.Insns = insns
						msg.Halted = halted
					}
				}

				if b.cpuMap != nil {
					cpu, err := readCpuMap(b.cpuMap)
					if err == nil {
						msg.PC = cpu.PC
						msg.Flags = cpu.Flags
						msg.Regs = cpu.Regs[:]
					} else {
						msg.Regs = make([]uint32, 16)
					}
				} else {
					msg.Regs = make([]uint32, 16)
				}
			}

			data, err := json.Marshal(msg)
			if err != nil {
				logf("ERROR", "Stats marshal error", "err", err)
				continue
			}

			b.broadcastText(data)
			tick++
		}
	}
}

// broadcastBinary sends a binary WebSocket message to all connected clients.
func (b *bridge) broadcastBinary(data []byte) {
	b.clientsMu.RLock()
	defer b.clientsMu.RUnlock()

	for c := range b.clients {
		c.mu.Lock()
		err := c.conn.WriteMessage(websocket.BinaryMessage, data)
		c.mu.Unlock()
		if err != nil {
			// Client will be cleaned up by readLoop
			logf("WARN", "Write error (client will be removed)", "err", err)
			atomic.AddInt64(&b.errorCount, 1)
		}
	}
}

// broadcastText sends a text WebSocket message to all connected clients.
func (b *bridge) broadcastText(data []byte) {
	b.clientsMu.RLock()
	defer b.clientsMu.RUnlock()

	for c := range b.clients {
		c.mu.Lock()
		err := c.conn.WriteMessage(websocket.TextMessage, data)
		c.mu.Unlock()
		if err != nil {
			logf("WARN", "Write error (client will be removed)", "err", err)
			atomic.AddInt64(&b.errorCount, 1)
		}
	}
}

// generateDryRunScreen produces a synthetic screen for testing without BPF maps.
// Creates an animated plasma/gradient pattern using the VGA palette.
func generateDryRunScreen(frame uint64) []byte {
	pixels := make([]byte, screenSize)
	t := float64(frame) * 0.05

	for y := 0; y < screenHeight; y++ {
		for x := 0; x < screenWidth; x++ {
			fx := float64(x) / float64(screenWidth)
			fy := float64(y) / float64(screenHeight)

			// Plasma effect using sin waves
			v1 := math.Sin(fx*10.0 + t)
			v2 := math.Sin(fy*8.0 + t*0.7)
			v3 := math.Sin((fx+fy)*6.0 + t*1.3)
			v4 := math.Sin(math.Sqrt(fx*fx+fy*fy)*12.0 + t*0.5)

			// Map to palette index (0-255)
			v := (v1 + v2 + v3 + v4 + 4.0) / 8.0 // normalize to 0..1
			idx := uint8(v * 255.0)

			// Add some Doom-ish flavor: fire effect at the bottom
			if y > 160 {
				fireIntensity := float64(y-160) / 40.0
				idx = uint8(float64(idx)*(1.0-fireIntensity) + fireIntensity*float64(32+rand.Intn(16)))
			}

			pixels[y*screenWidth+x] = idx
		}
	}
	return pixels
}
