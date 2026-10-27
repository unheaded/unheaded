// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis. All rights reserved.

package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cloudflare/circl/sign/mldsa/mldsa65"

	"unheaded/pkg/metrics"
	"unheaded/pkg/wotansig"
)

type signedCall struct {
	topic  string
	signed bool
}

type fakeSigningPublisher struct{ calls []signedCall }

func (f *fakeSigningPublisher) PublishWithSigner(_ context.Context, topic string, _ []byte, signer *wotansig.TopicSigner) error {
	f.calls = append(f.calls, signedCall{topic, signer != nil})
	return nil
}

// writeHealthKey writes a seed generated at test run time (never committed).
func writeHealthKey(t *testing.T, mode os.FileMode) string {
	t.Helper()
	var seed [mldsa65.SeedSize]byte
	if _, err := rand.Read(seed[:]); err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(t.TempDir(), "akira-health.key")
	if err := os.WriteFile(p, seed[:], 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(p, mode); err != nil {
		t.Fatal(err)
	}
	return p
}

// S-IRON-GATE Phase 9: without HEALTH_SIGNING_KEY_FILE Akira refuses its
// two signed topics (consensus, remediate) and still votes on
// system.health.reports; it never sends them unsigned.
func TestWotanPublisher_NoKeyRefusesSignedTopics(t *testing.T) {
	f := &fakeSigningPublisher{}
	reg := metrics.NewRegistry()
	p := newWotanPublisher(f, nil, reg)
	p.publish(topicConsensus, map[string]string{"band": "OK"})
	p.publish(topicRemediate, map[string]string{"service": "x"})
	p.publish(topicConsensus, map[string]string{"band": "WARN"})
	p.publish(topicReports, map[string]string{"service": "x"})
	if len(f.calls) != 1 || f.calls[0].topic != topicReports {
		t.Fatalf("calls %+v, want only the report", f.calls)
	}
	var b bytes.Buffer
	if err := reg.Gather(&b); err != nil {
		t.Fatal(err)
	}
	out := b.String()
	for _, want := range []string{
		`akira_signed_publish_refused_total{topic="system.health.consensus"} 2`,
		`akira_signed_publish_refused_total{topic="system.health.remediate"} 1`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("metrics lack %s:\n%s", want, out)
		}
	}
}

func TestWotanPublisher_KeySignsConsensusAndRemediate(t *testing.T) {
	t.Setenv(wotansig.HealthKeyEnv, writeHealthKey(t, 0o400))
	signer, err := loadHealthSigner()
	if err != nil || signer == nil {
		t.Fatalf("loadHealthSigner: %v, %v", signer, err)
	}
	f := &fakeSigningPublisher{}
	p := newWotanPublisher(f, signer, metrics.NewRegistry())
	p.publish(topicConsensus, map[string]string{"band": "OK"})
	p.publish(topicRemediate, map[string]string{"service": "x"})
	p.publish(topicReports, map[string]string{"service": "x"})
	want := []signedCall{{topicConsensus, true}, {topicRemediate, true}, {topicReports, true}}
	if len(f.calls) != len(want) {
		t.Fatalf("calls %+v", f.calls)
	}
	for i := range want {
		if f.calls[i] != want[i] {
			t.Errorf("call %d = %+v, want %+v", i, f.calls[i], want[i])
		}
	}
}

func TestLoadHealthSigner(t *testing.T) {
	t.Setenv(wotansig.HealthKeyEnv, "")
	if s, err := loadHealthSigner(); s != nil || err != nil {
		t.Fatalf("unset: %v, %v; want nil, nil", s, err)
	}
	t.Setenv(wotansig.HealthKeyEnv, writeHealthKey(t, 0o644))
	if _, err := loadHealthSigner(); err == nil || !strings.Contains(err.Error(), wotansig.HealthKeyEnv) {
		t.Fatalf("a world-readable key: %v, want an error naming %s", err, wotansig.HealthKeyEnv)
	}
}

func TestWotanGRPCAddr(t *testing.T) {
	for _, c := range []struct{ http, grpc, want string }{
		{"wotan:18000", "", "wotan:18001"},
		{"http://localhost:18000", "", "localhost:18001"},
		{"https://wotan:18000", "", "wotan:18001"},
		{"localhost", "", "localhost:18001"},
		{"wotan:18000", "wotan-grpc:9001", "wotan-grpc:9001"},
		{"[::1]:18000", "", "[::1]:18001"},
	} {
		if got := wotanGRPCAddr(c.http, c.grpc); got != c.want {
			t.Errorf("wotanGRPCAddr(%q, %q) = %q, want %q", c.http, c.grpc, got, c.want)
		}
	}
}
