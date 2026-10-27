// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis.

package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"unheaded/pkg/auth"
	"unheaded/pkg/auth/authtest"
	"unheaded/services/wotan/internal/member"
	"unheaded/services/wotan/internal/room"
	"unheaded/services/wotan/internal/wotan"
)

// authedHTTPWotan is Wotan's REST API behind auth.NewServiceHandler with
// AUTH_ALLOW_CALLERS timeguru, kev-poller, captain and auto-approve
// timeguru, captain; legacy display name "service" is listed too and must
// count for nothing with auth on.
type authedHTTPWotan struct {
	f   *authtest.Fixture
	s   *Server
	h   http.Handler
	cfg *TopicConfig
}

func newAuthedHTTPWotan(t *testing.T) *authedHTTPWotan {
	t.Helper()
	path := filepath.Join(t.TempDir(), "wotan.yaml")
	yml := "topics:\n  auto_approve: [timeguru, captain]\n  legacy_display_names: [service, kev-poller-label]\n"
	if err := os.WriteFile(path, []byte(yml), 0o600); err != nil {
		t.Fatal(err)
	}
	tc, err := LoadTopicConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	s := NewServer(room.NewManager(100), member.NewManager(), wotan.NewWotan(), time.Minute)
	s.TopicConfig = tc
	s.RequireIdentity = true
	mux := http.NewServeMux()
	mux.HandleFunc("/health", s.Health)
	mux.HandleFunc("/api/v1/topics/", s.TopicRouter)
	mux.HandleFunc("/api/v1/topics", s.TopicRouter)
	f := authtest.New(t, "wotan", "timeguru", "kev-poller", "captain", "outsider")
	h, err := auth.NewServiceHandler(mux, f.Config("wotan", "timeguru", "kev-poller", "captain"), auth.ServiceRoutes{})
	if err != nil {
		t.Fatal(err)
	}
	return &authedHTTPWotan{f: f, s: s, h: h, cfg: tc}
}

func (w *authedHTTPWotan) do(method, target, body, authz string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, target, strings.NewReader(body))
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	if authz != "" {
		req.Header.Set("Authorization", authz)
	}
	rec := httptest.NewRecorder()
	w.h.ServeHTTP(rec, req)
	return rec
}

// subscribe returns the subscriber's status and id.
func (w *authedHTTPWotan) subscribe(t *testing.T, authz, topic, name string) (int, string, string) {
	t.Helper()
	rec := w.do(http.MethodPost, "/api/v1/topics/"+topic+"/subscribe", `{"display_name":"`+name+`"}`, authz)
	var out struct {
		Subscriber TopicSubscriber `json:"subscriber"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	return rec.Code, out.Subscriber.Status, out.Subscriber.SubscriberID
}

func TestAuthE2E_HTTPWithoutTokenUnauthorized(t *testing.T) {
	w := newAuthedHTTPWotan(t)
	for _, c := range []struct{ method, target, body string }{
		{http.MethodPost, "/api/v1/topics/alerts.critical/subscribe", `{"display_name":"timeguru"}`},
		{http.MethodPost, "/api/v1/topics/alerts.critical/publish", `{"subscriber_id":"x","payload":"p"}`},
		{http.MethodGet, "/api/v1/topics/alerts.critical/messages", ""},
		{http.MethodGet, "/api/v1/topics", ""},
	} {
		if got := w.do(c.method, c.target, c.body, "").Code; got != http.StatusUnauthorized {
			t.Errorf("anonymous %s %s = %d, want 401", c.method, c.target, got)
		}
	}
	if got := w.do(http.MethodGet, "/health", "", "").Code; got != http.StatusOK {
		t.Errorf("/health = %d, want 200 without a token", got)
	}
}

func TestAuthE2E_HTTPWrongAudienceAndUntrustedKey(t *testing.T) {
	w := newAuthedHTTPWotan(t)
	// pkg/auth answers a wrong audience with 403 (ErrForbidden).
	if code, _, _ := w.subscribe(t, w.f.Bearer("timeguru", "captain"), "alerts.critical", "timeguru"); authtest.Authorized(code) {
		t.Errorf("aud=captain = %d, want 401 or 403", code)
	}
	if code, _, _ := w.subscribe(t, w.f.StrangerBearer("timeguru", "wotan"), "alerts.critical", "timeguru"); code != http.StatusUnauthorized {
		t.Errorf("untrusted kid = %d, want 401", code)
	}
	if code, _, _ := w.subscribe(t, w.f.Bearer("outsider", "wotan"), "alerts.critical", "timeguru"); code != http.StatusForbidden {
		t.Errorf("caller not in AUTH_ALLOW_CALLERS = %d, want 403", code)
	}
}

func TestAuthE2E_HTTPAutoApproveKeysOnVerifiedSub(t *testing.T) {
	w := newAuthedHTTPWotan(t)
	code, st, id := w.subscribe(t, w.f.Bearer("timeguru", "wotan"), "alerts.critical", "a-label")
	if code != http.StatusCreated || st != "approved" {
		t.Fatalf("timeguru (auto-approved sub, odd label) = %d %q, want 201 approved", code, st)
	}
	m, err := w.s.MemberManager.Get(mustUUID(t, id))
	if err != nil || m.Name != "timeguru" {
		t.Fatalf("member = %+v, %v: its name must be the verified sub", m, err)
	}
	// kev-poller may call Wotan, is not auto-approved, and claims names
	// that are: an auto-approved identity's and a legacy display name.
	for _, label := range []string{"timeguru", "service"} {
		code, st, _ := w.subscribe(t, w.f.Bearer("kev-poller", "wotan"), "alerts.critical", label)
		if code != http.StatusCreated || st != "pending" {
			t.Errorf("kev-poller labelled %q = %d %q, want 201 pending", label, code, st)
		}
	}
}

func TestAuthE2E_HTTPPublishBoundToCallerMember(t *testing.T) {
	w := newAuthedHTTPWotan(t)
	_, _, captainID := w.subscribe(t, w.f.Bearer("captain", "wotan"), "alerts.critical", "captain")
	body := `{"subscriber_id":"` + captainID + `","payload":"p"}`
	if got := w.do(http.MethodPost, "/api/v1/topics/alerts.critical/publish", body, w.f.Bearer("timeguru", "wotan")).Code; got != http.StatusForbidden {
		t.Errorf("timeguru through captain's subscriber_id = %d, want 403", got)
	}
	if got := w.do(http.MethodPost, "/api/v1/topics/alerts.critical/publish", body, w.f.Bearer("captain", "wotan")).Code; got != http.StatusCreated {
		t.Errorf("captain through its own subscriber_id = %d, want 201", got)
	}
	if got := w.do(http.MethodGet, "/api/v1/topics/alerts.critical/messages", "", w.f.Bearer("timeguru", "wotan")).Code; got != http.StatusOK {
		t.Errorf("authenticated read = %d, want 200", got)
	}
}

// RequireIdentity: a request that reaches the handler without a verified
// identity is refused, never approved by display name.
func TestAuthE2E_HTTPRequireIdentityRefusesBareRequest(t *testing.T) {
	w := newAuthedHTTPWotan(t)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/topics/alerts.critical/subscribe", strings.NewReader(`{"display_name":"timeguru"}`))
	w.s.TopicRouter(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("bare subscribe with auth required = %d, want 401", rec.Code)
	}
}

// With auth off, the display name still decides, and the legacy names
// count (the Appendix A3 rollback keeps today's behaviour).
func TestAutoApprove_AuthOffFallsBackToDisplayName(t *testing.T) {
	w := newAuthedHTTPWotan(t)
	for name, want := range map[string]bool{"timeguru": true, "service": true, "kev-poller-label": true, "nobody": false} {
		if got := w.cfg.IsAutoApproved(nil, name); got != want {
			t.Errorf("auth off, %q = %v, want %v", name, got, want)
		}
	}
	svc := &auth.Identity{Subject: "kev-poller-label", Roles: []string{auth.RoleService}, Extra: map[string]string{"auth_method": "eddsa"}}
	if w.cfg.IsAutoApproved(svc, "timeguru") {
		t.Error("a verified sub that is only a legacy display name must not be approved")
	}
	user := &auth.Identity{Subject: "timeguru", Roles: []string{auth.RoleOperator}, Extra: map[string]string{"auth_method": "jwks"}}
	if w.cfg.IsAutoApproved(user, "timeguru") {
		t.Error("only a verified service token is auto-approved, not a user or API key named like a service")
	}
}

func mustUUID(t *testing.T, s string) uuid.UUID {
	t.Helper()
	id, err := uuid.Parse(s)
	if err != nil {
		t.Fatalf("subscriber id %q: %v", s, err)
	}
	return id
}
