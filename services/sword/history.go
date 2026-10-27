// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package sword

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"unheaded/pkg/database"
	"unheaded/pkg/deploy/cuirassdriver"
	"unheaded/pkg/statefile"
)

// History is where Sword's deployment records live (S-WELL-SPRING Phase 2,
// M-072): The Well (unheaded_ops.deploy_history, migration 039) when
// WELL_DB is set, otherwise one JSON file under SWORD_DATA_DIR. Sword keeps
// the records in memory as well; every change is written through before it
// is reported. Record and Update get the whole in-memory history because
// the file store rewrites it; the Well store appends one row.
type History interface {
	// Mode is "well" or "file".
	Mode() string
	// Load returns the stored records, oldest first.
	Load(ctx context.Context) ([]*Deployment, error)
	// Record stores a new record.
	Record(ctx context.Context, d *Deployment, all []*Deployment) error
	// Update stores a change to d; from is its status before the change.
	Update(ctx context.Context, d *Deployment, from Status, all []*Deployment) error
	// Ready reports whether a write could succeed now.
	Ready(ctx context.Context) error
}

// --- file ---

type fileHistory struct {
	dir  string
	file *statefile.File
}

// NewFileHistory keeps the history in dir/sword.json (pkg/statefile). It
// returns nil for an empty dir: no history, so writes are refused.
func NewFileHistory(dir string) History {
	f := statefile.In(dir, stateFileName)
	if f == nil {
		return nil
	}
	return &fileHistory{dir: dir, file: f}
}

func (h *fileHistory) Mode() string { return "file" }

func (h *fileHistory) Load(context.Context) ([]*Deployment, error) {
	var st stateFile
	if _, err := h.file.Load(&st); err != nil {
		return nil, err
	}
	return st.Deployments, nil
}

func (h *fileHistory) save(all []*Deployment) error {
	return h.file.Save(stateFile{Version: 1, Deployments: all})
}

func (h *fileHistory) Record(_ context.Context, _ *Deployment, all []*Deployment) error {
	return h.save(all)
}

func (h *fileHistory) Update(_ context.Context, _ *Deployment, _ Status, all []*Deployment) error {
	return h.save(all)
}

func (h *fileHistory) Ready(context.Context) error { return probeWritable(h.dir) }

// --- The Well ---

type wellHistory struct {
	store *database.DeployStore
	host  string
}

// requestSpec is what a deployment asked for, the deploy_history.spec
// column: queried by people, never read back by Sword (the transitions'
// state is the record).
type requestSpec struct {
	Strategy StrategySpec              `json:"strategy"`
	Health   *cuirassdriver.HealthSpec `json:"health,omitempty"`
	Proxy    *cuirassdriver.ProxySpec  `json:"proxy,omitempty"`
	Labels   map[string]string         `json:"labels,omitempty"`
}

// NewWellHistory keeps the history in The Well as the connected role
// (ops_writer). host names this Sword's Kingdom node (KINGDOM_NODE): Load
// reads only that host's records, since a Sword deploys through its own
// host's Cuirass. It fails when migration 039 is missing.
func NewWellHistory(ctx context.Context, db *sql.DB, host string) (History, error) {
	if host == "" {
		return nil, errors.New("sword: the Well history needs a host name (KINGDOM_NODE)")
	}
	s := database.NewDeployStore(db)
	if err := s.CheckSchema(ctx); err != nil {
		return nil, err
	}
	return &wellHistory{store: s, host: host}, nil
}

func (h *wellHistory) Mode() string { return "well" }

func (h *wellHistory) Load(ctx context.Context) ([]*Deployment, error) {
	recs, err := h.store.List(ctx, h.host, "", maxHistory)
	if err != nil {
		return nil, err
	}
	out := make([]*Deployment, len(recs))
	for i, r := range recs {
		var d Deployment
		if err := json.Unmarshal(r.State, &d); err != nil {
			return nil, fmt.Errorf("deployment %s: stored state: %w", r.ID, err)
		}
		if d.ID != r.ID {
			return nil, fmt.Errorf("deployment %s: stored state names %q", r.ID, d.ID)
		}
		out[len(recs)-1-i] = &d // newest first from the store, oldest first here
	}
	return out, nil
}

func (h *wellHistory) Record(ctx context.Context, d *Deployment, _ []*Deployment) error {
	state, err := json.Marshal(d)
	if err != nil {
		return err
	}
	spec, err := json.Marshal(requestSpec{Strategy: d.Strategy, Health: d.Health, Proxy: d.Proxy, Labels: d.Labels})
	if err != nil {
		return err
	}
	return h.store.Record(ctx, database.DeployRecord{
		ID: d.ID, Service: d.Service, Version: d.Version, Strategy: string(d.Strategy.Type),
		Replicas: d.Replicas, Spec: spec, Host: h.host, CreatedAt: d.CreatedAt,
		Status: string(d.Status), State: state,
	})
}

func (h *wellHistory) Update(ctx context.Context, d *Deployment, from Status, _ []*Deployment) error {
	state, err := json.Marshal(d)
	if err != nil {
		return err
	}
	return h.store.Transition(ctx, d.ID, string(from), string(d.Status), d.Reason, state)
}

func (h *wellHistory) Ready(ctx context.Context) error { return h.store.Ping(ctx) }
