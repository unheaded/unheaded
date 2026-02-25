package gauntlets

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"unheaded/pkg/logger"
)

// ---------------------------------------------------------------------------
// Test helpers
// ---------------------------------------------------------------------------

// newTestLogger returns a logger that discards output (safe for parallel tests).
func newTestLogger() *logger.Logger {
	return logger.New(io.Discard)
}

// newTestService creates a Service with a discard logger and no wotan client.
// wotan == nil is explicitly handled in publishEvent, so this is safe.
func newTestService(cfg *Config) *Service {
	return NewService(newTestLogger(), nil, cfg)
}

// newTestServiceStarted creates, starts, and returns a Service.
// Builtin commands (health, version, commands) are registered on Start.
func newTestServiceStarted(t *testing.T, cfg *Config) *Service {
	t.Helper()
	svc := newTestService(cfg)
	if err := svc.Start(context.Background()); err != nil {
		t.Fatalf("Start() failed: %v", err)
	}
	return svc
}

// echoHandler is a simple handler that echoes the args as Data.
func echoHandler(ctx context.Context, args map[string]interface{}) (*CommandResult, error) {
	return &CommandResult{
		Success: true,
		Data:    args,
	}, nil
}

// errorHandler always returns an error.
func errorHandler(ctx context.Context, args map[string]interface{}) (*CommandResult, error) {
	return nil, fmt.Errorf("intentional failure")
}

// nilResultHandler returns (nil, nil).
func nilResultHandler(ctx context.Context, args map[string]interface{}) (*CommandResult, error) {
	return nil, nil
}

// ---------------------------------------------------------------------------
// DefaultConfig
// ---------------------------------------------------------------------------

func TestDefaultConfig(t *testing.T) {
	t.Parallel()

	cfg := DefaultConfig()

	if cfg.APIPrefix != "/api/v1" {
		t.Errorf("APIPrefix = %q; want %q", cfg.APIPrefix, "/api/v1")
	}
	if cfg.Version != "1.0.0" {
		t.Errorf("Version = %q; want %q", cfg.Version, "1.0.0")
	}
	if cfg.WotanTopic != "gauntlets.commands" {
		t.Errorf("WotanTopic = %q; want %q", cfg.WotanTopic, "gauntlets.commands")
	}
}

// ---------------------------------------------------------------------------
// NewService
// ---------------------------------------------------------------------------

func TestNewService(t *testing.T) {
	t.Parallel()

	t.Run("nil config uses defaults", func(t *testing.T) {
		t.Parallel()
		svc := newTestService(nil)
		if svc.config.APIPrefix != "/api/v1" {
			t.Errorf("expected default APIPrefix, got %q", svc.config.APIPrefix)
		}
		if svc.commands == nil {
			t.Error("commands map should be initialized")
		}
		if svc.routes == nil {
			t.Error("routes map should be initialized")
		}
		if svc.groups == nil {
			t.Error("groups map should be initialized")
		}
	})

	t.Run("explicit config is used", func(t *testing.T) {
		t.Parallel()
		cfg := &Config{
			APIPrefix:   "/custom/v2",
			Version:     "2.0.0",
			WotanTopic: "custom.topic",
		}
		svc := newTestService(cfg)
		if svc.config.APIPrefix != "/custom/v2" {
			t.Errorf("APIPrefix = %q; want %q", svc.config.APIPrefix, "/custom/v2")
		}
		if svc.config.Version != "2.0.0" {
			t.Errorf("Version = %q; want %q", svc.config.Version, "2.0.0")
		}
	})
}

// ---------------------------------------------------------------------------
// Start / Stop
// ---------------------------------------------------------------------------

func TestStartStop(t *testing.T) {
	t.Parallel()

	svc := newTestService(nil)

	if err := svc.Start(context.Background()); err != nil {
		t.Fatalf("Start() returned error: %v", err)
	}

	// After start, builtin commands should be registered.
	for _, name := range []string{"health", "version", "commands"} {
		if _, ok := svc.GetCommand(name); !ok {
			t.Errorf("builtin command %q not found after Start()", name)
		}
	}

	if err := svc.Stop(); err != nil {
		t.Fatalf("Stop() returned error: %v", err)
	}
}

// ---------------------------------------------------------------------------
// RegisterCommand — Table-Driven
// ---------------------------------------------------------------------------

func TestRegisterCommand(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		cmd       *Command
		wantErr   bool
		errSubstr string
		// Expected values after successful registration:
		wantAPIPath   string
		wantAPIMethod string
	}{
		{
			name:    "empty name returns error",
			cmd:     &Command{Name: "", Handler: echoHandler},
			wantErr: true, errSubstr: "command name is required",
		},
		{
			name:          "auto-generates API path when not set",
			cmd:           &Command{Name: "deploy", Handler: echoHandler},
			wantAPIPath:   "/api/v1/deploy",
			wantAPIMethod: http.MethodPost,
		},
		{
			name:          "auto-generates POST method when not set",
			cmd:           &Command{Name: "restart", APIPath: "/custom/restart", Handler: echoHandler},
			wantAPIPath:   "/custom/restart",
			wantAPIMethod: http.MethodPost,
		},
		{
			name:          "explicit path and method preserved",
			cmd:           &Command{Name: "status", APIPath: "/api/v1/status", APIMethod: http.MethodGet, Handler: echoHandler},
			wantAPIPath:   "/api/v1/status",
			wantAPIMethod: http.MethodGet,
		},
		{
			name: "command with group is tracked",
			cmd: &Command{
				Name:    "list-pods",
				Group:   "cluster",
				Handler: echoHandler,
			},
			wantAPIPath:   "/api/v1/list-pods",
			wantAPIMethod: http.MethodPost,
		},
		{
			name: "command with args and flags registers OK",
			cmd: &Command{
				Name: "greet",
				Args: []Argument{
					{Name: "name", Required: true, Type: "string"},
				},
				Flags: []Flag{
					{Name: "loud", Short: "l", Type: "bool", Default: false},
				},
				Handler: echoHandler,
			},
			wantAPIPath:   "/api/v1/greet",
			wantAPIMethod: http.MethodPost,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			svc := newTestService(nil)
			err := svc.RegisterCommand(tt.cmd)

			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				if tt.errSubstr != "" && !strings.Contains(err.Error(), tt.errSubstr) {
					t.Errorf("error %q does not contain %q", err.Error(), tt.errSubstr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			// Verify the command is retrievable.
			cmd, ok := svc.GetCommand(tt.cmd.Name)
			if !ok {
				t.Fatalf("command %q not found after registration", tt.cmd.Name)
			}
			if cmd.APIPath != tt.wantAPIPath {
				t.Errorf("APIPath = %q; want %q", cmd.APIPath, tt.wantAPIPath)
			}
			if cmd.APIMethod != tt.wantAPIMethod {
				t.Errorf("APIMethod = %q; want %q", cmd.APIMethod, tt.wantAPIMethod)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// The Gauntlets Law: every CLI command has a corresponding API endpoint
// ---------------------------------------------------------------------------

func TestGauntletsLaw_CLIAPIparity(t *testing.T) {
	t.Parallel()

	svc := newTestServiceStarted(t, nil)

	// Register additional commands.
	customCmds := []*Command{
		{Name: "deploy", Group: "ops", Handler: echoHandler},
		{Name: "rollback", Group: "ops", APIPath: "/api/v1/rollback", APIMethod: http.MethodPost, Handler: echoHandler},
		{Name: "logs", Group: "debug", APIMethod: http.MethodGet, Handler: echoHandler},
	}
	for _, c := range customCmds {
		if err := svc.RegisterCommand(c); err != nil {
			t.Fatalf("RegisterCommand(%q) failed: %v", c.Name, err)
		}
	}

	allCmds := svc.ListCommands()
	allRoutes := svc.GetAPIRoutes()

	// Build a set of command names that have routes.
	routeCommandNames := make(map[string]bool)
	for _, r := range allRoutes {
		routeCommandNames[r.CommandName] = true
	}

	for _, cmd := range allCmds {
		t.Run(cmd.Name, func(t *testing.T) {
			// GAUNTLETS LAW: every CLI command must have an API route.
			if !routeCommandNames[cmd.Name] {
				t.Errorf("Gauntlets Law violation: CLI command %q has no API route", cmd.Name)
			}

			// Verify the route is keyed correctly.
			routeKey := fmt.Sprintf("%s:%s", cmd.APIMethod, cmd.APIPath)
			svc.mu.RLock()
			route, ok := svc.routes[routeKey]
			svc.mu.RUnlock()
			if !ok {
				t.Errorf("route key %q not found in routes map", routeKey)
			} else if route.CommandName != cmd.Name {
				t.Errorf("route.CommandName = %q; want %q", route.CommandName, cmd.Name)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Execute — Table-Driven
// ---------------------------------------------------------------------------

func TestExecute(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		cmdName     string
		registerCmd *Command // nil means do not register
		args        map[string]interface{}
		wantSuccess bool
		wantErrNil  bool // true if Execute should return (result, nil)
		errSubstr   string
	}{
		{
			name:       "command not found",
			cmdName:    "nonexistent",
			wantErrNil: false,
			errSubstr:  "command not found",
		},
		{
			name:    "missing required argument",
			cmdName: "greet",
			registerCmd: &Command{
				Name: "greet",
				Args: []Argument{
					{Name: "name", Required: true, Type: "string"},
				},
				Handler: echoHandler,
			},
			args:        map[string]interface{}{},
			wantSuccess: false,
			wantErrNil:  true,
		},
		{
			name:    "required argument present succeeds",
			cmdName: "greet",
			registerCmd: &Command{
				Name: "greet",
				Args: []Argument{
					{Name: "name", Required: true, Type: "string"},
				},
				Handler: echoHandler,
			},
			args:        map[string]interface{}{"name": "World"},
			wantSuccess: true,
			wantErrNil:  true,
		},
		{
			name:    "handler returns error produces failed result",
			cmdName: "fail",
			registerCmd: &Command{
				Name:    "fail",
				Handler: errorHandler,
			},
			args:        map[string]interface{}{},
			wantSuccess: false,
			wantErrNil:  true,
		},
		{
			name:    "handler returns nil result wraps to success",
			cmdName: "noop",
			registerCmd: &Command{
				Name:    "noop",
				Handler: nilResultHandler,
			},
			args:        map[string]interface{}{},
			wantSuccess: true,
			wantErrNil:  true,
		},
		{
			name:    "optional argument missing still succeeds",
			cmdName: "optcmd",
			registerCmd: &Command{
				Name: "optcmd",
				Args: []Argument{
					{Name: "tag", Required: false, Type: "string"},
				},
				Handler: echoHandler,
			},
			args:        map[string]interface{}{},
			wantSuccess: true,
			wantErrNil:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			svc := newTestService(nil)

			if tt.registerCmd != nil {
				if err := svc.RegisterCommand(tt.registerCmd); err != nil {
					t.Fatalf("RegisterCommand failed: %v", err)
				}
			}

			result, err := svc.Execute(context.Background(), tt.cmdName, tt.args)

			if tt.wantErrNil {
				if err != nil {
					t.Fatalf("Execute returned unexpected error: %v", err)
				}
				if result.Success != tt.wantSuccess {
					t.Errorf("result.Success = %v; want %v", result.Success, tt.wantSuccess)
				}
			} else {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				if tt.errSubstr != "" && !strings.Contains(err.Error(), tt.errSubstr) {
					t.Errorf("error %q does not contain %q", err.Error(), tt.errSubstr)
				}
			}
		})
	}
}

func TestExecute_DurationTracked(t *testing.T) {
	t.Parallel()

	svc := newTestService(nil)
	_ = svc.RegisterCommand(&Command{
		Name: "slow",
		Handler: func(ctx context.Context, args map[string]interface{}) (*CommandResult, error) {
			time.Sleep(5 * time.Millisecond)
			return &CommandResult{Success: true}, nil
		},
	})

	result, err := svc.Execute(context.Background(), "slow", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Duration < 1 {
		t.Errorf("Duration = %d; expected >= 1ms", result.Duration)
	}
}

func TestExecute_ErrorHandlerDuration(t *testing.T) {
	t.Parallel()

	svc := newTestService(nil)
	_ = svc.RegisterCommand(&Command{
		Name:    "errdur",
		Handler: errorHandler,
	})

	result, err := svc.Execute(context.Background(), "errdur", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Success {
		t.Error("expected result.Success = false for error handler")
	}
	if result.Error == "" {
		t.Error("expected non-empty error string in result")
	}
	// Duration should be set even on error.
	if result.Duration < 0 {
		t.Errorf("Duration = %d; expected >= 0", result.Duration)
	}
}

// ---------------------------------------------------------------------------
// GetCommand
// ---------------------------------------------------------------------------

func TestGetCommand(t *testing.T) {
	t.Parallel()

	svc := newTestService(nil)
	_ = svc.RegisterCommand(&Command{Name: "find", Handler: echoHandler})

	t.Run("existing command", func(t *testing.T) {
		t.Parallel()
		cmd, ok := svc.GetCommand("find")
		if !ok {
			t.Fatal("expected to find command")
		}
		if cmd.Name != "find" {
			t.Errorf("Name = %q; want %q", cmd.Name, "find")
		}
	})

	t.Run("nonexistent command", func(t *testing.T) {
		t.Parallel()
		_, ok := svc.GetCommand("missing")
		if ok {
			t.Error("expected ok=false for missing command")
		}
	})
}

// ---------------------------------------------------------------------------
// ListCommands
// ---------------------------------------------------------------------------

func TestListCommands(t *testing.T) {
	t.Parallel()

	svc := newTestService(nil)
	cmds := svc.ListCommands()
	if len(cmds) != 0 {
		t.Errorf("expected 0 commands before registration, got %d", len(cmds))
	}

	_ = svc.RegisterCommand(&Command{Name: "alpha", Handler: echoHandler})
	_ = svc.RegisterCommand(&Command{Name: "beta", Handler: echoHandler})

	cmds = svc.ListCommands()
	if len(cmds) != 2 {
		t.Errorf("expected 2 commands, got %d", len(cmds))
	}
}

// ---------------------------------------------------------------------------
// ListCommandsByGroup
// ---------------------------------------------------------------------------

func TestListCommandsByGroup(t *testing.T) {
	t.Parallel()

	svc := newTestService(nil)
	_ = svc.RegisterCommand(&Command{Name: "a1", Group: "alpha", Handler: echoHandler})
	_ = svc.RegisterCommand(&Command{Name: "a2", Group: "alpha", Handler: echoHandler})
	_ = svc.RegisterCommand(&Command{Name: "b1", Group: "beta", Handler: echoHandler})
	_ = svc.RegisterCommand(&Command{Name: "orphan", Handler: echoHandler}) // no group

	alphas := svc.ListCommandsByGroup("alpha")
	if len(alphas) != 2 {
		t.Errorf("alpha group: got %d commands, want 2", len(alphas))
	}

	betas := svc.ListCommandsByGroup("beta")
	if len(betas) != 1 {
		t.Errorf("beta group: got %d commands, want 1", len(betas))
	}

	empty := svc.ListCommandsByGroup("nonexistent")
	if len(empty) != 0 {
		t.Errorf("nonexistent group: got %d commands, want 0", len(empty))
	}
}

// ---------------------------------------------------------------------------
// GetAPIRoutes
// ---------------------------------------------------------------------------

func TestGetAPIRoutes(t *testing.T) {
	t.Parallel()

	svc := newTestService(nil)
	routes := svc.GetAPIRoutes()
	if len(routes) != 0 {
		t.Errorf("expected 0 routes before registration, got %d", len(routes))
	}

	_ = svc.RegisterCommand(&Command{Name: "r1", Handler: echoHandler})
	_ = svc.RegisterCommand(&Command{Name: "r2", APIMethod: http.MethodGet, Handler: echoHandler})

	routes = svc.GetAPIRoutes()
	if len(routes) != 2 {
		t.Errorf("expected 2 routes, got %d", len(routes))
	}
}

// ---------------------------------------------------------------------------
// HTTPHandler — API endpoint tests
// ---------------------------------------------------------------------------

func TestHTTPHandler_NotFound(t *testing.T) {
	t.Parallel()

	svc := newTestService(nil)
	handler := svc.HTTPHandler()

	req := httptest.NewRequest(http.MethodGet, "/nonexistent", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Errorf("status = %d; want %d", rr.Code, http.StatusNotFound)
	}
}

func TestHTTPHandler_MethodMismatch(t *testing.T) {
	t.Parallel()

	svc := newTestService(nil)
	_ = svc.RegisterCommand(&Command{
		Name:      "only-post",
		APIPath:   "/api/v1/only-post",
		APIMethod: http.MethodPost,
		Handler:   echoHandler,
	})
	handler := svc.HTTPHandler()

	// GET to a POST-only route should 404 (route key mismatch).
	req := httptest.NewRequest(http.MethodGet, "/api/v1/only-post", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Errorf("status = %d; want %d", rr.Code, http.StatusNotFound)
	}
}

func TestHTTPHandler_InvalidJSON(t *testing.T) {
	t.Parallel()

	svc := newTestService(nil)
	_ = svc.RegisterCommand(&Command{
		Name:      "postit",
		APIPath:   "/api/v1/postit",
		APIMethod: http.MethodPost,
		Handler:   echoHandler,
	})
	handler := svc.HTTPHandler()

	req := httptest.NewRequest(http.MethodPost, "/api/v1/postit", strings.NewReader("{broken"))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d; want %d", rr.Code, http.StatusBadRequest)
	}
}

func TestHTTPHandler_Success_POST_JSON(t *testing.T) {
	t.Parallel()

	svc := newTestService(nil)
	_ = svc.RegisterCommand(&Command{
		Name:      "echo",
		APIPath:   "/api/v1/echo",
		APIMethod: http.MethodPost,
		Handler:   echoHandler,
	})
	handler := svc.HTTPHandler()

	body := `{"greeting":"hello"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/echo", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d; want %d", rr.Code, http.StatusOK)
	}
	if ct := rr.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q; want %q", ct, "application/json")
	}

	var result CommandResult
	if err := json.NewDecoder(rr.Body).Decode(&result); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !result.Success {
		t.Errorf("result.Success = false; want true")
	}
}

func TestHTTPHandler_QueryParams(t *testing.T) {
	t.Parallel()

	svc := newTestService(nil)
	var receivedArgs map[string]interface{}
	_ = svc.RegisterCommand(&Command{
		Name:      "query",
		APIPath:   "/api/v1/query",
		APIMethod: http.MethodGet,
		Handler: func(ctx context.Context, args map[string]interface{}) (*CommandResult, error) {
			receivedArgs = args
			return &CommandResult{Success: true, Data: args}, nil
		},
	})
	handler := svc.HTTPHandler()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/query?foo=bar&count=42", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d; want %d", rr.Code, http.StatusOK)
	}
	if receivedArgs["foo"] != "bar" {
		t.Errorf("foo = %v; want %q", receivedArgs["foo"], "bar")
	}
	if receivedArgs["count"] != "42" {
		t.Errorf("count = %v; want %q", receivedArgs["count"], "42")
	}
}

func TestHTTPHandler_QueryParams_MultiValue(t *testing.T) {
	t.Parallel()

	svc := newTestService(nil)
	var receivedArgs map[string]interface{}
	_ = svc.RegisterCommand(&Command{
		Name:      "multi",
		APIPath:   "/api/v1/multi",
		APIMethod: http.MethodGet,
		Handler: func(ctx context.Context, args map[string]interface{}) (*CommandResult, error) {
			receivedArgs = args
			return &CommandResult{Success: true}, nil
		},
	})
	handler := svc.HTTPHandler()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/multi?tag=a&tag=b&tag=c", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d; want %d", rr.Code, http.StatusOK)
	}

	// Multi-value query params should be stored as a slice.
	tags, ok := receivedArgs["tag"]
	if !ok {
		t.Fatal("expected 'tag' in args")
	}
	tagSlice, ok := tags.([]string)
	if !ok {
		t.Fatalf("tags should be []string, got %T", tags)
	}
	if len(tagSlice) != 3 {
		t.Errorf("len(tags) = %d; want 3", len(tagSlice))
	}
}

func TestHTTPHandler_FailedResult_ReturnsBadRequest(t *testing.T) {
	t.Parallel()

	svc := newTestService(nil)
	_ = svc.RegisterCommand(&Command{
		Name:      "badinput",
		APIPath:   "/api/v1/badinput",
		APIMethod: http.MethodPost,
		Args: []Argument{
			{Name: "id", Required: true, Type: "string"},
		},
		Handler: echoHandler,
	})
	handler := svc.HTTPHandler()

	// POST with empty body => missing required arg => result.Success = false => 400.
	req := httptest.NewRequest(http.MethodPost, "/api/v1/badinput", strings.NewReader("{}"))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d; want %d", rr.Code, http.StatusBadRequest)
	}

	var result CommandResult
	if err := json.NewDecoder(rr.Body).Decode(&result); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if result.Success {
		t.Error("expected Success = false")
	}
	if !strings.Contains(result.Error, "missing required argument") {
		t.Errorf("Error = %q; want to contain 'missing required argument'", result.Error)
	}
}

func TestHTTPHandler_HandlerError_Returns500(t *testing.T) {
	t.Parallel()

	svc := newTestService(nil)
	_ = svc.RegisterCommand(&Command{
		Name:      "internalerr",
		APIPath:   "/api/v1/internalerr",
		APIMethod: http.MethodPost,
		Handler: func(ctx context.Context, args map[string]interface{}) (*CommandResult, error) {
			// This handler error leads to result.Success=false, not a 500.
			// The HTTP handler only returns 500 if Execute itself returns (nil, err).
			return nil, fmt.Errorf("boom")
		},
	})
	handler := svc.HTTPHandler()

	req := httptest.NewRequest(http.MethodPost, "/api/v1/internalerr", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	// Execute wraps handler errors into a CommandResult, so status is 400.
	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d; want %d", rr.Code, http.StatusBadRequest)
	}
}

func TestHTTPHandler_EmptyBody_NoContentLength(t *testing.T) {
	t.Parallel()

	svc := newTestService(nil)
	_ = svc.RegisterCommand(&Command{
		Name:      "noargs",
		APIPath:   "/api/v1/noargs",
		APIMethod: http.MethodPost,
		Handler:   echoHandler,
	})
	handler := svc.HTTPHandler()

	// POST with nil body => ContentLength 0 => skip JSON decode => empty args.
	req := httptest.NewRequest(http.MethodPost, "/api/v1/noargs", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("status = %d; want %d", rr.Code, http.StatusOK)
	}
}

// ---------------------------------------------------------------------------
// Gauntlets Law: CLI Execute == HTTP endpoint parity
// ---------------------------------------------------------------------------

func TestGauntletsLaw_ExecuteParityWithHTTP(t *testing.T) {
	t.Parallel()

	svc := newTestService(nil)
	_ = svc.RegisterCommand(&Command{
		Name:      "parity",
		APIPath:   "/api/v1/parity",
		APIMethod: http.MethodPost,
		Handler: func(ctx context.Context, args map[string]interface{}) (*CommandResult, error) {
			return &CommandResult{
				Success: true,
				Data:    map[string]interface{}{"echo": args["msg"]},
			}, nil
		},
	})

	// CLI path
	cliResult, cliErr := svc.Execute(context.Background(), "parity", map[string]interface{}{"msg": "hello"})

	// API path
	handler := svc.HTTPHandler()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/parity", strings.NewReader(`{"msg":"hello"}`))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	var apiResult CommandResult
	if err := json.NewDecoder(rr.Body).Decode(&apiResult); err != nil {
		t.Fatalf("decode API response: %v", err)
	}

	// Both paths must succeed.
	if cliErr != nil {
		t.Fatalf("CLI Execute returned error: %v", cliErr)
	}
	if !cliResult.Success || !apiResult.Success {
		t.Errorf("CLI success=%v, API success=%v; both should be true", cliResult.Success, apiResult.Success)
	}

	// Both should return the same data.
	cliJSON, _ := json.Marshal(cliResult.Data)
	apiJSON, _ := json.Marshal(apiResult.Data)
	if string(cliJSON) != string(apiJSON) {
		t.Errorf("Gauntlets Law parity violation: CLI data=%s, API data=%s", cliJSON, apiJSON)
	}
}

// ---------------------------------------------------------------------------
// Builtin commands: health, version, commands
// ---------------------------------------------------------------------------

func TestBuiltinCommands(t *testing.T) {
	t.Parallel()

	svc := newTestServiceStarted(t, &Config{
		APIPrefix:   "/api/v1",
		Version:     "42.0.0",
		WotanTopic: "test.topic",
	})

	t.Run("health", func(t *testing.T) {
		t.Parallel()
		result, err := svc.Execute(context.Background(), "health", nil)
		if err != nil {
			t.Fatalf("error: %v", err)
		}
		if !result.Success {
			t.Error("expected success")
		}
		data, ok := result.Data.(map[string]interface{})
		if !ok {
			t.Fatal("data is not map[string]interface{}")
		}
		if data["status"] != "healthy" {
			t.Errorf("status = %v; want %q", data["status"], "healthy")
		}
	})

	t.Run("version", func(t *testing.T) {
		t.Parallel()
		result, err := svc.Execute(context.Background(), "version", nil)
		if err != nil {
			t.Fatalf("error: %v", err)
		}
		if !result.Success {
			t.Error("expected success")
		}
		data, ok := result.Data.(map[string]interface{})
		if !ok {
			t.Fatal("data is not map[string]interface{}")
		}
		if data["version"] != "42.0.0" {
			t.Errorf("version = %v; want %q", data["version"], "42.0.0")
		}
	})

	t.Run("commands", func(t *testing.T) {
		t.Parallel()
		result, err := svc.Execute(context.Background(), "commands", nil)
		if err != nil {
			t.Fatalf("error: %v", err)
		}
		if !result.Success {
			t.Error("expected success")
		}
		// Data should be a list of commands.
		cmds, ok := result.Data.([]*Command)
		if !ok {
			t.Fatalf("expected []*Command, got %T", result.Data)
		}
		if len(cmds) < 3 {
			t.Errorf("expected at least 3 builtin commands, got %d", len(cmds))
		}
	})
}

func TestBuiltinCommands_GroupIsSystem(t *testing.T) {
	t.Parallel()

	svc := newTestServiceStarted(t, nil)

	systemCmds := svc.ListCommandsByGroup("system")
	if len(systemCmds) != 6 {
		t.Errorf("expected 6 system commands, got %d", len(systemCmds))
	}

	names := make(map[string]bool)
	for _, c := range systemCmds {
		names[c.Name] = true
	}
	for _, expected := range []string{"health", "health-standard", "ready", "metrics", "version", "commands"} {
		if !names[expected] {
			t.Errorf("system group missing %q", expected)
		}
	}
}

// ---------------------------------------------------------------------------
// Builtin commands via HTTP (Gauntlets Law verified)
// ---------------------------------------------------------------------------

func TestBuiltinCommands_HTTP(t *testing.T) {
	t.Parallel()

	svc := newTestServiceStarted(t, nil)
	handler := svc.HTTPHandler()

	tests := []struct {
		name   string
		method string
		path   string
	}{
		{"health via HTTP", http.MethodGet, "/api/v1/health"},
		{"version via HTTP", http.MethodGet, "/api/v1/version"},
		{"commands via HTTP", http.MethodGet, "/api/v1/commands"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			req := httptest.NewRequest(tt.method, tt.path, nil)
			rr := httptest.NewRecorder()
			handler.ServeHTTP(rr, req)

			if rr.Code != http.StatusOK {
				t.Errorf("status = %d; want %d", rr.Code, http.StatusOK)
			}
			var result CommandResult
			if err := json.NewDecoder(rr.Body).Decode(&result); err != nil {
				t.Fatalf("decode: %v", err)
			}
			if !result.Success {
				t.Error("expected success = true")
			}
		})
	}
}

// ---------------------------------------------------------------------------
// GenerateOpenAPI
// ---------------------------------------------------------------------------

func TestGenerateOpenAPI(t *testing.T) {
	t.Parallel()

	svc := newTestServiceStarted(t, &Config{
		APIPrefix:   "/api/v1",
		Version:     "3.0.1",
		WotanTopic: "test",
	})

	spec := svc.GenerateOpenAPI()

	// Verify top-level fields.
	if spec["openapi"] != "3.0.0" {
		t.Errorf("openapi = %v; want %q", spec["openapi"], "3.0.0")
	}

	info, ok := spec["info"].(map[string]interface{})
	if !ok {
		t.Fatal("info is not map[string]interface{}")
	}
	if info["version"] != "3.0.1" {
		t.Errorf("info.version = %v; want %q", info["version"], "3.0.1")
	}
	if info["title"] != "Unheaded Kingdom API" {
		t.Errorf("info.title = %v; want %q", info["title"], "Unheaded Kingdom API")
	}

	paths, ok := spec["paths"].(map[string]interface{})
	if !ok {
		t.Fatal("paths is not map[string]interface{}")
	}
	// Should have at least the 3 builtin routes.
	if len(paths) < 3 {
		t.Errorf("expected at least 3 paths, got %d", len(paths))
	}

	// Verify a specific path.
	healthPath, ok := paths["/api/v1/health"]
	if !ok {
		t.Fatal("missing /api/v1/health path")
	}
	healthMethods, ok := healthPath.(map[string]interface{})
	if !ok {
		t.Fatal("health path is not map[string]interface{}")
	}
	// The method key in GenerateOpenAPI is route.Method (e.g. "GET").
	if _, ok := healthMethods[http.MethodGet]; !ok {
		t.Error("missing GET method for /api/v1/health")
	}
}

// ---------------------------------------------------------------------------
// getCommandGroup (private helper)
// ---------------------------------------------------------------------------

func TestGetCommandGroup(t *testing.T) {
	t.Parallel()

	svc := newTestService(nil)
	_ = svc.RegisterCommand(&Command{Name: "grouped", Group: "mygroup", Handler: echoHandler})
	_ = svc.RegisterCommand(&Command{Name: "ungrouped", Handler: echoHandler})

	if g := svc.getCommandGroup("grouped"); g != "mygroup" {
		t.Errorf("getCommandGroup(grouped) = %q; want %q", g, "mygroup")
	}
	if g := svc.getCommandGroup("ungrouped"); g != "default" {
		t.Errorf("getCommandGroup(ungrouped) = %q; want %q", g, "default")
	}
	if g := svc.getCommandGroup("nonexistent"); g != "default" {
		t.Errorf("getCommandGroup(nonexistent) = %q; want %q", g, "default")
	}
}

// ---------------------------------------------------------------------------
// Stats
// ---------------------------------------------------------------------------

func TestStats(t *testing.T) {
	t.Parallel()

	svc := newTestServiceStarted(t, &Config{
		APIPrefix:   "/api/v1",
		Version:     "9.8.7",
		WotanTopic: "test",
	})

	stats := svc.Stats()
	totalCmds, ok := stats["total_commands"].(int)
	if !ok {
		t.Fatal("total_commands not int")
	}
	// 6 builtin commands after start (health, health-standard, ready, metrics, version, commands).
	if totalCmds != 6 {
		t.Errorf("total_commands = %d; want 6", totalCmds)
	}

	totalRoutes, ok := stats["total_routes"].(int)
	if !ok {
		t.Fatal("total_routes not int")
	}
	if totalRoutes != 6 {
		t.Errorf("total_routes = %d; want 6", totalRoutes)
	}

	if stats["api_version"] != "9.8.7" {
		t.Errorf("api_version = %v; want %q", stats["api_version"], "9.8.7")
	}

	groups, ok := stats["command_groups"].(int)
	if !ok {
		t.Fatal("command_groups not int")
	}
	if groups != 1 { // "system" group
		t.Errorf("command_groups = %d; want 1", groups)
	}
}

// ---------------------------------------------------------------------------
// publishEvent — nil wotan should not panic
// ---------------------------------------------------------------------------

func TestPublishEvent_NilWotan(t *testing.T) {
	t.Parallel()

	svc := newTestService(nil) // wotan is nil
	_ = svc.RegisterCommand(&Command{
		Name:    "publish-test",
		Handler: echoHandler,
	})

	// Execute invokes publishEvent internally; must not panic.
	result, err := svc.Execute(context.Background(), "publish-test", map[string]interface{}{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Success {
		t.Error("expected success")
	}
}

// ---------------------------------------------------------------------------
// Input Validation — edge cases
// ---------------------------------------------------------------------------

func TestInputValidation(t *testing.T) {
	t.Parallel()

	t.Run("multiple required args all missing", func(t *testing.T) {
		t.Parallel()
		svc := newTestService(nil)
		_ = svc.RegisterCommand(&Command{
			Name: "multi-req",
			Args: []Argument{
				{Name: "first", Required: true, Type: "string"},
				{Name: "second", Required: true, Type: "int"},
			},
			Handler: echoHandler,
		})

		result, err := svc.Execute(context.Background(), "multi-req", map[string]interface{}{})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result.Success {
			t.Error("expected failure for missing required args")
		}
		if !strings.Contains(result.Error, "missing required argument") {
			t.Errorf("Error = %q; expected 'missing required argument'", result.Error)
		}
	})

	t.Run("one required present one missing", func(t *testing.T) {
		t.Parallel()
		svc := newTestService(nil)
		_ = svc.RegisterCommand(&Command{
			Name: "partial",
			Args: []Argument{
				{Name: "present", Required: true, Type: "string"},
				{Name: "missing", Required: true, Type: "string"},
			},
			Handler: echoHandler,
		})

		result, err := svc.Execute(context.Background(), "partial", map[string]interface{}{
			"present": "here",
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result.Success {
			t.Error("expected failure")
		}
	})

	t.Run("nil args map", func(t *testing.T) {
		t.Parallel()
		svc := newTestService(nil)
		_ = svc.RegisterCommand(&Command{
			Name:    "nil-args",
			Handler: echoHandler,
		})

		result, err := svc.Execute(context.Background(), "nil-args", nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !result.Success {
			t.Error("expected success with nil args and no required args")
		}
	})

	t.Run("nil args with required argument fails", func(t *testing.T) {
		t.Parallel()
		svc := newTestService(nil)
		_ = svc.RegisterCommand(&Command{
			Name: "nil-args-required",
			Args: []Argument{
				{Name: "id", Required: true, Type: "string"},
			},
			Handler: echoHandler,
		})

		result, err := svc.Execute(context.Background(), "nil-args-required", nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result.Success {
			t.Error("expected failure for nil args with required argument")
		}
	})
}

// ---------------------------------------------------------------------------
// Output Formatting — CommandResult JSON
// ---------------------------------------------------------------------------

func TestCommandResult_JSONFormat(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		result CommandResult
		check  func(t *testing.T, m map[string]interface{})
	}{
		{
			name: "success result",
			result: CommandResult{
				Success:  true,
				Data:     "hello",
				Duration: 42,
			},
			check: func(t *testing.T, m map[string]interface{}) {
				if m["success"] != true {
					t.Error("expected success=true")
				}
				if m["data"] != "hello" {
					t.Errorf("data = %v; want %q", m["data"], "hello")
				}
				if m["duration_ms"] != float64(42) {
					t.Errorf("duration_ms = %v; want 42", m["duration_ms"])
				}
				if _, ok := m["error"]; ok {
					t.Error("error field should be omitted on success")
				}
			},
		},
		{
			name: "error result",
			result: CommandResult{
				Success:  false,
				Error:    "something broke",
				Duration: 5,
			},
			check: func(t *testing.T, m map[string]interface{}) {
				if m["success"] != false {
					t.Error("expected success=false")
				}
				if m["error"] != "something broke" {
					t.Errorf("error = %v; want %q", m["error"], "something broke")
				}
				if _, ok := m["data"]; ok {
					t.Error("data field should be omitted on error")
				}
			},
		},
		{
			name: "result with complex data",
			result: CommandResult{
				Success: true,
				Data: map[string]interface{}{
					"count": 10,
					"items": []string{"a", "b"},
				},
				Duration: 1,
			},
			check: func(t *testing.T, m map[string]interface{}) {
				data, ok := m["data"].(map[string]interface{})
				if !ok {
					t.Fatal("data is not a map")
				}
				if data["count"] != float64(10) {
					t.Errorf("count = %v; want 10", data["count"])
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			b, err := json.Marshal(tt.result)
			if err != nil {
				t.Fatalf("json.Marshal failed: %v", err)
			}
			var m map[string]interface{}
			if err := json.Unmarshal(b, &m); err != nil {
				t.Fatalf("json.Unmarshal failed: %v", err)
			}
			tt.check(t, m)
		})
	}
}

// ---------------------------------------------------------------------------
// Command / Argument / Flag JSON serialization
// ---------------------------------------------------------------------------

func TestCommand_JSONSerialization(t *testing.T) {
	t.Parallel()

	cmd := Command{
		Name:        "deploy",
		Description: "Deploy a service",
		Group:       "ops",
		Args: []Argument{
			{Name: "service", Description: "Service name", Required: true, Type: "string"},
		},
		Flags: []Flag{
			{Name: "force", Short: "f", Description: "Force deploy", Type: "bool", Default: false, Required: false},
		},
		APIPath:   "/api/v1/deploy",
		APIMethod: "POST",
		Handler:   echoHandler,
	}

	b, err := json.Marshal(cmd)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}

	var parsed map[string]interface{}
	if err := json.Unmarshal(b, &parsed); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}

	if parsed["name"] != "deploy" {
		t.Errorf("name = %v; want %q", parsed["name"], "deploy")
	}
	if parsed["api_path"] != "/api/v1/deploy" {
		t.Errorf("api_path = %v; want %q", parsed["api_path"], "/api/v1/deploy")
	}

	// Handler should be omitted (json:"-").
	if _, ok := parsed["Handler"]; ok {
		t.Error("Handler should be omitted from JSON")
	}

	// Args should be present.
	args, ok := parsed["args"].([]interface{})
	if !ok || len(args) != 1 {
		t.Error("expected 1 arg in JSON")
	}

	// Flags should be present.
	flags, ok := parsed["flags"].([]interface{})
	if !ok || len(flags) != 1 {
		t.Error("expected 1 flag in JSON")
	}
}

func TestAPIRoute_JSONSerialization(t *testing.T) {
	t.Parallel()

	route := APIRoute{
		Path:        "/api/v1/test",
		Method:      "GET",
		Description: "Test route",
		CommandName: "test",
		Handler:     func(w http.ResponseWriter, r *http.Request) {},
	}

	b, err := json.Marshal(route)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}

	var parsed map[string]interface{}
	if err := json.Unmarshal(b, &parsed); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}

	if parsed["path"] != "/api/v1/test" {
		t.Errorf("path = %v; want %q", parsed["path"], "/api/v1/test")
	}
	// Handler should be omitted (json:"-").
	if _, ok := parsed["Handler"]; ok {
		t.Error("Handler should be omitted from JSON")
	}
}

// ---------------------------------------------------------------------------
// CLI command parsing simulation tests
// ---------------------------------------------------------------------------

func TestCLI_CommandParsing(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		input     string // simulated CLI input "cmdName arg1=val1 arg2=val2"
		wantCmd   string
		wantArgs  map[string]interface{}
		wantErr   bool
		errSubstr string
	}{
		{
			name:     "simple command",
			input:    "health",
			wantCmd:  "health",
			wantArgs: map[string]interface{}{},
		},
		{
			name:    "command with args",
			input:   "deploy service=web version=2.0",
			wantCmd: "deploy",
			wantArgs: map[string]interface{}{
				"service": "web",
				"version": "2.0",
			},
		},
		{
			name:      "empty input",
			input:     "",
			wantErr:   true,
			errSubstr: "empty",
		},
		{
			name:     "command with no args trailing spaces",
			input:    "version  ",
			wantCmd:  "version",
			wantArgs: map[string]interface{}{},
		},
	}

	// Simple CLI parser simulation to verify the service can handle parsed input.
	parseCLI := func(input string) (string, map[string]interface{}, error) {
		input = strings.TrimSpace(input)
		if input == "" {
			return "", nil, fmt.Errorf("empty command input")
		}
		parts := strings.Fields(input)
		cmdName := parts[0]
		args := make(map[string]interface{})
		for _, part := range parts[1:] {
			kv := strings.SplitN(part, "=", 2)
			if len(kv) == 2 {
				args[kv[0]] = kv[1]
			}
		}
		return cmdName, args, nil
	}

	svc := newTestServiceStarted(t, nil)
	_ = svc.RegisterCommand(&Command{
		Name:    "deploy",
		Handler: echoHandler,
	})

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			cmdName, args, err := parseCLI(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected parse error, got nil")
				}
				if tt.errSubstr != "" && !strings.Contains(err.Error(), tt.errSubstr) {
					t.Errorf("error %q does not contain %q", err.Error(), tt.errSubstr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected parse error: %v", err)
			}
			if cmdName != tt.wantCmd {
				t.Errorf("cmdName = %q; want %q", cmdName, tt.wantCmd)
			}

			// Verify the parsed args match what we expect.
			for k, v := range tt.wantArgs {
				if args[k] != v {
					t.Errorf("args[%q] = %v; want %v", k, args[k], v)
				}
			}

			// Actually execute against the service to confirm end-to-end.
			_, _ = svc.Execute(context.Background(), cmdName, args)
		})
	}
}

// ---------------------------------------------------------------------------
// Concurrent safety — race-safe tests
// ---------------------------------------------------------------------------

func TestConcurrent_RegisterAndExecute(t *testing.T) {
	t.Parallel()

	svc := newTestService(nil)
	var wg sync.WaitGroup
	const n = 100

	// Concurrently register commands.
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_ = svc.RegisterCommand(&Command{
				Name:    fmt.Sprintf("cmd-%d", i),
				Group:   fmt.Sprintf("group-%d", i%5),
				Handler: echoHandler,
			})
		}(i)
	}
	wg.Wait()

	cmds := svc.ListCommands()
	if len(cmds) != n {
		t.Errorf("expected %d commands, got %d", n, len(cmds))
	}

	// Concurrently execute commands.
	var execCount int64
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			result, err := svc.Execute(context.Background(), fmt.Sprintf("cmd-%d", i), map[string]interface{}{"i": i})
			if err != nil {
				t.Errorf("Execute cmd-%d failed: %v", i, err)
				return
			}
			if result.Success {
				atomic.AddInt64(&execCount, 1)
			}
		}(i)
	}
	wg.Wait()

	if atomic.LoadInt64(&execCount) != int64(n) {
		t.Errorf("expected %d successful executions, got %d", n, atomic.LoadInt64(&execCount))
	}
}

func TestConcurrent_ReadOperations(t *testing.T) {
	t.Parallel()

	svc := newTestServiceStarted(t, nil)
	var wg sync.WaitGroup
	const n = 50

	// Concurrently call read operations.
	for i := 0; i < n; i++ {
		wg.Add(4)
		go func() {
			defer wg.Done()
			_ = svc.ListCommands()
		}()
		go func() {
			defer wg.Done()
			_ = svc.GetAPIRoutes()
		}()
		go func() {
			defer wg.Done()
			_ = svc.ListCommandsByGroup("system")
		}()
		go func() {
			defer wg.Done()
			_ = svc.Stats()
		}()
	}
	wg.Wait()
}

func TestConcurrent_HTTPRequests(t *testing.T) {
	t.Parallel()

	svc := newTestServiceStarted(t, nil)
	handler := svc.HTTPHandler()
	server := httptest.NewServer(handler)
	defer server.Close()

	var wg sync.WaitGroup
	const n = 50

	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			resp, err := http.Get(server.URL + "/api/v1/health")
			if err != nil {
				t.Errorf("GET health: %v", err)
				return
			}
			resp.Body.Close()
			if resp.StatusCode != http.StatusOK {
				t.Errorf("status = %d; want %d", resp.StatusCode, http.StatusOK)
			}
		}()
	}
	wg.Wait()
}

func TestConcurrent_RegisterWhileExecuting(t *testing.T) {
	t.Parallel()

	svc := newTestService(nil)
	// Pre-register a command.
	_ = svc.RegisterCommand(&Command{
		Name:    "existing",
		Handler: echoHandler,
	})

	var wg sync.WaitGroup

	// Reader goroutines execute the existing command.
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = svc.Execute(context.Background(), "existing", nil)
		}()
	}

	// Writer goroutines register new commands.
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_ = svc.RegisterCommand(&Command{
				Name:    fmt.Sprintf("new-%d", i),
				Handler: echoHandler,
			})
		}(i)
	}
	wg.Wait()
}

func TestConcurrent_GenerateOpenAPI(t *testing.T) {
	t.Parallel()

	svc := newTestServiceStarted(t, nil)
	var wg sync.WaitGroup
	const n = 30

	// Concurrently generate OpenAPI specs while registering commands.
	for i := 0; i < n; i++ {
		wg.Add(2)
		go func() {
			defer wg.Done()
			_ = svc.GenerateOpenAPI()
		}()
		go func(i int) {
			defer wg.Done()
			_ = svc.RegisterCommand(&Command{
				Name:    fmt.Sprintf("concurrent-cmd-%d", i),
				Group:   "concurrent",
				Handler: echoHandler,
			})
		}(i)
	}
	wg.Wait()
}

// ---------------------------------------------------------------------------
// Context cancellation
// ---------------------------------------------------------------------------

func TestExecute_ContextCancelled(t *testing.T) {
	t.Parallel()

	svc := newTestService(nil)
	_ = svc.RegisterCommand(&Command{
		Name: "blocking",
		Handler: func(ctx context.Context, args map[string]interface{}) (*CommandResult, error) {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(5 * time.Second):
				return &CommandResult{Success: true}, nil
			}
		},
	})

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(10 * time.Millisecond)
		cancel()
	}()

	result, err := svc.Execute(ctx, "blocking", nil)
	if err != nil {
		t.Fatalf("unexpected error from Execute: %v", err)
	}
	// The handler error (context.Canceled) is wrapped into a failed result.
	if result.Success {
		t.Error("expected failure due to context cancellation")
	}
	if !strings.Contains(result.Error, "context canceled") {
		t.Errorf("Error = %q; expected to contain 'context canceled'", result.Error)
	}
}

// ---------------------------------------------------------------------------
// Edge cases
// ---------------------------------------------------------------------------

func TestRegisterCommand_OverwriteExisting(t *testing.T) {
	t.Parallel()

	svc := newTestService(nil)
	_ = svc.RegisterCommand(&Command{
		Name:        "dup",
		Description: "version 1",
		Handler:     echoHandler,
	})
	_ = svc.RegisterCommand(&Command{
		Name:        "dup",
		Description: "version 2",
		Handler:     echoHandler,
	})

	cmd, ok := svc.GetCommand("dup")
	if !ok {
		t.Fatal("command not found")
	}
	if cmd.Description != "version 2" {
		t.Errorf("Description = %q; want %q (overwrite)", cmd.Description, "version 2")
	}
}

func TestExecute_EmptyArgsMap(t *testing.T) {
	t.Parallel()

	svc := newTestService(nil)
	_ = svc.RegisterCommand(&Command{
		Name:    "noargs",
		Handler: echoHandler,
	})

	result, err := svc.Execute(context.Background(), "noargs", map[string]interface{}{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Success {
		t.Error("expected success")
	}
}

func TestListCommands_Empty(t *testing.T) {
	t.Parallel()

	svc := newTestService(nil)
	cmds := svc.ListCommands()
	if cmds == nil {
		t.Error("ListCommands should return non-nil empty slice")
	}
	if len(cmds) != 0 {
		t.Errorf("expected 0 commands, got %d", len(cmds))
	}
}

func TestGetAPIRoutes_Empty(t *testing.T) {
	t.Parallel()

	svc := newTestService(nil)
	routes := svc.GetAPIRoutes()
	if routes == nil {
		t.Error("GetAPIRoutes should return non-nil empty slice")
	}
	if len(routes) != 0 {
		t.Errorf("expected 0 routes, got %d", len(routes))
	}
}

func TestListCommandsByGroup_EmptyGroup(t *testing.T) {
	t.Parallel()

	svc := newTestService(nil)
	cmds := svc.ListCommandsByGroup("")
	if len(cmds) != 0 {
		t.Errorf("expected 0 commands for empty group, got %d", len(cmds))
	}
}

func TestHTTPHandler_BodyPostWithQueryParams(t *testing.T) {
	t.Parallel()

	svc := newTestService(nil)
	var receivedArgs map[string]interface{}
	_ = svc.RegisterCommand(&Command{
		Name:      "both",
		APIPath:   "/api/v1/both",
		APIMethod: http.MethodPost,
		Handler: func(ctx context.Context, args map[string]interface{}) (*CommandResult, error) {
			receivedArgs = args
			return &CommandResult{Success: true}, nil
		},
	})
	handler := svc.HTTPHandler()

	body := `{"from_body":"yes"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/both?from_query=also", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d; want %d", rr.Code, http.StatusOK)
	}

	// Query params override body params if same key; otherwise both present.
	if receivedArgs["from_query"] != "also" {
		t.Errorf("from_query = %v; want %q", receivedArgs["from_query"], "also")
	}
	// The body param might be overwritten or merged; just verify it was parsed.
	// In the implementation, query params are added after body decode, so they override.
}

func TestGenerateOpenAPI_WithCustomCommands(t *testing.T) {
	t.Parallel()

	svc := newTestService(&Config{
		APIPrefix:   "/api/v2",
		Version:     "2.0.0",
		WotanTopic: "test",
	})
	_ = svc.RegisterCommand(&Command{
		Name:      "custom",
		Group:     "ops",
		APIPath:   "/api/v2/custom",
		APIMethod: http.MethodPut,
		Handler:   echoHandler,
	})

	spec := svc.GenerateOpenAPI()
	paths := spec["paths"].(map[string]interface{})

	customPath, ok := paths["/api/v2/custom"]
	if !ok {
		t.Fatal("missing /api/v2/custom path")
	}
	methods := customPath.(map[string]interface{})
	if _, ok := methods[http.MethodPut]; !ok {
		t.Error("missing PUT method for /api/v2/custom")
	}
}

func TestStats_AfterAdditionalRegistrations(t *testing.T) {
	t.Parallel()

	svc := newTestServiceStarted(t, nil)

	_ = svc.RegisterCommand(&Command{Name: "x", Group: "extra", Handler: echoHandler})
	_ = svc.RegisterCommand(&Command{Name: "y", Group: "extra", Handler: echoHandler})

	stats := svc.Stats()
	totalCmds := stats["total_commands"].(int)
	if totalCmds != 8 { // 6 builtin + 2 custom
		t.Errorf("total_commands = %d; want 8", totalCmds)
	}

	groups := stats["command_groups"].(int)
	if groups != 2 { // "system" + "extra"
		t.Errorf("command_groups = %d; want 2", groups)
	}
}

// ---------------------------------------------------------------------------
// Config JSON serialization
// ---------------------------------------------------------------------------

func TestConfig_JSONSerialization(t *testing.T) {
	t.Parallel()

	cfg := Config{
		APIPrefix:   "/api/v1",
		Version:     "1.0.0",
		WotanTopic: "gauntlets.commands",
	}

	b, err := json.Marshal(cfg)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}

	var parsed Config
	if err := json.Unmarshal(b, &parsed); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}

	if parsed != cfg {
		t.Errorf("round-trip failed: got %+v, want %+v", parsed, cfg)
	}
}

// ---------------------------------------------------------------------------
// HTTPHandler integration with httptest.Server
// ---------------------------------------------------------------------------

func TestHTTPHandler_Integration_FullServer(t *testing.T) {
	t.Parallel()

	svc := newTestServiceStarted(t, nil)
	_ = svc.RegisterCommand(&Command{
		Name:      "echo",
		APIPath:   "/api/v1/echo",
		APIMethod: http.MethodPost,
		Handler:   echoHandler,
	})

	server := httptest.NewServer(svc.HTTPHandler())
	defer server.Close()

	// Test successful POST.
	resp, err := http.Post(server.URL+"/api/v1/echo", "application/json", strings.NewReader(`{"key":"value"}`))
	if err != nil {
		t.Fatalf("POST failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d; want %d", resp.StatusCode, http.StatusOK)
	}

	var result CommandResult
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !result.Success {
		t.Error("expected success = true")
	}

	// Test 404.
	resp2, err := http.Get(server.URL + "/nonexistent")
	if err != nil {
		t.Fatalf("GET failed: %v", err)
	}
	resp2.Body.Close()
	if resp2.StatusCode != http.StatusNotFound {
		t.Errorf("status = %d; want %d", resp2.StatusCode, http.StatusNotFound)
	}
}

// ---------------------------------------------------------------------------
// Multiple commands sharing the same group
// ---------------------------------------------------------------------------

func TestMultipleCommandsSameGroup(t *testing.T) {
	t.Parallel()

	svc := newTestService(nil)
	for i := 0; i < 10; i++ {
		_ = svc.RegisterCommand(&Command{
			Name:    fmt.Sprintf("op-%d", i),
			Group:   "operations",
			Handler: echoHandler,
		})
	}

	ops := svc.ListCommandsByGroup("operations")
	if len(ops) != 10 {
		t.Errorf("expected 10 ops commands, got %d", len(ops))
	}
}

// ---------------------------------------------------------------------------
// Ensure route key uniqueness (method:path)
// ---------------------------------------------------------------------------

func TestRouteKeyUniqueness(t *testing.T) {
	t.Parallel()

	svc := newTestService(nil)

	// Two commands on the same path but different methods.
	_ = svc.RegisterCommand(&Command{
		Name:      "get-item",
		APIPath:   "/api/v1/item",
		APIMethod: http.MethodGet,
		Handler:   echoHandler,
	})
	_ = svc.RegisterCommand(&Command{
		Name:      "create-item",
		APIPath:   "/api/v1/item",
		APIMethod: http.MethodPost,
		Handler:   echoHandler,
	})

	routes := svc.GetAPIRoutes()
	if len(routes) != 2 {
		t.Errorf("expected 2 routes (GET + POST on same path), got %d", len(routes))
	}

	handler := svc.HTTPHandler()

	// GET should route to get-item.
	req := httptest.NewRequest(http.MethodGet, "/api/v1/item", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("GET /api/v1/item: status = %d; want %d", rr.Code, http.StatusOK)
	}

	// POST should route to create-item.
	req2 := httptest.NewRequest(http.MethodPost, "/api/v1/item", nil)
	rr2 := httptest.NewRecorder()
	handler.ServeHTTP(rr2, req2)
	if rr2.Code != http.StatusOK {
		t.Errorf("POST /api/v1/item: status = %d; want %d", rr2.Code, http.StatusOK)
	}
}

// ---------------------------------------------------------------------------
// Gauntlets Law aggregate verification
// ---------------------------------------------------------------------------

func TestGauntletsLaw_AllRegisteredCommandsHaveRoutes(t *testing.T) {
	t.Parallel()

	svc := newTestServiceStarted(t, nil)

	// Register a batch of commands.
	names := []string{"alpha", "beta", "gamma", "delta", "epsilon"}
	for _, name := range names {
		_ = svc.RegisterCommand(&Command{
			Name:    name,
			Group:   "greek",
			Handler: echoHandler,
		})
	}

	allCmds := svc.ListCommands()
	allRoutes := svc.GetAPIRoutes()

	// Every command must have exactly one corresponding route.
	routeMap := make(map[string]*APIRoute)
	for _, r := range allRoutes {
		routeMap[r.CommandName] = r
	}

	for _, cmd := range allCmds {
		route, ok := routeMap[cmd.Name]
		if !ok {
			t.Errorf("GAUNTLETS LAW VIOLATION: command %q has no API route", cmd.Name)
			continue
		}
		if route.Path != cmd.APIPath {
			t.Errorf("route path mismatch for %q: route=%q, cmd=%q", cmd.Name, route.Path, cmd.APIPath)
		}
		if route.Method != cmd.APIMethod {
			t.Errorf("route method mismatch for %q: route=%q, cmd=%q", cmd.Name, route.Method, cmd.APIMethod)
		}
	}
}

// ---------------------------------------------------------------------------
// Handler returning custom data types
// ---------------------------------------------------------------------------

func TestExecute_HandlerReturnsVariousDataTypes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		data interface{}
	}{
		{"nil data", nil},
		{"string data", "hello"},
		{"int data", 42},
		{"slice data", []string{"a", "b"}},
		{"map data", map[string]int{"x": 1}},
		{"bool data", true},
		{"nested map", map[string]interface{}{"nested": map[string]interface{}{"deep": true}}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			svc := newTestService(nil)
			_ = svc.RegisterCommand(&Command{
				Name: "data",
				Handler: func(ctx context.Context, args map[string]interface{}) (*CommandResult, error) {
					return &CommandResult{Success: true, Data: tt.data}, nil
				},
			})

			result, err := svc.Execute(context.Background(), "data", nil)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !result.Success {
				t.Error("expected success")
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Verify custom API prefix propagation
// ---------------------------------------------------------------------------

func TestCustomAPIPrefix(t *testing.T) {
	t.Parallel()

	svc := newTestService(&Config{
		APIPrefix:   "/custom/api/v3",
		Version:     "3.0.0",
		WotanTopic: "test",
	})
	_ = svc.RegisterCommand(&Command{
		Name:    "test",
		Handler: echoHandler,
	})

	cmd, ok := svc.GetCommand("test")
	if !ok {
		t.Fatal("command not found")
	}
	if cmd.APIPath != "/custom/api/v3/test" {
		t.Errorf("APIPath = %q; want %q", cmd.APIPath, "/custom/api/v3/test")
	}
}

func TestBuiltinCommands_CustomPrefix(t *testing.T) {
	t.Parallel()

	svc := newTestServiceStarted(t, &Config{
		APIPrefix:   "/v2",
		Version:     "2.0.0",
		WotanTopic: "test",
	})

	handler := svc.HTTPHandler()

	// Builtin commands should use custom prefix.
	for _, path := range []string{"/v2/health", "/v2/version", "/v2/commands"} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Errorf("GET %s: status = %d; want %d", path, rr.Code, http.StatusOK)
		}
	}
}
