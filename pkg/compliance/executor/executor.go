// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

// Package executor applies fixes from the remediation model (ADR-099
// phase 2) the way scripts/host/remediate.sh does: plan by default and
// writing nothing; apply as root, backing up everything it touches and
// recording an undo record that replays newest-first; any failed
// validation undoes the whole fix. It runs only the closed action set and
// only allowlisted commands, as argv, never through a shell.
//
// Stricter than the script in two ways: it refuses to write or chmod
// through a symlink, and every failed validation restores the fix (the
// script left a halting auditd.conf installed and only told you how to
// undo it).
package executor

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"time"

	"unheaded/pkg/compliance/remediation"
)

// ErrRefused is a fix refused before or during apply; anything it had
// changed has been restored.
var ErrRefused = errors.New("fix refused")

// errRestoreFailed marks an apply whose own restore failed: the host is in
// a state nobody chose, and the journal says so.
var errRestoreFailed = errors.New("restore failed")

// Commands are the executables actions may run, by role. They default to
// the names on PATH and take the same environment overrides as
// remediate.sh, so the test harness can substitute fakes.
type Commands struct {
	Sysctl, Systemctl, SSHD, Visudo, Apt, Sudo, Augenrules string
}

// CommandsFromEnv reads SYSCTL, SYSTEMCTL, SSHD, VISUDO, APT, SUDO_BIN and AUGENRULES.
func CommandsFromEnv() Commands {
	get := func(k, def string) string {
		if v := os.Getenv(k); v != "" {
			return v
		}
		return def
	}
	return Commands{
		Sysctl: get("SYSCTL", "sysctl"), Systemctl: get("SYSTEMCTL", "systemctl"), SSHD: get("SSHD", "sshd"),
		Visudo: get("VISUDO", "visudo"), Apt: get("APT", "apt-get"), Sudo: get("SUDO_BIN", "sudo"),
		Augenrules: get("AUGENRULES", "augenrules"),
	}
}

// argv maps an allowlisted run or validate string to its argv.
func (c Commands) argv(s string) ([]string, error) {
	switch s {
	case "sysctl --system":
		return []string{c.Sysctl, "--system"}, nil
	case "systemctl reload ssh.service":
		return []string{c.Systemctl, "reload", "ssh.service"}, nil
	case "sshd -t":
		return []string{c.SSHD, "-t"}, nil
	case "visudo -c":
		return []string{c.Visudo, "-c"}, nil
	case "augenrules --load":
		return []string{c.Augenrules, "--load"}, nil
	}
	return nil, fmt.Errorf("%w: %q is not an allowlisted command", ErrRefused, s)
}

// Options configure an Executor.
type Options struct {
	Root         string // filesystem root; "" is /
	State        string // backups and journal; default /var/lib/unheaded/remediate
	Host         string // west or east
	Operator     string
	ModelCommit  string
	AllowNonRoot bool // tests only: apply without root, ownership changes best-effort
	Cmd          Commands
	Out          io.Writer
	Now          func() time.Time
}

// Executor plans and applies fixes on one host.
type Executor struct {
	o     Options
	facts remediation.Facts
}

var sudoRSRe = regexp.MustCompile(`(?m)^sudo-rs`)

// New checks the options and learns the host's facts (sudo-rs or not:
// sudo-rs prints its version on stderr).
func New(ctx context.Context, o Options) (*Executor, error) {
	known := false
	for _, h := range remediation.KnownHosts {
		known = known || h == o.Host
	}
	if !known {
		return nil, fmt.Errorf("%w: host %q is not west or east; host-specific content is not defined for it", ErrRefused, o.Host)
	}
	if o.State == "" {
		o.State = "/var/lib/unheaded/remediate"
	}
	if o.Out == nil {
		o.Out = io.Discard
	}
	if o.Now == nil {
		o.Now = time.Now
	}
	if strings.TrimSpace(o.Operator) == "" {
		o.Operator = "unknown"
	}
	e := &Executor{o: o}
	out, _ := exec.CommandContext(ctx, o.Cmd.Sudo, "--version").CombinedOutput() // #nosec G204 -- the operator-configured sudo binary, fixed argument
	e.facts.SudoRS = sudoRSRe.Match(out)
	return e, nil
}

// Facts reports what New learned about the host.
func (e *Executor) Facts() remediation.Facts { return e.facts }

func (e *Executor) path(p string) string { return filepath.Join(e.o.Root, p) }

func (e *Executor) printf(format string, a ...any) { fmt.Fprintf(e.o.Out, format, a...) }

// --- undo record --------------------------------------------------------------

type undoStep struct {
	Op     string   `json:"op"` // restore, remove, chmod, run
	Path   string   `json:"path,omitempty"`
	Backup string   `json:"backup,omitempty"`
	Mode   uint32   `json:"mode,omitempty"`
	UID    int      `json:"uid"`
	GID    int      `json:"gid"`
	Argv   []string `json:"argv,omitempty"`
}

type applyRun struct {
	e     *Executor
	dir   string
	steps []undoStep
	n     int
}

// record appends an undo step and rewrites undo.json at once, so a crash
// mid-apply still leaves a record that undoes what was done.
func (r *applyRun) record(s undoStep) error {
	r.steps = append(r.steps, s)
	b, err := json.MarshalIndent(r.steps, "", "  ")
	if err != nil {
		return err
	}
	return writeAtomic(filepath.Join(r.dir, "undo.json"), b, 0o600, -1, -1, false)
}

// --- filesystem helpers -------------------------------------------------------

func ownerOf(fi fs.FileInfo) (int, int) {
	if st, ok := fi.Sys().(*syscall.Stat_t); ok {
		return int(st.Uid), int(st.Gid)
	}
	return -1, -1
}

// writeAtomic writes via a temp file in the same directory and a rename,
// so a reader never sees half a file and a symlink at path is replaced,
// not followed.
func writeAtomic(path string, data []byte, mode os.FileMode, uid, gid int, allowNonRoot bool) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), ".remediate-*")
	if err != nil {
		return err
	}
	name := tmp.Name()
	defer os.Remove(name) // #nosec G104 -- best-effort cleanup; gone after a successful rename
	if _, err := tmp.Write(data); err != nil {
		return errors.Join(err, tmp.Close())
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(name, mode); err != nil {
		return err
	}
	if uid >= 0 {
		if err := os.Lchown(name, uid, gid); err != nil && !(allowNonRoot && errors.Is(err, fs.ErrPermission)) {
			return err
		}
	}
	return os.Rename(name, path)
}

// lookupID resolves a user or group name from the host's own passwd or
// group file under root, so a fake root resolves its own names.
func (e *Executor) lookupID(file, name string) (int, error) {
	if name == "" || name == "root" {
		return 0, nil
	}
	f, err := os.Open(e.path(file)) // #nosec G304 -- /etc/passwd or /etc/group under the configured root
	if err != nil {
		return -1, err
	}
	defer f.Close()
	sc := bufio.NewScanner(io.LimitReader(f, 1<<20))
	for sc.Scan() {
		fields := strings.Split(sc.Text(), ":")
		if len(fields) >= 3 && fields[0] == name {
			return strconv.Atoi(fields[2])
		}
	}
	return -1, fmt.Errorf("%w: %s has no %q", ErrRefused, file, name)
}

func (e *Executor) owner(user, group string) (int, int, error) {
	uid, err := e.lookupID("/etc/passwd", user)
	if err != nil {
		return -1, -1, err
	}
	gid, err := e.lookupID("/etc/group", group)
	return uid, gid, err
}

// noSymlink refuses a path that is a symlink: writing or chmod-ing through
// one would change whatever it points at.
func noSymlink(path string, fi fs.FileInfo) error {
	if fi.Mode()&fs.ModeSymlink != 0 {
		return fmt.Errorf("%w: %s is a symlink; not following it", ErrRefused, path)
	}
	return nil
}

// backup records p's current state (content, mode, owner, or absence).
func (r *applyRun) backup(p string) error {
	full := r.e.path(p)
	fi, err := os.Lstat(full)
	if errors.Is(err, fs.ErrNotExist) {
		return r.record(undoStep{Op: "remove", Path: full})
	}
	if err != nil {
		return err
	}
	if err := noSymlink(p, fi); err != nil {
		return err
	}
	if !fi.Mode().IsRegular() {
		return fmt.Errorf("%w: %s is not a regular file", ErrRefused, p)
	}
	data, err := os.ReadFile(full) // #nosec G304 -- a model path (clean, under /etc) under the configured root
	if err != nil {
		return err
	}
	r.n++
	saved := filepath.Join(r.dir, "files", strconv.Itoa(r.n))
	if err := os.MkdirAll(filepath.Dir(saved), 0o700); err != nil {
		return err
	}
	if err := os.WriteFile(saved, data, 0o600); err != nil { // #nosec G703 -- saved is <state>/<backup dir>/files/<counter>, built here, not from input
		return err
	}
	uid, gid := ownerOf(fi)
	return r.record(undoStep{Op: "restore", Path: full, Backup: saved, Mode: uint32(fi.Mode().Perm()), UID: uid, GID: gid})
}

// --- plan and apply ------------------------------------------------------------

func (e *Executor) groupFile() (io.Reader, error) {
	b, err := os.ReadFile(e.path("/etc/group"))
	if errors.Is(err, fs.ErrNotExist) {
		return strings.NewReader(""), nil
	}
	return bytes.NewReader(b), err
}

func (e *Executor) render(fa *remediation.FileAction) ([]byte, os.FileMode, error) {
	g, err := e.groupFile()
	if err != nil {
		return nil, 0, err
	}
	content, err := fa.Render(e.o.Host, e.facts, g)
	if err != nil {
		return nil, 0, err
	}
	perm, err := fa.Perm()
	if err != nil {
		return nil, 0, err
	}
	return []byte(content), os.FileMode(perm), nil
}

// Plan prints what applying fix would change, and changes nothing.
func (e *Executor) Plan(ctx context.Context, fix remediation.Fix) error {
	e.printf("plan %s on %s:\n", fix.ID, e.o.Host)
	for _, a := range fix.Actions {
		switch {
		case a.File != nil:
			want, mode, err := e.render(a.File)
			if err != nil {
				return err
			}
			have, err := os.ReadFile(e.path(a.File.Path))
			fi, serr := os.Lstat(e.path(a.File.Path))
			if err == nil && serr == nil && bytes.Equal(have, want) && fi.Mode().Perm() == mode {
				e.printf("  %s: already as wanted\n", a.File.Path)
			} else {
				e.printf("  %s: would write (mode %04o):\n", a.File.Path, mode)
				for _, l := range strings.Split(strings.TrimRight(string(want), "\n"), "\n") {
					e.printf("    + %s\n", l)
				}
			}
		case a.Setting != nil:
			e.printf("  %s: would set %s=%s (if the file exists)\n", a.Setting.Path, a.Setting.Key, a.Setting.Value)
		case a.Mode != nil:
			for _, p := range e.modePaths(a.Mode) {
				e.printf("  %s: would set %s %s:%s\n", p, a.Mode.Mode, a.Mode.Owner, a.Mode.Group)
			}
		case a.Absent != "":
			if _, err := os.Lstat(e.path(a.Absent)); err == nil {
				e.printf("  %s: would be removed\n", a.Absent)
			}
		case a.Service != nil:
			e.printf("  would run: %s\n", strings.Join(e.serviceArgv(a.Service), "; "))
		case a.Package != nil:
			e.printf("  would run: %s install -y -q %s\n", e.o.Cmd.Apt, a.Package.Name)
		case a.Run != "":
			e.printf("  then: %s\n", a.Run)
		case a.Validate != "":
			e.printf("  then check: %s (restored at once if it fails)\n", a.Validate)
		}
	}
	return ctx.Err()
}

func (e *Executor) modePaths(ma *remediation.ModeAction) []string {
	if ma.Glob == "" {
		return ma.Paths
	}
	matches, _ := filepath.Glob(e.path(ma.Glob))
	out := make([]string, 0, len(matches))
	for _, m := range matches {
		out = append(out, "/"+strings.TrimPrefix(strings.TrimPrefix(m, filepath.Clean(e.o.Root)), "/"))
	}
	return out
}

func (e *Executor) serviceArgv(s *remediation.ServiceAction) []string {
	var out []string
	if s.Enabled != nil && !*s.Enabled {
		out = append(out, e.o.Cmd.Systemctl+" disable --now "+s.Name)
	}
	if s.Masked != nil && *s.Masked {
		out = append(out, e.o.Cmd.Systemctl+" mask "+s.Name)
	}
	return out
}

// Apply applies fix and returns its backup directory. On any failure after
// the first change it undoes everything this apply did, then returns the
// error.
func (e *Executor) Apply(ctx context.Context, fix remediation.Fix) (dir string, err error) {
	if os.Geteuid() != 0 && !e.o.AllowNonRoot {
		return "", fmt.Errorf("%w: apply needs root", ErrRefused)
	}
	if err := os.MkdirAll(e.o.State, 0o700); err != nil {
		return "", err
	}
	if err := e.checkJournal(); err != nil {
		return "", err
	}
	sum, err := fix.SHA256()
	if err != nil {
		return "", err
	}
	// Every apply that got as far as a backup directory is journaled,
	// whatever its outcome.
	defer func() {
		if dir == "" {
			return
		}
		if jerr := e.appendJournal(fix.ID, sum, "apply", outcomeOf(err), dir); jerr != nil {
			err = errors.Join(err, fmt.Errorf("journal: %w", jerr))
		}
	}()
	if err := os.Chmod(e.o.State, 0o700); err != nil { // #nosec G302 -- a directory: 0700 is owner-only, and it needs the x bit
		return "", err
	}
	dir = filepath.Join(e.o.State, e.o.Now().UTC().Format("20060102T150405.000000000Z")+"-"+fix.ID)
	if err := os.Mkdir(dir, 0o700); err != nil {
		return "", err
	}
	if err := writeMeta(dir, undoMeta{Fix: fix.ID, FixSHA256: sum, Host: e.o.Host}); err != nil {
		return dir, err
	}
	r := &applyRun{e: e, dir: dir}
	// An empty record from the start: undo of an apply that changed nothing
	// yet is a no-op, not a missing file.
	if err := writeAtomic(filepath.Join(dir, "undo.json"), []byte("[]\n"), 0o600, -1, -1, false); err != nil {
		return dir, err
	}
	e.printf("apply %s on %s:\n", fix.ID, e.o.Host)
	for i, a := range fix.Actions {
		if err := r.apply(ctx, a); err != nil {
			e.printf("  action %d failed: %v\n  restoring\n", i+1, err)
			if uerr := runUndo(ctx, r.steps, e.o.Out, e.o.AllowNonRoot); uerr != nil {
				return dir, fmt.Errorf("%w: %w; AND the restore failed: %v (by hand: unheaded-remediate undo %s)", errRestoreFailed, err, uerr, dir)
			}
			return dir, fmt.Errorf("%w (everything this apply changed was restored)", err)
		}
	}
	e.printf("  undo: unheaded-remediate undo %s\n", dir)
	return dir, nil
}

func (r *applyRun) apply(ctx context.Context, a remediation.Action) error {
	e := r.e
	switch {
	case a.File != nil:
		return r.applyFile(ctx, a.File)
	case a.Setting != nil:
		return r.applySetting(a.Setting)
	case a.Mode != nil:
		return r.applyMode(a.Mode)
	case a.Absent != "":
		full := e.path(a.Absent)
		fi, err := os.Lstat(full)
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		if err != nil {
			return err
		}
		if err := noSymlink(a.Absent, fi); err != nil {
			return err
		}
		if err := r.backup(a.Absent); err != nil {
			return err
		}
		e.printf("  removed %s\n", a.Absent)
		return os.Remove(full)
	case a.Service != nil:
		return r.applyService(ctx, a.Service)
	case a.Package != nil:
		if err := r.record(undoStep{Op: "run", Argv: []string{e.o.Cmd.Apt, "remove", "-y", a.Package.Name}}); err != nil {
			return err
		}
		e.printf("  installing %s\n", a.Package.Name)
		return runCmd(ctx, []string{e.o.Cmd.Apt, "install", "-y", "-q", a.Package.Name}, "DEBIAN_FRONTEND=noninteractive")
	case a.Run != "":
		argv, err := e.o.Cmd.argv(a.Run)
		if err != nil {
			return err
		}
		// Undo replays it after the files are restored (sysctl --system
		// reloads the restored values; a reload picks up restored config).
		if err := r.record(undoStep{Op: "run", Argv: argv}); err != nil {
			return err
		}
		return runCmd(ctx, argv)
	case a.Validate != "":
		return r.validate(ctx, a.Validate)
	}
	return fmt.Errorf("%w: empty action", ErrRefused)
}

func (r *applyRun) applyFile(ctx context.Context, fa *remediation.FileAction) error {
	e := r.e
	want, mode, err := e.render(fa)
	if err != nil {
		return err
	}
	uid, gid, err := e.owner(fa.Owner, fa.Group)
	if err != nil {
		return err
	}
	if fa.Precheck != "" {
		if err := e.precheck(ctx, fa.Precheck, want); err != nil {
			return err
		}
	}
	if err := r.backup(fa.Path); err != nil {
		return err
	}
	full := e.path(fa.Path)
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil { // #nosec G301 -- /etc subdirectories are world-readable by convention
		return err
	}
	if err := writeAtomic(full, want, mode, uid, gid, e.o.AllowNonRoot); err != nil {
		return err
	}
	e.printf("  wrote %s\n", fa.Path)
	return nil
}

// precheck validates content before it is installed, so a file that does
// not parse is never put in place (visudo -cf on a temp copy).
func (e *Executor) precheck(ctx context.Context, check string, content []byte) error {
	if check != "visudo -cf" {
		return fmt.Errorf("%w: precheck %q is not allowlisted", ErrRefused, check)
	}
	tmp, err := os.CreateTemp(e.o.State, ".precheck-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name()) // #nosec G104 -- best-effort cleanup of a private temp file
	if _, err := tmp.Write(content); err != nil {
		return errors.Join(err, tmp.Close())
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := runCmd(ctx, []string{e.o.Cmd.Visudo, "-cf", tmp.Name()}); err != nil {
		return fmt.Errorf("%w: the sudoers content does not parse; nothing installed: %v", ErrRefused, err)
	}
	return nil
}

func (r *applyRun) applySetting(s *remediation.SettingAction) error {
	e := r.e
	full := e.path(s.Path)
	fi, err := os.Lstat(full)
	if errors.Is(err, fs.ErrNotExist) {
		return nil // as remediate.sh: a setting in a file the package did not install is left alone
	}
	if err != nil {
		return err
	}
	if err := noSymlink(s.Path, fi); err != nil {
		return err
	}
	data, err := os.ReadFile(full) // #nosec G304 -- a model path under the configured root
	if err != nil {
		return err
	}
	lines := strings.SplitAfter(string(data), "\n")
	prefix := s.Key + "="
	for i, l := range lines {
		if strings.HasPrefix(l, prefix) {
			nl := ""
			if strings.HasSuffix(l, "\n") {
				nl = "\n"
			}
			lines[i] = prefix + s.Value + nl
		}
	}
	if err := r.backup(s.Path); err != nil {
		return err
	}
	uid, gid := ownerOf(fi)
	if err := writeAtomic(full, []byte(strings.Join(lines, "")), fi.Mode().Perm(), uid, gid, e.o.AllowNonRoot); err != nil {
		return err
	}
	e.printf("  %s: %s=%s\n", s.Path, s.Key, s.Value)
	return nil
}

func (r *applyRun) applyMode(ma *remediation.ModeAction) error {
	e := r.e
	perm, err := strconv.ParseUint(ma.Mode, 8, 32)
	if err != nil {
		return err
	}
	uid, gid, err := e.owner(ma.Owner, ma.Group)
	if err != nil {
		return err
	}
	for _, p := range e.modePaths(ma) {
		full := e.path(p)
		fi, err := os.Lstat(full)
		if errors.Is(err, fs.ErrNotExist) {
			continue // as remediate.sh: nothing to set on a path that is not there
		}
		if err != nil {
			return err
		}
		if err := noSymlink(p, fi); err != nil {
			return err
		}
		ou, og := ownerOf(fi)
		if err := r.record(undoStep{Op: "chmod", Path: full, Mode: uint32(fi.Mode().Perm()), UID: ou, GID: og}); err != nil {
			return err
		}
		if err := os.Chmod(full, os.FileMode(perm)); err != nil {
			return err
		}
		if err := os.Lchown(full, uid, gid); err != nil && !(e.o.AllowNonRoot && errors.Is(err, fs.ErrPermission)) {
			return err
		}
		e.printf("  set %s to %s %s:%s\n", p, ma.Mode, ma.Owner, ma.Group)
	}
	return nil
}

// applyService supports what the model uses today (disable --now, mask)
// and refuses anything else rather than guess at its undo.
func (r *applyRun) applyService(ctx context.Context, s *remediation.ServiceAction) error {
	e := r.e
	if s.Active != nil && *s.Active || s.Enabled != nil && *s.Enabled || s.Masked != nil && !*s.Masked {
		return fmt.Errorf("%w: service %s: only disable/stop and mask are supported", ErrRefused, s.Name)
	}
	if s.Enabled != nil || s.Active != nil {
		if err := r.record(undoStep{Op: "run", Argv: []string{e.o.Cmd.Systemctl, "enable", "--now", s.Name}}); err != nil {
			return err
		}
		if err := runCmd(ctx, []string{e.o.Cmd.Systemctl, "disable", "--now", s.Name}); err != nil {
			return err
		}
	}
	if s.Masked != nil {
		if err := r.record(undoStep{Op: "run", Argv: []string{e.o.Cmd.Systemctl, "unmask", s.Name}}); err != nil {
			return err
		}
		if err := runCmd(ctx, []string{e.o.Cmd.Systemctl, "mask", s.Name}); err != nil {
			return err
		}
	}
	e.printf("  %s disabled and masked\n", s.Name)
	return nil
}

var auditdHaltRe = regexp.MustCompile(`(?im)^[ \t]*(disk_full_action|disk_error_action|admin_space_left_action)[ \t]*=[ \t]*halt`)

func (r *applyRun) validate(ctx context.Context, v string) error {
	e := r.e
	if v == "auditd-never-halts" {
		b, err := os.ReadFile(e.path("/etc/audit/auditd.conf"))
		if err != nil && !errors.Is(err, fs.ErrNotExist) {
			return err
		}
		if auditdHaltRe.Match(b) {
			return fmt.Errorf("%w: auditd.conf halts the host on a full or failing disk (ADR-098 forbids it)", ErrRefused)
		}
		return nil
	}
	argv, err := e.o.Cmd.argv(v)
	if err != nil {
		return err
	}
	if err := runCmd(ctx, argv); err != nil {
		return fmt.Errorf("%w: %s failed: %v", ErrRefused, v, err)
	}
	e.printf("  %s: ok\n", v)
	return nil
}

func runCmd(ctx context.Context, argv []string, env ...string) error {
	cmd := exec.CommandContext(ctx, argv[0], argv[1:]...) // #nosec G204 -- argv comes from the allowlist and the model's validated names
	cmd.Env = append(os.Environ(), env...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s: %w: %s", strings.Join(argv, " "), err, bytes.TrimSpace(out))
	}
	return nil
}

// --- undo ---------------------------------------------------------------------

// Undo replays a backup directory's undo record, newest step first.
func Undo(ctx context.Context, dir string, out io.Writer, allowNonRoot bool) error {
	if os.Geteuid() != 0 && !allowNonRoot {
		return fmt.Errorf("%w: undo needs root", ErrRefused)
	}
	b, err := os.ReadFile(filepath.Join(dir, "undo.json")) // #nosec G304 -- an operator-named backup directory
	if err != nil {
		return err
	}
	var steps []undoStep
	if err := json.Unmarshal(b, &steps); err != nil {
		return fmt.Errorf("%s/undo.json: %w", dir, err)
	}
	if out == nil {
		out = io.Discard
	}
	return runUndo(ctx, steps, out, allowNonRoot)
}

func runUndo(ctx context.Context, steps []undoStep, out io.Writer, allowNonRoot bool) error {
	var errs []error
	for i := len(steps) - 1; i >= 0; i-- {
		s := steps[i]
		var err error
		switch s.Op {
		case "restore":
			var data []byte
			if data, err = os.ReadFile(s.Backup); err == nil {
				err = writeAtomic(s.Path, data, os.FileMode(s.Mode), s.UID, s.GID, allowNonRoot)
			}
		case "remove":
			if err = os.Remove(s.Path); errors.Is(err, fs.ErrNotExist) {
				err = nil
			}
		case "chmod":
			if err = os.Chmod(s.Path, os.FileMode(s.Mode)); err == nil && s.UID >= 0 {
				if cerr := os.Lchown(s.Path, s.UID, s.GID); cerr != nil && !(allowNonRoot && errors.Is(cerr, fs.ErrPermission)) {
					err = cerr
				}
			}
		case "run":
			err = runCmd(ctx, s.Argv)
		default:
			err = fmt.Errorf("unknown undo step %q", s.Op)
		}
		if err != nil {
			errs = append(errs, fmt.Errorf("undo %s %s: %w", s.Op, s.Path+strings.Join(s.Argv, " "), err))
		} else {
			fmt.Fprintf(out, "  undone: %s %s\n", s.Op, s.Path+strings.Join(s.Argv, " "))
		}
	}
	return errors.Join(errs...)
}
