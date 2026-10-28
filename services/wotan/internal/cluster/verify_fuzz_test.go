// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package cluster

import (
	"testing"

	"unheaded/services/wotan/internal/signing"
	"unheaded/services/wotan/internal/store"
	chatpb "unheaded/services/wotan/proto"
)

// S-TRUE-WIRE Step 386: a standby decodes whatever bytes a primary streams.
// VerifyEntry must not panic on any of them, with no policy or a real one,
// and must never accept a signed-topic entry when no key is trusted.
func FuzzVerifyEntry(f *testing.F) {
	f.Add([]byte("entry-1"))
	f.Add([]byte{1, '{', '}'})
	f.Add([]byte(`\x01{"room_id":"config.x","content":"v1","signed":{"signature":"AA==","public_key":"AA==","algorithm":"ml-dsa-65"}}`))
	f.Add([]byte("\x01{\"room_id\":\"logs.x\",\"content\":\"hi\"}"))
	p, err := signing.NewPolicy(nil)
	if err != nil {
		f.Fatal(err)
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		e := &chatpb.WALEntry{Sequence: 1, Data: data}
		for _, pol := range []*signing.Policy{nil, p} {
			if err := VerifyEntry(pol, e); err == nil {
				if msg, derr := store.DecodeMessage(data); derr == nil && pol.RequiresSignature(msg.RoomID) {
					t.Fatalf("accepted a signed-topic entry (%q) with no trusted key", msg.RoomID)
				}
			}
		}
	})
}
