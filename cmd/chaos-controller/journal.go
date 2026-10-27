// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"time"

	"unheaded/pkg/database"
	"unheaded/pkg/statefile"
	"unheaded/services/yaldabaoth"
)

// Where the revert journals live (S-WELL-SPRING Phase 2, M-072): with
// WELL_DB set, The Well (unheaded_ops.chaos_journal, migration 039) as
// WELL_USER (default ops_writer); otherwise files under CHAOS_STATE_DIR.
// WELL_DB set and the Well unreachable (or 039 missing) is an error, never
// a quiet fall back to the file: a fault journaled in one place and looked
// for in the other would never be reverted.

// wellConn is an open Well connection for one host's journal.
type wellConn struct {
	store *database.ChaosStore
	host  string
	close func()
}

// openWell connects as the ops writer. The host is KINGDOM_NODE, else the
// hostname: each controller reads only its own host's entries.
var openWell = func(ctx context.Context, getenv func(string) string) (*wellConn, error) {
	cfg := database.OpsWriterConfig()
	cfg.DBName = getenv("WELL_DB")
	if v := getenv("WELL_HOST"); v != "" {
		cfg.Host = v
	}
	if v := getenv("WELL_PORT"); v != "" {
		cfg.Port = v
	}
	if v := getenv("WELL_USER"); v != "" {
		cfg.User = v
	}
	if v := getenv("WELL_PASSWORD"); v != "" {
		cfg.Password = v
	}
	if v := getenv("WELL_SSLMODE"); v != "" {
		cfg.SSLMode = v
	}
	db, err := database.Connect(ctx, cfg)
	if err != nil {
		return nil, err
	}
	host := getenv("KINGDOM_NODE")
	if host == "" {
		if host, err = os.Hostname(); err != nil {
			_ = db.Close()
			return nil, err
		}
	}
	s := database.NewChaosStore(db)
	if err := s.CheckSchema(ctx); err != nil {
		_ = db.Close()
		return nil, err
	}
	return &wellConn{store: s, host: host, close: func() { _ = db.Close() }}, nil
}

// experimentJournal returns the Well journal for Yaldabaoth when WELL_DB is
// set, nil (the file under CHAOS_STATE_DIR) otherwise.
func experimentJournal(ctx context.Context, getenv func(string) string) (yaldabaoth.JournalStore, func(), error) {
	if getenv("WELL_DB") == "" {
		return nil, func() {}, nil
	}
	w, err := openWell(ctx, getenv)
	if err != nil {
		return nil, func() {}, fmt.Errorf("chaos journal in the Well (WELL_DB=%s): %w", getenv("WELL_DB"), err)
	}
	j, err := yaldabaoth.NewWellJournalFromStore(w.store, w.host)
	if err != nil {
		w.close()
		return nil, func() {}, err
	}
	return j, w.close, nil
}

// --- packet-chaos's own journal ---

// packetJournalStore keeps packet-chaos's revert recipes.
type packetJournalStore interface {
	mode() string
	load(ctx context.Context) ([]packetJournalItem, error)
	add(ctx context.Context, it packetJournalItem) error
	remove(ctx context.Context, it packetJournalItem, why string) error
}

// filePacketJournal is CHAOS_STATE_DIR/packet-chaos.json. Entries written
// before ids existed are matched by their pin dir, as before.
type filePacketJournal struct {
	f *statefile.File
	j packetJournalFile
}

func (p *filePacketJournal) mode() string { return "file" }

func (p *filePacketJournal) load(context.Context) ([]packetJournalItem, error) {
	if _, err := p.f.Load(&p.j); err != nil {
		return nil, err
	}
	return append([]packetJournalItem(nil), p.j.Entries...), nil
}

func (p *filePacketJournal) add(_ context.Context, it packetJournalItem) error {
	next := p.j
	next.Version = 1
	next.Entries = append(append([]packetJournalItem(nil), p.j.Entries...), it)
	if err := p.f.Save(next); err != nil {
		return err
	}
	p.j = next
	return nil
}

func (p *filePacketJournal) remove(_ context.Context, it packetJournalItem, _ string) error {
	next := packetJournalFile{Version: 1}
	for _, e := range p.j.Entries {
		if e.Recipe["pin_dir"] != it.Recipe["pin_dir"] {
			next.Entries = append(next.Entries, e)
		}
	}
	if err := p.f.Save(next); err != nil {
		return err
	}
	p.j = next
	return nil
}

// wellPacketJournal is chaos_journal rows of kind 'packet' for this host.
type wellPacketJournal struct{ w *wellConn }

func (p *wellPacketJournal) mode() string { return "well" }

func (p *wellPacketJournal) load(ctx context.Context) ([]packetJournalItem, error) {
	rows, err := p.w.store.Open(ctx, p.w.host, database.ChaosKindPacket)
	if err != nil {
		return nil, err
	}
	out := make([]packetJournalItem, 0, len(rows))
	for _, r := range rows {
		it := packetJournalItem{ID: r.ID, Created: r.CreatedAt, Expires: r.ExpiresAt}
		if err := json.Unmarshal(r.Recipe, &it.Recipe); err != nil {
			return nil, fmt.Errorf("entry %s: recipe: %w", r.ID, err)
		}
		out = append(out, it)
	}
	return out, nil
}

func (p *wellPacketJournal) add(ctx context.Context, it packetJournalItem) error {
	recipe, err := json.Marshal(it.Recipe)
	if err != nil {
		return err
	}
	target, err := json.Marshal(yaldabaoth.ResolvedTarget{Kind: yaldabaoth.KindPacket, ID: it.Recipe["iface"]})
	if err != nil {
		return err
	}
	return p.w.store.Append(ctx, database.ChaosEntry{ID: it.ID, Host: p.w.host, Kind: database.ChaosKindPacket,
		Type: string(yaldabaoth.ExpTypePacketChaos), Target: target, Recipe: recipe,
		CreatedAt: it.Created, ExpiresAt: it.Expires})
}

func (p *wellPacketJournal) remove(ctx context.Context, it packetJournalItem, why string) error {
	if it.ID == "" {
		return errors.New("a Well journal entry without an id")
	}
	return p.w.store.MarkReverted(ctx, it.ID, why)
}

// openPacketJournal picks the store: the Well when WELL_DB is set, else
// the file in stateDir.
func openPacketJournal(ctx context.Context, stateDir string, getenv func(string) string) (packetJournalStore, func(), error) {
	if getenv("WELL_DB") == "" {
		return &filePacketJournal{f: statefile.In(stateDir, packetJournal)}, func() {}, nil
	}
	w, err := openWell(ctx, getenv)
	if err != nil {
		return nil, func() {}, fmt.Errorf("packet journal in the Well (WELL_DB=%s): %w", getenv("WELL_DB"), err)
	}
	pj := &wellPacketJournal{w: w}
	if err := movePacketFile(ctx, stateDir, pj); err != nil {
		w.close()
		return nil, func() {}, err
	}
	return pj, w.close, nil
}

// movePacketFile moves the file journal's entries into the Well when a
// host switches to it, so a fault journaled in the file is still reverted.
// The Well is written first, then the file emptied.
func movePacketFile(ctx context.Context, stateDir string, to packetJournalStore) error {
	f := statefile.In(stateDir, packetJournal)
	if f == nil {
		return nil
	}
	from := &filePacketJournal{f: f}
	items, err := from.load(ctx)
	if err != nil {
		return fmt.Errorf("packet journal: reading %s to move it into the Well: %w", packetJournal, err)
	}
	if len(items) == 0 {
		return nil
	}
	for i, it := range items {
		if it.ID == "" {
			it.ID = fmt.Sprintf("%s-%d", newPacketID(it.Created), i)
		}
		if err := to.add(ctx, it); err != nil && !errors.Is(err, database.ErrDuplicate) {
			return fmt.Errorf("packet journal: moving an entry into the Well: %w", err)
		}
	}
	if err := f.Save(packetJournalFile{Version: 1}); err != nil {
		return fmt.Errorf("packet journal: entries are in the Well, but %s could not be emptied: %w", packetJournal, err)
	}
	return nil
}

// newPacketID names a packet journal entry.
func newPacketID(now time.Time) string { return fmt.Sprintf("packet-%d", now.UnixNano()) }
