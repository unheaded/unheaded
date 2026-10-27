// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package executor

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"unheaded/pkg/compliance/remediation"
)

// Check reports how the host differs from what applying fix would leave,
// one line per difference, and changes nothing. It reads files, modes and
// owners, and asks dpkg-query, systemctl and the kernel's AppArmor list
// about packages, units and profiles. A run action (a reload) leaves no
// state to compare and is not checked; validations are (sshd -t, visudo -c
// and the auditd halt rule only read).
func (e *Executor) Check(ctx context.Context, fix remediation.Fix) ([]string, error) {
	var devs []string
	add := func(format string, a ...any) { devs = append(devs, fmt.Sprintf(format, a...)) }
	for _, a := range fix.Actions {
		switch {
		case a.File != nil:
			want, mode, err := e.render(a.File)
			if err != nil {
				return nil, err
			}
			fi, err := os.Lstat(e.path(a.File.Path))
			if errors.Is(err, fs.ErrNotExist) {
				add("%s: missing", a.File.Path)
				continue
			}
			if err != nil {
				return nil, err
			}
			if !fi.Mode().IsRegular() {
				add("%s: not a regular file", a.File.Path)
				continue
			}
			have, err := os.ReadFile(e.path(a.File.Path))
			if err != nil {
				return nil, err
			}
			if !bytes.Equal(have, want) {
				add("%s: content differs from the model", a.File.Path)
			}
			if fi.Mode().Perm() != mode {
				add("%s: mode %04o, want %04o", a.File.Path, fi.Mode().Perm(), mode)
			}
			if err := e.checkOwner(a.File.Path, fi, a.File.Owner, a.File.Group, add); err != nil {
				return nil, err
			}
		case a.Setting != nil:
			b, err := os.ReadFile(e.path(a.Setting.Path))
			if errors.Is(err, fs.ErrNotExist) {
				continue // as apply: a setting in a file the package did not install is left alone
			}
			if err != nil {
				return nil, err
			}
			prefix := a.Setting.Key + "="
			for _, l := range strings.Split(string(b), "\n") {
				if strings.HasPrefix(l, prefix) && l != prefix+a.Setting.Value {
					add("%s: %s, want %s%s", a.Setting.Path, l, prefix, a.Setting.Value)
				}
			}
		case a.Mode != nil:
			for _, p := range e.modePaths(a.Mode) {
				fi, err := os.Lstat(e.path(p))
				if errors.Is(err, fs.ErrNotExist) {
					continue
				}
				if err != nil {
					return nil, err
				}
				want, err := strconv.ParseUint(a.Mode.Mode, 8, 32)
				if err != nil {
					return nil, err
				}
				if fi.Mode().Perm() != os.FileMode(want) {
					add("%s: mode %04o, want %s", p, fi.Mode().Perm(), a.Mode.Mode)
				}
				if err := e.checkOwner(p, fi, a.Mode.Owner, a.Mode.Group, add); err != nil {
					return nil, err
				}
			}
		case a.Absent != "":
			if _, err := os.Lstat(e.path(a.Absent)); err == nil {
				add("%s: present", a.Absent)
			} else if !errors.Is(err, fs.ErrNotExist) {
				return nil, err
			}
		case a.Package != nil:
			out := query(ctx, e.o.Cmd.DpkgQuery, "-W", "-f=${db:Status-Status}", a.Package.Name)
			if out != "installed" {
				add("package %s: %q, want installed", a.Package.Name, out)
			}
		case a.Service != nil:
			e.checkService(ctx, a.Service, add)
		case a.AppArmor != nil:
			if err := e.checkAppArmor(a.AppArmor, add); err != nil {
				return nil, err
			}
		case a.Validate == "auditd-never-halts":
			b, err := os.ReadFile(e.path("/etc/audit/auditd.conf"))
			if err != nil && !errors.Is(err, fs.ErrNotExist) {
				return nil, err
			}
			if auditdHaltRe.Match(b) {
				add("auditd.conf halts the host on a full or failing disk")
			}
		case a.Validate != "":
			argv, err := e.o.Cmd.argv(a.Validate)
			if err != nil {
				return nil, err
			}
			if err := runCmd(ctx, argv); err != nil {
				add("%s fails: %v", a.Validate, err)
			}
		}
	}
	return devs, ctx.Err()
}

// checkOwner compares owner and group by name; skipped for tests running
// without root, where nothing can be root-owned.
func (e *Executor) checkOwner(p string, fi fs.FileInfo, user, group string, add func(string, ...any)) error {
	if e.o.AllowNonRoot {
		return nil
	}
	uid, gid, err := e.owner(user, group)
	if err != nil {
		return err
	}
	if hu, hg := ownerOf(fi); hu != uid || hg != gid {
		add("%s: owner %d:%d, want %d:%d", p, hu, hg, uid, gid)
	}
	return nil
}

// query runs a read-only status command and returns its trimmed stdout;
// these commands exit non-zero for "disabled", "inactive" or an unknown
// package, which is an answer, not a failure.
func query(ctx context.Context, argv ...string) string {
	cmd := exec.CommandContext(ctx, argv[0], argv[1:]...) // #nosec G204 -- fixed status queries on model-validated names
	out, _ := cmd.Output()
	return strings.TrimSpace(string(out))
}

func (e *Executor) checkService(ctx context.Context, s *remediation.ServiceAction, add func(string, ...any)) {
	enabled := query(ctx, e.o.Cmd.Systemctl, "is-enabled", s.Name)
	active := query(ctx, e.o.Cmd.Systemctl, "is-active", s.Name)
	if s.Masked != nil && *s.Masked && enabled != "masked" {
		add("%s: is-enabled %q, want masked", s.Name, enabled)
	}
	if s.Enabled != nil && *s.Enabled != (enabled == "enabled") {
		add("%s: is-enabled %q, want enabled=%v", s.Name, enabled, *s.Enabled)
	}
	if s.Active != nil && *s.Active != (active == "active") {
		add("%s: is-active %q, want active=%v", s.Name, active, *s.Active)
	}
}

// checkAppArmor reads the kernel's loaded profiles ("<name> (<mode>)"),
// which needs root. A profile whose file is not installed is not checked,
// as apply skips it.
func (e *Executor) checkAppArmor(aa *remediation.AppArmorAction, add func(string, ...any)) error {
	profiles := aa.ProfilesFor(e.o.Host)
	if len(profiles) == 0 {
		return nil
	}
	b, err := os.ReadFile(e.path("/sys/kernel/security/apparmor/profiles"))
	if err != nil {
		return fmt.Errorf("read the loaded AppArmor profiles: %w", err)
	}
	loaded := map[string]string{}
	sc := bufio.NewScanner(bytes.NewReader(b))
	for sc.Scan() {
		l := sc.Text()
		if i := strings.LastIndex(l, " ("); i > 0 && strings.HasSuffix(l, ")") {
			loaded[l[:i]] = l[i+2 : len(l)-1]
		}
	}
	for _, name := range profiles {
		if _, err := os.Lstat(e.path("/etc/apparmor.d/" + name)); errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if mode := loaded[name]; mode != "enforce" {
			add("apparmor profile %s: %q, want enforce", name, mode)
		}
	}
	return nil
}

const maxAppliedBy = 30 // the operator field holds at most 100 characters

// RecordExisting journals fix as applied without changing the host: for a
// fix applied before the journal existed (remediate.sh, 2026-10-01). It
// refuses unless Check finds the host exactly as the fix leaves it. The
// entry is marked backfilled and its operator field says who applied it,
// when, and that it was recorded later.
func (e *Executor) RecordExisting(ctx context.Context, fix remediation.Fix, appliedBy string) error {
	if appliedBy == "" || len(appliedBy) > maxAppliedBy || strings.ContainsFunc(appliedBy, func(r rune) bool { return r < 0x20 || r == 0x7f }) {
		return fmt.Errorf("%w: -applied-by must be 1-%d printable characters", ErrRefused, maxAppliedBy)
	}
	devs, err := e.Check(ctx, fix)
	if err != nil {
		return err
	}
	if len(devs) > 0 {
		return fmt.Errorf("%w: %s is not in place on %s, so nothing was recorded:\n  %s", ErrRefused, fix.ID, e.o.Host, strings.Join(devs, "\n  "))
	}
	if err := os.MkdirAll(e.o.State, 0o700); err != nil {
		return err
	}
	if err := e.checkJournal(); err != nil {
		return err
	}
	sum, err := fix.SHA256()
	if err != nil {
		return err
	}
	op := fmt.Sprintf("%s; applied_by %s, recorded %s", e.o.Operator, appliedBy, e.o.Now().UTC().Format(time.DateOnly))
	if err := e.appendJournalEntry(remediation.JournalEntry{FixID: fix.ID, FixSHA256: sum, Action: "apply",
		Outcome: "success", Operator: op, Backfilled: true}); err != nil {
		return err
	}
	e.printf("recorded %s on %s as applied by %s (nothing changed)\n", fix.ID, e.o.Host, appliedBy)
	return nil
}
