package architect

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/rs/zerolog/log"

	"unheaded/pkg/httputil"
)

// HTTPHandler wraps the architect service and provides HTTP endpoints
type HTTPHandler struct {
	service *ArchitectService
}

// NewHTTPHandler creates a new HTTP handler
func NewHTTPHandler(service *ArchitectService) *HTTPHandler {
	if service == nil {
		return &HTTPHandler{service: New()}
	}
	return &HTTPHandler{service: service}
}

// ============================================================================
// HELPER METHODS
// ============================================================================

func (h *HTTPHandler) writeError(w http.ResponseWriter, code int, errCode, message string) {
	httputil.WriteError(w, code, errCode, message)

	log.Warn().
		Int("status", code).
		Str("code", errCode).
		Str("message", message).
		Msg("error response")
}

func (h *HTTPHandler) writeSuccess(w http.ResponseWriter, code int, data interface{}) {
	httputil.WriteSuccess(w, code, data)
}

// ============================================================================
// HEALTH ENDPOINT
// ============================================================================

// Health handles GET /health
func (h *HTTPHandler) Health(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		h.writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED",
			fmt.Sprintf("method %s not allowed", r.Method))
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	healthy, err := h.service.Health(ctx)
	if err != nil {
		h.writeError(w, http.StatusInternalServerError, "HEALTH_CHECK_FAILED",
			fmt.Sprintf("health check failed: %v", err))
		return
	}

	if !healthy {
		h.writeError(w, http.StatusServiceUnavailable, "SERVICE_UNHEALTHY",
			"service is not healthy")
		return
	}

	httputil.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"service":   "architect",
		"status":    "healthy",
		"version":   "0.1.0",
		"timestamp": time.Now().UTC(),
	})
}

// ============================================================================
// READINESS ENDPOINT
// ============================================================================

// Ready handles GET /ready - returns 200 when service is ready to serve traffic
func (h *HTTPHandler) Ready(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		h.writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED",
			fmt.Sprintf("method %s not allowed", r.Method))
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	// Check if service is initialized
	if h.service == nil {
		h.writeError(w, http.StatusServiceUnavailable, "SERVICE_NOT_READY",
			"service is not initialized")
		return
	}

	// Check infrastructure state is initialized
	_, infraErr := h.service.GetInfrastructureState(ctx)
	if infraErr != nil {
		h.writeError(w, http.StatusServiceUnavailable, "SERVICE_NOT_READY",
			fmt.Sprintf("infrastructure state not initialized: %v", infraErr))
		return
	}

	// Check network state is initialized
	_, networkErr := h.service.GetNetworkTopology(ctx)
	if networkErr != nil {
		h.writeError(w, http.StatusServiceUnavailable, "SERVICE_NOT_READY",
			fmt.Sprintf("network state not initialized: %v", networkErr))
		return
	}

	h.writeSuccess(w, http.StatusOK, map[string]interface{}{
		"ready": true,
		"checks": map[string]bool{
			"service":        true,
			"infrastructure": true,
			"network":        true,
		},
	})
}

// ============================================================================
// INFRASTRUCTURE ENDPOINTS
// ============================================================================

// GetInfrastructure handles GET /infrastructure
func (h *HTTPHandler) GetInfrastructure(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		h.writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED",
			fmt.Sprintf("method %s not allowed", r.Method))
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	state, err := h.service.GetInfrastructureState(ctx)
	if err != nil {
		h.writeError(w, http.StatusInternalServerError, "INFRA_STATE_FAILED",
			fmt.Sprintf("failed to get infrastructure state: %v", err))
		return
	}

	h.writeSuccess(w, http.StatusOK, state)

	log.Info().
		Int("services", len(state.Services)).
		Int("decisions", len(state.Decisions)).
		Msg("infrastructure state retrieved")
}

// AddService handles POST /infrastructure/services
func (h *HTTPHandler) AddService(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		h.writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED",
			fmt.Sprintf("method %s not allowed", r.Method))
		return
	}

	var service Service
	if err := httputil.DecodeJSONBody(r, 0, &service); err != nil {
		httputil.HandleRequestError(w, err)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	if err := h.service.AddService(ctx, &service); err != nil {
		h.writeError(w, http.StatusBadRequest, "ADD_SERVICE_FAILED",
			fmt.Sprintf("failed to add service: %v", err))
		return
	}

	h.writeSuccess(w, http.StatusCreated, service)

	log.Info().
		Str("service_id", service.ServiceID).
		Str("name", service.Name).
		Msg("service added")
}

// ============================================================================
// NETWORK ENDPOINTS
// ============================================================================

// GetNetwork handles GET /network
func (h *HTTPHandler) GetNetwork(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		h.writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED",
			fmt.Sprintf("method %s not allowed", r.Method))
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	topology, err := h.service.GetNetworkTopology(ctx)
	if err != nil {
		h.writeError(w, http.StatusInternalServerError, "NETWORK_STATE_FAILED",
			fmt.Sprintf("failed to get network topology: %v", err))
		return
	}

	h.writeSuccess(w, http.StatusOK, topology)

	log.Info().
		Int("nodes", len(topology.Nodes)).
		Msg("network topology retrieved")
}

// AddNetworkNode handles POST /network/nodes
func (h *HTTPHandler) AddNetworkNode(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		h.writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED",
			fmt.Sprintf("method %s not allowed", r.Method))
		return
	}

	var node NetworkNode
	if err := httputil.DecodeJSONBody(r, 0, &node); err != nil {
		httputil.HandleRequestError(w, err)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	if err := h.service.AddNetworkNode(ctx, &node); err != nil {
		h.writeError(w, http.StatusBadRequest, "ADD_NODE_FAILED",
			fmt.Sprintf("failed to add node: %v", err))
		return
	}

	h.writeSuccess(w, http.StatusCreated, node)

	log.Info().
		Str("node_id", node.NodeID).
		Str("name", node.Name).
		Msg("network node added")
}

// ============================================================================
// DESIGN ENDPOINTS
// ============================================================================

// LogDesign handles POST /design
func (h *HTTPHandler) LogDesign(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		h.writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED",
			fmt.Sprintf("method %s not allowed", r.Method))
		return
	}

	var decision ArchitectureDecision
	if err := httputil.DecodeJSONBody(r, 0, &decision); err != nil {
		httputil.HandleRequestError(w, err)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	if err := h.service.LogDecision(ctx, &decision); err != nil {
		h.writeError(w, http.StatusBadRequest, "LOG_DECISION_FAILED",
			fmt.Sprintf("failed to log decision: %v", err))
		return
	}

	h.writeSuccess(w, http.StatusCreated, decision)

	log.Info().
		Str("title", decision.Title).
		Str("component", decision.Component).
		Msg("architecture decision logged")
}

// GetDesignDecisions handles GET /design
func (h *HTTPHandler) GetDesignDecisions(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		h.writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED",
			fmt.Sprintf("method %s not allowed", r.Method))
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	decisions, err := h.service.ListDecisions(ctx)
	if err != nil {
		h.writeError(w, http.StatusInternalServerError, "GET_DECISIONS_FAILED",
			fmt.Sprintf("failed to get decisions: %v", err))
		return
	}

	h.writeSuccess(w, http.StatusOK, decisions)

	log.Info().
		Int("decision_count", len(decisions)).
		Msg("design decisions retrieved")
}
