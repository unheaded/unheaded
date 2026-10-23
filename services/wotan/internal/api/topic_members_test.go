// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package api

import (
	"testing"
)

// An allowlisted subscribe made a new approved member on every call, and
// approved members are never reaped. Clients subscribe at start-up and again
// whenever Wotan forgets them, so each client restart left its old members
// behind for good. East, 2026-10-23: 93 members, of which 7 were
// trace-collector's logs.trace-collector.* members from before its restart
// on 2026-10-22 18:44 (each of those topics had two subscriber_created
// lines for it since Wotan started). Past --max-members every new
// subscriber is refused. One approved member per (topic, name) is enough,
// as on the gRPC unnamed-publisher path.
func TestSubscribeTopic_ResubscribeReusesTheApprovedMember(t *testing.T) {
	srv := setupTestServerWithTopics()

	first := subscribeID(t, srv, "logs.svc.info")
	for i := 0; i < 20; i++ {
		if id := subscribeID(t, srv, "logs.svc.info"); id != first {
			t.Fatalf("resubscribe %d: subscriber_id %s, want the first one (%s)", i, id, first)
		}
	}
	if got := srv.MemberManager.Count(); got != 1 {
		t.Fatalf("21 subscribes as one name to one topic left %d members, want 1", got)
	}
	publish(t, srv, "logs.svc.info", first, "still works")

	subscribeID(t, srv, "logs.svc.warn")
	if got := srv.MemberManager.Count(); got != 2 {
		t.Fatalf("members = %d, want 2: one per (topic, name)", got)
	}
}

// Names not on the allowlist still get a pending member per request: each
// is a separate request for an admin to judge, and pending members expire.
func TestSubscribeTopic_PendingStillOnePerRequest(t *testing.T) {
	srv := setupTestServerWithTopics()
	srv.TopicConfig = &TopicConfig{allowSet: map[string]struct{}{"other": {}}}

	for i := 0; i < 3; i++ {
		w := topicReq(srv, "POST", "/api/v1/topics/logs.svc.info/subscribe", `{"display_name":"stranger"}`)
		if w.Code != 201 {
			t.Fatalf("subscribe: %d %s", w.Code, w.Body.String())
		}
	}
	if got := len(srv.MemberManager.GetAllPending()); got != 3 {
		t.Fatalf("pending = %d, want 3", got)
	}
}
