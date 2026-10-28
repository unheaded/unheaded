// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

package grpc

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"unheaded/services/wotan/internal/api"
	chatpb "unheaded/services/wotan/proto"
)

func writeReloadConfig(t *testing.T, path, body string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

func publishAs(svc *TopicService, name string) error {
	_, err := svc.PublishTopic(context.Background(), &chatpb.TopicPublishRequest{
		Topic: "alerts.critical", Payload: []byte("x"), Metadata: map[string]string{"display_name": name},
	})
	return err
}

// S-TRUE-WIRE Step 154: the topic service's allowlist follows a reload of
// the config it was started with (SIGHUP in cmd/wotan), and a malformed file
// leaves the running allowlist as it was.
func TestTopicServiceFollowsConfigReload(t *testing.T) {
	path := filepath.Join(t.TempDir(), "wotan.yaml")
	writeReloadConfig(t, path, "topics:\n  auto_approve: [old-svc]\n")
	cfg, err := api.LoadTopicConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	svc := productionTopicService()
	svc.AutoApprove = cfg

	if err := publishAs(svc, "old-svc"); err != nil {
		t.Fatalf("old-svc before reload: %v", err)
	}
	if err := publishAs(svc, "new-svc"); err == nil {
		t.Fatal("new-svc approved before reload")
	}

	writeReloadConfig(t, path, "topics:\n  auto_approve: [new-svc]\n")
	if _, err := cfg.Reload(path); err != nil {
		t.Fatalf("reload: %v", err)
	}
	if err := publishAs(svc, "new-svc"); err != nil {
		t.Fatalf("new-svc after reload: %v", err)
	}
	if err := publishAs(svc, "old-svc"); err == nil {
		t.Fatal("old-svc still approved after it was removed")
	}

	writeReloadConfig(t, path, ": :\n")
	if _, err := cfg.Reload(path); err == nil {
		t.Fatal("malformed config reloaded")
	}
	if err := publishAs(svc, "new-svc"); err != nil {
		t.Fatalf("a failed reload changed the allowlist: %v", err)
	}
}
