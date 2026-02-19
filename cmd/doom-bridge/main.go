// Package main implements doom-bridge: a BPF-to-WebSocket bridge for the
// Doom-over-IPv6 project. It reads screen framebuffer and CPU stats from
// pinned BPF maps and streams them to browser clients via WebSocket.
// Keyboard input from clients is written back to the BPF keyboard map.
//
// Tasks: D-011 (screen output) and D-012 (keyboard input).
//
// Usage:
//
//	doom-bridge [--port 6660] [--map-path /sys/fs/bpf/unheaded/doom-ring/maps] [--dry-run]
package main

import (
	"encoding/binary"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"math"
	"math/rand"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"github.com/gorilla/websocket"
)

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
			WriteBufferSize: screenSize + 64, // enough for a screen frame
			CheckOrigin: func(r *http.Request) bool {
				return true // Allow all origins for development
			},
		},
	}

	// Open BPF maps unless in dry-run mode
	if !b.dryRun {
		b.openMaps()
	} else {
		log.Println("[dry-run] BPF maps disabled, serving synthetic data")
	}

	// Setup HTTP routes
	mux := http.NewServeMux()
	mux.HandleFunc("/ws", b.handleWebSocket)
	mux.HandleFunc("/health", b.handleHealth)
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
		log.Println("Shutting down...")
		close(stop)
		server.Close()
	}()

	log.Printf("doom-bridge listening on :%d (dry-run=%v)", b.port, b.dryRun)
	log.Printf("  WebSocket: ws://localhost:%d/ws", b.port)
	log.Printf("  Viewer:    http://localhost:%d/", b.port)
	log.Printf("  Health:    http://localhost:%d/health", b.port)
	if !b.dryRun {
		log.Printf("  BPF maps:  %s", b.mapPath)
	}

	if err := server.ListenAndServe(); err != http.ErrServerClosed {
		log.Fatalf("HTTP server error: %v", err)
	}

	wg.Wait()
	b.closeMaps()
	log.Println("doom-bridge stopped")
}

// openMaps opens all pinned BPF maps. Non-fatal if individual maps are missing.
func (b *bridge) openMaps() {
	var err error

	b.screenMap, err = openBPFMap(filepath.Join(b.mapPath, "SCREEN_MAP"))
	if err != nil {
		log.Printf("WARNING: SCREEN_MAP not available: %v", err)
	}

	b.kbdMap, err = openBPFMap(filepath.Join(b.mapPath, "KBD_MAP"))
	if err != nil {
		log.Printf("WARNING: KBD_MAP not available: %v", err)
	}

	b.statsMap, err = openBPFMap(filepath.Join(b.mapPath, "STATS"))
	if err != nil {
		log.Printf("WARNING: STATS map not available: %v", err)
	}

	b.cpuMap, err = openBPFMap(filepath.Join(b.mapPath, "CPU_MAP"))
	if err != nil {
		log.Printf("WARNING: CPU_MAP not available: %v", err)
	}

	// Probe batch support by attempting a small batch read
	if b.screenMap != nil {
		_, _, _, probeErr := b.screenMap.LookupBatch(1, 4, 1)
		if probeErr == nil {
			b.batchSupported = true
			log.Println("BPF batch lookup supported (fast screen reads)")
		} else {
			log.Printf("BPF batch lookup not available, using individual reads: %v", probeErr)
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

// handleWebSocket upgrades HTTP to WebSocket and manages the client lifecycle.
func (b *bridge) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := b.upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("WebSocket upgrade error: %v", err)
		return
	}

	c := &client{conn: conn}

	b.clientsMu.Lock()
	b.clients[c] = struct{}{}
	numClients := len(b.clients)
	b.clientsMu.Unlock()

	log.Printf("Client connected (total: %d)", numClients)

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
		log.Printf("Client disconnected (total: %d)", numClients)
	}()

	for {
		msgType, data, err := c.conn.ReadMessage()
		if err != nil {
			if !websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway) {
				log.Printf("WebSocket read error: %v", err)
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
			log.Printf("Malformed keyboard message: %d bytes", len(data))
			continue
		}

		scancode := binary.LittleEndian.Uint16(data[1:3])
		pressed := data[3] != 0

		if b.kbdMap != nil {
			if err := writeKbdMap(b.kbdMap, scancode, pressed); err != nil {
				log.Printf("KBD_MAP write error: %v", err)
			}
		}
	}
}

// screenLoop polls the SCREEN_MAP at ~30fps and broadcasts frames to all clients.
func (b *bridge) screenLoop(stop chan struct{}, wg *sync.WaitGroup) {
	defer wg.Done()

	ticker := time.NewTicker(time.Second / 30) // ~33ms per frame
	defer ticker.Stop()

	frameCount := uint64(0)

	for {
		select {
		case <-stop:
			return
		case <-ticker.C:
			var pixels []byte
			var err error

			if b.dryRun || b.screenMap == nil {
				pixels = generateDryRunScreen(frameCount)
			} else if b.batchSupported {
				pixels, err = readScreenBatch(b.screenMap)
				if err != nil {
					log.Printf("Screen batch read error (falling back): %v", err)
					pixels, err = readScreenIndividual(b.screenMap)
					if err != nil {
						log.Printf("Screen individual read error: %v", err)
						continue
					}
				}
			} else {
				pixels, err = readScreenIndividual(b.screenMap)
				if err != nil {
					log.Printf("Screen read error: %v", err)
					continue
				}
			}

			// Build frame: [tag byte] + [64000 pixel bytes]
			frame := make([]byte, 1+screenSize)
			frame[0] = tagScreen
			copy(frame[1:], pixels)

			b.broadcastBinary(frame)
			frameCount++
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
				log.Printf("Stats marshal error: %v", err)
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
			log.Printf("Write error (client will be removed): %v", err)
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
			log.Printf("Write error (client will be removed): %v", err)
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
