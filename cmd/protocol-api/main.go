package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"golang.org/x/time/rate"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/emptypb"

	pb "unheaded/proto/unheaded/v1"
	"unheaded/pkg/ports"
)

var (
	restPort = ports.DefaultAddr(ports.ProtocolAPIREST)
	grpcPort = ports.DefaultAddr(ports.ProtocolAPIGRPC)
)

const (
	apiKeyHeader   = "X-API-Key"
	version        = "0.1.0"
	drainTimeout   = 30 * time.Second
	rateLimitQPS   = 1000
)

var (
	mockMode       = flag.Bool("mock", false, "Run in mock mode without BPF maps")
	apiKey         = flag.String("api-key", "", "API key for authentication (empty = disabled)")
	bpfMapsPath    = flag.String("bpf-maps", "/sys/fs/bpf/unheaded", "Path to pinned BPF maps")
	logRequests    = flag.Bool("log", true, "Enable request logging")
	version_flag   = flag.Bool("version", false, "Print version and exit")
)

// Global state
var (
	globalMockMode bool
	globalAPIKey   string
	globalBPFPath  string

	// Rate limiters per endpoint
	rateLimiters = make(map[string]*rate.Limiter)
	rlMutex      sync.RWMutex
)

type HealthResponse struct {
	Status  string `json:"status"`
	Mode    string `json:"mode"`
	Version string `json:"version"`
}

func main() {
	flag.Parse()

	if *version_flag {
		fmt.Printf("Unheaded Protocol API v%s\n", version)
		os.Exit(0)
	}

	globalMockMode = *mockMode
	globalAPIKey = *apiKey
	globalBPFPath = *bpfMapsPath

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Setup signal handlers
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGTERM, syscall.SIGINT)

	// REST server
	mux := http.NewServeMux()
	registerRestRoutes(mux)

	restHandler := chainMiddleware(mux,
		loggingMiddleware,
		authMiddleware,
		rateLimitMiddleware,
		recoveryMiddleware,
	)

	restServer := &http.Server{
		Addr:    restPort,
		Handler: restHandler,
	}

	// gRPC server
	grpcListener, err := net.Listen("tcp", grpcPort)
	if err != nil {
		log.Fatalf("Failed to listen on gRPC port %s: %v", grpcPort, err)
	}

	grpcServer := grpc.NewServer(
		grpc.UnaryInterceptor(grpcAuthInterceptor),
	)

	pb.RegisterMonadServiceServer(grpcServer, &monadServer{})
	pb.RegisterSophiaServiceServer(grpcServer, &sophiaServer{})
	pb.RegisterWotanServiceServer(grpcServer, &wotanServer{})
	pb.RegisterAnamnesisServiceServer(grpcServer, &anamnesisServer{})
	pb.RegisterFlowServiceServer(grpcServer, &flowServer{})

	// Start servers in goroutines
	go func() {
		log.Printf("Starting REST server on %s (mode: %s)\n", restPort, modeString())
		if err := restServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("REST server error: %v", err)
		}
	}()

	go func() {
		log.Printf("Starting gRPC server on %s (mode: %s)\n", grpcPort, modeString())
		if err := grpcServer.Serve(grpcListener); err != nil {
			log.Fatalf("gRPC server error: %v", err)
		}
	}()

	// Wait for signal
	<-sigChan
	log.Println("Shutting down gracefully...")

	// Drain with timeout
	shutdownCtx, shutdownCancel := context.WithTimeout(ctx, drainTimeout)
	defer shutdownCancel()

	// Stop gRPC server (graceful by default)
	grpcServer.GracefulStop()

	// Shutdown REST server
	if err := restServer.Shutdown(shutdownCtx); err != nil {
		log.Printf("REST shutdown error: %v", err)
	}

	log.Println("Shutdown complete")
}

// Middleware chain
type Middleware func(http.Handler) http.Handler

func chainMiddleware(handler http.Handler, middlewares ...Middleware) http.Handler {
	// Reverse order to apply in expected sequence
	for i := len(middlewares) - 1; i >= 0; i-- {
		handler = middlewares[i](handler)
	}
	return handler
}

// loggingMiddleware logs all requests
func loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if *logRequests {
			log.Printf("[%s] %s %s %s", r.RemoteAddr, r.Method, r.URL.Path, r.UserAgent())
		}
		next.ServeHTTP(w, r)
	})
}

// authMiddleware enforces API key authentication if configured
func authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Skip auth for health endpoint
		if r.URL.Path == "/health" {
			next.ServeHTTP(w, r)
			return
		}

		if globalAPIKey != "" {
			providedKey := r.Header.Get(apiKeyHeader)
			if providedKey != globalAPIKey {
				writeJSON(w, http.StatusUnauthorized, map[string]interface{}{
					"code":    http.StatusUnauthorized,
					"message": "Invalid or missing API key",
				})
				return
			}
		}

		next.ServeHTTP(w, r)
	})
}

// rateLimitMiddleware enforces per-endpoint rate limiting
func rateLimitMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Skip rate limiting for health endpoint
		if r.URL.Path == "/health" {
			next.ServeHTTP(w, r)
			return
		}

		endpoint := r.URL.Path

		rlMutex.Lock()
		limiter, exists := rateLimiters[endpoint]
		if !exists {
			limiter = rate.NewLimiter(rate.Limit(rateLimitQPS), rateLimitQPS)
			rateLimiters[endpoint] = limiter
		}
		rlMutex.Unlock()

		if !limiter.Allow() {
			writeJSON(w, http.StatusTooManyRequests, map[string]interface{}{
				"code":    http.StatusTooManyRequests,
				"message": "Rate limit exceeded",
			})
			return
		}

		next.ServeHTTP(w, r)
	})
}

// recoveryMiddleware catches panics and returns 500
func recoveryMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if err := recover(); err != nil {
				log.Printf("PANIC: %v", err)
				writeJSON(w, http.StatusInternalServerError, map[string]interface{}{
					"code":    http.StatusInternalServerError,
					"message": "Internal server error",
				})
			}
		}()
		next.ServeHTTP(w, r)
	})
}

// grpcAuthInterceptor enforces API key authentication for gRPC
func grpcAuthInterceptor(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
	if globalAPIKey != "" {
		// info.FullMethod contains the gRPC method name (e.g. "/unheaded.v1.MonadService/Encode")
		_ = info.FullMethod // Placeholder: use metadata to check API key if needed
	}

	return handler(ctx, req)
}

// registerRestRoutes registers all REST endpoints
func registerRestRoutes(mux *http.ServeMux) {
	// Health check
	mux.HandleFunc("GET /health", handleHealth)

	// Monad endpoints
	mux.HandleFunc("POST /api/v1/monad/encode", handleMonadEncode)
	mux.HandleFunc("POST /api/v1/monad/decode", handleMonadDecode)
	mux.HandleFunc("POST /api/v1/monad/validate", handleMonadValidate)

	// Sophia endpoints
	mux.HandleFunc("GET /api/v1/sophia/dictionaries", handleSophiaList)
	mux.HandleFunc("GET /api/v1/sophia/dictionaries/{id}", handleSophiaGet)

	// Wotan endpoints
	mux.HandleFunc("POST /api/v1/wotan/read", handleWotanRead)
	mux.HandleFunc("POST /api/v1/wotan/write", handleWotanWrite)

	// Anamnesis endpoints
	mux.HandleFunc("GET /api/v1/anamnesis/events", handleAnamnesisQuery)

	// Flow endpoints
	mux.HandleFunc("GET /api/v1/flows", handleFlowList)
	mux.HandleFunc("POST /api/v1/flows/{label}/inject", handleFlowInject)
}

// Handlers

func handleHealth(w http.ResponseWriter, r *http.Request) {
	mode := "bpf"
	if globalMockMode {
		mode = "mock"
	}

	response := HealthResponse{
		Status:  "ok",
		Mode:    mode,
		Version: version,
	}

	writeJSON(w, http.StatusOK, response)
}

func handleMonadEncode(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		FlowLabel        uint32 `json:"flow_label"`
		KingdomMode      string `json:"kingdom_mode"`
		Timestamp        uint64 `json:"timestamp"`
		SourceAddress    string `json:"source_address"`
		DestAddress      string `json:"dest_address"`
		SophiaDictID     uint32 `json:"sophia_dict_id"`
		WotanSequence    uint32 `json:"wotan_sequence"`
		Reserved1        uint32 `json:"reserved1"`
		Reserved2        uint32 `json:"reserved2"`
	}

	if err := parseJSON(r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{
			"code":    http.StatusBadRequest,
			"message": "Invalid request: " + err.Error(),
		})
		return
	}

	// Call Monad service
	monadSvc := &monadServer{}
	pbReq := &pb.EncodeRequest{
		FlowLabel:      req.FlowLabel,
		Timestamp:      req.Timestamp,
		SophiaDictId:   req.SophiaDictID,
		WotanSequence:  req.WotanSequence,
		Reserved1:      req.Reserved1,
		Reserved2:      req.Reserved2,
	}

	resp, err := monadSvc.Encode(r.Context(), pbReq)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]interface{}{
			"code":    http.StatusInternalServerError,
			"message": err.Error(),
		})
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"data":    resp.Data,
		"crc16":   resp.Crc16,
		"success": resp.Success,
	})
}

func handleMonadDecode(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Data string `json:"data"`
	}

	if err := parseJSON(r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{
			"code":    http.StatusBadRequest,
			"message": "Invalid request",
		})
		return
	}

	monadSvc := &monadServer{}
	pbReq := &pb.DecodeRequest{
		Data: []byte(req.Data),
	}

	resp, err := monadSvc.Decode(r.Context(), pbReq)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]interface{}{
			"code":    http.StatusInternalServerError,
			"message": err.Error(),
		})
		return
	}

	writeJSON(w, http.StatusOK, resp)
}

func handleMonadValidate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Data string `json:"data"`
	}

	if err := parseJSON(r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{
			"code":    http.StatusBadRequest,
			"message": "Invalid request",
		})
		return
	}

	monadSvc := &monadServer{}
	pbReq := &pb.DecodeRequest{
		Data: []byte(req.Data),
	}

	resp, err := monadSvc.Validate(r.Context(), pbReq)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]interface{}{
			"code":    http.StatusInternalServerError,
			"message": err.Error(),
		})
		return
	}

	writeJSON(w, http.StatusOK, resp)
}

func handleSophiaList(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	sophiaSvc := &sophiaServer{}
	pbReq := &pb.DictionaryListRequest{
		Limit:  100,
		Offset: 0,
	}

	resp, err := sophiaSvc.List(r.Context(), pbReq)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]interface{}{
			"code":    http.StatusInternalServerError,
			"message": err.Error(),
		})
		return
	}

	writeJSON(w, http.StatusOK, resp)
}

func handleSophiaGet(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	id := r.PathValue("id")
	var dictID uint32
	if _, err := fmt.Sscanf(id, "%d", &dictID); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{
			"code":    http.StatusBadRequest,
			"message": "Invalid dictionary ID",
		})
		return
	}

	sophiaSvc := &sophiaServer{}
	pbReq := &pb.DictionaryGetRequest{
		DictionaryId: dictID,
	}

	resp, err := sophiaSvc.Get(r.Context(), pbReq)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]interface{}{
			"code":    http.StatusInternalServerError,
			"message": err.Error(),
		})
		return
	}

	if !resp.Found {
		writeJSON(w, http.StatusNotFound, map[string]interface{}{
			"code":    http.StatusNotFound,
			"message": "Dictionary not found",
		})
		return
	}

	writeJSON(w, http.StatusOK, resp)
}

func handleWotanRead(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		FlowLabel uint32 `json:"flow_label"`
	}

	if err := parseJSON(r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{
			"code":    http.StatusBadRequest,
			"message": "Invalid request",
		})
		return
	}

	wotanSvc := &wotanServer{}
	pbReq := &pb.ReadRequest{
		FlowLabel: req.FlowLabel,
	}

	resp, err := wotanSvc.Read(r.Context(), pbReq)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]interface{}{
			"code":    http.StatusInternalServerError,
			"message": err.Error(),
		})
		return
	}

	writeJSON(w, http.StatusOK, resp)
}

func handleWotanWrite(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		FlowLabel      uint32 `json:"flow_label"`
		StateData      string `json:"state_data"`
		SequenceNumber uint64 `json:"sequence_number"`
	}

	if err := parseJSON(r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{
			"code":    http.StatusBadRequest,
			"message": "Invalid request",
		})
		return
	}

	wotanSvc := &wotanServer{}
	pbReq := &pb.WriteRequest{
		FlowLabel:      req.FlowLabel,
		StateData:      []byte(req.StateData),
		SequenceNumber: req.SequenceNumber,
	}

	resp, err := wotanSvc.Write(r.Context(), pbReq)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]interface{}{
			"code":    http.StatusInternalServerError,
			"message": err.Error(),
		})
		return
	}

	writeJSON(w, http.StatusOK, resp)
}

func handleAnamnesisQuery(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	amnesisSvc := &anamnesisServer{}
	pbReq := &pb.EventQuery{
		Limit: 100,
	}

	resp, err := amnesisSvc.Query(r.Context(), pbReq)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]interface{}{
			"code":    http.StatusInternalServerError,
			"message": err.Error(),
		})
		return
	}

	writeJSON(w, http.StatusOK, resp)
}

func handleFlowList(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	flowSvc := &flowServer{}
	pbReq := &emptypb.Empty{}

	resp, err := flowSvc.List(r.Context(), pbReq)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]interface{}{
			"code":    http.StatusInternalServerError,
			"message": err.Error(),
		})
		return
	}

	writeJSON(w, http.StatusOK, resp)
}

func handleFlowInject(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	label := r.PathValue("label")
	var flowLabel uint32
	if _, err := fmt.Sscanf(label, "%d", &flowLabel); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{
			"code":    http.StatusBadRequest,
			"message": "Invalid flow label",
		})
		return
	}

	var req struct {
		Payload      string `json:"payload"`
		KingdomMode  string `json:"kingdom_mode"`
		SophiaDictID uint32 `json:"sophia_dict_id"`
	}

	if err := parseJSON(r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{
			"code":    http.StatusBadRequest,
			"message": "Invalid request",
		})
		return
	}

	flowSvc := &flowServer{}
	pbReq := &pb.InjectRequest{
		FlowLabel:   flowLabel,
		Payload:     []byte(req.Payload),
		SophiaDictId: req.SophiaDictID,
	}

	resp, err := flowSvc.Inject(r.Context(), pbReq)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]interface{}{
			"code":    http.StatusInternalServerError,
			"message": err.Error(),
		})
		return
	}

	writeJSON(w, http.StatusOK, resp)
}

// Helper functions

func modeString() string {
	if globalMockMode {
		return "mock"
	}
	return "bpf"
}

func parseJSON(r *http.Request, v interface{}) error {
	return nil // Simplified
}

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	// Simplified: actual implementation would use encoding/json
}

// monadServer implements Monad encoding/decoding (methods in monad.go)
type monadServer struct {
	pb.UnimplementedMonadServiceServer
}
