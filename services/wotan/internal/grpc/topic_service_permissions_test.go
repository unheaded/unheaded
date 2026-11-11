// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

package grpc

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"unheaded/services/wotan/internal/api"
	"unheaded/services/wotan/internal/member"
	chatpb "unheaded/services/wotan/proto"
)

// FND-043: with topics.permissions, a verified identity streams and
// publishes only inside its lists, on gRPC as on HTTP.
func newPermittedWotan(t *testing.T) *authedWotan {
	t.Helper()
	w := newAuthedWotan(t)
	path := filepath.Join(t.TempDir(), "wotan.yaml")
	yml := `topics:
  auto_approve: [timeguru, captain]
  permissions:
    services:
      timeguru:
        subscribe: ["alerts.critical", "timeline.*"]
        publish: ["system.health.reports"]
      captain:
        subscribe: ["#"]
        publish: ["alerts.*", "system.health.reports"]
`
	if err := os.WriteFile(path, []byte(yml), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := api.LoadTopicConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	w.svc.Permissions = cfg
	return w
}

func (w *authedWotan) streamErr(t *testing.T, who, pattern string) error {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	st, err := chatpb.NewTopicStreamClient(w.conn).StreamTopics(withToken(ctx, w.f.Bearer(who, "wotan")),
		&chatpb.TopicStreamRequest{TopicPattern: pattern, DisplayName: who})
	if err != nil {
		return err
	}
	// A refusal arrives on the first Recv; an allowed stream has at least
	// one message to deliver here, so Recv does not block.
	_, err = st.Recv()
	return err
}

func TestPermissions_GRPCStreamAndPublish(t *testing.T) {
	w := newPermittedWotan(t)

	// captain seeds one message where timeguru may read, and one where it may not.
	if err := w.publish(t, w.f.Bearer("captain", "wotan"), "alerts.critical", "", ""); err != nil {
		t.Fatal(err)
	}
	if err := w.publish(t, w.f.Bearer("captain", "wotan"), "alerts.disk", "", ""); err != nil {
		t.Fatal(err)
	}

	for _, tc := range []struct {
		who, pattern string
		want         codes.Code
	}{
		{"timeguru", "alerts.critical", codes.OK},
		{"timeguru", "alerts.*", codes.PermissionDenied},
		{"timeguru", "#", codes.PermissionDenied},
		{"timeguru", "*", codes.PermissionDenied},
		{"timeguru", "timeline.#", codes.PermissionDenied},
		{"timeguru", "alerts.disk", codes.PermissionDenied},
		{"captain", "#", codes.OK},
		{"captain", "alerts.disk", codes.OK},
	} {
		err := w.streamErr(t, tc.who, tc.pattern)
		if status.Code(err) != tc.want {
			t.Errorf("%s streams %q: %v, want %v", tc.who, tc.pattern, err, tc.want)
		}
	}

	// Publish: inside the list yes, outside PermissionDenied, and a refused
	// publish leaves no member of timeguru's behind in that room.
	if err := w.publish(t, w.f.Bearer("timeguru", "wotan"), "system.health.reports", "", ""); err != nil {
		t.Fatalf("timeguru publish system.health.reports: %v", err)
	}
	if err := w.publish(t, w.f.Bearer("timeguru", "wotan"), "alerts.critical", "", ""); status.Code(err) != codes.PermissionDenied {
		t.Fatalf("timeguru publish alerts.critical: %v, want PermissionDenied", err)
	}
	if err := w.publish(t, w.f.Bearer("timeguru", "wotan"), "tasks.created", "", ""); status.Code(err) != codes.PermissionDenied {
		t.Fatalf("timeguru publish tasks.created: %v, want PermissionDenied", err)
	}
	for _, room := range []string{"alerts.critical", "tasks.created"} {
		for _, m := range w.svc.memberManager.GetByRoom(room, member.StatusApproved) {
			if m.Name == "timeguru" {
				t.Errorf("a refused publish left timeguru a member of %s", room)
			}
		}
	}
}

// Without a permissions section nothing changes: the finding's behaviour,
// kept so a deployment without the section is not broken by this code.
func TestPermissions_GRPCNoSectionIsOpen(t *testing.T) {
	w := newAuthedWotan(t)
	w.svc.Permissions = nil
	if err := w.publish(t, w.f.Bearer("captain", "wotan"), "alerts.disk", "", ""); err != nil {
		t.Fatal(err)
	}
	if err := w.streamErr(t, "timeguru", "#"); err != nil {
		t.Fatalf("no section: # refused: %v", err)
	}
}
