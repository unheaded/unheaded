// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

// well-migrate brings an existing Well up to db/migrations/MANIFEST and
// records every file it runs in that database's well_schema_migrations
// ledger (ADR-101). db/init.sh does the same on an empty volume.
//
//	well-migrate check                 # manifest and files only, no database
//	well-migrate status                # ledger vs manifest, per database
//	well-migrate apply                 # print what would run
//	well-migrate apply -yes            # run it, in order, stop at the first failure
//	well-migrate apply -db unheaded_ops -yes   # one database only (also status, baseline)
//	well-migrate baseline -through 012_huginn_reader.sql -yes
//	                                   # record pending files up to that entry WITHOUT running them
//
// baseline is for a Well whose schema already matches the manifest up to a
// point (shown by a shape diff against a bootstrap of those files), so the
// ledger can start there; apply then runs the rest. -through is required:
// what the Well already has is a decision, never a default.
//
// Connection: WELL_HOST (127.0.0.1), WELL_PORT (5432), WELL_SSLMODE
// (require: The Well accepts TLS only; allow and prefer are refused, and
// disable is accepted only for this host), WELL_MIGRATE_USER (unheaded, the superuser) and
// WELL_MIGRATE_PASSWORD (required; no default, FND-007).
//
// Exit: 0 clean, 1 error or drift (an applied file changed, an orphan
// ledger row, a gap), 2 usage, 3 pending migrations (status only).
package main

import (
	"context"
	"database/sql"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"time"

	_ "github.com/lib/pq"

	"unheaded/pkg/cmdline"
	"unheaded/pkg/database"
	"unheaded/pkg/database/migrate"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	code := run(ctx, os.Args[1:], os.Stdout, os.Stderr)
	stop()
	os.Exit(code)
}

func run(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	fl := flag.NewFlagSet("well-migrate", flag.ContinueOnError)
	fl.SetOutput(io.Discard)
	spec := cmdline.Spec{
		Name:     "well-migrate",
		Purpose:  "Apply the Well's SQL migrations from the manifest, with a ledger (ADR-101).",
		Operands: "check | status | apply [-db D] [-yes] | baseline [-db D] -through FILE -yes",
	}
	if len(args) == 1 && cmdline.IsVersionArg(args[0]) {
		cmdline.PrintVersion(stdout, spec)
		return 0
	}
	dir := fl.String("dir", "db/migrations", "migrations directory (holds MANIFEST and LEDGER.sql)")
	lockTimeout := fl.Duration("lock-timeout", 5*time.Second, "lock wait limit inside each migration")
	stmtTimeout := fl.Duration("statement-timeout", 5*time.Minute, "limit per statement")
	usage := func() int {
		fmt.Fprintln(stderr, "usage: well-migrate [-dir D] [-lock-timeout T] [-statement-timeout T] check | status | apply [-db D] [-yes] | baseline [-db D] -through FILE -yes")
		fmt.Fprintln(stderr, "Try 'well-migrate --help' for more information.")
		return 2
	}
	if err := fl.Parse(args); err != nil {
		return cmdline.HandleParseError(err, fl, spec, args, stdout, stderr)
	}
	rest := fl.Args()
	if len(rest) == 0 {
		return usage()
	}
	cmd := rest[0]
	sub := flag.NewFlagSet(cmd, flag.ContinueOnError)
	sub.SetOutput(stderr)
	yes := sub.Bool("yes", false, "make the change (default: print it)")
	only := sub.String("db", "", "limit to one database (status, apply, baseline)")
	through := ""
	if cmd == "baseline" {
		sub.StringVar(&through, "through", "", "last manifest file the Well already has (required)")
	}
	if err := sub.Parse(rest[1:]); err != nil || sub.NArg() != 0 {
		return usage()
	}
	fail := func(err error) int {
		fmt.Fprintln(stderr, "well-migrate:", err)
		return 1
	}

	set, err := migrate.Load(*dir)
	if err != nil {
		return fail(err)
	}
	if cmd == "check" {
		fmt.Fprintf(stdout, "%d entries over %d databases; every numbered file routed or skipped; no psql meta-commands or transaction control\n",
			len(set.Files), len(set.Databases()))
		return 0
	}
	if cmd != "status" && cmd != "apply" && cmd != "baseline" {
		return usage()
	}
	if cmd == "baseline" && (!*yes || through == "") {
		fmt.Fprintln(stderr, "well-migrate: baseline records files as applied without running them; it needs -through FILE and -yes")
		return 2
	}

	cfg := database.Config{
		Host:     env("WELL_HOST", "127.0.0.1"),
		Port:     env("WELL_PORT", "5432"),
		User:     env("WELL_MIGRATE_USER", "unheaded"),
		Password: os.Getenv("WELL_MIGRATE_PASSWORD"),
		SSLMode:  env("WELL_SSLMODE", "require"),
	}
	if cfg.Password == "" {
		return fail(fmt.Errorf("WELL_MIGRATE_PASSWORD is not set: %w", database.ErrMissingCredentials))
	}
	if err := database.CheckTLS(cfg); err != nil {
		return fail(err)
	}
	r := &migrate.Runner{
		Open: func(name string) (*sql.DB, error) {
			c := cfg
			c.DBName = name
			db, err := sql.Open("postgres", c.DSN())
			if err == nil {
				db.SetMaxOpenConns(2)
			}
			return db, err
		},
		LockTimeout:      *lockTimeout,
		StatementTimeout: *stmtTimeout,
		Out:              stdout,
	}
	led, err := r.ReadLedger(ctx, set.Databases())
	if err != nil {
		return fail(err)
	}
	p := migrate.Compute(set, led)
	if *only != "" {
		if err := p.Drift(); err != nil {
			return fail(err)
		}
		if p, err = p.ForDB(*only); err != nil {
			return fail(err)
		}
	}

	if cmd == "status" || !*yes {
		printPlan(stdout, p)
		if err := p.Drift(); err != nil {
			return fail(err)
		}
		if n := len(p.Pending()); n > 0 {
			if cmd == "apply" {
				fmt.Fprintf(stdout, "%d to apply; nothing changed (add -yes)\n", n)
				return 0
			}
			return 3
		}
		return 0
	}
	if cmd == "baseline" {
		if err := p.Drift(); err != nil {
			return fail(err)
		}
		if p, err = p.Through(through); err != nil {
			return fail(err)
		}
	}
	if len(p.Pending()) == 0 {
		fmt.Fprintln(stdout, "nothing pending")
		return 0
	}
	if err := r.Apply(ctx, set, p, cmd == "baseline"); err != nil {
		if errors.Is(err, context.Canceled) {
			return fail(errors.New("interrupted"))
		}
		return fail(err)
	}
	return 0
}

func printPlan(w io.Writer, p migrate.Plan) {
	for _, s := range p.Steps {
		how := ""
		if s.Record != nil {
			how = fmt.Sprintf("%s %s by %s", s.Record.Mode, s.Record.AppliedAt.UTC().Format(time.RFC3339), s.Record.AppliedBy)
		}
		fmt.Fprintf(w, "%-8s %-16s %-42s %s\n", s.State, s.DB, s.File.File, how)
	}
	for _, o := range p.Orphans {
		fmt.Fprintf(w, "%-8s %-16s %-42s not in the manifest\n", "orphan", o.DB, o.File)
	}
}

func env(k, def string) string {
	if v, ok := os.LookupEnv(k); ok {
		return v
	}
	return def
}
