// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

// unheaded-aide runs AIDE the quiet way (S-EVIDENCE-RUNS DE1): one check a
// week from its own timer, one line per run, a change reported once, and a
// database that is built and updated only by hand after a review.
//
//	check          compare the tree with the database; never writes one
//	init           build a new database beside the current one and summarise it
//	promote        make the new database the current one
//	review         print the diff that is waiting for a review
//	accept <hash>  update the database to the reviewed diff, and no other
//
// check is what unheaded-aide-check.service runs. The rest is run by a
// person, as root.
package main

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"

	"unheaded/pkg/cmdline"
)

const (
	// maxReportBytes bounds the report read from aide. With
	// report_level=list_entries an entry is about 100 bytes, so this holds
	// some 150,000 changed entries; more than that is not a diff to review.
	maxReportBytes = 16 << 20
	// maxKeptEntries bounds what state.json keeps of a diff for review.
	maxKeptEntries = 1000
	stateFile      = "state.json"
	metricsFile    = "unheaded_aide.prom"
	newDB, curDB   = "aide.db.new", "aide.db"

	// exitUnreadable and exitNotStarted are this program's own values for
	// unheaded_aide_exit_code, beside AIDE's (0 clean, 1-7 change bits, 14
	// and above its errors).
	exitUnreadable = 99
	exitNotStarted = 127
)

// env is what a run takes from outside itself.
type env struct {
	now       func() time.Time
	readBytes func() int64 // bytes this unit read from disk, or -1
}

func main() {
	os.Exit(run(context.Background(), os.Args[1:], os.Stdout, os.Stderr, env{}))
}

// state is state.json: the last completed check and the diff nobody has
// reviewed yet.
type state struct {
	Version    int        `json:"version"`
	LastRun    int64      `json:"last_run"`
	LastExit   int        `json:"last_exit"`
	Entries    int        `json:"entries"`
	RunSeconds int64      `json:"run_seconds"`
	Pending    *pending   `json:"pending"`
	Accepted   []accepted `json:"accepted,omitempty"`
}

type pending struct {
	Hash      string  `json:"hash"`
	FirstSeen int64   `json:"first_seen"`
	Added     int     `json:"added"`
	Removed   int     `json:"removed"`
	Changed   int     `json:"changed"`
	Entries   []entry `json:"entries,omitempty"`
	Truncated int     `json:"truncated,omitempty"`
}

type entry struct {
	Kind  string `json:"kind"`
	Path  string `json:"path"`
	Flags string `json:"flags"`
}

type accepted struct {
	Hash string `json:"hash"`
	At   int64  `json:"at"`
}

// report is the part of AIDE's JSON report this program reads: what
// differs, and how. Times, the run time and the database hashes are not
// read, so two reports of the same diff are equal.
type report struct {
	Count struct {
		Total int `json:"total"`
	} `json:"number_of_entries"`
	Added   map[string]string `json:"added"`
	Removed map[string]string `json:"removed"`
	Changed map[string]string `json:"changed"`
}

func parseReport(raw []byte) (*report, error) {
	var r report
	if err := json.Unmarshal(raw, &r); err != nil {
		return nil, fmt.Errorf("the report is not AIDE's JSON: %w", err)
	}
	return &r, nil
}

func (r *report) entries() []entry {
	var out []entry
	for kind, m := range map[string]map[string]string{"added": r.Added, "removed": r.Removed, "changed": r.Changed} {
		for p, flags := range m {
			out = append(out, entry{Kind: kind, Path: p, Flags: strings.TrimRight(flags, " ")})
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Kind != out[j].Kind {
			return out[i].Kind < out[j].Kind
		}
		return out[i].Path < out[j].Path
	})
	return out
}

// hash names a diff: every changed path, what kind of change, and which
// attributes changed. No time is part of it.
func (r *report) hash() string {
	h := sha256.New()
	for _, e := range r.entries() {
		fmt.Fprintf(h, "%s\x00%s\x00%s\n", e.Kind, e.Path, e.Flags)
	}
	return hex.EncodeToString(h.Sum(nil))
}

type tool struct {
	aide, config, dbDir, stateDir, textfileDir string
	out, errw                                  io.Writer
	env                                        env
}

func run(ctx context.Context, args []string, stdout, stderr io.Writer, e env) int {
	if e.now == nil {
		e.now = time.Now
	}
	if e.readBytes == nil {
		e.readBytes = cgroupReadBytes
	}
	fl := flag.NewFlagSet("unheaded-aide", flag.ContinueOnError)
	spec := cmdline.Spec{
		Name:     "unheaded-aide",
		Purpose:  "Run AIDE quietly: one line per check, a change reported once, a database updated only after a review.",
		Operands: "check | init | promote | review | accept <hash>",
		MaxArgs:  cmdline.Unlimited,
		Commands: []cmdline.Command{
			{Name: "check", Purpose: "compare the tree with the database; one line, the metrics, the state; never writes a database"},
			{Name: "init", Purpose: "build aide.db.new and print a summary to read before promote"},
			{Name: "promote", Purpose: "make aide.db.new the database"},
			{Name: "review", Purpose: "print the diff that is waiting for a review"},
			{Name: "accept <hash>", Purpose: "update the database to the reviewed diff; refused if the tree has changed since"},
		},
		Examples: []string{
			"unheaded-aide check --textfile-dir /var/lib/unheaded/textfile",
			"sudo unheaded-aide init && sudo unheaded-aide promote",
			"sudo unheaded-aide review",
		},
		Details: "check exits 0 for a clean tree and for a detected change (the change is in the line, the metrics and\n" +
			"the state), 1 when AIDE could not run or its report could not be read. unheaded_aide_exit_code is AIDE's:\n" +
			"0 clean, 1-7 change bits, 14 and above its errors; 99 the report could not be read, 127 aide did not start.",
	}
	t := &tool{out: stdout, errw: stderr, env: e}
	fl.StringVar(&t.aide, "aide", "/usr/bin/aide", "the aide binary")
	fl.StringVar(&t.config, "config", "/etc/aide/aide-unheaded.conf", "AIDE config")
	fl.StringVar(&t.dbDir, "db-dir", "/var/lib/aide", "directory of aide.db and aide.db.new, as the config names them")
	fl.StringVar(&t.stateDir, "state", "/var/lib/unheaded/aide", "directory of state.json")
	fl.StringVar(&t.textfileDir, "textfile-dir", "", "with check: write unheaded_aide.prom here for the textfile collector")
	if code, exit := cmdline.ParseSet(fl, spec, flagsFirst(args), stdout, stderr); exit {
		return code
	}
	rest := fl.Args()
	if len(rest) == 0 {
		return cmdline.ReportUsageError(stderr, spec, "missing command")
	}
	want := map[string]int{"check": 0, "init": 0, "promote": 0, "review": 0, "accept": 1}
	n, ok := want[rest[0]]
	if !ok {
		return cmdline.ReportUnknownCommand(stderr, spec, rest[0])
	}
	if len(rest)-1 != n {
		return cmdline.ReportUsageError(stderr, spec, "%s takes %d operand(s), got %d", rest[0], n, len(rest)-1)
	}
	var err error
	switch rest[0] {
	case "check":
		return t.check(ctx)
	case "init":
		err = t.init(ctx)
	case "promote":
		err = t.promote()
	case "review":
		err = t.review()
	case "accept":
		err = t.accept(ctx, rest[1])
	}
	if err != nil {
		fmt.Fprintln(stderr, "unheaded-aide:", err)
		return 1
	}
	return 0
}

// flagsFirst moves the options in front of the operands, so "check
// --textfile-dir DIR" reads like "--textfile-dir DIR check". Every option
// but the built-in --help, --version and --man takes a value.
func flagsFirst(args []string) []string {
	var flags, operands []string
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--":
			operands = append(operands, args[i:]...)
			i = len(args)
		case len(a) > 1 && a[0] == '-':
			flags = append(flags, a)
			if !strings.Contains(a, "=") && !cmdline.IsHelpArg(a) && !cmdline.IsVersionArg(a) && !cmdline.IsManArg(a) && i+1 < len(args) {
				i++
				flags = append(flags, args[i])
			}
		default:
			operands = append(operands, a)
		}
	}
	return append(flags, operands...)
}

// runAide runs aide with the config and one action, and returns its report
// and exit code. code is exitNotStarted when it did not run at all.
func (t *tool) runAide(ctx context.Context, action string) (out []byte, code int, err error) {
	cmd := exec.CommandContext(ctx, t.aide, "--config="+t.config, action) // #nosec G204 -- the aide binary and config an operator names on the command line; action is one of three constants
	var buf bytes.Buffer
	cmd.Stdout = &capped{w: &buf, left: maxReportBytes + 1}
	// AIDE's own complaint (a config line it does not parse, a database it
	// cannot open) goes to the journal as it is; a clean run prints none.
	cmd.Stderr = t.errw
	runErr := cmd.Run()
	var ee *exec.ExitError
	switch {
	case runErr == nil:
		return buf.Bytes(), 0, nil
	case errors.As(runErr, &ee) && ee.ExitCode() >= 0:
		return buf.Bytes(), ee.ExitCode(), nil
	default:
		return nil, exitNotStarted, fmt.Errorf("aide did not run: %w", runErr)
	}
}

// capped keeps the first bytes written to it and drops the rest, so a
// runaway report cannot fill memory.
type capped struct {
	w    *bytes.Buffer
	left int
}

func (c *capped) Write(p []byte) (int, error) {
	n := len(p)
	if len(p) > c.left {
		p = p[:c.left]
	}
	c.left -= len(p)
	c.w.Write(p)
	return n, nil
}

// readReport judges one aide run: its report, or why there is none.
func readReport(out []byte, code int, runErr error) (*report, int, error) {
	if runErr != nil {
		return nil, code, runErr
	}
	if code > 7 {
		return nil, code, fmt.Errorf("aide failed")
	}
	if len(out) > maxReportBytes {
		return nil, exitUnreadable, fmt.Errorf("the report is larger than %d bytes", maxReportBytes)
	}
	r, err := parseReport(out)
	if err != nil {
		return nil, exitUnreadable, err
	}
	n := len(r.Added) + len(r.Removed) + len(r.Changed)
	if (code == 0) != (n == 0) {
		return nil, exitUnreadable, fmt.Errorf("aide exited %d with %d changed entries in its report", code, n)
	}
	return r, code, nil
}

func (t *tool) check(ctx context.Context) int {
	start := t.env.now()
	out, code, runErr := t.runAide(ctx, "--check")
	end := t.env.now()
	secs := int64(end.Sub(start) / time.Second)
	read := "unknown"
	if n := t.env.readBytes(); n >= 0 {
		read = strconv.FormatInt(n, 10)
	}

	st, stErr := t.loadState()
	r, code, err := readReport(out, code, runErr)
	if err == nil && stErr != nil {
		err, code = stErr, exitUnreadable
	}
	if err != nil {
		// The state stays as it was: the last completed run is still the
		// last one, and a pending diff is still pending.
		fmt.Fprintf(t.out, "aide: error exit=%d secs=%d: %v\n", code, secs, err)
		changed := 0
		if st.Pending != nil {
			changed = st.Pending.Added + st.Pending.Removed + st.Pending.Changed
		}
		if merr := t.writeMetrics(st.LastRun, st.Entries, changed, secs, code, 0); merr != nil {
			fmt.Fprintln(t.errw, "unheaded-aide: metrics:", merr)
		}
		return 1
	}

	st.Version, st.LastRun, st.LastExit, st.Entries, st.RunSeconds = 1, end.Unix(), code, r.Count.Total, secs
	newDiff, changed := 0, len(r.Added)+len(r.Removed)+len(r.Changed)
	if changed == 0 {
		// The tree matches the database again: nothing is left to review.
		st.Pending = nil
		fmt.Fprintf(t.out, "aide: clean entries=%d secs=%d read=%s\n", r.Count.Total, secs, read)
	} else {
		hash := r.hash()
		if st.Pending == nil || st.Pending.Hash != hash {
			newDiff = 1
			p := &pending{Hash: hash, FirstSeen: end.Unix(), Added: len(r.Added), Removed: len(r.Removed), Changed: len(r.Changed), Entries: r.entries()}
			if len(p.Entries) > maxKeptEntries {
				p.Truncated = len(p.Entries) - maxKeptEntries
				p.Entries = p.Entries[:maxKeptEntries]
			}
			st.Pending = p
		}
		fmt.Fprintf(t.out, "aide: changed entries=%d added=%d removed=%d changed=%d diff=%s new=%d secs=%d read=%s\n",
			r.Count.Total, len(r.Added), len(r.Removed), len(r.Changed), hash[:12], newDiff, secs, read)
	}
	if err := t.saveState(st); err != nil {
		fmt.Fprintln(t.errw, "unheaded-aide: state:", err)
		if merr := t.writeMetrics(st.LastRun, st.Entries, changed, secs, exitUnreadable, 0); merr != nil {
			fmt.Fprintln(t.errw, "unheaded-aide: metrics:", merr)
		}
		return 1
	}
	if err := t.writeMetrics(st.LastRun, st.Entries, changed, secs, code, newDiff); err != nil {
		fmt.Fprintln(t.errw, "unheaded-aide: metrics:", err)
		return 1
	}
	return 0
}

func (t *tool) writeMetrics(lastRun int64, entries, changed int, secs int64, code, newDiff int) error {
	if t.textfileDir == "" {
		return nil
	}
	var b strings.Builder
	for _, m := range []struct {
		name, help string
		v          int64
	}{
		{"unheaded_aide_last_run_timestamp_seconds", "End of the last AIDE check that completed, as Unix time.", lastRun},
		{"unheaded_aide_entries", "Entries AIDE compared in the last completed check.", int64(entries)},
		{"unheaded_aide_changed_entries", "Added, removed and changed entries waiting for a review.", int64(changed)},
		{"unheaded_aide_run_seconds", "Wall time of this run.", secs},
		{"unheaded_aide_exit_code", "AIDE's exit code for this run: 0 clean, 1-7 change bits, 14 and above errors; 99 unreadable report, 127 not started.", int64(code)},
		{"unheaded_aide_new_diff", "1 on the run that first saw a diff, 0 on every other run.", int64(newDiff)},
	} {
		fmt.Fprintf(&b, "# HELP %s %s\n# TYPE %s gauge\n%s %d\n", m.name, m.help, m.name, m.name, m.v)
	}
	return writeAtomic(t.textfileDir, metricsFile, []byte(b.String()), 0o644)
}

func (t *tool) loadState() (state, error) {
	var st state
	root, err := os.OpenRoot(t.stateDir)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return st, nil
		}
		return st, err
	}
	defer func() { _ = root.Close() }()
	f, err := root.Open(stateFile)
	if errors.Is(err, fs.ErrNotExist) {
		return st, nil
	}
	if err != nil {
		return st, err
	}
	defer func() { _ = f.Close() }()
	raw, err := io.ReadAll(io.LimitReader(f, 8<<20))
	if err != nil {
		return st, err
	}
	if err := json.Unmarshal(raw, &st); err != nil {
		return state{}, fmt.Errorf("%s is not readable; not overwriting it: %w", stateFile, err)
	}
	return st, nil
}

func (t *tool) saveState(st state) error {
	raw, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return err
	}
	return writeAtomic(t.stateDir, stateFile, append(raw, '\n'), 0o600)
}

// writeAtomic writes dir/name through a temporary file and a rename, so a
// reader sees the old file or the new one. A file written by root (a
// person running accept) is given to the directory's owner, so the
// service's own account can replace it at the next check.
func writeAtomic(dir, name string, data []byte, mode os.FileMode) error {
	root, err := os.OpenRoot(dir)
	if err != nil {
		return err
	}
	defer func() { _ = root.Close() }()
	tmp := "." + name + "." + strconv.Itoa(os.Getpid()) + ".tmp"
	f, err := root.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, mode)
	if err != nil {
		return err
	}
	_, err = f.Write(data)
	if err == nil {
		err = f.Chmod(mode)
	}
	if err == nil && os.Geteuid() == 0 {
		if di, serr := root.Stat("."); serr == nil {
			if st, ok := di.Sys().(*syscall.Stat_t); ok {
				err = f.Chown(int(st.Uid), int(st.Gid))
			}
		}
	}
	if err == nil {
		err = f.Sync()
	}
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err == nil {
		err = root.Rename(tmp, name)
	}
	if err != nil {
		_ = root.Remove(tmp)
	}
	return err
}

// cgroupReadBytes is what this unit's cgroup has read from disk: the run's
// own I/O when systemd started it with IOAccounting. -1 when it cannot be
// read.
func cgroupReadBytes() int64 {
	raw, err := os.ReadFile("/proc/self/cgroup")
	if err != nil {
		return -1
	}
	var cg string
	for _, l := range strings.Split(string(raw), "\n") {
		if p, ok := strings.CutPrefix(l, "0::"); ok {
			cg = strings.Trim(p, "/")
		}
	}
	if cg == "" {
		return -1 // the root cgroup, or cgroup v1: not this run's number
	}
	root, err := os.OpenRoot("/sys/fs/cgroup")
	if err != nil {
		return -1
	}
	defer func() { _ = root.Close() }()
	f, err := root.Open(cg + "/io.stat")
	if err != nil {
		return -1
	}
	defer func() { _ = f.Close() }()
	stat, err := io.ReadAll(io.LimitReader(f, 1<<20))
	if err != nil {
		return -1
	}
	var total int64
	for _, m := range regexp.MustCompile(`\brbytes=(\d+)`).FindAllStringSubmatch(string(stat), -1) {
		n, _ := strconv.ParseInt(m[1], 10, 64)
		total += n
	}
	return total
}

func (t *tool) dbRoot(create bool) (*os.Root, error) {
	if create {
		if err := os.MkdirAll(t.dbDir, 0o700); err != nil {
			return nil, err
		}
	}
	return os.OpenRoot(t.dbDir)
}

// init builds aide.db.new and says what is in it. It promotes nothing: a
// person reads the summary first.
func (t *tool) init(ctx context.Context) error {
	root, err := t.dbRoot(true)
	if err != nil {
		return err
	}
	defer func() { _ = root.Close() }()
	start := t.env.now()
	out, code, runErr := t.runAide(ctx, "--init")
	if runErr != nil {
		return runErr
	}
	if code != 0 {
		_ = root.Remove(newDB)
		return fmt.Errorf("aide --init failed (exit %d); no database was written", code)
	}
	secs := int64(t.env.now().Sub(start) / time.Second)
	fi, err := root.Stat(newDB)
	if err != nil {
		return fmt.Errorf("aide --init wrote no %s in %s: %w", newDB, t.dbDir, err)
	}
	total := 0
	if r, perr := parseReport(out); perr == nil {
		total = r.Count.Total
	}
	dirs, counted, err := dbDirectories(root)
	if err != nil {
		return err
	}
	if total == 0 {
		total = counted
	}
	fmt.Fprintf(t.out, "entries: %d\ndatabase: %s/%s, %d bytes", total, t.dbDir, newDB, fi.Size())
	if total > 0 {
		fmt.Fprintf(t.out, " (%d bytes an entry)", fi.Size()/int64(total))
	}
	fmt.Fprintf(t.out, "\nseconds: %d\nlargest directories by entries:\n", secs)
	for i, d := range dirs {
		if i == 10 {
			break
		}
		fmt.Fprintf(t.out, "  %s  %d\n", d.name, d.n)
	}
	fmt.Fprintf(t.out, "nothing is promoted yet. Read this, then: unheaded-aide promote\n")
	return nil
}

type dirCount struct {
	name string
	n    int
}

// dbDirectories counts the new database's entries by directory, two levels
// deep, largest first.
func dbDirectories(root *os.Root) ([]dirCount, int, error) {
	f, err := root.Open(newDB)
	if err != nil {
		return nil, 0, err
	}
	defer func() { _ = f.Close() }()
	var r io.Reader = bufio.NewReader(f)
	if magic, perr := r.(*bufio.Reader).Peek(2); perr == nil && magic[0] == 0x1f && magic[1] == 0x8b {
		zr, zerr := gzip.NewReader(r)
		if zerr != nil {
			return nil, 0, zerr
		}
		defer func() { _ = zr.Close() }()
		r = zr
	}
	counts, total, permAt := map[string]int{}, 0, -1
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 64<<10), 4<<20)
	for sc.Scan() {
		line := sc.Text()
		if spec, ok := strings.CutPrefix(line, "@@db_spec "); ok {
			for i, name := range strings.Fields(spec) {
				if name == "perm" {
					permAt = i
				}
			}
			continue
		}
		if !strings.HasPrefix(line, "/") {
			continue
		}
		fields := strings.Fields(line)
		dir := fields[0]
		isDir := false
		if permAt > 0 && permAt < len(fields) {
			if m, perr := strconv.ParseUint(fields[permAt], 8, 32); perr == nil {
				isDir = m&0o170000 == 0o040000
			}
		}
		if !isDir {
			dir = path.Dir(dir)
		}
		if parts := strings.SplitN(strings.TrimPrefix(dir, "/"), "/", 3); len(parts) > 2 {
			dir = "/" + parts[0] + "/" + parts[1]
		}
		counts[dir]++
		total++
	}
	if err := sc.Err(); err != nil {
		return nil, 0, fmt.Errorf("read %s: %w", newDB, err)
	}
	out := make([]dirCount, 0, len(counts))
	for name, n := range counts {
		out = append(out, dirCount{name, n})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].n != out[j].n {
			return out[i].n > out[j].n
		}
		return out[i].name < out[j].name
	})
	return out, total, nil
}

// promote makes aide.db.new the database. A diff found against the old
// database says nothing about the new one, so it is dropped.
func (t *tool) promote() error {
	root, err := t.dbRoot(false)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return fmt.Errorf("no new database to promote in %s: run unheaded-aide init first", t.dbDir)
		}
		return err
	}
	defer func() { _ = root.Close() }()
	fi, err := root.Stat(newDB)
	if errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("no new database to promote in %s: run unheaded-aide init first", t.dbDir)
	}
	if err != nil {
		return err
	}
	st, err := t.loadState()
	if err != nil {
		return err
	}
	if err := root.Rename(newDB, curDB); err != nil {
		return err
	}
	if st.Pending != nil {
		st.Pending = nil
		if err := t.saveState(st); err != nil {
			return fmt.Errorf("the database is promoted, but the pending diff could not be cleared: %w", err)
		}
	}
	fmt.Fprintf(t.out, "promoted %s/%s (%d bytes)\n", t.dbDir, curDB, fi.Size())
	return nil
}

func (t *tool) review() error {
	st, err := t.loadState()
	if err != nil {
		return err
	}
	p := st.Pending
	if p == nil {
		fmt.Fprintln(t.out, "no diff is pending")
		return nil
	}
	fmt.Fprintf(t.out, "diff %s\nfirst seen %s\nadded %d, removed %d, changed %d\n",
		p.Hash, time.Unix(p.FirstSeen, 0).UTC().Format(time.RFC3339), p.Added, p.Removed, p.Changed)
	for _, e := range p.Entries {
		fmt.Fprintf(t.out, "%-8s %s  %s\n", e.Kind, e.Path, e.Flags)
	}
	if p.Truncated > 0 {
		fmt.Fprintf(t.out, "(%d more entries are not kept here: aide --config=%s --check lists them all)\n", p.Truncated, t.config)
	}
	fmt.Fprintf(t.out, "once every line is explained: unheaded-aide accept %s\n", p.Hash)
	return nil
}

var hashRe = regexp.MustCompile(`^[0-9a-f]{64}$`)

// accept updates the database to the reviewed diff. It checks the hash
// before it runs anything, and again against what the update found: a tree
// that changed since the review is not what was read, so the update is
// thrown away.
func (t *tool) accept(ctx context.Context, hash string) error {
	st, err := t.loadState()
	if err != nil {
		return err
	}
	if !hashRe.MatchString(hash) {
		return fmt.Errorf("%q is not a diff hash (64 hex digits, as review prints it)", hash)
	}
	if st.Pending == nil {
		return fmt.Errorf("no diff is pending")
	}
	if st.Pending.Hash != hash {
		return fmt.Errorf("the pending diff is %s, not %s", st.Pending.Hash, hash)
	}
	root, err := t.dbRoot(false)
	if err != nil {
		return err
	}
	defer func() { _ = root.Close() }()
	out, code, runErr := t.runAide(ctx, "--update")
	r, code, err := readReport(out, code, runErr)
	if err != nil {
		_ = root.Remove(newDB)
		return fmt.Errorf("aide --update (exit %d): %w; the database is unchanged", code, err)
	}
	if r.hash() != hash {
		_ = root.Remove(newDB)
		return fmt.Errorf("the tree has changed since the reviewed check (now %s); the database is unchanged: run check, then review again", r.hash()[:12])
	}
	if err := root.Rename(newDB, curDB); err != nil {
		return err
	}
	now := t.env.now().Unix()
	st.Pending, st.Entries = nil, r.Count.Total
	st.Accepted = append(st.Accepted, accepted{Hash: hash, At: now})
	if len(st.Accepted) > 20 {
		st.Accepted = st.Accepted[len(st.Accepted)-20:]
	}
	if err := t.saveState(st); err != nil {
		return fmt.Errorf("the database is updated, but the state could not be written: %w", err)
	}
	fmt.Fprintf(t.out, "accepted %s: the database now holds %d entries\n", hash[:12], r.Count.Total)
	return nil
}
