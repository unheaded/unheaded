// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

// Package gauntlets provides CLI and API interface capabilities.
// Gauntlets the CLI & API is the Kingdom's grip - unified command interface.
// Implements The Gauntlets Law: every CLI command has a corresponding API endpoint.
package gauntlets

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"unheaded/pkg/logger"
	wotanClient "unheaded/pkg/wotan-client"
)

// Command represents a CLI/API command.
type Command struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Group       string         `json:"group"`
	Args        []Argument     `json:"args,omitempty"`
	Flags       []Flag         `json:"flags,omitempty"`
	APIPath     string         `json:"api_path"`
	APIMethod   string         `json:"api_method"`
	Handler     CommandHandler `json:"-"`
}

// CommandHandler processes a command.
type CommandHandler func(ctx context.Context, args map[string]interface{}) (*CommandResult, error)

// Argument represents a positional argument.
type Argument struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Required    bool   `json:"required"`
	Type        string `json:"type"` // string, int, bool
}

// Flag represents a command flag.
type Flag struct {
	Name        string      `json:"name"`
	Short       string      `json:"short,omitempty"`
	Description string      `json:"description"`
	Type        string      `json:"type"`
	Default     interface{} `json:"default,omitempty"`
	Required    bool        `json:"required"`
}

// CommandResult represents command output.
type CommandResult struct {
	Success  bool        `json:"success"`
	Data     interface{} `json:"data,omitempty"`
	Error    string      `json:"error,omitempty"`
	Duration int64       `json:"duration_ms"`
}

// APIRoute represents an API endpoint.
type APIRoute struct {
	Path        string           `json:"path"`
	Method      string           `json:"method"`
	Description string           `json:"description"`
	CommandName string           `json:"command_name"`
	Handler     http.HandlerFunc `json:"-"`
}

// Service is the main Gauntlets CLI/API service.
type Service struct {
	log    *logger.Logger
	wotan  *wotanClient.Client
	config *Config

	mu       sync.RWMutex
	commands map[string]*Command
	routes   map[string]*APIRoute
	groups   map[string][]string // group name -> command names

	// Degraded mode counter — messages dropped because Wotan is nil
	wotanDrops int64
}

// WotanDrops returns the total number of messages dropped because Wotan was nil.
func (s *Service) WotanDrops() int64 {
	return atomic.LoadInt64(&s.wotanDrops)
}

// Config holds Gauntlets service configuration.
type Config struct {
	APIPrefix  string `json:"api_prefix"`
	Version    string `json:"version"`
	WotanTopic string `json:"wotan_topic"`
}

// DefaultConfig returns sensible defaults.
func DefaultConfig() *Config {
	return &Config{
		APIPrefix:  "/api/v1",
		Version:    "1.0.0",
		WotanTopic: "gauntlets.commands",
	}
}

// NewService creates a new Gauntlets CLI/API service.
func NewService(log *logger.Logger, wotan *wotanClient.Client, cfg *Config) *Service {
	if cfg == nil {
		cfg = DefaultConfig()
	}

	return &Service{
		log:      log,
		wotan:    wotan,
		config:   cfg,
		commands: make(map[string]*Command),
		routes:   make(map[string]*APIRoute),
		groups:   make(map[string][]string),
	}
}

// Start initializes the Gauntlets service.
func (s *Service) Start(ctx context.Context) error {
	s.log.Info().Msg("Gauntlets equipped - CLI and API unified")
	s.registerBuiltinCommands()
	return nil
}

// Stop gracefully shuts down the service.
func (s *Service) Stop() error {
	s.log.Info().Msg("Gauntlets removed - CLI and API suspended")
	return nil
}

// RegisterCommand registers a new command (enforces The Gauntlets Law).
func (s *Service) RegisterCommand(cmd *Command) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if cmd.Name == "" {
		return fmt.Errorf("command name is required")
	}

	// The Gauntlets Law: every CLI command MUST have a corresponding API endpoint
	if cmd.APIPath == "" {
		cmd.APIPath = fmt.Sprintf("%s/%s", s.config.APIPrefix, cmd.Name)
	}
	if cmd.APIMethod == "" {
		cmd.APIMethod = http.MethodPost
	}

	s.commands[cmd.Name] = cmd

	// Register API route
	routeKey := fmt.Sprintf("%s:%s", cmd.APIMethod, cmd.APIPath)
	s.routes[routeKey] = &APIRoute{
		Path:        cmd.APIPath,
		Method:      cmd.APIMethod,
		Description: cmd.Description,
		CommandName: cmd.Name,
	}

	// Add to group
	if cmd.Group != "" {
		s.groups[cmd.Group] = append(s.groups[cmd.Group], cmd.Name)
	}

	s.log.Info().
		Str("name", cmd.Name).
		Str("api_path", cmd.APIPath).
		Str("method", cmd.APIMethod).
		Msg("Command registered (Gauntlets Law enforced)")

	return nil
}

// Execute runs a command by name.
func (s *Service) Execute(ctx context.Context, cmdName string, args map[string]interface{}) (*CommandResult, error) {
	s.mu.RLock()
	cmd, ok := s.commands[cmdName]
	s.mu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("command not found: %s", cmdName)
	}

	// Validate required arguments
	for _, arg := range cmd.Args {
		if arg.Required {
			if _, ok := args[arg.Name]; !ok {
				return &CommandResult{
					Success: false,
					Error:   fmt.Sprintf("missing required argument: %s", arg.Name),
				}, nil
			}
		}
	}

	start := time.Now()
	result, err := cmd.Handler(ctx, args)
	duration := time.Since(start).Milliseconds()

	if err != nil {
		return &CommandResult{
			Success:  false,
			Error:    err.Error(),
			Duration: duration,
		}, nil
	}

	if result == nil {
		result = &CommandResult{Success: true}
	}
	result.Duration = duration

	// Publish command execution event
	s.publishEvent(ctx, "gauntlets.command.executed", map[string]interface{}{
		"command":     cmdName,
		"success":     result.Success,
		"duration_ms": duration,
	})

	return result, nil
}

// GetCommand returns a command by name.
func (s *Service) GetCommand(name string) (*Command, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	cmd, ok := s.commands[name]
	return cmd, ok
}

// ListCommands returns all commands.
func (s *Service) ListCommands() []*Command {
	s.mu.RLock()
	defer s.mu.RUnlock()

	cmds := make([]*Command, 0, len(s.commands))
	for _, cmd := range s.commands {
		cmds = append(cmds, cmd)
	}
	return cmds
}

// ListCommandsByGroup returns commands in a group.
func (s *Service) ListCommandsByGroup(group string) []*Command {
	s.mu.RLock()
	defer s.mu.RUnlock()

	names := s.groups[group]
	cmds := make([]*Command, 0, len(names))
	for _, name := range names {
		if cmd, ok := s.commands[name]; ok {
			cmds = append(cmds, cmd)
		}
	}
	return cmds
}

// GetAPIRoutes returns all API routes.
func (s *Service) GetAPIRoutes() []*APIRoute {
	s.mu.RLock()
	defer s.mu.RUnlock()

	routes := make([]*APIRoute, 0, len(s.routes))
	for _, route := range s.routes {
		routes = append(routes, route)
	}
	return routes
}

// HTTPHandler returns an HTTP handler for API routes.
func (s *Service) HTTPHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		routeKey := fmt.Sprintf("%s:%s", r.Method, r.URL.Path)

		s.mu.RLock()
		route, ok := s.routes[routeKey]
		s.mu.RUnlock()

		if !ok {
			http.Error(w, "Not found", http.StatusNotFound)
			return
		}

		// Parse request body as args
		args := make(map[string]interface{})
		if r.Body != nil && r.ContentLength > 0 {
			// SECURITY: Enforce body size limit (1MB) to prevent denial-of-service
			if err := json.NewDecoder(io.LimitReader(r.Body, 1*1024*1024)).Decode(&args); err != nil {
				http.Error(w, "Invalid request body", http.StatusBadRequest)
				return
			}
		}

		// Add query params
		for k, v := range r.URL.Query() {
			if len(v) == 1 {
				args[k] = v[0]
			} else {
				args[k] = v
			}
		}

		result, err := s.Execute(r.Context(), route.CommandName, args)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		if !result.Success {
			w.WriteHeader(http.StatusBadRequest)
		}
		json.NewEncoder(w).Encode(result)
	})
}

// GenerateOpenAPI generates OpenAPI spec for all routes.
func (s *Service) GenerateOpenAPI() map[string]interface{} {
	s.mu.RLock()
	defer s.mu.RUnlock()

	paths := make(map[string]interface{})
	for _, route := range s.routes {
		methodLower := route.Method
		if paths[route.Path] == nil {
			paths[route.Path] = make(map[string]interface{})
		}
		pathOps, _ := paths[route.Path].(map[string]interface{})
		pathOps[methodLower] = map[string]interface{}{
			"summary":     route.Description,
			"operationId": route.CommandName,
			"tags":        []string{s.getCommandGroup(route.CommandName)},
		}
	}

	return map[string]interface{}{
		"openapi": "3.0.0",
		"info": map[string]interface{}{
			"title":   "Unheaded Kingdom API",
			"version": s.config.Version,
		},
		"paths": paths,
	}
}

func (s *Service) getCommandGroup(cmdName string) string {
	for group, names := range s.groups {
		for _, name := range names {
			if name == cmdName {
				return group
			}
		}
	}
	return "default"
}

func (s *Service) registerBuiltinCommands() {
	// Health check command (API prefix path)
	_ = s.RegisterCommand(&Command{
		Name:        "health",
		Description: "Check service health",
		Group:       "system",
		APIPath:     fmt.Sprintf("%s/health", s.config.APIPrefix),
		APIMethod:   http.MethodGet,
		Handler: func(ctx context.Context, args map[string]interface{}) (*CommandResult, error) {
			return &CommandResult{
				Success: true,
				Data: map[string]interface{}{
					"status":    "healthy",
					"service":   "gauntlets",
					"timestamp": time.Now(),
				},
			}, nil
		},
	})

	// Standard /health endpoint (required by CLAUDE.md)
	_ = s.RegisterCommand(&Command{
		Name:        "health-standard",
		Description: "Standard health check endpoint",
		Group:       "system",
		APIPath:     "/health",
		APIMethod:   http.MethodGet,
		Handler: func(ctx context.Context, args map[string]interface{}) (*CommandResult, error) {
			return &CommandResult{
				Success: true,
				Data: map[string]interface{}{
					"status":  "healthy",
					"service": "gauntlets",
				},
			}, nil
		},
	})

	// Standard /ready endpoint (required by CLAUDE.md)
	_ = s.RegisterCommand(&Command{
		Name:        "ready",
		Description: "Readiness check endpoint",
		Group:       "system",
		APIPath:     "/ready",
		APIMethod:   http.MethodGet,
		Handler: func(ctx context.Context, args map[string]interface{}) (*CommandResult, error) {
			return &CommandResult{
				Success: true,
				Data: map[string]interface{}{
					"ready":   true,
					"service": "gauntlets",
				},
			}, nil
		},
	})

	// Standard /metrics endpoint (required by CLAUDE.md)
	_ = s.RegisterCommand(&Command{
		Name:        "metrics",
		Description: "Prometheus-style metrics endpoint",
		Group:       "system",
		APIPath:     "/metrics",
		APIMethod:   http.MethodGet,
		Handler: func(ctx context.Context, args map[string]interface{}) (*CommandResult, error) {
			stats := s.Stats()
			return &CommandResult{
				Success: true,
				Data:    stats,
			}, nil
		},
	})

	// Version command
	_ = s.RegisterCommand(&Command{
		Name:        "version",
		Description: "Get API version",
		Group:       "system",
		APIPath:     fmt.Sprintf("%s/version", s.config.APIPrefix),
		APIMethod:   http.MethodGet,
		Handler: func(ctx context.Context, args map[string]interface{}) (*CommandResult, error) {
			return &CommandResult{
				Success: true,
				Data: map[string]interface{}{
					"version": s.config.Version,
				},
			}, nil
		},
	})

	// List commands
	_ = s.RegisterCommand(&Command{
		Name:        "commands",
		Description: "List all available commands",
		Group:       "system",
		APIPath:     fmt.Sprintf("%s/commands", s.config.APIPrefix),
		APIMethod:   http.MethodGet,
		Handler: func(ctx context.Context, args map[string]interface{}) (*CommandResult, error) {
			cmds := s.ListCommands()
			return &CommandResult{
				Success: true,
				Data:    cmds,
			}, nil
		},
	})
}

func (s *Service) publishEvent(ctx context.Context, eventType string, data map[string]interface{}) {
	if s.wotan == nil {
		atomic.AddInt64(&s.wotanDrops, 1)
		s.log.Warn().Str("event_type", eventType).Msg("wotan unavailable — event dropped (degraded mode)")
		return
	}

	event := map[string]interface{}{
		"event_type": eventType,
		"timestamp":  time.Now().Unix(),
		"data":       data,
	}

	payload, err := json.Marshal(event)
	if err != nil {
		return
	}

	_ = s.wotan.Publish(ctx, s.config.WotanTopic, payload)
}

// Stats returns service statistics.
func (s *Service) Stats() map[string]interface{} {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return map[string]interface{}{
		"total_commands": len(s.commands),
		"total_routes":   len(s.routes),
		"command_groups": len(s.groups),
		"api_version":    s.config.Version,
	}
}
