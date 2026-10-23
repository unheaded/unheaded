// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

package main

import (
	"encoding/json"
	"os"
	"sort"
	"testing"
)

// genesisCard is one card of db/seed/kanban-genesis.json, in a fixed
// field order.
type genesisCard struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Status      string `json:"status"`
	Type        string `json:"type"`
	Owner       string `json:"owner"`
	Progress    int    `json:"progress"`
	CreatedAt   string `json:"created_at"`
	UpdatedAt   string `json:"updated_at"`
}

// TestDumpGenesis writes getInitialTasks() as JSON, sorted by id, when
// KANBAN_GENESIS_DUMP names a file (S-TWO-BRAINS Step 416). The
// timestamps it writes are placeholders; scripts/kanban/genesis-timestamps.py
// replaces them with dates from git.
func TestDumpGenesis(t *testing.T) {
	path := os.Getenv("KANBAN_GENESIS_DUMP")
	if path == "" {
		t.Skip("KANBAN_GENESIS_DUMP unset")
	}
	tasks := getInitialTasks()
	cards := make([]genesisCard, 0, len(tasks))
	for _, x := range tasks {
		cards = append(cards, genesisCard{ID: x.ID, Title: x.Title, Description: x.Description, Status: x.Status,
			Type: x.Type, Owner: x.Owner, Progress: x.Progress, CreatedAt: "", UpdatedAt: ""})
	}
	sort.Slice(cards, func(i, j int) bool { return cards[i].ID < cards[j].ID })
	b, err := json.MarshalIndent(cards, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, append(b, '\n'), 0o600); err != nil {
		t.Fatal(err)
	}
}
