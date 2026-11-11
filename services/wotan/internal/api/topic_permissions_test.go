// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

package api

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"unheaded/pkg/auth"
	"unheaded/pkg/auth/authtest"
	"unheaded/services/wotan/internal/member"
	"unheaded/services/wotan/internal/room"
	"unheaded/services/wotan/internal/wotan"
)

// permissionsYAML is a topics section with permissions (FND-043):
// timeguru reads two topics and publishes one; captain reads everything;
// kev-poller is approved but unlisted.
const permissionsYAML = `topics:
  auto_approve: [timeguru, captain, kev-poller]
  permissions:
    unlisted: deny
    services:
      timeguru:
        subscribe: ["alerts.critical", "timeline.*"]
        publish: ["system.health.reports"]
      captain:
        subscribe: ["#"]
        publish: ["alerts.*"]
`

func loadYAML(t *testing.T, yml string) *TopicConfig {
	t.Helper()
	path := filepath.Join(t.TempDir(), "wotan.yaml")
	if err := os.WriteFile(path, []byte(yml), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadTopicConfig(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	return cfg
}

func serviceIdentity(name string) *auth.Identity {
	return &auth.Identity{Subject: name, Roles: []string{auth.RoleService}, Extra: map[string]string{"auth_method": "eddsa"}}
}

func TestPermissions_ParseRefusesMistakes(t *testing.T) {
	for name, yml := range map[string]string{
		"unknown field":     "topics:\n  permissions:\n    unlisted: deny\n    service:\n      a: {subscribe: [x]}\n",
		"unknown entry key": "topics:\n  permissions:\n    services:\n      a: {subscribes: [x]}\n",
		"bad unlisted":      "topics:\n  permissions:\n    unlisted: maybe\n",
		"bad mode":          "topics:\n  permissions:\n    mode: log\n",
		"bad pattern":       "topics:\n  permissions:\n    services:\n      a: {subscribe: [\"tasks..x\"]}\n",
		"bad pattern chars": "topics:\n  permissions:\n    services:\n      a: {publish: [\"a b\"]}\n",
		"empty identity":    "topics:\n  permissions:\n    services:\n      \"  \": {subscribe: [x]}\n",
		"not a mapping":     "topics:\n  permissions: [a, b]\n",
		"entry not mapping": "topics:\n  permissions:\n    services:\n      a: [x]\n",
		"hash not last":     "topics:\n  permissions:\n    services:\n      a: {subscribe: [\"a.#.b\"]}\n",
		"duplicate by case": "topics:\n  permissions:\n    services:\n      a: {subscribe: [x]}\n      A: {subscribe: [y]}\n",
	} {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "wotan.yaml")
			if err := os.WriteFile(path, []byte(yml), 0o600); err != nil {
				t.Fatal(err)
			}
			if _, err := LoadTopicConfig(path); err == nil {
				t.Fatalf("loaded:\n%s", yml)
			}
		})
	}
}

func TestPermissions_Permit(t *testing.T) {
	cfg := loadYAML(t, permissionsYAML)
	timeguru, captain, kev := serviceIdentity("timeguru"), serviceIdentity("captain"), serviceIdentity("kev-poller")
	user := &auth.Identity{Subject: "captain", Roles: []string{auth.RoleAdmin}, Extra: map[string]string{"auth_method": "jwks"}}
	cases := []struct {
		name    string
		id      *auth.Identity
		action  string
		pattern string
		want    bool
	}{
		{"listed topic", timeguru, PermitSubscribe, "alerts.critical", true},
		{"covered by one-level", timeguru, PermitSubscribe, "timeline.updates", true},
		{"the pattern itself", timeguru, PermitSubscribe, "timeline.*", true},
		{"deeper than the allowance", timeguru, PermitSubscribe, "timeline.#", false},
		{"other topic", timeguru, PermitSubscribe, "tasks.created", false},
		{"everything", timeguru, PermitSubscribe, "#", false},
		{"may read what it publishes", timeguru, PermitSubscribe, "system.health.reports", true},
		{"publish listed", timeguru, PermitPublish, "system.health.reports", true},
		{"publish a read-only topic", timeguru, PermitPublish, "alerts.critical", false},
		{"publish a pattern", captain, PermitPublish, "alerts.*", false},
		{"publish under a pattern", captain, PermitPublish, "alerts.disk", true},
		{"wildcard reader", captain, PermitSubscribe, "#", true},
		{"wildcard reader, any topic", captain, PermitSubscribe, "audit.auth.login", true},
		{"unlisted identity", kev, PermitSubscribe, "alerts.critical", false},
		{"unlisted identity publish", kev, PermitPublish, "threats.cisa-kev.updates", false},
		{"a user named like a service", user, PermitSubscribe, "alerts.critical", false},
		{"case of the identity", serviceIdentity("TimeGuru"), PermitSubscribe, "alerts.critical", true},
		{"unknown action", timeguru, "delete", "alerts.critical", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, reason := cfg.Permit(c.id, c.action, c.pattern)
			if got != c.want {
				t.Fatalf("Permit(%s, %s, %q) = %v (%s), want %v", c.id.Subject, c.action, c.pattern, got, reason, c.want)
			}
			if got && reason != "" {
				t.Fatalf("an allowed call carries a reason: %q", reason)
			}
		})
	}

	// No identity (auth off), no section, nil config: everything goes, as before FND-043.
	if ok, _ := cfg.Permit(nil, PermitSubscribe, "#"); !ok {
		t.Error("auth off: refused")
	}
	plain := loadYAML(t, "topics:\n  auto_approve: [timeguru]\n")
	if ok, _ := plain.Permit(timeguru, PermitSubscribe, "#"); !ok {
		t.Error("no permissions section: refused")
	}
	var none *TopicConfig
	if ok, _ := none.Permit(timeguru, PermitSubscribe, "#"); !ok {
		t.Error("nil config: refused")
	}
	if plain.PermissionsConfigured() || !cfg.PermissionsConfigured() {
		t.Error("PermissionsConfigured is wrong")
	}
}

func TestPermissions_UnlistedAllowAndAudit(t *testing.T) {
	kev := serviceIdentity("kev-poller")
	open := loadYAML(t, "topics:\n  permissions:\n    unlisted: allow\n    services:\n      timeguru: {subscribe: [x]}\n")
	if ok, _ := open.Permit(kev, PermitSubscribe, "#"); !ok {
		t.Error("unlisted: allow refused an unlisted identity")
	}
	if ok, _ := open.Permit(serviceIdentity("timeguru"), PermitSubscribe, "#"); ok {
		t.Error("unlisted: allow widened a listed identity")
	}
	audit := loadYAML(t, "topics:\n  permissions:\n    mode: audit\n    services:\n      timeguru: {subscribe: [x]}\n")
	if ok, reason := audit.Permit(serviceIdentity("timeguru"), PermitSubscribe, "#"); !ok || reason != "" {
		t.Errorf("audit mode: refused (%v, %q); it must let the call through", ok, reason)
	}
}

func TestPermissions_ReloadSwapsThem(t *testing.T) {
	path := filepath.Join(t.TempDir(), "wotan.yaml")
	if err := os.WriteFile(path, []byte(permissionsYAML), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadTopicConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	timeguru := serviceIdentity("timeguru")
	if ok, _ := cfg.Permit(timeguru, PermitSubscribe, "tasks.created"); ok {
		t.Fatal("before reload: tasks.created allowed")
	}
	next := strings.Replace(permissionsYAML, `subscribe: ["alerts.critical", "timeline.*"]`, `subscribe: ["alerts.critical", "timeline.*", "tasks.*"]`, 1)
	if err := os.WriteFile(path, []byte(next), 0o600); err != nil {
		t.Fatal(err)
	}
	res, err := cfg.Reload(path)
	if err != nil {
		t.Fatal(err)
	}
	if !res.PermissionsChanged {
		t.Error("reload did not report the permissions change")
	}
	if ok, _ := cfg.Permit(timeguru, PermitSubscribe, "tasks.created"); !ok {
		t.Fatal("after reload: tasks.created still refused")
	}
	// A broken section is refused and the running one kept.
	if err := os.WriteFile(path, []byte("topics:\n  permissions:\n    mode: nope\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := cfg.Reload(path); err == nil {
		t.Fatal("a bad mode reloaded")
	}
	if ok, _ := cfg.Permit(timeguru, PermitSubscribe, "tasks.created"); !ok {
		t.Fatal("a refused reload changed the running permissions")
	}
	// Removing the section is a change too, back to the old behaviour.
	if err := os.WriteFile(path, []byte("topics:\n  auto_approve: [timeguru]\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if res, err := cfg.Reload(path); err != nil || !res.PermissionsChanged {
		t.Fatalf("remove section: %v, changed %v", err, res.PermissionsChanged)
	}
	if ok, _ := cfg.Permit(timeguru, PermitSubscribe, "#"); !ok {
		t.Fatal("section removed, still refused")
	}
}

// permittedHTTPWotan is the REST API behind real auth with permissionsYAML.
type permittedHTTPWotan struct {
	f *authtest.Fixture
	h http.Handler
}

func newPermittedHTTPWotan(t *testing.T) *permittedHTTPWotan {
	t.Helper()
	s := NewServer(room.NewManager(100), member.NewManager(), wotan.NewWotan(), time.Minute)
	s.TopicConfig = loadYAML(t, permissionsYAML)
	s.RequireIdentity = true
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/topics/", s.TopicRouter)
	mux.HandleFunc("/api/v1/topics", s.TopicRouter)
	f := authtest.New(t, "wotan", "timeguru", "kev-poller", "captain")
	h, err := auth.NewServiceHandler(mux, f.Config("wotan", "timeguru", "kev-poller", "captain"), auth.ServiceRoutes{})
	if err != nil {
		t.Fatal(err)
	}
	return &permittedHTTPWotan{f: f, h: h}
}

func (w *permittedHTTPWotan) do(method, target, body, who string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, target, strings.NewReader(body))
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Authorization", w.f.Bearer(who, "wotan"))
	rec := httptest.NewRecorder()
	w.h.ServeHTTP(rec, req)
	return rec
}

func TestPermissions_HTTPSubscribeReadPublish(t *testing.T) {
	w := newPermittedHTTPWotan(t)

	// timeguru: its topics yes, others 403 with the access-denied code.
	if rec := w.do(http.MethodPost, "/api/v1/topics/alerts.critical/subscribe", `{}`, "timeguru"); rec.Code != http.StatusCreated {
		t.Fatalf("timeguru subscribe alerts.critical: %d %s", rec.Code, rec.Body.String())
	}
	for _, topic := range []string{"tasks.created", "audit.auth.login", "system.health.consensus"} {
		rec := w.do(http.MethodPost, "/api/v1/topics/"+topic+"/subscribe", `{}`, "timeguru")
		if rec.Code != http.StatusForbidden || !strings.Contains(rec.Body.String(), "may not subscribe") {
			t.Fatalf("timeguru subscribe %s: %d %s, want 403", topic, rec.Code, rec.Body.String())
		}
		if got := rec.Header().Get("X-Wotan-Error"); got == "" {
			t.Errorf("subscribe %s: no X-Wotan-Error code on the refusal", topic)
		}
	}

	// Reading a topic's messages is gated like a subscription, pattern or not.
	for _, target := range []string{"/api/v1/topics/tasks.created/messages", "/api/v1/topics/tasks.*/messages", "/api/v1/topics/%23/messages", "/api/v1/topics/timeline.%23/messages"} {
		if rec := w.do(http.MethodGet, target, "", "timeguru"); rec.Code != http.StatusForbidden {
			t.Fatalf("timeguru GET %s: %d %s, want 403", target, rec.Code, rec.Body.String())
		}
	}
	for _, target := range []string{"/api/v1/topics/alerts.critical/messages", "/api/v1/topics/timeline.*/messages", "/api/v1/topics/timeline.updates/messages?after_seq=0"} {
		if rec := w.do(http.MethodGet, target, "", "timeguru"); rec.Code != http.StatusOK {
			t.Fatalf("timeguru GET %s: %d %s, want 200", target, rec.Code, rec.Body.String())
		}
	}
	// captain reads everything.
	if rec := w.do(http.MethodGet, "/api/v1/topics/%23/messages", "", "captain"); rec.Code != http.StatusOK {
		t.Fatalf("captain GET #: %d %s", rec.Code, rec.Body.String())
	}

	// Publish: timeguru on system.health.reports yes, on alerts.critical no,
	// each through its own approved member.
	rec := w.do(http.MethodPost, "/api/v1/topics/system.health.reports/subscribe", `{}`, "timeguru")
	if rec.Code != http.StatusCreated {
		t.Fatalf("timeguru subscribe system.health.reports: %d %s", rec.Code, rec.Body.String())
	}
	id := subscriberIDFrom(t, rec)
	if rec := w.do(http.MethodPost, "/api/v1/topics/system.health.reports/publish", `{"subscriber_id":"`+id+`","payload":"ok"}`, "timeguru"); rec.Code != http.StatusCreated {
		t.Fatalf("timeguru publish system.health.reports: %d %s", rec.Code, rec.Body.String())
	}
	rec = w.do(http.MethodPost, "/api/v1/topics/alerts.critical/subscribe", `{}`, "timeguru")
	id = subscriberIDFrom(t, rec)
	if rec := w.do(http.MethodPost, "/api/v1/topics/alerts.critical/publish", `{"subscriber_id":"`+id+`","payload":"no"}`, "timeguru"); rec.Code != http.StatusForbidden || !strings.Contains(rec.Body.String(), "may not publish") {
		t.Fatalf("timeguru publish alerts.critical: %d %s, want 403", rec.Code, rec.Body.String())
	}

	// kev-poller is approved but unlisted: nothing.
	if rec := w.do(http.MethodPost, "/api/v1/topics/threats.cisa-kev.updates/subscribe", `{}`, "kev-poller"); rec.Code != http.StatusForbidden {
		t.Fatalf("kev-poller subscribe: %d %s, want 403", rec.Code, rec.Body.String())
	}
	if rec := w.do(http.MethodGet, "/api/v1/topics/alerts.critical/messages", "", "kev-poller"); rec.Code != http.StatusForbidden {
		t.Fatalf("kev-poller read: %d %s, want 403", rec.Code, rec.Body.String())
	}
}

func subscriberIDFrom(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	body := rec.Body.String()
	i := strings.Index(body, `"subscriber_id":"`)
	if i < 0 {
		t.Fatalf("no subscriber_id in %s", body)
	}
	rest := body[i+len(`"subscriber_id":"`):]
	return rest[:strings.Index(rest, `"`)]
}
