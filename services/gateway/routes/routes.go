// Package routes provides route definitions and management for the API Gateway.
package routes

import (
	"encoding/json"
	"net/http"
	"sort"
	"strings"
	"sync"

	"unheaded/pkg/logger"
	"unheaded/pkg/metrics"
	"unheaded/services/gateway/config"
	"unheaded/services/gateway/proxy"
)

// Route represents a configured route.
type Route struct {
	Config       *config.RouteConfig
	LoadBalancer proxy.LoadBalancer
	Circuit      *proxy.CircuitBreaker
	Proxy        *proxy.ReverseProxy
}

// Router manages routes and route matching.
type Router struct {
	routes   []*Route
	routeMap map[string]*Route
	log      *logger.Logger
	metrics  *metrics.Registry
	mu       sync.RWMutex
}

// NewRouter creates a new router.
func NewRouter(log *logger.Logger, metricsReg *metrics.Registry) *Router {
	return &Router{
		routes:   make([]*Route, 0),
		routeMap: make(map[string]*Route),
		log:      log,
		metrics:  metricsReg,
	}
}

// AddRoute adds a route to the router.
func (r *Router) AddRoute(cfg *config.RouteConfig, circuitCfg *config.CircuitConfig) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Create load balancer
	lb := proxy.NewLoadBalancer(cfg.LoadBalancer, cfg.Backends, r.log)

	// Create circuit breaker
	cb := proxy.NewCircuitBreaker(circuitCfg, r.log)

	// Create reverse proxy
	p := proxy.NewReverseProxy(cfg, r.log, r.metrics, lb, cb)

	route := &Route{
		Config:       cfg,
		LoadBalancer: lb,
		Circuit:      cb,
		Proxy:        p,
	}

	r.routes = append(r.routes, route)
	r.routeMap[cfg.Name] = route

	// Sort routes by path prefix length (longer prefixes first)
	sort.Slice(r.routes, func(i, j int) bool {
		return len(r.routes[i].Config.PathPrefix) > len(r.routes[j].Config.PathPrefix)
	})

	r.log.Info().
		Str("name", cfg.Name).
		Str("path_prefix", cfg.PathPrefix).
		Int("backends", len(cfg.Backends)).
		Msg("Route added")

	return nil
}

// Match finds a route that matches the request path.
func (r *Router) Match(path string) *Route {
	r.mu.RLock()
	defer r.mu.RUnlock()

	for _, route := range r.routes {
		if strings.HasPrefix(path, route.Config.PathPrefix) {
			return route
		}
	}

	return nil
}

// GetRoute returns a route by name.
func (r *Router) GetRoute(name string) *Route {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.routeMap[name]
}

// GetRoutes returns all routes.
func (r *Router) GetRoutes() []*Route {
	r.mu.RLock()
	defer r.mu.RUnlock()

	routes := make([]*Route, len(r.routes))
	copy(routes, r.routes)
	return routes
}

// RemoveRoute removes a route by name.
func (r *Router) RemoveRoute(name string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	route, ok := r.routeMap[name]
	if !ok {
		return false
	}

	// Close the proxy
	route.Proxy.Close()

	// Remove from slice
	for i, rt := range r.routes {
		if rt.Config.Name == name {
			r.routes = append(r.routes[:i], r.routes[i+1:]...)
			break
		}
	}

	delete(r.routeMap, name)

	r.log.Info().Str("name", name).Msg("Route removed")
	return true
}

// ServeHTTP implements http.Handler.
func (r *Router) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	route := r.Match(req.URL.Path)
	if route == nil {
		r.log.Debug().Str("path", req.URL.Path).Msg("No route matched")
		writeNotFound(w)
		return
	}

	route.Proxy.ServeHTTP(w, req)
}

// Close closes all routes.
func (r *Router) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	for _, route := range r.routes {
		route.Proxy.Close()
	}

	return nil
}

// RouteInfo provides information about a route.
type RouteInfo struct {
	Name           string   `json:"name"`
	PathPrefix     string   `json:"path_prefix"`
	Backends       []string `json:"backends"`
	HealthyBackends []string `json:"healthy_backends"`
	LoadBalancer   string   `json:"load_balancer"`
	CircuitState   string   `json:"circuit_state"`
}

// Info returns information about all routes.
func (r *Router) Info() []RouteInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()

	infos := make([]RouteInfo, 0, len(r.routes))
	for _, route := range r.routes {
		info := RouteInfo{
			Name:            route.Config.Name,
			PathPrefix:      route.Config.PathPrefix,
			Backends:        route.Config.Backends,
			HealthyBackends: route.LoadBalancer.GetHealthy(),
			LoadBalancer:    route.Config.LoadBalancer,
			CircuitState:    route.Circuit.State().String(),
		}
		infos = append(infos, info)
	}

	return infos
}

// writeNotFound writes a 404 response.
func writeNotFound(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusNotFound)
	json.NewEncoder(w).Encode(map[string]string{
		"error":   "not_found",
		"message": "No route matched the request path",
	})
}

// RoutesHandler provides an HTTP handler for route information.
type RoutesHandler struct {
	router *Router
}

// NewRoutesHandler creates a new routes handler.
func NewRoutesHandler(router *Router) *RoutesHandler {
	return &RoutesHandler{router: router}
}

// ServeHTTP implements http.Handler.
func (h *RoutesHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	infos := h.router.Info()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(infos)
}
