// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

package auth

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"
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

// L16: a service refusing everyone for want of credentials still records it.
func TestSetupMiddleware_AuditsRefusalWithNoCredentials(t *testing.T) {
	buf := withAuditStderr(t)
	mw := SetupMiddleware(ServiceAuthConfig{Enabled: true, ServiceName: "svc"})
	ok := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	auditDo(mw(ok), http.MethodGet, "/api/v1/x", "")
	if !hasAudit(auditEvents(t, buf.Bytes()), AuditUnauthorized, http.MethodGet, "/api/v1/x") {
		t.Fatalf("no refusal recorded: %q", buf.String())
	}
}

const auditTestGRPCMethod = "/unheaded.v1.Example/Call"

// grpcAuditCall runs one call through the unary or the stream interceptor.
func grpcAuditCall(a Authenticator, ctx context.Context, stream bool) error {
	if stream {
		return GRPCStreamInterceptor(a)(nil, &mockServerStream{ctx: ctx},
			&grpc.StreamServerInfo{FullMethod: auditTestGRPCMethod},
			func(any, grpc.ServerStream) error { return nil })
	}
	_, err := GRPCUnaryInterceptor(a)(ctx, nil, &grpc.UnaryServerInfo{FullMethod: auditTestGRPCMethod},
		func(context.Context, any) (any, error) { return nil, nil })
	return err
}

func grpcPeerContext() context.Context {
	return peer.NewContext(context.Background(), &peer.Peer{Addr: &net.TCPAddr{IP: net.IPv4(192, 0, 2, 7), Port: 4242}})
}

// L16: a gRPC refusal is recorded with the method and the peer, whether or
// not the call carried a credential.
func TestGRPCInterceptors_RecordRefusals(t *testing.T) {
	calls := map[string]context.Context{
		"no metadata":      grpcPeerContext(),
		"no authorization": metadata.NewIncomingContext(grpcPeerContext(), metadata.MD{}),
		"bad token":        metadata.NewIncomingContext(grpcPeerContext(), metadata.Pairs("authorization", "Bearer not-a-token")),
	}
	for name, ctx := range calls {
		for _, stream := range []bool{false, true} {
			t.Run(name, func(t *testing.T) {
				buf := withAuditStderr(t)
				cfg, _ := serviceHandlerFixture(t)
				a, err := NewServiceAuthenticator(cfg)
				if err != nil {
					t.Fatal(err)
				}
				if c := status.Code(grpcAuditCall(a, ctx, stream)); c != codes.Unauthenticated {
					t.Fatalf("code = %v, want Unauthenticated", c)
				}
				events := auditEvents(t, buf.Bytes())
				if len(events) != 1 {
					t.Fatalf("events = %+v, want one", events)
				}
				e := events[0]
				if e.EventType != AuditUnauthorized || e.Result != "denied" ||
					e.Method != auditMethodGRPC || e.Endpoint != auditTestGRPCMethod || e.IP != "192.0.2.7" {
					t.Fatalf("event = %+v", e)
				}
			})
		}
	}
}

// L16: an authenticator with no trail of its own is still refused to, as
// before, with nothing to record.
func TestGRPCInterceptors_PlainAuthenticatorStillRefusesAnonymous(t *testing.T) {
	for _, stream := range []bool{false, true} {
		if c := status.Code(grpcAuditCall(&MultiAuthenticator{}, grpcPeerContext(), stream)); c != codes.Unauthenticated {
			t.Fatalf("code = %v, want Unauthenticated", c)
		}
	}
}

// L16: AUTH_AUDIT_LOG keeps allowed decisions on the paths with no RBAC too:
// SetupMiddleware and gRPC.
func TestAuditFile_KeepsAllowedDecisionsWithoutRBAC(t *testing.T) {
	cfg, bearer := serviceHandlerFixture(t)
	cfg.AuditLog = filepath.Join(t.TempDir(), "audit.log")
	ok := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	auditDo(SetupMiddleware(cfg)(ok), http.MethodGet, "/api/v1/ok", bearer("caller"))

	a, err := NewServiceAuthenticator(cfg)
	if err != nil {
		t.Fatal(err)
	}
	ctx := metadata.NewIncomingContext(grpcPeerContext(), metadata.Pairs("authorization", bearer("caller")))
	if err := grpcAuditCall(a, ctx, false); err != nil {
		t.Fatalf("valid token refused: %v", err)
	}
	raw, err := os.ReadFile(cfg.AuditLog)
	if err != nil {
		t.Fatal(err)
	}
	events := auditEvents(t, raw)
	if !hasAudit(events, AuditAuthenticated, http.MethodGet, "/api/v1/ok") {
		t.Errorf("no allowed HTTP decision in the file: %s", raw)
	}
	if !hasAudit(events, AuditAuthenticated, auditMethodGRPC, auditTestGRPCMethod) {
		t.Errorf("no allowed gRPC decision in the file: %s", raw)
	}
	for _, e := range events {
		if e.Subject != "caller" || e.Result != "allowed" {
			t.Errorf("event = %+v, want subject caller, allowed", e)
		}
	}
}

// L16: a service whose log will not open and which cannot exit refuses
// everything and keeps the trail on stderr.
func TestSetupMiddleware_UnusableAuditLogKeepsATrail(t *testing.T) {
	buf := withAuditStderr(t)
	cfg, bearer := serviceHandlerFixture(t)
	cfg.AuditLog = filepath.Join(t.TempDir(), "no", "such", "dir", "audit.log")
	ok := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	h := SetupMiddleware(cfg)(ok)
	r := httptest.NewRequest(http.MethodGet, "/api/v1/x", nil)
	r.Header.Set("Authorization", bearer("caller"))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", w.Code)
	}
	if !hasAudit(auditEvents(t, buf.Bytes()), AuditUnauthorized, http.MethodGet, "/api/v1/x") {
		t.Fatalf("no refusal recorded: %q", buf.String())
	}
}

type refusingAuthenticator struct{}

func (refusingAuthenticator) Authenticate(context.Context, *http.Request) (*Identity, error) {
	return nil, errors.New("refused")
}

// L16: a service whose only credential is a route authenticator (a BFF's
// session cookie) has its refusals recorded too.
func TestServiceChain_AuditsRouteAuthenticatorRefusal(t *testing.T) {
	buf := withAuditStderr(t)
	ok := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	h, err := NewServiceHandler(ok, ServiceAuthConfig{Enabled: true, ServiceName: "svc"},
		ServiceRoutes{Authenticators: []Authenticator{refusingAuthenticator{}}})
	if err != nil {
		t.Fatal(err)
	}
	auditDo(h, http.MethodGet, "/api/v1/x", "")
	if !hasAudit(auditEvents(t, buf.Bytes()), AuditUnauthorized, http.MethodGet, "/api/v1/x") {
		t.Fatalf("no refusal recorded: %q", buf.String())
	}
}

// L16: an event's size does not follow the size of the request.
func TestAuditLogger_CapsRequestFields(t *testing.T) {
	var buf bytes.Buffer
	NewAuditLogger(&buf, "svc").Log(AuditEvent{
		EventType: AuditUnauthorized,
		Endpoint:  "/" + strings.Repeat("a", 4*maxAuditField),
		Method:    strings.Repeat("M", 4*maxAuditField),
	})
	events := auditEvents(t, buf.Bytes())
	if len(events) != 1 || len(events[0].Endpoint) != maxAuditField || len(events[0].Method) != maxAuditField {
		t.Fatalf("event not capped to %d bytes: %d bytes", maxAuditField, buf.Len())
	}
}
