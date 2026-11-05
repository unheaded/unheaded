// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

package anomaly

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"unheaded/pkg/auth"
	"unheaded/pkg/auth/authtest"
	"unheaded/pkg/logger"
)

// L6: with no ListenAddr the service binds loopback.
func TestListenAddr_DefaultsToLoopback(t *testing.T) {
	s := NewService(logger.New(io.Discard), nil)
	if got, want := s.listenAddr(), "127.0.0.1:19018"; got != want {
		t.Fatalf("default = %q, want %q", got, want)
	}
}

// L6: off loopback needs auth on.
func TestStart_RefusesOpenBindWithAuthOff(t *testing.T) {
	s := NewService(logger.New(io.Discard), nil)
	s.ListenAddr = ":0"
	if err := s.Start(context.Background()); !errors.Is(err, auth.ErrOpenListener) {
		t.Fatalf("Start: %v, want ErrOpenListener", err)
	}
}

// L6: with auth configured, the writes need a credential.
func TestHandler_Auth(t *testing.T) {
	f := authtest.New(t, "anomaly", "unheaded-cli")
	s := NewService(logger.New(io.Discard), nil)
	s.Auth = f.Config("anomaly", "unheaded-cli")
	h, err := s.handler()
	if err != nil {
		t.Fatal(err)
	}
	if c := authtest.Code(h, http.MethodGet, "/health", "", ""); c != http.StatusOK {
		t.Errorf("GET /health anonymous: %d, want 200", c)
	}
	body := `{"version":"1"}`
	if c := authtest.Code(h, http.MethodPost, "/api/v1/anomalies/model", body, ""); c != http.StatusUnauthorized {
		t.Errorf("POST model anonymous: %d, want 401", c)
	}
	if c := authtest.Code(h, http.MethodPost, "/api/v1/anomalies/model", body, f.Bearer("unheaded-cli", "anomaly")); !authtest.Authorized(c) {
		t.Errorf("POST model with a token: %d, want past auth", c)
	}
}

// L6: model and threshold bodies are bounded.
func TestWrites_BodyLimit(t *testing.T) {
	s := NewService(logger.New(io.Discard), nil)
	big := `{"version":"` + strings.Repeat("a", maxBodyBytes+1) + `"}`
	for path, h := range map[string]http.HandlerFunc{
		"/api/v1/anomalies/model":        s.handleModelUpdate,
		"/api/v1/anomalies/threshold/s1": s.handleThresholdOverride,
	} {
		w := httptest.NewRecorder()
		h(w, httptest.NewRequest(http.MethodPost, path, strings.NewReader(big)))
		if w.Code != http.StatusRequestEntityTooLarge {
			t.Errorf("POST %s over the limit: %d, want 413", path, w.Code)
		}
	}
}
