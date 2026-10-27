// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis.

package wotanClient

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"

	"unheaded/pkg/auth"
	"unheaded/pkg/auth/authtest"
	chatpb "unheaded/services/wotan/proto"
)

// authRecorder keeps every Authorization value a fake Wotan saw.
type authRecorder struct {
	mu   sync.Mutex
	seen []string
}

func (a *authRecorder) add(v string) {
	a.mu.Lock()
	a.seen = append(a.seen, v)
	a.mu.Unlock()
}

func (a *authRecorder) all() []string {
	a.mu.Lock()
	defer a.mu.Unlock()
	return append([]string(nil), a.seen...)
}

// fakeHTTPWotan answers subscribe, publish and messages, recording the
// Authorization header of each.
func fakeHTTPWotan(t *testing.T, rec *authRecorder) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rec.add(r.Header.Get("Authorization"))
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasSuffix(r.URL.Path, "/subscribe"):
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(map[string]any{"subscriber": map[string]any{
				"subscriber_id": "11111111-1111-1111-1111-111111111111", "topic": "t.a", "display_name": "x", "status": "approved"}})
		case strings.HasSuffix(r.URL.Path, "/publish"):
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"message_id":"m","seq":1}`))
		default:
			_, _ = w.Write([]byte(`{"messages":[]}`))
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

// verifier accepts tokens minted for aud=wotan by the allowed callers.
func verifier(t *testing.T, f *authtest.Fixture, allow ...string) auth.Authenticator {
	t.Helper()
	a, err := auth.NewServiceAuthenticator(f.Config(WotanAudience, allow...))
	if err != nil {
		t.Fatal(err)
	}
	return a
}

// subjectOf verifies an Authorization value as Wotan would and returns its sub.
func subjectOf(t *testing.T, a auth.Authenticator, authz string) string {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", authz)
	id, err := a.Authenticate(context.Background(), req)
	if err != nil {
		t.Fatalf("token %q does not verify for aud=wotan: %v", authz, err)
	}
	return id.Subject
}

func TestServiceToken_HTTPRequestsCarryWotanToken(t *testing.T) {
	f := authtest.New(t, "wotan", "timeguru")
	v := verifier(t, f, "timeguru")
	rec := &authRecorder{}
	srv := fakeHTTPWotan(t, rec)

	c, err := NewClient(strings.TrimPrefix(srv.URL, "http://"), WithServiceToken(f.Minter("timeguru")))
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	ctx := context.Background()
	if _, err := c.Subscribe(ctx, "t.a", "timeguru"); err != nil {
		t.Fatal(err)
	}
	if err := c.Publish(ctx, "t.a", []byte("x")); err != nil {
		t.Fatal(err)
	}
	if _, err := c.GetMessages(ctx, "t.a", 0, 10); err != nil {
		t.Fatal(err)
	}
	got := rec.all()
	if len(got) != 3 {
		t.Fatalf("requests = %d, want 3", len(got))
	}
	for i, h := range got {
		if sub := subjectOf(t, v, h); sub != "timeguru" {
			t.Errorf("request %d: sub %q, want timeguru", i, sub)
		}
	}
}

func TestServiceToken_NoMinterNoHeader(t *testing.T) {
	SetServiceIdentity(nil)
	rec := &authRecorder{}
	srv := fakeHTTPWotan(t, rec)
	c, err := NewClient(strings.TrimPrefix(srv.URL, "http://"))
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	if _, err := c.Subscribe(context.Background(), "t.a", "x"); err != nil {
		t.Fatal(err)
	}
	if got := rec.all(); len(got) != 1 || got[0] != "" {
		t.Fatalf("Authorization = %q, want none while no identity is set (Wotan still open)", got)
	}
}

func TestServiceToken_ProcessIdentityReachesEveryClient(t *testing.T) {
	f := authtest.New(t, "wotan", "akira")
	v := verifier(t, f, "akira")
	rec := &authRecorder{}
	srv := fakeHTTPWotan(t, rec)

	// Built before the identity is set, as pkg/healthreport's client may be.
	c, err := NewClient(strings.TrimPrefix(srv.URL, "http://"))
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	SetServiceIdentity(f.Minter("akira"))
	t.Cleanup(func() { SetServiceIdentity(nil) })
	if _, err := c.Subscribe(context.Background(), "t.a", "akira"); err != nil {
		t.Fatal(err)
	}
	if sub := subjectOf(t, v, rec.all()[0]); sub != "akira" {
		t.Fatalf("sub %q, want akira", sub)
	}
}

func TestServiceToken_UseServiceIdentityMissingKey(t *testing.T) {
	t.Setenv("AUTH_KEY_FILE", filepath.Join(t.TempDir(), "absent.key"))
	SetServiceIdentity(nil)
	m, err := UseServiceIdentity("kev-poller")
	if err != nil || m != nil {
		t.Fatalf("missing key: (%v, %v), want (nil, nil): a host before unheaded-keygen still runs", m, err)
	}
	if ServiceIdentity() != nil {
		t.Fatal("no identity should be set")
	}
}

func TestServiceToken_UseServiceIdentityLoadsKey(t *testing.T) {
	f := authtest.New(t, "wotan", "kev-poller")
	t.Setenv("AUTH_KEY_FILE", filepath.Join(f.Keys, "kev-poller.key"))
	m, err := UseServiceIdentity("kev-poller")
	t.Cleanup(func() { SetServiceIdentity(nil) })
	if err != nil || m == nil {
		t.Fatalf("UseServiceIdentity = (%v, %v)", m, err)
	}
	if ServiceIdentity() != m || m.Name() != "kev-poller" {
		t.Fatal("process identity not set to the loaded key")
	}
}

// fakeTopicServer records the authorization metadata of every call.
type fakeTopicServer struct {
	chatpb.UnimplementedTopicStreamServer
	rec *authRecorder
}

func (s *fakeTopicServer) note(ctx context.Context) {
	md, _ := metadata.FromIncomingContext(ctx)
	v := md.Get("authorization")
	if len(v) == 0 {
		s.rec.add("")
		return
	}
	s.rec.add(v[0])
}

func (s *fakeTopicServer) PublishTopic(ctx context.Context, _ *chatpb.TopicPublishRequest) (*chatpb.TopicPublishResponse, error) {
	s.note(ctx)
	return &chatpb.TopicPublishResponse{MessageId: "m", Seq: 1}, nil
}

func (s *fakeTopicServer) StreamTopics(_ *chatpb.TopicStreamRequest, stream chatpb.TopicStream_StreamTopicsServer) error {
	s.note(stream.Context())
	return stream.Send(&chatpb.TopicEvent{Topic: "t.a", MessageId: "m1", Payload: []byte("p"), Seq: 1,
		Type: chatpb.TopicEventType_TOPIC_MESSAGE_PUBLISHED})
}

func startFakeGRPC(t *testing.T, rec *authRecorder) string {
	t.Helper()
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	s := grpc.NewServer()
	chatpb.RegisterTopicStreamServer(s, &fakeTopicServer{rec: rec})
	go func() { _ = s.Serve(lis) }()
	t.Cleanup(s.Stop)
	return lis.Addr().String()
}

func TestServiceToken_GRPCUnaryAndStreamCarryToken(t *testing.T) {
	f := authtest.New(t, "wotan", "trace-collector")
	v := verifier(t, f, "trace-collector")
	rec := &authRecorder{}
	addr := startFakeGRPC(t, rec)

	gc, err := NewGRPCClient(addr, WithServiceToken(f.Minter("trace-collector")), WithInsecureTransport())
	if err != nil {
		t.Fatal(err)
	}
	defer gc.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, err := gc.Subscribe(ctx, "t.a", "trace-collector"); err != nil {
		t.Fatal(err)
	}
	if err := gc.Publish(ctx, "t.a", []byte("x")); err != nil {
		t.Fatal(err)
	}
	ch, err := gc.StreamMessages(ctx, "t.a")
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-ch:
	case <-ctx.Done():
		t.Fatal("no streamed message")
	}
	got := rec.all()
	if len(got) < 2 {
		t.Fatalf("calls = %d, want publish + stream", len(got))
	}
	for i, h := range got {
		if sub := subjectOf(t, v, h); sub != "trace-collector" {
			t.Errorf("call %d: sub %q", i, sub)
		}
	}
}

func TestServiceToken_TopicStreamClientCarriesToken(t *testing.T) {
	f := authtest.New(t, "wotan", "dashboard-backend")
	v := verifier(t, f, "dashboard-backend")
	rec := &authRecorder{}
	addr := startFakeGRPC(t, rec)

	c, err := NewTopicStreamClient(addr, WithStreamServiceToken(f.Minter("dashboard-backend")))
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, err := c.Subscribe(ctx, "t.a", "dashboard-backend"); err != nil {
		t.Fatal(err)
	}
	if err := c.Publish(ctx, "t.a", []byte("x")); err != nil {
		t.Fatal(err)
	}
	got := rec.all()
	if len(got) != 1 {
		t.Fatalf("calls = %d, want 1", len(got))
	}
	if sub := subjectOf(t, v, got[0]); sub != "dashboard-backend" {
		t.Fatalf("sub %q", sub)
	}
}

func TestServiceToken_GRPCNoMinterNoMetadata(t *testing.T) {
	SetServiceIdentity(nil)
	rec := &authRecorder{}
	addr := startFakeGRPC(t, rec)
	gc, err := NewGRPCClient(addr)
	if err != nil {
		t.Fatal(err)
	}
	defer gc.Close()
	ctx := context.Background()
	if _, err := gc.Subscribe(ctx, "t.a", "x"); err != nil {
		t.Fatal(err)
	}
	if err := gc.Publish(ctx, "t.a", []byte("x")); err != nil {
		t.Fatal(err)
	}
	if got := rec.all(); len(got) != 1 || got[0] != "" {
		t.Fatalf("authorization = %q, want none", got)
	}
}

// The token is minted per call: when the cached one nears exp, a fresh one
// replaces it (the minter's own refresh), with no client rebuild.
func TestServiceToken_RefreshesWhenMinterRotates(t *testing.T) {
	f := authtest.New(t, "wotan", "sword")
	priv, kid, err := auth.LoadPrivateKey(filepath.Join(f.Keys, "sword.key"))
	if err != nil {
		t.Fatal(err)
	}
	var mu sync.Mutex
	now := time.Now()
	clock := func() time.Time { mu.Lock(); defer mu.Unlock(); return now }
	m, err := auth.NewServiceTokenMinter("sword", priv, kid, clock)
	if err != nil {
		t.Fatal(err)
	}
	ta := &tokenAuth{minter: m}
	h1, err := ta.header()
	if err != nil {
		t.Fatal(err)
	}
	h2, _ := ta.header()
	if h1 != h2 {
		t.Fatal("token re-minted while still fresh")
	}
	mu.Lock()
	now = now.Add(auth.EdDSAServiceTokenTTL)
	mu.Unlock()
	h3, _ := ta.header()
	if h3 == h1 {
		t.Fatal("token not replaced after its lifetime")
	}
}

func TestServiceToken_PlaintextVersusTLS(t *testing.T) {
	if !(&tokenAuth{}).RequireTransportSecurity() {
		t.Fatal("a TLS client must never send its token over anything else")
	}
	if (&tokenAuth{plaintext: true}).RequireTransportSecurity() {
		t.Fatal("a plaintext client (before Phase 8) must still be able to send its token")
	}
}
