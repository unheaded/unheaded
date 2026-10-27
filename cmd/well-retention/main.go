// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

// well-retention enforces The Well's retention classes
// (compliance/retention/classes.yaml; retention policy v1 §11; S-WELL-SPRING
// Phase 4, M-074). One weekly run, unheaded-well-retention.timer:
//
//	well-retention -dry-run                    # counts only; always allowed
//	well-retention                             # live: refused while the file says ratified: false
//	well-retention -dry-run -class zhen-conversations
//
// Per class, in file order: purge (batched deletes of rows older than the
// class's online age), purge-function (the table's own purge, 033),
// archive-purge (a redacted copy into unheaded_ops.retention_archive is
// committed before the delete is), or partition-check (ERDA 5.4's trigger
// on an append-only table, which is never deleted from). Every class,
// dry or live, writes one unheaded_ops.retention_runs row. A class under
// legal hold (hold: true) deletes nothing and records action held. The
// job logs counts, never row contents.
//
// Connection, as the login role retention_runner (migrations 042/043):
// WELL_HOST (127.0.0.1), WELL_PORT (5432), WELL_USER (retention_runner),
// WELL_PASSWORD or WELL_PASSWORD_FILE (not both), else the systemd
// credential well-password (no default, FND-007),
// WELL_SSLMODE (require; disable only for this host), WELL_APP_DB
// (unheaded_app), WELL_OPS_DB (unheaded_ops). -metrics-dir (or
// WELL_METRICS_DIR) writes well_retention.prom for node_exporter.
//
// Exit: 0 every class done, 1 a class or the connection failed, 2 usage
// (including a live run of an unratified file).
package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"unheaded/pkg/database"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	code := run(ctx, os.Args[1:], os.Stdout, os.Stderr)
	stop()
	os.Exit(code)
}

func getenv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// credentialName is the systemd credential the unit loads
// (LoadCredential=well-password:...).
const credentialName = "well-password"

// password returns the login password from WELL_PASSWORD, or the file
// WELL_PASSWORD_FILE names, or else the systemd credential well-password
// ($CREDENTIALS_DIRECTORY); never WELL_PASSWORD and WELL_PASSWORD_FILE
// both, never none.
func password(get func(string) string) (string, error) {
	pw, file := get("WELL_PASSWORD"), get("WELL_PASSWORD_FILE")
	if pw == "" && file == "" {
		if dir := get("CREDENTIALS_DIRECTORY"); dir != "" {
			file = filepath.Join(dir, credentialName)
		}
	}
	switch {
	case pw != "" && file != "":
		return "", errors.New("set WELL_PASSWORD or WELL_PASSWORD_FILE, not both")
	case pw != "":
		return pw, nil
	case file != "":
		b, err := os.ReadFile(file) // #nosec G304 -- the operator names the credential file
		if err != nil {
			return "", fmt.Errorf("WELL_PASSWORD_FILE: %w", err)
		}
		pw = strings.TrimRight(string(b), "\r\n")
		if pw == "" {
			return "", errors.New("WELL_PASSWORD_FILE is empty")
		}
		return pw, nil
	}
	return "", database.ErrMissingCredentials
}

func run(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	fl := flag.NewFlagSet("well-retention", flag.ContinueOnError)
	fl.SetOutput(stderr)
	classesPath := fl.String("classes", "/etc/unheaded/well/retention-classes.yaml", "retention classes (compliance/retention/classes.yaml)")
	dryRun := fl.Bool("dry-run", false, "count what each class would do; delete nothing")
	only := fl.String("class", "", "run this class only")
	batch := fl.Int("batch", 1000, "rows per delete transaction (1..100000)")
	stmtTimeout := fl.Duration("statement-timeout", 5*time.Minute, "limit per statement")
	metricsDir := fl.String("metrics-dir", os.Getenv("WELL_METRICS_DIR"), "write well_retention.prom here (node_exporter textfile); empty: none")
	if err := fl.Parse(args); err != nil {
		return 2
	}
	if fl.NArg() != 0 {
		fmt.Fprintf(stderr, "well-retention: unexpected argument %q\n", fl.Arg(0))
		return 2
	}
	if *batch < 1 || *batch > 100000 {
		fmt.Fprintln(stderr, "well-retention: -batch must be 1..100000")
		return 2
	}
	if *stmtTimeout <= 0 {
		fmt.Fprintln(stderr, "well-retention: -statement-timeout must be positive")
		return 2
	}
	fh, err := os.Open(*classesPath)
	if err != nil {
		fmt.Fprintf(stderr, "well-retention: %v\n", err)
		return 2
	}
	file, err := parseClasses(fh)
	fh.Close() // #nosec G104 -- read-only file, fully read
	if err != nil {
		fmt.Fprintf(stderr, "well-retention: %s: %v\n", *classesPath, err)
		return 2
	}
	classes, err := file.selectClasses(*only)
	if err != nil {
		fmt.Fprintf(stderr, "well-retention: %v\n", err)
		return 2
	}
	if !*dryRun && !file.Ratified {
		fmt.Fprintf(stderr, "well-retention: refusing a live run: %s says ratified: false (retention policy v1 §11, E-C); -dry-run works\n", *classesPath)
		return 2
	}
	pw, err := password(os.Getenv)
	if err != nil {
		fmt.Fprintf(stderr, "well-retention: %v\n", err)
		return 2
	}
	base := database.Config{
		Host:     getenv("WELL_HOST", "127.0.0.1"),
		Port:     getenv("WELL_PORT", "5432"),
		User:     getenv("WELL_USER", "retention_runner"),
		Password: pw,
		SSLMode:  getenv("WELL_SSLMODE", "require"),
	}
	appCfg, opsCfg := base, base
	appCfg.DBName = getenv("WELL_APP_DB", dbApp)
	opsCfg.DBName = getenv("WELL_OPS_DB", dbOps)

	r := &runner{dryRun: *dryRun, batch: *batch, stmtTimeout: *stmtTimeout, now: time.Now, host: hostName()}
	if r.runID, err = newRunID(); err != nil {
		fmt.Fprintf(stderr, "well-retention: %v\n", err)
		return 1
	}
	// unheaded_ops always (retention_runs); unheaded_app only when a class
	// lives there.
	if r.ops, err = database.Connect(ctx, opsCfg); err != nil {
		fmt.Fprintf(stderr, "well-retention: %s: %v\n", opsCfg.DBName, err)
		return 1
	}
	defer r.ops.Close()
	for _, c := range classes {
		if targets[c.Table].db == dbApp {
			if r.app, err = database.Connect(ctx, appCfg); err != nil {
				fmt.Fprintf(stderr, "well-retention: %s: %v\n", appCfg.DBName, err)
				return 1
			}
			defer r.app.Close()
			break
		}
	}

	fmt.Fprintf(stdout, "well-retention run=%s dry_run=%v ratified=%v classes=%d host=%s\n", r.runID, r.dryRun, file.Ratified, len(classes), r.host)
	var results []result
	failed := 0
	for _, c := range classes {
		if ctx.Err() != nil {
			fmt.Fprintf(stderr, "well-retention: stopped before class %s: %v\n", c.Name, ctx.Err())
			failed++
			break
		}
		res := r.runClass(ctx, c)
		results = append(results, res)
		fmt.Fprintln(stdout, describe(res))
		if res.Err != "" {
			fmt.Fprintf(stderr, "well-retention: class %s: %s\n", res.Class, res.Err)
			failed++
		}
	}
	if *metricsDir != "" {
		if err := writeMetrics(*metricsDir, metricsText(r.now(), r.dryRun, results)); err != nil {
			fmt.Fprintf(stderr, "well-retention: metrics: %v\n", err)
			failed++
		}
	}
	if failed > 0 {
		return 1
	}
	return 0
}

func describe(r result) string {
	var b strings.Builder
	fmt.Fprintf(&b, "class=%s table=%s action=%s", r.Class, r.Table, r.Action)
	if r.Cutoff != nil {
		fmt.Fprintf(&b, " cutoff=%s", r.Cutoff.UTC().Format(time.RFC3339))
	}
	if r.Verdict == nil {
		fmt.Fprintf(&b, " eligible=%d deleted=%d archived=%d held=%d", r.Eligible, r.Deleted, r.Archived, r.Held)
	} else {
		v := r.Verdict
		fmt.Fprintf(&b, " rows=%d bytes=%d partition_due=%v", v.Rows, v.Bytes, v.Due)
		if v.PartitionBy != nil {
			fmt.Fprintf(&b, " partition_by=%s", v.PartitionBy.UTC().Format("2006-01-02"))
		}
		if v.Reason != "" {
			fmt.Fprintf(&b, " reason=%q", v.Reason)
		}
	}
	if r.Err != "" {
		b.WriteString(" error=1")
	}
	return b.String()
}

func newRunID() (string, error) {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("run id: %w", err)
	}
	return hex.EncodeToString(b), nil
}

// hostName is KINGDOM_NODE when set (as Sword and the chaos journal use),
// else the kernel's host name.
func hostName() string {
	h := os.Getenv("KINGDOM_NODE")
	if h == "" {
		h, _ = os.Hostname()
	}
	if h == "" {
		h = "unknown"
	}
	return clip(h, 253)
}
