// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package main

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	_ "github.com/lib/pq"

	"unheaded/pkg/database"
	"unheaded/pkg/logger"
	"unheaded/services/yaldabaoth/packet"
)

// withTestWell points openWell at WELL_TEST_DSN (a throwaway unheaded_ops
// with migration 039, as ops_writer: scripts/well/test-deploy-journal.sh)
// under a host name of its own, or skips. It returns the connection, the
// host, and a getenv that sets WELL_DB.
func withTestWell(t *testing.T) (*sql.DB, string, func(string) string) {
	t.Helper()
	dsn := os.Getenv("WELL_TEST_DSN")
	if dsn == "" {
		t.Skip("WELL_TEST_DSN not set (scripts/well/test-deploy-journal.sh sets it)")
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	host := fmt.Sprintf("t%d", time.Now().UnixNano())
	old := openWell
	openWell = func(ctx context.Context, _ func(string) string) (*wellConn, error) {
		s := database.NewChaosStore(db)
		if err := s.CheckSchema(ctx); err != nil {
			return nil, err
		}
		return &wellConn{store: s, host: host, close: func() {}}, nil
	}
	t.Cleanup(func() { openWell = old })
	return db, host, func(k string) string {
		if k == "WELL_DB" {
			return "unheaded_ops"
		}
		return ""
	}
}

func countRows(t *testing.T, db *sql.DB, q string, args ...any) int {
	t.Helper()
	var n int
	if err := db.QueryRow(q, args...).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

// revert-all on the Well: three open faults, three 'reverted' rows, the
// journal rows untouched, and a second run finds nothing.
func TestRevertAllWell(t *testing.T) {
	db, host, getenv := withTestWell(t)
	ctx := context.Background()
	st := database.NewChaosStore(db)
	inj := &testInjector{active: map[string]bool{}}
	var ids []string
	for i := 0; i < 3; i++ {
		id := fmt.Sprintf("chaos-left-%d-%s", i, host)
		fault := fmt.Sprintf("steel-echo-v2-%d", i)
		inj.active[fault] = true
		if err := st.Append(ctx, database.ChaosEntry{ID: id, Host: host, Kind: database.ChaosKindExperiment, Type: "network_delay",
			Target: json.RawMessage(`{"kind":"lxd","id":"` + fault + `"}`), Recipe: json.RawMessage(`{"fault":"` + fault + `"}`),
			CreatedAt: time.Now().UTC().Add(-time.Hour), ExpiresAt: time.Now().UTC().Add(time.Hour)}); err != nil {
			t.Fatal(err)
		}
		ids = append(ids, id)
	}
	c := lab(t)
	c.getenv = getenv
	var out bytes.Buffer
	// No injector for the type: nothing can be reverted, nothing is closed.
	if code := revertAll(c, &out); code != 1 {
		t.Fatalf("revert-all with no injector = %d (%s)", code, out.String())
	}
	if n := countRows(t, db, `SELECT count(*) FROM chaos_journal_transitions WHERE entry_id = ANY($1)`, pqArray(ids)); n != 0 {
		t.Fatalf("an unreverted fault was closed: %d rows", n)
	}
	withInjector(t, inj)
	out.Reset()
	if code := revertAll(c, &out); code != 0 || inj.count() != 0 {
		t.Fatalf("revert-all = %d (%s), faults %d", code, out.String(), inj.count())
	}
	if n := countRows(t, db, `SELECT count(*) FROM chaos_journal_transitions WHERE entry_id = ANY($1) AND to_status = 'reverted' AND note = 'revert-all'`, pqArray(ids)); n != 3 {
		t.Fatalf("reverted rows: %d, want 3", n)
	}
	if n := countRows(t, db, `SELECT count(*) FROM chaos_journal WHERE entry_id = ANY($1)`, pqArray(ids)); n != 3 {
		t.Fatalf("journal rows: %d, want 3 (untouched)", n)
	}
	if code := revertAll(c, &out); code != 0 {
		t.Fatalf("second revert-all = %d", code)
	}
}

// WELL_DB set and the Well not there: revert-all says so and exits 2
// rather than reading an empty file journal and calling it done.
func TestRevertAllWellUnreachable(t *testing.T) {
	old := openWell
	openWell = func(context.Context, func(string) string) (*wellConn, error) {
		return nil, fmt.Errorf("dial tcp: connection refused")
	}
	t.Cleanup(func() { openWell = old })
	c := lab(t)
	c.getenv = func(k string) string { return map[string]string{"WELL_DB": "unheaded_ops"}[k] }
	var out bytes.Buffer
	if code := revertAll(c, &out); code != 2 || !strings.Contains(out.String(), "WELL_DB") {
		t.Fatalf("revert-all with the Well down = %d (%s)", code, out.String())
	}
	if _, err := build(c, logger.New(io.Discard)); err == nil {
		t.Fatal("the controller started with WELL_DB set and the Well down")
	}
}

// packet-chaos on the Well: a leftover file entry is moved in and reverted
// first, the run's own fault is a 'packet' row closed after its revert.
func TestPacketChaosWell(t *testing.T) {
	db, host, getenv := withTestWell(t)
	be := newRecBackend()
	dir := t.TempDir()
	pin := packet.DefaultPinRoot + "/0123456789abcdef"
	be.pinned[pin], be.linked[pin], be.qdisc["steel0-h"] = true, true, true
	j := packetJournalFile{Version: 1, Entries: []packetJournalItem{{Created: time.Now().UTC(), Expires: time.Now().UTC().Add(time.Minute),
		Recipe: map[string]string{"iface": "steel0-h", "pin_dir": pin, "flow_labels": "0x5ee1", "mode": "marker", "netem": "loss 50%", "qdisc": "c4a0:"}}}}
	raw, _ := json.Marshal(j)
	if err := os.WriteFile(filepath.Join(dir, packetJournal), raw, 0o600); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	code := runPacketChaos(context.Background(), []string{"--iface", "steel0-h", "--flow-label", "0x5EE2", "--duration", "10ms",
		"--state-dir", dir, "--abort-file", filepath.Join(dir, "none")}, &out, be, getenv)
	if code != 0 {
		t.Fatalf("exit %d: %s", code, out.String())
	}
	s := out.String()
	if !strings.Contains(s, "journal: well") || !strings.Contains(s, "0x5ee1 on steel0-h: reverted") || !strings.Contains(s, "reverted and verified gone") {
		t.Fatalf("output:\n%s", s)
	}
	if n := countRows(t, db, `SELECT count(*) FROM chaos_journal WHERE host = $1 AND kind = 'packet'`, host); n != 2 {
		t.Fatalf("packet rows: %d, want 2 (the leftover and this run)", n)
	}
	if n := countRows(t, db, `SELECT count(*) FROM chaos_journal_open WHERE host = $1`, host); n != 0 {
		t.Fatalf("open packet rows after the run: %d", n)
	}
	raw, _ = os.ReadFile(filepath.Join(dir, packetJournal))
	var after packetJournalFile
	if err := json.Unmarshal(raw, &after); err != nil || len(after.Entries) != 0 {
		t.Fatalf("file journal after the move: %s (%v)", raw, err)
	}
}

func pqArray(s []string) any { return "{" + strings.Join(s, ",") + "}" }
