// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

package api

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"unheaded/pkg/crypto/pqc"
	"unheaded/pkg/wotansig"
)

func writeTopicConfig(t *testing.T, body string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "wotan.yaml")
	if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func testPubKeyB64(t *testing.T) (string, []byte) {
	t.Helper()
	_, s, err := pqc.GenerateMLDSAKeyPair(pqc.MLDSA65)
	if err != nil {
		t.Fatal(err)
	}
	return base64.StdEncoding.EncodeToString(s.PublicKey()), s.PublicKey()
}

func TestLoadTopicConfig_SignedPrefixes(t *testing.T) {
	k, raw := testPubKeyB64(t)
	cfg, err := LoadTopicConfig(writeTopicConfig(t, fmt.Sprintf(`
topics:
  auto_approve: [heimdall]
  signing_keys: []
  signed_prefixes:
    - prefix: "drift."
      keys: [%q]
    - prefix: "audit."
      keys: []
`, k)))
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	p, err := cfg.SigningPolicy()
	if err != nil {
		t.Fatalf("policy: %v", err)
	}
	for _, topic := range []string{"drift.detected.n1", "audit.x", "config.y"} {
		if !p.RequiresSignature(topic) || !cfg.RequiresSignature(topic) {
			t.Errorf("%s not signed", topic)
		}
	}
	if cfg.RequiresSignature("logs.x") {
		t.Error("logs.x signed")
	}
	if len(cfg.Topics.SignedPrefixes) != 2 || cfg.Topics.SignedPrefixes[0].Prefix != "drift." {
		t.Fatalf("parsed %+v", cfg.Topics.SignedPrefixes)
	}
	if v, ok := p.VerifierFor("drift.detected.n1"); !ok || v == nil {
		t.Fatal("no drift verifier")
	}
	_ = raw
}

// Each broken signed_prefixes entry stops Wotan loading its config: a typo
// must not leave a prefix unsigned or a key silently untrusted.
func TestLoadTopicConfig_SignedPrefixesRefused(t *testing.T) {
	k, _ := testPubKeyB64(t)
	short := base64.StdEncoding.EncodeToString([]byte("too-short-for-ml-dsa-65"))
	cases := map[string]string{
		"empty prefix":       `[{prefix: "", keys: []}]`,
		"no trailing dot":    `[{prefix: "drift", keys: []}]`,
		"wildcard":           `[{prefix: "drift.*", keys: []}]`,
		"duplicate":          `[{prefix: "drift.", keys: []}, {prefix: "drift.", keys: []}]`,
		"config prefix":      `[{prefix: "config.", keys: []}]`,
		"key not base64":     `[{prefix: "drift.", keys: ["not base64!"]}]`,
		"key wrong length":   fmt.Sprintf(`[{prefix: "drift.", keys: [%q]}]`, short),
		"key empty":          `[{prefix: "drift.", keys: [""]}]`,
		"unknown field":      fmt.Sprintf(`[{prefix: "drift.", key: [%q]}]`, k),
		"duplicate key list": fmt.Sprintf(`[{prefix: "drift.", keys: [%q, %q]}]`, k, k),
	}
	for name, list := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := LoadTopicConfig(writeTopicConfig(t, "topics:\n  signed_prefixes: "+list+"\n"))
			if err == nil {
				t.Fatalf("accepted signed_prefixes %s", list)
			}
			if !strings.Contains(err.Error(), "signed_prefixes") {
				t.Errorf("error does not name signed_prefixes: %v", err)
			}
		})
	}
}

// With no signed_prefixes at all (every config before ADR-105), drift.* is
// still signed and still closed.
func TestSigningPolicy_DefaultsCloseDrift(t *testing.T) {
	cfg, err := LoadTopicConfig(writeTopicConfig(t, "topics:\n  auto_approve: [x]\n"))
	if err != nil {
		t.Fatal(err)
	}
	p, err := cfg.SigningPolicy()
	if err != nil {
		t.Fatal(err)
	}
	if !p.RequiresSignature("drift.detected.n1") {
		t.Fatal("drift.* unsigned by default")
	}
	var none *TopicConfig
	if !none.RequiresSignature("drift.x") || none.RequiresSignature("logs.x") {
		t.Fatal("nil config must fall back to the built-in prefixes")
	}
	if p, err := none.SigningPolicy(); err != nil || !p.RequiresSignature("config.x") {
		t.Fatalf("nil config policy: %v", err)
	}
}

// drift.* and any configured signed prefix take only signed messages over
// gRPC: all three HTTP write paths refuse them, as they refuse config.*.
func TestHTTPRefusesSignedPrefixes(t *testing.T) {
	srv := setupTestServerWithTopics()
	srv.TopicConfig.Topics.SignedPrefixes = []SignedPrefix{{Prefix: "audit."}}

	for _, topic := range []string{"drift.detected.n1", "audit.log"} {
		id := subscribeID(t, srv, topic)
		w := topicReq(srv, http.MethodPost, "/api/v1/topics/"+topic+"/publish",
			fmt.Sprintf(`{"subscriber_id":%q,"payload":"forged"}`, id))
		if w.Code != http.StatusForbidden {
			t.Fatalf("HTTP publish to %s = %d, want 403: %s", topic, w.Code, w.Body.String())
		}
		if rm, err := srv.RoomManager.Get(topic); err == nil && len(rm.GetMessages()) != 0 {
			t.Fatalf("a %s message was stored", topic)
		}

		rm, _ := srv.RoomManager.GetOrCreate(topic, topic)
		mbr, _ := srv.MemberManager.RequestJoin(topic, "Mallory", "", 5*time.Minute)
		srv.MemberManager.Approve(mbr.ID, "admin")
		body, _ := json.Marshal(SendMessageRequest{MemberID: mbr.ID, RoomID: topic, Content: "forged"})
		rec := httptest.NewRecorder()
		srv.SendMessage(rec, httptest.NewRequest(http.MethodPost, "/send", bytes.NewReader(body)))
		if rec.Code != http.StatusForbidden || len(rm.GetMessages()) != 0 {
			t.Fatalf("room message to %s = %d", topic, rec.Code)
		}

		signed, _ := rm.SendMessage(mbr.ID, "signed")
		del, _ := json.Marshal(DeleteMessageRequest{MemberID: mbr.ID, RoomID: topic, MessageID: signed.ID, Content: "signed"})
		rec = httptest.NewRecorder()
		srv.DeleteMessage(rec, httptest.NewRequest(http.MethodPost, "/delete", bytes.NewReader(del)))
		if rec.Code != http.StatusForbidden || len(rm.GetMessages()) != 1 {
			t.Fatalf("delete on %s = %d", topic, rec.Code)
		}
	}
	other := subscribeID(t, srv, "kenoma.drift.detected")
	publish(t, srv, "kenoma.drift.detected", other, "ok")
}

// FuzzLoadTopicConfig: hostile YAML never panics, and a config that loads
// always yields a policy that requires the built-in prefixes.
func FuzzLoadTopicConfig(f *testing.F) {
	f.Add("topics:\n  signed_prefixes: [{prefix: \"drift.\", keys: []}]\n")
	f.Add("topics:\n  signed_prefixes: [{prefix: \"\", keys: [\"\"]}]\n")
	f.Add("topics: {auto_approve: [a], signing_keys: [Zm9v]}\n")
	f.Add("")
	dir := f.TempDir()
	f.Fuzz(func(t *testing.T, body string) {
		p := filepath.Join(dir, "c.yaml")
		if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
		cfg, err := LoadTopicConfig(p)
		if err != nil {
			return
		}
		pol, err := cfg.SigningPolicy()
		if err != nil {
			t.Fatalf("loaded config has no policy: %v", err)
		}
		if !pol.RequiresSignature("drift.x") || !pol.RequiresSignature("config.x") {
			t.Fatal("built-in prefixes not required")
		}
	})
}

// The shipped config loads, lists drift. with no keys, and so refuses every
// drift.* publish until a Heimdall key is enrolled (E5). No key is ever
// added to it from this repo's work.
func TestShippedConfig_DriftFailsClosed(t *testing.T) {
	cfg, err := LoadTopicConfig(filepath.Join("..", "..", "..", "..", "configs", "wotan.yaml"))
	if err != nil {
		t.Fatalf("configs/wotan.yaml: %v", err)
	}
	found := false
	for _, sp := range cfg.Topics.SignedPrefixes {
		if sp.Prefix == "drift." {
			found = true
			if len(sp.Keys) != 0 {
				t.Fatalf("configs/wotan.yaml trusts %d drift keys; enrolment is E5, not a repo change", len(sp.Keys))
			}
		}
	}
	if !found {
		t.Fatal("configs/wotan.yaml does not list drift.")
	}
	p, err := cfg.SigningPolicy()
	if err != nil {
		t.Fatal(err)
	}
	v, ok := p.VerifierFor("drift.detected.east")
	if !ok {
		t.Fatal("no drift verifier")
	}
	_, s, _ := pqc.GenerateMLDSAKeyPair(pqc.MLDSA65)
	if err := v.Verify("drift.detected.east", "heimdall", []byte("x"), signedFor(t, s, "drift.detected.east", "heimdall", "x")); err == nil {
		t.Fatal("shipped config accepted a drift signature")
	}
}

func signedFor(t *testing.T, s *pqc.MLDSASigner, topic, sender, payload string) wotansig.Signed {
	t.Helper()
	sg, err := wotansig.NewTopicSigner(s).Sign(topic, sender, []byte(payload))
	if err != nil {
		t.Fatal(err)
	}
	return sg
}
