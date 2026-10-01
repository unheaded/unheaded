// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

// Package snapshot is what a host acts on (ADR-099 amended): one host's
// binding and exactly the fixes its profile turns on, exported from The
// Well and sealed with ML-DSA-65 (pkg/gungnir). A host never reads a live
// database row: a tampered row is at worst a snapshot nobody signs, and a
// snapshot is refused if its seal, expiry, host or content does not check.
package snapshot

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/cloudflare/circl/sign/mldsa/mldsa65"

	"unheaded/pkg/compliance/remediation"
	"unheaded/pkg/compliance/well"
	"unheaded/pkg/gungnir"
)

const version = 1

// ErrRefused wraps every reason a snapshot is not trusted.
var ErrRefused = errors.New("snapshot refused")

// Snapshot is one host's desired state.
type Snapshot struct {
	Version     int               `json:"version"`
	Host        string            `json:"host"`
	Profile     string            `json:"profile"`
	Mode        string            `json:"mode"`
	ModelCommit string            `json:"model_commit"`
	IssuedAt    time.Time         `json:"issued_at"`
	Fixes       []remediation.Fix `json:"fixes"`
}

// Envelope is the file a host receives: the exact payload bytes and their
// seal.
type Envelope struct {
	Payload []byte              `json:"payload"`
	Seal    gungnir.GungnirSeal `json:"seal"`
}

// Build takes host's binding from the store (The Well) and the fixes its
// profile turns on, in model order.
func Build(ctx context.Context, s well.Store, host, commit string, now time.Time) (*Snapshot, error) {
	ps, err := s.Profiles(ctx)
	if err != nil {
		return nil, err
	}
	bs, err := s.Bindings(ctx)
	if err != nil {
		return nil, err
	}
	for _, b := range bs {
		if b.Host != host {
			continue
		}
		on := map[string]bool{}
		for _, id := range ps.Resolve(b.Profile) {
			on[id] = true
		}
		snap := &Snapshot{Version: version, Host: host, Profile: b.Profile, Mode: b.Mode, ModelCommit: commit, IssuedAt: now.UTC()}
		for _, f := range s.Model().Fixes {
			if on[f.ID] {
				snap.Fixes = append(snap.Fixes, f)
			}
		}
		return snap, nil
	}
	return nil, fmt.Errorf("no binding for host %q", host)
}

// Seal signs the snapshot; ttl bounds how long a host will trust it.
func Seal(snap *Snapshot, s *gungnir.Signer, ttl time.Duration) ([]byte, error) {
	payload, err := json.Marshal(snap)
	if err != nil {
		return nil, err
	}
	seal, err := s.SignFor(payload, ttl)
	if err != nil {
		return nil, err
	}
	return json.MarshalIndent(Envelope{Payload: payload, Seal: *seal}, "", "  ")
}

func refuse(format string, a ...any) error {
	return fmt.Errorf("%w: %s", ErrRefused, fmt.Sprintf(format, a...))
}

// Open verifies an envelope for host and returns its snapshot: the seal
// must verify against pub and be unexpired, the snapshot must name host,
// be in audit mode (ADR-098), and its fixes must pass the same validation
// as the repository model.
func Open(raw []byte, pub *mldsa65.PublicKey, host string) (*Snapshot, error) {
	if len(raw) > 1<<20 {
		return nil, refuse("larger than 1 MiB")
	}
	var env Envelope
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&env); err != nil {
		return nil, refuse("envelope: %v", err)
	}
	if err := gungnir.Verify(env.Payload, &env.Seal, pub); err != nil {
		return nil, refuse("seal: %v", err)
	}
	var snap Snapshot
	dec = json.NewDecoder(bytes.NewReader(env.Payload))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&snap); err != nil {
		return nil, refuse("payload: %v", err)
	}
	switch {
	case snap.Version != version:
		return nil, refuse("version %d, want %d", snap.Version, version)
	case snap.Host != host:
		return nil, refuse("snapshot is for %q, this host is %q", snap.Host, host)
	case snap.Mode != "audit":
		return nil, refuse("mode %q: only audit until ADR-098's enforcing gate is met", snap.Mode)
	}
	// JSON is YAML: the fixes go through the model's own strict parser.
	fixes, err := json.Marshal(struct {
		Fixes []remediation.Fix `json:"fixes"`
	}{snap.Fixes})
	if err != nil {
		return nil, err
	}
	if _, err := remediation.Parse(fixes); err != nil {
		return nil, refuse("fixes: %v", err)
	}
	return &snap, nil
}

// Model is the snapshot's fixes as a model, for the executor.
func (s *Snapshot) Model() *remediation.Model {
	return &remediation.Model{Fixes: append([]remediation.Fix(nil), s.Fixes...)}
}
