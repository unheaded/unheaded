// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package member

import (
	"fmt"
	"testing"
	"time"
)

// CleanupExpiredPending and Deny removed the member but left its room's
// byRoom entry behind as an empty slice. Anyone can make a pending member
// under any room name (an HTTP subscribe to a pattern such as "x123.*"
// creates no room, so the room cap does not apply), and pending members
// expire, so the member cap did not bound this either: one map key per
// name ever used, for the life of the process.
func TestExpiredAndDeniedMembersLeaveNoRoomKey(t *testing.T) {
	m := NewManager()
	for i := 0; i < 100; i++ {
		if _, err := m.RequestJoin(fmt.Sprintf("x%d.*", i), "stranger", "", -time.Second); err != nil {
			t.Fatal(err)
		}
	}
	if n := m.CleanupExpiredPending(); n != 100 {
		t.Fatalf("cleaned %d, want 100", n)
	}

	denied, err := m.RequestJoin("deny.me", "stranger", "", time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if err := m.Deny(denied.ID); err != nil {
		t.Fatal(err)
	}

	if m.Count() != 0 {
		t.Fatalf("members = %d, want 0", m.Count())
	}
	if n := len(m.byRoom); n != 0 {
		t.Fatalf("byRoom holds %d keys after every member went, want 0", n)
	}
}
