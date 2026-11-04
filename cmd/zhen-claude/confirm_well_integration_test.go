// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

//go:build wellint

package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	_ "github.com/lib/pq"

	"unheaded/pkg/champion"
	"unheaded/pkg/claudebridge"
)

func TestAdv_QueueRoundTripThroughPostgres(t *testing.T) {
	if os.Getenv("WELL_TEST_APP_DSN") == "" {
		t.Skip("WELL_TEST_APP_DSN unset (run scripts/well/test-zhen-confirm.sh)")
	}
	typedTTY(t, true)
	db, err := sql.Open("postgres", os.Getenv("WELL_TEST_APP_DSN"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx := context.Background()
	q := claudebridge.SQLConfirmQueue{DB: db}
	root := "/var/zhen/projects/default"
	champ, err := champion.New(champion.Config{ProjectRoot: root}, nil)
	if err != nil {
		t.Fatal(err)
	}
	cp, err := champion.NewToolProvider(champ, "zhen-agent")
	if err != nil {
		t.Fatal(err)
	}
	qc := &claudebridge.QueueConfirmer{Champion: champ, Queue: q, Source: "zhen-claude"}

	taught := filepath.Join(t.TempDir(), "zhen-taught")
	if err := os.Mkdir(taught, 0o750); err != nil {
		t.Fatal(err)
	}
	t.Setenv(envTaughtDir, taught)
	allowed := map[string]bool{root: true}

	// Bytes JSONB could reshape: quotes, escapes, html, unicode, CRLF, tabs.
	text := "line \"one\" <b>&amp;</b>\tcafé 日本語 \\n literal\r\nsecond line \U0001F600"
	out := qc.Confirm(ctx, claudebridge.ConfirmRequest{Conversation: "s", Tool: "teach_write",
		Args: map[string]any{"title": "Round Trip", "text": text}, EmittedBy: "zhen-agent", Reason: "r", Provider: cp})
	if out.Decision != "queued" {
		t.Fatalf("not queued: %+v", out)
	}
	id := out.QueueID

	var so, se bytes.Buffer
	if rc := confirmApprove(ctx, q, id, allowed, &so, &se); rc != 0 {
		t.Fatalf("approve rc=%d: %s", rc, se.String())
	}
	ents, _ := os.ReadDir(taught)
	if len(ents) != 1 {
		t.Fatalf("files: %v", ents)
	}
	p := filepath.Join(taught, ents[0].Name())
	st, _ := os.Stat(p)
	data, _ := os.ReadFile(p)
	sum := sha256.Sum256([]byte(text))
	if st.Mode().Perm() != 0o640 || !strings.Contains(string(data), text) || !strings.Contains(string(data), hex.EncodeToString(sum[:])) {
		t.Fatalf("mode %v, data %q", st.Mode().Perm(), data)
	}
	t.Logf("wrote %s mode %v", ents[0].Name(), st.Mode().Perm())
	row, _ := q.Get(ctx, id)
	if row.Status != "executed" {
		t.Fatalf("status %s", row.Status)
	}

	// Replay.
	so.Reset()
	se.Reset()
	if rc := confirmApprove(ctx, q, id, allowed, &so, &se); rc == 0 {
		t.Fatal("replay approved")
	}
	t.Logf("replay: %s", strings.TrimSpace(se.String()))

	// Row mutation by the role the daemon holds.
	if _, err := db.Exec(`UPDATE zhen_pending_confirmations SET call = '{"title":"x","text":"evil"}' WHERE id = $1`, id); err == nil {
		t.Fatal("app_zhen changed call")
	} else {
		t.Logf("UPDATE call: %v", err)
	}
	if _, err := db.Exec(`UPDATE zhen_pending_confirmations SET status = 'pending', decided_at = NULL, decided_by = NULL WHERE id = $1`, id); err == nil {
		t.Fatal("executed went back to pending")
	} else {
		t.Logf("reset status: %v", err)
	}

	// A hand-written row with a self-consistent hash, a secret and a forged
	// summary: the role can INSERT, so the body must be what refuses it.
	insert := func(id, title, txt string) {
		t.Helper()
		args := map[string]any{"title": title, "text": txt}
		_, h, err := champion.CanonicalCall("teach_write", args, root)
		if err != nil {
			t.Fatal(err)
		}
		b, _ := json.Marshal(args)
		now := time.Now().UTC()
		if err := q.Enqueue(ctx, claudebridge.QueuedCall{ID: id, CreatedAt: now, ExpiresAt: now.Add(time.Hour), SourceService: "zhen-claude",
			EmittedBy: "zhen-agent", ProjectRoot: root, Tool: "teach_write", Call: b, CallSHA256: h, Summary: "harmless note"}); err != nil {
			t.Fatal(err)
		}
	}
	insert("00000000000000a1", "note", "password=hunter2hunter2")
	so.Reset()
	se.Reset()
	if rc := confirmApprove(ctx, q, "00000000000000a1", allowed, &so, &se); rc == 0 {
		t.Fatal("secret row approved")
	}
	if strings.Contains(so.String()+se.String(), "hunter2") {
		t.Fatalf("secret printed: %s %s", so.String(), se.String())
	}
	r2, _ := q.Get(ctx, "00000000000000a1")
	t.Logf("secret row: status %s; files %d", r2.Status, func() int { e, _ := os.ReadDir(taught); return len(e) }())

	// No env: row stays pending.
	t.Setenv(envTaughtDir, "")
	insert("00000000000000a2", "second", "body")
	so.Reset()
	se.Reset()
	if rc := confirmApprove(ctx, q, "00000000000000a2", allowed, &so, &se); rc == 0 {
		t.Fatal("approved with no taught dir")
	}
	r3, _ := q.Get(ctx, "00000000000000a2")
	if r3.Status != "pending" {
		t.Fatalf("status %s", r3.Status)
	}
	t.Setenv(envTaughtDir, taught)

	// Wrong root on the allowlist: refused, stays pending.
	so.Reset()
	se.Reset()
	if rc := confirmApprove(ctx, q, "00000000000000a2", map[string]bool{"/other": true}, &so, &se); rc == 0 {
		t.Fatal("approved under a root not allowed")
	}
	t.Logf("wrong root: %s", strings.TrimSpace(se.String()))

	// A queued write_file aimed at the taught dir, approved as Stevie.
	wf := map[string]any{"path": filepath.Join(taught, "evil.md"), "content": "evil"}
	_, h, _ := champion.CanonicalCall("write_file", wf, root)
	b, _ := json.Marshal(wf)
	now := time.Now().UTC()
	if err := q.Enqueue(ctx, claudebridge.QueuedCall{ID: "00000000000000a3", CreatedAt: now, ExpiresAt: now.Add(time.Hour), SourceService: "zhen-claude",
		EmittedBy: "zhen-agent", ProjectRoot: root, Tool: "write_file", Call: b, CallSHA256: h, Summary: "x"}); err != nil {
		t.Fatal(err)
	}
	so.Reset()
	se.Reset()
	rc := confirmApprove(ctx, q, "00000000000000a3", allowed, &so, &se)
	t.Logf("write_file row rc=%d: %s", rc, strings.TrimSpace(se.String()))
	if _, err := os.Stat(filepath.Join(taught, "evil.md")); err == nil {
		t.Fatal("queued write_file reached the taught dir")
	}
}
