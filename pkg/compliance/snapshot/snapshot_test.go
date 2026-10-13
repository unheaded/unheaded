// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package snapshot

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/cloudflare/circl/sign/mldsa/mldsa65"

	"unheaded/pkg/compliance/well"
	"unheaded/pkg/gungnir"
)

func signer(t *testing.T) (*gungnir.Signer, *mldsa65.PublicKey) {
	t.Helper()
	s, err := gungnir.NewSignerWithID(gungnir.GenerateDevKey())
	if err != nil {
		t.Fatal(err)
	}
	var pub mldsa65.PublicKey
	if err := pub.UnmarshalBinary(s.PublicKey()); err != nil {
		t.Fatal(err)
	}
	return s, &pub
}

func sealed(t *testing.T, host string) ([]byte, *mldsa65.PublicKey) {
	t.Helper()
	store, err := well.NewYAMLStore(filepath.Join("..", "..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	snap, err := Build(context.Background(), store, host, "abc123", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	s, pub := signer(t)
	raw, err := Seal(snap, s, 24*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	return raw, pub
}

// West is the example host: still bound to lab (east moved to lab-hardened
// on 2026-10-12).
func TestBuildSealOpen(t *testing.T) {
	raw, pub := sealed(t, "west")
	snap, err := Open(raw, pub, "west")
	if err != nil {
		t.Fatal(err)
	}
	if snap.Profile != "lab" || len(snap.Fixes) != 9 || snap.ModelCommit != "abc123" {
		t.Errorf("snapshot %s with %d fixes", snap.Profile, len(snap.Fixes))
	}
	for _, f := range snap.Fixes {
		if f.ID == "sshd-safe" || f.ID == "shell-tmout" {
			t.Errorf("%s is not in the lab profile but is in the snapshot", f.ID)
		}
	}
}

func TestOpenRefuses(t *testing.T) {
	raw, pub := sealed(t, "west")
	_, otherPub := signer(t)
	tamper := func(f func(*Envelope)) []byte {
		var env Envelope
		if err := json.Unmarshal(raw, &env); err != nil {
			t.Fatal(err)
		}
		f(&env)
		b, _ := json.Marshal(env)
		return b
	}
	cases := map[string]struct {
		raw  []byte
		pub  *mldsa65.PublicKey
		host string
	}{
		"other host":      {raw, pub, "east"},
		"other key":       {raw, otherPub, "west"},
		"payload edited":  {tamper(func(e *Envelope) { e.Payload = bytes.Replace(e.Payload, []byte(`"lab"`), []byte(`"all"`), 1) }), pub, "west"},
		"expired":         {tamper(func(e *Envelope) { e.Seal.ExpiresAtUnix = time.Now().Add(-time.Minute).Unix() }), pub, "west"},
		"downgraded algo": {tamper(func(e *Envelope) { e.Seal.Algorithm = "ed25519" }), pub, "west"},
		"unknown field":   {bytes.Replace(raw, []byte(`"payload"`), []byte(`"extra": 1, "payload"`), 1), pub, "west"},
		"not json":        {[]byte("nope"), pub, "west"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := Open(c.raw, c.pub, c.host); !errors.Is(err, ErrRefused) {
				t.Fatalf("got %v, want refusal", err)
			}
		})
	}
}

// A validly sealed snapshot whose fixes the model parser rejects (a key
// compromise or a signing-side bug) is still refused.
func TestOpenRevalidatesFixes(t *testing.T) {
	store, err := well.NewYAMLStore(filepath.Join("..", "..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	snap, err := Build(context.Background(), store, "east", "x", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	snap.Fixes[0].Actions[0].Run = "rm -rf /"
	snap.Fixes[0].Actions[0].File = nil
	s, pub := signer(t)
	raw, err := Seal(snap, s, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Open(raw, pub, "east"); !errors.Is(err, ErrRefused) {
		t.Fatalf("got %v, want refusal of a non-allowlisted command", err)
	}
}
