// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis.

package cmd

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"strconv"
	"strings"
	"time"

	"unheaded/cmd/unheaded-cli/output"
	"unheaded/pkg/auth"
)

// openAPIKeyStore opens the API-key store (ADR-103 §5): AUTH_APIKEY_STORE,
// else the Well when WELL_DB or AUTH_WELL_PASSWORD is set (as app_auth),
// else the file /etc/unheaded/auth/apikeys.jsonl. Tests replace it.
var openAPIKeyStore = func(ctx context.Context) (auth.APIKeyStore, string, error) {
	spec := auth.DefaultAPIKeyStoreSpec()
	s, err := auth.OpenAPIKeyStore(ctx, spec)
	return s, spec, err
}

// APIKeyListResult is the output of apikey list.
type APIKeyListResult struct {
	Keys []auth.APIKeyRecord `json:"keys"`
}

// WriteTable implements output.TableWriter.
func (r *APIKeyListResult) WriteTable(t *output.Table, wide bool) {
	if wide {
		t.SetHeaders("PREFIX", "OWNER", "ROLE", "STATE", "CREATED", "EXPIRES", "LAST USED", "ID")
	} else {
		t.SetHeaders("PREFIX", "OWNER", "ROLE", "STATE", "EXPIRES")
	}
	now := time.Now()
	for _, k := range r.Keys {
		state := "active"
		switch {
		case k.RevokedAt != nil:
			state = "revoked"
		case !now.Before(k.ExpiresAt):
			state = "expired"
		}
		expires := k.ExpiresAt.Format("2006-01-02")
		if wide {
			lastUsed := "-"
			if k.LastUsedAt != nil {
				lastUsed = formatAge(*k.LastUsedAt)
			}
			t.AddRow(k.Prefix, k.Owner, k.Role, state, k.CreatedAt.Format("2006-01-02"), expires, lastUsed, k.ID)
		} else {
			t.AddRow(k.Prefix, k.Owner, k.Role, state, expires)
		}
	}
}

// NewAPIKeyCommand creates the apikey command.
func NewAPIKeyCommand() *Command {
	cmd := &Command{
		Name:  "apikey",
		Short: "Create, list and revoke API keys (uhk_) for automation",
		Long: `API keys for scripts, CI and this CLI (ADR-103 §5).

A key is uhk_ plus 32 random bytes. It is printed once, by create; the store
keeps only its SHA-256, its first 12 characters (the prefix list and revoke
use), owner, role and expiry. Every key expires (default 90 days, at most
365). Services accept a key in X-API-Key or as "Authorization: Bearer uhk_..."
when they run with AUTH_APIKEY_STORE.

Store: AUTH_APIKEY_STORE ("well" or "file:<absolute path>"); unset, the Well
when WELL_DB or AUTH_WELL_PASSWORD is set (unheaded_app.api_keys as app_auth,
AUTH_WELL_PASSWORD), else ` + auth.DefaultAPIKeyFile + `.`,
		Aliases:     []string{"apikeys"},
		SubCommands: make(map[string]*Command),
	}
	cmd.AddCommand(newAPIKeyCreateCommand())
	cmd.AddCommand(newAPIKeyListCommand())
	cmd.AddCommand(newAPIKeyRevokeCommand())
	return cmd
}

func newAPIKeyCreateCommand() *Command {
	var owner, role, expires string
	flags := flag.NewFlagSet("create", flag.ContinueOnError)
	flags.StringVar(&owner, "owner", "", "Who holds the key (a person or a job), 1-128 of [A-Za-z0-9._@-]")
	flags.StringVar(&role, "role", auth.RoleService, "Role: "+strings.Join(auth.APIKeyRoles, ", "))
	flags.StringVar(&expires, "expires", "90d", "Lifetime (90d, 12h) or expiry date (2026-12-31); at most 365d")

	return &Command{
		Name:  "create",
		Short: "Create a key and print it once",
		Usage: "unheaded apikey create --owner <owner> [--role service] [--expires 90d]",
		Flags: flags,
		RunFunc: func(ctx *Context, args []string) error {
			if len(args) > 0 {
				return fmt.Errorf("unexpected argument %q", args[0])
			}
			if owner == "" {
				return errors.New("--owner is required")
			}
			expiresAt, err := parseKeyExpiry(expires, time.Now())
			if err != nil {
				return err
			}
			store, spec, err := openAPIKeyStore(context.Background())
			if err != nil {
				return fmt.Errorf("open API key store: %w", err)
			}
			key, rec, err := store.Create(context.Background(), owner, role, expiresAt)
			if err != nil {
				return err
			}
			_ = ctx.Output.WriteStringln("store this now; it is not shown again:")
			_ = ctx.Output.WriteStringln(key)
			_ = ctx.Output.WriteStringln(fmt.Sprintf("prefix %s  owner %s  role %s  expires %s  id %s  (store %s)",
				rec.Prefix, rec.Owner, rec.Role, rec.ExpiresAt.Format(time.RFC3339), rec.ID, spec))
			return nil
		},
	}
}

func newAPIKeyListCommand() *Command {
	return &Command{
		Name:    "list",
		Short:   "List keys (prefix, owner, role, state; never the key)",
		Usage:   "unheaded apikey list [-o wide|json|yaml]",
		Aliases: []string{"ls"},
		RunFunc: func(ctx *Context, args []string) error {
			store, _, err := openAPIKeyStore(context.Background())
			if err != nil {
				return fmt.Errorf("open API key store: %w", err)
			}
			keys, err := store.List(context.Background())
			if err != nil {
				return err
			}
			if keys == nil {
				keys = []auth.APIKeyRecord{}
			}
			return ctx.Output.Write(&APIKeyListResult{Keys: keys})
		},
	}
}

func newAPIKeyRevokeCommand() *Command {
	return &Command{
		Name:  "revoke",
		Short: "Revoke a key by its prefix (uhk_ + 8) or id",
		Usage: "unheaded apikey revoke <prefix|id>",
		RunFunc: func(ctx *Context, args []string) error {
			if len(args) != 1 {
				return errors.New("revoke takes one prefix or id (see unheaded apikey list)")
			}
			store, _, err := openAPIKeyStore(context.Background())
			if err != nil {
				return fmt.Errorf("open API key store: %w", err)
			}
			rec, err := store.Revoke(context.Background(), args[0])
			if err != nil {
				return err
			}
			ctx.Output.WriteSuccess("revoked %s (owner %s, role %s) at %s", rec.Prefix, rec.Owner, rec.Role, rec.RevokedAt.Format(time.RFC3339))
			return nil
		},
	}
}

// parseKeyExpiry reads --expires: a positive lifetime in days ("90d") or
// as a Go duration ("12h"), or an expiry date ("2026-12-31", UTC midnight).
func parseKeyExpiry(s string, now time.Time) (time.Time, error) {
	if t, err := time.Parse("2006-01-02", s); err == nil {
		return t, nil
	}
	var d time.Duration
	if days, ok := strings.CutSuffix(s, "d"); ok {
		n, err := strconv.Atoi(days)
		if err != nil || n > 3660 {
			return time.Time{}, fmt.Errorf("--expires %q: want e.g. 90d, 12h or 2026-12-31", s)
		}
		d = time.Duration(n) * 24 * time.Hour
	} else {
		var err error
		if d, err = time.ParseDuration(s); err != nil {
			return time.Time{}, fmt.Errorf("--expires %q: want e.g. 90d, 12h or 2026-12-31", s)
		}
	}
	if d <= 0 {
		return time.Time{}, fmt.Errorf("--expires %q: must be in the future", s)
	}
	return now.Add(d), nil
}
