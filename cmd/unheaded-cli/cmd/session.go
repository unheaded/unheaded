// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis.

package cmd

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"strings"
	"time"

	"unheaded/cmd/unheaded-cli/output"
	"unheaded/pkg/bff/sessions"
)

// openSessionAdmin opens the Well's browser sessions for listing and
// revoking (ADR-114 N5): unheaded_app.bff_sessions as app_auth, the login
// `unheaded apikey` uses (AUTH_WELL_PASSWORD, WELL_HOST, ...). Tests
// replace it.
var openSessionAdmin = func(ctx context.Context) (sessions.Admin, error) {
	return sessions.OpenWellAdmin(ctx)
}

// SessionRow is one session as the command line shows it. ID is the first
// 16 characters of the SHA-256 the session is stored under: enough to name
// it, and nothing a browser could present.
type SessionRow struct {
	ID                string     `json:"id" yaml:"id"`
	Subject           string     `json:"subject" yaml:"subject"`
	Front             string     `json:"front" yaml:"front"`
	Roles             []string   `json:"roles" yaml:"roles"`
	State             string     `json:"state" yaml:"state"`
	CreatedAt         time.Time  `json:"created_at" yaml:"created_at"`
	LastUsedAt        time.Time  `json:"last_used_at" yaml:"last_used_at"`
	ExpiresAt         time.Time  `json:"expires_at" yaml:"expires_at"`
	AbsoluteExpiresAt time.Time  `json:"absolute_expires_at" yaml:"absolute_expires_at"`
	RevokedAt         *time.Time `json:"revoked_at,omitempty" yaml:"revoked_at,omitempty"`
}

// SessionListResult is the output of session list.
type SessionListResult struct {
	Sessions []SessionRow `json:"sessions" yaml:"sessions"`
}

func sessionRow(s sessions.Session, now time.Time) SessionRow {
	roles := s.Roles
	if roles == nil {
		roles = []string{}
	}
	return SessionRow{
		ID: s.ID(), Subject: s.Subject, Front: s.Front, Roles: roles, State: s.State(now),
		CreatedAt: s.CreatedAt, LastUsedAt: s.LastUsedAt, ExpiresAt: s.ExpiresAt,
		AbsoluteExpiresAt: s.AbsoluteExpiresAt, RevokedAt: s.RevokedAt,
	}
}

// WriteTable implements output.TableWriter.
func (r *SessionListResult) WriteTable(t *output.Table, wide bool) {
	if wide {
		t.SetHeaders("ID", "SUBJECT", "FRONT", "STATE", "LAST USED", "EXPIRES", "CREATED", "MAX AGE ENDS", "ROLES")
	} else {
		t.SetHeaders("ID", "SUBJECT", "FRONT", "STATE", "LAST USED", "EXPIRES")
	}
	const stamp = "2006-01-02 15:04Z"
	for _, s := range r.Sessions {
		expires := s.ExpiresAt.UTC().Format(stamp)
		if wide {
			t.AddRow(s.ID, s.Subject, s.Front, s.State, formatAge(s.LastUsedAt), expires,
				s.CreatedAt.UTC().Format(stamp), s.AbsoluteExpiresAt.UTC().Format(stamp), strings.Join(s.Roles, ","))
		} else {
			t.AddRow(s.ID, s.Subject, s.Front, s.State, formatAge(s.LastUsedAt), expires)
		}
	}
}

// NewSessionCommand creates the session command.
func NewSessionCommand() *Command {
	cmd := &Command{
		Name:  "session",
		Short: "List and revoke browser login sessions kept in the Well",
		Long: `Browser sessions of the login fronts (dashboard, board, wiki), for a front
that runs with AUTH_SESSION_STORE=well (ADR-114 N5).

A session is who logged in to which front, when it was last used and when it
ends. The Well keeps it under the SHA-256 of the browser's cookie; the ID
shown here is the first 16 characters of that hash. Neither can be turned
into the cookie. A revoked session is refused on its next request.

Store: the Well, unheaded_app.bff_sessions, as app_auth (AUTH_WELL_PASSWORD,
with WELL_HOST, WELL_PORT, WELL_SSLMODE and WELL_SSLROOTCERT as for apikey).
A front that keeps sessions in its own memory (AUTH_SESSION_STORE unset or
dev-memory) has nothing here: restart it to end its sessions.`,
		Aliases:     []string{"sessions"},
		SubCommands: make(map[string]*Command),
	}
	cmd.AddCommand(newSessionListCommand())
	cmd.AddCommand(newSessionRevokeCommand())
	cmd.AddCommand(newSessionRevokeAllCommand())
	return cmd
}

func newSessionListCommand() *Command {
	var subject, front string
	var all bool
	flags := flag.NewFlagSet("list", flag.ContinueOnError)
	flags.StringVar(&subject, "subject", "", "Only this user's sessions")
	flags.StringVar(&front, "front", "", "Only this front's sessions (dashboard-backend, kanban-app, wiki-server)")
	flags.BoolVar(&all, "all", false, "Also expired and revoked sessions not yet swept")

	return &Command{
		Name:    "list",
		Short:   "List sessions (id, subject, front, state; never a cookie)",
		Usage:   "unheaded session list [--subject <user>] [--front <front>] [--all] [-o wide|json|yaml]",
		Aliases: []string{"ls"},
		Flags:   flags,
		RunFunc: func(ctx *Context, args []string) error {
			if len(args) > 0 {
				return fmt.Errorf("unexpected argument %q", args[0])
			}
			admin, err := openSessionAdmin(context.Background())
			if err != nil {
				return fmt.Errorf("open session store: %w", err)
			}
			defer admin.Close()
			now := time.Now()
			list, err := admin.List(context.Background(), sessions.Filter{Subject: subject, Front: front, Ended: all}, now)
			if err != nil {
				return err
			}
			rows := make([]SessionRow, 0, len(list))
			for _, s := range list {
				rows = append(rows, sessionRow(s, now))
			}
			if len(list) == sessions.ListLimit {
				ctx.Output.WriteWarning("showing the newest %d sessions; narrow with --subject or --front", sessions.ListLimit)
			}
			return ctx.Output.Write(&SessionListResult{Sessions: rows})
		},
	}
}

func newSessionRevokeCommand() *Command {
	return &Command{
		Name:  "revoke",
		Short: "Revoke one session by its id",
		Usage: "unheaded session revoke <id>",
		RunFunc: func(ctx *Context, args []string) error {
			if len(args) != 1 {
				return errors.New("revoke takes one session id (see unheaded session list)")
			}
			admin, err := openSessionAdmin(context.Background())
			if err != nil {
				return fmt.Errorf("open session store: %w", err)
			}
			defer admin.Close()
			s, err := admin.Revoke(context.Background(), strings.ToLower(args[0]), time.Now())
			if err != nil {
				if errors.Is(err, sessions.ErrAmbiguous) {
					return fmt.Errorf("%w (or end every session of the user: unheaded session revoke-all --subject <user>)", err)
				}
				return err
			}
			at := time.Now()
			if s.RevokedAt != nil {
				at = *s.RevokedAt
			}
			ctx.Output.WriteSuccess("revoked session %s (subject %s, front %s) at %s; its next request is refused",
				s.ID(), s.Subject, s.Front, at.UTC().Format(time.RFC3339))
			return nil
		},
	}
}

func newSessionRevokeAllCommand() *Command {
	var subject, front string
	flags := flag.NewFlagSet("revoke-all", flag.ContinueOnError)
	flags.StringVar(&subject, "subject", "", "The user whose sessions end (required)")
	flags.StringVar(&front, "front", "", "Only on this front (default: every front)")

	return &Command{
		Name:  "revoke-all",
		Short: "Revoke every session of one user",
		Usage: "unheaded session revoke-all --subject <user> [--front <front>]",
		Flags: flags,
		RunFunc: func(ctx *Context, args []string) error {
			if len(args) > 0 {
				return fmt.Errorf("unexpected argument %q", args[0])
			}
			// Never "everyone": the subject is required and must be one.
			if !sessions.ValidSubject(subject) {
				return errors.New("--subject is required: the user whose sessions end")
			}
			if front != "" && !sessions.ValidFront(front) {
				return fmt.Errorf("--front %q is not a front name", front)
			}
			admin, err := openSessionAdmin(context.Background())
			if err != nil {
				return fmt.Errorf("open session store: %w", err)
			}
			defer admin.Close()
			n, err := admin.RevokeSubject(context.Background(), subject, front, time.Now())
			if err != nil {
				return err
			}
			where := "every front"
			if front != "" {
				where = front
			}
			noun := "sessions"
			if n == 1 {
				noun = "session"
			}
			ctx.Output.WriteSuccess("revoked %d %s of %s on %s", n, noun, subject, where)
			return nil
		},
	}
}
