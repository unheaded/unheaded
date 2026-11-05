// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

package auth

import (
	"bufio"
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func auditEvents(t *testing.T, raw []byte) []AuditEvent {
	t.Helper()
	var out []AuditEvent
	sc := bufio.NewScanner(bytes.NewReader(raw))
	for sc.Scan() {
		var e AuditEvent
		if err := json.Unmarshal(sc.Bytes(), &e); err != nil {
			t.Fatalf("audit line %q: %v", sc.Text(), err)
		}
		out = append(out, e)
	}
	return out
}

func auditDo(h http.Handler, method, path, authz string) {
	r := httptest.NewRequest(method, path, strings.NewReader("{}"))
	if authz != "" {
		r.Header.Set("Authorization", authz)
	}
	h.ServeHTTP(httptest.NewRecorder(), r)
}

func hasAudit(events []AuditEvent, typ, method, path string) bool {
	for _, e := range events {
		if e.EventType == typ && e.Method == method && e.Endpoint == path {
			return true
		}
	}
	return false
}

// withAuditStderr points the default sink at a buffer for one test.
func withAuditStderr(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	saved := auditStderr
	auditStderr = &buf
	t.Cleanup(func() { auditStderr = saved })
	return &buf
}

// L16: with no AUTH_AUDIT_LOG, refusals reach the service's stderr, which is
// its journal. Allowed requests do not: one line per Wotan publish would
// drown it.
func TestServiceChain_AuditsRefusalsToStderr(t *testing.T) {
	buf := withAuditStderr(t)
	cfg, bearer := serviceHandlerFixture(t)
	ok := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	h, err := NewServiceHandler(ok, cfg, ServiceRoutes{})
	if err != nil {
		t.Fatal(err)
	}
	stolen := bearer("other")
	auditDo(h, http.MethodGet, "/health", "")
	auditDo(h, http.MethodGet, "/api/v1/anon", "")
	auditDo(h, http.MethodGet, "/api/v1/stranger", stolen)
	auditDo(h, http.MethodGet, "/api/v1/ok", bearer("caller"))

	raw := buf.Bytes()
	events := auditEvents(t, raw)
	if !hasAudit(events, AuditUnauthorized, http.MethodGet, "/api/v1/anon") {
		t.Errorf("no refusal recorded for an anonymous request: %s", raw)
	}
	if !hasAudit(events, AuditUnauthorized, http.MethodGet, "/api/v1/stranger") {
		t.Errorf("no refusal recorded for a caller off the allowlist: %s", raw)
	}
	for _, e := range events {
		if e.Endpoint == "/health" || e.Endpoint == "/api/v1/ok" {
			t.Errorf("recorded %+v, want refusals only", e)
		}
		if e.Service != cfg.ServiceName {
			t.Errorf("service = %q, want %q", e.Service, cfg.ServiceName)
		}
		if e.IP == "" {
			t.Errorf("no peer address in %+v", e)
		}
	}
	if tok := strings.TrimPrefix(stolen, "Bearer "); bytes.Contains(raw, []byte(tok)) {
		t.Error("a credential reached the audit log")
	}
}

// L16: a role refusal is recorded, with who was refused.
func TestServiceChain_AuditsRoleRefusal(t *testing.T) {
	buf := withAuditStderr(t)
	cfg, bearer := serviceHandlerFixture(t)
	rbac := DefaultServicePolicies(cfg.ServiceName)
	rbac.Policies = append([]EndpointPolicy{{PathPrefix: "/api/v1/admin-only", RequiredRoles: []string{RoleAdmin}}}, rbac.Policies...)
	ok := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	h, err := NewServiceHandler(ok, cfg, ServiceRoutes{RBAC: &rbac})
	if err != nil {
		t.Fatal(err)
	}
	auditDo(h, http.MethodGet, "/api/v1/admin-only", bearer("caller"))
	events := auditEvents(t, buf.Bytes())
	if !hasAudit(events, AuditRBACDenied, http.MethodGet, "/api/v1/admin-only") {
		t.Fatalf("no role refusal recorded: %s", buf.Bytes())
	}
	if events[0].Subject != "caller" || events[0].IP == "" {
		t.Errorf("event = %+v, want subject caller and a peer address", events[0])
	}
}

// L16: AUTH_AUDIT_LOG keeps the full trail, allowed requests included.
func TestServiceChain_AuditFileKeepsEveryDecision(t *testing.T) {
	path := filepath.Join(t.TempDir(), "audit.log")
	t.Setenv("AUTH_AUDIT_LOG", path)
	if got := LoadServiceAuthConfig("x").AuditLog; got != path {
		t.Fatalf("AuditLog = %q, want AUTH_AUDIT_LOG", got)
	}
	cfg, bearer := serviceHandlerFixture(t)
	cfg.AuditLog = path
	ok := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	h, err := NewServiceHandler(ok, cfg, ServiceRoutes{})
	if err != nil {
		t.Fatal(err)
	}
	auditDo(h, http.MethodGet, "/api/v1/anon", "")
	auditDo(h, http.MethodGet, "/api/v1/ok", bearer("caller"))
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	events := auditEvents(t, raw)
	if !hasAudit(events, AuditUnauthorized, http.MethodGet, "/api/v1/anon") {
		t.Errorf("no refusal in the file: %s", raw)
	}
	if !hasAudit(events, AuditRBACAllowed, http.MethodGet, "/api/v1/ok") {
		t.Errorf("no allowed decision in the file: %s", raw)
	}
}

// L16: an audit log that cannot be opened stops the service, rather than
// running with no trail.
func TestServiceChain_RefusesUnusableAuditLog(t *testing.T) {
	cfg, _ := serviceHandlerFixture(t)
	cfg.AuditLog = filepath.Join(t.TempDir(), "no", "such", "dir", "audit.log")
	if _, err := NewServiceChain(cfg, ServiceRoutes{}); err == nil {
		t.Fatal("unusable AUTH_AUDIT_LOG: no error, want refusal to start")
	}
}

// L16: gRPC listeners use NewServiceAuthenticator directly; a refusal there
// is recorded too.
func TestServiceAuthenticator_AuditsRefusal(t *testing.T) {
	buf := withAuditStderr(t)
	cfg, _ := serviceHandlerFixture(t)
	a, err := NewServiceAuthenticator(cfg)
	if err != nil {
		t.Fatal(err)
	}
	r := &http.Request{Header: http.Header{"Authorization": {"Bearer not-a-token"}}}
	if _, err := a.Authenticate(t.Context(), r); err == nil {
		t.Fatal("bad token accepted")
	}
	events := auditEvents(t, buf.Bytes())
	if len(events) != 1 || events[0].EventType != AuditUnauthorized {
		t.Fatalf("events = %+v, want one refusal", events)
	}
}
