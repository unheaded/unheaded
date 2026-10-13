// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

// Package well reads and writes compliance state in The Well (ADR-099
// amended): profiles and host bindings in unheaded_config. When The Well
// is unreachable, Open falls back to the repository YAML, labels the
// result yaml-fallback, and refuses every write: no DB, read-only.
package well

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode"

	"github.com/lib/pq"

	"unheaded/pkg/compliance/crosswalk"
	"unheaded/pkg/compliance/remediation"
	"unheaded/pkg/database"
)

// Source says where compliance state came from. Every reader shows it.
type Source string

const (
	SourceWell Source = "well"
	SourceYAML Source = "yaml-fallback"
)

var (
	// ErrReadOnlyFallback refuses writes while state comes from YAML.
	ErrReadOnlyFallback = errors.New("compliance state is read-only (YAML fallback): The Well is unavailable, and changes need it")
	// ErrInvalidChange wraps every rejected change.
	ErrInvalidChange = errors.New("invalid compliance change")

	errNotConfigured = errors.New("compliance store: The Well is not configured")
)

// Store is compliance desired state, from The Well or its YAML fallback.
type Store interface {
	Source() Source
	Model() *remediation.Model
	Profiles(ctx context.Context) (*remediation.ProfileSet, error)
	Bindings(ctx context.Context) ([]remediation.Binding, error)
	SetBinding(ctx context.Context, c Change) error
}

// Change binds a host to a profile and mode, recording who and why.
type Change struct {
	Host, Profile, Mode, Actor, Reason string
}

const (
	maxActor  = 100
	maxReason = 500
)

func printable(s string) bool {
	for _, r := range s {
		if !unicode.IsPrint(r) {
			return false
		}
	}
	return true
}

// Validate checks a change against the profiles, before any write.
func (c Change) Validate(ps *remediation.ProfileSet) error {
	if strings.TrimSpace(c.Actor) == "" || len(c.Actor) > maxActor || !printable(c.Actor) {
		return fmt.Errorf("%w: actor must be 1-%d printable characters", ErrInvalidChange, maxActor)
	}
	if strings.TrimSpace(c.Reason) == "" || len(c.Reason) > maxReason || !printable(c.Reason) {
		return fmt.Errorf("%w: reason must be 1-%d printable characters", ErrInvalidChange, maxReason)
	}
	b := remediation.Binding{Host: c.Host, Profile: c.Profile, Mode: c.Mode}
	if err := b.Validate(ps); err != nil {
		if errors.Is(err, remediation.ErrEnforcingGate) {
			return err
		}
		return fmt.Errorf("%w: %v", ErrInvalidChange, err)
	}
	return nil
}

// LoadModel reads and checks the repo-authored fix model against the
// catalog. Invalid repo data is an error, never a reason to fall back.
func LoadModel(repoDir string) (*remediation.Model, error) {
	root, err := os.OpenRoot(repoDir)
	if err != nil {
		return nil, err
	}
	defer func() { _ = root.Close() }()
	raw, err := root.ReadFile("compliance/remediation/fixes.yaml")
	if err != nil {
		return nil, err
	}
	m, err := remediation.Parse(raw)
	if err != nil {
		return nil, err
	}
	cat, err := crosswalk.LoadFS(root.FS(), "compliance/catalog/frameworks", "compliance/catalog/controls.yaml")
	if err != nil {
		return nil, err
	}
	if err := m.CheckCatalog(cat); err != nil {
		return nil, err
	}
	return m, nil
}

// ConfigFromEnv is the unheaded_config connection. The default user is
// the read-only role; a command that writes sets WELL_CONFIG_USER to
// config_admin. There is no default password (FND-007).
func ConfigFromEnv() database.Config {
	get := func(k, def string) string {
		if v, ok := os.LookupEnv(k); ok {
			return v
		}
		return def
	}
	return database.Config{
		Host:     get("WELL_HOST", "127.0.0.1"),
		Port:     get("WELL_PORT", "5432"),
		User:     get("WELL_CONFIG_USER", "config_reader"),
		Password: os.Getenv("WELL_CONFIG_PASSWORD"),
		DBName:   get("WELL_CONFIG_DB", "unheaded_config"),
		SSLMode:  get("WELL_SSLMODE", "disable"),
	}
}

// Open returns The Well's store, or the YAML fallback with the reason The
// Well could not be used. The error is for invalid repo data only.
func Open(ctx context.Context, repoDir string, cfg database.Config) (Store, string, error) {
	m, err := LoadModel(repoDir)
	if err != nil {
		return nil, "", fmt.Errorf("fix model: %w", err)
	}
	db, err := database.Connect(ctx, cfg)
	if err != nil {
		y, yerr := newYAMLStore(repoDir, m)
		if yerr != nil {
			return nil, "", yerr
		}
		return y, err.Error(), nil
	}
	return NewPGStore(db, m), "", nil
}

// --- YAML fallback ----------------------------------------------------------

type yamlStore struct {
	model    *remediation.Model
	profiles *remediation.ProfileSet
	bindings []remediation.Binding
}

// NewYAMLStore reads the seed/fallback files under repoDir.
func NewYAMLStore(repoDir string) (Store, error) {
	m, err := LoadModel(repoDir)
	if err != nil {
		return nil, err
	}
	return newYAMLStore(repoDir, m)
}

func newYAMLStore(repoDir string, m *remediation.Model) (*yamlStore, error) {
	root, err := os.OpenRoot(repoDir)
	if err != nil {
		return nil, err
	}
	defer func() { _ = root.Close() }()
	raw, err := root.ReadFile("compliance/profiles/profiles.yaml")
	if err != nil {
		return nil, err
	}
	ps, err := remediation.ParseProfiles(raw, m)
	if err != nil {
		return nil, err
	}
	s := &yamlStore{model: m, profiles: ps}
	for _, h := range remediation.KnownHosts {
		raw, err := root.ReadFile(filepath.Join("compliance/hosts", h+".yaml"))
		if err != nil {
			return nil, err
		}
		b, err := remediation.ParseBinding(raw, ps)
		if err != nil {
			return nil, err
		}
		s.bindings = append(s.bindings, *b)
	}
	return s, nil
}

func (s *yamlStore) Source() Source            { return SourceYAML }
func (s *yamlStore) Model() *remediation.Model { return s.model }
func (s *yamlStore) Profiles(context.Context) (*remediation.ProfileSet, error) {
	return s.profiles, nil
}
func (s *yamlStore) Bindings(context.Context) ([]remediation.Binding, error) {
	return append([]remediation.Binding(nil), s.bindings...), nil
}
func (s *yamlStore) SetBinding(context.Context, Change) error { return ErrReadOnlyFallback }

// --- The Well ---------------------------------------------------------------

type pgStore struct {
	db    *sql.DB
	model *remediation.Model
}

// NewPGStore is the store over an unheaded_config connection.
func NewPGStore(db *sql.DB, m *remediation.Model) Store { return &pgStore{db: db, model: m} }

func (s *pgStore) Source() Source            { return SourceWell }
func (s *pgStore) Model() *remediation.Model { return s.model }

func (s *pgStore) Profiles(ctx context.Context) (*remediation.ProfileSet, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT name, description, all_fixes, fixes FROM compliance_profiles ORDER BY name`)
	if err != nil {
		return nil, fmt.Errorf("profiles: %w", err)
	}
	defer rows.Close()
	var out []remediation.Profile
	for rows.Next() {
		var p remediation.Profile
		if err := rows.Scan(&p.Name, &p.Description, &p.All, pq.Array(&p.Fixes)); err != nil {
			return nil, fmt.Errorf("profiles: %w", err)
		}
		out = append(out, p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("profiles: %w", err)
	}
	// Rows pass the same validation as the YAML: a row the database
	// accepted but the model does not is an error, not data.
	return remediation.NewProfileSet(out, s.model)
}

func (s *pgStore) Bindings(ctx context.Context) ([]remediation.Binding, error) {
	ps, err := s.Profiles(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, `SELECT host, profile, mode FROM compliance_host_bindings ORDER BY host`)
	if err != nil {
		return nil, fmt.Errorf("bindings: %w", err)
	}
	defer rows.Close()
	var out []remediation.Binding
	for rows.Next() {
		var b remediation.Binding
		if err := rows.Scan(&b.Host, &b.Profile, &b.Mode); err != nil {
			return nil, fmt.Errorf("bindings: %w", err)
		}
		if err := b.Validate(ps); err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

// SetBinding validates the change, then writes it; the table's trigger
// records the history row (who, why, old and new) in the same statement.
func (s *pgStore) SetBinding(ctx context.Context, c Change) error {
	ps, err := s.Profiles(ctx)
	if err != nil {
		return err
	}
	if err := c.Validate(ps); err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `INSERT INTO compliance_host_bindings (host, profile, mode, updated_by, change_reason)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (host) DO UPDATE SET profile = EXCLUDED.profile, mode = EXCLUDED.mode,
			updated_by = EXCLUDED.updated_by, change_reason = EXCLUDED.change_reason`,
		c.Host, c.Profile, c.Mode, c.Actor, c.Reason)
	if err != nil {
		return fmt.Errorf("set binding: %w", err)
	}
	return nil
}

// pqArray adapts a string slice for a text[] parameter. A nil slice is
// sent as an empty array: pq.Array(nil) is NULL, which the NOT NULL
// columns refuse (profiles "none" and "all" have no fix list).
func pqArray(s []string) any {
	if s == nil {
		s = []string{}
	}
	return pq.Array(s)
}
